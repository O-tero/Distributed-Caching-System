package models

import (
	"fmt"
	"math"
	"sort"
	"time"
)

// MetricSnapshot represents a point-in-time snapshot of cache metrics.
//
// Design: Uses primitive types for zero-allocation access in hot paths.
// All fields are exported for direct access but should be treated as immutable
// after creation.
type MetricSnapshot struct {
	Timestamp time.Time // When snapshot was taken

	// Counter metrics
	CacheHits     uint64 // Total cache hits
	CacheMisses   uint64 // Total cache misses
	Sets          uint64 // Total set operations
	Deletes       uint64 // Total delete operations
	Evictions     uint64 // Total evictions

	// Size metrics
	L1Size        uint64 // Number of entries in L1 cache
	L2Size        uint64 // Number of entries in L2 cache (if applicable)
	TotalSize     uint64 // Total number of cached entries

	// Latency metrics
	Latency       LatencySummary // Latency statistics

	// Derived metrics (calculated fields)
	HitRate       float64 // Cache hit rate (hits / total requests)
	MissRate      float64 // Cache miss rate (misses / total requests)
}

// LatencySummary provides statistical summary of latency measurements.
//
// Memory: Fixed size struct (no allocations for updates).
// Thread Safety: Caller must synchronize access.
type LatencySummary struct {
	Count uint64        // Number of samples
	Sum   time.Duration // Sum of all samples
	Min   time.Duration // Minimum latency
	Max   time.Duration // Maximum latency
	P50   time.Duration // 50th percentile (median)
	P90   time.Duration // 90th percentile
	P95   time.Duration // 95th percentile
	P99   time.Duration // 99th percentile
}

// NewMetricSnapshot creates a new metric snapshot with calculated derived fields.
func NewMetricSnapshot(hits, misses, sets, deletes, evictions uint64, latency LatencySummary) MetricSnapshot {
	total := hits + misses
	hitRate := 0.0
	missRate := 0.0

	if total > 0 {
		hitRate = float64(hits) / float64(total)
		missRate = float64(misses) / float64(total)
	}

	return MetricSnapshot{
		Timestamp:   time.Now(),
		CacheHits:   hits,
		CacheMisses: misses,
		Sets:        sets,
		Deletes:     deletes,
		Evictions:   evictions,
		Latency:     latency,
		HitRate:     hitRate,
		MissRate:    missRate,
	}
}

// TotalRequests returns the total number of cache requests.
func (m *MetricSnapshot) TotalRequests() uint64 {
	return m.CacheHits + m.CacheMisses
}

// EvictionRate returns evictions per request (0-1 range).
func (m *MetricSnapshot) EvictionRate() float64 {
	total := m.TotalRequests()
	if total == 0 {
		return 0
	}
	return float64(m.Evictions) / float64(total)
}

// MergeSnapshots combines two metric snapshots.
// Complexity: O(1)
//
// Usage:
//   snapshot1 := GetSnapshot(node1)
//   snapshot2 := GetSnapshot(node2)
//   combined := MergeSnapshots(snapshot1, snapshot2)
func MergeSnapshots(a, b MetricSnapshot) MetricSnapshot {
	// Sum counters
	hits := a.CacheHits + b.CacheHits
	misses := a.CacheMisses + b.CacheMisses
	sets := a.Sets + b.Sets
	deletes := a.Deletes + b.Deletes
	evictions := a.Evictions + b.Evictions

	// Merge size metrics
	l1Size := a.L1Size + b.L1Size
	l2Size := a.L2Size + b.L2Size
	totalSize := a.TotalSize + b.TotalSize

	// Merge latency summaries
	latency := MergeLatencySummaries(a.Latency, b.Latency)

	// Calculate derived metrics
	total := hits + misses
	hitRate := 0.0
	missRate := 0.0

	if total > 0 {
		hitRate = float64(hits) / float64(total)
		missRate = float64(misses) / float64(total)
	}

	return MetricSnapshot{
		Timestamp:   time.Now(),
		CacheHits:   hits,
		CacheMisses: misses,
		Sets:        sets,
		Deletes:     deletes,
		Evictions:   evictions,
		L1Size:      l1Size,
		L2Size:      l2Size,
		TotalSize:   totalSize,
		Latency:     latency,
		HitRate:     hitRate,
		MissRate:    missRate,
	}
}

// MergeLatencySummaries combines two latency summaries.
// Note: Percentiles are approximated by taking weighted average based on sample count.
// For exact percentiles, original sample data is required.
func MergeLatencySummaries(a, b LatencySummary) LatencySummary {
	if a.Count == 0 {
		return b
	}
	if b.Count == 0 {
		return a
	}

	totalCount := a.Count + b.Count
	weightA := float64(a.Count) / float64(totalCount)
	weightB := float64(b.Count) / float64(totalCount)

	return LatencySummary{
		Count: totalCount,
		Sum:   a.Sum + b.Sum,
		Min:   minDuration(a.Min, b.Min),
		Max:   maxDuration(a.Max, b.Max),
		P50:   time.Duration(float64(a.P50)*weightA + float64(b.P50)*weightB),
		P90:   time.Duration(float64(a.P90)*weightA + float64(b.P90)*weightB),
		P95:   time.Duration(float64(a.P95)*weightA + float64(b.P95)*weightB),
		P99:   time.Duration(float64(a.P99)*weightA + float64(b.P99)*weightB),
	}
}

// UpdateLatency updates a latency summary with a new sample.
// Note: This does NOT update percentiles accurately. For accurate percentiles,
// store samples and recalculate periodically using CalculateLatencySummary.
//
// This method only updates Count, Sum, Min, Max for efficiency.
// Percentiles should be recalculated from raw samples.
func UpdateLatency(summary *LatencySummary, sample time.Duration) {
	if summary.Count == 0 {
		summary.Min = sample
		summary.Max = sample
	} else {
		if sample < summary.Min {
			summary.Min = sample
		}
		if sample > summary.Max {
			summary.Max = sample
		}
	}

	summary.Count++
	summary.Sum += sample
}

// CalculateLatencySummary computes accurate latency summary from samples.
// Complexity: O(n log n) due to sorting for percentiles.
//
// Example:
//   samples := []time.Duration{1*time.Millisecond, 5*time.Millisecond, 10*time.Millisecond}
//   summary := CalculateLatencySummary(samples)
func CalculateLatencySummary(samples []time.Duration) LatencySummary {
	if len(samples) == 0 {
		return LatencySummary{}
	}

	// Sort samples for percentile calculation
	sorted := make([]time.Duration, len(samples))
	copy(sorted, samples)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i] < sorted[j]
	})

	var sum time.Duration
	for _, sample := range sorted {
		sum += sample
	}

	return LatencySummary{
		Count: uint64(len(sorted)),
		Sum:   sum,
		Min:   sorted[0],
		Max:   sorted[len(sorted)-1],
		P50:   percentileDuration(sorted, 0.50),
		P90:   percentileDuration(sorted, 0.90),
		P95:   percentileDuration(sorted, 0.95),
		P99:   percentileDuration(sorted, 0.99),
	}
}

// AvgLatency returns the average latency.
func (ls *LatencySummary) AvgLatency() time.Duration {
	if ls.Count == 0 {
		return 0
	}
	return ls.Sum / time.Duration(ls.Count)
}

// SnapshotToPrometheusFormat converts a snapshot to Prometheus-compatible metrics map.
// Returns a map of metric_name -> float64 value suitable for Prometheus export.
//
// Usage:
//   metrics := SnapshotToPrometheusFormat(snapshot, "cache")
//   for name, value := range metrics {
//       prometheus.GaugeSet(name, value)
//   }
func SnapshotToPrometheusFormat(snapshot MetricSnapshot, prefix string) map[string]float64 {
	metrics := make(map[string]float64)

	// Counter metrics
	metrics[fmt.Sprintf("%s_hits_total", prefix)] = float64(snapshot.CacheHits)
	metrics[fmt.Sprintf("%s_misses_total", prefix)] = float64(snapshot.CacheMisses)
	metrics[fmt.Sprintf("%s_sets_total", prefix)] = float64(snapshot.Sets)
	metrics[fmt.Sprintf("%s_deletes_total", prefix)] = float64(snapshot.Deletes)
	metrics[fmt.Sprintf("%s_evictions_total", prefix)] = float64(snapshot.Evictions)

	// Gauge metrics
	metrics[fmt.Sprintf("%s_hit_rate", prefix)] = snapshot.HitRate
	metrics[fmt.Sprintf("%s_miss_rate", prefix)] = snapshot.MissRate
	metrics[fmt.Sprintf("%s_l1_size", prefix)] = float64(snapshot.L1Size)
	metrics[fmt.Sprintf("%s_l2_size", prefix)] = float64(snapshot.L2Size)

	// Latency metrics (in milliseconds)
	metrics[fmt.Sprintf("%s_latency_avg_ms", prefix)] = float64(snapshot.Latency.AvgLatency().Milliseconds())
	metrics[fmt.Sprintf("%s_latency_min_ms", prefix)] = float64(snapshot.Latency.Min.Milliseconds())
	metrics[fmt.Sprintf("%s_latency_max_ms", prefix)] = float64(snapshot.Latency.Max.Milliseconds())
	metrics[fmt.Sprintf("%s_latency_p50_ms", prefix)] = float64(snapshot.Latency.P50.Milliseconds())
	metrics[fmt.Sprintf("%s_latency_p90_ms", prefix)] = float64(snapshot.Latency.P90.Milliseconds())
	metrics[fmt.Sprintf("%s_latency_p95_ms", prefix)] = float64(snapshot.Latency.P95.Milliseconds())
	metrics[fmt.Sprintf("%s_latency_p99_ms", prefix)] = float64(snapshot.Latency.P99.Milliseconds())

	return metrics
}

// Helper functions

func minDuration(a, b time.Duration) time.Duration {
	if a < b {
		return a
	}
	return b
}

func maxDuration(a, b time.Duration) time.Duration {
	if a > b {
		return a
	}
	return b
}

// percentileDuration calculates the p-th percentile from sorted durations.
// Assumes samples is already sorted.
func percentileDuration(samples []time.Duration, p float64) time.Duration {
	if len(samples) == 0 {
		return 0
	}

	index := p * float64(len(samples)-1)
	lower := int(math.Floor(index))
	upper := int(math.Ceil(index))

	if lower == upper {
		return samples[lower]
	}

	// Linear interpolation
	weight := index - float64(lower)
	return time.Duration(float64(samples[lower])*(1-weight) + float64(samples[upper])*weight)
}