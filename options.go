package storagex

import (
	"fmt"
	"strings"
	"time"

	"github.com/gostratum/core/logx"
)

// Options holds functional options for customizing storage behavior
type Options struct {
	logger     logx.Logger
	keyBuilder KeyBuilder
	clock      func() time.Time
}

// Option is a functional option for configuring Storage
type Option func(*Options)

// WithLogger sets a custom core logx.Logger
func WithLogger(logger logx.Logger) Option {
	return func(opts *Options) {
		opts.logger = logger
	}
}

// WithLogxLogger allows passing a core/logx.Logger directly. This is a
// lightweight migration helper for callers still constructing Storage
// imperatively; prefer providing `logx.Logger` via FX DI and using
// WithLogger.
func WithLogxLogger(l any) Option {
	return func(opts *Options) {
		if lx, ok := l.(logx.Logger); ok {
			opts.logger = lx
		}
	}
}

// WithCoreLogger is a convenience wrapper to accept a core logger. The
// storagex module now uses `core/logx.Logger` directly; prefer passing a
// `logx.Logger` explicitly via WithLogger or via FX DI. This helper will
// accept a `logx.Logger` value and use it if provided.
func WithCoreLogger(l any) Option {
	return func(opts *Options) {
		// If the provided logger matches the expected coreLogger interface,
		// wrap it. Otherwise, ignore and leave the default logger.
		if lx, ok := l.(logx.Logger); ok {
			opts.logger = lx
		}
	}
}

// WithKeyBuilder sets a custom key building strategy
func WithKeyBuilder(kb KeyBuilder) Option {
	return func(opts *Options) {
		opts.keyBuilder = kb
	}
}

// WithClock sets a custom time provider (useful for testing)
func WithClock(clock func() time.Time) Option {
	return func(opts *Options) {
		opts.clock = clock
	}
}

// applyDefaults applies default values to unset options
func (opts *Options) applyDefaults() {
	if opts.logger == nil {
		opts.logger = logx.NewNoopLogger()
	}
	if opts.keyBuilder == nil {
		opts.keyBuilder = &PrefixKeyBuilder{}
	}
	if opts.clock == nil {
		opts.clock = time.Now
	}
}

// GetLogger returns the configured logger
func (opts *Options) GetLogger() logx.Logger {
	if opts.logger == nil {
		return logx.NewNoopLogger()
	}
	return opts.logger
}

// GetKeyBuilder returns the configured key builder
func (opts *Options) GetKeyBuilder() KeyBuilder {
	if opts.keyBuilder == nil {
		return &PrefixKeyBuilder{}
	}
	return opts.keyBuilder
}

// GetClock returns the configured clock function
func (opts *Options) GetClock() func() time.Time {
	if opts.clock == nil {
		return time.Now
	}
	return opts.clock
}

// GetEffectiveConfig returns the configuration with options applied
func GetEffectiveConfig(cfg *Config, options ...Option) (*Config, *Options) {
	opts := &Options{}
	for _, opt := range options {
		opt(opts)
	}
	opts.applyDefaults()

	// Create a copy of the config to avoid mutations
	effective := *cfg
	return &effective, opts
}

// GetMultipartConfig creates a MultipartConfig from base configuration
func (c *Config) GetMultipartConfig() *MultipartConfig {
	return &MultipartConfig{
		PartSizeBytes: c.DefaultPartSize,
		Concurrency:   c.DefaultParallel,
	}
}

// IsMinIO returns true if the configuration appears to be for MinIO
func (c *Config) IsMinIO() bool {
	if c.Endpoint == "" {
		return false
	}

	endpoint := strings.ToLower(c.Endpoint)
	return strings.Contains(endpoint, "minio") ||
		strings.Contains(endpoint, "localhost") ||
		strings.Contains(endpoint, "127.0.0.1") ||
		c.UsePathStyle // Path style often indicates MinIO
}

// GetEndpointURL returns the full endpoint URL
func (c *Config) GetEndpointURL() string {
	if c.Endpoint == "" {
		return ""
	}

	if strings.HasPrefix(c.Endpoint, "http://") || strings.HasPrefix(c.Endpoint, "https://") {
		return c.Endpoint
	}

	scheme := "https"
	if c.DisableSSL {
		scheme = "http"
	}

	return fmt.Sprintf("%s://%s", scheme, c.Endpoint)
}

// String returns a safe string representation (redacts secrets)
func (c *Config) String() string {
	return fmt.Sprintf("Config{Provider:%s, Bucket:%s, Region:%s, Endpoint:%s, UsePathStyle:%v}",
		c.Provider, c.Bucket, c.Region, c.Endpoint, c.UsePathStyle)
}
