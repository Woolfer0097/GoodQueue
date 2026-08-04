package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/Woolfer0097/GoodQueue/internal/pkg/domain"
	"github.com/google/uuid"
	"github.com/samber/oops"
)

const getProductByIDQuery = `SELECT
    id,
    title,
    description,
    image_url,
    queue_enabled,
    allocatable_stock,
    right_ttl_seconds,
    created_at,
    updated_at,
    price_kopecks
FROM public.products
WHERE id = $1;`

type ProductRepository struct{ database *sql.DB }

func NewProductRepository(database *sql.DB) *ProductRepository {
	return &ProductRepository{database: database}
}

func (repository *ProductRepository) List(context.Context) ([]domain.Product, error) {
	return nil, oops.Code("not_implemented").Wrap(domain.ErrNotImplemented)
}

func (repository *ProductRepository) GetByID(ctx context.Context, productID domain.ProductID) (*domain.Product, error) {
	var product domain.Product
	var id uuid.UUID

	err := repository.database.QueryRowContext(ctx, getProductByIDQuery, uuid.UUID(productID)).Scan(
		&id,
		&product.Title,
		&product.Description,
		&product.ImageURL,
		&product.QueueEnabled,
		&product.AllocatableStock,
		&product.RightTTLSeconds,
		&product.CreatedAt,
		&product.UpdatedAt,
		&product.PriceKopecks,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, domain.ErrProductNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get product by ID: %w", err)
	}

	product.ID = domain.ProductID(id)
	return &product, nil
}
