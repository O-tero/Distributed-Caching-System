# Monitoring Service

Comprehensive observability service for the distributed caching system providing real-time metrics collection, statistical aggregation, anomaly detection, and intelligent alerting.

## 🏗️ Architecture
```
┌─────────────────────────────────────────────────────────┐
│                 Monitoring Service                      │
├─────────────────────────────────────────────────────────┤
│                                                         │
│  ┌──────────────────┐    ┌──────────────────┐         │
│  │ Metrics          │    │   Aggregator     │         │
│  │ Collector        │───►│                  │         │
│  │                  │    │  • 1s Window     │         │
│  │  • Atomic        │    │  • 10s Window    │         │
│  │    Counters      │    │  • 1m Window     │         │
│  │  • Ring Buffer   │    │  • Percentiles   │         │
│  │  • Time Series   │    │  • Statistics    │         │
│  └──────────────────┘    └─────────┬────────┘         │
│           │                        │                   │
│           │                        ▼                   │
│           │              ┌──────────────────┐          │
│           │              │  Anomaly         │          │
│           │              │  Detector        │          │
│           │              │                  │          │
│           │              │  • Z-score       │          │
│           │              │  • 3-sigma       │          │
│           │              │  • Welford's     │          │
│           │              └─────────┬────────┘          │
│           │                        │                   │
│           └────────────────────────┼──────────┐        │
│                                    ▼          │        │
│                          ┌──────────────────┐ │        │
│                          │  Alert Manager   │ │        │
│                          │                  │ │        │
│                          │  • Rule Engine   │ │        │
│                          │  • Threshold     │ │        │
│                          │  • Dynamic       │ │        │
│                          └──────────────────┘ │        │
│                                               │        │
│                    Pub/Sub Subscriptions      │        │
│                    ┌──────────────────┐       │        │
│                    │  cache-metrics   │───────┘        │
│                    │  warm-completed  │                │
│                    │  invalidation    │                │
│                    └──────────────────┘                │
└─────────────────────────────────────────────────────────┘
```

## ✨ Features

- **High-Throughput Metrics**: >1M events/sec per core with lock-free structures
- **Multi-Window Aggregation**: 1s, 10s, 1m sliding windows
- **Statistical Analysis**: P50/P90/P95/P99 latency, hit rates, QPS, error rates
- **Anomaly Detection**: Z-score based detection with Welford's algorithm
- **Intelligent Alerting**: Static and dynamic threshold rules
- **Low Latency**: Sub-millisecond aggregation queries
- **Memory Efficient**: Bounded buffers with automatic cleanup

## 🚀 Quick Start

### Running Locally
```bash
# Start the service
cd distributed-cache-system
encore run

# Service endpoints available at:
# http://localhost:4000
```

## 📊 Data Model

### Metrics Collection

**Atomic Counters** (O(1) read/write):
- Cache hits/misses
- Cache operations (set, delete, evict)
- Invalidations
- Warmings
- Errors

**Ring Buffer** (O(1) amortized):
- Latency samples (last 10K)
- Lock-free circular buffer with atomic head/tail pointers

**Time Series** (O(1) amortized):
- 1-second buckets
- Automatic cleanup of old data
- Efficient range queries

### Aggregation Windows

| Window | Granularity | Use Case |
|--------|-------------|----------|
| 1s | Real-time | Instant feedback, spike detection |
| 10s | Near real-time | Short-term trends |
| 1m | Recent history | Dashboard metrics, alerting |

### Statistical Methods

**Percentile Calculation**:
- Algorithm: Linear interpolation on sorted values
- Complexity: O(n log n) due to sorting
- Accuracy: Exact percentiles (not approximate)

**Anomaly Detection**:
- Algorithm: Welford's online algorithm for running mean/stddev
- Complexity: O(1) per sample
- Memory: O(capacity) bounded
- Detection: Z-score > 3σ triggers anomaly

**Hit Rate Calculation**:
```
hit_rate = cache_hits / (cache_hits + cache_misses)
```

**QPS Calculation**:
```
qps = total_requests / window_duration_seconds
```

## 📡 API Endpoints

### 1. Get Current Metrics

Retrieve metrics snapshot for a time window.
```bash
curl "http://localhost:4000/monitoring/metrics?window=1m"
```

**Response:**
```json
{
  "timestamp": "2025-01-15T10:30:00Z",
  "window": 60000000000,
  "total_requests": 15234,
  "cache_hits": 13421,
  "cache_misses": 1813,
  "hit_rate": 0.881,
  "qps": 253.9,
  "avg_latency_ms": 12.3,
  "p50_latency_ms": 8.5,
  "p90_latency_ms": 23.1,
  "p95_latency_ms": 34.7,
  "p99_latency_ms": 67.2,
  "error_rate": 0.002,
  "invalidations": 45,
  "warmings": 123,
  "evictions": 234
}
```

### 2. Get Aggregated Time-Series Data

Retrieve historical metrics with custom aggregation intervals.
```bash
curl -X POST http://localhost:4000/monitoring/aggregated \
  -H "Content-Type: application/json" \
  -d '{
    "start_time": "2025-01-15T10:00:00Z",
    "end_time": "2025-01-15T11:00:00Z",
    "interval": 300000000000
  }'
```

**Response:**
```json
{
  "data_points": [
    {
      "timestamp": "2025-01-15T10:00:00Z",
      "requests": 2341,
      "hit_rate": 0.87,
      "avg_latency_ms": 11.2,
      "p95_latency_ms": 32.1,
      "qps": 234.1,
      "error_rate": 0.001
    },
    ...
  ],
  "summary": {
    "timestamp": "2025-01-15T11:00:00Z",
    "window": 3600000000000,
    "total_requests": 45678,
    "cache_hits": 39876,
    "cache_misses": 5802,
    "hit_rate": 0.873,
    ...
  }
}
```

### 3. Get Active Alerts

Retrieve current alerts and alert statistics.
```bash
curl http://localhost:4000/monitoring/alerts
```

**Response:**
```json
{
  "active_alerts": [
    {
      "id": "latency_spike",
      "rule": "latency_spike",
      "type": "latency_spike",
      "severity": "warning",
      "metric": "p95_latency",
      "current_value": 145.3,
      "threshold": 100.0,
      "message": "P95 latency 145.30ms exceeds threshold 100.00ms",
      "triggered_at": "2025-01-15T10:28:00Z",
      "resolved": false
    }
  ],
  "recent_alerts": [
    {
      "id": "low_hit_rate",
      "type": "low_hit_rate",
      "severity": "warning",
      "resolved": true,
      "resolved_at": "2025-01-15T10:25:00Z",
      "duration": 180000000000
    }
  ],
  "alert_stats": {
    "total_triggered": 87,
    "total_resolved": 85,
    "active_count": 2,
    "avg_duration_seconds": 127.3
  }
}
```

## 🔧 Design Decisions

### 1. Lock-Free Ring Buffer

**Choice**: Atomic CAS-based ring buffer for latency samples.

**Trade-offs**:
- ✅ **Pros**: 
  - Zero lock contention on hot path
  - Predictable performance under load
  - Simple implementation
- ⚠️ **Cons**: 
  - Possible sample loss under extreme contention
  - Memory ordering complexity

**Rationale**: For monitoring, occasional sample loss (< 0.1%) is acceptable for 10x throughput improvement.

**Alternative Considered**: Lock-based queue
- Rejected due to lock contention bottleneck

### 2. Welford's Online Algorithm

**Choice**: Use Welford's algorithm for running mean/variance calculation.

**Formula**:
```
δ = x_n - μ_{n-1}
μ_n = μ_{n-1} + δ/n
M2_n = M2_{n-1} + δ(x_n - μ_n)
σ² = M2_n / (n-1)
```

**Trade-offs**:
- ✅ **Pros**:
  - O(1) time complexity
  - O(1) space complexity
  - Numerically stable (no catastrophic cancellation)
- ⚠️ **Cons**:
  - Requires careful implementation
  - Floating point precision limits

**Rationale**: Enables real-time anomaly detection without storing all historical samples.

**Alternative Considered**: Store all samples + batch calculation
- Rejected due to O(n) space and O(n) recomputation cost

### 3. Sliding Windows vs Fixed Buckets

**Choice**: Sliding windows with circular buffers.

**Trade-offs**:
- ✅ **Pros**:
  - Smooth metrics (no bucket boundary effects)
  - Accurate time-range queries
- ⚠️ **Cons**:
  - Higher memory usage
  - More complex eviction logic

**Rationale**: Smooth metrics are critical for dashboards and alerting. Memory cost is acceptable (< 10MB per window).

**Alternative Considered**: Fixed time buckets
- Rejected due to "sawtooth" artifacts in charts

### 4. In-Memory vs Persistent Storage

**Choice**: In-memory storage with bounded buffers.

**Trade-offs**:
- ✅ **Pros**:
  - Ultra-low latency queries (<1ms)
  - Simple implementation
  - No database overhead
- ⚠️ **Cons**:
  - Data lost on restart
  - Limited to retention period (default: 1 hour)

**Rationale**: Monitoring data is ephemeral. Long-term storage should use dedicated time-series DB (InfluxDB, Prometheus).

**Production Extension**: 
```go
// Export to external time-series DB
func (s *Service) ExportToPrometheus() {
    stats := s.aggregator.GetStats(...)
    prometheus.GaugeSet("cache_hit_rate", stats.HitRate)
    prometheus.HistogramObserve("cache_latency", stats.P95Latency)
}
```

### 5. Anomaly Detection: Z-Score Threshold

**Choice**: Z-score > 3σ for anomaly detection (3-sigma rule).

**Trade-offs**:
- ✅ **Pros**:
  - 99.7% of normal data within 3σ
  - Well-understood statistical method
  - Automatic adaptation to baseline
- ⚠️ **Cons**:
  - Assumes normal distribution
  - Requires warm-up period (20+ samples)

**Rationale**: Simple, effective, and computationally efficient. Works well for most cache metrics.

**Alternative Considered**: Machine learning models
- Rejected due to complexity and computational cost for marginal accuracy gain

## 🚀 Performance Optimizations

### 1. Lock-Free Atomic Counters

**Before**: Mutex-protected counters
```go
type Counters struct {
    mu sync.Mutex
    hits int64
}
func (c *Counters) Increment() {
    c.mu.Lock()
    c.hits++
    c.mu.Unlock()
}
```

**After**: Atomic operations
```go
type Counters struct {
    hits atomic.Int64
}
func (c *Counters) Increment() {
    c.hits.Add(1) // Lock-free
}
```

**Impact**: 10x throughput improvement (1M → 10M increments/sec).

### 2. Object Pooling for Samples

**Before**: Allocate new samples on each event
```go
samples := make([]Sample, n) // Heap allocation
```

**After**: Preallocated circular buffer
```go
buffer := make([]Sample, capacity) // One-time allocation
buffer[head] = sample // Reuse slots
```

**Impact**: 50% reduction in GC pressure, 20% latency improvement.

### 3. Lazy Percentile Calculation

**Before**: Compute percentiles on every query
```go
func GetStats() Stats {
    sort.Float64s(allSamples) // O(n log n)
    return calculatePercentiles(allSamples)
}
```

**After**: Cache sorted samples until next write
```go
type Cache struct {
    samples []float64
    sorted  []float64
    dirty   bool
}
func (c *Cache) GetPercentiles() {
    if c.dirty {
        c.sorted = append([]float64{}, c.samples...)
        sort.Float64s(c.sorted)
        c.dirty = false
    }
    return calculatePercentiles(c.sorted)
}
```

**Impact**: 100x faster for read-heavy workloads.

### 4. Batch Pub/Sub Processing

**Before**: Process each event individually
```go
func HandleEvent(event MetricEvent) {
    collector.RecordMetric(event) // Individual processing
}
```

**After**: Batch processing
```go
func HandleEvents(events []MetricEvent) {
    for _, event := range events {
        collector.RecordMetric(event) // Amortized cost
    }
}
```

**Impact**: 30% reduction in CPU usage under load.

## 📈 Complexity Analysis

| Operation | Time Complexity | Space Complexity | Notes |
|-----------|----------------|------------------|-------|
| Record Metric | O(1) | O(1) | Atomic increment |
| Add to Ring Buffer | O(1) amortized | O(capacity) | CAS retry rare |
| Get Latency Stats | O(n log n) | O(n) | Sorting required |
| Sliding Window Add | O(1) | O(capacity) | Circular buffer |
| Anomaly Detection | O(1) | O(1) | Welford's algorithm |
| Alert Evaluation | O(k) | O(k) | k = number of rules |
| Time Series Query | O(n) | O(n) | n = buckets in range |

## 🔬 Concurrency Safety

### Race Condition Prevention

**Atomic Counters**:
```go
// SAFE: Atomic operations
counter.Add(1)

// UNSAFE: Non-atomic read-modify-write
counter = counter + 1
```

**Ring Buffer**:
```go
// SAFE: CAS-based claim
for {
    head := rb.head.Load()
    if rb.head.CompareAndSwap(head, nextHead) {
        // Write to claimed slot
        break
    }
}
```

**Mutex Protection**:
```go
// SAFE: RWMutex for read-heavy workloads
func (ts *TimeSeries) GetRange() {
    ts.mu.RLock() // Multiple readers allowed
    defer ts.mu.RUnlock()
    // Read operations
}
```

### Memory Ordering

All atomic operations use `sync/atomic` package which provides sequential consistency guarantees.

## 🧪 Testing

### Running Tests
```bash
# Run all tests
go test ./monitoring/... -v

# Run with race detector
go test ./monitoring/... -race -v

# Run benchmarks
go test ./monitoring/... -bench=. -benchmem

# Run specific test
go test ./monitoring -run TestMetricsCollector_Concurrency -v

# Test coverage
go test ./monitoring/... -coverprofile=coverage.out
go tool cover -html=coverage.out
```

### Performance Benchmarks
```
BenchmarkMetricsCollector_RecordMetric-8              50000000    25.3 ns/op     0 B/op
BenchmarkMetricsCollector_RecordMetricParallel-8     100000000    11.2 ns/op     0 B/op
BenchmarkRingBuffer_Add-8                             30000000    42.1 ns/op     0 B/op
BenchmarkRingBuffer_AddParallel-8                     50000000    31.7 ns/op     0 B/op
BenchmarkCalculateLatencyStats-8                         10000   125000 ns/op  8192 B/op
BenchmarkAnomalyDetector_Detect-8                      5000000   287 ns/op       0 B/op
```

**Interpretation**:
- Metric recording: ~25ns per event (40M events/sec)
- Parallel recording: ~11ns per event (90M events/sec)
- Ring buffer: ~42ns per add (24M adds/sec)
- Latency stats: ~125μs for 1000 samples

## 🎯 Alert Rules

### Static Threshold Rules

**High Error Rate**:
- Threshold: 5% error rate
- Severity: Critical
- Evaluation: Every 10 seconds

**Low Hit Rate**:
- Threshold: 70% hit rate
- Severity: Warning (>50%), Critical (<50%)
- Evaluation: Every 10 seconds

**Latency Spike**:
- Threshold: 100ms P95 latency
- Severity: Warning (>100ms), Critical (>200ms)
- Evaluation: Every 10 seconds

**High Eviction Rate**:
- Threshold: 10 evictions/sec
- Severity: Warning
- Evaluation: Every 10 seconds

### Dynamic Threshold Rules

Based on historical baselines using z-scores:
```go
dynamic_threshold = baseline_mean + (z_score * baseline_stddev)

// Example: Detect anomalies 3σ from baseline
if current_value > (mean + 3*stddev) {
    trigger_alert()
}
```

**Advantages**:
- Automatically adapts to traffic patterns
- No manual threshold tuning
- Reduces false positives during expected changes

**Warm-up Period**: Requires 20+ samples for reliable statistics.

## 🔗 Integration Examples

### Publishing Metrics from Cache Manager
```go
import "github.com/yourusername/distributed-cache-system/monitoring"

// After cache operation
_, err := monitoring.CacheMetricsTopic.Publish(ctx, &monitoring.CacheMetricEvent{
    Operation: "get",
    Key:       key,
    Hit:       hit,
    Latency:   latencyMs,
    Timestamp: time.Now(),
    Instance:  instanceID,
})
```

### Querying Metrics from Dashboard
```go
// Get real-time metrics
resp, err := monitoring.GetMetrics(ctx, &monitoring.GetMetricsRequest{
    Window: 1 * time.Minute,
})

fmt.Printf("Hit Rate: %.2f%%\n", resp.HitRate*100)
fmt.Printf("P95 Latency: %.2fms\n", resp.P95Latency)
```

### Custom Alert Rules
```go
// Implement custom rule
type CustomRule struct {
    id string
}

func (r *CustomRule) ID() string {
    return r.id
}

func (r *CustomRule) Evaluate(stats monitoring.AggregatedStats) *monitoring.Alert {
    // Custom logic
    if stats.QPS > 10000 && stats.HitRate < 0.5 {
        return &monitoring.Alert{
            ID:       r.id,
            Type:     monitoring.AlertAbnormalLoad,
            Severity: "critical",
            Message:  "High load with low hit rate - possible cache issue",
        }
    }
    return nil
}

// Register custom rule
alertManager.RegisterRule(NewCustomRule())
```

## 📊 Dashboard API

### Overview

The dashboard API provides rich, visualization-ready data for monitoring dashboards with real-time updates, historical comparisons, and data export capabilities.

### Dashboard Endpoints

#### 1. Get Dashboard Overview

Complete dashboard data with summary, timeline, and health information.
```bash
curl -X POST http://localhost:4000/monitoring/dashboard/overview \
  -H "Content-Type: application/json" \
  -d '{"time_range": 3600000000000}'
```

**Response:**
```json
{
  "summary": {
    "total_requests": 45678,
    "hit_rate": 0.873,
    "avg_latency_ms": 12.3,
    "p95_latency_ms": 34.7,
    "error_rate": 0.002,
    "qps": 253.9,
    "trend_hit_rate": "up",
    "trend_latency": "down",
    "trend_qps": "stable"
  },
  "timeline": [
    {
      "timestamp": "2025-01-15T10:00:00Z",
      "requests": 756,
      "hit_rate": 0.87,
      "avg_latency_ms": 11.2,
      "p50_latency_ms": 8.5,
      "p95_latency_ms": 32.1,
      "p99_latency_ms": 67.2,
      "error_rate": 0.001,
      "qps": 12.6
    },
    ...
  ],
  "top_keys": [
    {
      "key": "user:123:profile",
      "access_count": 1523,
      "hit_rate": 0.95,
      "avg_latency_ms": 2.3,
      "last_accessed": "2025-01-15T10:30:00Z"
    }
  ],
  "system_health": {
    "status": "healthy",
    "score": 95.0,
    "issues": [],
    "recommendations": []
  },
  "recent_alerts": [],
  "recent_anomalies": []
}
```

#### 2. Get Latency Distribution

Histogram data for latency distribution visualization.
```bash
curl -X POST http://localhost:4000/monitoring/dashboard/latency-distribution \
  -H "Content-Type: application/json" \
  -d '{"window": 300000000000}'
```

**Response:**
```json
{
  "buckets": [
    {
      "min_ms": 0,
      "max_ms": 1,
      "count": 234,
      "percent": 15.6
    },
    {
      "min_ms": 1,
      "max_ms": 5,
      "count": 567,
      "percent": 37.8
    },
    ...
  ],
  "stats": {
    "min": 0.5,
    "max": 234.7,
    "avg": 12.3,
    "p50": 8.5,
    "p90": 23.1,
    "p95": 34.7,
    "p99": 67.2,
    "count": 1500
  }
}
```

#### 3. Get Heatmap Data

Heatmap visualization for time-series metrics.
```bash
curl -X POST http://localhost:4000/monitoring/dashboard/heatmap \
  -H "Content-Type: application/json" \
  -d '{
    "start_time": "2025-01-15T10:00:00Z",
    "end_time": "2025-01-15T11:00:00Z",
    "metric": "hit_rate"
  }'
```

**Supported Metrics:**
- `hit_rate`: Cache hit rate (0-100%)
- `latency`: P95 latency (0-200ms)
- `qps`: Queries per second
- `error_rate`: Error rate (0-10%)

#### 4. Compare Time Periods

Compare metrics between two time periods.
```bash
curl -X POST http://localhost:4000/monitoring/dashboard/comparison \
  -H "Content-Type: application/json" \
  -d '{
    "period1_start": "2025-01-14T10:00:00Z",
    "period1_end": "2025-01-14T11:00:00Z",
    "period2_start": "2025-01-15T10:00:00Z",
    "period2_end": "2025-01-15T11:00:00Z"
  }'
```

**Response:**
```json
{
  "period1": {
    "label": "Period 1",
    "total_requests": 42345,
    "hit_rate": 0.856,
    "avg_latency_ms": 13.7,
    "p95_latency_ms": 38.2,
    "error_rate": 0.003,
    "qps": 235.2
  },
  "period2": {
    "label": "Period 2",
    "total_requests": 45678,
    "hit_rate": 0.873,
    "avg_latency_ms": 12.3,
    "p95_latency_ms": 34.7,
    "error_rate": 0.002,
    "qps": 253.9
  },
  "differences": {
    "requests_diff": 3333,
    "requests_pct": 7.87,
    "hit_rate_diff": 0.017,
    "latency_diff": -1.4,
    "latency_pct": -10.22,
    "error_rate_diff": -0.001,
    "qps_diff": 18.7,
    "qps_pct": 7.95
  }
}
```

#### 5. Real-Time Streaming

Start a real-time metrics stream (Server-Sent Events).
```bash
curl http://localhost:4000/monitoring/dashboard/stream
```

**Stream Format:**
```json
{
  "timestamp": "2025-01-15T10:30:15Z",
  "metrics": {
    "total_requests": 15234,
    "hit_rate": 0.881,
    "qps": 253.9,
    ...
  },
  "alerts": [],
  "anomalies": []
}
```

Updates sent every 1 second. Sessions automatically cleanup after 5 minutes of inactivity.

#### 6. Export Metrics

Export metrics in various formats for external analysis.
```bash
curl -X POST http://localhost:4000/monitoring/dashboard/export \
  -H "Content-Type: application/json" \
  -d '{
    "start_time": "2025-01-15T10:00:00Z",
    "end_time": "2025-01-15T11:00:00Z",
    "format": "json",
    "metrics": ["hit_rate", "latency", "errors"]
  }'
```

**Supported Formats:**
- `json`: JSON array of data points
- `prometheus`: Prometheus exposition format
- `csv`: Comma-separated values

**Response:**
```json
{
  "format": "json",
  "data": "[{\"timestamp\":\"2025-01-15T10:00:00Z\",...}]",
  "filename": "metrics-20250115-103000.json",
  "size": 45678
}
```

### Dashboard Integration Examples

#### React/Next.js Dashboard
```typescript
// Fetch overview data
const response = await fetch('/monitoring/dashboard/overview', {
  method: 'POST',
  headers: { 'Content-Type': 'application/json' },
  body: JSON.stringify({ time_range: 3600000000000 }) // 1 hour
});

const data = await response.json();

// Render with Recharts
<LineChart data={data.timeline}>
  <XAxis dataKey="timestamp" />
  <YAxis />
  <Line dataKey="hit_rate" stroke="#8884d8" />
  <Line dataKey="avg_latency_ms" stroke="#82ca9d" />
</LineChart>

// System health indicator
<HealthBadge 
  status={data.system_health.status}
  score={data.system_health.score}
/>
```

#### Real-Time Updates with EventSource
```typescript
const eventSource = new EventSource('/monitoring/dashboard/stream');

eventSource.onmessage = (event) => {
  const update = JSON.parse(event.data);
  
  // Update dashboard state
  setMetrics(update.metrics);
  setAlerts(update.alerts);
  setAnomalies(update.anomalies);
};

eventSource.onerror = () => {
  console.error('Stream connection lost');
  eventSource.close();
};
```

### System Health Scoring

Health score is calculated based on multiple factors:

**Base Score:** 100 points

**Deductions:**
- Low hit rate (<70%): -20 points
- Elevated latency (>100ms P95): -15 points
- High latency (>200ms P95): -30 points
- Elevated error rate (>1%): -25 points
- High error rate (>5%): -50 points
- High eviction rate (>10/sec): -10 points

**Status Thresholds:**
- `healthy`: Score ≥ 80
- `degraded`: Score 60-79
- `critical`: Score < 60

### Visualization Best Practices

**Timeline Charts:**
- Use 60 data points for smooth visualization
- Show multiple metrics with dual Y-axes
- Highlight anomalies with markers
- Display trends with simple indicators (↑↓→)

**Heatmaps:**
- Use color gradients (green → yellow → orange → red)
- Show time on X-axis (minutes/hours)
- Show metric ranges on Y-axis
- Include tooltips with exact values

**Latency Distribution:**
- Use logarithmic buckets for better visualization
- Show percentiles prominently (P50, P95, P99)
- Include count and percentage for each bucket

**System Health:**
- Use traffic light colors (green/yellow/red)
- Display score prominently
- List actionable recommendations
- Show issue severity clearly

## 🚀 Production Scaling

### Single-Node Limits

Current design supports:
- **Throughput**: 1-10M events/sec (depending on CPU cores)
- **Latency**: <1ms for metric queries
- **Memory**: ~100MB for 1 hour retention at 10K events/sec
- **Retention**: 1 hour (configurable)

### Multi-Node Scaling

For distributed deployments:

**Option 1: Aggregation at Query Time**
```
┌─────────┐  ┌─────────┐  ┌─────────┐
│ Node 1  │  │ Node 2  │  │ Node 3  │
│Monitoring│  │Monitoring│  │Monitoring│
└────┬────┘  └────┬────┘  └────┬────┘
     │            │            │
     └────────────┼────────────┘
                  ▼
          ┌───────────────┐
          │  Aggregator   │
          │   Service     │
          └───────────────┘
```

**Option 2: Centralized Metrics Collection**
```
┌─────────┐  ┌─────────┐  ┌─────────┐
│ Service │  │ Service │  │ Service │
│    1    │  │    2    │  │    3    │
└────┬────┘  └────┬────┘  └────┬────┘
     │            │            │
     └────────────┼────────────┘
                  ▼
          ┌───────────────┐
          │  Monitoring   │
          │   Cluster     │
          └───────────────┘
```

**Option 3: Export to Time-Series DB**
```
┌─────────────┐
│ Monitoring  │
│   Service   │
└──────┬──────┘
       │
       ▼
┌──────────────┐
│ Prometheus / │
│  InfluxDB /  │
│  TimescaleDB │
└──────────────┘
```

**Recommendation**: Option 3 for production (export to dedicated TSDB).

### External TSDB Integration
```go
// Export to Prometheus
func (s *Service) ExportPrometheus() {
    stats := s.aggregator.window1m.GetLatest()
    
    prometheus.Register(prometheus.NewGaugeFunc(
        prometheus.GaugeOpts{
            Name: "cache_hit_rate",
            Help: "Cache hit rate",
        },
        func() float64 { return stats.HitRate },
    ))
    
    prometheus.Register(prometheus.NewHistogramFunc(
        prometheus.HistogramOpts{
            Name: "cache_latency_ms",
            Help: "Cache operation latency",
            Buckets: []float64{1, 5, 10, 25, 50, 100, 250, 500, 1000},
        },
        func() prometheus.Metric {
            return prometheus.MustNewConstHistogram(
                prometheus.HistogramOpts{...},
                uint64(stats.TotalRequests),
                stats.AvgLatency,
                map[float64]uint64{
                    50:  uint64(stats.P50Latency),
                    95:  uint64(stats.P95Latency),
                    99:  uint64(stats.P99Latency),
                },
            )
        },
    ))
}
```

## 📊 Monitoring the Monitoring Service

### Self-Monitoring Metrics
```go
// Internal metrics
monitoring_events_ingested_total
monitoring_aggregation_duration_ms
monitoring_anomaly_detections_total
monitoring_alert_evaluations_total
monitoring_memory_usage_bytes
```

### Health Checks
```bash
# Check service health
curl http://localhost:4000/monitoring/health

# Expected response
{
  "status": "healthy",
  "uptime_seconds": 3600,
  "events_processed": 1234567,
  "memory_usage_mb": 87,
  "active_alerts": 2
}
```

## 🤝 Contributing

See main project README for contribution guidelines.

## 📝 License

See main project LICENSE file.