package domain

import "time"

type QueueEntryStatus string

const (
	QueueEntryWaiting     QueueEntryStatus = "waiting"
	QueueEntryRightIssued QueueEntryStatus = "right_issued"
	QueueEntryCompleted   QueueEntryStatus = "completed"
	QueueEntryCancelled   QueueEntryStatus = "cancelled"
	QueueEntryExpired     QueueEntryStatus = "expired"
)

type QueueEntry struct {
	TicketID       int64
	ProductID      ProductID
	ExternalUserID ExternalUserID
	Status         QueueEntryStatus
	JoinedAt       time.Time
	RightIssuedAt  *time.Time
	CompletedAt    *time.Time
	CancelledAt    *time.Time
	ExpiredAt      *time.Time
}
