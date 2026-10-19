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

type fakeServices struct {
	mu       sync.Mutex
	requests []recordedRequest
	srv      *httptest.Server
	etag     string
}

func newFakeServices(t *testing.T) *fakeServices {
	t.Helper()
	f := &fakeServices{etag: `"3"`}
	service := func() map[string]any {
		return map[string]any{
			"id": "44444444-4444-4444-4444-444444444444", "canonical_id": "github", "display_name": "GitHub", "client_id": "gh-client",
			"client_secret": "REDACTED", "token_endpoint_auth_method": nil, "oauth2_flavor": "github", "issuer_uri": "https://github.com",
			"discovery": map[string]any{"enable_discovery": false}, "endpoints": map[string]any{"token_endpoint": "https://github.com/t", "authorize_endpoint": "https://github.com/a"},
			"scopes": []any{map[string]any{"scope_value": "repo", "description": "Repos"}}, "protected_resources": []any{"https://api.github.com/"},
			"created_at": "c", "updated_at": "u",
		}
	}
	cimd := func() map[string]any {
		return map[string]any{
			"id": "55555555-5555-5555-5555-555555555555", "canonical_id": "cimd", "display_name": "CIMD",
			"client_id":                  "https://broker.example.com/.well-known/oauth-client/55555555-5555-5555-5555-555555555555",
			"token_endpoint_auth_method": "private_key_jwt", "oauth2_flavor": "standard", "issuer_uri": "https://idp.example.com",
			"discovery": map[string]any{"enable_discovery": true},
			"endpoints": map[string]any{"token_endpoint": "https://idp.example.com/token", "authorize_endpoint": "https://idp.example.com/authorize"},
			"scopes":    []any{}, "created_at": "c", "updated_at": "u",
		}
	}
	f.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rec := recordedRequest{method: r.Method, path: r.URL.RequestURI(), prefer: r.Header.Get("Prefer"), contentType: r.Header.Get("Content-Type"), ifMatch: r.Header.Get("If-Match")}
		_ = json.NewDecoder(r.Body).Decode(&rec.body)
		f.mu.Lock()
		f.requests = append(f.requests, rec)
		f.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/services":
			_ = json.NewEncoder(w).Encode([]any{service()})
		case r.Method == http.MethodGet && r.URL.Path == "/api/services/github":
			w.Header().Set("ETag", f.etag)
			_ = json.NewEncoder(w).Encode(service())
		case r.Method == http.MethodPut && r.URL.Path == "/api/services/github":
			if rec.ifMatch != f.etag {
				w.WriteHeader(412)
				_, _ = w.Write([]byte(`{"error":"precondition failed","message":"service changed"}`))
				return
			}
			_ = json.NewEncoder(w).Encode(service())
		case r.Method == http.MethodGet && r.URL.Path == "/api/services/cimd":
			w.Header().Set("ETag", f.etag)
			_ = json.NewEncoder(w).Encode(cimd())
		case r.Method == http.MethodPut && r.URL.Path == "/api/services/cimd":
			if rec.ifMatch != f.etag {
				w.WriteHeader(412)
				_, _ = w.Write([]byte(`{"error":"precondition failed","message":"service changed"}`))
				return
			}
			_ = json.NewEncoder(w).Encode(cimd())
		case r.Method == http.MethodPost && r.URL.Path == "/api/services":
			w.WriteHeader(201)
			_ = json.NewEncoder(w).Encode(service())
		case r.Method == http.MethodGet && r.URL.Path == "/api/services/github/protected-resources":
			w.Header().Set("ETag", f.etag)
			_ = json.NewEncoder(w).Encode(map[string]any{"protected_resources": []any{"https://api.github.com/"}})
		case r.Method == http.MethodPut && strings.HasPrefix(r.URL.Path, "/api/services/github/protected-resources/"):
			if strings.HasSuffix(r.URL.RawPath, "existing") {
				w.WriteHeader(200)
			} else {
				w.WriteHeader(201)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"resource": "https://x/", "protected_resources": []any{"https://api.github.com/", "https://x/"}})
		case r.Method == http.MethodDelete && strings.HasPrefix(r.URL.Path, "/api/services/github/protected-resources/"):
			_ = json.NewEncoder(w).Encode(map[string]any{"resource": "https://x/", "protected_resources": []any{"https://api.github.com/"}})
		case r.Method == http.MethodPatch && strings.HasPrefix(r.URL.Path, "/api/services/github/protected-resources/"):
			_ = json.NewEncoder(w).Encode(map[string]any{"resource": rec.body["to"], "protected_resources": []any{rec.body["to"]}})
		default:
			w.WriteHeader(500)
			_, _ = w.Write([]byte(`{"error":"unexpected","message":"` + r.Method + " " + r.URL.Path + `"}`))
		}
	}))
	t.Cleanup(f.srv.Close)
	return f
}

func (f *fakeServices) args(t *testing.T, extra ...string) []string {
	t.Helper()
	return append([]string{"--config", configPath(t), "--server", f.srv.URL, "--principal", "admin@example.com"}, extra...)
}

func (f *fakeServices) last() recordedRequest {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.requests[len(f.requests)-1]
}

func TestServiceListAndGet(t *testing.T) {
	f := newFakeServices(t)
	r := run(t, "", f.args(t, "service", "list")...)
	require.Equal(t, 0, r.code, r.stderr)
	lines := strings.Split(strings.TrimSpace(r.stdout), "\n")
	assert.Equal(t, []string{"ID", "DISPLAY_NAME", "FLAVOR", "AUTH_METHOD", "ISSUER", "PROTECTED_RESOURCES", "UPDATED"}, strings.Fields(lines[0]))
	assert.Equal(t, []string{"github", "GitHub", "github", "client_secret", "https://github.com", "1", "u"}, strings.Fields(lines[1]))

	r = run(t, "", f.args(t, "service", "list", "-o", "wide")...)
	require.Equal(t, 0, r.code, r.stderr)
	lines = strings.Split(strings.TrimSpace(r.stdout), "\n")
	assert.Equal(t, "1", strings.Fields(lines[1])[len(strings.Fields(lines[1]))-1], "wide scopes are counts")

	r = run(t, "", f.args(t, "svc", "get", "github")...)
	require.Equal(t, 0, r.code, r.stderr)
	assert.Contains(t, r.stdout, "Client Secret:         REDACTED")
}

func TestServiceCreateFromFlags(t *testing.T) {
	f := newFakeServices(t)
	r := run(t, "", f.args(t, "service", "create", "--display-name", "GitHub", "--canonical-id", "github", "--client-id", "gh", "--client-secret", "s3",
		"--oauth2-flavor", "github", "--issuer-uri", "https://github.com", "--token-endpoint", "https://github.com/t", "--authorize-endpoint", "https://github.com/a",
		"--scope", "repo=Repos", "--scope", "read:org", "--protected-resource", "https://api.github.com/", "--authorization-param", "prompt=consent")...)
	require.Equal(t, 0, r.code, r.stderr)
	req := f.last()
	assert.Equal(t, http.MethodPost, req.method)
	assert.Equal(t, "s3", req.body["client_secret"])
	assert.Equal(t, map[string]any{"enable_discovery": false}, req.body["discovery"])
	assert.Equal(t, map[string]any{"token_endpoint": "https://github.com/t", "authorize_endpoint": "https://github.com/a"}, req.body["endpoints"])
	assert.Equal(t, []any{map[string]any{"scope_value": "repo", "description": "Repos"}, map[string]any{"scope_value": "read:org", "description": ""}}, req.body["scopes"])
	assert.Equal(t, map[string]any{"prompt": "consent"}, req.body["authorization_params"])
	assert.NotContains(t, req.body, "token_endpoint_auth_method")

	r = run(t, "", f.args(t, "service", "create", "--display-name", "Pub", "--auth-method", "none", "--enable-discovery", "--issuer-uri", "https://x")...)
	require.Equal(t, 0, r.code, r.stderr)
	req = f.last()
	assert.Equal(t, "none", req.body["token_endpoint_auth_method"])
	assert.Equal(t, map[string]any{"enable_discovery": true}, req.body["discovery"])

	r = run(t, `{"display_name":"X","discovery":{"enable_discovery":false},"client_secret":"REDACTED"}`, f.args(t, "service", "create", "-f", "-")...)
	assert.Equal(t, aibctlcmd.ExitUsage, r.code)
	assert.Contains(t, r.stderr, "redacted placeholder")
}

func TestServiceUpdateUsesETag(t *testing.T) {
	f := newFakeServices(t)
	r := run(t, "", f.args(t, "service", "update", "github", "--display-name", "GH")...)
	assert.Equal(t, aibctlcmd.ExitUsage, r.code, "static secret must be re-supplied")
	assert.Contains(t, r.stderr, "--client-secret")

	r = run(t, "", f.args(t, "service", "update", "github", "--display-name", "GH", "--client-secret", "new")...)
	require.Equal(t, 0, r.code, r.stderr)
	req := f.last()
	assert.Equal(t, http.MethodPut, req.method)
	assert.Equal(t, `"3"`, req.ifMatch, "ETag from the preceding GET")
	assert.Equal(t, "GH", req.body["display_name"])
	assert.Equal(t, "new", req.body["client_secret"])
	assert.Equal(t, "github", req.body["canonical_id"])
	assert.Equal(t, []any{"https://api.github.com/"}, req.body["protected_resources"])
	assert.NotContains(t, req.body, "id")

	r = run(t, "", f.args(t, "service", "update", "github", "--client-secret", "new", "--if-match", `"2"`)...)
	assert.Equal(t, aibctlcmd.ExitConflict, r.code)
	assert.Contains(t, r.stderr, "service changed")

	r = run(t, "", f.args(t, "service", "update", "github", "--auth-method", "none", "--clear-canonical-id")...)
	require.Equal(t, 0, r.code, r.stderr)
	req = f.last()
	assert.Equal(t, "none", req.body["token_endpoint_auth_method"])
	assert.Nil(t, req.body["canonical_id"])
	assert.NotContains(t, req.body, "client_secret")

	doc := `{"display_name":"Doc","discovery":{"enable_discovery":false},"client_secret":"fresh","scopes":[]}`
	r = run(t, doc, f.args(t, "service", "update", "github", "-f", "-")...)
	require.Equal(t, 0, r.code, r.stderr)
	req = f.last()
	assert.Equal(t, `"3"`, req.ifMatch)
	assert.NotContains(t, req.body, "protected_resources", "omitted set is preserved server-side")
}

func TestServiceUpdateOmitsGeneratedCIMDClientID(t *testing.T) {
	f := newFakeServices(t)
	r := run(t, "", f.args(t, "service", "update", "cimd", "--display-name", "Renamed")...)
	require.Equal(t, 0, r.code, r.stderr)
	req := f.last()
	assert.Equal(t, http.MethodPut, req.method)
	assert.Equal(t, f.etag, req.ifMatch)
	assert.NotContains(t, req.body, "client_id")
	assert.Equal(t, "private_key_jwt", req.body["token_endpoint_auth_method"])

	r = run(t, "", f.args(t, "service", "get", "cimd", "-o", "yaml")...)
	require.Equal(t, 0, r.code, r.stderr)
	r = run(t, r.stdout, f.args(t, "service", "update", "cimd", "-f", "-")...)
	require.Equal(t, 0, r.code, r.stderr)
	assert.NotContains(t, f.last().body, "client_id")

	r = run(t, "", f.args(t, "service", "update", "github", "--auth-method", "private_key_jwt")...)
	require.Equal(t, 0, r.code, r.stderr)
	assert.NotContains(t, f.last().body, "client_id")

	r = run(t, "", f.args(t, "service", "update", "github", "--auth-method", "private_key_jwt", "--client-id", "explicit")...)
	require.Equal(t, 0, r.code, r.stderr)
	assert.Equal(t, "explicit", f.last().body["client_id"])
}

func TestProtectedResourceCommands(t *testing.T) {
	f := newFakeServices(t)
	r := run(t, "", f.args(t, "service", "pr", "list", "github")...)
	require.Equal(t, 0, r.code, r.stderr)
	assert.Equal(t, "RESOURCE\nhttps://api.github.com/\n", r.stdout)

	r = run(t, "", f.args(t, "service", "protected-resource", "add", "github", "https://x/?q=1", "https://y/existing")...)
	require.Equal(t, 0, r.code, r.stderr)
	assert.Contains(t, r.stderr, "Added https://x/.")
	assert.Contains(t, r.stderr, "was already protected")
	f.mu.Lock()
	paths := []string{f.requests[len(f.requests)-2].path, f.requests[len(f.requests)-1].path}
	f.mu.Unlock()
	assert.Equal(t, "/api/services/github/protected-resources/https:%2F%2Fx%2F%3Fq=1", paths[0])
	assert.Equal(t, "/api/services/github/protected-resources/https:%2F%2Fy%2Fexisting", paths[1])
	assert.Equal(t, "application/json", f.last().contentType, "empty PUT still declares JSON")

	r = run(t, "", f.args(t, "service", "pr", "remove", "github", "https://x/")...)
	assert.Equal(t, aibctlcmd.ExitUsage, r.code)
	r = run(t, "", f.args(t, "service", "pr", "remove", "github", "https://x/", "-y")...)
	require.Equal(t, 0, r.code, r.stderr)
	assert.Equal(t, http.MethodDelete, f.last().method)

	r = run(t, "", f.args(t, "service", "pr", "rename", "github", "https://x/", "https://z/")...)
	require.Equal(t, 0, r.code, r.stderr)
	assert.Equal(t, http.MethodPatch, f.last().method)
	assert.Equal(t, "https://z/", f.last().body["to"])
	assert.Equal(t, "RESOURCE\nhttps://z/\n", r.stdout)

	r = run(t, "", f.args(t, "__complete", "service", "pr", "remove", "github", "")...)
	require.Equal(t, 0, r.code, r.stderr)
	assert.Contains(t, r.stdout, "https://api.github.com/\n")
}
