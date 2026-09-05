package authorization

// OPAInput is the document passed to OPA for policy evaluation.
//
// The base layer is produced by envoyauth.RequestToInput from the opa-envoy-plugin
// library, which converts an ext_authz v3 CheckRequest protobuf into a map[string]any.
// This ensures full compatibility with existing opa-envoy-plugin Rego libraries
// (e.g. `import input.attributes.request.http as http_request`, `input.parsed_path`,
// `input.parsed_body`, etc.).
//
// ExtProc adds the following top-level extension keys on top of the envoy-plugin base:
//   - "type": discriminator string (mcp_tool_call, mcp_method, mcp_headers_only, unknown)
//   - "mcp": MCP protocol fields plus MCP target server information from agentgateway (MCPInput)
//   - "context": authorization context (ContextInput)
type OPAInput = map[string]any

// MCPInput contains MCP protocol fields plus MCP target server information
// extracted from agentgateway.
// Fields that are only relevant to specific request types use omitempty so that
// mcp_headers_only inputs (session_id only) do not emit empty JSON-RPC fields.
type MCPInput struct {
	JSONRPC          string         `json:"jsonrpc,omitempty"`            // "2.0" for JSON-RPC requests; absent for header-only
	Method           string         `json:"method,omitempty"`             // MCP method; absent for header-only
	ID               any            `json:"id,omitempty"`                 // JSON-RPC request ID; absent for header-only
	ToolName         string         `json:"tool_name,omitempty"`          // Tool name (only for tools/call)
	Arguments        map[string]any `json:"arguments,omitempty"`          // Tool arguments (only for tools/call)
	Params           map[string]any `json:"params,omitempty"`             // JSON-RPC params (for non-tools/call methods)
	SessionID        string         `json:"session_id,omitempty"`         // MCP session ID (from Mcp-Session-Id header)
	TargetServerName string         `json:"target_server_name,omitempty"` // agentgateway's mcp_server metadata; empty/omitted when absent
}

// ContextInput contains authorization context forwarded from the token exchange response as token-bound snapshot data.
type ContextInput struct {
	GrantedPermissionSetsAvailable bool                `json:"granted_permission_sets_available"`
	GrantedPermissionSets          map[string][]string `json:"granted_permission_sets,omitempty"`
	AgentSessionID                 string              `json:"agent_session_id,omitempty"`
}
