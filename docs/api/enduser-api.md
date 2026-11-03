# End-User API

The end-user API serves OAuth2 authorization, token, and metadata requests. The broker serves it on the end-user listener.

## CIMD Client Authentication

In local or hybrid mode with CIMD enabled, a pre-registered CIMD client can authenticate at the token endpoint with `private_key_jwt`. The client signs a short-lived assertion with a key from its metadata document. The broker requires a fresh assertion for each authorization-code exchange and refresh request.

The assertion uses `RS256` or `ES256`, and its algorithm must match the client's metadata document. It must include `kid`, `jti`, `iss`, `sub`, `aud`, and `exp`. Both `iss` and `sub` must equal the CIMD URL. The `jti` is single-use per client.

For an authorization-code exchange, send the assertion fields with the existing grant fields:

```http
POST /oauth2/token HTTP/1.1
Content-Type: application/x-www-form-urlencoded

grant_type=authorization_code
&client_id=https%3A%2F%2Fchatgpt.com%2Foauth%2FEBkIgu5McFbd%2Fclient.json
&code=AUTHORIZATION_CODE
&redirect_uri=https%3A%2F%2Fchatgpt.com%2Fconnector%2Foauth%2FEBkIgu5McFbd
&code_verifier=PKCE_VERIFIER
&client_assertion_type=urn%3Aietf%3Aparams%3Aoauth%3Aclient-assertion-type%3Ajwt-bearer
&client_assertion=SIGNED_CLIENT_ASSERTION
```

For refresh, use `grant_type=refresh_token`, `refresh_token`, `client_id`, `client_assertion_type`, and a new `client_assertion`. Do not send `client_secret` or an `Authorization` header with an assertion.

Malformed parameter combinations return `400 invalid_request`. Authentication failures return `401 invalid_client` with a stable description. A document or key-set dependency failure returns `server_error`. A failed request does not consume an authorization code or rotate a refresh token.

See the [token endpoint contract](https://github.com/zalando-incubator/agentic-identity-broker/blob/main/specs/051-private-key-jwt-auth/contracts/token-endpoint.md) for the complete assertion profile and response rules.

## Agent-Subject Token Exchange

In local mode, a privileged client can request a token for a registered agent acting for itself. The [canonical OpenAPI](../../api/enduser/openapi.yaml) defines End-User API **2.0.0**. Use the complete [agent-subject configuration](../../examples/config/impersonation-agent-subject.yaml) and [configuration reference](../configuration.md#subject-categories-and-self-agent-issuance).

Classification is rule-local: the subject must be signed and verified, and its raw string `sub` must exactly equal the audience-selected target's canonical UUID. Everything else is human, including unsigned subjects. There is no request category field. Aliases, a different agent's UUID, or alternate UUID spellings never select agent issuance. The target must exist and existing scope limits still apply. Every credential validation, principal extraction and CEL authorization must succeed. The actor and client assertion always require signatures.

| `allowed_subject_categories` | Human | Signed exact-target subject |
| --- | --- | --- |
| Omitted or `[human]` | Active delegation required | Ineligible, even with same-text human delegation |
| `[agent]` | Ineligible | No human delegation required |
| `[human, agent]` (either order) | Active delegation required | No human delegation required |

Category-ineligible rules fall through. Missing/expired delegation for an authorized human is terminal, with the established consent `error_uri`; no automatic agent retry is allowed. Unsigned-subject rules may list `agent`, but unsigned subjects remain human and that entry has no effect.

### Request and signed response

This example uses registered target `research-agent` with UUID `550e8400-e29b-41d4-a716-446655440000`. Its rule explicitly allows `agent` and authorizes `gateway-prod`, `research-runner` and the target UUID in CEL. The three markers below represent externally signed ES256 credentials with issuer `https://idp.example.com`, audience `https://broker.example.com/impersonation` and valid lifetimes. Their subjects are respectively the gateway, runner and target UUID. Line breaks in the form body are for display only.

```http
POST /oauth2/token HTTP/1.1
Host: broker.example.com
Content-Type: application/x-www-form-urlencoded
Accept: application/json

grant_type=urn:ietf:params:oauth:grant-type:token-exchange
&audience=https%3A%2F%2Fbroker.example.com%2Fimpersonation%2Fresearch-agent
&client_assertion_type=urn:ietf:params:oauth:client-assertion-type:jwt-bearer
&client_assertion=SYNTHETIC_CLIENT_JWS
&actor_token_type=urn:ietf:params:oauth:token-type:jwt
&actor_token=SYNTHETIC_ACTOR_JWS
&subject_token_type=urn:ietf:params:oauth:token-type:jwt
&subject_token=SYNTHETIC_AGENT_JWS

HTTP/1.1 200 OK
Content-Type: application/json
Cache-Control: no-store
Pragma: no-cache

{"access_token":"SYNTHETIC_BROKER_AGENT_JWS","token_type":"Bearer","issued_token_type":"urn:ietf:params:oauth:token-type:access_token","expires_in":3600}
```

Decoded access-token claims (authorization-relevant excerpt; normal `jti` may also be present):

```json
{
  "iss": "https://broker.example.com",
  "sub": "550e8400-e29b-41d4-a716-446655440000",
  "agent_id": "550e8400-e29b-41d4-a716-446655440000",
  "aud": "https://policy.example.com",
  "scope": "",
  "iat": 1793005200,
  "exp": 1793008800,
  "act": {"iss": "https://idp.example.com", "sub": "research-runner"}
}
```

The broker creates no human grant/session and initiates no consent interaction. It retains `act`, even if actor and subject identities happen to be equal. Reserved `sub` and `agent_id` cannot be overridden by local policy. No category claim is added. Subject email extraction is skipped and no credential-derived email is supplied; output email remains policy-owned. Issuer, signing, lifetime, scope and audience policy keep their normal semantics.

### Denials

If no rule accepts the category (including signed exact-target equality on a human-only rule with or without a grant), the response is the generic existing envelope, without consent recovery:

```http
HTTP/1.1 403 Forbidden
Content-Type: application/json

{"error":"access_denied","error_description":"impersonation not permitted"}
```

For an otherwise valid third-party exchange using the issued token as `subject_token`, resource `https://api.thirdparty.example.com`, and a fresh valid third-party client assertion, default detection rejects before agent extraction, authorization or human grant/session/provider access. A matching compliant override has the same result:

```http
HTTP/1.1 403 Forbidden
Content-Type: application/json

{"error":"access_denied","error_description":"agent-subject tokens cannot be exchanged for third-party service tokens"}
```

There is no `error_uri`: human consent cannot recover this denial. Default detection is the guarded equality of `sub` and `agent_id`; it applies regardless of issuer or issuance date. Runtime detection errors, timeouts and dynamic non-boolean output instead return `500 server_error`, also without `error_uri` or human-state access. A configured detector fully replaces the default. Operators must retain broker agent-token matching; a non-matching override that references `sub` (a detector without `subject_token.sub`, such as `false`, fails startup) removes categorical rejection and same-text human-credential isolation. Existing checks may then consult human grants/sessions or return consent errors; this is not support for third-party agent identity or guaranteed exchange success.

See the [exchange concepts](../concepts/token-exchange.md#agent-subject-boundary), [diagnostic reference](../reference/token-exchange.md#agent-subject-boundary), and [detector example](../../examples/config/token-exchange.yaml).

## Migrate to End-User API 2.0.0

API 2.0.0 replaces the previous End-User contract on existing routes, including `POST /oauth2/token`. No runtime version selector, parallel compatibility route, endpoint removal or deprecation is introduced. The contract version is separate from broker release tags and the Admin API version. Treat the baseline as released; the checked-out baseline declares 1.1.0, while the feature's approved proposal described 1.0.0. Both require this major bump.

Before deployment:

1. Inventory clients using local impersonation and third-party RFC 8693 exchange. URLs, request fields and response envelopes are unchanged, but two formerly accepted collision cases now deny: signed raw `sub` equal to target UUID on human-only/omitted rules, even with delegation; and any token with equal `sub`/`agent_id` under default third-party detection. Nonmatching human flows retain their existing delegation, errors and consent links.
2. For intended self-agent issuance, configure a signed rule allowing `agent` and a narrowly authorizing CEL policy. Do not enable it to work around a real human identity collision; same-text grants cannot restore human-only eligibility.
3. For real human collisions, correct the upstream subject or principal mapping so the minted human principal differs from the target UUID. Explicitly reestablish required human delegation and provider sessions through existing flows under the intended human identity. Never automatically rewrite grants/sessions, fabricate consent, or add a database migration for this feature.
4. Review previously minted and trusted external issuer tokens with equal `sub`/`agent_id`, including mapped-principal and unsigned-subject human outputs. The new default rejects them regardless of age or issuer. Reissuing without fixing the collision does not help. Do not use `agent_subject_expression: false` as a compatibility fix: it removes categorical protection. Any override retains operator responsibility to keep matching broker agent tokens.
5. Require consumer readiness and passing complete acceptance/regression/full checks before rollout. If consumers are not ready, defer deployment rather than weaken controls or claim compatibility with the earlier major contract.

The [configuration reference](../configuration.md#subject-categories-and-self-agent-issuance), [concepts](../concepts/token-exchange.md#agent-subject-boundary), [field reference](../reference/token-exchange.md#agent-subject-boundary) and [example configurations](../../examples/config/README.md#impersonation-agent-subjectyaml--signed-self-agent-issuance) describe the same contract.
