package usecase

import (
	"context"

	"github.com/Woolfer0097/GoodQueue/internal/pkg/domain"
	"github.com/Woolfer0097/GoodQueue/internal/pkg/repository"
	"github.com/google/uuid"
	"github.com/samber/oops"
)

type QueueUseCase struct{ queue repository.Queue }

func NewQueueUseCase(queue repository.Queue) *QueueUseCase { return &QueueUseCase{queue: queue} }

func (useCase *QueueUseCase) Join(context.Context, domain.ProductID, domain.ExternalUserID, uuid.UUID) (domain.QueueEntry, error) {
	return domain.QueueEntry{}, oops.Code("not_implemented").Wrap(domain.ErrNotImplemented)
}

func (useCase *QueueUseCase) Current(context.Context, domain.ProductID, domain.ExternalUserID) (domain.QueueEntry, error) {
	return domain.QueueEntry{}, oops.Code("not_implemented").Wrap(domain.ErrNotImplemented)
}

func (useCase *QueueUseCase) Leave(context.Context, domain.ProductID, domain.ExternalUserID) error {
	return oops.Code("not_implemented").Wrap(domain.ErrNotImplemented)
}
