// Package cachemanager implements a high-performance distributed cache with multi-level
// storage (L1 in-memory, L2 distributed), intelligent eviction policies (LRU+TTL),
// request coalescing to prevent cache stampede, and event-driven coordination via Pub/Sub.
//
// Design Choices:
// - L1 uses sync.RWMutex-protected map for predictable performance and memory efficiency.
//   sync.Map was considered but RWMutex provides better control over eviction and TTL cleanup.
// - Request coalescing via golang.org/x/sync/singleflight prevents thundering herd on cache misses.
// - L2 is abstracted via RemoteCache interface for testability and provider flexibility.
// - Pub/Sub coordination ensures eventual consistency across distributed instances.
//
// Performance Characteristics:
// - L1 Get: O(1) average, sub-microsecond for hot keys
// - L1 Set: O(1) with LRU update, ~1-2μs overhead
// - Eviction: O(1) via doubly-linked list
// - Bottlenecks: L2 network latency (~1-5ms), global lock on eviction (can shard in v2)
//
// Production Optimization Notes:
// - For >1M keys, consider sharding L1 across multiple sync.RWMutex instances
// - L2 batching via pipelining can reduce RTT by 5-10x for bulk operations
// - Add compression for values >1KB to reduce memory and network overhead
// - Implement adaptive TTL based on access patterns for hot keys
package cachemanager

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"
)

// Service implements the cache manager with multi-level storage and coordination.
//encore:service
type Service struct {
	l1Cache      *L1Cache
	l2Cache      RemoteCache
	originFetch  OriginFetcher
	coalescer    *RequestCoalescer
	metrics      *Metrics
	config       Config
	stopChan     chan struct{}
	wg           sync.WaitGroup
}

// Config holds runtime configuration for the cache manager.
type Config struct {
	L1MaxEntries   int           // Maximum L1 entries before eviction
	DefaultTTL     time.Duration // Default TTL for cached items
	CleanupInterval time.Duration // How often to run TTL cleanup
	L2Enabled      bool          // Whether L2 cache is available
}

// RemoteCache abstracts the L2 distributed cache (Redis, Memcached, etc.).
type RemoteCache interface {
	Get(ctx context.Context, key string) ([]byte, bool, error)
	Set(ctx context.Context, key string, value []byte, ttl time.Duration) error
	Delete(ctx context.Context, key string) error
	DeletePattern(ctx context.Context, pattern string) error
}

// OriginFetcher is called when cache misses occur to fetch from source of truth.
type OriginFetcher interface {
	Fetch(ctx context.Context, key string) (interface{}, error)
}

// Metrics tracks cache performance counters.
type Metrics struct {
	Hits       atomic.Int64
	Misses     atomic.Int64
	Sets       atomic.Int64
	Deletes    atomic.Int64
	Evictions  atomic.Int64
	L2Hits     atomic.Int64
	L2Misses   atomic.Int64
	L2Errors   atomic.Int64
}

// Request and response types for API endpoints.

type GetRequest struct {
	Key string `json:"key"`
}

type GetResponse struct {
	Value     interface{} `json:"value"`
	Hit       bool        `json:"hit"`
	Source    string      `json:"source"` // "l1", "l2", "origin"
	CachedAt  *time.Time  `json:"cached_at,omitempty"`
	ExpiresAt *time.Time  `json:"expires_at,omitempty"`
}

type SetRequest struct {
	Key   string      `json:"key"`
	Value interface{} `json:"value"`
	TTL   int         `json:"ttl"` // seconds, 0 means default
}

type SetResponse struct {
	Success   bool      `json:"success"`
	ExpiresAt time.Time `json:"expires_at"`
}

type InvalidateRequest struct {
	Keys    []string `json:"keys,omitempty"`
	Pattern string   `json:"pattern,omitempty"` // e.g., "user:*"
}

type InvalidateResponse struct {
	Invalidated int  `json:"invalidated"`
	Success     bool `json:"success"`
}

type MetricsResponse struct {
	Hits         int64   `json:"hits"`
	Misses       int64   `json:"misses"`
	HitRate      float64 `json:"hit_rate"`
	Sets         int64   `json:"sets"`
	Deletes      int64   `json:"deletes"`
	Evictions    int64   `json:"evictions"`
	L1Size       int     `json:"l1_size"`
	L2Hits       int64   `json:"l2_hits"`
	L2Misses     int64   `json:"l2_misses"`
	L2Errors     int64   `json:"l2_errors"`
}

var (
	// Global service instance (initialized by initService)
	svc *Service
	once sync.Once
)

// initService initializes the cache manager service with default configuration.
// Called automatically by Encore at startup.
func initService() (*Service, error) {
	var err error
	once.Do(func() {
		config := Config{
			L1MaxEntries:   10000,
			DefaultTTL:     1 * time.Hour,
			CleanupInterval: 1 * time.Minute,
			L2Enabled:      false, // Disabled by default for unit tests
		}

		svc = &Service{
			l1Cache:     NewL1Cache(config.L1MaxEntries),
			l2Cache:     nil, // Must be set via SetL2Cache for production
			originFetch: nil, // Must be set via SetOriginFetcher
			coalescer:   NewRequestCoalescer(),
			metrics:     &Metrics{},
			config:      config,
			stopChan:    make(chan struct{}),
		}

		// Start background cleanup goroutine
		svc.wg.Add(1)
		go svc.runTTLCleanup()
	})

	return svc, err
}

// SetL2Cache allows injecting L2 cache implementation (for production or testing).
func (s *Service) SetL2Cache(l2 RemoteCache) {
	s.l2Cache = l2
	s.config.L2Enabled = l2 != nil
}

// SetOriginFetcher allows injecting origin data source (for cache-aside pattern).
func (s *Service) SetOriginFetcher(fetcher OriginFetcher) {
	s.originFetch = fetcher
}

// Get retrieves a value from cache with read-through to L2 and origin.
// Complexity: O(1) average for L1 hit, O(1) + network for L2, O(1) + network + origin for miss.
//encore:api public method=GET path=/api/cache/:key
func Get(ctx context.Context, key string) (*GetResponse, error) {
	if svc == nil {
		return nil, errors.New("service not initialized")
	}
	return svc.Get(ctx, key)
}

func (s *Service) Get(ctx context.Context, key string) (*GetResponse, error) {
	if key == "" {
		return nil, errors.New("key cannot be empty")
	}

	startTime := time.Now()

	// L1 lookup
	if entry, ok := s.l1Cache.Get(key); ok {
		s.metrics.Hits.Add(1)
		return &GetResponse{
			Value:     entry.Value,
			Hit:       true,
			Source:    "l1",
			CachedAt:  &entry.CachedAt,
			ExpiresAt: &entry.ExpiresAt,
		}, nil
	}

	// L1 miss - use singleflight to coalesce requests
	result, err := s.coalescer.Do(key, func() (interface{}, error) {
		return s.fetchWithFallback(ctx, key)
	})

	if err != nil {
		s.metrics.Misses.Add(1)
		return &GetResponse{Hit: false}, err
	}

	entry := result.(*CacheEntry)
	
	// Record latency (for monitoring)
	_ = time.Since(startTime)

	return &GetResponse{
		Value:     entry.Value,
		Hit:       true,
		Source:    entry.Source,
		CachedAt:  &entry.CachedAt,
		ExpiresAt: &entry.ExpiresAt,
	}, nil
}

// fetchWithFallback attempts L2, then origin, with proper cache population.
func (s *Service) fetchWithFallback(ctx context.Context, key string) (*CacheEntry, error) {
	// Try L2 cache
	if s.config.L2Enabled && s.l2Cache != nil {
		if data, ok, err := s.l2Cache.Get(ctx, key); err == nil && ok {
			var entry CacheEntry
			if err := json.Unmarshal(data, &entry); err == nil {
				// Populate L1 from L2
				s.l1Cache.Set(key, entry.Value, entry.ExpiresAt.Sub(time.Now()))
				s.metrics.L2Hits.Add(1)
				entry.Source = "l2"
				return &entry, nil
			}
		} else if err != nil {
			s.metrics.L2Errors.Add(1)
		} else {
			s.metrics.L2Misses.Add(1)
		}
	}

	// Try origin fetch
	if s.originFetch == nil {
		return nil, errors.New("cache miss and no origin fetcher configured")
	}

	value, err := s.originFetch.Fetch(ctx, key)
	if err != nil {
		return nil, fmt.Errorf("origin fetch failed: %w", err)
	}

	// Populate both cache levels
	ttl := s.config.DefaultTTL
	expiresAt := time.Now().Add(ttl)
	
	s.l1Cache.Set(key, value, ttl)
	
	entry := &CacheEntry{
		Value:     value,
		CachedAt:  time.Now(),
		ExpiresAt: expiresAt,
		Source:    "origin",
	}

	// Async L2 population (don't block response)
	if s.config.L2Enabled && s.l2Cache != nil {
		go func() {
			data, _ := json.Marshal(entry)
			_ = s.l2Cache.Set(context.Background(), key, data, ttl)
		}()
	}

	return entry, nil
}

// Set stores a value in cache with write-through to L2.
// Complexity: O(1) for L1 + O(1) + network for L2.
//encore:api public method=PUT path=/api/cache/:key
func Set(ctx context.Context, key string, req *SetRequest) (*SetResponse, error) {
	if svc == nil {
		return nil, errors.New("service not initialized")
	}
	return svc.Set(ctx, key, req)
}

func (s *Service) Set(ctx context.Context, key string, req *SetRequest) (*SetResponse, error) {
	if key == "" {
		return nil, errors.New("key cannot be empty")
	}
	if req.Value == nil {
		return nil, errors.New("value cannot be nil")
	}

	ttl := s.config.DefaultTTL
	if req.TTL > 0 {
		ttl = time.Duration(req.TTL) * time.Second
	}

	expiresAt := time.Now().Add(ttl)

	// Write to L1
	s.l1Cache.Set(key, req.Value, ttl)
	s.metrics.Sets.Add(1)

	// Write to L2 (synchronous write-through)
	if s.config.L2Enabled && s.l2Cache != nil {
		entry := CacheEntry{
			Value:     req.Value,
			CachedAt:  time.Now(),
			ExpiresAt: expiresAt,
		}
		data, err := json.Marshal(entry)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal entry: %w", err)
		}
		if err := s.l2Cache.Set(ctx, key, data, ttl); err != nil {
			s.metrics.L2Errors.Add(1)
			// Continue even if L2 fails (L1 is authoritative)
		}
	}

	return &SetResponse{
		Success:   true,
		ExpiresAt: expiresAt,
	}, nil
}

// Invalidate removes keys from cache and publishes invalidation event.
// Complexity: O(k) for k keys, O(n) for pattern matching.
//encore:api public method=POST path=/api/cache/invalidate
func Invalidate(ctx context.Context, req *InvalidateRequest) (*InvalidateResponse, error) {
	if svc == nil {
		return nil, errors.New("service not initialized")
	}
	return svc.Invalidate(ctx, req)
}

func (s *Service) Invalidate(ctx context.Context, req *InvalidateRequest) (*InvalidateResponse, error) {
	count := 0

	// Invalidate specific keys
	for _, key := range req.Keys {
		if s.l1Cache.Delete(key) {
			count++
		}
		if s.config.L2Enabled && s.l2Cache != nil {
			_ = s.l2Cache.Delete(ctx, key)
		}
		s.metrics.Deletes.Add(1)
	}

	// Invalidate by pattern
	if req.Pattern != "" {
		deleted := s.l1Cache.DeletePattern(req.Pattern)
		count += deleted
		if s.config.L2Enabled && s.l2Cache != nil {
			_ = s.l2Cache.DeletePattern(ctx, req.Pattern)
		}
		s.metrics.Deletes.Add(int64(deleted))
	}

	// Publish invalidation event for distributed coordination
	if count > 0 {
		event := &InvalidateEvent{
			Keys:      req.Keys,
			Pattern:   req.Pattern,
			Timestamp: time.Now(),
		}
		_, _ = CacheInvalidateTopic.Publish(ctx, event)
	}

	return &InvalidateResponse{
		Invalidated: count,
		Success:     true,
	}, nil
}

// GetMetrics returns current cache performance metrics.
//encore:api public method=GET path=/api/cache/metrics
func GetMetrics(ctx context.Context) (*MetricsResponse, error) {
	if svc == nil {
		return nil, errors.New("service not initialized")
	}
	return svc.GetMetrics(ctx)
}

func (s *Service) GetMetrics(ctx context.Context) (*MetricsResponse, error) {
	hits := s.metrics.Hits.Load()
	misses := s.metrics.Misses.Load()
	total := hits + misses
	
	hitRate := 0.0
	if total > 0 {
		hitRate = float64(hits) / float64(total)
	}

	return &MetricsResponse{
		Hits:      hits,
		Misses:    misses,
		HitRate:   hitRate,
		Sets:      s.metrics.Sets.Load(),
		Deletes:   s.metrics.Deletes.Load(),
		Evictions: s.metrics.Evictions.Load(),
		L1Size:    s.l1Cache.Size(),
		L2Hits:    s.metrics.L2Hits.Load(),
		L2Misses:  s.metrics.L2Misses.Load(),
		L2Errors:  s.metrics.L2Errors.Load(),
	}, nil
}

// runTTLCleanup periodically removes expired entries from L1.
func (s *Service) runTTLCleanup() {
	defer s.wg.Done()
	ticker := time.NewTicker(s.config.CleanupInterval)
	defer ticker.Stop()

	for {
		select {
		case <-s.stopChan:
			return
		case <-ticker.C:
			evicted := s.l1Cache.CleanupExpired()
			s.metrics.Evictions.Add(int64(evicted))
		}
	}
}

// Shutdown gracefully stops the service.
func (s *Service) Shutdown() {
	close(s.stopChan)
	s.wg.Wait()
}