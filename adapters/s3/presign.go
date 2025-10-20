package s3

import (
	"context"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"

	"github.com/gostratum/storagex"
)

// PresignGet generates a presigned URL for downloading an object
func (s *S3Storage) PresignGet(ctx context.Context, key string, opts *storagex.PresignOptions) (string, error) {
	if opts == nil {
		opts = &storagex.PresignOptions{
			Expiry: 15 * time.Minute,
		}
	}

	// Validate expiry
	if opts.Expiry <= 0 {
		opts.Expiry = 15 * time.Minute
	}
	if opts.Expiry > 7*24*time.Hour { // AWS limit is 7 days
		opts.Expiry = 7 * 24 * time.Hour
	}

	storageKey := s.keyBuilder.BuildKey(key, nil)

	s.logger.Debug("Generating presigned GET URL",
		"key", key,
		"storage_key", storageKey,
		"expiry", opts.Expiry)

	// Build GetObject input
	input := &s3.GetObjectInput{
		Bucket: aws.String(s.client.GetConfig().Bucket),
		Key:    aws.String(storageKey),
	}

	// Set response content type if specified
	if opts.ContentType != "" {
		input.ResponseContentType = aws.String(opts.ContentType)
	}

	// Generate presigned request
	req, err := s.client.GetPresignClient().PresignGetObject(ctx, input, func(presignOpts *s3.PresignOptions) {
		presignOpts.Expires = opts.Expiry
	})
	if err != nil {
		return "", &storagex.StorageError{
			Op:  "presign_get",
			Key: key,
			Err: fmt.Errorf("failed to generate presigned GET URL: %w", err),
		}
	}

	s.logger.Debug("Presigned GET URL generated successfully",
		"key", key,
		"expiry", opts.Expiry)

	return req.URL, nil
}

// PresignPut generates a presigned URL for uploading an object
func (s *S3Storage) PresignPut(ctx context.Context, key string, opts *storagex.PresignOptions) (string, error) {
	if opts == nil {
		opts = &storagex.PresignOptions{
			Expiry: 15 * time.Minute,
		}
	}

	// Validate expiry
	if opts.Expiry <= 0 {
		opts.Expiry = 15 * time.Minute
	}
	if opts.Expiry > 7*24*time.Hour { // AWS limit is 7 days
		opts.Expiry = 7 * 24 * time.Hour
	}

	storageKey := s.keyBuilder.BuildKey(key, nil)

	s.logger.Debug("Generating presigned PUT URL",
		"key", key,
		"storage_key", storageKey,
		"expiry", opts.Expiry)

	// Build PutObject input
	input := &s3.PutObjectInput{
		Bucket: aws.String(s.client.GetConfig().Bucket),
		Key:    aws.String(storageKey),
	}

	// Set content type if specified
	if opts.ContentType != "" {
		input.ContentType = aws.String(opts.ContentType)
	}

	// Set metadata if provided
	if len(opts.Metadata) > 0 {
		input.Metadata = opts.Metadata
	}

	// Generate presigned request with conditions
	presignOpts := func(presignOpts *s3.PresignOptions) {
		presignOpts.Expires = opts.Expiry
	}

	req, err := s.client.GetPresignClient().PresignPutObject(ctx, input, presignOpts)
	if err != nil {
		return "", &storagex.StorageError{
			Op:  "presign_put",
			Key: key,
			Err: fmt.Errorf("failed to generate presigned PUT URL: %w", err),
		}
	}

	s.logger.Debug("Presigned PUT URL generated successfully",
		"key", key,
		"expiry", opts.Expiry)

	return req.URL, nil
}

// PresignPostPolicy generates a presigned POST policy for browser uploads
// This is more flexible than PresignPut as it allows additional conditions
func (s *S3Storage) PresignPostPolicy(ctx context.Context, key string, opts *storagex.PresignOptions) (*PresignedPost, error) {
	if opts == nil {
		opts = &storagex.PresignOptions{
			Expiry: 15 * time.Minute,
		}
	}

	// Validate expiry
	if opts.Expiry <= 0 {
		opts.Expiry = 15 * time.Minute
	}
	if opts.Expiry > 7*24*time.Hour {
		opts.Expiry = 7 * 24 * time.Hour
	}

	storageKey := s.keyBuilder.BuildKey(key, nil)

	s.logger.Debug("Generating presigned POST policy",
		"key", key,
		"storage_key", storageKey,
		"expiry", opts.Expiry)

	// Build POST presign input
	input := &s3.PutObjectInput{
		Bucket: aws.String(s.client.GetConfig().Bucket),
		Key:    aws.String(storageKey),
	}

	// Set content type condition if specified
	conditions := make(map[string]string)
	if opts.ContentType != "" {
		input.ContentType = aws.String(opts.ContentType)
		conditions["Content-Type"] = opts.ContentType
	}

	// Set content length range conditions if specified
	if opts.ContentLengthRange[0] > 0 || opts.ContentLengthRange[1] > 0 {
		conditions["content-length-range"] = fmt.Sprintf("%d,%d",
			opts.ContentLengthRange[0], opts.ContentLengthRange[1])
	}

	// Set metadata conditions
	if len(opts.Metadata) > 0 {
		input.Metadata = opts.Metadata
		for k, v := range opts.Metadata {
			conditions["x-amz-meta-"+k] = v
		}
	}

	// Generate presigned POST
	req, err := s.client.GetPresignClient().PresignPutObject(ctx, input, func(presignOpts *s3.PresignOptions) {
		presignOpts.Expires = opts.Expiry
	})
	if err != nil {
		return nil, &storagex.StorageError{
			Op:  "presign_post",
			Key: key,
			Err: fmt.Errorf("failed to generate presigned POST policy: %w", err),
		}
	}

	// Build the response (simplified - in production you might want a more complete policy)
	post := &PresignedPost{
		URL: req.URL,
		Fields: map[string]string{
			"key": storageKey,
		},
		Conditions: conditions,
		Expiry:     time.Now().Add(opts.Expiry),
	}

	// Add content type field if specified
	if opts.ContentType != "" {
		post.Fields["Content-Type"] = opts.ContentType
	}

	// Add metadata fields
	for k, v := range opts.Metadata {
		post.Fields["x-amz-meta-"+k] = v
	}

	s.logger.Debug("Presigned POST policy generated successfully",
		"key", key,
		"expiry", opts.Expiry)

	return post, nil
}

// PresignedPost contains the data needed for a presigned POST upload
type PresignedPost struct {
	// URL is the endpoint for the POST request
	URL string `json:"url"`

	// Fields contains the form fields that must be included in the POST request
	Fields map[string]string `json:"fields"`

	// Conditions contains the policy conditions
	Conditions map[string]string `json:"conditions"`

	// Expiry is when this presigned POST expires
	Expiry time.Time `json:"expiry"`
}

// IsValid returns true if the presigned POST is still valid
func (pp *PresignedPost) IsValid() bool {
	return time.Now().Before(pp.Expiry)
}

// GetFormData returns the presigned POST data formatted for HTML forms
func (pp *PresignedPost) GetFormData() map[string]string {
	formData := make(map[string]string)

	// Copy all fields
	for k, v := range pp.Fields {
		formData[k] = v
	}

	return formData
}

// GetCurlCommand generates a curl command for testing the presigned POST
func (pp *PresignedPost) GetCurlCommand(filePath string) string {
	cmd := fmt.Sprintf("curl -X POST %q", pp.URL)

	for k, v := range pp.Fields {
		cmd += fmt.Sprintf(` -F %s=%q`, k, v)
	}

	cmd += fmt.Sprintf(` -F "file=@%s"`, filePath)

	return cmd
}

// PresignUpload generates a presigned URL for uploading with optional conditions
// This is a convenience method that chooses between PUT and POST based on needs
func (s *S3Storage) PresignUpload(ctx context.Context, key string, opts *storagex.PresignOptions) (*UploadPresign, error) {
	if opts == nil {
		opts = &storagex.PresignOptions{
			Expiry: 15 * time.Minute,
		}
	}

	// Use POST policy if we have conditions, otherwise use simple PUT
	hasConditions := opts.ContentType != "" ||
		len(opts.Metadata) > 0 ||
		opts.ContentLengthRange[0] > 0 ||
		opts.ContentLengthRange[1] > 0

	if hasConditions {
		post, err := s.PresignPostPolicy(ctx, key, opts)
		if err != nil {
			return nil, err
		}

		return &UploadPresign{
			Method: "POST",
			URL:    post.URL,
			Fields: post.Fields,
			Expiry: post.Expiry,
		}, nil
	}

	// Simple PUT presign
	url, err := s.PresignPut(ctx, key, opts)
	if err != nil {
		return nil, err
	}

	return &UploadPresign{
		Method: "PUT",
		URL:    url,
		Fields: make(map[string]string),
		Expiry: time.Now().Add(opts.Expiry),
	}, nil
}

// UploadPresign contains presigned upload information
type UploadPresign struct {
	// Method is the HTTP method to use (PUT or POST)
	Method string `json:"method"`

	// URL is the presigned URL
	URL string `json:"url"`

	// Fields contains form fields for POST requests (empty for PUT)
	Fields map[string]string `json:"fields,omitempty"`

	// Expiry is when this presign expires
	Expiry time.Time `json:"expiry"`
}

// IsValid returns true if the upload presign is still valid
func (up *UploadPresign) IsValid() bool {
	return time.Now().Before(up.Expiry)
}

// GetHeaders returns HTTP headers for PUT requests
func (up *UploadPresign) GetHeaders() map[string]string {
	headers := make(map[string]string)

	if up.Method == "PUT" {
		// For PUT requests, any conditions would be in query parameters or headers
		// This is a simplified implementation
		headers["Content-Type"] = "application/octet-stream"
	}

	return headers
}

// ValidatePresignOptions validates presign options
func ValidatePresignOptions(opts *storagex.PresignOptions) error {
	if opts == nil {
		return nil
	}

	// Validate expiry
	if opts.Expiry < 0 {
		return fmt.Errorf("expiry cannot be negative")
	}
	if opts.Expiry > 7*24*time.Hour {
		return fmt.Errorf("expiry cannot exceed 7 days")
	}

	// Validate content length range
	if opts.ContentLengthRange[0] < 0 || opts.ContentLengthRange[1] < 0 {
		return fmt.Errorf("content length range values cannot be negative")
	}
	if opts.ContentLengthRange[1] > 0 && opts.ContentLengthRange[0] > opts.ContentLengthRange[1] {
		return fmt.Errorf("content length range minimum cannot exceed maximum")
	}

	// Validate metadata keys (S3 has restrictions on metadata keys)
	for key := range opts.Metadata {
		if len(key) == 0 {
			return fmt.Errorf("metadata key cannot be empty")
		}
		// S3 metadata keys must be valid HTTP header names
		for _, r := range key {
			if !isValidHeaderRune(r) {
				return fmt.Errorf("invalid character in metadata key %q", key)
			}
		}
	}

	return nil
}

// isValidHeaderRune checks if a rune is valid in HTTP header names
func isValidHeaderRune(r rune) bool {
	return (r >= 'a' && r <= 'z') ||
		(r >= 'A' && r <= 'Z') ||
		(r >= '0' && r <= '9') ||
		r == '-' || r == '_'
}
