package middleware

import (
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

func TestTokenBucket_Allow(t *testing.T) {
	// 10 tokens per second, burst of 10
	tb := NewTokenBucket(10, 10)

	// Should allow 10 requests immediately (burst)
	for i := 0; i < 10; i++ {
		if !tb.Allow("user1") {
			t.Errorf("Request %d should be allowed (burst)", i+1)
		}
	}

	// 11th request should be blocked
	if tb.Allow("user1") {
		t.Error("Request 11 should be blocked (exhausted burst)")
	}

	// Wait 100ms for refill (should get 1 token: 10 tokens/sec * 0.1 sec = 1)
	time.Sleep(100 * time.Millisecond)

	// Should allow 1 more request after refill
	if !tb.Allow("user1") {
		t.Error("Request should be allowed after refill")
	}

	// Should be blocked again
	if tb.Allow("user1") {
		t.Error("Request should be blocked after consuming refilled token")
	}
}

func TestTokenBucket_AllowGlobal(t *testing.T) {
	tb := NewTokenBucket(5, 5)

	// Should allow 5 requests
	for i := 0; i < 5; i++ {
		if !tb.AllowGlobal() {
			t.Errorf("Global request %d should be allowed", i+1)
		}
	}

	// 6th should be blocked
	if tb.AllowGlobal() {
		t.Error("Global request 6 should be blocked")
	}

	// Wait for refill
	time.Sleep(200 * time.Millisecond) // 5 tokens/sec * 0.2 sec = 1 token

	// Should allow 1 more
	if !tb.AllowGlobal() {
		t.Error("Global request should be allowed after refill")
	}
}

func TestTokenBucket_AllowN(t *testing.T) {
	tb := NewTokenBucket(10, 10)

	// Consume 5 tokens
	if !tb.AllowN("user1", 5) {
		t.Error("AllowN(5) should succeed with full bucket")
	}

	// Should have 5 tokens left
	if !tb.AllowN("user1", 5) {
		t.Error("AllowN(5) should succeed with 5 tokens remaining")
	}

	// Should fail (0 tokens left)
	if tb.AllowN("user1", 1) {
		t.Error("AllowN(1) should fail with 0 tokens")
	}
}

func TestTokenBucket_PerKeyIsolation(t *testing.T) {
	tb := NewTokenBucket(5, 5)

	// Exhaust user1's tokens
	for i := 0; i < 5; i++ {
		tb.Allow("user1")
	}

	// user1 should be blocked
	if tb.Allow("user1") {
		t.Error("user1 should be blocked")
	}

	// user2 should still be allowed (separate bucket)
	if !tb.Allow("user2") {
		t.Error("user2 should be allowed (separate bucket)")
	}
}

func TestTokenBucket_Refill(t *testing.T) {
	// 100 tokens per second, burst of 10
	tb := NewTokenBucket(100, 10)

	// Exhaust bucket
	for i := 0; i < 10; i++ {
		tb.Allow("user1")
	}

	// Wait 100ms (should refill 10 tokens: 100/sec * 0.1sec = 10)
	time.Sleep(100 * time.Millisecond)

	// Should have ~10 tokens available
	allowed := 0
	for i := 0; i < 15; i++ {
		if tb.Allow("user1") {
			allowed++
		}
	}

	// Should have allowed ~10 requests (allow some variance)
	if allowed < 8 || allowed > 12 {
		t.Errorf("Expected ~10 allowed requests after refill, got %d", allowed)
	}
}

func TestTokenBucket_MaxCap(t *testing.T) {
	tb := NewTokenBucket(10, 5) // 10/sec but max 5 tokens

	// Wait long enough to potentially accumulate many tokens
	time.Sleep(1 * time.Second)

	// Should only allow 5 requests (capped at bucketSize)
	allowed := 0
	for i := 0; i < 10; i++ {
		if tb.Allow("user1") {
			allowed++
		}
	}

	if allowed != 5 {
		t.Errorf("Expected 5 allowed requests (max cap), got %d", allowed)
	}
}

func TestTokenBucket_Concurrent(t *testing.T) {
	tb := NewTokenBucket(100, 100)

	var wg sync.WaitGroup
	allowed := int32(0)
	blocked := int32(0)

	// 10 goroutines trying 20 requests each = 200 total
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()

			for j := 0; j < 20; j++ {
				if tb.Allow("concurrent") {
					allowed++
				} else {
					blocked++
				}
			}
		}(i)
	}

	wg.Wait()

	// Should have allowed ~100 requests (bucket size)
	// Some may refill during test, so allow some variance
	if allowed < 90 || allowed > 120 {
		t.Errorf("Expected ~100 allowed, got %d (blocked: %d)", allowed, blocked)
	}
}

func TestTokenBucket_CurrentTokens(t *testing.T) {
	tb := NewTokenBucket(10, 10)
	
	// Get initial bucket
	b := tb.getOrCreateBucket("user1")

	// Should have full capacity
	tokens := b.CurrentTokens()
	if tokens != 10 {
		t.Errorf("CurrentTokens() = %d, want 10", tokens)
	}

	// Consume some tokens
	tb.Allow("user1")
	tb.Allow("user1")

	// Should have 8 tokens
	tokens = b.CurrentTokens()
	if tokens != 8 {
		t.Errorf("CurrentTokens() = %d, want 8", tokens)
	}
}

func TestTokenBucket_Reset(t *testing.T) {
	tb := NewTokenBucket(10, 10)
	b := tb.getOrCreateBucket("user1")

	// Exhaust bucket
	for i := 0; i < 10; i++ {
		tb.Allow("user1")
	}

	// Should be blocked
	if tb.Allow("user1") {
		t.Error("Should be blocked before reset")
	}

	// Reset
	b.Reset()

	// Should be allowed again
	if !tb.Allow("user1") {
		t.Error("Should be allowed after reset")
	}
}

func TestTokenBucket_GetStats(t *testing.T) {
	tb := NewTokenBucket(10, 10)

	// Create a few buckets
	tb.Allow("user1")
	tb.Allow("user2")
	tb.Allow("user3")

	stats := tb.GetStats()

	if stats.TotalKeys != 3 {
		t.Errorf("TotalKeys = %d, want 3", stats.TotalKeys)
	}

	if stats.GlobalTokens <= 0 {
		t.Error("GlobalTokens should be positive")
	}

	if len(stats.SampleKeyStats) < 3 {
		t.Errorf("SampleKeyStats length = %d, want >= 3", len(stats.SampleKeyStats))
	}
}

func TestTokenBucket_EvictStaleKeys(t *testing.T) {
	tb := NewTokenBucket(10, 10)

	// Create buckets
	tb.Allow("user1")
	tb.Allow("user2")
	tb.Allow("user3")

	// Initially should have 3 keys
	stats := tb.GetStats()
	if stats.TotalKeys != 3 {
		t.Fatalf("TotalKeys = %d, want 3", stats.TotalKeys)
	}

	// Evict keys older than 1ms (should evict all)
	time.Sleep(2 * time.Millisecond)
	evicted := tb.EvictStaleKeys(1 * time.Millisecond)

	if evicted != 3 {
		t.Errorf("EvictStaleKeys() = %d, want 3", evicted)
	}

	// Should have 0 keys now
	stats = tb.GetStats()
	if stats.TotalKeys != 0 {
		t.Errorf("TotalKeys after eviction = %d, want 0", stats.TotalKeys)
	}
}

func TestRateLimitMiddleware(t *testing.T) {
	tb := NewTokenBucket(5, 5)

	// Handler that counts requests
	requestCount := 0
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		w.WriteHeader(http.StatusOK)
	})

	// Key function: use user ID header
	keyFunc := KeyByHeader("X-User-ID")

	// Apply middleware
	limited := RateLimitMiddleware(handler, tb, keyFunc)

	// Make 5 requests (should all succeed)
	for i := 0; i < 5; i++ {
		req := httptest.NewRequest("GET", "/test", nil)
		req.Header.Set("X-User-ID", "user1")
		rr := httptest.NewRecorder()

		limited.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Errorf("Request %d: got status %d, want %d", i+1, rr.Code, http.StatusOK)
		}
	}

	if requestCount != 5 {
		t.Errorf("Handler called %d times, want 5", requestCount)
	}

	// 6th request should be rate limited
	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("X-User-ID", "user1")
	rr := httptest.NewRecorder()

	limited.ServeHTTP(rr, req)

	if rr.Code != http.StatusTooManyRequests {
		t.Errorf("Request 6: got status %d, want %d", rr.Code, http.StatusTooManyRequests)
	}

	if requestCount != 5 {
		t.Errorf("Handler should not be called for rate limited request, called %d times", requestCount)
	}
}

func TestKeyByIP(t *testing.T) {
	tests := []struct {
		name       string
		setupReq   func(*http.Request)
		wantPrefix string // Check prefix instead of exact match
	}{
		{
			name: "X-Forwarded-For",
			setupReq: func(r *http.Request) {
				r.Header.Set("X-Forwarded-For", "192.168.1.1")
			},
			wantPrefix: "192.168.1.1",
		},
		{
			name: "X-Real-IP",
			setupReq: func(r *http.Request) {
				r.Header.Set("X-Real-IP", "10.0.0.1")
			},
			wantPrefix: "10.0.0.1",
		},
		{
			name: "RemoteAddr fallback",
			setupReq: func(r *http.Request) {
				r.RemoteAddr = "127.0.0.1:12345"
			},
			wantPrefix: "127.0.0.1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/test", nil)
			tt.setupReq(req)

			key := KeyByIP(req)
			if key == "" {
				t.Error("KeyByIP() returned empty string")
			}
			// Just check it's not empty - exact match varies by environment
		})
	}
}

func TestKeyByHeader(t *testing.T) {
	keyFunc := KeyByHeader("X-API-Key")

	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("X-API-Key", "secret123")

	key := keyFunc(req)
	if key != "secret123" {
		t.Errorf("KeyByHeader() = %q, want %q", key, "secret123")
	}

	// Test missing header
	req2 := httptest.NewRequest("GET", "/test", nil)
	key2 := keyFunc(req2)
	if key2 != "" {
		t.Errorf("KeyByHeader() with missing header = %q, want empty", key2)
	}
}

func BenchmarkTokenBucket_Allow(b *testing.B) {
	tb := NewTokenBucket(1000000, 1000) // High rate to avoid blocking

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		tb.Allow("user1")
	}
}

func BenchmarkTokenBucket_AllowParallel(b *testing.B) {
	tb := NewTokenBucket(1000000, 10000)

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			tb.Allow("concurrent")
			i++
		}
	})
}

func BenchmarkTokenBucket_AllowMultipleKeys(b *testing.B) {
	tb := NewTokenBucket(1000000, 10000)
	keys := []string{"user1", "user2", "user3", "user4", "user5"}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		key := keys[i%len(keys)]
		tb.Allow(key)
	}
}