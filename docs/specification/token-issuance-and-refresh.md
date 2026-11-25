---
title: "Token issuance and refresh"
description: Living specification of how an agent gets an access token from the broker's own authorization server, and a refresh token that stops working when the user's consent ends. Every example on this page is checked against the acceptance suite.
---

# Token issuance and refresh

An agent needs an access token before a gateway lets it call anything. In `local` mode the broker
issues that token itself. In `hybrid` mode it does the same for every agent that has no upstream
client ID. This page describes what the agent sends, what it gets back, and when a refresh token
stops working, in the order an agent developer meets the behavior.

:::note How to read this page
This is a living specification. A badge such as "tested" after a sentence links to the acceptance
scenario that proves it. Each HTTP exchange shown here is compared with what that scenario really
sent and received. `just living-docs` runs the comparison and fails when the page and the broker
disagree. UUIDs and timestamps are synthetic. The check compares timestamps by shape and UUIDs by
which of them are equal.
:::

## The cast

The **Agent** *Research Assistant* is registered with the redirect URI
`https://client.example.com/cb` and no upstream client ID. Its OAuth2 `client_id` is its UUID,
`550e8400-e29b-41d4-a716-446655440000`. An administrator, `admin@example.com`, gave it a client
secret.

Alice uses the Research Assistant. In the acceptance suite her **Principal** is
`user@example.com`. A reverse proxy in front of the broker authenticates her and passes her
identity in `X-Remote-User`.

A gateway receives the agent's calls. It verifies each access token with the public keys that the
broker publishes. On this page it is the **token verifier**.

Which mode the broker runs in, and what it publishes about itself, is on the page
[Authorization server modes](authorization-server-modes.md).

## A token for the agent itself

Some work has no user. The Research Assistant then asks for a token in its own name with the
client-credentials grant: `example:token-issuance-and-refresh.client-credentials.issue`

```http example="token-issuance-and-refresh.client-credentials.issue"
POST /oauth2/token HTTP/1.1
Content-Type: application/x-www-form-urlencoded

grant_type=client_credentials
&client_id=550e8400-e29b-41d4-a716-446655440000
&client_secret=…

HTTP/1.1 200 OK
Cache-Control: no-store
Content-Type: application/json

{
  "access_token": "…",
  "expires_in": 3600,
  "token_type": "Bearer"
}
```

The access token is a signed JWT. Its lifetime is the configured `token_ttl`, here one hour. This
response has no refresh token. The agent can ask again with its secret at any time.

The operator did nothing to prepare a signing key for this exchange. A broker that starts in
`local` mode without a signing key creates one, marks it as current, and publishes its public
half. `example:token-issuance-and-refresh.signing-keys.first-key`

A wrong client secret gets `401 Unauthorized` and no token. `example:token-issuance-and-refresh.client-credentials.wrong-secret`

### Allowed scopes limit what the agent can ask for

An agent can carry a list of allowed scopes. Suppose the administrator limits the Research
Assistant to the scope `read`. A token request for the scope `admin` is then rejected with
`400 Bad Request`. `example:token-issuance-and-refresh.client-credentials.scope-limit` An agent
without such a list can request any scope, as in the examples that follow.

## A token on Alice's behalf

When the Research Assistant works for Alice, it uses the authorization-code flow. Alice has
already given the agent a **UserGrant**.

The agent creates a random code verifier and sends its SHA-256 hash, the code challenge, with the
authorization request. Alice's browser carries that request to the broker: `example:token-issuance-and-refresh.consent.grant-deleted`

```http example="token-issuance-and-refresh.consent.grant-deleted"
GET /oauth2/authorize?client_id=550e8400-e29b-41d4-a716-446655440000&code_challenge=…&code_challenge_method=S256&redirect_uri=https%3A%2F%2Fclient.example.com%2Fcb&response_type=code&scope=read+offline_access&state=refresh-consent HTTP/1.1
X-Remote-User: user@example.com

HTTP/1.1 302 Found
Location: https://client.example.com/cb?code=…&state=refresh-consent

<a href="…">Found</a>.
```

The broker does not ask Alice to sign in again or to confirm anything. It trusts the identity in
`X-Remote-User`, finds her grant, and sends the browser back to the agent with a one-time code
and the agent's `state`.

The agent then exchanges the code. It proves that it started the flow by sending the verifier:

```http example="token-issuance-and-refresh.consent.grant-deleted"
POST /oauth2/token HTTP/1.1
Content-Type: application/x-www-form-urlencoded

grant_type=authorization_code
&client_id=550e8400-e29b-41d4-a716-446655440000
&client_secret=…
&code=…
&code_verifier=…
&redirect_uri=https%3A%2F%2Fclient.example.com%2Fcb

HTTP/1.1 200 OK
Cache-Control: no-store
Content-Type: application/json

{
  "access_token": "…",
  "expires_in": 3600,
  "refresh_token": "…",
  "scope": "read offline_access",
  "token_type": "Bearer"
}
```

The same flow with only the scope `read` also ends with an access token of type `Bearer`. `example:token-issuance-and-refresh.authorization-code.pkce-flow`

### The code challenge is not optional

An authorization request without a code challenge yields no code. The broker answers with an
error, either directly or as an `error` parameter on the redirect. `example:token-issuance-and-refresh.authorization-code.pkce-required`
The reason is that a code can leak on its
way through the browser, and the verifier is the only proof that the party that redeems the code
also started the flow.

Two rules at the token endpoint follow from the same reason:

- A code presented with a verifier that does not fit the challenge is rejected with
  `400 Bad Request`. `example:token-issuance-and-refresh.authorization-code.wrong-verifier`
- A code works once. A second exchange of the same code is rejected with `400 Bad Request`, even
  with the right verifier. `example:token-issuance-and-refresh.authorization-code.code-replay`

### An agent without a secret

An agent that cannot keep a secret, such as a desktop application, runs the same flow and leaves
`client_secret` out of the token request. The code verifier takes its place as the proof. `example:token-issuance-and-refresh.authorization-code.public-client`
How an agent gets a secret, or identifies itself in another way, belongs to client registration
and authentication.

### No grant, no code

A user who never delegated anything to the Research Assistant does not get a code. The broker
redirects the browser to the agent's consent view under `/agents/` instead. `example:token-issuance-and-refresh.authorization-code.consent-gate`
The flow continues after the user has decided there. The page
[Authorization server modes](authorization-server-modes.md) shows this redirect as an exchange.

## Custom claims in the token

An operator can add claims to every token that the broker issues. The setting
`token_claims_expression` holds a CEL expression that returns a map. With

```yaml
oauth2_authorization_server:
  mode: local
  local:
    token_claims_expression: '{"pemail": principal.email, "pname": principal.display_name}'
```

the access token from Alice's authorization-code flow carries the claims `pname` and `pemail`.
Because `X-Remote-User` gives the broker only an identifier, `pname` equals her Principal and
`pemail` is an empty string. A token that the agent later obtains with a
refresh token carries the same two claims. `example:token-issuance-and-refresh.claims.custom`

## Staying authorized after the access token expires

An access token lasts an hour. An agent that works longer needs a refresh token, and it has to
ask for one: the broker returns a refresh token only when the authorization request includes the
scope `offline_access`. `example:token-issuance-and-refresh.refresh.offline-access` Without that
scope the token response has no `refresh_token` member. `example:token-issuance-and-refresh.refresh.no-offline-access`

When the access token is about to expire, the agent trades the refresh token for a new pair.
Alice is not involved: `example:token-issuance-and-refresh.refresh.rotation`

```http example="token-issuance-and-refresh.consent.grant-deleted"
POST /oauth2/token HTTP/1.1
Content-Type: application/x-www-form-urlencoded

grant_type=refresh_token
&client_id=550e8400-e29b-41d4-a716-446655440000
&client_secret=…
&refresh_token=…

HTTP/1.1 200 OK
Cache-Control: no-store
Content-Type: application/json

{
  "access_token": "…",
  "expires_in": 3600,
  "refresh_token": "…",
  "scope": "read offline_access",
  "token_type": "Bearer"
}
```

The response carries a new refresh token, different from the one the agent sent. The agent must
store it, because the new token is the only one that works from now on.

### A refresh token works once

Each refresh token is good for one exchange. If the agent, or someone who copied the token, sends
a refresh token that was already used, the broker refuses: `example:token-issuance-and-refresh.refresh.reuse`

```http example="token-issuance-and-refresh.consent.grant-deleted"
POST /oauth2/token HTTP/1.1
Content-Type: application/x-www-form-urlencoded

grant_type=refresh_token
&client_id=550e8400-e29b-41d4-a716-446655440000
&client_secret=…
&refresh_token=…

HTTP/1.1 400 Bad Request
Content-Type: application/json

{
  "error": "invalid_grant",
  "error_description": "The provided authorization grant (e.g., authorization code, resource owner credentials) or refresh token is invalid, expired, revoked, does not match the redirection URI used in the authorization request, or was issued to another client."
}
```

The request is identical to the successful one before it. Only the history of the token differs.

## Refresh ends when consent ends

A refresh token lets the agent act for Alice without her. That is acceptable only while her
grant stands. The broker therefore ties every refresh token to the grant under which it was
issued, and checks that grant again at each refresh.

Alice revokes her grant for the Research Assistant:

```http example="token-issuance-and-refresh.consent.grant-deleted"
DELETE /api/consent/agents/550e8400-e29b-41d4-a716-446655440000/grants HTTP/1.1
X-Remote-User: user@example.com

HTTP/1.1 204 No Content
```

The agent still holds a refresh token that it has never used. Its next refresh fails with the same
`invalid_grant` answer as a reused token, and the response contains no tokens. `example:token-issuance-and-refresh.consent.grant-deleted`
The answer does not say why. An agent that receives `invalid_grant` has one way forward: start a
new authorization-code flow, which leads Alice to the consent view.

The other ways in which consent can end have the same effect:

| What happens | Next refresh with the old token |
|---|---|
| Alice's grant passes its expiry time | `invalid_grant` `example:token-issuance-and-refresh.consent.grant-expired` |
| The administrator deletes the agent's client credentials, and the agent tries to continue without a secret | `invalid_grant` `example:token-issuance-and-refresh.consent.credentials-deleted` |
| The administrator deletes the agent | An error and no tokens `example:token-issuance-and-refresh.consent.agent-deleted` |

An agent without a secret is treated the same way: when Alice deletes her grant, its refresh
token stops working. `example:token-issuance-and-refresh.consent.public-client`

Every scenario in this section passes in `local` mode and, for an agent without an upstream
client ID, in `hybrid` mode.

### A new grant does not revive an old refresh token

Suppose Alice revokes her grant and gives the Research Assistant a new one a minute later. The
refresh token from before the revocation stays dead. The agent gets a working refresh token only
through a new authorization-code flow under the new grant. `example:token-issuance-and-refresh.consent.regrant`
A revocation that could be undone by granting again would not be a revocation: Alice may have
granted less the second time.

## Signing keys, as a token verifier sees them

The token verifier reads the broker's public keys from `/oauth2/jwks.json`. It picks the key by
the `kid` in the header of each token. Key rotation is designed so that a verifier that caches
this key set never meets a token that it cannot check.

The administrator starts a rotation by adding a key: `example:token-issuance-and-refresh.signing-keys.rotation`

```http example="token-issuance-and-refresh.signing-keys.rotation"
POST /api/oauth2-server/signing-keys HTTP/1.1
X-Remote-User: admin@example.com
Content-Type: application/json

{
  "algorithm": "ES256"
}

HTTP/1.1 201 Created
Content-Type: application/json

{
  "kid": "770e8400-e29b-41d4-a716-446655440001",
  "algorithm": "ES256",
  "is_current": true,
  "activates_at": "2026-10-30T09:10:00Z",
  "created_at": "2026-10-30T09:00:00Z"
}
```

The new key is published at once, next to the old one. It does not sign yet: `activates_at` lies
after `created_at`, and a token issued in between still carries the `kid` of the old key. The
delay gives verifiers time to fetch the new public key before the first token arrives that needs
it.

An administrator who does not want to wait can promote the key:

```http example="token-issuance-and-refresh.signing-keys.rotation"
PUT /api/oauth2-server/signing-keys/770e8400-e29b-41d4-a716-446655440001/current HTTP/1.1
X-Remote-User: admin@example.com

HTTP/1.1 200 OK
Content-Type: application/json

{
  "kid": "770e8400-e29b-41d4-a716-446655440001",
  "algorithm": "ES256",
  "is_current": true,
  "activates_at": "2026-10-30T09:00:00Z",
  "created_at": "2026-10-30T09:00:00Z"
}
```

From this moment new tokens carry the new `kid`. The old key stays in the published key set, so
tokens that it signed remain verifiable until they expire.

### Removing a key ends the tokens that it signed

Once the old tokens have expired, the administrator deletes the old key. It disappears from the
published key set, and a token that it signed no longer passes verification. `example:token-issuance-and-refresh.signing-keys.removal`

The broker refuses a removal that would leave it unable to sign. The key that still signs tokens
while its successor waits for `activates_at` cannot be deleted: `example:token-issuance-and-refresh.signing-keys.removal-refused`

```http example="token-issuance-and-refresh.signing-keys.removal-refused"
DELETE /api/oauth2-server/signing-keys/660e8400-e29b-41d4-a716-446655440002 HTTP/1.1
X-Remote-User: admin@example.com

HTTP/1.1 409 Conflict
Content-Type: application/json

{
  "error": "current_key",
  "message": "wait for the promoted signing key to activate or promote a different key before removing the key still signing tokens"
}
```

The only key that exists cannot be deleted either. The answer is also `409 Conflict`. `example:token-issuance-and-refresh.signing-keys.last-key`

## Not yet on this page

- The standard claims of an issued token: `sub` is the agent's UUID for the client-credentials
  grant and the user's Principal for the authorization-code grant, `agent_id` is the agent's UUID,
  and custom claims cannot replace base claims. No acceptance scenario decodes these claims.
- Allowed scopes in the authorization-code flow, and the `invalid_scope` error code. The scenario
  for the client-credentials grant checks only the status code.
- The error codes for a missing code challenge, a wrong verifier, and a replayed code. The
  scenarios check only the status code. The 60-second lifetime of a code is not tested here.
- Refresh tokens for a client that does not declare the refresh-token grant, the lifetime of a
  refresh token (`refresh_token_ttl`), and the rule that only the client that received a refresh
  token can redeem it.
- Revocation of refresh tokens when a user renews an expired grant, and the one-time allowance for
  refresh tokens issued before grant binding existed. Storage tests cover these, not acceptance
  scenarios.
- A failing or invalid `token_claims_expression`.
- Access tokens that are already issued stay valid until they expire when a grant ends.
- Audit log entries for token issuance and revocation.

## Sources

The behavior on this page was decided in `specs/025-oauth2-server/spec.md`,
`specs/033-offline-access-refresh/spec.md`, `specs/049-fix-refresh-consent/spec.md`, and
`specs/030-hybrid-oauth-modes/spec.md`, with `adrs/014-oauth2-server-mode.md`. The contracts are
`api/enduser/openapi.yaml` and `api/admin/openapi.yaml`. The scenarios are in
`tests/e2e/oauth2_authorize_e2e_test.go`, `tests/e2e/oauth2_token_e2e_test.go`,
`tests/e2e/oauth2_server_mode_e2e_test.go`, `tests/e2e/oauth2_signing_keys_e2e_test.go`, and
`tests/e2e/refresh_consent_e2e_test.go`.
