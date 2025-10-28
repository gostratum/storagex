# StorageX Troubleshooting Guide

Comprehensive guide to diagnosing and resolving common StorageX issues using metrics, traces, and logs.

## Overview

This guide helps you identify and fix common problems with StorageX. Each issue includes:
- **Symptoms** - How to recognize the problem
- **Diagnosis** - Metrics queries to pinpoint the cause
- **Solution** - Step-by-step resolution
- **Prevention** - How to avoid in the future

---

## Quick Diagnostic Checklist

Before diving into specific issues, run this quick health check:

```promql
# 1. Error rate (should be < 1%)
rate(storage_operations_total{status="error"}[5m]) 
  / rate(storage_operations_total[5m]) * 100

# 2. P95 latency (should be < 2s for most operations)
histogram_quantile(0.95, rate(storage_operation_duration_seconds_bucket[5m]))

# 3. Recent errors
increase(storage_operations_total{status="error"}[1h])

# 4. Multipart abort rate (should be < 5%)
rate(storage_multipart_operations_total{operation="abort"}[5m]) 
  / rate(storage_multipart_operations_total{operation="create"}[5m]) * 100
```

---

## Common Issues

### Issue 1: "Operation Timed Out"

**Symptoms:**
- Context deadline exceeded errors
- Operations fail after configured timeout
- Metrics show high P99 latency

**Diagnosis:**

```promql
# Check timeout frequency
rate(storage_operations_total{status="error"}[5m])

# Identify slow operations
topk(5, 
  histogram_quantile(0.99, 
    sum by (operation) (rate(storage_operation_duration_seconds_bucket[5m]))
  )
)

# Check file sizes for slow operations
histogram_quantile(0.95, rate(storage_operation_bytes_bucket[5m]))
```

**Common Causes:**

| Cause | Detection | Solution |
|-------|-----------|----------|
| Timeout too short for file size | Large files + short timeout | Increase `request_timeout` |
| Network latency | All operations slow | Check network connectivity; use closer region |
| S3 throttling | Sporadic slowness | Add backoff; reduce request rate |
| Large multipart uploads | Only multipart slow | Increase `default_parallel` |

**Solution Steps:**

1. **Temporary Fix - Increase timeout:**
```yaml
storage:
  request_timeout: 300s  # 5 minutes (was 30s)
```

2. **Long-term Fix - Optimize uploads:**
```yaml
storage:
  default_part_size: 16777216  # 16MB
  default_parallel: 8           # More concurrency
```

3. **Code Fix - Stream large files:**
```go
// Instead of buffering
storage.Put(ctx, key, reader, nil)  // Streams data

// Or use multipart explicitly
storage.MultipartUpload(ctx, key, reader, &storagex.MultipartConfig{
    PartSize:    10 * 1024 * 1024,
    Concurrency: 10,
})
```

**Prevention:**
- Set timeouts based on expected file sizes
- Monitor P99 latency and adjust proactively
- Use multipart for files > 50MB

---

### Issue 2: "Access Denied" / 403 Errors

**Symptoms:**
- Operations fail with permission errors
- Metrics show consistent errors for specific operations
- Works in development but fails in production

**Diagnosis:**

```promql
# Check which operations are failing
sum by (operation) (rate(storage_operations_total{status="error"}[5m]))

# Recent 403 errors (check logs for this pattern)
# Look for "AccessDenied" in application logs
```

**Common Causes:**

| Cause | Detection | Solution |
|-------|-----------|----------|
| Missing IAM permissions | Specific operations fail | Add required S3 permissions |
| Bucket policy restrictions | All operations fail | Review bucket policy |
| Wrong credentials | All operations fail | Verify access/secret keys |
| Cross-account access | Works for some buckets | Configure role assumption |

**Solution Steps:**

1. **Verify credentials:**
```bash
# Check if credentials are loaded
aws s3 ls s3://your-bucket --profile your-profile

# Test with AWS CLI
aws s3 cp test.txt s3://your-bucket/ --profile your-profile
```

2. **Review IAM policy** - Minimum required permissions:
```json
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Effect": "Allow",
      "Action": [
        "s3:GetObject",
        "s3:PutObject",
        "s3:DeleteObject",
        "s3:ListBucket",
        "s3:GetBucketLocation"
      ],
      "Resource": [
        "arn:aws:s3:::your-bucket",
        "arn:aws:s3:::your-bucket/*"
      ]
    }
  ]
}
```

3. **For multipart uploads, add:**
```json
{
  "Action": [
    "s3:AbortMultipartUpload",
    "s3:ListMultipartUploadParts"
  ],
  "Resource": "arn:aws:s3:::your-bucket/*"
}
```

4. **Configuration for role assumption:**
```yaml
storage:
  use_sdk_defaults: true
  role_arn: "arn:aws:iam::123456789012:role/StorageRole"
```

**Prevention:**
- Use IAM roles instead of static credentials
- Implement least-privilege access
- Test permissions in staging before production
- Monitor for 403 errors and alert

---

### Issue 3: "Object Not Found" / 404 Errors

**Symptoms:**
- Get operations fail with `ErrNotFound`
- Inconsistent results (works sometimes, fails others)
- Metrics show errors on specific keys

**Diagnosis:**

```promql
# Check 404 error rate
rate(storage_operations_total{operation="get",status="error"}[5m])

# Compare to total get operations
rate(storage_operations_total{operation="get"}[5m])
```

**Common Causes:**

| Cause | Detection | Solution |
|-------|-----------|----------|
| Key doesn't exist | Consistent 404 for same key | Add existence check with `Head` |
| Wrong bucket/prefix | All operations fail | Verify bucket and prefix config |
| Race condition | Intermittent failures | Add retry with backoff |
| Eventual consistency | Fails right after Put | Add delay or retry logic |

**Solution Steps:**

1. **Add existence check:**
```go
// Check before downloading
stat, err := storage.Head(ctx, key)
if err != nil {
    if errors.Is(err, storagex.ErrNotFound) {
        return fmt.Errorf("file does not exist: %s", key)
    }
    return err
}

// Now safe to download
reader, _, err := storage.Get(ctx, key)
```

2. **Handle not found gracefully:**
```go
reader, _, err := storage.Get(ctx, key)
if err != nil {
    if errors.Is(err, storagex.ErrNotFound) {
        // Handle missing file (use default, prompt user, etc.)
        return useDefaultFile()
    }
    return err
}
```

3. **Retry for eventual consistency:**
```go
var reader storagex.ReaderAtCloser
var stat storagex.Stat
err := retry.Do(func() error {
    var getErr error
    reader, stat, getErr = storage.Get(ctx, key)
    return getErr
}, retry.Attempts(3), retry.Delay(100*time.Millisecond))
```

**Prevention:**
- Always check errors with `errors.Is(err, storagex.ErrNotFound)`
- Verify keys exist before critical operations
- Implement retry logic for read-after-write scenarios
- Use consistent key generation logic

---

### Issue 4: High Memory Usage During Uploads

**Symptoms:**
- Memory usage spikes during uploads
- Out-of-memory errors
- Application slowness during file operations

**Diagnosis:**

```promql
# Check upload sizes
histogram_quantile(0.95, rate(storage_operation_bytes_bucket{operation="put"}[5m]))

# Monitor multipart usage
rate(storage_multipart_operations_total{operation="create"}[5m]) 
  / rate(storage_operations_total{operation="put"}[5m])
```

**Common Causes:**

| Cause | Detection | Solution |
|-------|-----------|----------|
| Buffering entire file | Large files, high memory | Use streaming |
| Not using multipart | Large files as single PUT | Enable multipart |
| Too many concurrent uploads | Many parallel operations | Limit concurrency |
| Memory leak | Gradual memory growth | Check for unclosed readers |

**Solution Steps:**

1. **Use streaming instead of buffering:**

❌ **Bad (buffers entire file):**
```go
data, _ := io.ReadAll(file)
storage.PutBytes(ctx, key, data, nil)  // Loads entire file into memory
```

✅ **Good (streams):**
```go
storage.Put(ctx, key, file, nil)  // Streams from reader
```

2. **Enable multipart for large files:**
```go
// Automatic multipart for files > threshold
storage.PutFile(ctx, key, localPath, &storagex.PutOptions{
    // Automatically uses multipart if file > 50MB
})

// Or explicit multipart
if fileSize > 50*1024*1024 {
    storage.MultipartUpload(ctx, key, file, &storagex.MultipartConfig{
        PartSize:    10 * 1024 * 1024,  // 10MB parts
        Concurrency: 4,
    })
}
```

3. **Limit concurrent uploads:**
```go
// Use semaphore to limit concurrency
sem := make(chan struct{}, 5)  // Max 5 concurrent uploads

for _, file := range files {
    sem <- struct{}{}  // Acquire
    go func(f string) {
        defer func() { <-sem }()  // Release
        storage.PutFile(ctx, key, f, nil)
    }(file)
}
```

4. **Always close readers:**
```go
reader, _, err := storage.Get(ctx, key)
if err != nil {
    return err
}
defer reader.Close()  // ← CRITICAL: Prevents memory leak

// Process reader...
```

**Prevention:**
- Always use streaming for large files
- Set appropriate multipart thresholds
- Profile memory usage during development
- Implement upload concurrency limits

---

### Issue 5: Slow List Operations

**Symptoms:**
- List operations take > 2 seconds
- High latency for directory listings
- Timeouts on list calls

**Diagnosis:**

```promql
# P95 list latency
histogram_quantile(0.95, 
  rate(storage_operation_duration_seconds_bucket{operation="list"}[5m]))

# Average items per list
rate(storage_list_items_sum[5m]) / rate(storage_list_items_count[5m])

# Truncation rate
rate(storage_list_truncated_total[5m]) / rate(storage_operations_total{operation="list"}[5m])
```

**Common Causes:**

| Cause | Detection | Solution |
|-------|-----------|----------|
| Listing entire bucket | High item count | Use specific prefixes |
| Missing pagination | High truncation rate | Implement pagination |
| Many objects in prefix | > 1000 items | Partition data with subprefixes |
| No delimiter | Listing all nested objects | Use delimiter for directory view |

**Solution Steps:**

1. **Use specific prefixes:**

❌ **Bad (lists everything):**
```go
page, _ := storage.List(ctx, storagex.ListOptions{})
```

✅ **Good (targeted query):**
```go
page, _ := storage.List(ctx, storagex.ListOptions{
    Prefix: "users/12345/documents/2024/",
    PageSize: 100,
})
```

2. **Implement pagination:**
```go
var allKeys []storagex.Stat
opts := storagex.ListOptions{
    Prefix:   "users/",
    PageSize: 1000,
}

for {
    page, err := storage.List(ctx, opts)
    if err != nil {
        return err
    }
    
    allKeys = append(allKeys, page.Keys...)
    
    if !page.IsTruncated {
        break
    }
    
    opts.StartAfter = page.NextMarker
}
```

3. **Use delimiter for directory-like listing:**
```go
// List "directories" (common prefixes)
page, _ := storage.List(ctx, storagex.ListOptions{
    Prefix:    "photos/",
    Delimiter: "/",
})

// page.CommonPrefixes contains "directories"
for _, prefix := range page.CommonPrefixes {
    fmt.Println("Directory:", prefix)
}
```

4. **Partition data structure:**

Instead of:
```
photos/photo1.jpg
photos/photo2.jpg
photos/photo3.jpg
... (10,000 files)
```

Use date-based partitioning:
```
photos/2024/01/01/photo1.jpg
photos/2024/01/01/photo2.jpg
photos/2024/01/02/photo3.jpg
```

**Prevention:**
- Design key structure with prefixes from the start
- Never list entire buckets in production
- Implement pagination by default
- Monitor list operation sizes

---

### Issue 6: Multipart Upload Failures

**Symptoms:**
- High multipart abort rate
- Uploads start but never complete
- "Incomplete multipart upload" errors

**Diagnosis:**

```promql
# Abort rate
rate(storage_multipart_operations_total{operation="abort"}[5m]) 
  / rate(storage_multipart_operations_total{operation="create"}[5m])

# Average parts per upload
rate(storage_multipart_operations_total{operation="upload_part"}[5m]) 
  / rate(storage_multipart_operations_total{operation="create"}[5m])

# Completion rate
rate(storage_multipart_operations_total{operation="complete"}[5m]) 
  / rate(storage_multipart_operations_total{operation="create"}[5m])
```

**Common Causes:**

| Cause | Detection | Solution |
|-------|-----------|----------|
| Context cancellation | Aborts correlate with timeouts | Increase context timeout |
| Network failures | Random aborts | Add retry on individual parts |
| Part size too small | > 50 parts per upload | Increase part size |
| Concurrency issues | Completion rate < 50% | Check for race conditions |

**Solution Steps:**

1. **Increase timeout for large uploads:**
```go
ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
defer cancel()

storage.MultipartUpload(ctx, key, reader, &storagex.MultipartConfig{
    PartSize:    10 * 1024 * 1024,
    Concurrency: 8,
})
```

2. **Cleanup incomplete uploads:**
```go
// Periodic cleanup job
func cleanupIncompleteUploads(ctx context.Context) {
    // List all multipart uploads
    uploads, err := s3Client.ListMultipartUploads(ctx, &s3.ListMultipartUploadsInput{
        Bucket: aws.String(bucket),
    })
    
    for _, upload := range uploads.Uploads {
        // Abort uploads older than 24 hours
        if upload.Initiated.Before(time.Now().Add(-24 * time.Hour)) {
            s3Client.AbortMultipartUpload(ctx, &s3.AbortMultipartUploadInput{
                Bucket:   aws.String(bucket),
                Key:      upload.Key,
                UploadId: upload.UploadId,
            })
        }
    }
}
```

3. **Tune part size:**
```go
// For 1GB file:
// Bad: 1024 parts of 1MB each → overhead + risk
// Good: 64 parts of 16MB each → efficient

fileSize := getFileSize(file)
partSize := 16 * 1024 * 1024  // 16MB

if fileSize > 5*1024*1024*1024 {  // > 5GB
    partSize = 100 * 1024 * 1024  // Use 100MB parts
}

storage.MultipartUpload(ctx, key, file, &storagex.MultipartConfig{
    PartSize:    partSize,
    Concurrency: 8,
})
```

**Prevention:**
- Set appropriate timeouts based on file size
- Implement cleanup jobs for orphaned uploads
- Monitor abort rates and alert
- Use appropriate part sizes (8-100MB)

---

### Issue 7: Distributed Tracing Not Working

**Symptoms:**
- No traces appear in Jaeger/tracing backend
- Spans not connected across services
- Missing trace context

**Diagnosis:**

1. **Check if tracing module is included:**
```go
// Should include tracingx.Module()
app := core.New(
    storagex.Module(),
    s3.Module(),
    tracingx.Module(),  // ← Must be present
)
```

2. **Verify tracer configuration:**
```yaml
tracing:
  provider: otlp
  otlp:
    endpoint: http://localhost:4318
    insecure: true
```

3. **Check logs for tracing errors:**
```bash
grep -i "tracing" application.log
grep -i "span" application.log
```

**Common Causes:**

| Cause | Detection | Solution |
|-------|-----------|----------|
| Module not included | No traces at all | Add `tracingx.Module()` |
| Wrong endpoint | Connection errors in logs | Verify OTLP endpoint |
| Context not propagated | Disconnected spans | Pass context through call chain |
| Sampling disabled | Some traces missing | Check sampling configuration |

**Solution Steps:**

1. **Enable tracing:**
```go
app := core.New(
    storagex.Module(),
    s3.Module(),
    metricsx.Module(),
    tracingx.Module(),  // Add this
)
```

2. **Propagate context:**
```go
// ❌ Bad: Creates new context
func handler(w http.ResponseWriter, r *http.Request) {
    ctx := context.Background()  // Lost trace context!
    storage.Get(ctx, key)
}

// ✅ Good: Uses request context
func handler(w http.ResponseWriter, r *http.Request) {
    ctx := r.Context()  // Preserves trace context
    storage.Get(ctx, key)
}
```

3. **Verify tracing backend:**
```bash
# Test OTLP endpoint
curl http://localhost:4318/v1/traces

# Check Jaeger UI
open http://localhost:16686
```

**Prevention:**
- Always pass context through entire call chain
- Test tracing in development before production
- Monitor trace collection rates
- Use consistent context propagation patterns

---

## Using Traces for Debugging

### Example Trace Analysis

**Problem:** Upload taking 30 seconds

**Trace:**
```
storage.put [30.2s]
  ├─ storage.multipart.create [0.1s]
  ├─ storage.multipart.upload_part [5.2s] ← Slow!
  ├─ storage.multipart.upload_part [5.1s] ← Slow!
  ├─ storage.multipart.upload_part [5.3s] ← Slow!
  ├─ storage.multipart.upload_part [5.2s] ← Slow!
  └─ storage.multipart.complete [0.3s]
```

**Analysis:**
- Each part taking ~5s suggests network bottleneck
- Parts uploaded sequentially (should be parallel!)

**Solution:**
- Increase `default_parallel` from 4 to 8
- Investigate network latency to S3 endpoint

---

## Escalation Path

If issues persist after following this guide:

1. **Gather diagnostics:**
   - Metrics screenshots (last 1 hour)
   - Recent error logs
   - Trace examples
   - Configuration file

2. **Check GitHub Issues:**
   - Search existing issues
   - Look for similar symptoms

3. **Create support ticket with:**
   - StorageX version
   - Go version
   - S3 provider (AWS/MinIO/other)
   - Minimal reproduction case

4. **Include metrics queries:**
```promql
# Error rate
rate(storage_operations_total{status="error"}[5m])

# Latency
histogram_quantile(0.95, rate(storage_operation_duration_seconds_bucket[5m]))

# Recent operations
increase(storage_operations_total[1h])
```

---

## See Also

- [METRICS.md](METRICS.md) - Complete metrics reference
- [PERFORMANCE.md](PERFORMANCE.md) - Performance tuning guide
- [README.md](README.md) - General documentation
- [OBSERVABILITY.md](OBSERVABILITY.md) - Observability setup guide
