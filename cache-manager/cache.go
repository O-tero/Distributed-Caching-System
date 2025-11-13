package cachemanager

import (
	"container/list"
	"strings"
	"sync"
	"time"
)

type CacheEntry struct {
	Value     interface{} `json:"value"`
	CachedAt  time.Time   `json:"cached_at"`
	ExpiresAt time.Time   `json:"expires_at"`
	Source    string      `json:"source"` // "l1", "l2", "origin"
}

type lruEntry struct {
	key       string
	value     interface{}
	expiresAt time.Time
	element   *list.Element // pointer to list element for O(1) removal
}

// L1Cache implements a thread-safe in-memory cache with LRU eviction and TTL expiration.
// Trade-offs:
// - RWMutex chosen over sync.Map for better control over eviction and TTL.
// - sync.Map lacks ordered iteration needed for LRU, and atomic eviction is complex.
// - Global lock on write is acceptable for <100K ops/sec; shard for higher loads.
type L1Cache struct {
	mu         sync.RWMutex
	cache      map[string]*lruEntry
	lruList    *list.List
	maxEntries int
}

// NewL1Cache creates a new L1 cache with specified capacity.
func NewL1Cache(maxEntries int) *L1Cache {
	return &L1Cache{
		cache:      make(map[string]*lruEntry, maxEntries),
		lruList:    list.New(),
		maxEntries: maxEntries,
	}
}

// Get retrieves a value from L1 cache and updates LRU ordering.
// Returns (entry, true) if found and not expired, (nil, false) otherwise.
// Complexity: O(1) average.
func (c *L1Cache) Get(key string) (*CacheEntry, bool) {
	c.mu.RLock()
	entry, exists := c.cache[key]
	c.mu.RUnlock()

	if !exists {
		return nil, false
	}

	// Check expiration (lazy)
	if time.Now().After(entry.expiresAt) {
		c.mu.Lock()
		c.deleteUnsafe(key)
		c.mu.Unlock()
		return nil, false
	}

	c.mu.Lock()
	c.lruList.MoveToFront(entry.element)
	c.mu.Unlock()

	return &CacheEntry{
		Value:     entry.value,
		CachedAt:  entry.expiresAt.Add(-1 * time.Hour), // approximate
		ExpiresAt: entry.expiresAt,
		Source:    "l1",
	}, true
}

// Set stores a value in L1 cache with TTL, evicting LRU entry if at capacity.
// Complexity: O(1).
func (c *L1Cache) Set(key string, value interface{}, ttl time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()

	expiresAt := time.Now().Add(ttl)

	if entry, exists := c.cache[key]; exists {
		entry.value = value
		entry.expiresAt = expiresAt
		c.lruList.MoveToFront(entry.element)
		return
	}

	if c.lruList.Len() >= c.maxEntries {
		c.evictLRUUnsafe()
	}

	entry := &lruEntry{
		key:       key,
		value:     value,
		expiresAt: expiresAt,
	}
	entry.element = c.lruList.PushFront(entry)
	c.cache[key] = entry
}

// Delete removes a key from L1 cache.
// Returns true if key existed, false otherwise.
func (c *L1Cache) Delete(key string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.deleteUnsafe(key)
}

// deleteUnsafe is the non-locking internal delete implementation.
func (c *L1Cache) deleteUnsafe(key string) bool {
	entry, exists := c.cache[key]
	if !exists {
		return false
	}

	c.lruList.Remove(entry.element)
	delete(c.cache, key)
	return true
}

// DeletePattern removes all keys matching a pattern (e.g., "user:*").
// Returns number of keys deleted.
func (c *L1Cache) DeletePattern(pattern string) int {
	c.mu.Lock()
	defer c.mu.Unlock()

	count := 0
	prefix := strings.TrimSuffix(pattern, "*")

	// Collect matching keys first to avoid modification during iteration
	var toDelete []string
	for key := range c.cache {
		if matchesPattern(key, pattern, prefix) {
			toDelete = append(toDelete, key)
		}
	}

	for _, key := range toDelete {
		if c.deleteUnsafe(key) {
			count++
		}
	}

	return count
}

// matchesPattern checks if a key matches a pattern with wildcard support.
func matchesPattern(key, pattern, prefix string) bool {
	if strings.HasSuffix(pattern, "*") {
		return strings.HasPrefix(key, prefix)
	}
	return key == pattern
}

// CleanupExpired removes all expired entries.
// Returns number of entries removed.
func (c *L1Cache) CleanupExpired() int {
	c.mu.Lock()
	defer c.mu.Unlock()

	now := time.Now()
	count := 0

	var expired []string
	for key, entry := range c.cache {
		if now.After(entry.expiresAt) {
			expired = append(expired, key)
		}
	}

	for _, key := range expired {
		if c.deleteUnsafe(key) {
			count++
		}
	}

	return count
}

// evictLRUUnsafe removes the least recently used entry.
// Must be called with write lock held.
func (c *L1Cache) evictLRUUnsafe() {
	if c.lruList.Len() == 0 {
		return
	}

	oldest := c.lruList.Back()
	if oldest != nil {
		entry := oldest.Value.(*lruEntry)
		c.lruList.Remove(oldest)
		delete(c.cache, entry.key)
	}
}

// Size returns the current number of entries in L1 cache.
func (c *L1Cache) Size() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.cache)
}

// Clear removes all entries from the cache.
func (c *L1Cache) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.cache = make(map[string]*lruEntry, c.maxEntries)
	c.lruList = list.New()
}