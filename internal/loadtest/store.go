package loadtest

import (
	"context"
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
		if err := cleanupRun(ctx, transaction, prefix); err != nil {
			return err
		}
	} else {
		var attempts int
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
	if err := transaction.Commit(ctx); err != nil {
		return fmt.Errorf("commit load-test seed transaction: %w", err)
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
	if err := cleanupRun(ctx, transaction, prefix); err != nil {
		return err
	}
	if err := transaction.Commit(ctx); err != nil {
		return fmt.Errorf("commit load-test cleanup transaction: %w", err)
	}
	return nil
}

func cleanupRun(ctx context.Context, transaction pgx.Tx, prefix string) error {
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
	return nil
}
