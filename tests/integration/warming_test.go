package integration

import (
	"net/http"
	"testing"
)

type warmKeyResponse struct {
	Success       bool     `json:"success"`
	Queued        int      `json:"queued"`
	Keys          []string `json:"keys"`
	JobID         string   `json:"job_id"`
	EstimatedTime int      `json:"estimated_time_ms"`
}

type warmPatternResponse struct {
	Success       bool     `json:"success"`
	Pattern       string   `json:"pattern"`
	Queued        int      `json:"queued"`
	MatchedKeys   []string `json:"matched_keys"`
	JobID         string   `json:"job_id"`
	EstimatedTime int      `json:"estimated_time_ms"`
}

type warmStatusResponse struct {
	ActiveJobs    int  `json:"active_jobs"`
	QueuedTasks   int  `json:"queued_tasks"`
	EmergencyStop bool `json:"emergency_stop"`
}

type warmConfigResponse struct {
	Config struct {
		MaxOriginRPS    int    `json:"max_origin_rps"`
		DefaultStrategy string `json:"default_strategy"`
	} `json:"config"`
}

func TestWarmingEndpoints(t *testing.T) {
	requireService(t)

	t.Run("POST /warm/key", func(t *testing.T) {
		status, body := doJSON(t, http.MethodPost, "/warm/key", map[string]any{
			"keys":     []string{"warm:test:key:1", "warm:test:key:2"},
			"priority": 50,
			"strategy": "priority",
		})
		assertStatusIn(t, status, 200)

		var resp warmKeyResponse
		mustUnmarshalJSON(t, body, &resp)
		if !resp.Success {
			t.Fatalf("expected success=true")
		}
		if resp.JobID == "" {
			t.Fatalf("expected job_id to be set")
		}
	})

	t.Run("POST /warm/key - empty keys (expected error)", func(t *testing.T) {
		status, _ := doJSON(t, http.MethodPost, "/warm/key", map[string]any{"keys": []string{}})
		assertStatusIn(t, status, 400, 500)
	})

	t.Run("POST /warm/pattern", func(t *testing.T) {
		status, body := doJSON(t, http.MethodPost, "/warm/pattern", map[string]any{
			"pattern":  "warm:test:*",
			"limit":    10,
			"priority": 50,
			"strategy": "priority",
		})
		assertStatusIn(t, status, 200)

		var resp warmPatternResponse
		mustUnmarshalJSON(t, body, &resp)
		if !resp.Success {
			t.Fatalf("expected success=true")
		}
		if resp.Pattern != "warm:test:*" {
			t.Fatalf("expected pattern echo")
		}
		if resp.JobID == "" {
			t.Fatalf("expected job_id to be set")
		}
	})

	t.Run("GET /warm/status", func(t *testing.T) {
		status, body := doJSON(t, http.MethodGet, "/warm/status", nil)
		assertStatusIn(t, status, 200)

		var resp warmStatusResponse
		mustUnmarshalJSON(t, body, &resp)
		if resp.ActiveJobs < 0 || resp.QueuedTasks < 0 {
			t.Fatalf("expected non-negative status counters")
		}
	})

	t.Run("POST /warm/trigger-predictive", func(t *testing.T) {
		status, body := doJSON(t, http.MethodPost, "/warm/trigger-predictive", nil)
		assertStatusIn(t, status, 200)

		var resp warmKeyResponse
		mustUnmarshalJSON(t, body, &resp)
		// Success may be true with queued=0, depending on predictor.
		_ = resp.Success
	})

	t.Run("GET /warm/config", func(t *testing.T) {
		status, body := doJSON(t, http.MethodGet, "/warm/config", nil)
		assertStatusIn(t, status, 200)

		var resp warmConfigResponse
		mustUnmarshalJSON(t, body, &resp)
		if resp.Config.MaxOriginRPS <= 0 {
			t.Fatalf("expected max_origin_rps > 0")
		}
		if resp.Config.DefaultStrategy == "" {
			t.Fatalf("expected default_strategy to be set")
		}
	})

	t.Run("POST /warm/config", func(t *testing.T) {
		status, body := doJSON(t, http.MethodPost, "/warm/config", map[string]any{"max_origin_rps": 200})
		assertStatusIn(t, status, 200)

		var resp warmConfigResponse
		mustUnmarshalJSON(t, body, &resp)
		if resp.Config.MaxOriginRPS != 200 {
			t.Fatalf("expected max_origin_rps updated to 200, got %d", resp.Config.MaxOriginRPS)
		}
	})
}
