package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/Woolfer0097/GoodQueue/internal/loadtest"
)

func main() {
	passed, err := run()
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "loadtest-verify: %v\n", err)
		os.Exit(1)
	}
	if !passed {
		os.Exit(1)
	}
}

func run() (bool, error) {
	config, err := loadtest.LoadConfig()
	if err != nil {
		return false, err
	}
	data, err := loadtest.ReadData(config.DataFile)
	if err != nil {
		return false, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	connection, err := loadtest.Connect(ctx, config.DatabaseURL)
	if err != nil {
		return false, err
	}
	defer func() { _ = connection.Close(context.Background()) }()

	result, err := loadtest.Verify(ctx, connection, config, data)
	if err != nil {
		return false, err
	}
	resultPath := filepath.Join(config.ResultsDir, config.RunID, "verifier.json")
	if err := loadtest.WriteVerification(resultPath, result); err != nil {
		return false, err
	}
	for _, check := range result.Checks {
		status := "PASS"
		if !check.Passed {
			status = "FAIL"
		}
		fmt.Printf("[%s] %s\n", status, check.Name)
		for _, violation := range check.Violations {
			fmt.Printf("  - %s\n", violation)
		}
	}
	fmt.Printf(
		"Verifier %s: users=%d products=%d attempts=%d waiting=%d invited=%d checkout=%d terminal=%d; JSON=%s\n",
		map[bool]string{true: "PASSED", false: "FAILED"}[result.Passed],
		result.Counts.Users, result.Counts.Products, result.Counts.Attempts,
		result.Counts.Waiting, result.Counts.Invited, result.Counts.Checkout, result.Counts.Terminal, resultPath,
	)
	return result.Passed, nil
}
