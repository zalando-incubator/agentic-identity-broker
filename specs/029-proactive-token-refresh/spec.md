# Feature Specification: Proactive Token Refresh

**Feature Branch**: `029-proactive-token-refresh`  
**Created**: 2026-05-07  
**Status**: Draft  
**Input**: User description: "Refresh and access token from the third-party services per user never refreshed. The refresh of these tokens happens only on the read path when its are loaded / inspected. As a problem it causes extra latency for latency sensitive token exchange operation. It is required to solve the problem for (a) active ongoing sessions in stateless, no-coordination mode; (b) inactive sessions within the admin process (refresh process has to be scoped)"

## Clarifications

### Session 2026-05-07

- Q: What is the deduplication scope for background refresh — per-replica or cross-replica? → A: Per-replica (`singleflight` within each process). Across replicas, a row lock serializes refreshes. Each replica re-checks the latest session before calling the provider. Refreshes update an existing row only.
- Q: Should background refresh goroutines be bounded or unbounded? → A: Bounded worker pool with configurable concurrency limit (default: 10); excess proactive refresh requests are dropped silently — the caller already received a valid token, so a dropped refresh is safe.
- Q: What observability signals are required? → A: Structured log events (INFO on success, WARN on dropped/failed, ERROR on infrastructure failures) plus OpenTelemetry `Int64` counters emitted through the existing telemetry pipeline — no Prometheus registry, no `/metrics` endpoint, and no `/health` counter surface: `proactive_refresh_triggered_total`, `proactive_refresh_dropped_total`, `proactive_refresh_failed_total`, `session_sweep_refreshed_total`, `session_sweep_failed_total`.
- Q: Is `lookahead_duration` required or optional in the sweep request body? → A: Optional — omitting it uses the server-configured `token_refresh.lookahead_duration` default; callers may override it for wider ad-hoc sweeps.
- Q: What HTTP status code should the sweep return when all per-session refreshes fail? → A: `200 OK` always when the sweep itself completed (even if every session failed); `5xx` is reserved for infrastructure failures such as database unavailability or handler panics.

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Zero-Latency Token Exchange for Active Users (Priority: P1)

An AI agent performs a token exchange during an active user session. The user's third-party access token was recently issued and is well within its validity window, so the exchange completes immediately without any upstream OAuth2 refresh round-trip.

When a valid token approaches expiry, the exchange returns it without waiting for background work. If a refresh slot is available and the provider succeeds, the broker stores new tokens. A later exchange then uses the fresh token. Dropped or failed work does not change the current response.

**Why this priority**: This is the primary latency-sensitive hot path. An AI agent token exchange must complete quickly. Any synchronous upstream OAuth2 call on the critical path causes user-visible delay. Solving this first delivers the core value of the feature.

**Independent Test**: Exchange a valid token inside the lookahead window while the provider response is gated. The exchange returns the current token before the refresh completes. After releasing the gate, a successful refresh stores new tokens without another exchange.

**Acceptance Scenarios**:

1. **Given** one near-expiry session occupies the only background refresh slot, another session is near expiry, and a third is healthy, **When** agents exchange tokens for the latter two sessions, **Then** both receive their current valid tokens without waiting. The due session's refresh is dropped with a WARN event. The healthy session causes no upstream refresh.
2. **Given** a refreshable session whose access token is near expiry but still valid, **When** an agent requests a token exchange, **Then** the exchange returns the current valid token without waiting for the background refresh. If the provider completes the admitted refresh successfully, the stored session receives the new tokens. A failed or dropped refresh does not change the current response.
3. **Given** a background refresh is already in progress for a session, **When** a second token exchange request arrives for the same session, **Then** the second request returns without waiting for that refresh to finish and without triggering a duplicate upstream refresh call.

---

### User Story 2 - Expired Session Refresh Without User Interruption (Priority: P1)

An AI agent attempts a token exchange for a user whose access token has already expired but whose refresh token is still valid. Rather than failing the request or prompting user re-authentication, the system transparently refreshes the session using the stored refresh token and returns the newly issued access token.

**Why this priority**: Tied in priority with Story 1 — this is the other half of the on-demand path. Without it, agents silently fail when tokens expire between requests. The feature description explicitly requires this to work on the read path as the baseline.

**Independent Test**: Can be fully tested by directly expiring a stored session's access token (leaving the refresh token valid) and verifying that a token exchange request succeeds and returns a fresh access token.

**Acceptance Scenarios**:

1. **Given** a session with an expired access token and a valid refresh token, **When** an agent exchanges a token, **Then** one synchronous refresh returns a fresh access token. After the update commits, an INFO `session.oauth2.token_refreshed` event contains `session_id` and `triggered_by=on-demand`.
2. **Given** a session with expired access and refresh tokens, **When** an agent exchanges a token, **Then** the system requires re-authentication without an upstream call or a partial token. One ERROR `session.oauth2.refresh_failed` event contains `session_id` and `triggered_by=on-demand`.
3. **Given** the provider rejects a refresh, **When** an agent exchanges a token, **Then** the system returns a re-authentication error without changing stored tokens. One ERROR `session.oauth2.refresh_failed` event contains `session_id` and `triggered_by=on-demand`.

---

### User Story 3 - Admin-Triggered Sweep of Expiring Sessions (Priority: P2)

An operator (or a Kubernetes CronJob acting on behalf of operations) calls an administrative endpoint to proactively refresh all sessions whose access tokens are expiring within a configurable time window. The sweep operates across all principals and services in the system. The process is scoped: it processes only sessions below the expiry threshold and skips healthy sessions.

The broker does not schedule sweeps or use a distributed lock. An operator schedules the endpoint. A row lock and a re-check of the latest session serialize concurrent refreshes across replicas. Refresh paths update existing rows only.

**Why this priority**: This covers the inactive session population that Story 1 and 2 cannot reach (no agent is actively requesting tokens for those users). It requires an admin API addition, which has higher coordination cost (API-first requirement) and more operational setup.

**Independent Test**: Can be fully tested by seeding the database with sessions at various expiry states, calling the sweep endpoint, and verifying that only sessions within the threshold were refreshed and others were left unchanged.

**Acceptance Scenarios**:

1. **Given** the admin sweep endpoint is called with a lookahead window, **When** there are sessions expiring within the window and sessions not expiring within the window, **Then** only the expiring sessions are candidates for the sweep and sessions outside the window do not contribute to `refreshed`, `skipped`, or `failed` counts.
2. **Given** a session's refresh token has expired (cannot be refreshed), **When** the sweep processes that session, **Then** the sweep does not refresh the session and records it as a failure — distinct from skipped candidates that no longer require work at evaluation time — in the response summary, and continues processing remaining sessions.
3. **Given** multiple broker replicas receive a sweep request for the same session, **When** both process it concurrently, **Then** the final stored session is valid. The row lock and re-check prevent duplicate upstream refreshes and partial updates.
4. **Given** the sweep endpoint is called with `dry_run: true` in the request body, **When** there are sessions that would be refreshed, **Then** the response returns the count of sessions that would be refreshed but no actual refresh calls are made to upstream providers.

---

### Edge Cases

- What happens when the upstream OAuth2 provider is unavailable during a background proactive refresh? The background refresh MUST fail silently (log the error), leave the existing token in place, and not affect the current exchange response which already returned a valid token.
- What happens when all refresh tokens for a given third-party service have expired (e.g., service revoked all tokens)? The sweep MUST report this as a recoverable per-session failure, not abort the entire sweep. Sessions requiring re-authentication are left intact.
- What happens when a session is deleted (user revoked consent) while a background refresh is in progress? The locked, update-only refresh MUST discard the result without re-creating the session.
- What happens if the sweep lookahead window is set to a very large value covering all sessions? The sweep MUST paginate its database queries to avoid loading the entire `user_sessions` table into memory at once.
- What happens if an access token has no expiry set (`access_token_expires_at IS NULL`)? The system treats it as perpetually valid — neither background refresh nor sweep will act on it.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: When a token exchange is requested, the system MUST return the current valid access token without blocking if the token is not within the proactive refresh lookahead window.
- **FR-002**: When a token exchange is requested and a valid access token is within the configurable lookahead window, the system MUST return that token immediately. If the session has a usable refresh token, the system MUST submit it for background refresh without making the caller wait. If the bounded pool is full, the system drops the refresh and still returns the valid token.
- **FR-003**: Background refresh MUST use per-replica deduplication (`singleflight` within each broker process). If a refresh is already in progress for a session within the same process, subsequent requests for that session MUST NOT trigger another upstream refresh call. Across replicas, the locked refresh MUST re-check the latest session before an upstream call and update only an existing row.
- **FR-004**: When a token exchange is requested and the access token has already expired, the system MUST attempt an on-demand synchronous refresh using the stored refresh token before returning a result.
- **FR-005**: When a refresh token is absent or expired, the system MUST return an error indicating re-authentication is required rather than attempting a refresh.
- **FR-006**: The admin server MUST expose an endpoint to trigger a sweep of sessions with access tokens expiring within a caller-specified lookahead duration.
- **FR-007**: The sweep endpoint MUST accept a `dry_run` request body parameter. When `dry_run: true`, it MUST report how many sessions would be refreshed without performing any upstream calls.
- **FR-008**: The sweep MUST process sessions in pages to prevent loading the entire session table into memory.
- **FR-009**: The sweep MUST be safe to invoke concurrently from multiple broker replicas. A row lock and a re-check of the stored session MUST prevent duplicate upstream refreshes and partial updates. Refresh MUST NOT re-create a deleted session.
- **FR-010**: The sweep MUST report a per-sweep summary in its response: sessions refreshed, candidate sessions skipped because they no longer require action at evaluation time, sessions failed (refresh token expired or upstream error), and total sessions evaluated.
- **FR-011**: A failed refresh for one session during a sweep MUST NOT abort the sweep — the sweep MUST continue processing remaining sessions.
- **FR-012**: The proactive refresh lookahead window MUST be configurable via the standard configuration port. The default value is `5m`.
- **FR-013**: Sessions with no expiry set (`access_token_expires_at IS NULL`) MUST be excluded from both background proactive refresh and admin sweep processing.
- **FR-014**: The database MUST have an index on `access_token_expires_at` to support efficient sweep range queries.

### Domain Model

**Activity / Flow Diagram — On-Demand Path with Lookahead Refresh**:

```mermaid
sequenceDiagram
    participant Agent
    participant TokenExchangeService
    participant OAuth2SessionService
    participant ThirdPartyOAuth2Provider
    participant SessionRepository

    Agent->>TokenExchangeService: Exchange(subjectToken, resourceURI)
    TokenExchangeService->>OAuth2SessionService: GetValidAccessToken(principal, serviceID)
    OAuth2SessionService->>SessionRepository: FindByPrincipalAndService(principal, serviceID)
    SessionRepository-->>OAuth2SessionService: UserSession (encrypted tokens)

    alt Token valid, outside lookahead window
        OAuth2SessionService-->>TokenExchangeService: plaintext access token
    else Token valid, within lookahead window
        OAuth2SessionService-->>TokenExchangeService: plaintext access token (immediate)
        OAuth2SessionService--)OAuth2SessionService: background refresh (non-blocking)
        OAuth2SessionService->>SessionRepository: WithLockedSession (lock and re-check)
        SessionRepository-->>OAuth2SessionService: latest session under lock
        OAuth2SessionService->>ThirdPartyOAuth2Provider: POST refresh_token grant
        ThirdPartyOAuth2Provider-->>OAuth2SessionService: new access + refresh tokens
        OAuth2SessionService->>SessionRepository: UPDATE existing locked row only
    else Token expired, refresh token valid
        OAuth2SessionService->>SessionRepository: WithLockedSession (lock and re-check)
        SessionRepository-->>OAuth2SessionService: latest session under lock
        OAuth2SessionService->>ThirdPartyOAuth2Provider: POST refresh_token grant (blocking)
        ThirdPartyOAuth2Provider-->>OAuth2SessionService: new access + refresh tokens
        OAuth2SessionService->>SessionRepository: UPDATE existing locked row only
        OAuth2SessionService-->>TokenExchangeService: plaintext access token
    else Token expired, refresh token absent/expired
        OAuth2SessionService-->>TokenExchangeService: error (re-authentication required)
    end

    TokenExchangeService-->>Agent: access token or error
```

**Activity / Flow Diagram — Admin Sweep Path**:

```mermaid
sequenceDiagram
    participant CronJob
    participant AdminAPI
    participant SessionSweepService
    participant SessionRepository
    participant ThirdPartyOAuth2Provider

    CronJob->>AdminAPI: POST /api/sessions/sweep
    AdminAPI->>SessionSweepService: Sweep(lookaheadDuration, dryRun, pageSize)

    loop Pages of expiring sessions
        SessionSweepService->>SessionRepository: ListExpiringSessions(threshold, cursor, pageSize)
        SessionRepository-->>SessionSweepService: page of UserSessions

        loop Each session in page
            alt dryRun=true
                SessionSweepService->>SessionSweepService: count session, skip refresh
            else refresh token valid
                SessionSweepService->>SessionRepository: WithLockedSession (lock and re-check)
                SessionRepository-->>SessionSweepService: latest session under lock
                SessionSweepService->>ThirdPartyOAuth2Provider: POST refresh_token grant
                ThirdPartyOAuth2Provider-->>SessionSweepService: new tokens
                SessionSweepService->>SessionRepository: UPDATE existing locked row only
            else refresh token absent/expired
                SessionSweepService->>SessionSweepService: record as failed, continue
            end
        end
    end

    SessionSweepService-->>AdminAPI: SweepResult (refreshed, skipped, failed, total)
    AdminAPI-->>CronJob: 200 OK with sweep summary
```

**Domain Event**:
- **SessionTokensRefreshed**: A successful, committed session refresh. The broker records this event as the structured `session.oauth2.token_refreshed` log. It contains `session_id`, `service_id`, and `triggered_by` (background, on-demand, sweep). The log does not contain the user principal.

### Configuration Requirements

**Configuration Parameters**:
- **`token_refresh.lookahead_duration`**: Duration, the time before access token expiry at which proactive background refresh is triggered. Default: `5m`.
- **`token_refresh.background_workers`**: Integer, maximum number of concurrent background refresh goroutines per broker replica. Excess proactive refresh requests are dropped silently. Default: `10`.
- **`token_refresh.sweep.default_page_size`**: Integer, number of sessions processed per page during an admin sweep. Default: `100`.

**Example YAML Configuration**:
```yaml
token_refresh:
  lookahead_duration: 5m
  background_workers: 10
  sweep:
    default_page_size: 100
```

**Configuration Location**: Will be added to `internal/ports/config.go` and `examples/config/third-party-oauth2.yaml`.

### API Requirements

- **API-001**: The admin sweep endpoint MUST be documented in `/api/admin/openapi.yaml`.
- **API-002**: `POST /api/sessions/sweep` triggers the sweep. All request body parameters are optional: `lookahead_duration` (positive ISO 8601 `P[nD][T[nH][nM][n[.f]S]]`; year, month, and week units are not accepted; default: `token_refresh.lookahead_duration`), `dry_run` (boolean, default `false`), and `page_size` (integer, `1–1000`; default: `token_refresh.sweep.default_page_size`).
- **API-003**: The sweep response body MUST include: `refreshed` (integer), `skipped` (integer), `failed` (integer), `total_evaluated` (integer), and `dry_run` (boolean). When `dry_run=true`, sessions that would be refreshed MUST still be counted in `refreshed`; the broker simply skips upstream calls and persistence. `skipped` counts only evaluated candidates that no longer require action, not healthy sessions excluded by the repository query.
- **API-004**: The endpoint MUST return `200 OK` whenever the sweep process itself completes, regardless of per-session refresh outcomes — including the case where every session fails to refresh. The response body counts (`refreshed`, `failed`, `total_evaluated`) convey outcome to the caller. `500` is reserved for infrastructure failures (database unreachable, handler panic).
- **API-005**: The endpoint MUST be on the admin server (port 14000) and require administrative access enforced at the proxy level.

### Database Requirements

- **DB-001**: A migration MUST add an index on `user_sessions(access_token_expires_at)` for efficient sweep queries. For a large table, the migration MUST use `CREATE INDEX CONCURRENTLY` with a no-transaction directive as required by `AGENTS.md`. The migration MUST fully apply or fully roll back on failure. The current migration design does not satisfy all these requirements; implementation is blocked until it does.
- **DB-002**: The existing `user_sessions` table structure MUST NOT change. Initial session creation keeps the existing upsert. Proactive, on-demand, and sweep refreshes MUST use the existing `UserSessionRefreshRepository.WithLockedSession` operation. The locked callback re-checks the current session and updates tokens only when the row still exists. A deleted session MUST NOT be re-created.
- **DB-003**: A focused `UserSessionExpiryRepository` port MUST define `ListExpiringSessions(ctx, threshold time.Time, cursor id.SessionID, limit int) ([]*storage.UserSession, error)`. Both the in-memory and PostgreSQL adapters MUST implement it. `UserSessionRepository` remains unchanged.

### Non-Functional Requirements

- **NFR-001**: The system MUST emit a structured log line at `INFO` level each time a session's tokens are successfully refreshed, including fields: `triggered_by` (background, on-demand, sweep), `service_id`, and session ID. Principal MUST be omitted or hashed to protect privacy.
- **NFR-002**: The system MUST emit a structured log line at `WARN` level when a proactive background refresh is dropped because the worker pool is at capacity, including `service_id` and session ID.
- **NFR-003**: The system MUST emit a structured log line at `WARN` level for each per-session failure during a sweep (refresh token expired, upstream error), and at `ERROR` level for infrastructure failures (database unreachable).
- **NFR-004**: The system MUST emit OpenTelemetry `Int64` counters through the existing telemetry pipeline (no Prometheus registry, no `/metrics` endpoint, and no `/health` counter surface): `proactive_refresh_triggered_total`, `proactive_refresh_dropped_total`, `proactive_refresh_failed_total`, `session_sweep_refreshed_total`, `session_sweep_failed_total`. Counters MUST be labeled with `triggered_by` where applicable.

### Security Requirements

- **SR-001**: Background refresh operations MUST encrypt newly issued tokens using the same encryption context (`{"service_id": "..."}`) before persisting. Plaintext tokens MUST never be written to storage.
- **SR-002**: The sweep endpoint MUST NOT expose token plaintext in its response. The response MUST contain only counts and status summaries.
- **SR-003**: Background goroutines performing token refresh MUST respect context cancellation (server shutdown) and MUST NOT hold references to decrypted tokens beyond the scope of a single refresh operation.
- **SR-004**: Upstream refresh calls MUST use the service's encrypted client secret, decrypted only at call time and not cached in memory.

### Key Entities

- **UserSession**: Existing entity. The refresh feature reads and updates `EncryptedAccessToken`, `EncryptedRefreshToken`, `AccessTokenExpiresAt`, `RefreshTokenExpiresAt`. No new fields required.
- **SweepResult**: Value object representing the outcome of a sweep operation: `{Refreshed int, Skipped int, Failed int, TotalEvaluated int, DryRun bool}`.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: Token exchange requests for sessions with valid, non-expiring access tokens complete with no upstream OAuth2 calls — measurable by observing zero upstream refresh calls in traces for healthy sessions.
- **SC-002**: Token exchange requests for sessions whose access token is near expiry (within the lookahead window) return the current token on the non-blocking fast path — measurable by asserting that the response is returned before the mocked background refresh completes, and that the request path makes zero blocking upstream OAuth2 calls.
- **SC-003**: Concurrent token exchanges MUST trigger at most one upstream refresh per session while that refresh is in flight. Across replicas, a locked re-check MUST prevent a second upstream refresh of a session that another replica already renewed. A failed attempt can be retried on a later exchange.
- **SC-004**: The admin sweep endpoint processes all sessions expiring within the configured window and returns a correct summary — measurable by asserting that `refreshed + skipped + failed = total_evaluated` and that healthy sessions excluded by the repository query do not appear in those counts.
- **SC-005**: The admin sweep can be invoked concurrently from multiple broker replicas without producing corrupted session state — the final stored tokens are always consistent.
- **SC-006**: An infra-backed PostgreSQL verification of `ListExpiringSessions(...)` shows that the sweep query uses the `access_token_expires_at` index under normal operation.

## Assumptions

- The upstream OAuth2 provider supports the `refresh_token` grant type (RFC 6749 §6). Sessions for providers that do not issue refresh tokens will not benefit from this feature and will continue to require re-authentication on expiry.
- The Kubernetes CronJob is responsible for scheduling and coordinating the admin sweep. The broker provides the endpoint but does not manage scheduling internally.
- The default lookahead window of 5 minutes is sufficient for typical access token lifetimes (usually 1 hour). Operators can adjust via configuration if upstream providers issue short-lived tokens.
- The `page_size` default of 100 sessions per sweep page is conservative. Operators can increase it via the request body if their session volume warrants it.
