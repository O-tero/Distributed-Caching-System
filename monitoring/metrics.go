package monitoring

import (
	"math"
	"sort"
	"sync"
	"sync/atomic"
	"time"
)

// MetricsCollector collects and stores metrics in lock-free or minimal-lock structures.
//
// Design: Uses atomic counters for simple metrics and a lock-free ring buffer
// for latency histograms. Optimized for high throughput (>1M events/sec).
//
// Memory: Bounded circular buffer prevents unbounded growth. Old data is
// automatically evicted based on retention policy.
type MetricsCollector struct {
	// Atomic counters for high-frequency metrics
	cacheHits    atomic.Int64
	cacheMisses  atomic.Int64
	cacheSets    atomic.Int64
	cacheDeletes atomic.Int64
	evictions    atomic.Int64
	invalidations atomic.Int64
	warmings     atomic.Int64
	errors       atomic.Int64

	// Latency histogram using lock-free ring buffer
	latencyBuffer *RingBuffer

	// Time-series data for windowed aggregation
	timeSeries *TimeSeries

	config Config
}

// NewMetricsCollector creates a new metrics collector.
func NewMetricsCollector(config Config) *MetricsCollector {
	return &MetricsCollector{
		latencyBuffer: NewRingBuffer(10000), // Keep last 10K latency samples
		timeSeries:    NewTimeSeries(config.MetricsRetention),
		config:        config,
	}
}

// RecordMetric records a metric event.
// Complexity: O(1) for counters, O(1) amortized for histogram.
func (mc *MetricsCollector) RecordMetric(event MetricEvent) {
	// Update atomic counters
	switch event.Type {
	case MetricCacheHit:
		mc.cacheHits.Add(int64(event.Value))
	case MetricCacheMiss:
		mc.cacheMisses.Add(int64(event.Value))
	case MetricCacheSet:
		mc.cacheSets.Add(int64(event.Value))
	case MetricCacheDelete:
		mc.cacheDeletes.Add(int64(event.Value))
	case MetricCacheEviction:
		mc.evictions.Add(int64(event.Value))
	case MetricInvalidation:
		mc.invalidations.Add(int64(event.Value))
	case MetricWarming:
		mc.warmings.Add(int64(event.Value))
	case MetricError:
		mc.errors.Add(int64(event.Value))
	case MetricLatency:
		// Record in histogram
		mc.latencyBuffer.Add(event.Value, event.Timestamp)
	}

	// Add to time series for windowed queries
	mc.timeSeries.Add(event)
}

// GetCounters returns current counter values.
func (mc *MetricsCollector) GetCounters() Counters {
	return Counters{
		CacheHits:     mc.cacheHits.Load(),
		CacheMisses:   mc.cacheMisses.Load(),
		CacheSets:     mc.cacheSets.Load(),
		CacheDeletes:  mc.cacheDeletes.Load(),
		Evictions:     mc.evictions.Load(),
		Invalidations: mc.invalidations.Load(),
		Warmings:      mc.warmings.Load(),
		Errors:        mc.errors.Load(),
	}
}

// GetLatencyStats returns latency statistics.
func (mc *MetricsCollector) GetLatencyStats() LatencyStats {
	samples := mc.latencyBuffer.GetAll()
	if len(samples) == 0 {
		return LatencyStats{}
	}

	return calculateLatencyStats(samples)
}

// Counters holds all counter metrics.
type Counters struct {
	CacheHits     int64
	CacheMisses   int64
	CacheSets     int64
	CacheDeletes  int64
	Evictions     int64
	Invalidations int64
	Warmings      int64
	Errors        int64
}

// LatencyStats holds latency percentile statistics.
type LatencyStats struct {
	Min    float64
	Max    float64
	Avg    float64
	P50    float64
	P90    float64
	P95    float64
	P99    float64
	Count  int
}

// RingBuffer is a lock-free circular buffer for storing latency samples.
//
// Design: Uses atomic operations for head/tail pointers. Trade-off: occasional
// sample loss under extreme contention (acceptable for monitoring).
//
// Complexity: Add O(1), GetAll O(n) where n = buffer size.
type RingBuffer struct {
	buffer    []Sample
	head      atomic.Uint64
	tail      atomic.Uint64
	size      uint64
	mu        sync.RWMutex // Only for GetAll to prevent concurrent reads
}

// Sample represents a single latency sample.
type Sample struct {
	Value     float64
	Timestamp time.Time
}

// NewRingBuffer creates a new ring buffer.
func NewRingBuffer(size int) *RingBuffer {
	return &RingBuffer{
		buffer: make([]Sample, size),
		size:   uint64(size),
	}
}

// Add adds a sample to the ring buffer.
// Lock-free implementation using atomic CAS.
func (rb *RingBuffer) Add(value float64, timestamp time.Time) {
	for {
		head := rb.head.Load()
		nextHead := (head + 1) % rb.size

		// Try to claim this slot
		if rb.head.CompareAndSwap(head, nextHead) {
			// Successfully claimed, write sample
			rb.buffer[head] = Sample{
				Value:     value,
				Timestamp: timestamp,
			}

			// Update tail (may occasionally lag, acceptable)
			for {
				tail := rb.tail.Load()
				if nextHead > tail {
					rb.tail.CompareAndSwap(tail, nextHead)
					break
				}
				break
			}

			return
		}
		// CAS failed, retry
	}
}

// GetAll returns all samples in the buffer.
// Complexity: O(n) where n = buffer size.
func (rb *RingBuffer) GetAll() []Sample {
	rb.mu.RLock()
	defer rb.mu.RUnlock()

	head := rb.head.Load()
	tail := rb.tail.Load()

	if head == tail {
		return []Sample{}
	}

	size := (head - tail) % rb.size
	if size == 0 {
		size = rb.size
	}

	result := make([]Sample, 0, size)
	for i := tail; i != head; i = (i + 1) % rb.size {
		result = append(result, rb.buffer[i])
	}

	return result
}

// GetRecent returns samples from the last duration.
func (rb *RingBuffer) GetRecent(duration time.Duration) []Sample {
	allSamples := rb.GetAll()
	cutoff := time.Now().Add(-duration)

	result := make([]Sample, 0)
	for _, sample := range allSamples {
		if sample.Timestamp.After(cutoff) {
			result = append(result, sample)
		}
	}

	return result
}

// calculateLatencyStats computes percentile statistics from samples.
// Complexity: O(n log n) due to sorting.
func calculateLatencyStats(samples []Sample) LatencyStats {
	if len(samples) == 0 {
		return LatencyStats{}
	}

	// Extract values
	values := make([]float64, len(samples))
	sum := 0.0
	min := math.MaxFloat64
	max := 0.0

	for i, sample := range samples {
		values[i] = sample.Value
		sum += sample.Value
		if sample.Value < min {
			min = sample.Value
		}
		if sample.Value > max {
			max = sample.Value
		}
	}

	// Sort for percentile calculation
	sort.Float64s(values)

	return LatencyStats{
		Min:   min,
		Max:   max,
		Avg:   sum / float64(len(values)),
		P50:   percentile(values, 0.50),
		P90:   percentile(values, 0.90),
		P95:   percentile(values, 0.95),
		P99:   percentile(values, 0.99),
		Count: len(values),
	}
}

// percentile calculates the p-th percentile of sorted values.
// Assumes values is already sorted.
func percentile(values []float64, p float64) float64 {
	if len(values) == 0 {
		return 0
	}

	index := p * float64(len(values)-1)
	lower := int(math.Floor(index))
	upper := int(math.Ceil(index))

	if lower == upper {
		return values[lower]
	}

	// Linear interpolation
	weight := index - float64(lower)
	return values[lower]*(1-weight) + values[upper]*weight
}

// TimeSeries stores metric events in time-ordered buckets for windowed queries.
//
// Design: Uses a map of time buckets (1-second granularity) with automatic
// cleanup of old buckets. Trade-off: uses more memory than pure circular buffer
// but allows efficient time-range queries.
type TimeSeries struct {
	mu         sync.RWMutex
	buckets    map[int64]*Bucket // Unix timestamp (seconds) -> Bucket
	retention  time.Duration
	lastCleanup time.Time
}

// Bucket holds metrics for a 1-second time window.
type Bucket struct {
	Timestamp     time.Time
	Events        []MetricEvent
	CacheHits     int64
	CacheMisses   int64
	Latencies     []float64
	Errors        int64
	Invalidations int64
	Warmings      int64
}

// NewTimeSeries creates a new time series store.
func NewTimeSeries(retention time.Duration) *TimeSeries {
	return &TimeSeries{
		buckets:     make(map[int64]*Bucket),
		retention:   retention,
		lastCleanup: time.Now(),
	}
}

// Add adds an event to the time series.
// Complexity: O(1) amortized (occasional cleanup is O(n)).
func (ts *TimeSeries) Add(event MetricEvent) {
	ts.mu.Lock()
	defer ts.mu.Unlock()

	// Get or create bucket
	bucketKey := event.Timestamp.Unix()
	bucket, exists := ts.buckets[bucketKey]
	if !exists {
		bucket = &Bucket{
			Timestamp: time.Unix(bucketKey, 0),
			Events:    make([]MetricEvent, 0),
			Latencies: make([]float64, 0),
		}
		ts.buckets[bucketKey] = bucket
	}

	// Add event to bucket
	bucket.Events = append(bucket.Events, event)

	// Update bucket aggregates
	switch event.Type {
	case MetricCacheHit:
		bucket.CacheHits++
	case MetricCacheMiss:
		bucket.CacheMisses++
	case MetricLatency:
		bucket.Latencies = append(bucket.Latencies, event.Value)
	case MetricError:
		bucket.Errors++
	case MetricInvalidation:
		bucket.Invalidations++
	case MetricWarming:
		bucket.Warmings++
	}

	// Periodic cleanup
	if time.Since(ts.lastCleanup) > 1*time.Minute {
		ts.cleanup()
		ts.lastCleanup = time.Now()
	}
}

// GetRange returns buckets within a time range.
// Complexity: O(n) where n = number of buckets in range.
func (ts *TimeSeries) GetRange(start, end time.Time) []*Bucket {
	ts.mu.RLock()
	defer ts.mu.RUnlock()

	result := make([]*Bucket, 0)
	startKey := start.Unix()
	endKey := end.Unix()

	for key, bucket := range ts.buckets {
		if key >= startKey && key <= endKey {
			result = append(result, bucket)
		}
	}

	// Sort by timestamp
	sort.Slice(result, func(i, j int) bool {
		return result[i].Timestamp.Before(result[j].Timestamp)
	})

	return result
}

// cleanup removes buckets older than retention period.
func (ts *TimeSeries) cleanup() {
	cutoff := time.Now().Add(-ts.retention).Unix()

	for key := range ts.buckets {
		if key < cutoff {
			delete(ts.buckets, key)
		}
	}
}