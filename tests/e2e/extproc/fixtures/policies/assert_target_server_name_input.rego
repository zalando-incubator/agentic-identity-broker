package aib.extproc.authz

import rego.v1

# Spec 044: asserts the shape of input.mcp.target_server_name across MCP request
# types (tool call, non-tool-call method, header-only). target_server_name is
# mandatory for MCP requests (FR-004) — ExtProc rejects absent mcp_server metadata
# with 403 before OPA ever evaluates, so no "absent" input shape reaches this policy.

allow contains {"action": "allow"} if {
	input.type == "mcp_tool_call"
	input.mcp.target_server_name == "github-mcp"
}

allow contains {"action": "allow"} if {
	input.type == "mcp_method"
	input.mcp.target_server_name == "github-mcp"
}

allow contains {"action": "allow"} if {
	input.type == "mcp_headers_only"
	input.mcp.target_server_name == "github-mcp"
}

result := decision if {
	count(allow) > 0
	decision := {"action": "allow"}
} else := {"action": "deny", "reasons": ["unexpected target_server_name input shape"]}
