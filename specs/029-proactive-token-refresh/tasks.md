---
description: "Task list for proactive token refresh"
---

# Tasks: Proactive Token Refresh

**Input**: `specs/029-proactive-token-refresh/plan.md`, `spec.md`, `research.md`, `data-model.md`, `contracts/`, and `quickstart.md`.

**Tests**: Write one compiling, semantically failing Ginkgo `It()` for each of the ten acceptance scenarios before feature implementation. US1-S1 asserts the new saturation WARN event. US2-S1..S3 retain the baseline token behavior and assert new trigger-aware log fields. Record the red results before implementation. Do not use skips or placeholder assertions.

**Organization**: Phase 0 is a behavior-preserving refactor in this feature branch. The stakeholder waived a separate refactor PR on 2026-10-09. Phase 2 contains required design preconditions and a blocking foundational subphase. Backend only: there is no frontend work or new persisted entity.

## Phase 0: Pre-implementation Refactoring

**Purpose**: Prepare the existing HTTP wrapper and locked-refresh path without adding proactive behavior. Complete its tests before feature implementation.

- [X] T001 Create a compiling, behaviorally failing real-server write-deadline test in `internal/adapters/http/server_test.go`. Send a request through `NewHandler` with a short `WriteTimeout` and a handler that lifts its deadline.
- [X] T002 Add `Unwrap() http.ResponseWriter` to the logging `responseWriter` in `internal/adapters/http/middleware.go`. Make T001 pass without changing existing routes.
- [X] T003 [P] Create a table-driven predicate test in `internal/domain/storage/user_session_test.go`. Include a compiling stub only for the red phase. Assert boundary inclusion, `nil` expiry, and equivalence with `!HasValidAccessToken()`.
- [X] T004 Implement `UserSession.AccessTokenExpiresBy(t)` in `internal/domain/storage/user_session.go`. Preserve the model constraint: "A session without an access-token expiry never expires and is never due (FR-013)."
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

- [X] T009 Write `adrs/038-proactive-token-refresh.md` with Context, Decision, Consequences, and Status: Accepted. Record the bounded drop-on-saturation pool, row-lock correctness, external synchronous sweep, deadline lift, drain-before-cancel shutdown, and rejected alternatives from `specs/029-proactive-token-refresh/research.md` R13.
- [X] T010 Add ADR 038 to the decision index in `AGENTS.md` after the ADR is accepted.
- [X] T011 Add Refresh Lookahead Window, Proactive Refresh, Session Sweep, SweepRequest, SweepResult, RefreshTrigger, and SessionTokensRefreshed with their relationships to the glossary in `ARCHITECTURE.md`. Use `specs/029-proactive-token-refresh/data-model.md`.
- [X] T012 Verify that DB-002, DB-003, the clarification, FR-003, FR-009, the domain diagrams, and `specs/029-proactive-token-refresh/contracts/ports.md` agree on a locked, update-only refresh and the focused expiry port before code changes.

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

**Constitution**: Principle IX. The table has no new columns.

- [X] T019 Obtain maintainer acceptance of the named Principle IX exception in `adrs/039-concurrent-session-expiry-index.md` under the feature-029 scoped same-PR approval. Implement and prove the enforced `-- migrate:no-transaction` guard for both migration image and every Go runner that can apply migration 036. Record the accepted decision and proof in `research.md` and `plan.md`. Do not create migration 036 or start Phase 2.5 before the guard is proven.
- [X] T020 After T019, add migration `036_user_sessions_access_token_expiry_index.{up,down}.sql` in `migrations/`. Test apply, rollback, reapply, invalid-index failure, dirty-version recovery, and real-runner malformed-directive rejection before SQL in `tests/integration/migrations/migrations_test.go`. Review the intentional `Migrate(35)` pin in `tests/integration/storage/infra/thirdparty_service_test.go`. Never claim operator recovery is atomic rollback.

**Checkpoint**: ADR 039 is accepted. Both runner versions enforce the directive and migrate index 036 outside explicit transactions. PostgreSQL lifecycle and invalid-index recovery tests pass. Phase 2.5 may begin; the normal-planner index test remains T032.

### Phase 2e: Frontend/Design System Review

**Constitution**: Principle XI applies only to UI changes. This feature changes no frontend component, so no design-system or Playwright task applies.

### Phase 2f: E2E Acceptance Test Design

**Constitution**: Principle XIII. Use `Describe` → story `Describe` → `Context` → one `It()` per spec scenario. Use production bootstrap and fixtures.

- [X] T021 Add an idempotent `WithTokenResponseGate() (release func())` to `tests/e2e/helpers/mock_upstream.go`. Record each token request before blocking and release on signal or request cancellation.
- [X] T022 Add US1-S1, US1-S2, and US1-S3 as separate, semantically failing Ginkgo `It()` cases in `tests/e2e/proactive_token_refresh_test.go`. For US1-S1, set one background slot, gate its first due refresh, and exchange a second due and a healthy session. Assert immediate valid tokens, no healthy upstream call, and one WARN `session.oauth2.proactive_refresh_dropped` event with session and service IDs through `bootstrap.NewBufferedJSONLogger`. For US1-S2/S3, assert the old-token response while the provider gate stays closed, eventual stored renewal, and no duplicate call.
- [X] T023 Add US2-S1, US2-S2, and US2-S3 as separate, semantically failing Ginkgo `It()` cases in `tests/e2e/proactive_token_refresh_test.go`. Assert existing token results and unchanged ciphertext after provider rejection. With `bootstrap.NewBufferedJSONLogger`, assert that success logs INFO after commit with `session_id` and `triggered_by=on-demand`, and that each failure logs exactly one ERROR `session.oauth2.refresh_failed` event with both fields, including an expired refresh token without an upstream call.
- [X] T024 Add US3-S1, US3-S2, US3-S3, and US3-S4 as separate Ginkgo `It()` cases in `tests/e2e/proactive_token_refresh_test.go`. Assert page-size-one selection, excluded healthy/NULL rows, failed-candidate continuation, shared-storage concurrent replicas, and dry-run zero writes/calls. Check `refreshed + skipped + failed = total_evaluated`.
- [X] T025 Run all ten focused cases in `tests/e2e/proactive_token_refresh_test.go` before feature implementation. Record a concrete semantic failure for each: US1-S1 drop event, US1-S2/S3 background call, US2-S1..S3 new log fields, and US3-S1..S4 route/response. Keep tests compilable, with spec references and no skips or placeholder assertions.

**Checkpoint**: All ten scenarios map one-to-one to executable tests before proactive feature code starts.

## Phase 2.5: Foundational Infrastructure

**Purpose**: Build the configuration and expiring-session read port shared by US1 and US3. Complete Phase 2 first.

- [X] T026 [P] Write compiling, semantically failing defaults, file/environment/CLI-precedence, and validation-bound tests in `internal/config/loader_test.go` and `internal/config/validator_test.go`. Cover `5m`, `10`, `100`, nonpositive lookahead, workers outside `1–20`, page sizes outside `1–1000`, and CLI > environment > file > defaults for all three keys.
- [X] T027 Add `TokenRefreshConfig` and `TokenRefreshSweepConfig` to `internal/ports/config.go`. Define `Config.TokenRefresh` with `mapstructure:"token_refresh"` and the nested key names from `specs/029-proactive-token-refresh/contracts/configuration.md`.
- [X] T028 Register `--token_refresh.lookahead_duration` (Duration), `--token_refresh.background_workers` (Int), and `--token_refresh.sweep.default_page_size` (Int) in `cmd/agentic-identity-broker/root.go`. Bind changed flags and explicit `IDENTITY_BROKER_TOKEN_REFRESH_*` environment variables, and apply defaults in `internal/config/loader.go`. Keep CLI > environment > file > defaults precedence.
- [X] T029 Validate the configuration in `internal/config/validator.go`. Enforce `lookahead_duration > 0`, `background_workers` in `1–20`, and `sweep.default_page_size` in `1–1000`. Explain the 25-connection pool limit in the worker error.
- [X] T030 [P] Define the one-method `UserSessionExpiryRepository` in `internal/ports/storage.go`. Use `ListExpiringSessions(ctx context.Context, threshold time.Time, cursor storage.SessionExpiryCursor, limit int)` and leave `UserSessionRepository` unchanged. Define `SessionExpiryCursor{AccessTokenExpiresAt time.Time, ID id.SessionID}` in `internal/domain/storage/user_session.go`; zero/zero starts a scan, and a partial cursor is invalid.
- [X] T031 [P] Write E1–E7 and L1–L4 locked-update tests in `internal/adapters/storage/memory/user_session_test.go`. Cover expiry-first, UUID-byte tie order, zero/partial cursors, small pages under mutations, and ciphertext snapshots. Add only a compiling method stub to `internal/adapters/storage/memory/user_session.go` for semantic red.
- [X] T032 [P] Write E1–E7 and an `EXPLAIN (FORMAT JSON)` test in `internal/adapters/storage/postgres/user_session_test.go`. Add only a compiling method stub and production-query constants to `internal/adapters/storage/postgres/user_session.go` for semantic red. Seed about 5000 rows with at most 2% due, run `ANALYZE`, and assert the named valid composite index for first, middle, and late small-limit pages under normal planner settings. Do not disable sequential scans.
- [X] T033 [P] Implement keyset `ListExpiringSessions` in `internal/adapters/storage/memory/user_session.go`. Exclude `nil` expiry, order by expiry then UUID bytes, advance past the composite cursor, reject partial cursors and limits outside `1–1000`, and copy only selected sessions. Make T031 green.
- [X] T034 [P] Implement first-page and later-page `ListExpiringSessions` queries in `internal/adapters/storage/postgres/user_session.go`. Filter `access_token_expires_at IS NOT NULL` and `<= threshold`. For later pages require `(access_token_expires_at, id) > (cursor.AccessTokenExpiresAt, cursor.ID)`; order both queries by expiry then ID. Apply the read timeout, sqlx `SelectContext`, domain `StorageError`, and limit validation. Make T032 green.
- [X] T035 Add `SessionExpiry() ports.UserSessionExpiryRepository` to `internal/adapters/storage/factory.go`. Return the existing session repository for both memory and PostgreSQL backends.
- [X] T036 Run the configuration, memory-adapter, PostgreSQL integration, and domain-model tests for `internal/config/validator.go`, `internal/adapters/storage/factory.go`, and `internal/domain/storage/user_session.go`. Confirm the E1–E7 contract passes on both backends.

**Checkpoint**: Both adapters expose the same bounded, update-safe session scan. Configuration rejects invalid worker counts before startup.

## Phase 3: User Story 1 — Zero-Latency Token Exchange for Active Users (P1 MVP)

**Goal**: Serve a valid access token immediately. Start one bounded background refresh per due session without blocking the exchange.

**Independent Test**: Gate the upstream token response. Exchange a token expiring in two minutes and receive the old token before releasing the gate. A later exchange returns the new token without a second upstream call.

### Tests for User Story 1

- [X] T037 [P] [US1] Write semantically failing tests in `internal/domain/oauth2session/background_refresh_test.go`. Cover nonblocking submission, saturation, in-flight dedup, and unchanged caller response. Cover shutdown drain, idempotence, deadline cancellation, deletion, provider errors, and counters.

### Implementation for User Story 1

- [X] T038 [US1] Implement `backgroundRefresher` in `internal/domain/oauth2session/background_refresh.go`. Use `sync.Map.LoadOrStore` before a nonblocking channel-slot acquire. Share `singleflight`, detach from request cancellation, link a new root `oauth2session.background_refresh` span, and drain before cancellation on `Close(ctx)`. Reuse per-call client-secret decryption and the existing encrypted, update-only refresh.
- [X] T039 [US1] Submit only valid, refreshable sessions due within the lookahead from `GetValidAccessToken` in `internal/domain/oauth2session/service.go`. Use `AccessTokenExpiresBy(now+lookahead) && CanRefresh()`. Create `proactive_refresh_triggered_total`, `proactive_refresh_dropped_total`, and `proactive_refresh_failed_total` as `Int64Counter` instruments once per service. Emit a WARN `session.oauth2.proactive_refresh_dropped` event with `session_id` and `service_id` on saturation. Log `session.oauth2.token_refreshed` only after commit with `triggered_by=background`; classify other background failures at WARN or ERROR. Never log principal or tokens.
- [X] T040 [US1] Map `TokenRefreshConfig` to `oauth2session.Config` in `internal/domain/oauth2session/service.go`. Pass it from `internal/app/builder.go` through `NewConfigFromPorts`, using LSP references to migrate every caller. Add idempotent `OAuth2SessionService.Close(ctx)` to the shutdown chain before transport and telemetry shutdown.
- [X] T041 [US1] Run the US1 cases in `tests/e2e/proactive_token_refresh_test.go` and race-enabled tests in `internal/domain/oauth2session/background_refresh_test.go`. Assert the response precedes the gated refresh, one call serves concurrent exchanges, and no deleted session is recreated.

**Checkpoint**: US1 works with no admin sweep. Background failure or saturation never delays an exchange that already has a valid token.

## Phase 4: User Story 2 — Expired Session Refresh Without User Interruption (P1)

**Goal**: Keep the existing synchronous expired-token refresh. Report re-authentication failures clearly and preserve the stored row on provider failure.

**Independent Test**: Expire the stored access token while its refresh token remains valid. One exchange returns a new token. Absent or rejected refresh tokens return `invalid_grant` without a partial write.

### Tests for User Story 2

- [X] T042 [P] [US2] Extend `internal/domain/oauth2session/service_refresh_concurrency_test.go` with commit-before-success-log checks for `on-demand`, `background`, and `sweep`. Assert on-demand failure classification, no duplicate log, and no partial ciphertext update.

### Implementation for User Story 2

- [X] T043 [US2] Apply shared trigger-aware success/failure log helpers to the on-demand path and `ForceRefreshSession` in `internal/domain/oauth2session/service.go`. Emit one ERROR `session.oauth2.refresh_failed` event with `session_id` and `triggered_by=on-demand` for missing/expired refresh tokens and provider errors, not only from `refreshSessionTokens`. Remove the old failure log to prevent duplicates. Emit success only after commit.
- [X] T044 [US2] Run US2-S1..S3 in `tests/e2e/proactive_token_refresh_test.go` and the tests in `internal/domain/oauth2session/service_refresh_concurrency_test.go`. Confirm the existing sync refresh still calls the provider once and leaves rejected sessions unchanged.

**Checkpoint**: US2 remains independently testable. No new on-demand refresh path or second persistence mechanism exists.

## Phase 5: User Story 3 — Admin-Triggered Sweep of Expiring Sessions (P2)

**Goal**: Let an authenticated operator run a synchronous, paged sweep across all principals and services. Report a count-only summary.

**Independent Test**: Seed due, healthy, unrefreshable, and NULL-expiry sessions. Call the admin endpoint with page size one. Only due rows change. Check `refreshed + skipped + failed = total_evaluated`.

### Tests for User Story 3

- [X] T045 [P] [US3] Write a compiling, semantically failing parser table in `internal/adapters/http/handlers/admin/iso8601_duration_test.go`. Accept `PT5M`, `P1D`, and `PT1.5S`. Reject zero, overflow, `P`, `PT`, `P1Y`, `P1W`, and Go-duration input.
- [X] T046 [P] [US3] Write a compiling, semantically failing classification table in `internal/domain/oauth2session/sweep_test.go`. Cover defaults, fixed threshold, page traversal, counts, and dry-run behavior. Cover per-session failures, repository aborts, cancellation between sessions, and detached refresh completion.
- [X] T047 [P] [US3] Write a compiling, semantically failing handler contract in `internal/adapters/http/handlers/admin/session_sweep_handler_test.go`. Cover operator identity, optional fields, 4 KiB body cap, unknown fields, 200 on all per-session failures, 400 validation, 500 infrastructure errors, deadline lift, and audit fields.

### Implementation for User Story 3

- [X] T048 [US3] Implement strict `P[nD][T[nH][nM][n[.f]S]]` parsing in `internal/adapters/http/handlers/admin/iso8601_duration.go`. Require a positive duration, reject years/months/weeks and overflow, and leave defaulting to the domain service.
- [X] T049 [US3] Define `SweepRequest` and `SweepResult` in `internal/domain/oauth2session/sweep.go`. Preserve these field constraints verbatim: Lookahead zero value means "use `token_refresh.lookahead_duration`" and "after defaulting: `> 0`". PageSize zero value means "use `token_refresh.sweep.default_page_size`" and "after defaulting: `1 ≤ n ≤ 1000`". DryRun zero value means "real sweep". Preserve `"Refreshed + Skipped + Failed == TotalEvaluated"`.
- [X] T050 [US3] Implement `SessionSweepService.Sweep` in `internal/domain/oauth2session/sweep.go`. Fix `threshold = sweepStart + lookahead`, page by increasing `(access_token_expires_at, id)`, and re-check under `WithLockedSession`. Count missing refresh tokens and provider errors as failures, but continue. Count deleted/already-refreshed rows as skipped. Abort on repository failures or cancellation between sessions. Classify provider failures at WARN and encryption failures at ERROR. Encrypt with the existing `service_id` context before update. Emit `session_sweep_refreshed_total` and `session_sweep_failed_total` with only `triggered_by=sweep` for real sweeps. Keep dry runs lock-free without upstream calls or metrics.
- [X] T051 [US3] Implement the input-only `SessionSweepHandler` in `internal/adapters/http/handlers/admin/session_sweep_handler.go`. Require an operator principal, cap JSON at 4 KiB, and reject unknown fields. Lift the per-route write deadline and map service errors. Return a count-only 200 and emit an INFO audit log. Never return token material.
- [X] T052 [US3] Add `AdminHandlers.SessionSweep` in `internal/app/handlers.go` and construct it through `internal/app/builder.go`. Register `POST /api/sessions/sweep` only inside the admin `/api` group in `internal/adapters/http/routing/admin.go`, inheriting host, JSON, proxy, and CORS middleware. Do not add an internal scheduler.
- [X] T053 [US3] Run US3-S1..S4 in `tests/e2e/proactive_token_refresh_test.go`, the parser/handler tests in `internal/adapters/http/handlers/admin/`, and the sweep tests in `internal/domain/oauth2session/sweep_test.go`. Assert concurrent replicas persist consistent tokens and dry run never contacts providers.

**Checkpoint**: The admin API works without active exchanges, paginates all due candidates, and returns 200 when the sweep completes even if every candidate fails.

## Phase N: Constitution Compliance & Polish

**Purpose**: Complete the architecture and operator documentation. Then examine every constitution gate and run the project checks.

### Additional Polish

- [X] T054 Update route, shutdown, session-refresh concurrency, row-lock, and operator-sweep sections in `ARCHITECTURE.md`. Add the seven glossary entries from T011.
- [X] T055 Add the three settings, defaults, environment names, CLI flags, provider-lifetime warning, worker limit, and runbook link to `docs/configuration.md`.
- [X] T056 Write `docs/operations/session-sweep.md` with an operator-managed CronJob example, `concurrencyPolicy: Forbid`, admin-proxy authentication, caller timeout, count/metric interpretation, and the accepted ADR 039 failure procedure after T019. State that invalid-index cleanup is not atomic rollback. Do not ship a broker-chart CronJob.

### Constitution Compliance Verification: Design Phase

- [X] T057 Verify Principles II/V in `specs/029-proactive-token-refresh/data-model.md`, `adrs/038-proactive-token-refresh.md`, and the `ARCHITECTURE.md` glossary. Confirm the aggregate invariant and all seven terms, including SweepRequest and SessionTokensRefreshed.
- [X] T058 [P] Verify Principle VII in `examples/config/third-party-oauth2.yaml`, `examples/config/README.md`, and `charts/agentic-identity-broker/values.yaml`. Confirm example and Helm defaults agree.
- [X] T059 [P] Verify Principles IV/X in `api/admin/openapi.yaml` and `specs/029-proactive-token-refresh/plan.md`. Check written stakeholder approval of the restricted ISO 8601 grammar and every documented request, response, example, and authentication rule. No end-user API changes apply.
- [X] T060 Verify Principles IX/XIII in ADR 039, `specs/029-proactive-token-refresh/research.md`, migration 036, the guarded migration paths, `tests/integration/migrations/migrations_test.go`, and all ten red E2E scenarios. Confirm the accepted exception, non-atomic recovery evidence, and semantic red results before implementation.

### Constitution Compliance Verification: Implementation Phase

- [X] T061 Verify Principles IV/X in `api/admin/openapi.yaml` and `internal/adapters/http/handlers/admin/session_sweep_handler.go`. Check exact response fields, dry-run semantics, 200 per-session failures, 500 infrastructure errors, and admin-only proxy identity.
- [X] T062 [P] Verify Principles I/III in `internal/domain/oauth2session/service.go`, `internal/domain/oauth2session/background_refresh.go`, and `internal/domain/oauth2session/sweep.go`. Check encrypted writes, per-call client-secret decryption, no bypass, no token/principal leakage, counter labels, and drain-before-cancel.
- [X] T063 [P] Verify Principle VII in `internal/ports/config.go`, `internal/config/loader.go`, `internal/config/validator.go`, `cmd/agentic-identity-broker/root.go`, `charts/agentic-identity-broker/values.schema.json`, and `charts/agentic-identity-broker/templates/configmap.yaml`. Assert CLI precedence for all three keys. Render a custom lookahead and reject worker count 50 through schema validation.
- [X] T064 Verify the accepted Principle IX exception in ADR 039 and the storage and migration integration tests. Run guarded migration apply/rollback/reapply, invalid-index and dirty-version recovery, directive rejection, and the normal-planner `EXPLAIN` assertion for the production query.
- [X] T065 Verify Principles VI/XII in `internal/ports/storage.go`, `internal/app/builder.go`, and `internal/adapters/http/routing/admin.go`. Confirm focused ports, builder-only construction, and route-only registration.
- [X] T066 Verify Principles VIII/XIII in `tests/e2e/proactive_token_refresh_test.go`, `internal/domain/oauth2session/background_refresh_test.go`, and `internal/domain/oauth2session/sweep_test.go`. Confirm recorded semantic red results for all ten acceptance scenarios, one `It()` per scenario, meaningful assertions, minimal fixture edits, and no skip markers.
- [X] T067 Run `just check` and `just test` from `justfile`. Resolve failures in the changed Go, config, and HTTP packages.
- [X] T068 Run `just test-integration-infra` from `justfile`. Confirm migration 036, E1–E7, and index-planner assertions against PostgreSQL.
- [X] T069 Run `just test-e2e-backend` from `justfile`. Confirm all ten scenarios in `tests/e2e/proactive_token_refresh_test.go` pass alongside existing exchange journeys.
- [X] T070 Follow the API, observability, and Helm smoke steps in `specs/029-proactive-token-refresh/quickstart.md`. Confirm the default dry-run summary, error codes, linked background span, fixed-cardinality counters, and chart output.

**Checkpoint**: The published API, ADR, docs, migrations, Helm chart, and all acceptance checks agree.

## Dependencies & Execution Order

### Phase Dependencies

- Phase 0 is a behavior-preserving refactor in this branch. A separate PR is not required by stakeholder decision. Phase 1 checks existing dependencies.
- Phase 2a, 2b, 2c, and 2d can proceed in parallel after Phase 0. Phase 2e records the backend-only scope. Phase 2f needs the test helper before its scenario tests.
- The design preconditions block all feature implementation. The admin endpoint also needs T018 written stakeholder sign-off.
- Phase 2.5 follows all Phase 2 gates. T019 requires accepted ADR 039 and enforced directive coverage for the migration image and every Go migration path; T020 follows it. T026 precedes T027–T029. T030 precedes T031/T032. Adapter tests precede T033/T034, and both implementations precede T035.
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
5. Complete US2 logging without changing its existing expired-token behavior.
6. Complete US3 when the operator API is approved. Finish Phase N and run the full gates.

### Incremental Delivery

- Deliver US1 after its independent test passes. The broker still handles expired tokens through its existing synchronous path.
- Deliver US2 as a regression and observability increment. Its test confirms re-authentication and atomic updates.
- Deliver US3 as an admin-only increment. It scans inactive sessions without adding a scheduler or a distributed coordinator.
- Run the final documentation, migration, Helm, security, and E2E checks before declaring the feature complete.
