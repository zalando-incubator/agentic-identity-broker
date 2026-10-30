# Implementation Plan: Revoke Agent Consent

**Branch**: `20260320-120100-revoke-agent-consent` | **Date**: 2026-03-20 | **Spec**: [spec.md](spec.md)
**Input**: Feature specification from `/specs/20260320-120100-revoke-agent-consent/spec.md`

## Summary

Add user-initiated grant revocation for AI agents. Users can revoke all permissions for a specific agent from both the agent detail page (P1) and the consent overview page (P2). A new `DELETE /api/consent/agent/{agent-id}/grants` RESTful endpoint is the canonical revocation path. Authorization enforcement at token-exchange time is already satisfied by the existing `VerifyAgentAccess` hard-delete logic — no schema changes or new domain concepts are needed, only the endpoint, UI, and supporting storage/service wiring.

## Technical Context

**Language/Version**: Go 1.25.6 (backend), React 19 + TypeScript + Vite 7 (frontend)
**Primary Dependencies**: chi v5 (router), sqlx + pgx v5 (PostgreSQL), slog (structured logging), Ginkgo/Gomega (E2E tests), vitest (frontend tests), Tailwind CSS v4 (design tokens)
**Storage**: PostgreSQL (production), in-memory (dev/test) — no schema migration required
**Testing**: Go stdlib + testify (unit), testcontainers (integration), Ginkgo/Gomega (E2E), vitest (frontend)
**Target Platform**: Linux server (Go) + browser SPA (React)
**Project Type**: Full-stack — hexagonal Go backend + React frontend
**Performance Goals**: Standard HTTP response times (<200ms p95 for DELETE endpoint); no special caching concerns
**Constraints**: No schema migration, no soft-delete field, no bulk revocation
**Scale/Scope**: One new endpoint, one new handler, two new frontend components, three page updates

## Constitution Check

*GATE: Must pass before implementation begins.*

**Design Preconditions (BLOCKING)**:

- [x] **Domain Model**: `UserGrant` is unchanged. New port method `DeleteByPrincipalAndAgentID` and new service method `RevokeConsentForPrincipal` identified. No new entities.
- [x] **Domain Concepts**: `GrantRevoked` audit event documented in `data-model.md`. Will be added to `ARCHITECTURE.md` glossary.
- [x] **Entity IDs**: No new entity with UUID PK. Existing `GrantID`, `AgentID`, `Principal` used. No new typed ID required.
- [x] **Configuration Design**: No new configuration required. Endpoint is always available when broker is deployed.
- [x] **Config Examples**: N/A — no config changes.
- [x] **API Design First**: `DELETE /api/consent/agent/{agent-id}/grants` designed in `contracts/delete-grant.yaml`. Confirmed by stakeholder via spec clarifications and acceptance scenarios.
- [x] **API Documentation**: OpenAPI changes documented in `contracts/delete-grant.yaml` for merge into `/api/enduser/openapi.yaml`.
- [x] **API Changes**: API design confirmed in spec document (stakeholder sign-off via spec clarification session 2026-03-20). PR review will document this.
- [x] **Database Design**: No schema changes. `DeleteByPrincipalAndAgentID` uses existing `user_grants` table with `DELETE WHERE principal = $1 AND agent_id = $2`.
- [x] **E2E Acceptance Tests**: Will be written before implementation. 8 Ginkgo scenarios covering all API-level acceptance criteria. UI scenarios covered by frontend component tests.
- [x] **E2E Test Mapping**: Each acceptance scenario maps to one `It()` block. See Testing Strategy section.
- [x] **E2E Red Phase**: E2E tests will fail semantically (endpoint returns 404/405) until DELETE route is registered.

**Implementation Considerations**:

- [x] **Security-First**: Principal check before UUID validation (existing pattern). Cross-user delete returns 404 (no grant found for that principal — no existence leakage). `ErrNotFound` from storage is not surfaced raw.
- [x] **Architecture Docs**: `ARCHITECTURE.md` glossary updated with `GrantRevoked`. No structural architecture changes.
- [x] **ADRs**: No new major architectural decisions. Follows all existing ADRs (ADR 20251215-120100 chi, ADR 20251215-120300 storage, ADR 20260223-120100 DI). No new ADR needed.
- [x] **Library-First Security**: No crypto. Existing `slog` for audit logging. No custom security implementations.
- [x] **Zalando Guidelines**: DELETE endpoint returns 204 (no body). 404 for missing resource. Consistent with existing REST patterns.
- [x] **End-User Docs**: API changes will be reflected in `/api/enduser/openapi.yaml` (the docs source).
- [x] **Migration Testing**: No migrations. N/A.
- [x] **Hexagonal Architecture**: New port method → both adapters implement it → service uses port → handler uses service interface. Clean hexagonal flow.
- [x] **Persistence Patterns**: Follows ADR 20251215-120300: ISP repository, sqlx, `ports.ErrNotFound` error wrapping, write timeout.

## Project Structure

### Documentation (this feature)

```text
specs/20260320-120100-revoke-agent-consent/
├── plan.md              # This file
├── research.md          # Phase 0 — decisions and rationale
├── data-model.md        # Phase 1 — entities, port changes, service changes
├── quickstart.md        # Phase 1 — implementation checklist
├── contracts/
│   └── delete-grant.yaml  # Phase 1 — OpenAPI fragment for DELETE endpoint
└── tasks.md             # Phase 2 output (/speckit.tasks — NOT created here)
```

### Source Code (relevant files)

```text
# Backend
api/enduser/openapi.yaml                        # Add DELETE /api/consent/agent/{agent-id}/grants
ARCHITECTURE.md                                  # Add GrantRevoked to glossary

internal/ports/storage.go                        # Add DeleteByPrincipalAndAgentID to interface
internal/domain/consent/service.go               # Add RevokeConsentForPrincipal, ErrGrantNotFound, remove TODO
internal/adapters/http/handlers/consent/
  service_interface.go                           # Add RevokeConsentForPrincipal to ConsentService interface
  revoke_grant_handler.go                        # New handler
internal/adapters/storage/memory/user_grants.go  # Implement DeleteByPrincipalAndAgentID
internal/adapters/storage/postgres/user_grants.go # Implement DeleteByPrincipalAndAgentID
internal/app/handlers.go                         # Add RevokeGrant *consent.RevokeGrantHandler
internal/app/builder.go                          # Wire RevokeGrantHandler
internal/adapters/http/routing/enduser.go        # Register DELETE route

# Frontend
web/src/components/consent/RevokeGrantButton.tsx  # New component (destructive action trigger)
web/src/components/consent/RevokeGrantDialog.tsx  # New shared confirmation modal
web/src/pages/AgentGrantDetailPage.tsx            # Add P1 revoke section
web/src/components/consent/DelegationCard.tsx     # Add P2 onRevoke prop + button
web/src/pages/ConsentOverviewPage.tsx             # Add P2 revoke state management

# E2E Tests
tests/e2e/revoke_grant_test.go                    # 8 Ginkgo scenarios
```

## Testing Strategy

### End-to-End (E2E) Acceptance Tests

**Test Location**: `tests/e2e/revoke_grant_test.go`

**Framework**: Ginkgo/Gomega following patterns in `tests/e2e/README.md`

**Scope decision**: Ginkgo E2E tests cover HTTP API behavior and full system integration. UI-specific assertions (button visibility, dialog text, focus trapping, keyboard navigation) are covered by frontend component tests (`*.test.tsx` with vitest) because they are more efficient for DOM assertions.

**Scenario Mapping**:

| Spec Scenario | E2E Test Description | Type |
|---|---|---|
| US1-S3: grant deleted on confirm | `It("returns 204 and removes grant on DELETE")` | E2E |
| US1-S4: agent no longer has active grant | `It("GET grants returns null after DELETE")` | E2E |
| US1-S5 / US2-S4: cancel = no change | `It("grant still exists when DELETE not called")` | E2E (verify via GET) |
| US2-S3: overview revoke works | Same DELETE endpoint test as US1-S3 | E2E (shared) |
| US3-S1: token exchange denied after revoke | `It("token exchange returns error after grant revoked")` | E2E |
| US3-S2: immediate re-attempt denied | `It("immediate token exchange re-attempt is denied")` | E2E |
| Edge: non-existent grant | `It("returns 404 when no grant exists")` | E2E |
| Edge: wrong principal | `It("returns 404 when grant belongs to different principal")` | E2E (security) |
| Extra: no auth | `It("returns 401 when no principal header")` | E2E (security) |
| Extra: invalid UUID | `It("returns 400 for invalid agent-id format")` | E2E (validation) |

UI scenarios (from spec) covered by frontend tests:
- US1-S1: "Revoke All Access" button visible when grant exists → `AgentGrantDetailPage.test.tsx`
- US1-S2: Dialog shows agent name, consequence, services note → `RevokeGrantDialog.test.tsx`
- US2-S1: "Revoke" action on each overview card → `DelegationCard.test.tsx`
- US2-S2: Dialog content from overview → `RevokeGrantDialog.test.tsx`
- Edge: button hidden when no grant → `AgentGrantDetailPage.test.tsx`

**Test Data Strategy**:
- Use `tests/e2e/fixtures/grants.go` for creating test grants (`NewUserGrant(principal, agentID)`)
- Use `tests/e2e/fixtures/principals.go` for test principals
- Use `tests/e2e/fixtures/agents.go` for test agents
- No new fixtures required

**Test Execution Flow**:
1. Write E2E tests first (red phase)
2. Verify: `ginkgo -v ./tests/e2e/ -run "Revoke Agent Grant"` — all tests FAIL (405 Method Not Allowed until route added, then 404 until service implemented)
3. Implement backend incrementally
4. E2E tests turn GREEN when all scenarios pass

**Bootstrap Strategy**:
- Uses `bootstrap.NewEndUserTestServer(app, logger)` wrapping production bootstrap
- Fresh in-memory storage per test via `BeforeEach`/`AfterEach`

### Unit & Integration Tests

**Unit Tests** (TDD — written before implementation):

| File | Coverage |
|---|---|
| `internal/domain/consent/service_test.go` | `RevokeConsentForPrincipal`: success, ErrGrantNotFound, transient error fail-closed, audit log emitted |
| `internal/adapters/http/handlers/consent/revoke_grant_handler_test.go` | 204 success, 404 not found, 401 no principal, 400 invalid UUID, 500 service error |
| `internal/adapters/storage/memory/user_grants_test.go` | `DeleteByPrincipalAndAgentID`: success, not found, wrong principal isolation |

**Integration Tests** (PostgreSQL — testcontainers):

| File | Coverage |
|---|---|
| `internal/adapters/storage/postgres/user_grants_integration_test.go` | `DeleteByPrincipalAndAgentID`: success (1 row affected), not found (0 rows), concurrent safety, principal isolation |

**Frontend Tests** (vitest):

| File | Coverage |
|---|---|
| `web/src/components/consent/RevokeGrantDialog.test.tsx` | Dialog renders, Escape closes, Enter confirms, loading state, agent name displayed |
| `web/src/components/consent/RevokeGrantButton.test.tsx` | Button renders with aria-label, click opens dialog |
| `web/src/pages/AgentGrantDetailPage.test.tsx` | Button visible when grant exists, hidden when no grant, success toast + navigation |
| `web/src/components/consent/DelegationCard.test.tsx` | Revoke button present when onRevoke provided, absent when not |

**Test Coverage Goals**:
- Backend unit coverage: all branches of `RevokeConsentForPrincipal` and `RevokeGrantHandler`
- Integration coverage: `DeleteByPrincipalAndAgentID` in both adapters
- E2E coverage: 100% of API-level acceptance scenarios from spec.md (mandatory per Principle XIII)

## Complexity Tracking

No constitution violations to justify. All implementation follows established patterns.
