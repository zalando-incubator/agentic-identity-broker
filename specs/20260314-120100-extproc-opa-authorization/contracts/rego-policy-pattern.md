# Contract: Example Rego Policy

**Branch**: `20260314-120100-extproc-opa-authorization`
**Date**: 2026-03-14

## Recommended Policy Pattern

The recommended policy pattern uses **set-based `allow` and `deny` helper rules** with a single `result` aggregation rule. This avoids Rego v1 complete-rule conflicts — multiple helper rules can match the same input without colliding. Deny always wins over allow.

ExtProc reads exactly one document: `data.aib.extproc.authz.result`. The `allow` and `deny` rules are policy-local composition helpers, not alternate decision documents.

### Package and Imports

```rego
package aib.extproc.authz

import rego.v1
```

### Allow/Deny Rule Sets

Policy authors add rules to the `allow` and `deny` helper sets. Each rule typically produces an object with a `reason` field so the final `result.reasons` array can explain why access was denied.

```rego
# Allow MCP lifecycle methods (safe, no tool execution)
allow contains {"reason": "safe MCP lifecycle method"} if {
    input.type == "mcp_method"
    input.mcp.method in {"initialize", "ping", "notifications/initialized"}
}

# Allow read-only MCP tool calls
allow contains {"reason": "read-only tool"} if {
    input.type == "mcp_tool_call"
    input.mcp.tool_name in {"list_repositories", "get_file_contents", "search_code"}
}

# Deny destructive tool calls
deny contains {"reason": "destructive operations are not permitted"} if {
    input.type == "mcp_tool_call"
    input.mcp.tool_name in {"delete_repository", "force_push"}
}

# Allow all non-MCP traffic (passthrough)
allow contains {"reason": "non-MCP passthrough"} if {
    input.type == "unknown"
}
```

### Aggregation Rule

The `result` rule aggregates allow/deny sets. Deny wins over allow. If nothing matches, default deny applies.

```rego
# Deny wins if any deny rule fired
result := {"action": "deny", "reasons": _deny_reasons} if {
    count(deny) > 0
    _deny_reasons := [r | some entry in deny; r := entry.reason]
}

# Allow if no deny and at least one allow
result := {"action": "allow"} if {
    count(deny) == 0
    count(allow) > 0
}

# Default: deny (fail closed) — no rule matched
default result := {"action": "deny", "reasons": ["no policy rule matched"]}
```

## ExtProc Reads the `result` Document

ExtProc queries the OPA SDK with path `/aib/extproc/authz/result` (package dots become path separators, plus the decision document name).

The `result` document is always a JSON object with:
- `action` (string): `"allow"` or `"deny"` (or future `"approval_required"`, `"ciba_required"`)
- `reasons` (array of strings, optional): Present when action is `"deny"`


## Permission-Set-Aware Rule Example

Policies can also enforce token-bound permission-set context from `input.context.granted_permission_sets`, but they must gate on `input.context.granted_permission_sets_available` rather than assuming the map is present.

```rego
required_service_id := "22222222-2222-2222-2222-222222222222"

permission_set_context_available if {
    input.context.granted_permission_sets_available
}

permission_set_grants_service(service_id) if {
    permission_set_context_available
    some permission_set_id
    service_id in input.context.granted_permission_sets[permission_set_id]
}

deny contains {"reason": "permission-set context unavailable"} if {
    input.type == "mcp_tool_call"
    input.mcp.tool_name == "permissioned_read"
    not permission_set_context_available
}

allow contains {"reason": "permission set grants service access"} if {
    input.type == "mcp_tool_call"
    input.mcp.tool_name == "permissioned_read"
    permission_set_grants_service(required_service_id)
}

deny contains {"reason": "required permission set missing"} if {
    input.type == "mcp_tool_call"
    input.mcp.tool_name == "permissioned_read"
    permission_set_context_available
    not permission_set_grants_service(required_service_id)
}
```

This pattern keeps permission-set checks composable with the same deny-wins aggregation rule shown above, while making unavailable context fail closed. Keep any blanket `mcp_headers_only` allow as an explicit opt-in rule separate from permission-set checks.

## Full Example Policy File

```rego
package aib.extproc.authz

import rego.v1

# --- allow / deny sets (policy authors add rules here) ---

# Allow MCP initialize and ping (safe, non-tool operations)
allow contains {"reason": "safe MCP lifecycle method"} if {
    input.type == "mcp_method"
    input.mcp.method in {"initialize", "ping", "notifications/initialized"}
}

# Allow read-only MCP tool calls
allow contains {"reason": "read-only tool"} if {
    input.type == "mcp_tool_call"
    input.mcp.tool_name in {"list_repositories", "get_file_contents", "search_code"}
}

# Deny destructive tool calls with explicit reason
deny contains {"reason": "destructive operations are not permitted"} if {
    input.type == "mcp_tool_call"
    input.mcp.tool_name in {"delete_repository", "force_push"}
}

# Allow all non-MCP traffic (passthrough, including A2A until future feature)
allow contains {"reason": "non-MCP passthrough"} if {
    input.type == "unknown"
}

# --- aggregation (deny wins, then allow, then default deny) ---

result := {"action": "deny", "reasons": _deny_reasons} if {
    count(deny) > 0
    _deny_reasons := [r | some entry in deny; r := entry.reason]
}

result := {"action": "allow"} if {
    count(deny) == 0
    count(allow) > 0
}

# Default: deny (fail closed) — no rule matched
default result := {"action": "deny", "reasons": ["no policy rule matched"]}
```
