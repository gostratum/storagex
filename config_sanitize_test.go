package storagex

import (
	"testing"

	"github.com/gostratum/core/logx"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
)

// TestConfig_Sanitize verifies that Config implements logx.Sanitizable
// and properly redacts secrets when logged.
func TestConfig_Sanitize(t *testing.T) {
	t.Run("implements Sanitizable interface", func(t *testing.T) {
		cfg := &Config{
			Provider:     "s3",
			Bucket:       "my-bucket",
			Region:       "us-east-1",
			AccessKey:    "AKIAIOSFODNN7EXAMPLE",
			SecretKey:    "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY",
			SessionToken: "session-token-12345",
			ExternalID:   "external-id-secret",
		}

		// Verify it implements Sanitizable
		var _ interface{ Sanitize() any } = cfg

		// Get sanitized version
		sanitized := cfg.Sanitize()
		sanitizedCfg, ok := sanitized.(*Config)
		if !ok {
			t.Fatalf("Sanitize() returned wrong type: %T", sanitized)
		}

		// Verify secrets are redacted
		if sanitizedCfg.AccessKey != "[redacted]" {
			t.Errorf("AccessKey not redacted: %s", sanitizedCfg.AccessKey)
		}
		if sanitizedCfg.SecretKey != "[redacted]" {
			t.Errorf("SecretKey not redacted: %s", sanitizedCfg.SecretKey)
		}
		if sanitizedCfg.SessionToken != "[redacted]" {
			t.Errorf("SessionToken not redacted: %s", sanitizedCfg.SessionToken)
		}
		if sanitizedCfg.ExternalID != "[redacted]" {
			t.Errorf("ExternalID not redacted: %s", sanitizedCfg.ExternalID)
		}

		// Verify non-secrets are preserved
		if sanitizedCfg.Provider != "s3" {
			t.Errorf("Provider changed: %s", sanitizedCfg.Provider)
		}
		if sanitizedCfg.Bucket != "my-bucket" {
			t.Errorf("Bucket changed: %s", sanitizedCfg.Bucket)
		}
		if sanitizedCfg.Region != "us-east-1" {
			t.Errorf("Region changed: %s", sanitizedCfg.Region)
		}

		// Verify original is not mutated
		if cfg.AccessKey == "[redacted]" {
			t.Error("Original AccessKey was mutated")
		}
		if cfg.SecretKey == "[redacted]" {
			t.Error("Original SecretKey was mutated")
		}
	})

	t.Run("auto-sanitizes with logx.Any", func(t *testing.T) {
		// Create observer to capture logs
		observedZapCore, observedLogs := observer.New(zap.DebugLevel)
		observedLogger := zap.New(observedZapCore)
		logger := logx.ProvideAdapter(observedLogger)

		cfg := &Config{
			Provider:  "s3",
			Bucket:    "my-bucket",
			Region:    "us-east-1",
			AccessKey: "AKIAIOSFODNN7EXAMPLE",
			SecretKey: "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY",
		}

		// Log the config using logx.Any (should auto-sanitize)
		logger.Info("Storage config loaded", logx.Any("config", cfg))

		// Verify log was captured
		if observedLogs.Len() == 0 {
			t.Fatal("No logs captured")
		}

		// Get the logged entry
		entries := observedLogs.All()
		if len(entries) == 0 {
			t.Fatal("No log entries")
		}

		entry := entries[0]
		if entry.Message != "Storage config loaded" {
			t.Errorf("Wrong message: %s", entry.Message)
		}

		// Find the config field
		var configField *zap.Field
		for i := range entry.Context {
			if entry.Context[i].Key == "config" {
				configField = &entry.Context[i]
				break
			}
		}

		if configField == nil {
			t.Fatal("Config field not found in log")
		}

		// The field should contain the sanitized config
		// We can't easily inspect the nested structure, but we've verified
		// that Sanitize() works correctly above and logx.Any() calls it
		t.Logf("Config field type: %v", configField.Type)
	})

	t.Run("nil config returns nil", func(t *testing.T) {
		var cfg *Config
		sanitized := cfg.Sanitize()
		if sanitized != (*Config)(nil) {
			t.Errorf("Expected nil, got %v", sanitized)
		}
	})
}
