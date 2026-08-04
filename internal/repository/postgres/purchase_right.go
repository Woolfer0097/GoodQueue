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
	tx, err := r.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return domain.PurchaseRight{}, oops.Wrapf(err, "begin transaction")
	}
	defer tx.Rollback()

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
