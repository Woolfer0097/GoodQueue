package loadtestrunner

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type Config struct {
	Enabled            bool
	Address            string
	APIKey             string
	AllowedOrigins     []string
	BaseURL            string
	DatabaseURL        string
	PrometheusWriteURL string
	GeneratedDir       string
	ResultsDir         string
	K6Binary           string
	SeedBinary         string
	VerifierBinary     string
	ScriptsDir         string
}

func LoadConfig() (Config, error) {
	enabled, err := parseBool("LOADTEST_RUNNER_ENABLED", false)
	if err != nil {
		return Config{}, err
	}
	config := Config{
		Enabled:            enabled,
		Address:            value("LOADTEST_RUNNER_ADDRESS", ":8081"),
		APIKey:             strings.TrimSpace(os.Getenv("LOADTEST_RUNNER_API_KEY")),
		AllowedOrigins:     splitCSV(value("LOADTEST_RUNNER_CORS_ALLOWED_ORIGINS", "http://localhost:8088,http://127.0.0.1:8088")),
		BaseURL:            value("LOADTEST_RUNNER_BASE_URL", "http://backend:8080"),
		DatabaseURL:        value("LOADTEST_RUNNER_DATABASE_URL", "postgres://goodqueue:goodqueue@postgres:5432/goodqueue?sslmode=disable"),
		PrometheusWriteURL: value("LOADTEST_RUNNER_PROMETHEUS_WRITE_URL", "http://prometheus:9090/api/v1/write"),
		GeneratedDir:       filepath.Clean(value("LOADTEST_RUNNER_GENERATED_DIR", "/work/loadtest/generated")),
		ResultsDir:         filepath.Clean(value("LOADTEST_RUNNER_RESULTS_DIR", "/work/loadtest/results")),
		K6Binary:           value("LOADTEST_RUNNER_K6_BINARY", "k6"),
		SeedBinary:         value("LOADTEST_RUNNER_SEED_BINARY", "loadtest-seed"),
		VerifierBinary:     value("LOADTEST_RUNNER_VERIFIER_BINARY", "loadtest-verify"),
		ScriptsDir:         filepath.Clean(value("LOADTEST_RUNNER_SCRIPTS_DIR", "/work/loadtest/k6")),
	}
	if config.Address == "" || config.BaseURL == "" || config.DatabaseURL == "" || config.PrometheusWriteURL == "" {
		return Config{}, fmt.Errorf("runner address and service URLs must not be empty")
	}
	return config, nil
}

func value(key, fallback string) string {
	if raw := strings.TrimSpace(os.Getenv(key)); raw != "" {
		return raw
	}
	return fallback
}

func splitCSV(raw string) []string {
	parts := strings.Split(raw, ",")
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		if part = strings.TrimSpace(part); part != "" {
			result = append(result, part)
		}
	}
	return result
}

func parseBool(key string, fallback bool) (bool, error) {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return fallback, nil
	}
	switch strings.ToLower(raw) {
	case "true":
		return true, nil
	case "false":
		return false, nil
	default:
		return false, fmt.Errorf("%s must be true or false", key)
	}
}
