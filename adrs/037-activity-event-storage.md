# ADR 037: Activity Event Storage

**Status**: Proposed
**Date**: 2026-09-08 · **Revised**: 2026-09-22 (renumbered from 035, which collided with ADR 035 root-mounted SPA)
**Feature**: 038-activity-audit-experience

---

## Context

The broker stores the current state of grants, sessions, and approvals. It does not store a user-readable history of changes.

The Activity feature needs curated, deduplicated events. It must retain history after a source record expires, changes, or is deleted.

The feature needs per-principal keyset pages. It also needs combined filters for agents, services, grants, outcomes, categories, keywords, and time windows, and threads derived from related identifiers.

The same underlying action can reach the broker more than once: each extproc replica keeps its own token cache and re-exchanges on eviction; expiry of sessions and approvals is detected lazily on every read; approval creation is idempotent. The spec requires exactly one visible event per action (FR-016b) and that routine internals be summarized into milestones (FR-001).

The application has no job scheduler, leader election, or distributed lock. Several emission seams already run inside a PostgreSQL transaction; the token-exchange path does not.

ADR 004 requires memory storage for development and tests. It requires PostgreSQL storage for production.

The existing DynamoDB table stores AWS Encryption SDK branch keys. Its schema and IAM policy are not for application data.

---

## Decision

Add the `internal/domain/activity` bounded context. It owns the `ActivityEvent` aggregate, the `ActivityRecorder` port, the `SafeFields` registry, and read-time thread derivation.

Add `ActivityEventRepository` to `internal/ports/storage.go`. Provide memory and PostgreSQL adapters. Use SQLx, migrations, storage errors, and timeouts as ADR 004 requires.

Store only curated business-significant and security-significant events. Do not store every token exchange or policy allow operation.

**Deduplicate and roll up.** Every event carries a deterministic `dedup_key`; the table enforces `UNIQUE (principal, dedup_key)` and `Record` is an idempotent upsert. Routine successes (`agent.acted_via_service`, `session.refreshed`) and bucketed failures are one row per `(principal, dedup_key)` per configured bucket with an `occurrence_count`.

**Two recorder modes.** Seams already inside a PostgreSQL transaction record the event in that transaction. Other seams enqueue to a bounded in-process queue drained by one goroutine per instance. A dropped or failed recording increments `activity_events_dropped_total` and logs at ERROR. Recording never blocks or fails the primary security operation; the security operation stays fail-closed on its own result.

Use PostgreSQL for the production event store. Do not add DynamoDB as an activity-data backend.

Use `(principal, occurred_at DESC, id DESC)` for the feed and cursor; the cursor is bound to its filter set. Add partial expression indexes for `agent_id`, `service_id`, `grant_id`, and `approval_id` in `related_refs`; these also serve read-time thread assembly. Add a `pg_trgm` GIN index on `summary` for keyword search, with an `ILIKE` fallback if the extension is unavailable. Do not persist a thread key.

Apply the configured retention window (default 30 days) to historical activity-event queries. Derive live needs-attention items from unresolved current state plus recent refresh-failure events, regardless of event retention. Prune old events in bounded batches under `pg_try_advisory_lock` so only one replica prunes at a time.

Only fields approved in the `SafeFields` registry are stored in `detail.context`; every route the API emits comes from a server-side allowlist. A `correlation_id` (trace id) is stored and never returned to the browser.

SC-005 remains a user task. A user must find a recent event within 30 seconds among 10,000 events; keyword and jump-to-date filters exist so this does not depend on paging.

The 2,000-principal, 10,000-events-per-principal, 330-writes-per-second workload is an exploratory storage benchmark. It is not a product quota, traffic forecast, capacity commitment, or API latency SLO. It captures repository and endpoint latency distributions on a warm isolated PostgreSQL database and measures roll-up upsert contention. SC-005 remains the sole user-facing performance criterion.

---

## Consequences

### Positive

The feature keeps historical labels and context after a referenced object is deleted. It can meet FR-001, FR-013, FR-015, FR-016b, and US5.

Feed volume is proportional to distinct actions, not to MCP traffic or extproc replica count.

Security-significant transitions (grants, approvals, sessions) and their events commit atomically. Loss on the async path is measurable.

The repository follows the current storage architecture. The production service reuses PostgreSQL migrations, SQLx, connection pooling, backups, and multi-instance operations. The prune is multi-replica safe without new infrastructure.

### Negative

Activity recording on the async path can be lost during a PostgreSQL outage or queue saturation; the counter makes this visible but does not recover it.

Roll-up upserts concentrate writes on a small number of rows per principal per bucket; the benchmark must confirm this does not contend under burst.

`pg_trgm` is an extension dependency that managed PostgreSQL offerings may or may not allow; the fallback is slower.

The event table and its indexes require operational sizing. Operators must size production from measured workload data; the benchmark does not change the current 10-GiB Helm default.

Phase 0 changes to `tokenexchange` (typed denial reasons, success hook) are a prerequisite.

---

## Alternatives Considered

1. **Aggregate on read**: Rejected. Current repositories cannot recover expired, revoked, or deleted historical state.
2. **DynamoDB activity store**: Rejected. It needs a new storage backend, table, IAM policy, configuration, and operational model. Combined filters need GSIs or duplicate index items. DynamoDB TTL does not replace the retention predicate because expiry is asynchronous.
3. **Structured log or telemetry query**: Rejected. Logs and spans are not a stable, retention-scoped per-principal product contract.
4. **Fire-and-forget synchronous recording everywhere**: Rejected. Unobservable loss for security-significant transitions and added latency on the exchange hot path.
5. **Persisted single thread key per event**: Rejected. Events belong to several narratives; the related-ref indexes already serve read-time assembly.
6. **A generic job scheduler for pruning**: Rejected. One advisory-locked batch delete does not justify new infrastructure.
