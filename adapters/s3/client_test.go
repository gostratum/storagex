package s3

import (
	"context"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/gostratum/core/logx"
	"github.com/gostratum/storagex"
)

func TestBuildAWSConfigWithLoader_Sources(t *testing.T) {
	logger := logx.NewNoopLogger()

	tests := []struct {
		name        string
		cfg         *storagex.Config
		wantSource  string
		expectError bool
		errorMsg    string
	}{
		// UseSDKDefaults = false (strict mode) - only explicit credentials
		{
			name:       "strict mode: static creds",
			cfg:        &storagex.Config{AccessKey: "A", SecretKey: "B", UseSDKDefaults: false},
			wantSource: "static",
		},
		{
			name:       "strict mode: profile",
			cfg:        &storagex.Config{Profile: "dev", UseSDKDefaults: false},
			wantSource: "profile",
		},
		{
			name:        "strict mode: no creds - should error",
			cfg:         &storagex.Config{UseSDKDefaults: false},
			expectError: true,
			errorMsg:    "UseSDKDefaults is false but no explicit credentials provided",
		},
		{
			name:        "strict mode: only access key - should error",
			cfg:         &storagex.Config{AccessKey: "A", UseSDKDefaults: false},
			expectError: true,
			errorMsg:    "UseSDKDefaults is false but no explicit credentials provided",
		},
		{
			name:        "strict mode: only secret key - should error",
			cfg:         &storagex.Config{SecretKey: "B", UseSDKDefaults: false},
			expectError: true,
			errorMsg:    "UseSDKDefaults is false but no explicit credentials provided",
		},

		// UseSDKDefaults = true (permissive mode) - allows SDK defaults
		{
			name:       "permissive mode: static creds take precedence",
			cfg:        &storagex.Config{AccessKey: "A", SecretKey: "B", UseSDKDefaults: true},
			wantSource: "static",
		},
		{
			name:       "permissive mode: profile takes precedence",
			cfg:        &storagex.Config{Profile: "dev", UseSDKDefaults: true},
			wantSource: "profile",
		},
		{
			name:       "permissive mode: sdk default fallback",
			cfg:        &storagex.Config{UseSDKDefaults: true},
			wantSource: "sdk-default",
		},
		{
			name:       "permissive mode: static creds win over profile",
			cfg:        &storagex.Config{AccessKey: "A", SecretKey: "B", Profile: "dev", UseSDKDefaults: true},
			wantSource: "static",
		},

		// Default behavior (UseSDKDefaults not set, defaults to false)
		{
			name:       "default: static creds work",
			cfg:        &storagex.Config{AccessKey: "A", SecretKey: "B"},
			wantSource: "static",
		},
		{
			name:       "default: profile works",
			cfg:        &storagex.Config{Profile: "dev"},
			wantSource: "profile",
		},
		{
			name:        "default: no creds - should error (strict by default)",
			cfg:         &storagex.Config{},
			expectError: true,
			errorMsg:    "UseSDKDefaults is false but no explicit credentials provided",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			loader := func(ctx context.Context, opts ...func(*config.LoadOptions) error) (aws.Config, error) {
				return aws.Config{}, nil
			}

			_, gotSource, err := buildAWSConfigWithLoader(context.Background(), tt.cfg, logger, loader)

			if tt.expectError {
				if err == nil {
					t.Fatalf("expected error containing %q, got nil", tt.errorMsg)
				}
				if tt.errorMsg != "" && !contains(err.Error(), tt.errorMsg) {
					t.Fatalf("expected error containing %q, got %q", tt.errorMsg, err.Error())
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if gotSource != tt.wantSource {
				t.Errorf("credential source mismatch: got %q, want %q", gotSource, tt.wantSource)
			}
		})
	}
}

// contains checks if a string contains a substring
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > len(substr) && containsHelper(s, substr))
}

func containsHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
