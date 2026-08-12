package loadtestrunner

import (
	"net/http"
	"sync"
	"time"

	"github.com/Woolfer0097/GoodQueue/internal/loadtest"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

type Metrics struct {
	registry     *prometheus.Registry
	running      prometheus.Gauge
	info         *prometheus.GaugeVec
	currentRun   *prometheus.GaugeVec
	seedInfo     *prometheus.GaugeVec
	runs         *prometheus.CounterVec
	verifiers    *prometheus.CounterVec
	lastDuration prometheus.Gauge
	startedAt    prometheus.Gauge
	finishedAt   prometheus.Gauge
	lastVerifier prometheus.Gauge
	violations   *prometheus.GaugeVec
	events       *prometheus.CounterVec

	mu              sync.RWMutex
	started         time.Time
	finishedElapsed time.Duration
	active          bool
}

func NewMetrics() *Metrics {
	metrics := &Metrics{
		registry:     prometheus.NewRegistry(),
		running:      prometheus.NewGauge(prometheus.GaugeOpts{Name: "goodqueue_loadtest_running", Help: "Whether a load test is active."}),
		info:         prometheus.NewGaugeVec(prometheus.GaugeOpts{Name: "goodqueue_loadtest_info", Help: "Current load-test profile, scenario, and status."}, []string{"profile", "scenario", "status"}),
		currentRun:   prometheus.NewGaugeVec(prometheus.GaugeOpts{Name: "goodqueue_loadtest_current_run_info", Help: "Current UI-triggered load-test run."}, []string{"run_id", "profile", "scenario"}),
		seedInfo:     prometheus.NewGaugeVec(prometheus.GaugeOpts{Name: "goodqueue_loadtest_seed_info", Help: "Current load-test seed status."}, []string{"status"}),
		runs:         prometheus.NewCounterVec(prometheus.CounterOpts{Name: "goodqueue_loadtest_runs_total", Help: "Completed load-test runs by result."}, []string{"profile", "scenario", "result"}),
		verifiers:    prometheus.NewCounterVec(prometheus.CounterOpts{Name: "goodqueue_loadtest_verifier_total", Help: "Verifier executions by result."}, []string{"result"}),
		lastDuration: prometheus.NewGauge(prometheus.GaugeOpts{Name: "goodqueue_loadtest_last_duration_seconds", Help: "Duration of the last load-test run."}),
		startedAt:    prometheus.NewGauge(prometheus.GaugeOpts{Name: "goodqueue_loadtest_started_timestamp_seconds", Help: "Unix timestamp when the current or last load test started."}),
		finishedAt:   prometheus.NewGauge(prometheus.GaugeOpts{Name: "goodqueue_loadtest_finished_timestamp_seconds", Help: "Unix timestamp when the last load test finished."}),
		lastVerifier: prometheus.NewGauge(prometheus.GaugeOpts{Name: "goodqueue_loadtest_last_verifier_success", Help: "Whether the last verifier execution passed."}),
		violations:   prometheus.NewGaugeVec(prometheus.GaugeOpts{Name: "goodqueue_loadtest_last_verifier_violations", Help: "Violation count from the last verifier result."}, []string{"check"}),
		events:       prometheus.NewCounterVec(prometheus.CounterOpts{Name: "goodqueue_loadtest_events_total", Help: "Load-test lifecycle events used for annotations."}, []string{"event"}),
	}
	elapsed := prometheus.NewGaugeFunc(prometheus.GaugeOpts{Name: "goodqueue_loadtest_elapsed_seconds", Help: "Elapsed seconds for the active or last load test."}, metrics.elapsedSeconds)
	metrics.registry.MustRegister(metrics.running, metrics.info, metrics.currentRun, metrics.seedInfo, metrics.runs, metrics.verifiers, metrics.lastDuration, metrics.startedAt, metrics.finishedAt, elapsed, metrics.lastVerifier, metrics.violations, metrics.events)
	metrics.setInfo("none", "none", StatusIdle)
	metrics.setSeedStatus("pending")
	return metrics
}

func (metrics *Metrics) setSeedStatus(status string) {
	metrics.seedInfo.Reset()
	metrics.seedInfo.WithLabelValues(status).Set(1)
}

func (metrics *Metrics) setCurrentRun(runID, profile, scenario string) {
	metrics.currentRun.Reset()
	metrics.currentRun.WithLabelValues(runID, profile, scenario).Set(1)
}

func (metrics *Metrics) Handler() http.Handler {
	return promhttp.HandlerFor(metrics.registry, promhttp.HandlerOpts{})
}

func (metrics *Metrics) setInfo(profile, scenario, status string) {
	metrics.info.Reset()
	metrics.info.WithLabelValues(profile, scenario, status).Set(1)
}

func (metrics *Metrics) recordStarted(profile, scenario string, started time.Time) {
	metrics.mu.Lock()
	metrics.started, metrics.finishedElapsed, metrics.active = started, 0, true
	metrics.mu.Unlock()
	metrics.running.Set(1)
	metrics.startedAt.Set(float64(started.Unix()))
	metrics.events.WithLabelValues("LOAD TEST START").Inc()
	metrics.setInfo(profile, scenario, StatusRunning)
}

func (metrics *Metrics) recordFinished(profile, scenario, result string, duration time.Duration) {
	metrics.mu.Lock()
	finished := metrics.started.Add(duration)
	metrics.finishedElapsed, metrics.active = duration, false
	metrics.mu.Unlock()
	metrics.running.Set(0)
	metrics.lastDuration.Set(duration.Seconds())
	metrics.finishedAt.Set(float64(finished.Unix()))
	metrics.runs.WithLabelValues(profile, scenario, result).Inc()
	metrics.events.WithLabelValues("LOAD TEST END").Inc()
}

func (metrics *Metrics) elapsedSeconds() float64 {
	metrics.mu.RLock()
	defer metrics.mu.RUnlock()
	if metrics.active {
		return time.Since(metrics.started).Seconds()
	}
	return metrics.finishedElapsed.Seconds()
}

func (metrics *Metrics) recordVerification(result loadtest.VerificationResult) {
	metrics.violations.Reset()
	for _, check := range result.Checks {
		metrics.violations.WithLabelValues(check.Name).Set(float64(len(check.Violations)))
	}
}

func (metrics *Metrics) clearVerification() {
	metrics.violations.Reset()
}

func (metrics *Metrics) recordVerifierResult(passed bool) {
	label, value, event := "fail", float64(0), "VERIFIER FAIL"
	if passed {
		label, value, event = "pass", 1, "VERIFIER PASS"
	}
	metrics.lastVerifier.Set(value)
	metrics.verifiers.WithLabelValues(label).Inc()
	metrics.events.WithLabelValues(event).Inc()
}
