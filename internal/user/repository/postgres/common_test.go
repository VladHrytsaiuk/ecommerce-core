//go:build integration && legacy

package postgres

import (
	"github.com/VladHrytsaiuk/ecommerce-core/internal/platform/logger"
	"go.uber.org/zap"
)

// noopLogger імплементує logger.Logger інтерфейс для тестів.
type noopLogger struct{}

func (l *noopLogger) Debug(msg string, fields ...zap.Field) {}
func (l *noopLogger) Info(msg string, fields ...zap.Field)  {}
func (l *noopLogger) Warn(msg string, fields ...zap.Field)  {}
func (l *noopLogger) Error(msg string, fields ...zap.Field) {}
func (l *noopLogger) Fatal(msg string, fields ...zap.Field) {}

func (l *noopLogger) Debugf(template string, args ...interface{}) {}
func (l *noopLogger) Infof(template string, args ...interface{})  {}
func (l *noopLogger) Warnf(template string, args ...interface{})  {}
func (l *noopLogger) Errorf(template string, args ...interface{}) {}
func (l *noopLogger) Fatalf(template string, args ...interface{}) {}

func (l *noopLogger) Debugw(msg string, kvs ...interface{}) {}
func (l *noopLogger) Infow(msg string, kvs ...interface{})  {}
func (l *noopLogger) Warnw(msg string, kvs ...interface{})  {}
func (l *noopLogger) Errorw(msg string, kvs ...interface{}) {}
func (l *noopLogger) Fatalw(msg string, kvs ...interface{}) {}

func (l *noopLogger) Sync() error                            { return nil }
func (l *noopLogger) With(fields ...zap.Field) logger.Logger { return l }
