// Package storagex provides a dependency-injectable object storage abstraction
// with S3-compatible implementations (AWS S3, MinIO).
//
// The package is designed to be imported from the module root:
//
//	import "github.com/gostratum/storagex"
//
// Use the Fx module (`storagex.Module`) or the programmatic constructors to
// obtain a `storagex.Storage` implementation. Concrete providers (for example
// the S3-based provider) are intentionally placed under `internal/` and are
// registered when imported with a blank import, e.g.:
//
//	import (
//	    "github.com/gostratum/storagex"
//	    _ "github.com/gostratum/storagex/internal/s3"
//	)
//
// Keeping concrete providers in `internal/` prevents consumers from directly
// referencing provider internals and keeps the public API surface small and
// stable.
package storagex
