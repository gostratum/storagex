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
		name       string
		cfg        *storagex.Config
		loader     func(ctx context.Context, opts ...func(*aws.Config)) (aws.Config, error)
		wantSource string
	}{
		{
			name: "static creds",
			cfg:  &storagex.Config{AccessKey: "A", SecretKey: "B"},
			loader: func(ctx context.Context, opts ...func(*aws.Config)) (aws.Config, error) {
				return aws.Config{}, nil
			},
			wantSource: "static",
		},
		{
			name: "profile selected",
			cfg:  &storagex.Config{Profile: "dev"},
			loader: func(ctx context.Context, opts ...func(*aws.Config)) (aws.Config, error) {
				return aws.Config{}, nil
			},
			wantSource: "profile",
		},
		{
			name: "sdk default",
			cfg:  &storagex.Config{},
			loader: func(ctx context.Context, opts ...func(*aws.Config)) (aws.Config, error) {
				return aws.Config{}, nil
			},
			wantSource: "sdk-default",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// adapt loader signature to awsConfigLoader
			loader := func(ctx context.Context, opts ...func(*config.LoadOptions) error) (aws.Config, error) {
				return aws.Config{}, nil
			}
			// Note: The loader itself isn't doing anything here since buildAWSConfigWithLoader
			// mainly inspects cfg fields to determine cred source. We call it and assert the source.
			_, gotSource, err := buildAWSConfigWithLoader(context.Background(), tt.cfg, logger, loader)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if gotSource != tt.wantSource {
				t.Fatalf("got source %q, want %q", gotSource, tt.wantSource)
			}
		})
	}
}
