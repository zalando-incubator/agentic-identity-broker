# Contract: ExtProc Grant Context (configuration, broker calls, request outcomes, and policy input)

**Owner**: standalone `extproc-token-exchange` (ADR 011). It uses the `EXTPROC_` configuration
namespace and is **not** part of the broker Helm chart. **Status**: Accepted under approval items
API-4–API-6 and ADR-1 (👍 on
<https://github.com/zalando-incubator/agentic-identity-broker/pull/292#issuecomment-6504077937>);
implemented in `internal/extproc/{config,grantcontext,server,authorization}` and
[ADR 048](../../../adrs/048-authzen-delegation-pip.md). The maintainer endorsed the new ADR decision
set (research R16) in the 2026-10-28 review. This contract changes ADR 028's input schema and
request ordering when the feature is enabled. The input schema in §4 follows the 2026-10-28
spec clarification (FR-017, FR-018, and research R15).

**Current approval (2026-11-05)**: supplied `context.approval` records `mjungsbluth` at
`2026-11-05T16:38:49Z` on
<https://github.com/zalando-incubator/agentic-identity-broker/pull/292#issuecomment-6661347255>.
It covers API-4–API-6 and the thirteen-item decision set, including D-5's unchanged body-path
exchange cooldown below. The Proposed wording in §3 is retained as decision history, not an
outstanding sign-off request. Quickstart §11 records the approval and validation separately.

**Approved revisions (ADR-2, API-7, D-8 and ADR-3)**: `mjungsbluth` approved them on 2026-11-06 ([ADR 048 revision history](../../../adrs/048-authzen-delegation-pip.md#revision-history)), and they are binding. §3.1 defines binding-consistent token reuse, not provider downscoping or exchange provenance; §4.1 designates the canonical field while preserving compatibility today. [Issue #441](https://github.com/zalando-incubator/agentic-identity-broker/issues/441) is the completed ADR-3 tracking prerequisite; future OPA removal remains separately approved work.

## 1. Configuration

```yaml
oauth2:
  token_endpoint: https://broker.example.com/oauth2/token   # existing; its origin is the default PDP
authorization:
  enabled: true                                              # existing; required when grant_context.enabled
grant_context:                                          # new
  enabled: false                                             # default; true opts in
  policy_decision_point: ""                                  # default: origin of oauth2.token_endpoint
  request_timeout: 5s
  max_staleness: 60s
```

| Key | Env var | Flag | Default |
|---|---|---|---|
| `grant_context.enabled` | `EXTPROC_GRANT_CONTEXT_ENABLED` | `--grant_context.enabled` | `false` |
| `grant_context.policy_decision_point` | `EXTPROC_GRANT_CONTEXT_POLICY_DECISION_POINT` | `--grant_context.policy_decision_point` | `""` |
| `grant_context.request_timeout` | `EXTPROC_GRANT_CONTEXT_REQUEST_TIMEOUT` | `--grant_context.request_timeout` | `5s` |
| `grant_context.max_staleness` | `EXTPROC_GRANT_CONTEXT_MAX_STALENESS` | `--grant_context.max_staleness` | `60s` |

Precedence is flags, then environment, then file, then defaults (existing `loader.go` behavior).
The flag names use the same dotted form as the existing `tool_approvals.*` flags in `RegisterFlags`.

### Startup validation and errors (enabled mode only)

Messages are credential-free and name the key.

| Condition | Error (substring) |
|---|---|
| `authorization.enabled: false` | `grant_context.enabled requires authorization.enabled` |
| `policy_decision_point` is not an absolute http(s) URL with a host, or has userinfo, query, or fragment | `grant_context.policy_decision_point must be an absolute URL` |
| `http` scheme without `oauth2.tls.allow_http` | `grant_context.policy_decision_point must use https` |
| `request_timeout <= 0` | `grant_context.request_timeout must be positive` |
| `max_staleness <= 0` | `grant_context.max_staleness must be positive` |
| Discovery transport error, non-200, invalid JSON, or missing members | Not fatal: retried with a backoff that starts at 1 s and doubles to a 10 s cap while the gRPC port stays closed. Each failure logs Warn `event=extproc.grant_context.discovery_failed` with `grant context discovery failed` and `retry_after` |
| `policy_decision_point` mismatch | `grant context discovery: policy_decision_point mismatch (expected <expected>, advertised <advertised>)`; the advertised value is truncated to 256 bytes and its control characters are escaped |
| `access_evaluation_endpoint` is not an absolute URL, or uses http without `allow_http` | `grant context discovery: invalid access_evaluation_endpoint` |

When the feature is disabled, no discovery runs, no resolver is created, and no new log lines
or requests appear (US4.1).

## 2. Broker calls

| Call | Method and URL | Credentials | Timeout |
|---|---|---|---|
| Discovery (at startup, retried until it succeeds) | `GET <expected PDP>/.well-known/authzen-configuration` (path insertion per API-003) | **None** | `request_timeout` |
| Evaluation (per cache miss) | `POST <access_evaluation_endpoint>` | `Authorization: Bearer <client assertion>` from the existing assertion provider; the broker rejects an assertion without a non-blank `sub` with 401 | `request_timeout` |

The evaluation body is this JSON, with `context` omitted:

```json
{"subject":{"type":"urn:ietf:params:oauth:token-type:jwt","id":"<subject token>"},
 "action":{"name":"urn:ietf:params:oauth:grant-type:token-exchange"},
 "resource":{"type":"protected_resource","id":"<resource uri>"}}
```

The client uses the existing traced HTTP client (`internal/extproc/httpclient`) with the
`oauth2.tls` trust settings. No new TLS settings are added.

**Freshness** (maintainer decisions of 2026-10-28, HIGH I1/U1 in the [2026-10-28 maintainer review on #292](https://github.com/zalando-incubator/agentic-identity-broker/pull/292#pullrequestreview-5688019903)):
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
| Resolution error (either path) | `503`, `{"error":"service_unavailable","error_description":"grant context unavailable"}` |
| Permit deadline passed before a policy evaluation (header-only policy, body policy, or any batch decision) | The same `503`. Policy is not evaluated, nothing is forwarded, and ExtProc does not re-resolve (HIGH U2). |
| Body path: permit and exchange error | Existing exchange-error handling (`exchangeErrorResponse`) |
| Body path: permit and exchange success | Apply the token binding (§3.1). Buffer the body. Policy sees the context, then the existing allow, deny, or approval flow runs. |
| Header-only: permit and policy deny | Existing 403 policy denial. **No exchange call.** |
| Header-only: permit and policy allow | Exchange under the permit's token binding (§3.1), then the existing exchange success or error handling |

**Precedence on the body path** (both calls complete first):

1. Delegation denial
2. Resolution error
3. Exchange error
4. Token binding (§3.1), then proceed

Immediately before every OPA evaluation that receives delegation input, ExtProc checks
`now < Deadline`. A permit that expires while ExtProc waits for the concurrent exchange,
buffers the body, or works through a batch therefore never reaches policy.

**Re-consent after a delegation denial (D-5)**: The resolver never caches a denial,
so both paths consult the broker again immediately after consent. Header-only requests can
permit immediately. The body path concurrently attempts exchange during the denied request;
that exchange can record the existing ADR 012 re-consent cooldown (5 s). Body-path forwarding
then resumes once that cooldown lapses, without changing exchange-cache semantics. The cooldown
entry follows the exchange cache key (subject token and resource), not the request path: on the
same ExtProc instance, a header-only request for that pair gets a fresh permit but then receives
the cached exchange error after policy allows, until the cooldown lapses. A header-only denial
alone creates no cooldown, because header-only requests exchange only after allow. This is
not a delegation negative cache. Rationale: preserve the existing exchange cache while
bounding delegation freshness independently. Alternative: bypass the exchange cooldown in
enabled mode, which changes ADR 012 semantics. D-5 records the approval (2026-11-05).

### 3.1 Permit-bound exchanged tokens (ADR-2, [ADR 048 §10](../../../adrs/048-authzen-delegation-pip.md#10-the-permit-governs-enabled-mode-token-reuse))

The permit governs local policy and token-result reuse. It does not prove provider-token scopes or the grant version read by exchange. A binding is the permit principal, agent UUID, resource scope set and grant update instant that first claimed a result handle. Exchange response identities must match the permit before any claim; mismatch or absence returns terminal API-7 503 without an identity-repair retry.

**Binding-aware port/result**: retain disabled `Exchange` semantics. The body path's initial exchange returns the credential plus an opaque result handle (entry identity and synchronized binding cell); the enabled binding-aware call accepts that observed handle and the desired `ExchangeResultBinding`. Header-only supplies no initial handle and exchanges after allow. Both return either a validated compatible handle with original token/identities/extensions or a typed exchange/binding error. Every successful flight, including a non-cacheable success, creates one handle shared by all followers. New installations have distinct identities even for identical token bytes. Atomic `BindOnce` claims an unbound cell; subsequent callers compare, never rebind. The exchanger never parses permits. Caller/leader status confers no exclusive ownership of a shared result.

| Situation (after body precedence, or header policy allow) | Behavior |
|---|---|
| Exchange principal or agent UUID differs from permit, or either exchange field is absent | Terminal API-7 503; no handle claim, approval lookup/creation, forwarding or identity-repair retry. |
| Unbound handle with matching exchange identity, whether cached, initial exchange, detached or singleflight delivery | Atomically claim with this permit. If another caller wins, compare its binding using the rows below; never overwrite. |
| Handle matches principal, agent UUID, scope set and grant instant | Use that result; cache reuse adds no broker call. |
| Initial handle mismatches | Never forward it; compare-and-delete the observed entry identity only. |
| Delete succeeds or observed entry is absent | Perform at most one additional exchange, with stale-on-error forbidden for removed/mismatched results. |
| Delete fails because a replacement exists | Preserve and read the replacement once. Compare/claim its handle; use it if compatible, otherwise perform the one additional exchange. Never delete by token bytes, including equal-token/different-binding replacements. |
| Concurrent permits share the additional exchange | All receive the same handle. First atomic claim wins; equal-binding followers may use it. Different-binding followers take the terminal conflict row, not another exchange. |
| Additional exchange delivers a mismatched handle, including one bound by another live permit | **API-7**: return `503`, `{"error":"service_unavailable","error_description":"exchange result binding unavailable"}`. Nothing forwarded; no further retry, rebind, competing-entry deletion or fallback. |
| Additional exchange fails (timeout, cancellation, broker error or breaker rejection) | Existing `exchangeErrorResponse`; nothing forwarded, no removed/mismatched stale fallback. |
| Permit expires before policy use or final forwarding | Existing `503 service_unavailable` / `grant context unavailable`; no expired-context forwarding or re-resolution. |
| Cooldown entry (no token) | Unchanged D-5; no binding. |

Body validation precedes buffering and Authorization mutation; header validation uses the exact permit policy evaluated. Check deadlines immediately before every policy decision, the approval gate and final forwarding, including expiry during header exchange or retry. A body the approval gate grants is forwarded without a further deadline check (ADR 048 §10). Each request has at most one additional exchange; revalidating its result cannot restart the loop. Disabled cache keys, TTLs, singleflight and cooldown remain unchanged. Successful non-cacheable results follow identical claim/compare and terminal rules.

**Guarantee and limitations (D-8)**: a request never forwards a handle bound to a different permit scope set/grant instant. It may forward identical broader provider token bytes under a new handle when the provider session is unchanged. First use does not attest exchange provenance; concurrent grant reads can differ. Only provider-session re-authorization in Journey 5 demonstrates upstream narrowing. Stronger broker downscoping or exchange-version response members require separately approved API/ADR scope.

The log-only Debug diagnostic remains `grant_context_binding=mismatch`, without credentials, scopes or principal.

Metadata validation (ADR 036), protocol checks, transport checks, the body-size limit, the
approval controls, and policy fail-closed behavior still apply. In enabled mode, the approval
gate principal and agent UUID come only from the exchange result under ADR 036, after
checking equality with the permit identities (approved F1/P1 on [the 2026-11-11 proposal](https://github.com/zalando-incubator/agentic-identity-broker/pull/292#issuecomment-6751402783)). Disabled-mode identity and session extraction are unchanged.

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
    "service_id": "880e8400-e29b-41d4-a716-446655440010",
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
| `granted_permission_sets` | `{permission_sets[].id: [permission_sets[].services[].id]}`, derived from the permit. Never taken from exchange extensions in enabled mode. Retained in this increment; see §4.1. |
| `granted_permission_sets_available` | Always `true` in enabled mode. This is the only availability signal, and it guarantees that the five members above are present. Retained in this increment; see §4.1. |
| `agent_session_id` | Unchanged existing behavior (omitted when absent) |

A denial (403), a resolution error (503), or an expired deadline (503) returns before policy
runs. Enabled mode therefore never produces `granted_permission_sets_available: false`.

**Disabled mode**: `principal`, `agent`, `resource`, `grant`, and `permission_sets` are
**omitted**. `granted_permission_sets_available`, `granted_permission_sets`, and
`agent_session_id` are produced exactly as today: from the exchange response on the body
path, and `false` without permission sets on the header-only path. The serialized input is
byte-for-byte unchanged.

### 4.1 Canonical permission-set field and compatibility boundary (ADR-3)

`input.context.permission_sets` is the canonical enabled-mode permission-set field for new policies. It carries typed IDs, canonical IDs and included-service contributions; `resource.scopes` remains the effective scope ceiling. `granted_permission_sets` and `granted_permission_sets_available` remain permit-derived compatibility inputs on both enabled paths, with exactly today's shape and values. This designation changes guidance, not serialization, availability, disabled behavior or policy execution.

The previous permanent-retention commitment is revised only to allow a future, separately approved removal; this feature does not remove or deprecate either OPA member. ADR-3 records the rationale and alternative. The maintainer-created [issue #441](https://github.com/zalando-incubator/agentic-identity-broker/issues/441) satisfied T104's prerequisite; it does not permit removal here. The issue covers OPA policy migration, enabled/disabled behavior, availability replacement and versioned contract approval. FR-019's separate exchange-response deprecation is unchanged; ExtProc still reads that response in disabled mode until its separately approved removal.

**Example canonical-ID policy** (`tests/e2e/fixtures/policies/grant_context_canonical.rego`, illustrative):

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

- an operation value: `grant_resolution`
- outcome details: `grant_context_denied`, `grant_context_unavailable`, `grant_context_discovery_failed`, and
  `grant_context_pdp_mismatch`
- a cache attribute: `hit` or `miss`

The subject token, the assertion, and the grant context are never logged.

Implementation signals: `extproc.grant_context.resolve` (Internal) and
`extproc.grant_context.evaluate` (Client), both in scope `extproc`, carry
`token_exchange.operation=grant_resolution`, `token_exchange.outcome` and
`token_exchange.failure_detail`. Outcome/detail pairs are `success`/`none`,
`authorization_denied`/`grant_context_denied`, `infrastructure_error`/`grant_context_unavailable`,
and `canceled`/`caller_canceled`. The resolver span adds `grant_context.cache=hit|miss`.
Infrastructure failures use a constant error status description and `error.type` from
`timeout|unavailable|rejected|invalid_response`; denials do not set error status. No raw error,
credential, principal, context, or requested resource URI is exported by these spans.
Server logs classify the same resolution outcomes; permit hit/miss diagnostics are Debug.
Security records carry `event=extproc.grant_context.denied` for a delegation denial,
`event=extproc.grant_context.unavailable` for resolution failure, and
`event=extproc.grant_context.permit_expired` when the permit expires before policy use.
The last two retain the `grant_context_unavailable` detail. These identifiers follow the
`extproc.grant_context.enabled` lifecycle family (F6, approved remediation proposal). A
failed startup discovery that will be retried logs the Warn record
`event=extproc.grant_context.discovery_failed`, and a shutdown signal during startup discovery
or its retry wait logs the Info lifecycle record
`event=extproc.grant_context.discovery_interrupted`.
