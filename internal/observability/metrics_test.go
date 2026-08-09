package observability

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus/testutil"
)

type snapshotReaderStub struct {
	snapshot BusinessSnapshot
	err      error
}

func (stub snapshotReaderStub) Read(context.Context) (BusinessSnapshot, error) {
	return stub.snapshot, stub.err
}

func TestHTTPMetricsUseNormalizedRoute(t *testing.T) {
	gin.SetMode(gin.TestMode)
	metrics := NewRegistry(nil, nil)
	router := gin.New()
	router.Use(metrics.Middleware())
	router.GET("/api/v1/products/:productID", func(c *gin.Context) { c.Status(http.StatusNoContent) })
	router.POST("/api/v1/fail", func(c *gin.Context) { c.Status(http.StatusInternalServerError) })
	router.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/api/v1/products/a-random-id", nil))
	router.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodPost, "/api/v1/fail", nil))

	want := `# HELP goodqueue_http_requests_total GoodQueue backend HTTP requests by normalized route and status code.
# TYPE goodqueue_http_requests_total counter
goodqueue_http_requests_total{method="GET",route="/api/v1/products/:productID",status_code="204"} 1
goodqueue_http_requests_total{method="POST",route="/api/v1/fail",status_code="500"} 1
`
	if err := testutil.GatherAndCompare(metrics.registry, strings.NewReader(want), "goodqueue_http_requests_total"); err != nil {
		t.Fatal(err)
	}
}

func TestBusinessCollectorExportsAggregates(t *testing.T) {
	metrics := NewRegistry(snapshotReaderStub{snapshot: BusinessSnapshot{
		AttemptCounts:   map[string]float64{"waiting": 7, "checkout": 2},
		CurrentCapacity: 10, RecommendedCapacity: 12, CurrentPercent: 100, RecommendedPercent: 125,
	}}, nil)
	count, err := testutil.GatherAndCount(metrics.registry,
		"goodqueue_queue_attempts", "goodqueue_queue_waiting_capacity",
		"goodqueue_queue_recommended_waiting_capacity", "goodqueue_business_metrics_collection_success")
	if err != nil {
		t.Fatal(err)
	}
	if count != 7 {
		t.Fatalf("collected metric samples=%d, want 7", count)
	}
}

func TestBusinessCollectorReportsCollectionFailure(t *testing.T) {
	metrics := NewRegistry(snapshotReaderStub{err: errors.New("database unavailable")}, nil)
	want := `# HELP goodqueue_business_metrics_collection_success Whether the last business metric collection succeeded.
# TYPE goodqueue_business_metrics_collection_success gauge
goodqueue_business_metrics_collection_success 0
`
	if err := testutil.GatherAndCompare(metrics.registry, strings.NewReader(want), "goodqueue_business_metrics_collection_success"); err != nil {
		t.Fatal(err)
	}
}
