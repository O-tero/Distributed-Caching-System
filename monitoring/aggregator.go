package monitoring

import (
	"math"
	"sync"
	"time"
)

// Aggregator performs real-time statistical aggregation on metrics.
//
// Design: Maintains sliding windows (1s, 10s, 1m) using efficient ring buffers.
// Computes percentiles, averages, and anomaly detection in real-time.
//
// Performance: Sub-millisecond aggregation for 1-second windows.
// Memory: ~1MB per window for 10K events/sec.
type Aggregator struct {
	collector *MetricsCollector
	config    Config

	// Sliding windows for different time scales
	window1s  *SlidingWindow
	window10s *SlidingWindow
	window1m  *SlidingWindow

	// Anomaly detector
	detector *AnomalyDetector

	// Control
	stopChan chan struct{}
	wg       sync.WaitGroup
}

// NewAggregator creates a new metrics aggregator.
func NewAggregator(collector *MetricsCollector, config Config) *Aggregator {
	return &Aggregator{
		collector: collector,
		config:    config,
		window1s:  NewSlidingWindow(1 * time.Second),
		window10s: NewSlidingWindow(10 * time.Second),
		window1m:  NewSlidingWindow(1 * time.Minute),
		detector:  NewAnomalyDetector(),
		stopChan:  make(chan struct{}),
	}
}

// Run starts the aggregation background worker.
func (a *Aggregator) Run() {
	a.wg.Add(1)
	defer a.wg.Done()

	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-a.stopChan:
			return
		case <-ticker.C:
			a.aggregate()
		}
	}
}

// aggregate performs aggregation for all windows.
func (a *Aggregator) aggregate() {
	now := time.Now()

	// Get current metrics
	counters := a.collector.GetCounters()
	latencyStats := a.collector.GetLatencyStats()

	// Create snapshot
	totalRequests := counters.CacheHits + counters.CacheMisses
	snapshot := AggregatedStats{
		Timestamp:     now,
		TotalRequests: totalRequests,
		CacheHits:     counters.CacheHits,
		CacheMisses:   counters.CacheMisses,
		HitRate:       calculateHitRate(counters.CacheHits, counters.CacheMisses),
		AvgLatency:    latencyStats.Avg,
		P50Latency:    latencyStats.P50,
		P90Latency:    latencyStats.P90,
		P95Latency:    latencyStats.P95,
		P99Latency:    latencyStats.P99,
		ErrorRate:     calculateErrorRate(counters.Errors, totalRequests),
		Invalidations: counters.Invalidations,
		Warmings:      counters.Warmings,
		Evictions:     counters.Evictions,
	}

	// Add to sliding windows
	a.window1s.Add(snapshot)
	a.window10s.Add(snapshot)
	a.window1m.Add(snapshot)

	// Detect anomalies
	a.detector.Detect(snapshot)
}

// GetStats returns aggregated statistics for a time range.
// Complexity: O(n) where n = number of buckets in range.
func (a *Aggregator) GetStats(start, end time.Time) AggregatedStats {
	duration := end.Sub(start)

	// Select appropriate window based on duration
	var window *SlidingWindow
	switch {
	case duration <= 1*time.Second:
		window = a.window1s
	case duration <= 10*time.Second:
		window = a.window10s
	case duration <= 1*time.Minute:
		window = a.window1m
	default:
		// For longer ranges, query time series directly
		return a.aggregateFromTimeSeries(start, end)
	}

	snapshots := window.GetRange(start, end)
	return aggregateSnapshots(snapshots, duration)
}

// aggregateFromTimeSeries aggregates data from raw time series.
func (a *Aggregator) aggregateFromTimeSeries(start, end time.Time) AggregatedStats {
	buckets := a.collector.timeSeries.GetRange(start, end)

	var totalHits, totalMisses, totalErrors, totalInvalidations, totalWarmings, totalEvictions int64
	allLatencies := make([]float64, 0)

	for _, bucket := range buckets {
		totalHits += bucket.CacheHits
		totalMisses += bucket.CacheMisses
		totalErrors += bucket.Errors
		totalInvalidations += bucket.Invalidations
		totalWarmings += bucket.Warmings
		allLatencies = append(allLatencies, bucket.Latencies...)
	}

	latencyStats := LatencyStats{}
	if len(allLatencies) > 0 {
		// Create samples for calculation
		samples := make([]Sample, len(allLatencies))
		for i, lat := range allLatencies {
			samples[i] = Sample{Value: lat}
		}
		latencyStats = calculateLatencyStats(samples)
	}

	duration := end.Sub(start)
	totalRequests := totalHits + totalMisses
	qps := 0.0
	if duration.Seconds() > 0 {
		qps = float64(totalRequests) / duration.Seconds()
	}

	return AggregatedStats{
		Timestamp:     end,
		TotalRequests: totalRequests,
		CacheHits:     totalHits,
		CacheMisses:   totalMisses,
		HitRate:       calculateHitRate(totalHits, totalMisses),
		QPS:           qps,
		AvgLatency:    latencyStats.Avg,
		P50Latency:    latencyStats.P50,
		P90Latency:    latencyStats.P90,
		P95Latency:    latencyStats.P95,
		P99Latency:    latencyStats.P99,
		ErrorRate:     calculateErrorRate(totalErrors, totalRequests),
		Invalidations: totalInvalidations,
		Warmings:      totalWarmings,
		Evictions:     totalEvictions,
	}
}

// Stop gracefully stops the aggregator.
func (a *Aggregator) Stop() {
	close(a.stopChan)
	a.wg.Wait()
}

// AggregatedStats holds aggregated statistics for a time window.
type AggregatedStats struct {
	Timestamp     time.Time
	TotalRequests int64
	CacheHits     int64
	CacheMisses   int64
	HitRate       float64
	QPS           float64
	AvgLatency    float64
	P50Latency    float64
	P90Latency    float64
	P95Latency    float64
	P99Latency    float64
	ErrorRate     float64
	Invalidations int64
	Warmings      int64
	Evictions     int64
}

// SlidingWindow maintains a time-ordered sliding window of aggregated stats.
//
// Design: Uses circular buffer with time-based indexing. Automatically
// evicts old data outside the window.
type SlidingWindow struct {
	mu       sync.RWMutex
	duration time.Duration
	buffer   []AggregatedStats
	capacity int
	head     int
}

// NewSlidingWindow creates a new sliding window.
func NewSlidingWindow(duration time.Duration) *SlidingWindow {
	// Calculate capacity based on duration (1 sample per second)
	capacity := int(duration.Seconds()) + 1

	return &SlidingWindow{
		duration: duration,
		buffer:   make([]AggregatedStats, capacity),
		capacity: capacity,
		head:     0,
	}
}

// Add adds a snapshot to the sliding window.
// Complexity: O(1).
func (sw *SlidingWindow) Add(stats AggregatedStats) {
	sw.mu.Lock()
	defer sw.mu.Unlock()

	sw.buffer[sw.head] = stats
	sw.head = (sw.head + 1) % sw.capacity
}

// GetRange returns snapshots within the time range.
// Complexity: O(n) where n = window capacity.
func (sw *SlidingWindow) GetRange(start, end time.Time) []AggregatedStats {
	sw.mu.RLock()
	defer sw.mu.RUnlock()

	result := make([]AggregatedStats, 0)

	for i := 0; i < sw.capacity; i++ {
		stats := sw.buffer[i]
		if !stats.Timestamp.IsZero() &&
			!stats.Timestamp.Before(start) &&
			!stats.Timestamp.After(end) {
			result = append(result, stats)
		}
	}

	return result
}

// GetLatest returns the most recent snapshot.
func (sw *SlidingWindow) GetLatest() AggregatedStats {
	sw.mu.RLock()
	defer sw.mu.RUnlock()

	// Most recent is at (head - 1)
	index := (sw.head - 1 + sw.capacity) % sw.capacity
	return sw.buffer[index]
}

// aggregateSnapshots combines multiple snapshots into a single aggregate.
func aggregateSnapshots(snapshots []AggregatedStats, duration time.Duration) AggregatedStats {
	if len(snapshots) == 0 {
		return AggregatedStats{}
	}

	var totalRequests, totalHits, totalMisses, totalErrors int64
	var totalInvalidations, totalWarmings, totalEvictions int64
	var sumAvgLatency, sumP50, sumP90, sumP95, sumP99 float64
	count := len(snapshots)

	for _, snap := range snapshots {
		totalRequests += snap.TotalRequests
		totalHits += snap.CacheHits
		totalMisses += snap.CacheMisses
		totalInvalidations += snap.Invalidations
		totalWarmings += snap.Warmings
		totalEvictions += snap.Evictions

		sumAvgLatency += snap.AvgLatency
		sumP50 += snap.P50Latency
		sumP90 += snap.P90Latency
		sumP95 += snap.P95Latency
		sumP99 += snap.P99Latency
	}

	qps := 0.0
	if duration.Seconds() > 0 {
		qps = float64(totalRequests) / duration.Seconds()
	}

	return AggregatedStats{
		Timestamp:     snapshots[len(snapshots)-1].Timestamp,
		TotalRequests: totalRequests,
		CacheHits:     totalHits,
		CacheMisses:   totalMisses,
		HitRate:       calculateHitRate(totalHits, totalMisses),
		QPS:           qps,
		AvgLatency:    sumAvgLatency / float64(count),
		P50Latency:    sumP50 / float64(count),
		P90Latency:    sumP90 / float64(count),
		P95Latency:    sumP95 / float64(count),
		P99Latency:    sumP99 / float64(count),
		ErrorRate:     calculateErrorRate(totalErrors, totalRequests),
		Invalidations: totalInvalidations,
		Warmings:      totalWarmings,
		Evictions:     totalEvictions,
	}
}

// Helper functions

func calculateHitRate(hits, misses int64) float64 {
	total := hits + misses
	if total == 0 {
		return 0
	}
	return float64(hits) / float64(total)
}

func calculateErrorRate(errors, total int64) float64 {
	if total == 0 {
		return 0
	}
	return float64(errors) / float64(total)
}

// AnomalyDetector detects statistical anomalies in metrics.
//
// Methods:
// - Z-score: Detects values far from mean (> 3 standard deviations)
// - 3-sigma rule: 99.7% of values should fall within 3σ of mean
// - Sudden changes: Detects rapid changes between consecutive samples
type AnomalyDetector struct {
	mu sync.RWMutex

	// Historical statistics for baseline
	hitRateHistory    *HistoricalStats
	latencyHistory    *HistoricalStats
	errorRateHistory  *HistoricalStats
	qpsHistory        *HistoricalStats

	// Detected anomalies
	anomalies []Anomaly
}

// Anomaly represents a detected anomaly.
type Anomaly struct {
	Type      AnomalyType
	Severity  string // "low", "medium", "high", "critical"
	Metric    string
	Value     float64
	Expected  float64
	Deviation float64
	Timestamp time.Time
	Message   string
}

// AnomalyType represents the type of anomaly.
type AnomalyType string

const (
	AnomalyLatencySpike   AnomalyType = "latency_spike"
	AnomalyHitRateDrop    AnomalyType = "hit_rate_drop"
	AnomalyErrorRateSpike AnomalyType = "error_rate_spike"
	AnomalyQPSAnomaly     AnomalyType = "qps_anomaly"
)

// NewAnomalyDetector creates a new anomaly detector.
func NewAnomalyDetector() *AnomalyDetector {
	return &AnomalyDetector{
		hitRateHistory:   NewHistoricalStats(100),
		latencyHistory:   NewHistoricalStats(100),
		errorRateHistory: NewHistoricalStats(100),
		qpsHistory:       NewHistoricalStats(100),
		anomalies:        make([]Anomaly, 0),
	}
}

// Detect detects anomalies in the given stats.
func (ad *AnomalyDetector) Detect(stats AggregatedStats) {
	ad.mu.Lock()
	defer ad.mu.Unlock()

	now := time.Now()

	// Update historical stats
	ad.hitRateHistory.Add(stats.HitRate)
	ad.latencyHistory.Add(stats.P95Latency)
	ad.errorRateHistory.Add(stats.ErrorRate)
	ad.qpsHistory.Add(stats.QPS)

	// Detect hit rate anomaly
	if ad.hitRateHistory.Count() > 10 {
		mean, stddev := ad.hitRateHistory.MeanStdDev()
		zscore := (stats.HitRate - mean) / stddev

		if zscore < -3.0 { // Hit rate dropped significantly
			anomaly := Anomaly{
				Type:      AnomalyHitRateDrop,
				Severity:  ad.calculateSeverity(zscore),
				Metric:    "hit_rate",
				Value:     stats.HitRate,
				Expected:  mean,
				Deviation: zscore,
				Timestamp: now,
				Message:   "Cache hit rate dropped below expected range",
			}
			ad.anomalies = append(ad.anomalies, anomaly)
		}
	}

	// Detect latency spike
	if ad.latencyHistory.Count() > 10 {
		mean, stddev := ad.latencyHistory.MeanStdDev()
		zscore := (stats.P95Latency - mean) / stddev

		if zscore > 3.0 { // Latency spiked
			anomaly := Anomaly{
				Type:      AnomalyLatencySpike,
				Severity:  ad.calculateSeverity(zscore),
				Metric:    "p95_latency",
				Value:     stats.P95Latency,
				Expected:  mean,
				Deviation: zscore,
				Timestamp: now,
				Message:   "P95 latency significantly higher than baseline",
			}
			ad.anomalies = append(ad.anomalies, anomaly)
		}
	}

	// Detect error rate spike
	if ad.errorRateHistory.Count() > 10 {
		mean, stddev := ad.errorRateHistory.MeanStdDev()
		if stddev > 0 {
			zscore := (stats.ErrorRate - mean) / stddev

			if zscore > 3.0 {
				anomaly := Anomaly{
					Type:      AnomalyErrorRateSpike,
					Severity:  "critical",
					Metric:    "error_rate",
					Value:     stats.ErrorRate,
					Expected:  mean,
					Deviation: zscore,
					Timestamp: now,
					Message:   "Error rate significantly elevated",
				}
				ad.anomalies = append(ad.anomalies, anomaly)
			}
		}
	}

	// Detect QPS anomaly (sudden traffic changes)
	if ad.qpsHistory.Count() > 10 {
		mean, stddev := ad.qpsHistory.MeanStdDev()
		if stddev > 0 {
			zscore := math.Abs((stats.QPS - mean) / stddev)

			if zscore > 4.0 { // Very unusual traffic pattern
				anomaly := Anomaly{
					Type:      AnomalyQPSAnomaly,
					Severity:  ad.calculateSeverity(zscore),
					Metric:    "qps",
					Value:     stats.QPS,
					Expected:  mean,
					Deviation: zscore,
					Timestamp: now,
					Message:   "Unusual traffic pattern detected",
				}
				ad.anomalies = append(ad.anomalies, anomaly)
			}
		}
	}

	// Cleanup old anomalies (keep last 100)
	if len(ad.anomalies) > 100 {
		ad.anomalies = ad.anomalies[len(ad.anomalies)-100:]
	}
}

// GetRecentAnomalies returns anomalies from the last duration.
func (ad *AnomalyDetector) GetRecentAnomalies(duration time.Duration) []Anomaly {
	ad.mu.RLock()
	defer ad.mu.RUnlock()

	cutoff := time.Now().Add(-duration)
	result := make([]Anomaly, 0)

	for _, anomaly := range ad.anomalies {
		if anomaly.Timestamp.After(cutoff) {
			result = append(result, anomaly)
		}
	}

	return result
}

// calculateSeverity calculates severity based on z-score.
func (ad *AnomalyDetector) calculateSeverity(zscore float64) string {
	absZ := math.Abs(zscore)
	switch {
	case absZ > 5.0:
		return "critical"
	case absZ > 4.0:
		return "high"
	case absZ > 3.5:
		return "medium"
	default:
		return "low"
	}
}

// HistoricalStats maintains rolling statistics for anomaly detection.
//
// Design: Uses Welford's online algorithm for numerically stable
// variance calculation. O(1) space, O(1) per update.
type HistoricalStats struct {
	values []float64
	count  int
	index  int
	mean   float64
	m2     float64 // Sum of squared differences from mean
}

// NewHistoricalStats creates a new historical stats tracker.
func NewHistoricalStats(capacity int) *HistoricalStats {
	return &HistoricalStats{
		values: make([]float64, capacity),
		count:  0,
		index:  0,
	}
}

// Add adds a value and updates running statistics.
// Uses Welford's online algorithm for numerical stability.
// Complexity: O(1).
func (hs *HistoricalStats) Add(value float64) {
	if hs.count < len(hs.values) {
		hs.count++
	} else {
		// Remove influence of old value
		oldValue := hs.values[hs.index]
		oldMean := hs.mean
		hs.mean -= (oldValue - hs.mean) / float64(hs.count)
		hs.m2 -= (oldValue - oldMean) * (oldValue - hs.mean)
	}

	// Add new value
	hs.values[hs.index] = value
	oldMean := hs.mean
	hs.mean += (value - hs.mean) / float64(hs.count)
	hs.m2 += (value - oldMean) * (value - hs.mean)

	hs.index = (hs.index + 1) % len(hs.values)
}

// MeanStdDev returns the mean and standard deviation.
func (hs *HistoricalStats) MeanStdDev() (float64, float64) {
	if hs.count < 2 {
		return hs.mean, 0
	}

	variance := hs.m2 / float64(hs.count-1)
	stddev := math.Sqrt(variance)

	return hs.mean, stddev
}

// Count returns the number of samples.
func (hs *HistoricalStats) Count() int {
	return hs.count
}