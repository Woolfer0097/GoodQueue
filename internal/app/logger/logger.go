package logger

import (
	"fmt"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

func New(level string) (*zap.Logger, error) {
	parsedLevel, err := zapcore.ParseLevel(level)
	if err != nil {
		return nil, fmt.Errorf("parse log level %q: %w", level, err)
	}
	configuration := zap.NewProductionConfig()
	configuration.Level = zap.NewAtomicLevelAt(parsedLevel)
	log, err := configuration.Build()
	if err != nil {
		return nil, fmt.Errorf("build logger: %w", err)
	}
	return log, nil
}
