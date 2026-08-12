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
	cleanupDisposableUI := flag.Bool("cleanup-disposable-ui", false, "delete the latest completed disposable UI run")
	markFailed := flag.Bool("mark-failed", false, "preserve LOADTEST_RUN_ID as a failed diagnostic run")
	flag.Parse()
	selectedModes := 0
	for _, selected := range []bool{*cleanupOnly, *cleanupDisposableUI, *markFailed} {
		if selected {
			selectedModes++
		}
	}
	if selectedModes > 1 {
		return fmt.Errorf("cleanup and failure modes are mutually exclusive")
	}

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
	if *cleanupDisposableUI {
		runID, err := loadtest.FindDisposableUIRun(ctx, connection)
		if err != nil {
			return err
		}
		if runID == "" {
			fmt.Println("No disposable UI load-test run to clean.")
			return nil
		}
		generatedRoot := filepath.Dir(filepath.Dir(config.DataFile))
		for _, path := range []string{filepath.Join(generatedRoot, runID), filepath.Join(config.ResultsDir, runID)} {
			if err := os.RemoveAll(path); err != nil {
				return fmt.Errorf("remove disposable run directory %s: %w", path, err)
			}
		}
		if err := loadtest.Cleanup(ctx, connection, runID); err != nil {
			return err
		}
		fmt.Printf("Cleaned disposable UI load-test run %q.\n", runID)
		return nil
	}
	if *markFailed {
		if err := loadtest.PreserveFailedRun(ctx, connection, config.RunID); err != nil {
			return err
		}
		fmt.Printf("Preserved failed load-test run %q.\n", config.RunID)
		return nil
	}

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
