package storagex

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/gostratum/metricsx"
	"github.com/gostratum/tracingx"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockMetrics implements metricsx.Metrics for testing
type mockMetrics struct {
	mu         sync.Mutex
	counters   map[string]float64
	histograms map[string][]float64
}

func newMockMetrics() *mockMetrics {
	return &mockMetrics{
		counters:   make(map[string]float64),
		histograms: make(map[string][]float64),
	}
}

func (m *mockMetrics) Counter(name string, opts ...metricsx.Option) metricsx.Counter {
	return &mockCounter{metrics: m, name: name}
}

func (m *mockMetrics) Gauge(name string, opts ...metricsx.Option) metricsx.Gauge {
	return &mockGauge{metrics: m, name: name}
}

func (m *mockMetrics) Histogram(name string, opts ...metricsx.Option) metricsx.Histogram {
	return &mockHistogram{metrics: m, name: name}
}

func (m *mockMetrics) Summary(name string, opts ...metricsx.Option) metricsx.Summary {
	return &mockSummary{metrics: m, name: name}
}

type mockCounter struct {
	metrics *mockMetrics
	name    string
}

func (c *mockCounter) Inc(labels ...string) {
	c.Add(1, labels...)
}

func (c *mockCounter) Add(value float64, labels ...string) {
	c.metrics.mu.Lock()
	defer c.metrics.mu.Unlock()
	key := c.name + ":" + joinLabels(labels)
	c.metrics.counters[key] += value
}

type mockHistogram struct {
	metrics *mockMetrics
	name    string
}

func (h *mockHistogram) Observe(value float64, labels ...string) {
	h.metrics.mu.Lock()
	defer h.metrics.mu.Unlock()
	key := h.name + ":" + joinLabels(labels)
	h.metrics.histograms[key] = append(h.metrics.histograms[key], value)
}

func (h *mockHistogram) Timer(labels ...string) metricsx.Timer {
	return &mockTimer{start: time.Now()}
}

type mockGauge struct {
	metrics *mockMetrics
	name    string
}

func (g *mockGauge) Set(value float64, labels ...string) {}
func (g *mockGauge) Inc(labels ...string)                {}
func (g *mockGauge) Dec(labels ...string)                {}
func (g *mockGauge) Add(value float64, labels ...string) {}
func (g *mockGauge) Sub(value float64, labels ...string) {}

type mockSummary struct {
	metrics *mockMetrics
	name    string
}

func (s *mockSummary) Observe(value float64, labels ...string) {}

type mockTimer struct {
	start time.Time
}

func (t *mockTimer) ObserveDuration() {}

func (t *mockTimer) Stop() time.Duration {
	return time.Since(t.start)
}

// mockTracer implements tracingx.Tracer for testing
type mockTracer struct {
	mu    sync.Mutex
	spans []*mockSpan
}

func newMockTracer() *mockTracer {
	return &mockTracer{
		spans: make([]*mockSpan, 0),
	}
}

func (t *mockTracer) Start(ctx context.Context, operationName string, opts ...tracingx.SpanOption) (context.Context, tracingx.Span) {
	span := &mockSpan{
		operationName: operationName,
		tags:          make(map[string]any),
		ended:         false,
	}

	// Apply options
	cfg := &tracingx.SpanConfig{
		Attributes: make(map[string]any),
	}
	for _, opt := range opts {
		opt(cfg)
	}
	for k, v := range cfg.Attributes {
		span.tags[k] = v
	}

	t.mu.Lock()
	t.spans = append(t.spans, span)
	t.mu.Unlock()

	return ctx, span
}

func (t *mockTracer) Extract(ctx context.Context, carrier any) (context.Context, error) {
	return ctx, nil
}

func (t *mockTracer) Inject(ctx context.Context, carrier any) error {
	return nil
}

func (t *mockTracer) Shutdown(ctx context.Context) error {
	return nil
}

type mockSpan struct {
	operationName string
	tags          map[string]any
	error         error
	ended         bool
}

func (s *mockSpan) End() {
	s.ended = true
}

func (s *mockSpan) SetTag(key string, value any) {
	s.tags[key] = value
}

func (s *mockSpan) SetError(err error) {
	s.error = err
}

func (s *mockSpan) LogFields(fields ...tracingx.Field) {}

func (s *mockSpan) Context() context.Context {
	return context.Background()
}

func (s *mockSpan) TraceID() string {
	return "mock-trace-id"
}

func (s *mockSpan) SpanID() string {
	return "mock-span-id"
}

func joinLabels(labels []string) string {
	result := ""
	for _, label := range labels {
		if result != "" {
			result += ","
		}
		result += label
	}
	return result
}

func TestNewInstrumenter(t *testing.T) {
	t.Run("creates instrumenter with metrics and tracer", func(t *testing.T) {
		metrics := newMockMetrics()
		tracer := newMockTracer()

		instrumenter := NewInstrumenter(metrics, tracer)

		assert.NotNil(t, instrumenter)
		assert.Equal(t, metrics, instrumenter.metrics)
		assert.Equal(t, tracer, instrumenter.tracer)
	})

	t.Run("creates instrumenter with nil metrics and tracer", func(t *testing.T) {
		instrumenter := NewInstrumenter(nil, nil)

		assert.NotNil(t, instrumenter)
		assert.Nil(t, instrumenter.metrics)
		assert.Nil(t, instrumenter.tracer)
	})
}

func TestTraceOperation(t *testing.T) {
	t.Run("successful operation with metrics and tracing", func(t *testing.T) {
		metrics := newMockMetrics()
		tracer := newMockTracer()
		instrumenter := NewInstrumenter(metrics, tracer)

		ctx := context.Background()
		called := false

		err := instrumenter.TraceOperation(ctx, "put", "test-key", func(ctx context.Context) error {
			called = true
			return nil
		})

		require.NoError(t, err)
		assert.True(t, called)

		// Check metrics
		assert.Equal(t, 1.0, metrics.counters["storage_operations_total:put,success"])
		assert.Len(t, metrics.histograms["storage_operation_duration_seconds:put"], 1)

		// Check tracing
		assert.Len(t, tracer.spans, 1)
		span := tracer.spans[0]
		assert.Equal(t, "storage.put", span.operationName)
		assert.Equal(t, "put", span.tags["storage.operation"])
		assert.Equal(t, "test-key", span.tags["storage.key"])
		assert.True(t, span.ended)
		assert.Nil(t, span.error)
	})

	t.Run("failed operation records error", func(t *testing.T) {
		metrics := newMockMetrics()
		tracer := newMockTracer()
		instrumenter := NewInstrumenter(metrics, tracer)

		ctx := context.Background()
		testErr := errors.New("test error")

		err := instrumenter.TraceOperation(ctx, "get", "test-key", func(ctx context.Context) error {
			return testErr
		})

		require.Error(t, err)
		assert.Equal(t, testErr, err)

		// Check metrics
		assert.Equal(t, 1.0, metrics.counters["storage_operations_total:get,error"])

		// Check tracing
		assert.Len(t, tracer.spans, 1)
		span := tracer.spans[0]
		assert.Equal(t, testErr, span.error)
	})

	t.Run("works without metrics", func(t *testing.T) {
		tracer := newMockTracer()
		instrumenter := NewInstrumenter(nil, tracer)

		ctx := context.Background()
		err := instrumenter.TraceOperation(ctx, "delete", "test-key", func(ctx context.Context) error {
			return nil
		})

		require.NoError(t, err)
		assert.Len(t, tracer.spans, 1)
	})

	t.Run("works without tracer", func(t *testing.T) {
		metrics := newMockMetrics()
		instrumenter := NewInstrumenter(metrics, nil)

		ctx := context.Background()
		err := instrumenter.TraceOperation(ctx, "list", "", func(ctx context.Context) error {
			return nil
		})

		require.NoError(t, err)
		assert.Equal(t, 1.0, metrics.counters["storage_operations_total:list,success"])
	})
}

func TestRecordOperationSize(t *testing.T) {
	t.Run("records operation size", func(t *testing.T) {
		metrics := newMockMetrics()
		instrumenter := NewInstrumenter(metrics, nil)

		instrumenter.RecordOperationSize("put", 1024)
		instrumenter.RecordOperationSize("get", 2048)

		assert.Len(t, metrics.histograms["storage_operation_bytes:put"], 1)
		assert.Equal(t, 1024.0, metrics.histograms["storage_operation_bytes:put"][0])

		assert.Len(t, metrics.histograms["storage_operation_bytes:get"], 1)
		assert.Equal(t, 2048.0, metrics.histograms["storage_operation_bytes:get"][0])
	})

	t.Run("no-op without metrics", func(t *testing.T) {
		instrumenter := NewInstrumenter(nil, nil)

		// Should not panic
		instrumenter.RecordOperationSize("put", 1024)
	})
}

func TestRecordMultipartOperation(t *testing.T) {
	t.Run("records multipart operation", func(t *testing.T) {
		metrics := newMockMetrics()
		instrumenter := NewInstrumenter(metrics, nil)

		instrumenter.RecordMultipartOperation("create", 0)
		instrumenter.RecordMultipartOperation("upload_part", 5)
		instrumenter.RecordMultipartOperation("complete", 0)

		assert.Equal(t, 1.0, metrics.counters["storage_multipart_operations_total:create"])
		assert.Equal(t, 1.0, metrics.counters["storage_multipart_operations_total:upload_part"])
		assert.Equal(t, 1.0, metrics.counters["storage_multipart_operations_total:complete"])
		assert.Equal(t, 5.0, metrics.counters["storage_multipart_parts_total:"])
	})
}

func TestRecordListOperation(t *testing.T) {
	t.Run("records list operation", func(t *testing.T) {
		metrics := newMockMetrics()
		instrumenter := NewInstrumenter(metrics, nil)

		instrumenter.RecordListOperation(50, false)
		instrumenter.RecordListOperation(1000, true)

		assert.Len(t, metrics.histograms["storage_list_items:"], 2)
		assert.Equal(t, 50.0, metrics.histograms["storage_list_items:"][0])
		assert.Equal(t, 1000.0, metrics.histograms["storage_list_items:"][1])
		assert.Equal(t, 1.0, metrics.counters["storage_list_truncated_total:"])
	})
}

func TestRecordBatchOperation(t *testing.T) {
	t.Run("records batch operation with failures", func(t *testing.T) {
		metrics := newMockMetrics()
		instrumenter := NewInstrumenter(metrics, nil)

		instrumenter.RecordBatchOperation("delete", 100, 5)

		assert.Len(t, metrics.histograms["storage_batch_operation_size:delete"], 1)
		assert.Equal(t, 100.0, metrics.histograms["storage_batch_operation_size:delete"][0])
		assert.Equal(t, 5.0, metrics.counters["storage_batch_operation_failures_total:delete"])
	})

	t.Run("records batch operation without failures", func(t *testing.T) {
		metrics := newMockMetrics()
		instrumenter := NewInstrumenter(metrics, nil)

		instrumenter.RecordBatchOperation("delete", 50, 0)

		assert.Len(t, metrics.histograms["storage_batch_operation_size:delete"], 1)
		assert.Equal(t, 50.0, metrics.histograms["storage_batch_operation_size:delete"][0])
		assert.Equal(t, 0.0, metrics.counters["storage_batch_operation_failures_total:delete"])
	})
}

func TestRecordPresignOperation(t *testing.T) {
	t.Run("records presign operations", func(t *testing.T) {
		metrics := newMockMetrics()
		instrumenter := NewInstrumenter(metrics, nil)

		instrumenter.RecordPresignOperation("get")
		instrumenter.RecordPresignOperation("put")

		assert.Equal(t, 1.0, metrics.counters["storage_presign_operations_total:get"])
		assert.Equal(t, 1.0, metrics.counters["storage_presign_operations_total:put"])
	})
}
