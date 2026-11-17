# Contract: ExtProc Delegation Context (configuration, broker calls, request outcomes, and policy input)

**Owner**: standalone `extproc-token-exchange` (ADR 011). It uses the `EXTPROC_` configuration
namespace and is **not** part of the broker Helm chart. The proposed ADR (research R16) must be
Accepted before implementation, because this contract changes ADR 028's input schema and
request ordering when the feature is enabled.

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

**Blocked freshness contract:** The missing/invalid `max-age` row below contradicts research
R13; the relative lifetime also omits response-age/transit accounting and expiry at delayed
policy use. HIGH I1/U1/U2 in [remediation.md](../remediation.md) require stakeholder decisions
and synchronized contract/model/journey/test updates. Neither conflicting interpretation is
selected here; do not implement these rows or stored-context policy use until reconciled.

**Response handling**:

| Broker response | Resolver result |
|---|---|
| 200, `decision: true`, object `context`, `Cache-Control` with valid `max-age` | Permit, cached for `min(max-age, max_staleness)` (not cached when `max-age=0`) |
| 200, `decision: true`, missing or invalid `max-age` | Permit, used for this request only and **not cached** |
| 200, `decision: false` | `DeniedError{Code, ConsentURL}`. The code is allowlisted. The URL must be absolute http(s). Never cached. |
| Any other status, malformed JSON, non-boolean `decision`, or timeout | Error. Never cached. No stale fallback. |

## 3. Request outcomes (enabled mode)

| Situation | Response to the client |
|---|---|
| Delegation denied (either path) | `403`, `{"error":"access_denied","error_description":"delegation denied: <code>","error_uri":"<consent_url>"}`. Policy is not evaluated and nothing is forwarded. |
| Resolution error (either path) | `503`, `{"error":"service_unavailable","error_description":"delegation context unavailable"}` |
| Body path: permit and exchange error | Existing exchange-error handling (`exchangeErrorResponse`) |
| Body path: permit and exchange success | Buffer the body. Policy sees the context, then the existing allow, deny, or approval flow runs. |
| Header-only: permit and policy deny | Existing 403 policy denial. **No exchange call.** |
| Header-only: permit and policy allow | Exchange, then the existing exchange success or error handling |

**Precedence on the body path** (both calls complete first):

1. Delegation denial
2. Resolution error
3. Exchange error
4. Proceed

Metadata validation (ADR 036), protocol checks, transport checks, the body-size limit, the
approval gate, and policy fail-closed behavior are unchanged and still apply.

## 4. OPA input (`input.context`)

**Enabled mode**:

```json
{
  "granted_permission_sets_available": true,
  "granted_permission_sets": {"770e8400-e29b-41d4-a716-446655440001": ["880e8400-e29b-41d4-a716-446655440010"]},
  "agent_session_id": "…",
  "delegation_available": true,
  "delegation": { "subject": {"id": "alice@example.com"}, "agent": {"id": "550e8400-…", "canonical_id": "data-assistant"}, "resource": {"…": "…"}, "grant": {"…": "…"}, "permission_sets": ["…"] }
}
```

- `delegation` is the broker permit's `context` object, verbatim, with numbers kept as
  `json.Number`.
- `granted_permission_sets` is derived from `delegation.permission_sets[].{id, services[].id}`.
  It is never taken from exchange extensions when the feature is enabled.
- `delegation_available` is never `false` in enabled mode. A failure rejects the request
  before policy runs.

**Disabled mode**: `delegation` and `delegation_available` are **omitted**. All other members
are produced exactly as today.

**Example canonical-ID policy** (`tests/e2e/fixtures/policies/delegation_canonical.rego`, illustrative):

```rego
package aib.extproc.authz
default result := {"action": "deny", "reasons": ["delegation does not cover repo-api"]}
result := {"action": "allow"} if {
  d := input.context.delegation
  d.agent.canonical_id == "data-assistant"
  d.resource.canonical_id == "repo-api"
  some ps in d.permission_sets
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
