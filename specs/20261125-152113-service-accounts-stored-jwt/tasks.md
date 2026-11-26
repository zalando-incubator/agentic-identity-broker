---

description: "Task list for service accounts on third-party services (jwt_bearer with a stored JWT)"
---

# Tasks: Service Accounts on Third-Party Services (First Kind: `jwt_bearer` with a Stored JWT)

**Input**: Design documents from `specs/20261125-152113-service-accounts-stored-jwt/`

**Prerequisites**: `plan.md`, `spec.md`, `research.md`, `data-model.md`,
`contracts/admin-service-accounts-openapi.yaml`, `contracts/observability.md`, `quickstart.md`

**Tests**: Mandatory red-green-refactor under Constitution Principles VIII and XIII. Write each
test first, make it compile against the Phase 2.7 scaffold, see it fail on a concrete assertion
where it runs locally, then implement. Red-run evidence is not required. Container-backed suites
(`just test-integration-infra`, `just test-e2e-docker`) may first run in CI on the pushed commit.

**Organization**: Shared design, scaffold and foundation precede one phase per user story in
spec priority order. Paths are relative to the repository root. `[P]` means a different file and
no dependency on an unfinished task in the same batch.

## 🔒 Feature Scope [MANDATORY]

- Every task cites a requirement, acceptance scenario, success criterion or binding constraint of
  this feature. No task hands a stored credential to any caller, changes token exchange, binds an
  account to an agent or touches the UI (FR-024).
- The maintainer approves the API contract (API-006, Principle X) and the other decisions in
  `plan.md` §Review Items by reviewing the PR. No task waits for or records a separate approval.
- The scenario for a delete refused while an agent grant binds the account belongs to #472. This
  feature defines the rule and its `409` (FR-020, T007) and covers the refusal path with a domain
  unit test (T060).
- Findings outside this scope are described to the user, not fixed here. Only user-approved issue
  URLs are recorded, under a `Future Work` note in this file.

---

## Phase 0: Pre-implementation Refactoring [OPTIONAL]

Skipped. No existing code is restructured (plan §Implementation Phase Overview). The two
additions to existing types, the provider service's delete guard and the memory provider
repository's service-account counter, are feature behaviour and belong to User Story 5.

---

## Phase 1: Setup

- [X] T001 Read `internal/AGENTS.md`, `internal/domain/AGENTS.md`, `internal/domain/id/AGENTS.md`, `internal/ports/AGENTS.md`, `internal/adapters/AGENTS.md`, `tests/e2e/AGENTS.md` and `tests/integration/AGENTS.md`; confirm that `050` is the next free number in `migrations/` and that `ServiceAccountID` is not yet in `internal/domain/id/gen_ids.go` (DB-002; Principles II, IX).

---

## 🔒 Phase 2: Design Preconditions (Blocking Prerequisites) [MANDATORY]

**Purpose**: Domain, configuration, API, database, journey, E2E and observability design before
production code.

**⚠️ CRITICAL**: No production code before this phase completes.

### Phase 2a: Domain Model & Glossary [MANDATORY]

**Constitution Reference**: Principles II, V

- [X] T002 Verify `specs/20261125-152113-service-accounts-stored-jwt/data-model.md` against `spec.md` §Domain Model and FR-001–FR-025: entity fields, value objects, derived `Narrowing` and `StatusAt`, update outcomes, domain events, invariants (one service for life, non-empty ceiling, exactly one credential, ID and kind fixed, `canonical_id` renamable). Correct `data-model.md` only where it contradicts the spec (Principle V).
- [X] T003 [P] Add a "Service Accounts" glossary section to `ARCHITECTURE.md` §12 and rows to the Domain Glossary table in `AGENTS.md` for Service Account, Kind, Credential Format, Ceiling, Narrowing and Credential Summary, with the definitions of #478 and their relationships to ThirdpartyOAuth2Service (spec §Domain Model; Principle V).
- [X] T004 [P] Write `adrs/20261125-154231-service-accounts-stored-jwt.md` (Context, Decision, Consequences, Status) covering research R2, R3, R6, R7, R9, R15 and SR-004: the entity and its repository port, the stored-JWT posture and why it is not a Principle I exception, `service_id` encryption of a format-discriminated credential document, status derived on read, optional `If-Match`, `ON DELETE RESTRICT`. Set Status to Accepted; Principle II permits accepting an ADR in the PR that introduces it. Add the ADR to `ARCHITECTURE.md` §10 and to the ADR Decision Index in `AGENTS.md` (ADR-SA; Principle II).

**Checkpoint**: Domain model documented, glossary updated, ADR drafted.

### Phase 2b: Configuration Design [MANDATORY]

**Constitution Reference**: Principle VII

- [X] T005 Confirm that no configuration parameter is added: no change to `internal/ports/config.go`, `docs/configuration.md`, `examples/config/` or `charts/agentic-identity-broker/`; the 16 KiB limit is fixed in the contract; the chart's existing migration Job applies `050`. Record the result in a `## Design notes` section of `specs/20261125-152113-service-accounts-stored-jwt/tasks.md` (spec Assumptions; Principle VII).

**Checkpoint**: No configuration or Helm change, confirmed.

### Phase 2c: API Design [MANDATORY]

**Constitution Reference**: Principles IV, X

- [X] T006 Merge `specs/20261125-152113-service-accounts-stored-jwt/contracts/admin-service-accounts-openapi.yaml` into `api/admin/openapi.yaml`: `info.version` `3.1.0`, a `Service Accounts` tag, the three paths, the parameters, header, responses and schemas, and the extra `409` cause on `DELETE /api/services/{service-id}` (reuse the existing `ServiceId`, `ErrorResponse`, `NotFoundError`, `InternalServerError` components). Leave every other operation unchanged (API-001–API-006; Principles IV, X).
- [X] T007 Verify the merged contract against Journeys 1–3 of `spec.md` (status codes, headers, bodies, messages) and FR-020's delete conflict; add the Admin API `3.1.0` note to the API versioning paragraph of `ARCHITECTURE.md` §5; confirm `api/enduser/openapi.yaml` is unchanged (API-004, API-005; Principles II, IV).

**Checkpoint**: Contract merged; the maintainer confirms it in PR review.

### Phase 2d: Database Design [MANDATORY]

**Constitution Reference**: Principle IX

- [X] T008 Verify the `service_accounts` schema in `data-model.md` §PostgreSQL schema against DB-001–DB-003 and FR-025: every column, `CHECK`, `uq_service_accounts_service_canonical_id`, `ON DELETE RESTRICT`, no plaintext column, explicit-column queries, one atomic pair `050_create_service_accounts.{up,down}.sql` without `-- migrate:no-transaction` (DB-050; Principle IX). The SQL files are written in T037 after their tests.

**Checkpoint**: Schema designed; migration number reserved.

### Phase 2e: Frontend/Design System Review [MANDATORY IF FRONTEND]

Not applicable. The feature changes no React UI, so no design-system review, Wireloom sequence,
Playwright test or screenshot applies (spec Assumptions: the admin UI comes later; Principle XI).

### Phase 2f: E2E Acceptance Test Design [MANDATORY]

**Constitution Reference**: Principles XIII, XIV

E2E tests reach the broker only over HTTP, so they compile before the scaffold; without routes
they fail on `404`, after Phase 2.7 on `501`.

- [X] T009 Review Journeys 1–3 in `spec.md` against their linked scenarios and the contract delta: HTTP exchanges, `Location` with the UUID path although the request used canonical IDs, ETag on create, read, update and status, decoded claims of tokens T1, T2 and U, and the absence of configuration. Record findings in the `## Design notes` section of `specs/20261125-152113-service-accounts-stored-jwt/tasks.md`; change `spec.md` only with the user's approval (Principle XIV).
- [X] T010 [P] Create `tests/e2e/fixtures/service_account.go`: the *Company Platform* service (canonical ID `company-platform`) built on `fixtures.ServiceWithID`; ES256 tokens signed with a key generated per call carrying the T1 and T2 header and claims of the spec (`kid`, `iss`, `sub`, `aud`, `scope`, `iat`, `exp`); a token without `exp`; a token whose `exp` lies a given number of seconds ahead; `alg` variants `none`, `NONE`, `None` and missing; a token of 16385 bytes; a five-segment token; a token with a leading space; a token whose payload is not a JSON object; a token with a boolean `exp`; create and update request bodies. All values synthetic (US1–US5; Principle XIII).
- [X] T011 [P] Create `tests/e2e/helpers/service_account.go` with a decoder for the `ServiceAccount` and `ServiceAccountStatus` representations and a Gomega matcher that fails when a response body or header contains a given JWT or any of its three segments; create `tests/e2e/service_accounts_backend_test.go` with `withServiceAccountBackend(backend string, action func(*bootstrap.TestServer, *storageadapter.Adapter))` for `memory` and `postgres`, following `withExternalIDBackend` in `tests/e2e/agent_external_id_backend_test.go` (SR-006, SC-008; Principle XIII).
- [X] T012 [US1] Write the seven User Story 1 `It()` blocks in `tests/e2e/service_accounts_register_test.go` (Scenarios 1–7) under `Describe("Service accounts")`, one `Context` per precondition, each preceded by `// Scenario N from specs/20261125-152113-service-accounts-stored-jwt/spec.md (User Story 1)`, with the descriptions of `plan.md` §Scenario Mapping. Scenario 5 carries `Label("docker")` and runs both store cases through `withServiceAccountBackend`. Assert status, `Location`, `ETag`, every representation member and summary value, list contents, and the credential-absence matcher on every response (US1.1–US1.7; FR-002, FR-011, FR-012, FR-016; SC-001).
- [X] T013 [US2] Write the seven User Story 2 `It()` blocks in `tests/e2e/service_accounts_register_test.go` (Scenarios 1–7). Scenarios 1, 2, 5 and 6 iterate over a table of variants inside one `It()`; every variant asserts `400`, the contract message, an unchanged list and the credential-absence matcher (US2.1–US2.7; FR-006, FR-007, FR-009, FR-010; SC-003).
- [X] T014 [US3] Write the nine User Story 3 `It()` blocks in `tests/e2e/service_accounts_lifecycle_test.go` (Scenarios 1–9). Scenarios 7 and 9 carry `Label("docker")` and run both store cases. Scenario 3 registers a token expiring a few seconds ahead and waits for `failed` with `Eventually` before rotating; no fixed sleep. Scenario 8's rename body repeats the stored `scopes`, which `PUT` requires (US3.1–US3.9; FR-005, FR-017–FR-019; SC-005).
- [X] T015 [US4] Write the four User Story 4 `It()` blocks in `tests/e2e/service_accounts_lifecycle_test.go` (Scenarios 1–4); Scenario 2 uses `Eventually` on both the status read and the list (US4.1–US4.4; FR-014, FR-015; SC-004).
- [X] T016 [US5] Write the four User Story 5 `It()` blocks in `tests/e2e/service_accounts_lifecycle_test.go` (Scenarios 1–4). Scenario 4 carries `Label("docker")`, runs both store cases, and asserts `409` on service deletion, the unchanged service and account, then `204` after the account is removed (US5.1–US5.4; FR-020, FR-021, FR-025).
- [ ] T017 Check both suites: `Describe` → `Context` → exactly one `It()` per scenario, scenario comments, fresh server and storage per test, fixtures from T010, no `XIt`, `PIt`, `Skip()` or red-phase comments. Run `ginkgo -v --label-filter="!performance && !docker" --focus "Service accounts" ./tests/e2e/` and confirm each test fails on a concrete assertion. Add `file:line` to each row of `plan.md` §Scenario Mapping (Principle XIII).

**Checkpoint**: Journeys reviewed; 31 E2E scenarios written and failing semantically.

### Phase 2g: Observability Design [MANDATORY IF TELEMETRY]

**Constitution Reference**: Principle XV

- [X] T018 Verify `contracts/observability.md` against `coding-standards/LOGGING.md` §§2, 3, 6, 8, 10 and `coding-standards/OPENTELEMETRY.md` §§3, 4, 6, 8: event names, levels, canonical keys, the seven `storage.*.service_account` spans, no new span attribute namespace and no metric instrument (so no metric entry and no §4.1 registry change), and the credential-path test matrix in §8 (Principle XV).
- [X] T019 Add the service-account credential row of `contracts/observability.md` §4 to the table in `coding-standards/LOGGING.md` §6.2, and draft the PR-description paragraph stating the change and its rationale in the `## Design notes` section of `specs/20261125-152113-service-accounts-stored-jwt/tasks.md` (FR-022, FR-023; Principle XV).

**Checkpoint**: Observability design checked against the standards; amendment applied.

---

## Phase 2.7: Entity Boilerplate (New Entity: ServiceAccount)

**Purpose**: Empty but compiling scaffold, committed separately ahead of business logic (plan
§Implementation Phase Overview). Handlers answer `501`; repositories answer not found or not
implemented; domain functions return zero values.

- [X] T020 Add `{"ServiceAccountID", "service_account"}` to `uuidTypes` in `internal/domain/id/gen_ids.go`, run `go generate ./internal/domain/id/`, commit `internal/domain/id/uuid_ids_gen.go`, add parse and serialization cases to `internal/domain/id/id_test.go`, and list the type in `internal/domain/id/AGENTS.md` (`data-model.md` `ServiceAccount.ID`; ADR 013).
- [X] T021 Create `internal/domain/model/service_account.go` and `internal/domain/model/service_account_credential.go` with the types of `data-model.md` §Entity and §Value objects: `ServiceAccount` (all fields of `data-model.md` §Entity, `ServiceAccount.ID` through `ServiceAccount.UpdatedAt`), `ServiceAccountKind`, `CredentialFormat`, `CredentialUse`, `Narrowing`, `ServiceAccountStatus`, `ServiceAccountFailureReason` (typed string constants, each with a doc comment), `CredentialSummary`, `ServiceAccountCredentialInput`, `CredentialValidationError`, and signatures for the validation, summary, `StatusAt` and `Narrowing` functions with empty bodies (Principle VIII scaffold).
- [X] T022 Add `ServiceAccountRepository` with the seven methods of `data-model.md` §Repository port, and the sentinels `ErrServiceAccountCanonicalIDConflict` and `ErrServiceAccountVersionMismatch`, to `internal/ports/storage.go` with the contract as doc comments; add a row to the catalogue in `internal/ports/AGENTS.md` (DB-001; ADR 004; Principle IX).
- [X] T023 [P] Create `internal/adapters/storage/memory/service_account_repository.go` implementing the port with methods that return not found or zero values (DB-001).
- [X] T024 [P] Create `internal/adapters/storage/postgres/service_account_repository.go` implementing the port with methods that return a `storage.StorageError` of kind unknown (DB-001).
- [X] T025 Add `ServiceAccounts() ports.ServiceAccountRepository` to the adapter in `internal/adapters/storage/factory.go` and construct the memory and PostgreSQL repositories in `newMemoryAdapter` and `newPostgresAdapter` (ADR 004; Principle IX).
- [X] T026 Create `internal/domain/thirdparty/service_account_service.go` with `ServiceAccountService`, its constructor taking the repository, `ports.EncryptionPort`, a service-existence lookup, a `ServiceAccountBindingChecker`, `now func() time.Time` and a logger (panic on a nil logger), the interfaces `ServiceAccountCounter` and `ServiceAccountBindingChecker`, `NoServiceAccountBindings`, and the methods `Create`, `List`, `Get`, `Resolve`, `Status`, `Update`, `Delete` returning zero values (plan §Design Notes; research R10; Principles VI, XII).
- [X] T027 Create `internal/adapters/http/handlers/admin/service_accounts_handler.go` with `ServiceAccountsHandler` (`Create`, `List`, `Get`, `Update`, `Delete`, `Status` answering `501`), add `ServiceAccounts` to `AdminHandlers` in `internal/app/handlers.go`, construct service and handler in `internal/app/builder.go`, and register the six routes under `/{service-id}/service-accounts` in `SetupAdminRoutes` in `internal/adapters/http/routing/admin.go`, including its route comment (API-001; Principle XII).
- [X] T028 Run `just build` and `just check`; confirm the E2E suites of T012–T016 now fail on `501` instead of `404`.

**Checkpoint**: Scaffold compiles; no business logic.

---

## Phase 2.5: Foundational Infrastructure

**Purpose**: Credential rules, entity rules, schema and both repositories, which every user story
needs. Tests first in each pair.

### Tests

- [ ] T029 [P] Write table tests in `internal/domain/model/service_account_credential_test.go` for every row of `data-model.md` §Validation of a `signed_jwt` credential, in order, with its reason and message: `CredentialFormat` and `CredentialUse` values (`assertion` refused), 16384 and 16385 bytes (refused before parsing), every row of the library table in research R4 with its broker outcome, `alg` `none`, `NONE` and missing, `exp` before, at and after the request time. Assert that no error text contains the input or any segment. Test `data-model.md` §Fingerprint (equal tokens equal, different tokens differ, format `SHA256:` plus 43 characters) and the `CredentialSummary` fields (`CredentialSummary.Iss`, `CredentialSummary.Sub`, `CredentialSummary.Kid` omitted when absent; `CredentialSummary.Exp` in whole seconds) (FR-007–FR-010, FR-012, SR-003, SR-007).
- [ ] T030 [P] Write tests in `internal/domain/model/service_account_test.go` for entity validation (`data-model.md` `ServiceAccount.CanonicalID`, `ServiceAccount.Kind`, `ServiceAccount.Scopes` per §Entity-level validation), `StatusAt` at `exp - 1ns`, at `exp` and after it, and without `exp` (`ServiceAccount.CredentialExpiresAt`), `Narrowing` for `jwt_bearer`, `CredentialSummary.LogValue` capping strings at 256 bytes and rendering `sub` as an `id.Principal` (assert the principal policy output through a test policy handler, LOGGING.md §9), and redacted `LogValue`, `String` and `GoString` of `ServiceAccountCredentialInput` (FR-002–FR-004, FR-014, FR-023).
- [ ] T031 [P] Write the shared repository contract in `tests/integration/storage/contract/service_account_repository.go` (every rule of `data-model.md` §Repository port: version 1 on create, unknown service, scoped reads and other-service not found, byte-ordered list, uniqueness within a service and reuse across services, rename and freeing of the old name, version precondition on update and delete, unconditional writes without one, `CountByService`, `StorageError` kinds and sentinels via `errors.Is`); run it on the memory store in `tests/integration/storage/service_account_repository_test.go`; add deep-copy and `-race` concurrency tests to `internal/adapters/storage/memory/service_account_repository_test.go` (DB-001, SC-008; Principle IX).
- [ ] T032 [P] Write `tests/integration/migrations/service_accounts_migration_test.go` (`integration` tag, shared migrated clones): apply, rollback and reapply of `050`, repeated twice, asserting that existing services and other rows are unchanged after each cycle (only `service_accounts` rows go on rollback, by design); the unique constraint; each `CHECK` of `data-model.md` §PostgreSQL schema (`ServiceAccount.CanonicalID` pattern, non-empty `ServiceAccount.Kind`, `ServiceAccount.Scopes` non-empty array, non-empty `ServiceAccount.Credential` ciphertext, object summary, `ServiceAccount.Version` ≥ 1); deleting a service that holds an account fails with `23503`; no column holds plaintext (DB-002, DB-003; Principle IX).
- [ ] T033 [P] Run the shared contract on PostgreSQL in `tests/integration/storage/infra/service_account_test.go`; write `internal/adapters/storage/postgres/service_account_repository_unit_test.go` (no build tag, no database) for error mapping to `StorageError` and the sentinels and for record conversion to and from the entity, as `signing_key_repo_unit_test.go` does (Principle IX: unit tests for both adapters); and in `internal/adapters/storage/postgres/service_account_repository_test.go` (`integration` tag) assert `23505` → canonical-ID sentinel, version mismatch → version sentinel, and the seven span names of `contracts/observability.md` §5 with kind `Client` and `storage.outcome` for not found and both conflicts (DB-001, DB-002, SC-008; Principle XV).

### Implementation

- [ ] T034 Implement the credential rules, summary and fingerprint in `internal/domain/model/service_account_credential.go` using `lestrrat-go/jwx/v4` for parsing (`jws.Parse` with `jws.WithCompact()`, `jwt.ParseInsecure`, no key, no verification) after the size check, and `crypto/sha256` for the fingerprint; never return or log a library error, applying the rules to `data-model.md` `ServiceAccountCredentialInput.Format`, `ServiceAccountCredentialInput.Use` (absent means `access_token`) and `ServiceAccountCredentialInput.JWT`, stopping at the first failed rule and returning `data-model.md` `CredentialValidationError.Reason` from its closed set; populate `CredentialSummary.Format`, `CredentialSummary.Use`, `CredentialSummary.Iss`, `CredentialSummary.Sub`, `CredentialSummary.Kid`, `CredentialSummary.Exp`, `CredentialSummary.Fingerprint` and the parsed expiry for `ServiceAccount.CredentialExpiresAt`; build the credential document of `data-model.md` §Credential document. Make T029 green (FR-008, FR-009, FR-012, SR-003, SR-007; DEC-FINGERPRINT, DEC-JWX).
- [ ] T035 Implement entity validation, `StatusAt`, `Narrowing`, `CredentialSummary.LogValue` (string fields capped at 256 bytes and `sub` rendered as `id.NewPrincipal`, per `data-model.md` `CredentialSummary`; the summary itself keeps `sub` as a string) and the redacting `slog.LogValuer`, `fmt.Stringer`, `fmt.GoStringer` methods of `ServiceAccountCredentialInput` in `internal/domain/model/service_account.go` and `service_account_credential.go`, per `data-model.md` `ServiceAccount.CanonicalID` (via `canonical.Validate`), `ServiceAccount.Kind`, `ServiceAccount.Scopes` (ascending byte order on store; only the ordering is added here), `ServiceAccount.CredentialExpiresAt`. Make T030 green (FR-002–FR-004, FR-014, FR-023; LOGGING.md §§5, 6.4).
- [ ] T036 Add `ServiceAccountCredentialInput` to `credential-types` in `.golangci.yml` next to `model.Secret`, so `telemetrylint` rejects it as a log or span attribute (LOGGING.md §§6.4, 11; FR-023).
- [ ] T037 Write `migrations/050_create_service_accounts.up.sql` and `migrations/050_create_service_accounts.down.sql` exactly as `data-model.md` §PostgreSQL schema (`ServiceAccount.ID`, `ServiceAccount.ServiceID`, `ServiceAccount.CanonicalID`, `ServiceAccount.Kind`, `ServiceAccount.Scopes`, `ServiceAccount.Credential` as `credential_encrypted`, `ServiceAccount.Summary` as `credential_summary`, `ServiceAccount.CredentialExpiresAt`, `ServiceAccount.Version`, `ServiceAccount.CreatedAt`, `ServiceAccount.UpdatedAt`), with table and column comments. Make T032 green (DB-002, DB-003; DB-050).
- [ ] T038 Implement `internal/adapters/storage/memory/service_account_repository.go`: one mutex, map by ID, index by `(service_id, canonical_id)`, deep copies, version rules and sentinels, service existence through an injected function wired in `internal/adapters/storage/factory.go`. Make the memory runs of T031 green (DB-001; data-model §In-memory adapter).
- [ ] T039 Implement `internal/adapters/storage/postgres/service_account_repository.go` with sqlx, explicit columns, the configured read and write timeouts, `version = $n` conditions for preconditions, error mapping to `StorageError` and the sentinels, and `startStorageSpan` with the names of `contracts/observability.md` §5. Make T033, including the unit tests, green (DB-001, DB-002; ADR 004; OPENTELEMETRY.md §3).
- [ ] T040 Add a description for each of the seven spans to `docs/reference/telemetry-descriptions.yaml`, run `just telemetry-doc` and commit `docs/reference/telemetry.md` (OPENTELEMETRY.md §10).
- [ ] T041 Run `just check`, `just test` and `just test-integration`; record the results in `specs/20261125-152113-service-accounts-stored-jwt/tasks.md`. `just test-integration-infra` runs locally only where a container runtime exists, otherwise in CI (Principles VIII, IX).

**Checkpoint**: Rules, schema and both repositories done; user stories can start.

---

## Phase 3: User Story 1 - Register a ready-signed JWT as a service account (Priority: P1) 🎯 MVP

**Goal**: An administrator attaches a JWT to an existing service and sees it listed and read as
`ready` with a summary, never the JWT.

**Independent Test**: Create an account on an existing service, list and read it; status `ready`,
narrowing `none`, no JWT in any response (spec User Story 1).

### Tests for User Story 1 [MANDATORY - Principle VIII] ⚠️

- [ ] T042 [P] [US1] Write tests in `internal/domain/thirdparty/service_account_service_test.go` for `Create`, `List`, `Get` and `Resolve`: the encryption call receives the credential document and exactly the owning service's `service_id` context; the repository receives ciphertext only; summary, `ServiceAccount.Version` 1, status `ready`; `service.service_account.created` with the keys of `contracts/observability.md` §1; unknown service → not found, nothing stored; name conflict → conflict, no audit; encryption failure → nothing stored, `service.service_account.write_failed` at `Error` without the cause; resolution by UUID and by `canonical_id` within the service; the JWT and its segments absent from a JSON buffer logger (FR-001, FR-002, FR-011, FR-016, FR-022, FR-023, SR-001, SR-002).
- [ ] T043 [P] [US1] Write tests in `internal/adapters/http/handlers/admin/service_accounts_handler_test.go` for `Create`, `List` and `Get`: `401` before any repository call; `404` for an unknown service and for an account of another service; `201` with UUID `Location` and `ETag: "1"`; list in byte order; single read with `ETag`; UUID and canonical addressing; body over 64 KiB and unknown members → `400`; no `credential` member in any response; the JWT and its segments absent from responses, the log buffer and an in-memory span recorder (API-001–API-004, SR-005, SR-006; research R11).
- [ ] T044 [P] [US1] Write `tests/integration/service_account_encryption_test.go` (self-contained, memory encryption backend): a credential stored for one service does not decrypt under another service's context; with a failing encryption port the store stays empty (SR-001, SR-002).

### Implementation for User Story 1

- [ ] T045 [US1] Implement `Create`, `List`, `Get` and `Resolve` in `internal/domain/thirdparty/service_account_service.go`: validate, summarize, encrypt with `encryption.NewServiceBranchKeySubject(serviceID).EncryptionContext()`, store, audit after commit; scoped reads with status from `now()` and no decryption. Make T042 green (FR-001–FR-003, FR-011–FR-016, FR-022, SR-001, SR-002).
- [ ] T046 [US1] Implement `Create`, `List` and `Get` in `internal/adapters/http/handlers/admin/service_accounts_handler.go`: operator principal via `operatorPrincipalFromContext` first, service resolution through `ThirdpartyOAuth2ProviderService.ResolveID`, account resolution, `http.MaxBytesReader` (64 KiB), `DisallowUnknownFields`, the representation of the contract with `failure_reason` only when failed, `Location`, `strongETag`. Make T043 green (API-001–API-004, SR-005).
- [ ] T047 [US1] Wire `ServiceAccountService` in `internal/app/builder.go` with the storage repository, the encryption adapter, the provider repository as existence lookup, `NoServiceAccountBindings`, `time.Now` and the builder logger; pass it to the handler. Run the User Story 1 E2E tests (memory) and confirm Scenarios 1–4, 6 and 7 pass (Principle XII; US1.1–US1.7).

**Checkpoint**: User Story 1 works on its own; Scenario 5 passes on both stores where containers run.

---

## Phase 4: User Story 2 - Refuse credentials that cannot work (Priority: P1)

**Goal**: Every unusable credential and invalid member is refused at write time with the failed
rule, without echoing the JWT, and nothing is stored.

**Independent Test**: Submit each invalid credential; each answers `400`, the list is unchanged,
no response contains the JWT (spec User Story 2).

### Tests for User Story 2 [MANDATORY - Principle VIII] ⚠️

- [ ] T048 [P] [US2] Add tests to `internal/domain/thirdparty/service_account_service_test.go`: each credential rule and each entity rule on create stores nothing and writes no `created` record; a credential refusal writes one `service.service_account.credential_rejected` record at `Warn` with `operation` and the enumerated `reason`; neither record nor error contains the input (FR-009, FR-010, FR-023; observability §2).
- [ ] T049 [P] [US2] Add tests to `internal/adapters/http/handlers/admin/service_accounts_handler_test.go` mapping each refusal to `400` with `error: validation failed` and the message of the contract's `ServiceAccountValidationError` examples, including unknown `kind`, unknown `format`, `use: assertion`, `null` members, empty or missing `scopes`, missing `credential` and invalid `canonical_id` (FR-006, FR-007, FR-010; API-002, API-004).

### Implementation for User Story 2

- [ ] T050 [US2] Add the `credential_rejected` record and the refusal flow to `Create` in `internal/domain/thirdparty/service_account_service.go`, so `CredentialValidationError` and entity errors return before any encryption or repository call. Make T048 green (FR-009, FR-010).
- [ ] T051 [US2] Map domain validation errors to the error envelope in `internal/adapters/http/handlers/admin/service_accounts_handler.go`, using only the constant messages. Make T049 green and run the User Story 2 E2E tests (US2.1–US2.7; SC-003).

**Checkpoint**: User Stories 1 and 2 work; MVP complete.

---

## Phase 5: User Story 3 - Rotate the credential and change the ceiling (Priority: P2)

**Goal**: Replace the credential in place, change the ceiling or rename, keeping the account's
identity, with optional optimistic concurrency.

**Independent Test**: Rotate, then change only the ceiling, then rename; same ID throughout, new
summary and ETag where something changed (spec User Story 3).

### Tests for User Story 3 [MANDATORY - Principle VIII] ⚠️

- [ ] T052 [P] [US3] Add `Update` tests to `internal/domain/thirdparty/service_account_service_test.go` for every row of `data-model.md` §Update outcomes: rotation (new summary, new ciphertext, `failed` → `ready`), ceiling change (credential and summary untouched), rename, combined change, no-op including reordered ceiling and resubmitted JWT (no write, same `ServiceAccount.Version` and `ServiceAccount.UpdatedAt`, no record), `kind` change refused, invalid credential leaves everything unchanged and writes `credential_rejected`, encryption failure during a rotation leaves the stored credential, summary and version unchanged and writes `write_failed` without the cause, name conflict, stale version, request errors before a stale version, other-service account not found, audit `event` precedence with `changes` and `previous_observed_*` keys (FR-005, FR-017–FR-019, FR-021, FR-022; research R7).
- [ ] T053 [P] [US3] Add `Update` tests to `internal/adapters/http/handlers/admin/service_accounts_handler_test.go`: `If-Match` absent, current, stale → `412`, weak or malformed → `400`; `scopes` required; `null` members refused; `200` with the new or unchanged `ETag`; the JWT absent from responses, logs and spans (API-004, SR-006; research R8).

### Implementation for User Story 3

- [ ] T054 [US3] Implement `Update` in `internal/domain/thirdparty/service_account_service.go`: load, supplied-version check, candidate validation, no-op detection by `canonical_id`, ceiling set and fingerprint/format/use, encryption of a new credential only (an encryption failure returns before any write), `repo.Update` with the expected version, one audit record per committed write. Make T052 green (FR-017–FR-019, FR-022).
- [ ] T055 [US3] Implement `Update` in `internal/adapters/http/handlers/admin/service_accounts_handler.go` with `parseStrongETag` for `If-Match` and the check order of `plan.md` §Design Notes; map the version sentinel to `412` and the name sentinel to `409`. Make T053 green and run the User Story 3 E2E tests (US3.1–US3.9; SC-005, SC-006).

**Checkpoint**: User Stories 1–3 work.

---

## Phase 6: User Story 4 - See whether an account is usable (Priority: P2)

**Goal**: An administrator or monitoring job reads `ready` or `failed` with `credential_expired`,
derived from the stated expiry at the time of the request.

**Independent Test**: Register a token expiring shortly, read status before and after expiry,
rotate, read again (spec User Story 4).

### Tests for User Story 4 [MANDATORY - Principle VIII] ⚠️

- [ ] T056 [P] [US4] Add `Status` and list tests with an injected clock to `internal/domain/thirdparty/service_account_service_test.go`: one nanosecond before `ServiceAccount.CredentialExpiresAt` → `ready`, at it and after → `failed` / `credential_expired`, nil expiry → `ready` at any time, the list reports the same; no write and no record on any read (FR-014, FR-015; SC-004; research R6).
- [ ] T057 [P] [US4] Add `Status` tests to `internal/adapters/http/handlers/admin/service_accounts_handler_test.go`: the `ServiceAccountStatus` body with `failure_reason` and `credential_expires_at` as `null` when not applicable, RFC 3339 UTC expiry, `ETag`, `404` for an unknown account, `401` before any read (FR-015, SR-005).

### Implementation for User Story 4

- [ ] T058 [US4] Implement `Status` in `internal/domain/thirdparty/service_account_service.go` using `StatusAt(now())`. Make T056 green (FR-014, FR-015).
- [ ] T059 [US4] Implement `Status` in `internal/adapters/http/handlers/admin/service_accounts_handler.go`. Make T057 green and run the User Story 4 E2E tests (US4.1–US4.4).

**Checkpoint**: User Stories 1–4 work.

---

## Phase 7: User Story 5 - Remove a service account (Priority: P3)

**Goal**: An administrator removes an account and its credential; a service that still holds
accounts cannot be deleted; deletion of a bound account is refused once agent grants exist.

**Independent Test**: Delete an account, list (absent), delete again (`404`); delete a service
with an account (`409`) (spec User Story 5).

### Tests for User Story 5 [MANDATORY - Principle VIII] ⚠️

- [ ] T060 [P] [US5] Add `Delete` tests to `internal/domain/thirdparty/service_account_service_test.go`: `service.service_account.deleted` after commit with the pre-delete name, ceiling and summary; not found; stale version; a `ServiceAccountBindingChecker` that answers true → conflict `service account is bound by an agent grant`, nothing deleted, no record (FR-020 refusal path; its acceptance scenario belongs to #472); a checker error propagates (FR-020, FR-022; research R10).
- [ ] T061 [P] [US5] Add tests to `internal/domain/thirdparty/service_test.go` for `ThirdpartyOAuth2ProviderService.Delete`: with N accounts → conflict `cannot delete service: N service account(s) are attached to it` and no repository delete; with none → deletion proceeds; a counter error propagates (FR-025; API-SVC-DELETE).
- [ ] T062 [P] [US5] Add a test to `internal/adapters/storage/memory/thirdparty_provider_test.go` that deleting a provider is refused with a conflict while the injected counter reports accounts, and add a case to `internal/adapters/storage/postgres/thirdparty_provider_test.go` (`integration` tag) that the `RESTRICT` foreign key surfaces as a conflict (FR-025, DB-002).
- [ ] T063 [P] [US5] Add `Delete` tests to `internal/adapters/http/handlers/admin/service_accounts_handler_test.go` (`204`, `404`, `409`, `412`, `400` for a malformed `If-Match`, `401` before any read) and a case to `internal/adapters/http/handlers/admin/services_handler_test.go` that `DELETE /api/services/{service-id}` answers `409` with the service-account message (FR-020, FR-025; API-004).

### Implementation for User Story 5

- [ ] T064 [US5] Implement `Delete` in `internal/domain/thirdparty/service_account_service.go`: load, supplied-version check, binding check, `repo.Delete` with the expected version, audit after commit. Make T060 green (FR-020, FR-022).
- [ ] T065 [US5] Add `WithServiceAccounts(ServiceAccountCounter)` and the guard to `Delete` in `internal/domain/thirdparty/service.go`, next to the permission-set guard, and inject the counter in `internal/app/builder.go`. Make T061 green (FR-025).
- [ ] T066 [US5] Add `WithServiceAccountRepository` to `internal/adapters/storage/memory/thirdparty_provider.go`, check the count under the write lock in `Delete`, and wire it in `newMemoryAdapter` in `internal/adapters/storage/factory.go`. Make T062 green (FR-025, DB-002).
- [ ] T067 [US5] Implement `Delete` in `internal/adapters/http/handlers/admin/service_accounts_handler.go` and confirm `DeleteService` in `internal/adapters/http/handlers/admin/services_handler.go` returns the guard's message through its existing conflict path. Make T063 green and run the User Story 5 E2E tests (US5.1–US5.4).

**Checkpoint**: All user stories work.

---

## Phase 8: Documentation and Validation

- [ ] T068 [P] Add an "Add a service account" section to `docs/guides/manage-agents-and-services.md`: what a service account is, create, list, read, status, rotate, change the ceiling, rename, remove, the 16 KiB and form rules, that the broker never shows or verifies the JWT, that an expired token marks the account failed until replaced, and that a service with accounts cannot be deleted. End-user content only (AGENTS.md §Documentation Scope; SC-001).
- [ ] T069 [P] Add the six service-account operations to the Admin API map in `docs/reference/api.md` under Services (API-001).
- [ ] T070 [P] Update `internal/AGENTS.md` (AdminHandlers list, Key Storage Types row for `ServiceAccount`) and the admin API, data store §4.1 and directory text of `ARCHITECTURE.md` for the new handler, table and domain service (Principles II, XII).
- [ ] T071 Run the validation steps of `specs/20261125-152113-service-accounts-stored-jwt/quickstart.md` §§1–4 and 6 (`just check`, `just test`, `just test-integration`, the memory E2E focus run, `just telemetry-doc`, then `just verify` where a container runtime exists) and record the results, naming any suite left to CI, in `specs/20261125-152113-service-accounts-stored-jwt/tasks.md` (Principles VIII, XIII).

---

## 🔒 Phase N: Constitution Compliance Verification [MANDATORY COMPLIANCE SECTION]

**Purpose**: Verify constitution requirements and feature-scope compliance before the feature is
done.

### 🔒 Constitution Compliance Verification [MANDATORY]

#### Design Phase Verification [MANDATORY]

- [ ] T072 Verify every task above traces to a requirement, scenario, success criterion or binding constraint, and that no task hands out a credential, changes token exchange, binds an agent or changes the UI (Feature Scope; FR-024).
- [ ] T073 Verify the six new terms are in the `ARCHITECTURE.md` glossary and the `AGENTS.md` glossary table (Principle V).
- [ ] T074 Verify no file in `examples/config/` or `charts/agentic-identity-broker/` changed and none was needed (Principle VII).
- [ ] T075 Verify `api/admin/openapi.yaml` holds the service-account operations at version `3.1.0` and that the PR description lists the API changes for the maintainer's review (Principles IV, X).
- [ ] T076 Verify the schema design and migration `050` match `data-model.md` (Principle IX).
- [ ] T077 Verify the three spec journeys link to their scenarios and agree with the merged contract, with decoded claims for T1, T2 and U and only synthetic tokens; no configuration, Wireloom or frontend journey applies (Principle XIV).
- [ ] T078 Verify `tests/e2e/service_accounts_register_test.go` and `tests/e2e/service_accounts_lifecycle_test.go` hold one `It()` for each of the 31 scenarios of `spec.md`, as mapped in `plan.md`, written before implementation with concrete expectations, without `XIt`, `PIt`, `Skip()` or red-phase comments; red-run evidence is not required (Principle XIII).
- [ ] T079 Verify the log events and storage spans match `contracts/observability.md`, no metric was added, and no new span attribute namespace was needed (Principle XV).

#### Implementation Phase Verification [MANDATORY]

**Feature Scope**:
- [ ] T080 Verify unrelated findings were reported to the user, not fixed, and that only user-approved issue URLs appear under `Future Work` in this file.

**API & Documentation** (Principles IV, X):
- [ ] T081 [P] Verify the handler behaviour matches `api/admin/openapi.yaml` exactly: status codes, headers, members, messages, `If-Match` semantics.
- [ ] T082 [P] Verify no `docs/api/` page is needed (admin-only change) and that `docs/guides/manage-agents-and-services.md` and `docs/reference/api.md` describe the operations.

**Architecture & Documentation** (Principle II):
- [ ] T083 Verify `ARCHITECTURE.md` describes the entity, table, handler, domain service and Admin API `3.1.0`.
- [ ] T084 [P] Verify `adrs/20261125-154231-service-accounts-stored-jwt.md` is Accepted, agrees with the code, and is listed in `ARCHITECTURE.md` §10 and `AGENTS.md`; no accepted ADR merged into main was deviated from.

**Configuration** (Principle VII):
- [ ] T085 [P] Verify no configuration parameter was added and nothing reads configuration outside `internal/ports/config.go`.

**Database & Persistence** (Principle IX):
- [ ] T086 [P] Verify migration `050` is atomic, sequentially numbered, uses explicit columns, and has apply, rollback and reapply tests.
- [ ] T087 [P] Verify the PostgreSQL repository is covered by the shared contract and its own integration tests, and both stores pass the same contract (SC-008).
- [ ] T088 Verify the port has seven methods in `internal/ports/storage.go`, errors are `StorageError`, and the adapter exposes `ServiceAccounts()` (ADR 004).

**Security** (Principles I, III):
- [ ] T089 Verify encryption is mandatory with no plaintext path, `alg: none` is refused, the operator principal is required before any read, and the stored JWT drives no decision (SR-001, SR-002, SR-004, SR-005, FR-013).
- [ ] T090 [P] Verify the JWT is parsed only through `lestrrat-go/jwx/v4` without verification, the fingerprint uses `crypto/sha256`, storage uses the existing encryption adapters, and there is no hand-written parser, signature operation or custom cryptography (SR-003).
- [ ] T091 [P] Verify audit records exist for create, rotation, ceiling change, rename and delete, after commit only, following `coding-standards/LOGGING.md` §8 (FR-022, SC-006).

**Coding Standards** (Principle XV):
- [ ] T092 Verify every added log call and span satisfies the review checklists of `coding-standards/LOGGING.md` §10 and `coding-standards/OPENTELEMETRY.md` §8, and `docs/reference/telemetry.md` is current.
- [ ] T093 Verify the credential-absence tests of `contracts/observability.md` §8 exist for every path (responses, logs, spans) (SR-006, SC-002).
- [ ] T094 Verify no lint rule is disabled or suppressed; the `LOGGING.md` §6.2 amendment and the `credential-types` addition are the only standard changes, and the PR description states the amendment and its rationale.

**Architecture Patterns** (Principles VI, XII):
- [ ] T095 Verify rules, no-op detection, status and audit live in `internal/domain`; the handler parses and maps only; no handler calls a port directly; all wiring is in `internal/app/builder.go`; routes are registered only in `SetupAdminRoutes`.

**Testing** (Principle VIII):
- [ ] T096 Verify unit tests were written first and drove the design; red-run evidence is not required.
- [ ] T097 Verify tests changed minimally during implementation.
- [ ] T098 Verify unit and integration tests cover every rule and repository method, and no Bash script validates code correctness.

**E2E Acceptance Testing** (Principle XIII):
- [ ] T099 Verify each `It()` maps to exactly one scenario and carries its scenario comment; `Describe` → `Context` → `It` structure; fixtures from `tests/e2e/fixtures/service_account.go`.
- [ ] T100 Verify the E2E tests changed minimally during implementation (fixture adjustments only) and turned green.
- [ ] T101 Run `just test-e2e-backend`; run `just test-e2e-docker` where a container runtime exists, otherwise confirm the `docker` scenarios pass in CI on the pushed commit. All must pass.

---

## Dependencies & Execution Order

### Phase Dependencies

- **Setup (Phase 1)**: none.
- **Design Preconditions (Phase 2)**: after Setup; 2a, 2b, 2c, 2d, 2f and 2g can proceed in parallel. The E2E tests (T012–T016) need only T010 and T011.
- **Entity Boilerplate (Phase 2.7)**: after all of Phase 2. Commit separately from business logic.
- **Foundational (Phase 2.5)**: after Phase 2.7. Blocks every user story.
- **User Stories (Phases 3–7)**: after Phase 2.5.
- **Documentation and Validation (Phase 8)**: after the user stories it describes.
- **Compliance (Phase N)**: last.

### User Story Dependencies

- **US1 (P1)**: after Phase 2.5. Creates the data every other story reads.
- **US2 (P1)**: after US1 (refusals are part of `Create`).
- **US3 (P2)**: after US1. Its invalid-credential scenario (US3.4) reuses the rules of Phase 2.5, not US2 code.
- **US4 (P2)**: after US1. US3.3 also needs US4's expiry behaviour, which `StatusAt` (T035) already provides.
- **US5 (P3)**: after US1. The service-delete guard (T065, T066) is independent of US2–US4.

### Within Each User Story

Tests before implementation; domain service before handler; handler before the story's E2E run.

### Parallel Opportunities

- Phase 2: T003 and T004; T010 and T011; then T012–T016 by different authors (two files: register and lifecycle).
- Phase 2.7: T023 and T024.
- Phase 2.5: T029–T033 together; then T034 with T037, T035 after T034.
- Each story: its `[P]` test tasks together (domain test file and handler test file).
- After US1: US3, US4 and US5 in parallel, as each touches different methods of the same two files; merge carefully.
- Phase 8: T068–T070.

---

## Parallel Example: User Story 1

```bash
# Tests for User Story 1, written together:
Task: "Create/List/Get/Resolve tests in internal/domain/thirdparty/service_account_service_test.go"
Task: "Create/List/Get handler tests in internal/adapters/http/handlers/admin/service_accounts_handler_test.go"
Task: "Service-context isolation test in tests/integration/service_account_encryption_test.go"

# Then, in order:
Task: "Create/List/Get/Resolve in internal/domain/thirdparty/service_account_service.go"
Task: "Create/List/Get in internal/adapters/http/handlers/admin/service_accounts_handler.go"
Task: "Builder wiring in internal/app/builder.go, then the User Story 1 E2E run"
```

## Parallel Example: Phase 2.5

```bash
Task: "Credential rule tests in internal/domain/model/service_account_credential_test.go"
Task: "Entity tests in internal/domain/model/service_account_test.go"
Task: "Shared repository contract in tests/integration/storage/contract/service_account_repository.go"
Task: "Migration tests in tests/integration/migrations/service_accounts_migration_test.go"
Task: "PostgreSQL contract run in tests/integration/storage/infra/service_account_test.go"
```

---

## Implementation Strategy

### MVP First (User Stories 1 and 2)

1. Phase 1: read the area instructions and reserve migration `050`.
2. Phase 2: design checks, ADR, contract merge, observability amendment, E2E tests written.
3. Phase 2.7: scaffold, committed on its own.
4. Phase 2.5: rules, migration, repositories.
5. Phase 3 (US1) and Phase 4 (US2): an administrator can register, recognise and is protected from unusable credentials.
6. Stop and validate: the US1 and US2 E2E tests pass; credential-absence tests pass.

### Incremental Delivery

1. US1 + US2 → reviewable inventory of machine credentials (MVP).
2. US3 → rotation, ceiling change, rename, concurrency.
3. US4 → status for monitoring.
4. US5 → removal and the service-delete guard.

Each step leaves earlier stories working. #472 then adds agent grants, the real binding checker
and the acceptance scenario for refusing to delete a bound account.

---

## Notes

- `[P]` tasks touch different files and depend on no unfinished task in their batch.
- `[USn]` maps a task to spec User Story n.
- Commit after each task or logical group; the Phase 2.7 scaffold is its own commit.
- The DCO rule applies to every commit on this branch.

---

## Design notes

### Configuration (T005)

No configuration parameter is added. `internal/ports/config.go`, `docs/configuration.md`,
`examples/config/` and `charts/agentic-identity-broker/` are unchanged. The 16 KiB JWT limit is a
fixed value in the API contract (`SignedJwtCredential.jwt.maxLength`) and in the domain rule. The
chart's existing migration Job applies every file in `migrations/`, so it applies `050` without a
chart change.

### Journey review (T009)

Journeys 1–3 agree with the merged contract (`api/admin/openapi.yaml` 3.1.0):

- Create answers `201` with a UUID `Location` although the request addressed the service by its
  canonical ID, and `ETag: "1"`; read, update and status carry the strong ETag.
- Journey 2 Steps 1–2 show the status bodies without the `ETag` header the contract also sends; the
  journey illustrates the body, so this is not a contradiction and `spec.md` is unchanged.
- Journey 3 messages (`credential.jwt must be a signed compact JWS`,
  `credential.use must be access_token`, `service account canonical_id already exists on this
  service`) are the contract's examples.
- Tokens T1, T2 and U are decoded in the spec; the fingerprints shown there are synthetic. The E2E
  tests generate their tokens per test and compute the expected fingerprint from the token they
  submit.
- No configuration block applies.

### Observability amendment (T019) — paragraph for the PR description

> **Coding-standard amendment (Principle XV):** `coding-standards/LOGGING.md` §6.2 gains one row:
> the SHA-256 fingerprint (`SHA256:` and the base64 digest) of a stored service-account credential
> is permitted in `service.service_account.*` audit records, never the credential, a fragment of it
> or its ciphertext. FR-022 requires audit records to carry the credential summary so that a
> rotation can be recognised. §6.2 already permits `iss`, `kid` and `exp`, and permits `sub` as an
> `id.Principal` through the principal policy, but had no row for a credential fingerprint. The
> digest cannot be reversed to the credential, and the row is scoped to these audit events.
> `ServiceAccountCredentialInput` is added to `credential-types` in `.golangci.yml`.
