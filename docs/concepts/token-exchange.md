---
title: "Token exchange"
description: "How the broker uses RFC 8693 for third-party token exchange and local user impersonation."
---

# Token exchange

The broker supports two distinct RFC 8693 flows on `POST /oauth2/token`:

| Flow | Activation | Result |
|---|---|---|
| Third-party token exchange | Standard exchange parameters, including `resource` | A provider credential held in the encrypted token vault. |
| User impersonation | One `audience` equal to `<impersonation.audience_prefix>/<canonical lower-case AgentID UUID or canonical_id>` in local mode | A locally issued broker token with an impersonated `sub`, target-derived UUID `agent_id`, and accountable `act.iss`/`act.sub`. |

This page explains third-party token exchange. User impersonation does not resolve a
third-party resource or return a provider credential. The target agent defines optional
scope policy. The routing audience does not control issued `aud`. See
[user impersonation](/docs/reference/token-exchange#user-impersonation) for its request,
response, error, and audit contract. See [Configuration](/docs/configuration) for its
operator configuration.

## Third-party token exchange

An agent that holds a third-party token has the problems that the broker prevents. The token
can have broad access. You cannot revoke it for one agent. It does not give useful audit
information. Token exchange keeps the provider credential in one controlled place. It
returns access for a request:

- The agent has only its broker-issued or upstream token. It never has a provider
  credential.
- Each exchange uses an active delegation. Revoking a grant stops future exchanges.
- Each exchange identifies a user, agent, and resource. This creates an audit record.

The broker implements this with **RFC 8693 OAuth2 Token Exchange** on its
`POST /oauth2/token` endpoint.

### The actors

An exchange involves four parties. Each party has a specific role:

- **The privileged gateway** exchanges the token for the agent. It authenticates to the
  broker with a signed `client_assertion` JWT. The broker validates this JWT against the
  upstream JWKS. Only a trusted gateway can request stored credentials. The assertion
  subject identifies the gateway in audit data.
- **The subject token** is the agent token in `subject_token`. The broker uses CEL
  expressions to get the **user** and **agent** identities. The default claims are `sub`
  and `azp`. Together, they identify the delegation.
- **The resource** is the target in `resource`. The broker normalizes it and compares it to
  third-party-service `protected_resources`. This identifies the provider token to return.
- **The broker** makes sure that the user has an active grant for the agent and service. It
  retrieves and refreshes the stored third-party token. It then returns the token.

`subject_token_type` must be `urn:ietf:params:oauth:token-type:access_token`. This is the
only accepted value.

### How an exchange flows

```mermaid
sequenceDiagram
    participant A as AI agent
    participant G as Privileged gateway
    participant B as Broker
    A->>G: Request to third-party resource<br/>(agent token as Bearer)
    G->>B: POST /oauth2/token (RFC 8693)<br/>client_assertion + subject_token + resource
    B->>B: Authenticate gateway (client_assertion vs upstream JWKS)
    B->>B: Extract principal and finalize security capture
    B->>B: Detect agent subject (match/error rejects before human state)
    B->>B: Extract agent from subject_token (CEL)
    B->>B: Resolve service by resource → protected_resources
    B->>B: CEL policy: may this exchange happen?
    B->>B: Verify active grant (user delegated this agent + service)
    B->>B: Retrieve/refresh stored third-party token (decrypt)
    B-->>G: Third-party access token + granted_permission_sets
    G->>A: (continues the proxied call with the third-party token)
```

The response contains the third-party `access_token`, `token_type`, and `issued_token_type`.
It also contains the `granted_permission_sets` used for the exchange. The caller can see
the delegated access.

### Two policy gates, composed as fail-closed AND

An exchange has two independent policy gates. Both gates must allow the request:

- **Broker CEL — Can this exchange occur?** The token endpoint evaluates a Common
  Expression Language policy with gateway assertion claims and RFC 8693 request fields. By
  default, the expression is `true`. You can restrict eligible gateways, agents, and
  resources.
- **ExtProc OPA — Can this proxied request continue?** The
  [ExtProc gateway sidecar](/docs/guides/token-exchange-gateway) can use an optional Open
  Policy Agent gate. It evaluates the proxied request and relevant MCP tool calls. It allows
  or denies the request. This gate is disabled by default.

The two gates form a fail-closed AND condition. ExtProc OPA can restrict a call that broker
CEL allowed. Broker CEL can reject an exchange even when OPA allows the downstream call.
Neither gate can grant access that the other rejected. A rejected exchange returns an error.
The sidecar does not forward the original agent token.

## Agent-subject boundary

Local impersonation can issue a token for an agent acting for itself under an explicitly agent-permitting signed rule. The raw verified `sub` must exactly equal the audience-selected target UUID. Its reserved output `sub` and `agent_id` are equal, and `act` remains present; no category claim is added. No human delegation or grant/session creation is involved. Human subjects still require delegation. A signed exact-target subject on a human-only/omitted rule is ineligible even with same-text human delegation.

This does **not** introduce third-party agent identities. Third-party exchange evaluates `token_exchange.claim_extraction.agent_subject_expression` after validated principal extraction and security capture, before agent extraction, authorization or human grant/session/provider access. Its default is `has(subject_token.sub) && has(subject_token.agent_id) && subject_token.sub == subject_token.agent_id`. Matches return `403 access_denied`, description `agent-subject tokens cannot be exchanged for third-party service tokens`, and no consent `error_uri`. This includes same-text human state and trusted external issuer tokens. Equal-claim legacy, mapped-principal and unsigned-subject human outputs are also rejected by default. Consent cannot recover these denials.

A configured expression fully replaces the default. Keep matching broker agent-subject tokens, for example by retaining the default as a disjunct. **A non-matching override such as `false` removes categorical rejection and protection against same-text human-credential access.** Existing checks can then read human grants/sessions and return consent errors; exchange success is not promised. Detection errors/timeouts and dynamic non-boolean output instead fail closed with `500 server_error`, no `error_uri`, and no human-state access. They never become false.

See the [End-User API 2.0.0 migration guide](../api/enduser-api.md#migrate-to-end-user-api-200) before upgrading, the [configuration reference](../configuration.md#agent-subject-detection-for-third-party-exchange), [detector example](../../examples/config/token-exchange.yaml) and [diagnostic reference](../reference/token-exchange.md#agent-subject-boundary). Correct human identity collisions and explicitly reestablish consent; do not automatically rewrite grants or disable detection as a compatibility fix.

## Related

- [Token exchange reference](/docs/reference/token-exchange) — the request and response
  fields in detail.
- [Token exchange at the gateway](/docs/guides/token-exchange-gateway) — deploy the sidecar
  that performs exchange transparently.
- [OAuth2 server modes](/docs/concepts/oauth2-server-modes) — how the agent's subject token is
  issued in the first place.
- [Delegation and consent](/docs/concepts/delegation-and-consent) — the grant the broker
  verifies during every exchange.
