package s3

import (
	"context"
	"os"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/gostratum/core/logx"
	"github.com/gostratum/storagex"
)

// TestMinIOConnection tests the fix for MinIO connections with UseSDKDefaults flag.
// This test validates that the credential handling changes properly support MinIO
// when using environment variables with UseSDKDefaults=true.
func TestMinIOConnection(t *testing.T) {
	// Set up MinIO-style environment variables
	os.Setenv("AWS_ACCESS_KEY_ID", "minioadmin")
	os.Setenv("AWS_SECRET_ACCESS_KEY", "minioadmin")
	defer os.Unsetenv("AWS_ACCESS_KEY_ID")
	defer os.Unsetenv("AWS_SECRET_ACCESS_KEY")

	tests := []struct {
		name          string
		config        *storagex.Config
		expectSuccess bool
		description   string
	}{
		{
			name: "MinIO with UseSDKDefaults=true and env vars",
			config: &storagex.Config{
				Provider:       "s3",
				Bucket:         "test-bucket",
				Region:         "us-east-1",
				Endpoint:       "http://localhost:9000",
				UsePathStyle:   true,
				UseSDKDefaults: true,
				DisableSSL:     true,
			},
			expectSuccess: true,
			description:   "Should successfully connect to MinIO using SDK defaults (env vars)",
		},
		{
			name: "MinIO with explicit credentials",
			config: &storagex.Config{
				Provider:     "s3",
				Bucket:       "test-bucket",
				Region:       "us-east-1",
				Endpoint:     "http://localhost:9000",
				UsePathStyle: true,
				AccessKey:    "minioadmin",
				SecretKey:    "minioadmin",
				DisableSSL:   true,
			},
			expectSuccess: true,
			description:   "Should successfully connect to MinIO using explicit credentials",
		},
		{
			name: "MinIO with UseSDKDefaults=false and no credentials",
			config: &storagex.Config{
				Provider:       "s3",
				Bucket:         "test-bucket",
				Region:         "us-east-1",
				Endpoint:       "http://localhost:9000",
				UsePathStyle:   true,
				UseSDKDefaults: false,
				DisableSSL:     true,
			},
			expectSuccess: false,
			description:   "Should fail validation when UseSDKDefaults=false and no explicit credentials",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			logger := logx.NewNoopLogger()

			// First validate the config
			if err := storagex.ValidateConfig(tt.config); err != nil {
				if tt.expectSuccess {
					t.Fatalf("config validation failed unexpectedly: %v", err)
				}
				t.Logf("Config validation failed as expected: %v", err)
				return
			}

			// Try to create the client manager
			clientConfig := ClientConfig{
				Config: tt.config,
				Logger: logger,
			}

			manager, err := NewClientManager(ctx, clientConfig)
			if err != nil {
				if tt.expectSuccess {
					t.Fatalf("failed to create client manager: %v\nDescription: %s", err, tt.description)
				}
				t.Logf("Client manager creation failed as expected: %v", err)
				return
			}
			defer manager.Close()

			if !tt.expectSuccess {
				t.Fatalf("expected client manager creation to fail, but it succeeded\nDescription: %s", tt.description)
			}

			// Verify we can check if bucket exists (connection test)
			exists, err := manager.BucketExists(ctx)
			if err != nil {
				t.Logf("Note: Bucket check failed (MinIO may not be running): %v", err)
				t.Skip("Skipping further tests - MinIO not available")
			}

			t.Logf("✓ Successfully connected to MinIO - bucket exists: %v", exists)
		})
	}
}

// TestMinIOCredentialSourceDetection verifies that the credential source is correctly
// identified when using different configurations with MinIO.
func TestMinIOCredentialSourceDetection(t *testing.T) {
	// Set up MinIO-style environment variables
	os.Setenv("AWS_ACCESS_KEY_ID", "minioadmin")
	os.Setenv("AWS_SECRET_ACCESS_KEY", "minioadmin")
	defer os.Unsetenv("AWS_ACCESS_KEY_ID")
	defer os.Unsetenv("AWS_SECRET_ACCESS_KEY")

	tests := []struct {
		name           string
		config         *storagex.Config
		expectedSource string
	}{
		{
			name: "Explicit credentials",
			config: &storagex.Config{
				Provider:  "s3",
				Bucket:    "test",
				Region:    "us-east-1",
				Endpoint:  "http://localhost:9000",
				AccessKey: "minioadmin",
				SecretKey: "minioadmin",
			},
			expectedSource: "static",
		},
		{
			name: "SDK defaults with env vars",
			config: &storagex.Config{
				Provider:       "s3",
				Bucket:         "test",
				Region:         "us-east-1",
				Endpoint:       "http://localhost:9000",
				UseSDKDefaults: true,
			},
			expectedSource: "sdk-default",
		},
		{
			name: "Explicit credentials take precedence over SDK defaults",
			config: &storagex.Config{
				Provider:       "s3",
				Bucket:         "test",
				Region:         "us-east-1",
				Endpoint:       "http://localhost:9000",
				AccessKey:      "minioadmin",
				SecretKey:      "minioadmin",
				UseSDKDefaults: true,
			},
			expectedSource: "static",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			logger := logx.NewNoopLogger()

			// Use the testable loader
			loader := func(ctx context.Context, opts ...func(*config.LoadOptions) error) (aws.Config, error) {
				return config.LoadDefaultConfig(ctx, opts...)
			}

			awsConfig, credSource, err := buildAWSConfigWithLoader(ctx, tt.config, logger, loader)
			if err != nil {
				t.Fatalf("buildAWSConfigWithLoader failed: %v", err)
			}

			if credSource != tt.expectedSource {
				t.Errorf("credential source mismatch: got %q, want %q", credSource, tt.expectedSource)
			}

			// Verify awsConfig is valid
			if tt.config.Region != "" && awsConfig.Region != tt.config.Region {
				t.Errorf("region mismatch: got %q, want %q", awsConfig.Region, tt.config.Region)
			}

			t.Logf("✓ Credential source correctly identified as: %s", credSource)
		})
	}
}
