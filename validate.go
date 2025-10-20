package storagex

import (
	"fmt"
	"strings"
	"time"
)

// ValidationError represents a configuration validation error
type ValidationError struct {
	Field   string
	Message string
}

func (e *ValidationError) Error() string {
	return fmt.Sprintf("invalid config field %q: %s", e.Field, e.Message)
}

// ValidateConfig performs comprehensive validation of storage configuration
func ValidateConfig(cfg *Config) error {
	if cfg == nil {
		return &ValidationError{Field: "config", Message: "configuration cannot be nil"}
	}

	var errors []string

	// Validate provider
	if cfg.Provider == "" {
		errors = append(errors, "provider cannot be empty")
	} else if cfg.Provider != "s3" {
		errors = append(errors, fmt.Sprintf("unsupported provider %q, only 's3' is supported", cfg.Provider))
	}

	// Validate bucket
	if cfg.Bucket == "" {
		errors = append(errors, "bucket cannot be empty")
	} else if err := validateBucketName(cfg.Bucket); err != nil {
		errors = append(errors, fmt.Sprintf("invalid bucket name: %v", err))
	}

	// Validate region (required for AWS, optional for MinIO)
	if cfg.Region == "" && cfg.Endpoint == "" {
		errors = append(errors, "region is required when endpoint is not specified (AWS mode)")
	}

	// Validate credentials
	if cfg.AccessKey == "" {
		errors = append(errors, "access_key cannot be empty")
	}
	if cfg.SecretKey == "" {
		errors = append(errors, "secret_key cannot be empty")
	}

	// Validate timeouts
	if cfg.RequestTimeout <= 0 {
		errors = append(errors, "request_timeout must be positive")
	}
	if cfg.RequestTimeout > 10*time.Minute {
		errors = append(errors, "request_timeout should not exceed 10 minutes")
	}

	// Validate retry configuration
	if cfg.MaxRetries < 0 {
		errors = append(errors, "max_retries cannot be negative")
	}
	if cfg.MaxRetries > 10 {
		errors = append(errors, "max_retries should not exceed 10")
	}

	if cfg.BackoffInitial <= 0 {
		errors = append(errors, "backoff_initial must be positive")
	}
	if cfg.BackoffMax <= cfg.BackoffInitial {
		errors = append(errors, "backoff_max must be greater than backoff_initial")
	}

	// Validate multipart configuration
	if cfg.DefaultPartSize < 5<<20 { // 5MB minimum for S3
		errors = append(errors, "default_part_size must be at least 5MB for S3 compatibility")
	}
	if cfg.DefaultPartSize > 5<<30 { // 5GB maximum for S3
		errors = append(errors, "default_part_size must not exceed 5GB for S3 compatibility")
	}

	if cfg.DefaultParallel <= 0 {
		errors = append(errors, "default_parallel must be positive")
	}
	if cfg.DefaultParallel > 50 {
		errors = append(errors, "default_parallel should not exceed 50 for reasonable resource usage")
	}

	// Validate endpoint format if provided
	if cfg.Endpoint != "" {
		if err := validateEndpoint(cfg.Endpoint); err != nil {
			errors = append(errors, fmt.Sprintf("invalid endpoint: %v", err))
		}
	}

	// Validate base prefix format
	if cfg.BasePrefix != "" {
		if err := validateBasePrefix(cfg.BasePrefix); err != nil {
			errors = append(errors, fmt.Sprintf("invalid base_prefix: %v", err))
		}
	}

	if len(errors) > 0 {
		return &ValidationError{
			Field:   "config",
			Message: strings.Join(errors, "; "),
		}
	}

	return nil
}

// validateBucketName validates S3 bucket naming rules
func validateBucketName(bucket string) error {
	if len(bucket) < 3 || len(bucket) > 63 {
		return fmt.Errorf("bucket name must be between 3 and 63 characters")
	}

	if strings.HasPrefix(bucket, "-") || strings.HasSuffix(bucket, "-") {
		return fmt.Errorf("bucket name cannot start or end with a hyphen")
	}

	if strings.HasPrefix(bucket, ".") || strings.HasSuffix(bucket, ".") {
		return fmt.Errorf("bucket name cannot start or end with a period")
	}

	// Check for consecutive periods or hyphens
	if strings.Contains(bucket, "..") || strings.Contains(bucket, "--") {
		return fmt.Errorf("bucket name cannot contain consecutive periods or hyphens")
	}

	// Check for invalid characters (simplified check)
	for _, char := range bucket {
		if !isValidBucketChar(char) {
			return fmt.Errorf("bucket name contains invalid character: %c", char)
		}
	}

	// Check for IP address pattern (simplified)
	parts := strings.Split(bucket, ".")
	if len(parts) == 4 {
		// Could be an IP address, which is not allowed
		allNumeric := true
		for _, part := range parts {
			if !isNumeric(part) {
				allNumeric = false
				break
			}
		}
		if allNumeric {
			return fmt.Errorf("bucket name cannot be formatted as an IP address")
		}
	}

	return nil
}

// isValidBucketChar checks if a character is valid in S3 bucket names
func isValidBucketChar(char rune) bool {
	return (char >= 'a' && char <= 'z') ||
		(char >= '0' && char <= '9') ||
		char == '-' || char == '.'
}

// isNumeric checks if a string contains only digits
func isNumeric(s string) bool {
	if len(s) == 0 {
		return false
	}
	for _, char := range s {
		if char < '0' || char > '9' {
			return false
		}
	}
	return true
}

// validateEndpoint validates the endpoint URL format
func validateEndpoint(endpoint string) error {
	endpoint = strings.TrimSpace(endpoint)
	if endpoint == "" {
		return fmt.Errorf("endpoint cannot be empty")
	}

	// Allow endpoints with or without protocol
	if strings.HasPrefix(endpoint, "http://") || strings.HasPrefix(endpoint, "https://") {
		// Full URL format is acceptable
		return nil
	}

	// Host:port format
	if strings.Contains(endpoint, "://") {
		return fmt.Errorf("endpoint protocol must be http or https")
	}

	// Basic host validation (simplified)
	if strings.Contains(endpoint, " ") {
		return fmt.Errorf("endpoint cannot contain spaces")
	}

	return nil
}

// validateBasePrefix validates the base prefix template format
func validateBasePrefix(prefix string) error {
	if strings.HasPrefix(prefix, "/") || strings.HasSuffix(prefix, "/") {
		return fmt.Errorf("base prefix should not start or end with '/'")
	}

	// Check for dangerous patterns
	if strings.Contains(prefix, "..") {
		return fmt.Errorf("base prefix cannot contain '..' patterns")
	}

	if strings.Contains(prefix, "//") {
		return fmt.Errorf("base prefix cannot contain consecutive slashes")
	}

	return nil
}

// SanitizeConfig applies automatic fixes to configuration where possible
func SanitizeConfig(cfg *Config) *Config {
	if cfg == nil {
		return DefaultConfig()
	}

	// Create a copy to avoid mutating the original
	sanitized := *cfg

	// Apply defaults for missing values
	if sanitized.Provider == "" {
		sanitized.Provider = "s3"
	}

	if sanitized.Region == "" && sanitized.Endpoint == "" {
		sanitized.Region = "us-east-1"
	}

	if sanitized.RequestTimeout == 0 {
		sanitized.RequestTimeout = 30 * time.Second
	}

	if sanitized.MaxRetries == 0 {
		sanitized.MaxRetries = 3
	}

	if sanitized.BackoffInitial == 0 {
		sanitized.BackoffInitial = 200 * time.Millisecond
	}

	if sanitized.BackoffMax == 0 {
		sanitized.BackoffMax = 5 * time.Second
	}

	if sanitized.DefaultPartSize == 0 {
		sanitized.DefaultPartSize = 8 << 20 // 8MB
	}

	if sanitized.DefaultParallel == 0 {
		sanitized.DefaultParallel = 4
	}

	// Clean up endpoint
	if sanitized.Endpoint != "" {
		sanitized.Endpoint = strings.TrimSpace(sanitized.Endpoint)
		sanitized.Endpoint = strings.TrimSuffix(sanitized.Endpoint, "/")
	}

	// Clean up base prefix
	if sanitized.BasePrefix != "" {
		sanitized.BasePrefix = strings.Trim(sanitized.BasePrefix, "/")
	}

	return &sanitized
}

// ConfigSummary returns a safe summary of the configuration for logging
func ConfigSummary(cfg *Config) map[string]any {
	if cfg == nil {
		return map[string]any{"error": "nil config"}
	}

	summary := map[string]any{
		"provider":          cfg.Provider,
		"bucket":            cfg.Bucket,
		"region":            cfg.Region,
		"endpoint":          cfg.Endpoint,
		"use_path_style":    cfg.UsePathStyle,
		"request_timeout":   cfg.RequestTimeout.String(),
		"max_retries":       cfg.MaxRetries,
		"default_part_size": fmt.Sprintf("%d MB", cfg.DefaultPartSize/(1<<20)),
		"default_parallel":  cfg.DefaultParallel,
		"base_prefix":       cfg.BasePrefix,
		"disable_ssl":       cfg.DisableSSL,
		"enable_logging":    cfg.EnableLogging,
	}

	// Don't include sensitive information
	if cfg.AccessKey != "" {
		summary["has_access_key"] = true
		summary["access_key_prefix"] = cfg.AccessKey[:min(4, len(cfg.AccessKey))] + "..."
	}

	if cfg.SecretKey != "" {
		summary["has_secret_key"] = true
	}

	if cfg.SessionToken != "" {
		summary["has_session_token"] = true
	}

	return summary
}

// min returns the minimum of two integers
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
