# Contract Delta: OPA Input Document Schema — `mcp.target_server_name` Field

**Branch**: `044-extproc-opa-mcp-target-server`
**Date**: 2026-09-16
**Base contract**: [specs/020-extproc-opa-authorization/contracts/opa-input-schema.md](../../020-extproc-opa-authorization/contracts/opa-input-schema.md)

## Overview

This document is a **delta** to the frozen `opa-input-schema.md` contract from spec 020. Editing that prior
feature's contract file directly is out of scope; this file documents
the single additive change instead. Apply this delta mentally on top of the base contract when authoring or
reviewing policies — the base contract remains the source of truth for every field not listed here.

## What Changes

One new required-for-MCP property is added to the `mcp` object in the OPA input document JSON Schema:

```json
{
  "mcp": {
    "type": "object",
    "properties": {
      "target_server_name": {
        "type": "string",
        "description": "MCP server identifier from agentgateway's metadataContext.agentgateway.mcp_server metadata attribute. Always present and non-empty when the discriminated top-level `type` is one of mcp_tool_call, mcp_method, or mcp_headers_only — ExtProc rejects the request with HTTP 403 at the headers phase before OPA ever evaluates if the attribute is absent or empty (FR-004). Passed through unmodified — no case-folding, trimming, or allow-list validation."
      }
    }
  }
}
```

No JSON Schema `required` entries change (the schema itself still describes `mcp.target_server_name` as an
optional property of the `mcp` object, since it is never populated for non-MCP `type` values). However, unlike
`mcp.jsonrpc`/`mcp.method` — which are genuinely optional depending on JSON-RPC message shape — `mcp.target_server_name`
is now **mandatory** for any request that reaches OPA with an MCP `type`: its absence or emptiness causes ExtProc
to reject the request outright (FR-004), so no MCP-typed OPA input document can ever lack it.

## Field Reference

| Field | Type | Presence | Description |
|-------|------|----------|--------------|
| `mcp.target_server_name` | string | Required for MCP requests | The MCP server identifier the request targets, sourced from `MetadataContext.FilterMetadata["agentgateway"]["mcp_server"]`. Absent or empty causes ExtProc to reject the request with HTTP 403 before OPA is invoked. Never populated when `protocol != "mcp"` (i.e. `type == "unknown"`), since the field has no meaning outside MCP requests. |

## Source of the Metadata Value

`mcp.target_server_name` is read from the same `agentgateway` filter-metadata namespace already used for `protocol`
(spec 020):

```
MetadataContext.FilterMetadata["agentgateway"]["mcp_server"] = "<server-identifier>"
```

This is a sibling field to `MetadataContext.FilterMetadata["agentgateway"]["protocol"]` — agentgateway operators
add one line to the existing metadata configuration block rather than configuring a new filter-metadata
namespace.

## Presence Rules by `type`

| `type` | `mcp.target_server_name` present? |
|--------|----------------------|
| `mcp_tool_call` | Always — ExtProc rejects with HTTP 403 before this input document is built if `mcp_server` metadata is absent or empty |
| `mcp_method` | Always — same rejection rule as above |
| `mcp_headers_only` | Always — same rejection rule as above |
| `unknown` (i.e. `protocol != "mcp"`) | Never — `mcp.target_server_name` is never populated for non-MCP protocol requests, even if `mcp_server` metadata happens to be present (FR-005) |

An empty-string `mcp_server` metadata value is treated identically to the attribute being entirely absent: both
cause ExtProc to reject the request with HTTP 403 at the headers phase (FR-004).

## Updated Examples

### MCP Tool Call with `mcp.target_server_name`

```json
{
  "type": "mcp_tool_call",
  "attributes": {
    "request": {
      "http": {
        "method": "POST",
        "path": "/mcp",
        "scheme": "https",
        "host": "mcp-server.example.com",
        "headers": {
          "content-type": "application/json",
          "authorization": "******",
          "mcp-session-id": "session-abc-123"
        },
        "body": "{\"jsonrpc\":\"2.0\",\"method\":\"tools/call\",\"id\":1,\"params\":{\"name\":\"delete_record\",\"arguments\":{\"id\":\"42\"}}}"
      }
    }
  },
  "mcp": {
    "jsonrpc": "2.0",
    "method": "tools/call",
    "id": 1,
    "tool_name": "delete_record",
    "arguments": { "id": "42" },
    "session_id": "session-abc-123",
    "target_server_name": "salesforce-mcp"
  },
  "context": {
    "granted_permission_sets_available": true,
    "granted_permission_sets": {
      "11111111-1111-1111-1111-111111111111": ["22222222-2222-2222-2222-222222222222"]
    }
  }
}
```

### MCP Header-Only with `mcp.target_server_name`

```json
{
  "type": "mcp_headers_only",
  "attributes": {
    "request": {
      "http": {
        "method": "GET",
        "path": "/mcp",
        "scheme": "https",
        "host": "mcp-server.example.com",
        "headers": {
          "accept": "text/event-stream",
          "authorization": "******"
        },
        "body": ""
      }
    }
  },
  "mcp": {
    "session_id": "sess-abc123",
    "target_server_name": "github-mcp"
  },
  "context": {
    "granted_permission_sets_available": false
  }
}
```

### MCP Tool Call — `mcp_server` Metadata Absent (rejected with 403, request never reaches OPA)

When `protocol` is `mcp` but `metadataContext.agentgateway.mcp_server` is absent or an empty string, ExtProc
rejects the request at the headers phase — no OPA input document is built at all:

```json
{
  "error": "access_denied",
  "error_description": "mcp_server metadata is required when authorization is enabled for MCP requests"
}
```

This mirrors the existing missing-`protocol` rejection body shape (`missing_protocol_metadata`), using error
type `missing_target_server_metadata` internally for structured logging (FR-004).

## Unaffected: OPA Decision Result Schema and 403 Error Bodies

The `OPADecision` result schema and the 403 `ImmediateResponse` error body shapes documented in the base contract
are unchanged. This feature only extends the *input* document; it introduces no new decision actions, error
codes, or response shapes.
