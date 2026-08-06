package usecase

import (
	"context"
	"time"

	"github.com/Woolfer0097/GoodQueue/internal/pkg/domain"
	"github.com/Woolfer0097/GoodQueue/internal/pkg/repository"
)

type QueueMetricsUseCase struct {
	metrics              repository.QueueMetrics
	waitingBufferPercent int
}

func NewQueueMetricsUseCase(metrics repository.QueueMetrics, waitingBufferPercent int) *QueueMetricsUseCase {
	return &QueueMetricsUseCase{metrics: metrics, waitingBufferPercent: waitingBufferPercent}
}

func (useCase *QueueMetricsUseCase) Report(
	ctx context.Context,
	start time.Time,
	end time.Time,
) (domain.QueueBufferReport, error) {
	products, err := useCase.metrics.QueueBufferMetrics(ctx, start, end)
	if err != nil {
		return domain.QueueBufferReport{}, err
	}
	report := domain.QueueBufferReport{
		WindowStart: start, WindowEnd: end, WaitingBufferPercent: useCase.waitingBufferPercent,
		Products: products,
	}
	var weightedWait float64
	for _, product := range products {
		report.Totals.JoinedAttempts += product.JoinedAttempts
		report.Totals.IssuedRights += product.IssuedRights
		report.Totals.ActiveRights += product.ActiveRights
		report.Totals.ResolvedRights += product.ResolvedRights
		report.Totals.Purchases += product.Purchases
		report.Totals.InviteExpired += product.InviteExpired
		report.Totals.CheckoutExpired += product.CheckoutExpired
		report.Totals.PaymentFailed += product.PaymentFailed
		report.Totals.CancelledAfterRight += product.CancelledAfterRight
		weightedWait += product.AverageQueueWaitSeconds * float64(product.IssuedRights)
	}
	if report.Totals.IssuedRights > 0 {
		report.Totals.AverageQueueWaitSeconds = weightedWait / float64(report.Totals.IssuedRights)
	}
	return report, nil
}
