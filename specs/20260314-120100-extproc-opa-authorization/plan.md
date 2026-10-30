# Implementation Plan: OPA-Based Authorization in ExtProc

**Branch**: `20260314-120100-extproc-opa-authorization` | **Date**: 2026-03-14 | **Spec**: [specs/20260314-120100-extproc-opa-authorization/spec.md](spec.md)
**Input**: Feature specification from `/specs/20260314-120100-extproc-opa-authorization/spec.md`

## Summary

Add optional Open Policy Agent (OPA) authorization to the ExtProc application. When enabled, body-bearing requests are evaluated in `RequestBody` against a configurable Rego policy, while header-only requests are evaluated in `RequestHeaders`. The protocol type (MCP) is communicated via ExtProc metadata from agentgateway. ExtProc parses MCP JSON-RPC messages and provides structured, protocol-namespaced input to OPA so that policies can reason about tool names and MCP attributes.

**Technical approach**: Embed OPA via the `github.com/open-policy-agent/opa/v1/sdk` package. Use `mcp-go` (already in go.mod) for MCP JSON-RPC type definitions. When OPA is enabled, body-bearing requests exchange first in the headers phase so the Authorization mutation is carried in the headers-phase ExtProc response. Header-only requests evaluate OPA first and exchange only after allow, so `mcp_headers_only` inputs set `granted_permission_sets_available = false` and omit `granted_permission_sets`. HTTP proxies (including agentgateway) apply header mutations only from the headers-phase response — body-phase header mutations are silently dropped. The exchange response also includes `granted_permission_sets` (map of permission set UUID → service UUID array); this is extracted into `requestState`, and `BuildOPAInput` exposes availability explicitly so policies can distinguish authoritative snapshots from omission/pre-exchange inputs.

## Technical Context

**Language/Version**: Go 1.25.6
**Primary Dependencies**: `github.com/open-policy-agent/opa/v1` (OPA SDK — new), `github.com/mark3labs/mcp-go v0.44.0` (MCP types — existing), `github.com/envoyproxy/go-control-plane` (ExtProc proto — existing)
**Storage**: N/A (no persistence — all state is ephemeral per-request; OPA engine is in-memory)
**Testing**: Go `testing` + `testify` for unit tests; Ginkgo/Gomega for E2E; `testcontainers-go` for agentgateway Docker integration; `mcp-go` client for real MCP protocol tests
**Target Platform**: Linux containers (Docker), standalone binary
**Project Type**: Single Go binary (ExtProc standalone, per ADR 20260223-120000)
**Performance Goals**: OPA evaluation adds <10ms p99 latency for typical policies (<50 rules)
**Constraints**: <100ms evaluation timeout (configurable), <1 MiB max body buffer, fail-closed on errors
**Scale/Scope**: Single ExtProc binary, ~8 new source files, ~2 new E2E test files

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

**Design Preconditions (BLOCKING)**:

- [x] **Domain Model**: Value objects identified: OPAInput, OPADecision, MCPMessage. Documented in [data-model.md](data-model.md).
- [x] **Domain Concepts**: OPAInput, OPADecision, Authorizer, ProtocolParser to be added to ARCHITECTURE.md Glossary.
- [x] **Configuration Design**: All config params identified with YAML examples. See [contracts/configuration.md](contracts/configuration.md).
- [x] **Config Examples**: Example YAML to be added to `examples/config/extproc-opa-authorization.yaml`.
- [x] **API Design First**: No REST/HTTP API changes. ExtProc is gRPC-only (Envoy protocol). OPA input schema designed in [contracts/opa-input-schema.md](contracts/opa-input-schema.md).
- [x] **API Documentation**: N/A — no REST API changes. ExtProc uses gRPC ExtProc protocol. OPA input/output schemas documented in contracts/.
- [x] **API Changes**: N/A — no existing API changes. New authorization behavior is opt-in (disabled by default).
- [x] **Database Design**: N/A — no database changes. No migrations needed.
- [x] **E2E Acceptance Tests**: E2E tests will cover all spec scenarios using real agentgateway container + mcp-go client.
- [x] **E2E Test Mapping**: Each acceptance scenario maps 1:1 to an It() block in `tests/e2e/extproc/opa_authorization_test.go`.
- [x] **E2E Red Phase**: E2E tests will be written first and fail semantically before implementation.

**Implementation Considerations**:

- [x] **Security-First**: OPA authorization is disabled by default only because this additive layer has no safe universal default policy; existing token exchange + ADR 20260116-120100 CEL remain authoritative when ExtProc OPA is disabled. When enabled, default decision is deny (fail-closed). Evaluation timeout enforced. Body size bounded.
- [x] **Architecture Docs**: ARCHITECTURE.md Glossary will be updated with OPAInput, OPADecision, Authorizer, ProtocolParser.
- [ ] **ADRs**: An ADR for OPA integration in ExtProc should be created (decision to use OPA SDK vs rego package, embedded vs sidecar).
- [x] **Library-First Security**: OPA is the industry-standard policy engine. No custom policy evaluation. Using official `open-policy-agent/opa` Go module.
- [x] **Zalando Guidelines**: N/A — no REST API. gRPC error responses follow ExtProc protocol conventions.
- [x] **End-User Docs**: Configuration documentation will be added to `docs/configuration.md` and example config.
- [x] **Migration Testing**: N/A — no database migrations.
- [x] **Hexagonal Architecture**: `Authorizer` interface defined as the port. `OPAAuthorizer` is the production adapter. Server depends on the interface. ExtProc is standalone (ADR 20260223-120000) — separate from identity broker hexagonal core.
- [x] **Persistence Patterns**: N/A — no persistence.

## Project Structure

### Documentation (this feature)

```text
specs/20260314-120100-extproc-opa-authorization/
├── plan.md              # This file
├── research.md          # Phase 0: OPA SDK, MCP parsing, metadata, testing research
├── data-model.md        # Phase 1: OPAInput, OPADecision, config types
├── quickstart.md        # Phase 1: Implementation guide
├── contracts/
│   ├── configuration.md       # ExtProc authorization config schema
│   ├── opa-input-schema.md    # OPA input/output JSON schemas
│   └── rego-policy-pattern.md # Recommended Rego policy pattern
└── tasks.md             # Phase 2 output (NOT created by /speckit.plan)
```

### Source Code (repository root)

```text
internal/extproc/
├── authorization/                    # NEW package for OPA integration
│   ├── authorizer.go                 # Authorizer interface + OPAAuthorizer impl
│   ├── authorizer_test.go            # Unit tests for OPAAuthorizer
│   ├── input.go                      # OPAInput, MCPInput, RequestInput, ContextInput types
│   ├── input_builder.go              # BuildOPAInput() — constructs input from protocol + body + headers
│   ├── input_builder_test.go         # Unit tests for input construction
│   ├── decision.go                   # OPADecision type + parsing from sdk.DecisionResult
│   ├── decision_test.go              # Unit tests for decision parsing
│   ├── parser.go                     # MCP protocol parser (JSON-RPC → MCPInput)
│   └── parser_test.go                # Unit tests for MCP parsing
├── config/
│   ├── config.go                     # MODIFIED: add AuthorizationConfig
│   ├── loader.go                     # MODIFIED: add defaults + flags for authorization
│   └── validate.go                   # MODIFIED: add authorization validation rules
└── server/
    ├── server.go                     # MODIFIED: OPA-enabled request processing flow
    └── server_test.go                # MODIFIED: unit tests for OPA-enabled flow

cmd/extproc-token-exchange/
└── root.go                           # MODIFIED: OPA Authorizer initialization + shutdown

tests/e2e/extproc/
├── opa_authorization_test.go         # NEW: OPA authorization E2E tests (agentgateway + MCP)
├── opa_agentgateway_e2e_test.go      # NEW: OPA + agentgateway integration (Ordered suite)
└── fixtures/
    └── policies/                     # NEW: test Rego policy files
        ├── allow_readonly.rego       # Allows read-only tools, denies destructive
        ├── deny_all.rego             # Denies everything
        └── allow_all.rego            # Allows everything

examples/config/
└── extproc-opa-authorization.yaml    # NEW: example config with OPA authorization
```

**Structure Decision**: Follows the existing ExtProc standalone binary structure (per ADR 20260223-120000). The new `authorization/` package lives within `internal/extproc/` and does NOT import from the identity broker's `internal/domain/`, `internal/ports/`, or `internal/adapters/`. The `Authorizer` interface is the hexagonal port within ExtProc's scope.

## Testing Strategy

### End-to-End (E2E) Acceptance Tests

**Test Location**: `tests/e2e/extproc/opa_authorization_test.go` and `tests/e2e/extproc/opa_agentgateway_e2e_test.go`

**Framework**: Ginkgo/Gomega BDD framework following patterns in existing `tests/e2e/extproc/`

**Test Organization**:
- **Top-level Describe**: "OPA Authorization" for in-process tests, "OPA Authorization via Agentgateway" for Docker integration
- **Nested Context**: Protocol type, tool call type, policy decision
- **It blocks**: Individual acceptance scenarios (1:1 mapping to spec.md)

**Scenario Mapping**:

| Spec Scenario | E2E Test File | Test Description |
|---------------|---------------|------------------|
| US1 Scenario 1 (allow `list_repositories`) | `opa_agentgateway_e2e_test.go` | `It("should allow read-only tool call and exchange token")` |
| US1 Scenario 2 (deny `delete_repository`) | `opa_agentgateway_e2e_test.go` | `It("should deny destructive tool call with 403 and reasons")` |
| US1 Scenario 3 (default deny) | `opa_agentgateway_e2e_test.go` | `It("should deny unmatched tool call by default")` |
| US1 Scenario 4 (non-tools/call MCP method) | `opa_agentgateway_e2e_test.go` | `It("should evaluate non-tool-call MCP methods against policy")` |
| US2 Scenario 1 (MCP tool_name extraction) | `opa_authorization_test.go` | `It("should build OPA input with mcp.tool_name for tools/call")` |
| US2 Scenario 2 (unknown protocol) | `opa_authorization_test.go` | `It("should set type=unknown for unrecognized protocol")` |
| US2 Scenario 3 (MCP initialize) | `opa_authorization_test.go` | `It("should build OPA input with mcp.method=initialize")` |
| US3 Scenario 1 (valid local Rego) | `opa_authorization_test.go` | `It("should compile and apply local Rego policy at startup")` |
| US3 Scenario 2 (Rego syntax error) | `opa_authorization_test.go` | `It("should fail startup with clear error for invalid Rego")` |
| US3 Scenario 3 (missing Rego file) | `opa_authorization_test.go` | `It("should fail startup when Rego file path does not exist")` |
| US4 Scenario 1 (OPA config file) | `opa_authorization_test.go` | `It("should initialize OPA from config file with bundle settings")` |
| US4 Scenario 2 (unreachable bundle server) | `opa_authorization_test.go` | `It("should follow OPA retry semantics when bundle server is unreachable")` |
| US5 Scenario 1 (OPA disabled) | `opa_authorization_test.go` | `It("should process requests without OPA when not configured")` |
| US5 Scenario 2 (OPA disabled, Bearer token) | `opa_authorization_test.go` | `It("should skip body inspection when OPA disabled")` |
| Edge: both sources | `opa_authorization_test.go` | `It("should reject config with both local file and OPA config")` |
| Edge: missing metadata | `opa_agentgateway_e2e_test.go` | `It("should reject with 403 when protocol metadata missing")` |
| Edge: evaluation timeout | `opa_authorization_test.go` | `It("should deny on evaluation timeout")` |

**Test Data Strategy**:
- Rego policy files in `tests/e2e/extproc/fixtures/policies/` (reusable across scenarios)
- Use existing `fixtures/tokens.go` for Bearer tokens and resource URIs
- Mock OAuth2 and token exchange servers from existing bootstrap infrastructure
- MCP tool call payloads constructed via real `mcp-go` client (not hand-crafted JSON)

**Test Execution Flow**:
1. Write E2E tests with all spec scenarios
2. Verify Red Phase: `cd tests/e2e/extproc && ginkgo -v --focus="OPA" ./...` — all must FAIL
3. Implement feature incrementally
4. Verify Green Phase: E2E tests turn GREEN as implementation satisfies acceptance criteria

**Bootstrap Strategy**:
- In-process tests: Extend existing `TestEnvironment` with optional `Authorizer`
- agentgateway tests: Follow `agentgateway_e2e_test.go` pattern (Ordered suite, Docker container)
- OPA authorizer created with temp Rego policy files per test scenario
- Real `mcp-go` client and server for protocol fidelity

**Helper Utilities**:
- New matchers: `HaveImmediateResponseWithBody(containing string)` for 403 error body assertions
- Extend `ProcessingRequestBuilder` with `WithRequestBody(body []byte)` for body-phase testing
- Reuse existing `MockOAuth2Server` and `MockTokenExchangeServer`

### Unit Tests

**Location**: `internal/extproc/authorization/*_test.go`, `internal/extproc/server/server_test.go`, `internal/extproc/config/*_test.go`

**Coverage**:
- `authorization/authorizer_test.go` — OPA SDK initialization, decision evaluation, timeout, undefined handling
- `authorization/parser_test.go` — MCP JSON-RPC parsing, tool call extraction, non-tool-call methods, malformed JSON
- `authorization/input_builder_test.go` — OPAInput construction from various protocol+body+header combinations
- `authorization/decision_test.go` — Decision result parsing, action interpretation, reasons extraction
- `server/server_test.go` — OPA-enabled processing flow (mock Authorizer), body buffering, mode_override
- `config/validate_test.go` — New authorization validation rules (mutual exclusivity, path traversal)

**Strategy**: TDD — write tests FIRST, verify they FAIL semantically, then implement.

### E2E Test Architecture (agentgateway integration)

```
┌─────────────────┐     ┌──────────────────────────┐     ┌───────────────────────┐
│   MCP Client    │────▶│   agentgateway (Docker)  │────▶│  ExtProc + OPA (SUT)  │
│   (mcp-go)      │     │   cr.agentgateway.dev/   │     │  (in-process gRPC)    │
│                 │     │   agentgateway:0.12.0     │     │                       │
└─────────────────┘     └──────────┬───────────────┘     └───────────┬───────────┘
                                   │                                 │
                                   ▼                                 ▼
                        ┌──────────────────────┐          ┌─────────────────────┐
                        │  Mock MCP Server     │          │  Mock Identity      │
                        │  (mcp-go, host)      │          │  Broker (httptest)  │
                        └──────────────────────┘          └─────────────────────┘
```

## Complexity Tracking

| Consideration | Assessment | Justification |
|---------------|-----------|---------------|
| New dependency (OPA SDK) | Required | OPA is the industry-standard policy engine. No simpler alternative provides bundle management + set-based rules + embedded evaluation. |
| Body buffering change | Required | OPA authorization needs request body for MCP tool name extraction. Without body, policies cannot inspect tool calls. |
| Processing flow change | Contained | When OPA is enabled, body-bearing requests exchange first in the headers phase and evaluate OPA in the body phase. Header-only requests reverse the order so policy can gate broker calls before any exchange. When OPA is disabled, zero behavioral change. |
| Permission set propagation | Required | `Exchanger` interface returns `ExchangeResult{Token, GrantedPermissionSets}`; `cachedToken` caches both; `requestState` stores both; `BuildOPAInput` forwards permission sets into `context.granted_permission_sets` only when the snapshot is authoritative and exposes `context.granted_permission_sets_available` so policies can fail closed on broker omission or header-only pre-exchange evaluation. |
| ADR needed | Yes | Decision to use OPA SDK (vs rego package, vs sidecar) should be recorded for future reference. |
