// Package middleware provides rate limiting middleware using token bucket algorithm.
//
// This file implements a generic token-bucket rate limiter with:
//   - Per-key rate limiting (e.g., per-user, per-IP)
//   - Global rate limiting (all requests)
//   - Concurrent-safe using sync.Map and atomic operations
//   - On-demand refill (no background goroutines)
//   - Configurable bucket size and refill rate
//
// Design Notes:
//   - Token bucket allows bursts up to bucket capacity
//   - Refill happens on-demand during Allow() calls
//   - Uses sync.Map for per-key buckets (concurrent access)
//   - Uses sync/atomic for lock-free token operations
//   - No cleanup of stale keys (recommend periodic eviction)
//
// Algorithm:
//   - Tokens refill at constant rate (refillRate tokens/second)
//   - Max tokens = bucketSize
//   - Each request consumes 1 token
//   - Request blocked if tokens < 1
//
// Trade-offs:
//   - Token bucket vs leaky bucket: chose token for burst support
//   - Per-key state vs shared: chose per-key for isolation
//   - Lazy refill vs background: chose lazy to avoid goroutines
//   - Memory: O(N) where N = number of unique keys
//
// Complexity:
//   - Allow(): O(1) amortized
//   - Memory: ~200 bytes per key (bucket state + map overhead)
//
// Production extensions:
//   - Add TTL-based eviction for inactive keys
//   - Implement sliding window for more precise limiting
//   - Add distributed rate limiting with Redis
//   - Expose metrics (allowed/blocked counts)
package middleware

import (
	"fmt"
	"net/http"
	"sync"
	"sync/atomic"
	"time"
)

// TokenBucket implements a token bucket rate limiter.
//
// Example usage:
//
//	// 100 requests per second, burst of 200
//	limiter := NewTokenBucket(100, 200)
//
//	// Per-IP limiting
//	if limiter.Allow(clientIP) {
//	    handleRequest()
//	}
//
//	// Global limiting
//	if limiter.AllowGlobal() {
//	    handleRequest()
//	}
type TokenBucket struct {
	refillRate float64 // Tokens per second
	bucketSize int64   // Maximum tokens

	// Per-key buckets stored in sync.Map
	// Key: string, Value: *bucket
	buckets sync.Map

	// Global bucket for AllowGlobal()
	globalBucket *bucket
}

// bucket represents a single token bucket.
type bucket struct {
	tokens     int64 // Current token count (atomic)
	lastRefill int64 // Last refill timestamp in nanoseconds (atomic)
	maxTokens  int64 // Maximum tokens
	refillRate float64
}

// NewTokenBucket creates a new token bucket rate limiter.
//
// Parameters:
//   - refillRate: Tokens added per second (e.g., 100 = 100 requests/sec)
//   - bucketSize: Maximum tokens (burst capacity)
//
// Example:
//   - 10 req/sec with burst of 20: NewTokenBucket(10, 20)
//   - 1000 req/sec with burst of 5000: NewTokenBucket(1000, 5000)
func NewTokenBucket(refillRate float64, bucketSize int64) *TokenBucket {
	if refillRate <= 0 {
		panic("refillRate must be positive")
	}
	if bucketSize <= 0 {
		panic("bucketSize must be positive")
	}

	return &TokenBucket{
		refillRate: refillRate,
		bucketSize: bucketSize,
		globalBucket: &bucket{
			tokens:     bucketSize,
			lastRefill: time.Now().UnixNano(),
			maxTokens:  bucketSize,
			refillRate: refillRate,
		},
	}
}

// Allow checks if a request for the given key is allowed.
// Returns true if allowed, false if rate limited.
//
// This is thread-safe and uses atomic operations for lock-free updates.
//
// Complexity: O(1) amortized
func (tb *TokenBucket) Allow(key string) bool {
	if key == "" {
		return false
	}

	// Get or create bucket for this key
	b := tb.getOrCreateBucket(key)

	// Try to consume a token
	return b.tryConsume(1)
}

// AllowGlobal checks if a request is allowed against the global limit.
// This applies to all requests regardless of key.
//
// Use this for system-wide rate limiting.
func (tb *TokenBucket) AllowGlobal() bool {
	return tb.globalBucket.tryConsume(1)
}

// AllowN checks if N tokens can be consumed for the given key.
// Useful for operations with variable cost (e.g., batch writes).
func (tb *TokenBucket) AllowN(key string, n int) bool {
	if key == "" || n <= 0 {
		return false
	}

	b := tb.getOrCreateBucket(key)
	return b.tryConsume(int64(n))
}

// getOrCreateBucket retrieves or creates a bucket for the given key.
func (tb *TokenBucket) getOrCreateBucket(key string) *bucket {
	// Fast path: bucket exists
	if b, ok := tb.buckets.Load(key); ok {
		return b.(*bucket)
	}

	// Slow path: create new bucket
	newBucket := &bucket{
		tokens:     tb.bucketSize,
		lastRefill: time.Now().UnixNano(),
		maxTokens:  tb.bucketSize,
		refillRate: tb.refillRate,
	}

	// Try to store (may lose race, that's OK)
	actual, _ := tb.buckets.LoadOrStore(key, newBucket)
	return actual.(*bucket)
}

// tryConsume attempts to consume n tokens from the bucket.
// Returns true if successful, false if insufficient tokens.
//
// This method is lock-free using atomic operations.
func (b *bucket) tryConsume(n int64) bool {
	now := time.Now().UnixNano()

	for {
		// Load current state
		currentTokens := atomic.LoadInt64(&b.tokens)
		lastRefill := atomic.LoadInt64(&b.lastRefill)

		// Calculate tokens to add based on elapsed time
		elapsed := time.Duration(now - lastRefill)
		tokensToAdd := int64(b.refillRate * elapsed.Seconds())

		// Calculate new token count (capped at max)
		newTokens := currentTokens + tokensToAdd
		if newTokens > b.maxTokens {
			newTokens = b.maxTokens
		}

		// Check if we have enough tokens
		if newTokens < n {
			return false
		}

		// Try to consume tokens atomically
		if atomic.CompareAndSwapInt64(&b.tokens, currentTokens, newTokens-n) {
			// Update last refill time (best-effort, race is OK)
			atomic.StoreInt64(&b.lastRefill, now)
			return true
		}

		// CAS failed, retry
	}
}

// Reset resets the bucket to full capacity.
// Useful for testing or manual intervention.
func (b *bucket) Reset() {
	atomic.StoreInt64(&b.tokens, b.maxTokens)
	atomic.StoreInt64(&b.lastRefill, time.Now().UnixNano())
}

// CurrentTokens returns the current token count (approximate).
// This is a snapshot and may change immediately.
func (b *bucket) CurrentTokens() int64 {
	b.tryConsume(0) // Trigger refill
	return atomic.LoadInt64(&b.tokens)
}

// RateLimitMiddleware wraps an HTTP handler with rate limiting.
//
// Key extraction strategies:
//   - keyFunc: function to extract rate limit key from request
//     Common keys: IP address, user ID, API key
//
// Example:
//
//	limiter := NewTokenBucket(100, 200)
//	keyFunc := func(r *http.Request) string {
//	    return r.RemoteAddr // Rate limit by IP
//	}
//	limited := RateLimitMiddleware(handler, limiter, keyFunc)
func RateLimitMiddleware(
	next http.Handler,
	limiter *TokenBucket,
	keyFunc func(*http.Request) string,
) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Extract rate limit key
		key := keyFunc(r)
		if key == "" {
			// No key = allow (or could default to global limit)
			next.ServeHTTP(w, r)
			return
		}

		// Check rate limit
		if !limiter.Allow(key) {
			http.Error(w, "Rate limit exceeded", http.StatusTooManyRequests)
			return
		}

		// Request allowed
		next.ServeHTTP(w, r)
	})
}

// KeyByIP extracts the IP address from the request for rate limiting.
func KeyByIP(r *http.Request) string {
	// Check X-Forwarded-For header (if behind proxy)
	if forwarded := r.Header.Get("X-Forwarded-For"); forwarded != "" {
		return forwarded
	}

	// Check X-Real-IP header
	if realIP := r.Header.Get("X-Real-IP"); realIP != "" {
		return realIP
	}

	// Fallback to RemoteAddr
	return r.RemoteAddr
}

// KeyByHeader extracts a header value for rate limiting.
// Useful for API key or user ID based limiting.
func KeyByHeader(headerName string) func(*http.Request) string {
	return func(r *http.Request) string {
		return r.Header.Get(headerName)
	}
}

// Stats returns rate limiter statistics.
type Stats struct {
	TotalKeys      int              // Number of unique keys
	GlobalTokens   int64            // Current global tokens
	SampleKeyStats []KeyStats       // Sample of per-key stats
}

type KeyStats struct {
	Key    string
	Tokens int64
}

// GetStats returns current rate limiter statistics.
// WARNING: This iterates all keys and can be slow for large key counts.
func (tb *TokenBucket) GetStats() Stats {
	stats := Stats{
		GlobalTokens:   tb.globalBucket.CurrentTokens(),
		SampleKeyStats: make([]KeyStats, 0, 10),
	}

	// Count keys and sample some
	count := 0
	tb.buckets.Range(func(key, value interface{}) bool {
		count++
		
		// Sample first 10 keys
		if len(stats.SampleKeyStats) < 10 {
			b := value.(*bucket)
			stats.SampleKeyStats = append(stats.SampleKeyStats, KeyStats{
				Key:    key.(string),
				Tokens: b.CurrentTokens(),
			})
		}

		return true
	})

	stats.TotalKeys = count
	return stats
}

// EvictStaleKeys removes keys that haven't been used in the given duration.
// Call this periodically to prevent unbounded memory growth.
//
// WARNING: This iterates all keys and can be slow.
func (tb *TokenBucket) EvictStaleKeys(staleDuration time.Duration) int {
	staleThreshold := time.Now().Add(-staleDuration).UnixNano()
	evicted := 0

	tb.buckets.Range(func(key, value interface{}) bool {
		b := value.(*bucket)
		lastRefill := atomic.LoadInt64(&b.lastRefill)

		if lastRefill < staleThreshold {
			tb.buckets.Delete(key)
			evicted++
		}

		return true
	})

	return evicted
}

// String returns a human-readable representation of the rate limiter config.
func (tb *TokenBucket) String() string {
	return fmt.Sprintf("TokenBucket{rate=%.1f/s, burst=%d}", tb.refillRate, tb.bucketSize)
}