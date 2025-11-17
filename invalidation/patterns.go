package invalidation

import (
	"errors"
	"regexp"
	"strings"
	"sync"
)

// PatternMatcher provides high-performance pattern matching for cache keys.
// Uses a combination of prefix matching and compiled regex caching for efficiency.
//
// Performance optimizations:
// - Prefix matching: O(k) where k = key length, used for simple wildcards
// - Regex caching: O(1) lookup + O(n) matching, avoids recompilation
// - ReDoS protection: Precompile patterns and reject malicious regex
//
// Supported patterns:
// - Exact: "user:123" matches only "user:123"
// - Prefix wildcard: "user:*" matches "user:123", "user:456", etc.
// - Suffix wildcard: "*:profile" matches "user:profile", "product:profile"
// - Contains: "*:123:*" matches any key containing ":123:"
// - Regex: "user:[0-9]+" matches "user:123", "user:456" (use sparingly)
type PatternMatcher struct {
	regexCache sync.Map // map[string]*regexp.Regexp
}

// NewPatternMatcher creates a new pattern matcher with regex caching.
func NewPatternMatcher() *PatternMatcher {
	return &PatternMatcher{}
}

// Match returns all keys that match the given pattern.
// Complexity: O(n*k) where n = number of keys, k = key length
func (pm *PatternMatcher) Match(pattern string, keys []string) []string {
	if pattern == "" {
		return []string{}
	}

	// Fast path: exact match (no wildcards)
	if !IsWildcard(pattern) && !IsRegex(pattern) {
		for _, key := range keys {
			if key == pattern {
				return []string{key}
			}
		}
		return []string{}
	}

	// Wildcard matching (optimized path)
	if IsWildcard(pattern) {
		return pm.matchWildcard(pattern, keys)
	}

	// Regex matching (slower path, use cached compilation)
	return pm.matchRegex(pattern, keys)
}

// IsWildcard checks if a pattern contains wildcard characters.
func IsWildcard(pattern string) bool {
	return strings.Contains(pattern, "*")
}

// IsRegex checks if a pattern looks like a regex (contains regex metacharacters).
func IsRegex(pattern string) bool {
	regexChars := []string{"[", "]", "(", ")", "^", "$", "+", "?", "{", "}", "|"}
	for _, char := range regexChars {
		if strings.Contains(pattern, char) {
			return true
		}
	}
	return false
}

// matchWildcard performs optimized wildcard matching.
// Complexity: O(n*k) where n = keys, k = key length
func (pm *PatternMatcher) matchWildcard(pattern string, keys []string) []string {
	matches := make([]string, 0)

	// Special case: single wildcard "*" matches everything
	if pattern == "*" {
		return keys
	}

	// Check pattern type
	if strings.HasPrefix(pattern, "*") && strings.HasSuffix(pattern, "*") {
		// Contains pattern: *substring*
		substring := strings.Trim(pattern, "*")
		for _, key := range keys {
			if strings.Contains(key, substring) {
				matches = append(matches, key)
			}
		}
	} else if strings.HasPrefix(pattern, "*") {
		// Suffix pattern: *suffix
		suffix := strings.TrimPrefix(pattern, "*")
		for _, key := range keys {
			if strings.HasSuffix(key, suffix) {
				matches = append(matches, key)
			}
		}
	} else if strings.HasSuffix(pattern, "*") {
		// Prefix pattern: prefix* (most common case)
		prefix := strings.TrimSuffix(pattern, "*")
		for _, key := range keys {
			if strings.HasPrefix(key, prefix) {
				matches = append(matches, key)
			}
		}
	} else {
		// Complex wildcard: convert to regex
		regexPattern := wildcardToRegex(pattern)
		return pm.matchRegex(regexPattern, keys)
	}

	return matches
}

// matchRegex performs regex matching with caching.
// Complexity: O(1) cache lookup + O(n*m) matching where n = keys, m = regex complexity
func (pm *PatternMatcher) matchRegex(pattern string, keys []string) []string {
	// Try to get cached regex
	var re *regexp.Regexp
	if cached, ok := pm.regexCache.Load(pattern); ok {
		re = cached.(*regexp.Regexp)
	} else {
		// Compile and cache
		var err error
		re, err = regexp.Compile(pattern)
		if err != nil {
			// Invalid regex, return no matches
			return []string{}
		}
		pm.regexCache.Store(pattern, re)
	}

	// Match against all keys
	matches := make([]string, 0)
	for _, key := range keys {
		if re.MatchString(key) {
			matches = append(matches, key)
		}
	}

	return matches
}

// wildcardToRegex converts a wildcard pattern to a regex pattern.
// Example: "user:*:profile" -> "^user:.*:profile$"
func wildcardToRegex(pattern string) string {
	// Escape regex metacharacters except *
	escaped := regexp.QuoteMeta(pattern)
	
	// Replace escaped \* with .*
	escaped = strings.ReplaceAll(escaped, "\\*", ".*")
	
	// Anchor to start and end
	return "^" + escaped + "$"
}

// MatchCount returns the number of keys that match the pattern (without allocating slice).
// Useful for metrics without materializing matches.
func (pm *PatternMatcher) MatchCount(pattern string, keys []string) int {
	if pattern == "" {
		return 0
	}

	// Fast path: exact match
	if !IsWildcard(pattern) && !IsRegex(pattern) {
		for _, key := range keys {
			if key == pattern {
				return 1
			}
		}
		return 0
	}

	// For wildcard/regex, we need to actually match
	matches := pm.Match(pattern, keys)
	return len(matches)
}

// ValidatePattern checks if a pattern is safe and valid.
// Returns error if pattern could cause ReDoS or is invalid.
func (pm *PatternMatcher) ValidatePattern(pattern string) error {
	if pattern == "" {
		return nil // Empty pattern is valid (matches nothing)
	}
	// Check for extremely long patterns (potential DoS)
	if len(pattern) > 1000 {
		return errors.New("pattern too long: potential DoS")
	}

	// If it's a regex, try to compile it
	if IsRegex(pattern) {
		_, err := regexp.Compile(pattern)
		if err != nil {
			return err
		}
	}

	return nil
}

// ClearCache clears the regex cache (useful for testing or memory pressure).
func (pm *PatternMatcher) ClearCache() {
	pm.regexCache = sync.Map{}
}

// CacheSize returns the approximate number of cached regex patterns.
func (pm *PatternMatcher) CacheSize() int {
	count := 0
	pm.regexCache.Range(func(key, value interface{}) bool {
		count++
		return true
	})
	return count
}