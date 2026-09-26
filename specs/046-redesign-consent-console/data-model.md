# Data Model — Consent UI v2

**Status**: Proposed design. Existing aggregates retain their ownership and authorization semantics.

## Existing entities and projections

| Entity or value | Relevant fields | Invariant |
| --- | --- | --- |
| Agent | Typed AgentID, display name, CIMD metadata, service requirements, permission sets | Domain verification never claims legal publisher verification |
| UserGrant | Principal, AgentID, `granted_permission_sets`, selected services, validity | One grant per principal/agent pair. Consent preserves existing selected access |
| Permission Set | Identity, required/optional assignment, service scopes | Required groups remain locked. Optional scope catalogues do not imply required access |
| UserSession | Principal, ServiceID, encrypted tokens, granted scope, expiry, refresh capacity | One principal/service session. Connection does not imply delegation |
| ToolApproval | ID, owner, agent, tool, arguments, status, persistence, scope, expiry | Only the owner resolves a pending request. Resolved/expired requests cannot be resubmitted |
| Authorization context | Existing `session_token`, selected access, callback state | Backend validation remains authoritative. UI decoding never authorizes a request |

No existing entity receives new token semantics. No credential enters browser preference storage or activity records.

## ConsentDraft: transient UI state

Fields: requested permission-set/service selections, existing granted selections, selected duration, custom expiry, dirty state, and the existing authorization-session reference.

The decision flow preserves `consent_state` across the existing provider authorization callback. It does not introduce a new persisted consent model.

Transitions:

- Loaded request → unchanged or edited draft.
- Edited draft → validated submission → server outcome and existing safe continuation.
- Edited draft → Cancel → original selections in console context.
- Decision draft → Deny → local terminal outcome, without a mutation or constructed redirect.
- Invalid or expired authorization context → terminal error, never management-mode fallback.

Re-consent computes a selection delta from the existing grants read response. Previously granted permission sets and services remain selected when the user allows new access. The UI never silently widens the grant.

## Session read-model addition

### Field

`required_scopes?: string[]` is the sole proposed additive session field. It belongs to `SessionStatus` and the existing flat `UserSessionSummary` representation where that representation is returned.

The field is additive and optional for old consumers. New servers emit a sorted, duplicate-free array after successful aggregation. Existing `scope`, expiry, `is_expired`, and refresh fields retain their meaning.

### Derivation

The domain computes requirements for the acting principal's active grants that reference this service. It excludes another principal's grants, expired/revoked grants, and unselected optional permission sets or services.

Reuse explicit service requirements and the existing `RequireAllScopes` resolution. For selected services, resolve required scope coverage from the applicable granted permission sets. Do not use every available provider scope.

Load requirements and permission sets in batches for the session list. Do not add a query per session, agent, or scope. No persistence migration is needed for this derived field.

`[]` means the successful computation found no required scopes for the applicable grants. An absent field means coverage is unknown, not empty. A requirement lookup failure remains an error, not a fabricated `[]`.

### ConnectionState: derived presentation

The UI uses a value object, not another response field or stored column.

| Condition, in precedence order | Visible state | Available action |
| --- | --- | --- |
| No session | No connection | Connect through the existing authorization flow |
| Known rejected refresh result or other authoritative unusable-token result | Needs re-authentication | Reconnect |
| Refresh token expired, or access expired without usable refresh capacity | Expired | Reconnect |
| Access expired with apparently usable refresh token, but refresh not yet successful | Needs re-authentication, with refresh explanation | Refresh when supported, otherwise reconnect |
| Usable access and granted scopes lack a known required scope | Missing scopes | Reconnect with existing required-scope flow |
| Usable access and known required scope coverage is complete | Connected | Existing refresh and confirmed disconnect |
| Scope coverage cannot be established | Scope coverage unknown | Retry the read, without claiming Connected |

The fourth row avoids calling a session Connected before a failed refresh reveals itself. A successful refresh recomputes the state. Unknown token lifetime follows existing domain usability semantics, not a fabricated expiry.

A refresh failure that is only a network error does not prove credential rejection. Show the operation error and preserve the last authoritative state as stale.

No last-use value is invented. Before activity exists, display “Not recorded”. Later, label the latest known successful event as recorded use, not a complete history guarantee.

## ActivityEvent: new immutable domain entity

| Field | Domain type | Persistence / response | Rule |
| --- | --- | --- | --- |
| ID | `id.ActivityEventID` | UUID / `id` | New typed ID generated through `internal/domain/id/gen_ids.go` |
| Principal | `id.Principal` | Principal column / not returned | Required authenticated owner, never accepted as a query parameter |
| OccurredAt | UTC time | TIMESTAMPTZ / `occurred_at` | Server observation time |
| EventType | Closed enum | Text / `event_type` | `grant.create`, `grant.update`, `grant.revoke`, `approval.approve`, `approval.deny`, `approval.revoke`, `session.disconnect`, `token.exchange` |
| Outcome | Closed enum | Text / `outcome` | `succeeded`, `denied`, or `failed` |
| AgentID | Optional `id.AgentID` | Nullable UUID / `agent_id` | Only an attributable agent, never an unverified client claim |
| ServiceID | Optional `id.ServiceID` | Nullable UUID / `service_id` | Only an applicable service |

Event type identifies the attempted operation. Outcome identifies its result. A failed grant creation is `grant.create` with `failed`, not a claim that a grant exists.

No free-form payload, raw error string, token, credential, secret, tool arguments, or serialized request belongs in this entity. The UI maps enum values to centralized safe copy.

IDs remain valid historical references after agent or service deletion. Do not cascade-delete activity through foreign keys. The UI can show a deleted-record label and the identifier.

### Lifecycle and recording

The event lifecycle is append → readable within retention → excluded at expiry → deleted by cleanup. There is no update or user deletion API.

Record the action outcome after the original action commits or returns its attributed denial/failure. The append uses a bounded context and storage timeout. No per-request unbounded goroutine or retry queue is required.

Activity append is outside the original transaction. On append failure, preserve the original response and produce credential-free operational logging. Existing security audit logging remains mandatory.

Record from the consent, approval, session, and token-exchange service boundaries through an injected recorder port. Builders construct the recorder and repositories. HTTP handlers only parse filters and call the activity service.

Record token exchange only after authentication identifies the principal. Authentication failures without a trustworthy principal remain operational audit events, not user history. Never attribute an event from unvalidated JWT content.

Local consent Deny performs no backend request. It creates no activity event. This limit follows the prohibition on another write API. Approval denial and backend-observed attributable denials remain in scope.

### Repository and storage

Add a focused ActivityEventRepository in `internal/ports/storage.go` with Append, ListByPrincipal, and DeleteBefore operations. Keep query validation and retention policy in the domain service.

Implement memory and PostgreSQL adapters. PostgreSQL uses sqlx, domain storage errors, and configured storage timeouts. Add the next migration pair, expected `029_create_user_activity_events.{up,down}.sql` after the current 028 migration. Recheck sequence during implementation.

Use a primary UUID key and indexes beginning with principal, followed by filter columns where useful and `(occurred_at DESC, id DESC)`. The unfiltered principal/time/id index is required. The agent and type filtered reads must retain stable ordering. Add an occurrence-time index for bounded cleanup batches.

Use a builder-owned cleanup worker with startup cleanup, a fixed daily schedule, bounded deletion batches, and cancellation during shutdown. Retention is fixed at 90 days, not a new runtime setting.

### Query and pagination

`GET /api/activity` uses principal from authentication context. Optional filters are `agent_id` and `event_type`. `limit` defaults to 50 and is capped at 100.

Use keyset pagination ordered by `(occurred_at DESC, id DESC)`. Fetch `limit + 1` records to determine the next page. Each query enforces both principal ownership and `occurred_at >= now - 90 days`, including queries with a cursor.

The opaque cursor contains a version, first-page snapshot time, last timestamp/ID, and filter values. Validate its shape and filter agreement. It does not select a principal and is not an authorization credential. Editing or copying it cannot bypass the ownership or retention predicates.

A snapshot bound excludes new events from later pages. Retention still advances between requests. If records expire during traversal, later pages can be shorter. Omit the next cursor at the end.

Do not backfill from logs. History starts when recording deploys. Empty history is valid and does not prove that no earlier access occurred.

## Browser preferences

| Key | Values | Default | Scope |
| --- | --- | --- | --- |
| `aib.theme` | light, dark, system | system | Browser |
| `aib.approval-persistence` | once, session, permanent | once | Browser |
| `aib.sidebar-collapsed` | true, false | false | Browser |

Preferences never contain authorization context or credentials. They do not change previous decisions. A remembered persistence default still requires explicit user confirmation and scope review.

## Contract approval

The [OpenAPI proposal](contracts/enduser-additions.openapi.yaml) and [session note](contracts/session-state.md) define the additive contracts. Promote approved additions into `api/enduser/openapi.yaml` before backend implementation. Update `docs/api/` and TypeScript types in the same implementation change.
