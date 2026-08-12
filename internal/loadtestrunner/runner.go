package loadtestrunner

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"sync"
	"time"

	"github.com/Woolfer0097/GoodQueue/internal/loadtest"
	"go.uber.org/zap"
)

const (
	StatusIdle      = "idle"
	StatusStarting  = "starting"
	StatusCleaning  = "cleaning"
	StatusSeeding   = "seeding"
	StatusRunning   = "running"
	StatusVerifying = "verifying"
	StatusCompleted = "completed"
	StatusFailed    = "failed"
)

var (
	ErrAlreadyRunning = errors.New("a load test is already running")
	ErrInvalidRequest = errors.New("invalid load-test request")
	ErrInvalidFixture = errors.New("invalid load-test fixture")
)

type RunRequest struct {
	Profile  string `json:"profile"`
	Scenario string `json:"scenario"`
	KeepData *bool  `json:"keepData"`
}

type State struct {
	RunID          string     `json:"runId,omitempty"`
	Profile        string     `json:"profile,omitempty"`
	Scenario       string     `json:"scenario,omitempty"`
	KeepData       bool       `json:"keepData"`
	Status         string     `json:"status"`
	StartedAt      *time.Time `json:"startedAt"`
	FinishedAt     *time.Time `json:"finishedAt"`
	ExitCode       *int       `json:"exitCode"`
	SeedStatus     string     `json:"seedStatus"`
	VerifierStatus string     `json:"verifierStatus"`
	Error          string     `json:"error,omitempty"`
}

type Command struct {
	Name string
	Args []string
	Env  []string
	Dir  string
}

type CommandRunner interface {
	Run(context.Context, Command) (int, error)
}

type OSCommandRunner struct{}

func (OSCommandRunner) Run(ctx context.Context, command Command) (int, error) {
	process := exec.CommandContext(ctx, command.Name, command.Args...) //nolint:gosec // Executables and arguments come from validated configuration.
	process.Env, process.Dir = command.Env, command.Dir
	process.Stdout, process.Stderr = os.Stdout, os.Stderr
	err := process.Run()
	if err == nil {
		return 0, nil
	}
	var exitError *exec.ExitError
	if errors.As(err, &exitError) {
		return exitError.ExitCode(), err
	}
	return -1, err
}

type Runner struct {
	config   Config
	log      *zap.Logger
	commands CommandRunner
	metrics  *Metrics
	now      func() time.Time

	mu    sync.RWMutex
	state State
}

func New(config Config, log *zap.Logger, commands CommandRunner, metrics *Metrics) *Runner {
	if log == nil {
		log = zap.NewNop()
	}
	if commands == nil {
		commands = OSCommandRunner{}
	}
	if metrics == nil {
		metrics = NewMetrics()
	}
	return &Runner{config: config, log: log, commands: commands, metrics: metrics, now: time.Now, state: State{Status: StatusIdle, SeedStatus: "pending", VerifierStatus: "pending"}}
}

func (runner *Runner) Current() State {
	runner.mu.RLock()
	defer runner.mu.RUnlock()
	return runner.state
}

func (runner *Runner) Start(ctx context.Context, request RunRequest) (State, error) {
	runner.mu.Lock()
	if isActive(runner.state.Status) {
		state := runner.state
		runner.mu.Unlock()
		return state, ErrAlreadyRunning
	}
	if request.KeepData == nil {
		runner.mu.Unlock()
		return runner.Current(), fmt.Errorf("%w: keepData is required", ErrInvalidRequest)
	}
	runID, err := newRunID(runner.now())
	if err != nil {
		runner.mu.Unlock()
		return runner.Current(), fmt.Errorf("generate run ID: %w", err)
	}
	effective, err := runner.requestConfig(request, runID)
	if err != nil {
		runner.mu.Unlock()
		return runner.Current(), fmt.Errorf("%w: %w", ErrInvalidRequest, err)
	}
	runner.state = State{RunID: runID, Profile: request.Profile, Scenario: request.Scenario, KeepData: *request.KeepData, Status: StatusStarting, SeedStatus: "pending", VerifierStatus: "pending"}
	runner.metrics.setInfo(request.Profile, request.Scenario, StatusStarting)
	runner.metrics.setCurrentRun(runID, request.Profile, request.Scenario)
	runner.metrics.setSeedStatus("pending")
	runner.mu.Unlock()
	started := runner.now().UTC()
	runner.mu.Lock()
	runner.state.StartedAt = &started
	runner.mu.Unlock()
	go runner.execute(context.WithoutCancel(ctx), request, effective, started)
	return runner.Current(), nil
}

func (runner *Runner) requestConfig(request RunRequest, runID string) (loadtest.Config, error) {
	lookup := func(key string) (string, bool) {
		values := map[string]string{
			"LOADTEST_RUN_ID": runID, "LOADTEST_PROFILE": request.Profile,
			"LOADTEST_SCENARIO": request.Scenario, "LOADTEST_SOURCE": loadtest.SourceRunnerUI,
			"LOADTEST_KEEP_DATA": strconv.FormatBool(*request.KeepData),
		}
		value, ok := values[key]
		return value, ok
	}
	return loadtest.LoadConfigFrom(lookup)
}

func (runner *Runner) validateFixture(expected loadtest.Config) (loadtest.Data, string, error) {
	dataPath := filepath.Join(runner.config.GeneratedDir, expected.RunID, "data.json")
	data, err := loadtest.ReadData(dataPath)
	if err != nil {
		return loadtest.Data{}, "", fmt.Errorf("%w: %w", ErrInvalidFixture, err)
	}
	if data.RunID != expected.RunID || data.EffectiveConfig.Profile != expected.Profile || data.EffectiveConfig.Scenario != expected.Scenario {
		return loadtest.Data{}, "", fmt.Errorf("%w: data.json does not match runId, profile, and scenario", ErrInvalidFixture)
	}
	return data, dataPath, nil
}

func (runner *Runner) execute(ctx context.Context, request RunRequest, effective loadtest.Config, started time.Time) {
	environment := runner.configEnvironment(effective)
	runner.metrics.recordStarted(request.Profile, request.Scenario, started)
	runner.setStatus(StatusCleaning, "pending")
	runner.log.Info("load test started", zap.String("run_id", effective.RunID), zap.String("profile", request.Profile), zap.String("scenario", request.Scenario))
	if exitCode, err := runner.commands.Run(ctx, Command{Name: runner.config.SeedBinary, Args: []string{"--cleanup-disposable-ui"}, Env: environment, Dir: "/work"}); err != nil {
		runner.finish(request, started, exitCode, fmt.Errorf("cleanup failed: %w", err))
		return
	}

	runner.setSeedStatus(StatusSeeding, "running")
	if exitCode, err := runner.commands.Run(ctx, Command{Name: runner.config.SeedBinary, Env: environment, Dir: "/work"}); err != nil {
		runner.setSeedStatus(StatusFailed, "fail")
		runner.preserveFailure(ctx, environment)
		runner.finish(request, started, exitCode, fmt.Errorf("seed failed: %w", err))
		return
	}
	data, dataPath, err := runner.validateFixture(effective)
	if err != nil {
		runner.setSeedStatus(StatusFailed, "fail")
		runner.preserveFailure(ctx, environment)
		runner.finish(request, started, -1, err)
		return
	}
	runner.setSeedStatus(StatusRunning, "pass")

	environment = runner.commandEnvironment(data, dataPath)
	script := "queue-join-polling.js"
	args := []string{"run", "-o", "experimental-prometheus-rw"}
	if request.Scenario == loadtest.ScenarioPurchaseOutcomes {
		script = "queue-purchase-outcomes.js"
		eventsPath := filepath.Join(runner.config.ResultsDir, effective.RunID, "k6-events.log")
		if err := os.MkdirAll(filepath.Dir(eventsPath), 0o750); err != nil {
			runner.preserveFailure(ctx, environment)
			runner.finish(request, started, -1, fmt.Errorf("create results directory: %w", err))
			return
		}
		args = append(args, "--log-format=raw", "--log-output=file="+eventsPath)
	}
	args = append(args, filepath.Join(runner.config.ScriptsDir, script))
	runner.log.Info("k6 process started", zap.String("run_id", effective.RunID), zap.String("script", script))
	exitCode, err := runner.commands.Run(ctx, Command{Name: runner.config.K6Binary, Args: args, Env: environment, Dir: "/work"})
	runner.log.Info("k6 process exited", zap.String("run_id", effective.RunID), zap.Int("exit_code", exitCode), zap.Error(err))
	if err != nil {
		runner.preserveFailure(ctx, environment)
		runner.finish(request, started, exitCode, fmt.Errorf("k6 failed: %w", err))
		return
	}

	runner.setStatus(StatusVerifying, "running")
	runner.metrics.events.WithLabelValues("VERIFIER START").Inc()
	runner.log.Info("verifier started", zap.String("run_id", effective.RunID))
	verifyExitCode, verifyErr := runner.commands.Run(ctx, Command{Name: runner.config.VerifierBinary, Env: environment, Dir: "/work"})
	verification, reportErr := readVerification(filepath.Join(runner.config.ResultsDir, effective.RunID, "verifier.json"))
	if reportErr == nil {
		runner.metrics.recordVerification(verification)
	} else {
		runner.metrics.clearVerification()
	}
	verifierPassed := verifyErr == nil && reportErr == nil && verification.Passed
	runner.metrics.recordVerifierResult(verifierPassed)
	runner.log.Info("verifier completed", zap.String("run_id", effective.RunID), zap.Int("exit_code", verifyExitCode), zap.Bool("passed", verifierPassed), zap.Error(verifyErr), zap.Error(reportErr))
	if !verifierPassed {
		runner.preserveFailure(ctx, environment)
		runner.finish(request, started, verifyExitCode, errors.Join(verifyErr, reportErr, errors.New("verifier failed")))
		return
	}
	runner.mu.Lock()
	runner.state.VerifierStatus = "pass"
	runner.mu.Unlock()
	runner.finish(request, started, 0, nil)
}

func (runner *Runner) configEnvironment(config loadtest.Config) []string {
	data := loadtest.Data{RunID: config.RunID, EffectiveConfig: config.Effective()}
	return runner.commandEnvironment(data, filepath.Join(runner.config.GeneratedDir, config.RunID, "data.json"))
}

func (runner *Runner) preserveFailure(ctx context.Context, environment []string) {
	_, err := runner.commands.Run(ctx, Command{Name: runner.config.SeedBinary, Args: []string{"--mark-failed"}, Env: environment, Dir: "/work"})
	if err != nil {
		runner.log.Error("failed load test could not be marked for retention", zap.Error(err))
	}
	runner.mu.Lock()
	runner.state.KeepData = true
	runner.mu.Unlock()
}

func (runner *Runner) commandEnvironment(data loadtest.Data, dataPath string) []string {
	effective := data.EffectiveConfig
	values := map[string]string{
		"LOADTEST_RUN_ID": data.RunID, "LOADTEST_PROFILE": effective.Profile, "LOADTEST_SCENARIO": effective.Scenario,
		"LOADTEST_SOURCE": loadtest.SourceRunnerUI, "LOADTEST_KEEP_DATA": strconv.FormatBool(effective.KeepData),
		"LOADTEST_BASE_URL": runner.config.BaseURL, "LOADTEST_DATABASE_URL": runner.config.DatabaseURL,
		"LOADTEST_DATA_FILE": dataPath, "LOADTEST_RESULTS_DIR": runner.config.ResultsDir,
		"LOADTEST_RANDOM_SEED": strconv.FormatInt(effective.RandomSeed, 10),
		"LOADTEST_USERS":       strconv.Itoa(effective.Users), "LOADTEST_PRODUCTS": strconv.Itoa(effective.Products),
		"LOADTEST_PRODUCTS_PER_USER": strconv.Itoa(effective.ProductsPerUser), "LOADTEST_RAMP_DURATION": effective.RampDuration,
		"LOADTEST_POLL_INTERVAL": effective.PollInterval, "LOADTEST_POLL_DURATION": effective.PollDuration,
		"LOADTEST_OUTCOME_TIMEOUT": effective.OutcomeTimeout, "LOADTEST_QUEUE_CAPACITY": strconv.Itoa(effective.QueueCapacity),
		"LOADTEST_DUPLICATE_JOIN_PERCENT": strconv.Itoa(effective.DuplicateJoinPercent),
		"LOADTEST_MIN_STOCK":              strconv.Itoa(effective.MinStock), "LOADTEST_MAX_STOCK": strconv.Itoa(effective.MaxStock),
		"K6_PROMETHEUS_RW_SERVER_URL":  runner.config.PrometheusWriteURL,
		"K6_PROMETHEUS_RW_TREND_STATS": "avg,min,max,p(90),p(95),p(99)", "K6_PROMETHEUS_RW_STALE_MARKERS": "false",
	}
	base := make(map[string]string)
	for _, item := range os.Environ() {
		for index := range item {
			if item[index] == '=' {
				base[item[:index]] = item[index+1:]
				break
			}
		}
	}
	for key, value := range values {
		base[key] = value
	}
	environment := make([]string, 0, len(base))
	for key, value := range base {
		environment = append(environment, key+"="+value)
	}
	return environment
}

func (runner *Runner) setStatus(status, verifier string) {
	runner.mu.Lock()
	runner.state.Status, runner.state.VerifierStatus = status, verifier
	profile, scenario := runner.state.Profile, runner.state.Scenario
	runner.mu.Unlock()
	runner.metrics.setInfo(profile, scenario, status)
}

func (runner *Runner) setSeedStatus(status, seed string) {
	runner.mu.Lock()
	runner.state.Status, runner.state.SeedStatus = status, seed
	profile, scenario := runner.state.Profile, runner.state.Scenario
	runner.mu.Unlock()
	runner.metrics.setInfo(profile, scenario, status)
	runner.metrics.setSeedStatus(seed)
}

func (runner *Runner) finish(request RunRequest, started time.Time, exitCode int, err error) {
	finished := runner.now().UTC()
	status, result := StatusCompleted, "success"
	if err != nil {
		status, result = StatusFailed, "failed"
	}
	runner.metrics.recordFinished(request.Profile, request.Scenario, result, finished.Sub(started))
	runner.metrics.setInfo(request.Profile, request.Scenario, status)
	runner.mu.Lock()
	runner.state.Status, runner.state.FinishedAt, runner.state.ExitCode = status, &finished, &exitCode
	runID := runner.state.RunID
	if err != nil {
		runner.state.Error = err.Error()
		if runner.state.VerifierStatus == "running" {
			runner.state.VerifierStatus = "fail"
		}
	}
	runner.mu.Unlock()
	if err != nil {
		runner.log.Error("load test failed", zap.String("run_id", runID), zap.Error(err))
		return
	}
	runner.log.Info("load test completed", zap.String("run_id", runID), zap.Duration("duration", finished.Sub(started)))
}

func isActive(status string) bool {
	return status == StatusStarting || status == StatusCleaning || status == StatusSeeding || status == StatusRunning || status == StatusVerifying
}

func newRunID(now time.Time) (string, error) {
	var suffix [4]byte
	if _, err := rand.Read(suffix[:]); err != nil {
		return "", err
	}
	return fmt.Sprintf("ui-%s-%x", now.UTC().Format("20060102T150405"), suffix), nil
}

func readVerification(path string) (loadtest.VerificationResult, error) {
	contents, err := os.ReadFile(path) //nolint:gosec // Path is derived from the validated run ID and configured results directory.
	if err != nil {
		return loadtest.VerificationResult{}, fmt.Errorf("read verifier report: %w", err)
	}
	var result loadtest.VerificationResult
	if err := json.Unmarshal(contents, &result); err != nil {
		return loadtest.VerificationResult{}, fmt.Errorf("decode verifier report: %w", err)
	}
	return result, nil
}
