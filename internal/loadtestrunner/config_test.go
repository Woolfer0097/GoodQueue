package loadtestrunner

import "testing"

func TestConfigDefaultsToDisabled(t *testing.T) {
	t.Setenv("LOADTEST_RUNNER_ENABLED", "")
	config, err := LoadConfig()
	if err != nil || config.Enabled {
		t.Fatalf("config=%+v err=%v", config, err)
	}
}

func TestConfigRejectsInvalidEnabledFlag(t *testing.T) {
	t.Setenv("LOADTEST_RUNNER_ENABLED", "sometimes")
	if _, err := LoadConfig(); err == nil {
		t.Fatal("expected invalid enabled flag to fail")
	}
}
