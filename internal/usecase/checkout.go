package usecase

import (
	"context"

	"github.com/Woolfer0097/GoodQueue/internal/pkg/domain"
	"github.com/Woolfer0097/GoodQueue/internal/pkg/repository"
	"github.com/samber/oops"
)

type CheckoutUseCase struct{ purchaseRights repository.PurchaseRight }

func NewCheckoutUseCase(purchaseRights repository.PurchaseRight) *CheckoutUseCase {
	return &CheckoutUseCase{purchaseRights: purchaseRights}
}

func (useCase *CheckoutUseCase) Authorize(context.Context, domain.ProductID, domain.ExternalUserID) (domain.PurchaseRight, error) {
	return domain.PurchaseRight{}, oops.Code("not_implemented").Wrap(domain.ErrNotImplemented)
}
