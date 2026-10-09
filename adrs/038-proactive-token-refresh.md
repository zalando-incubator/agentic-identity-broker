# ADR 038: Proactive Token Refresh

**Status**: Accepted
**Date**: 2026-10-08

## Context

Token exchange currently waits for an upstream OAuth2 refresh when a third-party access token has expired. Active sessions need refresh before expiry without adding latency to the exchange. Inactive sessions need an operator-controlled way to refresh tokens.

Broker replicas do not share a scheduler or an in-memory lock. The existing `UserSessionRefreshRepository.WithLockedSession` operation locks the current session and updates an existing row only. Refresh-token rotation makes interruption between the provider response and the database update unsafe.

## Decision

- Token exchange returns a valid access token immediately. If the token is inside the Refresh Lookahead Window and can be refreshed, it submits a background refresh without waiting.
- Each replica bounds background refreshes with a nonblocking slot pool. It deduplicates in-flight submissions by session before it takes a slot. A full pool drops new work and records a WARN event and counter. It never delays the exchange.
- Background and on-demand refreshes share per-process `singleflight`. Correctness across replicas depends on `WithLockedSession`, not on `singleflight`. Under the lock, each refresh re-checks whether the current row is due before it calls the provider. The operation updates tokens only if the row still exists. A deleted session cannot be recreated.
- The admin API runs a Session Sweep synchronously, one session at a time in bounded keyset pages. An external operator schedule, such as a CronJob, invokes the endpoint. The broker has no internal scheduler or distributed lock. The sweep uses the focused `UserSessionExpiryRepository` read port and the existing locked, update-only refresh operation.
- The sweep handler lifts the HTTP write deadline only for its route. The caller owns the overall sweep timeout. The sweep checks request cancellation between sessions. Each admitted session refresh uses a bounded context detached from the request, so a client disconnect does not interrupt its token update.
- Shutdown stops new background submissions and drains admitted refreshes within the shutdown deadline. It cancels their contexts only after the drain or when the deadline expires. This order reduces the risk of losing a rotated refresh token before it is stored.

### Approved admin API contract

The endpoint uses `POST /api/sessions/sweep` on the admin server. The proxy authenticates the operator. The published contract is in `api/admin/openapi.yaml`.

- `page_size` accepts 1 through 1000. The configured default is 100.
- `lookahead_duration` accepts a positive ISO 8601 duration with days, hours, minutes, and seconds: `P[nD][T[nH][nM][n[.f]S]]`. It rejects years, months, weeks, and Go durations.
- Unknown JSON request properties return 400.
- A missing operator principal returns 500 `server_misconfiguration`. The broker does not run the sweep without an operator identity.

The stakeholder approved all four choices in writing in this conversation on 2026-10-09. PR #196 is the review reference, not the location of that approval.

## Consequences

- A valid token remains available to an exchange even if background work fails or the pool is full. An operator can schedule sweeps for inactive sessions without adding a broker scheduler.
- The row lock protects updates and prevents duplicate provider calls when another replica has already moved the token beyond the refresh threshold. The sweep re-checks under the lock and counts a no-longer-due candidate as skipped.
- Each locked refresh holds a database connection during the upstream call. The background worker cap protects capacity for request traffic. Synchronous sweeps limit provider and database load, but their duration depends on the number of candidates and provider latency.
- The external caller must set a suitable timeout. A shutdown deadline that expires during a provider call can still interrupt the update of a rotated refresh token.
- The separate expiry-index migration remains blocked until every migration runner enforces the directive and tests prove this guard. ADR 039 accepts a named non-atomic exception for migration `036`. This ADR does not authorize its creation.

## Alternatives Considered

- **Internal ticker scheduler**: Rejected. A ticker in each replica multiplies sweeps and requires coordination. An external operator controls the schedule.
- **Distributed lease**: Rejected. The row lock already serializes each existing session across replicas. A lease adds coordination infrastructure without replacing that check.
- **Asynchronous job API**: Rejected. A `202` response and status resource contradict the required synchronous `200` summary and add persisted job state.
- **New `UpdateTokensIfExists` write path**: Rejected. An update without the locked re-check allows replicas to call the provider concurrently. A rotated token can then be overwritten. The existing `WithLockedSession` operation provides the required update-only behavior.

## References

- Feature specification: `specs/029-proactive-token-refresh/spec.md` (FR-002, FR-003, FR-009, DB-002, DB-003)
- Design rationale: `specs/029-proactive-token-refresh/research.md` (R1, R2, R6–R8, R13)
- Domain model: `specs/029-proactive-token-refresh/data-model.md`
- Port contract: `specs/029-proactive-token-refresh/contracts/ports.md`
