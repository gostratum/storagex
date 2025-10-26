# StorageX Testing Utilities

This package provides testing utilities for StorageX. For most S3 testing needs, **we recommend using [gofakes3](https://github.com/johannesboyne/gofakes3)** instead of the custom mock.

## Recommended: gofakes3

gofakes3 is a production-ready, in-memory S3 server that's fully compatible with AWS SDK v2:

```go
import (
    "github.com/johannesboyne/gofakes3"
    "github.com/johannesboyne/gofakes3/backend/s3mem"
    "net/http/httptest"
)

func TestWithFakeS3(t *testing.T) {
    // Create in-memory S3 server
    backend := s3mem.New()
    faker := gofakes3.New(backend)
    ts := httptest.NewServer(faker.Server())
    defer ts.Close()
    
    // Create bucket
    backend.CreateBucket("test-bucket")
    
    // Configure storage to use fake server
    cfg := &storagex.Config{
        Bucket:       "test-bucket",
        Endpoint:     ts.URL,
        AccessKey:    "TEST",
        SecretKey:    "TEST",
        UsePathStyle: true,
    }
    
    storage, _ := s3.NewS3Storage(context.Background(), cfg)
    // Use storage like real S3!
}
```

**Advantages of gofakes3:**
- ✅ Full S3 protocol implementation
- ✅ Works with real AWS SDK
- ✅ Battle-tested in production
- ✅ Catches S3-specific edge cases
- ✅ No mocking needed

See `/Users/danecao/source/gostratum/storagex/test/integration_test.go` for a complete example.

## Alternative: Custom MockStorage

For simple interface-level testing without HTTP overhead:

## Usage

```go
import (
    "context"
    "testing"
    
    "github.com/gostratum/storagex/internal/testutil"
    "github.com/stretchr/testify/assert"
    "github.com/stretchr/testify/require"
)

func TestYourCode(t *testing.T) {
    // Create mock storage
    storage := testutil.NewMockStorage()
    
    // Use it like any other storagex.Storage
    ctx := context.Background()
    
    // Put an object
    stat, err := storage.PutBytes(ctx, "test/file.txt", []byte("hello world"), nil)
    require.NoError(t, err)
    assert.Equal(t, int64(11), stat.Size)
    
    // Get the object
    reader, stat, err := storage.Get(ctx, "test/file.txt")
    require.NoError(t, err)
    defer reader.Close()
    
    // Read content
    data, err := io.ReadAll(reader)
    require.NoError(t, err)
    assert.Equal(t, "hello world", string(data))
}
```

## Features

The mock storage implements all `storagex.Storage` interface methods:

- ✅ **Put/PutBytes**: Store objects in memory
- ✅ **Get**: Retrieve objects with metadata
- ✅ **Head**: Get object metadata without content
- ✅ **List**: List objects with prefix/delimiter filtering and pagination
- ✅ **Delete/DeleteBatch**: Delete objects (idempotent like S3)
- ✅ **MultipartUpload**: Simplified multipart upload (stores as regular object)
- ✅ **PresignGet/PresignPut**: Returns mock URLs
- ✅ **Thread-safe**: All operations use read/write locks

## Testing Against Real S3

The integration tests in `storagex/test/integration_test.go` use mock storage by default for fast, isolated unit tests. To run tests against real S3/MinIO:

```bash
# Set environment variable to use real S3
export STORAGEX_USE_REAL_S3=true

# Configure S3 credentials
export STRATUM_STORAGE_BUCKET=test-bucket
export STRATUM_STORAGE_REGION=us-east-1
export STRATUM_STORAGE_ACCESS_KEY=your-access-key
export STRATUM_STORAGE_SECRET_KEY=your-secret-key

# For MinIO
export STRATUM_STORAGE_ENDPOINT=http://localhost:9000
export STRATUM_STORAGE_USE_PATH_STYLE=true
export STRATUM_STORAGE_DISABLE_SSL=true

# Run tests
go test ./test/...
```

## Design Philosophy

Following GoStratum's testing philosophy:

1. **Test behavior, not internals** - Mock implements the public Storage interface
2. **Fakes at boundaries** - Mock replaces network/S3 dependency
3. **No external dependencies** - Tests run fast without Docker/MinIO
4. **Concurrency-safe** - Uses proper locking for thread safety

## Limitations

The mock storage is intentionally simplified:

- **PutFile**: Not implemented (returns error)
- **ETag generation**: Simple checksum-based, not MD5 like real S3
- **Multipart uploads**: Simplified to regular Put operations
- **Presigned URLs**: Returns mock URLs that can't actually be used
- **Storage in memory**: All data is lost when test completes

For comprehensive S3-specific behavior testing (e.g., actual presigned URL uploads, large file multipart uploads), use the real S3/MinIO tests by setting `STORAGEX_USE_REAL_S3=true`.
