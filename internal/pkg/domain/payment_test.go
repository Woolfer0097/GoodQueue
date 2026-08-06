package domain

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"
)

func TestParsePaymentCommandNormalizesCanonicalFields(t *testing.T) {
	command, err := ParsePaymentCommand(
		" provider ",
		" event-1 ",
		" AAAAAAAA-AAAA-4AAA-8AAA-AAAAAAAAAAAA ",
		" SUCCEEDED ",
		" reference-1 ",
	)
	if err != nil {
		t.Fatalf("parse payment: %v", err)
	}
	if command.Provider != "provider" || command.EventID != "event-1" || command.Outcome != PaymentSucceeded ||
		command.PaymentReference != "reference-1" || uuid.UUID(command.AttemptID).String() != "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa" {
		t.Fatalf("unexpected normalized command: %+v", command)
	}
}

func TestParseFailedPaymentDiscardsReference(t *testing.T) {
	command, err := ParsePaymentCommand("provider", "event", uuid.NewString(), " FAILED ", " reference-is-ignored ")
	if err != nil {
		t.Fatalf("parse failed payment: %v", err)
	}
	if command.PaymentReference != "" {
		t.Fatalf("failed reference = %q, want empty", command.PaymentReference)
	}
}

func TestParsePaymentCommandBoundaries(t *testing.T) {
	attemptID := uuid.NewString()
	if _, err := ParsePaymentCommand(strings.Repeat("p", MaxPaymentProviderLength), strings.Repeat("e", MaxPaymentEventIDLength), attemptID, "succeeded", strings.Repeat("r", MaxPaymentReferenceLength)); err != nil {
		t.Fatalf("maximum lengths rejected: %v", err)
	}
	for _, testCase := range []struct {
		name      string
		provider  string
		eventID   string
		outcome   string
		reference string
	}{
		{name: "provider too long", provider: strings.Repeat("p", MaxPaymentProviderLength+1), eventID: "e", outcome: "failed"},
		{name: "event too long", provider: "p", eventID: strings.Repeat("e", MaxPaymentEventIDLength+1), outcome: "failed"},
		{name: "reference too long", provider: "p", eventID: "e", outcome: "succeeded", reference: strings.Repeat("r", MaxPaymentReferenceLength+1)},
		{name: "missing success reference", provider: "p", eventID: "e", outcome: "succeeded"},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			_, err := ParsePaymentCommand(testCase.provider, testCase.eventID, attemptID, testCase.outcome, testCase.reference)
			if !errors.Is(err, ErrInvalidInput) {
				t.Fatalf("got %v, want invalid input", err)
			}
		})
	}
}

func TestCanonicalPaymentHashHasFixedOrderAndLengthFraming(t *testing.T) {
	command, err := ParsePaymentCommand("provider", "event", "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa", "succeeded", "reference")
	if err != nil {
		t.Fatal(err)
	}
	digest := CanonicalPaymentHash(command)
	framed := make([]byte, 0)
	for _, field := range []string{"v1", "provider", "event", "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa", "succeeded", "reference"} {
		var length [4]byte
		binary.BigEndian.PutUint32(length[:], uint32(len(field))) //nolint:gosec // Test fields are fixed and far below uint32.
		framed = append(framed, length[:]...)
		framed = append(framed, field...)
	}
	wantDigest := sha256.Sum256(framed)
	if digest != wantDigest {
		t.Fatalf("canonical digest = %x, want %x", digest, wantDigest)
	}
	changed := command
	changed.Provider, changed.EventID = command.EventID, command.Provider
	changedDigest := CanonicalPaymentHash(changed)
	if bytes.Equal(digest[:], changedDigest[:]) {
		t.Fatal("field ordering did not affect canonical digest")
	}

	ambiguousOne := command
	ambiguousOne.Provider, ambiguousOne.EventID = "ab", "c"
	ambiguousTwo := command
	ambiguousTwo.Provider, ambiguousTwo.EventID = "a", "bc"
	ambiguousOneDigest := CanonicalPaymentHash(ambiguousOne)
	ambiguousTwoDigest := CanonicalPaymentHash(ambiguousTwo)
	if bytes.Equal(ambiguousOneDigest[:], ambiguousTwoDigest[:]) {
		t.Fatal("length framing did not distinguish ambiguous concatenation")
	}

	wantPayload := `{"version":"v1","provider":"provider","event_id":"event","attempt_id":"aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa","outcome":"succeeded","payment_reference":"reference"}`
	if got := string(CanonicalPaymentPayload(command)); got != wantPayload {
		t.Fatalf("canonical payload = %s", got)
	}
}
