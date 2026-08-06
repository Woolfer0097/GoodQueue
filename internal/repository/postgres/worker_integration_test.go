package postgres

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/Woolfer0097/GoodQueue/internal/pkg/domain"
	"github.com/google/uuid"
)

func TestIntegrationBoundedReconciliationAndCycleExclusion(t *testing.T) {
	database := openIntegrationDatabase(t)
	repository := NewQueueAttemptRepository(database, 10*time.Minute, 5*time.Minute, 100)
	productOne := mustProductID(t, integrationProductOne)
	productTwo := mustProductID(t, integrationProductTwo)
	resetIntegrationProduct(t, database, productOne, 2)
	resetIntegrationProduct(t, database, productTwo, 0)

	ctx := context.Background()
	attempts := make([]domain.QueueAttempt, 4)
	for index := range attempts {
		joined, err := repository.Join(ctx, joinCommand(productOne, 700+index, fmt.Sprintf("worker-%d", index)))
		if err != nil {
			t.Fatalf("prepare attempt %d: %v", index, err)
		}
		attempts[index] = joined.Attempt
	}
	makeAttemptDue(t, database, attempts[0].ID, domain.QueueAttemptCheckout)
	makeAttemptDue(t, database, attempts[1].ID, domain.QueueAttemptCheckout)
	insertWaitingAttempt(t, database, productTwo, 1, "worker-sold-out")

	first, err := repository.ReconcileNextProduct(ctx, 1, []domain.ProductID{productTwo})
	if err != nil {
		t.Fatalf("first bounded reconciliation: %v", err)
	}
	if first.Transitions != 1 || !first.MoreWork {
		t.Fatalf("unexpected first batch: %+v", first)
	}
	if first.ProductID != productOne {
		t.Fatalf("unexpected bounded product: got %v, want %v", first.ProductID, productOne)
	}
	secondProduct, err := repository.ReconcileNextProduct(ctx, 1, []domain.ProductID{first.ProductID})
	if err != nil {
		t.Fatalf("excluded-product reconciliation: %v", err)
	}
	if secondProduct.ProductID == first.ProductID || secondProduct.Transitions != 1 {
		t.Fatalf("hot product starved independent product: first=%+v second=%+v", first, secondProduct)
	}

	for iteration := 0; iteration < 5; iteration++ {
		result, reconcileErr := repository.ReconcileNextProduct(ctx, 1, nil)
		if reconcileErr != nil {
			t.Fatalf("reconciliation batch %d: %v", iteration, reconcileErr)
		}
		if result.ProductID == (domain.ProductID{}) {
			break
		}
	}
	assertAttemptState(t, database, attempts[0].ID, domain.QueueAttemptCheckoutExpired)
	assertAttemptState(t, database, attempts[1].ID, domain.QueueAttemptCheckoutExpired)
	assertAttemptState(t, database, attempts[2].ID, domain.QueueAttemptInvited)
	assertAttemptState(t, database, attempts[3].ID, domain.QueueAttemptInvited)
	assertReservedMatchesAttempts(t, database, productOne)
}

func TestIntegrationConcurrentReconcilersDistributeProducts(t *testing.T) {
	database := openIntegrationDatabase(t)
	repository := NewQueueAttemptRepository(database, time.Minute, time.Minute, 100)
	productOne := mustProductID(t, integrationProductOne)
	productTwo := mustProductID(t, integrationProductTwo)
	resetIntegrationProduct(t, database, productOne, 0)
	resetIntegrationProduct(t, database, productTwo, 0)
	insertWaitingAttempt(t, database, productOne, 1, "concurrent-worker-one")
	insertWaitingAttempt(t, database, productTwo, 1, "concurrent-worker-two")

	if _, err := database.Exec(`UPDATE products SET updated_at=created_at WHERE id=$1`, uuid.UUID(productOne)); err != nil {
		t.Fatalf("prioritize first product: %v", err)
	}
	firstLocked := make(chan struct{})
	releaseFirst := make(chan struct{})
	var releaseOnce sync.Once
	t.Cleanup(func() { releaseOnce.Do(func() { close(releaseFirst) }) })
	repository.afterProductSelected = func(productID domain.ProductID) {
		if productID != productOne {
			return
		}
		close(firstLocked)
		<-releaseFirst
	}

	firstResult := make(chan domain.ReconciliationResult, 1)
	firstError := make(chan error, 1)
	go func() {
		result, err := repository.ReconcileNextProduct(context.Background(), 10, nil)
		firstResult <- result
		firstError <- err
	}()
	select {
	case <-firstLocked:
	case <-time.After(time.Second):
		t.Fatal("first reconciler did not hold the selected product lock")
	}

	secondResult := make(chan domain.ReconciliationResult, 1)
	secondError := make(chan error, 1)
	go func() {
		result, err := repository.ReconcileNextProduct(context.Background(), 10, nil)
		secondResult <- result
		secondError <- err
	}()
	var second domain.ReconciliationResult
	select {
	case second = <-secondResult:
	case <-time.After(time.Second):
		t.Fatal("second reconciler blocked instead of using SKIP LOCKED")
	}
	if err := <-secondError; err != nil {
		t.Fatalf("second concurrent reconciler: %v", err)
	}
	if second.ProductID != productTwo {
		t.Fatalf("second reconciler selected %v, want unlocked product %v", second.ProductID, productTwo)
	}

	releaseOnce.Do(func() { close(releaseFirst) })
	first := <-firstResult
	if err := <-firstError; err != nil {
		t.Fatalf("first concurrent reconciler: %v", err)
	}
	if first.ProductID != productOne {
		t.Fatalf("first reconciler selected %v, want %v", first.ProductID, productOne)
	}
}

func TestIntegrationReconciliationCursorRotatesAcrossSaturatedCycles(t *testing.T) {
	database := openIntegrationDatabase(t)
	repository := NewQueueAttemptRepository(database, time.Minute, time.Minute, 100)
	products := []domain.ProductID{
		mustProductID(t, integrationProductOne),
		mustProductID(t, integrationProductTwo),
		mustProductID(t, "33333333-3333-3333-3333-333333333333"),
	}
	for index, productID := range products {
		resetIntegrationProduct(t, database, productID, 0)
		insertWaitingAttempt(t, database, productID, 1, fmt.Sprintf("cursor-%d-a", index))
		insertWaitingAttempt(t, database, productID, 2, fmt.Sprintf("cursor-%d-b", index))
	}
	if _, err := database.Exec(`UPDATE products SET updated_at=created_at`); err != nil {
		t.Fatalf("align fairness cursors: %v", err)
	}

	seen := make(map[domain.ProductID]bool, len(products))
	const maxProductsPerCycle = 1
	for cycle := 0; cycle < len(products); cycle++ {
		for range maxProductsPerCycle {
			result, err := repository.ReconcileNextProduct(context.Background(), 1, nil)
			if err != nil {
				t.Fatalf("reconciliation cycle %d: %v", cycle, err)
			}
			if result.Transitions != 1 {
				t.Fatalf("cycle %d did not saturate its transition batch: %+v", cycle, result)
			}
			seen[result.ProductID] = true
		}
	}
	if len(seen) != len(products) {
		t.Fatalf("later products starved across saturated cycles: saw %d of %d products", len(seen), len(products))
	}
}

func TestIntegrationReconciliationCursorAdvancesForNetZeroTransitionsButNotEmptySelection(t *testing.T) {
	database := openIntegrationDatabase(t)
	repository := NewQueueAttemptRepository(database, time.Minute, time.Minute, 100)
	productID := mustProductID(t, integrationProductOne)
	otherProducts := []domain.ProductID{
		mustProductID(t, integrationProductTwo),
		mustProductID(t, "33333333-3333-3333-3333-333333333333"),
	}
	resetIntegrationProduct(t, database, productID, 1)
	first, err := repository.Join(context.Background(), joinCommand(productID, 910, "cursor-net-zero-first"))
	if err != nil {
		t.Fatalf("join reserved attempt: %v", err)
	}
	if _, err := repository.Join(context.Background(), joinCommand(productID, 911, "cursor-net-zero-second")); err != nil {
		t.Fatalf("join waiting attempt: %v", err)
	}
	makeAttemptDue(t, database, first.Attempt.ID, domain.QueueAttemptCheckout)
	var before time.Time
	if err := database.QueryRow(`SELECT updated_at FROM products WHERE id=$1`, uuid.UUID(productID)).Scan(&before); err != nil {
		t.Fatalf("read cursor before reconciliation: %v", err)
	}

	result, err := repository.ReconcileNextProduct(context.Background(), 2, otherProducts)
	if err != nil {
		t.Fatalf("net-zero reconciliation: %v", err)
	}
	if result.Transitions != 2 {
		t.Fatalf("expected expiry balanced by promotion, got %+v", result)
	}
	var after time.Time
	if err := database.QueryRow(`SELECT updated_at FROM products WHERE id=$1`, uuid.UUID(productID)).Scan(&after); err != nil {
		t.Fatalf("read cursor after reconciliation: %v", err)
	}
	if !after.After(before) {
		t.Fatalf("net-zero reconciliation did not advance cursor: before=%s after=%s", before, after)
	}

	empty, err := repository.ReconcileNextProduct(context.Background(), 2, otherProducts)
	if err != nil || empty.ProductID != (domain.ProductID{}) {
		t.Fatalf("expected empty reconciliation selection: %+v, %v", empty, err)
	}
	var afterEmpty time.Time
	if err := database.QueryRow(`SELECT updated_at FROM products WHERE id=$1`, uuid.UUID(productID)).Scan(&afterEmpty); err != nil {
		t.Fatalf("read cursor after empty reconciliation: %v", err)
	}
	if !afterEmpty.Equal(after) {
		t.Fatalf("empty selection churned cursor: before=%s after=%s", after, afterEmpty)
	}
}

func TestIntegrationNotificationOutboxFencingAndRetry(t *testing.T) {
	database := openIntegrationDatabase(t)
	if _, err := database.Exec(`DELETE FROM notification_outbox`); err != nil {
		t.Fatalf("clear outbox: %v", err)
	}
	payload := []byte(`{"payment_inbox_id":"demo"}`)
	digest := sha256.Sum256(payload)
	outboxID := uuid.New()
	if _, err := database.Exec(`
		INSERT INTO notification_outbox(id,event_type,deduplication_key,payload,payload_hash,available_at)
		VALUES($1,'payment.compensation_required',$2,$3,$4,clock_timestamp())`,
		outboxID, "worker-test:"+outboxID.String(), payload, digest[:]); err != nil {
		t.Fatalf("insert outbox event: %v", err)
	}

	repository := NewNotificationOutboxRepository(database)
	claim, err := repository.ClaimNotification(context.Background(), time.Minute)
	if err != nil || claim == nil {
		t.Fatalf("claim outbox event: %+v, %v", claim, err)
	}
	if claim.ID != outboxID || claim.AttemptCount != 1 || claim.LeaseGeneration != 1 || claim.PayloadHash != digest {
		t.Fatalf("unexpected claim: %+v", claim)
	}
	second, err := repository.ClaimNotification(context.Background(), time.Minute)
	if err != nil || second != nil {
		t.Fatalf("live lease was reclaimed: %+v, %v", second, err)
	}

	stale := *claim
	stale.LeaseToken = uuid.New()
	if err := repository.MarkNotificationSent(context.Background(), stale); !errors.Is(err, domain.ErrStaleClaim) {
		t.Fatalf("wrong token finalize: %v", err)
	}
	backoff, err := repository.RetryNotification(context.Background(), *claim, errors.New("temporary"), time.Second, time.Minute)
	if err != nil || backoff != time.Second {
		t.Fatalf("schedule retry: %s, %v", backoff, err)
	}
	notDue, err := repository.ClaimNotification(context.Background(), time.Minute)
	if err != nil || notDue != nil {
		t.Fatalf("retry was immediately claimable: %+v, %v", notDue, err)
	}
	if _, err := database.Exec(`UPDATE notification_outbox SET available_at=clock_timestamp()-interval '1 second' WHERE id=$1`, outboxID); err != nil {
		t.Fatalf("make retry due: %v", err)
	}
	rotated, err := repository.ClaimNotification(context.Background(), time.Minute)
	if err != nil || rotated == nil {
		t.Fatalf("reclaim retry: %+v, %v", rotated, err)
	}
	if rotated.LeaseToken == claim.LeaseToken || rotated.LeaseGeneration != claim.LeaseGeneration+1 {
		t.Fatalf("lease fencing did not rotate: old=%+v new=%+v", claim, rotated)
	}
	if err := repository.MarkNotificationSent(context.Background(), *rotated); err != nil {
		t.Fatalf("mark sent: %v", err)
	}
	var status string
	var leaseCleared, sent bool
	if err := database.QueryRow(`SELECT status,lease_token IS NULL AND lease_until IS NULL,sent_at IS NOT NULL FROM notification_outbox WHERE id=$1`, outboxID).
		Scan(&status, &leaseCleared, &sent); err != nil {
		t.Fatalf("read sent outbox: %v", err)
	}
	if status != "sent" || !leaseCleared || !sent {
		t.Fatalf("invalid sent state: status=%s leaseCleared=%t sent=%t", status, leaseCleared, sent)
	}
}

func TestIntegrationConcurrentOutboxClaimersRespectLiveLease(t *testing.T) {
	database := openIntegrationDatabase(t)
	clearNotificationOutbox(t, database)
	outboxID, _, _ := insertNotification(t, database)
	repository := NewNotificationOutboxRepository(database)
	firstLocked := make(chan struct{})
	releaseFirst := make(chan struct{})
	var releaseOnce sync.Once
	t.Cleanup(func() { releaseOnce.Do(func() { close(releaseFirst) }) })
	repository.afterClaimLocked = func(claim domain.NotificationClaim) {
		if claim.ID != outboxID || claim.LeaseGeneration != 1 {
			return
		}
		close(firstLocked)
		<-releaseFirst
	}

	firstClaim := make(chan *domain.NotificationClaim, 1)
	firstError := make(chan error, 1)
	go func() {
		claim, err := repository.ClaimNotification(context.Background(), time.Minute)
		firstClaim <- claim
		firstError <- err
	}()
	select {
	case <-firstLocked:
	case <-time.After(time.Second):
		t.Fatal("first claimer did not hold the outbox row lock")
	}

	secondClaim := make(chan *domain.NotificationClaim, 1)
	secondError := make(chan error, 1)
	go func() {
		claim, err := repository.ClaimNotification(context.Background(), time.Minute)
		secondClaim <- claim
		secondError <- err
	}()
	select {
	case claim := <-secondClaim:
		if claim != nil {
			t.Fatalf("second claimer acquired live row: %+v", claim)
		}
	case <-time.After(time.Second):
		t.Fatal("second claimer blocked instead of skipping the locked outbox row")
	}
	if err := <-secondError; err != nil {
		t.Fatalf("second claimer: %v", err)
	}

	releaseOnce.Do(func() { close(releaseFirst) })
	if claim := <-firstClaim; claim == nil || claim.ID != outboxID {
		t.Fatalf("first claimer did not acquire row: %+v", claim)
	}
	if err := <-firstError; err != nil {
		t.Fatalf("first claimer: %v", err)
	}
}

func TestIntegrationReclaimedOutboxRejectsEveryStaleFencingVariant(t *testing.T) {
	database := openIntegrationDatabase(t)
	clearNotificationOutbox(t, database)
	outboxID, _, _ := insertNotification(t, database)
	repository := NewNotificationOutboxRepository(database)
	oldClaim, err := repository.ClaimNotification(context.Background(), time.Minute)
	if err != nil || oldClaim == nil {
		t.Fatalf("initial claim: %+v, %v", oldClaim, err)
	}
	if _, err := database.Exec(`UPDATE notification_outbox SET lease_until=clock_timestamp()-interval '1 second' WHERE id=$1`, outboxID); err != nil {
		t.Fatalf("expire initial lease: %v", err)
	}
	currentClaim, err := repository.ClaimNotification(context.Background(), time.Minute)
	if err != nil || currentClaim == nil {
		t.Fatalf("reclaim expired lease: %+v, %v", currentClaim, err)
	}
	before := readOutboxFenceState(t, database, outboxID)

	wrongHash := currentClaim.PayloadHash
	wrongHash[0] ^= 0xff
	variants := []struct {
		name  string
		claim domain.NotificationClaim
	}{
		{name: "old token and generation", claim: *oldClaim},
		{name: "old token", claim: withOutboxFence(*currentClaim, oldClaim.LeaseToken, currentClaim.LeaseGeneration, currentClaim.PayloadHash)},
		{name: "old generation", claim: withOutboxFence(*currentClaim, currentClaim.LeaseToken, oldClaim.LeaseGeneration, currentClaim.PayloadHash)},
		{name: "wrong token", claim: withOutboxFence(*currentClaim, uuid.New(), currentClaim.LeaseGeneration, currentClaim.PayloadHash)},
		{name: "wrong generation", claim: withOutboxFence(*currentClaim, currentClaim.LeaseToken, currentClaim.LeaseGeneration+1, currentClaim.PayloadHash)},
		{name: "mismatched payload hash", claim: withOutboxFence(*currentClaim, currentClaim.LeaseToken, currentClaim.LeaseGeneration, wrongHash)},
	}
	finalizers := []struct {
		name string
		run  func(domain.NotificationClaim) error
	}{
		{name: "sent", run: func(claim domain.NotificationClaim) error {
			return repository.MarkNotificationSent(context.Background(), claim)
		}},
		{name: "obsolete", run: func(claim domain.NotificationClaim) error {
			return repository.MarkNotificationObsolete(context.Background(), claim)
		}},
		{name: "retry", run: func(claim domain.NotificationClaim) error {
			_, retryErr := repository.RetryNotification(context.Background(), claim, errors.New("stale"), time.Second, time.Minute)
			return retryErr
		}},
	}
	for _, finalizer := range finalizers {
		for _, variant := range variants {
			t.Run(finalizer.name+"/"+variant.name, func(t *testing.T) {
				if err := finalizer.run(variant.claim); !errors.Is(err, domain.ErrStaleClaim) {
					t.Fatalf("expected ErrStaleClaim, got %v", err)
				}
				if after := readOutboxFenceState(t, database, outboxID); after != before {
					t.Fatalf("stale finalization altered reclaimed row: before=%+v after=%+v", before, after)
				}
			})
		}
	}
}

type outboxFenceState struct {
	status          string
	leaseToken      uuid.UUID
	leaseGeneration int64
	attemptCount    int
	lastError       string
	payloadHash     string
	leaseUntil      time.Time
	availableAt     time.Time
	updatedAt       time.Time
	sentAt          sql.NullTime
}

func clearNotificationOutbox(t *testing.T, database *sql.DB) {
	t.Helper()
	if _, err := database.Exec(`DELETE FROM notification_outbox`); err != nil {
		t.Fatalf("clear outbox: %v", err)
	}
}

func insertNotification(t *testing.T, database *sql.DB) (uuid.UUID, []byte, [sha256.Size]byte) {
	t.Helper()
	payload := []byte(`{"payment_inbox_id":"concurrency-test"}`)
	digest := sha256.Sum256(payload)
	outboxID := uuid.New()
	if _, err := database.Exec(`
		INSERT INTO notification_outbox(id,event_type,deduplication_key,payload,payload_hash,available_at)
		VALUES($1,'payment.compensation_required',$2,$3,$4,clock_timestamp())`,
		outboxID, "worker-concurrency:"+outboxID.String(), payload, digest[:]); err != nil {
		t.Fatalf("insert outbox notification: %v", err)
	}
	return outboxID, payload, digest
}

func withOutboxFence(
	claim domain.NotificationClaim,
	token uuid.UUID,
	generation int64,
	payloadHash [sha256.Size]byte,
) domain.NotificationClaim {
	claim.LeaseToken = token
	claim.LeaseGeneration = generation
	claim.PayloadHash = payloadHash
	return claim
}

func readOutboxFenceState(t *testing.T, database *sql.DB, outboxID uuid.UUID) outboxFenceState {
	t.Helper()
	var state outboxFenceState
	if err := database.QueryRow(`
		SELECT status,lease_token,lease_generation,attempt_count,COALESCE(last_error,''),
		       encode(payload_hash,'hex'),lease_until,available_at,updated_at,sent_at
		FROM notification_outbox WHERE id=$1`, outboxID).Scan(
		&state.status,
		&state.leaseToken,
		&state.leaseGeneration,
		&state.attemptCount,
		&state.lastError,
		&state.payloadHash,
		&state.leaseUntil,
		&state.availableAt,
		&state.updatedAt,
		&state.sentAt,
	); err != nil {
		t.Fatalf("read outbox fence state: %v", err)
	}
	return state
}
