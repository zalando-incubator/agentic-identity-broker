# Implementation Plan: Proactive Token Refresh

**Branch**: `029-proactive-token-refresh` | **Date**: 2026-10-08 | **Spec**: [spec.md](spec.md)

**Input**: Feature specification from `/specs/029-proactive-token-refresh/spec.md`

## Summary

Third-party access tokens are refreshed only when token exchange finds them **already expired**.
The agent's exchange then waits on a synchronous upstream refresh. This feature moves refresh off
the hot path in two ways:

1. **Proactive refresh on the read path (US1)**: When token exchange serves a token that expires
   within `token_refresh.lookahead_duration` (default 5m), the current token is returned at once and
   the session is handed to a bounded, per-replica background pool. Duplicate submissions for a
   session already in flight consume no slot. When the pool is saturated, the refresh is dropped.
   The caller already holds a valid token, so this is safe.
2. **Admin session sweep (US3)**: `POST /api/sessions/sweep` on the admin server pages through
   `user_sessions` in access-token expiry, then UUID-byte order. It refreshes sessions due within a
   fixed window, including those with no active agent traffic. An operator CronJob schedules it.
   The broker runs no scheduler and no distributed lock.

Research found that much of the spec already exists, and the design builds on it. The expired-token
path (US2) is implemented, with a `singleflight` group and a row-locked, update-only refresh
(`UserSessionRefreshRepository.WithLockedSession`). That primitive already gives the "no
resurrection" semantics that DB-002 asks for. It also deduplicates **across replicas**: a refresh
waiting on the lock re-checks the row and skips the upstream call if another replica has already
refreshed it. All three triggers therefore share one locked-refresh function,
`refreshDueSession(threshold, trigger)`. Only the threshold differs: `now` for on-demand,
`now + lookahead` for background, and a fixed `sweepStart + lookahead` for the sweep (research R1, R5).

Two codebase facts shape the design:
- **Admin `WriteTimeout` is 15 s.** A realistic sweep (hundreds of upstream calls) would be cut off.
  The sweep handler lifts its own write deadline through `http.ResponseController`. That requires
  the logging middleware's `responseWriter` to implement `Unwrap()`, which is a Phase 0 refactor
  (R8).
- **Each locked refresh holds a database connection for the length of the upstream call**, and the
  pool is fixed at 25. `background_workers` is therefore capped at 20, so background refresh cannot
  starve the exchange path it exists to speed up (R9).

## Technical Context

**Language/Version**: Go 1.27.1

**Primary Dependencies**: chi v5 (admin routing), sqlx + pgx v5 (PostgreSQL), `golang.org/x/sync/singleflight`
(already a direct dependency, `go.mod:62`), OpenTelemetry Go v1.46 (`metric`, `trace`; global providers per
ADR 011), Viper/Cobra (config). **No new module dependency**; the ISO 8601 parser uses the Go standard library (R10).

**Storage**: PostgreSQL (production) and in-memory (dev/test). There is no table or column change.
Accepted ADR 039 permits a concurrent, two-key B-tree index on `(access_token_expires_at, id)` under a narrow, non-atomic Principle IX exception. The guard, migration lifecycle and recovery, packaged v4.17.0 CLI, and normal-planner pagination checks passed on PostgreSQL 15 (research R4).

**Testing**: stdlib `testing` + testify (unit, table-driven). Ginkgo/Gomega E2E in `tests/e2e/`.
testcontainers `postgres:15-alpine` for repository, migration, and `EXPLAIN` integration tests (`-tags=integration`).

**Target Platform**: Linux container. The broker binary is `cmd/agentic-identity-broker` (dual server
:8000/:14000, ADR 004), deployed by `charts/agentic-identity-broker/`.

**Project Type**: Single Go service within the monorepo. Backend only; no frontend change.

**Performance Goals**:
- Exchange hot path: zero upstream calls for tokens outside the window (SC-001). For tokens inside
  the window, submission is O(1) and non-blocking: one `sync.Map.LoadOrStore` and one non-blocking
  channel send. It allocates only on acceptance (goroutine and closure), and the response never waits
  on it (SC-002).
- Per-session refresh dedup: at most one upstream refresh in flight per session per replica (FR-003).
  The row lock adds cross-replica suppression after commit (SC-003).
- Sweep: sequential, about 1/upstream-latency sessions per second per request. Each page holds at
  most `page_size ≤ 1000` sessions (FR-008). A visited-ID set prevents repeated accounting when a
  refreshed row moves forward within the threshold. Memory is O(page_size + unique candidates),
  not strictly page-bounded.

**Constraints**:
- Fail closed. Tokens are never persisted unencrypted (SR-001) and never logged. Responses contain
  counts only (SR-002).
- Shutdown drains in-flight refreshes within `server.shutdown.timeout`, then cancels (SR-003).
  Cancelling mid-refresh can lose a rotated refresh token (R7).
- The client secret is decrypted per refresh and never cached (SR-004). This uses the existing
  `GetForTokenAcquisition`.
- `background_workers ≤ 20` of the 25 pooled database connections (R9).
- The per-route write-deadline lift applies to `POST /api/sessions/sweep` only. The caller owns the
  sweep timeout (R8).

**Scale/Scope**: Two separately accepted ADRs (038 for refresh, 039 for the index exception), one
gated migration pair, one new port method (two adapters), three new domain files
(`refresh_trigger.go`, `background_refresh.go`, `sweep.go`), one admin handler and parser, config,
Helm, docs, one E2E file (10 scenarios), and about 250 lines changed in `oauth2session/service.go`.

Research R1–R13 resolve the design questions. R4 records the accepted non-atomic exception, the
implemented runner guard, and the migration recovery proof. The design gates passed before feature
implementation.

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

**Design Preconditions (BLOCKING)**:

- [x] **Domain Model**: Documented in [data-model.md](data-model.md). There is no new entity. New
  items: the `UserSession.AccessTokenExpiresBy` predicate on the existing aggregate, the
  `SweepRequest`/`SweepResult` value objects, the `RefreshTrigger` enumeration, and the
  process-scoped `backgroundRefresher`. Sweep and background state machines are diagrammed. The
  `SessionTokensRefreshed` domain event is realized as the existing `session.oauth2.token_refreshed`
  structured event. The codebase has no event bus.
- [x] **Domain Concepts**: The original seven glossary entries go to `ARCHITECTURE.md` (data-model.md
  → Glossary additions): Refresh Lookahead Window, Proactive Refresh, Session Sweep, SweepRequest,
  SweepResult, RefreshTrigger, and SessionTokensRefreshed. The amended design adds SessionExpiryCursor.
- [x] **Entity IDs**: N/A. There is no new entity with a UUID key. `SessionExpiryCursor` holds
  `AccessTokenExpiresAt time.Time` and `ID id.SessionID` for the expiry-then-UUID keyset.
- [x] **Configuration Design**: The `token_refresh` section has three keys, defaults, environment
  variables, CLI flags, and validation bounds in [contracts/configuration.md](contracts/configuration.md).
- [x] **Config Examples**: The example YAML and README entry are specified in the configuration
  contract. T013 and T014 add them before feature implementation.
- [x] **Helm Deployment Contract**: The broker is chart-managed. `broker.tokenRefresh` will be added
  to `values.yaml`, `values.schema.json` (strict), `templates/configmap.yaml`, and `README.md`
  (configuration.md → Helm). There is no CronJob template, because scheduling is an operator
  responsibility per the spec. A reference manifest goes in the runbook.
- [x] **API Design First**: The contract fragment
  [contracts/admin-session-sweep.openapi.yaml](contracts/admin-session-sweep.openapi.yaml) is
  written and validated (openapi-spec-validator: OK) before any implementation.
- [x] **API Documentation**: The fragment merges into `api/admin/openapi.yaml` in Phase 2c (new
  `Sessions` tag, path, and two schemas). `docs/api/` is for end-user APIs only, so it is N/A for an
  admin endpoint.
- [x] **API Changes**: The endpoint is additive. On 2026-10-09, the user approved all four admin
  API choices in writing for PR #196: the page-size cap of 1000, the restricted ISO 8601 grammar,
  unknown-field rejection with `400`, and missing operator principal with `500`. ADR 038 records
  those choices. This is user approval in this conversation, not a claim of a PR review comment.
  T018 records this approval before endpoint implementation.
- [x] **Database Design and Migration — VERIFIED**: ADR 039 accepts the named non-atomic exception for the two-key migration `036`. The shared validator guards every runner. PostgreSQL 15 tests covered UP, DOWN, reapply, an invalid interrupted build, and dirty-version recovery. First, middle, and late sparse-due pages used the valid composite index under normal planner settings. Recovery remains non-atomic.
- [x] **E2E Acceptance Tests**: All 10 acceptance scenarios get E2E tests in
  `tests/e2e/proactive_token_refresh_test.go` before implementation (Testing Strategy).
- [x] **E2E Test Mapping**: 1:1 scenario-to-`It()` mapping, tabulated below.
- [x] **E2E Red-Phase Design**: All ten scenarios now include an expectation that fails semantically
  before implementation. US1-S1 observes a missing saturation-drop WARN event. US2-S1..S3
  require new trigger-aware success or failure events. US1-S2/S3 and US3-S1..S4 already target
  missing proactive refresh or sweep behavior. T025 records the actual red results below.
- [x] **Frontend Playwright E2E**: N/A. No React UI change.
- [x] **Frontend Screenshots**: N/A. No React UI change.

**Implementation Considerations**:

- [x] **Security-First**: The feature is always on with no bypass. All persisted tokens go through
  the existing service-scoped encryption. The sweep requires an operator principal (fail closed:
  `500 server_misconfiguration`) and is audit-logged. Responses carry counts only. No principal
  appears in session logs or any metric attribute. Shutdown cannot discard a rotated refresh token
  in the normal case (R7).
- [x] **Architecture Docs**: `ARCHITECTURE.md` is updated in the same PR: the admin route tree
  (`:463-533`), worker shutdown (`:538-556`), session refresh and concurrency (`:664-726`, including
  the existing singleflight description at `:721-723`), and the glossary.
- [x] **ADRs — ACCEPTED FOR FEATURE 029**: On 2026-10-09, the user separately accepted
  `adrs/038-proactive-token-refresh.md` and `adrs/039-concurrent-session-expiry-index.md` in
  writing for PR #196. The same-PR approval is scoped to feature 029 despite the general
  proposal rule in `AGENTS.md`. ADR 009 still governs the dedicated migration image.
- [x] **Library-First Security**: No cryptography is introduced. Encryption stays behind
  `ports.EncryptionPort` (AWS Encryption SDK / memory). The ISO 8601 parser is not a security
  primitive.
- [x] **Zalando Guidelines**: snake_case properties, ISO 8601 `format: duration`, `additionalProperties: false`,
  standard status codes, and the existing `ErrorResponse`. The verb-like path `POST /api/sessions/sweep`
  departs from Zalando's "avoid actions" guidance. The spec fixes the path (API-002) for a
  non-idempotent maintenance operation with no persistent resource to model. This is recorded here
  for the stakeholder review.
- [x] **End-User Docs**: N/A for `docs/api/` (admin API). Operator docs:
  `docs/operations/session-sweep.md` (runbook and reference CronJob) and the `docs/configuration.md`
  Token Refresh section.
- [x] **Migration Testing — VERIFIED**: The two-key index passed PostgreSQL lifecycle, invalid-index recovery, directive rejection, and normal-planner first/middle/late `EXPLAIN` checks. The packaged v4.17.0 guard applied, rolled back, and reapplied valid migration `036` on a disposable database. An alternate image path failed before validation or exec. The manual recovery procedure does not make concurrent DDL atomic (research R4, ADR 039).
- [x] **Hexagonal Architecture**: The domain (`oauth2session`) depends on the
  `UserSessionExpiryRepository` and `UserSessionRefreshRepository` ports. The admin handler parses
  input only, and defaulting, bounds, and classification live in `SessionSweepService`. No adapter
  imports another. The wiring is in `builder.go`, and routing receives the handler.
- [x] **Persistence Patterns**: A new focused ISP port (one method) beside the existing
  `UserSessionRefreshRepository` precedent. `SessionExpiryCursor` uses expiry and UUID bytes in
  both adapters. PostgreSQL uses first-page and later-page composite-keyset queries with sqlx
  `SelectContext`, the adapter read timeout, and `StorageError` wrapping (R2, contracts/ports.md §1).

**Gate status**: ADR 039 and the admin API have written approval. Both migration runners, both Helm modes, the packaged image, and the two-key PostgreSQL lifecycle and planner checks have execution proof. Migration recovery remains non-atomic.

**Post-Design Re-check**: DB-002 names the existing locked, update-only refresh. DB-003 uses the
focused expiry port with `SessionExpiryCursor{AccessTokenExpiresAt, ID}` and expiry-then-UUID order.
The domain model names `SessionSweepService` and the logged `SessionTokensRefreshed` event.
`ForceRefreshSession` uses `triggered_by=on-demand`. The admin write timeout requires the Phase 0
deadline-lift refactor. R4 records the non-atomic exception and the verified two-key migration, guard, and query plans.

## Spec Alignment

The specification and design agree on DB-002, DB-003, the `SessionSweepService` diagram,
`SessionTokensRefreshed` log semantics, and the on-demand trigger. DB-003 uses a composite cursor
in `(access_token_expires_at, id)` order. The clarification and FR-003, FR-009 name the locked
re-check instead of idempotent refresh upserts. API-002 names the restricted ISO 8601 grammar.
DB-001 names the accepted non-atomic exception in ADR 039. The concurrent path requires an enforced
directive and cannot provide atomic rollback (research R4).

## Project Structure

### Documentation (this feature)

```text
specs/029-proactive-token-refresh/
├── plan.md                                   # This file
├── research.md                               # Phase 0 — R1..R13
├── data-model.md                             # Phase 1 — predicate, value objects, state machines, glossary
├── quickstart.md                             # Phase 1 — validation guide
├── contracts/
│   ├── admin-session-sweep.openapi.yaml      # Phase 1 — OpenAPI fragment (validated)
│   ├── ports.md                              # Phase 1 — Go port/service/handler contracts
│   ├── configuration.md                      # Phase 1 — token_refresh config + Helm contract
│   └── observability.md                      # Phase 1 — metrics, log events, spans
├── checklists/                               # (none yet)
└── tasks.md                                  # Phase 2 output (/speckit-tasks — not created here)
```

### Source Code (repository root)

```text
adrs/
├── 038-proactive-token-refresh.md                       # ACCEPTED — feature 029 refresh behavior
└── 039-concurrent-session-expiry-index.md                # ACCEPTED — feature 029 Principle IX exception

api/admin/openapi.yaml                                   # MODIFIED — Sessions tag, /api/sessions/sweep, 2 schemas

migrations/
├── 036_user_sessions_access_token_expiry_index.up.sql   # VERIFIED — guarded two-key concurrent index
└── 036_user_sessions_access_token_expiry_index.down.sql # VERIFIED — guarded concurrent drop

internal/ports/
├── config.go                                            # MODIFIED — TokenRefreshConfig, Config.TokenRefresh
└── storage.go                                           # MODIFIED — UserSessionExpiryRepository

internal/config/
├── loader.go                                            # MODIFIED — defaults + IDENTITY_BROKER_TOKEN_REFRESH_* bindings
├── validator.go                                         # MODIFIED — validateTokenRefreshConfig
└── *_test.go                                            # MODIFIED — defaults, env, bounds

internal/domain/storage/
├── user_session.go                                      # MODIFIED — AccessTokenExpiresBy
└── user_session_test.go                                 # MODIFIED — boundary table + equivalence

internal/domain/oauth2session/
├── service.go                                           # MODIFIED — Config, NewConfigFromPorts, GetValidAccessToken,
│                                                        #   refreshDueSession (replaces refreshExpiredSession),
│                                                        #   trigger-aware logging, Close
├── refresh_trigger.go                                   # NEW — RefreshTrigger
├── background_refresh.go                                # NEW — backgroundRefresher (slots, inflight, drain)
├── background_refresh_test.go                           # NEW
├── sweep.go                                             # NEW — SessionSweepService, SweepRequest/Result, ErrInvalidSweepRequest
├── sweep_test.go                                        # NEW
└── service_refresh_concurrency_test.go                  # MODIFIED — commit-before-log for all triggers

internal/adapters/storage/
├── factory.go                                           # MODIFIED — SessionExpiry()
├── memory/user_session.go                               # MODIFIED — ListExpiringSessions
├── memory/user_session_test.go                          # MODIFIED — contract E1–E7
├── postgres/user_session.go                             # MODIFIED — ListExpiringSessions (+ exported-to-test query const)
└── postgres/user_session_test.go                        # MODIFIED — contract E1–E7, SC-006 EXPLAIN (integration tag)

internal/adapters/http/
├── middleware.go                                        # MODIFIED (Phase 0) — responseWriter.Unwrap
├── server_test.go                                       # MODIFIED (Phase 0) — write-deadline lift through NewHandler chain
├── handlers/admin/session_sweep_handler.go              # NEW
├── handlers/admin/session_sweep_handler_test.go         # NEW
├── handlers/admin/iso8601_duration.go                   # NEW — parseISO8601Duration
├── handlers/admin/iso8601_duration_test.go              # NEW
└── routing/admin.go                                     # MODIFIED — POST /api/sessions/sweep (+ route-tree comment)

internal/app/
├── handlers.go                                          # MODIFIED — AdminHandlers.SessionSweep
└── builder.go                                           # MODIFIED — config flow, SessionSweepService, handler, Close in shutdown chain

tests/e2e/
├── proactive_token_refresh_test.go                      # NEW — US1-S1..S3, US2-S1..S3, US3-S1..S4
└── helpers/mock_upstream.go                             # MODIFIED — WithTokenResponseGate

examples/config/third-party-oauth2.yaml                  # MODIFIED — token_refresh block
examples/config/README.md                                # MODIFIED — mention
docs/configuration.md                                    # MODIFIED — TOC, reference rows, Token Refresh section
docs/operations/session-sweep.md                         # NEW — runbook, reference CronJob, approved migration failure procedure
charts/agentic-identity-broker/
├── values.yaml                                          # MODIFIED — broker.tokenRefresh
├── values.schema.json                                   # MODIFIED — strict schema
├── templates/configmap.yaml                             # MODIFIED — token_refresh rendering
└── README.md                                            # MODIFIED — parameter rows
ARCHITECTURE.md                                          # MODIFIED — routes, shutdown, refresh concurrency, glossary
```

**Structure Decision**: There is no new package. Proactive refresh, the sweep, and the trigger
vocabulary live in `internal/domain/oauth2session`, the bounded context that already owns the
third-party session lifecycle and the refresh primitive. A separate `domain/sweep` package would
have to reach back into `oauth2session` internals, which would make the split artificial
(constitution "Domain packaging"). The admin handler joins its siblings in
`internal/adapters/http/handlers/admin/`. Untouched: the frontend, `infra/`, `internal/extproc/`
(its cache, ADR 012, sees only faster broker responses), `internal/domain/tokenexchange/` (its call
to `GetValidAccessToken` is unchanged), and `cmd/` (the shutdown chain is already invoked through
`App.Shutdown`).

## Implementation Phase Overview

| Phase | Purpose | Required? |
|-------|---------|-----------|
| **Phase 0** | Pre-implementation refactoring: `Unwrap` on response wrappers; extract `refreshDueSession` | Included |
| **Phase 1** | Setup: confirm no dependency change | Required (no-op) |
| **Phase 2** | Design Preconditions: accepted ADRs and API, glossary, config + Helm, guarded index migration, E2E red | **MANDATORY; gates passed** |
| **Phase 2.7** | Entity Boilerplate | Skipped |
| **Phase 2.5** | Foundational: expiry port + adapters + contract and `EXPLAIN` tests; `RefreshTrigger`; predicate | Required |
| **Phase 3** | US1 (P1): background refresher, lookahead submission, shutdown drain, metrics and logs | Required |
| **Phase 4** | US2 (P1): trigger-aware on-demand logging; semantically red US2 E2E cases | Required |
| **Phase 5** | US3 (P2): sweep service, parser, handler, route, wiring, audit | Required |
| **Phase 6** | Documentation: ARCHITECTURE.md, runbook, configuration docs | Required |
| **Phase N** | Constitution Compliance verification | **MANDATORY** |

- [x] Phase 0 (refactoring): **include** without behavior change. The stakeholder waived a separate PR on 2026-10-09; this refactor remains in the feature branch.
  1. Add `Unwrap() http.ResponseWriter` to `responseWriter` (`internal/adapters/http/middleware.go:107`).
     Confirm that every other wrapper on the admin chain (otelchi, `tokenEndpointTelemetry`) passes
     `http.ResponseController` through. Add the `NewHandler` + real `http.Server{WriteTimeout}` test
     that proves a handler can lift its deadline. This changes nothing for existing routes.
  2. Replace `refreshExpiredSession` with `refreshDueSession(ctx, principal, serviceID, threshold, trigger)`,
     called only as `(…, time.Now(), RefreshTriggerOnDemand)`. Add `UserSession.AccessTokenExpiresBy`
     and use it in the lock callback (equivalent to the current `HasValidAccessToken` check, asserted
     by test). Return a ciphertext-bearing session through singleflight and decrypt per joiner.
     The final shared result also records whether the leader refreshed and which trigger ran, so a
     background joiner does not refresh a short-lived token again. Existing token-exchange tests pass.

  Reason: Both are structural prerequisites that reviewers can approve without understanding
  proactive refresh. Item 2 is the seam that US1 and US3 plug into.
- [x] Phase 2.7 (entity boilerplate): **skip**. There is no new entity. The single new port method
  and one handler land with their logic in Phases 2.5 and 5. The E2E red phase needs no stub route,
  because the absent route returns `404`, which fails the `200` assertions semantically.

### Phase 1: Setup

1. Confirm that `golang.org/x/sync` (direct), `go.opentelemetry.io/otel/metric`, and `/trace` are
   present at the required versions. Expect no `go.mod` change.

### Phase 2: Design Preconditions

**2a — Domain model & ADR**
2. Record the separate written acceptance of `adrs/038-proactive-token-refresh.md` on
   2026-10-09 for PR #196. The scoped same-PR approval applies to feature 029 only.
3. Add the seven original glossary entries and `SessionExpiryCursor` to `ARCHITECTURE.md`
   (data-model.md → Glossary additions).

**2b — Configuration**
4. Add `ports.TokenRefreshConfig` and `Config.TokenRefresh`. Register the three persistent CLI
   flags in `cmd/agentic-identity-broker/root.go`, bind them in `Loader.bindFlags`, and apply
   loader defaults and environment bindings. Write tests first for precedence and validation.
5. Add the example YAML, the README mention, and the `docs/configuration.md` rows and section.
6. Add Helm `broker.tokenRefresh` to values, schema, ConfigMap, and README (configuration.md → Helm).

**2c — API**
7. Merge the fragment into `api/admin/openapi.yaml`. ADR 038 records the four admin API choices.
   The user approved them in writing on 2026-10-09 for PR #196, before endpoint implementation.

**2d — Database (TWO-KEY MIGRATION VERIFIED)**
8. ADR 039 accepts the named non-atomic exception for migration `036`. The shared guard protects the image, both Helm Job modes, and every Go runner listed in ADR 039. PostgreSQL 15 tests proved the two-key index lifecycle and invalid-index/dirty-version recovery without changing sessions. The packaged v4.17.0 CLI applied, dropped, and reapplied migration `036` on a disposable database. The Go driver completed the same lifecycle and recovery tests.

**2e — Frontend**: N/A.

**2f — E2E red phase**
9. Add `WithTokenResponseGate` to `tests/e2e/helpers/mock_upstream.go`.
10. Write ten `It()` blocks in `tests/e2e/proactive_token_refresh_test.go`. Use
    `bootstrap.NewBufferedJSONLogger` for US1-S1 and US2-S1..S3. Compile and run all ten before
    feature implementation. Record concrete semantic failures for every scenario; no skips or
    placeholder assertions.

### Phase 2.5: Foundational

11. `ports.UserSessionExpiryRepository`, `storage.SessionExpiryCursor{AccessTokenExpiresAt time.Time, ID id.SessionID}`,
    the memory and PostgreSQL `ListExpiringSessions`, and the factory `SessionExpiry()`. Zero/zero
    starts a scan; reject a cursor with only one populated field. Both adapters order by expiry and
    then UUID bytes. Test E1–E7 with mutations and ciphertext snapshots (contracts/ports.md §1).
12. After the migration guard passes, test the named valid two-key index with normal-planner
    `EXPLAIN (FORMAT JSON)` for first, middle, and late small-limit pages. Seed sparse due rows and
    run `ANALYZE`; do not disable sequential scans (R4, SC-006).
13. `RefreshTrigger` and the five counters. Instruments are created once in constructors.

### Phase 3: US1 — Zero-latency exchange for active users (P1)

14. Unit tests first for `backgroundRefresher`: accept, saturation drop, in-flight dedup without a
    slot, submit after `Close`, idempotent `Close`, drain-then-cancel ordering, deleted-session
    no-op, upstream failure counter, and that the caller's token is unaffected.
15. Implement `background_refresh.go` and the submission in `GetValidAccessToken`, behind
    `AccessTokenExpiresBy(now+lookahead) && CanRefresh()`.
16. Add the background span with a link to the request span. Add the NFR-001/NFR-002 log lines.
17. Builder: pass `TokenRefreshConfig` through `NewConfigFromPorts` (migrate all callers via
    `lsp references`), and chain `OAuth2SessionService.Close` into `App.Shutdown` before transport
    and telemetry shutdown.
18. US1 E2E turns green.

### Phase 4: US2 — Expired session refresh (P1)

19. Move failure logging from `refreshSessionTokens` into the trigger-aware caller. On-demand keeps
    ERROR and gains `session_id` and `triggered_by=on-demand`. Route `ForceRefreshSession` success
    and failure logs through the same helper.
20. Extend `service_refresh_concurrency_test.go` with commit-before-success-log checks for all
    three triggers. The US2 E2E cases turn green when the trigger-aware log fields are implemented.

### Phase 5: US3 — Admin sweep (P2)

21. `parseISO8601Duration`, table-tested first (R10).
22. `SessionSweepService`, unit-tested first: defaulting, bounds, and `ErrInvalidSweepRequest`.
    Hold one threshold and advance `SessionExpiryCursor` from the last raw page row, even if that
    page contains only previously seen IDs. Track visited IDs per invocation for real and dry runs.
    A row whose refresh moves it forward within the threshold is evaluated once. Reject a nil
    expiry or non-increasing tuple as a repository failure. Also cover the count invariant,
    classification (R11), dry runs without writes, upstream calls, or metrics, cancellation between
    sessions, and bounded detached refresh.
23. `SessionSweepHandler`: operator-principal requirement, 4 KiB limit, unknown-field rejection,
    write-deadline lift, error mapping, audit line. Handler unit tests use a fake `sessionSweeper`.
24. Add the route in `SetupAdminRoutes` and update the route-tree comment. Wire
    `AdminHandlers.SessionSweep` in the builder.
25. US3 E2E turns green.

### Phase 6: Documentation

26. Update the `ARCHITECTURE.md` admin route tree, worker shutdown, refresh concurrency, and
    row-lock sections. Add `SessionExpiryCursor` beside the seven original glossary entries.
27. `docs/operations/session-sweep.md`: purpose, reference CronJob, schedule and lookahead
    guidance, `concurrencyPolicy: Forbid`, caller timeout, admin-proxy access, counts and metrics,
    and the approved migration failure procedure after the Phase 2d gate passes.

### Phase N: Constitution Compliance Verification

28. Run `just check`, `just test`, `just test-integration-infra`, and `just test-e2e-backend`.
29. Walk the constitution Implementation-Phase checklist against the diff. Cite the written
    2026-10-09 approval of the four admin API choices and separate ADR 038 and ADR 039 acceptance
    for PR #196. Confirm that the directive runner, migration image, PostgreSQL recovery tests,
    and Helm paths meet ADR 039.
    Check that session logs and metrics omit principals and that builder-only wiring remains intact.

## Testing Strategy

### End-to-End (E2E) Acceptance Tests

**Test Location**: `tests/e2e/proactive_token_refresh_test.go`

**Framework**: Ginkgo/Gomega following [tests/e2e/README.md](../../tests/e2e/README.md). Setup mirrors
`tests/e2e/token_exchange_test.go:44-157`.

**Test Organization**: `Describe("Proactive Token Refresh")` → one `Describe` per user story →
`Context` per precondition → one `It` per scenario. Each `It` carries
`// Scenario X.Y from specs/029-proactive-token-refresh/spec.md`.

**Scenario Mapping**:

| Spec Scenario | E2E Test Location | Test Description |
|---------------|-------------------|------------------|
| US1-S1 | `tests/e2e/proactive_token_refresh_test.go` | Drops a saturated refresh without delaying due or healthy exchanges. |
| US1-S2 | same | Returns the old token before a background renewal commits. |
| US1-S3 | same | Deduplicates exchanges during an in-flight refresh. |
| US2-S1 | same | Renews once and logs success after persistence. |
| US2-S2 | same | Requires reauthentication without an upstream call. |
| US2-S3 | same | Preserves encrypted tokens after provider rejection. |
| US3-S1 | same | Paginates due rows and excludes healthy and timeless rows. |
| US3-S2 | same | Counts an expired refresh token as a failure and continues. |
| US3-S3 | same | Refreshes a shared session once across concurrent replicas. |
| US3-S4 | same | Counts due rows without provider calls or writes. |

See [quickstart.md §2](quickstart.md#2-acceptance-scenarios-e2e-principle-xiii) for pass conditions.

**Red Phase Requirements**:
- US1-S1 fails because the baseline cannot admit one background refresh or emit the saturation
  WARN event. Assert that both due and healthy exchanges still return the stored valid tokens.
- US1-S2 and US1-S3 fail because the baseline does not submit a background refresh.
- US2-S1 fails on the missing `session_id` and `triggered_by=on-demand` success fields.
- US2-S2 and US2-S3 fail on the missing single ERROR `session.oauth2.refresh_failed` event
  with `session_id` and `triggered_by=on-demand`. Assert the existing error and storage behavior.
- US3-S1..S4 fail on the absent route and the exact expected counts.
- `XIt`, `PIt`, `XDescribe`, `PDescribe`, `XContext`, `PContext`, and `Skip()` are forbidden. No
  red-phase comments.

**Observed Red Phase (2026-10-08)**:

`go run github.com/onsi/ginkgo/v2/ginkgo --label-filter='!performance' --focus='Proactive Token Refresh' ./tests/e2e/` compiled and ran all ten cases with the pinned CLI.
The result was 0 passed, 10 failed, and 0 pending. The focus filter excluded 646 unrelated specs.

| Scenario | Concrete pre-implementation failure |
|---|---|
| US1-S1 | No `session.oauth2.proactive_refresh_dropped` WARN event appeared; expected one. |
| US1-S2 | The provider received zero background refresh requests; expected one. |
| US1-S3 | The provider received zero requests for the in-flight refresh; expected one. |
| US2-S1 | The committed refresh INFO event lacked `session_id`. |
| US2-S2 | No `session.oauth2.refresh_failed` ERROR event appeared; expected one. |
| US2-S3 | The provider-rejection ERROR event lacked `session_id`. |
| US3-S1 | The absent admin route returned 404; expected a 200 sweep summary. |
| US3-S2 | The absent admin route returned 404; expected a 200 sweep summary. |
| US3-S3 | The absent admin route returned 404; expected a 200 sweep summary. |
| US3-S4 | The absent admin route returned 404; expected a 200 dry-run summary. |

The existing success and provider-rejection events include the raw `actor` principal. The new tests reject that value after they check trigger fields. This privacy requirement remains open.

**Test Data Strategy**:
- Fixtures: `fixtures.DefaultPrincipal()`, `fixtures.ValidAgent()`, `fixtures.GitHubService()`
  (token endpoint pointed at the mock), `fixtures.ActiveGrant(...)`,
  `fixtures.SeedPlaceholderGrantData(...)`, `fixtures.GitHubSessionForPrincipal`,
  `ExpiredGitHubSessionForPrincipal`, and `FullyExpiredSessionForPrincipal`
  (`tests/e2e/fixtures/sessions.go:10-80`).
- Expiry offsets: seed sessions at `+2m` (inside the window), `+2h` (outside), or with `nil` expiry (FR-013). Use `UserSessions().Create` only for fixture setup. US1-S1 uses two due sessions and one healthy session with one background slot. US3 uses several principals for one service.
- No new fixture file. If more than two tests need it, a small `SessionForPrincipalExpiringIn`
  helper may be added to `fixtures/sessions.go`.

**Test Execution Flow**: Follow the template: write in 2f → verify red → implement → green →
fixture-only changes during implementation.

**Bootstrap Strategy**:
- Production `ServerFactory.BuildApp` → `app.Builder.Build()`, with fresh in-memory storage per test.
- US3-S3 builds two `App`s over the **same** `testStorage` to model two replicas (shared database,
  per-process singleflight and pools). Both servers are closed in `AfterEach`. `App.Shutdown`
  idempotency covers the double close.

**Helper Utilities**:
- New: `MockUpstreamOAuth2Server.WithTokenResponseGate() (release func())` (contracts/ports.md §6).
- Use `bootstrap.NewBufferedJSONLogger` to assert event names, levels, and fields in
  US1-S1 and US2-S1..S3. Assert exactly one failure event per rejected on-demand attempt.
- Existing: `WithSuccessfulTokenResponse`, `WithAccessToken`, `WithRefreshToken`,
  `GetTokenRequests`, the error-response controls for US2-S3, and
  `adminServer.AuthenticatedPOST(path, principal, "application/json", body)`.
- No new matchers.

### Unit & Integration Tests

**Unit Tests** (TDD: write first, see them fail, then implement):
- `internal/domain/storage/user_session_test.go`: `AccessTokenExpiresBy` boundary table and its
  equivalence with `!HasValidAccessToken()`.
- `internal/domain/oauth2session/background_refresh_test.go`: pool and lifecycle behavior (Phase 3,
  step 14). A gated fake provider makes the tests deterministic, with no sleeps.
- `internal/domain/oauth2session/sweep_test.go`: classification table (R11), invariants, dry run,
  cancellation, abort, visited-ID dedup after an expiry moves forward, and rejection of nil or
  non-increasing cursor tuples.
- `internal/adapters/http/handlers/admin/iso8601_duration_test.go`: accepted and rejected grammar,
  overflow.
- `internal/adapters/http/handlers/admin/session_sweep_handler_test.go`: input parsing, error
  mapping, operator requirement, audit fields.
- `internal/adapters/http/server_test.go`: write-deadline lift through the full middleware chain on
  a real `http.Server`.
- `internal/config/*_test.go`: defaults, environment, and bounds.

**Integration Tests** (`-tags=integration`, testcontainers PostgreSQL 15):
- `internal/adapters/storage/postgres/user_session_test.go`: `ListExpiringSessions` contract E1–E7,
  and the SC-006 normal-planner `EXPLAIN (FORMAT JSON)` assertion for first, middle, and late pages
  against the named valid two-key index. Inspect `EXPLAIN (ANALYZE, BUFFERS, FORMAT JSON)` for
  actual rows and buffers without turning off sequential scans.
- `tests/integration/migrations/`: Verify guarded migration `036` UP, DOWN, reapply, directive
  rejection, invalid-index failure, dirty-version recovery, and exactly two ordered B-tree key
  columns, `(access_token_expires_at, id)`.

**Test Coverage Goals**:
- Unit: every branch of `backgroundRefresher.submit` and `Close`, every R11 classification row,
  every parser rule.
- Integration: the full repository contract on both adapters, plus migration 036.
- E2E: 100% of the 10 acceptance scenarios.
- Frontend E2E: N/A.

## Complexity Tracking

No TDD exception is planned. Every feature E2E case must compile and fail semantically before
implementation. ADR 039 permits only migration `036` to be non-atomic. The original guard and
migration passed their checks. The amended two-key migration and planner still need verification.
