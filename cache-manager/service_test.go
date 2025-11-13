package cachemanager

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// MockOriginFetcher simulates fetching from source of truth.
type MockOriginFetcher struct {
	mu     sync.Mutex
	data   map[string]interface{}
	calls  int
	delay  time.Duration
	errors map[string]error
}

func NewMockOriginFetcher() *MockOriginFetcher {
	return &MockOriginFetcher{
		data:   make(map[string]interface{}),
		errors: make(map[string]error),
	}
}

func (m *MockOriginFetcher) Fetch(ctx context.Context, key string) (interface{}, error) {
	m.mu.Lock()
	m.calls++
	delay := m.delay
	err := m.errors[key]
	m.mu.Unlock()

	if delay > 0 {
		time.Sleep(delay)
	}

	if err != nil {
		return nil, err
	}

	m.mu.Lock()
	val, exists := m.data[key]
	m.mu.Unlock()

	if !exists {
		return nil, errors.New("not found")
	}

	return val, nil
}

func (m *MockOriginFetcher) Set(key string, value interface{}) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.data[key] = value
}

func (m *MockOriginFetcher) SetError(key string, err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.errors[key] = err
}

func (m *MockOriginFetcher) CallCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.calls
}

func (m *MockOriginFetcher) ResetCalls() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.calls = 0
}

// MockRemoteCache simulates L2 distributed cache.
type MockRemoteCache struct {
	mu    sync.RWMutex
	data  map[string][]byte
	calls map[string]int
}

func NewMockRemoteCache() *MockRemoteCache {
	return &MockRemoteCache{
		data:  make(map[string][]byte),
		calls: make(map[string]int),
	}
}

func (m *MockRemoteCache) Get(ctx context.Context, key string) ([]byte, bool, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	
	m.calls["get"]++
	
	val, exists := m.data[key]
	return val, exists, nil
}

func (m *MockRemoteCache) Set(ctx context.Context, key string, value []byte, ttl time.Duration) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	
	m.calls["set"]++
	m.data[key] = value
	return nil
}

func (m *MockRemoteCache) Delete(ctx context.Context, key string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	
	m.calls["delete"]++
	delete(m.data, key)
	return nil
}

func (m *MockRemoteCache) DeletePattern(ctx context.Context, pattern string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	
	m.calls["delete_pattern"]++
	// Simple pattern matching for tests
	for key := range m.data {
		if matchesPattern(key, pattern, pattern[:len(pattern)-1]) {
			delete(m.data, key)
		}
	}
	return nil
}

func (m *MockRemoteCache) CallCount(op string) int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.calls[op]
}

// setupTestService creates a service instance with mocks for testing.
func setupTestService() (*Service, *MockOriginFetcher, *MockRemoteCache) {
	config := Config{
		L1MaxEntries:    100,
		DefaultTTL:      1 * time.Hour,
		CleanupInterval: 100 * time.Millisecond,
		L2Enabled:       true,
	}

	mockOrigin := NewMockOriginFetcher()
	mockL2 := NewMockRemoteCache()

	svc := &Service{
		l1Cache:     NewL1Cache(config.L1MaxEntries),
		l2Cache:     mockL2,
		originFetch: mockOrigin,
		coalescer:   NewRequestCoalescer(),
		metrics:     &Metrics{},
		config:      config,
		stopChan:    make(chan struct{}),
	}

	return svc, mockOrigin, mockL2
}

func TestL1Cache_BasicOperations(t *testing.T) {
	cache := NewL1Cache(100)

	// Test Set and Get
	cache.Set("key1", "value1", 1*time.Hour)
	entry, ok := cache.Get("key1")
	if !ok || entry.Value != "value1" {
		t.Errorf("Expected value1, got %v, ok=%v", entry, ok)
	}

	// Test non-existent key
	_, ok = cache.Get("nonexistent")
	if ok {
		t.Error("Expected false for non-existent key")
	}

	// Test Delete
	if !cache.Delete("key1") {
		t.Error("Expected successful delete")
	}
	_, ok = cache.Get("key1")
	if ok {
		t.Error("Key should be deleted")
	}
}

func TestL1Cache_TTLExpiration(t *testing.T) {
	cache := NewL1Cache(100)

	// Set with short TTL
	cache.Set("key1", "value1", 50*time.Millisecond)
	
	// Should be available immediately
	_, ok := cache.Get("key1")
	if !ok {
		t.Error("Key should exist immediately after set")
	}

	// Wait for expiration
	time.Sleep(100 * time.Millisecond)

	// Should be expired
	_, ok = cache.Get("key1")
	if ok {
		t.Error("Key should be expired")
	}
}

func TestL1Cache_LRUEviction(t *testing.T) {
	cache := NewL1Cache(3) // Small capacity for testing

	// Fill cache
	cache.Set("key1", "value1", 1*time.Hour)
	cache.Set("key2", "value2", 1*time.Hour)
	cache.Set("key3", "value3", 1*time.Hour)

	// Access key1 to make it recently used
	cache.Get("key1")

	// Add new key, should evict key2 (least recently used)
	cache.Set("key4", "value4", 1*time.Hour)

	// key1 and key3 should still exist
	if _, ok := cache.Get("key1"); !ok {
		t.Error("key1 should still exist")
	}
	if _, ok := cache.Get("key3"); !ok {
		t.Error("key3 should still exist")
	}

	// key2 should be evicted
	if _, ok := cache.Get("key2"); ok {
		t.Error("key2 should be evicted")
	}
}

func TestL1Cache_PatternDelete(t *testing.T) {
	cache := NewL1Cache(100)

	// Set multiple keys with pattern
	cache.Set("user:1:profile", "profile1", 1*time.Hour)
	cache.Set("user:1:settings", "settings1", 1*time.Hour)
	cache.Set("user:2:profile", "profile2", 1*time.Hour)
	cache.Set("product:1", "product1", 1*time.Hour)

	// Delete by pattern
	deleted := cache.DeletePattern("user:1:*")
	if deleted != 2 {
		t.Errorf("Expected 2 deletions, got %d", deleted)
	}

	// Verify correct keys deleted
	if _, ok := cache.Get("user:1:profile"); ok {
		t.Error("user:1:profile should be deleted")
	}
	if _, ok := cache.Get("user:1:settings"); ok {
		t.Error("user:1:settings should be deleted")
	}
	if _, ok := cache.Get("user:2:profile"); !ok {
		t.Error("user:2:profile should still exist")
	}
	if _, ok := cache.Get("product:1"); !ok {
		t.Error("product:1 should still exist")
	}
}

func TestL1Cache_CleanupExpired(t *testing.T) {
	cache := NewL1Cache(100)

	// Set keys with different TTLs
	cache.Set("key1", "value1", 50*time.Millisecond)
	cache.Set("key2", "value2", 200*time.Millisecond)
	cache.Set("key3", "value3", 1*time.Hour)

	// Wait for key1 to expire
	time.Sleep(100 * time.Millisecond)

	// Run cleanup
	evicted := cache.CleanupExpired()
	if evicted != 1 {
		t.Errorf("Expected 1 eviction, got %d", evicted)
	}

	// Verify key1 removed, others remain
	if _, ok := cache.Get("key1"); ok {
		t.Error("key1 should be expired")
	}
	if _, ok := cache.Get("key2"); !ok {
		t.Error("key2 should still exist")
	}
	if _, ok := cache.Get("key3"); !ok {
		t.Error("key3 should still exist")
	}
}

func TestService_Get_L1Hit(t *testing.T) {
	svc, _, _ := setupTestService()

	// Pre-populate L1 cache
	svc.l1Cache.Set("key1", "value1", 1*time.Hour)

	// Get should hit L1
	resp, err := svc.Get(context.Background(), "key1")
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if !resp.Hit || resp.Source != "l1" || resp.Value != "value1" {
		t.Errorf("Expected L1 hit with value1, got %+v", resp)
	}

	// Verify metrics
	if svc.metrics.Hits.Load() != 1 {
		t.Errorf("Expected 1 hit, got %d", svc.metrics.Hits.Load())
	}
}

func TestService_Get_OriginFetch(t *testing.T) {
	svc, mockOrigin, _ := setupTestService()

	// Set up origin data
	mockOrigin.Set("key1", "origin_value")

	// Get should miss cache and fetch from origin
	resp, err := svc.Get(context.Background(), "key1")
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if !resp.Hit || resp.Source != "origin" || resp.Value != "origin_value" {
		t.Errorf("Expected origin fetch with origin_value, got %+v", resp)
	}

	// Verify origin was called
	if mockOrigin.CallCount() != 1 {
		t.Errorf("Expected 1 origin call, got %d", mockOrigin.CallCount())
	}

	// Second get should hit L1 (populated from origin)
	mockOrigin.ResetCalls()
	resp2, _ := svc.Get(context.Background(), "key1")
	if resp2.Source != "l1" {
		t.Errorf("Expected L1 hit on second call, got %s", resp2.Source)
	}
	if mockOrigin.CallCount() != 0 {
		t.Error("Origin should not be called on L1 hit")
	}
}

func TestService_Set(t *testing.T) {
	svc, _, mockL2 := setupTestService()

	req := &SetRequest{
		Key:   "key1",
		Value: "value1",
		TTL:   3600,
	}

	resp, err := svc.Set(context.Background(), "key1", req)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if !resp.Success {
		t.Error("Expected successful set")
	}

	// Verify L1 contains value
	entry, ok := svc.l1Cache.Get("key1")
	if !ok || entry.Value != "value1" {
		t.Errorf("L1 should contain value1, got %v", entry)
	}

	// Give L2 async write time to complete
	time.Sleep(50 * time.Millisecond)

	// Verify L2 was called
	if mockL2.CallCount("set") == 0 {
		t.Error("L2 set should be called")
	}

	// Verify metrics
	if svc.metrics.Sets.Load() != 1 {
		t.Errorf("Expected 1 set, got %d", svc.metrics.Sets.Load())
	}
}

func TestService_Invalidate_Keys(t *testing.T) {
	svc, _, mockL2 := setupTestService()

	// Set up some cached data
	svc.l1Cache.Set("key1", "value1", 1*time.Hour)
	svc.l1Cache.Set("key2", "value2", 1*time.Hour)

	req := &InvalidateRequest{
		Keys: []string{"key1"},
	}

	resp, err := svc.Invalidate(context.Background(), req)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if resp.Invalidated != 1 || !resp.Success {
		t.Errorf("Expected 1 invalidation, got %+v", resp)
	}

	// Verify key1 deleted, key2 remains
	if _, ok := svc.l1Cache.Get("key1"); ok {
		t.Error("key1 should be deleted")
	}
	if _, ok := svc.l1Cache.Get("key2"); !ok {
		t.Error("key2 should still exist")
	}

	// Verify L2 delete called
	if mockL2.CallCount("delete") == 0 {
		t.Error("L2 delete should be called")
	}
}

func TestService_Invalidate_Pattern(t *testing.T) {
	svc, _, _ := setupTestService()

	// Set up cached data with pattern
	svc.l1Cache.Set("user:1:profile", "profile1", 1*time.Hour)
	svc.l1Cache.Set("user:1:settings", "settings1", 1*time.Hour)
	svc.l1Cache.Set("user:2:profile", "profile2", 1*time.Hour)

	req := &InvalidateRequest{
		Pattern: "user:1:*",
	}

	resp, err := svc.Invalidate(context.Background(), req)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if resp.Invalidated != 2 {
		t.Errorf("Expected 2 invalidations, got %d", resp.Invalidated)
	}

	// Verify correct keys deleted
	if _, ok := svc.l1Cache.Get("user:1:profile"); ok {
		t.Error("user:1:profile should be deleted")
	}
	if _, ok := svc.l1Cache.Get("user:2:profile"); !ok {
		t.Error("user:2:profile should still exist")
	}
}

func TestService_Metrics(t *testing.T) {
	svc, mockOrigin, _ := setupTestService()

	// Perform various operations
	mockOrigin.Set("key1", "value1")
	
	svc.Get(context.Background(), "key1")          // miss + origin
	svc.Get(context.Background(), "key1")          // hit
	svc.Set(context.Background(), "key2", &SetRequest{Key: "key2", Value: "value2"})
	svc.Invalidate(context.Background(), &InvalidateRequest{Keys: []string{"key1"}})

	// Get metrics
	resp, err := svc.GetMetrics(context.Background())
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	// Verify metrics
	if resp.Hits != 1 {
		t.Errorf("Expected 1 hit, got %d", resp.Hits)
	}
	if resp.Misses != 1 {
		t.Errorf("Expected 1 miss, got %d", resp.Misses)
	}
	if resp.Sets != 1 {
		t.Errorf("Expected 1 set, got %d", resp.Sets)
	}
	if resp.Deletes != 1 {
		t.Errorf("Expected 1 delete, got %d", resp.Deletes)
	}

	// Hit rate should be 50%
	expectedHitRate := 0.5
	if resp.HitRate != expectedHitRate {
		t.Errorf("Expected hit rate %.2f, got %.2f", expectedHitRate, resp.HitRate)
	}
}

func TestRequestCoalescer_Basic(t *testing.T) {
	coalescer := NewRequestCoalescer()
	callCount := 0

	fn := func() (interface{}, error) {
		callCount++
		time.Sleep(50 * time.Millisecond) // Simulate slow operation
		return "result", nil
	}

	// Single call
	val, err := coalescer.Do("key1", fn)
	if err != nil || val != "result" {
		t.Errorf("Expected result, got %v, %v", val, err)
	}
	if callCount != 1 {
		t.Errorf("Expected 1 call, got %d", callCount)
	}
}

func TestRequestCoalescer_ConcurrentCalls(t *testing.T) {
	coalescer := NewRequestCoalescer()
	var callCount int32

	fn := func() (interface{}, error) {
		atomic.AddInt32(&callCount, 1)
		time.Sleep(100 * time.Millisecond) // Simulate slow operation
		return "result", nil
	}

	// Launch 10 concurrent requests for same key
	var wg sync.WaitGroup
	results := make(chan interface{}, 10)
	errors := make(chan error, 10)

	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			val, err := coalescer.Do("key1", fn)
			results <- val
			errors <- err
		}()
	}

	wg.Wait()
	close(results)
	close(errors)

	// Verify only 1 call was made
	if atomic.LoadInt32(&callCount) != 1 {
		t.Errorf("Expected 1 call, got %d (should coalesce)", callCount)
	}

	// Verify all goroutines got the same result
	for val := range results {
		if val != "result" {
			t.Errorf("Expected result, got %v", val)
		}
	}

	for err := range errors {
		if err != nil {
			t.Errorf("Unexpected error: %v", err)
		}
	}
}

func TestRequestCoalescer_DifferentKeys(t *testing.T) {
	coalescer := NewRequestCoalescer()
	var callCount int32

	fn := func() (interface{}, error) {
		atomic.AddInt32(&callCount, 1)
		time.Sleep(50 * time.Millisecond)
		return "result", nil
	}

	// Launch concurrent requests for different keys
	var wg sync.WaitGroup
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func(key string) {
			defer wg.Done()
			_, _ = coalescer.Do(key, fn)
		}(fmt.Sprintf("key%d", i))
	}

	wg.Wait()

	// Each key should trigger its own call
	if atomic.LoadInt32(&callCount) != 5 {
		t.Errorf("Expected 5 calls for 5 keys, got %d", callCount)
	}
}

func TestHandleInvalidateEvent(t *testing.T) {
	svc, _, _ := setupTestService()
	
	// Set up initial data
	svc.l1Cache.Set("key1", "value1", 1*time.Hour)
	svc.l1Cache.Set("key2", "value2", 1*time.Hour)

	// Simulate invalidation event
	event := &InvalidateEvent{
		Keys:      []string{"key1"},
		Timestamp: time.Now(),
	}

	err := HandleInvalidateEvent(context.Background(), event)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	// Verify key1 deleted
	if _, ok := svc.l1Cache.Get("key1"); ok {
		t.Error("key1 should be deleted after invalidation event")
	}

	// Verify key2 still exists
	if _, ok := svc.l1Cache.Get("key2"); !ok {
		t.Error("key2 should still exist")
	}
}

func TestHandleRefreshEvent(t *testing.T) {
	svc, _, _ := setupTestService()

	// Simulate refresh event
	event := &RefreshEvent{
		Key:       "key1",
		Value:     "fresh_value",
		TTL:       3600,
		Timestamp: time.Now(),
		Priority:  "high",
	}

	err := HandleRefreshEvent(context.Background(), event)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	// Verify key1 populated in L1
	entry, ok := svc.l1Cache.Get("key1")
	if !ok || entry.Value != "fresh_value" {
		t.Errorf("Expected fresh_value in L1, got %v", entry)
	}
}

func TestConcurrentAccess(t *testing.T) {
	svc, mockOrigin, _ := setupTestService()

	// Set up origin data
	for i := 0; i < 100; i++ {
		mockOrigin.Set(fmt.Sprintf("key%d", i), fmt.Sprintf("value%d", i))
	}

	// Concurrent reads and writes
	var wg sync.WaitGroup
	errors := make(chan error, 300)

	// 100 concurrent readers
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(key string) {
			defer wg.Done()
			_, err := svc.Get(context.Background(), key)
			if err != nil {
				errors <- err
			}
		}(fmt.Sprintf("key%d", i%50))
	}

	// 100 concurrent writers
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, err := svc.Set(context.Background(), fmt.Sprintf("key%d", i), &SetRequest{
				Key:   fmt.Sprintf("key%d", i),
				Value: fmt.Sprintf("new_value%d", i),
			})
			if err != nil {
				errors <- err
			}
		}(i)
	}

	// 100 concurrent deletes
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, err := svc.Invalidate(context.Background(), &InvalidateRequest{
				Keys: []string{fmt.Sprintf("key%d", i%20)},
			})
			if err != nil {
				errors <- err
			}
		}(i)
	}

	wg.Wait()
	close(errors)

	// Check for errors
	for err := range errors {
t.Errorf("Concurrent operation error: %v", err)
	}

	// Verify service is still functional
	resp, err := svc.GetMetrics(context.Background())
	if err != nil {
		t.Errorf("GetMetrics failed after concurrent test: %v", err)
	}

	t.Logf("After concurrent test - Hits: %d, Misses: %d, Sets: %d, Deletes: %d",
		resp.Hits, resp.Misses, resp.Sets, resp.Deletes)
}

func TestTTLCleanup_Background(t *testing.T) {
	config := Config{
		L1MaxEntries:    100,
		DefaultTTL:      1 * time.Hour,
		CleanupInterval: 50 * time.Millisecond,
		L2Enabled:       false,
	}

	svc := &Service{
		l1Cache:     NewL1Cache(config.L1MaxEntries),
		l2Cache:     nil,
		originFetch: nil,
		coalescer:   NewRequestCoalescer(),
		metrics:     &Metrics{},
		config:      config,
		stopChan:    make(chan struct{}),
	}

	// Start background cleanup
	svc.wg.Add(1)
	go svc.runTTLCleanup()

	// Add entries with short TTL
	svc.l1Cache.Set("expire1", "val1", 100*time.Millisecond)
	svc.l1Cache.Set("expire2", "val2", 100*time.Millisecond)
	svc.l1Cache.Set("keep", "val3", 1*time.Hour)

	// Wait for cleanup to run
	time.Sleep(200 * time.Millisecond)

	// Check evictions happened
	evictions := svc.metrics.Evictions.Load()
	if evictions < 2 {
		t.Errorf("Expected at least 2 evictions, got %d", evictions)
	}

	// Verify expired keys removed
	if _, ok := svc.l1Cache.Get("expire1"); ok {
		t.Error("expire1 should be removed")
	}
	if _, ok := svc.l1Cache.Get("keep"); !ok {
		t.Error("keep should still exist")
	}

	// Shutdown
	svc.Shutdown()
}

func BenchmarkL1Cache_Get(b *testing.B) {
	cache := NewL1Cache(10000)
	cache.Set("key1", "value1", 1*time.Hour)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		cache.Get("key1")
	}
}

func BenchmarkL1Cache_Set(b *testing.B) {
	cache := NewL1Cache(10000)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		cache.Set(fmt.Sprintf("key%d", i), fmt.Sprintf("value%d", i), 1*time.Hour)
	}
}

func BenchmarkL1Cache_ConcurrentGet(b *testing.B) {
	cache := NewL1Cache(10000)
	
	// Pre-populate
	for i := 0; i < 1000; i++ {
		cache.Set(fmt.Sprintf("key%d", i), fmt.Sprintf("value%d", i), 1*time.Hour)
	}

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			cache.Get(fmt.Sprintf("key%d", i%1000))
			i++
		}
	})
}

func BenchmarkRequestCoalescer(b *testing.B) {
	coalescer := NewRequestCoalescer()
	
	fn := func() (interface{}, error) {
		return "result", nil
	}

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			coalescer.Do(fmt.Sprintf("key%d", i%100), fn)
			i++
		}
	})
}

func TestService_EmptyKey(t *testing.T) {
	svc, _, _ := setupTestService()

	// Test Get with empty key
	_, err := svc.Get(context.Background(), "")
	if err == nil {
		t.Error("Expected error for empty key")
	}

	// Test Set with empty key
	_, err = svc.Set(context.Background(), "", &SetRequest{Value: "value"})
	if err == nil {
		t.Error("Expected error for empty key")
	}
}

func TestService_NilValue(t *testing.T) {
	svc, _, _ := setupTestService()

	_, err := svc.Set(context.Background(), "key1", &SetRequest{
		Key:   "key1",
		Value: nil,
	})
	if err == nil {
		t.Error("Expected error for nil value")
	}
}

func TestService_CustomTTL(t *testing.T) {
	svc, _, _ := setupTestService()

	req := &SetRequest{
		Key:   "key1",
		Value: "value1",
		TTL:   2, // 2 seconds
	}

	resp, err := svc.Set(context.Background(), "key1", req)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	// Verify TTL is set correctly
	expectedExpiry := time.Now().Add(2 * time.Second)
	if resp.ExpiresAt.Before(expectedExpiry.Add(-1*time.Second)) ||
		resp.ExpiresAt.After(expectedExpiry.Add(1*time.Second)) {
		t.Errorf("Expected expiry around %v, got %v", expectedExpiry, resp.ExpiresAt)
	}
}

func TestL1Cache_Size(t *testing.T) {
	cache := NewL1Cache(100)

	if cache.Size() != 0 {
		t.Errorf("Expected size 0, got %d", cache.Size())
	}

	cache.Set("key1", "value1", 1*time.Hour)
	cache.Set("key2", "value2", 1*time.Hour)

	if cache.Size() != 2 {
		t.Errorf("Expected size 2, got %d", cache.Size())
	}

	cache.Delete("key1")

	if cache.Size() != 1 {
		t.Errorf("Expected size 1, got %d", cache.Size())
	}
}

func TestL1Cache_Clear(t *testing.T) {
	cache := NewL1Cache(100)

	cache.Set("key1", "value1", 1*time.Hour)
	cache.Set("key2", "value2", 1*time.Hour)

	cache.Clear()

	if cache.Size() != 0 {
		t.Errorf("Expected size 0 after clear, got %d", cache.Size())
	}

	if _, ok := cache.Get("key1"); ok {
		t.Error("Cache should be empty after clear")
	}
}

func TestRequestCoalescer_InFlight(t *testing.T) {
	coalescer := NewRequestCoalescer()

	if coalescer.InFlight() != 0 {
		t.Errorf("Expected 0 in-flight, got %d", coalescer.InFlight())
	}

	// Start a slow call
	done := make(chan bool)
	go func() {
		coalescer.Do("key1", func() (interface{}, error) {
			time.Sleep(100 * time.Millisecond)
			return "result", nil
		})
		done <- true
	}()

	// Give goroutine time to start
	time.Sleep(10 * time.Millisecond)

	// Should have 1 in-flight
	if coalescer.InFlight() != 1 {
		t.Errorf("Expected 1 in-flight, got %d", coalescer.InFlight())
	}

	<-done

	// Should be back to 0
	time.Sleep(10 * time.Millisecond)
	if coalescer.InFlight() != 0 {
		t.Errorf("Expected 0 in-flight after completion, got %d", coalescer.InFlight())
	}
}

func TestRequestCoalescer_Forget(t *testing.T) {
	coalescer := NewRequestCoalescer()

	// Start a call
	go coalescer.Do("key1", func() (interface{}, error) {
		time.Sleep(100 * time.Millisecond)
		return "result", nil
	})

	time.Sleep(10 * time.Millisecond)

	// Forget the key
	coalescer.Forget("key1")

	// Should allow new call for same key
	callCount := 0
	coalescer.Do("key1", func() (interface{}, error) {
		callCount++
		return "new_result", nil
	})

	if callCount != 1 {
		t.Error("Forget should allow new call")
	}
}

func TestPolicyEngine(t *testing.T) {
	engine := DefaultPolicyEngine()

	entry := &CacheEntry{
		Value:     "test",
		ExpiresAt: time.Now().Add(1 * time.Hour),
	}

	// Should not evict valid entry
	if engine.ShouldEvict(entry) {
		t.Error("Should not evict non-expired entry")
	}

	// Expired entry should be evicted
	expiredEntry := &CacheEntry{
		Value:     "test",
		ExpiresAt: time.Now().Add(-1 * time.Hour),
	}

	if !engine.ShouldEvict(expiredEntry) {
		t.Error("Should evict expired entry")
	}

	// Test access recording
	engine.RecordAccess("key1")
	engine.RecordSet("key2", "value2", 1*time.Hour)
}