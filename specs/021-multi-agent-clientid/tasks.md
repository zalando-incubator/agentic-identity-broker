# Tasks: Multi-Agent OAuth2 Client Delegation

**Input**: Design documents from `/specs/021-multi-agent-clientid/`
**Prerequisites**: plan.md ✅, research.md ✅, data-model.md ✅, contracts/admin-api-changes.md ✅, quickstart.md ✅

**Tests**: Per Constitution Principle VIII (TDD), unit and E2E tests MUST be written before implementation. All 14 E2E acceptance scenarios must be in red phase before Phase 3+ implementation begins.

**Organization**: Tasks are grouped by user story to enable independent implementation and testing of each story.

---

## Phase 1: Setup

**Purpose**: Verify existing design artifacts and project structure before implementation begins.

- [X] T001 Verify specs/021-multi-agent-clientid/ contains research.md, data-model.md, contracts/admin-api-changes.md, quickstart.md, and plan.md

---

## 🔒 Phase 2: Design Preconditions (Blocking Prerequisites) [MANDATORY]

**Purpose**: All design preconditions are complete per plan.md (Phases 0 and 1 done). This phase verifies and finalises the remaining design artifacts before implementation.

**⚠️ CRITICAL**: No code implementation can begin until this entire phase is complete.

### Phase 2a: Domain Model & Glossary [MANDATORY]

- [X] T002 [P] Update ARCHITECTURE.md Glossary with MultiAgentClientConfig and resolveAgentIdByClientId definitions per data-model.md §Glossary Additions
- [X] T003 [P] Confirm data-model.md is complete: MultiAgentClientConfig struct, CELEvaluatorConfig extension, MultiAgentTokenVerifier, migration 008, state-transition flows

**Checkpoint**: Domain model documented in ARCHITECTURE.md

### Phase 2b: Configuration Design [MANDATORY]

- [X] T004 [P] Add multi_agent_client block (enabled and disabled examples) to examples/config/oauth2-authorization-server.yaml per quickstart.md §Feature Modes
- [X] T005 [P] Add updated CEL expression examples to examples/config/token-exchange.yaml per contracts/admin-api-changes.md §Token Exchange CEL Policy

**Checkpoint**: Config examples committed

### Phase 2c: API Design [MANDATORY]

- [X] T006 [P] Update /api/enduser/openapi.yaml: change client_id parameter description on /oauth2/authorize to document UUID requirement per contracts/admin-api-changes.md §1
- [X] T007 [P] Update /api/admin/openapi.yaml: update POST /api/agents and PUT /api/agents/{agent-id} descriptions with client_id uniqueness conditional per contracts/admin-api-changes.md §2

**Checkpoint**: OpenAPI specs updated; breaking changes confirmed acknowledged in plan.md

### Phase 2d: Database Design [MANDATORY]

- [X] T008 [P] Create migrations/008_drop_agent_client_id_unique.up.sql: DROP UNIQUE constraint on agents.client_id, DROP old index, CREATE non-unique index idx_agents_client_id per data-model.md §Migration 008
- [X] T009 [P] Create migrations/008_drop_agent_client_id_unique.down.sql: DROP index, CREATE UNIQUE INDEX, ADD CONSTRAINT agents_client_id_key per data-model.md §Migration 008

**Checkpoint**: Migration files created and verified against data-model.md

### Phase 2e: Frontend/Design System Review [N/A]

Backend-only feature per plan.md §Technical Context. No frontend changes.

### Phase 2f: E2E Acceptance Test Design [MANDATORY]

> **Constitution Requirement (Principle XIII)**: E2E tests MUST be written before implementation and verified to FAIL (red phase).

- [X] T010 Create tests/e2e/multi_agent_client_test.go with all 14 Ginkgo It() blocks per plan.md §Scenario Mapping table (US1×6, US2×4, US3×4); add spec scenario comment references in each It()
- [X] T011 [P] Extend tests/e2e/helpers/ (MockUpstreamOAuth2Server) to support ReturnTokenWithClaim(claimName, value string) for configurable JWT token responses
- [X] T012 [P] Add multi-agent fixtures to tests/e2e/fixtures/: two agents sharing client_id "shared-upstream", multi-agent enabled/disabled config helpers, bootstrap.StorageFactory seeding
- [X] T013 Add ContainAgentIDClaim(claimName, agentID string) Gomega matcher to tests/e2e/matchers/ for verifying JWT claim values in token response bodies
- [X] T014 Tests compile with one caveat: fixtures/multi_agent.go and multi_agent_client_test.go reference ports.MultiAgentClientConfig (added in Phase 2.5 T015/T016) — 4 expected compile errors. All 14 tests will fail semantically once Phase 2.5 adds the type. Tests compile cleanly within each story's scope (US1 S1–S6, US2 S1–S4, US3 S1–S4).
  - **Test fix (2026-03-20)**: US2 "when enabled" BeforeEach extended with GitHubService + UserGrant storage setup (matching token_exchange_test.go pattern). `"resource"`, `"client_assertion"`, `"client_assertion_type"` added to US2 S1/S2/S3 form data. Subject tokens updated to use full RFC 8693 claims (`iss`, `aud`, `exp`, `iat`) matching mock upstream. All 14 tests compile clean. **Final red-phase state: 11 FAIL (behavioral) | 3 PASS (correct greens)** — no false greens, no infrastructure failures.

**Checkpoint**: 14 E2E tests exist, compile, and fail semantically before any implementation

---

## Phase 2.5: Foundational Infrastructure

**Purpose**: Config + domain type extensions that ALL user stories depend on. Must complete before any US implementation.

**⚠️ CRITICAL**: No user story work can begin until this phase is complete.

- [X] T015 Add MultiAgentClientConfig struct to internal/ports/config.go nested under OAuth2AuthServerConfig (fields: Enabled bool, AgentIDParamName string, AgentIDClaimName string, all mapstructure tagged)
- [X] T016 Add MultiAgentClient MultiAgentClientConfig field to OAuth2AuthServerConfig struct in internal/ports/config.go
- [X] T017 [P] Add MultiAgentClient MultiAgentClientConfig field to OAuth2Config struct in internal/domain/oauth2/service.go
- [X] T018 [P] Add ResolveAgentIDByClientID func(clientID string) (agentID string, err error) field to CELEvaluatorConfig struct in internal/domain/tokenexchange/cel_evaluator.go

**Checkpoint**: Config types and domain struct extensions compile; all existing tests still pass

---

## Phase 3: User Story 1 — Multi-Agent Client Sharing: Authorize + Token (Priority: P1) 🎯 MVP

**Goal**: Multiple agents sharing an upstream OAuth2 client_id can each authorize independently. Each authorize redirect appends the agent's UUID as a query param; each token response is verified to contain a matching agent ID claim before being forwarded.

**Independent Test**: Run `ginkgo -v --label-filter="US1" ./tests/e2e/...` — all 6 US1 scenarios pass.

### Tests for User Story 1 [MANDATORY - Principle VIII] ⚠️

> **Constitution Requirement (Principle VIII)**: Tests MUST be written FIRST using TDD. Ensure they FAIL before implementation begins.

- [X] T019 [P] [US1] Write unit tests for HandleAuthorization() UUID resolution in internal/domain/oauth2/service_test.go: valid agent UUID resolves correct agent; non-UUID client_id returns invalid_client; non-existent UUID returns invalid_client
- [X] T020 [P] [US1] Write unit tests for proxyToUpstream() claim verification in internal/adapters/http/enduser/oauth2_token_test.go: buffered write when MultiAgentVerifier nil (feature disabled); 500 error when VerifyAgentIDClaim returns claim-absent error; 500 error when VerifyAgentIDClaim returns claim-mismatch error; success when VerifyAgentIDClaim returns nil

### Implementation for User Story 1

- [X] T021 [US1] In internal/domain/oauth2/service.go HandleAuthorization(): replace agentRepo.GetByClientID(req.ClientID) with id.ParseAgentID(string(req.ClientID)) + agentRepo.Get(agentID); return invalid_client on UUID parse error or not-found
- [X] T022 [P] [US1] In internal/domain/oauth2/service.go buildUpstreamAuthorizeURL(): add q.Set(s.config.MultiAgentClient.AgentIDParamName, agent.ID.String()) when MultiAgentClient.Enabled, immediately before u.RawQuery = q.Encode()
- [X] T023 [P] [US1] Add structured audit log event AgentIDParamInjected (Info, fields: agent_id, param_name, upstream_url) in internal/domain/oauth2/service.go alongside the param injection
- [X] T024 [US1] Implement MultiAgentTokenVerifier struct in internal/domain/oauth2/multi_agent_verifier.go with VerifyAgentIDClaim(ctx context.Context, responseBody []byte, expectedAgentID id.AgentID) error using jwt.ParseInsecure from lestrrat-go/jwx/v3/jwt
- [X] T025 [US1] Define MultiAgentVerifier interface with single method VerifyAgentIDClaim(ctx context.Context, responseBody []byte, expectedAgentID id.AgentID) error; add MultiAgentVerifier field (nilable — nil means feature disabled) on OAuth2TokenHandler struct in internal/adapters/http/enduser/oauth2_token.go
- [X] T026 [US1] In internal/adapters/http/enduser/oauth2_token.go proxyToUpstream(): extract client_id from token request form data; buffer upstream response body (replace io.Copy with io.ReadAll + bytes.Buffer); call MultiAgentVerifier.VerifyAgentIDClaim when verifier non-nil; write buffered body on success or OAuth2 error on failure
- [X] T027 [P] [US1] Add structured audit log events AgentIDClaimVerified (Info), AgentIDClaimMissing (Error, fields: agent_id, claim_name), AgentIDClaimMismatch (Error, fields: expected_agent_id, claim_value, claim_name) in internal/adapters/http/enduser/oauth2_token.go

**Checkpoint**: US1 complete — all 6 E2E scenarios pass, `just test` passes, no regression

---

## Phase 4: User Story 2 — Token Exchange via Agent ID Resolution (Priority: P2)

**Goal**: Token exchange policies can resolve agent identity either via `resolveAgentIdByClientId(clientId)` CEL function (feature disabled) or directly from a JWT claim (feature enabled). Both paths use `agentRepo.Get(agentID)` (UUID lookup).

**Independent Test**: Run `ginkgo -v --label-filter="US2" ./tests/e2e/...` — all 4 US2 scenarios pass.

### Tests for User Story 2 [MANDATORY - Principle VIII] ⚠️

> **Constitution Requirement (Principle VIII)**: Tests MUST be written FIRST using TDD. Ensure they FAIL before implementation begins.

- [X] T028 [P] [US2] Write unit tests for resolveAgentIdByClientId CEL function in internal/domain/tokenexchange/cel_evaluator_test.go: function evaluates correctly with mock resolver; returns types.NewErr on unknown clientID; function NOT registered when ResolveAgentIDByClientID is nil
- [X] T029 [P] [US2] Write unit tests for agent lookup in internal/domain/tokenexchange/service_test.go: Get(agentID) used instead of GetByClientID(); UUID parse error returns token exchange error (invalid_request or invalid_target)

### Implementation for User Story 2

- [X] T030 [US2] In internal/domain/tokenexchange/service.go (~line 227): replace GetByClientID(id.NewClientID(agentClientID)) with id.ParseAgentID(agentClientID) + Get(agentID); return appropriate token exchange error on UUID parse failure
- [X] T031 [US2] In internal/domain/tokenexchange/cel_evaluator.go compileExpression(): when e.config.ResolveAgentIDByClientID != nil, register resolveAgentIdByClientId as a CEL unary function via cel.Function() + cel.UnaryBinding() calling the injected closure; return types.NewErr on lookup failure

**Checkpoint**: US2 complete — all 4 E2E scenarios pass, `just test` passes, no regression

---

## Phase 5: User Story 3 — Configuration Validation & Admin API Uniqueness (Priority: P3)

**Goal**: The broker fails fast with a clear error at startup when multi_agent_client is enabled but param/claim names are missing. The admin API enforces client_id uniqueness only when feature is disabled.

**Independent Test**: Run `ginkgo -v --label-filter="US3" ./tests/e2e/...` — all 4 US3 scenarios pass.

### Tests for User Story 3 [MANDATORY - Principle VIII] ⚠️

> **Constitution Requirement (Principle VIII)**: Tests MUST be written FIRST using TDD. Ensure they FAIL before implementation begins.

- [X] T032 [P] [US3] Write unit tests for OAuth2AuthServerConfig.Validate() in internal/ports/config_test.go: enabled + empty AgentIDParamName → startup error; enabled + empty AgentIDClaimName → startup error; disabled + empty fields → no error
- [X] T033 [P] [US3] Write unit tests for admin agent handler client_id uniqueness in internal/adapters/http/handlers/admin/agents_test.go: 409 Conflict when !multiAgentEnabled and duplicate client_id on Create; 409 Conflict when !multiAgentEnabled and duplicate client_id on Update (excluding self); no 409 when multiAgentEnabled and duplicate client_id

### Implementation for User Story 3

- [X] T034 [US3] Add validation in OAuth2AuthServerConfig.Validate() in internal/ports/config.go: when Enabled=true and AgentIDParamName=="" → return error "oauth2_authorization_server.multi_agent_client.agent_id_param_name is required"; same for AgentIDClaimName
- [X] T035 [US3] Add multiAgentEnabled bool field to AgentsHandler struct in internal/adapters/http/handlers/admin/agents.go and update constructor/NewAgentsHandler to accept it
- [X] T036 [US3] In internal/adapters/http/handlers/admin/agents.go Create handler: when !multiAgentEnabled, call agentRepo.GetByClientID(ctx, clientID); return 409 Conflict if agent found
- [X] T037 [US3] In internal/adapters/http/handlers/admin/agents.go Update handler: same conditional uniqueness check as Create, excluding the current agent.ID from the conflict check

**Checkpoint**: US3 complete — all 4 E2E scenarios pass, `just test` passes, no regression

---

## Phase 6: Builder Wiring & Integration Tests

**Purpose**: Wire all feature components in the DI container and verify migration in a real PostgreSQL integration test.

- [X] T038 In internal/app/builder.go: pass MultiAgentClientConfig from cfg.OAuth2AuthServer.MultiAgentClient through to oauth2.NewServiceWithSessions() via OAuth2Config
- [X] T039 In internal/app/builder.go: build resolverFn closure (calls agentRepo.GetByClientID with 5s context timeout) when !cfg.OAuth2AuthServer.MultiAgentClient.Enabled; inject as CELEvaluatorConfig.ResolveAgentIDByClientID
- [X] T040 In internal/app/builder.go: build and inject MultiAgentTokenVerifier into OAuth2TokenHandler when cfg.OAuth2AuthServer.MultiAgentClient.Enabled
- [X] T041 [P] In internal/app/builder.go: inject cfg.OAuth2AuthServer.MultiAgentClient.Enabled as multiAgentEnabled into AgentsHandler constructor
- [X] T042 [P] Write PostgreSQL integration test in internal/adapters/storage/postgres/agent_repository_migration008_test.go: apply migration 008, verify two agents can share client_id; apply only 001-007, verify duplicate client_id rejected by DB constraint

**Checkpoint**: `just verify` passes and `just build` succeeds

---

## Phase 7: Documentation

- [X] T043 [P] Document compatibility changes in contracts/admin-api-changes.md §Compatibility Changes (client_id semantics, CEL expression update, new multi_agent_client block, resolveAgentIdByClientId function)
- [X] T044 [P] Update examples/config/token-exchange.yaml with updated agent_id_expression examples (resolveAgentIdByClientId for disabled mode, subject_token.x_agent_id for enabled mode)
- [X] T077 [P] Add multi_agent_client configuration documentation to docs/configuration.md: describe enabled/disabled modes, all three parameters (enabled, agent_id_param_name, agent_id_claim_name), startup validation behaviour, and link to examples/config/oauth2-authorization-server.yaml (Constitution Principle VII)
- [X] T078 [P] Update examples/config/README.md to reference the new multi_agent_client block in oauth2-authorization-server.yaml and note the required token-exchange.yaml CEL expression update (Constitution Principle VII)

**Checkpoint**: Docs complete, all breaking changes documented

---

## 🔒 Phase N: Constitution Compliance & Polish [MANDATORY]

**Purpose**: Verify all constitution principles followed; final validation gate before feature is considered done.

### 🔒 Constitution Compliance Verification [MANDATORY]

#### Design Phase Verification [MANDATORY]

- [X] T045 Verify ARCHITECTURE.md Glossary contains MultiAgentClientConfig and resolveAgentIdByClientId entries (Principle V)
- [X] T046 Verify examples/config/oauth2-authorization-server.yaml contains multi_agent_client block with enabled and disabled examples (Principle VII)
- [X] T047 Verify /api/enduser/openapi.yaml client_id parameter description updated per contracts/ (Principles IV, X)
- [X] T048 Verify /api/admin/openapi.yaml agent client_id uniqueness note updated per contracts/ (Principles IV, X)
- [X] T049 Verify breaking changes acknowledged by stakeholder (documented in plan.md §Constitution Check and contracts/admin-api-changes.md) (Principle X)
- [X] T050 Verify migrations/008_drop_agent_client_id_unique.up.sql and .down.sql exist and match data-model.md (Principle IX)
- [X] T051 Verify 14 E2E acceptance tests exist in tests/e2e/multi_agent_client_test.go and were verified to FAIL before implementation (Principle XIII)

#### Implementation Phase Verification [MANDATORY]

**API & Documentation** (Principles IV, X):
- [X] T052 [P] Verify /api/enduser/openapi.yaml implementation matches contracts/admin-api-changes.md §1 exactly
- [X] T053 [P] Verify /api/admin/openapi.yaml implementation matches contracts/admin-api-changes.md §2 exactly

**Architecture & Documentation** (Principle II):
- [X] T054 Verify ARCHITECTURE.md Glossary additions are present (T002 complete)
- [X] T055 [P] Confirm no new ADR needed (pattern follows existing CEL and DI patterns per plan.md §Constitution Check)

**Configuration** (Principle VII):
- [X] T056 [P] Verify MultiAgentClientConfig uses internal/ports/config.go (not custom loading); Viper mapstructure tags present

**Database & Persistence** (Principle IX):
- [X] T057 [P] Verify migration files follow go-migrate naming convention: 008_drop_agent_client_id_unique.up.sql and .down.sql
- [X] T058 [P] Verify integration test in internal/adapters/storage/postgres/ covers migration 008 apply, rollback, and shared client_id acceptance

**Security** (Principles I, III):
- [X] T059 Verify token is withheld (fail closed) when agent ID claim is absent or mismatched — FR-004, SR-001
- [X] T060 [P] Verify resolveAgentIdByClientId CEL function is NOT registered when MultiAgentClient.Enabled=true — FR-010, SR-005
- [X] T061 [P] Verify audit log events AgentIDParamInjected, AgentIDClaimVerified, AgentIDClaimMissing, AgentIDClaimMismatch are emitted — SR-004

**Architecture Patterns** (Principle VI):
- [X] T062 Verify internal/domain/oauth2/service.go does not import adapters/
- [X] T063 Verify MultiAgentTokenVerifier is injected into OAuth2TokenHandler (not instantiated inside domain)
- [X] T064 Verify resolver closure is injected into CELEvaluatorConfig by builder.go (not built inside domain)
- [X] T065 Verify all wiring is in internal/app/builder.go (Principle XII)

**Testing** (Principle VIII — Unit & Integration Tests):
- [X] T066 Verify unit tests in service_test.go, oauth2_token_test.go, cel_evaluator_test.go were written FIRST (red-green TDD)
- [X] T067 Verify unit tests changed minimally during implementation
- [X] T068 Run `just verify` — full verification gate passes

**E2E Acceptance Testing** (Principle XIII):
- [X] T069 Verify each of the 14 It() blocks in tests/e2e/multi_agent_client_test.go maps to exactly ONE acceptance scenario from spec.md
- [X] T070 Verify tests/e2e/multi_agent_client_test.go uses Ginkgo/Gomega following tests/e2e/README.md patterns
- [X] T071 Verify hierarchical structure: Describe (feature) → Context (preconditions) → It (scenario)
- [X] T072 Verify spec scenario comment references present in each It() block
- [X] T073 Verify E2E tests changed minimally during implementation (fixture adjustments only)
- [X] T074 Run full E2E suite: `ginkgo -v ./tests/e2e/` — all tests pass including all 14 multi-agent scenarios

### Additional Polish

- [X] T075 [P] Run `just check` (fmt → vet → lint) and `just verify` — all checks pass
- [ ] T076 [P] Validate quickstart.md steps work end-to-end against running broker (feature disabled and enabled modes)

---

## Dependencies & Execution Order

### Phase Dependencies

- **Phase 1 (Setup)**: No dependencies — start immediately
- **Phase 2 (Design Preconditions)**: Depends on Phase 1. Sub-phases 2a–2f can run in parallel; ALL must complete before Phase 2.5
- **Phase 2.5 (Foundational)**: Depends on ALL Phase 2 completion — blocks all user stories
- **User Stories (Phase 3–5)**: All depend on Phase 2 + Phase 2.5 completion; can proceed in parallel if staffed
- **Phase 6 (Builder Wiring)**: Depends on Phase 3 + Phase 4 + Phase 5 completion (all component types needed for wiring)
- **Phase 7 (Docs)**: Can run in parallel with Phase 3–6
- **Phase N (Compliance)**: Depends on all preceding phases complete

### User Story Dependencies

- **US1 (P1)**: Depends on Phase 2.5. Changes oauth2/service.go (authorize path) + oauth2_token.go (token path)
- **US2 (P2)**: Depends on Phase 2.5. Changes tokenexchange/service.go + cel_evaluator.go. Independent of US1 files.
- **US3 (P3)**: Depends on Phase 2.5. Changes ports/config.go Validate() + admin agents.go. Independent of US1 and US2.

### Within Each User Story

- Unit tests written and FAILING before implementation tasks (TDD)
- Service/domain changes before adapter changes
- Handler changes after service changes

### Parallel Opportunities

- Phase 2a–2f can all run in parallel (separate files)
- T008 + T009 (migration files) parallel
- T015–T018 (Phase 2.5 type additions) all parallel
- US1, US2, US3 can be implemented in parallel by different agents
- T019 + T020 (US1 tests) parallel; T028 + T029 (US2 tests) parallel; T032 + T033 (US3 tests) parallel
- T022 + T023 + T024 (US1 param injection + audit log) parallel
- T038–T041 (builder wiring) can be done in one pass; T042 (integration test) parallel

---

## Parallel Example: User Story 1

```bash
# Write unit tests in parallel (before implementation):
Task T019: "Write unit tests for HandleAuthorization() UUID resolution in internal/domain/oauth2/service_test.go"
Task T020: "Write unit tests for proxyToUpstream() claim verification in internal/adapters/http/enduser/oauth2_token_test.go"

# After T021 (breaking change) lands, run in parallel:
Task T022: "Add agent ID param injection in buildUpstreamAuthorizeURL() in internal/domain/oauth2/service.go"
Task T023: "Add AgentIDParamInjected audit log in internal/domain/oauth2/service.go"
```

---

## Implementation Strategy

### MVP First (User Story 1 Only — Phase 3)

1. Complete Phase 1: Setup (fast — just verification)
2. Complete Phase 2 sub-phases in parallel: T002–T014
3. Complete Phase 2.5: T015–T018 (type extensions — small, foundational)
4. Write US1 unit tests (T019–T020), verify they FAIL
5. Implement US1 (T021–T027)
6. Verify US1 E2E scenarios 1–6 pass
7. **STOP and VALIDATE**: `just check` and `just verify` pass; US1 works end-to-end

### Incremental Delivery

1. Phase 1 + Phase 2 + Phase 2.5 → Foundational types ready
2. Phase 3 (US1) → Breaking change live, param injection, token claim verification → MVP
3. Phase 4 (US2) → Token exchange CEL function → Full feature parity
4. Phase 5 (US3) → Config validation + admin uniqueness → Operational safety
5. Phase 6 → Builder wiring ties all components together → Integration complete
6. Phase N → Compliance gate → Feature done

### Parallel Team Strategy

With multiple developers or agents:
1. **Phase 2 (Design finalisation)**: Run 2a–2f in parallel — use different files
2. **Phase 2.5 (Types)**: Single pass — all 4 struct additions small, commit together
3. **Phase 3–5 (User Stories)**: Assign to separate agents simultaneously (no file overlap between US1/US2/US3 except builder.go which comes after)
4. **Phase 6 (Builder wiring)**: Single pass after all US complete
5. **Phase N (Compliance)**: Parallel verification tasks (all marked [P])

---

## Notes

- [P] tasks = different files, no dependencies on incomplete tasks
- [USn] label maps task to specific user story for traceability
- Breaking change (client_id → agent.id UUID) affects ALL existing OAuth2 clients — no migration shim
- US1 and US2 are independent implementation targets (different source files)
- The resolveAgentIdByClientId CEL function is ONLY registered when feature is DISABLED (nil closure = feature enabled = function absent)
- Security rule: token is always withheld on claim absence/mismatch (fail closed, SR-001)
- Commit after each task or logical group; run `just check` and `just verify` before each commit
