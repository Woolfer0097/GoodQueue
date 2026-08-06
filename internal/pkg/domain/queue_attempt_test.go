package domain

import (
	"errors"
	"math"
	"strings"
	"testing"

	"github.com/google/uuid"
)

func TestParseIdempotencyKey(t *testing.T) {
	key, err := ParseIdempotencyKey(" order:123_retry-1 ")
	if err != nil {
		t.Fatalf("parse idempotency key: %v", err)
	}
	if key != "order:123_retry-1" {
		t.Fatalf("unexpected key: %q", key)
	}
}

func TestWaitingCapacityRoundsUpWithoutOverflow(t *testing.T) {
	tests := []struct {
		stock   int32
		percent int
		want    int64
	}{
		{stock: 3, percent: 100, want: 3},
		{stock: 3, percent: 50, want: 2},
		{stock: 1, percent: 1, want: 1},
		{stock: math.MaxInt32, percent: 500, want: 10737418235},
	}
	for _, test := range tests {
		got, err := WaitingCapacity(test.stock, test.percent)
		if err != nil {
			t.Fatalf("capacity for stock=%d percent=%d: %v", test.stock, test.percent, err)
		}
		if got != test.want {
			t.Fatalf("capacity for stock=%d percent=%d: got %d, want %d", test.stock, test.percent, got, test.want)
		}
	}
}

func TestWaitingCapacityRejectsNegativeInputs(t *testing.T) {
	if _, err := WaitingCapacity(-1, 100); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("expected invalid stock error, got %v", err)
	}
	if _, err := WaitingCapacity(1, -1); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("expected invalid percent error, got %v", err)
	}
}

func TestCanonicalStockAdjustmentHashNormalizesFields(t *testing.T) {
	productID := ProductID(uuid.MustParse("AAAAAAAA-AAAA-4AAA-8AAA-AAAAAAAAAAAA"))
	left := CanonicalStockAdjustmentHash(StockAdjustmentCommand{
		ProductID: productID, Delta: 12, Reason: "  restock  ", ExternalReference: " ref-7 ",
	})
	right := CanonicalStockAdjustmentHash(StockAdjustmentCommand{
		ProductID: productID, Delta: 12, Reason: "restock", ExternalReference: "ref-7",
	})
	if left != right {
		t.Fatal("normalized adjustment fields produced different hashes")
	}
}

func TestCanonicalStockAdjustmentHashPreservesFieldBoundaries(t *testing.T) {
	productID := ProductID(uuid.MustParse("aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"))
	left := CanonicalStockAdjustmentHash(StockAdjustmentCommand{
		ProductID: productID, Delta: -3, Reason: "ab", ExternalReference: "c",
	})
	right := CanonicalStockAdjustmentHash(StockAdjustmentCommand{
		ProductID: productID, Delta: -3, Reason: "a", ExternalReference: "bc",
	})
	if left == right {
		t.Fatal("length-prefixed fields produced a boundary collision")
	}
}

func TestCanonicalStockAdjustmentHashChangesEveryField(t *testing.T) {
	base := StockAdjustmentCommand{
		ProductID: ProductID(uuid.MustParse("aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa")),
		Delta:     1, Reason: "restock", ExternalReference: "shipment-1",
	}
	baseHash := CanonicalStockAdjustmentHash(base)
	variants := []StockAdjustmentCommand{
		{ProductID: ProductID(uuid.MustParse("bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb")), Delta: 1, Reason: "restock", ExternalReference: "shipment-1"},
		{ProductID: base.ProductID, Delta: -1, Reason: "restock", ExternalReference: "shipment-1"},
		{ProductID: base.ProductID, Delta: 1, Reason: "return", ExternalReference: "shipment-1"},
		{ProductID: base.ProductID, Delta: 1, Reason: "restock", ExternalReference: "shipment-2"},
	}
	for _, variant := range variants {
		if CanonicalStockAdjustmentHash(variant) == baseHash {
			t.Fatal("changing a canonical field did not change the hash")
		}
	}
}

func TestParseIdempotencyKeyRejectsInvalidFormat(t *testing.T) {
	invalidKeys := []string{"", "contains spaces", "../path", strings.Repeat("a", 129)}
	for _, raw := range invalidKeys {
		if _, err := ParseIdempotencyKey(raw); !errors.Is(err, ErrInvalidInput) {
			t.Fatalf("expected invalid input for %q, got %v", raw, err)
		}
	}
}

func TestNextQueueSequenceFailsBeforeOverflow(t *testing.T) {
	if _, err := NextQueueSequence(math.MaxInt64); !errors.Is(err, ErrQueueSequenceExhausted) {
		t.Fatalf("expected sequence exhaustion, got %v", err)
	}
}
