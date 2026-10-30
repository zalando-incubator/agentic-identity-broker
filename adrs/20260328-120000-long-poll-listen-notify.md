# ADR 20260328-120000: Long-Poll with PostgreSQL LISTEN/NOTIFY for Approval Sync

**Status**: Accepted
**Date**: 2026-03-28

---

## Context

The tool approval system (spec 20260327-120000) requires a sync endpoint (`GET /api/approvals`) that allows ExtProc gateway instances to maintain an up-to-date local cache of approval decisions. ExtProc needs to know within approximately 2 seconds when a user approves or denies a pending tool call.

In a multi-instance broker deployment, approval mutations may happen on any instance. All instances must wake their local long-poll connections when any approval state changes, regardless of which instance processed the mutation.

Key constraints from the spec:
- Support ≥100 concurrent gateway connections
- Approval state propagation ≤2 seconds
- Configurable coalesce window (default 1 second) to prevent per-mutation connection churn
- Standard HTTP (no WebSocket/SSE) — fits ExtProc's request-response model

---

## Decision

### Long-Poll HTTP Pattern

Implement `GET /api/approvals` as a standard chi handler that blocks on a `select{}` between three events:

1. **Change channel**: Fires when any approval state changes (from the `ApprovalSyncBroadcaster`)
2. **Timeout timer**: Fires after `X-Long-Poll-Timeout` seconds (client-specified, max 120s)
3. **Context cancellation**: Fires when the client disconnects

The handler uses `If-None-Match` with the `approval_sync_state.version` counter as the ETag value (format: `"v{version}"`). If the client's ETag matches the current version, the handler blocks. If it differs (or is absent), the handler returns immediately with current state.

### Cross-Instance Wake-Up via PostgreSQL LISTEN/NOTIFY

Use PostgreSQL's built-in `LISTEN/NOTIFY` mechanism for cross-instance notification:

1. **NOTIFY side**: When any approval mutation increments the `approval_sync_state.version` counter, the PostgreSQL adapter issues `NOTIFY approval_sync` in the same transaction.
2. **LISTEN side**: Each broker instance runs a dedicated `ApprovalSyncSubscriber` goroutine that holds a long-lived pgx connection subscribed to the `approval_sync` channel. On receipt of a notification, it calls `ApprovalSyncBroadcaster.Broadcast()` to wake all local long-poll goroutines.

### Coalesce Window

The `ApprovalSyncBroadcaster` implements a configurable coalesce window (default 1 second). When a broadcast is triggered, it waits for the coalesce duration before actually waking subscribers. Additional broadcasts during the window are absorbed. This prevents per-mutation connection churn when multiple approvals change in rapid succession.

### Architecture Components

```
                    ┌─────────────────┐
                    │  PostgreSQL      │
                    │  NOTIFY channel  │
                    └────────┬────────┘
                             │
              ┌──────────────┼──────────────┐
              │              │              │
    ┌─────────▼──────┐ ┌────▼─────────┐ ┌──▼──────────────┐
    │ Broker Instance │ │ Broker Inst. │ │ Broker Instance  │
    │ SyncSubscriber  │ │ SyncSubscr.  │ │ SyncSubscriber   │
    │       │         │ │      │       │ │       │          │
    │  SyncBroadcaster│ │ SyncBroadc.  │ │  SyncBroadcaster │
    │  (coalesce 1s)  │ │ (coalesce)   │ │  (coalesce 1s)   │
    │   │   │   │     │ │  │   │   │   │ │   │   │   │      │
    │ LP1 LP2 LP3     │ │LP1 LP2 LP3   │ │ LP1 LP2 LP3      │
    └─────────────────┘ └──────────────┘ └──────────────────┘
```

---

## Alternatives Considered

1. **Redis Pub/Sub**: Would work well but adds an infrastructure dependency not present in the project. PostgreSQL is already a hard requirement, making LISTEN/NOTIFY a zero-new-dependency solution.

2. **Application-level polling**: Each broker instance polls the `approval_sync_state.version` table periodically. Simple but introduces latency (polling interval) and generates O(instances × polling_rate) queries per second.

3. **HTTP Server-Sent Events (SSE)**: Would provide real-time push but requires ExtProc to maintain persistent SSE connections with reconnection logic. Long-poll with ETag/If-None-Match is simpler for the client and fits the standard HTTP request-response model.

4. **WebSocket**: Maximum flexibility but highest client-side complexity. ExtProc would need WebSocket connection management, heartbeats, and reconnection. Overkill for a simple "has anything changed?" query.

---

## Consequences

### Positive
- Zero additional infrastructure dependencies — uses PostgreSQL's native capability
- Simple client implementation — standard HTTP with `If-None-Match` and `X-Long-Poll-Timeout`
- Graceful degradation — if LISTEN/NOTIFY fails, clients still get data on timeout/reconnect
- Coalesce window prevents thundering herd on rapid mutations

### Negative
- Requires a dedicated pgx connection per broker instance (separate from sqlx pool)
- LISTEN/NOTIFY payloads are limited to ~8000 bytes (not a concern — we only signal "something changed", not the data itself)
- If the dedicated LISTEN connection drops, there is a window where long-poll clients are not woken until reconnection completes (mitigated by subscriber reconnection logic with exponential backoff)

### Implementation Notes
- The `ApprovalSyncSubscriber` uses a raw `pgx.Conn` (not from the sqlx pool) because `WaitForNotification()` blocks the connection
- The subscriber must implement reconnection with exponential backoff (1s, 2s, 4s, ..., 30s cap)
- Memory backend uses the `ApprovalSyncBroadcaster` directly (no LISTEN/NOTIFY needed for single-instance in-memory mode)
