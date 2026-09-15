# Implementation Plan: Envoy ExtProc Token Exchange Service

**Branch**: `015-extproc-token-exchange` | **Date**: 2026-02-23 | **Spec**: [spec.md](spec.md)
**Input**: Feature specification from `/specs/015-extproc-token-exchange/spec.md`

**Note**: This template is filled in by the `/speckit.plan` command. See `.specify/templates/commands/plan.md` for the execution workflow.
> **Implementation note (superseded raw input):**
> [Feature 043](../043-extproc-metadata-input/contracts/extproc-metadata-input.md) and [ADR 036](../../adrs/036-extproc-metadata-token-exchange-input.md) replace raw `Authorization` and pseudo-header input, no-Bearer pass-through, and raw resource validation.
> This document retains the standalone process, configuration, cache, circuit-breaker, and exchange mechanics that remain applicable.


## Summary

A standalone Go gRPC application implements Envoy ExtProc token exchange. Feature 043 defines metadata-only inputs and outcomes. This feature retains cache, configuration, standalone-process, and exchange mechanics.


## Technical Context

**Language/Version**: Go 1.25.6+ (matching project go.mod)  
**Primary Dependencies**: google.golang.org/grpc, envoyproxy/go-control-plane (ExtProc v3), spf13/cobra, spf13/viper, golang.org/x/sync/singleflight  
**Storage**: In-memory (sync.RWMutex + map) — no database required  
**Testing**: Ginkgo/Gomega BDD framework (separate E2E suite at tests/e2e/extproc/)  
**Target Platform**: Linux containers (Docker), compatible with agentgateway v0.12.0+  
**Project Type**: Single standalone gRPC service  
**Performance Goals**: Sub-10ms header processing (cache hit), <200ms token exchange (cache miss)  
**Constraints**: Must use streaming ExtProc protocol (agentgateway always sends headers + body), failClosed mode  
**Scale/Scope**: Single-binary deployment, in-memory cache, no horizontal scaling required for MVP

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

Before proceeding, verify compliance with [.specify/memory/constitution.md](../../.specify/memory/constitution.md):

**Design Preconditions (BLOCKING)**:

- [x] **Domain Model**: Have entities, aggregates, value objects been identified and documented? → See [data-model.md](data-model.md): Config types, cachedToken, tokenExchangeResponse, clientAssertionEntry. No DB entities (standalone app).
- [x] **Domain Concepts**: Will new domain terms be added to ARCHITECTURE.md Glossary? → Yes: ExtProc, agentgateway, MCP Streamable HTTP, singleflight refresh.
- [x] **Configuration Design**: Have all config requirements been identified with YAML examples? → See [contracts/configuration.md](contracts/configuration.md): full schema with GRPCConfig, OAuth2Config, CacheConfig, LogConfig.
- [x] **Config Examples**: Will example YAML snippets be added to examples/config/? → Yes: `examples/config/extproc-token-exchange.yaml`
- [x] **API Design First**: Will APIs be designed (OpenAPI spec) and confirmed BEFORE implementation? → See [contracts/extproc-grpc.md](contracts/extproc-grpc.md): gRPC contract with all scenarios.
- [ ] **API Documentation**: Will OpenAPI specs be created in `/api/enduser/` or `/api/admin/` as applicable? → N/A: This is a gRPC service, not HTTP REST. Contract documented in contracts/extproc-grpc.md.
- [ ] **API Changes**: Are all API changes confirmed by user/stakeholder (document in PR)? → N/A: No changes to existing APIs. New gRPC service only.
- [ ] **Database Design**: Will all schema changes use go-migrate naming in `/migrations/`? → N/A: No database required (in-memory only).
- [x] **E2E Acceptance Tests**: Will E2E tests be written for ALL spec scenarios BEFORE implementation? → Yes: Separate Ginkgo suite at `tests/e2e/extproc/` with 1:1 mapping to spec scenarios.
- [x] **E2E Test Mapping**: Will each acceptance scenario map 1:1 to one It() block in tests/e2e/? → Yes: 12 scenarios → 12 It() blocks (see Testing Strategy below).
- [x] **E2E Red Phase**: Will E2E tests FAIL initially, proving they test actual functionality? → Yes: Phase 2 produces skeleton E2E tests that compile but fail.

**Implementation Considerations**:

- [x] **Security-First**: Are security features enabled by default? No bypasses or optional security? → Yes: `failClosed` mode, client secrets from env vars, tokens redacted from logs. Requests rejected when exchange fails.
- [x] **Architecture Docs**: Will ARCHITECTURE.md be updated if this touches architecture? → Yes: New standalone application adds ExtProc domain concepts to glossary, new cmd/ entry point.
- [ ] **ADRs**: Does this require an ADR in adrs/ for major decisions? → Potentially: ExtProc streaming architecture pattern. Will evaluate during implementation.
- [x] **Library-First Security**: Are we using vetted libraries for crypto/security (no custom implementations)? → Yes: standard Go net/http TLS, envoyproxy/go-control-plane, golang.org/x/sync/singleflight.
- [ ] **Zalando Guidelines**: Will APIs follow Zalando RESTful API and Event Guidelines? → N/A: gRPC service, not REST.
- [ ] **End-User Docs**: Will API documentation be rendered in `docs/api/` with examples? → Deferred: ExtProc integration guide for docs site is out of scope for initial implementation.
- [ ] **Migration Testing**: Will migrations be tested (apply/rollback) in PostgreSQL integration tests? → N/A: No migrations.
- [x] **Hexagonal Architecture**: Does domain logic use ports (interfaces) with clear adapter separation? → Yes: TokenExchanger interface for exchange logic, injectable HTTP client for testing.
- [ ] **Persistence Patterns**: If adding persistence, will it follow specs/004-persistence-layer/quickstart.md? → N/A: In-memory only.

*All BLOCKING preconditions satisfied. No gate violations.*

## Project Structure

### Documentation (this feature)

```text
specs/[###-feature]/
├── plan.md              # This file (/speckit.plan command output)
├── research.md          # Phase 0 output (/speckit.plan command)
├── data-model.md        # Phase 1 output (/speckit.plan command)
├── quickstart.md        # Phase 1 output (/speckit.plan command)
├── contracts/           # Phase 1 output (/speckit.plan command)
└── tasks.md             # Phase 2 output (/speckit.tasks command - NOT created by /speckit.plan)
```

### Source Code (repository root)

```text
cmd/
├── extproc-token-exchange/
│   ├── main.go                    # Entry point
│   └── root.go                    # Cobra root command, gRPC server lifecycle

internal/
├── extproc/
│   ├── config/
│   │   ├── config.go              # Config struct (GRPCConfig, OAuth2Config, CacheConfig, LogConfig)
│   │   ├── loader.go              # Viper loader with EXTPROC_ prefix
│   │   └── loader_test.go         # Config loading & validation tests
│   └── server/
│       ├── server.go              # ExtProc gRPC server (Process streaming RPC)
│       ├── server_test.go         # Unit tests for ExtProc logic
│       ├── exchanger.go           # TokenExchanger with cache & singleflight
│       └── exchanger_test.go      # Exchange + cache unit tests

mocks/
├── mcp-server/
│   ├── cmd/mcp-server/main.go     # MCP server mock entry point
│   ├── internal/
│   │   ├── handlers/handlers.go   # MCP JSON-RPC handlers (show_claims tool)
│   │   └── server/server.go       # HTTP server setup
│   ├── config.yaml                # Mock configuration
│   ├── go.mod                     # Separate module (mock, not production)
│   └── Dockerfile                 # Multi-stage build
├── agentgateway/
│   └── config.yaml                # agentgateway config (ExtProc + MCP backend)

tests/
├── e2e/
│   └── extproc/
│       ├── extproc_suite_test.go  # Ginkgo suite runner
│       ├── token_exchange_test.go # E2E acceptance tests (12 scenarios)
│       ├── bootstrap/             # ExtProc-specific test bootstrap
│       ├── helpers/               # gRPC client helpers, mock servers
│       └── fixtures/              # Test tokens, configs

examples/
├── config/
│   └── extproc-token-exchange.yaml  # Example configuration
```

**Structure Decision**: Standalone application in `cmd/extproc-token-exchange/` with domain logic in `internal/extproc/`. Follows existing patterns from `cmd/agentic-identity-broker/` and `internal/config/` but with complete separation. MCP server mock follows established pattern from `mocks/sample-agent/`.

## Testing Strategy

### End-to-End (E2E) Acceptance Tests

**Test Location**: `tests/e2e/extproc/` (separate suite — does NOT reuse existing E2E harness per spec requirement FR-017)

**Framework**: Ginkgo/Gomega BDD framework

**Test Organization**:
- **Top-level Describe**: "ExtProc Token Exchange"
- **Nested Describe/Context**: User stories and edge cases
- **It blocks**: Individual acceptance scenarios (one It() per scenario from spec.md)

**Scenario Mapping**:

| Spec Scenario | E2E Test File | Test Description |
|---------------|---------------|------------------|
| Feature 043 metadata input | `metadata_input_test.go` | Validated metadata supplies both exchange inputs |

| US1 Scenario 2 | `token_exchange_test.go` | `It("should replace the Authorization header with the exchanged token")` |
| Feature 043 exchange errors | `metadata_input_test.go` | Fixed matrix rejects failed exchanges without forwarding a credential |

| US2 Scenario 1 | `token_exchange_test.go` | `It("should use cached token for same subject token and resource")` |
| US2 Scenario 2 | `token_exchange_test.go` | `It("should perform fresh exchange when cached token is expired")` |
| US3 Scenario 1 | `token_exchange_test.go` | `It("should bind to configured host/port and log startup summary")` |
| US3 Scenario 2 | `token_exchange_test.go` | `It("should exit with validation error on invalid configuration")` |
| Feature 043 raw attributes | `metadata_input_test.go` | Missing metadata returns the defined rejection |

| Edge: timeout | `token_exchange_test.go` | `It("should return 500 response and log the failure")` |
| Edge: no expiry | `token_exchange_test.go` | `It("should use the default cache TTL")` |
| Edge: singleflight | `token_exchange_test.go` | `It("should perform only one token exchange via singleflight")` |

**Test Data Strategy**:
- Fixtures: test JWT tokens, ExtProc config objects, mock OAuth2 server responses
- New fixtures: mock token exchange endpoint, mock upstream OAuth2 server
- No shared fixtures with existing E2E suite

**Test Execution Flow**:
1. Phase 2 (Design): Write E2E test skeletons with all It() blocks
2. Verify Red Phase: `cd tests/e2e/extproc && ginkgo -v ./...` — all tests must FAIL
3. Implementation: Implement ExtProc server incrementally
4. Verify Green Phase: E2E tests turn GREEN as implementation satisfies criteria

**Bootstrap Strategy**:
- Dedicated bootstrap in `tests/e2e/extproc/bootstrap/` — starts ExtProc gRPC server in-process
- Mock token exchange endpoint (HTTP server returning controlled responses)
- Mock OAuth2 authorization server for client credentials
- Fresh server per test (BeforeEach/AfterEach isolation)

**Helper Utilities**:
- Custom matchers: gRPC response matchers for ProcessingResponse types
- gRPC client helpers: connect to in-process ExtProc server, send ProcessingRequest
- Mock servers: controllable HTTP servers for token exchange and OAuth2

### Unit & Integration Tests

**Unit Tests**:
- `internal/extproc/config/loader_test.go` — Config loading, validation, env var expansion
- `internal/extproc/server/server_test.go` — ExtProc streaming logic and metadata extraction

- `internal/extproc/server/exchanger_test.go` — Token exchange, caching, singleflight

**Integration Tests**:
- N/A: No database or external service adapters. HTTP client behavior tested via mock servers in unit tests.

**Test Coverage Goals**:
- Unit test coverage: Critical paths (config validation, token exchange, cache logic)
- E2E test coverage: 100% of acceptance scenarios from spec.md (12 It() blocks)

## Complexity Tracking

No constitution violations to justify. All blocking preconditions are satisfied.

## Generated Artifacts

| Artifact | Path | Status |
|----------|------|--------|
| Research | [research.md](research.md) | Complete |
| Data Model | [data-model.md](data-model.md) | Complete |
| ExtProc gRPC Contract | [contracts/extproc-grpc.md](contracts/extproc-grpc.md) | Complete |
| Configuration Contract | [contracts/configuration.md](contracts/configuration.md) | Complete |
| Docker Compose Contract | [contracts/docker-compose.md](contracts/docker-compose.md) | Complete |
| Quickstart Guide | [quickstart.md](quickstart.md) | Complete |
| [e.g., Repository pattern] | [specific problem] | [why direct DB access insufficient] |
