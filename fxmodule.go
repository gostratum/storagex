package storagex

import (
	"context"
	"fmt"

	"github.com/gostratum/core/configx"
	"go.uber.org/fx"
)

// Module is the Fx module that provides storage functionality
var Module = fx.Module("storage",
	fx.Provide(
		NewConfig,
		NewStorage,
		NewKeyBuilder,
		NewLogger,
	),
	fx.Invoke(registerLifecycle),
)

// StorageParams defines the parameters needed for storage creation
type StorageParams struct {
	fx.In

	Config     *Config
	Logger     Logger     `optional:"true"`
	KeyBuilder KeyBuilder `optional:"true"`
}

// NewConfig creates a new configuration from the configx loader
func NewConfig(loader configx.Loader) (*Config, error) {
	cfg := DefaultConfig()
	if err := loader.Bind(cfg); err != nil {
		return nil, fmt.Errorf("failed to load config: %w", err)
	}

	// Sanitize and validate
	cfg = SanitizeConfig(cfg)
	if err := ValidateConfig(cfg); err != nil {
		return nil, fmt.Errorf("invalid configuration: %w", err)
	}

	return cfg, nil
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

// NewLogger creates a logger based on configuration.
// The concrete logger implementation is provided by the application; this
// function returns a storagex.Logger adapter. Currently we return a no-op
// logger until a concrete gostratum/core logger is wired here.
func NewLogger(cfg *Config) (Logger, error) {
	if !cfg.EnableLogging {
		return NewNopLogger(), nil
	}
	// TODO: create and return a gostratum/core logger instance here.
	// For now return a no-op logger to keep behavior consistent until
	// we wire the concrete core logger factory.
	return NewNopLogger(), nil
}

// LifecycleParams defines parameters for lifecycle management
type LifecycleParams struct {
	fx.In

	Lifecycle fx.Lifecycle
	Storage   Storage
	Logger    Logger `optional:"true"`
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
						params.Logger.Error("Error closing storage", "error", err)
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

// WithCustomKeyBuilder provides a custom key builder to the DI container
func WithCustomKeyBuilder(kb KeyBuilder) fx.Option {
	return fx.Supply(kb)
}

// WithCustomLogger provides a custom logger to the DI container
func WithCustomLogger(logger Logger) fx.Option {
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

	return fx.Module("storage", providers...)
}

// Example usage documentation

/*
Basic usage with fx:

	package main

	import (
		"context"
		"log"

		"go.uber.org/fx"
		"github.com/gostratum/storagex"
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

	import "github.com/gostratum/core/configx"

	func main() {
		c := configx.New()
		c.Set("storage.bucket", "my-bucket")
		c.Set("storage.endpoint", "http://localhost:9000")

		app := fx.New(
			storagex.Module,
			fx.Supply(c), // configx.Loader is automatically injected
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
