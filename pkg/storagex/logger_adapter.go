package storagex

import "go.uber.org/zap"

// Logger is an adapter interface that storagex uses for logging.
// It intentionally mirrors the subset of zap.Logger used across the project
// so we can adapt other logging implementations later (for example,
// github.com/gostratum/core's logger) without touching the call sites.
type Logger interface {
	Debug(msg string, fields ...zap.Field)
	Info(msg string, fields ...zap.Field)
	Warn(msg string, fields ...zap.Field)
	Error(msg string, fields ...zap.Field)
}

// WrapZapLogger wraps a *zap.Logger into the storagex Logger interface.
func WrapZapLogger(l *zap.Logger) Logger {
	if l == nil {
		return &nopLogger{}
	}
	return &zapLoggerAdapter{l}
}

// NewNopLogger returns a no-op logger implementing Logger.
func NewNopLogger() Logger { return &nopLogger{} }

type zapLoggerAdapter struct{ l *zap.Logger }

func (z *zapLoggerAdapter) Debug(msg string, fields ...zap.Field) {
	if z.l != nil {
		z.l.Debug(msg, fields...)
	}
}
func (z *zapLoggerAdapter) Info(msg string, fields ...zap.Field) {
	if z.l != nil {
		z.l.Info(msg, fields...)
	}
}
func (z *zapLoggerAdapter) Warn(msg string, fields ...zap.Field) {
	if z.l != nil {
		z.l.Warn(msg, fields...)
	}
}
func (z *zapLoggerAdapter) Error(msg string, fields ...zap.Field) {
	if z.l != nil {
		z.l.Error(msg, fields...)
	}
}

type nopLogger struct{}

func (n *nopLogger) Debug(_ string, _ ...zap.Field) {}
func (n *nopLogger) Info(_ string, _ ...zap.Field)  {}
func (n *nopLogger) Warn(_ string, _ ...zap.Field)  {}
func (n *nopLogger) Error(_ string, _ ...zap.Field) {}
