package domain

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"math"
	"strings"

	"github.com/google/uuid"
)

const (
	MaxPaymentProviderLength  = 64
	MaxPaymentEventIDLength   = 200
	MaxPaymentReferenceLength = 200
)

type PaymentCommand struct {
	Provider         string
	EventID          string
	AttemptID        AttemptID
	Outcome          PaymentOutcome
	PaymentReference string
}

type canonicalPaymentPayload struct {
	Version          string `json:"version"`
	Provider         string `json:"provider"`
	EventID          string `json:"event_id"`
	AttemptID        string `json:"attempt_id"`
	Outcome          string `json:"outcome"`
	PaymentReference string `json:"payment_reference"`
}

type PaymentResult struct {
	InboxID      uuid.UUID
	Code         string
	HTTPStatus   int
	ResponseBody []byte
	Replayed     bool
}

type PaymentClaim struct {
	InboxID     uuid.UUID
	Token       uuid.UUID
	Generation  int64
	PayloadHash [sha256.Size]byte
	Command     PaymentCommand
}

type PaymentClaimResult struct {
	Claim    *PaymentClaim
	Response *PaymentResult
}

func ParsePaymentCommand(provider, eventID, attemptID, outcome, paymentReference string) (PaymentCommand, error) {
	normalizedProvider, err := normalizeRequiredPaymentField("provider", provider, MaxPaymentProviderLength)
	if err != nil {
		return PaymentCommand{}, err
	}
	normalizedEventID, err := normalizeRequiredPaymentField("event ID", eventID, MaxPaymentEventIDLength)
	if err != nil {
		return PaymentCommand{}, err
	}
	parsedAttemptID, err := ParseAttemptID(strings.TrimSpace(attemptID))
	if err != nil {
		return PaymentCommand{}, err
	}
	normalizedOutcome := PaymentOutcome(strings.ToLower(strings.TrimSpace(outcome)))
	if normalizedOutcome != PaymentSucceeded && normalizedOutcome != PaymentFailed {
		return PaymentCommand{}, fmt.Errorf("%w: payment outcome must be succeeded or failed", ErrInvalidInput)
	}

	normalizedReference := strings.TrimSpace(paymentReference)
	if normalizedOutcome == PaymentFailed {
		normalizedReference = ""
	}
	if normalizedOutcome == PaymentSucceeded && normalizedReference == "" {
		return PaymentCommand{}, fmt.Errorf("%w: succeeded payment reference is required", ErrInvalidInput)
	}
	if len(normalizedReference) > MaxPaymentReferenceLength {
		return PaymentCommand{}, fmt.Errorf("%w: payment reference exceeds %d bytes", ErrInvalidInput, MaxPaymentReferenceLength)
	}

	return PaymentCommand{
		Provider:         normalizedProvider,
		EventID:          normalizedEventID,
		AttemptID:        parsedAttemptID,
		Outcome:          normalizedOutcome,
		PaymentReference: normalizedReference,
	}, nil
}

func CanonicalPaymentPayload(command PaymentCommand) []byte {
	payload, err := json.Marshal(canonicalPaymentPayload{
		Version:          "v1",
		Provider:         command.Provider,
		EventID:          command.EventID,
		AttemptID:        strings.ToLower(uuid.UUID(command.AttemptID).String()),
		Outcome:          string(command.Outcome),
		PaymentReference: command.PaymentReference,
	})
	if err != nil {
		panic(fmt.Sprintf("marshal canonical payment payload: %v", err))
	}
	return payload
}

func CanonicalPaymentHash(command PaymentCommand) [sha256.Size]byte {
	fields := []string{
		"v1",
		command.Provider,
		command.EventID,
		strings.ToLower(uuid.UUID(command.AttemptID).String()),
		string(command.Outcome),
		command.PaymentReference,
	}
	hasher := sha256.New()
	var length [4]byte
	for _, field := range fields {
		if uint64(len(field)) > math.MaxUint32 {
			panic("canonical payment field exceeds uint32 length")
		}
		binary.BigEndian.PutUint32(length[:], uint32(len(field))) //nolint:gosec // Explicitly bounded above.
		_, _ = hasher.Write(length[:])
		_, _ = hasher.Write([]byte(field))
	}
	var digest [sha256.Size]byte
	copy(digest[:], hasher.Sum(nil))
	return digest
}

func normalizeRequiredPaymentField(name, value string, maxLength int) (string, error) {
	normalized := strings.TrimSpace(value)
	if normalized == "" {
		return "", fmt.Errorf("%w: payment %s is required", ErrInvalidInput, name)
	}
	if len(normalized) > maxLength {
		return "", fmt.Errorf("%w: payment %s exceeds %d bytes", ErrInvalidInput, name, maxLength)
	}
	return normalized, nil
}
