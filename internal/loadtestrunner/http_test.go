package loadtestrunner

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

func TestHTTPDisabledAndAPIKey(t *testing.T) {
	config := testRunnerConfig(t)
	config.Enabled = false
	runner := New(config, nil, &commandRunnerStub{}, nil)
	request := httptest.NewRequest(http.MethodGet, "/api/v1/loadtests/runs/current", nil)
	response := httptest.NewRecorder()
	NewHTTPHandler(config, runner).ServeHTTP(response, request)
	if response.Code != http.StatusNotFound {
		t.Fatalf("disabled status=%d", response.Code)
	}

	config.Enabled, config.APIKey = true, "secret-value"
	request = httptest.NewRequest(http.MethodGet, "/api/v1/loadtests/runs/current", nil)
	response = httptest.NewRecorder()
	NewHTTPHandler(config, runner).ServeHTTP(response, request)
	if response.Code != http.StatusUnauthorized || strings.Contains(response.Body.String(), config.APIKey) {
		t.Fatalf("unauthorized response=%d %s", response.Code, response.Body.String())
	}
	request = httptest.NewRequest(http.MethodGet, "/api/v1/loadtests/runs/current", nil)
	request.Header.Set("X-Loadtest-Api-Key", config.APIKey)
	response = httptest.NewRecorder()
	NewHTTPHandler(config, runner).ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("authorized status=%d body=%s", response.Code, response.Body.String())
	}
}

func TestRunnerLogsNeverContainAPIKey(t *testing.T) {
	config := testRunnerConfig(t)
	config.APIKey = "never-log-this-secret"
	var output bytes.Buffer
	encoder := zapcore.NewJSONEncoder(zap.NewProductionEncoderConfig())
	log := zap.New(zapcore.NewCore(encoder, zapcore.AddSync(&output), zap.DebugLevel))
	runner := New(config, log, &commandRunnerStub{}, nil)
	_, _ = runner.Start(t.Context(), RunRequest{RunID: "missing", Profile: "smoke", Scenario: "queue_join_polling"})
	if strings.Contains(output.String(), config.APIKey) {
		t.Fatal("runner log contains the configured API key")
	}
}

func TestHTTPStartAndConflict(t *testing.T) {
	block := make(chan struct{})
	runner, _, runRequest := newTestRunner(t, "smoke", "queue_join_polling", block)
	handler := NewHTTPHandler(runner.config, runner)
	body := []byte(`{"runId":"` + runRequest.RunID + `","profile":"smoke","scenario":"queue_join_polling"}`)

	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/api/v1/loadtests/runs", bytes.NewReader(body)))
	if response.Code != http.StatusAccepted {
		t.Fatalf("start status=%d body=%s", response.Code, response.Body.String())
	}
	waitForState(t, runner, StatusRunning)
	response = httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/api/v1/loadtests/runs", bytes.NewReader(body)))
	if response.Code != http.StatusConflict || !strings.Contains(response.Body.String(), "ALREADY RUNNING") {
		t.Fatalf("conflict status=%d body=%s", response.Code, response.Body.String())
	}
	close(block)
	waitForState(t, runner, StatusCompleted)
}

func TestCORSAllowsOnlyConfiguredGrafana(t *testing.T) {
	config := testRunnerConfig(t)
	config.AllowedOrigins = []string{"http://localhost:2002"}
	handler := NewHTTPHandler(config, New(config, nil, &commandRunnerStub{}, nil))
	for _, test := range []struct {
		origin string
		status int
	}{{"http://localhost:2002", 204}, {"https://example.com", 403}} {
		request := httptest.NewRequest(http.MethodOptions, "/api/v1/loadtests/runs", nil)
		request.Header.Set("Origin", test.origin)
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		if response.Code != test.status {
			t.Fatalf("origin=%s status=%d", test.origin, response.Code)
		}
	}
}
