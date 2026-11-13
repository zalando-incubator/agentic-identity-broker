# Contract: ExtProc Delegation Context (configuration, broker calls, request outcomes, and policy input)

**Owner**: standalone `extproc-token-exchange` (ADR 011). It uses the `EXTPROC_` configuration
namespace and is **not** part of the broker Helm chart. **Status**: Proposed; sign-off is
approval items API-4–API-6 and ADR-1 in the plan. The maintainer endorsed the new ADR decision
set (research R16) in the 2026-10-28 review. This contract changes ADR 028's input schema and
request ordering when the feature is enabled. The input schema in §4 follows the 2026-10-28
spec clarification (FR-017, FR-018, and research R15).

## 1. Configuration

```yaml
oauth2:
  token_endpoint: https://broker.example.com/oauth2/token   # existing; its origin is the default PDP
authorization:
  enabled: true                                              # existing; required when delegation_context.enabled
delegation_context:                                          # new
  enabled: false                                             # default; true opts in
  policy_decision_point: ""                                  # default: origin of oauth2.token_endpoint
  request_timeout: 5s
  max_staleness: 60s
```

| Key | Env var | Flag | Default |
|---|---|---|---|
| `delegation_context.enabled` | `EXTPROC_DELEGATION_CONTEXT_ENABLED` | `--delegation_context.enabled` | `false` |
| `delegation_context.policy_decision_point` | `EXTPROC_DELEGATION_CONTEXT_POLICY_DECISION_POINT` | `--delegation_context.policy_decision_point` | `""` |
| `delegation_context.request_timeout` | `EXTPROC_DELEGATION_CONTEXT_REQUEST_TIMEOUT` | `--delegation_context.request_timeout` | `5s` |
| `delegation_context.max_staleness` | `EXTPROC_DELEGATION_CONTEXT_MAX_STALENESS` | `--delegation_context.max_staleness` | `60s` |

Precedence is flags, then environment, then file, then defaults (existing `loader.go` behavior).
The flag names use the same dotted form as the existing `tool_approvals.*` flags in `RegisterFlags`.

### Startup validation and errors (enabled mode only)

Messages are credential-free and name the key.

| Condition | Error (substring) |
|---|---|
| `authorization.enabled: false` | `delegation_context.enabled requires authorization.enabled` |
| `policy_decision_point` is not an absolute http(s) URL with a host, or has userinfo, query, or fragment | `delegation_context.policy_decision_point must be an absolute URL` |
| `http` scheme without `oauth2.tls.allow_http` | `delegation_context.policy_decision_point must use https` |
| `request_timeout <= 0` | `delegation_context.request_timeout must be positive` |
| `max_staleness <= 0` | `delegation_context.max_staleness must be positive` |
| Discovery transport error, non-200, invalid JSON, or missing members | `delegation context discovery failed` |
| `policy_decision_point` mismatch | `delegation context discovery: policy_decision_point mismatch (expected <expected>, advertised <advertised>)` |
| `access_evaluation_endpoint` is not an absolute URL, or uses http without `allow_http` | `delegation context discovery: invalid access_evaluation_endpoint` |

When the feature is disabled, no discovery runs, no resolver is created, and no new log lines
or requests appear (US4.1).

## 2. Broker calls

| Call | Method and URL | Credentials | Timeout |
|---|---|---|---|
| Discovery (once, at startup) | `GET <expected PDP>/.well-known/authzen-configuration` (path insertion per API-003) | **None** | `request_timeout` |
| Evaluation (per cache miss) | `POST <access_evaluation_endpoint>` | `Authorization: Bearer <client assertion>` from the existing assertion provider | `request_timeout` |

The evaluation body is `{subject:{type:jwt,id:<subject token>}, action:{name:token-exchange URN}, resource:{type:protected_resource,id:<resource uri>}}`.
`context` is omitted.

The client uses the existing traced HTTP client (`internal/extproc/httpclient`) with the
`oauth2.tls` trust settings. No new TLS settings are added.

**Freshness** (maintainer decisions of 2026-10-28, HIGH I1/U1 in [remediation.md](../remediation.md)):
The client reads `requestStart` just before it sends the POST. A permit's deadline is
`requestStart + min(max-age, max_staleness)`, computed once. The cache, the singleflight
followers, and the request state carry this deadline unchanged. ExtProc does no `Age` or
transit accounting. The maintainer chose to accept the remaining skew.

**Response handling**:

| Broker response | Resolver result |
|---|---|
| 200, `decision: true`, a `context` that passes the typed validation (data model §3.3.1), exactly one positive integer `max-age` | Permit with the typed `Context` and `Deadline`, cached until that deadline. Unknown context members are dropped. |
| 200, `decision: true`, `max-age` missing, `0`, malformed, duplicated, or not an integer | Error (`invalid_response`), so the request gets 503. The permit is **not used** and **not cached**. |
| 200, `decision: true`, a `context` that is missing a required member, has a wrong type, an empty `permission_sets`, an invalid timestamp, or an optional member that is `null` or `""` | Error (`invalid_response`), so the request gets 503. **Not used** and **not cached**. |
| 200, `decision: false` | `DeniedError{Code, ConsentURL}`. The code is allowlisted. The URL must be absolute http(s). Never cached. |
| Any other status, malformed JSON, non-boolean `decision`, or timeout | Error. Never cached. No stale fallback. |

## 3. Request outcomes (enabled mode)

| Situation | Response to the client |
|---|---|
| Delegation denied (either path) | `403`, `{"error":"access_denied","error_description":"delegation denied: <code>","error_uri":"<consent_url>"}`. Policy is not evaluated and nothing is forwarded. |
| Resolution error (either path) | `503`, `{"error":"service_unavailable","error_description":"delegation context unavailable"}` |
| Permit deadline passed before a policy evaluation (header-only policy, body policy, or any batch decision) | The same `503`. Policy is not evaluated, nothing is forwarded, and ExtProc does not re-resolve (HIGH U2). |
| Body path: permit and exchange error | Existing exchange-error handling (`exchangeErrorResponse`) |
| Body path: permit and exchange success | Buffer the body. Policy sees the context, then the existing allow, deny, or approval flow runs. |
| Header-only: permit and policy deny | Existing 403 policy denial. **No exchange call.** |
| Header-only: permit and policy allow | Exchange, then the existing exchange success or error handling |

**Precedence on the body path** (both calls complete first):

1. Delegation denial
2. Resolution error
3. Exchange error
4. Proceed

Immediately before every OPA evaluation that receives delegation input, ExtProc checks
`now < Deadline`. A permit that expires while ExtProc waits for the concurrent exchange,
buffers the body, or works through a batch therefore never reaches policy.

Metadata validation (ADR 036), protocol checks, transport checks, the body-size limit, the
approval gate, and policy fail-closed behavior are unchanged and still apply.

## 4. OPA input (`input.context`)

ExtProc maps its typed permit `Context` (data model §3.3.1) into the existing `input.context`
object. It never copies the permit verbatim. Policies see broker vocabulary, not AuthZEN
wire-format details. There is no `delegation` wrapper, no `subject` member, and no
`delegation_available` flag. Unknown permit members never appear.

**Enabled mode** (body-bearing, every batch decision, and header-only), for spec Journey 4
step 3:

```json
{
  "granted_permission_sets_available": true,
  "granted_permission_sets": {
    "770e8400-e29b-41d4-a716-446655440001": ["880e8400-e29b-41d4-a716-446655440010"]
  },
  "agent_session_id": "…",
  "principal": "alice@example.com",
  "agent": {"id": "550e8400-e29b-41d4-a716-446655440000", "canonical_id": "data-assistant"},
  "resource": {
    "id": "880e8400-e29b-41d4-a716-446655440010",
    "canonical_id": "repo-api",
    "resource_uri": "https://api.example.com/repos",
    "scopes": ["read:repo"]
  },
  "grant": {
    "id": "990e8400-e29b-41d4-a716-446655440020",
    "updated_at": "2026-10-26T09:59:00Z",
    "valid_until": "2026-10-26T10:30:00Z"
  },
  "permission_sets": [{
    "id": "770e8400-e29b-41d4-a716-446655440001",
    "canonical_id": "repo-reader",
    "services": [{
      "id": "880e8400-e29b-41d4-a716-446655440010",
      "canonical_id": "repo-api",
      "scopes": ["read:repo"]
    }]
  }]
}
```

| Member | Rule |
|---|---|
| `principal` | The permit's `subject.id` |
| `agent`, `resource`, `grant`, `permission_sets` | The documented fields of the corresponding permit members, with the received values. Timestamps are the received RFC 3339 strings. |
| `canonical_id` (all levels), `grant.valid_until` | **Omitted** when the permit omits them, never `null` |
| `scopes` (all levels) | Always an array; `[]` when empty |
| `granted_permission_sets` | `{permission_sets[].id: [permission_sets[].services[].id]}`, derived from the permit. Never taken from exchange extensions in enabled mode. Retained permanently. |
| `granted_permission_sets_available` | Always `true` in enabled mode. This is the only availability signal, and it guarantees that the five members above are present. Retained permanently. |
| `agent_session_id` | Unchanged existing behavior (omitted when absent) |

A denial (403), a resolution error (503), or an expired deadline (503) returns before policy
runs. Enabled mode therefore never produces `granted_permission_sets_available: false`.

**Disabled mode**: `principal`, `agent`, `resource`, `grant`, and `permission_sets` are
**omitted**. `granted_permission_sets_available`, `granted_permission_sets`, and
`agent_session_id` are produced exactly as today: from the exchange response on the body
path, and `false` without permission sets on the header-only path. The serialized input is
byte-for-byte unchanged.

**Compatibility**: The OPA members `granted_permission_sets` and
`granted_permission_sets_available` are permanent. FR-019 deprecates only the
`granted_permission_sets` field of the `/oauth2/token` *response*. ExtProc continues to read
that field in disabled mode until its separately approved removal.

**Example canonical-ID policy** (`tests/e2e/fixtures/policies/delegation_canonical.rego`, illustrative):

```rego
package aib.extproc.authz
default result := {"action": "deny", "reasons": ["delegation does not cover repo-api"]}
result := {"action": "allow"} if {
  c := input.context
  c.granted_permission_sets_available == true
  c.agent.canonical_id == "data-assistant"
  c.resource.canonical_id == "repo-api"
  some ps in c.permission_sets
  ps.canonical_id == "repo-reader"
}
```

## 5. Observability

The ExtProc logs and spans use the existing diagnostic vocabulary. They add these values:

- an operation value: `delegation_resolution`
- outcome details: `delegation_denied`, `delegation_unavailable`, `delegation_discovery_failed`, and
  `delegation_pdp_mismatch`
- a cache attribute: `hit` or `miss`

The subject token, the assertion, and the delegation context are never logged.
