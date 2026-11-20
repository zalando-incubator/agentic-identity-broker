---
title: "Client registration and authentication"
description: Living specification of how an agent identifies itself to the broker, which redirect URIs the broker accepts, and how a client with a metadata document authenticates at the token endpoint. Every example on this page is checked against the acceptance suite.
---

# Client registration and authentication

An **Agent** that asks the broker for a token must first say who it is. This page describes the
two ways an agent can identify itself, where the broker agrees to send the user back, and how the
agent proves its identity at the token endpoint.

:::note How to read this page
This is a living specification. A badge such as "tested" after a sentence links to the acceptance
scenario that proves it. Each HTTP exchange shown here is compared with what that scenario really
sent and received. `just living-docs` runs the comparison and fails when the page and the broker
disagree. UUIDs and timestamps are synthetic. The check compares timestamps by shape and UUIDs by
which of them are equal.
:::

## The cast

An administrator, `admin@example.com`, registers agents through the admin API. The *Research
Assistant* is one of them. The user in the examples is signed in as `user@example.com`.

The broker issues its own tokens here, so it runs in `local` or `hybrid` mode. The page
"Authorization server modes" describes the modes.

An agent identifies itself in one of two ways. The administrator decides which one when
registering the agent:

| The agent sends as `client_id` | The administrator registers | Redirect URIs come from | Token endpoint authentication |
|---|---|---|---|
| its agent UUID | the agent, and optionally credentials | the agent's `redirect_uris` | a client secret, or none with PKCE |
| the HTTPS URL of its **Client ID Metadata Document** (CIMD) | that URL, or a pattern for it, in the agent's `client_uris` | the document | none with PKCE, or a signed assertion |

The `client_id` of a request is always one of these two broker identities. An agent that is also
a client of an upstream authorization server still sends its UUID, not its upstream client ID. `example:client-registration-and-authentication.client-id.agent-uuid`

## Identify by agent UUID

### The broker issues the credentials

The administrator asks the broker for credentials for the Research Assistant. The request has no
body. The response shows the secret once: `example:client-registration-and-authentication.credentials.rotate`

```http example="client-registration-and-authentication.credentials.rotate"
POST /api/agents/550e8400-e29b-41d4-a716-446655440000/client-credentials HTTP/1.1
X-Remote-User: admin@example.com
Content-Type: application/json

HTTP/1.1 201 Created

{
  "client_id": "550e8400-e29b-41d4-a716-446655440000",
  "client_secret": "…",
  "created_at": "2026-10-30T09:00:00Z"
}
```

The client ID is the agent's UUID. The broker never shows the secret again: a later `GET` of the
same path returns only `client_id` and `created_at`.

The same call on an agent that already has credentials rotates them. It answers `200 OK` with a
new secret and the time at which the broker invalidated the previous one:

```http example="client-registration-and-authentication.credentials.rotate"
POST /api/agents/550e8400-e29b-41d4-a716-446655440000/client-credentials HTTP/1.1
X-Remote-User: admin@example.com
Content-Type: application/json

HTTP/1.1 200 OK

{
  "client_id": "550e8400-e29b-41d4-a716-446655440000",
  "client_secret": "…",
  "created_at": "2026-10-30T09:00:00Z",
  "previous_invalidated_at": "2026-10-31T14:00:00Z"
}
```

A web page could try to trigger a rotation in the administrator's browser. The broker refuses a
rotation request that comes from another origin or that does not declare a JSON body, and the
stored credential stays as it was. `example:client-registration-and-authentication.credentials.rotate`

### The administrator lists the redirect URIs

An agent's `redirect_uris` list says where the broker may send the user after authorization. The
broker accepts three kinds of entry:

- an `https` URI, such as `https://agent.example.com/callback`;
- an `http` URI on a loopback host, such as `http://127.0.0.1/callback`;
- a URI with a private-use scheme, such as `com.example.app:/oauth2redirect/example-provider`,
  for a native app.

The broker stores the list as written and returns it in the same order. `example:client-registration-and-authentication.private-use.register`
It refuses any other kind of URI, such as `javascript://oauth/callback`, with `400` and a message
that names the position of the bad entry, here `redirect_uris[0]`. It refuses in the same way a
private-use URI that is not in its plainest form: one with a fragment, an upper-case scheme, user
information, a `..` segment, or a percent-encoded character. `example:client-registration-and-authentication.private-use.unsafe`

### The broker compares the redirect URI of each request with the list

Two rules decide whether a requested `redirect_uri` is registered:

- **A loopback URI matches on any port.** A native app listens on a port that the operating
  system picks at run time. An agent registered with `http://localhost:3000/callback` can ask
  for `http://localhost:9999/callback`, and the broker delivers the code there. `example:client-registration-and-authentication.redirect.loopback-uuid-agent`
- **Every other URI must match exactly.** For an agent registered with
  `com.example.app:/oauth2redirect/example-provider`, a different path, an upper-case scheme, a
  double slash, a trailing slash, and an added query are all refused. The broker answers `400`
  with `invalid_redirect_uri` itself. It does not redirect to an address that it does not trust. `example:client-registration-and-authentication.private-use.look-alike`

## Identify by a metadata document URL

Some agents are distributed software that many companies install. Their vendor publishes one JSON
document at an HTTPS URL, and the agent sends that URL as its `client_id`. The broker reads the
document to learn the client's name and redirect URIs. The operator turns this on with
`oauth2_authorization_server.cimd.enabled: true`.

A URL alone does not make a client welcome. The administrator must first add the URL to the
`client_uris` of an agent. The agent then stands for that client: grants and tokens refer to the
agent's UUID.

### A registered URL leads to the consent screen

The Research Assistant has the client URI `https://cimd-e2e.test.invalid/client`. The vendor
serves this document at that URL:

```json
{
  "client_id": "https://cimd-e2e.test.invalid/client",
  "client_name": "Test CIMD Agent",
  "redirect_uris": ["https://cimd-e2e.test.invalid/callback"]
}
```

The agent starts authorization with the URL. The broker finds the agent, fetches and checks the
document, and sends the user to the consent screen of that agent: `example:client-registration-and-authentication.cimd.authorize`

```http example="client-registration-and-authentication.cimd.authorize"
GET /oauth2/authorize?client_id=https://cimd-e2e.test.invalid/client&redirect_uri=https://cimd-e2e.test.invalid/callback&response_type=code&state=xyz HTTP/1.1
X-Remote-User: user@example.com

HTTP/1.1 302 Found
Location: http://localhost:8000/agents/550e8400-e29b-41d4-a716-446655440000?session_token=…

<a href="…">Found</a>.
```

The `session_token` carries the authorization request in sealed form. The page "Consent and
delegation" describes what the screen shows for such a client.

### An unregistered URL is refused before any fetch

A URL that no agent lists is refused. The broker does not fetch anything for it, so a stranger
cannot make the broker call arbitrary hosts: `example:client-registration-and-authentication.cimd.unregistered-url`

```http example="client-registration-and-authentication.cimd.unregistered-url"
GET /oauth2/authorize?client_id=https://unregistered.example.com/client&redirect_uri=https://unregistered.example.com/cb&response_type=code&state=xyz HTTP/1.1
X-Remote-User: user@example.com

HTTP/1.1 400 Bad Request

{
  "error": "invalid_client",
  "error_description": "Client not registered"
}
```

### One pattern can cover many hosted clients

Some vendors give each installation its own document URL. The administrator can register a
pattern in which `*` stands for exactly one path segment. Suppose the Research Assistant has the
client URI `https://cimd-wildcard.test.invalid/oauth/codex/*/client.json`. A concrete URL with
one segment in that place, such as `…/oauth/codex/dIwd44EtAHp-/client.json`, resolves to the
agent and leads to its consent screen. `example:client-registration-and-authentication.wildcard.match`

Three rules keep a pattern from matching more than the administrator meant:

- `*` never spans two segments. `…/oauth/codex/tenant/nested/client.json` gets the same
  `Client not registered` answer as an unknown URL. `example:client-registration-and-authentication.wildcard.extra-segment`
- An exact client URI wins over a pattern. When one agent lists the exact URL and another agent
  lists a pattern that matches it, the request resolves to the agent with the exact URL. `example:client-registration-and-authentication.wildcard.exact-first`
- The broker does not guess between agents. When the patterns of two different agents match the
  same URL, the request is refused with `invalid_client`. `example:client-registration-and-authentication.wildcard.ambiguous`

### The document must be consistent with the request

The broker refuses the client with `invalid_client` when the `client_id` inside the document
differs from the URL that served it. `example:client-registration-and-authentication.cimd.document-id-mismatch`
It also refuses a document that asks for a shared-secret method such as `client_secret_post`: a
client that anyone can install cannot keep a shared secret. `example:client-registration-and-authentication.cimd.shared-secret-method`

For a client with a metadata document, the redirect URIs come from the document. A requested
`redirect_uri` that the document does not list is refused with `400` and `invalid_request`, and
the broker does not redirect. `example:client-registration-and-authentication.cimd.redirect-not-in-document`

The two matching rules of the previous section apply here as well:

- A document that lists `http://localhost:3000/callback` lets the client use
  `http://localhost:9999/callback`. `example:client-registration-and-authentication.redirect.loopback-any-port`
- Only the port is free. With `http://localhost/callback` in the document, the broker refuses
  `http://localhost:52341/other`. `example:client-registration-and-authentication.redirect.loopback-path`
- A URI that is not loopback keeps its port. With `https://…:443/callback` in the document, the
  broker refuses the same host and path on port 9000. `example:client-registration-and-authentication.redirect.https-exact-port`

### A private-use redirect URI needs two approvals

Any app on a device can claim a private-use scheme, so a remote document alone cannot authorize
one. The broker accepts a private-use `redirect_uri` for such a client only when the document
lists it and the administrator has also put the identical URI in the agent's `redirect_uris`. `example:client-registration-and-authentication.private-use.cimd-accepted`
The client can then complete the flow: the broker delivers the code to the private-use URI, and
the client exchanges it with its PKCE verifier and no credential. `example:client-registration-and-authentication.private-use.cimd-flow`

- When the document lists a private-use URI that the agent does not allow, the broker rejects the
  whole document. Even a request for the loopback URI of that document fails with
  `invalid_client`. `example:client-registration-and-authentication.private-use.cimd-not-allow-listed`
- When only the agent lists the URI and the document does not, the request fails with
  `invalid_request`, as for any redirect URI that is missing from the document. `example:client-registration-and-authentication.private-use.cimd-not-in-document`

## The broker protects itself when it fetches a document

Fetching a URL that a client supplies is a risk: the URL could point into the operator's own
network. Pre-registration is the first defense. The fetch itself has more.

The broker refuses to fetch from an address that is not public. Even when an administrator
registers such a URL on an agent, authorization fails: `example:client-registration-and-authentication.fetch.private-address`

```http example="client-registration-and-authentication.fetch.private-address"
GET /oauth2/authorize?client_id=https://10.0.0.1/client&redirect_uri=https://10.0.0.1/callback&response_type=code&state=xyz HTTP/1.1
X-Remote-User: user@example.com

HTTP/1.1 400 Bad Request

{
  "error": "invalid_client",
  "error_description": "CIMD document validation failed"
}
```

The fetch is also bounded in size and time:

- A document larger than `oauth2_authorization_server.cimd.max_response_bytes` gets the same
  `invalid_client` answer. The scenario uses a limit of 5120 bytes. `example:client-registration-and-authentication.fetch.size-limit`
- A host that does not answer within `oauth2_authorization_server.cimd.fetch_timeout` makes the
  request fail with `500` and `server_error`: the fault is in a dependency, not in the client. `example:client-registration-and-authentication.fetch.timeout`

The broker keeps a valid document in a cache. A second authorization request for the same client
within the cache lifetime does not fetch the document again. `example:client-registration-and-authentication.fetch.cache-hit`
A failed fetch is not kept: when the document host answers with an error, the next request tries
again. `example:client-registration-and-authentication.fetch.failure-not-cached`

## A client with a metadata document authenticates at the token endpoint

The document also says how the client authenticates when it exchanges a code or refreshes a
token. It names one of two methods in `token_endpoint_auth_method`. With client metadata
documents enabled, the broker's discovery metadata lists what it accepts: `example:client-registration-and-authentication.assertion.discovery`

```http example="client-registration-and-authentication.assertion.discovery"
GET /.well-known/oauth-authorization-server HTTP/1.1

HTTP/1.1 200 OK

{
  "token_endpoint_auth_methods_supported": ["none", "client_secret_post", "private_key_jwt"],
  "token_endpoint_auth_signing_alg_values_supported": ["RS256", "ES256"],
  "client_id_metadata_document_supported": true,
  "…": "…"
}
```

`client_secret_post` is for agents that identify by UUID. The other two are for clients with a
metadata document.

### `none`: a public client

A client whose document says `none` has no credential. PKCE protects its
code. It exchanges the code and later refreshes its token with its `client_id` alone. `example:client-registration-and-authentication.public-client.token`
Such a client must not send an assertion. The broker rejects a token request that carries one
with `401` and `invalid_client`, and the same code still works without it. `example:client-registration-and-authentication.assertion.public-client-sends-one`

### `private_key_jwt`: a signed assertion

A client that can keep a private key publishes the public key and declares the method in its
document:

```json
{
  "client_id": "https://private-key-jwt-e2e.test.invalid/client",
  "client_name": "Private Key JWT E2E Agent",
  "redirect_uris": ["https://private-key-jwt-e2e.test.invalid/callback"],
  "grant_types": ["authorization_code", "refresh_token"],
  "response_types": ["code"],
  "token_endpoint_auth_method": "private_key_jwt",
  "token_endpoint_auth_signing_alg": "RS256",
  "jwks_uri": "https://private-key-jwt-e2e.test.invalid/jwks.json"
}
```

With each token request the client sends a short-lived JWT that it signs with that key, in the
form fields `client_assertion` and `client_assertion_type`. The JWT names the client as issuer
and subject, names the broker's token endpoint as audience, and carries a key ID (`kid`) and a
unique ID (`jti`). The broker verifies it with the keys at `jwks_uri` and then issues tokens. `example:client-registration-and-authentication.assertion.code-exchange`
A refresh needs a new assertion. `example:client-registration-and-authentication.assertion.refresh`

The declared method is a promise that the broker enforces:

- **No assertion, no token.** A code exchange or refresh without an assertion fails with `401`
  and `invalid_client`. `example:client-registration-and-authentication.assertion.required`
- **A wrong assertion fails without detail.** The answer is the same `401` for a bad signature,
  a wrong issuer, subject, or audience, an expired assertion, a lifetime above five minutes, a
  missing `kid` or `jti`, an algorithm other than the declared one, and an unsigned JWT. The
  attempt does not use up the code: the client can repeat the request with a valid assertion. `example:client-registration-and-authentication.assertion.invalid`
- **The method cannot be downgraded.** When the document later changes to `none`, a refresh
  token that was issued to the authenticated client still needs an assertion. `example:client-registration-and-authentication.assertion.document-downgrade`
- **An assertion works once.** A second token request with the same assertion fails with `401`. `example:client-registration-and-authentication.assertion.replay`
  This holds across broker replicas that share a PostgreSQL database: of two simultaneous uses,
  one succeeds. `example:client-registration-and-authentication.assertion.replay-two-brokers`

A client can rotate its key without telling the broker. When an assertion names a `kid` that the
broker has not seen, the broker reads the key set again and accepts the new key. `example:client-registration-and-authentication.assertion.key-rotation`

## Not yet on this page

- The exchanges of the token endpoint for a client with a metadata document. The scenarios that
  prove the token rules above pass, but their HTTP traffic is not recorded, so the page cites
  them without showing requests.
- Registering `client_uris` through the admin API, including the refusal of a malformed pattern
  and of a client URI that another agent already holds.
- The other checks on a document: an empty `redirect_uris` list, a `client_secret` member, a
  redirect URI on another origin, and a blocked client name.
- The other limits on fetching: loopback and link-local addresses, plain `http` URLs, dot
  segments, the cache lifetime bounds, the same protections for `jwks_uri`, the limit on how
  often an unknown `kid` causes a new read of the key set, and the refusal of weak keys.
- Allowed scopes, token contents, and refresh ("Token issuance and refresh"); the consent screen
  for a client with a metadata document ("Consent and delegation"); and how the broker
  identifies itself to a third-party provider, which is a different capability.

## Sources

The behavior on this page was decided in `specs/025-oauth2-server/spec.md`,
`specs/028-cimd-support/spec.md`, `specs/028b-portless-registration/spec.md`,
`specs/028c-cimd-wildcard-client-uris/spec.md`, `specs/051-private-key-jwt-auth/spec.md`, and
`specs/021-multi-agent-clientid/spec.md`, and in
`adrs/042-agent-allowlisted-private-use-redirect-uris.md`,
`adrs/043-inbound-cimd-private-key-jwt.md`, and `adrs/015-cimd-fetcher-architecture.md`. The
contracts are `api/admin/openapi.yaml` and `api/enduser/openapi.yaml`.

The scenarios are in `tests/e2e/`: `oauth2_client_credentials_e2e_test.go`,
`oauth2_private_use_redirect_test.go`, `portless_opaque_test.go`, `multi_agent_client_test.go`,
`cimd_authorization_test.go`, `cimd_wildcard_client_uri_test.go`, `cimd_redirect_uri_test.go`,
`cimd_private_use_redirect_test.go`, `cimd_ssrf_test.go`, `cimd_caching_test.go`, and the
`cimd_private_key_jwt_…_test.go` files.
