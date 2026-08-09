package app

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Woolfer0097/GoodQueue/internal/app/config"
	"github.com/Woolfer0097/GoodQueue/internal/mockapi"
	"go.uber.org/zap"
)

func TestMockApplicationFrontendContract(t *testing.T) {
	application, err := New(mockConfig(), zap.NewNop())
	if err != nil {
		t.Fatalf("new mock application: %v", err)
	}
	handler := application.server.Handler

	assertStatus(t, handler, http.MethodGet, "/readyz", nil, http.StatusOK)
	products := assertStatus(t, handler, http.MethodGet, "/api/v1/products", nil, http.StatusOK)
	var productList []map[string]any
	if err := json.Unmarshal(products.Body.Bytes(), &productList); err != nil || len(productList) != 4 {
		t.Fatalf("products response: count=%d err=%v body=%s", len(productList), err, products.Body.String())
	}
	assertStatus(t, handler, http.MethodGet, "/api/v1/products/"+mockapi.ProductScarceID, nil, http.StatusOK)
	assertStatus(t, handler, http.MethodGet, "/api/v1/products/"+mockapi.ProductScarceID+"/alternatives", nil, http.StatusOK)
	assertStatus(t, handler, http.MethodGet, "/api/v1/demo/users", nil, http.StatusOK)

	waitingHeaders := map[string]string{"X-User-ID": mockapi.DemoUserTwoID}
	waiting := assertStatus(t, handler, http.MethodGet, "/api/v1/products/"+mockapi.ProductScarceID+"/queue-entry", waitingHeaders, http.StatusOK)
	var waitingBody map[string]any
	if err := json.Unmarshal(waiting.Body.Bytes(), &waitingBody); err != nil {
		t.Fatal(err)
	}
	if waitingBody["state"] != "waiting" || waitingBody["position"] != float64(1) || waitingBody["total_waiting"] != float64(2) ||
		waitingBody["next_action"] != "wait" || waitingBody["message_code"] != "queue_waiting" {
		t.Fatalf("waiting contract: %v", waitingBody)
	}
	statusScenarios := []struct {
		product, user, state, action, message string
	}{
		{mockapi.ProductScarceID, mockapi.DemoUserOneID, "checkout", "complete_payment", "checkout_started"},
		{mockapi.ProductPopularID, mockapi.DemoUserOneID, "invited", "start_checkout", "checkout_available"},
		{mockapi.ProductPopularID, mockapi.DemoUserTwoID, "purchased", "none", "purchased"},
		{mockapi.ProductPopularID, mockapi.DemoUserThreeID, "cancelled", "join_queue", "cancelled"},
		{mockapi.ProductPopularID, mockapi.DemoUserFourID, "invite_expired", "join_queue", "invitation_expired"},
		{mockapi.ProductPopularID, mockapi.DemoUserFiveID, "checkout_expired", "join_queue", "checkout_expired"},
	}
	for _, scenario := range statusScenarios {
		response := assertStatus(t, handler, http.MethodGet, "/api/v1/products/"+scenario.product+"/queue-entry", map[string]string{"X-User-ID": scenario.user}, http.StatusOK)
		var body map[string]any
		if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
			t.Fatal(err)
		}
		if body["state"] != scenario.state || body["next_action"] != scenario.action || body["message_code"] != scenario.message {
			t.Fatalf("scenario %s/%s: %v", scenario.product, scenario.user, body)
		}
	}

	newUser := "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"
	joinHeaders := map[string]string{"X-User-ID": newUser, "Idempotency-Key": "frontend-flow"}
	created := assertStatus(t, handler, http.MethodPost, "/api/v1/products/"+mockapi.ProductScarceID+"/queue-entries", joinHeaders, http.StatusCreated)
	var createdBody map[string]any
	if err := json.Unmarshal(created.Body.Bytes(), &createdBody); err != nil || createdBody["position"] != float64(3) {
		t.Fatalf("join contract: err=%v body=%v", err, createdBody)
	}
	assertStatus(t, handler, http.MethodPost, "/api/v1/products/"+mockapi.ProductScarceID+"/queue-entries", joinHeaders, http.StatusOK)
	assertStatus(t, handler, http.MethodDelete, "/api/v1/products/"+mockapi.ProductScarceID+"/queue-entry", map[string]string{"X-User-ID": newUser}, http.StatusNoContent)

	invitedHeaders := map[string]string{"X-User-ID": mockapi.DemoUserOneID}
	invited := assertStatus(t, handler, http.MethodGet, "/api/v1/products/"+mockapi.ProductPopularID+"/queue-entry", invitedHeaders, http.StatusOK)
	var invitedBody map[string]any
	if err := json.Unmarshal(invited.Body.Bytes(), &invitedBody); err != nil {
		t.Fatal(err)
	}
	if invitedBody["state"] != "invited" || invitedBody["expires_at"] == nil || invitedBody["next_action"] != "start_checkout" {
		t.Fatalf("invited contract: %v", invitedBody)
	}
	attemptID, ok := invitedBody["attempt_id"].(string)
	if !ok {
		t.Fatalf("missing attempt ID: %v", invitedBody)
	}
	assertStatus(t, handler, http.MethodPost, "/api/v1/queue-attempts/"+attemptID+"/checkout", invitedHeaders, http.StatusOK)
	demoPaymentHeaders := map[string]string{
		"X-User-ID": mockapi.DemoUserOneID, "Idempotency-Key": "mock-demo-payment",
	}
	demoPaymentPath := "/api/v1/products/" + mockapi.ProductPopularID + "/queue-attempts/" + attemptID + "/demo-payment"
	purchased := assertStatus(t, handler, http.MethodPost, demoPaymentPath, demoPaymentHeaders, http.StatusOK)
	var purchasedBody map[string]any
	if err := json.Unmarshal(purchased.Body.Bytes(), &purchasedBody); err != nil || purchasedBody["state"] != "purchased" {
		t.Fatalf("demo payment contract: err=%v body=%v", err, purchasedBody)
	}
	assertStatus(t, handler, http.MethodPost, demoPaymentPath, demoPaymentHeaders, http.StatusOK)

	assertStatus(t, handler, http.MethodPost, "/api/v1/products/"+mockapi.ProductSoldOutID+"/queue-entries", joinHeaders, http.StatusGone)
	assertStatus(t, handler, http.MethodPost, "/api/v1/products/"+mockapi.ProductDisabledID+"/queue-entries", joinHeaders, http.StatusConflict)
	assertStatus(t, handler, http.MethodPost, "/internal/v1/products/"+mockapi.ProductScarceID+"/stock-adjustments", nil, http.StatusNotFound)
}

func mockConfig() config.Config {
	return config.Config{
		Mode: config.ModeMock, HTTPAddress: ":0", HTTPReadHeaderTimeout: time.Second,
		ShutdownTimeout: time.Second, DatabasePingTimeout: time.Second,
		InvitationTTL: 10 * time.Minute, CheckoutTTL: 5 * time.Minute,
	}
}

func assertStatus(t *testing.T, handler http.Handler, method, path string, headers map[string]string, expected int) *httptest.ResponseRecorder {
	t.Helper()
	request := httptest.NewRequest(method, path, nil)
	for key, value := range headers {
		request.Header.Set(key, value)
	}
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	if recorder.Code != expected {
		t.Fatalf("%s %s: got %d, want %d: %s", method, path, recorder.Code, expected, recorder.Body.String())
	}
	return recorder
}
