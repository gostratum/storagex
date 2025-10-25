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

	"github.com/aws/aws-sdk-go-v2/credentials/stscreds"
	"github.com/aws/aws-sdk-go-v2/service/sts"
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

	// Create AWS config (capture credential source for logging)
	awsConfig, credSource, err := buildAWSConfigWithLoader(ctx, cfg, logger, func(ctx context.Context, opts ...func(*config.LoadOptions) error) (aws.Config, error) {
		return config.LoadDefaultConfig(ctx, opts...)
	})
	if err != nil {
		return nil, fmt.Errorf("failed to build AWS config: %w", err)
	}

	logger.Info("Credential source selected", storagex.ArgsToFields("cred_source", credSource)...)

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

// awsConfigLoader is a function that loads an aws.Config given LoadOptions.
type awsConfigLoader func(ctx context.Context, opts ...func(*config.LoadOptions) error) (aws.Config, error)

// buildAWSConfigWithLoader builds an AWS config using the supplied loader (testable).
// It returns the loaded aws.Config and the detected credential source (one of:
// "static", "profile", "sdk-default", "assumed-role").
func buildAWSConfigWithLoader(ctx context.Context, cfg *storagex.Config, logger logx.Logger, loader awsConfigLoader) (aws.Config, string, error) {
	var options []func(*config.LoadOptions) error
	credSource := "unknown"

	// Set region
	if cfg.Region != "" {
		options = append(options, config.WithRegion(cfg.Region))
	}

	logger.Debug("Storage config values", storagex.ArgsToFields(
		"access_key_set", cfg.AccessKey != "",
		"secret_key_set", cfg.SecretKey != "",
		"use_sdk_defaults", cfg.UseSDKDefaults,
		"endpoint", cfg.Endpoint,
		"bucket", cfg.Bucket,
	)...)

	// Credential handling based on UseSDKDefaults flag
	if !cfg.UseSDKDefaults {
		// When UseSDKDefaults is false, only use explicitly provided credentials
		if cfg.AccessKey != "" && cfg.SecretKey != "" {
			credProvider := credentials.NewStaticCredentialsProvider(
				cfg.AccessKey,
				cfg.SecretKey,
				cfg.SessionToken,
			)
			options = append(options, config.WithCredentialsProvider(credProvider))
			credSource = "static"
		} else if cfg.Profile != "" {
			options = append(options, config.WithSharedConfigProfile(cfg.Profile))
			credSource = "profile"
		} else {
			return aws.Config{}, credSource, fmt.Errorf("UseSDKDefaults is false but no explicit credentials provided (access_key/secret_key or profile)")
		}
	} else {
		// When UseSDKDefaults is true, prefer explicit credentials but allow SDK defaults as fallback
		if cfg.AccessKey != "" && cfg.SecretKey != "" {
			credProvider := credentials.NewStaticCredentialsProvider(cfg.AccessKey, cfg.SecretKey, cfg.SessionToken)
			options = append(options, config.WithCredentialsProvider(credProvider))
			credSource = "static"
		} else if cfg.Profile != "" {
			options = append(options, config.WithSharedConfigProfile(cfg.Profile))
			credSource = "profile"
		}
		// If no explicit credentials, loader will use SDK default chain
	}

	// Configure retries with exponential backoff
	options = append(options, config.WithRetryer(func() aws.Retryer {
		return retry.NewStandard(func(o *retry.StandardOptions) {
			o.MaxAttempts = cfg.MaxRetries
			o.MaxBackoff = cfg.BackoffMax
			o.Backoff = createBackoffStrategy(cfg)
		})
	}))

	// Load the configuration via injected loader
	awsConfig, err := loader(ctx, options...)
	if err != nil {
		return aws.Config{}, credSource, fmt.Errorf("unable to load AWS SDK config: %w", err)
	}

	if credSource == "unknown" {
		credSource = "sdk-default"
	}

	logger.Debug("AWS config loaded", storagex.ArgsToFields(
		"region", awsConfig.Region,
		"max_retries", cfg.MaxRetries,
		"cred_source", credSource,
	)...)

	// If RoleARN is set, create an STS AssumeRole provider and swap credentials
	if cfg.RoleARN != "" {
		// NOTE: RoleARN is not a credential by itself — it instructs the SDK to
		// call STS:AssumeRole and obtain temporary credentials. AssumeRole will
		// use the already-loaded credentials from awsConfig as the source to
		// authenticate to STS. Credential precedence is:
		//   1) static credentials (access_key + secret_key)
		//   2) shared profile (profile)
		//   3) SDK default chain (env, instance profile, etc)
		//
		// For non-AWS/custom endpoints (e.g., MinIO) STS may not be available,
		// so AssumeRole can fail at runtime. We attempt a lightweight
		// credential retrieval here to fail fast with a clearer error when the
		// source provider cannot produce credentials.
		logger.Info("Config requests STS AssumeRole", storagex.ArgsToFields("role_arn", cfg.RoleARN)...)

		// Attempt to resolve underlying credentials quickly to provide a better
		// error when none are available. Use a short timeout to avoid blocking
		// startup for long when providers like IMDS are slow. This behavior can
		// be disabled via cfg.AssumeRoleValidateCredentials (default: false)
		// to avoid startup network calls in restrictive environments.
		if awsConfig.Credentials != nil {
			if cfg.AssumeRoleValidateCredentials {
				ctxTimeout, cancel := context.WithTimeout(ctx, 2*time.Second)
				defer cancel()
				if _, derr := awsConfig.Credentials.Retrieve(ctxTimeout); derr != nil {
					// If retrieval failed, return an actionable error instead of
					// proceeding to create an AssumeRole provider which would fail
					// later with a less clear message.
					return aws.Config{}, credSource, fmt.Errorf("unable to resolve underlying credentials for assume-role: %w", derr)
				}
			} else {
				// Log a warning so users know that assume-role may fail at
				// runtime if underlying credentials are absent. We continue to
				// allow permissive configurations (e.g., custom endpoints) by
				// default.
				logger.Warn("assume-role credential validation is disabled; assume-role may fail at runtime if underlying credentials are missing", storagex.ArgsToFields("role_arn", cfg.RoleARN)...)
			}
		}

		stsClient := sts.NewFromConfig(awsConfig)
		assumeProv := stscreds.NewAssumeRoleProvider(stsClient, cfg.RoleARN, func(o *stscreds.AssumeRoleOptions) {
			if cfg.ExternalID != "" {
				o.ExternalID = &cfg.ExternalID
			}
			o.RoleSessionName = "storagex-assume-role"
		})

		awsConfig.Credentials = aws.NewCredentialsCache(assumeProv)
		credSource = "assumed-role"
	}

	return awsConfig, credSource, nil
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
