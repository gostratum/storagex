//go:build integration
// +build integration

package test

import (
	"bytes"
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	_ "github.com/gostratum/storagex/internal/s3" // Register S3 implementation
	"github.com/gostratum/storagex/pkg/storagex"
)

func TestS3Integration(t *testing.T) {
	if os.Getenv("RUN_INTEGRATION_TESTS") != "true" {
		t.Skip("Skipping integration tests - set RUN_INTEGRATION_TESTS=true to run")
	}

	// Build config directly from environment variables
	// This is a standalone test, not using FX, so we construct config manually
	cfg := &storagex.Config{
		Provider:       getEnvOrDefault("STRATUM_STORAGE_PROVIDER", "s3"),
		Bucket:         getEnvOrDefault("STRATUM_STORAGE_BUCKET", "test-bucket"),
		Region:         getEnvOrDefault("STRATUM_STORAGE_REGION", "us-east-1"),
		Endpoint:       getEnvOrDefault("STRATUM_STORAGE_ENDPOINT", ""),
		AccessKey:      getEnvOrDefault("STRATUM_STORAGE_ACCESS_KEY", ""),
		SecretKey:      getEnvOrDefault("STRATUM_STORAGE_SECRET_KEY", ""),
		UsePathStyle:   getEnvOrDefault("STRATUM_STORAGE_USE_PATH_STYLE", "false") == "true",
		DisableSSL:     getEnvOrDefault("STRATUM_STORAGE_DISABLE_SSL", "false") == "true",
		EnableLogging:  getEnvOrDefault("STRATUM_STORAGE_ENABLE_LOGGING", "false") == "true",
		RequestTimeout: 30 * time.Second,
		MaxRetries:     3,
	}

	// Validate config
	err := storagex.ValidateConfig(cfg)
	require.NoError(t, err, "Config should be valid")

	// Create storage instance
	ctx := context.Background()
	storage, err := storagex.NewStorageFromConfig(ctx, cfg)
	require.NoError(t, err, "Should create storage successfully")

	t.Run("BasicOperations", func(t *testing.T) {
		testBasicOperations(t, storage)
	})

	t.Run("LargeFileUpload", func(t *testing.T) {
		testLargeFileUpload(t, storage)
	})

	t.Run("ListOperations", func(t *testing.T) {
		testListOperations(t, storage)
	})

	t.Run("PresignedURLs", func(t *testing.T) {
		testPresignedURLs(t, storage)
	})

	t.Run("ErrorHandling", func(t *testing.T) {
		testErrorHandling(t, storage)
	})
}

func testBasicOperations(t *testing.T, storage storagex.Storage) {
	ctx := context.Background()
	testKey := "integration-test/basic-file.txt"
	testContent := []byte("Hello from StorageX integration test!")

	// Test Put
	stat, err := storage.PutBytes(ctx, testKey, testContent, nil)
	require.NoError(t, err, "Should upload file successfully")
	assert.Equal(t, int64(len(testContent)), stat.Size)
	assert.Equal(t, testKey, stat.Key)

	// Test Head (existence check)
	headStat, err := storage.Head(ctx, testKey)
	require.NoError(t, err, "Should get file metadata")
	assert.Equal(t, stat.Size, headStat.Size)
	assert.Equal(t, stat.ETag, headStat.ETag)

	// Test Get
	reader, getStat, err := storage.Get(ctx, testKey)
	require.NoError(t, err, "Should download file successfully")
	defer reader.Close()

	assert.Equal(t, stat.Size, getStat.Size)

	// Read content
	buf := make([]byte, len(testContent))
	n, err := reader.Read(buf)
	require.NoError(t, err)
	assert.Equal(t, len(testContent), n)
	assert.Equal(t, testContent, buf[:n])

	// Test Delete
	err = storage.Delete(ctx, testKey)
	require.NoError(t, err, "Should delete file successfully")

	// Verify deletion
	_, err = storage.Head(ctx, testKey)
	assert.Error(t, err, "File should not exist after deletion")
	assert.True(t, storagex.IsNotFound(err), "Should be NotFound error")
}

func testLargeFileUpload(t *testing.T, storage storagex.Storage) {
	ctx := context.Background()
	testKey := "integration-test/large-file.bin"

	// Create 10MB of test data
	largeData := make([]byte, 10*1024*1024)
	for i := range largeData {
		largeData[i] = byte(i % 256)
	}

	// Test multipart upload
	stat, err := storage.MultipartUpload(ctx, testKey, bytes.NewReader(largeData),
		&storagex.MultipartConfig{
			PartSizeBytes: 5 * 1024 * 1024, // 5MB parts
			Concurrency:   2,
		}, nil)
	require.NoError(t, err, "Should upload large file successfully")
	assert.Equal(t, int64(len(largeData)), stat.Size)

	// Verify by downloading
	reader, _, err := storage.Get(ctx, testKey)
	require.NoError(t, err, "Should download large file successfully")
	defer reader.Close()

	// Read and verify first 1KB
	buf := make([]byte, 1024)
	n, err := reader.Read(buf)
	require.NoError(t, err)
	assert.Equal(t, 1024, n)
	assert.Equal(t, largeData[:1024], buf)

	// Cleanup
	err = storage.Delete(ctx, testKey)
	require.NoError(t, err, "Should delete large file successfully")
}

func testListOperations(t *testing.T, storage storagex.Storage) {
	ctx := context.Background()
	prefix := "integration-test/list/"

	// Upload several test files
	testFiles := []string{"file1.txt", "file2.txt", "subdir/file3.txt"}
	for _, filename := range testFiles {
		key := prefix + filename
		content := []byte("test content for " + filename)
		_, err := storage.PutBytes(ctx, key, content, nil)
		require.NoError(t, err, "Should upload test file: %s", filename)
	}

	// List files with prefix
	page, err := storage.List(ctx, storagex.ListOptions{
		Prefix:   prefix,
		PageSize: 10,
	})
	require.NoError(t, err, "Should list files successfully")
	assert.GreaterOrEqual(t, len(page.Keys), len(testFiles), "Should find uploaded files")

	// Verify files are in the list
	foundFiles := make(map[string]bool)
	for _, obj := range page.Keys {
		if strings.HasPrefix(obj.Key, prefix) {
			foundFiles[strings.TrimPrefix(obj.Key, prefix)] = true
		}
	}

	for _, filename := range testFiles {
		assert.True(t, foundFiles[filename], "Should find file: %s", filename)
	}

	// Cleanup
	keys := make([]string, len(testFiles))
	for i, filename := range testFiles {
		keys[i] = prefix + filename
	}
	failedKeys, err := storage.DeleteBatch(ctx, keys)
	require.NoError(t, err, "Should delete test files successfully")
	assert.Empty(t, failedKeys, "All deletes should succeed")
}

func testPresignedURLs(t *testing.T, storage storagex.Storage) {
	ctx := context.Background()
	testKey := "integration-test/presigned-test.txt"

	// Test presigned PUT URL
	putURL, err := storage.PresignPut(ctx, testKey, &storagex.PresignOptions{
		Expiry:      time.Hour,
		ContentType: "text/plain",
	})
	require.NoError(t, err, "Should generate presigned PUT URL")
	assert.Contains(t, putURL, testKey, "URL should contain the key")

	// Upload some content first for GET test
	testContent := []byte("content for presigned GET test")
	_, err = storage.PutBytes(ctx, testKey, testContent, nil)
	require.NoError(t, err, "Should upload test content")

	// Test presigned GET URL
	getURL, err := storage.PresignGet(ctx, testKey, &storagex.PresignOptions{
		Expiry: time.Hour,
	})
	require.NoError(t, err, "Should generate presigned GET URL")
	assert.Contains(t, getURL, testKey, "URL should contain the key")

	// Cleanup
	err = storage.Delete(ctx, testKey)
	require.NoError(t, err, "Should delete test file")
}

func testErrorHandling(t *testing.T, storage storagex.Storage) {
	ctx := context.Background()
	nonExistentKey := "integration-test/does-not-exist.txt"

	// Test getting non-existent file
	_, _, err := storage.Get(ctx, nonExistentKey)
	require.Error(t, err, "Should return error for non-existent file")
	assert.True(t, storagex.IsNotFound(err), "Should be NotFound error")

	// Test head on non-existent file
	_, err = storage.Head(ctx, nonExistentKey)
	require.Error(t, err, "Should return error for non-existent file")
	assert.True(t, storagex.IsNotFound(err), "Should be NotFound error")

	// Test delete non-existent file (should not error)
	err = storage.Delete(ctx, nonExistentKey)
	assert.NoError(t, err, "Delete of non-existent file should not error")
}

// Helper function to get environment variable with default
func getEnvOrDefault(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

// Additional helper for creating storage from config
func init() {
	// Register a helper function to create storage from config
	// This avoids the dependency injection complexity for tests
}
