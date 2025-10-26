package testutil

import (
	"bytes"
	"context"
	"errors"
	"io"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/gostratum/storagex"
)

// MockStorage is a thread-safe in-memory implementation of storagex.Storage for testing
type MockStorage struct {
	mu      sync.RWMutex
	objects map[string]*mockObject // key -> object
}

type mockObject struct {
	data         []byte
	contentType  string
	metadata     map[string]string
	lastModified time.Time
	etag         string
	storageClass string
}

// NewMockStorage creates a new in-memory mock storage
func NewMockStorage() *MockStorage {
	return &MockStorage{
		objects: make(map[string]*mockObject),
	}
}

// Put stores an object from an io.Reader
func (m *MockStorage) Put(ctx context.Context, key string, r io.Reader, opts *storagex.PutOptions) (storagex.Stat, error) {
	if opts == nil {
		opts = &storagex.PutOptions{Overwrite: true}
	}

	// Check context
	if err := ctx.Err(); err != nil {
		return storagex.Stat{}, &storagex.StorageError{
			Op:  "put",
			Key: key,
			Err: err,
		}
	}

	// Read data
	data, err := io.ReadAll(r)
	if err != nil {
		return storagex.Stat{}, &storagex.StorageError{
			Op:  "put",
			Key: key,
			Err: err,
		}
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	// Check if object exists when overwrite is false
	if !opts.Overwrite {
		if _, exists := m.objects[key]; exists {
			return storagex.Stat{}, &storagex.StorageError{
				Op:  "put",
				Key: key,
				Err: storagex.ErrConflict,
			}
		}
	}

	// Generate simple ETag (MD5-like hash simulation)
	etag := generateETag(data)

	// Copy metadata to avoid mutation
	metadata := make(map[string]string)
	for k, v := range opts.Metadata {
		metadata[k] = v
	}

	// Store object
	m.objects[key] = &mockObject{
		data:         data,
		contentType:  opts.ContentType,
		metadata:     metadata,
		lastModified: time.Now().UTC(),
		etag:         etag,
		storageClass: "STANDARD",
	}

	return storagex.Stat{
		Key:          key,
		Size:         int64(len(data)),
		ETag:         etag,
		ContentType:  opts.ContentType,
		Metadata:     metadata,
		LastModified: m.objects[key].lastModified,
		StorageClass: "STANDARD",
	}, nil
}

// PutBytes stores an object from a byte slice
func (m *MockStorage) PutBytes(ctx context.Context, key string, data []byte, opts *storagex.PutOptions) (storagex.Stat, error) {
	return m.Put(ctx, key, bytes.NewReader(data), opts)
}

// PutFile stores an object from a local file path
func (m *MockStorage) PutFile(ctx context.Context, key string, path string, opts *storagex.PutOptions) (storagex.Stat, error) {
	return storagex.Stat{}, errors.New("PutFile not implemented in mock")
}

// Get retrieves an object as a streaming reader with metadata
func (m *MockStorage) Get(ctx context.Context, key string) (storagex.ReaderAtCloser, storagex.Stat, error) {
	// Check context
	if err := ctx.Err(); err != nil {
		return nil, storagex.Stat{}, &storagex.StorageError{
			Op:  "get",
			Key: key,
			Err: err,
		}
	}

	m.mu.RLock()
	defer m.mu.RUnlock()

	obj, exists := m.objects[key]
	if !exists {
		return nil, storagex.Stat{}, &storagex.StorageError{
			Op:  "get",
			Key: key,
			Err: storagex.ErrNotFound,
		}
	}

	// Copy metadata
	metadata := make(map[string]string)
	for k, v := range obj.metadata {
		metadata[k] = v
	}

	stat := storagex.Stat{
		Key:          key,
		Size:         int64(len(obj.data)),
		ETag:         obj.etag,
		ContentType:  obj.contentType,
		Metadata:     metadata,
		LastModified: obj.lastModified,
		StorageClass: obj.storageClass,
	}

	// Create reader from copy of data
	dataCopy := make([]byte, len(obj.data))
	copy(dataCopy, obj.data)
	reader := &mockReader{
		Reader: bytes.NewReader(dataCopy),
		size:   int64(len(dataCopy)),
	}

	return reader, stat, nil
}

// Head retrieves object metadata without the payload
func (m *MockStorage) Head(ctx context.Context, key string) (storagex.Stat, error) {
	// Check context
	if err := ctx.Err(); err != nil {
		return storagex.Stat{}, &storagex.StorageError{
			Op:  "head",
			Key: key,
			Err: err,
		}
	}

	m.mu.RLock()
	defer m.mu.RUnlock()

	obj, exists := m.objects[key]
	if !exists {
		return storagex.Stat{}, &storagex.StorageError{
			Op:  "head",
			Key: key,
			Err: storagex.ErrNotFound,
		}
	}

	// Copy metadata
	metadata := make(map[string]string)
	for k, v := range obj.metadata {
		metadata[k] = v
	}

	return storagex.Stat{
		Key:          key,
		Size:         int64(len(obj.data)),
		ETag:         obj.etag,
		ContentType:  obj.contentType,
		Metadata:     metadata,
		LastModified: obj.lastModified,
		StorageClass: obj.storageClass,
	}, nil
}

// List retrieves objects with optional filtering and pagination
func (m *MockStorage) List(ctx context.Context, opts storagex.ListOptions) (storagex.ListPage, error) {
	// Check context
	if err := ctx.Err(); err != nil {
		return storagex.ListPage{}, &storagex.StorageError{
			Op:  "list",
			Err: err,
		}
	}

	m.mu.RLock()
	defer m.mu.RUnlock()

	// Collect matching keys
	var matchingKeys []string
	for key := range m.objects {
		if opts.Prefix == "" || strings.HasPrefix(key, opts.Prefix) {
			matchingKeys = append(matchingKeys, key)
		}
	}

	// Sort for consistent ordering
	sort.Strings(matchingKeys)

	// Handle pagination
	pageSize := int(opts.PageSize)
	if pageSize <= 0 {
		pageSize = 1000
	}

	startIdx := 0
	if opts.ContinuationToken != "" {
		// Find the starting index based on continuation token
		for i, key := range matchingKeys {
			if key > opts.ContinuationToken {
				startIdx = i
				break
			}
		}
	}

	endIdx := startIdx + pageSize
	if endIdx > len(matchingKeys) {
		endIdx = len(matchingKeys)
	}

	pageKeys := matchingKeys[startIdx:endIdx]

	// Build result
	result := storagex.ListPage{
		Keys:        make([]storagex.Stat, 0, len(pageKeys)),
		IsTruncated: endIdx < len(matchingKeys),
	}

	if result.IsTruncated {
		result.NextToken = matchingKeys[endIdx-1]
	}

	// Handle delimiter (common prefixes)
	if opts.Delimiter != "" {
		prefixes := make(map[string]bool)
		for _, key := range pageKeys {
			// Strip the prefix to get the relative path
			relative := key
			if opts.Prefix != "" {
				relative = strings.TrimPrefix(key, opts.Prefix)
			}

			// Check if there's a delimiter
			if idx := strings.Index(relative, opts.Delimiter); idx >= 0 {
				// This is a "folder" - add to common prefixes
				prefix := opts.Prefix + relative[:idx+len(opts.Delimiter)]
				prefixes[prefix] = true
			} else {
				// This is a file at this level
				obj := m.objects[key]
				metadata := make(map[string]string)
				for k, v := range obj.metadata {
					metadata[k] = v
				}

				result.Keys = append(result.Keys, storagex.Stat{
					Key:          key,
					Size:         int64(len(obj.data)),
					ETag:         obj.etag,
					ContentType:  obj.contentType,
					Metadata:     metadata,
					LastModified: obj.lastModified,
					StorageClass: obj.storageClass,
				})
			}
		}

		// Add common prefixes
		for prefix := range prefixes {
			result.CommonPrefixes = append(result.CommonPrefixes, prefix)
		}
		sort.Strings(result.CommonPrefixes)
	} else {
		// No delimiter - return all objects
		for _, key := range pageKeys {
			obj := m.objects[key]
			metadata := make(map[string]string)
			for k, v := range obj.metadata {
				metadata[k] = v
			}

			result.Keys = append(result.Keys, storagex.Stat{
				Key:          key,
				Size:         int64(len(obj.data)),
				ETag:         obj.etag,
				ContentType:  obj.contentType,
				Metadata:     metadata,
				LastModified: obj.lastModified,
				StorageClass: obj.storageClass,
			})
		}
	}

	return result, nil
}

// Delete removes a single object
func (m *MockStorage) Delete(ctx context.Context, key string) error {
	// Check context
	if err := ctx.Err(); err != nil {
		return &storagex.StorageError{
			Op:  "delete",
			Key: key,
			Err: err,
		}
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	// S3 delete is idempotent - doesn't error if key doesn't exist
	delete(m.objects, key)
	return nil
}

// DeleteBatch removes multiple objects, returns keys that failed to delete
func (m *MockStorage) DeleteBatch(ctx context.Context, keys []string) ([]string, error) {
	// Check context
	if err := ctx.Err(); err != nil {
		return keys, &storagex.StorageError{
			Op:  "delete_batch",
			Err: err,
		}
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	// S3 delete is idempotent - doesn't error if keys don't exist
	for _, key := range keys {
		delete(m.objects, key)
	}

	return nil, nil
}

// MultipartUpload uploads large objects using multipart upload
func (m *MockStorage) MultipartUpload(ctx context.Context, key string, src io.Reader, cfg *storagex.MultipartConfig, putOpts *storagex.PutOptions) (storagex.Stat, error) {
	// For mock, just read all data and store normally
	if putOpts == nil {
		putOpts = &storagex.PutOptions{Overwrite: true}
	}
	return m.Put(ctx, key, src, putOpts)
}

// CreateMultipart initiates a multipart upload session
func (m *MockStorage) CreateMultipart(ctx context.Context, key string, putOpts *storagex.PutOptions) (uploadID string, err error) {
	return "mock-upload-id-" + key, nil
}

// UploadPart uploads a single part in a multipart upload
func (m *MockStorage) UploadPart(ctx context.Context, key, uploadID string, partNumber int32, part io.Reader, size int64) (etag string, err error) {
	data, err := io.ReadAll(part)
	if err != nil {
		return "", err
	}
	return generateETag(data), nil
}

// CompleteMultipart finalizes a multipart upload
func (m *MockStorage) CompleteMultipart(ctx context.Context, key, uploadID string, etags []string) (storagex.Stat, error) {
	// For mock, just return a basic stat
	return storagex.Stat{
		Key:          key,
		Size:         0,
		ETag:         "mock-etag",
		LastModified: time.Now().UTC(),
	}, nil
}

// AbortMultipart cancels a multipart upload and cleans up parts
func (m *MockStorage) AbortMultipart(ctx context.Context, key, uploadID string) error {
	return nil
}

// PresignGet generates a presigned URL for downloading an object
func (m *MockStorage) PresignGet(ctx context.Context, key string, opts *storagex.PresignOptions) (url string, err error) {
	// Check context
	if err := ctx.Err(); err != nil {
		return "", &storagex.StorageError{
			Op:  "presign_get",
			Key: key,
			Err: err,
		}
	}

	// For presign get, don't require the object to exist (similar to S3 behavior)
	// Return a mock URL
	return "https://mock-storage.example.com/" + key + "?signature=mock", nil
}

// PresignPut generates a presigned URL for uploading an object
func (m *MockStorage) PresignPut(ctx context.Context, key string, opts *storagex.PresignOptions) (url string, err error) {
	// Check context
	if err := ctx.Err(); err != nil {
		return "", &storagex.StorageError{
			Op:  "presign_put",
			Key: key,
			Err: err,
		}
	}

	// Return a mock URL
	return "https://mock-storage.example.com/" + key + "?signature=mock&upload=true", nil
}

// mockReader implements storagex.ReaderAtCloser
type mockReader struct {
	*bytes.Reader
	size int64
}

func (r *mockReader) Close() error {
	return nil
}

func (r *mockReader) Size() int64 {
	return r.size
}

// generateETag creates a simple hash-like ETag for testing
func generateETag(data []byte) string {
	// Simple length-based ETag for testing
	// In real S3, this would be an MD5 hash
	sum := len(data)
	for i, b := range data {
		if i >= 100 {
			break
		}
		sum += int(b)
	}
	return `"` + strings.Repeat("a", 32-len(string(rune(sum)))) + string(rune(sum)) + `"`
}
