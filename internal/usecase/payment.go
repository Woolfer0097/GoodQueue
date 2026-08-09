package usecase

import (
	"context"
	"crypto/sha256"
	"encoding/hex"

	"github.com/Woolfer0097/GoodQueue/internal/pkg/domain"
	"github.com/google/uuid"
)

const demoPaymentProvider = "goodqueue-demo"

type paymentProcessor interface {
	ProcessPayment(context.Context, domain.PaymentCommand) (domain.PaymentResult, error)
}

type currentAttemptFinder interface {
	FindCurrent(context.Context, domain.ProductID, domain.ExternalUserID) (domain.CurrentQueueResult, error)
}

type PaymentUseCase struct {
	inbox    paymentProcessor
	attempts currentAttemptFinder
}

func NewPaymentUseCase(inbox paymentProcessor, attempts ...currentAttemptFinder) *PaymentUseCase {
	useCase := &PaymentUseCase{inbox: inbox}
	if len(attempts) > 0 {
		useCase.attempts = attempts[0]
	}
	return useCase
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

func (useCase *PaymentUseCase) CompleteDemo(
	ctx context.Context,
	productID domain.ProductID,
	attemptID domain.AttemptID,
	externalUserID domain.ExternalUserID,
	idempotencyKey domain.IdempotencyKey,
) (domain.DemoPaymentResult, error) {
	if useCase.inbox == nil || useCase.attempts == nil {
		return domain.DemoPaymentResult{}, domain.ErrNotImplemented
	}

	current, err := useCase.attempts.FindCurrent(ctx, productID, externalUserID)
	if err != nil {
		return domain.DemoPaymentResult{}, err
	}
	if current.Attempt.ID != attemptID {
		return domain.DemoPaymentResult{}, domain.ErrAttemptNotFound
	}
	if current.Attempt.State == domain.QueueAttemptPurchased {
		return domain.DemoPaymentResult{Attempt: current.Attempt}, nil
	}
	if current.Attempt.State != domain.QueueAttemptCheckout {
		return domain.DemoPaymentResult{}, demoPaymentStateError(current.Attempt.State)
	}

	token := demoPaymentToken(attemptID, externalUserID, idempotencyKey)
	payment, err := useCase.Process(
		ctx,
		demoPaymentProvider,
		"event-"+token,
		uuid.UUID(attemptID).String(),
		string(domain.PaymentSucceeded),
		"payment-"+token,
	)
	if err != nil {
		return domain.DemoPaymentResult{}, err
	}
	if payment.HTTPStatus >= 400 {
		switch payment.Code {
		case "attempt_not_found":
			return domain.DemoPaymentResult{}, domain.ErrAttemptNotFound
		case "event_conflict":
			return domain.DemoPaymentResult{}, domain.ErrConflict
		default:
			return domain.DemoPaymentResult{}, domain.ErrInvalidTransition
		}
	}

	latest, err := useCase.attempts.FindCurrent(ctx, productID, externalUserID)
	if err != nil {
		return domain.DemoPaymentResult{}, err
	}
	if latest.Attempt.ID != attemptID {
		return domain.DemoPaymentResult{}, domain.ErrAttemptNotFound
	}
	if latest.Attempt.State == domain.QueueAttemptPurchased {
		return domain.DemoPaymentResult{Attempt: latest.Attempt}, nil
	}
	if latest.Attempt.State == domain.QueueAttemptCheckout && payment.HTTPStatus == 202 {
		return domain.DemoPaymentResult{Attempt: latest.Attempt, Processing: true}, nil
	}
	return domain.DemoPaymentResult{}, demoPaymentStateError(latest.Attempt.State)
}

func demoPaymentToken(
	attemptID domain.AttemptID,
	externalUserID domain.ExternalUserID,
	idempotencyKey domain.IdempotencyKey,
) string {
	payload := uuid.UUID(attemptID).String() + "\n" + string(externalUserID) + "\n" + string(idempotencyKey)
	digest := sha256.Sum256([]byte(payload))
	return hex.EncodeToString(digest[:])
}

func demoPaymentStateError(state domain.QueueAttemptState) error {
	switch state {
	case domain.QueueAttemptInviteExpired, domain.QueueAttemptCheckoutExpired, domain.QueueAttemptSoldOut:
		return domain.ErrAttemptGone
	default:
		return domain.ErrInvalidTransition
	}
}
