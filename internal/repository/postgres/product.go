package postgres

import (
	"context"
	"database/sql"
	"errors"

	"github.com/Woolfer0097/GoodQueue/internal/pkg/domain"
	"github.com/google/uuid"
	"github.com/samber/oops"
)

type ProductRepository struct {
	db                   *sql.DB
	waitingBufferPercent int
}

func NewProductRepository(db *sql.DB, waitingBufferPercent ...int) *ProductRepository {
	percent := 100
	if len(waitingBufferPercent) > 0 {
		percent = waitingBufferPercent[0]
	}
	return &ProductRepository{db: db, waitingBufferPercent: percent}
}

func (r *ProductRepository) List(ctx context.Context) ([]domain.Product, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT p.id, p.title, p.description, p.image_url, p.category, p.price_cents,
		       p.queue_enabled, p.allocatable_stock,
		       p.reserved, p.next_queue_sequence,
		       COUNT(q.id) FILTER (WHERE q.state = 'waiting')
		FROM products p
		LEFT JOIN queue_attempts q ON q.product_id = p.id
		GROUP BY p.id
		ORDER BY p.id
	`)
	if err != nil {
		return nil, oops.Wrapf(err, "query products")
	}
	defer func() { _ = rows.Close() }()

	var products []domain.Product
	for rows.Next() {
		var p domain.Product
		var idStr string
		if err := rows.Scan(
			&idStr,
			&p.Title,
			&p.Description,
			&p.ImageURL,
			&p.Category,
			&p.PriceCents,
			&p.QueueEnabled,
			&p.AllocatableStock,
			&p.Reserved,
			&p.NextQueueSequence,
			&p.WaitingCount,
		); err != nil {
			return nil, oops.Wrapf(err, "scan product")
		}
		pid, err := uuid.Parse(idStr)
		if err != nil {
			return nil, oops.Wrapf(err, "parse product ID")
		}
		p.ID = domain.ProductID(pid)
		p.WaitingCapacity, err = domain.WaitingCapacity(p.AllocatableStock, r.waitingBufferPercent)
		if err != nil {
			return nil, oops.Wrapf(err, "calculate product waiting capacity")
		}
		products = append(products, p)
	}
	if err = rows.Err(); err != nil {
		return nil, oops.Wrapf(err, "rows iteration")
	}
	return products, nil
}

func (r *ProductRepository) Get(ctx context.Context, id domain.ProductID) (domain.Product, error) {
	var p domain.Product
	var idStr string
	err := r.db.QueryRowContext(ctx, `
		SELECT p.id, p.title, p.description, p.image_url, p.category, p.price_cents,
		       p.queue_enabled, p.allocatable_stock,
		       p.reserved, p.next_queue_sequence,
		       COUNT(q.id) FILTER (WHERE q.state = 'waiting')
		FROM products p
		LEFT JOIN queue_attempts q ON q.product_id = p.id
		WHERE p.id = $1
		GROUP BY p.id
	`, uuid.UUID(id).String()).Scan(
		&idStr,
		&p.Title,
		&p.Description,
		&p.ImageURL,
		&p.Category,
		&p.PriceCents,
		&p.QueueEnabled,
		&p.AllocatableStock,
		&p.Reserved,
		&p.NextQueueSequence,
		&p.WaitingCount,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.Product{}, oops.Code("not_found").Wrapf(domain.ErrNotFound, "product %s", uuid.UUID(id))
	}
	if err != nil {
		return domain.Product{}, oops.Wrapf(err, "get product %s", uuid.UUID(id))
	}
	pid, err := uuid.Parse(idStr)
	if err != nil {
		return domain.Product{}, oops.Wrapf(err, "parse product ID")
	}
	p.ID = domain.ProductID(pid)
	p.WaitingCapacity, err = domain.WaitingCapacity(p.AllocatableStock, r.waitingBufferPercent)
	if err != nil {
		return domain.Product{}, oops.Wrapf(err, "calculate product waiting capacity")
	}
	return p, nil
}
