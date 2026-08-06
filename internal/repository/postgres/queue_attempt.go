package postgres

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/Woolfer0097/GoodQueue/internal/pkg/domain"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
)

const (
	invitationEventType     = "queue.invited"
	terminalInviteExpired   = "invite_expired"
	terminalCheckoutExpired = "checkout_expired"
	terminalSoldOut         = "sold_out"
	terminalCancelled       = "cancelled"
)

type QueueAttemptRepository struct {
	db                   *sql.DB
	invitationTTL        time.Duration
	checkoutTTL          time.Duration
	waitingBufferPercent int
	afterProductSelected func(domain.ProductID)
}

func NewQueueAttemptRepository(
	db *sql.DB,
	invitationTTL time.Duration,
	checkoutTTL time.Duration,
	waitingBufferPercent int,
) *QueueAttemptRepository {
	return &QueueAttemptRepository{
		db:                   db,
		invitationTTL:        invitationTTL,
		checkoutTTL:          checkoutTTL,
		waitingBufferPercent: waitingBufferPercent,
	}
}

type lockedProduct struct {
	id                domain.ProductID
	allocatableStock  int32
	reserved          int32
	nextQueueSequence int64
	queueEnabled      bool
}

type transactionState struct {
	tx       *sql.Tx
	product  lockedProduct
	now      time.Time
	attempts []domain.QueueAttempt
}

func (repository *QueueAttemptRepository) Join(
	ctx context.Context,
	command domain.JoinQueueCommand,
) (domain.JoinQueueResult, error) {
	var result domain.JoinQueueResult
	var outcomeError error
	err := repository.withLockedProduct(ctx, command.ProductID, func(state *transactionState) error {
		if replay := findReplay(state.attempts, command.ExternalUserID, command.IdempotencyKey); replay != nil {
			result.Attempt = *replay
			if replay.State == domain.QueueAttemptWaiting {
				result.PositionAhead = countWaitingAhead(state.attempts, replay.QueueSequence)
			}
			return nil
		}
		if hasPurchasedAttempt(state.attempts, command.ExternalUserID) {
			outcomeError = domain.ErrAlreadyPurchased
			return nil
		}
		active := findActiveAttempt(state.attempts, command.ExternalUserID)
		if active != nil && isEffectiveAt(*active, state.now) {
			result.Attempt = *active
			if active.State == domain.QueueAttemptWaiting {
				result.PositionAhead = countWaitingAhead(state.attempts, active.QueueSequence)
			}
			return nil
		}
		if active != nil {
			if err := repository.reconcileLockedProduct(ctx, state); err != nil {
				return err
			}
			if hasPurchasedAttempt(state.attempts, command.ExternalUserID) {
				outcomeError = domain.ErrAlreadyPurchased
				return nil
			}
			if effective := findActiveAttempt(state.attempts, command.ExternalUserID); effective != nil {
				result.Attempt = *effective
				if effective.State == domain.QueueAttemptWaiting {
					result.PositionAhead = countWaitingAhead(state.attempts, effective.QueueSequence)
				}
				return nil
			}
		}
		if !state.product.queueEnabled {
			outcomeError = domain.ErrQueueDisabled
			return nil
		}
		if state.product.allocatableStock == 0 {
			if err := repository.reconcileLockedProduct(ctx, state); err != nil {
				return err
			}
			outcomeError = domain.ErrOutOfStock
			return nil
		}
		if err := repository.reconcileLockedProduct(ctx, state); err != nil {
			return err
		}

		attempt, err := repository.createAttempt(ctx, state, command)
		if err != nil {
			if errors.Is(err, domain.ErrQueueFull) {
				outcomeError = domain.ErrQueueFull
				return nil
			}
			return err
		}
		result = domain.JoinQueueResult{Attempt: attempt, Created: true}
		if attempt.State == domain.QueueAttemptWaiting {
			result.PositionAhead = countWaitingAhead(state.attempts, attempt.QueueSequence)
		}
		return nil
	})
	if err != nil {
		return domain.JoinQueueResult{}, err
	}
	return result, outcomeError
}

func (repository *QueueAttemptRepository) StartCheckout(
	ctx context.Context,
	command domain.StartCheckoutCommand,
) (domain.QueueAttempt, error) {
	productID, err := repository.findAttemptProduct(ctx, command.AttemptID)
	if err != nil {
		return domain.QueueAttempt{}, err
	}

	var result domain.QueueAttempt
	var outcomeError error
	err = repository.withLockedProduct(ctx, productID, func(state *transactionState) error {
		if err := repository.reconcileLockedProduct(ctx, state); err != nil {
			return err
		}
		attempt := findAttemptByID(state.attempts, command.AttemptID)
		if attempt == nil || attempt.ExternalUserID != command.ExternalUserID {
			outcomeError = domain.ErrAttemptNotFound
			return nil
		}
		result = *attempt
		switch attempt.State {
		case domain.QueueAttemptCheckout:
			return nil
		case domain.QueueAttemptInvited:
			checkoutDeadline := state.now.Add(repository.checkoutTTL)
			updated, updateErr := repository.transitionToCheckout(ctx, state, *attempt, checkoutDeadline)
			if updateErr != nil {
				return updateErr
			}
			result = updated
			return nil
		case domain.QueueAttemptInviteExpired, domain.QueueAttemptCheckoutExpired, domain.QueueAttemptSoldOut:
			outcomeError = domain.ErrAttemptGone
			return nil
		default:
			outcomeError = domain.ErrInvalidTransition
			return nil
		}
	})
	if err != nil {
		return domain.QueueAttempt{}, err
	}
	return result, outcomeError
}

func (repository *QueueAttemptRepository) Cancel(
	ctx context.Context,
	command domain.CancelQueueCommand,
) (domain.QueueAttempt, error) {
	var result domain.QueueAttempt
	var outcomeError error
	err := repository.withLockedProduct(ctx, command.ProductID, func(state *transactionState) error {
		if err := repository.reconcileLockedProduct(ctx, state); err != nil {
			return err
		}
		attempt := latestAttemptForUser(state.attempts, command.ExternalUserID)
		if attempt == nil {
			outcomeError = domain.ErrAttemptNotFound
			return nil
		}
		if attempt.State == domain.QueueAttemptPurchased {
			outcomeError = domain.ErrAlreadyPurchased
			return nil
		}
		if isTerminalState(attempt.State) {
			result = *attempt
			return nil
		}

		cancelled, err := repository.transitionToCancelled(ctx, state, *attempt)
		if err != nil {
			return err
		}
		result = cancelled
		return repository.reconcileLockedProduct(ctx, state)
	})
	if err != nil {
		return domain.QueueAttempt{}, err
	}
	return result, outcomeError
}

func (repository *QueueAttemptRepository) FindCurrent(
	ctx context.Context,
	productID domain.ProductID,
	externalUserID domain.ExternalUserID,
) (domain.CurrentQueueResult, error) {
	var result domain.CurrentQueueResult
	var outcomeError error
	err := repository.withLockedProduct(ctx, productID, func(state *transactionState) error {
		if err := repository.reconcileLockedProduct(ctx, state); err != nil {
			return err
		}
		attempt := findActiveAttempt(state.attempts, externalUserID)
		if attempt == nil {
			attempt = latestAttemptForUser(state.attempts, externalUserID)
		}
		if attempt == nil {
			outcomeError = domain.ErrAttemptNotFound
			return nil
		}
		result.Attempt = *attempt
		if attempt.State == domain.QueueAttemptWaiting {
			result.PositionAhead = countWaitingAhead(state.attempts, attempt.QueueSequence)
		}
		return nil
	})
	if err != nil {
		return domain.CurrentQueueResult{}, err
	}
	return result, outcomeError
}

func (repository *QueueAttemptRepository) AdjustStock(
	ctx context.Context,
	command domain.StockAdjustmentCommand,
) (domain.StockAdjustmentResult, error) {
	if command.Delta == 0 {
		return domain.StockAdjustmentResult{}, fmt.Errorf("%w: stock delta must not be zero", domain.ErrInvalidInput)
	}

	digest := domain.CanonicalStockAdjustmentHash(command)
	var result domain.StockAdjustmentResult
	var outcomeError error
	err := repository.withLockedProduct(ctx, command.ProductID, func(state *transactionState) error {
		replay, replayErr := repository.findStockAdjustment(ctx, state.tx, command.ProductID, command.IdempotencyKey)
		if replayErr != nil && !errors.Is(replayErr, sql.ErrNoRows) {
			return replayErr
		}
		if replayErr == nil {
			if !bytes.Equal(replay.payloadHash, digest[:]) {
				outcomeError = domain.ErrAdjustmentConflict
				return nil
			}
			result = replay.result
			result.Replayed = true
			if replay.status == "rejected" {
				outcomeError = rejectedAdjustmentError(result.ResponseBody)
			}
			return nil
		}

		stockAfter := int64(state.product.allocatableStock) + int64(command.Delta)
		if stockAfter < 0 {
			result, outcomeError = repository.persistRejectedAdjustment(ctx, state, command, digest, domain.ErrStockBelowZero)
			return nil
		}
		if stockAfter > math.MaxInt32 {
			result, outcomeError = repository.persistRejectedAdjustment(ctx, state, command, digest, domain.ErrStockOverflow)
			return nil
		}
		if stockAfter < int64(state.product.reserved) {
			result, outcomeError = repository.persistRejectedAdjustment(ctx, state, command, digest, domain.ErrStockBelowReserved)
			return nil
		}

		stockBefore := state.product.allocatableStock
		adjustedStock := int32(stockAfter) //nolint:gosec // Range checked against int32 bounds immediately above.
		stockUpdate, updateErr := state.tx.ExecContext(ctx, `
			UPDATE products SET allocatable_stock = $1, updated_at = $2
			WHERE id = $3 AND allocatable_stock = $4 AND reserved = $5`,
			adjustedStock, state.now, uuid.UUID(command.ProductID), stockBefore, state.product.reserved,
		)
		if updateErr != nil {
			return fmt.Errorf("update product stock: %w", updateErr)
		}
		if updateErr = requireOneRow(stockUpdate, "update product stock"); updateErr != nil {
			return updateErr
		}
		state.product.allocatableStock = adjustedStock
		if err := repository.reconcileLockedProduct(ctx, state); err != nil {
			return err
		}

		body, marshalErr := json.Marshal(struct {
			StockBefore int32 `json:"stock_before"`
			StockAfter  int32 `json:"stock_after"`
		}{StockBefore: stockBefore, StockAfter: adjustedStock})
		if marshalErr != nil {
			return fmt.Errorf("marshal stock adjustment response: %w", marshalErr)
		}
		adjustmentID := uuid.New()
		if _, insertErr := state.tx.ExecContext(ctx, `
			INSERT INTO inventory_adjustments (
				id, product_id, adjustment_key, delta, payload_hash, status,
				stock_before, stock_after, response_http_status, response_body, completed_at
			) VALUES ($1,$2,$3,$4,$5,'applied',$6,$7,200,$8,$9)`,
			adjustmentID, uuid.UUID(command.ProductID), string(command.IdempotencyKey), command.Delta,
			digest[:], stockBefore, adjustedStock, body, state.now,
		); insertErr != nil {
			return mapPostgreSQLError(insertErr)
		}
		result = domain.StockAdjustmentResult{
			AdjustmentID: adjustmentID,
			StockBefore:  stockBefore,
			StockAfter:   adjustedStock,
			HTTPStatus:   200,
			ResponseBody: body,
		}
		return nil
	})
	if err != nil {
		return domain.StockAdjustmentResult{}, err
	}
	return result, outcomeError
}

func (repository *QueueAttemptRepository) withLockedProduct(
	ctx context.Context,
	productID domain.ProductID,
	operation func(*transactionState) error,
) error {
	tx, err := repository.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin queue transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	product, err := lockProduct(ctx, tx, productID)
	if err != nil {
		return err
	}
	var now time.Time
	if err := tx.QueryRowContext(ctx, `SELECT clock_timestamp()`).Scan(&now); err != nil {
		return fmt.Errorf("read database clock: %w", err)
	}
	attempts, err := lockProductAttempts(ctx, tx, productID)
	if err != nil {
		return err
	}
	state := &transactionState{tx: tx, product: product, now: now, attempts: attempts}
	if err := operation(state); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return mapPostgreSQLError(err)
	}
	return nil
}

func (repository *QueueAttemptRepository) ReconcileNextProduct(
	ctx context.Context,
	transitionLimit int,
	excludedProductIDs []domain.ProductID,
) (domain.ReconciliationResult, error) {
	if transitionLimit <= 0 {
		return domain.ReconciliationResult{}, fmt.Errorf("%w: reconciliation transition limit must be positive", domain.ErrInvalidInput)
	}
	tx, err := repository.db.BeginTx(ctx, nil)
	if err != nil {
		return domain.ReconciliationResult{}, fmt.Errorf("begin background reconciliation: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	product, err := selectEligibleProduct(ctx, tx, excludedProductIDs)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.ReconciliationResult{}, nil
	}
	if err != nil {
		return domain.ReconciliationResult{}, err
	}
	if repository.afterProductSelected != nil {
		repository.afterProductSelected(product.id)
	}
	var now time.Time
	if err := tx.QueryRowContext(ctx, `SELECT clock_timestamp()`).Scan(&now); err != nil {
		return domain.ReconciliationResult{}, fmt.Errorf("read reconciliation clock after product lock: %w", err)
	}
	attempts, err := lockProductAttempts(ctx, tx, product.id)
	if err != nil {
		return domain.ReconciliationResult{}, err
	}
	state := &transactionState{tx: tx, product: product, now: now, attempts: attempts}
	transitions, moreWork, err := repository.reconcileLockedProductBounded(ctx, state, transitionLimit)
	if err != nil {
		return domain.ReconciliationResult{}, err
	}
	if err := tx.Commit(); err != nil {
		return domain.ReconciliationResult{}, mapPostgreSQLError(err)
	}
	return domain.ReconciliationResult{ProductID: product.id, Transitions: transitions, MoreWork: moreWork}, nil
}

func selectEligibleProduct(ctx context.Context, tx *sql.Tx, excluded []domain.ProductID) (lockedProduct, error) {
	arguments := make([]any, 0, len(excluded))
	placeholders := make([]string, 0, len(excluded))
	for index, productID := range excluded {
		arguments = append(arguments, uuid.UUID(productID))
		placeholders = append(placeholders, fmt.Sprintf("$%d", index+1))
	}
	exclusion := ""
	if len(placeholders) > 0 {
		exclusion = "AND p.id NOT IN (" + strings.Join(placeholders, ",") + ")"
	}
	query := `
		SELECT p.id, p.allocatable_stock, p.reserved, p.next_queue_sequence, p.queue_enabled
		FROM products p
		WHERE 1=1 ` + exclusion + `
			AND EXISTS (SELECT 1 FROM queue_attempts a WHERE a.product_id=p.id AND (
				(a.state='invited' AND a.invitation_deadline <= clock_timestamp()) OR
				(a.state='checkout' AND a.checkout_deadline <= clock_timestamp()) OR
				(a.state='waiting' AND (p.allocatable_stock=0 OR p.allocatable_stock-p.reserved>0))
			))
		ORDER BY p.updated_at, p.id
		FOR UPDATE OF p SKIP LOCKED
		LIMIT 1`
	var product lockedProduct
	var productID uuid.UUID
	err := tx.QueryRowContext(ctx, query, arguments...).Scan(
		&productID, &product.allocatableStock, &product.reserved, &product.nextQueueSequence, &product.queueEnabled,
	)
	product.id = domain.ProductID(productID)
	if err != nil {
		return lockedProduct{}, fmt.Errorf("select eligible product for reconciliation: %w", err)
	}
	return product, nil
}

func lockProduct(ctx context.Context, tx *sql.Tx, productID domain.ProductID) (lockedProduct, error) {
	var product lockedProduct
	product.id = productID
	err := tx.QueryRowContext(ctx, `
		SELECT allocatable_stock, reserved, next_queue_sequence, queue_enabled
		FROM products WHERE id = $1 FOR UPDATE`, uuid.UUID(productID)).Scan(
		&product.allocatableStock, &product.reserved, &product.nextQueueSequence, &product.queueEnabled,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return lockedProduct{}, domain.ErrNotFound
	}
	if err != nil {
		return lockedProduct{}, fmt.Errorf("lock product: %w", err)
	}
	return product, nil
}

func lockProductAttempts(ctx context.Context, tx *sql.Tx, productID domain.ProductID) ([]domain.QueueAttempt, error) {
	rows, err := tx.QueryContext(ctx, `
		SELECT id, product_id, queue_sequence, external_user_id, idempotency_key, state,
		       created_at, updated_at, invited_at, invitation_deadline, checkout_started_at,
		       checkout_deadline, terminal_at, purchased_at, accepted_payment_provider,
		       accepted_payment_reference, terminal_reason, terminal_message, version
		FROM queue_attempts WHERE product_id = $1 ORDER BY queue_sequence, id FOR UPDATE`, uuid.UUID(productID))
	if err != nil {
		return nil, fmt.Errorf("lock product queue attempts: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var attempts []domain.QueueAttempt
	for rows.Next() {
		attempt, scanErr := scanAttempt(rows)
		if scanErr != nil {
			return nil, fmt.Errorf("scan locked queue attempt: %w", scanErr)
		}
		attempts = append(attempts, attempt)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate locked queue attempts: %w", err)
	}
	return attempts, nil
}

func (repository *QueueAttemptRepository) reconcileLockedProduct(ctx context.Context, state *transactionState) error {
	_, _, err := repository.reconcileLockedProductBounded(ctx, state, len(state.attempts)*2+1)
	return err
}

func (repository *QueueAttemptRepository) reconcileLockedProductBounded(
	ctx context.Context,
	state *transactionState,
	transitionLimit int,
) (int, bool, error) {
	reservedCount := int32(0)
	for index := range state.attempts {
		if reservesStock(state.attempts[index].State) {
			reservedCount++
		}
	}
	if reservedCount != state.product.reserved || state.product.reserved < 0 || state.product.reserved > state.product.allocatableStock {
		return 0, false, fmt.Errorf("%w: product %s stores reserved=%d, active reservations=%d, stock=%d",
			domain.ErrReservedInvariant, uuid.UUID(state.product.id), state.product.reserved, reservedCount, state.product.allocatableStock)
	}

	originalReserved := state.product.reserved
	transitions := 0
	for index := range state.attempts {
		if transitions == transitionLimit {
			break
		}
		attempt := &state.attempts[index]
		if attempt.State == domain.QueueAttemptInvited && !state.now.Before(*attempt.InvitationDeadline) {
			if err := transitionToExpired(ctx, state.tx, attempt, state.now, domain.QueueAttemptInviteExpired, terminalInviteExpired); err != nil {
				return 0, false, err
			}
			state.product.reserved--
			transitions++
			continue
		}
		if attempt.State == domain.QueueAttemptCheckout && !state.now.Before(*attempt.CheckoutDeadline) {
			if err := transitionToExpired(ctx, state.tx, attempt, state.now, domain.QueueAttemptCheckoutExpired, terminalCheckoutExpired); err != nil {
				return 0, false, err
			}
			state.product.reserved--
			transitions++
		}
	}

	if state.product.allocatableStock == 0 {
		for index := range state.attempts {
			if transitions == transitionLimit {
				break
			}
			attempt := &state.attempts[index]
			if attempt.State != domain.QueueAttemptWaiting {
				continue
			}
			if err := transitionWaitingToSoldOut(ctx, state.tx, attempt, state.now); err != nil {
				return 0, false, err
			}
			transitions++
		}
	} else {
		available := state.product.allocatableStock - state.product.reserved
		for index := range state.attempts {
			if available == 0 || transitions == transitionLimit {
				break
			}
			attempt := &state.attempts[index]
			if attempt.State != domain.QueueAttemptWaiting {
				continue
			}
			if err := repository.promoteWaitingAttempt(ctx, state, attempt); err != nil {
				return 0, false, err
			}
			available--
			state.product.reserved++
			transitions++
		}
	}

	if transitions > 0 {
		result, err := state.tx.ExecContext(ctx, `
		UPDATE products SET reserved = $1, updated_at = $2
		WHERE id = $3 AND reserved = $4 AND $1 >= 0 AND $1 <= allocatable_stock`,
			state.product.reserved, state.now, uuid.UUID(state.product.id), originalReserved)
		if err != nil {
			return 0, false, fmt.Errorf("update reconciled product reservations: %w", err)
		}
		affected, err := result.RowsAffected()
		if err != nil {
			return 0, false, fmt.Errorf("read reconciled product update count: %w", err)
		}
		if affected != 1 {
			return 0, false, fmt.Errorf("%w: guarded reservation update affected %d rows", domain.ErrReservedInvariant, affected)
		}
	}
	return transitions, hasReconciliationWork(state), nil
}

func hasReconciliationWork(state *transactionState) bool {
	available := state.product.allocatableStock - state.product.reserved
	for index := range state.attempts {
		attempt := state.attempts[index]
		if attempt.State == domain.QueueAttemptInvited && !state.now.Before(*attempt.InvitationDeadline) {
			return true
		}
		if attempt.State == domain.QueueAttemptCheckout && !state.now.Before(*attempt.CheckoutDeadline) {
			return true
		}
		if attempt.State == domain.QueueAttemptWaiting && (state.product.allocatableStock == 0 || available > 0) {
			return true
		}
	}
	return false
}

func (repository *QueueAttemptRepository) createAttempt(
	ctx context.Context,
	state *transactionState,
	command domain.JoinQueueCommand,
) (domain.QueueAttempt, error) {
	nextSequence, err := domain.NextQueueSequence(state.product.nextQueueSequence)
	if err != nil {
		return domain.QueueAttempt{}, err
	}

	attempt := domain.QueueAttempt{
		ID: uuidToAttemptID(uuid.New()), ProductID: command.ProductID,
		QueueSequence: state.product.nextQueueSequence, ExternalUserID: command.ExternalUserID,
		IdempotencyKey: command.IdempotencyKey, CreatedAt: state.now, UpdatedAt: state.now, Version: 1,
	}
	if state.product.reserved < state.product.allocatableStock {
		attempt.State = domain.QueueAttemptCheckout
		attempt.InvitedAt = timePointer(state.now)
		attempt.InvitationDeadline = timePointer(state.now.Add(repository.invitationTTL))
		attempt.CheckoutStartedAt = timePointer(state.now)
		attempt.CheckoutDeadline = timePointer(state.now.Add(repository.checkoutTTL))
		if err := updateProductForNewAttempt(ctx, state, nextSequence, true); err != nil {
			return domain.QueueAttempt{}, err
		}
	} else {
		waitingCount := countWaiting(state.attempts)
		capacity, capacityErr := domain.WaitingCapacity(state.product.allocatableStock, repository.waitingBufferPercent)
		if capacityErr != nil {
			return domain.QueueAttempt{}, capacityErr
		}
		if waitingCount >= capacity {
			return domain.QueueAttempt{}, domain.ErrQueueFull
		}
		attempt.State = domain.QueueAttemptWaiting
		if err := updateProductForNewAttempt(ctx, state, nextSequence, false); err != nil {
			return domain.QueueAttempt{}, err
		}
	}

	_, err = state.tx.ExecContext(ctx, `
		INSERT INTO queue_attempts (
			id, product_id, queue_sequence, external_user_id, idempotency_key, state,
			created_at, updated_at, invited_at, invitation_deadline, checkout_started_at, checkout_deadline, version
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$7,$8,$9,$10,$11,1)`,
		uuid.UUID(attempt.ID), uuid.UUID(attempt.ProductID), attempt.QueueSequence, string(attempt.ExternalUserID),
		string(attempt.IdempotencyKey), string(attempt.State), state.now, attempt.InvitedAt,
		attempt.InvitationDeadline, attempt.CheckoutStartedAt, attempt.CheckoutDeadline,
	)
	if err != nil {
		return domain.QueueAttempt{}, mapPostgreSQLError(err)
	}
	state.attempts = append(state.attempts, attempt)
	return attempt, nil
}

func updateProductForNewAttempt(ctx context.Context, state *transactionState, nextSequence int64, reserve bool) error {
	reservedIncrement := int32(0)
	if reserve {
		reservedIncrement = 1
	}
	newReserved := state.product.reserved + reservedIncrement
	result, err := state.tx.ExecContext(ctx, `
		UPDATE products SET next_queue_sequence = $1, reserved = $2, updated_at = $3
		WHERE id = $4 AND next_queue_sequence = $5 AND reserved = $6
		  AND $2 >= 0 AND $2 <= allocatable_stock`,
		nextSequence, newReserved, state.now, uuid.UUID(state.product.id),
		state.product.nextQueueSequence, state.product.reserved,
	)
	if err != nil {
		return fmt.Errorf("advance product queue sequence: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("read sequence update count: %w", err)
	}
	if affected != 1 {
		return fmt.Errorf("%w: guarded sequence update affected %d rows", domain.ErrReservedInvariant, affected)
	}
	state.product.nextQueueSequence = nextSequence
	state.product.reserved = newReserved
	return nil
}

func (repository *QueueAttemptRepository) promoteWaitingAttempt(
	ctx context.Context,
	state *transactionState,
	attempt *domain.QueueAttempt,
) error {
	deadline := state.now.Add(repository.invitationTTL)
	newVersion := attempt.Version + 1
	result, err := state.tx.ExecContext(ctx, `
		UPDATE queue_attempts
		SET state='invited', invited_at=$1, invitation_deadline=$2, updated_at=$1, version=$3
		WHERE id=$4 AND state='waiting' AND version=$5`,
		state.now, deadline, newVersion, uuid.UUID(attempt.ID), attempt.Version)
	if err != nil {
		return fmt.Errorf("promote waiting queue attempt: %w", err)
	}
	if err := requireOneRow(result, "promote waiting queue attempt"); err != nil {
		return err
	}

	payload, err := json.Marshal(struct {
		AttemptID string    `json:"attempt_id"`
		ProductID string    `json:"product_id"`
		Deadline  time.Time `json:"invitation_deadline"`
	}{uuid.UUID(attempt.ID).String(), uuid.UUID(attempt.ProductID).String(), deadline})
	if err != nil {
		return fmt.Errorf("marshal invitation notification: %w", err)
	}
	payloadHash := sha256.Sum256(payload)
	deduplicationKey := fmt.Sprintf("queue.invited:%s:v%d", uuid.UUID(attempt.ID), newVersion)
	_, err = state.tx.ExecContext(ctx, `
		INSERT INTO notification_outbox (
			id, attempt_id, event_type, deduplication_key, payload, payload_hash, available_at, created_at, updated_at
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$7,$7)`,
		uuid.New(), uuid.UUID(attempt.ID), invitationEventType, deduplicationKey, string(payload), payloadHash[:], state.now)
	if err != nil {
		return mapPostgreSQLError(err)
	}
	attempt.State = domain.QueueAttemptInvited
	attempt.InvitedAt = timePointer(state.now)
	attempt.InvitationDeadline = timePointer(deadline)
	attempt.UpdatedAt = state.now
	attempt.Version = newVersion
	return nil
}

func transitionToExpired(
	ctx context.Context,
	tx *sql.Tx,
	attempt *domain.QueueAttempt,
	now time.Time,
	state domain.QueueAttemptState,
	reason string,
) error {
	result, err := tx.ExecContext(ctx, `
		UPDATE queue_attempts SET state=$1, terminal_at=$2, terminal_reason=$3,
		terminal_message=$4, updated_at=$2, version=version+1
		WHERE id=$5 AND state=$6 AND version=$7`,
		string(state), now, reason, reason, uuid.UUID(attempt.ID), string(attempt.State), attempt.Version)
	if err != nil {
		return fmt.Errorf("expire queue attempt: %w", err)
	}
	if err := requireOneRow(result, "expire queue attempt"); err != nil {
		return err
	}
	attempt.State = state
	attempt.TerminalAt = timePointer(now)
	attempt.TerminalReason = stringPointer(reason)
	attempt.TerminalMessage = stringPointer(reason)
	attempt.UpdatedAt = now
	attempt.Version++
	return nil
}

func transitionWaitingToSoldOut(ctx context.Context, tx *sql.Tx, attempt *domain.QueueAttempt, now time.Time) error {
	result, err := tx.ExecContext(ctx, `
		UPDATE queue_attempts SET state='sold_out', terminal_at=$1, terminal_reason=$2,
		terminal_message=$2, updated_at=$1, version=version+1
		WHERE id=$3 AND state='waiting' AND version=$4`, now, terminalSoldOut, uuid.UUID(attempt.ID), attempt.Version)
	if err != nil {
		return fmt.Errorf("mark waiting queue attempt sold out: %w", err)
	}
	if err := requireOneRow(result, "mark waiting queue attempt sold out"); err != nil {
		return err
	}
	attempt.State = domain.QueueAttemptSoldOut
	attempt.TerminalAt = timePointer(now)
	attempt.TerminalReason = stringPointer(terminalSoldOut)
	attempt.TerminalMessage = stringPointer(terminalSoldOut)
	attempt.UpdatedAt = now
	attempt.Version++
	return nil
}

func (repository *QueueAttemptRepository) transitionToCheckout(
	ctx context.Context,
	state *transactionState,
	attempt domain.QueueAttempt,
	deadline time.Time,
) (domain.QueueAttempt, error) {
	result, err := state.tx.ExecContext(ctx, `
		UPDATE queue_attempts SET state='checkout', checkout_started_at=$1,
		checkout_deadline=$2, updated_at=$1, version=version+1
		WHERE id=$3 AND state='invited' AND version=$4`,
		state.now, deadline, uuid.UUID(attempt.ID), attempt.Version)
	if err != nil {
		return domain.QueueAttempt{}, fmt.Errorf("start checkout: %w", err)
	}
	if err := requireOneRow(result, "start checkout"); err != nil {
		return domain.QueueAttempt{}, err
	}
	attempt.State = domain.QueueAttemptCheckout
	attempt.CheckoutStartedAt = timePointer(state.now)
	attempt.CheckoutDeadline = timePointer(deadline)
	attempt.UpdatedAt = state.now
	attempt.Version++
	return attempt, nil
}

func (repository *QueueAttemptRepository) transitionToCancelled(
	ctx context.Context,
	state *transactionState,
	attempt domain.QueueAttempt,
) (domain.QueueAttempt, error) {
	result, err := state.tx.ExecContext(ctx, `
		UPDATE queue_attempts SET state='cancelled', terminal_at=$1, terminal_reason=$2,
		terminal_message=$2, updated_at=$1, version=version+1
		WHERE id=$3 AND state=$4 AND version=$5`,
		state.now, terminalCancelled, uuid.UUID(attempt.ID), string(attempt.State), attempt.Version)
	if err != nil {
		return domain.QueueAttempt{}, fmt.Errorf("cancel queue attempt: %w", err)
	}
	if err := requireOneRow(result, "cancel queue attempt"); err != nil {
		return domain.QueueAttempt{}, err
	}
	if reservesStock(attempt.State) {
		productUpdate, updateErr := state.tx.ExecContext(ctx, `
			UPDATE products SET reserved=reserved-1, updated_at=$1
			WHERE id=$2 AND reserved=$3 AND reserved > 0`,
			state.now, uuid.UUID(state.product.id), state.product.reserved)
		if updateErr != nil {
			return domain.QueueAttempt{}, fmt.Errorf("release cancelled reservation: %w", updateErr)
		}
		if updateErr = requireOneRow(productUpdate, "release cancelled reservation"); updateErr != nil {
			return domain.QueueAttempt{}, updateErr
		}
		state.product.reserved--
	}
	attempt.State = domain.QueueAttemptCancelled
	attempt.TerminalAt = timePointer(state.now)
	attempt.TerminalReason = stringPointer(terminalCancelled)
	attempt.TerminalMessage = stringPointer(terminalCancelled)
	attempt.UpdatedAt = state.now
	attempt.Version++
	for index := range state.attempts {
		if state.attempts[index].ID == attempt.ID {
			state.attempts[index] = attempt
			break
		}
	}
	return attempt, nil
}

func (repository *QueueAttemptRepository) findAttemptProduct(ctx context.Context, attemptID domain.AttemptID) (domain.ProductID, error) {
	var productUUID uuid.UUID
	err := repository.db.QueryRowContext(ctx, `SELECT product_id FROM queue_attempts WHERE id=$1`, uuid.UUID(attemptID)).Scan(&productUUID)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.ProductID{}, domain.ErrAttemptNotFound
	}
	if err != nil {
		return domain.ProductID{}, fmt.Errorf("find queue attempt product: %w", err)
	}
	return domain.ProductID(productUUID), nil
}

type storedAdjustment struct {
	payloadHash []byte
	status      string
	result      domain.StockAdjustmentResult
}

func (repository *QueueAttemptRepository) findStockAdjustment(
	ctx context.Context,
	tx *sql.Tx,
	productID domain.ProductID,
	key domain.IdempotencyKey,
) (storedAdjustment, error) {
	var stored storedAdjustment
	var status int16
	var body []byte
	err := tx.QueryRowContext(ctx, `
		SELECT id, payload_hash, status, COALESCE(stock_before,0), COALESCE(stock_after,0),
		       response_http_status, response_body
		FROM inventory_adjustments WHERE product_id=$1 AND adjustment_key=$2`,
		uuid.UUID(productID), string(key)).Scan(
		&stored.result.AdjustmentID, &stored.payloadHash, &stored.status,
		&stored.result.StockBefore, &stored.result.StockAfter, &status, &body,
	)
	if err != nil {
		return storedAdjustment{}, err
	}
	stored.result.HTTPStatus = int(status)
	stored.result.ResponseBody = body
	return stored, nil
}

func (repository *QueueAttemptRepository) persistRejectedAdjustment(
	ctx context.Context,
	state *transactionState,
	command domain.StockAdjustmentCommand,
	digest [sha256.Size]byte,
	rejection error,
) (domain.StockAdjustmentResult, error) {
	status := 409
	body, marshalErr := json.Marshal(struct {
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}{Error: struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	}{Code: "stock_adjustment_rejected", Message: rejection.Error()}})
	if marshalErr != nil {
		return domain.StockAdjustmentResult{}, fmt.Errorf("marshal rejected stock adjustment response: %w", marshalErr)
	}
	adjustmentID := uuid.New()
	_, err := state.tx.ExecContext(ctx, `
		INSERT INTO inventory_adjustments (
			id, product_id, adjustment_key, delta, payload_hash, status,
			response_http_status, response_body, completed_at
		) VALUES ($1,$2,$3,$4,$5,'rejected',$6,$7,$8)`,
		adjustmentID, uuid.UUID(command.ProductID), string(command.IdempotencyKey), command.Delta,
		digest[:], status, body, state.now)
	if err != nil {
		return domain.StockAdjustmentResult{}, mapPostgreSQLError(err)
	}
	return domain.StockAdjustmentResult{
		AdjustmentID: adjustmentID,
		HTTPStatus:   status,
		ResponseBody: body,
	}, rejection
}

func rejectedAdjustmentError(body []byte) error {
	if bytes.Contains(body, []byte(domain.ErrStockBelowZero.Error())) {
		return domain.ErrStockBelowZero
	}
	if bytes.Contains(body, []byte(domain.ErrStockBelowReserved.Error())) {
		return domain.ErrStockBelowReserved
	}
	if bytes.Contains(body, []byte(domain.ErrStockOverflow.Error())) {
		return domain.ErrStockOverflow
	}
	return domain.ErrConflict
}

type rowScanner interface {
	Scan(...any) error
}

func scanAttempt(scanner rowScanner) (domain.QueueAttempt, error) {
	var attempt domain.QueueAttempt
	var id uuid.UUID
	var productID uuid.UUID
	err := scanner.Scan(
		&id, &productID, &attempt.QueueSequence, &attempt.ExternalUserID, &attempt.IdempotencyKey, &attempt.State,
		&attempt.CreatedAt, &attempt.UpdatedAt, &attempt.InvitedAt, &attempt.InvitationDeadline,
		&attempt.CheckoutStartedAt, &attempt.CheckoutDeadline, &attempt.TerminalAt, &attempt.PurchasedAt,
		&attempt.AcceptedProvider, &attempt.AcceptedReference, &attempt.TerminalReason, &attempt.TerminalMessage, &attempt.Version,
	)
	if err != nil {
		return domain.QueueAttempt{}, err
	}
	attempt.ID = uuidToAttemptID(id)
	attempt.ProductID = domain.ProductID(productID)
	return attempt, nil
}

func findReplay(attempts []domain.QueueAttempt, userID domain.ExternalUserID, key domain.IdempotencyKey) *domain.QueueAttempt {
	for index := range attempts {
		if attempts[index].ExternalUserID == userID && attempts[index].IdempotencyKey == key {
			return &attempts[index]
		}
	}
	return nil
}

func hasPurchasedAttempt(attempts []domain.QueueAttempt, userID domain.ExternalUserID) bool {
	for index := range attempts {
		if attempts[index].ExternalUserID == userID && attempts[index].State == domain.QueueAttemptPurchased {
			return true
		}
	}
	return false
}

func findActiveAttempt(attempts []domain.QueueAttempt, userID domain.ExternalUserID) *domain.QueueAttempt {
	for index := range attempts {
		if attempts[index].ExternalUserID == userID && isActiveState(attempts[index].State) {
			return &attempts[index]
		}
	}
	return nil
}

func latestAttemptForUser(attempts []domain.QueueAttempt, userID domain.ExternalUserID) *domain.QueueAttempt {
	for index := len(attempts) - 1; index >= 0; index-- {
		if attempts[index].ExternalUserID == userID {
			return &attempts[index]
		}
	}
	return nil
}

func findAttemptByID(attempts []domain.QueueAttempt, attemptID domain.AttemptID) *domain.QueueAttempt {
	for index := range attempts {
		if attempts[index].ID == attemptID {
			return &attempts[index]
		}
	}
	return nil
}

func countWaiting(attempts []domain.QueueAttempt) int64 {
	var count int64
	for index := range attempts {
		if attempts[index].State == domain.QueueAttemptWaiting {
			count++
		}
	}
	return count
}

func countWaitingAhead(attempts []domain.QueueAttempt, queueSequence int64) int64 {
	var count int64
	for index := range attempts {
		if attempts[index].State == domain.QueueAttemptWaiting && attempts[index].QueueSequence < queueSequence {
			count++
		}
	}
	return count
}

func isEffectiveAt(attempt domain.QueueAttempt, now time.Time) bool {
	switch attempt.State {
	case domain.QueueAttemptInvited:
		return now.Before(*attempt.InvitationDeadline)
	case domain.QueueAttemptCheckout:
		return now.Before(*attempt.CheckoutDeadline)
	case domain.QueueAttemptWaiting:
		return true
	default:
		return false
	}
}

func isActiveState(state domain.QueueAttemptState) bool {
	return state == domain.QueueAttemptWaiting || state == domain.QueueAttemptInvited || state == domain.QueueAttemptCheckout
}

func isTerminalState(state domain.QueueAttemptState) bool {
	return !isActiveState(state) && state != domain.QueueAttemptPurchased
}

func reservesStock(state domain.QueueAttemptState) bool {
	return state == domain.QueueAttemptInvited || state == domain.QueueAttemptCheckout
}

func requireOneRow(result sql.Result, operation string) error {
	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("%s affected rows: %w", operation, err)
	}
	if affected != 1 {
		return fmt.Errorf("%s affected %d rows: %w", operation, affected, domain.ErrConflict)
	}
	return nil
}

func mapPostgreSQLError(err error) error {
	var postgresError *pgconn.PgError
	if !errors.As(err, &postgresError) {
		return err
	}
	switch postgresError.ConstraintName {
	case "queue_attempts_idempotency_unique":
		return fmt.Errorf("join idempotency key already exists: %w", domain.ErrConflict)
	case "queue_attempts_one_active_per_user_product_idx":
		return fmt.Errorf("active queue attempt already exists: %w", domain.ErrConflict)
	case "queue_attempts_product_sequence_unique":
		return fmt.Errorf("queue sequence already exists: %w", domain.ErrConflict)
	case "queue_attempts_one_purchase_per_user_product_idx":
		return fmt.Errorf("purchase already exists: %w", domain.ErrAlreadyPurchased)
	case "queue_attempts_accepted_payment_reference_unique_idx":
		return fmt.Errorf("accepted payment reference already exists: %w", domain.ErrConflict)
	case "inventory_adjustments_product_key_unique":
		return fmt.Errorf("stock adjustment key already exists: %w", domain.ErrAdjustmentConflict)
	case "notification_outbox_deduplication_key_key":
		return fmt.Errorf("invitation notification already exists: %w", domain.ErrConflict)
	default:
		return fmt.Errorf("PostgreSQL constraint %s: %w", postgresError.ConstraintName, err)
	}
}

func timePointer(value time.Time) *time.Time           { return &value }
func stringPointer(value string) *string               { return &value }
func uuidToAttemptID(value uuid.UUID) domain.AttemptID { return domain.AttemptID(value) }
