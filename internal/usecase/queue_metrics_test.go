package usecase

import (
	"context"
	"testing"
	"time"

	"github.com/Woolfer0097/GoodQueue/internal/pkg/domain"
)

type queueMetricsRepositoryStub struct{ metrics []domain.QueueBufferMetrics }

func (stub queueMetricsRepositoryStub) QueueBufferMetrics(
	context.Context, time.Time, time.Time,
) ([]domain.QueueBufferMetrics, error) {
	return stub.metrics, nil
}

func TestQueueMetricsReportAggregatesProducts(t *testing.T) {
	useCase := NewQueueMetricsUseCase(queueMetricsRepositoryStub{metrics: []domain.QueueBufferMetrics{
		{IssuedRights: 2, ResolvedRights: 1, Purchases: 1, AverageQueueWaitSeconds: 10},
		{IssuedRights: 1, ActiveRights: 1, JoinedAttempts: 3, AverageQueueWaitSeconds: 40},
	}}, 150)

	report, err := useCase.Report(context.Background(), time.Now().Add(-time.Hour), time.Now())
	if err != nil {
		t.Fatalf("report: %v", err)
	}
	if report.WaitingBufferPercent != 150 || report.Totals.IssuedRights != 3 || report.Totals.Purchases != 1 {
		t.Fatalf("unexpected totals: %+v", report)
	}
	if report.Totals.AverageQueueWaitSeconds != 20 {
		t.Fatalf("weighted average wait: got %f, want 20", report.Totals.AverageQueueWaitSeconds)
	}
}
