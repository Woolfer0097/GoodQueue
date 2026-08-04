package repository

import (
	"context"
	"time"

	"github.com/Woolfer0097/GoodQueue/internal/pkg/domain"
)

type PurchaseRight interface {
	ActiveForUserAndProduct(context.Context, domain.ExternalUserID, domain.ProductID) (domain.PurchaseRight, error)
	AcquireRight(ctx context.Context, queueTicketID int64, productID domain.ProductID, ttlSeconds int) (domain.PurchaseRight, error)
	ReleaseRight(ctx context.Context, queueTicketID int64, finalStatus domain.QueueEntryStatus) error
	ListExpiredActiveRights(ctx context.Context, before time.Time) ([]domain.PurchaseRight, error)
}
