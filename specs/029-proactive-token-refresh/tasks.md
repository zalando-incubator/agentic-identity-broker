---
description: "Task list for proactive token refresh"
---

# Tasks: Proactive Token Refresh

**Input**: `specs/029-proactive-token-refresh/plan.md`, `spec.md`, `research.md`, `data-model.md`, `contracts/`, and `quickstart.md`.

**Tests**: The ten original proactive acceptance cases compile and fail semantically before implementation. Preserve their recorded red evidence. The corrected validity regression forces provider rejection before access expiry, then requires acquisition to return no token. US2-S3 extends to the full HTTP rejection and callback-recovery lifecycle. Do not substitute compilation failure, skips, or placeholder assertions.

**Organization**: Phase 0 is a behavior-preserving refactor in this feature branch. The stakeholder waived a separate refactor PR on 2026-10-09. Phase 2 contains design preconditions and the shared foundation. The original proactive mechanism is backend-only. Its corrected validity contract uses online phases 038 prepare, 039 expand, and 040 contract, with end-user API v2 and feature 008's Login recovery.

## Phase 0: Pre-implementation Refactoring

**Purpose**: Prepare the existing HTTP wrapper and locked-refresh path without adding proactive behavior. Complete its tests before feature implementation.

- [X] T001 Create a compiling, behaviorally failing real-server write-deadline test in `internal/adapters/http/server_test.go`. Send a request through `NewHandler` with a short `WriteTimeout` and a handler that lifts its deadline.
- [X] T002 Add `Unwrap() http.ResponseWriter` to the logging `responseWriter` in `internal/adapters/http/middleware.go`. Make T001 pass without changing existing routes.
- [X] T003 [P] Create a table-driven predicate test in `internal/domain/storage/user_session_test.go`. Include a compiling stub only for the red phase. Assert boundary inclusion, `nil` expiry, and equivalence with `!HasValidAccessToken()`.
- [X] T004 Implement `UserSession.AccessTokenExpiresBy(t)` in `internal/domain/storage/user_session.go`. Missing access expiry means unknown expiry, not permanent validity, and is never due (FR-013).
- [X] T005 Add a concurrent-joiner regression test in `internal/domain/oauth2session/service_refresh_concurrency_test.go`. Assert one upstream call and a valid decrypted result for each on-demand caller.
- [X] T006 Create `RefreshTrigger` with exact values `"on-demand"`, `"background"`, and `"sweep"` in `internal/domain/oauth2session/refresh_trigger.go`. Replace `refreshExpiredSession` with threshold-aware `refreshDueSession` in `internal/domain/oauth2session/service.go`. Keep the on-demand threshold at `time.Now()`, keep the row-locked update, and return ciphertext-only sessions from `singleflight`.
- [X] T007 Run the existing OAuth2-session, token-exchange, storage-model, and HTTP tests for `internal/domain/oauth2session/service.go` and `internal/adapters/http/middleware.go`. Run `just check` from `justfile` and confirm no existing route changes.

**Checkpoint**: The behavior-preserving refactor is complete in this branch. The expired-token path and all existing tests still work.

## Phase 1: Setup (Shared Infrastructure)

**Purpose**: Confirm that the existing Go service already has every required dependency.

- [X] T008 Confirm `golang.org/x/sync/singleflight`, OpenTelemetry `metric` and `trace`, chi v5, sqlx, and pgx v5 in `go.mod`. Do not add an ISO 8601 parsing dependency.

## Phase 2: Design Preconditions (Blocking Prerequisites)

**Purpose**: Complete the domain, configuration, API, database, and E2E preconditions before feature implementation. Phase 2.5 is the shared foundation and begins only after these gates pass.

### Phase 2a: Domain Model & Glossary

**Constitution**: Principles II and V.

- [X] T009 Record the proactive design in accepted ADR 038 and the corrected validity contract in accepted ADR 040. Preserve the bounded pool, locking, external sweep, deadline lift, and drain-before-cancel decisions. ADR 040 narrowly supersedes availability after terminal provider rejection. Transient errors and dropped work still permit an otherwise valid access token.
- [X] T010 Add ADR 038 to the decision index in `AGENTS.md` after the ADR is accepted.
- [X] T011 Add Refresh Lookahead Window, Proactive Refresh, Session Sweep, SweepRequest, SweepResult, RefreshTrigger, and SessionTokensRefreshed with their relationships to the glossary in `ARCHITECTURE.md`. Use `specs/029-proactive-token-refresh/data-model.md`.
- [X] T012 Align DB-002, DB-003, FR-003, FR-009, the model, and `contracts/ports.md` on locked update-only session-state persistence. Include `refresh_token_rejected` in L4. Preserve L3: callback `(false, nil)` or `(_, err)` never writes. Terminal rejection commits through `(true, nil)` before the service returns the saved error. It does not count as refresh success.

**Checkpoint**: The aggregate, value objects, invariants, glossary, ADR, and spec wording agree before code changes.

### Phase 2b: Configuration Design

**Constitution**: Principle VII. The broker is a chart-managed workload.

- [X] T013 Add the documented `token_refresh` YAML block with defaults `5m`, `10`, and `100` to `examples/config/third-party-oauth2.yaml`. Include the provider-lifetime and worker-cap guidance from `specs/029-proactive-token-refresh/contracts/configuration.md`.
- [X] T014 Reference the token-refresh settings in the third-party OAuth2 entry of `examples/config/README.md`.
- [X] T015 [P] Add `broker.tokenRefresh` defaults and `# --` comments to `charts/agentic-identity-broker/values.yaml`. Add the strict object, duration pattern, and integer bounds `1–20` and `1–1000` to `charts/agentic-identity-broker/values.schema.json`.
- [X] T016 [P] Render all three `token_refresh` keys from chart values in `charts/agentic-identity-broker/templates/configmap.yaml`. Document their types and defaults in `charts/agentic-identity-broker/README.md`. Do not add a CronJob template.

**Checkpoint**: Example YAML and the Helm deployment contract define the same keys and defaults.

### Phase 2c: API Design

**Constitution**: Principles IV and X. There is no end-user API change.

- [X] T017 Merge the Sessions tag, `POST /api/sessions/sweep`, and request/result schemas from `specs/029-proactive-token-refresh/contracts/admin-session-sweep.openapi.yaml` into `api/admin/openapi.yaml`. Preserve proxy authentication, optional inputs, 200/400/500 responses, and count-only output.
- [X] T018 Obtain written stakeholder approval of `api/admin/openapi.yaml` before implementing the endpoint. Record its PR or issue reference in `specs/029-proactive-token-refresh/plan.md`. Explicitly cover the 1000 page cap, restricted ISO 8601 grammar, unknown-field rejection, and missing-principal 500.

**Checkpoint**: The published admin contract has written approval. Do not start the endpoint without T018.

### Phase 2d: Database Design

**Constitution**: Principle IX. Migration 037 remains unchanged. The shared validity rollout adds a stable SQL boundary before expanding the session table.

- [X] T019 Obtain maintainer acceptance of the named Principle IX exception in ADR 039. Record its 2026-10-12 in-place amendment after the stakeholder chose direct migration execution in this unmerged PR. Retain the separate migration image and Helm migration/grants Job. Document the loss of pre-execution validation in `research.md` and `plan.md`.
- [X] T020 Retain migration `037_user_sessions_access_token_expiry_index.{up,down}.sql` and its concurrent-index design. Preserve lifecycle, interrupted-index recovery, and planner coverage. Use online validity phases 038 prepare, 039 expand, and 040 contract. Preserve legacy strict readers/writes at 38. Roll preparation brokers across every replica and drain all unprepared replicas before 39 activates durable rejection. Keep the same invoker SQL boundary at every phase. Cover serving preparation apps during 39/40, fail-fast lock contention, data preservation, guarded DOWN 039, DOWN 040's retained rejection and nullable legacy column, dirty-metadata inspection, and reapply. Cover actual rendered Helm upgrade refusal at target 0, phase arguments in both grants modes, and execution through the real migration image. Positive stage execution must abort on dirty metadata and leave equal/newer clean schemas unchanged. Serialize migration runners and use explicit separate-runner DOWN for rollback. Document API compatibility, retained SPA assets, and rolling-capacity prerequisites. Never rewrite history, infer rejection from dates, clear rejection, automate `Force`, or delete rows automatically.

**Checkpoint**: Amended ADR 039 defines migration `037` without a guard. Both runners execute concurrent SQL outside explicit transactions. Before renumbering, PostgreSQL lifecycle, invalid-index recovery, and normal-planner index tests passed after the cutover.

### Phase 2e: Frontend/Design System Review

**Constitution**: The original proactive mechanism changes no frontend component. The shared validity cutover uses feature 008's session-card status and built-browser Login recovery coverage. Backend HTTP tests separately prove post-recovery token exchange.

### Phase 2f: E2E Acceptance Test Design

**Constitution**: Principle XIII. Use `Describe` → story `Describe` → `Context` → one `It()` per spec scenario. Use production bootstrap and fixtures.

- [X] T021 Add an idempotent `WithTokenResponseGate() (release func())` to `tests/e2e/helpers/mock_upstream.go`. Record each token request before blocking and release on signal or request cancellation.
- [X] T022 Add US1-S1, US1-S2, and US1-S3 as separate, semantically failing Ginkgo `It()` cases in `tests/e2e/proactive_token_refresh_test.go`. For US1-S1, set one background slot, gate its first due refresh, and exchange a second due and a healthy session. Assert immediate valid tokens, no healthy upstream call, and one WARN `session.oauth2.proactive_refresh_dropped` event with session and service IDs through `bootstrap.NewBufferedJSONLogger`. For US1-S2/S3, assert the old-token response while the provider gate stays closed, eventual stored renewal, and no duplicate call.
- [X] T023 Keep the original US2 cases and extend US2-S3 in `tests/e2e/proactive_token_refresh_test.go`. Use expired access without refresh credentials for US2-S2. Assert one INFO success after commit and one ERROR per on-demand failure with `session_id` and `triggered_by=on-demand`. Force HTTP 400 `invalid_grant` before access expiry. Assert manual 409 `refresh_rejected` and its exact message, unchanged token metadata, and retained list state: reauthentication true, access expired false, refresh present true. The next exchange returns 400 `invalid_grant` plus `/sessions`, no token, and no new provider request. A `PT2H` sweep returns all four counts zero. Complete real authorize/provider/callback redirects to upsert recovery, then require successful exchange and one `authorization_code` grant.
- [X] T024 Keep US3-S1..S4 in `tests/e2e/proactive_token_refresh_test.go`. Preserve page-size-one selection, healthy/unknown-expiry exclusion, repeated missing-credential failures, concurrent replicas, and dry-run zero writes/calls. Extend domain and repository coverage for known rejection exclusion, stale rejection skips, and newly observed rejection failing once. Preserve `refreshed + skipped + failed = total_evaluated`.
- [X] T025 Run all ten focused cases in `tests/e2e/proactive_token_refresh_test.go` before feature implementation. Record a concrete semantic failure for each: US1-S1 drop event, US1-S2/S3 background call, US2-S1..S3 new log fields, and US3-S1..S4 route/response. Keep tests compilable, with spec references and no skips or placeholder assertions.

**Checkpoint**: The ten original scenario identifiers retain executable coverage. US2-S3 also maps to the full HTTP rejection/recovery journey. Rejection races and sweep classification have domain and repository regressions.

## Phase 2.5: Foundational Infrastructure

**Purpose**: Build the configuration and expiring-session read port shared by US1 and US3. Complete Phase 2 first.

- [X] T026 [P] Write compiling, semantically failing defaults, file/environment/CLI-precedence, and validation-bound tests in `internal/config/loader_test.go` and `internal/config/validator_test.go`. Cover `5m`, `10`, `100`, nonpositive lookahead, workers outside `1–20`, page sizes outside `1–1000`, and CLI > environment > file > defaults for all three keys.
- [X] T027 Add `TokenRefreshConfig` and `TokenRefreshSweepConfig` to `internal/ports/config.go`. Define `Config.TokenRefresh` with `mapstructure:"token_refresh"` and the nested key names from `specs/029-proactive-token-refresh/contracts/configuration.md`.
- [X] T028 Register `--token_refresh.lookahead_duration` (Duration), `--token_refresh.background_workers` (Int), and `--token_refresh.sweep.default_page_size` (Int) in `cmd/agentic-identity-broker/root.go`. Bind changed flags and explicit `IDENTITY_BROKER_TOKEN_REFRESH_*` environment variables, and apply defaults in `internal/config/loader.go`. Keep CLI > environment > file > defaults precedence.
- [X] T029 Validate the configuration in `internal/config/validator.go`. Enforce `lookahead_duration > 0`, `background_workers` in `1–20`, and `sweep.default_page_size` in `1–1000`. Explain the 25-connection pool limit in the worker error.
- [X] T030 [P] Define the one-method `UserSessionExpiryRepository` in `internal/ports/storage.go`. Use `ListExpiringSessions(ctx context.Context, threshold time.Time, cursor storage.SessionExpiryCursor, limit int)` and leave `UserSessionRepository` unchanged. Define `SessionExpiryCursor{AccessTokenExpiresAt time.Time, ID id.SessionID}` in `internal/domain/storage/user_session.go`. A zero ID and zero expiry start a scan. An expiry without an ID is invalid. An ID with a zero expiry continues the scan.
- [X] T031 [P] Cover E1–E9 and L1–L4 in `internal/adapters/storage/memory/user_session_test.go`. Preserve expiry/UUID order, cursor boundaries, bounded pages, and encrypted snapshots. Add known rejection exclusion and persisted rejection writes. A callback error must not persist even after snapshot mutation.
- [X] T032 [P] Cover E1–E9, L1–L4, rejection upsert reset, active filters, and token-free summary projection in `internal/adapters/storage/postgres/user_session_test.go`. Preserve normal-planner `EXPLAIN` coverage against migration 037's two-key index. Do not disable sequential scans or change that index design.
- [X] T033 [P] Keep memory keyset paging ordered by access expiry and UUID bytes. Exclude `nil` access expiry and known rejection before selection. Preserve cursor and limit validation and selected-session snapshots. A zero expiry with an ID remains a valid cursor.
- [X] T034 [P] Keep both PostgreSQL access-expiry paging queries and add `NOT refresh_token_rejected`. Preserve non-NULL expiry, threshold, composite cursor, order, timeouts, sqlx reads, wrapping, and limits. Persist `refresh_token_rejected` through reads, upserts, and locked updates without fetching blobs for summaries.
- [X] T035 Add `SessionExpiry() ports.UserSessionExpiryRepository` to `internal/adapters/storage/factory.go`. Return the existing session repository for both memory and PostgreSQL backends.
- [X] T036 Run the configuration, memory, PostgreSQL integration, and domain-model regressions. Confirm E1–E9 on both backends, rejection persistence, and reauthentication semantics. Unknown access expiry must not imply permanent validity. False rejection state means no observed rejection, not proven provider validity.

**Checkpoint**: Both adapters expose the same bounded, update-safe session scan. Configuration rejects invalid worker counts before startup.

## Phase 3: User Story 1 — Zero-Latency Token Exchange for Active Users (P1 MVP)

**Goal**: Serve a valid access token immediately. Start one bounded background refresh per due session without blocking the exchange.

**Independent Test**: Gate the upstream token response. Exchange a token expiring in two minutes and receive the old token before releasing the gate. A later exchange returns the new token without a second upstream call.

### Tests for User Story 1

- [X] T037 [P] [US1] Write semantically failing tests in `internal/domain/oauth2session/background_refresh_test.go`. Cover nonblocking submission, saturation, in-flight dedup, and unchanged caller response. Cover shutdown drain, idempotence, deadline cancellation, deletion, provider errors, and counters.

### Implementation for User Story 1

- [X] T038 [US1] Implement `backgroundRefresher` in `internal/domain/oauth2session/background_refresh.go`. Use `sync.Map.LoadOrStore` before a nonblocking channel-slot acquire. Share `singleflight`, detach from request cancellation, link a new root `oauth2session.background_refresh` span, and drain before cancellation on `Close(ctx)`. Reuse per-call client-secret decryption and the existing encrypted, update-only refresh.
- [X] T039 [US1] Submit only valid, refreshable sessions due within the lookahead from `GetValidAccessToken` in `internal/domain/oauth2session/service.go`. Use `AccessTokenExpiresBy(now+lookahead) && CanRefresh()`. Create `proactive_refresh_triggered_total`, `proactive_refresh_dropped_total`, and `proactive_refresh_failed_total` as `Int64Counter` instruments once per service. Emit a WARN `session.oauth2.proactive_refresh_dropped` event with the owner `principal`, `session_id`, and `service_id` on saturation. Log `session.oauth2.token_refreshed` only after commit with `triggered_by=background`; classify other background failures at WARN or ERROR. Identify the session owner on per-session events, never token material.
- [X] T040 [US1] Map `TokenRefreshConfig` to `oauth2session.Config` in `internal/domain/oauth2session/service.go`. Pass it from `internal/app/builder.go` through `NewConfigFromPorts`, using LSP references to migrate every caller. Add idempotent `OAuth2SessionService.Close(ctx)` to the shutdown chain before transport and telemetry shutdown.
- [X] T041 [US1] Run the US1 cases in `tests/e2e/proactive_token_refresh_test.go` and race-enabled tests in `internal/domain/oauth2session/background_refresh_test.go`. Assert the response precedes the gated refresh, one call serves concurrent exchanges, and no deleted session is recreated.

**Checkpoint**: US1 works with no admin sweep. Background failure or saturation never delays an exchange that already has a valid token.

## Phase 4: User Story 2 — Expired Session Refresh Without User Interruption (P1)

**Goal**: Keep synchronous refresh after access expiry. Persist observed terminal refresh rejection and stop subsequent issuance without deleting the retained session.

**Independent Test**: Expire access while stored refresh credentials have no recorded rejection. A successful provider refresh permits exchange. Missing credentials require reauthentication after access expiry. Recorded rejection requires reauthentication even before access expiry. US2-S3 proves real callback recovery.

### Tests for User Story 2

- [X] T042 [P] [US2] Extend refresh classification and concurrency regressions. Preserve commit-before-success logging for all triggers. Prove forced unexpired-access rejection prevents later acquisition, while 429, 5xx, client errors, malformed responses, and transport errors never persist rejection. Prove callback rollback, rejection commit failure classification, shared-result acquisition guard, and no background follow-up or success event for rejection.

### Implementation for User Story 2

- [X] T043 [US2] Use the shared locked refresh and trigger-aware logs. On terminal HTTP 4xx `invalid_grant`, excluding 429, capture the error and set only rejection and `UpdatedAt`. Commit through callback `(true, nil)`, then return the saved error with `ErrReauthenticationRequired` and `refreshed=false`. Callback errors never persist. Write or commit errors return `persistence_failed` without claiming rejection committed. Reject recorded rejection before acquisition's valid-token branch and after shared results. Already-rejected background/sweep work is a no-op. No rejection emits refresh-success events or increments refresh-success counters.
- [X] T044 [US2] Exercise US2-S1..S3 and session regressions. Assert one upstream refresh, retained ciphertext and access expiry after rejection, immediate later token refusal, and no new provider request. Prove successful OAuth authorize/callback clears rejection through the existing upsert and restores exchange. Keep explicit termination as deletion, not automatic recovery.

**Checkpoint**: US2 remains independently testable. No new on-demand refresh path or second persistence mechanism exists.

## Phase 5: User Story 3 — Admin-Triggered Sweep of Expiring Sessions (P2)

**Goal**: Let an authenticated operator run a synchronous, paged sweep across all principals and services. Report a count-only summary.

**Independent Test**: Seed due, healthy, missing-refresh, rejected, and unknown-access-expiry rows. Page size one selects only eligible due rows. Missing credentials fail repeatedly. New rejection fails once, known rejection is excluded, and stale rejection skips. Preserve the count invariant.

### Tests for User Story 3

- [X] T045 [P] [US3] Write a compiling, semantically failing parser table in `internal/adapters/http/handlers/admin/iso8601_duration_test.go`. Accept `PT5M`, `P1D`, and `PT1.5S`. Reject zero, overflow, `P`, `PT`, `P1Y`, `P1W`, and Go-duration input.
- [X] T046 [P] [US3] Cover defaults, fixed threshold, page traversal, counts, and dry-run behavior in `internal/domain/oauth2session/sweep_test.go`. Include known rejection exclusion, stale rejection skips in real and dry runs, new rejection failing once, and repeated missing-credential failures. Preserve repository aborts, cancellation between sessions, and detached refresh completion.
- [X] T047 [P] [US3] Write a compiling, semantically failing handler contract in `internal/adapters/http/handlers/admin/session_sweep_handler_test.go`. Cover operator identity, optional fields, 4 KiB body cap, unknown fields, 200 on all per-session failures, 400 validation, 500 infrastructure errors, deadline lift, and audit fields.

### Implementation for User Story 3

- [X] T048 [US3] Implement strict `P[nD][T[nH][nM][n[.f]S]]` parsing in `internal/adapters/http/handlers/admin/iso8601_duration.go`. Require a positive duration, reject years/months/weeks and overflow, and leave defaulting to the domain service.
- [X] T049 [US3] Define `SweepRequest` and `SweepResult` in `internal/domain/oauth2session/sweep.go`. Preserve these field constraints verbatim: Lookahead zero value means "use `token_refresh.lookahead_duration`" and "after defaulting: `> 0`". PageSize zero value means "use `token_refresh.sweep.default_page_size`" and "after defaulting: `1 ≤ n ≤ 1000`". DryRun zero value means "real sweep". Preserve `"Refreshed + Skipped + Failed == TotalEvaluated"`.
- [X] T050 [US3] Keep the fixed threshold, composite cursor, and locked re-check in `SessionSweepService.Sweep`. Exclude known rejected rows at the repository. Skip rejected stale candidates, including dry-run snapshots. A new terminal rejection commits and fails once, then later sweeps exclude it. Missing credentials remain failures while access is due. Preserve per-session continuation, repository aborts, cancellation, log levels, encryption context, count invariant, and real-sweep counters. Rejection never increments the refresh-success counter. Dry runs remain lock-free without upstream calls, writes, or metrics.
- [X] T051 [US3] Implement the input-only `SessionSweepHandler` in `internal/adapters/http/handlers/admin/session_sweep_handler.go`. Require an operator principal, cap JSON at 4 KiB, and reject unknown fields. Lift the per-route write deadline and map service errors. Return a count-only 200 and emit an INFO audit log. Never return token material.
- [X] T052 [US3] Add `AdminHandlers.SessionSweep` in `internal/app/handlers.go` and construct it through `internal/app/builder.go`. Register `POST /api/sessions/sweep` only inside the admin `/api` group in `internal/adapters/http/routing/admin.go`, inheriting host, JSON, proxy, and CORS middleware. Do not add an internal scheduler.
- [X] T053 [US3] Exercise US3 scenarios, parser/handler tests, sweep classification, and repository filtering. Assert concurrent replicas do not repeat refresh after renewal or recorded rejection. Dry runs never contact providers. A `PT2H` sweep after manual rejection evaluates zero rows even though the unchanged one-hour access expiry lies within its window.

**Checkpoint**: The admin API works without active exchanges, paginates all due candidates, and returns 200 when the sweep completes even if every candidate fails.

## Phase N: Constitution Compliance & Polish

**Purpose**: Complete the architecture and operator documentation. Then examine every constitution gate and run the project checks.

### Additional Polish

- [X] T054 Update route, shutdown, session-refresh concurrency, row-lock, and operator-sweep sections in `ARCHITECTURE.md`. Add the seven glossary entries from T011.
- [X] T055 Add the three settings, defaults, environment names, CLI flags, provider-lifetime warning, worker limit, and runbook link to `docs/configuration.md`.
- [X] T056 Write `docs/operations/session-sweep.md` for broker administrators with a reference CronJob, `concurrencyPolicy: Forbid`, admin-proxy authentication, caller timeout, and count/metric interpretation. Keep migration recovery in ADR 039. Do not ship a broker-chart CronJob.

### Constitution Compliance Verification: Design Phase

- [X] T057 Verify Principles II/V in `specs/029-proactive-token-refresh/data-model.md`, `adrs/038-proactive-token-refresh.md`, and the `ARCHITECTURE.md` glossary. Confirm the aggregate invariant and all seven terms, including SweepRequest and SessionTokensRefreshed.
- [X] T058 [P] Verify Principle VII in `examples/config/third-party-oauth2.yaml`, `examples/config/README.md`, and `charts/agentic-identity-broker/values.yaml`. Confirm example and Helm defaults agree.
- [X] T059 [P] Verify the approved admin API and its request, response, example, and authentication rules. Coordinate the shared validity cutover with end-user API v2: `requires_reauthentication`, ciphertext-presence `has_refresh_token`, and manual 409 `refresh_rejected`. Existing endpoint paths remain unchanged.
- [X] T060 Verify Principles IX/XIII in amended ADR 039, `specs/029-proactive-token-refresh/research.md`, migration `037`, the direct image and Go runners, `tests/integration/migrations/migrations_test.go`, and all ten red E2E scenarios. Confirm the named non-atomic exception, manual recovery, and semantic red evidence.

### Constitution Compliance Verification: Implementation Phase

- [X] T061 Verify Principles IV/X in `api/admin/openapi.yaml` and `internal/adapters/http/handlers/admin/session_sweep_handler.go`. Check exact response fields, dry-run semantics, 200 per-session failures, 500 infrastructure errors, and admin-only proxy identity.
- [X] T062 [P] Verify Principles I/III in `internal/domain/oauth2session/service.go`, `internal/domain/oauth2session/background_refresh.go`, and `internal/domain/oauth2session/sweep.go`. Check encrypted writes, per-call client-secret decryption, no bypass, correct per-session owner fields, no token leakage, fixed-cardinality counter labels, and drain-before-cancel.
- [X] T063 [P] Verify Principle VII in `internal/ports/config.go`, `internal/config/loader.go`, `internal/config/validator.go`, `cmd/agentic-identity-broker/root.go`, `charts/agentic-identity-broker/values.schema.json`, and `charts/agentic-identity-broker/templates/configmap.yaml`. Assert CLI precedence for all three keys. Render a custom lookahead and reject worker count 50 through schema validation.
- [X] T064 Verify the accepted Principle IX exception in amended ADR 039 and the storage and migration integration tests. Run direct migration UP, DOWN, reapply, invalid-index and dirty-version recovery, and normal-planner `EXPLAIN` assertions for the production query.
- [X] T065 Verify Principles VI/XII in `internal/ports/storage.go`, `internal/app/builder.go`, and `internal/adapters/http/routing/admin.go`. Confirm focused ports, builder-only construction, and route-only registration.
- [X] T066 Verify Principles VIII/XIII in the E2E and session-domain tests. Preserve the original ten scenarios' historical red evidence. Map expanded US2-S3 to the full rejection and callback-recovery journey. Cover known/stale/new rejection, repeated missing credentials, and failure-state commit semantics without skip markers.
- [X] T067 Run `just check` and `just test` from `justfile`. Resolve failures in the changed Go, config, and HTTP packages.
- [X] T068 Run release-scoped storage and migration coverage. Preserve migration 037 recovery and planner assertions. PR1 covers migration 038 data preservation, legacy continuity, true-write refusal, ordinary prepared traffic, and preparation rollback/reapply. PR2 adds 039's durable rejection, refusal, cross-replica, and lock-contention proof. This PR3 release adds the final 040 lifecycle, nullable-column restoration, retained rejection, reapplication, concurrent HTTP continuity, and actual CLI dirty/stale forward-stage guards. All three phases and their coverage are present, not skipped or vacuous cases.
- [X] T069 Run `just test-e2e-backend` from `justfile`. Confirm all ten scenarios in `tests/e2e/proactive_token_refresh_test.go` pass alongside existing exchange journeys.
- [X] T070 Follow the API, observability, and Helm smoke steps in `specs/029-proactive-token-refresh/quickstart.md`. Confirm the default dry-run summary, error codes, linked background span, fixed-cardinality counters, and chart output.

**Checkpoint**: The published API, ADR, docs, migrations, Helm chart, and all acceptance checks agree.

**Sequential release contract**: This cleanup PR adds migration 040 to PR1 #224's migration 038 and prepared application code and PR2 #225's migration 039. All three phases ship in this release. The default/max target is 40 and compatibility defaults false. Fresh installs run direct UP-all through 40. Normal upgrades use the guarded wrapper. Before merging or deploying PR2, complete PR1 across every replica. Drain every unprepared replica and in-flight operation first. Migration 039 activates durable rejection on already-running PR1 brokers without restart. Before merging or deploying PR3, complete PR2 across every replica. End all legacy database dependencies and upgrade the SPA and all external session-API consumers first. Migration 040 removes only the unused legacy column. Normal deployment omits the legacy wire field. Explicit compatibility true remains a deliberate override for legacy consumers, not a waiver of the consumer-upgrade gate.

## Dependencies & Execution Order

### Phase Dependencies

- Phase 0 is a behavior-preserving refactor in this branch. A separate PR is not required by stakeholder decision. Phase 1 checks existing dependencies.
- Phase 2a, 2b, 2c, and 2d can proceed in parallel after Phase 0. Phase 2e records the backend-only scope. Phase 2f needs the test helper before its scenario tests.
- The design preconditions block all feature implementation. The admin endpoint also needs T018 written stakeholder sign-off.
- Phase 2.5 follows all Phase 2 gates. T019 records ADR 039's in-place amendment for direct execution; T020 keeps the concurrent migration pair and informational directive. T026 precedes T027–T029. T030 precedes T031/T032. Adapter tests precede T033/T034, and both implementations precede T035.
- US1, US2, and US3 tests can start after Phase 2.5. US1 is the MVP. US2's baseline behavior is independent, while its new log helper integration follows T039. US3 uses the locked-refresh seam, not the US1 background pool.
- T043 follows T039 in `internal/domain/oauth2session/service.go` to reuse the trigger-aware log helper. T052 and T040 both edit `internal/app/builder.go`. Finish one builder change before the other.
- Phase N follows the desired story increments. Finish its documentation before the matching compliance checks. Run the full checks once after all code changes.

### User Story Dependency Graph

```text
Phase 0 → Phase 1 → Phase 2a–2f → Phase 2.5
Phase 2.5 → US1 tests → US1 complete (P1, MVP)
Phase 2.5 → US2 tests (P1)
US1 T039 + US2 tests → US2 T043 → US2 complete
Phase 2.5 + API sign-off T018 → US3 complete (P2)
US1 + US2 + US3 → Phase N
```

The US3 API work cannot begin without T018. The common foundation supplies the predicate, configuration, expiry port, and locked refresh. All three user journeys have independent tests. US2's log integration follows the US1 shared helper, and builder edits need serial ownership.

### Parallel Execution Examples

- **US1**: After Phase 2.5, work on T037 in `internal/domain/oauth2session/background_refresh_test.go` while a separate owner works on T042 in `internal/domain/oauth2session/service_refresh_concurrency_test.go`. Complete the US1 red test before T038. Do not parallelize T039 with T043.
- **US2**: T042 in `internal/domain/oauth2session/service_refresh_concurrency_test.go` can run beside US3 parser test T045 in `internal/adapters/http/handlers/admin/iso8601_duration_test.go`. Complete its test before T043. Its implementation edits the same service file as US1, so it has no safe same-file parallel implementation task.
- **US3**: T045, T046, and T047 can run in parallel in separate parser, sweep, and handler test files. After T049 defines the service contract, T048 and T050 can proceed in separate files. T051 waits for the parser and service. Keep T052 separate from US1 builder wiring T040.

## Implementation Strategy

### MVP First (US1)

1. Complete the Phase 0 refactor in this branch and the Phase 1 dependency check.
2. Complete all Phase 2 design gates, including a compliant migration design, written admin API approval, and ten semantically red E2E cases.
3. Complete the shared Phase 2.5 foundation.
4. Complete US1 and run its gated-refresh and concurrency checks. This delivers nonblocking token exchange without the admin sweep.
5. Complete US2 logging and the shared validity cutover. Reject known invalid refresh authority before future token issuance.
6. Complete US3 when the operator API is approved. Finish Phase N and run the full gates.

### Incremental Delivery

- Deliver US1 after its independent test passes. The broker still handles expired tokens through its existing synchronous path.
- Deliver US2 with persisted observed rejection, immediate token refusal, and real OAuth callback recovery. Its tests preserve callback rollback and atomic state writes.
- Deliver US3 as an admin-only increment. It scans inactive sessions without adding a scheduler or a distributed coordinator.
- Run the final documentation, migration, Helm, security, and E2E checks before declaring the feature complete.
