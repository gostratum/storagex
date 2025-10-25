# StorageX Module — AI Coding Agent Instructions

**S3-compatible object storage abstraction with clean architecture and FX dependency injection**

## Module Overview

StorageX is a **production-ready object storage module** that provides:
- Unified interface for S3-compatible storage (AWS S3, MinIO, DigitalOcean Spaces, etc.)
- Pluggable adapter pattern (S3 adapter in `adapters/s3/`)
- Multi-tenant key building with organizational prefixes
- Automatic multipart uploads for large files
- Presigned URLs for direct client access
- Full context-aware operations with proper cancellation

**Package Structure:**
```
storagex/
  ├── fxmodule.go          # Base FX module (config + key builder)
  ├── config.go            # Configuration with validation
  ├── storage.go           # Storage interface definition
  ├── keybuilder.go        # Key building strategies
  └── adapters/
      └── s3/
          ├── module.go    # S3 adapter FX module
          ├── client.go    # AWS SDK v2 client management
          └── storage.go   # S3 Storage implementation
```

## Critical Patterns (DO NOT DEVIATE)

### 1. Two-Module Architecture

**Base Module** (`storagex.Module()`):
- Provides configuration (`*storagex.Config`)
- Provides key builder (`storagex.KeyBuilder`)
- Does **NOT** provide a concrete `storagex.Storage` implementation

**Adapter Module** (e.g., `s3.Module()`):
- Provides the actual `storagex.Storage` implementation
- Registers lifecycle hooks for connection management
- **Required** for a working storage system

```go
// ✅ Correct - includes both modules
app := core.New(
    storagex.Module(),  // Config + key builder
    s3.Module(),        // S3 storage implementation
    fx.Invoke(func(storage storagex.Storage) {
        // Storage is ready to use
    }),
)

// ❌ Wrong - missing adapter module
app := core.New(
    storagex.Module(),  // Only config, no storage implementation!
    fx.Invoke(func(storage storagex.Storage) {
        // Will fail - no provider for Storage
    }),
)
```

### 2. Configuration Pattern

**Config prefix:** `storage`

**Required fields:**
- `bucket` - S3 bucket name (no default)
- `region` - AWS region (default: `us-east-1`)

**Credential precedence:**
1. **Static credentials**: `access_key` + `secret_key` (highest priority)
2. **Profile**: `profile` (shared credentials file)
3. **SDK defaults**: Environment variables, instance profiles, IRSA (when `use_sdk_defaults: true`)
4. **Assumed role**: `role_arn` performs STS AssumeRole using source credentials

```go
type Config struct {
    Provider   string `mapstructure:"provider" default:"s3"`
    Bucket     string `mapstructure:"bucket" validate:"required"`
    Region     string `mapstructure:"region" default:"us-east-1"`
    Endpoint   string `mapstructure:"endpoint"`
    
    // Credentials (in order of precedence)
    AccessKey    string `mapstructure:"access_key"`
    SecretKey    string `mapstructure:"secret_key"`
    SessionToken string `mapstructure:"session_token"`
    Profile      string `mapstructure:"profile"`
    
    // Advanced credential options
    UseSDKDefaults bool   `mapstructure:"use_sdk_defaults" default:"false"`
    RoleARN        string `mapstructure:"role_arn"`
    ExternalID     string `mapstructure:"external_id"`
    
    // S3-specific
    UsePathStyle bool `mapstructure:"use_path_style" default:"false"`
    DisableSSL   bool `mapstructure:"disable_ssl" default:"false"`
    
    // Performance
    RequestTimeout  time.Duration `mapstructure:"request_timeout" default:"30s"`
    MaxRetries      int          `mapstructure:"max_retries" default:"3"`
    BackoffInitial  time.Duration `mapstructure:"backoff_initial" default:"200ms"`
    BackoffMax      time.Duration `mapstructure:"backoff_max" default:"5s"`
    DefaultPartSize int64        `mapstructure:"default_part_size" default:"8388608"` // 8MB
    DefaultParallel int          `mapstructure:"default_parallel" default:"4"`
    
    // Multi-tenant
    BasePrefix string `mapstructure:"base_prefix"`
}

func (Config) Prefix() string { return "storage" }
```

**Environment variable examples:**
```bash
# Required
export STRATUM_STORAGE_BUCKET=my-bucket

# AWS S3 with static credentials
export STRATUM_STORAGE_ACCESS_KEY=AKIA...
export STRATUM_STORAGE_SECRET_KEY=...

# MinIO (local development)
export STRATUM_STORAGE_ENDPOINT=http://localhost:9000
export STRATUM_STORAGE_USE_PATH_STYLE=true
export STRATUM_STORAGE_DISABLE_SSL=true
export STRATUM_STORAGE_ACCESS_KEY=minioadmin
export STRATUM_STORAGE_SECRET_KEY=minioadmin

# Production with SDK defaults (instance profile/IRSA)
export STRATUM_STORAGE_USE_SDK_DEFAULTS=true

# Production with assumed role
export STRATUM_STORAGE_USE_SDK_DEFAULTS=true
export STRATUM_STORAGE_ROLE_ARN=arn:aws:iam::123456789012:role/StorageRole
export STRATUM_STORAGE_EXTERNAL_ID=optional-external-id
```

### 3. Storage Interface

**ALL operations use `context.Context`** - this is non-negotiable.

```go
type Storage interface {
    // Basic operations
    Put(ctx context.Context, key string, r io.Reader, opts *PutOptions) (Stat, error)
    PutBytes(ctx context.Context, key string, data []byte, opts *PutOptions) (Stat, error)
    PutFile(ctx context.Context, key string, path string, opts *PutOptions) (Stat, error)
    
    Get(ctx context.Context, key string) (ReaderAtCloser, Stat, error)
    Head(ctx context.Context, key string) (Stat, error)
    
    List(ctx context.Context, opts ListOptions) (ListPage, error)
    
    Delete(ctx context.Context, key string) error
    DeleteBatch(ctx context.Context, keys []string) ([]string, error)
    
    // Multipart upload (automatic for large files)
    MultipartUpload(ctx context.Context, key string, src io.Reader, 
                    cfg *MultipartConfig, putOpts *PutOptions) (Stat, error)
    CreateMultipart(ctx context.Context, key string, putOpts *PutOptions) (uploadID string, err error)
    UploadPart(ctx context.Context, key, uploadID string, partNumber int32, 
               part io.Reader, size int64) (etag string, err error)
    CompleteMultipart(ctx context.Context, key, uploadID string, etags []string) (Stat, error)
    AbortMultipart(ctx context.Context, key, uploadID string) error
    
    // Presigned URLs
    PresignGet(ctx context.Context, key string, opts *PresignOptions) (url string, err error)
    PresignPut(ctx context.Context, key string, opts *PresignOptions) (url string, err error)
}
```

### 4. Key Building Strategies

StorageX supports multi-tenant architectures through key builders:

```go
// Interface
type KeyBuilder interface {
    BuildKey(originalKey string, context map[string]string) string
    StripKey(fullKey string) string
}

// Built-in implementations
kb := NewPrefixKeyBuilder("org/{org_id}/workspace/{workspace_id}")
kb := NewDatePartitionedKeyBuilder("logs", "2006/01/02", timeFunc)
kb := NewKeyBuilderChain(prefix, datePartition)  // Compose multiple

// Configure via config
export STRATUM_STORAGE_BASE_PREFIX="tenant/{tenant_id}/env/{environment}"
```

**Key transformation example:**
```go
// With BasePrefix = "org/{org_id}/workspace/{workspace_id}"
storage.Put(ctx, "document.pdf", reader, nil)
// Actual S3 key: "org/acme-corp/workspace/proj-1/document.pdf"

storage.Get(ctx, "document.pdf")
// Looks for: "org/acme-corp/workspace/proj-1/document.pdf"
```

## Module-Specific Conventions

### Storage Operations

```go
// Small files (< 5MB) - use PutBytes
stat, err := storage.PutBytes(ctx, "document.pdf", pdfData, &storagex.PutOptions{
    ContentType: "application/pdf",
    Metadata: map[string]string{
        "author": "john.doe",
        "version": "1.0",
    },
})

// Large files (> 5MB) - use MultipartUpload
stat, err := storage.MultipartUpload(ctx, "video.mp4", videoReader, 
    &storagex.MultipartConfig{
        PartSizeBytes: 64 << 20, // 64MB parts
        Concurrency:   8,        // 8 concurrent uploads
    }, 
    &storagex.PutOptions{
        ContentType: "video/mp4",
    })

// Stream download
reader, stat, err := storage.Get(ctx, "large-file.csv")
if err != nil {
    return err
}
defer reader.Close()

// Metadata only (no download)
stat, err := storage.Head(ctx, "document.pdf")

// List with pagination
page, err := storage.List(ctx, storagex.ListOptions{
    Prefix:    "photos/2024/",
    Delimiter: "/",  // Group by "folders"
    PageSize:  100,
})

// Batch delete
failedKeys, err := storage.DeleteBatch(ctx, []string{
    "temp/file1.txt",
    "temp/file2.txt",
})
```

### S3 Adapter Specifics

**Client management:**
- Uses AWS SDK v2 (`github.com/aws/aws-sdk-go-v2`)
- `ClientManager` handles S3 client + presign client lifecycle
- Automatic retries with exponential backoff
- Connection validation on startup (`HeadBucket`)

**Credential resolution:**
```go
// buildAWSConfigWithLoader returns (awsConfig, credSource, error)
// credSource is one of: "static", "profile", "sdk-default", "assumed-role"

// Order of precedence:
1. Static credentials (access_key + secret_key)
2. Profile (shared credentials file)
3. SDK default chain (env vars, instance profile, IRSA)
4. AssumeRole (if role_arn is set)
```

**AssumeRole behavior:**
- If `role_arn` is set, the module performs STS AssumeRole
- Source credentials (from static/profile/SDK) are used to authenticate to STS
- Resulting temporary credentials are used for S3 operations
- `assume_role_validate_credentials` (default: false) controls startup validation

**MinIO compatibility:**
- Set `use_path_style: true` (required)
- Set `disable_ssl: true` (local development)
- Provide explicit `access_key` + `secret_key`
- Endpoint format: `http://localhost:9000`

### Error Handling

StorageX uses typed errors with `errors.Is` support:

```go
stat, err := storage.Get(ctx, "missing-file.txt")
if err != nil {
    switch {
    case errors.Is(err, storagex.ErrNotFound):
        // File doesn't exist
    case errors.Is(err, storagex.ErrTimeout):
        // Request timed out - consider retry
    case errors.Is(err, storagex.ErrTooLarge):
        // File exceeds size limits
    case errors.Is(err, storagex.ErrAborted):
        // Context was cancelled
    default:
        // Unexpected error
    }
}

// Detailed error information
var storageErr *storagex.StorageError
if errors.As(err, &storageErr) {
    log.Printf("Operation: %s, Key: %s, Error: %v", 
        storageErr.Op, storageErr.Key, storageErr.Err)
}
```

**Standard errors:**
- `ErrNotFound` - Object does not exist
- `ErrConflict` - Object already exists (when overwrite=false)
- `ErrInvalidConfig` - Configuration is invalid
- `ErrAborted` - Operation was cancelled
- `ErrTimeout` - Operation timed out
- `ErrTooLarge` - Object exceeds size limits
- `ErrInvalidKey` - Object key is invalid

## Testing Conventions

### Unit Tests

```go
func TestStorageOperations(t *testing.T) {
    app := fxtest.New(t,
        storagex.Module(),
        s3.Module(),
        fx.Invoke(func(storage storagex.Storage) {
            // Test storage operations
            ctx := context.Background()
            stat, err := storage.PutBytes(ctx, "test.txt", []byte("hello"), nil)
            assert.NoError(t, err)
            assert.NotEmpty(t, stat.ETag)
        }),
    )
    defer app.RequireStart().RequireStop()
}
```

### Integration Tests

Use `docker-compose` for MinIO:

```yaml
# tools/docker-compose.yml
services:
  minio:
    image: minio/minio:latest
    command: server /data --console-address ":9001"
    ports:
      - "9000:9000"
      - "9001:9001"
    environment:
      MINIO_ROOT_USER: minioadmin
      MINIO_ROOT_PASSWORD: minioadmin
```

Tag integration tests with build tag:
```go
//go:build integration

func TestS3Integration(t *testing.T) {
    // Requires MinIO running on localhost:9000
}
```

Run with: `go test -tags=integration ./...`

### Test Configuration

```go
// Use storagex.TestModule for pre-configured test setup
app := fxtest.New(t,
    storagex.TestModule,  // Provides test config (MinIO defaults)
    s3.Module(),
    fx.Invoke(testFunc),
)

// Or provide custom test config
testConfig := &storagex.Config{
    Bucket:       "test-bucket",
    Endpoint:     "http://localhost:9000",
    UsePathStyle: true,
    AccessKey:    "minioadmin",
    SecretKey:    "minioadmin",
    DisableSSL:   true,
}

app := fxtest.New(t,
    fx.Supply(testConfig),
    s3.Module(),
    fx.Invoke(testFunc),
)
```

## Common Mistakes to Avoid

1. **❌ Including only base module** - Must include adapter (e.g., `s3.Module()`)
2. **❌ Not using context.Context** - All operations require context
3. **❌ Ignoring error types** - Use `errors.Is` for typed error checking
4. **❌ Not closing readers** - Always `defer reader.Close()` after `Get()`
5. **❌ Hardcoding credentials** - Use environment variables or IAM roles
6. **❌ Wrong endpoint format for MinIO** - Must include `http://` or `https://`
7. **❌ Not setting use_path_style for MinIO** - Required for MinIO compatibility
8. **❌ Using static credentials in production** - Prefer `use_sdk_defaults: true` with IAM roles
9. **❌ Not handling pagination** - Check `page.IsTruncated` and use `NextToken`
10. **❌ Multipart without proper part size** - Default 8MB is good for most cases

## Performance Guidelines

### Multipart Upload Recommendations

| File Size | Part Size | Concurrency |
|-----------|-----------|-------------|
| < 100MB | 8MB (default) | 2-4 |
| 100MB - 1GB | 16-32MB | 4-8 |
| 1GB - 10GB | 64MB | 8-16 |
| > 10GB | 128MB+ | 16-32 |

### Configuration Tuning

```yaml
storage:
  request_timeout: 60s      # Increase for large files
  max_retries: 5           # High reliability environments
  backoff_max: 30s         # Longer backoff for rate limiting
  default_parallel: 16     # High throughput uploads
  default_part_size: 67108864  # 64MB for large files
```

### Connection Pooling

The AWS SDK v2 automatically handles connection pooling. The HTTP client is configured with:
- Timeout: `request_timeout` (default 30s)
- Retry mode: Adaptive
- Max retries: `max_retries` (default 3)

## Deployment Best Practices

### Production AWS S3

```yaml
storage:
  bucket: prod-bucket-name
  region: us-west-2
  use_sdk_defaults: true      # Use IAM instance profile/IRSA
  request_timeout: 60s
  max_retries: 5
  enable_logging: false       # Disable debug logging in prod
```

**Never use static credentials in production containers.** Configure EKS pods with IRSA or EC2 instances with instance profiles.

### Production with AssumeRole

```yaml
storage:
  bucket: prod-bucket-name
  region: us-west-2
  use_sdk_defaults: true
  role_arn: arn:aws:iam::123456789012:role/StorageRole
  external_id: your-external-id
  assume_role_validate_credentials: true  # Fail fast if source creds missing
```

### Local Development (MinIO)

```yaml
storage:
  bucket: dev-bucket
  endpoint: http://localhost:9000
  use_path_style: true
  disable_ssl: true
  access_key: minioadmin
  secret_key: minioadmin
  enable_logging: true        # Debug logging for development
```

## Key Files to Reference

- **Module pattern:** `storagex/fxmodule.go`, `storagex/adapters/s3/module.go`
- **Storage interface:** `storagex/storage.go`
- **Config validation:** `storagex/config.go`, `storagex/validate.go`
- **Client management:** `storagex/adapters/s3/client.go`
- **S3 implementation:** `storagex/adapters/s3/storage.go`
- **Test examples:** `storagex/fxtest_lifecycle_test.go`
- **Integration example:** `storagex/examples/basic/main.go`

## When to Ask the Human

1. **Adding new storage backends** - Azure Blob, Google Cloud Storage
2. **Breaking changes** to `Storage` interface (impacts all consumers)
3. **Credential security** - Exposure of secrets in logs or errors
4. **Performance defaults** - Changes to part sizes, concurrency, timeouts
5. **New features** - Server-side encryption, lifecycle policies, versioning
6. **AWS SDK upgrades** - Major version changes that affect compatibility

## Quick Reference

| Task | Code Pattern |
|------|-------------|
| Create app | `core.New(storagex.Module(), s3.Module(), ...)` |
| Upload small file | `storage.PutBytes(ctx, key, data, &PutOptions{...})` |
| Upload large file | `storage.MultipartUpload(ctx, key, reader, &MultipartConfig{...}, &PutOptions{...})` |
| Download file | `reader, stat, err := storage.Get(ctx, key); defer reader.Close()` |
| Check if exists | `stat, err := storage.Head(ctx, key); if errors.Is(err, ErrNotFound) { ... }` |
| List files | `page, err := storage.List(ctx, ListOptions{Prefix: "path/"})` |
| Delete files | `failedKeys, err := storage.DeleteBatch(ctx, keys)` |
| Presigned URL | `url, err := storage.PresignGet(ctx, key, &PresignOptions{Expiry: time.Hour})` |
| Custom key builder | `storagex.WithKeyBuilder(kb)` in options |
| Test setup | `app := fxtest.New(t, storagex.TestModule, s3.Module(), ...)` |

---

**StorageX Philosophy:** Clean interface, pluggable adapters, production-ready. One storage abstraction for all S3-compatible backends.
