package config

import (
	"strings"
	"testing"
	"time"
)

func TestLoadFromRequiresDatabaseURL(t *testing.T) {
	_, err := LoadFrom(func(string) (string, bool) { return "", false })
	if err == nil || !strings.Contains(err.Error(), "GOODQUEUE_DATABASE_URL") {
		t.Fatalf("expected required database URL error, got %v", err)
	}
}

func TestLoadFromAllowsMockModeWithoutDatabaseURL(t *testing.T) {
	config, err := LoadFrom(func(key string) (string, bool) {
		if key == "GOODQUEUE_MODE" {
			return ModeMock, true
		}
		return "", false
	})
	if err != nil {
		t.Fatalf("load mock config: %v", err)
	}
	if config.Mode != ModeMock || config.DatabaseURL != "" {
		t.Fatalf("unexpected mock config: %+v", config)
	}
}

func TestLoadFromRejectsUnknownMode(t *testing.T) {
	_, err := LoadFrom(func(key string) (string, bool) {
		if key == "GOODQUEUE_MODE" {
			return "memory", true
		}
		return "", false
	})
	if err == nil || !strings.Contains(err.Error(), "GOODQUEUE_MODE") {
		t.Fatalf("expected mode validation error, got %v", err)
	}
}

func TestLoadFromParsesValuesOnceAtBoundary(t *testing.T) {
	values := map[string]string{
		"GOODQUEUE_DATABASE_URL":                 "postgres://database/goodqueue",
		"GOODQUEUE_DATABASE_MAX_OPEN_CONNS":      "8",
		"GOODQUEUE_DATABASE_MAX_IDLE_CONNS":      "4",
		"GOODQUEUE_HTTP_READ_HEADER_TIMEOUT":     "3s",
		"GOODQUEUE_DATABASE_PING_TIMEOUT":        "750ms",
		"GOODQUEUE_DATABASE_CONN_MAX_LIFETIME":   "5m",
		"GOODQUEUE_LOADTEST_PROMETHEUS_URL":      "http://prometheus:9090/",
		"GOODQUEUE_LOADTEST_SUCCESS_RATE_WINDOW": "45m",
		"GOODQUEUE_CORS_ALLOWED_ORIGINS":         "http://localhost:5173, https://demo.example, http://localhost:5173",
	}
	config, err := LoadFrom(func(key string) (string, bool) {
		value, exists := values[key]
		return value, exists
	})
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if config.DatabaseMaxOpenConns != 8 || config.DatabaseMaxIdleConns != 4 {
		t.Fatalf("unexpected pool config: %+v", config)
	}
	if config.HTTPReadHeaderTimeout != 3*time.Second || config.DatabasePingTimeout != 750*time.Millisecond || config.DatabaseConnMaxLifetime != 5*time.Minute {
		t.Fatalf("unexpected duration config: %+v", config)
	}
	if len(config.CORSAllowedOrigins) != 2 || config.CORSAllowedOrigins[1] != "https://demo.example" {
		t.Fatalf("unexpected CORS origins: %#v", config.CORSAllowedOrigins)
	}
	if config.LoadtestPrometheusURL != "http://prometheus:9090" || config.LoadtestSuccessWindow != 45*time.Minute {
		t.Fatalf("unexpected loadtest metrics config: %+v", config)
	}
}

func TestLoadFromDefaultsReadHeaderTimeout(t *testing.T) {
	config, err := LoadFrom(func(key string) (string, bool) {
		if key == "GOODQUEUE_DATABASE_URL" {
			return "postgres://database/goodqueue", true
		}
		return "", false
	})
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if config.HTTPReadHeaderTimeout != 5*time.Second {
		t.Fatalf("unexpected read header timeout: %s", config.HTTPReadHeaderTimeout)
	}
	if config.Mode != ModePostgres {
		t.Fatalf("unexpected default mode: %s", config.Mode)
	}
	if config.InvitationTTL != 10*time.Minute {
		t.Fatalf("unexpected invitation TTL: %s", config.InvitationTTL)
	}
	if config.CheckoutTTL != 5*time.Minute {
		t.Fatalf("unexpected checkout TTL: %s", config.CheckoutTTL)
	}
	if config.WaitingBufferPercent != 100 {
		t.Fatalf("unexpected waiting buffer: %d", config.WaitingBufferPercent)
	}
	if config.UnsafePaymentCallback {
		t.Fatal("unsafe payment callback must default to false")
	}
	if config.LoadtestPrometheusURL != "" || config.LoadtestSuccessWindow != 30*time.Minute {
		t.Fatalf("unexpected loadtest metrics defaults: %+v", config)
	}
	if len(config.CORSAllowedOrigins) != 2 || config.CORSAllowedOrigins[0] != "http://localhost:5173" {
		t.Fatalf("unexpected default CORS origins: %#v", config.CORSAllowedOrigins)
	}
}

func TestLoadFromRejectsInvalidReadHeaderTimeout(t *testing.T) {
	values := map[string]string{
		"GOODQUEUE_DATABASE_URL":             "postgres://database/goodqueue",
		"GOODQUEUE_HTTP_READ_HEADER_TIMEOUT": "0s",
	}
	_, err := LoadFrom(func(key string) (string, bool) {
		value, exists := values[key]
		return value, exists
	})
	if err == nil || !strings.Contains(err.Error(), "GOODQUEUE_HTTP_READ_HEADER_TIMEOUT") {
		t.Fatalf("expected invalid read header timeout error, got %v", err)
	}
}

func TestLoadFromRejectsInvalidPoolBounds(t *testing.T) {
	values := map[string]string{
		"GOODQUEUE_DATABASE_URL":            "postgres://database/goodqueue",
		"GOODQUEUE_DATABASE_MAX_OPEN_CONNS": "2",
		"GOODQUEUE_DATABASE_MAX_IDLE_CONNS": "3",
	}
	_, err := LoadFrom(func(key string) (string, bool) {
		value, exists := values[key]
		return value, exists
	})
	if err == nil {
		t.Fatal("expected invalid pool bounds to fail")
	}
}

func TestLoadFromParsesQueueConfiguration(t *testing.T) {
	values := map[string]string{
		"GOODQUEUE_DATABASE_URL":                         "postgres://database/goodqueue",
		"GOODQUEUE_INVITATION_TTL":                       "12m",
		"GOODQUEUE_CHECKOUT_TTL":                         "4m",
		"GOODQUEUE_WAITING_BUFFER_PERCENT":               "500",
		"GOODQUEUE_WORKER_INTERVAL":                      "250ms",
		"GOODQUEUE_UNSAFE_PAYMENT_CALLBACK":              "true",
		"GOODQUEUE_RECONCILIATION_TRANSITION_BATCH_SIZE": "25",
		"GOODQUEUE_MAX_PRODUCTS_PER_CYCLE":               "12",
		"GOODQUEUE_MAX_OUTBOX_ITEMS_PER_CYCLE":           "34",
		"GOODQUEUE_OUTBOX_LEASE_DURATION":                "45s",
		"GOODQUEUE_OUTBOX_RETRY_BASE_DURATION":           "2s",
		"GOODQUEUE_OUTBOX_RETRY_MAX_DURATION":            "2m",
		"GOODQUEUE_PUBLISHER_TIMEOUT":                    "3s",
	}
	config, err := LoadFrom(func(key string) (string, bool) {
		value, exists := values[key]
		return value, exists
	})
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if config.InvitationTTL != 12*time.Minute || config.CheckoutTTL != 4*time.Minute {
		t.Fatalf("unexpected queue TTLs: %+v", config)
	}
	if config.WaitingBufferPercent != 500 || config.WorkerInterval != 250*time.Millisecond {
		t.Fatalf("unexpected worker config: %+v", config)
	}
	if !config.UnsafePaymentCallback {
		t.Fatal("expected unsafe payment callback override")
	}
	if config.ReconciliationBatchSize != 25 || config.MaxProductsPerCycle != 12 || config.MaxOutboxItemsPerCycle != 34 {
		t.Fatalf("unexpected worker bounds: %+v", config)
	}
	if config.OutboxLeaseDuration != 45*time.Second || config.OutboxRetryBase != 2*time.Second ||
		config.OutboxRetryMax != 2*time.Minute || config.PublisherTimeout != 3*time.Second {
		t.Fatalf("unexpected outbox durations: %+v", config)
	}
}

func TestLoadFromRejectsInvalidQueueConfiguration(t *testing.T) {
	tests := []struct {
		name  string
		key   string
		value string
	}{
		{name: "negative waiting buffer", key: "GOODQUEUE_WAITING_BUFFER_PERCENT", value: "-1"},
		{name: "excessive waiting buffer", key: "GOODQUEUE_WAITING_BUFFER_PERCENT", value: "501"},
		{name: "zero invitation TTL", key: "GOODQUEUE_INVITATION_TTL", value: "0s"},
		{name: "zero checkout TTL", key: "GOODQUEUE_CHECKOUT_TTL", value: "0s"},
		{name: "zero worker interval", key: "GOODQUEUE_WORKER_INTERVAL", value: "0s"},
		{name: "invalid unsafe callback", key: "GOODQUEUE_UNSAFE_PAYMENT_CALLBACK", value: "sometimes"},
		{name: "zero reconciliation batch", key: "GOODQUEUE_RECONCILIATION_TRANSITION_BATCH_SIZE", value: "0"},
		{name: "excessive reconciliation batch", key: "GOODQUEUE_RECONCILIATION_TRANSITION_BATCH_SIZE", value: "1001"},
		{name: "zero products per cycle", key: "GOODQUEUE_MAX_PRODUCTS_PER_CYCLE", value: "0"},
		{name: "zero outbox items per cycle", key: "GOODQUEUE_MAX_OUTBOX_ITEMS_PER_CYCLE", value: "0"},
		{name: "excessive outbox lease", key: "GOODQUEUE_OUTBOX_LEASE_DURATION", value: "2h"},
		{name: "excessive retry max", key: "GOODQUEUE_OUTBOX_RETRY_MAX_DURATION", value: "25h"},
		{name: "excessive publisher timeout", key: "GOODQUEUE_PUBLISHER_TIMEOUT", value: "6m"},
		{name: "zero loadtest window", key: "GOODQUEUE_LOADTEST_SUCCESS_RATE_WINDOW", value: "0s"},
		{name: "excessive loadtest window", key: "GOODQUEUE_LOADTEST_SUCCESS_RATE_WINDOW", value: "721h"},
		{name: "invalid Prometheus URL", key: "GOODQUEUE_LOADTEST_PROMETHEUS_URL", value: "prometheus:9090"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			values := map[string]string{
				"GOODQUEUE_DATABASE_URL": "postgres://database/goodqueue",
				test.key:                 test.value,
			}
			_, err := LoadFrom(func(key string) (string, bool) {
				value, exists := values[key]
				return value, exists
			})
			if err == nil || !strings.Contains(err.Error(), test.key) {
				t.Fatalf("expected %s validation error, got %v", test.key, err)
			}
		})
	}
}

func TestLoadFromRejectsRetryBaseAboveMaximum(t *testing.T) {
	values := map[string]string{
		"GOODQUEUE_DATABASE_URL":               "postgres://database/goodqueue",
		"GOODQUEUE_OUTBOX_RETRY_BASE_DURATION": "10s",
		"GOODQUEUE_OUTBOX_RETRY_MAX_DURATION":  "5s",
	}
	_, err := LoadFrom(func(key string) (string, bool) {
		value, exists := values[key]
		return value, exists
	})
	if err == nil || !strings.Contains(err.Error(), "GOODQUEUE_OUTBOX_RETRY_BASE_DURATION") {
		t.Fatalf("expected retry bounds error, got %v", err)
	}
}

func TestLoadFromRejectsPublisherTimeoutWithoutLeaseSafetyMargin(t *testing.T) {
	tests := []struct {
		name             string
		leaseDuration    string
		publisherTimeout string
	}{
		{name: "publisher outlives lease", leaseDuration: "10s", publisherTimeout: "11s"},
		{name: "lease lacks two-times margin", leaseDuration: "10s", publisherTimeout: "6s"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			values := map[string]string{
				"GOODQUEUE_DATABASE_URL":          "postgres://database/goodqueue",
				"GOODQUEUE_OUTBOX_LEASE_DURATION": test.leaseDuration,
				"GOODQUEUE_PUBLISHER_TIMEOUT":     test.publisherTimeout,
			}
			_, err := LoadFrom(func(key string) (string, bool) {
				value, exists := values[key]
				return value, exists
			})
			if err == nil || !strings.Contains(err.Error(), "GOODQUEUE_OUTBOX_LEASE_DURATION") {
				t.Fatalf("expected unsafe lease/publisher error, got %v", err)
			}
		})
	}
}

func TestLoadFromAcceptsExactPublisherLeaseSafetyMargin(t *testing.T) {
	values := map[string]string{
		"GOODQUEUE_DATABASE_URL":          "postgres://database/goodqueue",
		"GOODQUEUE_OUTBOX_LEASE_DURATION": "10s",
		"GOODQUEUE_PUBLISHER_TIMEOUT":     "5s",
	}
	if _, err := LoadFrom(func(key string) (string, bool) {
		value, exists := values[key]
		return value, exists
	}); err != nil {
		t.Fatalf("exact safety margin should be valid: %v", err)
	}
}
