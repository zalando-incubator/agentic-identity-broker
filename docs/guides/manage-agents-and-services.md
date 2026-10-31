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
- `credential_source` — Select `stored` or `filesystem`. Omitted creation selects `stored`. Omitted PUT preserves the current source.
- `client_registration` — Select `static` for an operator client ID or `cimd` for a broker-hosted client identifier URL. This is independent of `token_endpoint_auth_method`.
- `token_endpoint_auth_method` — Omit it or send `null` for a shared secret. Use `none` for no client authentication or `private_key_jwt` for a signed assertion.
- `client_id` / `client_secret` — Inline credentials for stored shared-secret services. The broker encrypts the secret and returns `"REDACTED"`.
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

The response contains a generated service `id` and its `credential_source`. Stored shared-secret responses redact the secret. Filesystem responses omit both credential fields.
The response also includes the resolved `client_registration`. Admin API 2.1.0 adds this field to the 2.0.0 contract without changing its routes.

### Increment 2 (issue #264): Select client registration

| Client registration | Token endpoint method | Service mode |
| --- | --- | --- |
| `static` | Absent or `null` | Static confidential service |
| `static` | `none` | Static public service |
| `cimd` | `private_key_jwt` | CIMD confidential service |
| `cimd` | `none` | CIMD public service |

On create or full-replacement PUT, an omitted `client_registration` resolves to `cimd` for `private_key_jwt` and `static` otherwise. The broker never infers CIMD public. **Repeat `"client_registration": "cimd"` on every CIMD public PUT.** Otherwise, the request resolves to static and fails because it has no operator `client_id`.

The broker rejects `static` with `private_key_jwt` and `cimd` with an absent or `null` method. It returns HTTP 400 before discovery or provider traffic. An explicit `null`, empty, or unknown `client_registration` also returns HTTP 400. Google services and filesystem credential services cannot use `cimd`; explicit `static` remains valid.

Read and list responses always report the registration. Existing `private_key_jwt` services classify as `cimd`; all other stored services classify as `static`. This classification does not change their client IDs, methods, or sessions.

Record the `id`. Permission sets reference it.

:::note
The broker prevents deletion of a service that a grant references. `DELETE
/api/services/{service-id}` returns **409 `conflict`**. Revoke or migrate dependent grants
before you delete the service.
:::

## Migrate administrative consumers to Admin API 2.0.0

Admin API `2.0.0` is an unreleased breaking change from the `1.0.0` contract in broker release `v0.1.81`.
The existing `/api/services` routes remain unchanged. There is no compatibility endpoint or shim.

Before deployment:

1. Update administrative consumers to read `credential_source` and accept filesystem responses without `client_id` or `client_secret`.
2. Preserve stored-mode handling: responses include the stored client ID and redact applicable secrets as `REDACTED`.
3. For filesystem registration, supply a canonical ID and `credential_source: "filesystem"`. Omit both inline credential fields, including null placeholders.
4. Apply database migrations 039 and 040 before new broker pods start.
5. Enable filesystem mode only after all old broker pods are replaced.

Filesystem authentication acquires current credentials for each exchange or refresh attempt. Missing bindings or unusable files fail closed without stored-credential fallback.
Client-ID changes can require a new connection. The [filesystem service instructions](#register-and-update-a-filesystem-service) describe configuration and source transitions.

Guarded database downgrade refuses filesystem services and prior source transitions. A compatible downgrade discards non-secret session client-ID associations.

## Register and update a filesystem service

Filesystem mode supports standard and GitHub shared-secret authentication. Static public, CIMD confidential, CIMD public, and Google services retain stored mode.
The operator configures both paths in `third_party_oauth2.credential_files`, keyed by the exact, case-sensitive canonical ID.
See [credential file configuration](/docs/configuration#third-party-oauth2-credential-files) for configuration sources and publication rules.

Create a complete request without either inline credential field, including null-valued fields:

```bash
cat > service-request.json <<'JSON'
{
  "canonical_id": "corporate-api",
  "display_name": "Corporate API",
  "credential_source": "filesystem",
  "issuer_uri": "https://idp.corp.example.com",
  "discovery": {"enable_discovery": false},
  "endpoints": {
    "authorize_endpoint": "https://idp.corp.example.com/oauth/authorize",
    "token_endpoint": "https://idp.corp.example.com/oauth/token"
  },
  "scopes": []
}
JSON
curl --fail-with-body -X POST http://localhost:14000/api/services \
  -H 'X-Remote-User: admin@example.com' -H 'Content-Type: application/json' \
  --data-binary @service-request.json
```

Registration and metadata operations read no files. Missing bindings or files fail only when authentication requires them.
For the returned UUID in `$SERVICE_ID`, inspect metadata without resolving credentials:

```bash
curl --fail-with-body -H 'X-Remote-User: admin@example.com' \
  "http://localhost:14000/api/services/${SERVICE_ID}"
```

A full PUT can preserve filesystem mode by omitting `credential_source`. Keep the canonical ID, or omit it to preserve the existing ID.
To switch from stored to filesystem, send the complete filesystem request. The update atomically removes both stored credentials.

```bash
curl --fail-with-body -X PUT \
  -H 'X-Remote-User: admin@example.com' -H 'Content-Type: application/json' \
  --data-binary @service-request.json \
  "http://localhost:14000/api/services/${SERVICE_ID}"
```

To return to stored, send a complete request with `credential_source: "stored"` and both explicit, non-empty inline credentials.
Do not copy `REDACTED` from a response. The broker encrypts the supplied secret and imports no file value.
Set provider-issued credentials in the `PROVIDER_CLIENT_ID` and `PROVIDER_CLIENT_SECRET` environment variables for this request:

```bash
jq '. + {credential_source:"stored", client_id:env.PROVIDER_CLIENT_ID, client_secret:env.PROVIDER_CLIENT_SECRET}' \
  service-request.json | curl --fail-with-body -X PUT \
  -H 'X-Remote-User: admin@example.com' -H 'Content-Type: application/json' \
  --data-binary @- "http://localhost:14000/api/services/${SERVICE_ID}"
```

For protected-resource replacement, retain the existing strong `If-Match` requirement. A source transition never changes established client identities.
Matching recorded identities can continue. Different or missing identities require a new connection after a transition, including a return to stored.

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
    "client_registration": "cimd",
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

The response contains `client_registration: "cimd"`, `token_endpoint_auth_method: "private_key_jwt"`, and a broker-generated `client_id`. It omits `client_secret`.

The provider retrieves the public metadata document from `client_id`. It retrieves the CIMD public JWK Set from `<client_id>/jwks.json`.

For a static confidential service, send a complete replacement with `client_registration: "static"`, an operator `client_id`, and a non-empty `client_secret`. For a CIMD public service, keep `client_registration: "cimd"`, set the method to `none`, and omit both credentials.

### Increment 2 (issue #264): Register a CIMD public service

Set `server.enduser.public_url` to a stable public HTTPS URL before registration. A CIMD public service needs no CIMD client-authentication key. Do not create one for this service.

Replace the sample issuer and endpoint URLs with the values for your ServiceNow instance before you send this request.
For a ServiceNow-style provider that accepts CIMD but requires `none`, create the service without a client ID or secret:

```bash
curl --fail-with-body -X POST http://localhost:14000/api/services \
  -H 'X-Remote-User: admin@example.com' -H 'Content-Type: application/json' \
  -d '{
    "display_name": "ServiceNow with CIMD",
    "client_registration": "cimd",
    "token_endpoint_auth_method": "none",
    "issuer_uri": "https://servicenow.example.com",
    "discovery": { "enable_discovery": false },
    "endpoints": {
      "authorize_endpoint": "https://servicenow.example.com/oauth_auth.do",
      "token_endpoint": "https://servicenow.example.com/oauth_token.do"
    },
    "scopes": []
  }'
```

Record the response `id` as `SERVICE_ID`. The response reports `client_registration: "cimd"`, `token_endpoint_auth_method: "none"`, and a broker-hosted `client_id`. It has no `client_secret`.

Read the returned `client_id` from the service, then fetch its public document:

```bash
CLIENT_ID=$(curl --fail-with-body -sS -H 'X-Remote-User: admin@example.com' \
  "http://localhost:14000/api/services/${SERVICE_ID}" | jq -r '.client_id')
curl -i "${CLIENT_ID}"
curl -i "${CLIENT_ID}/jwks.json"
```

The first response is HTTP 200 with `Content-Type: application/json` and `Cache-Control: public, max-age=300`. Its JSON has exactly `client_id`, `redirect_uris`, `grant_types`, `response_types`, and `token_endpoint_auth_method: "none"`. The sole redirect URI is `<public_url>/api/third-party/<SERVICE_ID>/oauth2/callback`. The grant types are `authorization_code` and `refresh_token`; the response type is `code`. The document has no `jwks_uri` or `token_endpoint_auth_signing_alg`. The JWK request returns the normal JSON 404, even when the broker has CIMD keys for other services.

Add the service to a permission set and connect through the broker's normal user flow. Authorization uses the URL as `client_id` and PKCE S256. Code exchange sends `client_id`, `code`, and `code_verifier` without a secret, assertion, or Authorization header. Refresh sends `client_id` and `refresh_token` without client authentication. The broker stores the encrypted session and replacement tokens. A provider rejection fails closed; the broker does not retry with another method.

For a full-replacement PUT, send the same complete service fields and repeat `"client_registration": "cimd"`. Keep `client_id` and `client_secret` absent.

CIMD public and CIMD confidential can switch methods while retaining the same client identifier URL. A switch to CIMD confidential requires a usable CIMD key. On a static-to-CIMD switch, the broker replaces the operator client ID with its URL. It removes any stored secret only after validation.

On a CIMD-to-static switch, supply an operator `client_id`. Static confidential also requires a non-empty secret. The old hosted document then returns `404`. If the provider rejects old refresh tokens after the identity change, the user reconnects. Providers can cache the previous document for up to 300 seconds.

For example, replace the ServiceNow-style service without changing its registration:

```bash
curl --fail-with-body -X PUT "http://localhost:14000/api/services/${SERVICE_ID}" \
  -H 'X-Remote-User: admin@example.com' -H 'Content-Type: application/json' \
  -d '{
    "display_name": "ServiceNow with CIMD",
    "client_registration": "cimd",
    "token_endpoint_auth_method": "none",
    "issuer_uri": "https://servicenow.example.com",
    "discovery": { "enable_discovery": false },
    "endpoints": {
      "authorize_endpoint": "https://servicenow.example.com/oauth_auth.do",
      "token_endpoint": "https://servicenow.example.com/oauth_token.do"
    },
    "scopes": []
  }'
```

During a rolling upgrade after migration 044, old broker pods fail closed on two writes: creating a `private_key_jwt` service and rewriting a CIMD service to an operator client ID. PostgreSQL rejects those writes. Retry them after all old pods leave service.

CAUTION: Before a database downgrade through migration 044, convert or delete every CIMD public service. The rollback refuses to reinterpret its hosted URL as a static public client ID. If the migration leaves a dirty state, run `migrate force 44` after the conversion or deletion. Then retry the rollback.

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
`discovery.resource_url` requests must omit `client_registration`. A supplied value returns HTTP 400 before discovery traffic. Discovery never selects CIMD public.

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
  "client_registration": "cimd",
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
  "client_registration": "static",
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
The selected `discovery.client_method` determines registration reporting: hosted CIMD reports `cimd`, and DCR reports `static`.

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

**Feature 055 delivery status:** Admin API 3.0.0 has maintainer approval; implementation and
mandatory acceptance validation are pending. The uniqueness/reconcile descriptions below are its
approved contract, not a release announcement. Before upgrading, follow the
[external-ID uniqueness runbook](../operations/agent-external-id-uniqueness.md).

The fields:

- `display_name` and `description` — Required values for the consent interface.
- `permission_sets` — At least one `{permission_set_id, requirement_type}` value is
  required. Mandatory sets stop authorization until a user grants them. Optional sets let
  an agent continue without the access. See
  [delegation and consent](/docs/concepts/delegation-and-consent#mandatory-vs-optional-requirements).
- `client_id` — Optional. Use it only for an agent that uses an upstream OAuth2 client ID.
  Omit it for agents identified in another way.
- `external_id` — Optional external governance identifier, such as a URN. In Admin API 3.0.0 a set value must be non-empty and unique across agents by exact, case-sensitive equality. Any number of agents may omit it or store null. Empty strings return `400 validation failed`; a value held by another agent returns `409 conflict`.
- `redirect_uris` — Optional. Register HTTPS, loopback HTTP (`localhost`, `127.0.0.1`, or `[::1]`), or native-app private-use callbacks such as `com.example.app:/oauth2redirect/example-provider` and `github-app://oauth/callback`. The broker stores accepted entries unchanged and in order. Private-use entries must be canonical, lowercase, unencoded, and free of fragments.
- `governance_url`, `user_documentation_url`, and `agent_interface_url` — Optional URLs for
  the consent interface.

```bash
curl -X POST http://localhost:14000/api/agents \
  -H "X-Remote-User: admin@example.com" \
  -H "Content-Type: application/json" \
  -d '{
    "display_name": "Research Assistant",
    "external_id": "urn:example:ai-registry:agent:7f3a9c",
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

### Find agents by external ID

Filter the admin agent list by an external governance ID:

```bash
curl 'http://localhost:14000/api/agents?external_id=urn%3Aexample%3Aai-registry%3Aagent%3A7f3a9c' \
  -H 'X-Remote-User: admin@example.com'
```

In Admin API 3.0.0 the response remains an array, containing zero or one agent. Set external IDs are unique.
The filter matches the complete value exactly and respects case. It does not search agent UUIDs or canonical IDs.
Percent-encode reserved characters such as `&`, `+`, and `;` in the value. An unencoded semicolon is invalid.
If `external_id` is empty, repeated, undecodable, or longer than 255 Unicode code points, the broker returns `400` with `error: invalid external_id`.
Omit the parameter to list every agent. Filtered responses also accept `Prefer: reference-id=canonical` for nested references.

### Reconcile one registry entry without a preliminary lookup

Admin API 3.0.0 adds `PUT /api/agents?external_id={value}`. Supply one selector with 1–255
Unicode code points, valid UTF-8, no NUL, and correct percent encoding. It selects only by external
ID, never UUID, canonical ID or client ID. Missing, empty, repeated, malformed, or unencoded
semicolon selectors return `400 invalid external_id` and change nothing.

Use the existing `AgentCreateRequest`, with required display name, description and nonempty valid
permission declarations. This example requires an existing permission set with canonical ID
`github-read` (or replace it with its UUID). The first call creates an Agent with `201`, a UUID
`id` and relative `Location: /api/agents/{uuid}`. Identical completed replays return `200` with the
same representation and timestamps; changing the definition updates the same UUID with `200`.

```bash
curl -i -X PUT 'http://localhost:14000/api/agents?external_id=urn%3Aexample%3Aai-registry%3Aagent%3A7f3a9c' \
  -H 'X-Remote-User: admin@example.com' \
  -H 'Content-Type: application/json' \
  -H 'Prefer: reference-id=canonical' \
  -d '{
    "canonical_id": "research-assistant",
    "display_name": "Research Assistant",
    "description": "Helps with academic research",
    "permission_sets": [{"permission_set_id":"github-read","requirement_type":"mandatory"}],
    "client_uris": ["https://agent.example.com/z-client.json","https://agent.example.com/a-client.json"]
  }'
```

The response uses `github-read` for the nested reference and includes
`Preference-Applied: reference-id=canonical` and `Vary: Prefer`. The Agent identity and `Location`
remain UUID-addressed. Without the preference, nested references use UUIDs.

All Admin Agent responses (POST, both PUT forms, single GET and list/filter GET) present
`client_uris` in ascending Go string byte order. Thus this request returns `a-client.json` before
`z-client.json`; changing only their input order is a no-op. URI spelling, stored order, other
collections, and empty-field omission do not change.

All Admin Agent timestamps are UTC RFC 3339 with zero to six fractional digits, microsecond
truncation and no trailing zeroes. Reconcile creation uses the same microsecond instant for both
timestamps. A sequential real update sets `updated_at` to the later of microsecond now and the
observed value plus one microsecond, preserving `created_at`. A rapid update can therefore move
`2026-10-30T09:00:00Z` to `2026-10-30T09:00:00.000001Z`; its identical replay and next GET retain
that precise value. It can lead wall time slightly under rapid changes or a backward clock.
This is not an ETag or concurrent-write ordering guarantee. Other write operations retain their
existing timestamp assignment.

Collection PUT's replacement policy is not a general patch:

| Body field | Omitted on update | Explicit null | Supplied value |
| --- | --- | --- | --- |
| `client_id` | Preserve | `400 validation failed` | Replace under existing conflict rules |
| `canonical_id` | Preserve | Clear | Replace |
| `external_id` | Take selector | `400 invalid external_id` | Must equal selector |
| Three metadata URLs | Clear | Clear | Replace |
| `service_requirements`, `redirect_uris`, `allowed_scopes`, `client_uris` | Empty | Empty | Replace whole collection |
| `display_name`, `description`, `permission_sets` | Reject | Reject | Replace; references must be valid |

For example, sending only the three required fields preserves existing client/canonical IDs but
clears metadata and optional collections; replaying it is then unchanged. Adding
`"canonical_id": null` clears the canonical ID. Sending `"client_id": null` instead rejects the
whole request; to clear a client ID use the existing ID-addressed PUT. An omitted external ID takes
the selector, but a different, empty or null body external ID returns `400 invalid external_id`.

Canonical-ID, configuration-dependent client-ID and client-URI conflicts return `409 conflict` on
both branches. Invalid definitions/references return `400 validation failed`. Failures leave
Agents and relationships unchanged. Actual updates never reassign grants. Identical concurrent
first calls yield one creation and the remaining existing-Agent responses. Different concurrent
definitions remain last-writer-wins; no bulk sync, delete-by-external-ID, or `If-Match` is added.

### Migrate agent consumers to Admin API 3.0.0

This approved breaking contract replaces released Admin API 2.1.0 on the existing routes, without
a compatibility endpoint. It does not change the end-user or ExtProc APIs or client-ID sharing
configuration. Update consumers to accept 400/409 uniqueness errors, zero-or-one filtered arrays,
byte-sorted client URIs and fractional timestamps. Do not assume each PUT advances `updated_at`.

Migration 046 blocks the release if legacy duplicates or empty IDs remain. Use the
[runbook](../operations/agent-external-id-uniqueness.md) to inventory **every** affected UUID/value,
coordinate writers, assign distinct IDs or deliberately clear them through complete ID-addressed
PUTs, inspect dirty metadata, retry, and verify rollback/reapply. UUIDs, grants and all relationships
are preserved; neither migration nor broker chooses survivors or deletes data. Do not deploy until
implementation and the required PostgreSQL/E2E tests are complete.

### Allow a native-app callback for a CIMD agent

Register the exact callback on the Agent before the CIMD client uses it:

```bash
curl -X POST http://localhost:14000/api/agents \
  -H "X-Remote-User: admin@example.com" \
  -H "Content-Type: application/json" \
  -d '{
    "display_name": "GitHub Copilot",
    "description": "GitHub Copilot Desktop",
    "client_uris": ["https://github.com/copilot/desktop/client-metadata.json"],
    "redirect_uris": ["github-app://oauth/callback"],
    "permission_sets": [{
      "permission_set_id": "770e8400-e29b-41d4-a716-446655440001",
      "requirement_type": "mandatory"
    }]
  }'
```

The fetched CIMD document must also list `github-app://oauth/callback`. Without the Agent entry, the broker rejects the whole document, including its web callbacks. The broker matches private-use callbacks character for character in both lists. Register the lowercase, unencoded URI shown in the document. Web callbacks for a CIMD client come from the document, not the Agent list.

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

If either parameter is absent, the broker stores no refresh token. After the access token expires, token exchange returns `invalid_grant`.
Its `error_uri` starts third-party OAuth for the resolved service, for example:
`https://broker.example.com/api/third-party/550e8400-e29b-41d4-a716-446655440000/oauth2/authorize?redirect_uri=https%3A%2F%2Fbroker.example.com%2Fsessions`.
The broker uses its configured public URL and encodes the `/sessions` return URI. The provider callback returns the user to `/sessions`.

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
