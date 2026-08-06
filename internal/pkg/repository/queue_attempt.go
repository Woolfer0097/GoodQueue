package repository

import (
	"context"

	"github.com/Woolfer0097/GoodQueue/internal/pkg/domain"
)

type QueueAttempt interface {
	Join(context.Context, domain.JoinQueueCommand) (domain.JoinQueueResult, error)
	StartCheckout(context.Context, domain.StartCheckoutCommand) (domain.QueueAttempt, error)
	Cancel(context.Context, domain.CancelQueueCommand) (domain.QueueAttempt, error)
	FindCurrent(context.Context, domain.ProductID, domain.ExternalUserID) (domain.CurrentQueueResult, error)
	AdjustStock(context.Context, domain.StockAdjustmentCommand) (domain.StockAdjustmentResult, error)
	ReconcileNextProduct(context.Context, int, []domain.ProductID) (domain.ReconciliationResult, error)
}
