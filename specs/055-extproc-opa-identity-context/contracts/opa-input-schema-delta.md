# Contract Delta: OPA Input Document Schema — `context.principal` and `context.agent_id`

**Branch**: `055-extproc-opa-identity-context`
**Date**: 2026-10-29
**Base contracts**: [specs/020-extproc-opa-authorization/contracts/opa-input-schema.md](../../020-extproc-opa-authorization/contracts/opa-input-schema.md), extended by [specs/026-extproc-approval-sync/contracts/opa-input-approval.md](../../026-extproc-approval-sync/contracts/opa-input-approval.md)

## Overview

This is a **delta** to the frozen base contracts. They remain the source of truth for every field not listed here.

## What Changes

Two optional properties are added to the `context` object:

```json
{
  "context": {
    "type": "object",
    "properties": {
      "principal": {
        "type": "string",
        "description": "The verified principal from the broker's token-exchange response for this request. Omitted when the response has none and on header-only evaluations."
      },
      "agent_id": {
        "type": "string",
        "format": "uuid",
        "description": "The resolved canonical agent ID from the broker's token-exchange response for this request. Omitted when the response has none and on header-only evaluations."
      }
    }
  }
}
```

No `required` entries change.

Go declaration (`internal/extproc/authorization/input.go`):

```go
type ContextInput struct {
	GrantedPermissionSetsAvailable bool                `json:"granted_permission_sets_available"`
	GrantedPermissionSets          map[string][]string `json:"granted_permission_sets,omitempty"`
	AgentSessionID                 string              `json:"agent_session_id,omitempty"`
	Principal                      string              `json:"principal,omitempty"` // NEW
	AgentID                        string              `json:"agent_id,omitempty"`  // NEW
}
```

## Field Reference

| Field | Type | Source | Presence |
|-------|------|--------|----------|
| `context.principal` | string | `principal` on the `POST /oauth2/token` token-exchange response (spec 026 FR-017) | Every evaluation after token exchange, when the response carries it |
| `context.agent_id` | string (UUID) | `agent_id` on the same response | Every evaluation after token exchange, when the response carries it |

Values are passed through unmodified. ExtProc never derives them from the subject token or a request header, and never emits an empty string or a placeholder.

## Presence Rules by `type`

| `type` | `context.principal` / `context.agent_id` present? |
|--------|----------------------|
| `mcp_tool_call` | Yes, when the exchange response carries them. Every element of a JSON-RPC batch gets the same values. |
| `mcp_method` | Same as `mcp_tool_call` |
| `unknown` (body-bearing) | Same as `mcp_tool_call` |
| `mcp_headers_only` | Never. Header-only requests are evaluated before token exchange. |

## Example

### MCP tool call

```json
{
  "type": "mcp_tool_call",
  "mcp": {
    "jsonrpc": "2.0",
    "method": "tools/call",
    "id": 7,
    "tool_name": "get_user",
    "arguments": { "user_id": "u-42" },
    "session_id": "sess-alice-1",
    "target_server_name": "directory-mcp"
  },
  "context": {
    "granted_permission_sets_available": false,
    "agent_session_id": "sess-alice-1",
    "principal": "alice@example.com",
    "agent_id": "7c9e6679-7425-40de-944b-e07fc1f90ae7"
  }
}
```

(`attributes`, `parsed_body` and the other envoy-plugin keys are unchanged and omitted.)

### Header-only MCP request

```json
{
  "type": "mcp_headers_only",
  "mcp": {
    "session_id": "sess-alice-1",
    "target_server_name": "directory-mcp"
  },
  "context": {
    "granted_permission_sets_available": false,
    "agent_session_id": "sess-alice-1"
  }
}
```

## Unaffected

The decision result schema, the 403 error bodies, the approval flow, and the order of OPA evaluation and token exchange are unchanged.
