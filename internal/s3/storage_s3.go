package s3

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"go.uber.org/zap"

	"github.com/gostratum/storagex/pkg/storagex"
)

// S3Storage implements the Storage interface using AWS S3
type S3Storage struct {
	client     *ClientManager
	keyBuilder storagex.KeyBuilder
	logger     storagex.Logger
}

// NewS3Storage creates a new S3 storage implementation
func NewS3Storage(ctx context.Context, cfg *storagex.Config, opts ...storagex.Option) (*S3Storage, error) {
	config, options := storagex.GetEffectiveConfig(cfg, opts...)

	// Validate configuration
	if err := storagex.ValidateConfig(config); err != nil {
		return nil, fmt.Errorf("invalid configuration: %w", err)
	}

	// Create client manager
	clientManager, err := NewClientManager(ctx, ClientConfig{
		Config: config,
		Logger: options.GetLogger(),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create client manager: %w", err)
	}

	storage := &S3Storage{
		client:     clientManager,
		keyBuilder: options.GetKeyBuilder(),
		logger:     options.GetLogger(),
	}

	return storage, nil
}

// Put stores an object from an io.Reader
func (s *S3Storage) Put(ctx context.Context, key string, r io.Reader, opts *storagex.PutOptions) (storagex.Stat, error) {
	if opts == nil {
		opts = &storagex.PutOptions{Overwrite: true}
	}

	// Build the actual storage key
	storageKey := s.keyBuilder.BuildKey(key, nil)

	s.logger.Debug("Putting object",
		zap.String("key", key),
		zap.String("storage_key", storageKey),
		zap.String("content_type", opts.ContentType))

	// Check if object exists when overwrite is false
	if !opts.Overwrite {
		if exists, err := s.objectExists(ctx, storageKey); err != nil {
			return storagex.Stat{}, MapS3Error(err, "put", key)
		} else if exists {
			return storagex.Stat{}, &storagex.StorageError{
				Op:  "put",
				Key: key,
				Err: storagex.ErrConflict,
			}
		}
	}

	// Read all data to get size and create seekable reader
	data, err := io.ReadAll(r)
	if err != nil {
		return storagex.Stat{}, &storagex.StorageError{
			Op:  "put",
			Key: key,
			Err: fmt.Errorf("failed to read data: %w", err),
		}
	}

	// Build PutObject input
	input := &s3.PutObjectInput{
		Bucket: aws.String(s.client.GetConfig().Bucket),
		Key:    aws.String(storageKey),
		Body:   bytes.NewReader(data),
	}

	// Set content type
	if opts.ContentType != "" {
		input.ContentType = aws.String(opts.ContentType)
	}

	// Set cache control
	if opts.CacheControl != "" {
		input.CacheControl = aws.String(opts.CacheControl)
	}

	// Set content encoding
	if opts.ContentEncoding != "" {
		input.ContentEncoding = aws.String(opts.ContentEncoding)
	}

	// Set metadata
	if len(opts.Metadata) > 0 {
		input.Metadata = opts.Metadata
	}

	// Execute the put operation
	output, err := s.client.GetS3Client().PutObject(ctx, input)
	if err != nil {
		return storagex.Stat{}, MapS3Error(err, "put", key)
	}

	// Build response stat
	stat := storagex.Stat{
		Key:          key,
		Size:         int64(len(data)),
		ContentType:  opts.ContentType,
		Metadata:     opts.Metadata,
		LastModified: time.Now(), // S3 doesn't return this in PutObject response
	}

	if output.ETag != nil {
		stat.ETag = aws.ToString(output.ETag)
	}

	s.logger.Debug("Object put successfully",
		zap.String("key", key),
		zap.Int64("size", stat.Size),
		zap.String("etag", stat.ETag))

	return stat, nil
}

// PutBytes stores an object from a byte slice (convenience method)
func (s *S3Storage) PutBytes(ctx context.Context, key string, data []byte, opts *storagex.PutOptions) (storagex.Stat, error) {
	return s.Put(ctx, key, bytes.NewReader(data), opts)
}

// PutFile stores an object from a local file path (convenience method)
func (s *S3Storage) PutFile(ctx context.Context, key string, path string, opts *storagex.PutOptions) (storagex.Stat, error) {
	file, err := os.Open(path)
	if err != nil {
		return storagex.Stat{}, &storagex.StorageError{
			Op:  "put_file",
			Key: key,
			Err: fmt.Errorf("failed to open file %q: %w", path, err),
		}
	}
	defer file.Close()

	// Auto-detect content type if not specified
	if opts == nil {
		opts = &storagex.PutOptions{}
	}

	if opts.ContentType == "" {
		if contentType := detectContentType(path); contentType != "" {
			opts.ContentType = contentType
		}
	}

	return s.Put(ctx, key, file, opts)
}

// Get retrieves an object as a streaming reader with metadata
func (s *S3Storage) Get(ctx context.Context, key string) (storagex.ReaderAtCloser, storagex.Stat, error) {
	storageKey := s.keyBuilder.BuildKey(key, nil)

	s.logger.Debug("Getting object",
		zap.String("key", key),
		zap.String("storage_key", storageKey))

	// Get object
	input := &s3.GetObjectInput{
		Bucket: aws.String(s.client.GetConfig().Bucket),
		Key:    aws.String(storageKey),
	}

	output, err := s.client.GetS3Client().GetObject(ctx, input)
	if err != nil {
		return nil, storagex.Stat{}, MapS3Error(err, "get", key)
	}

	// Build stat from response
	stat := storagex.Stat{
		Key: key,
	}

	if output.ContentLength != nil {
		stat.Size = aws.ToInt64(output.ContentLength)
	}

	if output.ETag != nil {
		stat.ETag = aws.ToString(output.ETag)
	}

	if output.ContentType != nil {
		stat.ContentType = aws.ToString(output.ContentType)
	}

	if output.LastModified != nil {
		stat.LastModified = *output.LastModified
	}

	if output.StorageClass != "" {
		stat.StorageClass = string(output.StorageClass)
	}

	// Convert metadata
	if output.Metadata != nil {
		stat.Metadata = output.Metadata
	}

	// Wrap the body in our reader interface
	reader := &s3Reader{
		ReadCloser: output.Body,
		size:       stat.Size,
	}

	s.logger.Debug("Object retrieved successfully",
		zap.String("key", key),
		zap.Int64("size", stat.Size),
		zap.String("etag", stat.ETag))

	return reader, stat, nil
}

// Head retrieves object metadata without the payload
func (s *S3Storage) Head(ctx context.Context, key string) (storagex.Stat, error) {
	storageKey := s.keyBuilder.BuildKey(key, nil)

	s.logger.Debug("Head object",
		zap.String("key", key),
		zap.String("storage_key", storageKey))

	input := &s3.HeadObjectInput{
		Bucket: aws.String(s.client.GetConfig().Bucket),
		Key:    aws.String(storageKey),
	}

	output, err := s.client.GetS3Client().HeadObject(ctx, input)
	if err != nil {
		return storagex.Stat{}, MapS3Error(err, "head", key)
	}

	// Build stat from response
	stat := storagex.Stat{
		Key: key,
	}

	if output.ContentLength != nil {
		stat.Size = aws.ToInt64(output.ContentLength)
	}

	if output.ETag != nil {
		stat.ETag = aws.ToString(output.ETag)
	}

	if output.ContentType != nil {
		stat.ContentType = aws.ToString(output.ContentType)
	}

	if output.LastModified != nil {
		stat.LastModified = *output.LastModified
	}

	if output.StorageClass != "" {
		stat.StorageClass = string(output.StorageClass)
	}

	// Convert metadata
	if output.Metadata != nil {
		stat.Metadata = output.Metadata
	}

	s.logger.Debug("Object head successful",
		zap.String("key", key),
		zap.Int64("size", stat.Size),
		zap.String("etag", stat.ETag))

	return stat, nil
}

// List retrieves objects with optional filtering and pagination
func (s *S3Storage) List(ctx context.Context, opts storagex.ListOptions) (storagex.ListPage, error) {
	s.logger.Debug("Listing objects",
		zap.String("prefix", opts.Prefix),
		zap.String("delimiter", opts.Delimiter),
		zap.Int32("page_size", opts.PageSize))

	// Build storage prefix
	storagePrefix := ""
	if opts.Prefix != "" {
		storagePrefix = s.keyBuilder.BuildKey(opts.Prefix, nil)
	}

	input := &s3.ListObjectsV2Input{
		Bucket: aws.String(s.client.GetConfig().Bucket),
	}

	if storagePrefix != "" {
		input.Prefix = aws.String(storagePrefix)
	}

	if opts.Delimiter != "" {
		input.Delimiter = aws.String(opts.Delimiter)
	}

	if opts.PageSize > 0 {
		input.MaxKeys = aws.Int32(opts.PageSize)
	} else {
		input.MaxKeys = aws.Int32(1000) // Default page size
	}

	if opts.ContinuationToken != "" {
		input.ContinuationToken = aws.String(opts.ContinuationToken)
	}

	output, err := s.client.GetS3Client().ListObjectsV2(ctx, input)
	if err != nil {
		return storagex.ListPage{}, MapS3Error(err, "list", "")
	}

	// Build response
	page := storagex.ListPage{
		Keys:           make([]storagex.Stat, 0, len(output.Contents)),
		CommonPrefixes: make([]string, 0, len(output.CommonPrefixes)),
		IsTruncated:    aws.ToBool(output.IsTruncated),
	}

	if output.NextContinuationToken != nil {
		page.NextToken = aws.ToString(output.NextContinuationToken)
	}

	// Convert objects
	for _, obj := range output.Contents {
		if obj.Key == nil {
			continue
		}

		// Strip prefix to get original key
		originalKey := s.keyBuilder.StripKey(aws.ToString(obj.Key))

		stat := storagex.Stat{
			Key: originalKey,
		}

		if obj.Size != nil {
			stat.Size = aws.ToInt64(obj.Size)
		}

		if obj.ETag != nil {
			stat.ETag = aws.ToString(obj.ETag)
		}

		if obj.LastModified != nil {
			stat.LastModified = *obj.LastModified
		}

		if obj.StorageClass != "" {
			stat.StorageClass = string(obj.StorageClass)
		}

		page.Keys = append(page.Keys, stat)
	}

	// Convert common prefixes
	for _, prefix := range output.CommonPrefixes {
		if prefix.Prefix != nil {
			originalPrefix := s.keyBuilder.StripKey(aws.ToString(prefix.Prefix))
			page.CommonPrefixes = append(page.CommonPrefixes, originalPrefix)
		}
	}

	s.logger.Debug("Objects listed successfully",
		zap.Int("count", len(page.Keys)),
		zap.Int("prefixes", len(page.CommonPrefixes)),
		zap.Bool("truncated", page.IsTruncated))

	return page, nil
}

// Delete removes a single object
func (s *S3Storage) Delete(ctx context.Context, key string) error {
	storageKey := s.keyBuilder.BuildKey(key, nil)

	s.logger.Debug("Deleting object",
		zap.String("key", key),
		zap.String("storage_key", storageKey))

	input := &s3.DeleteObjectInput{
		Bucket: aws.String(s.client.GetConfig().Bucket),
		Key:    aws.String(storageKey),
	}

	_, err := s.client.GetS3Client().DeleteObject(ctx, input)
	if err != nil {
		return MapS3Error(err, "delete", key)
	}

	s.logger.Debug("Object deleted successfully", zap.String("key", key))
	return nil
}

// DeleteBatch removes multiple objects, returns keys that failed to delete
func (s *S3Storage) DeleteBatch(ctx context.Context, keys []string) ([]string, error) {
	if len(keys) == 0 {
		return nil, nil
	}

	s.logger.Debug("Deleting objects batch", zap.Int("count", len(keys)))

	// Build delete objects input
	objects := make([]types.ObjectIdentifier, 0, len(keys))
	for _, key := range keys {
		storageKey := s.keyBuilder.BuildKey(key, nil)
		objects = append(objects, types.ObjectIdentifier{
			Key: aws.String(storageKey),
		})
	}

	input := &s3.DeleteObjectsInput{
		Bucket: aws.String(s.client.GetConfig().Bucket),
		Delete: &types.Delete{
			Objects: objects,
		},
	}

	output, err := s.client.GetS3Client().DeleteObjects(ctx, input)
	if err != nil {
		return keys, MapS3Error(err, "delete_batch", "")
	}

	// Collect failed keys
	var failedKeys []string
	for _, deleteError := range output.Errors {
		if deleteError.Key != nil {
			// Find the original key that corresponds to this storage key
			storageKey := aws.ToString(deleteError.Key)
			for _, originalKey := range keys {
				if s.keyBuilder.BuildKey(originalKey, nil) == storageKey {
					failedKeys = append(failedKeys, originalKey)
					break
				}
			}
		}
	}

	s.logger.Debug("Batch delete completed",
		zap.Int("requested", len(keys)),
		zap.Int("deleted", len(output.Deleted)),
		zap.Int("failed", len(failedKeys)))

	return failedKeys, nil
}

// objectExists checks if an object exists
func (s *S3Storage) objectExists(ctx context.Context, storageKey string) (bool, error) {
	input := &s3.HeadObjectInput{
		Bucket: aws.String(s.client.GetConfig().Bucket),
		Key:    aws.String(storageKey),
	}

	_, err := s.client.GetS3Client().HeadObject(ctx, input)
	if err != nil {
		var notFound *types.NotFound
		var noSuchKey *types.NoSuchKey
		if errors.As(err, &notFound) || errors.As(err, &noSuchKey) {
			return false, nil
		}
		return false, err
	}

	return true, nil
}

// s3Reader implements ReaderAtCloser for S3 objects
type s3Reader struct {
	io.ReadCloser
	size int64
}

func (r *s3Reader) Size() int64 {
	return r.size
}

// detectContentType attempts to detect content type from file extension
func detectContentType(filename string) string {
	// This is a simplified implementation. In production, you might want
	// to use the mime package or a more comprehensive content type detection.
	extensions := map[string]string{
		".html": "text/html",
		".css":  "text/css",
		".js":   "application/javascript",
		".json": "application/json",
		".xml":  "application/xml",
		".pdf":  "application/pdf",
		".jpg":  "image/jpeg",
		".jpeg": "image/jpeg",
		".png":  "image/png",
		".gif":  "image/gif",
		".svg":  "image/svg+xml",
		".txt":  "text/plain",
		".zip":  "application/zip",
		".tar":  "application/x-tar",
		".gz":   "application/gzip",
	}

	// Find extension
	for ext := len(filename) - 1; ext >= 0; ext-- {
		if filename[ext] == '.' {
			extension := filename[ext:]
			if contentType, exists := extensions[extension]; exists {
				return contentType
			}
			break
		}
	}

	return ""
}
