package repository

import (
	"context"

	"github.com/Woolfer0097/GoodQueue/internal/pkg/domain"
)

type PurchaseRight interface {
	ActiveForUserAndProduct(context.Context, domain.ExternalUserID, domain.ProductID) (domain.PurchaseRight, error)
}
