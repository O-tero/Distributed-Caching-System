// Package models provides canonical data models used across the distributed caching system.
//
// Design Philosophy:
// - Minimal allocations on hot paths
// - Thread-safe operations using atomic primitives
// - Clean serialization via encoding helpers
// - Explicit expiry semantics
package models

import (
	"sync/atomic"
	"time"
)

// DefaultTTL is the default time-to-live for cache entries.
const DefaultTTL = 1 * time.Hour

// Entry represents a cache entry with metadata and access tracking.
//
// Thread Safety: AccessCount uses atomic operations. Other fields should be
// protected by the caller if concurrent modification is needed.
//
// Memory Layout: Optimized with hot fields (Key, Value) at the start for
// better cache locality.
type Entry struct {
	// Hot fields (frequently accessed)
	Key   string // Cache key
	Value []byte // Serialized value (allows any type after deserialization)

	// Temporal fields
	CreatedAt  time.Time     // When entry was created
	LastAccess time.Time     // Last access timestamp
	TTL        time.Duration // Time-to-live

	// Access tracking (atomic)
	AccessCount uint64 // Number of accesses (use atomic operations)

	// Metadata
	Metadata map[string]string // Optional key-value metadata
}

// NewEntry creates a new cache entry with default TTL.
func NewEntry(key string, value []byte) *Entry {
	now := time.Now()
	return &Entry{
		Key:         key,
		Value:       value,
		CreatedAt:   now,
		LastAccess:  now,
		TTL:         DefaultTTL,
		AccessCount: 0,
		Metadata:    make(map[string]string),
	}
}

// NewEntryWithTTL creates a new cache entry with custom TTL.
func NewEntryWithTTL(key string, value []byte, ttl time.Duration) *Entry {
	now := time.Now()
	return &Entry{
		Key:         key,
		Value:       value,
		CreatedAt:   now,
		LastAccess:  now,
		TTL:         ttl,
		AccessCount: 0,
		Metadata:    make(map[string]string),
	}
}

// IsExpired checks if the entry has expired based on TTL.
// Complexity: O(1)
//
// Example:
//   entry := NewEntry("key", []byte("value"))
//   time.Sleep(2 * time.Hour)
//   if entry.IsExpired(time.Now()) {
//       // Entry has expired, evict it
//   }
func (e *Entry) IsExpired(now time.Time) bool {
	if e.TTL == 0 {
		return false // No expiration
	}
	return now.After(e.CreatedAt.Add(e.TTL))
}

// ExpiresAt returns the absolute expiration time.
func (e *Entry) ExpiresAt() time.Time {
	if e.TTL == 0 {
		return time.Time{} // Never expires
	}
	return e.CreatedAt.Add(e.TTL)
}

// TimeUntilExpiry returns the duration until expiry, or 0 if already expired.
func (e *Entry) TimeUntilExpiry(now time.Time) time.Duration {
	if e.TTL == 0 {
		return time.Duration(1<<63 - 1) // Max duration (never expires)
	}

	expiry := e.CreatedAt.Add(e.TTL)
	remaining := expiry.Sub(now)

	if remaining < 0 {
		return 0
	}
	return remaining
}

// Touch updates the last access time and increments the access counter.
// Thread-safe: Uses atomic operations for AccessCount.
// Complexity: O(1)
//
// Example:
//   entry.Touch() // Record access for LRU/LFU algorithms
func (e *Entry) Touch() {
	e.LastAccess = time.Now()
	atomic.AddUint64(&e.AccessCount, 1)
}

// GetAccessCount returns the current access count (thread-safe).
func (e *Entry) GetAccessCount() uint64 {
	return atomic.LoadUint64(&e.AccessCount)
}

// ResetAccessCount resets the access counter to zero.
func (e *Entry) ResetAccessCount() {
	atomic.StoreUint64(&e.AccessCount, 0)
}

// Size returns the approximate memory size of the entry in bytes.
// Useful for memory-based eviction policies.
func (e *Entry) Size() int {
	size := len(e.Key) + len(e.Value)
	
	// Add metadata overhead
	for k, v := range e.Metadata {
		size += len(k) + len(v)
	}
	
	// Add struct overhead (approximate)
	size += 64 // timestamps, counters, pointers
	
	return size
}

// Clone creates a shallow copy of the entry.
// Value slice is copied, but metadata map is shallow copied.
func (e *Entry) Clone() *Entry {
	metadata := make(map[string]string, len(e.Metadata))
	for k, v := range e.Metadata {
		metadata[k] = v
	}

	value := make([]byte, len(e.Value))
	copy(value, e.Value)

	return &Entry{
		Key:         e.Key,
		Value:       value,
		CreatedAt:   e.CreatedAt,
		LastAccess:  e.LastAccess,
		TTL:         e.TTL,
		AccessCount: atomic.LoadUint64(&e.AccessCount),
		Metadata:    metadata,
	}
}

// SetMetadata sets a metadata key-value pair.
func (e *Entry) SetMetadata(key, value string) {
	if e.Metadata == nil {
		e.Metadata = make(map[string]string)
	}
	e.Metadata[key] = value
}

// GetMetadata retrieves a metadata value by key.
func (e *Entry) GetMetadata(key string) (string, bool) {
	if e.Metadata == nil {
		return "", false
	}
	val, ok := e.Metadata[key]
	return val, ok
}

// EntryStats provides statistics about a cache entry.
type EntryStats struct {
	Key              string
	Size             int
	Age              time.Duration
	TTL              time.Duration
	AccessCount      uint64
	TimeSinceAccess  time.Duration
	AccessFrequency  float64 // Accesses per second
}

// Stats returns statistics about the entry.
func (e *Entry) Stats(now time.Time) EntryStats {
	age := now.Sub(e.CreatedAt)
	timeSinceAccess := now.Sub(e.LastAccess)
	accessCount := e.GetAccessCount()

	frequency := 0.0
	if age.Seconds() > 0 {
		frequency = float64(accessCount) / age.Seconds()
	}

	return EntryStats{
		Key:             e.Key,
		Size:            e.Size(),
		Age:             age,
		TTL:             e.TTL,
		AccessCount:     accessCount,
		TimeSinceAccess: timeSinceAccess,
		AccessFrequency: frequency,
	}
}