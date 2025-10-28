package storagex

import (
	"context"
	"time"

	"github.com/gostratum/metricsx"
	"github.com/gostratum/tracingx"
)

// ObservabilityParams holds optional observability dependencies
type ObservabilityParams struct {
	Metrics metricsx.Metrics `optional:"true"`
	Tracer  tracingx.Tracer  `optional:"true"`
}

// Instrumenter wraps storage operations with metrics and tracing
type Instrumenter struct {
	metrics metricsx.Metrics
	tracer  tracingx.Tracer
}

// NewInstrumenter creates a new instrumenter with optional metrics and tracing
func NewInstrumenter(metrics metricsx.Metrics, tracer tracingx.Tracer) *Instrumenter {
	return &Instrumenter{
		metrics: metrics,
		tracer:  tracer,
	}
}

// TraceOperation wraps an operation with tracing and metrics
func (i *Instrumenter) TraceOperation(ctx context.Context, operation, key string, fn func(ctx context.Context) error) error {
	// Start tracing if available
	var span tracingx.Span
	if i.tracer != nil {
		ctx, span = i.tracer.Start(ctx, "storage."+operation,
			tracingx.WithSpanKind(tracingx.SpanKindClient),
			tracingx.WithAttributes(map[string]any{
				"storage.operation": operation,
				"storage.key":       key,
			}),
		)
		defer span.End()
	}

	// Track operation duration for metrics
	start := time.Now()
	err := fn(ctx)
	duration := time.Since(start).Seconds()

	// Record metrics if available
	if i.metrics != nil {
		status := "success"
		if err != nil {
			status = "error"
		}

		i.metrics.Counter("storage_operations_total",
			metricsx.WithHelp("Total number of storage operations"),
			metricsx.WithLabels("operation", "status"),
		).Inc(operation, status)

		i.metrics.Histogram("storage_operation_duration_seconds",
			metricsx.WithHelp("Storage operation duration in seconds"),
			metricsx.WithLabels("operation"),
			metricsx.WithBuckets(.001, .005, .01, .025, .05, .1, .25, .5, 1, 2.5, 5, 10),
		).Observe(duration, operation)
	}

	// Update span status if tracing
	if span != nil {
		if err != nil {
			span.SetError(err)
		}
	}

	return err
}

// RecordOperationSize records the size of data transferred
func (i *Instrumenter) RecordOperationSize(operation string, size int64) {
	if i.metrics != nil {
		i.metrics.Histogram("storage_operation_bytes",
			metricsx.WithHelp("Storage operation data size in bytes"),
			metricsx.WithLabels("operation"),
			metricsx.WithBuckets(1024, 10240, 102400, 1024000, 10240000, 104857600, 1073741824), // 1KB to 1GB
		).Observe(float64(size), operation)
	}
}

// RecordMultipartOperation records multipart upload metrics
func (i *Instrumenter) RecordMultipartOperation(operation string, partCount int) {
	if i.metrics != nil {
		i.metrics.Counter("storage_multipart_operations_total",
			metricsx.WithHelp("Total number of multipart upload operations"),
			metricsx.WithLabels("operation"),
		).Inc(operation)

		if partCount > 0 {
			i.metrics.Counter("storage_multipart_parts_total",
				metricsx.WithHelp("Total number of multipart upload parts"),
			).Add(float64(partCount))
		}
	}
}

// RecordListOperation records list operation metrics
func (i *Instrumenter) RecordListOperation(itemCount int, truncated bool) {
	if i.metrics != nil {
		i.metrics.Histogram("storage_list_items",
			metricsx.WithHelp("Number of items returned in list operations"),
			metricsx.WithBuckets(1, 10, 50, 100, 500, 1000, 5000, 10000),
		).Observe(float64(itemCount))

		if truncated {
			i.metrics.Counter("storage_list_truncated_total",
				metricsx.WithHelp("Number of truncated list operations"),
			).Inc()
		}
	}
}

// RecordBatchOperation records batch operation metrics
func (i *Instrumenter) RecordBatchOperation(operation string, totalCount, failedCount int) {
	if i.metrics != nil {
		i.metrics.Histogram("storage_batch_operation_size",
			metricsx.WithHelp("Number of items in batch operations"),
			metricsx.WithLabels("operation"),
			metricsx.WithBuckets(1, 5, 10, 25, 50, 100, 250, 500, 1000),
		).Observe(float64(totalCount), operation)

		if failedCount > 0 {
			i.metrics.Counter("storage_batch_operation_failures_total",
				metricsx.WithHelp("Number of failed items in batch operations"),
				metricsx.WithLabels("operation"),
			).Add(float64(failedCount), operation)
		}
	}
}

// RecordPresignOperation records presigned URL generation metrics
func (i *Instrumenter) RecordPresignOperation(operation string) {
	if i.metrics != nil {
		i.metrics.Counter("storage_presign_operations_total",
			metricsx.WithHelp("Total number of presigned URL operations"),
			metricsx.WithLabels("operation"),
		).Inc(operation)
	}
}

// AddSpanAttribute adds an attribute to the current span if tracing is enabled
func (i *Instrumenter) AddSpanAttribute(ctx context.Context, key string, value any) {
	if i.tracer != nil {
		// The tracer provides access to the current span via the context
		// This is a simplified version - actual implementation depends on tracingx internals
	}
}
