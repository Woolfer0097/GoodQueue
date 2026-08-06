package usecase

import (
	"context"

	"github.com/Woolfer0097/GoodQueue/internal/pkg/domain"
	"github.com/Woolfer0097/GoodQueue/internal/pkg/repository"
)

type PaymentUseCase struct{ inbox repository.PaymentInbox }

func NewPaymentUseCase(inbox repository.PaymentInbox) *PaymentUseCase {
	return &PaymentUseCase{inbox: inbox}
}

func (useCase *PaymentUseCase) Process(
	ctx context.Context,
	provider, eventID, attemptID, outcome, paymentReference string,
) (domain.PaymentResult, error) {
	command, err := domain.ParsePaymentCommand(provider, eventID, attemptID, outcome, paymentReference)
	if err != nil {
		return domain.PaymentResult{}, err
	}
	return useCase.inbox.ProcessPayment(ctx, command)
}
