# StorageX

[![Go Version](https://img.shields.io/github/go-mod/go-version/gostratum/storagex)](https://golang.org/)
[![License](https://img.shields.io/badge/license-MIT-blue.svg)](LICENSE)
[![Go Report Card](https://goreportcard.com/badge/github.com/gostratum/storagex)](https://goreportcard.com/report/github.com/gostratum/storagex)

StorageX is a clean, production-ready Go module that provides a **DI-friendly object storage abstraction** with a **first-class S3-compatible implementation**. Built for **Go 1.25** using clean architecture principles, small interfaces, and 100% composable design.

## Features

✨ **Clean Architecture**
- Small, focused interfaces following Go best practices
- Zero global state - fully dependency-injectable
- Context-aware operations with proper cancellation
- Structured error handling with `errors.Is` support

🚀 **Production Ready** 
- Automatic retries with exponential backoff
- Configurable timeouts and circuit breakers  
- Graceful shutdown and lifecycle management
- Comprehensive logging and observability

📦 **S3 Compatible**
- Works with **AWS S3**, **MinIO**, and other S3-compatible services
- Automatic multipart uploads for large files
- Presigned URLs for direct client access
- Path-style and virtual-hosted-style addressing

🏗️ **Multi-Tenant Ready**
- Pluggable key building strategies
- Organizational and workspace prefixes
- Date-based partitioning support
- Custom key transformation chains

⚡ **High Performance**
- Concurrent multipart uploads
- Streaming operations with minimal memory usage
- Connection pooling and reuse
- Efficient batch operations

## Quick Start

### 1. Install

```bash
go get github.com/gostratum/storagex
```

### 2. Start MinIO for Testing

```bash
cd tools
docker-compose up -d minio
```

This starts MinIO on `localhost:9000` with admin/admin credentials and creates test buckets.

### 3. Basic Usage

```go
package main

import (
    "context"
    "log"
    
    "go.uber.org/fx"
    "github.com/gostratum/storagex/pkg/storagex"
    _ "github.com/gostratum/storagex/internal/s3" // Register S3 implementation
)

func main() {
    app := fx.New(
        storagex.Module,
        fx.Invoke(useStorage),
    )
    
    ctx := context.Background()
    if err := app.Start(ctx); err != nil {
        log.Fatal(err)
    }
    defer app.Stop(ctx)
}

func useStorage(storage storagex.Storage) {
    ctx := context.Background()
    
    // Store a file
    stat, err := storage.PutBytes(ctx, "hello.txt", []byte("Hello, StorageX!"), nil)
    if err != nil {
        log.Fatal(err)
    }
    
    // Retrieve the file
    reader, _, err := storage.Get(ctx, "hello.txt")
    if err != nil {
        log.Fatal(err) 
    }
    defer reader.Close()
    
    // List objects
    page, err := storage.List(ctx, storagex.ListOptions{
        Prefix:   "hello",
        PageSize: 10,
    })
    if err != nil {
        log.Fatal(err)
    }
    
    log.Printf("Found %d objects", len(page.Keys))
}
```

## Configuration

StorageX supports configuration via **environment variables**, **YAML files**, and **Viper**.

### Environment Variables

```bash
# S3 Configuration
export STORAGEX_PROVIDER=s3
export STORAGEX_BUCKET=my-bucket
export STORAGEX_REGION=us-east-1
export STORAGEX_ACCESS_KEY=your-access-key
export STORAGEX_SECRET_KEY=your-secret-key

# MinIO Configuration  
export STORAGEX_ENDPOINT=http://localhost:9000
export STORAGEX_USE_PATH_STYLE=true
export STORAGEX_DISABLE_SSL=true

# Performance Tuning
export STORAGEX_REQUEST_TIMEOUT=30s
export STORAGEX_MAX_RETRIES=3
export STORAGEX_DEFAULT_PART_SIZE=8388608  # 8MB
export STORAGEX_DEFAULT_PARALLEL=4

# Multi-tenant Support
export STORAGEX_BASE_PREFIX="org/{org_id}/workspace/{workspace_id}"

# Logging
export STORAGEX_ENABLE_LOGGING=true
```

### YAML Configuration

Create `config.yaml`:

```yaml
storagex:
  provider: s3
  bucket: my-bucket
  region: us-east-1
  endpoint: http://localhost:9000  # For MinIO
  use_path_style: true
  access_key: minioadmin
  secret_key: minioadmin
  request_timeout: 30s
  max_retries: 3
  default_part_size: 8388608  # 8MB
  default_parallel: 4
  base_prefix: "tenant/{tenant_id}"
  disable_ssl: true
  enable_logging: true
```

### Configuration Reference

| Setting | Environment Variable | Default | Description |
|---------|---------------------|---------|-------------|
| `provider` | `STORAGEX_PROVIDER` | `"s3"` | Storage backend (`s3` only) |
| `bucket` | `STORAGEX_BUCKET` | **required** | S3 bucket name |
| `region` | `STORAGEX_REGION` | `"us-east-1"` | AWS region |
| `endpoint` | `STORAGEX_ENDPOINT` | `""` | Custom endpoint (MinIO/etc) |
| `use_path_style` | `STORAGEX_USE_PATH_STYLE` | `false` | Path-style URLs (true for MinIO) |
| `access_key` | `STORAGEX_ACCESS_KEY` | **required** | AWS access key |
| `secret_key` | `STORAGEX_SECRET_KEY` | **required** | AWS secret key |
| `session_token` | `STORAGEX_SESSION_TOKEN` | `""` | AWS session token (STS) |
| `request_timeout` | `STORAGEX_REQUEST_TIMEOUT` | `30s` | Request timeout |
| `max_retries` | `STORAGEX_MAX_RETRIES` | `3` | Maximum retry attempts |
| `backoff_initial` | `STORAGEX_BACKOFF_INITIAL` | `200ms` | Initial retry delay |
| `backoff_max` | `STORAGEX_BACKOFF_MAX` | `5s` | Maximum retry delay |
| `default_part_size` | `STORAGEX_DEFAULT_PART_SIZE` | `8388608` | Multipart size (bytes) |
| `default_parallel` | `STORAGEX_DEFAULT_PARALLEL` | `4` | Multipart concurrency |
| `base_prefix` | `STORAGEX_BASE_PREFIX` | `""` | Multi-tenant prefix template |
| `disable_ssl` | `STORAGEX_DISABLE_SSL` | `false` | Disable SSL (local only) |
| `enable_logging` | `STORAGEX_ENABLE_LOGGING` | `false` | Enable debug logging |

## API Reference

### Core Interface

The main `Storage` interface provides all object storage operations:

```go
type Storage interface {
    // Basic Operations
    Put(ctx context.Context, key string, r io.Reader, opts *PutOptions) (Stat, error)
    PutBytes(ctx context.Context, key string, data []byte, opts *PutOptions) (Stat, error)
    PutFile(ctx context.Context, key string, path string, opts *PutOptions) (Stat, error)
    
    Get(ctx context.Context, key string) (ReaderAtCloser, Stat, error)
    Head(ctx context.Context, key string) (Stat, error)
    
    List(ctx context.Context, opts ListOptions) (ListPage, error)
    
    Delete(ctx context.Context, key string) error
    DeleteBatch(ctx context.Context, keys []string) ([]string, error)
    
    // Multipart Upload
    MultipartUpload(ctx context.Context, key string, src io.Reader, cfg *MultipartConfig, putOpts *PutOptions) (Stat, error)
    CreateMultipart(ctx context.Context, key string, putOpts *PutOptions) (uploadID string, err error)
    UploadPart(ctx context.Context, key, uploadID string, partNumber int32, part io.Reader, size int64) (etag string, err error)
    CompleteMultipart(ctx context.Context, key, uploadID string, etags []string) (Stat, error)
    AbortMultipart(ctx context.Context, key, uploadID string) error
    
    // Presigned URLs
    PresignGet(ctx context.Context, key string, opts *PresignOptions) (url string, err error)
    PresignPut(ctx context.Context, key string, opts *PresignOptions) (url string, err error)
}
```

### Operations

#### Upload Files

```go
// Small files
stat, err := storage.PutBytes(ctx, "document.pdf", pdfData, &storagex.PutOptions{
    ContentType: "application/pdf",
    Metadata: map[string]string{
        "author": "john.doe",
        "version": "1.0",
    },
})

// Large files (automatic multipart)
stat, err := storage.MultipartUpload(ctx, "video.mp4", videoReader, 
    &storagex.MultipartConfig{
        PartSizeBytes: 64 << 20, // 64MB parts
        Concurrency:   8,        // 8 concurrent uploads
    }, 
    &storagex.PutOptions{
        ContentType: "video/mp4",
    })

// From local file
stat, err := storage.PutFile(ctx, "backup.zip", "/path/to/backup.zip", nil)
```

#### Download Files

```go
// Stream download
reader, stat, err := storage.Get(ctx, "large-dataset.csv")
if err != nil {
    return err
}
defer reader.Close()

// Process streaming data
scanner := bufio.NewScanner(reader)
for scanner.Scan() {
    processLine(scanner.Text())
}

// Get metadata only
stat, err := storage.Head(ctx, "document.pdf")
fmt.Printf("Size: %d bytes, Modified: %s\n", stat.Size, stat.LastModified)
```

#### List Objects

```go
// List with prefix
page, err := storage.List(ctx, storagex.ListOptions{
    Prefix:    "photos/2024/",
    Delimiter: "/",           // Group by "folders"
    PageSize:  100,
})

for _, obj := range page.Keys {
    fmt.Printf("%s (%d bytes)\n", obj.Key, obj.Size)
}

// Handle pagination
for page.IsTruncated {
    page, err = storage.List(ctx, storagex.ListOptions{
        Prefix:            "photos/2024/", 
        ContinuationToken: page.NextToken,
    })
    // Process page...
}
```

#### Batch Operations

```go
// Delete multiple files
failedKeys, err := storage.DeleteBatch(ctx, []string{
    "temp/file1.txt",
    "temp/file2.txt", 
    "temp/file3.txt",
})

if len(failedKeys) > 0 {
    log.Printf("Failed to delete: %v", failedKeys)
}
```

#### Presigned URLs

```go
// Generate upload URL for client
uploadURL, err := storage.PresignPut(ctx, "user-upload.jpg", &storagex.PresignOptions{
    Expiry:      time.Hour,
    ContentType: "image/jpeg",
    ContentLengthRange: [2]int64{100, 10 << 20}, // 100B - 10MB
})

// Generate download URL
downloadURL, err := storage.PresignGet(ctx, "shared-document.pdf", &storagex.PresignOptions{
    Expiry: 24 * time.Hour,
})
```

## Multi-Tenant Support

StorageX supports multi-tenant architectures through pluggable key builders:

```go
// Organization-based prefixes
keyBuilder := storagex.NewPrefixKeyBuilder("org/{org_id}/workspace/{workspace_id}")

storage := storagex.NewStorage(ctx, config, storagex.WithKeyBuilder(keyBuilder))

// Keys are automatically prefixed
storage.Put(ctx, "document.pdf", reader, nil)
// Actual S3 key: "org/acme-corp/workspace/proj-1/document.pdf"
```

### Key Builder Types

```go
// Simple prefix
kb := storagex.NewPrefixKeyBuilder("tenant-123")

// Template-based
kb := storagex.NewPrefixKeyBuilder("org/{org_id}/env/{environment}")

// Date partitioned  
kb := storagex.NewDatePartitionedKeyBuilder("logs", "2006/01/02", 
    func() string { return time.Now().Format("2006/01/02") })

// Custom chain
kb := storagex.NewKeyBuilderChain(
    storagex.NewPrefixKeyBuilder("company/{org_id}"),
    storagex.NewDatePartitionedKeyBuilder("", "2006/01", timeFunc),
)
```

## Error Handling

StorageX uses typed errors with `errors.Is` support:

```go
stat, err := storage.Get(ctx, "missing-file.txt")
if err != nil {
    switch {
    case errors.Is(err, storagex.ErrNotFound):
        log.Println("File not found")
    case errors.Is(err, storagex.ErrTimeout):
        log.Println("Request timed out - retry") 
    case errors.Is(err, storagex.ErrTooLarge):
        log.Println("File too large")
    default:
        log.Printf("Unexpected error: %v", err)
    }
}

// Detailed error information
var storageErr *storagex.StorageError
if errors.As(err, &storageErr) {
    log.Printf("Operation: %s, Key: %s, Error: %v", 
        storageErr.Op, storageErr.Key, storageErr.Err)
}
```

### Error Types

- `ErrNotFound` - Object does not exist
- `ErrConflict` - Object already exists (when overwrite=false)  
- `ErrInvalidConfig` - Configuration is invalid
- `ErrAborted` - Operation was cancelled  
- `ErrTimeout` - Operation timed out
- `ErrTooLarge` - Object exceeds size limits
- `ErrInvalidKey` - Object key is invalid

## Testing

### Unit Tests

```bash
make test-unit
```

### Integration Tests with MinIO

```bash
# Start MinIO
make up

# Run integration tests
make test-integration

# Clean up
make down
```

### Manual Testing

```bash
# Start MinIO
make up

# Run example
make run-example

# View logs
make logs
```

## Performance Guidelines

### Multipart Upload Recommendations

| File Size | Part Size | Concurrency |
|-----------|-----------|-------------|
| < 100MB | 8MB | 2-4 |
| 100MB - 1GB | 16-32MB | 4-8 |
| 1GB - 10GB | 64MB | 8-16 |
| > 10GB | 128MB+ | 16-32 |

### Connection Tuning

```yaml
storagex:
  request_timeout: 60s      # For large files
  max_retries: 5           # High reliability
  backoff_max: 30s         # Longer backoff
  default_parallel: 16     # High throughput
```

## AWS vs MinIO Setup

### AWS S3 Production

```bash
export STORAGEX_PROVIDER=s3
export STORAGEX_REGION=us-west-2
export STORAGEX_BUCKET=my-prod-bucket
export STORAGEX_ACCESS_KEY=AKIA...
export STORAGEX_SECRET_KEY=...
# Use IAM roles in production instead of keys
```

### MinIO Development

```bash  
export STORAGEX_ENDPOINT=http://localhost:9000
export STORAGEX_USE_PATH_STYLE=true
export STORAGEX_ACCESS_KEY=minioadmin
export STORAGEX_SECRET_KEY=minioadmin
export STORAGEX_DISABLE_SSL=true
export STORAGEX_BUCKET=dev-bucket
```

### Other S3-Compatible Services

StorageX works with any S3-compatible service:

- **DigitalOcean Spaces**: Set endpoint to your region
- **Wasabi**: Use wasabi endpoints  
- **Backblaze B2**: Via S3-compatible API
- **Google Cloud Storage**: Via S3-compatible API

## Advanced Usage

### Custom Dependency Injection

```go
import "github.com/spf13/viper"

func main() {
    v := viper.New()
    v.Set("storagex.bucket", "custom-bucket")
    
    app := fx.New(
        storagex.Module,
        storagex.ConfigFromViper(v),
        storagex.WithCustomLogger(customLogger),
        fx.Invoke(useStorage),
    )
}
```

### Custom Key Builder

```go
type MyKeyBuilder struct{}

func (kb *MyKeyBuilder) BuildKey(originalKey string, context map[string]string) string {
    userID := context["user_id"]
    return fmt.Sprintf("users/%s/files/%s", userID, originalKey)
}

func (kb *MyKeyBuilder) StripKey(fullKey string) string {
    parts := strings.Split(fullKey, "/")
    if len(parts) >= 3 {
        return strings.Join(parts[3:], "/")
    }
    return fullKey
}
```

### Lifecycle Management

```go
app := fx.New(
    storagex.NewModuleWithOptions(storagex.ModuleOptions{
        DisableLifecycle: false, // Enable graceful shutdown
        CustomProviders: []fx.Option{
            fx.Provide(myCustomLogger),
        },
    }),
    fx.Invoke(useStorage),
)
```

## Contributing

1. **Fork & Clone**
   ```bash
   git clone https://github.com/gostratum/storagex
   cd storagex
   ```

2. **Install Dependencies**
   ```bash
   make deps
   ```

3. **Start Development Services**
   ```bash
   make up
   ```

4. **Run Tests**
   ```bash
   make test
   ```

5. **Submit PR**
   - Add tests for new features
   - Ensure `make lint` passes
   - Update documentation

## Roadmap

- [ ] **Additional Backends**: Azure Blob, Google Cloud Storage
- [ ] **Advanced Features**: Server-side encryption, lifecycle policies
- [ ] **Observability**: Metrics, distributed tracing
- [ ] **CLI Tool**: Command-line interface for operations
- [ ] **FUSE Interface**: Mount as filesystem

## License

MIT License - see [LICENSE](LICENSE) for details.

## Support

- **GitHub Issues**: Bug reports and feature requests
- **Discussions**: Questions and community support  
- **Documentation**: Full API docs at [pkg.go.dev](https://pkg.go.dev/github.com/gostratum/storagex)

---

**StorageX** - Production-ready object storage for Go applications 🚀