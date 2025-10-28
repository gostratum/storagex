# StorageX Performance Tuning Guide

Comprehensive guide to optimizing StorageX performance using metrics and configuration.

## Overview

StorageX provides extensive metrics to identify bottlenecks and tune configuration for optimal performance. This guide covers common performance scenarios and how to use metrics to diagnose and resolve them.

## Quick Wins

### 1. Enable Connection Pooling

**Configuration:**
```yaml
storage:
  max_retries: 3
  request_timeout: 30s
```

**Impact:** Reduces connection overhead for repeated operations.

### 2. Tune Multipart Upload Settings

**Default Configuration:**
```yaml
storage:
  default_part_size: 8388608  # 8MB
  default_parallel: 4         # 4 concurrent uploads
```

**Optimization Path:**
1. Check average upload size from metrics
2. Adjust part size based on typical file sizes
3. Increase parallelism based on available bandwidth

---

## Performance Scenarios

### Scenario 1: Slow Upload Performance

**Symptoms:**
```promql
# P95 upload latency > 5s
histogram_quantile(0.95, 
  rate(storage_operation_duration_seconds_bucket{operation="put"}[5m])) > 5
```

**Diagnosis Steps:**

1. **Check upload sizes:**
```promql
# P95 upload size
histogram_quantile(0.95, 
  rate(storage_operation_bytes_bucket{operation="put"}[5m]))
```

2. **Check if multipart is being used:**
```promql
# Ratio of multipart to regular uploads
rate(storage_multipart_operations_total{operation="create"}[5m]) 
  / rate(storage_operations_total{operation="put"}[5m])
```

**Solutions:**

| Observed Pattern | Recommended Action |
|-----------------|-------------------|
| Large files (> 50MB) not using multipart | Reduce `MultipartThreshold` in code |
| Many small parts (> 50 per upload) | Increase `default_part_size` to 16MB or 32MB |
| High latency per part | Increase `default_parallel` to 8 or 16 |
| Low parallelism utilization | Check network bandwidth; may be bottleneck |

**Example Configuration Change:**
```yaml
storage:
  default_part_size: 16777216  # 16MB (was 8MB)
  default_parallel: 8          # 8 concurrent (was 4)
```

**Expected Impact:**
- 2-3x faster uploads for files > 100MB
- Reduced total upload time by 40-60%

---

### Scenario 2: High List Operation Latency

**Symptoms:**
```promql
# P95 list latency > 2s
histogram_quantile(0.95, 
  rate(storage_operation_duration_seconds_bucket{operation="list"}[5m])) > 2
```

**Diagnosis Steps:**

1. **Check list sizes:**
```promql
# Average items per list
rate(storage_list_items_sum[5m]) / rate(storage_list_items_count[5m])
```

2. **Check truncation rate:**
```promql
# Percentage of truncated lists
rate(storage_list_truncated_total[5m]) 
  / rate(storage_operations_total{operation="list"}[5m]) * 100
```

**Solutions:**

| Metric Reading | Issue | Solution |
|---------------|-------|----------|
| High item count (> 1000) | Overly broad queries | Add more specific prefixes |
| High truncation rate (> 50%) | Missing pagination | Implement pagination in code |
| Low item count but slow | S3 latency | Check region/endpoint configuration |

**Code Optimization:**

Before (slow):
```go
// Lists entire bucket
page, _ := storage.List(ctx, storagex.ListOptions{})
```

After (fast):
```go
// Use specific prefix
page, _ := storage.List(ctx, storagex.ListOptions{
    Prefix:   "users/12345/documents/",
    PageSize: 100,
})
```

**Expected Impact:**
- 10-100x faster list operations
- Reduced S3 request costs

---

### Scenario 3: High Error Rates

**Symptoms:**
```promql
# Error rate > 5%
rate(storage_operations_total{status="error"}[5m]) 
  / rate(storage_operations_total[5m]) > 0.05
```

**Diagnosis Steps:**

1. **Identify failing operations:**
```promql
# Errors by operation type
sum by (operation) (rate(storage_operations_total{status="error"}[5m]))
```

2. **Check retry exhaustion:**
- If errors persist after retries, may need to increase `max_retries`

**Solutions:**

| Error Pattern | Likely Cause | Solution |
|--------------|--------------|----------|
| Timeout errors | Slow network or large files | Increase `request_timeout` |
| 503 errors | S3 throttling | Add exponential backoff; reduce request rate |
| 404 errors | Missing objects | Add existence checks before operations |
| 403 errors | Permission issues | Review IAM policies |

**Configuration for Throttling:**
```yaml
storage:
  max_retries: 5              # Increase retries (was 3)
  backoff_initial: 500ms      # Longer initial backoff (was 200ms)
  backoff_max: 30s            # Longer max backoff (was 5s)
```

**Expected Impact:**
- Reduced error rate from 5% to < 1%
- Better handling of temporary failures

---

### Scenario 4: High Multipart Abort Rate

**Symptoms:**
```promql
# Abort rate > 10%
rate(storage_multipart_operations_total{operation="abort"}[5m]) 
  / rate(storage_multipart_operations_total{operation="create"}[5m]) > 0.1
```

**Diagnosis Steps:**

1. **Check average parts per upload:**
```promql
rate(storage_multipart_operations_total{operation="upload_part"}[5m]) 
  / rate(storage_multipart_operations_total{operation="create"}[5m])
```

2. **Correlate with error logs** to identify abort reasons

**Common Causes & Solutions:**

| Abort Reason | Explanation | Solution |
|-------------|-------------|----------|
| Context cancellation | Client timeouts or user cancellation | Increase timeout; improve UX |
| Network failures | Unstable connections | Enable retry on individual parts |
| Part size too small | Too many parts causing overhead | Increase `default_part_size` |
| Memory pressure | OOM during upload | Stream data instead of buffering |

**Code Improvement (Streaming):**

Before (buffers entire file):
```go
data, _ := io.ReadAll(file)
storage.PutBytes(ctx, key, data, nil)
```

After (streams):
```go
storage.Put(ctx, key, file, nil)  // Streams from reader
```

---

### Scenario 5: Inconsistent Latency

**Symptoms:**
```promql
# High variance between P50 and P95
histogram_quantile(0.95, rate(storage_operation_duration_seconds_bucket[5m])) 
  / histogram_quantile(0.50, rate(storage_operation_duration_seconds_bucket[5m])) > 5
```

**Diagnosis Steps:**

1. **Check if cold start issue:**
- Monitor latency immediately after deployment vs steady state

2. **Check for bursty traffic:**
```promql
rate(storage_operations_total[1m])  # 1-minute granularity
```

**Solutions:**

| Pattern | Issue | Solution |
|---------|-------|----------|
| First request slow | Cold connection pool | Implement warmup logic |
| Periodic spikes | Batch jobs or cron | Spread load with jitter |
| Random spikes | S3 backend variability | Add client-side caching |

**Warmup Example:**
```go
func warmupStorage(ctx context.Context, storage storagex.Storage) {
    // Perform a dummy operation to initialize connections
    _, _ = storage.Head(ctx, "warmup-key")
}
```

---

## Configuration Matrix

### Small Files (< 1MB)

**Use Case:** Thumbnails, JSON configs, small documents

```yaml
storage:
  request_timeout: 10s
  max_retries: 3
  # No multipart needed - handled by single PUT
```

**Expected Performance:**
- P50: 50-100ms
- P95: 200-500ms

---

### Medium Files (1MB - 100MB)

**Use Case:** Documents, images, reports

```yaml
storage:
  request_timeout: 30s
  max_retries: 3
  default_part_size: 8388608   # 8MB
  default_parallel: 4
```

**Expected Performance:**
- P50: 200-500ms
- P95: 1-3s

---

### Large Files (100MB - 5GB)

**Use Case:** Videos, datasets, backups

```yaml
storage:
  request_timeout: 300s         # 5 minutes
  max_retries: 5
  default_part_size: 16777216   # 16MB
  default_parallel: 8
```

**Expected Performance:**
- P50: 5-30s
- P95: 30-90s

---

### Very Large Files (> 5GB)

**Use Case:** Database backups, media archives, ML datasets

```yaml
storage:
  request_timeout: 3600s        # 1 hour
  max_retries: 5
  default_part_size: 104857600  # 100MB
  default_parallel: 16
```

**Expected Performance:**
- Throughput: 50-200 MB/s (network dependent)
- Total time: Minutes to hours

**Code Configuration:**
```go
storage.MultipartUpload(ctx, key, reader, &storagex.MultipartConfig{
    PartSize:    100 * 1024 * 1024,  // 100MB
    Concurrency: 16,
})
```

---

## Monitoring Dashboard

### Essential Panels

**1. Throughput**
```promql
# Operations per second
sum(rate(storage_operations_total[5m]))
```

**2. Latency Percentiles**
```promql
histogram_quantile(0.50, rate(storage_operation_duration_seconds_bucket[5m]))
histogram_quantile(0.95, rate(storage_operation_duration_seconds_bucket[5m]))
histogram_quantile(0.99, rate(storage_operation_duration_seconds_bucket[5m]))
```

**3. Error Rate**
```promql
sum(rate(storage_operations_total{status="error"}[5m])) 
  / sum(rate(storage_operations_total[5m])) * 100
```

**4. Data Transfer Rate**
```promql
# MB/s
sum(rate(storage_operation_bytes_sum[5m])) / 1024 / 1024
```

**5. Multipart Efficiency**
```promql
# Average parts per upload
rate(storage_multipart_operations_total{operation="upload_part"}[5m]) 
  / rate(storage_multipart_operations_total{operation="create"}[5m])
```

---

## Benchmarking

### Running Benchmarks

Use the provided benchmark tool:

```bash
cd storagex
go test -bench=. -benchmem -benchtime=10s ./...
```

### Interpreting Results

```
BenchmarkPut_Small-8      1000  1205043 ns/op   2048 B/op   15 allocs/op
BenchmarkPut_Large-8        10  105234567 ns/op  8192 B/op   45 allocs/op
```

**Key Metrics:**
- `ns/op`: Nanoseconds per operation (lower is better)
- `B/op`: Bytes allocated per operation (lower is better)
- `allocs/op`: Number of allocations (lower is better)

**Targets:**
- Small files (< 1MB): < 10ms, < 5 allocs
- Large files (> 100MB): Throughput > 50 MB/s

---

## Cost Optimization

### Reducing S3 Costs

**1. Minimize List Operations**
- Use specific prefixes
- Cache list results when possible
- Implement pagination

**2. Optimize Multipart Uploads**
- Right-size parts (larger = fewer requests)
- Clean up failed uploads (abort multipart)

**3. Batch Operations**
- Use `DeleteBatch` instead of individual deletes
- Reduces request count by up to 1000x

**Example Savings:**

| Operation | Before | After | Savings |
|-----------|--------|-------|---------|
| Delete 1000 files | 1000 DELETE requests | 1 DeleteObjects request | 99.9% |
| List 5000 items | Multiple paginated calls | Single call with prefix | 80% |
| Upload 100MB file | 1 PUT request | 1 multipart (6 parts) | -500% cost, +300% speed |

---

## Advanced Tuning

### Connection Pool Configuration

For high-throughput scenarios, tune AWS SDK connection pool:

```go
// In S3 adapter configuration
cfg.HTTPClient = &http.Client{
    Transport: &http.Transport{
        MaxIdleConns:        100,
        MaxIdleConnsPerHost: 20,
        IdleConnTimeout:     90 * time.Second,
    },
}
```

### Regional Optimization

**Cross-Region Transfers:**
- Expect 2-5x higher latency
- Consider using S3 Transfer Acceleration
- Monitor with region-specific metrics

**Same-Region Transfers:**
- Expected latency: 10-50ms
- Use VPC endpoints for private connectivity
- Eliminate data transfer costs

---

## Troubleshooting Performance Issues

See [TROUBLESHOOTING.md](TROUBLESHOOTING.md) for detailed diagnostic procedures.

**Quick Checklist:**

- [ ] Check metrics for error rates and latency
- [ ] Verify network connectivity to S3 endpoint
- [ ] Review multipart configuration for file sizes
- [ ] Check for throttling (503 errors)
- [ ] Validate IAM permissions (403 errors)
- [ ] Monitor system resources (CPU, memory, network)
- [ ] Review application logs for context

---

## See Also

- [METRICS.md](METRICS.md) - Complete metrics reference
- [TROUBLESHOOTING.md](TROUBLESHOOTING.md) - Diagnostic procedures
- [README.md](README.md#configuration) - Configuration reference
