package loadtest

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/Woolfer0097/GoodQueue/internal/pkg/domain"
)

const loadtestScenarioSelector = `loadtest_scenario=~"queue_join_polling|purchase_outcomes"`
const purchaseScenarioSelector = `loadtest_scenario="purchase_outcomes"`

type RequestSuccessStats struct {
	Successful float64
	Total      float64
}

type PurchaseSuccessStats struct {
	Purchased       float64
	Cancelled       float64
	CheckoutExpired float64
}

type PrometheusClient struct {
	baseURL string
	client  *http.Client
}

func NewPrometheusClient(baseURL string, client *http.Client) *PrometheusClient {
	if client == nil {
		client = http.DefaultClient
	}
	return &PrometheusClient{baseURL: strings.TrimRight(baseURL, "/"), client: client}
}

func (client *PrometheusClient) RequestSuccessStats(ctx context.Context, window time.Duration) (RequestSuccessStats, error) {
	windowSeconds := strconv.FormatInt(int64(math.Ceil(window.Seconds())), 10) + "s"
	totalQuery := requestCountQuery(loadtestScenarioSelector, windowSeconds)
	successQuery := requestCountQuery(loadtestScenarioSelector+`,expected_response="true"`, windowSeconds)
	evaluationTime := time.Now()

	total, err := client.scalarQuery(ctx, totalQuery, evaluationTime)
	if err != nil {
		return RequestSuccessStats{}, err
	}
	successful, err := client.scalarQuery(ctx, successQuery, evaluationTime)
	if err != nil {
		return RequestSuccessStats{}, err
	}
	if successful > total {
		successful = total
	}
	return RequestSuccessStats{Successful: successful, Total: total}, nil
}

func (client *PrometheusClient) PurchaseSuccessStats(ctx context.Context, window time.Duration) (PurchaseSuccessStats, error) {
	windowSeconds := strconv.FormatInt(int64(math.Ceil(window.Seconds())), 10) + "s"
	evaluationTime := time.Now()
	queries := []struct {
		metric string
		value  *float64
	}{
		{metric: "k6_purchased_outcomes_total"},
		{metric: "k6_cancelled_outcomes_total"},
		{metric: "k6_checkout_expired_outcomes_total"},
	}
	stats := PurchaseSuccessStats{}
	queries[0].value = &stats.Purchased
	queries[1].value = &stats.Cancelled
	queries[2].value = &stats.CheckoutExpired
	for _, item := range queries {
		value, err := client.scalarQuery(
			ctx,
			counterCountQuery(item.metric, purchaseScenarioSelector, windowSeconds),
			evaluationTime,
		)
		if err != nil {
			return PurchaseSuccessStats{}, err
		}
		*item.value = value
	}
	return stats, nil
}

// requestCountQuery uses the last counter value for series created inside the
// window and subtracts the value at the window boundary for older series. A
// plain increase() loses short-lived k6 series that only managed one remote
// write before the test ended.
func requestCountQuery(selector, window string) string {
	return counterCountQuery("k6_http_reqs_total", selector, window)
}

func counterCountQuery(metricName, selector, window string) string {
	metric := fmt.Sprintf(`%s{%s}`, metricName, selector)
	last := fmt.Sprintf(`last_over_time(%s[%s])`, metric, window)
	return fmt.Sprintf(`sum(%s - (%s offset %s or %s * 0)) or vector(0)`, last, metric, window, last)
}

func (client *PrometheusClient) scalarQuery(ctx context.Context, query string, evaluationTime time.Time) (float64, error) {
	endpoint, err := url.Parse(client.baseURL + "/api/v1/query")
	if err != nil {
		return 0, fmt.Errorf("%w: build Prometheus query URL: %w", domain.ErrMetricsUnavailable, err)
	}
	parameters := endpoint.Query()
	parameters.Set("query", query)
	parameters.Set("time", strconv.FormatFloat(float64(evaluationTime.UnixNano())/float64(time.Second), 'f', 3, 64))
	endpoint.RawQuery = parameters.Encode()

	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return 0, fmt.Errorf("%w: build Prometheus request: %w", domain.ErrMetricsUnavailable, err)
	}
	response, err := client.client.Do(request)
	if err != nil {
		return 0, fmt.Errorf("%w: query Prometheus: %w", domain.ErrMetricsUnavailable, err)
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode != http.StatusOK {
		_, _ = io.Copy(io.Discard, response.Body)
		return 0, fmt.Errorf("%w: Prometheus returned HTTP %d", domain.ErrMetricsUnavailable, response.StatusCode)
	}

	var payload prometheusQueryResponse
	if err := json.NewDecoder(response.Body).Decode(&payload); err != nil {
		return 0, fmt.Errorf("%w: decode Prometheus response: %w", domain.ErrMetricsUnavailable, err)
	}
	if payload.Status != "success" || payload.Data.ResultType != "vector" || len(payload.Data.Result) != 1 || len(payload.Data.Result[0].Value) != 2 {
		return 0, fmt.Errorf("%w: unexpected Prometheus response", domain.ErrMetricsUnavailable)
	}
	var rawValue string
	if err := json.Unmarshal(payload.Data.Result[0].Value[1], &rawValue); err != nil {
		return 0, fmt.Errorf("%w: decode Prometheus scalar: %w", domain.ErrMetricsUnavailable, err)
	}
	value, err := strconv.ParseFloat(rawValue, 64)
	if err != nil || math.IsNaN(value) || math.IsInf(value, 0) || value < 0 {
		return 0, fmt.Errorf("%w: invalid Prometheus scalar", domain.ErrMetricsUnavailable)
	}
	return value, nil
}

type prometheusQueryResponse struct {
	Status string `json:"status"`
	Data   struct {
		ResultType string `json:"resultType"`
		Result     []struct {
			Value []json.RawMessage `json:"value"`
		} `json:"result"`
	} `json:"data"`
}
