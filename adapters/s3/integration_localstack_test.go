//go:build integration

package s3

import (
	"context"
	"os"
	"os/exec"
	"testing"
	"time"

	"github.com/gostratum/storagex"
)

func ensureDockerCompose(t *testing.T) {
	if _, err := exec.LookPath("docker-compose"); err != nil {
		t.Skip("docker-compose not found; skipping integration")
	}
}

func startLocalstack(t *testing.T) {
	ensureDockerCompose(t)
	cmd := exec.Command("docker-compose", "-f", "test/localstack/docker-compose.yml", "up", "-d")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("failed to start localstack: %v", err)
	}
	// wait for health
	time.Sleep(5 * time.Second)
}

func stopLocalstack(t *testing.T) {
	cmd := exec.Command("docker-compose", "-f", "test/localstack/docker-compose.yml", "down")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	_ = cmd.Run()
}

func TestLocalstackS3Basic(t *testing.T) {
	startLocalstack(t)
	defer stopLocalstack(t)

	// Configure test env to point AWS SDK to localstack
	os.Setenv("AWS_ACCESS_KEY_ID", "test")
	os.Setenv("AWS_SECRET_ACCESS_KEY", "test")
	os.Setenv("AWS_REGION", "us-east-1")
	endpoint := "http://localhost:4566"

	ctx := context.Background()
	cfg := &storagex.Config{
		Provider:       "s3",
		Bucket:         "integration-bucket",
		Region:         "us-east-1",
		Endpoint:       endpoint,
		UseSDKDefaults: true,
	}

	// Create storage (no RoleARN here)
	s, err := NewS3Storage(ctx, cfg)
	if err != nil {
		t.Fatalf("failed to create storage: %v", err)
	}
	defer s.Close()

	// Attempt to create bucket via client manager
	cm := s.(*S3Storage).client
	if err := cm.CreateBucketIfNotExists(ctx); err != nil {
		t.Fatalf("CreateBucketIfNotExists failed: %v", err)
	}

	// Head the bucket
	exists, err := cm.BucketExists(ctx)
	if err != nil {
		t.Fatalf("BucketExists failed: %v", err)
	}
	if !exists {
		t.Fatalf("expected bucket to exist")
	}

	// Put a small object
	_, err = s.PutBytes(ctx, "hello.txt", []byte("hello"), nil)
	if err != nil {
		t.Fatalf("PutBytes failed: %v", err)
	}

	t.Log("Localstack S3 integration test passed")
}
