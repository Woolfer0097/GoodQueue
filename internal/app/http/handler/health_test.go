package handler

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

type pingerFunc func(context.Context) error

func (function pingerFunc) PingContext(ctx context.Context) error { return function(ctx) }

func TestHealthDoesNotPingDatabase(t *testing.T) {
	gin.SetMode(gin.TestMode)
	pinged := false
	handler := NewHealthHandler(pingerFunc(func(context.Context) error { pinged = true; return nil }), time.Second)
	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	handler.Health(context)
	if recorder.Code != http.StatusOK || recorder.Body.String() != `{"status":"ok"}` {
		t.Fatalf("unexpected health response: %d %s", recorder.Code, recorder.Body.String())
	}
	if pinged {
		t.Fatal("liveness probe queried the database")
	}
}

func TestReadyReportsDatabaseStateWithoutLeakingError(t *testing.T) {
	gin.SetMode(gin.TestMode)
	tests := []struct {
		name string
		err  error
		code int
		body string
	}{
		{name: "ready", code: http.StatusOK, body: `{"status":"ok"}`},
		{name: "unavailable", err: errors.New("postgres://secret@host SQL details"), code: http.StatusServiceUnavailable, body: `{"status":"unavailable"}`},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			handler := NewHealthHandler(pingerFunc(func(context.Context) error { return test.err }), time.Second)
			recorder := httptest.NewRecorder()
			ginContext, _ := gin.CreateTestContext(recorder)
			ginContext.Request = httptest.NewRequest(http.MethodGet, "/readyz", nil)
			handler.Ready(ginContext)
			if recorder.Code != test.code || recorder.Body.String() != test.body {
				t.Fatalf("unexpected readiness response: %d %s", recorder.Code, recorder.Body.String())
			}
		})
	}
}

func TestReadyBoundsPingWithTimeout(t *testing.T) {
	handler := NewHealthHandler(pingerFunc(func(ctx context.Context) error {
		<-ctx.Done()
		return ctx.Err()
	}), 5*time.Millisecond)
	recorder := httptest.NewRecorder()
	ginContext, _ := gin.CreateTestContext(recorder)
	ginContext.Request = httptest.NewRequest(http.MethodGet, "/readyz", nil)
	startedAt := time.Now()
	handler.Ready(ginContext)
	if recorder.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected unavailable, got %d", recorder.Code)
	}
	if time.Since(startedAt) > time.Second {
		t.Fatal("readiness ping was not bounded")
	}
}
