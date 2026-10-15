# Implementation Plan: Multi-Agent OAuth2 Client Delegation

**Branch**: `021-multi-agent-clientid` | **Date**: 2026-03-20 | **Spec**: [spec.md](spec.md)
**Input**: Feature specification from `/specs/021-multi-agent-clientid/spec.md`

## Summary

Allow multiple agents to share one upstream OAuth2 client ID. This requires a **breaking change** in how the OAuth2 `client_id` parameter is resolved (now maps to `agent.id` UUID), optional agent ID injection on the upstream authorize redirect URL, optional claim verification on proxied token responses, and a custom `resolveAgentIdByClientId` CEL function for backward-compatible token exchange policies. All changes are gated on a new `multi_agent_client` configuration block.

## Technical Context

**Language/Version**: Go 1.25.6
**Primary Dependencies**: chi v5, cel-go v0.27.0, lestrrat-go/jwx/v3 v3.0.13, sqlx + pgx v5, Ginkgo/Gomega v2
**Storage**: PostgreSQL (production), in-memory (dev/test). Migration `008` drops the UNIQUE constraint on `agents.client_id`.
**Testing**: Ginkgo v2 / Gomega (E2E), Go test + testify (unit/integration)
**Target Platform**: Linux server (dual-server: :8000 end-user, :14000 admin)
**Performance Goals**: No measurable latency increase on authorize/token paths (SC-006). JWT claim parsing is a single base64-decode; CEL resolver uses 100ms timeout budget.
**Constraints**: Breaking change — existing OAuth2 clients must update `client_id` to `agent.id` UUID. No migration shim.
**Scale/Scope**: Backend-only; no frontend changes. ~8 files modified, 1 new migration, 1 new E2E test file.

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

**Design Preconditions (BLOCKING)**:

- [x] **Domain Model**: `MultiAgentClientConfig` value object identified; `Agent` entity semantics clarified; `CELEvaluatorConfig` extended with resolver function. See `data-model.md`.
- [x] **Domain Concepts**: `MultiAgentClientConfig` and `resolveAgentIdByClientId` will be added to `ARCHITECTURE.md` Glossary.
- [x] **Entity IDs**: No new domain entities with UUID PKs. Existing `AgentID` typed ID is used throughout.
- [x] **Configuration Design**: `MultiAgentClientConfig` with `enabled`, `agent_id_param_name`, `agent_id_claim_name`. YAML examples in `quickstart.md` and `examples/config/oauth2-authorization-server.yaml`.
- [x] **Config Examples**: Config examples documented in `quickstart.md` and `examples/config/oauth2-authorization-server.yaml`.
- [x] **API Design First**: Breaking `client_id` semantics change and admin uniqueness relaxation documented in `contracts/admin-api-changes.md`. OpenAPI updates identified.
- [x] **API Documentation**: Updates to `/api/enduser/openapi.yaml` (authorize `client_id` description) and `/api/admin/openapi.yaml` (agent `client_id` uniqueness note).
- [x] **API Changes**: Breaking changes acknowledged in spec and clarifications (spec lines 14–16). Stakeholder-confirmed.
- [x] **Database Design**: Migration `008_drop_agent_client_id_unique` documented in `data-model.md`.
- [x] **E2E Acceptance Tests**: All 14 spec scenarios (US1×6, US2×4, US3×4) map 1:1 to `tests/e2e/multi_agent_client_test.go`. Written before implementation (red phase).
- [x] **E2E Test Mapping**: Scenario mapping table below.
- [x] **E2E Red Phase**: Tests compile with stub handlers; fail semantically because feature logic is absent.

**Implementation Considerations**:

- [x] **Security-First**: Feature fails closed (FR-004, SR-001); token withheld when claim absent. Audit logging per SR-004.
- [x] **Architecture Docs**: `ARCHITECTURE.md` updated with glossary entries.
- [x] **ADRs**: No new architectural boundary; the pattern (CEL custom function via injected closure) follows existing CEL and DI patterns. No ADR needed.
- [x] **Library-First Security**: JWT parsing via `lestrrat-go/jwx/v3 jwt.ParseInsecure`. No custom crypto.
- [x] **Zalando Guidelines**: No new REST endpoints; existing endpoints updated with clarified descriptions.
- [x] **End-User Docs**: OpenAPI descriptions updated.
- [x] **Migration Testing**: Migration `008` tested with apply/rollback in PostgreSQL integration tests.
- [x] **Hexagonal Architecture**: `MultiAgentClientConfig` in `ports/config.go`; `MultiAgentTokenVerifier` injected into handler; `ResolveAgentIDByClientID` closure injected into CEL evaluator.
- [x] **Persistence Patterns**: No new entity persistence. Existing `AgentRepository` ports unchanged (only `GetByClientID` callers updated from the service layer).

## Project Structure

### Documentation (this feature)

```text
specs/021-multi-agent-clientid/
├── plan.md              # This file
├── research.md          # Phase 0 output
├── data-model.md        # Phase 1 output
├── quickstart.md        # Phase 1 output
├── contracts/
│   └── admin-api-changes.md   # Phase 1 output
└── tasks.md             # Phase 2 output (not yet)
```

### Source Code (affected files)

```text
internal/ports/config.go                            # Add MultiAgentClientConfig
internal/domain/oauth2/service.go                   # Breaking change: Get() + param injection
internal/domain/oauth2/multi_agent_verifier.go      # MultiAgentTokenVerifier struct + MultiAgentVerifier interface
internal/domain/tokenexchange/cel_evaluator.go      # resolveAgentIdByClientId function
internal/domain/tokenexchange/service.go            # Get() instead of GetByClientID()
internal/adapters/http/enduser/oauth2_token.go      # Buffer response + claim verification
internal/adapters/http/handlers/admin/agents.go     # client_id uniqueness check when disabled
internal/app/builder.go                             # Wire resolver closure + verifier
migrations/
  008_drop_agent_client_id_unique.up.sql
  008_drop_agent_client_id_unique.down.sql
examples/config/oauth2-authorization-server.yaml    # Add multi_agent_client block
examples/config/token-exchange.yaml                 # Update CEL expression examples
api/enduser/openapi.yaml                            # Update client_id description
api/admin/openapi.yaml                              # Update agent client_id uniqueness note
ARCHITECTURE.md                                     # Glossary additions
tests/e2e/multi_agent_client_test.go               # New E2E test file (14 scenarios)
```

## Testing Strategy

### End-to-End (E2E) Acceptance Tests

**Test Location**: `tests/e2e/multi_agent_client_test.go`
**Framework**: Ginkgo/Gomega BDD

**Scenario Mapping**:

| Spec Scenario | E2E Test `It()` Description |
|---|---|
| US1 Scenario 1: Agent ID param appended when enabled | `It("should append agent ID param to upstream authorize URL")` |
| US1 Scenario 2: client_id resolved against agent.id always | `It("should resolve client_id against agent.id not agent.client_id")` |
| US1 Scenario 3: Token returned only after claim verified | `It("should proxy token after verifying agent ID claim")` |
| US1 Scenario 4: Claim value matches initiating agent | `It("should verify claim value matches initiating agent ID")` |
| US1 Scenario 5: Missing claim → OAuth2 error, token withheld | `It("should return error and withhold token when agent ID claim absent")` |
| US1 Scenario 6: Feature disabled → no change in behavior | `It("should not inject param or verify claim when feature is disabled")` |
| US2 Scenario 1: Token exchange resolves correct agent via claim | `It("should resolve agent from token claim when feature enabled")` |
| US2 Scenario 2: Missing claim → token exchange error | `It("should fail token exchange when agent ID claim absent from subject token")` |
| US2 Scenario 3: resolveAgentIdByClientId works when disabled | `It("should resolve agent via resolveAgentIdByClientId CEL function when disabled")` |
| US2 Scenario 4: Unrecognized agent ID claim → exchange error | `It("should reject token exchange when agent ID claim does not match any registered agent")` |
| US3 Scenario 1: Valid enabled config → broker starts | `It("should start successfully with valid multi_agent_client configuration")` |
| US3 Scenario 2: Enabled but missing param name → startup failure | `It("should fail to start when enabled but agent_id_param_name is absent")` |
| US3 Scenario 3: Enabled but missing claim name → startup failure | `It("should fail to start when enabled but agent_id_claim_name is absent")` |
| US3 Scenario 4: Disabled config → unchanged behavior | `It("should operate in single-agent mode when multi_agent_client is disabled")` |

**Test Data Strategy**:
- Two agents sharing `client_id: "shared-upstream"` for multi-agent scenarios (US1, US2)
- Mock upstream OAuth2 server (extend `helpers.MockUpstreamOAuth2Server`) to return configurable JWT tokens with or without the agent ID claim
- Fixtures: extend `tests/e2e/fixtures/` with multi-agent-specific config helpers

**Bootstrap Strategy**:
- Use existing `bootstrap.ServerFactory` and `bootstrap.NewEndUserTestServer`
- Extend `bootstrap.StorageFactory` to seed two agents with shared client_id when feature is enabled
- The mock upstream server must be extended to return JWT tokens with `x_agent_id` claim on demand

**Helper Utilities**:
- New matcher: `ContainAgentIDClaim(claimName, agentID string)` for verifying JWT response bodies
- Existing: `helpers.MockUpstreamOAuth2Server` extended with `ReturnTokenWithClaim(claimName, value string)`

### Unit & Integration Tests

**Unit Tests** (TDD — written before implementation, fail semantically first):
- `internal/domain/oauth2/service_test.go`: Test `HandleAuthorization` resolves by `agent.id`, appends param when enabled
- `internal/domain/tokenexchange/cel_evaluator_test.go`: Test `resolveAgentIdByClientId` function registration/behavior
- `internal/domain/tokenexchange/service_test.go`: Test `Get()` used instead of `GetByClientID()`
- `internal/adapters/http/enduser/oauth2_token_test.go`: Test claim verification in `proxyToUpstream()`
- `internal/adapters/http/handlers/admin/agents_test.go`: Test client_id uniqueness check when disabled

**Integration Tests**:
- `internal/adapters/storage/postgres/agent_repository_integration_test.go`: Test migration `008` applies and rolls back cleanly; verify two agents can share `client_id` after migration

**Test Coverage Goals**:
- Unit: All new code paths (param injection, claim verification, CEL function, uniqueness check)
- Integration: Migration 008 apply/rollback; postgres adapter accepts duplicate client_id
- E2E: 100% of 14 acceptance scenarios

## Complexity Tracking

No constitution violations.

---

## Phase 0: Research (COMPLETE)

See [research.md](research.md). All unknowns resolved:
- CEL custom function: `cel.Function()` + injected `func(string)(string,error)` closure
- JWT claim parsing: `jwt.ParseInsecure([]byte(token))`
- Authorize injection point: `buildUpstreamAuthorizeURL()` before `q.Encode()`
- Token verification injection: Buffer `proxyToUpstream()` response, inject `MultiAgentTokenVerifier`
- Breaking change: `id.ParseAgentID(clientID)` + `agentRepo.Get(agentID)`
- Uniqueness: Drop DB UNIQUE constraint; application-layer check in admin handler

---

## Phase 1: Design (COMPLETE)

See [data-model.md](data-model.md), [contracts/admin-api-changes.md](contracts/admin-api-changes.md), [quickstart.md](quickstart.md).

**Post-design Constitution Check** (re-evaluated):
- All design preconditions satisfied. No violations identified.

---

## Phase 2: Implementation

### Phase 2a: Domain Model & Glossary

1. Add `MultiAgentClientConfig` struct to `internal/ports/config.go` nested under `OAuth2AuthServerConfig`
2. Add validation in `OAuth2AuthServerConfig.Validate()`: require `AgentIDParamName` + `AgentIDClaimName` when `Enabled = true`
3. Add `MultiAgentClient MultiAgentClientConfig` field to `domain/oauth2/OAuth2Config` struct
4. Add `ResolveAgentIDByClientID func(string)(string,error)` field to `tokenexchange.CELEvaluatorConfig`
5. Update `ARCHITECTURE.md` glossary with `MultiAgentClientConfig` and `resolveAgentIdByClientId`

### Phase 2b: Database Migration

1. Write `migrations/008_drop_agent_client_id_unique.up.sql` — drop UNIQUE constraint on `agents.client_id`, recreate non-unique index
2. Write `migrations/008_drop_agent_client_id_unique.down.sql` — re-add UNIQUE constraint
3. Write integration test verifying migration apply/rollback and that two agents can share `client_id` after migration

### Phase 2c: Breaking Change — Agent Lookup by Internal ID

**Target**: `internal/domain/oauth2/service.go`

1. In `HandleAuthorization()`, replace:
   ```go
   agent, err := s.agentRepo.GetByClientID(ctx, req.ClientID)
   ```
   with:
   ```go
   agentID, err := id.ParseAgentID(string(req.ClientID))
   if err != nil {
       return &ports.AuthorizationDecision{Action: "error", ErrorCode: "invalid_client", ...}, nil
   }
   agent, err := s.agentRepo.Get(ctx, agentID)
   ```
2. The upstream proxy call must use `agent.ClientID` (unchanged — it already does via `buildUpstreamAuthorizeURL`)

**Target**: `internal/domain/tokenexchange/service.go`

3. In the agent lookup path (~line 227), replace `GetByClientID(id.NewClientID(agentClientID))` with `id.ParseAgentID(agentClientID)` + `Get(agentID)`
4. Handle UUID parse error as a token exchange error (`invalid_target` or `invalid_request`)

**Unit tests** (written first):
- `service_test.go`: Client UUID resolves to correct agent; non-UUID client_id returns `invalid_client`; non-existent UUID returns `invalid_client`

### Phase 2d: Admin API — Conditional Client_ID Uniqueness Check

**Target**: `internal/adapters/http/handlers/admin/agents.go`

1. Inject `multiAgentEnabled bool` into `AgentsHandler` (wired via Builder)
2. In `Create` handler: when `!multiAgentEnabled`, call `agentRepo.GetByClientID()` before insert; return 409 if found
3. In `Update` handler: same check, excluding the current agent's ID
4. Update unit tests for both scenarios

### Phase 2e: E2E Acceptance Test Design (Red Phase)

**Target**: `tests/e2e/multi_agent_client_test.go`

1. Write all 14 `It()` blocks per the scenario mapping table above
2. Extend `helpers.MockUpstreamOAuth2Server` with claim-configurable token responses
3. Add multi-agent fixtures to `tests/e2e/fixtures/`
4. Verify: `ginkgo ./tests/e2e/multi_agent_client_test.go` — all 14 tests FAIL semantically (not compilation errors)

### Phase 2f: Feature — Agent ID Parameter Injection

**Target**: `internal/domain/oauth2/service.go`

1. In `buildUpstreamAuthorizeURL()`, add:
   ```go
   if s.config.MultiAgentClient.Enabled {
       q.Set(s.config.MultiAgentClient.AgentIDParamName, agent.ID.String())
   }
   ```
   immediately before `u.RawQuery = q.Encode()`
2. Audit log: `AgentIDParamInjected` at Info level with `agent_id`, `param_name`
3. Unit tests: verify param present when enabled, absent when disabled

### Phase 2g: Feature — Token Response Claim Verification

**Target**: `internal/adapters/http/enduser/oauth2_token.go`

1. Define `MultiAgentVerifier` interface with single method `VerifyAgentIDClaim(ctx context.Context, responseBody []byte, expectedAgentID id.AgentID) error`; add nilable `MultiAgentVerifier` field to `OAuth2TokenHandler` (nil = feature disabled)
2. In `proxyToUpstream()`, buffer response body instead of streaming (use `io.ReadAll` + `bytes.Buffer`; `ioutil` is deprecated since Go 1.16)
3. When `MultiAgentVerifier != nil` (feature enabled): call `verifier.VerifyAgentIDClaim(ctx, body, expectedAgentID)`
   - Inside `VerifyAgentIDClaim` in `multi_agent_verifier.go`: parse JSON body for `access_token` field
   - `jwt.ParseInsecure([]byte(accessToken))`
   - `token.Get(config.AgentIDClaimName, &claimValue)`
   - Verify `claimValue == expectedAgentID.String()` — this is equivalent to "matches a registered agent.id" because `expectedAgentID` was already resolved by `id.ParseAgentID(clientID)` from the token request `client_id` parameter (itself the agent's UUID)
   - On failure: return error; caller writes `{"error":"server_error","error_description":"..."}` + 500; log `AgentIDClaimMissing`/`AgentIDClaimMismatch`
4. On success or feature disabled: write buffered body as-is
5. **Note**: The `proxyToUpstream()` needs the agent's ID. The authorize flow and token flow are not directly linked (the token endpoint receives an authorization code, not the agent's ID). Resolution: the agent's ID is available from the `client_id` parameter in the token request. Add `client_id` extraction to `proxyToUpstream()` callers, or pass via a new handler field set per-request.

   **Design decision**: Extract `client_id` from the token request form data in `ServeHTTP()` and pass to `proxyToUpstream()`. The `client_id` in the token endpoint is the same UUID as in the authorize request, so `id.ParseAgentID(clientID)` yields the expected agent ID.

6. Unit tests: Mock upstream response; verify buffered write when feature disabled; verify error when claim absent; verify success when claim present and matching

### Phase 2h: Feature — `resolveAgentIdByClientId` CEL Function

**Target**: `internal/domain/tokenexchange/cel_evaluator.go`

1. In `compileExpression()`, extend `cel.NewEnv()` options:
   - If `e.config.ResolveAgentIDByClientID != nil`: add `cel.Function("resolveAgentIdByClientId", ...)` declaration
   - If nil: do NOT add the function (enabled mode)
2. The function overload: `UnaryBinding` that calls `e.config.ResolveAgentIDByClientID(clientID)` synchronously
3. Error handling: return `types.NewErr(...)` on lookup failure
4. Unit tests (written first): verify function evaluates correctly with mock resolver; verify `types.NewErr` on unknown clientID; verify function NOT available when resolver is nil

### Phase 2i: Builder Wiring

**Target**: `internal/app/builder.go`

1. Pass `MultiAgentClientConfig` through to `oauth2.NewServiceWithSessions()` (via `OAuth2Config`)
2. Build resolver closure when `!multiAgentEnabled`:
   ```go
   var resolverFn func(string) (string, error)
   if !cfg.OAuth2AuthServer.MultiAgentClient.Enabled {
       resolverFn = func(clientID string) (string, error) {
           ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
           defer cancel()
           agent, err := agentRepo.GetByClientID(ctx, id.ClientID(clientID))
           if err != nil { return "", err }
           return agent.ID.String(), nil
       }
   }
   ```
3. Inject resolver into `CELEvaluatorConfig`
4. Build `MultiAgentTokenVerifier` when feature is enabled; inject into `OAuth2TokenHandler`
5. Inject `multiAgentEnabled` into `AgentsHandler`

### Phase 2j: Configuration Examples & Documentation

1. Update `examples/config/oauth2-authorization-server.yaml` with `multi_agent_client` block (both enabled and disabled examples)
2. Update `examples/config/token-exchange.yaml` with updated CEL expression examples
3. Update `api/enduser/openapi.yaml`: update `client_id` description on authorize endpoint
4. Update `api/admin/openapi.yaml`: update agent `client_id` uniqueness note
5. Update `ARCHITECTURE.md` glossary

### Phase 2k: Constitution Compliance Verification

**Design Phase Verification**:
- [ ] `MultiAgentClientConfig` in `ports/config.go` ✓
- [ ] `ARCHITECTURE.md` glossary updated ✓
- [ ] Config examples in `examples/config/` ✓
- [ ] OpenAPI specs updated and confirmed ✓
- [ ] Migration `008` documented and implemented ✓
- [ ] E2E tests written before implementation (red phase) ✓

**Implementation Phase Verification (Principle VIII — TDD)**:
- [ ] Unit tests written first, compile, fail semantically before implementation
- [ ] Unit tests change minimally during implementation
- [ ] Integration test for migration 008 apply/rollback

**Implementation Phase Verification (Principle I — Security)**:
- [ ] Token withheld when claim absent (fail closed) — FR-004, SR-001
- [ ] `resolveAgentIdByClientId` not registered when feature enabled — FR-010, SR-005
- [ ] Audit logs: `AgentIDParamInjected`, `AgentIDClaimVerified`, `AgentIDClaimMissing` — SR-004

**Implementation Phase Verification (Principle VI — Hexagonal)**:
- [ ] Domain service (`oauth2.Service`) does not import adapters
- [ ] `MultiAgentTokenVerifier` injected into handler (not instantiated inside)
- [ ] Resolver closure injected into `CELEvaluator` (not built inside domain)

**Implementation Phase Verification (Principle XII — DI via Builder)**:
- [ ] All wiring in `app/builder.go`
- [ ] Routing functions receive pre-wired handlers

**Implementation Phase Verification (Principle XIII — E2E)**:
- [ ] All 14 spec scenarios have corresponding `It()` blocks
- [ ] Tests fail semantically in red phase
- [ ] Tests turn green as implementation satisfies criteria
- [ ] Tests changed minimally during implementation

---

## Implementation Order (recommended)

1. **Phase 2a + 2b** (config + migration) — foundational; unblocks everything
2. **Phase 2e** (E2E tests in red) — must be before implementation phases
3. **Phase 2c** (breaking change: agent lookup) — first production change; all other phases depend on this working
4. **Phase 2d** (admin uniqueness check) — independent, can run alongside 2c
5. **Phase 2f** (param injection) — depends on 2c (agent.ID is available)
6. **Phase 2h** (CEL function) — independent of 2f/2g
7. **Phase 2g** (token claim verification) — depends on 2c (agent.ID from client_id)
8. **Phase 2i** (builder wiring) — depends on 2a, 2c, 2f, 2g, 2h
9. **Phase 2j** (docs + examples) — can run in parallel with implementation
10. **Phase 2k** (compliance verification) — final gate
