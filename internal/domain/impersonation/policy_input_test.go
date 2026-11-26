package impersonation

import (
	"encoding/json"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
)

func samplePolicyInputs() policyInputs {
	return policyInputs{
		clientClaims:      map[string]any{"iss": "https://idp.example.com", "sub": "gateway-1", "level": float64(3)},
		actorClaims:       map[string]any{"sub": "automation-worker", "groups": []any{"ops"}},
		subjectClaims:     map[string]any{"sub": "alice@example.com", "profile": map[string]any{"dept": "inventory"}},
		subjectUnverified: false,
		request:           requestContext{GrantType: GrantType, Scope: "read", AgentID: "550e8400-e29b-41d4-a716-446655440000"}.toMap(),
		http: RequestAttributes{
			Method: "POST", Path: "/oauth2/token", RawQuery: "", Protocol: "HTTP/1.1",
			ContentType: "application/x-www-form-urlencoded",
			Headers: map[string][]string{
				"Content-Type":  {"application/x-www-form-urlencoded"},
				"X-Tenant":      {"acme", "blue"},
				"Authorization": {"Bearer synthetic"}, "Cookie": {"s=1"}, "Proxy-Authorization": {"Basic x"}, ":authority": {"broker"},
			},
			Form: map[string][]string{
				"grant_type": {GrantType}, "scope": {"read"}, "audience": {"https://broker/impersonation/x"},
				"client_assertion": {"jwt-a"}, "actor_token": {"jwt-b"}, "subject_token": {"jwt-c"}, "client_secret": {"s"},
				"refresh_token": {"r"}, "code": {"c"}, "code_verifier": {"v"}, "resource": {"", "x"},
			},
		},
		agent: &AgentContext{ID: "550e8400-e29b-41d4-a716-446655440000", CanonicalID: "inventory-assistant", ExternalID: "inventory-agent"},
	}
}

func TestBuildPolicyInput(t *testing.T) {
	publicURL, err := url.Parse("https://broker.example.com")
	require.NoError(t, err)
	input := buildPolicyInput(samplePolicyInputs(), publicURL)

	assert.Equal(t, "impersonation", input["type"])
	assert.Equal(t, map[string]any{"ext_authz": "v3", "encoding": "protojson"}, input["version"])
	assert.Equal(t, false, input["truncated_body"])
	http := input["attributes"].(map[string]any)["request"].(map[string]any)["http"].(map[string]any)
	assert.Equal(t, map[string]any{
		"method": "POST", "path": "/oauth2/token", "protocol": "HTTP/1.1", "host": "broker.example.com", "scheme": "https",
		"headers": map[string]any{"content-type": "application/x-www-form-urlencoded", "x-tenant": "acme, blue"},
	}, http)
	assert.Equal(t, []any{"oauth2", "token"}, input["parsed_path"])
	assert.Equal(t, map[string]any{}, input["parsed_query"])
	assert.Equal(t, map[string]any{
		"grant_type": []any{GrantType}, "scope": []any{"read"}, "audience": []any{"https://broker/impersonation/x"}, "resource": []any{"", "x"},
	}, input["parsed_body"], "credential-bearing parameters never reach parsed_body")

	context := input["context"].(map[string]any)
	assert.Equal(t, float64(3), context["client_assertion"].(map[string]any)["level"])
	assert.Equal(t, []any{"ops"}, context["actor_token"].(map[string]any)["groups"])
	assert.Equal(t, map[string]any{"dept": "inventory"}, context["subject_token"].(map[string]any)["profile"])
	assert.Equal(t, false, context["subject_unverified"])
	assert.Equal(t, map[string]any{"resource": "", "grant_type": GrantType, "scope": "read", "agent_id": "550e8400-e29b-41d4-a716-446655440000"}, context["request"])
	assert.Equal(t, map[string]any{"id": "550e8400-e29b-41d4-a716-446655440000", "canonical_id": "inventory-assistant", "external_id": "inventory-agent"}, context["agent"])
	for _, reserved := range []string{"principal", "resource", "grant", "permission_sets", "granted_permission_sets", "granted_permission_sets_available", "agent_session_id"} {
		assert.NotContains(t, context, reserved)
	}
	for _, absent := range []string{"mcp", "source", "destination", "id", "raw_body"} {
		assert.NotContains(t, input, absent)
	}
	assert.NotContains(t, http, "body")

	encoded, err := json.Marshal(map[string]any(input))
	require.NoError(t, err)
	for _, credential := range []string{"jwt-a", "jwt-b", "jwt-c", "Bearer synthetic", "s=1", "Basic x"} {
		assert.NotContains(t, string(encoded), credential)
	}
}

func TestBuildPolicyInputEdgeCases(t *testing.T) {
	in := samplePolicyInputs()
	in.clientClaims, in.actorClaims, in.subjectClaims = nil, nil, nil
	in.http.ContentType = "application/json"
	in.http.RawQuery = "trace=1&tag=a&tag=b"
	in.agent = agentContext(&storage.Agent{ID: id.NewAgentID()})
	input := buildPolicyInput(in, nil)

	assert.Nil(t, input["parsed_body"], "parsed_body is null for other content types")
	assert.Equal(t, map[string]any{"trace": []any{"1"}, "tag": []any{"a", "b"}}, input["parsed_query"])
	context := input["context"].(map[string]any)
	assert.Equal(t, map[string]any{}, context["client_assertion"])
	agent := context["agent"].(map[string]any)
	assert.NotContains(t, agent, "canonical_id", "unset identifiers are omitted, never empty")
	assert.NotContains(t, agent, "external_id")

	in.http.ContentType = "application/x-www-form-urlencoded; charset=utf-8"
	assert.NotNil(t, buildPolicyInput(in, nil)["parsed_body"])
}

// envoyVector is a shared broker/ExtProc input vector in tests/testdata/opa-envoy-input.
type envoyVector struct {
	Name    string `json:"name"`
	Request struct {
		Method   string            `json:"method"`
		Path     string            `json:"path"`
		RawQuery string            `json:"raw_query"`
		Headers  map[string]string `json:"headers"`
	} `json:"request"`
	Expected              map[string]any    `json:"expected"`
	ExpectedPath          map[string]string `json:"expected_path"`
	BrokerExcludedHeaders []string          `json:"broker_excluded_headers"`
}

func TestBuildPolicyInputMatchesSharedEnvoyVectors(t *testing.T) {
	paths, err := filepath.Glob(filepath.Join("..", "..", "..", "tests", "testdata", "opa-envoy-input", "*.json"))
	require.NoError(t, err)
	require.Len(t, paths, 4)
	for _, path := range paths {
		raw, err := os.ReadFile(path)
		require.NoError(t, err)
		var vector envoyVector
		require.NoError(t, json.Unmarshal(raw, &vector))
		t.Run(vector.Name, func(t *testing.T) {
			headers := map[string][]string{}
			for name, value := range vector.Request.Headers {
				headers[name] = strings.Split(value, ", ")
			}
			in := samplePolicyInputs()
			in.http = RequestAttributes{Method: vector.Request.Method, Path: vector.Request.Path, RawQuery: vector.Request.RawQuery, Headers: headers}
			input := buildPolicyInput(in, nil)
			http := input["attributes"].(map[string]any)["request"].(map[string]any)["http"].(map[string]any)

			want := vector.Expected["attributes.request.http"].(map[string]any)
			wantHeaders := want["headers"].(map[string]any)
			for _, excluded := range vector.BrokerExcludedHeaders {
				delete(wantHeaders, excluded)
			}
			assert.Equal(t, want["method"], http["method"])
			assert.Equal(t, wantHeaders, http["headers"])
			assert.Equal(t, vector.ExpectedPath["broker"], http["path"])
			assert.Equal(t, vector.Expected["parsed_path"], input["parsed_path"])
			assert.Equal(t, vector.Expected["parsed_query"], input["parsed_query"])
		})
	}
}
