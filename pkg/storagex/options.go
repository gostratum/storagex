package storagex

import (
	"fmt"
	"strings"
	"time"
)

// Config holds all storage configuration options
type Config struct {
	// Provider specifies the storage backend ("s3")
	Provider string `mapstructure:"provider" yaml:"provider"`

	// Bucket is the storage bucket name
	Bucket string `mapstructure:"bucket" yaml:"bucket"`

	// Region is the AWS region (e.g., "us-west-2")
	Region string `mapstructure:"region" yaml:"region"`

	// Endpoint is the custom endpoint URL (for MinIO, etc.)
	Endpoint string `mapstructure:"endpoint" yaml:"endpoint"`

	// UsePathStyle forces path-style addressing (true for MinIO)
	UsePathStyle bool `mapstructure:"use_path_style" yaml:"use_path_style"`

	// AccessKey is the access key ID
	AccessKey string `mapstructure:"access_key" yaml:"access_key"`

	// SecretKey is the secret access key
	SecretKey string `mapstructure:"secret_key" yaml:"secret_key"`

	// SessionToken is the temporary session token (optional)
	SessionToken string `mapstructure:"session_token" yaml:"session_token"`

	// RequestTimeout is the timeout for individual requests
	RequestTimeout time.Duration `mapstructure:"request_timeout" yaml:"request_timeout"`

	// MaxRetries is the maximum number of retry attempts
	MaxRetries int `mapstructure:"max_retries" yaml:"max_retries"`

	// BackoffInitial is the initial backoff delay
	BackoffInitial time.Duration `mapstructure:"backoff_initial" yaml:"backoff_initial"`

	// BackoffMax is the maximum backoff delay
	BackoffMax time.Duration `mapstructure:"backoff_max" yaml:"backoff_max"`

	// DefaultPartSize is the default multipart upload part size
	DefaultPartSize int64 `mapstructure:"default_part_size" yaml:"default_part_size"`

	// DefaultParallel is the default multipart upload concurrency
	DefaultParallel int `mapstructure:"default_parallel" yaml:"default_parallel"`

	// BasePrefix is the base prefix for multi-tenant keys (e.g., "org/%s/ws/%s")
	BasePrefix string `mapstructure:"base_prefix" yaml:"base_prefix"`

	// DisableSSL disables SSL for connections (development only)
	DisableSSL bool `mapstructure:"disable_ssl" yaml:"disable_ssl"`

	// EnableLogging enables detailed operation logging
	EnableLogging bool `mapstructure:"enable_logging" yaml:"enable_logging"`
}

// DefaultConfig returns a configuration with sensible defaults
func DefaultConfig() *Config {
	return &Config{
		Provider:        "s3",
		Region:          "us-east-1",
		UsePathStyle:    false,
		RequestTimeout:  30 * time.Second,
		MaxRetries:      3,
		BackoffInitial:  200 * time.Millisecond,
		BackoffMax:      5 * time.Second,
		DefaultPartSize: 8 << 20, // 8MB
		DefaultParallel: 4,
		DisableSSL:      false,
		EnableLogging:   false,
	}
}

// Options holds functional options for customizing storage behavior
type Options struct {
	logger     Logger
	keyBuilder KeyBuilder
	clock      func() time.Time
}

// Option is a functional option for configuring Storage
type Option func(*Options)

// WithLogger sets a custom logger
// WithLogger sets a custom logger implementing the storagex Logger adapter.
func WithLogger(logger Logger) Option {
	return func(opts *Options) {
		opts.logger = logger
	}
}

// WithCoreLogger is a convenience wrapper to accept a core logger and wrap it
// into the storagex Logger adapter. This preserves backward compatibility for
// callers using github.com/gostratum/core/logger.
func WithCoreLogger(l interface{}) Option {
	return func(opts *Options) {
		// If the provided logger matches the expected coreLogger interface,
		// wrap it. Otherwise, ignore and leave the default logger.
		if cl, ok := l.(interface {
			Debug(string, ...interface{})
			Info(string, ...interface{})
			Warn(string, ...interface{})
			Error(string, ...interface{})
		}); ok {
			opts.logger = WrapCoreLogger(cl)
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
		opts.logger = NewNopLogger()
	}
	if opts.keyBuilder == nil {
		opts.keyBuilder = &PrefixKeyBuilder{}
	}
	if opts.clock == nil {
		opts.clock = time.Now
	}
}

// GetLogger returns the configured logger
func (opts *Options) GetLogger() Logger {
	if opts.logger == nil {
		return NewNopLogger()
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
