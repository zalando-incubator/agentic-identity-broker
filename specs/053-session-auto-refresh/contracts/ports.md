# Contract: Ports, domain services, and handler changes

**Feature**: `053-session-auto-refresh` | DB-001 to DB-004, FR-017 to FR-024 | Research R9, R11 to R13

## 1. New port: `UserSessionHealthRepository` (`internal/ports/storage.go`)

```go
// UserSessionHealthRepository counts stored sessions for session-health metrics.
// It reads metadata columns only and never returns or decrypts token material.
type UserSessionHealthRepository interface {
	// CountSessionsByExpiry groups every stored session by service, by access-token expiry
	// relative to now and threshold (after threshold; due when after now and at or before
	// threshold; expired when at or before now; unknown when not recorded), and by whether the
	// refresh token is present and unexpired at now. Groups with no sessions are omitted.
	// The caller bounds the query through ctx. Failures return domain storage.StorageError values.
	CountSessionsByExpiry(ctx context.Context, threshold, now time.Time) ([]storage.SessionExpiryGroup, error)
}
```

| Adapter | Implementation |
|---|---|
| PostgreSQL (`internal/adapters/storage/postgres/user_session.go`) | The single `GROUP BY` query from research R9, run with `SelectContext` under the caller's context, which `token_refresh.health_metrics.query_timeout` bounds; `storage.timeouts.read` does not apply. Errors go through `wrapError`. A `CASE` label outside the four known values is an `ErrorKindUnknown` storage error. |
| Memory (`internal/adapters/storage/memory/user_session.go`) | Under `RLock`, apply the same predicates to each stored session with the supplied `threshold` and `now`. Return `ErrorKindTimeout` when `ctx.Err() != nil`. |

Compile-time assertions `var _ ports.UserSessionHealthRepository = (*...)(nil)` go in both adapters. `storage.Adapter` (`internal/adapters/storage/factory.go`) gains a `SessionHealth()` accessor, wired for both backends.

### Parity scenarios (same cases in both adapter test files)

| ID | Setup | Expected groups |
|---|---|---|
| H1 | no sessions | empty slice, no error |
| H2 | one session, access expiry after threshold, refresh usable | `{svc, AfterThreshold, true, 1}` |
| H3 | access expiry exactly at threshold | `AccessExpiry = Due` |
| H4 | access expiry nil | `AccessExpiry = Unknown` |
| H5 | due, refresh token present, refresh expiry nil | `RefreshUsable = true` |
| H6 | due, refresh token present, refresh expiry at `now` | `RefreshUsable = false` |
| H7 | due, refresh token absent (`NULL` / empty) | `RefreshUsable = false` |
| H8 | two services, mixed classes, two sessions in one group | one group per distinct key, counts summed |
| H9 | canceled context | timeout-kind storage error |
| H10 | access expiry equal to now | `expired` |
| H11 | access expiry in the past, usable refresh token | `expired`, refresh usable |

PostgreSQL cases run under `//go:build integration` with `setupMigratedAdapter` and `insertTestService` (`just test-integration-infra`).

## 2. New domain component: `SessionHealthMetrics` (`internal/domain/oauth2session/health_metrics.go`)

```go
type SessionHealthExpiry string

const (
	SessionHealthExpiryValid    SessionHealthExpiry = "valid"
	SessionHealthExpiryExpiring SessionHealthExpiry = "expiring"
	SessionHealthExpiryExpired  SessionHealthExpiry = "expired"
	SessionHealthExpiryUnknown  SessionHealthExpiry = "unknown"
)

// SessionHealthKey identifies one oauth2_sessions series of a service.
type SessionHealthKey struct {
	Expiry      SessionHealthExpiry
	Refreshable bool
}

// ClassifySessionHealth maps a query group to its gauge series (FR-018).
func ClassifySessionHealth(group storage.SessionExpiryGroup) (SessionHealthKey, error)

// NewSessionHealthMetrics registers the oauth2_sessions observable gauge on the global meter.
func NewSessionHealthMetrics(
	repo ports.UserSessionHealthRepository,
	lookahead, queryTimeout time.Duration,
	logger *slog.Logger,
) (*SessionHealthMetrics, error)

// Close unregisters the gauge callback. It is safe to call more than once.
func (m *SessionHealthMetrics) Close() error
```

Callback behavior:
1. `now := m.now()`, `threshold := now.Add(lookahead)`.
2. Run `CountSessionsByExpiry` under `context.WithTimeout(ctx, queryTimeout)`.
3. On error, or if a group fails classification, observe nothing. Log `session health query failed` at WARN with `error_kind` (the storage error kind) and return `nil`.
4. Otherwise, build the snapshot, zero-fill all eight `(expiry, refreshable)` combinations for every service present, and observe one point per `(service_id, expiry, refreshable)` with exactly those three attributes.

Unit tests (`health_metrics_test.go`, ManualReader pattern from `sweep_test.go:262-266`):
- classification table for all eight group shapes
- zero-filling
- attribute set is exactly `{service_id, expiry, refreshable}`
- error yields no points and one WARN record
- timeout is applied: a blocking fake repo returns after `queryTimeout`
- `Close` stops further callbacks

## 3. Builder wiring (`internal/app/builder.go`)

- After the storage adapter and before handlers: when `b.config.TokenRefresh.HealthMetrics.Enabled`, construct `NewSessionHealthMetrics(b.storage.SessionHealth(), b.config.TokenRefresh.LookaheadDuration, b.config.TokenRefresh.HealthMetrics.QueryTimeout, b.logger)`.
- Add `healthMetrics.Close()` to the shutdown chain beside `OAuth2SessionService.Close` (`builder.go:1185-1193`), so it runs while telemetry is still live.
- Builder failure paths created after construction call `Close`.

## 4. Sweep service (`internal/domain/oauth2session/sweep.go`)

- Add `skippedCounter metric.Int64Counter`, named `session_sweep_skipped_total` (unit `{refresh}`, description "Admin sweep candidates skipped because they were already refreshed or deleted"), created beside the existing counters at `:55-65`.
- Call `s.skippedCounter.Add(ctx, 1, sweepMetricAttributes)` at both real-sweep `result.Skipped++` sites (deleted candidate, not refreshed). Dry runs do not count.
- No signature change.

## 5. Sweep handler (`internal/adapters/http/handlers/admin/session_sweep_handler.go`)

- `NewSessionSweepHandler` creates `session_sweep_duration_seconds`: `Float64Histogram`, unit `s`, description "Duration of admin session sweep requests by outcome", buckets `1, 5, 10, 30, 60, 120, 300, 600, 1200, 1800, 3600`, on `otel.Meter("oauth2session")`.
- In the existing deferred block, after the audit log, record `time.Since(started).Seconds()` with the attributes `outcome` and `dry_run` (`strconv.FormatBool(request.DryRun)`).
- The missing-principal early return stays before `started`, so it records nothing.
- No signature change. The builder call site is unchanged.
