package storagex

import (
	"context"
	"fmt"

	"github.com/gostratum/core/configx"
	"github.com/gostratum/core/logx"
	"go.uber.org/fx"
)

// Module is the Fx module that provides storage functionality
var Module = fx.Module("storage",
	fx.Provide(
		NewConfig,
		NewKeyBuilder,
	),
	fx.Invoke(registerLifecycle),
)

// StorageParams defines the parameters needed for storage creation
type StorageParams struct {
	fx.In

	Config     *Config
	Logger     logx.Logger `optional:"true"`
	KeyBuilder KeyBuilder  `optional:"true"`
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

// NewStorage creates a new Storage implementation.
//
// NOTE: storagex does not include built-in provider implementations. You must
// include an adapter module (for example, `github.com/gostratum/storagex/adapters/s3.Module()`)
// in your FX application or construct a provider directly (for example,
// `adapters/s3.NewS3Storage`). This function previously relied on a global
// provider registration (deprecated); that pattern has been removed.
func NewStorage(params StorageParams) (Storage, error) {
	return nil, fmt.Errorf("no storage implementation registered - include an adapter module (e.g. s3.Module()) or construct a provider directly")
}

// NewStorageFromParams creates a new Storage implementation using the provided
// context. This allows callers to control cancellation and timeouts during
// storage initialization. Providers should prefer this constructor when they
// need to run network requests or long-running setup during creation.
func NewStorageFromParams(ctx context.Context, params StorageParams) (Storage, error) {
	return nil, fmt.Errorf("no storage implementation registered - include an adapter module (e.g. s3.Module()) or construct a provider directly")
}

// NewKeyBuilder creates a default key builder from configuration
func NewKeyBuilder(cfg *Config) KeyBuilder {
	if cfg.BasePrefix == "" {
		return &NoOpKeyBuilder{}
	}

	return NewPrefixKeyBuilder(cfg.BasePrefix)
}

// LifecycleParams defines parameters for lifecycle management
type LifecycleParams struct {
	fx.In

	Lifecycle fx.Lifecycle
	Storage   Storage
	Logger    logx.Logger `optional:"true"`
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
						params.Logger.Error("Error closing storage", ArgsToFields("error", err)...)
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
func WithCustomLogger(logger logx.Logger) fx.Option {
	return fx.Supply(logger)
}

// WithCustomStorage provides a concrete Storage instance to the FX graph.
// Useful for tests or for applications that construct storage outside of
// adapter modules.
func WithCustomStorage(s Storage) fx.Option {
	return fx.Supply(s)
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
	return nil, fmt.Errorf("no storage implementation registered - include an adapter (e.g. s3.Module()) or construct a provider directly")
}
