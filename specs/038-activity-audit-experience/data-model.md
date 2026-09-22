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
| `significance` | `Significance` enum | `security` \| `business`. Drives emphasis; all persisted events are already curated-significant. |
| `summary` | `string` | Server-composed, human-readable, redacted sentence answering what/who/what-affected/outcome (FR-002, FR-010), produced from the per-`type` wording template (§3.2). |
| `detail` | `ActivityDetail` (value object, JSONB) | Structured explanation, approved context, related links (§3). |
| `related_refs` | `RelatedRefs` (value object, JSONB) | Foreign IDs for filtering, read-time threading, and outbound links: `agent_id?`, `service_id?`, `grant_id?`, `session_id?`, `approval_id?`, `client_id?`, `resource_uri?`. |
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
- `category`, `actor_kind`, `subject_kind`, `outcome`, `significance` MUST be valid enum members.
- `dedup_key` non-empty and deterministic for the underlying action (§1.1). A second `Record`
  with the same `(principal, dedup_key)` is a no-op (or a counter increment for roll-up types).
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

### 1.1 Deduplication keys (FR-016b)

The same underlying action can reach the recorder more than once: each extproc replica keeps its
own token cache and re-exchanges on eviction/TTL; the 5 s re-auth cooldown retries; expiry of
sessions and approvals is detected lazily on every read; approval creation is itself idempotent
(`CreateApprovalResult.IsNew == false` on a repeated request). The recorder therefore derives a
deterministic `dedup_key` per `type` and inserts with `ON CONFLICT (principal, dedup_key) DO
NOTHING` (or `DO UPDATE` for roll-ups). Keys:

| `type` | `dedup_key` |
|---|---|
| `grant.created` / `grant.updated` | `grant:{grant_id}:{updated_at_unix}` |
| `grant.revoked` | `grant.revoked:{grant_id}` |
| `session.connected` | `session.connected:{session_id}` |
| `session.refreshed` (roll-up) | `session.refreshed:{session_id}:{bucket}` |
| `session.refresh_failed` | `session.refresh_failed:{session_id}:{bucket}` |
| `session.expired` | `session.expired:{session_id}:{refresh_expires_at_unix}` |
| `session.terminated` | `session.terminated:{session_id}` |
| `agent.acted_via_service` (roll-up) | `agent.acted:{agent_id}:{service_id}:{bucket}` |
| `policy.denied` / `policy.blocked` | `policy:{agent_id}:{service_id}:{reason}:{bucket}` |
| `approval.requested` | `approval.requested:{approval_id}` (emit only when `IsNew`) |
| `approval.approved` / `.denied` / `.consumed` / `.revoked` | `approval.{state}:{approval_id}` |
| `approval.expired` | `approval.expired:{approval_id}` |
| `token.issue_failed` | `token.issue_failed:{client_id}:{reason}:{bucket}` |

`bucket` = `occurred_at` truncated to `activity.rollup_bucket` (default `1h`, config §10).

### 1.2 Roll-ups (FR-001 "summarize into milestones")

Routine, high-frequency successes are not stored individually. For roll-up types
(`agent.acted_via_service`, `session.refreshed`, and the bucketed failure/denial types) the
recorder upserts one event per `(principal, dedup_key)` bucket:

```sql
INSERT INTO activity_events (...) VALUES (...)
ON CONFLICT (principal, dedup_key) DO UPDATE
   SET occurrence_count = activity_events.occurrence_count + 1,
       occurred_at      = GREATEST(activity_events.occurred_at, EXCLUDED.occurred_at);
```

The card reads "Research Agent used GitHub · 41 times between 09:00 and 10:00". This keeps the
feed proportional to *distinct* things that happened rather than to MCP traffic, which is what
makes SC-005 reachable. The bucket is UTC-aligned in storage; the client renders the bucket span
in the user's local time.

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
| `pending` | `bool` | Mirror of `outcome=pending` for honest rendering. |

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

---

## 4. ActivityCategory (enum)

Curated, user-comprehensible taxonomy (spec **Activity Category**; research Decision 10). Maps to
real domain seams (§11).

| Value | Sourced from | Example `type`s |
|---|---|---|
| `delegation` | `UserGrant` create/update | `grant.created`, `grant.updated` (with scope delta) |
| `session` | `UserSession` lifecycle | `session.connected`, `session.refreshed` (roll-up), `session.expired`, `session.terminated` |
| `agent_action` | Token-exchange milestone (roll-up) | `agent.acted_via_service` |
| `policy_decision` | Token-exchange denial with `DenialReason` | `policy.denied`, `policy.blocked` |
| `approval` | `ToolApproval` transitions | `approval.requested`, `approval.approved`, `approval.denied`, `approval.consumed`, `approval.expired` |
| `revocation` | grant/session/approval revoke | `grant.revoked`, `session.terminated`, `approval.revoked` |
| `reconnect` | live reconnect-required derivation | `service.reconnect_required` |
| `failure` | token issuance / refresh failure | `token.issue_failed`, `session.refresh_failed` |

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
Decision 6; FR-003, FR-004). Never stored or dismissed; it clears automatically. History retention
does not suppress unresolved items.

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
- `reconnect_required`: for a `UserSession` of the principal, **either** it is past refresh
  expiry (`UserSession.IsExpired`) **or** the most recent `session.refresh_failed` event for that
  service is newer than the most recent `session.connected`/`session.refreshed` — **and** an
  active `UserGrant` covers that service. Clears when the session is re-authenticated.
- `approval_pending`: a `ToolApproval` for the principal with `status=pending` and not expired.
  Clears when approved/denied/consumed/expired.

**Non-actionable notable events** (blocked/denied actions, completed revocations) are history only
and MUST NOT produce a `NeedsAttentionItem` (FR-003).

Per-event `needs_attention` is **not** part of `ActivityEvent`. The feed filter
`needs_attention=true` is defined as "events whose `related_refs` reference the subject of a
current attention item" and is implemented as a subquery over that small set.

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

Empty result of a filter combination → "no matches" state (FR-014); no filters + no events →
"no activity yet" state.

---

## 10. Persistence & storage mapping (Principle IX, ADR 004, ADR 037)

**Port** `ActivityEventRepository` in `internal/ports/storage.go` (ISP, ≤7 methods):
`Record(ctx, ActivityEvent) error` (idempotent upsert per §1.1/§1.2),
`ListByPrincipal(ctx, principal, q ActivityQuery) ([]ActivityEvent, Cursor, error)`,
`GetByID(ctx, principal, id) (ActivityEvent, error)`,
`ListRelated(ctx, principal, ref RelatedRefSelector, window, limit) ([]ActivityEvent, int, error)`
(thread assembly; returns total count), `PruneOlderThan(ctx, cutoff, batch) (int, error)`.

**Recorder durability (two modes, research Decision 12)**:
- *Transactional*: seams that already run inside a PostgreSQL transaction (grant upsert/revoke,
  approval transitions, session store/terminate) call `Record` with the same `sqlx.Tx`, so the
  state change and its event commit or roll back together.
- *Async*: the token-exchange hot path and lazy-expiry detections enqueue to a bounded channel
  (`activity.queue_size`, default 1024) drained by one goroutine per instance. On a full queue or
  insert error the recorder increments the OTel counter `activity_events_dropped_total{type}` and
  logs at `ERROR` — this is the "operational error" FR-016a requires. It never blocks or fails the
  primary operation.

**Prune (research Decision 8)**: no scheduler exists in the app. The async drainer, every
`activity.prune_interval` (default `10m`), attempts `pg_try_advisory_lock(hashtext('activity_prune'))`;
if acquired it deletes `activity.prune_batch` (default 5000) rows with `recorded_at < now() -
retention` and releases. Multi-replica safe; read-window filtering stays authoritative so lag is
harmless. The memory adapter prunes inline.

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
| `significance` | `text` | NOT NULL; `CHECK IN (security,business)` |
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
| `consent` service `UserGrant` upsert | `delegation` / `grant.created`\|`grant.updated` | tx | Include `scope_added`/`scope_removed` delta in context. |
| `consent` service revoke / empty-set (`internal/domain/consent/service.go` ~`:675`) | `revocation` / `grant.revoked` | tx | Revocation deletes the grant; the event is the only durable record of it. |
| `oauth2session` store (new session) | `session` / `session.connected` | tx | |
| `oauth2session` refresh success / failure | `session.refreshed` (roll-up) / `failure` `session.refresh_failed` | async | |
| `oauth2session.GetValidAccessToken` → `ErrSessionExpired` (`service.go` ~`:1155`) **lazy** | `session` / `session.expired` | async | `occurred_at = refresh_expires_at`; dedup key makes repeated detection a no-op. |
| `oauth2session` terminate | `revocation` / `session.terminated` | tx | |
| `TokenExchangeService.Exchange` success (`internal/domain/tokenexchange/service.go:170`) **(Phase 0 hook)** | `agent_action` / `agent.acted_via_service` (roll-up) | async | `TokenIssued` does **not** cover RFC 8693; hook `Exchange` directly. |
| `TokenExchangeService.Exchange` failure with `DenialReason` **(Phase 0)** | `policy_decision` / `policy.denied`\|`policy.blocked` | async | `policy_denied` → `blocked`; others → `failed`/`blocked` per §7. |
| oauth2 grant strategies `TokenRequestFailed` (`token_grant_strategy.go`) | `failure` / `token.issue_failed` | async | Local grants only. |
| `approval.Create` when `IsNew` (`internal/domain/approval/service.go:360-378`) | `approval` / `approval.requested` | tx | **Do not emit on the idempotent hit.** |
| `approval.Approve/Deny/Consume/Revoke` | `approval.*` | tx | Consume is already idempotent. |
| `approval` lazy expiry detection (`service.go` ~`:190`) | `approval.expired` | async | `occurred_at = expires_at`. |

Emission never blocks or fails the primary security operation (fail-open on recording,
fail-closed remains on the operation itself); see §10 for how drops are made observable.
