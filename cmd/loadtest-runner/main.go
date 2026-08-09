package main

import (
	"context"
	"errors"
	"net/http"
	"os/signal"
	"syscall"
	"time"

	"github.com/Woolfer0097/GoodQueue/internal/app/logger"
	"github.com/Woolfer0097/GoodQueue/internal/loadtestrunner"
	"go.uber.org/zap"
)

func main() {
	config, err := loadtestrunner.LoadConfig()
	if err != nil {
		panic(err)
	}
	log, err := logger.New("info")
	if err != nil {
		panic(err)
	}
	defer func() { _ = log.Sync() }()
	metrics := loadtestrunner.NewMetrics()
	runner := loadtestrunner.New(config, log, nil, metrics)
	server := &http.Server{Addr: config.Address, Handler: loadtestrunner.NewHTTPHandler(config, runner), ReadHeaderTimeout: 5 * time.Second}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	go func() {
		<-ctx.Done()
		shutdown, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = server.Shutdown(shutdown)
	}()
	log.Info("load-test runner listening", zap.String("address", config.Address), zap.Bool("enabled", config.Enabled))
	if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatal("load-test runner stopped", zap.Error(err))
	}
}
