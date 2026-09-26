# Data Model — Consent UI v2

**Status**: Proposed design. Existing aggregates retain their ownership and authorization semantics.

## Existing entities and projections

| Entity or value | Relevant fields | Invariant |
| --- | --- | --- |
| Agent | Typed AgentID, display name, CIMD metadata, service requirements, permission sets | The origin badge derives only from the authorization session and never claims legal publisher verification |
| UserGrant | Principal, AgentID, `granted_permission_sets`, selected services, validity | One grant per principal/agent pair. Consent preserves existing selected access |
| Permission Set | Identity, name, description, required/optional assignment, services | Required groups remain locked. The UI presents the name and description, never raw scope strings |
| UserSession | Principal, ServiceID, encrypted tokens, granted scope, expiry, refresh capacity | One principal/service session. Connection does not imply delegation |
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

Re-consent computes a selection delta from the existing grants read response. Previously granted permission sets and services remain selected when the user allows new access. The UI never silently widens the grant.

## ConnectionState: derived presentation

The UI derives this value object from existing session fields. It adds no response field, stored column, or backend state.

| Condition, in precedence order | Visible state | Available action |
| --- | --- | --- |
| No session | No connection | Connect through the existing authorization flow |
| Known rejected refresh result or other authoritative unusable-token result | Needs re-authentication | Reconnect |
| Refresh token expired, or access expired without usable refresh capacity | Expired | Reconnect |
| Access expired with apparently usable refresh token, but refresh not yet successful | Needs re-authentication, with refresh explanation | Refresh when supported, otherwise reconnect |
| Usable access | Connected | Existing refresh and confirmed disconnect |
| Session read fails | Error or explicitly stale prior state, never Connected from missing data | Retry the read |

The fourth row avoids calling a session Connected before a failed refresh reveals itself. A successful refresh recomputes the state. Unknown token lifetime follows existing domain usability semantics, not a fabricated expiry.

A refresh failure that is only a network error does not prove credential rejection. Show the operation error and preserve the last authoritative state as stale.

Existing session data does not identify required scope coverage. The UI shows no Missing scopes state and never infers a gap from available provider scopes. It shows no last-use time.

## Browser preferences

| Key | Values | Default | Scope |
| --- | --- | --- | --- |
| `aib.theme` | light, dark, system | system | Browser |
| `aib.sidebar-collapsed` | true, false | false | Browser |

Preferences never contain authorization context or credentials. They never affect a consent or tool decision.

## Contract boundary

This feature adds or changes no API contract, response field, persistence, or migration. Every screen uses existing end-user responses from `api/enduser/openapi.yaml`.
