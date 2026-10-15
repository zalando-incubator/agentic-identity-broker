# Research: Proactive Token Refresh

**Feature**: `029-proactive-token-refresh` | **Date**: 2026-10-08 | **Spec**: [spec.md](spec.md)

This document resolves every unknown in the plan's Technical Context. Each entry records the
decision, the evidence from the current codebase, and the alternatives considered.

## Baseline: what already exists

The spec was written on 2026-05-07. This table records the original proactive-refresh baseline, except the corrected validity contract in FR-005. ADR 040 governs that later correction. Original red-phase evidence remains historical, not the current validity contract.

| Spec item | Current state | Evidence |
|---|---|---|
| US2 (expired token → synchronous refresh) | **Implemented.** `GetValidAccessToken` refreshes expired access tokens synchronously. | `internal/domain/oauth2session/service.go:1294-1355` |
| FR-003 per-replica dedup | **Implemented for the expired path only.** `refreshGroup singleflight.Group` is keyed by `principal|serviceID`. | `service.go:65`, `:1329-1341` |
| DB-002 "no resurrection" | **Implemented.** `UserSessionRefreshRepository.WithLockedSession` re-reads under `SELECT … FOR UPDATE`, returns `nil` for a missing row, and persists with `UPDATE … WHERE id` only. | `internal/ports/storage.go:255-261`; `internal/adapters/storage/postgres/user_session.go:150-205`; memory per-key lock `internal/adapters/storage/memory/user_session.go:97-134` |
| FR-005 absent or provider-rejected refresh authority | Missing refresh credentials require reauthentication after access expiry. Recorded terminal rejection stops issuance even before access expiry. Errors include `ErrReauthenticationRequired` and the specific refresh classification. Exchange returns `invalid_grant` with a `/sessions` recovery URI. | `internal/domain/oauth2session/service.go`: `GetValidAccessToken`, `refreshUnavailableError`, `refreshLockedSession` |
| SR-001 / SR-004 | **Implemented.** `setSessionTokens` encrypts with `{"service_id"}`. `GetForTokenAcquisition` decrypts a static confidential client's secret at call time; public and CIMD confidential clients have no secret. | `service.go:988-1023`; `internal/domain/thirdparty/service.go:271-286` |
| FR-001/FR-002 lookahead + background refresh | **Missing.** `HasValidAccessToken()` has no lookahead. | `internal/domain/storage/user_session.go:106-111` |
| FR-006..FR-011 admin sweep | **Missing.** There is no admin `/api/sessions` route. | `internal/adapters/http/routing/admin.go:36-112` |
| FR-014 / DB-001 expiry index | **Missing.** | `migrations/004_create_user_sessions.up.sql:57-69` |
| NFR-001 `triggered_by` + session ID | **Missing.** `logRefreshSuccess` emits `service_id` and `public_client` only. | `service.go:1467-1474` |

---

## R1 — Update-only persistence for refreshed tokens (DB-002)

**Decision**: Reuse the existing `UserSessionRefreshRepository.WithLockedSession` for all three
triggers (on-demand, background, sweep). Do **not** add an `UpdateTokensIfExists` method.

**Rationale**:
- `WithLockedSession` already has the DB-002 semantics. It is update-only, and a missing row
  returns `(nil, nil)`, so a session that was deleted mid-refresh is never resurrected.
- The callback re-evaluates the locked row before it calls the provider. A second replica sees
  a committed refresh and skips the upstream call. This meets FR-003 and FR-009 without a
  coordination service.
- A second update-only write path would violate "Extend, do not duplicate" (AGENTS.md Code Style).

**Spec alignment**: DB-002 now names the existing `UserSessionRefreshRepository.WithLockedSession`
operation. The clarification, FR-003, FR-009, and domain diagrams also describe the locked
re-check and update-only write.

The same locked operation persists terminal rejection without a second write path. The service captures HTTP 4xx `invalid_grant`, excluding 429, and changes only `RefreshTokenRejected` and `UpdatedAt`. It returns `(true, nil)` from the callback so the adapter commits. Only after commit does the service return the saved rejection error with `refreshed=false`.

Callback errors still write nothing, including after snapshot mutation. Write or commit failure returns `persistence_failed`, not a claim that rejection persisted. This preserves L3/L4 in [contracts/ports.md](contracts/ports.md).

The rejection flag means observed invalidity. False means no observed rejection, not proven validity. The broker retains ciphertext and all token metadata, then refuses subsequent issuance even before access expiry. Successful OAuth authorization and callback clear rejection through the existing upsert. Explicit termination deletes the row. Rejection never emits refresh-success events or increments refresh-success counters.

The stakeholder selected “Allow staged compatibility” on 2026-10-14 in the unmerged ADR 040 revision. The release sequence separates preparation, activation, and cleanup into deployable PRs. This cleanup release includes migrations 038, 039, and 040 and the prepared code. Migration 038 preserves the original table shape and adds the explicit view and stable invoker write functions. The preparation view reports false. True writes report `persistence_failed`, while ordinary PostgreSQL traffic works normally. Migrations 039 and 040 enforce durable rejection through the same boundary. Migration 040 removes only the unused legacy column. This release defaults and limits Helm targets to 40. Fresh installs run direct UP-all through 40, while upgrades use the guarded wrapper.

Before merging or deploying PR2 #225, complete PR1 #224 across every replica. Drain every unprepared replica and in-flight operation first. Migration 039 adds the real false-default flag, activating durable rejection on already-running PR1 brokers without restart. Before merging or deploying PR3, complete PR2 across every replica. End all legacy database dependencies and upgrade the embedded SPA and every external session-API consumer first. The legacy expiry column at 38 and 39 supplies storage compatibility only, never provider evidence. All phases preserve sessions and real metadata. Compatibility defaulted true in preparation and activation. Cleanup defaults false and omits the legacy wire field. Explicit true remains a deliberate override for legacy consumers, not a waiver of the consumer-upgrade gate. DOWN 040 retains rejection and restores only a nullable legacy column. DOWN 039 guards rejected rows. Resolve them through reauthentication or explicit termination, never flag clearing or automatic deletion. Drain prepared/new replicas before DOWN 038 removes their SQL boundary. Inspect actual schema and dirty metadata before deliberate repair, never automatic `Force`.

Migration 037, its index, and historical migrations remain unchanged. See [quickstart.md](quickstart.md#online-validity-deployment-and-rollback-migrations-038040) for availability prerequisites, fail-fast lock handling, and phase-compatible rollback.

**Alternatives considered**:
- *New `UpdateTokensIfExists(ctx, session) (bool, error)`*: This creates a second write path with
  weaker semantics. It has no row lock, so two replicas would both call the provider and the last
  writer would win. With refresh-token rotation, the loser's rotated refresh token is persisted and
  then overwritten. Some providers invalidate the whole token family on reuse. Rejected.
- *Optimistic concurrency on `updated_at`*: This needs schema-aware compare-and-swap and still
  allows duplicate upstream calls. Rejected.

## R2 — Placement of `ListExpiringSessions` (DB-003, ISP)

**Decision**: Add a new single-method port `UserSessionExpiryRepository` in
`internal/ports/storage.go`, next to `UserSessionRefreshRepository`:

```go
type UserSessionExpiryRepository interface {
    ListExpiringSessions(ctx context.Context, threshold time.Time, cursor storage.SessionExpiryCursor, limit int) ([]*storage.UserSession, error)
}
```

The existing `InMemoryUserSessionRepository` and `PostgresUserSessionRepository` types implement
it. The storage factory exposes the same instance through a new `SessionExpiry()` accessor. This
mirrors how `SessionRefresh()` exposes the refresh port
(`internal/adapters/storage/factory.go:96-109,131-146,220-230`).

**Rationale**: `UserSessionRepository` already declares 8 methods (`internal/ports/storage.go:213-253`),
which exceeds the constitution's 5–7 method ISP limit (Principle IX). Adding a ninth would widen the
violation. The refresh port split is the established precedent for a focused session capability.
The method signature is exactly the one in the amended DB-003 contract.

**Spec alignment**: DB-003 assigns the method to the focused `UserSessionExpiryRepository` port. Its one method takes `storage.SessionExpiryCursor` with expiry and ID.

**Alternatives considered**: The original spec draft put the method on `UserSessionRepository`.
That would exceed the Principle IX limit, so the focused port was selected.

## R3 — Keyset pagination and ordering (FR-008)

**Decision**: Use a composite `(access_token_expires_at, id)` keyset with a threshold fixed at sweep start. `storage.SessionExpiryCursor{AccessTokenExpiresAt time.Time, ID id.SessionID}` holds the last raw page row's two keys.

The first page uses the zero-time, zero-ID cursor and this query:

```sql
SELECT * FROM user_sessions
WHERE access_token_expires_at IS NOT NULL
  AND access_token_expires_at <= $1
  AND NOT refresh_token_rejected
ORDER BY access_token_expires_at, id
LIMIT $2
```

Later pages use a complete cursor and this query:

```sql
SELECT * FROM user_sessions
WHERE access_token_expires_at IS NOT NULL
  AND access_token_expires_at <= $1
  AND NOT refresh_token_rejected
  AND (access_token_expires_at, id) > ($2, $3)
ORDER BY access_token_expires_at, id
LIMIT $4
```

A zero ID and zero expiry start the scan. An expiry without an ID is invalid. A cursor with an ID can have a zero expiry. The memory adapter compares expiries first and uses `bytes.Compare` on the 16 UUID bytes for equal expiries. PostgreSQL `uuid` uses the same tie order.

The sweep advances its cursor from the last raw row of every full page, even if it already evaluated every row in that page. It stops after a short page. A nil expiry or non-increasing tuple from the read port aborts through the repository-failure path instead of causing a panic or loop.

**Rationale**:
- A prior row that expires after the threshold or is deleted disappears without shifting later keyset pages. Rows inserted behind the cursor are not included. Remaining rows ahead of the cursor stay eligible.
- A refreshed row can remain within the threshold and move ahead of its old key. A per-invocation `map[id.SessionID]struct{}` skips its later occurrence without another lock, upstream call, metric, log, or count. The same rule applies to dry runs.
- This visited-ID set costs O(unique candidates) broker memory. The limit bounds only the session snapshots in one page, not total broker memory. The fixed threshold excludes new rows with later expiries.
- `IS NOT NULL` satisfies FR-013. Both queries exclude known rejection with `NOT refresh_token_rejected`. The tuple predicate and order retain migration 037's two-key index design.

**Alternatives considered**:
- *UUID-only keyset*: A one-key `id` cursor does not follow the expiry index's sort order. It can require sorting or scanning the full due set on later pages. Rejected by the approved amendment.
- *`LIMIT/OFFSET`*: The precedent is `UserFilter` (`internal/ports/storage.go:56-60`). It skips rows under mutation. Rejected.

## R4 — Concurrent index migration without preflight (DB-001, FR-014, SC-006)

**Decision**: On 2026-10-12, the stakeholder amended accepted ADR 039 in this unmerged feature PR. Keep the migration `037` non-atomic exception and the separate migration image. Remove the guard from the image and all Go runners. Keep the `-- migrate:no-transaction` first-line comment as an informational project convention, not an enforced control.

On 2026-10-12, merging `main` into this unmerged PR renumbered the migration from `036` to `037`. `main` already uses version 36 for `036_add_refresh_token_grant_id.{up,down}.sql`, and `golang-migrate` rejects two migrations with one version. The SQL, index, and named exception are unchanged.

**Why the exception is necessary**: A blocking index build can stop writes to the session table. PostgreSQL 15 cannot run `CREATE INDEX CONCURRENTLY` inside a transaction. A failed build can leave an invalid index and dirty version. This does not meet Constitution Principle IX's normal atomic migration rule. The named exception applies only to migration `037`.

**Runner behavior**: `build/docker/Dockerfile.migrate` packages `golang-migrate` v4.17.0, its direct entrypoint, and the migration SQL. Both Helm grants modes retain direct `-path /app/migrations -database ... up` for fresh-install target 0. PostgreSQL upgrades with migrations enabled refuse target 0 during rendering. Positive stages use a scoped shell wrapper in the existing Alpine image. The same tool inspects version and dirty state. Dirty or unreadable metadata aborts without `Force`. Equal/newer clean schemas are non-destructive no-ops. Only a lower clean version runs `goto <version>`. Serialize database migration runners to prevent a version-check/execution race. Helm stages never request DOWN. Explicit rollback uses the separate runner after phase compatibility is established. Go v4.20.1 test runners use their existing migration paths. Neither runner interprets the no-transaction comment. PostgreSQL drivers execute migration SQL without a migration-wide transaction. Version updates do not make concurrent DDL atomic. This stage guard does not restore the removed migration-037 preflight.

**Removed preflight**: There is no check of the SQL pair, exact statement, URL options, or alternate image migration paths before SQL. A missing DOWN can go unnoticed until after UP applies and rollback is needed. Incorrect SQL can execute or leave a dirty version. The user selected this tradeoff after reviewing it.

**Index contract**: Migration `037` creates the plain B-tree `idx_user_sessions_access_token_expires_at` with `CREATE INDEX CONCURRENTLY idx_user_sessions_access_token_expires_at ON user_sessions (access_token_expires_at, id);`. DOWN uses `DROP INDEX CONCURRENTLY`. Neither direction uses `IF NOT EXISTS`. Both files retain the informational first-line directive.

**Failure handling**: A failed Helm migration Job blocks the release. Inspect `schema_migrations.version`, `dirty`, and the named index in `pg_index`. Do not retry or force a version before inspecting the schema. Drop only an inspected invalid index outside a transaction. At dirty UP version `37`, return to version `36` only when the schema matches version `36`. If the expected index is valid, review whether the build finished before version recording failed. ADR 039 describes manual recovery. It is not atomic rollback.

**Verification (2026-10-12, before renumbering)**: Direct Go runners passed PostgreSQL 15 UP, DOWN, reapply, invalid-index interruption, dirty-version recovery, and preserved session data. The direct ARM64 v4.17.0 image applied, dropped, and reapplied migration `036` on disposable PostgreSQL (as migration `036`, before the `main` merge renumbered it to `037`). Successful UP ended at clean version `36` with a valid two-key B-tree; DOWN ended at clean version `35` with the index absent. Both Helm grants modes kept their migration arguments and grants order. `helm lint`, `just check`, and focused proactive-refresh E2E passed. The adapter's normal-planner `EXPLAIN` used the two-key index for first, middle, and late pages. Earlier observed `EXPLAIN (ANALYZE, BUFFERS, FORMAT JSON)` pages returned two index rows with three shared-hit blocks and zero shared-read blocks each.

**Alternatives**: The stakeholder kept the separate migration image and Helm grants Job under ADR 009. Removing either would break the version-matched migration path. A blocking transactional build could avoid the named exception but can stop writes to the session table.

## R5 — Lookahead predicate and where it lives (FR-001, FR-002, FR-013)

**Decision**: Add one domain predicate on the aggregate:

```go
// internal/domain/storage/user_session.go
func (s *UserSession) AccessTokenExpiresBy(t time.Time) bool // false when AccessTokenExpiresAt == nil; else !AccessTokenExpiresAt.After(t)
```

All three triggers evaluate it under the row lock with a trigger-specific threshold:

| Trigger | Threshold | Effect when `false` |
|---|---|---|
| on-demand (expired token) | `time.Now()` read after the lock is acquired | Another caller already refreshed. Return the current session. |
| background | `time.Now() + lookahead` read after the lock is acquired | Another replica already refreshed. Make no upstream call. |
| sweep | `sweepStart + lookahead` (fixed per sweep) | Counted as `skipped`. |

`GetValidAccessToken` rejects recorded rejection before its valid-token branch. Otherwise, `AccessTokenExpiresBy(now + lookahead) && CanRefresh()` permits background submission. `CanRefresh()` means ciphertext exists without recorded rejection, not proven provider validity.

**Rationale**:
- One predicate expresses "due for refresh at threshold T" for every trigger. The current inline
  `HasValidAccessToken()` check in the lock callback becomes `AccessTokenExpiresBy(now)`, which is
  equivalent by construction (`!exp.After(now)` ⇔ `!now.Before(exp)`).
- NULL access expiry returns `false` for FR-013. Expiry is unknown, not permanently valid. Recorded rejection still prevents issuance.
- `CanRefresh()` prevents background submission without refresh authority. Missing ciphertext permits an otherwise valid access token until its recorded expiry. Recorded rejection instead requires immediate reauthentication, even before access expiry.

**Alternatives considered**: Use a separate `NeedsProactiveRefresh(lookahead)` that reads
`time.Now()` internally. It is not testable at the boundary, and the sweep needs a fixed threshold.
Rejected.

## R6 — Bounded background pool, in-flight dedup, and singleflight interplay (FR-002, FR-003, SC-003)

**Decision**: Add an unexported `backgroundRefresher` owned by `OAuth2SessionService`:

- `slots chan struct{}` with capacity `token_refresh.background_workers`. Acquisition is a
  non-blocking `select`. When no slot is free, the submission is **dropped**: WARN log
  (NFR-002) and `proactive_refresh_dropped_total`.
- `inflight sync.Map` keyed by `principal|serviceID`. `LoadOrStore` runs **before** slot acquisition.
  A key already in flight returns at once with no slot consumed, no counter, and a DEBUG log only.
- An accepted submission enters the shared flight after acquiring a slot.
  The flight calls `refreshDueSession(bgCtx, principal, serviceID, backgroundDue, RefreshTriggerBackground)`.
  `backgroundDue` computes current time plus lookahead under the lock.
- It shares `refreshGroup` (the existing singleflight) with the on-demand path. An on-demand caller
  whose token expires while a background flight is running joins that flight instead of starting a
  second upstream call.

**Rationale**:
- Without the in-flight guard, a hot session would stall the pool. An agent making N exchanges per
  second during the five-minute window would submit N times. Each submission would take a slot just
  to join the same singleflight, and refreshes for other sessions would be dropped. The guard makes a
  slot mean "one distinct session refreshing".
- Sharing `refreshGroup` gives FR-003 across both triggers. The `principal|serviceID` key is the
  session identity (`UNIQUE (principal, service_id)`), so it is equivalent to a per-session key.
- Goroutines are spawned per accepted submission and bounded by `slots`. No long-lived worker
  goroutines idle.
- Correctness does not depend on the singleflight. The row lock plus the `AccessTokenExpiresBy`
  re-check (R1, R5) prevents duplicate upstream calls even when two flights race. Singleflight and
  the in-flight map are latency and resource optimizations.

**Flight result shape**: The flight returns `refreshFlightResult{session, refreshed, trigger, client}`. The session contains encrypted tokens, never the decrypted token. Each on-demand joiner checks rejection before decrypting its own copy. The `refreshed` flag prevents another background refresh of a newly issued short-lived token.

A background joiner retries only after an on-demand no-op that remains due and is not rejected. A rejected shared result causes no metadata lookup or locked follow-up. This narrows SR-003 exposure. The shared value carries only the provider's known/public boolean, never decrypted provider credentials.

**Known operational behavior**:
- Transient provider errors permit another background attempt after the previous attempt finishes. At most one runs per session per replica. Backoff is outside this feature. Transient errors and dropped work leave session state unchanged and permit an otherwise valid access token. A committed terminal rejection instead prevents future issuance and refresh attempts. It cannot recall a token already returned by the current exchange.
- If a provider issues access tokens shorter than the lookahead, every exchange falls inside the
  window and refreshes continuously. The spec Assumptions direct operators to lower
  `lookahead_duration` for such providers. The quickstart and `docs/configuration.md` state this.

**Alternatives considered**:
- *`errgroup.SetLimit`*: `Go` blocks when the group is full, but FR-002 requires drop-on-saturation.
  `TryGo` drops correctly but cannot tie a dropped key back to the in-flight map without the same
  bookkeeping. A plain channel semaphore is the smaller construct.
- *`golang.org/x/sync/semaphore.Weighted.TryAcquire`*: This works too, but it is an extra API
  surface for a weight-1 semaphore. A channel is idiomatic and easy to inspect. Rejected as no
  better.
- *Fixed worker goroutines reading a buffered queue*: A queue holds stale work. By the time a queued
  item runs, the token may already be refreshed, and the queue depth is a second tuning knob.
  Rejected.

## R7 — Background context, shutdown, and refresh-token rotation (SR-003)

**Decision**:
- `backgroundRefresher` creates `baseCtx, cancel := context.WithCancel(context.Background())` when
  it is constructed. Each flight runs under `s.refreshOperationContext(baseCtx)`, the existing
  timeout budget (`service.go:1407-1414`). It never uses the request context, which is cancelled as
  soon as the exchange response is written.
- The flight context carries a trace **link** to the triggering request span
  (`trace.WithLinks(trace.LinkFromContext(reqCtx))`) on a new root span named
  `oauth2session.background_refresh`. Background work becomes traceable without being parented
  under a request that has already ended. The span records no token material. When telemetry is
  disabled, this is a no-op through the global no-op tracer.
- `OAuth2SessionService.Close(ctx context.Context) error` is **idempotent** (`sync.Once`). It
  (1) stops accepting submissions, so later submits are dropped as `stopped` without counting as
  `dropped`, (2) waits for in-flight flights until `ctx` is done, and only then (3) cancels `baseCtx`
  and returns `ctx.Err()`.
- The builder chains `Close` into `App.Shutdown` after the `SessionCleanup` stop and before
  `transport.CloseIdleConnections()` and telemetry shutdown. It follows the existing chain pattern
  (`internal/app/builder.go:1175-1207`).

**Rationale**:
- SR-003 requires that background goroutines respect server shutdown. Cancelling at once is unsafe
  for OAuth2 refresh. If the provider rotated the refresh token and the context is cancelled before
  the locked `UPDATE` commits, the only valid refresh token is lost and the user must
  re-authenticate. The existing on-demand flight already uses a detached, time-bounded context for
  this reason (`context.WithoutCancel` at `service.go:1334`). "Drain within the shutdown deadline,
  then cancel" respects cancellation and avoids self-inflicted session loss in the normal case.
- HTTP servers drain first (`cmd/agentic-identity-broker/root.go:214-277`), then `App.Shutdown`
  runs. No new submissions arrive while the pool drains.
- Idempotency is required. E2E tests share one `App` between two `TestServer`s, and each server's
  `Close` calls `App.Shutdown` (`tests/e2e/bootstrap/test_server.go:561-570`).

## R8 — Sweep execution model and the admin server write timeout (FR-006..FR-011, API-004)

**Decision**:
- The sweep runs synchronously in the request. It processes candidates sequentially in access-expiry, then UUID-byte order, one page at a time. Both adapters exclude known rejected rows before selection.
- Before it starts, the handler lifts the per-request write deadline with
  `http.NewResponseController(w).SetWriteDeadline(time.Time{})`. The caller owns the overall timeout
  (the CronJob's HTTP client timeout and the admin proxy timeout), which matches the spec's
  delegation of coordination to the external trigger.
- Request-context cancellation (client disconnect or server shutdown) is checked **between**
  sessions. Each per-session refresh runs under
  `refreshOperationContext(context.WithoutCancel(ctx))`, so a disconnect can never interrupt a refresh
  between the upstream response and the persisting `UPDATE` (R7 rotation hazard).
- The sweep does not use `refreshGroup`. It calls the locked refresh directly. A no-longer-due row counts as skipped. A row rejected after listing also counts as skipped without another token request. A newly observed rejection commits the flag and counts as failed once. Later sweeps exclude it. Due rows without refresh credentials remain failures on later sweeps. Dry runs skip rejected stale snapshots without a lock or provider call.

**Rationale**:
- The admin server applies a global `WriteTimeout: 15s` (`internal/adapters/http/server.go:189-194`).
  At about 300 ms per upstream refresh, the response would be cut off after about 50 sessions. A
  realistic window holds hundreds: 10k sessions with one-hour tokens and a five-minute window is
  about 830 candidates. Lifting the deadline only for this route is the narrowest fix and changes no
  other endpoint.
- **Prerequisite**: `http.ResponseController` reaches the underlying connection only if every
  wrapper exposes `Unwrap() http.ResponseWriter`. `LoggingMiddleware`'s `responseWriter`
  (`internal/adapters/http/middleware.go:107-116`) does not. Without `Unwrap`, `SetWriteDeadline`
  returns `http.ErrNotSupported` and the 15 s cut-off still applies. Adding `Unwrap` is a
  behavior-preserving Phase 0 refactor. Also check the otelchi and `tokenEndpointTelemetry` wrappers
  on the admin chain, and add a test that drives a sweep through `NewHandler` on a real `http.Server`
  with a short `WriteTimeout` to prove it.
  `responseWriterWrapper` in `internal/adapters/http/middleware/oauth2_audit.go:61-64` is on the
  `/oauth2/*` chain only, so it is not on this route.
- Sequential processing bounds load on upstream providers (rate limits) and on the database. Each
  locked refresh holds one connection for the whole upstream call (R9). A sweep throughput of about
  1/latency sessions per second is documented as an operational characteristic.
- `bypass refreshGroup`: singleflight results computed under a different threshold would misreport
  `skipped` versus `refreshed`. An on-demand flight with threshold `now` returns "not due" for a
  token that is inside the sweep window. The row lock gives correct, deterministic classification.

**Alternatives considered**:
- *A new `token_refresh.sweep.timeout` config and a server-side deadline*: This adds config surface
  the spec does not have, and a timed-out sweep has no defined API outcome. Rejected. A partial
  result needs an API field, which needs stakeholder confirmation (Principle X).
- *Asynchronous job API (`202` + status resource)*: This contradicts API-004 (`200` with a summary)
  and adds a persistence entity. Rejected.
- *Bounded concurrency within a page*: This multiplies database connections held across upstream
  calls and upstream request rate with no spec requirement. It can be added later behind a new
  configuration key if throughput requires. Deferred, not built.

## R9 — Configuration bounds (FR-012, Principle VII)

**Decision**: Add a new top-level `token_refresh` section (`ports.TokenRefreshConfig`). Defaults
are set in `Loader.setDefaults`, environment variables are bound with the `IDENTITY_BROKER_` prefix,
and `internal/config/validator.go` validates centrally:

| Key | Type | Default | Validation |
|---|---|---|---|
| `token_refresh.lookahead_duration` | Go duration | `5m` | `> 0` |
| `token_refresh.background_workers` | int | `10` | `1 ≤ n ≤ 20` |
| `token_refresh.sweep.default_page_size` | int | `100` | `1 ≤ n ≤ 1000` |

**Rationale**:
- `background_workers ≤ 20`: each in-flight background refresh holds one PostgreSQL connection for
  the length of the upstream call (`WithLockedSession` keeps the `FOR UPDATE` transaction open while
  the callback runs; `user_session.go:159-204`). The pool is fixed at `SetMaxOpenConns(25)`
  (`internal/adapters/storage/postgres/adapter.go:100`). A cap of 20 keeps at least five connections
  for request traffic, so background refresh cannot starve the latency-sensitive exchange path this
  feature exists to protect. The validator error message names this coupling.
- `default_page_size ≤ 1000`: this bounds session snapshots per page (FR-008). A per-sweep visited-ID set uses O(unique candidates) additional broker memory. The same page bound applies to the request body's `page_size` (R10).
- Nothing can disable proactive refresh. The spec defines none, and `background_workers ≥ 1` keeps
  the behavior always on.

## R10 — Sweep request body and ISO 8601 durations (API-002, API-003)

**Decision**:
- Request body fields are all optional: `lookahead_duration` (string, ISO 8601 duration),
  `dry_run` (boolean, default `false`), and `page_size` (integer, `1..1000`). An empty body and `{}`
  are equivalent. Unknown properties are rejected with `400`. The body is limited to 4 KiB with
  `http.MaxBytesReader` (precedent: `agents_handler.go:255`).
- `lookahead_duration` is parsed by a small, strict parser in the admin HTTP adapter (input parsing
  belongs at the boundary, per the constitution's "domain logic leakage" rule). Accepted grammar:
  `P[nD][T[nH][nM][n[.f]S]]`, with at least one component and non-negative integer components except
  fractional seconds. Years, months, and weeks are rejected, because they are calendar-relative and
  ambiguous as a fixed duration. Overflow beyond `time.Duration` is rejected. The parsed value must
  be `> 0`.
- The domain applies defaults. The handler passes the zero value for omitted fields, and
  `SessionSweepService` substitutes the configured defaults and validates bounds. The admin handler
  maps `ErrInvalidSweepRequest` to `400`.

**Rationale**:
- The implementation keeps its existing strict ISO 8601 parser and table-driven tests. It rounds nonzero fractional seconds up to nanoseconds and rejects overflow. Keeping the parser avoids an unrelated dependency cutover. Principle III covers cryptography and security primitives, not duration parsing. A third-party dependency would add supply-chain surface (`#192` dependency remediation) without benefit.
- Zalando guidelines require ISO 8601 for durations in APIs, which is why the API and the
  configuration use different syntaxes (Go duration in YAML, ISO 8601 on the wire).

**Alternatives considered**: `github.com/sosodev/duration` / `github.com/senseyeio/duration`. Both
accept years and months, which would have to be rejected anyway. Rejected.

## R11 — Failure classification, logging levels, and metrics (NFR-001..NFR-004, FR-010, FR-011)

**Decision**:

*Classification* reuses the existing `ErrorMetadata` on `OperationError`: `Kind()` plus
`Dependency()`, derived from `ErrorDetail` in `internal/domain/oauth2session/errors.go:155-174`.
The rows are evaluated in order, and the first match wins:

| Condition | Example details | Sweep effect | Sweep log | Background log |
|---|---|---|---|---|
| `DetailSessionMissing` under lock (deleted mid-refresh) | `session_missing` | `skipped++` | DEBUG | DEBUG |
| Already rejected under the lock | unchanged no-op, no new provider error | `skipped++` | DEBUG | DEBUG |
| `KindCanceled` | `caller_canceled` | stop the sweep (client gone) | INFO | — |
| `KindInfrastructure` ∧ dependency ∈ {`session_repository`, `provider_repository`} | `repository_unavailable`, `persistence_failed` | **abort → 500** | ERROR | ERROR |
| `KindInfrastructure` ∧ dependency = `encryption` | `decryption_failed`, `encryption_failed` | `failed++`, continue | ERROR | ERROR |
| `KindInfrastructure` ∧ dependency = `provider` | `provider_unavailable` | `failed++`, continue | WARN | WARN |
| `KindSession` | `refresh_unavailable`, `access_token_expired` | `failed++`, continue | WARN | WARN |
| `KindProvider` | newly observed `refresh_rejected`, `provider_rejected`, `provider_response_invalid` | `failed++`, continue. Only terminal refresh rejection commits its flag. | WARN | WARN |
| `KindConfiguration` | `provider_client_rejected`, `configuration` | `failed++`, continue | WARN | WARN |
| `KindInternal` | `internal_unclassified` | `failed++`, continue | ERROR | ERROR |

Why it is not "abort on any `KindInfrastructure`":
- `DetailProviderUnavailable` is classified `KindInfrastructure` with `DependencyProvider`
  (`errors.go:162-163`). FR-010 and the spec edge cases make upstream errors per-session failures,
  so a provider outage MUST NOT abort the sweep.
- Encryption failures (`DependencyEncryption`) are logged at ERROR (NFR-003, infrastructure) but do
  **not** abort. The broker cannot tell a KMS outage from one undecryptable row. Aborting would let a
  single corrupt row wedge every future sweep at the same cursor position. During a KMS outage, each
  session fails fast, before any upstream call, so continuing costs little.
- Database failures abort. API-004 names "database unreachable" as the `500` case. Continuing is
  pointless, because every following lock acquisition fails the same way. `ListExpiringSessions`
  failures abort for the same reason.

The on-demand path keeps its current log levels. Failure logging moves out of
`refreshSessionTokens` (`service.go:1445-1451`, today always ERROR) into the trigger-aware caller,
so NFR-003's WARN for per-session sweep failures is met without double-logging.

*Success event* (NFR-001, `SessionTokensRefreshed`): extend the existing
`event=session.oauth2.token_refreshed` INFO line with `triggered_by` (`background` | `on-demand` |
`sweep`), `session_id`, and the session owner's `principal`. Log it only after commit.
`ForceRefreshSession` (the end-user `POST /api/third-party/{serviceId}/session/refresh`) reports `triggered_by=on-demand`. No token material appears in the event.

A rejection-state commit is not a successful token refresh. Return `refreshed=false`, log the failure, and do not emit `session.oauth2.token_refreshed` or increment a refresh-success counter. Later recorded-rejection errors contain no fabricated provider status or code. Manual refresh returns HTTP 409 `refresh_rejected` with `refresh token rejected by provider; reauthentication required`. Missing refresh credentials remain HTTP 409 `refresh_unavailable`. Non-terminal upstream errors remain HTTP 502 `refresh_failed`.

*Metrics* (NFR-004): five monotonic `metric.Int64Counter` instruments are created once in the
constructors from `otel.Meter("oauth2session")`, following the broker precedent of instruments
created from the global provider in constructors (`internal/domain/approval/service.go:122-146`,
ADR 011). Each carries exactly one attribute, `triggered_by`, so cardinality is fixed. Dry-run
sweeps emit no counters, because nothing was refreshed. See
[contracts/observability.md](contracts/observability.md).

*Sweep audit*: the admin handler emits INFO `session sweep` with `operator_principal`,
`dry_run`, `lookahead`, `page_size`, the four counts, `duration_ms`, and `outcome`. This follows
the CIMD key audit pattern (`cimd_client_keys_handler.go:152-163`). An operator principal is
required, and its absence is a `500 server_misconfiguration`, as in the precedent.

**Rationale**: `ErrorKind` already exists as the bounded, credential-free classification used by
the telemetry contract (`#186 refactor(otel): classification contract`). A parallel taxonomy would
duplicate it.

## R12 — E2E strategy and test seams (Principle XIII)

**Decision**:
- Create a new file `tests/e2e/proactive_token_refresh_test.go`. It mirrors the `token_exchange_test.go`
  setup (`:44-157`): production `ServerFactory.BuildApp`, fresh in-memory storage, signed JWT
  fixtures, and `helpers.MockUpstreamOAuth2Server`.
- Add one test helper, `(*MockUpstreamOAuth2Server).WithTokenResponseGate() (release func())`, in
  `tests/e2e/helpers/mock_upstream.go`. The token endpoint records the request and then blocks until
  `release` is called (or the request is cancelled). This gives US1-S2 and US1-S3 a deterministic
  "response arrives before the refresh finishes" without timing heuristics. The existing
  `WithTokenHangUntilCanceled` has no release hook (`mock_upstream.go:189-199`).
- Expiry offsets are set well inside or outside the window (+2 min and +2 h against the 5 min
  default), so no injectable clock is needed (WiringScout #8). `config.TokenRefresh.LookaheadDuration`
  stays at the default.
- US3-S3 (concurrent replicas) builds **two** `App`s from one shared `testStorage` and fires the
  sweeps concurrently at both admin servers.
- US1-S1 uses a one-slot pool with a gated in-flight refresh. A second due exchange returns its valid token and emits a dropped-refresh WARN event. A healthy exchange still makes no upstream call. The baseline has no pool or drop event.
- US2-S1 checks `session_id` and `triggered_by=on-demand` on INFO success after commit. US2-S2 uses an expired access token without refresh credentials. US2-S3 checks one ERROR failure event and retained ciphertext after a new provider rejection.
- US2-S3 also covers the real HTTP recovery lifecycle. A forced refresh of an unexpired one-hour access token returns 409 `refresh_rejected`. The retained list entry reports reauthentication required, unexpired access, and refresh ciphertext present. The next exchange returns 400 `invalid_grant`, a `/sessions` recovery URI, and no token without another provider request. A `PT2H` sweep evaluates zero rows even though the unchanged access expiry falls within its threshold. A real authorize/provider/callback flow clears rejection through upsert. The next exchange succeeds after one `authorization_code` grant.
- The recovery context uses `bootstrap.NewTestServerBuilder(...).Build()` so the real callback URL exists before `app.Builder.Build()`. The separate admin listener uses `bootstrap.NewAdminTestServer(enduser.App(), logger)`. Redirects are followed explicitly. Broker authentication headers never reach the provider.
- Capture structured events with `bootstrap.NewBufferedJSONLogger`. The original ten cases compiled and failed semantically before proactive implementation. The later validity regression forces rejection before access expiry, then asserts that acquisition returns no token. It fails behaviorally under the obsolete model, not at compilation.

**Rationale**: The ten scenario identifiers remain stable. Rejection coverage extends US2-S3 with the full recovery journey. US3-S2 preserves repeated missing-credential failures. Repository and domain tests cover known rejection exclusion, stale rejection skips, and newly observed rejection failing once.

## R13 — ADR

**Decision**: Write `adrs/038-proactive-token-refresh.md` (Status: Accepted, before
implementation). It records:
1. Lookahead-triggered in-process background refresh with a bounded, drop-on-saturation pool and
   per-process deduplication. There is no cross-replica coordinator, and the row lock is the
   cross-replica correctness mechanism.
2. The externally scheduled admin sweep. There is no internal scheduler and no distributed lock.
   The sweep is synchronous, with a per-route write-deadline lift and caller-owned timeout (R8).
3. "Drain, then cancel" shutdown, motivated by refresh-token rotation (R7).
4. The rejected alternatives: internal ticker scheduler, distributed lease, async job API, and a new
   `UpdateTokensIfExists` write path.

ADR 038 records the original proactive-refresh design. ADR 040 narrowly supersedes its availability consequence for terminal provider rejection. Dropped work and transient background errors still permit an otherwise valid access token.

**Rationale**: These are binding architectural choices that later features (for example, any
internal scheduler) would have to supersede. Principle II requires them to be recorded.
