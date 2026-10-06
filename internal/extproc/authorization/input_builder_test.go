package authorization_test

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/extproc/authorization"
)

// TestBuildOPAInput_ToolsCall_ValidName verifies that a well-formed tools/call
// message produces type="mcp_tool_call" with the correct tool name.
func TestBuildOPAInput_ToolsCall_ValidName(t *testing.T) {
	body := mustMarshal(t, map[string]any{
		"jsonrpc": "2.0",
		"method":  "tools/call",
		"id":      1,
		"params": map[string]any{
			"name":      "list_files",
			"arguments": map[string]any{"path": "/tmp"},
		},
	})

	input, err := buildOPAInput(t, "mcp", body, map[string]string{}, testTargetServerName, authorization.ContextInput{})
	require.NoError(t, err)
	assert.Equal(t, "mcp_tool_call", input["type"])

	mcp, ok := input["mcp"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "list_files", mcp["tool_name"])
	assert.Equal(t, map[string]any{"path": "/tmp"}, mcp["arguments"])
}

// TestBuildOPAInput_ToolsCall_MissingParams verifies that tools/call with absent
// params returns an error (deny path — FR-003 strict validation).
func TestBuildOPAInput_ToolsCall_MissingParams(t *testing.T) {
	body := mustMarshal(t, map[string]any{
		"jsonrpc": "2.0",
		"method":  "tools/call",
		"id":      1,
	})

	_, err := buildOPAInput(t, "mcp", body, map[string]string{}, testTargetServerName, authorization.ContextInput{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "tools/call missing params")
}

// TestBuildOPAInput_ToolsCall_EmptyName verifies that tools/call with an empty
// params.name returns an error.
func TestBuildOPAInput_ToolsCall_EmptyName(t *testing.T) {
	body := mustMarshal(t, map[string]any{
		"jsonrpc": "2.0",
		"method":  "tools/call",
		"id":      1,
		"params":  map[string]any{"name": ""},
	})

	_, err := buildOPAInput(t, "mcp", body, map[string]string{}, testTargetServerName, authorization.ContextInput{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "tools/call missing or invalid params.name")
}

// TestBuildOPAInput_ToolsCall_NonStringName verifies that tools/call with a
// non-string params.name (e.g. integer) returns an error.
func TestBuildOPAInput_ToolsCall_NonStringName(t *testing.T) {
	body := mustMarshal(t, map[string]any{
		"jsonrpc": "2.0",
		"method":  "tools/call",
		"id":      1,
		"params":  map[string]any{"name": 42},
	})

	_, err := buildOPAInput(t, "mcp", body, map[string]string{}, testTargetServerName, authorization.ContextInput{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "tools/call missing or invalid params.name")
}

// TestBuildOPAInput_Initialize_MethodShape verifies that initialize requests produce
// type="mcp_method" with the parsed method and params preserved.
func TestBuildOPAInput_Initialize_MethodShape(t *testing.T) {
	body := mustMarshal(t, map[string]any{
		"jsonrpc": "2.0",
		"method":  "initialize",
		"id":      1,
		"params": map[string]any{
			"protocolVersion": "2025-03-26",
			"clientInfo": map[string]any{
				"name":    "test-agent",
				"version": "1.0",
			},
		},
	})

	input, err := buildOPAInput(t, "mcp", body, map[string]string{}, testTargetServerName, authorization.ContextInput{})
	require.NoError(t, err)
	assert.Equal(t, "mcp_method", input["type"])

	mcp, ok := input["mcp"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "initialize", mcp["method"])
	params, ok := mcp["params"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "2025-03-26", params["protocolVersion"])
}

func TestBuildOPAInput_RejectsCaseFoldedEnvelopeKeys(t *testing.T) {
	for _, tc := range []struct {
		name      string
		duplicate string
		value     any
	}{
		{name: "jsonrpc", duplicate: "JSONRPC", value: "2.0"},
		{name: "method", duplicate: "Method", value: "tools/call"},
		{name: "id", duplicate: "ID", value: 2},
		{name: "params", duplicate: "Params", value: map[string]any{"name": "delete_repository"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			message := map[string]any{"jsonrpc": "2.0", "method": "initialize", "id": 1, "params": map[string]any{}}
			message[tc.duplicate] = tc.value
			_, err := buildOPAInput(t, "mcp", mustMarshal(t, message), nil, testTargetServerName, authorization.ContextInput{})
			require.ErrorContains(t, err, "duplicate "+tc.name+" field")
		})
	}
}

// TestBuildOPAInput_UnknownProtocol_PreservesRawBody verifies that unrecognized
// protocols keep the request as type="unknown" and expose the raw body string.
func TestBuildOPAInput_UnknownProtocol_PreservesRawBody(t *testing.T) {
	body := []byte(`{"action":"run","tool":"bash","command":"ls -la"}`)
	headers := map[string]string{
		":method":    "POST",
		":path":      "/rpc",
		":scheme":    "https",
		":authority": "example.com",
	}

	input, err := buildOPAInput(t, "some-unknown-protocol", body, headers, "", authorization.ContextInput{})
	require.NoError(t, err)
	assert.Equal(t, "unknown", input["type"])

	attrs, ok := input["attributes"].(map[string]any)
	require.True(t, ok)
	requestAttrs, ok := attrs["request"].(map[string]any)
	require.True(t, ok)
	httpAttrs, ok := requestAttrs["http"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, string(body), httpAttrs["body"])
	_, hasRequest := input["request"]
	assert.False(t, hasRequest, "OPA input must not duplicate HTTP request fields at top level")
	_, hasMCP := input["mcp"]
	assert.False(t, hasMCP, "unknown inputs must not fabricate an mcp payload")
}

// TestBuildOPAInputHeadersOnly_MCPShape_SessionIDOnly verifies mcp_headers_only
// serializes only session_id and target_server_name — no empty JSON-RPC fields.
func TestBuildOPAInputHeadersOnly_MCPShape_SessionIDOnly(t *testing.T) {
	headers := map[string]string{
		":method":        "GET",
		":path":          "/mcp",
		":scheme":        "https",
		":authority":     "example.com",
		"mcp-session-id": "sess-abc123",
	}

	input, err := authorization.BuildOPAInputHeadersOnly("mcp", headers, testTargetServerName)
	require.NoError(t, err)
	assert.Equal(t, "mcp_headers_only", input["type"])

	mcp, ok := input["mcp"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, map[string]any{"session_id": "sess-abc123", "target_server_name": testTargetServerName}, mcp,
		"mcp_headers_only mcp object must contain only session_id and target_server_name")
}

// TestBuildOPAInputHeadersOnly_MCPShape_NoSessionID verifies mcp_headers_only omits
// session_id and JSON-RPC fields but still carries target_server_name.
func TestBuildOPAInputHeadersOnly_MCPShape_NoSessionID(t *testing.T) {
	headers := map[string]string{
		":method":    "GET",
		":path":      "/mcp",
		":scheme":    "https",
		":authority": "example.com",
	}

	input, err := authorization.BuildOPAInputHeadersOnly("mcp", headers, testTargetServerName)
	require.NoError(t, err)
	assert.Equal(t, "mcp_headers_only", input["type"])

	mcp, ok := input["mcp"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, map[string]any{"target_server_name": testTargetServerName}, mcp,
		"mcp object must contain only target_server_name when no session_id is present")
}

// TestBuildOPAInput_GrantedPermissionSets_Propagated verifies that non-nil
// grantedPermissionSets is forwarded into context.granted_permission_sets and marked available.
func TestBuildOPAInput_GrantedPermissionSets_Propagated(t *testing.T) {
	body := mustMarshal(t, map[string]any{
		"jsonrpc": "2.0",
		"method":  "tools/call",
		"id":      1,
		"params": map[string]any{
			"name":      "list_files",
			"arguments": map[string]any{},
		},
	})

	gps := map[string][]string{
		"perm-set-uuid-1": {"svc-a", "svc-b"},
		"perm-set-uuid-2": {"svc-c"},
	}

	input, err := buildOPAInput(t, "mcp", body, map[string]string{}, testTargetServerName, authorization.ContextInput{GrantedPermissionSets: gps, GrantedPermissionSetsAvailable: gps != nil})
	require.NoError(t, err)

	ctx, ok := input["context"].(authorization.ContextInput)
	require.True(t, ok, "context must be ContextInput")
	assert.True(t, ctx.GrantedPermissionSetsAvailable,
		"non-nil grantedPermissionSets must mark the snapshot as available")
	assert.Equal(t, gps, ctx.GrantedPermissionSets,
		"granted_permission_sets must be propagated from exchange result to OPA context")

	raw, err := json.Marshal(ctx)
	require.NoError(t, err)
	var ctxMap map[string]any
	require.NoError(t, json.Unmarshal(raw, &ctxMap))
	assert.Equal(t, true, ctxMap["granted_permission_sets_available"])
	assert.Contains(t, ctxMap, "granted_permission_sets")
}

// TestBuildOPAInput_GrantedPermissionSets_EmptySnapshotStillAvailable verifies that
// an authoritative empty snapshot remains distinguishable from unavailable context.
func TestBuildOPAInput_GrantedPermissionSets_EmptySnapshotStillAvailable(t *testing.T) {
	body := mustMarshal(t, map[string]any{
		"jsonrpc": "2.0",
		"method":  "tools/call",
		"id":      1,
		"params":  map[string]any{"name": "list_files", "arguments": map[string]any{}},
	})

	input, err := buildOPAInput(t, "mcp", body, map[string]string{}, testTargetServerName, authorization.ContextInput{GrantedPermissionSets: map[string][]string{}, GrantedPermissionSetsAvailable: map[string][]string{} != nil})
	require.NoError(t, err)

	ctx, ok := input["context"].(authorization.ContextInput)
	require.True(t, ok)
	assert.True(t, ctx.GrantedPermissionSetsAvailable,
		"an empty but present snapshot must still be marked available")
	assert.Empty(t, ctx.GrantedPermissionSets)

	raw, err := json.Marshal(ctx)
	require.NoError(t, err)
	var ctxMap map[string]any
	require.NoError(t, json.Unmarshal(raw, &ctxMap))
	assert.Equal(t, true, ctxMap["granted_permission_sets_available"])
	assert.NotContains(t, ctxMap, "granted_permission_sets",
		"omitempty drops empty authoritative snapshots, so availability must carry the distinction")
}

// TestBuildOPAInput_GrantedPermissionSets_NilUnavailable verifies that omitted
// broker data is marked unavailable and the granted_permission_sets field is omitted.
func TestBuildOPAInput_GrantedPermissionSets_NilUnavailable(t *testing.T) {
	body := mustMarshal(t, map[string]any{
		"jsonrpc": "2.0",
		"method":  "tools/call",
		"id":      1,
		"params":  map[string]any{"name": "list_files", "arguments": map[string]any{}},
	})

	input, err := buildOPAInput(t, "mcp", body, map[string]string{}, testTargetServerName, authorization.ContextInput{})
	require.NoError(t, err)

	ctx, ok := input["context"].(authorization.ContextInput)
	require.True(t, ok)
	assert.False(t, ctx.GrantedPermissionSetsAvailable,
		"nil grantedPermissionSets must mark the snapshot as unavailable")
	assert.Nil(t, ctx.GrantedPermissionSets,
		"unavailable granted_permission_sets must be omitted rather than normalised to {}")

	raw, err := json.Marshal(ctx)
	require.NoError(t, err)
	var ctxMap map[string]any
	require.NoError(t, json.Unmarshal(raw, &ctxMap))
	assert.Equal(t, false, ctxMap["granted_permission_sets_available"])
	assert.NotContains(t, ctxMap, "granted_permission_sets")
}

// TestBuildOPAInputHeadersOnly_GrantedPermissionSetsUnavailable verifies that
// header-only requests mark granted permission sets unavailable and omit the field.
func TestBuildOPAInputHeadersOnly_GrantedPermissionSetsUnavailable(t *testing.T) {
	headers := map[string]string{
		":method":    "GET",
		":path":      "/mcp",
		":scheme":    "https",
		":authority": "example.com",
	}

	input, err := authorization.BuildOPAInputHeadersOnly("mcp", headers, testTargetServerName)
	require.NoError(t, err)

	ctx, ok := input["context"].(authorization.ContextInput)
	require.True(t, ok)
	assert.False(t, ctx.GrantedPermissionSetsAvailable,
		"header-only requests must evaluate before token exchange with unavailable permission context")
	assert.Nil(t, ctx.GrantedPermissionSets,
		"header-only requests must omit granted_permission_sets rather than using {}")

	raw, err := json.Marshal(ctx)
	require.NoError(t, err)
	var ctxMap map[string]any
	require.NoError(t, json.Unmarshal(raw, &ctxMap))
	assert.Equal(t, false, ctxMap["granted_permission_sets_available"])
	assert.NotContains(t, ctxMap, "granted_permission_sets")
}

func TestBuildOPAInput_PreservesAgentSessionContext(t *testing.T) {
	body := mustMarshal(t, map[string]any{"jsonrpc": "2.0", "method": "tools/call", "id": 1, "params": map[string]any{"name": "list_files", "arguments": map[string]any{}}})
	input, err := buildOPAInput(t, "mcp", body, map[string]string{"Mcp-Session-Id": "mcp-session"}, testTargetServerName, authorization.ContextInput{AgentSessionID: "agent-session"})
	require.NoError(t, err)
	contextInput, ok := input["context"].(authorization.ContextInput)
	require.True(t, ok)
	assert.Equal(t, "agent-session", contextInput.AgentSessionID)
	mcpInput, ok := input["mcp"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "mcp-session", mcpInput["session_id"])
}

func mustMarshal(t *testing.T, v any) []byte {
	t.Helper()
	b, err := json.Marshal(v)
	require.NoError(t, err)
	return b
}

func buildOPAInput(t *testing.T, protocol string, body []byte, headers map[string]string, targetServerName string, contextInput authorization.ContextInput) (authorization.OPAInput, error) {
	t.Helper()
	builder, err := authorization.NewInputBuilder(protocol, headers, targetServerName, contextInput)
	if err != nil {
		return nil, err
	}
	var parsed any
	if len(body) != 0 {
		decoder := json.NewDecoder(bytes.NewReader(body))
		decoder.UseNumber()
		require.NoError(t, decoder.Decode(&parsed))
	}
	return builder.Build(body, parsed)
}

// noTargetServerName covers the empty target_server_name path in the input builder.
const noTargetServerName = ""

// testTargetServerName is a realistic value for tests unrelated to
// target_server_name itself, since it's mandatory for MCP requests (FR-004).
const testTargetServerName = "test-mcp-server"

// TestBuildOPAInput_TargetServerName_Propagated verifies that a non-empty
// targetServerName populates mcp.target_server_name for tools/call requests.
func TestBuildOPAInput_TargetServerName_Propagated(t *testing.T) {
	body := mustMarshal(t, map[string]any{
		"jsonrpc": "2.0",
		"method":  "tools/call",
		"id":      1,
		"params":  map[string]any{"name": "list_files", "arguments": map[string]any{}},
	})

	input, err := buildOPAInput(t, "mcp", body, map[string]string{}, "github-mcp", authorization.ContextInput{})
	require.NoError(t, err)

	mcp, ok := input["mcp"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "github-mcp", mcp["target_server_name"])
}

// TestBuildOPAInput_TargetServerName_AbsentOmitted verifies the builder omits an
// empty targetServerName. Production rejects absent MCP metadata before building input.
func TestBuildOPAInput_TargetServerName_AbsentOmitted(t *testing.T) {
	body := mustMarshal(t, map[string]any{
		"jsonrpc": "2.0",
		"method":  "initialize",
		"id":      1,
		"params":  map[string]any{},
	})

	input, err := buildOPAInput(t, "mcp", body, map[string]string{}, noTargetServerName, authorization.ContextInput{})
	require.NoError(t, err)

	mcp, ok := input["mcp"].(map[string]any)
	require.True(t, ok)
	assert.NotContains(t, mcp, "target_server_name")
}

// TestBuildOPAInput_TargetServerName_NotSetForNonMCPProtocol verifies that
// targetServerName is never surfaced for non-MCP protocols, since agentgateway's
// mcp_server metadata is only meaningful alongside protocol="mcp".
func TestBuildOPAInput_TargetServerName_NotSetForNonMCPProtocol(t *testing.T) {
	body := []byte(`{"action":"run"}`)

	input, err := buildOPAInput(t, "a2a", body, map[string]string{}, "github-mcp", authorization.ContextInput{})
	require.NoError(t, err)

	assert.Equal(t, "unknown", input["type"])
	_, hasMCP := input["mcp"]
	assert.False(t, hasMCP, "non-MCP protocols must not fabricate an mcp payload even when targetServerName is set")
}

// TestBuildOPAInputHeadersOnly_TargetServerName_Propagated verifies that
// header-only MCP requests carry target_server_name alongside session_id.
func TestBuildOPAInputHeadersOnly_TargetServerName_Propagated(t *testing.T) {
	headers := map[string]string{
		":method":        "GET",
		":path":          "/mcp",
		":scheme":        "https",
		":authority":     "example.com",
		"mcp-session-id": "sess-abc123",
	}

	input, err := authorization.BuildOPAInputHeadersOnly("mcp", headers, "github-mcp")
	require.NoError(t, err)

	mcp, ok := input["mcp"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "sess-abc123", mcp["session_id"])
	assert.Equal(t, "github-mcp", mcp["target_server_name"])
}
