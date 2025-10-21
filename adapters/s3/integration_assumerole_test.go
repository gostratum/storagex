//go:build integration

package s3

import (
    "context"
    "os"
    "testing"

    "github.com/gostratum/storagex"
)

// This integration test requires a running localstack or AWS endpoint that supports STS.
// It is intentionally skipped by default; set LOCALSTACK_ENDPOINT to run.
func TestAssumeRoleIntegration(t *testing.T) {
    ep := os.Getenv("LOCALSTACK_ENDPOINT")
    if ep == "" {
        t.Skip("LOCALSTACK_ENDPOINT not set; skipping integration test")
    }

    ctx := context.Background()

    cfg := &storagex.Config{
        Provider:       "s3",
        Bucket:         "test-bucket",
        Region:         "us-east-1",
        Endpoint:       ep,
        UseSDKDefaults: true,
        RoleARN:        os.Getenv("TEST_ROLE_ARN"),
    }

    // If no role provided, skip
    if cfg.RoleARN == "" {
        t.Skip("TEST_ROLE_ARN not set; skipping AssumeRole integration test")
    }

    // Build storage via NewS3Storage which will load SDK config and attempt AssumeRole
    _, err := NewS3Storage(ctx, cfg)
    if err != nil {
        t.Fatalf("failed to create S3 storage: %v", err)
    }
}
package s3
//go:build integration

package s3

//go:build integration

package s3

import (
    "context"
    "os"
    "testing"

    "github.com/gostratum/storagex"
)

// This integration test requires a running localstack or AWS endpoint that supports STS.
// It is intentionally skipped by default; set LOCALSTACK_ENDPOINT to run.
func TestAssumeRoleIntegration(t *testing.T) {
    ep := os.Getenv("LOCALSTACK_ENDPOINT")
    if ep == "" {
        t.Skip("LOCALSTACK_ENDPOINT not set; skipping integration test")
    }

    ctx := context.Background()

    cfg := &storagex.Config{
        Provider:       "s3",
        Bucket:         "test-bucket",
        Region:         "us-east-1",
        Endpoint:       ep,
        UseSDKDefaults: true,
        RoleARN:        os.Getenv("TEST_ROLE_ARN"),
    }

    // If no role provided, skip
    if cfg.RoleARN == "" {
        t.Skip("TEST_ROLE_ARN not set; skipping AssumeRole integration test")
    }

    // Build storage via NewS3Storage which will load SDK config and attempt AssumeRole
    _, err := NewS3Storage(ctx, cfg)
    if err != nil {
        t.Fatalf("failed to create S3 storage: %v", err)
    }
}
