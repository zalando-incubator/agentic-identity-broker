# OPA contract: approval-gating input and decision (feature 026)

**Feature**: `026-extproc-approval-sync` | **Owner**: `internal/extproc/authorization`

This contract is additive to feature 020's shipped OPA contract. Every field feature 020 defined keeps
its meaning and its position in the document.

---

## 1. Input additions

### `input.context.agent_session_id` (new)

| Property | Value |
|---|---|
| Type | `string` |
| Presence | Emitted when the configured header is present; `omitempty` otherwise |
| Source | The HTTP request header named by `sessions.extraction.http_header` (default `Mcp-Session-Id`) |
| Purpose | Lets policies reason about session-scoped approvals; also the value ExtProc uses to scope session approvals and to populate `metadata.agent_session_id` on approval creation |

Go declaration (`internal/extproc/authorization/input.go`):

```go
type ContextInput struct {
	GrantedPermissionSetsAvailable bool                `json:"granted_permission_sets_available"`
	GrantedPermissionSets          map[string][]string `json:"granted_permission_sets,omitempty"`
	AgentSessionID                 string              `json:"agent_session_id,omitempty"` // NEW
}
```

### Relationship to `input.mcp.session_id` — deliberately distinct

`input.mcp.session_id` is **unchanged**: the MCP protocol session, always read from the fixed
`Mcp-Session-Id` header by `extractSessionID` (`input_builder.go:162-170`), populated for both
`mcp_tool_call` and `mcp_headers_only` inputs.

The two fields coincide under the default configuration and diverge only when an operator maps a
different header. Folding them was considered and rejected: `input.mcp.session_id` is already asserted
by shipped ExtProc code, by `input_builder_test.go`, and by feature 020's published contract, so
moving it would create a spec-versus-code contradiction for no functional gain. Semantically the MCP
protocol session is an `mcp.*` attribute; the agent session is an authorization-context attribute.

### Unchanged: permission sets

`input.context.granted_permission_sets` (map of permission-set id → service ids) and
`input.context.granted_permission_sets_available` keep feature 020's exact contract. They are sourced
from the **RFC 8693 token-exchange response snapshot**, never from the approval cache (spec FR-001).
When the availability flag is `false`, policy MUST fail closed.

### Forbidden: approval records

Approval records MUST NOT appear anywhere in the OPA input. Cache matching happens in ExtProc, outside
the policy, using `internal/toolpattern` (spec FR-001/FR-015; ADR 035). Policies decide only whether
approval is *required*; they stay approval-agnostic.

---

## 2. Builder signature change

`BuildOPAInput`'s trailing permission-set argument is replaced by a `ContextInput` value so future
context fields do not grow the signature again:

```go
// before
func BuildOPAInput(protocol string, body []byte, headers map[string]string,
	grantedPermissionSets map[string][]string) (OPAInput, error)

// after
func BuildOPAInput(protocol string, body []byte, headers map[string]string,
	ctx ContextInput) (OPAInput, error)
```

`BuildOPAInputHeadersOnly` keeps its shape and continues to emit
`granted_permission_sets_available: false` with no permission-set map — header-only requests evaluate
before token exchange (ADR 028) and are not approval-gated. It additionally emits
`context.agent_session_id` when the configured header is present, so a policy can read the field
uniformly across both input types.

---

## 3. Decision contract

### Action values

```go
const (
	ActionAllow            = "allow"
	ActionDeny             = "deny"
	ActionApprovalRequired = "approval_required"
	ActionCIBARequired     = "ciba_required"
)
```

`result` must remain an object carrying at least `action`. `ParseDecision` behavior:

| `result.action` | Parsed `OPADecision.Action` | ExtProc behavior |
|---|---|---|
| `"allow"` | `ActionAllow` | Echo the buffered body; the exchanged token is already on the header |
| `"deny"` | `ActionDeny` | 403 `access_denied`. **No approval created** (FR-014) |
| `"approval_required"` | `ActionApprovalRequired` — **changed** | Enter the approval gate (FR-002) |
| `"ciba_required"` | `ActionDeny` — **unchanged** | 403, logged with the raw action and `result_code=unsupported_action` (deferred; FR-002) |
| undefined / missing / wrong type / unknown string | `ActionDeny` | 403, logged as a policy-misconfiguration warning (FR-012) |

**Superseded behavior**: `decision.go:51-55` currently normalizes `approval_required` to deny with the
reason `"approval_required is not yet supported"` — feature 020 FR-016. Feature 026 supersedes FR-016
for that single action only. The `ciba_required` half of FR-016 is retained verbatim.

Two existing unit tests assert the superseded behavior and must be rewritten:

- `internal/extproc/authorization/decision_test.go:28-32` — `TestParseDecision_ApprovalRequired_MappedToDeny`
- `internal/extproc/authorization/authorizer_test.go:455-472` — `TestOPAAuthorizer_ApprovalRequired_LogsRawActionAndReturnsDeny`

### `approval_context` (optional policy output)

When `action == "approval_required"`, policy MAY emit an `approval_context` object. Every field is
optional and ExtProc degrades gracefully on absence:

| Field | Type | Used for | Fallback when absent |
|---|---|---|---|
| `description` | `string` | `metadata.description` on create — **required by the broker** | `"Approval required for <tool_name>"` |
| `risk_level` | `string` | `risk_level` on create | Field omitted; broker treats it as unset |

Any other key is ignored. `reasons` keeps its feature-020 meaning and is surfaced in deny responses.

---

## 4. Example policy fragment

```rego
package aib.extproc.authz

import rego.v1

default result := {"action": "deny", "reasons": ["no matching rule"]}

# Tier 1: fail closed when the exchange snapshot is unavailable.
result := {"action": "deny", "reasons": ["permission sets unavailable"]} if {
	not input.context.granted_permission_sets_available
}

# Tier 1: permission-set boundary.
result := {"action": "deny", "reasons": ["tool not covered by a granted permission set"]} if {
	input.context.granted_permission_sets_available
	input.type == "mcp_tool_call"
	not tool_granted
}

# Tier 2: covered but requires human approval.
result := {
	"action": "approval_required",
	"approval_context": {
		"description": sprintf("Approval required for %s", [input.mcp.tool_name]),
		"risk_level": "medium",
	},
} if {
	input.context.granted_permission_sets_available
	input.type == "mcp_tool_call"
	tool_granted
	data.tool_permissions[input.mcp.tool_name].risk == "medium"
}

result := {"action": "allow"} if {
	input.context.granted_permission_sets_available
	input.type == "mcp_tool_call"
	tool_granted
	data.tool_permissions[input.mcp.tool_name].risk == "low"
}

tool_granted if {
	some set_id
	input.context.granted_permission_sets[set_id]
	set_id in data.tool_permissions[input.mcp.tool_name].permission_sets
}
```

---

## 5. Full input document shape (body-bearing `tools/call`)

Fields marked **NEW** are added by this feature; all others are feature 020's shipped contract.

```json
{
  "type": "mcp_tool_call",
  "attributes": { "request": { "http": { "method": "POST", "path": "/mcp", "headers": {} } } },
  "parsed_body": { },
  "truncated_body": false,
  "mcp": {
    "jsonrpc": "2.0",
    "method": "tools/call",
    "id": 1,
    "tool_name": "create_pull_request",
    "arguments": {"repo": "acme/app"},
    "session_id": "sess-abc"
  },
  "context": {
    "granted_permission_sets_available": true,
    "granted_permission_sets": {"ps-1": ["svc-github"]},
    "agent_session_id": "sess-abc"
  }
}
```

**NEW**: `context.agent_session_id` only. Everything else is unchanged.
