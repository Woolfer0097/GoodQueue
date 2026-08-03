package storage

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib" // Register pgx as a database/sql driver.
)

type PostgreSQLConfig struct {
	URL             string
	MaxOpenConns    int
	MaxIdleConns    int
	ConnMaxLifetime time.Duration
}

func OpenPostgreSQL(config PostgreSQLConfig) (*sql.DB, error) {
	database, err := sql.Open("pgx", config.URL)
	if err != nil {
		return nil, fmt.Errorf("open PostgreSQL: %w", err)
	}
	database.SetMaxOpenConns(config.MaxOpenConns)
	database.SetMaxIdleConns(config.MaxIdleConns)
	database.SetConnMaxLifetime(config.ConnMaxLifetime)
	return database, nil
}

func Ping(ctx context.Context, database *sql.DB) error {
	if err := database.PingContext(ctx); err != nil {
		return fmt.Errorf("ping PostgreSQL: %w", err)
	}
	return nil
}
