package storagex

import (
	"context"
	"fmt"
	"strings"

	"github.com/spf13/viper"
	"go.uber.org/fx"
	"go.uber.org/zap"
)

// Module is the Fx module that provides storage functionality
var Module = fx.Module("storagex",
	fx.Provide(
		NewConfig,
		NewStorage,
		NewKeyBuilder,
		NewLogger,
	),
	fx.Invoke(registerLifecycle),
)

// ConfigParams defines the parameters needed for config creation
type ConfigParams struct {
	fx.In

	// Viper instance for configuration (optional)
	Viper *viper.Viper `optional:"true"`
}

// StorageParams defines the parameters needed for storage creation
type StorageParams struct {
	fx.In

	Config     *Config
	Logger     *zap.Logger `optional:"true"`
	KeyBuilder KeyBuilder  `optional:"true"`
}

// NewConfig creates a new configuration from Viper or defaults
func NewConfig(params ConfigParams) (*Config, error) {
	var v *viper.Viper
	if params.Viper != nil {
		v = params.Viper
	} else {
		v = viper.New()
		// Set up default configuration sources
		setupViper(v)
	}

	// Start with defaults
	cfg := DefaultConfig()

	// Bind environment variables
	bindEnvVars(v)

	// Read configuration from viper
	if err := v.Unmarshal(cfg); err != nil {
		return nil, fmt.Errorf("failed to unmarshal config: %w", err)
	}

	// Sanitize and validate
	cfg = SanitizeConfig(cfg)
	if err := ValidateConfig(cfg); err != nil {
		return nil, fmt.Errorf("invalid configuration: %w", err)
	}

	return cfg, nil
}

// setupViper configures viper with default settings
func setupViper(v *viper.Viper) {
	// Set config name and paths
	v.SetConfigName("storagex")
	v.SetConfigType("yaml")
	v.AddConfigPath(".")
	v.AddConfigPath("./config")
	v.AddConfigPath("/etc/storagex")
	v.AddConfigPath("$HOME/.config/storagex")

	// Set default values
	defaults := map[string]interface{}{
		"storagex.provider":          "s3",
		"storagex.region":            "us-east-1",
		"storagex.use_path_style":    false,
		"storagex.request_timeout":   "30s",
		"storagex.max_retries":       3,
		"storagex.backoff_initial":   "200ms",
		"storagex.backoff_max":       "5s",
		"storagex.default_part_size": 8 << 20, // 8MB
		"storagex.default_parallel":  4,
		"storagex.disable_ssl":       false,
		"storagex.enable_logging":    false,
	}

	for key, value := range defaults {
		v.SetDefault(key, value)
	}

	// Try to read config file (ignore errors as it's optional)
	_ = v.ReadInConfig()
}

// bindEnvVars binds environment variables to viper keys
func bindEnvVars(v *viper.Viper) {
	// Enable environment variable reading
	v.SetEnvPrefix("STORAGEX")
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()

	// Explicit bindings for complex keys
	envBindings := map[string]string{
		"storagex.provider":          "STORAGEX_PROVIDER",
		"storagex.bucket":            "STORAGEX_BUCKET",
		"storagex.region":            "STORAGEX_REGION",
		"storagex.endpoint":          "STORAGEX_ENDPOINT",
		"storagex.use_path_style":    "STORAGEX_USE_PATH_STYLE",
		"storagex.access_key":        "STORAGEX_ACCESS_KEY",
		"storagex.secret_key":        "STORAGEX_SECRET_KEY",
		"storagex.session_token":     "STORAGEX_SESSION_TOKEN",
		"storagex.request_timeout":   "STORAGEX_REQUEST_TIMEOUT",
		"storagex.max_retries":       "STORAGEX_MAX_RETRIES",
		"storagex.backoff_initial":   "STORAGEX_BACKOFF_INITIAL",
		"storagex.backoff_max":       "STORAGEX_BACKOFF_MAX",
		"storagex.default_part_size": "STORAGEX_DEFAULT_PART_SIZE",
		"storagex.default_parallel":  "STORAGEX_DEFAULT_PARALLEL",
		"storagex.base_prefix":       "STORAGEX_BASE_PREFIX",
		"storagex.disable_ssl":       "STORAGEX_DISABLE_SSL",
		"storagex.enable_logging":    "STORAGEX_ENABLE_LOGGING",
	}

	for key, env := range envBindings {
		_ = v.BindEnv(key, env)
	}
}

// NewStorage creates a new Storage implementation
// This is a factory function that needs to be implemented by storage providers
var NewStorageFunc func(ctx context.Context, cfg *Config, opts ...Option) (Storage, error)

func NewStorage(params StorageParams) (Storage, error) {
	if NewStorageFunc == nil {
		return nil, fmt.Errorf("no storage implementation registered - did you import a provider?")
	}

	// Prepare options
	var opts []Option

	if params.Logger != nil {
		opts = append(opts, WithLogger(params.Logger))
	}

	if params.KeyBuilder != nil {
		opts = append(opts, WithKeyBuilder(params.KeyBuilder))
	}

	// Create storage implementation using registered factory with background context
	ctx := context.Background()
	storage, err := NewStorageFunc(ctx, params.Config, opts...)
	if err != nil {
		return nil, fmt.Errorf("failed to create storage: %w", err)
	}

	return storage, nil
}

// NewKeyBuilder creates a default key builder from configuration
func NewKeyBuilder(cfg *Config) KeyBuilder {
	if cfg.BasePrefix == "" {
		return &NoOpKeyBuilder{}
	}

	return NewPrefixKeyBuilder(cfg.BasePrefix)
}

// NewLogger creates a logger based on configuration
func NewLogger(cfg *Config) (*zap.Logger, error) {
	if !cfg.EnableLogging {
		return zap.NewNop(), nil
	}

	// Create development or production logger based on environment
	config := zap.NewProductionConfig()

	// Configure based on storage config
	if cfg.IsMinIO() || cfg.Endpoint != "" {
		// Development settings for local/MinIO
		config = zap.NewDevelopmentConfig()
		config.Level = zap.NewAtomicLevelAt(zap.DebugLevel)
	}

	logger, err := config.Build()
	if err != nil {
		return nil, fmt.Errorf("failed to create logger: %w", err)
	}

	return logger, nil
}

// LifecycleParams defines parameters for lifecycle management
type LifecycleParams struct {
	fx.In

	Lifecycle fx.Lifecycle
	Storage   Storage
	Logger    *zap.Logger `optional:"true"`
}

// registerLifecycle registers shutdown hooks for graceful cleanup
func registerLifecycle(params LifecycleParams) {
	params.Lifecycle.Append(fx.Hook{
		OnStart: func(ctx context.Context) error {
			if params.Logger != nil {
				params.Logger.Info("StorageX module started")
			}
			return nil
		},
		OnStop: func(ctx context.Context) error {
			if params.Logger != nil {
				params.Logger.Info("StorageX module stopping")
			}

			// If storage implements io.Closer, close it
			if closer, ok := params.Storage.(interface{ Close() error }); ok {
				if err := closer.Close(); err != nil {
					if params.Logger != nil {
						params.Logger.Error("Error closing storage", zap.Error(err))
					}
					return err
				}
			}

			if params.Logger != nil {
				params.Logger.Info("StorageX module stopped")
			}
			return nil
		},
	})
}

// TestModule provides a module for testing with mock/test implementations
var TestModule = fx.Module("storagex-test",
	fx.Provide(
		NewTestConfig,
		NewStorage,
		NewTestKeyBuilder,
		NewTestLogger,
	),
)

// NewTestConfig creates a test configuration
func NewTestConfig() *Config {
	cfg := DefaultConfig()
	cfg.Bucket = "test-bucket"
	cfg.Endpoint = "http://localhost:9000"
	cfg.UsePathStyle = true
	cfg.AccessKey = "minioadmin"
	cfg.SecretKey = "minioadmin"
	cfg.DisableSSL = true
	cfg.EnableLogging = true
	return cfg
}

// NewTestKeyBuilder creates a test key builder
func NewTestKeyBuilder() KeyBuilder {
	return NewPrefixKeyBuilder("test")
}

// NewTestLogger creates a test logger
func NewTestLogger() *zap.Logger {
	config := zap.NewDevelopmentConfig()
	config.Level = zap.NewAtomicLevelAt(zap.DebugLevel)
	logger, _ := config.Build()
	return logger
}

// ConfigFromViper creates configuration from an existing Viper instance
func ConfigFromViper(v *viper.Viper) fx.Option {
	return fx.Supply(ConfigParams{Viper: v})
}

// WithCustomKeyBuilder provides a custom key builder to the DI container
func WithCustomKeyBuilder(kb KeyBuilder) fx.Option {
	return fx.Supply(kb)
}

// WithCustomLogger provides a custom logger to the DI container
func WithCustomLogger(logger *zap.Logger) fx.Option {
	return fx.Supply(logger)
}

// ModuleOptions allows customization of the storage module
type ModuleOptions struct {
	// DisableLifecycle disables automatic lifecycle management
	DisableLifecycle bool

	// CustomProviders allows adding custom providers to the module
	CustomProviders []fx.Option
}

// NewModuleWithOptions creates a customized storage module
func NewModuleWithOptions(opts ModuleOptions) fx.Option {
	providers := []fx.Option{
		fx.Provide(
			NewConfig,
			NewStorage,
			NewKeyBuilder,
			NewLogger,
		),
	}

	// Add custom providers
	providers = append(providers, opts.CustomProviders...)

	// Add lifecycle management unless disabled
	if !opts.DisableLifecycle {
		providers = append(providers, fx.Invoke(registerLifecycle))
	}

	return fx.Module("storagex", providers...)
}

// Example usage documentation

/*
Basic usage with fx:

	package main

	import (
		"context"
		"log"

		"go.uber.org/fx"
		"github.com/gostratum/storagex/pkg/storagex"
	)

	func main() {
		app := fx.New(
			storagex.Module,
			fx.Invoke(useStorage),
		)

		if err := app.Start(context.Background()); err != nil {
			log.Fatal(err)
		}

		defer app.Stop(context.Background())
	}

	func useStorage(storage storagex.Storage) {
		// Use the storage...
	}

With custom configuration:

	import "github.com/spf13/viper"

	func main() {
		v := viper.New()
		v.Set("storagex.bucket", "my-bucket")
		v.Set("storagex.endpoint", "http://localhost:9000")

		app := fx.New(
			storagex.Module,
			storagex.ConfigFromViper(v),
			fx.Invoke(useStorage),
		)

		// ...
	}

For testing:

	func TestMyApp(t *testing.T) {
		app := fx.New(
			storagex.TestModule,
			fx.Invoke(func(storage storagex.Storage) {
				// Test with MinIO
			}),
		)

		// ...
	}
*/

// NewStorageFromConfig creates a storage instance directly from configuration
// This is useful for testing and simple cases that don't need full DI
func NewStorageFromConfig(ctx context.Context, cfg *Config, opts ...Option) (Storage, error) {
	if NewStorageFunc == nil {
		return nil, fmt.Errorf("no storage implementation registered - did you import a provider?")
	}

	return NewStorageFunc(ctx, cfg, opts...)
}
