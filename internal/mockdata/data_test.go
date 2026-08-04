package mockdata

import (
	"testing"

	"github.com/Woolfer0097/GoodQueue/internal/pkg/domain"
)

func TestProductsReturnsIndependentCopy(t *testing.T) {
	first := Products()
	first[0].Title = "changed"
	second := Products()
	if second[0].Title != "Лимитированная игровая приставка" {
		t.Fatalf("mock catalog was mutated: %+v", second[0])
	}
}

func TestQueueEntryReturnsIndependentPointers(t *testing.T) {
	first, err := QueueEntry(QueueStatusWaiting, domain.ProductID{}, "user-1")
	if err != nil {
		t.Fatalf("first queue entry: %v", err)
	}
	*first.Position = 100
	second, err := QueueEntry(QueueStatusWaiting, domain.ProductID{}, "user-1")
	if err != nil {
		t.Fatalf("second queue entry: %v", err)
	}
	if *second.Position != 3 {
		t.Fatalf("mock queue entry was mutated: %+v", second)
	}
}

func TestQueueEntryRejectsUnknownStatus(t *testing.T) {
	if _, err := QueueEntry("unknown", domain.ProductID{}, "user-1"); err == nil {
		t.Fatal("expected unknown mock queue status to fail")
	}
}
