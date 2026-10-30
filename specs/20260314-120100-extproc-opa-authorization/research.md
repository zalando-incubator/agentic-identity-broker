# Research: OPA-Based Authorization in ExtProc

**Branch**: `20260314-120100-extproc-opa-authorization`
**Date**: 2026-03-14

## Research Task 1: OPA Go SDK Integration Approach

### Decision: Use `github.com/open-policy-agent/opa/v1/sdk` package

### Rationale

The OPA SDK (`v1/sdk`) provides the high-level API recommended by the OPA project for embedding OPA inside Go programs. It supports:

- **`sdk.New(ctx, Options)`** — Creates an OPA instance with configuration provided as a JSON/YAML byte stream. Blocks until ready (or uses a `Ready` channel).
- **`opa.Decision(ctx, DecisionOptions)`** — Thread-safe named policy evaluation. Accepts `Path` (the decision document path, e.g., `/aib/extproc/authz/result`) and `Input` (any Go type — map, struct, etc.).
- **`sdk.IsUndefinedErr(err)`** — Detects SDK undefined results (e.g. bundle not ready or decision document unavailable). In `config_file` mode these are denied to preserve fail-closed behavior.
- **`opa.Stop(ctx)`** — Graceful shutdown.
- **`opa.Configure(ctx, ConfigOptions)`** — Atomic reconfiguration (useful for future hot-reload).

Key advantages over the low-level `rego` package:
1. **Native bundle support** — The SDK handles bundle polling, caching, and hot-reload via OPA's built-in management loop. No custom bundle fetching needed.
2. **Decision logging** — Built-in support for console decision logging.
3. **Single initialization** — One `sdk.New()` call handles policy compilation, plugin initialization, and readiness — matching our fail-fast startup pattern.
4. **Thread-safe** — `Decision()` is explicitly documented as safe for concurrent use.

### Configuration Mapping

For **local Rego file**, the SDK accepts a file-scheme bundle resource:
```json
{
  "bundles": {
    "local": {
      "resource": "file:///absolute/path/to/policy/dir"
    }
  }
}
```
For **local Rego file** mode, the configured `authorization.policy.path` may point to a file or directory. At startup, ExtProc will:
1. Validate the configured path by rejecting `../` traversal segments, resolving relative paths against the process working directory, and checking that the path exists
2. Load the modules with `rego.Load(...)` and compile them once with `rego.New(...).PrepareForEval(...)`
3. Return a startup error on compilation failure so invalid policies fail fast
4. Reuse the prepared query for request-time evaluation

For **OPA config file**, the user-provided config file is read and passed directly to `sdk.New()` as the `Config` reader. This supports bundles, discovery, decision logs, etc.

### Decision Path

The SDK `Decision()` method uses a path format like `/aib/extproc/authz/result` (package path + decision document name, with dots replaced by slashes). Configurable via `authorization.policy.package` and `authorization.policy.decision`.

### Alternatives Considered

1. **Low-level `rego` package** — Requires manual policy compilation, no bundle management. Would need custom bundle polling implementation for the OPA config file mode. Rejected: more code, less functionality.
2. **OPA as sidecar** — External OPA server queried via REST API. Rejected: contradicts ExtProc's single-binary deployment model (ADR 20260223-120000).
3. **CEL (already in codebase)** — cel-go is used for token exchange authorization (ADR 20260116-120100). However, CEL lacks the policy management features (bundles, decision logging) and the set-based allow/deny rule composition that OPA provides. OPA is better suited for the multi-tier authorization model from issue #200.

---

## Research Task 2: MCP Protocol Parsing Library

### Decision: Use `github.com/mark3labs/mcp-go` (already a dependency at v0.44.0) for MCP type definitions

### Rationale

`mcp-go` is already a dependency in `go.mod` and is used extensively in the agentgateway E2E tests. However, after analysis, the MCP protocol support needed for OPA authorization is **limited to parsing JSON-RPC messages** — we do not need a full MCP client or server.

The mcp-go library provides type definitions for MCP messages. In particular:
- The library uses JSON-RPC 2.0 as transport, and the request body is a standard JSON-RPC message.
- For `tools/call`, the JSON body has `method: "tools/call"` and `params.name` + `params.arguments`.

**Approach**: Use `mcp-go`'s JSON-RPC types for parsing. The library defines request/response types that can be used for JSON unmarshaling. Specifically, we need:
- Parse the JSON body into a generic JSON-RPC envelope (`jsonrpc`, `method`, `id`, `params`)
- For `tools/call`, extract `params.name` as `tool_name` and `params.arguments`
- For other methods, pass `params` through as-is

The mcp-go library provides `mcp.JSONRPCRequest` and related types. We'll use these for type-safe deserialization rather than raw `json.Unmarshal` into `map[string]any`.

### Alternatives Considered

1. **Raw JSON parsing** (`encoding/json` into `map[string]any`) — Simpler but loses type safety and is more error-prone. Rejected: fragile, no compile-time guarantees.
2. **Custom JSON-RPC types** — Duplicate what mcp-go already provides. Rejected: unnecessary code duplication.
3. **Full MCP server/client integration** — Over-engineered for parsing request bodies. We only need deserialization, not protocol execution.

---

## Research Task 3: Protocol Metadata from agentgateway

### Decision: Use `ProcessingRequest.MetadataContext` for protocol type detection

### Rationale

The Envoy ExtProc protobuf `ProcessingRequest` includes two fields for conveying context:

1. **`MetadataContext`** (`core.v3.Metadata`) — Dynamic metadata associated with the request. Structured as a map of filter names to `google.protobuf.Struct` values.
2. **`Attributes`** (`map[string]*structpb.Struct`) — Selected property values from the data plane.

agentgateway, as a proxy that understands MCP and A2A protocols, can populate `MetadataContext` with the protocol type when routing through ExtProc. The metadata key structure is:

```
metadata_context.filter_metadata["agentgateway"] = {
  "protocol": "mcp"   // or "a2a", "http"
}
```

**Implementation approach**:
1. Extract `MetadataContext` from the `ProcessingRequest` (available on both `RequestHeaders` and `RequestBody` phases)
2. Look for the `agentgateway` filter metadata key
3. Read the `protocol` field value
4. If protocol metadata is missing when OPA is enabled → reject with 403 + log warning

The exact metadata key name (`agentgateway` / `protocol`) will be verified against agentgateway's actual implementation during E2E testing. The key names should be configurable or at least documented.

### E2E Test Metadata Strategy

For E2E tests with the real agentgateway container, the protocol metadata is populated automatically because agentgateway knows it's routing MCP traffic (the backend is declared as `mcp:` type). This means:
- No special test configuration needed — the metadata flows naturally
- If agentgateway doesn't populate this metadata yet, we can use the `attributes` field or fall back to detecting protocol from the backend type header

**Fallback for testing**: If agentgateway doesn't populate ExtProc metadata with protocol info in the current version, we can:
1. Check the request path (e.g., `/mcp`) as a secondary signal
2. Use agentgateway's request headers that may contain protocol hints
3. Configure a custom header in agentgateway that ExtProc reads

---

## Research Task 4: OPA SDK Configuration for Local Files

### Decision: Use file-scheme bundle resource for local Rego files

### Rationale

The OPA SDK documentation explicitly supports loading policy from the filesystem without a bundle server using a `file://` scheme in the bundle resource:

```json
{
  "bundles": {
    "local": {
      "resource": "file:///etc/extproc/policies"
    }
  }
}
```

This approach:
1. Uses the same SDK initialization path for both local and remote policies
2. Leverages OPA's built-in bundle loading (handles compilation, validation)
3. Enables future migration from local to remote bundles without code changes
4. Fails fast on compilation errors (SDK.New returns error)

**Path validation**: Per SR-007, policy file paths must be validated. Before constructing the file:// URI:
- Resolve relative paths to absolute using the working directory
- Reject paths containing `../` traversal sequences
- Verify the path exists and is readable

For the OPA config file mode, the file is read and passed as-is to the SDK — OPA validates it internally.

---

## Research Task 5: Request Body Buffering in ExtProc

### Decision: Buffer body during RequestBody phase, parse before OPA evaluation

### Rationale

Currently, ExtProc echoes the request body unchanged during the `RequestBody` phase. For OPA authorization:

1. **When OPA is disabled** or no Bearer token: Continue echoing body unchanged (FR-017, zero overhead)
2. **When OPA is enabled** and Bearer token is present:
   a. During `RequestHeaders` phase: Extract Bearer token and HTTP metadata, store them in the stream context
   b. For body-bearing requests: perform token exchange in `RequestHeaders`, return the Authorization mutation there, and request the buffered body
   c. During `RequestBody` phase: parse protocol message, build OPA input, evaluate policy
   d. If OPA allows a body-bearing request → echo body (the Authorization header was already mutated in the headers phase)
   e. If OPA denies a body-bearing request → return ImmediateResponse with 403
   f. For header-only requests (`end_of_stream=true`): evaluate OPA first, then perform token exchange only after allow

**Key design change**: body-bearing requests exchange in `RequestHeaders` when OPA is enabled because agentgateway and similar proxies only apply header mutations from the first ExtProc response; body-phase header mutations are dropped.

**Processing mode**: The `RequestHeaders` response must set `mode_override` to request body processing. When OPA is enabled:
```go
// In RequestHeaders response, request body processing:
HeadersResponse{
    Response: &CommonResponse{
        HeaderMutation: ..., // no mutations yet
    },
    ModeOverride: &ProcessingMode{
        RequestBodyMode: ProcessingMode_BUFFERED,
    },
}
```

This tells Envoy/agentgateway to buffer and send the body to ExtProc.

**Max body size**: Bounded by `authorization.max_body_size` (default 1 MiB) per SR-006. If the body exceeds this, it's truncated or the request is rejected.

---

## Research Task 6: E2E Testing Strategy with Real agentgateway

### Decision: Extend existing agentgateway E2E test pattern with OPA-enabled ExtProc and real MCP client

### Rationale

The existing `agentgateway_e2e_test.go` establishes a proven pattern:
```
MCP Client (mcp-go) → agentgateway (Docker) → ExtProc (in-process gRPC) → Mock Identity Broker
                       agentgateway (Docker) → Mock MCP Server (mcp-go)
```

For OPA authorization E2E tests, the architecture extends to:
```
MCP Client (mcp-go) → agentgateway (Docker) → ExtProc+OPA (in-process gRPC) → Mock Identity Broker
                       agentgateway (Docker) → Mock MCP Server (mcp-go)
```

**Test scenarios** (mapped to spec acceptance criteria):

1. **Allowed tool call** — Policy allows `list_repositories`, MCP client calls it, receives result with exchanged token
2. **Denied tool call** — Policy denies `delete_repository`, MCP client receives 403 error
3. **Default deny** — Tool not in any rule, request denied
4. **Non-tool MCP methods** — `initialize`, `notifications/initialized` — allowed by policy
5. **OPA disabled** — No OPA config, behavior identical to existing token exchange tests
6. **Local Rego file** — Load policy from a temp file, verify evaluation
7. **OPA config file** — Use OPA config file with file-scheme bundle, verify evaluation
8. **Invalid policy** — Malformed Rego file, startup fails with clear error

**Key test infrastructure**:
- Reuse `TestEnvironment` pattern from existing tests
- Create temp Rego policy files per test scenario
- Use real `mcp-go` client for MCP protocol fidelity
- Token exchange mocked (Mock Identity Broker) as in existing tests
- OPA decisions are real (embedded OPA engine with test policies)

**MCP client for authorization tests**: The mcp-go library's `client.Client` sends proper MCP JSON-RPC messages through agentgateway. This ensures:
- Real JSON-RPC framing (not hand-crafted test payloads)
- Proper MCP headers (`Mcp-Session-Id`, etc.)
- Realistic protocol flow (initialize → tool calls)

### Alternatives Considered

1. **Unit tests only** — Would miss integration issues between agentgateway metadata, body buffering, and OPA. Rejected: insufficient coverage for the metadata flow.
2. **Mock agentgateway** — Would not test real metadata population. Rejected: defeats the purpose of E2E testing.
3. **Separate OPA sidecar in Docker** — Over-complicated; we use embedded OPA. Rejected: contradicts single-binary model.

---

## Research Task 7: OPA Decision Result Interpretation

### Decision: Type-assert `DecisionResult.Result` to `map[string]any` and extract `action` + `reasons`

### Rationale

The OPA SDK's `Decision()` returns a `DecisionResult` where `Result` is `any`. For our policy pattern:

```rego
result := {"action": "deny", "reasons": ["..."]}
```

The SDK returns `Result` as `map[string]interface{}` (Go's default JSON unmarshaling). The interpretation:

1. **`result.action == "allow"`** → continue request processing (body-bearing requests have already exchanged; header-only requests exchange immediately after allow)
2. **`result.action == "deny"`** → return 403 with reasons
3. **`result.action == "approval_required"` or `"ciba_required"`** → treat as deny in this implementation, log the future-oriented action
4. **`sdk.IsUndefinedErr(err)`** → SDK mode undefined / unready / bad decision path → deny (fail closed)
5. **Error during evaluation** → deny (fail closed)
6. **Timeout** → deny (fail closed, per SR-003)

Define a Go struct for type-safe parsing:
```go
type OPADecision struct {
    Action  string   `json:"action"`
    Reasons []string `json:"reasons,omitempty"`
}
```

Parse `DecisionResult.Result` via JSON round-trip or direct type assertion.

---

## Research Task 8: Agentgateway ExtProc Body Processing Mode

### Decision: Use `mode_override` in HeadersResponse to request body buffering

### Rationale

agentgateway's ExtProc integration follows the Envoy ExtProc protocol. When the ExtProc server wants to inspect the request body, it must signal this in the `HeadersResponse`:

```go
&extprocv3.ProcessingResponse{
    Response: &extprocv3.ProcessingResponse_RequestHeaders{
        RequestHeaders: &extprocv3.HeadersResponse{
            Response: &extprocv3.CommonResponse{},
        },
    },
    ModeOverride: &extprocv3.ProcessingMode{
        RequestBodyMode: extprocv3.ProcessingMode_BUFFERED,
    },
}
```

This tells agentgateway to buffer the request body and send it to ExtProc in a `RequestBody` message. Without this override, the body may not be sent at all (depending on agentgateway's default processing mode).

**Key consideration**: The current agentgateway config uses `failureMode: failClosed`. The processing mode for ExtProc in agentgateway may need to be configured to support body processing. The agentgateway config already routes MCP traffic to ExtProc, and MCP uses POST requests with JSON bodies, so body processing should work.

**Verification needed**: During E2E testing, verify that:
1. agentgateway sends the body to ExtProc when mode_override requests BUFFERED
2. The body contains the complete MCP JSON-RPC message
3. The body is available for both tool calls and other MCP methods
