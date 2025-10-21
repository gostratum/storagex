package storagex

import (
	"testing"
	"time"
)

func TestValidateConfig_PermissiveRoleARNWithCustomEndpoint(t *testing.T) {
	// custom endpoint + RoleARN only -> permissive (allowed)
	cfg := &Config{
		Provider:        "s3",
		Bucket:          "test-bucket",
		Region:          "",
		Endpoint:        "http://minio.local:9000",
		UseSDKDefaults:  false,
		RoleARN:         "arn:aws:iam::123456789012:role/TestRole",
		RequestTimeout:  10 * time.Second,
		BackoffInitial:  200 * time.Millisecond,
		BackoffMax:      5 * time.Second,
		DefaultPartSize: 8 << 20,
		DefaultParallel: 4,
	}

	if err := ValidateConfig(cfg); err != nil {
		t.Fatalf("expected config to validate, got error: %v", err)
	}
}

func TestValidateConfig_CustomEndpointWithRoleAndExplicitCreds(t *testing.T) {
	// custom endpoint + RoleARN + explicit creds -> allowed
	cfg := &Config{
		Provider:        "s3",
		Bucket:          "test-bucket",
		Region:          "",
		Endpoint:        "http://minio.local:9000",
		UseSDKDefaults:  false,
		RoleARN:         "arn:aws:iam::123456789012:role/TestRole",
		AccessKey:       "AKIAEXAMPLE",
		SecretKey:       "SECRETEXAMPLE",
		RequestTimeout:  10 * time.Second,
		BackoffInitial:  200 * time.Millisecond,
		BackoffMax:      5 * time.Second,
		DefaultPartSize: 8 << 20,
		DefaultParallel: 4,
	}

	if err := ValidateConfig(cfg); err != nil {
		t.Fatalf("expected config to validate with explicit creds, got error: %v", err)
	}
}

func TestValidateConfig_CustomEndpointWithUseSDKDefaultsAndRole(t *testing.T) {
	// custom endpoint + UseSDKDefaults=true + RoleARN -> allowed (user opted in)
	cfg := &Config{
		Provider:        "s3",
		Bucket:          "test-bucket",
		Region:          "",
		Endpoint:        "http://minio.local:9000",
		UseSDKDefaults:  true,
		RoleARN:         "arn:aws:iam::123456789012:role/TestRole",
		RequestTimeout:  10 * time.Second,
		BackoffInitial:  200 * time.Millisecond,
		BackoffMax:      5 * time.Second,
		DefaultPartSize: 8 << 20,
		DefaultParallel: 4,
	}

	if err := ValidateConfig(cfg); err != nil {
		t.Fatalf("expected config to validate with UseSDKDefaults, got error: %v", err)
	}
}

func TestValidateConfig_AWSEndpointRoleOnly(t *testing.T) {
	// AWS endpoint (region provided) + RoleARN only -> allowed
	cfg := &Config{
		Provider:        "s3",
		Bucket:          "test-bucket",
		Region:          "us-east-1",
		Endpoint:        "",
		UseSDKDefaults:  false,
		RoleARN:         "arn:aws:iam::123456789012:role/TestRole",
		RequestTimeout:  10 * time.Second,
		BackoffInitial:  200 * time.Millisecond,
		BackoffMax:      5 * time.Second,
		DefaultPartSize: 8 << 20,
		DefaultParallel: 4,
	}

	if err := ValidateConfig(cfg); err != nil {
		t.Fatalf("expected AWS config with RoleARN to validate, got error: %v", err)
	}
}
