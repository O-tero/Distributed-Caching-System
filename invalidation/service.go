// Package invalidation provides a distributed cache invalidation service that coordinates
// cache invalidation across multiple cache-manager instances.
//
// Design Philosophy:
// - Pub/Sub broadcast ensures eventual consistency across all cache nodes
// - Audit logging provides immutable invalidation history for compliance and debugging
// - Pattern matching supports flexible invalidation strategies (exact, prefix, wildcard)
// - Metrics enable observability of invalidation patterns and performance
//
// Performance Characteristics:
// - Key invalidation: O(k) where k = number of keys
// - Pattern invalidation: O(n) where n = total cache keys (with optimization via prefix trees)
// - Pub/Sub publish: O(1) + network latency
// - Audit insert: O(1) database write
//
// Consistency Model:
// - At-least-once delivery via Pub/Sub guarantees all nodes receive invalidation
// - Idempotent invalidation ensures correctness under duplicate events
// - Audit log provides single source of truth for invalidation history
package invalidation

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync/atomic"
	"time"

	"encore.dev/pubsub"
	"encore.dev/storage/sqldb"
)

//encore:service
type Service struct {
	patternMatcher *PatternMatcher
	auditLogger    AuditLoggerInterface
	metrics        *Metrics
}

// AuditLoggerInterface defines the interface for audit logging operations.
type AuditLoggerInterface interface {
	Insert(ctx context.Context, log AuditLog) error
	GetRecent(ctx context.Context, limit, offset int, patternFilter string) ([]AuditLog, error)
	GetCount(ctx context.Context, patternFilter string) (int, error)
	GetByRequestID(ctx context.Context, requestID string) ([]AuditLog, error)
}

// Metrics tracks invalidation performance counters.
type Metrics struct {
	TotalInvalidations   atomic.Int64
	KeyInvalidations     atomic.Int64
	PatternInvalidations atomic.Int64
	AuditWrites          atomic.Int64
	PubSubPublishes      atomic.Int64
	Errors               atomic.Int64
}

// Database for audit logging
var db = sqldb.Named("invalidation_db")

// Initialize service with dependencies
func initService() (*Service, error) {
	auditLogger, err := NewAuditLogger(db)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize audit logger: %w", err)
	}

	return &Service{
		patternMatcher: NewPatternMatcher(),
		auditLogger:    auditLogger,
		metrics:        &Metrics{},
	}, nil
}

// Global service instance
var svc *Service

func init() {
	var err error
	svc, err = initService()
	if err != nil {
		panic(fmt.Sprintf("failed to initialize invalidation service: %v", err))
	}
}

// InvalidationEvent represents a cache invalidation broadcast to all cache instances.
type InvalidationEvent struct {
	Pattern     string    `json:"pattern"`      // Pattern or exact key
	MatchedKeys []string  `json:"matched_keys"` // Keys that matched the pattern
	TriggeredBy string    `json:"triggered_by"` // Source: "cache_manager", "admin", "warming"
	Timestamp   time.Time `json:"timestamp"`    // When invalidation was triggered
	RequestID   string    `json:"request_id"`   // For tracing and correlation
}

// Pub/Sub topic for cache invalidation events
var CacheInvalidateTopic = pubsub.NewTopic[*InvalidationEvent](
	"cache-invalidate",
	pubsub.TopicConfig{
		DeliveryGuarantee: pubsub.AtLeastOnce,
	},
)

// Request and response types

type InvalidateKeyRequest struct {
	Keys        []string `json:"keys"`         // Exact keys to invalidate
	TriggeredBy string   `json:"triggered_by"` // Source identifier
	RequestID   string   `json:"request_id"`   // Optional correlation ID
}

type InvalidateKeyResponse struct {
	Success          bool      `json:"success"`
	InvalidatedCount int       `json:"invalidated_count"`
	Keys             []string  `json:"keys"`
	RequestID        string    `json:"request_id"`
	PublishedAt      time.Time `json:"published_at"`
}

type InvalidatePatternRequest struct {
	Pattern     string   `json:"pattern"`      // Wildcard pattern (e.g., "user:*", "product:123:*")
	TriggeredBy string   `json:"triggered_by"` // Source identifier
	RequestID   string   `json:"request_id"`   // Optional correlation ID
	CacheKeys   []string `json:"cache_keys"`   // Optional: provide current cache keys for matching
}

type InvalidatePatternResponse struct {
	Success          bool      `json:"success"`
	Pattern          string    `json:"pattern"`
	MatchedKeys      []string  `json:"matched_keys"`
	InvalidatedCount int       `json:"invalidated_count"`
	RequestID        string    `json:"request_id"`
	PublishedAt      time.Time `json:"published_at"`
}

type GetAuditLogsRequest struct {
	Limit   int    `json:"limit"`             // Number of logs to retrieve
	Offset  int    `json:"offset"`            // Pagination offset
	Pattern string `json:"pattern,omitempty"` // Optional: filter by pattern
}

type GetAuditLogsResponse struct {
	Logs       []AuditLog `json:"logs"`
	TotalCount int        `json:"total_count"`
	HasMore    bool       `json:"has_more"`
}

type MetricsResponse struct {
	TotalInvalidations       int64   `json:"total_invalidations"`
	KeyInvalidations         int64   `json:"key_invalidations"`
	PatternInvalidations     int64   `json:"pattern_invalidations"`
	AuditWrites              int64   `json:"audit_writes"`
	PubSubPublishes          int64   `json:"pubsub_publishes"`
	Errors                   int64   `json:"errors"`
	PatternInvalidationRatio float64 `json:"pattern_invalidation_ratio"`
}

// InvalidateKey invalidates specific cache keys and broadcasts the event.
// This is used for targeted invalidation when exact keys are known.
//
// Complexity: O(k) where k = number of keys
//
//encore:api public method=POST path=/invalidate/key
func InvalidateKey(ctx context.Context, req *InvalidateKeyRequest) (*InvalidateKeyResponse, error) {
	if svc == nil {
		return nil, errors.New("service not initialized")
	}
	return svc.InvalidateKey(ctx, req)
}

func (s *Service) InvalidateKey(ctx context.Context, req *InvalidateKeyRequest) (*InvalidateKeyResponse, error) {
	startTime := time.Now()

	// Validation
	if len(req.Keys) == 0 {
		return nil, errors.New("keys cannot be empty")
	}
	if req.TriggeredBy == "" {
		req.TriggeredBy = "unknown"
	}
	if req.RequestID == "" {
		req.RequestID = generateRequestID()
	}

	// Deduplicate keys
	uniqueKeys := deduplicateKeys(req.Keys)

	// Create invalidation event
	event := &InvalidationEvent{
		Pattern:     "", // Empty for exact key invalidation
		MatchedKeys: uniqueKeys,
		TriggeredBy: req.TriggeredBy,
		Timestamp:   time.Now(),
		RequestID:   req.RequestID,
	}

	// Publish to Pub/Sub (broadcast to all cache instances)
	_, err := CacheInvalidateTopic.Publish(ctx, event)
	if err != nil {
		s.metrics.Errors.Add(1)
		return nil, fmt.Errorf("failed to publish invalidation event: %w", err)
	}
	s.metrics.PubSubPublishes.Add(1)

	// Write audit log (async to not block response)
	go func() {
		auditLog := AuditLog{
			Pattern:     formatKeysAsPattern(uniqueKeys),
			Keys:        uniqueKeys,
			TriggeredBy: req.TriggeredBy,
			Timestamp:   event.Timestamp,
			RequestID:   req.RequestID,
			Latency:     time.Since(startTime).Milliseconds(),
		}
		if err := s.auditLogger.Insert(context.Background(), auditLog); err != nil {
			// Log error but don't fail the request
			s.metrics.Errors.Add(1)
		} else {
			s.metrics.AuditWrites.Add(1)
		}
	}()

	// Update metrics
	s.metrics.TotalInvalidations.Add(1)
	s.metrics.KeyInvalidations.Add(1)

	return &InvalidateKeyResponse{
		Success:          true,
		InvalidatedCount: len(uniqueKeys),
		Keys:             uniqueKeys,
		RequestID:        req.RequestID,
		PublishedAt:      event.Timestamp,
	}, nil
}

// InvalidatePattern invalidates cache keys matching a pattern and broadcasts the event.
// Supports wildcard patterns like "user:*", "product:123:*", etc.
//
// Complexity: O(n) where n = total cache keys (optimized with prefix trees)
//
//encore:api public method=POST path=/invalidate/pattern
func InvalidatePattern(ctx context.Context, req *InvalidatePatternRequest) (*InvalidatePatternResponse, error) {
	if svc == nil {
		return nil, errors.New("service not initialized")
	}
	return svc.InvalidatePattern(ctx, req)
}

func (s *Service) InvalidatePattern(ctx context.Context, req *InvalidatePatternRequest) (*InvalidatePatternResponse, error) {
	startTime := time.Now()

	// Validation
	if req.Pattern == "" {
		return nil, errors.New("pattern cannot be empty")
	}
	if req.TriggeredBy == "" {
		req.TriggeredBy = "unknown"
	}
	if req.RequestID == "" {
		req.RequestID = generateRequestID()
	}

	// Match keys against pattern
	var matchedKeys []string
	if len(req.CacheKeys) > 0 {
		// If cache keys provided, match locally
		matchedKeys = s.patternMatcher.Match(req.Pattern, req.CacheKeys)
	} else {
		// Otherwise, broadcast pattern and let each cache instance match locally
		matchedKeys = []string{} // Empty means pattern-based, each node matches
	}

	// Create invalidation event
	event := &InvalidationEvent{
		Pattern:     req.Pattern,
		MatchedKeys: matchedKeys,
		TriggeredBy: req.TriggeredBy,
		Timestamp:   time.Now(),
		RequestID:   req.RequestID,
	}

	// Publish to Pub/Sub
	_, err := CacheInvalidateTopic.Publish(ctx, event)
	if err != nil {
		s.metrics.Errors.Add(1)
		return nil, fmt.Errorf("failed to publish invalidation event: %w", err)
	}
	s.metrics.PubSubPublishes.Add(1)

	// Write audit log (async)
	go func() {
		auditLog := AuditLog{
			Pattern:     req.Pattern,
			Keys:        matchedKeys,
			TriggeredBy: req.TriggeredBy,
			Timestamp:   event.Timestamp,
			RequestID:   req.RequestID,
			Latency:     time.Since(startTime).Milliseconds(),
		}
		if err := s.auditLogger.Insert(context.Background(), auditLog); err != nil {
			s.metrics.Errors.Add(1)
		} else {
			s.metrics.AuditWrites.Add(1)
		}
	}()

	// Update metrics
	s.metrics.TotalInvalidations.Add(1)
	s.metrics.PatternInvalidations.Add(1)

	return &InvalidatePatternResponse{
		Success:          true,
		Pattern:          req.Pattern,
		MatchedKeys:      matchedKeys,
		InvalidatedCount: len(matchedKeys),
		RequestID:        req.RequestID,
		PublishedAt:      event.Timestamp,
	}, nil
}

// GetAuditLogs retrieves invalidation audit history with pagination.
//
//encore:api public method=GET path=/audit/logs
func GetAuditLogs(ctx context.Context, req *GetAuditLogsRequest) (*GetAuditLogsResponse, error) {
	if svc == nil {
		return nil, errors.New("service not initialized")
	}
	return svc.GetAuditLogs(ctx, req)
}

func (s *Service) GetAuditLogs(ctx context.Context, req *GetAuditLogsRequest) (*GetAuditLogsResponse, error) {
	// Default pagination
	if req.Limit <= 0 {
		req.Limit = 50
	}
	if req.Limit > 1000 {
		req.Limit = 1000 // Max page size
	}
	if req.Offset < 0 {
		req.Offset = 0
	}

	// Fetch logs
	logs, err := s.auditLogger.GetRecent(ctx, req.Limit+1, req.Offset, req.Pattern)
	if err != nil {
		s.metrics.Errors.Add(1)
		return nil, fmt.Errorf("failed to fetch audit logs: %w", err)
	}

	// Check if there are more results
	hasMore := len(logs) > req.Limit
	if hasMore {
		logs = logs[:req.Limit]
	}

	// Get total count (for pagination info)
	totalCount, err := s.auditLogger.GetCount(ctx, req.Pattern)
	if err != nil {
		totalCount = len(logs) // Fallback
	}

	return &GetAuditLogsResponse{
		Logs:       logs,
		TotalCount: totalCount,
		HasMore:    hasMore,
	}, nil
}

// GetMetrics returns invalidation service metrics.
//
//encore:api public method=GET path=/invalidate/metrics
func GetMetrics(ctx context.Context) (*MetricsResponse, error) {
	if svc == nil {
		return nil, errors.New("service not initialized")
	}
	return svc.GetMetrics(ctx)
}

func (s *Service) GetMetrics(ctx context.Context) (*MetricsResponse, error) {
	total := s.metrics.TotalInvalidations.Load()
	pattern := s.metrics.PatternInvalidations.Load()

	patternRatio := 0.0
	if total > 0 {
		patternRatio = float64(pattern) / float64(total)
	}

	return &MetricsResponse{
		TotalInvalidations:       total,
		KeyInvalidations:         s.metrics.KeyInvalidations.Load(),
		PatternInvalidations:     pattern,
		AuditWrites:              s.metrics.AuditWrites.Load(),
		PubSubPublishes:          s.metrics.PubSubPublishes.Load(),
		Errors:                   s.metrics.Errors.Load(),
		PatternInvalidationRatio: patternRatio,
	}, nil
}

// Helper functions

// deduplicateKeys removes duplicate keys while preserving order.
func deduplicateKeys(keys []string) []string {
	seen := make(map[string]bool, len(keys))
	result := make([]string, 0, len(keys))

	for _, key := range keys {
		if !seen[key] {
			seen[key] = true
			result = append(result, key)
		}
	}

	return result
}

// formatKeysAsPattern converts multiple keys into a pattern representation.
func formatKeysAsPattern(keys []string) string {
	if len(keys) == 0 {
		return ""
	}
	if len(keys) == 1 {
		return keys[0]
	}

	// For multiple keys, create a compact representation
	data, _ := json.Marshal(keys)
	return string(data)
}

// generateRequestID creates a unique request identifier for tracing.
func generateRequestID() string {
	return fmt.Sprintf("inv-%d-%d", time.Now().UnixNano(), time.Now().Nanosecond()%1000)
}
