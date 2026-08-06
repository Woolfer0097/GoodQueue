package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/Woolfer0097/GoodQueue/internal/pkg/domain"
	"github.com/google/uuid"
)

type QueueMetricsRepository struct{ db *sql.DB }

func NewQueueMetricsRepository(db *sql.DB) *QueueMetricsRepository {
	return &QueueMetricsRepository{db: db}
}

func (repository *QueueMetricsRepository) QueueBufferMetrics(
	ctx context.Context,
	start time.Time,
	end time.Time,
) ([]domain.QueueBufferMetrics, error) {
	rows, err := repository.db.QueryContext(ctx, `
		SELECT p.id, p.title,
		       COUNT(a.id) FILTER (WHERE a.created_at >= $1 AND a.created_at < $2),
		       COUNT(a.id) FILTER (WHERE a.invited_at >= $1 AND a.invited_at < $2),
		       COUNT(a.id) FILTER (WHERE a.invited_at >= $1 AND a.invited_at < $2
		           AND a.state IN ('invited', 'checkout')),
		       COUNT(a.id) FILTER (WHERE a.invited_at >= $1 AND a.invited_at < $2
		           AND a.state IN ('purchased', 'invite_expired', 'checkout_expired', 'payment_failed', 'cancelled')),
		       COUNT(a.id) FILTER (WHERE a.invited_at >= $1 AND a.invited_at < $2 AND a.state = 'purchased'),
		       COUNT(a.id) FILTER (WHERE a.invited_at >= $1 AND a.invited_at < $2 AND a.state = 'invite_expired'),
		       COUNT(a.id) FILTER (WHERE a.invited_at >= $1 AND a.invited_at < $2 AND a.state = 'checkout_expired'),
		       COUNT(a.id) FILTER (WHERE a.invited_at >= $1 AND a.invited_at < $2 AND a.state = 'payment_failed'),
		       COUNT(a.id) FILTER (WHERE a.invited_at >= $1 AND a.invited_at < $2 AND a.state = 'cancelled'),
		       COALESCE(AVG(EXTRACT(EPOCH FROM (a.invited_at - a.created_at)))
		           FILTER (WHERE a.invited_at >= $1 AND a.invited_at < $2), 0),
		       COALESCE(PERCENTILE_CONT(0.95) WITHIN GROUP
		           (ORDER BY EXTRACT(EPOCH FROM (a.invited_at - a.created_at)))
		           FILTER (WHERE a.invited_at >= $1 AND a.invited_at < $2), 0)
		FROM products p
		LEFT JOIN queue_attempts a ON a.product_id = p.id
		GROUP BY p.id, p.title
		ORDER BY p.id`, start, end)
	if err != nil {
		return nil, fmt.Errorf("query queue buffer metrics: %w", err)
	}
	defer func() { _ = rows.Close() }()

	metrics := make([]domain.QueueBufferMetrics, 0)
	for rows.Next() {
		var rawID string
		var metric domain.QueueBufferMetrics
		if err := rows.Scan(
			&rawID, &metric.ProductTitle, &metric.JoinedAttempts, &metric.IssuedRights,
			&metric.ActiveRights, &metric.ResolvedRights, &metric.Purchases, &metric.InviteExpired,
			&metric.CheckoutExpired, &metric.PaymentFailed, &metric.CancelledAfterRight,
			&metric.AverageQueueWaitSeconds, &metric.P95QueueWaitSeconds,
		); err != nil {
			return nil, fmt.Errorf("scan queue buffer metrics: %w", err)
		}
		parsedID, err := uuid.Parse(rawID)
		if err != nil {
			return nil, fmt.Errorf("parse metrics product ID: %w", err)
		}
		metric.ProductID = domain.ProductID(parsedID)
		metrics = append(metrics, metric)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate queue buffer metrics: %w", err)
	}
	return metrics, nil
}
