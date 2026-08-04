package usecase

import (
	"context"

	"github.com/Woolfer0097/GoodQueue/internal/pkg/domain"
	"github.com/Woolfer0097/GoodQueue/internal/pkg/repository"
)

type ProductUseCase struct{ products repository.Product }

func NewProductUseCase(products repository.Product) *ProductUseCase {
	return &ProductUseCase{products: products}
}

func (useCase *ProductUseCase) List(ctx context.Context) ([]domain.Product, error) {
	return useCase.products.List(ctx)
}

func (useCase *ProductUseCase) Get(ctx context.Context, id domain.ProductID) (domain.Product, error) {
	return useCase.products.Get(ctx, id)
}
