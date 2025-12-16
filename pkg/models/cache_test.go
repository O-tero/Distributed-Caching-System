package models

import (
	"testing"
	"time"
)

func TestNewEntry(t *testing.T) {
	entry := NewEntry("test:key", []byte("test value"))

	if entry.Key != "test:key" {
		t.Errorf("Expected key 'test:key', got '%s'", entry.Key)
	}

	if string(entry.Value) != "test value" {
		t.Errorf("Expected value 'test value', got '%s'", string(entry.Value))
	}

	if entry.TTL != DefaultTTL {
		t.Errorf("Expected TTL %v, got %v", DefaultTTL, entry.TTL)
	}

	if entry.GetAccessCount() != 0 {
		t.Errorf("Expected access count 0, got %d", entry.GetAccessCount())
	}
}

func TestEntry_IsExpired(t *testing.T) {
	tests := []struct {
		name     string
		ttl      time.Duration
		age      time.Duration
		expected bool
	}{
		{
			name:     "not expired",
			ttl:      1 * time.Hour,
			age:      30 * time.Minute,
			expected: false,
		},
		{
			name:     "expired",
			ttl:      1 * time.Hour,
			age:      2 * time.Hour,
			expected: true,
		},
		{
			name:     "exactly at expiry",
			ttl:      1 * time.Hour,
			age:      1 * time.Hour,
			expected: false,
		},
		{
			name:     "zero TTL never expires",
			ttl:      0,
			age:      100 * time.Hour,
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			entry := NewEntryWithTTL("key", []byte("value"), tt.ttl)
			entry.CreatedAt = time.Now().Add(-tt.age)

			if got := entry.IsExpired(time.Now()); got != tt.expected {
				t.Errorf("IsExpired() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestEntry_Touch(t *testing.T) {
	entry := NewEntry("key", []byte("value"))

	initialAccess := entry.LastAccess
	initialCount := entry.GetAccessCount()

	// Small delay to ensure time difference
	time.Sleep(10 * time.Millisecond)

	entry.Touch()

	if !entry.LastAccess.After(initialAccess) {
		t.Error("LastAccess should be updated")
	}

	if entry.GetAccessCount() != initialCount+1 {
		t.Errorf("AccessCount should be %d, got %d", initialCount+1, entry.GetAccessCount())
	}

	// Touch multiple times
	for i := 0; i < 10; i++ {
		entry.Touch()
	}

	if entry.GetAccessCount() != initialCount+11 {
		t.Errorf("AccessCount should be %d, got %d", initialCount+11, entry.GetAccessCount())
	}
}

func TestEntry_Touch_Concurrent(t *testing.T) {
	entry := NewEntry("key", []byte("value"))

	const goroutines = 100
	const touchesPerGoroutine = 100

	done := make(chan bool, goroutines)

	for i := 0; i < goroutines; i++ {
		go func() {
			for j := 0; j < touchesPerGoroutine; j++ {
				entry.Touch()
			}
			done <- true
		}()
	}

	// Wait for all goroutines
	for i := 0; i < goroutines; i++ {
		<-done
	}

	expected := uint64(goroutines * touchesPerGoroutine)
	if entry.GetAccessCount() != expected {
		t.Errorf("Expected access count %d, got %d", expected, entry.GetAccessCount())
	}
}

func TestEntry_TimeUntilExpiry(t *testing.T) {
	entry := NewEntryWithTTL("key", []byte("value"), 1*time.Hour)
	now := time.Now()

	remaining := entry.TimeUntilExpiry(now)

	// Should be approximately 1 hour
	if remaining < 59*time.Minute || remaining > 61*time.Minute {
		t.Errorf("Expected remaining time around 1 hour, got %v", remaining)
	}

	// After expiry
	future := now.Add(2 * time.Hour)
	remaining = entry.TimeUntilExpiry(future)

	if remaining != 0 {
		t.Errorf("Expected 0 remaining time after expiry, got %v", remaining)
	}
}

func TestEntry_Size(t *testing.T) {
	entry := NewEntry("short", []byte("val"))
	size1 := entry.Size()

	if size1 <= 0 {
		t.Error("Size should be positive")
	}

	// Add metadata
	entry.SetMetadata("tag", "production")
	size2 := entry.Size()

	if size2 <= size1 {
		t.Error("Size should increase after adding metadata")
	}
}

func TestEntry_Clone(t *testing.T) {
	original := NewEntry("key", []byte("value"))
	original.Touch()
	original.SetMetadata("env", "prod")

	clone := original.Clone()

	// Verify clone has same values
	if clone.Key != original.Key {
		t.Error("Cloned key mismatch")
	}

	if string(clone.Value) != string(original.Value) {
		t.Error("Cloned value mismatch")
	}

	if clone.GetAccessCount() != original.GetAccessCount() {
		t.Error("Cloned access count mismatch")
	}

	// Verify independence
	clone.Value[0] = 'X'
	if original.Value[0] == 'X' {
		t.Error("Clone should have independent value slice")
	}

	clone.SetMetadata("env", "dev")
	if val, _ := original.GetMetadata("env"); val != "prod" {
		t.Error("Clone should have independent metadata")
	}
}

func TestEntry_Stats(t *testing.T) {
	entry := NewEntryWithTTL("key", []byte("value"), 1*time.Hour)
	
	// Simulate some accesses
	for i := 0; i < 10; i++ {
		entry.Touch()
		time.Sleep(1 * time.Millisecond)
	}

	stats := entry.Stats(time.Now())

	if stats.Key != "key" {
		t.Errorf("Expected key 'key', got '%s'", stats.Key)
	}

	if stats.AccessCount != 10 {
		t.Errorf("Expected 10 accesses, got %d", stats.AccessCount)
	}

	if stats.Size <= 0 {
		t.Error("Stats size should be positive")
	}

	if stats.AccessFrequency <= 0 {
		t.Error("Access frequency should be positive")
	}
}

func BenchmarkEntry_Touch(b *testing.B) {
	entry := NewEntry("key", []byte("value"))

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		entry.Touch()
	}
}

func BenchmarkEntry_Touch_Parallel(b *testing.B) {
	entry := NewEntry("key", []byte("value"))

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			entry.Touch()
		}
	})
}

func BenchmarkEntry_IsExpired(b *testing.B) {
	entry := NewEntry("key", []byte("value"))
	now := time.Now()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = entry.IsExpired(now)
	}
}