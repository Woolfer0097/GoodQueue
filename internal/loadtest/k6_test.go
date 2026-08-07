package loadtest

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestK6MetricTagsDoNotContainIdentifiers(t *testing.T) {
	t.Parallel()
	root := repositoryRoot(t)
	files, err := filepath.Glob(filepath.Join(root, "loadtest", "k6", "*.js"))
	if err != nil {
		t.Fatal(err)
	}
	libraryFiles, err := filepath.Glob(filepath.Join(root, "loadtest", "k6", "lib", "*.js"))
	if err != nil {
		t.Fatal(err)
	}
	files = append(files, libraryFiles...)
	for _, path := range files {
		contents, err := os.ReadFile(path) //nolint:gosec // Glob results are constrained to repository JavaScript files.
		if err != nil {
			t.Fatal(err)
		}
		text := string(contents)
		for _, forbidden := range []string{"user_id:", "product_id:", "attempt_id:", "idempotency_key:"} {
			if strings.Contains(text, "tags: {"+forbidden) || strings.Contains(text, ", "+forbidden) {
				t.Errorf("%s contains dynamic identifier metric tag %q", path, forbidden)
			}
		}
	}
}

func TestK6ScriptSyntaxWhenK6IsInstalled(t *testing.T) {
	k6, err := exec.LookPath("k6")
	if err != nil {
		t.Skip("k6 is not installed")
	}
	root := repositoryRoot(t)
	for _, scenario := range []struct {
		name, script string
	}{
		{name: ScenarioQueueJoinPolling, script: "queue-join-polling.js"},
		{name: ScenarioPurchaseOutcomes, script: "queue-purchase-outcomes.js"},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			config, configErr := LoadConfigFrom(func(key string) (string, bool) {
				if key == "LOADTEST_SCENARIO" {
					return scenario.name, true
				}
				return "", false
			})
			if configErr != nil {
				t.Fatal(configErr)
			}
			data, generateErr := GenerateData(config)
			if generateErr != nil {
				t.Fatal(generateErr)
			}
			dataPath := filepath.Join(t.TempDir(), "data.json")
			if writeErr := WriteData(dataPath, data); writeErr != nil {
				t.Fatal(writeErr)
			}
			command := exec.Command( //nolint:gosec // The executable is resolved from PATH only when the optional tool is installed.
				k6, "inspect", "--include-system-env-vars",
				filepath.Join(root, "loadtest", "k6", scenario.script),
			)
			command.Env = append(os.Environ(), "LOADTEST_DATA_FILE="+dataPath, "LOADTEST_SCENARIO="+scenario.name)
			if output, inspectErr := command.CombinedOutput(); inspectErr != nil {
				t.Fatalf("k6 inspect failed: %v\n%s", inspectErr, output)
			}
		})
	}
}

func repositoryRoot(t *testing.T) string {
	t.Helper()
	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot locate current test file")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(currentFile), "..", ".."))
}
