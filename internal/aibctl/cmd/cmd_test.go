package aibctlcmd_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	aibctlcmd "github.com/agentic-identity-broker/agentic-identity-broker/internal/aibctl/cmd"
)

type result struct {
	stdout, stderr string
	code           int
}

func run(t *testing.T, stdin string, args ...string) result {
	t.Helper()
	var out, errOut bytes.Buffer
	code := aibctlcmd.Execute(aibctlcmd.Deps{Stdout: &out, Stderr: &errOut, Stdin: strings.NewReader(stdin)}, args)
	return result{stdout: out.String(), stderr: errOut.String(), code: code}
}

func configPath(t *testing.T) string {
	t.Helper()
	return filepath.Join(t.TempDir(), "aibctl", "config.yaml")
}

func TestVersion(t *testing.T) {
	r := run(t, "", "version")
	require.Equal(t, 0, r.code, r.stderr)
	assert.True(t, strings.HasPrefix(r.stdout, "aibctl dev (commit none"), r.stdout)

	r = run(t, "", "version", "-o", "json")
	require.Equal(t, 0, r.code, r.stderr)
	assert.Contains(t, r.stdout, `"version": "dev"`)
}

func TestUnknownFlagIsUsageError(t *testing.T) {
	r := run(t, "", "version", "--bogus")
	assert.Equal(t, aibctlcmd.ExitUsage, r.code)
	assert.Contains(t, r.stderr, "Error: unknown flag")
	assert.Contains(t, r.stderr, "--help")
	assert.Empty(t, r.stdout)
}

func TestContextLifecycle(t *testing.T) {
	path := configPath(t)

	r := run(t, "", "--config", path, "context", "set", "local", "--server", "http://localhost:14000", "--principal", "admin@example.com")
	require.Equal(t, 0, r.code, r.stderr)
	assert.Contains(t, r.stderr, "saved")

	r = run(t, "", "--config", path, "context", "set", "staging", "--server", "https://admin.example.com", "--token-command", "ztoken --audience broker", "--header", "X-Tenant=platform", "-o", "json")
	require.Equal(t, 0, r.code, r.stderr)

	if runtime.GOOS != "windows" {
		info, err := os.Stat(path)
		require.NoError(t, err)
		assert.Equal(t, os.FileMode(0o600), info.Mode().Perm())
	}

	r = run(t, "", "--config", path, "context", "current")
	require.Equal(t, 0, r.code, r.stderr)
	assert.Equal(t, "local\n", r.stdout, "first context becomes current")

	r = run(t, "", "--config", path, "context", "use", "staging")
	require.Equal(t, 0, r.code, r.stderr)
	r = run(t, "", "--config", path, "context", "current")
	assert.Equal(t, "staging\n", r.stdout)

	r = run(t, "", "--config", path, "context", "list")
	require.Equal(t, 0, r.code, r.stderr)
	assert.Contains(t, r.stdout, "CURRENT  NAME     SERVER")
	assert.Contains(t, r.stdout, "*        staging  https://admin.example.com  token_command  json")
	assert.Contains(t, r.stdout, "         local    http://localhost:14000     principal")

	r = run(t, "", "--config", path, "context", "set", "staging", "--token", "static")
	require.Equal(t, 0, r.code, r.stderr)
	r = run(t, "", "--config", path, "config", "view")
	require.Equal(t, 0, r.code, r.stderr)
	assert.Contains(t, r.stdout, "token: '***REDACTED***'")
	assert.NotContains(t, r.stdout, "static")
	assert.NotContains(t, r.stdout, "token_command", "setting a token clears the token command")
	assert.Contains(t, r.stdout, "X-Tenant: platform")

	r = run(t, "", "--config", path, "config", "view", "--show-secrets")
	assert.Contains(t, r.stdout, "token: static")

	r = run(t, "", "--config", path, "config", "path")
	assert.Equal(t, path+"\n", r.stdout)

	r = run(t, "", "--config", path, "context", "delete", "staging")
	assert.Equal(t, aibctlcmd.ExitUsage, r.code, "non-interactive delete needs --yes")

	r = run(t, "", "--config", path, "context", "delete", "staging", "--yes")
	require.Equal(t, 0, r.code, r.stderr)
	r = run(t, "", "--config", path, "context", "current")
	assert.Equal(t, aibctlcmd.ExitUsage, r.code, "deleting the current context clears it")

	r = run(t, "", "--config", path, "context", "use", "missing")
	assert.Equal(t, aibctlcmd.ExitUsage, r.code)
}

func TestContextSetRequiresServer(t *testing.T) {
	r := run(t, "", "--config", configPath(t), "context", "set", "x")
	assert.Equal(t, aibctlcmd.ExitUsage, r.code)
	assert.Contains(t, r.stderr, "--server")
}

func TestContextSetRejectsBadHeader(t *testing.T) {
	r := run(t, "", "--config", configPath(t), "context", "set", "x", "--server", "http://h", "--header", "novalue")
	assert.Equal(t, aibctlcmd.ExitUsage, r.code)
}

func TestContextSetRejectsManagedHeader(t *testing.T) {
	r := run(t, "", "--config", configPath(t), "context", "set", "x", "--server", "http://h", "--principal", "p", "--header", "Prefer=x")
	assert.Equal(t, aibctlcmd.ExitUsage, r.code)
	assert.Contains(t, r.stderr, "Prefer is set by aibctl")
}

func TestConfirmRefusesNullDevice(t *testing.T) {
	path := configPath(t)
	r := run(t, "", "--config", path, "context", "set", "local", "--server", "http://h", "--principal", "p")
	require.Equal(t, 0, r.code, r.stderr)
	null, err := os.Open(os.DevNull)
	require.NoError(t, err)
	defer func() { _ = null.Close() }()
	var stdout, stderr bytes.Buffer
	code := aibctlcmd.Execute(aibctlcmd.Deps{Stdin: null, Stdout: &stdout, Stderr: &stderr}, []string{"--config", path, "context", "delete", "local"})
	assert.Equal(t, aibctlcmd.ExitUsage, code)
	assert.Contains(t, stderr.String(), "requires confirmation")
	assert.NotContains(t, stderr.String(), "[y/N]")
	assert.Empty(t, stdout.String())
	r = run(t, "", "--config", path, "context", "current")
	require.Equal(t, 0, r.code, r.stderr)
	assert.Equal(t, "local\n", r.stdout)
}

func TestLocalCommandsHonorSelectionAndOutput(t *testing.T) {
	path := configPath(t)
	r := run(t, "", "--config", path, "context", "set", "local", "--server", "http://h", "--principal", "p")
	require.Equal(t, 0, r.code, r.stderr)
	r = run(t, "", "--config", path, "context", "set", "staging", "--server", "https://staging.example.com", "--token-command", "ztoken --audience broker", "--timeout", "15s", "-o", "json")
	require.Equal(t, 0, r.code, r.stderr)

	r = run(t, "", "--config", path, "--context", "staging", "context", "list")
	require.Equal(t, 0, r.code, r.stderr)
	var selected []string
	for _, line := range strings.Split(r.stdout, "\n") {
		fields := strings.Fields(line)
		if len(fields) > 0 && fields[0] == "*" {
			selected = fields
		}
	}
	require.GreaterOrEqual(t, len(selected), 2, r.stdout)
	assert.Equal(t, "staging", selected[1])

	t.Setenv("AIBCTL_OUTPUT", "json")
	r = run(t, "", "--config", path, "context", "list")
	require.Equal(t, 0, r.code, r.stderr)
	var contexts []map[string]any
	require.NoError(t, json.Unmarshal([]byte(r.stdout), &contexts))
	require.Len(t, contexts, 2)
	assert.Equal(t, "staging", contexts[1]["name"])
	assert.Equal(t, "15s", contexts[1]["timeout"])
	assert.Equal(t, []any{"ztoken", "--audience", "broker"}, contexts[1]["auth"].(map[string]any)["token_command"])

	r = run(t, "", "--config", path, "config", "view")
	require.Equal(t, 0, r.code, r.stderr)
	var document map[string]any
	require.NoError(t, json.Unmarshal([]byte(r.stdout), &document))
	assert.Equal(t, "local", document["current_context"])

	brokenPath := filepath.Join(t.TempDir(), "broken.yaml")
	require.NoError(t, os.WriteFile(brokenPath, []byte("contexts: ["), 0o600))
	r = run(t, "", "--config", brokenPath, "version")
	require.Equal(t, 0, r.code, r.stderr)
	assert.Contains(t, r.stdout, `"version": "dev"`)
}

func TestHealthUsesContext(t *testing.T) {
	var gotPrincipal, gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPrincipal = r.Header.Get("X-Remote-User")
		gotAuth = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"healthy","timestamp":"2026-01-01T00:00:00Z","uptime_seconds":90}`))
	}))
	defer srv.Close()

	path := configPath(t)
	r := run(t, "", "--config", path, "context", "set", "local", "--server", srv.URL, "--principal", "admin@example.com")
	require.Equal(t, 0, r.code, r.stderr)

	r = run(t, "", "--config", path, "health")
	require.Equal(t, 0, r.code, r.stderr)
	assert.Equal(t, "admin@example.com", gotPrincipal)
	assert.Empty(t, gotAuth)
	assert.Contains(t, r.stdout, "Status:     healthy")
	assert.Contains(t, r.stdout, "Uptime:     1m30s")

	r = run(t, "", "--config", path, "health", "-o", "json", "--token", "abc")
	require.Equal(t, 0, r.code, r.stderr)
	assert.Equal(t, "Bearer abc", gotAuth)
	assert.Contains(t, r.stdout, `"uptime_seconds": 90`)

	r = run(t, "", "--config", path, "health", "-v")
	require.Equal(t, 0, r.code, r.stderr)
	assert.Contains(t, r.stderr, "> GET "+srv.URL+"/health")
	assert.Contains(t, r.stderr, "< 200 OK")
}

func TestHealthWithoutContextIsUsageError(t *testing.T) {
	r := run(t, "", "--config", configPath(t), "health")
	assert.Equal(t, aibctlcmd.ExitUsage, r.code)
	assert.Contains(t, r.stderr, "no context")
}

func TestExitCodesFromServer(t *testing.T) {
	tests := []struct {
		name   string
		status int
		body   string
		code   int
	}{
		{"not found", 404, `{"error":"not_found","message":"nope"}`, aibctlcmd.ExitNotFound},
		{"conflict", 409, `{"error":"conflict"}`, aibctlcmd.ExitConflict},
		{"precondition", 412, `{"error":"stale"}`, aibctlcmd.ExitConflict},
		{"bad request", 400, `{"error":"invalid"}`, aibctlcmd.ExitValidation},
		{"forbidden", 403, "forbidden", aibctlcmd.ExitAuth},
		{"server error", 500, `{"error":"boom"}`, aibctlcmd.ExitGeneric},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				if strings.HasPrefix(tc.body, "{") {
					w.Header().Set("Content-Type", "application/json")
				}
				w.WriteHeader(tc.status)
				_, _ = w.Write([]byte(tc.body))
			}))
			defer srv.Close()
			r := run(t, "", "--config", configPath(t), "--server", srv.URL, "--principal", "p", "health")
			assert.Equal(t, tc.code, r.code, r.stderr)
			assert.Contains(t, r.stderr, "Error: ")
		})
	}
}

func TestTokenCommandFailureIsAuthError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(200) }))
	defer srv.Close()
	r := run(t, "", "--config", configPath(t), "--server", srv.URL, "--token-command", "aibctl-definitely-missing-binary", "health")
	assert.Equal(t, aibctlcmd.ExitAuth, r.code)
	assert.Contains(t, r.stderr, "aibctl-definitely-missing-binary")
}

func TestConnectionErrorIsGeneric(t *testing.T) {
	r := run(t, "", "--config", configPath(t), "--server", "http://127.0.0.1:1", "--principal", "p", "--timeout", "2s", "health")
	assert.Equal(t, aibctlcmd.ExitGeneric, r.code)
}

func TestCompletionCommandAndContextCompletion(t *testing.T) {
	r := run(t, "", "completion", "bash")
	require.Equal(t, 0, r.code, r.stderr)
	assert.Contains(t, r.stdout, "__start_aibctl")

	path := configPath(t)
	require.Equal(t, 0, run(t, "", "--config", path, "context", "set", "alpha", "--server", "http://a").code)
	require.Equal(t, 0, run(t, "", "--config", path, "context", "set", "beta", "--server", "http://b").code)
	r = run(t, "", "--config", path, "__complete", "context", "use", "a")
	require.Equal(t, 0, r.code, r.stderr)
	assert.Contains(t, r.stdout, "alpha\thttp://a")
	assert.NotContains(t, r.stdout, "beta")
}
