---
description: "Task list for OAuth2 User Impersonation implementation"
---

# Tasks: OAuth2 User Impersonation

**Input**: Design documents from `/specs/20260825-120000-oauth2-user-impersonation/`

**Prerequisites**: plan.md (required), spec.md (required), research.md, data-model.md, contracts/

**Tests**: Per Constitution Principle VIII (TDD) and Principle XIII (E2E acceptance). E2E acceptance
tests are written first (red) in Phase 2f; unit/config tests accompany each implementation task.

**Organization**: Tasks are grouped by user story. US1/US2/US3 are all **P1**; US4 is **P2**.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Can run in parallel (different files, no dependencies)
- **[Story]**: Which user story this task belongs to (US1, US2, US3, US4)
- Every task includes exact file paths.


## Feature Scope Notes (from plan.md)

- **Phase 0 (refactoring)**: SKIPPED — feature is additive.
- **Phase 1 (deps)**: MINIMAL — all dependencies (jwx v3, cel-go, chi, fosite, slog) already present.
- **Phase 2.7 (entity boilerplate)**: SKIPPED — impersonation adds no persisted entity or migration;
  each request resolves an existing typed `AgentID` through the existing repository.
- **Frontend / Design System**: N/A — no React/UI change.
- **Database**: N/A — no schema changes or migrations.

---


---

## Phase 1: Setup (Shared Infrastructure)

- [X] T002 Establish baseline: run `just build` and `just test` green and confirm no new Go
      dependencies are required (jwx v3, cel-go v0.28.1, chi v5, fosite, slog already in `go.mod`)

---

## 🔒 Phase 2: Design Preconditions (Blocking Prerequisites) [MANDATORY]

**⚠️ CRITICAL**: No code implementation begins until this entire phase is complete.

### Phase 2a: Domain Model & Glossary [MANDATORY]

- [X] T003 Confirm configuration and in-flight value objects map to `internal/domain/impersonation/`
      and `internal/ports/config.go`; audience routing resolves an existing target `AgentID`.
- [X] T004 [P] Add impersonation domain terms to the Glossary in `ARCHITECTURE.md`
      (Impersonation Rule, Trusted Token Issuer, Unverified Subject, Impersonated Broker Token,
      Privileged Client Identity, Actor Identity, Subject Identity)
- [X] T005 [P] Document the `internal/domain/impersonation` bounded context, the activation/first-match/
      mint flow, and invariants (actor==subject permitted → `act.sub == sub`; `act.iss` from the validated actor token;
      target-owned granted scope; fail-closed) in `ARCHITECTURE.md`

**Checkpoint**: Domain model documented in ARCHITECTURE.md.

### Phase 2b: Configuration Design [MANDATORY]

- [X] T006 Confirm the config contract matches `internal/ports/config.go`: `audience_prefix` plus
      ordered rules, target-agent resolution, per-role credential semantics, trusted issuers, and CEL authorization.
- [X] T007 [P] Create `examples/config/impersonation.yaml` — a complete, bootable local-mode config whose
      `impersonation` block contains a **signed-subject** rule with trusted issuers, per-role expected
      audience + extraction, and an authorization predicate (FR-014)
- [X] T008 [P] Extend `examples/config/impersonation.yaml` with a second, **unverified-subject**
      rule (`verification: none`, subject-binding predicate) and demonstrate the same `issuer_uri` reused
      across rules (FR-014).
- [X] T009 Update `examples/config/README.md` to reference the new `impersonation.yaml` example
- [X] T010 [P] Update `charts/agentic-identity-broker/` (`values.yaml`, `values.schema.json`,
      `templates/configmap.yaml`, `README.md`) for the `oauth2_authorization_server.impersonation`
      subtree (FR-015)

**Checkpoint**: Configuration designed with a working example and Helm chart updated.

### Phase 2c: API Design [MANDATORY]

- [X] T011 Merge the signed-path portions of
      `specs/20260825-120000-oauth2-user-impersonation/contracts/openapi-token-impersonation.yaml` into
      `api/enduser/openapi.yaml`: impersonation request params, RFC 8693 success shape
      (`issued_token_type`, `token_type`, target-allowed scope), `act` claim, local-mode-only + audience
      activation, error responses (FR-013)
- [X] T012 Document the unverified-subject profile extension in `api/enduser/openapi.yaml`
      (`subject_token` as an unsigned `alg:none` JWT under the RFC 8693 JWT type, selected by a
      rule declaring `verification: none`; explicit broker-specific extension to RFC 8693 note) (FR-013).
- [X] T013 [P] Update `docs/api/` (rendered) and `docs/configuration.md` with the impersonation
      request/response shape and the `impersonation` configuration reference (FR-013/FR-014); unverified
      extension docs land with T012
- [X] T014 Confirm stakeholder sign-off is satisfied by the user's directive to extend the existing
      `/oauth2/token` endpoint (record reference in PR description) (spec Assumptions)

**Checkpoint**: API contract merged and confirmed.

### Phase 2d: Database Design [MANDATORY]

- [X] T015 Confirm no database schema changes or migrations are required; request-time target lookup
      reuses the existing Agent repository and persisted agents.

**Checkpoint**: Confirmed no DB changes.

### Phase 2e: Frontend/Design System Review

- [X] T016 N/A — no frontend or design-system change; record confirmation in the PR

### Phase 2f: E2E Acceptance Test Design [MANDATORY]

**Constitution Reference**: Principle XIII. All tests written first and verified to FAIL semantically.

- [X] T017 Add impersonation E2E fixtures: httptest JWKS servers signing ES256/RS256 client-assertion/
      actor/subject JWTs (pattern from `internal/adapters/jwks/adapter_test.go`), plus a signed-rule
      config fixture, under `tests/e2e/fixtures/`
- [X] T018 [P] Add impersonation success matchers asserting decoded `sub`/`aud`/`act.iss`/`act.sub`, granted JWT
      `scope`, and response scope omission only when no scope is granted in `tests/e2e/matchers/oauth2_matchers.go`
- [X] T019 Write US1 signed-path scenarios (S1–S5) as one `It()` each in
      `tests/e2e/impersonation_test.go` (signed mint, type rejection, base-claim inspection,
      non-activation, proxy/hybrid rejection), with spec references
- [X] T020 Write US1 unverified-subject scenario (S6) in `tests/e2e/impersonation_test.go`
      (unsigned `alg:none` subject under broker type; predicate binds+permits; `sub`=principal id,
      `email`=supplied email, `act.iss`=validated actor-token issuer, `act.sub`=actor) plus its unverified-rule fixture.
- [X] T021 Write US2 scenarios (S1–S5) in `tests/e2e/impersonation_test.go`
      (client-assertion validation, actor+signed-subject extraction, role/issuer mismatch rejection,
      startup config-error failure, first-match fall-through with shared issuer)
- [X] T022 Write US3 signed-path scenarios (S1–S4) in `tests/e2e/impersonation_test.go`
      (unsigned signed-role rejection, credential-failure rejection, `access_denied`, extraction failure)
- [X] T023 Write US3 unverified-subject guard scenario (S5) in
      `tests/e2e/impersonation_test.go` (no rule accepts unverified mode, OR predicate does not permit
      subject, OR predicate ignores `subject_token` → rejected).
- [X] T024 Write US4 scenarios (S1–S3) in `tests/e2e/impersonation_test.go`
      (success audit, failure audit, no-credential audit)
- [X] T025 Write edge-case `It()` blocks in `tests/e2e/impersonation_test.go` (`scope` present,
      `resource` present, `requested_token_type` mismatch, actor==subject permitted, JWKS unavailable
      fall-through)
- [X] T026 Verify the E2E suite FAILS semantically (red): realistic status/JWT/audit assertions, no
      `XIt`/`PIt`/`Skip()`, no placeholder always-fail assertions, no "red phase" comments
      (`ginkgo -v ./tests/e2e/ --focus "OAuth2 User Impersonation"`)

**Checkpoint**: E2E acceptance tests written and verified failing before implementation.

---

## Phase 2.5: Foundational Infrastructure [BLOCKING]

**⚠️ CRITICAL**: No user story work can begin until this phase is complete. Provides shared config
types, the issuer port, CEL machinery, the algorithm allow-list, and the local-issuer mint method that
all stories depend on.

- [X] T027 Add impersonation constants in `internal/domain/impersonation/constants.go`: the approved
      asymmetric algorithm set (`RS256/384/512, PS256/384/512, ES256/384/512, EdDSA`), reusing RFC 8693
      parameter constants from `internal/domain/tokenexchange/constants.go`
- [X] T028 Reuse the RFC 8693 JWT token type identifier from
      `internal/domain/tokenexchange/constants.go` for signed and unverified subjects
      in `internal/domain/impersonation/constants.go` (FR-003d).
- [X] T029 Add config types to `internal/ports/config.go`: optional `Impersonation *ImpersonationConfig`
      on `OAuth2AuthServerConfig`, plus `ImpersonationRuleConfig`, `ImpersonationRoleConfig`,
      `TrustedTokenIssuerConfig`, and the `CredentialRole` enum (data-model §1). Reuse the existing
      token-exchange `AuthorizationConfig` (`type: cel` + `cel.expression` + `cel.evaluation_timeout`)
      verbatim for the rule's `authorization`. Do NOT reuse `ClaimExtractionConfig` for per-role
      extraction — it mandates `agent_id_expression` and lacks an email field; instead define
      `principal_expression` (required) and optional `email_expression` on `ImpersonationRoleConfig`
- [X] T030 [P] Add the `ImpersonationTokenIssuer` port and `ImpersonationMintInput` type to
      `internal/ports/oauth2.go` (data-model §2 ImpersonationMintInput)
- [X] T031 [P] Unit tests for signed-path static config validation (table-driven CR-001..CR-008 with
      indexed field paths; proxy/hybrid rejection) in `internal/config/validator_test.go` and
      `internal/ports/config_test.go` — write FIRST; must fail before T032
- [X] T032 Implement static validation in `internal/config/validator.go`: required valid routing
      audience_prefix/rules, unique rule names, role shape, local-issuer exclusion, authorization,
      per-issuer allow-list, role coverage, and local-mode-only enforcement.
- [X] T033 [P] Unit tests for unverified-mode config validation (CR-005a/CR-008 unverified
      cases) in `internal/config/validator_test.go` — write FIRST; must fail before T034.
- [X] T034 Add unverified-mode config validation to `internal/config/validator.go`:
      accept `verification: none` on the subject role, forbid `expected_audience` for it, enforce
      subject-none never appears in any `signs_roles` (CR-008), and require the authorization predicate to
      reference `subject_token` when the rule accepts the unverified mode (CR-005a) — the compiled-expression
      check is finalized in T055 (makes T033 green).
- [X] T035 [P] Unit tests for the 5-variable environment and reference enumeration in
      `internal/domain/impersonation/cel_test.go` — write FIRST; must fail before T036
- [X] T036 Extend the CEL machinery (impersonation-scoped evaluator built from the ADR-20260116-120100 pattern in
      `internal/domain/tokenexchange/cel_evaluator.go`) to declare the five impersonation variables
      (`client_assertion`, `actor_token`, `subject_token`, `subject_unverified`, `request`), retain the
      checked AST, and expose reference enumeration (`ReferenceMap`) for the startup subject-binding check
      (makes T035 green)
- [ ] T037 [P] Update focused parity tests in `internal/domain/oauth2server/impersonation_mint_test.go`
      asserting normal local minting preserves target agent/policy claims and granted scope while only
      impersonation supplies protected `act.iss`/`act.sub` — write FIRST; must fail before T038
- [ ] T038 Refactor local-issuer minting via `strategies.go` through the normal access-token mint path:
      target-derived `agent_id`, target local-token CEL context, protected standard `act`, granted scope,
      and local-policy-owned `aud`.

**Checkpoint**: Foundation ready — user story implementation can begin.

---

## Phase 3: User Story 1 - Privileged Client Mints an Impersonated Broker Token (Priority: P1) 🎯 MVP

**Goal**: A privileged client submits a request to `<audience_prefix>/<canonical AgentID>` and
receives a locally issued token with target-derived `agent_id`, `sub`, and `act.sub`.

**Independent Test**: In local mode with a registered target and matching rule, verify target
`agent_id`/CEL context, policy-owned `aud`, `sub`, and `act.sub`.

### Tests for User Story 1 [Principle VIII]

- [X] T039 [P] [US1] Unit tests for request parsing/type validation (singleton/non-empty; scope,
      resource, `requested_token_type` rules; type identifiers) in
      `internal/domain/impersonation/request_test.go`
- [X] T040 [P] [US1] Unit tests for the signed-credential validator + per-issuer asymmetric algorithm
      allow-list enforcement (approved accept, `none`/`HS*` reject) in
      `internal/domain/impersonation/validator_test.go`
- [X] T041 [P] [US1] Unit tests for signed-subject identity + optional email extraction in
      `internal/domain/impersonation/extract_test.go`
- [X] T042 [P] [US1] Unit tests for the unverified subject parser (require `alg == none` +
      empty signature; reject any signed JWS) in `internal/domain/impersonation/unverified_test.go`.

### Implementation for User Story 1

- [X] T043 [US1] Implement `ImpersonationRequest` parse/validate in
      `internal/domain/impersonation/request.go` (FR-002a/003/003a/003b/003c): type identifiers,
      singleton/non-empty credentials, optional literal-space scope parsing, reject `resource`, optional
      `requested_token_type`
- [X] T044 [US1] Implement the compiled rule model (roles, trusted issuers, subject verification mode,
      authorization predicate) in `internal/domain/impersonation/rule.go`
- [X] T045 [US1] Implement the signed-credential validator + per-issuer algorithm allow-list in
      `internal/domain/impersonation/validator.go`, reusing the JWKS adapter
      (`internal/adapters/jwks`) and the `parseWithPolicy` verify pattern (FR-004/005, CR-007)
- [X] T046 [US1] Implement CEL authorization evaluation per rule and per-role signed-subject
      identity/email extraction in `internal/domain/impersonation/cel.go` and
      `internal/domain/impersonation/extract.go` (FR-006/006a/007), populating the local-token
      `principal` context
- [X] T047 [US1] Implement service orchestration (activate → validate target scopes → select rule → mint →
      assemble the RFC 8693 success body) for the signed path in `internal/domain/impersonation/service.go`:
      on match, return `access_token`, `issued_token_type` = access-token type identifier, `token_type` =
      `Bearer`, and non-empty granted `scope` (FR-008/008a)
- [X] T048 Implement handler target resolution before the generic resource guard: exactly one canonical
      suffixed audience selects a registered target; malformed/missing targets fail closed; resource is
      rejected after activation and scope is validated against the target allow-list.
- [X] T049 [US1] Wire the impersonation service in `internal/app/builder.go` for the mint path (compile
      rules → JWKS validators + CEL predicates + extraction; inject the service and
      `ImpersonationTokenIssuer` into the enduser token handler)
- [X] T050 [US1] Implement the unsigned unverified subject parser in
      `internal/domain/impersonation/unverified.go` (`jwt.ParseInsecure` + `alg==none` + empty-signature
      assertion; no issuer resolution) and route unverified subjects through the service's subject
      extraction (FR-003d, ADR 20260825-120000, research R3).
- [X] T051 [US1] Run US1 signed-path E2E scenarios (S1–S5) in `tests/e2e/impersonation_test.go` green
      (`ginkgo -v ./tests/e2e/ --focus "OAuth2 User Impersonation"`)
- [X] T052 Run US1 unverified-subject E2E scenario (S6) in `tests/e2e/impersonation_test.go`
      green (`ginkgo -v ./tests/e2e/ --focus "OAuth2 User Impersonation"`).

**Checkpoint**: US1 mints signed-subject and unverified-subject impersonated tokens end-to-end.

---

## Phase 4: User Story 2 - Operator Defines Trusted Credential Sources as Rules (Priority: P1)

**Goal**: Operators configure an ordered list of self-contained rules; valid tokens from configured
sources/roles are accepted, unconfigured ones rejected, and misconfiguration fails startup.

**Independent Test**: Configure rules with trusted issuers per role; verify accepted vs rejected
credentials and actionable startup failures (T021 covers S1–S5).

### Tests for User Story 2 [Principle VIII]

- [X] T053 [P] [US2] Unit tests for first-match selection, uniform fall-through, and
      shared-issuer-across-rules behavior in `internal/domain/impersonation/service_test.go` —
      write FIRST; must fail before T056

### Implementation for User Story 2

- [X] T054 [US2] Implement builder startup validation in `internal/app/builder.go`: construct JWKS
      validators per trusted issuer, compile each rule's CEL predicate, enforce local-issuer exclusion,
      and fail with actionable errors that name the offending rule
- [X] T055 Extend the builder startup validation to verify subject-binding for unverified
      rules from the compiled expression's references (`ReferenceMap` must include `subject_token`; FR-007a/CR-005a) and fail startup otherwise in `internal/app/builder.go`.
- [X] T056 [US2] Implement first-match rule evaluation with uniform fall-through in
      `internal/domain/impersonation/service.go` (a credential-validation failure OR a false predicate
      falls through to the next rule) (FR-004a) (makes T053 green)
- [X] T057 [US2] Run US2 E2E + startup-failure scenarios (US2 contexts) in
      `tests/e2e/impersonation_test.go` green (`ginkgo -v ./tests/e2e/ --focus "OAuth2 User Impersonation"`)

**Checkpoint**: Multi-rule configuration validated at startup and enforced at request time.

---

## Phase 5: User Story 3 - Broker Rejects Unsafe or Unauthorized Impersonation (Priority: P1)

**Goal**: The broker fails closed for malformed, untrusted, expired, unsigned-signed-role, or
unauthorized credentials, with the correct OAuth2 error and no token.

**Independent Test**: Submit each failing credential class and verify an OAuth2 error and no token
(T022 covers S1–S4; T023 covers S5).

### Tests for User Story 3 [Principle VIII]

- [X] T058 [P] [US3] Unit tests for error precedence and fail-closed taxonomy in
      `internal/domain/impersonation/errors_test.go` — write FIRST; must fail before T059/T060

### Implementation for User Story 3

- [X] T059 [US3] Implement the error taxonomy and order-independent no-match precedence
      (`access_denied` > `invalid_client` > `invalid_request`, `server_error` for fail-closed internal
      errors) in `internal/domain/impersonation/errors.go`, reusing
      `tokenexchange.TokenExchangeError` envelopes (FR-004a/011, research R11)
- [X] T060 [US3] Enforce fail-closed rejection paths in `internal/domain/impersonation/service.go` and
      `validator.go`: `alg:none`/`HS*` for signed roles, signature/issuer/audience/expiry/nbf failures,
      empty/conflicting identity, and JWKS-unavailable fall-through (FR-005/006) (makes T058 green)
- [X] T061 [US3] Run US3 signed-path E2E rejection scenarios (S1–S4, US3 contexts) in
      `tests/e2e/impersonation_test.go` green (`ginkgo -v ./tests/e2e/ --focus "OAuth2 User Impersonation"`)
- [X] T062 Enforce unverified-mode guards (reject when no rule accepts unverified mode, when
      the predicate does not permit the subject, or when the predicate ignores `subject_token`) in
      `internal/domain/impersonation/service.go`, and run US3 unverified scenario (S5) green.

**Checkpoint**: All unsafe/unauthorized requests fail closed with the correct error.

---

## Phase 6: User Story 4 - Operator Audits Impersonation Decisions (Priority: P2)

**Goal**: Every impersonation decision emits a credential-free structured audit event with outcome,
selected rule, issuer identifiers/roles, available identities, and correlation.

**Independent Test**: Process a success and each failure class; verify structured audit events with no
token values (T024 covers S1–S3).

### Tests for User Story 4 [Principle VIII]

- [X] T063 [P] [US4] Unit test the audit event field whitelist and the no-credential-value invariant in
      `internal/adapters/http/enduser/oauth2_token_audit_test.go` — write FIRST; must fail before T064

### Implementation for User Story 4

- [X] T064 [US4] Emit a credential-free `impersonation_decision` structured `slog` event at the token
      endpoint boundary in `internal/adapters/http/enduser/oauth2_token.go`, whitelisting fields per
      research R9 (outcome, failure category, `audience`, selected `rule`, issuer ids+roles, available
      identities, OAuth error code, `request_id`) (FR-012, SC-005) (makes T063 green)
- [X] T065 [US4] Wrap `POST /oauth2/token` with the existing `OAuth2AuditMiddleware` and export a safe
      request-ID accessor in `internal/adapters/http/middleware/oauth2_audit.go` for audit correlation
      (research R9)
- [X] T066 [US4] Run US4 E2E audit scenarios (US4 contexts) in `tests/e2e/impersonation_test.go` green
      (`ginkgo -v ./tests/e2e/ --focus "OAuth2 User Impersonation"`)

**Checkpoint**: All impersonation decisions are auditable without credential exposure.

---

## 🔒 Phase N: Constitution Compliance & Polish [MANDATORY]

### Design Phase Verification

- [X] T067 Verify impersonation domain terms and bounded context are documented in `ARCHITECTURE.md`
      Glossary (Principle V)
- [X] T068 Verify `examples/config/impersonation.yaml` exists, is referenced by
      `examples/config/README.md`, and boots (Principle VII)
- [X] T069 Verify the impersonation API is documented in `api/enduser/openapi.yaml` and rendered
      `docs/api/` (Principles IV, X)
- [X] T070 Verify no DB changes were introduced (stateless; Principle IX)
- [X] T071 Verify E2E acceptance tests in `tests/e2e/impersonation_test.go` map 1:1 to spec scenarios
      and were red before implementation (Principle XIII)

### Implementation Phase Verification

- [X] T072 [P] Verify the API implementation matches `api/enduser/openapi.yaml` exactly (Principles IV, X)
- [X] T073 [P] Verify configuration is loaded only via `internal/ports/config.go` with no ad-hoc loading,
      and the Helm chart reflects the `impersonation` subtree (Principle VII)
- [X] T074 [P] Verify no custom cryptography — jwx for verify/sign, cel-go for policy only
      (Principles I, III)
- [X] T075 Verify hexagonal boundaries: handler → domain service → `ImpersonationTokenIssuer` port
      (no port bypass); `internal/domain/impersonation` imports only ports/domain (Principle VI)
- [X] T076 Verify security fails closed by default with no bypass, and signed roles ALWAYS verify
      signatures (unsigned accepted ONLY on the bounded unverified-subject role) (Principle I)
- [X] T077 Run `just check` (fmt → vet → lint) and `just test` (unit + config) green
- [X] T078 Run the full E2E suite `ginkgo -v ./tests/e2e/ --focus "OAuth2 User Impersonation"` green
- [X] T079 Run `just verify` (full gate) green
- [X] T080 Run the `specs/20260825-120000-oauth2-user-impersonation/quickstart.md` manual smoke test (single
      signed-subject request) and confirm decoded `sub`/`aud`/`act.iss`/`act.sub` and granted `scope`
- [X] T081 Verify SC-001 performance: labelled functional exclusion is in place; run
      `just test-e2e-performance` against a warmed local-mode server, confirm ≥95% succeed and p95 is
      under 500ms, and record that command plus its SC-001 summary line in PR #464 (SC-001, plan.md
      Performance Goals). T078/T079 remain functional/full-gate evidence, not SC-001 evidence.

### Review remediation

- [X] T083 Update the audience-prefix target contract across configuration, OpenAPI, feature documents, Helm, examples, and architecture: one canonical registered AgentID suffix selects the target; target-derived `agent_id`/CEL context and local-policy `aud` are proven.
- [X] T084 Add and pass unit, HTTP, and E2E regression coverage for audience-target resolution, target policy context, cache headers, error precedence, reserved subject token types, and signed assertion validation.
- [ ] T085 Add and pass target-scope contract coverage: optional target-allowed scope reaches minting and
      JWT/RFC 8693 response; rejected scope returns credential-free `invalid_scope`; empty target
      allow-list is unrestricted; unit, HTTP, and E2E coverage prove the contract.

### User-delegation consent remediation

- [X] T086 Update the feature design artefacts (`plan.md`, `data-model.md`, `quickstart.md`) and
      architecture/governance records (`ARCHITECTURE.md`, ADR 20260825-120100, ADR index) for mandatory
      user-delegation enforcement and the token-error `error_uri` signal (FR-017–019, CR-010).
- [X] T087 [P] Update `api/enduser/openapi.yaml`, `docs/configuration.md`, and
      `examples/config/impersonation.yaml` plus its README reference with the mandatory
      delegation contract and consent `error_uri` (FR-013/014/018).
- [X] T088 [P] Write failing User Story 5 E2E scenarios in `tests/e2e/impersonation_test.go`:
      seed active delegation for existing mint scenarios; assert missing, expired, and unverified subjects
      without delegation return 403 `access_denied` plus the exact target-agent `error_uri` (US5.2–US5.4,
      Principles VIII, XIII).
- [X] T089 [P] Write failing `internal/domain/impersonation/service_test.go` cases for active,
      missing, expired, and unavailable user-delegation verification; prove predicate denial does
      not query delegation and delegation denial is terminal. Write app classifier tests first.
- [X] T090 Add `ports.UserDelegationVerifier` and `UserDelegationStatus` to
      `internal/ports/oauth2.go`; add the app-boundary consent-service classifier and its tests in
      `internal/app/impersonation_delegation.go` (FR-017, Principle VI).
- [X] T091 Implement `consentRequired` in `internal/domain/impersonation/errors.go` and enforce
      terminal user-delegation verification after rule authorization and before minting in
      `internal/domain/impersonation/service.go` (FR-017a/017b/018/019).
- [X] T092 Wire the verifier and end-user public URL in `internal/app/builder.go`, failing startup
      when mandatory dependencies are unavailable (CR-010).
- [X] T093 Run the focused US5 E2E suite, `just check`, `just test`, and `just verify`; confirm the
      consent URL is actionable for a first-time delegation and audits remain credential-free.
- [X] T094 [US5] Add a production-bootstrap E2E scenario for a delegation-storage lookup failure. Assert
      `500 server_error`, no token or `error_uri`, and a credential-free `user_grant_lookup_failed` audit event
      (US5.5, Principles VIII, XIII).


## Amendment 2026-09-04: canonical-ID audience targets

- [X] T086 Resolve an audience suffix that is either the target agent's canonical lower-case UUID or
      its `canonical_id` in `internal/domain/impersonation/audience.go`; require canonical-ID
      resolution capability at `NewService` startup.
- [X] T087 Cover both addressing forms and the grammar/not-found error split in
      `internal/domain/impersonation/audience_test.go` and `service_test.go`.
- [X] T088 Add the canonical-ID audience minting scenario and rejection cases to
      `tests/e2e/impersonation_test.go`.
- [X] T089 Update the two-form activation contract in spec 20260825-120000, `api/enduser/openapi.yaml`,
      `docs/`, `examples/config/`, `charts/`, and `ARCHITECTURE.md`.


---

## Dependencies & Execution Order

### Phase Dependencies

- **Setup (Phase 1)**: no dependencies.
- **Design Preconditions (Phase 2)**: depends on Setup; BLOCKS all implementation. Sub-phases 2a–2f may
  proceed in parallel; all must complete before Phase 2.5.
- **Foundational (Phase 2.5)**: depends on ALL of Phase 2; BLOCKS all user stories.
- **User Stories (Phase 3–6)**: depend on Phase 2.5.
  - US1 (Phase 3) is the MVP: signed-path engine + happy-path DI.
  - US2 (Phase 4) extends the builder with startup validation + first-match fall-through.
  - US3 (Phase 5) adds the rejection taxonomy.
  - US4 (Phase 6, P2) adds auditing at the handler boundary; independent of US2/US3 internals.
- **Constitution Compliance (Phase N)**: depends on all desired user stories.

### User Story Dependencies

- **US1 (P1)**: foundation only — no dependency on other stories.
- **US2 (P1)**: reuses US1's rule-compilation wiring (T049) for the builder; independently testable via
  startup + fall-through scenarios.
- **US3 (P1)**: reuses US1's validator/service; independently testable via rejection scenarios.
- **US4 (P2)**: independent — audit at the handler boundary.

### Within Each Story

- Tests written and failing before implementation.
- Request/validator/parser/extraction before service orchestration; service before handler wiring.

---

## Parallel Opportunities

- Phase 2 design tasks marked [P] (T004, T005, T007, T010, T013, T018) run in parallel.
- Foundational [P] tasks (T030 port definition, plus test tasks T031, T035, T037) run in parallel after
  config types (T029) land; their implementation counterparts (T032, T036, T038) follow their tests (TDD).
- US1 signed-path test tasks T039–T041 run in parallel (different files).
- Across stories, once Phase 2.5 completes: US1 signed engine first; US3 (rejection) and US4 (audit) can
  then proceed in parallel; US2 builder validation parallel to US3/US4.

## Parallel Example: User Story 1 Tests (signed path)

```bash
# Launch US1 signed-path unit tests together (different files, no dependencies):
Task: "Request parsing tests in internal/domain/impersonation/request_test.go"
Task: "Signed validator + algo allow-list tests in internal/domain/impersonation/validator_test.go"
Task: "Signed-subject extraction tests in internal/domain/impersonation/extract_test.go"
```

---

## Implementation Strategy

### MVP First (User Story 1, signed path)

1. Phase 1: Setup baseline.
2. Phase 2: Design Preconditions (domain doc, signed-path config example + Helm, OpenAPI merge, E2E red).
3. Phase 2.5: Foundational (config types, port, CEL machinery + algo allow-list, mint method).
4. Phase 3: US1 signed path — mint signed-subject impersonated tokens.
5. STOP and VALIDATE: US1 signed E2E green; demo the MVP.

### Incremental Delivery

- US1 signed (MVP) → US2 (rule config + startup validation) → US3 (fail-closed rejection) → US4 (audit).
- Each story adds value without breaking prior stories; validate independently at each checkpoint.

---

## Notes

- [P] = different files, no dependencies.
- [Story] label maps each task to its user story for traceability.
- Verify tests fail before implementing (Principles VIII, XIII).
- No DB migrations, no new ID types, and no frontend; requests resolve an existing target AgentID.
