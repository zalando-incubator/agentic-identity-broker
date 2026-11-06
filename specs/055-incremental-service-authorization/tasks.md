# Tasks: Incremental Service Authorization

**Input**: `specs/055-incremental-service-authorization/` — `spec.md`, `plan.md`, `research.md`, `data-model.md`, `contracts/`, and `quickstart.md`.
**Status**: Blocked after contract and acceptance-test progress: 11 of 101 tasks complete. The supplied 11-item approval is recorded in ADRs 045/046. Approved API deltas are in OpenAPI 1.2.0, contract tests pass, and the three US2 backend acceptance tests compile and fail semantically. No feature behavior or migration is implemented. See [the latest implementation checkpoint](./quickstart.md#implementation-checkpoint--2026-10-30) for actual checks, deferred tasks and required actions.
**Tests**: Mandatory red → green → refactor under Principles VIII and XIII. Write each test first, make it compile with only minimal structural scaffolding, and observe a semantic failure before implementing its behavior. Never use pending/skip markers, vacuous assertions, or red-phase annotations in test files.
**Organization**: Setup → mandatory design preconditions → shared foundations → US1 (P1) → US3 (P1) → US2 (P1) → US4 (P2) → mandatory compliance. The three P1 stories share priority; this order follows the plan's dependency order, not a change to their priorities.

## Format: `[ID] [P?] [Story] Description`

`[P]` identifies independent files within a dependency-ready batch, not permission to run a task before its prerequisites. Story labels retain the specification's numbering. Paths are relative to the repository root. Every task cites a requirement, scenario, approval item, or binding principle.

## 🔒 Feature Scope [MANDATORY]

Implement only the items covered by the maintainer's `context.approval`; the supplied approval by mjungsbluth on 2026-10-29 covers the 11 published items in `plan.md` ([approval comment](https://github.com/zalando-incubator/agentic-identity-broker/pull/323#issuecomment-6509178189)). The immutable plan still labels them Proposed; this record does not extend approval to the new contract revisions in T099/T100. If an item is not covered, leave its dependent tasks unchecked, explain why, and continue independent approved work. Do not infer approval from a draft, task, or existing implementation. Keep `spec.md` and `plan.md` unchanged during implementation.

No new dependencies, broker configuration keys, historical aliases, client retry implementation, gateway metadata hosting service, or unrelated refactoring are included. Preserve accepted ADRs; ADR 045 partially supersedes only the specified ADR 044 clauses. Do not edit the merged ADR 044. Phase 0 refactoring and Phase 2.7 entity boilerplate are inapplicable: the coverage extraction is small and there is no new entity, typed ID, or repository.

## Phase 1: Setup (Shared Infrastructure)

- [X] T001 Verify the supplied maintainer approval against `specs/055-incremental-service-authorization/plan.md` Approval Items; for each covered draft in `specs/055-incremental-service-authorization/adrs/045-service-session-recovery-return.md` and `specs/055-incremental-service-authorization/adrs/046-virtual-service-scopes.md`, record `Accepted (approved by <login> on <date>: <url>)` and move it to `adrs/045-service-session-recovery-return.md` or `adrs/046-virtual-service-scopes.md`; repair moved relative links and leave reference-only pointers at the feature-local paths so immutable spec/plan links still resolve. Record only supplied approvals; leave uncovered work unchecked (Principles II, IV, X; ADR-RECOVERY, ADR-SCOPES).
- [X] T002 Read the nearest area instructions for the paths in this backlog and confirm available `justfile` commands, installed tool versions, migration 046 availability, and test prerequisites; record genuine infrastructure blockers in `specs/055-incremental-service-authorization/quickstart.md` without weakening checks or changing workflows, skills, hooks, the constitution, or automation scripts (Principles VIII, IX, XIII).

## 🔒 Phase 2: Design Preconditions (Blocking Prerequisites) [MANDATORY]

**Purpose**: Complete domain, configuration, API, database, concrete spec-journey, and E2E design before implementation. All six sub-phases are mandatory. Only minimal compile-enabling scaffolding is permitted before the semantic-red checkpoint; no feature business logic is implemented here. Migration design is verified here; executable SQL is written only after its failing tests in Phase 2.5.

### Phase 2a: Domain Model & Glossary [MANDATORY]

- [ ] T003 Confirm all value objects, extended aggregates, invariants, and state transitions in `specs/055-incremental-service-authorization/data-model.md`, including immutable scope-to-UUID bindings, request-local requirements, four usability outcomes, and no new entity/repository; reconcile only editable supporting artifacts if needed (Principles V, VI; FR-003, FR-007, FR-019).
- [X] T004 Add Virtual Service Scope, Requested Service Set, and Service-Scope Binding to `ARCHITECTURE.md` and `AGENTS.md` glossaries; refine the existing Session Usability definition to distinguish stored refreshability from verified current usability rather than duplicating that term (Principles II, V; FR-008).

### Phase 2b: Configuration Design [MANDATORY]

- [X] T005 Create `examples/config/incremental-authorization.yaml` using existing `server.enduser.public_url` and `oauth2_authorization_server.mode`, plus `examples/config/incremental-authorization-agentgateway.yaml` showing strict JWT authentication, `preserveToken: false`, fail-closed ExtProc, and route metadata including optional `aib.tokenexchange.resource_metadata_url`; show the absent/default variant without inventing a global configuration key (FR-017; API-METADATA-CONFIG; ADR 036).
- [X] T006 Reference both examples from `examples/config/README.md` and document owning configuration, absence/default and validation semantics in `docs/configuration.md`; the metadata field has no environment/CLI equivalent or precedence chain because it is a route input, not a deployable-process setting (Principle VII; FR-017).
- [X] T007 Verify and document in `docs/guides/token-exchange-gateway.md` that Agentgateway owns the metadata producer under ADRs 011/036 and `charts/agentic-identity-broker/` does not deploy that setting; leave broker chart values, templates, and README unchanged (Principle VII; API-METADATA-CONFIG).

### Phase 2c: API Design [MANDATORY]

- [X] T008 After confirming the supplied API approvals, copy the approved deltas from `specs/055-incremental-service-authorization/contracts/enduser-api.md` into `api/enduser/openapi.yaml`: version 1.2.0, exchange error scope and recovery target, authorization outcomes, local/proxy token behavior, consent arrays and submission error, and additive authenticated session-detail route with complete schemas/examples/errors; keep `api/admin/openapi.yaml` and the existing UUID-only session read unchanged (Principles IV, X; API-001–API-004; all five API approval items).

### Phase 2d: Database Design [MANDATORY]

- [X] T009 Verify the approved paired SQL design in `specs/055-incremental-service-authorization/data-model.md` for `migrations/046_add_service_scope_bindings.up.sql` and `migrations/046_add_service_scope_bindings.down.sql`: nullable JSONB on both existing records, default transaction, no backfill, no user-session schema change, and coordinated code/schema rollback with binding loss explicitly documented (Principle IX; DEC-003, DEC-004; FR-019).

### Phase 2e: Frontend/Design System Review [MANDATORY IF FRONTEND]

- [X] T010 Review `web/src/design-system/docs/INDEX.md`, `COMMON_MISTAKES.md`, `DECISION_TREES.md`, and `COMPONENT_PAIRING_GUIDE.md`; document composition for the four Wireloom states in `specs/055-incremental-service-authorization/quickstart.md`, using existing Card/Badge/Alert/Button and existing toggle primitives, semantic tokens, keyboard focus, light/dark contrast, and accessible Save reasons; confirm the banner and detail page are application-specific, not new universal components (Principle XI; FR-009–FR-011, FR-015).

### Phase 2f: E2E Acceptance Test Design [MANDATORY]

Use production `bootstrap.ServerFactory` and fresh storage/server per `It`, fixture factories, `Describe → Context → It`, and a nearby exact spec scenario reference. In the checklist below, abbreviated sibling filenames share the most recent explicit directory in that task. Gateway tests use `bootstrap.ExtProcEnvironment` with a real broker and assert the ExtProc gRPC immediate response, not a fake broker-only header. Each of the 19 scenarios owns exactly one `It` as mapped below; variants stay within that scenario's test. Add page-object methods before browser tests, not raw selectors in the scenarios.

- [X] T011 Extend `tests/e2e/fixtures/services.go`, `agents.go`, `grants.go`, and `sessions.go` with isolated shared-cast factories, covering permission sets with complementary required scopes, and multiple services; extend existing helpers in `tests/e2e/helpers/` only where needed for deterministic provider refresh rejection/transience and upstream form recording (Principles VIII, XIII; US1–US4).
- [X] T012 Write the nine mapped backend `It` blocks in `tests/e2e/incremental_authorization_test.go` for US1 S2/S3/S5/S6/S7, US3 S1/S4, and US4 S2/S3, asserting real OAuth/consent/code/refresh/exchange journeys, exact errors, unchanged grants/tokens on failure, reassignment after Save before authorize re-entry (US4 S2), all-scopes/no-requirements missing-effective-scope variants (US1 S7), full → narrowed → scope-less refresh with all assurance bindings rechecked and no restored scopes (US3 S1), and no widening (FR-002–FR-003, FR-007–FR-012, FR-019–FR-020).
- [X] T013 [P] Write US1 S1 and US4 S1 in `tests/e2e/incremental_authorization_gateway_test.go`: fail a real service exchange after provider rejection, verify persisted unusability, scope/action propagation and unchanged URL elicitation, then repair and retry; compare metadata-present/absent variants through the broker-plus-ExtProc bootstrap (FR-004–FR-006, FR-008, FR-018; SC-001, SC-006).
- [X] T014 [P] Write US3 S2/S3 in `tests/e2e/incremental_authorization_proxy_test.go`: interactive checks/continuation followed by upstream authorization without virtual scopes, and upstream refresh rotation preserved despite an unusable service followed by exchange challenge and recovery; assert upstream signed claims are not modified (FR-013, FR-021; SC-005).
- [X] T015 [P] Write US2 S1/S3/S4 in `tests/e2e/service_session_page_test.go`: detail with no session for both identifiers, principal isolation and authenticated unknown-service errors, and recovery login/callback to the same service page followed by successful exchange without a grant change (FR-006, FR-015–FR-016; SR-005; SC-003).
- [X] T016 Add requested-service and detail/action locators to `tests/e2e/pages/consent_page.go` and `tests/e2e/pages/sessions_page.go`, then write US1 S4 and US2 S2 in `tests/e2e/frontend/incremental_authorization_test.go`; exercise unsaved toggles including all-scopes/no-requirements valid-token/missing-candidate-scope variants (selecting a failing candidate disables Save, deselecting it enables Save), provider login, restored end date/context, protected Save continuation and tool retry, plus detail login/refresh/termination states; configure the four screenshot names from the plan (FR-009–FR-011, FR-015–FR-016; SC-001–SC-003).
- [X] T017 [P] Write US4 S4 in `tests/e2e/incremental_authorization_docs_test.go`, combining live discovery assertions with parsing of the operator guide's route/config example and recovery flow checks; assert observable scope/metadata contract properties, not exact prose (FR-014, FR-017; SC-006).
- [ ] T018 Review Journeys 1–4, HTTP/config/JWT examples, Wireloom source and SVGs in `specs/055-incremental-service-authorization/spec.md` against the approved OpenAPI; run `just test-e2e-backend` and `just test-e2e-frontend` to prove all new mapped scenarios compile and fail semantically before feature implementation, preserving red evidence and configuring screenshots through the existing harness; do not skip tests or modify workflows to make the checkpoint pass (Principles VIII, XIII, XIV).

**Checkpoint**: Design prerequisites and all 19 semantic-red acceptance tests are complete before foundations. A test failure is evidence for red, not a passing validation result. Existing unrelated failures must be distinguished and reported.

## Phase 2.5: Foundational Infrastructure

**Purpose**: One shared scope/coverage invariant, verified usability, and identity-binding continuity. Tests precede each implementation pair. The approved shared issuance path is foundational because both focused consent and local-refresh stories consume it.

- [X] T019 Write table-driven grammar/emission tests in `internal/domain/servicescope/scope_test.go` against the complete contract, quoting the identifier constraint `^[A-Za-z0-9._-]{1,128}$` (never UUID-shaped), lowercase canonical UUIDs only, exact case-sensitive configured prefix, trailing-public-URL slash removal, malformed remainder rejection, external lookalikes ordinary, and no Host-derived origin (FR-001–FR-002; SR-004).
- [X] T020 Implement `internal/domain/servicescope/scope.go` and `errors.go` to make T019 green, retaining `Raw`: "The exact requested string. It is never rewritten when granted"; distinguish ordinary/virtual/malformed and emit current canonical ID or UUID fallback (FR-001–FR-002, FR-020; data-model VirtualServiceScope).
- [X] T021 Write resolution/coverage/evaluation tests in `internal/domain/servicescope/requested_set_test.go`, `coverage_test.go`, and `evaluate_test.go`: eligibility with/without explicit requirements, duplicate exact tokens and canonical/UUID deduplication to one service while retaining both raw forms, unknown versus repository failure, binding mismatch, selected union intersected with ceiling, complementary sets, `require_all_scopes` and no-explicit-requirements effective unions despite empty configured required scopes, valid tokens missing a selected contribution, per-candidate snapshot usability and live-selection composition, preserved structural/empty-intersection denials, multiple services and every usability outcome (FR-003, FR-007–FR-010, FR-019; SR-001, SR-003).
- [X] T022 Implement `internal/domain/servicescope/requested_set.go`, `coverage.go`, and `evaluate.go` with narrow consumer-declared interfaces; preserve immutable `map[string]id.ServiceID` bindings and an ordered, duplicate-free service set; compute distinct `EffectiveScopes` with the exchange union/ceiling rule and pass them to usability; enforce `Satisfied = Selected && RequiredScopesCovered && Usability == usable`, with baseline plus selected-candidate composition for live consent; add no infrastructure imports or competing coverage mechanism (Principles V, VI; FR-003, FR-007–FR-010, FR-019).
- [X] T023 Extract the existing coverage computation from `internal/domain/tokenexchange/service.go` into the shared rule, preserving existing outcomes and diagnostics; run the existing `internal/domain/tokenexchange/service_test.go` and `service_security_test.go` regressions before/after extraction (Principle VI; FR-003; SR-001).
- [X] T024 Write usability/classification and lifecycle race regressions in `internal/domain/oauth2session/service_usability_test.go`, `service_refresh_classification_test.go`, `service_refresh_concurrency_test.go`, and `service_reconnect_regression_test.go`: valid access with sufficient effective scopes and matching identity plus absent refresh or expired refresh (both usable with zero refresh calls), successful expired-token refresh, expired access with absent/expired refresh (unusable), valid/refreshed access missing an effective permission-set scope under all-scopes/no-requirements despite empty configured required scopes (unusable), established-client mismatch, definitive `invalid_grant`, transient 5xx/429/network/storage/cancellation, reconnect versus stale rejection, termination versus refresh/callback, and background/sweep visibility (FR-008, FR-018; DEC-003; SR-003).
- [X] T025 Add `CheckServiceUsability` in `internal/domain/oauth2session/service.go` through `GetValidAccessToken`, existing singleflight/row locking and explicit effective-scope/client-identity checks, keeping valid access usable with absent/expired refresh; return exactly `usable`, `missing`, `unusable_definitive`, or `unverifiable_transient`; reuse only the existing refresh-expiry rejection marker for provider `invalid_grant`, never persist transient invalidation or overwrite a reconnect (FR-008, FR-018; DEC-003; ADR 038).
- [X] T026 Run the T024 regressions and existing background/sweep/session tests in `internal/domain/oauth2session/` via `just test`, retaining existing refresh rotation, reconnect precedence, and update-only deletion behavior; verify no extra provider calls for valid tokens (FR-008, FR-018; Principles I, VIII).
- [X] T027 Write validation tests in `internal/domain/storage/authorization_code_test.go` and `refresh_token_session_test.go` for nullable bindings: code binding keys must be in code `Scope`; refresh assurance bindings may outlive scope narrowing; every current virtual scope requires a binding, every binding key has valid virtual grammar and every value is a non-zero UUID; assert legacy nil only without virtual scopes, no binding mutation/addition at succession, and no successor scope widening (FR-019; data-model extended records).
- [X] T028 Extend `internal/domain/storage/authorization_code.go` and `refresh_token_session.go` with `ServiceScopeBindings` and the tested constraints without a new entity/typed ID/repository (Principles V, IX; FR-019; DEC-004).
- [X] T029 Write binding round-trip/deep-copy/legacy-nil/successor-inheritance tests in `internal/adapters/storage/memory/authorization_code_store_test.go` and `refresh_token_session_store_test.go`, including the three-step code/full-scope → narrowed refresh → scope-less refresh sequence, narrowed successor `Scope` plus all inherited assurance bindings, rejection of widened scope or changed/added bindings, caller mutation isolation and existing rotation/revocation races (Principle IX; FR-012, FR-019).
- [X] T030 Add binding persistence and deep map copies in `internal/adapters/storage/memory/authorization_code_store.go` and `refresh_token_session_store.go`, enforcing exact predecessor-to-successor binding equality and scope subset under the existing lock, preserving successor atomicity, revocation and cleanup (Principle IX; FR-019; DEC-004).
- [ ] T031 Write migration apply/down/reapply tests with representative pre-feature rows in `tests/integration/migrations/service_scope_bindings_test.go` using `framework.go` and shared PostgreSQL helpers; verify both nullable JSONB columns, no backfill, retained unrelated row data, and documented binding loss on down (Principle IX; DEC-004).
- [ ] T032 Create `migrations/046_add_service_scope_bindings.up.sql` and `migrations/046_add_service_scope_bindings.down.sql` exactly as approved, under the default transaction, then make T031 green on real PostgreSQL (Principle IX; DEC-004).
- [ ] T033 Write PostgreSQL binding round trips, legacy NULL reads, full → narrowed → scope-less successor scope/binding round trips, immutable assurance-map inheritance, no scope restoration, and validation/error cases in `internal/adapters/storage/postgres/authorization_code_repo_test.go` and `refresh_token_session_repo_test.go`, reusing the shared migrated-database bootstrap rather than starting containers per test (Principle IX; FR-019).
- [ ] T034 Implement sqlx JSONB mapping and explicit column lists in `internal/adapters/storage/postgres/authorization_code_repo.go` and `refresh_token_session_repo.go` for Create/CreateSuccessor/FindBySignature/FindByCodeHash as applicable; enforce exact predecessor-to-successor binding equality and scope subset in the rotation transaction; preserve StorageError wrapping, timeouts, transactions and lock order; make T033 green with `just test-integration-infra` (Principle IX; DEC-004; FR-019).
- [ ] T035 Write JWE continuity/security tests in `internal/domain/oauth2/sessiontoken/service_test.go`: empty fields omitted, server-derived IDs and bindings preserved, existing 10-minute TTL/principal/agent binding, and changed scope/service identity or forged/expired/wrong-user/wrong-agent context rejected by consumers at Save and authorize re-entry; deterministically Save successfully, reassign the canonical ID, then follow returned `redirect_url` with the same JWE and assert `invalid_scope` with no code/upstream redirect; assert unchanged original expiry and no continuation-token forwarding (FR-007, FR-011, FR-019; SR-002).
- [X] T036 Extend `internal/domain/oauth2/sessiontoken/claims.go` with `requested_service_ids` and `service_scope_bindings` (both "omitted when empty"), retaining original URL normalization and existing vetted JWE transport; expose no browser-authoritative requested IDs (FR-007, FR-011, FR-019; SR-002; ADR 016).
- [X] T037 Write code-issuance/hydration/exchange tests in `internal/domain/oauth2server/provider_test.go` and `fosite_storage_test.go` for virtual-scope allow-list exemption, ordinary-scope restrictions, exact requested forms, code-to-refresh bindings, rename/reuse mismatch on code exchange, and unchanged PKCE/client-auth/code-replay checks (FR-002, FR-019–FR-020; SR-001–SR-002).
- [X] T038 Implement shared issuance and binding hydration in `internal/domain/oauth2server/provider.go`, `fosite_storage.go`, and `fosite_client.go`, threading project-native data through existing authorization DTOs in `internal/ports/oauth2.go` and `oauth2server.go` only as needed; retain fosite type containment, signed-token behavior and exact granted scope forms, then make T037 green (FR-019–FR-020; Principles III, VI; ADR 014).

**Checkpoint**: Foundations are green in unit/storage tests. No approval is implied by a green test. Refresh-specific revalidation and consumer integration follow below; do not release an incomplete assurance path.

## Phase 3: User Story 1 — Recover an optional-service tool call through focused consent (Priority: P1)

**Goal**: Challenge → interactive authorization → focused consent → provider login → Save → successful tool retry, preserving user-selected permissions and expiry.
**Independent test**: Run US1 S1–S7 with the production broker/ExtProc and browser harness; refresh widening must fail without consuming the token, and the original tool call must succeed on its first post-consent retry.

### Tests for User Story 1 [MANDATORY - Principle VIII]

- [ ] T039 [P] [US1] Write authorization tests in `internal/domain/oauth2/service_test.go` for validation after redirect validation, mixed ordinary/virtual scopes, request-local requirements plus permanent required services, no-grant/coverage/usability consent outcomes, transient `temporarily_unavailable`, successful refresh without login, duplicate/multiple services, and continuity revalidation after service/requirement changes, especially canonical reassignment after successful Save but before following `redirect_url` with its JWE; assert no code/upstream redirect, preserved client state, no fresh unbound fallback, request-parameter tampering rejection and no continuation-token forwarding (FR-002–FR-003, FR-007–FR-008, FR-019; SR-001–SR-003).
- [ ] T040 [P] [US1] Write consent domain tests in `internal/domain/consent/service_test.go` for both server-derived arrays, every candidate set and supplied required-scope subset plus contribution usability against one snapshot, all usability states, all-scopes/no-requirements with empty configured required scopes but a nonempty selected union and valid access missing one contribution (Save/submission fail), selection/deselection of failing candidates, fresh effective-scope validation, missing/partial selection or unusable-session submission rejection before writes, unchanged existing grants/expiry on failure, and sealed-context tampering (FR-009–FR-011; SR-001–SR-003).
- [ ] T041 [P] [US1] Write contract serialization/error tests in `internal/adapters/http/handlers/consent/agent_detail_handler_test.go` and `grants_handler_test.go` for arrays always present (`[]` without a requested service), baseline and candidate usability enum `[usable, missing, unusable, unavailable]`, unchanged connection-status enum, successful server-built `redirect_url` retaining the same JWE, and `400 requested service not satisfied` without `redirect_url` or persistence (API-CONSENT; FR-009–FR-010).
- [ ] T042 [P] [US1] Write exchange/ExtProc decoder and challenge tests in `internal/domain/tokenexchange/service_test.go`, `internal/adapters/http/enduser/oauth2_token_test.go`, `internal/extproc/server/exchanger_test.go`, and `reauth_challenge_test.go`: the full recoverable scope matrix, no scope on unresolved/nonrecoverable errors, no broker `WWW-Authenticate`, exact header order, malformed optional values omitted, 429/5xx never challenged, unchanged elicitation body and both metadata/header paths (FR-004–FR-005; SR-004).
- [ ] T043 [P] [US1] Write Vitest behavior tests in `web/src/components/consent/RequestedServiceBanner.test.tsx`, `PermissionSetsList.test.tsx`, and `web/src/pages/AgentGrantDetailPage.test.tsx` for every candidate/service marker, editable optional toggles, complementary-scope coverage, per-selected-candidate usability with all-scopes/no-requirements and empty required scopes, immediate Save gating when a failing candidate is selected/deselected, multiple requested services, absent candidates, retryable unavailable status, accessible disabled Save and restored selections/end date/context after provider login (FR-009–FR-011; SC-002; Principle XI).

### Implementation for User Story 1

- [ ] T044 [US1] Integrate the shared requested-set resolution/evaluation into `internal/domain/oauth2/service.go`, reusing normal grant and permanent required checks; seal IDs/bindings in consent, revalidate the same JWE/request/user/agent/expiry and sealed bindings on browser authorize re-entry before issuance or upstream redirect, preserving the original TTL and consuming the continuation parameter locally, and log credential-free `incremental_authorization_required` events while keeping no-virtual-scope behavior unchanged (FR-007–FR-008, FR-011; SR-001–SR-005).
- [ ] T045 [US1] Implement requested-service detail and pre-persistence submission invariants in `internal/domain/consent/service.go`; derive candidate required-scope subsets and effective-contribution usability against one verified session snapshot server-side, evaluate the submitted live effective union afresh, return the original authorize URL with the same JWE after successful Save, preserve existing expiry/unrelated selections, and log `requested_service_submission_rejected` without credentials (FR-009–FR-011; SR-002, SR-005).
- [ ] T046 [US1] Serialize domain results and map the approved error in `internal/adapters/http/handlers/consent/agent_detail_handler.go` and `grants_handler.go`; map internal `unusable_definitive`/`unverifiable_transient` to API `unusable`/`unavailable` and leave coverage/business decisions in domain services (API-CONSENT; Principles IV, VI).
- [X] T047 [US1] Add semantic-red widening tests in `internal/domain/oauth2server/provider_test.go` for adding a virtual scope to an ordinary refresh grant, retaining the original token for a subsequent valid refresh; cover both restricted and empty allow-lists (FR-012; US1 S2; API-LOCAL-SCOPES).
- [X] T048 [US1] Implement the virtual-scope widening refusal before rotation in `internal/domain/oauth2server/provider.go` and scope strategy handling as needed; return the approved `invalid_scope` description without revoking/rotating the original session, then make T047 green (FR-012; US1 S2).
- [ ] T049 [US1] Write recovery URL tests in `internal/domain/oauth2session/service_test.go` and exchange tests in `internal/domain/tokenexchange/service_security_test.go`: configured origin only, resolved canonical/UUID fallback, encoded same-origin detail return, login versus consent recovery distinction, and no changes to permanent agent requirements (FR-004, FR-006; SR-004; ADR-RECOVERY).
- [ ] T050 [US1] Add recoverable `Scope` to `internal/domain/tokenexchange/errors.go` and populate it after service resolution in `service.go`; serialize it in `internal/adapters/http/enduser/oauth2_token.go`; update `ServiceAuthorizeURL` and consumers in `internal/domain/oauth2session/service.go` to use the resolved service identity and approved service-page return, preserving consent links on grant denial (FR-004, FR-006; ADR-RECOVERY).
- [ ] T051 [US1] Decode scope and thread optional route metadata through `internal/extproc/server/exchanger.go`, `server.go`, and `reauth_challenge.go`; enforce the contract's scope URI checks and metadata rules: absolute HTTPS/nonempty host, no userinfo/fragment, byte range `0x21`–`0x7E` excluding double quote (`"`) and backslash (`\`), and "at most 2048 bytes"; omit invalid/non-string optional values, log `challenge_metadata_invalid` without their values, and log presence booleans only (FR-005; API-METADATA-CONFIG; SR-004–SR-005; ADR 036).
- [ ] T052 [US1] Extend typed consent responses in `web/src/types/consent.ts` and `web/src/services/api/consent.ts` for requested IDs/coverage and the exact approved enums; retain the sealed `session_token` across reads/submissions without treating client-supplied IDs as authority (API-CONSENT; FR-009–FR-011; SR-002).
- [ ] T053 [US1] Implement `web/src/components/consent/RequestedServiceBanner.tsx` and requested candidate/service markers in `PermissionSetsList.tsx` and `PermissionSetCard.tsx` using design-system primitives and semantic tokens; explain partial required-scope coverage and candidate-specific missing-scope usability without implying a candidate alone suffices (FR-009–FR-010; Principle XI).
- [ ] T054 [US1] Integrate live-selection coverage, baseline plus every selected candidate's contribution usability for Save gating (including all-scopes/no-requirements without exposing raw scope lists) and `aria-describedby` reasons in `web/src/pages/AgentGrantDetailPage.tsx` and `web/src/hooks/useAgentGrants.ts`; reuse existing consent return state for original request, end date and unrelated toggles, invalidate/refetch relevant detail data after login, and never auto-select or lock optional sets (FR-009–FR-011; SC-002).
- [ ] T055 [US1] Wire shared scope/usability dependencies into authorization, consent, issuance and exchange in `internal/app/builder.go`, updating native DTO consumers and `internal/adapters/http/enduser/proceed_strategy.go` as needed; add builder/handler regressions proving production construction, no handler-to-repository bypass and no adapter cross-imports (Principles VI, XII; FR-007–FR-011).
- [ ] T056 [US1] Make US1 S1–S7 green in `tests/e2e/incremental_authorization_test.go`, `incremental_authorization_gateway_test.go`, and `frontend/incremental_authorization_test.go`; run `just check`, `just test`, `just extproc-test`, `just web-test`, `just test-e2e-backend`, and `just test-e2e-frontend`; record the first successful post-consent retry and absence of silent loops (SC-001–SC-002; Principles VIII, XIII).

## Phase 4: User Story 3 — Keep refresh and upstream authorization truthful (Priority: P1)

**Goal**: Local refresh rechecks all granted services before rotation; proxy flows strip virtual scopes and preserve upstream outcomes without claiming service usability.
**Independent test**: US3 S1–S4 exercise local success/failure, scope-less refresh, mixed-scope proxy authorization/refresh and rotated upstream credentials followed by challenged exchange and repair.

### Tests for User Story 3 [MANDATORY - Principle VIII]

- [ ] T057 [P] [US3] Write local refresh tests in `internal/domain/oauth2server/provider_test.go` and `refresh_consent_test.go` for every immutable bound service even with omitted/narrowed request scope or a binding no longer in current `Scope`, all-scopes/no-requirements valid access missing a permission-set scope despite empty required scopes, full → narrowed → scope-less success with narrowed response/JWT/successor scopes and all bindings retained, subsequent unusable-service refusal, re-requested removed-scope widening refusal without consumption, eligibility/coverage changes, canonical rename/reuse, missing binding, definitive `invalid_scope`, transient `server_error`, no rotation/revocation on either, and revoked-grant `invalid_grant` precedence (FR-008, FR-012, FR-019–FR-020; SR-003).
- [ ] T058 [P] [US3] Write proxy tests in `internal/domain/oauth2/service_test.go` and `internal/adapters/http/enduser/oauth2_token_proxy_test.go` for mixed/virtual-only/ordinary scopes, omit-empty upstream scope, byte-preserved unrelated form fields and success/error responses, rotated refresh token propagation, and zero post-response provider usability calls (FR-013, FR-021; DEC-002).
- [ ] T059 [P] [US3] Extend mode/security tests in `internal/domain/oauth2/mode_strategy_test.go` and `internal/domain/oauth2server/provider_client_assertion_test.go` to cover hybrid per-agent selection, CIMD public/confidential client continuity, `client_credentials` virtual-scope `invalid_scope`, and unchanged PKCE, client authentication, code replay and refresh-chain consent protections (FR-002, FR-013, FR-020; SR-001; ADR 014).

### Implementation for User Story 3

- [ ] T060 [US3] Revalidate the complete immutable assurance binding set (not just virtual scopes still in current `Scope`) with current grant-derived effective provider scopes after fosite request validation and before `PopulateTokenEndpointResponse` in `internal/domain/oauth2server/provider.go` and `fosite_storage.go`; fail closed on missing/mismatched bindings, enforce eligibility/coverage/usability, persist response/JWT/successor `Scope` as the requested subset or predecessor current `Scope` when omitted, inherit all bindings unchanged without restoring removed scopes, preserve no-consumption outcomes, reject userless `client_credentials`, and log `refresh_service_recheck_failed` with definitive/transient reasons but no token values (FR-002, FR-008, FR-012, FR-019–FR-020; SR-003, SR-005).
- [ ] T061 [US3] Strip broker virtual scopes in the domain-built upstream authorization URL in `internal/domain/oauth2/service.go` and through a domain-owned pure scope operation consumed by `internal/adapters/http/enduser/oauth2_token_proxy.go`; omit empty scope, leave upstream bodies/status/headers/rotation unchanged, and preserve startup strategy and hybrid dispatch rather than introducing a third policy (FR-013, FR-021; DEC-002; Principle VI).
- [ ] T062 [US3] Add credential-free audit assertions in `internal/domain/oauth2server/provider_test.go` for definitive/transient local refresh refusal and in `internal/adapters/http/enduser/oauth2_token_proxy_test.go` for response preservation, ensuring no broker scope claim is fabricated in upstream tokens (FR-013, FR-021; SR-005).
- [ ] T063 [US3] Add zero-extra-I/O regressions for requests without virtual scopes or retained assurance bindings in `internal/domain/oauth2/service_test.go` and `internal/domain/oauth2server/provider_test.go` using dependency call counts, then retain that fast path in both services (plan Performance Goals; FR-007, FR-012; Principle VIII).
- [ ] T064 [US3] Make US3 S1–S4 green in `tests/e2e/incremental_authorization_test.go` and `incremental_authorization_proxy_test.go`; run `just check`, `just test`, and `just test-e2e-backend`, proving no unusable local success and zero virtual scopes in recorded upstream requests (SC-004–SC-005; Principles VIII, XIII).

## Phase 5: User Story 2 — Repair the service by opening its scope link (Priority: P1)

**Goal**: Authenticated single-service management with Login, Refresh and Terminate, without implicitly granting an agent permission.
**Independent test**: US2 S1–S4 cover both identifiers, no-session versus unknown-service, provider return, active-session actions and successful exchange after direct recovery.

### Tests for User Story 2 [MANDATORY - Principle VIII]

- [ ] T065 [P] [US2] Write domain detail tests in `internal/domain/oauth2session/session_details_security_test.go` for canonical/UUID resolution, nil session versus unknown service, persisted rejected state, principal-owned dependent/waiting agents only, and zero provider calls on reads (FR-015–FR-016; SR-005).
- [ ] T066 [P] [US2] Write handler/routing contract tests in `internal/adapters/http/oauth2_sessions/handler_test.go` for `200` with `session: null`, `401`, unknown-service `404`, storage `500`, approved service/agent fields, and unchanged UUID-only legacy read/refresh/terminate routes (API-SESSION-DETAIL; FR-015).
- [ ] T067 [P] [US2] Write Vitest tests in `web/src/pages/ThirdPartySessionDetailPage.test.tsx` and `web/src/services/api/sessions.test.ts` for no-session/unusable/active/loading/not-found/retryable errors, visible identity/scopes/expiry/agents, login return, existing refresh and confirmed termination, and refreshed cache state (FR-015–FR-016; Principle XI).

### Implementation for User Story 2

- [ ] T068 [US2] Add `GetServiceSessionDetail(principal, identifier)` in `internal/domain/oauth2session/service.go`, resolving current canonical/UUID identity, returning service plus nullable session and principal-owned dependent agents, deriving status without provider refresh or implicit grant mutation (FR-015–FR-016; API-SESSION-DETAIL; SR-005).
- [ ] T069 [US2] Add the thin detail handler in `internal/adapters/http/oauth2_sessions/handler.go`, inject it via `internal/app/builder.go` and `handlers.go` as needed, and register `GET /api/third-party/sessions/{service}` with existing principal middleware in `internal/adapters/http/routing/enduser.go`; keep legacy routes and protocol/SPA precedence intact (API-SESSION-DETAIL; Principles VI, XII; ADR 035).
- [ ] T070 [US2] Add typed detail reads in `web/src/services/api/sessions.ts` using `{service, session, dependent_agents}`, nullable session and `scope_url`; reuse existing UUID action methods and cache invalidation for refresh/termination and provider callback return (API-SESSION-DETAIL; FR-015–FR-016).
- [ ] T071 [US2] Implement lazy-loaded `/sessions/:service` in `web/src/App.tsx` and `web/src/pages/ThirdPartySessionDetailPage.tsx`, reusing `web/src/components/sessions/SessionCard.tsx` and `TerminationDialog.tsx` content/actions with design-system primitives; distinguish every planned state and use the same-service login return without granting permissions (FR-015–FR-016; Principles XI, XIII).
- [ ] T072 [US2] Update existing recovery-return assertions in `internal/domain/tokenexchange/service_test.go`, `internal/domain/oauth2session/service_test.go`, and affected tests under `tests/e2e/` from encoded generic `/sessions` to resolved service-page returns only where ADR-RECOVERY applies; preserve same-origin validation, PKCE/state and termination/reconnect regression coverage (FR-006, FR-016, FR-018; ADR-RECOVERY).
- [ ] T073 [US2] Make US2 S1–S4 green in `tests/e2e/service_session_page_test.go` and `frontend/incremental_authorization_test.go`; run `just check`, `just test`, `just web-test`, `just test-e2e-backend`, and `just test-e2e-frontend`, proving authenticated isolation and one-login return to the same page (SC-003; SR-005; Principles VIII, XIII).

## Phase 6: User Story 4 — Integrate without leaking or over-granting scopes (Priority: P2)

**Goal**: Reviewable operator deployment, safe boundary cases, UUID rename durability and no virtual-scope catalog in discovery.
**Independent test**: US4 S1–S4 compare metadata variants, identifiers/rename/reuse, crafted invalid or ineligible requests, and the guide's initial-challenge example against live discovery.

### Tests for User Story 4 [MANDATORY - Principle VIII]

- [ ] T074 [P] [US4] Extend `internal/domain/servicescope/requested_set_test.go`, `internal/domain/oauth2/service_test.go`, and `internal/domain/oauth2server/provider_test.go` with canonical-ID reuse during consent, specifically after successful Save before browser authorize re-entry (follow the server-built URL with its original JWE; assert non-issuance and no proxy redirect), before code exchange and across refresh; cover UUID durability, old-canonical rejection without consumption, canonical-absent emission, duplicate-form identity pinning, malformed/unknown/ineligible scopes and external-origin ordinary-scope rules (FR-001–FR-002, FR-019; SR-001–SR-004).
- [ ] T075 [P] [US4] Add discovery and metadata-boundary regressions in `internal/domain/oauth2/service_metadata_test.go` and `internal/extproc/server/reauth_challenge_test.go` for catalog non-advertisement, absent/invalid/non-string metadata, forbidden HTTP-header sourcing, header injection characters, URI byte/length boundaries, and unchanged outcomes for transient/nonrecoverable broker failures (FR-005, FR-014; SR-004; API-METADATA-CONFIG).

### Implementation for User Story 4

- [ ] T076 [US4] Complete only boundary corrections driven by T074/T075 in `internal/domain/servicescope/requested_set.go`, `internal/domain/oauth2/service.go`, `internal/domain/oauth2server/provider.go`, and `internal/extproc/server/reauth_challenge.go`; retain no alias history, no discovery catalog and exact requested forms with UUID-bound continuity (FR-002, FR-014, FR-019–FR-020).
- [ ] T077 [US4] Write `docs/guides/incremental-authorization.md` with scope syntax/current identifiers, initial gateway route challenge, optional protected-resource metadata publishing, strict gateway JWT policy and route metadata, focused consent versus direct URL repair, bounded client-owned retry policy, local/proxy/hybrid limitations, rename recovery and rollback impact; state explicitly that client implementation and metadata hosting are out of scope (FR-017, FR-019, FR-021; SC-006).
- [ ] T078 [US4] Link the guide from `docs/guides/token-exchange-gateway.md` and update `docs/api/enduser-api.md`, `docs/reference/token-exchange.md`, and `docs/configuration.md` to the approved OpenAPI and metadata contract, including transient versus definitive failures, non-rotating local errors, proxy assurance limits, and unchanged ordinary URL elicitation (API-002–API-003; FR-004–FR-006, FR-012–FR-017, FR-021).
- [ ] T079 [US4] Validate examples through the existing loader/test conventions in `tests/integration/config_test.go` and parse the guide route example in `tests/e2e/incremental_authorization_docs_test.go`; keep tests tied to effective contract behavior, not prose, and make US4 S1–S4 green through `just test-integration` and `just test-e2e-backend` (FR-017; SC-005–SC-006; Principle VIII).

## 🔒 Phase N: Constitution Compliance Verification [MANDATORY COMPLIANCE SECTION]

**Purpose**: Verify constitution requirements and feature-scope compliance. All stories, not only the MVP checkpoint, are required for completion.

### 🔒 Constitution Compliance Verification [MANDATORY]

#### Design Phase Verification [MANDATORY]

- [ ] T080 Verify every task and implemented change traces to the feature requirements or a binding constraint, and the 11 sign-off items in `specs/055-incremental-service-authorization/plan.md` are covered by supplied approval before their implementation; verify accepted ADR status/approval URLs in `adrs/045-service-session-recovery-return.md` and `adrs/046-virtual-service-scopes.md`, with no edit to merged ADR 044 (Principles II, IV, X; Feature Scope & Deferred Findings).
- [ ] T081 Verify `ARCHITECTURE.md` has the model/glossary and `examples/config/README.md` links both concrete examples; verify `docs/configuration.md` and the owning gateway guide explain absence/default/validation and the standalone deployment boundary without irrelevant broker-chart configuration (Principles II, V, VII).
- [ ] T082 Verify `api/enduser/openapi.yaml` exactly captures approved contracts and version 1.2.0, the admin/legacy UUID APIs remain unchanged, and migration 046 matches `specs/055-incremental-service-authorization/data-model.md`, including rollback binding loss (Principles IV, IX, X).
- [ ] T083 Verify `specs/055-incremental-service-authorization/spec.md` Journeys 1–4 still match implemented configuration, HTTP, decoded synthetic JWT claims and ordered Wireloom source/SVGs; verify the design-system composition decisions are documented in `quickstart.md` without editing immutable spec/plan during implementation (Principles XI, XIV).
- [ ] T084 Verify all 19 scenario mappings below exist in `tests/e2e/`, each `It` maps to exactly one scenario with a nearby reference, uses production bootstrap/fixtures and independent state, and has recorded compiling semantic-red evidence before implementation with no skipped/pending specs or vacuous assertions; verify browser tests and four maintained screenshot captures (Principles VIII, XIII).

#### Implementation Phase Verification [MANDATORY]

**Feature Scope** (Development Requirements):

- [ ] T085 Review the diff against `specs/055-incremental-service-authorization/tasks.md` and `research.md` Deferred Findings; exclude independent refactoring/hardening and leave unrelated findings as non-task notes, with only user-approved issue URLs under Future Work; do not create issues without authorization (Feature Scope & Deferred Findings).

**API & Documentation** (Principles IV, X):

- [ ] T086 Verify handler responses and error precedence against `api/enduser/openapi.yaml` and `specs/055-incremental-service-authorization/contracts/enduser-api.md`; reconcile `docs/api/enduser-api.md` and `docs/reference/token-exchange.md` with implementation without introducing an unapproved contract change (Principles IV, X).

**Architecture & Documentation** (Principle II):

- [ ] T087 Update `ARCHITECTURE.md` OAuth2 modes, consent, exchange recovery, session management, ExtProc challenge, ADR index and API-version note; update package maps/API routing notes in `AGENTS.md`, `internal/AGENTS.md`, `internal/domain/AGENTS.md`, `internal/extproc/AGENTS.md`, and `web/AGENTS.md`; verify moved ADR links/pointers and the single shared rule agree with final accepted ADRs (Principles II, V).

**Configuration** (Principle VII):

- [ ] T088 Verify `internal/ports/config.go` remains the broker configuration source, no ad-hoc loader or new process key was introduced, the route metadata remains confined to `aib.tokenexchange`, and `charts/agentic-identity-broker/` is unchanged for the documented standalone setting (Principle VII; ADRs 011/036).

**Database & Persistence** (Principle IX):

- [ ] T089 Verify both memory and PostgreSQL adapters implement the same binding semantics, including deep-copy isolation and successor inheritance, explicit SQL projections, StorageError wrapping, timeouts and existing transaction/lock ordering in `internal/adapters/storage/memory/` and `internal/adapters/storage/postgres/`; run `just test-integration-infra` for real migration apply/rollback/repeat and both repository round trips (Principle IX; DEC-004; FR-018–FR-019).

**Security** (Principles I, III):

- [ ] T090 Verify security tests in `internal/domain/servicescope/`, `oauth2/`, `oauth2server/`, `oauth2session/`, `consent/`, and `tokenexchange/` prove fail-closed eligibility/coverage/usability, JWE principal/agent/TTL protection, canonical-ID pinning, no provider-scope leakage and safe configured-origin recovery; preserve existing JWE/fosite/PKCE/signature mechanisms with no custom crypto or optional bypass (Principles I, III; SR-001–SR-004).
- [ ] T091 Verify credential-free structured denial/refresh-transition logs in `internal/domain/oauth2/service.go`, `consent/service.go`, `oauth2server/provider.go`, `oauth2session/service.go`, and `internal/extproc/server/reauth_challenge.go`, including required event names, definitive/transient distinctions, value-free invalid metadata diagnostics and presence-only challenge attributes; verify other users' agent relationships never appear (Principle I; SR-005).

**Architecture Patterns** (Principles VI, XII):

- [ ] T092 Verify `internal/domain/servicescope/` is the sole invariant owner with no infrastructure imports, handlers do no conditional business orchestration or direct repository access, adapters do not import each other, fosite remains contained, and `internal/extproc/` does not import broker scope packages; verify all DI in `internal/app/builder.go` and route registration only in `internal/adapters/http/routing/enduser.go` (Principles VI, XII; ADRs 014/036).

**Testing** (Principle VIII - Unit & Integration Tests):

- [ ] T093 Verify unit/Vitest/integration tests were written first, compiled and failed semantically for missing behavior, drove implementation and changed minimally; run `just check`, `just test`, `just extproc-test`, `just web-test`, and `just test-integration`; record commands/outcomes in `specs/055-incremental-service-authorization/quickstart.md`, never using Bash correctness scripts or accepting skipped infrastructure as a pass (Principle VIII).

**E2E Acceptance Testing** (Principle XIII):

- [ ] T094 Run `just test-e2e-backend`, `just test-e2e-extproc`, and `just test-e2e-frontend`; verify all mapped scenarios in `tests/e2e/` turn green with only minimal fixture adjustments and concrete full-journey assertions, production bootstrap, Ginkgo/Gomega hierarchy, isolation and scenario comments intact (Principle XIII; SC-001–SC-006).
- [ ] T095 Capture the four meaningful UI states using the existing `E2E_CAPTURE_SCREENSHOTS=true GINKGO_FRONTEND_PROCS=1 just test-e2e-frontend` harness path, retain descriptive artifacts under `tests/e2e/screenshots/` using existing screenshot handling without changing workflows, and verify images against the consent C/D and session A/B Wireloom states (Principles XI, XIII, XIV).

**Frontend** (Principle XI - if applicable):

- [ ] T096 Verify `web/src/components/consent/RequestedServiceBanner.tsx` and `web/src/pages/ThirdPartySessionDetailPage.tsx` reuse design-system primitives and semantic tokens, no custom palette/CSS or external assets, keyboard/screen-reader Save reasons, focus, light/dark WCAG 2.1 AA contrast (4.5:1 text, 3:1 UI), and loading/retry/not-found handling; confirm no new universal component, or require any genuinely reusable addition to have design-system stories/accessibility coverage in both themes (Principle XI).

**Final verification and handoff**:

- [ ] T097 Run the feature's full required gate `just verify` from `justfile`, including real infra-backed migrations and all E2E suites; fix change-caused failures without weakening checks and report unavailable infrastructure as a blocker, not success; update `specs/055-incremental-service-authorization/quickstart.md` with actual outcomes and any deferred task/path/reason (Principles VIII, IX, XIII).
- [ ] T098 Verify `specs/055-incremental-service-authorization/checklists/requirements.md` is satisfied by evidence and every completed task in `tasks.md` is accurately checked; leave unfinished tasks/checklist items unchecked and report their dependency/blocker and required maintainer action, while keeping the PR draft and spec/plan unchanged (Principles II, VIII, XIII; implementation handoff rules).

## Implementation Follow-ups from Reanalysis

These tasks retain the two MEDIUM findings from the supplied reanalysis. The recommended
contract revisions and alternatives are recorded in [quickstart.md](./quickstart.md#remaining-reanalysis-contract-boundaries),
not silently added to the immutable spec/plan or accepted ADR text. Both remain unchecked.

- [ ] T099 [US3] Resolve I2's externally visible proxy-refresh pre-forward contract under explicit approval: grammar validation, existing client-to-agent resolution, current-service lookup and eligibility before forwarding; proposed `400 invalid_scope` / `500 server_error` outcomes are detailed in `quickstart.md`. Reconcile editable supporting contracts/research and approved ADR text only after approval; keep spec/plan revisions in an authorized pre-implementation run. Add malformed, unknown and ineligible variants to US3 S3 in T014 and to T058/T061; assert zero upstream calls on rejection, no credential consumption, and no provider-usability check or post-response veto. Ordinary-only requests and upstream response/rotation remain unchanged (FR-002, FR-013, FR-021; reanalysis I2).
- [ ] T100 [US2] Resolve U4 under explicit API/ADR approval: the recommended exception emits UUID scopes and recovery/page URLs for complete canonical IDs `.` and `..` while keeping existing records, canonical validation, raw requested scope strings and immutable bindings unchanged. Reconcile the immutable spec/plan in an authorized pre-implementation run, then supporting contracts/data model/research/ADRs and OpenAPI. Add grammar/emission variants to T019, recovery variants to T049 and browser direct-navigation/provider-return variants inside US2 S1 (T015), with exact-form grant and UUID durability regressions; do not silently tighten global canonical-ID validation or implement an unapproved fallback (FR-001, FR-006, FR-015, FR-019–FR-020; reanalysis U4).

- [X] T101 Add self-contained approved-contract validation in `tests/integration/incremental_authorization_contract_test.go` for nullable service detail and required fields, baseline/candidate usability enums, mandatory requested-service arrays, consent examples, local token examples and `invalid_scope`. Observe semantic failures for `session: null` and the error enum before correcting their OpenAPI representation; preserve legacy `SessionStatus` and `AffectedAgent` schemas and validate through `just test-integration` (T008; Principles IV, VIII, X).

T008 and T015 cover only the originally approved contract and scenarios. T099/T100 remain unchecked;
no pre-forward rejection contract or dot-segment URL exception is inferred from those completions.
T003 remains unchecked until the emission model is reconciled with the authorized U4 resolution.

## Acceptance Scenario → Task Mapping

The plan's exact `It` descriptions remain authoritative. The following assigns one owning test to every scenario; unit/integration tests add depth without duplicating acceptance ownership.

| Scenario | Owning file under `tests/e2e/` | Red task | Green task | Principal result |
|---|---|---|---|---|
| US1 S1 | `incremental_authorization_gateway_test.go` | T013 | T050–T051, T056 | Persist rejected refresh; scope, action and elicitation survive gateway |
| US1 S2 | `incremental_authorization_test.go` | T012 | T048, T056 | No refresh widening/consumption; interactive consent |
| US1 S3 | `incremental_authorization_test.go` | T012 | T044–T046, T056 | Server-derived service/candidate coverage and preserved grant |
| US1 S4 | `frontend/incremental_authorization_test.go` | T016 | T052–T056 | User selects covering scopes, logs in, saves, retries successfully |
| US1 S5 | `incremental_authorization_test.go` | T012 | T045–T046, T056 | Forged uncovered submission cannot mutate grant |
| US1 S6 | `incremental_authorization_test.go` | T012 | T044–T046, T056 | Every requested service satisfied without permanent config changes |
| US1 S7 | `incremental_authorization_test.go` | T012 | T025, T044, T056 | Eligible provider refresh avoids unnecessary login |
| US2 S1 | `service_session_page_test.go` | T015 | T068–T071, T073 | Both identifiers, no-session detail, login return |
| US2 S2 | `frontend/incremental_authorization_test.go` | T016 | T070–T073 | Active detail, refresh and confirmed termination |
| US2 S3 | `service_session_page_test.go` | T015 | T068–T069, T073 | Auth boundary and authenticated unknown-service not found |
| US2 S4 | `service_session_page_test.go` | T015 | T050, T068–T073 | Direct repair restores exchange without new grant |
| US3 S1 | `incremental_authorization_test.go` | T012 | T060, T064 | Covered/usable refresh success; unusable refusal |
| US3 S2 | `incremental_authorization_proxy_test.go` | T014 | T061, T064 | Requested-service checks before stripped upstream authorization |
| US3 S3 | `incremental_authorization_proxy_test.go` | T014 | T061, T064 | Upstream rotated response survives; later exchange challenges |
| US3 S4 | `incremental_authorization_test.go` | T012 | T038, T060, T064 | Exact token/JWT virtual scopes and scope-less revalidation |
| US4 S1 | `incremental_authorization_gateway_test.go` | T013 | T051, T079 | Metadata-present/absent challenge variants |
| US4 S2 | `incremental_authorization_test.go` | T012 | T076, T079 | Identifier/rename/reuse continuity and no failed-refresh consumption |
| US4 S3 | `incremental_authorization_test.go` | T012 | T076, T079 | Crafted service scope never over-grants |
| US4 S4 | `incremental_authorization_docs_test.go` | T017 | T077–T079 | Deployable guide example and no discovery catalog |

## Requirement Coverage

| Requirements | Main build/test tasks |
|---|---|
| FR-001–FR-003 | T019–T023, T037–T039, T044, T059–T060, T074–T076 |
| FR-004–FR-006 | T013, T042, T049–T051, T072 |
| FR-007–FR-008 | T012, T021–T026, T035–T036, T039, T044–T045, T057, T060 |
| FR-009–FR-011 | T012, T016, T035–T036, T040–T046, T052–T055 |
| FR-012 | T012, T029–T030, T047–T048, T057, T060 |
| FR-013–FR-014 | T014, T017, T058–T061, T075–T076 |
| FR-015–FR-016 | T015–T016, T065–T073 |
| FR-017 | T005–T007, T017, T077–T079 |
| FR-018 | T024–T026, T029–T034, T072, T089 |
| FR-019–FR-020 | T019–T022, T027–T038, T057–T060, T074–T076 |
| FR-021 | T014, T058, T061–T062, T077–T078 |
| API-001–API-004 | T008, T037–T042, T046, T050–T052, T057–T061, T066, T069–T070, T078, T086 |
| SR-001–SR-005 | T019–T025, T035–T051, T057–T063, T065–T069, T074–T076, T090–T092 |
| SC-001–SC-006 | All 19 mapped scenarios; T056, T064, T073, T079, T094–T097 |

## Dependencies & Execution Order

### Phase Dependencies

```text
T001 approval recording (only supplied approvals) → T002
  → Phase 2a–2e design/approved contract work + Phase 2f acceptance tests
  → T018 compiling semantic-red checkpoint
  → Phase 2.5 shared foundations
  → US1 focused consent core (P1)
  → US3 refresh/proxy assurance (P1)
  → US2 service-page recovery (P1)
  → US4 operator/boundary coverage (P2)
  → Phase N compliance + full verification
```

- Design work T003–T010 and independent test files T012–T017 may be prepared in parallel after T001/T002 and fixture T011 readiness, but no feature logic begins before T018.
- Foundation pairs are test-first: T019→T020; T021→T022→T023; T024→T025→T026; T027→T028; T029→T030; T031→T032; T033→T034; T035→T036; T037→T038. T021 depends on scope types, T024 on the usability contract, T029/T033 on record scaffolding, and T033's green run needs migration 046. T035/T036 and binding persistence precede T037/T038.
- Every story's unit/contract/Vitest tests must fail semantically before its matching implementation. Shared-file edits are serialized, even when separate tests can be prepared independently.
- US1 owns widening refusal; US3 completes continuing refresh assurance. US2 consumes US1's recovery URL and the shared usability state. US4 integrates rename/metadata/operator variants across them. All three P1 stories and US4 remain required for completion.
- Approval dependencies: exchange/ExtProc tasks need API-EXCHANGE-CHALLENGE; metadata tasks also need API-METADATA-CONFIG; authorization/consent tasks need API-CONSENT; detail tasks need API-SESSION-DETAIL; local issuance/refresh tasks need API-LOCAL-SCOPES. Rename/pinning depends on DEC-001, DEC-004 and ADR-SCOPES; proxy refresh depends on DEC-002; rejection-state changes on DEC-003; recovery target on ADR-RECOVERY. Missing coverage defers dependent work, not independent approved tasks.

### Parallel Opportunities and Per-Story Examples

These are scheduling examples only, not delegation to another pipeline phase. Each row assumes all prerequisite foundation tasks and approvals are ready.

| Story | Independent test work that can run together | Serialization/integration boundary |
|---|---|---|
| US1 | T039 authorization tests, T040 consent tests, T041 handler tests, T042 exchange/ExtProc tests, T043 Vitest tests | Merge tests before matching green work; T044→T045→T046 and T052→T053→T054; T055 after domain integration |
| US3 | T057 refresh tests, T058 proxy tests, T059 mode/auth tests | T058 and T059 do not overlap files, but serialize any shared authorization/provider edits with US1; T060/T061 before T064 |
| US2 | T065 domain tests, T066 handler tests, T067 SPA/API tests | T068→T069 and T070→T071; integrate both before T073 |
| US4 | T074 scope/continuity tests and T075 discovery/header tests | T076 after both red batches; T077 guide and T078 references before T079 document checks |

There is no blanket story-level parallelism: authorization/provider/session/ExtProc files are shared. Documentation work can run alongside dependency-ready code work only when files do not overlap. Final checks run against the integrated tree.

## Implementation Strategy

1. Establish approval evidence, design prerequisites and all 19 acceptance tests before feature logic.
2. Build the single invariant owner and binding/usability foundations with test-first pairs and both storage adapters.
3. Deliver US1 as the focused-consent core/MVP checkpoint, then immediately complete US3's refresh assurance and US2's URL recovery. US1 alone is not a releasable implementation of the approved feature: the P1 security and compatibility stories are mandatory.
4. Add US4's operator guide and cross-path boundary checks, preserving every requirement and approval boundary.
5. Run compliance and the full verification gate. Mark only evidence-backed completed tasks, keep the PR draft, and report any remaining task or unavailable validation as blocked rather than declaring partial success.

## Notes

Task generation did not execute implementation work. The current checked tasks record only the setup/design-precondition progress described in the implementation checkpoint. Acceptance red/green evidence, feature behavior and final passing validation remain unfinished. The existing specification-quality checklist is read-only and is not implementation evidence.

**Future Work**: The unrelated unrequested-service refresh behavior in `research.md` remains deferred, not an implementation task. No new issue is authorized here.
