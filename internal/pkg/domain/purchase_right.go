package domain

import (
	"time"

	"github.com/google/uuid"
)

type PurchaseRightStatus string

const (
	PurchaseRightActive   PurchaseRightStatus = "active"
	PurchaseRightExpired  PurchaseRightStatus = "expired"
	PurchaseRightConsumed PurchaseRightStatus = "consumed"
)

type PurchaseRight struct {
	ID            uuid.UUID
	QueueTicketID int64
	ProductID     ProductID
	Status        PurchaseRightStatus
	IssuedAt      time.Time
	ExpiresAt     time.Time
	ConsumedAt    *time.Time
}
