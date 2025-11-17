package invalidation

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"
)

// MockAuditLogger provides a test implementation of audit logging.
type MockAuditLogger struct {
	mu   sync.Mutex
	logs []AuditLog
}

func NewMockAuditLogger() *MockAuditLogger {
	return &MockAuditLogger{
		logs: make([]AuditLog, 0),
	}
}

func (m *MockAuditLogger) Insert(ctx context.Context, log AuditLog) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	
	log.ID = int64(len(m.logs) + 1)
	m.logs = append(m.logs, log)
	return nil
}

func (m *MockAuditLogger) GetRecent(ctx context.Context, limit, offset int, patternFilter string) ([]AuditLog, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Filter by pattern if provided
	filtered := make([]AuditLog, 0)
	for i := len(m.logs) - 1; i >= 0; i-- {
		log := m.logs[i]
		if patternFilter == "" || log.Pattern == patternFilter {
			filtered = append(filtered, log)
		}
	}

	// Apply pagination
	if offset >= len(filtered) {
		return []AuditLog{}, nil
	}

	end := offset + limit
	if end > len(filtered) {
		end = len(filtered)
	}

	return filtered[offset:end], nil
}

func (m *MockAuditLogger) GetCount(ctx context.Context, patternFilter string) (int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if patternFilter == "" {
		return len(m.logs), nil
	}

	count := 0
	for _, log := range m.logs {
		if log.Pattern == patternFilter {
			count++
		}
	}
	return count, nil
}

func (m *MockAuditLogger) GetByRequestID(ctx context.Context, requestID string) ([]AuditLog, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	result := make([]AuditLog, 0)
	for _, log := range m.logs {
		if log.RequestID == requestID {
			result = append(result, log)
		}
	}
	return result, nil
}

// setupTestService creates a test service with mocks.
func setupTestService() *Service {
	return &Service{
		patternMatcher: NewPatternMatcher(),
		auditLogger:    NewMockAuditLogger(),
		metrics:        &Metrics{},
	}
}

func TestPatternMatcher_ExactMatch(t *testing.T) {
	pm := NewPatternMatcher()
	keys := []string{"user:123", "user:456", "product:789"}

	matches := pm.Match("user:123", keys)
	if len(matches) != 1 || matches[0] != "user:123" {
		t.Errorf("Expected exact match for user:123, got %v", matches)
	}
}

func TestPatternMatcher_PrefixWildcard(t *testing.T) {
	pm := NewPatternMatcher()
	keys := []string{
		"user:123:profile",
		"user:123:settings",
		"user:456:profile",
		"product:789",
	}

	matches := pm.Match("user:123:*", keys)
	if len(matches) != 2 {
		t.Errorf("Expected 2 matches, got %d: %v", len(matches), matches)
	}

	// Verify correct keys matched
	expectedMatches := map[string]bool{
		"user:123:profile":  true,
		"user:123:settings": true,
	}

	for _, match := range matches {
		if !expectedMatches[match] {
			t.Errorf("Unexpected match: %s", match)
		}
	}
}

func TestPatternMatcher_SuffixWildcard(t *testing.T) {
	pm := NewPatternMatcher()
	keys := []string{
		"user:profile",
		"admin:profile",
		"product:profile",
		"user:settings",
	}

	matches := pm.Match("*:profile", keys)
	if len(matches) != 3 {
		t.Errorf("Expected 3 matches, got %d: %v", len(matches), matches)
	}
}

func TestPatternMatcher_ContainsWildcard(t *testing.T) {
	pm := NewPatternMatcher()
	keys := []string{
		"user:123:profile",
		"admin:123:settings",
		"product:456:details",
	}

	matches := pm.Match("*:123:*", keys)
	if len(matches) != 2 {
		t.Errorf("Expected 2 matches, got %d: %v", len(matches), matches)
	}
}

func TestPatternMatcher_AllWildcard(t *testing.T) {
	pm := NewPatternMatcher()
	keys := []string{"key1", "key2", "key3"}

	matches := pm.Match("*", keys)
	if len(matches) != 3 {
		t.Errorf("Expected all keys to match, got %d", len(matches))
	}
}

func TestPatternMatcher_RegexPattern(t *testing.T) {
	pm := NewPatternMatcher()
	keys := []string{
		"user:123",
		"user:456",
		"user:abc",
		"product:789",
	}

	// Match numeric user IDs
	matches := pm.Match("^user:[0-9]+$", keys)
	if len(matches) != 2 {
		t.Errorf("Expected 2 numeric matches, got %d: %v", len(matches), matches)
	}
}

func TestPatternMatcher_CacheEfficiency(t *testing.T) {
	pm := NewPatternMatcher()
	keys := []string{"user:123", "user:456"}

	// First call compiles regex
	pm.Match("^user:[0-9]+$", keys)
	
	// Check cache
	if pm.CacheSize() != 1 {
		t.Errorf("Expected 1 cached regex, got %d", pm.CacheSize())
	}

	// Second call uses cached regex
	pm.Match("^user:[0-9]+$", keys)

	// Should still be 1
	if pm.CacheSize() != 1 {
		t.Errorf("Cache should not grow on reuse, got %d", pm.CacheSize())
	}
}

func TestPatternMatcher_ValidatePattern(t *testing.T) {
	pm := NewPatternMatcher()

	tests := []struct {
		pattern string
		valid   bool
	}{
		{"user:*", true},
		{"user:[0-9]+", true},
		{"*:profile", true},
		{"", true}, // Empty is valid (matches nothing)
		{"user:[", false}, // Invalid regex
	}

	for _, tt := range tests {
		err := pm.ValidatePattern(tt.pattern)
		if (err == nil) != tt.valid {
			t.Errorf("Pattern %q: expected valid=%v, got error=%v", tt.pattern, tt.valid, err)
		}
	}
}

func TestService_InvalidateKey(t *testing.T) {
	svc := setupTestService()
	ctx := context.Background()

	req := &InvalidateKeyRequest{
		Keys:        []string{"user:123", "user:456"},
		TriggeredBy: "test",
		RequestID:   "test-req-1",
	}

	resp, err := svc.InvalidateKey(ctx, req)
	if err != nil {
		t.Fatalf("InvalidateKey failed: %v", err)
	}

	if !resp.Success {
		t.Error("Expected success=true")
	}

	if resp.InvalidatedCount != 2 {
		t.Errorf("Expected 2 invalidated, got %d", resp.InvalidatedCount)
	}

	if resp.RequestID != "test-req-1" {
		t.Errorf("Expected request ID test-req-1, got %s", resp.RequestID)
	}

	// Verify metrics
	if svc.metrics.KeyInvalidations.Load() != 1 {
		t.Errorf("Expected 1 key invalidation metric, got %d", svc.metrics.KeyInvalidations.Load())
	}
}

func TestService_InvalidateKey_Deduplication(t *testing.T) {
	svc := setupTestService()
	ctx := context.Background()

	req := &InvalidateKeyRequest{
		Keys:        []string{"user:123", "user:123", "user:456"},
		TriggeredBy: "test",
	}

	resp, err := svc.InvalidateKey(ctx, req)
	if err != nil {
		t.Fatalf("InvalidateKey failed: %v", err)
	}

	// Should deduplicate to 2 unique keys
	if resp.InvalidatedCount != 2 {
		t.Errorf("Expected 2 unique keys after deduplication, got %d", resp.InvalidatedCount)
	}
}

func TestService_InvalidateKey_EmptyKeys(t *testing.T) {
	svc := setupTestService()
	ctx := context.Background()

	req := &InvalidateKeyRequest{
		Keys:        []string{},
		TriggeredBy: "test",
	}

	_, err := svc.InvalidateKey(ctx, req)
	if err == nil {
		t.Error("Expected error for empty keys")
	}
}

func TestService_InvalidatePattern(t *testing.T) {
	svc := setupTestService()
	ctx := context.Background()

	cacheKeys := []string{
		"user:123:profile",
		"user:123:settings",
		"user:456:profile",
		"product:789",
	}

	req := &InvalidatePatternRequest{
		Pattern:     "user:123:*",
		TriggeredBy: "test",
		RequestID:   "test-req-2",
		CacheKeys:   cacheKeys,
	}

	resp, err := svc.InvalidatePattern(ctx, req)
	if err != nil {
		t.Fatalf("InvalidatePattern failed: %v", err)
	}

	if !resp.Success {
		t.Error("Expected success=true")
	}

	if resp.Pattern != "user:123:*" {
		t.Errorf("Expected pattern user:123:*, got %s", resp.Pattern)
	}

	if resp.InvalidatedCount != 2 {
		t.Errorf("Expected 2 matched keys, got %d", resp.InvalidatedCount)
	}

	// Verify metrics
	if svc.metrics.PatternInvalidations.Load() != 1 {
		t.Errorf("Expected 1 pattern invalidation, got %d", svc.metrics.PatternInvalidations.Load())
	}
}

func TestService_InvalidatePattern_EmptyPattern(t *testing.T) {
	svc := setupTestService()
	ctx := context.Background()

	req := &InvalidatePatternRequest{
		Pattern:     "",
		TriggeredBy: "test",
	}

	_, err := svc.InvalidatePattern(ctx, req)
	if err == nil {
		t.Error("Expected error for empty pattern")
	}
}

func TestService_GetMetrics(t *testing.T) {
	svc := setupTestService()
	ctx := context.Background()

	// Perform some invalidations
	svc.InvalidateKey(ctx, &InvalidateKeyRequest{
		Keys:        []string{"key1"},
		TriggeredBy: "test",
	})

	svc.InvalidatePattern(ctx, &InvalidatePatternRequest{
		Pattern:     "user:*",
		TriggeredBy: "test",
	})

	// Get metrics
	metrics, err := svc.GetMetrics(ctx)
	if err != nil {
		t.Fatalf("GetMetrics failed: %v", err)
	}

	if metrics.TotalInvalidations != 2 {
		t.Errorf("Expected 2 total invalidations, got %d", metrics.TotalInvalidations)
	}

	if metrics.KeyInvalidations != 1 {
		t.Errorf("Expected 1 key invalidation, got %d", metrics.KeyInvalidations)
	}

	if metrics.PatternInvalidations != 1 {
		t.Errorf("Expected 1 pattern invalidation, got %d", metrics.PatternInvalidations)
	}

	expectedRatio := 0.5 // 1 pattern out of 2 total
	if metrics.PatternInvalidationRatio != expectedRatio {
		t.Errorf("Expected pattern ratio %.2f, got %.2f", expectedRatio, metrics.PatternInvalidationRatio)
	}
}

func TestMockAuditLogger_Insert(t *testing.T) {
	logger := NewMockAuditLogger()
	ctx := context.Background()

	log := AuditLog{
		Pattern:     "user:*",
		Keys:        []string{"user:123"},
		TriggeredBy: "test",
		Timestamp:   time.Now(),
		RequestID:   "req-1",
	}

	err := logger.Insert(ctx, log)
	if err != nil {
		t.Fatalf("Insert failed: %v", err)
	}

	// Verify insertion
	logs, err := logger.GetRecent(ctx, 10, 0, "")
	if err != nil {
		t.Fatalf("GetRecent failed: %v", err)
	}

	if len(logs) != 1 {
		t.Errorf("Expected 1 log, got %d", len(logs))
	}

	if logs[0].Pattern != "user:*" {
		t.Errorf("Expected pattern user:*, got %s", logs[0].Pattern)
	}
}

func TestMockAuditLogger_GetRecent_Pagination(t *testing.T) {
	logger := NewMockAuditLogger()
	ctx := context.Background()

	// Insert multiple logs
	for i := 0; i < 10; i++ {
		logger.Insert(ctx, AuditLog{
			Pattern:     fmt.Sprintf("key:%d", i),
			Keys:        []string{fmt.Sprintf("key:%d", i)},
			TriggeredBy: "test",
			Timestamp:   time.Now(),
			RequestID:   fmt.Sprintf("req-%d", i),
		})
	}

	// Get first page
	logs, err := logger.GetRecent(ctx, 5, 0, "")
	if err != nil {
		t.Fatalf("GetRecent failed: %v", err)
	}

	if len(logs) != 5 {
		t.Errorf("Expected 5 logs, got %d", len(logs))
	}

	// Get second page
	logs, err = logger.GetRecent(ctx, 5, 5, "")
	if err != nil {
		t.Fatalf("GetRecent failed: %v", err)
	}

	if len(logs) != 5 {
		t.Errorf("Expected 5 logs on second page, got %d", len(logs))
	}
}

func TestMockAuditLogger_GetByRequestID(t *testing.T) {
	logger := NewMockAuditLogger()
	ctx := context.Background()

	// Insert logs with different request IDs
	logger.Insert(ctx, AuditLog{
		Pattern:     "user:*",
		RequestID:   "req-1",
		TriggeredBy: "test",
		Timestamp:   time.Now(),
	})

	logger.Insert(ctx, AuditLog{
		Pattern:     "product:*",
		RequestID:   "req-2",
		TriggeredBy: "test",
		Timestamp:   time.Now(),
	})

	logger.Insert(ctx, AuditLog{
		Pattern:     "order:*",
		RequestID:   "req-1",
		TriggeredBy: "test",
		Timestamp:   time.Now(),
	})

	// Query by request ID
	logs, err := logger.GetByRequestID(ctx, "req-1")
	if err != nil {
		t.Fatalf("GetByRequestID failed: %v", err)
	}

	if len(logs) != 2 {
		t.Errorf("Expected 2 logs for req-1, got %d", len(logs))
	}

	for _, log := range logs {
		if log.RequestID != "req-1" {
			t.Errorf("Expected request ID req-1, got %s", log.RequestID)
		}
	}
}

func TestConcurrentInvalidations(t *testing.T) {
	svc := setupTestService()
	ctx := context.Background()

	var wg sync.WaitGroup
	concurrency := 100

	// Concurrent key invalidations
	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			req := &InvalidateKeyRequest{
				Keys:        []string{fmt.Sprintf("key:%d", i)},
				TriggeredBy: "concurrent-test",
			}
			_, _ = svc.InvalidateKey(ctx, req)
		}(i)
	}

	wg.Wait()

	// Verify metrics
	totalInvalidations := svc.metrics.TotalInvalidations.Load()
	if totalInvalidations != int64(concurrency) {
		t.Errorf("Expected %d invalidations, got %d", concurrency, totalInvalidations)
	}
}

func TestIsWildcard(t *testing.T) {
	tests := []struct {
		pattern  string
		expected bool
	}{
		{"user:*", true},
		{"*:profile", true},
		{"*", true},
		{"user:123", false},
		{"", false},
	}

	for _, tt := range tests {
		result := IsWildcard(tt.pattern)
		if result != tt.expected {
			t.Errorf("IsWildcard(%q) = %v, expected %v", tt.pattern, result, tt.expected)
		}
	}
}

func TestIsRegex(t *testing.T) {
	tests := []struct {
		pattern  string
		expected bool
	}{
		{"user:[0-9]+", true},
		{"user:(123|456)", true},
		{"^user:.*$", true},
		{"user:*", false},
		{"user:123", false},
	}

	for _, tt := range tests {
		result := IsRegex(tt.pattern)
		if result != tt.expected {
			t.Errorf("IsRegex(%q) = %v, expected %v", tt.pattern, result, tt.expected)
		}
	}
}

func BenchmarkPatternMatcher_PrefixWildcard(b *testing.B) {
	pm := NewPatternMatcher()
	
	// Generate test keys
	keys := make([]string, 10000)
	for i := 0; i < 10000; i++ {
		keys[i] = fmt.Sprintf("user:%d:profile", i)
	}

	pattern := "user:123:*"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		pm.Match(pattern, keys)
	}
}

func BenchmarkPatternMatcher_RegexCached(b *testing.B) {
	pm := NewPatternMatcher()
	
	keys := make([]string, 1000)
	for i := 0; i < 1000; i++ {
		keys[i] = fmt.Sprintf("user:%d", i)
	}

	pattern := "^user:[0-9]+$"

	// Prime the cache
	pm.Match(pattern, keys)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		pm.Match(pattern, keys)
	}
}

func BenchmarkService_InvalidateKey(b *testing.B) {
	svc := setupTestService()
	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		req := &InvalidateKeyRequest{
			Keys:        []string{fmt.Sprintf("key:%d", i)},
			TriggeredBy: "benchmark",
		}
		svc.InvalidateKey(ctx, req)
	}
}