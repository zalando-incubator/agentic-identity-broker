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

In local mode, a privileged client can request a token for a registered agent acting for itself. The [canonical OpenAPI](../../api/enduser/openapi.yaml) defines the contract. Use the complete [agent-subject configuration](../../examples/config/impersonation-agent-subject.yaml) and [configuration reference](../configuration.md#subject-categories-and-self-agent-issuance).

Classification is rule-local: the raw string `sub` must exactly equal the audience-selected target's canonical UUID, and the subject must be signed and verified or, for an unsigned (`verification: none`) subject, the rule's predicate must bind `subject_token.sub`. Everything else is human. There is no request category field. Aliases and alternate UUID spellings never select agent issuance, and a different registered agent's UUID is rejected without falling back to human delegation. The target must exist and existing scope limits still apply. Every credential validation, principal extraction and CEL authorization must succeed. The actor and client assertion always require signatures.

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

### Approval endpoints

The same detector guards the approval endpoints that authenticate a subject token (ADR 045 decision 6a). An agent-subject token presented to `POST /api/approvals` or `POST /api/approvals/{id}/consume` is rejected before any approval is read or written:

```http
HTTP/1.1 403 Forbidden
Content-Type: application/json

{"error":"forbidden","message":"agent-subject tokens cannot authenticate approval requests"}
```

A detection failure returns `503 service_unavailable`. `GET /api/approvals` authenticates only the client assertion and is unaffected.

The [configuration reference](../configuration.md#subject-categories-and-self-agent-issuance), [concepts](../concepts/token-exchange.md#agent-subject-boundary), [field reference](../reference/token-exchange.md#agent-subject-boundary) and [example configurations](https://github.com/zalando-incubator/agentic-identity-broker/blob/main/examples/config/README.md#impersonation-agent-subjectyaml--signed-self-agent-issuance) describe the same contract.
## Incremental Service Authorization

API version 1.3.0 adds Virtual Service Scopes, `<public URL>/sessions/<canonical-id-or-UUID>`. The [incremental authorization guide](../guides/incremental-authorization.md) describes the flows, and [`api/enduser/openapi.yaml`](../../api/enduser/openapi.yaml) is the authoritative contract.

- `GET /oauth2/authorize` accepts virtual scopes. They are exempt from the agent's `allowed_scopes` and make each named service required for that request. Malformed, unknown or ineligible service scopes redirect with `invalid_scope`. An unsatisfied service redirects to consent; unverifiable usability redirects with `temporarily_unavailable`. After Save, the server-built continuation carries the same `session_token`, which the endpoint revalidates before issuing a code or redirecting upstream. Forged, malformed, expired or request/principal/agent-mismatched context returns `302 access_denied` with `authorization session is invalid or expired`; a valid JWE with reassigned binding returns `302 invalid_scope` with `requested service scope is not permitted`. Redirect validation runs first; errors use only the registered callback and request state (omitted if absent), never forwarding JWE or PKCE material.
- `GET /api/consent/agents/{agent-id}` always returns `requested_service_ids` and `requested_service_coverage`. Coverage lists the service's required scopes, baseline usability and every covering permission set with its supplied required scopes and usability (`usable`, `missing`, `unusable` or `unavailable`).
- `POST /api/consent/agents/{agent-id}/grants` returns `400 requested service not satisfied` without writing the grant when a requested service is not selected, lacks required-scope coverage or has no usable session.
- `POST /oauth2/token`: recoverable token-exchange errors include `scope`. Local responses and JWT `scope` claims include granted virtual scopes. Refresh never widens: adding a scope returns `invalid_scope`, a bound service that is no longer satisfied returns `invalid_scope`, and unverifiable usability returns `server_error`. None of these rotate the refresh token. Proxy refresh validates broker-prefixed syntax, current service existence and agent eligibility before forwarding: `400 invalid_scope` (`requested service scope is not permitted`) or lookup-failure `500 server_error` makes no upstream call. Valid virtual scopes are removed; ordinary-only requests add no lookup, and upstream response/rotation remain unchanged. No session-usability or grant-coverage check runs.
- `GET /api/third-party/sessions/{service}` returns the service, the caller's session (or `null`) and the caller's dependent agents for a canonical ID or UUID, and `404` for an unknown service.
