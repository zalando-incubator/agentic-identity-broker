package aib.extproc.authz

import rego.v1

default result := {"action": "deny", "reasons": ["tool is not permitted"]}

github_issues_service := "22222222-2222-2222-2222-222222222222"

granted(service_id) if {
	input.context.granted_permission_sets_available
	some permission_set_id
	service_id in input.context.granted_permission_sets[permission_set_id]
}

result := {"action": "allow"} if {
	input.type == "mcp_tool_call"
	input.mcp.tool_name == "list_repositories"
	granted(github_issues_service)
}

result := {"action": "approval_required", "approval_context": {"description": "Approval required for create_issue", "risk_level": "medium"}} if {
	input.type == "mcp_tool_call"
	input.mcp.tool_name == "create_issue"
	granted(github_issues_service)
}

result := {"action": "deny", "reasons": ["github-full permission set is required"]} if {
	input.type == "mcp_tool_call"
	input.mcp.tool_name == "delete_repository"
}

result := {"action": "ciba_required", "reasons": ["CIBA is deferred"]} if {
	input.type == "mcp_tool_call"
	input.mcp.tool_name == "elevated_action"
}
