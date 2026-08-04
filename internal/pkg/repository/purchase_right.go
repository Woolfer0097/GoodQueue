package repository

import (
	"context"

	"github.com/Woolfer0097/GoodQueue/internal/pkg/domain"
)

type PurchaseRight interface {
	ActiveForUserAndProduct(context.Context, domain.ExternalUserID, domain.ProductID) (domain.PurchaseRight, error)
	AcquireRight(ctx context.Context, queueTicketID int64, productID domain.ProductID, ttlSeconds int) (domain.PurchaseRight, error)
}
