# Contract Delta: End-User API (`api/enduser/openapi.yaml`)

**Status**: Proposed. Approval item API-001 in [plan.md](../plan.md). Nothing here is applied to `api/enduser/openapi.yaml` until the maintainer approves it.

**Version (Proposed API-001)**: `info.version` `1.2.0` → `2.0.0` on the existing routes. ADR 046 already defines the reauthentication fields. This proposal changes their identity-related behavior and the manual-refresh outcome; it does not add duplicate fields. No paths, parameters, or request bodies change. A major cutover makes changed released behavior explicit under Constitution Principles IV and X. The main alternative is new versioned endpoints; an additive minor bump does not describe this behavioral change.

## 1. Schemas

### `UserSessionSummary` (returned by `GET /api/third-party/sessions` and `POST /api/third-party/{serviceId}/session/refresh`)

Keep the existing required `requires_reauthentication` property and extend its description:

```yaml
requires_reauthentication:
  type: boolean
  description: |
    True when this session cannot supply or refresh tokens until the user signs in to the
    service again. This includes a recorded terminal provider refresh rejection, an expired
    access token without a stored refresh token, or an unusable client identity or revision.
    Rejection blocks issuance even with an unexpired access token. An expired token with a
    stored, not-rejected refresh token and usable identity remains refreshable. Restoring a
    previous client ID does not clear identity invalidation; fresh authorization under the
    current identity is required. `is_expired` remains an equal deprecated alias;
    `access_token_expired` remains the independent access-expiry signal.
```

### `SessionStatus` (returned by `GET /api/third-party/{serviceId}/session`)

Retain the existing `allOf` reference to `UserSessionSummary`, which supplies the same required property and deprecated alias; do not duplicate them. Ownership and lifecycle metadata remain unchanged.

### `ServiceRequirementForUser.connectionStatus`

The enum is unchanged (`connected`, `not_connected`). Replace the description:

```yaml
description: |
  `connected` when the user has a session for this service that can supply tokens now.
  `not_connected` when there is no session, or the session requires reauthentication,
  including after the service's client ID changed. In both cases the user must sign in to
  the service.
```

### Example (sessions list item after a client-ID change)

```json
{
  "id": "660e8400-e29b-41d4-a716-446655440001",
  "service_id": "880e8400-e29b-41d4-a716-446655440010",
  "service_display_name": "Reports Production",
  "token_type": "Bearer",
  "scope": ["reports:read"],
  "initiated_at": "2026-11-04T08:00:00Z",
  "is_expired": true,
  "access_token_expired": false,
  "has_refresh_token": true,
  "requires_reauthentication": true,
  "dependent_agent_count": 1,
  "is_encrypted": true
}
```

## 2. Behavior documented on existing operations

| Operation | Added documentation |
|---|---|
| `GET /oauth2/authorize` | In local and hybrid mode, an existing grant does not bypass consent when a service included in that grant has a session that requires reauthentication, including after a client-ID change. The response is the existing `302` to `/agents/{agent-id}?session_token=...`. |
| `POST /oauth2/token` (RFC 8693 exchange) | A session whose service client ID changed after it was established returns `400 invalid_grant` with ADR 044's service-specific `error_uri`, even when its access token is unexpired or has no recorded expiry. No token, `WWW-Authenticate` header, or new error code is introduced. If the service's current client ID cannot be determined, for example because the client-ID file is unreadable, the response is `500 server_error` without `error_uri`. |
| `GET /api/third-party/{serviceId}/oauth2/callback` | An authorization started before a client-ID change cannot complete after it, including when the previous client ID was restored in the meantime. The broker does not exchange the code. The browser receives the existing `302` to `/sessions?error=callback_failed&error_description=...`. Starting a new authorization succeeds under the current client ID. |
| `GET/POST /api/third-party/{serviceId}/oauth2/authorize` | No change. New authorizations always use the current client ID. |
| `POST /api/third-party/{serviceId}/session/refresh` | A session that requires reauthentication because of a client-ID change (mismatched, superseded, or missing identity) is not refreshed, and stored tokens are unchanged. The response becomes the existing `409 refresh_unavailable`. Today an identity mismatch here falls through to `500 internal_error`. An unreadable client-ID file keeps the `500` outcome. ADR 046's `409 refresh_rejected` for recorded or newly observed terminal rejection remains unchanged; identity handling must not replace it. |
| `POST /api/consent/agents/{agent-id}/grants` | A session that requires reauthentication does not satisfy a mandatory service. The existing validation error applies. |
| `GET /api/consent/agents/{agent-id}` | `connectionStatus` follows §1. |

## 3. Compatibility

- The response shape is unchanged from 1.2.0. Consumers must account for the new identity-related reauthentication cause and the manual-refresh status change at the proposed major cutover. The SPA is updated in the same change.
- `is_expired` remains an equal deprecated alias of `requires_reauthentication`, so both become true after identity invalidation. `access_token_expired` is unchanged. No refresh expiry or rejection timestamp is exposed.
- Existing provider-rejection behavior, including `409 refresh_rejected`, exclusion from background refresh and sweep, and no issuance after committed rejection, remains binding under ADR 046.
- `docs/api/` and `docs/reference/token-exchange.md` are updated to match (Principle IV).
