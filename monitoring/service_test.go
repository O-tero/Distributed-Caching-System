package monitoring

import (
	"context"
	"sync"
	"testing"
	"time"
)

func TestMetricsCollector_RecordMetric(t *testing.T) {
	collector := NewMetricsCollector(DefaultConfig())

	// Record cache hit
	collector.RecordMetric(MetricEvent{
		Type:      MetricCacheHit,
		Value:     1,
		Timestamp: time.Now(),
		Source:    "cache-manager",
	})

	// Record cache miss
	collector.RecordMetric(MetricEvent{
		Type:      MetricCacheMiss,
		Value:     1,
		Timestamp: time.Now(),
		Source:    "cache-manager",
	})

	// Verify counters
	counters := collector.GetCounters()
	if counters.CacheHits != 1 {
		t.Errorf("Expected 1 cache hit, got %d", counters.CacheHits)
	}
	if counters.CacheMisses != 1 {
		t.Errorf("Expected 1 cache miss, got %d", counters.CacheMisses)
	}
}

func TestMetricsCollector_Latency(t *testing.T) {
	collector := NewMetricsCollector(DefaultConfig())

	// Record latency samples
	latencies := []float64{10, 20, 30, 40, 50, 60, 70, 80, 90, 100}
	for _, lat := range latencies {
		collector.RecordMetric(MetricEvent{
			Type:      MetricLatency,
			Value:     lat,
			Timestamp: time.Now(),
			Source:    "cache-manager",
		})
	}

	// Get latency stats
	stats := collector.GetLatencyStats()

	if stats.Count != 10 {
		t.Errorf("Expected 10 samples, got %d", stats.Count)
	}

	if stats.Min != 10 {
		t.Errorf("Expected min 10, got %.2f", stats.Min)
	}

	if stats.Max != 100 {
		t.Errorf("Expected max 100, got %.2f", stats.Max)
	}

	if stats.Avg != 55 {
		t.Errorf("Expected avg 55, got %.2f", stats.Avg)
	}

	// P50 should be around 50
	if stats.P50 < 45 || stats.P50 > 55 {
		t.Errorf("Expected P50 around 50, got %.2f", stats.P50)
	}
}

func TestMetricsCollector_Concurrency(t *testing.T) {
	collector := NewMetricsCollector(DefaultConfig())

	// Concurrent writes
	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 1000; j++ {
				collector.RecordMetric(MetricEvent{
					Type:      MetricCacheHit,
					Value:     1,
					Timestamp: time.Now(),
					Source:    "test",
				})
			}
		}()
	}

	wg.Wait()

	// Verify count
	counters := collector.GetCounters()
	if counters.CacheHits != 100000 {
		t.Errorf("Expected 100000 hits, got %d", counters.CacheHits)
	}
}

func TestRingBuffer_AddGet(t *testing.T) {
	buffer := NewRingBuffer(10)

	// Add samples
	for i := 0; i < 5; i++ {
		buffer.Add(float64(i), time.Now())
	}

	// Get all
	samples := buffer.GetAll()
	if len(samples) != 5 {
		t.Errorf("Expected 5 samples, got %d", len(samples))
	}

	// Verify values
	for i := 0; i < 5; i++ {
		if samples[i].Value != float64(i) {
			t.Errorf("Expected value %d, got %.2f", i, samples[i].Value)
		}
	}
}

func TestRingBuffer_Overflow(t *testing.T) {
	buffer := NewRingBuffer(5)

	// Add more than capacity
	for i := 0; i < 10; i++ {
		buffer.Add(float64(i), time.Now())
	}

	// Should only keep last 5
	samples := buffer.GetAll()
	if len(samples) > 5 {
		t.Errorf("Expected at most 5 samples, got %d", len(samples))
	}

	// Latest values should be 5-9
	lastValue := samples[len(samples)-1].Value
	if lastValue != 9 {
		t.Errorf("Expected last value 9, got %.2f", lastValue)
	}
}

func TestRingBuffer_Concurrent(t *testing.T) {
	buffer := NewRingBuffer(1000)

	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				buffer.Add(float64(id*100+j), time.Now())
			}
		}(i)
	}

	wg.Wait()

	// Should have samples (may have some overwrites due to concurrency)
	samples := buffer.GetAll()
	if len(samples) == 0 {
		t.Error("Expected some samples")
	}
}

func TestTimeSeries_AddGet(t *testing.T) {
	ts := NewTimeSeries(1 * time.Hour)

	now := time.Now()

	// Add events
	for i := 0; i < 10; i++ {
		ts.Add(MetricEvent{
			Type:      MetricCacheHit,
			Value:     1,
			Timestamp: now.Add(time.Duration(i) * time.Second),
			Source:    "test",
		})
	}

	// Get range
	buckets := ts.GetRange(now, now.Add(10*time.Second))

	if len(buckets) < 5 {
		t.Errorf("Expected at least 5 buckets, got %d", len(buckets))
	}
}

func TestAggregator_BasicAggregation(t *testing.T) {
	collector := NewMetricsCollector(DefaultConfig())
	aggregator := NewAggregator(collector, DefaultConfig())

	// Record some metrics
	for i := 0; i < 100; i++ {
		collector.RecordMetric(MetricEvent{
			Type:      MetricCacheHit,
			Value:     1,
			Timestamp: time.Now(),
			Source:    "test",
		})
	}

	for i := 0; i < 50; i++ {
		collector.RecordMetric(MetricEvent{
			Type:      MetricCacheMiss,
			Value:     1,
			Timestamp: time.Now(),
			Source:    "test",
		})
	}

	// Get stats
	now := time.Now()
	stats := aggregator.GetStats(now.Add(-1*time.Minute), now)

	if stats.CacheHits != 100 {
		t.Errorf("Expected 100 hits, got %d", stats.CacheHits)
	}

	if stats.CacheMisses != 50 {
		t.Errorf("Expected 50 misses, got %d", stats.CacheMisses)
	}

	expectedHitRate := 100.0 / 150.0
	if stats.HitRate < expectedHitRate-0.01 || stats.HitRate > expectedHitRate+0.01 {
		t.Errorf("Expected hit rate %.2f, got %.2f", expectedHitRate, stats.HitRate)
	}
}

func TestSlidingWindow_AddGet(t *testing.T) {
	window := NewSlidingWindow(10 * time.Second)

	now := time.Now()

	// Add snapshots
	for i := 0; i < 5; i++ {
		window.Add(AggregatedStats{
			Timestamp:     now.Add(time.Duration(i) * time.Second),
			TotalRequests: int64(i * 10),
			CacheHits:     int64(i * 8),
			CacheMisses:   int64(i * 2),
		})
	}

	// Get latest
	latest := window.GetLatest()
	if latest.TotalRequests != 40 {
		t.Errorf("Expected 40 requests, got %d", latest.TotalRequests)
	}

	// Get range
	snapshots := window.GetRange(now, now.Add(5*time.Second))
	if len(snapshots) != 5 {
		t.Errorf("Expected 5 snapshots, got %d", len(snapshots))
	}
}

func TestAnomalyDetector_LatencySpike(t *testing.T) {
	detector := NewAnomalyDetector()

	// Add normal latency samples
	for i := 0; i < 50; i++ {
		detector.Detect(AggregatedStats{
			P95Latency: 10.0,
		})
	}

	// Add spike
	detector.Detect(AggregatedStats{
		P95Latency: 100.0, // 10x normal
	})

	// Check for anomalies
	anomalies := detector.GetRecentAnomalies(1 * time.Minute)
	if len(anomalies) == 0 {
		t.Error("Expected latency spike anomaly")
	}

	// Verify anomaly type
	found := false
	for _, anomaly := range anomalies {
		if anomaly.Type == AnomalyLatencySpike {
			found = true
			if anomaly.Severity != "critical" && anomaly.Severity != "high" {
				t.Errorf("Expected high/critical severity, got %s", anomaly.Severity)
			}
		}
	}

	if !found {
		t.Error("Expected latency spike anomaly type")
	}
}

func TestAnomalyDetector_HitRateDrop(t *testing.T) {
	detector := NewAnomalyDetector()

	// Add normal hit rate samples
	for i := 0; i < 50; i++ {
		detector.Detect(AggregatedStats{
			HitRate: 0.9,
		})
	}

	// Add drop
	detector.Detect(AggregatedStats{
		HitRate: 0.3, // Dropped to 30%
	})

	// Check for anomalies
	anomalies := detector.GetRecentAnomalies(1 * time.Minute)
	if len(anomalies) == 0 {
		t.Error("Expected hit rate drop anomaly")
	}

	// Verify anomaly type
	found := false
	for _, anomaly := range anomalies {
		if anomaly.Type == AnomalyHitRateDrop {
			found = true
		}
	}

	if !found {
		t.Error("Expected hit rate drop anomaly type")
	}
}

func TestHistoricalStats_WelfordAlgorithm(t *testing.T) {
	stats := NewHistoricalStats(100)

	// Add values: 10, 20, 30, 40, 50
	values := []float64{10, 20, 30, 40, 50}
	for _, v := range values {
		stats.Add(v)
	}

	mean, stddev := stats.MeanStdDev()

	// Expected mean: 30
	if mean != 30 {
		t.Errorf("Expected mean 30, got %.2f", mean)
	}

	// Expected stddev: ~14.14 (sample stddev)
	expectedStddev := 15.81 // sqrt(250)
	if stddev < expectedStddev-1 || stddev > expectedStddev+1 {
		t.Errorf("Expected stddev around %.2f, got %.2f", expectedStddev, stddev)
	}
}

func TestAlertManager_TriggerResolve(t *testing.T) {
	collector := NewMetricsCollector(DefaultConfig())
	aggregator := NewAggregator(collector, DefaultConfig())
	alertMgr := NewAlertManager(aggregator, DefaultConfig())

	// Manually trigger an alert
	alert := &Alert{
		ID:       "test_alert",
		Type:     AlertHighErrorRate,
		Severity: "critical",
		Message:  "Test alert",
	}

	alertMgr.triggerAlert(alert)

	// Verify active
	activeAlerts := alertMgr.GetActiveAlerts()
	if len(activeAlerts) != 1 {
		t.Errorf("Expected 1 active alert, got %d", len(activeAlerts))
	}

	// Resolve alert
	alertMgr.resolveAlert("test_alert")

	// Verify resolved
	activeAlerts = alertMgr.GetActiveAlerts()
	if len(activeAlerts) != 0 {
		t.Errorf("Expected 0 active alerts, got %d", len(activeAlerts))
	}

	resolvedAlerts := alertMgr.GetRecentResolvedAlerts(10)
	if len(resolvedAlerts) != 1 {
		t.Errorf("Expected 1 resolved alert, got %d", len(resolvedAlerts))
	}
}

func TestHighErrorRateRule(t *testing.T) {
	rule := NewHighErrorRateRule()

	// Normal error rate
	stats := AggregatedStats{
		ErrorRate: 0.01, // 1% - below threshold
	}

	alert := rule.Evaluate(stats)
	if alert != nil {
		t.Error("Should not trigger alert for normal error rate")
	}

	// High error rate
	stats.ErrorRate = 0.10 // 10% - above threshold

	alert = rule.Evaluate(stats)
	if alert == nil {
		t.Error("Should trigger alert for high error rate")
	}

	if alert.Type != AlertHighErrorRate {
		t.Errorf("Expected AlertHighErrorRate, got %s", alert.Type)
	}

	if alert.Severity != "critical" {
		t.Errorf("Expected critical severity, got %s", alert.Severity)
	}
}

func TestLowHitRateRule(t *testing.T) {
	rule := NewLowHitRateRule()

	// Normal hit rate
	stats := AggregatedStats{
		TotalRequests: 1000,
		HitRate:       0.85, // 85% - above threshold
	}

	alert := rule.Evaluate(stats)
	if alert != nil {
		t.Error("Should not trigger alert for normal hit rate")
	}

	// Low hit rate
	stats.HitRate = 0.50 // 50% - below threshold

	alert = rule.Evaluate(stats)
	if alert == nil {
		t.Error("Should trigger alert for low hit rate")
	}

	if alert.Type != AlertLowHitRate {
		t.Errorf("Expected AlertLowHitRate, got %s", alert.Type)
	}
}

func TestLatencySpikeRule(t *testing.T) {
	rule := NewLatencySpikeRule()

	// Normal latency
	stats := AggregatedStats{
		P95Latency: 50.0, // 50ms - below threshold
	}

	alert := rule.Evaluate(stats)
	if alert != nil {
		t.Error("Should not trigger alert for normal latency")
	}

	// High latency
	stats.P95Latency = 150.0 // 150ms - above threshold

	alert = rule.Evaluate(stats)
	if alert == nil {
		t.Error("Should trigger alert for high latency")
	}

	if alert.Type != AlertLatencySpike {
		t.Errorf("Expected AlertLatencySpike, got %s", alert.Type)
	}
}

func TestService_GetMetrics(t *testing.T) {
	svc, _ := initService()
	ctx := context.Background()

	// Record some metrics
	for i := 0; i < 100; i++ {
		svc.collector.RecordMetric(MetricEvent{
			Type:      MetricCacheHit,
			Value:     1,
			Timestamp: time.Now(),
			Source:    "test",
		})
	}

	// Get metrics
	req := &GetMetricsRequest{
		Window: "1m",
	}

	resp, err := svc.GetMetrics(ctx, req)
	if err != nil {
		t.Fatalf("GetMetrics failed: %v", err)
	}

	if resp.CacheHits != 100 {
		t.Errorf("Expected 100 hits, got %d", resp.CacheHits)
	}

	if resp.Window != "1m" {
		t.Errorf("Expected 1m window, got %v", resp.Window)
	}
}

func TestService_GetAggregated(t *testing.T) {
	svc, _ := initService()
	ctx := context.Background()

	now := time.Now()

	// Record metrics over time
	for i := 0; i < 60; i++ {
		timestamp := now.Add(time.Duration(i) * time.Second)
		svc.collector.RecordMetric(MetricEvent{
			Type:      MetricCacheHit,
			Value:     1,
			Timestamp: timestamp,
			Source:    "test",
		})
	}

	// Get aggregated data
	req := &GetAggregatedRequest{
		StartTime: now,
		EndTime:   now.Add(1 * time.Minute),
		Interval:  "10s",
	}

	resp, err := svc.GetAggregated(ctx, req)
	if err != nil {
		t.Fatalf("GetAggregated failed: %v", err)
	}

	if len(resp.DataPoints) == 0 {
		t.Error("Expected data points")
	}
}

func TestService_GetAlerts(t *testing.T) {
	svc, _ := initService()
	ctx := context.Background()

	// Trigger an alert manually
	svc.alertMgr.triggerAlert(&Alert{
		ID:       "test_alert",
		Type:     AlertHighErrorRate,
		Severity: "critical",
		Message:  "Test alert",
	})

	// Get alerts
	resp, err := svc.GetAlerts(ctx)
	if err != nil {
		t.Fatalf("GetAlerts failed: %v", err)
	}

	if len(resp.ActiveAlerts) != 1 {
		t.Errorf("Expected 1 active alert, got %d", len(resp.ActiveAlerts))
	}

	if resp.AlertStats.TotalTriggered != 1 {
		t.Errorf("Expected 1 triggered alert, got %d", resp.AlertStats.TotalTriggered)
	}
}

// Benchmarks

func BenchmarkMetricsCollector_RecordMetric(b *testing.B) {
	collector := NewMetricsCollector(DefaultConfig())
	event := MetricEvent{
		Type:      MetricCacheHit,
		Value:     1,
		Timestamp: time.Now(),
		Source:    "bench",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		collector.RecordMetric(event)
	}
}

func BenchmarkMetricsCollector_RecordMetricParallel(b *testing.B) {
	collector := NewMetricsCollector(DefaultConfig())

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		event := MetricEvent{
			Type:      MetricCacheHit,
			Value:     1,
			Timestamp: time.Now(),
			Source:    "bench",
		}
		for pb.Next() {
			collector.RecordMetric(event)
		}
	})
}

func BenchmarkRingBuffer_Add(b *testing.B) {
	buffer := NewRingBuffer(10000)
	now := time.Now()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		buffer.Add(float64(i), now)
	}
}

func BenchmarkRingBuffer_AddParallel(b *testing.B) {
	buffer := NewRingBuffer(10000)

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			buffer.Add(float64(i), time.Now())
			i++
		}
	})
}

func BenchmarkCalculateLatencyStats(b *testing.B) {
	samples := make([]Sample, 1000)
	for i := 0; i < 1000; i++ {
		samples[i] = Sample{Value: float64(i)}
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		calculateLatencyStats(samples)
	}
}

func BenchmarkAnomalyDetector_Detect(b *testing.B) {
	detector := NewAnomalyDetector()

	stats := AggregatedStats{
		HitRate:    0.8,
		P95Latency: 50.0,
		ErrorRate:  0.01,
		QPS:        1000.0,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		detector.Detect(stats)
	}
}