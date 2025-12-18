package integration

import (
	"net/http"
	"testing"
	"time"
)

type monitoringMetricsResponse struct {
	Window    string  `json:"window"`
	HitRate   float64 `json:"hit_rate"`
	Timestamp string  `json:"timestamp"`
}

type monitoringAggregatedResponse struct {
	DataPoints []any `json:"data_points"`
	Summary    any   `json:"summary"`
}

type monitoringAlertsResponse struct {
	ActiveAlerts []any `json:"active_alerts"`
	AlertStats   any   `json:"alert_stats"`
}

func TestMonitoringEndpoints(t *testing.T) {
	requireService(t)

	t.Run("GET /monitoring/metrics", func(t *testing.T) {
		status, body := doJSON(t, http.MethodGet, "/monitoring/metrics?window=1m", nil)
		assertStatusIn(t, status, 200)

		var resp monitoringMetricsResponse
		mustUnmarshalJSON(t, body, &resp)
		if resp.Window == "" {
			t.Fatalf("expected window to be set")
		}
		if resp.Timestamp == "" {
			t.Fatalf("expected timestamp to be set")
		}
		_ = resp.HitRate
	})

	t.Run("POST /monitoring/aggregated", func(t *testing.T) {
		now := time.Now().UTC()
		start := now.Add(-10 * time.Minute)

		status, body := doJSON(t, http.MethodPost, "/monitoring/aggregated", map[string]any{
			"start_time": start,
			"end_time":   now,
			"interval":   "1m",
		})
		assertStatusIn(t, status, 200)

		var resp monitoringAggregatedResponse
		mustUnmarshalJSON(t, body, &resp)
		// Presence checks only; actual data depends on runtime activity.
		_ = resp.DataPoints
		_ = resp.Summary
	})

	t.Run("GET /monitoring/alerts", func(t *testing.T) {
		status, body := doJSON(t, http.MethodGet, "/monitoring/alerts", nil)
		assertStatusIn(t, status, 200)

		var resp monitoringAlertsResponse
		mustUnmarshalJSON(t, body, &resp)
		_ = resp.ActiveAlerts
		_ = resp.AlertStats
	})
}
