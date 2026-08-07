package loadtest

import (
	"testing"
	"time"
)

func TestEvaluateDetectsInvariantViolations(t *testing.T) {
	t.Parallel()
	now := time.Now().UTC()
	data := Data{
		RunID:    "verify",
		Users:    []User{{ID: "user-1", Assignments: []Assignment{{ProductID: "product-1", IdempotencyKey: "same"}}}},
		Products: []Product{{ID: "product-1"}},
	}
	users := map[string]struct{}{"user-1": {}}
	products := []productSnapshot{{ID: "product-1", AllocatableStock: 1, Reserved: 2, NextQueueSequence: 3}}
	attempts := []attemptSnapshot{
		{ID: "a1", ProductID: "product-1", QueueSequence: 1, ExternalUserID: "user-1", IdempotencyKey: "same", State: "waiting", CreatedAt: now, UpdatedAt: now},
		{ID: "a2", ProductID: "product-1", QueueSequence: 2, ExternalUserID: "user-1", IdempotencyKey: "same", State: "waiting", CreatedAt: now.Add(-time.Second), UpdatedAt: now},
	}
	result := Evaluate("verify", data, users, products, attempts)
	if result.Passed {
		t.Fatal("Evaluate() passed invalid snapshots")
	}
	wantedFailures := map[string]bool{
		"inventory_bounds": false, "one_active_attempt_per_user_product": false,
		"fifo_sequence_order": false, "attempts_within_unique_user_product_links": false,
		"idempotent_join_single_attempt": false,
	}
	for _, check := range result.Checks {
		if _, wanted := wantedFailures[check.Name]; wanted {
			wantedFailures[check.Name] = !check.Passed
		}
	}
	for name, failed := range wantedFailures {
		if !failed {
			t.Errorf("check %s did not fail", name)
		}
	}
}

func TestEvaluateAcceptsValidFirstVersionState(t *testing.T) {
	t.Parallel()
	now := time.Now().UTC()
	data := Data{
		RunID:    "valid",
		Users:    []User{{ID: "user-1", Assignments: []Assignment{{ProductID: "product-1", IdempotencyKey: "key"}}}},
		Products: []Product{{ID: "product-1"}},
	}
	result := Evaluate(
		"valid", data, map[string]struct{}{"user-1": {}},
		[]productSnapshot{{ID: "product-1", AllocatableStock: 2, Reserved: 1, NextQueueSequence: 2}},
		[]attemptSnapshot{{
			ID: "a1", ProductID: "product-1", QueueSequence: 1, ExternalUserID: "user-1",
			IdempotencyKey: "key", State: "waiting", CreatedAt: now, UpdatedAt: now,
		}},
	)
	if !result.Passed {
		t.Fatalf("Evaluate() failed valid state: %+v", result.Checks)
	}
}
