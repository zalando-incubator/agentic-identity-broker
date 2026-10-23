---

description: "Task list for feature 053: automatic third-party session refresh"
---

# Tasks: Automatic Third-Party Session Refresh

**Input**: Design documents from `/specs/053-session-auto-refresh/`

**Prerequisites**: [plan.md](plan.md), [spec.md](spec.md), [research.md](research.md), [data-model.md](data-model.md), [contracts/](contracts/), [quickstart.md](quickstart.md)

**Tests**: Per Constitution Principle VIII, tests are mandatory and come before implementation. Every acceptance scenario has one E2E `It()` (Principle XIII).

**Organization**: Grouped by user story.
- Phase 0 (refactoring) and Phase 2.7 (entity boilerplate) are skipped. The plan records why.
- The only compile-time scaffolding before the red E2E tests is the config field in T006, because E2E tests build `*ports.Config` as a struct.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Can run in parallel (different files, no dependency on an incomplete task)
- **[Story]**: US1, US2, US3 from spec.md

## 🔒 Feature Scope

- Every task cites a requirement, scenario, or binding constraint.
- Deferred, not tasks:
  - the migration Job's selector labels (research R16)
  - broker-side admin authentication for service callers (spec Assumptions)

  Neither has an approved issue URL, so there is no Future Work entry.
- No task may name company-internal systems (spec Input).

---

## Phase 1: Setup

**Purpose**: Tooling the E2E suites need.

- [X] T001 Removed. The chart render checks live in `tests/integration/helm_chart_test.go`, which skips when `helm` is absent, so no CI job gains a Helm step (research R14).
- [X] T002 Removed with T001: no E2E chart render helper.
- [X] T003 Removed with T001: no script runner; run-time script behavior is verified manually (quickstart §5).

---

## 🔒 Phase 2: Design Preconditions (Blocking Prerequisites)

**⚠️ CRITICAL**: No feature implementation before this phase completes.

### Phase 2a: Domain Model & Glossary

- [X] T004 [P] Add Session Health State, Session Health Snapshot, and Session Sweep Schedule to the "Third-Party OAuth2 Session Management" glossary in `ARCHITECTURE.md` (starts at line 1502). Use the definitions in `specs/053-session-auto-refresh/data-model.md` (Principle V).
- [X] T005 Verify that these agree before code changes:
  - spec FR-018
  - `data-model.md`: the classification flowchart and the `AccessExpiryClass` table, where "at or before `threshold`" is inclusive
  - `contracts/ports.md` §1 and §2

  They must agree on the classes `unknown` → `after_threshold` → `due` → `expired` (an expiry equal to `now` is expired), on `threshold = now + token_refresh.lookahead_duration`, and on "refresh usable = present and expiry nil or after now". Record any correction in `spec.md` (Principle II).

### Phase 2b: Configuration Design

- [X] T006 Add `TokenRefreshHealthMetricsConfig{Enabled bool "enabled"; QueryTimeout time.Duration "query_timeout"}` and the field `TokenRefreshConfig.HealthMetrics` (`mapstructure:"health_metrics"`) in `internal/ports/config.go`, exactly as in `contracts/configuration.md`. This is the contract type only. Defaults, bindings, and validation come in T019 and T020.
- [X] T007 [P] Add the `token_refresh.health_metrics` block with `enabled: true` and `query_timeout: 5s` to `examples/config/telemetry.yaml` and `examples/config/config.production.yaml`, with comments from `contracts/configuration.md` (Principle VII).
- [X] T008 Reference the new keys in the telemetry and production entries of `examples/config/README.md` (Principle VII).
- [X] T009 [P] Add `broker.tokenRefresh.healthMetrics.{enabled: true, queryTimeout: 5s}` to `charts/agentic-identity-broker/values.yaml`. Add it to the strict `tokenRefresh` object in `charts/agentic-identity-broker/values.schema.json` (`:1173`), with `enabled` boolean and `queryTimeout` using the existing Go duration pattern (`:1180`). Render `token_refresh.health_metrics.enabled` and `query_timeout` in `charts/agentic-identity-broker/templates/configmap.yaml` (`:81-86`). Add both rows to the README parameter table (`README.md:179-181`) (Principle VII, Helm contract).
- [X] T010 [P] Add the `sessionSweep` block to `charts/agentic-identity-broker/values.yaml`, with every key, default, and `# --` comment from `contracts/chart-values.md`. Add the strict `sessionSweep` object to `values.schema.json` with every rule in the "Schema rules" table:
  - `additionalProperties: false` at every level
  - integers `minimum: 1`
  - `pageSize` `null` or `1..1000`
  - the header-name pattern
  - `headersFromFile` entries `required: [path]`
  - `pullPolicy` enum
  - the `lookaheadDuration` `allOf` from research R5: `^P([0-9]+D)?(T([0-9]+H)?([0-9]+M)?([0-9]+(\.[0-9]+)?S)?)?$`, `not: {pattern: "T$"}`, `pattern: "[1-9]"`

  (FR-003, FR-004, FR-005)

### Phase 2c: API Design

- [X] T011 Confirm in `plan.md` that the feature adds no endpoint or field (API-001) and that only descriptive text and examples change (API-002, T046). No stakeholder API sign-off is needed (Principle X).

### Phase 2d: Database Design

- [X] T012 Confirm in `plan.md` that there is no schema change or migration (DB-001), and that the health query reads only `service_id`, `access_token_expires_at`, `octet_length(encrypted_refresh_token)`, and `refresh_token_expires_at` (DB-002, DB-003) (Principle IX).

### Phase 2e: Frontend/Design System Review

- [X] T013 Record "N/A, no UI change" in `plan.md` (Principle XI).

### Phase 2f: E2E Acceptance Test Design

- [X] T013a Add `tests/e2e/fixtures/session_health.go` with the shared test data for T014 to T016: session builders for each health state (`ValidSession`, `ExpiringRefreshableSession`, `ExpiredUnrefreshableSession`, `UnknownExpirySession`, each taking a service ID and `now`) plus `HealthJourneySessions(now)` reproducing spec Journey 2's eight sessions (Principle XIII: test data from `tests/e2e/fixtures/`).

- [X] T014 [P] [US1] Add `TestHelmTemplate_SessionSweep` to `tests/integration/helm_chart_test.go`, one subtest per render-time scenario with a `// US1-S<n> from specs/053-session-auto-refresh/spec.md` comment:
  - **US1-S1**: a render with `enabled: false` plus a non-default `schedule` and `headers` contains no `CronJob` and nothing named `session-sweep`.
  - **US1-S2**: `schedule: "0 * * * *"`, `concurrencyPolicy: Forbid`, `backoffLimit: 0`, `activeDeadlineSeconds: 2100`; the curl configuration contains `url = "http://broker-agentic-identity-broker.identity.svc:14000/api/sessions/sweep"`, `header = "Host: broker-admin.example.test"`, the literal operator header, the PT2H body, and `max-time = 1800`.
  - **US1-S5** (render-time part): a `headersFromFile` entry renders `variable = "header_0@<path>"` and `expand-header = "<Name>: <prefix>{{header_0:trim:json}}"`, and the script never reads the file itself.
  - **US1-S7, US1-S11**: renders without an identity, with `timeoutSeconds >= activeDeadlineSeconds`, with `tls.certFile` alone, and with `lookaheadDuration: 2h` fail with the messages from `contracts/chart-values.md` (the last through the values schema).
  - **US1-S10**: the pod template carries `app.kubernetes.io/component: session-sweep` and no `app.kubernetes.io/name`.
  - US1-S3, S4, S6, S8, S9 (run-time behavior of the rendered script) are verified manually per quickstart §5.

  (FR-001 to FR-016, SR-002 to SR-005)
- [X] T015 [P] [US2] Write `tests/e2e/session_metrics_test.go` with US2-S1 to US2-S9 as separate `It()` blocks with spec comments.
  - **Setup**: Install `sdkmetric.NewManualReader()` as the global MeterProvider in `BeforeEach` with `DeferCleanup` before `BuildApp`, as in `tests/e2e/approval_api_test.go:136-143`. Set `config.TokenRefresh.LookaheadDuration = 5*time.Minute`, `config.TokenRefresh.HealthMetrics = {Enabled: true, QueryTimeout: 2*time.Second}`. Seed sessions with `fixtures.HealthJourneySessions(now)` and the per-state builders from T013a. Collect and filter `oauth2_sessions`.
  - **US2-S1**: one point per `(service_id, expiry, refreshable)`, and the per-service sum equals the seeded count.
  - **US2-S2**: zero-valued points are present.
  - **US2-S3 to US2-S6**: the expected `(expiry, refreshable)` series per seeded session shape. S4 seeds one session due within the lookahead and one already expired, both with a usable refresh token, and expects `expiring/true` and `expired/true`. S5 covers both a missing refresh token and an expired refresh expiry.
  - **US2-S7**: Use `bootstrap.NewPostgresFixture` storage. Hold `LOCK TABLE user_sessions IN ACCESS EXCLUSIVE MODE` in a separate open transaction. Collect: no `oauth2_sessions` points, and one WARN `session health query failed` with `error_kind` through `bootstrap.NewBufferedJSONLogger`. Release the lock and collect again: points return.
  - **US2-S8**: Postgres fixture, `HealthMetrics.Enabled = false`, the lock held. Collect: completes within 1s, no `oauth2_sessions`, no WARN. This proves no query was issued.
  - **US2-S9**: every point's attribute set has exactly the keys `service_id`, `expiry`, and `refreshable`.

  (FR-017 to FR-022, SR-007)
- [X] T016 [P] [US3] Add US3-S1 to US3-S3 to `tests/e2e/session_metrics_test.go`, with the same reader.
  - **US3-S1**: A real sweep plus a dry run via `POST /api/sessions/sweep` with `fixtures.AdminPrincipal()`. `session_sweep_duration_seconds` has a histogram data point with count 1 for `{outcome=completed, dry_run=false}` and 1 for `{outcome=completed, dry_run=true}`.
  - **US3-S2**: an unknown-field body gives `rejected`. A canceled client context gives `canceled`. A storage failure gives `aborted`: use the Postgres fixture with the table lock and `storage.timeouts.read: 1s`. None adds a `completed` count. A request without a principal adds nothing.
  - **US3-S3**: One candidate is refreshed by token exchange before the sweep evaluates it, and one is deleted. `session_sweep_skipped_total{triggered_by=sweep}` increases by 2.

  (FR-023, FR-024)
- [X] T017 Run `go test ./tests/integration/ -run TestHelmTemplate_SessionSweep` and `ginkgo -v --focus-file session_metrics_test.go ./tests/e2e/`. Record one concrete semantic failure per scenario under "Red phase results" in `plan.md`, for example "US1-S2: no CronJob document" or "US2-S1: no oauth2_sessions metric". Confirm the E2E file compiles and contains no `Skip`, `XIt`, `PIt`, placeholder assertions, or red-phase comments (Principle XIII).
- [X] T018 Review spec Journeys 1 to 4 against US1-S3, S5, S7, S8, S9 and US2-S1 to S7, and against `contracts/sweep-job.md`. Check the values, rendered CronJob excerpt, HTTP exchanges, log lines, and metric names. Fix mismatches in `spec.md`. No Wireloom or JWT claims apply (Principle XIV).

**Checkpoint**: Glossary, config contract, chart values and schema, red chart render checks, and red metrics E2E tests.

---

## Phase 2.5: Foundational Infrastructure

**Purpose**: Config behavior and the health query port shared by US2. US1 does not depend on this phase.

- [X] T019 [P] Write failing tests in `internal/config/loader_test.go` and `internal/config/validator_test.go`:
  - defaults `token_refresh.health_metrics.enabled = true` and `query_timeout = 5s`
  - `IDENTITY_BROKER_TOKEN_REFRESH_HEALTH_METRICS_ENABLED` and `IDENTITY_BROKER_TOKEN_REFRESH_HEALTH_METRICS_QUERY_TIMEOUT`
  - flags `--token_refresh.health_metrics.enabled` and `--token_refresh.health_metrics.query_timeout`, with CLI > env > file > defaults precedence
  - a validation error for `query_timeout <= 0` with field `token_refresh.health_metrics.query_timeout` and expected "greater than 0"

  (contracts/configuration.md)
- [X] T020 Implement the config behavior:
  - `SetDefault` and `BindEnv` at `internal/config/loader.go:210-215`
  - default key list entries at `loader.go:304`
  - changed-flag copying in `bindFlags` (`loader.go:739-752`)
  - `PersistentFlags().Bool` and `.Duration` with "(default: …)" help in `cmd/agentic-identity-broker/root.go:453-455`
  - the `query_timeout` check in `validateTokenRefreshConfig` (`internal/config/validator.go:688-699`) via `formatValidationError`

  Makes T019 green.
- [X] T021 Add `AccessExpiryClass` and `SessionExpiryGroup{ServiceID id.ServiceID; AccessExpiry AccessExpiryClass; RefreshUsable bool; Count int}` in `internal/domain/storage/user_session.go`.
  - `AccessExpiryClass` has three values: `AccessExpiryUnknown`, `AccessExpiryAfterThreshold`, and `AccessExpiryDue`.
  - Doc comments state the data-model constraints verbatim: "expiry is at or before `threshold` (inclusive, same as `AccessTokenExpiresBy`)", "Refresh token present and its expiry is nil or after `now` (same as `CanRefresh`)", "Number of sessions in this group, `> 0`", and "Each `(ServiceID, AccessExpiry, RefreshUsable)` appears at most once per result."

  (DB-002)
- [X] T022 Add `UserSessionHealthRepository` with `CountSessionsByExpiry(ctx context.Context, threshold, now time.Time) ([]storage.SessionExpiryGroup, error)` to `internal/ports/storage.go`. Use the doc comment from `contracts/ports.md` §1 (DB-002, DB-003, Principle IX ISP).
- [X] T023 [P] Write memory parity tests H1 to H9 from `contracts/ports.md` as `TestUserSessionCountSessionsByExpiry` in `internal/adapters/storage/memory/user_session_test.go`. Use a fixed `now` and `threshold` (DB-002).
- [X] T024 [P] Write PostgreSQL parity tests H1 to H9 in `internal/adapters/storage/postgres/user_session_test.go` (`//go:build integration`), using `setupMigratedAdapter` and `insertTestService` (DB-002, Principle IX).
- [X] T025 Implement `CountSessionsByExpiry` in `internal/adapters/storage/memory/user_session.go`:
  - `RLock`
  - the same predicates with the supplied `threshold` and `now`
  - `ErrorKindTimeout` on `ctx.Err()`
  - the compile-time assertion `var _ ports.UserSessionHealthRepository`

  Makes T023 green.
- [X] T026 Implement `CountSessionsByExpiry` in `internal/adapters/storage/postgres/user_session.go`:
  - the `GROUP BY` query from research R9, with `$1 = threshold` and `$2 = now`
  - `SelectContext` under the caller's context, which `token_refresh.health_metrics.query_timeout` bounds (no `timeouts.Read`)
  - `wrapError`
  - an unknown `access_expiry` label returns an `ErrorKindUnknown` storage error
  - the compile-time assertion

  Makes T024 green with `just test-integration-infra`.
- [X] T027 Add `SessionHealth() ports.UserSessionHealthRepository` to `storage.Adapter` in `internal/adapters/storage/factory.go`, wired for the memory (`:98`) and PostgreSQL (`:134`) backends (Principle IX).

**Checkpoint**: Config and health port complete on both backends.

---

## Phase 3: User Story 1 - Scheduled Session Sweep From the Chart (Priority: P1) 🎯 MVP

**Goal**: An operator enables an hourly, hardened sweep CronJob that carries operator-supplied credentials.

**Independent Test**: T014 passes. `just helm-lint` passes. The disabled render is unchanged (quickstart step 3).

### Tests for User Story 1

T014 (E2E) is the test for this story. It must be red before T028.

### Implementation for User Story 1

- [X] T028 [US1] Add these helpers to `charts/agentic-identity-broker/templates/_helpers.tpl`:
  - `agentic-identity-broker.sessionSweep.validate`: the three `fail` checks with the exact messages in `contracts/chart-values.md` (FR-011, FR-012, SR-002)
  - `.podLabels`: instance, part-of, managed-by, then `commonLabels`, then `sessionSweep.podLabels`, then a fixed `app.kubernetes.io/component: session-sweep`, never `app.kubernetes.io/name` (research R6, FR-016)
  - `.url` and `.host`: the target table in `contracts/sweep-job.md`, using `urlParse` and the `<fullname>.<namespace>.svc:<service.ports.admin>` default (research R3, FR-006)
  - `.body`: key order `lookahead_duration`, `dry_run`, optional `page_size` (FR-004, FR-005)
  - `.curlConfigLiteral`: escape `\` and `"`, sort header names (research R2, FR-007)
- [X] T029 [P] [US1] Create `charts/agentic-identity-broker/templates/serviceaccount-session-sweep.yaml`:
  - gated on `sessionSweep.enabled`
  - name `<fullname>-session-sweep`, the chart namespace helper
  - `agentic-identity-broker.labels` plus `app.kubernetes.io/component: session-sweep`
  - `automountServiceAccountToken: false`
  - no hook annotations

  (FR-015)
- [X] T030 [US1] Create `charts/agentic-identity-broker/templates/cronjob-session-sweep.yaml` per `contracts/sweep-job.md` "CronJob (fields fixed by the chart)":
  - call `.validate` first
  - `concurrencyPolicy: Forbid`, `backoffLimit: 0`, `restartPolicy: Never`
  - `timeZone` only when set
  - `imagePullSecrets`, `podSecurityContext`, and `securityContext` from values
  - `automountServiceAccountToken: false`
  - the `tmp` emptyDir at `/tmp` plus `sessionSweep.volumes` and `volumeMounts`
  - `podAnnotations`

  (FR-002, FR-003, FR-014, FR-015, SR-004)
- [X] T031 [US1] Render the container `command: ["/bin/sh", "-ec", <script>]` in `cronjob-session-sweep.yaml`, implementing `contracts/sweep-job.md` "Script contract" steps 1 to 5:
  - read file headers in sorted order, with the missing/empty and unsupported-character messages
  - generate curl config lines and pipe them to `curl --config -` with `--silent --show-error --connect-timeout 10 --max-time <timeoutSeconds> --request POST --data '<body>' --write-out '\n%{http_code}'`, plus TLS flags when set
  - print the body; exit `0` only for `2xx`, otherwise print `session sweep failed: HTTP <status>` and exit `1`
  - no heredocs, `set -x`, `--verbose`, or `--trace`

  (FR-008, FR-009, FR-012, FR-013, SR-003, SR-005; research R1, R2, R7, R8)
- [X] T032 [US1] Run `docker run --rm --entrypoint /bin/sh curlimages/curl:8.16.0 -c 'type printf; curl --version | head -1'` and record the output in `research.md` R7. If `printf` is not a builtin, stop and propose an alternative that keeps file values out of argv before continuing (SR-003).
- [X] T033 [US1] Add a "Scheduled session sweep" section to `charts/agentic-identity-broker/README.md`:
  - the values table from `contracts/chart-values.md`, with the warning that literal headers are rendered into the manifest
  - two recipes, both through the authenticating proxy: a projected ServiceAccount token (spec Journey 1) and a TLS client certificate. Document `headers` only as a mechanism for non-secret headers. Do not add a direct-call recipe (spec Architecture Decisions)
  - the trust guidance from FR-025
  - a recommendation to give the job a dedicated operator principal, so the sweep audit event distinguishes scheduled sweeps from manual ones (SR-006)
  - `helm template --show-only templates/cronjob-session-sweep.yaml`

  Replace the line 40 instruction to route sweep CronJobs through the proxy (FR-026).
- [X] T034 [US1] Run `just helm-lint` and the quickstart step 3 checks, including the disabled-values diff (FR-001). Run `go test ./tests/integration/ -run TestHelmTemplate_SessionSweep` until the render checks are green, then run the rendered script manually per quickstart §5.

**Checkpoint**: US1 is shippable alone: the sweep runs hourly from the chart.

---

## Phase 4: User Story 2 - Session Health Visible Per Service (Priority: P1)

**Goal**: The `oauth2_sessions{service_id,expiry,refreshable}` gauge for all services and users.

**Independent Test**: T015 passes. Unit tests in `health_metrics_test.go` pass.

### Tests for User Story 2

- [X] T035 [US2] Write failing unit tests in `internal/domain/oauth2session/health_metrics_test.go`, using the ManualReader pattern from `sweep_test.go:262-266` and a hand-written port fake with function fields:
  - `ClassifySessionHealth` table for all eight `(AccessExpiry, RefreshUsable)` shapes plus an invalid class
  - zero-fill of all eight `(expiry, refreshable)` series per present service; absent services produce no points
  - exact attribute set `{service_id, expiry, refreshable}`
  - repo error gives no points, one WARN `session health query failed` with `error_kind`, and a callback that returns `nil`
  - a blocking fake returns `context.DeadlineExceeded` after `queryTimeout`
  - `Close` stops further queries and is idempotent
  - injected `now` and `threshold = now + lookahead` are passed to the repo

  (FR-017 to FR-022)

### Implementation for User Story 2

- [X] T036 [US2] Implement `internal/domain/oauth2session/health_metrics.go` per `contracts/ports.md` §2:
  - `SessionHealthExpiry` constants and `SessionHealthKey`
  - `ClassifySessionHealth`
  - `NewSessionHealthMetrics(repo, lookahead, queryTimeout, logger)`, which registers `Int64ObservableGauge("oauth2_sessions", unit "{session}")` and one callback on `otel.Meter("oauth2session")`, with the callback steps 1 to 4
  - `Close()` via `Registration.Unregister`

  Makes T035 green.
- [X] T037 [US2] Wire `NewSessionHealthMetrics(b.storage.SessionHealth(), b.config.TokenRefresh.LookaheadDuration, b.config.TokenRefresh.HealthMetrics.QueryTimeout, b.logger)` in `internal/app/builder.go`:
  - only when `HealthMetrics.Enabled` (research R11)
  - add `Close` to the shutdown chain beside `OAuth2SessionService.Close` (`:1185-1193`), before telemetry shutdown
  - call `Close` on builder failure paths after construction

  (FR-021, Principle XII)
- [X] T038 [US2] Run `ginkgo -v --focus-file session_metrics_test.go --focus "US2" ./tests/e2e/` until US2-S1 to US2-S9 are green, changing fixtures only.

**Checkpoint**: US2 works without US1.

---

## Phase 5: User Story 3 - Sweep Runs Visible in Metrics (Priority: P2)

**Goal**: Duration histogram by outcome and `dry_run`, and the skipped counter.

**Independent Test**: T016 passes. The handler and sweep unit tests pass.

### Tests for User Story 3

- [X] T039 [P] [US3] Extend `internal/domain/oauth2session/sweep_test.go`. Assert `session_sweep_skipped_total{triggered_by=sweep}` for a deleted candidate and a no-longer-due candidate, and no increment in a dry run. Extend `sweepMetrics` (`:937-961`) to include the new name (FR-024).
- [X] T040 [P] [US3] Extend `internal/adapters/http/handlers/admin/session_sweep_handler_test.go` with a ManualReader:
  - one `session_sweep_duration_seconds` data point per outcome `completed`, `rejected` (parse error and `ErrInvalidSweepRequest`), `aborted`, and `canceled`, each with the correct `dry_run`
  - unit `s` and the bucket boundaries `1, 5, 10, 30, 60, 120, 300, 600, 1200, 1800, 3600`
  - nothing recorded when the operator principal is missing

  (FR-023)

### Implementation for User Story 3

- [X] T041 [US3] Add `skippedCounter` (`session_sweep_skipped_total`, unit `{refresh}`, description from `contracts/ports.md` §4) in `NewSessionSweepService` (`internal/domain/oauth2session/sweep.go:55-65`). Increment it at both real-sweep `result.Skipped++` sites with `sweepMetricAttributes`. Makes T039 green.
- [X] T042 [US3] In `internal/adapters/http/handlers/admin/session_sweep_handler.go`:
  - create the histogram in `NewSessionSweepHandler` per `contracts/ports.md` §5
  - record `time.Since(started).Seconds()` with `outcome` and `dry_run` in the existing deferred block, after the audit log
  - keep the missing-principal return before `started`

  Makes T040 green.
- [X] T043 [US3] Run `ginkgo -v --focus-file session_metrics_test.go --focus "US3" ./tests/e2e/` until US3-S1 to US3-S3 are green, changing fixtures only.

**Checkpoint**: All three stories work independently.

---

## Phase 6: Documentation

- [X] T044 Rewrite "Schedule sweeps" and "Monitor and resolve failures" in `docs/operations/session-sweep.md` per FR-025:
  - the chart values replace the hand-written CronJob, plus a `helm template` path for plain manifests
  - both lookahead constraints (keep the guidance already corrected on this branch)
  - proxy recipes with a projected ServiceAccount token and a TLS client certificate
  - the admin-authenticates-but-does-not-authorize trust statement, with an example NetworkPolicy selecting `app.kubernetes.io/component: session-sweep`
  - why `failed > 0` succeeds
  - a recommendation to give the job a dedicated operator principal (SR-006)
  - the new instruments, `max` versus `sum` aggregation, the OTLP name-translation note
  - the example queries and kube-state-metrics signals from `contracts/observability.md`

  Also update "How refresh works" so it no longer says the broker cannot be scheduled from the chart.
- [X] T045 [P] Update `docs/configuration.md`:
  - two rows in the Token Refresh table (`:248-255`)
  - the YAML block and "The three flags" sentence (`:700-712`)
  - replace `:722` with a pointer to the chart schedule and runbook
  - mention `oauth2_sessions` in the telemetry reference (`:1486-1503`)

  (FR-026)
- [X] T046 [P] Update `api/admin/openapi.yaml` description text only, at `:1701` (operator-managed job becomes the chart-managed schedule or another external job) and the example at `:1752` (an hourly CronJob with `PT2H`). Validate the file with the repository's OpenAPI check. No field, status, or schema change (API-002).
- [X] T047 [P] Update `ARCHITECTURE.md`:
  - the admin sweep text (`:538-542`)
  - `:573`: keep "no broker timer" and add the chart CronJob
  - session metrics (`:750-763`): add the gauge, histogram, and skipped counter

  (Principle II)

---

## 🔒 Phase N: Constitution Compliance Verification

#### Design Phase Verification

- [X] T048 Verify every task traces to a requirement, scenario, or binding constraint, and that no unrelated remediation was added (Feature Scope).
- [X] T049 Verify the glossary entries in `ARCHITECTURE.md` (Principle V).
- [X] T050 Verify `examples/config/` YAML and the `examples/config/README.md` references (Principle VII).
- [X] T051 Verify `api/admin/openapi.yaml` changed only in descriptive text and examples (API-001, API-002, Principles IV, X).
- [X] T052 Verify there is no file in `migrations/` from this branch (DB-001, Principle IX).
- [X] T053 Verify spec Journeys 1 to 4 match the implemented values, rendered manifests, HTTP exchanges, logs, and metric names (Principle XIV).
- [X] T054 Verify the 12 US2 and US3 E2E scenarios exist, one `It()` each, and the US1 render subtests, with spec comments and red results recorded in `plan.md` (Principle XIII).

#### Implementation Phase Verification

- [X] T055 Verify the deferred findings in research R16 appear in no task or diff (Feature Scope).
- [X] T056 Verify the Helm values, schema, templates, configmap, and README cover every changed setting (Principle VII).
- [X] T057 Verify security defaults:
  - sweep disabled by default
  - render fails without an identity
  - no credential in the manifest or the rendered script (US1-S5); curl reads the file itself
  - hardened pod
  - no principal or session attribute on any metric

  (Principles I, III; SR-001 to SR-007)
- [X] T058 Verify the hexagonal boundaries:
  - the domain uses `ports.UserSessionHealthRepository`
  - state precedence is in the domain, not SQL
  - the handler only records the histogram
  - wiring is in `builder.go`
  - no adapter imports another

  (Principles VI, XII)
- [X] T059 Verify that unit tests T019, T023, T024, T035, T039, and T040 were red before T020, T025, T026, T036, T041, and T042, and that the E2E tests changed only in fixtures after T017 (Principle VIII, XIII).
- [X] T060 Run `just check`, `just test`, `just test-integration-infra`, `just helm-lint`, and `just verify` from the repository root, and record the results (Principle XIII).
- [X] T061 Review the final diff for debug output and comments that restate code. Run `grep -rniE` over the diff for company-internal names, hosts, and tool names, with zero matches (spec Input).

---

## Dependencies & Execution Order

### Phase dependencies

- Phase 1 (T001 to T003) → Phase 2 (T004 to T018) → Phase 2.5 (T019 to T027)
- US1 (T028 to T034) needs Phase 2 only. It does not use the health port, so it can start before Phase 2.5 finishes.
- US2 (T035 to T038) needs Phase 2.5.
- US3 (T039 to T043) needs Phase 2. US3-S2's `aborted` case uses the Postgres fixture only.
- Phase 6 (T044 to T047) follows the stories whose behavior it documents: T044 needs US1 to US3, T045 needs T020, T046 needs US1, T047 needs US2 and US3.
- Phase N follows everything.

### Within phases

- T006 before T015 (the config field must compile).
- T002, T003, and T013a before T014. T013a before T015 and T016.
- T019 before T020. T021 and T022 before T023 to T026. T023 before T025. T024 before T026. T025 and T026 before T027.
- T028 before T030 and T031. T031 before T032 and T034.
- T035 before T036 before T037 before T038.
- T039 before T041. T040 before T042.

### Parallel opportunities

- T002 and T003. T004, T007, T009, and T010. T014, T015, and T016 (T015 and T016 share a file, so one owner writes US2 first).
- T023 and T024. T029 beside T028.
- US1 and US3 in parallel with Phase 2.5. US2 after Phase 2.5 in parallel with US1.
- T045, T046, and T047.

## Parallel Example: after Phase 2

```text
Owner A (US1): T028 → T029 → T030 → T031 → T032 → T033 → T034
Owner B (foundation, then US2): T019 → T020; T021 → T022 → (T023 ∥ T024) → T025 → T026 → T027 → T035 → T036 → T037 → T038
Owner C (US3): (T039 ∥ T040) → T041 → T042 → T043
```

## Implementation Strategy

1. **MVP**: Phases 1 and 2, then US1 (T028 to T034). This alone meets acceptance criterion 1. A deployment can switch from a hand-written CronJob to the chart.
2. **Criterion 2**: Phase 2.5 and US2. This adds the session-health gauge across all services and users.
3. **Run visibility**: US3, then Phase 6 docs and Phase N.
