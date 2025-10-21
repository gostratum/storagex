package storagex

import (
	"time"
)

// Config holds all storage configuration options
type Config struct {
	// Provider specifies the storage backend ("s3")
	Provider string `mapstructure:"provider" yaml:"provider" default:"s3"`

	// Bucket is the storage bucket name
	Bucket string `mapstructure:"bucket" yaml:"bucket"`

	// Region is the AWS region (e.g., "us-west-2")
	Region string `mapstructure:"region" yaml:"region" default:"us-east-1"`

	// Endpoint is the custom endpoint URL (for MinIO, etc.)
	Endpoint string `mapstructure:"endpoint" yaml:"endpoint"`

	// UsePathStyle forces path-style addressing (true for MinIO)
	UsePathStyle bool `mapstructure:"use_path_style" yaml:"use_path_style" default:"false"`

	// AccessKey is the access key ID
	AccessKey string `mapstructure:"access_key" yaml:"access_key"`

	// SecretKey is the secret access key
	SecretKey string `mapstructure:"secret_key" yaml:"secret_key"`

	// SessionToken is the temporary session token (optional)
	SessionToken string `mapstructure:"session_token" yaml:"session_token"`

	// UseSDKDefaults when true will let the AWS SDK default credential chain (env, shared config, instance profile)
	// be used when explicit credentials are not provided. Default: false
	UseSDKDefaults bool `mapstructure:"use_sdk_defaults" yaml:"use_sdk_defaults" default:"false"`

	// RoleARN optionally specifies an ARN to assume via STS. When set, the module will use the
	// SDK default provider (or explicit creds if present) as the source and assume this role.
	RoleARN string `mapstructure:"role_arn" yaml:"role_arn"`

	// ExternalID is passed to STS AssumeRole when RoleARN is used.
	ExternalID string `mapstructure:"external_id" yaml:"external_id"`

	// Profile selects a shared credentials/profile name when loading SDK defaults.
	Profile string `mapstructure:"profile" yaml:"profile"`

	// RequestTimeout is the timeout for individual requests
	RequestTimeout time.Duration `mapstructure:"request_timeout" yaml:"request_timeout" default:"30s"`

	// MaxRetries is the maximum number of retry attempts
	MaxRetries int `mapstructure:"max_retries" yaml:"max_retries" default:"3"`

	// BackoffInitial is the initial backoff delay
	BackoffInitial time.Duration `mapstructure:"backoff_initial" yaml:"backoff_initial" default:"200ms"`

	// BackoffMax is the maximum backoff delay
	BackoffMax time.Duration `mapstructure:"backoff_max" yaml:"backoff_max" default:"5s"`

	// DefaultPartSize is the default multipart upload part size
	DefaultPartSize int64 `mapstructure:"default_part_size" yaml:"default_part_size" default:"8388608"` // 8MB

	// DefaultParallel is the default multipart upload concurrency
	DefaultParallel int `mapstructure:"default_parallel" yaml:"default_parallel" default:"4"`

	// BasePrefix is the base prefix for multi-tenant keys (e.g., "org/%s/ws/%s")
	BasePrefix string `mapstructure:"base_prefix" yaml:"base_prefix"`

	// DisableSSL disables SSL for connections (development only)
	DisableSSL bool `mapstructure:"disable_ssl" yaml:"disable_ssl" default:"false"`

	// EnableLogging enables detailed operation logging
	EnableLogging bool `mapstructure:"enable_logging" yaml:"enable_logging" default:"false"`
}

// Prefix implements configx.Configurable and returns the configuration prefix
func (Config) Prefix() string { return "storage" }

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

// NewConfigFromLoader creates a Config using the standard configx.Loader pattern.
// This is useful for standalone usage without FX dependency injection.
// For FX-based applications, use the Module which provides NewConfig automatically.
func NewConfigFromLoader(loader interface {
	Unmarshal(any) error
}) (*Config, error) {
	cfg := DefaultConfig()
	if err := loader.Unmarshal(cfg); err != nil {
		return nil, err
	}

	// Sanitize and validate
	cfg = cfg.Sanitize()
	if err := ValidateConfig(cfg); err != nil {
		return nil, err
	}

	return cfg, nil
}
