package loadtestrunner

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strconv"
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
	results  string
	result   loadtest.VerificationResult
	block    <-chan struct{}
	failAt   int
}

func (stub *commandRunnerStub) Run(_ context.Context, command Command) (int, error) {
	stub.mu.Lock()
	stub.commands = append(stub.commands, command)
	call := len(stub.commands)
	stub.mu.Unlock()
	if stub.block != nil && call == 1 {
		<-stub.block
	}
	if stub.failAt == call && call != 2 {
		return 23, errors.New("process failed")
	}
	if call == 2 {
		path := filepath.Join(stub.results, stub.result.RunID, "verifier.json")
		if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
			return -1, err
		}
		contents, _ := json.Marshal(stub.result)
		if err := os.WriteFile(path, contents, 0o600); err != nil {
			return -1, err
		}
	}
	if stub.failAt == call {
		return 23, errors.New("process failed")
	}
	return 0, nil
}

func TestRunnerAcceptsAllProfilesAndScenarios(t *testing.T) {
	for _, profile := range []string{loadtest.ProfileSmoke, loadtest.ProfileMedium, loadtest.ProfileMain} {
		for _, scenario := range []string{loadtest.ScenarioQueueJoinPolling, loadtest.ScenarioPurchaseOutcomes} {
			t.Run(profile+"/"+scenario, func(t *testing.T) {
				runner, commands, request := newTestRunner(t, profile, scenario, nil)
				state, err := runner.Start(context.Background(), request)
				if err != nil || (state.Status != StatusStarting && state.Status != StatusRunning) {
					t.Fatalf("start state=%+v err=%v", state, err)
				}
				waitForState(t, runner, StatusCompleted)
				commands.mu.Lock()
				defer commands.mu.Unlock()
				if len(commands.commands) != 2 {
					t.Fatalf("commands=%d, want k6 and verifier", len(commands.commands))
				}
			})
		}
	}
}

func TestRunnerRejectsMissingAndMismatchedFixtures(t *testing.T) {
	config := testRunnerConfig(t)
	runner := New(config, nil, &commandRunnerStub{}, nil)
	request := RunRequest{RunID: "missing", Profile: "smoke", Scenario: loadtest.ScenarioQueueJoinPolling}
	if _, err := runner.Start(context.Background(), request); !errors.Is(err, ErrInvalidFixture) {
		t.Fatalf("missing fixture err=%v", err)
	}
	writeFixture(t, config, RunRequest{RunID: "mismatch", Profile: "medium", Scenario: request.Scenario})
	request.RunID = "mismatch"
	if _, err := runner.Start(context.Background(), request); !errors.Is(err, ErrInvalidFixture) {
		t.Fatalf("mismatched fixture err=%v", err)
	}
}

func TestRunnerRejectsRunIDWithExistingResults(t *testing.T) {
	config := testRunnerConfig(t)
	request := RunRequest{RunID: "used-run", Profile: "smoke", Scenario: loadtest.ScenarioQueueJoinPolling}
	writeFixture(t, config, request)
	resultDir := filepath.Join(config.ResultsDir, request.RunID)
	if err := os.MkdirAll(resultDir, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(resultDir, "verifier.json"), []byte(`{}`), 0o600); err != nil {
		t.Fatal(err)
	}
	runner := New(config, nil, &commandRunnerStub{}, nil)
	if _, err := runner.Start(context.Background(), request); !errors.Is(err, ErrInvalidFixture) {
		t.Fatalf("existing results err=%v", err)
	}
}

func TestRunnerAllowsOnlyOneConcurrentRun(t *testing.T) {
	block := make(chan struct{})
	runner, commands, request := newTestRunner(t, "smoke", loadtest.ScenarioQueueJoinPolling, block)
	if _, err := runner.Start(context.Background(), request); err != nil {
		t.Fatal(err)
	}
	waitForState(t, runner, StatusRunning)
	const callers = 20
	var group sync.WaitGroup
	group.Add(callers)
	conflicts := make(chan error, callers)
	for range callers {
		go func() {
			defer group.Done()
			_, err := runner.Start(context.Background(), request)
			conflicts <- err
		}()
	}
	group.Wait()
	close(conflicts)
	for err := range conflicts {
		if !errors.Is(err, ErrAlreadyRunning) {
			t.Fatalf("concurrent start err=%v", err)
		}
	}
	commands.mu.Lock()
	if len(commands.commands) != 1 {
		t.Fatalf("started commands=%d, want one", len(commands.commands))
	}
	commands.mu.Unlock()
	close(block)
	waitForState(t, runner, StatusCompleted)
}

func TestRunnerMapsK6AndVerifierFailures(t *testing.T) {
	for _, failAt := range []int{1, 2} {
		t.Run(strconv.Itoa(failAt), func(t *testing.T) {
			runner, commands, request := newTestRunner(t, "smoke", loadtest.ScenarioQueueJoinPolling, nil)
			commands.failAt = failAt
			if _, err := runner.Start(context.Background(), request); err != nil {
				t.Fatal(err)
			}
			state := waitForState(t, runner, StatusFailed)
			if state.ExitCode == nil || *state.ExitCode != 23 {
				t.Fatalf("exit code=%v", state.ExitCode)
			}
			if failAt == 2 {
				want := `# HELP goodqueue_loadtest_verifier_total Verifier executions by result.
# TYPE goodqueue_loadtest_verifier_total counter
goodqueue_loadtest_verifier_total{result="fail"} 1
`
				if err := testutil.GatherAndCompare(runner.metrics.registry, strings.NewReader(want), "goodqueue_loadtest_verifier_total"); err != nil {
					t.Fatal(err)
				}
			}
		})
	}
}

func newTestRunner(t *testing.T, profile, scenario string, block <-chan struct{}) (*Runner, *commandRunnerStub, RunRequest) {
	t.Helper()
	config := testRunnerConfig(t)
	scenarioID := "queue"
	if scenario == loadtest.ScenarioPurchaseOutcomes {
		scenarioID = "purchase"
	}
	request := RunRequest{RunID: "run-" + profile + "-" + scenarioID, Profile: profile, Scenario: scenario}
	writeFixture(t, config, request)
	commands := &commandRunnerStub{results: config.ResultsDir, result: loadtest.VerificationResult{RunID: request.RunID, Passed: true}, block: block}
	return New(config, nil, commands, nil), commands, request
}

func testRunnerConfig(t *testing.T) Config {
	t.Helper()
	root := t.TempDir()
	return Config{Enabled: true, BaseURL: "http://backend:8080", DatabaseURL: "postgres://db", PrometheusWriteURL: "http://prometheus/api/v1/write", GeneratedDir: filepath.Join(root, "generated"), ResultsDir: filepath.Join(root, "results"), ScriptsDir: "/scripts", K6Binary: "k6", VerifierBinary: "verify"}
}

func writeFixture(t *testing.T, config Config, request RunRequest) {
	t.Helper()
	loaded, err := loadtest.LoadConfigFrom(func(key string) (string, bool) {
		values := map[string]string{"LOADTEST_RUN_ID": request.RunID, "LOADTEST_PROFILE": request.Profile, "LOADTEST_SCENARIO": request.Scenario}
		value, ok := values[key]
		return value, ok
	})
	if err != nil {
		t.Fatal(err)
	}
	data, err := loadtest.GenerateData(loaded)
	if err != nil {
		t.Fatal(err)
	}
	if err := loadtest.WriteData(filepath.Join(config.GeneratedDir, request.RunID, "data.json"), data); err != nil {
		t.Fatal(err)
	}
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
