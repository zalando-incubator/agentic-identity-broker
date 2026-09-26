# Data Model — Consent UI v2

**Status**: Proposed design. Existing aggregates retain their ownership and authorization semantics.

## Existing entities and projections

| Entity or value | Relevant fields | Invariant |
| --- | --- | --- |
| Agent | Typed AgentID, display name, CIMD metadata, service requirements with `connectionStatus`, permission sets, governance and documentation links | The Agent Origin Label derives only from the authorization session and never claims legal publisher verification. No existing response carries a publisher |
| UserGrant | Principal, AgentID, `granted_permission_sets`, selected services, validity | One grant per principal/agent pair. Consent preserves existing selected access |
| Delegation (list projection of an unexpired UserGrant) | `agentId`, `displayName`, `logoUrl`, `activeGrantCount`, `lastModifiedAt`, `expiresAt` | The existing list excludes expired grants and names no services. The UI shows `activeGrantCount` as the number of granted permission sets and removes a row once its `expiresAt` passes |
| Permission Set | Identity, name, description, required/optional assignment, services | Required groups remain locked. The UI presents the name and description, never raw scope strings |
| UserSession | Principal, ServiceID, encrypted tokens, granted scope, expiry, refresh capacity | One principal/service session. Connection does not imply delegation. The session list returns stored sessions only and carries no provider account identifier |
| ToolApproval | ID, owner, agent, tool, arguments, status, persistence, scope, expiry | Only the owner resolves a pending request. Resolved/expired requests cannot be resubmitted |
| Authorization context | Existing `session_token`, selected access, callback state | Backend validation remains authoritative. UI decoding never authorizes a request |

No existing entity receives new token semantics. No credential enters browser preference storage.

## ConsentDraft: transient UI state

Fields: requested permission-set/service selections, existing granted selections, selected duration, custom expiry, dirty state, and the existing authorization-session reference.

The decision flow preserves `consent_state` across the existing provider authorization callback. It does not introduce a new persisted consent model.

Transitions:

- Loaded request → unchanged or edited draft.
- Edited draft → validated submission → server outcome and existing safe continuation.
- Edited draft → Cancel → original selections in console context.
- Decision draft → Deny → local terminal outcome, without a mutation or constructed redirect.
- Invalid or expired authorization context → terminal error, never management-mode fallback.

Re-consent computes a selection delta from the existing grants read response. Previously granted permission sets and services remain selected and read-only in the decision view when the user allows new access; only the console detail view changes or removes them. The UI never silently widens the grant.

A grant has one validity. On re-consent the duration choice starts from the existing grant: Until revoked for a null `valid_until`, otherwise Custom date. Unless the user changes the duration, submission sends the existing `valid_until` unchanged. A changed duration applies to the whole grant.

## ConnectionState: derived presentation

The UI derives this value object from existing session fields, refresh responses in the current page, and the agent requirement `connectionStatus`. It adds no response field, stored column, or backend state.

| Condition, in precedence order | Visible state | Available action |
| --- | --- | --- |
| No session for a service the agent requires (`connectionStatus: not_connected`) | No connection, on the agent's Connections tab and in the consent service prompt only | Connect through the existing authorization flow |
| Known rejected refresh result or other authoritative unusable-token result | Needs re-authentication | Reconnect |
| Refresh token expired, or access expired without usable refresh capacity | Expired | Reconnect |
| Access expired with apparently usable refresh token, but refresh not yet successful | Needs re-authentication, with refresh explanation | Refresh when supported, otherwise reconnect |
| Usable access | Connected | Existing refresh and confirmed disconnect |
| Session read fails | Error or explicitly stale prior state, never Connected from missing data | Retry the read |

`/sessions` lists stored sessions only, so it never shows No connection.

A known rejected refresh result is a `409` (no valid refresh token) or `502` (provider rejected the refresh) response to the existing refresh call in the current page. It lasts until a successful refresh or reconnect. After a reload, the session fields decide the state again, so an expired access token with a refresh token shows the fourth row.

The fourth row avoids calling a session Connected before a failed refresh reveals itself. A successful refresh recomputes the state. Unknown token lifetime follows existing domain usability semantics, not a fabricated expiry.

A refresh failure that is only a network error, or any other non-authoritative failure, does not prove credential rejection. Show the operation error and preserve the last authoritative state as stale. A `404` means the session no longer exists; refetch the list.

Existing session data does not identify required scope coverage. The UI shows no Missing scopes state and never infers a gap from available provider scopes. It shows no last-use time.

## Browser preferences

| Key | Values | Default | Scope |
| --- | --- | --- | --- |
| `aib.theme` | light, dark, system | system | Browser |
| `aib.sidebar-collapsed` | true, false | false | Browser |

Preferences never contain authorization context or credentials. They never affect a consent or tool decision.

## Contract boundary

This feature adds or changes no API contract, response field, persistence, or migration. Every screen uses existing end-user responses from `api/enduser/openapi.yaml`.
