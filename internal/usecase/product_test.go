package usecase

import (
	"context"
	"errors"
	"testing"

	"github.com/Woolfer0097/GoodQueue/internal/pkg/domain"
)

type productRepositoryStub struct {
	getByID func(context.Context, domain.ProductID) (*domain.Product, error)
}

func (stub productRepositoryStub) List(context.Context) ([]domain.Product, error) {
	return nil, domain.ErrNotImplemented
}

func (stub productRepositoryStub) GetByID(ctx context.Context, productID domain.ProductID) (*domain.Product, error) {
	return stub.getByID(ctx, productID)
}

func TestProductUseCaseGetByIDReturnsRepositoryProduct(t *testing.T) {
	expected := &domain.Product{Title: "product"}
	useCase := NewProductUseCase(productRepositoryStub{getByID: func(context.Context, domain.ProductID) (*domain.Product, error) {
		return expected, nil
	}})

	actual, err := useCase.GetByID(context.Background(), domain.ProductID{})
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if actual != expected {
		t.Fatalf("product = %p, want %p", actual, expected)
	}
}

func TestProductUseCaseGetByIDPreservesNotFound(t *testing.T) {
	useCase := NewProductUseCase(productRepositoryStub{getByID: func(context.Context, domain.ProductID) (*domain.Product, error) {
		return nil, domain.ErrProductNotFound
	}})

	_, err := useCase.GetByID(context.Background(), domain.ProductID{})
	if !errors.Is(err, domain.ErrProductNotFound) {
		t.Fatalf("error = %v, want ErrProductNotFound", err)
	}
}

func TestProductUseCaseGetByIDWrapsUnknownError(t *testing.T) {
	databaseErr := errors.New("database failed")
	useCase := NewProductUseCase(productRepositoryStub{getByID: func(context.Context, domain.ProductID) (*domain.Product, error) {
		return nil, databaseErr
	}})

	_, err := useCase.GetByID(context.Background(), domain.ProductID{})
	if !errors.Is(err, databaseErr) {
		t.Fatalf("error = %v, want wrapped repository error", err)
	}
}
