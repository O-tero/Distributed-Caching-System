package utils

import (
	"fmt"
	"testing"
)

func TestMatchPattern(t *testing.T) {
	tests := []struct {
		name    string
		pattern string
		key     string
		want    bool
		wantErr bool
	}{
		// Exact matches
		{"exact match", "user:123", "user:123", true, false},
		{"exact no match", "user:123", "user:456", false, false},

		// Prefix matches
		{"prefix match", "users:*", "users:123", true, false},
		{"prefix match multiple", "users:*", "users:abc:profile", true, false},
		{"prefix no match", "users:*", "sessions:123", false, false},
		{"prefix empty key", "users:*", "", false, false},

		// Wildcard match-all
		{"wildcard all", "*", "any:key:here", true, false},
		{"wildcard all empty", "*", "", true, false},

		// Simple wildcards
		{"middle wildcard", "user:*:profile", "user:123:profile", true, false},
		{"middle wildcard no match", "user:*:profile", "user:123:settings", false, false},

		// Question mark wildcard
		{"question mark", "user:?", "user:1", true, false},
		{"question mark no match", "user:?", "user:12", false, false},

		// Complex patterns
		{"multiple wildcards", "user:*:*", "user:123:profile", true, false},
		{"complex pattern", "user:*:prof?le", "user:123:profile", true, false},

		// Edge cases
		{"empty pattern", "", "key", false, true},
		{"empty both", "", "", false, true},
		{"pattern longer", "user:123:456", "user:123", false, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := MatchPattern(tt.pattern, tt.key)
			if (err != nil) != tt.wantErr {
				t.Errorf("MatchPattern() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("MatchPattern(%q, %q) = %v, want %v", tt.pattern, tt.key, got, tt.want)
			}
		})
	}
}

func TestMatchPattern_RegexPatterns(t *testing.T) {
	tests := []struct {
		name    string
		pattern string
		key     string
		want    bool
	}{
		{"digits only", "user:[0-9]+", "user:123", true},
		{"digits only no match", "user:[0-9]+", "user:abc", false},
		{"alphanumeric", "user:[a-zA-Z0-9]+", "user:abc123", true},
		{"optional group", "user:(123|456)", "user:123", true},
		{"optional group no match", "user:(123|456)", "user:789", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := MatchPattern(tt.pattern, tt.key)
			if err != nil {
				t.Fatalf("MatchPattern() error = %v", err)
			}
			if got != tt.want {
				t.Errorf("MatchPattern(%q, %q) = %v, want %v", tt.pattern, tt.key, got, tt.want)
			}
		})
	}
}

func TestFilterKeys(t *testing.T) {
	keys := []string{
		"user:123",
		"user:456",
		"user:789",
		"session:abc",
		"session:def",
		"product:p1",
		"product:p2",
	}

	tests := []struct {
		name    string
		pattern string
		want    []string
		wantErr bool
	}{
		{
			name:    "match all",
			pattern: "*",
			want:    keys,
			wantErr: false,
		},
		{
			name:    "prefix users",
			pattern: "user:*",
			want:    []string{"user:123", "user:456", "user:789"},
			wantErr: false,
		},
		{
			name:    "prefix sessions",
			pattern: "session:*",
			want:    []string{"session:abc", "session:def"},
			wantErr: false,
		},
		{
			name:    "exact match",
			pattern: "user:123",
			want:    []string{"user:123"},
			wantErr: false,
		},
		{
			name:    "no matches",
			pattern: "admin:*",
			want:    []string{},
			wantErr: false,
		},
		{
			name:    "empty pattern",
			pattern: "",
			want:    nil,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := FilterKeys(tt.pattern, keys)
			if (err != nil) != tt.wantErr {
				t.Errorf("FilterKeys() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr {
				if len(got) != len(tt.want) {
					t.Errorf("FilterKeys() returned %d keys, want %d", len(got), len(tt.want))
					t.Logf("Got: %v", got)
					t.Logf("Want: %v", tt.want)
					return
				}

				// Check all expected keys are present
				gotMap := make(map[string]bool)
				for _, k := range got {
					gotMap[k] = true
				}

				for _, wantKey := range tt.want {
					if !gotMap[wantKey] {
						t.Errorf("FilterKeys() missing key %q", wantKey)
					}
				}
			}
		})
	}
}

func TestPrefixMatch(t *testing.T) {
	tests := []struct {
		prefix string
		key    string
		want   bool
	}{
		{"user:", "user:123", true},
		{"user:", "session:123", false},
		{"", "any", true}, // Empty prefix matches all
		{"user:123", "user:123", true},
		{"user:123", "user:12", false},
	}

	for _, tt := range tests {
		t.Run(fmt.Sprintf("%s/%s", tt.prefix, tt.key), func(t *testing.T) {
			got := PrefixMatch(tt.prefix, tt.key)
			if got != tt.want {
				t.Errorf("PrefixMatch(%q, %q) = %v, want %v", tt.prefix, tt.key, got, tt.want)
			}
		})
	}
}

func TestGlobToRegex(t *testing.T) {
	tests := []struct {
		glob  string
		regex string
	}{
		{"user:*", "user:.*"},
		{"user:?", "user:."},
		{"user:*:profile", "user:.*:profile"},
		{"user:[123]", "user:\\[123\\]"}, // Brackets escaped
		{"user.test", "user\\.test"},      // Dot escaped
		{"*", ".*"},
		{"???", "..."},
		{"user:*:?:*", "user:.*:.:.*"},
	}

	for _, tt := range tests {
		t.Run(tt.glob, func(t *testing.T) {
			got := globToRegex(tt.glob)
			if got != tt.regex {
				t.Errorf("globToRegex(%q) = %q, want %q", tt.glob, got, tt.regex)
			}
		})
	}
}

func TestRegexCaching(t *testing.T) {
	// Clear cache before test
	ClearRegexCache()

	pattern := "user:[0-9]+"
	key := "user:123"

	// First match should compile and cache
	_, err := MatchPattern(pattern, key)
	if err != nil {
		t.Fatalf("MatchPattern() error = %v", err)
	}

	// Check cache size
	if RegexCacheSize() != 1 {
		t.Errorf("RegexCacheSize() = %d, want 1", RegexCacheSize())
	}

	// Second match should use cache
	_, err = MatchPattern(pattern, "user:456")
	if err != nil {
		t.Fatalf("MatchPattern() error = %v", err)
	}

	// Cache size should still be 1
	if RegexCacheSize() != 1 {
		t.Errorf("RegexCacheSize() = %d, want 1 (should reuse cached regex)", RegexCacheSize())
	}

	// Different pattern should add to cache
	_, err = MatchPattern("session:[a-z]+", "session:abc")
	if err != nil {
		t.Fatalf("MatchPattern() error = %v", err)
	}

	if RegexCacheSize() != 2 {
		t.Errorf("RegexCacheSize() = %d, want 2", RegexCacheSize())
	}

	// Clear and verify
	ClearRegexCache()
	if RegexCacheSize() != 0 {
		t.Errorf("RegexCacheSize() after clear = %d, want 0", RegexCacheSize())
	}
}

func TestMatchPattern_Consistency(t *testing.T) {
	// Same pattern should always return same result
	pattern := "user:*:profile"
	key := "user:123:profile"

	for i := 0; i < 100; i++ {
		match, err := MatchPattern(pattern, key)
		if err != nil {
			t.Fatalf("MatchPattern() error = %v", err)
		}
		if !match {
			t.Errorf("MatchPattern() inconsistent result at iteration %d", i)
		}
	}
}

func BenchmarkMatchPattern_Exact(b *testing.B) {
	pattern := "user:123"
	key := "user:123"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		MatchPattern(pattern, key)
	}
}

func BenchmarkMatchPattern_Prefix(b *testing.B) {
	pattern := "users:*"
	key := "users:12345"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		MatchPattern(pattern, key)
	}
}

func BenchmarkMatchPattern_Regex(b *testing.B) {
	pattern := "user:[0-9]+"
	key := "user:12345"

	// First match to compile and cache
	MatchPattern(pattern, key)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		MatchPattern(pattern, key)
	}
}

func BenchmarkFilterKeys_Prefix(b *testing.B) {
	// Generate 1000 keys
	keys := make([]string, 1000)
	for i := 0; i < 1000; i++ {
		keys[i] = fmt.Sprintf("user:%d", i)
	}

	pattern := "user:*"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		FilterKeys(pattern, keys)
	}
}

func BenchmarkFilterKeys_Regex(b *testing.B) {
	// Generate 1000 keys
	keys := make([]string, 1000)
	for i := 0; i < 1000; i++ {
		keys[i] = fmt.Sprintf("user:%d", i)
	}

	pattern := "user:[0-9]+"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		FilterKeys(pattern, keys)
	}
}