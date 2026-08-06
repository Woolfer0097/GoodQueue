package repository

import (
	"context"
	"time"

	"github.com/Woolfer0097/GoodQueue/internal/pkg/domain"
)

type QueueMetrics interface {
	QueueBufferMetrics(context.Context, time.Time, time.Time) ([]domain.QueueBufferMetrics, error)
}
