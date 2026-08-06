package postgres

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/Woolfer0097/GoodQueue/internal/pkg/domain"
	"github.com/google/uuid"
)

func TestIntegrationPaymentReplayConflictAndUnknownAttempt(t *testing.T) {
	database := openIntegrationDatabase(t)
	repository := NewQueueAttemptRepository(database, 10*time.Minute, 5*time.Minute, 100)
	if _, err := database.Exec(`DELETE FROM payment_inbox WHERE provider='replay-provider'`); err != nil {
		t.Fatal(err)
	}
	command := mustPaymentCommand(t, "replay-provider", "unknown-event", uuid.NewString(), "failed", "discarded")

	first, err := repository.ProcessPayment(context.Background(), command)
	if err != nil || first.HTTPStatus != 404 || first.Code != "attempt_not_found" {
		t.Fatalf("unknown payment = %+v, %v", first, err)
	}
	replay, err := repository.ProcessPayment(context.Background(), command)
	if err != nil || !replay.Replayed || replay.HTTPStatus != first.HTTPStatus || !bytes.Equal(replay.ResponseBody, first.ResponseBody) {
		t.Fatalf("unknown replay = %+v, %v", replay, err)
	}

	changed := command
	changed.AttemptID = domain.AttemptID(uuid.New())
	conflict, err := repository.ProcessPayment(context.Background(), changed)
	if err != nil || conflict.HTTPStatus != 409 || conflict.Code != "event_conflict" {
		t.Fatalf("changed event = %+v, %v", conflict, err)
	}
	var attemptID uuid.UUID
	var status string
	var referenceIsNull bool
	if err := database.QueryRow(`SELECT attempt_id,status,payment_reference IS NULL FROM payment_inbox WHERE provider=$1 AND event_id=$2`, command.Provider, command.EventID).Scan(&attemptID, &status, &referenceIsNull); err != nil {
		t.Fatal(err)
	}
	if attemptID != uuid.UUID(command.AttemptID) || status != "rejected" || !referenceIsNull {
		t.Fatalf("conflict altered original row: attempt=%s status=%s reference_null=%t", attemptID, status, referenceIsNull)
	}
}

func TestIntegrationPaymentClaimLeaseAndFencing(t *testing.T) {
	database := openIntegrationDatabase(t)
	repository := NewQueueAttemptRepository(database, 10*time.Minute, 5*time.Minute, 100)
	productID := mustProductID(t, integrationProductOne)
	resetIntegrationProduct(t, database, productID, 1)
	attempt := mustJoinPaymentAttempt(t, repository, productID, 401, "lease-attempt")
	command := mustPaymentCommand(t, "lease-provider", "lease-event", uuid.UUID(attempt.ID).String(), "failed", "")

	claimed, err := repository.ClaimPayment(context.Background(), command)
	if err != nil || claimed.Claim == nil {
		t.Fatalf("initial claim = %+v, %v", claimed, err)
	}
	firstClaim := *claimed.Claim
	var firstCount int
	var firstToken uuid.UUID
	var firstGeneration int64
	if err := database.QueryRow(`SELECT attempt_count,claim_token,claim_generation FROM payment_inbox WHERE id=$1`, firstClaim.InboxID).Scan(&firstCount, &firstToken, &firstGeneration); err != nil {
		t.Fatal(err)
	}
	live, err := repository.ClaimPayment(context.Background(), command)
	if err != nil || live.Response == nil || live.Response.HTTPStatus != 202 {
		t.Fatalf("live lease response = %+v, %v", live, err)
	}
	var liveCount int
	var liveToken uuid.UUID
	var liveGeneration int64
	if err := database.QueryRow(`SELECT attempt_count,claim_token,claim_generation FROM payment_inbox WHERE id=$1`, firstClaim.InboxID).Scan(&liveCount, &liveToken, &liveGeneration); err != nil {
		t.Fatal(err)
	}
	if liveCount != firstCount || liveToken != firstToken || liveGeneration != firstGeneration {
		t.Fatalf("live lease mutated claim: count %d/%d token %s/%s generation %d/%d", firstCount, liveCount, firstToken, liveToken, firstGeneration, liveGeneration)
	}

	if _, err := database.Exec(`UPDATE payment_inbox SET lease_until=clock_timestamp()-interval '1 second' WHERE id=$1`, firstClaim.InboxID); err != nil {
		t.Fatal(err)
	}
	reclaimed, err := repository.ClaimPayment(context.Background(), command)
	if err != nil || reclaimed.Claim == nil {
		t.Fatalf("expired reclaim = %+v, %v", reclaimed, err)
	}
	secondClaim := *reclaimed.Claim
	if secondClaim.Token == firstClaim.Token || secondClaim.Generation != firstClaim.Generation+1 {
		t.Fatalf("reclaim did not fence old worker: old=%+v new=%+v", firstClaim, secondClaim)
	}
	if _, err := repository.ProcessPaymentClaim(context.Background(), firstClaim); !errors.Is(err, domain.ErrStaleClaim) {
		t.Fatalf("old claim finalized: %v", err)
	}
	if err := repository.RetryPaymentClaim(context.Background(), firstClaim, errors.New("old worker")); !errors.Is(err, domain.ErrStaleClaim) {
		t.Fatalf("old claim retried: %v", err)
	}
	wrongGeneration := secondClaim
	wrongGeneration.Generation = firstClaim.Generation
	if _, err := repository.ProcessPaymentClaim(context.Background(), wrongGeneration); !errors.Is(err, domain.ErrStaleClaim) {
		t.Fatalf("old generation finalized with new token: %v", err)
	}
	result, err := repository.ProcessPaymentClaim(context.Background(), secondClaim)
	if err != nil || result.Code != "accepted" {
		t.Fatalf("new claim finalization = %+v, %v", result, err)
	}
}

func TestIntegrationPaymentClaimRejectsCommandSubstitutionWithoutMutation(t *testing.T) {
	testCases := []struct {
		name              string
		originalOutcome   string
		originalReference string
		changedProvider   string
		changedEventID    string
		useSecondAttempt  bool
		changedOutcome    string
		changedReference  string
	}{
		{name: "provider", originalOutcome: "failed", changedProvider: "substituted-provider"},
		{name: "event", originalOutcome: "failed", changedEventID: "substituted-event"},
		{name: "attempt", originalOutcome: "failed", useSecondAttempt: true},
		{name: "outcome", originalOutcome: "failed", changedOutcome: "succeeded", changedReference: "substituted-reference"},
		{name: "reference", originalOutcome: "succeeded", originalReference: "original-reference", changedReference: "substituted-reference"},
		{
			name: "attempt_outcome_reference", originalOutcome: "failed", useSecondAttempt: true,
			changedOutcome: "succeeded", changedReference: "substituted-reference",
		},
	}

	for index, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			database := openIntegrationDatabase(t)
			repository := NewQueueAttemptRepository(database, 10*time.Minute, 5*time.Minute, 100)
			productID := mustProductID(t, integrationProductOne)
			resetIntegrationProduct(t, database, productID, 2)
			first := mustJoinPaymentAttempt(t, repository, productID, 1200+index*2, "integrity-first")
			second := mustJoinPaymentAttempt(t, repository, productID, 1201+index*2, "integrity-second")
			provider := "integrity-provider-" + testCase.name
			eventID := "integrity-event-" + testCase.name
			original := mustPaymentCommand(
				t, provider, eventID, uuid.UUID(first.ID).String(), testCase.originalOutcome, testCase.originalReference,
			)
			claimed, err := repository.ClaimPayment(context.Background(), original)
			if err != nil || claimed.Claim == nil {
				t.Fatalf("claim original payment = %+v, %v", claimed, err)
			}

			changedProvider := provider
			if testCase.changedProvider != "" {
				changedProvider = testCase.changedProvider
			}
			changedEventID := eventID
			if testCase.changedEventID != "" {
				changedEventID = testCase.changedEventID
			}
			changedAttemptID := first.ID
			if testCase.useSecondAttempt {
				changedAttemptID = second.ID
			}
			changedOutcome := testCase.originalOutcome
			if testCase.changedOutcome != "" {
				changedOutcome = testCase.changedOutcome
			}
			changedReference := testCase.originalReference
			if testCase.changedReference != "" {
				changedReference = testCase.changedReference
			}
			mutatedClaim := *claimed.Claim
			mutatedClaim.Command = mustPaymentCommand(
				t, changedProvider, changedEventID, uuid.UUID(changedAttemptID).String(), changedOutcome, changedReference,
			)

			before := readPaymentIntegrityState(t, database, productID, first.ID, second.ID, mutatedClaim.InboxID)
			result, err := repository.ProcessPaymentClaim(context.Background(), mutatedClaim)
			if !errors.Is(err, domain.ErrPaymentClaimIntegrity) || !errors.Is(err, domain.ErrInternal) {
				t.Fatalf("mutated claim result = %+v, error = %v", result, err)
			}
			after := readPaymentIntegrityState(t, database, productID, first.ID, second.ID, mutatedClaim.InboxID)
			if before != after {
				t.Fatalf("mutated claim changed persisted state:\nbefore=%+v\nafter=%+v", before, after)
			}

			legitimate, err := repository.ProcessPaymentClaim(context.Background(), *claimed.Claim)
			if err != nil || legitimate.Code != "accepted" {
				t.Fatalf("legitimate claim after rejection = %+v, %v", legitimate, err)
			}
		})
	}
}

func TestIntegrationPaymentOutcomeMatrix(t *testing.T) {
	states := []domain.QueueAttemptState{
		domain.QueueAttemptWaiting,
		domain.QueueAttemptInvited,
		domain.QueueAttemptCheckout,
		domain.QueueAttemptPurchased,
		domain.QueueAttemptInviteExpired,
		domain.QueueAttemptCheckoutExpired,
		domain.QueueAttemptPaymentFailed,
		domain.QueueAttemptCancelled,
		domain.QueueAttemptSoldOut,
	}
	for _, outcome := range []domain.PaymentOutcome{domain.PaymentSucceeded, domain.PaymentFailed} {
		for _, state := range states {
			state, outcome := state, outcome
			t.Run(string(outcome)+"_"+string(state), func(t *testing.T) {
				database := openIntegrationDatabase(t)
				repository := NewQueueAttemptRepository(database, 10*time.Minute, 5*time.Minute, 100)
				productID := mustProductID(t, integrationProductOne)
				resetIntegrationProduct(t, database, productID, 3)
				attempt := preparePaymentAttemptState(t, database, repository, productID, state, 500)
				reference := "matrix-reference"
				if state == domain.QueueAttemptPurchased {
					reference = "owned-reference"
				}
				command := mustPaymentCommand(t, "matrix-provider", fmt.Sprintf("%s-%s", outcome, state), uuid.UUID(attempt.ID).String(), string(outcome), reference)
				result, err := repository.ProcessPayment(context.Background(), command)
				if err != nil {
					t.Fatalf("process matrix payment: %v", err)
				}
				wantStatus, wantCode := expectedPaymentOutcome(outcome, state)
				if result.HTTPStatus != wantStatus || result.Code != wantCode {
					t.Fatalf("result = %+v, want status=%d code=%s", result, wantStatus, wantCode)
				}
				assertReservedMatchesAttempts(t, database, productID)
			})
		}
	}
}

func TestIntegrationDueCheckoutPaymentUsesReconciledState(t *testing.T) {
	for _, outcome := range []domain.PaymentOutcome{domain.PaymentSucceeded, domain.PaymentFailed} {
		t.Run(string(outcome), func(t *testing.T) {
			database := openIntegrationDatabase(t)
			repository := NewQueueAttemptRepository(database, 10*time.Minute, 5*time.Minute, 100)
			productID := mustProductID(t, integrationProductOne)
			resetIntegrationProduct(t, database, productID, 1)
			attempt := mustJoinPaymentAttempt(t, repository, productID, 601, "due-checkout")
			makeAttemptDue(t, database, attempt.ID, domain.QueueAttemptCheckout)
			command := mustPaymentCommand(t, "due-provider", "due-"+string(outcome), uuid.UUID(attempt.ID).String(), string(outcome), "due-reference")
			result, err := repository.ProcessPayment(context.Background(), command)
			if err != nil {
				t.Fatal(err)
			}
			want := "ignored_terminal"
			if outcome == domain.PaymentSucceeded {
				want = "compensation_required"
			}
			if result.Code != want {
				t.Fatalf("due result = %s, want %s", result.Code, want)
			}
			assertAttemptState(t, database, attempt.ID, domain.QueueAttemptCheckoutExpired)
			assertReservedMatchesAttempts(t, database, productID)
		})
	}
}

func TestIntegrationAcceptedPaymentAccountingSoldOutAndFailurePromotion(t *testing.T) {
	database := openIntegrationDatabase(t)
	repository := NewQueueAttemptRepository(database, 10*time.Minute, 5*time.Minute, 200)
	productID := mustProductID(t, integrationProductOne)
	resetIntegrationProduct(t, database, productID, 1)
	checkout := mustJoinPaymentAttempt(t, repository, productID, 701, "purchase-checkout")
	waiter := mustJoinPaymentAttempt(t, repository, productID, 702, "purchase-waiter")
	result, err := repository.ProcessPayment(context.Background(), mustPaymentCommand(t, "accounting-provider", "purchase-event", uuid.UUID(checkout.ID).String(), "succeeded", "purchase-reference"))
	if err != nil || result.Code != "accepted" {
		t.Fatalf("accepted purchase = %+v, %v", result, err)
	}
	assertAttemptState(t, database, waiter.ID, domain.QueueAttemptSoldOut)
	var stock, reserved int32
	if err := database.QueryRow(`SELECT allocatable_stock,reserved FROM products WHERE id=$1`, uuid.UUID(productID)).Scan(&stock, &reserved); err != nil {
		t.Fatal(err)
	}
	if stock != 0 || reserved != 0 {
		t.Fatalf("purchase accounting stock=%d reserved=%d", stock, reserved)
	}

	resetIntegrationProduct(t, database, productID, 1)
	failed := mustJoinPaymentAttempt(t, repository, productID, 703, "failed-checkout")
	promoted := mustJoinPaymentAttempt(t, repository, productID, 704, "failed-waiter")
	result, err = repository.ProcessPayment(context.Background(), mustPaymentCommand(t, "accounting-provider", "failed-event", uuid.UUID(failed.ID).String(), "failed", "ignored"))
	if err != nil || result.Code != "accepted" {
		t.Fatalf("accepted failure = %+v, %v", result, err)
	}
	assertAttemptState(t, database, failed.ID, domain.QueueAttemptPaymentFailed)
	assertAttemptState(t, database, promoted.ID, domain.QueueAttemptInvited)
	assertReservedMatchesAttempts(t, database, productID)
}

func TestIntegrationAcceptedReferenceOwnershipAndExactReplay(t *testing.T) {
	database := openIntegrationDatabase(t)
	repository := NewQueueAttemptRepository(database, 10*time.Minute, 5*time.Minute, 100)
	productID := mustProductID(t, integrationProductOne)
	resetIntegrationProduct(t, database, productID, 4)
	first := mustJoinPaymentAttempt(t, repository, productID, 801, "owner-one")
	firstCommand := mustPaymentCommand(t, "reference-provider", "owner-event", uuid.UUID(first.ID).String(), "succeeded", "shared-reference")
	accepted, err := repository.ProcessPayment(context.Background(), firstCommand)
	if err != nil || accepted.Code != "accepted" {
		t.Fatalf("first acceptance = %+v, %v", accepted, err)
	}

	sameAttempt := mustPaymentCommand(t, "reference-provider", "same-attempt-event", uuid.UUID(first.ID).String(), "succeeded", "shared-reference")
	already, err := repository.ProcessPayment(context.Background(), sameAttempt)
	if err != nil || already.Code != "already_accepted" {
		t.Fatalf("same accepted reference = %+v, %v", already, err)
	}
	second := mustJoinPaymentAttempt(t, repository, productID, 802, "owner-two")
	collisionCommand := mustPaymentCommand(t, "reference-provider", "collision-event", uuid.UUID(second.ID).String(), "succeeded", "shared-reference")
	collision, err := repository.ProcessPayment(context.Background(), collisionCommand)
	if err != nil || collision.Code != "compensation_required" {
		t.Fatalf("reference collision = %+v, %v", collision, err)
	}
	assertAttemptState(t, database, second.ID, domain.QueueAttemptCheckout)

	third := mustJoinPaymentAttempt(t, repository, productID, 803, "owner-three")
	differentProvider := mustPaymentCommand(t, "other-provider", "other-provider-event", uuid.UUID(third.ID).String(), "succeeded", "shared-reference")
	providerScoped, err := repository.ProcessPayment(context.Background(), differentProvider)
	if err != nil || providerScoped.Code != "accepted" {
		t.Fatalf("provider-scoped reference = %+v, %v", providerScoped, err)
	}

	var outboxBefore, outboxAfter int
	if err := database.QueryRow(`SELECT count(*) FROM notification_outbox WHERE deduplication_key=$1`, "payment.compensation:"+collision.InboxID.String()).Scan(&outboxBefore); err != nil {
		t.Fatal(err)
	}
	replayed, err := repository.ProcessPayment(context.Background(), collisionCommand)
	if err != nil || !replayed.Replayed || !bytes.Equal(replayed.ResponseBody, collision.ResponseBody) {
		t.Fatalf("collision replay = %+v, %v", replayed, err)
	}
	if err := database.QueryRow(`SELECT count(*) FROM notification_outbox WHERE deduplication_key=$1`, "payment.compensation:"+collision.InboxID.String()).Scan(&outboxAfter); err != nil {
		t.Fatal(err)
	}
	if outboxBefore != 1 || outboxAfter != 1 {
		t.Fatalf("replay duplicated compensation: before=%d after=%d", outboxBefore, outboxAfter)
	}
	assertReservedMatchesAttempts(t, database, productID)
}

func TestIntegrationPaymentFinalizationFailureRollsBackCapacity(t *testing.T) {
	database := openIntegrationDatabase(t)
	repository := NewQueueAttemptRepository(database, 10*time.Minute, 5*time.Minute, 100)
	productID := mustProductID(t, integrationProductOne)
	resetIntegrationProduct(t, database, productID, 1)
	attempt := mustJoinPaymentAttempt(t, repository, productID, 901, "rollback-checkout")
	if _, err := database.Exec(`
		CREATE OR REPLACE FUNCTION fail_payment_terminal_test() RETURNS trigger LANGUAGE plpgsql AS $$
		BEGIN
			IF NEW.status IN ('completed','rejected') THEN RAISE EXCEPTION 'forced finalization failure'; END IF;
			RETURN NEW;
		END $$;
		CREATE TRIGGER fail_payment_terminal_test BEFORE UPDATE ON payment_inbox
		FOR EACH ROW EXECUTE FUNCTION fail_payment_terminal_test();`); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = database.Exec(`DROP TRIGGER IF EXISTS fail_payment_terminal_test ON payment_inbox; DROP FUNCTION IF EXISTS fail_payment_terminal_test()`)
	})
	command := mustPaymentCommand(t, "rollback-provider", "rollback-event", uuid.UUID(attempt.ID).String(), "succeeded", "rollback-reference")
	if _, err := repository.ProcessPayment(context.Background(), command); err == nil {
		t.Fatal("forced finalization unexpectedly succeeded")
	}
	assertAttemptState(t, database, attempt.ID, domain.QueueAttemptCheckout)
	var stock, reserved int32
	if err := database.QueryRow(`SELECT allocatable_stock,reserved FROM products WHERE id=$1`, uuid.UUID(productID)).Scan(&stock, &reserved); err != nil {
		t.Fatal(err)
	}
	if stock != 1 || reserved != 1 {
		t.Fatalf("capacity did not roll back: stock=%d reserved=%d", stock, reserved)
	}
}

func TestIntegrationConcurrentDuplicateAndReferenceCollision(t *testing.T) {
	database := openIntegrationDatabase(t)
	repository := NewQueueAttemptRepository(database, 10*time.Minute, 5*time.Minute, 100)
	productID := mustProductID(t, integrationProductOne)
	resetIntegrationProduct(t, database, productID, 3)
	first := mustJoinPaymentAttempt(t, repository, productID, 1001, "concurrent-first")
	second := mustJoinPaymentAttempt(t, repository, productID, 1002, "concurrent-second")
	commands := []domain.PaymentCommand{
		mustPaymentCommand(t, "concurrent-provider", "concurrent-event-one", uuid.UUID(first.ID).String(), "succeeded", "concurrent-reference"),
		mustPaymentCommand(t, "concurrent-provider", "concurrent-event-two", uuid.UUID(second.ID).String(), "succeeded", "concurrent-reference"),
	}
	var waitGroup sync.WaitGroup
	results := make(chan domain.PaymentResult, 4)
	errorsChannel := make(chan error, 4)
	for index := 0; index < 4; index++ {
		waitGroup.Add(1)
		go func(command domain.PaymentCommand) {
			defer waitGroup.Done()
			result, err := repository.ProcessPayment(context.Background(), command)
			if err != nil {
				errorsChannel <- err
				return
			}
			results <- result
		}(commands[index%2])
	}
	waitGroup.Wait()
	close(results)
	close(errorsChannel)
	for err := range errorsChannel {
		t.Fatalf("concurrent payment: %v", err)
	}
	accepted, compensation := 0, 0
	for result := range results {
		switch result.Code {
		case "accepted", "already_accepted":
			accepted++
		case "compensation_required", "processing":
			compensation++
		default:
			t.Fatalf("unexpected concurrent result: %+v", result)
		}
	}
	if accepted == 0 || compensation == 0 {
		t.Fatalf("concurrent outcomes accepted=%d compensation/processing=%d", accepted, compensation)
	}
	var owners int
	if err := database.QueryRow(`SELECT count(*) FROM queue_attempts WHERE accepted_payment_provider='concurrent-provider' AND accepted_payment_reference='concurrent-reference'`).Scan(&owners); err != nil {
		t.Fatal(err)
	}
	if owners != 1 {
		t.Fatalf("accepted reference owners=%d", owners)
	}
	assertReservedMatchesAttempts(t, database, productID)
}

func TestIntegrationPaymentCancelAndStockAdjustmentSerializeWithoutDeadlock(t *testing.T) {
	database := openIntegrationDatabase(t)
	repository := NewQueueAttemptRepository(database, 10*time.Minute, 5*time.Minute, 100)
	productID := mustProductID(t, integrationProductTwo)
	resetIntegrationProduct(t, database, productID, 2)
	attempt := mustJoinPaymentAttempt(t, repository, productID, 1101, "race-target")
	command := mustPaymentCommand(t, "race-provider", "race-event", uuid.UUID(attempt.ID).String(), "succeeded", "race-reference")

	start := make(chan struct{})
	errorsChannel := make(chan error, 3)
	var waitGroup sync.WaitGroup
	waitGroup.Add(3)
	go func() {
		defer waitGroup.Done()
		<-start
		result, err := repository.ProcessPayment(context.Background(), command)
		if err != nil {
			errorsChannel <- err
			return
		}
		if result.Code != "accepted" && result.Code != "compensation_required" {
			errorsChannel <- fmt.Errorf("unexpected payment result %s", result.Code)
		}
	}()
	go func() {
		defer waitGroup.Done()
		<-start
		_, err := repository.Cancel(context.Background(), domain.CancelQueueCommand{ProductID: productID, ExternalUserID: "user-1101"})
		if err != nil && !errors.Is(err, domain.ErrAlreadyPurchased) {
			errorsChannel <- err
		}
	}()
	go func() {
		defer waitGroup.Done()
		<-start
		_, err := repository.AdjustStock(context.Background(), domain.StockAdjustmentCommand{
			ProductID: productID, IdempotencyKey: "race-adjustment", Delta: 1, Reason: "race restock",
		})
		if err != nil {
			errorsChannel <- err
		}
	}()
	close(start)
	done := make(chan struct{})
	go func() { waitGroup.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("payment/cancel/stock race deadlocked")
	}
	close(errorsChannel)
	for err := range errorsChannel {
		t.Fatalf("serialized race: %v", err)
	}
	assertReservedMatchesAttempts(t, database, productID)
	var validStock bool
	if err := database.QueryRow(`SELECT reserved >= 0 AND reserved <= allocatable_stock FROM products WHERE id=$1`, uuid.UUID(productID)).Scan(&validStock); err != nil {
		t.Fatal(err)
	}
	if !validStock {
		t.Fatal("payment/cancel/stock race violated capacity bounds")
	}
}

func expectedPaymentOutcome(outcome domain.PaymentOutcome, state domain.QueueAttemptState) (int, string) {
	if outcome == domain.PaymentSucceeded {
		switch state {
		case domain.QueueAttemptCheckout:
			return 200, "accepted"
		case domain.QueueAttemptPurchased:
			return 200, "already_accepted"
		case domain.QueueAttemptWaiting, domain.QueueAttemptInvited:
			return 409, "invalid_transition"
		default:
			return 200, "compensation_required"
		}
	}
	switch state {
	case domain.QueueAttemptCheckout:
		return 200, "accepted"
	case domain.QueueAttemptPurchased:
		return 200, "ignored_purchase_final"
	case domain.QueueAttemptWaiting, domain.QueueAttemptInvited:
		return 409, "invalid_transition"
	default:
		return 200, "ignored_terminal"
	}
}

func preparePaymentAttemptState(
	t *testing.T,
	database *sql.DB,
	repository *QueueAttemptRepository,
	productID domain.ProductID,
	state domain.QueueAttemptState,
	userNumber int,
) domain.QueueAttempt {
	t.Helper()
	attempt := mustJoinPaymentAttempt(t, repository, productID, userNumber, "matrix-attempt")
	if state == domain.QueueAttemptCheckout {
		return attempt
	}
	if state == domain.QueueAttemptPurchased {
		result, err := repository.ProcessPayment(context.Background(), mustPaymentCommand(t, "matrix-provider", "prepare-purchase", uuid.UUID(attempt.ID).String(), "succeeded", "owned-reference"))
		if err != nil || result.Code != "accepted" {
			t.Fatalf("prepare purchase: %+v, %v", result, err)
		}
		return attempt
	}

	statement := `UPDATE queue_attempts SET state=$1, updated_at=clock_timestamp(), version=version+1,
		terminal_at=NULL, terminal_reason=NULL, terminal_message=NULL, purchased_at=NULL,
		accepted_payment_provider=NULL, accepted_payment_reference=NULL WHERE id=$2`
	switch state {
	case domain.QueueAttemptWaiting:
		statement = `UPDATE queue_attempts SET state='waiting', invited_at=NULL, invitation_deadline=NULL,
			checkout_started_at=NULL, checkout_deadline=NULL, updated_at=clock_timestamp(), version=version+1 WHERE id=$1`
	case domain.QueueAttemptInvited:
		statement = `UPDATE queue_attempts SET state='invited', checkout_started_at=NULL, checkout_deadline=NULL,
			updated_at=clock_timestamp(), version=version+1 WHERE id=$1`
	case domain.QueueAttemptInviteExpired:
		statement = `UPDATE queue_attempts SET state='invite_expired', invitation_deadline=created_at+interval '1 microsecond',
			checkout_started_at=NULL, checkout_deadline=NULL, terminal_at=clock_timestamp(), terminal_reason='invite_expired',
			terminal_message='invite_expired', updated_at=clock_timestamp(), version=version+1 WHERE id=$1`
	case domain.QueueAttemptCheckoutExpired:
		statement = `UPDATE queue_attempts SET state='checkout_expired', checkout_deadline=created_at+interval '2 microseconds',
			terminal_at=clock_timestamp(), terminal_reason='checkout_expired', terminal_message='checkout_expired',
			updated_at=clock_timestamp(), version=version+1 WHERE id=$1`
	case domain.QueueAttemptPaymentFailed, domain.QueueAttemptCancelled:
		statement = `UPDATE queue_attempts SET state=$1, terminal_at=clock_timestamp(), terminal_reason='prepared_terminal',
			terminal_message='prepared_terminal', updated_at=clock_timestamp(), version=version+1 WHERE id=$2`
	case domain.QueueAttemptSoldOut:
		statement = `UPDATE queue_attempts SET state='sold_out', invited_at=NULL, invitation_deadline=NULL,
			checkout_started_at=NULL, checkout_deadline=NULL, terminal_at=clock_timestamp(), terminal_reason='sold_out',
			terminal_message='sold_out', updated_at=clock_timestamp(), version=version+1 WHERE id=$1`
	default:
		t.Fatalf("unsupported prepared state %s", state)
	}
	var err error
	if state == domain.QueueAttemptPaymentFailed || state == domain.QueueAttemptCancelled {
		_, err = database.Exec(statement, string(state), uuid.UUID(attempt.ID))
	} else {
		_, err = database.Exec(statement, uuid.UUID(attempt.ID))
	}
	if err != nil {
		t.Fatalf("prepare %s: %v", state, err)
	}
	if state != domain.QueueAttemptInvited {
		if _, err := database.Exec(`UPDATE products SET reserved=0 WHERE id=$1`, uuid.UUID(productID)); err != nil {
			t.Fatalf("release prepared reservation: %v", err)
		}
	}
	return attempt
}

func mustJoinPaymentAttempt(t *testing.T, repository *QueueAttemptRepository, productID domain.ProductID, userNumber int, key string) domain.QueueAttempt {
	t.Helper()
	joined, err := repository.Join(context.Background(), joinCommand(productID, userNumber, key))
	if err != nil {
		t.Fatalf("join payment attempt: %v", err)
	}
	return joined.Attempt
}

func mustPaymentCommand(t *testing.T, provider, eventID, attemptID, outcome, reference string) domain.PaymentCommand {
	t.Helper()
	command, err := domain.ParsePaymentCommand(provider, eventID, attemptID, outcome, reference)
	if err != nil {
		t.Fatalf("parse payment command: %v", err)
	}
	return command
}

type paymentIntegrityState struct {
	stock               int32
	reserved            int32
	firstState          string
	firstVersion        int64
	secondState         string
	secondVersion       int64
	inboxStatus         string
	inboxToken          uuid.UUID
	inboxGeneration     int64
	inboxAttemptCount   int
	inboxLeaseUntil     time.Time
	inboxUpdatedAt      time.Time
	inboxResponseIsNil  bool
	inboxCompletedIsNil bool
	inboxLastErrorIsNil bool
	outboxCount         int
}

func readPaymentIntegrityState(
	t *testing.T,
	database *sql.DB,
	productID domain.ProductID,
	firstID domain.AttemptID,
	secondID domain.AttemptID,
	inboxID uuid.UUID,
) paymentIntegrityState {
	t.Helper()
	var state paymentIntegrityState
	if err := database.QueryRow(`SELECT allocatable_stock,reserved FROM products WHERE id=$1`, uuid.UUID(productID)).Scan(
		&state.stock, &state.reserved,
	); err != nil {
		t.Fatalf("read product integrity state: %v", err)
	}
	if err := database.QueryRow(`SELECT state,version FROM queue_attempts WHERE id=$1`, uuid.UUID(firstID)).Scan(
		&state.firstState, &state.firstVersion,
	); err != nil {
		t.Fatalf("read first attempt integrity state: %v", err)
	}
	if err := database.QueryRow(`SELECT state,version FROM queue_attempts WHERE id=$1`, uuid.UUID(secondID)).Scan(
		&state.secondState, &state.secondVersion,
	); err != nil {
		t.Fatalf("read second attempt integrity state: %v", err)
	}
	if err := database.QueryRow(`
		SELECT status,claim_token,claim_generation,attempt_count,lease_until,updated_at,
			response_http_status IS NULL AND response_body IS NULL,
			completed_at IS NULL,last_error IS NULL
		FROM payment_inbox WHERE id=$1`, inboxID).Scan(
		&state.inboxStatus, &state.inboxToken, &state.inboxGeneration, &state.inboxAttemptCount,
		&state.inboxLeaseUntil, &state.inboxUpdatedAt,
		&state.inboxResponseIsNil, &state.inboxCompletedIsNil, &state.inboxLastErrorIsNil,
	); err != nil {
		t.Fatalf("read inbox integrity state: %v", err)
	}
	if err := database.QueryRow(`
		SELECT count(*) FROM notification_outbox WHERE attempt_id IN ($1,$2)`,
		uuid.UUID(firstID), uuid.UUID(secondID),
	).Scan(&state.outboxCount); err != nil {
		t.Fatalf("read outbox integrity state: %v", err)
	}
	return state
}
