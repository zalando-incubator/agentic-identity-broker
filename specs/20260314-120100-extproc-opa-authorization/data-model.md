# Data Model: OPA-Based Authorization in ExtProc

**Branch**: `20260314-120100-extproc-opa-authorization`
**Date**: 2026-03-14

## Overview

This feature introduces authorization value objects and configuration types to the ExtProc application. No persistent entities are added — all state is ephemeral per-request or held by the embedded OPA engine. The data model focuses on the OPA input document schema, the decision result structure, and the configuration types.

## Value Objects

### OPAInput

The OPA input document passed to policy evaluation. Constructed per-request, immutable by convention once built.

**Implemented model** — `OPAInput` is not a closed Go struct. The code uses a map alias so ExtProc can preserve the full opa-envoy-plugin-compatible base document produced by `envoyauth.RequestToInput` (`attributes.request.http.*`, `parsed_path`, `parsed_query`, `parsed_body`, `truncated_body`, `version`) and then layer ExtProc-specific top-level keys on top: `type`, `mcp`, and `context`.

```go
// OPAInput is the top-level document passed to OPA for policy evaluation.
type OPAInput = map[string]any

// MCPInput contains parsed MCP protocol fields.
type MCPInput struct {
    JSONRPC   string         `json:"jsonrpc,omitempty"`    // "2.0" for JSON-RPC; omitted for mcp_headers_only
    Method    string         `json:"method,omitempty"`     // MCP method; omitted for mcp_headers_only
    ID        any            `json:"id,omitempty"`         // JSON-RPC request ID; omitted for mcp_headers_only
    ToolName  string         `json:"tool_name,omitempty"`  // Tool name (only for tools/call)
    Arguments map[string]any `json:"arguments,omitempty"`  // Tool arguments (only for tools/call)
    Params    map[string]any `json:"params,omitempty"`     // JSON-RPC params (for non-tools/call methods)
    SessionID string         `json:"session_id,omitempty"` // MCP session ID from header; omitted when header absent
}

// ContextInput contains authorization context.
type ContextInput struct {
    GrantedPermissionSetsAvailable bool                `json:"granted_permission_sets_available"` // True only when the broker supplied an authoritative snapshot
    GrantedPermissionSets          map[string][]string `json:"granted_permission_sets,omitempty"` // Token-bound snapshot from the exchange response; omitted when unavailable or authoritative-empty
}
```

**HTTP request fields** — ExtProc reuses the envoy-compatible base document instead of duplicating request data at the top level.

| Field | Type | Description |
|-------|------|-------------|
| `attributes.request.http.method` | string | HTTP method (from `:method` pseudo-header) |
| `attributes.request.http.path` | string | Request path (from `:path` pseudo-header) |
| `attributes.request.http.scheme` | string | Request scheme (from `:scheme` pseudo-header) |
| `attributes.request.http.host` | string | Request authority/host (from `:authority` pseudo-header) |
| `attributes.request.http.headers` | object | All non-pseudo request headers (keys lowercased) |
| `attributes.request.http.body` | string | Raw request body string |

**Type discriminator values**:

| Type | When | MCP field |
|------|------|-----------|
| `mcp_tool_call` | MCP method is `tools/call` | `tool_name`, `arguments` populated |
| `mcp_method` | Any other MCP JSON-RPC method | `params` populated |
| `mcp_headers_only` | MCP GET request (SSE stream setup, end_of_stream in RequestHeaders phase) | only `session_id` populated (omitted if header absent); `jsonrpc`, `method`, `id` omitted |
| `unknown` | Non-MCP or unrecognized protocol | `mcp` is absent |

**Validation rules**:
- `type` is always set (never empty)
- `attributes.request.http.method` and `attributes.request.http.path` are always present from HTTP/2 pseudo-headers
- `mcp.jsonrpc` is `"2.0"` for `mcp_tool_call` and `mcp_method`; omitted for `mcp_headers_only`
- `context.granted_permission_sets_available` is `true` only when the token exchange response supplied an authoritative snapshot; it is `false` for broker omission and for header-only requests evaluated before token exchange
- `context.granted_permission_sets` is omitted when unavailable and may also be omitted for an authoritative empty snapshot; policies MUST rely on `context.granted_permission_sets_available` rather than object presence
- Envoy-compatible base fields remain available alongside the ExtProc-specific top-level additions
### OPADecision

The result of OPA policy evaluation. Parsed from OPA's `DecisionResult.Result`.

```go
// OPADecision represents the structured result of OPA policy evaluation.
type OPADecision struct {
    Action  string   `json:"action"`            // "allow", "deny", "approval_required", "ciba_required"
    Reasons []string `json:"reasons,omitempty"` // Human-readable denial reasons
}
```

**Action values**:

| Action | Effect | Description |
|--------|--------|-------------|
| `allow` | Continue request processing | Request authorized by policy. For body-bearing requests the token exchange already occurred in RequestHeaders; for header-only requests exchange runs immediately after allow.|
| `deny` | Return 403 Forbidden | Request denied by policy, reasons included |
| `approval_required` | Treated as deny (future Tier 2) | Logged for observability, denied in this implementation |
| `ciba_required` | Treated as deny (future Tier 3) | Logged for observability, denied in this implementation |

**Default decision** (when no rule matches): `authorization.default_decision` is retained for config-shape compatibility but MUST remain `"deny"` in this implementation.

**State transitions**: None — OPADecision is immutable once returned from evaluation.

---

### MCPMessage

Intermediate type for JSON-RPC parsing from the request body.

```go
// MCPMessage represents a parsed JSON-RPC 2.0 message from the MCP protocol.
type MCPMessage struct {
    JSONRPC string         `json:"jsonrpc"`
    Method  string         `json:"method"`
    ID      any            `json:"id"`
    Params  map[string]any `json:"params,omitempty"`
}
```

For `tools/call`, `Params` contains `name` (tool name) and `arguments` (tool arguments). These are extracted during input construction.

---

## Configuration Types

### AuthorizationConfig

Added to `internal/extproc/config/config.go` as a new top-level section in the ExtProc config.

```go
// AuthorizationConfig holds OPA authorization settings.
type AuthorizationConfig struct {
    Enabled           bool           `mapstructure:"enabled"`
    Policy            PolicyConfig   `mapstructure:"policy"`
    DefaultDecision   string         `mapstructure:"default_decision"`
    EvaluationTimeout time.Duration  `mapstructure:"evaluation_timeout"`
    MaxBodySize       int            `mapstructure:"max_body_size"`
}

// PolicyConfig holds OPA policy source settings.
type PolicyConfig struct {
    Path       string `mapstructure:"path"`        // Filesystem path to local Rego file
    ConfigFile string `mapstructure:"config_file"` // Filesystem path to OPA config file
    Package    string `mapstructure:"package"`     // OPA package name (default: aib.extproc.authz)
    Decision   string `mapstructure:"decision"`    // Decision document name (default: result)
}
```

**Validation rules** (added to `Validate()`):
- `path` and `config_file` are mutually exclusive — specifying both is an error (FR-009)
- When `enabled` is true, exactly one of `path` or `config_file` must be set
- `path` must be an absolute path or resolvable relative path; `../` traversal rejected (SR-007)
- `config_file` must exist and be readable
- `default_decision` must remain `"deny"` (default: `"deny"`)
- `evaluation_timeout` must be positive (default: `100ms`)
- `max_body_size` must be positive (default: `1048576` = 1 MiB)

**Defaults**:

| Field | Default |
|-------|---------|
| `enabled` | `false` |
| `policy.package` | `aib.extproc.authz` |
| `policy.decision` | `result` |
| `default_decision` | `deny` |
| `evaluation_timeout` | `100ms` |
| `max_body_size` | `1048576` (1 MiB) |

---

## Component Architecture

### OPA Engine (Authorizer interface)

```go
// Authorizer evaluates authorization policies against a request.
// Implementations must be safe for concurrent use.
type Authorizer interface {
    // Evaluate evaluates the authorization policy with the given input document.
    // Returns the decision result or an error.
    Evaluate(ctx context.Context, input OPAInput) (*OPADecision, error)
    // Stop releases resources held by the authorizer.
    Stop(ctx context.Context)
}
```

**Production implementation**: `OPAAuthorizer` wraps either `rego.PreparedEvalQuery` (path mode) or `sdk.OPA` (config-file/bundle mode) and implements `Authorizer`.

**Test implementation**: `MockAuthorizer` returns configurable decisions for testing.
### Protocol Parsing

```go
func BuildOPAInput(protocol string, body []byte, headers map[string]string, grantedPermissionSets map[string][]string) (OPAInput, error)
func BuildOPAInputHeadersOnly(protocol string, headers map[string]string, grantedPermissionSets map[string][]string) (OPAInput, error)
func ParseMCPMessage(body []byte) (*MCPMessage, error)
func ParseMCPBatch(body []byte) ([]*MCPMessage, error)
```

**Production implementation**: function-based dispatch. `BuildOPAInput` and `BuildOPAInputHeadersOnly` add protocol-specific fields on top of the envoy-compatible base document, and MCP parsing uses `encoding/json` plus the local `MCPMessage` type.

There is no standalone `ProtocolParser` interface or `MCPParser` struct in the implementation.
## Relationships

```
Config.Authorization
    └── AuthorizationConfig
            ├── PolicyConfig (where to load policy from)
            ├── Authorizer (OPA engine, initialized from PolicyConfig)
            │       └── rego.PreparedEvalQuery or sdk.OPA
            └── Server.Process()
                    ├── Body-bearing RequestHeaders: extract metadata, headers, Bearer token → token exchange → replace header → buffer body
                    ├── RequestBody: BuildOPAInput(...) → Authorizer.Evaluate()
                    │       ├── OPADecision{action: "allow"} → echo body
                    │       └── OPADecision{action: "deny"} → ImmediateResponse 403
                    ├── Header-only RequestHeaders: BuildOPAInputHeadersOnly(...) → Authorizer.Evaluate() → allow: token exchange → replace header
                    │                                                                    deny: ImmediateResponse 403
                    └── Other phases: passthrough (unchanged)
```

## Domain Glossary Additions

| Term | Definition |
|------|-----------|
| **OPAInput** | Map-based OPA document constructed by ExtProc for policy evaluation. Starts with the opa-envoy-plugin-compatible base document and adds top-level `type`, `mcp`, and `context` keys. |
| **OPADecision** | Result of OPA policy evaluation — a structured object with an action (`allow`/`deny`) and optional `reasons` array of strings. |
| **Authorizer** | Interface for evaluating authorization policies in ExtProc. The production implementation wraps `rego.PreparedEvalQuery` or the OPA SDK. |
| **ProtocolParser** | Conceptual parsing stage implemented by `BuildOPAInput`, `BuildOPAInputHeadersOnly`, `ParseMCPMessage`, and `ParseMCPBatch`; not a standalone Go interface. |
