package loadtestrunner

import (
	"encoding/json"
	"errors"
	"net/http"
)

type HTTPHandler struct {
	config Config
	runner *Runner
}

func NewHTTPHandler(config Config, runner *Runner) http.Handler {
	handler := &HTTPHandler{config: config, runner: runner}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", handler.health)
	mux.Handle("GET /metrics", runner.metrics.Handler())
	mux.HandleFunc("POST /api/v1/loadtests/runs", handler.start)
	mux.HandleFunc("GET /api/v1/loadtests/runs/current", handler.current)
	return handler.cors(mux)
}

func (handler *HTTPHandler) health(response http.ResponseWriter, _ *http.Request) {
	response.WriteHeader(http.StatusNoContent)
}

func (handler *HTTPHandler) start(response http.ResponseWriter, request *http.Request) {
	if !handler.authorized(response, request) {
		return
	}
	var payload RunRequest
	decoder := json.NewDecoder(request.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&payload); err != nil {
		writeError(response, http.StatusBadRequest, "invalid_request", "request body must contain runId, profile, and scenario")
		return
	}
	state, err := handler.runner.Start(request.Context(), payload)
	switch {
	case errors.Is(err, ErrAlreadyRunning):
		writeError(response, http.StatusConflict, "already_running", "ALREADY RUNNING")
	case errors.Is(err, ErrInvalidFixture):
		writeError(response, http.StatusUnprocessableEntity, "invalid_fixture", err.Error())
	case errors.Is(err, ErrInvalidRequest):
		writeError(response, http.StatusBadRequest, "invalid_request", err.Error())
	case err != nil:
		writeError(response, http.StatusInternalServerError, "internal_error", "load test could not be started")
	default:
		writeJSON(response, http.StatusAccepted, state)
	}
}

func (handler *HTTPHandler) current(response http.ResponseWriter, request *http.Request) {
	if !handler.authorized(response, request) {
		return
	}
	writeJSON(response, http.StatusOK, handler.runner.Current())
}

func (handler *HTTPHandler) authorized(response http.ResponseWriter, request *http.Request) bool {
	if !handler.config.Enabled {
		writeError(response, http.StatusNotFound, "not_found", "not found")
		return false
	}
	if handler.config.APIKey != "" && request.Header.Get("X-Loadtest-Api-Key") != handler.config.APIKey {
		writeError(response, http.StatusUnauthorized, "unauthorized", "invalid load-test API key")
		return false
	}
	return true
}

func (handler *HTTPHandler) cors(next http.Handler) http.Handler {
	allowed := make(map[string]struct{}, len(handler.config.AllowedOrigins))
	for _, origin := range handler.config.AllowedOrigins {
		allowed[origin] = struct{}{}
	}
	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		origin := request.Header.Get("Origin")
		if _, ok := allowed[origin]; origin != "" && ok {
			response.Header().Set("Access-Control-Allow-Origin", origin)
			response.Header().Set("Vary", "Origin")
			response.Header().Set("Access-Control-Allow-Headers", "Content-Type,X-Grafana-Action,X-Loadtest-Api-Key")
			response.Header().Set("Access-Control-Allow-Methods", "GET,POST,OPTIONS")
		}
		if request.Method == http.MethodOptions {
			if _, ok := allowed[origin]; !ok {
				writeError(response, http.StatusForbidden, "origin_forbidden", "origin is not allowed")
				return
			}
			response.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(response, request)
	})
}

func writeError(response http.ResponseWriter, status int, code, message string) {
	writeJSON(response, status, map[string]any{"error": map[string]string{"code": code, "message": message}})
}

func writeJSON(response http.ResponseWriter, status int, payload any) {
	response.Header().Set("Content-Type", "application/json")
	response.WriteHeader(status)
	_ = json.NewEncoder(response).Encode(payload)
}
