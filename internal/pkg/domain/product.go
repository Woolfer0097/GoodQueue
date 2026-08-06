package domain

import (
	"fmt"

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
	ID                ProductID
	Title             string
	Description       string
	ImageURL          string
	QueueEnabled      bool
	AllocatableStock  int32
	Reserved          int32
	NextQueueSequence int64
	WaitingCount      int64
	WaitingCapacity   int64
}

func (product Product) FreeStock() int32 {
	if product.Reserved >= product.AllocatableStock {
		return 0
	}
	return product.AllocatableStock - product.Reserved
}
