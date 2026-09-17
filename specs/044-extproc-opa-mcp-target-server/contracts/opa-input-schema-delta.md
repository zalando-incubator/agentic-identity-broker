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

One new optional property is added to the `mcp` object in the OPA input document JSON Schema:

```json
{
  "mcp": {
    "type": "object",
    "properties": {
      "target_server_name": {
        "type": "string",
        "description": "MCP server identifier from agentgateway's metadataContext.agentgateway.mcp_server metadata attribute. Present only when the attribute was supplied as a non-empty string by agentgateway AND the discriminated top-level `type` is one of mcp_tool_call, mcp_method, or mcp_headers_only. Passed through unmodified — no case-folding, trimming, or allow-list validation."
      }
    }
  }
}
```

No `required` entries change. `mcp.target_server_name` is always optional — unlike `mcp.jsonrpc`/`mcp.method`, its absence is
never a schema violation and never causes ExtProc to reject the request (FR-004).

## Field Reference

| Field | Type | Presence | Description |
|-------|------|----------|--------------|
| `mcp.target_server_name` | string | Optional | The MCP server identifier the request targets, sourced from `MetadataContext.FilterMetadata["agentgateway"]["mcp_server"]`. Omitted (not empty string) when the attribute is absent, empty, or `protocol != "mcp"` (i.e. `type == "unknown"`). |

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
| `mcp_tool_call` | Yes, when `mcp_server` metadata is present and non-empty |
| `mcp_method` | Yes, when `mcp_server` metadata is present and non-empty |
| `mcp_headers_only` | Yes, when `mcp_server` metadata is present and non-empty |
| `unknown` (i.e. `protocol != "mcp"`) | Never — `mcp.target_server_name` is never populated for non-MCP protocol requests, even if `mcp_server` metadata happens to be present (FR-005) |

An empty-string `mcp_server` metadata value is treated identically to the attribute being entirely absent: the
field is omitted, never emitted as `"target_server_name": ""`.

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

### MCP Tool Call — `mcp_server` Metadata Absent (no rejection, field omitted)

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
        "headers": { "content-type": "application/json", "authorization": "******" },
        "body": "{\"jsonrpc\":\"2.0\",\"method\":\"tools/call\",\"id\":2,\"params\":{\"name\":\"list_repositories\",\"arguments\":{}}}"
      }
    }
  },
  "mcp": {
    "jsonrpc": "2.0",
    "method": "tools/call",
    "id": 2,
    "tool_name": "list_repositories",
    "arguments": {}
  },
  "context": {
    "granted_permission_sets_available": true,
    "granted_permission_sets": {}
  }
}
```

Note the absence of the `target_server_name` key entirely — not `"target_server_name": ""` — matching the FR-004 requirement.

## Unaffected: OPA Decision Result Schema and 403 Error Bodies

The `OPADecision` result schema and the 403 `ImmediateResponse` error body shapes documented in the base contract
are unchanged. This feature only extends the *input* document; it introduces no new decision actions, error
codes, or response shapes.
