package worker

import (
	"context"
	"time"

	"github.com/Woolfer0097/GoodQueue/internal/pkg/domain"
	"github.com/Woolfer0097/GoodQueue/internal/pkg/repository"
	"github.com/Woolfer0097/GoodQueue/internal/usecase"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

type Worker struct {
	log            *zap.Logger
	interval       time.Duration
	products       repository.Product
	queue          *usecase.QueueUseCase
	purchaseRights repository.PurchaseRight
}

func New(log *zap.Logger, interval time.Duration, products repository.Product, queue *usecase.QueueUseCase, purchaseRights repository.PurchaseRight) *Worker {
	return &Worker{log: log, interval: interval, products: products, queue: queue, purchaseRights: purchaseRights}
}

func (worker *Worker) Run(ctx context.Context) {
	grantTicker := time.NewTicker(worker.interval)
	defer grantTicker.Stop()
	expireTicker := time.NewTicker(worker.interval)
	defer expireTicker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-grantTicker.C:
			worker.grantFreeSlots(ctx)
		case <-expireTicker.C:
			worker.expireOverdueRights(ctx)
		}
	}
}

func (worker *Worker) grantFreeSlots(ctx context.Context) {
	products, err := worker.products.List(ctx)
	if err != nil {
		worker.log.Error("worker: list products", zap.Error(err))
		return
	}

	for _, product := range products {
		if !product.QueueEnabled {
			continue
		}
		for {
			granted, err := worker.queue.TryGrantNext(ctx, product.ID)
			if err != nil {
				worker.log.Error("worker: grant next", zap.String("product_id", uuid.UUID(product.ID).String()), zap.Error(err))
				break
			}
			if !granted {
				break
			}
		}
	}
}

func (worker *Worker) expireOverdueRights(ctx context.Context) {
	expired, err := worker.purchaseRights.ListExpiredActiveRights(ctx, time.Now())
	if err != nil {
		worker.log.Error("worker: list expired active rights", zap.Error(err))
		return
	}

	for _, right := range expired {
		if err := worker.purchaseRights.ReleaseRight(ctx, right.QueueTicketID, domain.QueueEntryExpired); err != nil {
			worker.log.Error("worker: release expired right", zap.Int64("queue_ticket_id", right.QueueTicketID), zap.Error(err))
		}
	}
}
