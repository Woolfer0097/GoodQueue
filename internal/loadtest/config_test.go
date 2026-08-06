package loadtest

import (
	"strings"
	"testing"
	"time"
)

func TestLoadConfigProfileAndOverrides(t *testing.T) {
	t.Parallel()
	environment := map[string]string{
		"LOADTEST_PROFILE": "medium", "LOADTEST_USERS": "17", "LOADTEST_POLL_INTERVAL": "250ms",
		"LOADTEST_RUN_ID": "ci-17", "LOADTEST_DUPLICATE_JOIN_PERCENT": "25",
		"LOADTEST_SCENARIO": "purchase_outcomes", "LOADTEST_OUTCOME_TIMEOUT": "45s",
	}
	config, err := LoadConfigFrom(func(key string) (string, bool) {
		value, exists := environment[key]
		return value, exists
	})
	if err != nil {
		t.Fatalf("LoadConfigFrom() error = %v", err)
	}
	if config.Profile != ProfileMedium || config.Users != 17 || config.Products != 20 || config.ProductsPerUser != 5 {
		t.Fatalf("unexpected effective profile: %+v", config)
	}
	if config.PollInterval != 250*time.Millisecond || config.PollDuration != 2*time.Minute {
		t.Fatalf("unexpected durations: interval=%s duration=%s", config.PollInterval, config.PollDuration)
	}
	if config.Scenario != ScenarioPurchaseOutcomes || config.OutcomeTimeout != 45*time.Second {
		t.Fatalf("unexpected outcome config: scenario=%s timeout=%s", config.Scenario, config.OutcomeTimeout)
	}
}

func TestLoadConfigValidation(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name        string
		environment map[string]string
		want        string
	}{
		{name: "unknown profile", environment: map[string]string{"LOADTEST_PROFILE": "huge"}, want: "LOADTEST_PROFILE"},
		{name: "unknown scenario", environment: map[string]string{"LOADTEST_SCENARIO": "payments"}, want: "LOADTEST_SCENARIO"},
		{name: "unsafe run", environment: map[string]string{"LOADTEST_RUN_ID": "bad_run"}, want: "LOADTEST_RUN_ID"},
		{name: "too many per user", environment: map[string]string{"LOADTEST_PRODUCTS": "2", "LOADTEST_PRODUCTS_PER_USER": "3"}, want: "must not exceed"},
		{name: "invalid percent", environment: map[string]string{"LOADTEST_DUPLICATE_JOIN_PERCENT": "101"}, want: "between 0 and 100"},
		{name: "capacity overflow", environment: map[string]string{"LOADTEST_USERS": "10", "LOADTEST_PRODUCTS": "2", "LOADTEST_PRODUCTS_PER_USER": "2", "LOADTEST_QUEUE_CAPACITY": "5"}, want: "exceed"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			_, err := LoadConfigFrom(func(key string) (string, bool) {
				value, exists := test.environment[key]
				return value, exists
			})
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error = %v, want substring %q", err, test.want)
			}
		})
	}
}

func TestRunPrefixRejectsUnsafeFilterInput(t *testing.T) {
	t.Parallel()
	for _, runID := range []string{"", "run%", "run_1", "../../all", strings.Repeat("x", 41)} {
		if _, err := RunPrefix(runID); err == nil {
			t.Errorf("RunPrefix(%q) unexpectedly succeeded", runID)
		}
	}
	prefix, err := RunPrefix("main-42.test")
	if err != nil || prefix != "LT-main-42.test-" {
		t.Fatalf("RunPrefix() = %q, %v", prefix, err)
	}
}
