package postgres

import (
	"testing"
	"time"

	"github.com/Woolfer0097/GoodQueue/internal/pkg/domain"
)

func TestQueueAttemptStateDecisions(t *testing.T) {
	activeStates := []domain.QueueAttemptState{
		domain.QueueAttemptWaiting,
		domain.QueueAttemptInvited,
		domain.QueueAttemptCheckout,
	}
	for _, state := range activeStates {
		if !isActiveState(state) {
			t.Fatalf("expected %s to be active", state)
		}
	}

	reservedStates := []domain.QueueAttemptState{domain.QueueAttemptInvited, domain.QueueAttemptCheckout}
	for _, state := range reservedStates {
		if !reservesStock(state) {
			t.Fatalf("expected %s to reserve stock", state)
		}
	}
	if reservesStock(domain.QueueAttemptWaiting) {
		t.Fatal("waiting attempt must not reserve stock")
	}
	if isTerminalState(domain.QueueAttemptPurchased) {
		t.Fatal("purchased is handled as a distinct conflict state")
	}
	if !isTerminalState(domain.QueueAttemptCancelled) {
		t.Fatal("cancelled must be terminal")
	}
}

func TestReservedAttemptIsNotEffectiveAtOrAfterDeadline(t *testing.T) {
	deadline := time.Date(2026, time.August, 6, 12, 0, 0, 0, time.UTC)
	for _, attempt := range []domain.QueueAttempt{
		{State: domain.QueueAttemptInvited, InvitationDeadline: &deadline},
		{State: domain.QueueAttemptCheckout, CheckoutDeadline: &deadline},
	} {
		if isEffectiveAt(attempt, deadline) {
			t.Fatalf("%s attempt was effective at its deadline", attempt.State)
		}
		if isEffectiveAt(attempt, deadline.Add(time.Nanosecond)) {
			t.Fatalf("%s attempt was effective after its deadline", attempt.State)
		}
		if !isEffectiveAt(attempt, deadline.Add(-time.Nanosecond)) {
			t.Fatalf("%s attempt was not effective before its deadline", attempt.State)
		}
	}
}
