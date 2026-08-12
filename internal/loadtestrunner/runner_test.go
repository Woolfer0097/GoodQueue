package loadtestrunner

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Woolfer0097/GoodQueue/internal/loadtest"
	"github.com/prometheus/client_golang/prometheus/testutil"
)

type commandRunnerStub struct {
	mu       sync.Mutex
	commands []Command
	config   Config
	block    <-chan struct{}
	failStep string
}

func (stub *commandRunnerStub) Run(_ context.Context, command Command) (int, error) {
	stub.mu.Lock()
	stub.commands = append(stub.commands, command)
	stub.mu.Unlock()
	if stub.block != nil && command.Name == stub.config.SeedBinary && len(command.Args) == 0 {
		<-stub.block
	}
	if commandStep(command, stub.config) == stub.failStep {
		return 23, errors.New("process failed")
	}
	environment := environmentMap(command.Env)
	if command.Name == stub.config.SeedBinary && len(command.Args) == 0 {
		loaded, err := loadtest.LoadConfigFrom(func(key string) (string, bool) {
			value, exists := environment[key]
			return value, exists
		})
		if err != nil {
			return -1, err
		}
		data, err := loadtest.GenerateData(loaded)
		if err != nil {
			return -1, err
		}
		if err := loadtest.WriteData(environment["LOADTEST_DATA_FILE"], data); err != nil {
			return -1, err
		}
	}
	if command.Name == stub.config.VerifierBinary {
		path := filepath.Join(stub.config.ResultsDir, environment["LOADTEST_RUN_ID"], "verifier.json")
		if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
			return -1, err
		}
		contents, _ := json.Marshal(loadtest.VerificationResult{RunID: environment["LOADTEST_RUN_ID"], Passed: true})
		if err := os.WriteFile(path, contents, 0o600); err != nil {
			return -1, err
		}
	}
	return 0, nil
}

func TestRunnerAcceptsAllProfilesAndScenarios(t *testing.T) {
	for _, profile := range []string{loadtest.ProfileSmoke, loadtest.ProfileMedium, loadtest.ProfileMain} {
		for _, scenario := range []string{loadtest.ScenarioQueueJoinPolling, loadtest.ScenarioPurchaseOutcomes} {
			t.Run(profile+"/"+scenario, func(t *testing.T) {
				runner, commands, request := newTestRunner(t, profile, scenario, nil)
				state, err := runner.Start(context.Background(), request)
				if err != nil {
					t.Fatalf("start: %v", err)
				}
				if !strings.HasPrefix(state.RunID, "ui-") || state.KeepData != *request.KeepData {
					t.Fatalf("generated state=%+v", state)
				}
				waitForState(t, runner, StatusCompleted)
				commands.mu.Lock()
				defer commands.mu.Unlock()
				if len(commands.commands) != 4 || commands.commands[0].Name != "seed" || commands.commands[1].Name != "seed" || commands.commands[2].Name != "k6" || commands.commands[3].Name != "verify" {
					t.Fatalf("commands=%s", commandNames(commands.commands))
				}
			})
		}
	}
}

func TestRunnerRejectsInvalidRequest(t *testing.T) {
	runner, _, request := newTestRunner(t, "smoke", loadtest.ScenarioQueueJoinPolling, nil)
	request.Profile = "huge"
	if _, err := runner.Start(context.Background(), request); !errors.Is(err, ErrInvalidRequest) {
		t.Fatalf("invalid profile err=%v", err)
	}
	request.Profile, request.KeepData = "smoke", nil
	if _, err := runner.Start(context.Background(), request); !errors.Is(err, ErrInvalidRequest) {
		t.Fatalf("missing keepData err=%v", err)
	}
}

func TestRunnerAllowsOnlyOneConcurrentRun(t *testing.T) {
	block := make(chan struct{})
	runner, _, request := newTestRunner(t, "smoke", loadtest.ScenarioQueueJoinPolling, block)
	if _, err := runner.Start(context.Background(), request); err != nil {
		t.Fatal(err)
	}
	waitForState(t, runner, StatusSeeding)
	if _, err := runner.Start(context.Background(), request); !errors.Is(err, ErrAlreadyRunning) {
		t.Fatalf("concurrent start err=%v", err)
	}
	close(block)
	waitForState(t, runner, StatusCompleted)
}

func TestRunnerStopsAfterSeedOrK6FailureAndPreservesSeededFailure(t *testing.T) {
	for _, failed := range []string{"cleanup", "seed", "k6", "verify"} {
		t.Run(failed, func(t *testing.T) {
			runner, commands, request := newTestRunner(t, "smoke", loadtest.ScenarioQueueJoinPolling, nil)
			commands.failStep = failed
			if _, err := runner.Start(context.Background(), request); err != nil {
				t.Fatal(err)
			}
			state := waitForState(t, runner, StatusFailed)
			if state.ExitCode == nil || *state.ExitCode != 23 {
				t.Fatalf("exit code=%v", state.ExitCode)
			}
			commands.mu.Lock()
			got := commandNames(commands.commands)
			commands.mu.Unlock()
			switch failed {
			case "cleanup":
				if got != "seed --cleanup-disposable-ui" {
					t.Fatalf("commands after cleanup failure=%s", got)
				}
			case "seed":
				if strings.Contains(got, "|k6") || strings.Contains(got, "|verify") {
					t.Fatalf("commands after seed failure=%s", got)
				}
				if !strings.Contains(got, "seed --mark-failed") || !state.KeepData {
					t.Fatalf("seed failure was not retained: state=%+v commands=%s", state, got)
				}
			case "k6", "verify":
				if !strings.Contains(got, "seed --mark-failed") || !state.KeepData {
					t.Fatalf("failed run was not preserved: state=%+v commands=%s", state, got)
				}
			}
		})
	}
}

func TestRunnerVerifierFailureMetric(t *testing.T) {
	runner, commands, request := newTestRunner(t, "smoke", loadtest.ScenarioQueueJoinPolling, nil)
	commands.failStep = "verify"
	_, _ = runner.Start(context.Background(), request)
	waitForState(t, runner, StatusFailed)
	want := `# HELP goodqueue_loadtest_verifier_total Verifier executions by result.
# TYPE goodqueue_loadtest_verifier_total counter
goodqueue_loadtest_verifier_total{result="fail"} 1
`
	if err := testutil.GatherAndCompare(runner.metrics.registry, strings.NewReader(want), "goodqueue_loadtest_verifier_total"); err != nil {
		t.Fatal(err)
	}
}

func newTestRunner(t *testing.T, profile, scenario string, block <-chan struct{}) (*Runner, *commandRunnerStub, RunRequest) {
	t.Helper()
	config := testRunnerConfig(t)
	keep := true
	request := RunRequest{Profile: profile, Scenario: scenario, KeepData: &keep}
	commands := &commandRunnerStub{config: config, block: block}
	return New(config, nil, commands, nil), commands, request
}

func testRunnerConfig(t *testing.T) Config {
	t.Helper()
	root := t.TempDir()
	return Config{Enabled: true, BaseURL: "http://backend:8080", DatabaseURL: "postgres://db", PrometheusWriteURL: "http://prometheus/api/v1/write", GeneratedDir: filepath.Join(root, "generated"), ResultsDir: filepath.Join(root, "results"), ScriptsDir: "/scripts", K6Binary: "k6", SeedBinary: "seed", VerifierBinary: "verify"}
}

func environmentMap(environment []string) map[string]string {
	result := make(map[string]string)
	for _, item := range environment {
		key, value, ok := strings.Cut(item, "=")
		if ok {
			result[key] = value
		}
	}
	return result
}

func commandNames(commands []Command) string {
	result := make([]string, 0, len(commands))
	for _, command := range commands {
		name := command.Name
		if len(command.Args) > 0 {
			name += " " + strings.Join(command.Args, " ")
		}
		result = append(result, name)
	}
	return strings.Join(result, "|")
}

func commandStep(command Command, config Config) string {
	if command.Name == config.SeedBinary {
		if len(command.Args) == 0 {
			return "seed"
		}
		if command.Args[0] == "--cleanup-disposable-ui" {
			return "cleanup"
		}
		if command.Args[0] == "--mark-failed" {
			return "preserve"
		}
	}
	return command.Name
}

func waitForState(t *testing.T, runner *Runner, want string) State {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		state := runner.Current()
		if state.Status == want {
			return state
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("state=%+v, want %s", runner.Current(), want)
	return State{}
}
