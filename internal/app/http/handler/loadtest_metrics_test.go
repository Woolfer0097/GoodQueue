package handler

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Woolfer0097/GoodQueue/internal/app/http/middleware"
	"github.com/Woolfer0097/GoodQueue/internal/loadtest"
	"github.com/Woolfer0097/GoodQueue/internal/pkg/domain"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

type loadtestMetricsStub struct {
	stats         loadtest.RequestSuccessStats
	purchaseStats loadtest.PurchaseSuccessStats
	err           error
	window        time.Duration
}

func (stub *loadtestMetricsStub) PurchaseSuccessStats(_ context.Context, window time.Duration) (loadtest.PurchaseSuccessStats, error) {
	stub.window = window
	return stub.purchaseStats, stub.err
}

func (stub *loadtestMetricsStub) RequestSuccessStats(_ context.Context, window time.Duration) (loadtest.RequestSuccessStats, error) {
	stub.window = window
	return stub.stats, stub.err
}

func TestLoadtestMetricsHandlerReturnsPercentage(t *testing.T) {
	stub := &loadtestMetricsStub{stats: loadtest.RequestSuccessStats{Successful: 95, Total: 100}}
	response := performLoadtestMetricsRequest(stub, 30*time.Minute)
	if response.Code != http.StatusOK || response.Body.String() != `{"window":"30m0s","window_seconds":1800,"successful_requests":95,"total_requests":100,"success_percentage":95}` {
		t.Fatalf("unexpected response: %d %s", response.Code, response.Body.String())
	}
	if stub.window != 30*time.Minute {
		t.Fatalf("unexpected window: %s", stub.window)
	}
}

func TestLoadtestMetricsHandlerMapsPrometheusFailure(t *testing.T) {
	stub := &loadtestMetricsStub{err: errors.Join(domain.ErrMetricsUnavailable, errors.New("connection refused"))}
	response := performLoadtestMetricsRequest(stub, 30*time.Minute)
	if response.Code != http.StatusServiceUnavailable || response.Body.String() != `{"error":{"code":"metrics_unavailable","message":"loadtest metrics are unavailable"}}` {
		t.Fatalf("unexpected response: %d %s", response.Code, response.Body.String())
	}
}

func TestLoadtestMetricsHandlerReturnsPurchasePercentage(t *testing.T) {
	stub := &loadtestMetricsStub{purchaseStats: loadtest.PurchaseSuccessStats{
		Purchased: 6, Cancelled: 3, CheckoutExpired: 1,
	}}
	response := performLoadtestPurchaseMetricsRequest(stub, 30*time.Minute)
	if response.Code != http.StatusOK || response.Body.String() != `{"window":"30m0s","window_seconds":1800,"purchased":6,"cancelled":3,"checkout_expired":1,"total_checkout_outcomes":10,"purchase_percentage":60}` {
		t.Fatalf("unexpected response: %d %s", response.Code, response.Body.String())
	}
	if stub.window != 30*time.Minute {
		t.Fatalf("unexpected window: %s", stub.window)
	}
}

func TestLoadtestMetricsHandlerReturnsZeroPurchasePercentageWithoutOutcomes(t *testing.T) {
	response := performLoadtestPurchaseMetricsRequest(&loadtestMetricsStub{}, time.Minute)
	if response.Code != http.StatusOK || response.Body.String() != `{"window":"1m0s","window_seconds":60,"purchased":0,"cancelled":0,"checkout_expired":0,"total_checkout_outcomes":0,"purchase_percentage":0}` {
		t.Fatalf("unexpected response: %d %s", response.Code, response.Body.String())
	}
}

func performLoadtestMetricsRequest(reader LoadtestMetricsReader, window time.Duration) *httptest.ResponseRecorder {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(middleware.ErrorHandler(zap.NewNop()))
	handler := NewLoadtestMetricsHandler(reader, window)
	router.GET("/internal/v1/loadtest/request-success-rate", handler.RequestSuccessRate)
	request := httptest.NewRequest(http.MethodGet, "/internal/v1/loadtest/request-success-rate", nil)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	return response
}

func performLoadtestPurchaseMetricsRequest(reader LoadtestMetricsReader, window time.Duration) *httptest.ResponseRecorder {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(middleware.ErrorHandler(zap.NewNop()))
	handler := NewLoadtestMetricsHandler(reader, window)
	router.GET("/internal/v1/loadtest/purchase-success-rate", handler.PurchaseSuccessRate)
	request := httptest.NewRequest(http.MethodGet, "/internal/v1/loadtest/purchase-success-rate", nil)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	return response
}
