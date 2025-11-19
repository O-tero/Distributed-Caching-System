package monitoring

import (
	"fmt"
	"math"
	"sync"
	"sync/atomic"
	"time"
)

// AlertManager manages alert evaluation, triggering, and resolution.
//
// Design: Periodically evaluates alert rules against current metrics.
// Maintains active alert registry with automatic resolution when conditions normalize.
type AlertManager struct {
	aggregator *Aggregator
	config     Config

	// Alert rules
	rules []AlertRule

	// Active alerts
	mu            sync.RWMutex
	activeAlerts  map[string]*Alert
	resolvedAlerts []Alert

	// Statistics
	stats AlertManagerStats

	// Control
	stopChan chan struct{}
	wg       sync.WaitGroup
}

// AlertManagerStats tracks alert manager statistics.
type AlertManagerStats struct {
	TotalTriggered atomic.Int64
	TotalResolved  atomic.Int64
	TotalDuration  atomic.Int64 // Cumulative milliseconds
}

// Alert represents an active or resolved alert.
type Alert struct {
	ID          string
	Rule        string
	Type        AlertType
	Severity    string
	Metric      string
	CurrentValue float64
	Threshold   float64
	Message     string
	TriggeredAt time.Time
	ResolvedAt  *time.Time
	Duration    time.Duration
	Resolved    bool
}

// AlertType represents the category of alert.
type AlertType string

const (
	AlertHighErrorRate   AlertType = "high_error_rate"
	AlertLowHitRate      AlertType = "low_hit_rate"
	AlertLatencySpike    AlertType = "latency_spike"
	AlertAbnormalLoad    AlertType = "abnormal_load"
	AlertHighEvictionRate AlertType = "high_eviction_rate"
)

// AlertRule defines a condition that triggers an alert.
type AlertRule interface {
	ID() string
	Evaluate(stats AggregatedStats) *Alert
}

// NewAlertManager creates a new alert manager.
func NewAlertManager(aggregator *Aggregator, config Config) *AlertManager {
	am := &AlertManager{
		aggregator:     aggregator,
		config:         config,
		activeAlerts:   make(map[string]*Alert),
		resolvedAlerts: make([]Alert, 0),
		stopChan:       make(chan struct{}),
	}

	// Register default alert rules
	am.rules = []AlertRule{
		NewHighErrorRateRule(),
		NewLowHitRateRule(),
		NewLatencySpikeRule(),
		NewHighEvictionRateRule(),
	}

	return am
}

// Run starts the alert evaluation loop.
func (am *AlertManager) Run() {
	am.wg.Add(1)
	defer am.wg.Done()

	ticker := time.NewTicker(am.config.AlertEvalInterval)
	defer ticker.Stop()

	for {
		select {
		case <-am.stopChan:
			return
		case <-ticker.C:
			am.evaluateRules()
		}
	}
}

// evaluateRules evaluates all alert rules against current metrics.
func (am *AlertManager) evaluateRules() {
	// Get latest stats
	latest := am.aggregator.window1m.GetLatest()

	for _, rule := range am.rules {
		alert := rule.Evaluate(latest)

		if alert != nil {
			// Alert condition met
			am.triggerAlert(alert)
		} else {
			// Check if alert should be resolved
			am.resolveAlert(rule.ID())
		}
	}
}

// triggerAlert activates an alert or updates an existing one.
func (am *AlertManager) triggerAlert(alert *Alert) {
	am.mu.Lock()
	defer am.mu.Unlock()

	existing, exists := am.activeAlerts[alert.ID]
	if exists {
		// Update existing alert
		existing.CurrentValue = alert.CurrentValue
		existing.Message = alert.Message
	} else {
		// New alert
		alert.TriggeredAt = time.Now()
		am.activeAlerts[alert.ID] = alert
		am.stats.TotalTriggered.Add(1)
	}
}

// resolveAlert marks an alert as resolved if it exists.
func (am *AlertManager) resolveAlert(alertID string) {
	am.mu.Lock()
	defer am.mu.Unlock()

	alert, exists := am.activeAlerts[alertID]
	if !exists {
		return
	}

	// Mark as resolved
	now := time.Now()
	alert.ResolvedAt = &now
	alert.Duration = now.Sub(alert.TriggeredAt)
	alert.Resolved = true

	// Move to resolved alerts
	am.resolvedAlerts = append(am.resolvedAlerts, *alert)
	delete(am.activeAlerts, alertID)

	// Update stats
	am.stats.TotalResolved.Add(1)
	am.stats.TotalDuration.Add(alert.Duration.Milliseconds())

	// Keep only last 100 resolved alerts
	if len(am.resolvedAlerts) > 100 {
		am.resolvedAlerts = am.resolvedAlerts[len(am.resolvedAlerts)-100:]
	}
}

// GetActiveAlerts returns all currently active alerts.
func (am *AlertManager) GetActiveAlerts() []Alert {
	am.mu.RLock()
	defer am.mu.RUnlock()

	alerts := make([]Alert, 0, len(am.activeAlerts))
	for _, alert := range am.activeAlerts {
		alerts = append(alerts, *alert)
	}

	return alerts
}

// GetRecentResolvedAlerts returns the N most recent resolved alerts.
func (am *AlertManager) GetRecentResolvedAlerts(n int) []Alert {
	am.mu.RLock()
	defer am.mu.RUnlock()

	if n > len(am.resolvedAlerts) {
		n = len(am.resolvedAlerts)
	}

	result := make([]Alert, n)
	for i := 0; i < n; i++ {
		result[i] = am.resolvedAlerts[len(am.resolvedAlerts)-1-i]
	}

	return result
}

// GetStats returns alert manager statistics.
func (am *AlertManager) GetStats() AlertStats {
	triggered := am.stats.TotalTriggered.Load()
	resolved := am.stats.TotalResolved.Load()
	totalDuration := am.stats.TotalDuration.Load()

	avgDuration := 0.0
	if resolved > 0 {
		avgDuration = float64(totalDuration) / float64(resolved) / 1000.0 // Convert to seconds
	}

	am.mu.RLock()
	activeCount := len(am.activeAlerts)
	am.mu.RUnlock()

	return AlertStats{
		TotalTriggered: triggered,
		TotalResolved:  resolved,
		ActiveCount:    activeCount,
		AvgDuration:    avgDuration,
	}
}

// Stop gracefully stops the alert manager.
func (am *AlertManager) Stop() {
	close(am.stopChan)
	am.wg.Wait()
}

// Concrete Alert Rules

// HighErrorRateRule triggers when error rate exceeds threshold.
type HighErrorRateRule struct {
	id        string
	threshold float64
}

func NewHighErrorRateRule() *HighErrorRateRule {
	return &HighErrorRateRule{
		id:        "high_error_rate",
		threshold: 0.05, // 5% error rate
	}
}

func (r *HighErrorRateRule) ID() string {
	return r.id
}

func (r *HighErrorRateRule) Evaluate(stats AggregatedStats) *Alert {
	if stats.ErrorRate > r.threshold {
		return &Alert{
			ID:           r.id,
			Rule:         r.id,
			Type:         AlertHighErrorRate,
			Severity:     "critical",
			Metric:       "error_rate",
			CurrentValue: stats.ErrorRate,
			Threshold:    r.threshold,
			Message:      fmt.Sprintf("Error rate %.2f%% exceeds threshold %.2f%%", stats.ErrorRate*100, r.threshold*100),
		}
	}
	return nil
}

// LowHitRateRule triggers when cache hit rate drops below threshold.
type LowHitRateRule struct {
	id        string
	threshold float64
}

func NewLowHitRateRule() *LowHitRateRule {
	return &LowHitRateRule{
		id:        "low_hit_rate",
		threshold: 0.70, // 70% hit rate
	}
}

func (r *LowHitRateRule) ID() string {
	return r.id
}

func (r *LowHitRateRule) Evaluate(stats AggregatedStats) *Alert {
	if stats.TotalRequests > 100 && stats.HitRate < r.threshold {
		severity := "warning"
		if stats.HitRate < 0.50 {
			severity = "critical"
		}

		return &Alert{
			ID:           r.id,
			Rule:         r.id,
			Type:         AlertLowHitRate,
			Severity:     severity,
			Metric:       "hit_rate",
			CurrentValue: stats.HitRate,
			Threshold:    r.threshold,
			Message:      fmt.Sprintf("Cache hit rate %.2f%% below threshold %.2f%%", stats.HitRate*100, r.threshold*100),
		}
	}
	return nil
}

// LatencySpikeRule triggers when P95 latency exceeds threshold.
type LatencySpikeRule struct {
	id        string
	threshold float64
}

func NewLatencySpikeRule() *LatencySpikeRule {
	return &LatencySpikeRule{
		id:        "latency_spike",
		threshold: 100.0, // 100ms P95 latency
	}
}

func (r *LatencySpikeRule) ID() string {
	return r.id
}

func (r *LatencySpikeRule) Evaluate(stats AggregatedStats) *Alert {
	if stats.P95Latency > r.threshold {
		severity := "warning"
		if stats.P95Latency > r.threshold*2 {
			severity = "critical"
		}

		return &Alert{
			ID:           r.id,
			Rule:         r.id,
			Type:         AlertLatencySpike,
			Severity:     severity,
			Metric:       "p95_latency",
			CurrentValue: stats.P95Latency,
			Threshold:    r.threshold,
			Message:      fmt.Sprintf("P95 latency %.2fms exceeds threshold %.2fms", stats.P95Latency, r.threshold),
		}
	}
	return nil
}

// HighEvictionRateRule triggers when eviction rate is too high.
type HighEvictionRateRule struct {
	id        string
	threshold float64 // Evictions per second
}

func NewHighEvictionRateRule() *HighEvictionRateRule {
	return &HighEvictionRateRule{
		id:        "high_eviction_rate",
		threshold: 10.0, // 10 evictions/sec
	}
}

func (r *HighEvictionRateRule) ID() string {
	return r.id
}

func (r *HighEvictionRateRule) Evaluate(stats AggregatedStats) *Alert {
	evictionRate := float64(stats.Evictions) / 60.0 // Per second (1-minute window)

	if evictionRate > r.threshold {
		return &Alert{
			ID:           r.id,
			Rule:         r.id,
			Type:         AlertHighEvictionRate,
			Severity:     "warning",
			Metric:       "eviction_rate",
			CurrentValue: evictionRate,
			Threshold:    r.threshold,
			Message:      fmt.Sprintf("Eviction rate %.2f/sec exceeds threshold %.2f/sec - consider increasing cache size", evictionRate, r.threshold),
		}
	}
	return nil
}

// DynamicThresholdRule uses moving averages for adaptive thresholds.
//
// Design: Maintains baseline statistics and triggers alerts when values
// deviate significantly from historical patterns.
type DynamicThresholdRule struct {
	id             string
	metric         string
	alertType      AlertType
	baseline       *HistoricalStats
	deviationLimit float64 // Z-score threshold
}

func NewDynamicThresholdRule(id, metric string, alertType AlertType, deviationLimit float64) *DynamicThresholdRule {
	return &DynamicThresholdRule{
		id:             id,
		metric:         metric,
		alertType:      alertType,
		baseline:       NewHistoricalStats(100),
		deviationLimit: deviationLimit,
	}
}

func (r *DynamicThresholdRule) ID() string {
	return r.id
}

func (r *DynamicThresholdRule) Evaluate(stats AggregatedStats) *Alert {
	var currentValue float64

	switch r.metric {
	case "hit_rate":
		currentValue = stats.HitRate
	case "p95_latency":
		currentValue = stats.P95Latency
	case "error_rate":
		currentValue = stats.ErrorRate
	case "qps":
		currentValue = stats.QPS
	default:
		return nil
	}

	// Update baseline
	r.baseline.Add(currentValue)

	// Need enough samples for meaningful statistics
	if r.baseline.Count() < 20 {
		return nil
	}

	mean, stddev := r.baseline.MeanStdDev()
	if stddev == 0 {
		return nil
	}

	zscore := (currentValue - mean) / stddev

	// Check if deviation exceeds limit
	if math.Abs(zscore) > r.deviationLimit {
		severity := "warning"
		if math.Abs(zscore) > r.deviationLimit*1.5 {
			severity = "critical"
		}

		return &Alert{
			ID:           r.id,
			Rule:         r.id,
			Type:         r.alertType,
			Severity:     severity,
			Metric:       r.metric,
			CurrentValue: currentValue,
			Threshold:    mean,
			Message:      fmt.Sprintf("%s value %.2f deviates %.2f standard deviations from baseline %.2f", r.metric, currentValue, zscore, mean),
		}
	}

	return nil
}