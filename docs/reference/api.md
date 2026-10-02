---
title: "API overview"
description: Shared conventions for the Agentic Identity Broker two-port API. This page covers reverse-proxy authentication, response envelopes, error codes, and endpoint groups.
---

# API overview

This page describes the conventions shared by the generated OpenAPI contracts. It explains
authentication, response structure, and error codes. It also maps the API endpoint groups.

The source specifications generate the field-level contracts:

- **[End-user API reference](/api/enduser)** — Port 8000.
- **[Admin API reference](/api/admin)** — Port 14000.

These pages define request and response schemas. This overview describes shared behavior and
points to the relevant endpoint group.

## Authentication model

The broker uses a **trusted reverse proxy** for pre-authentication. The proxy can be
oauth2-proxy, nginx `auth_request`, or a service mesh. It authenticates the caller and sends
the principal in a request header. The broker does not authenticate end users.

- **Principal header** — The default is **`X-Remote-User`**. It contains a principal email,
  username, or opaque ID. You can configure the header name. The broker accepts it only from
  a trusted source.
- **Session cookie** — After pre-authentication, the end-user server maintains a
  `session_token` cookie. A request can use the session cookie or principal header.
- **Admin privilege** — The proxy enforces administrator privilege before a request reaches
  the admin API.
- **CORS** — The end-user server enables CORS for `/api/*` routes for the browser consent
  interface.

### Public endpoints

These endpoints require no pre-authentication:

| Endpoint | Server |
|---|---|
| `GET /health` | Both |
| `GET /oauth2/jwks.json` | End-user |
| `GET /.well-known/oauth-authorization-server` | End-user |

`GET /oauth2/authorize` and `POST /oauth2/token` do not use pre-authentication. They use
OAuth2 parameters, such as agent client credentials or `client_assertion`. They do not use
the principal header. See
[Configure authentication](/docs/guides/configure-authentication) for the proxy trust
boundary.

## Response envelopes

Successful and error responses follow a small set of consistent shapes.

| Shape | Used by | Example |
|---|---|---|
| `{"data": <resource-or-array>}` | Most resource responses | `{"data": {"principal": "…"}}` |
| Bare JSON array | Admin list endpoints `GET /api/agents`, `GET /api/services` | `[{"id": "…"}]` |
| `{"items": [ … ]}` | `GET /api/oauth2-server/signing-keys` | `{"items": [{"kid": "…"}]}` |
| `{"error": "<code>", "message": "<text>"}` | Standard errors (end-user and admin) | `{"error": "agent not found", "message": "…"}` |
| `{"error": "<code>", "error_description": "…"}` | OAuth2 endpoints (RFC 6749 / 8693) | `{"error": "access_denied", "error_description": "…"}` |

The error envelope depends on the API surface. End-user and admin APIs use `{error, message}`.
Both fields are required. OAuth2 endpoints use the RFC `{error, error_description}` envelope.

## Error codes

The `error` field contains a machine-readable code. End-user and admin APIs use
human-readable strings. OAuth2 endpoints use RFC snake_case tokens.

| Surface | Codes |
|---|---|
| End-user consent / session | `session_expired`, `invalid_permission_set`, `forbidden`, `invalid_state`, `service_id_mismatch`, `unauthorized`, `bad request`, `invalid request`, `invalid scopes`, `service not found` |
| Admin | `invalid request body`, `validation failed`, `agent not found`, `service not found`, `conflict`, `last_key`, `current_key` |
| OAuth2 (`/oauth2/token`) | `invalid_request`, `invalid_client`, `invalid_grant`, `invalid_target`, `access_denied`, `server_error` |
| OAuth2 authorize | `invalid_client`, `invalid_redirect_uri` |

## End-user API map (port 8000)

The full request and response schemas for every endpoint below are in the
[end-user API reference](/api/enduser).

### Health

| Method | Path | Purpose |
|---|---|---|
| GET | `/health` | Server health and lifecycle status (public). |

### User info

| Method | Path | Purpose |
|---|---|---|
| GET | `/api/me` | The current authenticated user's profile. |

The response is `{"data": {"principal": "...", "displayName": "..."}}`, with optional `email` and `pictureUrl` fields.
The handler omits unavailable optional fields. Without an enriched profile, `displayName` equals `principal`.

### Consent

| Method | Path | Purpose |
|---|---|---|
| GET | `/api/consent/agents` | List agents that have active delegations for the user. |
| GET | `/api/consent/agents/{agent-id}` | Agent detail with its requested third-party services. |
| GET | `/api/consent/agents/{agent-id}/grants` | One stored grant for the user and agent, or `data: null`. |
| POST | `/api/consent/agents/{agent-id}/grants` | Create or update a grant; optionally resume an OAuth2 flow. |
| DELETE | `/api/consent/agents/{agent-id}/grants` | Revoke all of the agent's permissions. |

Consent responses use these existing wire formats:

| Operation | Success response |
|---|---|
| List agents | `200` with `data` as an array. `activeGrantCount` counts active `UserGrant` records, not services or permission sets. |
| Agent detail | `200` with `data.agent`, `data.services`, `data.permission_sets`, `data.active_session_service_ids`, and `data.service_requirements`. |
| Get grant | `200` with `data` as one grant object or `null`, never an array. This lookup can return an expired grant. |
| Create or update grant | `201` with the grant in `data`. A valid `session_token` adds a sibling `redirect_url` for authorization resumption. |
| Delete grant | `204` with no body. Returns `404` when no grant exists. Connected third-party sessions remain intact. |

Agent detail metadata uses `agentId` with snake_case fields, including `display_name`, `created_at`, and `updated_at`.
The service entries use camelCase fields, including `serviceId`, `requiredScopes`, and `connectionStatus`.
Agent detail does not return `available_services` or agent and service logo fields.
The optional `cimd_metadata` object comes from a validated authorization session token and is omitted when unavailable.

Grant responses omit `valid_until` for indefinite grants. The agent list omits `expiresAt` when all grants are indefinite.
The agent list excludes expired grants, unlike the individual grant lookup.
An empty `granted_permission_sets` object or array returns `400` on POST. Revocation requires DELETE, not an empty POST.

### Third-party sessions

| Method | Path | Purpose |
|---|---|---|
| GET | `/api/third-party/sessions` | List the current user's stored third-party sessions. |
| GET | `/api/third-party/{serviceId}/oauth2/authorize` | Start an authorization-code + PKCE flow to the third party. |
| GET | `/api/third-party/{serviceId}/oauth2/callback` | Handle the third-party OAuth2 callback. |
| GET | `/api/third-party/{serviceId}/session` | Session detail and the agents that depend on it. |
| DELETE | `/api/third-party/{serviceId}/session` | Terminate the session and delete its stored tokens. |
| POST | `/api/third-party/{serviceId}/session/refresh` | Refresh the access token through the existing provider flow. |
| GET | `/api/third-party/{serviceId}/session/affected-agents` | Agents that lose access when the session ends. |

The session list returns `{"data": {"sessions": [<UserSessionSummary>]}}`, with an empty `sessions` array when no stored sessions exist.
It includes expired stored sessions but excludes services without a stored session. Refresh returns the updated summary in `data`.

### OAuth2 server

| Method | Path | Purpose |
|---|---|---|
| GET | `/oauth2/authorize` | RFC 6749 authorization endpoint. |
| POST | `/oauth2/token` | Token exchange, authorization-code, or client-credentials grant. |
| GET | `/oauth2/jwks.json` | Aggregated public JWK set (public). |
| GET | `/.well-known/oauth-authorization-server` | RFC 8414 authorization-server metadata (public). |

For the `/oauth2/token` grant modes and the RFC 8693 field reference, see
[Token exchange](/docs/reference/token-exchange).

## Admin API map (port 14000)

Each admin endpoint requires `X-Remote-User`. The proxy enforces administrator privilege.
`GET /health` is public. See [admin API reference](/api/admin) for full schemas.

### Agents

| Method | Path | Purpose |
|---|---|---|
| GET | `/api/agents` | List all agents (bare array). |
| POST | `/api/agents` | Register an agent. |
| GET | `/api/agents/{agent-id}` | Get an agent. |
| PUT | `/api/agents/{agent-id}` | Update an agent. |
| DELETE | `/api/agents/{agent-id}` | Delete an agent (irreversible). |

### Services

| Method | Path | Purpose |
|---|---|---|
| GET | `/api/services` | List services (bare array; secrets redacted). |
| POST | `/api/services` | Register a third-party OAuth2 service. |
| GET | `/api/services/{service-id}` | Get a service (secret redacted). |
| PUT | `/api/services/{service-id}` | Update a service. |
| DELETE | `/api/services/{service-id}` | Delete a service (`409 conflict` if grants reference it). |

### Permission sets

| Method | Path | Purpose |
|---|---|---|
| POST | `/api/permission-sets` | Create a permission set. |
| GET | `/api/permission-sets` | List permission sets (optionally filtered by service). |
| GET | `/api/permission-sets/{permission-set-id}` | Get a permission set. |
| PUT | `/api/permission-sets/{permission-set-id}` | Replace a permission set. |
| DELETE | `/api/permission-sets/{permission-set-id}` | Delete a permission set. |

### Client credentials

| Method | Path | Purpose |
|---|---|---|
| POST | `/api/agents/{agent-id}/client-credentials` | Generate or rotate an agent's broker credentials. |
| GET | `/api/agents/{agent-id}/client-credentials` | Credential metadata (never the secret). |
| DELETE | `/api/agents/{agent-id}/client-credentials` | Revoke the agent's credentials (irreversible). |

### Signing keys

| Method | Path | Purpose |
|---|---|---|
| POST | `/api/oauth2-server/signing-keys` | Add a signing key. |
| GET | `/api/oauth2-server/signing-keys` | List signing keys (`{items}`, newest first). |
| PUT | `/api/oauth2-server/signing-keys/{kid}/current` | Promote a key to current. |
| DELETE | `/api/oauth2-server/signing-keys/{kid}` | Soft-delete a key (`409 last_key` / `current_key`). |

## Related

- [End-user API reference](/api/enduser) and [Admin API reference](/api/admin) — the full
  generated contracts.
- [Token exchange](/docs/reference/token-exchange) — the RFC 8693 field reference.
- [Configuration](/docs/configuration) — every configuration key.
- [Configure authentication](/docs/guides/configure-authentication) — the proxy trust
  boundary.
