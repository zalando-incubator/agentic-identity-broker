# Implementation Plan: Service Accounts on Third-Party Services (First Kind: `jwt_bearer` with a Stored JWT)

**Branch**: `speckit/issue-471` | **Date**: 2026-11-25 | **Spec**: [spec.md](./spec.md)

**Input**: Feature specification from `specs/20261125-152113-service-accounts-stored-jwt/spec.md`

**Status**: Planned. The maintainer approves the API contract (Constitution Principle X) and the
decisions in [Review Items](#review-items) by reviewing the PR.

## Summary

Add the `ServiceAccount` entity: a machine credential attached to one third-party service, with a
scope ceiling, managed through the admin API. The first kind is `jwt_bearer` with the
`signed_jwt` format: a JWT signed outside the broker, stored encrypted and never returned. No
agent can use an account yet (#472, #473).

- **Entity in the existing bounded context.** `model.ServiceAccount` and its value objects join
  `internal/domain/model`; a new `thirdparty.ServiceAccountService` holds the workflow
  ([research R1](./research.md#r1--where-the-domain-code-lives)).
- **Opaque credential, checked for form only.** `jwx` parses the token without verification; the
  broker adds only the size bound, the `alg: none` refusal and the expiry check, then derives the
  summary and a `SHA256:<base64>` fingerprint. No signature is verified and nothing is fetched ([R4](./research.md#r4--form-validation-of-the-jwt),
  [R5](./research.md#r5--credential-summary-and-fingerprint)).
- **Encrypted under the service.** A format-discriminated credential document is encrypted with
  the existing `EncryptionPort` under the owning service's `service_id` context; there is no
  plaintext path ([R3](./research.md#r3--what-is-encrypted-and-under-which-context)).
- **Status derived on read.** `ready` or `failed` comes from the stored expiry and the request
  time; nothing is swept or stored ([R6](./research.md#r6--status-is-derived-on-read)).
- **One focused repository port, two adapters.** `ports.ServiceAccountRepository` (seven
  service-scoped methods) with memory and PostgreSQL implementations, version preconditions and
  conflict sentinels; atomic migration `050` ([R2](./research.md#r2--repository-port),
  [data model](./data-model.md)).
- **Thin HTTP adapter.** `admin.ServiceAccountsHandler` under
  `/api/services/{service-id}/service-accounts`; optional `If-Match`; Admin API `3.1.0`
  ([contract delta](./contracts/admin-service-accounts-openapi.yaml),
  [R7](./research.md#r7--optimistic-concurrency-and-no-op-updates),
  [R11](./research.md#r11--http-adapter)).
- **Service deletion guarded.** A service that holds accounts answers `409`, enforced in the
  domain and by `ON DELETE RESTRICT`
  ([R9](./research.md#r9--a-service-that-holds-accounts-cannot-be-deleted)).
- **Audit without the credential.** One audit record per committed write, carrying the summary;
  seven storage spans; no metric ([observability contract](./contracts/observability.md)).

## Technical Context

**Language/Version**: Go 1.27.4

**Primary Dependencies**: chi v5 (routing), sqlx and pgx v5 (PostgreSQL adapter), golang-migrate
(migrations), `log/slog`, OpenTelemetry API, the existing `ports.EncryptionPort` adapters;
`lestrrat-go/jwx/v4` (`jws`, `jwt`, already a dependency) for parsing; standard library
`crypto/sha256` and `encoding/base64` for the fingerprint. No new dependency.

**Storage**: PostgreSQL (production) and in-memory (development, tests). One new table,
`service_accounts`, in migration `050_create_service_accounts.{up,down}.sql`.

**Testing**: Go unit tests with testify (domain model, domain service, memory adapter, handler);
PostgreSQL integration tests under the `integration` build tag (migration, repository contract);
Ginkgo/Gomega E2E through the production bootstrap, with `Label("docker")` for the scenarios that
depend on store semantics.

**Target Platform**: Linux broker container; Darwin for development.

**Project Type**: Backend Admin API feature in the Go monorepo. No React UI, ExtProc, end-user
API, configuration, Helm or CDK change.

**Performance Goals**: A list or status read is one indexed query and no decryption. A create or
rotation adds one `Encrypt` call. An unchanged update performs reads only.

**Constraints**: The JWT must not appear in any response, log record, span, metric or error
(FR-011, FR-023, SR-006). Fail closed with no plaintext fallback (SR-002). Memory and PostgreSQL
behave identically (DB-001, SC-008). The migration is atomic (DB-002). Existing behaviour is
unchanged (FR-024, SC-007).

**Scale/Scope**: An administrative registry: tens of accounts per service, hundreds in total. The
unpaginated list and a transactional table creation are adequate. All 31 acceptance
scenarios are tested in this feature.

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

Verified against [.specify/memory/constitution.md](../../.specify/memory/constitution.md):

**Design Preconditions (BLOCKING)**:

- [x] **Domain Model**: [data-model.md](./data-model.md) documents the `ServiceAccount` aggregate,
  its value objects, validation rules, derived values, state and update outcomes.
- [x] **Domain Concepts**: Service Account, Kind, Credential Format, Ceiling, Narrowing and
  Credential Summary are added to the `ARCHITECTURE.md` glossary and the root `AGENTS.md` glossary
  table with the definitions of #478.
- [x] **Entity IDs**: `ServiceAccountID` is added to `uuidTypes` in
  `internal/domain/id/gen_ids.go`, generated, tested in `id_test.go` and documented in
  `internal/domain/id/AGENTS.md` (`adrs/013-strongly-typed-entity-ids.md`).
- [x] **Configuration Design**: No configuration parameter. The 16 KiB limit is a fixed value in
  the API contract (spec Assumptions).
- [x] **Config Examples**: Not applicable; no new keys.
- [x] **Helm Deployment Contract**: No parameter change. The existing migration Job applies `050`.
- [x] **API Design First**:
  [contracts/admin-service-accounts-openapi.yaml](./contracts/admin-service-accounts-openapi.yaml)
  defines the change; it is merged into `api/admin/openapi.yaml` before any handler is implemented.
- [x] **API Documentation**: Admin API only (`api/admin/openapi.yaml`).
- [ ] **API Changes**: The maintainer confirms API-SA, API-SHAPE, API-UPDATE, API-SVC-DELETE,
  API-VERSION and DEC-FINGERPRINT by reviewing the PR (maintainer decision,
  2026-11-26). No separate approval record is kept.
- [x] **Database Design**: `050_create_service_accounts.{up,down}.sql`, go-migrate naming, atomic;
  schema in [data-model.md](./data-model.md#postgresql-schema--migration-050_create_service_accounts).
- [x] **E2E Acceptance Tests**: Planned for all scenarios before implementation
  ([Testing Strategy](#testing-strategy)).
- [x] **E2E Test Mapping**: One `It()` per scenario ([Scenario Mapping](#scenario-mapping)).
- [x] **E2E Red Phase**: Tests assert concrete status codes, headers, bodies and list contents
  against handlers that first answer `501`.
- [x] **Frontend Playwright E2E**: Not applicable; no UI change.
- [x] **Frontend Screenshots**: Not applicable.

**Implementation Considerations**:

- [x] **Security-First**: Encryption is mandatory with no plaintext fallback (SR-002). `alg: none`
  is refused. The stored JWT is never accepted as proof of anything, so storing it unverified is
  not an exception to the signature rule (SR-004); the ADR records this. Operator principal is
  required before any read (SR-005). Audit records follow LOGGING.md section 8.
- [x] **Architecture Docs**: `ARCHITECTURE.md` gets the glossary section, the data-store entry,
  the admin API summary, the Admin API `3.1.0` note and the ADR index entry.
- [x] **ADRs**: New `adrs/20261125-154231-service-accounts-stored-jwt.md`, introduced and accepted
  in this PR ([R15](./research.md#r15--adr)). No accepted ADR is superseded.
- [x] **Library-First Security**: `crypto/sha256` for the fingerprint, the existing encryption
  adapters for storage, `lestrrat-go/jwx/v4` for parsing the JWT without verification; no
  hand-written parser, and the broker performs no signature operation (SR-003).
- [x] **Zalando Guidelines**: Kebab-case path segments, snake_case members, sub-resource under its
  parent, standard status codes, optional `If-Match` with strong ETags. The unpaginated direct
  array follows the existing protected-resources list (spec Assumptions).
- [x] **End-User Docs**: Admin-only change; no `docs/api/` page. `docs/guides/manage-agents-and-services.md`
  gains a "Add a service account" section and `docs/reference/api.md` the admin API rows.
- [x] **Migration Testing**: Integration tests cover apply, rollback, reapply, and each constraint.
- [x] **Hexagonal Architecture**: Rules, no-op detection, status and audit live in
  `internal/domain`. The handler parses input and maps errors. Adapters implement the port. No
  cross-adapter import; no handler-to-port shortcut.
- [x] **Persistence Patterns**: Port in `internal/ports/storage.go` with seven methods; memory and
  PostgreSQL adapters; sqlx; `StorageError` wrapping; accessor `ServiceAccounts()` on the storage
  adapter (`adrs/004-storage-layer-architecture.md`).
- [x] **Coding Standards (Principle XV)**: New log events and spans are listed in
  [contracts/observability.md](./contracts/observability.md). One scoped amendment to
  `coding-standards/LOGGING.md` section 6.2 is proposed in the same PR (STD-LOGGING). No lint rule
  is disabled or suppressed.
- [x] **Feature Scope**: No hand-out of the credential, no token-exchange change, no agent
  binding, no UI (FR-024). No metric is added because no requirement asks for one.

**Post-design re-check (after Phase 1)**: All blocking preconditions pass except the API
confirmation, which the maintainer gives in PR review. No violation needs Complexity Tracking.

## Review Items

The maintainer reviews these decisions in the PR. Implementation does not wait for a separate
approval.

| ID | Type | Decision | Details |
|---|---|---|---|
| API-SA | API | Add create, list, read, update, delete and status for service accounts under `/api/services/{service-id}/service-accounts`, with the representation, `credential_summary`, error messages, `Location`, strong ETag and optional `If-Match` as in the contract. `status` and `failure_reason` are not part of the representation; only the status resource carries them. List ordered by `canonical_id`. `service_id` is always the UUID; `Prefer: reference-id=canonical` is not offered. Unknown request members are refused. | [contracts/admin-service-accounts-openapi.yaml](./contracts/admin-service-accounts-openapi.yaml) |
| API-SHAPE | API | One `ServiceAccountRequest` schema with `kind` as an enum member and `credential` as a `oneOf` discriminated by `format`, instead of a kind-level discriminator, because the spec's update journeys omit `kind` | [research R12](./research.md#r12--request-schema-shape) |
| API-UPDATE | API | On `PUT`, `scopes` is required and replaces the ceiling; `canonical_id`, `kind` and `credential` are optional; `null` is refused | [research R8](./research.md#r8--update-request-semantics) |
| API-SVC-DELETE | API | `DELETE /api/services/{service-id}` answers `409` while the service holds a service account | [research R9](./research.md#r9--a-service-that-holds-accounts-cannot-be-deleted) |
| API-VERSION | API | Raise the Admin API from `3.0.0` to `3.1.0` (additive) | spec API-006 |
| DEC-FINGERPRINT | Decision | Fingerprint text: `SHA256:` plus the unpadded standard base64 SHA-256 digest of the submitted JWT | [research R5](./research.md#r5--credential-summary-and-fingerprint) |
| DEC-JWX | Decision | Parse the JWT with `jwx` v4 and relax the spec's form rules to the library's behaviour (maintainer direction, 2026-11-26) | [research R4](./research.md#r4--form-validation-of-the-jwt) |
| STD-LOGGING | Standard | Amend `coding-standards/LOGGING.md` section 6.2: the SHA-256 fingerprint of a stored credential is permitted in `service.service_account.*` audit records. `sub` needs no amendment: it is logged as an `id.Principal` | [contracts/observability.md](./contracts/observability.md#4-amendment-to-coding-standardsloggingmd-same-pr) |
| ADR-SA | ADR | New ADR `20261125-154231-service-accounts-stored-jwt`: entity and port, stored-JWT posture, `service_id` encryption of a format-discriminated credential document, derived status, optional `If-Match`, `RESTRICT` on service deletion | [research R15](./research.md#r15--adr) |
| DB-050 | Decision | Migration `050`: table `service_accounts` with the columns, checks, unique constraint and `ON DELETE RESTRICT` foreign key of the data model; DOWN drops the table and with it the stored credentials | [data-model.md](./data-model.md#postgresql-schema--migration-050_create_service_accounts) |

## Project Structure

### Documentation (this feature)

```text
specs/20261125-152113-service-accounts-stored-jwt/
├── spec.md
├── plan.md                                   # This file
├── research.md                               # Phase 0
├── data-model.md                             # Phase 1
├── quickstart.md                             # Phase 1
├── contracts/
│   ├── admin-service-accounts-openapi.yaml   # Phase 1 — Admin API 3.1.0 delta
│   └── observability.md                      # Phase 1 — log events, spans, required tests
├── checklists/requirements.md
└── tasks.md                                  # Phase 2 (/speckit-tasks)
```

### Source Code (repository root)

```text
api/admin/openapi.yaml                                  # merge contract delta; version 3.1.0
adrs/20261125-154231-service-accounts-stored-jwt.md     # new ADR
ARCHITECTURE.md                                         # glossary, data store, admin API, ADR index
AGENTS.md                                               # glossary rows, ADR index row
coding-standards/LOGGING.md                             # section 6.2 row (STD-LOGGING)
docs/guides/manage-agents-and-services.md               # "Add a service account" section
docs/reference/api.md                                   # admin API map rows
docs/reference/telemetry-descriptions.yaml              # seven span descriptions
docs/reference/telemetry.md                             # regenerated (just telemetry-doc)

migrations/
├── 050_create_service_accounts.up.sql
└── 050_create_service_accounts.down.sql

internal/
├── domain/id/gen_ids.go, uuid_ids_gen.go, id_test.go, AGENTS.md   # ServiceAccountID
├── domain/model/service_account.go                     # entity, kind, status, narrowing
├── domain/model/service_account_credential.go          # form rules, summary, fingerprint
├── domain/thirdparty/service_account_service.go        # workflow, no-op detection, audit
├── domain/thirdparty/service.go                        # Delete guard; WithServiceAccounts
├── ports/storage.go                                    # ServiceAccountRepository, sentinels
├── ports/AGENTS.md                                     # catalogue entry
├── adapters/storage/memory/service_account_repository.go
├── adapters/storage/memory/thirdparty_provider.go      # delete guard via counter
├── adapters/storage/postgres/service_account_repository.go
├── adapters/storage/factory.go                         # ServiceAccounts() accessor, wiring
├── adapters/http/handlers/admin/service_accounts_handler.go
├── adapters/http/routing/admin.go                      # six routes
├── app/handlers.go                                     # AdminHandlers.ServiceAccounts
├── app/builder.go                                      # service and handler wiring
└── AGENTS.md                                           # handler list, key storage types

tests/
├── e2e/service_accounts_register_test.go               # User Stories 1 and 2
├── e2e/service_accounts_lifecycle_test.go              # User Stories 3, 4 and 5
├── e2e/fixtures/service_account.go                     # signed, unsigned, expiring tokens; requests
├── e2e/service_accounts_backend_test.go               # memory and PostgreSQL store cases
├── e2e/helpers/service_account.go                      # PUT/DELETE with If-Match, credential-absence matcher
├── integration/migrations/service_accounts_migration_test.go
├── integration/storage/contract/service_account_repository.go # shared repository contract
├── integration/storage/service_account_repository_test.go    # contract on the memory store
├── integration/storage/infra/service_account_test.go         # contract on PostgreSQL
└── integration/service_account_encryption_test.go      # service-context isolation (SR-001)
```

**Structure Decision**: Existing Go monorepo layout. The domain change stays in the third-party
service bounded context (`internal/domain/model`, `internal/domain/thirdparty`); no new domain
package. One new port interface, one new handler, one new table.

## Design Notes for Task Generation

**Domain service** (`thirdparty.ServiceAccountService`), constructed in `builder.go` from
`ports.ServiceAccountRepository`, `ports.EncryptionPort`, a service-existence lookup (the provider
repository), a `ServiceAccountBindingChecker`, `now func() time.Time` and a logger:

| Method | Behaviour |
|---|---|
| `Create` | Validate entity and credential; compute summary; encrypt; `repo.Create`; audit `created` |
| `List`, `Get`, `Status` | Scoped reads; status and narrowing derived with `now()`; no decryption |
| `Update` | Load; check a supplied version; validate; detect no-op; encrypt a new credential; `repo.Update`; audit by precedence with `changes` |
| `Delete` | Load; check a supplied version; refuse when bound; `repo.Delete`; audit `deleted` |
| `Resolve` | UUID or `canonical_id` within the service |

No method returns the plaintext credential (FR-024). Decryption for hand-out is added by #473.

**Handler order of checks**: operator principal (`401`) → service (`404`) → account (`404`) →
`If-Match` syntax (`400`) → body and rules (`400`) → precondition (`412`) → name conflict or
binding (`409`) → write.

**Wiring**: `storage.Adapter.ServiceAccounts()`; memory repositories cross-wired for the two
existence checks; `ProviderService.WithServiceAccounts(counter)`;
`AdminHandlers.ServiceAccounts`; routes registered only in `SetupAdminRoutes`.

## Implementation Phase Overview

*Detailed task breakdown is in `tasks.md` (generated by `/speckit-tasks`).*

| Phase | Purpose | Required? |
|-------|---------|-----------|
| **Phase 0** | Pre-implementation refactoring | Skipped |
| **Phase 1** | Setup — read area instructions, reserve the migration number | Required |
| **Phase 2** | Design Preconditions (domain model and glossary, API, DB, E2E tests, observability) | **MANDATORY** |
| **Phase 2.7** | Entity Boilerplate — typed ID, entity types, port, empty adapters, `501` handlers, routes | Included |
| **Phase 2.5** | Foundational Infrastructure — migration, both repositories, credential rules | Required |
| **Phase 3+** | User Stories — US1 and US2 (P1), US3 and US4 (P2), US5 (P3) | Required |
| **Phase N** | Constitution Compliance verification | **MANDATORY** |

- [x] Phase 0 (refactoring): skip — no existing code is restructured. The two small additions to
  existing types (the provider service's delete guard and the memory provider repository's
  counter) are feature behaviour and belong to User Story 5.
- [x] Phase 2.7 (entity boilerplate): include — the feature introduces a new entity, port, two
  repository adapters and a handler. The scaffold (types, interface, empty adapters, handlers
  answering `501`, routes, builder wiring) lands as its own commit ahead of business logic so
  the E2E suite compiles and fails semantically. Whether it also becomes its own PR is the
  maintainer's call; the branch for this issue is `speckit/issue-471`.

Ordering constraints for tasks:

1. ADR added; OpenAPI delta merged with version `3.1.0`; LOGGING.md amended.
2. Glossary and `ARCHITECTURE.md`; typed ID generated.
3. Scaffold (Phase 2.7), then E2E tests for all scenarios and the migration and repository
   contract tests, failing on concrete assertions.
4. Credential rules and summary (pure domain), then migration and both repositories.
5. Domain service and handler per user story: US1 → US2 → US3 → US4 → US5 (including the
   service-delete guard).
6. Guide, API reference, telemetry descriptions and `just telemetry-doc`; compliance gates.

## Testing Strategy

### End-to-End (E2E) Acceptance Tests

**Test Location**: `tests/e2e/service_accounts_register_test.go` (User Stories 1–2) and
`tests/e2e/service_accounts_lifecycle_test.go` (User Stories 3–5)

**Framework**: Ginkgo/Gomega following [tests/e2e/README.md](../../tests/e2e/README.md)

**Test Organization**:

- **Top-level Describe**: `"Service accounts"` in both files.
- **Contexts**: one per user story, with nested contexts for shared preconditions (an existing
  service; a service with one account; a service with two accounts).
- **It blocks**: one per acceptance scenario, each preceded by
  `// Scenario N from specs/20261125-152113-service-accounts-stored-jwt/spec.md (User Story M)`.

#### Scenario Mapping

Each row links the `It()` in `tests/e2e/` by `file:line`.

| Spec Scenario | File | Test Description |
|---|---|---|
| US1.1 | `service_accounts_register_test.go:53` | `It("should create a ready account with a summary and an ETag and without the JWT")` |
| US1.2 | `service_accounts_register_test.go:122` | `It("should list both accounts with summaries and no credential material")` |
| US1.3 | `service_accounts_register_test.go:80` | `It("should default use to access_token when the credential omits it")` |
| US1.4 | `service_accounts_register_test.go:95` | `It("should accept a JWT without exp and report no expiry")` |
| US1.5 | `service_accounts_register_test.go:144`, `Label("docker")`, both stores | `It("should refuse a taken canonical_id on the same service and accept it on another")` |
| US1.6 | `service_accounts_register_test.go:168` | `It("should answer 404 for create and list under an unknown service")` |
| US1.7 | `service_accounts_register_test.go:193` | `It("should read one account by ID and by canonical_id with its ETag")` |
| US2.1 | `service_accounts_register_test.go:220` | `It("should refuse a JWT that is not a compact JWS")` |
| US2.2 | `service_accounts_register_test.go:227` | `It("should refuse a JWT with alg none in any letter case or without alg")` |
| US2.3 | `service_accounts_register_test.go:240` | `It("should refuse a JWT whose exp is not in the future")` |
| US2.4 | `service_accounts_register_test.go:248` | `It("should refuse a JWT longer than 16384 bytes")` |
| US2.5 | `service_accounts_register_test.go:255` | `It("should refuse use assertion and unknown use, format and kind")` |
| US2.6 | `service_accounts_register_test.go:277` | `It("should refuse empty or missing scopes, a missing credential and an invalid canonical_id")` |
| US2.7 | `service_accounts_register_test.go:312` | `It("should name the failed rule without echoing any part of the JWT")` |
| US3.1 | `service_accounts_lifecycle_test.go:97` | `It("should rotate the credential in place and change summary and ETag")` |
| US3.2 | `service_accounts_lifecycle_test.go:120` | `It("should change the ceiling and keep credential and summary")` |
| US3.3 | `service_accounts_lifecycle_test.go:200` | `It("should return a failed account to ready when a valid JWT replaces the expired one")` |
| US3.4 | `service_accounts_lifecycle_test.go:134` | `It("should leave the account unchanged when an update carries an invalid credential")` |
| US3.5 | `service_accounts_lifecycle_test.go:153` | `It("should answer 200 with the same ETag when nothing changes")` |
| US3.6 | `service_accounts_lifecycle_test.go:164` | `It("should answer 404 for an unknown account and for an account of another service")` |
| US3.7 | `service_accounts_lifecycle_test.go:218`, `Label("docker")`, both stores | `It("should answer 412 for a stale If-Match and carry out a current or absent one")` |
| US3.8 | `service_accounts_lifecycle_test.go:176` | `It("should rename the account and stop addressing it by the old name")` |
| US3.9 | `service_accounts_lifecycle_test.go:248`, `Label("docker")`, both stores | `It("should refuse a rename to a name taken on the same service")` |
| US4.1 | `service_accounts_lifecycle_test.go:275` | `It("should report ready with the stated expiry and no failure reason")` |
| US4.2 | `service_accounts_lifecycle_test.go:295` | `It("should report failed with credential_expired in status and list once exp has passed")` |
| US4.3 | `service_accounts_lifecycle_test.go:320` | `It("should keep an account without exp ready")` |
| US4.4 | `service_accounts_lifecycle_test.go:336` | `It("should answer 404 for the status of an unknown account")` |
| US5.1 | `service_accounts_lifecycle_test.go:354` | `It("should delete the account and remove it from the list")` |
| US5.2 | `service_accounts_lifecycle_test.go:365` | `It("should answer 404 when deleting an unknown account")` |
| US5.3 | `service_accounts_lifecycle_test.go:381` | `It("should answer 412 and keep the account when a delete carries a stale If-Match")` |
| US5.4 | `service_accounts_lifecycle_test.go:393`, `Label("docker")`, both stores | `It("should refuse deleting a service that holds an account and allow it after the last one is removed")` |

US2.1, US2.2, US2.5 and US2.6 each state several variants of one rule. Each keeps one `It()` and
iterates over a table of inputs inside it, asserting status, message and an unchanged list for
every variant.

**Red Phase Requirements**: Tests compile against the Phase 2.7 scaffold (handlers answering
`501`) and fail on concrete assertions: status codes, `Location`, `ETag`, representation members,
error messages, list contents. No `XIt`, `PIt`, `Skip()` or red-phase comments.

**Test Data Strategy**:

- Existing fixtures: `fixtures.AdminPrincipal()`, `fixtures.ServiceWithID(...)` with a canonical
  ID for the *Company Platform* service of the spec journeys.
- New `tests/e2e/fixtures/service_account.go`: builders for token T1 and T2 of the spec (ES256,
  `kid`, `iss`, `sub`, `exp`), a token without `exp`, a token expiring in a few seconds, an
  `alg: none` token (letter-case variants), an oversized token, a five-segment token, and the
  create and update request bodies. Tokens are signed with a key generated per test; none is real.

**Test Execution Flow**:

1. **Phase 2f**: write the E2E tests for all mapped scenarios.
2. Run `ginkgo -v --label-filter="!performance && !docker" --focus "Service accounts" ./tests/e2e/`;
   all fail semantically. The `docker` scenarios run with `just test-e2e-docker`, or first in CI
   where no container runtime is available (Principle VIII).
3. Implement incrementally; tests turn green per user story with fixture adjustments only.

**Bootstrap Strategy**:

- Fresh admin `bootstrap.TestServer` and storage per test (`BeforeEach` / `AfterEach`).
- Store-dependent scenarios: one `It()` with a memory case and a `bootstrap.NewPostgresFixture`
  case inside ([research R16](./research.md#r16--storage-parity-sc-008)).
- Expiry scenarios (US3.3, US4.2): a token expiring a few seconds ahead and `Eventually` on the
  status; no sleep and no injected clock
  ([research R17](./research.md#r17--time-dependent-acceptance-scenarios)).

**Helper Utilities**:

- Existing: `server.AuthenticatedGET`, `AuthenticatedPOST`, `DirectRequest` (for `PUT`, `DELETE`,
  `If-Match`).
- New in `tests/e2e/helpers/service_account.go`: a decode helper for the representation and a
  Gomega matcher that asserts a response contains neither the JWT under test nor any of its three
  segments. Every scenario that submits a JWT applies it to each response it reads.
- Mock services: none. The feature makes no outbound call.

### Unit & Integration Tests

**Unit Tests** (TDD, written first):

- `internal/domain/model/service_account_credential_test.go`: table tests for every rule and its
  order, reason and message; the size boundary (16384 and 16385 bytes) before parsing; each
  library case of research R4; `alg` `none`, `NONE` and missing; `exp` at, before and after the
  request time; absent `iss`, `sub`, `kid`; fingerprint equality for equal tokens and difference for
  different ones; no error message contains the input.
- `internal/domain/model/service_account_test.go`: entity validation, ceiling rules,
  `StatusAt` boundary, `Narrowing`, `CredentialSummary.LogValue` capping.
- `internal/domain/thirdparty/service_account_service_test.go`: create, rotation, ceiling change,
  rename, combined change, no-op (no write, no audit), precondition, name conflict, kind change,
  delete, bound-account refusal, encryption failure stores nothing, encryption context equals the
  owning service's, audit records and keys per the observability contract, credential and its
  segments absent from the log buffer.
- `internal/domain/thirdparty/service_test.go`: service deletion refused while accounts exist.
- `internal/adapters/storage/memory/service_account_repository_test.go`: adapter-specific
  behaviour (deep copies in and out, service-existence check); concurrent creates and updates
  under `-race`.
- `internal/adapters/storage/postgres/service_account_repository_unit_test.go` (no build tag,
  no database): error mapping to `StorageError` and the sentinels, and record conversion to and
  from the entity (Principle IX requires unit tests for both adapters).
- `tests/integration/storage/service_account_repository_test.go` (self-contained,
  `just test-integration`): the shared repository contract from
  `tests/integration/storage/contract/` on the memory store.
- `internal/adapters/http/handlers/admin/service_accounts_handler_test.go`: operator required
  before any read; identifier resolution; `If-Match` parsing; body limit and unknown members;
  status, `Location`, `ETag`; error mapping; credential absent from responses, log buffer and
  recorded spans for every operation (SR-006).
- `internal/domain/id/id_test.go`: `ServiceAccountID`.

**Integration Tests**:

- `tests/integration/migrations/service_accounts_migration_test.go` (`integration` tag): apply,
  rollback, reapply repeated twice with existing services and other rows intact (rolling back
  `050` removes only `service_accounts` rows, by design); unique `(service_id, canonical_id)`; each `CHECK`; deleting a service with
  an account fails with a foreign-key violation; no plaintext column.
- `tests/integration/storage/infra/service_account_test.go` and
  `internal/adapters/storage/postgres/service_account_repository_test.go` (`integration` tag): the
  shared repository contract on PostgreSQL; `23505` and version mismatch mapped to the sentinels;
  `23503` on service deletion mapped to `conflict`; storage spans and `storage.outcome`.
- `tests/integration/service_account_encryption_test.go` (self-contained): with the memory
  encryption backend, a credential stored for one service does not decrypt under another
  service's context (SR-001), and a failing encryption port leaves the store empty (SR-002).

**Test Coverage Goals**:

- Unit: all branches of the credential rules, the domain service and the handler's error mapping.
- Integration: migration `050` in both directions; every repository method on PostgreSQL.
- E2E: 100% of the acceptance scenarios testable in this feature (31 scenarios).
- Frontend E2E: not applicable.

## Complexity Tracking

No constitution violations require justification.
