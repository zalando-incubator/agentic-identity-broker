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

## Register a CIMD confidential service

Use `private_key_jwt` when the broker must authenticate to the provider without a shared secret.

Set `server.enduser.public_url` to the stable public HTTPS URL of the broker first.

Do not send `client_id` or `client_secret` in this request. The broker allocates the service ID. It then generates the Client ID Metadata URL.

Before you register the service, create or retain a usable CIMD client-authentication key. The broker rejects registration when it cannot publish a CIMD public JWK.

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
