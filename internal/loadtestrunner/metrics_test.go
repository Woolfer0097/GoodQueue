package loadtestrunner

import (
	"strings"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus/testutil"
)

func TestMetricsExportTimestampsAndElapsedWithoutRunIDLabel(t *testing.T) {
	metrics := NewMetrics()
	started := time.Unix(1_700_000_000, 0)
	metrics.recordStarted("smoke", "queue_join_polling", started)
	metrics.recordFinished("smoke", "queue_join_polling", "success", 12*time.Second)

	want := `# HELP goodqueue_loadtest_elapsed_seconds Elapsed seconds for the active or last load test.
# TYPE goodqueue_loadtest_elapsed_seconds gauge
goodqueue_loadtest_elapsed_seconds 12
# HELP goodqueue_loadtest_finished_timestamp_seconds Unix timestamp when the last load test finished.
# TYPE goodqueue_loadtest_finished_timestamp_seconds gauge
goodqueue_loadtest_finished_timestamp_seconds 1.700000012e+09
# HELP goodqueue_loadtest_started_timestamp_seconds Unix timestamp when the current or last load test started.
# TYPE goodqueue_loadtest_started_timestamp_seconds gauge
goodqueue_loadtest_started_timestamp_seconds 1.7e+09
`
	if err := testutil.GatherAndCompare(metrics.registry, strings.NewReader(want),
		"goodqueue_loadtest_elapsed_seconds",
		"goodqueue_loadtest_finished_timestamp_seconds",
		"goodqueue_loadtest_started_timestamp_seconds",
	); err != nil {
		t.Fatal(err)
	}
}

func TestMetricsExposeOnlyCurrentRunID(t *testing.T) {
	metrics := NewMetrics()
	metrics.setCurrentRun("ui-first", "smoke", "queue_join_polling")
	metrics.setCurrentRun("ui-second", "medium", "purchase_outcomes")
	want := `# HELP goodqueue_loadtest_current_run_info Current UI-triggered load-test run.
# TYPE goodqueue_loadtest_current_run_info gauge
goodqueue_loadtest_current_run_info{profile="medium",run_id="ui-second",scenario="purchase_outcomes"} 1
`
	if err := testutil.GatherAndCompare(metrics.registry, strings.NewReader(want), "goodqueue_loadtest_current_run_info"); err != nil {
		t.Fatal(err)
	}
}
