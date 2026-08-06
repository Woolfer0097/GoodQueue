package postgres

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/Woolfer0097/GoodQueue/internal/pkg/domain"
	"github.com/google/uuid"
)

func TestIntegrationQueueBufferMetrics(t *testing.T) {
	database := openIntegrationDatabase(t)
	productID := mustProductID(t, integrationProductOne)
	resetIntegrationProduct(t, database, productID, 3)
	t.Cleanup(func() { resetIntegrationProduct(t, database, productID, 3) })

	now := time.Now().UTC()
	insertMetricAttempt(t, database, productID, 1, "metrics-waiting", domain.QueueAttemptWaiting, now.Add(-20*time.Minute))
	insertMetricAttempt(t, database, productID, 2, "metrics-active", domain.QueueAttemptInvited, now.Add(-15*time.Minute))
	insertMetricAttempt(t, database, productID, 3, "metrics-purchased", domain.QueueAttemptPurchased, now.Add(-10*time.Minute))
	insertMetricAttempt(t, database, productID, 4, "metrics-expired", domain.QueueAttemptInviteExpired, now.Add(-5*time.Minute))

	metrics, err := NewQueueMetricsRepository(database).QueueBufferMetrics(
		context.Background(), now.Add(-time.Hour), now.Add(time.Minute),
	)
	if err != nil {
		t.Fatalf("read metrics: %v", err)
	}
	var got *domain.QueueBufferMetrics
	for index := range metrics {
		if metrics[index].ProductID == productID {
			got = &metrics[index]
			break
		}
	}
	if got == nil {
		t.Fatal("product metrics not returned")
	}
	if got.JoinedAttempts != 4 || got.IssuedRights != 3 || got.ActiveRights != 1 ||
		got.ResolvedRights != 2 || got.Purchases != 1 || got.InviteExpired != 1 {
		t.Fatalf("unexpected product metrics: %+v", *got)
	}
}

func insertMetricAttempt(
	t *testing.T,
	database *sql.DB,
	productID domain.ProductID,
	sequence int64,
	user string,
	state domain.QueueAttemptState,
	created time.Time,
) {
	t.Helper()
	id := uuid.New()
	invited := created.Add(time.Minute)
	invitationDeadline := invited.Add(10 * time.Minute)
	checkout := invited.Add(time.Minute)
	checkoutDeadline := checkout.Add(5 * time.Minute)
	var terminal time.Time
	var statement string
	var arguments []any
	switch state {
	case domain.QueueAttemptWaiting:
		statement = `INSERT INTO queue_attempts
			(id,product_id,queue_sequence,external_user_id,idempotency_key,state,created_at,updated_at)
			VALUES ($1,$2,$3,$4,$4,'waiting',$5,$5)`
		arguments = []any{id, uuid.UUID(productID), sequence, user, created}
	case domain.QueueAttemptInvited:
		statement = `INSERT INTO queue_attempts
			(id,product_id,queue_sequence,external_user_id,idempotency_key,state,created_at,updated_at,invited_at,invitation_deadline)
			VALUES ($1,$2,$3,$4,$4,'invited',$5,$6,$6,$7)`
		arguments = []any{id, uuid.UUID(productID), sequence, user, created, invited, invitationDeadline}
	case domain.QueueAttemptPurchased:
		terminal = checkout.Add(time.Minute)
		statement = `INSERT INTO queue_attempts
			(id,product_id,queue_sequence,external_user_id,idempotency_key,state,created_at,updated_at,
			 invited_at,invitation_deadline,checkout_started_at,checkout_deadline,terminal_at,purchased_at,
			 accepted_payment_provider,accepted_payment_reference,terminal_reason)
			VALUES ($1,$2,$3,$4,$4,'purchased',$5,$10,$6,$7,$8,$9,$10,$10,'test',$4,'purchased')`
		arguments = []any{id, uuid.UUID(productID), sequence, user, created, invited,
			invitationDeadline, checkout, checkoutDeadline, terminal}
	case domain.QueueAttemptInviteExpired:
		terminal = invitationDeadline
		statement = `INSERT INTO queue_attempts
			(id,product_id,queue_sequence,external_user_id,idempotency_key,state,created_at,updated_at,
			 invited_at,invitation_deadline,terminal_at,terminal_reason)
			VALUES ($1,$2,$3,$4,$4,'invite_expired',$5,$8,$6,$7,$8,'invite_expired')`
		arguments = []any{id, uuid.UUID(productID), sequence, user, created, invited, invitationDeadline, terminal}
	default:
		t.Fatalf("unsupported metric state %s", state)
	}
	if _, err := database.Exec(statement, arguments...); err != nil {
		t.Fatalf("insert %s metric attempt: %v", state, err)
	}
}
