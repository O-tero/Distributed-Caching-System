package integration

import (
	"net/http"
	"testing"
	"time"
)

type dashboardOverviewResponse struct {
	Summary any `json:"summary"`
}

type dashboardLatencyDistributionResponse struct {
	Buckets []any `json:"buckets"`
}

type dashboardHeatmapResponse struct {
	XLabels []string `json:"x_labels"`
	YLabels []string `json:"y_labels"`
	Data    []any    `json:"data"`
}

type dashboardComparisonResponse struct {
	Differences any `json:"differences"`
}

type streamSessionResponse struct {
	ID        string `json:"id"`
	CreatedAt string `json:"created_at"`
}

type exportResponse struct {
	Format   string `json:"format"`
	Data     string `json:"data"`
	Filename string `json:"filename"`
	Size     int    `json:"size"`
}

func TestMonitoringDashboardEndpoints(t *testing.T) {
	requireService(t)

	t.Run("POST /monitoring/dashboard/overview", func(t *testing.T) {
		status, body := doJSON(t, http.MethodPost, "/monitoring/dashboard/overview", map[string]any{"time_range": "1h"})
		assertStatusIn(t, status, 200)

		var resp dashboardOverviewResponse
		mustUnmarshalJSON(t, body, &resp)
		if resp.Summary == nil {
			t.Fatalf("expected summary")
		}
	})

	t.Run("POST /monitoring/dashboard/latency-distribution", func(t *testing.T) {
		status, body := doJSON(t, http.MethodPost, "/monitoring/dashboard/latency-distribution", map[string]any{"window": "5m"})
		assertStatusIn(t, status, 200)

		var resp dashboardLatencyDistributionResponse
		mustUnmarshalJSON(t, body, &resp)
		_ = resp.Buckets
	})

	t.Run("POST /monitoring/dashboard/heatmap", func(t *testing.T) {
		now := time.Now().UTC()
		start := now.Add(-1 * time.Hour)

		status, body := doJSON(t, http.MethodPost, "/monitoring/dashboard/heatmap", map[string]any{
			"start_time": start,
			"end_time":   now,
			"metric":     "hit_rate",
		})
		assertStatusIn(t, status, 200)

		var resp dashboardHeatmapResponse
		mustUnmarshalJSON(t, body, &resp)
		// No strong assertions; depends on configured duration bucketization.
		_ = resp.XLabels
		_ = resp.YLabels
		_ = resp.Data
	})

	t.Run("POST /monitoring/dashboard/comparison", func(t *testing.T) {
		now := time.Now().UTC()
		p1s := now.Add(-2 * time.Hour)
		p1e := now.Add(-1 * time.Hour)
		p2s := now.Add(-1 * time.Hour)
		p2e := now

		status, body := doJSON(t, http.MethodPost, "/monitoring/dashboard/comparison", map[string]any{
			"period1_start": p1s,
			"period1_end":   p1e,
			"period2_start": p2s,
			"period2_end":   p2e,
		})
		assertStatusIn(t, status, 200)

		var resp dashboardComparisonResponse
		mustUnmarshalJSON(t, body, &resp)
		if resp.Differences == nil {
			t.Fatalf("expected differences")
		}
	})

	t.Run("GET /monitoring/dashboard/stream", func(t *testing.T) {
		status, body := doJSON(t, http.MethodGet, "/monitoring/dashboard/stream", nil)
		assertStatusIn(t, status, 200)

		var resp streamSessionResponse
		mustUnmarshalJSON(t, body, &resp)
		if resp.ID == "" {
			t.Fatalf("expected id")
		}
		if resp.CreatedAt == "" {
			t.Fatalf("expected created_at")
		}
	})

	t.Run("POST /monitoring/dashboard/export", func(t *testing.T) {
		now := time.Now().UTC()
		start := now.Add(-1 * time.Hour)

		status, body := doJSON(t, http.MethodPost, "/monitoring/dashboard/export", map[string]any{
			"start_time": start,
			"end_time":   now,
			"format":     "json",
			"metrics":    []string{"cache_hits", "cache_misses", "hit_rate"},
		})
		assertStatusIn(t, status, 200)

		var resp exportResponse
		mustUnmarshalJSON(t, body, &resp)
		if resp.Format != "json" {
			t.Fatalf("expected format=json")
		}
		if resp.Filename == "" {
			t.Fatalf("expected filename")
		}
		if resp.Data == "" {
			t.Fatalf("expected data")
		}
		if resp.Size <= 0 {
			t.Fatalf("expected size > 0")
		}
	})
}
