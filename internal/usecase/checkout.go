package usecase

import (
	"context"

	"github.com/Woolfer0097/GoodQueue/internal/pkg/domain"
	"github.com/Woolfer0097/GoodQueue/internal/pkg/repository"
)

type CheckoutUseCase struct{ attempts repository.QueueAttempt }

func NewCheckoutUseCase(attempts ...repository.QueueAttempt) *CheckoutUseCase {
	if len(attempts) == 0 {
		return &CheckoutUseCase{}
	}
	return &CheckoutUseCase{attempts: attempts[0]}
}

func (useCase *CheckoutUseCase) Start(
	ctx context.Context,
	attemptID domain.AttemptID,
	externalUserID domain.ExternalUserID,
) (domain.QueueAttempt, error) {
	if useCase.attempts == nil {
		return domain.QueueAttempt{}, domain.ErrNotImplemented
	}
	return useCase.attempts.StartCheckout(ctx, domain.StartCheckoutCommand{
		AttemptID: attemptID, ExternalUserID: externalUserID,
	})
}
