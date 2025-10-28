package testutil

import (
	"github.com/gostratum/storagex"
	"go.uber.org/fx"
)

// TestModule provides a module for testing with mock/test implementations.
// This module provides a test configuration and key builder suitable for
// unit tests without requiring external configuration.
//
// Example usage:
//
//	import "github.com/gostratum/storagex/internal/testutil"
//
//	func TestMyApp(t *testing.T) {
//	    app := fx.New(
//	        testutil.TestModule,
//	        fx.Invoke(func(cfg *storagex.Config) {
//	            // Use test config
//	        }),
//	    )
//	    // ...
//	}
var TestModule = fx.Module("storagex-test",
	fx.Provide(
		NewTestConfig,
		NewTestKeyBuilder,
	),
)

// NewTestConfig creates a test configuration suitable for unit tests.
// The configuration points to a local MinIO instance with default credentials.
func NewTestConfig() *storagex.Config {
	cfg := storagex.DefaultConfig()
	cfg.Bucket = "test-bucket"
	cfg.Endpoint = "http://localhost:9000"
	cfg.UsePathStyle = true
	cfg.AccessKey = "minioadmin"
	cfg.SecretKey = "minioadmin"
	cfg.DisableSSL = true
	cfg.EnableLogging = true
	return cfg
}

// NewTestKeyBuilder creates a test key builder with a "test" prefix.
func NewTestKeyBuilder() storagex.KeyBuilder {
	return storagex.NewPrefixKeyBuilder("test")
}
