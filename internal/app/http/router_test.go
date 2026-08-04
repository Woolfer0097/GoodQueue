package http

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Woolfer0097/GoodQueue/internal/pkg/domain"
	"github.com/Woolfer0097/GoodQueue/internal/usecase"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

type stubProductRepository struct{}

func (stubProductRepository) List(context.Context) ([]domain.Product, error) {
	return nil, domain.ErrNotFound
}
func (stubProductRepository) Get(context.Context, domain.ProductID) (domain.Product, error) {
	return domain.Product{}, domain.ErrNotFound
}

type stubQueueRepository struct{}

func (stubQueueRepository) Join(context.Context, domain.ProductID, domain.ExternalUserID, uuid.UUID) (domain.QueueEntry, error) {
	return domain.QueueEntry{}, domain.ErrNotFound
}
func (stubQueueRepository) Current(context.Context, domain.ProductID, domain.ExternalUserID) (domain.QueueEntry, error) {
	return domain.QueueEntry{}, domain.ErrNotFound
}
func (stubQueueRepository) Leave(context.Context, domain.ProductID, domain.ExternalUserID) error {
	return domain.ErrNotFound
}
func (stubQueueRepository) GetWaitingEntriesForProduct(context.Context, domain.ProductID, int) ([]domain.QueueEntry, error) {
	return nil, domain.ErrNotFound
}
func (stubQueueRepository) GetByTicketID(context.Context, int64) (domain.QueueEntry, error) {
	return domain.QueueEntry{}, domain.ErrNotFound
}
func (stubQueueRepository) UpdateStatus(context.Context, int64, domain.QueueEntryStatus) error {
	return domain.ErrNotFound
}

type stubPurchaseRightRepository struct{}

func (stubPurchaseRightRepository) ActiveForUserAndProduct(context.Context, domain.ExternalUserID, domain.ProductID) (domain.PurchaseRight, error) {
	return domain.PurchaseRight{}, domain.ErrNotFound
}
func (stubPurchaseRightRepository) AcquireRight(context.Context, int64, domain.ProductID, int) (domain.PurchaseRight, error) {
	return domain.PurchaseRight{}, domain.ErrNotFound
}
func (stubPurchaseRightRepository) ReleaseRight(context.Context, int64, domain.QueueEntryStatus) error {
	return domain.ErrNotFound
}
func (stubPurchaseRightRepository) ListExpiredActiveRights(context.Context, time.Time) ([]domain.PurchaseRight, error) {
	return nil, domain.ErrNotFound
}

type countingPinger struct{ calls int }

func (pinger *countingPinger) PingContext(context.Context) error { pinger.calls++; return nil }

func testRouter(pinger *countingPinger) http.Handler {
	gin.SetMode(gin.TestMode)
	return NewRouter(Dependencies{
		Log:             zap.NewNop(),
		Database:        pinger,
		PingTimeout:     time.Second,
		ProductService:  usecase.NewProductUseCase(stubProductRepository{}),
		QueueService:    usecase.NewQueueUseCase(stubQueueRepository{}, stubProductRepository{}, stubPurchaseRightRepository{}),
		CheckoutService: usecase.NewCheckoutUseCase(stubPurchaseRightRepository{}),
	})
}

func TestBusinessRoutesMapRepositoryErrorsToNotFound(t *testing.T) {
	pinger := &countingPinger{}
	router := testRouter(pinger)
	routes := []struct{ method, path string }{
		{http.MethodGet, "/api/v1/products"},
		{http.MethodGet, "/api/v1/products/not-a-uuid"},
		{http.MethodPost, "/api/v1/products/not-a-uuid/queue-entries"},
		{http.MethodGet, "/api/v1/products/not-a-uuid/queue-entry"},
		{http.MethodDelete, "/api/v1/products/not-a-uuid/queue-entry"},
		{http.MethodPost, "/api/v1/products/not-a-uuid/checkout-authorizations"},
	}

	for _, route := range routes {
		t.Run(route.method+" "+route.path, func(t *testing.T) {
			request := httptest.NewRequest(route.method, route.path, nil)
			recorder := httptest.NewRecorder()
			router.ServeHTTP(recorder, request)
			if recorder.Code != http.StatusNotFound {
				t.Fatalf("expected 404, got %d: %s", recorder.Code, recorder.Body.String())
			}
			const expected = `{"error":{"code":"not_found","message":"resource not found"}}`
			if recorder.Body.String() != expected {
				t.Fatalf("unexpected response body: %q", recorder.Body.String())
			}
		})
	}
	if pinger.calls != 0 {
		t.Fatalf("business routes pinged database %d times", pinger.calls)
	}
}

func TestSwaggerUIAndContract(t *testing.T) {
	router := testRouter(&countingPinger{})
	redirectRecorder := httptest.NewRecorder()
	router.ServeHTTP(redirectRecorder, httptest.NewRequest(http.MethodGet, "/docs", nil))
	if redirectRecorder.Code != http.StatusMovedPermanently || redirectRecorder.Header().Get("Location") != "/docs/index.html" {
		t.Fatalf("docs redirect returned %d to %q", redirectRecorder.Code, redirectRecorder.Header().Get("Location"))
	}

	uiRecorder := httptest.NewRecorder()
	router.ServeHTTP(uiRecorder, httptest.NewRequest(http.MethodGet, "/docs/index.html", nil))
	if uiRecorder.Code != http.StatusOK {
		t.Fatalf("swagger UI returned %d: %s", uiRecorder.Code, uiRecorder.Body.String())
	}

	specRecorder := httptest.NewRecorder()
	router.ServeHTTP(specRecorder, httptest.NewRequest(http.MethodGet, "/docs/doc.json", nil))
	if specRecorder.Code != http.StatusOK {
		t.Fatalf("swagger spec returned %d: %s", specRecorder.Code, specRecorder.Body.String())
	}

	for _, oldPath := range []string{"/swagger", "/swagger/index.html", "/swagger/doc.json"} {
		recorder := httptest.NewRecorder()
		router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, oldPath, nil))
		if recorder.Code != http.StatusNotFound {
			t.Errorf("old Swagger path %s returned %d, want 404", oldPath, recorder.Code)
		}
	}

	var document struct {
		Swagger string `json:"swagger"`
		Paths   map[string]map[string]struct {
			Responses map[string]struct {
				Schema map[string]any `json:"schema"`
			} `json:"responses"`
		} `json:"paths"`
		Definitions map[string]struct {
			Required   []string `json:"required"`
			Properties map[string]struct {
				Format string `json:"format"`
			} `json:"properties"`
		} `json:"definitions"`
	}
	if err := json.Unmarshal(specRecorder.Body.Bytes(), &document); err != nil {
		t.Fatalf("decode swagger spec: %v", err)
	}
	if document.Swagger != "2.0" {
		t.Fatalf("expected Swagger 2.0, got %q", document.Swagger)
	}
	businessOperations := []struct{ path, method string }{
		{"/api/v1/products", "get"},
		{"/api/v1/products/{productID}", "get"},
		{"/api/v1/products/{productID}/queue-entries", "post"},
		{"/api/v1/products/{productID}/queue-entry", "get"},
		{"/api/v1/products/{productID}/queue-entry", "delete"},
		{"/api/v1/products/{productID}/checkout-authorizations", "post"},
	}
	for _, operation := range businessOperations {
		method, exists := document.Paths[operation.path][operation.method]
		if !exists {
			t.Errorf("missing Swagger operation %s %s", operation.method, operation.path)
			continue
		}
		response, exists := method.Responses["501"]
		if !exists || response.Schema["$ref"] != "#/definitions/middleware.ErrorResponse" {
			t.Errorf("missing standard 501 schema for %s %s: %v", operation.method, operation.path, response.Schema)
		}
	}
	for _, infrastructurePath := range []string{"/healthz", "/readyz"} {
		if _, exists := document.Paths[infrastructurePath]["get"]; !exists {
			t.Errorf("missing Swagger operation GET %s", infrastructurePath)
		}
	}

	requiredFields := map[string][]string{
		"handler.JoinQueueRequest":              {"idempotency_key"},
		"handler.ProductResponse":               {"id", "title", "description", "image_url", "queue_enabled", "allocatable_stock", "right_ttl_seconds"},
		"handler.QueueEntryResponse":            {"ticket_id", "product_id", "status", "joined_at"},
		"handler.CheckoutAuthorizationResponse": {"purchase_right_id", "queue_ticket_id", "status", "issued_at", "expires_at"},
		"handler.HealthResponse":                {"status"},
		"middleware.ErrorBody":                  {"code", "message"},
		"middleware.ErrorResponse":              {"error"},
	}
	for definitionName, expected := range requiredFields {
		definition, exists := document.Definitions[definitionName]
		if !exists {
			t.Errorf("missing Swagger definition %s", definitionName)
			continue
		}
		assertSameStrings(t, definitionName+" required fields", definition.Required, expected)
	}

	expectedFormats := map[string]map[string]string{
		"handler.JoinQueueRequest":              {"idempotency_key": "uuid"},
		"handler.ProductResponse":               {"id": "uuid", "image_url": "uri"},
		"handler.QueueEntryResponse":            {"product_id": "uuid", "joined_at": "date-time", "right_issued_at": "date-time", "completed_at": "date-time", "cancelled_at": "date-time", "expired_at": "date-time"},
		"handler.CheckoutAuthorizationResponse": {"purchase_right_id": "uuid", "issued_at": "date-time", "expires_at": "date-time"},
	}
	for definitionName, properties := range expectedFormats {
		for propertyName, expected := range properties {
			actual := document.Definitions[definitionName].Properties[propertyName].Format
			if actual != expected {
				t.Errorf("%s.%s format = %q, want %q", definitionName, propertyName, actual, expected)
			}
		}
	}
}

func assertSameStrings(t *testing.T, label string, actual, expected []string) {
	t.Helper()
	if len(actual) != len(expected) {
		t.Errorf("%s = %v, want %v", label, actual, expected)
		return
	}
	wanted := make(map[string]struct{}, len(expected))
	for _, value := range expected {
		wanted[value] = struct{}{}
	}
	for _, value := range actual {
		if _, exists := wanted[value]; !exists {
			t.Errorf("%s = %v, want %v", label, actual, expected)
			return
		}
	}
}
