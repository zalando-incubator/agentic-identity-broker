---
title: "Impersonation"
description: Living specification of how a privileged client obtains a broker-signed token that represents a user or an agent. Every example on this page is checked against the acceptance suite.
---

# Impersonation

Some callers cannot send a user through a browser sign-in. A chat bridge or an API gateway already
knows who the user is and which agent works for that user. Impersonation lets such a
**Privileged Client** ask the broker for a broker-signed token that represents the user, or the
agent itself. This page describes that request, the rules that a platform operator writes for it,
and each answer the client can get.

:::note How to read this page
This is a living specification. A badge such as "tested" after a sentence links to the acceptance
scenario that proves it. Each HTTP exchange shown here is compared with what that scenario really
sent and received. `just living-docs` runs the comparison and fails when the page and the broker
disagree. UUIDs and timestamps are synthetic. The check compares timestamps by shape and UUIDs by
which of them are equal.
:::

## The cast

Alice uses the agent *Research Assistant*. The broker knows the agent under a UUID and under the
canonical ID `impersonation-target`. The agent's allowed scopes are `read` and `write`. In the
first part of this page Alice's **Principal** is `user-1` and her email is `user@example.com`.
Alice gave the agent a **UserGrant**.

A gateway is the Privileged Client. Its identity is `gateway-prod`. An identity provider that the
operator trusts signs three JWTs for each request:

| Credential | Request parameter | Who it identifies |
|---|---|---|
| **ClientAssertion** | `client_assertion` | The gateway: `gateway-prod` |
| Actor token | `actor_token` | The party that acts: `actor-1` |
| Subject token | `subject_token` | The party that the new token represents: `user-1` |

The broker runs in `local` mode and signs its own tokens. A broker in `hybrid` mode refuses to
start with an impersonation configuration. `example:impersonation.mode.local-only`

## What the operator configures

The operator sets an audience prefix and an ordered list of **Impersonation Rules**. The
acceptance suite uses the equivalent of this configuration. The block is an illustration; the
check does not compare it.

```yaml
oauth2_authorization_server:
  mode: local
  impersonation:
    audience_prefix: https://broker.example.com/impersonation
    rules:
      - name: internal-gateway
        roles:
          client_assertion:
            expected_audience: https://broker.example.com/impersonation
            principal_expression: client_assertion.sub
          actor:
            expected_audience: https://broker.example.com/impersonation
            principal_expression: actor_token.sub
          subject:
            expected_audience: https://broker.example.com/impersonation
            principal_expression: subject_token.sub
            email_expression: "has(subject_token.email) ? subject_token.email : ''"
        trusted_issuers:
          - issuer_uri: https://idp.example.com
            jwks_uri: https://idp.example.com/jwks.json
            allowed_algorithms: [RS256, ES256]
            signs_roles: [client_assertion, actor, subject]
        authorization:
          type: cel
          cel:
            expression: client_assertion.sub == "gateway-prod"
```

A rule names the issuers that can sign each credential, says how to read an identity from each
credential, and ends with one CEL predicate. The page "Trust anchors" describes where an issuer's
keys come from.

## One request, one broker token

The gateway sends a token exchange request. The `audience` parameter selects impersonation: it is
the configured prefix, a slash, and the UUID of the **Target Agent**. The broker answers with a
token that it signed itself: `example:impersonation.issue.signed-subject`

```http example="impersonation.issue.signed-subject"
POST /oauth2/token HTTP/1.1
Content-Type: application/x-www-form-urlencoded

grant_type=urn%3Aietf%3Aparams%3Aoauth%3Agrant-type%3Atoken-exchange
&audience=https%3A%2F%2Fbroker.example.com%2Fimpersonation%2F550e8400-e29b-41d4-a716-446655440000
&client_assertion_type=urn%3Aietf%3Aparams%3Aoauth%3Aclient-assertion-type%3Ajwt-bearer
&client_assertion=…
&actor_token_type=urn%3Aietf%3Aparams%3Aoauth%3Atoken-type%3Ajwt
&actor_token=…
&subject_token_type=urn%3Aietf%3Aparams%3Aoauth%3Atoken-type%3Ajwt
&subject_token=…

HTTP/1.1 200 OK
Content-Type: application/json
Cache-Control: no-store

{
  "access_token": "…",
  "token_type": "Bearer",
  "issued_token_type": "urn:ietf:params:oauth:token-type:access_token"
}
```

The same scenario decodes the access token and checks these claims:

- `sub` is `user-1`, the identity from the subject token. The token represents Alice.
- `agent_id` is the UUID of the Research Assistant, taken from `audience`.
- `act` holds `sub`: `actor-1` and `iss`: the issuer that signed the actor token. A receiver can
  see who acted for Alice.
- `scope` is empty, and the response has no `scope` member, because the request asked for none.

The token also has the usual `iss`, `iat`, `exp`, and `jti` claims of a broker token, and the
`email` claim `user@example.com` from the subject token. It contains none of the submitted
credentials. `example:impersonation.issue.claims`

The broker records an `impersonation_decision` audit event for the request. The event contains
none of the three submitted tokens and not the issued token. `example:impersonation.audit.no-credentials`

### The audience names the agent

The gateway can name the agent by its canonical ID instead of its UUID. The issued token is
equivalent, and `agent_id` is still the UUID. `example:impersonation.audience.canonical-id`

An audience under the prefix that names no registered agent gets `invalid_target`. The prefix
alone, or a suffix that is neither a UUID nor a valid canonical ID, gets `invalid_request`: `example:impersonation.audience.unknown-target`

```http example="impersonation.audience.unknown-target"
POST /oauth2/token HTTP/1.1
Content-Type: application/x-www-form-urlencoded

grant_type=urn%3Aietf%3Aparams%3Aoauth%3Agrant-type%3Atoken-exchange
&audience=https%3A%2F%2Fbroker.example.com%2Fimpersonation%2Funknown-canonical-target
&client_assertion_type=urn%3Aietf%3Aparams%3Aoauth%3Aclient-assertion-type%3Ajwt-bearer
&client_assertion=…
&actor_token_type=urn%3Aietf%3Aparams%3Aoauth%3Atoken-type%3Ajwt
&actor_token=…
&subject_token_type=urn%3Aietf%3Aparams%3Aoauth%3Atoken-type%3Ajwt
&subject_token=…

HTTP/1.1 400 Bad Request

{
  "error": "invalid_target",
  "error_description": "token exchange target is invalid or unavailable"
}
```

An audience outside the prefix does not select impersonation, and the broker issues no
impersonated token. `example:impersonation.audience.other` The broker then treats the request as
an ordinary token exchange, which the page "Token exchange" describes.

### Scopes stay inside the agent's allowed scopes

The gateway can ask for a scope. The Research Assistant allows `read`, so the request with
`scope=read` succeeds. The response and the token both carry the scope: `example:impersonation.scope.allowed`

```http example="impersonation.scope.allowed"
POST /oauth2/token HTTP/1.1
Content-Type: application/x-www-form-urlencoded

grant_type=urn%3Aietf%3Aparams%3Aoauth%3Agrant-type%3Atoken-exchange
&audience=https%3A%2F%2Fbroker.example.com%2Fimpersonation%2F550e8400-e29b-41d4-a716-446655440000
&scope=read
&client_assertion_type=urn%3Aietf%3Aparams%3Aoauth%3Aclient-assertion-type%3Ajwt-bearer
&client_assertion=…
&actor_token_type=urn%3Aietf%3Aparams%3Aoauth%3Atoken-type%3Ajwt
&actor_token=…
&subject_token_type=urn%3Aietf%3Aparams%3Aoauth%3Atoken-type%3Ajwt
&subject_token=…

HTTP/1.1 200 OK

{
  "access_token": "…",
  "token_type": "Bearer",
  "issued_token_type": "urn:ietf:params:oauth:token-type:access_token",
  "scope": "read"
}
```

The same request with `scope=admin` gets `400 Bad Request` with the error `invalid_scope`, and
no token. `example:impersonation.scope.outside-allow-list`

## Rules decide who can impersonate

The broker walks the rules in the configured order. A rule matches when all three credentials
validate against its trusted issuers and its predicate returns true. The first rule that matches
decides the request, and later rules are not applied. `example:impersonation.rules.first-match`
When the predicate of the first rule does not permit the gateway, the broker goes on to the
second rule. Two rules can trust the same issuer. `example:impersonation.rules.fall-through`

A client assertion from an issuer that no rule trusts is a failed client authentication. The
answer is `401 Unauthorized` with the error `invalid_client`. `example:impersonation.rules.untrusted-issuer`

A valid assertion for another client, `gateway-untrusted`, passes authentication but no predicate
permits it. The answer is `403 Forbidden` with the error `access_denied` and the description
`token exchange is not authorized`. `example:impersonation.rules.predicate-denies`

## The user must have delegated to the agent

A rule says what the gateway may ask for. It does not replace the user's own decision. Suppose
the gateway asks for a token for a user who never gave the Research Assistant a UserGrant. Every
credential is valid and the rule permits the request. The broker still refuses, and it says
where the user can give consent: `example:impersonation.delegation.missing`

```http example="impersonation.delegation.missing"
POST /oauth2/token HTTP/1.1
Content-Type: application/x-www-form-urlencoded

grant_type=urn%3Aietf%3Aparams%3Aoauth%3Agrant-type%3Atoken-exchange
&audience=https%3A%2F%2Fbroker.example.com%2Fimpersonation%2F550e8400-e29b-41d4-a716-446655440000
&client_assertion_type=urn%3Aietf%3Aparams%3Aoauth%3Aclient-assertion-type%3Ajwt-bearer
&client_assertion=…
&actor_token_type=urn%3Aietf%3Aparams%3Aoauth%3Atoken-type%3Ajwt
&actor_token=…
&subject_token_type=urn%3Aietf%3Aparams%3Aoauth%3Atoken-type%3Ajwt
&subject_token=…

HTTP/1.1 403 Forbidden
Content-Type: application/json
Cache-Control: no-store

{
  "error": "access_denied",
  "error_description": "User authorization is insufficient. Please re-consent.",
  "error_uri": "http://localhost:8000/agents/550e8400-e29b-41d4-a716-446655440000"
}
```

`error_uri` is the broker's consent page for the agent. The gateway can show the link to the
user. A user whose UserGrant has expired gets the identical answer, so the response does not
reveal that a grant once existed. `example:impersonation.delegation.expired`

## The bounded unsigned-subject exception

A chat bridge often has no identity provider that can sign a token for the chat user. For this
case a rule can set `verification: none` on its subject role. The gateway then sends the subject
as an unsigned JWT (`alg: none`). The client assertion and the actor token are still signed and
verified. The broker issues a token with the asserted `sub`, the email, and the same `act` claim. `example:impersonation.unsigned-subject.issue`

The exception has firm bounds:

- It exists only where a rule declares it. A rule with a signed subject role rejects an unsigned
  subject token with `invalid_request`. `example:impersonation.unsigned-subject.signed-rule-rejects`
- The predicate of such a rule must reference `subject_token`. Otherwise the broker does not
  start. `example:impersonation.unsigned-subject.predicate-must-bind` The predicate is the only
  control over which subjects the gateway can assert, so it cannot be left open.
- The user delegation is still required. An asserted user without a UserGrant gets the same
  `access_denied` answer with the consent link. `example:impersonation.unsigned-subject.delegation-missing`

## A token for the agent itself

Some work has no user. A scheduled research run acts for the Research Assistant alone. A rule
can allow this with its **Allowed Subject Categories**:

```yaml
      - name: subject-eligibility
        allowed_subject_categories: [agent]
```

The broker gives each request a **Subject Category**. The category is `agent` only when the
subject token's `sub` is exactly the UUID of the Target Agent. The
gateway sends the same request as in the first example, with a signed subject token whose `sub`
is the agent's UUID, and gets the same kind of `200 OK` answer.

The result is an **Agent-Subject Token**. Its `sub` and its `agent_id` are both the agent's UUID.
It has no `email` claim and no claim that names the category. No UserGrant is needed, and the
broker creates no UserGrant and no UserSession for the agent. `example:impersonation.agent-subject.issue`

### Each rule says which subjects it accepts

A rule accepts `human`, `agent`, or both. A rule that omits the list accepts `human` only. The
acceptance suite uses Alice as `alice@example.com` here, with a UserGrant, and tries both
subjects against each kind of rule: `example:impersonation.agent-subject.categories`

| Rule accepts | Subject `alice@example.com` | Subject is the Target Agent's UUID |
|---|---|---|
| `[human]` | Token for Alice | `access_denied` |
| `[agent]` | `access_denied` | Agent-Subject Token |
| `[human, agent]` | Token for Alice | Agent-Subject Token |

A denied request in this table gets `403 Forbidden` with the description
`impersonation not permitted` and no `error_uri`. Consent cannot repair it.

Two cases close the gaps between the categories:

- On a human-only rule, a signed `sub` equal to the Target Agent's UUID is denied. The broker
  does not treat it as a user, even when a UserGrant exists for a Principal with that same text. `example:impersonation.agent-subject.human-only-rule`
- A signed `sub` equal to the UUID of a different registered agent is denied, on a human-only
  rule and on a rule that accepts both categories. One agent cannot be represented in a token for another, and the broker does not fall
  back to treating that UUID as a user. `example:impersonation.agent-subject.other-agent`

An unsigned subject can also be an agent subject, when the rule sets `verification: none`,
accepts `agent`, and its predicate binds `subject_token.sub`. `example:impersonation.agent-subject.unsigned`

### What an Agent-Subject Token cannot do

An Agent-Subject Token represents no user, so it must not reach anything that belongs to a
user. The broker refuses to exchange it for a third-party service token. The answer is
`403 Forbidden` with `access_denied` and the description
`agent-subject tokens cannot be exchanged for third-party service tokens`. `example:impersonation.agent-subject.no-third-party-exchange`

It also cannot ask a user for a tool approval. The gateway sends the Agent-Subject Token as the
bearer token in `Authorization`, and the broker creates no **ToolApproval**: `example:impersonation.agent-subject.no-approvals`

```http example="impersonation.agent-subject.no-approvals"
POST /api/approvals HTTP/1.1
Content-Type: application/json
X-Client-Assertion: …

{
  "metadata": {
    "description": "Run tests"
  },
  "tool_name": "run_tests",
  "arguments": {
    "suite": "unit"
  }
}

HTTP/1.1 403 Forbidden

{
  "error": "forbidden",
  "message": "agent-subject tokens cannot authenticate approval requests"
}
```

The same request with a token that represents a user creates the approval.

## Not yet on this page

The capability includes more than this page shows. The tests cover these parts, but the page
omits them: the required token types and the other malformed-request answers, expired
credentials and wrong credential audiences, startup validation of rules and of
`allowed_subject_categories` values, an unreachable issuer key set, the full content of the audit
event, a replacement for the detector of Agent-Subject Tokens, and the failure answer when a
predicate cannot be evaluated. The `server_error` answer when delegation storage is unavailable
needs Docker. The policy for signed credentials without an `aud` claim, and the rejection of an
impersonation request by a running broker in `proxy` or `hybrid` mode, have no acceptance
scenario in the two test files that this page uses.

## Sources

The behavior on this page was decided in `specs/037-oauth2-user-impersonation/spec.md`,
`specs/054-agent-subject-impersonation/spec.md`,
`adrs/031-unverified-subject-unsigned-jwt.md`,
`adrs/032-impersonation-requires-user-delegation.md`,
`adrs/034-audience-absent-impersonation-credentials.md`, and
`adrs/045-agent-subject-impersonation.md`. The contract is `api/enduser/openapi.yaml`. An example
configuration is `examples/config/impersonation-agent-subject.yaml`. The scenarios are in
`tests/e2e/impersonation_test.go` and `tests/e2e/agent_subject_impersonation_test.go`.
