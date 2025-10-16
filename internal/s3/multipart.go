package s3

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"sync"
	"sync/atomic"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/google/uuid"

	"github.com/gostratum/storagex/pkg/storagex"
)

// MultipartUploader handles chunked uploads for large files
type MultipartUploader struct {
	storage *S3Storage
	client  *s3.Client
	bucket  string
	logger  storagex.Logger
}

// NewMultipartUploader creates a new multipart uploader
func NewMultipartUploader(storage *S3Storage) *MultipartUploader {
	return &MultipartUploader{
		storage: storage,
		client:  storage.client.GetS3Client(),
		bucket:  storage.client.GetConfig().Bucket,
		logger:  storage.logger,
	}
}

// MultipartUpload uploads large objects using multipart upload
// Automatically chunks the source reader and manages the upload process
func (s *S3Storage) MultipartUpload(ctx context.Context, key string, src io.Reader, cfg *storagex.MultipartConfig, putOpts *storagex.PutOptions) (storagex.Stat, error) {
	if cfg == nil {
		cfg = s.client.GetConfig().GetMultipartConfig()
	}

	// Validate configuration
	if cfg.PartSizeBytes < 5<<20 { // 5MB minimum for S3
		cfg.PartSizeBytes = 5 << 20
	}
	if cfg.Concurrency <= 0 {
		cfg.Concurrency = 4
	}

	// Generate upload token if not provided
	if cfg.UploadToken == "" {
		cfg.UploadToken = uuid.New().String()
	}

	s.logger.Info("Starting multipart upload",
		"key", key,
		"upload_token", cfg.UploadToken,
		"part_size_mb", cfg.PartSizeBytes/(1<<20),
		"concurrency", cfg.Concurrency)

	uploader := NewMultipartUploader(s)
	return uploader.Upload(ctx, key, src, cfg, putOpts)
}

// Upload performs the actual multipart upload
func (mu *MultipartUploader) Upload(ctx context.Context, key string, src io.Reader, cfg *storagex.MultipartConfig, putOpts *storagex.PutOptions) (storagex.Stat, error) {
	storageKey := mu.storage.keyBuilder.BuildKey(key, nil)

	// Create multipart upload
	uploadID, err := mu.storage.CreateMultipart(ctx, key, putOpts)
	if err != nil {
		return storagex.Stat{}, fmt.Errorf("failed to create multipart upload: %w", err)
	}

	// Ensure cleanup on failure
	defer func() {
		if err != nil {
			if abortErr := mu.storage.AbortMultipart(ctx, key, uploadID); abortErr != nil {
				mu.logger.Warn("Failed to abort multipart upload",
					"key", key,
					"upload_id", uploadID,
					"error", abortErr)
			}
		}
	}()

	// Upload parts concurrently
	parts, err := mu.uploadParts(ctx, storageKey, uploadID, src, cfg)
	if err != nil {
		return storagex.Stat{}, fmt.Errorf("failed to upload parts: %w", err)
	}

	// Complete the multipart upload
	etags := make([]string, len(parts))
	for i, part := range parts {
		etags[i] = part.ETag
	}

	stat, err := mu.storage.CompleteMultipart(ctx, key, uploadID, etags)
	if err != nil {
		return storagex.Stat{}, fmt.Errorf("failed to complete multipart upload: %w", err)
	}

	mu.logger.Info("Multipart upload completed successfully",
		"key", key,
		"upload_id", uploadID,
		"parts", len(parts),
		"size", stat.Size)

	return stat, nil
}

// uploadParts uploads all parts concurrently with proper error handling
func (mu *MultipartUploader) uploadParts(ctx context.Context, storageKey, uploadID string, src io.Reader, cfg *storagex.MultipartConfig) ([]storagex.PartETag, error) {
	// Channel for parts to upload
	partChan := make(chan partUploadTask, cfg.Concurrency*2)

	// Channel for results
	resultChan := make(chan partUploadResult, cfg.Concurrency*2)

	// Start worker goroutines
	var wg sync.WaitGroup
	for i := 0; i < cfg.Concurrency; i++ {
		wg.Add(1)
		go mu.partUploadWorker(ctx, storageKey, uploadID, partChan, resultChan, &wg)
	}

	// Start reader goroutine to chunk the input
	go mu.chunkReader(ctx, src, cfg.PartSizeBytes, partChan)

	// Collect results
	var parts []storagex.PartETag
	var uploadErr error
	partCount := int32(0)

	// Wait for workers to finish
	go func() {
		wg.Wait()
		close(resultChan)
	}()

	// Process results
	for result := range resultChan {
		if result.err != nil {
			uploadErr = result.err
			mu.logger.Error("Part upload failed",
				"part_number", result.partNumber,
				"error", result.err)
			break
		}

		parts = append(parts, storagex.PartETag{
			PartNumber: result.partNumber,
			ETag:       result.etag,
		})

		atomic.AddInt32(&partCount, 1)
		mu.logger.Debug("Part uploaded successfully",
			"part_number", result.partNumber,
			"etag", result.etag,
			"total_parts", partCount)
	}

	if uploadErr != nil {
		return nil, uploadErr
	}

	// Sort parts by part number to ensure correct order
	for i := 0; i < len(parts)-1; i++ {
		for j := i + 1; j < len(parts); j++ {
			if parts[i].PartNumber > parts[j].PartNumber {
				parts[i], parts[j] = parts[j], parts[i]
			}
		}
	}

	return parts, nil
}

// partUploadTask represents a part to be uploaded
type partUploadTask struct {
	partNumber int32
	data       []byte
}

// partUploadResult represents the result of uploading a part
type partUploadResult struct {
	partNumber int32
	etag       string
	err        error
}

// partUploadWorker uploads parts from the channel
func (mu *MultipartUploader) partUploadWorker(ctx context.Context, storageKey, uploadID string, partChan <-chan partUploadTask, resultChan chan<- partUploadResult, wg *sync.WaitGroup) {
	defer wg.Done()

	for {
		select {
		case <-ctx.Done():
			resultChan <- partUploadResult{err: ctx.Err()}
			return

		case task, ok := <-partChan:
			if !ok {
				return // Channel closed, worker finished
			}

			etag, err := mu.uploadPart(ctx, storageKey, uploadID, task.partNumber, task.data)
			resultChan <- partUploadResult{
				partNumber: task.partNumber,
				etag:       etag,
				err:        err,
			}
		}
	}
}

// chunkReader reads from src and sends chunks to the part channel
func (mu *MultipartUploader) chunkReader(ctx context.Context, src io.Reader, partSize int64, partChan chan<- partUploadTask) {
	defer close(partChan)

	partNumber := int32(1)
	buffer := make([]byte, partSize)

	for {
		select {
		case <-ctx.Done():
			return

		default:
			n, err := io.ReadFull(src, buffer)
			if err != nil && err != io.ErrUnexpectedEOF && err != io.EOF {
				mu.logger.Error("Error reading from source", "error", err)
				return
			}

			if n == 0 {
				return // No more data
			}

			// Create a copy of the data for this part
			partData := make([]byte, n)
			copy(partData, buffer[:n])

			select {
			case <-ctx.Done():
				return
			case partChan <- partUploadTask{
				partNumber: partNumber,
				data:       partData,
			}:
				partNumber++
			}

			if err == io.EOF || err == io.ErrUnexpectedEOF {
				return // Finished reading
			}
		}
	}
}

// uploadPart uploads a single part
func (mu *MultipartUploader) uploadPart(ctx context.Context, storageKey, uploadID string, partNumber int32, data []byte) (string, error) {
	input := &s3.UploadPartInput{
		Bucket:     aws.String(mu.bucket),
		Key:        aws.String(storageKey),
		PartNumber: aws.Int32(partNumber),
		UploadId:   aws.String(uploadID),
		Body:       bytes.NewReader(data),
	}

	output, err := mu.client.UploadPart(ctx, input)
	if err != nil {
		return "", MapS3Error(err, "upload_part", storageKey)
	}

	return aws.ToString(output.ETag), nil
}

// CreateMultipart initiates a multipart upload session
func (s *S3Storage) CreateMultipart(ctx context.Context, key string, putOpts *storagex.PutOptions) (string, error) {
	storageKey := s.keyBuilder.BuildKey(key, nil)

	s.logger.Debug("Creating multipart upload",
		"key", key,
		"storage_key", storageKey)

	input := &s3.CreateMultipartUploadInput{
		Bucket: aws.String(s.client.GetConfig().Bucket),
		Key:    aws.String(storageKey),
	}

	// Set options if provided
	if putOpts != nil {
		if putOpts.ContentType != "" {
			input.ContentType = aws.String(putOpts.ContentType)
		}

		if putOpts.CacheControl != "" {
			input.CacheControl = aws.String(putOpts.CacheControl)
		}

		if putOpts.ContentEncoding != "" {
			input.ContentEncoding = aws.String(putOpts.ContentEncoding)
		}

		if len(putOpts.Metadata) > 0 {
			input.Metadata = putOpts.Metadata
		}
	}

	output, err := s.client.GetS3Client().CreateMultipartUpload(ctx, input)
	if err != nil {
		return "", MapS3Error(err, "create_multipart", key)
	}

	uploadID := aws.ToString(output.UploadId)
	s.logger.Debug("Multipart upload created",
		"key", key,
		"upload_id", uploadID)

	return uploadID, nil
}

// UploadPart uploads a single part in a multipart upload
func (s *S3Storage) UploadPart(ctx context.Context, key, uploadID string, partNumber int32, part io.Reader, size int64) (string, error) {
	storageKey := s.keyBuilder.BuildKey(key, nil)

	s.logger.Debug("Uploading part",
		"key", key,
		"upload_id", uploadID,
		"part_number", partNumber,
		"size", size)

	// Read the part data
	data, err := io.ReadAll(part)
	if err != nil {
		return "", &storagex.StorageError{
			Op:  "upload_part",
			Key: key,
			Err: fmt.Errorf("failed to read part data: %w", err),
		}
	}

	input := &s3.UploadPartInput{
		Bucket:     aws.String(s.client.GetConfig().Bucket),
		Key:        aws.String(storageKey),
		PartNumber: aws.Int32(partNumber),
		UploadId:   aws.String(uploadID),
		Body:       bytes.NewReader(data),
	}

	output, err := s.client.GetS3Client().UploadPart(ctx, input)
	if err != nil {
		return "", MapS3Error(err, "upload_part", key)
	}

	etag := aws.ToString(output.ETag)
	s.logger.Debug("Part uploaded successfully",
		"key", key,
		"part_number", partNumber,
		"etag", etag)

	return etag, nil
}

// CompleteMultipart finalizes a multipart upload
func (s *S3Storage) CompleteMultipart(ctx context.Context, key, uploadID string, etags []string) (storagex.Stat, error) {
	storageKey := s.keyBuilder.BuildKey(key, nil)

	s.logger.Debug("Completing multipart upload",
		"key", key,
		"upload_id", uploadID,
		"parts", len(etags))

	// Build completed parts
	parts := make([]types.CompletedPart, len(etags))
	for i, etag := range etags {
		parts[i] = types.CompletedPart{
			ETag:       aws.String(etag),
			PartNumber: aws.Int32(int32(i + 1)),
		}
	}

	input := &s3.CompleteMultipartUploadInput{
		Bucket:   aws.String(s.client.GetConfig().Bucket),
		Key:      aws.String(storageKey),
		UploadId: aws.String(uploadID),
		MultipartUpload: &types.CompletedMultipartUpload{
			Parts: parts,
		},
	}

	output, err := s.client.GetS3Client().CompleteMultipartUpload(ctx, input)
	if err != nil {
		return storagex.Stat{}, MapS3Error(err, "complete_multipart", key)
	}

	// Get object metadata to build complete stat
	stat, err := s.Head(ctx, key)
	if err != nil {
		// Fallback to basic stat if head fails
		stat = storagex.Stat{
			Key: key,
		}
		if output.ETag != nil {
			stat.ETag = aws.ToString(output.ETag)
		}
	}

	s.logger.Info("Multipart upload completed successfully",
		"key", key,
		"upload_id", uploadID,
		"size", stat.Size,
		"etag", stat.ETag)

	return stat, nil
}

// AbortMultipart cancels a multipart upload and cleans up parts
func (s *S3Storage) AbortMultipart(ctx context.Context, key, uploadID string) error {
	storageKey := s.keyBuilder.BuildKey(key, nil)

	s.logger.Debug("Aborting multipart upload",
		"key", key,
		"upload_id", uploadID)

	input := &s3.AbortMultipartUploadInput{
		Bucket:   aws.String(s.client.GetConfig().Bucket),
		Key:      aws.String(storageKey),
		UploadId: aws.String(uploadID),
	}

	_, err := s.client.GetS3Client().AbortMultipartUpload(ctx, input)
	if err != nil {
		return MapS3Error(err, "abort_multipart", key)
	}

	s.logger.Debug("Multipart upload aborted successfully",
		"key", key,
		"upload_id", uploadID)

	return nil
}
