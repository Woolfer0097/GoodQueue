package postgres

import (
	"context"
	"database/sql"

	"github.com/Woolfer0097/GoodQueue/internal/pkg/domain"
	"github.com/samber/oops"
)

type PurchaseRightRepository struct{ database *sql.DB }

func NewPurchaseRightRepository(database *sql.DB) *PurchaseRightRepository {
	return &PurchaseRightRepository{database: database}
}

func (repository *PurchaseRightRepository) ActiveForUserAndProduct(context.Context, domain.ExternalUserID, domain.ProductID) (domain.PurchaseRight, error) {
	return domain.PurchaseRight{}, oops.Code("not_implemented").Wrap(domain.ErrNotImplemented)
}
