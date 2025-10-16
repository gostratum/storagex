package storagex

// Logger is an adapter interface that storagex uses for logging.
// It accepts simple key/value variadic pairs to keep call sites concise and
// to decouple from any particular structured-logging Field type.
type Logger interface {
	Debug(msg string, args ...interface{})
	Info(msg string, args ...interface{})
	Warn(msg string, args ...interface{})
	Error(msg string, args ...interface{})
}

// coreLogger is the minimal interface we expect from github.com/gostratum/core/logger
// implementations. This allows us to wrap a core logger without importing concrete
// types from that package directly in call sites.
type coreLogger interface {
	Debug(msg string, args ...interface{})
	Info(msg string, args ...interface{})
	Warn(msg string, args ...interface{})
	Error(msg string, args ...interface{})
}

// WrapCoreLogger wraps a core logger implementation into the storagex Logger
// interface. Pass any concrete logger from github.com/gostratum/core/logger
// that matches the expected methods.
func WrapCoreLogger(l coreLogger) Logger {
	if l == nil {
		return &nopLogger{}
	}
	return &coreLoggerAdapter{l}
}

// NewNopLogger returns a no-op logger implementing Logger.
func NewNopLogger() Logger { return &nopLogger{} }

type coreLoggerAdapter struct{ l coreLogger }

func (z *coreLoggerAdapter) Debug(msg string, args ...interface{}) {
	if z.l != nil {
		z.l.Debug(msg, args...)
	}
}
func (z *coreLoggerAdapter) Info(msg string, args ...interface{}) {
	if z.l != nil {
		z.l.Info(msg, args...)
	}
}
func (z *coreLoggerAdapter) Warn(msg string, args ...interface{}) {
	if z.l != nil {
		z.l.Warn(msg, args...)
	}
}
func (z *coreLoggerAdapter) Error(msg string, args ...interface{}) {
	if z.l != nil {
		z.l.Error(msg, args...)
	}
}

type nopLogger struct{}

func (n *nopLogger) Debug(_ string, _ ...interface{}) {}
func (n *nopLogger) Info(_ string, _ ...interface{})  {}
func (n *nopLogger) Warn(_ string, _ ...interface{})  {}
func (n *nopLogger) Error(_ string, _ ...interface{}) {}
