package storagex

import (
	"context"
	"fmt"

	"github.com/gostratum/core/configx"
	"github.com/gostratum/core/logx"
	"github.com/gostratum/metricsx"
	"github.com/gostratum/tracingx"
	"go.uber.org/fx"
)

// Module provides the storage module for fx.
// This base module provides configuration and key builder, but does NOT include
// a concrete storage provider. You must include an adapter module (e.g.,
// s3.Module()) to get a working Storage implementation.
//
// Example usage:
//
//	app := core.New(
//	    storagex.Module(),
//	    s3.Module(),  // Include the S3 adapter
//	    fx.Invoke(func(storage storagex.Storage) {
//	        // Use storage...
//	    }),
//	)
func Module() fx.Option {
	providers := []fx.Option{
		fx.Provide(
			NewConfig,
			NewKeyBuilder,
			NewObservabilityInstrumenter, // Provide optional observability
		),
	}

	// Only register lifecycle if storage is available
	// (when an adapter module is included)
	providers = append(providers, fx.Invoke(registerLifecycleIfAvailable))

	return fx.Module("storagex", providers...)
}

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
	cfg = cfg.Sanitize()
	if err := ValidateConfig(cfg); err != nil {
		return nil, fmt.Errorf("invalid configuration: %w", err)
	}

	return cfg, nil
}

// NewKeyBuilder creates a default key builder from configuration
func NewKeyBuilder(cfg *Config) KeyBuilder {
	if cfg.BasePrefix == "" {
		return &NoOpKeyBuilder{}
	}

	return NewPrefixKeyBuilder(cfg.BasePrefix)
}

// ObservabilityDeps defines optional observability dependencies
type ObservabilityDeps struct {
	fx.In

	Metrics metricsx.Metrics `optional:"true"`
	Tracer  tracingx.Tracer  `optional:"true"`
}

// NewObservabilityInstrumenter creates an instrumenter for storage operations
func NewObservabilityInstrumenter(deps ObservabilityDeps) *Instrumenter {
	return NewInstrumenter(deps.Metrics, deps.Tracer)
}

// LifecycleParams defines parameters for lifecycle management
type LifecycleParams struct {
	fx.In

	Lifecycle fx.Lifecycle
	Storage   Storage     `optional:"true"` // Optional: only present when adapter is included
	Logger    logx.Logger `optional:"true"`
}

// registerLifecycleIfAvailable registers shutdown hooks for graceful cleanup
// when a storage implementation is available (i.e., when an adapter module is included)
func registerLifecycleIfAvailable(params LifecycleParams) {
	if params.Storage == nil {
		// No storage implementation available - skip lifecycle registration
		if params.Logger != nil {
			params.Logger.Debug("StorageX module loaded without storage adapter")
		}
		return
	}

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

// WithCustomStorage provides a concrete Storage instance to the FX graph.
// Useful for tests or for applications that construct storage outside of
// adapter modules.
func WithCustomStorage(s Storage) fx.Option {
	return fx.Supply(s)
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

	import "github.com/gostratum/storagex/internal/testutil"

	func TestMyApp(t *testing.T) {
		app := fx.New(
			testutil.TestModule,
			fx.Invoke(func(cfg *storagex.Config) {
				// Use test configuration
			}),
		)

		// ...
	}
*/
