# Tasks: Invalidate Sessions After Service Client-ID Changes

**Input**: `specs/056-invalidate-client-sessions/{spec.md,plan.md,research.md,data-model.md,quickstart.md,contracts/}`.

**Prerequisites**: API-001, API-002, ADR-001, and DEC-001–DEC-005 have maintainer approval from jbspeakr dated 2026-11-05 ([approval](https://github.com/zalando-incubator/agentic-identity-broker/pull/412#issuecomment-6658788612)). This list grants no additional approval. Only performed and validated tasks are checked. Test-writing completion does not mean the tested behavior is implemented.

**Tests**: Mandatory red-green-refactor (Principles VIII, XIII). Write compiling tests before corresponding behavior changes. Observe semantic failure when the environment supports the suite. Container-backed suites can first run in CI with implementation included. Minimal types for compilation are permitted. Missing symbols, skipped tests, and vacuous assertions do not satisfy TDD. Recorded red/green output, logs, and CI links are not required completion evidence. Historical run summaries are informational.

**Organization**: Setup → mandatory design preconditions → foundation → US1 (P1) → US2 (P1) → US3 (P2) → mandatory compliance. Paths are relative to the repository root.

**Task summary**: All 80 tasks are checked. T079 records explicit user acceptance of F1 without code changes; T080 resolves F2's unknown-service failure logging. Phase 7 package, API, backend-journey, and browser validation passes. T069 validation execution is complete, but its recorded security gate still fails on dependency advisories. Full fast Go, PostgreSQL/integration, backend E2E, and Docker E2E gates pass, as recorded in the final CI repair validation. Structural quality-delta remains non-clean; Phase 8's scoped check has no gating findings.

## 🔒 Feature Scope [MANDATORY]

Every task cites a requirement, scenario, or binding principle. Scope is service client-identity invalidation, recovery, isolation, and the documentation and validation necessary to deliver them. No provider revocation, unsolicited browser navigation, new dependency, runtime switch, independent cleanup, or workflow change is included. `plan.md` records that no preparatory refactoring or new-entity CRUD scaffolding is needed. If required refactoring emerges, isolate it in a behavior-preserving commit before feature commits in this PR, with existing tests passing unchanged (Principles II, VIII).

### Recovery checkpoint (acceptance coverage complete)

Foundation behavior was implemented while required acceptance matrices remained incomplete. This violated the original Phase 2 gate and Principle XIII. The historical record remains intact.

T011–T013, T015, and T018 are complete as acceptance-test writing and traceability tasks. The non-container cases run with semantic failures for missing production enforcement; container-backed variants compile and can first execute in CI. See the current recovery record below for locations and validation limits.

Acceptance recovery, foundation, and T070 are complete. The historical sequencing violation remains recorded. Compliance execution is complete; the security gate remains failing and container-backed validation remains unverified, as documented at the end of this file.


## Phase 1: Setup (Approval Record)

- [X] T001 Record the supplied maintainer approval in the existing `adrs/048-client-identity-revisions.md`. Preserve its numbered filename. Retain partial supersession of `038-proactive-token-refresh` and `041-oauth2-client-secret-file-overrides`, and preservation of `046-observed-refresh-token-rejection` (ADR-001; Principle II). The feature-local file is a historical reference, not a second decision. For a genuinely new ADR, use `YYYYMMDD-HHMMSS-<slug>.md` with the UTC creation time. Cite full file stems in new or changed references. Do not invent approval or amend a main-merged Accepted ADR.

## 🔒 Phase 2: Design Preconditions (Blocking Prerequisites) [MANDATORY]

**Purpose**: Complete domain, configuration, API, database, journey, E2E, and observability design before implementation. The design and acceptance recovery checkpoint is complete. Container availability does not defer test writing. Container-backed execution can first occur in CI without recorded red/green evidence.

### Phase 2a: Domain Model & Glossary [MANDATORY]

- [X] T002 Reconcile `specs/056-invalidate-client-sessions/data-model.md` with `contracts/ports.md` and the approved ADR: existing service/session aggregates, exact-ID comparison, monotonic per-service revision, callback-only establishment, legacy `R == 0`, and ADR 046 predicate composition; preserve no backfill and no new typed entity ID (FR-001–FR-005, FR-013, FR-015; Principles II, V).
- [X] T003 Update `ARCHITECTURE.md` credential-source ownership (preserve administrative no-file-read metadata; explicitly replace the consent/session usability no-file-read rule under approved ADR 048), guarded rollout/rollback procedure, session lifecycle, both API major cutovers, and glossary with Effective Client Identity and Client Identity Revision; extend the existing Client-identity association and Requires Reauthentication entries rather than adding synonyms (FR-001–FR-015; Principles II, V). Update the ADR index and glossary in `AGENTS.md` only as ordinary architecture documentation, not via a context-update hook.

### Phase 2b: Configuration Design [MANDATORY]

- [X] T004 Confirm and document in `docs/configuration.md` that existing `third_party_oauth2.credential_files` and `server.enduser.public_url` suffice and enforcement has no opt-out; add a procedure reference in `examples/config/third-party-oauth2.yaml` and its `examples/config/README.md` entry, preserving real keys/defaults and synthetic examples (FR-003, FR-012; SR-001, SR-003; Principle VII).
- [X] T005 Document migrations 049/050, coherent publication, and ADR 048's continuous rolling upgrade in `charts/agentic-identity-broker/README.md`: freeze effective identity/source/canonical-ID/binding mutations and pause identity-mutating automation/direct writers and DCR registrations while keeping both ingress planes and existing traffic open; same-ID rotation and metadata-only writes continue. Apply additive migrations online before new replicas start, verify prior-release expanded-schema compatibility, and allow old/new serving only under unchanged identities. Drain each old process and its admitted work while healthy new replicas serve; verify complete revision-aware serving/writing inventory and prevent old-image restart before resuming identity changes. Failed checks preserve healthy traffic and the freeze. Pre-activation prior-binary abort requires unchanged identities, zero increments, drained new work, and retained evidence; post-activation rollback is revision-aware only. Confirm no Helm values/template or workflow/hook change is needed (DEC-002, DEC-004, DEC-005; Principles I, II, VII, IX). Never force past failed validation or discard evidence.

### Phase 2c: API Design [MANDATORY]

- [X] T006 Verify supplied approval covers API-001 and API-002, then apply `specs/056-invalidate-client-sessions/contracts/enduser-api.md` to `api/enduser/openapi.yaml`: approved 1.2.0 → 2.0.0 cutover, existing reauthentication/alias fields and inherited detail schema, connection status, automatic consent, stale callback denial, token infrastructure versus recovery outcomes, mandatory-service validation, and identity-related `409 refresh_unavailable` while preserving `409 refresh_rejected` (FR-006–FR-010, FR-014, FR-015; Principles IV, X). Keep the existing approval reference in Status and the implementation record. Reconcile factual artifact status without changing approved behavior.
- [X] T007 [P] Apply the approved `specs/056-invalidate-client-sessions/contracts/admin-api.md` major cutover to `api/admin/openapi.yaml`, 2.1.1 → 3.0.0: change sweep classification of identity-invalid candidates from refreshed to failed, including dry runs, and update both count descriptions/examples and consumer migration notes; retain request validation, schemas, statuses, ETags, internal-field non-binding/non-exposure, and dry-run no-write semantics (FR-003, FR-012; API-002; Principles IV, X).

### Phase 2d: Database Design [MANDATORY]

- [X] T008 Write migration tests first in `tests/integration/migrations/client_identity_revision_migration_test.go` using `framework.go` and shared PostgreSQL historical templates: 049/050 UP/DOWN/reapply, preserved ciphertext and null legacy identity, default revision 0, non-negative/non-empty constraints, lock contention with NOWAIT/timeout while live traffic continues, old-release explicit-column reads and writes against expanded schema (preserving unknown evidence columns and default-0 legacy inserts), atomic failure, validation state, and DOWN refusal after any service revision > 0 without data/schema mutation (FR-005, FR-013; DEC-002; Principle IX). Keep container tests integration-tagged and run them in container CI when local infrastructure is unavailable.
- [X] T009 Create `migrations/049_add_client_identity_revision.up.sql`, `migrations/049_add_client_identity_revision.down.sql`, `migrations/050_validate_client_identity_revision.up.sql`, and `migrations/050_validate_client_identity_revision.down.sql` exactly per `data-model.md`: nullable `client_identity`, service/session `BIGINT NOT NULL DEFAULT 0`, `NOT VALID` checks, `SET LOCAL statement_timeout = '1s'`, required NOWAIT locks, guarded DOWN 049, and DOWN 050 restoring unvalidated constraints; no identity backfill, wildcard projections, deletion, or non-atomic exception (DEC-002; Principles I, IX).

### Phase 2e: Frontend/Design System Review [MANDATORY IF FRONTEND]

- [X] T010 Review `web/src/design-system/docs/INDEX.md`, `DECISION_TREES.md`, `COMMON_MISTAKES.md`, `COMPONENT_PAIRING_GUIDE.md`, and `ACCESSIBILITY_GUIDE.md` against `web/src/components/sessions/SessionCard.tsx` and `web/src/components/consent/ServiceRequirementCard.tsx`; document reuse of existing status/action primitives and semantic tokens, light-theme keyboard, focus, and WCAG AA contrast checks, and why no new universal component is required (FR-008; Principle XI; spec States A–D).

### Phase 2f: E2E Acceptance Test Design [MANDATORY]

All story-labelled acceptance-test tasks were required before foundation implementation. The implementation record shows that this gate was crossed early. T011–T013, T015, and T018 now supply the missing coverage and traceability. This recovery does not establish retroactive test-first compliance.

- [X] T011 Create isolated Reports filesystem/stored and Calendar fixtures in `tests/e2e/fixtures/client_identity.go` and a coherent immutable-target/symlink-publication helper in `tests/e2e/helpers/client_identity.go`; reuse existing mock-provider helpers with both client IDs accepted and observable refresh/code-exchange counts; add the pinned prior-release image constant with its `CLIENT_IDENTITY_PRIOR_RELEASE_IMAGE` override and a testcontainers starter for the AS-1.6 mixed-version variant (research R12); configure files through production bootstrap, with temporary files confined to the permitted temporary directory (AS-1.1–AS-3.4; Principles VIII, XIII). Do not add test-only production APIs or import helpers from production.
- [X] T012 [US1] Write the seven AS-1.1–AS-1.7 primary cases in `tests/e2e/client_identity_change_test.go`, one scenario per `It`, with both change channels, two principals/unknown expiry, all refresh paths/no provider authentication, delayed callback and refresh races (including missing identity/revision pre-upgrade state rejected before exchange), revision-aware rollback, rejected/conflicting PUT, and successful fresh reconnect. T013 supplies supplemental continuous rolling/restart matrices in `tests/e2e/client_identity_rollout_test.go`: simultaneous old/new token, refresh, browser/session, same-ID rotation and metadata operations under fixed identities, incremental old-work drain before identity mutation, and failed-upgrade/pre-activation abort retaining traffic/freeze/evidence. Use the actual prior release, never an enforcement-disabled new binary; assert real HTTP status, token absence, configured service recovery URI, session state, and provider call counts (FR-001–FR-006, FR-013; SC-001, SC-005).
- [X] T013 [US1] Write the AS-1.6 `docker` mixed-version and restart variants in `tests/e2e/client_identity_rollout_test.go`, using T011 fixtures and the testcontainers starter in `tests/e2e/helpers/client_identity.go`. Pin published `ghcr.io/zalando-incubator/agentic-identity-broker:v0.1.147` with the `CLIENT_IDENTITY_PRIOR_RELEASE_IMAGE` mirror override from research R12. Run the actual prior-release container beside the production-bootstrapped new broker against the same expanded PostgreSQL schema, mock provider, and credential directory. Assert old/new token, refresh, browser/session, same-ID rotation and metadata traffic continues with fixed identities; old explicit-column reads/writes preserve evidence and omitted revision inserts default to 0; new handlers reject old callback state before exchange and old handlers consume additive new state only before activation; incremental old-work drain precedes identity changes, failed-upgrade/pre-activation abort retains healthy traffic/freeze/schema/evidence, and subsequent change/restart/rollback never revives legacy sessions. Each supplemental `It` maps only to AS-1.6. Unavailable images and failed startup fail rather than skip or substitute a new binary. Test writing and compilation are complete; execute in container CI via `just test-e2e-docker` when local infrastructure is unavailable. Do not run containers in the agent sandbox. Their first CI run can include implementation; recorded red/green output, logs, and CI links are not required (FR-003–FR-005, FR-013; DEC-005; Principles VIII, XIII; research R12).
- [X] T014 [US2] Write the five AS-2.1–AS-2.5 complete HTTP journeys in `tests/e2e/client_identity_change_test.go`, continuing the shared feature Describe: automatic consent despite a grant, reconnect preserving valid selections and completing original authorization, direct `error_uri` recovery returning to `/sessions` then successful exchange, cancellation/failure/retry with no grant expansion, and list/detail/consent sign-in state (FR-007–FR-010, FR-014; SC-002, SC-003).
- [X] T015 [US3] Write the four AS-3.1–AS-3.4 journeys in `tests/e2e/client_identity_change_test.go`: Calendar/login/grant isolation plus ADR 046-observed-refresh-token-rejection rejected/refreshable controls and alias equality; file secret rotation using the new secret on refresh; optional excluded versus included service; Admin secret/metadata/same-identity source-transition matrix including paused obsolete observers across source/canonical-ID/binding changes followed by a same-ID update with no spurious invalidation (FR-011, FR-012, FR-015; SC-004).
- [X] T016 [US2] Add browser cases for AS-2.1–AS-2.5 in `tests/e2e/frontend/client_identity_reconnect_test.go` using production bootstrap and page objects in `tests/e2e/pages/consent_page.go` and `tests/e2e/pages/sessions_page.go`; cover next-authorization entry, tab-local restored selections, direct recovery, failure/retry, next-state-load updates without unsolicited navigation, and service-specific keyboard-accessible sign-in (FR-007–FR-010, FR-014; Principles XI, XIII).
- [X] T017 [US2] Configure browser captures in `tests/e2e/frontend/client_identity_reconnect_test.go` for the six filenames in `plan.md`: `client_identity_consent_sign_in_required.png`, `client_identity_consent_reconnected.png`, `client_identity_sessions_sign_in_required.png`, `client_identity_sessions_reconnected.png`, `client_identity_reconnect_failed.png`, and `client_identity_reconnect_declined.png`; use the existing opt-in capture and maintained-image flow to `tests/e2e/screenshots/`, not workflow changes (FR-008–FR-010; Principle XIII).
- [X] T018 Verify the 16 primary scenario mappings and supplemental browser/restart variants against `tests/e2e/client_identity_change_test.go`, `tests/e2e/client_identity_rollout_test.go`, and `tests/e2e/frontend/client_identity_reconnect_test.go`; record actual test locations in the current recovery record below. Enforce Describe → Context → It, nearby scenario references, fixtures, isolated lifecycle hooks, separate Admin/end-user servers, no direct DI, fixed sleeps, placeholders, pending/Skip markers, or test-order dependencies (Principle XIII).
- [X] T019 Review the three journeys and four Wireloom SVGs in `specs/056-invalidate-client-sessions/spec.md` and `wireframes/` against approved OpenAPI, synthetic config/HTTP examples, callback state, PKCE, and return-URI safeguards; no output JWT is introduced, so do not invent one. Record discrepancies and approved resolutions. Keep artifact status accurate without silently changing approved behavior (SR-002, SR-003; Principle XIV).
- [X] T020 Run the compiling HTTP and browser acceptance tests in `tests/e2e/client_identity_change_test.go` and `tests/e2e/frontend/client_identity_reconnect_test.go`. Use focused non-container Ginkgo runs and matching `just test-e2e-backend`/`just test-e2e-frontend` recipes. The current recovery record distinguishes written matrices from missing runtime behavior and container execution. Run container-backed cases in CI when local infrastructure is unavailable. No saved red/green output, logs, or CI links are required (Principles VIII, XIII).

### Phase 2g: Observability Design [MANDATORY]

- [X] T070 Review `coding-standards/LOGGING.md` and `coding-standards/OPENTELEMETRY.md` before further logging changes. Verify `specs/056-invalidate-client-sessions/contracts/observability.md` covers the new identity-change event and changed source/refresh failure events. Specify levels, canonical attributes, closed reason/operation/channel sets, and committed-writer attribution. Preserve typed principals, one failure record at the handling layer, and no credentials, client-ID values, or paths. Map required constructor-injected buffer and telemetry safety tests to T035/T040/T064. Verify no new span, metric instrument, namespace, or principal span attribute is required. Resolve design discrepancies before implementation (SR-004; Principles I, XV).

**Checkpoint complete**: All design areas match the supplied approval, including Phase 2g. Acceptance tests and required matrices exist with meaningful assertions. Tests precede further behavior changes. Container-backed tests can first run in CI. Recorded red/green evidence is not required. Migration tests compile with the integration tag and `just check` passes; real PostgreSQL execution remains unverified.

## Phase 2.5: Foundational Infrastructure

- [X] T021 Write red model/copy/JSON and revision-preservation tests in `internal/domain/model/thirdparty_oauth2_provider_test.go` and `internal/domain/storage/user_session_test.go`: `ClientIdentity` is "Null until first recorded. Non-empty when set. Never an Admin API input or output. Not encryption AAD (ADR 008)."; service revision is ">= 0. Changes only by +1 together with ClientIdentity. Never an Admin API input or output."; session revision is ">= 0. Set only when a winning callback creates or replaces the session, from the sealed state. Refresh, source transitions, and observations never rewrite it. Pre-existing rows default to 0." (data-model field rules; FR-004, FR-005, FR-013; Principles VIII, IX).
- [X] T022 Extend `internal/domain/model/thirdparty_oauth2_provider.go` and `internal/domain/storage/user_session.go` with the internal fields and safe copies from T021, and define `ThirdpartyOAuth2ClientIdentityRepository` and the minimal state/observation DTOs directly in `internal/ports/storage.go` (not a conditional re-export), with atomic source/version/canonical-ID/revision preconditions and an explicit applied/no-op versus lost-precondition result; define `SessionUsabilityEvaluator` and modes in `internal/ports/session_usability.go` per `contracts/ports.md` (FR-001–FR-005, FR-013; Principles VI, IX). Change the existing provider repository `Update` result to `(identityChanged bool, err error)` per `contracts/ports.md`, with the update's committed identity/revision on the entity; adapt its callers, mocks, and wrappers without adding persisted/public fields. Public JSON and AAD must not gain identity evidence. Include presence-aware revision types and validation: explicit 0 differs from absence, and negative revisions are invalid. T022 covers models, ports, DTOs, signatures, callers, and type-level state support only. All-variant sealing, callback claim enforcement, and establishment copying belong to T033/T037. T022 does not depend on either task.
- [X] T023 [P] Write red recording/parity tests in `internal/adapters/storage/memory/thirdparty_provider_test.go` and `user_session_test.go`: stored/filesystem creation, fallback to stored client ID when evidence is null, atomic accepted update increments with lock-derived `identityChanged=true`, initialization/same-ID/resulting-filesystem updates return false, conflicting/invalid updates return false with no writes, and the deterministic interleaving from `contracts/ports.md`: pause a filesystem metadata PUT after its initial read, commit a differing observation at unchanged administrative version, then finish the successful PUT; it preserves observed identity/revision and returns `identityChanged=false`; first observation/no increment, same ID/no increment, source/version/canonical-ID/revision preconditions on null/same/differing-ID observations, paused observers losing to filesystem→stored transitions or canonical-ID/binding changes and source round trips at unchanged revision, no writes from lost preconditions and no spurious increment on a later same-ID update, concurrent equal observations/one increment, rollback increments, unchanged version/ETag/updated_at, copy-on-read, locked refresh preservation (FR-001, FR-003–FR-005, FR-012, FR-013; Principle IX).
- [X] T024 [P] Write corresponding PostgreSQL record/mapping unit tests in `internal/adapters/storage/postgres/client_identity_record_test.go`, and integration-tagged behavior/concurrency tests in `internal/adapters/storage/postgres/client_identity_*_integration_test.go` using shared cloned databases and production migrations; assert discovery/version preconditions, explicit-column projections, session-lock/callback-copy behavior, exactly one concurrent increment; atomic source/version/canonical-ID/revision observation preconditions including initialization/no-op; paused filesystem→stored, canonical-ID/binding-change, and same-revision source-round-trip races that discard obsolete observations and preserve later same-ID updates; restart persistence, unchanged admin version/timestamps on observations, and row-lock/transaction-derived `Update.identityChanged` parity with T023 including initialization/no-op/resulting-filesystem/error/rollback results; reproduce T023's paused filesystem metadata PUT/intervening differing-observation race against real PostgreSQL, asserting successful PUT, preserved observation evidence and `identityChanged=false` (FR-001, FR-003–FR-005, FR-013; Principle IX).
- [X] T025 Implement the recording rules and session revision copies in `internal/adapters/storage/memory/thirdparty_provider.go`, `thirdparty_provider_record.go`, and `user_session.go` after T023 fails semantically; derive `Update.identityChanged` and its committed identity/revision under the same memory write lock, preserve latest identity evidence on resulting-filesystem updates, and never infer the outcome from an unlocked read; expose the focused repository through the memory adapter factory/accessor as needed without duplicating repository business logic (FR-001, FR-003–FR-005, FR-012, FR-013; Principles VI, IX).
- [X] T026 [P] Implement equivalent sqlx mappings and conditional recording in `internal/adapters/storage/postgres/thirdparty_provider.go`, `thirdparty_provider_record.go`, and `user_session.go` after T024's locally runnable tests fail semantically; extend every service/session projection and write with explicit columns, preserve preconditions/locks/timeouts, derive `Update.identityChanged` and returned identity/revision from the locked predecessor in the same row-lock/transaction as the conditional update (no unlocked snapshot comparison), preserve latest evidence on resulting-filesystem updates, return successful outcomes only after commit and false on errors/rollback, wrap errors in domain StorageError, and expose the focused adapter interface (FR-001, FR-003–FR-005, FR-012, FR-013; Principles VI, IX). Real PostgreSQL execution can first occur in container CI. Stored red/green evidence is not required.
- [X] T027 Write table-driven red tests in `internal/domain/oauth2session/client_identity_test.go` for identity/revision match, mismatch, superseded, legacy stored never-transitioned R=0, legacy rejection after first change/transition/filesystem selection, rollback, first-file observation and CAS retry that reloads service/source/version/canonical ID/revision, reselects binding and reads anew instead of replaying obsolete values (including stored-source and canonical-ID/binding changes), stale replica observations within valid preconditions, Gate versus Display source failures, and the cross-product with ADR 046 recorded rejection/expiry/refreshability; assert no secret decryption on identity reads (FR-001–FR-005, FR-013, FR-015; SR-001; DEC-003, DEC-004).
- [X] T028 Implement `resolveClientIdentity`, `sessionIdentityStatus`, and evaluator Gate/Display modes in `internal/domain/oauth2session/client_identity.go`; read current identity without secret decryption through `internal/domain/thirdparty/service.go`, observe filesystem IDs with bounded `ReadClientID` and atomic source/version/canonical-ID/revision CAS; on lost preconditions discard the value, reload the service/source/binding and file, or use stored identity if now stored (no stale-value replay), preserve legacy exception only at R=0, compose ADR 046's predicate, and use persisted evidence only for non-token display fallback with a source-failure event (FR-001–FR-005, FR-013, FR-015; SR-001; DEC-003, DEC-004).
- [X] T029 Adapt metadata-only storage session summaries in `internal/domain/storage/user_session.go`, `internal/ports/storage.go`, and both `internal/adapters/storage/{memory,postgres}/user_session.go` so list/detail identity evaluation has internal `UpstreamClientID` and revision without encrypted token blobs or secret decryption; use an internal metadata projection/helper rather than forcing the full-session evaluator to load tokens; extend red/green copy/projection/JSON tests in the adjacent `_test.go` files (FR-008, FR-015; SR-004; Principles VI, VIII, IX; contracts/ports.md read models).
- [X] T030 Write red dependency-injection tests in `internal/app/builder_test.go`, then wire the identity repository and usability evaluator in `internal/app/builder.go` for session, authorization, consent, and third-party services; update test mocks and thin production-bootstrap wrappers only as required, without handler construction or domain imports across adapter boundaries (FR-001, FR-007, FR-008; Principles VI, VIII, XII).

**Checkpoint**: Both stores expose identical revision semantics; one tested domain rule serves all consumers. No new bounded context, dependency, scheduler, distributed lock, or credential cache.

## Phase 3: User Story 1 — Stop Using Connections Established Under the Previous Client ID (Priority: P1) 🎯 MVP

**Goal**: Both filesystem and Admin changes stop all previous-identity token release and refresh; delayed work, rollback, and restart cannot restore usability.

**Independent Test**: Connect Alice and Bob under v1, change to v2 through each channel, deny still-valid/unknown-expiry tokens and every refresh path, then reconnect only Alice. Bob must remain denied. Restore v1 and prove no old session revives.

### Tests for User Story 1 [MANDATORY - Principle VIII]

Primary E2E tasks: T012 and T020. T013 owns supplemental AS-1.6 prior-release compatibility/restart variants. Their tests must exist before further behavior changes, but their first execution can occur in container CI.

- [X] T031 [P] [US1] Add red token fast-path tests in `internal/domain/oauth2session/credential_identity_test.go` and `client_identity_test.go`: unexpired/unknown-expiry/expired tokens, all service variants (stored confidential, public, Google, CIMD public/confidential, DCR, filesystem), legacy R=0/change, and unreadable/missing identity; deny before decrypt/release with correct safe reasons, preserving ADR 046-observed-refresh-token-rejection rejection precedence and no background submission on identity denial (AS-1.1, AS-1.2, AS-1.5–AS-1.7; FR-003, FR-015; SR-001). Add deterministic call-count tests in `client_identity_test.go` and `client_identity_observation_test.go` for the plan's uncontended read goal, eight-attempt observation bound, Gate exhaustion, Display fallback, and one evaluation per service per batch. Reuse existing coverage where it already proves these limits.
- [X] T032 [P] [US1] Add red refresh/overlap tests in `internal/domain/oauth2session/service_refresh_concurrency_test.go`, `background_refresh_test.go`, and `sweep_test.go` covering identity check under lock, in-flight change/reconnect ordering, deletion, no provider call for superseded sessions, no token/revision rewrite, callback generation precedence, changed sweep counts (one due identity-invalid candidate gives failed=1, refreshed=0 for real and dry runs), and dry-run classification with zero observation/session writes (AS-1.3, AS-1.4; FR-004, FR-005; ADR 038).
- [X] T033 [P] [US1] Add red initiation/callback/JWE tests in `internal/domain/oauth2session/state_token_test.go`, `state_token_security_test.go`, and `service_reconnect_regression_test.go`: every new variant carries explicit `upstream_client_id` and presence-aware `upstream_client_revision` (>= 0); previous revision including rollback fails before code exchange; absent revision is distinct from explicit 0, and pre-upgrade state missing identity or revision for every variant is rejected with identity_missing before provider calls without changing the existing session; delayed results never replace a newer reconnect; failed callbacks preserve the old row and successful current callbacks clear rejection through existing behavior (AS-1.4–AS-1.6; FR-003–FR-005, FR-015; SR-002).
- [X] T034 [P] [US1] Add red error-mapping/HTTP tests in `internal/domain/tokenexchange/session_diagnostic_test.go` and `internal/adapters/http/oauth2_sessions/handler_test.go`: all three identity reasons give recoverable `invalid_grant` and resolved-service/public-origin URI, no token/WWW-Authenticate; unreadable source gives server_error with no URI; manual refresh uses `409 refresh_unavailable`, preserving `409 refresh_rejected` and infrastructure 500 (FR-006, FR-015; SR-003; API-001).
- [X] T035 [P] [US1] Add red Admin boundary/audit tests in `internal/adapters/http/handlers/admin/services_handler_test.go` and `internal/domain/thirdparty/service_test.go` for accepted client-ID updates, rejection/conflicts, no file reads on metadata writes, ignored/unbound internal identity inputs, no internal-field response leak, and exactly one committed change event. Add a deterministic barrier race that pauses a filesystem metadata PUT after the initial service read, commits a differing filesystem observation without changing the administrative version, then completes the PUT successfully: preserve the observation's identity/revision, return `identityChanged=false`, and assert only the observation emits `channel=filesystem_observation` at its committed revision, with no admin identity-change event (AS-1.7, AS-3.4; FR-003, FR-012; SR-004; API-002).

### Implementation for User Story 1

- [X] T036 [US1] Enforce identity before every access-token return/decryption/background submission in `internal/domain/oauth2session/service.go` and maintain ADR 046 rejection checks; retain immediate valid-token return only after identity succeeds, without waiting for refresh (AS-1.1, AS-1.2, AS-1.5–AS-1.7; FR-002, FR-015; ADR-001).
- [X] T037 [US1] Seal/validate/copy presence-aware `{client ID, revision}` for all new flows in `internal/domain/oauth2session/credentials.go`, `state_token.go`, and `service.go`; require both claims and compare sealed current identity before code exchange (legacy missing revision is not 0 and must restart sign-in), and retain callback-ID/session-lock precedence so stale work cannot win or rewrite establishment evidence (AS-1.4; FR-003–FR-005, FR-009, FR-013; SR-002).
- [X] T038 [US1] Add identity enforcement inside locked refresh in `internal/domain/oauth2session/service.go` and candidate classification in `sweep.go`; cover on-demand/manual/background/sweep through the shared rule, preserve rejection exclusion, count superseded candidates as failed, and ensure dry-run comparison never persists an observation (AS-1.3, AS-1.4; FR-004, FR-015; API-002).
- [X] T039 [US1] Add `identity_superseded` to `internal/domain/model/credential_source_error.go`, map it in `internal/domain/tokenexchange/session_diagnostic.go`, and map missing/mismatch/superseded identity to `409 refresh_unavailable` in `internal/adapters/http/oauth2_sessions/handler.go`, keeping safe infrastructure failures and ADR 044 recovery construction unchanged (FR-006; API-001; SR-001, SR-003).
- [X] T040 [US1] Implement committed-update/observation audit events in `internal/domain/thirdparty/service.go` and `internal/domain/oauth2session/client_identity.go` per `specs/056-invalidate-client-sessions/contracts/observability.md`; emit `admin_update` only from a successful atomic `Update.identityChanged=true` using that update's committed revision, and `filesystem_observation` only from successful `applied && changed`, never from comparing a pre-read or follow-up read. Turn T035's paused metadata PUT/intervening observation audit test green with only the observation's event; extend tests in adjacent `_test.go` files for one event per winning increment, channel/revision/service metadata, typed admin operator principal, safe denied-use reason/operation,…
- [X] T041 [US1] Turn T031–T035 and AS-1.1–AS-1.7 green. Run `just check`, `just test`, and `just test-e2e-backend`. Preserve assertions and distinguish local checks from unexecuted container validation. No stored red/green output, logs, or CI links are required (SC-001, SC-005; Principles VIII, XIII).

**Checkpoint**: Token safety is independently demonstrable (technical MVP). US2 is also P1 and remains required for delivery; no partial MVP is declared feature completion.

## Phase 4: User Story 2 — Be Guided Through Consent and Reconnect (Priority: P1)

**Goal**: Next relevant browser authorization shows consent, every display offers sign-in, and existing reconnect flows restore only the owning user's service connection.

**Independent Test**: Begin with an existing valid delegation and unusable Reports session, enter agent authorization, reconnect from consent with selections intact, finish original authorization, then exercise direct error-URI recovery and failure/retry without grant expansion.

### Tests for User Story 2 [MANDATORY - Principle VIII]

Primary E2E/browser red tasks: T014, T016, T017, T020.

- [X] T042 [P] [US2] Add red evaluator-consumer tests in `internal/domain/oauth2/service_test.go` and `internal/domain/consent/service_test.go`: included unusable session forces next-authorization consent in local/hybrid modes; mandatory submission fails; detail renders not_connected; gating read errors fail closed while display uses persisted fallback; valid contexts/selections, grant expiry, optional inclusion and ADR 046 refreshability are preserved (AS-2.1, AS-2.2, AS-2.5; FR-007–FR-009, FR-014, FR-015).
- [X] T043 [P] [US2] Add red list/detail/manual-refresh-summary tests in `internal/domain/oauth2session/user_session_summary_test.go` and `session_details_security_test.go` proving unusable identity sets requires_reauthentication and its equal deprecated alias, access_token_expired remains independent, internal evidence stays hidden, and display failures do not release tokens or destroy sessions (AS-2.5; FR-008, FR-015; SR-004).
- [X] T044 [P] [US2] Extend red tests in `web/src/components/sessions/SessionCard.test.tsx`, `web/src/components/consent/ServiceRequirementCard.test.tsx`, and `web/src/hooks/useSessions.test.ts` for backend-driven sign-in state despite valid expiry, Login versus refresh affordances, retained Terminate action, callback refresh of connection state, failure/retry, and keyboard-accessible existing actions; mock API sources, not hooks (AS-2.1–AS-2.5; FR-008–FR-010; Principle XI).

### Implementation for User Story 2

- [X] T045 [US2] Integrate the Gate evaluator with `anyRequiredSessionUnusable` in `internal/domain/oauth2/service.go` and mandatory validation in `internal/domain/consent/service.go`; add identity to, never replace, existing rejection/missing-session checks and use the existing consent redirect only on relevant authorization (FR-007, FR-011, FR-014, FR-015; SR-002).
- [X] T046 [US2] Integrate Display mode into consent detail in `internal/domain/consent/service.go` and session list/detail/refresh summaries in `internal/domain/oauth2session/service.go`; read only the live client-ID file for usability (Accepted ADR 048-client-identity-revisions supersedes ADR 041-oauth2-client-secret-file-overrides's no-file-read rule on these displays only), retain no-secret/no-token-blob storage metadata reads and administrative service-metadata no-file-read tests, truthful identity state, equal requires_reauthentication/is_expired, distinct access expiry, and fallback logging (FR-008, FR-015; DEC-003).
- [X] T047 [US2] Align `web/src/services/api/sessions.ts` and `web/src/components/sessions/SessionCard.tsx` with the existing required reauthentication field and sign-in action rather than expiry-only decisions; reuse the current consent not_connected presentation in `web/src/components/consent/ServiceRequirementCard.tsx` and tab-local return state without adding an enum, navigation loop, or live redirect (FR-008–FR-010, FR-014; Principle XI).
- [X] T048 [US2] Verify and minimally repair the existing provider initiation/callback and refresh-after-return integration in `internal/domain/oauth2session/service.go` and `web/src/pages/ThirdPartySessionsPage.tsx` only where T014/T016 show a gap: consent reconnect preserves valid selections, direct recovery returns to `/sessions`, cancellation/failure never revives old tokens or broadens grants and leaves a retry; retain state/principal/PKCE/same-origin checks (AS-2.2–AS-2.4; FR-009, FR-010; SR-002, SR-003).
- [X] T049 [US2] Run `just check`, `just test`, `just web-test`, `just test-e2e-backend`, and `just test-e2e-frontend` until AS-2.1–AS-2.5 and their unit/browser tests pass; capture the six maintained screenshots listed in `plan.md`, including the separate failure/retry and cancellation/retry states, through opt-in serial capture described in `specs/056-invalidate-client-sessions/quickstart.md`, and keep assertions unchanged apart from fixture corrections (SC-002, SC-003; Principles VIII, XIII).

**Checkpoint**: Both P1 stories work end to end, including recovery; P2 isolation still gates full feature completion.

## Phase 5: User Story 3 — Keep Unaffected Access Working (Priority: P2)

**Goal**: No global logout, delegation reset, unrelated-service change, or reconnection from same-ID secret rotation/metadata/source transition.

**Independent Test**: Change only Reports while Calendar and grants retain their prior outcomes, rotate secrets through both channels with no revision increment, and prove excluded optional services do not force consent.

### Tests for User Story 3 [MANDATORY - Principle VIII]

Primary acceptance red task: T015 before foundation. Later unit tests expose any remaining isolation gaps before their fixes.

- [X] T050 [P] [US3] Extend red credential-rotation/source-transition tests in `internal/domain/oauth2session/credential_rotation_test.go` and `internal/domain/thirdparty/service_test.go` to assert unchanged identity revision for same-ID file/Admin rotation and metadata updates, refreshed authentication using the new secret, provider credential rejection not mislabelled as identity recovery, and existing validation retained across source transitions (AS-3.2, AS-3.4; FR-012; SC-004).
- [X] T051 [P] [US3] Extend red isolation/optional-service controls in `internal/domain/oauth2/service_test.go` and `internal/domain/consent/service_test.go`: only included services participate; unrelated principal/session/grant/scopes/validity unchanged; preserve Calendar rejected/refreshable ADR 046 outcomes, deprecated alias equality, and background/sweep rejection exclusion (AS-3.1, AS-3.3; FR-011, FR-015).

### Implementation for User Story 3

- [X] T052 [US3] Make only test-driven isolation/rotation corrections in `internal/domain/oauth2session/credentials.go`, `internal/domain/thirdparty/service.go`, and both `internal/adapters/storage/{memory,postgres}/thirdparty_provider.go`: increment only on an effective differing identity, no increment for first observation or same-ID rotation/metadata/transition, never infer history, and preserve explicit source-transition validation (AS-3.2, AS-3.4; FR-012, FR-013).
- [X] T053 [US3] Make only test-driven inclusion corrections in `internal/domain/oauth2/service.go` and `internal/domain/consent/service.go` so an excluded optional service never causes consent while an included changed service does; do not mutate unrelated sessions, broker authentication, scopes, grant validity, or ADR 046 outcomes (AS-3.1, AS-3.3; FR-011, FR-015).
- [X] T054 [US3] Run `just check`, `just test`, and `just test-e2e-backend` until AS-3.1–AS-3.4 and rotation/isolation matrices pass alongside US1/US2; record zero unintended invalidation and no extra revision/change event for routine rotations in `specs/056-invalidate-client-sessions/tasks.md` (SC-004; Principles VIII, XIII).

## 🔒 Phase N: Constitution Compliance Verification [MANDATORY COMPLIANCE SECTION]

**Purpose**: Verify constitution and feature-scope compliance. All required stories must be complete, not only MVP. Keep artifact status accurate. Further behavior changes require approval, not silent specification edits. Recorded red/green output, logs, and CI links are not completion requirements.

### 🔒 Constitution Compliance Verification [MANDATORY]

#### Design Phase Verification [MANDATORY]

- [X] T055 Verify every task traces to FR-001–FR-015, SR-001–SR-005, AS-1.1–AS-3.4 or an applicable binding principle; exclude unrelated remediation, and record only explicitly user-approved future-issue URLs as non-checkbox future work in `specs/056-invalidate-client-sessions/tasks.md` (Development Requirements: Feature Scope & Deferred Findings).
- [X] T056 Verify T002–T010 and T070 preconditions against `ARCHITECTURE.md`, `examples/config/third-party-oauth2.yaml`, `examples/config/README.md`, `docs/configuration.md`, `charts/agentic-identity-broker/README.md`, `api/enduser/openapi.yaml`, `api/admin/openapi.yaml`, `migrations/049_add_client_identity_revision.up.sql`, `migrations/050_validate_client_identity_revision.up.sql`, and the approved ADR; confirm supplied written API/decision approval, no new config defaults or Helm settings, and completed design-system review (Principles II, IV, V, VII, IX, X, XI, XV).
- [X] T057 Verify all three concrete journeys in `specs/056-invalidate-client-sessions/spec.md` and four SVGs in `wireframes/` still match implemented configuration, HTTP, recovery and browser behavior; reconcile implementation to approved artifacts, not by silently changing spec/plan; retain synthetic values and no invented output JWT (Principle XIV).
- [X] T058 Verify T012–T020 test coverage and scenario mapping against `tests/e2e/client_identity_change_test.go` and `tests/e2e/frontend/client_identity_reconnect_test.go`. Each primary scenario has one `It`. Supplemental variants each reference one scenario. Verify meaningful assertions, isolated fixtures, and no placeholders, pending/Skip markers, direct DI, order dependence, or weakened assertions. Verify the recovery checkpoint, not retroactive red-run evidence (Principles VIII, XIII).

#### Implementation Phase Verification [MANDATORY]

**API & Documentation** (Principles IV, X):

- [X] T059 [P] Update `docs/api/` and `docs/reference/token-exchange.md` with approved identity-related reauthentication behavior and concrete token/reconnect examples; verify implemented statuses/error bodies, existing fields/alias, no new endpoint, and both API major cutovers (End-User 2.0.0, Admin 3.0.0) against `api/enduser/openapi.yaml` and `api/admin/openapi.yaml` (FR-006–FR-010, FR-015; Principles IV, X).
- [X] T060 [P] Complete operator guidance in `docs/guides/manage-agents-and-services.md` and `docs/configuration.md`: both change channels, routine same-ID secret rotation, multi-replica publication/extra-reconnect risk, retained sessions, legacy R=0 policy, pre-upgrade callback sign-in retry on new replicas, and the continuous rolling/rollback boundary from ADR 048, consistent with T005. Include a checklist proving identity/source/canonical-ID/binding freeze across Admin/filesystem/DCR/direct writers, continued traffic and safe routine rotation during mixed-version overlap, online schema compatibility, completed old-work drains, and pinned revision-aware inventory/no-old-image restart before resuming identity changes. Failed checks retain healthy traffic/freeze; prior-binary abort is only pre-activation with unchanged identities, zero revisions, drained new work, and evidence retained. No unaware binary may resume serving/writing after activation (FR-003, FR-005, FR-012, FR-013; DEC-002–DEC-005; Principles II, VII, IX).

**Architecture & Documentation** (Principle II):

- [X] T061 Verify `ARCHITECTURE.md`, `AGENTS.md`, and `adrs/048-client-identity-revisions.md` match the approved implementation and cite the actual maintainer sign-off; retain Proposed status for any unapproved decision and do not edit merged ADRs 038/041/046 (Principles II, V).

**Configuration** (Principle VII):

- [X] T062 Verify all identity-file/public-origin use still comes through `internal/ports/config.go` and builder-injected configuration, existing `examples/config/` keys remain unchanged, and `charts/agentic-identity-broker/` has only required migration/procedure documentation, with no switch weakening enforcement (FR-003; SR-001, SR-003; Principle VII).

**Database & Persistence** (Principle IX):

- [X] T063 Verify both stores and explicit-column queries in `internal/adapters/storage/memory/` and `internal/adapters/storage/postgres/` preserve revision, copy-on-read, callback-only writes, rejected-session predicates, preconditions, StorageError wrapping and timeouts; confirm migrations 049/050 numbering, guard, atomicity and real PostgreSQL integration coverage in `tests/integration/migrations/client_identity_revision_migration_test.go` and `internal/adapters/storage/postgres/client_identity_*_integration_test.go` (FR-004, FR-005, FR-013, FR-015; Principle IX). Record `just test-integration-infra` and `just test-e2e-docker` as container-CI validation, never as locally passed/deferred implementation prerequisites.

**Security** (Principles I, III):

- [X] T064 Verify fail-closed token/refresh paths, no availability fallback or legacy backfill, callback claim presence/generation/PKCE/state/principal/same-origin protections, obsolete-observation fencing and fresh-read retry, continuous rolling upgrade with fixed identities during mixed-version traffic, verified old-work drain before identity mutations resume, guarded pre-activation abort and revision-aware-only post-activation rollback procedures, and unchanged crypto/AAD in `internal/domain/oauth2session/`, `internal/domain/tokenexchange/`, and `internal/domain/consent/`; test logs/events against `contracts/observability.md` for credential/path/client-ID-value absence and typed admin principals, with no new span/instrument or sensitive attributes (SR-001–SR-005; Principles I, III).

**Architecture Patterns** (Principles VI, XII):

- [X] T065 Verify ports-only domain dependencies, no new domain package or adapter cross-import, thin HTTP mapping, narrow identity repository, and builder-only DI in `internal/ports/`, `internal/domain/`, `internal/adapters/`, and `internal/app/builder.go`; no handler-to-repository bypass or test-only production APIs (Principles VI, IX, XII).

**Testing** (Principle VIII - Unit & Integration Tests):

- [X] T066 Verify meaningful happy-path, error, edge, and concurrency tests in `internal/domain/oauth2session/`, both storage adapters, and consumers. Preserve assertions through refactoring. Do not use Bash correctness tests or hide unchecked work behind a passing subset. Run `just check`, `just test`, `just web-test`, and `just test-integration` successfully. Do not require stored red/green output, logs, or CI links (Principle VIII).

**E2E Acceptance Testing** (Principle XIII):

- [X] T067 Run the complete non-container acceptance gate with `just test-e2e-backend` and `just test-e2e-frontend`; verify AS-1.1–AS-3.4 green, full user journeys, production bootstrap, scenario comments, isolated fixtures, minimal assertion changes, and all six UI artifacts listed in `plan.md` captured in `tests/e2e/screenshots/` using opt-in serial screenshot capture, including `client_identity_reconnect_declined.png` for cancellation/retry; run container restart variants separately with `just test-e2e-docker` in container CI (Principle XIII; SC-001–SC-005).

**Frontend** (Principle XI - if applicable):

- [X] T068 Verify the changed sign-in states in `web/src/components/sessions/SessionCard.tsx` and `web/src/components/consent/ServiceRequirementCard.tsx` use design-system primitives/semantic tokens, preserve keyboard actions and visible focus, work in the single light theme with WCAG AA text 4.5:1/UI 3:1 contrast, and introduce no custom CSS or unnecessary universal component; if a reusable primitive is actually required, cover it with existing Storybook/accessibility conventions (FR-008; Principle XI).

**Final validation** (Principles VIII, XIII):

- [X] T069 Run every non-container recipe of the full `just verify` gate: `just check`, `just security`, `just test`, `just web-test`, `just cdk-test`, `just mock-sample-agent-test`, `just mock-upstream-oauth2-test`, `just test-integration`, `just test-e2e-backend`, and `just test-e2e-frontend`. Verify current ExtProc recipe/label boundaries before its non-container run. Fix change-caused failures without weakening tests or altering CI. Do not invoke container-backed recipes in the sandbox. Run `just test-integration-infra`, `just test-e2e-docker`, and container-backed ExtProc cases in container CI. Report validation limits honestly. Stored red/green output, logs, and CI links are not required (Principles VIII, XIII). **Execution complete, not a passing gate:** baseline security failures and unexecuted container checks remain explicitly recorded below.

## Dependencies & Execution Order

### Phase Dependencies

```text
T001 approval record
  → Phase 2a–2g design (including T070) + T011 fixtures
  → T012 → T013 → T014 → T015 (primary backend scenarios stay in one file; supplemental T013 has its own docker file and can first run in container CI)
  → T016 → T017 (same frontend file)
  → T018–T020 scenario mapping, journeys, and acceptance tests
  → T021–T030 foundation (tests → models/ports → both adapters → rule → DI)
  → US1 T031–T041
  → US2 T042–T049
  → US3 T050–T054
  → Phase N T055–T069
  → Phase 6 T071–T075
  → Phase 7 T076 → T077 → T078
  → Phase 8 T079 (accepted without change) → T080
```

Acceptance recovery, foundation, and observability design are complete. T022 never depended on T033/T037. Historical completion does not imply that the original test-first order was followed.


The existing approval covers the listed API and decision items. Missing test coverage is not a new approval requirement. Unapproved changes still require explicit approval before implementation.

### User Story Dependencies

- **US1 (P1)** depends on design and foundation; independently validates broker token safety. Tests T012/T031–T035 precede implementation T036–T040.
- **US2 (P1)** shares the identity rule and depends on US1's establishment/recovery/error paths for complete reconnect journeys. Consumer tests T042–T044 can be prepared after foundation; integrate after US1. P1 recovery is not optional.
- **US3 (P2)** validates and closes isolation/rotation gaps across the shared implementation; unit tests T050/T051 can be prepared after foundation, but final assertions require US1/US2 integration. AS-3 acceptance tests already exist before implementation.
- Same-file tasks run sequentially even across stories. Within any test task, observe semantic failure before its corresponding implementation; missing API scaffolding may be added solely to compile.
- T008/T013/T024 container execution is independent of the local critical path. Required test writing is not deferred. Container-backed suites can first run in CI with implementation included. Do not require recorded red/green evidence or claim local validation without execution.

### Parallel Opportunities and Examples

`[P]` means distinct files and no dependency on another incomplete task in the same execution batch; listed prerequisites still apply. Do not run a test task concurrently with implementation of the behavior it is meant to prove red.

- **Foundation**: after T022, T023 (memory tests) and T024 (PostgreSQL tests) are independent. Tests precede corresponding adapter changes. T027/T028 depend on port/mapping scaffolding. T033/T037 depend on foundation, never the reverse.
- **US1**: after foundation, prepare T031 fast-path tests, T032 maintenance/overlap tests, T033 callback/JWE tests, T034 mapping tests, and T035 Admin tests in parallel; T031 owns `client_identity_test.go`, so do not overlap T027. Then sequence T036–T038 because they share `service.go`.
- **US2**: after foundation, T042 authorization/consent, T043 session read models, and T044 frontend tests are independent; T045/T046 share consent code and run sequentially. Do not overlap T042 with US3 T051 in the same test files.
- **US3**: T050 credential rotation/third-party tests and T051 authorization/consent tests can run together after earlier same-file tasks finish. T052 then T053 fix only the failures they expose.
- **Finishing**: T059 API consumer docs and T060 operator docs are independent after behavior stabilizes. Design/API documentation T007 can run alongside T006 once approval is confirmed.

## Acceptance and Requirement Coverage

Every primary row maps to a separate `It` in `tests/e2e/client_identity_change_test.go`; variants stay within their scenario. Supplemental browser/restart cases must each cite one scenario, not combine multiple acceptance criteria into an untraceable test.

| Scenario | Primary red task | Implementation / validation | Supplemental coverage |
|---|---|---|---|
| AS-1.1 | T012 | T028, T036, T039–T041 | T031 filesystem, safe errors |
| AS-1.2 | T012 | T036, T037, T041 | T031 unknown expiry, principal independence |
| AS-1.3 | T012 | T038, T041 | T032 background/manual/sweep/dry-run |
| AS-1.4 | T012 | T037, T038, T041 | T032/T033 overlapping callback and refresh |
| AS-1.5 | T012 | T025–T028, T036, T041 | T027/T031 rollback |
| AS-1.6 | T012 | T025–T028, T036, T037, T041 | T005/T060 continuous rolling/fixed-identity overlap, old-work drain, abort/rollback and legacy callback retry; T008/T024 persistence; T013 actual-prior-release docker mixed-version/schema and restart variants |
| AS-1.7 | T012 | T025, T026, T035, T036, T040, T041 | T023/T024 atomic committed-update outcome and accepted versus rejected/conflicting PUT; T035/T040 observation-only audit attribution during paused filesystem metadata PUT |
| AS-2.1 | T014 | T045, T049 | T016/T017 browser State A |
| AS-2.2 | T014 | T037, T047–T049 | T016/T017 browser State B |
| AS-2.3 | T014 | T039, T048, T049 | T016/T017 browser State D |
| AS-2.4 | T014 | T037, T048, T049 | T016/T017 browser failure/retry capture |
| AS-2.5 | T014 | T029, T046, T047, T049 | T016/T017 browser State C; T043/T044 |
| AS-3.1 | T015 | T051, T053, T054 | ADR 046 control matrix/alias, T027/T032 |
| AS-3.2 | T015 | T050, T052, T054 | New secret used without revision change |
| AS-3.3 | T015 | T051, T053, T054 | Included versus excluded optional service |
| AS-3.4 | T015 | T050, T052, T054 | T023/T024/T027 both-store source/configuration-generation CAS and fresh-read retry, no spurious later same-ID invalidation |

| Requirement / outcome | Tasks |
|---|---|
| FR-001, FR-002, SC-001 | T021–T028, T031, T036, T041 |
| FR-003 | T005, T007, T008, T011–T013, T022–T026, T031, T033, T035, T037, T060, T064 |
| FR-004, FR-005, SC-005 | T008, T009, T023–T028, T032, T033, T037, T038 |
| FR-006 | T006, T034, T039, T059 |
| FR-007, FR-014, SC-002 | T014, T016, T042, T045, T049 |
| FR-008 | T010, T014, T016, T017, T029, T043, T044, T046, T047, T068 |
| FR-009, FR-010, SC-003 | T014, T016, T017, T033, T037, T044, T048, T049 |
| FR-011, FR-012, SC-004 | T015, T023–T026, T050–T054 |
| FR-013 | T005, T008, T009, T012, T013, T021–T028, T031, T037, T060, T064 |
| FR-015 | T006, T015, T027, T031–T034, T038, T039, T042, T043, T045, T046, T051, T053 |
| SR-001 | T027, T028, T031, T034, T036, T038, T064 |
| SR-002, SR-003 | T019, T033, T034, T037, T042, T045, T048, T064 |
| SR-004 | T029, T035, T040, T043, T064 |
| SR-005 | Scope boundary, T055, T064 |

## Implementation Strategy

1. Record real approval, apply approved contracts and migration design, and establish acceptance red before behavior changes.
2. Implement shared models/ports and both stores under TDD, then the single identity/evaluator rule and DI.
3. Deliver US1 as the independently testable token-safety MVP, then US2 recovery (also P1), then US3 isolation; all three are required before claiming completion.
4. Commit logical increments locally with the required co-author trailer. Preserve separate behavior-neutral refactoring commits if needed. No separate PR, direct git push, workflow edits, or readiness/merge action.
5. Complete Phase N. Distinguish local validation from container-CI validation. Keep incomplete work unchecked. Stored red/green output, logs, and CI links are not required.

## Implementation Evidence

### 2026-11-05 — incomplete implementation increment

**Outcome: blocked / partial progress, not a delivered feature.** Eleven design/test-writing tasks are complete. Runtime behavior is unchanged: no model, repository, evaluator, DI, handler, or SPA enforcement has been implemented. The mandatory test-first checkpoint is not complete. All remaining tasks stay unchecked. The approved `spec.md` and `plan.md` are unchanged.

#### Approval and design evidence

- T001: supplied approval covers all eight listed items (API-001, API-002, ADR-001, DEC-001–DEC-005), approved by **jbspeakr on 2026-11-05** at https://github.com/zalando-incubator/agentic-identity-broker/pull/412#issuecomment-6658788612. ADR 048's Status records that approval in `adrs/048-client-identity-revisions.md:3`. The feature-local reference page and approved-artifact links use the new number after rebase; it is not a second decision, and the supplied approval is unchanged.
- T002: reconciled the model and ports with the approved ADR: exact identifiers, per-service monotonic revision, all-variant callback-only establishment, presence-aware explicit state revision, legacy stored/never-transitioned/R=0 only, no historical backfill, and ADR 046 predicate composition agree. No new entity ID is required. Updated the ADR reference in the model and internal contract.
- T003: updated architecture ownership, lifecycle, glossary, both API cutovers, atomic operation-local audit attribution, and guarded rollout/rollback. `AGENTS.md` adds only the ADR index entry and two feature terms.
- T004/T005: existing credential-file and public-origin settings suffice; no opt-out, new defaults, Helm settings, templates, workflows, or hooks. Configuration examples link to the procedure. Chart guidance covers online 049/050 expansion/validation, coherent publication, continued mixed-version traffic under frozen identities/sources/canonical IDs/bindings, paused identity-mutating writers/registrations, old-work drain, revision-aware inventory, failed-check freeze, guarded pre-activation abort, and revision-aware-only post-activation rollback. The historical 040/041 rollback section now explicitly defers to the stronger boundary after activation.
- T006/T007: applied the approved End-User 2.0.0 and Admin 3.0.0 contract deltas before behavior implementation. Kept routes, shapes, status/error vocabulary, alias inheritance, request validation, and internal-field non-binding unchanged. Added identity-invalid real/dry-run sweep examples and an unexpired-but-reauthentication-required session example. These are approved target contracts; passing runtime conformance is still required.
- T009: wrote both migration pairs after the migration tests compiled; they are now numbered 049/050 after rebase reconciliation. 049 uses metadata-only revision defaults, nullable identity, NOT VALID checks, one-second timeout, NOWAIT locks, and a guarded atomic DOWN. 050 validates separately and its DOWN restores unvalidated constraints. No backfill, token rewrite, or deletion. Container execution is deferred to CI, not reported as passed.
- T010: design review selects the existing Card, Badge, StatusIndicator, Stack, and secondary Login Button. SessionCard already consumes `requires_reauthentication`; consent already consumes `not_connected`. Reuse their semantic variants and focus behavior, retain Terminate and separate expiry information, and change only feature-required status copy under T047. No new universal component or CSS is needed. T016/T068 must exercise Tab/Shift+Tab and Enter/Space, visible focus, the single light theme, text contrast >=4.5:1 and UI contrast >=3:1. This is a completed design review, not a claim that the new browser states have passed accessibility checks.
- T019: reviewed all three concrete journeys and four SVG states against the approved contract: configured origin/resolved service recovery, same-origin return URI, principal-bound state, S256 PKCE, tab-local consent state, no unsolicited navigation, preserved grant selections, and no new output JWT. The SVGs are behavioral sketches; existing design-system primitives govern final presentation. No requirement or approved decision was changed.

#### Acceptance tests written before runtime implementation

`tests/e2e/client_identity_change_test.go` contains one primary `It` for each of the 16 scenarios. Helpers reuse the production credential journey, separate Admin/end-user servers, real provider authorization/callbacks, signed token-exchange credentials, and immutable-target/symlink pair publication. Reports and Calendar fixtures have distinct resources. Fixture corrections removed the unrelated placeholder service from grants and avoid warming the permission-set cache before Calendar setup.

| Scenario | Historical primary test location (2026-11-05; current locations are in the acceptance recovery record below) |
|---|---|
| AS-1.1 | `tests/e2e/client_identity_change_test.go:193` |
| AS-1.2 | `tests/e2e/client_identity_change_test.go:204` |
| AS-1.3 | `tests/e2e/client_identity_change_test.go:219` |
| AS-1.4 | `tests/e2e/client_identity_change_test.go:256` |
| AS-1.5 | `tests/e2e/client_identity_change_test.go:276` |
| AS-1.6 | `tests/e2e/client_identity_change_test.go:556` |
| AS-1.7 | `tests/e2e/client_identity_change_test.go:573` |
| AS-2.1 | `tests/e2e/client_identity_change_test.go:286` |
| AS-2.2 | `tests/e2e/client_identity_change_test.go:298` |
| AS-2.3 | `tests/e2e/client_identity_change_test.go:347` |
| AS-2.4 | `tests/e2e/client_identity_change_test.go:370` |
| AS-2.5 | `tests/e2e/client_identity_change_test.go:412` |
| AS-3.1 | `tests/e2e/client_identity_change_test.go:474` |
| AS-3.2 | `tests/e2e/client_identity_change_test.go:519` |
| AS-3.3 | `tests/e2e/client_identity_change_test.go:503` |
| AS-3.4 | `tests/e2e/client_identity_change_test.go:587` |

T014's five HTTP recovery journeys are written and compile; their failures are semantic, not missing symbols. Browser restoration of tab-local selections is separately unfinished under T016. Primary scenario presence does **not** complete T012/T015: the richer race, source-transition, background, rejected-update, alias, and rollout matrices remain required.

Focused semantic-red command:

```bash
TMPDIR=/tmp/gh-aw/agent ginkgo -v --procs=1 --label-filter='client-identity-change && !docker' ./tests/e2e/
```

Result before runtime changes: **16 run, 3 passed, 13 failed, 0 pending**. Callback identity-mismatch protection and same-ID filesystem/Admin rotation controls pass. Missing token invalidation returns 200 instead of 400, consent remains bypassed, displays report false reauthentication, and identity-invalid sweep candidates do not count as failed. This is red evidence only, not a validated implementation. No skip/pending markers or placeholder assertions were added.

Migration tests in `tests/integration/migrations/client_identity_revision_migration_test.go` cover lifecycle/defaults/no backfill/no table rewrite, validated versus unvalidated checks, ciphertext/row preservation, explicit-column legacy-shaped writes preserving evidence, default-0 legacy inserts, DOWN refusal, expansion contention/atomicity/live traffic, and validation alongside ordinary traffic versus maintenance contention. `go test -tags=integration -run '^$' ./tests/integration/migrations` passes compilation only. Historical-template reuse and actual PostgreSQL execution remain part of unchecked T008; actual prior-release binary compatibility belongs to unfinished T013, not the legacy-shaped SQL assertions.

#### Validation performed

Passing commands:

- `just check` (formatting corrected with gofmt before the passing run).
- `just test`.
- `just web-test` (261 tests; existing frontend coverage only).
- `just cdk-test`.
- `just mock-sample-agent-test`.
- `just mock-upstream-oauth2-test`.
- `just test-integration` (self-contained).
- `just test-e2e-frontend` (existing browser suite; no new feature browser cases or captures yet).
- `bash "$SECURITY_SCAN_SCRIPT" .` (prepared scanner, in place of `just security`).

`just test-e2e-backend` is **not passing**: 962 run, 941 passed, 21 failed. Thirteen are the new feature's expected red failures. Eight CIMD portless-registration journeys additionally returned 404 at `tests/e2e/authorization_flow_helpers_test.go:63`; a focused `--focus='CIMD Redirect URI Matching'` rerun passed, so the full-suite discrepancy remains unexplained and is not labeled a proven baseline defect or fixed by this increment.

The prescribed ExtProc `--label-filter='!docker'` command still selected unlabelled Agentgateway container cases. It did not pass (98 passed, one container startup failure, 16 skipped by existing suite behavior). No container access was enabled or retried. Those cases are deferred to container CI and their absence is **not** the implementation blocker. Label boundaries require maintainer attention; no CI or unrelated test was changed.

`just test-integration-infra`, `just test-e2e-docker`, and container-backed ExtProc cases are deferred to container CI. No container-backed behavior is claimed validated, and no local task waits for container evidence.

#### Remaining work and why it remains unchecked

- **T008**: finish historical-template reuse and obtain PostgreSQL migration evidence in container CI. The tests and migrations exist; this is not a local infrastructure blocker.
- **T011–T013**: finish provider counters/race fixtures and scenario matrices, pin the actual preceding release, and write actual-prior-release mixed-version/restart tests. Only the initial Reports/Calendar fixtures and primary HTTP scenarios are present.
- **T015–T018, T020**: finish isolation/rotation/source-transition/rejection/alias matrices, add all feature Playwright journeys and five captures, and complete the full test-first checkpoint. No feature browser file or screenshot exists yet.
- **T021–T030**: model/ports, both storage adapters, observation fencing, evaluator, metadata projections, and builder wiring are unimplemented because the design/test-first checkpoint is incomplete.
- **T031–T054**: runtime token/refresh/callback/consent/display/audit behavior and associated unit/concurrency tests are unimplemented; these depend on the missing foundation. The existing code must not be described as enforcing ADR 048 yet.
- **T055–T069**: final compliance, API consumer/operator documentation, screenshot/accessibility evidence, and passing feature/full verification remain unfinished. The full backend discrepancy must be resolved or demonstrated independently before a completion claim.

No new approval is requested. Resume under the supplied approval, finish the mandatory test-first checkpoint, implement the existing task order, and keep the PR draft. This increment must not be deployed as a completed invalidation feature.

### 2026-11-05 — browser acceptance and migration-test increment

**Outcome: blocked / partial progress, not a delivered feature.** T016, T017, and T020 are now complete as test-writing/capture-configuration/red-evidence tasks. Fourteen tasks are checked; 55 remain unchecked. No runtime enforcement was added. The approved spec and plan are unchanged, and the existing supplied approval remains sufficient; no additional decision or approval is requested.

#### Browser acceptance coverage (T016/T017)

Added `tests/e2e/frontend/client_identity_reconnect_test.go` with five primary browser journeys and a separate AS-2.4 cancellation variant. Each case has its own production-bootstrapped, URL-aligned end-user/Admin servers, isolated storage, Reports filesystem binding and Calendar connection. Both providers accept the fixture clients. The agent callback is a browser-local fixture at the broker test origin, not an external TLS destination; provider callbacks and broker authorization remain real HTTP flows. Page-object additions scope Reports Login to its accessible service article and exercise focused Enter/Space activation.

| Scenario | Browser test location | Configured screenshot |
|---|---|---|
| AS-2.1 | `tests/e2e/frontend/client_identity_reconnect_test.go:152` | `client_identity_consent_sign_in_required.png` |
| AS-2.2 | `tests/e2e/frontend/client_identity_reconnect_test.go:163` | `client_identity_consent_reconnected.png` |
| AS-2.3 | `tests/e2e/frontend/client_identity_reconnect_test.go:187` | `client_identity_sessions_reconnected.png` |
| AS-2.4 provider failure/retry | `tests/e2e/frontend/client_identity_reconnect_test.go:210` | `client_identity_reconnect_failed.png` |
| AS-2.4 cancellation/retry | `tests/e2e/frontend/client_identity_reconnect_test.go:232` | `client_identity_reconnect_declined.png` |
| AS-2.5 | `tests/e2e/frontend/client_identity_reconnect_test.go:263` | `client_identity_sessions_sign_in_required.png` |

AS-2.2 changes the optional permission-set selection and excludes Calendar before Reports login, asserts both tab-local choices are restored, verifies the current Reports credentials used by the provider, and completes the original authorization with its state and code. AS-2.3 checks the resolved service and configured origin in the actual token error URI, then returns through provider authorization to sessions. Failure/cancellation retain the previous session and grant, still deny token issuance, and offer keyboard retry. AS-2.5 first loads a usable session, publishes the identity change while the page stays open without navigating it, and expects sign-in on the next state load while Calendar remains refreshable.

All five planned screenshot calls and the supplemental decline capture use the existing opt-in page-object mechanism. No images have been captured: the required new states are not implemented yet. T049/T067/T068 remain unchecked; no screenshot or light/dark/accessibility validation is claimed.

#### Migration test progress (T008 remains unchecked)

`tests/integration/migrations/client_identity_revision_migration_test.go` now clones shared historical templates at 46, 47, and 48 rather than replaying the full history for each test. Migration discovery remains version-based without a maintained filename list. Lifecycle/guard/contention snapshots list every relevant existing service/session column explicitly, including ciphertext, establishment evidence and timestamps; a separate explicit identity/revision snapshot verifies DOWN refusal preserves the new evidence. No migration or framework behavior was changed. Compilation passes with the integration tag. Real PostgreSQL execution and actual-prior-release compatibility are still deferred to container CI and T013, respectively; this is not a local container-availability blocker.

#### Current validation and red evidence (T020)

- `just check`: passed, including the final changed tests.
- `just test`: passed on the rerun after the initial cold race-build invocation exceeded its 300-second command budget. The timeout is not recorded as a passing run.
- `go test -race ./tests/e2e/helpers/ ./tests/e2e/pages/`: passed.
- `go test -run '^$' ./tests/e2e/frontend/`: passed compilation only.
- `go test -tags=integration -run '^$' ./tests/integration/migrations/`: passed compilation only.
- `just web-build`: passed (also rebuilt by both acceptance recipes).
- `just test-e2e-backend`: **962 run, 949 passed, 13 failed, 0 pending**. Every failure is in the pre-existing feature acceptance tests and demonstrates missing invalidation/recovery. The eight CIMD failures from the earlier increment did not recur in this run; no unrelated fix or baseline-defect claim is made.
- `just test-e2e-frontend`: **73 run, 67 passed, 6 failed, 0 pending, 0 skipped**. All existing 67 browser tests pass; the six new cases fail semantically because consent is bypassed, token acquisition still returns 200, or session management has no Login action after a client-ID change. Initial fixture-only navigation errors were corrected before recording this final semantic-red evidence. No failed assertion was skipped or disabled.

Commands used `TMPDIR=/tmp/gh-aw/agent` so runtime temporary files and credential generations remain under the permitted directory. Acceptance failures are red evidence, not successful implementation validation. Container-backed migration/storage/restart/prior-release suites (`just test-integration-infra`, `just test-e2e-docker`, and container-labelled ExtProc cases) remain deferred to CI, never locally validated.

#### Remaining tasks and dependencies

- **T008**: PostgreSQL execution/actual-prior-release compatibility remains unverified; the historical-template and explicit-column changes are complete, but this task is not claimed fully delivered.
- **T011, T012, T013**: richer race fixtures and both-channel/background/rejected-update matrices, release pinning, and actual-prior-release mixed-version/restart tests remain unwritten. Container evidence is deferred, but writing those tests is still required locally.
- **T015, T018**: complete isolation/rotation/source-transition/rejection/alias matrices and supplemental browser/restart traceability. The 16 primary HTTP cases and six browser cases exist; supplemental prior-release variants do not.
- **T021–T030**: models/ports, both adapters, observation fencing, usability evaluator, metadata projections and builder wiring remain unimplemented; the mandatory test-first checkpoint is still incomplete.
- **T031–T041**: all token/refresh/callback/error/audit enforcement and corresponding unit/concurrency tests remain unimplemented and depend on the foundation.
- **T042–T049**: authorization/consent/list/detail consumers, frontend unit tests and status copy, integrated recovery validation, and actual screenshots remain unfinished. Browser tests are written, not green.
- **T050–T054**: the full routine-rotation/inclusion/rejection controls and test-driven corrections depend on runtime enforcement.
- **T055–T069**: final compliance, consumer/operator documentation, persistence/security/DI audits, complete passing verification, and maintained screenshot/theme/accessibility evidence depend on the unfinished feature. No final verification gate is claimed complete.

The implementation remains blocked by unfinished required work, not by a new approval requirement or the intentional absence of containers. This test increment must not be deployed as a completed invalidation feature.

### 2026-11-10 — identity-revision persistence increment

**Outcome: blocked / partial progress, not a delivered invalidation feature.** T021, T023, T025, T026, and T029 are complete as model/persistence implementation tasks. Nineteen tasks are checked; 50 remain unchecked. Both repositories now record identity evidence, but no runtime consumer enforces it yet. The approved `spec.md` and `plan.md` remain unchanged. The supplied eight-item approval still covers this work; no new API, ADR, configuration, dependency, or consequential decision was introduced.

#### Models, ports, and persistence

- T021: added JSON-excluded `ClientIdentity`, `ClientIdentityRevision`, and `UpstreamClientRevision`; provider copies independently own the identity pointer, revisions reject negative values, and an explicitly present empty provider identity is invalid. The new tests are in `internal/domain/model/client_identity_test.go` and the existing session model tests. Both adapters preserve establishment revision during refresh with the same callback generation; a new callback generation can replace it. Session revision validation errors are domain `StorageError` values at the repository boundary.
- T022 (partial, left unchecked): defined the focused identity repository and observation/state DTOs directly in `internal/ports/storage.go`, defined `SessionUsabilityEvaluator` and modes in `internal/ports/session_usability.go`, changed provider `Update` to return its operation-local `(identityChanged, error)` outcome, and mechanically adapted callers/mocks. The third-party domain currently discards that outcome; audit consumption belongs to unfinished T035/T040. Presence-aware authorization-state revision support is not implemented, so T022 remains unchecked.
- T023/T025: memory creation initializes stored identity at revision 0 and filesystem identity as null. Stored updates use the predecessor under the existing write lock, including the legacy stored-ID fallback. A differing identity increments once, including rollback; initialization, same-ID secret rotation/metadata, and resulting-filesystem updates do not increment. Resulting-filesystem metadata updates preserve an observation that committed after the caller's initial read and report `identityChanged=false`. Observations atomically fence source, version, canonical ID, and revision, including initialization and same-ID no-ops; lost preconditions write nothing. Returned evidence is independently copied. Cancellation and exhausted revision counters fail without mutation.
- T024 (partial, left unchecked): added locally runnable PostgreSQL record/copy/JSON/validation tests in `internal/adapters/storage/postgres/client_identity_record_test.go`. Added integration-tagged conformance and session-generation tests in `internal/adapters/storage/postgres/client_identity_integration_test.go`, reusing the existing migrated-template setup. Both stores run the same contract in `tests/storagecontracts/client_identity.go`: initialization, same-ID rotation, accepted/rejected/invalid updates, rollback, stale metadata snapshots, all observation preconditions, source round trips, canonical changes, concurrent observations, every service projection, and copy isolation. The remaining explicit PostgreSQL rollback/failure/restart matrices are not written; real database execution is deferred to container CI.
- T026: PostgreSQL creation, service projections, and adapter-local records include identity/revision explicitly. Stored updates capture the predecessor under `FOR UPDATE`, compute the identity outcome within that transaction, retain the latest evidence for filesystem updates, and expose success only after commit. Filesystem observations use a conditional `UPDATE ... RETURNING` with all four fences, preserve admin version/timestamps, and distinguish a lost precondition from a missing service. Read/write timeouts and domain storage errors are retained. The factory exposes the narrow repository for both backends. This is implemented and unit-tested mapping behavior, **not** a claim of real PostgreSQL validation.
- T029: session summaries carry JSON-excluded establishment ID/revision. PostgreSQL summary reads still project metadata only, without selecting encrypted token blobs; memory uses the shared summary constructor. Copies and JSON non-exposure have local tests, and the PostgreSQL integration test asserts summary metadata. Full and locked session reads/writes carry the revision explicitly. No rejection, refreshability, expiry, encryption, or AAD rule changed. Domain display/evaluator integration is still T028/T046.

The shared conformance tests save an administrative snapshot, commit a differing observation at the unchanged admin version, then complete the stale-snapshot PUT. This deterministically reproduces the repository interleaving without sleeps or test-only production APIs. Domain-level paused-request audit attribution still requires T035/T040.

#### Red/green evidence

Before implementation, the compiling model tests failed because provider copies lost revision evidence and negative/empty identity evidence was accepted. The memory repository tests then failed because creation retained caller-supplied revision 99, updates retained forged revision 100, observations did not apply, and concurrent observations produced no increment. After creation was implemented, the latter recording failures were observed separately before implementing them. The session-generation test failed because refresh replaced revision 7 with 8 and summaries lost establishment metadata. PostgreSQL mapping tests failed because identity/revision fields were dropped. The factory test failed because the focused repository was not wired. Finally, both stores' negative-session-revision tests failed on raw validation errors before adding storage-error wrapping.

All of these locally runnable tests are now green, including race detection through `just test`. PostgreSQL behavioral test binaries compile; their red/green execution belongs to container CI. No assertion was weakened or skipped to obtain a pass.

#### Current checks and failures

Passed:

- `just check` (final run after formatting: zero lint issues).
- `just test` (final run, including race-enabled model/repository tests).
- `just web-test` (261 tests).
- `just cdk-test`.
- `just mock-sample-agent-test`.
- `just mock-upstream-oauth2-test`.
- `just test-integration` (self-contained).
- Compile only: `go test -c -tags=integration -o /tmp/gh-aw/agent/postgres.test ./internal/adapters/storage/postgres` and the equivalent compile of `./tests/integration/storage/infra`.

Not passing:

- `just test-e2e-backend`: **962 run, 949 passed, 13 failed**. All failures are the feature's missing enforcement/recovery behaviors; existing backend cases pass. The 11 container/performance cases are excluded by the maintained recipe.
- `just test-e2e-frontend`: **73 run, 67 passed, 6 failed, 0 pending/skipped**. All six failures are the existing feature browser cases. No new state screenshot is claimed captured.
- `bash "$SECURITY_SCAN_SCRIPT" .`: the initial concurrent invocation exited 137 during govulncheck. A standalone rerun completed gosec with zero issues, then failed govulncheck on **GO-2026-6865**, `github.com/klauspost/compress v1.20.1`, fixed in `v1.20.2` (`go.mod:151`). `go.mod` and `go.sum` are unchanged by this increment. The scanner stopped before subsequent scans, so the complete security gate did not pass. This unrelated dependency finding was not investigated through exploitation, suppressed, or fixed by expanding the feature; no issue was opened without maintainer authorization.
- The attempted ExtProc non-container invocation with label/focus arguments nevertheless selected all 115 cases, including unlabelled Agentgateway container cases, and failed at a container startup (98 passed, 1 failed, 16 skipped by existing suite behavior). No further container attempt was made, and no CLI, harness, label, workflow, or assertion was changed. Do not count this as a passed non-container ExtProc gate. Container suites remain deferred, not blockers caused by the intentional sandbox boundary.

An early `go test -tags=integration -run '^$'` attempt also reached the PostgreSQL package's existing `TestMain` infrastructure check; it was not real database validation. Subsequent infrastructure checks used `go test -c` only. `just test-integration-infra`, `just test-e2e-docker`, and container-backed ExtProc cases remain deferred to the workflow's container tests, never reported as locally validated.

#### Every remaining unchecked task

- **T008**: real migration lifecycle/locking evidence is deferred to container CI; actual prior-release compatibility still needs T013's fixture. Existing migration tests are not claimed database-validated.
- **T011–T013, T015, T018**: finish richer acceptance fixtures, delayed/background/source-transition/rejected-update/isolation matrices, actual prior-release mixed-version/restart test writing and release pinning, and supplemental traceability. Primary HTTP/browser cases already exist; the supplemental work is still required, not deferred merely because containers are unavailable.
- **T022, T024, T027, T028, T030**: add presence-aware state support, remaining PostgreSQL rollback/failure/restart cases, the current-identity resolver/usability rule and its fresh-read retry/error-mode tests, and builder wiring. Repository recording alone does not enforce session usability.
- **T031–T054**: implement and test all token-release, refresh, callback, sweep, diagnostic/HTTP, audit, authorization/consent/display, recovery/UI, and isolation/rotation consumers. In particular, consume only the committed `Update.identityChanged` outcome for admin events; never compare a stale pre-read to returned evidence.
- **T055–T069**: final compliance, API consumer/operator documentation, persistence/security/DI audits, complete feature/full verification, and maintained screenshot/theme/accessibility evidence remain unfinished. The security dependency failure also needs maintainer triage outside this increment.

No new approval is requested. Continue under the supplied approval and retain the draft PR. This persistence increment must not be deployed as the completed client-identity invalidation feature.

### 2026-11-12 — identity resolution and usability foundation

**Outcome: blocked / partial progress, not a delivered invalidation feature.** T027 and T028 are complete; 21 tasks are checked and 48 remain unchecked. The shared domain evaluator is implemented and tested, but production consumers and builder wiring are not connected to it. Token acquisition, refresh, callback establishment, consent and session displays therefore still lack the approved revision enforcement. The approved specification and plan are unchanged. No new approval item is introduced.

#### Completed foundation

- **T027/T028**: `internal/domain/oauth2session/client_identity.go` implements the shared identity/revision rule for every service variant and composes it with ADR 046's rejection/expiry/refreshability predicate. Identity-less legacy sessions retain only the never-transitioned stored-source, revision-0 exception. Exact ID comparison and revision comparison have distinct safe reasons; restoring an earlier ID cannot make an earlier revision match.
- Filesystem observations fence source, version, canonical ID and revision through the existing port. A rejected observation is discarded, the service is reloaded without credential decryption, the binding is selected again, and the file is read anew. A transition to stored uses the freshly loaded stored ID instead of replaying the obsolete file value. The retry budget is eight observations per evaluation; exhausted contention fails closed with `generation_changed`, without a new setting or an availability fallback.
- Gate mode returns a safe source error on read/lookup failure. Display mode uses the latest persisted evidence, including evidence reloaded after a failed observation, and emits the source-failure event. If the service itself cannot be loaded there is no trustworthy current evidence, so the display conservatively requires sign-in. Neither mode decrypts a secret or reads a secret file. One service lookup/evaluation is shared across entries of a batch.
- `thirdparty.GetForIdentityEvaluation` returns an independently copied configuration with `Secret` absent, validates the requested service identity and never calls encryption. Reader/repository error text is not retained in printable credential-source errors; closed reasons and cancellation/deadline sentinels survive without paths or dependency payloads.
- The resolver emits `session.oauth2.client_identity_changed` only for `applied && changed`, using that observation's committed revision and `channel=filesystem_observation`. Initialization, same-ID observations, losing observations and errors emit no change event. This is the filesystem portion of T040 only; admin audit attribution and denied-use logging remain unfinished. The observability contract additionally names `usability_display` for display-fallback source failures.
- **T022 partial, still unchecked**: `OAuth2StateTokenClaims.UpstreamClientRevision` is now a presence-aware `*int64`; explicit 0 survives JSON serialization, absence remains absent, and a negative value is rejected. This is claim/type support, not all-variant initiation sealing, callback presence enforcement, or session establishment copying. T033/T037 must finish those behaviors before T022's complete constraints can be claimed.

#### Test-first evidence

The new compiling `TestSessionIdentityStatus`, resolver and evaluator suites initially failed semantically: mismatch/supersession/legacy denial returned nil, observations were not retried, and gating/display conditions were not evaluated. After implementation those assertions pass. The state round-trip tests initially dropped explicit revisions 0 and 9 and accepted -1; they now pass unchanged. Additional invalid-source/negative-revision tests failed before fail-closed validation was added. Dependency-error tests then exposed raw synthetic paths/payloads before error sanitization; those assertions now pass with cancellation identity retained.

Tests in `client_identity_test.go` and `client_identity_observation_test.go` cover confidential/public/CIMD/Google/DCR/filesystem variants, identity mismatch versus supersession, legacy transitions, ADR 046 predicate combinations, initialization/same-ID/change/stale-replica observations, fresh reads across stored-source changes/new bindings/source round trips, bounded contention, source errors, unavailable services, post-reload display fallback and credential-free audit attribution. `thirdparty/client_identity_test.go` checks no decryption, missing/mismatched service handling and independent copies. `state_token_revision_test.go` checks revision presence and validation. No existing acceptance assertions were weakened or disabled.

#### Current validation

Passed:

- `just check` (final run: zero lint issues).
- `just test` (final race-enabled package gate, including all new domain tests).
- `go test -race ./internal/domain/oauth2session ./internal/domain/thirdparty ./internal/domain/model`.
- `just web-test` (261 tests).
- `just cdk-test`.
- `just mock-sample-agent-test`.
- `just mock-upstream-oauth2-test`.
- `just test-integration` (self-contained).
- Non-container ExtProc slice: `go test -v ./tests/e2e/extproc/ -run '^TestExtProcTokenExchange$' -ginkgo.v -ginkgo.label-filter='!docker' -ginkgo.skip='Agentgateway'`: 98 passed, 17 container cases excluded. Source inspection identified all five Agentgateway container contexts; no container-backed test was started, no labels/assertions were changed, and this result does not validate those excluded cases.

Not passing:

- `just test-e2e-backend`: 962 run, 949 passed, 13 failed; all failures remain in the feature's missing enforcement/recovery journeys. Eleven container/performance cases are excluded by the maintained recipe.
- `just test-e2e-frontend`: 73 run, 67 passed, six failed, zero pending/skipped; all six failures are the feature's missing recovery/sign-in state. No maintained screenshot is claimed captured.
- `bash "$SECURITY_SCAN_SCRIPT" .`: standalone scan completed gosec with zero issues, then govulncheck failed on GO-2026-6865 (`github.com/klauspost/compress v1.20.1`, fixed in v1.20.2) and reported GO-2026-6859 (`net/http` in Go 1.27.3, fixed in Go 1.27.4). The unchanged baseline pins these at `go.mod:151` and `go.mod:3`. No exception, suppression, toolchain/dependency change or exploitation was attempted. Later scan stages did not run, so the complete security gate is not validated. Maintainer baseline remediation is required.

All commands used `TMPDIR=/tmp/gh-aw/agent`. Initial concurrent lint/security processes exited 137; standalone retries produced the final lint pass and deterministic security findings above, not a claimed passing security scan.

`just test-integration-infra`, `just test-e2e-docker`, and the 17 Agentgateway/other container-backed ExtProc cases remain deferred to container CI on the pushed commit, never locally validated and never a blocker caused by the intentional sandbox boundary.

#### Every remaining unchecked task

- **T008**: real migration lifecycle/locking evidence remains deferred to container CI; actual prior-release compatibility still needs the T013 fixture.
- **T011–T013, T015, T018**: richer acceptance fixtures and delayed/background/source-transition/rejected-update/isolation matrices, actual-prior-release mixed-version/restart test writing and release pinning, and supplemental scenario traceability remain unfinished. Container test writing is required locally even though execution is deferred.
- **T022**: verify the scoped model/port/DTO/caller and state-type contracts before marking complete. All-variant sealing and callback establishment remain T033/T037, not T022 prerequisites.
- **T024**: remaining PostgreSQL rollback/failure/restart matrices are unwritten; real database execution remains container CI work.
- **T030**: builder wiring of the identity repository/evaluator and consumer dependencies is unfinished. The standalone rule is not enforcement.
- **T031–T041**: token-release, locked refresh, callback, sweep/dry-run, diagnostic/manual-refresh HTTP mapping, committed admin audit attribution, denial logging and their unit/concurrency/HTTP tests are unfinished. The new reason exists, but its token-exchange and HTTP mappings do not yet exist. The resolver's filesystem change event alone does not complete T040.
- **T042–T049**: authorization/consent/list/detail consumer tests and integration, frontend status copy/unit tests, reconnect validation, maintained screenshots and passing recovery journeys are unfinished.
- **T050–T054**: complete inclusion/rotation/isolation controls and their implementation corrections/green acceptance validation depend on the missing enforcement consumers.
- **T055–T069**: final traceability/design/API/operator documentation, persistence/security/DI audits, screenshot/theme/accessibility evidence and a passing full verification gate are unfinished. The unrelated dependency/toolchain findings separately prevent a passing security gate.

No new approval is requested. Resume under the existing supplied approval and keep the PR draft. This foundation increment must not be deployed as the completed invalidation feature.


### 2026-11-18 — analysis findings remediation

This increment changes feature documentation only. It does not complete runtime enforcement, missing acceptance matrices, or infrastructure validation. There are 70 tasks: 21 checked and 49 unchecked.

- C1: Recorded red/green output, logs, and CI links are no longer completion requirements. Historical summaries remain informational.
- C2: Frontend validation requires the single light theme, keyboard access, focus, and WCAG AA contrast. Dark-theme work is not required.
- C3: Phase 2g and T070 supply the missing observability design gate. The design review remains unchecked.
- C4: The recovery checkpoint records the sequencing violation and blocks further behavior changes until T011–T013/T015/T018 finish. Those tests remain incomplete.
- I1: Spec/plan status now references the existing approval and accepted `048-client-identity-revisions`. Approved behavior is unchanged.
- I2: T022 covers foundational contracts and state types. T033/T037 own flow sealing and callback enforcement, with no reverse dependency.
- U1: The plan distinguishes uncontended reads from bounded observation retries. T031 verifies the read budget, safe exhaustion, and batch reuse.

The existing numbered ADR remains unchanged. New ADRs require UTC timestamp filenames. New or changed ADR references require full file stems.


### 2026-11-18 — acceptance recovery checkpoint

The historical sequencing violation remains recorded. This recovery increment adds acceptance coverage without adding production enforcement. Test-writing completion is distinct from green feature behavior and from real PostgreSQL/prior-release execution.

The screenshot inventory in `plan.md`, T017, T049, T067, and `quickstart.md` now includes six states: consent sign-in required, consent reconnected, sessions sign-in required, sessions reconnected, provider failure/retry, and provider cancellation/retry. The cancellation artifact is `client_identity_reconnect_declined.png`. Capture configuration already exists; no new screenshot is claimed captured.

Browser traceability verified against `tests/e2e/frontend/client_identity_reconnect_test.go`:

| Scenario | Browser test location | Observable acceptance behavior |
|---|---|---|
| AS-2.1 | `tests/e2e/frontend/client_identity_reconnect_test.go:152` | Next authorization enters consent and offers service-specific sign-in. |
| AS-2.2 | `tests/e2e/frontend/client_identity_reconnect_test.go:163` | Reconnect restores tab-local selections and completes the original authorization. |
| AS-2.3 | `tests/e2e/frontend/client_identity_reconnect_test.go:187` | Resolved-service recovery returns to sessions and restores token exchange. |
| AS-2.4 | `tests/e2e/frontend/client_identity_reconnect_test.go:210` | Failed reconnect preserves session/grant, denies issuance, and permits keyboard retry. |
| AS-2.4 | `tests/e2e/frontend/client_identity_reconnect_test.go:232` | Provider decline performs no code exchange, preserves session/grant, and permits retry. |
| AS-2.5 | `tests/e2e/frontend/client_identity_reconnect_test.go:263` | Next state load shows sign-in without unsolicited navigation; Calendar remains usable. |

Each browser case has a nearby scenario reference and runs under the shared Context with a fresh production-bootstrapped journey and cleanup. Failure and cancellation are separate supplemental variants of AS-2.4. Screenshot calls cover all six states through the existing opt-in mechanism.

Primary HTTP scenario locations after the recovery matrix additions:

| Scenario | Primary `It` in `tests/e2e/client_identity_change_test.go` |
|---|---|
| AS-1.1 | `:294` |
| AS-1.2 | `:305` |
| AS-1.3 | `:332` |
| AS-1.4 | `:376` |
| AS-1.5 | `:505` |
| AS-1.6 | `:923` |
| AS-1.7 | `:962` |
| AS-2.1 | `:531` |
| AS-2.2 | `:543` |
| AS-2.3 | `:592` |
| AS-2.4 | `:615` |
| AS-2.5 | `:657` |
| AS-3.1 | `:719` |
| AS-3.2 | `:854` |
| AS-3.3 | `:823` |
| AS-3.4 | `:1014` |

Supplemental HTTP variants in that file map to AS-1.4 at `:445` (missing pre-upgrade callback claims), AS-1.6 at `:470` (legacy sessions already rejected by source rules), and AS-3.4 at `:1109` (four obsolete-observer source/canonical-ID/binding interleavings). Primary scenarios retain one `It` each; variants exercise both change channels, independent principals, delayed work, rollback, routine rotation, and unchanged-service controls.

Supplemental AS-1.6 PostgreSQL/prior-release cases are in `tests/e2e/client_identity_rollout_test.go` under the `docker` label:

| Test location | Acceptance matrix |
|---|---|
| `:379` | Actual prior-release and new-replica token, refresh, browser, and session traffic under fixed identities; same-ID rotation; expanded-schema/default-0 compatibility; cross-version callbacks; incremental old-work drain; activation; restart/rollback and independent reconnection. |
| `:523` | Failed online validation retains healthy prior-release traffic, the identity freeze, dirty-state visibility, and schema evidence. |
| `:548` | Guarded preactivation abort drains new work while prior-release traffic continues, retaining fixed identities, zero increments, schema, and ciphertext. |

The fixture pins published `v0.1.147` through `fixtures.ClientIdentityPriorReleaseImage`; `CLIENT_IDENTITY_PRIOR_RELEASE_IMAGE` supports an operator-provided mirror. The released binary is not simulated or replaced with enforcement-disabled new code. All cases use production bootstrap and separate Admin/end-user listeners. The obsolete-observer variants use a bootstrap-only reader wrapper that delegates regular-file reads and pauses the captured value before observation; scenarios do not construct domain services or change production APIs.

Validation performed for this recovery increment:

- `just check`: passed with zero lint issues after replacing a deprecated testcontainers bind-mount API with its supported host-config modifier.
- `go test -race ./tests/e2e/helpers ./tests/e2e/bootstrap`: passed.
- `ginkgo -v --procs=1 --label-filter='client-identity-change && !docker' ./tests/e2e/`: all backend cases, including docker cases, compiled; 22 selected cases ran, with 2 passed, 20 failed, and 0 pending. Routine filesystem rotation and Admin/source-transition controls pass. Remaining selected failures expose missing invalidation, callback-revision/legacy enforcement, consent/display integration, and the unused identity-read barrier on the token fast path. These are semantic-red acceptance outcomes, not delivered feature behavior. Label-excluded cases were not executed.
- The Docker CLI cannot connect because the configured Colima socket is absent. No PostgreSQL/prior-release container journey was executed or claimed passed; run the docker-labelled slice in container CI.
- `ripwire --quality-delta` was run and is not clean: it reports Ginkgo closure growth, generic locking/constructor duplication matches, HTTP helper duplication, and new container-starter size/complexity warnings. No finding was suppressed or acknowledged, and no unrelated refactoring was added to this test-first recovery.

T011–T013/T015/T018 are complete as writing/compilation/traceability work. The task inventory is 26 checked and 44 unchecked. T070 remains open before further logging changes; runtime enforcement, green acceptance, actual screenshots, and final compliance remain unfinished.

The GitHub release API confirms that `v0.1.147` was published on 2026-11-17 at 13:31:23 UTC from `main` ([release](https://github.com/zalando-incubator/agentic-identity-broker/releases/tag/v0.1.147)). This verifies the published tag and date, not the registry image digest or mixed-version runtime compatibility; those remain unverified until the container journey executes.

### 2026-11-18 — runtime implementation and functional validation

All three user stories are implemented. Tokens and refreshes fail closed against current identity/revision evidence. Every new authorization seals both claims. Winning callbacks preserve generation precedence and alone establish session evidence. Rollback cannot revive old sessions. Consent, session displays, recovery, and isolation use the same rule. The full security gate is not passing.

The remaining PostgreSQL coverage is in `internal/adapters/storage/postgres/client_identity_{failure,concurrency,restart}_integration_test.go`, alongside the shared repository contract and mapping tests. A deterministic observation-versus-observation case now blocks the second writer behind the first increment and checks the lost revision precondition. Both PostgreSQL and migration test binaries compile with the integration tag. Real PostgreSQL execution, prior-release compatibility, restart, and rollback remain unverified here. Run `just test-integration-infra` and `just test-e2e-docker` in container CI.

The primary and supplemental non-container acceptance journeys pass. Same-ID rotation, metadata/source-transition, optional-service, unrelated-principal/service, rejection, and refreshability controls show zero unintended invalidation and no extra identity revision/change event for routine rotation. Existing credential-file tests retain their safety assertions and assert the exact newly permitted observation evidence.

The six required captures are published in `tests/e2e/screenshots/`. Browser tests cover restored selections, original authorization completion, real provider denial, failure/retry, and keyboard actions. The status label changed from `Login required` to `Sign-in required`; badge-visibility assertions remain. The user selected darker existing semantic tokens for Login controls only. All unrelated badge, indicator, scope, and Refresh styling is unchanged. Changed Login text contrast is 5.49:1; its focus ring exceeds 3:1. This does not claim remediation of pre-existing contrast outside the changed sign-in states.

The darker Login styling and its contrast figures in this historical record are superseded by the UI color correction below.

| Validation | Observed result |
|---|---|
| `just check` | Passed, zero lint issues |
| `just test` | Passed, race-enabled package gate |
| `just web-test` | Passed, 271 tests |
| `just cdk-test` | Passed |
| `just mock-sample-agent-test` | Passed |
| `just mock-upstream-oauth2-test` | Passed |
| `just test-integration` | Passed, self-contained suites |
| `just test-e2e-backend` | Passed, 982 specs; 21 container/performance cases excluded |
| `just test-e2e-frontend` | Passed, 73 specs; all six feature captures refreshed afterward with final scoped styling |
| Non-container ExtProc slice | Passed, 99 specs; 17 Agentgateway container cases excluded after inspecting their descriptions |
| PostgreSQL and migration integration binaries | Compilation passed; no database execution |

`just security` reached an existing Bash portability failure (`mapfile` under platform Bash). Running the unchanged script under Bash 5 completed its stages. Gosec passed, and govulncheck applied the repository's existing mock-only GO-2026-6911 exception. OSV still reports `fast-xml-parser` 5.11.1 (fixed in 5.12.0) and unpatched `picomatch` 2.3.2/4.0.7 findings in existing documentation/frontend lockfiles. No dependency, scanner workflow, or advisory exception was changed. The closed `token_acquisition` audit enum has a justified G101 false-positive annotation, consistent with existing diagnostic constants.

Read-only security/architecture review found no concrete feature-caused violation. Coverage review supplied the deterministic PostgreSQL overlap improvement. Structural quality-delta was run and remains non-clean: it flags generic test-fixture/counter/parser clones and intentional audit-boundary duplication/necessary refresh growth. Feature-added callback/provider logic and dry-run classification were consolidated; no quality acknowledgment or blanket suppression was added.

The synthetic third-party exchange example now uses the existing canonical `access_token` token type. This corrects an example typo, not accepted runtime types or approved requirements. No new configuration, endpoint, domain package, runtime bypass, dependency, or workflow is introduced. No future issue was opened or invented. Functional completion is distinct from a passing security gate or verified container deployment.

### 2026-11-18 — review corrections and focused revalidation

Authorization now evaluates the union of included and mandatory services through one Gate pass. Regression coverage verifies mandatory-only superseded and unknown-expiry sessions, fail-closed source failures, excluded optional services, and single reads for services that are both included and mandatory. Scope and rejection checks remain in force. The orphaned `ListActiveServiceIDsByPrincipal` interface, implementations, and dedicated tests were removed; callers use full session metadata. Identity-error classification is shared, including bounded reason/level extraction at usability audit boundaries. Admin update logging retains its privacy change; create-validation logging remains unchanged.

T024 and the plan now reference the actual PostgreSQL mapping and integration test files. Active design references use `anyRequiredSessionUnusable`; listing documentation retains the existing batch secret-state, expired-summary, and timeout guarantees. Earlier red-run records above are historical, not the current functional status.

After these code corrections, `just check` passed with zero lint issues. Race-enabled tests passed for OAuth2 authorization, consent, OAuth2 sessions, token exchange, in-memory storage, Admin handlers, consent handlers, and session HTTP handlers. The PostgreSQL integration-tagged test binary compiled. `just test-integration` passed; all 22 selected non-container backend feature journeys and all 6 browser recovery journeys passed. PostgreSQL/prior-release container execution remains unverified, and the security gate remains failing as described above.

Final quality-delta remains non-clean: 90 gating findings comprise 4 complexity, 54 duplication, 25 clone-of-helper, and 7 verbosity findings. Generic test-fixture/parser/counter matches remain, as does necessary refresh-path growth. No acknowledgment or blanket suppression was introduced. These findings are distinct from the baseline security failures and are not claimed clean.

Compliance closeout: T055–T069 execution is complete. T069 records execution and reporting, not a claim that every gate passed. All required non-container recipes were attempted; the unchanged security script was also run under Bash 5 after its platform-Bash failure. Its baseline OSV findings remain unresolved. Container-backed PostgreSQL, prior-release, and ExtProc validation must still execute in container CI; no local pass is claimed. Existing quality-delta findings and historical test-first sequencing limits remain disclosed. No baseline dependency remediation, scanner change, or exception was introduced to make this status green.

## Phase 6: Convergence

- [X] T071 CRITICAL — Eliminate the duplicate manual-refresh credential-source failure record across `internal/domain/oauth2session/service.go` (`ForceRefreshSession`) and `internal/adapters/http/oauth2_sessions/handler.go` (`RefreshSession`, generic 500 branch). Keep one owning `session.oauth2.credential_source_failed` event and preserve the existing HTTP status/body. Extend `internal/adapters/http/oauth2_sessions/handler_test.go` with constructor-injected JSON-buffer coverage of the real service-to-handler path, asserting one failure record, bounded metadata, and no credential/path values for unavailable identity sources; retain the existing identity/rejection outcomes. Resolve F1 per Constitution XV, SR-004, `contracts/observability.md` failure ownership, and T040/T064 (contradicts).
- [X] T072 CRITICAL — Make the live consent reconnect Login action in `web/src/components/consent/ServiceCard.tsx`, rendered by `web/src/pages/AgentGrantDetailPage.tsx`, meet WCAG AA using existing semantic tokens. Its current `bg-success-primary text-white` small-text combination measures 3.77:1, below 4.5:1; do not limit the correction or regression checks to `ServiceRequirementCard.tsx`, which is not this rendered surface. Verify normal/hover text contrast, visible keyboard focus, and the actual consent recovery journey; refresh affected maintained screenshots through the existing capture flow without restyling unrelated controls. Resolve F2 per Constitution XI, FR-008, and T010/T044/T047/T068 (contradicts).
- [X] T073 HIGH — Persist filesystem client-ID changes observed during coherent credential-pair selection in `internal/domain/oauth2session/credentials.go`, including mismatches that currently return before any subsequent identity resolution in refresh or callback. Reuse the identity observation mechanism in `client_identity.go` with source/version/canonical-ID/revision fencing; discard obsolete observations and reload after lost preconditions rather than recording a retired binding or relying only on a later file reread. Write deterministic regressions for refresh and callback where the initial identity read sees v1, `ReadPair` sees v2 and denies before provider authentication, and v1 is restored before another identity read: the old session must remain unusable until a fresh current-identity connection. Preserve coherent-pair acquisition, secret safety, no provider call on denial, and unchanged-identity rotation behavior. Resolve F3 per FR-003, FR-005, AS-1.4/AS-1.5, DEC-004, and T032/T033 (partial).
- [X] T074 MEDIUM — Strengthen the AS-2.5 browser journey in `tests/e2e/frontend/client_identity_reconnect_test.go` and the existing sessions page object so status/action assertions identify Reports and Calendar separately rather than relying on page-wide text and button counts. Prove Reports offers sign-in without Refresh while Calendar remains connected and refreshable, and observe the already-open page over a bounded interval after publication before explicitly reloading so delayed unsolicited navigation would fail the test. Retain session/grant preservation and screenshot assertions. Resolve F4 per FR-008, FR-014, AS-2.5, and T016 (partial).
- [X] T075 MEDIUM — Add consent-originated AS-2.4 browser variants in `tests/e2e/frontend/client_identity_reconnect_test.go`: enter consent after invalidation, initiate Reports sign-in there, then exercise provider failure and decline through the existing callback error return to session management. Assert Reports remains unusable, no grant expansion or old-token release occurs, the error and service-specific keyboard retry are available without navigation loops, and a subsequent successful retry restores eligible token acquisition. Reuse existing fixtures/page objects and capture any distinct required UI state through the maintained screenshot flow. Resolve F5 per FR-010, AS-2.4, Journey 2 failure variant, and T016 (partial).

### Convergence validation

T071–T075 are complete. Manual refresh retains domain ownership of credential-source failure logging, with constructor-injected JSON-buffer tests through the real handler path. Coherent credential-pair selection records fenced identity observations before mismatch denial; callback and refresh regressions retain invalidation after a transient publication is reverted. Lost preconditions discard credentials and reload the current configuration and binding. `ARCHITECTURE.md` documents this boundary.

The live consent `ServiceCard` Login uses existing primary semantic tokens. Browser assertions verify normal/hover AA text contrast and visible keyboard focus. Named session articles allow Reports and Calendar status/actions to be checked independently. The open-page case observes navigation for two seconds before reload. Consent-originated provider failure and decline preserve sessions and grants, deny old-token release, remain on the error recovery surface, and allow successful keyboard retry.

Validation performed:

- `just check`: passed, zero lint issues.
- `go test -race ./internal/domain/oauth2session ./internal/adapters/http/oauth2_sessions -count=1`: passed.
- Focused `ServiceCard` and `SessionCard` Vitest run: 45 tests passed.
- ESLint on changed frontend components and the new component test: passed.
- `just web-build`: passed.
- Built, serial `Client identity browser recovery` Ginkgo capture run: eight passed, zero failed; 67 unrelated cases excluded by focus. An initial failure exposed an incorrect single-request expectation for OAuth authentication-style negotiation; the incidental count assertion was removed, retaining the stable failure/no-token/no-mutation/current-identity/retry checks.
- All six existing screenshot states were captured. The changed consent sign-in capture was explicitly refreshed because the small Login color change falls below the maintained 0.5% whole-image threshold. The other five captures are within that threshold and their maintained images are unchanged.
- `ripwire --quality-delta --legend=compact`: non-clean, with 28 gating findings at measurement, including credential-selection/test complexity and page-object duplication. No finding was suppressed or acknowledged. This is not a clean structural-quality gate.

No full security or container gate was rerun; the earlier security findings and PostgreSQL/prior-release validation limits above remain unresolved, not newly verified. The optional post-implementation commit hook was offered, not executed. No commit or push was made.

Review follow-up: the consent-originated provider-failure case checks every token request since initiation, requiring an `authorization_code` grant and current Reports v2 credentials for each request. It allows authentication-style negotiation without fixing the request count, while rejecting stale-client or refresh-token attempts. Existing no-token-release, session/grant preservation, stable error recovery, and successful retry assertions remain.

Follow-up validation: the focused consent-originated provider-failure browser journey passed, and `just check` passed with zero lint issues. Scoped quality-delta remains non-clean (two gating findings in the browser test file); no suppression or acknowledgment was added.

## Phase 7: Convergence

- [X] T076 CRITICAL — Eliminate duplicate credential-source failure logging for authorization initiation and callback across `internal/domain/oauth2session/service.go` (`initiateOAuth2Flow`, `HandleCallback`) and `internal/adapters/http/oauth2_sessions/handler.go` (`InitiateFlow`, `HandleCallback`). The domain audit defers currently emit the canonical failure event, followed by a second generic HTTP-layer Error record; identity denials must not be escalated from Warn to Error. Establish one owning layer while preserving existing HTTP responses, redirects, closed metadata, and typed principals. Extend constructor-injected JSON-buffer regression coverage through the real service-to-handler paths for unreadable initiation identity and stale/missing callback identity or revision, asserting exactly one failure record with the required level/event and no credential or path values. Preserve T071's manual-refresh ownership fix. Resolve F1 per Constitution XV, SR-004, `contracts/observability.md` failure ownership, and T040/T064 (contradicts).
- [X] T077 HIGH — Validate the union of submitted included services and the Agent's mandatory service requirements through the existing Gate evaluator in `internal/domain/consent/service.go`, including mandatory services covered only by an optional, unselected permission set. Submission currently filters sessions and checks connectivity only for included IDs, allowing a Calendar-only grant to be committed while mandatory Reports is identity-invalid. Reject missing/unusable mandatory connections and fail closed on identity-source errors before grant mutation; do not silently select permission sets, include optional services, broaden scopes, or extend grant validity. Write regression coverage in `internal/domain/consent/service_test.go` and a complete HTTP acceptance variant for a valid Agent whose mandatory Reports requirement is covered by an unselected optional permission set; retain usable mandatory-service and excluded optional-service controls and prove rejected submissions preserve the existing grant. Resolve F2 per SR-002, spec Edge Cases (mandatory service requirements), plan mandatory grant-validation decision, and T042/T045 (partial).
- [X] T078 HIGH — Prevent stale client-identity usability from being reused on the next SPA connection-state load in `web/src/services/api/sessions.ts`, `web/src/services/api/consent.ts`, and their consuming hooks/pages. Session list/detail currently return two-minute cached summaries and ordinary consent detail returns five-minute cached connection state; mount/refetch does not bypass those entries, so returning through SPA navigation can still show Reports as connected without sign-in after an identity change. Use the existing fetching/cache mechanisms to obtain current usability on these loads, without polling, notifications, unsolicited navigation, or unrelated cache changes; preserve the already-uncached session-token authorization detail flow. Add API/hook regressions with changed backend state inside the old TTL and extend `tests/e2e/frontend/client_identity_reconnect_test.go` to visit connected sessions and ordinary consent detail, navigate away through SPA controls, change Reports identity, and return within the same JavaScript context. Assert service-specific Reports sign-in/no Refresh, unchanged Calendar usability and grants, and no unsolicited navigation; a full-document `Goto` must not substitute for the SPA regression. Resolve F3 per FR-008, FR-014, AS-2.5, and T016/T044/T047 (partial).

### Phase 7 validation

T076–T078 are complete. Initiation and callback credential-source denials retain one canonical domain audit record. Constructor-injected handler tests cover unavailable initiation identity and mismatched, superseded, or missing callback evidence. HTTP responses and redirects remain unchanged.

Consent submission evaluates included and mandatory services together without adding selections. Domain cases cover missing/unusable mandatory sessions, source errors, exact updates, replay, excluded optional services, and unchanged grants. The supplemental AS-2.2 HTTP journey reconnects mandatory Reports while its optional permission set remains unselected, then completes the original authorization with the same Calendar-only selection.

Live session list/detail and consent agent-detail reads bypass the frontend TTL cache. Unrelated metadata and grant caches remain. API regressions use the real cache. The AS-2.5 browser journey retains one JavaScript context, populates connected views, publishes the new identity, and returns through actual SPA controls. Reports offers sign-in while Calendar remains usable. Sessions and grants remain unchanged, with no unsolicited navigation or provider calls.

Validation performed:

- `just check`: passed, zero lint issues.
- `go test -race ./internal/adapters/http/oauth2_sessions -count=1`: passed.
- `go test -race ./internal/domain/consent -count=1`: passed.
- Focused backend feature acceptance run: 23 passed, zero failed; container cases excluded.
- Scoped Vitest API/hook run: 30 passed across usability freshness, consent API, and session hooks.
- ESLint on the changed API services and new regression file: passed.
- `just web-build`: passed.
- Built, serial browser recovery capture run: eight passed, zero failed. The six existing screenshot states were captured through the existing opt-in flow. No visual style changed.
- Scoped `ripwire --quality-delta --legend=compact`: non-clean, with ten gating findings. These comprise generic map/page-helper/HTTP-wrapper duplication matches and browser-test self-churn. New tests also carry size/complexity warnings. No suppression, acknowledgment, or unrelated refactoring was added.

The first browser run exposed an incorrect setup assertion: connected services appear in permission sets, not in disconnected-service headings. The corrected setup checks the selected Calendar service and enabled Save action. The final run exercises the full same-document journey. The first Vitest invocation used the repository root instead of the frontend root; the corrected package-root run passed.

No full security or container gate was rerun. The previously recorded dependency advisories and PostgreSQL/prior-release validation limits remain unchanged. No commit or push was made.

### CI repair validation — 2026-11-19

CI run `44542750710` exposed documentation links, PostgreSQL regressions, and historical-schema fixture failures.
Repository-only ADR and chart links use GitHub URLs so the documentation build can resolve them.
Provider identity reads reuse the locked session's transaction instead of waiting for another pooled connection.
Discovery fixtures preserve the existing error contract: valid stale attempts conflict, while missing timestamps fail validation. The production update ordering is unchanged.
The PostgreSQL fixtures use valid discovery timestamps, CIMD identifiers, and session scopes, and compare persisted timestamps by instant.
Source-round-trip coverage no longer waits for a row lock when the committed stored source already rejects the observation.

Historical-schema setup uses the actual preceding release until migration succeeds.
Remote requests preserve the configured Admin Host and each replica's public end-user origin.
The rollout Agent declares both services that its traffic matrix requests.
The container runtime must share the credential directory. Local Colima runs used `TMPDIR` under its shared home directory.
The first full integration run exposed a 32 ms Colima clock lag and failed immediate CIMD key selection.
After VM clock synchronization, the complete integration command passed without a key-activation code change.

Validation performed:

- `npm run build --prefix assets/docusaurus`: passed with broken-link validation enabled.
- `just test`: passed with race detection across 71 packages.
- `go test -count=1 -tags=integration -p 2 ./internal/adapters/storage/postgres/... ./tests/integration/...`: all seven packages passed, including migrations, bootstrap, and infrastructure suites.
- The normal-mode run skipped only `TestShortModeBypassesContainerStartup`, whose prerequisite is short mode. No requested package was omitted.
- Focused non-Docker credential, client-identity, and external-ID backend journeys: 175 passed with two workers.
- `go test -race ./cmd/agentic-identity-broker ./internal/config -run Credential`: both packages passed.
- `just test-e2e-backend`: 983 passed, zero failed, with performance and Docker cases excluded by the recipe.
- `just test-e2e-docker`: all 18 Docker scenarios passed, including historical-schema upgrade, mixed-version serving, drain, restart, rollback, and reconnect journeys.
- Final `just check`: passed with zero lint issues after all production, fixture, and test-helper changes.
- Structural quality-delta remains non-clean. It reports normalized-constructor/bootstrap duplication and growth in the concurrency test. No suppression or acknowledgment was added.

All requested Go, PostgreSQL/integration, backend E2E, and Docker E2E gates pass. No requested package was omitted. The security, frontend E2E, and ExtProc E2E gates were not rerun. No commit or push was made.

### UI color correction — 2026-11-19

The spec requires existing button variants without feature-specific colors. Session and service-requirement Login buttons use the standard secondary variant without green overrides. Existing success indicators remain unchanged.

Validation: 54 component tests, ESLint, the production frontend build, and all eight client-identity browser recovery journeys passed. All six screenshot states were captured and inspected. The three affected session sign-in, reconnection-failure, and reconnection-decline screenshots were refreshed; the other tracked captures remain unchanged.

## Phase 8: Convergence

- [X] T079 Record F1 as accepted without code changes per the user's explicit direction on 2026-11-19. Keep the current session Login variant and colors; no contrast, shared-token, screenshot, or related UI change is requested.
- [X] T080 CRITICAL — Give unknown-service authorization initiation one correctly leveled failure record across `internal/domain/oauth2session/service.go`, `client_identity.go`, `credentials.go`, and `internal/adapters/http/oauth2_sessions/handler.go`. A nonexistent service currently becomes a credential-source error, triggering the domain's Error-level `session.oauth2.credential_source_failed` record before the handler emits a second Warn and returns 404. Distinguish this caller-input outcome from unavailable identity infrastructure at the owning audit boundary; preserve existing error matching and the `404 service_not_found` response body, retain exactly one Warn record for the handled missing-service request, and keep genuine source failures at Error and identity denials at Warn. Extend the constructor-injected JSON-buffer real service-to-handler coverage in `internal/adapters/http/oauth2_sessions/handler_test.go` with a valid unknown service UUID, asserting status/body, one failure record, no Error-level infrastructure record, and safe bounded metadata; preserve existing initiation/callback/manual-refresh ownership coverage. Resolve F2 per Constitution XV, `coding-standards/LOGGING.md` section 3, SR-004, `contracts/observability.md` failure ownership, and T040/T064/T076 (contradicts).

### Phase 8 disposition and validation

F1 is accepted without code changes by explicit user direction. The current Login variant, colors, and screenshots remain unchanged; T079 records that disposition, not a contrast correction.

F2 is resolved. Authorization initiation leaves the missing-service warning to the existing HTTP handler instead of also emitting a domain credential-source failure. The 404 status/body and both existing error matches remain unchanged. The real service-to-handler ownership regression now covers an unknown service UUID, asserting one Warn record, no credential-source failure event, and no sensitive values. Existing unavailable-source and callback identity/revision cases retain their level, metadata, and response assertions. The observability contract documents the ownership distinction.

Validation performed:

- `just check`: passed, zero lint issues.
- `go test -race ./internal/adapters/http/oauth2_sessions ./internal/domain/oauth2session -count=1`: both packages passed, including the ownership regression and existing error-matching tests.
- Scoped `ripwire --quality-delta --legend=compact`: zero gating findings; five minor complexity, verbosity, and self-churn findings remain. No suppression or acknowledgment was added.

No full security, browser, or container gate was rerun. No commit or push was made.
