---
title: "Token exchange"
description: Living specification of how a gateway swaps a user's token for that user's token at a third-party service. Every example on this page is checked against the acceptance suite.
---

# Token exchange

An agent calls a third-party API on behalf of a user. The agent never holds the user's third-party
token. A gateway in front of the API asks the broker for that token at the moment of the call. This
page describes that request and each answer the gateway can get, in the order of the broker's
checks.

:::note How to read this page
This is a living specification. A badge such as "tested" after a sentence links to the acceptance
scenario that proves it. Each HTTP exchange shown here is compared with what that scenario really
sent and received. `just living-docs` runs the comparison and fails when the page and the broker
disagree. UUIDs and timestamps are synthetic. The check compares timestamps by shape and UUIDs by
which of them are equal.
:::

## The cast

Alice uses the agent *Research Assistant*. In the acceptance suite her **Principal** is
`user@example.com`. She gave the agent a **UserGrant** that covers the provider *GitHub*, and she
connected her GitHub account, so the broker holds a **UserSession** with her GitHub tokens.

GitHub is registered as a **ThirdpartyOAuth2Service** with the protected resource
`https://api.github.com`.

A **Privileged Client**, here an API gateway, sits between the agent and GitHub. The agent sends
its request with a token that names Alice in `sub` and the agent's UUID in `azp`. This token is
the *subject token*. The gateway also holds a signed JWT of its own, the **ClientAssertion**. The
broker runs at `http://localhost:8000` in these examples.

## One request, one third-party token

The gateway sends the subject token, its ClientAssertion, and the resource that the agent wants to
reach. The broker answers with Alice's GitHub access token: `example:token-exchange.exchange.success`

```http example="token-exchange.exchange.success"
POST /oauth2/token HTTP/1.1
Content-Type: application/x-www-form-urlencoded

grant_type=urn%3Aietf%3Aparams%3Aoauth%3Agrant-type%3Atoken-exchange
&subject_token=…
&subject_token_type=urn%3Aietf%3Aparams%3Aoauth%3Atoken-type%3Aaccess_token
&client_assertion_type=urn%3Aietf%3Aparams%3Aoauth%3Aclient-assertion-type%3Ajwt-bearer
&client_assertion=…
&resource=https%3A%2F%2Fapi.github.com

HTTP/1.1 200 OK
Content-Type: application/json
Cache-Control: no-store

{
  "access_token": "github-token-xyz",
  "token_type": "Bearer",
  "issued_token_type": "urn:ietf:params:oauth:token-type:access_token",
  "scope": "repo user",
  "granted_permission_sets": {
    "770e8400-e29b-41d4-a716-446655440001": [
      "660e8400-e29b-41d4-a716-446655440011",
      "660e8400-e29b-41d4-a716-446655440010"
    ]
  },
  "principal": "user@example.com",
  "agent_id": "550e8400-e29b-41d4-a716-446655440000",
  "…": "…"
}
```

Four details are worth noticing:

- `access_token` is the token that GitHub issued to Alice. The broker does not mint it.
- The response also has `expires_in`, the remaining lifetime of that token in seconds.
- `granted_permission_sets` repeats what Alice granted: each permission set, with the services
  she included in it. Here it is one permission set with two services, and GitHub is the second.
- `principal` and `agent_id` tell the gateway whom the broker verified. A gateway needs them to
  apply its own rules to the call.

These eight members are the complete success response. `example:token-exchange.exchange.permission-sets`

The broker returns `agent_id` as the agent's UUID in its canonical lower-case form, also when the
subject token carries the same UUID in upper case. `example:token-exchange.exchange.identity`

## The resource selects the service

The gateway does not name a service. It names the resource, and the broker finds the service that
registered it. A resource that no service registered gives no token: `example:token-exchange.resource.unknown`

```http example="token-exchange.resource.unknown"
POST /oauth2/token HTTP/1.1
Content-Type: application/x-www-form-urlencoded

grant_type=urn%3Aietf%3Aparams%3Aoauth%3Agrant-type%3Atoken-exchange
&subject_token=…
&subject_token_type=urn%3Aietf%3Aparams%3Aoauth%3Atoken-type%3Aaccess_token
&client_assertion_type=urn%3Aietf%3Aparams%3Aoauth%3Aclient-assertion-type%3Ajwt-bearer
&client_assertion=…
&resource=https%3A%2F%2Fapi.nonexistent.com

HTTP/1.1 400 Bad Request

{
  "error": "invalid_target",
  "error_description": "token exchange target is invalid or unavailable"
}
```

One exchange returns one token. A request that names two different resources gets the same
`invalid_target` answer, even when one of them is registered. `example:token-exchange.resource.distinct`

## The gateway and the subject token must both be valid

The broker first checks who asks. A ClientAssertion whose signature does not verify ends the
exchange: `example:token-exchange.gateway.invalid-assertion`

```http example="token-exchange.gateway.invalid-assertion"
POST /oauth2/token HTTP/1.1
Content-Type: application/x-www-form-urlencoded

grant_type=urn%3Aietf%3Aparams%3Aoauth%3Agrant-type%3Atoken-exchange
&subject_token=…
&subject_token_type=urn%3Aietf%3Aparams%3Aoauth%3Atoken-type%3Aaccess_token
&client_assertion_type=urn%3Aietf%3Aparams%3Aoauth%3Aclient-assertion-type%3Ajwt-bearer
&client_assertion=…
&resource=https%3A%2F%2Fapi.github.com

HTTP/1.1 401 Unauthorized

{
  "error": "invalid_client",
  "error_description": "client authentication failed"
}
```

A valid gateway with a subject token that is not a valid JWT gets a different answer. The request
itself is well formed, but the credential that it presents for Alice is not acceptable: `example:token-exchange.subject.invalid`

```http example="token-exchange.subject.invalid"
POST /oauth2/token HTTP/1.1
Content-Type: application/x-www-form-urlencoded

grant_type=urn%3Aietf%3Aparams%3Aoauth%3Agrant-type%3Atoken-exchange
&subject_token=not.a.valid.jwt.structure
&subject_token_type=urn%3Aietf%3Aparams%3Aoauth%3Atoken-type%3Aaccess_token
&client_assertion_type=urn%3Aietf%3Aparams%3Aoauth%3Aclient-assertion-type%3Ajwt-bearer
&client_assertion=…
&resource=https%3A%2F%2Fapi.github.com

HTTP/1.1 400 Bad Request

{
  "error": "invalid_grant",
  "error_description": "token exchange grant is invalid or unavailable"
}
```

Both descriptions are general on purpose. They do not say which check failed.

An administrator can add a **CEL Expression** that decides which gateways may exchange tokens. The
broker checks the expression when it starts. It does not start with an expression that does not
compile. `example:token-exchange.policy.invalid-expression`

### When the broker issues the subject token

A broker can issue the agent's tokens itself instead of relying on an upstream authorization
server. A token that the broker issued to the Research Assistant for Alice is then a valid subject
token. `example:token-exchange.subject.locally-issued` The gateway still has to prove its own
identity with a ClientAssertion from an external issuer. A token that the broker issued is not
accepted as a ClientAssertion, and the answer is `401` with `invalid_client`. `example:token-exchange.gateway.local-assertion-rejected`

## No grant, no token

The broker returns a token only when Alice's UserGrant for this agent covers the requested
service. Suppose Alice never gave the Research Assistant a grant: `example:token-exchange.grant.missing`

```http example="token-exchange.grant.missing"
POST /oauth2/token HTTP/1.1
Content-Type: application/x-www-form-urlencoded

grant_type=urn%3Aietf%3Aparams%3Aoauth%3Agrant-type%3Atoken-exchange
&subject_token=…
&subject_token_type=urn%3Aietf%3Aparams%3Aoauth%3Atoken-type%3Aaccess_token
&client_assertion_type=urn%3Aietf%3Aparams%3Aoauth%3Aclient-assertion-type%3Ajwt-bearer
&client_assertion=…
&resource=https%3A%2F%2Fapi.github.com

HTTP/1.1 403 Forbidden

{
  "error": "access_denied",
  "error_description": "User authorization is insufficient. Please re-consent.",
  "error_uri": "http://localhost:8000/agents/550e8400-e29b-41d4-a716-446655440000"
}
```

`error_uri` is a link for Alice, not for the gateway. It opens the broker page where she can grant
this agent access. The UUID in the link is the agent's UUID.

The answer is the same in two more situations:

- Alice's grant has expired. `example:token-exchange.grant.expired`
- Alice has a grant for this agent, but it does not include GitHub. The broker decides this before
  it reads the stored tokens: it sends no request to GitHub, even when the stored access token has
  expired. `example:token-exchange.grant.excludes-service`

## A grant without a usable session

A grant says what the agent may do. A UserSession holds the tokens. Suppose a user has a grant
that covers GitHub but never connected a GitHub account. The broker cannot return a token, and the
error is not `access_denied`, because nothing is forbidden: `example:token-exchange.session.missing`

```http example="token-exchange.session.missing"
POST /oauth2/token HTTP/1.1
Content-Type: application/x-www-form-urlencoded

grant_type=urn%3Aietf%3Aparams%3Aoauth%3Agrant-type%3Atoken-exchange
&subject_token=…
&subject_token_type=urn%3Aietf%3Aparams%3Aoauth%3Atoken-type%3Aaccess_token
&client_assertion_type=urn%3Aietf%3Aparams%3Aoauth%3Aclient-assertion-type%3Ajwt-bearer
&client_assertion=…
&resource=https%3A%2F%2Fapi.github.com

HTTP/1.1 400 Bad Request

{
  "error": "invalid_grant",
  "error_description": "User session is unavailable. Please re-authenticate.",
  "error_uri": "http://localhost:8000/api/third-party/660e8400-e29b-41d4-a716-446655440010/oauth2/authorize?redirect_uri=http%3A%2F%2Flocalhost%3A8000%2Fsessions"
}
```

This `error_uri` is different from the one for a missing grant. It starts the sign-in at the one
service that the exchange needs, and it names the broker's sessions page as the place to return to.
The UUID in the link is the service's UUID. The broker builds the link from its own public URL and
from the service that it found for the resource.

### The link repairs the session

The administrator `admin@example.com` also uses the Research Assistant and is in this situation.
The gateway shows the link, and the administrator opens it in a browser. The broker redirects to
GitHub's authorization page: `example:token-exchange.session.recovery`

```http example="token-exchange.session.recovery"
GET /api/third-party/660e8400-e29b-41d4-a716-446655440010/oauth2/authorize?redirect_uri=http%3A%2F%2Flocalhost%3A8000%2Fsessions HTTP/1.1
X-Remote-User: admin@example.com

HTTP/1.1 302 Found
Location: http://127.0.0.1:…/oauth/authorize?client_id=github-client-id&…

<a href="http://127.0.0.1:…/oauth/authorize?client_id=github-client-id&amp;…">Found</a>.
```

GitHub sends the browser back to the broker's callback with a code. The broker stores the new
tokens and sends the browser to the sessions page:

```http example="token-exchange.session.recovery"
GET /api/third-party/660e8400-e29b-41d4-a716-446655440010/oauth2/callback?code=mock-auth-code-123&state=… HTTP/1.1
X-Remote-User: admin@example.com

HTTP/1.1 302 Found
Location: http://localhost:8000/sessions?service_id=660e8400-e29b-41d4-a716-446655440010&success=true

<a href="http://localhost:8000/sessions?service_id=660e8400-e29b-41d4-a716-446655440010&amp;success=true">Found</a>.
```

The gateway repeats the same exchange, and now it succeeds:

```http example="token-exchange.session.recovery"
POST /oauth2/token HTTP/1.1
Content-Type: application/x-www-form-urlencoded

grant_type=urn%3Aietf%3Aparams%3Aoauth%3Agrant-type%3Atoken-exchange
&subject_token=…
&subject_token_type=urn%3Aietf%3Aparams%3Aoauth%3Atoken-type%3Aaccess_token
&client_assertion_type=urn%3Aietf%3Aparams%3Aoauth%3Aclient-assertion-type%3Ajwt-bearer
&client_assertion=…
&resource=https%3A%2F%2Fapi.github.com

HTTP/1.1 200 OK

{
  "access_token": "reconnected-github-access",
  "token_type": "Bearer",
  "principal": "admin@example.com",
  "…": "…"
}
```

## The broker keeps the token fresh

Access tokens expire. The gateway does not handle that; the broker does.

- **Expired access token.** Alice's stored access token has expired, and the session has a refresh
  token. The broker refreshes the token at GitHub during the exchange and returns the new one. `example:token-exchange.refresh.expired-access-token`
- **Token that expires soon.** Alice's stored token is valid for two more minutes. The exchange
  returns it at once and the broker renews it in the background. A later exchange returns the
  renewed token. `example:token-exchange.refresh.near-expiry`
- **Short provider outage.** GitHub answers the refresh with `503` once. The exchange still
  succeeds, and the gateway sees no `error_uri`. `example:token-exchange.refresh.transient-outage`

### When the provider rejects the refresh token

GitHub can answer the refresh with `invalid_grant`, for example after Alice revoked the broker's
access at GitHub. This is a **Refresh Rejection**. The gateway gets the same `400 invalid_grant`
answer and the same sign-in link as for a missing session. The answer contains nothing of GitHub's
error text. The broker keeps the stored tokens as they are and records the rejection. For the next
exchange it does not ask GitHub again; it returns the same answer directly. `example:token-exchange.refresh.rejected`
When the user follows the sign-in link, the broker clears the recorded rejection on the same
session, and exchanges succeed again. `example:token-exchange.session.recovery-after-rejection`

### When the provider rejects the broker

GitHub can also reject the broker's own client credentials during a refresh. Alice cannot repair
that, so the answer has no `error_uri`: `example:token-exchange.refresh.provider-client-rejected`

```http example="token-exchange.refresh.provider-client-rejected"
POST /oauth2/token HTTP/1.1
Content-Type: application/x-www-form-urlencoded

grant_type=urn%3Aietf%3Aparams%3Aoauth%3Agrant-type%3Atoken-exchange
&subject_token=…
&subject_token_type=urn%3Aietf%3Aparams%3Aoauth%3Atoken-type%3Aaccess_token
&client_assertion_type=urn%3Aietf%3Aparams%3Aoauth%3Aclient-assertion-type%3Ajwt-bearer
&client_assertion=…
&resource=https%3A%2F%2Fapi.github.com

HTTP/1.1 500 Internal Server Error

{
  "error": "server_error",
  "error_description": "internal server error during token exchange"
}
```

## What the gateway does with each answer

| Answer | Meaning | Who acts |
|---|---|---|
| `200` | Token returned | The gateway forwards the call with the token |
| `400 invalid_target` | No service registered the resource | An administrator |
| `401 invalid_client` | The gateway did not prove its identity | The gateway operator |
| `400 invalid_grant` without `error_uri` | The subject token is not acceptable | The agent gets a new token |
| `403 access_denied` with `error_uri` | The grant is missing, expired, or does not cover the service | The user opens the link and consents |
| `400 invalid_grant` with `error_uri` | The session is missing or cannot be refreshed | The user opens the link and signs in at the provider |
| `500 server_error` | The broker or its registration at the provider is at fault | An administrator |

## Not yet on this page

- How the scopes in `scope` follow from the granted permission sets and the agent's service
  requirements, and the denial when the stored session does not cover them.
- The rules of the CEL Expression for gateways: the claims and request values that it can read,
  and the denial when it evaluates to false. No acceptance scenario shows a denied gateway.
- The configurable expressions that read the Principal and the agent from the subject token, and
  agents that are found by their client ID.
- Hybrid mode, where upstream-issued and broker-issued subject tokens are both accepted.
- A dedicated key source for ClientAssertions. Trust-anchor key sources have their own page.
- Diagnostic logs and telemetry of an exchange.
- Subject tokens that are expired, come from another issuer, or lack a required claim.

Minting a broker token for a user or an agent with `audience=` is impersonation and has its own
page. The gateway filter that calls this endpoint for each request also has its own page.

## Sources

The behavior on this page was decided in `specs/013-token-exchange/spec.md`,
`specs/019-permission-sets/spec.md`, `specs/023-extproc-mcp-elicitation/spec.md`,
`specs/026-extproc-approval-sync/spec.md`, `specs/029-proactive-token-refresh/spec.md`, and
`adrs/044-service-specific-session-recovery-url.md`. The contract is `api/enduser/openapi.yaml`.
The scenarios are in `tests/e2e/token_exchange_test.go`, `tests/e2e/grant_diagnostics_test.go`,
`tests/e2e/session_reliability_test.go`, `tests/e2e/proactive_token_refresh_test.go`, and
`tests/e2e/local_token_exchange_e2e_test.go`.
