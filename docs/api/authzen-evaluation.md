# AuthZEN Delegation Evaluation

The broker acts as an AuthZEN 1.0 Policy Decision Point (PDP) for user delegations
([ADR 045](https://github.com/zalando-incubator/agentic-identity-broker/blob/main/adrs/045-authzen-delegation-pip.md)).
A gateway asks whether a verified user's delegation to an agent covers one protected resource and
receives a credential-free answer. Both endpoints exist on the end-user server whenever token
exchange is configured; there is no separate broker setting. Without token exchange, both return
404.

## Discovery

```http
GET /.well-known/authzen-configuration HTTP/1.1
Host: broker.example.com
```

```http
HTTP/1.1 200 OK
Content-Type: application/json
Cache-Control: max-age=3600

{"policy_decision_point":"https://broker.example.com",
 "access_evaluation_endpoint":"https://broker.example.com/access/v1/evaluation"}
```

`policy_decision_point` is `server.enduser.public_url` without a trailing slash, the same base as
consent links. A consumer must compare it exactly with its expected identifier before sending any
credential. Batch and search endpoints are not offered and are therefore omitted.

AuthZEN inserts the well-known string between the host and the path of the identifier. For a broker
published under a path prefix, such as `https://example.com/broker`, consumers request
`https://example.com/.well-known/authzen-configuration/broker`. The broker serves only
`/.well-known/authzen-configuration`, so the reverse proxy must route that URL to it.

## Access Evaluation

The gateway authenticates with `Authorization: Bearer <client assertion>`, validated by the
token-exchange client-assertion trust anchor. The caller is authorized by the unchanged
`token_exchange.authorization.cel.expression`, with
`request.grant_type = urn:ietf:params:oauth:grant-type:token-exchange`,
`request.resource = resource.id` and an empty `request.scope`. A policy cannot tell evaluation from
exchange, so evaluation is never more permissive than exchange.

```http
POST /access/v1/evaluation HTTP/1.1
Host: broker.example.com
Authorization: Bearer <client assertion>
Content-Type: application/json
X-Request-ID: delegation-001

{"subject":{"type":"urn:ietf:params:oauth:token-type:jwt","id":"<subject JWT>"},
 "action":{"name":"urn:ietf:params:oauth:grant-type:token-exchange"},
 "resource":{"type":"protected_resource","id":"https://api.example.com/repos"},
 "context":{"agent":"550e8400-e29b-41d4-a716-446655440000"}}
```

- `subject.type`, `action.name` and `resource.type` must have exactly these values.
- `resource.id` is an absolute URI, normalized as in token exchange.
- `context` is optional. `context.agent` is an optional Agent UUID anchor; it must equal the agent
  resolved from the verified subject token, and a mismatch returns 400 before any grant is read.
- Unknown members, AuthZEN `properties` and other `context` members are ignored, as the AuthZEN
  HTTPS binding requires. They never influence the decision.
- The body is limited to 64 KiB. A supplied `X-Request-ID` is echoed on every response.

### Permit

```http
HTTP/1.1 200 OK
Cache-Control: private, max-age=15
X-Request-ID: delegation-001

{"decision":true,"context":{
  "subject":{"id":"alice@example.com"},
  "agent":{"id":"550e8400-e29b-41d4-a716-446655440000","canonical_id":"data-assistant"},
  "resource":{"id":"880e8400-e29b-41d4-a716-446655440010","canonical_id":"repo-api",
              "resource_uri":"https://api.example.com/repos","scopes":["read:repo"]},
  "grant":{"id":"990e8400-e29b-41d4-a716-446655440020",
           "updated_at":"2026-10-26T09:59:00Z","valid_until":"2026-10-26T10:00:15Z"},
  "permission_sets":[{"id":"770e8400-e29b-41d4-a716-446655440001","canonical_id":"repo-reader",
    "services":[{"id":"880e8400-e29b-41d4-a716-446655440010","canonical_id":"repo-api",
                 "scopes":["read:repo"]}]}]}}
```

- `resource.scopes` is the effective scope set after the agent's scope ceiling.
  `permission_sets[].services[].scopes` are each permission set's raw contributions for
  grant-included services, `[]` when it defines none.
- Optional `canonical_id` members and `grant.valid_until` are omitted when unset, never null.
- Freshness: `max-age` is `floor(seconds until grant expiry)` capped at 60, or 60 without an
  expiry. The broker re-checks expiry before permitting, so a permit never outlives its grant.
- Evaluation reads no provider session and never refreshes or returns a credential. A permit
  means the delegation covers the resource, not that a provider session exists.

### Delegation denial

A grant that does not cover the resource is a decision, not an error:

```json
{"decision":false,"context":{
  "reason_admin":{"code":"grant_missing"},
  "reason_user":{"consent_url":"https://broker.example.com/agents/550e8400-e29b-41d4-a716-446655440000"}}}
```

`code` is one of `grant_missing`, `grant_expired`, `grant_empty`,
`grant_permission_set_undeclared`, `grant_permission_set_missing`, `grant_service_omitted`,
`grant_service_definition_missing`, `grant_service_requirement_excluded` or
`grant_scope_intersection_empty`. Denials are `Cache-Control: no-store`.

### Errors

Errors are never decisions. Every error is `Cache-Control: no-store` and uses the end-user
`ErrorResponse` body with a fixed message.

| Status | `error` | When |
|---|---|---|
| 400 | `invalid_request` | Malformed or oversized body, wrong `Content-Type`, missing or unsupported member, invalid or expired subject token, `context.agent` mismatch, unregistered resource |
| 401 | `unauthorized` | Missing, invalid or expired client assertion; `WWW-Authenticate: Bearer`, with `error="invalid_token"` for an invalid credential |
| 403 | `forbidden` | The token-exchange caller policy rejects the gateway or resource |
| 500 | `server_error` | Configuration or dependency failure, such as a grant-repository outage |

## ExtProc consumer

The standalone ExtProc service can consume this endpoint before every policy decision. Enable it
with `delegation_context.enabled` (see
[ExtProc delegation context configuration](../configuration.md#extproc-delegation-context-configuration)).
ExtProc caches only permits, for at most `min(max-age, max_staleness)` from the moment it sent the
request, and fails closed: a denial returns 403 with the consent URL, and a broker failure or an
expired permit returns 503. OPA policies receive `input.context.principal`, `agent`, `resource`,
`grant` and `permission_sets` and can key on canonical IDs, for example
`input.context.permission_sets[_].canonical_id == "repo-reader"`.

## Deprecation of `granted_permission_sets`

End-user API `1.2.0` marks the token-exchange response field `granted_permission_sets` deprecated.
It remains populated, and `principal` and `agent_id` are unchanged. Removal is scheduled for one
release after the ExtProc delegation resolver ships and requires a separately approved end-user API
`2.0.0`.
