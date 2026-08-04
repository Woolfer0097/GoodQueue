package repository

import (
	"context"

	"github.com/Woolfer0097/GoodQueue/internal/pkg/domain"
)

type Product interface {
	List(context.Context) ([]domain.Product, error)
	GetByID(context.Context, domain.ProductID) (*domain.Product, error)
}
