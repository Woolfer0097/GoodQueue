package http

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Woolfer0097/GoodQueue/internal/pkg/domain"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

const (
	testProductID = "10000000-0000-4000-8000-000000000001"
	testAttemptID = "20000000-0000-4000-8000-000000000001"
	testUserID    = "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"
)

type countingPinger struct{ calls int }

func (pinger *countingPinger) PingContext(context.Context) error { pinger.calls++; return nil }

type apiStub struct {
	joinCreated          bool
	joinCalls            int
	joinErr              error
	joinTotalWaiting     int64
	currentAttempt       *domain.QueueAttempt
	currentPositionAhead int64
	currentTotalWaiting  int64
	paymentCalls         int
}

func (stub *apiStub) List(context.Context) ([]domain.Product, error) { return nil, nil }
func (stub *apiStub) Alternatives(context.Context, domain.ProductID) ([]domain.ProductRecommendation, error) {
	return nil, nil
}
func (stub *apiStub) Get(context.Context, domain.ProductID) (domain.Product, error) {
	return domain.Product{}, domain.ErrNotFound
}
func (stub *apiStub) Join(
	context.Context, domain.ProductID, domain.ExternalUserID, domain.IdempotencyKey,
) (domain.JoinQueueResult, error) {
	stub.joinCalls++
	if stub.joinErr != nil {
		return domain.JoinQueueResult{}, stub.joinErr
	}
	return domain.JoinQueueResult{
		Attempt:       testAttempt(domain.QueueAttemptWaiting),
		PositionAhead: 3,
		TotalWaiting:  stub.joinTotalWaiting,
		Created:       stub.joinCreated,
	}, nil
}
func (stub *apiStub) Current(context.Context, domain.ProductID, domain.ExternalUserID) (domain.CurrentQueueResult, error) {
	attempt := testAttempt(domain.QueueAttemptWaiting)
	if stub.currentAttempt != nil {
		attempt = *stub.currentAttempt
	}
	return domain.CurrentQueueResult{
		Attempt:       attempt,
		PositionAhead: stub.currentPositionAhead,
		TotalWaiting:  stub.currentTotalWaiting,
	}, nil
}
func (stub *apiStub) Leave(context.Context, domain.ProductID, domain.ExternalUserID) error {
	return nil
}
func (stub *apiStub) Start(context.Context, domain.AttemptID, domain.ExternalUserID) (domain.QueueAttempt, error) {
	attempt := testAttempt(domain.QueueAttemptCheckout)
	deadline := time.Now().UTC().Add(time.Minute)
	attempt.CheckoutDeadline = &deadline
	return attempt, nil
}
func (stub *apiStub) Adjust(context.Context, domain.StockAdjustmentCommand) (domain.StockAdjustmentResult, error) {
	return domain.StockAdjustmentResult{HTTPStatus: 200, ResponseBody: []byte(`{"stock_before":1,"stock_after":2}`)}, nil
}
func (stub *apiStub) Process(context.Context, string, string, string, string, string) (domain.PaymentResult, error) {
	stub.paymentCalls++
	return domain.PaymentResult{HTTPStatus: 202, ResponseBody: []byte(`{"code":"processing"}`)}, nil
}
func (stub *apiStub) ListDemo(context.Context) ([]domain.DemoUser, error) { return nil, nil }

type demoStub struct{ api *apiStub }

func (stub demoStub) List(ctx context.Context) ([]domain.DemoUser, error) {
	return stub.api.ListDemo(ctx)
}

func newTestRouter(stub *apiStub, unsafe bool) http.Handler {
	gin.SetMode(gin.TestMode)
	return NewRouter(Dependencies{
		Log: zap.NewNop(), Database: &countingPinger{}, PingTimeout: time.Second,
		ProductService: stub, QueueService: stub, CheckoutService: stub, DemoUserService: demoStub{stub},
		StockService: stub, PaymentService: stub, UnsafePaymentCallback: unsafe,
	})
}

func TestProductListReturnsEmptyArray(t *testing.T) {
	recorder := performRequest(newTestRouter(&apiStub{}, false), http.MethodGet, "/api/v1/products", "", nil)
	if recorder.Code != http.StatusOK || recorder.Body.String() != "[]" {
		t.Fatalf("unexpected response: %d %s", recorder.Code, recorder.Body.String())
	}
}

func TestProductAlternativesReturnsEmptyArray(t *testing.T) {
	recorder := performRequest(
		newTestRouter(&apiStub{}, false),
		http.MethodGet,
		"/api/v1/products/"+testProductID+"/alternatives",
		"",
		nil,
	)
	if recorder.Code != http.StatusOK || recorder.Body.String() != "[]" {
		t.Fatalf("unexpected alternatives response: %d %s", recorder.Code, recorder.Body.String())
	}
}

func TestJoinRequiresCanonicalIdentityAndHeader(t *testing.T) {
	router := newTestRouter(&apiStub{joinCreated: true}, false)
	path := "/api/v1/products/" + testProductID + "/queue-entries"
	for _, identity := range []string{"", " ", strings.ToUpper(testUserID), "not-a-uuid"} {
		recorder := performRequest(router, http.MethodPost, path, "", map[string]string{"X-User-ID": identity, "Idempotency-Key": "join-1"})
		if recorder.Code != http.StatusUnauthorized {
			t.Fatalf("identity %q returned %d: %s", identity, recorder.Code, recorder.Body.String())
		}
	}
	recorder := performRequest(router, http.MethodPost, path, "ignored body", map[string]string{"X-User-ID": testUserID})
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("missing idempotency key returned %d", recorder.Code)
	}
}

func TestJoinCreatedAndReplayStatusesAndMapping(t *testing.T) {
	stub := &apiStub{joinCreated: true, joinTotalWaiting: 4}
	router := newTestRouter(stub, false)
	path := "/api/v1/products/" + testProductID + "/queue-entries"
	headers := map[string]string{"X-User-ID": testUserID, "Idempotency-Key": "join-1"}
	created := performRequest(router, http.MethodPost, path, "this body is deliberately ignored", headers)
	if created.Code != http.StatusCreated {
		t.Fatalf("created join returned %d: %s", created.Code, created.Body.String())
	}
	var response map[string]any
	if err := json.Unmarshal(created.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if response["position_ahead"] != float64(3) || response["position"] != float64(4) ||
		response["total_waiting"] != float64(4) || response["next_action"] != "wait" || response["message_code"] != "queue_waiting" {
		t.Fatalf("unexpected queue mapping: %v", response)
	}
	stub.joinCreated = false
	replay := performRequest(router, http.MethodPost, path, "", headers)
	if replay.Code != http.StatusOK {
		t.Fatalf("replay returned %d", replay.Code)
	}
}

func TestJoinQueueDisabledUsesStableConflictResponse(t *testing.T) {
	stub := &apiStub{joinErr: domain.ErrQueueDisabled}
	recorder := performRequest(
		newTestRouter(stub, false),
		http.MethodPost,
		"/api/v1/products/"+testProductID+"/queue-entries",
		"",
		map[string]string{"X-User-ID": testUserID, "Idempotency-Key": "disabled-join"},
	)
	if recorder.Code != http.StatusConflict || recorder.Body.String() != `{"error":{"code":"queue_disabled","message":"queue is disabled"}}` {
		t.Fatalf("queue-disabled response changed: %d %s", recorder.Code, recorder.Body.String())
	}
	if stub.joinCalls != 1 {
		t.Fatalf("queue-disabled join calls: got %d, want 1", stub.joinCalls)
	}
}

func TestCurrentQueueEntryIncludesFrontendPollingFields(t *testing.T) {
	router := newTestRouter(&apiStub{currentPositionAhead: 2, currentTotalWaiting: 5}, false)
	recorder := performRequest(
		router,
		http.MethodGet,
		"/api/v1/products/"+testProductID+"/queue-entry",
		"",
		map[string]string{"X-User-ID": testUserID},
	)
	if recorder.Code != http.StatusOK {
		t.Fatalf("current queue entry returned %d: %s", recorder.Code, recorder.Body.String())
	}
	var response map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if response["position"] != float64(3) || response["total_waiting"] != float64(5) {
		t.Fatalf("unexpected polling fields: %v", response)
	}
	if _, exists := response["expires_at"]; exists {
		t.Fatalf("waiting response must not include expires_at: %v", response)
	}
}

func TestCurrentQueueEntryUsesStateDeadlineAsExpiresAt(t *testing.T) {
	deadline := time.Date(2026, time.August, 6, 12, 0, 0, 0, time.UTC)
	attempt := testAttempt(domain.QueueAttemptInvited)
	attempt.InvitationDeadline = &deadline
	router := newTestRouter(&apiStub{currentAttempt: &attempt, currentTotalWaiting: 4}, false)
	recorder := performRequest(
		router,
		http.MethodGet,
		"/api/v1/products/"+testProductID+"/queue-entry",
		"",
		map[string]string{"X-User-ID": testUserID},
	)
	if recorder.Code != http.StatusOK {
		t.Fatalf("current invited entry returned %d: %s", recorder.Code, recorder.Body.String())
	}
	var response map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if response["expires_at"] != deadline.Format(time.RFC3339) || response["total_waiting"] != float64(4) {
		t.Fatalf("unexpected invited polling fields: %v", response)
	}
	if _, exists := response["position"]; exists {
		t.Fatalf("invited response must not include position: %v", response)
	}
}

func TestCancelAndCheckoutRoutes(t *testing.T) {
	router := newTestRouter(&apiStub{}, false)
	headers := map[string]string{"X-User-ID": testUserID}
	cancel := performRequest(router, http.MethodDelete, "/api/v1/products/"+testProductID+"/queue-entry", "", headers)
	if cancel.Code != http.StatusNoContent || cancel.Body.Len() != 0 {
		t.Fatalf("cancel returned %d %q", cancel.Code, cancel.Body.String())
	}
	checkout := performRequest(router, http.MethodPost, "/api/v1/queue-attempts/"+testAttemptID+"/checkout", "", headers)
	if checkout.Code != http.StatusOK || !strings.Contains(checkout.Body.String(), `"deadline_at"`) {
		t.Fatalf("checkout returned %d %s", checkout.Code, checkout.Body.String())
	}
	old := performRequest(router, http.MethodPost, "/api/v1/products/"+testProductID+"/checkout-authorizations", "", headers)
	if old.Code != http.StatusNotFound {
		t.Fatalf("old checkout route returned %d", old.Code)
	}
}

func TestStrictInternalBodiesAndExactBytes(t *testing.T) {
	router := newTestRouter(&apiStub{}, true)
	stockPath := "/internal/v1/products/" + testProductID + "/stock-adjustments"
	stockHeaders := map[string]string{"Idempotency-Key": "adjust-1", "Content-Type": "application/json"}
	bad := performRequest(router, http.MethodPost, stockPath, `{"delta":1,"reason":"restock","external_reference":"r1","extra":true}`, stockHeaders)
	if bad.Code != http.StatusBadRequest {
		t.Fatalf("unknown stock field returned %d", bad.Code)
	}
	stock := performRequest(router, http.MethodPost, stockPath, `{"delta":1,"reason":"restock","external_reference":"r1"}`, stockHeaders)
	if stock.Code != 200 || stock.Body.String() != `{"stock_before":1,"stock_after":2}` {
		t.Fatalf("stock bytes changed: %d %q", stock.Code, stock.Body.String())
	}
	payment := performRequest(router, http.MethodPost, "/internal/v1/payment-events",
		`{"provider":"demo","event_id":"e1","attempt_id":"`+testAttemptID+`","outcome":"failed","payment_reference":""}`,
		map[string]string{"Content-Type": "application/json"})
	if payment.Code != 202 || payment.Body.String() != `{"code":"processing"}` {
		t.Fatalf("payment bytes changed: %d %q", payment.Code, payment.Body.String())
	}
}

func TestUnsafePaymentRouteDisabled(t *testing.T) {
	stub := &apiStub{}
	recorder := performRequest(newTestRouter(stub, false), http.MethodPost, "/internal/v1/payment-events", `{}`, nil)
	if recorder.Code != http.StatusNotFound {
		t.Fatalf("disabled callback returned %d", recorder.Code)
	}
	if stub.paymentCalls != 0 {
		t.Fatalf("disabled callback mutated payment service %d times", stub.paymentCalls)
	}
}

func TestSwaggerContract(t *testing.T) {
	router := newTestRouter(&apiStub{}, false)
	spec := performRequest(router, http.MethodGet, "/docs/doc.json", "", nil)
	if spec.Code != http.StatusOK {
		t.Fatalf("swagger returned %d", spec.Code)
	}
	body := spec.Body.String()
	for _, path := range []string{
		"/api/v1/products", "/api/v1/products/{productID}", "/api/v1/products/{productID}/queue-entries",
		"/api/v1/products/{productID}/queue-entry", "/api/v1/queue-attempts/{attemptID}/checkout",
		"/api/v1/demo/users", "/internal/v1/products/{productID}/stock-adjustments", "/internal/v1/payment-events",
	} {
		if !strings.Contains(body, `"`+path+`"`) {
			t.Errorf("missing Swagger path %s", path)
		}
	}
	if strings.Contains(body, "checkout-authorizations") || strings.Contains(body, `"501"`) || strings.Contains(body, "not implemented") {
		t.Fatal("Swagger contains removed skeleton contract")
	}
	if !strings.Contains(body, "queue_disabled") {
		t.Fatal("Swagger does not document the stable queue_disabled response")
	}
}

func performRequest(router http.Handler, method, path, body string, headers map[string]string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(method, path, strings.NewReader(body))
	for key, value := range headers {
		request.Header.Set(key, value)
	}
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	return recorder
}

func testAttempt(state domain.QueueAttemptState) domain.QueueAttempt {
	now := time.Now().UTC()
	return domain.QueueAttempt{
		ID: domain.AttemptID(uuid.MustParse(testAttemptID)), ProductID: domain.ProductID(uuid.MustParse(testProductID)),
		QueueSequence: 7, ExternalUserID: testUserID, IdempotencyKey: "join-1", State: state,
		CreatedAt: now, UpdatedAt: now,
	}
}
