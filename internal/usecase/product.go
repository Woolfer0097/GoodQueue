package usecase

import (
	"context"

	"github.com/Woolfer0097/GoodQueue/internal/pkg/domain"
	"github.com/Woolfer0097/GoodQueue/internal/pkg/repository"
	"github.com/samber/oops"
)

type ProductUseCase struct{ products repository.Product }

func NewProductUseCase(products repository.Product) *ProductUseCase {
	return &ProductUseCase{products: products}
}

func (useCase *ProductUseCase) List(context.Context) ([]domain.Product, error) {
	return nil, oops.Code("not_implemented").Wrap(domain.ErrNotImplemented)
}

func (useCase *ProductUseCase) Get(context.Context, domain.ProductID) (domain.Product, error) {
	return domain.Product{}, oops.Code("not_implemented").Wrap(domain.ErrNotImplemented)
}
