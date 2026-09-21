# Implementation Plan: Permission Sets

**Branch**: `019-permission-sets` | **Date**: 2026-03-25 | **Spec**: [spec.md](spec.md)
**Input**: Feature specification from `/specs/019-permission-sets/spec.md`

---

## Summary

Introduce `PermissionSet` as a first-class domain entity — an admin-defined bundle of OAuth2 scopes spanning one or more third-party services. Agents declare an ordered list of permission sets (mandatory/optional) alongside `ServiceRequirements` which act as session gating plus either per-agent scope ceilings or an explicit `require_all_scopes` Permission Set-union mode. The consent screen renders permission sets as a card-based selection UI above service connections; within each PS card, only services intersecting with the agent's `service_requirements` are shown, with per-service toggles for optional SR services. Service connections are dynamic — mandatory SR services always visible, optional SR services only when included via a selected PS. The Approve button is disabled until all displayed services have active sessions. For `require_all_scopes`, the consent response discloses the assigned-PS scope union for that requirement service while retaining scope redaction in permission-set cards.

---

## Technical Context

**Language/Version**: Go 1.25.6 (backend) + React 19 + TypeScript (frontend)
**Primary Dependencies**: chi v5 (HTTP), sqlx + pgx v5 (Postgres), Ginkgo/Gomega (E2E), Vitest (frontend unit), Playwright (frontend E2E)
**Storage**: PostgreSQL (primary) + in-memory (dev/test)
**Testing**: Ginkgo/Gomega (E2E), testify (unit/integration), Playwright (frontend E2E)
**Target Platform**: Linux server, dual-port HTTP (:8000 end-user / :14000 admin)
**Project Type**: Web (Go backend + React SPA)
**Performance Goals**: ≤5ms cache-hit overhead on token exchange path (NFR-002); cache TTL ≈60s (NFR-001)
**Constraints**: Fail closed on scope validation (FR-018); no scope expansion at token exchange (FR-017)
**Scale/Scope**: Full-stack feature touching domain, storage, HTTP handlers, React consent UI

---

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

**Design Preconditions (BLOCKING)**:

- [x] **Domain Model**: `PermissionSet` entity, `ServiceScope` value object, `AgentPermissionSetEntry` value object, `UserGrant` modification — all identified and documented in `data-model.md`
- [x] **Domain Concepts**: `PermissionSet`, `ServiceScope`, `AgentPermissionSetEntry`, `granted_permission_sets` will be added to ARCHITECTURE.md Glossary
- [x] **Entity IDs**: New `PermissionSetID` (`type PermissionSetID uuid.UUID`) to be added to `gen_ids.go` and documented in `internal/domain/id/AGENTS.md` (ADR 013)
- [x] **Configuration Design**: No new config parameters — cache TTL is a code constant (≈60s). No config changes required.
- [x] **Config Examples**: N/A — no new config parameters
- [x] **Helm Chart**: No config parameters added/changed — Helm chart update not required
- [x] **API Design First**: Admin + enduser API contracts documented in `contracts/admin-permission-sets.yaml` and `contracts/enduser-permission-sets.yaml` before implementation
- [x] **API Documentation**: Will be merged into `/api/admin/openapi.yaml` and `/api/enduser/openapi.yaml`
- [x] **API Changes**: API-005 — changes documented in contracts; requires stakeholder confirmation before implementation begins
- [x] **Database Design**: Three atomic migrations (008, 009, 010) in `/migrations/` using go-migrate naming
- [x] **E2E Acceptance Tests**: 24 E2E tests (6+7+5+5+1 edge cases) + 8 Playwright frontend tests written before implementation
- [x] **available_services in consent-info**: `AgentConsentInfoResponse` includes `available_services: [AvailableServiceInfo]` (all `ThirdpartyOAuth2Service` records); `ConsentService` injects `ThirdpartyOAuth2ServiceRepository` alongside `PermissionSetRepository` and `UserSessionRepository`; FR-007 and API endpoint description updated in spec.md
- [x] **E2E Test Mapping**: Each acceptance scenario from spec.md maps to one `It()` block in `tests/e2e/permission_sets_test.go`
- [x] **E2E Red Phase**: Detailed assertions against real system output; no placeholder always-fail assertions; no `XIt`/`PIt`/`Skip()`
- [x] **Frontend Playwright E2E**: Playwright tests in `tests/e2e/frontend/permission_sets_frontend_test.go`
- [x] **Frontend Screenshots**: Screenshots in `tests/e2e/screenshots/consent_permission_sets_*.png`

**Implementation Considerations**:

- [x] **Security-First**: Admin endpoints on `:14000` only (SR-001); no raw scopes in token response (SR-003); audit logs on CRUD (SR-004); fail-closed on insufficient token scope (FR-018)
- [x] **Architecture Docs**: ARCHITECTURE.md will be updated with new glossary terms and permission set domain description
- [x] **ADRs**: No new ADR required — decision follows existing patterns (ADR 004 storage, ADR 005 DDD, ADR 013 typed IDs). No architectural departure.
- [x] **Library-First Security**: No custom crypto; existing encryption stack unchanged
- [x] **Zalando Guidelines**: REST endpoints follow existing admin/enduser patterns; `PUT` for full replacement; `409` for conflicts; `422` for validation
- [x] **End-User Docs**: OpenAPI specs will be updated; rendered via existing docs pipeline
- [x] **Migration Testing**: All three migrations tested (apply → rollback → apply) in PostgreSQL integration tests
- [x] **Hexagonal Architecture**: `PermissionSetRepository` defined as a port interface; memory + postgres adapters implement it; domain service depends on port, not adapter
- [x] **Persistence Patterns**: Follows specs/004-persistence-layer/quickstart.md (ISP repositories, sqlx for Postgres, deep-copy in memory adapter)

*All blocking preconditions satisfied. Implementation may proceed after API-005 stakeholder confirmation.*

---

## Project Structure

### Documentation (this feature)

```text
specs/019-permission-sets/
├── plan.md               # This file
├── research.md           # Phase 0 output
├── data-model.md         # Phase 1 output
├── quickstart.md         # Phase 1 output
├── contracts/
│   ├── admin-permission-sets.yaml    # Admin API contract
│   └── enduser-permission-sets.yaml  # End-user API changes
└── tasks.md              # Phase 2 output (/speckit.tasks command)
```

### Source Code

```text
internal/
├── domain/
│   ├── id/
│   │   └── gen_ids.go                        # Add PermissionSetID
│   ├── storage/
│   │   ├── permission_set.go                 # NEW: PermissionSet, ServiceScope
│   │   ├── agent.go                          # EXTEND: AgentPermissionSetEntry, PermissionSets field
│   │   └── user_grant.go                     # MODIFY: replace DelegatedOAuth2Tokens with GrantedPermissionSets (structured: {ps_id: [service_ids]})
│   ├── permissionset/
│   │   └── service.go                        # NEW: PermissionSetService — CRUD, TTL cache, ValidateIDs, deletion protection
│   ├── consent/
│   │   └── service.go                        # EXTEND: inject *permissionset.Service + *thirdparty.ThirdpartyOAuth2ProviderService
│   └── tokenexchange/
│       └── service.go                        # EXTEND: inject *permissionset.Service for scope resolution + SR scope ceiling intersection + response field
├── ports/
│   └── storage.go                            # ADD: PermissionSetRepository interface
├── adapters/
│   ├── storage/
│   │   ├── memory/
│   │   │   ├── permission_set_repository.go  # NEW
│   │   │   └── factory.go                    # EXTEND: include PermissionSetRepository
│   │   ├── postgres/
│   │   │   ├── permission_set_repository.go  # NEW
│   │   │   └── factory.go                    # EXTEND: include PermissionSetRepository
│   │   └── factory.go                        # Return PermissionSetRepository from Adapter
│   └── http/
│       └── handlers/
│           └── admin/
│               └── permission_sets_handler.go  # NEW
└── app/
    ├── builder.go                              # EXTEND: wire PermissionSetsHandler
    └── handlers.go                             # EXTEND: AdminHandlers.PermissionSets

migrations/
├── 008_add_permission_sets.up.sql
├── 008_add_permission_sets.down.sql
├── 009_add_agent_permission_sets.up.sql
├── 009_add_agent_permission_sets.down.sql
├── 010_migrate_user_grants_to_permission_sets.up.sql
└── 010_migrate_user_grants_to_permission_sets.down.sql

api/
├── admin/openapi.yaml         # ADD: PermissionSets section; EXTEND: Agent schema
└── enduser/openapi.yaml       # EXTEND: consent-info, grants, token response

web/src/
└── components/
    └── consent/
        ├── PermissionSetCard.tsx      # NEW
        ├── PermissionSetsList.tsx     # NEW
        └── ConsentScreen.tsx          # RESTRUCTURE: PSets section before service connections

tests/
├── e2e/
│   ├── permission_sets_test.go        # NEW: 24 Ginkgo It() blocks
│   └── frontend/
│       └── permission_sets_frontend_test.go  # NEW: Playwright tests
└── e2e/screenshots/
    └── consent_permission_sets_*.png  # NEW: captured during frontend E2E
```

**Structure Decision**: Monorepo layout — Go backend (`internal/`) + React frontend (`web/`) in single repo. New files follow existing package conventions exactly.

---

## Testing Strategy

### End-to-End (E2E) Acceptance Tests

**Test Location**: `tests/e2e/permission_sets_test.go`
**Framework**: Ginkgo/Gomega BDD

**Scenario Mapping**:

| Spec Scenario | Test Description |
|---------------|-----------------|
| US1.S1 | It("creates a permission set and returns its generated ID") |
| US1.S2 | It("returns name, description, and service_scope entries for GET by ID") |
| US1.S3 | It("reflects updated values after PUT with modified scopes") |
| US1.S4 | It("removes the permission set and returns 404 on subsequent GET") |
| US1.S5 | It("rejects DELETE with 409 when permission set is referenced by an agent") |
| US1.S6 | It("filters permission set list by service_id") |
| US2.S1 | It("renders mandatory PS locked, optional PS togglable, PS cards filter to SR-intersecting services with per-service toggles") |
| US2.S2 | It("displays only SR-intersecting services in PS card without raw OAuth2 scope strings") |
| US2.S3 | It("shows active session indicator and no connect button for service with active session") |
| US2.S4 | It("shows connect button with Required/Optional badge for service without active session") |
| US2.S5 | It("dynamically updates service connections when user toggles optional PS on/off") |
| US2.S6 | It("renders single mandatory card with Approve disabled until all services connected") |
| US2.S7 | It("enables Approve button when all displayed services have active sessions") |
| US3.S1 | It("persists agent permission_sets list in declaration order") |
| US3.S2 | It("rejects agent registration with non-existent permission_set_id with 422") |
| US3.S3 | It("returns resolved permission set objects with requirement_type in consent-info") |
| US3.S4 | It("returns service_requirements and permission_sets independently in agent response") |
| US3.S5 | It("rejects agent creation when SR service_id is not covered by any PS with 422 — FR-019") |
| US4.S1 | It("stores granted_permission_sets with ps_id and included_service_ids for mandatory + toggled optional") |
| US4.S2 | It("includes granted_permission_sets in token exchange response") |
| US4.S3 | It("upserts UserGrant with new granted_permission_sets on re-consent") |
| US4.S4 | It("computes scope union for two permission sets both covering Service A, intersected with an explicit SR ceiling") |
| US3.S6 | It("discloses the assigned permission-set scope union for a require-all-scopes requirement") |
| Edge | It("fails token exchange with descriptive error when UserSession scopes are insufficient") |

**Red Phase Requirements**: Realistic assertions against actual HTTP responses — no placeholder failures. All 24 tests compile and fail before implementation.

**Bootstrap Strategy**: Production bootstrap via `tests/e2e/bootstrap/`. Fresh server + storage per test (BeforeEach/AfterEach).

### Frontend Playwright E2E Tests

**Test Location**: `tests/e2e/frontend/permission_sets_frontend_test.go`
**Framework**: Playwright via existing harness

| UI Scenario | Screenshot |
|-------------|-----------|
| Consent screen: permission sets section above service connections | `consent_permission_sets_above_services.png` |
| Mandatory permission set card (locked, pre-selected, no checkbox) | `consent_mandatory_ps_locked.png` |
| Optional permission set card (togglable, initially unchecked) | `consent_optional_ps_togglable.png` |
| PS card filters to SR-intersecting services with per-service toggles | `consent_ps_card_per_service_toggles.png` |
| Dynamic service connections update when optional PS toggled on/off | `consent_dynamic_service_connections.png` |
| Service already connected (satisfied indicator, no connect button) | `consent_service_already_connected.png` |
| Approve button disabled until all displayed services connected | `consent_approve_blocked_unconnected.png` |
| Approve button enabled when all services connected | `consent_approve_enabled_all_connected.png` |

### Unit & Integration Tests

**Unit Tests**:
- `internal/domain/storage/permission_set_test.go` — entity validation
- `internal/domain/consent/service_test.go` — ConsentInfo resolution: PS card filtering to SR-intersecting services, dynamic service connections computation, submission gating validation, and require-all-scopes disclosure union (hand-rolled mocks)
- `internal/domain/tokenexchange/service_test.go` — cache behavior, scope union, explicit SR scope ceiling intersection, require-all-scopes passthrough, insufficient scope error
- `internal/adapters/http/handlers/admin/permission_sets_handler_test.go` — HTTP handler (testify/mock)
- `internal/adapters/http/handlers/admin/agents_handler_test.go` — FR-019 coverage invariant validation (testify/mock)
- `internal/adapters/http/handlers/consent/agent_detail_handler_test.go` — serialized all-scope consent disclosure

**Integration Tests**:
- `internal/adapters/storage/postgres/permission_set_repository_test.go` — real Postgres via testcontainers
- Migration apply/rollback tests in same file

**Test Coverage Goals**:
- Domain + handler unit tests: critical paths only
- Storage integration: all `PermissionSetRepository` methods (DB-005)
- E2E: 100% of acceptance scenarios (mandatory per Principle XIII)
- Frontend E2E: all 8 UI acceptance scenarios

---

## Complexity Tracking

No constitution violations. All choices follow established ADRs and patterns.

---

## Phase 0: Research — COMPLETE

See [research.md](research.md) for all decisions with rationale:

1. JSONB for `agent.permission_sets` (matches `service_requirements` pattern)
2. Native JSONB for `user_grants.granted_permission_sets` (structured `{ps_id: [service_ids]}` — positive inclusion model per Q3 clarification)
3. `sync.RWMutex` + map with per-entry TTL in `PermissionSetService` (not `sync.Map` — no per-key expiry support); single cache instance shared by `ConsentService` and `TokenExchangeService` consumers via the service layer
4. Application-layer deletion protection for agent→PS direction; DB `RESTRICT` FK for service→PS
5. Relational schema for `permission_set_service_scopes` (enables FK + filtering)
6. JSONB `@>` containment query for agent deletion protection check
7. React: `PermissionSetCard` + `PermissionSetsList` components
8. Three migrations: 008, 009, 010 (split for independent rollback)
9. New `PermissionSetID` typed ID via `gen_ids.go`

---

## Phase 1: Design & Contracts — COMPLETE

### Artifacts Generated

| Artifact | Location | Status |
|----------|----------|--------|
| Data model | `specs/019-permission-sets/data-model.md` | ✅ |
| Admin API contract | `specs/019-permission-sets/contracts/admin-permission-sets.yaml` | ✅ |
| End-user API contract | `specs/019-permission-sets/contracts/enduser-permission-sets.yaml` | ✅ |
| Developer quickstart | `specs/019-permission-sets/quickstart.md` | ✅ |

### Constitution Re-Check (Post-Design)

All pre-conditions still met. No deviations introduced in design phase.

- Security: admin-only endpoints, no raw scopes in response, fail-closed scope validation ✅
- Hexagonal architecture: `PermissionSetRepository` is a port; `PermissionSetService` (domain service) owns all business logic; handlers depend on the domain service, never on repos directly; domain never imports adapters ✅
- TDD: E2E tests written first (red phase) before any implementation ✅
- ISP: `PermissionSetRepository` is a focused, single-concern interface ✅

---

## Implementation Order (Dependencies)

```
Step 1:  PermissionSetID (gen_ids.go) — unblocks all downstream
Step 2:  Domain entities (permission_set.go, agent.go, user_grant.go) — unblocks ports + adapters
Step 3:  PermissionSetRepository port (ports/storage.go) — unblocks adapters
Step 4:  Migrations 008, 009, 010 — unblocks postgres adapter
Step 5:  Memory adapter — enables unit + E2E tests (no DB required)
Step 6:  Postgres adapter — enables integration tests
Step 7:  PermissionSetService (domain/permissionset/service.go) — CRUD, TTL cache, ValidateIDs,
         deletion protection; unblocks every handler and domain service that resolves PSets
Step 8:  ThirdpartyOAuth2ProviderService — inject PermissionSetRepository for service deletion
         check (DB-006); no handler changes required
Step 9:  Admin HTTP handler + routing — US1 (depends on PermissionSetService)
Step 10: Consent service extension — US2, US3 (depends on PermissionSetService +
         ThirdpartyOAuth2ProviderService; no direct repo deps)
Step 11: Token exchange extension — US4 (depends on PermissionSetService; no own cache;
         intersects PS scopes with SR scope ceiling per FR-013)
Step 12: React components + ConsentScreen restructure — US2 frontend
Step 13: E2E tests (red phase) — written alongside Steps 9–12
Step 14: API spec updates (admin/enduser openapi.yaml)
Step 15: ARCHITECTURE.md glossary update
Step 16: Playwright frontend E2E tests + screenshots
```
