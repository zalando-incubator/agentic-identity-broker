package aibctlcmd_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	aibctlcmd "github.com/agentic-identity-broker/agentic-identity-broker/internal/aibctl/cmd"
)

type fakeKeys struct {
	mu       sync.Mutex
	requests []recordedRequest
	srv      *httptest.Server
	hasCreds bool
}

func newFakeKeys(t *testing.T) *fakeKeys {
	t.Helper()
	f := &fakeKeys{}
	key := func(kid string, current bool) map[string]any {
		return map[string]any{"kid": kid, "algorithm": "ES256", "is_current": current, "activates_at": "a", "created_at": "c"}
	}
	f.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rec := recordedRequest{method: r.Method, path: r.URL.RequestURI(), contentType: r.Header.Get("Content-Type")}
		_ = json.NewDecoder(r.Body).Decode(&rec.body)
		f.mu.Lock()
		f.requests = append(f.requests, rec)
		f.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && (r.URL.Path == "/api/oauth2-server/signing-keys" || r.URL.Path == "/api/cimd-client-keys"):
			_ = json.NewEncoder(w).Encode(map[string]any{"items": []any{key("k2", true), key("k1", false)}})
		case r.Method == http.MethodPost && (r.URL.Path == "/api/oauth2-server/signing-keys" || r.URL.Path == "/api/cimd-client-keys"):
			w.WriteHeader(201)
			_ = json.NewEncoder(w).Encode(key("k3", false))
		case r.Method == http.MethodPut && strings.HasSuffix(r.URL.Path, "/k1/current"):
			_ = json.NewEncoder(w).Encode(key("k1", true))
		case r.Method == http.MethodDelete && strings.HasSuffix(r.URL.Path, "/k2"):
			w.WriteHeader(409)
			_, _ = w.Write([]byte(`{"error":"current_key","message":"cannot remove the current key"}`))
		case r.Method == http.MethodDelete && strings.HasSuffix(r.URL.Path, "/k1"):
			w.WriteHeader(204)
		case r.URL.Path == "/api/agents/research-agent/client-credentials":
			switch r.Method {
			case http.MethodGet:
				if !f.hasCreds {
					w.WriteHeader(404)
					_, _ = w.Write([]byte(`{"error":"not_found","message":"no credentials"}`))
					return
				}
				_ = json.NewEncoder(w).Encode(map[string]any{"client_id": "u", "created_at": "c", "rotated_at": "r"})
			case http.MethodPost:
				if f.hasCreds {
					_ = json.NewEncoder(w).Encode(map[string]any{"client_id": "u", "client_secret": "s2", "created_at": "c", "previous_invalidated_at": "p"})
					return
				}
				f.hasCreds = true
				w.WriteHeader(201)
				_ = json.NewEncoder(w).Encode(map[string]any{"client_id": "u", "client_secret": "s1", "created_at": "c"})
			case http.MethodDelete:
				f.hasCreds = false
				w.WriteHeader(204)
			}
		default:
			w.WriteHeader(500)
			_, _ = w.Write([]byte(`{"error":"unexpected","message":"` + r.Method + " " + r.URL.Path + `"}`))
		}
	}))
	t.Cleanup(f.srv.Close)
	return f
}

func (f *fakeKeys) args(t *testing.T, extra ...string) []string {
	t.Helper()
	return append([]string{"--config", configPath(t), "--server", f.srv.URL, "--principal", "admin@example.com"}, extra...)
}

func (f *fakeKeys) last() recordedRequest {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.requests[len(f.requests)-1]
}

func TestKeyCommands(t *testing.T) {
	for _, family := range []struct{ noun, path, title string }{{"signing-key", "/api/oauth2-server/signing-keys", "Signing key"}, {"cimd-client-key", "/api/cimd-client-keys", "CIMD client key"}} {
		t.Run(family.noun, func(t *testing.T) {
			f := newFakeKeys(t)
			r := run(t, "", f.args(t, family.noun, "list")...)
			require.Equal(t, 0, r.code, r.stderr)
			assert.Equal(t, family.path, f.last().path)
			lines := strings.Split(strings.TrimSpace(r.stdout), "\n")
			assert.Equal(t, []string{"KID", "ALGORITHM", "CURRENT", "ACTIVATES_AT", "CREATED_AT"}, strings.Fields(lines[0]))
			assert.Equal(t, []string{"k2", "ES256", "true", "a", "c"}, strings.Fields(lines[1]))

			r = run(t, "", f.args(t, family.noun, "create")...)
			require.Equal(t, 0, r.code, r.stderr)
			req := f.last()
			assert.Equal(t, http.MethodPost, req.method)
			assert.Equal(t, "application/json", req.contentType, "body-less POST still declares JSON")
			assert.Nil(t, req.body)
			assert.Contains(t, r.stderr, family.title+` "k3" created.`)

			r = run(t, "", f.args(t, family.noun, "create", "--algorithm", "ES256")...)
			require.Equal(t, 0, r.code, r.stderr)
			assert.Equal(t, "ES256", f.last().body["algorithm"])

			r = run(t, "", f.args(t, family.noun, "set-current", "k1")...)
			require.Equal(t, 0, r.code, r.stderr)
			assert.Equal(t, family.path+"/k1/current", f.last().path)
			assert.Equal(t, "application/json", f.last().contentType)
			assert.Contains(t, r.stdout, "Current:       true")

			r = run(t, "", f.args(t, family.noun, "delete", "k2", "-y")...)
			assert.Equal(t, aibctlcmd.ExitConflict, r.code)
			assert.Contains(t, r.stderr, "cannot remove the current key (current_key, HTTP 409)")

			r = run(t, "", f.args(t, family.noun, "delete", "k1", "-y")...)
			require.Equal(t, 0, r.code, r.stderr)

			r = run(t, "", f.args(t, "__complete", family.noun, "delete", "")...)
			assert.Contains(t, r.stdout, "k2\tES256 (current)\n")
			assert.Contains(t, r.stdout, "k1\tES256\n")
		})
	}
}

func TestClientCredentialsCommands(t *testing.T) {
	f := newFakeKeys(t)
	r := run(t, "", f.args(t, "agent", "client-credentials", "get", "research-agent")...)
	require.Equal(t, aibctlcmd.ExitNotFound, r.code, r.stderr)

	r = run(t, "", f.args(t, "agent", "creds", "rotate", "research-agent", "--yes")...)
	require.Equal(t, aibctlcmd.ExitNotFound, r.code, r.stderr)
	assert.Contains(t, r.stderr, "no client credentials yet")

	r = run(t, "", f.args(t, "agent", "client-credentials", "create", "research-agent")...)
	require.Equal(t, 0, r.code, r.stderr)
	assert.Contains(t, r.stderr, "created")
	assert.Contains(t, r.stderr, "shown once")
	assert.Contains(t, r.stdout, "Client Secret:         s1")
	assert.Equal(t, "application/json", f.last().contentType)

	r = run(t, "", f.args(t, "agent", "client-credentials", "create", "research-agent")...)
	assert.Equal(t, aibctlcmd.ExitConflict, r.code)
	assert.Contains(t, r.stderr, "already has client credentials")
	assert.Equal(t, http.MethodGet, f.last().method, "create must not rotate")

	r = run(t, "", f.args(t, "agent", "client-credentials", "get", "research-agent", "-o", "json")...)
	require.Equal(t, 0, r.code, r.stderr)
	assert.Contains(t, r.stdout, `"rotated_at": "r"`)
	assert.NotContains(t, r.stdout, "client_secret")

	r = run(t, "", f.args(t, "agent", "client-credentials", "rotate", "research-agent", "-o", "json")...)
	assert.Equal(t, aibctlcmd.ExitUsage, r.code)
	assert.Contains(t, r.stderr, "requires confirmation")
	assert.Equal(t, http.MethodGet, f.last().method, "unconfirmed rotation must not POST")

	r = run(t, "", f.args(t, "agent", "client-credentials", "rotate", "research-agent", "--yes", "-o", "json")...)
	require.Equal(t, 0, r.code, r.stderr)
	assert.Contains(t, r.stderr, "rotated")
	assert.Contains(t, r.stdout, `"previous_invalidated_at": "p"`)

	r = run(t, "", f.args(t, "agent", "client-credentials", "delete", "research-agent")...)
	assert.Equal(t, aibctlcmd.ExitUsage, r.code)
	r = run(t, "", f.args(t, "agent", "client-credentials", "revoke", "research-agent", "--yes")...)
	require.Equal(t, 0, r.code, r.stderr)
	assert.Equal(t, http.MethodDelete, f.last().method)
}
