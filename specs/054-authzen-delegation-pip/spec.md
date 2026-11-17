# Feature Specification: AuthZEN Delegation Policy Information Point

**Feature Branch**: `speckit/issue-289`

**Created**: 2026-10-26

**Status**: Original API/ADR decisions and D-6 disposition have recorded approval in quickstart §§11–12; they are not reopened. Current remediation proposes ADR-2, ADR-3, API-7, D-7 and D-8 through the plan's [Approval Items](./plan.md#approval-items). No new approval or application implementation is recorded. FR-020 specifies compatible token-result reuse with provider/provenance limitations; US3.11–US3.13 and Journeys 5–6 cover the new work.

**Input**: The title and body supplied for [issue #289](https://github.com/zalando-incubator/agentic-identity-broker/issues/289): “Broker AuthZEN Access Evaluation endpoint as Delegation Policy Information Point”. No issue comments were used.

## Clarifications

### Apply-review 2026-11-11

Reason: [F1/P1–F4/P4 proposal approved by mjungsbluth](https://github.com/zalando-incubator/agentic-identity-broker/pull/292#issuecomment-6751402783) at `2026-11-11T18:57:51Z`.

- F1/P1: enabled-mode result reuse MUST compare exchange `principal`/`agent_id` with the permit identities as well as scope sets and grant-update instants. An identity mismatch or absent exchange identity MUST return terminal API-7 503 before approval or forwarding, without an identity-repair retry. Approval identity remains exchange-derived under ADR 036. This replaces the 2026-11-09 permit override; US3.1, Journey 4 and FR-020 reflect the correction.
- F2/P2: restore ADR 028's original Accepted Status and move the existing ADR 048 partial-supersession note below the header as a banner. Its original approval is not reattributed.
- F3/P3: track the deferred `/oauth2/token` grant-version/effective-scope redesign in [Follow-ups for #292](https://github.com/zalando-incubator/agentic-identity-broker/issues?q=is%3Aissue%20in%3Atitle%20%22Follow-ups%20for%20%23292%22), also linked from FR-020 and ADR 048 §10. No redesign is implemented here.
- F4/P4: finish ExchangeResultBinding vocabulary in file names and log messages. Retain the API-7 response text and telemetry identifiers.

The earlier apply-review entries below are historical where superseded by this clarification.

### Apply-review 2026-11-09

Reason for each change below: [mjungsbluth's selected remediation proposal](https://github.com/zalando-incubator/agentic-identity-broker/pull/292#issuecomment-6729032045), approved at `2026-11-09T20:20:21Z`.

- F2/P2: In enabled mode, the approval gate MUST use the same permit principal and agent UUID as OPA, including when cached exchange identities differ or are absent. Disabled-mode approval identity and session extraction are unchanged.
- F3/P3: The additive AuthZEN paths and exchange-field deprecation ship as end-user API `1.3.0` over the released `1.2.0`; earlier `1.1.0` → `1.2.0` statements below are historical planning, superseded by this correction. ADR 028 and its architecture index explicitly record ADR 048's existing partial supersession.
- F4/P4: Invalid grant timestamps use a server-local sentinel; arbitrary permit-client error types are not timestamp failures. F5/P5: the unavailable event uses `error_kind` and includes binding-conflict and grant-timestamp emitters (observability contract §2.1).
- F6/P6: Use `ExchangeResultBinding` for local scope-set/grant-instant reuse, not IETF Token Binding, and `PermitResolver` for the ExtProc port and permit cache, not the broker `DelegationResolver`. Protocol fields, configuration keys, telemetry identifiers and API-7's response text are unchanged.
- F1/P1 remains unresolved in this run. No exchange-response provenance fields or reconciliation removal are implemented; FR-020 and its limitations still describe the code, not acceptance of the proposal's alternative.

### Session 2026-10-26

- Q: Must every evaluation request include `context.agent`, or may callers omit it and use the agent resolved from the verified subject token? → A: Option A — `context.agent` is an optional consistency anchor; omission uses the agent resolved from the verified subject token, and a supplied mismatch returns HTTP 400.
- Q: When an agent, service, or permission set lacks a configured canonical ID, should evaluation omit `canonical_id`, return null, or fail as a configuration error? → A: Option A — omit unset `canonical_id` fields, retaining real UUIDs and otherwise valid evaluation behavior without a canonical-ID rollout prerequisite.

Both answers were supplied by maintainer `mjungsbluth` as “Q1: A; Q2: A” in clarification answer [6418822603](https://github.com/zalando-incubator/agentic-identity-broker/pull/292#issuecomment-6418822603). They resolve these two product questions only; they do not approve an OpenAPI amendment or accept an ADR.

### Session 2026-10-27

- Q: Should evaluation introduce a distinct caller-policy grant type, or reuse the existing token-exchange CEL policy for the token-exchange action? → A: Reuse the same policy without a new grant type: for `action: {"name": "urn:ietf:params:oauth:grant-type:token-exchange"}`, the caller-policy input is `request.grant_type = "urn:ietf:params:oauth:grant-type:token-exchange"`.

This clarification records maintainer `mjungsbluth`'s phase guidance in command [6438164850](https://github.com/zalando-incubator/agentic-identity-broker/pull/292#issuecomment-6438164850), not an answer supplied on the maintainer's behalf. It does not approve an OpenAPI amendment or accept an ADR.

### Session 2026-10-28

- Q: Where must ExtProc expose delegation members to policies? → A: Directly in the existing `input.context`, beside `granted_permission_sets_available`, `granted_permission_sets`, and `agent_session_id`: `principal` (string from permit `subject.id`), `agent {id, canonical_id}`, `resource {service_id, canonical_id, resource_uri, scopes}`, `grant {id, updated_at, valid_until}`, and `permission_sets[] {id, canonical_id, services[] {id, canonical_id, scopes}}`. No `delegation` wrapper; unset optional `canonical_id` and `valid_until` fields are omitted, never null.
- Q: Must the policy context be a verbatim copy of the AuthZEN permit? → A: No. ExtProc decodes the permit into its own typed struct and emits only the documented fields. Unknown permit fields do not reach policies, and AuthZEN wire-format details do not leak into Rego.
- Q: How long must the existing OPA permission-set inputs remain? → A: `granted_permission_sets` and `granted_permission_sets_available` remain permanently, populated from the permit on both enabled request paths. This is separate from FR-019's unchanged exchange-response deprecation.
- Q: Which availability signal applies to the extended context? → A: Use only `granted_permission_sets_available`. In enabled mode, `true` guarantees all five new members are present, including on the header-only path, subject to nested optional-field omissions. Resolution failure returns 503 before policy, never an availability-false fallback. Disabled mode omits the new members and keeps existing behavior byte-for-byte unchanged; no `delegation_available` field is added.
- Q: Which paths must the canonical-ID policy fixture use? → A: `input.context.agent.canonical_id`, `input.context.resource.canonical_id`, and `input.context.permission_sets[].canonical_id`.

These five points record maintainer `mjungsbluth`'s phase guidance in command [6480426911](https://github.com/zalando-incubator/agentic-identity-broker/pull/292#issuecomment-6480426911), resolving R15's input-shape questions. They do not approve an OpenAPI amendment, accept an ADR, or authorize planning or implementation in this phase.

### Analysis remediation

- I1 — Proposed approval item API-3: use end-user API `1.2.0` from the existing `1.1.0` baseline for the additive endpoints and exchange-field deprecation. Reason: the earlier design incorrectly assumed a `1.0.0` baseline. Main alternative: intentionally reuse `1.1.0`, which would not identify this revision separately. This correction is not maintainer approval; later removal still requires a separately approved `2.0.0` change.
- I2 — Clarify FR-007/FR-008 and the Proposed API-2 permit contract to match the existing consumer invariants: `subject.id` is non-empty, `permission_sets` has at least one entry, and any present `canonical_id` is non-empty at every nesting level. Optional members remain omittable; empty scope/service arrays remain valid. Reason: schema-valid broker permits must interoperate with the typed consumer. Main alternative: relax consumer validation, which would admit identity-less or empty-delegation permits and contradict the domain invariants. T014/T033/T049 cover producer/schema/consumer parity.
- I3 — Replace the obsolete future planning/task-regeneration handoff with the completed reconciliation record. No product scope changes; sign-off, final-standard verification and implementation gates remain open.

### Session 2026-11-05

- Q: How does evaluation treat request members it does not support? → A: It follows the AuthZEN 1.0 HTTPS binding: receivers ignore unknown fields. Unknown top-level members, AuthZEN `properties`, and `context` members other than `agent` are ignored and never influence the decision. The supported values stay required exactly: `subject.type` is the JWT token-type URN, `action.name` the token-exchange grant-type URN, and `resource.type` is `protected_resource`. A present `context` must be an object whose `agent`, when present, is an Agent UUID string. The delegation is always resolved from the verified subject token alone. Reason: the API-003 verification found that the earlier strict rejection (research R10) conflicts with the standard; ADR 048 §1, the OpenAPI and the implementation already ignore unknown members. Main alternative: strict rejection with 400, which signals unsupported members clearly but violates the standard. Applied to FR-003, the edge cases, research R10, data model §2.5, and the evaluation contract §2.1. **Approval item**: the revised API-2 row in the plan.
- Q: When does consent given after a delegation denial take effect on the body-bearing path? → A: Once the existing ADR 012 exchange re-consent cooldown (5 s) lapses. The concurrent exchange during the denied request records that cooldown, and this feature leaves the exchange cache unchanged; the header-only path permits immediately and the delegation resolver never caches denials. Reason: ADR 048 Consequences record this behavior and the maintainer accepted it on 2026-10-31; US3.5 previously implied an immediate permit on both paths. Main alternative: bypass the re-consent cooldown in enabled mode, which changes ADR 012 exchange-cache semantics. Applied to US3.5. **Approval item** D-5 in the plan.

These points record maintainer `mjungsbluth`'s planning direction in command [6659711535](https://github.com/zalando-incubator/agentic-identity-broker/pull/292#issuecomment-6659711535). They do not themselves approve an API change or decision; sign-off remains the approval comment for the plan's Approval Items.

### Analysis remediation (run 42039632078/1)

- C1 — **Proposed approval item D-6**: request a one-time exception under Constitution VIII/XIII's “When Constraints Cannot Be Met” procedure for the original test-first/compile-only sequencing that was not performed. [historical-validation-exception.md](./historical-validation-exception.md) defines the affected obligations, independent test review, 44-scenario mapping and unchanged validation gates. Reason: `quickstart.md` §§7–8 admits tests authored alongside implementation; later green runs or retrospective stubs cannot restore that historical order. Main alternative: decline the exception and retain the obligations as unresolved, without declaring feature completion. All new corrections still require genuine test-first development; no requirement, scenario, security guarantee or failing check is waived. No exception or approval is recorded here.
- I1 — Separate design coverage from historical execution in the plan's gate table: all 44 scenarios remain mapped, but original test-first/scaffolding compliance is unresolved under Proposed D-6. Reason: the former Pass entries contradicted the evidence. No product requirement changes.
- I2 — Preserve API-003's final-standard verification requirement while identifying the TLS-download failure as historical and linking the completed verification and ignore-unknown-fields outcome in `quickstart.md` §7. No contract change.

### Task reconciliation

- Q: Does Journey 2 promise immediate body-path forwarding after renewed consent? → A: No. Immediate broker re-evaluation is distinct from downstream forwarding: the header-only path permits immediately, while the body path waits for the unchanged 5 s exchange re-consent cooldown recorded in US3.5. Reason: keep the concrete journey consistent with the 2026-11-05 clarification and Proposed D-5. Main alternative: bypass that cooldown, which changes exchange-cache semantics and remains outside this proposal. No additional scope or approval is implied.

### Task reconciliation — permit-bound token continuation

The current task list adds US3.11/FR-020 coverage, binding equality and concurrency tests, the mandatory observability design inventory and fresh compliance gates. The original D-6 disposition covers only the historical 44 scenarios, not this new work. F4 naming is applied consistently to the ExtProc port and `resource.service_id`; references to the feature ADR are corrected to ADR 048. No requirement, scope or approval changes in this reconciliation. Reason: align executable tasks and supporting documents with the Proposed F1 design while preserving historical evidence.

### Review remediation 2026-11-06

Reason for the following scoped requirement changes: `mjungsbluth` selected P3/P4/P5
(F4/F5/F6) on [the approved remediation proposal](https://github.com/zalando-incubator/agentic-identity-broker/pull/292#issuecomment-6669627407)
at `2026-11-06T06:59:39Z`.

- **F4 (FR-007/FR-014/FR-017)**: The permit and enabled OPA context use
  `resource.service_id` for the owning service UUID, distinct from the unchanged AuthZEN
  request's URI-valued `resource.id`. The ExtProc port is `PermitResolver`, not
  the broker domain's `DelegationResolver`. Delegation and Access Evaluation are glossary terms.
- **F5 (SR-004)**: A conformance test under `tests/` must compare the complete domain denial
  code set, ExtProc denial allowlist, and nested producer/consumer context field sets to OpenAPI.
- **F6 (SR-004)**: ExtProc security logs identify denial, unavailability and permit expiry
  with `extproc.delegation_context.{denied,unavailable,permit_expired}` events.

P1/F2 is selected but unapplied: the push allowlist rejects its required namespace
registration in `coding-standards/OPENTELEMETRY.md`. Its code, tests and docs remain unchanged.
P2/F3 is also selected but unapplied: filing its required follow-up issue is unavailable to
this workflow. Existing enabled-mode compatibility requirements remain unchanged pending that work.
No choice about the unassigned F1 scope-authority question is made here; the planning
direction below later resolves it.

### Analysis remediation (run 42232121230/1)

- I1/U1: compare opaque entry identity, not credential bytes; atomically first-claim shared result bindings and revalidate retry delivery. Repeated conflict returns deterministic API-7 503. Reason: protect same-token replacements and concurrent live permits. Alternative: token comparison/rebind/unbounded retry, rejected as unsafe. ADR-2/API-7 remain Proposed.
- I2: reconcile FR-020/SC-007 with compatible reuse rather than downscoping or exchange provenance. Add US3.12/US3.13 and Journey 6 for unchanged provider capability and concurrent first use. Reason: unchanged exchange contract supplies neither narrowing nor grant attestation. Alternative: stronger downscoping/exchange-version API, requiring separate scope approval. Limited guarantee is Proposed D-8, not silently substituted for F1.
- C1/A1: exact telemetry identifiers, explicit kinds and required namespace rows are specified in observability §3. T103 remains mandatory; publication permission is an external prerequisite, not a standards waiver. Breaking migration is Proposed D-7. Alternative: exception for old identifiers, rejected.
- U2: designate `input.context.permission_sets` canonical for new enabled policies; preserve both compatibility members today. Revise permanent retention only to allow future separately approved removal with an issue/migration. Reason: implement selected F3 without current breakage. Alternative: permanent duplicate fields. Proposed ADR-3; T104 requires maintainer-created issue.
- I3/I4: repair approval locations and active status; original API-2/D-5 approval and D-6 closure remain recorded in quickstart §§11–12, not reopened. New revisions need new supplied approval.

### Planning direction 2026-11-06 (F1 — historical; clarified above)

- Q: When a fresh permit's scopes or grant version differ from those of the exchanged token
  that ExtProc has cached, which one governs what goes upstream? → A: The permit is
  authoritative for upstream scopes. In enabled mode ExtProc must not forward a cached ADR 012
  token whose binding (the permit's `resource.scopes` and `grant.updated_at` at exchange or
  first use) differs from the current permit. It removes that cache entry and exchanges again
  before forwarding; a failed re-exchange never falls back to the removed token. Reason:
  maintainer `mjungsbluth`'s planning command for this run; before it, policy saw a narrowed
  grant within `min(max-age, max_staleness)` while upstream could receive a token exchanged
  under the broader grant until `cache.max_ttl`. Main alternative: forward the cached token
  until its ADR 012 expiry. Added FR-020, US3.11, SC-007, an edge case, and Journey 5.
  **Approval item** ADR-2 in the plan ([ADR 048 revision draft](./adr-048-revision-permit-bound-tokens.md), contract §3.1).
- Limitation recorded with the decision: the broker exchanges the provider session's current
  token, whose scopes come from provider authorization, not from the grant. Upstream receives
  narrowed scopes when the provider session carries them (Journey 5: Alice re-authorizes the
  service). This feature does not narrow provider tokens in the broker. Main alternative:
  require broker-side narrowing, which changes provider-session behavior outside this feature.

## User Scenarios & Testing *(mandatory)*

The broker tells an authenticated gateway whether a user's delegation to an agent covers one protected resource. It returns credential-free policy information, not a provider credential or a final decision about a tool invocation. ExtProc can opt into this information independently of its exchanged-token lifetime, so revocations stop informing policy within a bounded freshness window and header-only requests receive the same information as body-bearing requests.

Scenario identifiers below are stable acceptance-test references. Each numbered scenario MUST map to exactly one complete-journey E2E test in `tests/e2e/` (broker) or `tests/e2e/extproc/` (consumer). The denial matrix rows are separate scenarios, not one combined test. E2E tests must compile and fail semantically before implementation; this specification phase creates no tests or implementation.

### User Story 1 - A gateway obtains current delegation information (Priority: P1)

As a gateway operator, I want to discover the broker's evaluation endpoint and obtain verified delegation information without retrieving the user's provider credentials, so that my policies can use current grants and recognizable identifiers.

**Why this priority**: This is the independently useful broker increment and enables other AuthZEN-capable gateways, not only ExtProc.

**Independent Test**: Register an agent and protected resource, establish a user's grant, and use an authorized gateway assertion to discover and evaluate that delegation through the real end-user server. No ExtProc deployment or provider session is needed.

**Acceptance Scenarios**:

1. **US1.1 — Discovery**: **Given** token exchange is configured and the public broker URL is `https://broker.example.com`, **When** a gateway reads `/.well-known/authzen-configuration` on the end-user server, **Then** it receives `policy_decision_point: https://broker.example.com` and `access_evaluation_endpoint: https://broker.example.com/access/v1/evaluation`, derived from the same public base used for consent links and RFC 8414 issuer metadata.
2. **US1.2 — Permit**: **Given** Alice's valid subject token identifies a registered agent, the gateway is authenticated and authorized, and Alice's active grant covers the resource, **When** the gateway evaluates it with `X-Request-ID: delegation-001`, first with a matching `context.agent` and then without `context.agent` (including omission of the whole request `context` object), **Then** both evaluations receive HTTP 200 with `decision: true`, the same token-derived agent and credential-free Delegation Context described in FR-007, the same request ID, and a private cache lifetime of at most 60 seconds. The unchanged caller CEL policy authorizes both evaluation and token exchange for the same verified subject, gateway, and resource using the existing token-exchange grant-type value.
3. **US1.3 — Delegation is not credential availability**: **Given** the same covered delegation but no usable third-party provider session, **When** the gateway evaluates it and separately attempts token exchange, **Then** evaluation still permits without accessing or refreshing a provider credential, while exchange retains its existing session error behavior.
4. **US1.4 — Effective scope ceiling**: **Given** the granted permission sets provide `read:repo` and `write:repo` but the agent's service requirement permits only `read:repo`, **When** the gateway evaluates the resource, **Then** `resource.scopes` contains only `read:repo`, and exchanging for that resource continues to apply the same scope ceiling.
5. **US1.5 — Resource normalization**: **Given** a registered resource URI and a valid covering grant, **When** the gateway evaluates an equivalent URI under existing token-exchange normalization rules, **Then** it obtains the same service identity and delegation decision as token exchange, not a new resource-resolution rule.
6. **US1.6 — Grant expiry bounds caching**: **Given** a covering grant expires in 15 whole seconds, **When** the gateway evaluates it, **Then** the permit advertises at most 15 seconds of freshness, and evaluation at expiry returns the expired-grant denial rather than a permit.
7. **US1.7 — No new broker switch**: **Given** a broker deployment without configured token exchange, **When** a gateway attempts discovery and evaluation, **Then** neither route provides a usable AuthZEN service; configuring token exchange makes both available on the end-user server without an additional AuthZEN configuration flag.
8. **US1.8 — Optional canonical metadata**: **Given** an otherwise valid delegation whose agent, service, or permission set has no canonical ID, **When** it is evaluated, **Then** evaluation still permits with the real UUIDs, omits every unset `canonical_id` field (including nested permission-set service entries), preserves configured canonical IDs, and neither returns null or fabricated aliases nor fails because a canonical ID is absent.

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
17. **US2.17 — Unauthorized gateway**: **Given** valid subject and assertion credentials but an unchanged token-exchange caller policy that rejects this gateway or resource with `request.grant_type = "urn:ietf:params:oauth:grant-type:token-exchange"`, **When** evaluation is requested for that subject and resource, **Then** it receives HTTP 403, not HTTP 200 with a delegation-denial reason; evaluation cannot evade the policy that also rejects the equivalent token-exchange request.
18. **US2.18 — Agent mismatch**: **Given** the token resolves to the Data Assistant, **When** the request anchors `context.agent` to another registered agent, **Then** evaluation returns HTTP 400 and never evaluates the other agent's grant.
19. **US2.19 — Dependency failure**: **Given** valid credentials and a grant-repository outage, **When** the gateway evaluates the delegation, **Then** it receives HTTP 500 without a permit, cached success, or fabricated consent-recovery decision.
20. **US2.20 — Unregistered resource**: **Given** valid credentials, **When** the gateway evaluates an unregistered protected-resource URI, **Then** it receives HTTP 400 without pretending a service grant exists.

### User Story 3 - ExtProc supplies fresh context before every policy decision (Priority: P2)

As an operator who enables delegation context, I want both request paths to evaluate policy against a bounded-freshness Delegation Context, so a token cached for an hour does not keep supplying revoked permissions for an hour.

**Why this priority**: This consumes the additive broker capability and fixes lifetime and ordering coupling without forcing existing ExtProc deployments to migrate immediately.

**Independent Test**: Enable the resolver with a broker and policy fixture. Send metadata-validated body-bearing and header-only requests through the real ExtProc service; inspect the downstream result and policy-observed canonical IDs. Use controlled times for cache expiry.

**Acceptance Scenarios**:

1. **US3.1 — Body-bearing permit**: **Given** enabled delegation context and an allowed delegation, **When** a body-bearing request reaches ExtProc, **Then** exchange and delegation resolution run concurrently during request headers, both must succeed before body policy evaluation, and policy receives the documented `principal`, `agent`, `resource`, `grant`, and `permission_sets` members directly in `input.context`, with permit-derived `granted_permission_sets` and `granted_permission_sets_available: true`, before any downstream forwarding. ExtProc maps its own typed permit struct rather than copying the wire context; unknown permit fields, a `delegation` wrapper, and `delegation_available` are absent. Unset optional `canonical_id` and `valid_until` fields are omitted, never null. Exchange principal and agent UUID must match this permit; a mismatch or absent exchange identity returns terminal API-7 503 before body policy, approval creation or forwarding. If policy requires approval, the gate uses the matching exchange response identity under ADR 036.
2. **US3.2 — Header-only permit**: **Given** the same delegation, **When** a supported header-only request reaches ExtProc, **Then** it resolves delegation before policy evaluation, policy sees the same documented direct `input.context` members and permit-derived `granted_permission_sets` with `granted_permission_sets_available: true` as the body path, and exchange occurs only after policy allows.
3. **US3.3 — Delegation denial**: **Given** the broker returns `decision: false` with Alice's consent URL, **When** ExtProc handles the request, **Then** it returns HTTP 403 `access_denied` with that recovery URL before policy evaluation or downstream forwarding; a concurrent successful exchange cannot override the denial.
4. **US3.4 — Independent freshness**: **Given** a permit cached at 10:00:00 for 60 seconds and an exchanged token usable until 11:00:00, **When** Alice revokes her grant at 10:00:10 and repeats her request at 10:01:00, **Then** ExtProc resolves again and denies instead of feeding the hour-old permission snapshot to policy.
5. **US3.5 — Denials not cached**: **Given** a denied request and subsequent valid user consent, **When** Alice repeats the request immediately, **Then** ExtProc consults the broker again without waiting for a delegation negative-cache lifetime: a header-only request permits immediately, and a body-bearing request permits once the existing ADR 012 exchange re-consent cooldown (5 s), recorded by the concurrent exchange during the denied request, lapses.
6. **US3.6 — No stale-on-error**: **Given** an expired delegation permit but a still-cached exchanged token, **When** resolution times out or the broker is unavailable, **Then** ExtProc returns 503 before policy and does not substitute stale delegation, exchange-response permission sets, or `granted_permission_sets_available: false` that policy could allow.
7. **US3.7 — Cache isolation and local cap**: **Given** two different subject tokens or resource URIs, and `max_staleness: 20s`, **When** a 60-second permit is cached for one pair, **Then** another pair cannot use it and the original pair requires revalidation after at most 20 seconds.
8. **US3.8 — Canonical policy and compatibility projection**: **Given** a valid permit and a policy keyed on `input.context.agent.canonical_id`, `input.context.resource.canonical_id`, and `input.context.permission_sets[].canonical_id` rather than deployment UUIDs, **When** equivalent configured entities use different UUIDs, **Then** policy makes the same decision on both request paths, the documented direct context members retain the corresponding permit values through the typed mapping, and currently retained compatibility `granted_permission_sets` and `granted_permission_sets_available: true` are derived from the permit, not the exchange response. Unknown permit fields never become policy inputs.
9. **US3.9 — Policy denial still wins**: **Given** a permitted delegation but a header-only request forbidden by policy, **When** ExtProc evaluates it, **Then** it denies without invoking exchange or forwarding downstream; delegation permission is not tool-call permission.
10. **US3.10 — Exchange failure still wins**: **Given** a permitted delegation but an unusable provider session, **When** a body-bearing request triggers concurrent exchange and resolution, **Then** exchange failure prevents forwarding despite successful delegation resolution and preserves existing exchange-error handling.
11. **US3.11 — Narrowed grant reaches upstream**: **Given** enabled delegation context, a permit with `resource.scopes` `["read:repo","write:repo"]`, and a forwarded result first bound to it that the ADR 012 cache still holds, **When** Alice narrows her grant to the reader permission set, re-authorizes Repository API so that the provider session holds only `read:repo`, and repeats body-bearing and header-only requests after the cached permit's deadline, **Then** ExtProc does not forward the cached token: it exchanges again before forwarding, upstream receives the token of the `read:repo` session, and policy sees `resource.scopes` `["read:repo"]` and the new `grant.updated_at`. A failed re-exchange forwards nothing. Upstream narrowing is conditional on provider re-authorization, not a downscoping guarantee.
12. **US3.12 — Unchanged provider capability**: **Given** a broad provider session and a cached result bound to the older broad permit, **When** Alice narrows her grant without provider re-authorization and requests after permit expiry on both paths, **Then** policy sees new effective scopes, ExtProc rejects the old handle and performs a bounded extra exchange, but may forward identical broader provider token bytes under a new compatible handle. No downscoping or exchange-version attestation is claimed.
13. **US3.13 — Concurrent live permits**: **Given** enabled ExtProc and two authorized requests for the same subject/resource with different still-live permits, **When** barrier-controlled exchanges share a result first claimed by one permit and the other request receives an incompatible handle again on its one additional exchange, **Then** the losing request gets API-7 503 with no Authorization forwarding, rebind, stale fallback or further retry; the compatible request succeeds and its replacement survives even if token bytes equal the old entry. Controlled broker barriers exercise differing grant reads and unbound first claim on both paths.

### User Story 4 - Operators roll out discovery and migrate safely (Priority: P2)

As an operator, I want startup to reject discovery mix-ups and invalid opt-in settings, while disabled consumers and existing token-exchange identities remain compatible.

**Why this priority**: A gateway must not silently send a subject credential to a mismatched service, and operators need a deliberate migration window.

**Independent Test**: Start ExtProc with each configuration or metadata condition and observe readiness or rejection; compare disabled-mode behavior with today's exchange and policy behavior.

**Acceptance Scenarios**:

1. **US4.1 — Disabled by default**: **Given** an existing ExtProc configuration with no delegation-context block, **When** it starts and serves requests, **Then** it performs no AuthZEN discovery or resolution, omits the new `principal`, `agent`, `resource`, `grant`, and `permission_sets` policy-context members, and retains existing token-exchange and policy behavior byte-for-byte, including existing permission-set availability semantics.
2. **US4.2 — Authorization dependency**: **Given** `delegation_context.enabled: true` and `authorization.enabled: false`, **When** ExtProc starts, **Then** startup fails with an actionable, credential-free configuration error.
3. **US4.3 — Discovery failure**: **Given** delegation resolution is enabled but metadata is unavailable or invalid, **When** ExtProc starts, **Then** startup fails rather than guessing an evaluation endpoint or silently disabling the resolver.
4. **US4.4 — Metadata mix-up**: **Given** the expected policy-decision-point identifier is `https://broker.example.com` but discovery advertises `https://other.example.com`, **When** ExtProc starts, **Then** it fails without sending any evaluation credentials to the mismatched service.
5. **US4.5 — Default discovery target**: **Given** delegation resolution is enabled, `oauth2.token_endpoint` is `https://broker.example.com/oauth2/token`, and no policy-decision-point override is supplied, **When** ExtProc starts, **Then** it discovers from `https://broker.example.com/.well-known/authzen-configuration`, verifies the identifier, and uses the advertised evaluation endpoint.
6. **US4.6 — Deprecation window**: **Given** the resolver-shipping release, **When** existing clients exchange a token, **Then** `granted_permission_sets` remains available but documented as deprecated, and `principal` and `agent_id` remain unchanged; removal of the deprecated field is scheduled for one release after the resolver ships, not the additive broker-only release.

### Edge Cases

- A grant with no `valid_until` is still bounded by the 60-second permit cap; a grant with less than one second remaining must not be rounded up into a reusable permit. At expiry it denies.
- Canonical IDs are optional, case-sensitive metadata, not replacements for UUID identity. An unset `canonical_id` is omitted wherever that entity appears, including nested service entries; it is not null, a synthetic alias, or a UUID-as-canonical-ID fallback. Its absence alone never changes a delegation decision or makes evaluation unavailable.
- `context.agent` is optional and cannot override a verified subject-token claim. Omission, including omission of request `context`, uses the verified token's resolved agent; supplied mismatches always return 400.
- Unknown request members, AuthZEN `properties`, and `context` members other than `agent` (for example `context.actor`) are ignored, as the AuthZEN 1.0 HTTPS binding requires. They never influence the decision, and they cannot supply a principal, agent, actor, or resource. A present `context` that is not an object, or a non-string `context.agent`, returns 400.
- Resource normalization, multiple permission-set scope union, included-service filtering, `require_all_scopes`, service-requirement exclusion, and empty-intersection rules remain identical to exchange. Declared zero-scope services must retain their existing distinction from scopes capped to zero.
- Unknown or invalid agent identity and ambiguous resource registration are configuration failures, not evidence of user non-consent. Dependency/configuration errors must not be disguised as the grant-denial matrix.
- Missing, unsupported, or invalid subject/resource/action values are request failures; arbitrary asserted `identity` subjects are explicitly unsupported.
- Both permit and deny decisions echo a supplied `X-Request-ID`; request errors must also echo it. Existing trace correlation remains separate and unchanged. No new request-ID generation requirement is introduced.
- Missing/invalid cache freshness must not extend the consumer's freshness allowance; denies and request errors are not reusable permits. Expired delegation cannot use the token cache's outage fallback.
- A public broker identifier with a path prefix may differ from the origin default. Operators must set the explicit policy-decision-point identifier; comparison must not strip a path or otherwise turn a mismatch into a match.
- Both concurrent operations must complete successfully before the body path proceeds. Cancellation or completion order cannot turn a deny/error into a forwarded request.
- Result binding records first permit use, not minting provenance. New handles may carry identical provider tokens; deletion compares entry identity, not token bytes. Concurrent first claims are serialized; a mismatched additional-exchange delivery fails closed with API-7 503, not rebinding or indefinite retries. Scope order and timestamp formatting do not change equality; cooldown stays unchanged.
- Existing strict gateway JWT validation, metadata-only token/resource inputs, MCP transport checks, body limits, approval-gate behavior, and policy fail-closed behavior remain in force.

## Concrete Example Journeys *(mandatory)*

All identifiers, timestamps, token markers, and claims below are synthetic. HTTP examples follow the feature contract, including its verified denial form (a); `quickstart.md` §7 records API-003 final-standard evidence. Revised API-2 and D-5 remain Proposed for current sign-off. Existing token-exchange and consent contracts are not changed by these examples.

**Shared cast**:

| Concept | Synthetic value |
| --- | --- |
| Principal | `alice@example.com` |
| Agent | Data Assistant, UUID `550e8400-e29b-41d4-a716-446655440000`, canonical ID `data-assistant` |
| Service | Repository API, UUID `880e8400-e29b-41d4-a716-446655440010`, canonical ID `repo-api` |
| Protected resource | `https://api.example.com/repos` |
| Permission set | Repository Reader, UUID `770e8400-e29b-41d4-a716-446655440001`, canonical ID `repo-reader` |
| Second permission set (Journey 5) | Repository Maintainer, UUID `770e8400-e29b-41d4-a716-446655440002`, canonical ID `repo-maintainer`, scopes `read:repo write:repo` on Repository API |
| Grant | UUID `990e8400-e29b-41d4-a716-446655440020`; updated `2026-10-26T09:59:00Z`; expires `2026-10-26T10:30:00Z` |
| Gateway assertion subject | `gateway-prod` |
| Subject token marker | `SYNTHETIC_SUBJECT_JWT` |
| Assertion marker | `SYNTHETIC_CLIENT_ASSERTION_JWT` |

### Journey 1 — Discover and permit a delegation (User Story 1, US1.1–US1.6, US1.8)

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
      expression: 'client_assertion.sub == "gateway-prod" && request.grant_type == "urn:ietf:params:oauth:grant-type:token-exchange"'
```

This existing caller policy is used unchanged for both token exchange and evaluation. Journey 1's supported `action` maps to the same `request.grant_type` value; the verified assertion, subject, extracted principal/agent, and requested resource retain their existing CEL meanings. Evaluation introduces no grant type or separate caller-policy discriminator and does not issue a token.

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
}
```

`resource.service_id` identifies the resolved service, not the protected-resource child record. The example uses a string Agent UUID anchor and a minimal principal subject object as proposed contract representations; these require explicit contract review. The response subject is the verified principal, never the original JWT. The optional anchor checks consistency only; it never supplies the agent identity in place of verified token extraction. The following variant illustrates the maintainer's omission decisions.

**Step 3a — Optional metadata variant: 10:00:01 (US1.2, US1.8)**

In an otherwise identical fixture, the agent, service, and permission set have no configured canonical IDs. The gateway omits the request `context` object, so the agent is resolved solely from the verified subject token. Omitting just `context.agent` has the same effect. Configured canonical IDs, when present as in step 3, remain in the response.

```http
POST /access/v1/evaluation HTTP/1.1
Host: broker.example.com
Authorization: Bearer SYNTHETIC_CLIENT_ASSERTION_JWT
Content-Type: application/json
X-Request-ID: delegation-optional-001

{
  "subject": {"type": "urn:ietf:params:oauth:token-type:jwt", "id": "SYNTHETIC_SUBJECT_JWT"},
  "action": {"name": "urn:ietf:params:oauth:grant-type:token-exchange"},
  "resource": {"type": "protected_resource", "id": "https://api.example.com/repos"}
}

HTTP/1.1 200 OK
Content-Type: application/json
X-Request-ID: delegation-optional-001
Cache-Control: private, max-age=60

{
  "decision": true,
  "context": {
    "subject": {"id": "alice@example.com"},
    "agent": {"id": "550e8400-e29b-41d4-a716-446655440000"},
    "resource": {
      "service_id": "880e8400-e29b-41d4-a716-446655440010",
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
      "services": [{
        "id": "880e8400-e29b-41d4-a716-446655440010",
        "scopes": ["read:repo"]
      }]
    }]
  }
}
```

**Step 4 — Expiry variant: 10:29:45**

The same valid delegation returns the same entities with `Cache-Control: private, max-age=15`. At 10:30:00, evaluation returns `decision: false` with `grant_expired`. With no provider session, step 3 still permits; a subsequent `/oauth2/token` exchange fails with its existing session-recovery error instead of returning a provider credential.

**Observable result**: The gateway knows Alice delegated `read:repo` to `data-assistant` for `repo-api`, without receiving, decrypting, refreshing, or depending on a provider token.

### Journey 2 — Revocation yields consent recovery, not a reusable denial (User Story 2, US2.1–US2.10; User Story 3, US3.4–US3.5)

**Step 1 — 10:00:00: ExtProc obtains Journey 1's permit**

The exchanged token remains usable in its separate cache until 11:00:00. The delegation permit is fresh only until 10:01:00.

**Step 2 — 10:00:10: Alice revokes her delegation through the existing consent journey**

The active grant is removed. This feature changes no consent screen or revocation API.

**Step 3 — 10:01:00: the gateway repeats Journey 1's evaluation request with `X-Request-ID: delegation-002`**

Denial form (a), as recorded in the feature contract and API-003 verification:

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

Because delegation denials are not cached, the gateway consults the broker and can obtain Journey 1's permit again. No delegation negative-cache wait is imposed. The header-only path can forward immediately after policy and exchange allow. On the body path, a concurrent exchange during the earlier denied request may have recorded the existing ADR 012 exchange re-consent cooldown (5 s); body-path forwarding resumes once that cooldown lapses. This does not delay broker re-consultation or cache a delegation denial (US3.5; Proposed D-5).

**Observable result**: Revocation stops supplying delegation to policy by 10:01:00 despite a usable cached provider token; restored consent is recognized on the next evaluation, with immediate header-only forwarding and body-path forwarding after any existing exchange cooldown.

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
Content-Type: application/json
WWW-Authenticate: Bearer
X-Request-ID: delegation-003
Cache-Control: no-store

{"error":"unauthorized","message":"client authentication required"}
```

This completes the example using the existing feature contract's `ErrorResponse` and missing-credential challenge; it introduces no new error code or response shape.

**Step 2 — The gateway corrects its assertion but submits `subject.type: identity`**

Keeping the other request fields unchanged yields HTTP 400 with `X-Request-ID: delegation-003` and no permit. A valid signed but expired subject likewise yields 400; an invalid or expired gateway assertion yields 401; a valid gateway rejected by the unchanged token-exchange caller policy yields 403; a grant-repository outage yields 500. With valid credentials, changing `action.name` to `read` yields 400 rather than selecting another policy or grant type. Those errors are not transformed into HTTP 200 `decision: false` responses.

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

The gateway's existing strict JWT validation supplies `aib.tokenexchange.subject_token = SYNTHETIC_SUBJECT_JWT` and `aib.tokenexchange.resource_uri = https://api.example.com/repos`. Raw headers cannot override them. ExtProc concurrently exchanges the token and performs Journey 1 step 3 using its existing assertion provider. After both succeed, ExtProc maps its own typed permit struct into these members of `input.context` (other existing members, including `agent_session_id` when set, are unchanged):

```json
{
  "granted_permission_sets_available": true,
  "granted_permission_sets": {
    "770e8400-e29b-41d4-a716-446655440001": ["880e8400-e29b-41d4-a716-446655440010"]
  },
  "principal": "alice@example.com",
  "agent": {
    "id": "550e8400-e29b-41d4-a716-446655440000",
    "canonical_id": "data-assistant"
  },
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

The `principal` string comes from the permit's `subject.id`; no `subject` member, `delegation` wrapper, or `delegation_available` flag is added to OPA input. Unknown permit fields are ignored rather than exposed to Rego. In Journey 1's optional-metadata variant, the corresponding `canonical_id` members are omitted; for a grant with no expiry, `grant.valid_until` is omitted, never null. A policy matching `input.context.agent.canonical_id == "data-assistant"`, `input.context.resource.canonical_id == "repo-api"`, and an entry in `input.context.permission_sets` with `canonical_id == "repo-reader"` allows this request. Only the exchanged credential is forwarded downstream.

**Step 3a — Identity mismatch fails closed before approval (US3.1, approved F1/P1 on 2026-11-11)**

Alice's permit identifies Alice and the resolved agent UUID. If the exchange response carries
a different principal or agent UUID, or omits either field, ExtProc returns API-7 503
`service_unavailable` / `delegation token binding unavailable` during request headers.
It emits no Authorization mutation, buffers no body, creates no approval for either identity
and does not retry to repair identity. US3.1 exercises this through the real broker and
production ExtProc. When both identities match and OPA requires approval, the gate still uses
the exchange response under ADR 036. Session IDs and disabled-mode identity are unchanged.

**Step 4 — The same subject makes a header-only request**

ExtProc resolves the same pair (or uses its still-fresh permit), supplies the same direct context members and permit-derived `granted_permission_sets` with `granted_permission_sets_available: true` to policy, then exchanges only if policy allows. If policy denies, exchange is not called. A resolver denial instead returns 403 `access_denied` with the consent URL before policy; a resolution failure returns 503 and never falls through as `granted_permission_sets_available: false`. An expired permit never becomes stale policy input. A successful resolution with a failed exchange also cannot forward a request.

**Step 5 — Operator migrates policy while retaining compatibility**

Canonical-ID policy uses `input.context.agent.canonical_id`, `input.context.resource.canonical_id`, and `input.context.permission_sets[].canonical_id` and works for the same named entities even if another deployment assigns different UUIDs. The canonical field for new enabled policies is `input.context.permission_sets` (Proposed ADR-3). Both compatibility OPA members remain permit-derived today, with no removal or deprecation in this increment. Future removal needs its own issue, versioned contract approval and migration. Existing `agent_session_id` behavior remains unchanged. Disabling the feature omits the five new members, performs no discovery or resolution, and preserves existing behavior byte-for-byte. Separately, existing exchange `principal` and `agent_id` fields remain; only the exchange-response `granted_permission_sets` field is documented as deprecated and removed one release after the resolver ships under the separately approved contract change (FR-019).

**Observable result**: Both request paths have delegation information before policy evaluation, and no permit, cache hit, or compatibility field bypasses caller authentication, delegation freshness, policy denial, exchange failure, or the existing approval gate.

### Journey 5 — A narrowed grant reaches upstream before the cached token expires (User Story 3, US3.11; Proposed ADR-2)

Journey 4's ExtProc configuration applies (`max_staleness: 60s`). Alice's grant starts with the Repository Maintainer permission set.

**Step 1 — 10:00:00: a body-bearing request is forwarded under the broad grant**

The permit's relevant members are:

```json
{
  "resource": {"service_id": "880e8400-e29b-41d4-a716-446655440010", "resource_uri": "https://api.example.com/repos", "scopes": ["read:repo", "write:repo"]},
  "grant": {"id": "990e8400-e29b-41d4-a716-446655440020", "updated_at": "2026-10-26T09:59:00Z"}
}
```

Exchange returns the provider session's token `SYNTHETIC_PROVIDER_TOKEN_RW` (session scopes `read:repo write:repo`, `expires_in: 3600`). ExtProc caches it until 11:00:00 (subject to `cache.max_ttl`), binds it to `{read:repo, write:repo}` and `09:59:00Z`, and forwards:

```http
Authorization: Bearer SYNTHETIC_PROVIDER_TOKEN_RW
```

**Step 2 — 10:00:20: Alice narrows the grant through the existing consent journey**

She replaces Repository Maintainer with Repository Reader. The grant keeps its UUID; `updated_at` becomes `2026-10-26T10:00:20Z`. She then re-authorizes Repository API through the existing provider journey, and the provider grants only `read:repo`, so the provider session now holds `SYNTHETIC_PROVIDER_TOKEN_R`. This feature changes neither journey.

**Step 3 — 10:01:00: the same request arrives after the permit deadline**

ExtProc re-resolves. The new permit carries:

```json
{
  "resource": {"service_id": "880e8400-e29b-41d4-a716-446655440010", "resource_uri": "https://api.example.com/repos", "scopes": ["read:repo"]},
  "grant": {"id": "990e8400-e29b-41d4-a716-446655440020", "updated_at": "2026-10-26T10:00:20Z"}
}
```

The concurrent exchange returns cached `SYNTHETIC_PROVIDER_TOKEN_RW` with handle G1. Its binding differs, so ExtProc does not forward G1. It deletes only G1 if still current, exchanges once more and atomically claims or compares new handle G2 against `{read:repo}` and `10:00:20Z`. In this isolated journey G2 is compatible; policy evaluates with `input.context.resource.scopes == ["read:repo"]`. An incompatible shared G2 instead returns API-7 503 (Journey 6). Upstream receives:

```http
Authorization: Bearer SYNTHETIC_PROVIDER_TOKEN_R
```

A header-only request behaves the same way after policy allows. If the re-exchange fails, the existing exchange-error response is returned and `SYNTHETIC_PROVIDER_TOKEN_RW` is not used as a fallback.

**Observable result**: Upstream receives the narrowed-scope token within the permit freshness bound (`min(60 s, max_staleness)`), not at the 11:00:00 token-cache expiry. Without Step 2's re-authorization, the re-exchange would still re-validate the narrowed grant, but the broker would return the unchanged provider token; narrowing provider tokens is outside this feature.

### Journey 6 — Same provider token, distinct result ownership (US3.12–US3.13; Proposed ADR-2/API-7/D-8)

Use Journey 5's configuration and actors, but do not re-authorize Repository API after narrowing Alice's grant. The permit now has `scopes: ["read:repo"]` and `updated_at: "2026-10-26T10:00:20Z"`; the provider session still holds `SYNTHETIC_PROVIDER_TOKEN_RW`.

1. The next body/header request obtains the new permit, rejects old handle G1 and performs one additional broker exchange. That exchange returns the **same** provider token bytes under new handle G2. First claim binds G2 to the reader permit. OPA sees reader scopes, but upstream still sees:

   ```http
   GET /repos HTTP/1.1
   Host: api.example.com
   Authorization: Bearer SYNTHETIC_PROVIDER_TOKEN_RW
   ```

   This is the documented capability limitation, not broker downscoping. No token claim or response member attests which grant exchange read.
2. In a separate isolated concurrency run, two authorized requests use live permits A (reader) and B (reader/writer). Broker barriers hold exchange/evaluation reads around Alice's grant update. Both callers receive an unbound shared handle G3; B's atomic first claim wins. A cannot rebind G3. Its deletion attempt must preserve any concurrent G4 replacement, even if G4 holds the same `SYNTHETIC_PROVIDER_TOKEN_RW` bytes under B's binding.
3. A reads the replacement once and finds it incompatible. Its one additional exchange joins a shared flight whose handle G5 was already bound to B. A returns the following immediate response; it emits no Authorization mutation, performs no third exchange and leaves B's entry intact. B may forward its compatible result. Repeat the controlled flow for each request path.

   ```http
   HTTP/1.1 503 Service Unavailable
   Content-Type: application/json

   {"error":"service_unavailable","error_description":"delegation token binding unavailable"}
   ```

The first-claim serialization guarantees compatible handle reuse, not matching broker grant reads. Unit tests use barriers for every claim/delete/delivery permutation; US3.13 has one complete-journey `It()` using controlled broker responses through production ExtProc. This journey is Proposed and has no new approval recorded.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: The broker MUST provide AuthZEN 1.0 single Access Evaluation and its discovery metadata on the end-user server whenever token exchange is configured, without a new broker enable flag. It MUST not expose this capability as an admin-server API.
- **FR-002**: Discovery MUST publish `policy_decision_point` from the broker public base used for consent URLs and RFC 8414 issuer metadata, and the absolute `access_evaluation_endpoint` for this deployment. A consumer MUST verify the advertised identifier against the expected identifier before sending subject credentials.
- **FR-003**: Evaluation MUST accept exactly one subject token and one protected-resource URI per request. Subject type MUST be `urn:ietf:params:oauth:token-type:jwt`; asserted `identity` and all other subject types MUST return 400. The action name MUST be `urn:ietf:params:oauth:grant-type:token-exchange`; the resource type MUST be `protected_resource`. Missing required members and wrong JSON types MUST return 400. Following the AuthZEN 1.0 HTTPS binding, evaluation MUST ignore unknown request members, AuthZEN `properties`, and `context` members other than `agent`; an ignored member MUST NOT influence the decision. A present `context` MUST be an object whose `agent`, when present, is an Agent UUID string. The delegation MUST be resolved from the verified subject token alone.
- **FR-004**: Evaluation MUST resolve principal and agent using the same subject-token validation and `token_exchange.claim_extraction` CEL semantics as third-party token exchange. Evaluation MUST authenticate the PEP through a bearer client assertion and reuse the existing privileged-client CEL policy unchanged. For the supported `action: {"name": "urn:ietf:params:oauth:grant-type:token-exchange"}`, it MUST supply `request.grant_type = "urn:ietf:params:oauth:grant-type:token-exchange"`, with the same verified assertion/subject claims, extracted principal/agent, resource, and absent-scope semantics as the equivalent token-exchange request. It MUST NOT introduce a new grant type or require an evaluation-specific caller-policy discriminator. Unsupported actions MUST return 400, not select another grant type. An ordinary authenticated browser principal is not a substitute, and policy reuse does not authorize token issuance from evaluation.
- **FR-005**: `context.agent` MUST be optional. If it is absent, including when the request `context` object is omitted, evaluation MUST use the agent resolved from the verified subject token. The broker MUST compare any supplied anchor against that resolved agent; mismatch returns 400. An anchor MUST NOT replace or override verified token-derived agent resolution.
- **FR-006**: Evaluation and exchange MUST share one authoritative delegation resolution, preserving subject/assertion validation, claim extraction, caller authorization, resource normalization/resolution, registered-agent resolution, active-grant verification, declared permission-set checks, service coverage, scope union, and agent scope ceilings. The issue calls this `ResolveDelegation` (exchange steps 2–9a); it MUST not duplicate or weaken those invariants. Existing token-exchange session retrieval, refresh, decryption, session-scope checks, diagnostics, and response behavior remain downstream of this shared resolution and unchanged except for the explicitly staged deprecation.
- **FR-007**: A permit MUST return `decision: true` and a credential-free Delegation Context: the verified principal in `subject`; `agent {id, canonical_id}`; resolved service in `resource {service_id, canonical_id, resource_uri, scopes}` with effective scopes; `grant {id, updated_at, valid_until}`; and granted `permission_sets[] {id, canonical_id, services[] {id, canonical_id, scopes}}`. The verified principal in `subject.id` MUST be non-empty, and `permission_sets` MUST contain at least one entry. Only grant-included service entries may appear; empty scope/service arrays remain valid. Permission-set scopes describe the included permission-set contributions; `resource.scopes` is the final union after agent ceilings. Permission-set entries MUST NOT be interpreted as an override of that effective resource scope ceiling.
- **FR-008**: Context identities MUST retain real UUIDs and include configured canonical IDs for stable readable policy keys. When an agent, service, or permission set has no configured canonical ID, its `canonical_id` field MUST be omitted wherever that entity appears, including `permission_sets[].services[]`. A present `canonical_id` MUST be a non-empty string at every nesting level. It MUST NOT be null or fabricated, and absence alone MUST NOT fail evaluation or change its delegation decision. Configuring canonical IDs is not an evaluation rollout prerequisite.
- **FR-009**: Permit freshness MUST be `Cache-Control: private, max-age=min(60, whole seconds remaining until grant.valid_until)`, bounded below by zero. With no grant expiry it MUST be at most 60 seconds. Expired grants MUST deny. The 60-second broker cap is fixed for this increment; no configurable broker cap is introduced without a later explicit decision.
- **FR-010**: Each grant-denial condition in US2.1–US2.10 MUST return HTTP 200 with `decision: false`, the matching existing `FailureDetail` in the administrative reason and the resolved agent's consent URL in the user reason, and `Cache-Control: no-store`. It MUST not include a permit's delegation data or any credential. Denials MUST not be cached.
- **FR-011**: Malformed/unsupported requests, invalid/expired subjects, and agent-anchor mismatches MUST return 400. Missing/invalid gateway credentials MUST return 401; authenticated but unauthorized gateways MUST return 403; internal dependency/configuration failures MUST return 500. These request failures MUST not be represented as successful decisions. Unregistered resources are 400; resource ambiguity is a configuration failure. Request errors MUST not be cached as permits.
- **FR-012**: Every response MUST echo a supplied `X-Request-ID`, including permit, denial, and request-error responses, while preserving existing trace correlation behavior. Transport MUST use the AuthZEN 1.0 JSON contract and HTTP status semantics.
- **FR-013**: Evaluation MUST be read-only policy information: it MUST not retrieve/decrypt/refresh provider tokens, mint tokens, create grants, create tool approvals, or prove that a provider session is usable. A delegation permit MUST not bypass exchange or policy authorization.
- **FR-014**: ExtProc MUST offer a `PermitResolver` boundary independent of token exchange, discover its endpoint from the metadata document, authenticate through its existing assertion provider, and fail startup on discovery failure or policy-decision-point identifier mismatch. It MUST preserve the accepted metadata-only gateway input boundary for subject token and resource URI.
- **FR-015**: ExtProc MUST cache only permit contexts per exact `{subjectToken, resourceURI}` pair for no longer than the shorter of the advertised freshness and `max_staleness`. It MUST not cache denies or serve stale delegation on errors, including when exchanged-token fallback succeeds. Missing usable freshness MUST not imply an unbounded lifetime.
- **FR-016**: With delegation context enabled, body-bearing requests MUST start exchange and resolution concurrently during request headers and require both successes before body policy processing; delegation denial MUST return 403 `access_denied` with the consent URL before policy. Header-only requests MUST resolve, evaluate policy, and only then exchange on policy allow. Every error/deny MUST prevent forwarding, irrespective of concurrent completion order.
- **FR-017**: With delegation context enabled, ExtProc MUST decode the permit into its own typed struct and emit only these documented members directly in the existing `input.context`, beside `granted_permission_sets_available`, `granted_permission_sets`, and `agent_session_id`, before both body and header-only policy evaluation:
  - `principal`: string mapped from the permit's `subject.id`, using the broker term rather than exposing an AuthZEN `subject` object.
  - `agent`: `{id, canonical_id}`.
  - `resource`: `{service_id, canonical_id, resource_uri, scopes}`.
  - `grant`: `{id, updated_at, valid_until}`.
  - `permission_sets[]`: `{id, canonical_id, services[] {id, canonical_id, scopes}}`.
  Unset optional `canonical_id` (including nested service entries) and `valid_until` fields MUST be omitted, never null. ExtProc MUST NOT copy the permit verbatim, expose unknown permit fields to policies, or add a `delegation` wrapper or `delegation_available` flag. `granted_permission_sets` and `granted_permission_sets_available` MUST remain in OPA input in this increment, independently of FR-019's exchange-response deprecation. On both enabled paths, `granted_permission_sets` MUST be derived from permit permission-set IDs and included service IDs, not token-exchange extensions, and `granted_permission_sets_available: true` MUST guarantee that all five new members are present, subject to nested optional-field omissions. A resolution failure MUST return 503 before policy, never fall through with availability `false`. Existing `agent_session_id` behavior MUST remain unchanged.
- **FR-018**: ExtProc MUST default `delegation_context.enabled` to false, require `authorization.enabled` when true, and preserve existing disabled-mode behavior byte-for-byte without discovery or evaluation calls, omitting the new `principal`, `agent`, `resource`, `grant`, and `permission_sets` members. Policy fixtures affected by enabled-mode acceptance MUST use `input.context.agent.canonical_id`, `input.context.resource.canonical_id`, and `input.context.permission_sets[].canonical_id` rather than fixed deployment UUIDs.
- **FR-019**: The broker-only increment MUST be additive. `granted_permission_sets` on exchange responses MUST be deprecated when the resolver ships and removed one release later under a confirmed contract change; `principal` and `agent_id` MUST stay. The new endpoint MUST not cause immediate removal or an unrelated change to impersonation or other token grants.
- **FR-020**: With delegation context enabled, the permit MUST govern token-result reuse: forward only a result handle whose exchange principal and agent UUID match the permit and whose binding matches both identities, the permit resource scope set and grant-update instant. An identity mismatch or absent exchange identity MUST return terminal API-7 503 without approval lookup/creation, forwarding or an identity-repair retry; approval identity remains exchange-derived under ADR 036. Binding records first permit use, NOT the grant read by broker exchange. Each new result MUST have a distinct entry identity shared by all followers; first binding MUST be atomic and irreversible. On mismatch preserve concurrent replacements (even identical credentials with new identities), use a compatible replacement or exchange at most once more, and revalidate the delivered handle. A remaining mismatch MUST return API-7 503 `service_unavailable` / `delegation token binding unavailable`, with no rebind, loop, forwarding or removed/mismatched stale fallback. Existing exchange errors retain their handling. Permit expiry before final forwarding MUST return existing delegation-unavailable 503. Unchanged broader provider tokens and unbound/concurrent first use remain possible: this guarantees compatible reuse, NOT provider downscoping or exchange provenance (Proposed D-8). Stronger guarantees need separately approved scope; [Follow-ups for #292](https://github.com/zalando-incubator/agentic-identity-broker/issues?q=is%3Aissue%20in%3Atitle%20%22Follow-ups%20for%20%23292%22) tracks the selected exchange grant-version/effective-scope redesign and consistency contract (approved F3/P3 on the 2026-11-11 proposal). Disabled mode and D-5 cooldown are unchanged.

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
- **Delegation Context**: Credential-free snapshot accompanying a permit, relating principal, agent, service/resource, grant identity/version/expiry, and granted permission-set/service scope data. Entity UUIDs remain present; configured canonical IDs are included and unset ones omitted, including in nested service entries. Its freshness is independent of a provider token's lifetime.
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
- **API-002**: These contracts MUST be reviewed and explicitly confirmed before implementation. This draft is not authorization to change `api/enduser/openapi.yaml`. The answered Q1/Q2 decisions (optional request agent anchor and omission of unset canonical IDs) MUST be reflected in the reviewed schema. Journey 1's anchor representation, subject projection, nullable grant expiry, and denial reason examples must also be reconciled with that schema. Existing exchange/consent APIs remain binding.
- **API-003**: Verify the exact AuthZEN 1.0 metadata, request, response, `reason_admin`/`reason_user` envelope, and request-ID rules against the final standard before freezing OpenAPI. The issue's required administrative diagnostic and user consent URL must be expressible without private RFC 8693 response extensions. The initial specification session could not download the reference because TLS verification failed; that failure is historical. Completed final-standard verification is recorded in `quickstart.md` §7, including AuthZEN 1.0's requirement to ignore unknown fields, reflected in FR-003 and revised API-2. No unverified standard text is treated as authoritative.
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
- **SC-005**: Equivalent named agents, permission sets, and services with different deployment UUIDs produce the same policy outcome on both request paths in acceptance fixtures using `input.context.agent.canonical_id`, `input.context.resource.canonical_id`, and `input.context.permission_sets[].canonical_id`, without hardcoded UUID policy dependencies. Both enabled paths expose the documented direct context members and currently retained permit-derived compatibility inputs; unknown permit fields never appear in policy input.
- **SC-006**: Existing disabled-mode deployments require zero new settings and make zero delegation discovery/resolution requests; the additive broker release preserves existing exchange fields and session behavior.
- **SC-007**: No enabled request forwards a result handle bound to a different permit scope set/grant instant. Journey 5 demonstrates first-request upstream narrowing only after provider re-authorization; Journey 6 demonstrates unchanged broader capability without it and deterministic terminal 503 on a repeated shared-result conflict. No exchange grant-version provenance is claimed.

## Assumptions

- Scope is one feature with three delivery increments: additive broker evaluation/discovery and shared delegation semantics; opt-in ExtProc consumer and canonical-ID policy fixtures; documented exchange-field deprecation with removal one release after the resolver ships. These are product increments, not permission to run planning or implementation in this session.
- The four issue-recorded stakeholder decisions dated 2026-10-24 are preserved without re-asking: Access Evaluation (not Resource Search), JWT subject in the AuthZEN body with assertion in Authorization, asserted `identity` returns 400, and only exchange `granted_permission_sets` is deprecated/removed while `principal` and `agent_id` stay. Other proposed details remain subject to normal contract review; this draft invents no acceptance of an ADR.
- The maintainer confirmed the optional agent consistency anchor and omission of unset canonical IDs on 2026-10-26. These decisions preserve token-derived identity and UUID-based compatibility; policies that rely on canonical IDs still need those IDs configured for their own matching rules.
- Immediate exchange-response field removal is excluded (FR-019); current OPA compatibility removal/deprecation is also excluded (Proposed ADR-3). The fixed broker freshness cap is 60 seconds for this increment, consistent with “no new configuration”. A configurable broker cap is follow-up scope, not a blocker to this draft. Batch `/access/v1/evaluations`, Resource Search, actor-token support, `context.actor`, new `act`-chain response data, new UI, and new persistence/schema migrations are excluded.
- Permission-set service scopes describe included permission-set contributions; the requested resource's effective scopes remain authoritative after agent ceilings. The final API review must keep that distinction visible to policy authors.
- No new storage schema is expected: decisions project existing registered entities and grants. Evaluation intentionally does not guarantee provider-session health or credential availability.
- The issue references `specs/design-authzen-delegation-pip.md` on another branch. That file is absent from this checkout and was not used to import additional requirements; neither the other branch nor the external artifact was fetched. The supplied issue title/body are the feature input.
- Relevant retrieved baselines: `ARCHITECTURE.md` policy-gates table and glossary; ADRs 011, 012, 018, 028, 029, 033, 036 (ExtProc metadata input), and existing 043; current token-exchange resolution/scoping and `api/enduser/openapi.yaml`. Proposed ADRs are background, not accepted authority. Existing source behavior is used to expose compatibility constraints, not to add unrelated requirements.

### Clarification handoff

The original clarified contract and D-6 exception disposition have supplied approval and historical evidence in quickstart §§11–12. The missing original sequence is not retroactively performed. Current remediation resolves the document findings with Proposed ADR-2/ADR-3/API-7/D-7/D-8, 47 acceptance scenarios and consistent supporting artifacts. New token binding guarantees compatible handle reuse only; provider downscoping and exchange provenance remain explicit limitations. T103/T104 retain their publication-permission and follow-up-issue prerequisites, and new tests/implementation/validation remain unchecked. No further clarification questions remain. Document readiness does not grant approval or certify runtime compliance.
