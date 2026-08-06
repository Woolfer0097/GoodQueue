package repository

import (
	"context"

	"github.com/Woolfer0097/GoodQueue/internal/pkg/domain"
)

type DemoUser interface {
	List(context.Context) ([]domain.DemoUser, error)
}
