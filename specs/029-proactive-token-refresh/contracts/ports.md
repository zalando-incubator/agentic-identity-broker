# Contract: Ports and Domain Service Interfaces

**Feature**: `029-proactive-token-refresh` | Binding for implementation; signatures are final unless
this file is amended.

## 1. New driven port — `UserSessionExpiryRepository`

**File**: `internal/ports/storage.go`, placed directly after `UserSessionRefreshRepository` (research R2).

```go
// UserSessionExpiryRepository lists sessions whose access tokens expire at or before a threshold,
// for the admin session sweep. Sessions without an access-token expiry are never returned.
type UserSessionExpiryRepository interface {
    // ListExpiringSessions returns up to limit sessions with
    // access_token_expires_at IS NOT NULL AND access_token_expires_at <= threshold AND id > cursor,
    // ordered by id ascending. A zero cursor starts from the beginning.
    // Returns an empty slice (not an error) when no rows match.
    // Errors are domain StorageErrors; limit must be in 1..1000.
    ListExpiringSessions(ctx context.Context, threshold time.Time, cursor id.SessionID, limit int) ([]*storage.UserSession, error)
}
```

**Behavioral contract** (both adapters MUST pass the same table-driven suite):

| # | Given | Then |
|---|---|---|
| E1 | rows expiring at `t-1h`, `t`, `t+1s`, `NULL` | `threshold=t` returns the `t-1h` and `t` rows only (boundary inclusive; `NULL` excluded) |
| E2 | 5 matching rows, `limit=2` | pages `[2,2,1]`; concatenation equals a single `limit=1000` call; no duplicates, no gaps |
| E3 | a returned row is updated to expire after the threshold before the next page | it is not returned again; rows after the cursor are unaffected |
| E4 | a row with id < cursor is inserted mid-traversal | it is not returned (keyset monotonicity) |
| E5 | `limit` ∉ 1..1000 | validation error; no query executed |
| E6 | returned sessions | tokens remain ciphertext (storage contract unchanged) |
| E7 | ordering | the memory adapter's order equals the PostgreSQL `ORDER BY id` order for the same UUID set (`bytes.Compare` on the 16 bytes) |

**PostgreSQL specifics**: use the adapter read timeout (`r.adapter.timeouts.Read`). Wrap errors with
the existing `wrapError`. Use sqlx `SelectContext` into `[]userSessionRecord`, then `recordToSession`.
The query text is the one in research R3, so the SC-006 `EXPLAIN` test exercises the production
statement. Expose it as an unexported package-level `const` that the test imports.

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
| L3 | `(false, nil)` from the callback writes nothing. `(_, err)` writes nothing and returns `err` unchanged (no partial state, US2-S3). |
| L4 | `(true, nil)` persists the encrypted access token, encrypted refresh token, `access_token_expires_at`, and `updated_at` atomically. |

## 3. Domain service — `OAuth2SessionService` (changes)

**File**: `internal/domain/oauth2session/service.go` (+ `background_refresh.go`, `refresh_trigger.go`)

| Member | Change |
|---|---|
| `Config` | Add `RefreshLookahead time.Duration`, `BackgroundRefreshWorkers int`, `SweepDefaultPageSize int`. `NewOAuth2SessionService` applies defaults `5m`, `10`, `100` when zero, following the existing zero-value pattern at `service.go:137-148`. |
| `NewConfigFromPorts` | New signature `NewConfigFromPorts(portsCfg ports.ThirdPartyOAuth2Config, refreshCfg ports.TokenRefreshConfig, callbackBaseURL string) Config`. Migrate every caller. |
| `NewOAuth2SessionService` | Constructs the `backgroundRefresher` and the three proactive counters. Same parameter list. |
| `GetValidAccessToken` | Same signature. After the valid-token branch: if `session.AccessTokenExpiresBy(now+lookahead) && session.CanRefresh()`, call `submit` (non-blocking), then return the current token. The expired branch calls `refreshDueSession(…, time.Now(), RefreshTriggerOnDemand)` inside the shared `refreshGroup` flight. The internal result carries the encrypted session, whether the leader committed a refresh, and its trigger; each caller decrypts its own token. A background joiner retries under the lock only after an on-demand no-op that remains due within lookahead. |
| `refreshExpiredSession` | **Replaced by** `refreshDueSession(ctx, principal, serviceID, threshold time.Time, trigger RefreshTrigger) (*storage.UserSession, bool /*refreshed*/, *model.ThirdpartyOAuth2ProviderEntity, error)`. `refreshLockedSession` performs the single locked update. The due wrapper preserves the provider already fetched for `public_client` failure logging without another lookup. `ForceRefreshSession` keeps its unconditional semantics and routes logging through the same trigger-aware helper. |
| `logRefreshSuccess` | Gains `sessionID id.SessionID` and `trigger RefreshTrigger`. Emits `session_id` and `triggered_by` (NFR-001). |
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

The sweep invokes `sessions.refreshDueSession(refreshOperationContext(context.WithoutCancel(ctx)), …, threshold, RefreshTriggerSweep)`
per candidate. It does **not** use `refreshGroup` (research R8). It checks `ctx.Err()` before each
candidate. It owns the counters `session_sweep_refreshed_total` and `session_sweep_failed_total`.

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
