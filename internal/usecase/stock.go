package usecase

import (
	"context"

	"github.com/Woolfer0097/GoodQueue/internal/pkg/domain"
	"github.com/Woolfer0097/GoodQueue/internal/pkg/repository"
)

type StockUseCase struct{ attempts repository.QueueAttempt }

func NewStockUseCase(attempts repository.QueueAttempt) *StockUseCase {
	return &StockUseCase{attempts: attempts}
}

func (useCase *StockUseCase) Adjust(ctx context.Context, command domain.StockAdjustmentCommand) (domain.StockAdjustmentResult, error) {
	return useCase.attempts.AdjustStock(ctx, command)
}
