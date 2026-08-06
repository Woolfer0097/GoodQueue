package usecase

import (
	"context"
	"errors"
	"fmt"

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

func (useCase *ProductUseCase) GetByID(ctx context.Context, productID domain.ProductID) (*domain.Product, error) {
	product, err := useCase.products.GetByID(ctx, productID)
	if errors.Is(err, domain.ErrProductNotFound) {
		return nil, domain.ErrProductNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get product: %w", err)
	}
	return product, nil
}
