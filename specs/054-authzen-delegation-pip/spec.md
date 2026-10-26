# Feature Specification: AuthZEN Delegation Policy Information Point

**Feature Branch**: `speckit/issue-289`

**Created**: 2026-10-26

**Status**: Draft — needs clarification before contract approval and planning

**Input**: The title and body supplied for [issue #289](https://github.com/zalando-incubator/agentic-identity-broker/issues/289): “Broker AuthZEN Access Evaluation endpoint as Delegation Policy Information Point”. No issue comments were used.

## User Scenarios & Testing *(mandatory)*

The broker tells an authenticated gateway whether a user's delegation to an agent covers one protected resource. It returns credential-free policy information, not a provider credential or a final decision about a tool invocation. ExtProc can opt into this information independently of its exchanged-token lifetime, so revocations stop informing policy within a bounded freshness window and header-only requests receive the same information as body-bearing requests.

Scenario identifiers below are stable acceptance-test references. Each numbered scenario MUST map to exactly one complete-journey E2E test in `tests/e2e/` (broker) or `tests/e2e/extproc/` (consumer). The denial matrix rows are separate scenarios, not one combined test. E2E tests must compile and fail semantically before implementation; this specification phase creates no tests or implementation.

### User Story 1 - A gateway obtains current delegation information (Priority: P1)

As a gateway operator, I want to discover the broker's evaluation endpoint and obtain verified delegation information without retrieving the user's provider credentials, so that my policies can use current grants and recognizable identifiers.

**Why this priority**: This is the independently useful broker increment and enables other AuthZEN-capable gateways, not only ExtProc.

**Independent Test**: Register an agent and protected resource, establish a user's grant, and use an authorized gateway assertion to discover and evaluate that delegation through the real end-user server. No ExtProc deployment or provider session is needed.

**Acceptance Scenarios**:

1. **US1.1 — Discovery**: **Given** token exchange is configured and the public broker URL is `https://broker.example.com`, **When** a gateway reads `/.well-known/authzen-configuration` on the end-user server, **Then** it receives `policy_decision_point: https://broker.example.com` and `access_evaluation_endpoint: https://broker.example.com/access/v1/evaluation`, derived from the same public base used for consent links and RFC 8414 issuer metadata.
2. **US1.2 — Permit**: **Given** Alice's valid subject token identifies a registered agent, the gateway is authenticated and authorized, and Alice's active grant covers the resource, **When** the gateway evaluates it with `X-Request-ID: delegation-001`, **Then** it receives HTTP 200 with `decision: true`, the credential-free Delegation Context described in FR-007, the same request ID, and a private cache lifetime of at most 60 seconds.
3. **US1.3 — Delegation is not credential availability**: **Given** the same covered delegation but no usable third-party provider session, **When** the gateway evaluates it and separately attempts token exchange, **Then** evaluation still permits without accessing or refreshing a provider credential, while exchange retains its existing session error behavior.
4. **US1.4 — Effective scope ceiling**: **Given** the granted permission sets provide `read:repo` and `write:repo` but the agent's service requirement permits only `read:repo`, **When** the gateway evaluates the resource, **Then** `resource.scopes` contains only `read:repo`, and exchanging for that resource continues to apply the same scope ceiling.
5. **US1.5 — Resource normalization**: **Given** a registered resource URI and a valid covering grant, **When** the gateway evaluates an equivalent URI under existing token-exchange normalization rules, **Then** it obtains the same service identity and delegation decision as token exchange, not a new resource-resolution rule.
6. **US1.6 — Grant expiry bounds caching**: **Given** a covering grant expires in 15 whole seconds, **When** the gateway evaluates it, **Then** the permit advertises at most 15 seconds of freshness, and evaluation at expiry returns the expired-grant denial rather than a permit.
7. **US1.7 — No new broker switch**: **Given** a broker deployment without configured token exchange, **When** a gateway attempts discovery and evaluation, **Then** neither route provides a usable AuthZEN service; configuring token exchange makes both available on the end-user server without an additional AuthZEN configuration flag.
8. **US1.8 — Optional canonical metadata**: **Given** an otherwise valid delegation whose agent, service, or permission set has no canonical ID, **When** it is evaluated, **Then** the response follows the stakeholder-selected missing-canonical-ID contract in FR-008; this scenario cannot be finalized until Q2 is answered.

### User Story 2 - A gateway distinguishes delegation denial from request failure (Priority: P1)

As a gateway operator, I want trustworthy denial reasons and consent recovery information, while malformed requests, invalid credentials, and service failures remain distinguishable from delegation decisions.

**Why this priority**: Returning a false permit or hiding an infrastructure failure as an ordinary denial would undermine policy enforcement and recovery.

**Independent Test**: Start with a known permitted delegation, change one condition per scenario, and evaluate it through the end-user server. Verify the status, bounded diagnostic, request-ID echo, cache headers, and absence of credentials.

**Acceptance Scenarios — delegation denials**:

Each row has the common **Given** of an authorized, authenticated gateway, a valid subject token, and a resolved registered agent and service. **When** that gateway evaluates the resource, **Then** HTTP 200 contains `decision: false`, the listed existing `FailureDetail` in the administrative reason, the resolved agent's consent URL in the user reason, `Cache-Control: no-store`, and no permit context or credentials.

| Scenario | Additional Given | Administrative reason |
| --- | --- | --- |
| US2.1 | Alice has no grant for the agent | `grant_missing` |
| US2.2 | Alice revoked the previously permitted grant | `grant_missing` (current revocation removes the active grant) |
| US2.3 | Alice's grant has expired | `grant_expired` |
| US2.4 | A stored grant has no permission-set entries | `grant_empty` |
| US2.5 | The grant references a permission set no longer declared by the agent | `grant_permission_set_undeclared` |
| US2.6 | A granted permission set no longer exists | `grant_permission_set_missing` |
| US2.7 | The requested service is omitted from the grant's included services | `grant_service_omitted` |
| US2.8 | The grant includes the service but its permission-set service definition is missing | `grant_service_definition_missing` |
| US2.9 | The requested service is excluded by the agent's service requirements | `grant_service_requirement_excluded` |
| US2.10 | Nonempty granted scopes have an empty intersection with the agent's scope ceiling | `grant_scope_intersection_empty` |

**Acceptance Scenarios — request errors**:

11. **US2.11 — Invalid subject**: **Given** an authorized gateway, **When** it evaluates a subject token with an invalid signature, **Then** it receives HTTP 400, no delegation permit, no credentials, and its request ID echoed.
12. **US2.12 — Expired subject**: **Given** an authorized gateway, **When** it evaluates an expired subject token, **Then** it receives HTTP 400 rather than an ordinary delegation denial.
13. **US2.13 — Unsupported subject type**: **Given** an authorized gateway, **When** it supplies an asserted `identity` subject instead of the JWT token-type URI, **Then** it receives HTTP 400; asserted identity never substitutes for subject-token validation.
14. **US2.14 — Invalid request shape**: **Given** an authorized gateway, **When** it submits malformed JSON, **Then** it receives HTTP 400 without attempting to resolve delegation. Required-field, unsupported-action, and unsupported-resource-type variants are also covered by request validation tests.
15. **US2.15 — Missing gateway credential**: **Given** a valid subject, **When** evaluation has no bearer client assertion, **Then** it receives HTTP 401 without exposing grant information.
16. **US2.16 — Invalid gateway credential**: **Given** a valid subject, **When** the bearer assertion is expired or fails its configured trust validation, **Then** it receives HTTP 401 without exposing grant information.
17. **US2.17 — Unauthorized gateway**: **Given** valid subject and assertion credentials but a caller policy that rejects `request.grant_type = "authzen_evaluation"`, **When** evaluation is requested, **Then** it receives HTTP 403, not HTTP 200 with a delegation-denial reason.
18. **US2.18 — Agent mismatch**: **Given** the token resolves to the Data Assistant, **When** the request anchors `context.agent` to another registered agent, **Then** evaluation returns HTTP 400 and never evaluates the other agent's grant.
19. **US2.19 — Dependency failure**: **Given** valid credentials and a grant-repository outage, **When** the gateway evaluates the delegation, **Then** it receives HTTP 500 without a permit, cached success, or fabricated consent-recovery decision.
20. **US2.20 — Unregistered resource**: **Given** valid credentials, **When** the gateway evaluates an unregistered protected-resource URI, **Then** it receives HTTP 400 without pretending a service grant exists.

### User Story 3 - ExtProc supplies fresh context before every policy decision (Priority: P2)

As an operator who enables delegation context, I want both request paths to evaluate policy against a bounded-freshness Delegation Context, so a token cached for an hour does not keep supplying revoked permissions for an hour.

**Why this priority**: This consumes the additive broker capability and fixes lifetime and ordering coupling without forcing existing ExtProc deployments to migrate immediately.

**Independent Test**: Enable the resolver with a broker and policy fixture. Send metadata-validated body-bearing and header-only requests through the real ExtProc service; inspect the downstream result and policy-observed canonical IDs. Use controlled times for cache expiry.

**Acceptance Scenarios**:

1. **US3.1 — Body-bearing permit**: **Given** enabled delegation context and an allowed delegation, **When** a body-bearing request reaches ExtProc, **Then** exchange and delegation resolution run concurrently during request headers, both must succeed before the request proceeds to body policy evaluation, and policy receives the resolved context verbatim with `delegation_available: true` before any downstream forwarding.
2. **US3.2 — Header-only permit**: **Given** the same delegation, **When** a supported header-only request reaches ExtProc, **Then** it resolves delegation before policy evaluation, policy sees the same delegation information, and exchange occurs only after policy allows.
3. **US3.3 — Delegation denial**: **Given** the broker returns `decision: false` with Alice's consent URL, **When** ExtProc handles the request, **Then** it returns HTTP 403 `access_denied` with that recovery URL before policy evaluation or downstream forwarding; a concurrent successful exchange cannot override the denial.
4. **US3.4 — Independent freshness**: **Given** a permit cached at 10:00:00 for 60 seconds and an exchanged token usable until 11:00:00, **When** Alice revokes her grant at 10:00:10 and repeats her request at 10:01:00, **Then** ExtProc resolves again and denies instead of feeding the hour-old permission snapshot to policy.
5. **US3.5 — Denials not cached**: **Given** a denied request and subsequent valid user consent, **When** Alice repeats the request immediately, **Then** ExtProc consults the broker again and can permit without waiting for a negative-cache lifetime.
6. **US3.6 — No stale-on-error**: **Given** an expired delegation permit but a still-cached exchanged token, **When** resolution times out or the broker is unavailable, **Then** ExtProc fails closed and does not substitute stale delegation, exchange-response permission sets, or an availability-false input that policy could allow.
7. **US3.7 — Cache isolation and local cap**: **Given** two different subject tokens or resource URIs, and `max_staleness: 20s`, **When** a 60-second permit is cached for one pair, **Then** another pair cannot use it and the original pair requires revalidation after at most 20 seconds.
8. **US3.8 — Canonical policy and compatibility projection**: **Given** a valid context and a policy keyed on canonical IDs rather than deployment UUIDs, **When** equivalent configured entities use different UUIDs, **Then** policy makes the same decision, the context remains unchanged, and legacy `granted_permission_sets*` inputs during the deprecation window are derived from that context, not the exchange response.
9. **US3.9 — Policy denial still wins**: **Given** a permitted delegation but a header-only request forbidden by policy, **When** ExtProc evaluates it, **Then** it denies without invoking exchange or forwarding downstream; delegation permission is not tool-call permission.
10. **US3.10 — Exchange failure still wins**: **Given** a permitted delegation but an unusable provider session, **When** a body-bearing request triggers concurrent exchange and resolution, **Then** exchange failure prevents forwarding despite successful delegation resolution and preserves existing exchange-error handling.

### User Story 4 - Operators roll out discovery and migrate safely (Priority: P2)

As an operator, I want startup to reject discovery mix-ups and invalid opt-in settings, while disabled consumers and existing token-exchange identities remain compatible.

**Why this priority**: A gateway must not silently send a subject credential to a mismatched service, and operators need a deliberate migration window.

**Independent Test**: Start ExtProc with each configuration or metadata condition and observe readiness or rejection; compare disabled-mode behavior with today's exchange and policy behavior.

**Acceptance Scenarios**:

1. **US4.1 — Disabled by default**: **Given** an existing ExtProc configuration with no delegation-context block, **When** it starts and serves requests, **Then** it performs no AuthZEN discovery or resolution and retains existing token-exchange and policy behavior.
2. **US4.2 — Authorization dependency**: **Given** `delegation_context.enabled: true` and `authorization.enabled: false`, **When** ExtProc starts, **Then** startup fails with an actionable, credential-free configuration error.
3. **US4.3 — Discovery failure**: **Given** delegation resolution is enabled but metadata is unavailable or invalid, **When** ExtProc starts, **Then** startup fails rather than guessing an evaluation endpoint or silently disabling the resolver.
4. **US4.4 — Metadata mix-up**: **Given** the expected policy-decision-point identifier is `https://broker.example.com` but discovery advertises `https://other.example.com`, **When** ExtProc starts, **Then** it fails without sending any evaluation credentials to the mismatched service.
5. **US4.5 — Default discovery target**: **Given** delegation resolution is enabled, `oauth2.token_endpoint` is `https://broker.example.com/oauth2/token`, and no policy-decision-point override is supplied, **When** ExtProc starts, **Then** it discovers from `https://broker.example.com/.well-known/authzen-configuration`, verifies the identifier, and uses the advertised evaluation endpoint.
6. **US4.6 — Deprecation window**: **Given** the resolver-shipping release, **When** existing clients exchange a token, **Then** `granted_permission_sets` remains available but documented as deprecated, and `principal` and `agent_id` remain unchanged; removal of the deprecated field is scheduled for one release after the resolver ships, not the additive broker-only release.

### Edge Cases

- A grant with no `valid_until` is still bounded by the 60-second permit cap; a grant with less than one second remaining must not be rounded up into a reusable permit. At expiry it denies.
- Canonical IDs are optional, case-sensitive metadata today, not replacements for UUID identity. No synthetic canonical alias or UUID-as-canonical-ID fallback is authorized by this draft (Q2).
- `context.agent` cannot override a verified subject-token claim. Omission behavior remains Q1; mismatches always fail.
- Resource normalization, multiple permission-set scope union, included-service filtering, `require_all_scopes`, service-requirement exclusion, and empty-intersection rules remain identical to exchange. Declared zero-scope services must retain their existing distinction from scopes capped to zero.
- Unknown or invalid agent identity and ambiguous resource registration are configuration failures, not evidence of user non-consent. Dependency/configuration errors must not be disguised as the grant-denial matrix.
- Missing, unsupported, or invalid subject/resource/action values are request failures; arbitrary asserted `identity` subjects are explicitly unsupported.
- Both permit and deny decisions echo a supplied `X-Request-ID`; request errors must also echo it. Existing trace correlation remains separate and unchanged. No new request-ID generation requirement is introduced.
- Missing/invalid cache freshness must not extend the consumer's freshness allowance; denies and request errors are not reusable permits. Expired delegation cannot use the token cache's outage fallback.
- A public broker identifier with a path prefix may differ from the origin default. Operators must set the explicit policy-decision-point identifier; comparison must not strip a path or otherwise turn a mismatch into a match.
- Both concurrent operations must complete successfully before the body path proceeds. Cancellation or completion order cannot turn a deny/error into a forwarded request.
- Existing strict gateway JWT validation, metadata-only token/resource inputs, MCP transport checks, body limits, approval-gate behavior, and policy fail-closed behavior remain in force.

## Concrete Example Journeys *(mandatory)*

All identifiers, timestamps, token markers, and claims below are synthetic. Proposed new HTTP representations are review examples, not an approved OpenAPI amendment. The eventual contract must verify AuthZEN 1.0 reason-envelope details before freezing the exact schema. Existing token-exchange and consent contracts are not changed by this draft.

**Shared cast**:

| Concept | Synthetic value |
| --- | --- |
| Principal | `alice@example.com` |
| Agent | Data Assistant, UUID `550e8400-e29b-41d4-a716-446655440000`, canonical ID `data-assistant` |
| Service | Repository API, UUID `880e8400-e29b-41d4-a716-446655440010`, canonical ID `repo-api` |
| Protected resource | `https://api.example.com/repos` |
| Permission set | Repository Reader, UUID `770e8400-e29b-41d4-a716-446655440001`, canonical ID `repo-reader` |
| Grant | UUID `990e8400-e29b-41d4-a716-446655440020`; updated `2026-10-26T09:59:00Z`; expires `2026-10-26T10:30:00Z` |
| Gateway assertion subject | `gateway-prod` |
| Subject token marker | `SYNTHETIC_SUBJECT_JWT` |
| Assertion marker | `SYNTHETIC_CLIENT_ASSERTION_JWT` |

### Journey 1 — Discover and permit a delegation (User Story 1, US1.1–US1.6)

**Step 1 — Before 10:00:00: operator enables the existing broker exchange capability**

The broker already has trusted keys, a registered agent and service, and Alice's grant for the reader permission set including Repository API. Its existing configuration includes this excerpt (other required settings omitted):

```yaml
server:
  enduser:
    public_url: https://broker.example.com # public discovery identifier and consent-link base
```

The applicable claim-extraction and caller-policy excerpt is:

```yaml
token_exchange:
  claim_extraction:
    principal_expression: subject_token.sub # Alice is the principal
    agent_id_expression: subject_token.agent_id # internal Agent UUID, not a client ID
  authorization:
    type: cel
    cel:
      expression: 'client_assertion.sub == "gateway-prod" && request.grant_type in ["authzen_evaluation", "urn:ietf:params:oauth:grant-type:token-exchange"]'
```

No broker `authzen.enabled` setting is introduced. Subject and assertion trust are configured through the existing, separate trust paths. The subject marker represents a signature-verified token with decoded claims:

```json
{
  "header": {"alg": "ES256", "kid": "subject-demo"},
  "payload": {
    "iss": "https://subject-idp.example.com",
    "sub": "alice@example.com",
    "aud": "https://broker.example.com",
    "iat": 1793008500,
    "exp": 1793012400,
    "agent_id": "550e8400-e29b-41d4-a716-446655440000"
  }
}
```

`iat` is 09:55:00 and `exp` is 11:00:00 on 2026-10-26 UTC. The assertion marker represents a separately signature-verified credential with issuer `https://gateway-idp.example.com`, subject `gateway-prod`, audience `https://broker.example.com`, the same example issue/expiry times, and header `{"alg":"ES256","kid":"gateway-demo"}`. These are illustrative claims, not a change to either credential's configured validation profile. Unsigned tokens are never accepted by this evaluation flow.

**Step 2 — 10:00:00: the gateway discovers the service**

```http
GET /.well-known/authzen-configuration HTTP/1.1
Host: broker.example.com

HTTP/1.1 200 OK
Content-Type: application/json

{
  "policy_decision_point": "https://broker.example.com",
  "access_evaluation_endpoint": "https://broker.example.com/access/v1/evaluation"
}
```

**Step 3 — 10:00:00: the gateway evaluates one subject and resource**

```http
POST /access/v1/evaluation HTTP/1.1
Host: broker.example.com
Authorization: Bearer SYNTHETIC_CLIENT_ASSERTION_JWT
Content-Type: application/json
X-Request-ID: delegation-001

{
  "subject": {"type": "urn:ietf:params:oauth:token-type:jwt", "id": "SYNTHETIC_SUBJECT_JWT"},
  "action": {"name": "urn:ietf:params:oauth:grant-type:token-exchange"},
  "resource": {"type": "protected_resource", "id": "https://api.example.com/repos"},
  "context": {"agent": "550e8400-e29b-41d4-a716-446655440000"}
}

HTTP/1.1 200 OK
Content-Type: application/json
X-Request-ID: delegation-001
Cache-Control: private, max-age=60

{
  "decision": true,
  "context": {
    "subject": {"id": "alice@example.com"},
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
}
```

`resource.id` identifies the resolved service, not the protected-resource child record. The example uses a string Agent UUID anchor and a minimal principal subject object as proposed contract representations; these require explicit contract review. The response subject is the verified principal, never the original JWT. All examples include an anchor and configured canonical IDs, so they do not preempt Q1 or Q2.

**Step 4 — Expiry variant: 10:29:45**

The same valid delegation returns the same entities with `Cache-Control: private, max-age=15`. At 10:30:00, evaluation returns `decision: false` with `grant_expired`. With no provider session, step 3 still permits; a subsequent `/oauth2/token` exchange fails with its existing session-recovery error instead of returning a provider credential.

**Observable result**: The gateway knows Alice delegated `read:repo` to `data-assistant` for `repo-api`, without receiving, decrypting, refreshing, or depending on a provider token.

### Journey 2 — Revocation yields consent recovery, not a reusable denial (User Story 2, US2.1–US2.10; User Story 3, US3.4–US3.5)

**Step 1 — 10:00:00: ExtProc obtains Journey 1's permit**

The exchanged token remains usable in its separate cache until 11:00:00. The delegation permit is fresh only until 10:01:00.

**Step 2 — 10:00:10: Alice revokes her delegation through the existing consent journey**

The active grant is removed. This feature changes no consent screen or revocation API.

**Step 3 — 10:01:00: the gateway repeats Journey 1's evaluation request with `X-Request-ID: delegation-002`**

Proposed denial representation, subject to AuthZEN reason-envelope verification in API-003:

```http
HTTP/1.1 200 OK
Content-Type: application/json
X-Request-ID: delegation-002
Cache-Control: no-store

{
  "decision": false,
  "context": {
    "reason_admin": {"code": "grant_missing"},
    "reason_user": {"consent_url": "https://broker.example.com/agents/550e8400-e29b-41d4-a716-446655440000"}
  }
}
```

Every other denial-matrix condition substitutes its listed reason without changing the decision/status/cache semantics. The administrative reason conveys a bounded diagnostic, not internal error text.

**Step 4 — Alice grants access again, then immediately repeats the request**

Because denials are not cached, the gateway consults the broker and can obtain Journey 1's permit again. No negative-cache wait is imposed.

**Observable result**: Revocation stops supplying delegation to policy by 10:01:00 despite a usable cached provider token; restored consent is recognized on the next evaluation.

### Journey 3 — An unauthorized or malformed evaluation never becomes a delegation decision (User Story 2, US2.11–US2.20)

**Step 1 — The gateway submits Journey 1's request without `Authorization`**

```http
POST /access/v1/evaluation HTTP/1.1
Host: broker.example.com
Content-Type: application/json
X-Request-ID: delegation-003

{
  "subject": {"type": "urn:ietf:params:oauth:token-type:jwt", "id": "SYNTHETIC_SUBJECT_JWT"},
  "action": {"name": "urn:ietf:params:oauth:grant-type:token-exchange"},
  "resource": {"type": "protected_resource", "id": "https://api.example.com/repos"},
  "context": {"agent": "550e8400-e29b-41d4-a716-446655440000"}
}

HTTP/1.1 401 Unauthorized
X-Request-ID: delegation-003
Cache-Control: no-store
```

The request-error body and applicable authentication challenge must be specified in the reviewed OpenAPI design; this status/header excerpt intentionally makes no new OAuth error-body promise.

**Step 2 — The gateway corrects its assertion but submits `subject.type: identity`**

Keeping the other request fields unchanged yields HTTP 400 with `X-Request-ID: delegation-003` and no permit. A valid signed but expired subject likewise yields 400; an invalid or expired gateway assertion yields 401; a valid gateway rejected by caller policy yields 403; a grant-repository outage yields 500. Those errors are not transformed into HTTP 200 `decision: false` responses.

**Step 3 — The gateway supplies valid credentials but changes the anchor**

`context.agent: 550e8400-e29b-41d4-a716-446655440099` conflicts with the verified token's agent and yields 400 without reading the other agent's grant. Returning to the original request obtains Journey 1's permit.

**Observable result**: The gateway can repair credentials or requests, distinguish an outage from non-consent, and never obtain delegation information by supplying an asserted identity or a different agent anchor.

### Journey 4 — An operator enables fresh context for body and header paths (User Story 3, US3.1–US3.3, US3.6–US3.10; User Story 4, US4.1–US4.6)

**Step 1 — Operator prepares ExtProc configuration**

Existing keys and proposed new keys are distinguished explicitly. Other required existing OAuth2 credentials, TLS trust, and policy configuration are omitted, not replaced with real secrets.

```yaml
oauth2:
  token_endpoint: https://broker.example.com/oauth2/token # existing key; origin is the discovery default
authorization:
  enabled: true # existing key; required for delegation_context.enabled
  policy:
    path: /etc/extproc/policy.rego # existing key; installed policy uses canonical IDs
delegation_context: # proposed new ExtProc configuration, not broker configuration
  enabled: true
  policy_decision_point: https://broker.example.com # may be omitted for the origin default
  request_timeout: 5s
  max_staleness: 60s # permit freshness is also bounded by the broker's max-age
```

These settings belong to the standalone ExtProc configuration path and `EXTPROC_` namespace, not the broker chart. With `enabled` absent/false, no discovery or resolution occurs.

**Step 2 — At startup, ExtProc discovers and verifies the identifier**

ExtProc performs Journey 1 step 2. Advertising `https://other.example.com` as the policy-decision-point identifier stops startup; failed discovery also stops startup. Setting `authorization.enabled: false` while enabling delegation context stops startup before serving traffic.

**Step 3 — At 10:00:00, a body-bearing proxied request arrives**

The gateway's existing strict JWT validation supplies `aib.tokenexchange.subject_token = SYNTHETIC_SUBJECT_JWT` and `aib.tokenexchange.resource_uri = https://api.example.com/repos`. Raw headers cannot override them. ExtProc concurrently exchanges the token and performs Journey 1 step 3 using its existing assertion provider. After both succeed, body policy sees `input.context.delegation` exactly equal to the returned context and `input.context.delegation_available = true`. A policy allowing `data-assistant` with `repo-reader` access to `repo-api` allows the request. Only the exchanged credential is forwarded downstream.

**Step 4 — The same subject makes a header-only request**

ExtProc resolves the same pair (or uses its still-fresh permit), supplies the same delegation to policy, then exchanges only if policy allows. If policy denies, exchange is not called. A resolver denial instead returns 403 `access_denied` with the consent URL before policy; an expired permit and a failed resolver cause rejection, never stale policy input. A successful resolution with a failed exchange also cannot forward a request.

**Step 5 — Operator migrates and observes the release window**

Canonical-ID policy works for the same named entities even if another deployment assigns different UUIDs. During the resolver release's compatibility window, legacy permission-set inputs are projected from this fresh context. Existing exchange `principal` and `agent_id` fields remain. `granted_permission_sets` is documented as deprecated and removed one release after the resolver ships under the separately approved contract change.

**Observable result**: Both request paths have delegation information before policy evaluation, and no permit, cache hit, or compatibility field bypasses caller authentication, delegation freshness, policy denial, exchange failure, or the existing approval gate.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: The broker MUST provide AuthZEN 1.0 single Access Evaluation and its discovery metadata on the end-user server whenever token exchange is configured, without a new broker enable flag. It MUST not expose this capability as an admin-server API.
- **FR-002**: Discovery MUST publish `policy_decision_point` from the broker public base used for consent URLs and RFC 8414 issuer metadata, and the absolute `access_evaluation_endpoint` for this deployment. A consumer MUST verify the advertised identifier against the expected identifier before sending subject credentials.
- **FR-003**: Evaluation MUST accept exactly one subject token and one protected-resource URI per request. Subject type MUST be `urn:ietf:params:oauth:token-type:jwt`; asserted `identity` and all other subject types MUST return 400. The action name MUST be `urn:ietf:params:oauth:grant-type:token-exchange`; the resource type MUST be `protected_resource`.
- **FR-004**: Evaluation MUST resolve principal and agent using the same subject-token validation and `token_exchange.claim_extraction` CEL semantics as third-party token exchange. Evaluation MUST authenticate the PEP through a bearer client assertion and apply the existing privileged-client CEL policy with `request.grant_type = "authzen_evaluation"`; an ordinary authenticated browser principal is not a substitute.
- **FR-005**: The broker MUST compare any supplied `context.agent` anchor against the agent resolved from the verified token; mismatch returns 400. [NEEDS CLARIFICATION: Q1 — Is the agent anchor optional, as proposed, or mandatory in every evaluation request? Confirm omission behavior before OpenAPI is frozen.]
- **FR-006**: Evaluation and exchange MUST share one authoritative delegation resolution, preserving subject/assertion validation, claim extraction, caller authorization, resource normalization/resolution, registered-agent resolution, active-grant verification, declared permission-set checks, service coverage, scope union, and agent scope ceilings. The issue calls this `ResolveDelegation` (exchange steps 2–9a); it MUST not duplicate or weaken those invariants. Existing token-exchange session retrieval, refresh, decryption, session-scope checks, diagnostics, and response behavior remain downstream of this shared resolution and unchanged except for the explicitly staged deprecation.
- **FR-007**: A permit MUST return `decision: true` and a credential-free Delegation Context: the verified principal in `subject`; `agent {id, canonical_id}`; resolved service in `resource {id, canonical_id, resource_uri, scopes}` with effective scopes; `grant {id, updated_at, valid_until}`; and granted `permission_sets[] {id, canonical_id, services[] {id, canonical_id, scopes}}`. Only grant-included service entries may appear. Permission-set scopes describe the included permission-set contributions; `resource.scopes` is the final union after agent ceilings. Permission-set entries MUST NOT be interpreted as an override of that effective resource scope ceiling.
- **FR-008**: Context identities MUST retain real UUIDs and use configured canonical IDs for stable readable policy keys. [NEEDS CLARIFICATION: Q2 — Existing canonical IDs are optional. Should absent canonical IDs be omitted, be null, or make evaluation unavailable until configured? Confirm the response and rollout contract for agents, services, and permission sets lacking them; do not invent IDs.]
- **FR-009**: Permit freshness MUST be `Cache-Control: private, max-age=min(60, whole seconds remaining until grant.valid_until)`, bounded below by zero. With no grant expiry it MUST be at most 60 seconds. Expired grants MUST deny. The 60-second broker cap is fixed for this increment; no configurable broker cap is introduced without a later explicit decision.
- **FR-010**: Each grant-denial condition in US2.1–US2.10 MUST return HTTP 200 with `decision: false`, the matching existing `FailureDetail` in the administrative reason and the resolved agent's consent URL in the user reason, and `Cache-Control: no-store`. It MUST not include a permit's delegation data or any credential. Denials MUST not be cached.
- **FR-011**: Malformed/unsupported requests, invalid/expired subjects, and agent-anchor mismatches MUST return 400. Missing/invalid gateway credentials MUST return 401; authenticated but unauthorized gateways MUST return 403; internal dependency/configuration failures MUST return 500. These request failures MUST not be represented as successful decisions. Unregistered resources are 400; resource ambiguity is a configuration failure. Request errors MUST not be cached as permits.
- **FR-012**: Every response MUST echo a supplied `X-Request-ID`, including permit, denial, and request-error responses, while preserving existing trace correlation behavior. Transport MUST use the AuthZEN 1.0 JSON contract and HTTP status semantics.
- **FR-013**: Evaluation MUST be read-only policy information: it MUST not retrieve/decrypt/refresh provider tokens, mint tokens, create grants, create tool approvals, or prove that a provider session is usable. A delegation permit MUST not bypass exchange or policy authorization.
- **FR-014**: ExtProc MUST offer a `DelegationResolver` boundary independent of token exchange, discover its endpoint from the metadata document, authenticate through its existing assertion provider, and fail startup on discovery failure or policy-decision-point identifier mismatch. It MUST preserve the accepted metadata-only gateway input boundary for subject token and resource URI.
- **FR-015**: ExtProc MUST cache only permit contexts per exact `{subjectToken, resourceURI}` pair for no longer than the shorter of the advertised freshness and `max_staleness`. It MUST not cache denies or serve stale delegation on errors, including when exchanged-token fallback succeeds. Missing usable freshness MUST not imply an unbounded lifetime.
- **FR-016**: With delegation context enabled, body-bearing requests MUST start exchange and resolution concurrently during request headers and require both successes before body policy processing; delegation denial MUST return 403 `access_denied` with the consent URL before policy. Header-only requests MUST resolve, evaluate policy, and only then exchange on policy allow. Every error/deny MUST prevent forwarding, irrespective of concurrent completion order.
- **FR-017**: With delegation context enabled, ExtProc MUST provide `input.context.delegation` verbatim from the permit and `input.context.delegation_available: true` before both policy paths. During the deprecation window, `granted_permission_sets*` compatibility fields MUST be derived from that context, not token-exchange extensions. A resolution failure MUST reject the request, not continue with `delegation_available: false`.
- **FR-018**: ExtProc MUST default `delegation_context.enabled` to false, require `authorization.enabled` when true, and preserve existing disabled-mode behavior without discovery or evaluation calls. Policy fixtures affected by enabled-mode acceptance MUST use canonical IDs rather than fixed deployment UUIDs.
- **FR-019**: The broker-only increment MUST be additive. `granted_permission_sets` on exchange responses MUST be deprecated when the resolver ships and removed one release later under a confirmed contract change; `principal` and `agent_id` MUST stay. The new endpoint MUST not cause immediate removal or an unrelated change to impersonation or other token grants.

### Domain Model *(document before API or database design)*

```mermaid
erDiagram
    Principal ||--o{ UserGrant : owns
    Agent ||--o{ UserGrant : receives
    UserGrant ||--o{ GrantedPermissionSet : includes
    PermissionSet ||--o{ GrantedPermissionSet : identifies
    PermissionSet ||--o{ ServiceScope : defines
    Service ||--o{ ServiceScope : scopes
    Service ||--o{ ProtectedResource : owns
    UserGrant ||--o{ DelegationDecision : informs
    ProtectedResource ||--o{ DelegationDecision : targets
    DelegationDecision ||--o| DelegationContext : permits
```

```mermaid
sequenceDiagram
    participant Gateway
    participant Broker
    participant Policy
    participant Provider
    Gateway->>Broker: Discover and verify policy decision point
    Gateway->>Broker: Evaluate verified subject and resource with client assertion
    Broker-->>Gateway: Permit context or denial with consent recovery
    Gateway->>Policy: Evaluate request with fresh permitted delegation
    Policy-->>Gateway: Allow or deny the actual request
    Gateway->>Broker: Exchange credential only on the allowed header path
    Broker-->>Gateway: Provider token or session error
    Gateway->>Provider: Forward only when every required gate permits
```

**Entities**: Reuse existing Principal, Agent, UserGrant, PermissionSet, ThirdpartyOAuth2Service, and ProtectedResource identities. No new persisted entity is required. The `resource` context projects the service resolved for a normalized protected-resource URI.

**Aggregates**: Existing UserGrant remains the principal-agent delegation consistency boundary. Grant-included permission-set/service relationships and agent-declared scope ceilings govern resolution; cached projections never become a source of authority.

**Value Objects**:

- **Delegation Decision**: Point-in-time permit or denial of a verified principal's delegation to a resolved agent for one protected resource; includes bounded denial/recovery information when denied. It is not a tool-call policy decision or proof of usable provider credentials.
- **Delegation Context**: Credential-free snapshot accompanying a permit, relating principal, agent, service/resource, grant identity/version/expiry, and granted permission-set/service scope data. Its freshness is independent of a provider token's lifetime.
- **Effective scopes**: Existing per-service grant scope union after included-service filtering and the agent's service-requirement ceiling. Reuse existing semantics rather than introducing a new scope calculus.

No new domain event is required because evaluation is read-only; existing grant updates/revocation affect later resolutions. The eventual architectural change must add “Delegation Decision” and “Delegation Context” to `ARCHITECTURE.md`'s glossary and add the delegation gate to its policy-gates table, without conflating it with broker caller CEL, ExtProc policy, or the approval gate.

### Configuration Requirements *(document before implementation)*

The broker reuses existing token-exchange trust, CEL, and public-base configuration with no new keys. ExtProc uses its standalone configuration owner under accepted ADR 011, not the broker configuration port or broker Helm ConfigMap.

| Proposed ExtProc key | Type | Default | Behavior |
| --- | --- | --- | --- |
| `delegation_context.enabled` | boolean | `false` | Opt in; requires `authorization.enabled: true` |
| `delegation_context.policy_decision_point` | URL identifier | Origin of `oauth2.token_endpoint` | Expected discovery identifier; explicit override for non-origin identifiers |
| `delegation_context.request_timeout` | positive duration | `5s` | Maximum duration of a resolver request |
| `delegation_context.max_staleness` | positive duration | `60s` | Upper bound on permit reuse, never extending broker freshness |

Journey 4 shows proposed configuration and its effects. New keys must be exposed through ExtProc's existing file/environment/CLI precedence and startup validation. Invalid URLs, nonpositive durations, authorization dependency failures, discovery errors, and identifier mismatches must prevent enabled-mode readiness. Disabled mode makes no discovery requests. Transport and key trust must retain the existing verified-TLS posture.

Implementation documentation must update `docs/configuration.md`, the appropriate `examples/config/` ExtProc example and index, and the owning ExtProc deployment guidance. Do not add ExtProc-only values to `charts/agentic-identity-broker/`, which does not deploy ExtProc. No new broker configuration or broker chart value is required by this increment.

### API Requirements *(design before database)*

- **API-001**: The future end-user OpenAPI additions MUST cover `GET /.well-known/authzen-configuration` and `POST /access/v1/evaluation`, all entities, bearer assertion authentication, supported typed subjects/actions/resources, optionality decisions, headers, status codes, error bodies, and examples. No admin API change is proposed.
- **API-002**: These contracts MUST be reviewed and explicitly confirmed before implementation. This draft is not authorization to change `api/enduser/openapi.yaml`. Q1 and Q2 remain open; Journey 1's anchor, subject projection, nullable grant expiry, and denial reason examples must be reconciled with the reviewed schema. Existing exchange/consent APIs remain binding.
- **API-003**: Verify the exact AuthZEN 1.0 metadata, request, response, `reason_admin`/`reason_user` envelope, and request-ID rules against the final standard before freezing OpenAPI. The issue's required administrative diagnostic and user consent URL must be expressible without private RFC 8693 response extensions. This session could not download the reference because TLS verification failed; no unverified standard text is treated as authoritative.
- **API-004**: Protocol choices and exceptions to general REST conventions MUST be documented in an architectural decision and reflected in end-user API documentation under `docs/api/`. Do not copy this draft's illustrative error-body or anchor assumptions into a frozen contract without review.
- **API-005**: A new uniquely numbered proposed ADR MUST describe the AuthZEN information boundary, its opt-in consumer, independent freshness, the changed ADR 028 policy input, and exchange-field deprecation. The issue's suggested “ADR 043” cannot be used: `adrs/043-inbound-cimd-private-key-jwt.md` already exists and is Accepted. Preserve it. Follow constitution Principle II for a new superseding decision covering changes to merged Accepted ADRs; do not amend ADR 028 in place or declare acceptance in this phase. ADR 036's metadata-only trust boundary and retained principal/agent fields remain binding.
- **API-006**: Deprecation MUST be documented before removal, and the removal release MUST be one release after resolver shipment. Planning must reconcile the confirmed release/versioning contract with constitution Principles IV and X; this specification does not authorize an immediate breaking change.

### Security Requirements *(mandatory for security-critical features)*

- **SR-001**: Subject-token signature and configured claim validation MUST remain mandatory for evaluation. The separate unsigned-credential impersonation exception does not apply. PEP assertion validation MUST retain its separate trusted-caller boundary; neither a supplied principal nor a browser auth header may replace it.
- **SR-002**: All enabled-mode errors MUST fail closed; stale context, cached exchange extensions, or a concurrent exchange success cannot authorize a request whose evaluation failed. The opt-in deployment default is not an option to disable validation inside the evaluation path. The proposed ADR must record the additive opt-in rationale consistently with ADR 028.
- **SR-003**: Context and denial/error bodies MUST contain no subject token, assertion, provider access/refresh token, client secret, key material, unfiltered token claims, or internal error text. Logs, audit events, and traces MUST also redact credentials. The caller supplies tokens only for validation, never to be echoed as context.
- **SR-004**: Evaluation MUST preserve grant-before-session ordering and credential-free structured auditability, including existing actor/calling-peer separation and trace correlation semantics where applicable. Report bounded diagnostic categories rather than dumping token or dependency contents.
- **SR-005**: Discovery identifier mismatch MUST fail startup without credential transmission to the mismatched service. Trusted transport and cryptographic verification MUST use the project's vetted mechanisms; no custom crypto or TLS-verification bypass is introduced.
- **SR-006**: Existing strict gateway metadata provenance, policy fail-closed behavior, body-size enforcement, and approval requirements remain independent gates. A Delegation Decision does not override them.

### Key Entities

- **Principal**: User identified by verified subject-token claims and existing CEL extraction.
- **Agent**: Registered delegate with stable UUID, optional canonical ID, declared permission sets, and service requirements.
- **ThirdpartyOAuth2Service / ProtectedResource**: Registered service and its protected-resource URI ownership, resolved using existing normalization rules.
- **UserGrant**: Existing principal-agent delegation with permission-set/service selections, update time, and optional expiry.
- **PermissionSet**: Existing named scope grouping, with stable UUID and optional canonical ID; only included services contribute to the delegation projection.
- **Delegation Decision / Delegation Context**: Read-only value objects described above, not new stored entities or credentials.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: In the controlled acceptance journeys, every covered delegation returns the correct principal, agent, service, grant, and effective permissions, and every listed grant-denial condition denies with its exact diagnostic and consent recovery destination.
- **SC-002**: With default freshness limits, a delegation revoked immediately after a cached permit stops contributing permissions to later gateway decisions within at most 60 seconds; a sooner grant expiry or configured consumer cap shortens that bound.
- **SC-003**: Both body-bearing and header-only accepted journeys provide the same current delegation information before policy evaluation; neither forwards any request unless all applicable gates succeed.
- **SC-004**: Every invalid-credential, caller-denied, malformed-request, dependency-outage, expired-context, and discovery-mismatch journey rejects safely, with zero credential disclosure in its observable responses and captured audit output.
- **SC-005**: Equivalent named agents, permission sets, and services with different deployment UUIDs produce the same canonical-ID policy outcome in the acceptance fixtures, without hardcoded service UUID policy dependencies.
- **SC-006**: Existing disabled-mode deployments require zero new settings and make zero delegation discovery/resolution requests; the additive broker release preserves existing exchange fields and session behavior.

## Assumptions

- Scope is one feature with three delivery increments: additive broker evaluation/discovery and shared delegation semantics; opt-in ExtProc consumer and canonical-ID policy fixtures; documented exchange-field deprecation with removal one release after the resolver ships. These are product increments, not permission to run planning or implementation in this session.
- The four issue-recorded stakeholder decisions dated 2026-10-24 are preserved without re-asking: Access Evaluation (not Resource Search), JWT subject in the AuthZEN body with assertion in Authorization, asserted `identity` returns 400, and only exchange `granted_permission_sets` is deprecated/removed while `principal` and `agent_id` stay. Other proposed details remain subject to normal contract review; this draft invents no acceptance of an ADR.
- The proposed optional agent anchor remains unresolved because the issue itself requests confirmation before OpenAPI freeze. Absent canonical IDs also need a stakeholder compatibility decision because today's domain permits them.
- The fixed broker freshness cap is 60 seconds for this increment, consistent with “no new configuration”. A configurable broker cap is follow-up scope, not a blocker to this draft. Batch `/access/v1/evaluations`, Resource Search, actor-token support, `context.actor`, new `act`-chain response data, new UI, and new persistence/schema migrations are excluded.
- Permission-set service scopes describe included permission-set contributions; the requested resource's effective scopes remain authoritative after agent ceilings. The final API review must keep that distinction visible to policy authors.
- No new storage schema is expected: decisions project existing registered entities and grants. Evaluation intentionally does not guarantee provider-session health or credential availability.
- The issue references `specs/design-authzen-delegation-pip.md` on another branch. That file is absent from this checkout and was not used to import additional requirements; neither the other branch nor the external artifact was fetched. The supplied issue title/body are the feature input.
- Relevant retrieved baselines: `ARCHITECTURE.md` policy-gates table and glossary; ADRs 011, 012, 018, 028, 029, 033, 036 (ExtProc metadata input), and existing 043; current token-exchange resolution/scoping and `api/enduser/openapi.yaml`. Proposed ADRs are background, not accepted authority. Existing source behavior is used to expose compatibility constraints, not to add unrelated requirements.

### Clarification handoff

The separate clarification session will ask stakeholders to resolve the outstanding questions. This draft records two product decisions for that handoff:

| Question | Context | Options and implications |
| --- | --- | --- |
| Q1 — Agent anchor requiredness | FR-005; US1.2 and US2.18 | A: optional consistency anchor (proposal), allowing omission when token extraction is sufficient. B: required anchor, with 400 on omission. Custom: stakeholder-defined alternative retaining token-derived identity and mismatch rejection. |
| Q2 — Missing canonical IDs | FR-008; US1.8 and US3.8 | A: omit unset canonical IDs, preserving UUID-based evaluation compatibility. B: return explicit nulls, requiring nullable consumer contracts. C: require configured canonical IDs, introducing an explicit rollout prerequisite and a defined failure contract. Custom: another explicit representation without fabricating IDs. |

The requirements checklist intentionally leaves affected completeness/readiness items unchecked. Draft publication is the specify handoff, not contract approval or readiness to implement.
