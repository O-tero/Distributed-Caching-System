// Package utils provides pattern matching utilities for cache key filtering.
//
// This file implements efficient pattern matching with support for:
//   - Exact match: "user:123" matches only "user:123"
//   - Prefix match: "users:*" matches "users:123", "users:abc", etc.
//   - Simple wildcard: "user:*:profile" matches "user:123:profile"
//   - Regex fallback: Complex patterns compile to regex with caching
//
// Design Notes:
//   - Prefix matching is O(1) per key (fast path)
//   - Regex patterns are compiled once and cached in sync.Map
//   - Bounded regex cache with LRU eviction recommended for production
//   - Thread-safe via sync.Map for regex cache
//
// Trade-offs:
//   - Prefix match: O(n) for scanning keys but O(1) per check
//   - Regex compile: One-time cost O(k) where k = pattern length
//   - Regex match: O(m) where m = key length
//   - Memory: Unbounded regex cache (recommend TTL eviction in production)
//
// Production extensions:
//   - Implement LRU cache for compiled regexes with max size
//   - Add TTL-based eviction for rarely-used patterns
//   - Consider bloom filters for negative matches on large keysets
package utils

import (
	"fmt"
	"regexp"
	"strings"
	"sync"
)

// regexCache caches compiled regular expressions to avoid recompilation.
// Key: pattern string, Value: *regexp.Regexp
// Thread-safe via sync.Map.
//
// PRODUCTION NOTE: This cache is unbounded. For production use, implement:
//   - LRU eviction with max size (e.g., 1000 patterns)
//   - TTL-based cleanup for unused patterns
//   - Metrics on cache hit rate
var regexCache sync.Map

// MatchPattern checks if a key matches the given pattern.
//
// Pattern syntax:
//   - Exact: "user:123" matches only "user:123"
//   - Prefix: "users:*" matches any key starting with "users:"
//   - Wildcard: "*" matches any substring (simplified glob)
//   - Regex: Complex patterns fallback to regex (e.g., "user:[0-9]+")
//
// Returns:
//   - match: true if key matches pattern
//   - error: if pattern is invalid regex
//
// Performance:
//   - Exact match: O(1)
//   - Prefix match: O(n) where n = len(prefix)
//   - Regex match: O(m) where m = len(key), one-time compile cost
func MatchPattern(pattern, key string) (bool, error) {
	if pattern == "" {
		return false, fmt.Errorf("pattern cannot be empty")
	}

	// Fast path: exact match
	if pattern == key {
		return true, nil
	}

	// Fast path: prefix match (most common case for cache invalidation)
	// Pattern "users:*" matches any key starting with "users:"
	if strings.HasSuffix(pattern, "*") && !strings.Contains(pattern[:len(pattern)-1], "*") {
		prefix := pattern[:len(pattern)-1]
		return strings.HasPrefix(key, prefix), nil
	}

	// Fast path: single wildcard match-all
	if pattern == "*" {
		return true, nil
	}

	// Regex fallback for complex patterns
	// Convert simple glob patterns to regex if needed
	regexPattern := pattern
	if strings.Contains(pattern, "*") || strings.Contains(pattern, "?") {
		regexPattern = globToRegex(pattern)
	}

	// Check cache for compiled regex
	cached, ok := regexCache.Load(regexPattern)
	var re *regexp.Regexp
	if ok {
		re = cached.(*regexp.Regexp)
	} else {
		// Compile and cache regex
		var err error
		re, err = regexp.Compile("^" + regexPattern + "$")
		if err != nil {
			return false, fmt.Errorf("invalid pattern regex: %w", err)
		}
		regexCache.Store(regexPattern, re)
	}

	return re.MatchString(key), nil
}

// FilterKeys returns all keys matching the given pattern.
//
// This is optimized for prefix patterns (O(n) scan with O(1) checks).
// For regex patterns, still scans all keys but checks are O(m) per key.
//
// Performance:
//   - Prefix: O(n) where n = len(keys)
//   - Regex: O(n * m) where n = len(keys), m = avg key length
func FilterKeys(pattern string, keys []string) ([]string, error) {
	if pattern == "" {
		return nil, fmt.Errorf("pattern cannot be empty")
	}

	// Fast path: match all
	if pattern == "*" {
		result := make([]string, len(keys))
		copy(result, keys)
		return result, nil
	}

	// Fast path: prefix match
	if strings.HasSuffix(pattern, "*") && !strings.Contains(pattern[:len(pattern)-1], "*") {
		prefix := pattern[:len(pattern)-1]
		result := make([]string, 0, len(keys)/10) // Estimate 10% match

		for _, key := range keys {
			if strings.HasPrefix(key, prefix) {
				result = append(result, key)
			}
		}
		return result, nil
	}

	// Regex fallback
	result := make([]string, 0, len(keys)/10)
	for _, key := range keys {
		match, err := MatchPattern(pattern, key)
		if err != nil {
			return nil, err
		}
		if match {
			result = append(result, key)
		}
	}

	return result, nil
}

// PrefixMatch is a specialized fast prefix matcher.
// Returns true if key starts with prefix.
// O(n) where n = len(prefix).
func PrefixMatch(prefix, key string) bool {
	return strings.HasPrefix(key, prefix)
}

// globToRegex converts a simple glob pattern to regex.
// Supports:
//   - * = match any characters (.*) 
//   - ? = match single character (.)
//   - Other chars = literal match (escaped)
//
// Example: "user:*:profile" -> "user:.*:profile"
func globToRegex(pattern string) string {
	var result strings.Builder
	result.Grow(len(pattern) * 2) // Estimate

	for i := 0; i < len(pattern); i++ {
		ch := pattern[i]
		switch ch {
		case '*':
			result.WriteString(".*")
		case '?':
			result.WriteString(".")
		case '.', '+', '(', ')', '|', '[', ']', '{', '}', '^', '$', '\\':
			// Escape regex special chars
			result.WriteByte('\\')
			result.WriteByte(ch)
		default:
			result.WriteByte(ch)
		}
	}

	return result.String()
}

// ClearRegexCache clears the compiled regex cache.
// Useful for testing and memory management.
func ClearRegexCache() {
	regexCache.Range(func(key, value interface{}) bool {
		regexCache.Delete(key)
		return true
	})
}

// RegexCacheSize returns the number of cached compiled regexes.
// Useful for monitoring and debugging.
func RegexCacheSize() int {
	count := 0
	regexCache.Range(func(key, value interface{}) bool {
		count++
		return true
	})
	return count
}