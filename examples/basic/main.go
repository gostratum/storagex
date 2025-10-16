package main

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log"
	"time"

	"go.uber.org/fx"
	"go.uber.org/zap"

	"github.com/gostratum/storagex/pkg/storagex"

	// Import S3 implementation to register it
	_ "github.com/gostratum/storagex/internal/s3"
)

func main() {
	// Create the Fx application
	app := fx.New(
		storagex.Module,
		fx.Invoke(runDemo),
	)

	// Start the application
	ctx := context.Background()
	if err := app.Start(ctx); err != nil {
		log.Fatal("Failed to start application:", err)
	}

	// Graceful shutdown
	defer func() {
		if err := app.Stop(ctx); err != nil {
			log.Println("Error stopping application:", err)
		}
	}()

	// Keep the application running for a bit to see the demo
	time.Sleep(2 * time.Second)
}

// runDemo demonstrates all storage operations
func runDemo(storage storagex.Storage, logger storagex.Logger) {
	ctx := context.Background()

	logger.Info("Starting StorageX demo")

	// Demonstrate basic operations
	if err := demonstrateBasicOps(ctx, storage, logger); err != nil {
		logger.Error("Basic operations demo failed", zap.Error(err))
		return
	}

	// Demonstrate multipart upload
	if err := demonstrateMultipartUpload(ctx, storage, logger); err != nil {
		logger.Error("Multipart upload demo failed", zap.Error(err))
		return
	}

	// Demonstrate presigned URLs
	if err := demonstratePresignedURLs(ctx, storage, logger); err != nil {
		logger.Error("Presigned URLs demo failed", zap.Error(err))
		return
	}

	// Demonstrate batch operations
	if err := demonstrateBatchOps(ctx, storage, logger); err != nil {
		logger.Error("Batch operations demo failed", zap.Error(err))
		return
	}

	logger.Info("StorageX demo completed successfully")
}

// demonstrateBasicOps shows basic CRUD operations
func demonstrateBasicOps(ctx context.Context, storage storagex.Storage, logger storagex.Logger) error {
	logger.Info("=== Basic Operations Demo ===")

	// Test data
	testKey := "demo/hello.txt"
	testData := []byte("Hello, StorageX! This is a test file.")

	// 1. Put object
	logger.Info("Putting object", zap.String("key", testKey))
	stat, err := storage.PutBytes(ctx, testKey, testData, &storagex.PutOptions{
		ContentType: "text/plain",
		Metadata: map[string]string{
			"demo":      "basic-ops",
			"timestamp": time.Now().Format(time.RFC3339),
		},
		Overwrite: true,
	})
	if err != nil {
		return fmt.Errorf("failed to put object: %w", err)
	}
	logger.Info("Object put successfully",
		zap.String("key", stat.Key),
		zap.Int64("size", stat.Size),
		zap.String("etag", stat.ETag))

	// 2. Head object (get metadata only)
	logger.Info("Getting object metadata", zap.String("key", testKey))
	headStat, err := storage.Head(ctx, testKey)
	if err != nil {
		return fmt.Errorf("failed to head object: %w", err)
	}
	logger.Info("Object metadata retrieved",
		zap.String("key", headStat.Key),
		zap.Int64("size", headStat.Size),
		zap.String("content_type", headStat.ContentType),
		zap.Any("metadata", headStat.Metadata))

	// 3. Get object
	logger.Info("Getting object content", zap.String("key", testKey))
	reader, getStat, err := storage.Get(ctx, testKey)
	if err != nil {
		return fmt.Errorf("failed to get object: %w", err)
	}
	defer reader.Close()

	// Read the content
	content, err := io.ReadAll(reader)
	if err != nil {
		return fmt.Errorf("failed to read object content: %w", err)
	}

	logger.Info("Object content retrieved",
		zap.String("key", getStat.Key),
		zap.Int("content_length", len(content)),
		zap.String("content_preview", string(content[:min(50, len(content))])))

	// 4. List objects
	logger.Info("Listing objects with prefix", zap.String("prefix", "demo/"))
	page, err := storage.List(ctx, storagex.ListOptions{
		Prefix:   "demo/",
		PageSize: 10,
	})
	if err != nil {
		return fmt.Errorf("failed to list objects: %w", err)
	}

	logger.Info("Objects listed",
		zap.Int("count", len(page.Keys)),
		zap.Bool("truncated", page.IsTruncated))

	for i, obj := range page.Keys {
		logger.Info("Listed object",
			zap.Int("index", i),
			zap.String("key", obj.Key),
			zap.Int64("size", obj.Size),
			zap.Time("modified", obj.LastModified))
	}

	return nil
}

// demonstrateMultipartUpload shows large file upload
func demonstrateMultipartUpload(ctx context.Context, storage storagex.Storage, logger storagex.Logger) error {
	logger.Info("=== Multipart Upload Demo ===")

	// Create a large test file in memory (10MB)
	testKey := "demo/large-file.bin"
	largeData := make([]byte, 10*1024*1024) // 10MB

	// Fill with some pattern
	for i := range largeData {
		largeData[i] = byte(i % 256)
	}

	logger.Info("Starting multipart upload",
		zap.String("key", testKey),
		zap.Int("size_mb", len(largeData)/(1024*1024)))

	// Configure multipart upload
	multipartConfig := &storagex.MultipartConfig{
		PartSizeBytes: 5 * 1024 * 1024, // 5MB parts
		Concurrency:   2,               // 2 concurrent uploads
	}

	putOptions := &storagex.PutOptions{
		ContentType: "application/octet-stream",
		Metadata: map[string]string{
			"demo":    "multipart-upload",
			"size":    fmt.Sprintf("%d", len(largeData)),
			"pattern": "sequential-bytes",
		},
	}

	// Perform multipart upload
	start := time.Now()
	stat, err := storage.MultipartUpload(ctx, testKey, bytes.NewReader(largeData), multipartConfig, putOptions)
	if err != nil {
		return fmt.Errorf("multipart upload failed: %w", err)
	}

	duration := time.Since(start)
	logger.Info("Multipart upload completed",
		zap.String("key", stat.Key),
		zap.Int64("size", stat.Size),
		zap.String("etag", stat.ETag),
		zap.Duration("duration", duration),
		zap.Float64("throughput_mb_s", float64(stat.Size)/(1024*1024)/duration.Seconds()))

	// Verify the upload by getting metadata
	headStat, err := storage.Head(ctx, testKey)
	if err != nil {
		return fmt.Errorf("failed to verify uploaded file: %w", err)
	}

	logger.Info("Multipart upload verified",
		zap.Int64("uploaded_size", stat.Size),
		zap.Int64("verified_size", headStat.Size),
		zap.Bool("sizes_match", stat.Size == headStat.Size))

	return nil
}

// demonstratePresignedURLs shows presigned URL generation
func demonstratePresignedURLs(ctx context.Context, storage storagex.Storage, logger storagex.Logger) error {
	logger.Info("=== Presigned URLs Demo ===")

	testKey := "demo/presigned-test.txt"

	// 1. Generate presigned PUT URL
	logger.Info("Generating presigned PUT URL", zap.String("key", testKey))
	putURL, err := storage.PresignPut(ctx, testKey, &storagex.PresignOptions{
		Expiry:      15 * time.Minute,
		ContentType: "text/plain",
		Metadata: map[string]string{
			"uploaded_via": "presigned_url",
			"demo":         "presigned-put",
		},
	})
	if err != nil {
		return fmt.Errorf("failed to generate presigned PUT URL: %w", err)
	}

	logger.Info("Presigned PUT URL generated",
		zap.String("url", putURL[:min(100, len(putURL))]+"..."),
		zap.Int("url_length", len(putURL)))

	// 2. Upload some content first so we can generate a GET URL
	testContent := []byte("This file was uploaded to test presigned URLs!")
	_, err = storage.PutBytes(ctx, testKey, testContent, &storagex.PutOptions{
		ContentType: "text/plain",
		Metadata: map[string]string{
			"demo": "presigned-demo",
		},
	})
	if err != nil {
		return fmt.Errorf("failed to upload test file for presigned GET: %w", err)
	}

	// 3. Generate presigned GET URL
	logger.Info("Generating presigned GET URL", zap.String("key", testKey))
	getURL, err := storage.PresignGet(ctx, testKey, &storagex.PresignOptions{
		Expiry:      15 * time.Minute,
		ContentType: "text/plain",
	})
	if err != nil {
		return fmt.Errorf("failed to generate presigned GET URL: %w", err)
	}

	logger.Info("Presigned GET URL generated",
		zap.String("url", getURL[:min(100, len(getURL))]+"..."),
		zap.Int("url_length", len(getURL)))

	// Note: In a real application, these URLs would be used by clients to upload/download directly
	logger.Info("Presigned URLs can be used for direct client access",
		zap.String("put_curl_example", fmt.Sprintf(`curl -X PUT -H "Content-Type: text/plain" --data "Hello World" "%s"`, putURL[:min(80, len(putURL))]+"...")),
		zap.String("get_curl_example", fmt.Sprintf(`curl "%s"`, getURL[:min(80, len(getURL))]+"...")))

	return nil
}

// demonstrateBatchOps shows batch operations
func demonstrateBatchOps(ctx context.Context, storage storagex.Storage, logger storagex.Logger) error {
	logger.Info("=== Batch Operations Demo ===")

	// Create multiple test files
	testKeys := []string{
		"demo/batch/file1.txt",
		"demo/batch/file2.txt",
		"demo/batch/file3.txt",
		"demo/batch/file4.txt",
		"demo/batch/file5.txt",
	}

	// Upload multiple files
	logger.Info("Uploading multiple files", zap.Int("count", len(testKeys)))
	for i, key := range testKeys {
		content := fmt.Sprintf("This is test file number %d\nCreated for batch operations demo\nTimestamp: %s",
			i+1, time.Now().Format(time.RFC3339))

		_, err := storage.PutBytes(ctx, key, []byte(content), &storagex.PutOptions{
			ContentType: "text/plain",
			Metadata: map[string]string{
				"demo":  "batch-ops",
				"index": fmt.Sprintf("%d", i+1),
				"batch": "test-batch-1",
			},
		})
		if err != nil {
			return fmt.Errorf("failed to upload file %s: %w", key, err)
		}
	}

	logger.Info("All files uploaded successfully")

	// List all batch files
	logger.Info("Listing batch files")
	page, err := storage.List(ctx, storagex.ListOptions{
		Prefix:   "demo/batch/",
		PageSize: 20,
	})
	if err != nil {
		return fmt.Errorf("failed to list batch files: %w", err)
	}

	logger.Info("Batch files listed",
		zap.Int("count", len(page.Keys)))

	for _, obj := range page.Keys {
		logger.Info("Batch file",
			zap.String("key", obj.Key),
			zap.Int64("size", obj.Size),
			zap.Any("metadata", obj.Metadata))
	}

	// Delete batch of files
	deleteKeys := testKeys[:3] // Delete first 3 files
	logger.Info("Deleting batch of files", zap.Strings("keys", deleteKeys))

	failedKeys, err := storage.DeleteBatch(ctx, deleteKeys)
	if err != nil {
		return fmt.Errorf("batch delete failed: %w", err)
	}

	if len(failedKeys) > 0 {
		logger.Warn("Some files failed to delete", zap.Strings("failed_keys", failedKeys))
	} else {
		logger.Info("All files deleted successfully")
	}

	// Verify deletion by listing again
	logger.Info("Verifying deletion")
	finalPage, err := storage.List(ctx, storagex.ListOptions{
		Prefix: "demo/batch/",
	})
	if err != nil {
		return fmt.Errorf("failed to list files after deletion: %w", err)
	}

	logger.Info("Remaining files after batch delete",
		zap.Int("count", len(finalPage.Keys)))

	for _, obj := range finalPage.Keys {
		logger.Info("Remaining file", zap.String("key", obj.Key))
	}

	return nil
}

// Helper function for minimum
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
