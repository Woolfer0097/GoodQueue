package worker

import (
	"context"

	"github.com/Woolfer0097/GoodQueue/internal/pkg/domain"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

type LoggingPublisher struct {
	log *zap.Logger
}

func NewLoggingPublisher(log *zap.Logger) *LoggingPublisher {
	return &LoggingPublisher{log: log}
}

func (publisher *LoggingPublisher) Publish(ctx context.Context, claim domain.NotificationClaim) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	attemptID := ""
	if claim.AttemptID != nil {
		attemptID = uuid.UUID(*claim.AttemptID).String()
	}
	publisher.log.Info("demo notification published",
		zap.String("event_type", claim.EventType),
		zap.String("deduplication_key", claim.DeduplicationKey),
		zap.String("attempt_id", attemptID),
	)
	return nil
}
