# Contract: Ports and Domain Service Interfaces

**Feature**: `029-proactive-token-refresh` | Binding for implementation; signatures are final unless
this file is amended.

## 1. New driven port — `UserSessionExpiryRepository`

**File**: `internal/ports/storage.go`, placed directly after `UserSessionRefreshRepository` (research R2).

`SessionExpiryCursor` is defined in `internal/domain/storage/user_session.go`. The port uses that value type and does not add a second expiry-read method.

```go
// SessionExpiryCursor is the last returned (expiry, ID) tuple.
// A zero ID and zero expiry start the scan. An expiry without an ID is invalid.
// A cursor with an ID continues after its (expiry, ID) tuple, including a zero expiry.
type SessionExpiryCursor struct {
    AccessTokenExpiresAt time.Time
    ID                   id.SessionID
}

// UserSessionExpiryRepository lists sessions whose access tokens expire at or before a threshold.
// Sessions without recorded access expiry or with recorded refresh rejection are never returned.
type UserSessionExpiryRepository interface {
    // ListExpiringSessions returns at most limit full encrypted sessions in ascending
    // (access_token_expires_at, id) order, strictly after a nonzero cursor.
    // It returns an empty slice when no rows match and rejects an expiry without an ID or a limit outside 1..1000.
    ListExpiringSessions(ctx context.Context, threshold time.Time, cursor storage.SessionExpiryCursor, limit int) ([]*storage.UserSession, error)
}
```

**Behavioral contract** (both adapters MUST pass the same table-driven suite):

| # | Given | Then |
|---|---|---|
| E1 | rows expiring at `t-1h`, `t`, `t+1s`, `NULL` | `threshold=t` returns the `t-1h` and `t` rows only (boundary inclusive; `NULL` excluded) |
| E2 | five due rows at unequal and equal expiries, `limit=2`, zero-time/zero-ID cursor | pages `[2,2,1]` in expiry-then-UUID order; concatenation equals one `limit=1000` call without gaps or duplicate tuples |
| E3 | an earlier row is refreshed after the threshold or deleted between pages; another refreshed row moves forward but stays due | remaining candidates ahead of the tuple cursor are returned without gaps; the moved row can reappear but the sweep evaluates its ID only once |
| E4 | a new due row is inserted behind the tuple cursor mid-traversal | the new row is not returned; a new due row ahead of the cursor remains eligible |
| E5 | `limit` ∉ 1..1000, or a cursor expiry without an ID | validation error. No query is executed. |
| E6 | returned sessions | full access and refresh token ciphertext snapshots remain encrypted (storage contract unchanged) |
| E7 | differing expiry values and equal-expiry UUIDs | the memory adapter's order matches PostgreSQL `ORDER BY access_token_expires_at, id`; ties compare the 16 UUID bytes |
| E8 | a due row with zero expiry followed by a later expiry, `limit=1` | the second page continues after the zero-expiry tuple. The following page is empty. |
| E9 | due rows with recorded refresh rejection | Both adapters exclude them before candidate selection. No ciphertext is removed. |

**PostgreSQL specifics**: Read explicit columns from the stable `user_session_state` view at phases 38, 39, and 40. The first page uses `NOT refresh_token_rejected AND access_token_expires_at IS NOT NULL AND access_token_expires_at <= $1 ORDER BY access_token_expires_at, id LIMIT $2`. Later pages add `(access_token_expires_at, id) > ($2, $3)` with `LIMIT $4`. Both queries use the adapter read timeout (`r.adapter.timeouts.Read`) and wrap errors with the existing `wrapError`. Use sqlx `SelectContext` into `[]userSessionRecord`, then `recordToSession`. The SC-006 `EXPLAIN` checks these production queries against the valid B-tree `idx_user_sessions_access_token_expires_at` on `user_sessions (access_token_expires_at, id)` without disabling sequential scans. Migration 037 and its index design remain unchanged.

**Factory**: `internal/adapters/storage/factory.go` gains `SessionExpiry() ports.UserSessionExpiryRepository`.
It returns the same instance as `UserSessions()` and `SessionRefresh()`. A compile-time assertion
`var _ ports.UserSessionExpiryRepository = (*PostgresUserSessionRepository)(nil)` (and the memory
equivalent) guards the implementation.

## 2. Existing driven port — `UserSessionRefreshRepository` (usage contract, unchanged signature)

```go
WithLockedSession(ctx context.Context, principal id.Principal, serviceID id.ServiceID,
    refresh func(context.Context, *storage.UserSession) (bool, error)) (*storage.UserSession, error)
```

This feature relies on these existing guarantees. Tests assert them for both adapters; they are
already covered for PostgreSQL in `user_session_test.go` and must be added for memory where missing:

| # | Guarantee |
|---|---|
| L1 | The callback sees the latest committed row under an exclusive per-session lock. |
| L2 | A missing row returns `(nil, nil)` **without invoking the callback**, and nothing is inserted (DB-002, no resurrection). Deletes take the same lock (PostgreSQL row lock; memory per-key gate, `memory/user_session.go:185,213`), so a delete that races a refresh is serialized and never undone. Callers map `nil` to `ErrSessionNotFound`, which is `skipped` for the sweep and a no-op for background. |
| L3 | `(false, nil)` writes nothing. `(_, err)` writes nothing and returns `err` unchanged. This includes a callback that mutates its snapshot before returning an error. |
| L4 | `(true, nil)` atomically persists the encrypted access token, encrypted refresh token, `access_token_expires_at`, `refresh_token_rejected`, and `updated_at`. The boolean requests a session-state write, not proof of a successful token refresh. |

Terminal rejection does not relax L3. `refreshLockedSession` captures the provider error outside the callback return value. The callback sets `RefreshTokenRejected=true` and `UpdatedAt`, then returns `(true, nil)` so L4 commits. Ciphertext and other token metadata remain unchanged.

After commit, the service returns the captured rejection error with `ErrReauthenticationRequired` and `refreshed=false`. It emits no refresh-success event or metric. A write or commit error returns `persistence_failed`, without claiming durable rejection.

Only HTTP 4xx `invalid_grant`, excluding 429, takes this path. Transient, configuration, transport, malformed-response, encryption, and repository errors do not persist rejection. A known rejection wins before due-time, refresh-availability, and provider-metadata errors inside the lock. Already-rejected background or sweep work returns an unchanged no-op. Forced or on-demand work returns the rejection classification without another token request.

The PostgreSQL adapter holds `SELECT ... FOR UPDATE` through the stable view, locking the underlying original row for the callback and persistence. It calls the six-argument `update_user_session_state` function in that transaction. The function updates only an existing row. Initial authorization uses the 13-argument `upsert_user_session_state` function. Both functions use `SECURITY INVOKER`, and the PostgreSQL 15 view uses `security_invoker=true`. Principal and service filtering remain unchanged.

This PR ships migration 038 and the prepared adapter only. At 38, the view reports rejection=false and true writes fail as persistence errors. Ordinary acquisition, refresh, and authorization remain supported. Memory already supports rejection. Durable PostgreSQL rejection activates with PR2's migration 039. Complete PR1 and drain every unprepared replica and in-flight operation before merging or deploying PR2. PR3's migration 040 follows only after SPA/external consumers upgrade and legacy schema dependencies end. Neither later migration is present in PR1. Prepared/new adapters keep the stable boundary through the following phases, without schema detection, JSON conversion, SQL retries, or fabricated refresh dates.

## 3. Domain service — `OAuth2SessionService` (changes)

**File**: `internal/domain/oauth2session/service.go` (+ `background_refresh.go`, `refresh_trigger.go`)

| Member | Change |
|---|---|
| `Config` | Add `RefreshLookahead time.Duration`, `BackgroundRefreshWorkers int`, `SweepDefaultPageSize int`. `NewOAuth2SessionService` applies defaults `5m`, `10`, `100` when zero, following the existing zero-value pattern at `service.go:137-148`. |
| `NewConfigFromPorts` | New signature `NewConfigFromPorts(portsCfg ports.ThirdPartyOAuth2Config, refreshCfg ports.TokenRefreshConfig, callbackBaseURL string) Config`. Migrate every caller. |
| `NewOAuth2SessionService` | Constructs the `backgroundRefresher` and the three proactive counters. Same parameter list. |
| `GetValidAccessToken` | Same signature. Reject recorded rejection before the valid-token branch. Otherwise, submit background work when `AccessTokenExpiresBy(now+lookahead) && CanRefresh()`, then return the current token. The expired branch uses the shared `refreshGroup` flight. Reject a shared result containing recorded rejection before decryption. The result carries encrypted session state, committed-refresh status, and trigger. A background joiner retries only after an on-demand no-op that remains due and is not rejected. |
| `refreshDueSession` / `refreshLockedSession` | The due wrapper calls `refreshLockedSession(ctx, principal, serviceID, dueBy func() time.Time, trigger RefreshTrigger)`. A nil `dueBy` forces refresh regardless of access expiry, but never bypasses rejection. Provider metadata stays outside the lock. Under the lock, rejection wins before due-time and provider-error decisions. Terminal rejection commits through `(true, nil)` before the saved error returns. `ForceRefreshSession` uses the same path and trigger-aware logs. |
| `logRefreshSuccess` / `logRefreshFailure` | Take the known owner as `principal id.Principal` along with `sessionID id.SessionID` and `trigger RefreshTrigger`. Emit `principal`, `session_id`, and `triggered_by` on per-session refresh events. Success logs only after the locked update commits (NFR-001). |
| `Close(ctx context.Context) error` | **New**, idempotent. Drains background flights until `ctx` is done, then cancels them (research R7). |

`tokenexchange.TokenExchangeService` is unchanged. Its consumer-side interface still calls
`GetValidAccessToken(ctx, principal, serviceID)`, so the proactive behavior is invisible to it.

## 4. Domain service — `SessionSweepService` (new)

**File**: `internal/domain/oauth2session/sweep.go`

```go
var ErrInvalidSweepRequest = errors.New("invalid session sweep request")

type SweepRequest struct {
    Lookahead time.Duration // 0 → configured default
    DryRun    bool
    PageSize  int           // 0 → configured default
}

type SweepResult struct {
    Refreshed, Skipped, Failed, TotalEvaluated int
    DryRun                                     bool
}

type SessionSweepService struct { /* expiry ports.UserSessionExpiryRepository; sessions *OAuth2SessionService; counters; logger */ }

func NewSessionSweepService(expiry ports.UserSessionExpiryRepository, sessions *OAuth2SessionService, logger *slog.Logger) *SessionSweepService

// Sweep evaluates every session whose access token expires at or before
// time.Now()+effective lookahead, in keyset pages.
// Returns ErrInvalidSweepRequest (wrapped, with a client-safe message) for invalid input.
// Returns a non-nil error only for invalid input, caller cancellation, or an aborting
// infrastructure failure (research R11); per-session failures are counted, not returned.
func (s *SessionSweepService) Sweep(ctx context.Context, req SweepRequest) (SweepResult, error)
```

The sweep advances the `storage.SessionExpiryCursor` from the last raw page row before processing the next page, even when all rows were seen before. It rejects nil expiry or non-increasing tuples from the read port as repository failures. A per-invocation `map[id.SessionID]struct{}` tracks unique evaluated candidates for both real and dry runs. It skips later occurrences without another lock, upstream call, metric, log, or count. The broker holds at most `page_size` session snapshots per page and O(unique candidates) visited IDs. The sweep stops only after a short page, cancellation, or an aborting error.

The sweep invokes `sessions.refreshDueSession(refreshOperationContext(context.WithoutCancel(ctx)), …, sweepDue, RefreshTriggerSweep)` per new candidate, where `sweepDue` returns the fixed start threshold. It does **not** use `refreshGroup` (research R8). It checks `ctx.Err()` before each candidate. It owns the counters `session_sweep_refreshed_total` and `session_sweep_failed_total`.

Known rejected rows never enter the repository candidate pages. A rejection after listing makes the candidate stale: real sweeps re-check under the lock and skip it. Dry runs skip rejected listed snapshots without a lock. Newly observed provider rejection counts as failed once after commit. Later sweeps exclude it. Due rows without refresh credentials still fail on each sweep. Transient errors leave the row unchanged and permit an otherwise valid access token.

**Error contract for callers**:

| Returned error | Handler status |
|---|---|
| `errors.Is(err, ErrInvalidSweepRequest)` | `400`, message = the wrapped client-safe text |
| `ctx.Err() != nil` (client gone) | none written; log INFO `outcome=canceled` |
| any other error | `500 internal server error` |

## 5. Driving adapter — `SessionSweepHandler` (new)

**File**: `internal/adapters/http/handlers/admin/session_sweep_handler.go`

```go
type sessionSweeper interface {
    Sweep(ctx context.Context, req oauth2session.SweepRequest) (oauth2session.SweepResult, error)
}

func NewSessionSweepHandler(sweeper sessionSweeper, logger *slog.Logger) *SessionSweepHandler
func (h *SessionSweepHandler) Sweep(w http.ResponseWriter, r *http.Request)
```

Responsibilities (input parsing only; no domain decisions):
1. `operatorPrincipalFromContext`. If it is absent, return `500 server_misconfiguration` (CIMD precedent).
2. `http.MaxBytesReader(w, r.Body, 4<<10)`. Decode with `DisallowUnknownFields`; `io.EOF` means an empty body.
3. Parse `lookahead_duration` with `parseISO8601Duration` (same package, unexported, table-tested; research R10).
4. Call `http.NewResponseController(w).SetWriteDeadline(time.Time{})`. On error, log WARN once and
   continue. The response is then subject to the server `WriteTimeout`. The Phase 0 `Unwrap`
   refactor makes this path unreachable in production, and a test proves it.
5. Call `Sweep`, map errors per §4, and write `200` with the snake_case JSON body.
6. Emit the INFO audit line (research R11).

**Wiring**: `app.AdminHandlers.SessionSweep *admin.SessionSweepHandler` is built in `Builder.Build()`
from `oauth2session.NewSessionSweepService(b.storage.SessionExpiry(), app.OAuth2SessionService, b.logger)`.
`routing.SetupAdminRoutes` registers `r.Post("/sessions/sweep", h.SessionSweep.Sweep)` inside the
existing `/api` group, so it inherits `RequireAdminHost`, `CrossOriginProtection`, `RequireAdminJSON`,
and CORS. The route is registered unconditionally, because the feature has no off switch.

## 6. Test-only helper (not a production API)

**File**: `tests/e2e/helpers/mock_upstream.go`

```go
// WithTokenResponseGate makes the token endpoint record each request and then block until release
// is called or the request context ends. release is idempotent.
func (m *MockUpstreamOAuth2Server) WithTokenResponseGate() (release func())
```
