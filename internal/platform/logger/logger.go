package logger

import (
	"os"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// Logger визначає основний контракт для логування в проекті (згідно з docs/infra/logger.md)
type Logger interface {
	Debug(msg string, fields ...zap.Field)
	Info(msg string, fields ...zap.Field)
	Warn(msg string, fields ...zap.Field)
	Error(msg string, fields ...zap.Field)
	Fatal(msg string, fields ...zap.Field)

	// Цукрові методи (Printf-style)
	Debugf(template string, args ...interface{})
	Infof(template string, args ...interface{})
	Warnf(template string, args ...interface{})
	Errorf(template string, args ...interface{})
	Fatalf(template string, args ...interface{})

	// Цукрові методи (Structured-style)
	Debugw(msg string, keysAndValues ...interface{})
	Infow(msg string, keysAndValues ...interface{})
	Warnw(msg string, keysAndValues ...interface{})
	Errorw(msg string, keysAndValues ...interface{})
	Fatalw(msg string, keysAndValues ...interface{})

	With(fields ...zap.Field) Logger
	Sync() error
}

// Log — це глобальний логер, який можна використовувати без DI (для main, ініціалізацій тощо).
var Log Logger

// Init ініціалізує глобальний логер
func Init() {
	config := zap.NewProductionEncoderConfig()
	config.EncodeTime = zapcore.ISO8601TimeEncoder
	config.EncodeLevel = zapcore.CapitalColorLevelEncoder

	core := zapcore.NewCore(
		zapcore.NewConsoleEncoder(config),
		zapcore.AddSync(os.Stdout),
		zap.DebugLevel,
	)

	l := zap.New(core, zap.AddCaller())
	Log = &zapLogger{l.Sugar()}
}

// zapLogger — обгортка над SugaredLogger для підтримки як структурованого, так і "цукрового" логування.
type zapLogger struct {
	s *zap.SugaredLogger
}

func (l *zapLogger) Debug(msg string, fields ...zap.Field) { l.s.Desugar().Debug(msg, fields...) }
func (l *zapLogger) Info(msg string, fields ...zap.Field)  { l.s.Desugar().Info(msg, fields...) }
func (l *zapLogger) Warn(msg string, fields ...zap.Field)  { l.s.Desugar().Warn(msg, fields...) }
func (l *zapLogger) Error(msg string, fields ...zap.Field) { l.s.Desugar().Error(msg, fields...) }
func (l *zapLogger) Fatal(msg string, fields ...zap.Field) { l.s.Desugar().Fatal(msg, fields...) }

func (l *zapLogger) Debugf(template string, args ...interface{}) { l.s.Debugf(template, args...) }
func (l *zapLogger) Infof(template string, args ...interface{})  { l.s.Infof(template, args...) }
func (l *zapLogger) Warnf(template string, args ...interface{})  { l.s.Warnf(template, args...) }
func (l *zapLogger) Errorf(template string, args ...interface{}) { l.s.Errorf(template, args...) }
func (l *zapLogger) Fatalf(template string, args ...interface{}) { l.s.Fatalf(template, args...) }

func (l *zapLogger) Debugw(msg string, kvs ...interface{}) { l.s.Debugw(msg, kvs...) }
func (l *zapLogger) Infow(msg string, kvs ...interface{})  { l.s.Infow(msg, kvs...) }
func (l *zapLogger) Warnw(msg string, kvs ...interface{})  { l.s.Warnw(msg, kvs...) }
func (l *zapLogger) Errorw(msg string, kvs ...interface{}) { l.s.Errorw(msg, kvs...) }
func (l *zapLogger) Fatalw(msg string, kvs ...interface{}) { l.s.Fatalw(msg, kvs...) }

func (l *zapLogger) Sync() error { return l.s.Sync() }

func (l *zapLogger) With(fields ...zap.Field) Logger {
	newLogger := l.s.Desugar().With(fields...).Sugar()
	return &zapLogger{newLogger}
}
