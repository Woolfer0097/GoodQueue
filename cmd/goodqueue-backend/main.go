// Package main starts the GoodQueue HTTP API.
//
//	@title			GoodQueue API
//	@version		0.1.0
//	@description	REST API for a fair two-stage limited-product purchase queue.
//	@BasePath		/
//	@schemes		http
//	@produce		json
//	@accept			json
package main

import (
	"context"
	"errors"
	"os/signal"
	"syscall"

	"github.com/Woolfer0097/GoodQueue/internal/app"
	"github.com/Woolfer0097/GoodQueue/internal/app/config"
	"github.com/Woolfer0097/GoodQueue/internal/app/logger"
	"go.uber.org/zap"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		panic(err)
	}

	log, err := logger.New(cfg.LogLevel)
	if err != nil {
		panic(err)
	}
	defer func() { _ = log.Sync() }()

	application, err := app.New(cfg, log)
	if err != nil {
		log.Fatal("initialize application", zap.Error(err))
	}

	shutdownSignal, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	if err := application.Run(shutdownSignal); err != nil && !errors.Is(err, context.Canceled) {
		log.Fatal("application stopped", zap.Error(err))
	}
}
