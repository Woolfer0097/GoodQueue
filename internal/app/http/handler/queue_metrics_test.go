package handler

import (
	"testing"
	"time"

	"github.com/Woolfer0097/GoodQueue/internal/pkg/domain"
	"github.com/google/uuid"
)

func TestParseMetricsWindow(t *testing.T) {
	for _, invalid := range []string{"bad", "59s", "2161h", "-1h"} {
		if _, err := parseMetricsWindow(invalid); err == nil {
			t.Fatalf("window %q must be rejected", invalid)
		}
	}
	if got, err := parseMetricsWindow("24h"); err != nil || got != 24*time.Hour {
		t.Fatalf("parse 24h: got %s, err %v", got, err)
	}
}

func TestQueueMetricsResponseUsesResolvedRightsForConversion(t *testing.T) {
	productID := domain.ProductID(uuid.MustParse("11111111-1111-1111-1111-111111111111"))
	response := queueMetricsResponse(domain.QueueBufferReport{
		WaitingBufferPercent: 100,
		Totals:               domain.QueueBufferMetrics{IssuedRights: 10, ActiveRights: 2, ResolvedRights: 8, Purchases: 6},
		Products: []domain.QueueBufferMetrics{{
			ProductID: productID, ProductTitle: "Limited", IssuedRights: 10,
			ActiveRights: 2, ResolvedRights: 8, Purchases: 6,
		}},
	})
	if response.Totals.ConversionPercent != 75 || response.Products[0].ConversionPercent != 75 {
		t.Fatalf("conversion must exclude active rights: %+v", response)
	}
}
