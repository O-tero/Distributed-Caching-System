package integration

import (
	"net/http"
	"testing"
)

type invalidateKeyResponse struct {
	Success          bool     `json:"success"`
	InvalidatedCount int      `json:"invalidated_count"`
	Keys             []string `json:"keys"`
	RequestID        string   `json:"request_id"`
}

type invalidatePatternResponse struct {
	Success          bool     `json:"success"`
	Pattern          string   `json:"pattern"`
	MatchedKeys      []string `json:"matched_keys"`
	InvalidatedCount int      `json:"invalidated_count"`
	RequestID        string   `json:"request_id"`
}

type auditLogsResponse struct {
	Logs       []any `json:"logs"`
	TotalCount int   `json:"total_count"`
	HasMore    bool  `json:"has_more"`
}

type invalidationMetricsResponse struct {
	TotalInvalidations int64 `json:"total_invalidations"`
	Errors             int64 `json:"errors"`
}

func TestInvalidationEndpoints(t *testing.T) {
	requireService(t)

	t.Run("POST /invalidate/key", func(t *testing.T) {
		status, body := doJSON(t, http.MethodPost, "/invalidate/key", map[string]any{
			"keys":         []string{"test:inv:user:1"},
			"triggered_by": "go-tests",
			"request_id":   "",
		})
		assertStatusIn(t, status, 200)

		var resp invalidateKeyResponse
		mustUnmarshalJSON(t, body, &resp)
		if !resp.Success {
			t.Fatalf("expected success=true")
		}
		if resp.InvalidatedCount <= 0 {
			t.Fatalf("expected invalidated_count > 0")
		}
		if resp.RequestID == "" {
			t.Fatalf("expected request_id to be set")
		}
	})

	t.Run("POST /invalidate/key - empty keys (expected error)", func(t *testing.T) {
		status, _ := doJSON(t, http.MethodPost, "/invalidate/key", map[string]any{
			"keys": []string{},
		})
		assertStatusIn(t, status, 400, 500)
	})

	t.Run("POST /invalidate/pattern", func(t *testing.T) {
		status, body := doJSON(t, http.MethodPost, "/invalidate/pattern", map[string]any{
			"pattern":      "test:inv:*",
			"triggered_by": "go-tests",
			"request_id":   "",
		})
		assertStatusIn(t, status, 200)

		var resp invalidatePatternResponse
		mustUnmarshalJSON(t, body, &resp)
		if !resp.Success {
			t.Fatalf("expected success=true")
		}
		if resp.Pattern != "test:inv:*" {
			t.Fatalf("expected pattern to echo back")
		}
		if resp.RequestID == "" {
			t.Fatalf("expected request_id to be set")
		}
	})

	t.Run("POST /invalidate/pattern - empty pattern (expected error)", func(t *testing.T) {
		status, _ := doJSON(t, http.MethodPost, "/invalidate/pattern", map[string]any{
			"pattern": "",
		})
		assertStatusIn(t, status, 400, 500)
	})

	t.Run("GET /audit/logs", func(t *testing.T) {
		status, body := doJSON(t, http.MethodGet, "/audit/logs?limit=10&offset=0", nil)
		assertStatusIn(t, status, 200)

		var resp auditLogsResponse
		mustUnmarshalJSON(t, body, &resp)
		if resp.TotalCount < 0 {
			t.Fatalf("expected non-negative total_count")
		}
		_ = resp.HasMore
	})

	t.Run("GET /invalidate/metrics", func(t *testing.T) {
		status, body := doJSON(t, http.MethodGet, "/invalidate/metrics", nil)
		assertStatusIn(t, status, 200)

		var resp invalidationMetricsResponse
		mustUnmarshalJSON(t, body, &resp)
		if resp.TotalInvalidations < 0 || resp.Errors < 0 {
			t.Fatalf("expected non-negative metrics")
		}
	})
}
