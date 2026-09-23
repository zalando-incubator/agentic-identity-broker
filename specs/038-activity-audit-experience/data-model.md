# Phase 1 Data Model: End-User Activity & Auditing Experience

**Feature**: 038-activity-audit-experience
**Input**: [spec.md](./spec.md) Key Entities · [research.md](./research.md) Decisions 2, 6, 7, 10, 11, 12, 13

This model backs a durable, per-user, curated activity record plus a live-derived
needs-attention view. New bounded context: `internal/domain/activity`.

**Revision 2026-09-22 (plan review)**: adds deduplication (`dedup_key`), roll-up of routine
milestones (`occurrence_count`), read-time threading (no persisted `thread_key`), a structured
explanation model, the approved-field registry, correlation IDs, recorder durability modes, and a
coordinated prune. Per-event `needs_attention` is removed from the persisted model and the feed.

---

## 1. ActivityEvent (aggregate, persisted)

A single business-/security-significant occurrence within one principal's scope. Append-only
with one exception: a **roll-up** event may have its `occurrence_count` / `last_occurred_at`
incremented within its bucket (§1.2). Nothing else is ever updated. Corresponds to the spec's
**Activity Event**.

| Field | Type | Notes |
|---|---|---|
| `id` | `id.ActivityEventID` (typed UUID, ADR 013) | PK; add to `gen_ids.go` |
| `principal` | `id.Principal` (string) | Owner, as extracted by the principal middleware (configurable header, not necessarily an email). Every query is scoped by this. |
| `dedup_key` | `string` | **Idempotency key** (§1.1). `UNIQUE (principal, dedup_key)`. |
| `category` | `ActivityCategory` enum | User-comprehensible kind (§4). |
| `type` | `string` | Finer sub-kind, e.g. `session.refreshed`, `grant.revoked`, `approval.denied`. Every `type` MUST have an entry in the `SafeFields` registry (§3.1). |
| `actor_ref` | `ActorRef` (value object) | `{kind, id, display_label}`; `display_label` is the historical human name captured at emit time (FR-015). |
| `subject_ref` | `SubjectRef` (value object) | `{kind, id, display_label}`; captured at emit time. |
| `outcome` | `Outcome` enum | `succeeded` \| `failed` \| `blocked` \| `pending` (§7). |
| `summary` | `string` | Server-composed, human-readable, redacted sentence answering what/who/what-affected/outcome (FR-002, FR-010), produced from the per-`type` wording template (§3.2). |
| `detail` | `ActivityDetail` (value object, JSONB) | Structured explanation, approved context, related links (§3). |
| `related_refs` | `RelatedRefs` (value object, JSONB) | Approved foreign IDs for filtering, read-time threading, and links: `agent_id?`, `service_id?`, `grant_id?`, `session_id?`, `approval_id?`, `client_id?`. Never store a raw resource URI. |
| `occurrence_count` | `int` | ≥1. >1 only for roll-up types (§1.2). |
| `first_occurred_at` | `time.Time` | Domain time of the first occurrence in the bucket (= `occurred_at` for non-roll-ups). |
| `occurred_at` | `time.Time` (RFC3339) | Domain event time; for roll-ups the **latest** occurrence (ordering key). |
| `recorded_at` | `time.Time` | Persistence time (prune, debug). For lazily detected expiries this is the detection time; `occurred_at` is the expiry time. |
| `correlation_id` | `string` (nullable) | OTel `trace_id` (or oauth2 `X-Request-ID`) captured from context. **Never displayed**; support/incident correlation only. |
| `redacted` | `bool` | True if any sensitive value was abstracted anywhere in this event. |

**Ordering & pagination**: strictly `(occurred_at DESC, id DESC)`; cursor encodes the last
`(occurred_at, id)` seen **plus a hash of the filter set**; a cursor presented with different
filters is rejected with `400 invalid_cursor` (research Decision 5).

**Validation / invariants**:
- `principal` non-empty; a query MUST NOT return events for any other principal (FR-012).
- `category`, `actor_kind`, `subject_kind`, and `outcome` MUST be valid enum members.
- Non-roll-up `dedup_key`s MUST remain stable for repeated reports of one transition and unique
  for separate transitions. A second `Record` with the same key is a no-op. Roll-ups intentionally
  share a bucket key; each new source observation increments its count once (§1.2).
- `summary` MUST NOT be empty and MUST NOT contain a raw token/secret or a bare UUID where a label exists (FR-010, FR-011).
- `detail.context` MAY contain only keys allowed for the event's `type` (§3.1).
- `outcome=pending` events MUST NOT be rendered as succeeded/failed anywhere.
- Once written, an `ActivityEvent` is immutable except for the roll-up counter fields.

**Redaction rule (FR-011, SC-007, research Decision 7)**: `summary` and every string inside
`detail`/`*_ref.display_label` are produced already-redacted by the recorder through the
`SafeFields` registry. Sensitive inputs (secrets, raw tokens, `ToolApproval.Arguments`, parameters
flagged sensitive) are never placed into any field. Where a value existed, it is represented by an
abstracted marker (`redacted: true` and/or `"•••"`) or by a *shape* description ("3 arguments,
redacted"), never the clear value.

Protected-resource URIs can contain credentials, query strings, fragments, or sensitive path
segments. Resolve the service on the server, then record its ID and approved display label. Do not
copy the raw URI into the event, its actor/subject ID or label, templates, context, or the browser
DTO. Build the response from the approved fields rather than serializing stored event JSONB.

### 1.1 Deduplication keys (FR-016b)

The same transition can reach the recorder more than once, and session/approval expiry can be
detected on repeated reads. Non-roll-up keys stay stable across reports of one transition and
differ for distinct transitions, even within one second. Roll-ups share one key per bucket.
The recorder inserts with `ON CONFLICT (principal, dedup_key) DO NOTHING` (or `DO UPDATE` for
roll-ups). Keys:

| `type` | `dedup_key` |
|---|---|
| `grant.created` | `grant.created:{grant_id}` |
| `grant.updated` | `grant.updated:{grant_id}:{transition_id}` |
| `grant.revoked` | `grant.revoked:{grant_id}` |
| `session.connected` | `session.connected:{session_id}` |
| `session.reconnected` | `session.reconnected:{session_id}:{token_revision}` |
| `session.refreshed` (roll-up) | `session.refreshed:{session_id}:{bucket}` |
| `session.refresh_failed` | `session.refresh_failed:{session_id}:{token_revision}:{bucket}` |
| `session.expired` | `session.expired:{session_id}:{token_revision}` |
| `session.terminated` | `session.terminated:{session_id}` |
| `agent.access_issued` (roll-up) | `agent.access_issued:{agent_id}:{service_id}:{bucket}` |
| `policy.denied` / `policy.blocked` | `policy:{agent_id}:{service_id}:{reason}:{bucket}` |
| `approval.requested` | `approval.requested:{approval_id}` (emit only when `IsNew`) |
| `approval.approved` / `.denied` / `.consumed` / `.revoked` | `approval.{state}:{approval_id}` |
| `approval.expired` | `approval.expired:{approval_id}` |

`transition_id` is a UUID created once for a successful grant update, not once per recorder
attempt. `token_revision` is the session's committed token revision (§8); it increases on each
successful token write. A new connection emits `session.connected` once. Each later OAuth callback
emits `session.reconnected` with its new revision, even when the session ID is unchanged.

The expiry observer (§10) and lazy request seams reuse these keys. An OAuth callback captures
the expired session's old revision before replacement. After a successful token write, it
enqueues `session.expired:{session_id}:{token_revision}` with the old revision, followed by
`session.reconnected`. The approval observer uses `approval.expired:{approval_id}` without detail access.

`bucket` = `occurred_at` truncated to `activity.rollup_bucket` (default `1h`, config §10).

### 1.2 Roll-ups (FR-001 "summarize into milestones")

Routine, high-frequency successes are not stored individually. For roll-up types
(`agent.access_issued`, `session.refreshed`, and the bucketed failure/denial types) the
recorder upserts one event per `(principal, dedup_key)` bucket:

```sql
INSERT INTO activity_events (...) VALUES (...)
ON CONFLICT (principal, dedup_key) DO UPDATE
   SET occurrence_count = activity_events.occurrence_count + 1,
       occurred_at      = GREATEST(activity_events.occurred_at, EXCLUDED.occurred_at);
```

The producer enqueues each successful broker exchange or refresh once. The worker does not retry
a failed insert. A retry of the same roll-up occurrence would increment the count again, so it
must not be enqueued as a new observation.

The card reads "Broker issued GitHub access for Research Agent · 41 times between 09:00 and
10:00". The count means successful broker token exchanges, not downstream requests or completed
actions. ExtProc can reuse a cached token without another broker exchange. The bucket is
UTC-aligned in storage; the client renders the time span in the user's local time.

---

## 2. ActivityThread (derived on read, not persisted)

An ordered grouping of related events forming a narrative/timeline (spec **Activity Thread**;
US5). Threads are **computed at read time from `related_refs`**; no `thread_key` column exists.
An event may appear in more than one thread (an `approval.approved` event belongs to the tool
flow, the agent's journey, and the service lifecycle).

| Kind | Membership rule | Title |
|---|---|---|
| `tool_flow` | same `related_refs.approval_id` | "{tool} requested by {agent}" |
| `service_lifecycle` | same `related_refs.service_id` | "{service} connection" |
| `agent_journey` | same `related_refs.agent_id` within the **requested window** | "{agent}" |

| Field | Type | Notes |
|---|---|---|
| `thread_key` | `string` | Derived, e.g. `service:{service_id}`, `agent:{agent_id}`, `toolflow:{approval_id}`. Stable across pages. |
| `kind` | `ThreadKind` enum | `service_lifecycle` \| `agent_journey` \| `tool_flow`. |
| `title` | `string` | Human-readable thread label. |
| `events` | `[]ActivityEvent` | Ascending by `occurred_at` for the Timeline. |
| `latest_outcome` | `Outcome` | Outcome of the most recent event. |
| `total_events` | `int` | Count of events in the thread within retention (across all pages). |

### API projection

`GET /api/activity` returns `ActivityThreadResponse` with ordered `event_ids` restricted to the
events present on the page, plus `total_events` and `truncated: bool` so the client can render
"N earlier events" instead of a silently partial timeline. The detail endpoint returns a bounded
`sequence` window (`sequence_limit`, default 20) around the focal event with `sequence_truncated`.

Day grouping ("Today / Yesterday / date") is a **client** concern using the browser timezone;
it is not encoded in storage.

---

## 3. ActivityDetail (value object, JSONB column)

Progressive-disclosure payload for the detail view (US4, FR-009). All values pre-approved via §3.1.

| Field | Type | Notes |
|---|---|---|
| `explanation` | `Explanation` | Structured "why" (§3.2). |
| `context` | `map[string]string` | Approved key/value context only (§3.1). Never sensitive values. |
| `related_links` | `[]RelatedLink` | `{label, target_route}`; `target_route` MUST match one of the route templates in §3.3. |

The detail view derives its pending state from the event's `outcome=pending`; no separate
`detail.pending` field is persisted or returned.

### 3.1 `SafeFields` registry (approved-for-display fields — clarification 2026-09-21)

`internal/domain/activity/safefields.go` declares, per event `type`: the allowed `context` keys,
a formatter for each, and the wording template (§3.2). The recorder **strips** any key not in the
registry and sets `redacted = true` if a stripped key was marked sensitive-by-name. A unit test
asserts that every `type` in §4 has a registry entry and that no registry key is on the deny list
(`token`, `secret`, `arguments`, `authorization`, `assertion`, `code`, `password`, `refresh`).
This gives SC-007 a concrete oracle and stops the discipline from depending on each seam author.

Examples of approved keys: `scope` (delegation), `permission_set`, `provider`, `reason`
(denial-reason label), `tool`, `argument_count`, `expires_in_label`, `client_label`,
`scope_added`, `scope_removed` (grant update delta).

### 3.2 `Explanation` (structured "why")

```go
type Explanation struct {
    Trigger     string   // what asked for this: "Research Agent called delete_repo"
    Basis       Basis    // what decided it: {Kind: grant|permission_set|policy|approval|provider|user, ID, Label}
    Consequence string   // what it means now: "The agent cannot run delete_repo until you approve it."
}
```

Rendered as three short lines in the detail panel. `Basis.ID/Kind` doubles as the FR-008
single-action back-link. Each `type` has a wording template with placeholders for labels; the
templates live next to the registry so SC-003 wording can be reviewed before code exists.

### 3.3 Route templates (open-redirect guard)

`target_route` values are generated server-side only from this allowlist, matching the SPA's
current router (`web/src/App.tsx`, `basename="/"`):

| Purpose | Template |
|---|---|
| Agent delegation detail (revoke lives here) | `/agents/{agent_id}` |
| Third-party sessions (reconnect) | `/sessions` |
| Tool authorization detail | `/approvals/{approval_id}` |
| Tool authorizations list | `/approvals` |
| Activity event | `/activity/{event_id}` |

The client renders only relative paths starting with `/` and no scheme/host.

`/sessions` currently shows expiry text but has no re-authentication control. The session page
must read `/api/activity/attention` and match reconnect items by service ID. It then offers
"Reconnect" for those sessions, even when `is_expired` is false. The action starts the existing
`GET /api/third-party/{serviceId}/oauth2/authorize` flow and returns to `/sessions` after the
OAuth callback. Do not change the existing `is_expired` API meaning (refresh expiry only).

---

## 4. ActivityCategory (enum)

Curated, user-comprehensible taxonomy (spec **Activity Category**; research Decision 10). Maps to
real domain seams (§11).

| Value | Sourced from | Example `type`s |
|---|---|---|
| `delegation` | `UserGrant` create/update | `grant.created`, `grant.updated` (with scope delta) |
| `session` | `UserSession` lifecycle | `session.connected`, `session.refreshed` (roll-up), `session.expired`, `session.terminated` |
| `agent_access` | Successful broker token exchange (roll-up), not downstream use | `agent.access_issued` |
| `policy_decision` | Token-exchange denial with `DenialReason` | `policy.denied`, `policy.blocked` |
| `approval` | `ToolApproval` transitions | `approval.requested`, `approval.approved`, `approval.denied`, `approval.consumed`, `approval.expired` |
| `revocation` | grant/session/approval revoke | `grant.revoked`, `session.terminated`, `approval.revoked` |
| `reconnect` | Successful OAuth callback for an existing session | `session.reconnected` |
| `failure` | Principal-bound refresh failure | `session.refresh_failed` |

---

## 5. ActorKind + ActorRef (spec **Actor**)

`ActorKind`: `agent` | `service` | `user` | `broker`. `ActorRef = {kind, id, display_label}`.
`display_label` is captured at emit time so a since-revoked/removed actor still renders (FR-015).

## 6. SubjectKind + SubjectRef (spec **Affected Subject**)

`SubjectKind`: `grant` | `permission_set` | `service` | `session` | `tool` | `resource`.
`SubjectRef = {kind, id, display_label}`, label captured at emit time.

## 7. Outcome (enum)

`succeeded` | `failed` | `blocked` | `pending`. Distinguished in UI by label + icon + text, never
color alone (FR-019). `blocked` = disallowed by policy/authorization; `failed` = attempted but
errored; `pending` = in flight, no final outcome.

### 7.1 `DenialReason` (new, token-exchange)

Today token-exchange failures are `access_denied`/`invalid_grant` plus free text, and revocation
deletes the grant so "revoked" is indistinguishable from "never granted". Phase 0 adds a typed
reason on the exchange error: `agent_unknown | no_grant | grant_expired | service_not_in_grant |
session_missing | session_expired | policy_denied`. "Revoked" is not a live reason; it is
reconstructed on read from a `grant.revoked` event for the same `(agent, service)`.

---

## 8. NeedsAttentionItem (derived live, NOT persisted)

Spec **Needs-Attention Item**. Computed from current unresolved state on each request (research
Decision 6; FR-003, FR-004). The item is never stored or dismissed. It clears when its source
state resolves. History retention does not suppress unresolved items.

| Field | Type | Notes |
|---|---|---|
| `kind` | `AttentionKind` enum | `reconnect_required` \| `approval_pending`. |
| `title` | `string` | Plain-language situation (e.g. "GitHub needs to be reconnected"). |
| `summary` | `string` | Why it needs attention. |
| `subject_ref` | `SubjectRef` | The service/tool involved. |
| `next_step` | `NextStep` value object | `{label, target_route}` from §3.3: reconnect → `/sessions`; approval → `/approvals/{id}`. |
| `since` | `time.Time` | When the unresolved state began. |
| `group_key` | `string` | Same-kind items sharing a subject collapse in the UI ("4 tool calls waiting → Review all" → `/approvals`). |

**Derivation rules**:
- `reconnect_required`: for a `UserSession` of the principal, either it is past refresh expiry
  (`UserSession.IsExpired`) or its `reconnect_required_at` is set, and an active `UserGrant`
  covers that service. Use the expiry or marker time for `since`. A successful refresh or OAuth
  reconnection clears the marker; an activity event is never the source of this live state.
- `approval_pending`: a `ToolApproval` for the principal with `status=pending` and not expired.
  Clears when approved/denied/consumed/expired.

**Non-actionable notable events** (blocked/denied actions, completed revocations) are history only
and MUST NOT produce a `NeedsAttentionItem` (FR-003).

Per-event `needs_attention` is **not** part of `ActivityEvent`. For `needs_attention=true`,
derive the principal's unresolved items from current state before querying retained events:
- A pending, unexpired approval matches only its `approval.requested` event by `approval_id`.
  A tool name or display subject cannot identify an approval.
- A reconnect-required session matches only its `session.expired` and `session.refresh_failed`
  transitions by `session_id` and the current `token_revision`. Match the exact expiry
  `dedup_key` or the refresh-failure key prefix for that revision, not `service_id` alone.

Apply this predicate with every other selected filter and the retention cutoff. An unresolved
item whose event is missing or outside retention stays visible on `/api/activity/attention`;
it does not cause unrelated historical events to enter the filtered feed.

**Live session state**: add `reconnect_required_at *time.Time` and a monotonically increasing
`token_revision` to `UserSession`, its memory/PostgreSQL adapters, and the `user_sessions` table
(migration `032`). Existing sessions start at revision 1. A new session insert has revision 1;
each later successful token write (OAuth callback or refresh) increments it and clears the marker
in the same repository write. `UserSessionRepository.Create` fills the passed session with its
committed ID and revision. A callback with revision 1 emits `session.connected`, otherwise
`session.reconnected`.

When an upstream refresh is rejected or no usable refresh token exists after access expiry,
the session-status port sets `reconnect_required_at` to the first failure time. It does this
only if the stored session ID and token revision match those read before the attempt. Both
adapters implement the conditional update. A late failure cannot reinstate the marker after
a new token write. Persist this state before enqueueing a historical failure or expiry event.
The marker is live session state, not an activity event; a failed event insert cannot undo it.
`ListActiveByPrincipal` excludes marked sessions.

**Status port**: `SessionReconnectStatusRepository` in `internal/ports/storage.go` provides
`MarkReconnectRequired(ctx, principal, sessionID, expectedRevision, at) (bool, error)`.
`false` means a newer token write or deletion won. The PostgreSQL adapter makes this a
conditional update; the memory adapter checks the revision under its session lock.

---

## 9. ExplorationContext (query model — spec **Exploration Context**)

Filter dimensions for `GET /api/activity` (US3, FR-006). Combinable and clearable; URL-synced.

| Dimension | Query param | Values |
|---|---|---|
| Agent | `agent_id` | agent ID |
| Connected service | `service_id` | service ID |
| Grant / permission context | `grant_id` | grant ID |
| Time window (relative presets) | `window` | `24h` \| `7d` \| `30d` \| `all` (within retention) |
| Jump-to-date | `before` | RFC3339; upper bound for `occurred_at` (no range picker needed) |
| Outcome | `outcome` | `succeeded` \| `failed` \| `blocked` \| `pending` |
| Category | `category` | any `ActivityCategory` |
| Keyword | `q` | matched against `summary`, actor/subject labels (`pg_trgm`) |
| Needs-attention | `needs_attention` | `true` (see §8) |
| Pagination | `cursor`, `limit` | opaque cursor bound to the filter set; `limit` bounded by config |

`ActivityQuery` also carries a server-computed `retention_cutoff`. The client cannot supply it.
The effective feed lower bound is the later of this cutoff and the selected window's start.

Empty result of a filter combination → "no matches" state (FR-014); no filters + no events →
"no activity yet" state.

---

## 10. Persistence & storage mapping (Principle IX, ADR 004, ADR 037)

**Port** `ActivityEventRepository` in `internal/ports/storage.go` (ISP, ≤7 methods):
`Record(ctx, ActivityEvent) error` (deduplicates single events, increments roll-ups per new observation; §1.1/§1.2),
`ListByPrincipal(ctx, principal, q ActivityQuery) ([]ActivityEvent, Cursor, error)` (query carries the retention cutoff),
`GetByID(ctx, principal, id, cutoff time.Time) (ActivityEvent, error)`,
`ListRelated(ctx, principal, ref RelatedRefSelector, windowStart, cutoff time.Time, limit int) ([]ActivityEvent, int, error)`
(thread assembly; returns the retained total count), `PruneOlderThan(ctx, cutoff, batch) (int, error)`.
The read service computes one `occurred_at >= now - retention_window` cutoff per request and
applies it to the feed, the detail lookup, every related sequence, and thread counts. The
effective related-sequence lower bound is `max(windowStart, cutoff)`. An owned event before
the cutoff is not found even when pruning has not deleted its row.

**Recorder durability (best-effort after the primary write, research Decision 12)**:
- Grant upsert/revoke, approval transitions, and session store/terminate enqueue only after the
  repository write succeeds (including its internal transaction commit). The recorder does not
  share a transaction with these writes. It captures labels and related IDs before deletion.
- All seams use a bounded, non-blocking queue (`activity.queue_size`, default 1024) drained by
  one goroutine per instance. The worker uses an application-lifecycle context, not a completed
  request context. On a full queue or insert error, it increments the OTel counter
  `activity_events_dropped_total{type}` and logs at `ERROR`. Recording never changes the result
  of the primary operation. Successful state changes and events are not atomic.

**Prune (research Decision 8)**: no scheduler exists in the app. The async drainer, every
`activity.prune_interval` (default `10m`), attempts `pg_try_advisory_lock(hashtext('activity_prune'))`;
if acquired it deletes `activity.prune_batch` (default 5000) rows with `recorded_at < now() -
retention` and releases. Multi-replica safe; read-window filtering stays authoritative so lag is
harmless. The memory adapter prunes inline.

**Expiry observation (US5)**: on the worker's existing periodic tick, scan bounded batches of
the current session revisions that need reconnection and approvals still pending past
`expires_at`. A session qualifies after refresh expiry, or after access expiry when it has no
usable refresh token. Access expiry alone does not qualify when refresh remains available.
Use refresh/access expiry or `expires_at` as `occurred_at`; do not need a token exchange or
approval-detail request. A small expiry-read port in `internal/ports/storage.go` selects due
sessions and pending approvals with their principals, IDs, and session revisions. Both storage
adapters implement bounded, indexed queries. Limit to transitions inside retention and exclude
keys already recorded; this prevents old pending rows from starving new expiries. Resume each
batch on the next tick. Concurrent replicas may observe the same expiry: the unique
`(principal, dedup_key)` constraint absorbs duplicate reports. Before an OAuth callback
overwrites an expired session, capture its old revision. After the successful token write,
enqueue expiry ahead of reconnection. This closes the gap between worker ticks. The recorder
remains best-effort; the source session and approval state stays authoritative.

**Config** (`internal/ports/config.go`, `activity` section): `retention_window` (Go duration,
default `720h` = 30 days), `page_size` (25), `rollup_bucket` (`1h`), `queue_size` (1024),
`prune_interval` (`10m`), `prune_batch` (5000), `attention_poll_interval_seconds` (30; surfaced
to the SPA via the existing config endpoint or a build-time constant).

### Table sketch `activity_events`

| Column | Type | Constraints / Index |
|---|---|---|
| `id` | `uuid` | PK |
| `principal` | `text` | NOT NULL |
| `dedup_key` | `text` | NOT NULL; `UNIQUE (principal, dedup_key)` |
| `category` | `text` | NOT NULL; `CHECK` in enum |
| `type` | `text` | NOT NULL |
| `actor_kind` | `text` | NOT NULL; `CHECK` |
| `actor_id` | `text` | NULL |
| `actor_label` | `text` | NOT NULL |
| `subject_kind` | `text` | NOT NULL; `CHECK` |
| `subject_id` | `text` | NULL |
| `subject_label` | `text` | NOT NULL |
| `outcome` | `text` | NOT NULL; `CHECK IN (succeeded,failed,blocked,pending)` |
| `summary` | `text` | NOT NULL |
| `detail` | `jsonb` | NOT NULL DEFAULT `'{}'` |
| `related_refs` | `jsonb` | NOT NULL DEFAULT `'{}'` |
| `occurrence_count` | `integer` | NOT NULL DEFAULT 1 CHECK (>=1) |
| `first_occurred_at` | `timestamptz` | NOT NULL |
| `occurred_at` | `timestamptz` | NOT NULL |
| `recorded_at` | `timestamptz` | NOT NULL DEFAULT now() |
| `correlation_id` | `text` | NULL |
| `redacted` | `boolean` | NOT NULL DEFAULT false |

**Indexes**:

1. `(principal, occurred_at DESC, id DESC)` — feed/keyset cursor.
2. `UNIQUE (principal, dedup_key)` — idempotent insert/upsert.
3. Partial expression B-trees `(principal, (related_refs->>'agent_id'), occurred_at DESC, id DESC)`
   and the `service_id`, `grant_id`, `approval_id` forms, each `WHERE related_refs ? '<key>'` —
   selective filters **and** read-time thread assembly.
4. `GIN (summary gin_trgm_ops)` (extension `pg_trgm`) — the `q=` keyword filter.
5. `(recorded_at)` — bounded prune batches.
6. `outcome`/`category` deliberately have no standalone index until the exploratory benchmark
   shows one is needed.

Add due-time indexes for current session expiry and pending approval `expires_at` in migration
`032`. The due-query port excludes already recorded `(principal, dedup_key)` keys through the
activity-event unique index; expired source rows older than retention do not enter the scan.

`DOWN` drops the table (and leaves `pg_trgm` in place if it pre-existed). No foreign keys reference
agents, services, or sessions: events must survive deletion of the referenced entity (FR-015).

### Exploratory storage benchmark

Unchanged in intent (see [research.md](./research.md) Decision 2), but with roll-ups the
10,000-events-per-principal fixture must be seeded as *distinct* curated events, and the benchmark
additionally measures upsert contention on the roll-up path (330 writes/s hitting a handful of
`(principal, dedup_key)` rows).

---

## 11. Emission seams (where events are recorded)

Verified against the code on 2026-09-22. Seams marked **(Phase 0)** do not exist yet and are
created in the plan's Phase 0.

| Seam (existing code) | Emitted event | Mode | Notes |
|---|---|---|---|
| `consent` service `UserGrant` upsert | `delegation` / `grant.created`\|`grant.updated` | post-write async | Include `scope_added`/`scope_removed` delta in context; skip unchanged grants. |
| `consent` service revoke / empty-set (`internal/domain/consent/service.go` ~`:675`) | `revocation` / `grant.revoked` | post-write async | Capture labels before deletion; enqueue only after a successful delete. |
| `oauth2session` store (new session or OAuth callback reconnect) | `session` / `session.connected` or `reconnect` / `session.reconnected` | post-write async | Select the type from committed revision (1 = new); use the returned session ID. |
| `oauth2session` refresh success / failure | `session.refreshed` (roll-up) / `failure` `session.refresh_failed` | async | Persist live reconnect state on failure; clear it with a successful token write, independently of event recording. |
| `oauth2session` expired session on periodic observation, lazy `ErrSessionExpired`, or successful OAuth callback that replaced an expired revision | `session` / `session.expired` | async | Use the old session ID and token revision. Worker/lazy detection marks only a current revision. Callback captures the old one before its token write, then enqueues expiry after success, without restoring the marker. |
| `oauth2session` terminate | `revocation` / `session.terminated` | post-write async | |
| `TokenExchangeService.Exchange` success (`internal/domain/tokenexchange/service.go:170`) **(Phase 0 hook)** | `agent_access` / `agent.access_issued` (roll-up) | async | Counts successful broker exchanges, not downstream actions or ExtProc cache hits. |
| `TokenExchangeService.Exchange` failure with `DenialReason` **(Phase 0)** | `policy_decision` / `policy.denied`\|`policy.blocked` | async | `policy_denied` → `blocked`; others → `failed`/`blocked` per §7. |
| `approval.Create` when `IsNew` (`internal/domain/approval/service.go:360-378`) | `approval` / `approval.requested` | post-write async | **Do not emit on the idempotent hit.** |
| `approval.Approve/Deny/Consume/Revoke` | `approval.*` | post-write async | Consume is already idempotent. |
| `approval` pending past `expires_at`, observed by the periodic worker or lazy detail access | `approval.expired` | async | `occurred_at = expires_at`; emit once by approval ID without requiring a detail request. |

Local `/oauth2/token` grant failures remain in structured operational audit logs. The public
token route and its `TokenRequestFailed` log do not establish a user principal. Never infer the
owner from `client_id`, an unverified authorization code, or a caller-supplied header; without a
verified principal, no `ActivityEvent` is emitted.

Emission never blocks or fails the primary security operation (fail-open on recording,
fail-closed remains on the operation itself); see §10 for how drops are made observable.
