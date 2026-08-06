package usecase

import (
	"context"

	"github.com/Woolfer0097/GoodQueue/internal/pkg/domain"
	"github.com/Woolfer0097/GoodQueue/internal/pkg/repository"
)

type DemoUserUseCase struct{ users repository.DemoUser }

func NewDemoUserUseCase(users repository.DemoUser) *DemoUserUseCase {
	return &DemoUserUseCase{users: users}
}

func (useCase *DemoUserUseCase) List(ctx context.Context) ([]domain.DemoUser, error) {
	return useCase.users.List(ctx)
}
