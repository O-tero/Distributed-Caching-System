package models

import (
	"testing"
	"time"
)

func TestNewMetricSnapshot(t *testing.T) {
	latency := LatencySummary{
		Count: 100,
		Sum:   100 * time.Millisecond,
		Min:   1 * time.Millisecond,
		Max:   10 * time.Millisecond,
		P50:   5 * time.Millisecond,
		P95:   9 * time.Millisecond,
	}

	snapshot := NewMetricSnapshot(80, 20, 50, 10, 5, latency)

	if snapshot.CacheHits != 80 {
		t.Errorf("Expected 80 hits, got %d", snapshot.CacheHits)
	}

	if snapshot.CacheMisses != 20 {
		t.Errorf("Expected 20 misses, got %d", snapshot.CacheMisses)
	}

	expectedHitRate := 0.8
	if snapshot.HitRate != expectedHitRate {
		t.Errorf("Expected hit rate %.2f, got %.2f", expectedHitRate, snapshot.HitRate)
	}

	expectedMissRate := 0.2
	if snapshot.MissRate != expectedMissRate {
		t.Errorf("Expected miss rate %.2f, got %.2f", expectedMissRate, snapshot.MissRate)
	}
}

func TestMergeSnapshots(t *testing.T) {
	snapshot1 := MetricSnapshot{
		CacheHits:   100,
		CacheMisses: 20,
		Sets:        50,
		Deletes:     10,
		Evictions:   5,
		L1Size:      1000,
		Latency: LatencySummary{
			Count: 100,
			Sum:   500 * time.Millisecond,
			Min:   1 * time.Millisecond,
			Max:   50 * time.Millisecond,
			P50:   5 * time.Millisecond,
		},
	}

	snapshot2 := MetricSnapshot{
		CacheHits:   80,
		CacheMisses: 30,
		Sets:        40,
		Deletes:     8,
		Evictions:   3,
		L1Size:      800,
		Latency: LatencySummary{
			Count: 80,
			Sum:   400 * time.Millisecond,
			Min:   2 * time.Millisecond,
			Max:   40 * time.Millisecond,
			P50:   6 * time.Millisecond,
		},
	}

	merged := MergeSnapshots(snapshot1, snapshot2)

	if merged.CacheHits != 180 {
		t.Errorf("Expected 180 hits, got %d", merged.CacheHits)
	}

	if merged.CacheMisses != 50 {
		t.Errorf("Expected 50 misses, got %d", merged.CacheMisses)
	}

	if merged.L1Size != 1800 {
		t.Errorf("Expected L1 size 1800, got %d", merged.L1Size)
	}

	if merged.Latency.Count != 180 {
		t.Errorf("Expected latency count 180, got %d", merged.Latency.Count)
	}

	if merged.Latency.Sum != 900*time.Millisecond {
		t.Errorf("Expected latency sum 900ms, got %v", merged.Latency.Sum)
	}
}

func TestUpdateLatency(t *testing.T) {
	summary := LatencySummary{}

	// First sample
	UpdateLatency(&summary, 5*time.Millisecond)

	if summary.Count != 1 {
		t.Errorf("Expected count 1, got %d", summary.Count)
	}

	if summary.Min != 5*time.Millisecond {
		t.Errorf("Expected min 5ms, got %v", summary.Min)
	}

	if summary.Max != 5*time.Millisecond {
		t.Errorf("Expected max 5ms, got %v", summary.Max)
	}

	// Add more samples
	UpdateLatency(&summary, 2*time.Millisecond)
	UpdateLatency(&summary, 10*time.Millisecond)

	if summary.Count != 3 {
		t.Errorf("Expected count 3, got %d", summary.Count)
	}

	if summary.Min != 2*time.Millisecond {
		t.Errorf("Expected min 2ms, got %v", summary.Min)
	}

	if summary.Max != 10*time.Millisecond {
		t.Errorf("Expected max 10ms, got %v", summary.Max)
	}

	if summary.Sum != 17*time.Millisecond {
		t.Errorf("Expected sum 17ms, got %v", summary.Sum)
	}
}

func TestCalculateLatencySummary(t *testing.T) {
	samples := []time.Duration{
		1 * time.Millisecond,
		2 * time.Millisecond,
		3 * time.Millisecond,
		4 * time.Millisecond,
		5 * time.Millisecond,
		6 * time.Millisecond,
		7 * time.Millisecond,
		8 * time.Millisecond,
		9 * time.Millisecond,
		10 * time.Millisecond,
	}

	summary := CalculateLatencySummary(samples)

	if summary.Count != 10 {
		t.Errorf("Expected count 10, got %d", summary.Count)
	}

	if summary.Min != 1*time.Millisecond {
		t.Errorf("Expected min 1ms, got %v", summary.Min)
	}

	if summary.Max != 10*time.Millisecond {
		t.Errorf("Expected max 10ms, got %v", summary.Max)
	}

	// P50 should be around 5-6ms
	if summary.P50 < 4*time.Millisecond || summary.P50 > 6*time.Millisecond {
		t.Errorf("Expected P50 around 5ms, got %v", summary.P50)
	}

	// P99 should be around 10ms
	if summary.P99 < 9*time.Millisecond || summary.P99 > 10*time.Millisecond {
		t.Errorf("Expected P99 around 10ms, got %v", summary.P99)
	}
}

func TestLatencySummary_AvgLatency(t *testing.T) {
	summary := LatencySummary{
		Count: 10,
		Sum:   100 * time.Millisecond,
	}

	avg := summary.AvgLatency()
	expected := 10 * time.Millisecond

	if avg != expected {
		t.Errorf("Expected avg %v, got %v", expected, avg)
	}

	// Empty summary
	empty := LatencySummary{}
	if empty.AvgLatency() != 0 {
		t.Error("Expected 0 for empty summary")
	}
}

func TestSnapshotToPrometheusFormat(t *testing.T) {
	snapshot := MetricSnapshot{
		CacheHits:   100,
		CacheMisses: 20,
		Sets:        50,
		Deletes:     10,
		Evictions:   5,
		HitRate:     0.833,
		L1Size:      1000,
		Latency: LatencySummary{
			Count: 100,
			P50:   5 * time.Millisecond,
			P95:   20 * time.Millisecond,
		},
	}

	metrics := SnapshotToPrometheusFormat(snapshot, "cache")

	// Check key metrics exist
	if _, ok := metrics["cache_hits_total"]; !ok {
		t.Error("Missing cache_hits_total metric")
	}

	if _, ok := metrics["cache_hit_rate"]; !ok {
		t.Error("Missing cache_hit_rate metric")
	}

	if _, ok := metrics["cache_latency_p95_ms"]; !ok {
		t.Error("Missing cache_latency_p95_ms metric")
	}

	// Verify values
	if metrics["cache_hits_total"] != 100 {
		t.Errorf("Expected hits 100, got %v", metrics["cache_hits_total"])
	}

	if metrics["cache_hit_rate"] != 0.833 {
		t.Errorf("Expected hit rate 0.833, got %v", metrics["cache_hit_rate"])
	}
}

func BenchmarkMergeSnapshots(b *testing.B) {
	snapshot1 := MetricSnapshot{
		CacheHits:   100,
		CacheMisses: 20,
		Latency: LatencySummary{
			Count: 100,
			Sum:   500 * time.Millisecond,
		},
	}

	snapshot2 := MetricSnapshot{
		CacheHits:   80,
		CacheMisses: 30,
		Latency: LatencySummary{
			Count: 80,
			Sum:   400 * time.Millisecond,
		},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = MergeSnapshots(snapshot1, snapshot2)
	}
}

func BenchmarkCalculateLatencySummary(b *testing.B) {
	samples := make([]time.Duration, 1000)
	for i := 0; i < 1000; i++ {
		samples[i] = time.Duration(i) * time.Microsecond
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = CalculateLatencySummary(samples)
	}
}	