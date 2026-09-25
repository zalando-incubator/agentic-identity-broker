# Implementation Plan: Tool Approval API & UI

**Branch**: `024-approval-api-ui` | **Date**: 2026-03-28 | **Spec**: [spec.md](spec.md)
**Input**: Feature specification from `/specs/024-approval-api-ui/spec.md`

**Note**: This template is filled in by the `/speckit.plan` command. See `.specify/templates/commands/plan.md` for the execution workflow.

## Summary

Implement a human-in-the-loop tool approval system for the Agentic Identity Broker. When an agent invokes a tool that requires human approval, ExtProc calls the broker to create a pending approval record. The user reviews the tool call context in the Approval UI (React SPA) and approves or denies it with a persistence scope (once/session/permanent). ExtProc syncs approval state via a long-poll endpoint to avoid per-request round-trips. The feature spans the full stack: Go domain service, PostgreSQL-backed storage with long-poll/LISTEN-NOTIFY, RESTful API endpoints (6 new), and React UI components.

## Technical Context

**Language/Version**: Go 1.25.6 (backend), TypeScript + React 19 (frontend)
**Primary Dependencies**: chi v5 (router), sqlx (PostgreSQL), Ginkgo/Gomega (BDD tests), cel-go (authorization policies), lestrrat-go/jwx (JWKS/JWT), React Router DOM 7, Axios (HTTP client), Tailwind CSS v4, Headless UI, CVA (variants)
**Storage**: PostgreSQL (production) + in-memory (dev/test) — hexagonal dual-adapter pattern per ADR 004
**Testing**: Go `testing` + Ginkgo/Gomega E2E + Vitest (frontend unit) + Playwright (frontend E2E)
**Target Platform**: Linux server (Docker/Kubernetes) + browser SPA
**Project Type**: Monorepo (Go backend + React frontend)
**Performance Goals**: Long-poll endpoint must support ≥100 concurrent gateway connections; approval state propagation ≤2 seconds; approval UI interactive within 30 seconds of URL open
**Constraints**: 1-second coalesce window for long-poll wake-ups; 10-minute default TTL on pending approvals; rate limit 10 req/min per (principal, agent_id) pair
**Scale/Scope**: 6 new API endpoints, 2 new DB tables, 1 new domain service, 6 new React components, 1 new SPA route

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

Before proceeding, verify compliance with [.specify/memory/constitution.md](.specify/memory/constitution.md):

**Design Preconditions (BLOCKING)**:

- [x] **Domain Model**: ToolApproval (aggregate root) and ApprovalSyncState entities defined in spec. ApprovalPersistence and ApprovalStatus value objects. Domain events: ApprovalCreated, ApprovalApproved, ApprovalDenied, ApprovalConsumed, ApprovalExpired.
- [x] **Domain Concepts**: ToolApproval, ApprovalSyncState, ApprovalPersistence, ApprovalStatus, and domain events will be added to ARCHITECTURE.md Glossary.
- [x] **Entity IDs**: `ApprovalID` typed ID will be added to `internal/domain/id/gen_ids.go` per ADR 013. ToolApproval has UUID PK.
- [x] **Configuration Design**: Four new config params under `approvals:` — `pending_ttl`, `sync_coalesce_window`, `rate_limit.max_pending_per_pair`, `rate_limit.max_requests_per_minute`. YAML examples in spec.
- [x] **Config Examples**: YAML snippets will be added to `examples/config/`.
- [x] **Helm Chart**: `charts/agentic-identity-broker/values.yaml` will be updated with `approvals.*` configuration block.
- [x] **API Design First**: 6 new endpoints specified in spec. OpenAPI will be designed BEFORE implementation and confirmed by stakeholder.
- [x] **API Documentation**: OpenAPI specs will be added to `/api/enduser/openapi.yaml`.
- [x] **API Changes**: All API changes require stakeholder confirmation before implementation.
- [x] **Database Design**: Migrations 022 (`tool_approvals`) and 023 (`approval_sync_state`) in `/migrations/` using go-migrate naming.
- [x] **E2E Acceptance Tests**: E2E tests for all 27 acceptance scenarios from spec.md will be written BEFORE implementation.
- [x] **E2E Test Mapping**: Each acceptance scenario maps 1:1 to one `It()` block in `tests/e2e/approval_api_test.go`.
- [x] **E2E Red Phase**: E2E tests will contain realistic assertions (HTTP status codes, JSON response bodies, field values) and fail semantically.
- [x] **Frontend Playwright E2E**: Playwright tests will be added in `tests/e2e/frontend/approval_ui_test.go` for all Approval UI scenarios.
- [x] **Frontend Screenshots**: Screenshots will be captured to `tests/e2e/screenshots/` for each approval UI state (pending, approved, denied, expired, forbidden, loading).

**Implementation Considerations**:

- [x] **Security-First**: Approval endpoint auth is split by trust boundary: dual auth (subject token + client assertion) on `POST /api/approvals`, client assertion only on `GET /api/approvals`, subject token only on `POST /api/approvals/{id}/consume`, and acting-user principal auth on browser-facing approval routes. TTL enforcement server-side. No optional security bypasses.
- [x] **Architecture Docs**: ARCHITECTURE.md will be updated with approval domain concepts, long-poll pattern, new API endpoints, and the approval endpoint authentication boundary decision.
- [x] **ADRs**: Long-poll with PostgreSQL LISTEN/NOTIFY is documented in ADR 014, and approval endpoint authentication boundaries are documented in ADR 018.
- [x] **Library-First Security**: JWT/JWKS validation via lestrrat-go/jwx. CEL via cel-go. No custom crypto needed for approval flow.
- [x] **Zalando Guidelines**: All 6 endpoints follow Zalando RESTful API Guidelines (proper status codes, error format, resource naming).
- [x] **End-User Docs**: API documentation will be rendered in `docs/api/` with approval flow examples.
- [x] **Migration Testing**: The approval migrations (022, 023) will have apply/rollback integration tests against real PostgreSQL.
- [x] **Hexagonal Architecture**: `ToolApprovalRepository` port in `internal/ports/storage.go`. In-memory + PostgreSQL adapters. Domain service depends on port, not adapter.
- [x] **Persistence Patterns**: Follows `specs/004-persistence-layer/quickstart.md` — ISP repository, sqlx, StorageError wrapping, dual adapters.

*All BLOCKING checks pass. Implementation may proceed after Phase 0/1 design artifacts are complete.*

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
# Backend — Go hexagonal architecture
internal/
├── domain/
│   ├── id/gen_ids.go                          # + ApprovalID typed ID
│   ├── storage/tool_approval.go               # ToolApproval entity, ApprovalSyncState
│   └── approval/                              # NEW: ApprovalService domain logic
│       ├── service.go                         # Create, Approve, Deny, Consume, Expire
│       ├── service_test.go                    # Unit tests (TDD)
│       ├── rate_limiter.go                    # Per-pair rate limiting logic
│       └── rate_limiter_test.go               # Rate limiter tests
├── ports/
│   └── storage.go                             # + ToolApprovalRepository, ApprovalSyncStateRepository
├── adapters/
│   ├── storage/
│   │   ├── memory/                            # + ToolApproval in-memory adapter
│   │   └── postgres/                          # + ToolApproval PostgreSQL adapter
│   └── http/
│       ├── handlers/approval/                 # NEW: Approval HTTP handlers
│       │   ├── create_handler.go              # POST /api/approvals
│       │   ├── get_handler.go                 # GET /api/approvals/{id}
│       │   ├── approve_handler.go             # POST /api/approvals/{id}/approve
│       │   ├── deny_handler.go                # POST /api/approvals/{id}/deny
│       │   ├── consume_handler.go             # POST /api/approvals/{id}/consume
│       │   └── sync_handler.go                # GET /api/approvals (long-poll)
│       └── routing/enduser.go                 # + approval route registration
├── app/
│   └── builder.go                             # + ApprovalService wiring
└── config/
    └── schema.go                              # + approvals config section

# Database migrations
migrations/
├── 022_create_tool_approvals.up.sql
├── 022_create_tool_approvals.down.sql
├── 023_create_approval_sync_state.up.sql
└── 023_create_approval_sync_state.down.sql

# Frontend — React SPA
web/src/
├── components/approvals/                      # NEW: Approval UI components
│   ├── ApprovalReviewPage.tsx                 # Full approval review page
│   ├── ToolCallCard.tsx                       # Tool call details display
│   ├── PersistenceSelector.tsx                # Once/session/permanent radio group
│   ├── ApprovalConfirmation.tsx               # Post-action confirmation
│   ├── ApprovalLoadingSkeleton.tsx            # Loading state skeleton
│   └── ApprovalErrorBanner.tsx                # Inline error states
├── pages/
│   └── ApprovalPage.tsx                       # NEW: Route-level page /consent/approvals/:id
├── services/api/
│   └── approvals.ts                           # NEW: Approval API client
├── hooks/
│   └── useApproval.ts                         # NEW: Approval data fetching hook
└── types/
    └── approval.ts                            # NEW: TypeScript types

# API contract
api/enduser/openapi.yaml                       # + 6 approval endpoints

# Tests
tests/
├── e2e/
│   ├── approval_api_test.go                   # NEW: Backend E2E tests (27 scenarios)
│   └── frontend/
│       └── approval_ui_test.go                # NEW: Playwright frontend E2E
└── e2e/screenshots/                           # NEW: UI state screenshots

# Configuration & Deployment
examples/config/                               # + approval config examples
charts/agentic-identity-broker/values.yaml     # + approvals.* config
docs/api/                                      # + approval flow documentation
adrs/014-long-poll-listen-notify.md            # ADR for long-poll sync pattern
adrs/018-approval-endpoint-auth-boundaries.md  # ADR for approval endpoint authentication split
```

**Structure Decision**: Follows existing hexagonal architecture. New domain service in `internal/domain/approval/`, new handlers in `internal/adapters/http/handlers/approval/`, new storage port and dual adapters. Frontend components in `web/src/components/approvals/` following design system patterns.

## Implementation Phase Overview

*Detailed task breakdown is in `tasks.md` (generated by `/speckit.tasks`). The table below reflects
the standard phase structure — include or omit optional phases based on this feature's needs.*

| Phase | Purpose | Required? |
|-------|---------|-----------|
| **Phase 0** | Pre-implementation refactoring — not needed, no structural cleanup required | Skip |
| **Phase 1** | Setup — typed IDs, config schema, database migrations | **YES** |
| **Phase 2** | Design Preconditions — domain model, API OpenAPI spec, E2E test red phase | **MANDATORY** |
| **Phase 2.7** | Entity Boilerplate — empty ToolApproval CRUD scaffolding (501 handlers, interface stubs) | **YES** |
| **Phase 2.8** | Foundational Infrastructure — long-poll goroutine manager, LISTEN/NOTIFY subscriber, rate limiter | **YES** |
| **Phase 3** | User Story 1+2 — Approve/Deny flow (core domain logic + handlers + UI) | **YES** |
| **Phase 4** | User Story 3 — ExtProc creates pending approval (POST /api/approvals) | **YES** |
| **Phase 5** | User Story 4 — Long-poll sync endpoint (GET /api/approvals) | **YES** |
| **Phase 6** | User Story 5 — One-time approval consumption | **YES** |
| **Phase 7** | User Story 6 — Permanent approvals in consent management UI | **YES** |
| **Phase N** | Constitution Compliance verification | **MANDATORY** |

**Review ergonomics rationale**: Phases 0 and 2.7 exist to keep PRs focused and reviewable:
- **Phase 0 PR** (if needed): contains only refactoring with no behavior change — reviewers approve
  structural changes first without needing to understand new feature intent
- **Phase 2.7 PR** (if new entities): contains only empty scaffolding (501 handlers, interface stubs)
  — reviewers approve skeleton quickly, then review business logic in sharp focus

*Document which phases apply to this feature and why any optional phases are included or skipped:*

- [x] Phase 0 (refactoring): **Skip** — no pre-existing code requires refactoring for this feature. The approval domain is entirely new.
- [x] Phase 2.7 (entity boilerplate): **Include** — ToolApproval is a new entity requiring repository interfaces, empty handler stubs (returning 501), migration files, and typed ID registration. Isolating this scaffolding from business logic keeps the subsequent PRs focused on domain logic and UI.

## Testing Strategy

<!--
  Per Constitution Principle XIII (End-to-End Acceptance Testing & Spec Traceability):
  All features MUST have E2E acceptance tests mapped 1:1 to spec scenarios.
  Frontend UI changes additionally require Playwright E2E tests and screenshots.

  This section documents HOW E2E tests will be structured and implemented for this feature.
-->

### End-to-End (E2E) Acceptance Tests

**Test Location**: `tests/e2e/approval_api_test.go`

**Framework**: Ginkgo/Gomega BDD framework following patterns in [tests/e2e/README.md](../../tests/e2e/README.md)

**Test Organization**:
- **Top-level Describe**: "Tool Approval API & UI"
- **Nested Describe**: Per user story ("User Approves Pending Tool Call", "User Denies Pending Tool Call", etc.)
- **Context blocks**: Preconditions ("when a pending approval exists", "when the approval is expired")
- **It blocks**: Individual acceptance scenarios (one It() per scenario from spec.md)

**Scenario Mapping**:

| Spec Scenario | E2E Test Location | Test Description |
|---------------|-------------------|------------------|
| US1-S1 (approval page display) | `tests/e2e/approval_api_test.go` | `It("displays tool name, parameters, description, agent, risk level, and persistence choices")` |
| US1-S2 (approve once) | `tests/e2e/approval_api_test.go` | `It("transitions to approved with persistence once")` |
| US1-S3 (approve session) | `tests/e2e/approval_api_test.go` | `It("transitions to approved with persistence session")` |
| US1-S4 (approve permanent) | `tests/e2e/approval_api_test.go` | `It("transitions to approved with persistence permanent with warning")` |
| US1-S5 (forbidden cross-user) | `tests/e2e/approval_api_test.go` | `It("returns 403 when acting user does not match approval principal")` |
| US1-S6 (expired) | `tests/e2e/approval_api_test.go` | `It("shows expiry message for expired approvals")` |
| US1-S7 (OTel tracing) | `tests/e2e/approval_api_test.go` | `It("emits approval.review and approval.approve spans linked to originating trace")` |
| US2-S1 (deny) | `tests/e2e/approval_api_test.go` | `It("transitions to denied on deny action")` |
| US2-S2 (deny OTel) | `tests/e2e/approval_api_test.go` | `It("emits approval.deny span linked to originating trace")` |
| US2-S3 (deny no dedup) | `tests/e2e/approval_api_test.go` | `It("creates new approval for previously denied tool+arguments")` |
| US2-S4 (deny permanent) | `tests/e2e/approval_api_test.go` | `It("transitions to denied with permanent persistence")` |
| US3-S1 (create pending) | `tests/e2e/approval_api_test.go` | `It("creates pending approval with approval_url and emits created span")` |
| US3-S2 (idempotent create) | `tests/e2e/approval_api_test.go` | `It("returns existing approval for duplicate pending request")` |
| US3-S3 (new after consumed) | `tests/e2e/approval_api_test.go` | `It("creates new approval when previous once was consumed")` |
| US3-S4 (new after denied) | `tests/e2e/approval_api_test.go` | `It("creates new approval when previous was denied")` |
| US3-S5 (missing subject token) | `tests/e2e/approval_api_test.go` | `It("returns 401 without Authorization header")` |
| US3-S6 (invalid client assertion) | `tests/e2e/approval_api_test.go` | `It("returns 401 for invalid or missing client assertion on create")` |
| US3-S7 (bad principal) | `tests/e2e/approval_api_test.go` | `It("returns 401 when principal cannot be extracted from subject token")` |
| US4-S1 (sync full) | `tests/e2e/approval_api_test.go` | `It("returns all pairs with approvals and ETag on initial GET")` |
| US4-S2 (long-poll 304) | `tests/e2e/approval_api_test.go` | `It("returns 304 Not Modified when no changes within timeout")` |
| US4-S3 (long-poll wake) | `tests/e2e/approval_api_test.go` | `It("wakes long-poll connection within 2s on approval state change")` |
| US4-S4 (sync bad auth) | `tests/e2e/approval_api_test.go` | `It("returns 401 for invalid client assertion on sync endpoint")` |
| US4-S5 (principal filter) | `tests/e2e/approval_api_test.go` | `It("filters sync response by principal query param")` |
| US5-S1 (consume once) | `tests/e2e/approval_api_test.go` | `It("marks once-persistence approval as consumed")` |
| US5-S2 (consume idempotent) | `tests/e2e/approval_api_test.go` | `It("returns 200 idempotently on already-consumed approval")` |
| US5-S3 (consume non-once) | `tests/e2e/approval_api_test.go` | `It("returns 422 for session or permanent approval consumption")` |
| US6-S1 (permanent visible) | `tests/e2e/approval_api_test.go` | `It("displays permanent approval on consent management page")` |
| US6-S2 (revoke permanent) | `tests/e2e/approval_api_test.go` | `It("revokes permanent approval from consent management page")` |

*Note: Line numbers will be populated during Phase 2f (E2E Acceptance Test Design)*

**Red Phase Requirements**:
- E2E tests MUST compile and contain detailed, realistic expectations
- **Assertions MUST use typed Go response structs** — never `ContainSubstring` or raw JSON string matching
- Response bodies MUST be unmarshaled into typed structs defined in `tests/e2e/helpers/approval_types.go`
- Field assertions MUST use Gomega struct matchers (`HaveField`, `MatchFields`) against typed fields
- `XIt`, `PIt`, `Skip()` are FORBIDDEN
- Tests MUST NOT contain red-phase comments

**E2E Response Types** (defined in `tests/e2e/helpers/approval_types.go`):

```go
// Typed response structs for E2E assertions — no string matching on JSON bodies.

type CreateApprovalResponse struct {
    Data struct {
        ID          string `json:"id"`
        Status      string `json:"status"`
        ApprovalURL string `json:"approval_url"`
        CreatedAt   string `json:"created_at"`
    } `json:"data"`
}

type ApprovalDetailResponse struct {
    Data ApprovalRecord `json:"data"`
}

type ApprovalRecord struct {
    ID               string                 `json:"id"`
    Principal        string                 `json:"principal"`
    AgentID          string                 `json:"agent_id"`
    AgentDisplayName string                 `json:"agent_display_name"`
    ToolName         string                 `json:"tool_name"`
    Arguments        map[string]interface{} `json:"arguments"`
    Description      string                 `json:"description"`
    RiskLevel        string                 `json:"risk_level"`
    Status           string                 `json:"status"`
    Persistence      *string                `json:"persistence"`
    Consumed         bool                   `json:"consumed"`
    ApprovalURL      string                 `json:"approval_url"`
    CreatedAt        string                 `json:"created_at"`
    ApprovedAt       *string                `json:"approved_at"`
    DeniedAt         *string                `json:"denied_at"`
    ConsumedAt       *string                `json:"consumed_at"`
    ExpiresAt        string                 `json:"expires_at"`
}

type ApproveResponse struct {
    Data struct {
        ID          string `json:"id"`
        Status      string `json:"status"`
        Persistence string `json:"persistence"`
        ApprovedAt  string `json:"approved_at"`
    } `json:"data"`
}

type DenyResponse struct {
    Data struct {
        ID          string  `json:"id"`
        Status      string  `json:"status"`
        Persistence *string `json:"persistence"`
        DeniedAt    string  `json:"denied_at"`
    } `json:"data"`
}

type ConsumeResponse struct {
    Data struct {
        ID         string `json:"id"`
        Consumed   bool   `json:"consumed"`
        ConsumedAt string `json:"consumed_at"`
    } `json:"data"`
}

type ApprovalSyncResponse struct {
    Data struct {
        Pairs []ApprovalPair `json:"pairs"`
    } `json:"data"`
}

type ApprovalPair struct {
    Principal            string              `json:"principal"`
    AgentID              string              `json:"agent_id"`
    Approvals            []ApprovalSummary   `json:"approvals"`
    GrantedPermissionSets map[string]any     `json:"granted_permission_sets"`
}

type ApprovalSummary struct {
    ID          string  `json:"id"`
    ToolName    string  `json:"tool_name"`
    ArgsHash    string  `json:"arguments_hash"`
    Status      string  `json:"status"`
    Persistence *string `json:"persistence"`
    Consumed    bool    `json:"consumed"`
    SessionID   *string `json:"session_id"`
}

type ErrorResponse struct {
    Error   string `json:"error"`
    Message string `json:"message"`
}
```

**Example Assertion Patterns** (what E2E tests MUST look like):

```go
// CORRECT — typed struct assertion on create response
var created CreateApprovalResponse
Expect(json.NewDecoder(resp.Body).Decode(&created)).To(Succeed())
Expect(resp.StatusCode).To(Equal(http.StatusCreated))
Expect(created.Data.Status).To(Equal("pending"))
Expect(created.Data.ApprovalURL).To(HavePrefix("http"))
Expect(created.Data.ID).ToNot(BeEmpty())

// CORRECT — typed struct assertion on approval detail
var detail ApprovalDetailResponse
Expect(json.NewDecoder(resp.Body).Decode(&detail)).To(Succeed())
Expect(detail.Data.ToolName).To(Equal("create_pull_request"))
Expect(detail.Data.Arguments).To(HaveKeyWithValue("repo", "acme/app"))
Expect(detail.Data.Description).ToNot(BeEmpty())
Expect(detail.Data.AgentDisplayName).ToNot(BeEmpty())
Expect(detail.Data.Status).To(Equal("pending"))
Expect(detail.Data.ExpiresAt).ToNot(BeEmpty())

// CORRECT — typed struct assertion on approve response
var approved ApproveResponse
Expect(json.NewDecoder(resp.Body).Decode(&approved)).To(Succeed())
Expect(resp.StatusCode).To(Equal(http.StatusOK))
Expect(approved.Data.Status).To(Equal("approved"))
Expect(approved.Data.Persistence).To(Equal("once"))
Expect(approved.Data.ApprovedAt).ToNot(BeEmpty())

// CORRECT — typed error response assertion
var errResp ErrorResponse
Expect(json.NewDecoder(resp.Body).Decode(&errResp)).To(Succeed())
Expect(resp.StatusCode).To(Equal(http.StatusForbidden))
Expect(errResp.Error).To(Equal("forbidden"))

// CORRECT — sync endpoint typed assertions with ETag
var sync ApprovalSyncResponse
Expect(json.NewDecoder(resp.Body).Decode(&sync)).To(Succeed())
Expect(resp.StatusCode).To(Equal(http.StatusOK))
Expect(resp.Header.Get("ETag")).ToNot(BeEmpty())
Expect(sync.Data.Pairs).To(HaveLen(1))
Expect(sync.Data.Pairs[0].Principal).To(Equal("alice@example.com"))
Expect(sync.Data.Pairs[0].Approvals).To(HaveLen(1))
Expect(sync.Data.Pairs[0].Approvals[0].Status).To(Equal("approved"))

// WRONG — string matching on raw body (FORBIDDEN)
// body, _ := io.ReadAll(resp.Body)
// Expect(string(body)).To(ContainSubstring("pending"))  // ← NEVER do this
```

**Test Data Strategy**:
- Use fixtures from `tests/e2e/fixtures/` for stable, reusable test data
- Required fixtures: agents (with client_id for ExtProc), principals (alice/bob for cross-user tests), config (approval TTL, rate limits)
- New fixture creation: `approval_agent` fixture (agent with tool-level requirements), `approval_config` (approval-specific configuration)

**Test Execution Flow**:
1. **Phase 2f (Design)**: Write E2E tests for all 27 spec scenarios with detailed expectations
2. **Verify Red Phase**: Run `ginkgo -v ./tests/e2e/approval_api_test.go` — all tests must FAIL semantically
3. **Implementation**: Implement feature incrementally (phases 3–7)
4. **Verify Green Phase**: E2E tests turn GREEN as implementation satisfies acceptance criteria
5. **Minimal Changes**: Only fixture adjustments during implementation, not test logic

**Bootstrap Strategy**:
- Tests use production bootstrap code via `tests/e2e/bootstrap/` (app.Builder, HTTP server, routing)
- Fresh server and storage for each test (BeforeEach/AfterEach isolation)
- Long-poll tests require concurrent goroutines to hold connections while mutations occur
- Rate limit tests need time-aware fixtures or clock injection

### Frontend Playwright E2E Tests (if UI changes)

<!--
  INCLUDE THIS SECTION if the feature changes any React UI in web/src/.
  Remove this section if the feature is purely backend with no UI changes.
-->

**Test Location**: `tests/e2e/frontend/approval_ui_test.go`

**Framework**: Playwright via existing harness in `tests/e2e/frontend/` and page objects in `tests/e2e/pages/`

**Screenshot Location**: `tests/e2e/screenshots/`

**UI Scenario Mapping**:

| UI Scenario | Playwright Test Location | Screenshot Filename |
|-------------|--------------------------|---------------------|
| Pending approval review page | `tests/e2e/frontend/approval_ui_test.go` | `approval_pending_review.png` |
| Persistence selector with permanent warning | `tests/e2e/frontend/approval_ui_test.go` | `approval_permanent_warning.png` |
| Approval success confirmation (once) | `tests/e2e/frontend/approval_ui_test.go` | `approval_confirmed_once.png` |
| Approval success confirmation (permanent) | `tests/e2e/frontend/approval_ui_test.go` | `approval_confirmed_permanent.png` |
| Denial confirmation | `tests/e2e/frontend/approval_ui_test.go` | `approval_denied_confirmation.png` |
| Expired approval error | `tests/e2e/frontend/approval_ui_test.go` | `approval_expired_error.png` |
| Forbidden (wrong user) error | `tests/e2e/frontend/approval_ui_test.go` | `approval_forbidden_error.png` |
| Not found error | `tests/e2e/frontend/approval_ui_test.go` | `approval_not_found_error.png` |
| Loading skeleton | `tests/e2e/frontend/approval_ui_test.go` | `approval_loading_skeleton.png` |
| Network error with retry | `tests/e2e/frontend/approval_ui_test.go` | `approval_network_error.png` |

**Screenshot Naming Convention**:
- Use descriptive filenames that identify the UI state (e.g., `consent_page_with_github_scopes.png`,
  `revoke_dialog_open_from_detail_page.png`)
- One screenshot per meaningful UI state; capture before and after key interactions if relevant

**Test Execution**:
- Run frontend E2E suite: `ginkgo -v ./tests/e2e/frontend/`
- Screenshots are automatically saved during test runs to `tests/e2e/screenshots/`
- Frontend E2E tests MUST follow the same red-green discipline: write tests first with detailed
  element assertions, verify they fail before implementing the UI

### Unit & Integration Tests

**Unit Tests**:
- Location: `internal/domain/approval/service_test.go`, `internal/domain/approval/rate_limiter_test.go`
- Coverage: Domain logic (state transitions, TTL enforcement, principal verification, rate limiting, idempotency)
- Strategy: TDD - write tests FIRST, verify they FAIL, then implement. Table-driven tests for state transition validation.

**Integration Tests**:
- Location: `internal/adapters/storage/postgres/approval_test.go`
- Coverage: PostgreSQL adapter — CRUD operations, partial unique index enforcement, arguments_hash deduplication, approval_sync_state version increment, LISTEN/NOTIFY integration
- Strategy: Real PostgreSQL via testcontainers. Verify migrations apply/rollback. Test concurrent create (idempotency via unique index).

**Test Coverage Goals**:
- Unit test coverage: All domain invariants — state machine transitions, TTL expiry, principal matching, rate limit enforcement, arguments_hash computation
- Integration test coverage: PostgreSQL adapter (full CRUD), migration apply/rollback, concurrent operations
- E2E test coverage: 100% of acceptance scenarios from spec.md (27 tests — mandatory per Principle XIII)
- Frontend E2E coverage: All 10 UI states documented in screenshot mapping

## Complexity Tracking

| Violation | Why Needed | Simpler Alternative Rejected Because |
|-----------|------------|-------------------------------------|
| Long-poll + LISTEN/NOTIFY | Multi-instance cache invalidation without polling; ExtProc needs real-time approval state sync | Polling (request-per-second per gateway) creates unacceptable load at scale; WebSocket adds client complexity to ExtProc |
| ADR 014 (existing ADR) | Long-poll with PostgreSQL LISTEN/NOTIFY is a novel architectural pattern not covered by existing ADRs | Must be documented per Constitution Principle II (binding ADRs) |
| ADR 018 (new ADR) | Approval endpoints cross two trust boundaries (gateway control-plane and browser/user flows) and therefore require endpoint-specific authentication instead of a blanket policy | Captures why only create uses dual auth, sync uses client assertion only, and consume/browser routes stay single-auth |

## Extension: glob pattern approval matching

This approved extension is specified in [glob-approval-matching.md](glob-approval-matching.md). It adds the neutral, standard-library-only `internal/toolpattern` package for canonicalization, matching, formatting, and precedence, and authorizes its shared use by the broker and ExtProc through ADR 035.
