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
| `GET /.well-known/oauth-client/{service-id}` | End-user |
| `GET /.well-known/oauth-client/{service-id}/jwks.json` | End-user |
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
| Bare JSON object | Admin service create, get, update, and discovery-status GET | See the [service contracts](/api/admin#tag/Services). |
| `{"items": [ … ]}` | `GET /api/oauth2-server/signing-keys` | `{"items": [{"kid": "…"}]}` |
| `{"error": "<code>"}` with optional `message` | Standard errors (end-user and admin) | `{"error": "internal server error"}` |
| `{"error": "<code>", "error_description": "…"}` | OAuth2 endpoints (RFC 6749 / 8693) | `{"error": "access_denied", "error_description": "…"}` |

The error envelope depends on the API surface. The End-user and Admin `ErrorResponse` require `error`.
Their `message` is optional. OAuth2 endpoints use the RFC `{error, error_description}` envelope.

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

### Consent

| Method | Path | Purpose |
|---|---|---|
| GET | `/api/consent/agents` | List agents that have active delegations for the user. |
| GET | `/api/consent/agents/{agent-id}` | Agent detail with its requested third-party services. |
| GET | `/api/consent/agents/{agent-id}/grants` | The user's grants for an agent. |
| POST | `/api/consent/agents/{agent-id}/grants` | Create or update a grant; optionally resume an OAuth2 flow. |
| DELETE | `/api/consent/agents/{agent-id}/grants` | Revoke all of the agent's permissions. |

### Third-party sessions

| Method | Path | Purpose |
|---|---|---|
| GET | `/api/third-party/sessions` | List third-party services and per-user session status. |
| GET | `/api/third-party/{serviceId}/oauth2/authorize` | Start an authorization-code + PKCE flow to the third party. |
| POST | `/api/third-party/{serviceId}/oauth2/authorize` | Start a PKCE flow with a tab-local `consent_state_id` and clean same-origin return path; the provider-facing state stays under 6,000 bytes. |
| GET | `/api/third-party/{serviceId}/oauth2/callback` | Complete the third-party flow and, on success, return the sealed selection ID for same-tab restoration. |
| GET | `/api/third-party/{serviceId}/session` | Session detail and the agents that depend on it. |
| DELETE | `/api/third-party/{serviceId}/session` | Terminate the session and delete its stored tokens. |
| GET | `/api/third-party/{serviceId}/session/affected-agents` | Agents that lose access when the session ends. |

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
| GET | `/api/services/{service-id}/discovery-status` | Read stored discovery status without contacting the provider. |
| GET | `/api/services/{service-id}/protected-resources` | List the service's protected-resource URIs. |
| POST | `/api/services/{service-id}/protected-resources` | Add one protected-resource URI. |
| PUT | `/api/services/{service-id}/protected-resources/{resource}` | Add one URI by its encoded path segment. |
| PATCH | `/api/services/{service-id}/protected-resources/{resource}` | Rename a protected-resource URI. |
| DELETE | `/api/services/{service-id}/protected-resources/{resource}` | Remove a protected-resource URI. |

### Protected-resource discovery

Use the [service creation contract](/api/admin#tag/Services/operation/createService)
for protected-resource discovery requests, issuer selection, response examples,
and failure codes. The
[service update contract](/api/admin#tag/Services/operation/updateService)
defines refresh behavior and issuer or audience changes.

The [discovery-status contract](/api/admin#tag/Services/operation/getServiceDiscoveryStatus)
defines stored status and its response examples. For the operator workflow, follow
[Discover a protected resource with hosted CIMD or DCR](/docs/guides/manage-agents-and-services#discover-a-protected-resource-with-hosted-cimd-or-dcr).

### Outbound CIMD client registration

The [service creation contract](/api/admin#tag/Services/operation/createService)
defines registration and token-endpoint authentication choices. The
[service update contract](/api/admin#tag/Services/operation/updateService) defines
replacement and transition rules. Follow the
[outbound CIMD setup guide](/docs/guides/manage-agents-and-services#register-a-broker-hosted-outbound-cimd-confidential-service)
for key preparation and provider checks.

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

### CIMD client-authentication keys

These routes manage the ES256 keys that sign outbound `private_key_jwt` assertions. They are available in proxy, local, and hybrid modes.

| Method | Path | Purpose |
|---|---|---|
| POST | `/api/cimd-client-keys` | Generate an ES256 CIMD key. |
| GET | `/api/cimd-client-keys` | List active CIMD keys. |
| PUT | `/api/cimd-client-keys/{kid}/current` | Promote a CIMD key immediately. |
| DELETE | `/api/cimd-client-keys/{kid}` | Remove a non-signing CIMD key. |

The first generated key is immediately usable. Later generated keys remain public during the activation grace period.

The routes never return private key material, ciphertext, or client assertions. `/oauth2/jwks.json` does not contain CIMD key IDs.

Token-signing key routes remain unavailable in proxy mode.

## Related

- [End-user API reference](/api/enduser) and [Admin API reference](/api/admin) — the full
  generated contracts.
- [Token exchange](/docs/reference/token-exchange) — the RFC 8693 field reference.
- [Configuration](/docs/configuration) — every configuration key.
- [Configure authentication](/docs/guides/configure-authentication) — the proxy trust
  boundary.
