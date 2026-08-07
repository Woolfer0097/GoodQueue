package postgres

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"errors"
	"fmt"
	"math/rand"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/Woolfer0097/GoodQueue/internal/pkg/domain"
	queueworker "github.com/Woolfer0097/GoodQueue/internal/worker"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"
	"go.uber.org/zap"
)

func TestIntegrationConcurrentJoinStockOneHalfBuffer(t *testing.T) {
	database := openIntegrationDatabase(t)
	applicationName := newLockCompetitorApplicationName()
	competitorDatabase := openIntegrationDatabaseWithApplicationName(t, applicationName)
	repository := NewQueueAttemptRepository(competitorDatabase, 10*time.Minute, 5*time.Minute, 50)
	productID := mustProductID(t, integrationProductOne)
	resetIntegrationProduct(t, database, productID, 1)

	const callers = 24
	start := make(chan struct{})
	results := make(chan error, callers)
	blocker := lockProductForCompetitors(t, database, productID, applicationName)
	var callersReady sync.WaitGroup
	var callersDone sync.WaitGroup
	callersReady.Add(callers)
	callersDone.Add(callers)
	for index := 0; index < callers; index++ {
		go func(userNumber int) {
			defer callersDone.Done()
			callersReady.Done()
			<-start
			_, err := repository.Join(context.Background(), joinCommand(productID, 2000+userNumber, fmt.Sprintf("half-buffer-%d", userNumber)))
			results <- err
		}(index)
	}
	callersReady.Wait()
	close(start)
	blocker.waitUntilCompetitorsBlocked(t, callers)
	blocker.release(t)
	callersDone.Wait()
	close(results)

	queueFull := 0
	for err := range results {
		if errors.Is(err, domain.ErrQueueFull) {
			queueFull++
			continue
		}
		if err != nil {
			t.Fatalf("concurrent join returned %v", err)
		}
	}
	if queueFull != callers-2 {
		t.Fatalf("queue-full results: got %d, want %d", queueFull, callers-2)
	}

	var total, checkout, waiting, distinctSequences int
	var minSequence, maxSequence int64
	var stock, reserved int32
	if err := database.QueryRow(`
		SELECT count(*),count(*) FILTER (WHERE state='checkout'),count(*) FILTER (WHERE state='waiting'),
		       count(DISTINCT queue_sequence),min(queue_sequence),max(queue_sequence)
		FROM queue_attempts WHERE product_id=$1`, uuid.UUID(productID)).Scan(
		&total, &checkout, &waiting, &distinctSequences, &minSequence, &maxSequence,
	); err != nil {
		t.Fatal(err)
	}
	if err := database.QueryRow(`SELECT allocatable_stock,reserved FROM products WHERE id=$1`, uuid.UUID(productID)).Scan(&stock, &reserved); err != nil {
		t.Fatal(err)
	}
	if total != 2 || checkout != 1 || waiting != 1 || distinctSequences != 2 || minSequence != 1 || maxSequence != 2 {
		t.Fatalf("unexpected attempts total=%d checkout=%d waiting=%d distinct=%d range=%d..%d", total, checkout, waiting, distinctSequences, minSequence, maxSequence)
	}
	if stock != 1 || reserved != 1 {
		t.Fatalf("oversold product stock=%d reserved=%d", stock, reserved)
	}
	assertReservedMatchesAttempts(t, database, productID)
}

func TestIntegrationQueueDisabledHasNoMutation(t *testing.T) {
	database := openIntegrationDatabase(t)
	repository := NewQueueAttemptRepository(database, 10*time.Minute, 5*time.Minute, 100)
	productID := mustProductID(t, integrationProductOne)
	resetIntegrationProduct(t, database, productID, 2)
	if _, err := database.Exec(`
		UPDATE products SET title='disabled sentinel',description='must remain unchanged',image_url='https://example.invalid/disabled.png',
			queue_enabled=false,allocatable_stock=2,right_ttl_seconds=137,reserved=0,next_queue_sequence=1,
			updated_at=clock_timestamp()
		WHERE id=$1`, uuid.UUID(productID)); err != nil {
		t.Fatal(err)
	}

	before := readProductMutationState(t, database, productID)
	_, err := repository.Join(context.Background(), joinCommand(productID, 2100, "disabled"))
	if !errors.Is(err, domain.ErrQueueDisabled) {
		t.Fatalf("disabled join error: got %v, want ErrQueueDisabled", err)
	}
	after := readProductMutationState(t, database, productID)
	if after != before {
		t.Fatalf("disabled join mutated storage: before=%+v after=%+v", before, after)
	}
}

func TestIntegrationExactDeadlinesReplayAndEqualityExpiry(t *testing.T) {
	database := openIntegrationDatabase(t)
	repository := NewQueueAttemptRepository(database, 10*time.Minute, 5*time.Minute, 100)
	productID := mustProductID(t, integrationProductOne)
	resetIntegrationProduct(t, database, productID, 1)

	direct, err := repository.Join(context.Background(), joinCommand(productID, 2200, "direct-deadline"))
	if err != nil {
		t.Fatal(err)
	}
	assertPersistedDeadlineInterval(t, database, direct.Attempt.ID, "invitation_deadline-invited_at", 10*time.Minute)
	assertPersistedDeadlineInterval(t, database, direct.Attempt.ID, "checkout_deadline-checkout_started_at", 5*time.Minute)
	before := readAttemptDeadlines(t, database, direct.Attempt.ID)
	if _, err := repository.Join(context.Background(), joinCommand(productID, 2200, "direct-deadline")); err != nil {
		t.Fatal(err)
	}
	if replay := readAttemptDeadlines(t, database, direct.Attempt.ID); replay != before {
		t.Fatalf("join replay extended deadlines: before=%+v after=%+v", before, replay)
	}

	waiter, err := repository.Join(context.Background(), joinCommand(productID, 2201, "promoted-deadline"))
	if err != nil || waiter.Attempt.State != domain.QueueAttemptWaiting {
		t.Fatalf("prepare waiter: %+v, %v", waiter, err)
	}
	if _, err := repository.Cancel(context.Background(), domain.CancelQueueCommand{ProductID: productID, ExternalUserID: "user-2200"}); err != nil {
		t.Fatal(err)
	}
	assertPersistedDeadlineInterval(t, database, waiter.Attempt.ID, "invitation_deadline-invited_at", 10*time.Minute)
	checkout, err := repository.StartCheckout(context.Background(), domain.StartCheckoutCommand{AttemptID: waiter.Attempt.ID, ExternalUserID: "user-2201"})
	if err != nil {
		t.Fatal(err)
	}
	assertPersistedDeadlineInterval(t, database, waiter.Attempt.ID, "checkout_deadline-checkout_started_at", 5*time.Minute)
	checkoutBefore := readAttemptDeadlines(t, database, waiter.Attempt.ID)
	replayed, err := repository.StartCheckout(context.Background(), domain.StartCheckoutCommand{AttemptID: waiter.Attempt.ID, ExternalUserID: "user-2201"})
	if err != nil || !replayed.CheckoutDeadline.Equal(*checkout.CheckoutDeadline) {
		t.Fatalf("checkout replay: %+v, %v", replayed, err)
	}
	if checkoutAfter := readAttemptDeadlines(t, database, waiter.Attempt.ID); checkoutAfter != checkoutBefore {
		t.Fatalf("checkout replay extended deadlines: before=%+v after=%+v", checkoutBefore, checkoutAfter)
	}

	if _, err := database.Exec(`UPDATE queue_attempts SET checkout_deadline=clock_timestamp()-interval '1 microsecond' WHERE id=$1`, uuid.UUID(waiter.Attempt.ID)); err != nil {
		t.Fatal(err)
	}
	if _, err := repository.StartCheckout(context.Background(), domain.StartCheckoutCommand{AttemptID: waiter.Attempt.ID, ExternalUserID: "user-2201"}); !errors.Is(err, domain.ErrAttemptGone) {
		t.Fatalf("past checkout deadline was not expired through the external path: %v", err)
	}
	assertAttemptState(t, database, waiter.Attempt.ID, domain.QueueAttemptCheckoutExpired)
	assertReservedMatchesAttempts(t, database, productID)
}

func TestIntegrationReconciliationExpiresDeadlineEqualToCapturedNow(t *testing.T) {
	database := openIntegrationDatabase(t)
	repository := NewQueueAttemptRepository(database, 10*time.Minute, 5*time.Minute, 100)
	productID := mustProductID(t, integrationProductOne)
	resetIntegrationProduct(t, database, productID, 1)
	attempt := mustJoinPaymentAttempt(t, repository, productID, 2210, "equal-deadline")

	tx, err := database.BeginTx(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback() }()
	product, err := lockProduct(context.Background(), tx, productID)
	if err != nil {
		t.Fatal(err)
	}
	var reconciliationNow time.Time
	if err := tx.QueryRow(`SELECT clock_timestamp()`).Scan(&reconciliationNow); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(`UPDATE queue_attempts SET checkout_deadline=$1 WHERE id=$2`, reconciliationNow, uuid.UUID(attempt.ID)); err != nil {
		t.Fatal(err)
	}
	attempts, err := lockProductAttempts(context.Background(), tx, productID, attemptLockScope{})
	if err != nil {
		t.Fatal(err)
	}
	state := &transactionState{tx: tx, product: product, now: reconciliationNow, attempts: attempts}
	transitions, _, err := repository.reconcileLockedProductBounded(context.Background(), state, 1)
	if err != nil {
		t.Fatal(err)
	}
	if transitions != 1 {
		t.Fatalf("deadline equal to reconciliation now produced %d transitions, want 1", transitions)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}

	assertAttemptState(t, database, attempt.ID, domain.QueueAttemptCheckoutExpired)
	assertReservedMatchesAttempts(t, database, productID)
}

func TestIntegrationProductLocalFIFORollbackIsAtomic(t *testing.T) {
	database := openIntegrationDatabase(t)
	repository := NewQueueAttemptRepository(database, 10*time.Minute, 5*time.Minute, 200)
	productA := mustProductID(t, integrationProductOne)
	productB := mustProductID(t, integrationProductTwo)
	resetIntegrationProduct(t, database, productA, 1)
	resetIntegrationProduct(t, database, productB, 1)

	aHolder := mustJoinPaymentAttempt(t, repository, productA, 2300, "a-holder")
	aFirst := mustJoinPaymentAttempt(t, repository, productA, 2301, "a-first")
	aSecond := mustJoinPaymentAttempt(t, repository, productA, 2302, "a-second")
	bHolder := mustJoinPaymentAttempt(t, repository, productB, 2310, "b-holder")
	bFirst := mustJoinPaymentAttempt(t, repository, productB, 2311, "b-first")
	bSecond := mustJoinPaymentAttempt(t, repository, productB, 2312, "b-second")

	if _, err := database.Exec(`
		CREATE OR REPLACE FUNCTION phase5_fail_first_a_promotion() RETURNS trigger LANGUAGE plpgsql AS $$
		BEGIN
			IF NEW.attempt_id = '` + uuid.UUID(aFirst.ID).String() + `'::uuid THEN
				RAISE EXCEPTION 'forced promotion rollback';
			END IF;
			RETURN NEW;
		END $$;
		CREATE TRIGGER phase5_fail_first_a_promotion BEFORE INSERT ON notification_outbox
		FOR EACH ROW EXECUTE FUNCTION phase5_fail_first_a_promotion();`); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = database.Exec(`DROP TRIGGER IF EXISTS phase5_fail_first_a_promotion ON notification_outbox; DROP FUNCTION IF EXISTS phase5_fail_first_a_promotion()`)
	})

	if _, err := repository.Cancel(context.Background(), domain.CancelQueueCommand{ProductID: productA, ExternalUserID: "user-2300"}); err == nil {
		t.Fatal("forced promotion rollback unexpectedly committed")
	}
	assertAttemptState(t, database, aHolder.ID, domain.QueueAttemptCheckout)
	assertAttemptState(t, database, aFirst.ID, domain.QueueAttemptWaiting)
	assertAttemptState(t, database, aSecond.ID, domain.QueueAttemptWaiting)
	assertAttemptState(t, database, bHolder.ID, domain.QueueAttemptCheckout)
	assertAttemptState(t, database, bFirst.ID, domain.QueueAttemptWaiting)
	assertAttemptState(t, database, bSecond.ID, domain.QueueAttemptWaiting)
	assertProductCounters(t, database, productA, 1, 4)
	assertOutboxCount(t, database, aFirst.ID, 0)

	if _, err := database.Exec(`DROP TRIGGER phase5_fail_first_a_promotion ON notification_outbox`); err != nil {
		t.Fatal(err)
	}
	if _, err := repository.Cancel(context.Background(), domain.CancelQueueCommand{ProductID: productA, ExternalUserID: "user-2300"}); err != nil {
		t.Fatal(err)
	}
	assertAttemptState(t, database, aFirst.ID, domain.QueueAttemptInvited)
	assertAttemptState(t, database, aSecond.ID, domain.QueueAttemptWaiting)
	assertOutboxCount(t, database, aFirst.ID, 1)
	assertAttemptState(t, database, bFirst.ID, domain.QueueAttemptWaiting)

	if _, err := repository.Cancel(context.Background(), domain.CancelQueueCommand{ProductID: productB, ExternalUserID: "user-2310"}); err != nil {
		t.Fatal(err)
	}
	assertAttemptState(t, database, bFirst.ID, domain.QueueAttemptInvited)
	assertAttemptState(t, database, bSecond.ID, domain.QueueAttemptWaiting)
	assertReservedMatchesAttempts(t, database, productA)
	assertReservedMatchesAttempts(t, database, productB)
}

func TestIntegrationDirectAndPromotedPaymentAccounting(t *testing.T) {
	for _, path := range []string{"direct", "promoted"} {
		for _, outcome := range []domain.PaymentOutcome{domain.PaymentSucceeded, domain.PaymentFailed} {
			t.Run(path+"/"+string(outcome), func(t *testing.T) {
				database := openIntegrationDatabase(t)
				repository := NewQueueAttemptRepository(database, 10*time.Minute, 5*time.Minute, 300)
				productID := mustProductID(t, integrationProductOne)
				resetIntegrationProduct(t, database, productID, 1)

				var target domain.QueueAttempt
				if path == "direct" {
					target = mustJoinPaymentAttempt(t, repository, productID, 2400, "target")
				} else {
					holder := mustJoinPaymentAttempt(t, repository, productID, 2499, "holder")
					target = mustJoinPaymentAttempt(t, repository, productID, 2400, "target")
					if _, err := repository.Cancel(context.Background(), domain.CancelQueueCommand{ProductID: productID, ExternalUserID: holder.ExternalUserID}); err != nil {
						t.Fatal(err)
					}
					started, err := repository.StartCheckout(context.Background(), domain.StartCheckoutCommand{AttemptID: target.ID, ExternalUserID: target.ExternalUserID})
					if err != nil {
						t.Fatal(err)
					}
					target = started
				}
				follower := mustJoinPaymentAttempt(t, repository, productID, 2401, "follower")
				result, err := repository.ProcessPayment(context.Background(), mustPaymentCommand(
					t, "accounting-"+path, "event-"+string(outcome), uuid.UUID(target.ID).String(), string(outcome), "reference-"+path,
				))
				if err != nil || result.Code != "accepted" {
					t.Fatalf("payment result: %+v, %v", result, err)
				}

				var stock, reserved int32
				if err := database.QueryRow(`SELECT allocatable_stock,reserved FROM products WHERE id=$1`, uuid.UUID(productID)).Scan(&stock, &reserved); err != nil {
					t.Fatal(err)
				}
				if outcome == domain.PaymentSucceeded {
					if stock != 0 || reserved != 0 {
						t.Fatalf("successful accounting stock=%d reserved=%d", stock, reserved)
					}
					assertAttemptState(t, database, target.ID, domain.QueueAttemptPurchased)
					assertAttemptState(t, database, follower.ID, domain.QueueAttemptSoldOut)
					if _, err := repository.Join(context.Background(), domain.JoinQueueCommand{ProductID: productID, ExternalUserID: target.ExternalUserID, IdempotencyKey: "after-purchase"}); !errors.Is(err, domain.ErrAlreadyPurchased) {
						t.Fatalf("one-purchase rule: %v", err)
					}
				} else {
					if stock != 1 || reserved != 1 {
						t.Fatalf("failed accounting stock=%d reserved=%d", stock, reserved)
					}
					assertAttemptState(t, database, target.ID, domain.QueueAttemptPaymentFailed)
					assertAttemptState(t, database, follower.ID, domain.QueueAttemptInvited)
					if _, err := repository.Join(context.Background(), domain.JoinQueueCommand{ProductID: productID, ExternalUserID: target.ExternalUserID, IdempotencyKey: "after-failure"}); err != nil {
						t.Fatalf("failed buyer could not rejoin: %v", err)
					}
				}
				assertReservedMatchesAttempts(t, database, productID)
			})
		}
	}
}

func TestIntegrationTargetedTransitionRaces(t *testing.T) {
	testCases := []struct {
		name             string
		run              func(*testing.T, *sql.DB, *QueueAttemptRepository, domain.ProductID, string) domain.AttemptID
		state            domain.QueueAttemptState
		expectedStock    int32
		expectedReserved int32
	}{
		{
			name: "invite checkout versus cancel",
			run: func(t *testing.T, database *sql.DB, repository *QueueAttemptRepository, productID domain.ProductID, applicationName string) domain.AttemptID {
				attempt := preparePromotedInvitation(t, database, repository, productID, 2500)
				runOverlappingProductOperations(t, database, repository, productID, applicationName,
					func() error {
						_, err := repository.StartCheckout(context.Background(), domain.StartCheckoutCommand{AttemptID: attempt.ID, ExternalUserID: attempt.ExternalUserID})
						if errors.Is(err, domain.ErrInvalidTransition) {
							return nil
						}
						return err
					},
					func() error {
						_, err := repository.Cancel(context.Background(), domain.CancelQueueCommand{ProductID: productID, ExternalUserID: attempt.ExternalUserID})
						return err
					},
				)
				return attempt.ID
			},
			state: domain.QueueAttemptCancelled, expectedStock: 1, expectedReserved: 0,
		},
		{
			name: "checkout start versus invitation expiry",
			run: func(t *testing.T, database *sql.DB, repository *QueueAttemptRepository, productID domain.ProductID, applicationName string) domain.AttemptID {
				attempt := preparePromotedInvitation(t, database, repository, productID, 2510)
				if _, err := database.Exec(`UPDATE queue_attempts SET invitation_deadline=clock_timestamp() WHERE id=$1`, uuid.UUID(attempt.ID)); err != nil {
					t.Fatal(err)
				}
				runOverlappingProductOperationAndReconciler(t, database, repository, productID, applicationName,
					func() error {
						_, err := repository.StartCheckout(context.Background(), domain.StartCheckoutCommand{AttemptID: attempt.ID, ExternalUserID: attempt.ExternalUserID})
						if errors.Is(err, domain.ErrAttemptGone) {
							return nil
						}
						return err
					},
				)
				return attempt.ID
			},
			state: domain.QueueAttemptInviteExpired, expectedStock: 1, expectedReserved: 0,
		},
		{
			name: "payment success versus checkout expiry",
			run: func(t *testing.T, database *sql.DB, repository *QueueAttemptRepository, productID domain.ProductID, applicationName string) domain.AttemptID {
				attempt := mustJoinPaymentAttempt(t, repository, productID, 2520, "payment-expiry")
				makeAttemptDue(t, database, attempt.ID, domain.QueueAttemptCheckout)
				runOverlappingProductOperationAndReconciler(t, database, repository, productID, applicationName,
					func() error {
						_, err := repository.ProcessPayment(context.Background(), mustPaymentCommand(t, "race", "payment-expiry", uuid.UUID(attempt.ID).String(), "succeeded", "payment-expiry"))
						return err
					},
				)
				return attempt.ID
			},
			state: domain.QueueAttemptCheckoutExpired, expectedStock: 1, expectedReserved: 0,
		},
		{
			name: "duplicate checkout starts",
			run: func(t *testing.T, database *sql.DB, repository *QueueAttemptRepository, productID domain.ProductID, applicationName string) domain.AttemptID {
				attempt := preparePromotedInvitation(t, database, repository, productID, 2530)
				startCheckout := func() error {
					_, err := repository.StartCheckout(context.Background(), domain.StartCheckoutCommand{AttemptID: attempt.ID, ExternalUserID: attempt.ExternalUserID})
					return err
				}
				runOverlappingProductOperations(t, database, repository, productID, applicationName, startCheckout, startCheckout)
				assertPersistedDeadlineInterval(t, database, attempt.ID, "checkout_deadline-checkout_started_at", 5*time.Minute)
				assertOutboxCount(t, database, attempt.ID, 1)
				return attempt.ID
			},
			state: domain.QueueAttemptCheckout, expectedStock: 1, expectedReserved: 1,
		},
		{
			name: "two reconciliation triggers",
			run: func(t *testing.T, database *sql.DB, repository *QueueAttemptRepository, productID domain.ProductID, _ string) domain.AttemptID {
				attempt := mustJoinPaymentAttempt(t, repository, productID, 2540, "two-reconciles")
				makeAttemptDue(t, database, attempt.ID, domain.QueueAttemptCheckout)
				runOverlappingReconcilers(t, repository, productID)
				return attempt.ID
			},
			state: domain.QueueAttemptCheckoutExpired, expectedStock: 1, expectedReserved: 0,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			database := openIntegrationDatabase(t)
			applicationName := newLockCompetitorApplicationName()
			competitorDatabase := openIntegrationDatabaseWithApplicationName(t, applicationName)
			repository := NewQueueAttemptRepository(competitorDatabase, 10*time.Minute, 5*time.Minute, 100)
			productID := mustProductID(t, integrationProductOne)
			resetIntegrationProduct(t, database, productID, 1)
			attemptID := testCase.run(t, database, repository, productID, applicationName)
			assertAttemptState(t, database, attemptID, testCase.state)
			assertReservedMatchesAttempts(t, database, productID)
			assertProductStockAndReserved(t, database, productID, testCase.expectedStock, testCase.expectedReserved)
			assertNoDuplicateTransitionOutbox(t, database, attemptID)
		})
	}
}

func TestIntegrationReconciliationAndOutboxRollbackThenRestartCatchUp(t *testing.T) {
	database := openIntegrationDatabase(t)
	repository := NewQueueAttemptRepository(database, 10*time.Minute, 5*time.Minute, 100)
	productID := mustProductID(t, integrationProductOne)
	resetIntegrationProduct(t, database, productID, 0)
	insertWaitingAttempt(t, database, productID, 1, "rollback-reconcile")
	if _, err := database.Exec(`UPDATE products SET allocatable_stock=1 WHERE id=$1`, uuid.UUID(productID)); err != nil {
		t.Fatal(err)
	}
	if _, err := database.Exec(`
		CREATE OR REPLACE FUNCTION phase5_fail_reconcile_outbox() RETURNS trigger LANGUAGE plpgsql AS $$
		BEGIN RAISE EXCEPTION 'forced reconciliation outbox failure'; END $$;
		CREATE TRIGGER phase5_fail_reconcile_outbox BEFORE INSERT ON notification_outbox
		FOR EACH ROW EXECUTE FUNCTION phase5_fail_reconcile_outbox();`); err != nil {
		t.Fatal(err)
	}
	triggerPresent := true
	t.Cleanup(func() {
		if triggerPresent {
			_, _ = database.Exec(`DROP TRIGGER IF EXISTS phase5_fail_reconcile_outbox ON notification_outbox; DROP FUNCTION IF EXISTS phase5_fail_reconcile_outbox()`)
		}
	})
	if _, err := repository.ReconcileNextProduct(context.Background(), 10, nil); err == nil {
		t.Fatal("forced reconciliation failure unexpectedly committed")
	}
	assertAttemptStateByUser(t, database, productID, "rollback-reconcile", domain.QueueAttemptWaiting)
	assertProductCounters(t, database, productID, 0, 2)
	var outboxCount int
	if err := database.QueryRow(`
		SELECT count(*) FROM notification_outbox o
		JOIN queue_attempts a ON a.id=o.attempt_id WHERE a.product_id=$1`, uuid.UUID(productID)).Scan(&outboxCount); err != nil || outboxCount != 0 {
		t.Fatalf("failed reconciliation retained outbox rows: count=%d err=%v", outboxCount, err)
	}
	if _, err := database.Exec(`DROP TRIGGER phase5_fail_reconcile_outbox ON notification_outbox; DROP FUNCTION phase5_fail_reconcile_outbox()`); err != nil {
		t.Fatal(err)
	}
	triggerPresent = false

	if _, err := repository.ReconcileNextProduct(context.Background(), 10, nil); err != nil {
		t.Fatal(err)
	}
	clearNotificationOutbox(t, database)
	seedRestartOutboxRows(t, database)
	publisher := &restartPublisher{published: make(chan uuid.UUID, 3)}
	freshOutbox := NewNotificationOutboxRepository(database)
	finalized := make(chan uuid.UUID, 3)
	freshOutbox.afterFinalized = func(id uuid.UUID) { finalized <- id }
	supervisor := queueworker.NewSupervisor(queueworker.Config{
		Interval: time.Hour, ReconciliationBatchSize: 10, MaxReconciledProducts: 1,
		MaxOutboxItems: 3, OutboxLeaseDuration: time.Minute, OutboxRetryBase: time.Second,
		OutboxRetryMax: time.Minute, PublisherTimeout: time.Second,
	}, NewQueueAttemptRepository(database, 10*time.Minute, 5*time.Minute, 100), freshOutbox, publisher, queueworker.NoopObserver{}, zap.NewNop())
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { supervisor.Run(ctx); close(done) }()
	seen := make(map[uuid.UUID]struct{}, 3)
	for len(seen) < 3 {
		select {
		case id := <-finalized:
			seen[id] = struct{}{}
		case <-time.After(3 * time.Second):
			cancel()
			t.Fatal("fresh supervisor did not catch up restart rows")
		}
	}
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("fresh supervisor did not stop")
	}
	if publisher.duplicate {
		t.Fatal("restart catch-up published a row more than once")
	}
	var sent, total int
	if err := database.QueryRow(`SELECT count(*) FILTER (WHERE status='sent'),count(*) FROM notification_outbox`).Scan(&sent, &total); err != nil {
		t.Fatal(err)
	}
	if sent != 3 || total != 3 {
		t.Fatalf("restart outbox states sent=%d total=%d", sent, total)
	}
}

func TestIntegrationSeededRandomizedQueueInvariants(t *testing.T) {
	const seed int64 = 0x5eed2026
	database := openIntegrationDatabase(t)
	repository := NewQueueAttemptRepository(database, 10*time.Minute, 5*time.Minute, 200)
	products := []domain.ProductID{mustProductID(t, integrationProductOne), mustProductID(t, integrationProductTwo)}
	for _, productID := range products {
		resetIntegrationProduct(t, database, productID, 2)
	}
	random := rand.New(rand.NewSource(seed)) //nolint:gosec // Reproducibility, not security.
	terminalVersions := make(map[domain.AttemptID]int64)
	coverage := randomizedCoverage{}
	step := runInvariantCoveragePrelude(t, database, repository, products, terminalVersions, seed, &coverage)

	for randomStep := 0; randomStep < 36; randomStep++ {
		productID := products[random.Intn(len(products))]
		userNumber := 2700 + random.Intn(8)
		operation := randomStep % 6
		switch operation {
		case 0:
			result, err := repository.Join(context.Background(), joinCommand(productID, userNumber, fmt.Sprintf("seed-%d-step-%d", seed, step)))
			assertAllowedOperationError(t, seed, step, "join", err, domain.ErrQueueFull, domain.ErrAlreadyPurchased, domain.ErrOutOfStock)
			if err == nil {
				coverage.joins++
				coverage.recordAdmission(result.Attempt.State)
			}
		case 1:
			attempt := latestActiveAttempt(t, database, productID, fmt.Sprintf("user-%d", userNumber))
			command := domain.StartCheckoutCommand{AttemptID: domain.AttemptID(uuid.New()), ExternalUserID: domain.ExternalUserID(fmt.Sprintf("user-%d", userNumber))}
			allowed := []error{domain.ErrAttemptNotFound}
			if attempt != nil {
				command = domain.StartCheckoutCommand{AttemptID: attempt.ID, ExternalUserID: attempt.ExternalUserID}
				allowed = []error{domain.ErrInvalidTransition, domain.ErrAttemptGone}
			}
			_, err := repository.StartCheckout(context.Background(), command)
			assertAllowedOperationError(t, seed, step, "start checkout", err, allowed...)
			if err == nil {
				coverage.checkouts++
			}
		case 2:
			_, err := repository.Cancel(context.Background(), domain.CancelQueueCommand{ProductID: productID, ExternalUserID: domain.ExternalUserID(fmt.Sprintf("user-%d", userNumber))})
			assertAllowedOperationError(t, seed, step, "cancel", err, domain.ErrAttemptNotFound, domain.ErrAlreadyPurchased)
			if err == nil {
				coverage.cancels++
			}
		case 3:
			attempt := latestAttemptForProduct(t, database, productID)
			attemptID := domain.AttemptID(uuid.New())
			stateBefore := domain.QueueAttemptState("")
			if attempt != nil {
				attemptID = attempt.ID
				stateBefore = attempt.State
			}
			outcome := domain.PaymentFailed
			reference := ""
			if random.Intn(2) == 0 {
				outcome = domain.PaymentSucceeded
				reference = fmt.Sprintf("seed-%d-reference-%d", seed, step)
			}
			_, err := repository.ProcessPayment(context.Background(), mustPaymentCommand(t, "seeded", fmt.Sprintf("event-%d", step), uuid.UUID(attemptID).String(), string(outcome), reference))
			assertAllowedOperationError(t, seed, step, "payment", err)
			if stateBefore == domain.QueueAttemptCheckout {
				if outcome == domain.PaymentSucceeded {
					coverage.successfulPayments++
				} else {
					coverage.failedPayments++
				}
			}
		case 4:
			forceOldestActiveDue(t, database, productID)
			result, err := repository.ReconcileNextProduct(context.Background(), 20, otherProductIDs(products, productID))
			assertAllowedOperationError(t, seed, step, "reconcile", err)
			coverage.reconciliations++
			if result.Transitions > 0 {
				coverage.expiryTransitions += result.Transitions
			}
		case 5:
			delta := int32(1)
			if random.Intn(2) == 0 {
				delta = -1
			}
			_, err := repository.AdjustStock(context.Background(), domain.StockAdjustmentCommand{
				ProductID: productID, IdempotencyKey: domain.IdempotencyKey(fmt.Sprintf("seed-adjust-%d", step)), Delta: delta, Reason: "seeded invariant operation",
			})
			assertAllowedOperationError(t, seed, step, "stock adjustment", err, domain.ErrStockBelowZero, domain.ErrStockBelowReserved)
			if err == nil {
				coverage.stockAdjustments++
			} else {
				coverage.stockRejections++
			}
		}
		assertSeededInvariants(t, database, products, terminalVersions, seed, step)
		step++
	}
	coverage.assertComplete(t, seed)
}

type randomizedCoverage struct {
	joins              int
	directAdmissions   int
	waitingAdmissions  int
	checkouts          int
	cancels            int
	successfulPayments int
	failedPayments     int
	reconciliations    int
	expiryTransitions  int
	stockAdjustments   int
	stockRejections    int
	terminalReplays    int
}

func (coverage *randomizedCoverage) recordAdmission(state domain.QueueAttemptState) {
	switch state {
	case domain.QueueAttemptCheckout:
		coverage.directAdmissions++
	case domain.QueueAttemptWaiting:
		coverage.waitingAdmissions++
	}
}

func (coverage randomizedCoverage) assertComplete(t *testing.T, seed int64) {
	t.Helper()
	if coverage.joins == 0 || coverage.directAdmissions == 0 || coverage.waitingAdmissions == 0 ||
		coverage.checkouts == 0 || coverage.cancels == 0 || coverage.successfulPayments == 0 ||
		coverage.failedPayments == 0 || coverage.reconciliations == 0 || coverage.expiryTransitions == 0 ||
		coverage.stockAdjustments == 0 || coverage.stockRejections == 0 || coverage.terminalReplays == 0 {
		t.Fatalf("seed=%d did not exercise every required behavior: %+v", seed, coverage)
	}
}

func runInvariantCoveragePrelude(
	t *testing.T,
	database *sql.DB,
	repository *QueueAttemptRepository,
	products []domain.ProductID,
	terminalVersions map[domain.AttemptID]int64,
	seed int64,
	coverage *randomizedCoverage,
) int {
	t.Helper()
	productID := products[0]
	step := 0
	check := func() {
		assertSeededInvariants(t, database, products, terminalVersions, seed, step)
		step++
	}

	directACommand := joinCommand(productID, 2600, "prelude-direct-a")
	directA, err := repository.Join(context.Background(), directACommand)
	assertAllowedOperationError(t, seed, step, "prelude direct join A", err)
	coverage.joins++
	coverage.recordAdmission(directA.Attempt.State)
	check()

	directBCommand := joinCommand(productID, 2601, "prelude-direct-b")
	directB, err := repository.Join(context.Background(), directBCommand)
	assertAllowedOperationError(t, seed, step, "prelude direct join B", err)
	coverage.joins++
	coverage.recordAdmission(directB.Attempt.State)
	check()

	waiter, err := repository.Join(context.Background(), joinCommand(productID, 2602, "prelude-waiter"))
	assertAllowedOperationError(t, seed, step, "prelude waiting join", err)
	coverage.joins++
	coverage.recordAdmission(waiter.Attempt.State)
	check()

	_, err = repository.Cancel(context.Background(), domain.CancelQueueCommand{ProductID: productID, ExternalUserID: directA.Attempt.ExternalUserID})
	assertAllowedOperationError(t, seed, step, "prelude cancel", err)
	coverage.cancels++
	check()

	promoted := latestActiveAttempt(t, database, productID, string(waiter.Attempt.ExternalUserID))
	if promoted == nil || promoted.State != domain.QueueAttemptInvited {
		t.Fatalf("seed=%d step=%d: prelude waiter was not promoted: %+v", seed, step, promoted)
	}
	checkedOut, err := repository.StartCheckout(context.Background(), domain.StartCheckoutCommand{AttemptID: promoted.ID, ExternalUserID: promoted.ExternalUserID})
	assertAllowedOperationError(t, seed, step, "prelude checkout", err)
	coverage.checkouts++
	check()

	failedPayment, err := repository.ProcessPayment(context.Background(), mustPaymentCommand(t, "seeded-prelude", "failed", uuid.UUID(checkedOut.ID).String(), "failed", ""))
	assertAllowedOperationError(t, seed, step, "prelude failed payment", err)
	if failedPayment.Code != "accepted" {
		t.Fatalf("seed=%d step=%d: prelude failed payment result %+v", seed, step, failedPayment)
	}
	coverage.failedPayments++
	check()

	expiring, err := repository.Join(context.Background(), joinCommand(productID, 2603, "prelude-expiring"))
	assertAllowedOperationError(t, seed, step, "prelude expiring join", err)
	coverage.joins++
	coverage.recordAdmission(expiring.Attempt.State)
	check()

	succeededPayment, err := repository.ProcessPayment(context.Background(), mustPaymentCommand(t, "seeded-prelude", "succeeded", uuid.UUID(directB.Attempt.ID).String(), "succeeded", "prelude-reference"))
	assertAllowedOperationError(t, seed, step, "prelude successful payment", err)
	if succeededPayment.Code != "accepted" {
		t.Fatalf("seed=%d step=%d: prelude successful payment result %+v", seed, step, succeededPayment)
	}
	coverage.successfulPayments++
	check()

	replay, err := repository.Join(context.Background(), directBCommand)
	assertAllowedOperationError(t, seed, step, "prelude terminal replay", err)
	if replay.Created || replay.Attempt.ID != directB.Attempt.ID || replay.Attempt.State != domain.QueueAttemptPurchased {
		t.Fatalf("seed=%d step=%d: terminal replay returned %+v", seed, step, replay)
	}
	coverage.terminalReplays++
	check()

	makeAttemptDue(t, database, expiring.Attempt.ID, domain.QueueAttemptCheckout)
	reconciled, err := repository.ReconcileNextProduct(context.Background(), 20, otherProductIDs(products, productID))
	assertAllowedOperationError(t, seed, step, "prelude expiry reconciliation", err)
	if reconciled.Transitions == 0 {
		t.Fatalf("seed=%d step=%d: prelude reconciliation performed no expiry", seed, step)
	}
	coverage.reconciliations++
	coverage.expiryTransitions += reconciled.Transitions
	check()

	_, err = repository.AdjustStock(context.Background(), domain.StockAdjustmentCommand{
		ProductID: productID, IdempotencyKey: "prelude-stock-success", Delta: 1, Reason: "prelude stock increase",
	})
	assertAllowedOperationError(t, seed, step, "prelude stock adjustment", err)
	coverage.stockAdjustments++
	check()

	_, err = repository.AdjustStock(context.Background(), domain.StockAdjustmentCommand{
		ProductID: productID, IdempotencyKey: "prelude-stock-rejection", Delta: -3, Reason: "prelude rejected stock decrease",
	})
	assertAllowedOperationError(t, seed, step, "prelude stock rejection", err, domain.ErrStockBelowZero)
	if err == nil {
		t.Fatalf("seed=%d step=%d: prelude stock rejection unexpectedly succeeded", seed, step)
	}
	coverage.stockRejections++
	check()
	return step
}

func assertAllowedOperationError(t *testing.T, seed int64, step int, operation string, err error, allowed ...error) {
	t.Helper()
	if err == nil {
		return
	}
	for _, allowedError := range allowed {
		if errors.Is(err, allowedError) {
			return
		}
	}
	t.Fatalf("seed=%d step=%d operation=%s returned unexpected error: %v", seed, step, operation, err)
}

type productMutationState struct {
	title                string
	description          string
	imageURL             string
	queueEnabled         bool
	allocatableStock     int32
	rightTTLSeconds      int32
	createdAt            time.Time
	updatedAt            time.Time
	reserved             int32
	nextSequence         int64
	attempts             int
	outbox               int
	inventoryAdjustments int
	paymentEvents        int
}

func readProductMutationState(t *testing.T, database *sql.DB, productID domain.ProductID) productMutationState {
	t.Helper()
	var state productMutationState
	if err := database.QueryRow(`
		SELECT p.title,p.description,p.image_url,p.queue_enabled,p.allocatable_stock,p.right_ttl_seconds,
		       p.created_at,p.updated_at,p.reserved,p.next_queue_sequence,
		       (SELECT count(*) FROM queue_attempts WHERE product_id=p.id),
		       (SELECT count(*) FROM notification_outbox o JOIN queue_attempts a ON a.id=o.attempt_id WHERE a.product_id=p.id),
		       (SELECT count(*) FROM inventory_adjustments WHERE product_id=p.id),
		       (SELECT count(*) FROM payment_inbox i JOIN queue_attempts a ON a.id=i.attempt_id WHERE a.product_id=p.id)
		FROM products p WHERE p.id=$1`, uuid.UUID(productID)).Scan(
		&state.title, &state.description, &state.imageURL, &state.queueEnabled, &state.allocatableStock,
		&state.rightTTLSeconds, &state.createdAt, &state.updatedAt, &state.reserved, &state.nextSequence,
		&state.attempts, &state.outbox, &state.inventoryAdjustments, &state.paymentEvents,
	); err != nil {
		t.Fatal(err)
	}
	return state
}

type attemptDeadlines struct {
	invitedAt, invitationDeadline, checkoutStartedAt, checkoutDeadline sql.NullTime
}

func readAttemptDeadlines(t *testing.T, database *sql.DB, attemptID domain.AttemptID) attemptDeadlines {
	t.Helper()
	var deadlines attemptDeadlines
	if err := database.QueryRow(`SELECT invited_at,invitation_deadline,checkout_started_at,checkout_deadline FROM queue_attempts WHERE id=$1`, uuid.UUID(attemptID)).Scan(
		&deadlines.invitedAt, &deadlines.invitationDeadline, &deadlines.checkoutStartedAt, &deadlines.checkoutDeadline,
	); err != nil {
		t.Fatal(err)
	}
	return deadlines
}

func assertPersistedDeadlineInterval(t *testing.T, database *sql.DB, attemptID domain.AttemptID, expression string, want time.Duration) {
	t.Helper()
	var seconds float64
	query := `SELECT extract(epoch FROM (` + expression + `)) FROM queue_attempts WHERE id=$1`
	if err := database.QueryRow(query, uuid.UUID(attemptID)).Scan(&seconds); err != nil {
		t.Fatal(err)
	}
	if seconds != want.Seconds() {
		t.Fatalf("persisted %s: got %.6fs, want %.6fs", expression, seconds, want.Seconds())
	}
}

func assertProductCounters(t *testing.T, database *sql.DB, productID domain.ProductID, reserved int32, nextSequence int64) {
	t.Helper()
	var gotReserved int32
	var gotNext int64
	if err := database.QueryRow(`SELECT reserved,next_queue_sequence FROM products WHERE id=$1`, uuid.UUID(productID)).Scan(&gotReserved, &gotNext); err != nil {
		t.Fatal(err)
	}
	if gotReserved != reserved || gotNext != nextSequence {
		t.Fatalf("product counters: reserved=%d next=%d, want reserved=%d next=%d", gotReserved, gotNext, reserved, nextSequence)
	}
}

func assertProductStockAndReserved(t *testing.T, database *sql.DB, productID domain.ProductID, wantStock, wantReserved int32) {
	t.Helper()
	var stock, reserved int32
	if err := database.QueryRow(`SELECT allocatable_stock,reserved FROM products WHERE id=$1`, uuid.UUID(productID)).Scan(&stock, &reserved); err != nil {
		t.Fatal(err)
	}
	if stock != wantStock || reserved != wantReserved {
		t.Fatalf("product accounting: stock=%d reserved=%d, want stock=%d reserved=%d", stock, reserved, wantStock, wantReserved)
	}
}

func assertOutboxCount(t *testing.T, database *sql.DB, attemptID domain.AttemptID, want int) {
	t.Helper()
	var count int
	if err := database.QueryRow(`SELECT count(*) FROM notification_outbox WHERE attempt_id=$1`, uuid.UUID(attemptID)).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != want {
		t.Fatalf("attempt %s outbox count: got %d, want %d", uuid.UUID(attemptID), count, want)
	}
}

type restartPublisher struct {
	mu        sync.Mutex
	published chan uuid.UUID
	seen      map[uuid.UUID]struct{}
	duplicate bool
}

func (publisher *restartPublisher) Publish(_ context.Context, claim domain.NotificationClaim) error {
	publisher.mu.Lock()
	if publisher.seen == nil {
		publisher.seen = make(map[uuid.UUID]struct{})
	}
	if _, exists := publisher.seen[claim.ID]; exists {
		publisher.duplicate = true
	}
	publisher.seen[claim.ID] = struct{}{}
	publisher.mu.Unlock()
	publisher.published <- claim.ID
	return nil
}

func seedRestartOutboxRows(t *testing.T, database *sql.DB) {
	t.Helper()
	statuses := []string{"pending", "failed", "processing"}
	for index, status := range statuses {
		payload := []byte(fmt.Sprintf(`{"restart":%d}`, index))
		digest := sha256.Sum256(payload)
		id := uuid.New()
		if status == "processing" {
			_, err := database.Exec(`
				INSERT INTO notification_outbox(id,event_type,deduplication_key,payload,payload_hash,status,attempt_count,available_at,lease_until,lease_token,lease_generation)
				VALUES($1,'payment.compensation_required',$2,$3,$4,'processing',1,clock_timestamp()-interval '1 minute',clock_timestamp()-interval '1 second',$5,1)`,
				id, "restart:"+id.String(), payload, digest[:], uuid.New())
			if err != nil {
				t.Fatal(err)
			}
			continue
		}
		_, err := database.Exec(`
			INSERT INTO notification_outbox(id,event_type,deduplication_key,payload,payload_hash,status,available_at)
			VALUES($1,'payment.compensation_required',$2,$3,$4,$5,clock_timestamp()-interval '1 second')`,
			id, "restart:"+id.String(), payload, digest[:], status)
		if err != nil {
			t.Fatal(err)
		}
	}
}

func preparePromotedInvitation(
	t *testing.T,
	database *sql.DB,
	repository *QueueAttemptRepository,
	productID domain.ProductID,
	userNumber int,
) domain.QueueAttempt {
	t.Helper()
	if _, err := database.Exec(`UPDATE products SET allocatable_stock=0 WHERE id=$1`, uuid.UUID(productID)); err != nil {
		t.Fatal(err)
	}
	insertWaitingAttempt(t, database, productID, 1, fmt.Sprintf("user-%d", userNumber))
	if _, err := database.Exec(`UPDATE products SET allocatable_stock=1 WHERE id=$1`, uuid.UUID(productID)); err != nil {
		t.Fatal(err)
	}
	if _, err := repository.ReconcileNextProduct(context.Background(), 10, nil); err != nil {
		t.Fatal(err)
	}
	attempt := latestActiveAttempt(t, database, productID, fmt.Sprintf("user-%d", userNumber))
	if attempt == nil || attempt.State != domain.QueueAttemptInvited {
		t.Fatalf("prepare promoted invitation: %+v", attempt)
	}
	return *attempt
}

type databaseProductLockBlocker struct {
	transaction     *sql.Tx
	observer        *sql.Conn
	applicationName string
	releaseOnce     sync.Once
	releaseError    error
}

func newLockCompetitorApplicationName() string {
	return "goodqueue_phase5_lock_" + uuid.NewString()
}

func openIntegrationDatabaseWithApplicationName(t *testing.T, applicationName string) *sql.DB {
	t.Helper()
	databaseURL := os.Getenv("GOODQUEUE_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("GOODQUEUE_TEST_DATABASE_URL is not set")
	}
	config, err := pgx.ParseConfig(databaseURL)
	if err != nil {
		t.Fatalf("parse integration database URL: %v", err)
	}
	if config.RuntimeParams == nil {
		config.RuntimeParams = make(map[string]string)
	}
	config.RuntimeParams["application_name"] = applicationName
	database := stdlib.OpenDB(*config)
	t.Cleanup(func() { _ = database.Close() })
	return database
}

func lockProductForCompetitors(
	t *testing.T,
	database *sql.DB,
	productID domain.ProductID,
	applicationName string,
) *databaseProductLockBlocker {
	t.Helper()
	observer, err := database.Conn(context.Background())
	if err != nil {
		t.Fatalf("open product-lock observer connection: %v", err)
	}
	transaction, err := database.BeginTx(context.Background(), nil)
	if err != nil {
		_ = observer.Close()
		t.Fatalf("begin product-lock blocker transaction: %v", err)
	}
	var lockedProductID uuid.UUID
	if err := transaction.QueryRow(`SELECT id FROM products WHERE id=$1 FOR UPDATE`, uuid.UUID(productID)).Scan(&lockedProductID); err != nil {
		_ = transaction.Rollback()
		_ = observer.Close()
		t.Fatalf("lock product in blocker transaction: %v", err)
	}
	blocker := &databaseProductLockBlocker{
		transaction: transaction, observer: observer, applicationName: applicationName,
	}
	t.Cleanup(func() {
		_ = blocker.transaction.Rollback()
		_ = blocker.observer.Close()
	})
	return blocker
}

func (blocker *databaseProductLockBlocker) waitUntilCompetitorsBlocked(t *testing.T, expected int) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	ticker := time.NewTicker(5 * time.Millisecond)
	defer ticker.Stop()

	for {
		blocked, err := blocker.blockedCompetitorCount(ctx)
		if err != nil {
			t.Fatalf("observe blocked product-lock competitors: %v", err)
		}
		if blocked == expected {
			return
		}
		select {
		case <-ctx.Done():
			t.Fatalf(
				"timed out waiting for %d PostgreSQL product-lock waits; observed %d; activity: %s",
				expected, blocked, blocker.competitorActivityDiagnostic(),
			)
		case <-ticker.C:
		}
	}
}

func (blocker *databaseProductLockBlocker) blockedCompetitorCount(ctx context.Context) (int, error) {
	var count int
	err := blocker.observer.QueryRowContext(ctx, `
		SELECT count(*)
		FROM pg_stat_activity
		WHERE application_name=$1
		  AND backend_type='client backend'
		  AND state='active'
		  AND wait_event_type='Lock'
		  AND cardinality(pg_blocking_pids(pid)) > 0
		  AND query LIKE '%FROM products WHERE id = $1 FOR UPDATE%'`, blocker.applicationName).Scan(&count)
	return count, err
}

func (blocker *databaseProductLockBlocker) competitorActivityDiagnostic() string {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	rows, err := blocker.observer.QueryContext(ctx, `
		SELECT pid, state, COALESCE(wait_event_type,''), COALESCE(wait_event,''),
		       pg_blocking_pids(pid)::text, query
		FROM pg_stat_activity
		WHERE application_name=$1
		ORDER BY pid`, blocker.applicationName)
	if err != nil {
		return fmt.Sprintf("diagnostic query failed: %v", err)
	}
	defer func() { _ = rows.Close() }()
	activity := "none"
	for rows.Next() {
		var pid int
		var state, waitType, waitEvent, blockingPIDs, query string
		if err := rows.Scan(&pid, &state, &waitType, &waitEvent, &blockingPIDs, &query); err != nil {
			return fmt.Sprintf("diagnostic scan failed: %v", err)
		}
		activity += fmt.Sprintf(" [pid=%d state=%s wait=%s/%s blockers=%s query=%q]", pid, state, waitType, waitEvent, blockingPIDs, query)
	}
	if err := rows.Err(); err != nil {
		return fmt.Sprintf("diagnostic rows failed: %v", err)
	}
	return activity
}

func (blocker *databaseProductLockBlocker) release(t *testing.T) {
	t.Helper()
	blocker.releaseOnce.Do(func() { blocker.releaseError = blocker.transaction.Commit() })
	if blocker.releaseError != nil {
		t.Fatalf("release product-lock blocker transaction: %v", blocker.releaseError)
	}
}

func runOverlappingProductOperations(
	t *testing.T,
	database *sql.DB,
	repository *QueueAttemptRepository,
	productID domain.ProductID,
	applicationName string,
	firstOperation func() error,
	secondOperation func() error,
) {
	t.Helper()
	blocker := lockProductForCompetitors(t, database, productID, applicationName)
	firstError := make(chan error, 1)
	go func() { firstError <- firstOperation() }()

	secondError := make(chan error, 1)
	go func() { secondError <- secondOperation() }()
	blocker.waitUntilCompetitorsBlocked(t, 2)
	blocker.release(t)
	if err := <-firstError; err != nil {
		t.Fatalf("first overlapping operation: %v", err)
	}
	if err := <-secondError; err != nil {
		t.Fatalf("second overlapping operation: %v", err)
	}
}

func runOverlappingProductOperationAndReconciler(
	t *testing.T,
	database *sql.DB,
	repository *QueueAttemptRepository,
	productID domain.ProductID,
	applicationName string,
	productOperation func() error,
) {
	t.Helper()
	blocker := lockProductForCompetitors(t, database, productID, applicationName)
	operationError := make(chan error, 1)
	go func() { operationError <- productOperation() }()
	blocker.waitUntilCompetitorsBlocked(t, 1)

	excludedProductIDs := otherKnownProductIDs(t, productID)
	reconcileResult := make(chan domain.ReconciliationResult, 1)
	reconcileError := make(chan error, 1)
	go func() {
		result, err := repository.ReconcileNextProduct(context.Background(), 10, excludedProductIDs)
		reconcileResult <- result
		reconcileError <- err
	}()
	var skipped domain.ReconciliationResult
	select {
	case skipped = <-reconcileResult:
	case <-time.After(time.Second):
		t.Fatal("reconciler blocked instead of skipping the product locked by the competing operation")
	}
	if err := <-reconcileError; err != nil {
		t.Fatalf("overlapping reconciler: %v", err)
	}
	if skipped.ProductID != (domain.ProductID{}) || skipped.Transitions != 0 {
		t.Fatalf("reconciler did not conflict on the only eligible locked product: %+v", skipped)
	}

	blocker.release(t)
	if err := <-operationError; err != nil {
		t.Fatalf("overlapped product operation: %v", err)
	}
}

func otherKnownProductIDs(t *testing.T, selected domain.ProductID) []domain.ProductID {
	t.Helper()
	known := []domain.ProductID{
		mustProductID(t, integrationProductOne),
		mustProductID(t, integrationProductTwo),
		mustProductID(t, "33333333-3333-3333-3333-333333333333"),
	}
	return otherProductIDs(known, selected)
}

func runOverlappingReconcilers(
	t *testing.T,
	repository *QueueAttemptRepository,
	productID domain.ProductID,
) {
	t.Helper()
	firstLocked := make(chan struct{})
	releaseFirst := make(chan struct{})
	var lockOnce sync.Once
	var releaseOnce sync.Once
	repository.afterProductSelected = func(candidate domain.ProductID) {
		if candidate != productID {
			return
		}
		lockOnce.Do(func() {
			close(firstLocked)
			<-releaseFirst
		})
	}
	release := func() { releaseOnce.Do(func() { close(releaseFirst) }) }
	t.Cleanup(release)
	excludedProductIDs := otherKnownProductIDs(t, productID)

	firstResult := make(chan domain.ReconciliationResult, 1)
	firstError := make(chan error, 1)
	go func() {
		result, err := repository.ReconcileNextProduct(context.Background(), 10, excludedProductIDs)
		firstResult <- result
		firstError <- err
	}()
	select {
	case <-firstLocked:
	case <-time.After(time.Second):
		t.Fatal("first reconciler did not select and hold the eligible product")
	}

	secondResult := make(chan domain.ReconciliationResult, 1)
	secondError := make(chan error, 1)
	go func() {
		result, err := repository.ReconcileNextProduct(context.Background(), 10, excludedProductIDs)
		secondResult <- result
		secondError <- err
	}()
	var second domain.ReconciliationResult
	select {
	case second = <-secondResult:
	case <-time.After(time.Second):
		t.Fatal("second reconciler blocked instead of conflicting through SKIP LOCKED")
	}
	if err := <-secondError; err != nil {
		t.Fatalf("second overlapping reconciler: %v", err)
	}
	if second.ProductID != (domain.ProductID{}) || second.Transitions != 0 {
		t.Fatalf("second reconciler processed a product while the only eligible product was locked: %+v", second)
	}

	release()
	first := <-firstResult
	if err := <-firstError; err != nil {
		t.Fatalf("first overlapping reconciler: %v", err)
	}
	if first.ProductID != productID || first.Transitions == 0 {
		t.Fatalf("first reconciler did not process the locked product: %+v", first)
	}
}

func assertNoDuplicateTransitionOutbox(t *testing.T, database *sql.DB, attemptID domain.AttemptID) {
	t.Helper()
	var duplicates int
	if err := database.QueryRow(`
		SELECT count(*) FROM (
			SELECT event_type,deduplication_key FROM notification_outbox WHERE attempt_id=$1
			GROUP BY event_type,deduplication_key HAVING count(*)>1
		) duplicated`, uuid.UUID(attemptID)).Scan(&duplicates); err != nil {
		t.Fatal(err)
	}
	if duplicates != 0 {
		t.Fatalf("attempt %s has duplicate transition outbox rows", uuid.UUID(attemptID))
	}
}

func latestActiveAttempt(t *testing.T, database *sql.DB, productID domain.ProductID, userID string) *domain.QueueAttempt {
	t.Helper()
	row := database.QueryRow(`
		SELECT id,product_id,queue_sequence,external_user_id,idempotency_key,state,created_at,updated_at,
		       invited_at,invitation_deadline,checkout_started_at,checkout_deadline,terminal_at,purchased_at,
		       accepted_payment_provider,accepted_payment_reference,terminal_reason,terminal_message,version
		FROM queue_attempts WHERE product_id=$1 AND external_user_id=$2 AND state IN ('waiting','invited','checkout')
		ORDER BY queue_sequence DESC LIMIT 1`, uuid.UUID(productID), userID)
	attempt, err := scanAttempt(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		t.Fatal(err)
	}
	return &attempt
}

func latestAttemptForProduct(t *testing.T, database *sql.DB, productID domain.ProductID) *domain.QueueAttempt {
	t.Helper()
	row := database.QueryRow(`
		SELECT id,product_id,queue_sequence,external_user_id,idempotency_key,state,created_at,updated_at,
		       invited_at,invitation_deadline,checkout_started_at,checkout_deadline,terminal_at,purchased_at,
		       accepted_payment_provider,accepted_payment_reference,terminal_reason,terminal_message,version
		FROM queue_attempts WHERE product_id=$1 ORDER BY queue_sequence DESC LIMIT 1`, uuid.UUID(productID))
	attempt, err := scanAttempt(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		t.Fatal(err)
	}
	return &attempt
}

func forceOldestActiveDue(t *testing.T, database *sql.DB, productID domain.ProductID) {
	t.Helper()
	if _, err := database.Exec(`
		UPDATE queue_attempts SET
			invitation_deadline=CASE WHEN state='invited' THEN clock_timestamp() ELSE invitation_deadline END,
			checkout_deadline=CASE WHEN state='checkout' THEN clock_timestamp() ELSE checkout_deadline END
		WHERE id=(SELECT id FROM queue_attempts WHERE product_id=$1 AND state IN ('invited','checkout') ORDER BY queue_sequence LIMIT 1)`,
		uuid.UUID(productID)); err != nil {
		t.Fatal(err)
	}
}

func otherProductIDs(products []domain.ProductID, selected domain.ProductID) []domain.ProductID {
	others := make([]domain.ProductID, 0, len(products)-1)
	for _, productID := range products {
		if productID != selected {
			others = append(others, productID)
		}
	}
	return others
}

func assertSeededInvariants(
	t *testing.T,
	database *sql.DB,
	products []domain.ProductID,
	terminalVersions map[domain.AttemptID]int64,
	seed int64,
	step int,
) {
	t.Helper()
	fail := func(format string, arguments ...any) {
		t.Helper()
		t.Fatalf("seed=%d step=%d: "+format, append([]any{seed, step}, arguments...)...)
	}
	for _, productID := range products {
		var validCapacity, reservedMatches, uniqueSequences, monotonicSequence, noFIFOLeapfrog bool
		if err := database.QueryRow(`
			SELECT p.reserved BETWEEN 0 AND p.allocatable_stock,
			       p.reserved=(SELECT count(*) FROM queue_attempts WHERE product_id=p.id AND state IN ('invited','checkout')),
			       (SELECT count(*)=count(DISTINCT queue_sequence) FROM queue_attempts WHERE product_id=p.id),
			       p.next_queue_sequence>COALESCE((SELECT max(queue_sequence) FROM queue_attempts WHERE product_id=p.id),0),
			       NOT EXISTS (
				   SELECT 1 FROM queue_attempts promoted JOIN queue_attempts waiting ON waiting.product_id=promoted.product_id
				   WHERE promoted.product_id=p.id AND promoted.state='invited' AND waiting.state='waiting'
				     AND waiting.queue_sequence<promoted.queue_sequence)
			FROM products p WHERE p.id=$1`, uuid.UUID(productID)).Scan(
			&validCapacity, &reservedMatches, &uniqueSequences, &monotonicSequence, &noFIFOLeapfrog,
		); err != nil {
			fail("query invariants: %v", err)
		}
		if !validCapacity || !reservedMatches || !uniqueSequences || !monotonicSequence || !noFIFOLeapfrog {
			fail("product %s invariants capacity=%t accounting=%t sequences=%t monotonic=%t fifo=%t", uuid.UUID(productID), validCapacity, reservedMatches, uniqueSequences, monotonicSequence, noFIFOLeapfrog)
		}
	}
	var duplicateActive, duplicatePurchase bool
	if err := database.QueryRow(`
		SELECT EXISTS(SELECT 1 FROM queue_attempts WHERE state IN ('waiting','invited','checkout') GROUP BY product_id,external_user_id HAVING count(*)>1),
		       EXISTS(SELECT 1 FROM queue_attempts WHERE state='purchased' GROUP BY product_id,external_user_id HAVING count(*)>1)`).Scan(
		&duplicateActive, &duplicatePurchase,
	); err != nil {
		fail("query user uniqueness: %v", err)
	}
	if duplicateActive || duplicatePurchase {
		fail("duplicate active=%t purchased=%t", duplicateActive, duplicatePurchase)
	}
	rows, err := database.Query(`SELECT id,version FROM queue_attempts WHERE state IN ('purchased','invite_expired','checkout_expired','payment_failed','cancelled','sold_out') ORDER BY id`)
	if err != nil {
		fail("query terminal attempts: %v", err)
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var id uuid.UUID
		var version int64
		if err := rows.Scan(&id, &version); err != nil {
			fail("scan terminal attempt: %v", err)
		}
		attemptID := domain.AttemptID(id)
		if previous, exists := terminalVersions[attemptID]; exists && previous != version {
			fail("terminal attempt %s changed version from %d to %d", id, previous, version)
		}
		terminalVersions[attemptID] = version
	}
	if err := rows.Err(); err != nil {
		fail("iterate terminal attempts: %v", err)
	}
}
