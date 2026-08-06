package loadtest

import (
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

const (
	ProfileSmoke  = "smoke"
	ProfileMedium = "medium"
	ProfileMain   = "main"
)

var runIDPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9.-]{0,39}$`)

type Config struct {
	Profile              string        `json:"profile"`
	BaseURL              string        `json:"base_url"`
	DatabaseURL          string        `json:"-"`
	RunID                string        `json:"run_id"`
	RandomSeed           int64         `json:"random_seed"`
	Users                int           `json:"users"`
	Products             int           `json:"products"`
	ProductsPerUser      int           `json:"products_per_user"`
	RampDuration         time.Duration `json:"-"`
	PollInterval         time.Duration `json:"-"`
	PollDuration         time.Duration `json:"-"`
	QueueCapacity        int           `json:"queue_capacity"`
	DuplicateJoinPercent int           `json:"duplicate_join_percent"`
	MinStock             int           `json:"min_stock"`
	MaxStock             int           `json:"max_stock"`
	CleanupBeforeSeed    bool          `json:"cleanup_before_seed"`
	KeepData             bool          `json:"keep_data"`
	DataFile             string        `json:"data_file"`
	ResultsDir           string        `json:"results_dir"`
}

type EffectiveConfig struct {
	Config
	RampDuration string `json:"ramp_duration"`
	PollInterval string `json:"poll_interval"`
	PollDuration string `json:"poll_duration"`
}

type LookupEnv func(string) (string, bool)

type profileDefaults struct {
	users           int
	products        int
	productsPerUser int
	rampDuration    time.Duration
	pollDuration    time.Duration
}

var profiles = map[string]profileDefaults{
	ProfileSmoke:  {users: 10, products: 5, productsPerUser: 2, rampDuration: 30 * time.Second, pollDuration: time.Minute},
	ProfileMedium: {users: 100, products: 20, productsPerUser: 5, rampDuration: time.Minute, pollDuration: 2 * time.Minute},
	ProfileMain:   {users: 1000, products: 100, productsPerUser: 10, rampDuration: 5 * time.Minute, pollDuration: 5 * time.Minute},
}

func LoadConfig() (Config, error) {
	return LoadConfigFrom(os.LookupEnv)
}

func LoadConfigFrom(lookup LookupEnv) (Config, error) {
	profile := strings.ToLower(value(lookup, "LOADTEST_PROFILE", ProfileSmoke))
	defaults, exists := profiles[profile]
	if !exists {
		return Config{}, fmt.Errorf("LOADTEST_PROFILE must be one of smoke, medium, main")
	}

	users, err := intValue(lookup, "LOADTEST_USERS", defaults.users)
	if err != nil {
		return Config{}, err
	}
	products, err := intValue(lookup, "LOADTEST_PRODUCTS", defaults.products)
	if err != nil {
		return Config{}, err
	}
	productsPerUser, err := intValue(lookup, "LOADTEST_PRODUCTS_PER_USER", defaults.productsPerUser)
	if err != nil {
		return Config{}, err
	}
	rampDuration, err := durationValue(lookup, "LOADTEST_RAMP_DURATION", defaults.rampDuration)
	if err != nil {
		return Config{}, err
	}
	pollInterval, err := durationValue(lookup, "LOADTEST_POLL_INTERVAL", 10*time.Second)
	if err != nil {
		return Config{}, err
	}
	pollDuration, err := durationValue(lookup, "LOADTEST_POLL_DURATION", defaults.pollDuration)
	if err != nil {
		return Config{}, err
	}
	randomSeed, err := int64Value(lookup, "LOADTEST_RANDOM_SEED", 42)
	if err != nil {
		return Config{}, err
	}
	queueCapacity, err := intValue(lookup, "LOADTEST_QUEUE_CAPACITY", 1000)
	if err != nil {
		return Config{}, err
	}
	duplicateJoinPercent, err := intValue(lookup, "LOADTEST_DUPLICATE_JOIN_PERCENT", 10)
	if err != nil {
		return Config{}, err
	}
	minStock, err := intValue(lookup, "LOADTEST_MIN_STOCK", 1)
	if err != nil {
		return Config{}, err
	}
	maxStock, err := intValue(lookup, "LOADTEST_MAX_STOCK", 20)
	if err != nil {
		return Config{}, err
	}
	cleanupBeforeSeed, err := boolValue(lookup, "LOADTEST_CLEANUP_BEFORE_SEED", false)
	if err != nil {
		return Config{}, err
	}
	keepData, err := boolValue(lookup, "LOADTEST_KEEP_DATA", true)
	if err != nil {
		return Config{}, err
	}

	config := Config{
		Profile: profile, BaseURL: strings.TrimRight(value(lookup, "LOADTEST_BASE_URL", "http://localhost:8080"), "/"),
		DatabaseURL: value(lookup, "LOADTEST_DATABASE_URL", "postgres://goodqueue:goodqueue@localhost:5432/goodqueue?sslmode=disable"),
		RunID:       value(lookup, "LOADTEST_RUN_ID", "local"), RandomSeed: randomSeed,
		Users: users, Products: products, ProductsPerUser: productsPerUser,
		RampDuration: rampDuration, PollInterval: pollInterval, PollDuration: pollDuration,
		QueueCapacity: queueCapacity, DuplicateJoinPercent: duplicateJoinPercent,
		MinStock: minStock, MaxStock: maxStock, CleanupBeforeSeed: cleanupBeforeSeed, KeepData: keepData,
		DataFile:   value(lookup, "LOADTEST_DATA_FILE", filepath.FromSlash("loadtest/generated/data.json")),
		ResultsDir: value(lookup, "LOADTEST_RESULTS_DIR", filepath.FromSlash("loadtest/results")),
	}
	if err := config.Validate(); err != nil {
		return Config{}, err
	}
	return config, nil
}

func (config Config) Validate() error {
	if !runIDPattern.MatchString(config.RunID) {
		return fmt.Errorf("LOADTEST_RUN_ID must match %s", runIDPattern)
	}
	parsedURL, err := url.ParseRequestURI(config.BaseURL)
	if err != nil || (parsedURL.Scheme != "http" && parsedURL.Scheme != "https") || parsedURL.Host == "" {
		return fmt.Errorf("LOADTEST_BASE_URL must be an absolute HTTP(S) URL")
	}
	if strings.TrimSpace(config.DatabaseURL) == "" {
		return fmt.Errorf("LOADTEST_DATABASE_URL must not be empty")
	}
	if config.Users <= 0 || config.Products <= 0 || config.ProductsPerUser <= 0 {
		return fmt.Errorf("LOADTEST_USERS, LOADTEST_PRODUCTS, and LOADTEST_PRODUCTS_PER_USER must be positive")
	}
	if config.ProductsPerUser > config.Products {
		return fmt.Errorf("LOADTEST_PRODUCTS_PER_USER must not exceed LOADTEST_PRODUCTS")
	}
	if config.QueueCapacity < 1 || config.QueueCapacity > 1000 {
		return fmt.Errorf("LOADTEST_QUEUE_CAPACITY must be between 1 and 1000")
	}
	if int64(config.Users)*int64(config.ProductsPerUser) > int64(config.Products)*int64(config.QueueCapacity) {
		return fmt.Errorf("requested user-product links exceed LOADTEST_QUEUE_CAPACITY across all products")
	}
	if config.DuplicateJoinPercent < 0 || config.DuplicateJoinPercent > 100 {
		return fmt.Errorf("LOADTEST_DUPLICATE_JOIN_PERCENT must be between 0 and 100")
	}
	if config.MinStock < 1 || config.MaxStock < config.MinStock {
		return fmt.Errorf("LOADTEST_MIN_STOCK must be positive and not exceed LOADTEST_MAX_STOCK")
	}
	if int64(config.MaxStock) > int64(^uint32(0)>>1) {
		return fmt.Errorf("LOADTEST_MAX_STOCK must fit a PostgreSQL INTEGER")
	}
	if config.RampDuration <= 0 || config.PollInterval <= 0 || config.PollDuration <= 0 {
		return fmt.Errorf("load-test durations must be positive")
	}
	return nil
}

func (config Config) Effective() EffectiveConfig {
	return EffectiveConfig{
		Config: config, RampDuration: config.RampDuration.String(),
		PollInterval: config.PollInterval.String(), PollDuration: config.PollDuration.String(),
	}
}

func RunPrefix(runID string) (string, error) {
	if !runIDPattern.MatchString(runID) {
		return "", fmt.Errorf("unsafe load-test run ID")
	}
	return "LT-" + runID + "-", nil
}

func value(lookup LookupEnv, key, fallback string) string {
	if raw, exists := lookup(key); exists && strings.TrimSpace(raw) != "" {
		return strings.TrimSpace(raw)
	}
	return fallback
}

func intValue(lookup LookupEnv, key string, fallback int) (int, error) {
	raw := value(lookup, key, strconv.Itoa(fallback))
	parsed, err := strconv.Atoi(raw)
	if err != nil {
		return 0, fmt.Errorf("%s must be an integer: %w", key, err)
	}
	return parsed, nil
}

func int64Value(lookup LookupEnv, key string, fallback int64) (int64, error) {
	raw := value(lookup, key, strconv.FormatInt(fallback, 10))
	parsed, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("%s must be an integer: %w", key, err)
	}
	return parsed, nil
}

func durationValue(lookup LookupEnv, key string, fallback time.Duration) (time.Duration, error) {
	raw := value(lookup, key, fallback.String())
	parsed, err := time.ParseDuration(raw)
	if err != nil || parsed <= 0 {
		return 0, fmt.Errorf("%s must be a positive Go duration", key)
	}
	return parsed, nil
}

func boolValue(lookup LookupEnv, key string, fallback bool) (bool, error) {
	raw := value(lookup, key, strconv.FormatBool(fallback))
	parsed, err := strconv.ParseBool(raw)
	if err != nil {
		return false, fmt.Errorf("%s must be a boolean: %w", key, err)
	}
	return parsed, nil
}
