package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

const (
	ModePostgres = "postgres"
	ModeMock     = "mock"

	defaultMode               = ModePostgres
	defaultHTTPAddress        = ":8080"
	defaultCORSOrigins        = "http://localhost:5173,http://127.0.0.1:5173"
	defaultReadHeaderTimeout  = 5 * time.Second
	defaultShutdownTimeout    = 10 * time.Second
	defaultDatabasePing       = 2 * time.Second
	defaultMaxOpenConnections = 20
	defaultMaxIdleConnections = 10
	defaultConnectionLifetime = 30 * time.Minute
	defaultLogLevel           = "info"
	defaultWorkerInterval     = time.Second
	defaultInvitationTTL      = 10 * time.Minute
	defaultCheckoutTTL        = 5 * time.Minute
	defaultWaitingBuffer      = 100
	maxWaitingBuffer          = 500
	defaultReconcileBatch     = 100
	defaultMaxProductsCycle   = 100
	defaultMaxOutboxCycle     = 100
	maxWorkerBatch            = 1000
	defaultOutboxLease        = 30 * time.Second
	defaultOutboxRetryBase    = 5 * time.Second
	defaultOutboxRetryMax     = 15 * time.Minute
	defaultPublisherTimeout   = 5 * time.Second
	defaultOpenAIBaseURL      = "https://api.openai.com/v1"
	defaultEmbeddingModel     = "text-embedding-3-small"
	defaultEmbeddingTimeout   = 8 * time.Second
	maxOutboxLease            = time.Hour
	maxOutboxRetry            = 24 * time.Hour
	maxPublisherTimeout       = 5 * time.Minute
)

type Config struct {
	Mode                     string
	HTTPAddress              string
	CORSAllowedOrigins       []string
	HTTPReadHeaderTimeout    time.Duration
	DatabaseURL              string
	ShutdownTimeout          time.Duration
	DatabasePingTimeout      time.Duration
	DatabaseMaxOpenConns     int
	DatabaseMaxIdleConns     int
	DatabaseConnMaxLifetime  time.Duration
	LogLevel                 string
	WorkerInterval           time.Duration
	InvitationTTL            time.Duration
	CheckoutTTL              time.Duration
	WaitingBufferPercent     int
	UnsafePaymentCallback    bool
	ReconciliationBatchSize  int
	MaxProductsPerCycle      int
	MaxOutboxItemsPerCycle   int
	OutboxLeaseDuration      time.Duration
	OutboxRetryBase          time.Duration
	OutboxRetryMax           time.Duration
	PublisherTimeout         time.Duration
	RecommendationsAIEnabled bool
	OpenAIAPIKey             string
	OpenAIBaseURL            string
	OpenAIEmbeddingModel     string
	OpenAIEmbeddingTimeout   time.Duration
}

type LookupEnv func(string) (string, bool)

func Load() (Config, error) {
	return LoadFrom(os.LookupEnv)
}

func LoadFrom(lookup LookupEnv) (Config, error) {
	mode, err := modeValue(lookup)
	if err != nil {
		return Config{}, err
	}
	databaseURL, exists := lookup("GOODQUEUE_DATABASE_URL")
	if mode == ModePostgres && (!exists || strings.TrimSpace(databaseURL) == "") {
		return Config{}, fmt.Errorf("GOODQUEUE_DATABASE_URL is required")
	}
	databaseURL = strings.TrimSpace(databaseURL)

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
	workerInterval, err := durationValue(lookup, "GOODQUEUE_WORKER_INTERVAL", defaultWorkerInterval)
	if err != nil {
		return Config{}, err
	}
	invitationTTL, err := durationValue(lookup, "GOODQUEUE_INVITATION_TTL", defaultInvitationTTL)
	if err != nil {
		return Config{}, err
	}
	checkoutTTL, err := durationValue(lookup, "GOODQUEUE_CHECKOUT_TTL", defaultCheckoutTTL)
	if err != nil {
		return Config{}, err
	}
	waitingBufferPercent, err := boundedIntValue(
		lookup,
		"GOODQUEUE_WAITING_BUFFER_PERCENT",
		defaultWaitingBuffer,
		0,
		maxWaitingBuffer,
	)
	if err != nil {
		return Config{}, err
	}
	unsafePaymentCallback, err := boolValue(lookup, "GOODQUEUE_UNSAFE_PAYMENT_CALLBACK", false)
	if err != nil {
		return Config{}, err
	}
	reconciliationBatchSize, err := boundedIntValue(lookup, "GOODQUEUE_RECONCILIATION_TRANSITION_BATCH_SIZE", defaultReconcileBatch, 1, maxWorkerBatch)
	if err != nil {
		return Config{}, err
	}
	maxProductsPerCycle, err := boundedIntValue(lookup, "GOODQUEUE_MAX_PRODUCTS_PER_CYCLE", defaultMaxProductsCycle, 1, maxWorkerBatch)
	if err != nil {
		return Config{}, err
	}
	maxOutboxItemsPerCycle, err := boundedIntValue(lookup, "GOODQUEUE_MAX_OUTBOX_ITEMS_PER_CYCLE", defaultMaxOutboxCycle, 1, maxWorkerBatch)
	if err != nil {
		return Config{}, err
	}
	outboxLeaseDuration, err := boundedDurationValue(lookup, "GOODQUEUE_OUTBOX_LEASE_DURATION", defaultOutboxLease, maxOutboxLease)
	if err != nil {
		return Config{}, err
	}
	outboxRetryBase, err := boundedDurationValue(lookup, "GOODQUEUE_OUTBOX_RETRY_BASE_DURATION", defaultOutboxRetryBase, maxOutboxRetry)
	if err != nil {
		return Config{}, err
	}
	outboxRetryMax, err := boundedDurationValue(lookup, "GOODQUEUE_OUTBOX_RETRY_MAX_DURATION", defaultOutboxRetryMax, maxOutboxRetry)
	if err != nil {
		return Config{}, err
	}
	if outboxRetryBase > outboxRetryMax {
		return Config{}, fmt.Errorf("GOODQUEUE_OUTBOX_RETRY_BASE_DURATION must not exceed GOODQUEUE_OUTBOX_RETRY_MAX_DURATION")
	}
	publisherTimeout, err := boundedDurationValue(lookup, "GOODQUEUE_PUBLISHER_TIMEOUT", defaultPublisherTimeout, maxPublisherTimeout)
	if err != nil {
		return Config{}, err
	}
	if publisherTimeout > outboxLeaseDuration/2 {
		return Config{}, fmt.Errorf("GOODQUEUE_OUTBOX_LEASE_DURATION must be at least twice GOODQUEUE_PUBLISHER_TIMEOUT")
	}
	recommendationsAIEnabled, err := boolValue(lookup, "GOODQUEUE_RECOMMENDATIONS_AI_ENABLED", false)
	if err != nil {
		return Config{}, err
	}
	openAIAPIKey := strings.TrimSpace(stringValue(lookup, "GOODQUEUE_OPENAI_API_KEY", ""))
	if recommendationsAIEnabled && openAIAPIKey == "" {
		return Config{}, fmt.Errorf("GOODQUEUE_OPENAI_API_KEY is required when AI recommendations are enabled")
	}
	openAIEmbeddingTimeout, err := durationValue(
		lookup,
		"GOODQUEUE_OPENAI_EMBEDDING_TIMEOUT",
		defaultEmbeddingTimeout,
	)
	if err != nil {
		return Config{}, err
	}

	return Config{
		Mode:                     mode,
		HTTPAddress:              stringValue(lookup, "GOODQUEUE_HTTP_ADDRESS", defaultHTTPAddress),
		CORSAllowedOrigins:       commaSeparatedValue(lookup, "GOODQUEUE_CORS_ALLOWED_ORIGINS", defaultCORSOrigins),
		HTTPReadHeaderTimeout:    readHeaderTimeout,
		DatabaseURL:              databaseURL,
		ShutdownTimeout:          shutdownTimeout,
		DatabasePingTimeout:      databasePingTimeout,
		DatabaseMaxOpenConns:     maxOpen,
		DatabaseMaxIdleConns:     maxIdle,
		DatabaseConnMaxLifetime:  connectionLifetime,
		LogLevel:                 stringValue(lookup, "GOODQUEUE_LOG_LEVEL", defaultLogLevel),
		WorkerInterval:           workerInterval,
		InvitationTTL:            invitationTTL,
		CheckoutTTL:              checkoutTTL,
		WaitingBufferPercent:     waitingBufferPercent,
		UnsafePaymentCallback:    unsafePaymentCallback,
		ReconciliationBatchSize:  reconciliationBatchSize,
		MaxProductsPerCycle:      maxProductsPerCycle,
		MaxOutboxItemsPerCycle:   maxOutboxItemsPerCycle,
		OutboxLeaseDuration:      outboxLeaseDuration,
		OutboxRetryBase:          outboxRetryBase,
		OutboxRetryMax:           outboxRetryMax,
		PublisherTimeout:         publisherTimeout,
		RecommendationsAIEnabled: recommendationsAIEnabled,
		OpenAIAPIKey:             openAIAPIKey,
		OpenAIBaseURL:            stringValue(lookup, "GOODQUEUE_OPENAI_BASE_URL", defaultOpenAIBaseURL),
		OpenAIEmbeddingModel:     stringValue(lookup, "GOODQUEUE_OPENAI_EMBEDDING_MODEL", defaultEmbeddingModel),
		OpenAIEmbeddingTimeout:   openAIEmbeddingTimeout,
	}, nil
}

func modeValue(lookup LookupEnv) (string, error) {
	mode := strings.ToLower(strings.TrimSpace(stringValue(lookup, "GOODQUEUE_MODE", defaultMode)))
	switch mode {
	case ModePostgres, ModeMock:
		return mode, nil
	default:
		return "", fmt.Errorf("GOODQUEUE_MODE must be one of postgres, mock")
	}
}

func commaSeparatedValue(lookup LookupEnv, key, fallback string) []string {
	raw := stringValue(lookup, key, fallback)
	seen := make(map[string]struct{})
	values := make([]string, 0)
	for _, item := range strings.Split(raw, ",") {
		value := strings.TrimSpace(item)
		if value == "" {
			continue
		}
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		values = append(values, value)
	}
	return values
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

func boundedDurationValue(lookup LookupEnv, key string, fallback, maximum time.Duration) (time.Duration, error) {
	value, err := durationValue(lookup, key, fallback)
	if err != nil {
		return 0, err
	}
	if value > maximum {
		return 0, fmt.Errorf("%s must not exceed %s", key, maximum)
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

func boundedIntValue(lookup LookupEnv, key string, fallback, minimum, maximum int) (int, error) {
	value, err := integerValue(lookup, key, fallback)
	if err != nil || value < minimum || value > maximum {
		return 0, fmt.Errorf("%s must be an integer between %d and %d", key, minimum, maximum)
	}
	return value, nil
}

func boolValue(lookup LookupEnv, key string, fallback bool) (bool, error) {
	raw := stringValue(lookup, key, strconv.FormatBool(fallback))
	value, err := strconv.ParseBool(raw)
	if err != nil {
		return false, fmt.Errorf("%s must be a boolean", key)
	}
	return value, nil
}
