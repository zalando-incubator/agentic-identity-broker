package aib.extproc.authz

import rego.v1

# Spec 044: demonstrates a policy that scopes decisions per MCP server using
# input.mcp.target_server_name — agentgateway routing metadata, not an MCP
# protocol field. Only "github-mcp" is authorized; all other target servers
# (including requests with no target server at all) are denied.

allowed_server := "github-mcp"

allow contains {"action": "allow", "reason": "target server is authorized"} if {
	input.type == "mcp_tool_call"
	input.mcp.target_server_name == allowed_server
}

deny contains {"action": "deny", "reason": "target server is not authorized"} if {
	input.type == "mcp_tool_call"
	input.mcp.target_server_name != allowed_server
}

result := decision if {
	count(deny) > 0
	decision := {
		"action": "deny",
		"reasons": [r | some d in deny; r := d.reason],
	}
} else := decision if {
	count(allow) > 0
	decision := {"action": "allow"}
} else := {"action": "deny", "reasons": ["default deny: no matching allow rule"]}
