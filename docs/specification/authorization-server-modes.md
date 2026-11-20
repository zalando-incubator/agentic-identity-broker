---
title: "Authorization server modes"
description: Living specification of the three modes of the broker's OAuth2 authorization server and of what an agent sees in each mode. Every example on this page is checked against the acceptance suite.
---

# Authorization server modes

An **Agent** asks the broker for a token. Who signs that token depends on a choice that the
platform operator makes once, in configuration: the **OAuth2 server mode**. This page describes
the three modes and what an agent sees in each of them: how its `client_id` is resolved, what
discovery metadata says, and which keys the JWKS endpoint publishes.

:::note How to read this page
This is a living specification. A badge such as "tested" after a sentence links to the acceptance
scenario that proves it. Each HTTP exchange shown here is compared with what that scenario really
sent and received. `just living-docs` runs the comparison and fails when the page and the broker
disagree. UUIDs and timestamps are synthetic. The check compares timestamps by shape and UUIDs by
which of them are equal.
:::

## The cast

A platform operator runs the broker for a company. The company already has an OAuth2
authorization server, here called the **upstream** server. Some agents are registered there as
OAuth2 clients. The *Research Assistant* is one of them: the upstream server knows it under the
client ID `shared-upstream`.

Other agents are not known upstream. The broker itself has to issue their tokens.

The broker gives every agent a UUID. It also puts each agent into one `ClientType`, which it
derives from the registration and never stores:

| `ClientType` | The agent has | Who issues its tokens |
|---|---|---|
| `ProxyClient` | an upstream client ID | The upstream server |
| `LocalClient` | no upstream client ID and no client metadata document URL | The broker |
| `CIMDClient` | client metadata document URLs | The broker |

The user in the examples is signed in as `user@example.com`.

## The operator picks one of three modes

| Mode | The broker | It accepts |
|---|---|---|
| `proxy` | forwards authorization and token requests to the upstream server | `ProxyClient` |
| `local` | issues its own tokens | `LocalClient`, `CIMDClient` |
| `hybrid` | does both, and decides for each request by the agent's `ClientType` | all three |

The mode is one configuration value. Each mode has its own section, and `hybrid` needs both:

```yaml
oauth2_authorization_server:
  mode: "hybrid"
  proxy:
    upstream_issuer_uri: "https://auth.example.com"
    upstream_authorize_endpoint: "https://auth.example.com/oauth/authorize"
    upstream_token_endpoint: "https://auth.example.com/oauth/token"
  local:
    token_ttl: "1h"
```

The broker checks this block when it starts and refuses a combination that cannot work:

- The mode has no default. An empty `mode` is an error, even when the upstream endpoints are set. `example:authorization-server-modes.configuration.mode-required`
- The former mode name `issue_token` is an error. The message names `local` as the replacement. `example:authorization-server-modes.configuration.legacy-mode-name`
- `proxy` mode rejects settings of the `local` section, such as `local.token_ttl`. `local` mode
  rejects settings of the `proxy` section, such as `proxy.upstream_issuer_uri`. `example:authorization-server-modes.configuration.opposite-mode-fields`
- `hybrid` mode needs the three upstream endpoints. When `local.token_ttl` is absent, it becomes one hour. `example:authorization-server-modes.configuration.hybrid-requires-both`
- Client metadata documents can be enabled in `local` and `hybrid` mode, not in `proxy` mode. `example:authorization-server-modes.configuration.cimd-gating`

In `proxy` and `hybrid` mode the broker reads the metadata of the upstream server at startup.
When it cannot reach that metadata, it does not start. `example:authorization-server-modes.startup.upstream-unreachable`

## An agent always presents its broker identity

An agent never sends its upstream client ID to the broker. The `client_id` of a request is the
agent's UUID or, for a `CIMDClient`, the URL of its client metadata document. The broker finds
the agent first and only then decides, by `ClientType` and mode, what to do with the request.

## Proxy mode: the broker forwards to the upstream server

The broker runs in `proxy` mode. The user has already granted the Research Assistant access. The
agent starts an authorization request with its UUID. The broker finds the agent, replaces the
UUID with the upstream client ID, and sends the browser to the upstream server: `example:authorization-server-modes.proxy.forward-authorize`

```http example="authorization-server-modes.proxy.forward-authorize"
GET /oauth2/authorize?client_id=550e8400-e29b-41d4-a716-446655440000&redirect_uri=https://client.example.com/cb&response_type=code&state=xyz HTTP/1.1
X-Remote-User: user@example.com

HTTP/1.1 302 Found
Location: http://127.0.0.1:…/oauth/authorize?client_id=shared-upstream&redirect_uri=https%3A%2F%2Fclient.example.com%2Fcb&response_type=code&state=xyz

<a href="…">Found</a>.
```

The upstream server never sees the UUID, and the agent never needs to know `shared-upstream`.

### Several agents behind one upstream client

Registering every agent as its own upstream client can be slow or impossible. The operator can
therefore let several agents share one upstream client ID. The setting
`multi_agent_client` names a query parameter and a token claim, here both `x_agent_id`.

Two agents now hold the client ID `shared-upstream`. When the Research Assistant starts an
authorization request, the broker adds the agent's UUID to the upstream request, so that the
upstream server can tell the agents apart: `example:authorization-server-modes.proxy.shared-client`

```http example="authorization-server-modes.proxy.shared-client"
GET /oauth2/authorize?client_id=550e8400-e29b-41d4-a716-446655440000&redirect_uri=https://client.example.com/cb&response_type=code&state=xyz HTTP/1.1
X-Remote-User: user@example.com

HTTP/1.1 302 Found
Location: http://127.0.0.1:…/oauth/authorize?client_id=shared-upstream&redirect_uri=https%3A%2F%2Fclient.example.com%2Fcb&response_type=code&state=xyz&x_agent_id=550e8400-e29b-41d4-a716-446655440000

<a href="…">Found</a>.
```

The token request works the same way. The agent sends its UUID, the upstream server receives
`shared-upstream`, and the agent gets the upstream server's response: `example:authorization-server-modes.proxy.upstream-client-id`

```http example="authorization-server-modes.proxy.upstream-client-id"
POST /oauth2/token HTTP/1.1
Content-Type: application/x-www-form-urlencoded

grant_type=authorization_code
&client_id=550e8400-e29b-41d4-a716-446655440000
&code=mock-auth-code-123
&redirect_uri=https%3A%2F%2Fclient.example.com%2Fcb

HTTP/1.1 200 OK
Content-Type: application/json

{
  "access_token": "…",
  "expires_in": 3600,
  "refresh_token": "mock-refresh-token",
  "token_type": "Bearer"
}
```

The access token in this response comes from the upstream server.

### Proxy mode has no place for an agent without an upstream client

A `LocalClient` is registered in a broker that runs in `proxy` mode. The broker cannot forward
its request anywhere, and `proxy` mode issues no tokens. The request fails with a clear error,
not with a redirect to the wrong place: `example:authorization-server-modes.proxy.rejects-local-agent`

```http example="authorization-server-modes.proxy.rejects-local-agent"
GET /oauth2/authorize?client_id=660e8400-e29b-41d4-a716-446655440001&redirect_uri=https://example.com/cb&response_type=code&state=xyz HTTP/1.1
X-Remote-User: user@example.com

HTTP/1.1 400 Bad Request
Content-Type: application/json

{
  "error": "unauthorized_client",
  "error_description": "client mode not supported in proxy mode"
}
```

A `client_id` in URL form fails too, because client metadata documents do not exist in `proxy`
mode. The error is `invalid_client` with the description
`URL-format client_id requires CIMD support (disabled)`. `example:authorization-server-modes.client-id.url-without-cimd`

## Local mode: the broker is the authorization server

The broker runs in `local` mode. No upstream server exists. A `LocalClient` starts an
authorization request with its UUID. The user has not granted it anything yet, so the broker
sends the browser to its own consent page for that agent: `example:authorization-server-modes.local.authorize`

```http example="authorization-server-modes.local.authorize"
GET /oauth2/authorize?client_id=660e8400-e29b-41d4-a716-446655440001&redirect_uri=https://example.com/cb&response_type=code&state=xyz HTTP/1.1
X-Remote-User: user@example.com

HTTP/1.1 302 Found
Location: http://localhost:8000/agents/660e8400-e29b-41d4-a716-446655440001?session_token=…

<a href="…">Found</a>.
```

The mirror image of the proxy rule applies. The Research Assistant has an upstream client ID, so
it is a `ProxyClient`, and `local` mode has no upstream server to forward it to: `example:authorization-server-modes.local.rejects-proxy-agent`

```http example="authorization-server-modes.local.rejects-proxy-agent"
GET /oauth2/authorize?client_id=550e8400-e29b-41d4-a716-446655440000&redirect_uri=https://example.com/cb&response_type=code&state=xyz HTTP/1.1
X-Remote-User: user@example.com

HTTP/1.1 400 Bad Request
Content-Type: application/json

{
  "error": "unauthorized_client",
  "error_description": "client mode not supported in local mode"
}
```

The token endpoint gives the same answer for the same agent. `example:authorization-server-modes.local.rejects-proxy-agent-token`

### A CIMD client is addressed by its URL only

A `CIMDClient` has a UUID like every agent, but the UUID is not its `client_id`. A request that
uses the UUID is refused: `example:authorization-server-modes.client-id.cimd-agent-by-uuid`

```http example="authorization-server-modes.client-id.cimd-agent-by-uuid"
GET /oauth2/authorize?client_id=770e8400-e29b-41d4-a716-446655440002&redirect_uri=https://cimd-fr005.test.invalid/callback&response_type=code&state=xyz HTTP/1.1
X-Remote-User: user@example.com

HTTP/1.1 400 Bad Request
Content-Type: application/json

{
  "error": "invalid_client",
  "error_description": "CIMD client must use URL-form client_id"
}
```

## Hybrid mode: both kinds of agent on one broker

A company seldom moves all agents at once. In `hybrid` mode the Research Assistant keeps its
upstream client, and a new agent without one works next to it. Both use the same endpoints.

The user consents to the Research Assistant, whose upstream client ID is `upstream-client-abc`
in this example. The broker then forwards the authorization request to the upstream server, as
in `proxy` mode. The agent later exchanges the upstream code at the broker and receives the
upstream server's token unchanged. `example:authorization-server-modes.hybrid.proxy-agent`

```http example="authorization-server-modes.hybrid.proxy-agent"
GET /oauth2/authorize?client_id=550e8400-e29b-41d4-a716-446655440000&redirect_uri=https%3A%2F%2Fexample.com%2Fcb&response_type=code&state=hybrid-proxy-state HTTP/1.1
X-Remote-User: user@example.com

HTTP/1.1 302 Found
Location: http://127.0.0.1:…/oauth/authorize?client_id=upstream-client-abc&redirect_uri=https%3A%2F%2Fexample.com%2Fcb&response_type=code&state=hybrid-proxy-state

<a href="…">Found</a>.
```

The user also consents to a `LocalClient`. For that agent the same endpoint of the same broker
issues an authorization code itself and returns it to the agent's redirect URI. The agent
exchanges the code at the broker for an access token. `example:authorization-server-modes.hybrid.local-agent`

```http example="authorization-server-modes.hybrid.local-agent"
GET /oauth2/authorize?client_id=660e8400-e29b-41d4-a716-446655440001&code_challenge=…&code_challenge_method=S256&redirect_uri=http%3A%2F%2Flocalhost%3A9999%2Fcallback&response_type=code&state=hybrid-local-state HTTP/1.1
X-Remote-User: user@example.com

HTTP/1.1 302 Found
Location: http://localhost:9999/callback?code=…&state=hybrid-local-state

<a href="…">Found</a>.
```

## Discovery describes the broker in every mode

An agent that knows only the broker's URL reads the metadata document. Every mode serves it, and
the endpoints in it are always the broker's own, never the upstream server's. In `proxy` mode: `example:authorization-server-modes.discovery.proxy`

```http example="authorization-server-modes.discovery.proxy"
GET /.well-known/oauth-authorization-server HTTP/1.1

HTTP/1.1 200 OK
Content-Type: application/json
Cache-Control: max-age=3600

{
  "issuer": "http://127.0.0.1:…",
  "authorization_endpoint": "http://127.0.0.1:…/oauth2/authorize",
  "token_endpoint": "http://127.0.0.1:…/oauth2/token",
  "response_types_supported": ["code"],
  "grant_types_supported": ["authorization_code", "refresh_token"],
  "token_endpoint_auth_methods_supported": ["client_secret_post", "client_secret_basic"],
  "jwks_uri": "http://127.0.0.1:…/oauth2/jwks.json"
}
```

In `local` mode the document also tells the agent what the broker's own token endpoint supports: `example:authorization-server-modes.discovery.local`

```http example="authorization-server-modes.discovery.local"
GET /.well-known/oauth-authorization-server HTTP/1.1

HTTP/1.1 200 OK
Content-Type: application/json
Cache-Control: max-age=3600

{
  "issuer": "http://127.0.0.1:…",
  "authorization_endpoint": "http://127.0.0.1:…/oauth2/authorize",
  "token_endpoint": "http://127.0.0.1:…/oauth2/token",
  "response_types_supported": ["code"],
  "grant_types_supported": ["authorization_code", "refresh_token"],
  "token_endpoint_auth_methods_supported": ["client_secret_post"],
  "scopes_supported": ["offline_access"],
  "jwks_uri": "http://127.0.0.1:…/oauth2/jwks.json",
  "code_challenge_methods_supported": ["S256"]
}
```

`jwks_uri` points to the broker in `proxy` mode, not to the upstream server. `example:authorization-server-modes.discovery.jwks-uri-proxy`
The `hybrid` document carries `jwks_uri` as well. `example:authorization-server-modes.discovery.jwks-uri-hybrid`

## One JWKS endpoint for every token the agent can receive

Whoever receives a token must check its signature. The broker publishes the keys for that at
`/oauth2/jwks.json` in every mode, so a verifier needs to know one URL only. What the endpoint
returns follows from who can sign a token in that mode:

- `local`: only the broker's own token-signing keys. `example:authorization-server-modes.jwks.local`
- `proxy`: only the upstream server's keys, republished by the broker. `example:authorization-server-modes.jwks.proxy`
- `hybrid`: both sets in one document. `example:authorization-server-modes.jwks.hybrid`

A partial key set is worse than none: a verifier would reject valid tokens without knowing why.
So in `hybrid` mode, when the broker cannot get the upstream keys, the endpoint answers
`503 Service Unavailable` and does not fall back to the broker's own keys alone. `example:authorization-server-modes.jwks.upstream-unavailable`

A key ID must mean one key. When a key of the broker and a key of the upstream server carry the
same `kid`, the endpoint also answers `503 Service Unavailable`. `example:authorization-server-modes.jwks.kid-conflict`

## Not yet on this page

- Grants, token claims, refresh tokens, and the management of signing keys belong to the page
  "Token issuance and refresh".
- Client credentials, client metadata documents, and redirect URIs belong to the page "Client
  registration and authentication". That includes the successful path of a `CIMDClient`.
- The broker rejects a `client_id` that is neither a UUID nor a URL, and a UUID that no agent
  holds. These cases are tested below the acceptance suite and are not shown here.
- The rest of shared upstream clients: the check of the agent claim in the upstream token, and
  how token exchange finds the agent from that claim.
- The JWKS details: the `Cache-Control` header, the absence of private key material, the
  `upstream_jwks` component of the health endpoint, and a `kid` conflict that appears after
  startup.
- The JWKS scenarios use a plain HTTP client, so this page shows no recorded JWKS exchange.
- No acceptance scenario starts the broker from a configuration file with an invalid mode block.
  The configuration rules on this page are proven by tests of the validation itself.

## Sources

The behavior on this page was decided in `specs/009-oauth2-auth-server/spec.md`,
`specs/021-multi-agent-clientid/spec.md`, `specs/025-oauth2-server/spec.md`,
`specs/030-hybrid-oauth-modes/spec.md`, `specs/032-aggregated-jwks/spec.md`, and
`adrs/014-oauth2-server-mode.md`. The contract is `api/enduser/openapi.yaml`. The configuration
reference is `docs/configuration.md`. The scenarios are in
`tests/e2e/mode_boundary_e2e_test.go`, `tests/e2e/mode_configuration_e2e_test.go`,
`tests/e2e/hybrid_oauth_modes_e2e_test.go`, `tests/e2e/multi_agent_client_test.go`,
`tests/e2e/oauth2_metadata_test.go`, `tests/e2e/aggregated_jwks_test.go`, and
`internal/ports/config_test.go`.
