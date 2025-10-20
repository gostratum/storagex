package s3

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/gostratum/storagex"
)

// MapS3Error converts S3 SDK errors to domain errors
func MapS3Error(err error, op, key string) error {
	if err == nil {
		return nil
	}

	// Handle context errors
	if errors.Is(err, context.Canceled) {
		return &storagex.StorageError{
			Op:  op,
			Key: key,
			Err: storagex.ErrAborted,
		}
	}

	if errors.Is(err, context.DeadlineExceeded) {
		return &storagex.StorageError{
			Op:  op,
			Key: key,
			Err: storagex.ErrTimeout,
		}
	}

	// Handle specific S3 error types
	switch err.(type) {
	case *types.NoSuchBucket:
		return &storagex.StorageError{
			Op:  op,
			Key: key,
			Err: fmt.Errorf("%w: bucket does not exist", storagex.ErrNotFound),
		}

	case *types.NoSuchKey:
		return &storagex.StorageError{
			Op:  op,
			Key: key,
			Err: storagex.ErrNotFound,
		}

	case *types.BucketAlreadyExists:
		return &storagex.StorageError{
			Op:  op,
			Key: key,
			Err: fmt.Errorf("%w: bucket already exists", storagex.ErrConflict),
		}

	case *types.BucketAlreadyOwnedByYou:
		return &storagex.StorageError{
			Op:  op,
			Key: key,
			Err: fmt.Errorf("%w: bucket already owned by you", storagex.ErrConflict),
		}

	// TODO: Handle invalid bucket/request errors appropriately
	// case *types.SomeValidType:
	//    return &storagex.StorageError{
	//        Op:  op,
	//        Key: key,
	//        Err: fmt.Errorf("%w: invalid request", storagex.ErrInvalidConfig),
	//    }

	case *types.InvalidObjectState:
		return &storagex.StorageError{
			Op:  op,
			Key: key,
			Err: fmt.Errorf("%w: invalid object state", storagex.ErrConflict),
		}

	case *types.NotFound:
		return &storagex.StorageError{
			Op:  op,
			Key: key,
			Err: storagex.ErrNotFound,
		}

	default:
		// Try to extract HTTP status code and map based on that
		if httpErr := extractHTTPError(err); httpErr != nil {
			return mapHTTPError(httpErr, op, key)
		}

		// Check for AWS generic errors
		if awsErr := extractAWSError(err); awsErr != nil {
			return mapAWSError(awsErr, op, key)
		}

		// Handle string-based error matching for cases where type assertion fails
		if mappedErr := mapByErrorMessage(err, op, key); mappedErr != nil {
			return mappedErr
		}
	}

	// Default: wrap the original error
	return &storagex.StorageError{
		Op:  op,
		Key: key,
		Err: err,
	}
}

// HTTPError represents an HTTP-level error
type HTTPError struct {
	StatusCode int
	Status     string
	Message    string
}

// extractHTTPError attempts to extract HTTP status information from an error
func extractHTTPError(err error) *HTTPError {
	// This is a simplified implementation. In practice, you'd need to dig into
	// the AWS SDK's error structure to extract HTTP status codes properly.
	errStr := err.Error()

	// Look for HTTP status codes in error messages
	if strings.Contains(errStr, "404") || strings.Contains(strings.ToLower(errStr), "not found") {
		return &HTTPError{StatusCode: 404, Status: "Not Found", Message: errStr}
	}

	if strings.Contains(errStr, "403") || strings.Contains(strings.ToLower(errStr), "forbidden") {
		return &HTTPError{StatusCode: 403, Status: "Forbidden", Message: errStr}
	}

	if strings.Contains(errStr, "409") || strings.Contains(strings.ToLower(errStr), "conflict") {
		return &HTTPError{StatusCode: 409, Status: "Conflict", Message: errStr}
	}

	if strings.Contains(errStr, "413") || strings.Contains(strings.ToLower(errStr), "too large") {
		return &HTTPError{StatusCode: 413, Status: "Payload Too Large", Message: errStr}
	}

	if strings.Contains(errStr, "429") || strings.Contains(strings.ToLower(errStr), "too many requests") {
		return &HTTPError{StatusCode: 429, Status: "Too Many Requests", Message: errStr}
	}

	if strings.Contains(errStr, "500") || strings.Contains(strings.ToLower(errStr), "internal server") {
		return &HTTPError{StatusCode: 500, Status: "Internal Server Error", Message: errStr}
	}

	if strings.Contains(errStr, "503") || strings.Contains(strings.ToLower(errStr), "service unavailable") {
		return &HTTPError{StatusCode: 503, Status: "Service Unavailable", Message: errStr}
	}

	// Try to parse status code from error message
	if statusCode := parseStatusCodeFromMessage(errStr); statusCode > 0 {
		return &HTTPError{
			StatusCode: statusCode,
			Status:     http.StatusText(statusCode),
			Message:    errStr,
		}
	}

	return nil
}

// parseStatusCodeFromMessage attempts to extract HTTP status code from error message
func parseStatusCodeFromMessage(errStr string) int {
	// Look for patterns like "status code: 404" or "HTTP 404"
	patterns := []string{
		"status code: ",
		"status code ",
		"HTTP ",
		"http ",
	}

	for _, pattern := range patterns {
		if idx := strings.Index(strings.ToLower(errStr), pattern); idx >= 0 {
			start := idx + len(pattern)
			if start < len(errStr) {
				// Extract the number after the pattern
				numStr := ""
				for i := start; i < len(errStr) && len(numStr) < 3; i++ {
					if errStr[i] >= '0' && errStr[i] <= '9' {
						numStr += string(errStr[i])
					} else if len(numStr) > 0 {
						break
					}
				}

				if code, err := strconv.Atoi(numStr); err == nil && code >= 100 && code <= 599 {
					return code
				}
			}
		}
	}

	return 0
}

// mapHTTPError maps HTTP errors to domain errors
func mapHTTPError(httpErr *HTTPError, op, key string) error {
	switch httpErr.StatusCode {
	case 404:
		return &storagex.StorageError{
			Op:  op,
			Key: key,
			Err: storagex.ErrNotFound,
		}

	case 403:
		return &storagex.StorageError{
			Op:  op,
			Key: key,
			Err: fmt.Errorf("access denied: %w", storagex.ErrInvalidConfig),
		}

	case 409:
		return &storagex.StorageError{
			Op:  op,
			Key: key,
			Err: storagex.ErrConflict,
		}

	case 413:
		return &storagex.StorageError{
			Op:  op,
			Key: key,
			Err: storagex.ErrTooLarge,
		}

	case 429:
		return &storagex.StorageError{
			Op:  op,
			Key: key,
			Err: fmt.Errorf("rate limited: %w", storagex.ErrTimeout),
		}

	case 500, 502, 503, 504:
		return &storagex.StorageError{
			Op:  op,
			Key: key,
			Err: fmt.Errorf("server error (%d): %w", httpErr.StatusCode, storagex.ErrTimeout),
		}

	default:
		return &storagex.StorageError{
			Op:  op,
			Key: key,
			Err: fmt.Errorf("HTTP %d: %s", httpErr.StatusCode, httpErr.Message),
		}
	}
}

// AWSError represents a generic AWS API error
type AWSError struct {
	Code    string
	Message string
}

// extractAWSError attempts to extract AWS error information
func extractAWSError(err error) *AWSError {
	// This is a simplified implementation. The AWS SDK v2 has specific
	// error interfaces that should be used in production.
	errStr := err.Error()

	// Look for AWS error codes in the error message
	awsCodes := map[string]string{
		"NoSuchBucket":            "Bucket does not exist",
		"NoSuchKey":               "Object does not exist",
		"BucketAlreadyExists":     "Bucket already exists",
		"BucketAlreadyOwnedByYou": "Bucket already owned by you",
		"InvalidBucketName":       "Invalid bucket name",
		"AccessDenied":            "Access denied",
		"InvalidAccessKeyId":      "Invalid access key",
		"SignatureDoesNotMatch":   "Invalid secret key",
		"TokenRefreshRequired":    "Token refresh required",
		"RequestTimeTooSkewed":    "Request time too skewed",
		"EntityTooLarge":          "Entity too large",
		"InvalidPart":             "Invalid multipart upload part",
		"InvalidPartOrder":        "Invalid part order",
		"NoSuchUpload":            "Multipart upload does not exist",
		"MalformedXML":            "Malformed request",
		"InvalidRequest":          "Invalid request",
		"ServiceUnavailable":      "Service unavailable",
		"InternalError":           "Internal server error",
		"SlowDown":                "Reduce request rate",
	}

	for code, message := range awsCodes {
		if strings.Contains(errStr, code) {
			return &AWSError{Code: code, Message: message}
		}
	}

	return nil
}

// mapAWSError maps AWS API errors to domain errors
func mapAWSError(awsErr *AWSError, op, key string) error {
	switch awsErr.Code {
	case "NoSuchBucket", "NoSuchKey":
		return &storagex.StorageError{
			Op:  op,
			Key: key,
			Err: storagex.ErrNotFound,
		}

	case "BucketAlreadyExists", "BucketAlreadyOwnedByYou":
		return &storagex.StorageError{
			Op:  op,
			Key: key,
			Err: storagex.ErrConflict,
		}

	case "InvalidBucketName", "AccessDenied", "InvalidAccessKeyId",
		"SignatureDoesNotMatch", "MalformedXML", "InvalidRequest":
		return &storagex.StorageError{
			Op:  op,
			Key: key,
			Err: fmt.Errorf("%s: %w", awsErr.Message, storagex.ErrInvalidConfig),
		}

	case "EntityTooLarge":
		return &storagex.StorageError{
			Op:  op,
			Key: key,
			Err: storagex.ErrTooLarge,
		}

	case "TokenRefreshRequired", "RequestTimeTooSkewed", "SlowDown",
		"ServiceUnavailable", "InternalError":
		return &storagex.StorageError{
			Op:  op,
			Key: key,
			Err: fmt.Errorf("%s: %w", awsErr.Message, storagex.ErrTimeout),
		}

	case "InvalidPart", "InvalidPartOrder", "NoSuchUpload":
		return &storagex.StorageError{
			Op:  op,
			Key: key,
			Err: fmt.Errorf("multipart upload error: %s: %w", awsErr.Message, storagex.ErrAborted),
		}

	default:
		return &storagex.StorageError{
			Op:  op,
			Key: key,
			Err: fmt.Errorf("AWS error %s: %s", awsErr.Code, awsErr.Message),
		}
	}
}

// mapByErrorMessage performs string-based error matching as a fallback
func mapByErrorMessage(err error, op, key string) error {
	errStr := strings.ToLower(err.Error())

	// Common error patterns
	notFoundPatterns := []string{
		"not found",
		"does not exist",
		"no such",
		"nosuchkey",
		"nosuchbucket",
	}

	for _, pattern := range notFoundPatterns {
		if strings.Contains(errStr, pattern) {
			return &storagex.StorageError{
				Op:  op,
				Key: key,
				Err: storagex.ErrNotFound,
			}
		}
	}

	conflictPatterns := []string{
		"already exists",
		"conflict",
		"bucketalreadyexists",
	}

	for _, pattern := range conflictPatterns {
		if strings.Contains(errStr, pattern) {
			return &storagex.StorageError{
				Op:  op,
				Key: key,
				Err: storagex.ErrConflict,
			}
		}
	}

	timeoutPatterns := []string{
		"timeout",
		"deadline exceeded",
		"context canceled",
		"request timeout",
		"service unavailable",
	}

	for _, pattern := range timeoutPatterns {
		if strings.Contains(errStr, pattern) {
			return &storagex.StorageError{
				Op:  op,
				Key: key,
				Err: storagex.ErrTimeout,
			}
		}
	}

	tooLargePatterns := []string{
		"too large",
		"entity too large",
		"file too large",
		"exceeds maximum",
	}

	for _, pattern := range tooLargePatterns {
		if strings.Contains(errStr, pattern) {
			return &storagex.StorageError{
				Op:  op,
				Key: key,
				Err: storagex.ErrTooLarge,
			}
		}
	}

	return nil // No mapping found
}

// IsRetryableError determines if an error should be retried
func IsRetryableError(err error) bool {
	if err == nil {
		return false
	}

	// Don't retry context cancellation
	if errors.Is(err, context.Canceled) {
		return false
	}

	// Don't retry validation errors
	if errors.Is(err, storagex.ErrInvalidConfig) ||
		errors.Is(err, storagex.ErrInvalidKey) {
		return false
	}

	// Don't retry not found errors
	if errors.Is(err, storagex.ErrNotFound) {
		return false
	}

	// Don't retry conflict errors (usually)
	if errors.Is(err, storagex.ErrConflict) {
		return false
	}

	// Retry timeout and server errors
	if errors.Is(err, storagex.ErrTimeout) {
		return true
	}

	// Check HTTP status codes for retryability
	if httpErr := extractHTTPError(err); httpErr != nil {
		switch httpErr.StatusCode {
		case 429, 500, 502, 503, 504: // Rate limit or server errors
			return true
		case 400, 401, 403, 404, 409: // Client errors
			return false
		}
	}

	// Check AWS error codes for retryability
	if awsErr := extractAWSError(err); awsErr != nil {
		switch awsErr.Code {
		case "ServiceUnavailable", "InternalError", "SlowDown", "RequestTimeout":
			return true
		case "AccessDenied", "InvalidAccessKeyId", "SignatureDoesNotMatch",
			"NoSuchBucket", "NoSuchKey", "InvalidBucketName":
			return false
		}
	}

	// Default to retryable for unknown errors
	return true
}

// WrapError creates a StorageError with context
func WrapError(err error, op, key string) error {
	if err == nil {
		return nil
	}

	// If it's already a StorageError, don't double-wrap
	var storageErr *storagex.StorageError
	if errors.As(err, &storageErr) {
		return err
	}

	return &storagex.StorageError{
		Op:  op,
		Key: key,
		Err: err,
	}
}
