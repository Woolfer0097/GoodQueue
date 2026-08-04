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

type QueueRepository struct{ db *sql.DB }

func NewQueueRepository(db *sql.DB) *QueueRepository {
	return &QueueRepository{db: db}
}

func (r *QueueRepository) Join(ctx context.Context, productID domain.ProductID, userID domain.ExternalUserID, idempotencyKey uuid.UUID) (domain.QueueEntry, error) {
	var existing domain.QueueEntry
	err := r.db.QueryRowContext(ctx, `
		SELECT ticket_id, product_id, external_user_id, status, joined_at, right_issued_at, completed_at, cancelled_at, expired_at
		FROM queue_entries
		WHERE product_id = $1 AND external_user_id = $2 AND status IN ('waiting', 'right_issued')
	`, uuid.UUID(productID).String(), string(userID)).Scan(
		&existing.TicketID, &existing.ProductID, &existing.ExternalUserID, &existing.Status,
		&existing.JoinedAt, &existing.RightIssuedAt, &existing.CompletedAt, &existing.CancelledAt, &existing.ExpiredAt,
	)
	if err == nil {
		return existing, oops.Code("conflict").Wrapf(domain.ErrConflict, "user already has active entry for product")
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return domain.QueueEntry{}, oops.Wrapf(err, "check existing entry")
	}

	var entry domain.QueueEntry
	var idStr string
	now := time.Now()
	err = r.db.QueryRowContext(ctx, `
		INSERT INTO queue_entries (product_id, external_user_id, idempotency_key, status, joined_at)
		VALUES ($1, $2, $3, 'waiting', $4)
		RETURNING ticket_id, product_id, external_user_id, status, joined_at, right_issued_at, completed_at, cancelled_at, expired_at
	`, uuid.UUID(productID).String(), string(userID), idempotencyKey, now).Scan(
		&entry.TicketID, &idStr, &entry.ExternalUserID, &entry.Status,
		&entry.JoinedAt, &entry.RightIssuedAt, &entry.CompletedAt, &entry.CancelledAt, &entry.ExpiredAt,
	)
	if err != nil {
		return domain.QueueEntry{}, oops.Wrapf(err, "insert queue entry")
	}
	pid, err := uuid.Parse(idStr)
	if err != nil {
		return domain.QueueEntry{}, oops.Wrapf(err, "parse product ID")
	}
	entry.ProductID = domain.ProductID(pid)
	return entry, nil
}

func (r *QueueRepository) Current(ctx context.Context, productID domain.ProductID, userID domain.ExternalUserID) (domain.QueueEntry, error) {
	var entry domain.QueueEntry
	var idStr string
	err := r.db.QueryRowContext(ctx, `
		SELECT ticket_id, product_id, external_user_id, status, joined_at, right_issued_at, completed_at, cancelled_at, expired_at
		FROM queue_entries
		WHERE product_id = $1 AND external_user_id = $2
		ORDER BY joined_at DESC
		LIMIT 1
	`, uuid.UUID(productID).String(), string(userID)).Scan(
		&entry.TicketID, &idStr, &entry.ExternalUserID, &entry.Status,
		&entry.JoinedAt, &entry.RightIssuedAt, &entry.CompletedAt, &entry.CancelledAt, &entry.ExpiredAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.QueueEntry{}, oops.Code("not_found").Wrapf(domain.ErrNotFound, "no queue entry for product %s user %s", uuid.UUID(productID), userID)
	}
	if err != nil {
		return domain.QueueEntry{}, oops.Wrapf(err, "get current entry")
	}
	pid, err := uuid.Parse(idStr)
	if err != nil {
		return domain.QueueEntry{}, oops.Wrapf(err, "parse product ID")
	}
	entry.ProductID = domain.ProductID(pid)
	return entry, nil
}

func (r *QueueRepository) Leave(ctx context.Context, productID domain.ProductID, userID domain.ExternalUserID) error {
	res, err := r.db.ExecContext(ctx, `
		UPDATE queue_entries
		SET status = 'cancelled', cancelled_at = NOW()
		WHERE product_id = $1 AND external_user_id = $2 AND status IN ('waiting', 'right_issued')
	`, uuid.UUID(productID).String(), string(userID))
	if err != nil {
		return oops.Wrapf(err, "update queue entry to cancelled")
	}
	rows, _ := res.RowsAffected()
	if rows == 0 {
		return oops.Code("not_found").Wrapf(domain.ErrNotFound, "no active queue entry to cancel")
	}
	return nil
}

func (r *QueueRepository) GetWaitingEntriesForProduct(ctx context.Context, productID domain.ProductID, limit int) ([]domain.QueueEntry, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT ticket_id, product_id, external_user_id, status, joined_at, right_issued_at, completed_at, cancelled_at, expired_at
		FROM queue_entries
		WHERE product_id = $1 AND status = 'waiting'
		ORDER BY ticket_id ASC
		LIMIT $2
	`, uuid.UUID(productID).String(), limit)
	if err != nil {
		return nil, oops.Wrapf(err, "query waiting entries")
	}
	defer rows.Close()
	var entries []domain.QueueEntry
	for rows.Next() {
		var e domain.QueueEntry
		var idStr string
		if err := rows.Scan(&e.TicketID, &idStr, &e.ExternalUserID, &e.Status,
			&e.JoinedAt, &e.RightIssuedAt, &e.CompletedAt, &e.CancelledAt, &e.ExpiredAt); err != nil {
			return nil, oops.Wrapf(err, "scan entry")
		}
		pid, err := uuid.Parse(idStr)
		if err != nil {
			return nil, oops.Wrapf(err, "parse product ID")
		}
		e.ProductID = domain.ProductID(pid)
		entries = append(entries, e)
	}
	if err = rows.Err(); err != nil {
		return nil, oops.Wrapf(err, "rows iteration")
	}
	return entries, nil
}
