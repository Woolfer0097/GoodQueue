package observability

import (
	"context"
	"database/sql"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/Woolfer0097/GoodQueue/internal/adaptivequeue"
	"github.com/Woolfer0097/GoodQueue/internal/pkg/domain"
	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.uber.org/zap"
)

const businessCollectionTimeout = 2 * time.Second

type Registry struct {
	registry     *prometheus.Registry
	httpRequests *prometheus.CounterVec
}

func NewRegistry(reader BusinessSnapshotReader, log *zap.Logger) *Registry {
	if log == nil {
		log = zap.NewNop()
	}
	registry := prometheus.NewRegistry()
	httpRequests := prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "goodqueue_http_requests_total",
		Help: "GoodQueue backend HTTP requests by normalized route and status code.",
	}, []string{"method", "route", "status_code"})
	registry.MustRegister(httpRequests)
	if reader != nil {
		registry.MustRegister(newBusinessCollector(reader, log))
	}
	return &Registry{registry: registry, httpRequests: httpRequests}
}

func (metrics *Registry) Handler() http.Handler {
	return promhttp.HandlerFor(metrics.registry, promhttp.HandlerOpts{})
}

func (metrics *Registry) Middleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Next()
		route := c.FullPath()
		if route == "" || (!strings.HasPrefix(route, "/api/") && !strings.HasPrefix(route, "/internal/")) {
			return
		}
		metrics.httpRequests.WithLabelValues(c.Request.Method, route, strconv.Itoa(c.Writer.Status())).Inc()
	}
}

type BusinessSnapshot struct {
	AttemptCounts       map[string]float64
	CurrentCapacity     float64
	RecommendedCapacity float64
	CurrentPercent      float64
	RecommendedPercent  float64
}

type BusinessSnapshotReader interface {
	Read(context.Context) (BusinessSnapshot, error)
}

type WaitingBufferPercentSource interface {
	CurrentWaitingBufferPercent() int
}

type AdaptiveQueueSnapshotReader interface {
	Snapshot() adaptivequeue.Snapshot
}

type PostgreSQLBusinessSnapshotReader struct {
	database *sql.DB
	percent  WaitingBufferPercentSource
	adaptive AdaptiveQueueSnapshotReader
}

func NewPostgreSQLBusinessSnapshotReader(
	database *sql.DB,
	percent WaitingBufferPercentSource,
	adaptive AdaptiveQueueSnapshotReader,
) *PostgreSQLBusinessSnapshotReader {
	return &PostgreSQLBusinessSnapshotReader{database: database, percent: percent, adaptive: adaptive}
}

func (reader *PostgreSQLBusinessSnapshotReader) Read(ctx context.Context) (BusinessSnapshot, error) {
	snapshot := BusinessSnapshot{AttemptCounts: make(map[string]float64)}
	rows, err := reader.database.QueryContext(ctx, `SELECT state, count(*) FROM queue_attempts GROUP BY state`)
	if err != nil {
		return BusinessSnapshot{}, err
	}
	for rows.Next() {
		var state string
		var count float64
		if err := rows.Scan(&state, &count); err != nil {
			_ = rows.Close()
			return BusinessSnapshot{}, err
		}
		snapshot.AttemptCounts[state] = count
	}
	if err := rows.Close(); err != nil {
		return BusinessSnapshot{}, err
	}

	currentPercent := reader.percent.CurrentWaitingBufferPercent()
	recommendedPercent := currentPercent
	if reader.adaptive != nil {
		adaptiveSnapshot := reader.adaptive.Snapshot()
		if adaptiveSnapshot.TargetPercent != nil {
			recommendedPercent = *adaptiveSnapshot.TargetPercent
		}
	}
	snapshot.CurrentPercent = float64(currentPercent)
	snapshot.RecommendedPercent = float64(recommendedPercent)

	stockRows, err := reader.database.QueryContext(ctx, `SELECT allocatable_stock FROM products WHERE queue_enabled = TRUE`)
	if err != nil {
		return BusinessSnapshot{}, err
	}
	for stockRows.Next() {
		var stock int32
		if err := stockRows.Scan(&stock); err != nil {
			_ = stockRows.Close()
			return BusinessSnapshot{}, err
		}
		current, err := domain.WaitingCapacity(stock, currentPercent)
		if err != nil {
			_ = stockRows.Close()
			return BusinessSnapshot{}, err
		}
		recommended, err := domain.WaitingCapacity(stock, recommendedPercent)
		if err != nil {
			_ = stockRows.Close()
			return BusinessSnapshot{}, err
		}
		snapshot.CurrentCapacity += float64(current)
		snapshot.RecommendedCapacity += float64(recommended)
	}
	if err := stockRows.Close(); err != nil {
		return BusinessSnapshot{}, err
	}
	return snapshot, nil
}

type businessCollector struct {
	reader               BusinessSnapshotReader
	log                  *zap.Logger
	attempts             *prometheus.Desc
	currentCapacity      *prometheus.Desc
	recommendedCapacity  *prometheus.Desc
	currentPercent       *prometheus.Desc
	recommendedPercent   *prometheus.Desc
	collectionSuccessful *prometheus.Desc
}

func newBusinessCollector(reader BusinessSnapshotReader, log *zap.Logger) *businessCollector {
	return &businessCollector{
		reader: reader, log: log,
		attempts:             prometheus.NewDesc("goodqueue_queue_attempts", "Current queue attempt count by state.", []string{"state"}, nil),
		currentCapacity:      prometheus.NewDesc("goodqueue_queue_waiting_capacity", "Aggregated current waiting capacity.", nil, nil),
		recommendedCapacity:  prometheus.NewDesc("goodqueue_queue_recommended_waiting_capacity", "Aggregated adaptive target waiting capacity.", nil, nil),
		currentPercent:       prometheus.NewDesc("goodqueue_queue_waiting_buffer_percent", "Current waiting buffer percentage.", nil, nil),
		recommendedPercent:   prometheus.NewDesc("goodqueue_queue_recommended_buffer_percent", "Adaptive target waiting buffer percentage.", nil, nil),
		collectionSuccessful: prometheus.NewDesc("goodqueue_business_metrics_collection_success", "Whether the last business metric collection succeeded.", nil, nil),
	}
}

func (collector *businessCollector) Describe(output chan<- *prometheus.Desc) {
	output <- collector.attempts
	output <- collector.currentCapacity
	output <- collector.recommendedCapacity
	output <- collector.currentPercent
	output <- collector.recommendedPercent
	output <- collector.collectionSuccessful
}

func (collector *businessCollector) Collect(output chan<- prometheus.Metric) {
	ctx, cancel := context.WithTimeout(context.Background(), businessCollectionTimeout)
	defer cancel()
	snapshot, err := collector.reader.Read(ctx)
	if err != nil {
		collector.log.Warn("collect business metrics", zap.Error(err))
		output <- prometheus.MustNewConstMetric(collector.collectionSuccessful, prometheus.GaugeValue, 0)
		return
	}
	for _, state := range []string{"waiting", "invited", "checkout", "purchased"} {
		output <- prometheus.MustNewConstMetric(collector.attempts, prometheus.GaugeValue, snapshot.AttemptCounts[state], state)
	}
	output <- prometheus.MustNewConstMetric(collector.currentCapacity, prometheus.GaugeValue, snapshot.CurrentCapacity)
	output <- prometheus.MustNewConstMetric(collector.recommendedCapacity, prometheus.GaugeValue, snapshot.RecommendedCapacity)
	output <- prometheus.MustNewConstMetric(collector.currentPercent, prometheus.GaugeValue, snapshot.CurrentPercent)
	output <- prometheus.MustNewConstMetric(collector.recommendedPercent, prometheus.GaugeValue, snapshot.RecommendedPercent)
	output <- prometheus.MustNewConstMetric(collector.collectionSuccessful, prometheus.GaugeValue, 1)
}
