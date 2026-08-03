package postgres

import (
	"context"
	"database/sql"

	"github.com/Woolfer0097/GoodQueue/internal/pkg/domain"
	"github.com/samber/oops"
)

type ProductRepository struct{ database *sql.DB }

func NewProductRepository(database *sql.DB) *ProductRepository {
	return &ProductRepository{database: database}
}

func (repository *ProductRepository) List(context.Context) ([]domain.Product, error) {
	return nil, oops.Code("not_implemented").Wrap(domain.ErrNotImplemented)
}

func (repository *ProductRepository) Get(context.Context, domain.ProductID) (domain.Product, error) {
	return domain.Product{}, oops.Code("not_implemented").Wrap(domain.ErrNotImplemented)
}
