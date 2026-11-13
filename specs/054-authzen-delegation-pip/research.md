# Research: AuthZEN Delegation Policy Information Point

**Feature**: `specs/054-authzen-delegation-pip` | **Date**: 2026-10-27 | **Plan**: [plan.md](./plan.md)

This document resolves the Technical Context unknowns for the plan. Each entry records the
decision, rationale, and the alternatives considered. Entries marked **Stakeholder** or
**Verify** are design gates that must close before implementation. They are listed again in
[plan.md](./plan.md#stakeholder-decisions-before-implementation).

## Sources read

- `AGENTS.md`, `.specify/memory/constitution.md`, `internal/AGENTS.md`, and `internal/extproc/AGENTS.md`
- `ARCHITECTURE.md` §3.1.4 (public API), §3.2 (ExtProc), the policy-gates table, and the glossary
- ADR 009 (CEL), ADR 011 (ExtProc standalone), ADR 012 (ExtProc token cache), ADR 013 (typed IDs),
  ADR 028 (OPA in ExtProc), ADR 033 (request security context), ADR 035 (shared tool patterns),
  ADR 036 (ExtProc metadata input), and ADR 043 (existing, Accepted)
- `internal/domain/tokenexchange/service.go`, `request.go`, `errors.go`, `diagnostic.go`, and `cel_evaluator.go`
- `internal/domain/consent/service.go` (`VerifyAgentAccess`), `internal/domain/permissionset/service.go`,
  and `internal/domain/thirdparty/service.go` (`CanonicalIDs`)
- `internal/adapters/http/enduser/oauth2_token.go`, `internal/adapters/http/middleware/oauth2_audit.go`,
  and `internal/adapters/http/routing/enduser.go`
- `internal/app/builder.go` (token-exchange wiring and `tokenExchangeEnabled`)
- `internal/extproc/server/server.go`, `exchanger.go`, `internal/extproc/authorization/input.go`,
  `internal/extproc/approval/client.go`, `internal/extproc/config/*.go`, and `cmd/extproc-token-exchange/root.go`
- `api/enduser/openapi.yaml` (`/oauth2/token` response, `ErrorResponse`, and security schemes)
- `tests/e2e/bootstrap/extproc.go`, `tests/e2e/extproc/bootstrap/`, `tests/e2e/impersonation_test.go`
  (grant-repository outage technique), and `specs/052-inline-jwks-trust-anchors/plan.md` (plan precedent)

The AuthZEN Authorization API 1.0 text could not be downloaded in this session. The request
failed TLS certificate verification, and it was not retried insecurely. Everything below that
depends on the exact standard text is marked **Verify** (spec API-003).

---

## R1. Where the shared delegation resolution lives

**Decision**: Extract exchange steps 2–9a from `TokenExchangeService.Exchange` into one domain
type, `tokenexchange.DelegationResolver`, in a new file `internal/domain/tokenexchange/delegation.go`.
`Resolve(ctx, DelegationRequest) (*Delegation, error)` performs, in today's order:

1. Validate the subject token.
2. Validate the client assertion.
3. Extract the principal and finalize the ADR 033 security context.
4. Extract the agent ID.
5. Authorize the caller with the unchanged CEL policy.
6. Normalize the resource.
7. Resolve the service.
8. Parse and load the agent.
9. Verify the grant.
10. Check that the grant is not empty.
11. Run `resolveEffectiveScopes`.

`TokenExchangeService.Exchange` calls the resolver and then continues with session retrieval
(step 10), the session-scope check (step 11), and the response (step 12). The existing
`TokenExchangeError` values, diagnostics, `ServiceRef`, and `AuthorizationRef` observations
are unchanged.

**Rationale**:

- FR-006 requires one authoritative resolution.
- Every dependency the resolver needs (`JWTValidator`, `CELEvaluator`, `ThirdpartyOAuth2ProviderService`,
  `consent.Service`, `permissionset.Service`, and `ports.AgentRepository`) is already owned by `tokenexchange`.
- A separate `internal/domain/delegation/` package would either import `tokenexchange`, which
  would create an import cycle when exchange calls it, or duplicate the validators. Principle VI
  forbids a domain package that is not a genuinely independent bounded context.
- The resolver needs no `oauth2session` dependency. This makes FR-013 structural: evaluation
  cannot reach provider tokens. The consent-URL base is injected as a string. It is the same
  `server.enduser.public_url` value that `oauth2SessionService.GetCallbackBaseURL()` returns today.

**Alternatives considered**:

- *Duplicate steps 2–9a in an evaluation service*. Rejected because FR-006 forbids duplication
  and the copies would drift.
- *Call `Exchange` and discard the token*. Rejected because it retrieves, decrypts, and
  refreshes provider tokens, which violates FR-013 and US1.3.
- *A new `domain/delegation` package with ports for JWT and CEL*. Rejected because of the import
  cycle and the duplicated abstractions described above.

**Regression guard**: The existing token-exchange unit tests (`service_test.go`,
`service_diagnostic_test.go`, `service_classification_test.go`, `service_security_test.go`,
and `service_expiry_regression_test.go`) and the E2E suites (`token_exchange_test.go`,
`grant_diagnostics_test.go`, and `local_token_exchange_e2e_test.go`) must pass unchanged
after the extraction. The extraction is a no-behavior-change refactor in Phase 0.

## R2. Access Evaluation domain service

**Decision**: Add `tokenexchange.AccessEvaluationService` in `internal/domain/tokenexchange/evaluation.go`.
It owns the AuthZEN request invariants and turns the resolution result into one of three outcomes:

| Outcome | When | HTTP status (adapter) |
|---|---|---|
| `Permit` | Resolution succeeds | 200, `decision: true` |
| `Deny` | Resolution returns `AccessDenied` with a `StageGrantAuthorization` grant detail (US2.1–US2.10) | 200, `decision: false` |
| `RequestFailure` | Any other classified failure | 400, 401, 403, or 500 (R6) |

The service also performs these steps:

- It validates `subject.type`, `action.name`, and `resource.type` (FR-003).
- It compares the optional `context.agent` anchor with the resolved agent after resolution and
  before the outcome is built (FR-005).
- It computes freshness (R5).
- It builds the Delegation Context (R4).

The HTTP handler only decodes JSON, extracts the bearer assertion, calls the service, and
serializes the outcome. This avoids domain-logic leakage (Principle VI).

**Anchor ordering**: The anchor is compared before the grant read. The resolver accepts an
optional `ExpectedAgentID`. After it loads the agent (step 8), it returns `ErrAgentAnchorMismatch`
if the IDs differ. No grant is read for either agent in that case (US2.18: "never evaluates
the other agent's grant"). Token exchange passes no anchor, so its behavior is unchanged.

**Alternatives**: *Compare in the handler*. Rejected because of domain-logic leakage.
*Compare after the grant read*. Rejected because it would read Alice's grant before rejecting
the request. That reveals nothing externally, but it violates US2.18's wording.

## R3. Caller-policy input for evaluation

**Decision**: Evaluation builds the same `CELRequestContext` as exchange:

- `Resource`: the raw `resource.id`
- `GrantType`: `urn:ietf:params:oauth:grant-type:token-exchange`
- `Scope`: `""`
- `Principal` and `AgentID`: the extracted values

It calls `AuthorizePrivilegedClient` with the verified assertion and subject claims. No new
grant type, flag, or CEL variable is added. A CEL denial maps to HTTP 403 (US2.17).

**Rationale**: This applies the 2026-10-27 maintainer guidance recorded in the spec and FR-004
literally. An existing policy that rejects a gateway or resource for exchange rejects the same
evaluation.

**Consequence documented in the ADR**: A policy cannot distinguish evaluation from exchange.
This is intentional. Evaluation is never more permissive than exchange, and it issues no token.

## R4. Delegation Context projection

**Decision**: Build the context from the resolver's `Delegation` value:

| Field | Source |
|---|---|
| `subject.id` | Extracted principal |
| `agent.id`, `agent.canonical_id` | Loaded `storage.Agent`. The canonical ID is omitted when nil (FR-008). |
| `resource.id` | Resolved service UUID |
| `resource.canonical_id` | `ThirdpartyOAuth2ProviderEntity.CanonicalID`, omitted when nil |
| `resource.resource_uri` | The *normalized* resource URI (US1.5: equivalent URIs give the same result) |
| `resource.scopes` | `effectiveScopes[serviceID]`, sorted, after agent ceilings (FR-007). A declared zero-scope service yields `[]`. |
| `grant.id`, `grant.updated_at` | The grant |
| `grant.valid_until` | The grant. The member is **omitted** when the grant has no expiry (see API-002 below). |
| `permission_sets[]` | One entry per grant entry, in grant order: `id`, `canonical_id` (omitted when nil), and `services[]` |
| `permission_sets[].services[]` | One entry per `IncludedServiceIDs` element, in grant order: `id`, `canonical_id` from one batched `ThirdpartyOAuth2ProviderService.CanonicalIDs` call (omitted when absent), and `scopes` from the permission set's `ServiceScopes` for that service, sorted. The value is `[]` when the permission set has no definition for a non-requested included service. |

**Rationale**:

- Using all included service IDs keeps the legacy projection
  `granted_permission_sets[ps.id] = [services[].id]` (FR-017) identical to today's exchange
  field. That field lists every included service ID.
- The permission-set scopes are raw contributions. `resource.scopes` remains the authoritative
  ceiling (FR-007 and the Assumptions).
- One batched canonical-ID lookup avoids N+1 reads. If the lookup fails, the error maps to 500
  (`DetailResourceRepositoryUnavailable`). It is never omitted as though no canonical ID existed:
  FR-008 omits a canonical ID only when none is configured.

**Alternatives**:

- *Only services with permission-set definitions*. Rejected because the legacy projection
  would diverge from exchange.
- *Ceiling-capped permission-set scopes*. Rejected because FR-007 distinguishes contributions
  from the effective ceiling.
- *Null canonical IDs*. Rejected by the clarification (Q2: A).

## R5. Permit freshness

**Decision**: `max-age = clamp(floor((valid_until − now) / 1s), 0, 60)`. Without `valid_until`,
the value is `60`. Both values come from one `now` read through an injectable `func() time.Time`.
The production default is `time.Now`. Tests set the field through an unexported setter in
`export_test.go`. The response header is `Cache-Control: private, max-age=<n>`. Permits with
`n = 0` are valid but not reusable.

Grant expiry is checked by `consent.Service.VerifyAgentAccess` with its own `now`. A grant that
expires between the two reads denies with `grant_expired`. The freshness computation therefore
never advertises time beyond `valid_until`.

**Rationale**: FR-009 and US1.6. Flooring means 14.9 s remaining yields `max-age=14`, so
freshness is never rounded up.

## R6. Error classification and HTTP mapping

**Decision**: Classify by the existing `TokenExchangeError` code and `FailureDetail`. No new
detail strings are needed. New request-validation failures reuse `DetailRequestMalformed`.

| Condition | Existing detail or error | Evaluation status |
|---|---|---|
| Malformed JSON, oversized body, missing or unknown members, wrong member types | `request_malformed` | 400 |
| Unsupported `subject.type` (including `identity`), `action.name`, or `resource.type` | `request_malformed` | 400 |
| Invalid or missing `resource.id` URI | `resource_missing` or `request_malformed` | 400 |
| Invalid or expired subject token | `subject_invalid` or `subject_expired` (invalid_grant) | 400 |
| Malformed or mismatched `context.agent` anchor | new sentinel → `request_malformed` | 400 |
| Unregistered resource | `resource_unregistered` (invalid_target) | 400 |
| Missing `Authorization: Bearer` | Checked in the handler before the domain call | 401 |
| Invalid or expired client assertion | `client_invalid` or `client_expired` (invalid_client) | 401 |
| CEL caller policy denies | `client_policy_denied` (access_denied, `StageClientAuthorization`) | 403 |
| Grant denial matrix (US2.1–US2.10) | `grant_*` (access_denied, `StageGrantAuthorization`) | 200, `decision: false` |
| Agent missing or invalid, resource ambiguous, JWKS unavailable, CEL configuration or evaluation failure, any repository unavailable, caller canceled, unclassified | server_error details | 500 |

The 401 response carries `WWW-Authenticate: Bearer` (RFC 6750). Invalid credentials add
`error="invalid_token"`. Every non-200 body uses the end-user API's existing `ErrorResponse`
schema (`{"error", "message"}`) with fixed, credential-free messages. The 200 denial is the only
response that carries `reason_admin` and `reason_user`. All non-permit responses use
`Cache-Control: no-store` (FR-010 and FR-011).

**Ordering note**: The shared resolver keeps exchange's order: subject validation, then
assertion validation. A request that has a bearer header but both an invalid subject and an
invalid assertion therefore returns 400, as exchange returns `invalid_grant` today. Grant data
is read only after both credentials and the caller policy succeed. FR-011 and SR-001 hold.
Reversing the order for evaluation only would fork the shared resolution (FR-006).

**Alternatives**: *Problem JSON (`application/problem+json`)*. Rejected because the end-user
API does not use it anywhere. The reviewed OpenAPI can still choose it during contract review.

## R7. `X-Request-ID`

**Decision**: The evaluation and discovery handlers copy a supplied, non-empty `X-Request-ID`
request header to the response on every path, including 400, 401, 403, and 500. The
`OAuth2AuditMiddleware` on the route keeps generating an internal ID for audit when none is
supplied, but that generated ID is **not** echoed (spec Edge Cases: no new request-ID
generation requirement). Trace correlation is unchanged.

## R8. Discovery metadata and the public base

**Decision**: `GET /.well-known/authzen-configuration` returns these values:

- `policy_decision_point`: `strings.TrimRight(server.enduser.public_url, "/")`
- `access_evaluation_endpoint`: the same base plus `/access/v1/evaluation`

The response uses `Content-Type: application/json` and `Cache-Control: max-age=3600`,
the same header that the RFC 8414 handler (`oauth2_metadata.go`) sets.

**Rationale**:

- `server.enduser.public_url` is the base for consent URLs. `agentConsentURL` reads it through
  `GetCallbackBaseURL`, which the builder sets from `public_url` with trailing slashes trimmed.
  It is also the default local issuer.
- FR-002 names the consent-URL base. When `local.issuer_uri` overrides the issuer, consent
  links still use `public_url`. The PDP identifier follows the consent base, which is the
  single deployment-wide public origin.

**Verify (API-003)**:

- Confirm the exact metadata member names.
- Confirm whether AuthZEN inserts `/.well-known/authzen-configuration` before a path component
  of the PDP identifier, as RFC 8414 does for issuers.

The broker registers only the root path. Deployments whose `public_url` has a path prefix must
set `delegation_context.policy_decision_point` explicitly in ExtProc (spec Edge Cases). The
proxy must also route the standard discovery URL. The docs will state this.

## R9. Route availability and namespace reservation

**Decision**:

- The builder creates `AuthZENHandlers` only when `app.TokenExchangeService != nil`, which is
  the existing `tokenExchangeEnabled` condition (FR-001 and US1.7).
- `SetupEnduserRoutes` registers `GET /.well-known/authzen-configuration` inside the existing
  `/.well-known` route group and `POST /access/v1/evaluation` in a new `/access` route group.
  Both registrations happen only when their handler is non-nil.
- The `/access` namespace is always reserved, like `/health`. A disabled deployment therefore
  returns 404 instead of SPA HTML or a 405 from the SPA catch-all.
- The evaluation route uses `OAuth2AuditMiddleware` and no principal middleware. It is a
  machine-facing endpoint like `/oauth2/token`. The server-wide `SecurityContextMiddleware`
  currently defers finalization **only** for `POST /oauth2/token`; evaluation does not yet
  receive a capture holder. The proposed design must extend the shared deferred-route matcher
  to `POST /access/v1/evaluation`, so the resolver finalizes verified Actor and distinct
  CallingPeer before caller-policy authorization. Early decode/authentication errors must
  finalize the request-principal/anonymous fallback before in-handler audit, using the existing
  idempotent helper; the perimeter net remains a last resort. Preserve ordinary-request and
  token-route behavior. The maintainer endorsed this extension with the R16 decision set
  (2026-10-28 review); sign-off is approval item ADR-1 in the plan. It still needs the §3.1.4.2 documentation and T044/T047 test-first coverage;
  it is not existing functionality (HIGH I2 in [remediation.md](./remediation.md)).

## R10. Request body bounds and strictness

**Decision**:

- The body is limited to 64 KiB with `http.MaxBytesReader`. A JWT subject token is far
  smaller. Overflow returns 400.
- The decoder uses `DisallowUnknownFields`. Unknown or unsupported members return 400. This
  includes `properties`, `context.actor`, and any context member other than `agent`.
- `context` may be absent or an object. `context.agent` may be absent or a UUID string.

**Rationale**: Fail closed. Silently ignoring an `actor` or `properties` member could make a
caller believe it was evaluated (SR-002).

**Stakeholder (contract review)**: AuthZEN permits extensible `properties`. Strict rejection is
recorded in [contracts/authzen-evaluation-api.md](./contracts/authzen-evaluation-api.md). The
maintainer endorsed that contract in the 2026-10-28 review, including this strictness. Sign-off is
approval item API-2 in the plan.

## R11. Denial representation

**Decision (proposed, Verify)**: The proposed response is
`{"decision": false, "context": {"reason_admin": {"code": "<FailureDetail>"}, "reason_user": {"consent_url": "<public_url>/agents/<agent-uuid>"}}}`.
This is the spec's proposal.

**Verify (API-003)**: AuthZEN's informative examples key reason objects by language tag (for
example, `{"en": "..."}`). Before the OpenAPI contract is frozen, the stakeholder must choose
one of these forms:

- (a) keep the machine-readable members above, or
- (b) use language-tagged strings with the code and URL in separate extension members.

The contract file presents both forms. Implementation tasks depend only on the domain outcome
(`Deny{Detail, ConsentURL}`), so the choice affects serialization only.

**Contract review (2026-10-28)**: The maintainer endorsed the contract, whose
`AccessEvaluationDenial` schema uses form (a). The API-003 check against the final AuthZEN 1.0
text still runs before the OpenAPI is frozen. If that check finds a conflict with form (a), it
goes back to the stakeholder. It is not resolved silently.

## R12. Exchange-response deprecation and release timing

**Decision**:

- This feature adds `deprecated: true` and a removal note to the `granted_permission_sets`
  property of the `/oauth2/token` response schema.
- It bumps `info.version` of `api/enduser/openapi.yaml` from `1.0.0` to `1.1.0` for the
  additive endpoints.
- It documents the deprecation in `docs/reference/token-exchange.md`.
- The field remains populated. `principal` and `agent_id` are unchanged (US4.6 and FR-019).

**Removal** happens one release after the resolver ships. It is a breaking change that requires
end-user API `2.0.0`, following the feature-051 precedent. It is **not** an implementation task
of this feature, because it must ship in a later release. It needs a separately confirmed
contract change (spec API-006 and Principles IV and X). The plan records it as follow-up work.

**Stakeholder**: The contract endorsed in the 2026-10-28 review covers the 1.1.0 additive bump
and the deprecation marker with its removal note (contract §3). Sign-off is approval item API-3. The removal itself still needs a
separately confirmed 2.0.0 contract change in a later release.

## R13. ExtProc resolver boundary

**Freshness decisions (2026-10-28, maintainer review on HIGH I1/U1 in [remediation.md](./remediation.md))**:

- **Fail closed on unusable freshness (I1)**: A permit needs exactly one positive integer
  `max-age` directive. A missing, zero, malformed, duplicate, or non-integer value is an
  `invalid_response` error. The result is 503 for this request, and nothing is cached. No
  permit is ever used "for this request only".
- **One deadline, computed once (U1)**: `deadline = requestStart + min(max-age, max_staleness)`.
  `requestStart` is read from the clock just before the POST is sent. The `Result` carries
  the deadline. The cache, the singleflight followers, and `requestState` reuse it unchanged.
  ExtProc does no `Age` or transit accounting and applies no separate `grant.valid_until` cap.
  The broker already caps `max-age` by the grant expiry, and the maintainer chose to accept the
  remaining skew.

**Decision**: Add a new package, `internal/extproc/delegation/`, with no broker imports (ADR 011
and the AGENTS invariant). It contains:

- `discovery.go`: `Discover(ctx, client, expectedPDP) (Metadata, error)`. It builds the
  discovery URL from the expected identifier and fetches it with the outbound TLS client.
  It requires `policy_decision_point == expectedPDP` by exact string comparison, with no
  normalization or path stripping. It requires `access_evaluation_endpoint` to be an absolute
  URL, `https` unless `oauth2.tls.allow_http`, with a host and without userinfo or fragment.
  Discovery sends no credentials (SR-005 and US4.4).
- `client.go`: `Client.Evaluate(ctx, subjectToken, resourceURI) (Result, error)`. It POSTs the
  AuthZEN request with `Authorization: Bearer <assertion>` from the existing `AssertionProvider`
  (`TokenExchanger.ClientAssertion`, which the approval client already uses). It omits
  `context.agent`, because ExtProc has no verified agent before resolution and omission is
  allowed by FR-005. It parses `Cache-Control` `max-age`. It accepts only HTTP 200 with a
  boolean `decision`. Every other status, a malformed body, an unusable `max-age` on a
  permit (missing, zero, malformed, or duplicate), or a timeout is an error. A permit
  `Result` carries the absolute `Deadline` described above.
- `resolver.go`: `Resolver` with a permit-only cache keyed by the struct
  `cacheKey{subjectToken, resourceURI}` (the same separator-safe pattern as ADR 012), a
  `sync.RWMutex`, `singleflight`, and eviction.
  - An entry stays until the result's `Deadline`. The client already rejected an unusable
    `max-age`, so every permit that reaches the cache has a future deadline (FR-015).
  - A lookup returns an entry only while `now < Deadline`. A hit returns the stored
    deadline unchanged.
  - Denials and errors are never cached.
  - There is no stale-on-error fallback.

`server.DelegationResolver` becomes the fourth ExtProc port, next to `Exchanger`, `Authorizer`,
and `ApprovalGate`.

**Alternatives**:

- *Extend `TokenExchanger`*. Rejected because FR-014 requires a boundary independent of token
  exchange, and the exchanger's stale fallback must not apply.
- *Reuse `approval.Client`*. Rejected because it is a different bounded concern.

## R14. ExtProc request-path ordering

**Expiry before policy use (2026-10-28, maintainer review on HIGH U2 in [remediation.md](./remediation.md))**:
Reject with 503. `requestState` stores the permit's `Deadline` together with the context.
Immediately before every OPA evaluation that receives delegation input, ExtProc checks
`now < Deadline`. This covers the header-only policy call, the body policy call, and each
decision in a batch. If the check fails, ExtProc returns 503 `service_unavailable` with
`"delegation context unavailable"`, does not evaluate policy, and forwards nothing. It does not
re-resolve or retry.

**Decision** (when `delegation_context.enabled`):

**Body-bearing path** (`processRequestHeadersOPA`):

1. After metadata, protocol, and transport checks, start `Exchanger.Exchange` and
   `DelegationResolver.Resolve` concurrently.
2. **Wait for both results.** No sibling is canceled. Each call is bounded by its own timeout
   (`oauth2.exchange_timeout` and `delegation_context.request_timeout`). This makes the outcome
   independent of completion order (FR-016).
3. Apply this precedence:
   1. A delegation denial returns 403 `access_denied` with `error_uri` set to the consent URL.
      Consent recovery precedes session recovery, matching grant-before-session ordering (SR-004).
   2. A resolution error returns 503 `service_unavailable` with `"delegation context unavailable"`.
   3. An exchange error uses the existing `exchangeErrorResponse`.
   4. Otherwise, buffer the body with the Authorization mutation and store the context and its
      `Deadline` in `requestState`. The deadline is checked again before each body policy
      evaluation (503 if it has passed).

**Header-only path** (`processHeadersOnlyOPA`):

1. Resolve the delegation. Apply the same denial and error responses as the body path.
2. Check the deadline (503 if it has passed), build the input with the context, and evaluate policy.
3. On allow, call `Exchange`.

Policy denial never calls exchange (US3.9).

**Disabled mode**: The code paths are unchanged (FR-018 and US4.1).

**Rationale**:

- Waiting for both results costs at most `max(exchange, resolve)` latency, compared with the
  sum for sequential calls.
- It removes the race in which an early exchange failure could hide a consent denial.

**Alternative**: *Cancel the sibling on the first failure*. Rejected because the outcome would
depend on completion order.

**Response codes**: The 503 for resolution errors matches ExtProc's existing fail-closed
dependency responses (`service_unavailable`). The 403 denial body is
`{"error":"access_denied","error_description":"delegation denied: <code>","error_uri":"<consent_url>"}`.
The administrative code is a bounded enum from the broker. The resolver allowlists the
`grant_*` details and maps any other value to `delegation_denied`.

## R15. OPA input changes

**Spec clarification (2026-10-28)**: The maintainer review on the pre-review design said that
the `delegation` wrapper and the `delegation_available` flag let the way ExtProc obtains the
context bleed into OPA policies. The spec clarification (spec Clarifications, Session
2026-10-28) settles the shape. FR-017, FR-018, US3.1, US3.2, US3.6, US3.8, US4.1, Journey 4,
and SC-005 now require it. This entry replaces the pre-review decision.

**Decision**: Extend the existing `authorization.ContextInput` with five members. Add them
directly beside `granted_permission_sets_available`, `granted_permission_sets`, and
`agent_session_id`. There is no wrapper and no new flag.

| Member (`input.context.…`) | Go field and type | JSON tag | Source in the typed permit |
|---|---|---|---|
| `principal` | `Principal string` | `principal,omitempty` | `context.subject.id` |
| `agent` | `Agent *AgentContext` (`ID string`; `CanonicalID string`, `canonical_id,omitempty`) | `agent,omitempty` | `context.agent` |
| `resource` | `Resource *ResourceContext` (`ID`, `CanonicalID` with `omitempty`, `ResourceURI`, `Scopes []string`) | `resource,omitempty` | `context.resource` |
| `grant` | `Grant *GrantContext` (`ID`, `UpdatedAt string`, `ValidUntil string` with `omitempty`) | `grant,omitempty` | `context.grant` |
| `permission_sets` | `PermissionSets []PermissionSetContext` (`ID`, `CanonicalID` with `omitempty`, `Services []ServiceContext{ID, CanonicalID omitempty, Scopes}`) | `permission_sets,omitempty` | `context.permission_sets` |

The policy-input types live in `internal/extproc/authorization`, which owns the OPA input
document. Their `scopes` fields have no `omitempty`, so an empty scope list is `[]`, never
`null` and never absent.

**Typed decoding (no verbatim copy)**: `internal/extproc/delegation` decodes the permit's
`context` into its own unexported wire struct with `encoding/json`. Unknown members at any
level are ignored. They are never copied, so they cannot reach Rego. The client then validates
the documented shape. Any violation is an `invalid_response` resolution error: ExtProc returns
503 and does not cache or use the permit. The client checks these rules:

- `subject.id`, `agent.id`, `resource.id`, `resource.resource_uri`, `grant.id`, and every
  `permission_sets[].id` and `services[].id` are non-empty strings.
- `resource.scopes`, every `permission_sets[].services`, and every `services[].scopes` are
  arrays (empty arrays are allowed).
- `permission_sets` is a non-empty array. The broker never permits an empty grant
  (`grant_empty`).
- `grant.updated_at` is an RFC 3339 timestamp. `grant.valid_until`, when present, is an RFC
  3339 timestamp.
- An optional `canonical_id` or `valid_until` is either absent or a non-empty string. An
  explicit `null` or empty string violates the contract ("omitted, never null"), so
  the client rejects it. It never forwards a null value.

Timestamps are validated but kept as the received strings, so the policy sees the same values
as the permit. No numbers appear in the context, so `UseNumber` is not needed.

The client exports the validated value as `delegation.Context`, a Go struct whose fields mirror
the five members with `Principal` taken from `subject.id`. `delegation.Result` carries it with
the `Deadline` (R13). The server maps `delegation.Context` to `authorization.ContextInput` in
one pure function, `contextInputFromDelegation(result, agentSessionID)`. This follows the
existing pattern in which the server builds `ContextInput` from the exchange result. The
function copies every slice, so a cached `Result` that singleflight followers share is never
aliased into a policy input.

**Retained permission-set inputs (permanent)**: In enabled mode, on both the body and the
header-only paths, the same mapping sets:

- `granted_permission_sets = {ps.id: [ps.services[].id …]}` in permit order. A permission set
  whose `services` is empty maps to `[]`. Exchange extensions are never used.
- `granted_permission_sets_available = true`.

These OPA members stay permanently. FR-019's deprecation applies only to the exchange
*response* field, which ExtProc keeps parsing for disabled mode.

**Availability signal**: `granted_permission_sets_available` is the only signal. In enabled
mode, a policy evaluation happens only after a successful, validated, unexpired permit
(R13/R14), so `true` guarantees all five new members. Only the nested optional members can be
absent. A resolution error, a denial, or an expired deadline returns before policy runs, so
enabled mode never produces `false`. On the header-only path, enabled mode therefore reports
`true` where it reports `false` today.

**Disabled mode**: `contextInputFromDelegation` is never called. The five members are zero
values and `omitempty` drops them. The existing body path (`state.grantedPermissionSets` from
exchange) and header-only path (`false`, no permission sets) keep their current code. Their
serialized input is byte-for-byte unchanged. `ast.InterfaceToValue` honors the JSON tags.
`authorization` unit tests pin the disabled-mode JSON with golden values for both paths.

**Unchanged members**: `agent_session_id` keeps its current source and behavior. The approval
gate keeps using the exchange-derived `principal` and `agent_id` (ADR 028 and ExtProc approval
flow). `input.context.principal` is policy input only.

**Rationale**:

- Policies see broker vocabulary (`principal`, `agent`, `resource`, `grant`, and
  `permission_sets`) and do not need to know whether the data came from AuthZEN, from
  exchange, or from a cache.
- A typed mapping fixes the policy schema in ExtProc. A broker-side addition cannot silently
  become a policy input, and the AuthZEN `subject` wire name does not leak into Rego.
- Validating the documented shape keeps the guarantee that `true` means all five members are
  present. A malformed permit fails closed, as every other malformed response does.

**Alternatives**:

- *A `delegation` wrapper with the verbatim permit and a `delegation_available` flag* (the
  pre-review design). Rejected by the 2026-10-28 clarification.
- *`map[string]any` with a field allowlist*. Rejected because a typed struct makes the schema
  explicit and testable, and the clarification asks for one.
- *Decoding with `DisallowUnknownFields`*. Rejected because the clarification ignores unknown
  permit fields. Rejecting them would make every additive broker change a breaking change.
- *Treating a `null` optional member as absent*. Rejected for fail-closed handling of a
  response that contradicts the contract.

This changes ADR 028's input schema, which is merged and Accepted. The new superseding ADR
records it (R16).

## R16. Architecture decision record

**Decision**: The plan drafts a new ADR with status **Proposed** at the next unused number. On
2026-10-29, `main` ends at 044 (two merged ADRs use 044), and no remote branch uses 045. The
planned number is therefore **045**, rechecked when the draft moves to `adrs/`. The ADR records these decisions:

- the AuthZEN Access Evaluation information boundary on the end-user server, enabled by token
  exchange, with no new broker flag;
- reuse of the ADR 009 caller CEL policy with the token-exchange grant type (R3);
- independent permit freshness, a fixed 60 s cap, and no negative or stale caching. The ADR
  also records the 2026-10-28 freshness decisions: an unusable `max-age` fails closed (I1), a
  single deadline based on request start with accepted skew (U1), and a 503 rejection when the
  context expires before policy use (U2);
- the ExtProc `DelegationResolver` as a fourth port, with concurrent body-path ordering (R14);
- the superseded parts of ADR 028: the input schema (R15: the five direct `input.context`
  members, the typed permit mapping, the permanent permit-derived `granted_permission_sets*`
  inputs, and `true` availability on both enabled paths) and the header-only and body-bearing
  ordering when enabled. The rest of ADR 028 stays in force;
- the relation to ADR 012: a separate permit cache, with the exchange cache's stale fallback
  never applied to delegation;
- the ADR 036 metadata-only input boundary, unchanged;
- extension of ADR 033's token-only deferred-finalization seam to evaluation (R9), with
  verified Actor/CallingPeer before caller authorization, early-error fallback and unchanged
  ordinary/token request semantics; reconcile `ARCHITECTURE.md` §3.1.4.2 after approval;
- the opt-in default, reconciled with Principle I in the same way as ADR 028's Security-First
  reconciliation;
- the deprecation of the `granted_permission_sets` exchange field and its removal timing (R12).

**Stakeholder**: Maintainer `mjungsbluth` endorsed this ADR decision set in the 2026-10-28
review ("ADR accepted"). Under the pipeline's sign-off rules, review direction is not the
approval record. The planning session drafted the ADR as
[adr-045-authzen-delegation-pip.md](./adr-045-authzen-delegation-pip.md) with status
**Proposed**, and registered it as approval item ADR-1 in the plan. After the maintainer's 👍 on
the pipeline's approval comment, T001 records that approval in the Status line and moves the
draft to `adrs/045-authzen-delegation-pip.md`, rechecking the number. Its OPA input-schema
section follows R15 as revised after the 2026-10-28 spec clarification. Under Principle II, the
ADR may be revised in this PR before it merges, and the final ADR and the code must agree. The
planning session itself accepts nothing.

## R17. ExtProc configuration

**Decision**: Add `DelegationContextConfig` to `extprocconfig.Config` under the
`delegation_context` key:

| Key | Default |
|---|---|
| `enabled` | `false` |
| `policy_decision_point` | `""`; the derived default is the origin of `oauth2.token_endpoint` |
| `request_timeout` | `5s` |
| `max_staleness` | `60s` |

The keys load through `configutil` and the `EXTPROC_` prefix, with defaults in `loader.go`,
Cobra flags in `RegisterFlags`, and `bindFlags`. When `enabled`, validation adds these rules:

1. `authorization.enabled` must be true (US4.2).
2. `policy_decision_point`, if set, must be an absolute `https` URL (or `http` with
   `oauth2.tls.allow_http`), with a host and without query, fragment, or userinfo.
3. `request_timeout > 0`.
4. `max_staleness > 0`.

The origin derivation takes the scheme and host (including the port) of `oauth2.token_endpoint`.
Validation messages contain no credentials. Discovery runs in `root.go` after the exchanger is
built and before the gRPC server listens. Any error stops startup (US4.3–US4.5).

## R18. E2E harness strategy

**Decision**:

- **Broker scenarios** (US1.x, US2.x, and US4.6) go in the backend Ginkgo suite through
  `tests/e2e/bootstrap/` with the production `app.Builder`. They use helpers that already
  exist: `helpers/client_assertion.go`, `jwt_helpers.go`, `fixtures/agents.go`, `grants.go`,
  `services.go`, and `protected_resource.go`.
- **US2.19** reuses the grant-repository outage technique from `impersonation_test.go`
  (renaming `user_grants` on the PostgreSQL test database).
- **Consumer journeys** (US3.1–US3.10 and US4.5) also go in the backend suite. They run the
  production ExtProc composition against the real broker through `tests/e2e/bootstrap/extproc.go`.
  That file is extended with a delegation-context option. Revocation uses the real consent API.
  Freshness scenarios use a short `delegation_context.max_staleness` (for example, `2s`), not
  clock injection, so no test-only production API is needed.
- **Startup-only scenarios** (US4.1–US4.4) go in the ExtProc suite `tests/e2e/extproc/`. A new
  `MockAuthZENBroker` in `tests/e2e/extproc/bootstrap/` counts discovery and evaluation requests,
  and can serve a mismatched identifier or fail discovery.
- **Canonical-ID policy fixture** (R15 shape, FR-018): add
  `tests/e2e/fixtures/policies/delegation_canonical.rego`. It allows when the input satisfies
  all of these conditions:
  - `input.context.agent.canonical_id == "data-assistant"`
  - some `input.context.permission_sets[_]` has `canonical_id == "repo-reader"`
  - `input.context.resource.canonical_id == "repo-api"`

  The policy contains no UUIDs (FR-018 and SC-005). US3.8 runs it on both the body and the
  header-only paths against two fixture sets that use different UUIDs.
- **Context-shape assertions** (US3.1, US3.2, US3.8, US4.1): The policy decision cannot show
  the full OPA input, so ExtProc unit tests in `authorization` and `server` assert the exact
  input JSON. The E2E journeys assert it through policy outcomes. The canonical fixture also
  denies, with a distinct fixed reason for each case, when `input.context.delegation` or `input.context.delegation_available` is
  defined, when `input.context.subject` is defined, or when `granted_permission_sets_available`
  is not `true`. The consumer journeys use the real broker, which emits no unknown members, so
  `delegation` client and `server` unit tests cover unknown-member exclusion. Their permit
  fixtures carry an unknown member at every object level.

## Deferred findings (not tasks of this feature)

These are actionable findings outside the feature's required work. Under the constitution
(Feature Scope & Deferred Findings), the user decides whether to file issues:

1. **Feature-number collision**: `specs/054-agent-external-id-filter` (merged on `main`) and
   this feature both use number 054. Renumbering is a process decision for the maintainer.
2. **ADR number collision**: two ADRs on `main` both use 044
   (`044-client-registration-independent-of-authentication.md` and
   `044-service-specific-session-recovery-url.md`). This feature avoids the number. Renumbering
   them is a process decision for the maintainer.
3. **Stale ARCHITECTURE §3.2.3**: The section still describes extracting the bearer token and
   `:path` from raw headers, which contradicts accepted ADR 036. This feature updates only the
   ExtProc sections it changes, so the stale text remains unless separately approved.
