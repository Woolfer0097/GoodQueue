package loadtest

import (
	"context"
	"os"
	"testing"
	"time"
)

func TestIntegrationCleanupKeepsReportingTablesAndRemovesOnlyRun(t *testing.T) {
	databaseURL := os.Getenv("GOODQUEUE_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("GOODQUEUE_TEST_DATABASE_URL is not set")
	}
	config, err := LoadConfigFrom(func(key string) (string, bool) {
		values := map[string]string{
			"LOADTEST_DATABASE_URL":        databaseURL,
			"LOADTEST_RUN_ID":              "integration-report",
			"LOADTEST_SCENARIO":            "purchase_outcomes",
			"LOADTEST_USERS":               "3",
			"LOADTEST_PRODUCTS":            "3",
			"LOADTEST_PRODUCTS_PER_USER":   "1",
			"LOADTEST_CLEANUP_BEFORE_SEED": "true",
		}
		value, exists := values[key]
		return value, exists
	})
	if err != nil {
		t.Fatal(err)
	}
	data, err := GenerateData(config)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	connection, err := Connect(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = connection.Close(context.Background()) }()
	defer func() { _ = Cleanup(context.Background(), connection, config.RunID) }()

	if err := Seed(ctx, connection, config, data); err != nil {
		t.Fatal(err)
	}
	var runs, logs, planned int
	if err := connection.QueryRow(ctx, `
		SELECT (SELECT count(*) FROM loadtest.runs WHERE run_id=$1),
		       (SELECT count(*) FROM loadtest.request_logs WHERE run_id=$1),
		       (SELECT planned_purchase+planned_cancel+planned_ttl FROM loadtest.runs WHERE run_id=$1)`,
		config.RunID,
	).Scan(&runs, &logs, &planned); err != nil {
		t.Fatal(err)
	}
	if runs != 1 || logs != 3 || planned != 3 {
		t.Fatalf("seeded reports runs=%d logs=%d planned=%d", runs, logs, planned)
	}
	evaluation := outcomeEvaluation{Counts: VerificationCounts{QueueRejected: 3}}
	for _, user := range data.Users {
		for _, assignment := range user.Assignments {
			evaluation.Logs = append(evaluation.Logs, requestLogResult{
				UserID: user.ID, ProductID: assignment.ProductID,
				Operation:  "POST /api/v1/products/{productID}/queue-entries",
				HTTPStatus: int16TestPointer(409), FinalState: "queue_full", ActualOutcome: "queue_rejected",
			})
		}
	}
	if err := persistOutcomeEvaluation(ctx, connection, config.RunID, true, evaluation); err != nil {
		t.Fatal(err)
	}
	var completedLogs, actualRejected int
	var completed bool
	if err := connection.QueryRow(ctx, `
		SELECT status='completed' AND verification_passed,
		       actual_queue_rejected,
		       (SELECT count(*) FROM loadtest.request_logs
		        WHERE run_id=$1 AND actual_outcome='queue_rejected'
		          AND operation='POST /api/v1/products/{productID}/queue-entries'
		          AND http_status=409 AND completed_at IS NOT NULL)
		FROM loadtest.runs WHERE run_id=$1`, config.RunID,
	).Scan(&completed, &actualRejected, &completedLogs); err != nil {
		t.Fatal(err)
	}
	if !completed || actualRejected != 3 || completedLogs != 3 {
		t.Fatalf("persisted result completed=%t rejected=%d detailed_logs=%d", completed, actualRejected, completedLogs)
	}
	if err := Cleanup(ctx, connection, config.RunID); err != nil {
		t.Fatal(err)
	}
	var tablesExist bool
	if err := connection.QueryRow(ctx, `
		SELECT to_regclass('loadtest.runs') IS NOT NULL
		   AND to_regclass('loadtest.request_logs') IS NOT NULL
		   AND NOT EXISTS (SELECT 1 FROM loadtest.runs WHERE run_id=$1)`, config.RunID).Scan(&tablesExist); err != nil {
		t.Fatal(err)
	}
	if !tablesExist {
		t.Fatal("cleanup removed reporting tables or left run rows")
	}
}

func int16TestPointer(value int16) *int16 { return &value }
