package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

const (
	defaultHTTPAddress        = ":8080"
	defaultReadHeaderTimeout  = 5 * time.Second
	defaultShutdownTimeout    = 10 * time.Second
	defaultDatabasePing       = 2 * time.Second
	defaultMaxOpenConnections = 20
	defaultMaxIdleConnections = 10
	defaultConnectionLifetime = 30 * time.Minute
	defaultLogLevel           = "info"
	defaultMockAPI            = false
	defaultMockQueueStatus    = "waiting"
)

type Config struct {
	HTTPAddress             string
	HTTPReadHeaderTimeout   time.Duration
	DatabaseURL             string
	ShutdownTimeout         time.Duration
	DatabasePingTimeout     time.Duration
	DatabaseMaxOpenConns    int
	DatabaseMaxIdleConns    int
	DatabaseConnMaxLifetime time.Duration
	LogLevel                string
	MockAPI                 bool
	MockQueueStatus         string
}

type LookupEnv func(string) (string, bool)

func Load() (Config, error) {
	return LoadFrom(os.LookupEnv)
}

func LoadFrom(lookup LookupEnv) (Config, error) {
	databaseURL, exists := lookup("GOODQUEUE_DATABASE_URL")
	if !exists || strings.TrimSpace(databaseURL) == "" {
		return Config{}, fmt.Errorf("GOODQUEUE_DATABASE_URL is required")
	}

	shutdownTimeout, err := durationValue(lookup, "GOODQUEUE_SHUTDOWN_TIMEOUT", defaultShutdownTimeout)
	if err != nil {
		return Config{}, err
	}
	readHeaderTimeout, err := durationValue(lookup, "GOODQUEUE_HTTP_READ_HEADER_TIMEOUT", defaultReadHeaderTimeout)
	if err != nil {
		return Config{}, err
	}
	databasePingTimeout, err := durationValue(lookup, "GOODQUEUE_DATABASE_PING_TIMEOUT", defaultDatabasePing)
	if err != nil {
		return Config{}, err
	}
	maxOpen, err := positiveIntValue(lookup, "GOODQUEUE_DATABASE_MAX_OPEN_CONNS", defaultMaxOpenConnections)
	if err != nil {
		return Config{}, err
	}
	maxIdle, err := nonNegativeIntValue(lookup, "GOODQUEUE_DATABASE_MAX_IDLE_CONNS", defaultMaxIdleConnections)
	if err != nil {
		return Config{}, err
	}
	if maxIdle > maxOpen {
		return Config{}, fmt.Errorf("GOODQUEUE_DATABASE_MAX_IDLE_CONNS must not exceed GOODQUEUE_DATABASE_MAX_OPEN_CONNS")
	}
	connectionLifetime, err := durationValue(lookup, "GOODQUEUE_DATABASE_CONN_MAX_LIFETIME", defaultConnectionLifetime)
	if err != nil {
		return Config{}, err
	}
	mockAPI, err := booleanValue(lookup, "GOODQUEUE_MOCK_API", defaultMockAPI)
	if err != nil {
		return Config{}, err
	}
	mockQueueStatus, err := mockQueueStatusValue(lookup)
	if err != nil {
		return Config{}, err
	}

	return Config{
		HTTPAddress:             stringValue(lookup, "GOODQUEUE_HTTP_ADDRESS", defaultHTTPAddress),
		HTTPReadHeaderTimeout:   readHeaderTimeout,
		DatabaseURL:             databaseURL,
		ShutdownTimeout:         shutdownTimeout,
		DatabasePingTimeout:     databasePingTimeout,
		DatabaseMaxOpenConns:    maxOpen,
		DatabaseMaxIdleConns:    maxIdle,
		DatabaseConnMaxLifetime: connectionLifetime,
		LogLevel:                stringValue(lookup, "GOODQUEUE_LOG_LEVEL", defaultLogLevel),
		MockAPI:                 mockAPI,
		MockQueueStatus:         mockQueueStatus,
	}, nil
}

func stringValue(lookup LookupEnv, key, fallback string) string {
	value, exists := lookup(key)
	if !exists || strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}

func durationValue(lookup LookupEnv, key string, fallback time.Duration) (time.Duration, error) {
	raw := stringValue(lookup, key, fallback.String())
	value, err := time.ParseDuration(raw)
	if err != nil || value <= 0 {
		return 0, fmt.Errorf("%s must be a positive duration", key)
	}
	return value, nil
}

func positiveIntValue(lookup LookupEnv, key string, fallback int) (int, error) {
	value, err := integerValue(lookup, key, fallback)
	if err != nil || value <= 0 {
		return 0, fmt.Errorf("%s must be a positive integer", key)
	}
	return value, nil
}

func nonNegativeIntValue(lookup LookupEnv, key string, fallback int) (int, error) {
	value, err := integerValue(lookup, key, fallback)
	if err != nil || value < 0 {
		return 0, fmt.Errorf("%s must be a non-negative integer", key)
	}
	return value, nil
}

func integerValue(lookup LookupEnv, key string, fallback int) (int, error) {
	raw := stringValue(lookup, key, strconv.Itoa(fallback))
	value, err := strconv.Atoi(raw)
	if err != nil {
		return 0, err
	}
	return value, nil
}

func booleanValue(lookup LookupEnv, key string, fallback bool) (bool, error) {
	raw := stringValue(lookup, key, strconv.FormatBool(fallback))
	value, err := strconv.ParseBool(raw)
	if err != nil {
		return false, fmt.Errorf("%s must be true or false", key)
	}
	return value, nil
}

func mockQueueStatusValue(lookup LookupEnv) (string, error) {
	value := stringValue(lookup, "GOODQUEUE_MOCK_QUEUE_STATUS", defaultMockQueueStatus)
	switch value {
	case "waiting", "granted", "purchased", "cancelled", "expired":
		return value, nil
	default:
		return "", fmt.Errorf("GOODQUEUE_MOCK_QUEUE_STATUS must be one of waiting, granted, purchased, cancelled, expired")
	}
}
