package mockapi

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/Woolfer0097/GoodQueue/internal/pkg/domain"
	"github.com/google/uuid"
)

func TestFixtureScenarios(t *testing.T) {
	services := NewServices(10*time.Minute, 5*time.Minute)
	tests := []struct {
		product string
		user    string
		state   domain.QueueAttemptState
	}{
		{ProductScarceID, DemoUserOneID, domain.QueueAttemptCheckout},
		{ProductScarceID, DemoUserTwoID, domain.QueueAttemptWaiting},
		{ProductScarceID, DemoUserThreeID, domain.QueueAttemptWaiting},
		{ProductPopularID, DemoUserOneID, domain.QueueAttemptInvited},
		{ProductPopularID, DemoUserTwoID, domain.QueueAttemptPurchased},
		{ProductPopularID, DemoUserThreeID, domain.QueueAttemptCancelled},
		{ProductPopularID, DemoUserFourID, domain.QueueAttemptInviteExpired},
		{ProductPopularID, DemoUserFiveID, domain.QueueAttemptCheckoutExpired},
	}
	for _, test := range tests {
		t.Run(test.product+"/"+test.user, func(t *testing.T) {
			result, err := services.Queue.Current(context.Background(), productID(test.product), userID(test.user))
			if err != nil {
				t.Fatalf("current: %v", err)
			}
			if result.Attempt.State != test.state {
				t.Fatalf("state: got %s, want %s", result.Attempt.State, test.state)
			}
		})
	}

	first, _ := services.Queue.Current(context.Background(), productID(ProductScarceID), userID(DemoUserTwoID))
	second, _ := services.Queue.Current(context.Background(), productID(ProductScarceID), userID(DemoUserThreeID))
	if first.PositionAhead != 0 || second.PositionAhead != 1 || first.TotalWaiting != 2 || second.TotalWaiting != 2 {
		t.Fatalf("unexpected waiting positions: first=%+v second=%+v", first, second)
	}
	invited, _ := services.Queue.Current(context.Background(), productID(ProductPopularID), userID(DemoUserOneID))
	checkout, _ := services.Queue.Current(context.Background(), productID(ProductScarceID), userID(DemoUserOneID))
	if invited.Attempt.InvitationDeadline == nil || !invited.Attempt.InvitationDeadline.After(time.Now()) ||
		checkout.Attempt.CheckoutDeadline == nil || !checkout.Attempt.CheckoutDeadline.After(time.Now()) {
		t.Fatal("active fixture deadlines must be in the future")
	}
}

func TestStatefulJoinReplayCancelAndCheckout(t *testing.T) {
	services := NewServices(10*time.Minute, 5*time.Minute)
	ctx := context.Background()
	newUser := userID("aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa")
	key := domain.IdempotencyKey("join-new-user")
	created, err := services.Queue.Join(ctx, productID(ProductScarceID), newUser, key)
	if err != nil {
		t.Fatalf("join: %v", err)
	}
	if !created.Created || created.Attempt.State != domain.QueueAttemptWaiting || created.PositionAhead != 2 || created.TotalWaiting != 3 {
		t.Fatalf("unexpected created attempt: %+v", created)
	}
	replayed, err := services.Queue.Join(ctx, productID(ProductScarceID), newUser, key)
	if err != nil || replayed.Created || replayed.Attempt.ID != created.Attempt.ID {
		t.Fatalf("unexpected replay: result=%+v err=%v", replayed, err)
	}
	if err := services.Queue.Leave(ctx, productID(ProductScarceID), newUser); err != nil {
		t.Fatalf("leave: %v", err)
	}
	current, err := services.Queue.Current(ctx, productID(ProductScarceID), newUser)
	if err != nil || current.Attempt.State != domain.QueueAttemptCancelled || current.TotalWaiting != 2 {
		t.Fatalf("current after leave: result=%+v err=%v", current, err)
	}

	invited, _ := services.Queue.Current(ctx, productID(ProductPopularID), userID(DemoUserOneID))
	started, err := services.Checkout.Start(ctx, invited.Attempt.ID, userID(DemoUserOneID))
	if err != nil || started.State != domain.QueueAttemptCheckout || started.CheckoutDeadline == nil {
		t.Fatalf("start checkout: attempt=%+v err=%v", started, err)
	}
	replayedCheckout, err := services.Checkout.Start(ctx, invited.Attempt.ID, userID(DemoUserOneID))
	if err != nil || replayedCheckout.ID != started.ID {
		t.Fatalf("replay checkout: attempt=%+v err=%v", replayedCheckout, err)
	}
}

func TestPurchasedUserCanStartAnotherAttempt(t *testing.T) {
	services := NewServices(10*time.Minute, 5*time.Minute)
	ctx := context.Background()
	product := productID(ProductPopularID)
	user := userID(DemoUserTwoID)

	previous, err := services.Queue.Current(ctx, product, user)
	if err != nil || previous.Attempt.State != domain.QueueAttemptPurchased {
		t.Fatalf("fixture purchase: result=%+v err=%v", previous, err)
	}

	rejoined, err := services.Queue.Join(ctx, product, user, "repeat-purchase")
	if err != nil {
		t.Fatalf("rejoin after purchase: %v", err)
	}
	if !rejoined.Created || rejoined.Attempt.ID == previous.Attempt.ID || rejoined.Attempt.State != domain.QueueAttemptCheckout {
		t.Fatalf("unexpected repeat attempt: %+v", rejoined)
	}

	replay, err := services.Queue.Join(ctx, product, user, "repeat-purchase")
	if err != nil || replay.Created || replay.Attempt.ID != rejoined.Attempt.ID {
		t.Fatalf("repeat attempt replay: result=%+v err=%v", replay, err)
	}
}

func TestScenarioErrors(t *testing.T) {
	services := NewServices(10*time.Minute, 5*time.Minute)
	ctx := context.Background()
	user := userID("aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa")
	if _, err := services.Queue.Join(ctx, productID(ProductSoldOutID), user, "sold-out"); !errors.Is(err, domain.ErrOutOfStock) {
		t.Fatalf("sold-out join: %v", err)
	}
	if _, err := services.Queue.Join(ctx, productID(ProductDisabledID), user, "disabled"); !errors.Is(err, domain.ErrQueueDisabled) {
		t.Fatalf("disabled join: %v", err)
	}
	if _, err := services.Products.Get(ctx, productID("99999999-9999-4999-8999-999999999999")); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("unknown product: %v", err)
	}
}

func TestConcurrentJoinsRespectMockWaitingCapacity(t *testing.T) {
	services := NewServices(10*time.Minute, 5*time.Minute)
	ctx := context.Background()
	var wait sync.WaitGroup
	errorsChannel := make(chan error, 20)
	for index := 0; index < 20; index++ {
		wait.Add(1)
		go func(index int) {
			defer wait.Done()
			user := domain.ExternalUserID(uuid.New().String())
			_, err := services.Queue.Join(ctx, productID(ProductScarceID), user, domain.IdempotencyKey(fmt.Sprintf("join-%d", index)))
			errorsChannel <- err
		}(index)
	}
	wait.Wait()
	close(errorsChannel)
	created, full := 0, 0
	for err := range errorsChannel {
		switch {
		case err == nil:
			created++
		case errors.Is(err, domain.ErrQueueFull):
			full++
		default:
			t.Fatalf("unexpected concurrent join error: %v", err)
		}
	}
	if created != 8 || full != 12 {
		t.Fatalf("concurrent results: created=%d full=%d", created, full)
	}
	product, err := services.Products.Get(ctx, productID(ProductScarceID))
	if err != nil || product.WaitingCount != product.WaitingCapacity {
		t.Fatalf("waiting capacity invariant: product=%+v err=%v", product, err)
	}
}
