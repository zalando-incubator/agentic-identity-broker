package aib.extproc.authz

import rego.v1

# Spec 055 US1: one user approves each get_user call; everyone else calls it freely.

default result := {"action": "allow"}

result := {"action": "approval_required", "approval_context": {"description": "Approve looking up a user", "risk_level": "low"}} if {
	input.type == "mcp_tool_call"
	input.mcp.tool_name == "get_user"
	input.context.principal == "alice@example.com"
}
