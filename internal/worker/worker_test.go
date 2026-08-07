package worker

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/Woolfer0097/GoodQueue/internal/pkg/domain"
	"github.com/google/uuid"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
)

type fakeReconciler struct {
	mu           sync.Mutex
	results      []domain.ReconciliationResult
	resultErrors []error
	exclusions   [][]domain.ProductID
	calls        int
	err          error
}

func (fake *fakeReconciler) ReconcileNextProduct(
	_ context.Context,
	_ int,
	excluded []domain.ProductID,
) (domain.ReconciliationResult, error) {
	fake.mu.Lock()
	defer fake.mu.Unlock()
	fake.calls++
	fake.exclusions = append(fake.exclusions, append([]domain.ProductID(nil), excluded...))
	if fake.err != nil {
		return domain.ReconciliationResult{}, fake.err
	}
	if len(fake.results) == 0 {
		return domain.ReconciliationResult{}, nil
	}
	result := fake.results[0]
	fake.results = fake.results[1:]
	if len(fake.resultErrors) == 0 {
		return result, nil
	}
	err := fake.resultErrors[0]
	fake.resultErrors = fake.resultErrors[1:]
	return result, err
}

type fakeOutbox struct {
	mu           sync.Mutex
	claims       []*domain.NotificationClaim
	claimCalls   int
	claimErr     error
	obsolete     bool
	markedSent   int
	markedOld    int
	retries      int
	retryBackoff time.Duration
}

func (fake *fakeOutbox) ClaimNotification(context.Context, time.Duration) (*domain.NotificationClaim, error) {
	fake.mu.Lock()
	defer fake.mu.Unlock()
	fake.claimCalls++
	if fake.claimErr != nil {
		return nil, fake.claimErr
	}
	if len(fake.claims) == 0 {
		return nil, nil
	}
	claim := fake.claims[0]
	fake.claims = fake.claims[1:]
	return claim, nil
}

func (fake *fakeOutbox) ClassifyInvitation(context.Context, domain.NotificationClaim) (bool, error) {
	return fake.obsolete, nil
}

func (fake *fakeOutbox) MarkNotificationSent(context.Context, domain.NotificationClaim) error {
	fake.markedSent++
	return nil
}

func (fake *fakeOutbox) MarkNotificationObsolete(context.Context, domain.NotificationClaim) error {
	fake.markedOld++
	return nil
}

func (fake *fakeOutbox) RetryNotification(
	context.Context,
	domain.NotificationClaim,
	error,
	time.Duration,
	time.Duration,
) (time.Duration, error) {
	fake.retries++
	return fake.retryBackoff, nil
}

type fakePublisher struct {
	calls    int
	err      error
	deadline time.Time
}

func (fake *fakePublisher) Publish(ctx context.Context, _ domain.NotificationClaim) error {
	fake.calls++
	fake.deadline, _ = ctx.Deadline()
	return fake.err
}

func TestSupervisorRunsStartupCatchupAndStopsPromptly(t *testing.T) {
	reconciler := &fakeReconciler{}
	supervisor := testSupervisor(reconciler, &fakeOutbox{}, &fakePublisher{})
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		supervisor.Run(ctx)
		close(done)
	}()

	deadline := time.After(time.Second)
	for {
		reconciler.mu.Lock()
		calls := reconciler.calls
		reconciler.mu.Unlock()
		if calls > 0 {
			break
		}
		select {
		case <-deadline:
			t.Fatal("startup reconciliation did not run")
		default:
			time.Sleep(time.Millisecond)
		}
	}
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("supervisor did not stop after cancellation")
	}
}

func TestObsoleteInvitationSkipsPublisher(t *testing.T) {
	claim := notificationClaim("queue.invited")
	outbox := &fakeOutbox{claims: []*domain.NotificationClaim{&claim}, obsolete: true}
	publisher := &fakePublisher{}
	testSupervisor(&fakeReconciler{}, outbox, publisher).outboxCycle(context.Background())
	if publisher.calls != 0 || outbox.markedOld != 1 {
		t.Fatalf("obsolete delivery: publisher=%d obsolete=%d", publisher.calls, outbox.markedOld)
	}
}

func TestCompensationPublishesAndMarksSent(t *testing.T) {
	claim := notificationClaim("payment.compensation_required")
	outbox := &fakeOutbox{claims: []*domain.NotificationClaim{&claim}}
	publisher := &fakePublisher{}
	testSupervisor(&fakeReconciler{}, outbox, publisher).outboxCycle(context.Background())
	if publisher.calls != 1 || outbox.markedSent != 1 {
		t.Fatalf("compensation delivery: publisher=%d sent=%d", publisher.calls, outbox.markedSent)
	}
}

func TestPublicationFailureSchedulesRetry(t *testing.T) {
	claim := notificationClaim("payment.compensation_required")
	outbox := &fakeOutbox{claims: []*domain.NotificationClaim{&claim}, retryBackoff: time.Minute}
	publisher := &fakePublisher{err: errors.New("demo failure")}
	testSupervisor(&fakeReconciler{}, outbox, publisher).outboxCycle(context.Background())
	if publisher.calls != 1 || outbox.retries != 1 || outbox.markedSent != 0 {
		t.Fatalf("failed delivery: publisher=%d retries=%d sent=%d", publisher.calls, outbox.retries, outbox.markedSent)
	}
}

func TestReconcileCycleBoundsProductsAndExcludesProcessed(t *testing.T) {
	first := domain.ProductID(uuid.New())
	second := domain.ProductID(uuid.New())
	reconciler := &fakeReconciler{results: []domain.ReconciliationResult{
		{ProductID: first, Transitions: 1, MoreWork: true},
		{ProductID: second, Transitions: 1},
	}}
	supervisor := testSupervisor(reconciler, &fakeOutbox{}, &fakePublisher{})
	supervisor.config.MaxReconciledProducts = 2
	supervisor.reconcileCycle(context.Background())
	if reconciler.calls != 2 {
		t.Fatalf("reconciliation calls: got %d, want 2", reconciler.calls)
	}
}

func TestReconcileCycleSkipsFailedProductAndContinues(t *testing.T) {
	poisoned := domain.ProductID(uuid.New())
	healthy := domain.ProductID(uuid.New())
	reconciler := &fakeReconciler{
		results: []domain.ReconciliationResult{
			{ProductID: poisoned},
			{ProductID: healthy, Transitions: 1},
			{},
		},
		resultErrors: []error{errors.New("reserved invariant broken"), nil, nil},
	}
	supervisor := testSupervisor(reconciler, &fakeOutbox{}, &fakePublisher{})
	outcome := supervisor.reconcileCycle(context.Background())

	if reconciler.calls != 3 {
		t.Fatalf("reconciliation calls: got %d, want 3", reconciler.calls)
	}
	if len(reconciler.exclusions[1]) != 1 || reconciler.exclusions[1][0] != poisoned {
		t.Fatalf("failed product was not excluded: %#v", reconciler.exclusions[1])
	}
	if len(reconciler.exclusions[2]) != 2 || reconciler.exclusions[2][1] != healthy {
		t.Fatalf("processed products were not excluded: %#v", reconciler.exclusions[2])
	}
	if !outcome.failed {
		t.Fatal("cycle must report a product-scoped failure")
	}
}

func TestReconciliationImmediatelyRunsAnotherBoundedCycleWhenSaturated(t *testing.T) {
	productID := domain.ProductID(uuid.New())
	reconciler := &fakeReconciler{results: []domain.ReconciliationResult{
		{ProductID: productID, Transitions: 1},
	}}
	supervisor := testSupervisor(reconciler, &fakeOutbox{}, &fakePublisher{})
	supervisor.config.MaxReconciledProducts = 1

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		supervisor.runReconciliationLoop(ctx)
		close(done)
	}()
	waitForCalls(t, func() int {
		reconciler.mu.Lock()
		defer reconciler.mu.Unlock()
		return reconciler.calls
	}, 2)
	cancel()
	<-done
}

func TestOutboxImmediatelyRunsAnotherBoundedCycleWhenSaturated(t *testing.T) {
	claim := notificationClaim("payment.compensation_required")
	outbox := &fakeOutbox{claims: []*domain.NotificationClaim{&claim}}
	supervisor := testSupervisor(&fakeReconciler{}, outbox, &fakePublisher{})
	supervisor.config.MaxOutboxItems = 1

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		supervisor.runOutboxLoop(ctx)
		close(done)
	}()
	waitForCalls(t, func() int {
		outbox.mu.Lock()
		defer outbox.mu.Unlock()
		return outbox.claimCalls
	}, 2)
	cancel()
	<-done
}

func TestPersistentReconciliationErrorsWaitBetweenBoundedRetries(t *testing.T) {
	core, logs := observer.New(zap.ErrorLevel)
	reconciler := &fakeReconciler{err: errors.New("persistent reconciliation failure")}
	supervisor := testSupervisor(reconciler, &fakeOutbox{}, &fakePublisher{})
	supervisor.config.Interval = 20 * time.Millisecond
	supervisor.log = zap.New(core)

	ctx, cancel := context.WithTimeout(context.Background(), 75*time.Millisecond)
	defer cancel()
	supervisor.runReconciliationLoop(ctx)

	reconciler.mu.Lock()
	calls := reconciler.calls
	reconciler.mu.Unlock()
	if calls < 2 || calls > 5 || logs.Len() != calls {
		t.Fatalf("persistent error retries were not bounded: calls=%d logs=%d", calls, logs.Len())
	}
}

func TestPersistentClaimErrorsWaitBetweenBoundedRetries(t *testing.T) {
	core, logs := observer.New(zap.ErrorLevel)
	outbox := &fakeOutbox{claimErr: errors.New("persistent claim failure")}
	supervisor := testSupervisor(&fakeReconciler{}, outbox, &fakePublisher{})
	supervisor.config.Interval = 20 * time.Millisecond
	supervisor.log = zap.New(core)

	ctx, cancel := context.WithTimeout(context.Background(), 75*time.Millisecond)
	defer cancel()
	supervisor.runOutboxLoop(ctx)

	outbox.mu.Lock()
	calls := outbox.claimCalls
	outbox.mu.Unlock()
	if calls < 2 || calls > 5 || logs.Len() != calls {
		t.Fatalf("persistent claim retries were not bounded: calls=%d logs=%d", calls, logs.Len())
	}
}

func TestPublicationDeadlineLeavesPublisherTimeoutBeforeLeaseExpiry(t *testing.T) {
	claim := notificationClaim("payment.compensation_required")
	claim.LeaseUntil = time.Now().Add(500 * time.Millisecond)
	outbox := &fakeOutbox{claims: []*domain.NotificationClaim{&claim}}
	publisher := &fakePublisher{}
	supervisor := testSupervisor(&fakeReconciler{}, outbox, publisher)
	supervisor.config.PublisherTimeout = 100 * time.Millisecond

	supervisor.outboxCycle(context.Background())

	latestSafeDeadline := claim.LeaseUntil.Add(-supervisor.config.PublisherTimeout)
	if publisher.deadline.IsZero() || publisher.deadline.After(latestSafeDeadline) {
		t.Fatalf("unsafe publication deadline: got %s, latest safe %s", publisher.deadline, latestSafeDeadline)
	}
}

func testSupervisor(reconciler Reconciler, outbox Outbox, publisher NotificationPublisher) *Supervisor {
	return NewSupervisor(Config{
		Interval: time.Hour, ReconciliationBatchSize: 10, MaxReconciledProducts: 3,
		MaxOutboxItems: 3, OutboxLeaseDuration: time.Minute, OutboxRetryBase: time.Second,
		OutboxRetryMax: time.Minute, PublisherTimeout: time.Second,
	}, reconciler, outbox, publisher, NoopObserver{}, zap.NewNop())
}

func notificationClaim(eventType string) domain.NotificationClaim {
	return domain.NotificationClaim{
		ID: uuid.New(), EventType: eventType, DeduplicationKey: uuid.NewString(), AttemptCount: 1,
		LeaseUntil: time.Now().Add(time.Minute),
	}
}

func waitForCalls(t *testing.T, calls func() int, minimum int) {
	t.Helper()
	deadline := time.NewTimer(time.Second)
	defer deadline.Stop()
	for calls() < minimum {
		select {
		case <-deadline.C:
			t.Fatalf("timed out waiting for %d calls; got %d", minimum, calls())
		default:
			time.Sleep(time.Millisecond)
		}
	}
}
