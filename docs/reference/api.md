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
| Bare JSON object | Admin service create, get, update, and discovery-status GET | See the discovery-status response example. |
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

The authenticated Admin `POST /api/services` creates a service from protected-resource metadata.
Set `discovery.enable_discovery` to `true` and supply a public HTTPS `discovery.resource_url`.
Omit manual endpoints, client credentials, `discovery.metadata_url`, and `token_endpoint_auth_method`.
The broker validates the resource and issuer before it selects hosted CIMD or DCR.
It does not create a service when discovery or registration fails.

This create body does not contain credentials:

```json
{
  "display_name": "Example MCP",
  "discovery": {
    "enable_discovery": true,
    "resource_url": "https://mcp.example.com/mcp"
  }
}
```

If the resource advertises more than one issuer, the broker returns `400` with `issuer_selection_required`.
Only this error includes `authorization_servers`, a list of validated issuers from matching resource metadata:

```json
{
  "error": "discovery failed",
  "message": "issuer_selection_required",
  "authorization_servers": [
    "https://login-a.example.com",
    "https://login-b.example.com"
  ]
}
```

Retry the create request with an `issuer_uri` that matches one listed URI exactly:

```json
{
  "display_name": "Example MCP",
  "issuer_uri": "https://login-a.example.com",
  "discovery": {
    "enable_discovery": true,
    "resource_url": "https://mcp.example.com/mcp"
  }
}
```

With one advertised issuer, `issuer_uri` is optional. A successful create returns `201` and a service object.
The broker derives one effective `authorization_params.resource` from verified metadata unless an administrator supplies an explicit value.
The explicit value must be a non-empty absolute URI without a fragment.
This effective resource is distinct from `discovery.resource_url` and the `protected_resources` ownership set.
The authenticated Admin `GET /api/services/{service-id}` returns the active service and its effective resource without a DCR secret.
The same service representation appears in create, read, list, and update responses.
Its `discovery.authorization_param_resource_strategy` is required, read-only, and nullable:

| Value | Source of `authorization_params.resource` |
|---|---|
| `derived` | The verified `discovery.resource_url`. |
| `pinned` | An administrator-supplied `authorization_params.resource`, even when it equals the verified URL. |
| `null` | A manual or direct-`metadata_url` service. |

The response `discovery.metadata_url`, `discovery.resource_url`, and `discovery.client_method` can be null.
The strategy belongs to `discovery`, not to the forwarded `authorization_params` map.
Do not send the strategy in a create or update request.

These response fragments show a derived audience A, a pinned audience A, and a manual service:

```json
{"discovery":{"resource_url":"https://files.example/mcp","authorization_param_resource_strategy":"derived"},"authorization_params":{"resource":"https://files.example/mcp"}}
```

```json
{"discovery":{"resource_url":"https://files.example/mcp","authorization_param_resource_strategy":"pinned"},"authorization_params":{"resource":"https://files.example/mcp"}}
```

```json
{"discovery":{"enable_discovery":false,"authorization_param_resource_strategy":null},"authorization_params":{}}
```

For example, a DCR create response can contain this service without a client secret:

```json
{
  "id": "880e8400-e29b-41d4-a716-446655440003",
  "display_name": "Example MCP",
  "client_id": "dcr-client-123",
  "token_endpoint_auth_method": "client_secret_basic",
  "oauth2_flavor": "standard",
  "issuer_uri": "https://login-a.example.com",
  "discovery": {
    "enable_discovery": true,
    "resource_url": "https://mcp.example.com/mcp",
    "client_method": "dcr",
    "authorization_param_resource_strategy": "derived"
  },
  "endpoints": {
    "token_endpoint": "https://login-a.example.com/token",
    "authorize_endpoint": "https://login-a.example.com/authorize"
  },
  "authorization_params": {
    "resource": "https://mcp.example.com/mcp"
  },
  "created_at": "2026-10-06T10:00:00Z",
  "updated_at": "2026-10-06T10:00:00Z"
}
```

A confidential DCR response can show `client_secret_basic` or `client_secret_post` as `token_endpoint_auth_method`.
These DCR methods are response-only. Create and update requests cannot supply them.
DCR responses omit `client_secret` even when the broker stores an encrypted secret.

For DCR, configure the deployment-wide `third_party_oauth2.client_name` before registration.
The broker uses that name for the registered client, not the service `display_name`.
If the name is blank, DCR returns `400` with `client_name_unconfigured`.
Manual and CIMD services do not require this name.
Hosted CIMD requires an HTTPS end-user public URL and a generated ES256 CIMD client key.
DCR also requires a public HTTPS end-user URL for its registered callback.

The authenticated Admin `PUT /api/services/{service-id}` refreshes protected-resource discovery.
The request requires `display_name` and `discovery` and omits manual endpoints and credentials.
To pin the audience, supply `authorization_params.resource` in the request, even when it equals the current audience:

```json
{
  "display_name": "Example MCP",
  "discovery": {
    "enable_discovery": true,
    "resource_url": "https://mcp.example.com/mcp"
  },
  "authorization_params": {
    "resource": "https://mcp.example.com/mcp"
  }
}
```

Omit unchanged `authorization_params` on PUT to retain the parameter map and its derived or pinned source.
Supply `resource` in that map to pin the audience. Supply a map without `resource`, such as `{}`, to restore derivation from the verified URL.
An unchanged explicit audience permits a verified change to `discovery.resource_url`, even while user sessions exist.
If the effective audience changes, terminate all user sessions first, including expired sessions.
Otherwise, the broker returns `409` with `{"error":"conflict","message":"resource_change_requires_no_sessions"}` before provider discovery and checks again at commit.
An omitted `issuer_uri` retains the active issuer only when fresh metadata still advertises it.
A different issuer requires an explicit selection and no user sessions. The issuer conflict takes precedence when both the issuer and audience change.
For an unchanged issuer, the broker keeps the client ID, client method, credential, and exact token authentication method.
A callback from the prior issuer or audience cannot create a session after a session-free change.
On a full manual replacement, omit `authorization_params` to retain the map or supply `{}` to clear it. The response strategy becomes `null` after conversion.
If that conversion changes the audience while sessions exist, the broker returns the same `409` without a discovery-status write.
A rejected discovery-backed audience change or failed discovery attempt keeps active configuration, owned resources, sessions, and the service ETag unchanged.
It writes only the stored attempt time and safe failure reason. For an audience conflict, that reason is `resource_change_requires_no_sessions`.
An invalid request that stops before discovery does not write status.
The service ETag covers active configuration and owned protected resources, not this failure-only status.

The authenticated Admin `GET /api/services/{service-id}/discovery-status` reads stored state without provider traffic.
Its `status` is `ready`, `failed`, or `not_applicable`.
The response always includes `resource_url`, `issuer_uri`, `client_method`, `last_attempt_at`, `last_success_at`, and `failure_reason`.
Each of those fields can be `null`. A manual service returns `not_applicable` and null discovery fields.
After a failed refresh, `resource_url`, `issuer_uri`, and `client_method` describe the last successful active configuration.
The discovery-status GET has no ETag. A failure-only status update does not change the service version.
For example, a failed refresh can return:

```json
{
  "status": "failed",
  "resource_url": "https://mcp.example.com/mcp",
  "issuer_uri": "https://login-a.example.com",
  "client_method": "dcr",
  "last_attempt_at": "2026-10-06T11:00:00Z",
  "last_success_at": "2026-10-06T10:00:00Z",
  "failure_reason": "authorization_server_metadata_invalid"
}
```

For a manual service, the stored status has null discovery fields:

```json
{
  "status": "not_applicable",
  "resource_url": null,
  "issuer_uri": null,
  "client_method": null,
  "last_attempt_at": null,
  "last_success_at": null,
  "failure_reason": null
}
```

Discovery and registration errors use safe `message` codes. They never include provider bodies, URL queries, credentials, assertions, or tokens.

| HTTP status | Admin operation | Meaning |
|---|---|---|
| `400` | Create or update | Invalid request, discovery failure, or client registration failure. |
| `400` | Discovery-status GET | Malformed service ID. |
| `401` | Discovery-status GET | An operator principal is required. |
| `404` | Update or discovery-status GET | The service does not exist. Missing provider metadata instead returns `400`. |
| `409` | Create or update | Duplicate client identity or protected-resource ownership. Manual client-ID conflicts also return `409`. |
| `409` | Update | An issuer or effective audience change with user sessions, including expired sessions. |
| `504` | Create or update | Discovery or registration exceeded the 15-second attempt deadline. The `message` is `timeout`. |

Discovery failures use `error: "discovery failed"` and a safe `message` code.
Resource codes are `resource_metadata_not_found`, `resource_metadata_unavailable`, `resource_metadata_invalid`, and `resource_mismatch`.
Issuer codes are `authorization_server_missing`, `issuer_selection_required`, `issuer_not_advertised`, and `issuer_mismatch`.
Authorization-server codes are `authorization_server_metadata_not_found`, `authorization_server_metadata_unavailable`, and `authorization_server_metadata_invalid`.
Other discovery codes are `unsafe_destination` and `response_too_large`.
Registration failures use `error: "client registration failed"`.
Their codes are `no_compatible_client_method`, `cimd_unavailable`, `client_name_unconfigured`, `client_registration_rejected`, `client_registration_invalid`, and `client_method_changed`.
Conflict reasons include `duplicate_client_identity`, `issuer_change_requires_no_sessions`, and `resource_change_requires_no_sessions`.
Local storage or encryption errors return `500` without a discovery failure code.

### Outbound CIMD confidential services

The broker uses **outbound CIMD client authentication** for a `private_key_jwt` third-party service. This is separate from inbound CIMD client resolution, where an agent presents a metadata URL to the broker authorization server.

A manual service can use one of three token-endpoint authentication methods:

| Method | Client ID | `client_secret` in read responses |
|---|---|---|
| `null` | Operator-provided | `REDACTED` |
| `none` | Operator-provided | Omitted |
| `private_key_jwt` | Broker-generated HTTPS metadata URL | Omitted |

For `private_key_jwt`, omit `client_id` and `client_secret` in the create or replacement request.

The broker generates the client ID after it allocates the service ID. The broker signs each token request with a fresh ES256 client assertion.

The end-user server serves the public Client ID Metadata Document at the generated client ID. It serves the matching CIMD public JWK Set at `<client-id>/jwks.json`.

Both routes return `Cache-Control: public, max-age=300`. They return JSON `404` responses for unavailable services.

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
