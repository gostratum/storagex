package s3

import (
	"context"

	"github.com/gostratum/storagex"
	"go.uber.org/fx"
)

// Module returns an fx.Module which provides the S3 storage implementation.
// Consumers should opt-in this module explicitly (e.g. s3.Module()) instead
// of relying on package init side-effects.
func Module() fx.Option {
	return fx.Module("storage-s3",
		fx.Provide(
			provideS3Storage,
		),
	)
}

// provideS3Storage is an fx-friendly constructor that creates an S3 storage
// instance. It accepts optional key builder and logger from the FX graph.
func provideS3Storage(cfg *storagex.Config, kb storagex.KeyBuilder, logger storagex.Logger) (storagex.Storage, error) {
	var opts []storagex.Option
	if kb != nil {
		opts = append(opts, storagex.WithKeyBuilder(kb))
	}
	if logger != nil {
		opts = append(opts, storagex.WithLogger(logger))
	}

	// Use background context for construction; lifecycle-managed creation
	// can be done by callers using NewStorageFromParams when necessary.
	s, err := NewS3Storage(context.Background(), cfg, opts...)
	if err != nil {
		return nil, err
	}
	return s, nil
}
