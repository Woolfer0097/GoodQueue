package usecase

import (
	"context"

	"github.com/Woolfer0097/GoodQueue/internal/pkg/domain"
	"github.com/Woolfer0097/GoodQueue/internal/pkg/repository"
)

type QueueUseCase struct{ attempts repository.QueueAttempt }

func NewQueueUseCase(attempts ...repository.QueueAttempt) *QueueUseCase {
	if len(attempts) == 0 {
		return &QueueUseCase{}
	}
	return &QueueUseCase{attempts: attempts[0]}
}

func (useCase *QueueUseCase) Join(
	ctx context.Context,
	productID domain.ProductID,
	externalUserID domain.ExternalUserID,
	idempotencyKey domain.IdempotencyKey,
) (domain.JoinQueueResult, error) {
	if useCase.attempts == nil {
		return domain.JoinQueueResult{}, domain.ErrNotImplemented
	}
	return useCase.attempts.Join(ctx, domain.JoinQueueCommand{
		ProductID: productID, ExternalUserID: externalUserID, IdempotencyKey: idempotencyKey,
	})
}

func (useCase *QueueUseCase) Current(
	ctx context.Context,
	productID domain.ProductID,
	externalUserID domain.ExternalUserID,
) (domain.CurrentQueueResult, error) {
	if useCase.attempts == nil {
		return domain.CurrentQueueResult{}, domain.ErrNotImplemented
	}
	return useCase.attempts.FindCurrent(ctx, productID, externalUserID)
}

func (useCase *QueueUseCase) Active(
	ctx context.Context,
	externalUserID domain.ExternalUserID,
) ([]domain.CurrentQueueResult, error) {
	if useCase.attempts == nil {
		return nil, domain.ErrNotImplemented
	}
	return useCase.attempts.FindActiveByUser(ctx, externalUserID)
}

func (useCase *QueueUseCase) Leave(ctx context.Context, productID domain.ProductID, externalUserID domain.ExternalUserID) error {
	if useCase.attempts == nil {
		return domain.ErrNotImplemented
	}
	_, err := useCase.attempts.Cancel(ctx, domain.CancelQueueCommand{
		ProductID: productID, ExternalUserID: externalUserID,
	})
	return err
}
