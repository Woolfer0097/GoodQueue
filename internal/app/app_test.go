package app

import (
	"context"
	"errors"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/Woolfer0097/GoodQueue/internal/app/config"
	"go.uber.org/zap"
)

type lifecycleWorkers struct {
	done       chan struct{}
	ignoreStop bool
	release    chan struct{}
	onStop     func()
}

func (workers *lifecycleWorkers) Run(ctx context.Context) {
	defer close(workers.done)
	if workers.ignoreStop {
		<-workers.release
		return
	}
	<-ctx.Done()
	workers.onStop()
}

func TestApplicationCancelsWorkersBeforeDatabaseCloseOnSignal(t *testing.T) {
	var mu sync.Mutex
	workerStopped := false
	workers := &lifecycleWorkers{done: make(chan struct{}), onStop: func() {
		mu.Lock()
		workerStopped = true
		mu.Unlock()
	}}
	serverStopped := make(chan struct{})
	application := lifecycleApplication(workers)
	application.listenAndServe = func() error {
		<-serverStopped
		return http.ErrServerClosed
	}
	application.shutdownServer = func(context.Context) error {
		close(serverStopped)
		return nil
	}
	application.closeDatabase = func() error {
		mu.Lock()
		defer mu.Unlock()
		if !workerStopped {
			t.Error("database closed before workers joined")
		}
		return nil
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := application.Run(ctx); err != nil {
		t.Fatalf("run application: %v", err)
	}
	if err := application.Run(context.Background()); err == nil {
		t.Fatal("second Run unexpectedly succeeded")
	}
}

func TestApplicationStopsWorkersAfterHTTPFailure(t *testing.T) {
	workers := &lifecycleWorkers{done: make(chan struct{}), onStop: func() {}}
	application := lifecycleApplication(workers)
	serveFailure := errors.New("listen failed")
	application.listenAndServe = func() error { return serveFailure }
	application.shutdownServer = func(context.Context) error { return nil }
	closed := false
	application.closeDatabase = func() error {
		select {
		case <-workers.done:
			closed = true
		default:
			t.Error("database closed before workers stopped")
		}
		return nil
	}

	err := application.Run(context.Background())
	if !errors.Is(err, serveFailure) || !closed {
		t.Fatalf("HTTP failure shutdown: closed=%t err=%v", closed, err)
	}
}

func TestApplicationWorkerJoinRespectsShutdownTimeout(t *testing.T) {
	workers := &lifecycleWorkers{done: make(chan struct{}), ignoreStop: true, release: make(chan struct{}), onStop: func() {}}
	t.Cleanup(func() { close(workers.release) })
	application := lifecycleApplication(workers)
	application.config.ShutdownTimeout = 10 * time.Millisecond
	application.listenAndServe = func() error { return errors.New("listen failed") }
	application.shutdownServer = func(context.Context) error { return nil }
	databaseClosed := false
	application.closeDatabase = func() error { databaseClosed = true; return nil }

	started := time.Now()
	err := application.Run(context.Background())
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected worker join timeout, got %v", err)
	}
	if databaseClosed || time.Since(started) > time.Second {
		t.Fatalf("bounded shutdown failed: closed=%t duration=%s", databaseClosed, time.Since(started))
	}
}

func lifecycleApplication(workers workerRunner) *Application {
	return &Application{
		config:  config.Config{ShutdownTimeout: time.Second},
		log:     zap.NewNop(),
		server:  &http.Server{ReadHeaderTimeout: time.Second},
		workers: workers,
	}
}
