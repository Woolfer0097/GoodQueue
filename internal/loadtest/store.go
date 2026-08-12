package loadtest

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/jackc/pgx/v5"
)

func Connect(ctx context.Context, databaseURL string) (*pgx.Conn, error) {
	connection, err := pgx.Connect(ctx, databaseURL)
	if err != nil {
		return nil, fmt.Errorf("connect to PostgreSQL: %w", err)
	}
	if err := connection.Ping(ctx); err != nil {
		_ = connection.Close(ctx)
		return nil, fmt.Errorf("ping PostgreSQL: %w", err)
	}
	return connection, nil
}

func Seed(ctx context.Context, connection *pgx.Conn, config Config, data Data) error {
	prefix, err := RunPrefix(config.RunID)
	if err != nil {
		return err
	}
	transaction, err := connection.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin load-test seed transaction: %w", err)
	}
	defer func() { _ = transaction.Rollback(ctx) }()

	if config.CleanupBeforeSeed {
		if err := cleanupRun(ctx, transaction, config.RunID, prefix); err != nil {
			return err
		}
	} else {
		var attempts, reports int
		if err := transaction.QueryRow(ctx, `
			SELECT count(*)
			FROM queue_attempts qa
			JOIN products p ON p.id = qa.product_id
			WHERE left(p.title, char_length($1)) = $1`, prefix).Scan(&attempts); err != nil {
			return fmt.Errorf("check existing load-test attempts: %w", err)
		}
		if attempts > 0 {
			return fmt.Errorf("run %q already has %d attempts; use a new LOADTEST_RUN_ID or set LOADTEST_CLEANUP_BEFORE_SEED=true", config.RunID, attempts)
		}
		if err := transaction.QueryRow(ctx, `SELECT count(*) FROM loadtest.runs WHERE run_id = $1`, config.RunID).Scan(&reports); err != nil {
			return fmt.Errorf("check existing load-test report: %w", err)
		}
		if reports > 0 {
			return fmt.Errorf("run %q already exists in loadtest.runs; use a new LOADTEST_RUN_ID or set LOADTEST_CLEANUP_BEFORE_SEED=true", config.RunID)
		}
	}

	batch := &pgx.Batch{}
	for _, user := range data.Users {
		batch.Queue(`
			INSERT INTO users (name, external_user_id)
			VALUES ($1, $2::uuid)
			ON CONFLICT (external_user_id) DO UPDATE SET name = EXCLUDED.name`, user.Name, user.ID)
	}
	results := transaction.SendBatch(ctx, batch)
	for range data.Users {
		if _, err := results.Exec(); err != nil {
			_ = results.Close()
			return fmt.Errorf("batch insert load-test users: %w", err)
		}
	}
	if err := results.Close(); err != nil {
		return fmt.Errorf("close load-test user batch: %w", err)
	}

	batch = &pgx.Batch{}
	for _, product := range data.Products {
		batch.Queue(`
			INSERT INTO products (
				id, title, description, image_url, queue_enabled,
				allocatable_stock, reserved, right_ttl_seconds, next_queue_sequence
			) VALUES ($1::uuid, $2, $3, '', true, $4, 0, 600, 1)
			ON CONFLICT (id) DO UPDATE SET
				title = EXCLUDED.title,
				description = EXCLUDED.description,
				image_url = EXCLUDED.image_url,
				queue_enabled = true,
				allocatable_stock = EXCLUDED.allocatable_stock,
				reserved = 0,
				right_ttl_seconds = EXCLUDED.right_ttl_seconds,
				next_queue_sequence = 1,
				updated_at = clock_timestamp()`,
			product.ID, product.Title, "GoodQueue load-test product for run "+config.RunID, product.InitialStock,
		)
	}
	results = transaction.SendBatch(ctx, batch)
	for range data.Products {
		if _, err := results.Exec(); err != nil {
			_ = results.Close()
			return fmt.Errorf("batch insert load-test products: %w", err)
		}
	}
	if err := results.Close(); err != nil {
		return fmt.Errorf("close load-test product batch: %w", err)
	}

	effectiveConfig, err := json.Marshal(data.EffectiveConfig)
	if err != nil {
		return fmt.Errorf("marshal effective load-test config: %w", err)
	}
	plannedPurchase, plannedCancel, plannedTTL := plannedOutcomeCounts(data)
	if _, err := transaction.Exec(ctx, `
		INSERT INTO loadtest.runs (
			run_id, scenario, profile, source, keep_data, random_seed, effective_config,
			planned_purchase, planned_cancel, planned_ttl
		) VALUES ($1, $2, $3, $4, $5, $6, $7::jsonb, $8, $9, $10)`,
		config.RunID, config.Scenario, config.Profile, config.Source, config.KeepData, config.RandomSeed, effectiveConfig,
		plannedPurchase, plannedCancel, plannedTTL,
	); err != nil {
		return fmt.Errorf("insert loadtest.runs record (are load-test reporting migrations applied?): %w", err)
	}

	batch = &pgx.Batch{}
	for _, user := range data.Users {
		for _, assignment := range user.Assignments {
			var plannedOutcome any
			if assignment.PlannedOutcome != "" {
				plannedOutcome = assignment.PlannedOutcome
			}
			var paymentEventID any
			if assignment.PaymentEventID != "" {
				paymentEventID = assignment.PaymentEventID
			}
			batch.Queue(`
				INSERT INTO loadtest.request_logs (
					run_id, external_user_id, product_id, idempotency_key,
					planned_outcome, payment_event_id
				) VALUES ($1, $2::uuid, $3::uuid, $4, $5, $6)`,
				config.RunID, user.ID, assignment.ProductID, assignment.IdempotencyKey,
				plannedOutcome, paymentEventID,
			)
		}
	}
	results = transaction.SendBatch(ctx, batch)
	for _, user := range data.Users {
		for range user.Assignments {
			if _, err := results.Exec(); err != nil {
				_ = results.Close()
				return fmt.Errorf("batch insert loadtest.request_logs: %w", err)
			}
		}
	}
	if err := results.Close(); err != nil {
		return fmt.Errorf("close load-test request log batch: %w", err)
	}
	if err := transaction.Commit(ctx); err != nil {
		return fmt.Errorf("commit load-test seed transaction: %w", err)
	}
	return nil
}

func FindDisposableUIRun(ctx context.Context, connection *pgx.Conn) (string, error) {
	var runID string
	err := connection.QueryRow(ctx, `
		SELECT run_id
		FROM loadtest.runs
		WHERE source = 'runner_ui' AND keep_data = FALSE AND status = 'completed'
		ORDER BY completed_at DESC
		LIMIT 1`).Scan(&runID)
	if err == pgx.ErrNoRows {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("find disposable UI load-test run: %w", err)
	}
	return runID, nil
}

func PreserveFailedRun(ctx context.Context, connection *pgx.Conn, runID string) error {
	if _, err := RunPrefix(runID); err != nil {
		return err
	}
	_, err := connection.Exec(ctx, `
		UPDATE loadtest.runs SET
			status = 'failed',
			keep_data = TRUE,
			verification_passed = FALSE,
			completed_at = COALESCE(completed_at, clock_timestamp())
		WHERE run_id = $1 AND source = 'runner_ui'`, runID)
	if err != nil {
		return fmt.Errorf("preserve failed UI load-test run: %w", err)
	}
	return nil
}

func Cleanup(ctx context.Context, connection *pgx.Conn, runID string) error {
	prefix, err := RunPrefix(runID)
	if err != nil {
		return err
	}
	transaction, err := connection.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin load-test cleanup transaction: %w", err)
	}
	defer func() { _ = transaction.Rollback(ctx) }()
	if err := cleanupRun(ctx, transaction, runID, prefix); err != nil {
		return err
	}
	if err := transaction.Commit(ctx); err != nil {
		return fmt.Errorf("commit load-test cleanup transaction: %w", err)
	}
	return nil
}

func cleanupRun(ctx context.Context, transaction pgx.Tx, runID, prefix string) error {
	statements := []string{
		`DELETE FROM notification_outbox no USING queue_attempts qa, products p
		 WHERE no.attempt_id = qa.id AND qa.product_id = p.id AND left(p.title, char_length($1)) = $1`,
		`DELETE FROM payment_inbox pi USING queue_attempts qa, products p
		 WHERE pi.attempt_id = qa.id AND qa.product_id = p.id AND left(p.title, char_length($1)) = $1`,
		`DELETE FROM queue_attempts qa USING products p
		 WHERE qa.product_id = p.id AND left(p.title, char_length($1)) = $1`,
		`DELETE FROM inventory_adjustments ia USING products p
		 WHERE ia.product_id = p.id AND left(p.title, char_length($1)) = $1`,
		`DELETE FROM products WHERE left(title, char_length($1)) = $1`,
		`DELETE FROM users WHERE left(name, char_length($1)) = $1`,
	}
	for _, statement := range statements {
		if _, err := transaction.Exec(ctx, statement, prefix); err != nil {
			return fmt.Errorf("clean load-test run records: %w", err)
		}
	}
	if _, err := transaction.Exec(ctx, `DELETE FROM loadtest.runs WHERE run_id = $1`, runID); err != nil {
		return fmt.Errorf("clean load-test report rows: %w", err)
	}
	return nil
}

func plannedOutcomeCounts(data Data) (purchase, cancel, ttl int) {
	for _, user := range data.Users {
		for _, assignment := range user.Assignments {
			switch assignment.PlannedOutcome {
			case "purchase":
				purchase++
			case "cancel":
				cancel++
			case "ttl":
				ttl++
			}
		}
	}
	return purchase, cancel, ttl
}
