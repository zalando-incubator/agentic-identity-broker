# Contract: OPA Input Document Schema

**Branch**: `20260314-120100-extproc-opa-authorization`
**Date**: 2026-03-14

## Overview

This document specifies the JSON shape of the input document passed to OPA for policy evaluation.

The implemented input is a **superset** of the ExtProc-specific fields below:

- an opa-envoy-plugin / ext_authz-compatible base document (`attributes.request.http.*`, `parsed_path`, `parsed_query`, `parsed_body`, `truncated_body`, `version`)
- ExtProc-specific top-level additions (`type`, `mcp`, `context`)

Policies should reference HTTP request fields via the envoy-compatible base, e.g. `input.attributes.request.http.method` and `input.attributes.request.http.body`.

## JSON Schema

```json
{
  "$schema": "http://json-schema.org/draft-07/schema#",
  "title": "OPA Input Document for ExtProc Authorization",
  "type": "object",
  "required": ["type", "attributes", "context"],
  "properties": {
    "type": {
      "type": "string",
      "enum": ["mcp_tool_call", "mcp_method", "mcp_headers_only", "unknown"],
      "description": "Discriminator combining protocol and operation type"
    },
    "attributes": {
      "type": "object",
      "description": "Envoy ext_authz-compatible base document produced by opa-envoy-plugin RequestToInput",
      "properties": {
        "request": {
          "type": "object",
          "properties": {
            "http": {
              "type": "object",
              "properties": {
                "method": { "type": "string" },
                "path": { "type": "string" },
                "headers": {
                  "type": "object",
                  "additionalProperties": { "type": "string" }
                },
                "host": { "type": "string" },
                "scheme": { "type": "string" },
                "body": { "type": "string" },
                "protocol": { "type": "string" }
              }
            }
          }
        }
      }
    },
    "parsed_path": {
      "type": "array",
      "description": "Path split into segments by the opa-envoy-plugin-compatible base",
      "items": { "type": "string" }
    },
    "parsed_query": {
      "type": "object",
      "description": "Parsed query parameters from the opa-envoy-plugin-compatible base",
      "additionalProperties": {
        "type": "array",
        "items": { "type": "string" }
      }
    },
    "parsed_body": {
      "description": "Parsed JSON body from the Envoy-compatible base when the body is valid JSON; otherwise null"
    },
    "truncated_body": {
      "type": "boolean",
      "description": "Whether the request body was truncated before evaluation"
    },
    "version": {
      "type": "object",
      "description": "Envoy-compatible version metadata",
      "properties": {
        "ext_authz": { "type": "string" },
        "encoding": { "type": "string" }
      }
    },
    "mcp": {
      "type": "object",
      "description": "MCP protocol-specific fields (present when type starts with 'mcp_')",
      "properties": {
        "jsonrpc": {
          "type": "string",
          "const": "2.0",
          "description": "JSON-RPC version"
        },
        "method": {
          "type": "string",
          "description": "MCP method name (e.g., 'tools/call', 'initialize')"
        },
        "id": {
          "description": "JSON-RPC request ID (string, number, or null)"
        },
        "tool_name": {
          "type": "string",
          "description": "Tool name (present only when type is mcp_tool_call)"
        },
        "arguments": {
          "type": "object",
          "description": "Tool arguments (present only when type is mcp_tool_call)"
        },
        "params": {
          "type": "object",
          "description": "JSON-RPC params (present only when type is mcp_method)"
        },
        "session_id": {
          "type": "string",
          "description": "MCP session ID from Mcp-Session-Id header (if present)"
        }
      }
    },
    "context": {
      "type": "object",
      "description": "Authorization context",
      "required": ["granted_permission_sets_available"],
      "properties": {
        "granted_permission_sets_available": {
          "type": "boolean",
          "description": "Whether granted_permission_sets is authoritative for this request. False for pre-exchange header-only evaluation and broker omission."
        },
        "granted_permission_sets": {
          "type": "object",
          "description": "Authoritative permission set data from the RFC 8693 token exchange response. This field is omitted when unavailable and may also be omitted when the authoritative snapshot is empty."
        }
      }
    }
  },
  "allOf": [
    {
      "if": { "properties": { "type": { "const": "mcp_tool_call" } } },
      "then": { "required": ["mcp"], "properties": { "mcp": { "required": ["jsonrpc", "method", "tool_name"] } } }
    },
    {
      "if": { "properties": { "type": { "const": "mcp_method" } } },
      "then": { "required": ["mcp"], "properties": { "mcp": { "required": ["jsonrpc", "method"] } } }
    },
    {
      "if": { "properties": { "type": { "const": "mcp_headers_only" } } },
      "then": { "required": ["mcp"] }
    },
    {
      "if": { "properties": { "type": { "const": "unknown" } } },
      "then": { "not": { "required": ["mcp"] } }
    }
  ]
}
```

## Examples

### MCP Tool Call

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
          "authorization": "Bearer eyJhbGciOiJSUzI1NiIs...",
          "mcp-session-id": "session-abc-123"
        },
        "body": "{\"jsonrpc\":\"2.0\",\"method\":\"tools/call\",\"id\":1,\"params\":{\"name\":\"create_issue\",\"arguments\":{\"title\":\"Bug report\",\"repo\":\"acme/app\"}}}"
      }
    }
  },
  "mcp": {
    "jsonrpc": "2.0",
    "method": "tools/call",
    "id": 1,
    "tool_name": "create_issue",
    "arguments": {
      "title": "Bug report",
      "repo": "acme/app"
    },
    "session_id": "session-abc-123"
  },
  "context": {
    "granted_permission_sets_available": true,
    "granted_permission_sets": {
      "11111111-1111-1111-1111-111111111111": ["22222222-2222-2222-2222-222222222222"]
    }
  }
}
```

### MCP Method (non-tool-call)

```json
{
  "type": "mcp_method",
  "attributes": {
    "request": {
      "http": {
        "method": "POST",
        "path": "/mcp",
        "scheme": "https",
        "host": "mcp-server.example.com",
        "headers": {
          "content-type": "application/json",
          "authorization": "Bearer eyJhbGciOiJSUzI1NiIs..."
        },
        "body": "{\"jsonrpc\":\"2.0\",\"method\":\"initialize\",\"id\":1,\"params\":{\"protocolVersion\":\"2025-03-26\",\"clientInfo\":{\"name\":\"my-agent\",\"version\":\"1.0\"}}}"
      }
    }
  },
  "mcp": {
    "jsonrpc": "2.0",
    "method": "initialize",
    "id": 1,
    "params": {
      "protocolVersion": "2025-03-26",
      "clientInfo": { "name": "my-agent", "version": "1.0" }
    }
  },
  "context": {
    "granted_permission_sets_available": false
  }
}
```

### MCP Header-Only (GET /mcp, SSE stream setup)

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
          "authorization": "Bearer eyJhbGciOiJSUzI1NiIs..."
        },
        "body": ""
      }
    }
  },
  "mcp": {
    "session_id": "sess-abc123"
  },
  "context": {
    "granted_permission_sets_available": false
  }
}
```

Header-only `mcp_headers_only` inputs set `context.granted_permission_sets_available: false` and omit `context.granted_permission_sets` because OPA evaluates those requests before token exchange.

### Unknown Protocol

```json
{
  "type": "unknown",
  "attributes": {
    "request": {
      "http": {
        "method": "POST",
        "path": "/api/something",
        "scheme": "https",
        "host": "service.example.com",
        "headers": {
          "content-type": "application/json",
          "authorization": "Bearer token123"
        },
        "body": "<raw body string>"
      }
    }
  },
  "context": {
    "granted_permission_sets_available": false
  }
}
```

## OPA Decision Result Schema

```json
{
  "$schema": "http://json-schema.org/draft-07/schema#",
  "title": "OPA Decision Result",
  "type": "object",
  "required": ["action"],
  "properties": {
    "action": {
      "type": "string",
      "enum": ["allow", "deny", "approval_required", "ciba_required"],
      "description": "Authorization decision action"
    },
    "reasons": {
      "type": "array",
      "items": { "type": "string" },
      "description": "Human-readable denial reasons (present when action=deny)"
    }
  }
}
```

## Error Response Schema (403 Forbidden)

When OPA denies a request, ExtProc returns a 403 ImmediateResponse with this JSON body:

```json
{
  "error": "authorization_denied",
  "error_description": "Request denied by authorization policy: <comma-separated reasons>"
}
```

When protocol metadata is missing:

```json
{
  "error": "authorization_failed",
  "error_description": "Protocol metadata missing from request — authorization requires protocol type from agentgateway"
}
```
