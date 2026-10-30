# Tasks: Revoke Agent Consent

**Input**: Design documents from `/specs/20260320-120100-revoke-agent-consent/`
**Prerequisites**: plan.md ✅, spec.md ✅, research.md ✅, data-model.md ✅, contracts/delete-grant.yaml ✅, quickstart.md ✅

**Tests**: Per Constitution Principle VIII (TDD), test tasks are included in each phase and MUST be written before implementation.

**Organization**: Tasks grouped by user story to enable independent implementation and testing.

---

## Phase 1: Setup (API Documentation & Glossary)

**Purpose**: API spec and architecture documentation updates — no code dependencies, can start immediately.

- [ ] T001 Merge contracts/delete-grant.yaml DELETE operation into /api/enduser/openapi.yaml under `paths./api/consent/agent/{agent-id}/grants` (operationId: revokeAgentGrant, 204/400/401/404/500 responses)
- [ ] T002 [P] Add GrantRevoked domain term to ARCHITECTURE.md Glossary section: "Domain event representing a user's explicit deletion of their grant for an agent. Emitted as a structured audit log entry carrying principal, agent_id, grant_id, and revoked_at."

---

## 🔒 Phase 2: Design Preconditions [MANDATORY]

**Purpose**: Remaining design verification tasks — all design decisions are confirmed in plan.md; these tasks verify artifacts are in place before implementation begins.

**⚠️ CRITICAL**: No implementation can begin until this entire phase is complete.

### Phase 2e: Frontend / Design System Review [MANDATORY]

- [ ] T003 Review web/src/design-system/docs/DECISION_TREES.md and COMPONENT_PAIRING_GUIDE.md to confirm correct design system primitives for RevokeGrantButton (Button, variant="destructive", error-primary token) and RevokeGrantDialog (Modal primitive from overlays/)
- [ ] T004 [P] Review web/src/design-system/docs/COMMON_MISTAKES.md for destructive action and error-primary token anti-patterns before writing any frontend component

**Checkpoint**: Design system usage confirmed for RevokeGrantButton and RevokeGrantDialog

### Phase 2f: E2E Acceptance Test Design (Red Phase) [MANDATORY]

- [ ] T005 Write E2E acceptance tests in tests/e2e/revoke_grant_test.go using Ginkgo/Gomega — Describe "Revoke Agent Grant" → Context "DELETE /api/consent/agent/{agent-id}/grants" — following patterns in tests/e2e/README.md
- [ ] T006 [P] Map all 8 spec.md acceptance scenarios to individual It() blocks in tests/e2e/revoke_grant_test.go: "returns 204 when grant exists" [US1-S3/US2-S3], "returns 404 when no grant exists" [Edge], "returns 404 for different principal" [SR-001], "returns 401 when no principal header" [SR-001], "returns 400 for invalid agent-id UUID" [API validation], "removes grant from list after successful revocation" [US1-S4], "token exchange rejected after grant revoked" [US3-S1], "immediate token exchange re-attempt rejected" [US3-S2]
- [ ] T007 Verify E2E tests compile and fail semantically (expect 405 Method Not Allowed or 404 until route registered) — confirm red phase by running: `ginkgo -v ./tests/e2e/ -run "Revoke Agent Grant"`
- [ ] T007a Write frontend UI E2E tests in tests/e2e/frontend/revoke_grant_flow_test.go using the existing Go Playwright suite (playwright-community/playwright-go): extend tests/e2e/pages/consent_page.go ConsentPage object with revoke-specific helpers (GetRevokeButton, ClickRevokeButton, GetRevokeDialog, ConfirmRevoke, CancelRevoke, IsRevokeButtonPresent); cover the 5 UI acceptance scenarios not covered by the Ginkgo HTTP API tests: "Revoke All Access button visible on detail page when grant exists" [US1-S1], "confirmation dialog shows agent name and OAuth2 services note" [US1-S2], "Revoke All Access button absent when user has no active grant" [Edge], "Revoke action visible on each overview page agent card" [US2-S1], "overview dialog content matches spec" [US2-S2]; use the same BeforeSuite/BeforeEach Chromium setup pattern as tests/e2e/frontend/consent_flow_test.go; verify all 5 tests fail before implementation (red phase)

**Checkpoint**: E2E acceptance tests written and verified to FAIL before implementation begins (both HTTP API Ginkgo suite and Go Playwright UI suite)

---

## Phase 2.5: Foundational Infrastructure (Storage Port)

**Purpose**: `DeleteByPrincipalAndAgentID` storage port method — blocks ALL user stories; both adapters must implement this before any handler can be built.

**⚠️ CRITICAL**: No user story work can begin until this phase is complete.

- [ ] T008 Add `DeleteByPrincipalAndAgentID(ctx context.Context, principal id.Principal, agentID id.AgentID) error` to UserGrantRepository interface in internal/ports/storage.go — returns ports.ErrNotFound when no matching grant exists
- [ ] T009 [P] Write unit tests for memory adapter DeleteByPrincipalAndAgentID in internal/adapters/storage/memory/user_grants_test.go: success removes from all indexes, not-found returns ports.ErrNotFound, wrong-principal does not delete correct principal's grant (isolation)
- [ ] T010 [P] Write integration tests for postgres adapter DeleteByPrincipalAndAgentID in internal/adapters/storage/postgres/user_grants_integration_test.go: success (1 row affected), not-found (0 rows returns ports.ErrNotFound), concurrent safety, principal isolation
- [ ] T011 [P] Implement DeleteByPrincipalAndAgentID in internal/adapters/storage/memory/user_grants.go: lock, lookup byPrincipalAndAgent key, delete from grants/byPrincipalAndAgent/agent indexes, return ports.ErrNotFound if missing
- [ ] T012 [P] Implement DeleteByPrincipalAndAgentID in internal/adapters/storage/postgres/user_grants.go: write-timeout context, `DELETE FROM user_grants WHERE principal=$1 AND agent_id=$2`, check RowsAffected, return ports.ErrNotFound if 0 rows

**Checkpoint**: Both storage adapters implement DeleteByPrincipalAndAgentID; unit and integration tests pass

---

## Phase 3: User Story 1 — Revoke All Agent Permissions From Detail Page (Priority: P1) 🎯 MVP

**Goal**: Users can revoke all permissions for an agent from the agent detail page. A "Revoke All Access" button opens a confirmation dialog; on confirm, the grant is deleted and the user is redirected to the consent overview with a success notification.

**Independent Test**: Create a grant for an agent via POST /api/consent/agent/{agent-id}/grants, navigate to its detail page, confirm DELETE returns 204, verify GET grants returns null/empty, and verify the RevokeGrantButton is visible when a grant exists and hidden when no grant exists.

### Tests for User Story 1 [MANDATORY — Principle VIII] ⚠️

> **TDD**: Write tests FIRST and verify they FAIL before writing any implementation.

- [ ] T013 [P] [US1] Write unit tests for RevokeConsentForPrincipal in internal/domain/consent/service_test.go: success emits slog audit log (principal, agent_id, grant_id, action=grant_revoked), returns ErrGrantNotFound when storage returns ports.ErrNotFound, returns wrapped error on transient storage failure without deleting grant (fail-closed SR-003)
- [ ] T014 [P] [US1] Write unit tests for RevokeGrantHandler in internal/adapters/http/handlers/consent/revoke_grant_handler_test.go: 204 on success, 404 when service returns ErrGrantNotFound, 401 when no principal header present (check before UUID parse), 400 when agent-id is not a valid UUID, 500 when service returns unexpected error
- [ ] T015 [P] [US1] Write frontend tests for RevokeGrantDialog in web/src/components/consent/RevokeGrantDialog.test.tsx: renders agent name in body text, Escape key closes dialog without calling onConfirm, Enter key triggers onConfirm, shows loading state when isLoading=true, cancel button calls onClose
- [ ] T016 [P] [US1] Write frontend tests for RevokeGrantButton in web/src/components/consent/RevokeGrantButton.test.tsx: renders with aria-label containing agent name, click opens RevokeGrantDialog, dialog calls onRevoked after successful confirm
- [ ] T017 [P] [US1] Write frontend tests for AgentGrantDetailPage revoke section in web/src/pages/AgentGrantDetailPage.test.tsx: RevokeGrantButton visible when grant exists (non-null/non-empty grants), button absent when user has no active grant, success toast shown and navigation to / triggered after onRevoked

### Implementation for User Story 1

- [ ] T018 [US1] Add ErrGrantNotFound sentinel error and RevokeConsentForPrincipal(ctx, principal, agentID) method to internal/domain/consent/service.go — ensure *slog.Logger is on ConsentService struct (add if absent); emit slog.Info on success with fields: action="grant_revoked", principal, agent_id, grant_id; map ports.ErrNotFound → ErrGrantNotFound; fail closed on transient errors (SR-002, SR-003)
- [ ] T019 [US1] Add RevokeConsentForPrincipal(ctx context.Context, principal id.Principal, agentID id.AgentID) error to ConsentService interface in internal/adapters/http/handlers/consent/service_interface.go
- [ ] T020 [US1] Create internal/adapters/http/handlers/consent/revoke_grant_handler.go with RevokeGrantHandler struct and RevokeGrant(w, r) method: check principal (401 if missing) → parse agent-id UUID (400 if invalid) → call RevokeConsentForPrincipal → 204 No Content; map ErrGrantNotFound → 404; map other errors → 500
- [ ] T021 [US1] Add `RevokeGrant *consent.RevokeGrantHandler` field to EnduserHandlers struct in internal/app/handlers.go
- [ ] T022 [US1] Wire `RevokeGrantHandler` instantiation in internal/app/builder.go: `NewRevokeGrantHandler(consentService, logger)` assigned to `app.EnduserHandlers.RevokeGrant`
- [ ] T023 [US1] Register DELETE route in internal/adapters/http/routing/enduser.go: `r.Delete("/api/consent/agent/{agent-id}/grants", h.RevokeGrant.RevokeGrant)` gated on `h.RevokeGrant != nil` (consistent with existing nil-guard pattern)
- [ ] T024 [P] [US1] Create web/src/components/consent/RevokeGrantDialog.tsx: uses Modal primitive from design system overlays/, props (agentName, agentId, isOpen, onConfirm, onClose, isLoading?), body copy includes agent name and OAuth2 services caveat, CTA "Revoke All Access" with error-primary/destructive styling, "Cancel" secondary button, Escape dismisses, Enter confirms (WCAG 2.1 AA via Modal focus trap)
- [ ] T025 [P] [US1] Create web/src/components/consent/RevokeGrantButton.tsx: uses Button primitive with variant="destructive" (error-primary token), props (agentId, agentName, onRevoked), aria-label=`Revoke all access for ${agentName}`, opens RevokeGrantDialog on click, calls deleteGrant(agentId) in onConfirm, calls onRevoked on success
- [ ] T026 [US1] Add `deleteGrant(agentId: string): Promise<void>` method to `ConsentApiService` in web/src/services/api/consent.ts using `apiClient.delete(\`/consent/agent/${agentId}/grants\`)` (Axios, consistent with existing service layer pattern); invalidate the grants cache key (`/consent/agent/${agentId}/grants`) on success; on 404 throw `ApiError` with message "Grant not found — it may have already been revoked"; expose via the existing `consentApi` singleton; retain existing `revokeGrant()` method unchanged (backward-compatible POST workaround)
- [ ] T027 [US1] Update web/src/pages/AgentGrantDetailPage.tsx: render `<RevokeGrantButton agentId={...} agentName={...} onRevoked={...} />` alongside "Approve & Delegate" section only when grants is non-null and non-empty; on onRevoked callback navigate to / (consent overview) and show success toast notification

**Checkpoint**: User Story 1 is fully functional and independently testable. E2E tests for US1 scenarios (204, 404, 401, 400, grant removed from list) pass.

---

## Phase 4: User Story 2 — Revoke Agent Permissions From Consent Overview (Priority: P2)

**Goal**: Each agent card on the consent overview page has an inline "Revoke" action. Clicking opens the shared RevokeGrantDialog; on confirm the card is removed and a success notification is shown.

**Independent Test**: Create grants for multiple agents, open consent overview, click Revoke on one card, confirm dialog, verify only that card is removed while others remain, verify cancel leaves card unchanged.

### Tests for User Story 2 [MANDATORY — Principle VIII] ⚠️

- [ ] T028 [P] [US2] Write frontend tests for DelegationCard revoke action in web/src/components/consent/DelegationCard.test.tsx: "Revoke" button rendered when onRevoke prop provided, button absent when onRevoke not provided, click calls onRevoke with correct agentId
- [ ] T029 [P] [US2] Write frontend tests for ConsentOverviewPage revoke flow in web/src/pages/ConsentOverviewPage.test.tsx: RevokeGrantDialog opens when onRevoke called, card removed from list after successful confirm (deleteGrant resolves), success toast shown, card remains on cancel (dialog closed without confirm), error state shown when deleteGrant rejects

### Implementation for User Story 2

- [ ] T030 [P] [US2] Add optional `onRevoke?: (agentId: string) => void` prop to web/src/components/consent/DelegationCard.tsx: render secondary "Revoke" action button when prop provided, passing agentId to handler on click
- [ ] T030a [P] [US2] Update web/src/components/consent/DelegationList.tsx: add optional `onRevoke?: (agentId: string) => void` to `DelegationListProps` interface, thread it through to each `DelegationCard` via a stable per-item callback; update the custom `memo` comparator (lines ~79-95) to include `prevProps.onRevoke === nextProps.onRevoke` — without this fix the memoized component will not re-render when the `onRevoke` prop reference changes, causing the Revoke button to be stale or absent
- [ ] T031 [US2] Update web/src/pages/ConsentOverviewPage.tsx: add `revokingAgentId: string | null` state, render `<RevokeGrantDialog>` controlled by this state, pass `onRevoke` handler to DelegationList → DelegationCard, on confirm call `deleteGrant(revokingAgentId)`, refetch/invalidate delegation list, show success toast, on close reset revokingAgentId to null

**Checkpoint**: User Stories 1 AND 2 both work independently. Revoke action available on overview cards and on detail page.

---

## Phase 5: User Story 3 — Revocation Enforced at Authorization Time (Priority: P3)

**Goal**: Revoked grants are rejected at token-exchange time. This is already satisfied by the existing `VerifyAgentAccess` hard-delete logic — a missing grant returns `ports.ErrNotFound` which maps to `ErrAgentAccessDenied`. Only the speculative TODO comment cleanup and E2E verification are required.

**Independent Test**: Grant an agent access, revoke via DELETE endpoint, immediately attempt POST /oauth2/token for that agent — verify response is 401/403.

### Implementation for User Story 3

- [ ] T032 [US3] Remove speculative TODO comment block from VerifyAgentAccess in internal/domain/consent/service.go (lines 244-249 suggesting future Revoked field); replace with comment: "// Hard deletion is the revocation mechanism: a missing grant (ports.ErrNotFound) is treated as access denied (fail closed). No revoked status field is needed."
- [ ] T033 [P] [US3] Verify E2E test scenarios in tests/e2e/revoke_grant_test.go for US3 pass after DELETE endpoint is wired: "token exchange is rejected after grant is revoked" [US3-S1] and "immediate token exchange re-attempt is rejected" [US3-S2] — run: `ginkgo -v ./tests/e2e/ -run "Revoke Agent Grant"`

**Checkpoint**: All three user stories independently functional. Token exchange correctly denied after revocation.

---

## 🔒 Phase N: Constitution Compliance & Polish [MANDATORY]

**Purpose**: Verify all constitution requirements are met before marking the feature complete.

### 🔒 Constitution Compliance Verification [MANDATORY]

#### Design Phase Verification [MANDATORY]

- [ ] T034 Verify GrantRevoked domain term added to ARCHITECTURE.md Glossary (Principle V — domain terms documented)
- [ ] T035 [P] Verify DELETE /api/consent/agent/{agent-id}/grants documented in /api/enduser/openapi.yaml and matches contracts/delete-grant.yaml exactly (Principles IV, X)
- [ ] T036 [P] Verify API design confirmed by stakeholder — reference spec clarification session 2026-03-20 in PR description (Principle X — API changes require stakeholder confirmation)
- [ ] T037 [P] Verify no DB migration required — confirm DB-001: user_grants table unchanged, DeleteByPrincipalAndAgentID uses existing schema (Principle IX)
- [ ] T038 Verify design system review completed (T003-T004 done): error-primary token planned for destructive button, Modal primitive used for dialog, no custom CSS bypasses planned (Principle XI)
- [ ] T039 Verify 8 E2E It() blocks in tests/e2e/revoke_grant_test.go each map to exactly one spec.md acceptance scenario with comment reference (Principle XIII)
- [ ] T040 [P] Verify E2E tests confirmed to FAIL in red phase before implementation (T007 completed) (Principle XIII)

#### Implementation Phase Verification [MANDATORY]

**API & Documentation** (Principles IV, X):
- [ ] T041 [P] Verify DELETE endpoint implementation returns correct HTTP status codes: 204 on success, 404 not found, 401 no principal, 400 invalid UUID — matches /api/enduser/openapi.yaml exactly

**Architecture & Documentation** (Principle II):
- [ ] T042 Verify ARCHITECTURE.md Glossary updated with GrantRevoked (if T002 not yet verified as complete)

**Security** (Principles I, III):
- [ ] T043 Verify principal check occurs BEFORE UUID format validation in RevokeGrantHandler (AGENTS.md ordering rule: 401 before 400)
- [ ] T044 [P] Verify slog.Info audit log emitted from RevokeConsentForPrincipal on successful revocation with all required fields: principal, agent_id, grant_id, action=grant_revoked (SR-002)
- [ ] T045 [P] Verify fail-closed behavior: if storage returns a transient error (non-ErrNotFound), RevokeConsentForPrincipal returns the error and does NOT delete the grant (SR-003)
- [ ] T046 [P] Verify cross-user delete returns 404 (not 403): principal-scoped query naturally returns not-found for another user's grant — no existence leakage (SR-001, research.md §4)

**Architecture Patterns** (Principle VI):
- [ ] T047 Verify clean hexagonal flow: DeleteByPrincipalAndAgentID in ports/storage.go interface → memory + postgres adapters implement it → ConsentService uses port → RevokeGrantHandler uses ConsentService interface (no adapter→adapter imports)

**Database & Persistence** (Principle IX):
- [ ] T048 [P] Verify postgres integration tests cover all DB-004 scenarios: success (1 row affected), not-found (0 rows → ports.ErrNotFound), principal isolation (cannot delete other user's grant)

**Testing** (Principle VIII):
- [ ] T049 Verify unit tests for RevokeConsentForPrincipal and RevokeGrantHandler were written before implementation (TDD red-green cycle followed)
- [ ] T050 [P] Verify no raw sql.ErrNoRows surfaces from storage adapters — all not-found cases wrapped as ports.ErrNotFound (quickstart.md requirement)

**E2E Acceptance Testing** (Principle XIII):
- [ ] T051 Verify all 8 Ginkgo It() blocks in tests/e2e/revoke_grant_test.go each map to exactly ONE spec.md acceptance scenario and include scenario comment references
- [ ] T051a Verify all 5 Go Playwright It() blocks in tests/e2e/frontend/revoke_grant_flow_test.go each map to exactly ONE UI acceptance scenario from spec.md (US1-S1, US1-S2, US2-S1, US2-S2, Edge: button hidden)
- [ ] T052 [P] Verify E2E tests written BEFORE implementation and failed initially (red phase, T007 + T007a evidence)
- [ ] T053 [P] Verify E2E tests changed minimally during implementation (fixture adjustments only — no scenario rewrites)
- [ ] T054 Run full E2E test suite and verify all pass: `ginkgo -v ./tests/e2e/ -run "Revoke Agent Grant"` (both HTTP API suite and frontend Playwright suite)

**Frontend** (Principle XI):
- [ ] T055 [P] Verify RevokeGrantButton uses `error-primary` semantic token (variant="destructive") — not `red-600`, `bg-red-*`, or any custom CSS color
- [ ] T056 [P] Verify RevokeGrantDialog uses Modal primitive from web/src/design-system/components/overlays/ for focus trapping (WCAG 2.1 AA)
- [ ] T057 [P] Verify RevokeGrantButton aria-label includes agent name: `aria-label="Revoke all access for {agentName}"` (WCAG 2.1 AA, spec FR-010)
- [ ] T058 [P] Verify no custom CSS bypassing design tokens in RevokeGrantButton.tsx and RevokeGrantDialog.tsx (Principle XI)
- [ ] T059 [P] Verify keyboard navigation: Escape dismisses dialog, Enter confirms (WCAG 2.1 AA requirement)

### Additional Polish

- [ ] T060 Run `just check` (fmt → vet → lint) and `just verify` — all checks must pass with zero issues
- [ ] T061 [P] Verify RevokeConsent (idempotent, used by POST empty-array path) is UNCHANGED — quickstart.md confirms backward compatibility must be maintained

---

## Dependencies & Execution Order

### Phase Dependencies

- **Phase 1 (Setup)**: No dependencies — start immediately
- **Phase 2 (Design Preconditions)**: Can start alongside Phase 1; BLOCKS all implementation
  - 2e (Design System Review): parallel with 2f
  - 2f (E2E Tests, Red Phase): parallel with 2e; E2E tests must fail before Phase 2.5 begins
- **Phase 2.5 (Storage Port)**: Depends on Phase 2 completion — BLOCKS all user stories
- **Phase 3 (US1)**: Depends on Phase 2 + Phase 2.5 completion
- **Phase 4 (US2)**: Depends on Phase 2 + Phase 2.5 completion; reuses RevokeGrantDialog from Phase 3
- **Phase 5 (US3)**: Depends on Phase 3 completion (DELETE endpoint must be wired for E2E verification)
- **Phase N (Compliance)**: Depends on all desired user stories complete

### User Story Dependencies

- **US1 (P1)**: Can start after Phase 2 + 2.5 — independent
- **US2 (P2)**: Can start after Phase 2 + 2.5 — reuses `RevokeGrantDialog` and `deleteGrant()` from US1 (implement US1 frontend components first, or in parallel if two agents working)
- **US3 (P3)**: Requires DELETE endpoint registered (T023) so the E2E token-exchange scenarios can pass

### Within Each User Story

- Tests MUST be written and fail before implementation (TDD)
- Storage port (T008) → adapters (T011, T012) → service (T018) → handler (T020) → wiring (T021-T023)
- Frontend: API client (T026) → Dialog (T024) → Button (T025) → Page updates (T027)

### Parallel Opportunities (within Phase 3 / US1)

```bash
# Run in parallel — different files, no dependencies on each other:
Task T013: "Unit tests for RevokeConsentForPrincipal in internal/domain/consent/service_test.go"
Task T014: "Unit tests for RevokeGrantHandler in internal/adapters/http/handlers/consent/revoke_grant_handler_test.go"
Task T015: "Frontend tests for RevokeGrantDialog in web/src/components/consent/RevokeGrantDialog.test.tsx"
Task T016: "Frontend tests for RevokeGrantButton in web/src/components/consent/RevokeGrantButton.test.tsx"
Task T017: "Frontend tests for AgentGrantDetailPage in web/src/pages/AgentGrantDetailPage.test.tsx"

# After tests written, run in parallel:
Task T018: "RevokeConsentForPrincipal in internal/domain/consent/service.go"
Task T024: "RevokeGrantDialog component in web/src/components/consent/RevokeGrantDialog.tsx"
Task T025: "RevokeGrantButton component in web/src/components/consent/RevokeGrantButton.tsx"
```

---

## Implementation Strategy

### MVP First (User Story 1 Only)

1. Complete Phase 1: API spec + glossary updates
2. Complete Phase 2: Design system review + E2E tests in red phase
3. Complete Phase 2.5: Storage port + both adapter implementations
4. Complete Phase 3: US1 backend (service → handler → wiring) + frontend (Dialog → Button → DetailPage)
5. **STOP and VALIDATE**: Run E2E suite for US1 scenarios, verify pass; manually test detail page revoke flow
6. Deploy/demo if ready

### Incremental Delivery

1. Phase 1 + Phase 2 → Phase 2.5 → Foundation complete
2. Phase 3 (US1) → Detail page revoke functional → **Demo / deploy as MVP**
3. Phase 4 (US2) → Overview page inline revoke → **Demo / deploy**
4. Phase 5 (US3) → TODO cleanup + E2E verification → **Complete**

### Parallel Team Strategy (3 agents)

With three agents after Phase 2 + 2.5 are complete:
- **Agent 1 (golang-pro)**: Phase 3 backend (T013-T014 tests → T018-T023 implementation)
- **Agent 2 (react-specialist)**: Phase 3 frontend (T015-T017 tests → T024-T027 implementation)
- **Agent 3 (golang-pro or react-specialist)**: Phase 4 (T028-T031) once Phase 3 components available

---

## Notes

- `[P]` tasks = different files, no dependencies on each other — safe to run in parallel
- `[US1]`, `[US2]`, `[US3]` labels map tasks to spec.md user stories for traceability
- `RevokeConsent` (idempotent, POST path) is UNCHANGED throughout — only `RevokeConsentForPrincipal` (non-idempotent, DELETE path) is new
- US3 authorization enforcement is **already implemented** via hard-delete + `VerifyAgentAccess` — Phase 5 only removes the TODO comment and verifies via E2E
- All storage not-found cases must use `ports.ErrNotFound` — never raw `sql.ErrNoRows`
- Cross-user delete returns 404 (not 403) by design: principal-scoped query naturally returns not-found for another user's grant
- No database migration required — `user_grants` table schema is unchanged