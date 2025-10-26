package testutil_test

import (
	"bytes"
	"context"
	"io"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/gostratum/storagex"
	"github.com/gostratum/storagex/internal/testutil"
)

func TestMockStorage_BasicOperations(t *testing.T) {
	storage := testutil.NewMockStorage()
	ctx := context.Background()

	t.Run("Put and Get", func(t *testing.T) {
		testData := []byte("hello world")
		stat, err := storage.PutBytes(ctx, "test/file.txt", testData, nil)
		require.NoError(t, err)
		assert.Equal(t, int64(len(testData)), stat.Size)
		assert.NotEmpty(t, stat.ETag)

		reader, getStat, err := storage.Get(ctx, "test/file.txt")
		require.NoError(t, err)
		defer reader.Close()

		assert.Equal(t, stat.Size, getStat.Size)
		assert.Equal(t, stat.ETag, getStat.ETag)

		data, err := io.ReadAll(reader)
		require.NoError(t, err)
		assert.Equal(t, testData, data)
	})

	t.Run("Head", func(t *testing.T) {
		testData := []byte("metadata test")
		opts := &storagex.PutOptions{
			ContentType: "text/plain",
			Metadata: map[string]string{
				"custom-key": "custom-value",
			},
		}

		stat, err := storage.PutBytes(ctx, "test/metadata.txt", testData, opts)
		require.NoError(t, err)

		headStat, err := storage.Head(ctx, "test/metadata.txt")
		require.NoError(t, err)

		assert.Equal(t, stat.Size, headStat.Size)
		assert.Equal(t, stat.ETag, headStat.ETag)
		assert.Equal(t, "text/plain", headStat.ContentType)
		assert.Equal(t, "custom-value", headStat.Metadata["custom-key"])
	})

	t.Run("Delete", func(t *testing.T) {
		testData := []byte("to be deleted")
		_, err := storage.PutBytes(ctx, "test/delete.txt", testData, nil)
		require.NoError(t, err)

		// Verify it exists
		_, err = storage.Head(ctx, "test/delete.txt")
		require.NoError(t, err)

		// Delete it
		err = storage.Delete(ctx, "test/delete.txt")
		require.NoError(t, err)

		// Verify it's gone
		_, err = storage.Head(ctx, "test/delete.txt")
		assert.Error(t, err)
		assert.True(t, storagex.IsNotFound(err))
	})

	t.Run("Delete is idempotent", func(t *testing.T) {
		// Delete non-existent key should not error
		err := storage.Delete(ctx, "test/does-not-exist.txt")
		assert.NoError(t, err)
	})
}

func TestMockStorage_ListOperations(t *testing.T) {
	storage := testutil.NewMockStorage()
	ctx := context.Background()

	// Setup test data
	testFiles := map[string][]byte{
		"prefix/file1.txt":        []byte("data1"),
		"prefix/file2.txt":        []byte("data2"),
		"prefix/subdir/file3.txt": []byte("data3"),
		"other/file4.txt":         []byte("data4"),
	}

	for key, data := range testFiles {
		_, err := storage.PutBytes(ctx, key, data, nil)
		require.NoError(t, err)
	}

	t.Run("List with prefix", func(t *testing.T) {
		page, err := storage.List(ctx, storagex.ListOptions{
			Prefix:   "prefix/",
			PageSize: 10,
		})
		require.NoError(t, err)

		assert.Len(t, page.Keys, 3)
		assert.False(t, page.IsTruncated)

		// Verify all keys have the prefix
		for _, obj := range page.Keys {
			assert.Contains(t, obj.Key, "prefix/")
		}
	})

	t.Run("List with delimiter", func(t *testing.T) {
		page, err := storage.List(ctx, storagex.ListOptions{
			Prefix:    "prefix/",
			Delimiter: "/",
			PageSize:  10,
		})
		require.NoError(t, err)

		// Should have 2 files at this level and 1 common prefix (subdir/)
		assert.Len(t, page.Keys, 2)
		assert.Len(t, page.CommonPrefixes, 1)
		assert.Contains(t, page.CommonPrefixes, "prefix/subdir/")
	})

	t.Run("List with pagination", func(t *testing.T) {
		page, err := storage.List(ctx, storagex.ListOptions{
			Prefix:   "prefix/",
			PageSize: 2,
		})
		require.NoError(t, err)

		assert.Len(t, page.Keys, 2)
		assert.True(t, page.IsTruncated)
		assert.NotEmpty(t, page.NextToken)

		// Get next page
		page2, err := storage.List(ctx, storagex.ListOptions{
			Prefix:            "prefix/",
			PageSize:          2,
			ContinuationToken: page.NextToken,
		})
		require.NoError(t, err)

		assert.Len(t, page2.Keys, 1)
		assert.False(t, page2.IsTruncated)
	})
}

func TestMockStorage_DeleteBatch(t *testing.T) {
	storage := testutil.NewMockStorage()
	ctx := context.Background()

	// Setup test data
	keys := []string{"batch/file1.txt", "batch/file2.txt", "batch/file3.txt"}
	for _, key := range keys {
		_, err := storage.PutBytes(ctx, key, []byte("data"), nil)
		require.NoError(t, err)
	}

	// Delete batch
	failedKeys, err := storage.DeleteBatch(ctx, keys)
	require.NoError(t, err)
	assert.Empty(t, failedKeys)

	// Verify all deleted
	for _, key := range keys {
		_, err := storage.Head(ctx, key)
		assert.True(t, storagex.IsNotFound(err))
	}
}

func TestMockStorage_MultipartUpload(t *testing.T) {
	storage := testutil.NewMockStorage()
	ctx := context.Background()

	// Create large data (10MB)
	largeData := make([]byte, 10*1024*1024)
	for i := range largeData {
		largeData[i] = byte(i % 256)
	}

	stat, err := storage.MultipartUpload(ctx, "large/file.bin", bytes.NewReader(largeData),
		&storagex.MultipartConfig{
			PartSizeBytes: 5 * 1024 * 1024,
			Concurrency:   2,
		}, nil)
	require.NoError(t, err)
	assert.Equal(t, int64(len(largeData)), stat.Size)

	// Verify we can retrieve it
	reader, _, err := storage.Get(ctx, "large/file.bin")
	require.NoError(t, err)
	defer reader.Close()

	retrievedData, err := io.ReadAll(reader)
	require.NoError(t, err)
	assert.Equal(t, largeData, retrievedData)
}

func TestMockStorage_PresignedURLs(t *testing.T) {
	storage := testutil.NewMockStorage()
	ctx := context.Background()

	t.Run("PresignPut", func(t *testing.T) {
		url, err := storage.PresignPut(ctx, "test/presign.txt", &storagex.PresignOptions{})
		require.NoError(t, err)
		assert.Contains(t, url, "test/presign.txt")
		assert.Contains(t, url, "mock-storage.example.com")
	})

	t.Run("PresignGet", func(t *testing.T) {
		// PresignGet doesn't require object to exist
		url, err := storage.PresignGet(ctx, "test/presign.txt", &storagex.PresignOptions{})
		require.NoError(t, err)
		assert.Contains(t, url, "test/presign.txt")
		assert.Contains(t, url, "mock-storage.example.com")
	})
}

func TestMockStorage_ConflictHandling(t *testing.T) {
	storage := testutil.NewMockStorage()
	ctx := context.Background()

	// Put initial object
	_, err := storage.PutBytes(ctx, "test/conflict.txt", []byte("original"), nil)
	require.NoError(t, err)

	t.Run("Overwrite=true allows update", func(t *testing.T) {
		stat, err := storage.PutBytes(ctx, "test/conflict.txt", []byte("updated"),
			&storagex.PutOptions{Overwrite: true})
		require.NoError(t, err)
		assert.Equal(t, int64(7), stat.Size)

		// Verify update
		reader, _, err := storage.Get(ctx, "test/conflict.txt")
		require.NoError(t, err)
		defer reader.Close()

		data, _ := io.ReadAll(reader)
		assert.Equal(t, "updated", string(data))
	})

	t.Run("Overwrite=false returns conflict error", func(t *testing.T) {
		_, err := storage.PutBytes(ctx, "test/conflict.txt", []byte("another"),
			&storagex.PutOptions{Overwrite: false})
		require.Error(t, err)
		assert.ErrorIs(t, err, storagex.ErrConflict)
	})
}

func TestMockStorage_ContextCancellation(t *testing.T) {
	storage := testutil.NewMockStorage()

	t.Run("Cancelled context returns error", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel() // Cancel immediately

		_, err := storage.PutBytes(ctx, "test/cancelled.txt", []byte("data"), nil)
		require.Error(t, err)
		assert.ErrorIs(t, err, context.Canceled)
	})
}
