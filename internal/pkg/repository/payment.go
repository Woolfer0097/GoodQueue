package repository

import (
	"context"

	"github.com/Woolfer0097/GoodQueue/internal/pkg/domain"
)

type PaymentInbox interface {
	ProcessPayment(context.Context, domain.PaymentCommand) (domain.PaymentResult, error)
	ClaimPayment(context.Context, domain.PaymentCommand) (domain.PaymentClaimResult, error)
	ProcessPaymentClaim(context.Context, domain.PaymentClaim) (domain.PaymentResult, error)
	RetryPaymentClaim(context.Context, domain.PaymentClaim, error) error
}
