---
title: "Operate the OAuth2 server modes"
description: Configure proxy, local, and hybrid OAuth2 authorization-server modes. Manage ES256 signing keys and add custom token claims with CEL.
---

# Operate the OAuth2 server modes

The broker exposes an OAuth2 authorization-server surface on end-user port 8000. The
surface contains `/oauth2/authorize`, `/oauth2/token`, `/oauth2/jwks.json`, and
`/.well-known/oauth-authorization-server`. The broker uses one **mode**. The mode decides
where authorization requests and tokens originate:

- **proxy** (default) — Forward authorization and token requests to an upstream OAuth2
  server. The broker still serves its metadata and republishes upstream keys.
- **local** — The broker is an authorization server. It issues JWT access tokens with
  managed ES256 keys.
- **hybrid** — The broker uses both modes. Agent registration selects the mode.

This guide explains the configuration for each mode. It also explains signing-key management
and CEL claims. See [OAuth2 server modes](/docs/concepts/oauth2-server-modes) for the
conceptual model.

## What you need

- A selected deployment mode. Use **proxy** when you use a corporate OAuth2 server. Use
  **local** when the broker issues agent tokens. Use **hybrid** when one deployment must
  serve both agent types.
- For proxy or hybrid mode, the upstream issuer, authorization endpoint, and token endpoint.
- For local or hybrid mode, an encryption backend. The broker encrypts signing-key private
  material. It does not start in these modes without encryption. See
  [configure encryption](/docs/guides/configure-encryption).
- Access to the broker YAML configuration and the admin API on port 14000.

All modes require PKCE `S256` at the authorization endpoint. `/oauth2/authorize` uses the
agent UUID (`agent.id`) as `client_id`. It does not use the upstream OAuth2 client ID.

## Proxy mode

Set `mode: proxy`. Configure upstream endpoints in `proxy`. The broker validates the user
grant, then forwards authorization and token requests. It republishes upstream signing keys
through its JWKS. `upstream_jwks_min_refresh` and `upstream_jwks_max_refresh` limit key
refresh frequency.

```yaml
oauth2_authorization_server:
  mode: "proxy"
  proxy:
    upstream_issuer_uri: "https://auth.example.com"
    upstream_authorize_endpoint: "https://auth.example.com/authorize"
    upstream_token_endpoint: "https://auth.example.com/token"
    upstream_timeout: 30s
    upstream_jwks_min_refresh: "15m"
    upstream_jwks_max_refresh: "1h"
  supported_response_types:
    - code
  supported_grant_types:
    - authorization_code
```

The broker's own public URL — used as the `issuer` in the discovery metadata — comes from
`server.enduser.public_url`, not from this section.

## Local mode

Set `mode: local`. Configure token issuance in `local`. The broker issues JWT access tokens
in this mode. It does not use upstream endpoints. It supports `client_credentials` and
`authorization_code` with PKCE.

```yaml
oauth2_authorization_server:
  mode: "local"
  local:
    token_ttl: "1h"
    token_claims_expression: '{"team": agent.display_name}'
    signing_keys:
      bootstrap_timeout: 90s
```

- `token_ttl` — Token lifetime. Use a Go duration such as `30m`, `1h`, or `1h30m`.
- `token_claims_expression` — Optional CEL expression for custom claims. See
  [Custom token claims with CEL](#custom-token-claims-with-cel). An empty value adds no
  custom claims.
- `signing_keys.bootstrap_timeout` — The time limit to create the first signing key.

Local mode needs an encryption backend so it can protect signing-key private material:

```yaml
encryption:
  memory:
    raw_key: "base64-encoded-32-byte-key" # use aws_kms for production
```

## Hybrid mode

Set `mode: hybrid`. Configure both `proxy` and `local`. Both configurations are required.
The broker selects the mode from agent registration. Agents with an upstream client ID use
the proxy path. Local agents and CIMD agents receive locally issued tokens.

```yaml
oauth2_authorization_server:
  mode: "hybrid"
  proxy:
    upstream_issuer_uri: "https://auth.example.com"
    upstream_authorize_endpoint: "https://auth.example.com/authorize"
    upstream_token_endpoint: "https://auth.example.com/token"
    upstream_timeout: 30s
    upstream_jwks_min_refresh: "15m"
    upstream_jwks_max_refresh: "1h"
  local:
    token_ttl: "1h"
    token_claims_expression: ""
    signing_keys:
      bootstrap_timeout: 90s
  supported_response_types:
    - code
  supported_grant_types:
    - authorization_code
    - client_credentials
```

As with local mode, hybrid mode requires an encryption backend for signing-key material.

## Manage signing keys

Local and hybrid modes use broker-managed **ES256** keys. The broker encrypts the private
key material at rest. It publishes public keys at `/oauth2/jwks.json`. Use the admin API on
port 14000 to rotate keys. Every request includes the principal header from the proxy. The
proxy enforces administrator privilege. See
[configure authentication](/docs/guides/configure-authentication).

### Add a key

`POST /api/oauth2-server/signing-keys` creates a key. `ES256` is the default and the only
supported algorithm.

```bash
curl -X POST https://broker.internal:14000/api/oauth2-server/signing-keys \
  -H "X-Remote-User: admin@example.com" \
  -H "Content-Type: application/json" \
  -d '{"algorithm": "ES256"}'
```

The broker immediately publishes a new key to the JWKS. It marks the key as current. The key
does not sign tokens until `activates_at`. This grace period is twice the JWKS cache lifetime,
or 600 seconds. It lets verifiers obtain the public key before the broker issues a token with
it. The response contains `kid`, `algorithm`, `is_current`, `activates_at`, and `created_at`.

### List keys

`GET /api/oauth2-server/signing-keys` returns the keys, newest first, wrapped in an `items`
array.

```bash
curl https://broker.internal:14000/api/oauth2-server/signing-keys \
  -H "X-Remote-User: admin@example.com"
```

### Promote a key to current

`PUT /api/oauth2-server/signing-keys/{kid}/current` promotes an existing key. The previous
key is no longer current but remains valid for verification. Tokens that it signed remain
valid until they expire.

```bash
curl -X PUT https://broker.internal:14000/api/oauth2-server/signing-keys/{kid}/current \
  -H "X-Remote-User: admin@example.com"
```

### Retire a key

`DELETE /api/oauth2-server/signing-keys/{kid}` marks a key as deleted. The broker does not
delete the final key or current key. These requests return `409 last_key` or `409
current_key`. Add or promote a replacement first.

```bash
curl -X DELETE https://broker.internal:14000/api/oauth2-server/signing-keys/{kid} \
  -H "X-Remote-User: admin@example.com"
```

:::tip
Add a new key. Wait for its activation period. Promote it to current. Retire the old key
only after tokens that use it have expired.
:::


## Manage CIMD client-authentication keys

CIMD confidential third-party services use a separate ES256 key domain. These keys sign outbound `private_key_jwt` assertions.

They do not sign broker access tokens. The CIMD key routes work in proxy, local, and hybrid modes.

Token-signing key routes remain unavailable in proxy mode.

Create the first key before you register a CIMD confidential service:

```bash
curl -X POST https://broker.internal:14000/api/cimd-client-keys \
  -H "X-Remote-User: admin@example.com" \
  -H "Content-Type: application/json" \
  -d '{"algorithm":"ES256"}'
```

The first key is immediately usable. Later generated keys remain public during the activation grace period. Promote a key to make it usable immediately:

```bash
curl -X PUT https://broker.internal:14000/api/cimd-client-keys/{kid}/current \
  -H "X-Remote-User: admin@example.com"
```

List active keys with `GET /api/cimd-client-keys`. Remove only a non-current key with `DELETE /api/cimd-client-keys/{kid}`.

The broker rejects removal of the last, current, or effective-current key. The public CIMD JWK Set is at `<client-id>/jwks.json`.

`/oauth2/jwks.json` does not publish CIMD keys.

## Custom token claims with CEL

In local and hybrid modes, `local.token_claims_expression` can add claims to broker-issued
JWTs. The CEL expression must return a map of claim names to values. Three variables are
available:

| Variable | Type | Contents |
|---|---|---|
| `agent` | struct | The agent the token is issued for (for example `agent.display_name`). |
| `principal` | string | The authenticated user the token acts for. |
| `request` | map | The token request context. |

```yaml
oauth2_authorization_server:
  local:
    token_claims_expression: '{"team": agent.display_name}'
```

The broker sets the base OAuth2 claims, including `iss`, `sub`, and `exp`. This expression
cannot replace these claims. It adds claims beside them.

## Discovery and JWKS

The broker publishes two public endpoints on the end-user port in every mode. Clients and
gateways use these endpoints to discover the server and validate tokens:

- `GET /.well-known/oauth-authorization-server` — RFC 8414 metadata. This includes the
  issuer, endpoints, grant types, and `code_challenge_methods_supported: [S256]`.
- `GET /oauth2/jwks.json` — The public JWK set. Proxy mode republishes upstream keys. Local
  and hybrid modes publish the ES256 keys that you manage.

Point agents and token-validating gateways to these endpoints. Do not hard-code key or URL
values. This makes key rotation transparent.

## Related

- [OAuth2 server modes](/docs/concepts/oauth2-server-modes) — the concept behind these modes.
- [Manage agents and services](/docs/guides/manage-agents-and-services) — register the agents
  that authenticate against this server.
- [End-user API reference](/api/enduser) — the authorize, token, JWKS, and discovery
  contracts.
