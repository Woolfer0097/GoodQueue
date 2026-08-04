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

func TestLoadFromParsesValuesOnceAtBoundary(t *testing.T) {
	values := map[string]string{
		"GOODQUEUE_DATABASE_URL":               "postgres://database/goodqueue",
		"GOODQUEUE_DATABASE_MAX_OPEN_CONNS":    "8",
		"GOODQUEUE_DATABASE_MAX_IDLE_CONNS":    "4",
		"GOODQUEUE_HTTP_READ_HEADER_TIMEOUT":   "3s",
		"GOODQUEUE_DATABASE_PING_TIMEOUT":      "750ms",
		"GOODQUEUE_DATABASE_CONN_MAX_LIFETIME": "5m",
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

func TestLoadFromDefaultsMockConfiguration(t *testing.T) {
	config, err := LoadFrom(func(key string) (string, bool) {
		if key == "GOODQUEUE_DATABASE_URL" {
			return "postgres://database/goodqueue", true
		}
		return "", false
	})
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if config.MockAPI {
		t.Fatal("mock API must be disabled by default")
	}
	if config.MockQueueStatus != "waiting" {
		t.Fatalf("mock queue status = %q, want waiting", config.MockQueueStatus)
	}
}

func TestLoadFromParsesMockConfiguration(t *testing.T) {
	values := map[string]string{
		"GOODQUEUE_DATABASE_URL":      "postgres://database/goodqueue",
		"GOODQUEUE_MOCK_API":          "true",
		"GOODQUEUE_MOCK_QUEUE_STATUS": "granted",
	}
	config, err := LoadFrom(func(key string) (string, bool) {
		value, exists := values[key]
		return value, exists
	})
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if !config.MockAPI || config.MockQueueStatus != "granted" {
		t.Fatalf("unexpected mock config: %+v", config)
	}
}

func TestLoadFromAcceptsEveryMockQueueStatus(t *testing.T) {
	for _, status := range []string{"waiting", "granted", "purchased", "cancelled", "expired"} {
		t.Run(status, func(t *testing.T) {
			values := map[string]string{
				"GOODQUEUE_DATABASE_URL":      "postgres://database/goodqueue",
				"GOODQUEUE_MOCK_QUEUE_STATUS": status,
			}
			config, err := LoadFrom(func(key string) (string, bool) {
				value, exists := values[key]
				return value, exists
			})
			if err != nil {
				t.Fatalf("load config: %v", err)
			}
			if config.MockQueueStatus != status {
				t.Fatalf("status = %q, want %q", config.MockQueueStatus, status)
			}
		})
	}
}

func TestLoadFromRejectsInvalidMockConfiguration(t *testing.T) {
	tests := map[string]map[string]string{
		"boolean": {
			"GOODQUEUE_DATABASE_URL": "postgres://database/goodqueue",
			"GOODQUEUE_MOCK_API":     "enabled",
		},
		"queue status": {
			"GOODQUEUE_DATABASE_URL":      "postgres://database/goodqueue",
			"GOODQUEUE_MOCK_QUEUE_STATUS": "unknown",
		},
	}
	for name, values := range tests {
		t.Run(name, func(t *testing.T) {
			_, err := LoadFrom(func(key string) (string, bool) {
				value, exists := values[key]
				return value, exists
			})
			if err == nil {
				t.Fatal("expected invalid mock configuration to fail")
			}
		})
	}
}
