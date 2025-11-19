// Package monitoring provides comprehensive observability for the distributed caching system.
//
// Design Philosophy:
// - Lock-free or minimal-lock metrics collection for high throughput
// - Sliding window aggregation for real-time statistics
// - Anomaly detection for proactive alerting
// - Low memory overhead with bounded buffers
//
// Performance Characteristics:
// - Metrics ingestion: >1M events/sec per core
// - Aggregation latency: <1ms for 1-second windows
// - Memory overhead: ~10MB for 1 hour of metrics at 10K events/sec
// - GC pressure: Minimal via object pooling and preallocated buffers
//
// Architecture:
// - Event-driven ingestion via Pub/Sub subscriptions
// - In-memory time-series store with circular buffers
// - Real-time aggregation with configurable windows
// - Anomaly detection using statistical methods
// - Alert engine with threshold-based and dynamic rules
package monitoring

import (
	"context"
	"errors"
	"sync"
	"time"

	"encore.dev/pubsub"
)

//encore:service
type Service struct {
	collector  *MetricsCollector
	aggregator *Aggregator
	alertMgr   *AlertManager
	config     Config
	mu         sync.RWMutex
}

// Config holds monitoring service configuration.
type Config struct {
	MetricsRetention  time.Duration // How long to keep raw metrics
	AggregationWindow time.Duration // Aggregation window size
	AlertEvalInterval time.Duration // How often to evaluate alerts
	MaxMetricsPerSec  int           // Rate limit for metric ingestion
}

// DefaultConfig returns sensible default configuration.
func DefaultConfig() Config {
	return Config{
		MetricsRetention:  1 * time.Hour,
		AggregationWindow: 1 * time.Second,
		AlertEvalInterval: 10 * time.Second,
		MaxMetricsPerSec:  1000000, // 1M events/sec
	}
}

// MetricType represents the type of metric being recorded.
type MetricType string

const (
	MetricCacheHit        MetricType = "cache.hit"
	MetricCacheMiss       MetricType = "cache.miss"
	MetricCacheSet        MetricType = "cache.set"
	MetricCacheDelete     MetricType = "cache.delete"
	MetricCacheEviction   MetricType = "cache.eviction"
	MetricInvalidation    MetricType = "invalidation"
	MetricWarming         MetricType = "warming"
	MetricError           MetricType = "error"
	MetricLatency         MetricType = "latency"
)

// MetricEvent represents a single metric event from any service.
type MetricEvent struct {
	Type      MetricType             `json:"type"`
	Value     float64                `json:"value"`
	Timestamp time.Time              `json:"timestamp"`
	Source    string                 `json:"source"` // "cache-manager", "warming", "invalidation"
	Labels    map[string]string      `json:"labels,omitempty"`
}

// Request and response types

type GetMetricsRequest struct {
	Window time.Duration `json:"window"` // Time window (e.g., 1m, 5m, 1h)
}

type GetMetricsResponse struct {
	Timestamp      time.Time              `json:"timestamp"`
	Window         time.Duration          `json:"window"`
	TotalRequests  int64                  `json:"total_requests"`
	CacheHits      int64                  `json:"cache_hits"`
	CacheMisses    int64                  `json:"cache_misses"`
	HitRate        float64                `json:"hit_rate"`
	QPS            float64                `json:"qps"`
	AvgLatency     float64                `json:"avg_latency_ms"`
	P50Latency     float64                `json:"p50_latency_ms"`
	P90Latency     float64                `json:"p90_latency_ms"`
	P95Latency     float64                `json:"p95_latency_ms"`
	P99Latency     float64                `json:"p99_latency_ms"`
	ErrorRate      float64                `json:"error_rate"`
	Invalidations  int64                  `json:"invalidations"`
	Warmings       int64                  `json:"warmings"`
	Evictions      int64                  `json:"evictions"`
}

type GetAggregatedRequest struct {
	StartTime time.Time     `json:"start_time"`
	EndTime   time.Time     `json:"end_time"`
	Interval  time.Duration `json:"interval"` // Aggregation interval
}

type AggregatedDataPoint struct {
	Timestamp     time.Time `json:"timestamp"`
	Requests      int64     `json:"requests"`
	HitRate       float64   `json:"hit_rate"`
	AvgLatency    float64   `json:"avg_latency_ms"`
	P95Latency    float64   `json:"p95_latency_ms"`
	QPS           float64   `json:"qps"`
	ErrorRate     float64   `json:"error_rate"`
}

type GetAggregatedResponse struct {
	DataPoints []AggregatedDataPoint `json:"data_points"`
	Summary    GetMetricsResponse    `json:"summary"`
}

type GetAlertsResponse struct {
	ActiveAlerts   []Alert   `json:"active_alerts"`
	RecentAlerts   []Alert   `json:"recent_alerts"`   // Last 10 resolved alerts
	AlertStats     AlertStats `json:"alert_stats"`
}

type AlertStats struct {
	TotalTriggered int64   `json:"total_triggered"`
	TotalResolved  int64   `json:"total_resolved"`
	ActiveCount    int     `json:"active_count"`
	AvgDuration    float64 `json:"avg_duration_seconds"`
}

// Global service instance
var svc *Service

// initService initializes the monitoring service.
func initService() (*Service, error) {
	config := DefaultConfig()

	collector := NewMetricsCollector(config)
	aggregator := NewAggregator(collector, config)
	alertMgr := NewAlertManager(aggregator, config)

	s := &Service{
		collector:  collector,
		aggregator: aggregator,
		alertMgr:   alertMgr,
		config:     config,
	}

	// Start background workers
	go aggregator.Run()
	go alertMgr.Run()

	return s, nil
}

func init() {
	var err error
	svc, err = initService()
	if err != nil {
		panic(err)
	}
}

// GetMetrics returns current metrics snapshot for a time window.
//encore:api public method=GET path=/monitoring/metrics
func GetMetrics(ctx context.Context, req *GetMetricsRequest) (*GetMetricsResponse, error) {
	if svc == nil {
		return nil, errors.New("service not initialized")
	}
	return svc.GetMetrics(ctx, req)
}

func (s *Service) GetMetrics(ctx context.Context, req *GetMetricsRequest) (*GetMetricsResponse, error) {
	window := req.Window
	if window == 0 {
		window = 1 * time.Minute // Default window
	}

	// Get aggregated data for the window
	now := time.Now()
	startTime := now.Add(-window)

	stats := s.aggregator.GetStats(startTime, now)

	return &GetMetricsResponse{
		Timestamp:      now,
		Window:         window,
		TotalRequests:  stats.TotalRequests,
		CacheHits:      stats.CacheHits,
		CacheMisses:    stats.CacheMisses,
		HitRate:        stats.HitRate,
		QPS:            stats.QPS,
		AvgLatency:     stats.AvgLatency,
		P50Latency:     stats.P50Latency,
		P90Latency:     stats.P90Latency,
		P95Latency:     stats.P95Latency,
		P99Latency:     stats.P99Latency,
		ErrorRate:      stats.ErrorRate,
		Invalidations:  stats.Invalidations,
		Warmings:       stats.Warmings,
		Evictions:      stats.Evictions,
	}, nil
}

// GetAggregated returns time-series aggregated metrics.
//encore:api public method=POST path=/monitoring/aggregated
func GetAggregated(ctx context.Context, req *GetAggregatedRequest) (*GetAggregatedResponse, error) {
	if svc == nil {
		return nil, errors.New("service not initialized")
	}
	return svc.GetAggregated(ctx, req)
}

func (s *Service) GetAggregated(ctx context.Context, req *GetAggregatedRequest) (*GetAggregatedResponse, error) {
	// Validate request
	if req.EndTime.Before(req.StartTime) {
		return nil, errors.New("end_time must be after start_time")
	}

	interval := req.Interval
	if interval == 0 {
		interval = 1 * time.Minute // Default interval
	}

	// Generate data points
	dataPoints := make([]AggregatedDataPoint, 0)
	currentTime := req.StartTime

	for currentTime.Before(req.EndTime) {
		nextTime := currentTime.Add(interval)
		if nextTime.After(req.EndTime) {
			nextTime = req.EndTime
		}

		stats := s.aggregator.GetStats(currentTime, nextTime)

		dataPoints = append(dataPoints, AggregatedDataPoint{
			Timestamp:  currentTime,
			Requests:   stats.TotalRequests,
			HitRate:    stats.HitRate,
			AvgLatency: stats.AvgLatency,
			P95Latency: stats.P95Latency,
			QPS:        stats.QPS,
			ErrorRate:  stats.ErrorRate,
		})

		currentTime = nextTime
	}

	// Calculate overall summary
	overallStats := s.aggregator.GetStats(req.StartTime, req.EndTime)
	summary := &GetMetricsResponse{
		Timestamp:      req.EndTime,
		Window:         req.EndTime.Sub(req.StartTime),
		TotalRequests:  overallStats.TotalRequests,
		CacheHits:      overallStats.CacheHits,
		CacheMisses:    overallStats.CacheMisses,
		HitRate:        overallStats.HitRate,
		QPS:            overallStats.QPS,
		AvgLatency:     overallStats.AvgLatency,
		P50Latency:     overallStats.P50Latency,
		P90Latency:     overallStats.P90Latency,
		P95Latency:     overallStats.P95Latency,
		P99Latency:     overallStats.P99Latency,
		ErrorRate:      overallStats.ErrorRate,
		Invalidations:  overallStats.Invalidations,
		Warmings:       overallStats.Warmings,
		Evictions:      overallStats.Evictions,
	}

	return &GetAggregatedResponse{
		DataPoints: dataPoints,
		Summary:    *summary,
	}, nil
}

// GetAlerts returns current active alerts and alert statistics.
//encore:api public method=GET path=/monitoring/alerts
func GetAlerts(ctx context.Context) (*GetAlertsResponse, error) {
	if svc == nil {
		return nil, errors.New("service not initialized")
	}
	return svc.GetAlerts(ctx)
}

func (s *Service) GetAlerts(ctx context.Context) (*GetAlertsResponse, error) {
	activeAlerts := s.alertMgr.GetActiveAlerts()
	recentAlerts := s.alertMgr.GetRecentResolvedAlerts(10)
	stats := s.alertMgr.GetStats()

	return &GetAlertsResponse{
		ActiveAlerts: activeAlerts,
		RecentAlerts: recentAlerts,
		AlertStats:   stats,
	}, nil
}

// Pub/Sub subscriptions for metric events

// Subscribe to cache-manager metrics
var _ = pubsub.NewSubscription(
	CacheMetricsTopic,
	"monitoring-cache-metrics",
	pubsub.SubscriptionConfig[*CacheMetricEvent]{
		Handler: HandleCacheMetric,
	},
)

// CacheMetricEvent represents a metric event from cache-manager.
type CacheMetricEvent struct {
	Operation string    `json:"operation"` // "get", "set", "delete", "invalidate"
	Key       string    `json:"key"`
	Hit       bool      `json:"hit"`
	Latency   float64   `json:"latency"` // Milliseconds
	Size      int       `json:"size"`
	Timestamp time.Time `json:"timestamp"`
	Instance  string    `json:"instance"`
}

var CacheMetricsTopic = pubsub.NewTopic[*CacheMetricEvent](
	"cache-metrics",
	pubsub.TopicConfig{
		DeliveryGuarantee: pubsub.AtLeastOnce,
	},
)

// HandleCacheMetric processes cache metrics from cache-manager.
func HandleCacheMetric(ctx context.Context, event *CacheMetricEvent) error {
	if svc == nil {
		return nil
	}

	// Record hit/miss
	if event.Operation == "get" {
		if event.Hit {
			svc.collector.RecordMetric(MetricEvent{
				Type:      MetricCacheHit,
				Value:     1,
				Timestamp: event.Timestamp,
				Source:    "cache-manager",
			})
		} else {
			svc.collector.RecordMetric(MetricEvent{
				Type:      MetricCacheMiss,
				Value:     1,
				Timestamp: event.Timestamp,
				Source:    "cache-manager",
			})
		}
	}

	// Record operation
	switch event.Operation {
	case "set":
		svc.collector.RecordMetric(MetricEvent{
			Type:      MetricCacheSet,
			Value:     1,
			Timestamp: event.Timestamp,
			Source:    "cache-manager",
		})
	case "delete":
		svc.collector.RecordMetric(MetricEvent{
			Type:      MetricCacheDelete,
			Value:     1,
			Timestamp: event.Timestamp,
			Source:    "cache-manager",
		})
	}

	// Record latency
	if event.Latency > 0 {
		svc.collector.RecordMetric(MetricEvent{
			Type:      MetricLatency,
			Value:     event.Latency,
			Timestamp: event.Timestamp,
			Source:    "cache-manager",
			Labels:    map[string]string{"operation": event.Operation},
		})
	}

	return nil
}

// Subscribe to warming completion events
var _ = pubsub.NewSubscription(
	WarmCompletedTopic,
	"monitoring-warm-completed",
	pubsub.SubscriptionConfig[*WarmCompletedEvent]{
		Handler: HandleWarmCompleted,
	},
)

// WarmCompletedEvent represents a warming completion event.
type WarmCompletedEvent struct {
	Key        string    `json:"key"`
	Status     string    `json:"status"`
	DurationMs int64     `json:"duration_ms"`
	Strategy   string    `json:"strategy"`
	Timestamp  time.Time `json:"timestamp"`
}

var WarmCompletedTopic = pubsub.NewTopic[*WarmCompletedEvent](
	"cache-warm-completed",
	pubsub.TopicConfig{
		DeliveryGuarantee: pubsub.AtLeastOnce,
	},
)

// HandleWarmCompleted processes warming completion events.
func HandleWarmCompleted(ctx context.Context, event *WarmCompletedEvent) error {
	if svc == nil {
		return nil
	}

	svc.collector.RecordMetric(MetricEvent{
		Type:      MetricWarming,
		Value:     1,
		Timestamp: event.Timestamp,
		Source:    "warming",
		Labels:    map[string]string{"status": event.Status, "strategy": event.Strategy},
	})

	// Record warming duration as latency
	svc.collector.RecordMetric(MetricEvent{
		Type:      MetricLatency,
		Value:     float64(event.DurationMs),
		Timestamp: event.Timestamp,
		Source:    "warming",
		Labels:    map[string]string{"operation": "warm"},
	})

	if event.Status != "success" {
		svc.collector.RecordMetric(MetricEvent{
			Type:      MetricError,
			Value:     1,
			Timestamp: event.Timestamp,
			Source:    "warming",
		})
	}

	return nil
}

// Subscribe to invalidation events
var _ = pubsub.NewSubscription(
	InvalidationMetricsTopic,
	"monitoring-invalidation",
	pubsub.SubscriptionConfig[*InvalidationMetricEvent]{
		Handler: HandleInvalidationMetric,
	},
)

// InvalidationMetricEvent represents an invalidation metric event.
type InvalidationMetricEvent struct {
	Pattern     string    `json:"pattern"`
	KeysCount   int       `json:"keys_count"`
	DurationMs  int64     `json:"duration_ms"`
	TriggeredBy string    `json:"triggered_by"`
	Timestamp   time.Time `json:"timestamp"`
}

var InvalidationMetricsTopic = pubsub.NewTopic[*InvalidationMetricEvent](
	"invalidation-metrics",
	pubsub.TopicConfig{
		DeliveryGuarantee: pubsub.AtLeastOnce,
	},
)

// HandleInvalidationMetric processes invalidation metrics.
func HandleInvalidationMetric(ctx context.Context, event *InvalidationMetricEvent) error {
	if svc == nil {
		return nil
	}

	svc.collector.RecordMetric(MetricEvent{
		Type:      MetricInvalidation,
		Value:     float64(event.KeysCount),
		Timestamp: event.Timestamp,
		Source:    "invalidation",
		Labels:    map[string]string{"triggered_by": event.TriggeredBy},
	})

	// Record invalidation latency
	svc.collector.RecordMetric(MetricEvent{
		Type:      MetricLatency,
		Value:     float64(event.DurationMs),
		Timestamp: event.Timestamp,
		Source:    "invalidation",
		Labels:    map[string]string{"operation": "invalidate"},
	})

	return nil
}

// Shutdown gracefully stops the monitoring service.
func (s *Service) Shutdown() {
	s.aggregator.Stop()
	s.alertMgr.Stop()
}