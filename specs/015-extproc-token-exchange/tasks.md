# Tasks: Envoy ExtProc Token Exchange Service

**Input**: Design documents from `/specs/015-extproc-token-exchange/`
**Prerequisites**: plan.md ✅, spec.md ✅, research.md ✅, data-model.md ✅, contracts/ ✅, quickstart.md ✅

**Tests**: Per Constitution Principle VIII (Test-Driven Development & Automated Testing), automated tests are MANDATORY for all features. Per spec FR-017, this feature uses a **separate E2E test harness** in `tests/e2e/extproc/` — the existing E2E harness MUST NOT be reused.

**Organization**: Tasks are grouped by user story to enable independent implementation and testing of each story.
> **Implementation note (superseded raw input):**
> [Feature 043](../043-extproc-metadata-input/contracts/extproc-metadata-input.md) and [ADR 036](../../adrs/036-extproc-metadata-token-exchange-input.md) replace raw `Authorization` and pseudo-header input, no-Bearer pass-through, and raw resource validation.
> This document retains the standalone process, configuration, cache, circuit-breaker, and exchange mechanics that remain applicable.


## Format: `[ID] [P?] [Story?] Description`

- **[P]**: Can run in parallel (different files, no dependencies)
- **[Story]**: Which user story this task belongs to (e.g., US1, US2, US3)
- Include exact file paths in descriptions

---

## Phase 1: Setup (Shared Infrastructure)

**Purpose**: Project scaffolding, dependencies, and entry point

- [x] T001 Create cmd entry point in `cmd/extproc-token-exchange/main.go` with Cobra Execute call
- [x] T002 Create Cobra root command with gRPC server lifecycle in `cmd/extproc-token-exchange/root.go`
- [x] T003 Add gRPC and ExtProc dependencies (`google.golang.org/grpc`, `github.com/envoyproxy/go-control-plane`, `golang.org/x/sync/singleflight`) to `go.mod`
- [x] T004 [P] Create example configuration in `examples/config/extproc-token-exchange.yaml` per contracts/configuration.md schema

---

## 🔒 Phase 2: Design Preconditions (Blocking Prerequisites)

**Purpose**: Domain model, configuration, API, and database design MUST all be complete before implementation

**⚠️ CRITICAL**: No code implementation can begin until this entire phase is complete

### Phase 2a: Domain Model & Glossary [MANDATORY]

**Constitution Reference**: Principles II (Architecture Documentation), V (Domain-Driven Design & Glossary Management)

- [x] T005 Document internal value objects (CachedToken, ClientAssertionCache, TokenExchangeResult, tokenCacheKey) in data-model.md — already complete ✅
- [x] T005a Add ExtProc domain terms (ExtProc, agentgateway, MCP Streamable HTTP, singleflight refresh, client assertion) to ARCHITECTURE.md Glossary section
- [x] T005b [P] Document Exchanger port interface and state transitions in data-model.md — already complete ✅

**Checkpoint**: Domain model complete and documented

### Phase 2b: Configuration Design [MANDATORY]

**Constitution Reference**: Principle VII (Configuration-Driven Design)

- [x] T006 Identify all configuration parameters (GRPCConfig, OAuth2Config, TLSConfig, CacheConfig, LogConfig) per contracts/configuration.md — already complete ✅
- [x] T006a Verify example YAML in `examples/config/extproc-token-exchange.yaml` includes all config options with defaults and comments
- [x] T006b [P] Update `examples/config/README.md` to reference ExtProc token exchange configuration section

**Checkpoint**: Configuration requirements designed with YAML examples

### Phase 2c: API Design [MANDATORY]

**Constitution Reference**: Principles IV (API Documentation & OpenAPI Transparency), X (API-First Development)

- [x] T007 Document gRPC contract in contracts/extproc-grpc.md (N/A for OpenAPI — this is a gRPC service, not REST) — already complete ✅
- [x] T007a Document outbound HTTP contracts (client_credentials grant, RFC 8693 token exchange) in contracts/extproc-grpc.md — already complete ✅

**Checkpoint**: gRPC contract and outbound HTTP contracts designed and documented

### Phase 2d: Database Design [MANDATORY]

**Constitution Reference**: Principle IX (Persistence Pattern Consistency & Database Migration Management)

- [x] T008 Confirm no database changes needed — this is an in-memory-only standalone application (no migrations required) ✅

**Checkpoint**: No database schema changes needed

### Phase 2e: Frontend/Design System Review [MANDATORY — N/A]

**Constitution Reference**: Principle XI (Design System Compliance & Consistency)

- [x] T008a Confirm no frontend components needed — this is a backend-only gRPC service with no UI ✅

**Checkpoint**: Frontend/Design System review complete (N/A for this feature)

### Phase 2f: E2E Acceptance Test Design [MANDATORY]

**Constitution Reference**: Principle XIII (End-to-End Acceptance Testing & Spec Traceability)

- [x] T009 Create separate Ginkgo E2E test suite runner in `tests/e2e/extproc/extproc_suite_test.go`
- [x] T009a Write E2E acceptance test skeletons with all `It()` blocks in `tests/e2e/extproc/token_exchange_test.go` mapping 1:1 to spec.md scenarios (12 scenarios)
- [x] T009b [P] Create E2E test bootstrap in `tests/e2e/extproc/bootstrap/` (ExtProc server startup, mock HTTP servers for token exchange and OAuth2)
- [x] T009c [P] Create E2E test helpers in `tests/e2e/extproc/helpers/` (gRPC client helpers, ProcessingRequest builders, ProcessingResponse matchers)
- [x] T009d [P] Create E2E test fixtures in `tests/e2e/extproc/fixtures/` (test tokens, config objects, mock OAuth2 server responses)
- [x] T009e Add comment references to spec scenarios in E2E test files (e.g., `// Spec: US1 Scenario 1`)
- [x] T009f Verify E2E tests compile and FAIL initially (red phase): `cd tests/e2e/extproc && ginkgo -v ./...`

**Checkpoint**: E2E acceptance tests written and verified to fail before implementation

---

## Phase 2.5: Foundational Infrastructure

**Purpose**: Core infrastructure that MUST be complete before ANY user story can be implemented

**⚠️ CRITICAL**: No user story work can begin until this phase is complete

- [x] T010 [P] Create config types (Config, GRPCConfig, OAuth2Config, TLSConfig, CacheConfig, LogConfig) in `internal/extproc/config/config.go` per data-model.md schema
- [x] T011 Create config loader with Viper/Cobra, `EXTPROC_` env prefix, env key replacer, and `os.ExpandEnv` for `${VAR}` notation in `internal/extproc/config/loader.go`
- [x] T012 [P] Create config validation (startup fail-fast rules from data-model.md) in `internal/extproc/config/loader.go`
- [x] T013 [P] Write unit tests for config loading, validation, env var expansion, and default values in `internal/extproc/config/loader_test.go`
- [x] T014 Create Docker configuration for ExtProc service in `config.extproc.docker.yaml` (project root) per contracts/docker-compose.md

**Checkpoint**: Foundation ready — user story implementation can now begin

---

## Phase 3: User Story 1 — Transparent Token Exchange for Agent Requests (Priority: P1) 🎯 MVP

**Goal**: Incoming requests with Bearer tokens get their tokens exchanged for downstream service tokens transparently via the ExtProc gRPC interface

**Independent Test**: Send a request with a Bearer token through ExtProc and verify the outgoing Authorization header contains the exchanged token tied to the request URI

### Tests for User Story 1 [MANDATORY — Principle VIII] ⚠️

> **Constitution Requirement (Principle VIII)**: Tests MUST be written FIRST using TDD. Ensure they FAIL before implementation begins.

- [x] T015 [P] [US1] Write unit tests for ExtProc gRPC streaming logic (RequestHeaders, RequestBody, ResponseHeaders pass-through) in `internal/extproc/server/server_test.go`
- [x] T016 [P] [US1] Write unit tests for token exchange HTTP client (RFC 8693 request format, response parsing, error handling) in `internal/extproc/server/exchanger_test.go`
- [x] T017 [P] [US1] Write unit tests for client assertion acquisition (client_credentials grant, startup fail-fast, ID token extraction) in `internal/extproc/server/exchanger_test.go`

### Implementation for User Story 1

- [x] T018 [US1] Define Exchanger port interface in `internal/extproc/server/exchanger.go` (Exchange method with context, subjectToken, resourceURI; Shutdown method)
- [x] T019 [US1] Implement ExtProc gRPC server with streaming `Process` RPC in `internal/extproc/server/server.go` (RequestHeaders processing, body/trailer pass-through, Register and HealthServer methods)
- [x] T020 [US1] Implement `processRequestHeaders` method in `internal/extproc/server/server.go` (extract Bearer token, extract `:path`, call Exchanger, replace Authorization header or return ImmediateResponse)
- [x] T021 [US1] Implement helper functions in `internal/extproc/server/server.go` (`extractBearerToken`, `extractHeader`, `immediateResponse` for 500/503 error responses)
- [x] T022 [US1] Implement `TokenExchanger` struct with RFC 8693 token exchange HTTP client in `internal/extproc/server/exchanger.go` (doExchange method: build form-encoded request, parse JSON response, return access_token)
- [x] T023 [US1] Implement client assertion acquisition via client_credentials grant in `internal/extproc/server/exchanger.go` (refreshClientAssertion: POST to `{issuer}/oauth/token` or explicit `client_credentials_endpoint`, extract id_token, fail-fast at startup)
- [x] T024 [US1] Implement background client assertion refresh goroutine in `internal/extproc/server/exchanger.go` (refresh at 80% TTL or 30s before expiry, retry on failure)
- [x] T025 [US1] Implement request URI validation in `internal/extproc/server/server.go` (`:path` must be non-empty absolute URI with http/https scheme per contracts/extproc-grpc.md SSRF mitigation; return 503 on invalid)
- [x] T026 [US1] Add structured logging for security-critical operations (token exchange success/failure, client assertion refresh, request rejection) in `internal/extproc/server/server.go` and `internal/extproc/server/exchanger.go`
- [x] T027 [US1] Wire ExtProc server creation and gRPC server lifecycle in `cmd/extproc-token-exchange/root.go` (config load → logger init → server.New → grpc.NewServer → Register → Listen → graceful shutdown)

**Checkpoint**: At this point, User Story 1 should be fully functional — ExtProc exchanges Bearer tokens and replaces Authorization headers

---

## Phase 4: User Story 2 — Token Exchange Cache for Repeated Calls (Priority: P2)

**Goal**: Repeated requests with the same Bearer token and resource reuse cached exchanged tokens, avoiding unnecessary exchanges

**Independent Test**: Perform two requests with identical tokens and resource; verify the second reuses the cached token without a new exchange call

### Tests for User Story 2 [MANDATORY — Principle VIII] ⚠️

> **Constitution Requirement (Principle VIII)**: Tests MUST be written FIRST using TDD. Ensure they FAIL before implementation begins.

- [x] T028 [P] [US2] Write unit tests for token cache (cache hit, cache miss, cache expiry, default TTL fallback, max TTL cap) in `internal/extproc/server/exchanger_test.go`
- [x] T029 [P] [US2] Write unit tests for singleflight deduplication (concurrent requests for same key trigger one exchange) in `internal/extproc/server/exchanger_test.go`
- [x] T030 [P] [US2] Write unit tests for background cache eviction goroutine (expired entries removed) in `internal/extproc/server/exchanger_test.go`

### Implementation for User Story 2

- [x] T031 [US2] Implement in-memory token cache with `sync.RWMutex` + `map[tokenCacheKey]*cachedToken` in `internal/extproc/server/exchanger.go` (struct key: `{subjectToken, resourceURI}` — no hash, direct map key)
- [x] T032 [US2] Integrate `singleflight.Group` for concurrent cache refresh deduplication in `internal/extproc/server/exchanger.go`
- [x] T033 [US2] Implement cache TTL logic: use `expires_in` from exchange response, fall back to `cache.default_ttl` when absent or ≤0, cap at `cache.max_ttl` in `internal/extproc/server/exchanger.go`
- [x] T034 [US2] Implement background cache eviction goroutine (ticker at `default_ttl / 2`, sweep expired entries) in `internal/extproc/server/exchanger.go`
- [x] T035 [US2] Wire cache lookup into Exchange method: check cache before calling doExchange, store result after successful exchange in `internal/extproc/server/exchanger.go`

**Checkpoint**: At this point, User Stories 1 AND 2 should both work — token exchange with caching and singleflight

---

## Phase 5: User Story 3 — Operable Configuration and Startup Validation (Priority: P3)

**Goal**: The ExtProc service starts predictably with valid configuration and fails fast on invalid configuration

**Independent Test**: Launch the service with valid configuration and verify it starts; launch with invalid configuration and verify startup fails with a clear error

### Tests for User Story 3 [MANDATORY — Principle VIII] ⚠️

> **Constitution Requirement (Principle VIII)**: Tests MUST be written FIRST using TDD. Ensure they FAIL before implementation begins.

- [x] T036 [P] [US3] Write unit tests for startup validation (all 10 validation rules from contracts/configuration.md, including TLS enforcement for non-`allow_http` mode) in `internal/extproc/config/loader_test.go`
- [x] T037 [P] [US3] Write unit tests for startup logging summary (bind address, port, token_endpoint, issuer — client_secret redacted) in `internal/extproc/server/server_test.go`

### Implementation for User Story 3

- [x] T038 [US3] Implement TLS enforcement validation: `token_endpoint` and `issuer` must use `https://` scheme unless `oauth2.tls.allow_http: true` in `internal/extproc/config/loader.go`
- [x] T039 [US3] Implement configurable HTTP client with TLS settings (insecure_skip_verify, ca_bundle_path) in `internal/extproc/server/exchanger.go`
- [x] T040 [US3] Implement exchange_timeout for outbound HTTP calls in `internal/extproc/server/exchanger.go`
- [x] T041 [US3] Implement max_concurrent_streams configuration on gRPC server in `cmd/extproc-token-exchange/root.go`
- [x] T042 [US3] Implement startup logging summary with sensitive field redaction (client_secret masked) in `cmd/extproc-token-exchange/root.go`

**Checkpoint**: All user stories should now be independently functional

---

## Phase 6: Docker Compose & Mocks Integration

**Goal**: Full integration environment with agentgateway, ExtProc, MCP server mock, and existing identity broker services

- [x] T043 Create MCP server mock entry point in `mocks/mcp-server/cmd/mcp-server/main.go` (HTTP server on port 9003 with MCP Streamable HTTP transport)
- [x] T044 Implement MCP JSON-RPC handlers (initialize, tools/list, tools/call for `show_claims` tool) in `mocks/mcp-server/internal/handlers/handlers.go`
- [x] T045 [P] Implement MCP HTTP server setup in `mocks/mcp-server/internal/server/server.go`
- [x] T046 [P] Create MCP server mock configuration in `mocks/mcp-server/config.yaml`
- [x] T047 [P] Create MCP server mock `go.mod` (separate module) in `mocks/mcp-server/go.mod`
- [x] T048 [P] Create MCP server mock Dockerfile in `mocks/mcp-server/Dockerfile` (multi-stage build)
- [x] T049 Create agentgateway configuration (ExtProc policy + MCP backend) in `mocks/agentgateway/config.yaml` per contracts/docker-compose.md
- [x] T050 Add three new services to `docker-compose.yml`: agentgateway (ghcr.io/agentgateway/agentgateway:v0.12.0), extproc-token-exchange (Dockerfile.mock), mcp-server-mock (Dockerfile.mock) per contracts/docker-compose.md
- [x] T051 [P] Add `just` targets for ExtProc development: `just extproc-build`, `just extproc-run`, `just test-e2e-extproc` in `justfile`

**Checkpoint**: `docker compose up` starts full integration environment with transparent token exchange

---

## Phase 7: Sample Agent MCP Button

**Goal**: End-to-end demo showing token exchange from sample agent through agentgateway to MCP server

- [x] T052 Add `/call-mcp` HTTP handler to sample agent that sends MCP tool call through agentgateway in `mocks/sample-agent/` (reads user access token, calls agentgateway `/mcp` with JSON-RPC initialize + tools/call for `show_claims`)
- [x] T053 Add "Call MCP Tool (Token Exchange)" button to sample agent home page UI in `mocks/sample-agent/`

**Checkpoint**: End-to-end demo works via sample agent → agentgateway → ExtProc → identity broker → MCP server

---

## 🔒 Phase 8: Constitution Compliance & Polish [MANDATORY COMPLIANCE SECTION]

**Purpose**: Verify constitution requirements and final polish

### 🔒 Constitution Compliance Verification [MANDATORY]

#### Design Phase Verification [MANDATORY]

- [x] T054 Verify domain model terms added to ARCHITECTURE.md Glossary (Principle V)
- [x] T055 Verify configuration example exists in `examples/config/extproc-token-exchange.yaml` (Principle VII)
- [x] T056 [P] Verify configuration example referenced in `examples/config/README.md` (Principle VII)
- [x] T057 Verify gRPC contract documented in `specs/015-extproc-token-exchange/contracts/extproc-grpc.md` (Principles IV, X — N/A for OpenAPI)
- [x] T058 Confirm no database changes needed (Principle IX) — in-memory only ✅
- [x] T059 Verify E2E acceptance tests written in `tests/e2e/extproc/` for all 12 spec scenarios (Principle XIII)
- [x] T060 Verify E2E tests verified to FAIL before implementation (red phase) (Principle XIII)

#### Implementation Phase Verification [MANDATORY]

**Architecture & Documentation** (Principle II):
- [x] T061 Update ARCHITECTURE.md with ExtProc standalone application architecture section
- [x] T062 [P] Evaluate need for ADR: ExtProc streaming architecture pattern (ADR in `adrs/011-extproc-standalone-binary.md` or `adrs/012-extproc-in-memory-token-cache.md` — may already exist)

**Configuration** (Principle VII):
- [x] T063 [P] Verify ExtProc configuration uses standalone Viper loader with `EXTPROC_` prefix (not shared config port)

**Security** (Principles I, III):
- [x] T064 Verify security features enabled by default: failClosed mode, TLS enforcement, token redaction in logs (Principle I)
- [x] T065 [P] Verify no custom cryptography — standard Go crypto, go-control-plane, singleflight only (Principle III)
- [x] T066 [P] Verify structured logging for security-critical operations: token exchange failures, assertion refresh, request rejection

**Architecture Patterns** (Principle VI):
- [x] T067 Verify domain logic uses Exchanger port interface with clear adapter separation (Principle VI)

**Testing** (Principles VIII, XIII):
- [x] T068 Verify unit tests written FIRST and failed before implementation (red-green TDD)
- [x] T069 Verify E2E tests exist in `tests/e2e/extproc/` for all 12 acceptance scenarios from spec.md
- [x] T070 Verify each `It()` block maps to exactly ONE acceptance scenario
- [x] T071 Verify E2E tests use Ginkgo/Gomega framework with hierarchical structure (Describe → Context → It)
- [x] T072 Verify E2E tests include comment references to spec scenarios
- [x] T073 Run full E2E test suite: `cd tests/e2e/extproc && ginkgo -v ./...` (all tests must pass)

### Additional Polish

- [x] T074 Code cleanup and refactoring across all ExtProc packages
- [x] T075 [P] Run quickstart.md validation: verify implementation matches quickstart steps
- [x] T076 Run `just check` (fmt, vet, lint) and `just verify` — all checks must pass

---

## Dependencies & Execution Order

### Phase Dependencies

- **Setup (Phase 1)**: No dependencies — can start immediately
- **Design Preconditions (Phase 2)**: Depends on Setup completion — BLOCKS all implementation
  - Phase 2a, 2b, 2c, 2d, 2f can proceed in parallel, but all must complete before Phase 2.5 begins
  - **CRITICAL**: E2E acceptance tests must be written and verified to FAIL (red phase)
- **Foundational Infrastructure (Phase 2.5)**: Depends on ALL of Phase 2 completion — BLOCKS all user stories
- **User Story 1 (Phase 3)**: Depends on Phase 2 + Phase 2.5 — core token exchange, no other story dependencies
- **User Story 2 (Phase 4)**: Depends on Phase 2 + Phase 2.5 — extends US1 with caching (can start after US1 Exchange method exists)
- **User Story 3 (Phase 5)**: Depends on Phase 2 + Phase 2.5 — extends US1 with validation/startup (can start after config loader exists)
- **Docker & Mocks (Phase 6)**: Depends on US1 being functional — needs working ExtProc binary
- **Sample Agent (Phase 7)**: Depends on Phase 6 — needs Docker environment running
- **Polish (Phase 8)**: Depends on all phases complete

### User Story Dependencies

- **User Story 1 (P1)**: Can start after Phase 2 + Phase 2.5 — no dependencies on other stories
- **User Story 2 (P2)**: Extends US1's exchanger with caching — best implemented after US1 T018/T022 (Exchanger interface + doExchange)
- **User Story 3 (P3)**: Extends config loader with validation — best implemented after Phase 2.5 T011 (config loader)

### Within Each User Story

- Tests MUST be written and FAIL before implementation
- Interface/port definitions before implementations
- Core logic before helper functions
- Server wiring after all components exist

### Parallel Opportunities

- T001, T003, T004 (Setup) can run in parallel after T002
- T009, T009b, T009c, T009d (E2E test design) can run in parallel
- T010, T012, T013 (Foundational) can run in parallel after T011
- T015, T016, T017 (US1 tests) can run in parallel
- T028, T029, T030 (US2 tests) can run in parallel
- T036, T037 (US3 tests) can run in parallel
- T043-T048 (Mock creation tasks marked [P]) can run in parallel
- Phase 6 Docker tasks and Phase 5 US3 can run in parallel

---

## Parallel Example: User Story 1

```bash
# Launch all US1 tests together (they write to different test files/sections):
Task T015: "Unit tests for ExtProc gRPC streaming in internal/extproc/server/server_test.go"
Task T016: "Unit tests for token exchange client in internal/extproc/server/exchanger_test.go"
Task T017: "Unit tests for client assertion in internal/extproc/server/exchanger_test.go"

# Then implement sequentially:
Task T018: Define Exchanger interface (other tasks depend on this)
Task T019: Implement ExtProc gRPC server
Task T020-T021: Request header processing and helpers
Task T022-T024: Token exchange, client assertion, background refresh
Task T025-T027: URI validation, logging, cmd wiring
```

---

## Implementation Strategy

### MVP First (User Story 1 Only)

1. Complete Phase 1: Setup (cmd entry point, dependencies)
2. Complete Phase 2: Design Preconditions (config, contracts, E2E skeletons)
3. Complete Phase 2.5: Foundational Infrastructure (config types, loader, validation)
4. Complete Phase 3: User Story 1 (ExtProc gRPC server + token exchange)
5. **STOP and VALIDATE**: Verify ExtProc exchanges tokens correctly via unit tests + E2E red→green
6. Deploy/demo if ready

### Incremental Delivery

1. Setup → Design → Foundation ready
2. Add User Story 1 → Token exchange works → MVP!
3. Add User Story 2 → Caching + singleflight → Performance optimized
4. Add User Story 3 → Startup validation + TLS → Production-ready config
5. Add Docker & Mocks → Full integration demo
6. Add Sample Agent button → End-to-end demo
7. Each story adds value without breaking previous stories

### Parallel Team Strategy

With multiple agents:

1. **Agent A**: Phase 1 Setup + Phase 2.5 Foundational
2. **Agent B**: Phase 2 E2E test skeletons (T009 series)
3. **Agent C**: Phase 6 MCP Server Mock (T043-T048, independent Go module)
4. After foundation ready:
   - **Agent A**: User Story 1 (core exchange)
   - **Agent B**: User Story 3 (config validation, can start once config loader exists)
5. After US1 complete:
   - **Agent A**: User Story 2 (caching, extends US1)
   - **Agent B**: Docker Compose integration (Phase 6 T049-T051)
6. Final: Phase 7 Sample Agent + Phase 8 Polish

---

## Notes

- [P] tasks = different files, no dependencies
- [Story] label maps task to specific user story for traceability
- Each user story should be independently completable and testable
- This feature is a **standalone application** — it does not share domain objects with the identity broker
- The E2E test harness is **separate** from the existing `tests/e2e/` suite (FR-017)
- Cache key uses Go struct (`tokenCacheKey{subjectToken, resourceURI}`) as map key — no hash, no separator injection risk
- Client assertion acquired at **startup** (fail-fast) and refreshed in background goroutine
- The `id_token` from client_credentials response is used as `client_assertion` (not `access_token`)
- Verify tests fail before implementing
- Commit after each task or logical group
- Stop at any checkpoint to validate story independently
