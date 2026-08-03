package postgres

import (
	"context"
	"database/sql"

	"github.com/Woolfer0097/GoodQueue/internal/pkg/domain"
	"github.com/google/uuid"
	"github.com/samber/oops"
)

type QueueRepository struct{ database *sql.DB }

func NewQueueRepository(database *sql.DB) *QueueRepository {
	return &QueueRepository{database: database}
}

func (repository *QueueRepository) Join(context.Context, domain.ProductID, domain.ExternalUserID, uuid.UUID) (domain.QueueEntry, error) {
	return domain.QueueEntry{}, oops.Code("not_implemented").Wrap(domain.ErrNotImplemented)
}

func (repository *QueueRepository) Current(context.Context, domain.ProductID, domain.ExternalUserID) (domain.QueueEntry, error) {
	return domain.QueueEntry{}, oops.Code("not_implemented").Wrap(domain.ErrNotImplemented)
}

func (repository *QueueRepository) Leave(context.Context, domain.ProductID, domain.ExternalUserID) error {
	return oops.Code("not_implemented").Wrap(domain.ErrNotImplemented)
}
