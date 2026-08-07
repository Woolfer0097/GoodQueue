package loadtest

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadtestDotenvSuppliesDefaultsWithoutOverridingEnvironment(t *testing.T) {
	root := repositoryRoot(t)
	envPath := filepath.Join(t.TempDir(), "loadtest.env")
	contents := "LOADTEST_USERS=10\n\nK6_PROMETHEUS_RW_TREND_STATS=\"avg,min,max,p(90)\"\n"
	if err := os.WriteFile(envPath, []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}
	command := exec.Command("sh", "-c", //nolint:gosec // Static test command with a temporary fixture path.
		`set -eu; loadtest_env_file="$1"; . ./scripts/loadtest-env-defaults.sh; printf '%s|%s' "$LOADTEST_USERS" "$K6_PROMETHEUS_RW_TREND_STATS"`,
		"sh", envPath,
	)
	command.Dir = root
	command.Env = append(os.Environ(), "LOADTEST_USERS=3")
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("source dotenv defaults: %v: %s", err, output)
	}
	if strings.TrimSpace(string(output)) != "3|avg,min,max,p(90)" {
		t.Fatalf("unexpected dotenv precedence: %q", output)
	}
}
