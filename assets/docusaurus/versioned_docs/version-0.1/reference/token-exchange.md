---
title: "Token exchange"
description: Field-level reference for RFC 8693 `POST /oauth2/token` grants. This page covers third-party token exchange and local user impersonation requests, responses, errors, and audit events.
---

# Token exchange

This page describes the broker RFC 8693 token-exchange flows. The token endpoint supports
third-party token exchange and local user impersonation. They share a grant type but use
different requests, token issuance, and authorization.

:::note
For the concept and the request flow, read [Token exchange](../concepts/token-exchange.md).
To deploy transparent exchange at an Envoy-based gateway, see the
[token-exchange gateway guide](../guides/token-exchange-gateway.md). The complete generated
contract for this endpoint lives in the [end-user API reference](/api/enduser).
:::

The broker supports two RFC 8693 flows:

- **Third-party token exchange** returns a stored provider credential. The broker resolves a
  `resource` and validates the user delegation.
- **User impersonation** is available only in local mode. A privileged client provides
  assertion, actor, and subject credentials for the `audience` target agent. The broker
  issues a local access token. `sub` contains the subject. `act.iss` and `act.sub` contain
  the validated actor issuer and identity.

The following sections describe the request and response contracts.

## Overview

- **Endpoint:** `POST /oauth2/token` (end-user server, port 8000).
- **Grant type:** `urn:ietf:params:oauth:grant-type:token-exchange`.
- **Content type:** `application/x-www-form-urlencoded`.

The token endpoint uses `grant_type` to identify an RFC 8693 request. It processes the
request under RFC 8693. The same endpoint also supports `authorization_code` and
`client_credentials`. See [API overview](./api.md) and
[OAuth2 server modes](../concepts/oauth2-server-modes.md).

## Key concepts

### Tokens

**Subject token** (`subject_token`) — A JWT that carries the user principal and agent
identifier. CEL expressions select these values. The default user claim is `sub`. The
default agent claim is `azp`. The privileged client presents the token for exchange.

**Client assertion** (`client_assertion`) — A JWT that identifies the privileged client,
such as a gateway or reverse proxy. The broker validates it with the upstream OAuth2 server
JWKS. Its `sub` identifies the gateway in audit data.

**Third-party token** — The OAuth2 access token held in the broker token vault for a target
service, such as GitHub or Google. A successful exchange returns this token.

### Resource parameter

`resource` identifies the service token to return. The broker compares it to
service `protected_resources`. The broker removes a trailing slash before comparison. A
resource URI can belong to only one service.

```mermaid
flowchart LR
    A["resource=https://api.github.com"] --> B["Broker matches protected_resources"]
    B --> C["Service found: GitHub"]
    C --> D["Return that service's stored token"]
```

Configure `protected_resources` for a service through the [admin API](/api/admin). See
[Manage agents and services](../guides/manage-agents-and-services.md). The resource must be
a valid HTTP or HTTPS URI. The broker stores it without a trailing slash. A duplicate URI
in another service returns `409 conflict`.

### User grant

Before it returns a token, the broker validates the user grant. The subject token identifies
the user and agent. The grant must permit the agent to use the target service. Without an
active, unexpired grant, the broker denies token exchange. This enforces consent at request
time.

## Third-party token exchange request

`POST /oauth2/token` with `Content-Type: application/x-www-form-urlencoded`. This section
applies only when the request does not activate user impersonation.

### Required parameters

| Parameter | Value | Description |
|---|---|---|
| `grant_type` | `urn:ietf:params:oauth:grant-type:token-exchange` | Selects RFC 8693 token exchange. |
| `subject_token` | JWT string | The user's token (contains user principal + agent identifier). |
| `subject_token_type` | `urn:ietf:params:oauth:token-type:access_token` | The only accepted subject-token type. |
| `client_assertion` | JWT string | The gateway's authentication token, validated against upstream JWKS. |
| `client_assertion_type` | `urn:ietf:params:oauth:client-assertion-type:jwt-bearer` | The client authentication method. |
| `resource` | URI string | Target service resource URI (for example `https://api.github.com`). |

### Optional parameters

| Parameter | Value | Description |
|---|---|---|
| `scope` | Space-delimited scopes | Requested scopes. |
| `audience` | String | Intended audience for the issued token. |

### Request example

```bash
curl -X POST http://localhost:8000/oauth2/token \
  -H "Content-Type: application/x-www-form-urlencoded" \
  -d "grant_type=urn:ietf:params:oauth:grant-type:token-exchange" \
  -d "subject_token=eyJhbGciOiJSUzI1NiIsInR5cCI6IkpXVCJ9..." \
  -d "subject_token_type=urn:ietf:params:oauth:token-type:access_token" \
  -d "client_assertion=eyJhbGciOiJSUzI1NiIsInR5cCI6IkpXVCJ9..." \
  -d "client_assertion_type=urn:ietf:params:oauth:client-assertion-type:jwt-bearer" \
  -d "resource=https://api.github.com"
```

## Third-party token exchange response

A successful third-party exchange returns `200 OK` with `Content-Type: application/json`.

```json
{
  "access_token": "ghu_xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx",
  "token_type": "Bearer",
  "issued_token_type": "urn:ietf:params:oauth:token-type:access_token",
  "expires_in": 3600,
  "scope": "read:user repo"
}
```

### Response fields

| Field | Presence | Description |
|---|---|---|
| `access_token` | Always | The third-party OAuth2 access token from the token vault. |
| `token_type` | Always | Token type, passed through from the stored token (typically `Bearer`). |
| `issued_token_type` | Always | `urn:ietf:params:oauth:token-type:access_token`. |
| `expires_in` | Optional | Remaining lifetime in seconds, when the stored token carries expiry. |
| `scope` | Optional | Scopes associated with the token, when they differ from the request. |
| `refresh_token` | Optional | A refresh token — typically absent, since the broker manages refresh. |
| `granted_permission_sets` | Optional | Map of permission-set UUID → array of service UUIDs, when the exchange is scoped to specific permission sets. |

### Success example

```bash
curl -X POST http://localhost:8000/oauth2/token \
  -H "Content-Type: application/x-www-form-urlencoded" \
  -d "grant_type=urn:ietf:params:oauth:grant-type:token-exchange" \
  -d "subject_token=$UPSTREAM_TOKEN" \
  -d "subject_token_type=urn:ietf:params:oauth:token-type:access_token" \
  -d "client_assertion=$GATEWAY_JWT" \
  -d "client_assertion_type=urn:ietf:params:oauth:client-assertion-type:jwt-bearer" \
  -d "resource=https://api.github.com"
```

```json
{
  "access_token": "ghu_1234567890abcdefghijklmnopqrstuvwxyz",
  "token_type": "Bearer",
  "issued_token_type": "urn:ietf:params:oauth:token-type:access_token",
  "expires_in": 28800
}
```

## Third-party token exchange errors

Third-party token-exchange errors use the OAuth2 error format. The format contains `error`
and optional `error_description`.

| HTTP status | `error` | When it occurs |
|---|---|---|
| 400 | `invalid_request` | The request is malformed or missing a required parameter. |
| 400 | `invalid_target` | The `resource` matches no configured service. |
| 400 | `invalid_grant` | The user has no active session with the target service. |
| 401 | `invalid_client` | The `client_assertion` (or broker credentials) cannot be verified. |
| 403 | `access_denied` | The user has not granted the agent access, or a CEL policy denied it. |
| 500 | `server_error` | An unexpected server error. |

### 400 invalid_request

```json
{
  "error": "invalid_request",
  "error_description": "subject_token is required"
}
```

Common causes include a missing `subject_token`, `client_assertion`, or `resource`. Other
causes include an invalid token format or signature, an invalid resource URI, or a missing
claim such as `sub`.

### 400 invalid_target

```json
{
  "error": "invalid_target",
  "error_description": "No service configured for the requested resource"
}
```

No service has the requested URI in its `protected_resources`.

### 400 invalid_grant

```json
{
  "error": "invalid_grant",
  "error_description": "User has no active session with the requested service"
}
```

The user has no session for the service. Or, both stored access and refresh tokens expired
and cannot be refreshed.

### 401 invalid_client

```json
{
  "error": "invalid_client",
  "error_description": "Invalid client_assertion signature"
}
```

The `client_assertion` signature did not validate against the upstream JWKS. The assertion
can also be expired or have no broker audience.

### 403 access_denied

```json
{
  "error": "access_denied",
  "error_description": "User has not granted this agent access to the requested service"
}
```

The user has no active grant for the agent and service. The grant can be revoked or expired.
A CEL authorization expression can also be false.

### 500 server_error

```json
{
  "error": "server_error",
  "error_description": "An unexpected error occurred"
}
```

## User impersonation

User impersonation is a separate RFC 8693 profile on this endpoint. It is available only
when `oauth2_authorization_server.mode` is `local`. Proxy and hybrid modes reject the
request without forwarding it upstream. Impersonation does not get a third-party token,
require a user grant, or accept `resource`.

The token contains subject claim `sub` and actor claim `act`. In RFC 8693 terms, this is
delegation. This feature uses the name "impersonation" for operators. `act` preserves actor
accountability.

### Activation and request

The request activates impersonation when it contains one `audience`. The value must be
`<impersonation.audience_prefix>/<canonical lower-case AgentID UUID or canonical_id>`. The
suffix must resolve to a registered target agent. It can be the canonical target UUID or
`canonical_id`. Both forms produce the target UUID as `agent_id`. The target agent supplies
the local token policy `agent.*` context and optional-scope allow list. The routing audience
does not set issued `aud`. Only `token_claims_expression` controls this claim.

Send these form fields:

| Parameter | Value | Requirement |
|---|---|---|
| `grant_type` | `urn:ietf:params:oauth:grant-type:token-exchange` | Required. |
| `audience` | `<audience_prefix>/<canonical lower-case AgentID UUID or canonical_id>` | Required. Exactly one value. |
| `client_assertion_type` | `urn:ietf:params:oauth:client-assertion-type:jwt-bearer` | Required. |
| `client_assertion` | Signed JWT | Required. Authenticates the privileged client. |
| `actor_token_type` | `urn:ietf:params:oauth:token-type:jwt` | Required. |
| `actor_token` | Signed JWT | Required. Its validated issuer and identity populate `act.iss` and `act.sub`. |
| `subject_token_type` | `urn:ietf:params:oauth:token-type:jwt` | Required. |
| `subject_token` | Signed JWT or unsigned `alg:none` JWT for a matching `verification: none` rule | Required. Its extracted identity populates `sub`. |
| `scope` | Literal-space-separated scopes | Optional. Each non-reserved scope must be in the target `allowed_scopes`. An empty allow list permits all. `offline` and `offline_access` are reserved and permitted. |
| `requested_token_type` | `urn:ietf:params:oauth:token-type:access_token` | Optional. Another value returns `invalid_request`. |

`resource` must be absent. A bare suffix or invalid suffix returns `invalid_request`. A
well-formed UUID or canonical-ID suffix for an unregistered target returns `invalid_target`.

```bash
curl -X POST http://localhost:8000/oauth2/token \
  -H "Content-Type: application/x-www-form-urlencoded" \
  -d "grant_type=urn:ietf:params:oauth:grant-type:token-exchange" \
  -d "audience=https://broker.example.com/impersonation/550e8400-e29b-41d4-a716-446655440000" \
  -d "client_assertion_type=urn:ietf:params:oauth:client-assertion-type:jwt-bearer" \
  -d "client_assertion=$CLIENT_ASSERTION" \
  -d "actor_token_type=urn:ietf:params:oauth:token-type:jwt" \
  -d "actor_token=$ACTOR_TOKEN" \
  -d "subject_token_type=urn:ietf:params:oauth:token-type:jwt" \
  -d "subject_token=$SUBJECT_TOKEN" \
  -d "scope=read"
```

### Subject profiles

Both profiles use `subject_token_type=urn:ietf:params:oauth:token-type:jwt`. The signed
profile validates the client assertion, actor, and subject. It validates the selected issuer,
audience, expiry, not-before time, signature, and asymmetric algorithm allow list.

The unverified-subject profile is a broker extension. A matching rule declares
`verification: none`. It accepts an unsigned `alg:none` JWT as `subject_token`. It rejects
signed JWSs on this path. It does not allow unsigned client assertions, actors, or signed
subjects. A signed client assertion and the subject-binding CEL predicate authorize
caller-provided claims. The predicate must bind `subject_token.email` before the broker
issues an email claim.

### Response and errors

Success returns a locally signed broker JWT. It does not return a provider credential:

```json
{
  "access_token": "eyJhbGciOiJFUzI1NiIsInR5cCI6IkpXVCJ9...",
  "token_type": "Bearer",
  "issued_token_type": "urn:ietf:params:oauth:token-type:access_token",
  "expires_in": 3600,
  "scope": "read"
}
```

The JWT contains `sub`, target-derived `agent_id`, `act`, local-policy claims, and the
granted `scope`. The response includes `scope` only when it has a value. Each requested
non-reserved scope must be in the target allow list. An empty allow list permits all scopes.
`offline` and `offline_access` are reserved and permitted. A disallowed scope returns
`invalid_scope`. Malformed credentials and target syntax return `invalid_request`. An
untrusted client assertion returns `invalid_client`. A valid request with no permitted rule
returns `access_denied`. Errors do not expose credentials, signing keys, trust sources, or
rejected scopes.

### Audit events

Each impersonation decision records a credential-free structured log event. The event name
is `impersonation_decision`. It contains the routing audience, target, selected rule, issuer
identifiers, credential roles, safe identities, OAuth error or failure category, and
`request_id`. It does not contain client assertions, actor or subject tokens, issued access
tokens, or signing keys. Use this audit record to investigate a rejected request.

See [Configuration](../configuration.md) and
`examples/config/impersonation.yaml` for operator setup and examples.


## Configuration

Configure token exchange under `token_exchange`. The
[Configuration reference](../configuration.md) describes all settings. These two settings
matter most:

- **Claim extraction** — CEL expressions get the principal and agent identifier from the
  subject token.
- **Authorization** — A CEL expression evaluates `client_assertion` claims. It determines
  whether a privileged client can exchange a token.

```yaml
token_exchange:
  claim_extraction:
    principal_expression: subject_token.sub
    agent_id_expression: subject_token.azp
  authorization:
    cel:
      expression: "client_assertion.iss == 'api-gateway.example.com'"
```

The authorization expression can allow all clients (`"true"`). It can also select one
issuer or several issuers:

```yaml
token_exchange:
  authorization:
    cel:
      expression: |
        client_assertion.iss in [
          'api-gateway.example.com',
          'reverse-proxy.example.com'
        ]
```

## Worked example

This walks a GitHub delegation end to end. Administrative calls go to the admin server
(port 14000); consent and token exchange go to the end-user server (port 8000).

```bash
# 1. Admin registers the GitHub service with a protected resource (admin API, :14000)
curl -X POST http://localhost:14000/api/services \
  -H "X-Remote-User: admin@example.com" \
  -H "Content-Type: application/json" \
  -d '{
    "display_name": "GitHub",
    "client_id": "github-oauth-client",
    "issuer_uri": "https://github.com",
    "protected_resources": ["https://api.github.com"]
  }'
# → {"id": "aa0e8400-e29b-41d4-a716-446655440000", ...}

# 2. Admin creates a permission set bundling the GitHub scopes (admin API, :14000)
curl -X POST http://localhost:14000/api/permission-sets \
  -H "X-Remote-User: admin@example.com" \
  -H "Content-Type: application/json" \
  -d '{
    "name": "Read repositories",
    "description": "Read access to GitHub repositories",
    "service_scopes": [
      {
        "service_id": "aa0e8400-e29b-41d4-a716-446655440000",
        "scopes": ["repo"],
        "requirement_type": "mandatory"
      }
    ]
  }'
# → {"id": "770e8400-e29b-41d4-a716-446655440001", ...}

# 3. Admin registers the agent (admin API, :14000)
curl -X POST http://localhost:14000/api/agents \
  -H "X-Remote-User: admin@example.com" \
  -H "Content-Type: application/json" \
  -d '{"display_name": "GitHub Assistant"}'
# → {"id": "550e8400-e29b-41d4-a716-446655440000", ...}

# 4. User grants the permission set for that service (end-user API, :8000)
curl -X POST http://localhost:8000/api/consent/agents/550e8400-e29b-41d4-a716-446655440000/grants \
  -H "X-Remote-User: user@example.com" \
  -H "Content-Type: application/json" \
  -d '{
    "granted_permission_sets": {
      "770e8400-e29b-41d4-a716-446655440001": [
        "aa0e8400-e29b-41d4-a716-446655440000"
      ]
    }
  }'
# → 201 {"data": {"agent_id": "550e8400-...", "granted_permission_sets": {...}}}

# 5. User completes the GitHub authorization flow in the browser,
#    which stores an encrypted third-party session.

# 6. The gateway exchanges the agent's token for the GitHub token (end-user API, :8000)
curl -X POST http://localhost:8000/oauth2/token \
  -H "Content-Type: application/x-www-form-urlencoded" \
  -d "grant_type=urn:ietf:params:oauth:grant-type:token-exchange" \
  -d "subject_token=$UPSTREAM_JWT_WITH_USER_SUB_AND_AGENT_AZP" \
  -d "subject_token_type=urn:ietf:params:oauth:token-type:access_token" \
  -d "client_assertion=$GATEWAY_JWT" \
  -d "client_assertion_type=urn:ietf:params:oauth:client-assertion-type:jwt-bearer" \
  -d "resource=https://api.github.com"
# → 200 {"access_token": "ghu_...", "token_type": "Bearer", ...}

# 7. The gateway calls GitHub with the exchanged token
curl -H "Authorization: Bearer ghu_..." https://api.github.com/user
```

## Security considerations

- **Both JWTs are verified.** The `client_assertion` and `subject_token` signatures are
  validated against the upstream OAuth2 server's JWKS; expired or wrongly-audienced tokens are
  rejected.
- **A user grant is always required.** The broker returns a token only when the principal has
  an active, non-expired grant for the agent and service.
- **Token values are never logged.** The returned `access_token` does not appear in logs;
  audit records reference the principal, agent, service, and resource, not the token.
- **Transport is HTTPS.** Terminate TLS in front of the broker; tokens must not cross the
  network in the clear.

## Related

- [Token exchange (concept)](../concepts/token-exchange.md) — the model and request flow.
- [Token-exchange gateway guide](../guides/token-exchange-gateway.md) — transparent exchange
  at an Envoy-based gateway.
- [End-user API reference](/api/enduser) — the generated `/oauth2/token` contract.
- [RFC 8693 — OAuth 2.0 Token Exchange](https://www.rfc-editor.org/rfc/rfc8693).
