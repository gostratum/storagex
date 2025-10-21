package storagex_test

import (
	"context"
	"io"
	"testing"

	"github.com/gostratum/core/logx"
	"github.com/gostratum/storagex"
	"github.com/stretchr/testify/require"
	"go.uber.org/fx"
	"go.uber.org/fx/fxtest"
)

// mockStorage is a tiny in-test implementation of storagex.Storage that
// avoids any network calls. Useful for lifecycle/unit tests.
type mockStorage struct{}

func (m *mockStorage) Put(ctx context.Context, key string, r io.Reader, opts *storagex.PutOptions) (storagex.Stat, error) {
	return storagex.Stat{Key: key, Size: 0}, nil
}
func (m *mockStorage) PutBytes(ctx context.Context, key string, data []byte, opts *storagex.PutOptions) (storagex.Stat, error) {
	return storagex.Stat{Key: key, Size: int64(len(data))}, nil
}
func (m *mockStorage) PutFile(ctx context.Context, key string, path string, opts *storagex.PutOptions) (storagex.Stat, error) {
	return storagex.Stat{Key: key, Size: 0}, nil
}
func (m *mockStorage) Get(ctx context.Context, key string) (storagex.ReaderAtCloser, storagex.Stat, error) {
	return nil, storagex.Stat{Key: key, Size: 0}, storagex.ErrNotFound
}
func (m *mockStorage) Head(ctx context.Context, key string) (storagex.Stat, error) {
	return storagex.Stat{}, storagex.ErrNotFound
}
func (m *mockStorage) List(ctx context.Context, opts storagex.ListOptions) (storagex.ListPage, error) {
	return storagex.ListPage{}, nil
}
func (m *mockStorage) Delete(ctx context.Context, key string) error { return nil }
func (m *mockStorage) DeleteBatch(ctx context.Context, keys []string) ([]string, error) {
	return nil, nil
}
func (m *mockStorage) MultipartUpload(ctx context.Context, key string, src io.Reader, cfg *storagex.MultipartConfig, putOpts *storagex.PutOptions) (storagex.Stat, error) {
	return storagex.Stat{}, nil
}
func (m *mockStorage) CreateMultipart(ctx context.Context, key string, putOpts *storagex.PutOptions) (string, error) {
	return "", nil
}
func (m *mockStorage) UploadPart(ctx context.Context, key, uploadID string, partNumber int32, part io.Reader, size int64) (string, error) {
	return "", nil
}
func (m *mockStorage) CompleteMultipart(ctx context.Context, key, uploadID string, etags []string) (storagex.Stat, error) {
	return storagex.Stat{}, nil
}
func (m *mockStorage) AbortMultipart(ctx context.Context, key, uploadID string) error { return nil }
func (m *mockStorage) PresignGet(ctx context.Context, key string, opts *storagex.PresignOptions) (string, error) {
	return "", nil
}
func (m *mockStorage) PresignPut(ctx context.Context, key string, opts *storagex.PresignOptions) (string, error) {
	return "", nil
}

func TestModuleLifecycleProvidesStorage(t *testing.T) {
	app := fxtest.New(t,
		fx.Options(
			storagex.TestModule,
			fx.Provide(func() storagex.Storage { return &mockStorage{} }),
			fx.Provide(func() logx.Logger { return logx.NewNoopLogger() }),
		),
		fx.Invoke(func(s storagex.Storage) {
			require.NotNil(t, s)
		}),
	)

	defer app.RequireStart().RequireStop()
}
