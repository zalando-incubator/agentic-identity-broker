---

description: "Task list for CIMD client authentication for third-party OAuth2 services"
---

# Tasks: CIMD Client Authentication for Third-Party OAuth2 Services

**Input**: Design documents from `/specs/046-cimd-upstream-client/`

**Prerequisites**: `plan.md`, `spec.md`, `research.md`, `data-model.md`, `contracts/admin-api.md`, `contracts/enduser-api.md`, and `quickstart.md`

**Tests**: Automated tests are mandatory under Constitution Principles VIII and XIII. Write every listed test before its implementation task, make it compile and fail semantically, and keep it unchanged except for fixture adjustments as the implementation turns green. Before a red test uses a new symbol, add only the compile-time contracts and inert stubs needed to compile. Do not add required behavior before the test fails.

**Organization**: Phase 2f creates the red, 1:1 E2E acceptance suite. Later phases are organized by user story and add the unit, adapter, integration, and production code needed to turn only that story's scenarios green.

## Format and scope

- `[P]` means the task can proceed concurrently with other marked tasks after its stated prerequisites complete.
- `[US#]` identifies the user story served by the task.
- `private_key_jwt` is explicit; it is never inferred from missing input or provider metadata.
- Do not introduce a frontend surface. This feature changes backend, API, persistence, documentation, and E2E coverage only.
- `cimd-upstream-client` is the unique Ginkgo label for this feature. Its label-filtered command must select only the 31 feature scenarios.

---

## Phase 0: Pre-implementation Refactoring (Separate Delivery)

**Purpose**: Isolate the shared signing-key lifecycle refactoring from CIMD HTTP behavior. Deliver this phase as a separate PR or isolated commit batch; it must preserve existing token issuance and `/oauth2/jwks.json` behavior.

- [X] T001 Draft `adrs/037-cimd-client-authentication-key-domain.md` with Context, Decision, Consequences, alternatives, and a proposed distinct `cimd_client_authentication` key domain; state that it shares lifecycle mechanics but never shares token-signing trust surfaces.
- [X] T002 Obtain architecture approval and change the Status to `Accepted` in `adrs/037-cimd-client-authentication-key-domain.md` before T003 starts or any key-domain production code is merged.
- [X] T003 Add only the compile-time `KeyDomain` type and repository method signatures needed by the tests in `internal/domain/storage/signing_key.go` and `internal/ports/storage.go`. Then write semantic-red regression tests for per-domain current-key selection, global `kid` uniqueness, grace-period fallback, and token-signing compatibility in `internal/domain/storage/signing_key_test.go` and `internal/domain/oauth2server/signing_key_service_test.go`. Do not implement domain behavior in this task.
- [X] T004 [P] After T003, write semantic-red adapter regression tests for domain-filtered key CRUD, promotion, deletion, locking, and global `kid` collision rejection in `internal/adapters/storage/memory/signing_key_store_test.go` and `internal/adapters/storage/postgres/signing_key_repo_test.go`.
- [X] T005 [P] Write semantic-red migration-032 and public-JWK regression coverage in `tests/integration/migrations/migrations_test.go`, `tests/integration/infra/oauth2_signing_key_bootstrap_test.go`, and `tests/e2e/aggregated_jwks_test.go`: backfill legacy signing keys, prove `032` apply/replay and its CIMD-key rollback guard, and assert that `/oauth2/jwks.json` keeps its existing token/upstream mode-appropriate contents and contains no CIMD-domain keys.
- [X] T006 Complete the `KeyDomain` behavior declared in T003: existing rows become `token_signing`; `cimd_client_authentication` is distinct; `KID` stays globally unique; and each domain has at most one active current key.
- [X] T007 Implement the domain discriminator in `internal/adapters/storage/memory/signing_key_store.go` and `internal/adapters/storage/postgres/signing_key_repo.go`, applying it to every read, list, count, current-key, promotion, deletion, row-lock, and bootstrap-lock path without weakening global `kid` uniqueness.
- [X] T008 Add `migrations/032_add_cimd_key_domain.up.sql` and `migrations/032_add_cimd_key_domain.down.sql`: backfill `token_signing`, add a closed domain check, replace the global active-current index with a per-domain active-current index, and refuse rollback while any `cimd_client_authentication` row exists.
- [X] T009 Scope existing token issuance and aggregated key publication explicitly to `token_signing` in `internal/domain/oauth2server/signing_key_service.go` and `internal/domain/oauth2/jwks_publisher.go`, preserving all existing token and proxy-mode behavior.
- [X] T010 Run the focused behavior-only refactoring tests in `internal/domain/oauth2server/signing_key_service_test.go`, `internal/adapters/storage/memory/signing_key_store_test.go`, `internal/adapters/storage/postgres/signing_key_repo_test.go`, `tests/integration/migrations/migrations_test.go`, and `tests/e2e/aggregated_jwks_test.go`. Then run `just verify`. All existing tests must pass before Phase 1 begins.

**Checkpoint**: The accepted ADR and refactoring are isolated. Token-signing keys remain behaviorally unchanged and cannot enter the future CIMD key domain. The full existing test suite passes before Phase 1 begins.

---

## Phase 1: Setup (Shared Test Infrastructure)

**Purpose**: Establish the isolated test infrastructure needed to write all feature acceptance tests before production behavior changes.

- [X] T011 [P] Create `tests/e2e/helpers/mock_cimd_upstream.go`, a per-test conformant provider double that fetches broker metadata and JWKS, validates ES256 assertions and PKCE, caches keys for rotation tests, rejects any secret authentication, and never logs secrets, assertions, codes, verifiers, or tokens.
- [X] T012 [P] Create deterministic CIMD service and key fixtures in `tests/e2e/fixtures/cimd_client.go` for the three authentication modes, each OAuth server mode, active/grace/promotion key states, and an HTTPS end-user public URL.
- [X] T013 Extend `tests/e2e/bootstrap/cimd.go` so production `app.Builder` servers can use the public-HTTPS test URL and the CIMD upstream double without duplicating application DI or relying on fixed sleeps.

---

## 🔒 Phase 2: Design Preconditions (Blocking Prerequisites) [MANDATORY]

**Purpose**: Complete and lock the domain, configuration, API, database, frontend-scope, and E2E designs before feature production code proceeds.

**⚠️ CRITICAL**: No user-story implementation may begin until every Phase 2 task is complete and the E2E suite has compiled with semantic red failures.

### Phase 2a: Domain Model & Glossary [MANDATORY]

**Constitution References**: Principles II (Architecture Documentation) and V (Domain-Driven Design & Glossary Management)

- [X] T014 Confirm the data-model invariants in `specs/046-cimd-upstream-client/data-model.md`: the broker writes `<enduser-public-url>/.well-known/oauth-client/<service-id>` and the operator cannot supply or replace it; `Secret` “Must be absent. The broker must not encrypt, persist, decrypt, or return a shared secret”; credential-derived variants, including Google, reject `private_key_jwt`; and this feature introduces no inactive service state.
- [X] T015 [P] Update the route inventory, key-domain boundary, outbound client-authentication flow, glossary terms, and SC-008 load profile and p95 target in `ARCHITECTURE.md`, including CIMD confidential service, CIMD client-authentication key, broker-hosted metadata document, client assertion, and the two `KeyDomain` values.

**Checkpoint**: The ubiquitous language, domain model, and accepted architectural decision describe the same invariants.

### Phase 2b: Configuration Design [MANDATORY]

**Constitution Reference**: Principle VII (Configuration-Driven Design)

- [X] T016 Document in `docs/configuration.md` that this feature adds no configuration parameter, but a CIMD confidential service requires the existing `server.enduser.public_url` to be a stable public HTTPS URL; HTTP must be rejected for this mode.
- [X] T017 [P] Add a safe HTTPS `server.enduser.public_url` example and the CIMD confidential-service precondition to `examples/config/third-party-oauth2.yaml` and reference it from `examples/config/README.md`.
- [X] T018 Verify `internal/ports/config.go`, `charts/agentic-identity-broker/values.yaml`, and `charts/agentic-identity-broker/templates/` require no change because this feature adds no configuration input; record that conclusion in `specs/046-cimd-upstream-client/plan.md`.

**Checkpoint**: The existing unified configuration port remains the only configuration source, and the HTTPS requirement is discoverable to operators.

### Phase 2c: API Design [MANDATORY]

**Constitution References**: Principles IV (OpenAPI Transparency) and X (API-First Development)

- [X] T019 [P] Apply `specs/046-cimd-upstream-client/contracts/admin-api.md` to `api/admin/openapi.yaml`: add the explicit `private_key_jwt` service mode; forbid caller `client_id` and non-empty `client_secret` in that mode; omit `client_secret` for secretless modes; and document the four flat `/api/cimd-client-keys` operations, first-key immediate activation, later-key grace activation, ES256-only requests, canonical errors, and no private key material.
- [X] T020 [P] Apply `specs/046-cimd-upstream-client/contracts/enduser-api.md` to `api/enduser/openapi.yaml`: document anonymous `GET /.well-known/oauth-client/{service-id}` and `GET /.well-known/oauth-client/{service-id}/jwks.json`, `security: []`, exact JSON/cache semantics, and canonical JSON 404 responses.
- [X] T021 Verify the stakeholder-confirmation record remains explicit in `specs/046-cimd-upstream-client/contracts/admin-api.md` and `specs/046-cimd-upstream-client/contracts/enduser-api.md`; do not implement an API variation not approved by those documents.

**Checkpoint**: Both root OpenAPI contracts are complete and confirmed before their handlers are implemented.

### Phase 2d: Database Design [MANDATORY]

**Constitution Reference**: Principle IX (Persistence Pattern Consistency & Database Migration Management)

- [X] T022 [P] Add semantic-red migration-033 cases in `tests/integration/migrations/migrations_test.go` for legacy static/public rows, valid secretless `private_key_jwt` rows, apply/replay, and the guarded rollback that refuses to discard CIMD service authentication state.
- [X] T023 Verify the completed `migrations/032_add_cimd_key_domain.up.sql` and `migrations/032_add_cimd_key_domain.down.sql` from T008 against `specs/046-cimd-upstream-client/data-model.md`: confirm the backfill, closed domain check, per-domain active-current index, global `kid` preservation, and CIMD-key rollback guard; do not create a second migration in this phase.
- [X] T024 Add `migrations/033_add_cimd_private_key_jwt_authentication.up.sql` and `migrations/033_add_cimd_private_key_jwt_authentication.down.sql`; allow `private_key_jwt` only when `client_secret_encrypted IS NULL`, and refuse rollback while any service has `token_endpoint_auth_method = private_key_jwt`.

**Checkpoint**: Schema evolution is reversible only when it can preserve all client-authentication state without fabrication or loss.

### Phase 2e: Frontend/Design System Review [MANDATORY]

**Constitution Reference**: Principle XI (Design System Compliance & Consistency)

- [X] T025 Record in `specs/046-cimd-upstream-client/plan.md` that the approved scope has no React or administrative UI change; therefore no design-system component, Playwright scenario, or screenshot task is applicable.

**Checkpoint**: The absence of frontend work is deliberate and traceable, not an omitted review.

### Phase 2f: E2E Acceptance Test Design [MANDATORY]

**Constitution Reference**: Principle XIII (End-to-End Acceptance Testing & Spec Traceability)
**Feature filter and traceability**: Every functional `It()` below MUST use `Label("cimd-upstream-client")` and a nearby comment in the form `// US#-S# from specs/046-cimd-upstream-client/spec.md`. The SC-008 test also uses `Label("performance")`.

- [X] T026 [P] [US1] Write five isolated Ginkgo `It()` blocks in `tests/e2e/thirdparty_cimd_service_test.go`: registration without caller credentials, contradiction rejection before provider traffic, static compatibility, public compatibility, and Google/credential-derived flavor rejection.
- [X] T027 [P] [US2] Write seven isolated Ginkgo `It()` blocks in `tests/e2e/thirdparty_cimd_metadata_test.go`: exact client ID, callback/method/JWK fields, public-only output, non-CIMD non-exposure, JSON/cache headers, canonical `404` for unknown, deleted, public, static, and unusable service states, and disjoint CIMD/token `kid` sets.
- [X] T028 [P] [US3] Write eight isolated Ginkgo `It()` blocks in `tests/e2e/thirdparty_cimd_authentication_test.go`: broker URL authorization identity, code assertion, refresh assertion, fail-closed errors, public/static compatibility, exact assertion form and claims, proxy isolation, and no cross-domain signing.
- [X] T029 [P] [US4] Write eight isolated Ginkgo `It()` blocks in `tests/e2e/thirdparty_cimd_rotation_test.go`: stable published location, cached-key refresh, no-usable-key failure, grace publication, immediate promotion, immediate first-key activation, all-mode route isolation, and non-ES256 rejection.
- [X] T030 [US5] After T026 has completed its edit of `tests/e2e/thirdparty_cimd_service_test.go`, add three isolated Ginkgo `It()` blocks: list all three postures, read a CIMD service without a secret, and convert static confidential state to CIMD while removing its secret.

### SC-008 Performance Measurement [MANDATORY]

- [X] T087 [P] [SC-008] Write `tests/e2e/thirdparty_cimd_metadata_performance_test.go` with a nearby SC-008 reference and `Label("cimd-upstream-client", "performance")`. Warm each route once, send 100 anonymous requests to each route with ten concurrent clients, require `200 OK` from every request, and assert a p95 below one second for each route.
- [X] T031 After T087, run `ginkgo -v --label-filter="cimd-upstream-client && !performance" ./tests/e2e/`; record that it selects all and only the 31 feature scenarios, and that they compile and fail semantically because feature behavior is absent, never because of placeholders, skips, or compilation failures. Then run `ginkgo -v --procs=1 --label-filter="cimd-upstream-client && performance" ./tests/e2e/`; record that the SC-008 test compiles and fails semantically before metadata behavior exists.
>
> Red-phase result: 31 feature scenarios were selected. Twenty-nine feature-dependent scenarios failed semantically; the two pre-existing static/public compatibility scenarios passed unchanged.

**Checkpoint**: Every acceptance scenario has exactly one production-bootstrap E2E assertion, the 31 functional scenarios are semantic red, and the separate SC-008 measurement is semantic red.

---

## Phase 2.5: Foundational Infrastructure (Blocking Prerequisites)

**Purpose**: Add the reusable outbound-CIMD key boundary and startup coordination that all user stories require. This phase does not expose CIMD HTTP behavior yet.

- [X] T032 Define the completed narrow CIMD ports and typed failure sentinels in `internal/ports/cimd_client.go`. Then define only the compile-time CIMD contracts that T033, T034, T049, T050, T056, and T063 need in `internal/domain/cimdclient/contracts.go`, `internal/domain/encryption/subject.go`, `internal/app/builder.go`, `internal/adapters/http/handlers/admin/cimd_client_keys_handler.go`, and `internal/adapters/http/handlers/enduser/cimd_metadata_handler.go`. Add only types, interface signatures, constructors, and inert paths. Do not implement key lifecycle, signing, metadata, readiness, or HTTP behavior.
- [X] T033 [P] After T032, write semantic-red key-domain tests in `internal/domain/cimdclient/key_service_test.go` and `internal/domain/encryption/subject_test.go` for ES256-only selection, a distinct `cimd_client_authentication` branch-key subject, one-subject encryption context, public-only JWKs, no token-key use, and typed failure sentinels for unavailable keys and failed public-key readiness.
- [X] T034 [P] After T032, write semantic-red startup and dependency-wiring tests in `internal/app/builder_test.go` for all three OAuth server modes: no-CIMD-service startup remains available without a key; persisted CIMD services bootstrap an immediately usable published first key; bootstrap failure fails closed; and registration does not create a key.
- [X] T035 Implement the CIMD key branch-key namespace in `internal/domain/encryption/subject.go` from T032's inert declaration. Retain exactly one AAD subject key (`kid`) and add no secret, service ID, or mixed-subject context.
- [X] T036 Implement `internal/domain/cimdclient/key_service.go` and `internal/domain/cimdclient/errors.go` over the domain-scoped signing-key repository: only `ES256` is generated or selected; `PrivateKeyEncrypted` is never serialized; a bootstrap or operator-provisioned first key is active immediately; a later generated key is published during grace; operator promotion is immediate; removal preserves the last/current/effective-current guards; and lifecycle success and rejection audits record only key ID, operation, and outcome.
- [X] T037 Construct the CIMD key manager and readiness dependency in `internal/app/builder.go` before the proxy/local/hybrid split. When persisted CIMD services exist, call the CIMD equivalent of `EnsureInitialKey` and fail closed if it cannot produce or publish the key. When none exist, create no key. Registration must not create a key as a side effect.
- [X] T038 Verify the shared foundation with `internal/domain/cimdclient/key_service_test.go`, `internal/domain/encryption/subject_test.go`, `internal/app/builder_test.go`, and the key-domain tests from Phase 0 before starting user-story code.

**Checkpoint**: A separate, encrypted, ES256-only CIMD key domain exists behind ports and is available in all modes without changing token-key routes or the aggregated token JWKS.

**Foundation order**: T032 → T033/T034 → T035 → T036 → T037 → T038.

---

## Phase 3: User Story 1 — Register a CIMD Confidential Service (Priority: P1) 🎯 MVP

**Goal**: Let an operator explicitly create or fully replace a third-party service as a CIMD confidential `private_key_jwt` client without supplying a client identifier or shared secret.

**Independent Test**: Register a service with broker-managed confidential authentication and no client identifier or shared secret; read it back and confirm the stable broker-hosted URL, `private_key_jwt`, and no stored shared secret.

- [X] T039 [P] [US1] Add table-driven semantic-red model coverage in `internal/domain/model/token_endpoint_auth_method_test.go` and `internal/domain/model/thirdparty_oauth2_provider_test.go` for the three-state matrix: omitted/null static confidential, `none` public, and explicit `private_key_jwt` CIMD confidential.
- [X] T040 [P] [US1] Add semantic-red lifecycle and audit coverage in `internal/domain/thirdparty/service_test.go` for validation before endpoint discovery, generated identity after service-ID allocation, complete-replacement validation before secret removal, absent-secret non-encryption/non-decryption, and credential-free audit records.
- [X] T041 [P] [US1] Add semantic-red API cases in `internal/adapters/http/handlers/admin/services_handler_test.go` for create, get, and update: reject a caller client ID or non-empty secret with `400`; reject Google; omit rather than redact CIMD secrets; and retain static/public compatibility.
- [X] T042 [P] [US1] Add storage round-trip and update-transition coverage in `internal/adapters/storage/memory/thirdparty_provider_test.go`, `internal/adapters/storage/postgres/thirdparty_provider_test.go`, and `tests/integration/storage/infra/thirdparty_service_test.go` for generated client IDs and absent CIMD secrets.
- [X] T043 [US1] Extend `internal/domain/model/token_endpoint_auth_method.go` and `internal/domain/model/thirdparty_oauth2_provider.go` with explicit `private_key_jwt` predicates and validation. Enforce verbatim model constraints: `Endpoints.TokenEndpoint` is an “HTTPS URI” and “The exact assertion audience”; `ClientID` is broker-written; `Secret` “Must be absent”; and credential-derived flavors reject this mode.
- [X] T044 [US1] Update `internal/domain/thirdparty/service.go` so CIMD create/update validates the full replacement before mutating state, requires a ready public CIMD key, preserves the existing client ID across ordinary updates, removes a prior encrypted secret only after validation succeeds, never encrypts/decrypts an absent CIMD secret, and emits safe service-ID/outcome audit events.
- [X] T045 [US1] Update `internal/adapters/http/handlers/admin/services_handler.go` to parse the explicit method, reject contradictions before discovery/provider traffic, allocate the immutable service ID before assigning `<enduser-public-url>/.well-known/oauth-client/<service-id>`, enforce the HTTPS base URL, and expose the method and generated URL without a `client_secret` field.
- [X] T046 [US1] Update `internal/adapters/storage/memory/thirdparty_provider.go`, `internal/adapters/storage/memory/thirdparty_provider_record.go`, `internal/adapters/storage/postgres/thirdparty_provider.go`, and `internal/adapters/storage/postgres/thirdparty_provider_record.go` to persist `private_key_jwt`, the generated existing `client_id` column value, and an absent secret without reinterpreting existing public or static rows.
- [X] T047 [US1] Inject CIMD readiness into the service path through `internal/app/builder.go` and retain thin handler construction in `internal/app/handlers.go`; do not instantiate domain services in routing.
- [X] T048 [US1] Turn `US1-S1` through `US1-S5` green in `tests/e2e/thirdparty_cimd_service_test.go` while preserving the public-client scenarios in `tests/e2e/thirdparty_public_client_test.go`.

**Checkpoint**: An operator can safely register, read, and update the distinct CIMD posture without changing existing static or public behavior.

---

## Phase 4: User Story 2 — Third-Party Service Discovers Broker Metadata (Priority: P1)

**Goal**: An authorization server can anonymously retrieve a stable public Client ID Metadata Document and CIMD-only JWK Set for an existing, non-deleted CIMD confidential service with a usable published key.

**Independent Test**: Fetch the configured service's client identifier URL anonymously and verify an exact matching client ID, callback URI, `private_key_jwt`, and only public verification keys.

- [X] T049 [P] [US2] Using the compile-time contracts from T032, add semantic-red metadata and JWK composition coverage in `internal/domain/cimdclient/metadata_service_test.go` for the exact metadata fields, public ES256 filtering, grace-period/publication behavior, stable URLs, and unknown, deleted, public, static, and unusable service states.
- [X] T050 [P] [US2] Using the compile-time contracts from T032, add semantic-red anonymous HTTP coverage in `internal/adapters/http/handlers/enduser/cimd_metadata_handler_test.go` for `application/json`, `Cache-Control: public, max-age=300`, the canonical error JSON `404`, no redirects, and absence of private material.
- [X] T051 [US2] Implement a metadata-safe third-party service projection in `internal/ports/cimd_client.go` and `internal/domain/thirdparty/service.go`; it may reveal only ID, authentication mode, generated client ID, callback inputs, and document eligibility. It must never reveal secret state or decrypted credentials.
- [X] T052 [US2] Implement `internal/domain/cimdclient/metadata_service.go` to derive a document only for an existing, non-deleted, ready `private_key_jwt` service. Return exactly one callback URI, grant types `authorization_code` and `refresh_token`, response type `code`, auth method `private_key_jwt`, signing algorithm `ES256`, and `<client-id>/jwks.json`; publish only CIMD-domain public P-256 keys with `use: sig`, `kid`, `alg`, `x`, and `y`.
- [X] T053 [US2] Implement anonymous document and key handlers in `internal/adapters/http/handlers/enduser/cimd_metadata_handler.go`, mapping unknown, deleted, public, static, and unusable states to the same existing JSON `404` and emitting metadata-access audit records with identifiers and outcomes only.
- [X] T054 [US2] Add the pre-wired CIMD public handler fields in `internal/app/handlers.go`, construct them in `internal/app/builder.go`, and register both `/.well-known/oauth-client/{service-id}` routes before the SPA fallback in `internal/adapters/http/routing/enduser.go`.
- [X] T055 [US2] Turn `US2-S1` through `US2-S7` green in `tests/e2e/thirdparty_cimd_metadata_test.go`, including proof that `/oauth2/jwks.json` remains free of CIMD key IDs.

### SC-008 Performance Measurement

- [X] T088 [SC-008] Turn `tests/e2e/thirdparty_cimd_metadata_performance_test.go` green. Each anonymous route must return `200 OK` for the defined workload and meet the one-second p95 target.

**Checkpoint**: The public discovery documents are stable, anonymous, cacheable, and disclose only the key material needed to validate client assertions. The SC-008 measurement meets its p95 target.

---

## Phase 5: User Story 3 — Connect with CIMD Confidential Authentication (Priority: P1)

**Goal**: The existing authorization-code and refresh journeys authenticate a CIMD service with a fresh signed `private_key_jwt` assertion while retaining PKCE and never sending a secret.

**Independent Test**: Complete connect and refresh against the conformant upstream double, which retrieves metadata/JWKS, accepts valid assertions, rejects shared-secret authentication, and confirms encrypted session replacement.

- [X] T056 [P] [US3] Using the compile-time contracts from T032, add semantic-red signer coverage in `internal/domain/cimdclient/assertion_signer_test.go`: protected `alg` is `ES256`; `kid` belongs to the public CIMD JWK Set; `iss` and `sub` exactly equal the service URL; `aud` has one configured token-endpoint URL; `exp` is no more than five minutes after `iat`; and every outbound attempt has a fresh unique `jti`.
- [X] T057 [P] [US3] Add semantic-red exchange and refresh security coverage in `internal/domain/oauth2session/service_security_test.go` and `internal/domain/oauth2session/service_test.go` for one assertion pair per request, per-retry re-signing, `AuthStyleInParams`, no `ClientSecret` or Authorization header, no downgrade, and byte-for-byte compatible public/static behavior.
- [X] T058 [US3] Implement `internal/domain/cimdclient/assertion_signer.go` using JWX v4 and the CIMD key port; sign only with a currently usable advertised ES256 CIMD key, keep private key bytes inside the key service, and return a fail-closed error if selection, decryption, or signing cannot complete.
- [X] T059 [US3] Extend `internal/domain/oauth2session/dependencies.go` and `internal/domain/oauth2session/service.go` so CIMD code exchange uses an empty secret, `oauth2.AuthStyleInParams`, and exactly one JWT-bearer assertion pair inside every retry; make the manual refresh form mint a new assertion each request; preserve PKCE S256; and never fall back to public or static credentials.
- [X] T060 [US3] Wire the assertion signer through `internal/app/builder.go` and add credential-free signing, rejection, and token-acquisition audit events in `internal/domain/cimdclient/assertion_signer.go` and `internal/domain/oauth2session/service.go`.
- [X] T061 [US3] Turn `US3-S1` through `US3-S8` green in `tests/e2e/thirdparty_cimd_authentication_test.go`, including proxy-mode key isolation and encrypted session refresh replacement.

**Checkpoint**: A CIMD service can complete and refresh a user session only with a correctly formed, separately signed client assertion.

---

## Phase 6: User Story 4 — Maintain Continuity During Key Rotation (Priority: P2)

**Goal**: Operators can manage the dedicated CIMD key lifecycle in every OAuth server mode without making token-signing keys or routes reachable through the CIMD surface.

**Independent Test**: Rotate a CIMD key, observe it in the public CIMD JWK Set before use, and complete a validated CIMD token request after activation or promotion.

- [X] T062 [P] [US4] Add semantic-red lifecycle cases in `internal/domain/cimdclient/key_service_test.go` for immediate activation of the first bootstrap key and the first operator-generated key, generated-key grace overlap, immediate operator promotion, key-removal guards, retirement publication behavior, ES256-only enforcement, and credential-free audit records for lifecycle success and rejection.
- [X] T063 [P] [US4] Using the compile-time contracts from T032, add semantic-red administrative HTTP cases in `internal/adapters/http/handlers/admin/cimd_client_keys_handler_test.go` for create/list/promote/remove, immediate first-key activation, operator authentication, `400` invalid algorithms, `404` unknown keys, `409` removal guards, credential-free audit records for successful and rejected lifecycle actions, and never serializing private bytes, ciphertext, branch-key IDs, or assertion data.
- [X] T064 [P] [US4] Add real PostgreSQL lifecycle coverage in `tests/integration/infra/oauth2_signing_key_bootstrap_test.go` and `tests/integration/storage/infra/postgres_test.go` for conditional bootstrap, per-domain locks/indexes, active-current isolation, rollback-safe state, and global `kid` uniqueness across both domains.
- [X] T065 [US4] Implement the flat CIMD key handler in `internal/adapters/http/handlers/admin/cimd_client_keys_handler.go` by reusing the CIMD key port and canonical administrative error representation. Accept an optional `algorithm` that defaults to `ES256` and reject every other value before key creation. Preserve the key service's credential-free lifecycle audit events.
- [X] T066 [US4] Add `CIMDClientKeys` to `internal/app/handlers.go`, construct it in `internal/app/builder.go`, and register `POST/GET /api/cimd-client-keys`, `PUT /api/cimd-client-keys/{kid}/current`, and `DELETE /api/cimd-client-keys/{kid}` in `internal/adapters/http/routing/admin.go` for proxy, local, and hybrid modes while leaving `/api/oauth2-server/signing-keys` unavailable in proxy mode.
- [X] T067 [US4] Turn `US4-S1` through `US4-S8` green in `tests/e2e/thirdparty_cimd_rotation_test.go`, including immediate bootstrap first-key activation, publication-before-use, cache refresh, all-mode route isolation, and no usable-key fail-closed behavior.
**Checkpoint**: CIMD rotation is operator-triggered, ES256-only, observable before use, and strictly separated from broker token-signing lifecycle and routes.

---

## Phase 7: User Story 5 — Review Authentication Posture (Priority: P3)

**Goal**: Operators can distinguish public, static confidential, and CIMD confidential services through existing read/list responses without ever receiving a secret.

**Independent Test**: List all three postures and read a CIMD service; every response reports the explicit method, only CIMD exposes the broker-hosted URL, and no secret is revealed.

- [X] T068 [P] [US5] Add read/list posture-matrix assertions to `internal/adapters/http/handlers/admin/services_handler_test.go` for static `null` with redaction, public `none` with no secret field, and CIMD `private_key_jwt` with its immutable broker URL and no secret field.
- [X] T069 [P] [US5] Add credential-free representation and transition-audit assertions to `internal/domain/thirdparty/service_test.go` for static-to-CIMD replacement, CIMD-to-static requiring a new non-empty secret, and CIMD-to-public removing secret state.
- [X] T070 [US5] Turn `US5-S1` through `US5-S3` green in `tests/e2e/thirdparty_cimd_service_test.go` and re-run the US1 service scenarios to prove posture review does not regress the shared US1 service representation.

**Checkpoint**: The existing service API gives operators a complete, unambiguous, non-secret view of every authentication posture.

---

## 🔒 Phase N: Constitution Compliance & Polish [MANDATORY COMPLIANCE SECTION]

**Purpose**: Verify every design and implementation principle with the completed contracts, migrations, code, documentation, and tests before feature completion.

### Design Phase Verification [MANDATORY]

- [X] T071 Verify Principles II and V against `ARCHITECTURE.md`, `adrs/037-cimd-client-authentication-key-domain.md`, and `specs/046-cimd-upstream-client/data-model.md`: terminology, invariants, route inventory, key separation, accepted ADR status, and the SC-008 load profile and p95 target must agree.
- [X] T072 [P] Verify Principle VII against `docs/configuration.md`, `examples/config/third-party-oauth2.yaml`, `examples/config/README.md`, `internal/ports/config.go`, and `charts/agentic-identity-broker/values.yaml`; HTTPS is documented and no new config/Helm value was silently introduced.
- [X] T073 [P] Verify Principles IV and X against `api/admin/openapi.yaml`, `api/enduser/openapi.yaml`, `specs/046-cimd-upstream-client/contracts/admin-api.md`, and `specs/046-cimd-upstream-client/contracts/enduser-api.md`; implementation must match the confirmed contracts exactly.
- [X] T074 [P] Verify Principles IX and XIII against `migrations/032_add_cimd_key_domain.up.sql`, `migrations/032_add_cimd_key_domain.down.sql`, `migrations/033_add_cimd_private_key_jwt_authentication.up.sql`, `migrations/033_add_cimd_private_key_jwt_authentication.down.sql`, and the four `tests/e2e/thirdparty_cimd_*_test.go` files; all migrations and all 31 scenario mappings must be present.
- [X] T075 Verify Principle XI is intentionally not applicable by confirming `web/` has no feature change and `specs/046-cimd-upstream-client/plan.md` records the no-frontend decision.
- [X] T076 [P] Verify Principle I in `internal/domain/cimdclient/key_service.go`, `internal/domain/cimdclient/assertion_signer.go`, `internal/domain/oauth2session/service.go`, and their `_security_test.go` or `_test.go` files: key, metadata, and assertion failures are enabled by default and fail closed; successful and rejected CIMD key lifecycle operations are auditable; and structured security audit records include only identifiers and outcomes.
- [X] T077 [P] Verify Principle III in `internal/domain/cimdclient/assertion_signer.go`, `internal/domain/cimdclient/key_service.go`, and `internal/domain/cimdclient/assertion_signer_test.go`: all ES256 operations delegate to Go `crypto/ecdsa` and JWX v4, with no custom cryptographic implementation.
- [X] T078 [P] Update and build the confirmed API/operator documentation under Principles IV and X in `docs/reference/api.md`, `docs/guides/manage-agents-and-services.md`, and `docs/guides/operate-oauth2-server-modes.md`; run `just docs-build` from the root OpenAPI contracts.
- [X] T079 [P] Verify Principle VII in `internal/ports/config.go`, `internal/config/validator.go`, `docs/configuration.md`, and `charts/agentic-identity-broker/values.yaml`: all behavior uses the existing unified ConfigPort/public URL, and no ad hoc configuration or undocumented Helm input exists.
- [X] T080 [P] Verify Principle IX with `tests/integration/migrations/migrations_test.go`, `tests/integration/storage/infra/thirdparty_service_test.go`, `tests/integration/storage/infra/postgres_test.go`, and the `032`/`033` migration files: apply, guarded rollback, replay, adapter parity, and domain-scoped persistence pass against real PostgreSQL.
- [X] T081 [P] Verify Principle VI in `internal/ports/cimd_client.go`, `internal/domain/cimdclient/`, and `internal/adapters/storage/`: domain logic depends on narrow ports, adapters implement those ports, and no adapter-to-adapter or domain-to-infrastructure import was introduced.
- [X] T082 [P] Verify Principle VIII red-green unit and adapter testing in `internal/domain/cimdclient/*_test.go`, `internal/domain/oauth2session/service_security_test.go`, `internal/domain/thirdparty/service_test.go`, and `internal/adapters/http/handlers/admin/*_test.go`: tests compiled, failed semantically before implementation, use real observable assertions, and changed minimally after turning green.
- [X] T083 [P] Verify Principle VIII's no-Bash-correctness rule against `Justfile`, `tests/`, and `specs/046-cimd-upstream-client/quickstart.md`: code correctness is exercised by Go/Ginkgo/integration tests rather than bespoke shell assertions.
- [X] T084 [P] Verify Principle XII in `internal/app/builder.go`, `internal/app/handlers.go`, `internal/adapters/http/routing/admin.go`, and `internal/adapters/http/routing/enduser.go`: all services are constructed by the Builder, handlers are pre-wired, and routing only registers routes.
- [X] T085 [P] Verify Principle XIII in `tests/e2e/thirdparty_cimd_service_test.go`, `tests/e2e/thirdparty_cimd_metadata_test.go`, `tests/e2e/thirdparty_cimd_authentication_test.go`, and `tests/e2e/thirdparty_cimd_rotation_test.go`: every US1–US5 scenario has exactly one nearby `US#-S# from specs/046-cimd-upstream-client/spec.md` reference, every functional feature `It()` uses `Label("cimd-upstream-client")`, and the non-performance label-filtered suite runs all 31 scenarios through production bootstrap. Verify SC-008 in `tests/e2e/thirdparty_cimd_metadata_performance_test.go` separately.
- [X] T086 Run the final non-Bash verification commands from `specs/046-cimd-upstream-client/quickstart.md`: `just check`, `just test`, `just test-integration`, `just test-integration-infra`, `ginkgo -v --label-filter="cimd-upstream-client && !performance" ./tests/e2e/`, and `ginkgo -v --procs=1 --label-filter="cimd-upstream-client && performance" ./tests/e2e/`. Record the p95 for each anonymous route against SC-008.

**Checkpoint**: The feature is complete only when contracts, documentation, architecture, migrations, security, DI boundaries, and all mandatory tests are green.

---

## Dependencies & Execution Order

```text
Phase 0 (accepted ADR + token-key refactor)
  → Phase 1 (isolated test infrastructure)
  → Phase 2 (design preconditions and semantic-red E2E suite)
  → Phase 2.5 (CIMD key-domain foundation)
  → US1 (service registration)
  ├→ US2 (public metadata and CIMD JWKS)
  │   ├→ US3 (connect and refresh with private_key_jwt)
  │   └→ US4 (rotation continuity and all-mode key administration)
  └→ US5 (read/list posture review)
  → Phase N (constitution compliance and polish)
```

### User-story dependencies

- **US1** depends on the key-domain foundation because registration must reject when public CIMD key publication is unavailable.
- **US2** depends on US1 because documents exist only for an existing, non-deleted CIMD confidential service with a usable published key.
- **US3** depends on US2 because the conformant provider validates the advertised metadata/JWK material during the end-to-end journey.
- **US4** depends on US2 because rotation continuity is observed through the service's public JWK URL.
- **US5** depends on US1 because it reviews the service representation introduced at registration.

### Within each story

1. Complete the story's test tasks first and observe semantic red failures.
2. Implement model/domain behavior before adapters and HTTP handlers.
3. Wire services only in `internal/app/builder.go`; routing only registers pre-wired handlers.
4. Turn that story's 1:1 E2E scenarios green before advancing.

---

## Parallel Execution Examples

### User Story 1

```text
After Phase 2.5, run T039, T040, T041, and T042 in parallel.
Then sequence T043 → T044 → T045 → T046/T047 → T048.
```

### User Story 2

```text
After US1, run T049 and T050 in parallel.
Then sequence T051 → T052 → T053 → T054 → T055 → T088.
```

### User Story 3

```text
After US2, run T056 and T057 in parallel.
Then sequence T058 → T059 → T060 → T061.
```

### User Story 4

```text
After US2, run T062, T063, and T064 in parallel.
Then sequence T065 → T066 → T067.
```

### User Story 5

```text
After US1, run T068 and T069 in parallel.
Then run T070; it exercises the shared US1 mapper and adds no speculative production change.
```

---

## Implementation Strategy

### MVP first

1. Complete Phase 0 as a separate, behavior-preserving key-domain refactoring delivery.
2. Complete Phase 1, all Phase 2 design preconditions, and Phase 2.5; do not begin production behavior without the 31 red E2E scenarios.
3. Deliver US1 and validate registration/read behavior independently.
4. Deliver US2 so third-party authorization servers can discover public metadata and keys.
5. Deliver US3 and validate the complete connect-and-refresh journey. This is the P1 end-user MVP.

### Incremental delivery

1. Add US4 after the P1 flow to expose safe operator-controlled rotation and mode-independent key administration.
2. Add US5 to complete the operator posture-review experience using the one shared service representation.
3. Complete Phase N only after all desired stories are green; do not trade security isolation or migration safety for incremental progress.

### Guardrails

- Use Go `crypto/ecdsa` and JWX v4; do not implement cryptographic primitives.
- Use `internal/ports/` interfaces and both in-memory/PostgreSQL adapters; do not bypass storage ports.
- Use existing JSON error contracts and structured logs with only identifiers/outcomes.
- Never add CIMD keys to `/oauth2/jwks.json`, reuse proxy credentials, expose private material, or downgrade authentication after an assertion/key error.
