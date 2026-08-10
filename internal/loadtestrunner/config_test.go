package loadtestrunner

import (
	"slices"
	"testing"
)

func TestConfigDefaultsToDisabled(t *testing.T) {
	t.Setenv("LOADTEST_RUNNER_ENABLED", "")
	t.Setenv("LOADTEST_RUNNER_CORS_ALLOWED_ORIGINS", "")
	config, err := LoadConfig()
	if err != nil || config.Enabled {
		t.Fatalf("config=%+v err=%v", config, err)
	}
	wantOrigins := []string{"http://localhost:8088", "http://127.0.0.1:8088"}
	if !slices.Equal(config.AllowedOrigins, wantOrigins) {
		t.Fatalf("allowed origins=%v want=%v", config.AllowedOrigins, wantOrigins)
	}
}

func TestConfigRejectsInvalidEnabledFlag(t *testing.T) {
	t.Setenv("LOADTEST_RUNNER_ENABLED", "sometimes")
	if _, err := LoadConfig(); err == nil {
		t.Fatal("expected invalid enabled flag to fail")
	}
}
