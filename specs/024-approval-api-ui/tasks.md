# Tasks: Tool Approval API & UI

**Input**: Design documents from `/specs/024-approval-api-ui/`
**Prerequisites**: plan.md (required), spec.md (required for user stories), research.md, data-model.md, contracts/, quickstart.md

**Tests**: Per Constitution Principle VIII (Test-Driven Development & Automated Testing), automated tests are MANDATORY for all features. Test tasks are included in each user story below and MUST be written before or alongside implementation.

**Organization**: Tasks are grouped by user story to enable independent implementation and testing of each story.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Can run in parallel (different files, no dependencies)
- **[Story]**: Which user story this task belongs to (e.g., US1, US2, US3)
- Include exact file paths in descriptions

---

## Phase 1: Setup (Shared Infrastructure)

**Purpose**: Typed IDs, configuration schema, database migrations, and dependency additions

- [X] T001 Add `ApprovalID` typed ID to `internal/domain/id/gen_ids.go` and regenerate (`go generate ./internal/domain/id/...`); update `internal/domain/id/AGENTS.md`
- [X] T002 [P] Add `golang.org/x/time` dependency for rate limiting (`go get golang.org/x/time`)
- [X] T003 [P] Add `ApprovalsConfig` and `ApprovalRateLimitConfig` structs to `internal/ports/config.go` under the `Config` struct; add `Approvals ApprovalsConfig` field to `Config`
- [X] T004 [P] Register Viper defaults for `approvals.*` keys in `internal/config/loader.go` (pending_ttl=10m, sync_coalesce_window=1s, rate_limit.max_pending_per_pair=50, rate_limit.max_requests_per_minute=10)
- [X] T005 [P] Add approvals configuration block to `config.dev.yaml`, `config.test.yaml`, and `examples/config/`
- [X] T006 [P] Update Helm chart for `approvals.*` config: (a) add `approvals.*` block to `charts/agentic-identity-broker/values.yaml` with defaults and `--` doc comments; (b) update ConfigMap/Secret templates under `charts/agentic-identity-broker/templates/` to pass `APPROVAL_PENDING_TTL`, `APPROVAL_SYNC_COALESCE_WINDOW`, `APPROVAL_RATE_LIMIT_MAX_PENDING`, `APPROVAL_RATE_LIMIT_REQUESTS_PER_MINUTE` env vars to pods (Principle VII)

**Checkpoint**: Config schema compiles, defaults load correctly, Helm chart updated

---

## 🔒 Phase 2: Design Preconditions (Blocking Prerequisites)

**Purpose**: Domain model, configuration, API, and database design MUST all be complete before implementation

**⚠️ CRITICAL**: No code implementation can begin until this entire phase is complete

### Phase 2a: Domain Model & Glossary

- [X] T007 Define `ToolApproval` entity struct, `ApprovalStatus` and `ApprovalPersistence` value objects, and domain methods (`Approve`, `Deny`, `Consume`, `IsExpired`, `IsActionable`) in `internal/domain/storage/tool_approval.go`
- [X] T008 [P] Define `ComputeArgumentsHash(arguments map[string]interface{}) string` utility in `internal/domain/storage/tool_approval.go` using `crypto/sha256` on canonical JSON
- [X] T009 [P] Add ToolApproval, ApprovalSyncState, ApprovalPersistence, ApprovalStatus, and all domain events (ApprovalCreated, ApprovalApproved, ApprovalDenied, ApprovalConsumed, ApprovalExpired) to the Glossary section in `ARCHITECTURE.md`

**Checkpoint**: Domain model complete and documented

### Phase 2b: Configuration Design

- [X] T010 Verify configuration YAML examples exist in `examples/config/` (created in T005)
- [X] T011 [P] Update `examples/config/README.md` to reference new approvals configuration section
- [X] T012 [P] Verify Helm chart updated (created in T006)

**Checkpoint**: Configuration requirements designed with YAML examples and Helm chart updated

### Phase 2c: API Design

- [X] T013 Merge the 6 approval endpoints from `specs/024-approval-api-ui/contracts/approval-api.yaml` into `/api/enduser/openapi.yaml`
- [X] T014 Get user/stakeholder confirmation for end-user API design

**Checkpoint**: APIs designed and confirmed by user/stakeholder

### Phase 2d: Database Design

- [X] T015 Create `migrations/008_create_tool_approvals.up.sql` with table, partial unique index, GIN index, principal index, principal+agent index, and expiry index per data-model.md
- [X] T016 [P] Create `migrations/008_create_tool_approvals.down.sql` (`DROP TABLE IF EXISTS tool_approvals`)
- [X] T017 [P] Create `migrations/009_create_approval_sync_state.up.sql` with single-row table and initial seed row per data-model.md
- [X] T018 [P] Create `migrations/009_create_approval_sync_state.down.sql` (`DROP TABLE IF EXISTS approval_sync_state`)

**Checkpoint**: Database schema designed, migrations created

### Phase 2e: Frontend/Design System Review

- [X] T019 Review `web/src/design-system/docs/INDEX.md` and `DECISION_TREES.md` for approval page component selection
- [X] T020 [P] Identify design system primitives: Card, Badge, Radio, Alert, Skeleton, Button; plan semantic token usage (trust-deep, success-primary, warning-primary, error-primary for risk levels)

**Checkpoint**: Design system usage planned

### Phase 2f: E2E Acceptance Test Design

- [X] T021 Create E2E response type structs in `tests/e2e/helpers/approval_types.go` (CreateApprovalResponse, ApprovalDetailResponse, ApprovalRecord, ApproveResponse, DenyResponse, ConsumeResponse, ApprovalSyncResponse, ApprovalPair, ApprovalSummary, ErrorResponse)
- [X] T022 Write E2E acceptance tests for US1+US2 (approve/deny flow) scenarios in `tests/e2e/approval_api_test.go`: US1-S1 through US1-S7, US2-S1 through US2-S4 (11 `It()` blocks)
- [X] T023 [P] Write E2E acceptance tests for US3 (create pending) scenarios in `tests/e2e/approval_api_test.go`: US3-S1 through US3-S6 (6 `It()` blocks)
- [X] T024 [P] Write E2E acceptance tests for US4 (long-poll sync) scenarios in `tests/e2e/approval_api_test.go`: US4-S1 through US4-S5 (5 `It()` blocks)
- [X] T025 [P] Write E2E acceptance tests for US5 (consume) scenarios in `tests/e2e/approval_api_test.go`: US5-S1 through US5-S3 (3 `It()` blocks)
- [X] T026 [P] Write E2E acceptance tests for US6 (permanent visibility/revocation) scenarios in `tests/e2e/approval_api_test.go`: US6-S1 through US6-S2 (2 `It()` blocks)
- [X] T027 Add E2E test fixtures for approval testing: approval_agent fixture and approval_config in `tests/e2e/fixtures/`
- [X] T028 ~~Moved to T028b in Phase 2.7~~ — red phase verification requires handler stubs (Phase 2.7) to compile; see T028b
- [X] T029 Write Playwright frontend E2E tests in `tests/e2e/frontend/approval_ui_test.go` for 10 UI states: pending review, permanent warning, confirmed once, confirmed permanent, denied, expired, forbidden, not found, loading skeleton, network error
- [X] T030 Verify Playwright tests capture screenshots to `tests/e2e/screenshots/` with descriptive filenames (approval_pending_review.png, approval_expired_error.png, etc.)

**Checkpoint**: E2E acceptance tests written and verified to fail semantically before implementation; frontend Playwright tests added and screenshots configured

---

## Phase 2.7: Entity Boilerplate (ToolApproval)

**Purpose**: Empty-but-compiling CRUD scaffolding for ToolApproval entity, isolated from business logic for clean review

### Boilerplate: ToolApproval

- [X] T031 Define three ISP-compliant repository interfaces in `internal/ports/storage.go` (Principle IX — max 5-7 methods each): `ToolApprovalRepository` (Create, Get, Approve, Deny, Consume), `ToolApprovalQueryRepository` (ListAllActive, ListActiveByPrincipalAndAgent, ListPermanentByPrincipal), `ToolApprovalMetricsRepository` (CountPendingByPrincipalAndAgent) — see data-model.md for full signatures; all errors MUST be `StorageError`-wrapped
- [X] T032 [P] Define `ApprovalSyncStateRepository` interface in `internal/ports/storage.go` with methods: GetVersion, IncrementVersion
- [X] T033 [P] Implement in-memory stub struct in `internal/adapters/storage/memory/tool_approval_repository.go` implementing all three interfaces (`ToolApprovalRepository`, `ToolApprovalQueryRepository`, `ToolApprovalMetricsRepository`) — compiling methods returning zero values/not-found
- [X] T034 [P] Implement in-memory `ApprovalSyncStateRepository` in `internal/adapters/storage/memory/approval_sync_state_repository.go` (compiling methods, atomic int64 counter)
- [X] T035 [P] Implement PostgreSQL stub struct in `internal/adapters/storage/postgres/tool_approval_repository.go` implementing all three interfaces (`ToolApprovalRepository`, `ToolApprovalQueryRepository`, `ToolApprovalMetricsRepository`) — compiling methods returning not-implemented
- [X] T036 [P] Implement PostgreSQL `ApprovalSyncStateRepository` stub in `internal/adapters/storage/postgres/approval_sync_state_repository.go` (compiling methods, returning not-implemented)
- [X] T037 Add `toolApprovals ports.ToolApprovalRepository`, `toolApprovalQueries ports.ToolApprovalQueryRepository`, `toolApprovalMetrics ports.ToolApprovalMetricsRepository`, and `approvalSyncState ports.ApprovalSyncStateRepository` fields to `Adapter` struct in `internal/adapters/storage/factory.go`; add typed accessor methods `ToolApprovals()`, `ToolApprovalQueries()`, `ToolApprovalMetrics()`, `ApprovalSyncState()`; wire in both `newMemoryAdapter` and `newPostgresAdapter`
- [X] T038 [P] Create empty approval handler stubs (returning 501) in `internal/adapters/http/handlers/approval/` — one file per endpoint: `create_handler.go`, `get_handler.go`, `approve_handler.go`, `deny_handler.go`, `consume_handler.go`, `sync_handler.go` (plus `revoke_handler.go` and `permanent_handler.go` per OpenAPI spec)
- [X] T039 Register empty approval routes in `internal/adapters/http/routing/enduser.go` within a new `/api/approvals` route group
- [X] T040 Add approval handler fields to `EnduserHandlers` struct in `internal/app/handlers.go` and wire empty handler instances in `internal/app/builder.go`
- [X] T041 Verify project compiles with all new empty scaffolding in place (`just build`)
- [X] T028b Verify E2E tests FAIL semantically (red phase — moved here from Phase 2f because handler stubs are required for tests to compile): run `ginkgo -v ./tests/e2e/` and confirm all 27 tests fail with detailed expectations (not vacuous assertions); confirm no `XIt`/`PIt`/`Skip()` markers; confirm each test file contains spec scenario comment references (e.g. `// Scenario US1-S2 from specs/024-approval-api-ui/spec.md`) per Principle XIII

**Checkpoint**: All ToolApproval scaffolding compiles, empty handlers return 501, no business logic yet; E2E tests verified failing semantically (red phase)

---

## Phase 2.8: Foundational Infrastructure

**Purpose**: Core infrastructure that MUST be complete before ANY user story can be implemented

**⚠️ CRITICAL**: No user story work can begin until this phase is complete

- [X] T042 Write ADR 014 for long-poll with PostgreSQL LISTEN/NOTIFY pattern in `adrs/014-long-poll-listen-notify.md`
- [X] T043 Implement `ApprovalRateLimiter` (in-memory, token-bucket via `golang.org/x/time/rate`, keyed by `principal|agent_id`) in `internal/domain/approval/rate_limiter.go`
- [X] T044 [P] Write unit tests for `ApprovalRateLimiter` in `internal/domain/approval/rate_limiter_test.go` (TDD: red-green)
- [X] T045 [P] Implement `ApprovalSyncBroadcaster` for managing long-poll subscriber channels in `internal/domain/approval/sync_broadcaster.go` (subscribe/unsubscribe/broadcast with coalesce window)
- [X] T046 [P] Write unit tests for `ApprovalSyncBroadcaster` in `internal/domain/approval/sync_broadcaster_test.go` (TDD: red-green — test coalesce, subscribe, unsubscribe, broadcast)
- [X] T047 Implement `ApprovalSyncSubscriber` goroutine for PostgreSQL LISTEN/NOTIFY via raw pgx connection in `internal/adapters/storage/postgres/approval_sync_subscriber.go`
- [X] T047a [P] Implement lazy expiry enforcement in `ApprovalService.GetApproval` and domain method `IsActionable` — domain methods `IsExpired` and `IsActionable` implemented in `internal/domain/storage/tool_approval.go`; service-level 410 Gone and OTel span linking deferred to T052 (ApprovalService creation)

- [X] T118a Update `ARCHITECTURE.md` with the long-poll + PostgreSQL LISTEN/NOTIFY pattern introduced in this phase (Principle II — ARCHITECTURE.md MUST be updated in the same PR that introduces the pattern, not deferred to Phase N)

**Checkpoint**: Foundation ready — rate limiter, sync broadcaster, and LISTEN/NOTIFY subscriber implemented; ARCHITECTURE.md updated for long-poll pattern

---

## Phase 3: User Story 1+2 — Approve/Deny Flow (Priority: P1) 🎯 MVP

**Goal**: User can review a pending tool call in the Approval UI and approve or deny it with a persistence scope

**Independent Test**: Create a pending approval via API, navigate to approval UI, complete approve/deny action, verify state transitions

### Tests for User Story 1+2 [MANDATORY - Principle VIII] ⚠️

- [X] T048 [P] [US1] Write unit tests for `ApprovalService.ApproveApproval` in `internal/domain/approval/service_test.go` (TDD: principal verification, expiry check, state transitions for once/session/permanent)
- [X] T049 [P] [US1] Write unit tests for `ApprovalService.DenyApproval` in `internal/domain/approval/service_test.go` (TDD: principal verification, basic deny, permanent deny)
- [X] T050 [P] [US1] Write unit tests for `ApprovalService.GetApproval` in `internal/domain/approval/service_test.go` (TDD: principal match, not found, forbidden)
- [X] T051 [P] [US1] Write unit tests for `ToolApproval` domain methods (`Approve`, `Deny`, `IsExpired`, `IsActionable`) in `internal/domain/storage/tool_approval_test.go` (TDD: state machine invariants)

### Implementation for User Story 1+2

- [X] T052 [US1] Implement `ApprovalService` constructor and `GetApproval` method in `internal/domain/approval/service.go` (depends on ports only — injects `ToolApprovalRepository`, `ToolApprovalQueryRepository`, `ToolApprovalMetricsRepository`, `ApprovalSyncStateRepository`, `AgentRepository`, config, publicURL; NO concrete adapter types)
- [X] T053 [US1] Implement `ApprovalService.ApproveApproval` in `internal/domain/approval/service.go` (verify principal, check expiry, transition state via repository, increment sync version)
- [X] T054 [US1] Implement `ApprovalService.DenyApproval` in `internal/domain/approval/service.go` (verify principal, check expiry, transition state via repository, increment sync version)
- [X] T055 [US1] Implement in-memory `ToolApprovalRepository` business logic — fully functional `Get`, `Approve`, `Deny` methods in `internal/adapters/storage/memory/tool_approval_repository.go`; all errors MUST be wrapped in domain `StorageError` (never expose raw adapter errors — Principle IX)
- [X] T056 [P] [US1] Implement PostgreSQL `ToolApprovalRepository` — `Get`, `Approve`, `Deny` methods with sqlx parameterized queries in `internal/adapters/storage/postgres/tool_approval_repository.go`; all errors MUST be wrapped in domain `StorageError` (Principle IX)
- [X] T057 [US1] Implement `GET /api/approvals/{id}` handler in `internal/adapters/http/handlers/approval/get_handler.go` (extract principal from X-Remote-User header, parse UUID, call service, return ApprovalDetailResponse)
- [X] T058 [US1] Implement `POST /api/approvals/{id}/approve` handler in `internal/adapters/http/handlers/approval/approve_handler.go` (extract principal from X-Remote-User, parse UUID, validate persistence, call service, return ApproveResponse)
- [X] T059 [US1] Implement `POST /api/approvals/{id}/deny` handler in `internal/adapters/http/handlers/approval/deny_handler.go` (extract principal from X-Remote-User, parse UUID, optional persistence, call service, return DenyResponse)
- [X] T060 [US1] Wire ApprovalService and real handlers in `internal/app/builder.go`; update route registration in `internal/adapters/http/routing/enduser.go`
- [X] T061 [P] [US1] Create TypeScript types in `web/src/types/approval.ts` (ToolApproval, ApproveRequest, DenyRequest, ApprovalDetailResponse, ApproveResponse, DenyResponse)
- [X] T062 [P] [US1] Create API client in `web/src/services/api/approvals.ts` (getApproval, approveApproval, denyApproval)
- [X] T063 [P] [US1] Create `useApproval` data-fetching hook in `web/src/hooks/useApproval.ts`
- [X] T064 [P] [US1] Create `ApprovalLoadingSkeleton` component in `web/src/components/approvals/ApprovalLoadingSkeleton.tsx`
- [X] T065 [P] [US1] Create `ApprovalErrorBanner` component in `web/src/components/approvals/ApprovalErrorBanner.tsx` (expired, already_actioned, forbidden, not_found, network_error states)
- [X] T066 [P] [US1] Create `ToolCallCard` component in `web/src/components/approvals/ToolCallCard.tsx` (tool name, parameters, description, agent name, risk badge)
- [X] T067 [P] [US1] Create `PersistenceSelector` component in `web/src/components/approvals/PersistenceSelector.tsx` (radio group: once/session/permanent with permanent warning)
- [X] T068 [P] [US1] Create `ApprovalConfirmation` component in `web/src/components/approvals/ApprovalConfirmation.tsx` (success/denial confirmation screen)
- [X] T069 [US1] Create `ApprovalReviewPage` component in `web/src/components/approvals/ApprovalReviewPage.tsx` (compose ToolCallCard + PersistenceSelector + action buttons + error states)
- [X] T070 [US1] Create `ApprovalPage` route-level page in `web/src/pages/ApprovalPage.tsx` (fetch approval by ID, render review page or error/loading states)
- [X] T071 [US1] Register route `/consent/approvals/:id` in `web/src/App.tsx` pointing to `ApprovalPage`
- [X] T072 [US1] Link `approval.review` (GET handler), `approval.approve`, and `approval.deny` spans to the traceparent persisted from approval creation request headers in `internal/domain/approval/service.go` — all spans are backend-side
- [X] T072a [US1] Add structured audit logging for all approval lifecycle transitions (approved/denied/consumed/expired) in `internal/domain/approval/service.go` — each log entry MUST contain: principal, agent_id, tool_name, action, persistence (if applicable), and timestamp (SR-005)

**Checkpoint**: User Story 1+2 fully functional — approve/deny flow works end-to-end with UI

---

## Phase 4: User Story 3 — ExtProc Creates Pending Approval (Priority: P1)

**Goal**: ExtProc calls `POST /api/approvals` to register a pending tool call, gets back approval_url

**Independent Test**: Send POST with valid subject token + client assertion, verify pending approval created with approval_url

### Tests for User Story 3 [MANDATORY - Principle VIII] ⚠️

- [X] T073 [P] [US3] Write unit tests for `ApprovalService.CreatePendingApproval` in `internal/domain/approval/service_test.go` (TDD: rate limiting, idempotency, arguments hash, TTL computation, approval_url construction)
- [X] T074 [P] [US3] Write unit tests for `ComputeArgumentsHash` in `internal/domain/storage/tool_approval_test.go` (TDD: deterministic output, key order independence)

### Implementation for User Story 3

- [X] T075 [US3] Implement `ApprovalService.CreatePendingApproval` in `internal/domain/approval/service.go` (compute arguments_hash, check rate limits, check idempotency via repo, create record, increment sync version, construct approval_url)
- [X] T076 [US3] Implement in-memory `ToolApprovalRepository.Create` (with deduplication) and `ToolApprovalMetricsRepository.CountPendingByPrincipalAndAgent` in `internal/adapters/storage/memory/tool_approval_repository.go`; all errors MUST be wrapped in domain `StorageError` (Principle IX)
- [X] T077 [P] [US3] Implement PostgreSQL `ToolApprovalRepository.Create` (ON CONFLICT on partial unique index for idempotency) and `ToolApprovalMetricsRepository.CountPendingByPrincipalAndAgent` in `internal/adapters/storage/postgres/tool_approval_repository.go`; all errors MUST be wrapped in domain `StorageError` (Principle IX)
- [X] T078 [US3] Implement `POST /api/approvals` handler in `internal/adapters/http/handlers/approval/create_handler.go` (dual auth: subject token + client assertion via CEL, parse request body, call service, return CreateApprovalResponse with 201/200)
- [X] T079 [US3] Wire create handler with dual auth middleware (subject token + CEL client assertion) in `internal/adapters/http/routing/enduser.go`
- [X] T080 [US3] Capture `traceparent` request headers for `approval.created` linkage in `internal/adapters/http/handlers/approval/create_handler.go`
- [X] T081 [P] [US3] Write PostgreSQL integration test for approval creation idempotency (concurrent create with same principal+agent+tool+args) in `internal/adapters/storage/postgres/tool_approval_repository_test.go`

**Checkpoint**: User Story 3 fully functional — ExtProc can create pending approvals, idempotent

---

## Phase 5: User Story 4 — Long-Poll Sync Endpoint (Priority: P2)

**Goal**: ExtProc maintains approval cache via `GET /api/approvals` long-poll endpoint

**Independent Test**: Seed approvals, establish long-poll connection with If-None-Match, mutate approval, verify connection returns updated data within 2 seconds

### Tests for User Story 4 [MANDATORY - Principle VIII] ⚠️

- [X] T082 [P] [US4] Write unit tests for `ApprovalService.GetSyncState` in `internal/domain/approval/service_test.go` (TDD: grouping by pair, principal filter, ETag generation)
- [X] T083 [P] [US4] Write unit tests for long-poll handler timeout and wake-up logic in `internal/adapters/http/handlers/approval/sync_handler_test.go` (TDD: immediate return, 304 on timeout, wake on change)

### Implementation for User Story 4

- [X] T084 [US4] Implement `ApprovalService.GetSyncState` in `internal/domain/approval/service.go` (query all active approvals via repo, group by `(principal, agent_id)` pair, return version for ETag)
- [X] T085 [US4] Implement in-memory `ToolApprovalQueryRepository.ListAllActive` and `ListActiveByPrincipalAndAgent` in `internal/adapters/storage/memory/tool_approval_repository.go`; all errors MUST be wrapped in domain `StorageError` (Principle IX)
- [X] T086 [P] [US4] Implement PostgreSQL `ToolApprovalQueryRepository.ListAllActive` and `ListActiveByPrincipalAndAgent` in `internal/adapters/storage/postgres/tool_approval_repository.go`; all errors MUST be wrapped in domain `StorageError` (Principle IX)
- [X] T087 [P] [US4] Implement in-memory `ApprovalSyncStateRepository` fully functional (GetVersion, IncrementVersion with atomic counter) in `internal/adapters/storage/memory/approval_sync_state_repository.go`
- [X] T088 [P] [US4] Implement PostgreSQL `ApprovalSyncStateRepository` (GetVersion, IncrementVersion with `UPDATE ... SET version = version + 1 RETURNING version` + `NOTIFY approval_sync`) in `internal/adapters/storage/postgres/approval_sync_state_repository.go`; all errors MUST be wrapped in domain `StorageError` (Principle IX)
- [X] T089 [US4] Implement `GET /api/approvals` sync handler with long-poll semantics in `internal/adapters/http/handlers/approval/sync_handler.go` (parse ETag + timeout headers, subscribe to broadcaster injected via constructor, select on change/timeout/disconnect, return grouped pairs with ETag)
- [X] T090 [US4] Register sync route in `internal/adapters/http/routing/enduser.go` with client assertion auth middleware — routing function MUST only register routes; `ApprovalSyncBroadcaster` is injected into the handler via `builder.go` and passed pre-wired through `EnduserHandlers` (Principle XII — routing functions MUST NOT wire services)
- [X] T091 [US4] Wire `ApprovalSyncSubscriber` startup and inject `ApprovalSyncBroadcaster` into sync handler constructor in `internal/app/builder.go` as background goroutine alongside HTTP server (connects to PostgreSQL LISTEN, feeds ApprovalSyncBroadcaster)

**Checkpoint**: User Story 4 fully functional — long-poll sync endpoint works with ETag, 304, and real-time wake-up

---

## Phase 6: User Story 5 — One-Time Approval Consumption (Priority: P2)

**Goal**: ExtProc consumes a `once`-persistence approval after use, preventing re-matching

**Independent Test**: Create and approve once-persistence approval, call consume, verify consumed=true; subsequent POST creates fresh record

### Tests for User Story 5 [MANDATORY - Principle VIII] ⚠️

- [X] T092 [P] [US5] Write unit tests for `ApprovalService.ConsumeApproval` in `internal/domain/approval/service_test.go` (TDD: once-only enforcement, idempotent consume, 422 for session/permanent)
- [X] T093 [P] [US5] Write unit tests for `ToolApproval.Consume` domain method in `internal/domain/storage/tool_approval_test.go` (TDD: state invariants)

### Implementation for User Story 5

- [X] T094 [US5] Implement `ApprovalService.ConsumeApproval` in `internal/domain/approval/service.go` (verify once-persistence, mark consumed via repo, increment sync version)
- [X] T095 [US5] Implement in-memory `ToolApprovalRepository.Consume` in `internal/adapters/storage/memory/tool_approval_repository.go`; all errors MUST be wrapped in domain `StorageError` (Principle IX)
- [X] T096 [P] [US5] Implement PostgreSQL `ToolApprovalRepository.Consume` in `internal/adapters/storage/postgres/tool_approval_repository.go`; all errors MUST be wrapped in domain `StorageError` (Principle IX)
- [X] T097 [US5] Implement `POST /api/approvals/{id}/consume` handler in `internal/adapters/http/handlers/approval/consume_handler.go` (extract principal from subject token, parse UUID, call service, return ConsumeResponse)
- [X] T098 [US5] Wire consume handler in `internal/adapters/http/routing/enduser.go`

**Checkpoint**: User Story 5 fully functional — once-persistence approvals can be consumed

---

## Phase 7: User Story 6 — Permanent Approvals in Consent Management (Priority: P3)

**Goal**: Permanent approvals and denials are visible and revocable from the consent management UI

**Independent Test**: Create permanent approval, verify it appears on consent management page, click revoke

### Tests for User Story 6 [MANDATORY - Principle VIII] ⚠️

- [X] T099 [P] [US6] Write unit tests for listing permanent approvals by principal in `internal/domain/approval/service_test.go` (TDD)

### Implementation for User Story 6

- [X] T100 [US6] Add `ListPermanentApprovals(principal)` method to `ApprovalService` in `internal/domain/approval/service.go`
- [X] T101 [US6] Add `RevokePermanentApproval(id, principal)` method to `ApprovalService` in `internal/domain/approval/service.go` (transition permanent approval to denied, increment sync version)
- [X] T102 [US6] Add `GET /api/approvals/permanent` endpoint (auth: `X-Remote-User`, principal implicit from header) in `internal/adapters/http/handlers/approval/permanent_handler.go` — returns permanent approvals and denials for the acting user; add to OpenAPI contract and register route
- [X] T102a [US6] Add `POST /api/approvals/{id}/revoke` endpoint (auth: `X-Remote-User`, principal verification) in `internal/adapters/http/handlers/approval/revoke_handler.go` — calls `RevokePermanentApproval` service method; add to OpenAPI contract
- [X] T103 [US6] Update consent management UI to add a top-level "Tool Authorizations" button/tab that navigates to a permanent approvals view, displaying each agent's permanent approvals/denials with tool name and "Revoke" button
- [X] T104 [US6] Handle revoke action in consent UI — call RevokePermanentApproval API, remove from displayed list

**Checkpoint**: User Story 6 fully functional — permanent approvals visible and revocable

---

## 🔒 Phase N: Constitution Compliance & Polish

**Purpose**: Verify constitution requirements and final polish

### 🔒 Constitution Compliance Verification

#### Design Phase Verification

- [X] T105 Verify domain model design is documented in ARCHITECTURE.md Glossary (Principle V)
- [X] T106 Verify configuration design YAML examples exist in `examples/config/` (Principle VII)
- [X] T107 Verify configuration examples referenced in `examples/config/README.md` (Principle VII)
- [X] T108 Verify API designs documented in `/api/enduser/openapi.yaml` (Principles IV, X)
- [X] T109 Verify user/stakeholder confirmed API designs (document reference in PR) (Principle X)
- [X] T110 Verify database schema design documented — migrations 022 and 023 (Principle IX)
- [X] T111 Verify design system review completed and components identified (Principle XI)
- [X] T112 Verify E2E acceptance tests written in `tests/e2e/` for all 27 spec scenarios (Principle XIII)
- [X] T113 Verify E2E tests verified to FAIL before implementation (red phase): detailed expectations written and failing; no placeholder always-fail assertions; no `XIt`/`PIt`/`Skip()` markers; no "red phase" comments in test files (Principle XIII)
- [X] T114 Verify Playwright E2E tests added in `tests/e2e/frontend/approval_ui_test.go` (Principle XIII)
- [X] T115 Verify screenshots captured to `tests/e2e/screenshots/` (Principle XIII)

#### Implementation Phase Verification

**API & Documentation** (Principles IV, X):

- [X] T116 [P] Verify API implementation matches confirmed OpenAPI specification in `/api/enduser/openapi.yaml` exactly
- [X] T117 [P] Update `docs/api/` with approval flow documentation and integration examples

**Architecture & Documentation** (Principle II):

- [X] T118 Update ARCHITECTURE.md with approval domain concepts and new API endpoints (long-poll pattern was added in T118a during Phase 2.8)
- [X] T119 [P] Verify ADR 014 (long-poll with LISTEN/NOTIFY) is written and accepted in `adrs/014-long-poll-listen-notify.md`

**Configuration** (Principle VII):

- [X] T120 [P] Verify configuration uses unified system configuration port (no custom loading)
- [X] T121 Verify Helm chart reflects all new configuration parameters

**Database & Persistence** (Principle IX):

- [X] T122 [P] Verify database migrations 022 and 023 follow sequential numbering
- [X] T123 [P] Write integration tests verifying migrations apply, rollback, and re-apply against PostgreSQL in `internal/adapters/storage/postgres/approval_test.go`
- [X] T124 [P] Verify all PostgreSQL-backed approval repositories tested in integration tests

**Security** (Principles I, III):

- [X] T125 Verify security features enabled by default — principal verification on all mutations, fail-closed on missing auth
- [X] T126 [P] Verify no custom cryptography used (arguments hash uses standard crypto/sha256)
- [X] T127 [P] Add structured audit logging for approval lifecycle transitions (approved/denied/consumed/expired) with principal, agent_id, tool_name, action, persistence, timestamp

**Architecture Patterns** (Principle VI):

- [X] T128 Verify domain logic (ApprovalService) uses ports (ToolApprovalRepository, ApprovalSyncStateRepository interfaces), not adapters

**Testing** (Principle VIII):

- [X] T129 Verify unit tests written FIRST and failed before implementation (red-green TDD)
- [X] T130 Verify automated tests included (unit + integration)

**E2E Acceptance Testing** (Principle XIII):

- [X] T131 Verify E2E tests in `tests/e2e/approval_api_test.go` cover all 27 spec scenarios (1:1 mapping)
- [X] T132 Verify E2E tests written BEFORE implementation and failed initially (red phase)
- [X] T133 Verify E2E tests turned GREEN as implementation satisfied acceptance criteria
- [X] T134 Run full E2E test suite: `ginkgo -v ./tests/e2e/` (all tests must pass)
- [X] T135 Verify Playwright E2E tests in `tests/e2e/frontend/` pass
- [X] T136 Verify screenshots saved to `tests/e2e/screenshots/` with descriptive filenames
- [X] T137 Run frontend E2E suite: `ginkgo -v ./tests/e2e/frontend/` (all tests must pass)

**Frontend** (Principle XI):

- [X] T138 Verify frontend components use design system primitives and semantic tokens
- [X] T139 Verify no custom CSS bypassing design tokens
- [X] T140 Verify WCAG 2.1 AA accessibility compliance (4.5:1 text, 3:1 UI component contrast, keyboard navigable)

### Additional Polish

- [X] T141 Add `approvals_pending_total` gauge metric labelled by `agent_id` in `internal/domain/approval/service.go`
- [X] T142 [P] Add `long_poll_connections_active` gauge metric in `internal/adapters/http/handlers/approval/sync_handler.go`
- [X] T143 Run `just check` — all fmt, vet, lint, and test checks must pass
- [X] T144 Run quickstart.md validation steps end-to-end

---

## Dependencies & Execution Order

### Phase Dependencies

- **Setup (Phase 1)**: No dependencies — can start immediately
- **Design Preconditions (Phase 2)**: Depends on Phase 1 completion — BLOCKS all implementation
  - Phase 2a, 2b, 2c, 2d, 2e, 2f can proceed in parallel, but all must complete before Phase 2.7
- **Entity Boilerplate (Phase 2.7)**: Depends on all Phase 2 tasks (2a–2f)
- **Foundational Infrastructure (Phase 2.8)**: Depends on Phase 2 + Phase 2.7 — BLOCKS all user stories
- **User Stories (Phase 3–7)**: All depend on Phase 2 + Phase 2.8 + Phase 2.7
  - US1+US2 (Phase 3) must complete before US3 (Phase 4) for approve/deny handlers
  - US3 (Phase 4) must complete before US5 (Phase 6) for consume flow
  - US4 (Phase 5) can proceed in parallel with US3 after Phase 2.8
  - US6 (Phase 7) can start after US1+US2 (needs permanent approval records)
- **Polish (Phase N)**: Depends on all user stories being complete

### User Story Dependencies

- **US1+US2 (P1)**: Can start after Phase 2.8 — no external dependencies
- **US3 (P1)**: Can start after Phase 2.8 — benefits from US1+US2 for integration testing
- **US4 (P2)**: Can start after Phase 2.8 — independent of US1–US3
- **US5 (P2)**: Depends on US3 (needs create flow) and US1+US2 (needs approve flow)
- **US6 (P3)**: Depends on US1+US2 (needs permanent approval records)

### Within Each User Story

- Tests MUST be written and FAIL before implementation
- Domain model → Service → Repository adapters → HTTP handlers → Frontend
- Story complete before moving to next priority

### Parallel Opportunities

- All Phase 1 setup tasks marked [P] can run in parallel
- Phase 2a–2f can proceed in parallel
- Phase 2.7 boilerplate tasks marked [P] can run in parallel
- Within Phase 3: all frontend component tasks marked [P] can run in parallel
- US4 and US3 can be worked on in parallel after Phase 2.8
- All [P] unit test tasks within a phase can run in parallel

---

## Parallel Example: User Story 1+2

```bash
# Launch all unit tests in parallel:
Task T048: "Unit tests for ApproveApproval"
Task T049: "Unit tests for DenyApproval"
Task T050: "Unit tests for GetApproval"
Task T051: "Unit tests for ToolApproval domain methods"

# Launch all frontend components in parallel:
Task T061: "TypeScript types"
Task T062: "API client"
Task T063: "useApproval hook"
Task T064: "ApprovalLoadingSkeleton"
Task T065: "ApprovalErrorBanner"
Task T066: "ToolCallCard"
Task T067: "PersistenceSelector"
Task T068: "ApprovalConfirmation"
```

---

## Implementation Strategy

### MVP First (User Story 1+2 Only)

1. Complete Phase 1: Setup
2. Complete Phase 2: Design Preconditions (all 6 sub-phases in parallel)
3. Complete Phase 2.7: Entity Boilerplate — separate PR, project compiles
4. Complete Phase 2.8: Foundational Infrastructure
5. Complete Phase 3: User Story 1+2 (Approve/Deny flow)
6. **STOP and VALIDATE**: Test approve/deny flow end-to-end with UI
7. Deploy/demo if ready

### Incremental Delivery

1. Setup → Design → Boilerplate → Foundation ready
2. Add US1+US2 → Test independently → Deploy/Demo (MVP!)
3. Add US3 → Test create flow → Deploy/Demo
4. Add US4 → Test long-poll sync → Deploy/Demo
5. Add US5 → Test consume → Deploy/Demo
6. Add US6 → Test consent management → Deploy/Demo
7. Each story adds value without breaking previous stories

### Parallel Team Strategy

With multiple developers or agents:

1. Parallel design work (Phase 2):
   - **Phase 2a (Domain Model)**: architecture-reviewer or golang-pro
   - **Phase 2b (Configuration)**: golang-pro
   - **Phase 2c (API Design)**: architect-reviewer (coordinates with user for confirmation)
   - **Phase 2d (Database Schema)**: golang-pro
   - **Phase 2e (Frontend)**: react-specialist or ui-designer
   - **Phase 2f (E2E Tests)**: golang-pro (writes failing Ginkgo tests)
2. Once ALL of Phase 2 complete:
   - All agents complete Phase 2.7 + Phase 2.8 together
3. Once Phase 2.8 done, assign stories:
   - **US1+US2**: golang-pro (backend) + react-specialist (frontend) in parallel
   - **US3**: golang-pro (after US1+US2 backend done)
   - **US4**: golang-pro (can start in parallel with US3)

---

## Phase 8: Extension — Glob Pattern Approval Matching

### Tests for Glob Pattern Approval Matching [MANDATORY - Principle VIII] ⚠️

- [ ] T145 [P] Add vector-driven unit tests for `internal/toolpattern` covering matching, canonicalization, validation, formatting, and precedence.
- [ ] T146 [P] Add aggregate and approval-service tests for exact-pattern creation, decision resolution, and invalid-pattern rejection.
- [ ] T147 [P] Add storage, handler, and sync tests for persisted and exposed approval patterns.
- [ ] T148 [P] Add backend E2E acceptance coverage for edited, unconstrained, exact, and rejected patterns.
- [ ] T149 [P] Add frontend component and Playwright acceptance tests for the pattern editor and preview.
- [ ] T150 [P] Add migration integration coverage for apply, rollback, re-apply, and exact-pattern backfill.

### Implementation for Glob Pattern Approval Matching

- [ ] T151 Update the OpenAPI contracts, specification artifacts, and rendered approval API documentation for decomposed approval patterns.
- [ ] T152 Implement the shared `internal/toolpattern` package and record the ADR 035 shared-dependency boundary.
- [ ] T153 Extend the approval aggregate, repository port, in-memory and PostgreSQL adapters, and migration 031 with non-null pattern fields.
- [ ] T154 Resolve approval patterns in the domain service, expose them through HTTP read and sync endpoints, and add the React pattern editor.
