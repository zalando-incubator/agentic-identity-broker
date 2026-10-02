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
//   - "mcp": MCP protocol fields plus MCP target server information from agentgateway
//   - "context": authorization context (ContextInput)
type OPAInput = map[string]any

// ContextInput contains authorization context forwarded from the token exchange response as token-bound snapshot data.
type ContextInput struct {
	GrantedPermissionSetsAvailable bool                `json:"granted_permission_sets_available"`
	GrantedPermissionSets          map[string][]string `json:"granted_permission_sets,omitempty"`
	AgentSessionID                 string              `json:"agent_session_id,omitempty"`
}
