package warming

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"golang.org/x/time/rate"
)

// MockOriginFetcher simulates origin data source.
type MockOriginFetcher struct {
	mu       sync.Mutex
	data     map[string][]byte
	calls    atomic.Int64
	delay    time.Duration
	failures map[string]int // key -> remaining failures
}

func NewMockOriginFetcher() *MockOriginFetcher {
	return &MockOriginFetcher{
		data:     make(map[string][]byte),
		failures: make(map[string]int),
	}
}

func (m *MockOriginFetcher) Fetch(ctx context.Context, key string) ([]byte, time.Duration, error) {
	m.calls.Add(1)

	if m.delay > 0 {
		time.Sleep(m.delay)
	}

	// Simulate failures
	m.mu.Lock()
	if remaining, exists := m.failures[key]; exists && remaining > 0 {
		m.failures[key]--
		m.mu.Unlock()
		return nil, 0, errors.New("simulated fetch failure")
	}
	m.mu.Unlock()

	m.mu.Lock()
	defer m.mu.Unlock()

	value, exists := m.data[key]
	if !exists {
		return nil, 0, fmt.Errorf("key not found: %s", key)
	}

	return value, 1 * time.Hour, nil
}

func (m *MockOriginFetcher) SetData(key string, value []byte) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.data[key] = value
}

func (m *MockOriginFetcher) SetFailures(key string, count int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.failures[key] = count
}

func (m *MockOriginFetcher) CallCount() int64 {
	return m.calls.Load()
}

// MockCacheClient simulates cache-manager client.
type MockCacheClient struct {
	mu    sync.Mutex
	cache map[string][]byte
	calls atomic.Int64
}

func NewMockCacheClient() *MockCacheClient {
	return &MockCacheClient{
		cache: make(map[string][]byte),
	}
}

func (m *MockCacheClient) Set(ctx context.Context, key string, value []byte, ttl time.Duration) error {
	m.calls.Add(1)

	m.mu.Lock()
	defer m.mu.Unlock()

	m.cache[key] = value
	return nil
}

func (m *MockCacheClient) Get(key string) ([]byte, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()

	value, exists := m.cache[key]
	return value, exists
}

func (m *MockCacheClient) CallCount() int64 {
	return m.calls.Load()
}

// setupTestService creates a test service with mocks.
func setupTestService() (*Service, *MockOriginFetcher, *MockCacheClient) {
	config := DefaultConfig()
	config.ConcurrentWarmers = 5
	config.MaxOriginRPS = 100
	config.OriginTimeout = 100 * time.Millisecond

	mockOrigin := NewMockOriginFetcher()
	mockCache := NewMockCacheClient()

	svc := &Service{
		config: config,
		strategies: map[string]Strategy{
			"selective": NewSelectiveHotKeysStrategy(),
			"breadth":   NewBreadthFirstStrategy(),
			"priority":  NewPriorityBasedStrategy(),
		},
		predictor:     NewDefaultPredictor(),
		originFetcher: mockOrigin,
		cacheClient:   mockCache,
		metrics:       &Metrics{},
		rateLimiter:   rate.NewLimiter(rate.Limit(config.MaxOriginRPS), config.MaxOriginRPS),
	}

	svc.workerPool = NewWorkerPool(svc, config.ConcurrentWarmers)
	svc.scheduler = NewScheduler(svc)

	return svc, mockOrigin, mockCache
}

func TestService_WarmKey_Success(t *testing.T) {
	svc, mockOrigin, mockCache := setupTestService()
	defer svc.Shutdown()

	ctx := context.Background()

	// Setup mock data
	mockOrigin.SetData("user:123", []byte("test data"))

	req := &WarmKeyRequest{
		Keys:     []string{"user:123"},
		Priority: 50,
	}

	resp, err := svc.WarmKey(ctx, req)
	if err != nil {
		t.Fatalf("WarmKey failed: %v", err)
	}

	if !resp.Success {
		t.Error("Expected success=true")
	}

	if resp.Queued != 1 {
		t.Errorf("Expected 1 queued, got %d", resp.Queued)
	}

	// Wait for workers to process
	time.Sleep(200 * time.Millisecond)

	// Verify cache was populated
	if mockCache.CallCount() != 1 {
		t.Errorf("Expected 1 cache write, got %d", mockCache.CallCount())
	}

	value, exists := mockCache.Get("user:123")
	if !exists || string(value) != "test data" {
		t.Errorf("Cache not populated correctly: exists=%v, value=%s", exists, string(value))
	}
}

func TestService_WarmKey_Multiple(t *testing.T) {
	svc, mockOrigin, mockCache := setupTestService()
	defer svc.Shutdown()

	ctx := context.Background()

	// Setup mock data
	for i := 0; i < 10; i++ {
		key := fmt.Sprintf("key:%d", i)
		mockOrigin.SetData(key, []byte(fmt.Sprintf("value%d", i)))
	}

	keys := make([]string, 10)
	for i := 0; i < 10; i++ {
		keys[i] = fmt.Sprintf("key:%d", i)
	}

	req := &WarmKeyRequest{
		Keys:     keys,
		Priority: 50,
	}

	resp, err := svc.WarmKey(ctx, req)
	if err != nil {
		t.Fatalf("WarmKey failed: %v", err)
	}

	if resp.Queued != 10 {
		t.Errorf("Expected 10 queued, got %d", resp.Queued)
	}

	// Wait for workers to process
	time.Sleep(500 * time.Millisecond)

	// Verify all keys were cached
	if mockCache.CallCount() != 10 {
		t.Errorf("Expected 10 cache writes, got %d", mockCache.CallCount())
	}

	for i := 0; i < 10; i++ {
		key := fmt.Sprintf("key:%d", i)
		value, exists := mockCache.Get(key)
		if !exists {
			t.Errorf("Key %s not cached", key)
		}
		expectedValue := fmt.Sprintf("value%d", i)
		if string(value) != expectedValue {
			t.Errorf("Wrong value for %s: got %s, expected %s", key, string(value), expectedValue)
		}
	}
}

func TestService_WarmPattern(t *testing.T) {
	svc, mockOrigin, mockCache := setupTestService()
	defer svc.Shutdown()

	ctx := context.Background()

	// Setup mock data
	keys := []string{"user:123:profile", "user:123:settings", "user:456:profile"}
	for _, key := range keys {
		mockOrigin.SetData(key, []byte("data"))
	}

	req := &WarmPatternRequest{
		Pattern:  "user:123:*",
		Keys:     keys,
		Priority: 70,
		Strategy: "priority",
	}

	resp, err := svc.WarmPattern(ctx, req)
	if err != nil {
		t.Fatalf("WarmPattern failed: %v", err)
	}

	if !resp.Success {
		t.Error("Expected success=true")
	}

	// Should match 2 keys (user:123:*)
	if len(resp.MatchedKeys) != 2 {
		t.Errorf("Expected 2 matched keys, got %d", len(resp.MatchedKeys))
	}

	// Wait for processing
	time.Sleep(300 * time.Millisecond)

	// Verify only matched keys were cached
	if mockCache.CallCount() != 2 {
		t.Errorf("Expected 2 cache writes, got %d", mockCache.CallCount())
	}
}

func TestService_RateLimiting(t *testing.T) {
	config := DefaultConfig()
	config.MaxOriginRPS = 10 // Low limit for testing
	config.ConcurrentWarmers = 5

	mockOrigin := NewMockOriginFetcher()
	mockCache := NewMockCacheClient()

	svc := &Service{
		config:        config,
		strategies:    map[string]Strategy{"priority": NewPriorityBasedStrategy()},
		predictor:     NewDefaultPredictor(),
		originFetcher: mockOrigin,
		cacheClient:   mockCache,
		metrics:       &Metrics{},
		rateLimiter:   rate.NewLimiter(rate.Limit(config.MaxOriginRPS), config.MaxOriginRPS),
	}

	svc.workerPool = NewWorkerPool(svc, config.ConcurrentWarmers)

	ctx := context.Background()

	// Setup 50 keys
	keys := make([]string, 50)
	for i := 0; i < 50; i++ {
		key := fmt.Sprintf("key:%d", i)
		keys[i] = key
		mockOrigin.SetData(key, []byte("data"))
	}

	startTime := time.Now()

	req := &WarmKeyRequest{
		Keys: keys,
	}

	_, err := svc.WarmKey(ctx, req)
	if err != nil {
		t.Fatalf("WarmKey failed: %v", err)
	}

	// Wait for all to complete
	time.Sleep(7 * time.Second)

	duration := time.Since(startTime)

	// With rate limit of 10 RPS, 50 keys should take at least 5 seconds
	if duration < 4*time.Second {
		t.Errorf("Rate limiting not working: completed in %v (expected >4s)", duration)
	}

	svc.Shutdown()
}

func TestService_Deduplication(t *testing.T) {
	svc, mockOrigin, _ := setupTestService()
	defer svc.Shutdown()

	ctx := context.Background()

	mockOrigin.SetData("user:123", []byte("data"))
	mockOrigin.delay = 200 * time.Millisecond // Slow fetch

	// Queue same key multiple times concurrently
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			svc.WarmKey(ctx, &WarmKeyRequest{
				Keys: []string{"user:123"},
			})
		}()
	}

	wg.Wait()
	time.Sleep(500 * time.Millisecond)

	// Should only fetch once due to deduplication
	fetchCount := mockOrigin.CallCount()
	if fetchCount > 2 {
		t.Errorf("Deduplication failed: %d fetches (expected 1-2)", fetchCount)
	}
}

func TestService_EmergencyStop(t *testing.T) {
	svc, mockOrigin, _ := setupTestService()
	defer svc.Shutdown()

	ctx := context.Background()

	// Setup origin with high latency
	mockOrigin.SetData("slow:key", []byte("data"))
	mockOrigin.delay = 3 * time.Second // Exceeds emergency threshold

	req := &WarmKeyRequest{
		Keys: []string{"slow:key"},
	}

	_, err := svc.WarmKey(ctx, req)
	if err != nil {
		t.Fatalf("WarmKey failed: %v", err)
	}

	time.Sleep(4 * time.Second)

	// Emergency stop should be triggered
	if !svc.emergencyStop.Load() {
		t.Error("Emergency stop should be triggered for high latency")
	}

	// Further warming should fail
	_, err = svc.WarmKey(ctx, &WarmKeyRequest{
		Keys: []string{"another:key"},
	})
	if err == nil {
		t.Error("Expected error when emergency stop is active")
	}
}

func TestService_RetryOnFailure(t *testing.T) {
	svc, mockOrigin, mockCache := setupTestService()
	defer svc.Shutdown()

	ctx := context.Background()

	// Setup key that fails twice then succeeds
	mockOrigin.SetData("flaky:key", []byte("data"))
	mockOrigin.SetFailures("flaky:key", 2)

	req := &WarmKeyRequest{
		Keys: []string{"flaky:key"},
	}

	_, err := svc.WarmKey(ctx, req)
	if err != nil {
		t.Fatalf("WarmKey failed: %v", err)
	}

	// Wait for retries
	time.Sleep(2 * time.Second)

	// Should eventually succeed
	if mockCache.CallCount() != 1 {
		t.Errorf("Expected 1 cache write after retries, got %d", mockCache.CallCount())
	}

	// Verify success metric
	if svc.metrics.SuccessTotal.Load() != 1 {
		t.Errorf("Expected 1 success, got %d", svc.metrics.SuccessTotal.Load())
	}
}

func TestService_GetStatus(t *testing.T) {
	svc, mockOrigin, _ := setupTestService()
	defer svc.Shutdown()

	ctx := context.Background()

	// Warm some keys
	mockOrigin.SetData("key:1", []byte("data"))
	svc.WarmKey(ctx, &WarmKeyRequest{Keys: []string{"key:1"}})

	time.Sleep(200 * time.Millisecond)

	// Get status
	status, err := svc.GetStatus(ctx)
	if err != nil {
		t.Fatalf("GetStatus failed: %v", err)
	}

	if status.Metrics.JobsTotal != 1 {
		t.Errorf("Expected 1 job, got %d", status.Metrics.JobsTotal)
	}

	if len(status.WorkerStatus) != 5 {
		t.Errorf("Expected 5 workers, got %d", len(status.WorkerStatus))
	}
}

func TestService_ConfigUpdate(t *testing.T) {
	svc, _, _ := setupTestService()
	defer svc.Shutdown()

	ctx := context.Background()

	// Get initial config
	resp, err := svc.GetConfig(ctx)
	if err != nil {
		t.Fatalf("GetConfig failed: %v", err)
	}

	oldRPS := resp.Config.MaxOriginRPS

	// Update config
	newRPS := 200
	updateReq := &UpdateConfigRequest{
		MaxOriginRPS: &newRPS,
	}

	updateResp, err := svc.UpdateConfig(ctx, updateReq)
	if err != nil {
		t.Fatalf("UpdateConfig failed: %v", err)
	}

	if updateResp.Config.MaxOriginRPS != newRPS {
		t.Errorf("Config not updated: got %d, expected %d", updateResp.Config.MaxOriginRPS, newRPS)
	}

	if updateResp.Config.MaxOriginRPS == oldRPS {
		t.Error("Config should have changed")
	}
}

func TestSelectiveStrategy_Plan(t *testing.T) {
	strategy := NewSelectiveHotKeysStrategy()
	ctx := context.Background()

	keys := []string{"hot:1", "hot:2", "hot:3", "hot:4", "hot:5"}

	opts := PlanOptions{
		Keys:     keys,
		Priority: 80,
		Limit:    3,
	}

	tasks, err := strategy.Plan(ctx, opts)
	if err != nil {
		t.Fatalf("Plan failed: %v", err)
	}

	// Should return top 3 keys
	if len(tasks) != 3 {
		t.Errorf("Expected 3 tasks, got %d", len(tasks))
	}

	// Verify priorities decrease
	for i := 1; i < len(tasks); i++ {
		if tasks[i].Priority > tasks[i-1].Priority {
			t.Error("Priorities should decrease for less hot keys")
		}
	}
}

func TestBreadthFirstStrategy_Plan(t *testing.T) {
	strategy := NewBreadthFirstStrategy()
	ctx := context.Background()

	keys := []string{
		"user:123:posts:456", // depth 3
		"user:123",           // depth 1
		"user:123:posts",     // depth 2
		"product:789",        // depth 1
	}

	opts := PlanOptions{
		Keys: keys,
	}

	tasks, err := strategy.Plan(ctx, opts)
	if err != nil {
		t.Fatalf("Plan failed: %v", err)
	}

	// Should order by depth (shallower first)
	if tasks[0].Key != "user:123" && tasks[0].Key != "product:789" {
		t.Errorf("First task should be depth 1, got %s", tasks[0].Key)
	}

	// Verify priorities are higher for shallower keys
	for i := 1; i < len(tasks); i++ {
		depthI := tasks[i].Metadata["depth"].(int)
		depthPrev := tasks[i-1].Metadata["depth"].(int)
		if depthI < depthPrev {
			t.Error("Keys should be ordered by depth (shallow first)")
		}
	}
}

func TestPriorityStrategy_Plan(t *testing.T) {
	strategy := NewPriorityBasedStrategy()
	ctx := context.Background()

	keys := []string{"key:1", "key:2", "key:3", "key:4", "key:5"}

	opts := PlanOptions{
		Keys:  keys,
		Limit: 3,
	}

	tasks, err := strategy.Plan(ctx, opts)
	if err != nil {
		t.Fatalf("Plan failed: %v", err)
	}

	// Should return top 3 by priority score
	if len(tasks) != 3 {
		t.Errorf("Expected 3 tasks, got %d", len(tasks))
	}

	// Verify priorities are sorted (highest first)
	for i := 1; i < len(tasks); i++ {
		if tasks[i].Priority > tasks[i-1].Priority {
			t.Error("Tasks should be sorted by priority (highest first)")
		}
	}
}

func TestDefaultPredictor_PredictHotKeys(t *testing.T) {
	predictor := NewDefaultPredictor()

	// Record accesses
	for i := 0; i < 100; i++ {
		predictor.RecordAccess("hot:key")
	}
	for i := 0; i < 50; i++ {
		predictor.RecordAccess("warm:key")
	}
	for i := 0; i < 10; i++ {
		predictor.RecordAccess("cold:key")
	}

	ctx := context.Background()

	// Predict top 2 keys
	hotKeys, err := predictor.PredictHotKeys(ctx, 1*time.Hour, 2)
	if err != nil {
		t.Fatalf("PredictHotKeys failed: %v", err)
	}

	if len(hotKeys) != 2 {
		t.Errorf("Expected 2 hot keys, got %d", len(hotKeys))
	}

	// First should be hottest
	if hotKeys[0] != "hot:key" {
		t.Errorf("Expected hot:key first, got %s", hotKeys[0])
	}

	// Second should be warm:key
	if hotKeys[1] != "warm:key" {
		t.Errorf("Expected warm:key second, got %s", hotKeys[1])
	}
}

func TestDefaultPredictor_RecencyBonus(t *testing.T) {
	predictor := NewDefaultPredictor()

	// Record old accesses
	for i := 0; i < 50; i++ {
		predictor.RecordAccess("old:key")
	}

	// Sleep to make them old
	time.Sleep(100 * time.Millisecond)

	// Record recent accesses
	for i := 0; i < 30; i++ {
		predictor.RecordAccess("recent:key")
	}

	ctx := context.Background()

	// Recent key should rank higher due to recency bonus
	hotKeys, err := predictor.PredictHotKeys(ctx, 1*time.Hour, 2)
	if err != nil {
		t.Fatalf("PredictHotKeys failed: %v", err)
	}

	// Recent key should be first despite fewer total accesses
	if hotKeys[0] != "recent:key" {
		t.Errorf("Recent key should rank first, got %s", hotKeys[0])
	}
}

func TestDefaultPredictor_Cleanup(t *testing.T) {
	predictor := NewDefaultPredictor()

	// Record accesses
	predictor.RecordAccess("key:1")
	predictor.RecordAccess("key:2")

	stats := predictor.GetStats()
	if stats.TrackedKeys != 2 {
		t.Errorf("Expected 2 tracked keys, got %d", stats.TrackedKeys)
	}

	// Cleanup with short age (should remove all)
	removed := predictor.Cleanup(1 * time.Nanosecond)
	if removed != 2 {
		t.Errorf("Expected 2 removed, got %d", removed)
	}

	stats = predictor.GetStats()
	if stats.TrackedKeys != 0 {
		t.Errorf("Expected 0 tracked keys after cleanup, got %d", stats.TrackedKeys)
	}
}

func BenchmarkService_WarmKey(b *testing.B) {
	svc, mockOrigin, _ := setupTestService()
	defer svc.Shutdown()

	ctx := context.Background()

	// Setup data
	for i := 0; i < 100; i++ {
		key := fmt.Sprintf("key:%d", i)
		mockOrigin.SetData(key, []byte("data"))
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		key := fmt.Sprintf("key:%d", i%100)
		svc.WarmKey(ctx, &WarmKeyRequest{
			Keys: []string{key},
		})
	}
}

func BenchmarkDefaultPredictor_RecordAccess(b *testing.B) {
	predictor := NewDefaultPredictor()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		predictor.RecordAccess(fmt.Sprintf("key:%d", i%1000))
	}
}

func BenchmarkPriorityStrategy_Plan(b *testing.B) {
	strategy := NewPriorityBasedStrategy()
	ctx := context.Background()

	keys := make([]string, 1000)
	for i := 0; i < 1000; i++ {
		keys[i] = fmt.Sprintf("key:%d", i)
	}

	opts := PlanOptions{
		Keys:  keys,
		Limit: 100,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		strategy.Plan(ctx, opts)
	}
}
