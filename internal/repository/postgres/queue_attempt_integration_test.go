package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/Woolfer0097/GoodQueue/internal/pkg/domain"
	"github.com/Woolfer0097/GoodQueue/internal/usecase"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	_ "github.com/jackc/pgx/v5/stdlib"
)

const (
	integrationProductOne = "11111111-1111-1111-1111-111111111111"
	integrationProductTwo = "22222222-2222-2222-2222-222222222222"
)

func TestIntegrationQueueLifecycleAndStockAdjustment(t *testing.T) {
	database := openIntegrationDatabase(t)
	repository := NewQueueAttemptRepository(database, 10*time.Minute, 5*time.Minute, 100)
	productOne := mustProductID(t, integrationProductOne)
	productTwo := mustProductID(t, integrationProductTwo)
	resetIntegrationProduct(t, database, productOne, 3)
	resetIntegrationProduct(t, database, productTwo, 2)

	ctx := context.Background()
	attempts := make([]domain.QueueAttempt, 6)
	for index := range attempts {
		result, err := repository.Join(ctx, joinCommand(productOne, index+1, fmt.Sprintf("key-%d", index+1)))
		if err != nil {
			t.Fatalf("join user %d: %v", index+1, err)
		}
		expectedWaiting := int64(0)
		if index >= 3 {
			expectedWaiting = int64(index - 2)
		}
		if result.TotalWaiting != expectedWaiting {
			t.Fatalf("join user %d waiting count: got %d, want %d", index+1, result.TotalWaiting, expectedWaiting)
		}
		attempts[index] = result.Attempt
	}
	for index := 0; index < 3; index++ {
		if attempts[index].State != domain.QueueAttemptCheckout {
			t.Fatalf("attempt %d state: got %s, want checkout", index+1, attempts[index].State)
		}
	}
	for index := 3; index < 6; index++ {
		if attempts[index].State != domain.QueueAttemptWaiting {
			t.Fatalf("attempt %d state: got %s, want waiting", index+1, attempts[index].State)
		}
	}
	if _, err := repository.Join(ctx, joinCommand(productOne, 7, "key-7")); !errors.Is(err, domain.ErrQueueFull) {
		t.Fatalf("seventh join: got %v, want queue full", err)
	}

	activeReplay, err := repository.Join(ctx, joinCommand(productOne, 1, "different-key"))
	if err != nil || activeReplay.Created || activeReplay.Attempt.ID != attempts[0].ID {
		t.Fatalf("active precedence returned %+v, %v", activeReplay, err)
	}
	if _, err := repository.Join(ctx, joinCommand(productTwo, 1, "key-1")); err != nil {
		t.Fatalf("same scoped key on another product: %v", err)
	}

	cancelled, err := repository.Cancel(ctx, domain.CancelQueueCommand{ProductID: productOne, ExternalUserID: "user-1"})
	if err != nil || cancelled.State != domain.QueueAttemptCancelled {
		t.Fatalf("cancel checkout: %+v, %v", cancelled, err)
	}
	assertAttemptState(t, database, attempts[3].ID, domain.QueueAttemptInvited)

	checkout, err := repository.StartCheckout(ctx, domain.StartCheckoutCommand{AttemptID: attempts[3].ID, ExternalUserID: "user-4"})
	if err != nil {
		t.Fatalf("start checkout: %v", err)
	}
	originalDeadline := *checkout.CheckoutDeadline
	replayedCheckout, err := repository.StartCheckout(ctx, domain.StartCheckoutCommand{AttemptID: attempts[3].ID, ExternalUserID: "user-4"})
	if err != nil || !replayedCheckout.CheckoutDeadline.Equal(originalDeadline) {
		t.Fatalf("checkout replay extended deadline: %+v, %v", replayedCheckout, err)
	}

	terminalReplay, err := repository.Join(ctx, joinCommand(productOne, 1, "key-1"))
	if err != nil || terminalReplay.Created || terminalReplay.Attempt.State != domain.QueueAttemptCancelled {
		t.Fatalf("terminal scoped replay: %+v, %v", terminalReplay, err)
	}

	if _, err := repository.Cancel(ctx, domain.CancelQueueCommand{ProductID: productOne, ExternalUserID: "user-2"}); err != nil {
		t.Fatalf("second cancellation: %v", err)
	}
	assertAttemptState(t, database, attempts[4].ID, domain.QueueAttemptInvited)
	if _, err := repository.Join(ctx, joinCommand(productOne, 7, "key-7")); err != nil {
		t.Fatalf("join after promotion: %v", err)
	}
	current, err := repository.FindCurrent(ctx, productOne, "user-7")
	if err != nil || current.PositionAhead != 1 {
		t.Fatalf("current waiting position: %+v, %v", current, err)
	}

	if _, err := database.Exec(`
		UPDATE queue_attempts SET invited_at=created_at,
		invitation_deadline=created_at+interval '1 microsecond',
		checkout_started_at=created_at,
		checkout_deadline=created_at+interval '1 microsecond', updated_at=clock_timestamp()
		WHERE id=$1`, uuid.UUID(attempts[3].ID)); err != nil {
		t.Fatalf("age checkout: %v", err)
	}
	adjustment := domain.StockAdjustmentCommand{
		ProductID: productOne, IdempotencyKey: "increase-1", Delta: 1,
		Reason: " restock ", ExternalReference: " shipment-1 ",
	}
	adjusted, err := repository.AdjustStock(ctx, adjustment)
	if err != nil || adjusted.StockBefore != 3 || adjusted.StockAfter != 4 {
		t.Fatalf("increase stock: %+v, %v", adjusted, err)
	}
	assertAttemptState(t, database, attempts[3].ID, domain.QueueAttemptCheckoutExpired)
	assertReservedMatchesAttempts(t, database, productOne)
	var expiredVersion int64
	if err := database.QueryRow(`SELECT version FROM queue_attempts WHERE id=$1`, uuid.UUID(attempts[3].ID)).Scan(&expiredVersion); err != nil {
		t.Fatalf("read expired version: %v", err)
	}

	replayedAdjustment, err := repository.AdjustStock(ctx, adjustment)
	if err != nil || !replayedAdjustment.Replayed || string(replayedAdjustment.ResponseBody) != string(adjusted.ResponseBody) {
		t.Fatalf("adjustment replay: %+v, %v", replayedAdjustment, err)
	}
	changedAdjustment := adjustment
	changedAdjustment.Reason = "different"
	if _, err := repository.AdjustStock(ctx, changedAdjustment); !errors.Is(err, domain.ErrAdjustmentConflict) {
		t.Fatalf("adjustment hash conflict: %v", err)
	}
	if _, err := repository.AdjustStock(ctx, domain.StockAdjustmentCommand{
		ProductID: productOne, IdempotencyKey: "increase-2", Delta: 1, Reason: "second restock",
	}); err != nil {
		t.Fatalf("second reconciliation trigger: %v", err)
	}
	var versionAfterSecondReconcile int64
	if err := database.QueryRow(`SELECT version FROM queue_attempts WHERE id=$1`, uuid.UUID(attempts[3].ID)).Scan(&versionAfterSecondReconcile); err != nil {
		t.Fatalf("read expired version after second reconcile: %v", err)
	}
	if versionAfterSecondReconcile != expiredVersion {
		t.Fatalf("expiry applied twice: version changed from %d to %d", expiredVersion, versionAfterSecondReconcile)
	}

	rejectedCommand := domain.StockAdjustmentCommand{
		ProductID: productOne, IdempotencyKey: "below-reserved", Delta: -2, Reason: "invalid shrink",
	}
	rejected, err := repository.AdjustStock(ctx, rejectedCommand)
	if !errors.Is(err, domain.ErrStockBelowReserved) || rejected.HTTPStatus != 409 {
		t.Fatalf("rejected adjustment: %+v, %v", rejected, err)
	}
	replayedRejection, err := repository.AdjustStock(ctx, rejectedCommand)
	if !errors.Is(err, domain.ErrStockBelowReserved) || !replayedRejection.Replayed ||
		string(replayedRejection.ResponseBody) != string(rejected.ResponseBody) {
		t.Fatalf("rejected adjustment replay: %+v, %v", replayedRejection, err)
	}

	terminalCurrent, err := repository.FindCurrent(ctx, productOne, "user-1")
	if err != nil || terminalCurrent.Attempt.State != domain.QueueAttemptCancelled {
		t.Fatalf("terminal current attempt: %+v, %v", terminalCurrent, err)
	}
	assertReservedMatchesAttempts(t, database, productOne)
}

func TestIntegrationStockShrinkRetainsExistingWaiters(t *testing.T) {
	database := openIntegrationDatabase(t)
	repository := NewQueueAttemptRepository(database, 10*time.Minute, 5*time.Minute, 100)
	productID := mustProductID(t, integrationProductTwo)
	resetIntegrationProduct(t, database, productID, 3)
	ctx := context.Background()

	for index := 1; index <= 6; index++ {
		if _, err := repository.Join(ctx, joinCommand(productID, 200+index, fmt.Sprintf("shrink-%d", index))); err != nil {
			t.Fatalf("prepare shrink attempt %d: %v", index, err)
		}
	}
	if _, err := database.Exec(`UPDATE products SET allocatable_stock=5 WHERE id=$1`, uuid.UUID(productID)); err != nil {
		t.Fatalf("prepare stock shrink: %v", err)
	}
	if _, err := repository.AdjustStock(ctx, domain.StockAdjustmentCommand{
		ProductID: productID, IdempotencyKey: "shrink-to-reserved", Delta: -2, Reason: "allocation correction",
	}); err != nil {
		t.Fatalf("shrink stock: %v", err)
	}
	var waitingCount int
	if err := database.QueryRow(`SELECT count(*) FROM queue_attempts WHERE product_id=$1 AND state='waiting'`, uuid.UUID(productID)).Scan(&waitingCount); err != nil {
		t.Fatalf("count retained waiters: %v", err)
	}
	if waitingCount != 3 {
		t.Fatalf("stock shrink retained %d waiters, want 3", waitingCount)
	}
	assertReservedMatchesAttempts(t, database, productID)
}

func TestIntegrationStockShrinkZeroAndFIFOIncrease(t *testing.T) {
	database := openIntegrationDatabase(t)
	repository := NewQueueAttemptRepository(database, 10*time.Minute, 5*time.Minute, 100)
	productID := mustProductID(t, integrationProductTwo)
	resetIntegrationProduct(t, database, productID, 0)
	ctx := context.Background()

	insertWaitingAttempt(t, database, productID, 1, "fifo-1")
	insertWaitingAttempt(t, database, productID, 2, "fifo-2")
	result, err := repository.AdjustStock(ctx, domain.StockAdjustmentCommand{
		ProductID: productID, IdempotencyKey: "fifo-increase", Delta: 1, Reason: "restock",
	})
	if err != nil || result.StockAfter != 1 {
		t.Fatalf("FIFO increase: %+v, %v", result, err)
	}
	assertAttemptStateByUser(t, database, productID, "fifo-1", domain.QueueAttemptInvited)
	assertAttemptStateByUser(t, database, productID, "fifo-2", domain.QueueAttemptWaiting)
	assertReservedMatchesAttempts(t, database, productID)

	if _, err := repository.Cancel(ctx, domain.CancelQueueCommand{ProductID: productID, ExternalUserID: "fifo-1"}); err != nil {
		t.Fatalf("cancel FIFO invitation: %v", err)
	}
	if _, err := repository.Cancel(ctx, domain.CancelQueueCommand{ProductID: productID, ExternalUserID: "fifo-2"}); err != nil {
		t.Fatalf("cancel FIFO second invitation: %v", err)
	}
	insertWaitingAttempt(t, database, productID, 3, "zero-1")
	insertWaitingAttempt(t, database, productID, 4, "zero-2")
	if _, err := repository.AdjustStock(ctx, domain.StockAdjustmentCommand{
		ProductID: productID, IdempotencyKey: "to-zero", Delta: -1, Reason: "withdraw",
	}); err != nil {
		t.Fatalf("adjust stock to zero: %v", err)
	}
	assertAttemptStateByUser(t, database, productID, "zero-1", domain.QueueAttemptSoldOut)
	assertAttemptStateByUser(t, database, productID, "zero-2", domain.QueueAttemptSoldOut)
	assertReservedMatchesAttempts(t, database, productID)
}

func TestIntegrationConcurrentJoinDoesNotOversellOrDuplicateSequence(t *testing.T) {
	database := openIntegrationDatabase(t)
	repository := NewQueueAttemptRepository(database, 10*time.Minute, 5*time.Minute, 100)
	productID := mustProductID(t, integrationProductTwo)
	resetIntegrationProduct(t, database, productID, 3)

	const callers = 20
	var waitGroup sync.WaitGroup
	results := make(chan error, callers)
	for index := 0; index < callers; index++ {
		waitGroup.Add(1)
		go func(userNumber int) {
			defer waitGroup.Done()
			_, err := repository.Join(context.Background(), joinCommand(productID, 100+userNumber, fmt.Sprintf("concurrent-%d", userNumber)))
			if err != nil && !errors.Is(err, domain.ErrQueueFull) {
				results <- err
			}
		}(index)
	}
	waitGroup.Wait()
	close(results)
	for err := range results {
		t.Fatalf("concurrent join: %v", err)
	}

	var total int
	var distinctSequences int
	var reserved int32
	if err := database.QueryRow(`
		SELECT count(*), count(DISTINCT queue_sequence),
		       (SELECT reserved FROM products WHERE id=$1)
		FROM queue_attempts WHERE product_id=$1`, uuid.UUID(productID)).Scan(&total, &distinctSequences, &reserved); err != nil {
		t.Fatalf("query concurrent joins: %v", err)
	}
	if total != 6 || distinctSequences != 6 || reserved != 3 {
		t.Fatalf("concurrent result total=%d distinct=%d reserved=%d", total, distinctSequences, reserved)
	}
	assertReservedMatchesAttempts(t, database, productID)
}

func TestIntegrationJoinReconcilesDueAttemptOnlyForNewKey(t *testing.T) {
	for _, testCase := range []struct {
		name         string
		dueState     domain.QueueAttemptState
		expiredState domain.QueueAttemptState
	}{
		{name: "invitation", dueState: domain.QueueAttemptInvited, expiredState: domain.QueueAttemptInviteExpired},
		{name: "checkout", dueState: domain.QueueAttemptCheckout, expiredState: domain.QueueAttemptCheckoutExpired},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			database := openIntegrationDatabase(t)
			repository := NewQueueAttemptRepository(database, 10*time.Minute, 5*time.Minute, 100)
			productID := mustProductID(t, integrationProductOne)
			resetIntegrationProduct(t, database, productID, 1)

			ctx := context.Background()
			due, err := repository.Join(ctx, joinCommand(productID, 301, "original-key"))
			if err != nil {
				t.Fatalf("join due caller: %v", err)
			}
			waiter, err := repository.Join(ctx, joinCommand(productID, 302, "waiter-key"))
			if err != nil || waiter.Attempt.State != domain.QueueAttemptWaiting {
				t.Fatalf("join waiter: %+v, %v", waiter, err)
			}
			makeAttemptDue(t, database, due.Attempt.ID, testCase.dueState)

			replay, err := repository.Join(ctx, joinCommand(productID, 301, "original-key"))
			if err != nil || replay.Created || replay.Attempt.State != testCase.dueState || replay.Attempt.ID != due.Attempt.ID {
				t.Fatalf("scoped replay changed stored attempt: %+v, %v", replay, err)
			}
			assertAttemptState(t, database, due.Attempt.ID, testCase.dueState)
			assertAttemptState(t, database, waiter.Attempt.ID, domain.QueueAttemptWaiting)

			rejoined, err := repository.Join(ctx, joinCommand(productID, 301, "new-key"))
			if err != nil || !rejoined.Created || rejoined.Attempt.State != domain.QueueAttemptWaiting || rejoined.Attempt.ID == due.Attempt.ID {
				t.Fatalf("new-key join returned stale active attempt: %+v, %v", rejoined, err)
			}
			assertAttemptState(t, database, due.Attempt.ID, testCase.expiredState)
			assertAttemptState(t, database, waiter.Attempt.ID, domain.QueueAttemptInvited)
			assertReservedMatchesAttempts(t, database, productID)
		})
	}
}

func TestIntegrationCancelPreservesDueExpiryAndReleasesOnce(t *testing.T) {
	for _, testCase := range []struct {
		name         string
		dueState     domain.QueueAttemptState
		expiredState domain.QueueAttemptState
	}{
		{name: "invitation", dueState: domain.QueueAttemptInvited, expiredState: domain.QueueAttemptInviteExpired},
		{name: "checkout", dueState: domain.QueueAttemptCheckout, expiredState: domain.QueueAttemptCheckoutExpired},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			database := openIntegrationDatabase(t)
			repository := NewQueueAttemptRepository(database, 10*time.Minute, 5*time.Minute, 100)
			productID := mustProductID(t, integrationProductOne)
			resetIntegrationProduct(t, database, productID, 1)

			ctx := context.Background()
			due, err := repository.Join(ctx, joinCommand(productID, 311, "cancel-due"))
			if err != nil {
				t.Fatalf("join due caller: %v", err)
			}
			waiter, err := repository.Join(ctx, joinCommand(productID, 312, "cancel-waiter"))
			if err != nil {
				t.Fatalf("join cancellation waiter: %v", err)
			}
			makeAttemptDue(t, database, due.Attempt.ID, testCase.dueState)

			expired, err := repository.Cancel(ctx, domain.CancelQueueCommand{ProductID: productID, ExternalUserID: "user-311"})
			if err != nil || expired.State != testCase.expiredState {
				t.Fatalf("cancel due attempt: %+v, %v", expired, err)
			}
			assertAttemptState(t, database, waiter.Attempt.ID, domain.QueueAttemptInvited)
			assertReservedMatchesAttempts(t, database, productID)

			replayed, err := repository.Cancel(ctx, domain.CancelQueueCommand{ProductID: productID, ExternalUserID: "user-311"})
			if err != nil || replayed.State != testCase.expiredState || replayed.Version != expired.Version {
				t.Fatalf("expired cancellation replay: %+v, %v", replayed, err)
			}
			assertReservedMatchesAttempts(t, database, productID)
		})
	}
}

func TestIntegrationCurrentReconcilesAndReturnsPositionThroughUseCase(t *testing.T) {
	database := openIntegrationDatabase(t)
	repository := NewQueueAttemptRepository(database, 10*time.Minute, 5*time.Minute, 300)
	queueUseCase := usecase.NewQueueUseCase(repository)
	productID := mustProductID(t, integrationProductTwo)
	resetIntegrationProduct(t, database, productID, 1)

	ctx := context.Background()
	due, err := repository.Join(ctx, joinCommand(productID, 321, "current-due"))
	if err != nil {
		t.Fatalf("join due caller: %v", err)
	}
	waiters := make([]domain.QueueAttempt, 3)
	for index := range waiters {
		joined, joinErr := repository.Join(ctx, joinCommand(productID, 322+index, fmt.Sprintf("current-waiter-%d", index)))
		if joinErr != nil {
			t.Fatalf("join current waiter %d: %v", index, joinErr)
		}
		waiters[index] = joined.Attempt
	}
	makeAttemptDue(t, database, due.Attempt.ID, domain.QueueAttemptCheckout)

	current, err := queueUseCase.Current(ctx, productID, "user-324")
	if err != nil || current.Attempt.ID != waiters[2].ID || current.Attempt.State != domain.QueueAttemptWaiting ||
		current.PositionAhead != 1 || current.TotalWaiting != 2 {
		t.Fatalf("effective current through use case: %+v, %v", current, err)
	}
	assertAttemptState(t, database, due.Attempt.ID, domain.QueueAttemptCheckoutExpired)
	assertAttemptState(t, database, waiters[0].ID, domain.QueueAttemptInvited)
	assertAttemptState(t, database, waiters[1].ID, domain.QueueAttemptWaiting)
	assertReservedMatchesAttempts(t, database, productID)
}

func TestIntegrationAttemptLockScopeLeavesUnrelatedHistoryUnlocked(t *testing.T) {
	database := openIntegrationDatabase(t)
	repository := NewQueueAttemptRepository(database, 10*time.Minute, 5*time.Minute, 100)
	productID := mustProductID(t, integrationProductOne)
	resetIntegrationProduct(t, database, productID, 1)

	holder, err := repository.Join(context.Background(), joinCommand(productID, 340, "lock-holder"))
	if err != nil {
		t.Fatal(err)
	}
	waiter, err := repository.Join(context.Background(), joinCommand(productID, 341, "lock-waiter"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repository.Cancel(context.Background(), domain.CancelQueueCommand{
		ProductID: productID, ExternalUserID: holder.Attempt.ExternalUserID,
	}); err != nil {
		t.Fatal(err)
	}

	tx, err := database.BeginTx(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := lockProductAttempts(context.Background(), tx, productID, attemptLockScope{}); err != nil {
		t.Fatal(err)
	}

	var terminalID uuid.UUID
	if err := database.QueryRow(`SELECT id FROM queue_attempts WHERE id=$1 FOR UPDATE NOWAIT`,
		uuid.UUID(holder.Attempt.ID)).Scan(&terminalID); err != nil {
		t.Fatalf("unrelated terminal attempt remained locked: %v", err)
	}
	var activeID uuid.UUID
	err = database.QueryRow(`SELECT id FROM queue_attempts WHERE id=$1 FOR UPDATE NOWAIT`,
		uuid.UUID(waiter.Attempt.ID)).Scan(&activeID)
	var postgresError *pgconn.PgError
	if !errors.As(err, &postgresError) || postgresError.Code != "55P03" {
		t.Fatalf("active attempt was not locked, err=%v", err)
	}
}

func TestIntegrationReconciliationErrorIdentifiesPoisonedProduct(t *testing.T) {
	database := openIntegrationDatabase(t)
	repository := NewQueueAttemptRepository(database, 10*time.Minute, 5*time.Minute, 100)
	productID := mustProductID(t, integrationProductOne)
	resetIntegrationProduct(t, database, productID, 1)
	t.Cleanup(func() { resetIntegrationProduct(t, database, productID, 1) })

	attempt := mustJoinPaymentAttempt(t, repository, productID, 350, "poisoned-product")
	attempt, err := repository.StartCheckout(context.Background(), domain.StartCheckoutCommand{
		AttemptID: attempt.ID, ExternalUserID: attempt.ExternalUserID,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := database.Exec(`
		UPDATE products SET reserved=0, updated_at=clock_timestamp()-interval '1 day' WHERE id=$1`, uuid.UUID(productID)); err != nil {
		t.Fatal(err)
	}
	makeAttemptDue(t, database, attempt.ID, domain.QueueAttemptCheckout)

	rows, err := database.Query(`SELECT id FROM products WHERE id<>$1`, uuid.UUID(productID))
	if err != nil {
		t.Fatal(err)
	}
	var excluded []domain.ProductID
	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err != nil {
			t.Fatal(err)
		}
		excluded = append(excluded, domain.ProductID(id))
	}
	_ = rows.Close()

	result, err := repository.ReconcileNextProduct(context.Background(), 10, excluded)
	if !errors.Is(err, domain.ErrReservedInvariant) {
		t.Fatalf("reconciliation error=%v, want reserved invariant", err)
	}
	if result.ProductID != productID {
		t.Fatalf("failed reconciliation product=%s, want %s", result.ProductID, productID)
	}
}

func openIntegrationDatabase(t *testing.T) *sql.DB {
	t.Helper()
	databaseURL := os.Getenv("GOODQUEUE_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("GOODQUEUE_TEST_DATABASE_URL is not set")
	}
	database, err := sql.Open("pgx", databaseURL)
	if err != nil {
		t.Fatalf("open integration database: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })
	return database
}

func resetIntegrationProduct(t *testing.T, database *sql.DB, productID domain.ProductID, stock int32) {
	t.Helper()
	if _, err := database.Exec(`DELETE FROM payment_inbox WHERE attempt_id IN (SELECT id FROM queue_attempts WHERE product_id=$1)`, uuid.UUID(productID)); err != nil {
		t.Fatalf("clear payment inbox: %v", err)
	}
	if _, err := database.Exec(`DELETE FROM notification_outbox WHERE attempt_id IN (SELECT id FROM queue_attempts WHERE product_id=$1)`, uuid.UUID(productID)); err != nil {
		t.Fatalf("clear notification outbox: %v", err)
	}
	if _, err := database.Exec(`DELETE FROM inventory_adjustments WHERE product_id=$1`, uuid.UUID(productID)); err != nil {
		t.Fatalf("clear inventory adjustments: %v", err)
	}
	if _, err := database.Exec(`DELETE FROM queue_attempts WHERE product_id=$1`, uuid.UUID(productID)); err != nil {
		t.Fatalf("clear queue attempts: %v", err)
	}
	if _, err := database.Exec(`UPDATE products SET allocatable_stock=$1, reserved=0, next_queue_sequence=1, queue_enabled=true WHERE id=$2`, stock, uuid.UUID(productID)); err != nil {
		t.Fatalf("reset product: %v", err)
	}
}

func insertWaitingAttempt(t *testing.T, database *sql.DB, productID domain.ProductID, sequence int64, userID string) {
	t.Helper()
	if _, err := database.Exec(`
		INSERT INTO queue_attempts (id, product_id, queue_sequence, external_user_id, idempotency_key, state)
		VALUES ($1,$2,$3,$4,$5,'waiting')`,
		uuid.New(), uuid.UUID(productID), sequence, userID, userID); err != nil {
		t.Fatalf("insert waiting attempt: %v", err)
	}
	if _, err := database.Exec(`UPDATE products SET next_queue_sequence=GREATEST(next_queue_sequence,$1+1) WHERE id=$2`, sequence, uuid.UUID(productID)); err != nil {
		t.Fatalf("advance test queue sequence: %v", err)
	}
}

func makeAttemptDue(t *testing.T, database *sql.DB, attemptID domain.AttemptID, state domain.QueueAttemptState) {
	t.Helper()
	var statement string
	switch state {
	case domain.QueueAttemptInvited:
		statement = `UPDATE queue_attempts
			SET state='invited', invitation_deadline=clock_timestamp(),
			    checkout_started_at=NULL, checkout_deadline=NULL, updated_at=clock_timestamp()
			WHERE id=$1`
	case domain.QueueAttemptCheckout:
		statement = `UPDATE queue_attempts
			SET checkout_deadline=clock_timestamp(), updated_at=clock_timestamp()
			WHERE id=$1`
	default:
		t.Fatalf("unsupported due state %s", state)
	}
	if _, err := database.Exec(statement, uuid.UUID(attemptID)); err != nil {
		t.Fatalf("make %s attempt due: %v", state, err)
	}
}

func joinCommand(productID domain.ProductID, userNumber int, key string) domain.JoinQueueCommand {
	return domain.JoinQueueCommand{ProductID: productID, ExternalUserID: domain.ExternalUserID(fmt.Sprintf("user-%d", userNumber)), IdempotencyKey: domain.IdempotencyKey(key)}
}

func mustProductID(t *testing.T, raw string) domain.ProductID {
	t.Helper()
	productID, err := domain.ParseProductID(raw)
	if err != nil {
		t.Fatalf("parse product ID: %v", err)
	}
	return productID
}

func assertAttemptState(t *testing.T, database *sql.DB, attemptID domain.AttemptID, want domain.QueueAttemptState) {
	t.Helper()
	var got domain.QueueAttemptState
	if err := database.QueryRow(`SELECT state FROM queue_attempts WHERE id=$1`, uuid.UUID(attemptID)).Scan(&got); err != nil {
		t.Fatalf("read attempt state: %v", err)
	}
	if got != want {
		t.Fatalf("attempt state: got %s, want %s", got, want)
	}
}

func assertAttemptStateByUser(t *testing.T, database *sql.DB, productID domain.ProductID, userID string, want domain.QueueAttemptState) {
	t.Helper()
	var got domain.QueueAttemptState
	if err := database.QueryRow(`SELECT state FROM queue_attempts WHERE product_id=$1 AND external_user_id=$2 ORDER BY queue_sequence DESC LIMIT 1`, uuid.UUID(productID), userID).Scan(&got); err != nil {
		t.Fatalf("read attempt state for %s: %v", userID, err)
	}
	if got != want {
		t.Fatalf("attempt state for %s: got %s, want %s", userID, got, want)
	}
}

func assertReservedMatchesAttempts(t *testing.T, database *sql.DB, productID domain.ProductID) {
	t.Helper()
	var matches bool
	if err := database.QueryRow(`
		SELECT reserved = (SELECT count(*) FROM queue_attempts WHERE product_id=products.id AND state IN ('invited','checkout'))
		FROM products WHERE id=$1`, uuid.UUID(productID)).Scan(&matches); err != nil {
		t.Fatalf("check reserved accounting: %v", err)
	}
	if !matches {
		t.Fatal("product reserved does not match invited plus checkout attempts")
	}
}
