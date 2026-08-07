package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/Woolfer0097/GoodQueue/internal/loadtest"
)

func main() {
	if err := run(); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "loadtest-seed: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	cleanupOnly := flag.Bool("cleanup-only", false, "delete only records belonging to LOADTEST_RUN_ID")
	flag.Parse()

	config, err := loadtest.LoadConfig()
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	connection, err := loadtest.Connect(ctx, config.DatabaseURL)
	if err != nil {
		return err
	}
	defer func() { _ = connection.Close(context.Background()) }()

	if *cleanupOnly {
		if err := loadtest.Cleanup(ctx, connection, config.RunID); err != nil {
			return err
		}
		fmt.Printf("Cleaned PostgreSQL records for load-test run %q only.\n", config.RunID)
		return nil
	}

	data, err := loadtest.GenerateData(config)
	if err != nil {
		return err
	}
	if err := loadtest.Seed(ctx, connection, config, data); err != nil {
		return err
	}
	if err := loadtest.WriteData(config.DataFile, data); err != nil {
		return err
	}
	resultDirectory := filepath.Join(config.ResultsDir, config.RunID)
	if err := os.MkdirAll(resultDirectory, 0o750); err != nil {
		return fmt.Errorf("create load-test result directory: %w", err)
	}
	fmt.Printf(
		"Seeded run %q: users=%d products=%d assignments=%d data=%s\n",
		config.RunID, len(data.Users), len(data.Products), len(data.Users)*config.ProductsPerUser, config.DataFile,
	)
	return nil
}
