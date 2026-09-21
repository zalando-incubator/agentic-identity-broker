# ADR 028: OPA-Based Authorization in ExtProc via Embedded SDK

**Status**: Accepted
**Date**: 2026-03-15
**Feature**: 020-extproc-opa-authorization

---

## Context

The ExtProc token exchange service (`cmd/extproc-token-exchange`) performs transparent RFC 8693 token exchange for all requests with a Bearer token. However, it has no mechanism to evaluate whether a given request (specifically, an MCP tool call) is authorized by policy before performing the token exchange. Without authorization, any agent holding a valid Bearer token can invoke any tool on any MCP server — regardless of organizational policy.

Three design options were evaluated:

1. **Embedded OPA SDK** (`github.com/open-policy-agent/opa/v1`) — OPA compiled into the binary. Policy loaded from local file or OPA config YAML.
2. **Raw `rego` package** (`rego.PreparedEvalQuery`) — Lower-level OPA API, compile-once static policies only. No bundle support.
3. **OPA Sidecar** — Separate OPA process, ExtProc calls it over HTTP for each decision.

---

## Decision

Embed OPA via the `github.com/open-policy-agent/opa/v1` SDK with a dual-backend `OPAAuthorizer`:

- **Path mode** (`authorization.policy.path`): uses `rego.PreparedEvalQuery` (compile-once, zero overhead per evaluation). Suitable for local `.rego` files in static deployments.
- **SDK mode** (`authorization.policy.config_file`): uses `sdk.New()` with an OPA config YAML. Enables bundle pulling, discovery, and other OPA-native policy management. Suitable for dynamic/enterprise deployments.

The `Authorizer` interface (`internal/extproc/authorization/authorizer.go`) is the hexagonal boundary between the gRPC server and the OPA evaluation engine. `Server` depends on the interface, never on `OPAAuthorizer` directly.
This ADR defines a separate authorization layer in the standalone ExtProc service. It does not replace or relax ADR 009's broker-side CEL authorization for RFC 8693 token exchange; requests may pass ExtProc OPA and still be denied by the broker's CEL policy.

Operators use the two mechanisms for different questions:
- ADR 009 CEL answers whether a privileged gateway may perform token exchange for a given broker request.
- ExtProc OPA answers whether a specific proxied request or MCP tool call may proceed through the gateway at all.

Authorization is **disabled by default** (`authorization.enabled: false`). When disabled, the service behaves identically to the pre-feature token-exchange-only behavior — no body inspection, no overhead.

### Security-First reconciliation

Constitution Principle I prefers security features enabled by default. This ExtProc OPA layer is an additive, operator-supplied policy surface with no safe universal default:

- enabling it by default without a policy source would fail startup and break existing token-exchange deployments
- a built-in allow-all policy would be insecure
- a built-in deny-all policy would make the proxy unusable

This ADR therefore records the explicit exception rationale for shipping `authorization.enabled: false` by default. The existing authoritative controls remain active when ExtProc OPA is disabled — RFC 8693 token exchange plus ADR 009 broker-side CEL authorization — and, once enabled, every ExtProc OPA failure mode remains fail-closed (`default_decision = deny`, undefined result = deny, timeout = deny, evaluation error = deny).

---

## Rationale

### Why not OPA sidecar?

A sidecar adds operational complexity (two containers, sidecar lifecycle, health checks, mTLS between ExtProc and OPA). For the expected call volume (one policy evaluation per MCP tool call), embedded OPA's latency (<1ms for compiled policies) is more than acceptable. Sidecar is the right choice when policies must be updated without restarting ExtProc; OPA SDK's bundle pulling (US4) covers that use case without a separate process.

### Why dual-backend (PreparedEvalQuery + sdk.New)?

`rego.PreparedEvalQuery` is the fastest path for static local policies — it compiles the Rego module once at startup and evaluates with minimal overhead. `sdk.New()` adds bundle lifecycle management (background refresh, HTTP polling) which has non-trivial overhead for users who only need a local file. Providing both allows operators to choose the right tradeoff.

### Fail-closed behavior

All error paths (evaluation error, timeout, undefined result, parse failure) return `deny`. This is non-negotiable: a policy evaluation failure must not grant access (SR-001, SR-003).

---

## Processing Flow Change (Body Buffering)

When `authorization.enabled: true`, the ExtProc request processing pipeline changes:

**Before (token exchange only)**:
```
RequestHeaders → extract Bearer + path → token exchange → replace Authorization header
```

**After (OPA authorization enabled)**:
```
Body-bearing requests:
RequestHeaders → extract Bearer + path + protocol metadata → token exchange + auth mutation + request BUFFERED body
RequestBody    → parse body (MCP JSON-RPC) → OPA evaluate → allow: echo body
                                                         → deny: 403 ImmediateResponse

Header-only requests:
RequestHeaders(end_of_stream=true) → extract metadata → OPA evaluate → allow: token exchange + auth mutation
                                                                    → deny: 403 ImmediateResponse
```

This body-bearing path is implemented via `processRequestHeadersOPA` returning `ModeOverride.RequestBodyMode=BUFFERED`, which instructs Envoy to collect and send the full request body as a `RequestBody` message.

---

## Input Schema

The OPA input document (`OPAInput`) uses a protocol-namespaced layout:

```json
{
  "type": "mcp_tool_call",
  "mcp": {
    "jsonrpc": "2.0",
    "method": "tools/call",
    "tool_name": "list_repositories",
    "arguments": {"repo": "acme/app"},
    "session_id": "sess-abc123",
    "target_server_name": "github-mcp"
  },
  "request": {
    "method": "POST",
    "path": "/mcp",
    "scheme": "http",
    "authority": "mcp-server:9003",
    "headers": {"authorization": "Bearer ..."},
    "body": "{...}"
  },
  "context": {
    "granted_permission_sets": {}
  }
}
```

Type discriminators: `mcp_tool_call`, `mcp_method`, `unknown`.

---

## Amendment (2026-09-21): Mandatory `target_server_name` for MCP Requests

This ADR's original Input Schema did not include `mcp.target_server_name`. This amendment extends the `mcp` object with a `target_server_name` field and makes it a mandatory input for all MCP-protocol requests when authorization is enabled.

### Extension

`mcp.target_server_name` carries the agentgateway-resolved target MCP server name (from the `mcp_server` dynamic metadata key, the same source as `protocol`). It is required whenever `type` is `mcp`, following the same mandatory-metadata pattern already established for `protocol` in this ADR: `processRequestHeadersOPA` rejects the request with a 403 `ImmediateResponse` before any OPA evaluation if the metadata is absent or empty.

### Rationale

Authorization policies that scope permissions per downstream MCP server (for example, restricting an agent's granted permission sets to a specific server) cannot make a sound decision without knowing which server is the actual target. Treating the field as optional would let requests reach OPA with an ambiguous or missing target, undermining the fail-closed posture this ADR already establishes for `protocol`. Optionality would also let a misconfigured or misspelled `mcp_server` metadata attribute silently degrade into an unintended fail-open outcome instead of surfacing the error.

### Impact

- The Input Schema example above now includes `target_server_name`.
- `BuildOPAInput` and `BuildOPAInputHeadersOnly` both take a `targetServerName` parameter, populated only after the mandatory-metadata check has passed.
- Existing non-MCP request types (`type: "unknown"`) are unaffected; the mandatory check only applies when `protocol == "mcp"`.

---

## Consequences

- **Latency**: OPA evaluation adds <1ms p99 for compiled policies (PreparedEvalQuery). SDK mode with bundle pulling may add up to ~5ms for the first evaluation after a bundle refresh.
- **Body buffering**: When OPA is enabled, all requests with Bearer tokens must buffer their bodies. This increases memory usage proportionally to `authorization.max_body_size` (default 1 MiB). Bodies exceeding this limit are rejected with 403 (not truncated — truncation is a policy-evasion vector).
- **Operational**: Two new config fields (`authorization.policy.path` and `authorization.policy.config_file`) are mutually exclusive. Operators must choose one. Neither is required when `authorization.enabled: false`.
- **Dependencies**: Adds `github.com/open-policy-agent/opa v1.14.1` as a direct dependency. OPA brings transitive dependencies (~15 packages). Binary size increases by approximately 25-30 MiB.
