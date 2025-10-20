// Package storagex provides a clean, DI-friendly object storage abstraction
// with first-class S3-compatible implementation (AWS S3 / MinIO).
package storagex

import (
	"context"
	"errors"
	"fmt"
	"io"
	"time"
)

// Domain Errors - use errors.Is for checking
var (
	// ErrNotFound indicates the requested object was not found
	ErrNotFound = errors.New("storagex: object not found")

	// ErrConflict indicates a conflict during object creation (e.g., already exists when overwrite=false)
	ErrConflict = errors.New("storagex: object conflict")

	// ErrInvalidConfig indicates the storage configuration is invalid
	ErrInvalidConfig = errors.New("storagex: invalid configuration")

	// ErrAborted indicates the operation was aborted (e.g., multipart upload cancelled)
	ErrAborted = errors.New("storagex: operation aborted")

	// ErrTimeout indicates the operation timed out
	ErrTimeout = errors.New("storagex: operation timeout")

	// ErrTooLarge indicates the object is too large for the operation
	ErrTooLarge = errors.New("storagex: object too large")

	// ErrInvalidKey indicates the object key is invalid
	ErrInvalidKey = errors.New("storagex: invalid object key")
)

// StorageError wraps underlying errors with additional context
type StorageError struct {
	Op  string // operation that failed
	Key string // object key (if applicable)
	Err error  // underlying error
}

func (e *StorageError) Error() string {
	if e.Key != "" {
		return fmt.Sprintf("storagex %s %q: %v", e.Op, e.Key, e.Err)
	}
	return fmt.Sprintf("storagex %s: %v", e.Op, e.Err)
}

func (e *StorageError) Unwrap() error {
	return e.Err
}

// IsNotFound checks if an error is or wraps ErrNotFound
func IsNotFound(err error) bool {
	return errors.Is(err, ErrNotFound)
}

// PutOptions configures object storage operations
type PutOptions struct {
	// ContentType specifies the MIME type of the object
	ContentType string

	// Metadata contains user-defined key-value pairs
	Metadata map[string]string

	// Overwrite controls conflict behavior:
	// - true: overwrite existing objects
	// - false: fail if object already exists
	Overwrite bool

	// CacheControl sets the Cache-Control header
	CacheControl string

	// ContentEncoding sets the Content-Encoding header
	ContentEncoding string
}

// Stat contains object metadata and statistics
type Stat struct {
	// Key is the object identifier
	Key string

	// Size is the object size in bytes
	Size int64

	// ETag is the entity tag (often MD5 hash)
	ETag string

	// ContentType is the MIME type
	ContentType string

	// Metadata contains user-defined key-value pairs
	Metadata map[string]string

	// LastModified is when the object was last modified
	LastModified time.Time

	// StorageClass indicates the storage tier (if applicable)
	StorageClass string
}

// ListOptions configures object listing operations
type ListOptions struct {
	// Prefix filters objects by key prefix
	Prefix string

	// Delimiter creates a hierarchical view (e.g., "/" for folders)
	Delimiter string

	// PageSize limits the number of objects per page (default: 1000)
	PageSize int32

	// ContinuationToken continues from a previous listing
	ContinuationToken string
}

// ListPage contains a page of listing results
type ListPage struct {
	// Keys contains object metadata for this page
	Keys []Stat

	// CommonPrefixes contains "folder" prefixes when Delimiter is used
	CommonPrefixes []string

	// NextToken can be used to continue listing (empty if done)
	NextToken string

	// IsTruncated indicates if there are more results available
	IsTruncated bool
}

// MultipartConfig configures large file uploads
type MultipartConfig struct {
	// PartSizeBytes is the size of each upload part (default: 8MB)
	// Must be between 5MB and 5GB for S3
	PartSizeBytes int64

	// Concurrency is the number of parts to upload in parallel (default: 4)
	Concurrency int

	// UploadToken provides idempotency for retry scenarios
	// If empty, a UUID will be generated
	UploadToken string
}

// PresignOptions configures presigned URL generation
type PresignOptions struct {
	// Expiry is how long the URL remains valid (default: 15m)
	Expiry time.Duration

	// ContentType restricts the upload content type
	ContentType string

	// Metadata contains user-defined key-value pairs for uploads
	Metadata map[string]string

	// ContentLengthRange restricts upload size [min, max] bytes
	ContentLengthRange [2]int64
}

// ReaderAtCloser combines io.ReadCloser with optional ReadAt capability
type ReaderAtCloser interface {
	io.ReadCloser

	// Size returns the total size if known, -1 if unknown
	Size() int64
}

// Storage is the main interface for object storage operations
type Storage interface {
	// Put stores an object from an io.Reader
	Put(ctx context.Context, key string, r io.Reader, opts *PutOptions) (Stat, error)

	// PutBytes stores an object from a byte slice (convenience method)
	PutBytes(ctx context.Context, key string, data []byte, opts *PutOptions) (Stat, error)

	// PutFile stores an object from a local file path (convenience method)
	PutFile(ctx context.Context, key string, path string, opts *PutOptions) (Stat, error)

	// Get retrieves an object as a streaming reader with metadata
	Get(ctx context.Context, key string) (ReaderAtCloser, Stat, error)

	// Head retrieves object metadata without the payload
	Head(ctx context.Context, key string) (Stat, error)

	// List retrieves objects with optional filtering and pagination
	List(ctx context.Context, opts ListOptions) (ListPage, error)

	// Delete removes a single object
	Delete(ctx context.Context, key string) error

	// DeleteBatch removes multiple objects, returns keys that failed to delete
	DeleteBatch(ctx context.Context, keys []string) ([]string, error)

	// MultipartUpload uploads large objects using multipart upload
	// Automatically chunks the source reader and manages the upload process
	MultipartUpload(ctx context.Context, key string, src io.Reader, cfg *MultipartConfig, putOpts *PutOptions) (Stat, error)

	// CreateMultipart initiates a multipart upload session
	CreateMultipart(ctx context.Context, key string, putOpts *PutOptions) (uploadID string, err error)

	// UploadPart uploads a single part in a multipart upload
	UploadPart(ctx context.Context, key, uploadID string, partNumber int32, part io.Reader, size int64) (etag string, err error)

	// CompleteMultipart finalizes a multipart upload
	CompleteMultipart(ctx context.Context, key, uploadID string, etags []string) (Stat, error)

	// AbortMultipart cancels a multipart upload and cleans up parts
	AbortMultipart(ctx context.Context, key, uploadID string) error

	// PresignGet generates a presigned URL for downloading an object
	PresignGet(ctx context.Context, key string, opts *PresignOptions) (url string, err error)

	// PresignPut generates a presigned URL for uploading an object
	PresignPut(ctx context.Context, key string, opts *PresignOptions) (url string, err error)
}

// PartETag represents a completed multipart upload part
type PartETag struct {
	PartNumber int32
	ETag       string
}

// MultipartUploadInfo contains details about an active multipart upload
type MultipartUploadInfo struct {
	Key       string
	UploadID  string
	Initiated time.Time
}
