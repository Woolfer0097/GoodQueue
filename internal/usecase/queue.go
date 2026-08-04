package usecase

import (
	"context"
	"errors"

	"github.com/Woolfer0097/GoodQueue/internal/pkg/domain"
	"github.com/Woolfer0097/GoodQueue/internal/pkg/repository"
	"github.com/google/uuid"
)

type QueueUseCase struct {
	queue          repository.Queue
	products       repository.Product
	purchaseRights repository.PurchaseRight
}

func NewQueueUseCase(queue repository.Queue, products repository.Product, purchaseRights repository.PurchaseRight) *QueueUseCase {
	return &QueueUseCase{queue: queue, products: products, purchaseRights: purchaseRights}
}

func (useCase *QueueUseCase) Join(ctx context.Context, productID domain.ProductID, userID domain.ExternalUserID, idempotencyKey uuid.UUID) (domain.QueueEntry, error) {
	if _, err := useCase.products.Get(ctx, productID); err != nil {
		return domain.QueueEntry{}, err
	}

	entry, err := useCase.queue.Join(ctx, productID, userID, idempotencyKey)
	if err != nil {
		return domain.QueueEntry{}, err
	}

	if _, err := useCase.TryGrantNext(ctx, productID); err != nil {
		return domain.QueueEntry{}, err
	}

	return useCase.queue.GetByTicketID(ctx, entry.TicketID)
}

func (useCase *QueueUseCase) Current(ctx context.Context, productID domain.ProductID, userID domain.ExternalUserID) (domain.QueueEntry, error) {
	return useCase.queue.Current(ctx, productID, userID)
}

func (useCase *QueueUseCase) Leave(ctx context.Context, productID domain.ProductID, userID domain.ExternalUserID) error {
	return useCase.queue.Leave(ctx, productID, userID)
}

func (useCase *QueueUseCase) TryGrantNext(ctx context.Context, productID domain.ProductID) (bool, error) {
	waiting, err := useCase.queue.GetWaitingEntriesForProduct(ctx, productID, 1)
	if err != nil {
		return false, err
	}
	if len(waiting) == 0 {
		return false, nil
	}

	product, err := useCase.products.Get(ctx, productID)
	if err != nil {
		return false, err
	}

	_, err = useCase.purchaseRights.AcquireRight(ctx, waiting[0].TicketID, productID, product.RightTTLSeconds)
	if errors.Is(err, domain.ErrOutOfStock) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}
