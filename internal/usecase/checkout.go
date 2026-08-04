package usecase

import (
	"context"
	"time"

	"github.com/Woolfer0097/GoodQueue/internal/pkg/domain"
	"github.com/Woolfer0097/GoodQueue/internal/pkg/repository"
	"github.com/samber/oops"
)

type CheckoutUseCase struct{ purchaseRights repository.PurchaseRight }

func NewCheckoutUseCase(purchaseRights repository.PurchaseRight) *CheckoutUseCase {
	return &CheckoutUseCase{purchaseRights: purchaseRights}
}

func (useCase *CheckoutUseCase) Authorize(ctx context.Context, productID domain.ProductID, userID domain.ExternalUserID) (domain.PurchaseRight, error) {
	right, err := useCase.purchaseRights.ActiveForUserAndProduct(ctx, userID, productID)
	if err != nil {
		return domain.PurchaseRight{}, err
	}

	if !right.ExpiresAt.After(time.Now()) {
		return domain.PurchaseRight{}, oops.Code("grant_expired").Wrap(domain.ErrGrantExpired)
	}

	if err := useCase.purchaseRights.ReleaseRight(ctx, right.QueueTicketID, domain.QueueEntryCompleted); err != nil {
		return domain.PurchaseRight{}, err
	}

	consumedAt := time.Now()
	right.Status = domain.PurchaseRightConsumed
	right.ConsumedAt = &consumedAt
	return right, nil
}
