---

description: "Task list for 042-thirdparty-public-pkce implementation"
---

# Tasks: Public Client Support for Third-Party OAuth2 Services

**Input**: Design documents from `/specs/042-thirdparty-public-pkce/`

**Prerequisites**: [plan.md](./plan.md), [spec.md](./spec.md), [research.md](./research.md),
[data-model.md](./data-model.md), [contracts/admin-api.md](./contracts/admin-api.md),
[contracts/upstream-token-requests.md](./contracts/upstream-token-requests.md),
[quickstart.md](./quickstart.md)

**Tests**: Per Constitution Principle VIII, automated tests are MANDATORY. Test tasks appear before
implementation tasks in every phase and MUST fail semantically before the implementation they cover.

**Organization**: Tasks are grouped by user story so each story can be implemented and tested
independently.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Can run in parallel (different files, no dependencies on incomplete tasks)
- **[Story]**: Which user story the task belongs to (US1, US2, US3, US4)
- Every task names its exact file path

## Path Conventions

Existing hexagonal Go backend at repository root: `internal/domain/`, `internal/ports/`,
`internal/adapters/`, `migrations/`, `api/`, `tests/e2e/`, `tests/integration/`. No new package,
module, or frontend work.

---

## Phase 0: Pre-implementation Refactoring — SKIPPED

Per [plan.md](./plan.md#implementation-phase-overview): no rename, restructure, or interface
extraction is required. Every change is additive to existing types and no existing test needs
modification. The one naming divergence (`ThirdpartyOAuth2ProviderEntity` versus the specification's
`ThirdpartyOAuth2Service`) predates this feature; renaming it would inflate the diff across dozens of
unrelated files.

---

## Phase 1: Setup — SKIPPED

No new dependency, module, or tool. `golang.org/x/oauth2` v0.36.0, `chi` v5, `sqlx`/`pgx` v5,
`golang-migrate` v4.19.1, and Ginkgo/Gomega v2 are already present. Linting and formatting are
already configured behind `just check`.

---

## 🔒 Phase 2: Design Preconditions (Blocking Prerequisites) [MANDATORY]

**Purpose**: Domain model, configuration, API, database, and E2E acceptance test design MUST all be
complete before any implementation begins.

**⚠️ CRITICAL**: No production code behaviour may be implemented until this entire phase is complete.

### Phase 2a: Domain Model & Glossary [MANDATORY]

**Constitution Reference**: Principles II (Architecture Documentation), V (Domain-Driven Design)

- [X] T001 Add the `TokenEndpointAuthMethod`, **Public client**, and **Confidential client** entries
      to the Glossary in `ARCHITECTURE.md` and amend the existing **Secret** entry to read
      "exclusive plaintext, encrypted, or absent state", per
      [data-model.md §8](./data-model.md#8-glossary-additions-architecturemd)
- [X] T002 Document the two upstream client-authentication modes in `ARCHITECTURE.md`: confidential
      services keep `AuthStyleAutoDetect`, public services pin `AuthStyleInParams` with an empty
      secret, and PKCE `S256` applies unconditionally to both
- [X] T003 Write `adrs/036-public-client-token-endpoint-auth.md` (Status: Accepted) recording the
      four decisions from [research.md §D11](./research.md#d11--architectural-record): absence is
      the confidential state with no default; `none` is the only accepted value; `AuthStyleInParams`
      is pinned for public exchanges while `AutoDetect` is preserved for confidential ones; and
      `Secret` gains an explicitly constructed absent state that supersedes the two-state decision in
      ADR 012. Include the ADR 017 representation precedent, the deliberate divergence from ADR 017's
      update semantics, and the operator runbook for the guarded rollback (convert or remove the named
      public services, `migrate force 31`, retry)
- [X] T004 Add an "Amended by ADR 036" note to the two-state `Secret` section of
      `adrs/012-encryption-layer-separation.md` (lines 32-43), stating that ADR 036 supersedes that
      decision, plaintext remains impossible to persist, and absence is reachable only through
      `NewAbsentSecret()`

**Checkpoint**: Domain model documented, ADR 036 written, ADR 012 amendment recorded

### Phase 2b: Configuration Design [MANDATORY]

**Constitution Reference**: Principle VII (Configuration-Driven Design)

- [X] T005 Assert the Principle VII trigger is not met by running
      `git diff --stat main -- internal/config/schema.go internal/ports/config.go docs/configuration.md examples/config/ charts/agentic-identity-broker/`
      and confirming it reports no changed files: third-party services are registered through the
      administrative API, not startup configuration, so no `values.yaml`, ConfigMap template, or
      config example is added

**Checkpoint**: No configuration surface changes; Helm chart untouched by design

### Phase 2c: API Design [MANDATORY]

**Constitution Reference**: Principles IV (OpenAPI Transparency), X (API-First Development)

> API-010 was cleared by the stakeholder on 2026-09-10
> ([contracts/admin-api.md §8](./contracts/admin-api.md#8-confirmation-record-api-010)). These tasks
> apply the confirmed delta verbatim.

- [X] T006 Update the `Service` read/list schema in `api/admin/openapi.yaml` (~lines 2166-2290): add
      `token_endpoint_auth_method` to `required`, remove `client_secret` from `required`, and add the
      property as `type: string`, `nullable: true`, `enum: [none, null]`, `readOnly: true` per
      [contracts/admin-api.md §1.3](./contracts/admin-api.md#13-add-token_endpoint_auth_method)
- [X] T007 Amend the `Service.client_secret` description in `api/admin/openapi.yaml` (~lines
      2207-2220): replace the `Always returns "REDACTED"` sentence with the confidential/public
      wording from [contracts/admin-api.md §1.2](./contracts/admin-api.md#12-amend-client_secret-currently-lines-2207-2220)
- [X] T008 Update `ServiceCreateRequest` in `api/admin/openapi.yaml` (~lines 2295-2390): remove
      `client_secret` from `required`, remove its `minLength: 1`, add `nullable: true`, append the
      conditional-requirement description, and add `token_endpoint_auth_method` with
      `enum: [none, null]` per [contracts/admin-api.md §2](./contracts/admin-api.md#2-servicecreaterequest)
- [X] T009 Update `ServiceUpdateRequest` in `api/admin/openapi.yaml` (~lines 2398-2487) with the same
      three changes plus the full-replacement wording from
      [contracts/admin-api.md §3](./contracts/admin-api.md#3-serviceupdaterequest)
- [X] T010 Update the four path descriptions in `api/admin/openapi.yaml` — `POST /api/services`
      (~526-547), `GET /api/services` (~454-470), `GET /api/services/{service-id}` (~634-636), and
      `PUT /api/services/{service-id}` (~692-708) — per
      [contracts/admin-api.md §4](./contracts/admin-api.md#4-path-descriptions)
- [X] T011 [P] Amend the authorize (~751-818) and callback (~819-921) endpoint descriptions in
      `api/enduser/openapi.yaml` to state that a client credential is presented only for confidential
      services while PKCE applies to every service; paths, parameters, and responses stay unchanged
      (API-008)
- [X] T012 [P] Record the `Service` read-schema `required` relaxation (`client_secret` no longer
      unconditionally required) in `docs/changelog.md`

**Checkpoint**: `api/admin/openapi.yaml` matches contracts/admin-api.md exactly; end-user document
carries description-only amendments

### Phase 2d: Database Design [MANDATORY]

**Constitution Reference**: Principle IX (Persistence & Migration Management)

- [X] T013 Create `migrations/031_add_token_endpoint_auth_method.up.sql`: drop `NOT NULL` on
      `thirdparty_oauth2_services.client_secret_encrypted`, add
      `token_endpoint_auth_method VARCHAR(32)` nullable with **no** `DEFAULT` and **no** backfill,
      and add `chk_thirdparty_oauth2_services_client_auth` enforcing
      `(method IS NULL AND ciphertext IS NOT NULL) OR (method IS NOT DISTINCT FROM 'none' AND ciphertext IS NULL)` per
      [data-model.md §6](./data-model.md#6-persistence-schema)
- [X] T014 Create `migrations/031_add_token_endpoint_auth_method.down.sql`: a
      `DO $$ … RAISE EXCEPTION` guard that aborts and names the blocking services when any row has a
      non-null `token_endpoint_auth_method`, followed by dropping the constraint, dropping the
      column, and restoring `NOT NULL` — following the guard precedent in
      `migrations/028_normalize_service_protected_resources.up.sql` (DB-006)
- [X] T015 Verify the committed SQL in `migrations/031_add_token_endpoint_auth_method.up.sql` and
      `.down.sql` matches the documented column list, constraint name, and rollback guard in
      [data-model.md §6](./data-model.md#6-persistence-schema)

**Checkpoint**: Migration pair `031` committed, sequential after `030`, both directions present

### Phase 2e: Frontend/Design System Review [NOT APPLICABLE]

- [X] T016 Assert the Principle XI trigger is not met by running
      `git diff --stat main -- web/ tests/e2e/frontend/ tests/e2e/screenshots/` and confirming it
      reports no changed files: no administrative UI exists for third-party service registration and
      the end-user session pages are unaffected, so no Playwright test and no screenshot is added

**Checkpoint**: Frontend scope explicitly confirmed empty

### Phase 2f: E2E Acceptance Test Design [MANDATORY]

**Constitution Reference**: Principle XIII (E2E Acceptance Testing & Spec Traceability)

- [X] T017 Extend `tests/e2e/helpers/mock_upstream.go` with an opt-in strict public-client mode
      (default off, so no existing suite changes behaviour): `/authorize` records `code_challenge`
      and `code_challenge_method` keyed by `state`; `/token` returns `401 invalid_client` when an
      `Authorization` header or a `client_secret` form value is present, and `400 invalid_grant` when
      `code_verifier` does not `S256`-hash to the recorded challenge, per
      [contracts/upstream-token-requests.md §5](./contracts/upstream-token-requests.md#5-test-double-conformance)
- [X] T019 Add compile-level scaffolding ONLY so the E2E and unit tests compile and fail
      semantically: `internal/domain/model/token_endpoint_auth_method.go` declaring the type and
      `TokenEndpointAuthMethodNone`; the `absent` field with `NewAbsentSecret()` and `IsAbsent()` in
      `internal/domain/model/secret.go`; and the `TokenEndpointAuthMethod` field plus
      `IsPublicClient()` in `internal/domain/model/thirdparty_oauth2_provider.go`. No validation,
      encryption, persistence, or request-building behaviour — those land in Phase 2.5 and the story
      phases
- [X] T018 Add `PublicClientService` to `tests/e2e/fixtures/services.go` after T019: return the
      production domain type with a fresh `id.ServiceID` per call, `TokenEndpointAuthMethod: "none"`,
      `Secret: model.NewAbsentSecret()`, deterministic configuration, and no file or network access.
      During Phase 2f, use this fixture only to compile the red-phase tests. T039 and T042 establish
      the public-client validation invariant before an E2E scenario depends on validation.
- [X] T020 Write `tests/e2e/thirdparty_public_client_test.go` with the top-level
      `Describe("Public Client Support for Third-Party OAuth2 Services")`, one `Context` per user
      story, and 22 `It()` blocks matching the scenario table in
      [plan.md Testing Strategy](./plan.md#testing-strategy) one-to-one (US1×6, US2×6, US3×4, US4×6)
- [X] T021 Add the traceability comment `// USx-Sy from specs/042-thirdparty-public-pkce/spec.md`
      immediately above every `It()` in `tests/e2e/thirdparty_public_client_test.go`
      (`tests/e2e/AGENTS.md` rule 2)
- [X] T022 In `tests/e2e/thirdparty_public_client_test.go`, construct a per-test
      `MockUpstreamOAuth2Server` in strict mode inside `BeforeEach` for the US2 and US3 contexts
      instead of the suite-wide double from `e2e_suite_test.go`, and bootstrap with
      `NewAdminTestServer` for US1/US4 and `NewEndUserTestServer` for US2/US3, keeping setup inside
      each `It()` under 15 lines
- [X] T023 Verify the red phase for `tests/e2e/thirdparty_public_client_test.go`:
      `ginkgo -v --label-filter="!performance" --focus="Public Client" ./tests/e2e/` compiles and all
      22 fail semantically against concrete expectations (status codes, `has("client_secret")`,
      `GetLastBody()` form contents, `GetLastRequest().Header.Get("Authorization")`, stored session
      state); confirm no `XIt`/`PIt`/`XDescribe`/`PDescribe`/`XContext`/`PContext`/`Skip()` and no
      red-phase comments

**Checkpoint**: All 22 acceptance tests written, traceable, and failing semantically

---

## Phase 2.7: Entity Boilerplate — SKIPPED

No new domain entity, typed ID, port, storage adapter, or HTTP handler. `id.ServiceID` already exists
(`internal/domain/id/uuid_ids_gen.go:66-74`), so no `gen_ids.go` change is required (ADR 013).
`internal/app/builder.go` and `internal/adapters/http/routing/admin.go` are untouched: the handler
and service already exist and are already wired (Principle XII). Scaffolding would be empty.

---

## Phase 2.5: Foundational Infrastructure

**Purpose**: The absent `Secret` state, the `TokenEndpointAuthMethod` value object, migration `031`,
and both storage adapters — prerequisites shared by all four user stories.

**⚠️ CRITICAL**: No user story work can begin until this phase is complete.

### Tests (write first, verify failing)

- [X] T024 [P] Write table-driven tests in `internal/domain/model/secret_test.go` for the absent
      state: `NewAbsentSecret()` is the only constructor reaching it; `IsAbsent`/`IsPlaintext`/
      `IsEncrypted` for all four rows of the state table in
      [data-model.md §2](./data-model.md#2-value-object-secret-extended); `GetPlaintext()` and
      `GetCiphertext()` return an error naming the absent state; the zero value still reports
      plaintext
- [X] T025 [P] Write table-driven tests in `internal/domain/model/token_endpoint_auth_method_test.go`
      for `Validate()`: `""` accepted, `"none"` accepted, every other value rejected with a message
      naming `none` as the only accepted value (FR-002); plus `IsAbsent()`
- [X] T026 [P] Add `TestMigration031` to `tests/integration/migrations/migrations_test.go` covering
      the six-step sequence: apply `031` on a `030` database; assert no backfill (every pre-existing
      row has `token_endpoint_auth_method IS NULL`); assert the `CHECK` rejects a direct `INSERT` in
      both contradictory directions; `Down(30)` returns an error naming the blocking public service;
      `Version()` reports `dirty == true` and the column still exists; `Force(31)` clears the flag;
      after deleting the service `Down(30)` succeeds with `dirty == false` and the column gone
- [X] T027 [P] Extend `internal/adapters/storage/memory/thirdparty_provider_test.go` with the
      absent-credential round trip — a public entity is stored and returned with `Secret.IsAbsent()`
      true — while a plaintext secret is still rejected (DB-008)
- [X] T028 [P] Extend `tests/integration/storage/infra/thirdparty_service_test.go` with a public
      service round trip through the PostgreSQL repository (`token_endpoint_auth_method` persisted as
      `none`, `client_secret_encrypted` `NULL`, absent secret returned) and a confidential→public
      update that nulls the ciphertext column

### Implementation

- [X] T029 Implement `Validate()` and `IsAbsent()` on `TokenEndpointAuthMethod` in
      `internal/domain/model/token_endpoint_auth_method.go` following the `OAuth2Flavor` string-enum
      style but with **no default** — no value is ever inferred, assigned, or persisted (FR-003)
- [X] T030 Implement the absent state in `internal/domain/model/secret.go`: `IsPlaintext()` becomes
      `!s.absent && s.ciphertext == nil`, `IsEncrypted()` becomes `!s.absent && s.ciphertext != nil`,
      `GetPlaintext()`/`GetCiphertext()` error on absent, `Redacted()` and the zero-value meaning
      unchanged
- [X] T031 Add `Force(t *testing.T, version uint)` to `tests/integration/migrations/framework.go`
      wrapping `m.Force(int(version))` in the existing helper style, so the dirty flag left by a
      refused rollback can be cleared
- [X] T032 [P] Extend the record mapping in
      `internal/adapters/storage/postgres/thirdparty_provider_record.go`: add
      `TokenEndpointAuthMethod *string \`db:"token_endpoint_auth_method"\`` to
      `ThirdpartyOAuth2ProviderRecord`; in `entityToRecord` skip the `GetCiphertext()` call for an
      absent secret (leaving `SecretCiphertext` nil, not an error) and convert the domain value —
      `""` → nil, `"none"` → pointer to `"none"`; in `recordToEntity` convert back — nil → `""`,
      otherwise `model.TokenEndpointAuthMethod(*record.TokenEndpointAuthMethod)` — and select
      `model.NewAbsentSecret()` when `SecretCiphertext` is nil instead of the unconditional
      `model.NewEncryptedSecret(record.SecretCiphertext)` at line 187
- [X] T033 Add `token_endpoint_auth_method` to `providerColumns`, the `INSERT`, and the `UPDATE`
      column lists in `internal/adapters/storage/postgres/thirdparty_provider.go`
- [X] T034 [P] Accept an absent secret in `providerEntityCopy` in
      `internal/adapters/storage/memory/thirdparty_provider_record.go` while still rejecting a
      plaintext secret
- [X] T035 [P] Update the contract documentation in `internal/ports/thirdparty_provider.go` from
      "stored entities always carry an encrypted secret" to "encrypted **or absent**"; the interface
      method set stays exactly as it is (ISP unchanged)
- [X] T036 Add the absent state to the secret-state check in `Validate()` in
      `internal/domain/model/thirdparty_oauth2_provider.go`, which runs on entities loaded from
      storage
- [X] T037 Run `just test-integration-infra` and confirm `TestMigration031` and both repository round
      trips are green
- [X] T038 Run `just check` and
      `go test ./internal/domain/model/... ./internal/adapters/storage/...`

**Checkpoint**: The absent credential can be constructed, validated, persisted, and read back through
both adapters; migration `031` applies and refuses rollback correctly — user stories can begin

---

## Phase 3: User Story 1 - Register a public third-party service without a client secret (Priority: P1) 🎯 MVP

**Goal**: An operator registers a provider that issues no client secret, declaring
`token_endpoint_auth_method: none` and supplying no credential; the service is created, appears in
the catalogue marked public, and reports no credential value.

**Independent Test**: `POST /api/services` with `token_endpoint_auth_method: "none"` and no
`client_secret` returns 201; `GET /api/services` shows the entry with
`"token_endpoint_auth_method": "none"` and no `client_secret` property, while confidential entries
show `null` and `"REDACTED"`.

### Tests for User Story 1 [MANDATORY - Principle VIII] ⚠️

- [X] T039 [P] [US1] Write table-driven create and update validation tests in
      `internal/domain/model/thirdparty_oauth2_provider_test.go` for the six rules in
      [data-model.md §3.1](./data-model.md#31-validation-rules), both directions of each, asserting
      the google rejection fires before any credential-derived enrichment and that the confidential
      no-secret message is still `client_secret is required`
- [X] T040 [P] [US1] Write tests in `internal/domain/thirdparty/service_test.go` (hand-rolled mocks
      with function fields, never `testify/mock`) asserting `branchKeyManager.Create` still runs for a
      public create and update (FR-028), that encryption is skipped, and that `Get`/`List` return an
      absent secret with no error, warning, or degraded state (FR-009)
- [X] T041 [P] [US1] Write create-path tests in
      `internal/adapters/http/handlers/admin/services_handler_test.go` using the adapter conventions
      (`testify/mock` with `.On()`/`.AssertExpectations()`, helpers in `mocks_test.go`,
      `httptest.NewRecorder()`/`NewRequest()` — not the domain's hand-rolled style) covering every
      row of the mapping table in
      [research.md §D5](./research.md#d5--wire-format-presence-semantics): key absent, explicit
      `null`, `"none"` with an absent secret, `"none"` with `""`, `"none"` with a non-empty secret
      (400), and any other value (400). Assert the response shaping on the decoded JSON, not the
      struct: a public service has no `client_secret` key and
      `"token_endpoint_auth_method": "none"`; a confidential service has
      `"client_secret": "REDACTED"` and the key present with a literal `null`

### Implementation for User Story 1

- [X] T042 [US1] Implement validation rules 1-5 ahead of the existing flavor switch in
      `ValidateForCreate` and `ValidateForUpdate` in
      `internal/domain/model/thirdparty_oauth2_provider.go`, replacing the plaintext extraction for
      public services and leaving the remaining flavor checks unchanged
- [X] T043 [US1] Branch on `IsPublicClient()` in `Create` and `Update` in
      `internal/domain/thirdparty/service.go` to skip `Secret.GetPlaintext()` and
      `encryption.Encrypt` while leaving `branchKeyManager.Create` unconditional (FR-028)
- [X] T044 [US1] Skip `decryptSecret` for public services on the read path in
      `internal/domain/thirdparty/service.go` so `Get` and `List` return the absent secret without
      entering the decryption-failure warning path
- [X] T045 [US1] Add `TokenEndpointAuthMethod *string \`json:"token_endpoint_auth_method"\`` to the
      shared `ServiceRequest` DTO (line 46) in
      `internal/adapters/http/handlers/admin/services_handler.go` — one field serving both create and
      update — and apply the D5 branch in `CreateService`: construct `model.NewAbsentSecret()` —
      never `NewPlaintextSecret("")` — on the public branch, branching on the method rather than on
      the secret being non-empty
- [X] T046 [US1] Reshape the single `ServiceResponse` (line 80) and its mapper in
      `internal/adapters/http/handlers/admin/services_handler.go`: change `ClientSecret` to `*string`
      with `json:"client_secret,omitempty"` and add
      `TokenEndpointAuthMethod *string \`json:"token_endpoint_auth_method"\`` **without** `omitempty`,
      so the key is always emitted. Map from `entity.IsPublicClient()`: public → method pointer to
      `"none"` and `ClientSecret` nil so the credential key is omitted entirely; confidential →
      method **nil pointer** so the JSON carries `"token_endpoint_auth_method": null` and
      `ClientSecret` pointer to `"REDACTED"`. Never emit `""` for the method and never omit the key —
      the read schema declares it `required` **and** `nullable`, so every entry in a single list read
      answers whether the service is public (FR-024, FR-025, SC-007)
- [X] T047 [US1] Return 400 `error: "validation failed"` with the exact messages from
      [contracts/admin-api.md §5](./contracts/admin-api.md#5-error-responses) for each new failure,
      reusing the existing `ErrorResponse` shape in
      `internal/adapters/http/handlers/admin/services_handler.go`
- [X] T048 [US1] Add the `event` key (`service.thirdparty.provider_created`,
      `service.thirdparty.provider_updated`) and a `public_client` boolean attribute to the lifecycle
      logs in `internal/domain/thirdparty/service.go`, passing no credential, verifier, code, or
      token to the logger ([research.md §D9](./research.md#d9--audit-logging))
- [X] T049 [US1] Run `go test ./internal/domain/model/... ./internal/domain/thirdparty/... ./internal/adapters/http/handlers/admin/...`
- [X] T050 [US1] Verify scenarios US1-S1 through US1-S6 pass:
      `ginkgo -v --label-filter="!performance" --focus="Public Client" ./tests/e2e/`

**Checkpoint**: A public service can be registered, listed, and read; contradictory configuration is
rejected before any side effect — User Story 1 is independently demonstrable

---

## Phase 4: User Story 2 - Authorize a user against a public-client provider (Priority: P2)

**Goal**: A user connects their account at a public-client provider; the broker exchanges the
authorization code proving possession of the code verifier only, sending no credential in the body
and no `Authorization` header, and stores an encrypted session.

**Independent Test**: With a public service registered, run the full connect journey against the
strict-mode upstream double that rejects credentialed requests; confirm a usable session exists and
the captured token request carried `client_id` and `code_verifier` and no credential of any kind.

### Tests for User Story 2 [MANDATORY - Principle VIII] ⚠️

- [X] T051 [P] [US2] Write tests in `internal/domain/oauth2session/service_test.go` asserting
      `buildOAuth2Config` pins `Endpoint.AuthStyle = oauth2.AuthStyleInParams` with an empty
      `ClientSecret` for a public service and leaves `AuthStyle` at its zero value with the decrypted
      secret for a confidential service (FR-012, FR-013, FR-015, FR-017)
- [X] T052 [P] [US2] Create `internal/domain/oauth2session/service_security_test.go` asserting SR-003
      for the authorization-code exchange: the captured request carries no `client_secret` body
      parameter, no `Authorization` header of any scheme including an empty password, and no
      credential query parameter. Simulate an upstream error containing sentinel credential, verifier,
      authorization-code, access-token, and refresh-token values. Assert that no sentinel reaches a
      log entry or callback error response (FR-027, SC-008).

### Implementation for User Story 2

- [X] T053 [US2] Branch `buildOAuth2Config` in `internal/domain/oauth2session/service.go` on
      `entity.IsPublicClient()`: empty `ClientSecret` and pinned `oauth2.AuthStyleInParams` for
      public, unchanged construction for confidential
- [X] T054 [US2] Ensure the exchange path in `internal/domain/oauth2session/service.go` never calls
      `Secret.GetPlaintext()` on an absent secret, so a public service reaches
      `exchangeCodeWithRetry` without an error and the retry loop itself stays unmodified (FR-016)
- [X] T055 [US2] Replace raw upstream error logging in `exchangeCodeWithRetry` and raw error exposure
      from `HandleCallback` in `internal/domain/oauth2session/service.go` with safe reasons that do
      not contain upstream response text. Retain only the classification needed for retry handling.
      Add the `public_client` attribute to `session.oauth2.flow_initiated`,
      `session.oauth2.session_established`, and `session.oauth2.pkce_validation_failed` (FR-026,
      FR-027, SR-008).
- [X] T056 [US2] Confirm `InitiateOAuth2Flow` in `internal/domain/oauth2session/service.go` requires
      no change: `oauth2.GenerateVerifier`, `oauth2.S256ChallengeOption`, and the provider
      authorization parameter application stay unconditional for both modes (FR-010, FR-018, SR-001)
- [X] T057 [US2] Run `go test ./internal/domain/oauth2session/...`
- [X] T058 [US2] Run
      `ginkgo -v --label-filter="!performance" --focus="authorizes a user against a public-client provider" ./tests/e2e/`
      and confirm US2-S1 through US2-S6 in `tests/e2e/thirdparty_public_client_test.go` pass against
      the strict-mode double, including the confidential regression US2-S6

**Checkpoint**: A user obtains a working session through a provider that rejects client credentials;
confidential connect flows are unchanged

---

## Phase 5: User Story 3 - Keep a public-client session alive (Priority: P3)

**Goal**: An expired public-client access token is refreshed with the client identifier and refresh
token only; the refreshed tokens are encrypted and replace the stored ones.

**Independent Test**: Force expiry of a stored public-client session, trigger a refresh, and confirm
new tokens are persisted while the captured refresh body carried no `client_secret`.

### Tests for User Story 3 [MANDATORY - Principle VIII] ⚠️

- [X] T059 [P] [US3] Write tests in `internal/domain/oauth2session/service_test.go` asserting the
      refresh form omits `client_secret` for a public service and still carries `grant_type`,
      `refresh_token`, `client_id`, and the provider authorization params — and is byte-identical to
      today for a confidential service (FR-014, FR-017, FR-018)
- [X] T060 [P] [US3] Extend `internal/domain/oauth2session/service_security_test.go` with the SR-003
      assertions for the refresh request: no `client_secret` body value, no `Authorization` header,
      and no credential query parameter. Simulate an upstream refresh error containing sentinel
      credential, verifier, authorization-code, access-token, and refresh-token values. Assert that
      no sentinel reaches a log entry or returned refresh error (FR-027, SC-008).

### Implementation for User Story 3

- [X] T061 [US3] Omit the `data.Set("client_secret", …)` line for public services in
      `RefreshAccessToken` in `internal/domain/oauth2session/service.go`, changing nothing else on
      that path
- [X] T062 [US3] Ensure a failed public-client refresh in
      `internal/domain/oauth2session/service.go` surfaces to the caller without a credentialed retry
      and without silently downgrading the session (FR-015, SR-005)
- [X] T063 [US3] Replace raw upstream error logging and exposure from `refreshSessionTokens` in
      `internal/domain/oauth2session/service.go` with safe reasons that do not contain upstream
      response text. Retain only the classification needed for retry handling. Add the
      `public_client` attribute to `session.oauth2.token_refreshed` and `session.oauth2.refresh_failed`
      (FR-026, FR-027, SR-008).
- [X] T064 [US3] Confirm `internal/domain/tokenexchange/service.go` needs no change: RFC 8693
      refreshes through `GetValidAccessToken` and inherits public-client behaviour
- [X] T065 [US3] Run `go test ./internal/domain/oauth2session/... ./internal/domain/tokenexchange/...`
      and verify scenarios US3-S1 through US3-S4 pass

**Checkpoint**: Public-client sessions refresh without operator intervention; confidential refresh is
unchanged

---

## Phase 6: User Story 4 - Change an existing service between public and confidential (Priority: P4)

**Goal**: An operator switches a service between modes. Becoming public removes the stored credential;
becoming confidential requires a credential in the same request. Omission and explicit `null` are
equivalent and both mean confidential.

**Independent Test**: Update a confidential service declaring `none` with no credential, confirm the
stored ciphertext is gone at the storage layer, then update it back and confirm rejection unless a
credential is supplied.

### Tests for User Story 4 [MANDATORY - Principle VIII] ⚠️

- [X] T066 [P] [US4] Write update-path tests in
      `internal/adapters/http/handlers/admin/services_handler_test.go` for every row of the
      [research.md §D5](./research.md#d5--wire-format-presence-semantics) mapping applied to
      `UpdateService`, including the US4-S6 round trip that submits back the
      `"token_endpoint_auth_method": null` a read returned, and the rejection when a public service
      is updated without the method and without a credential
- [X] T067 [P] [US4] Write a test in `internal/domain/thirdparty/service_test.go` asserting that a
      confidential→public update passes an absent secret to `repo.Update` — so the ciphertext is
      removed rather than left dormant (FR-020, SR-006) — and that a public→confidential update
      encrypts and stores the supplied credential

### Implementation for User Story 4

- [X] T068 [US4] Apply the same method-first branch in `UpdateService` in
      `internal/adapters/http/handlers/admin/services_handler.go`, reusing the `ServiceRequest` field
      added in T045 (no DTO change here — the type is shared) and keeping full-replacement semantics:
      the stored method is never consulted (FR-019)
- [X] T069 [US4] Ensure a confidential→public update in `internal/domain/thirdparty/service.go`
      persists the absent secret so `client_secret_encrypted` is written as `NULL`
- [X] T070 [US4] Ensure a public→confidential update writes `token_endpoint_auth_method` back to
      `NULL` through `internal/adapters/storage/postgres/thirdparty_provider.go` and the in-memory
      adapter
- [X] T071 [US4] Assert in `tests/e2e/thirdparty_public_client_test.go` (US4-S5) that neither
      transition deletes or rewrites rows through the user-session repository accessed by
      `internal/domain/oauth2session/service.go` — the stored session is unchanged across the update
      — and that the next `RefreshAccessToken` call in that file reads the new
      `TokenEndpointAuthMethod`, including or omitting `client_secret` accordingly (FR-022)
- [X] T072 [US4] Run `go test ./internal/domain/thirdparty/... ./internal/adapters/http/handlers/admin/... ./internal/adapters/storage/...`
- [X] T073 [US4] Run
      `ginkgo -v --label-filter="!performance" --focus="Public Client" ./tests/e2e/` and confirm
      US4-S1 through US4-S6 in `tests/e2e/thirdparty_public_client_test.go` pass, then confirm the
      stored ciphertext is gone with the `psql` query in
      [quickstart.md Scenario 4](./quickstart.md#scenario-4--switch-a-service-between-modes-user-story-4)

**Checkpoint**: All four user stories are independently functional; no dormant credential survives a
transition

---

## 🔒 Phase N: Constitution Compliance & Polish [MANDATORY COMPLIANCE SECTION]

### 🔒 Constitution Compliance Verification [MANDATORY]

#### Design Phase Verification

- [X] T074 Verify the Glossary in `ARCHITECTURE.md` carries `TokenEndpointAuthMethod`, **Public
      client**, **Confidential client**, and the amended **Secret** entry (Principle V)
- [ ] T075 Verify no configuration change was required: `examples/config/`,
      `examples/config/README.md`, `docs/configuration.md`, and `charts/agentic-identity-broker/` are
      unmodified in the diff (Principle VII)
- [X] T076 Verify `api/admin/openapi.yaml` matches [contracts/admin-api.md](./contracts/admin-api.md)
      exactly across all three schemas and four path descriptions, and that
      `api/enduser/openapi.yaml` changed descriptions only (Principles IV, X)
- [X] T077 Verify the dated API-010 confirmation record survives intact in
      `specs/042-thirdparty-public-pkce/contracts/admin-api.md:243-261` — "Confirmed by the
      stakeholder on 2026-09-10" with all five items still `- [x]` — and that its two substantive
      decisions are still backed by the matching clarification entries in
      `specs/042-thirdparty-public-pkce/spec.md:34-35`; then confirm
      `internal/adapters/http/handlers/admin/services_handler.go` implements both: `client_secret`
      omitted for public services, and explicit `null` accepted as equivalent to omission
      (Principle X)
- [X] T078 Verify `migrations/031_add_token_endpoint_auth_method.{up,down}.sql` follow go-migrate
      naming, are sequential after `030`, and both directions are present (Principle IX)
- [X] T079 Verify `tests/e2e/thirdparty_public_client_test.go` contains exactly 22 `It()` blocks,
      one per acceptance scenario in [spec.md](./spec.md), each with its
      `// USx-Sy from specs/042-thirdparty-public-pkce/spec.md` comment (Principle XIII)
- [X] T080 Verify the red-phase rules still hold in `tests/e2e/thirdparty_public_client_test.go`: no
      `XIt`/`PIt`/`XDescribe`/`PDescribe`/`XContext`/`PContext`/`Skip()`, no placeholder always-fail
      assertions, and no red-phase comments (Principles VIII, XIII)

#### Implementation Phase Verification

**API & Documentation** (Principles IV, X):

- [X] T081 [P] Verify the implemented request and response shapes in
      `internal/adapters/http/handlers/admin/services_handler.go` match the confirmed schemas in
      `api/admin/openapi.yaml` field for field
- [X] T082 [P] Verify `docs/changelog.md` records the read-schema `required` relaxation and that no
      `docs/api/` guide is required (none exists for service CRUD)

**Architecture & Documentation** (Principle II):

- [X] T083 [P] Verify `adrs/036-public-client-token-endpoint-auth.md` is Status: Accepted, records
      all four decisions, explicitly supersedes the two-state `Secret` decision in ADR 012, and that
      `adrs/012-encryption-layer-separation.md` carries the reciprocal amendment note

**Database & Persistence** (Principle IX):

- [X] T084 [P] Verify `tests/integration/migrations/migrations_test.go` covers apply, no backfill,
      the `CHECK` in both contradictory directions, rollback refusal with the dirty-flag assertion,
      and successful rollback after `Force(31)`
- [X] T085 [P] Verify both storage adapters have tests for the absent-credential state —
      `internal/adapters/storage/memory/thirdparty_provider_test.go` and
      `tests/integration/storage/infra/thirdparty_service_test.go` (DB-008)

**Security** (Principles I, III):

- [X] T086 Verify fail-closed behaviour for every contradictory configuration in the validation table
      of `specs/042-thirdparty-public-pkce/quickstart.md` Scenario 1 — unknown method value, `none`
      with a non-empty secret, `none` with `oauth2_flavor: google`, `none` with no `client_id`, and
      no method with no secret — confirming each is rejected by
      `ValidateForCreate`/`ValidateForUpdate` in
      `internal/domain/model/thirdparty_oauth2_provider.go` or by the presence mapping in
      `internal/adapters/http/handlers/admin/services_handler.go`, so rejection happens before
      `internal/domain/oauth2session/service.go` can issue any upstream request (SR-004, SC-005)
- [X] T087 [P] Verify no custom cryptography was introduced: PKCE stays with `golang.org/x/oauth2`
      and encryption stays with the existing branch-key manager (Principle III, SR-009)
- [X] T088 [P] Verify the audit entries added in `internal/domain/thirdparty/service.go` and
      `internal/domain/oauth2session/service.go` carry the `event` key and `public_client` attribute.
      Verify the sentinel tests from T052 and T060 prove that logs, callback errors, and refresh
      errors contain no credential, code verifier, code challenge, authorization code, or token
      (FR-026, FR-027, SR-008)

**Architecture Patterns** (Principle VI):

- [X] T089 Verify the invariant lives in the domain entity and that
      `internal/adapters/http/handlers/admin/services_handler.go` only maps JSON presence to domain
      state without re-implementing a domain rule; confirm no new port and no change to
      `internal/app/builder.go` or `internal/adapters/http/routing/admin.go`

**Testing** (Principle VIII):

- [X] T090 Verify the unit tests in `internal/domain/` and `internal/adapters/` were written first,
      failed semantically, and changed only minimally during implementation
- [X] T091 Verify no Bash script was used for code correctness validation: every check added by this
      feature lives in a Go `_test.go` file under `internal/` or `tests/`, and `justfile` targets are
      unchanged

**E2E Acceptance Testing** (Principle XIII):

- [X] T092 Run `just test-e2e-backend` and confirm every pre-existing third-party suite —
      `tests/e2e/oauth2_provider_flavor_test.go`,
      `tests/e2e/oauth2_provider_github_flavor_test.go`,
      `tests/e2e/provider_authorization_params_test.go`, `tests/e2e/oauth2_token_test.go`,
      `tests/e2e/canonical_resource_ids_test.go` — passes unmodified (FR-017, SC-004)
- [X] T093 Verify `tests/e2e/thirdparty_public_client_test.go` changed only in fixtures during
      implementation, never in test logic, and that all 22 scenarios are green

**Frontend** (Principle XI): not applicable — no `web/` change, no Playwright test, no screenshot.

### Additional Polish

- [ ] T094 Run the full validation ladder from
      [quickstart.md](./quickstart.md#validation-ladder): `just check`, the domain and adapter unit
      packages, `just test-integration-infra`, the focused Ginkgo run, and `just verify`; then
      execute Scenario 7 and confirm the captured broker log contains no credential material, and
      confirm no scaffolding comment, TODO, or debug artifact from T019 remains

**Checkpoint**: Constitution compliance verified, full gate green

---

## Dependencies & Execution Order

### Phase Dependencies

- **Phase 0 (Refactoring)**: skipped
- **Phase 1 (Setup)**: skipped
- **Phase 2 (Design Preconditions)**: BLOCKS all implementation
  - 2a, 2b, 2c, 2d, 2e can proceed in parallel with each other
  - T017 and T019 can run in parallel. T018 depends on T019 and is compile-only during Phase 2f.
  - T020 through T023 depend on T017, T018, and T019 for the strict-mode double, fixture, and
    compile-level domain types.
  - All of Phase 2 must complete before Phase 2.5
- **Phase 2.7 (Entity Boilerplate)**: skipped — no new entity
- **Phase 2.5 (Foundational)**: depends on all of Phase 2; BLOCKS all user stories
- **Phase 3-6 (User Stories)**: all depend on Phase 2 + Phase 2.5
- **Phase N (Compliance & Polish)**: depends on all user stories being complete

### User Story Dependencies

- **US1 (P1)**: depends only on Phase 2.5. Viable standalone increment (MVP)
- **US2 (P2)**: depends on US1 — a public service must exist to authorize against
- **US3 (P3)**: depends on US2 — a stored public-client session must exist to refresh
- **US4 (P4)**: depends on US1 — `CreateService` and `UpdateService` share the single `ServiceRequest`
  DTO and the `ServiceResponse` mapper, and both call the same entity validation rules, so T068 reuses
  the field T045 adds. Can land in parallel with US3, which touches a different file

### File Conflicts (do NOT parallelize across these)

| File | Tasks |
|---|---|
| `api/admin/openapi.yaml` | T006, T007, T008, T009, T010 — sequential |
| `internal/domain/model/thirdparty_oauth2_provider.go` | T019, T036, T042 — sequential |
| `internal/domain/model/secret.go` | T019, T030 — sequential |
| `internal/adapters/http/handlers/admin/services_handler.go` | T045, T046, T047, T068 — sequential; US1 before US4 |
| `internal/domain/thirdparty/service.go` | T043, T044, T048, T069 — sequential; US1 before US4 |
| `internal/domain/oauth2session/service.go` | T053, T054, T055, T061, T062, T063 — sequential; US2 before US3 |
| `internal/domain/oauth2session/service_security_test.go` | T052 creates, T060 extends — sequential |
| `tests/e2e/thirdparty_public_client_test.go` | T020, T021, T022 — sequential |
| `internal/domain/thirdparty/service_test.go` | T040, T067 — sequential; US1 before US4 |
| `internal/adapters/http/handlers/admin/services_handler_test.go` | T041, T066 — sequential; US1 before US4 |

### Within Each User Story

- Tests written and failing semantically before implementation
- Domain model → domain service → adapter (HTTP, storage)
- Story complete and its E2E scenarios green before the next priority

---

## Parallel Opportunities

### Phase 2 (design)

```text
T001, T002, T003, T004   # ARCHITECTURE.md and adrs/ — T001+T002 same file, run sequentially
T011, T012               # api/enduser/openapi.yaml and docs/changelog.md — independent files
T013, T014               # the two migration SQL files
T017 || T019              # independent helper and compile scaffolding
T018                      # after T019; compile-only during Phase 2f
T020, T021, T022, T023    # after T017, T018, and T019
```

### Phase 2.5 (foundational)

```text
# All five test tasks touch different files and can be written concurrently:
T024  internal/domain/model/secret_test.go
T025  internal/domain/model/token_endpoint_auth_method_test.go
T026  tests/integration/migrations/migrations_test.go
T027  internal/adapters/storage/memory/thirdparty_provider_test.go
T028  tests/integration/storage/infra/thirdparty_service_test.go

# Then the independent adapter edits:
T032  postgres/thirdparty_provider_record.go
T034  memory/thirdparty_provider_record.go
T035  internal/ports/thirdparty_provider.go
```

### User Story 1

```text
T039  internal/domain/model/thirdparty_oauth2_provider_test.go
T040  internal/domain/thirdparty/service_test.go
T041  internal/adapters/http/handlers/admin/services_handler_test.go
```

### User Stories 3 and 4

US3 (`internal/domain/oauth2session/service.go`) and US4
(`internal/adapters/http/handlers/admin/services_handler.go`,
`internal/domain/thirdparty/service.go`) touch disjoint files once US1 and US2 have landed, so they
can be implemented concurrently by different agents.

---

## Implementation Strategy

### MVP First (User Story 1 only)

1. Phase 2 — design preconditions: glossary and ADR 036, the admin API delta, migration `031`, and
   all 22 E2E tests written and verified red
2. Phase 2.5 — absent `Secret` state, `TokenEndpointAuthMethod`, migration applied, both storage
   adapters persisting an absent credential
3. Phase 3 — User Story 1
4. **STOP and VALIDATE**: run [quickstart.md Scenario 1](./quickstart.md#scenario-1--register-a-public-service-user-story-1);
   US1-S1 through US1-S6 green
5. Ship: the previously impossible provider is now representable and auditable

### Incremental Delivery

1. Phase 2 → Phase 2.5 → foundation ready
2. US1 → registration works → demo (MVP)
3. US2 → users obtain sessions through credential-rejecting providers → demo
4. US3 → sessions stay alive without re-authorization → demo
5. US4 → operators correct or migrate a service between modes → demo

Each story adds value without altering the previous ones, and confidential behaviour stays
byte-identical throughout (FR-017, SC-004).

### Parallel Team Strategy

- Phase 2: one agent on 2a+2c (documentation and API), one on 2d (migrations), one on 2f (E2E tests)
  once T019 scaffolding exists
- Phase 2.5: split the five test tasks, then the three independent adapter edits
- Stories: US1 first and alone (it owns the shared handler mapper and entity validation), then US2;
  US3 and US4 in parallel afterwards on disjoint files

---

## Notes

- `[P]` tasks touch different files and have no dependency on an incomplete task
- The trap flagged in [research.md §D5](./research.md#d5--wire-format-presence-semantics): the
  handler must construct `model.NewAbsentSecret()` on the public branch, **not**
  `NewPlaintextSecret("")`, otherwise `{"token_endpoint_auth_method":"none","client_secret":""}` is
  rejected by domain validation rule 3 despite being an accepted representation
- `internal/adapters/http/handlers/admin/services_handler.go` declares **one** `ServiceRequest` type
  (line 46) for both create and update and **one** `ServiceResponse` (line 80). The
  `token_endpoint_auth_method` field is added once (T045) and the response is reshaped once (T046);
  US4 only adds the branch in `UpdateService`
- `recordToEntity` in `internal/adapters/storage/postgres/thirdparty_provider_record.go` currently
  calls `model.NewEncryptedSecret(record.SecretCiphertext)` unconditionally (line 187) and
  `entityToRecord` calls `GetCiphertext()` unconditionally — both must branch on the absent state, and
  the method column needs its own `*string` ↔ `model.TokenEndpointAuthMethod` conversion in each
  direction
- Rule 2 (google rejection) must precede `enrichForGoogleFlavor`, which parses a credential document
  a public service does not have
- The refused rollback leaves `schema_migrations.dirty = true` by design; `Force(31)` in tests and
  `migrate force 31` for operators is the documented recovery
- Commit after each task or logical group; stop at any checkpoint to validate a story independently
