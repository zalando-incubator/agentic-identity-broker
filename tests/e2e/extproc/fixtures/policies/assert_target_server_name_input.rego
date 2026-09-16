package aib.extproc.authz

import rego.v1

# Spec 044: asserts the shape of input.mcp.target_server_name across MCP request
# types (tool call, non-tool-call method, header-only) and confirms the field is
# absent when agentgateway sends no mcp_server metadata.

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

allow contains {"action": "allow"} if {
	input.type == "mcp_tool_call"
	not input.mcp.target_server_name
}

result := decision if {
	count(allow) > 0
	decision := {"action": "allow"}
} else := {"action": "deny", "reasons": ["unexpected target_server_name input shape"]}
