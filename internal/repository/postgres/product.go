package postgres

import (
	"context"
	"database/sql"
	"errors"

	"github.com/Woolfer0097/GoodQueue/internal/pkg/domain"
	"github.com/google/uuid"
	"github.com/samber/oops"
)

type ProductRepository struct{ db *sql.DB }

func NewProductRepository(db *sql.DB) *ProductRepository {
	return &ProductRepository{db: db}
}

func (r *ProductRepository) List(ctx context.Context) ([]domain.Product, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, title, description, image_url, queue_enabled, allocatable_stock, right_ttl_seconds
		FROM products
	`)
	if err != nil {
		return nil, oops.Wrapf(err, "query products")
	}
	defer func() { _ = rows.Close() }()

	var products []domain.Product
	for rows.Next() {
		var p domain.Product
		var idStr string
		if err := rows.Scan(&idStr, &p.Title, &p.Description, &p.ImageURL, &p.QueueEnabled, &p.AllocatableStock, &p.RightTTLSeconds); err != nil {
			return nil, oops.Wrapf(err, "scan product")
		}
		pid, err := uuid.Parse(idStr)
		if err != nil {
			return nil, oops.Wrapf(err, "parse product ID")
		}
		p.ID = domain.ProductID(pid)
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
		SELECT id, title, description, image_url, queue_enabled, allocatable_stock, right_ttl_seconds
		FROM products WHERE id = $1
	`, uuid.UUID(id).String()).Scan(&idStr, &p.Title, &p.Description, &p.ImageURL, &p.QueueEnabled, &p.AllocatableStock, &p.RightTTLSeconds)
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
	return p, nil
}
