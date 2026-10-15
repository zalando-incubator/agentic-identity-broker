package aibctlcmd_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	aibctlcmd "github.com/agentic-identity-broker/agentic-identity-broker/internal/aibctl/cmd"
)

// fakeAdmin records requests and serves canned agent/permission-set responses.
type fakeAdmin struct {
	mu       sync.Mutex
	requests []recordedRequest
	agent    map[string]any
	srv      *httptest.Server
}

type recordedRequest struct {
	method, path, prefer, contentType, ifMatch string
	body                                       map[string]any
}

func newFakeAdmin(t *testing.T) *fakeAdmin {
	t.Helper()
	canon := "research-agent"
	f := &fakeAdmin{agent: map[string]any{
		"id": "11111111-1111-1111-1111-111111111111", "canonical_id": canon, "client_id": "cli-1",
		"display_name": "Research", "description": "d",
		"permission_sets":      []any{map[string]any{"permission_set_id": "github-read", "requirement_type": "mandatory"}},
		"service_requirements": []any{map[string]any{"service_id": "github", "service_name": "GitHub", "requirement_type": "optional", "required_scopes": []any{"read"}}},
		"created_at":           "2026-01-01T00:00:00Z", "updated_at": "2026-01-02T00:00:00Z",
	}}
	f.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rec := recordedRequest{method: r.Method, path: r.URL.RequestURI(), prefer: r.Header.Get("Prefer"), contentType: r.Header.Get("Content-Type"), ifMatch: r.Header.Get("If-Match")}
		if r.Body != nil {
			_ = json.NewDecoder(r.Body).Decode(&rec.body)
		}
		f.mu.Lock()
		f.requests = append(f.requests, rec)
		f.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/agents":
			w.Header().Set("Preference-Applied", "reference-id=canonical")
			_ = json.NewEncoder(w).Encode([]any{f.agent, map[string]any{"id": "22222222-2222-2222-2222-222222222222", "canonical_id": nil, "display_name": "Plain", "description": "", "created_at": "c", "updated_at": "u"}})
		case r.Method == http.MethodGet && r.URL.Path == "/api/agents/research-agent":
			_ = json.NewEncoder(w).Encode(f.agent)
		case r.Method == http.MethodGet && r.URL.Path == "/api/agents/missing":
			w.WriteHeader(404)
			_, _ = w.Write([]byte(`{"error":"not_found","message":"agent not found"}`))
		case r.Method == http.MethodPost && r.URL.Path == "/api/agents":
			w.WriteHeader(201)
			_ = json.NewEncoder(w).Encode(f.agent)
		case r.Method == http.MethodPut && r.URL.Path == "/api/agents/research-agent":
			_ = json.NewEncoder(w).Encode(f.agent)
		case r.Method == http.MethodDelete && r.URL.Path == "/api/agents/research-agent":
			w.WriteHeader(204)
		case r.Method == http.MethodGet && r.URL.Path == "/api/permission-sets":
			_ = json.NewEncoder(w).Encode(map[string]any{"items": []any{map[string]any{"id": "33333333-3333-3333-3333-333333333333", "canonical_id": "github-read", "name": "GitHub Read", "description": "", "service_scopes": []any{map[string]any{"service_id": "github", "scopes": []any{"read"}, "requirement_type": "mandatory"}}, "created_at": "c", "updated_at": "u"}}})
		case r.Method == http.MethodPost && r.URL.Path == "/api/permission-sets":
			w.WriteHeader(201)
			_ = json.NewEncoder(w).Encode(map[string]any{"id": "33333333-3333-3333-3333-333333333333", "canonical_id": rec.body["canonical_id"], "name": rec.body["name"], "description": "", "service_scopes": rec.body["service_scopes"], "created_at": "c", "updated_at": "u"})
		case r.Method == http.MethodDelete && r.URL.Path == "/api/permission-sets/github-read":
			w.WriteHeader(409)
			_, _ = w.Write([]byte(`{"error":"conflict","message":"permission set is referenced by an agent"}`))
		default:
			w.WriteHeader(500)
			_, _ = w.Write([]byte(`{"error":"unexpected","message":"` + r.Method + " " + r.URL.Path + `"}`))
		}
	}))
	t.Cleanup(f.srv.Close)
	return f
}

func (f *fakeAdmin) args(t *testing.T, extra ...string) []string {
	t.Helper()
	return append([]string{"--config", configPath(t), "--server", f.srv.URL, "--principal", "admin@example.com"}, extra...)
}

func (f *fakeAdmin) last() recordedRequest {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.requests[len(f.requests)-1]
}

func TestAgentList(t *testing.T) {
	f := newFakeAdmin(t)
	r := run(t, "", f.args(t, "agent", "list")...)
	require.Equal(t, 0, r.code, r.stderr)
	assert.Equal(t, "reference-id=canonical", f.last().prefer)
	lines := strings.Split(strings.TrimSpace(r.stdout), "\n")
	require.Len(t, lines, 3)
	assert.Equal(t, []string{"ID", "DISPLAY_NAME", "CLIENT_ID", "PERMISSION_SETS", "SERVICES", "UPDATED"}, strings.Fields(lines[0]))
	assert.Equal(t, []string{"research-agent", "Research", "cli-1", "1", "1", "2026-01-02T00:00:00Z"}, strings.Fields(lines[1]))
	assert.Equal(t, []string{"22222222-2222-2222-2222-222222222222", "Plain", "-", "0", "0", "u"}, strings.Fields(lines[2]))

	r = run(t, "", f.args(t, "agents", "ls", "--show-uuid", "--no-headers")...)
	require.Equal(t, 0, r.code, r.stderr)
	assert.NotContains(t, r.stdout, "DISPLAY_NAME")
	lines = strings.Split(strings.TrimSpace(r.stdout), "\n")
	require.Len(t, lines, 2)
	assert.Equal(t, []string{"research-agent", "Research", "cli-1", "1", "1", "2026-01-02T00:00:00Z", "11111111-1111-1111-1111-111111111111"}, strings.Fields(lines[0]))

	r = run(t, "", f.args(t, "agent", "list", "-o", "wide")...)
	require.Equal(t, 0, r.code, r.stderr)
	lines = strings.Split(strings.TrimSpace(r.stdout), "\n")
	assert.Equal(t, []string{"research-agent", "Research", "cli-1", "1", "1", "2026-01-02T00:00:00Z", "11111111-1111-1111-1111-111111111111", "-", "0", "0"}, strings.Fields(lines[1]))

	r = run(t, "", f.args(t, "agent", "list", "-o", "json")...)
	require.Equal(t, 0, r.code, r.stderr)
	var parsed []map[string]any
	require.NoError(t, json.Unmarshal([]byte(r.stdout), &parsed))
	assert.Len(t, parsed, 2)
}

func TestAgentGet(t *testing.T) {
	f := newFakeAdmin(t)
	r := run(t, "", f.args(t, "agent", "get", "research-agent")...)
	require.Equal(t, 0, r.code, r.stderr)
	assert.Contains(t, r.stdout, "ID:                      research-agent\n")
	assert.Contains(t, r.stdout, "UUID:                    11111111-1111-1111-1111-111111111111\n")
	assert.Contains(t, r.stdout, "Service Requirements:    github (optional: read)\n")

	r = run(t, "", f.args(t, "agent", "get", "missing")...)
	assert.Equal(t, aibctlcmd.ExitNotFound, r.code)
	assert.Contains(t, r.stderr, "Error: agent not found (not_found, HTTP 404)")

	r = run(t, "", f.args(t, "agent", "get")...)
	assert.Equal(t, aibctlcmd.ExitUsage, r.code)
}

func TestAgentCreateFromFlags(t *testing.T) {
	f := newFakeAdmin(t)
	r := run(t, "", f.args(t, "agent", "create", "--display-name", "Research", "--description", "d", "--canonical-id", "research-agent",
		"--permission-set", "github-read:mandatory", "--permission-set", "other", "--service-requirement", "github=read,user:email:mandatory", "--service-requirement-all", "jira",
		"--redirect-uri", "https://a/cb", "--external-id", "ext-1")...)
	require.Equal(t, 0, r.code, r.stderr)
	assert.Contains(t, r.stderr, `Agent "research-agent" created.`)
	req := f.last()
	assert.Equal(t, http.MethodPost, req.method)
	assert.Equal(t, "application/json", req.contentType)
	assert.Equal(t, "research-agent", req.body["canonical_id"])
	assert.Equal(t, "ext-1", req.body["external_id"])
	assert.NotContains(t, req.body, "client_id")
	assert.Equal(t, []any{map[string]any{"permission_set_id": "github-read", "requirement_type": "mandatory"}, map[string]any{"permission_set_id": "other", "requirement_type": "optional"}}, req.body["permission_sets"])
	assert.Equal(t, []any{map[string]any{"service_id": "github", "requirement_type": "mandatory", "required_scopes": []any{"read", "user:email"}}, map[string]any{"service_id": "jira", "requirement_type": "optional", "required_scopes": []any{}, "require_all_scopes": true}}, req.body["service_requirements"])
	assert.Equal(t, []any{"https://a/cb"}, req.body["redirect_uris"])
}

func TestAgentCreateValidation(t *testing.T) {
	f := newFakeAdmin(t)
	r := run(t, "", f.args(t, "agent", "create")...)
	assert.Equal(t, aibctlcmd.ExitUsage, r.code)
	assert.Contains(t, r.stderr, "--file or field flags")

	r = run(t, "", f.args(t, "agent", "create", "-f", "-", "--display-name", "x")...)
	assert.Equal(t, aibctlcmd.ExitUsage, r.code)
	assert.Contains(t, r.stderr, "--file cannot be combined with --display-name")

	r = run(t, "", f.args(t, "agent", "create", "--display-name", "x", "--permission-set", "ps:sometimes")...)
	assert.Equal(t, aibctlcmd.ExitUsage, r.code)
	assert.Contains(t, r.stderr, "mandatory or optional")

	r = run(t, "", f.args(t, "agent", "create", "--file", "")...)
	assert.Equal(t, aibctlcmd.ExitUsage, r.code)
	assert.Contains(t, r.stderr, `--file needs a path, or "-" for stdin`)
	f.mu.Lock()
	assert.Empty(t, f.requests)
	f.mu.Unlock()
}

func TestAgentCreateFromFile(t *testing.T) {
	f := newFakeAdmin(t)
	path := filepath.Join(t.TempDir(), "agent.yaml")
	require.NoError(t, os.WriteFile(path, []byte("display_name: Research\ndescription: d\ncanonical_id: research-agent\nclient_id: null\npermission_sets:\n  - permission_set_id: github-read\n    requirement_type: mandatory\n"), 0o600))
	r := run(t, "", f.args(t, "agent", "create", "-f", path, "-o", "json")...)
	require.Equal(t, 0, r.code, r.stderr)
	req := f.last()
	assert.Equal(t, "research-agent", req.body["canonical_id"])
	assert.Contains(t, req.body, "client_id")
	assert.Nil(t, req.body["client_id"])
	assert.Contains(t, r.stdout, `"canonical_id": "research-agent"`)
}

func TestAgentUpdateThreeState(t *testing.T) {
	f := newFakeAdmin(t)

	r := run(t, "", f.args(t, "agent", "update", "research-agent", "--display-name", "Renamed")...)
	require.Equal(t, 0, r.code, r.stderr)
	req := f.last()
	assert.Equal(t, http.MethodPut, req.method)
	assert.Equal(t, "Renamed", req.body["display_name"])
	assert.Equal(t, "research-agent", req.body["canonical_id"], "fetched value is kept")
	assert.Equal(t, "cli-1", req.body["client_id"])
	assert.NotContains(t, req.body, "id")
	sr := req.body["service_requirements"].([]any)[0].(map[string]any)
	assert.NotContains(t, sr, "service_name")

	r = run(t, "", f.args(t, "agent", "update", "research-agent", "--clear-canonical-id", "--clear-client-id")...)
	require.Equal(t, 0, r.code, r.stderr)
	req = f.last()
	assert.Contains(t, req.body, "canonical_id")
	assert.Nil(t, req.body["canonical_id"])
	assert.Nil(t, req.body["client_id"])

	r = run(t, "", f.args(t, "agent", "update", "research-agent", "--canonical-id", "x", "--clear-canonical-id")...)
	assert.Equal(t, aibctlcmd.ExitUsage, r.code)

	r = run(t, `{"display_name":"Doc","description":"d","permission_sets":[{"permission_set_id":"github-read","requirement_type":"optional"}]}`, f.args(t, "agent", "update", "research-agent", "-f", "-")...)
	require.Equal(t, 0, r.code, r.stderr)
	req = f.last()
	assert.Equal(t, "Doc", req.body["display_name"])
	assert.NotContains(t, req.body, "canonical_id", "omitted key keeps the canonical id")
}

func TestAgentUpdatePreservesRequireAllScopes(t *testing.T) {
	f := newFakeAdmin(t)
	f.agent["service_requirements"] = []any{map[string]any{
		"service_id": "github", "service_name": "GitHub", "requirement_type": "mandatory",
		"required_scopes": []any{}, "require_all_scopes": true,
	}}
	r := run(t, "", f.args(t, "agent", "update", "research-agent", "--display-name", "X")...)
	require.Equal(t, 0, r.code, r.stderr)
	reqs := f.last().body["service_requirements"].([]any)
	assert.Equal(t, true, reqs[0].(map[string]any)["require_all_scopes"])
	assert.Equal(t, []any{}, reqs[0].(map[string]any)["required_scopes"])
	r = run(t, "", f.args(t, "agent", "get", "research-agent")...)
	require.Equal(t, 0, r.code, r.stderr)
	assert.Contains(t, r.stdout, "Service Requirements:    github (mandatory: all scopes)")
}

func TestAgentDelete(t *testing.T) {
	f := newFakeAdmin(t)
	r := run(t, "", f.args(t, "agent", "delete", "research-agent")...)
	assert.Equal(t, aibctlcmd.ExitUsage, r.code)
	assert.Contains(t, r.stderr, "--yes")

	r = run(t, "", f.args(t, "agent", "rm", "research-agent", "-y")...)
	require.Equal(t, 0, r.code, r.stderr)
	assert.Equal(t, http.MethodDelete, f.last().method)
	assert.Empty(t, r.stdout)
	assert.Contains(t, r.stderr, "deleted")

	t.Setenv("AIBCTL_ASSUME_YES", "1")
	r = run(t, "", f.args(t, "agent", "delete", "research-agent")...)
	require.Equal(t, 0, r.code, r.stderr)
}

func TestPermissionSetCommands(t *testing.T) {
	f := newFakeAdmin(t)
	r := run(t, "", f.args(t, "ps", "list", "--service", "github")...)
	require.Equal(t, 0, r.code, r.stderr)
	assert.Equal(t, "/api/permission-sets?service_id=github", f.last().path)
	assert.Equal(t, []string{"github-read", "GitHub", "Read", "1", "1", "u"}, strings.Fields(strings.Split(r.stdout, "\n")[1]))

	r = run(t, "", f.args(t, "permission-set", "create", "--name", "GitHub Read", "--description", "d", "--canonical-id", "github-read", "--service-scope", "github=read:org,repo:mandatory")...)
	require.Equal(t, 0, r.code, r.stderr)
	req := f.last()
	assert.Equal(t, []any{map[string]any{"service_id": "github", "scopes": []any{"read:org", "repo"}, "requirement_type": "mandatory"}}, req.body["service_scopes"])
	assert.Contains(t, r.stdout, "ID:              github-read")

	r = run(t, "", f.args(t, "permission-set", "delete", "github-read", "--yes")...)
	assert.Equal(t, aibctlcmd.ExitConflict, r.code)
	assert.Contains(t, r.stderr, "referenced by an agent")
}

func TestAgentIDCompletion(t *testing.T) {
	f := newFakeAdmin(t)
	r := run(t, "", f.args(t, "__complete", "agent", "get", "")...)
	require.Equal(t, 0, r.code, r.stderr)
	assert.Contains(t, r.stdout, "research-agent\tResearch\n")
	assert.Contains(t, r.stdout, "22222222-2222-2222-2222-222222222222\tPlain\n")
	assert.Contains(t, r.stdout, ":4\n", "no file completion directive")

	r = run(t, "", f.args(t, "__complete", "agent", "get", "res")...)
	assert.Contains(t, r.stdout, "research-agent\tResearch\n")
	assert.NotContains(t, r.stdout, "Plain")

	r = run(t, "", f.args(t, "__complete", "agent", "create", "--permission-set", "")...)
	assert.Contains(t, r.stdout, "github-read\tGitHub Read\n")

	r = run(t, "", "--config", configPath(t), "--server", "http://127.0.0.1:1", "--principal", "p", "__complete", "agent", "get", "")
	require.Equal(t, 0, r.code)
	assert.NotContains(t, r.stdout, "research-agent")
	assert.Contains(t, r.stdout, ":5\n", "error directive without stdout noise")
}
