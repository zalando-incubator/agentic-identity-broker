---
title: "Manage agents and services"
description: Register third-party OAuth2 services and define permission sets. Register agents and issue broker client credentials through the admin API on port 14000.
---

# Manage agents and services

This guide explains how an administrator creates the objects for delegation with the admin
API on port 14000. First, register a third-party service. Next, group its scopes into
permission sets. Then register an agent that requests those sets. In local and hybrid
server modes, issue broker client credentials to the agent.

Create the objects in this order. A permission set references a service. An agent
references permission sets.

## What you need

- **Admin access through the proxy.** A trusted proxy in front of the admin port
  authenticates the caller, enforces administrator privilege, and sends the principal
  header. The default header is `X-Remote-User`. The broker accepts it only from a trusted
  source. Examples send the header directly to a local development stack. In production,
  the proxy sends the header.
- **The admin base URL.** Examples use `http://localhost:14000`. In production, use the
  internal admin endpoint behind the proxy. Do not use end-user port 8000.

Every request carries the principal header:

```bash
-H "X-Remote-User: admin@example.com"
```

## Register a third-party OAuth2 service

A third-party service is an external OAuth2 provider, such as GitHub, Google, or an internal
API. Create the service with `POST /api/services`.

The core fields:

- `display_name` — The human-readable name in the admin interface.
- `client_id` / `client_secret` — The OAuth2 client credentials from the provider. The broker
  encrypts the secret at rest. Each read returns `"REDACTED"`.
- `issuer_uri` — The provider issuer. It must be an HTTPS URL.
- `discovery.enable_discovery` — When `true`, the broker gets provider endpoints from
  `{issuer_uri}/.well-known/oauth-authorization-server`. When `false`, provide
  `endpoints.token_endpoint` and `endpoints.authorize_endpoint`.
- `scopes` — Provider scopes. Each entry has `{scope_value, description}`. Omit the field or
  use an empty list when the provider has no OAuth2 scopes. The broker then omits `scope`
  from its upstream authorization request.
- `protected_resources` — Optional resource URIs for
  [token exchange](/docs/concepts/delegation-and-consent#using-a-delegation).
- `authorization_params` — Optional static provider parameters for upstream authorization
  requests. For Zalando Platform, use `{ "business_partner_id": "12345" }`. These values
  come from the administrator, not the browser. Omit the field during update to retain it.
  Use `{}` to clear it.

With endpoint discovery enabled:

```bash
curl -X POST http://localhost:14000/api/services \
  -H "X-Remote-User: admin@example.com" \
  -H "Content-Type: application/json" \
  -d '{
    "display_name": "GitHub",
    "client_id": "Iv1.1234567890abcdef",
    "client_secret": "ghp_secretkey1234567890abcdef",
    "issuer_uri": "https://github.com",
    "discovery": { "enable_discovery": true },
    "scopes": [
      { "scope_value": "repo", "description": "Full control of private repositories" },
      { "scope_value": "read:org", "description": "Read organization membership" }
    ],
    "protected_resources": [ "https://api.github.com" ]
  }'
```

For a provider without discovery, set `enable_discovery` to `false` and pass the endpoints:

```bash
curl -X POST http://localhost:14000/api/services \
  -H "X-Remote-User: admin@example.com" \
  -H "Content-Type: application/json" \
  -d '{
    "display_name": "Corporate SSO",
    "client_id": "corp-sso-client-123",
    "client_secret": "secret-value-never-exposed",
    "issuer_uri": "https://sso.corp.example.com",
    "discovery": { "enable_discovery": false },
    "endpoints": {
      "token_endpoint": "https://sso.corp.example.com/oauth/token",
      "authorize_endpoint": "https://sso.corp.example.com/oauth/authorize"
    },
    "scopes": [
      { "scope_value": "profile", "description": "Basic user profile" }
    ]
  }'
```

For an IdP that does not accept scopes, omit `scopes` entirely:

```json
{
  "display_name": "Corporate session IdP",
  "client_id": "corporate-session-client",
  "client_secret": "secret-value-never-exposed",
  "issuer_uri": "https://idp.corp.example.com",
  "discovery": { "enable_discovery": false },
  "endpoints": {
    "token_endpoint": "https://idp.corp.example.com/oauth/token",
    "authorize_endpoint": "https://idp.corp.example.com/oauth/authorize"
  }
}
```

Add this service to a permission set with `"scopes": []`. A mandatory service still requires
a user session. It does not require a scope match.

The response contains a system-generated service `id` (a UUID). It redacts `client_secret`.
Record the `id`. Permission sets reference it.

:::note
The broker prevents deletion of a service that a grant references. `DELETE
/api/services/{service-id}` returns **409 `conflict`**. Revoke or migrate dependent grants
before you delete the service.
:::

## Register a broker-hosted outbound CIMD confidential service

Use `private_key_jwt` when the broker must authenticate to the provider without a shared secret. This is **outbound CIMD client authentication**: the broker acts as the client of the third-party authorization server. It is separate from inbound CIMD client resolution for agents that authenticate to the broker.

Set `server.enduser.public_url` to the stable public HTTPS URL of the broker first.

Do not send `client_id` or `client_secret` in this request. The broker allocates the service ID. It then generates the Client ID Metadata URL.

Before registration, list the broker-global CIMD client-authentication keys:

```bash
curl http://localhost:14000/api/cimd-client-keys \
  -H "X-Remote-User: admin@example.com"
```

If `items` is empty, generate the first ES256 key with the Admin API:

```bash
curl -X POST http://localhost:14000/api/cimd-client-keys \
  -H "X-Remote-User: admin@example.com" \
  -H "Content-Type: application/json" \
  -d '{"algorithm":"ES256"}'
```

The first key is usable immediately. If keys already exist, inspect `activates_at`
before registration. A later current key can await activation while the previous key
signs assertions. Do not generate another key merely because a key is pending.
See [Manage CIMD client-authentication keys](/docs/guides/operate-oauth2-server-modes#manage-cimd-client-authentication-keys)
for key rotation.

These CIMD keys sign outbound `private_key_jwt` assertions, not broker-issued access
tokens. The separate `third_party_oauth2.jwe_signing_key` seals OAuth state and does
not sign CIMD assertions.

Create the service after the CIMD key is usable:

```bash
curl -X POST http://localhost:14000/api/services \
  -H "X-Remote-User: admin@example.com" \
  -H "Content-Type: application/json" \
  -d '{
    "display_name": "Corporate SSO with CIMD",
    "token_endpoint_auth_method": "private_key_jwt",
    "issuer_uri": "https://sso.corp.example.com",
    "discovery": { "enable_discovery": false },
    "endpoints": {
      "token_endpoint": "https://sso.corp.example.com/oauth/token",
      "authorize_endpoint": "https://sso.corp.example.com/oauth/authorize"
    },
    "scopes": [
      { "scope_value": "profile", "description": "Basic user profile" }
    ]
  }'
```

The response contains `token_endpoint_auth_method: "private_key_jwt"` and a broker-generated `client_id`. It omits `client_secret`.

The provider retrieves the public metadata document from `client_id`. It retrieves the CIMD public JWK Set from `<client_id>/jwks.json`.

To return to static authentication, send a complete replacement with a new non-empty `client_secret`. To use public authentication, send `token_endpoint_auth_method: "none"` and omit `client_secret`.

## Discover a protected resource with hosted CIMD or DCR

Protected-resource discovery starts at a public HTTPS resource URL. The broker
validates the resource metadata and selects one advertised authorization server.
Then it selects a hosted CIMD identity or registers a client with dynamic client
registration (DCR). This flow differs from direct authorization-server discovery
through `discovery.metadata_url`.

Both hosted CIMD and DCR require a stable public HTTPS `server.enduser.public_url`.
The provider reads the hosted CIMD document and public JWK Set at this URL.
DCR uses the URL to build its registered callback.

If the CIMD key set is empty, follow the key steps in
[Register a broker-hosted outbound CIMD confidential service](#register-a-broker-hosted-outbound-cimd-confidential-service)
before creation.

For either method, set `discovery.enable_discovery` to `true` and supply
`discovery.resource_url`. Omit `discovery.metadata_url`, `endpoints`, `client_id`,
`client_secret`, and `token_endpoint_auth_method`. The broker selects the client
method. Do not supply manual endpoints or credentials as a fallback in this request.

### Create a discovery-backed service

If the resource advertises one issuer, omit `issuer_uri`. The broker selects that
issuer. Create the service through the local Admin API:

```bash
curl -X POST http://localhost:14000/api/services \
  -H "X-Remote-User: admin@example.com" \
  -H "Content-Type: application/json" \
  -d '{
    "display_name": "Example MCP",
    "discovery": {
      "enable_discovery": true,
      "resource_url": "https://mcp.example.com/mcp"
    }
  }'
```

If the broker has a usable CIMD key and the issuer advertises hosted CIMD,
`private_key_jwt`, and ES256, the broker selects CIMD before DCR.
A selected CIMD identity does not fall back to DCR on failure. The response includes
these fields, along with the other service fields:

```json
{
  "id": "6c84fb90-12c4-11e1-840d-7b25c5ee775a",
  "client_id": "https://broker.example.com/.well-known/oauth-client/6c84fb90-12c4-11e1-840d-7b25c5ee775a",
  "token_endpoint_auth_method": "private_key_jwt",
  "issuer_uri": "https://auth.example.com/tenant",
  "discovery": {
    "enable_discovery": true,
    "resource_url": "https://mcp.example.com/mcp",
    "client_method": "cimd",
    "authorization_param_resource_strategy": "derived"
  },
  "authorization_params": { "resource": "https://mcp.example.com/mcp" }
}
```

Record the returned `id` for the permission set and status requests.

The broker derives `authorization_params.resource` from the verified resource URL
when you omit an override. It sends this one effective resource on authorization,
code exchange, and refresh.

If the issuer offers DCR but not compatible hosted CIMD, configure the optional
deployment-wide broker name before creation:

```yaml
third_party_oauth2:
  client_name: "Example Platform"
```

The broker sends this value as DCR `client_name`, not the service's `display_name`.
An absent or blank `third_party_oauth2.client_name` stops DCR before registration
with HTTP 400 and `message: client_name_unconfigured`. It does not prevent broker
startup, manual services, or hosted CIMD. The broker keeps any DCR client secret
encrypted and omits it from every Admin API response.

If a provider requires a different token audience, set
`authorization_params.resource` explicitly:

```bash
curl -X POST http://localhost:14000/api/services \
  -H "X-Remote-User: admin@example.com" \
  -H "Content-Type: application/json" \
  -d '{
    "display_name": "Files MCP",
    "discovery": {
      "enable_discovery": true,
      "resource_url": "https://files.example.com/mcp"
    },
    "authorization_params": { "resource": "https://files.example.com/api" }
  }'
```

Use a non-empty absolute URI without a fragment for this override. The response
includes these fields for a confidential DCR selection. It omits `client_secret`
even though the broker stores an encrypted credential:

```json
{
  "id": "880e8400-e29b-41d4-a716-446655440003",
  "display_name": "Files MCP",
  "client_id": "dcr-client-123",
  "token_endpoint_auth_method": "client_secret_basic",
  "issuer_uri": "https://login.example.com",
  "discovery": {
    "enable_discovery": true,
    "resource_url": "https://files.example.com/mcp",
    "client_method": "dcr",
    "authorization_param_resource_strategy": "pinned"
  },
  "authorization_params": { "resource": "https://files.example.com/api" }
}
```

The broker prefers confidential DCR to public DCR. It uses the selected
`client_secret_basic` or `client_secret_post` method, or `none` for a public client.
Public DCR uses PKCE S256 without a shared secret. A rejected confidential
registration does not trigger a public retry.

`discovery.resource_url` identifies the resource for metadata validation.
`authorization_params.resource` is the effective token audience request. The
separate `protected_resources` list controls RFC 8693 token exchange. It does not
set this audience.

Create, read, list, and update responses include `discovery.authorization_param_resource_strategy`.
It is `derived` when the verified URL supplies the audience, or `pinned` when an administrator supplies `authorization_params.resource`.
The value is `pinned` even when the supplied audience equals the verified URL.
These response fragments show derived A, pinned A, and a manual service:

```json
{"discovery":{"resource_url":"https://files.example/mcp","authorization_param_resource_strategy":"derived"},"authorization_params":{"resource":"https://files.example/mcp"}}
```

```json
{"discovery":{"resource_url":"https://files.example/mcp","authorization_param_resource_strategy":"pinned"},"authorization_params":{"resource":"https://files.example/mcp"}}
```

```json
{"discovery":{"enable_discovery":false,"authorization_param_resource_strategy":null},"authorization_params":{}}
```

Manual and direct-`metadata_url` services report `null` for the strategy.
The strategy is response-only. Do not send it in a request or put it in the upstream `authorization_params` map.

### Select one issuer when the resource advertises several

If a resource advertises several issuers, a create request without `issuer_uri`
fails with HTTP 400.

Submit this request to see the validated choices:

```bash
curl -X POST http://localhost:14000/api/services \
  -H "X-Remote-User: admin@example.com" \
  -H "Content-Type: application/json" \
  -d '{
    "display_name": "Shared MCP",
    "discovery": {
      "enable_discovery": true,
      "resource_url": "https://shared.example.com/mcp"
    }
  }'
```

The authenticated error includes the validated `authorization_servers` list:

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

Select exactly one URL from that list. Retry the create request with that exact `issuer_uri`:

```bash
curl -X POST http://localhost:14000/api/services \
  -H "X-Remote-User: admin@example.com" \
  -H "Content-Type: application/json" \
  -d '{
    "display_name": "Shared MCP",
    "issuer_uri": "https://login-b.example.com",
    "discovery": {
      "enable_discovery": true,
      "resource_url": "https://shared.example.com/mcp"
    }
  }'
```

The broker validates resource metadata again on retry. It contacts only the
selected authorization server. A failed create stores no service, so it has no
discovery-status resource.

### Read status and refresh without replacing a working client

Read the stored discovery status through the authenticated Admin API:

```bash
curl http://localhost:14000/api/services/880e8400-e29b-41d4-a716-446655440003/discovery-status \
  -H "X-Remote-User: admin@example.com"
```

If the request has no operator principal, the broker returns HTTP 401 before it reads the service.

A successful setup returns `ready` with the active resource URL, issuer, client
method, attempt time, and success time. The read does not contact the provider:

```json
{
  "status": "ready",
  "resource_url": "https://files.example.com/mcp",
  "issuer_uri": "https://login.example.com",
  "client_method": "dcr",
  "last_attempt_at": "2026-10-06T10:00:00Z",
  "last_success_at": "2026-10-06T10:00:00Z",
  "failure_reason": null
}
```

Refresh discovery with `PUT /api/services/{service-id}`. Include the current
`display_name` and `discovery.resource_url`. Omit unchanged `authorization_params`
to retain both the parameter map and its derived or pinned source:

```bash
curl -X PUT http://localhost:14000/api/services/880e8400-e29b-41d4-a716-446655440003 \
  -H "X-Remote-User: admin@example.com" \
  -H "Content-Type: application/json" \
  -d '{
    "display_name": "Files MCP",
    "discovery": {
      "enable_discovery": true,
      "resource_url": "https://files.example.com/mcp"
    }
  }'
```

If no user sessions exist, a derived-audience refresh can update `authorization_params.resource`
from the new verified resource URL. An omitted `authorization_params` retains a pinned
audience instead. On a successful refresh with the same issuer, the broker updates
the endpoints but retains the client identity and exact token authentication method.

For example, a derived service can have both `discovery.resource_url` and
`authorization_params.resource` set to `https://files.example/mcp` (audience A).
To pin A without changing the audience, PUT `authorization_params.resource` with that same URI:

```json
{
  "display_name": "Files MCP",
  "discovery": { "enable_discovery": true, "resource_url": "https://files.example/mcp" },
  "authorization_params": { "resource": "https://files.example/mcp" }
}
```

The service response then reports `pinned` while the effective audience remains A.
To change only the verified resource URL to `https://files.example/new-mcp` (B), omit `authorization_params`:

```json
{
  "display_name": "Files MCP",
  "discovery": { "enable_discovery": true, "resource_url": "https://files.example/new-mcp" }
}
```

The effective audience remains A, and the response still reports `pinned`.
This URL change is allowed with user sessions because the effective audience does not change.
To restore derivation from B, PUT a map without `resource`, such as `"authorization_params": {}`.
That operation changes the effective audience. Terminate every user session for this service first, including expired sessions.

Use this order for an audience change:

1. Read the active service with `GET /api/services/{service-id}`. Record its effective audience and service ETag.
2. Terminate all user sessions for the service, including expired sessions.
3. PUT the new audience. Include `resource` to pin it, or supply a parameter map without `resource` to derive it.
4. Read the active service again. Compare its audience and source strategy with the intended values. Record its new ETag.

If sessions still exist, an audience-changing PUT returns HTTP 409:

```json
{"error":"conflict","message":"resource_change_requires_no_sessions"}
```

The broker checks for sessions before provider discovery and again when the update commits.
An issuer change with sessions returns `issuer_change_requires_no_sessions` first.
If you replace `protected_resources` in the PUT, send the current service ETag in `If-Match`.
The service ETag covers active configuration and owned protected resources.

If the refresh fails, the PUT can return a safe error such as:

```json
{
  "error": "discovery failed",
  "message": "authorization_server_metadata_invalid"
}
```

Read discovery status again with the GET request shown earlier. It can report
`failed` while retaining the last success:

```json
{
  "status": "failed",
  "resource_url": "https://files.example.com/mcp",
  "issuer_uri": "https://login.example.com",
  "client_method": "dcr",
  "last_attempt_at": "2026-10-06T11:00:00Z",
  "last_success_at": "2026-10-06T10:00:00Z",
  "failure_reason": "authorization_server_metadata_invalid"
}
```

When a discovery attempt fails, the PUT changes only the stored attempt time and safe failure reason.
It preserves the active issuer, client identity, credential, endpoints, effective audience,
owned protected resources, sessions, success time, and service ETag. An invalid request before discovery does not write status.
For an audience conflict, discovery status records `failure_reason: resource_change_requires_no_sessions`.
The discovery-status GET has no ETag. A failure-only status write does not change the service version.
If an audience-changing PUT converts a discovery-backed service to manual configuration,
the broker returns the same 409 without a discovery-status write.
An issuer change needs an explicit `issuer_uri` and no user sessions.
The failure reason is a safe code, not a provider response or secret.

The broker rejects a callback from the old issuer or audience before it exchanges the code.

Read the active service through the authenticated Admin API:

```bash
curl http://localhost:14000/api/services/880e8400-e29b-41d4-a716-446655440003 \
  -H "X-Remote-User: admin@example.com"
```

Compare its `client_id`, endpoints, and `authorization_params.resource` with the
last successful response. Discovery and registration failures return safe codes
in `message`, not remote response bodies.

For a manual service, read the same authenticated status endpoint with its service
ID:

```bash
curl http://localhost:14000/api/services/770e8400-e29b-41d4-a716-446655440002/discovery-status \
  -H "X-Remote-User: admin@example.com"
```

It returns `not_applicable` with null discovery fields:

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

The status reports metadata and registration outcomes, not the audience of an
issued token. A provider can reject the requested `resource` during account
connection. The broker does not retry without it.

Confirm the issued token's suitability with the protected resource before use.

If a discovery create fails, the broker does not create a partial service or
switch to static credentials. A failed refresh keeps the existing discovery-backed
service. It does not convert the service to manual configuration.

For manual setup, follow [Register a third-party OAuth2 service](#register-a-third-party-oauth2-service).
Set `enable_discovery: false` and provide an HTTPS `issuer_uri`, manual endpoints,
and provider credentials. Do not include `discovery.resource_url` in this separate
request.

## Define permission sets

A permission set is a business-readable group of scopes for one or more services. Users
consent to the group, not raw provider scope strings. Create a permission set with
`POST /api/permission-sets`.

The fields:

- `name` — A unique, human-readable name with at most 255 characters.
- `description` — The capabilities in words that users understand.
- `service_scopes` — One or more `{service_id, scopes[], requirement_type}` values.
  `service_id` is a service UUID. Omit `scopes` or use an empty list when the service has no
  scopes. Otherwise, each scope must match the service configuration. `requirement_type` is
  `mandatory` or `optional`.

```bash
curl -X POST http://localhost:14000/api/permission-sets \
  -H "X-Remote-User: admin@example.com" \
  -H "Content-Type: application/json" \
  -d '{
    "name": "GitHub Read Access",
    "description": "Read repository contents and user profile from GitHub",
    "service_scopes": [
      {
        "service_id": "550e8400-e29b-41d4-a716-446655440001",
        "scopes": [ "repo", "user:email" ],
        "requirement_type": "mandatory"
      }
    ]
  }'
```

The response contains the permission-set `id` (a UUID), `created_at`, and `updated_at`. An
agent references this `id`. A change to `service_scopes` applies to the next token exchange
for grants that reference the set.

## Register an agent

An agent is the AI agent that requests delegated access. Create one with `POST /api/agents`.

The fields:

- `display_name` and `description` — Required values for the consent interface.
- `permission_sets` — At least one `{permission_set_id, requirement_type}` value is
  required. Mandatory sets stop authorization until a user grants them. Optional sets let
  an agent continue without the access. See
  [delegation and consent](/docs/concepts/delegation-and-consent#mandatory-vs-optional-requirements).
- `client_id` — Optional. Use it only for an agent that uses an upstream OAuth2 client ID.
  Omit it for agents identified in another way.
- `governance_url`, `user_documentation_url`, and `agent_interface_url` — Optional URLs for
  the consent interface.

```bash
curl -X POST http://localhost:14000/api/agents \
  -H "X-Remote-User: admin@example.com" \
  -H "Content-Type: application/json" \
  -d '{
    "display_name": "Research Assistant",
    "description": "AI assistant that reads repositories and datasets on your behalf",
    "governance_url": "https://example.com/agents/research-assistant/governance",
    "user_documentation_url": "https://example.com/docs/research-assistant",
    "permission_sets": [
      {
        "permission_set_id": "a1b2c3d4-e5f6-7890-abcd-ef1234567890",
        "requirement_type": "mandatory"
      }
    ]
  }'
```

The response contains an agent with a system-generated `id` (a UUID). This `id` is the
agent canonical identifier. Use it as `client_id` on the broker `/oauth2/authorize`
endpoint. Do not use the optional upstream `client_id` field. The broker does not generate
an upstream `client_id`.

## Issue broker client credentials

In `local` or `hybrid` [server mode](/docs/guides/operate-oauth2-server-modes), the broker
issues agent credentials for the token endpoint. Generate one credential set for each agent
with `POST /api/agents/{agent-id}/client-credentials`. Use the agent UUID in the path.

```bash
curl -X POST http://localhost:14000/api/agents/550e8400-e29b-41d4-a716-446655440000/client-credentials \
  -H "Content-Type: application/json" \
  -H "X-Remote-User: admin@example.com"
```

The response returns:

```json
{
  "client_id": "550e8400-e29b-41d4-a716-446655440000",
  "client_secret": "brk_sec_a1b2c3d4e5f6g7h8i9j0k1l2m3n4o5p6",
  "created_at": "2025-12-19T10:30:00Z"
}
```

The response has this structure:

- `client_id` equals the agent UUID. Each agent has one credential set.
- `client_secret` starts with `brk_sec_`. The response shows it only once. Store it securely.
  You cannot read it again. `GET …/client-credentials` returns metadata only.
- A repeated `POST` rotates the credential. It returns `200`, a new secret, and
  `previous_invalidated_at`. Tokens from the previous secret remain valid until expiry.

## Provider-specific configuration hints

### Google

Google requires two `authorization_params` to support token refresh:

```json
"authorization_params": {
  "access_type": "offline",
  "prompt": "consent"
}
```

- **`access_type: offline`** — This parameter makes Google issue a refresh token with the
  access token. Without it, Google returns only a short-lived access token. The broker
  cannot refresh the session after that token expires.
- **`prompt: consent`** — This parameter shows the consent interface for every
  authorization. Google issues a refresh token only for the first consent grant for one
  user-client pair. If a user previously authorized the OAuth2 client, Google does not
  include a refresh token in later responses. `prompt: consent` requests a new refresh
  token.

If either parameter is absent, the broker stores no refresh token. After the access token
expires, the broker returns `invalid_grant` and *"User session has expired. All tokens are
no longer valid"*. The user must authenticate again.

:::tip
After you add or change `authorization_params`, existing sessions do not change. A user
whose session has no refresh token must authenticate again to use the new parameters.
:::

## Related

- **[/api/admin](/api/admin)** — the full admin OpenAPI reference: every field, response
  schema, and error code.
- **[Delegation and consent](/docs/concepts/delegation-and-consent)** — how the objects you
  create here become grants and sessions.
- **[Operate OAuth2 server modes](/docs/guides/operate-oauth2-server-modes)** — when an agent
  needs broker client credentials, and how proxy, local, and hybrid modes differ.
