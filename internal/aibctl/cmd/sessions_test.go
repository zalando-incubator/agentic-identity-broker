package aibctlcmd_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	aibctlcmd "github.com/agentic-identity-broker/agentic-identity-broker/internal/aibctl/cmd"
)

func TestSessionSweep(t *testing.T) {
	var mu sync.Mutex
	var requests []recordedRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rec := recordedRequest{method: r.Method, path: r.URL.Path, contentType: r.Header.Get("Content-Type")}
		_ = json.NewDecoder(r.Body).Decode(&rec.body)
		mu.Lock()
		requests = append(requests, rec)
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		if rec.body["lookahead_duration"] == "10m" {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"error":"invalid request body","message":"lookahead_duration must be an ISO 8601 duration"}`))
			return
		}
		time.Sleep(200 * time.Millisecond)
		_, _ = w.Write([]byte(`{"refreshed":2,"skipped":1,"failed":0,"total_evaluated":3,"dry_run":true}`))
	}))
	defer srv.Close()
	path := filepath.Join(t.TempDir(), "config.yaml")
	r := run(t, "", "--config", path, "context", "set", "local", "--server", srv.URL, "--principal", "admin@example.com", "--timeout", "20ms")
	require.Equal(t, 0, r.code, r.stderr)
	args := []string{"--config", path, "session", "sweep"}
	last := func() recordedRequest {
		mu.Lock()
		defer mu.Unlock()
		return requests[len(requests)-1]
	}

	r = run(t, "", args...)
	require.Equal(t, 0, r.code, r.stderr)
	assert.Equal(t, http.MethodPost, last().method)
	assert.Equal(t, "/api/sessions/sweep", last().path)
	assert.Equal(t, "application/json", last().contentType)
	assert.Equal(t, map[string]any{}, last().body)
	assert.Contains(t, r.stdout, "Total Evaluated:  3")

	r = run(t, "", append(args, "--dry-run", "--lookahead", "PT10M", "--page-size", "100", "-o", "json")...)
	require.Equal(t, 0, r.code, r.stderr)
	assert.Equal(t, map[string]any{"lookahead_duration": "PT10M", "dry_run": true, "page_size": float64(100)}, last().body)
	assert.Contains(t, r.stdout, `"total_evaluated": 3`)

	r = run(t, "", append(args, "--timeout", "20ms")...)
	assert.Equal(t, aibctlcmd.ExitGeneric, r.code, r.stderr)

	r = run(t, "", append(args, "--lookahead", "10m")...)
	assert.Equal(t, aibctlcmd.ExitValidation, r.code, r.stderr)
	assert.Empty(t, r.stdout)
}
