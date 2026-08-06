package repository

import (
	"context"
	"time"

	"github.com/Woolfer0097/GoodQueue/internal/pkg/domain"
)

type NotificationOutbox interface {
	ClaimNotification(context.Context, time.Duration) (*domain.NotificationClaim, error)
	ClassifyInvitation(context.Context, domain.NotificationClaim) (bool, error)
	MarkNotificationSent(context.Context, domain.NotificationClaim) error
	MarkNotificationObsolete(context.Context, domain.NotificationClaim) error
	RetryNotification(context.Context, domain.NotificationClaim, error, time.Duration, time.Duration) (time.Duration, error)
}
