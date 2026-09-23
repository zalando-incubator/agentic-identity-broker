# ADR 037: Activity Event Storage

**Status**: Proposed
**Date**: 2026-09-08 · **Revised**: 2026-09-22 (renumbered from 035, which collided with ADR 035 root-mounted SPA)
**Feature**: 038-activity-audit-experience

---

## Context

The broker stores the current state of grants, sessions, and approvals. It does not store a user-readable history of changes.

The Activity feature needs curated, deduplicated events. It must retain history after a source record expires, changes, or is deleted.

The feature needs per-principal keyset pages. It also needs combined filters for agents, services, grants, outcomes, categories, keywords, and time windows, and threads derived from related identifiers.

Repeated reports of one transition need one event; separate transitions need different identities.
Session and approval expiry can occur without a token exchange or approval-detail request.
An idle session can expire and reconnect before a periodic worker sees it. The spec requires one
visible event per transition (FR-016b) and summarized milestones for routine broker observations
(FR-001).

The application has no activity scheduler or caller-owned transaction spanning grant, approval,
session, and activity repositories. Grant writes commit inside their repository methods.

ADR 004 requires memory storage for development and tests. It requires PostgreSQL storage for production.

The existing DynamoDB table stores AWS Encryption SDK branch keys. Its schema and IAM policy are not for application data.

---

## Decision

Add the `internal/domain/activity` bounded context. It owns the `ActivityEvent` aggregate, the `ActivityRecorder` port, the `SafeFields` registry, and read-time thread derivation.

Add `ActivityEventRepository` to `internal/ports/storage.go`. Provide memory and PostgreSQL adapters. Use SQLx, migrations, storage errors, and timeouts as ADR 004 requires.

Store only curated business-significant and security-significant events. Do not store every token exchange or policy allow operation.

**Deduplicate and roll up.** Every event has a `dedup_key` stable across reports of one transition.
Grant updates use a UUID created once per mutation. Reconnections use the session's committed
token revision because the session ID survives reconnects. The table enforces `UNIQUE (principal,
dedup_key)`. Routine broker access issuances (`agent.access_issued`), session refreshes, and
bucketed failures roll up per principal/key/bucket with an `occurrence_count`. The access count
measures successful broker token exchanges, not downstream service uses.

**Best-effort post-write recording.** Grant, approval, and session seams enqueue after their
repository writes succeed. All seams use a bounded in-process queue drained by an application
worker. A full queue or failed event insert increments `activity_events_dropped_total` and logs
at `ERROR`. Recording never blocks or fails the primary operation. The event and source mutation
do not commit atomically; existing structured security audit logs remain independent.

Use PostgreSQL for the production event store. Do not add DynamoDB as an activity-data backend.

Use `(principal, occurred_at DESC, id DESC)` for the feed and cursor; the cursor is bound to its filter set. Add partial expression indexes for `agent_id`, `service_id`, `grant_id`, and `approval_id` in `related_refs`; these also serve read-time thread assembly. Add a `pg_trgm` GIN index on `summary` for keyword search, with an `ILIKE` fallback if the extension is unavailable. Do not persist a thread key.

Apply the configured retention window (default 30 days) to feed, detail, related-sequence, and
thread-count reads, even when pruning is pending. Return 404 for details outside the window.
Derive reconnect attention from session refresh expiry or a persisted `reconnect_required_at`,
never from activity events. A rejected refresh or absent usable refresh token marks the observed
token revision; a successful refresh or callback clears the marker with the next token write.
A delayed failure must not overwrite a newer token revision. Derive pending-approval attention
from current approval state.
Prune old events in bounded batches under `pg_try_advisory_lock`.

The same worker periodically scans bounded batches of due session and pending-approval expiries.
It emits `session.expired:{session_id}:{token_revision}` or
`approval.expired:{approval_id}` through the recorder. A reconnection snapshots an expired
session before its token write and enqueues expiry after success, ahead of reconnection.
Repeated and multi-replica observations share the dedup key. No general scheduler is added.

Only approved fields are used in display text and `detail.context`. Resolve protected resources
server-side. Never copy a raw resource URI into an activity event, its `related_refs`, or a
browser DTO. Build responses by projecting approved identifiers rather than serializing stored
JSONB. Every emitted route comes from a server-side allowlist. A `correlation_id` (trace id)
stays server-side. Local token-grant failures without a verified principal remain in
operational audit logs.

SC-005 remains a user task. A user must find a recent event within 30 seconds among 10,000 events; keyword and jump-to-date filters exist so this does not depend on paging.

The 2,000-principal, 10,000-events-per-principal, 330-writes-per-second workload is an exploratory storage benchmark. It is not a product quota, traffic forecast, capacity commitment, or API latency SLO. It captures repository and endpoint latency distributions on a warm isolated PostgreSQL database and measures roll-up upsert contention. SC-005 remains the sole user-facing performance criterion.

---

## Consequences

### Positive

The feature keeps historical labels and context after a referenced object is deleted. It can meet FR-001, FR-013, FR-015, FR-016b, and US5.

The feed rolls up broker token issuances; it does not imply a count of downstream actions.

Failed activity recording cannot roll back a committed grant, approval, or session write. Loss
on the async path is measurable, and live reconnect state is independent of history retention.

The repository follows the current storage architecture. The production service reuses PostgreSQL migrations, SQLx, connection pooling, backups, and multi-instance operations. The prune is multi-replica safe without new infrastructure.

### Negative

Post-write activity recording can be lost during a PostgreSQL outage or queue saturation; the
counter makes this visible but does not recover the missing event.

Roll-up upserts concentrate writes on a small number of rows per principal per bucket; the benchmark must confirm this does not contend under burst.

`pg_trgm` is an extension dependency that managed PostgreSQL offerings may or may not allow; the fallback is slower.

The event table and its indexes require operational sizing. Operators must size production from measured workload data; the benchmark does not change the current 10-GiB Helm default.

Phase 0 changes to `tokenexchange` (typed denial reasons, success hook) are a prerequisite.

---

## Alternatives Considered

1. **Aggregate on read**: Rejected. Current repositories cannot recover expired, revoked, or deleted historical state.
2. **DynamoDB activity store**: Rejected. It needs a new storage backend, table, IAM policy, configuration, and operational model. Combined filters need GSIs or duplicate index items. DynamoDB TTL does not replace the retention predicate because expiry is asynchronous.
3. **Structured log or telemetry query**: Rejected. Logs and spans are not a stable, retention-scoped per-principal product contract.
4. **Synchronous event insertion in the source mutation transaction**: Rejected. Current
   repositories expose no shared transaction. An insert failure would abort the mutation unless
   isolated with a savepoint, which requires a transaction-boundary redesign.
5. **Persisted single thread key per event**: Rejected. Events belong to several narratives; the related-ref indexes already serve read-time assembly.
6. **A generic job scheduler for pruning**: Rejected. One advisory-locked batch delete does not justify new infrastructure.
