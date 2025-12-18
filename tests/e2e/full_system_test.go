package e2e

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"testing"
	"time"
)

func baseURL() string {
	if v := os.Getenv("BASE_URL"); v != "" {
		return v
	}
	if v := os.Getenv("APP_URL"); v != "" {
		return v
	}
	return "http://localhost:4000"
}

func authToken() string {
	if v := os.Getenv("AUTH_TOKEN"); v != "" {
		return v
	}
	return os.Getenv("API_TOKEN_ADMIN")
}

func requireService(t *testing.T) {
	t.Helper()

	if os.Getenv("RUN_INTEGRATION_TESTS") != "1" {
		t.Skip("set RUN_INTEGRATION_TESTS=1 to run live HTTP e2e tests")
	}

	client := &http.Client{Timeout: 10 * time.Second}
	// Probe a JSON endpoint on the API gateway.
	req, _ := http.NewRequest(http.MethodGet, baseURL()+"/api/cache/metrics", nil)
	resp, err := client.Do(req)
	if err != nil {
		t.Skipf("service not reachable at %s: %v", baseURL(), err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		t.Skipf("service not ready at %s/api/cache/metrics: status=%d", baseURL(), resp.StatusCode)
	}
}

func doJSON(t *testing.T, method, path string, body any) (int, []byte) {
	t.Helper()

	var reqBody []byte
	var err error
	if body != nil {
		reqBody, err = json.Marshal(body)
		if err != nil {
			t.Fatalf("marshal body: %v", err)
		}
	}

	req, err := http.NewRequest(method, baseURL()+path, bytesReader(reqBody))
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if tok := authToken(); tok != "" {
		req.Header.Set("Authorization", "Bearer "+tok)
	}

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read response: %v", err)
	}
	return resp.StatusCode, data
}

func bytesReader(b []byte) *bytes.Reader {
	if len(b) == 0 {
		return bytes.NewReader(nil)
	}
	return bytes.NewReader(b)
}

func TestFullSystemSmoke(t *testing.T) {
	requireService(t)

	// 1) Write an entry
	status, _ := doJSON(t, http.MethodPut, "/api/cache/entry/e2e:user:1", map[string]any{
		"value": map[string]any{"name": "E2E User"},
		"ttl":   60,
	})
	if status != 200 {
		t.Fatalf("expected PUT cache entry 200, got %d", status)
	}

	// 2) Read it back
	status, _ = doJSON(t, http.MethodGet, "/api/cache/entry/e2e:user:1", nil)
	if status != 200 {
		t.Fatalf("expected GET cache entry 200, got %d", status)
	}

	// 3) Trigger invalidation via cache-manager
	status, _ = doJSON(t, http.MethodPost, "/api/cache/invalidate", map[string]any{
		"keys": []string{"e2e:user:1"},
	})
	if status != 200 {
		t.Fatalf("expected POST cache invalidate 200, got %d", status)
	}

	// 4) Trigger warming
	status, _ = doJSON(t, http.MethodPost, "/warm/pattern", map[string]any{
		"pattern":  "e2e:*",
		"limit":    10,
		"priority": 80,
		"strategy": "priority",
	})
	if status != 200 {
		t.Fatalf("expected POST warm/pattern 200, got %d", status)
	}

	// 5) Call monitoring endpoints (smoke)
	status, _ = doJSON(t, http.MethodGet, "/monitoring/metrics?window=1m", nil)
	if status != 200 {
		t.Fatalf("expected GET monitoring/metrics 200, got %d", status)
	}
}
