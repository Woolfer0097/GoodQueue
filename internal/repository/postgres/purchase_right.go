package postgres

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/Woolfer0097/GoodQueue/internal/pkg/domain"
	"github.com/google/uuid"
	"github.com/samber/oops"
)

type PurchaseRightRepository struct{ db *sql.DB }

func NewPurchaseRightRepository(db *sql.DB) *PurchaseRightRepository {
	return &PurchaseRightRepository{db: db}
}

func (r *PurchaseRightRepository) ActiveForUserAndProduct(ctx context.Context, userID domain.ExternalUserID, productID domain.ProductID) (domain.PurchaseRight, error) {
	var right domain.PurchaseRight
	var idStr string
	var productIDStr string
	err := r.db.QueryRowContext(ctx, `
		SELECT pr.id, pr.queue_ticket_id, pr.product_id, pr.status, pr.issued_at, pr.expires_at, pr.consumed_at
		FROM purchase_rights pr
		JOIN queue_entries qe ON pr.queue_ticket_id = qe.ticket_id
		WHERE qe.external_user_id = $1 AND qe.product_id = $2 AND pr.status = 'active'
		LIMIT 1
	`, string(userID), uuid.UUID(productID).String()).Scan(
		&idStr, &right.QueueTicketID, &productIDStr, &right.Status,
		&right.IssuedAt, &right.ExpiresAt, &right.ConsumedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.PurchaseRight{}, oops.Code("not_found").Wrapf(domain.ErrNotFound, "no active right for user %s product %s", userID, uuid.UUID(productID))
	}
	if err != nil {
		return domain.PurchaseRight{}, oops.Wrapf(err, "get active right")
	}
	right.ID, err = uuid.Parse(idStr)
	if err != nil {
		return domain.PurchaseRight{}, oops.Wrapf(err, "parse purchase right ID")
	}
	pid, err := uuid.Parse(productIDStr)
	if err != nil {
		return domain.PurchaseRight{}, oops.Wrapf(err, "parse product ID")
	}
	right.ProductID = domain.ProductID(pid)
	return right, nil
}

func (r *PurchaseRightRepository) AcquireRight(ctx context.Context, queueTicketID int64, productID domain.ProductID, ttlSeconds int) (domain.PurchaseRight, error) {
	tx, err := r.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if err != nil {
		return domain.PurchaseRight{}, oops.Wrapf(err, "begin transaction")
	}
	defer func() { _ = tx.Rollback() }()

	var allocatableStock, reserved int
	err = tx.QueryRowContext(ctx, `
		SELECT allocatable_stock, reserved FROM products WHERE id = $1 FOR UPDATE
	`, uuid.UUID(productID).String()).Scan(&allocatableStock, &reserved)
	if err != nil {
		return domain.PurchaseRight{}, oops.Wrapf(err, "lock product")
	}
	if allocatableStock-reserved <= 0 {
		return domain.PurchaseRight{}, oops.Code("out_of_stock").Wrap(domain.ErrOutOfStock)
	}

	newReserved := reserved + 1
	_, err = tx.ExecContext(ctx, `
		UPDATE products SET reserved = $1, updated_at = NOW() WHERE id = $2
	`, newReserved, uuid.UUID(productID).String())
	if err != nil {
		return domain.PurchaseRight{}, oops.Wrapf(err, "update reserved")
	}

	rightID := uuid.New()
	now := time.Now()
	expiresAt := now.Add(time.Duration(ttlSeconds) * time.Second)

	var right domain.PurchaseRight
	var idStr string
	var productIDStr string
	err = tx.QueryRowContext(ctx, `
		INSERT INTO purchase_rights (id, queue_ticket_id, product_id, status, issued_at, expires_at)
		VALUES ($1, $2, $3, 'active', $4, $5)
		RETURNING id, queue_ticket_id, product_id, status, issued_at, expires_at, consumed_at
	`, rightID, queueTicketID, uuid.UUID(productID).String(), now, expiresAt).Scan(
		&idStr, &right.QueueTicketID, &productIDStr, &right.Status,
		&right.IssuedAt, &right.ExpiresAt, &right.ConsumedAt,
	)
	if err != nil {
		return domain.PurchaseRight{}, oops.Wrapf(err, "insert purchase right")
	}
	right.ID, err = uuid.Parse(idStr)
	if err != nil {
		return domain.PurchaseRight{}, oops.Wrapf(err, "parse right ID")
	}
	pid, err := uuid.Parse(productIDStr)
	if err != nil {
		return domain.PurchaseRight{}, oops.Wrapf(err, "parse product ID")
	}
	right.ProductID = domain.ProductID(pid)

	_, err = tx.ExecContext(ctx, `
		UPDATE queue_entries
		SET status = 'right_issued', right_issued_at = $1
		WHERE ticket_id = $2
	`, now, queueTicketID)
	if err != nil {
		return domain.PurchaseRight{}, oops.Wrapf(err, "update queue entry to right_issued")
	}

	if err := tx.Commit(); err != nil {
		return domain.PurchaseRight{}, oops.Wrapf(err, "commit transaction")
	}
	return right, nil
}

func (r *PurchaseRightRepository) ReleaseRight(ctx context.Context, queueTicketID int64, finalStatus domain.QueueEntryStatus) error {
	tx, err := r.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if err != nil {
		return oops.Wrapf(err, "begin transaction")
	}
	defer func() { _ = tx.Rollback() }()

	// Получаем активное право для этого ticket_id
	var rightID string
	var productIDStr string
	var expiresAt time.Time
	err = tx.QueryRowContext(ctx, `
		SELECT id, product_id, expires_at FROM purchase_rights
		WHERE queue_ticket_id = $1 AND status = 'active'
		FOR UPDATE
	`, queueTicketID).Scan(&rightID, &productIDStr, &expiresAt)
	if errors.Is(err, sql.ErrNoRows) {
		return oops.Code("not_found").Wrapf(domain.ErrNotFound, "no active right for ticket %d", queueTicketID)
	}
	if err != nil {
		return oops.Wrapf(err, "query active right")
	}

	if finalStatus == domain.QueueEntryCompleted && !expiresAt.After(time.Now()) {
		return oops.Code("grant_expired").Wrap(domain.ErrGrantExpired)
	}

	// Определяем финальный статус для purchase_rights
	var prStatus domain.PurchaseRightStatus
	switch finalStatus {
	case domain.QueueEntryCompleted:
		prStatus = domain.PurchaseRightConsumed
	case domain.QueueEntryExpired:
		prStatus = domain.PurchaseRightExpired
	case domain.QueueEntryCancelled:
		prStatus = domain.PurchaseRightExpired // или можно сделать отдельный статус, но по логике expired
	default:
		return oops.Code("invalid_input").Wrapf(domain.ErrInvalidInput, "cannot release right with status %s", finalStatus)
	}

	// Обновляем purchase_rights
	var updateQuery string
	if prStatus == domain.PurchaseRightConsumed {
		updateQuery = `
			UPDATE purchase_rights
			SET status = 'consumed', consumed_at = NOW()
			WHERE queue_ticket_id = $1
		`
	} else {
		updateQuery = `
			UPDATE purchase_rights
			SET status = 'expired'
			WHERE queue_ticket_id = $1
		`
	}
	_, err = tx.ExecContext(ctx, updateQuery, queueTicketID)
	if err != nil {
		return oops.Wrapf(err, "update purchase right")
	}

	// Обновляем queue_entries (заодно и статус, и completed_at/expired_at/cancelled_at)
	var setClause string
	switch finalStatus {
	case domain.QueueEntryCompleted:
		setClause = "status = 'completed', completed_at = NOW()"
	case domain.QueueEntryExpired:
		setClause = "status = 'expired', expired_at = NOW()"
	case domain.QueueEntryCancelled:
		setClause = "status = 'cancelled', cancelled_at = NOW(), right_issued_at = NULL"
	default:
		return oops.Code("invalid_input").Wrapf(domain.ErrInvalidInput, "unknown final status")
	}
	// #nosec G202 -- setClause is always one of the fixed strings assigned above, never user input.
	_, err = tx.ExecContext(ctx, `
		UPDATE queue_entries
		SET `+setClause+`
		WHERE ticket_id = $1
	`, queueTicketID)
	if err != nil {
		return oops.Wrapf(err, "update queue entry")
	}

	if finalStatus != domain.QueueEntryCompleted {
		productID, err := uuid.Parse(productIDStr)
		if err != nil {
			return oops.Wrapf(err, "parse product ID")
		}
		_, err = tx.ExecContext(ctx, `
			UPDATE products
			SET reserved = reserved - 1, updated_at = NOW()
			WHERE id = $1 AND reserved > 0
		`, productID)
		if err != nil {
			return oops.Wrapf(err, "decrement reserved")
		}
	}

	if err := tx.Commit(); err != nil {
		return oops.Wrapf(err, "commit transaction")
	}
	return nil
}

func (r *PurchaseRightRepository) ListExpiredActiveRights(ctx context.Context, before time.Time) ([]domain.PurchaseRight, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, queue_ticket_id, product_id, status, issued_at, expires_at, consumed_at
		FROM purchase_rights
		WHERE status = 'active' AND expires_at <= $1
		ORDER BY expires_at ASC, id ASC
	`, before)
	if err != nil {
		return nil, oops.Wrapf(err, "query expired active rights")
	}
	defer func() { _ = rows.Close() }()

	var rights []domain.PurchaseRight
	for rows.Next() {
		var right domain.PurchaseRight
		var idStr string
		var productIDStr string
		if err := rows.Scan(
			&idStr, &right.QueueTicketID, &productIDStr, &right.Status,
			&right.IssuedAt, &right.ExpiresAt, &right.ConsumedAt,
		); err != nil {
			return nil, oops.Wrapf(err, "scan expired active right")
		}
		right.ID, err = uuid.Parse(idStr)
		if err != nil {
			return nil, oops.Wrapf(err, "parse purchase right ID")
		}
		pid, err := uuid.Parse(productIDStr)
		if err != nil {
			return nil, oops.Wrapf(err, "parse product ID")
		}
		right.ProductID = domain.ProductID(pid)
		rights = append(rights, right)
	}
	if err = rows.Err(); err != nil {
		return nil, oops.Wrapf(err, "rows iteration")
	}
	return rights, nil
}
