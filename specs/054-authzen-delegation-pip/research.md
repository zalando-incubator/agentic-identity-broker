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
  token-route behavior. This extension requires the R16 approval gate, §3.1.4.2 documentation,
  and T044/T047 test-first coverage; it is not existing functionality (HIGH I2 in
  [remediation.md](./remediation.md)).

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
a proposed choice, recorded in [contracts/authzen-evaluation-api.md](./contracts/authzen-evaluation-api.md),
and it is reversible before the contract is frozen.

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

**Stakeholder**: Confirm both the 1.1.0 additive bump with the deprecation marker and the
deferral of removal to a separately approved 2.0.0 change.

## R13. ExtProc resolver boundary

**Blocked freshness decision**: The missing/invalid `max-age` error below contradicts ExtProc
contract §2 and T049. The relative lifetime also omits end-to-end age accounting. Neither
interpretation is approved by this remediation; do not implement it until HIGH I1/U1 in
[remediation.md](./remediation.md) are answered and the affected artifacts reconciled.

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
  boolean `decision`. Every other status, a malformed body, a missing or invalid `max-age` on
  a permit, or a timeout is an error.
- `resolver.go`: `Resolver` with a permit-only cache keyed by the struct
  `cacheKey{subjectToken, resourceURI}` (the same separator-safe pattern as ADR 012), a
  `sync.RWMutex`, `singleflight`, and eviction.
  - Entry lifetime is `min(max-age, max_staleness)`. A missing or invalid `max-age` is not
    cached (FR-015).
  - Denials and errors are never cached.
  - There is no stale-on-error fallback.

`server.DelegationResolver` becomes the fourth ExtProc port, next to `Exchanger`, `Authorizer`,
and `ApprovalGate`.

**Alternatives**:

- *Extend `TokenExchanger`*. Rejected because FR-014 requires a boundary independent of token
  exchange, and the exchanger's stale fallback must not apply.
- *Reuse `approval.Client`*. Rejected because it is a different bounded concern.

## R14. ExtProc request-path ordering

**Blocked policy-use decision**: The stored context below has no deadline check at later body
or batch policy use. A header-time permit cannot authorize stale-context policy input. The
stakeholder must confirm rejection versus bounded re-resolution before this path is
implemented; see HIGH U2 in [remediation.md](./remediation.md).

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
   4. Otherwise, buffer the body with the Authorization mutation and store the context in `requestState`.

**Header-only path** (`processHeadersOnlyOPA`):

1. Resolve the delegation. Apply the same denial and error responses as the body path.
2. Build the input with the context and evaluate policy.
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

**Decision**: Extend `authorization.ContextInput` with two members:

- `delegation_available *bool` (`json:",omitempty"`)
- `delegation map[string]any` (`json:",omitempty"`), decoded with `UseNumber` from the permit's
  `context` object exactly as received (verbatim, FR-017)

When the feature is enabled, both paths set `delegation_available: true` and `delegation`, and
derive `granted_permission_sets` from `delegation.permission_sets` with
`granted_permission_sets_available: true`. The header-only path gains these inputs; today it
reports `false`. When the feature is disabled, the members are omitted and the existing
behavior is byte-for-byte unchanged.

This changes ADR 028's input schema, which is merged and Accepted. It therefore needs the new
superseding ADR (R16).

## R16. Architecture decision record

**Decision**: Phase 2a drafts a new ADR with status **Proposed** at the next unused number. On
2026-10-27, `main` ends at 043, and unmerged branches `origin/046-cimd-public-client` and
`origin/fix/extproc-reauth-403-challenge` each use 044. The planned number is therefore **045**,
rechecked when the ADR is drafted. The ADR records these decisions:

- the AuthZEN Access Evaluation information boundary on the end-user server, enabled by token
  exchange, with no new broker flag;
- reuse of the ADR 009 caller CEL policy with the token-exchange grant type (R3);
- independent permit freshness, a fixed 60 s cap, and no negative or stale caching;
- the ExtProc `DelegationResolver` as a fourth port, with concurrent body-path ordering (R14);
- the superseded parts of ADR 028: the input schema (R15) and the header-only and body-bearing
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

**Stakeholder**: Accepting the ADR is a precondition for implementation. This plan accepts nothing.

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
- **Canonical-ID policy fixture**: add `tests/e2e/fixtures/policies/delegation_canonical.rego`.
  It allows when the input satisfies all of these conditions:
  - `input.context.delegation.agent.canonical_id == "data-assistant"`
  - some permission set has `canonical_id == "repo-reader"`
  - `input.context.delegation.resource.canonical_id == "repo-api"`

  The policy contains no UUIDs (FR-018 and SC-005).

## Deferred findings (not tasks of this feature)

These are actionable findings outside the feature's required work. Under the constitution
(Feature Scope & Deferred Findings), the user decides whether to file issues:

1. **Feature-number collision**: `specs/054-agent-external-id-filter` (merged on `main`) and
   this feature both use number 054. Renumbering is a process decision for the maintainer.
2. **ADR number collision**: two unmerged branches each propose ADR 044. This feature avoids
   the number. The collision between those two branches is theirs to resolve.
3. **Stale ARCHITECTURE §3.2.3**: The section still describes extracting the bearer token and
   `:path` from raw headers, which contradicts accepted ADR 036. This feature updates only the
   ExtProc sections it changes, so the stale text remains unless separately approved.
