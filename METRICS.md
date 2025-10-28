# StorageX Metrics Reference

Complete reference for all metrics emitted by StorageX when observability is enabled.

## Overview

StorageX provides comprehensive metrics for monitoring storage operations, performance, and errors. All metrics follow Prometheus naming conventions and include labels for filtering and aggregation.

## Enabling Metrics

Include `metricsx.Module()` in your application:

```go
app := core.New(
    storagex.Module(),
    s3.Module(),
    metricsx.Module(), // ← Enables metrics
)
```

Metrics are exposed at the `/metrics` endpoint (default port 9090).

## Metrics Catalog

### Operation Metrics

#### `storage_operations_total`

**Type:** Counter  
**Labels:** `operation`, `status`  
**Description:** Total number of storage operations by type and result status.

**Labels:**
- `operation`: Operation type (`put`, `get`, `head`, `list`, `delete`, `delete_batch`, `multipart`, `presign_get`, `presign_put`)
- `status`: Result status (`success`, `error`)

**Example Queries:**
```promql
# Total operations per second
rate(storage_operations_total[5m])

# Error rate by operation
rate(storage_operations_total{status="error"}[5m]) 
  / rate(storage_operations_total[5m])

# Most common operations
topk(5, sum by (operation) (rate(storage_operations_total[5m])))
```

**Interpretation:**
- Sudden spikes may indicate batch processing or user activity
- High error rates require investigation (see troubleshooting)
- Uneven distribution may indicate inefficient access patterns

---

#### `storage_operation_duration_seconds`

**Type:** Histogram  
**Labels:** `operation`  
**Buckets:** `.001, .005, .01, .025, .05, .1, .25, .5, 1, 2.5, 5, 10` (seconds)  
**Description:** Duration of storage operations in seconds.

**Labels:**
- `operation`: Operation type (same as above)

**Example Queries:**
```promql
# P95 latency by operation
histogram_quantile(0.95, 
  rate(storage_operation_duration_seconds_bucket[5m]))

# P99 latency for uploads
histogram_quantile(0.99, 
  rate(storage_operation_duration_seconds_bucket{operation="put"}[5m]))

# Average operation time
rate(storage_operation_duration_seconds_sum[5m]) 
  / rate(storage_operation_duration_seconds_count[5m])
```

**Interpretation:**
- **< 100ms:** Excellent (cached or small objects)
- **100ms - 1s:** Good (typical S3 latency)
- **1s - 5s:** Acceptable (large files or slow network)
- **> 5s:** Investigate (network issues, throttling, or oversized objects)

**Bucket Guidance:**
- `.001` - `.01s`: Cached operations, metadata calls
- `.01` - `.1s`: Small object operations (< 1MB)
- `.1` - `1s`: Medium objects (1-10MB)
- `1` - `10s`: Large objects or multipart uploads

---

#### `storage_operation_bytes`

**Type:** Histogram  
**Labels:** `operation`  
**Buckets:** `1024, 10240, 102400, 1024000, 10240000, 104857600, 1073741824` (bytes)  
**Description:** Size of data transferred in storage operations.

**Bucket Labels (approximate):**
- `1024`: 1KB
- `10240`: 10KB
- `102400`: 100KB
- `1024000`: 1MB
- `10240000`: 10MB
- `104857600`: 100MB
- `1073741824`: 1GB

**Example Queries:**
```promql
# Average object size
rate(storage_operation_bytes_sum[5m]) 
  / rate(storage_operation_bytes_count[5m])

# P95 upload size
histogram_quantile(0.95, 
  rate(storage_operation_bytes_bucket{operation="put"}[5m]))

# Total data transferred per hour
increase(storage_operation_bytes_sum[1h])
```

**Interpretation:**
- Use to identify large file uploads/downloads
- Monitor data transfer costs (cloud providers charge per GB)
- Tune multipart settings based on typical sizes

---

### Multipart Upload Metrics

#### `storage_multipart_operations_total`

**Type:** Counter  
**Labels:** `operation`  
**Description:** Total number of multipart upload operations.

**Labels:**
- `operation`: Multipart operation type (`create`, `upload_part`, `complete`, `abort`)

**Example Queries:**
```promql
# Multipart upload initiation rate
rate(storage_multipart_operations_total{operation="create"}[5m])

# Abort rate (indicates failures or cancellations)
rate(storage_multipart_operations_total{operation="abort"}[5m])

# Average parts per upload
rate(storage_multipart_operations_total{operation="upload_part"}[5m]) 
  / rate(storage_multipart_operations_total{operation="create"}[5m])
```

**Interpretation:**
- High abort rate suggests configuration issues or client cancellations
- Many parts per upload may indicate inefficient part sizing

---

#### `storage_multipart_parts_total`

**Type:** Counter  
**Description:** Total number of parts uploaded in multipart operations.

**Example Queries:**
```promql
# Parts uploaded per second
rate(storage_multipart_parts_total[5m])

# Average parts per completed upload
increase(storage_multipart_parts_total[1h]) 
  / increase(storage_multipart_operations_total{operation="complete"}[1h])
```

**Interpretation:**
- Typical uploads: 5-20 parts (40MB - 160MB with 8MB parts)
- Very high part counts (> 100) suggest part size is too small
- Consider increasing `default_part_size` if consistently high

---

### List Operation Metrics

#### `storage_list_items`

**Type:** Histogram  
**Buckets:** `1, 10, 50, 100, 500, 1000, 5000, 10000`  
**Description:** Number of items returned in list operations.

**Example Queries:**
```promql
# P95 list size
histogram_quantile(0.95, rate(storage_list_items_bucket[5m]))

# Average items per list
rate(storage_list_items_sum[5m]) / rate(storage_list_items_count[5m])
```

**Interpretation:**
- Small counts (< 100): Normal prefix-based queries
- Large counts (> 1000): May indicate missing pagination
- Very large (> 5000): Consider adding prefix filters

---

#### `storage_list_truncated_total`

**Type:** Counter  
**Description:** Number of list operations that were truncated (more results available).

**Example Queries:**
```promql
# Truncation rate
rate(storage_list_truncated_total[5m])

# Percentage of truncated lists
rate(storage_list_truncated_total[5m]) 
  / rate(storage_operations_total{operation="list"}[5m]) * 100
```

**Interpretation:**
- High truncation rate suggests:
  - Need for pagination handling in client code
  - Overly broad prefix queries
  - Large directories that should be partitioned

---

### Batch Operation Metrics

#### `storage_batch_operation_size`

**Type:** Histogram  
**Labels:** `operation`  
**Buckets:** `1, 5, 10, 25, 50, 100, 250, 500, 1000`  
**Description:** Number of items in batch operations.

**Example Queries:**
```promql
# P95 batch size
histogram_quantile(0.95, 
  rate(storage_batch_operation_size_bucket[5m]))

# Average batch size
rate(storage_batch_operation_size_sum[5m]) 
  / rate(storage_batch_operation_size_count[5m])
```

**Interpretation:**
- Optimal batch size: 50-250 items (balances throughput and error handling)
- Very small batches (< 10): Consider batching more items
- Very large batches (> 500): Risk of timeouts

---

#### `storage_batch_operation_failures_total`

**Type:** Counter  
**Labels:** `operation`  
**Description:** Number of failed items in batch operations.

**Example Queries:**
```promql
# Batch failure rate
rate(storage_batch_operation_failures_total[5m])

# Failure percentage
rate(storage_batch_operation_failures_total[5m]) 
  / rate(storage_batch_operation_size_sum[5m]) * 100
```

**Interpretation:**
- Any failures require investigation (partial batch success)
- High failure rate may indicate:
  - Permission issues
  - Non-existent keys
  - Rate limiting

---

### Presigned URL Metrics

#### `storage_presign_operations_total`

**Type:** Counter  
**Labels:** `operation`  
**Description:** Number of presigned URL generation operations.

**Labels:**
- `operation`: Presign type (`get`, `put`)

**Example Queries:**
```promql
# Presign generation rate
rate(storage_presign_operations_total[5m])

# Ratio of presigned uploads to direct uploads
rate(storage_presign_operations_total{operation="put"}[5m]) 
  / rate(storage_operations_total{operation="put"}[5m])
```

**Interpretation:**
- Track presigned URL usage for security auditing
- High presign rate suggests direct client uploads (good architecture)

---

## Alerting Rules

### Recommended Prometheus Alerts

```yaml
groups:
  - name: storagex_alerts
    interval: 30s
    rules:
      # High error rate
      - alert: StorageHighErrorRate
        expr: |
          rate(storage_operations_total{status="error"}[5m]) 
          / rate(storage_operations_total[5m]) > 0.05
        for: 5m
        labels:
          severity: warning
        annotations:
          summary: "Storage error rate above 5%"
          description: "{{ $labels.operation }} operations failing at {{ $value | humanizePercentage }}"

      # Slow operations
      - alert: StorageSlowOperations
        expr: |
          histogram_quantile(0.95, 
            rate(storage_operation_duration_seconds_bucket[5m])) > 5
        for: 10m
        labels:
          severity: warning
        annotations:
          summary: "Storage operations are slow"
          description: "P95 latency is {{ $value }}s"

      # High multipart abort rate
      - alert: StorageMultipartAborts
        expr: |
          rate(storage_multipart_operations_total{operation="abort"}[5m]) 
          / rate(storage_multipart_operations_total{operation="create"}[5m]) > 0.1
        for: 5m
        labels:
          severity: warning
        annotations:
          summary: "High multipart upload abort rate"
          description: "{{ $value | humanizePercentage }} of uploads are being aborted"

      # Batch operation failures
      - alert: StorageBatchFailures
        expr: |
          rate(storage_batch_operation_failures_total[5m]) > 0
        for: 5m
        labels:
          severity: warning
        annotations:
          summary: "Batch operations experiencing failures"
          description: "{{ $value }} items failing per second"
```

---

## Grafana Dashboard

### Example Dashboard Panels

**Operations Overview:**
```promql
# Total operations/sec by type
sum by (operation) (rate(storage_operations_total[5m]))
```

**Error Rate:**
```promql
# Error rate percentage
sum(rate(storage_operations_total{status="error"}[5m])) 
  / sum(rate(storage_operations_total[5m])) * 100
```

**Latency Distribution:**
```promql
# P50, P95, P99 latencies
histogram_quantile(0.50, rate(storage_operation_duration_seconds_bucket[5m]))
histogram_quantile(0.95, rate(storage_operation_duration_seconds_bucket[5m]))
histogram_quantile(0.99, rate(storage_operation_duration_seconds_bucket[5m]))
```

**Data Transfer:**
```promql
# Upload vs download bandwidth
sum(rate(storage_operation_bytes_sum{operation="put"}[5m]))
sum(rate(storage_operation_bytes_sum{operation="get"}[5m]))
```

---

## Best Practices

1. **Baseline Metrics**: Establish baseline values in staging before production
2. **Alert Tuning**: Start with conservative thresholds and refine based on actual patterns
3. **Correlation**: Cross-reference storage metrics with application and infrastructure metrics
4. **Retention**: Keep high-resolution metrics for 24h, aggregated for 30d
5. **Cardinality**: Monitor metric cardinality if using custom labels (avoid high-cardinality values like object keys)

---

## See Also

- [PERFORMANCE.md](PERFORMANCE.md) - Performance tuning using metrics
- [TROUBLESHOOTING.md](TROUBLESHOOTING.md) - Diagnosing issues with metrics
- [observability.go](observability.go) - Metrics implementation source
