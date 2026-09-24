# Tasks: End-User Activity & Auditing Experience

**Input**: `specs/038-activity-audit-experience/{spec.md,plan.md,research.md,data-model.md,quickstart.md,contracts/}`

**Tests**: Required by Constitution Principles VIII and XIII. Write compiling, semantically failing tests before each implementation slice. Minimal compile-enabling declarations in the named production files are permitted during the red phase; keep no unfinished stubs in the completed feature. Phase 2f covers all 29 numbered story scenarios before feature implementation.

**Organization**: Shared gates precede five independently testable user-story phases. `[P]` means a task can run beside other ready tasks that touch different files; it does not waive a preceding gate. Paths are relative to the repository root.

## Phase 0: Pre-implementation Seam Preparation

**Purpose**: Land behavior-preserving token-exchange seams separately from the feature. Do not add activity emission here. Existing approval creation already returns `IsNew`; preserve that contract. Complete this isolated change before Phase 2.

- [ ] T001 Add table-driven, compiling red assertions for typed `agent_unknown | no_grant | grant_expired | service_not_in_grant | session_missing | session_expired | policy_denied` reasons while retaining existing OAuth error codes in `internal/domain/tokenexchange/service_test.go` and `internal/domain/tokenexchange/errors_test.go`.
- [ ] T002 Implement typed `DenialReason` on exchange errors and map every failure branch without changing existing public error codes in `internal/domain/tokenexchange/errors.go` and `internal/domain/tokenexchange/service.go`.
- [ ] T003 Add a semantic red test, then a once-per-success post-`Exchange` hook without a recorder dependency in `internal/domain/tokenexchange/service_test.go` and `internal/domain/tokenexchange/service.go`.
- [ ] T004 [P] Confirm the existing duplicate-create test asserts `CreateApprovalResult.IsNew=false`; strengthen it only if needed in `internal/domain/approval/service_test.go`, leaving `internal/domain/approval/service.go` behavior unchanged.

**Checkpoint**: Existing token-exchange and approval tests pass; this seam-only change can be reviewed separately. Keep the navigation behavior change in US1, not in this behavior-preserving phase.

## Phase 1: Setup

**Purpose**: Reuse the existing Go, React, storage, and test infrastructure; do not initialize a second project.

- [ ] T005 Confirm migration `032` is free, dependencies in `go.mod` and `web/package.json` cover the plan, and source paths remain accurate. Compare the plan/quickstart Go version to `go.mod` (1.27.1) and record any correction before design sign-off.

## Phase 2: Design Preconditions (Blocking)

**Purpose**: Complete Principles II, IV, V, VII, IX, X, XI, and XIII before feature implementation. The current fragment, data model, and plan are designs, not evidence that the OpenAPI merge, stakeholder confirmation, or red tests have occurred.

### Phase 2a: Domain Model & Glossary (Principles II, V)

- [ ] T006 Finalize `ActivityEvent` invariants, derived `ActivityThread`/`NeedsAttentionItem`, and user-reviewed wording per emitted type in `data-model.md`. Finalize §10's action-to-operational-audit matrix for every security-critical grant, session, approval, broker issuance, and verified-principal denial transition. Name the existing logger and safe principal/action/outcome/subject ID or request correlation; mark missing seams for one structured log at the committed mutation boundary. Do not assume generic HTTP request logs cover actions. Exclude tokens, approval arguments, raw resource URIs, and unapproved labels.
- [ ] T007 Add the seven Activity glossary terms (Activity Event, Activity Category, Actor, Affected Subject, Activity Thread, Needs-Attention Item, Exploration Context) with relationships to `ARCHITECTURE.md`. In the same feature architecture PR add an Activity subsection for the under-30-second user task finding one recent event among 10,000 (not a DB p95 SLO), principal isolation, approved-only display fields, read-time retention, independent structured audit, non-blocking recording, bounded queue/keyset reads/pruning, and multi-replica advisory-lock coordination. Describe these as planned until implemented.
- [ ] T008 Review proposed PostgreSQL/memory storage ADR in `adrs/037-activity-event-storage.md` against accepted `adrs/004-storage-layer-architecture.md`; record review status without treating the proposed ADR as binding.

**Checkpoint**: The model, reviewed wording, glossary, and storage decision have explicit review evidence.

### Phase 2b: Configuration Design (Principle VII)

- [ ] T009 Record and check the requester's 2026-09-23 approval of a `720h` default and arbitrary positive Go-duration values in `specs/038-activity-audit-experience/spec.md`; do not seek that decision again. API approval is separate (T014).
- [ ] T010 Keep the 30-second attention poll as an SPA constant (T066), not backend configuration. Confirm `plan.md`, `research.md`, and `data-model.md` name only six Activity backend settings: `retention_window=720h`, `page_size=25`, `rollup_bucket=1h`, `queue_size=1024`, `prune_interval=10m`, `prune_batch=5000`.
- [ ] T011 Add the reviewed `activity` YAML section and defaults to `examples/config/activity.yaml`; explain configuration and add the example link in `docs/configuration.md` and `examples/config/README.md`.
- [ ] T012 [P] Add matching defaults and chart comments to `charts/agentic-identity-broker/values.yaml`, render them in `charts/agentic-identity-broker/templates/configmap.yaml`, and update `charts/agentic-identity-broker/README.md`.

**Checkpoint**: The config design, example, docs, and Helm deployment contract agree; no unused runtime setting remains.

### Phase 2c: API Design (Principles IV, X)

- [ ] T013 Merge the revised three read-only paths, schemas, auth, errors, and cursor semantics from `specs/038-activity-audit-experience/contracts/enduser-activity.openapi.yaml` into `api/enduser/openapi.yaml` for stakeholder review **before** handlers; include required RFC3339 UTC `retention_cutoff_at` and response-only `ActivityEvent.related_links`. Leave `api/admin/openapi.yaml` unchanged.
- [ ] T014 Obtain and cite explicit **written** stakeholder sign-off on the revised three-endpoint contract, including `retention_cutoff_at` and `ActivityEvent.related_links`, in `specs/038-activity-audit-experience/spec.md`. The 2026-09-23 retention approval is not API approval. Block all handler implementation until sign-off.

**Checkpoint**: The proposed end-user contract is merged for review; written API approval remains a blocking implementation gate.

### Phase 2d: Database Design (Principle IX)

- [ ] T015 Reconcile `activity_events`, `user_sessions.token_revision`, nullable `reconnect_required_at`, due-time indexes, and the reversible `032` migration design against `specs/038-activity-audit-experience/data-model.md`; require `UNIQUE (principal, dedup_key)` and no foreign keys to removable actors/subjects.
- [ ] T016 Establish whether managed PostgreSQL permits `pg_trgm`; document the supported `GIN (summary gin_trgm_ops)` path or the specified `ILIKE` fallback in `specs/038-activity-audit-experience/plan.md` before authoring `migrations/032_create_activity_events.up.sql`.

**Checkpoint**: Storage constraints, indexes, and rollback behavior are decided before schema implementation.

### Phase 2e: Frontend / Design System Review (Principle XI)

- [ ] T017 Before frontend implementation, review `web/src/design-system/docs/{INDEX,DECISION_TREES,COMMON_MISTAKES,COMPONENT_PAIRING_GUIDE,DESIGN_PRINCIPLES,TOKEN_GUIDE,ACCESSIBILITY_GUIDE,MOTION_GUIDE}.md`. Record actual primitive reuse for Timeline, cards, grant facets, and RetentionFooter plus semantic tokens, focus/keyboard, reduced motion, and mobile/200% reflow evidence in `plan.md`. Keep the Principle XI review gate unchecked until that evidence exists; do not add a second universal primitive.

**Checkpoint**: The sole new universal primitive is Timeline; application components reuse the existing design system.

### Phase 2f: E2E Acceptance Test Design (Principles VIII, XIII)

- [ ] T018 Add isolated grants, sessions, approvals, sensitive/deleted refs, aged-out/no-history live attention, a second principal, and `36h` cutoff fixture in `tests/e2e/fixtures/activity.go`. For the opt-in study, generate exactly 10,000 distinct retained events for `user@example.com` with timestamps relative to startup, every T006-reviewed emitted type (currently 18 in data-model), a uniquely named recent rank-250 target, and a per-type approved what/when/who/affected/outcome answer rubric. Keep FR-014 proof supplemental and the 29 story keys distinct.
- [ ] T019 Write only the seven backend-assigned story `It()` keys (US1 #1/#2/#5/#7, US2 #4, US4 #2, US5 #1) in `tests/e2e/activity_test.go`, with concrete observations from the plan table and T018 fixtures. Add separate unnumbered contract/security tests for filter intersection, invalid cursor, principal isolation, approved fields, read-time retention, FR-014, independent action audit/drop behavior, and unaffiliated tokens.
- [ ] T020 [P] Add page-object actions in `tests/e2e/pages/activity_page.go` for direct-card and detail context, attention next step, exact agent/service/grant filters, navigation, screenshots, and keyboard access. Count each click, Enter/Space, menu opening, or completed query submission; passive rendering costs zero. Expose action counts for SC-002/004/008/010.
- [ ] T021 Write only the 22 frontend-assigned story `It()` keys from the plan table in `tests/e2e/frontend/activity_test.go`, including US1 #4 empty UI, US1 #6 navigation, US3 #6 grant-plus-second-filter, and US4 #3 detail context. Add an unnumbered SC-008 direct-card `It()` requiring one activation without opening detail. Assert attention, filter, and primary-nav action counts; never duplicate a numbered key across suites.
- [ ] T022 Compile and run the new backend and frontend specs in `tests/e2e/activity_test.go` and `tests/e2e/frontend/activity_test.go`; demonstrate semantic failures on contract values, never compilation errors, skips, placeholders, or red-phase comments.

**Checkpoint**: Acceptance tests compile and fail for missing behavior. Screenshot assertions are in place; capture the final images after the UI works.

## Phase 2.7: ActivityEvent Entity Boilerplate

**Purpose**: Isolate the new entity, repository contracts, storage adapters, migration, and read-only handler skeleton from business logic. A skeleton is not a deliverable or a substitute for the story phases.

- [ ] T023 Add `ActivityEventID` to `internal/domain/id/gen_ids.go`, regenerate `internal/domain/id/uuid_ids_gen.go`, and update the ID catalogue in `internal/domain/id/AGENTS.md` per ADR 013.
- [ ] T024 Define the compiling `ActivityEvent`, `ActorRef`, `SubjectRef`, `RelatedRefs`, and `ActivityDetail` types in `internal/domain/activity/event.go`: quote the constraints `principal` “non-empty”, `outcome` “succeeded | failed | blocked | pending”, `occurrence_count` “≥1. >1 only for roll-up types”, and `correlation_id` “Never displayed”.
- [ ] T025 Define the five-method `ActivityEventRepository` interface (`Record`, `ListByPrincipal`, `GetByID`, `ListRelated`, `PruneOlderThan`) plus minimal session status/expiry-read contracts in `internal/ports/storage.go`; keep interfaces segregated.
- [ ] T026 [P] Add a compiling activity-repository adapter and accessor, with safe empty results but no fabricated activity, to `internal/adapters/storage/memory/activity_events.go` and `internal/adapters/storage/memory/adapter.go`.
- [ ] T027 [P] Add a compiling sqlx activity-repository adapter and accessor to `internal/adapters/storage/postgres/activity_events.go` and `internal/adapters/storage/postgres/adapter.go`; do not claim its methods are complete until US1/US3/US4/US5.
- [ ] T028 Create `migrations/032_create_activity_events.up.sql` and `migrations/032_create_activity_events.down.sql` with `id uuid PK`, `principal`/`dedup_key text NOT NULL` and `UNIQUE (principal, dedup_key)`, enum `CHECK`s, `actor_id`/`subject_id`/`correlation_id text NULL`, labels/summary `text NOT NULL`, `detail`/`related_refs jsonb NOT NULL DEFAULT '{}'`, `occurrence_count integer NOT NULL DEFAULT 1 CHECK (>=1)`, timestamps `timestamptz NOT NULL`, and `redacted boolean NOT NULL DEFAULT false`. Add feed/ref/keyword/prune/due-time indexes and session revision/marker columns. Keep rollback safe for a pre-existing `pg_trgm` extension.
- [ ] T029 Create a read-only Activity handler skeleton and pre-wire it through `internal/adapters/http/handlers/activity/handler.go`, `internal/app/handlers.go`, `internal/app/builder.go`, and `internal/adapters/http/routing/enduser.go`; keep authenticated `/attention` before `/{event-id}`, return 501 until stories implement it.
- [ ] T030 Compile the scaffold with `just build` from `Justfile` and keep the semantic red failures in `tests/e2e/activity_test.go` and `tests/e2e/frontend/activity_test.go`; land this scaffold only as an intermediate review step.

**Checkpoint**: Every type and route compiles. The acceptance suite still fails for missing behavior; no 501 handler is treated as complete.

## Phase 2.5: Foundational Infrastructure

**Purpose**: Shared contracts and UI primitive needed before story work. This phase follows 2.7 in execution order despite its historical number.

- [ ] T031 Add the planned `activity` mapstructure section and `ActivityConfig` to `ports.Config` in `internal/ports/config.go`. Keep one unified model and loader.
  Add six defaults, source metadata, and explicit environment bindings in `internal/config/loader.go`: `IDENTITY_BROKER_ACTIVITY_RETENTION_WINDOW`, `IDENTITY_BROKER_ACTIVITY_PAGE_SIZE`, `IDENTITY_BROKER_ACTIVITY_ROLLUP_BUCKET`, `IDENTITY_BROKER_ACTIVITY_QUEUE_SIZE`, `IDENTITY_BROKER_ACTIVITY_PRUNE_INTERVAL`, and `IDENTITY_BROKER_ACTIVITY_PRUNE_BATCH`.
  Register six persistent flags in `cmd/agentic-identity-broker/root.go`: durations `activity.retention_window`, `activity.rollup_bucket`, `activity.prune_interval`; integers `activity.page_size`, `activity.queue_size`, `activity.prune_batch`. Bind each changed flag in `loader.go` `bindFlags` so CLI overrides YAML and environment sources.
  After semantic red tests, validate at startup in `internal/config/validator.go`. Keep approved `720h` default; accept any positive representable Go retention duration including `36h` without day rounding; reject zero, negative, or invalid values. Do not create a backend polling flag.
- [ ] T032 Add minimal revision/marker fields to `internal/domain/storage/user_session.go` so tests in `internal/adapters/storage/memory/user_session_test.go` and `internal/adapters/storage/postgres/user_session_test.go` compile and fail semantically. Then implement revision 1 for existing/new rows, atomic revision increments and marker clearing on token writes, and committed ID/revision return from `Create` in both `user_session.go` adapters.
- [ ] T033 Add a minimal compile-enabling Timeline export, then semantic red tests for ordered-list traversal, status labels, empty/loading states, and reduced motion in `web/src/design-system/components/data-display/Timeline/Timeline.tsx` and `web/src/design-system/components/data-display/Timeline/Timeline.test.tsx`.
- [ ] T034 Implement the reusable accessible `<ol>` Timeline with semantic tokens and reduced-motion support in `web/src/design-system/components/data-display/Timeline/Timeline.tsx` and `web/src/design-system/components/data-display/Timeline/index.ts`; add CSF3/a11y variants in `web/src/design-system/components/data-display/Timeline/Timeline.stories.tsx`.
- [ ] T035 Run focused Timeline tests from `web/package.json` and session repository tests in `internal/adapters/storage/memory/user_session_test.go` and `internal/adapters/storage/postgres/user_session_test.go`; confirm the shared contracts work without implementing story behavior.

**Checkpoint**: Configuration, committed session revision, and Timeline are ready; all story-specific tests remain red.

## Phase 3: User Story 1 - Understand recent activity at a glance (P1)

**Goal**: The user opens a first-class `/activity` view and reads one canonical, understandable account of recent significant events.

**Independent Test**: Report the same grant transition twice, make two distinct updates in one second, reconnect a session twice, and issue 41 broker exchanges. The feed shows distinct transitions once, one accurate access roll-up, historical labels, ordered cards, outcomes, and an empty state. Open Activity from desktop and mobile navigation.

### Tests (write and observe semantic red before implementation)

- [ ] T036 [US1] Add event validation, deterministic key, same-action dedup, distinct-transition, UTC bucket, and roll-up tests to `internal/domain/activity/event_test.go`.
- [ ] T037 [P] [US1] Add exhaustive event-type registry, deny-list, safe labels, wording, raw-URI exclusion, and approved-only response tests to `internal/domain/activity/safefields_test.go`.
- [ ] T038 [P] [US1] Add PostgreSQL tests for concurrent `Record`, `(principal, dedup_key)` uniqueness, distinct reconnection revisions, roll-up counts, ordering, and principal isolation to `internal/adapters/storage/postgres/activity_events_test.go`.
- [ ] T039 [P] [US1] Add minimal compile-enabling exports, then semantic red tests for humanized status, local-time spans, neutral empty history, visible `RetentionFooter` (including paginating and detail-child views), direct-card safe/absent links, and load-more in `web/src/components/activity/ActivityCard.test.tsx` and `web/src/hooks/useActivity.test.ts`. Test malformed initial and later-page cutoff responses: first shows fetch error; later retains cards with a retryable error.
- [ ] T040 [US1] Run the focused red tests in `internal/domain/activity/event_test.go`, `internal/domain/activity/safefields_test.go`, and `tests/e2e/activity_test.go`; failures must be semantic, not missing types.

### Models, services, endpoints, and UI

- [ ] T041 [US1] Enforce non-empty principal and summary, valid category `delegation | session | agent_access | policy_decision | approval | revocation | reconnect | failure`, actor kind `agent | service | user | broker`, subject kind `grant | permission_set | service | session | tool | resource`, historical `display_label`, and immutable non-roll-up fields in `internal/domain/activity/event.go`.
- [ ] T042 [US1] Register every emitted `type` and approved context key in `internal/domain/activity/safefields.go`; implement plain-language summaries, structured `trigger`/`basis`/`consequence`, and one per-type allowlisted context-route choice in `internal/domain/activity/templates.go`. Prefer `/approvals/{approval_id}` for approvals, `/sessions` for session/reconnect, `/agents/{agent_id}` for agent/grant/policy. Never copy raw resource URIs or unapproved labels.
- [ ] T043 [P] [US1] Implement memory `Record`, `ListByPrincipal`, `GetByID`, dedup, roll-ups, stable keyset ordering, and principal scoping in `internal/adapters/storage/memory/activity_events.go`.
- [ ] T044 [P] [US1] Implement sqlx `Record` with `ON CONFLICT (principal, dedup_key) DO NOTHING` for single transitions and atomic count/latest-time upsert for new roll-up observations. Retain `first_occurred_at` from the first observation and add principal-scoped keyset reads in `internal/adapters/storage/postgres/activity_events.go`.
- [ ] T045 [US1] Add a bounded non-blocking post-write queue, lifecycle drainer, no roll-up retries, `activity_events_dropped_total{type}`, and an ERROR drop log on full queue/write error in `internal/domain/activity/recorder.go`. Preserve independent safe structured action audits and the primary result; a recorder error never substitutes for an action audit.
- [ ] T046 [US1] Enqueue `grant.created`, `grant.updated` (scope delta), and `grant.revoked` after committed changes in `internal/domain/consent/service.go`; one ID per successful update, labels before deletion. Keep the existing principal/grant revocation audit and include a safe explicit outcome in that record. Add only missing create/update or alternate-revoke action logs from T006's matrix, with safe IDs and outcome.
- [ ] T047 [US1] Enqueue `session.connected`, revision-distinct `session.reconnected`, refresh roll-ups/failures, expiry, and `session.terminated` in `internal/domain/oauth2session/service.go` after appropriate state writes. Preserve create/refresh/termination audits; distinguish reconnect from connect in the existing log and add only missing marker-commit failure/expiry action logs from T006's matrix with verified principal, action/outcome, and safe session/service ID. Never count downstream uses or log tokens.
- [ ] T048 [US1] Enqueue `approval.requested` only when `CreateApprovalResult.IsNew`; enqueue approved/denied/consumed/revoked/expired transitions after committed writes in `internal/domain/approval/service.go`. Retain existing action audit records, but omit or sanitize any unapproved existing `tool_name` label; add only missing expiry-observer logs per T006. Never log approval arguments; Activity records argument count only.
- [ ] T049 [US1] Record successful RFC 8693 broker issuances as `agent.access_issued` roll-ups and verified-principal typed denials as policy events in `internal/domain/tokenexchange/service.go`. Add missing structured exchange action audits with verified principal, action, outcome, safe stable IDs or request correlation from T006. Local `TokenIssued`/`TokenRequestFailed` logs stay independent; unaffiliated local failures create no user Activity event.
- [ ] T050 [US1] Assemble the feed in `internal/domain/activity/service.go` with one request-clock cutoff and required RFC3339 UTC `retention_cutoff_at` (no day rounding); test `36h` includes a row at the cutoff and excludes one a second earlier. Project required `ActivityEvent.related_links` for feed and detail using existing agent/session/approval read ports. Resolve each unique target once per page (limit 100), omit missing, deleted, or cross-principal targets, and on lookup failure omit the link and log the operational failure. Keep historical text, approved-only fields, and strict `(occurred_at DESC, id DESC)` order.
- [ ] T051 [US1] Replace the 501 feed handler with authenticated `{data:{events,threads,next_cursor,retention_cutoff_at}}`, including required `related_links: []` for missing targets, in `internal/adapters/http/handlers/activity/handler.go`. Test route precedence and `36h` boundary in `internal/adapters/http/routing/enduser_test.go`; wire recorder/service lifecycle in `internal/app/builder.go`.
- [ ] T052 [P] [US1] Mirror `contracts/frontend-types.md` in `web/src/types/activity.ts`. Before caching each page in `web/src/services/api/activity.ts`, reject a missing/invalid `retention_cutoff_at` unless it matches `^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(?:\.\d+)?Z$` and has a finite `Date.parse` result. `useActivity.ts` shows the `useSessions`-style fetch error initially; later failure keeps loaded cards, shows retry, and never appends the malformed page or invents a footer date. Add initial/later-page tests. Include shared `related_links` in the typed feed.
- [ ] T053 [US1] Add lazy `/activity` routing in `web/src/App.tsx`; add the Activity primary link, a visible wrapping mobile link row, and exact-or-child-path active matching in `web/src/components/layout/AppLayout.tsx` without touching the orphan Header.
- [ ] T054 [US1] Compose day-grouped narrative cards, icon-plus-text outcomes, roll-up counts/local spans, a safe direct-card context link from the first `ActivityEvent.related_links` entry (guarded by `web/src/utils/inAppRoute.ts`), neutral empty copy, and load-more in `web/src/components/activity/{ActivityCard,OutcomeStatus,ActivityFeed}.tsx` and `web/src/pages/activity/ActivityPage.tsx`. Own `RetentionFooter` below the historical feed whenever a valid response loads: "Activity before {local date and time} is not shown." Format `retention_cutoff_at` with browser-locale `Intl.DateTimeFormat` as in `SessionCard.tsx`. Keep it for populated, paginating, aged-out/empty, filtered no-match, and detail-child views, never on live attention.
- [ ] T055 Make focused US1 acceptance tests green in both assigned suites. Check neutral aged-out/empty copy with exact local cutoff, direct-card SC-008 context in one activation, primary Activity navigation in one at desktop/mobile/200% zoom, and initial/later-page malformed cutoff failures; capture overview, empty, roll-up, and zoom states.

**Checkpoint**: US1 can be shown and tested without attention, filters, or detail. No event implies a completed downstream action.

## Phase 4: User Story 2 - See what needs attention and what to do next (P1)

**Goal**: Show live pending approval and reconnect items with valid next steps; clear them when source state resolves, independently of historical retention.

**Independent Test**: Cause a rejected refresh and a pending approval. Confirm both appear without relying on an activity row; follow `/sessions` and `/approvals/{id}`. Resolve each and observe clearance on the next poll. Keep blocked events in history only.

### Tests (write and observe semantic red before implementation)

- [ ] T056 [US2] Add table-driven tests for live reconnect and approval derivation, settled state, old-item persistence past history retention, group keys, allowlisted routes, and oldest-first ordering in `internal/domain/activity/service_test.go`.
- [ ] T057 [P] [US2] Add memory/PostgreSQL tests for stale expected revision rejection, first failure timestamp, successful token-write marker clearing, and dropped event persistence in `internal/adapters/storage/memory/user_session_test.go` and `internal/adapters/storage/postgres/user_session_test.go`.
- [ ] T058 [P] [US2] Add minimal compile-enabling exports, then semantic red tests for visible-tab polling, focus refresh, no-store responses, settled state, grouped items, and action links in `web/src/hooks/useNeedsAttention.test.ts` and `web/src/components/activity/NeedsAttentionPanel.test.tsx`.
- [ ] T059 [US2] Run focused semantic red tests for US2 #1–#6 in `tests/e2e/activity_test.go` and `tests/e2e/frontend/activity_test.go` before changing session or attention behavior.

### Models, services, endpoints, and UI

- [ ] T060 [US2] Define nullable `reconnect_required_at`, `token_revision`, conditional `MarkReconnectRequired(ctx, principal, sessionID, expectedRevision, at) (bool, error)`, and `ListActiveByPrincipal` exclusion in `internal/domain/storage/user_session.go` and `internal/ports/storage.go`.
- [ ] T061 [P] [US2] Implement the revision-checked reconnect marker under the session lock in `internal/adapters/storage/memory/user_session.go`.
- [ ] T062 [P] [US2] Implement the same conditional update and marker-aware reads with sqlx in `internal/adapters/storage/postgres/user_session.go`.
- [ ] T063 [US2] Persist the reconnect marker when refresh is rejected or no usable refresh token exists after access expiry, then enqueue `session.refresh_failed` without delaying the primary outcome. Clear the marker on successful refresh/reconnection without stale re-marking in `internal/domain/oauth2session/service.go`.
- [ ] T064 [US2] Derive `reconnect_required` only from an expired/marked current session with an active grant and `approval_pending` only from an unexpired pending approval. An expired/settled approval leaves the live band but any recorded expiry stays historical. Do not derive Needs-Attention Items from Activity events or persist a per-event attention flag in `internal/domain/activity/service.go`.
- [ ] T065 [US2] Serve oldest-first `{data:{items:[...]}}` with `Cache-Control: no-store` at authenticated `/api/activity/attention` in `internal/adapters/http/handlers/activity/handler.go`; generate next routes only from the allowlist in `internal/domain/activity/service.go`.
- [ ] T066 [P] [US2] Poll attention every 30 seconds only while visible, revalidate on focus, and bypass the GET cache in `web/src/hooks/useNeedsAttention.ts` and `web/src/services/api/activity.ts`.
- [ ] T067 [US2] Build the labelled needs-attention region, grouped `+N more` with three visible cards, next-step links, and settled EmptyState in `web/src/components/activity/NeedsAttentionPanel.tsx` and `web/src/pages/activity/ActivityPage.tsx`.
- [ ] T068 [US2] Show a marker-driven Reconnect control even when `is_expired=false`, starting the existing OAuth authorize flow, in `web/src/pages/ThirdPartySessionsPage.tsx` and `web/src/components/sessions/SessionCard.tsx`; do not change `is_expired` API meaning.
- [ ] T069 Make focused US2 scenarios green in assigned backend/frontend suites. Count SC-004 activations from each live attention item to its existing resolving flow (at most two); verify clearance independently of historical retention and capture reconnect, settled, and grouped states under `tests/e2e/screenshots/`.

**Checkpoint**: Attention remains actionable after history is pruned and clears after the underlying source state changes.

## Phase 5: User Story 3 - Explore activity by agent, service, grant, time, and outcome (P2)

**Goal**: Combine, clear, and deep-link feed filters; distinguish no matches from no activity.

**Independent Test**: Seed several agents, services, grants, dates, and outcomes. Apply every filter and a combination, clear them, and test no matches. Match only the exact unresolved approval or current session revision; reject a cursor from another filter set.

### Tests (write and observe semantic red before implementation)

- [ ] T070 [US3] Test filter precedence, bounded limits, cursor filter-hash mismatch, retention cutoff, and exact unresolved `approval_id` or `session_id` plus `token_revision` matching in `internal/domain/activity/service_test.go`. Filtered events must still be retained; live items remain authoritative without history.
- [ ] T071 [P] [US3] Add PostgreSQL tests for combined selective refs, summary/label keyword search, `before`, `30d`, keyset pages, old-row exclusion, and principal isolation to `internal/adapters/storage/postgres/activity_events_test.go`.
- [ ] T072 [P] [US3] Add semantic red URL round-trip, grant-card facet, `grant_id` chip removal/preservation, cursor-reset, no-match reset, and clear-all tests in `web/src/components/activity/ActivityFilterBar.test.tsx` and `web/src/hooks/useActivity.test.ts`. Test a card without approved `related_refs.grant_id` as a separate component case: it has no grant action.
- [ ] T073 [US3] Run focused semantic red US3 #1–#7 assertions in `tests/e2e/activity_test.go` and `tests/e2e/frontend/activity_test.go`.

### Query, endpoint, and UI

- [ ] T074 [US3] Define `ActivityQuery` with `agent_id`, `service_id`, `grant_id`, `window` “24h | 7d | 30d | all”, `before` “RFC3339”, `outcome`, `category`, `q` “maxLength: 100”, `needs_attention`, `cursor`, and `limit` “minimum: 1, maximum: 100, default: 25” in `internal/domain/activity/event.go`.
- [ ] T075 [P] [US3] Implement intersection, principal scope, summary/label keyword search (`pg_trgm` or documented `ILIKE`), current-approval-ID/current-session-revision matching, and bound keyset pages in `internal/adapters/storage/postgres/activity_events.go`.
- [ ] T076 [P] [US3] Implement equivalent filters, retention, and stable pagination semantics in `internal/adapters/storage/memory/activity_events.go`.
- [ ] T077 [US3] Encode `(occurred_at,id)` plus a complete-filter hash into the cursor; apply `max(window start, retention cutoff)` and reject filter-changed cursors in `internal/domain/activity/service.go`.
- [ ] T078 [US3] Validate query params and return `400 invalid_cursor` or contract-defined bad-filter errors in `internal/adapters/http/handlers/activity/handler.go`.
- [ ] T079 [US3] Render search, category/outcome, relative-window tabs, keyword, attention toggle, and facets in `ActivityFilterBar.tsx`, `ActiveFilterChips.tsx`, `ActivityCard.tsx`, and `ActivityPage.tsx`. A card with approved `related_refs.grant_id` offers “View this grant's activity”; otherwise omit it. Set the exact `grant_id` URL param, show a removable grant chip, preserve it when adding filters, and clear it with clear-all. Keep no-match reset and result count.
- [ ] T080 [US3] Use `useSearchParams` as the sole filter source in `web/src/hooks/useActivity.ts`; keep cursors out of URLs and reset pages on changes. In `AgentGrantDetailPage.tsx`, keep `agent_id` activity link and add a distinct `grant_id=grants.id` link only when `useAgentGrants` returns a grant (`UserGrant.id`). In `ThirdPartySessionsPage.tsx`, add the exact `service_id` activity link. For SC-002 seed agent and service events absent from page one; from `/agents/{agent_id}` and `/sessions` the respective link finds all retained matches across pages. Start window/outcome tasks at `/activity` within two activations; keyword search alone is insufficient.
- [ ] T081 Make frontend US3 #6 green from a grant-card or grant-detail entry, combining a second filter, checking exact `grant_id`, removing its chip, then clearing all. Prove SC-002 agent/service events absent from page one via `/agents/{agent_id}` and `/sessions` links, and window/outcome from `/activity`, all within two activations in `tests/e2e/frontend/activity_test.go`. Capture applied/no-match states.

**Checkpoint**: Each filter is useful by itself, combinations intersect, and the original feed returns when filters clear.

## Phase 6: User Story 4 - Inspect a single event in depth (P2)

**Goal**: Open a retained event within the Activity shell; explain why it happened, show related transitions, and expose only approved fields and safe routes.

**Independent Test**: Open an approval sequence and a pending event, follow an existing related route, and attempt to view a cross-user or retained-out event. A credential-bearing resource URI and unapproved context value never appear in feed, detail, or sequence.

### Tests (write and observe semantic red before implementation)

- [ ] T082 [US4] Add owned-ID, retention-before-prune, bounded sequence, approved-context, raw-URI, and missing/cross-user detail tests to `internal/domain/activity/service_test.go`.
- [ ] T083 [P] [US4] Add `ListRelated` retained-window, total count, ascending-sequence, and principal-isolation tests to `internal/adapters/storage/postgres/activity_events_test.go`.
- [ ] T084 [P] [US4] Add minimal compile-enabling exports, then semantic red tests for detail, pending status, unsafe target-as-text, and in-app route guards in `web/src/components/activity/ActivityDetail.test.tsx` and `web/src/utils/inAppRoute.test.ts`.
- [ ] T085 [US4] Run focused semantic red US4 #1–#5 assertions in `tests/e2e/activity_test.go` and `tests/e2e/frontend/activity_test.go`.

### Detail service, endpoint, and UI

- [ ] T086 [US4] Build per-type `Explanation` with non-empty `trigger`, `basis`, and `consequence`; permit only approved `detail.context` keys and per-type allowlisted routes for the shared response-only `ActivityEvent.related_links` in `internal/domain/activity/templates.go` and `internal/domain/activity/safefields.go`. Do not persist links in detail JSONB.
- [ ] T087 [P] [US4] Implement principal-scoped `ListRelated` with `max(windowStart, retention cutoff)`, retained total, and deterministic order in `internal/adapters/storage/memory/activity_events.go` and `internal/adapters/storage/postgres/activity_events.go`.
- [ ] T088 [US4] Assemble the retained owned event and bounded ordered sequence with `sequence_truncated`; return not found for an owned event before retention even if unpruned, and derive pending only from `outcome=pending` in `internal/domain/activity/service.go`.
- [ ] T089 [US4] Implement authenticated `GET /api/activity/{event-id}` with UUID and bounded `sequence_limit` in `internal/adapters/http/handlers/activity/handler.go`; project approved fields and inherited, required `ActivityEvent.related_links` (possibly `[]`) rather than serializing stored JSONB.
- [ ] T090 [US4] Fetch detail through typed `web/src/services/api/activity.ts` and `web/src/hooks/useActivityEvent.ts`; use `web/src/utils/inAppRoute.ts` to reject absolute, scheme-relative, and unknown routes from the shared `ActivityEvent.related_links` array.
- [ ] T091 [US4] Add lazy `/activity/:eventId` child routing in `web/src/App.tsx`; keep the feed and its retention note visible around `ActivityDetail.tsx` in `ActivityPage.tsx`. Show explanation, pending outcome, bounded sequence, and a one-activation detail context link from shared `ActivityEvent.related_links`, guarded by `inAppRoute.ts`; omit links for inaccessible targets.
- [ ] T092 Make focused US4 scenarios green in assigned suites. Count US4 #3's detail-context link separately from the one-action direct-card SC-008 check; assert safe missing-target text without broken link. Capture detail, redacted, and pending states in `tests/e2e/screenshots/`.

**Checkpoint**: Neither the detail nor its sequence leaks unapproved values or claims that a pending action completed.

## Phase 7: User Story 5 - Follow narratives and timelines (P3)

**Goal**: Group service, agent, and approval transitions into readable overlapping timelines and observe expiry while the system is idle.

**Independent Test**: Seed connect, refresh, expire, reconnect, agent access, and a tool approval. The same approval appears in tool and agent threads. Pages report truncated earlier events. Idle expiry and callback-between-ticks are recorded once per session revision or approval ID.

### Tests (write and observe semantic red before implementation)

- [ ] T093 [US5] Add read-time multi-membership, oldest-to-newest sequence, cross-page `total_events`/`truncated`, and no-persisted-thread tests in `internal/domain/activity/threads_test.go`.
- [ ] T094 [P] [US5] Add due-expiry, concurrent advisory-lock prune, apply/rollback/repeat migration, and late-callback tests in `internal/adapters/storage/postgres/activity_events_test.go`.
- [ ] T095 [P] [US5] Add a minimal compile-enabling thread export, then semantic red tests for collapsed Accordion, accessible Timeline, `N earlier events`, and reduced motion in `web/src/components/activity/ActivityThread.test.tsx`.
- [ ] T096 [US5] Run focused semantic red US5 #1–#4 assertions in `tests/e2e/activity_test.go` and `tests/e2e/frontend/activity_test.go`.

### Observers, threading, and UI

- [ ] T097 [US5] Define bounded session/approval due-expiry reads with principal, IDs, current token revision, and expiry time in `internal/ports/storage.go`. Implement indexed selections that exclude old/recorded transitions in `internal/adapters/storage/postgres/user_session.go` and `internal/adapters/storage/postgres/tool_approval_repository.go`.
- [ ] T098 [US5] After T097 defines the due-read port, implement equivalent bounded reads and `approval.expired:{approval_id}` dedup selection in `internal/adapters/storage/memory/user_session.go` and `internal/adapters/storage/memory/tool_approval_repository.go`.
- [ ] T099 [US5] Observe refresh/access expiry only when reconnection is actually required and observe pending approval expiry without a detail read; scan bounded batches on the worker tick and record once per key in `internal/domain/activity/recorder.go` and `internal/app/builder.go`.
- [ ] T100 [US5] Snapshot an expired session's previous revision before an OAuth callback token write; after success enqueue old `session.expired` before `session.reconnected` in `internal/domain/oauth2session/service.go`.
- [ ] T101 [US5] Compute overlapping service lifecycle, agent journey, and tool-flow threads on read from approved refs, with ascending page-local `event_ids`, retained `total_events`, and `truncated` in `internal/domain/activity/threads.go` and `internal/domain/activity/service.go`.
- [ ] T102 [US5] Render collapsed service/agent/tool timelines with the Timeline primitive, accessible ordered lists, and filtered `N earlier events` navigation in `web/src/components/activity/ActivityThread.tsx` and `web/src/pages/activity/ActivityPage.tsx`.
- [ ] T103 [US5] Add bounded PostgreSQL prune under `pg_try_advisory_lock(hashtext('activity_prune'))` and matching memory behavior without weakening read-time retention in `internal/domain/activity/prune.go`, `internal/adapters/storage/postgres/activity_events.go`, and `internal/adapters/storage/memory/activity_events.go`.
- [ ] T104 [US5] Make focused US5 scenarios green in `tests/e2e/activity_test.go` and `tests/e2e/frontend/activity_test.go`; capture timeline, high-volume, and reduced-motion states in `tests/e2e/screenshots/`.

**Checkpoint**: A shared event can belong to several honest narratives; pagination and idle expiry cannot silently lose context.

## Dependencies & Execution Order

### Phase Dependencies

- Phase 0 seam preparation and Phase 1 setup precede the Phase 2 gate. Phase 0 is a separate behavior-preserving review unit.
- Finish T006–T022 first. T009 records the approved retention default; T014 requires separate written API sign-off; T017 records its design review. After T022's semantic-red acceptance, run **T105–T107** as Phase N design verification **before T023**, despite their physical position at the document end. T107 counts 29 distinct story keys across both suites.
- Only after all three design checks pass can Phase 2.7 (T023–T030) create the first feature scaffold. Phase 2.5 (T031–T035) follows 2.7. These historical phase numbers are not sorted numerically; neither scaffold nor foundation is complete feature work.
- After foundation, US1 supplies the feed. US2 needs its UI shell, but live attention can advance separately. US3 and US4 require the feed. US5 uses US2 expiry state, US3 filters, and US4 detail links.
- Phase N implementation verification T108–T120 follows US1–US5. Never infer an implementation pass from design artifacts, a scaffold, or a focused subset of tests.

### User Story Dependency Graph

```text
Phase 0 T001–T004 + Phase 1 T005
                 |
                 v
Phase 2 T006–T022 (design approval + 29 semantic-red scenarios)
                 |
                 v
Phase N design T105–T107 (mandatory before first scaffold)
                 |
                 v
Phase 2.7 T023–T030 -> Phase 2.5 T031–T035 -> US1–US5 T036–T104
                                                 |
                                                 v
                                Phase N implementation T108–T120
```

### Parallel Execution Examples

- **US1**: After the shared types exist, run T037 (safe fields), T038 (PostgreSQL tests), and T039 (UI tests) in distinct files. After T041/T042, memory T043 and PostgreSQL T044 can proceed together.
- **US2**: After T060 defines the revision contract, T061 (memory) and T062 (PostgreSQL) can run together. T058 (UI red tests) can proceed while backend tests are authored.
- **US3**: After T074 fixes query types, T075 (PostgreSQL filters) and T076 (memory filters) can run together. T071 (storage tests) and T072 (UI tests) use distinct files.
- **US4**: T083 (related-query tests) and T084 (safe-link/UI tests) use distinct files and can start after the shared detail DTO exists.
- **US5**: T094 (PostgreSQL expiry/prune tests) and T095 (Timeline UI tests) use distinct files. T098 follows T097 because it uses the due-read port.

## Implementation Strategy

- **Minimum useful release**: Complete Phase 0, Phase 1, Phase 2, Phase N design checks, Phase 2.7, and Phase 2.5; then deliver **US1 and US2 together**. Demo each independently before releasing both.
- **Next increments**: Add US3 and US4 after US1; add US5 after US1–US4. Run each story's focused backend/frontend tests before the next increment, then Phase N implementation checks.
- **Safety gate**: Do not implement handlers without written approval of the revised API. Do not claim Principle VII until the separate constitution amendment is accepted and the unified config is implemented. Never infer a principal from an unaffiliated token failure; Activity recording is best-effort, authorization remains fail-closed.

## Phase N: Constitution Compliance Verification (Mandatory)

### Design Phase Verification

- [ ] T105 Review T007's seven glossary entries and Activity subsection for user-task performance, principal isolation, approved fields, retention, independent audit, bounded queue/keyset/pruning, and multi-replica coordination in `ARCHITECTURE.md`. Verify the proposed ADR review, per-type wording, and PostgreSQL/memory choice before claiming Principles II/V (secondary IX); do not mark unimplemented design as deployed.
- [ ] T106 Verify T009's recorded 2026-09-23 retention approval separately from T013's merged OpenAPI proposal and T014's explicit written three-endpoint API approval (including `retention_cutoff_at` and shared `ActivityEvent.related_links`). Check T017's recorded eight-guide component/token/motion/focus review evidence in `plan.md` before claiming Principles IV/X/XI; proposed artifacts alone do not pass this gate.
- [ ] T107 Count exactly 29 distinct numbered scenario keys: US1 7, US2 6, US3 7, US4 5, US5 4. Check the seven backend and 22 frontend assignments from the plan against compiled, semantically red Ginkgo `It()` blocks in `tests/e2e/activity_test.go` and `tests/e2e/frontend/activity_test.go`; no skips or placeholders. FR-014, FR-016a, principal isolation, unaffiliated-token, and direct-card SC-008 checks are supplemental, not extra numbered story keys (Principles VIII, XIII).

### Implementation Phase Verification

#### API contract (Principles IV/X)

- [ ] T108 Document three authenticated read-only endpoints, cursor errors, attention freshness, and safe example payloads in `docs/api/activity.md`; compare implementation against `api/enduser/openapi.yaml` (Principles IV, X).

#### Configuration (Principle VII)

- [ ] T109 Verify Principle VII through the unified `internal/ports/config.go` model, `internal/config/loader.go` bindings/defaults/source metadata, `internal/config/validator.go` startup checks, and six `cmd/agentic-identity-broker/root.go` flags. Exercise all six settings with file, `IDENTITY_BROKER_ACTIVITY_*` environment, and CLI inputs, source precedence, and CLI `--activity.retention_window=36h` overriding other sources; reject invalid/zero/negative retention. Check `720h` default, YAML examples, docs, and Helm value-to-ConfigMap propagation (after the separate constitution amendment is accepted).

#### Persistence (Principle IX)

- [ ] T110 Verify migration `032` applies, rolls back, and reapplies without data-integrity failures, then exercise every PostgreSQL activity repository method against real PostgreSQL in `migrations/032_create_activity_events.up.sql`, `migrations/032_create_activity_events.down.sql`, and `internal/adapters/storage/postgres/activity_events_test.go` (Principle IX).

#### Security (Principles I/III)

- [ ] T111 Verify principal isolation, approved-only projection, omitted unsafe/missing `ActivityEvent.related_links`, no raw URIs/secrets, allowlisted routes, and fail-closed authorization. On queue-full and failed Activity writes after a committed grant mutation, check the **independent** structured action audit includes verified principal, action, outcome, stable subject ID or correlation, plus the ERROR drop log and `activity_events_dropped_total`; no tokens, arguments, raw URIs, or unapproved labels in `internal/domain/activity/safefields_test.go` and `tests/e2e/activity_test.go` (Principles I, III; secondary VIII).
- [ ] T112 Verify queue-full and failed Activity writes preserve the primary mutation and safe structured action audit independently of the ERROR drop log and incremented `activity_events_dropped_total` in `internal/domain/activity/recorder_test.go` and `tests/e2e/activity_test.go` (Principles I, III; secondary VIII).

#### Architecture (Principles VI/XII)

- [ ] T113 Verify domain-to-port boundaries, sqlx/memory parity, Builder-only wiring, and thin HTTP routing in `internal/ports/storage.go`, `internal/app/builder.go`, and `internal/adapters/http/routing/enduser.go` (primary Principles VI, XII; secondary IX).

#### Tests (Principles VIII/XIII)

- [ ] T114 Verify unit and integration tests were semantically red before green, remained stable, and use no Bash correctness harness in `internal/domain/activity/event_test.go`, `internal/domain/activity/service_test.go`, and `internal/adapters/storage/postgres/activity_events_test.go` (Principle VIII).
- [ ] T115 Run focused and full broker Ginkgo acceptance for only the seven backend-assigned story keys in `tests/e2e/activity_test.go`; check exactly one `It()` per key and unnumbered API/security contract coverage, with no skips (Principle XIII).
- [ ] T116 Run 22 frontend-assigned Playwright stories plus an unnumbered direct-card SC-008 `It()` with `E2E_CAPTURE_SCREENSHOTS=true`. Check SC-002/004/008/010 action counts, including direct-card context without a detail-opening click. Retain 17 named screenshot states from `plan.md` and page-object mappings (Principle XIII).

#### Accessibility (Principle XI)

- [ ] T117 Audit keyboard traversal, non-visual equivalents, contrast (4.5:1 text/3:1 UI), reduced motion, narrow viewport, and 200% zoom with `axe-core` in `tests/e2e/frontend/activity_test.go`; check T017's eight-guide review evidence, semantic tokens, focus behavior, and Timeline stories in `web/src/design-system/components/data-display/Timeline/Timeline.stories.tsx` (Principle XI; secondary XIII).

#### Final evidence (Principles II/VIII/IX/XIII)

- [ ] T118 Run `just check`, `just test`, focused PostgreSQL integration, `just build-all`, and full frontend E2E via `Justfile`; fix failures without weakening either focused acceptance suite. Record red→green/full-suite evidence (Principles VIII, XIII).
- [ ] T119 Exercise the app/API per `quickstart.md` and prove aged-out/no-history FR-014 with exact local cutoff separately from the 29 stories. Add the opt-in standalone `tests/e2e/study/activity/main.go` driver outside both acceptance suites (the frontend suite tears down storage after each `It()`).
  Start PostgreSQL 15 via `bootstrap.NewPostgresFixture(ctx)` with migrations; set `ports.StorageConfig` to postgres, fixture URL, 5s/10s read/write; build `storageadapter.NewAdapter` and seed T018's 10,000-event repository fixture. Reuse `helpers.NewMockUpstreamOAuth2Server`, `fixtures.OAuth2ConfigWithUpstream`, set `ports.Config.Storage`, then `bootstrap.NewServerFactory(...).BuildApp` and `bootstrap.NewEndUserTestServer`.
  After `just web-build`, open headed Chromium with a fresh 1280×800 `X-Remote-User: user@example.com` context. Print `Activity study ready: <URL>/activity` only when a fixture card appears. Exit nonzero on setup/seed/browser error; on SIGINT, SIGTERM, and every error use the same cleanup path, including `PostgresFixture.Close` (Ryuk disabled). New launch means new migrated DB and context.
  Run ten distinct participants, at least two different types each and round-robin coverage of every emitted type. Start timing after card readiness. Score SC-001 per no-detail five-field trial (all correct ≤10s); SC-003 per participant (one preselected paraphrase, ≥9/10 jargon-free); SC-005 ten rank-250 trials (each <30s). Record anonymous answers, times, activations, browser, fixture count, and failures in `plan.md`. If participants/runtime unavailable, leave outcomes unverified. Use the selected production `pg_trgm`/`ILIKE` path; never substitute T120 timings (Principles VIII, XIII).
- [ ] T120 Run the exploratory `BenchmarkActivityEventRepository` on PostgreSQL for 10,000 distinct events, roll-up contention, and 2,000 principals × 10,000 events. Record p50/p95/p99, selected `pg_trgm` or `ILIKE` query plans, and index/WAL size in `plan.md`; if using `ILIKE`, capture its 10,000-row fallback query plan separately. This benchmark is diagnostic, never SC-005 user-task proof or a production SLO (Principles II, IX).

**Checkpoint**: All five story journeys, backend/frontend E2E, privacy, accessibility, deployment config, API documentation, and migration reversal have evidence.
