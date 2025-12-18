package integration

import (
	"bytes"
	"encoding/json"
	"fmt"
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
	if v := os.Getenv("ENCORE_URL"); v != "" {
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

func httpClient() *http.Client {
	return &http.Client{Timeout: 10 * time.Second}
}

func requireService(t *testing.T) {
	t.Helper()

	if os.Getenv("RUN_INTEGRATION_TESTS") != "1" {
		t.Skip("set RUN_INTEGRATION_TESTS=1 to run live HTTP integration tests")
	}

	// Probe a JSON endpoint on the API gateway.
	req, err := http.NewRequest(http.MethodGet, fmt.Sprintf("%s/api/cache/metrics", baseURL()), nil)
	if err != nil {
		t.Fatalf("build probe request: %v", err)
	}

	resp, err := httpClient().Do(req)
	if err != nil {
		t.Skipf("service not reachable at %s (set BASE_URL): %v", baseURL(), err)
		return
	}
	_ = resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		t.Skipf("service not ready at %s/api/cache/metrics: status=%d", baseURL(), resp.StatusCode)
	}
}

func doJSON(t *testing.T, method, path string, body any) (status int, respBody []byte) {
	t.Helper()

	var r io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("marshal body: %v", err)
		}
		r = bytes.NewReader(b)
	}

	req, err := http.NewRequest(method, baseURL()+path, r)
	if err != nil {
		t.Fatalf("build request: %v", err)
	}

	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if tok := authToken(); tok != "" {
		req.Header.Set("Authorization", "Bearer "+tok)
	}

	resp, err := httpClient().Do(req)
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

func mustUnmarshalJSON(t *testing.T, data []byte, v any) {
	t.Helper()
	if err := json.Unmarshal(data, v); err != nil {
		t.Fatalf("invalid JSON response: %v\nbody=%s", err, string(data))
	}
}

func assertStatusIn(t *testing.T, status int, allowed ...int) {
	t.Helper()
	for _, a := range allowed {
		if status == a {
			return
		}
	}
	t.Fatalf("unexpected status %d (allowed=%v)", status, allowed)
}
