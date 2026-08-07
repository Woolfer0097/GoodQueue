package handler

import (
	"context"
	"math"
	"net/http"
	"time"

	httpmiddleware "github.com/Woolfer0097/GoodQueue/internal/app/http/middleware"
	"github.com/Woolfer0097/GoodQueue/internal/loadtest"
	"github.com/gin-gonic/gin"
)

type LoadtestMetricsReader interface {
	RequestSuccessStats(context.Context, time.Duration) (loadtest.RequestSuccessStats, error)
	PurchaseSuccessStats(context.Context, time.Duration) (loadtest.PurchaseSuccessStats, error)
}

var _ = httpmiddleware.ErrorResponse{}

type LoadtestMetricsHandler struct {
	metrics LoadtestMetricsReader
	window  time.Duration
}

type LoadtestRequestSuccessResponse struct {
	Window             string  `json:"window" binding:"required" example:"30m0s"`
	WindowSeconds      int64   `json:"window_seconds" binding:"required" example:"1800"`
	SuccessfulRequests float64 `json:"successful_requests" binding:"required" example:"950"`
	TotalRequests      float64 `json:"total_requests" binding:"required" example:"1000"`
	SuccessPercentage  float64 `json:"success_percentage" binding:"required" example:"95"`
}

type LoadtestPurchaseSuccessResponse struct {
	Window                string  `json:"window" binding:"required" example:"30m0s"`
	WindowSeconds         int64   `json:"window_seconds" binding:"required" example:"1800"`
	Purchased             float64 `json:"purchased" binding:"required" example:"60"`
	Cancelled             float64 `json:"cancelled" binding:"required" example:"25"`
	CheckoutExpired       float64 `json:"checkout_expired" binding:"required" example:"15"`
	TotalCheckoutOutcomes float64 `json:"total_checkout_outcomes" binding:"required" example:"100"`
	PurchasePercentage    float64 `json:"purchase_percentage" binding:"required" example:"60"`
}

func NewLoadtestMetricsHandler(metrics LoadtestMetricsReader, window time.Duration) *LoadtestMetricsHandler {
	return &LoadtestMetricsHandler{metrics: metrics, window: window}
}

// RequestSuccessRate godoc
//
//	@Summary		k6 request success percentage
//	@Description	Calculates the request-weighted percentage of k6 HTTP requests whose response was expected during the configured time window.
//	@Tags			loadtest
//	@Produce		json
//	@Success		200	{object}	LoadtestRequestSuccessResponse
//	@Failure		503	{object}	middleware.ErrorResponse
//	@Router			/internal/v1/loadtest/request-success-rate [get]
func (handler *LoadtestMetricsHandler) RequestSuccessRate(c *gin.Context) {
	stats, err := handler.metrics.RequestSuccessStats(c.Request.Context(), handler.window)
	if err != nil {
		_ = c.Error(err)
		return
	}
	percentage := 0.0
	if stats.Total > 0 {
		percentage = math.Round(stats.Successful/stats.Total*10000) / 100
	}
	c.JSON(http.StatusOK, LoadtestRequestSuccessResponse{
		Window:             handler.window.String(),
		WindowSeconds:      int64(handler.window.Seconds()),
		SuccessfulRequests: stats.Successful,
		TotalRequests:      stats.Total,
		SuccessPercentage:  percentage,
	})
}

// PurchaseSuccessRate godoc
//
//	@Summary		k6 purchase success percentage
//	@Description	Calculates purchased / (purchased + cancelled + checkout_expired) for k6 purchase outcomes during the configured time window.
//	@Tags			loadtest
//	@Produce		json
//	@Success		200	{object}	LoadtestPurchaseSuccessResponse
//	@Failure		503	{object}	middleware.ErrorResponse
//	@Router			/internal/v1/loadtest/purchase-success-rate [get]
func (handler *LoadtestMetricsHandler) PurchaseSuccessRate(c *gin.Context) {
	stats, err := handler.metrics.PurchaseSuccessStats(c.Request.Context(), handler.window)
	if err != nil {
		_ = c.Error(err)
		return
	}
	total := stats.Purchased + stats.Cancelled + stats.CheckoutExpired
	percentage := 0.0
	if total > 0 {
		percentage = math.Round(stats.Purchased/total*10000) / 100
	}
	c.JSON(http.StatusOK, LoadtestPurchaseSuccessResponse{
		Window:                handler.window.String(),
		WindowSeconds:         int64(handler.window.Seconds()),
		Purchased:             stats.Purchased,
		Cancelled:             stats.Cancelled,
		CheckoutExpired:       stats.CheckoutExpired,
		TotalCheckoutOutcomes: total,
		PurchasePercentage:    percentage,
	})
}
