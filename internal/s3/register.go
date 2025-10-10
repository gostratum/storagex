package s3

import (
	"context"

	"github.com/gostratum/storagex/pkg/storagex"
)

// init registers the S3 storage implementation
func init() {
	storagex.NewStorageFunc = func(ctx context.Context, cfg *storagex.Config, opts ...storagex.Option) (storagex.Storage, error) {
		return NewS3Storage(ctx, cfg, opts...)
	}
}
