package repository

import (
	"context"

	"github.com/Woolfer0097/GoodQueue/internal/pkg/domain"
	"github.com/google/uuid"
)

type Queue interface {
	Join(context.Context, domain.ProductID, domain.ExternalUserID, uuid.UUID) (domain.QueueEntry, error)
	Current(context.Context, domain.ProductID, domain.ExternalUserID) (domain.QueueEntry, error)
	Leave(context.Context, domain.ProductID, domain.ExternalUserID) error
	GetWaitingEntriesForProduct(ctx context.Context, productID domain.ProductID, limit int) ([]domain.QueueEntry, error)
}
