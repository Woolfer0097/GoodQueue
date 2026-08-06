package domain

import "time"

// QueueBufferMetrics describes one product cohort for a queue-buffer experiment.
// Rights conversion only uses resolved rights, so active checkouts do not make the
// measured conversion artificially worse.
type QueueBufferMetrics struct {
	ProductID               ProductID
	ProductTitle            string
	JoinedAttempts          int64
	IssuedRights            int64
	ActiveRights            int64
	ResolvedRights          int64
	Purchases               int64
	InviteExpired           int64
	CheckoutExpired         int64
	PaymentFailed           int64
	CancelledAfterRight     int64
	AverageQueueWaitSeconds float64
	P95QueueWaitSeconds     float64
}

type QueueBufferReport struct {
	WindowStart          time.Time
	WindowEnd            time.Time
	WaitingBufferPercent int
	Totals               QueueBufferMetrics
	Products             []QueueBufferMetrics
}
