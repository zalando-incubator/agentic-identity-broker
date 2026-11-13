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

// ContextInput contains authorization context. In disabled delegation-context mode it is
// forwarded from the token exchange response as token-bound snapshot data. In enabled mode
// every member except AgentSessionID comes from a fresh broker delegation permit, and the
// five delegation members are present (ADR 045).
type ContextInput struct {
	GrantedPermissionSetsAvailable bool                   `json:"granted_permission_sets_available"`
	GrantedPermissionSets          map[string][]string    `json:"granted_permission_sets,omitempty"`
	AgentSessionID                 string                 `json:"agent_session_id,omitempty"`
	Principal                      string                 `json:"principal,omitempty"`
	Agent                          *AgentContext          `json:"agent,omitempty"`
	Resource                       *ResourceContext       `json:"resource,omitempty"`
	Grant                          *GrantContext          `json:"grant,omitempty"`
	PermissionSets                 []PermissionSetContext `json:"permission_sets,omitempty"`
}

// AgentContext identifies the delegated agent.
type AgentContext struct {
	ID          string `json:"id"`
	CanonicalID string `json:"canonical_id,omitempty"`
}

// ResourceContext describes the resolved protected resource and its effective scopes.
type ResourceContext struct {
	ID          string   `json:"id"`
	CanonicalID string   `json:"canonical_id,omitempty"`
	ResourceURI string   `json:"resource_uri"`
	Scopes      []string `json:"scopes"`
}

// GrantContext identifies the active user grant. Timestamps are the broker's RFC 3339 strings.
type GrantContext struct {
	ID         string `json:"id"`
	UpdatedAt  string `json:"updated_at"`
	ValidUntil string `json:"valid_until,omitempty"`
}

// PermissionSetContext is one granted permission set with its grant-included services.
type PermissionSetContext struct {
	ID          string           `json:"id"`
	CanonicalID string           `json:"canonical_id,omitempty"`
	Services    []ServiceContext `json:"services"`
}

// ServiceContext is a grant-included service and the permission set's scope contribution.
type ServiceContext struct {
	ID          string   `json:"id"`
	CanonicalID string   `json:"canonical_id,omitempty"`
	Scopes      []string `json:"scopes"`
}
