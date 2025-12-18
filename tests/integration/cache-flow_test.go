package integration

import (
	"encoding/json"
	"net/http"
	"testing"
)

type cacheSetResponse struct {
	Success   bool   `json:"success"`
	ExpiresAt string `json:"expires_at"`
}

type cacheGetResponse struct {
	Value  json.RawMessage `json:"value"`
	Hit    bool            `json:"hit"`
	Source string          `json:"source"`
}

type cacheInvalidateResponse struct {
	Invalidated int  `json:"invalidated"`
	Success     bool `json:"success"`
}

type cacheMetricsResponse struct {
	Hits   int64 `json:"hits"`
	Misses int64 `json:"misses"`
	L1Size int   `json:"l1_size"`
}

func TestCacheManagerEndpoints(t *testing.T) {
	requireService(t)

	t.Run("PUT /api/cache/entry/:key", func(t *testing.T) {
		status, body := doJSON(t, http.MethodPut, "/api/cache/entry/test:user:123", map[string]any{
			"value": map[string]any{"name": "John Doe", "age": 30},
			"ttl":   60,
		})
		assertStatusIn(t, status, 200)

		var resp cacheSetResponse
		mustUnmarshalJSON(t, body, &resp)
		if !resp.Success {
			t.Fatalf("expected success=true, got false")
		}
		if resp.ExpiresAt == "" {
			t.Fatalf("expected expires_at to be set")
		}
	})

	t.Run("GET /api/cache/entry/:key", func(t *testing.T) {
		status, body := doJSON(t, http.MethodGet, "/api/cache/entry/test:user:123", nil)
		assertStatusIn(t, status, 200)

		var resp cacheGetResponse
		mustUnmarshalJSON(t, body, &resp)
		if !resp.Hit {
			t.Fatalf("expected hit=true")
		}
		if resp.Source == "" {
			t.Fatalf("expected source to be set")
		}
		if len(resp.Value) == 0 {
			t.Fatalf("expected value to be present")
		}
	})

	t.Run("GET miss (expected error)", func(t *testing.T) {
		status, _ := doJSON(t, http.MethodGet, "/api/cache/entry/test:missing:key", nil)
		// The current implementation returns an error on miss unless an origin fetcher is configured.
		assertStatusIn(t, status, 400, 404, 500)
	})

	t.Run("POST /api/cache/invalidate", func(t *testing.T) {
		status, body := doJSON(t, http.MethodPost, "/api/cache/invalidate", map[string]any{
			"keys": []string{"test:user:123"},
		})
		assertStatusIn(t, status, 200)

		var resp cacheInvalidateResponse
		mustUnmarshalJSON(t, body, &resp)
		if !resp.Success {
			t.Fatalf("expected success=true")
		}
		if resp.Invalidated < 0 {
			t.Fatalf("expected invalidated >= 0")
		}
	})

	t.Run("GET /api/cache/metrics", func(t *testing.T) {
		status, body := doJSON(t, http.MethodGet, "/api/cache/metrics", nil)
		assertStatusIn(t, status, 200)

		var resp cacheMetricsResponse
		mustUnmarshalJSON(t, body, &resp)
		if resp.Hits < 0 || resp.Misses < 0 {
			t.Fatalf("expected non-negative hits/misses")
		}
		if resp.L1Size < 0 {
			t.Fatalf("expected non-negative l1_size")
		}
	})
}
