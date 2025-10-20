package s3

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/aws/retry"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	s3Types "github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/cenkalti/backoff/v4"

	"github.com/gostratum/core/logx"
	"github.com/gostratum/storagex"
)

// ClientConfig holds the configuration for creating S3 clients
type ClientConfig struct {
	Config *storagex.Config
	Logger logx.Logger
}

// ClientManager manages S3 client instances and configurations
type ClientManager struct {
	s3Client      *s3.Client
	presignClient *s3.PresignClient
	config        *storagex.Config
	logger        logx.Logger
}

// NewClientManager creates a new S3 client manager
func NewClientManager(ctx context.Context, clientConfig ClientConfig) (*ClientManager, error) {
	if clientConfig.Config == nil {
		return nil, fmt.Errorf("config cannot be nil")
	}

	if clientConfig.Logger == nil {
		clientConfig.Logger = logx.NewNoopLogger()
	}

	cfg := clientConfig.Config
	logger := clientConfig.Logger

	logger.Debug("Creating S3 client manager", storagex.ArgsToFields(
		"bucket", cfg.Bucket,
		"region", cfg.Region,
		"endpoint", cfg.Endpoint,
		"use_path_style", cfg.UsePathStyle,
	)...)

	// Create AWS config
	awsConfig, err := buildAWSConfig(ctx, cfg, logger)
	if err != nil {
		return nil, fmt.Errorf("failed to build AWS config: %w", err)
	}

	// Create S3 service client
	s3Client := s3.NewFromConfig(awsConfig, func(o *s3.Options) {
		// Configure path-style addressing for MinIO compatibility
		if cfg.UsePathStyle {
			o.UsePathStyle = true
		}

		// Set custom endpoint if provided
		if cfg.Endpoint != "" {
			o.BaseEndpoint = aws.String(cfg.GetEndpointURL())
		}

		// Configure retries
		o.RetryMaxAttempts = cfg.MaxRetries
		o.RetryMode = aws.RetryModeAdaptive

		// Custom HTTP client with timeout
		o.HTTPClient = &http.Client{
			Timeout: cfg.RequestTimeout,
		}
	})

	// Create presign client
	presignClient := s3.NewPresignClient(s3Client)

	manager := &ClientManager{
		s3Client:      s3Client,
		presignClient: presignClient,
		config:        cfg,
		logger:        logger,
	}

	// Validate connectivity
	if err := manager.validateConnection(ctx); err != nil {
		return nil, fmt.Errorf("failed to validate S3 connection: %w", err)
	}

	logger.Info("S3 client manager created successfully", storagex.ArgsToFields(
		"bucket", cfg.Bucket,
		"region", cfg.Region,
	)...)

	return manager, nil
}

// buildAWSConfig creates the AWS SDK configuration
func buildAWSConfig(ctx context.Context, cfg *storagex.Config, logger logx.Logger) (aws.Config, error) {
	var options []func(*config.LoadOptions) error

	// Set region
	if cfg.Region != "" {
		options = append(options, config.WithRegion(cfg.Region))
	}

	// Set credentials
	if cfg.AccessKey != "" && cfg.SecretKey != "" {
		credProvider := credentials.NewStaticCredentialsProvider(
			cfg.AccessKey,
			cfg.SecretKey,
			cfg.SessionToken,
		)
		options = append(options, config.WithCredentialsProvider(credProvider))
	}

	// Configure retries with exponential backoff
	options = append(options, config.WithRetryer(func() aws.Retryer {
		return retry.NewStandard(func(o *retry.StandardOptions) {
			o.MaxAttempts = cfg.MaxRetries
			o.MaxBackoff = cfg.BackoffMax
			o.Backoff = createBackoffStrategy(cfg)
		})
	}))

	// Load the configuration
	awsConfig, err := config.LoadDefaultConfig(ctx, options...)
	if err != nil {
		return aws.Config{}, fmt.Errorf("unable to load AWS SDK config: %w", err)
	}

	logger.Debug("AWS config loaded", storagex.ArgsToFields(
		"region", awsConfig.Region,
		"max_retries", cfg.MaxRetries,
	)...)

	return awsConfig, nil
}

// createBackoffStrategy creates a custom backoff strategy
func createBackoffStrategy(cfg *storagex.Config) retry.BackoffDelayerFunc {
	return func(attempt int, err error) (time.Duration, error) {
		// Use exponential backoff with jitter
		b := backoff.NewExponentialBackOff()
		b.InitialInterval = cfg.BackoffInitial
		b.MaxInterval = cfg.BackoffMax
		b.MaxElapsedTime = 0 // No maximum elapsed time
		b.Multiplier = 2.0
		b.RandomizationFactor = 0.1

		// Reset backoff for each attempt sequence
		b.Reset()

		// Apply the backoff for the current attempt
		var delay time.Duration
		for i := 0; i < attempt; i++ {
			delay = b.NextBackOff()
			if delay == backoff.Stop {
				break
			}
		}

		return delay, nil
	}
}

// validateConnection performs a basic connectivity check
func (cm *ClientManager) validateConnection(ctx context.Context) error {
	// Try to head the bucket to verify access and connectivity
	_, err := cm.s3Client.HeadBucket(ctx, &s3.HeadBucketInput{
		Bucket: aws.String(cm.config.Bucket),
	})

	if err != nil {
		cm.logger.Warn("Failed to validate bucket access", storagex.ArgsToFields(
			"bucket", cm.config.Bucket,
			"error", err,
		)...)
		return fmt.Errorf("cannot access bucket %q: %w", cm.config.Bucket, err)
	}

	cm.logger.Debug("Bucket access validated", storagex.ArgsToFields("bucket", cm.config.Bucket)...)

	return nil
}

// GetS3Client returns the configured S3 client
func (cm *ClientManager) GetS3Client() *s3.Client {
	return cm.s3Client
}

// GetPresignClient returns the configured presign client
func (cm *ClientManager) GetPresignClient() *s3.PresignClient {
	return cm.presignClient
}

// GetConfig returns the storage configuration
func (cm *ClientManager) GetConfig() *storagex.Config {
	return cm.config
}

// GetLogger returns the logger instance
func (cm *ClientManager) GetLogger() logx.Logger {
	return cm.logger
}

// Close performs cleanup operations
func (cm *ClientManager) Close() error {
	cm.logger.Debug("Closing S3 client manager")

	// The AWS SDK clients don't require explicit cleanup,
	// but we can perform any custom cleanup here if needed

	return nil
}

// BucketExists checks if the configured bucket exists and is accessible
func (cm *ClientManager) BucketExists(ctx context.Context) (bool, error) {
	_, err := cm.s3Client.HeadBucket(ctx, &s3.HeadBucketInput{
		Bucket: aws.String(cm.config.Bucket),
	})

	if err != nil {
		// Check if it's a "not found" error vs other errors
		var notFound *s3Types.NotFound
		if errors.As(err, &notFound) {
			return false, nil
		}
		return false, fmt.Errorf("error checking bucket existence: %w", err)
	}

	return true, nil
}

// CreateBucketIfNotExists creates the bucket if it doesn't exist
func (cm *ClientManager) CreateBucketIfNotExists(ctx context.Context) error {
	exists, err := cm.BucketExists(ctx)
	if err != nil {
		return fmt.Errorf("failed to check if bucket exists: %w", err)
	}

	if exists {
		cm.logger.Debug("Bucket already exists", storagex.ArgsToFields("bucket", cm.config.Bucket)...)
		return nil
	}

	cm.logger.Info("Creating bucket", storagex.ArgsToFields("bucket", cm.config.Bucket)...)

	input := &s3.CreateBucketInput{
		Bucket: aws.String(cm.config.Bucket),
	}

	// For regions other than us-east-1, we need to specify the location constraint
	if cm.config.Region != "" && cm.config.Region != "us-east-1" {
		input.CreateBucketConfiguration = &s3Types.CreateBucketConfiguration{
			LocationConstraint: s3Types.BucketLocationConstraint(cm.config.Region),
		}
	}

	_, err = cm.s3Client.CreateBucket(ctx, input)
	if err != nil {
		return fmt.Errorf("failed to create bucket %q: %w", cm.config.Bucket, err)
	}

	cm.logger.Info("Bucket created successfully", storagex.ArgsToFields("bucket", cm.config.Bucket)...)
	return nil
}
