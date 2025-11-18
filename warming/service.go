// Package warming provides proactive cache warming to prevent cold misses and cache stampedes.
//
// Design Philosophy:
// - Prevent thundering herd by warming cache before expiration or predicted access spikes
// - Multiple warming strategies for different use cases (scheduled, predictive, priority-based)
// - Rate limiting and backpressure to protect origin services
// - Worker pool for concurrent warming with deduplication
// - Observable via metrics and structured logging
//
// Performance Characteristics:
// - Worker pool processes N tasks concurrently (configurable CONCURRENT_WARMERS)
// - Rate limiter ensures origin protection (configurable MAX_ORIGIN_RPS)
// - Deduplication prevents redundant warming of same key
// - Batch warming reduces overhead for related keys
//
// Trade-offs:
// - In-memory job queue for simplicity (TODO: persistent queue for durability)
// - Simple predictor (TODO: ML-based predictor for better accuracy)
// - Synchronous origin fetch (TODO: async batching for higher throughput)
package warming

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"encore.dev/pubsub"
	"golang.org/x/sync/singleflight"
	"golang.org/x/time/rate"
)

//encore:service
type Service struct {
	config         Config
	strategies     map[string]Strategy
	predictor      Predictor
	originFetcher  OriginFetcher
	cacheClient    CacheClient
	scheduler      *Scheduler
	workerPool     *WorkerPool
	metrics        *Metrics
	rateLimiter    *rate.Limiter
	deduper        singleflight.Group
	emergencyStop  atomic.Bool
	mu             sync.RWMutex
}

// Config holds runtime configuration for the warming service.
type Config struct {
	MaxOriginRPS      int           `json:"max_origin_rps"`       // Max requests per second to origin
	MaxBatchSize      int           `json:"max_batch_size"`       // Max keys per warming batch
	ConcurrentWarmers int           `json:"concurrent_warmers"`   // Number of concurrent worker goroutines
	DefaultTTL        time.Duration `json:"default_ttl"`          // Default cache TTL
	OriginTimeout     time.Duration `json:"origin_timeout"`       // Timeout for origin requests
	RetryAttempts     int           `json:"retry_attempts"`       // Number of retry attempts on failure
	BackoffBase       time.Duration `json:"backoff_base"`         // Base duration for exponential backoff
	EmergencyThreshold time.Duration `json:"emergency_threshold"` // Origin latency threshold for emergency stop
	DefaultStrategy   string        `json:"default_strategy"`     // Default warming strategy
}

// DefaultConfig returns sensible default configuration.
func DefaultConfig() Config {
	return Config{
		MaxOriginRPS:       100,
		MaxBatchSize:       50,
		ConcurrentWarmers:  10,
		DefaultTTL:         1 * time.Hour,
		OriginTimeout:      5 * time.Second,
		RetryAttempts:      3,
		BackoffBase:        100 * time.Millisecond,
		EmergencyThreshold: 2 * time.Second,
		DefaultStrategy:    "priority",
	}
}

// Metrics tracks warming service performance.
type Metrics struct {
	JobsTotal       atomic.Int64
	SuccessTotal    atomic.Int64
	FailureTotal    atomic.Int64
	OriginRequests  atomic.Int64
	CacheWrites     atomic.Int64
	RateLimitHits   atomic.Int64
	EmergencyStops  atomic.Int64
	TotalDuration   atomic.Int64 // Cumulative milliseconds
}

// OriginFetcher abstracts the data source for cache warming.
type OriginFetcher interface {
	Fetch(ctx context.Context, key string) (value []byte, ttl time.Duration, err error)
}

// CacheClient abstracts the cache-manager API for warming.
type CacheClient interface {
	Set(ctx context.Context, key string, value []byte, ttl time.Duration) error
}

// Request and response types

type WarmKeyRequest struct {
	Keys     []string `json:"keys"`               // Keys to warm
	Priority int      `json:"priority,omitempty"` // Priority level (0-100)
	Strategy string   `json:"strategy,omitempty"` // Optional strategy override
}

type WarmKeyResponse struct {
	Success      bool     `json:"success"`
	Queued       int      `json:"queued"`        // Number of tasks queued
	Keys         []string `json:"keys"`
	JobID        string   `json:"job_id"`
	EstimatedTime int     `json:"estimated_time_ms"`
}

type WarmPatternRequest struct {
	Pattern  string   `json:"pattern"`            // Pattern to match (e.g., "user:*")
	Limit    int      `json:"limit,omitempty"`    // Max keys to warm
	Priority int      `json:"priority,omitempty"` // Priority level
	Strategy string   `json:"strategy,omitempty"` // Optional strategy override
	Keys     []string `json:"keys,omitempty"`     // Optional: explicit keys matching pattern
}

type WarmPatternResponse struct {
	Success       bool     `json:"success"`
	Pattern       string   `json:"pattern"`
	Queued        int      `json:"queued"`
	MatchedKeys   []string `json:"matched_keys,omitempty"`
	JobID         string   `json:"job_id"`
	EstimatedTime int      `json:"estimated_time_ms"`
}

type StatusResponse struct {
	ActiveJobs    int            `json:"active_jobs"`
	QueuedTasks   int            `json:"queued_tasks"`
	WorkerStatus  []WorkerStatus `json:"worker_status"`
	EmergencyStop bool           `json:"emergency_stop"`
	Metrics       MetricsSnapshot `json:"metrics"`
}

type WorkerStatus struct {
	ID          int    `json:"id"`
	State       string `json:"state"` // "idle", "busy", "stopped"
	CurrentKey  string `json:"current_key,omitempty"`
	StartedAt   *time.Time `json:"started_at,omitempty"`
}

type MetricsSnapshot struct {
	JobsTotal      int64   `json:"jobs_total"`
	SuccessTotal   int64   `json:"success_total"`
	FailureTotal   int64   `json:"failure_total"`
	SuccessRate    float64 `json:"success_rate"`
	OriginRequests int64   `json:"origin_requests"`
	CacheWrites    int64   `json:"cache_writes"`
	RateLimitHits  int64   `json:"rate_limit_hits"`
	EmergencyStops int64   `json:"emergency_stops"`
	AvgDurationMs  float64 `json:"avg_duration_ms"`
}

type ConfigResponse struct {
	Config Config `json:"config"`
}

type UpdateConfigRequest struct {
	MaxOriginRPS      *int   `json:"max_origin_rps,omitempty"`
	MaxBatchSize      *int   `json:"max_batch_size,omitempty"`
	ConcurrentWarmers *int   `json:"concurrent_warmers,omitempty"`
	DefaultStrategy   string `json:"default_strategy,omitempty"`
}

// Global service instance
var svc *Service

// initService initializes the warming service with default configuration.
func initService() (*Service, error) {
	config := DefaultConfig()

	// Initialize strategies
	strategies := map[string]Strategy{
		"selective": NewSelectiveHotKeysStrategy(),
		"breadth":   NewBreadthFirstStrategy(),
		"priority":  NewPriorityBasedStrategy(),
	}

	// Initialize predictor
	predictor := NewDefaultPredictor()

	// Create service
	s := &Service{
		config:     config,
		strategies: strategies,
		predictor:  predictor,
		metrics:    &Metrics{},
		rateLimiter: rate.NewLimiter(rate.Limit(config.MaxOriginRPS), config.MaxOriginRPS),
	}

	// Initialize worker pool
	s.workerPool = NewWorkerPool(s, config.ConcurrentWarmers)

	// Initialize scheduler
	s.scheduler = NewScheduler(s)

	return s, nil
}

func init() {
	var err error
	svc, err = initService()
	if err != nil {
		panic(fmt.Sprintf("failed to initialize warming service: %v", err))
	}
}

// SetOriginFetcher allows injecting custom origin fetcher (for production or testing).
func (s *Service) SetOriginFetcher(fetcher OriginFetcher) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.originFetcher = fetcher
}

// SetCacheClient allows injecting custom cache client (for production or testing).
func (s *Service) SetCacheClient(client CacheClient) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.cacheClient = client
}

// WarmKey warms specific cache keys immediately.
//encore:api public method=POST path=/warm/key
func WarmKey(ctx context.Context, req *WarmKeyRequest) (*WarmKeyResponse, error) {
	if svc == nil {
		return nil, errors.New("service not initialized")
	}
	return svc.WarmKey(ctx, req)
}

func (s *Service) WarmKey(ctx context.Context, req *WarmKeyRequest) (*WarmKeyResponse, error) {
	if len(req.Keys) == 0 {
		return nil, errors.New("keys cannot be empty")
	}

	if s.emergencyStop.Load() {
		return nil, errors.New("warming service in emergency stop mode")
	}

	// Default priority
	priority := req.Priority
	if priority == 0 {
		priority = 50 // Medium priority
	}

	// Create warm tasks
	tasks := make([]WarmTask, 0, len(req.Keys))
	for _, key := range req.Keys {
		tasks = append(tasks, WarmTask{
			Key:      key,
			Priority: priority,
			EstimatedCost: 50, // Default estimate
			TTL:      s.config.DefaultTTL,
			Strategy: req.Strategy,
		})
	}

	// Queue tasks
	jobID := generateJobID()
	queued := s.workerPool.QueueTasks(tasks)

	s.metrics.JobsTotal.Add(int64(queued))

	// Estimate completion time
	estimatedTime := (queued * 50) / s.config.ConcurrentWarmers // rough estimate

	return &WarmKeyResponse{
		Success:       true,
		Queued:        queued,
		Keys:          req.Keys,
		JobID:         jobID,
		EstimatedTime: estimatedTime,
	}, nil
}

// WarmPattern warms cache keys matching a pattern.
//encore:api public method=POST path=/warm/pattern
func WarmPattern(ctx context.Context, req *WarmPatternRequest) (*WarmPatternResponse, error) {
	if svc == nil {
		return nil, errors.New("service not initialized")
	}
	return svc.WarmPattern(ctx, req)
}

func (s *Service) WarmPattern(ctx context.Context, req *WarmPatternRequest) (*WarmPatternResponse, error) {
	if req.Pattern == "" {
		return nil, errors.New("pattern cannot be empty")
	}

	if s.emergencyStop.Load() {
		return nil, errors.New("warming service in emergency stop mode")
	}

	// Use provided keys or predict based on pattern
	var keysToWarm []string
	if len(req.Keys) > 0 {
		keysToWarm = req.Keys
	} else {
		// Use predictor to find keys matching pattern
		predicted, err := s.predictor.PredictHotKeys(ctx, 1*time.Hour, 100)
		if err != nil {
			return nil, fmt.Errorf("prediction failed: %w", err)
		}
		keysToWarm = filterByPattern(predicted, req.Pattern)
	}

	// Apply limit
	if req.Limit > 0 && len(keysToWarm) > req.Limit {
		keysToWarm = keysToWarm[:req.Limit]
	}

	// Select strategy
	strategyName := req.Strategy
	if strategyName == "" {
		strategyName = s.config.DefaultStrategy
	}

	strategy, exists := s.strategies[strategyName]
	if !exists {
		return nil, fmt.Errorf("unknown strategy: %s", strategyName)
	}

	// Plan warming tasks
	planOpts := PlanOptions{
		Keys:     keysToWarm,
		Priority: req.Priority,
		Limit:    req.Limit,
	}

	tasks, err := strategy.Plan(ctx, planOpts)
	if err != nil {
		return nil, fmt.Errorf("strategy planning failed: %w", err)
	}

	// Queue tasks
	jobID := generateJobID()
	queued := s.workerPool.QueueTasks(tasks)

	s.metrics.JobsTotal.Add(int64(queued))

	estimatedTime := (queued * 50) / s.config.ConcurrentWarmers

	return &WarmPatternResponse{
		Success:       true,
		Pattern:       req.Pattern,
		Queued:        queued,
		MatchedKeys:   keysToWarm,
		JobID:         jobID,
		EstimatedTime: estimatedTime,
	}, nil
}

// GetStatus returns current warming service status and metrics.
//encore:api public method=GET path=/warm/status
func GetStatus(ctx context.Context) (*StatusResponse, error) {
	if svc == nil {
		return nil, errors.New("service not initialized")
	}
	return svc.GetStatus(ctx)
}

func (s *Service) GetStatus(ctx context.Context) (*StatusResponse, error) {
	workerStatus := s.workerPool.GetWorkerStatus()

	jobs := s.metrics.JobsTotal.Load()
	success := s.metrics.SuccessTotal.Load()
	successRate := 0.0
	if jobs > 0 {
		successRate = float64(success) / float64(jobs)
	}

	avgDuration := 0.0
	if success > 0 {
		avgDuration = float64(s.metrics.TotalDuration.Load()) / float64(success)
	}

	return &StatusResponse{
		ActiveJobs:    s.workerPool.ActiveCount(),
		QueuedTasks:   s.workerPool.QueueSize(),
		WorkerStatus:  workerStatus,
		EmergencyStop: s.emergencyStop.Load(),
		Metrics: MetricsSnapshot{
			JobsTotal:      jobs,
			SuccessTotal:   success,
			FailureTotal:   s.metrics.FailureTotal.Load(),
			SuccessRate:    successRate,
			OriginRequests: s.metrics.OriginRequests.Load(),
			CacheWrites:    s.metrics.CacheWrites.Load(),
			RateLimitHits:  s.metrics.RateLimitHits.Load(),
			EmergencyStops: s.metrics.EmergencyStops.Load(),
			AvgDurationMs:  avgDuration,
		},
	}, nil
}

// TriggerPredictive manually triggers a predictive warming run.
//encore:api public method=POST path=/warm/trigger-predictive
func TriggerPredictive(ctx context.Context) (*WarmKeyResponse, error) {
	if svc == nil {
		return nil, errors.New("service not initialized")
	}
	return svc.TriggerPredictive(ctx)
}

func (s *Service) TriggerPredictive(ctx context.Context) (*WarmKeyResponse, error) {
	if s.emergencyStop.Load() {
		return nil, errors.New("warming service in emergency stop mode")
	}

	// Predict hot keys for next hour
	hotKeys, err := s.predictor.PredictHotKeys(ctx, 1*time.Hour, 100)
	if err != nil {
		return nil, fmt.Errorf("prediction failed: %w", err)
	}

	if len(hotKeys) == 0 {
		return &WarmKeyResponse{
			Success: true,
			Queued:  0,
			Keys:    []string{},
		}, nil
	}

	// Use priority strategy for predictive warming
	strategy := s.strategies["priority"]
	tasks, err := strategy.Plan(ctx, PlanOptions{
		Keys:     hotKeys,
		Priority: 80, // High priority for predicted keys
	})
	if err != nil {
		return nil, fmt.Errorf("strategy planning failed: %w", err)
	}

	jobID := generateJobID()
	queued := s.workerPool.QueueTasks(tasks)

	s.metrics.JobsTotal.Add(int64(queued))

	return &WarmKeyResponse{
		Success:       true,
		Queued:        queued,
		Keys:          hotKeys,
		JobID:         jobID,
		EstimatedTime: (queued * 50) / s.config.ConcurrentWarmers,
	}, nil
}

// GetConfig returns current service configuration.
//encore:api public method=GET path=/warm/config
func GetConfig(ctx context.Context) (*ConfigResponse, error) {
	if svc == nil {
		return nil, errors.New("service not initialized")
	}
	return svc.GetConfig(ctx)
}

func (s *Service) GetConfig(ctx context.Context) (*ConfigResponse, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return &ConfigResponse{
		Config: s.config,
	}, nil
}

// UpdateConfig updates service configuration at runtime.
//encore:api public method=POST path=/warm/config
func UpdateConfig(ctx context.Context, req *UpdateConfigRequest) (*ConfigResponse, error) {
	if svc == nil {
		return nil, errors.New("service not initialized")
	}
	return svc.UpdateConfig(ctx, req)
}

func (s *Service) UpdateConfig(ctx context.Context, req *UpdateConfigRequest) (*ConfigResponse, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Update configuration
	if req.MaxOriginRPS != nil {
		s.config.MaxOriginRPS = *req.MaxOriginRPS
		s.rateLimiter = rate.NewLimiter(rate.Limit(*req.MaxOriginRPS), *req.MaxOriginRPS)
	}

	if req.MaxBatchSize != nil {
		s.config.MaxBatchSize = *req.MaxBatchSize
	}

	if req.ConcurrentWarmers != nil {
		s.config.ConcurrentWarmers = *req.ConcurrentWarmers
		// Note: changing concurrent warmers requires worker pool restart
		// For simplicity, this is not implemented here (TODO: dynamic worker pool sizing)
	}

	if req.DefaultStrategy != "" {
		if _, exists := s.strategies[req.DefaultStrategy]; !exists {
			return nil, fmt.Errorf("unknown strategy: %s", req.DefaultStrategy)
		}
		s.config.DefaultStrategy = req.DefaultStrategy
	}

	return &ConfigResponse{
		Config: s.config,
	}, nil
}

// Helper functions

// filterByPattern filters keys that match the given pattern.
func filterByPattern(keys []string, pattern string) []string {
	// Simple prefix matching for now
	// TODO: integrate with invalidation service's pattern matcher
	if pattern == "*" {
		return keys
	}

	// Remove trailing * for prefix match
	prefix := pattern
	if len(pattern) > 0 && pattern[len(pattern)-1] == '*' {
		prefix = pattern[:len(pattern)-1]
	}

	filtered := make([]string, 0)
	for _, key := range keys {
		if len(key) >= len(prefix) && key[:len(prefix)] == prefix {
			filtered = append(filtered, key)
		}
	}

	return filtered
}

// generateJobID creates a unique job identifier.
func generateJobID() string {
	return fmt.Sprintf("warm-%d-%d", time.Now().UnixNano(), time.Now().Nanosecond()%1000)
}

// ExecuteWarmTask performs the actual warming operation for a single task.
// This is called by workers and includes deduplication, rate limiting, and error handling.
func (s *Service) ExecuteWarmTask(ctx context.Context, task WarmTask) error {
	startTime := time.Now()

	// Check emergency stop
	if s.emergencyStop.Load() {
		return errors.New("emergency stop active")
	}

	// Deduplicate concurrent warming of same key
	_, err, _ := s.deduper.Do(task.Key, func() (interface{}, error) {
		return nil, s.executeWarmTaskInternal(ctx, task)
	})

	duration := time.Since(startTime)
	s.metrics.TotalDuration.Add(duration.Milliseconds())

	if err != nil {
		s.metrics.FailureTotal.Add(1)
		return err
	}

	s.metrics.SuccessTotal.Add(1)

	// Publish completion event
	go s.publishWarmCompletion(task.Key, "success", duration, task.Strategy)

	return nil
}

// executeWarmTaskInternal performs the actual warming logic.
func (s *Service) executeWarmTaskInternal(ctx context.Context, task WarmTask) error {
	// Wait for rate limiter
	if err := s.rateLimiter.Wait(ctx); err != nil {
		s.metrics.RateLimitHits.Add(1)
		return fmt.Errorf("rate limit: %w", err)
	}

	// Fetch from origin with timeout
	fetchCtx, cancel := context.WithTimeout(ctx, s.config.OriginTimeout)
	defer cancel()

	s.mu.RLock()
	fetcher := s.originFetcher
	cacheClient := s.cacheClient
	s.mu.RUnlock()

	if fetcher == nil {
		return errors.New("origin fetcher not configured")
	}

	value, ttl, err := fetcher.Fetch(fetchCtx, task.Key)
	if err != nil {
		return fmt.Errorf("origin fetch failed: %w", err)
	}

	s.metrics.OriginRequests.Add(1)

	// Check for high latency (emergency throttle trigger)
	fetchDuration := time.Since(time.Now().Add(-s.config.OriginTimeout))
	if fetchDuration > s.config.EmergencyThreshold {
		s.emergencyStop.Store(true)
		s.metrics.EmergencyStops.Add(1)
		return errors.New("emergency stop triggered due to high origin latency")
	}

	// Use task TTL if origin doesn't specify
	if ttl == 0 {
		ttl = task.TTL
	}

	// Write to cache
	if cacheClient != nil {
		if err := cacheClient.Set(ctx, task.Key, value, ttl); err != nil {
			return fmt.Errorf("cache write failed: %w", err)
		}
		s.metrics.CacheWrites.Add(1)
	}

	return nil
}

// publishWarmCompletion publishes a warming completion event to Pub/Sub.
func (s *Service) publishWarmCompletion(key string, status string, duration time.Duration, strategy string) {
	event := &WarmCompletedEvent{
		Key:        key,
		Status:     status,
		DurationMs: duration.Milliseconds(),
		Strategy:   strategy,
		Timestamp:  time.Now(),
	}

	_, _ = WarmCompletedTopic.Publish(context.Background(), event)
}

// WarmCompletedEvent represents a cache warming completion event.
type WarmCompletedEvent struct {
	Key        string    `json:"key"`
	Status     string    `json:"status"` // "success", "failure"
	DurationMs int64     `json:"duration_ms"`
	Strategy   string    `json:"strategy"`
	Timestamp  time.Time `json:"timestamp"`
}

// Pub/Sub topics
var WarmCompletedTopic = pubsub.NewTopic[*WarmCompletedEvent](
	"cache-warm-completed",
	pubsub.TopicConfig{
		DeliveryGuarantee: pubsub.AtLeastOnce,
	},
)

// Shutdown gracefully stops the warming service.
func (s *Service) Shutdown() {
	s.workerPool.Shutdown()
	s.scheduler.Stop()
}