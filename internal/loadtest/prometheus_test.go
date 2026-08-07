package loadtest

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Woolfer0097/GoodQueue/internal/pkg/domain"
)

func TestPrometheusClientRequestSuccessStats(t *testing.T) {
	queries := make([]string, 0, 2)
	evaluationTimes := make([]string, 0, 2)
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		queries = append(queries, request.URL.Query().Get("query"))
		evaluationTimes = append(evaluationTimes, request.URL.Query().Get("time"))
		value := "125.5"
		if strings.Contains(queries[len(queries)-1], `expected_response="true"`) {
			value = "100.4"
		}
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{"status":"success","data":{"resultType":"vector","result":[{"metric":{},"value":[1786045000,"` + value + `"]}]}}`))
	}))
	defer server.Close()

	stats, err := NewPrometheusClient(server.URL, server.Client()).RequestSuccessStats(context.Background(), 30*time.Minute)
	if err != nil {
		t.Fatalf("request success stats: %v", err)
	}
	if stats.Total != 125.5 || stats.Successful != 100.4 {
		t.Fatalf("unexpected stats: %+v", stats)
	}
	if len(queries) != 2 || !strings.Contains(queries[0], `last_over_time(`) ||
		!strings.Contains(queries[0], `[1800s]`) || !strings.Contains(queries[0], `offset 1800s`) ||
		!strings.Contains(queries[0], loadtestScenarioSelector) || strings.Contains(queries[0], "expected_response") ||
		!strings.Contains(queries[1], `expected_response="true"`) {
		t.Fatalf("unexpected PromQL queries: %#v", queries)
	}
	if len(evaluationTimes) != 2 || evaluationTimes[0] == "" || evaluationTimes[0] != evaluationTimes[1] {
		t.Fatalf("queries used different evaluation times: %#v", evaluationTimes)
	}
}

func TestRequestCountQueryPreservesSingleSampleSeries(t *testing.T) {
	query := requestCountQuery(`loadtest_scenario="queue_join_polling"`, "1800s")
	for _, required := range []string{"last_over_time", "offset 1800s", "or", "* 0"} {
		if !strings.Contains(query, required) {
			t.Fatalf("query %q does not contain %q", query, required)
		}
	}
}

func TestPrometheusClientPurchaseSuccessStats(t *testing.T) {
	queries := make([]string, 0, 3)
	evaluationTimes := make([]string, 0, 3)
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		query := request.URL.Query().Get("query")
		queries = append(queries, query)
		evaluationTimes = append(evaluationTimes, request.URL.Query().Get("time"))
		value := "6"
		if strings.Contains(query, "cancelled_outcomes") {
			value = "3"
		}
		if strings.Contains(query, "checkout_expired_outcomes") {
			value = "1"
		}
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{"status":"success","data":{"resultType":"vector","result":[{"metric":{},"value":[1786045000,"` + value + `"]}]}}`))
	}))
	defer server.Close()

	stats, err := NewPrometheusClient(server.URL, server.Client()).PurchaseSuccessStats(context.Background(), 30*time.Minute)
	if err != nil {
		t.Fatalf("purchase success stats: %v", err)
	}
	if stats.Purchased != 6 || stats.Cancelled != 3 || stats.CheckoutExpired != 1 {
		t.Fatalf("unexpected stats: %+v", stats)
	}
	for index, metric := range []string{
		"k6_purchased_outcomes_total", "k6_cancelled_outcomes_total", "k6_checkout_expired_outcomes_total",
	} {
		if !strings.Contains(queries[index], metric) || !strings.Contains(queries[index], purchaseScenarioSelector) {
			t.Fatalf("unexpected PromQL query: %q", queries[index])
		}
		if evaluationTimes[index] == "" || evaluationTimes[index] != evaluationTimes[0] {
			t.Fatalf("queries used different evaluation times: %#v", evaluationTimes)
		}
	}
}

func TestPrometheusClientMapsDependencyFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer server.Close()

	_, err := NewPrometheusClient(server.URL, server.Client()).RequestSuccessStats(context.Background(), time.Minute)
	if !errors.Is(err, domain.ErrMetricsUnavailable) {
		t.Fatalf("expected metrics unavailable, got %v", err)
	}
}
