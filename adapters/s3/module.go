package s3

import (
	"context"
	"io"
	"sync"

	"github.com/gostratum/core"
	"github.com/gostratum/core/logx"
	"github.com/gostratum/storagex"
	"go.uber.org/fx"
)

// Module returns an fx.Module which provides the S3 storage implementation.
// Consumers should opt-in this module explicitly (e.g. s3.Module()) instead
// of relying on package init side-effects.
func Module() fx.Option {
	return fx.Module("storagex-s3",
		fx.Provide(
			provideS3Storage,
		),
		// Provide a health checker for the S3 client (optional)
		fx.Provide(
			fx.Annotated{
				Target: func(cm *ClientManager) core.Check {
					return &s3HealthCheck{client: cm}
				},
				Group: "health_checkers",
			},
		),
	)
}

// provideS3Storage is an fx-friendly constructor that creates an S3 storage
// instance. It accepts optional key builder and logger from the FX graph.
func provideS3Storage(lc fx.Lifecycle, cfg *storagex.Config, kb storagex.KeyBuilder, logger logx.Logger) (storagex.Storage, error) {
	var opts []storagex.Option
	if kb != nil {
		opts = append(opts, storagex.WithKeyBuilder(kb))
	}
	if logger != nil {
		opts = append(opts, storagex.WithLogger(logger))
	}

	// We'll create the real storage during OnStart so we can use the lifecycle
	// context and respect cancellation/timeouts. Meanwhile return a proxy that
	// blocks calls until the real storage is ready or returns an error if
	// startup failed.
	proxy := &lifecycleProxy{}

	lc.Append(fx.Hook{
		OnStart: func(ctx context.Context) error {
			s, err := NewS3Storage(ctx, cfg, opts...)
			if err != nil {
				proxy.setErr(err)
				return err
			}
			proxy.setStorage(s)
			return nil
		},
		OnStop: func(ctx context.Context) error {
			// Close underlying storage if it implements Close
			if proxy.storage != nil {
				if closer, ok := proxy.storage.(interface{ Close() error }); ok {
					return closer.Close()
				}
			}
			return nil
		},
	})

	return proxy, nil
}

// lifecycleProxy is a Storage implementation that waits for the real storage
// to be created during the FX OnStart hook. It returns an error if startup
// failed.
type lifecycleProxy struct {
	mu      sync.RWMutex
	storage storagex.Storage
	err     error
	ready   chan struct{}
}

func (p *lifecycleProxy) init() {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.ready == nil {
		p.ready = make(chan struct{})
	}
}

func (p *lifecycleProxy) setStorage(s storagex.Storage) {
	p.init()
	p.mu.Lock()
	p.storage = s
	close(p.ready)
	p.mu.Unlock()
}

func (p *lifecycleProxy) setErr(err error) {
	p.init()
	p.mu.Lock()
	p.err = err
	close(p.ready)
	p.mu.Unlock()
}

func (p *lifecycleProxy) wait() error {
	p.init()
	<-p.ready
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.err
}

func (p *lifecycleProxy) get() storagex.Storage {
	p.mu.RLock()
	s := p.storage
	p.mu.RUnlock()
	return s
}

// The following methods implement the storagex.Storage interface by
// delegating to the underlying storage after startup completes.
func (p *lifecycleProxy) Put(ctx context.Context, key string, r io.Reader, opts *storagex.PutOptions) (storagex.Stat, error) {
	if err := p.wait(); err != nil {
		return storagex.Stat{}, err
	}
	return p.get().Put(ctx, key, r, opts)
}

// Other interface methods follow similar pattern. For brevity, implement a
// small subset and let the compiler enforce the rest during build.
// In this repo we must implement all methods; implement them by delegating.
func (p *lifecycleProxy) PutBytes(ctx context.Context, key string, data []byte, opts *storagex.PutOptions) (storagex.Stat, error) {
	if err := p.wait(); err != nil {
		return storagex.Stat{}, err
	}
	return p.get().PutBytes(ctx, key, data, opts)
}

func (p *lifecycleProxy) PutFile(ctx context.Context, key string, path string, opts *storagex.PutOptions) (storagex.Stat, error) {
	if err := p.wait(); err != nil {
		return storagex.Stat{}, err
	}
	return p.get().PutFile(ctx, key, path, opts)
}

func (p *lifecycleProxy) Get(ctx context.Context, key string) (storagex.ReaderAtCloser, storagex.Stat, error) {
	if err := p.wait(); err != nil {
		return nil, storagex.Stat{}, err
	}
	return p.get().Get(ctx, key)
}

func (p *lifecycleProxy) Head(ctx context.Context, key string) (storagex.Stat, error) {
	if err := p.wait(); err != nil {
		return storagex.Stat{}, err
	}
	return p.get().Head(ctx, key)
}

func (p *lifecycleProxy) List(ctx context.Context, opts storagex.ListOptions) (storagex.ListPage, error) {
	if err := p.wait(); err != nil {
		return storagex.ListPage{}, err
	}
	return p.get().List(ctx, opts)
}

func (p *lifecycleProxy) Delete(ctx context.Context, key string) error {
	if err := p.wait(); err != nil {
		return err
	}
	return p.get().Delete(ctx, key)
}

func (p *lifecycleProxy) DeleteBatch(ctx context.Context, keys []string) ([]string, error) {
	if err := p.wait(); err != nil {
		return nil, err
	}
	return p.get().DeleteBatch(ctx, keys)
}

func (p *lifecycleProxy) MultipartUpload(ctx context.Context, key string, src io.Reader, cfg *storagex.MultipartConfig, putOpts *storagex.PutOptions) (storagex.Stat, error) {
	if err := p.wait(); err != nil {
		return storagex.Stat{}, err
	}
	return p.get().MultipartUpload(ctx, key, src, cfg, putOpts)
}

func (p *lifecycleProxy) CreateMultipart(ctx context.Context, key string, putOpts *storagex.PutOptions) (string, error) {
	if err := p.wait(); err != nil {
		return "", err
	}
	return p.get().CreateMultipart(ctx, key, putOpts)
}

func (p *lifecycleProxy) UploadPart(ctx context.Context, key, uploadID string, partNumber int32, part io.Reader, size int64) (string, error) {
	if err := p.wait(); err != nil {
		return "", err
	}
	return p.get().UploadPart(ctx, key, uploadID, partNumber, part, size)
}

func (p *lifecycleProxy) CompleteMultipart(ctx context.Context, key, uploadID string, etags []string) (storagex.Stat, error) {
	if err := p.wait(); err != nil {
		return storagex.Stat{}, err
	}
	return p.get().CompleteMultipart(ctx, key, uploadID, etags)
}

func (p *lifecycleProxy) AbortMultipart(ctx context.Context, key, uploadID string) error {
	if err := p.wait(); err != nil {
		return err
	}
	return p.get().AbortMultipart(ctx, key, uploadID)
}

func (p *lifecycleProxy) PresignGet(ctx context.Context, key string, opts *storagex.PresignOptions) (string, error) {
	if err := p.wait(); err != nil {
		return "", err
	}
	return p.get().PresignGet(ctx, key, opts)
}

func (p *lifecycleProxy) PresignPut(ctx context.Context, key string, opts *storagex.PresignOptions) (string, error) {
	if err := p.wait(); err != nil {
		return "", err
	}
	return p.get().PresignPut(ctx, key, opts)
}
