# Data Model — Consent UI v2

**Status**: Approved UI design, amended 2026-10-04 and aligned with the branch implementation on 2026-10-05. Existing aggregates retain ownership and authorization semantics.

## Existing entities and projections

| Entity or value | Relevant fields | Invariant |
| --- | --- | --- |
| Agent | Typed AgentID, display name, CIMD metadata, service requirements with `connectionStatus`, permission sets, governance and documentation links | The Agent Origin Label derives only from the authorization session and never claims legal publisher verification. No existing response carries a publisher |
| UserGrant | Principal, AgentID, `granted_permission_sets`, selected services, validity | One grant per principal/agent pair. Consent preserves existing selected access |
| Delegation (projection of an unexpired UserGrant) | `agentId`, `displayName`, `activeGrantCount`, `lastModifiedAt`, `expiresAt` | The existing list excludes expired grants and names no services. `activeGrantCount` counts grants, not permission sets; do not show it or fetch per-agent counts. Agents cards show expiry, a warning within seven days of expiry, and the `lastModifiedAt` date as “Last updated”, never last use. Remove a card or row when `expiresAt` passes |
| Permission Set | Identity, name, description, required/optional assignment, services | Required groups remain locked. The UI presents the name and description, never raw scope strings |
| UserSession | Principal, ServiceID, encrypted tokens, granted scope, expiry, refresh capacity | One principal/service session. Connection does not imply delegation. The session list returns stored sessions only and carries no provider account identifier |
| ToolApproval | ID, owner, agent, tool, arguments, status, persistence, scope, expiry | Only the owner resolves a pending request. Resolved/expired requests cannot be resubmitted |
| Authorization context | Existing `session_token`, selected access, callback state, redirect metadata | Only the authorization session supplies an Agent Origin Label and consent redirect host. Backend validation remains authoritative; UI decoding never authorizes a request |

No existing entity receives new token semantics. No credential enters browser preference storage.

## ConsentDraft: transient UI state

Fields: requested permission-set/service selections, existing granted selections, selected duration, custom expiry, dirty state, existing authorization-session reference, and the original same-origin return URL for a provider interruption. The visible consent form uses one shared `PermissionPanel` and `DurationSelect` with the console detail view; this reuse does not change data ownership.

The decision flow uses the existing `consent_state_id` provider callback. A tab-local record stores the canonical selections, duration, custom date, and original same-origin return URL for at most 15 minutes. The form POST sends only the opaque UUID and clean return path. Restoration requires matching ID, service, origin, path, and expiry, then removes the ID from browser history. Backend authorization-session validation remains authoritative.

A Connect start without selections, with the default duration, and without a query or fragment in the return URL stores no draft. It uses the existing GET authorize link with `redirect_uri` instead of the form POST.

Transitions:

- Loaded request → unchanged or edited draft.
- Edited draft → validated submission → server outcome and existing safe continuation.
- Edited draft → Cancel → original selections in console context.
- Decision draft → Deny → local terminal outcome, without a mutation or constructed redirect.
- Invalid or expired authorization context → terminal error, never management-mode fallback.

Re-consent computes a selection delta from the existing grants read response. Previously granted permission sets and services remain selected and read-only in the decision view when the user allows new access; only the console detail view changes or removes them. The UI never silently widens the grant.

A grant has one validity. Re-consent starts from an unexpired grant: Until revoked when `valid_until` is absent, otherwise Custom date. An expired lookup result is not prior active access. Unless the user changes the duration, submission preserves a still-future `valid_until` exactly. A changed duration applies to the whole grant.

Custom expiry is a local calendar date. Submission serializes local midnight, and reopening restores the same local date from the timestamp. Cards and detail headers show expiry in local time. The console previews saved access against the changed duration before Save. Dirty console drafts require explicit discard before internal navigation. Document departure uses the browser warning, except for deliberate draft-preserving provider navigation.

The editor prevents removing the final selected permission group or the final selected service within a group. These minimum-selection constraints do not make optional access required: optional choices remain removable when another selection remains. Console guidance points to Revoke access for removing all agent access. Save-time validation still rejects empty restored drafts.

Required service status is explicit beside each service name wherever it constrains editing. In the console, Granted describes saved access, not the checkbox state. Unsaved group selection changes show Removal pending or Addition pending. Service-only edits keep Granted while the group remains selected. A successful save adopts the accepted grant; a failed save leaves saved badges and pending edits unchanged.

## ConnectionState: derived presentation

The UI derives this value object from existing session fields, refresh responses in the current page, and the agent requirement `connectionStatus`. It adds no response field, stored column, or backend state.

| Condition, in precedence order | Visible state | Available action |
| --- | --- | --- |
| Session read fails, or a refresh fails without an authoritative rejection | Unavailable, with the last authoritative state kept as stale, never Connected from missing data | Retry the read |
| No stored session for an agent service requirement (`connectionStatus: not_connected`) | No connection badge, only in the agent-detail Connections rail. Consent marks the selected service with a warning icon and an inline Connect action | Connect through the existing authorization flow; the consent primary slot reads "Connect {Service} to continue" until callback return |
| Known rejected refresh result in the current page | Needs re-authentication (badge “Needs sign-in”) | Reconnect |
| Session expired, refresh token expired, or access expired without a refresh token | Expired | Reconnect |
| Access expired with a refresh token, but refresh not yet successful | Needs re-authentication, with refresh explanation | Refresh when supported, otherwise reconnect |
| Usable access | Connected | Confirmed Disconnect only; the card offers no Refresh |

`/connections` lists stored sessions only, so it never shows No connection. Reconnect returns to `/connections`, and the page replaces the callback URL with `/connections` after it reads the result. The broker callback also sends provider errors and a missing return URL to `/connections`. The provider authorize operation and callback parameters remain unchanged.

A known rejected refresh result is a `409` (no valid refresh token) or `502` (provider rejected the refresh) response to the existing refresh call in the current page. It lasts until a successful refresh or reconnect. After a reload, the session fields decide the state again, so an expired access token with a refresh token shows the fifth row.

The fifth row avoids calling a session Connected before a failed refresh reveals itself. A successful refresh recomputes the state. Unknown token lifetime follows existing domain usability semantics, not a fabricated expiry.

The connection hook schedules an update at the earliest known refresh-token expiry. The update changes visible state and actions without a fetch. New session data replaces the scheduled deadline. Unmount cancels it. Unknown lifetimes and server-supplied expiry booleans retain their existing semantics.

A refresh failure that is only a network error, or any other non-authoritative failure, does not prove credential rejection. Show the operation error, keep the last authoritative state as stale, and show Unavailable with Retry on the card. A `404` means the session no longer exists; refetch the list.

Existing session data does not identify required scope coverage. The UI shows no Missing scopes state and never infers a gap from available provider scopes. It shows no last-use time.

## Collection and approval projections

`EntityCard` and `EntityRow` present the same resource without changing Agent, UserGrant, UserSession, or ToolApproval. An entity has an ID-derived safe local tint or logo, one name, one supporting line, at most one status badge, optional meta, and at most two visible actions. A list-versus-grid preference changes presentation only; it never changes a grant, connection, query principal or scope. Page choice wins, then an explicit Appearance default, then the initial density fallback (list above twelve items, otherwise cards). Choosing a global default clears earlier Agents/Connections page choices and updates both collections; later page choices remain local. Approvals use fixed-height rows.

The pending approval projection shows tool, agent, server-provided risk, and request age, or the remaining minutes when less than one hour remains. A selected approval's detail panel shows existing arguments, exact scope, and expiry, with once, session and permanent choices. The existing review route at `/approvals/:id` shares that detail content. The remembered projection includes existing permanent allow and deny decisions under `/approvals/remembered`; its `filter` query (all, allowed, denied) does not change decisions. Pending list rows never expand for persistence or scope editing. Expired and resolved approvals have no decisions, whether opened in the panel or review route. At the expiry time, the selected panel removes its actions without a refetch; the row leaves at the next pending-list refresh. Revoke and decision requests remain bound to the acting principal.

## Browser preferences

| Key | Values | Default | Scope |
| --- | --- | --- | --- |
| `aib.theme` | light, dark, system | system | Browser |
| `aib.sidebar-collapsed` | true, false | false | Browser |
| `aib.collection-view-default` | grid, list | grid | Browser |
| `aib.collection-view.agents`, `aib.collection-view.connections` | grid, list | Unset: explicit Appearance default, otherwise list above twelve items or grid | Browser per page; cleared on explicit global choice |

Preferences never contain authorization context, drafts, selected services, credentials or API responses. They never affect a consent or tool decision. Search, sort and facet filters are URL query presentation state, not authorization input.

## Contract boundary

This feature adds no end-user API contract, response field, persistence, or migration. It uses current main's accepted ephemeral callback-state transport and existing end-user responses from `api/enduser/openapi.yaml`. With the route rename, the broker callback's error and missing-return redirects target `/connections` instead of `/sessions`.
