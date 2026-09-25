package aib.extproc.authz

import rego.v1

default result := {"action": "deny", "reasons": ["tool is not permitted"]}

result := {"action": "allow"} if {
	input.type == "mcp_tool_call"
	input.mcp.tool_name == "list_repositories"
}

result := {
	"action": "approval_required",
	"approval_context": {
		"description": "Approval required for create_issue",
		"risk_level": "medium",
	},
} if {
	input.type == "mcp_tool_call"
	input.mcp.tool_name == "create_issue"
}
