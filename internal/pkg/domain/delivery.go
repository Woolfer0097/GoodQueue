package domain

import (
	"crypto/sha256"
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

type PaymentOutcome string

const (
	PaymentSucceeded PaymentOutcome = "succeeded"
	PaymentFailed    PaymentOutcome = "failed"
)

type ReconciliationResult struct {
	ProductID   ProductID
	Transitions int
	MoreWork    bool
}

type NotificationClaim struct {
	ID               uuid.UUID
	AttemptID        *AttemptID
	EventType        string
	DeduplicationKey string
	Payload          json.RawMessage
	PayloadHash      [sha256.Size]byte
	AttemptCount     int
	LeaseToken       uuid.UUID
	LeaseGeneration  int64
	LeaseUntil       time.Time
}

type InboxStatus string

const (
	InboxReceived   InboxStatus = "received"
	InboxProcessing InboxStatus = "processing"
	InboxRetry      InboxStatus = "retry"
	InboxCompleted  InboxStatus = "completed"
	InboxRejected   InboxStatus = "rejected"
)

type OutboxStatus string

const (
	OutboxPending    OutboxStatus = "pending"
	OutboxProcessing OutboxStatus = "processing"
	OutboxSent       OutboxStatus = "sent"
	OutboxFailed     OutboxStatus = "failed"
)
