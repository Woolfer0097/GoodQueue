package worker

import (
	"context"
	"errors"
	"fmt"
	"runtime"
	"sync"
	"time"

	"github.com/Woolfer0097/GoodQueue/internal/pkg/domain"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

const loggedErrorLimit = 500

type Reconciler interface {
	ReconcileNextProduct(context.Context, int, []domain.ProductID) (domain.ReconciliationResult, error)
}

type Outbox interface {
	ClaimNotification(context.Context, time.Duration) (*domain.NotificationClaim, error)
	ClassifyInvitation(context.Context, domain.NotificationClaim) (bool, error)
	MarkNotificationSent(context.Context, domain.NotificationClaim) error
	MarkNotificationObsolete(context.Context, domain.NotificationClaim) error
	RetryNotification(context.Context, domain.NotificationClaim, error, time.Duration, time.Duration) (time.Duration, error)
}

type NotificationPublisher interface {
	Publish(context.Context, domain.NotificationClaim) error
}

type Observer interface {
	ObserveReconciliation(transitions int, outcome string, duration time.Duration)
	ObserveOutbox(outcome string, duration time.Duration)
}

type NoopObserver struct{}

func (NoopObserver) ObserveReconciliation(int, string, time.Duration) {}
func (NoopObserver) ObserveOutbox(string, time.Duration)              {}

type InMemoryObserver struct {
	mu       sync.Mutex
	Counters map[string]int64
}

func NewInMemoryObserver() *InMemoryObserver {
	return &InMemoryObserver{Counters: make(map[string]int64)}
}

func (observer *InMemoryObserver) ObserveReconciliation(transitions int, outcome string, _ time.Duration) {
	observer.mu.Lock()
	defer observer.mu.Unlock()
	observer.Counters["reconciliation."+outcome]++
	observer.Counters["reconciliation.transitions"] += int64(transitions)
}

func (observer *InMemoryObserver) ObserveOutbox(outcome string, _ time.Duration) {
	observer.mu.Lock()
	defer observer.mu.Unlock()
	observer.Counters["outbox."+outcome]++
}

type Config struct {
	Interval                time.Duration
	ReconciliationBatchSize int
	MaxReconciledProducts   int
	MaxOutboxItems          int
	OutboxLeaseDuration     time.Duration
	OutboxRetryBase         time.Duration
	OutboxRetryMax          time.Duration
	PublisherTimeout        time.Duration
}

type Supervisor struct {
	config     Config
	reconciler Reconciler
	outbox     Outbox
	publisher  NotificationPublisher
	observer   Observer
	log        *zap.Logger
}

type cycleOutcome struct {
	immediate bool
	failed    bool
}

func NewSupervisor(
	config Config,
	reconciler Reconciler,
	outbox Outbox,
	publisher NotificationPublisher,
	observer Observer,
	log *zap.Logger,
) *Supervisor {
	return &Supervisor{config: config, reconciler: reconciler, outbox: outbox, publisher: publisher, observer: observer, log: log}
}

func (supervisor *Supervisor) Run(ctx context.Context) {
	var workers sync.WaitGroup
	workers.Add(2)
	go func() {
		defer workers.Done()
		supervisor.runReconciliationLoop(ctx)
	}()
	go func() {
		defer workers.Done()
		supervisor.runOutboxLoop(ctx)
	}()
	workers.Wait()
}

func (supervisor *Supervisor) runReconciliationLoop(ctx context.Context) {
	supervisor.runLoop(ctx, supervisor.reconcileCycle)
}

func (supervisor *Supervisor) reconcileCycle(ctx context.Context) cycleOutcome {
	excluded := make([]domain.ProductID, 0, supervisor.config.MaxReconciledProducts)
	moreWork := false
	for processed := 0; processed < supervisor.config.MaxReconciledProducts && ctx.Err() == nil; processed++ {
		started := time.Now()
		result, err := supervisor.reconciler.ReconcileNextProduct(ctx, supervisor.config.ReconciliationBatchSize, excluded)
		duration := time.Since(started)
		if err != nil {
			supervisor.observer.ObserveReconciliation(0, "error", duration)
			supervisor.log.Error("worker operation failed", zap.String("worker", "reconciliation"),
				zap.String("operation", "reconcile_product"), zap.String("outcome", "error"),
				zap.Duration("duration", duration), zap.String("error", boundedError(err)))
			return cycleOutcome{failed: true}
		}
		if result.ProductID == (domain.ProductID{}) {
			supervisor.observer.ObserveReconciliation(0, "empty", duration)
			return cycleOutcome{immediate: moreWork}
		}
		excluded = append(excluded, result.ProductID)
		moreWork = moreWork || result.MoreWork
		supervisor.observer.ObserveReconciliation(result.Transitions, "success", duration)
		supervisor.log.Info("worker operation completed", zap.String("worker", "reconciliation"),
			zap.String("operation", "reconcile_product"), zap.String("product_id", uuid.UUID(result.ProductID).String()),
			zap.Int("transitions", result.Transitions), zap.Bool("more_work", result.MoreWork),
			zap.String("outcome", "success"), zap.Duration("duration", duration))
	}
	return cycleOutcome{immediate: ctx.Err() == nil}
}

func (supervisor *Supervisor) runOutboxLoop(ctx context.Context) {
	supervisor.runLoop(ctx, supervisor.outboxCycle)
}

func (supervisor *Supervisor) runLoop(ctx context.Context, runCycle func(context.Context) cycleOutcome) {
	for {
		outcome := runCycle(ctx)
		if ctx.Err() != nil {
			return
		}
		if outcome.immediate && !outcome.failed {
			runtime.Gosched()
			continue
		}
		timer := time.NewTimer(supervisor.config.Interval)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}
	}
}

func (supervisor *Supervisor) outboxCycle(ctx context.Context) cycleOutcome {
	for item := 0; item < supervisor.config.MaxOutboxItems && ctx.Err() == nil; item++ {
		started := time.Now()
		claim, err := supervisor.outbox.ClaimNotification(ctx, supervisor.config.OutboxLeaseDuration)
		if err != nil {
			supervisor.observeOutboxError("claim_error", started, nil, err)
			return cycleOutcome{failed: true}
		}
		if claim == nil {
			supervisor.observer.ObserveOutbox("empty", time.Since(started))
			return cycleOutcome{}
		}
		supervisor.observer.ObserveOutbox("claimed", time.Since(started))
		supervisor.deliverNotification(ctx, *claim, started)
	}
	return cycleOutcome{immediate: ctx.Err() == nil}
}

func (supervisor *Supervisor) deliverNotification(ctx context.Context, claim domain.NotificationClaim, started time.Time) {
	obsolete, err := supervisor.outbox.ClassifyInvitation(ctx, claim)
	if err != nil {
		supervisor.retryNotification(ctx, claim, err, started)
		return
	}
	if obsolete {
		err = supervisor.outbox.MarkNotificationObsolete(ctx, claim)
		supervisor.observeFinalize(claim, "obsolete", started, err)
		return
	}

	publicationDeadline := earlierTime(
		time.Now().Add(supervisor.config.PublisherTimeout),
		claim.LeaseUntil.Add(-supervisor.config.PublisherTimeout),
	)
	if !publicationDeadline.After(time.Now()) {
		supervisor.retryNotification(ctx, claim, fmt.Errorf("safe publication window elapsed before publish"), started)
		return
	}
	publishContext, cancel := context.WithDeadline(ctx, publicationDeadline)
	err = supervisor.publisher.Publish(publishContext, claim)
	cancel()
	if err != nil {
		supervisor.retryNotification(ctx, claim, err, started)
		return
	}
	err = supervisor.outbox.MarkNotificationSent(ctx, claim)
	supervisor.observeFinalize(claim, "sent", started, err)
}

func (supervisor *Supervisor) retryNotification(ctx context.Context, claim domain.NotificationClaim, cause error, started time.Time) {
	backoff, err := supervisor.outbox.RetryNotification(ctx, claim, cause, supervisor.config.OutboxRetryBase, supervisor.config.OutboxRetryMax)
	if err != nil {
		supervisor.observeFinalize(claim, staleOutcome(err), started, err)
		return
	}
	supervisor.observer.ObserveOutbox("retry", time.Since(started))
	supervisor.log.Warn("notification publication scheduled for retry", notificationFields(claim,
		zap.String("outcome", "retry"), zap.Duration("backoff", backoff),
		zap.Duration("duration", time.Since(started)), zap.String("error", boundedError(cause)))...)
}

func (supervisor *Supervisor) observeFinalize(claim domain.NotificationClaim, outcome string, started time.Time, err error) {
	if err != nil {
		outcome = staleOutcome(err)
		supervisor.log.Error("notification finalization failed", notificationFields(claim,
			zap.String("outcome", outcome), zap.Duration("duration", time.Since(started)),
			zap.String("error", boundedError(err)))...)
	} else {
		supervisor.log.Info("notification delivery completed", notificationFields(claim,
			zap.String("outcome", outcome), zap.Duration("duration", time.Since(started)))...)
	}
	supervisor.observer.ObserveOutbox(outcome, time.Since(started))
}

func (supervisor *Supervisor) observeOutboxError(outcome string, started time.Time, claim *domain.NotificationClaim, err error) {
	fields := []zap.Field{zap.String("worker", "outbox"), zap.String("operation", "claim"),
		zap.String("outcome", outcome), zap.Duration("duration", time.Since(started)), zap.String("error", boundedError(err))}
	if claim != nil {
		fields = append(fields, notificationFields(*claim)...)
	}
	supervisor.log.Error("outbox worker operation failed", fields...)
	supervisor.observer.ObserveOutbox(outcome, time.Since(started))
}

func notificationFields(claim domain.NotificationClaim, extra ...zap.Field) []zap.Field {
	fields := []zap.Field{zap.String("worker", "outbox"), zap.String("operation", "deliver"),
		zap.String("outbox_id", claim.ID.String()), zap.String("event_type", claim.EventType),
		zap.String("deduplication_key", claim.DeduplicationKey), zap.Int("attempt_count", claim.AttemptCount),
		zap.Int64("lease_generation", claim.LeaseGeneration)}
	return append(fields, extra...)
}

func staleOutcome(err error) string {
	if errors.Is(err, domain.ErrStaleClaim) {
		return "stale_claim"
	}
	return "error"
}

func boundedError(err error) string {
	message := err.Error()
	if len(message) <= loggedErrorLimit {
		return message
	}
	return message[:loggedErrorLimit]
}

func earlierTime(first, second time.Time) time.Time {
	if first.Before(second) {
		return first
	}
	return second
}
