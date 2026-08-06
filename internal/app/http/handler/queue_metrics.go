package handler

import (
	"context"
	"fmt"
	"net/http"
	"time"

	httpmiddleware "github.com/Woolfer0097/GoodQueue/internal/app/http/middleware"
	"github.com/Woolfer0097/GoodQueue/internal/pkg/domain"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

const (
	defaultMetricsWindow = 7 * 24 * time.Hour
	maxMetricsWindow     = 90 * 24 * time.Hour
)

type QueueMetricsService interface {
	Report(context.Context, time.Time, time.Time) (domain.QueueBufferReport, error)
}

type QueueMetricsHandler struct{ metrics QueueMetricsService }

type QueueMetricTotalsResponse struct {
	JoinedAttempts      int64   `json:"joined_attempts"`
	IssuedRights        int64   `json:"issued_rights"`
	ActiveRights        int64   `json:"active_rights"`
	ResolvedRights      int64   `json:"resolved_rights"`
	Purchases           int64   `json:"purchases"`
	InviteExpired       int64   `json:"invite_expired"`
	CheckoutExpired     int64   `json:"checkout_expired"`
	PaymentFailed       int64   `json:"payment_failed"`
	CancelledAfterRight int64   `json:"cancelled_after_right"`
	ConversionPercent   float64 `json:"resolved_right_conversion_percent"`
	AverageWaitSeconds  float64 `json:"average_queue_wait_seconds"`
}

type QueueProductMetricResponse struct {
	ProductID           string  `json:"product_id" format:"uuid"`
	ProductTitle        string  `json:"product_title"`
	JoinedAttempts      int64   `json:"joined_attempts"`
	IssuedRights        int64   `json:"issued_rights"`
	ActiveRights        int64   `json:"active_rights"`
	ResolvedRights      int64   `json:"resolved_rights"`
	Purchases           int64   `json:"purchases"`
	InviteExpired       int64   `json:"invite_expired"`
	CheckoutExpired     int64   `json:"checkout_expired"`
	PaymentFailed       int64   `json:"payment_failed"`
	CancelledAfterRight int64   `json:"cancelled_after_right"`
	ConversionPercent   float64 `json:"resolved_right_conversion_percent"`
	AverageWaitSeconds  float64 `json:"average_queue_wait_seconds"`
	P95WaitSeconds      float64 `json:"p95_queue_wait_seconds"`
}

type QueueMetricsResponse struct {
	WindowStart          time.Time                    `json:"window_start"`
	WindowEnd            time.Time                    `json:"window_end"`
	WaitingBufferPercent int                          `json:"waiting_buffer_percent"`
	Totals               QueueMetricTotalsResponse    `json:"totals"`
	Products             []QueueProductMetricResponse `json:"products"`
}

func NewQueueMetricsHandler(metrics QueueMetricsService) *QueueMetricsHandler {
	return &QueueMetricsHandler{metrics: metrics}
}

// Report godoc
//
//	@Summary		Queue-buffer experiment metrics
//	@Description	Reports durable reservation conversion and wait-time metrics. Active rights are excluded from conversion.
//	@Tags			internal
//	@Produce		json
//	@Param			window	query		string	false	"Cohort duration (Go duration, 1m..2160h)"	default(168h)
//	@Success		200		{object}	QueueMetricsResponse
//	@Failure		400,500	{object}	middleware.ErrorResponse
//	@Router			/internal/v1/queue-buffer-metrics [get]
func (handler *QueueMetricsHandler) Report(c *gin.Context) {
	window, err := parseMetricsWindow(c.Query("window"))
	if err != nil {
		_ = c.Error(err)
		return
	}
	end := time.Now().UTC()
	report, err := handler.metrics.Report(c.Request.Context(), end.Add(-window), end)
	if err != nil {
		_ = c.Error(err)
		return
	}
	c.JSON(http.StatusOK, queueMetricsResponse(report))
}

func parseMetricsWindow(raw string) (time.Duration, error) {
	if raw == "" {
		return defaultMetricsWindow, nil
	}
	window, err := time.ParseDuration(raw)
	if err != nil || window < time.Minute || window > maxMetricsWindow {
		return 0, fmt.Errorf("%w: metrics window must be between 1m and 2160h", domain.ErrInvalidInput)
	}
	return window, nil
}

func queueMetricsResponse(report domain.QueueBufferReport) QueueMetricsResponse {
	response := QueueMetricsResponse{
		WindowStart: report.WindowStart, WindowEnd: report.WindowEnd,
		WaitingBufferPercent: report.WaitingBufferPercent,
		Totals: QueueMetricTotalsResponse{
			JoinedAttempts: report.Totals.JoinedAttempts, IssuedRights: report.Totals.IssuedRights,
			ActiveRights: report.Totals.ActiveRights, ResolvedRights: report.Totals.ResolvedRights,
			Purchases: report.Totals.Purchases, InviteExpired: report.Totals.InviteExpired,
			CheckoutExpired: report.Totals.CheckoutExpired, PaymentFailed: report.Totals.PaymentFailed,
			CancelledAfterRight: report.Totals.CancelledAfterRight,
			ConversionPercent:   conversionPercent(report.Totals),
			AverageWaitSeconds:  report.Totals.AverageQueueWaitSeconds,
		},
		Products: make([]QueueProductMetricResponse, 0, len(report.Products)),
	}
	for _, metric := range report.Products {
		response.Products = append(response.Products, QueueProductMetricResponse{
			ProductID: uuid.UUID(metric.ProductID).String(), ProductTitle: metric.ProductTitle,
			JoinedAttempts: metric.JoinedAttempts, IssuedRights: metric.IssuedRights,
			ActiveRights: metric.ActiveRights, ResolvedRights: metric.ResolvedRights,
			Purchases: metric.Purchases, InviteExpired: metric.InviteExpired,
			CheckoutExpired: metric.CheckoutExpired, PaymentFailed: metric.PaymentFailed,
			CancelledAfterRight: metric.CancelledAfterRight,
			ConversionPercent:   conversionPercent(metric),
			AverageWaitSeconds:  metric.AverageQueueWaitSeconds, P95WaitSeconds: metric.P95QueueWaitSeconds,
		})
	}
	return response
}

func conversionPercent(metric domain.QueueBufferMetrics) float64 {
	if metric.ResolvedRights == 0 {
		return 0
	}
	return float64(metric.Purchases) * 100 / float64(metric.ResolvedRights)
}

var _ = httpmiddleware.ErrorResponse{}
