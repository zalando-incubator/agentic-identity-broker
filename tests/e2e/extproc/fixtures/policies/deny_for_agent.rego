package aib.extproc.authz

import rego.v1

# Spec 055 US2: one agent may not call get_user, whichever user it acts for.

default result := {"action": "allow"}

result := {"action": "deny", "reasons": ["this agent may not look up users"]} if {
	input.type == "mcp_tool_call"
	input.mcp.tool_name == "get_user"
	input.context.agent_id == "7c9e6679-7425-40de-944b-e07fc1f90ae7"
}
