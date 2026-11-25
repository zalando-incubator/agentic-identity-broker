---
title: "Gateway token exchange"
description: Living specification of how the ExtProc service beside a gateway replaces the caller's token with the right third-party token on each request. Every example on this page is checked against the acceptance suite.
---

# Gateway token exchange

An agent must never hold a provider credential. The **ExtProc service** runs beside an
Envoy-compatible gateway and, on each request, replaces the caller's token with the third-party
token for the resource behind the gateway. This page describes that behavior in the order a
platform operator meets it: input, exchange, cache, outage, and recovery.

:::note How to read this page
This is a living specification. A badge such as "tested" after a sentence links to the acceptance
scenario that proves it. Each HTTP exchange shown here is compared with what that scenario really
sent and received. `just living-docs` runs the comparison and fails when the page and the broker
disagree. UUIDs and timestamps are synthetic. The check compares timestamps by shape and UUIDs by
which of them are equal.
:::

The ExtProc suite records outcomes, not HTTP exchanges. This page therefore has badges and no
compared exchanges. The code blocks are illustrations, copied from test fixtures and assertions.
The check does not compare them.

## The cast

A platform operator runs agentgateway in front of an MCP server at `https://mcp.example.com/mcp`.
That server calls the provider *GitHub* for the user. The agent *Research Assistant* calls the
gateway for Alice (`alice@example.com`) and sends a bearer token that identifies Alice to the
broker. Alice has granted the agent the permission set `github-read`.

The operator runs the ExtProc service next to the gateway. The gateway validates the caller's
token, then asks the ExtProc service what to do with the request. The ExtProc service asks the
broker for Alice's GitHub token for this resource and puts it on the request. The broker decides
whether the exchange is allowed; the page "Token exchange" describes those rules.

Through a real agentgateway, the MCP server receives the exchanged token and the broker receives
the caller's token together with the configured resource. `example:gateway-token-exchange.gateway.end-to-end`
That scenario needs Docker.

## The gateway supplies both inputs

The ExtProc service needs two values: the **subject token** (the caller's credential) and the
**ResourceURI** (the protected resource that the request is for). It reads both from gateway
dynamic metadata in the namespace `aib.tokenexchange`, in the string fields `subject_token` and
`resource_uri`. The operator configures the gateway to fill them:

```yaml
# Illustration: agentgateway route policy. Not compared by the check.
extProc:
  host: "extproc-token-exchange:50051"
  failureMode: failClosed
  metadataContext:
    aib.tokenexchange:
      subject_token: "jwt.rawToken.unredacted()"
      resource_uri: "'https://mcp.example.com/mcp'"
```

The ExtProc service never reads the `Authorization` header or the request path as input. Suppose
a request carries one token and path in its HTTP headers and another token and resource in the
metadata. The broker receives the metadata values only. `example:gateway-token-exchange.input.metadata-only`

The reason is trust. The gateway has validated the token that it writes into metadata. A raw
header is whatever the caller sent.

A request that has a bearer token and a path but no metadata is rejected. It is not passed
through, and the broker is not called. `example:gateway-token-exchange.input.raw-request-rejected`

### Missing or invalid metadata has two fixed answers

The ExtProc service checks the metadata before it calls the broker. Both rejections are HTTP 503
with a JSON body, and neither sets an `Authorization` header on the request.

The subject token must be a non-empty string without the `Bearer ` scheme prefix. A missing,
empty, prefixed, or non-string value is rejected. `example:gateway-token-exchange.input.invalid-subject-token`

```text
HTTP/1.1 503 Service Unavailable
content-type: application/json

{"error":"invalid_subject_token","error_description":"subject token metadata is missing or invalid"}
```

The resource must be a non-empty string. A missing, empty, non-string, or relative value is
rejected. `example:gateway-token-exchange.input.invalid-resource`

```text
HTTP/1.1 503 Service Unavailable
content-type: application/json

{"error":"invalid_resource","error_description":"resource metadata is missing or invalid"}
```

The resource must also be an absolute HTTP or HTTPS URL with a host. A relative path, another
scheme, and `https:///missing-host` all get the `invalid_resource` answer. `example:gateway-token-exchange.input.resource-absolute-url`

The subject token is checked first. When both values are invalid, the answer is
`invalid_subject_token`. `example:gateway-token-exchange.input.subject-checked-first`

The status is 503 and not 401 because these cases are gateway configuration faults. The caller
cannot fix them.

## The exchange

### The ExtProc service authenticates as itself

The ExtProc service is an OAuth2 client of its own. It obtains a token with the
`client_credentials` grant from the configured issuer. With no scopes configured, it asks for the
scope `openid`. `example:gateway-token-exchange.broker-auth.default-scope`

It sends the ID token from that response to the broker as its client assertion. `example:gateway-token-exchange.broker-auth.id-token-assertion`

```yaml
# Illustration: ExtProc service configuration. Not compared by the check.
oauth2:
  token_endpoint: "https://broker.example.com/oauth2/token"
  issuer: "https://issuer.example.com"
  client_id: "extproc-gateway"
  client_secret: "${EXTPROC_OAUTH2_CLIENT_SECRET}"
  client_assertion_type: "id_token"
  exchange_timeout: "5s"
```

### One RFC 8693 request for each subject token and resource

The request to the broker's token endpoint is a form with these parameters: `example:gateway-token-exchange.exchange.rfc8693-request`

```text
grant_type=urn:ietf:params:oauth:grant-type:token-exchange
&subject_token=…
&subject_token_type=urn:ietf:params:oauth:token-type:access_token
&resource=https://mcp.example.com/mcp
&client_assertion_type=urn:ietf:params:oauth:client-assertion-type:jwt-bearer
&client_assertion=…
```

### On success, the request continues with the third-party token

The ExtProc service tells the gateway to set the `Authorization` header to `Bearer` followed by
the exchanged token. The request then continues to the MCP server. `example:gateway-token-exchange.exchange.authorization-replaced`

### On failure, no credential is forwarded

Suppose the broker refuses the exchange with `403 access_denied`, for example because Alice has
no grant for the agent. The ExtProc service stops the request with HTTP 500 and a fixed body. It
sets no `Authorization` header, so the caller's token never reaches the MCP server. `example:gateway-token-exchange.exchange.failure-rejected`

```text
HTTP/1.1 500 Internal Server Error
content-type: application/json

{"error":"token_exchange_failed","error_description":"token exchange request failed"}
```

The body is the same for every ordinary failure. It does not repeat the broker's error, so a
caller learns nothing about grants or sessions from it.

## The token cache

The ExtProc service keeps exchanged tokens in memory. The cache key is the pair of subject token
and resource.

A second request with the same subject token and resource gets the cached token. The broker is
not called again. `example:gateway-token-exchange.cache.repeat-request`

Ten requests that arrive together for a pair that is not in the cache cause one exchange. All ten
continue with the same token. `example:gateway-token-exchange.cache.concurrent-requests`

### Tokens leave the cache before they expire

A cached token must still be valid when it reaches the provider. The ExtProc service therefore
stops serving a token some time before the expiry that the broker reported, and exchanges again. `example:gateway-token-exchange.cache.expiry-margin`

| Reported lifetime (`expires_in`) | Served from cache for |
|---|---|
| 60 seconds | 50 seconds |
| 200 seconds | 180 seconds |
| 1 second | half a second |

The margin is 10 seconds or 10 percent of the lifetime, whichever is larger, but never more than
half the lifetime.

Two settings bound the cache:

```yaml
# Illustration: ExtProc service configuration with its defaults. Not compared by the check.
cache:
  default_ttl: "5m"
  max_ttl: "1h"
```

- `cache.max_ttl` is the maximum lifetime of any entry. A token that the broker reports as valid
  for a week is exchanged again after `max_ttl`. `example:gateway-token-exchange.cache.max-lifetime`
- `cache.default_ttl` applies when the broker's response has no `expires_in`. Such a token is
  still cached. `example:gateway-token-exchange.cache.unknown-expiry`

### A short broker outage does not stop traffic

Suppose a token with a 60-second lifetime is in the cache and the broker starts to answer 503.
After the 50 seconds of normal cache use, each request asks the broker again. While the broker
keeps failing, the request continues with the old token. Five seconds after the reported expiry,
the fallback ends and the failure reaches the caller. `example:gateway-token-exchange.cache.outage-fallback`

The fallback is for outages only. When the broker answers the renewal with a clear refusal such
as `403 access_denied`, the old token is not used and the request fails. `example:gateway-token-exchange.cache.rejection-not-masked`

## The circuit breaker

A broker that is down should not receive a retry for every gateway request. The ExtProc service
counts consecutive server-side failures of the exchange call.

```yaml
# Illustration: ExtProc service configuration with its defaults. Not compared by the check.
circuit_breaker:
  enabled: true
  max_failures: 5
  reset_timeout: "30s"
```

After `max_failures` consecutive failures, the circuit opens. Further requests fail at once, and
the broker receives no call. `example:gateway-token-exchange.circuit-breaker.opens`

While the circuit is open, the caller gets HTTP 503 with this body and no `Authorization` header: `example:gateway-token-exchange.circuit-breaker.response`

```text
HTTP/1.1 503 Service Unavailable
content-type: application/json

{"error":"service_unavailable","error_description":"circuit breaker is open"}
```

Three more rules complete the picture:

- A request whose token is still in the cache succeeds while the circuit is open. `example:gateway-token-exchange.circuit-breaker.cached-token-served`
- After `reset_timeout`, the ExtProc service tries the broker again. One success closes the
  circuit. `example:gateway-token-exchange.circuit-breaker.recovers`
- Refusals do not count. Any number of `400 invalid_grant` answers leaves the circuit closed,
  because the broker is healthy when it refuses. `example:gateway-token-exchange.circuit-breaker.client-errors-ignored`

One more 503 exists. If the ExtProc service's own client assertion has expired and cannot be
renewed, the body says `client assertion expired`. `example:gateway-token-exchange.errors.assertion-expired`

## When the user must sign in to the provider again

Alice's GitHub session can end: the provider revokes it, or its refresh token stops working. The
broker then refuses the exchange and adds an `error_uri` to its error. That URL is the recovery
link: it starts a new sign-in at GitHub for Alice.

The ExtProc service turns this refusal into a challenge that an agent can act on. The response
is HTTP 403. Its `WWW-Authenticate` header carries `insufficient_scope`, the broker's description,
and the recovery link. Its body is a JSON-RPC URL-elicitation error with the same link, a null
`id`, and a fresh elicitation ID. No `Authorization` header is set. `example:gateway-token-exchange.reauth.challenge`

```text
HTTP/1.1 403 Forbidden
content-type: application/json
www-authenticate: Bearer error="insufficient_scope", error_description="mock exchange error", error_uri="https://broker.example.com/api/third-party/550e8400-e29b-41d4-a716-446655440000/oauth2/authorize?redirect_uri=https%3A%2F%2Fbroker.example.com%2Fsessions"
```

```json
{
  "jsonrpc": "2.0",
  "id": null,
  "error": {
    "code": -32042,
    "message": "mock exchange error",
    "data": {
      "elicitations": [
        {
          "mode": "url",
          "elicitationId": "770e8400-e29b-41d4-a716-446655440001",
          "url": "https://broker.example.com/api/third-party/550e8400-e29b-41d4-a716-446655440000/oauth2/authorize?redirect_uri=https%3A%2F%2Fbroker.example.com%2Fsessions",
          "message": "mock exchange error"
        }
      ]
    }
  }
}
```

The description is the text that the broker sent; the test broker sends `mock exchange error`.

The tested request is an A2A call, not an MCP call. The challenge does not depend on the
protocol of the request.

A server error is not a reason to send Alice to GitHub. When the broker answers 500, the caller
gets the ordinary HTTP 500 failure without a challenge, even if the error has an `error_uri`. `example:gateway-token-exchange.reauth.server-error-no-challenge`

Two scenarios prove the challenge through a real agentgateway. Both need Docker. The gateway
passes the 403, the header, and the body to the client unchanged. `example:gateway-token-exchange.reauth.through-gateway`
An MCP client that supports step-up authorization follows the challenge, obtains a new token, and
its retried call succeeds. `example:gateway-token-exchange.reauth.step-up-retry`

## Related capabilities

The same ExtProc service can ask an OPA policy whether a request is allowed; see "Gateway policy
authorization". It can hold a tool call until a person approves it; see "Tool approvals". Its
traces, metrics, and logs are described in "Observability and audit".

## Not yet on this page

- Startup: configuration loading, validation errors, TLS settings for the broker connection, and
  the `access_token` assertion type.
- Renewal of the client assertion in the background.
- The five-second pause between broker calls after a re-authentication refusal.
- Removal of expired cache entries, and the absence of a size limit for the cache.
- A disabled circuit breaker, and a failed probe that opens the circuit again.
- Timeouts and network errors of the exchange call. Unit tests cover them; no scenario of the
  ExtProc suite does.
- The exact error code `-32042` and the rule that the challenge omits a description or link with
  characters that a header cannot carry.
- How the broker builds the recovery link. That belongs to "Third-party sessions".

## Sources

The behavior on this page was decided in `specs/015-extproc-token-exchange/spec.md` (cache and
circuit breaker), `specs/043-extproc-metadata-input/spec.md` (input and rejections), and
`specs/023-extproc-mcp-elicitation/spec.md` (challenge). The decisions are
`adrs/011-extproc-standalone-binary.md`, `adrs/012-extproc-in-memory-token-cache.md`,
`adrs/036-extproc-metadata-token-exchange-input.md`, and
`adrs/044-service-specific-session-recovery-url.md`. The scenarios are in
`tests/e2e/extproc/token_exchange_test.go`, `tests/e2e/extproc/metadata_input_test.go`,
`tests/e2e/extproc/reauth_challenge_test.go`, `tests/e2e/extproc/agentgateway_e2e_test.go`, and
`tests/e2e/extproc/agentgateway_metadata_e2e_test.go`. Cache timing, the circuit breaker, and the
request to the broker are proven by the tests in `internal/extproc/server/`.
