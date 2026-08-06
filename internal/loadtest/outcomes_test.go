package loadtest

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestReadK6OutcomeEvents(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "k6-events.log")
	contents := "unrelated log line\nGOODQUEUE_OUTCOME " +
		`{"run_id":"run-1","external_user_id":"user-1","product_id":"product-1",` +
		`"http_status":410,"actual_outcome":"sold_out","final_state":"sold_out"}` + "\n"
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}
	events, err := readK6OutcomeEvents(path, "run-1")
	if err != nil {
		t.Fatal(err)
	}
	event, exists := events["user-1/product-1"]
	if !exists || event.ActualOutcome != "sold_out" || event.HTTPStatus == nil || *event.HTTPStatus != 410 {
		t.Fatalf("unexpected parsed event: %+v", event)
	}
}

func TestEvaluatePurchaseOutcomesClassifiesAndValidates(t *testing.T) {
	t.Parallel()
	now := time.Now().UTC()
	started := now.Add(-time.Minute)
	statusOK := int16(200)
	data := Data{
		Products: []Product{{ID: "product-1", InitialStock: 3}},
		Users: []User{
			{ID: "user-1", Assignments: []Assignment{{ProductID: "product-1", PlannedOutcome: "purchase", PaymentEventID: "event-1"}}},
			{ID: "user-2", Assignments: []Assignment{{ProductID: "product-1", PlannedOutcome: "cancel"}}},
			{ID: "user-3", Assignments: []Assignment{{ProductID: "product-1", PlannedOutcome: "ttl"}}},
			{ID: "user-4", Assignments: []Assignment{{ProductID: "product-1", PlannedOutcome: "purchase"}}},
		},
	}
	attempts := []attemptSnapshot{
		{ID: "attempt-1", ProductID: "product-1", ExternalUserID: "user-1", State: "purchased", CheckoutStartedAt: &started},
		{ID: "attempt-2", ProductID: "product-1", ExternalUserID: "user-2", State: "cancelled", CheckoutStartedAt: &started},
		{ID: "attempt-3", ProductID: "product-1", ExternalUserID: "user-3", State: "checkout_expired", CheckoutStartedAt: &started},
	}
	payments := []paymentSnapshot{{
		ID: "inbox-1", Provider: "goodqueue-loadtest", EventID: "event-1", AttemptID: "attempt-1",
		Outcome: "succeeded", Status: "completed", ResponseHTTPStatus: &statusOK,
	}}
	evaluation := evaluatePurchaseOutcomes(
		data,
		[]productSnapshot{{ID: "product-1", AllocatableStock: 2, Reserved: 0}},
		attempts,
		payments,
		map[string]k6OutcomeEvent{
			"user-1/product-1": {AttemptID: "attempt-1", Operation: "POST /internal/v1/payment-events", HTTPStatus: &statusOK, ActualOutcome: "purchased", FinalState: "purchased"},
			"user-2/product-1": {AttemptID: "attempt-2", Operation: "DELETE /api/v1/products/{productID}/queue-entry", ActualOutcome: "cancelled", FinalState: "cancelled"},
			"user-3/product-1": {AttemptID: "attempt-3", Operation: "wait_checkout_ttl", ActualOutcome: "checkout_expired", FinalState: "checkout_expired"},
			"user-4/product-1": {Operation: "POST /api/v1/products/{productID}/queue-entries", ActualOutcome: "queue_rejected", FinalState: "queue_full"},
		},
	)
	if evaluation.Counts.Purchased != 1 || evaluation.Counts.Cancelled != 1 ||
		evaluation.Counts.CheckoutExpired != 1 || evaluation.Counts.QueueRejected != 1 ||
		evaluation.Counts.PaymentAccepted != 1 || evaluation.Counts.Unresolved != 0 {
		t.Fatalf("unexpected outcome counts: %+v", evaluation.Counts)
	}
	for _, check := range evaluation.Checks {
		if !check.Passed {
			t.Fatalf("check %s failed: %v", check.Name, check.Violations)
		}
	}
	if len(evaluation.Logs) != 4 {
		t.Fatalf("request logs=%d, want 4", len(evaluation.Logs))
	}
}

func TestEvaluatePurchaseOutcomesRejectsWrongTerminalState(t *testing.T) {
	t.Parallel()
	started := time.Now().UTC().Add(-time.Minute)
	evaluation := evaluatePurchaseOutcomes(
		Data{
			Products: []Product{{ID: "product-1", InitialStock: 1}},
			Users:    []User{{ID: "user-1", Assignments: []Assignment{{ProductID: "product-1", PlannedOutcome: "ttl"}}}},
		},
		[]productSnapshot{{ID: "product-1", AllocatableStock: 1}},
		[]attemptSnapshot{{ID: "attempt-1", ProductID: "product-1", ExternalUserID: "user-1", State: "cancelled", CheckoutStartedAt: &started}},
		nil,
		map[string]k6OutcomeEvent{"user-1/product-1": {AttemptID: "attempt-1", ActualOutcome: "cancelled", FinalState: "cancelled"}},
	)
	if evaluation.Checks[1].Passed {
		t.Fatal("planned outcome mismatch was accepted")
	}
}
