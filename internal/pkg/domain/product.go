package domain

import (
	"fmt"
	"time"

	"github.com/google/uuid"
)

type ProductID uuid.UUID

func ParseProductID(raw string) (ProductID, error) {
	value, err := uuid.Parse(raw)
	if err != nil {
		return ProductID{}, fmt.Errorf("%w: product ID must be a UUID", ErrInvalidInput)
	}
	return ProductID(value), nil
}

type Product struct {
	ID               ProductID
	Title            string
	Description      string
	ImageURL         string
	QueueEnabled     bool
	AllocatableStock int
	RightTTLSeconds  int
	PriceKopecks     int64
	CreatedAt        time.Time
	UpdatedAt        time.Time
}
