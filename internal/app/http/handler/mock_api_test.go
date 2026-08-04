package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Woolfer0097/GoodQueue/internal/app/http/middleware"
	"github.com/Woolfer0097/GoodQueue/internal/mockapi"
	"github.com/Woolfer0097/GoodQueue/internal/mockdata"
	"github.com/Woolfer0097/GoodQueue/internal/pkg/domain"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

type panicProductGetter struct{}

func (panicProductGetter) GetByID(context.Context, domain.ProductID) (*domain.Product, error) {
	panic("real product getter called by mock list")
}

func mockAPITestRouter(queueStatus string) http.Handler {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(middleware.RequestIDMiddleware(), middleware.ErrorHandler(zap.NewNop()))
	productHandler := NewProductHandler(mockapi.NewProductService(panicProductGetter{}))
	queueHandler := NewQueueHandler(mockapi.NewQueueService(queueStatus))
	checkoutHandler := NewCheckoutHandler(mockapi.NewCheckoutService())
	router.GET("/api/v1/products", productHandler.List)
	router.POST("/api/v1/products/:productID/queue-entries", queueHandler.Join)
	router.GET("/api/v1/products/:productID/queue-entry", queueHandler.Current)
	router.DELETE("/api/v1/products/:productID/queue-entry", queueHandler.Leave)
	router.POST("/api/v1/products/:productID/checkout-authorizations", checkoutHandler.Authorize)
	return router
}

func TestMockProductListContract(t *testing.T) {
	recorder := httptest.NewRecorder()
	mockAPITestRouter(mockdata.QueueStatusWaiting).ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/v1/products", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	var products []ProductResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &products); err != nil {
		t.Fatalf("decode products: %v", err)
	}
	if len(products) != 3 {
		t.Fatalf("product count = %d, want 3", len(products))
	}
	if products[0].ID != testProductID || products[0].Price != 1999900 || products[0].Available != 1 || !products[0].QueueEnabled || products[0].RightTTLSeconds != 120 {
		t.Fatalf("unexpected first product: %+v", products[0])
	}
	if products[2].ID != "33333333-3333-4333-8333-333333333333" || products[2].QueueEnabled {
		t.Fatalf("unexpected final product: %+v", products[2])
	}
}

func TestMockQueueJoinContract(t *testing.T) {
	response := performMockQueueRequest(t, mockdata.QueueStatusWaiting, http.MethodPost, "/api/v1/products/"+testProductID+"/queue-entries", true, "user-1", http.StatusCreated)
	assertQueueResponse(t, response, "waiting", intPointer(3), intPointer(7), nil)
}

func TestMockQueueCurrentStatuses(t *testing.T) {
	tests := []struct {
		status       string
		expected     string
		position     *int
		totalWaiting *int
		expiresAt    *string
	}{
		{status: "waiting", expected: "waiting", position: intPointer(3), totalWaiting: intPointer(7)},
		{status: "granted", expected: "granted", expiresAt: stringPointer("2026-08-04T12:00:00Z")},
		{status: "purchased", expected: "purchased"},
		{status: "cancelled", expected: "cancelled"},
		{status: "expired", expected: "expired", expiresAt: stringPointer("2026-08-04T12:00:00Z")},
	}
	for _, test := range tests {
		t.Run(test.status, func(t *testing.T) {
			response := performMockQueueRequest(t, test.status, http.MethodGet, "/api/v1/products/"+testProductID+"/queue-entry", true, "user-1", http.StatusOK)
			assertQueueResponse(t, response, test.expected, test.position, test.totalWaiting, test.expiresAt)
		})
	}
}

func TestMockQueueLeaveContract(t *testing.T) {
	response := performMockQueueRequest(t, mockdata.QueueStatusWaiting, http.MethodDelete, "/api/v1/products/"+testProductID+"/queue-entry", true, "user-1", http.StatusOK)
	assertQueueResponse(t, response, "cancelled", nil, nil, nil)
}

func TestMockCheckoutContract(t *testing.T) {
	request := httptest.NewRequest(http.MethodPost, "/api/v1/products/"+testProductID+"/checkout-authorizations", strings.NewReader("{}"))
	request.Header.Set("X-User-ID", "user-1")
	recorder := httptest.NewRecorder()
	mockAPITestRouter(mockdata.QueueStatusWaiting).ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	var response CheckoutAuthorizationResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode checkout response: %v", err)
	}
	if !response.Authorized || response.AuthorizationID != "41cd68a0-5e63-4d6e-a610-b5d3281a4fea" || response.EntryID != 42 || response.ProductID != testProductID || response.Status != "purchased" || response.AuthorizedAt != "2026-08-04T10:16:20Z" {
		t.Fatalf("unexpected checkout response: %+v", response)
	}
}

func TestMockAPIValidationErrors(t *testing.T) {
	tests := []struct {
		name          string
		path          string
		headerPresent bool
		userID        string
		status        int
		code          string
	}{
		{name: "invalid product ID", path: "/api/v1/products/not-a-uuid/queue-entry", headerPresent: true, userID: "user-1", status: http.StatusBadRequest, code: "INVALID_PRODUCT_ID"},
		{name: "missing user ID", path: "/api/v1/products/" + testProductID + "/queue-entry", status: http.StatusUnauthorized, code: "UNAUTHORIZED"},
		{name: "empty user ID", path: "/api/v1/products/" + testProductID + "/queue-entry", headerPresent: true, status: http.StatusUnauthorized, code: "INVALID_USER_ID"},
		{name: "blank user ID", path: "/api/v1/products/" + testProductID + "/queue-entry", headerPresent: true, userID: "   ", status: http.StatusUnauthorized, code: "INVALID_USER_ID"},
		{name: "long user ID", path: "/api/v1/products/" + testProductID + "/queue-entry", headerPresent: true, userID: strings.Repeat("x", 256), status: http.StatusUnauthorized, code: "INVALID_USER_ID"},
		{name: "unknown product", path: "/api/v1/products/99999999-9999-4999-8999-999999999999/queue-entry", headerPresent: true, userID: "user-1", status: http.StatusNotFound, code: "PRODUCT_NOT_FOUND"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodGet, test.path, nil)
			request.Header.Set("X-Request-ID", "validation-request")
			if test.headerPresent {
				request.Header.Set("X-User-ID", test.userID)
			}
			recorder := httptest.NewRecorder()
			mockAPITestRouter(mockdata.QueueStatusWaiting).ServeHTTP(recorder, request)
			if recorder.Code != test.status {
				t.Fatalf("status = %d, want %d; body = %s", recorder.Code, test.status, recorder.Body.String())
			}
			var response ErrorResponse
			if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
				t.Fatalf("decode error: %v", err)
			}
			if response.Code != test.code || response.RequestID != "validation-request" {
				t.Fatalf("unexpected error response: %+v", response)
			}
		})
	}
}

func performMockQueueRequest(t *testing.T, queueStatus, method, path string, headerPresent bool, userID string, expectedStatus int) QueueEntryResponse {
	t.Helper()
	request := httptest.NewRequest(method, path, strings.NewReader("{}"))
	if headerPresent {
		request.Header.Set("X-User-ID", userID)
	}
	recorder := httptest.NewRecorder()
	mockAPITestRouter(queueStatus).ServeHTTP(recorder, request)
	if recorder.Code != expectedStatus {
		t.Fatalf("status = %d, want %d; body = %s", recorder.Code, expectedStatus, recorder.Body.String())
	}
	var raw map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &raw); err != nil {
		t.Fatalf("decode raw queue response: %v", err)
	}
	for _, field := range []string{"position", "total_waiting", "expires_at"} {
		if _, exists := raw[field]; !exists {
			t.Fatalf("field %s is absent from %s", field, recorder.Body.String())
		}
	}
	var response QueueEntryResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode queue response: %v", err)
	}
	return response
}

func assertQueueResponse(t *testing.T, response QueueEntryResponse, status string, position, totalWaiting *int, expiresAt *string) {
	t.Helper()
	if response.EntryID != 42 || response.ProductID != testProductID || response.Status != status || !equalIntPointers(response.Position, position) || !equalIntPointers(response.TotalWaiting, totalWaiting) || !equalStringPointers(response.ExpiresAt, expiresAt) {
		t.Fatalf("unexpected queue response: %+v", response)
	}
}

func intPointer(value int) *int { return &value }

func stringPointer(value string) *string { return &value }

func equalIntPointers(left, right *int) bool {
	return left == nil && right == nil || left != nil && right != nil && *left == *right
}

func equalStringPointers(left, right *string) bool {
	return left == nil && right == nil || left != nil && right != nil && *left == *right
}
