package postgres

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/Woolfer0097/GoodQueue/internal/pkg/domain"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
)

const (
	paymentClaimLease       = 30 * time.Second
	paymentRetryBackoff     = 5 * time.Second
	paymentCompensationType = "payment.compensation_required"
)

type storedPayment struct {
	id              uuid.UUID
	payloadHash     []byte
	status          string
	nextAttemptAt   time.Time
	leaseUntil      *time.Time
	claimToken      *uuid.UUID
	claimGeneration int64
	responseStatus  *int16
	responseBody    []byte
}

type validatedPaymentClaim struct {
	inboxID     uuid.UUID
	token       uuid.UUID
	generation  int64
	payloadHash [sha256.Size]byte
	command     domain.PaymentCommand
}

type lockedPaymentClaim struct {
	payloadHash      []byte
	payload          []byte
	provider         string
	eventID          string
	attemptID        uuid.UUID
	outcome          string
	paymentReference sql.NullString
	status           string
	token            *uuid.UUID
	generation       int64
}

func (repository *QueueAttemptRepository) ProcessPayment(
	ctx context.Context,
	command domain.PaymentCommand,
) (domain.PaymentResult, error) {
	claimResult, err := repository.ClaimPayment(ctx, command)
	if err != nil {
		return domain.PaymentResult{}, err
	}
	if claimResult.Response != nil {
		return *claimResult.Response, nil
	}
	if claimResult.Claim == nil {
		return domain.PaymentResult{}, fmt.Errorf("claim payment returned neither claim nor response: %w", domain.ErrInternal)
	}

	result, err := repository.ProcessPaymentClaim(ctx, *claimResult.Claim)
	if err == nil || errors.Is(err, domain.ErrStaleClaim) || errors.Is(err, domain.ErrReservedInvariant) ||
		errors.Is(err, domain.ErrPaymentClaimIntegrity) {
		return result, err
	}
	if retryErr := repository.RetryPaymentClaim(ctx, *claimResult.Claim, err); retryErr != nil && !errors.Is(retryErr, domain.ErrStaleClaim) {
		return domain.PaymentResult{}, errors.Join(err, retryErr)
	}
	return domain.PaymentResult{}, err
}

func (repository *QueueAttemptRepository) ClaimPayment(
	ctx context.Context,
	command domain.PaymentCommand,
) (domain.PaymentClaimResult, error) {
	payload := domain.CanonicalPaymentPayload(command)
	digest := domain.CanonicalPaymentHash(command)
	tx, err := repository.db.BeginTx(ctx, nil)
	if err != nil {
		return domain.PaymentClaimResult{}, fmt.Errorf("begin payment claim: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	inboxID := uuid.New()
	_, err = tx.ExecContext(ctx, `
		INSERT INTO payment_inbox (
			id, provider, event_id, attempt_id, outcome, payment_reference, payload, payload_hash
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8)
		ON CONFLICT (provider,event_id) DO NOTHING`,
		inboxID, command.Provider, command.EventID, uuid.UUID(command.AttemptID), string(command.Outcome),
		nullablePaymentReference(command), payload, digest[:])
	if err != nil {
		return domain.PaymentClaimResult{}, fmt.Errorf("insert payment inbox event: %w", err)
	}

	stored, err := lockStoredPayment(ctx, tx, command.Provider, command.EventID)
	if err != nil {
		return domain.PaymentClaimResult{}, err
	}
	var now time.Time
	if err := tx.QueryRowContext(ctx, `SELECT clock_timestamp()`).Scan(&now); err != nil {
		return domain.PaymentClaimResult{}, fmt.Errorf("read payment claim clock: %w", err)
	}
	if !bytes.Equal(stored.payloadHash, digest[:]) {
		if err := tx.Commit(); err != nil {
			return domain.PaymentClaimResult{}, mapPostgreSQLError(err)
		}
		result := paymentResult(stored.id, 409, "event_conflict", false)
		return domain.PaymentClaimResult{Response: &result}, nil
	}
	if stored.status == "completed" || stored.status == "rejected" {
		if err := tx.Commit(); err != nil {
			return domain.PaymentClaimResult{}, mapPostgreSQLError(err)
		}
		result := replayStoredPayment(stored)
		return domain.PaymentClaimResult{Response: &result}, nil
	}
	if stored.status == "processing" && stored.leaseUntil != nil && now.Before(*stored.leaseUntil) {
		if err := tx.Commit(); err != nil {
			return domain.PaymentClaimResult{}, mapPostgreSQLError(err)
		}
		result := paymentResult(stored.id, 202, "processing", false)
		return domain.PaymentClaimResult{Response: &result}, nil
	}
	if stored.status == "retry" && now.Before(stored.nextAttemptAt) {
		if err := tx.Commit(); err != nil {
			return domain.PaymentClaimResult{}, mapPostgreSQLError(err)
		}
		result := paymentResult(stored.id, 202, "processing", false)
		return domain.PaymentClaimResult{Response: &result}, nil
	}

	token := uuid.New()
	generation := stored.claimGeneration + 1
	leaseUntil := now.Add(paymentClaimLease)
	claimUpdate, err := tx.ExecContext(ctx, `
		UPDATE payment_inbox
		SET status='processing', claim_token=$1, claim_generation=$2, lease_until=$3,
			attempt_count=attempt_count+1, updated_at=$4, last_error=NULL
		WHERE id=$5 AND payload_hash=$6`, token, generation, leaseUntil, now, stored.id, digest[:])
	if err != nil {
		return domain.PaymentClaimResult{}, fmt.Errorf("claim payment inbox event: %w", err)
	}
	if err := requireOneRow(claimUpdate, "claim payment inbox event"); err != nil {
		return domain.PaymentClaimResult{}, err
	}
	if err := tx.Commit(); err != nil {
		return domain.PaymentClaimResult{}, mapPostgreSQLError(err)
	}
	return domain.PaymentClaimResult{Claim: &domain.PaymentClaim{
		InboxID: stored.id, Token: token, Generation: generation, PayloadHash: digest, Command: command,
	}}, nil
}

func (repository *QueueAttemptRepository) ProcessPaymentClaim(
	ctx context.Context,
	claim domain.PaymentClaim,
) (domain.PaymentResult, error) {
	validatedClaim, err := validatePaymentClaim(claim)
	if err != nil {
		return domain.PaymentResult{}, err
	}

	productID, err := repository.findAttemptProduct(ctx, validatedClaim.command.AttemptID)
	if errors.Is(err, domain.ErrAttemptNotFound) {
		return repository.finalizePaymentWithoutProduct(ctx, validatedClaim, 404, "attempt_not_found")
	}
	if err != nil {
		return domain.PaymentResult{}, err
	}

	result, err := repository.processClaimForProduct(ctx, productID, validatedClaim, false)
	if !isAcceptedPaymentReferenceConflict(err) {
		return result, err
	}
	return repository.processClaimForProduct(ctx, productID, validatedClaim, true)
}

func (repository *QueueAttemptRepository) RetryPaymentClaim(
	ctx context.Context,
	claim domain.PaymentClaim,
	processingError error,
) error {
	result, err := repository.db.ExecContext(ctx, `
		UPDATE payment_inbox
		SET status='retry', next_attempt_at=clock_timestamp()+$1,
			lease_until=NULL, claim_token=NULL, last_error=$2, updated_at=clock_timestamp()
		WHERE id=$3 AND status='processing' AND claim_token=$4 AND claim_generation=$5`,
		paymentRetryBackoff, processingError.Error(), claim.InboxID, claim.Token, claim.Generation)
	if err != nil {
		return fmt.Errorf("schedule payment retry: %w", err)
	}
	if err := requireOneRow(result, "schedule payment retry"); err != nil {
		return domain.ErrStaleClaim
	}
	return nil
}

func (repository *QueueAttemptRepository) processClaimForProduct(
	ctx context.Context,
	productID domain.ProductID,
	claim validatedPaymentClaim,
	forceReferenceCompensation bool,
) (domain.PaymentResult, error) {
	var result domain.PaymentResult
	err := repository.withLockedProduct(ctx, productID, attemptLockScope{attemptID: claim.command.AttemptID}, func(state *transactionState) error {
		if err := lockAndValidatePaymentClaim(ctx, state.tx, claim); err != nil {
			return err
		}
		if err := repository.reconcileLockedProduct(ctx, state); err != nil {
			return err
		}
		attempt := findAttemptByID(state.attempts, claim.command.AttemptID)
		if attempt == nil || attempt.ProductID != productID {
			return domain.ErrStaleClaim
		}

		if forceReferenceCompensation {
			return repository.completeWithCompensation(ctx, state, claim, attempt, &result)
		}
		if claim.command.Outcome == domain.PaymentSucceeded {
			return repository.applySucceededPayment(ctx, state, claim, attempt, &result)
		}
		return repository.applyFailedPayment(ctx, state, claim, attempt, &result)
	})
	return result, err
}

func (repository *QueueAttemptRepository) applySucceededPayment(
	ctx context.Context,
	state *transactionState,
	claim validatedPaymentClaim,
	attempt *domain.QueueAttempt,
	result *domain.PaymentResult,
) error {
	if attempt.State == domain.QueueAttemptPurchased {
		if attempt.AcceptedProvider != nil && attempt.AcceptedReference != nil &&
			*attempt.AcceptedProvider == claim.command.Provider && *attempt.AcceptedReference == claim.command.PaymentReference {
			return repository.completePayment(ctx, state.tx, claim, 200, "already_accepted", result)
		}
		return repository.completeWithCompensation(ctx, state, claim, attempt, result)
	}
	if attempt.State == domain.QueueAttemptWaiting || attempt.State == domain.QueueAttemptInvited {
		return repository.completeWithCompensation(ctx, state, claim, attempt, result)
	}
	if attempt.State != domain.QueueAttemptCheckout {
		return repository.completeWithCompensation(ctx, state, claim, attempt, result)
	}

	var owner uuid.UUID
	err := state.tx.QueryRowContext(ctx, `
		SELECT id FROM queue_attempts
		WHERE state='purchased' AND accepted_payment_provider=$1 AND accepted_payment_reference=$2`,
		claim.command.Provider, claim.command.PaymentReference).Scan(&owner)
	if err == nil && owner != uuid.UUID(attempt.ID) {
		return repository.completeWithCompensation(ctx, state, claim, attempt, result)
	}
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("find accepted payment owner: %w", err)
	}

	if state.product.allocatableStock <= 0 || state.product.reserved <= 0 {
		return fmt.Errorf("%w: purchase requires positive stock and reservation", domain.ErrReservedInvariant)
	}
	purchaseUpdate, err := state.tx.ExecContext(ctx, `
		UPDATE queue_attempts
		SET state='purchased', terminal_at=$1, purchased_at=$1, terminal_reason='purchased',
			terminal_message='purchased', accepted_payment_provider=$2, accepted_payment_reference=$3,
			updated_at=$1, version=version+1
		WHERE id=$4 AND state='checkout' AND version=$5`,
		state.now, claim.command.Provider, claim.command.PaymentReference, uuid.UUID(attempt.ID), attempt.Version)
	if err != nil {
		return err
	}
	if err := requireOneRow(purchaseUpdate, "accept payment purchase"); err != nil {
		return err
	}
	productUpdate, err := state.tx.ExecContext(ctx, `
		UPDATE products SET allocatable_stock=allocatable_stock-1, reserved=reserved-1, updated_at=$1
		WHERE id=$2 AND allocatable_stock=$3 AND reserved=$4 AND allocatable_stock>0 AND reserved>0`,
		state.now, uuid.UUID(state.product.id), state.product.allocatableStock, state.product.reserved)
	if err != nil {
		return fmt.Errorf("consume purchased inventory: %w", err)
	}
	if err := requireOneRow(productUpdate, "consume purchased inventory"); err != nil {
		return fmt.Errorf("%w: %w", domain.ErrReservedInvariant, err)
	}
	provider, reference := claim.command.Provider, claim.command.PaymentReference
	attempt.State = domain.QueueAttemptPurchased
	attempt.TerminalAt, attempt.PurchasedAt = timePointer(state.now), timePointer(state.now)
	attempt.TerminalReason, attempt.TerminalMessage = stringPointer("purchased"), stringPointer("purchased")
	attempt.AcceptedProvider, attempt.AcceptedReference = &provider, &reference
	attempt.UpdatedAt, attempt.Version = state.now, attempt.Version+1
	state.product.allocatableStock--
	state.product.reserved--
	if err := repository.reconcileLockedProduct(ctx, state); err != nil {
		return err
	}
	return repository.completePayment(ctx, state.tx, claim, 200, "accepted", result)
}

func (repository *QueueAttemptRepository) applyFailedPayment(
	ctx context.Context,
	state *transactionState,
	claim validatedPaymentClaim,
	attempt *domain.QueueAttempt,
	result *domain.PaymentResult,
) error {
	switch attempt.State {
	case domain.QueueAttemptCheckout:
		if state.product.reserved <= 0 {
			return fmt.Errorf("%w: failed checkout requires reservation", domain.ErrReservedInvariant)
		}
		attemptUpdate, err := state.tx.ExecContext(ctx, `
			UPDATE queue_attempts
			SET state='payment_failed', terminal_at=$1, terminal_reason='payment_failed',
				terminal_message='payment_failed', updated_at=$1, version=version+1
			WHERE id=$2 AND state='checkout' AND version=$3`, state.now, uuid.UUID(attempt.ID), attempt.Version)
		if err != nil {
			return fmt.Errorf("mark checkout payment failed: %w", err)
		}
		if err := requireOneRow(attemptUpdate, "mark checkout payment failed"); err != nil {
			return err
		}
		productUpdate, err := state.tx.ExecContext(ctx, `
			UPDATE products SET reserved=reserved-1, updated_at=$1
			WHERE id=$2 AND reserved=$3 AND reserved>0`, state.now, uuid.UUID(state.product.id), state.product.reserved)
		if err != nil {
			return fmt.Errorf("release failed payment reservation: %w", err)
		}
		if err := requireOneRow(productUpdate, "release failed payment reservation"); err != nil {
			return fmt.Errorf("%w: %w", domain.ErrReservedInvariant, err)
		}
		attempt.State = domain.QueueAttemptPaymentFailed
		attempt.TerminalAt = timePointer(state.now)
		attempt.TerminalReason, attempt.TerminalMessage = stringPointer("payment_failed"), stringPointer("payment_failed")
		attempt.UpdatedAt, attempt.Version = state.now, attempt.Version+1
		state.product.reserved--
		if err := repository.reconcileLockedProduct(ctx, state); err != nil {
			return err
		}
		return repository.completePayment(ctx, state.tx, claim, 200, "accepted", result)
	case domain.QueueAttemptPurchased:
		return repository.completePayment(ctx, state.tx, claim, 200, "ignored_purchase_final", result)
	case domain.QueueAttemptWaiting, domain.QueueAttemptInvited:
		return repository.completePayment(ctx, state.tx, claim, 409, "invalid_transition", result)
	default:
		return repository.completePayment(ctx, state.tx, claim, 200, "ignored_terminal", result)
	}
}

func (repository *QueueAttemptRepository) completeWithCompensation(
	ctx context.Context,
	state *transactionState,
	claim validatedPaymentClaim,
	attempt *domain.QueueAttempt,
	result *domain.PaymentResult,
) error {
	payload, err := json.Marshal(struct {
		PaymentInboxID   string `json:"payment_inbox_id"`
		Provider         string `json:"provider"`
		EventID          string `json:"event_id"`
		AttemptID        string `json:"attempt_id"`
		PaymentReference string `json:"payment_reference"`
	}{claim.inboxID.String(), claim.command.Provider, claim.command.EventID,
		uuid.UUID(claim.command.AttemptID).String(), claim.command.PaymentReference})
	if err != nil {
		return fmt.Errorf("marshal payment compensation: %w", err)
	}
	payloadHash := sha256.Sum256(payload)
	deduplicationKey := "payment.compensation:" + claim.inboxID.String()
	_, err = state.tx.ExecContext(ctx, `
		INSERT INTO notification_outbox (
			id, attempt_id, event_type, deduplication_key, payload, payload_hash,
			available_at, created_at, updated_at
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$7,$7)
		ON CONFLICT (deduplication_key) DO NOTHING`, uuid.New(), uuid.UUID(attempt.ID), paymentCompensationType,
		deduplicationKey, payload, payloadHash[:], state.now)
	if err != nil {
		return fmt.Errorf("enqueue payment compensation: %w", err)
	}
	return repository.completePayment(ctx, state.tx, claim, 200, "compensation_required", result)
}

func (repository *QueueAttemptRepository) completePayment(
	ctx context.Context,
	tx *sql.Tx,
	claim validatedPaymentClaim,
	httpStatus int,
	code string,
	result *domain.PaymentResult,
) error {
	response := paymentResult(claim.inboxID, httpStatus, code, false)
	status := "completed"
	if httpStatus >= 400 {
		status = "rejected"
	}
	update, err := tx.ExecContext(ctx, `
		UPDATE payment_inbox
		SET status=$1, response_http_status=$2, response_body=$3, completed_at=clock_timestamp(),
			updated_at=clock_timestamp(), lease_until=NULL, claim_token=NULL, last_error=NULL
		WHERE id=$4 AND status='processing' AND claim_token=$5 AND claim_generation=$6`,
		status, httpStatus, response.ResponseBody, claim.inboxID, claim.token, claim.generation)
	if err != nil {
		return fmt.Errorf("complete payment inbox event: %w", err)
	}
	if err := requireOneRow(update, "complete payment inbox event"); err != nil {
		return domain.ErrStaleClaim
	}
	*result = response
	return nil
}

func (repository *QueueAttemptRepository) finalizePaymentWithoutProduct(
	ctx context.Context,
	claim validatedPaymentClaim,
	httpStatus int,
	code string,
) (domain.PaymentResult, error) {
	tx, err := repository.db.BeginTx(ctx, nil)
	if err != nil {
		return domain.PaymentResult{}, fmt.Errorf("begin payment finalization: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if err := lockAndValidatePaymentClaim(ctx, tx, claim); err != nil {
		return domain.PaymentResult{}, err
	}
	var result domain.PaymentResult
	if err := repository.completePayment(ctx, tx, claim, httpStatus, code, &result); err != nil {
		return domain.PaymentResult{}, err
	}
	if err := tx.Commit(); err != nil {
		return domain.PaymentResult{}, mapPostgreSQLError(err)
	}
	return result, nil
}

func lockStoredPayment(ctx context.Context, tx *sql.Tx, provider, eventID string) (storedPayment, error) {
	var stored storedPayment
	err := tx.QueryRowContext(ctx, `
		SELECT id, payload_hash, status, next_attempt_at, lease_until, claim_token,
			claim_generation, response_http_status, response_body
		FROM payment_inbox WHERE provider=$1 AND event_id=$2 FOR UPDATE`, provider, eventID).Scan(
		&stored.id, &stored.payloadHash, &stored.status, &stored.nextAttemptAt, &stored.leaseUntil,
		&stored.claimToken, &stored.claimGeneration, &stored.responseStatus, &stored.responseBody)
	if err != nil {
		return storedPayment{}, fmt.Errorf("lock payment inbox event: %w", err)
	}
	return stored, nil
}

func validatePaymentClaim(claim domain.PaymentClaim) (validatedPaymentClaim, error) {
	recomputedHash := domain.CanonicalPaymentHash(claim.Command)
	if recomputedHash != claim.PayloadHash {
		return validatedPaymentClaim{}, paymentClaimIntegrityError("supplied command does not match its payload hash")
	}
	return validatedPaymentClaim{
		inboxID: claim.InboxID, token: claim.Token, generation: claim.Generation,
		payloadHash: recomputedHash, command: claim.Command,
	}, nil
}

func lockAndValidatePaymentClaim(ctx context.Context, tx *sql.Tx, claim validatedPaymentClaim) error {
	var stored lockedPaymentClaim
	err := tx.QueryRowContext(ctx, `
		SELECT payload_hash, payload, provider, event_id, attempt_id, outcome, payment_reference,
			status, claim_token, claim_generation
		FROM payment_inbox WHERE id=$1 FOR UPDATE`, claim.inboxID).Scan(
		&stored.payloadHash, &stored.payload, &stored.provider, &stored.eventID, &stored.attemptID,
		&stored.outcome, &stored.paymentReference, &stored.status, &stored.token, &stored.generation,
	)
	if err != nil {
		return fmt.Errorf("lock claimed payment inbox event: %w", err)
	}
	if stored.status != "processing" || stored.token == nil || *stored.token != claim.token || stored.generation != claim.generation {
		return domain.ErrStaleClaim
	}
	if !bytes.Equal(stored.payloadHash, claim.payloadHash[:]) {
		return paymentClaimIntegrityError("stored payload hash does not match claimed command")
	}
	if !bytes.Equal(stored.payload, domain.CanonicalPaymentPayload(claim.command)) {
		return paymentClaimIntegrityError("stored canonical payload does not match claimed command")
	}
	if stored.provider != claim.command.Provider || stored.eventID != claim.command.EventID ||
		stored.attemptID != uuid.UUID(claim.command.AttemptID) || stored.outcome != string(claim.command.Outcome) ||
		!storedPaymentReferenceMatches(stored.paymentReference, claim.command) {
		return paymentClaimIntegrityError("stored payment fields do not match claimed command")
	}
	return nil
}

func storedPaymentReferenceMatches(stored sql.NullString, command domain.PaymentCommand) bool {
	if command.Outcome == domain.PaymentFailed {
		return !stored.Valid
	}
	return stored.Valid && stored.String == command.PaymentReference
}

func paymentClaimIntegrityError(reason string) error {
	return fmt.Errorf("%w: %w: %s", domain.ErrInternal, domain.ErrPaymentClaimIntegrity, reason)
}

func paymentResult(inboxID uuid.UUID, httpStatus int, code string, replayed bool) domain.PaymentResult {
	body, err := json.Marshal(struct {
		Code string `json:"code"`
	}{Code: code})
	if err != nil {
		panic(fmt.Sprintf("marshal deterministic payment response: %v", err))
	}
	return domain.PaymentResult{InboxID: inboxID, Code: code, HTTPStatus: httpStatus, ResponseBody: body, Replayed: replayed}
}

func replayStoredPayment(stored storedPayment) domain.PaymentResult {
	status := 0
	if stored.responseStatus != nil {
		status = int(*stored.responseStatus)
	}
	var response struct {
		Code string `json:"code"`
	}
	_ = json.Unmarshal(stored.responseBody, &response)
	return domain.PaymentResult{
		InboxID: stored.id, Code: response.Code, HTTPStatus: status,
		ResponseBody: append([]byte(nil), stored.responseBody...), Replayed: true,
	}
}

func nullablePaymentReference(command domain.PaymentCommand) any {
	if command.Outcome == domain.PaymentFailed {
		return nil
	}
	return command.PaymentReference
}

func isAcceptedPaymentReferenceConflict(err error) bool {
	var postgresError *pgconn.PgError
	return errors.As(err, &postgresError) && postgresError.ConstraintName == "queue_attempts_accepted_payment_reference_unique_idx"
}
