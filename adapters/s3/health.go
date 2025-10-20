package s3

import (
	"context"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/gostratum/core"
)

// s3HealthCheck implements core.Check for S3 connectivity
type s3HealthCheck struct {
	client *ClientManager
}

func (s *s3HealthCheck) Name() string { return "storagex.s3" }

func (s *s3HealthCheck) Kind() core.Kind { return core.Readiness }

func (s *s3HealthCheck) Check(ctx context.Context) error {
	if s.client == nil {
		return fmt.Errorf("no client manager")
	}

	// Use a short timeout for health checks
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()

	// Attempt to head the bucket
	_, err := s.client.GetS3Client().HeadBucket(ctx, &s3.HeadBucketInput{
		Bucket: aws.String(s.client.GetConfig().Bucket),
	})
	if err != nil {
		return fmt.Errorf("s3 head bucket failed: %w", err)
	}
	return nil
}
