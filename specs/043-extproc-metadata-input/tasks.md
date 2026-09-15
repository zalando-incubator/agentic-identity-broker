---

description: "Task list for ExtProc Metadata Input implementation"
---

# Tasks: ExtProc Metadata Input

**Input**: Design documents from `/specs/043-extproc-metadata-input/`

**Prerequisites**: `plan.md`, `spec.md`, `research.md`, `data-model.md`, `contracts/extproc-metadata-input.md`, and `quickstart.md`

**Tests**: Automated tests are mandatory under Constitution Principles VIII and XIII. Write the listed unit and E2E tests first, verify that they fail semantically, then implement the production changes.

**Organization**: Tasks are grouped by user story. Phase 2 acceptance tests deliberately cover every story before production implementation because they are blocking constitution preconditions.

## Phase 1: Setup (Shared Infrastructure)

**Purpose**: Verify the existing standalone ExtProc service already provides the required protobuf dependencies; no project initialization, runtime configuration, or schema setup change is applicable.

- [X] T001 Verify existing protobuf metadata dependencies cover the feature and require no initialization change in `go.mod`

---

## 🔒 Phase 2: Design Preconditions (Blocking Prerequisites) [MANDATORY]

**Purpose**: Complete the blocking ADR, wire-contract/architecture alignment, and semantic-red acceptance-test prerequisites; retain explicit N/A determinations for domain entities, runtime configuration, public APIs, persistence, and frontend work.

**Critical**: No production code implementation may begin until this phase is complete.

### Phase 2a: Domain Model & Glossary [MANDATORY]

**Constitution Reference**: Principles II and V

- [X] T002 [P] Confirm the wire-only model and absence of new domain entities, aggregates, IDs, and domain terms in `specs/043-extproc-metadata-input/data-model.md`
- [X] T003 [P] Draft the accepted trust-boundary decision, validation precedence, producer ordering, and clean-cutover consequences in `adrs/036-extproc-metadata-token-exchange-input.md`

**Checkpoint**: The wire contract, terminology, invariants, and governing ADR are documented.

### Phase 2b: Configuration Design [MANDATORY]

**Constitution Reference**: Principle VII

- [X] T004 Verify the explicit N/A determination that metadata fields are request data, not ExtProc configuration, and require no `EXTPROC_`, example-config, Helm, or chart change in `specs/043-extproc-metadata-input/plan.md`

**Checkpoint**: Configuration impact is explicitly closed with no custom configuration path.

### Phase 2c: API Design [MANDATORY]

**Constitution Reference**: Principles IV and X

- [X] T005 [P] Confirm the Envoy dynamic-metadata wire contract is already specified and that neither public OpenAPI document changes in `specs/043-extproc-metadata-input/contracts/extproc-metadata-input.md`

**Checkpoint**: The changed wire contract is defined before its implementation; public HTTP APIs are unaffected.

### Phase 2d: Database Design [MANDATORY]

**Constitution Reference**: Principle IX

- [X] T006 Verify the explicit N/A determination that the stateless ExtProc metadata-input change requires no persistence entity, repository, or migration in `specs/043-extproc-metadata-input/plan.md`

**Checkpoint**: Database scope is explicitly confirmed as unchanged.

### Phase 2e: Frontend/Design System Review [MANDATORY]

**Constitution Reference**: Principle XI

- [X] T007 Verify the explicit N/A determination that this standalone gRPC and gateway-policy feature changes no React surface or design-system component in `specs/043-extproc-metadata-input/plan.md`

**Checkpoint**: Frontend and design-system scope is explicitly confirmed as not applicable.

### Phase 2f: E2E Acceptance Test Design [MANDATORY]

**Constitution Reference**: Principle XIII

- [X] T008 Add composable `WithTokenExchangeMetadata(subjectToken, resourceURI)` metadata construction while preserving the protocol namespace in `tests/e2e/extproc/helpers/grpc_helpers.go`
- [X] T009 [P] Add testcontainer-generated, per-suite RS256 key generation, JWKS serialization, and token minting support for gateway tests in `tests/e2e/extproc/fixtures/jwks.go`
- [X] T010 Write direct-gRPC Ginkgo acceptance coverage for US1-S2 through US1-S4, US2-S2, US3-S1 through US3-S2, and every specified edge case in `tests/e2e/extproc/metadata_input_test.go`
- [X] T011 Write Docker Agentgateway Ginkgo acceptance coverage for US1-S1 and US2-S1 with the testcontainer-generated JWKS from `tests/e2e/extproc/fixtures/jwks.go` and an `Ordered` shared container in `tests/e2e/extproc/agentgateway_metadata_e2e_test.go`
- [X] T012 Verify the new metadata-input E2E tests compile and fail semantically, with one scenario reference and concrete assertion per `It()`, in `tests/e2e/extproc/metadata_input_test.go` and `tests/e2e/extproc/agentgateway_metadata_e2e_test.go`

**Checkpoint**: Every acceptance scenario has a traceable, semantically failing E2E test before production behavior changes.

---

## Phase 2.5: Foundational Infrastructure

**Purpose**: Establish one metadata extraction and validation primitive shared by both ExtProc header-processing paths.

- [X] T013 Add table-driven unit tests for metadata namespace and field presence, protobuf kind checks, blank values, `Bearer ` prefixes, validation precedence, verbatim token propagation, and both header paths in `internal/extproc/server/server_test.go`
- [X] T014 [P] Update expected source-neutral resource-URI validation diagnostics in `internal/extproc/server/security_test.go`
- [X] T015 Add the metadata namespace and field constants plus the single `extractTokenExchangeInput` validation path with typed credential-free rejections in `internal/extproc/server/server.go`
- [X] T016 Add fixed 503 JSON response builders for `invalid_subject_token` and `invalid_resource`, and reword `validateResourceURI` diagnostics without source attributes, in `internal/extproc/server/server.go`

**Checkpoint**: Both header paths can use the same fail-closed, byte-preserving metadata-input primitive.

---

## Phase 3: User Story 1 - Exchange From Dynamic Metadata (Priority: P1) 🎯 MVP

**Goal**: Exchange only the validated `aib.tokenexchange` subject token and resource URI, including when raw attributes conflict.

**Independent Test**: Send direct ExtProc requests with both metadata values and conflicting raw headers or pseudo-headers; assert that the stub exchanger receives precisely the metadata values, and that exchange failure forwards no credential.

### Tests for User Story 1

> The test code is authored in Phase 2. T017 confirms its US1 subset was semantically red before this phase changes production behavior; green execution is reserved for Phase N.

- [X] T017 [US1] Verify the Phase 2 unit and direct-gRPC acceptance cases for US1-S2 through US1-S4 have semantic-red assertions before implementing the US1 request-processing changes in `internal/extproc/server/server.go`, as recorded in `internal/extproc/server/server_test.go` and `tests/e2e/extproc/metadata_input_test.go`

### Implementation for User Story 1

- [X] T018 [US1] Rewrite the non-OPA `processRequestHeaders` path to validate metadata before exchange and replace downstream authorization only after success in `internal/extproc/server/server.go`
- [X] T019 [US1] Rewrite the OPA `processRequestHeadersOPA` and header-only OPA flow to validate metadata before protocol or OPA checks and carry only validated inputs in `requestState` in `internal/extproc/server/server.go`
- [X] T020 [US1] Rename `requestState.bearerToken`, delete `extractBearerToken` and `buildResourceURI`, remove their pass-through and `:path` semantics, and update metadata-only processing comments in `internal/extproc/server/server.go`

**Checkpoint**: US1 source is ready for Phase N direct-metadata acceptance execution without raw token-exchange input attributes.

---

## Phase 4: User Story 2 - Configure Instance Resource (Priority: P2)

**Goal**: Configure every Agentgateway instance to publish the intended protected-resource URI and a JWT-validated subject token to ExtProc.

**Independent Test**: Route a valid RS256 JWT through an Agentgateway instance with its configured metadata producer; assert that the mock broker receives the configured absolute HTTP(S) resource URI.

### Tests for User Story 2

> The test code is authored in Phase 2. T021 confirms its US1-S1 and US2 semantic-red assertions before this phase changes gateway configuration; green execution is reserved for Phase N.

- [X] T021 [US2] Verify the Phase 2 gateway acceptance cases for US1-S1 and US2-S1/S2 have semantic-red assertions before editing gateway configuration in `tests/e2e/extproc/agentgateway_metadata_e2e_test.go`, `tests/e2e/extproc/metadata_input_test.go`, and `mocks/agentgateway/config.yaml`

### Implementation for User Story 2

- [X] T022 [P] [US2] Add strict JWT-before-ExtProc policies, explicit `preserveToken: false`, the testcontainer-generated JWKS mount from `tests/e2e/extproc/fixtures/jwks.go`, and the `aib.tokenexchange` producer to generated container configurations in `tests/e2e/extproc/agentgateway_e2e_test.go` and `tests/e2e/extproc/opa_agentgateway_e2e_test.go`
- [X] T023 [P] [US2] Add the strict JWT policy and metadata producer plus separate committed, non-secret Compose JWKS and private-key fixtures in `mocks/agentgateway/config.yaml`, `mocks/agentgateway/jwks.json`, and `mocks/agentgateway/jwks-dev-key.pem`
- [X] T024 [US2] Mount the committed Compose JWKS fixture from `mocks/agentgateway/jwks.json` read-only beside the Agentgateway configuration in `docker-compose.yml`
- [X] T025 [US2] Add post-implementation assertions that the metadata resource URI reaches the exchanger byte-for-byte and remains a cache-key component, then align metadata fixture documentation in `internal/extproc/server/server_test.go`, `internal/extproc/server/exchanger_test.go`, `internal/extproc/server/exchanger_cache_test.go`, and `tests/e2e/extproc/fixtures/tokens.go`
- [X] T025a [US2] After T009 and T022, replace hard-coded bearer values with per-suite RS256 JWTs minted by `tests/e2e/extproc/fixtures/jwks.go` in all real-gateway flows: the shared `connectAgentgwMCPClient` used by `tests/e2e/extproc/agentgateway_e2e_test.go` and `tests/e2e/extproc/agentgateway_otel_e2e_test.go`, plus the separate OPA client/helper in `tests/e2e/extproc/opa_agentgateway_e2e_test.go`. Complete this migration before T050.

**Checkpoint**: Gateway and Compose configurations are ready for Phase N resource-URI acceptance execution, and every generated-gateway client supplies a valid JWT.

---

## Phase 5: User Story 3 - Remove Raw Attribute Dependency (Priority: P3)

**Goal**: Complete the clean cutover so raw authorization and request-target attributes cannot supply either token-exchange input.

**Independent Test**: Send a request that has a raw Authorization header and raw target attributes but no token-exchange metadata; assert an `invalid_subject_token` 503 and no exchanger call or forwarded credential.

### Tests for User Story 3

> The test code is authored in Phase 2. T026 confirms its raw-attribute rejection assertions were semantically red before the metadata cutover; green execution is reserved for Phase N.

- [X] T026 [US3] Verify the Phase 2 raw-attribute-only rejection and no-pass-through cases for US3-S1 and US3-S2 have semantic-red assertions before migrating legacy fixtures in `internal/extproc/server/server_test.go` and `tests/e2e/extproc/metadata_input_test.go`

### Implementation for User Story 3

- [X] T027 [US3] Delete obsolete raw-input and pass-through assertions, then migrate remaining ExtProc unit request fixtures to metadata in `internal/extproc/server/server_test.go` and `internal/extproc/server/circuit_breaker_test.go`
- [X] T028 [US3] Migrate ExtProc E2E request fixtures to metadata while retaining raw `:method` only for OPA transport checks in `tests/e2e/extproc/token_exchange_test.go`, `tests/e2e/extproc/opa_authorization_test.go`, `tests/e2e/extproc/request_trace_context_test.go`, `tests/e2e/extproc/telemetry_test.go`, and `tests/e2e/extproc/agentgateway_otel_e2e_test.go`

**Checkpoint**: Legacy fixtures are metadata-only and ready for Phase N raw-attribute rejection execution.

---

## Phase 6: Polish & Cross-Cutting Documentation

**Purpose**: Align operator, architecture, and local ExtProc guidance with the accepted metadata-only trust boundary.

- [X] T029 [P] Document the Agentgateway producer block, strict JWT ordering, `.unredacted()` requirement, URI rules, and development-key minting procedure in `docs/guides/token-exchange-gateway.md`
- [X] T030 [P] Replace all raw-attribute ExtProc flow descriptions and amend the ExtProc glossary entry in `ARCHITECTURE.md`
- [X] T031 [P] Replace the obsolete resource-URI construction invariant and no-Bearer narrative with the metadata-only ADR 036 invariant in `internal/extproc/AGENTS.md`

**Checkpoint**: Operators and maintainers receive one accurate description of the metadata-only contract.

---

## 🔒 Phase N: Constitution Compliance & Polish [MANDATORY COMPLIANCE SECTION]

**Purpose**: Verify the completed feature satisfies binding design, security, architecture, TDD, and E2E requirements.

### 🔒 Design Phase Verification [MANDATORY]

#### Domain Model & Glossary (Principles II and V)

- [X] T032 Verify the wire-only model introduces no domain entities, IDs, or new ubiquitous terms and that the ExtProc glossary amendment is planned in `specs/043-extproc-metadata-input/data-model.md` and `ARCHITECTURE.md`

#### Configuration & Helm (Principle VII)

- [X] T033 Verify the contract adds no runtime configuration key, sidecar example change, or Helm rendering requirement in `specs/043-extproc-metadata-input/plan.md` and `charts/agentic-identity-broker/values.yaml`

#### API Contract (Principles IV and X)

- [X] T034 Verify the Envoy wire contract is defined before implementation and that no end-user or admin OpenAPI change or stakeholder API approval is applicable in `specs/043-extproc-metadata-input/contracts/extproc-metadata-input.md`, `api/enduser/openapi.yaml`, and `api/admin/openapi.yaml`

#### Persistence (Principle IX)

- [X] T035 Verify the stateless ExtProc change adds no persisted entity, repository, or migration in `specs/043-extproc-metadata-input/plan.md` and `migrations/`

#### Frontend/Design System (Principle XI)

- [X] T036 Verify the feature has no frontend surface, design-system component, Playwright scenario, or screenshot requirement in `specs/043-extproc-metadata-input/plan.md` and `web/src/design-system/`

#### E2E Acceptance Design (Principle XIII)

- [X] T037 Verify every acceptance scenario has one traceable E2E `It()` with concrete semantic-red assertions and no new pending markers in `tests/e2e/extproc/metadata_input_test.go` and `tests/e2e/extproc/agentgateway_metadata_e2e_test.go`

#### Decision Record (Principle II)

- [X] T038 Verify ADR 036 is accepted and records the trust boundary, rejected alternatives, validation precedence, producer ordering, and clean cutover in `adrs/036-extproc-metadata-token-exchange-input.md`

### 🔒 Implementation Phase Verification [MANDATORY]

#### API & Documentation (Principles IV and X)

- [X] T039 Verify that the implementation changes only the declared Envoy metadata contract and leaves both public API specifications unchanged in `specs/043-extproc-metadata-input/contracts/extproc-metadata-input.md`, `api/enduser/openapi.yaml`, and `api/admin/openapi.yaml`

#### Architecture & Documentation (Principles II and V)

- [X] T040 Verify the metadata-only flow, obsolete raw-attribute removal, ExtProc glossary, and local invariant agree across `ARCHITECTURE.md`, `internal/extproc/AGENTS.md`, and `adrs/036-extproc-metadata-token-exchange-input.md`

#### Configuration & Helm (Principle VII)

- [X] T041 Verify the metadata producer is gateway policy data rather than a new ExtProc setting and that no Helm change is required in `mocks/agentgateway/config.yaml`, `examples/config/extproc-token-exchange.yaml`, and `charts/agentic-identity-broker/values.yaml`

#### Database & Persistence (Principle IX)

- [X] T042 Verify the implementation introduces no storage adapter, repository, schema, or migration change in `internal/extproc/`, `internal/ports/storage.go`, and `migrations/`

#### Security & Library-First Controls (Principles I and III)

- [X] T043 Verify both header paths fail closed, use no raw input fallback, expose no credential or rejected URI in telemetry or responses, and use library-first JOSE in `tests/e2e/extproc/fixtures/jwks.go`


#### Hexagonal Boundaries & Wiring (Principles VI and XII)

- [X] T044 Verify the standalone ExtProc service retains no broker-domain, port, or adapter import and needs no builder or HTTP-routing wiring change in `internal/extproc/AGENTS.md`, `adrs/011-extproc-standalone-binary.md`, and `internal/app/builder.go`

#### Unit Tests & TDD (Principle VIII)

- [X] T045 Verify table-driven unit tests were written and semantically failed before the extraction implementation, then cover valid, invalid, boundary, and precedence behavior in `internal/extproc/server/server_test.go`, `internal/extproc/server/security_test.go`, and `specs/043-extproc-metadata-input/tasks.md`

#### E2E Acceptance Testing (Principle XIII)

- [X] T046 Verify all acceptance scenarios are green through production ExtProc behavior, use isolated state and a fresh MCP client per gateway `It()`, retain one-scenario-per-`It()` traceability, and change only fixtures after red in `tests/e2e/extproc/metadata_input_test.go` and `tests/e2e/extproc/agentgateway_metadata_e2e_test.go`


#### Frontend/Design System (Principle XI)

- [X] T047 Verify no frontend component, custom styling, Playwright test, or screenshot was added because the feature remains backend and gateway-only in `specs/043-extproc-metadata-input/plan.md` and `web/`

#### Final Verification

- [X] T048 Run static checks with `just check` from `Justfile` and resolve every reported issue
- [X] T049 Run the race-enabled ExtProc unit suite with `just extproc-test` from `Justfile`
- [X] T050 Run the ExtProc E2E suite with `GOEXPERIMENT=jsonv2 go run github.com/onsi/ginkgo/v2/ginkgo -v --procs=4 ./tests/e2e/extproc/` and confirm all metadata-input scenarios are green with the per-suite minted JWTs
- [X] T051 Start the Compose validation path with `just compose-extproc-up` from `Justfile` and confirm the configured Agentgateway reaches a ready state
- [X] T052 Run the documented operator validation and rejection paths from `specs/043-extproc-metadata-input/quickstart.md` without exposing a credential or raw resource URI in output; `GOEXPERIMENT=jsonv2 go run github.com/onsi/ginkgo/v2/ginkgo -v --focus "ExtProc Metadata Input" ./tests/e2e/extproc/` passed 11 focused metadata specs, including exact rejection-response assertions, and the Compose local-client flow completed `Initialize` and `whoami` after a user grant and third-party session were created

---

## Dependencies & Execution Order

### Phase Dependencies

- **Phase 1** has no dependencies; it confirms the deliberately empty setup surface.
- **Phase 2** follows Phase 1 and blocks production changes: T008 supplies metadata construction, T009 supplies only an uncommitted testcontainer JWKS, T010 and T011 author acceptance tests, and T012 proves the semantic-red state.
- **Phase 2.5** starts only after all Phase 2 preconditions. T013 and T014 are test-first work; T015 and T016 implement the shared validation foundation.
- **US1 (Phase 3)** depends on Phase 2.5. T017 confirms its red tests before T018 through T020 establish metadata-only processing; Phase N runs the direct-gRPC cases green.
- **US2 (Phase 4)** depends on Phase 2.5. T021 confirms its red gateway tests before the independently startable generated testcontainer configuration T022 and committed Compose configuration T023; T024 depends on T023, T025a depends on the T009 fixture and T022 configuration, and T025 verifies resource propagation only after T018 through T020 and T022 through T024 are complete.
- **US3 (Phase 5)** depends on T018 through T020 because legacy fixture cleanup can begin only after raw-input fallback has been removed; T026 confirms the preceding red test evidence before T027 through T028 migrate fixtures.
- **Phase 6** and **Phase N** require all selected user-story work to be complete.

### User Story Completion Order

```text
Phase 2.5 ──┬──→ US1 red proof (T017) → US1 core (T018–T020) ──→ US3 red proof (T026) → US3 cleanup (T027–T028)
            └──→ US2 red proof (T021) ──┬──→ generated testcontainer gateway fixture (T022)
                                         └──→ committed Compose configuration (T023) → Compose JWKS mount (T024)
US1 core (T018–T020) + US2 configuration (T022–T024) ──→ resource/cache verification (T025)
T009 + generated gateway configuration (T022) ──→ minted-JWT client migration (T025a) ──→ Phase N gateway acceptance
US1 core + US2 configuration + US3 cleanup → Documentation/Polish → Constitution Compliance
```

### Explicit Task Edges

- `T017 → T018–T020`: US1 semantic-red evidence precedes its request-processing implementation.
- `T021 → T022/T023`: US2 semantic-red gateway evidence precedes its independently startable configuration tasks.
- `T023 → T024`: the Compose JWKS mount requires the committed Compose JWKS fixture.
- `T018–T020 + T022–T024 → T025`: resource propagation and cache-key assertions require both the metadata processor and gateway configuration.
- `T009 + T022 → T025a`: every generated-gateway client must use the per-suite minted JWT once Strict validation is enabled.
- `T026 → T027–T028`: US3 semantic-red evidence precedes legacy-fixture cleanup.
- `T029–T031 → T032–T052`: documentation alignment precedes final constitution-compliance verification.

- **US1 (P1)** proves semantic-red direct tests before its processor change; its final real-gateway US1-S1 acceptance run additionally requires T022's JWT/metadata fixture.
- **US2 (P2)** configuration starts after Phase 2.5 in parallel with US1 core work; only its resource/cache verification and gateway acceptance/green completion depend on US1's processor changes.
- **US3 (P3)** is not independent: it verifies its red contract before legacy cleanup, then migrates fixtures only after US1 removes raw-attribute input.


## Parallel Execution Examples

### Fixture and gateway configuration work

```text
Task: "Draft ADR 036 in adrs/036-extproc-metadata-token-exchange-input.md"
Task: "Generate per-suite testcontainer JWKS in tests/e2e/extproc/fixtures/jwks.go"
Task: "Add committed Compose JWKS in mocks/agentgateway/jwks.json"
```

### User Story 2 configuration

```text
Task: "Update generated testcontainer gateway policies in tests/e2e/extproc/agentgateway_e2e_test.go"
Task: "Add the separate Compose gateway policy and JWKS in mocks/agentgateway/config.yaml"
```

### Documentation alignment

```text
Task: "Update operator guidance in docs/guides/token-exchange-gateway.md"
Task: "Update architecture flow and glossary in ARCHITECTURE.md"
Task: "Update ExtProc invariants in internal/extproc/AGENTS.md"
```

## Implementation Strategy

### MVP: US1 Processor Plus Required Gateway Fixture

1. Complete Phases 1 and 2, including semantically failing E2E acceptance tests.
2. Complete the Phase 2.5 shared metadata validation primitive and its unit tests.
3. Complete T017, then implement the US1 processor through T018 to T020 in `internal/extproc/server/server.go`.
4. Complete the US2 gateway red proof and generated testcontainer fixture in `tests/e2e/extproc/agentgateway_e2e_test.go` before the final US1-S1 gateway acceptance run; reserve green commands for Phase N.

### Incremental Delivery

1. Deliver US1 to establish and prove the fail-closed metadata input boundary.
2. Deliver US2 to make both testcontainer and Compose Agentgateway instances publish the required metadata.
3. Deliver US3 to remove legacy raw-attribute test assumptions and demonstrate no fallback remains.
4. Complete documentation, then run every Phase N verification command before considering the feature complete.

## Notes

- `[P]` identifies tasks in distinct files with no dependency on incomplete work.
- `[US1]`, `[US2]`, and `[US3]` provide story traceability; precondition and compliance tasks intentionally carry no story label.
- No OpenAPI, ExtProc configuration schema, Helm, frontend, persistence, or broker-domain implementation task is included because the approved plan explicitly rules each out.
