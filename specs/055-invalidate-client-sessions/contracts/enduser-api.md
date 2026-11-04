# Contract Delta: End-User API (`api/enduser/openapi.yaml`)

**Status**: Proposed. Approval item API-001 in [plan.md](../plan.md). Nothing here is applied to `api/enduser/openapi.yaml` until the maintainer approves it.

**Version**: `info.version` `1.1.0` → `1.2.0`. The schema change is additive, and the behavior changes keep every route, status code, and error code. No paths, parameters, or request bodies change.

## 1. Schemas

### `UserSessionSummary` (returned by `GET /api/third-party/sessions` and `POST /api/third-party/{serviceId}/session/refresh`)

Add `requires_reauthentication` to `required` and to `properties`:

```yaml
requires_reauthentication:
  type: boolean
  description: |
    True when this session cannot supply or refresh tokens until the user signs in to the
    service again. This is the case when the session has expired without usable refresh
    credentials (`is_expired`), or when the service's client ID changed after the session was
    established. Restoring a previous client ID does not clear it. Only a successful sign-in
    clears it. `is_expired` keeps its meaning and is not affected by client-ID changes.
```

### `SessionStatus` (returned by `GET /api/third-party/{serviceId}/session`)

Add the same required `requires_reauthentication` property with the same description.

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
  "is_expired": false,
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
| `POST /api/third-party/{serviceId}/session/refresh` | A session that requires reauthentication because of a client-ID change (mismatched, superseded, or missing identity) is not refreshed, and stored tokens are unchanged. The response becomes the existing `409 refresh_unavailable`. Today an identity mismatch here falls through to `500 internal_error`. An unreadable client-ID file keeps the `500` outcome. |
| `POST /api/consent/agents/{agent-id}/grants` | A session that requires reauthentication does not satisfy a mandatory service. The existing validation error applies. |
| `GET /api/consent/agents/{agent-id}` | `connectionStatus` follows §1. |

## 3. Compatibility

- Consumers that ignore unknown response properties are unaffected. The SPA in this repository is updated in the same change.
- `is_expired` keeps its meaning, so existing consumers keep their behavior. They might show an invalidated session as active until they read `requires_reauthentication`. Token release is enforced server-side regardless.
- `docs/api/` and `docs/reference/token-exchange.md` are updated to match (Principle IV).
