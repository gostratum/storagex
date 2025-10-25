# Changelog

All notable changes to StorageX will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

## [0.1.5] - 2025-10-26

### Changed
- **BREAKING**: Removed `ValidateFunc` from `ClientConfig` - S3 client creation no longer requires injecting validation functions
- **BREAKING**: Removed startup-time bucket validation - clients are created immediately without network calls
- Startup behavior now matches GoStratum framework patterns (fast startup, health check validation)
- Health checks handle S3 connectivity validation via `/healthz` endpoint instead of blocking startup

### Removed
- `ClientConfig.ValidateFunc` field - no longer needed for tests or production code
- `ClientManager.validateConnection()` method - validation moved to health checks
- Test-specific validation no-ops (`ValidateFunc: func(...) { return nil }`)

### Fixed
- Startup failures when S3/MinIO is temporarily unavailable - apps now start quickly and signal "not ready" via health checks
- Over-engineered test patterns that required passing no-op functions

### Migration Guide
If you were creating `ClientManager` directly (not common):

**Before:**
```go
manager, err := NewClientManager(ctx, ClientConfig{
    Config:       cfg,
    Logger:       logger,
    ValidateFunc: func(ctx context.Context, cm *ClientManager) error { return nil },
})
```

**After:**
```go
manager, err := NewClientManager(ctx, ClientConfig{
    Config: cfg,
    Logger: logger,
})
```

Most users consuming via `storagex.Module()` and `s3.Module()` are unaffected - the change is internal to the S3 adapter.

## [0.1.4] - 2025-10-25

### Added
- Credential sourcing: `use_sdk_defaults`, `profile`, and `role_arn` support for AWS SDK v2 default credential chain
- Runtime wiring: Refactored `buildAWSConfig` to be testable with injectable loader
- AssumeRole credential validation: Added `assume_role_validate_credentials` option (default: false)
- Lifecycle and cleanup: `S3Storage.Close()` forwards to `ClientManager.Close()` for proper FX shutdown
- Integration test scaffolding with localstack and docker-compose

### Changed
- AWS config builder now uses dependency injection for testing
- STS AssumeRole credentials are now swapped in when `role_arn` is configured
- Client performs optional credential retrieval before AssumeRole for better error messages

### Security
- Improved credential handling: prefer `use_sdk_defaults: true` in cloud environments
- Logs indicate credential source without printing secret values

## [0.1.3] - 2025-10-20

### Added
- Multi-tenant key builder support
- Presigned URL generation for direct client access
- Automatic multipart upload for large files
- Comprehensive error types with `errors.Is` support

### Changed
- Improved logging with structured fields
- Better error messages for configuration validation

## [0.1.2] - 2025-10-15

### Added
- MinIO compatibility with path-style addressing
- Configurable retry with exponential backoff
- Connection pooling and timeout configuration

### Fixed
- S3 endpoint URL construction for custom endpoints
- SSL configuration for local development

## [0.1.1] - 2025-10-10

### Added
- Initial S3 adapter implementation
- Basic CRUD operations (Put, Get, Delete, List)
- FX module integration
- Configuration via environment variables and YAML

### Changed
- Improved documentation with examples

## [0.1.0] - 2025-10-05

### Added
- Initial release of StorageX
- Core `Storage` interface definition
- Base configuration and error handling
- Key builder abstraction for multi-tenancy

[Unreleased]: https://github.com/gostratum/storagex/compare/v0.1.5...HEAD
[0.1.5]: https://github.com/gostratum/storagex/compare/v0.1.4...v0.1.5
[0.1.4]: https://github.com/gostratum/storagex/compare/v0.1.3...v0.1.4
[0.1.3]: https://github.com/gostratum/storagex/compare/v0.1.2...v0.1.3
[0.1.2]: https://github.com/gostratum/storagex/compare/v0.1.1...v0.1.2
[0.1.1]: https://github.com/gostratum/storagex/compare/v0.1.0...v0.1.1
[0.1.0]: https://github.com/gostratum/storagex/releases/tag/v0.1.0
