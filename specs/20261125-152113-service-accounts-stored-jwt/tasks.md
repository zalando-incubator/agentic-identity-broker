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
- Sign-off inventory: API-SA, API-SHAPE, API-UPDATE, API-SVC-DELETE, API-VERSION,
  DEC-FINGERPRINT, DEC-EXP-RANGE, STD-LOGGING, ADR-SA and DB-050 in `plan.md` §Approval Items.
  T001 records an approval the maintainer has given; it does not grant one. A task that depends
  on an item without a recorded approval stays unchecked; independent work continues.
- User Story 5 Scenario 3 (delete refused while an agent grant binds the account) gets its E2E
  test with #472, as the spec states. This feature covers the refusal path with a domain unit test
  (T061) and defines the `409` in the contract (T008).
- Findings outside this scope are described to the user, not fixed here. Only user-approved issue
  URLs are recorded, under a `Future Work` note in this file.

---

## Phase 0: Pre-implementation Refactoring [OPTIONAL]

Skipped. No existing code is restructured (plan §Implementation Phase Overview). The two
additions to existing types, the provider service's delete guard and the memory provider
repository's service-account counter, are feature behaviour and belong to User Story 5.

---

## Phase 1: Setup

- [ ] T001 Record the maintainer's approval of each item in `plan.md` §Approval Items (login, date, link to the PR or issue comment) in a new `## Approval record` section of `specs/20261125-152113-service-accounts-stored-jwt/tasks.md`. Record only approvals actually given; list any item still open (Principles II, X; API-006).
- [ ] T002 Read `internal/AGENTS.md`, `internal/domain/AGENTS.md`, `internal/domain/id/AGENTS.md`, `internal/ports/AGENTS.md`, `internal/adapters/AGENTS.md`, `tests/e2e/AGENTS.md` and `tests/integration/AGENTS.md`; confirm that `050` is the next free number in `migrations/` and that `ServiceAccountID` is not yet in `internal/domain/id/gen_ids.go` (DB-002; Principles II, IX).

---

## 🔒 Phase 2: Design Preconditions (Blocking Prerequisites) [MANDATORY]

**Purpose**: Domain, configuration, API, database, journey, E2E and observability design before
production code.

**⚠️ CRITICAL**: No production code before this phase completes. The API items need the T001
approval first.

### Phase 2a: Domain Model & Glossary [MANDATORY]

**Constitution Reference**: Principles II, V

- [ ] T003 Verify `specs/20261125-152113-service-accounts-stored-jwt/data-model.md` against `spec.md` §Domain Model and FR-001–FR-025: entity fields, value objects, derived `Narrowing` and `StatusAt`, update outcomes, domain events, invariants (one service for life, non-empty ceiling, exactly one credential, ID and kind fixed, `canonical_id` renamable). Correct `data-model.md` only where it contradicts the spec (Principle V).
- [ ] T004 [P] Add a "Service Accounts" glossary section to `ARCHITECTURE.md` §12 and rows to the Domain Glossary table in `AGENTS.md` for Service Account, Kind, Credential Format, Ceiling, Narrowing and Credential Summary, with the definitions of #478 and their relationships to ThirdpartyOAuth2Service (spec §Domain Model; Principle V).
- [ ] T005 [P] Write `adrs/20261125-154231-service-accounts-stored-jwt.md` (Context, Decision, Consequences, Status) covering research R2, R3, R6, R7, R9, R15 and SR-004: the entity and its repository port, the stored-JWT posture and why it is not a Principle I exception, `service_id` encryption of a format-discriminated credential document, status derived on read, optional `If-Match`, `ON DELETE RESTRICT`. Set Status to Accepted only when T001 records ADR-SA. Add the ADR to `ARCHITECTURE.md` §10 and to the ADR Decision Index in `AGENTS.md` (ADR-SA; Principle II).

**Checkpoint**: Domain model documented, glossary updated, ADR drafted.

### Phase 2b: Configuration Design [MANDATORY]

**Constitution Reference**: Principle VII

- [ ] T006 Confirm that no configuration parameter is added: no change to `internal/ports/config.go`, `docs/configuration.md`, `examples/config/` or `charts/agentic-identity-broker/`; the 16 KiB limit is fixed in the contract; the chart's existing migration Job applies `050`. Record the result in `specs/20261125-152113-service-accounts-stored-jwt/tasks.md` under `## Approval record` (spec Assumptions; Principle VII).

**Checkpoint**: No configuration or Helm change, confirmed.

### Phase 2c: API Design [MANDATORY]

**Constitution Reference**: Principles IV, X

- [ ] T007 After T001 records API-SA, API-SHAPE, API-UPDATE, API-SVC-DELETE and API-VERSION, merge `specs/20261125-152113-service-accounts-stored-jwt/contracts/admin-service-accounts-openapi.yaml` into `api/admin/openapi.yaml`: `info.version` `3.1.0`, a `Service Accounts` tag, the three paths, the parameters, header, responses and schemas, and the extra `409` cause on `DELETE /api/services/{service-id}` (reuse the existing `ServiceId`, `ErrorResponse`, `NotFoundError`, `InternalServerError` components). Leave every other operation unchanged (API-001–API-006; Principles IV, X).
- [ ] T008 Verify the merged contract against Journeys 1–3 of `spec.md` (status codes, headers, bodies, messages) and FR-020's delete conflict; add the Admin API `3.1.0` note to the API versioning paragraph of `ARCHITECTURE.md` §5; confirm `api/enduser/openapi.yaml` is unchanged (API-004, API-005; Principles II, IV).

**Checkpoint**: Contract merged after recorded sign-off.

### Phase 2d: Database Design [MANDATORY]

**Constitution Reference**: Principle IX

- [ ] T009 Verify the `service_accounts` schema in `data-model.md` §PostgreSQL schema against DB-001–DB-003 and FR-025: every column, `CHECK`, `uq_service_accounts_service_canonical_id`, `ON DELETE RESTRICT`, no plaintext column, explicit-column queries, one atomic pair `050_create_service_accounts.{up,down}.sql` without `-- migrate:no-transaction` (DB-050; Principle IX). The SQL files are written in T038 after their tests.

**Checkpoint**: Schema designed; migration number reserved.

### Phase 2e: Frontend/Design System Review [MANDATORY IF FRONTEND]

Not applicable. The feature changes no React UI, so no design-system review, Wireloom sequence,
Playwright test or screenshot applies (spec Assumptions: the admin UI comes later; Principle XI).

### Phase 2f: E2E Acceptance Test Design [MANDATORY]

**Constitution Reference**: Principles XIII, XIV

E2E tests reach the broker only over HTTP, so they compile before the scaffold; without routes
they fail on `404`, after Phase 2.7 on `501`.

- [ ] T010 Review Journeys 1–3 in `spec.md` against their linked scenarios and the contract delta: HTTP exchanges, `Location` with the UUID path although the request used canonical IDs, ETag on create, read, update and status, decoded claims of tokens T1, T2 and U, and the absence of configuration. Record findings in `specs/20261125-152113-service-accounts-stored-jwt/tasks.md`; change `spec.md` only with the user's approval (Principle XIV).
- [ ] T011 [P] Create `tests/e2e/fixtures/service_account.go`: the *Company Platform* service (canonical ID `company-platform`) built on `fixtures.ServiceWithID`; ES256 tokens signed with a key generated per call carrying the T1 and T2 header and claims of the spec (`kid`, `iss`, `sub`, `aud`, `scope`, `iat`, `exp`); a token without `exp`; a token whose `exp` lies a given number of seconds ahead; `alg` variants `none`, `NONE`, `None` and missing; a token of 16385 bytes; a five-segment token; a token with surrounding whitespace; a token whose payload is not a JSON object; a token with a string `exp`; create and update request bodies. All values synthetic (US1–US5; Principle XIII).
- [ ] T012 [P] Create `tests/e2e/helpers/service_account.go` with a decoder for the `ServiceAccount` and `ServiceAccountStatus` representations and a Gomega matcher that fails when a response body or header contains a given JWT or any of its three segments; create `tests/e2e/service_accounts_backend_test.go` with `withServiceAccountBackend(backend string, action func(*bootstrap.TestServer, *storageadapter.Adapter))` for `memory` and `postgres`, following `withExternalIDBackend` in `tests/e2e/agent_external_id_backend_test.go` (SR-006, SC-008; Principle XIII).
- [ ] T013 [US1] Write the seven User Story 1 `It()` blocks in `tests/e2e/service_accounts_register_test.go` (Scenarios 1–7) under `Describe("Service accounts")`, one `Context` per precondition, each preceded by `// Scenario N from specs/20261125-152113-service-accounts-stored-jwt/spec.md (User Story 1)`, with the descriptions of `plan.md` §Scenario Mapping. Scenario 5 carries `Label("docker")` and runs both store cases through `withServiceAccountBackend`. Assert status, `Location`, `ETag`, every representation member and summary value, list contents, and the credential-absence matcher on every response (US1.1–US1.7; FR-002, FR-011, FR-012, FR-016; SC-001).
- [ ] T014 [US2] Write the seven User Story 2 `It()` blocks in `tests/e2e/service_accounts_register_test.go` (Scenarios 1–7). Scenarios 1, 2, 5 and 6 iterate over a table of variants inside one `It()`; every variant asserts `400`, the contract message, an unchanged list and the credential-absence matcher (US2.1–US2.7; FR-006, FR-007, FR-009, FR-010; SC-003).
- [ ] T015 [US3] Write the nine User Story 3 `It()` blocks in `tests/e2e/service_accounts_lifecycle_test.go` (Scenarios 1–9). Scenarios 7 and 9 carry `Label("docker")` and run both store cases. Scenario 3 registers a token expiring a few seconds ahead and waits for `failed` with `Eventually` before rotating; no fixed sleep (US3.1–US3.9; FR-005, FR-017–FR-019; SC-005).
- [ ] T016 [US4] Write the four User Story 4 `It()` blocks in `tests/e2e/service_accounts_lifecycle_test.go` (Scenarios 1–4); Scenario 2 uses `Eventually` on both the status read and the list (US4.1–US4.4; FR-014, FR-015; SC-004).
- [ ] T017 [US5] Write the User Story 5 `It()` blocks for Scenarios 1, 2, 4 and 5 in `tests/e2e/service_accounts_lifecycle_test.go`. Scenario 5 carries `Label("docker")`, runs both store cases, and asserts `409` on service deletion, the unchanged service and account, then `204` after the account is removed. Scenario 3 is not written here (Feature Scope) (US5.1, US5.2, US5.4, US5.5; FR-020, FR-021, FR-025).
- [ ] T018 Check both suites: `Describe` → `Context` → exactly one `It()` per scenario, scenario comments, fresh server and storage per test, fixtures from T011, no `XIt`, `PIt`, `Skip()` or red-phase comments. Run `ginkgo -v --label-filter="!performance && !docker" --focus "Service accounts" ./tests/e2e/` and confirm each test fails on a concrete assertion. Add `file:line` to each row of `plan.md` §Scenario Mapping (Principle XIII).

**Checkpoint**: Journeys reviewed; 31 E2E scenarios written and failing semantically.

### Phase 2g: Observability Design [MANDATORY IF TELEMETRY]

**Constitution Reference**: Principle XV

- [ ] T019 Verify `contracts/observability.md` against `coding-standards/LOGGING.md` §§2, 3, 6, 8, 10 and `coding-standards/OPENTELEMETRY.md` §§3, 4, 6, 8: event names, levels, canonical keys, the seven `storage.*.service_account` spans, no new span attribute namespace and no metric instrument (so no metric entry and no §4.1 registry change), and the credential-path test matrix in §8 (Principle XV).
- [ ] T020 After T001 records STD-LOGGING, add the service-account credential row of `contracts/observability.md` §4 to the table in `coding-standards/LOGGING.md` §6.2, and draft the PR-description paragraph stating the change and its rationale in `specs/20261125-152113-service-accounts-stored-jwt/tasks.md` (FR-022, FR-023; Principle XV).

**Checkpoint**: Observability design checked against the standards; amendment applied.

---

## Phase 2.7: Entity Boilerplate (New Entity: ServiceAccount)

**Purpose**: Empty but compiling scaffold, committed separately ahead of business logic (plan
§Implementation Phase Overview). Handlers answer `501`; repositories answer not found or not
implemented; domain functions return zero values.

- [ ] T021 Add `{"ServiceAccountID", "service_account"}` to `uuidTypes` in `internal/domain/id/gen_ids.go`, run `go generate ./internal/domain/id/`, commit `internal/domain/id/uuid_ids_gen.go`, add parse and serialization cases to `internal/domain/id/id_test.go`, and list the type in `internal/domain/id/AGENTS.md` (`data-model.md` `ServiceAccount.ID`; ADR 013).
- [ ] T022 Create `internal/domain/model/service_account.go` and `internal/domain/model/service_account_credential.go` with the types of `data-model.md` §Entity and §Value objects: `ServiceAccount` (all fields of `data-model.md` §Entity, `ServiceAccount.ID` through `ServiceAccount.UpdatedAt`), `ServiceAccountKind`, `CredentialFormat`, `CredentialUse`, `Narrowing`, `ServiceAccountStatus`, `ServiceAccountFailureReason` (typed string constants, each with a doc comment), `CredentialSummary`, `ServiceAccountCredentialInput`, `CredentialValidationError`, and signatures for the validation, summary, `StatusAt` and `Narrowing` functions with empty bodies (Principle VIII scaffold).
- [ ] T023 Add `ServiceAccountRepository` with the seven methods of `data-model.md` §Repository port, and the sentinels `ErrServiceAccountCanonicalIDConflict` and `ErrServiceAccountVersionMismatch`, to `internal/ports/storage.go` with the contract as doc comments; add a row to the catalogue in `internal/ports/AGENTS.md` (DB-001; ADR 004; Principle IX).
- [ ] T024 [P] Create `internal/adapters/storage/memory/service_account_repository.go` implementing the port with methods that return not found or zero values (DB-001).
- [ ] T025 [P] Create `internal/adapters/storage/postgres/service_account_repository.go` implementing the port with methods that return a `storage.StorageError` of kind unknown (DB-001).
- [ ] T026 Add `ServiceAccounts() ports.ServiceAccountRepository` to the adapter in `internal/adapters/storage/factory.go` and construct the memory and PostgreSQL repositories in `newMemoryAdapter` and `newPostgresAdapter` (ADR 004; Principle IX).
- [ ] T027 Create `internal/domain/thirdparty/service_account_service.go` with `ServiceAccountService`, its constructor taking the repository, `ports.EncryptionPort`, a service-existence lookup, a `ServiceAccountBindingChecker`, `now func() time.Time` and a logger (panic on a nil logger), the interfaces `ServiceAccountCounter` and `ServiceAccountBindingChecker`, `NoServiceAccountBindings`, and the methods `Create`, `List`, `Get`, `Resolve`, `Status`, `Update`, `Delete` returning zero values (plan §Design Notes; research R10; Principles VI, XII).
- [ ] T028 Create `internal/adapters/http/handlers/admin/service_accounts_handler.go` with `ServiceAccountsHandler` (`Create`, `List`, `Get`, `Update`, `Delete`, `Status` answering `501`), add `ServiceAccounts` to `AdminHandlers` in `internal/app/handlers.go`, construct service and handler in `internal/app/builder.go`, and register the six routes under `/{service-id}/service-accounts` in `SetupAdminRoutes` in `internal/adapters/http/routing/admin.go`, including its route comment (API-001; Principle XII).
- [ ] T029 Run `just build` and `just check`; confirm the E2E suites of T013–T017 now fail on `501` instead of `404`.

**Checkpoint**: Scaffold compiles; no business logic.

---

## Phase 2.5: Foundational Infrastructure

**Purpose**: Credential rules, entity rules, schema and both repositories, which every user story
needs. Tests first in each pair.

### Tests

- [ ] T030 [P] Write table tests in `internal/domain/model/service_account_credential_test.go` for every row of `data-model.md` §Validation of a `signed_jwt` credential, in order, with its reason and message: `CredentialFormat` and `CredentialUse` values (`assertion` refused), 16384 and 16385 bytes, segment count and emptiness, padded and non-base64url segments, surrounding whitespace, non-object header or payload, `alg` `none` in three letter cases and missing, `exp` before, at and after the request time, fractional, string, negative and above 253402300799. Assert that no error text contains the input or any segment. Test `data-model.md` §Fingerprint (equal tokens equal, different tokens differ, format `SHA256:` plus 43 characters) and the `CredentialSummary` fields (`CredentialSummary.Iss`, `CredentialSummary.Sub`, `CredentialSummary.Kid` omitted when absent or not a string; `CredentialSummary.Exp` kept as stated) (FR-007–FR-010, FR-012, SR-003, SR-007).
- [ ] T031 [P] Write tests in `internal/domain/model/service_account_test.go` for entity validation (`data-model.md` `ServiceAccount.CanonicalID`, `ServiceAccount.Kind`, `ServiceAccount.Scopes` per §Entity-level validation), `StatusAt` at `exp - 1ns`, at `exp` and after it, and without `exp` (`ServiceAccount.CredentialExpiresAt`), `Narrowing` for `jwt_bearer`, `CredentialSummary.LogValue` capping strings at 256 bytes, and redacted `LogValue`, `String` and `GoString` of `ServiceAccountCredentialInput` (FR-002–FR-004, FR-014, FR-023).
- [ ] T032 [P] Write the shared repository contract in `tests/integration/storage/contract/service_account_repository.go` (every rule of `data-model.md` §Repository port: version 1 on create, unknown service, scoped reads and other-service not found, byte-ordered list, uniqueness within a service and reuse across services, rename and freeing of the old name, version precondition on update and delete, unconditional writes without one, `CountByService`, `StorageError` kinds and sentinels via `errors.Is`); run it on the memory store in `tests/integration/storage/service_account_repository_test.go`; add deep-copy and `-race` concurrency tests to `internal/adapters/storage/memory/service_account_repository_test.go` (DB-001, SC-008; Principle IX).
- [ ] T033 [P] Write `tests/integration/migrations/service_accounts_migration_test.go` (`integration` tag, shared migrated clones): apply, rollback and reapply of `050`; the unique constraint; each `CHECK` of `data-model.md` §PostgreSQL schema (`ServiceAccount.CanonicalID` pattern, non-empty `ServiceAccount.Kind`, `ServiceAccount.Scopes` non-empty array, non-empty `ServiceAccount.Credential` ciphertext, object summary, `ServiceAccount.Version` ≥ 1); deleting a service that holds an account fails with `23503`; no column holds plaintext (DB-002, DB-003; Principle IX).
- [ ] T034 [P] Run the shared contract on PostgreSQL in `tests/integration/storage/infra/service_account_test.go`, and in `internal/adapters/storage/postgres/service_account_repository_test.go` (`integration` tag) assert `23505` → canonical-ID sentinel, version mismatch → version sentinel, and the seven span names of `contracts/observability.md` §5 with kind `Client` and `storage.outcome` for not found and both conflicts (DB-001, DB-002, SC-008; Principle XV).

### Implementation

- [ ] T035 Implement the credential rules, summary and fingerprint in `internal/domain/model/service_account_credential.go` with the standard library only (`encoding/base64` strict raw URL decoding, `encoding/json` with `UseNumber`, `crypto/sha256`), stopping at the first failed rule; populate `CredentialSummary.Format`, `CredentialSummary.Use`, `CredentialSummary.Iss`, `CredentialSummary.Sub`, `CredentialSummary.Kid`, `CredentialSummary.Exp`, `CredentialSummary.Fingerprint` and the parsed expiry for `ServiceAccount.CredentialExpiresAt`; build the credential document of `data-model.md` §Credential document. Make T030 green (FR-008, FR-009, FR-012, SR-003, SR-007; DEC-FINGERPRINT, DEC-EXP-RANGE).
- [ ] T036 Implement entity validation, `StatusAt`, `Narrowing` and the redacting `slog.LogValuer`, `fmt.Stringer`, `fmt.GoStringer` methods in `internal/domain/model/service_account.go` and `service_account_credential.go`, per `data-model.md` `ServiceAccount.CanonicalID` (via `canonical.Validate`), `ServiceAccount.Kind`, `ServiceAccount.Scopes` (ascending byte order on store; only the ordering is added here), `ServiceAccount.CredentialExpiresAt`. Make T031 green (FR-002–FR-004, FR-014; LOGGING.md §6.4).
- [ ] T037 Add `ServiceAccountCredentialInput` to `credential-types` in `.golangci.yml` next to `model.Secret`, so `telemetrylint` rejects it as a log or span attribute (LOGGING.md §§6.4, 11; FR-023).
- [ ] T038 Write `migrations/050_create_service_accounts.up.sql` and `migrations/050_create_service_accounts.down.sql` exactly as `data-model.md` §PostgreSQL schema (`ServiceAccount.ID`, `ServiceAccount.ServiceID`, `ServiceAccount.CanonicalID`, `ServiceAccount.Kind`, `ServiceAccount.Scopes`, `ServiceAccount.Credential` as `credential_encrypted`, `ServiceAccount.Summary` as `credential_summary`, `ServiceAccount.CredentialExpiresAt`, `ServiceAccount.Version`, `ServiceAccount.CreatedAt`, `ServiceAccount.UpdatedAt`), with table and column comments. Make T033 green (DB-002, DB-003; DB-050).
- [ ] T039 Implement `internal/adapters/storage/memory/service_account_repository.go`: one mutex, map by ID, index by `(service_id, canonical_id)`, deep copies, version rules and sentinels, service existence through an injected function wired in `internal/adapters/storage/factory.go`. Make the memory runs of T032 green (DB-001; data-model §In-memory adapter).
- [ ] T040 Implement `internal/adapters/storage/postgres/service_account_repository.go` with sqlx, explicit columns, the configured read and write timeouts, `version = $n` conditions for preconditions, error mapping to `StorageError` and the sentinels, and `startStorageSpan` with the names of `contracts/observability.md` §5. Make T034 green (DB-001, DB-002; ADR 004; OPENTELEMETRY.md §3).
- [ ] T041 Add a description for each of the seven spans to `docs/reference/telemetry-descriptions.yaml`, run `just telemetry-doc` and commit `docs/reference/telemetry.md` (OPENTELEMETRY.md §10).
- [ ] T042 Run `just check`, `just test` and `just test-integration`; record the results in `specs/20261125-152113-service-accounts-stored-jwt/tasks.md`. `just test-integration-infra` runs locally only where a container runtime exists, otherwise in CI (Principles VIII, IX).

**Checkpoint**: Rules, schema and both repositories done; user stories can start.

---

## Phase 3: User Story 1 - Register a ready-signed JWT as a service account (Priority: P1) 🎯 MVP

**Goal**: An administrator attaches a JWT to an existing service and sees it listed and read as
`ready` with a summary, never the JWT.

**Independent Test**: Create an account on an existing service, list and read it; status `ready`,
narrowing `none`, no JWT in any response (spec User Story 1).

### Tests for User Story 1 [MANDATORY - Principle VIII] ⚠️

- [ ] T043 [P] [US1] Write tests in `internal/domain/thirdparty/service_account_service_test.go` for `Create`, `List`, `Get` and `Resolve`: the encryption call receives the credential document and exactly the owning service's `service_id` context; the repository receives ciphertext only; summary, `ServiceAccount.Version` 1, status `ready`; `service.service_account.created` with the keys of `contracts/observability.md` §1; unknown service → not found, nothing stored; name conflict → conflict, no audit; encryption failure → nothing stored, `service.service_account.write_failed` at `Error` without the cause; resolution by UUID and by `canonical_id` within the service; the JWT and its segments absent from a JSON buffer logger (FR-001, FR-002, FR-011, FR-016, FR-022, FR-023, SR-001, SR-002).
- [ ] T044 [P] [US1] Write tests in `internal/adapters/http/handlers/admin/service_accounts_handler_test.go` for `Create`, `List` and `Get`: `401` before any repository call; `404` for an unknown service and for an account of another service; `201` with UUID `Location` and `ETag: "1"`; list in byte order; single read with `ETag`; UUID and canonical addressing; body over 64 KiB and unknown members → `400`; no `credential` member in any response; the JWT and its segments absent from responses, the log buffer and an in-memory span recorder (API-001–API-004, SR-005, SR-006; research R11).
- [ ] T045 [P] [US1] Write `tests/integration/service_account_encryption_test.go` (self-contained, memory encryption backend): a credential stored for one service does not decrypt under another service's context; with a failing encryption port the store stays empty (SR-001, SR-002).

### Implementation for User Story 1

- [ ] T046 [US1] Implement `Create`, `List`, `Get` and `Resolve` in `internal/domain/thirdparty/service_account_service.go`: validate, summarize, encrypt with `encryption.NewServiceBranchKeySubject(serviceID).EncryptionContext()`, store, audit after commit; scoped reads with status from `now()` and no decryption. Make T043 green (FR-001–FR-003, FR-011–FR-016, FR-022, SR-001, SR-002).
- [ ] T047 [US1] Implement `Create`, `List` and `Get` in `internal/adapters/http/handlers/admin/service_accounts_handler.go`: operator principal via `operatorPrincipalFromContext` first, service resolution through `ThirdpartyOAuth2ProviderService.ResolveID`, account resolution, `http.MaxBytesReader` (64 KiB), `DisallowUnknownFields`, the representation of the contract with `failure_reason` only when failed, `Location`, `strongETag`. Make T044 green (API-001–API-004, SR-005).
- [ ] T048 [US1] Wire `ServiceAccountService` in `internal/app/builder.go` with the storage repository, the encryption adapter, the provider repository as existence lookup, `NoServiceAccountBindings`, `time.Now` and the builder logger; pass it to the handler. Run the User Story 1 E2E tests (memory) and confirm Scenarios 1–4, 6 and 7 pass (Principle XII; US1.1–US1.7).

**Checkpoint**: User Story 1 works on its own; Scenario 5 passes on both stores where containers run.

---

## Phase 4: User Story 2 - Refuse credentials that cannot work (Priority: P1)

**Goal**: Every unusable credential and invalid member is refused at write time with the failed
rule, without echoing the JWT, and nothing is stored.

**Independent Test**: Submit each invalid credential; each answers `400`, the list is unchanged,
no response contains the JWT (spec User Story 2).

### Tests for User Story 2 [MANDATORY - Principle VIII] ⚠️

- [ ] T049 [P] [US2] Add tests to `internal/domain/thirdparty/service_account_service_test.go`: each credential rule and each entity rule on create stores nothing and writes no `created` record; a credential refusal writes one `service.service_account.credential_rejected` record at `Warn` with `operation` and the enumerated `reason`; neither record nor error contains the input (FR-009, FR-010, FR-023; observability §2).
- [ ] T050 [P] [US2] Add tests to `internal/adapters/http/handlers/admin/service_accounts_handler_test.go` mapping each refusal to `400` with `error: validation failed` and the message of the contract's `ServiceAccountValidationError` examples, including unknown `kind`, unknown `format`, `use: assertion`, `null` members, empty or missing `scopes`, missing `credential` and invalid `canonical_id` (FR-006, FR-007, FR-010; API-002, API-004).

### Implementation for User Story 2

- [ ] T051 [US2] Add the `credential_rejected` record and the refusal flow to `Create` in `internal/domain/thirdparty/service_account_service.go`, so `CredentialValidationError` and entity errors return before any encryption or repository call. Make T049 green (FR-009, FR-010).
- [ ] T052 [US2] Map domain validation errors to the error envelope in `internal/adapters/http/handlers/admin/service_accounts_handler.go`, using only the constant messages. Make T050 green and run the User Story 2 E2E tests (US2.1–US2.7; SC-003).

**Checkpoint**: User Stories 1 and 2 work; MVP complete.

---

## Phase 5: User Story 3 - Rotate the credential and change the ceiling (Priority: P2)

**Goal**: Replace the credential in place, change the ceiling or rename, keeping the account's
identity, with optional optimistic concurrency.

**Independent Test**: Rotate, then change only the ceiling, then rename; same ID throughout, new
summary and ETag where something changed (spec User Story 3).

### Tests for User Story 3 [MANDATORY - Principle VIII] ⚠️

- [ ] T053 [P] [US3] Add `Update` tests to `internal/domain/thirdparty/service_account_service_test.go` for every row of `data-model.md` §Update outcomes: rotation (new summary, new ciphertext, `failed` → `ready`), ceiling change (credential and summary untouched), rename, combined change, no-op including reordered ceiling and resubmitted JWT (no write, same `ServiceAccount.Version` and `ServiceAccount.UpdatedAt`, no record), `kind` change refused, invalid credential leaves everything unchanged and writes `credential_rejected`, name conflict, stale version, request errors before a stale version, other-service account not found, audit `event` precedence with `changes` and `previous_observed_*` keys (FR-005, FR-017–FR-019, FR-021, FR-022; research R7).
- [ ] T054 [P] [US3] Add `Update` tests to `internal/adapters/http/handlers/admin/service_accounts_handler_test.go`: `If-Match` absent, current, stale → `412`, weak or malformed → `400`; `scopes` required; `null` members refused; `200` with the new or unchanged `ETag`; the JWT absent from responses, logs and spans (API-004, SR-006; research R8).

### Implementation for User Story 3

- [ ] T055 [US3] Implement `Update` in `internal/domain/thirdparty/service_account_service.go`: load, supplied-version check, candidate validation, no-op detection by `canonical_id`, ceiling set and fingerprint/format/use, encryption of a new credential only, `repo.Update` with the expected version, one audit record per committed write. Make T053 green (FR-017–FR-019, FR-022).
- [ ] T056 [US3] Implement `Update` in `internal/adapters/http/handlers/admin/service_accounts_handler.go` with `parseStrongETag` for `If-Match` and the check order of `plan.md` §Design Notes; map the version sentinel to `412` and the name sentinel to `409`. Make T054 green and run the User Story 3 E2E tests (US3.1–US3.9; SC-005, SC-006).

**Checkpoint**: User Stories 1–3 work.

---

## Phase 6: User Story 4 - See whether an account is usable (Priority: P2)

**Goal**: An administrator or monitoring job reads `ready` or `failed` with `credential_expired`,
derived from the stated expiry at the time of the request.

**Independent Test**: Register a token expiring shortly, read status before and after expiry,
rotate, read again (spec User Story 4).

### Tests for User Story 4 [MANDATORY - Principle VIII] ⚠️

- [ ] T057 [P] [US4] Add `Status` and list tests with an injected clock to `internal/domain/thirdparty/service_account_service_test.go`: one nanosecond before `ServiceAccount.CredentialExpiresAt` → `ready`, at it and after → `failed` / `credential_expired`, nil expiry → `ready` at any time, the list reports the same; no write and no record on any read (FR-014, FR-015; SC-004; research R6).
- [ ] T058 [P] [US4] Add `Status` tests to `internal/adapters/http/handlers/admin/service_accounts_handler_test.go`: the `ServiceAccountStatus` body with `failure_reason` and `credential_expires_at` as `null` when not applicable, RFC 3339 UTC expiry, `ETag`, `404` for an unknown account, `401` before any read (FR-015, SR-005).

### Implementation for User Story 4

- [ ] T059 [US4] Implement `Status` in `internal/domain/thirdparty/service_account_service.go` using `StatusAt(now())`. Make T057 green (FR-014, FR-015).
- [ ] T060 [US4] Implement `Status` in `internal/adapters/http/handlers/admin/service_accounts_handler.go`. Make T058 green and run the User Story 4 E2E tests (US4.1–US4.4).

**Checkpoint**: User Stories 1–4 work.

---

## Phase 7: User Story 5 - Remove a service account (Priority: P3)

**Goal**: An administrator removes an account and its credential; a service that still holds
accounts cannot be deleted; deletion of a bound account is refused once agent grants exist.

**Independent Test**: Delete an account, list (absent), delete again (`404`); delete a service
with an account (`409`) (spec User Story 5).

### Tests for User Story 5 [MANDATORY - Principle VIII] ⚠️

- [ ] T061 [P] [US5] Add `Delete` tests to `internal/domain/thirdparty/service_account_service_test.go`: `service.service_account.deleted` after commit with the pre-delete name, ceiling and summary; not found; stale version; a `ServiceAccountBindingChecker` that answers true → conflict `service account is bound by an agent grant`, nothing deleted, no record (US5.3 refusal path); a checker error propagates (FR-020, FR-022; research R10).
- [ ] T062 [P] [US5] Add tests to `internal/domain/thirdparty/service_test.go` for `ThirdpartyOAuth2ProviderService.Delete`: with N accounts → conflict `cannot delete service: N service account(s) are attached to it` and no repository delete; with none → deletion proceeds; a counter error propagates (FR-025; API-SVC-DELETE).
- [ ] T063 [P] [US5] Add a test to `internal/adapters/storage/memory/thirdparty_provider_test.go` that deleting a provider is refused with a conflict while the injected counter reports accounts, and add a case to `internal/adapters/storage/postgres/thirdparty_provider_test.go` (`integration` tag) that the `RESTRICT` foreign key surfaces as a conflict (FR-025, DB-002).
- [ ] T064 [P] [US5] Add `Delete` tests to `internal/adapters/http/handlers/admin/service_accounts_handler_test.go` (`204`, `404`, `409`, `412`, `400` for a malformed `If-Match`, `401` before any read) and a case to `internal/adapters/http/handlers/admin/services_handler_test.go` that `DELETE /api/services/{service-id}` answers `409` with the service-account message (FR-020, FR-025; API-004).

### Implementation for User Story 5

- [ ] T065 [US5] Implement `Delete` in `internal/domain/thirdparty/service_account_service.go`: load, supplied-version check, binding check, `repo.Delete` with the expected version, audit after commit. Make T061 green (FR-020, FR-022).
- [ ] T066 [US5] Add `WithServiceAccounts(ServiceAccountCounter)` and the guard to `Delete` in `internal/domain/thirdparty/service.go`, next to the permission-set guard, and inject the counter in `internal/app/builder.go`. Make T062 green (FR-025).
- [ ] T067 [US5] Add `WithServiceAccountRepository` to `internal/adapters/storage/memory/thirdparty_provider.go`, check the count under the write lock in `Delete`, and wire it in `newMemoryAdapter` in `internal/adapters/storage/factory.go`. Make T063 green (FR-025, DB-002).
- [ ] T068 [US5] Implement `Delete` in `internal/adapters/http/handlers/admin/service_accounts_handler.go` and confirm `DeleteService` in `internal/adapters/http/handlers/admin/services_handler.go` returns the guard's message through its existing conflict path. Make T064 green and run the User Story 5 E2E tests (US5.1, US5.2, US5.4, US5.5).

**Checkpoint**: All user stories work.

---

## Phase 8: Documentation and Validation

- [ ] T069 [P] Add an "Add a service account" section to `docs/guides/manage-agents-and-services.md`: what a service account is, create, list, read, status, rotate, change the ceiling, rename, remove, the 16 KiB and form rules, that the broker never shows or verifies the JWT, that an expired token marks the account failed until replaced, and that a service with accounts cannot be deleted. End-user content only (AGENTS.md §Documentation Scope; SC-001).
- [ ] T070 [P] Add the six service-account operations to the Admin API map in `docs/reference/api.md` under Services (API-001).
- [ ] T071 [P] Update `internal/AGENTS.md` (AdminHandlers list, Key Storage Types row for `ServiceAccount`) and the admin API, data store §4.1 and directory text of `ARCHITECTURE.md` for the new handler, table and domain service (Principles II, XII).
- [ ] T072 Run the validation steps of `specs/20261125-152113-service-accounts-stored-jwt/quickstart.md` §§1–4 and 6 (`just check`, `just test`, `just test-integration`, the memory E2E focus run, `just telemetry-doc`, then `just verify` where a container runtime exists) and record the results, naming any suite left to CI, in `specs/20261125-152113-service-accounts-stored-jwt/tasks.md` (Principles VIII, XIII).

---

## 🔒 Phase N: Constitution Compliance Verification [MANDATORY COMPLIANCE SECTION]

**Purpose**: Verify constitution requirements and feature-scope compliance before the feature is
done.

### 🔒 Constitution Compliance Verification [MANDATORY]

#### Design Phase Verification [MANDATORY]

- [ ] T073 Verify every task above traces to a requirement, scenario, success criterion or binding constraint, and that no task hands out a credential, changes token exchange, binds an agent or changes the UI (Feature Scope; FR-024).
- [ ] T074 Verify the six new terms are in the `ARCHITECTURE.md` glossary and the `AGENTS.md` glossary table (Principle V).
- [ ] T075 Verify no file in `examples/config/` or `charts/agentic-identity-broker/` changed and none was needed (Principle VII).
- [ ] T076 Verify `api/admin/openapi.yaml` holds the service-account operations at version `3.1.0` and the recorded approval covers each API item (Principles IV, X).
- [ ] T077 Verify the schema design and migration `050` match `data-model.md` (Principle IX).
- [ ] T078 Verify the three spec journeys link to their scenarios and agree with the merged contract, with decoded claims for T1, T2 and U and only synthetic tokens; no configuration, Wireloom or frontend journey applies (Principle XIV).
- [ ] T079 Verify `tests/e2e/service_accounts_register_test.go` and `tests/e2e/service_accounts_lifecycle_test.go` hold one `It()` for each of the 31 scenarios mapped in `plan.md`, written before implementation with concrete expectations, without `XIt`, `PIt`, `Skip()` or red-phase comments; red-run evidence is not required (Principle XIII).
- [ ] T080 Verify the log events and storage spans match `contracts/observability.md`, no metric was added, and no new span attribute namespace was needed (Principle XV).

#### Implementation Phase Verification [MANDATORY]

**Feature Scope**:
- [ ] T081 Verify unrelated findings were reported to the user, not fixed, and that only user-approved issue URLs appear under `Future Work` in this file.

**API & Documentation** (Principles IV, X):
- [ ] T082 [P] Verify the handler behaviour matches `api/admin/openapi.yaml` exactly: status codes, headers, members, messages, `If-Match` semantics.
- [ ] T083 [P] Verify no `docs/api/` page is needed (admin-only change) and that `docs/guides/manage-agents-and-services.md` and `docs/reference/api.md` describe the operations.

**Architecture & Documentation** (Principle II):
- [ ] T084 Verify `ARCHITECTURE.md` describes the entity, table, handler, domain service and Admin API `3.1.0`.
- [ ] T085 [P] Verify `adrs/20261125-154231-service-accounts-stored-jwt.md` is Accepted, agrees with the code, and is listed in `ARCHITECTURE.md` §10 and `AGENTS.md`; no accepted ADR merged into main was deviated from.

**Configuration** (Principle VII):
- [ ] T086 [P] Verify no configuration parameter was added and nothing reads configuration outside `internal/ports/config.go`.

**Database & Persistence** (Principle IX):
- [ ] T087 [P] Verify migration `050` is atomic, sequentially numbered, uses explicit columns, and has apply, rollback and reapply tests.
- [ ] T088 [P] Verify the PostgreSQL repository is covered by the shared contract and its own integration tests, and both stores pass the same contract (SC-008).
- [ ] T089 Verify the port has seven methods in `internal/ports/storage.go`, errors are `StorageError`, and the adapter exposes `ServiceAccounts()` (ADR 004).

**Security** (Principles I, III):
- [ ] T090 Verify encryption is mandatory with no plaintext path, `alg: none` is refused, the operator principal is required before any read, and the stored JWT drives no decision (SR-001, SR-002, SR-004, SR-005, FR-013).
- [ ] T091 [P] Verify only `crypto/sha256`, the standard library decoders and the existing encryption adapters are used; no signature operation and no custom cryptography (SR-003).
- [ ] T092 [P] Verify audit records exist for create, rotation, ceiling change, rename and delete, after commit only, following `coding-standards/LOGGING.md` §8 (FR-022, SC-006).

**Coding Standards** (Principle XV):
- [ ] T093 Verify every added log call and span satisfies the review checklists of `coding-standards/LOGGING.md` §10 and `coding-standards/OPENTELEMETRY.md` §8, and `docs/reference/telemetry.md` is current.
- [ ] T094 Verify the credential-absence tests of `contracts/observability.md` §8 exist for every path (responses, logs, spans) (SR-006, SC-002).
- [ ] T095 Verify no lint rule is disabled or suppressed; the `LOGGING.md` §6.2 amendment and the `credential-types` addition are the only standard changes, and the PR description states the amendment and its rationale.

**Architecture Patterns** (Principles VI, XII):
- [ ] T096 Verify rules, no-op detection, status and audit live in `internal/domain`; the handler parses and maps only; no handler calls a port directly; all wiring is in `internal/app/builder.go`; routes are registered only in `SetupAdminRoutes`.

**Testing** (Principle VIII):
- [ ] T097 Verify unit tests were written first and drove the design; red-run evidence is not required.
- [ ] T098 Verify tests changed minimally during implementation.
- [ ] T099 Verify unit and integration tests cover every rule and repository method, and no Bash script validates code correctness.

**E2E Acceptance Testing** (Principle XIII):
- [ ] T100 Verify each `It()` maps to exactly one scenario and carries its scenario comment; `Describe` → `Context` → `It` structure; fixtures from `tests/e2e/fixtures/service_account.go`.
- [ ] T101 Verify the E2E tests changed minimally during implementation (fixture adjustments only) and turned green.
- [ ] T102 Run `just test-e2e-backend`; run `just test-e2e-docker` where a container runtime exists, otherwise confirm the `docker` scenarios pass in CI on the pushed commit. All must pass.

---

## Dependencies & Execution Order

### Phase Dependencies

- **Setup (Phase 1)**: none. T001 gates T005 (Accepted status), T007 and T020.
- **Design Preconditions (Phase 2)**: after Setup; 2a, 2b, 2c, 2d, 2f and 2g can proceed in parallel. The E2E tests (T013–T017) need only T011 and T012.
- **Entity Boilerplate (Phase 2.7)**: after all of Phase 2. Commit separately from business logic.
- **Foundational (Phase 2.5)**: after Phase 2.7. Blocks every user story.
- **User Stories (Phases 3–7)**: after Phase 2.5.
- **Documentation and Validation (Phase 8)**: after the user stories it describes.
- **Compliance (Phase N)**: last.

### User Story Dependencies

- **US1 (P1)**: after Phase 2.5. Creates the data every other story reads.
- **US2 (P1)**: after US1 (refusals are part of `Create`).
- **US3 (P2)**: after US1. Its invalid-credential scenario (US3.4) reuses the rules of Phase 2.5, not US2 code.
- **US4 (P2)**: after US1. US3.3 also needs US4's expiry behaviour, which `StatusAt` (T036) already provides.
- **US5 (P3)**: after US1. The service-delete guard (T066, T067) is independent of US2–US4.

### Within Each User Story

Tests before implementation; domain service before handler; handler before the story's E2E run.

### Parallel Opportunities

- Phase 2: T004 and T005; T011 and T012; then T013–T017 by different authors (two files: register and lifecycle).
- Phase 2.7: T024 and T025.
- Phase 2.5: T030–T034 together; then T035 with T038, T036 after T035.
- Each story: its `[P]` test tasks together (domain test file and handler test file).
- After US1: US3, US4 and US5 in parallel, as each touches different methods of the same two files; merge carefully.
- Phase 8: T069–T071.

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

1. Phase 1: record approvals (T001).
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
and the E2E test for User Story 5 Scenario 3.

---

## Notes

- `[P]` tasks touch different files and depend on no unfinished task in their batch.
- `[USn]` maps a task to spec User Story n.
- Commit after each task or logical group; the Phase 2.7 scaffold is its own commit.
- The DCO rule applies to every commit on this branch.
