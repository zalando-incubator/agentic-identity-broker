# Tasks: Invalidate Sessions After Service Client-ID Changes

**Input**: `specs/055-invalidate-client-sessions/{spec.md,plan.md,research.md,data-model.md,quickstart.md,contracts/}`.

**Prerequisites**: Implementation requires the maintainer approval supplied by `context.approval` for API-001, API-002, ADR-001, DEC-001–DEC-005 in `plan.md`. This list grants no approval. All tasks remain unchecked until performed and validated.

**Tests**: Mandatory red-green-refactor (Principles VIII, XIII). Write compiling tests, observe semantic failure, implement the minimum, then refactor with unchanged assertions. Minimal types to compile tests are permitted; missing symbols, skipped tests, and vacuous assertions are not red evidence.

**Organization**: Setup → mandatory design preconditions → foundation → US1 (P1) → US2 (P1) → US3 (P2) → mandatory compliance. Paths are relative to the repository root.

## 🔒 Feature Scope [MANDATORY]

Every task cites a requirement, scenario, or binding principle. Scope is service client-identity invalidation, recovery, isolation, and the documentation and validation necessary to deliver them. No provider revocation, unsolicited browser navigation, new dependency, runtime switch, independent cleanup, or workflow change is included. `plan.md` records that no preparatory refactoring or new-entity CRUD scaffolding is needed. If required refactoring emerges, isolate it in a behavior-preserving commit before feature commits in this PR, with existing tests passing unchanged (Principles II, VIII).

## Phase 1: Setup (Approval Record)

- [ ] T001 Record only the supplied maintainer approval (login, date, approval-comment URL) in the Status of each approved feature ADR, then move `specs/055-invalidate-client-sessions/adr-047-client-identity-revisions.md` to `adrs/047-client-identity-revisions.md`; repair its relative references, retain partial supersession of ADRs 038/041 and full preservation of ADR 046, and never amend merged ADRs (ADR-001; Principle II). If the number is already occupied, use the next unused number and reconcile feature references without changing the decision. Do not mark anything Accepted without the corresponding supplied approval.

## 🔒 Phase 2: Design Preconditions (Blocking Prerequisites) [MANDATORY]

**Purpose**: Complete domain, configuration, API, database, concrete spec-journey, and E2E design before implementation. This phase blocks business implementation; only compiling test scaffolding is allowed before its checkpoint. Container-backed red evidence is collected separately in CI and must not hold up local work.

### Phase 2a: Domain Model & Glossary [MANDATORY]

- [ ] T002 Reconcile `specs/055-invalidate-client-sessions/data-model.md` with `contracts/ports.md` and the approved ADR: existing service/session aggregates, exact-ID comparison, monotonic per-service revision, callback-only establishment, legacy `R == 0`, and ADR 046 predicate composition; preserve no backfill and no new typed entity ID (FR-001–FR-005, FR-013, FR-015; Principles II, V).
- [ ] T003 Update `ARCHITECTURE.md` credential-source ownership (preserve administrative no-file-read metadata; explicitly replace the consent/session usability no-file-read rule under approved ADR 047), guarded rollout/rollback procedure, session lifecycle, both API major cutovers, and glossary with Effective Client Identity and Client Identity Revision; extend the existing Client-identity association and Requires Reauthentication entries rather than adding synonyms (FR-001–FR-015; Principles II, V). Update the ADR index and glossary in `AGENTS.md` only as ordinary architecture documentation, not via a context-update hook.

### Phase 2b: Configuration Design [MANDATORY]

- [ ] T004 Confirm and document in `docs/configuration.md` that existing `third_party_oauth2.credential_files` and `server.enduser.public_url` suffice and enforcement has no opt-out; add a procedure reference in `examples/config/third-party-oauth2.yaml` and its `examples/config/README.md` entry, preserving real keys/defaults and synthetic examples (FR-003, FR-012; SR-001, SR-003; Principle VII).
- [ ] T005 Document migrations 047/048, coherent publication, and ADR 047's continuous rolling upgrade in `charts/agentic-identity-broker/README.md`: freeze effective identity/source/canonical-ID/binding mutations and pause identity-mutating automation/direct writers and DCR registrations while keeping both ingress planes and existing traffic open; same-ID rotation and metadata-only writes continue. Apply additive migrations online before new replicas start, verify prior-release expanded-schema compatibility, and allow old/new serving only under unchanged identities. Drain each old process and its admitted work while healthy new replicas serve; verify complete revision-aware serving/writing inventory and prevent old-image restart before resuming identity changes. Failed checks preserve healthy traffic and the freeze. Pre-activation prior-binary abort requires unchanged identities, zero increments, drained new work, and retained evidence; post-activation rollback is revision-aware only. Confirm no Helm values/template or workflow/hook change is needed (DEC-002, DEC-004, DEC-005; Principles I, II, VII, IX). Never force past failed validation or discard evidence.

### Phase 2c: API Design [MANDATORY]

- [ ] T006 Verify supplied approval covers API-001 and API-002, then apply `specs/055-invalidate-client-sessions/contracts/enduser-api.md` to `api/enduser/openapi.yaml`: proposed 1.2.0 → 2.0.0 cutover, existing reauthentication/alias fields and inherited detail schema, connection status, automatic consent, stale callback denial, token infrastructure versus recovery outcomes, mandatory-service validation, and identity-related `409 refresh_unavailable` while preserving `409 refresh_rejected` (FR-006–FR-010, FR-014, FR-015; Principles IV, X). Record the approval reference in the implementation evidence, not by changing `spec.md` or `plan.md`.
- [ ] T007 [P] Apply the approved `specs/055-invalidate-client-sessions/contracts/admin-api.md` major cutover to `api/admin/openapi.yaml`, 2.1.1 → 3.0.0: change sweep classification of identity-invalid candidates from refreshed to failed, including dry runs, and update both count descriptions/examples and consumer migration notes; retain request validation, schemas, statuses, ETags, internal-field non-binding/non-exposure, and dry-run no-write semantics (FR-003, FR-012; API-002; Principles IV, X).

### Phase 2d: Database Design [MANDATORY]

- [ ] T008 Write migration tests first in `tests/integration/migrations/client_identity_revision_migration_test.go` using `framework.go` and shared PostgreSQL historical templates: 047/048 UP/DOWN/reapply, preserved ciphertext and null legacy identity, default revision 0, non-negative/non-empty constraints, lock contention with NOWAIT/timeout while live traffic continues, old-release explicit-column reads and writes against expanded schema (preserving unknown evidence columns and default-0 legacy inserts), atomic failure, validation state, and DOWN refusal after any service revision > 0 without data/schema mutation (FR-005, FR-013; DEC-002; Principle IX). Keep container tests integration-tagged and record their evidence in container CI.
- [ ] T009 Create `migrations/047_add_client_identity_revision.up.sql`, `migrations/047_add_client_identity_revision.down.sql`, `migrations/048_validate_client_identity_revision.up.sql`, and `migrations/048_validate_client_identity_revision.down.sql` exactly per `data-model.md`: nullable `client_identity`, service/session `BIGINT NOT NULL DEFAULT 0`, `NOT VALID` checks, `SET LOCAL statement_timeout = '1s'`, required NOWAIT locks, guarded DOWN 047, and DOWN 048 restoring unvalidated constraints; no identity backfill, wildcard projections, deletion, or non-atomic exception (DEC-002; Principles I, IX).

### Phase 2e: Frontend/Design System Review [MANDATORY IF FRONTEND]

- [ ] T010 Review `web/src/design-system/docs/INDEX.md`, `DECISION_TREES.md`, `COMMON_MISTAKES.md`, `COMPONENT_PAIRING_GUIDE.md`, and `ACCESSIBILITY_GUIDE.md` against `web/src/components/sessions/SessionCard.tsx` and `web/src/components/consent/ServiceRequirementCard.tsx`; document reuse of existing status/action primitives and semantic tokens, light/dark keyboard/contrast checks, and why no new universal component is required (FR-008; Principle XI; spec States A–D).

### Phase 2f: E2E Acceptance Test Design [MANDATORY]

All story-labelled acceptance-test tasks in this subsection execute before foundation implementation. They are placed here to meet the mandatory pre-implementation red gate, while retaining ownership by user story.

- [ ] T011 Create isolated Reports filesystem/stored and Calendar fixtures in `tests/e2e/fixtures/client_identity.go` and a coherent immutable-target/symlink-publication helper in `tests/e2e/helpers/client_identity.go`; reuse existing mock-provider helpers with both client IDs accepted and observable refresh/code-exchange counts; add the pinned prior-release image constant with its `CLIENT_IDENTITY_PRIOR_RELEASE_IMAGE` override and a testcontainers starter for the AS-1.6 mixed-version variant (research R12); configure files through production bootstrap, with temporary files confined to the permitted temporary directory (AS-1.1–AS-3.4; Principles VIII, XIII). Do not add test-only production APIs or import helpers from production.
- [ ] T012 [US1] Write the seven AS-1.1–AS-1.7 cases in `tests/e2e/client_identity_change_test.go`, one scenario per `It`, with variant matrices inside that scenario: both change channels, two principals/unknown expiry, all refresh paths/no provider authentication, delayed callback and refresh races (including missing identity/revision pre-upgrade state rejected before exchange), revision-aware rollback, continuous rolling legacy rollout/restart with simultaneous old/new token, refresh, browser/session, same-ID rotation, and metadata operations under fixed identities, incremental old-work drain before identity mutation, and failed-upgrade/pre-activation abort retaining traffic/freeze/evidence (the PostgreSQL variant uses the actual supported prior release against the expanded schema, never an enforcement-disabled new binary; no claim of safe mixed-version identity mutation), rejected/conflicting PUT, and successful fresh reconnect; assert real HTTP status, token absence, configured service recovery URI, session state, and provider call counts (FR-001–FR-006, FR-013; SC-001, SC-005).
- [ ] T013 [US2] Write the five AS-2.1–AS-2.5 complete HTTP journeys in `tests/e2e/client_identity_change_test.go`, continuing the shared feature Describe: automatic consent despite a grant, reconnect preserving valid selections and completing original authorization, direct `error_uri` recovery returning to `/sessions` then successful exchange, cancellation/failure/retry with no grant expansion, and list/detail/consent sign-in state (FR-007–FR-010, FR-014; SC-002, SC-003).
- [ ] T014 [US3] Write the four AS-3.1–AS-3.4 journeys in `tests/e2e/client_identity_change_test.go`: Calendar/login/grant isolation plus ADR 046 rejected/refreshable controls and alias equality; file secret rotation using the new secret on refresh; optional excluded versus included service; Admin secret/metadata/same-identity source-transition matrix including paused obsolete observers across source/canonical-ID/binding changes followed by a same-ID update with no spurious invalidation (FR-011, FR-012, FR-015; SC-004).
- [ ] T015 [US2] Add browser cases for AS-2.1–AS-2.5 in `tests/e2e/frontend/client_identity_reconnect_test.go` using production bootstrap and page objects in `tests/e2e/pages/consent_page.go` and `tests/e2e/pages/sessions_page.go`; cover next-authorization entry, tab-local restored selections, direct recovery, failure/retry, next-state-load updates without unsolicited navigation, and service-specific keyboard-accessible sign-in (FR-007–FR-010, FR-014; Principles XI, XIII).
- [ ] T016 [US2] Configure browser captures in `tests/e2e/frontend/client_identity_reconnect_test.go` for the five filenames in `plan.md`: `client_identity_consent_sign_in_required.png`, `client_identity_consent_reconnected.png`, `client_identity_sessions_sign_in_required.png`, `client_identity_sessions_reconnected.png`, `client_identity_reconnect_failed.png`; use the existing opt-in capture and maintained-image flow to `tests/e2e/screenshots/`, not workflow changes (FR-008–FR-010; Principle XIII).
- [ ] T017 Verify the 16 primary scenario mappings and supplemental browser/restart variants in `specs/055-invalidate-client-sessions/tasks.md` against `tests/e2e/client_identity_change_test.go` and `tests/e2e/frontend/client_identity_reconnect_test.go`; record actual test locations here. Enforce Describe → Context → It, nearby scenario references, fixtures, isolated BeforeEach/AfterEach, separate admin/end-user servers, no direct DI, fixed sleeps, placeholders, pending/Skip markers, or test-order dependencies (Principle XIII).
- [ ] T018 Review the three journeys and four Wireloom SVGs in `specs/055-invalidate-client-sessions/spec.md` and `wireframes/` against approved OpenAPI, synthetic config/HTTP examples, callback state, PKCE, and return-URI safeguards; no output JWT is introduced, so do not invent one. Record discrepancies and approved resolutions in implementation evidence without changing the approved spec/plan (SR-002, SR-003; Principle XIV).
- [ ] T019 Run compiling acceptance tests from `tests/e2e/client_identity_change_test.go` and `tests/e2e/frontend/client_identity_reconnect_test.go` before implementation; record semantic red evidence per missing behavior using focused non-container Ginkgo runs and the matching `just test-e2e-backend`/`just test-e2e-frontend` recipes. Keep `docker` restart variants out of local runs and defer their evidence to container CI; container availability must not block local implementation (Principles VIII, XIII).

**Checkpoint**: All design areas confirmed against supplied approval; acceptance tests exist with meaningful assertions and local semantic-red evidence. Container-only red/green evidence remains separately scheduled, not a local prerequisite.

## Phase 2.5: Foundational Infrastructure

- [ ] T020 Write red model/copy/JSON and revision-preservation tests in `internal/domain/model/thirdparty_oauth2_provider_test.go` and `internal/domain/storage/user_session_test.go`: `ClientIdentity` is "Null until first recorded. Non-empty when set. Never an Admin API input or output. Not encryption AAD (ADR 008)."; service revision is ">= 0. Changes only by +1 together with ClientIdentity. Never an Admin API input or output."; session revision is ">= 0. Set only when a winning callback creates or replaces the session, from the sealed state. Refresh, source transitions, and observations never rewrite it. Pre-existing rows default to 0." (data-model field rules; FR-004, FR-005, FR-013; Principles VIII, IX).
- [ ] T021 Extend `internal/domain/model/thirdparty_oauth2_provider.go` and `internal/domain/storage/user_session.go` with the internal fields and safe copies from T020, and define `ThirdpartyOAuth2ClientIdentityRepository` and the minimal state/observation DTOs directly in `internal/ports/storage.go` (not a conditional re-export), with atomic source/version/canonical-ID/revision preconditions and an explicit applied/no-op versus lost-precondition result; define `SessionUsabilityEvaluator` and modes in `internal/ports/session_usability.go` per `contracts/ports.md` (FR-001–FR-005, FR-013; Principles VI, IX). Public JSON and AAD must not gain identity evidence.
- [ ] T022 [P] Write red recording/parity tests in `internal/adapters/storage/memory/thirdparty_provider_test.go` and `user_session_test.go`: stored/filesystem creation, fallback to stored client ID when evidence is null, atomic accepted update increments, conflicting/invalid update no-op, first observation/no increment, same ID/no increment, source/version/canonical-ID/revision preconditions on null/same/differing-ID observations, paused observers losing to filesystem→stored transitions or canonical-ID/binding changes and source round trips at unchanged revision, no writes from lost preconditions and no spurious increment on a later same-ID update, concurrent equal observations/one increment, rollback increments, unchanged version/ETag/updated_at, copy-on-read, locked refresh preservation (FR-001, FR-003–FR-005, FR-012, FR-013; Principle IX).
- [ ] T023 [P] Write corresponding PostgreSQL record/mapping unit tests in `internal/adapters/storage/postgres/thirdparty_provider_record_test.go` and `user_session_test.go`, and integration-tagged behavior/concurrency tests in `tests/integration/storage/infra/client_identity_revision_test.go` using shared cloned databases and production migrations; assert discovery/version preconditions, explicit-column projections, session-lock/callback-copy behavior, exactly one concurrent increment; atomic source/version/canonical-ID/revision observation preconditions including initialization/no-op; paused filesystem→stored, canonical-ID/binding-change, and same-revision source-round-trip races that discard obsolete observations and preserve later same-ID updates; restart persistence, and unchanged admin version/timestamps on observations (FR-001, FR-003–FR-005, FR-013; Principle IX).
- [ ] T024 Implement the recording rules and session revision copies in `internal/adapters/storage/memory/thirdparty_provider.go`, `thirdparty_provider_record.go`, and `user_session.go` after T022 fails semantically; expose the focused repository through the memory adapter factory/accessor as needed without duplicating repository business logic (FR-001, FR-003–FR-005, FR-012, FR-013; Principles VI, IX).
- [ ] T025 [P] Implement equivalent sqlx mappings and conditional recording in `internal/adapters/storage/postgres/thirdparty_provider.go`, `thirdparty_provider_record.go`, and `user_session.go` after T023's locally runnable tests fail semantically; extend every service/session projection and write with explicit columns, preserve preconditions/locks/timeouts, wrap errors in domain StorageError, and expose the focused adapter interface (FR-001, FR-003–FR-005, FR-012, FR-013; Principles VI, IX). Real PostgreSQL red/green evidence comes from container CI.
- [ ] T026 Write table-driven red tests in `internal/domain/oauth2session/client_identity_test.go` for identity/revision match, mismatch, superseded, legacy stored never-transitioned R=0, legacy rejection after first change/transition/filesystem selection, rollback, first-file observation and CAS retry that reloads service/source/version/canonical ID/revision, reselects binding and reads anew instead of replaying obsolete values (including stored-source and canonical-ID/binding changes), stale replica observations within valid preconditions, Gate versus Display source failures, and the cross-product with ADR 046 recorded rejection/expiry/refreshability; assert no secret decryption on identity reads (FR-001–FR-005, FR-013, FR-015; SR-001; DEC-003, DEC-004).
- [ ] T027 Implement `resolveClientIdentity`, `sessionIdentityStatus`, and evaluator Gate/Display modes in `internal/domain/oauth2session/client_identity.go`; read current identity without secret decryption through `internal/domain/thirdparty/service.go`, observe filesystem IDs with bounded `ReadClientID` and atomic source/version/canonical-ID/revision CAS; on lost preconditions discard the value, reload the service/source/binding and file, or use stored identity if now stored (no stale-value replay), preserve legacy exception only at R=0, compose ADR 046's predicate, and use persisted evidence only for non-token display fallback with a source-failure event (FR-001–FR-005, FR-013, FR-015; SR-001; DEC-003, DEC-004).
- [ ] T028 Adapt metadata-only storage session summaries in `internal/domain/storage/user_session.go`, `internal/ports/storage.go`, and both `internal/adapters/storage/{memory,postgres}/user_session.go` so list/detail identity evaluation has internal `UpstreamClientID` and revision without encrypted token blobs or secret decryption; use an internal metadata projection/helper rather than forcing the full-session evaluator to load tokens; extend red/green copy/projection/JSON tests in the adjacent `_test.go` files (FR-008, FR-015; SR-004; Principles VI, VIII, IX; contracts/ports.md read models).
- [ ] T029 Write red dependency-injection tests in `internal/app/builder_test.go`, then wire the identity repository and usability evaluator in `internal/app/builder.go` for session, authorization, consent, and third-party services; update test mocks and thin production-bootstrap wrappers only as required, without handler construction or domain imports across adapter boundaries (FR-001, FR-007, FR-008; Principles VI, VIII, XII).

**Checkpoint**: Both stores expose identical revision semantics; one tested domain rule serves all consumers. No new bounded context, dependency, scheduler, distributed lock, or credential cache.

## Phase 3: User Story 1 — Stop Using Connections Established Under the Previous Client ID (Priority: P1) 🎯 MVP

**Goal**: Both filesystem and Admin changes stop all previous-identity token release and refresh; delayed work, rollback, and restart cannot restore usability.

**Independent Test**: Connect Alice and Bob under v1, change to v2 through each channel, deny still-valid/unknown-expiry tokens and every refresh path, then reconnect only Alice. Bob must remain denied. Restore v1 and prove no old session revives.

### Tests for User Story 1 [MANDATORY - Principle VIII]

Primary E2E red tasks: T012 and T019.

- [ ] T030 [P] [US1] Add red token fast-path tests in `internal/domain/oauth2session/credential_identity_test.go` and `client_identity_test.go`: unexpired/unknown-expiry/expired tokens, all service variants (stored confidential, public, Google, CIMD public/confidential, DCR, filesystem), legacy R=0/change, and unreadable/missing identity; deny before decrypt/release with correct safe reasons, preserving ADR 046 rejection precedence and no background submission on identity denial (AS-1.1, AS-1.2, AS-1.5–AS-1.7; FR-003, FR-015; SR-001).
- [ ] T031 [P] [US1] Add red refresh/overlap tests in `internal/domain/oauth2session/service_refresh_concurrency_test.go`, `background_refresh_test.go`, and `sweep_test.go` covering identity check under lock, in-flight change/reconnect ordering, deletion, no provider call for superseded sessions, no token/revision rewrite, callback generation precedence, changed sweep counts (one due identity-invalid candidate gives failed=1, refreshed=0 for real and dry runs), and dry-run classification with zero observation/session writes (AS-1.3, AS-1.4; FR-004, FR-005; ADR 038).
- [ ] T032 [P] [US1] Add red initiation/callback/JWE tests in `internal/domain/oauth2session/state_token_test.go`, `state_token_security_test.go`, and `service_reconnect_regression_test.go`: every new variant carries explicit `upstream_client_id` and presence-aware `upstream_client_revision` (>= 0); previous revision including rollback fails before code exchange; absent revision is distinct from explicit 0, and pre-upgrade state missing identity or revision for every variant is rejected with identity_missing before provider calls without changing the existing session; delayed results never replace a newer reconnect; failed callbacks preserve the old row and successful current callbacks clear rejection through existing behavior (AS-1.4–AS-1.6; FR-003–FR-005, FR-015; SR-002).
- [ ] T033 [P] [US1] Add red error-mapping/HTTP tests in `internal/domain/tokenexchange/session_diagnostic_test.go` and `internal/adapters/http/oauth2_sessions/handler_test.go`: all three identity reasons give recoverable `invalid_grant` and resolved-service/public-origin URI, no token/WWW-Authenticate; unreadable source gives server_error with no URI; manual refresh uses `409 refresh_unavailable`, preserving `409 refresh_rejected` and infrastructure 500 (FR-006, FR-015; SR-003; API-001).
- [ ] T034 [P] [US1] Add red Admin boundary/audit tests in `internal/adapters/http/handlers/admin/services_handler_test.go` and `internal/domain/thirdparty/service_test.go` for accepted client-ID updates, rejection/conflicts, no file reads on metadata writes, ignored/unbound internal identity inputs, no internal-field response leak, and exactly one committed change event (AS-1.7; FR-003; SR-004; API-002).

### Implementation for User Story 1

- [ ] T035 [US1] Enforce identity before every access-token return/decryption/background submission in `internal/domain/oauth2session/service.go` and maintain ADR 046 rejection checks; retain immediate valid-token return only after identity succeeds, without waiting for refresh (AS-1.1, AS-1.2, AS-1.5–AS-1.7; FR-002, FR-015; ADR-001).
- [ ] T036 [US1] Seal/validate/copy presence-aware `{client ID, revision}` for all new flows in `internal/domain/oauth2session/credentials.go`, `state_token.go`, and `service.go`; require both claims and compare sealed current identity before code exchange (legacy missing revision is not 0 and must restart sign-in), and retain callback-ID/session-lock precedence so stale work cannot win or rewrite establishment evidence (AS-1.4; FR-003–FR-005, FR-009, FR-013; SR-002).
- [ ] T037 [US1] Add identity enforcement inside locked refresh in `internal/domain/oauth2session/service.go` and candidate classification in `sweep.go`; cover on-demand/manual/background/sweep through the shared rule, preserve rejection exclusion, count superseded candidates as failed, and ensure dry-run comparison never persists an observation (AS-1.3, AS-1.4; FR-004, FR-015; API-002).
- [ ] T038 [US1] Add `identity_superseded` to `internal/domain/model/credential_source_error.go`, map it in `internal/domain/tokenexchange/session_diagnostic.go`, and map missing/mismatch/superseded identity to `409 refresh_unavailable` in `internal/adapters/http/oauth2_sessions/handler.go`, keeping safe infrastructure failures and ADR 044 recovery construction unchanged (FR-006; API-001; SR-001, SR-003).
- [ ] T039 [US1] Implement committed-update/observation audit events in `internal/domain/thirdparty/service.go` and `internal/domain/oauth2session/client_identity.go` per `specs/055-invalidate-client-sessions/contracts/observability.md`; extend tests in adjacent `_test.go` files for one event per winning increment, channel/revision/service metadata, typed admin operator principal, safe denied-use reason/operation, and absence of IDs' values, paths, credentials, or principal metric/span attributes; read both coding-standards documents before changing any signal (SR-004; Principles I, III; DEC-004).
- [ ] T040 [US1] Turn T030–T034 and AS-1.1–AS-1.7 green; run `just check`, `just test`, and `just test-e2e-backend`, recording local and container-deferred evidence in `specs/055-invalidate-client-sessions/tasks.md` without weakening assertions or claiming restart/PostgreSQL validation (SC-001, SC-005; Principles VIII, XIII).

**Checkpoint**: Token safety is independently demonstrable (technical MVP). US2 is also P1 and remains required for delivery; no partial MVP is declared feature completion.

## Phase 4: User Story 2 — Be Guided Through Consent and Reconnect (Priority: P1)

**Goal**: Next relevant browser authorization shows consent, every display offers sign-in, and existing reconnect flows restore only the owning user's service connection.

**Independent Test**: Begin with an existing valid delegation and unusable Reports session, enter agent authorization, reconnect from consent with selections intact, finish original authorization, then exercise direct error-URI recovery and failure/retry without grant expansion.

### Tests for User Story 2 [MANDATORY - Principle VIII]

Primary E2E/browser red tasks: T013, T015, T016, T019.

- [ ] T041 [P] [US2] Add red evaluator-consumer tests in `internal/domain/oauth2/service_test.go` and `internal/domain/consent/service_test.go`: included unusable session forces next-authorization consent in local/hybrid modes; mandatory submission fails; detail renders not_connected; gating read errors fail closed while display uses persisted fallback; valid contexts/selections, grant expiry, optional inclusion and ADR 046 refreshability are preserved (AS-2.1, AS-2.2, AS-2.5; FR-007–FR-009, FR-014, FR-015).
- [ ] T042 [P] [US2] Add red list/detail/manual-refresh-summary tests in `internal/domain/oauth2session/user_session_summary_test.go` and `session_details_security_test.go` proving unusable identity sets requires_reauthentication and its equal deprecated alias, access_token_expired remains independent, internal evidence stays hidden, and display failures do not release tokens or destroy sessions (AS-2.5; FR-008, FR-015; SR-004).
- [ ] T043 [P] [US2] Extend red tests in `web/src/components/sessions/SessionCard.test.tsx`, `web/src/components/consent/ServiceRequirementCard.test.tsx`, and `web/src/hooks/useSessions.test.ts` for backend-driven sign-in state despite valid expiry, Login versus refresh affordances, retained Terminate action, callback refresh of connection state, failure/retry, and keyboard-accessible existing actions; mock API sources, not hooks (AS-2.1–AS-2.5; FR-008–FR-010; Principle XI).

### Implementation for User Story 2

- [ ] T044 [US2] Integrate the Gate evaluator with `anyDelegatedSessionUnusable` in `internal/domain/oauth2/service.go` and mandatory validation in `internal/domain/consent/service.go`; add identity to, never replace, existing rejection/missing-session checks and use the existing consent redirect only on relevant authorization (FR-007, FR-011, FR-014, FR-015; SR-002).
- [ ] T045 [US2] Integrate Display mode into consent detail in `internal/domain/consent/service.go` and session list/detail/refresh summaries in `internal/domain/oauth2session/service.go`; read only the live client-ID file for usability (Proposed ADR 047 supersedes ADR 041's no-file-read rule on these displays only), retain no-secret/no-token-blob storage metadata reads and administrative service-metadata no-file-read tests, truthful identity state, equal requires_reauthentication/is_expired, distinct access expiry, and fallback logging (FR-008, FR-015; DEC-003).
- [ ] T046 [US2] Align `web/src/services/api/sessions.ts` and `web/src/components/sessions/SessionCard.tsx` with the existing required reauthentication field and sign-in action rather than expiry-only decisions; reuse the current consent not_connected presentation in `web/src/components/consent/ServiceRequirementCard.tsx` and tab-local return state without adding an enum, navigation loop, or live redirect (FR-008–FR-010, FR-014; Principle XI).
- [ ] T047 [US2] Verify and minimally repair the existing provider initiation/callback and refresh-after-return integration in `internal/domain/oauth2session/service.go` and `web/src/pages/ThirdPartySessionsPage.tsx` only where T013/T015 show a gap: consent reconnect preserves valid selections, direct recovery returns to `/sessions`, cancellation/failure never revives old tokens or broadens grants and leaves a retry; retain state/principal/PKCE/same-origin checks (AS-2.2–AS-2.4; FR-009, FR-010; SR-002, SR-003).
- [ ] T048 [US2] Run `just check`, `just test`, `just web-test`, `just test-e2e-backend`, and `just test-e2e-frontend` until AS-2.1–AS-2.5 and their unit/browser tests pass; capture the five maintained screenshots through opt-in serial capture described in `specs/055-invalidate-client-sessions/quickstart.md`, record results here, and keep assertions unchanged apart from fixture corrections (SC-002, SC-003; Principles VIII, XIII).

**Checkpoint**: Both P1 stories work end to end, including recovery; P2 isolation still gates full feature completion.

## Phase 5: User Story 3 — Keep Unaffected Access Working (Priority: P2)

**Goal**: No global logout, delegation reset, unrelated-service change, or reconnection from same-ID secret rotation/metadata/source transition.

**Independent Test**: Change only Reports while Calendar and grants retain their prior outcomes, rotate secrets through both channels with no revision increment, and prove excluded optional services do not force consent.

### Tests for User Story 3 [MANDATORY - Principle VIII]

Primary acceptance red task: T014 before foundation. Later unit tests expose any remaining isolation gaps before their fixes.

- [ ] T049 [P] [US3] Extend red credential-rotation/source-transition tests in `internal/domain/oauth2session/credential_rotation_test.go` and `internal/domain/thirdparty/service_test.go` to assert unchanged identity revision for same-ID file/Admin rotation and metadata updates, refreshed authentication using the new secret, provider credential rejection not mislabelled as identity recovery, and existing validation retained across source transitions (AS-3.2, AS-3.4; FR-012; SC-004).
- [ ] T050 [P] [US3] Extend red isolation/optional-service controls in `internal/domain/oauth2/service_test.go` and `internal/domain/consent/service_test.go`: only included services participate; unrelated principal/session/grant/scopes/validity unchanged; preserve Calendar rejected/refreshable ADR 046 outcomes, deprecated alias equality, and background/sweep rejection exclusion (AS-3.1, AS-3.3; FR-011, FR-015).

### Implementation for User Story 3

- [ ] T051 [US3] Make only test-driven isolation/rotation corrections in `internal/domain/oauth2session/credentials.go`, `internal/domain/thirdparty/service.go`, and both `internal/adapters/storage/{memory,postgres}/thirdparty_provider.go`: increment only on an effective differing identity, no increment for first observation or same-ID rotation/metadata/transition, never infer history, and preserve explicit source-transition validation (AS-3.2, AS-3.4; FR-012, FR-013).
- [ ] T052 [US3] Make only test-driven inclusion corrections in `internal/domain/oauth2/service.go` and `internal/domain/consent/service.go` so an excluded optional service never causes consent while an included changed service does; do not mutate unrelated sessions, broker authentication, scopes, grant validity, or ADR 046 outcomes (AS-3.1, AS-3.3; FR-011, FR-015).
- [ ] T053 [US3] Run `just check`, `just test`, and `just test-e2e-backend` until AS-3.1–AS-3.4 and rotation/isolation matrices pass alongside US1/US2; record zero unintended invalidation and no extra revision/change event for routine rotations in `specs/055-invalidate-client-sessions/tasks.md` (SC-004; Principles VIII, XIII).

## 🔒 Phase N: Constitution Compliance Verification [MANDATORY COMPLIANCE SECTION]

**Purpose**: Verify constitution requirements and feature-scope compliance. All required stories must be complete, not only MVP. Record evidence without changing the approved `spec.md` or `plan.md`.

### 🔒 Constitution Compliance Verification [MANDATORY]

#### Design Phase Verification [MANDATORY]

- [ ] T054 Verify every task traces to FR-001–FR-015, SR-001–SR-005, AS-1.1–AS-3.4 or an applicable binding principle; exclude unrelated remediation, and record only explicitly user-approved future-issue URLs as non-checkbox future work in `specs/055-invalidate-client-sessions/tasks.md` (Development Requirements: Feature Scope & Deferred Findings).
- [ ] T055 Verify T002–T010 preconditions against `ARCHITECTURE.md`, `examples/config/third-party-oauth2.yaml`, `examples/config/README.md`, `docs/configuration.md`, `charts/agentic-identity-broker/README.md`, `api/enduser/openapi.yaml`, `api/admin/openapi.yaml`, `migrations/047_add_client_identity_revision.up.sql`, `migrations/048_validate_client_identity_revision.up.sql`, and the approved ADR; confirm supplied written API/decision approval, no new config defaults or Helm settings, and completed design-system review (Principles II, IV, V, VII, IX, X, XI).
- [ ] T056 Verify all three concrete journeys in `specs/055-invalidate-client-sessions/spec.md` and four SVGs in `wireframes/` still match implemented configuration, HTTP, recovery and browser behavior; reconcile implementation to approved artifacts, not by silently changing spec/plan; retain synthetic values and no invented output JWT (Principle XIV).
- [ ] T057 Verify T012–T019 semantic-red evidence and scenario mapping against `tests/e2e/client_identity_change_test.go` and `tests/e2e/frontend/client_identity_reconnect_test.go`; all 16 primary scenarios each have one It, supplemental variants retain exactly one scenario reference, and no placeholder, skip/pending, direct-DI, test-order dependence or weakened assertion was introduced (Principles VIII, XIII).

#### Implementation Phase Verification [MANDATORY]

**API & Documentation** (Principles IV, X):

- [ ] T058 [P] Update `docs/api/` and `docs/reference/token-exchange.md` with approved identity-related reauthentication behavior and concrete token/reconnect examples; verify implemented statuses/error bodies, existing fields/alias, no new endpoint, and both API major cutovers (End-User 2.0.0, Admin 3.0.0) against `api/enduser/openapi.yaml` and `api/admin/openapi.yaml` (FR-006–FR-010, FR-015; Principles IV, X).
- [ ] T059 [P] Complete operator guidance in `docs/guides/manage-agents-and-services.md` and `docs/configuration.md`: both change channels, routine same-ID secret rotation, multi-replica publication/extra-reconnect risk, retained sessions, legacy R=0 policy, pre-upgrade callback sign-in retry on new replicas, and the continuous rolling/rollback boundary from ADR 047, consistent with T005. Include a checklist proving identity/source/canonical-ID/binding freeze across Admin/filesystem/DCR/direct writers, continued traffic and safe routine rotation during mixed-version overlap, online schema compatibility, completed old-work drains, and pinned revision-aware inventory/no-old-image restart before resuming identity changes. Failed checks retain healthy traffic/freeze; prior-binary abort is only pre-activation with unchanged identities, zero revisions, drained new work, and evidence retained. No unaware binary may resume serving/writing after activation (FR-003, FR-005, FR-012, FR-013; DEC-002–DEC-005; Principles II, VII, IX).

**Architecture & Documentation** (Principle II):

- [ ] T060 Verify `ARCHITECTURE.md`, `AGENTS.md`, and `adrs/047-client-identity-revisions.md` match the approved implementation and cite the actual maintainer sign-off; retain Proposed status for any unapproved decision and do not edit merged ADRs 038/041/046 (Principles II, V).

**Configuration** (Principle VII):

- [ ] T061 Verify all identity-file/public-origin use still comes through `internal/ports/config.go` and builder-injected configuration, existing `examples/config/` keys remain unchanged, and `charts/agentic-identity-broker/` has only required migration/procedure documentation, with no switch weakening enforcement (FR-003; SR-001, SR-003; Principle VII).

**Database & Persistence** (Principle IX):

- [ ] T062 Verify both stores and explicit-column queries in `internal/adapters/storage/memory/` and `internal/adapters/storage/postgres/` preserve revision, copy-on-read, callback-only writes, rejected-session predicates, preconditions, StorageError wrapping and timeouts; confirm migrations 047/048 numbering, guard, atomicity and real PostgreSQL integration coverage in `tests/integration/migrations/client_identity_revision_migration_test.go` and `tests/integration/storage/infra/client_identity_revision_test.go` (FR-004, FR-005, FR-013, FR-015; Principle IX). Record `just test-integration-infra` and `just test-e2e-docker` as container-CI validation, never as locally passed/deferred implementation prerequisites.

**Security** (Principles I, III):

- [ ] T063 Verify fail-closed token/refresh paths, no availability fallback or legacy backfill, callback claim presence/generation/PKCE/state/principal/same-origin protections, obsolete-observation fencing and fresh-read retry, continuous rolling upgrade with fixed identities during mixed-version traffic, verified old-work drain before identity mutations resume, guarded pre-activation abort and revision-aware-only post-activation rollback procedures, and unchanged crypto/AAD in `internal/domain/oauth2session/`, `internal/domain/tokenexchange/`, and `internal/domain/consent/`; test logs/events against `contracts/observability.md` for credential/path/client-ID-value absence and typed admin principals, with no new span/instrument or sensitive attributes (SR-001–SR-005; Principles I, III).

**Architecture Patterns** (Principles VI, XII):

- [ ] T064 Verify ports-only domain dependencies, no new domain package or adapter cross-import, thin HTTP mapping, narrow identity repository, and builder-only DI in `internal/ports/`, `internal/domain/`, `internal/adapters/`, and `internal/app/builder.go`; no handler-to-repository bypass or test-only production APIs (Principles VI, IX, XII).

**Testing** (Principle VIII - Unit & Integration Tests):

- [ ] T065 Audit red-green evidence for tests in `internal/domain/oauth2session/`, both storage adapters and consumers; preserve assertions through refactor, meaningful happy/error/edge/concurrency cases, no Bash correctness tests, and no unchecked required work hidden by a passing subset; run `just check`, `just test`, `just web-test`, and `just test-integration` successfully and record exact results in `specs/055-invalidate-client-sessions/tasks.md` (Principle VIII).

**E2E Acceptance Testing** (Principle XIII):

- [ ] T066 Run the complete non-container acceptance gate with `just test-e2e-backend` and `just test-e2e-frontend`; verify AS-1.1–AS-3.4 green, full user journeys, production bootstrap, scenario comments, isolated fixtures, minimal assertion changes, and five captured UI artifacts in `tests/e2e/screenshots/` using opt-in serial screenshot capture; record container restart variants separately, with evidence from `just test-e2e-docker` on the pushed commit (Principle XIII; SC-001–SC-005).

**Frontend** (Principle XI - if applicable):

- [ ] T067 Verify `web/src/components/sessions/SessionCard.tsx` and `web/src/components/consent/ServiceRequirementCard.tsx` use design-system primitives/semantic tokens, preserve keyboard actions and visible focus, work in both themes with WCAG AA text 4.5:1/UI 3:1 contrast, and introduce no custom CSS or unnecessary universal component; if a reusable primitive is actually required, cover it with existing Storybook/accessibility conventions (FR-008; Principle XI).

**Final gate and evidence** (Principles VIII, XIII):

- [ ] T068 Run every non-container recipe of the full `just verify` gate: `just check`, `just security`, `just test`, `just web-test`, `just cdk-test`, `just mock-sample-agent-test`, `just mock-upstream-oauth2-test`, `just test-integration`, `just test-e2e-backend`, `just test-e2e-frontend`, and the non-container ExtProc slice via `ginkgo -v --label-filter='!docker' ./tests/e2e/extproc/`; verify current recipe/label boundaries before running, record passing commands and failures in `specs/055-invalidate-client-sessions/tasks.md`, and fix change-caused failures without skipping/weakening tests or altering CI. Do not invoke container-backed recipes in the sandbox; list `just test-integration-infra`, `just test-e2e-docker`, and container-labelled ExtProc cases as deferred to container CI on the pushed commit, never as validated. Leave genuinely incomplete tasks unchecked, but do not delay completed local tasks for container evidence.

## Dependencies & Execution Order

### Phase Dependencies

```text
T001 approval record
  → Phase 2a–2e design + T011 fixtures
  → T012 → T013 → T014 (same backend file)
  → T015 → T016 (same frontend file)
  → T017–T019 mapping/journeys/local acceptance red
  → T020–T029 foundation (tests → models/ports → both adapters → rule → DI)
  → US1 T030–T040
  → US2 T041–T048
  → US3 T049–T053
  → Phase N T054–T068
```

All approval-dependent work uses only `context.approval`; unavailable/uncovered items are recorded and dependent tasks remain unchecked while unrelated permitted tasks continue. No task requests independent approval or readiness/merge actions.

### User Story Dependencies

- **US1 (P1)** depends on design and foundation; independently validates broker token safety. Tests T012/T030–T034 precede implementation T035–T039.
- **US2 (P1)** shares the identity rule and depends on US1's establishment/recovery/error paths for complete reconnect journeys. Consumer tests T041–T043 can be prepared after foundation; integrate after US1. P1 recovery is not optional.
- **US3 (P2)** validates and closes isolation/rotation gaps across the shared implementation; unit tests T049/T050 can be prepared after foundation, but final assertions require US1/US2 integration. AS-3 acceptance tests already exist before implementation.
- Same-file tasks run sequentially even across stories. Within any test task, observe semantic failure before its corresponding implementation; missing API scaffolding may be added solely to compile.
- T008/T023 container evidence is independent of the local critical path. Write tests and implementations in their normal order; container CI provides their red/green evidence. Never mark them locally validated merely because containers are unavailable.

### Parallel Opportunities and Examples

`[P]` means distinct files and no dependency on another incomplete task in the same execution batch; listed prerequisites still apply. Do not run a test task concurrently with implementation of the behavior it is meant to prove red.

- **Foundation**: after T021, T022 (memory tests) and T023 (PostgreSQL tests) are independent. After their red evidence, T024 and T025 adapters can proceed separately. T026/T027 wait for the required port/mapping scaffolding.
- **US1**: after foundation, prepare T030 fast-path tests, T031 maintenance/overlap tests, T032 callback/JWE tests, T033 mapping tests, and T034 Admin tests in parallel; T030 owns `client_identity_test.go`, so do not overlap T026. Then sequence T035–T037 because they share `service.go`.
- **US2**: after foundation, T041 authorization/consent, T042 session read models, and T043 frontend tests are independent; T044/T045 share consent code and run sequentially. Do not overlap T041 with US3 T050 in the same test files.
- **US3**: T049 credential rotation/third-party tests and T050 authorization/consent tests can run together after earlier same-file tasks finish. T051 then T052 fix only the failures they expose.
- **Finishing**: T058 API consumer docs and T059 operator docs are independent after behavior stabilizes. Design/API documentation T007 can run alongside T006 once approval is confirmed.

## Acceptance and Requirement Coverage

Every primary row maps to a separate `It` in `tests/e2e/client_identity_change_test.go`; variants stay within their scenario. Supplemental browser/restart cases must each cite one scenario, not combine multiple acceptance criteria into an untraceable test.

| Scenario | Primary red task | Implementation / validation | Supplemental coverage |
|---|---|---|---|
| AS-1.1 | T012 | T027, T035, T038–T040 | T030 filesystem, safe errors |
| AS-1.2 | T012 | T035, T036, T040 | T030 unknown expiry, principal independence |
| AS-1.3 | T012 | T037, T040 | T031 background/manual/sweep/dry-run |
| AS-1.4 | T012 | T036, T037, T040 | T031/T032 overlapping callback and refresh |
| AS-1.5 | T012 | T024–T027, T035, T040 | T026/T030 rollback |
| AS-1.6 | T012 | T024–T027, T035, T036, T040 | T005/T059 continuous rolling/fixed-identity overlap, old-work drain, abort/rollback and legacy callback retry; T008/T023 persistence; actual-prior-release docker mixed-version/schema and restart variants |
| AS-1.7 | T012 | T024, T025, T034, T035, T039, T040 | T022/T023 accepted versus rejected/conflicting PUT |
| AS-2.1 | T013 | T044, T048 | T015/T016 browser State A |
| AS-2.2 | T013 | T036, T046–T048 | T015/T016 browser State B |
| AS-2.3 | T013 | T038, T047, T048 | T015/T016 browser State D |
| AS-2.4 | T013 | T036, T047, T048 | T015/T016 browser failure/retry capture |
| AS-2.5 | T013 | T028, T045, T046, T048 | T015/T016 browser State C; T042/T043 |
| AS-3.1 | T014 | T050, T052, T053 | ADR 046 control matrix/alias, T026/T031 |
| AS-3.2 | T014 | T049, T051, T053 | New secret used without revision change |
| AS-3.3 | T014 | T050, T052, T053 | Included versus excluded optional service |
| AS-3.4 | T014 | T049, T051, T053 | T022/T023/T026 both-store source/configuration-generation CAS and fresh-read retry, no spurious later same-ID invalidation |

| Requirement / outcome | Tasks |
|---|---|
| FR-001, FR-002, SC-001 | T020–T027, T030, T035, T040 |
| FR-003 | T005, T007, T008, T011, T012, T021–T025, T030, T032, T034, T036, T059, T063 |
| FR-004, FR-005, SC-005 | T008, T009, T022–T027, T031, T032, T036, T037 |
| FR-006 | T006, T033, T038, T058 |
| FR-007, FR-014, SC-002 | T013, T015, T041, T044, T048 |
| FR-008 | T010, T013, T015, T016, T028, T042, T043, T045, T046, T067 |
| FR-009, FR-010, SC-003 | T013, T015, T016, T032, T036, T043, T047, T048 |
| FR-011, FR-012, SC-004 | T014, T022–T025, T049–T053 |
| FR-013 | T005, T008, T009, T012, T020–T027, T030, T036, T059, T063 |
| FR-015 | T006, T014, T026, T030–T033, T037, T038, T041, T042, T044, T045, T050, T052 |
| SR-001 | T026, T027, T030, T033, T035, T037, T063 |
| SR-002, SR-003 | T018, T032, T033, T036, T041, T044, T047, T063 |
| SR-004 | T028, T034, T039, T042, T063 |
| SR-005 | Scope boundary, T054, T063 |

## Implementation Strategy

1. Record real approval, apply approved contracts and migration design, and establish acceptance red before behavior changes.
2. Implement shared models/ports and both stores under TDD, then the single identity/evaluator rule and DI.
3. Deliver US1 as the independently testable token-safety MVP, then US2 recovery (also P1), then US3 isolation; all three are required before claiming completion.
4. Commit logical increments locally with the required co-author trailer. Preserve separate behavior-neutral refactoring commits if needed. No separate PR, direct git push, workflow edits, or readiness/merge action.
5. Complete Phase N and report local passing gates separately from container-CI evidence. Keep uncompleted work unchecked with reasons; container tests run on the pushed commit and are never represented as local passes.

## Implementation Evidence

Not started. Record semantic-red/green commands, scenario test locations, approved decision references, screenshot locations, container-deferred suites, and any genuinely incomplete tasks here during implementation. Do not edit the approved spec/plan to accommodate implementation drift.
