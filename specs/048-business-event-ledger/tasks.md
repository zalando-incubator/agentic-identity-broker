# Tasks: Business Event Ledger

**Input**: Design documents from `specs/048-business-event-ledger/`.
**Prerequisites**: `spec.md`, `plan.md`, `research.md`, `data-model.md`, `contracts/`, `quickstart.md`.
**Status**: Original design approved; review revisions await approval (T004). No code task has started.
**Delivery**: One branch, `048-business-event-ledger`, and one pull request. The phases below are the order of work and of commits in that pull request.
**Tests**: Mandatory red-green-refactor under Principles VIII and XIII. A red test compiles and fails semantically. A compilation failure is not red evidence.
**Format**: `[ID] [P?] [Story] Description`. `[P]` means a different file from every other open task of the same group and no dependency on one. Paths are relative to the repository root.

## 🔒 Feature Scope [MANDATORY]

Every task cites a requirement, an acceptance scenario, a success criterion, or a binding
principle. The feature ends with the first stored business event, `consent.grant.revoked`.
Out of scope, by `spec.md`: any further business event, erasure on request, any API or UI that reads the
ledger, SSF, events from ExtProc and agentgateway, and a transactional guarantee.

Findings outside this scope that appear during the work are reported to the user under
"Feature Scope & Deferred Findings" and are not fixed here. Known at planning time: several
services still log an `action` key and not `event` (LOGGING.md section 2.2). This feature corrects
only the grant revocation call site.

**Validation ownership**: Do not run container-backed recipes (`just test-integration-infra`,
`just test-e2e-docker`, `just test-e2e-performance`) in a sandbox without Docker. Take their red
and green evidence from CI on the pushed commit, and report them as deferred, not as validated.

---

## Phase 1: Setup

**Purpose**: The decision and the standard are reviewable before code exists.

- [X] T001 Write `adrs/048-business-events-as-typed-log-records.md` with Context, Decision, Consequences, Alternatives, status Proposed (Principle II)
- [X] T002 [P] Add ADR 048 to the ADR indexes in `AGENTS.md` and `ARCHITECTURE.md` (Principle II)
- [X] T003 [P] Amend `coding-standards/LOGGING.md` section 8 and its binding sources (STD-LOGGING-8; Principle XV)
- [ ] T004 Preserve the original maintainer approvals in `plan.md` and record approval of REVIEW-048 and revised DB-048 separately before implementation. The original ADR acceptance is not approval of the revised Decisions 3, 7, and 10. Do not infer approval (Principle II).

**Checkpoint**: Original approvals remain recorded. REVIEW-048 and revised DB-048 require approval before Phase 2.5. The standard amendments remain bounded by their Approval Items; no principal-policy exception is proposed.

---

## 🔒 Phase 2: Design Preconditions (Blocking Prerequisites) [MANDATORY]

**Purpose**: Complete domain, configuration, API, database, concrete spec-journey, E2E, and
observability design before implementation.

**⚠️ CRITICAL**: No implementation begins until this entire phase is complete. The only code
that this phase adds is the compile scaffold of T017, which Principle VIII permits so that the
E2E tests compile.

### Phase 2a: Domain Model & Glossary [MANDATORY]

**Constitution Reference**: Principles II, V

- [X] T005 Identify and document the value object, the entity, the typed ID, and the repository in `specs/048-business-event-ledger/data-model.md`
- [X] T006 Document the invariants of the business event in `data-model.md`
- [ ] T007 Complete the remaining non-functional constraints in `ARCHITECTURE.md`: the 5 ms p99 overhead budget (SC-004), synchronous write timeout, accepted post-commit loss window, policy-rendered principal storage, prohibited credential data, and default-disabled ledger (SR-001, SR-002, FR-007). Include retention defaults, validation bounds, disabled behavior, and healthy-operation and recovery limits (SR-003, SC-005). Link to detailed contracts. The four glossary terms, Domain Event relationship, lifecycle-audit projection, and principal-policy chain are already documented (Principles II, V).

**Checkpoint**: Domain model complete and documented.

### Phase 2b: Configuration Design [MANDATORY]

**Constitution Reference**: Principle VII

- [X] T008 Document the configuration keys with defaults and a YAML example in `contracts/configuration.md`
- [ ] T009 [P] Commit `examples/config/business-events.yaml` and its entry in `examples/config/README.md`
- [ ] T010 [P] Add the five keys to `docs/configuration.md`. Write `docs/operations/business-events.md`: how to enable the ledger, the loss window of ADR 048, the retention period and how to change it, that retention runs only while the ledger is enabled so rows stay after it is disabled, and the absence of an erasure operation (SR-003). State the healthy-operation age bound and recovery behavior of SC-005, including that deletion failures can extend retention without a fixed limit.
- [ ] T011 [P] Add `broker.businessEvents.enabled`, `.writeTimeout`, and `.retention.*` to `charts/agentic-identity-broker/values.yaml`, `templates/configmap.yaml`, `values.schema.json`, and the chart README; extend the chart tests

**Checkpoint**: Configuration designed, with a committed example and the Helm deployment contract.

### Phase 2c: API Design [MANDATORY]

**Constitution Reference**: Principles IV, X

- [X] T012 Confirm that `api/enduser/openapi.yaml` and `api/admin/openapi.yaml` do not change. This feature adds no endpoint (SR-004)

**Checkpoint**: No API change.

### Phase 2d: Database Design [MANDATORY]

**Constitution Reference**: Principle IX

- [X] T013 Document the table, its indexes, and the migration name in `data-model.md`
- [ ] T014 Create `migrations/048_create_business_events.up.sql` and `.down.sql` from `data-model.md`. If `main` has taken number `048`, use the next free number and update every reference in this directory

**Checkpoint**: Schema designed and migration files present.

### Phase 2e: Frontend/Design System Review [MANDATORY IF FRONTEND]

Not applicable. The feature changes no UI.

### Phase 2f: E2E Acceptance Test Design [MANDATORY]

**Constitution Reference**: Principles VIII, XIII, XIV

- [X] T015 Map each acceptance scenario of `spec.md` to one `It()` description in `plan.md` Testing Strategy
- [X] T016 Review Journeys 1 and 2 of `spec.md` against their scenarios, `api/enduser/openapi.yaml`, and the configuration contract
- [ ] T017 Add only the compile scaffold touched by the E2E tests (Principle VIII): `BusinessEventID` through `internal/domain/id/gen_ids.go`, regenerated `uuid_ids_gen.go`, and its ID catalogue entry; `model.BusinessEvent` in `internal/domain/model/business_event.go`; `BusinessEventAppender` and `BusinessEventRetention` in `internal/ports/storage.go` and its catalogue, with no update or read operation (FR-010); compiling no-op append/delete methods in both storage adapters and their two factory accessors; `BusinessEventsConfig` and nested retention config in `internal/ports/config.go`. Do not add a reader, filter, cursor, or inspection accessor. E2E reads use explicit-column SQL in test code. Do not install defaults or environment bindings before T037. No handler, retention service, wiring, or validation. Confirm with `just build`.
- [ ] T018 Write `tests/e2e/business_event_ledger_test.go` with `Label("docker")`, PostgreSQL production bootstrap, one `Context` per precondition, and one `It()` each for US1.1, US1.2, and US1.3 with scenario references. Read committed rows through explicit-column SQL in test helpers, not a production reader port. Supply all five business-event configuration values explicitly; do not depend on loader defaults. Add `HaveBusinessEvent` in `tests/e2e/matchers/` (Principle XIII; SC-003).
- [ ] T019 Add the `It()` for US1.4 to the same file with `Label("docker")`: PostgreSQL storage. The E2E database user is a superuser, so the test does not revoke a privilege: it creates a `BEFORE INSERT` trigger on `business_events` whose function raises an exception, and drops the trigger and the function in `AfterEach`. Assert `204` and a failure-counter increase of exactly one. Assert exactly one original `consent.grant.revoked` audit record and exactly one `business_event.ledger.write_failed` record. Assert that both records carry the revocation request's trace ID (FR-006; US1.4).
- [ ] T020 Add the `It()` for US2.1 to the same file with `Label("docker")`: PostgreSQL storage, `retention.period: 24h`, `retention.interval: 1s`, two revocations, `recorded_at` of the first row set to two days ago with SQL; assert that only the second row remains and that no log record reports the deletion (FR-009; SC-005)
- [ ] T021 Run the five ledger scenarios with `just test-e2e-docker` where Docker is available, otherwise take their red evidence from CI on the pushed commit. Confirm semantic failures, not compilation failures: the stored row, audit `event`, failure signal, or retention deletion is absent. No `XIt`, `PIt`, `Skip()`, placeholder assertion, or red-phase comment. Record output and line numbers in the scenario mapping. Local handler and repository red evidence remains mandatory under T037.

**Checkpoint**: The journeys are reviewed. The five E2E tests are written and fail semantically.

### Phase 2g: Observability Design [MANDATORY IF TELEMETRY]

**Constitution Reference**: Principle XV

- [X] T022 Read `coding-standards/LOGGING.md` and `coding-standards/OPENTELEMETRY.md`; list every new log record, span, and metric instrument in `contracts/observability.md`
- [X] T023 [P] Confirm that the feature adds no span attribute namespace, so `OPENTELEMETRY.md` section 4.1 does not change. The new meter scope `business_events` needs the section 2 amendment of T025 (STD-OTEL-2)
- [X] T024 Identify the credential-handling path and the principals that need a safety test (`contracts/observability.md`, Safety tests)
- [ ] T025 Amend `coding-standards/LOGGING.md`: name the ledger handler and its failure-record path in the section 1 handler-chain rule (STD-LOGGING-1). Add the typed-definition rule to section 2.2, the guard test to section 9, and one line to the section 10 checklist (STD-LOGGING-2.2). Include the section 2.2 clarification that the PR description records event renames under `Breaking changes`, and generated release notes link to it without copying its contents. Remove the sentence "until they merge, no record is persisted" from section 8. Add the scope `business_events`, owned by the ledger handler, to the scope table in `coding-standards/OPENTELEMETRY.md` section 2 (STD-OTEL-2). State each change and its rationale in the pull request description (Principle XV)

**Checkpoint**: The observability design is listed against both standards, and the standard is amended.

---

## Phase 2.5: Foundational Infrastructure

**Purpose**: The definition type, both storage adapters, the ledger handler, and retention work
and are wired, with the ledger off. No definition exists in production code yet.

### Tests first [MANDATORY - Principle VIII]

- [ ] T026 [P] Write `internal/domain/bizevent/definition_test.go`: valid and invalid identifiers, panic on a duplicate, `LogValue` and `String` return the identifier, `Definitions()` is sorted, the zero value is not a definition (FR-002; research R12). Declare distinct definitions without and with `bizevent.OwnerVisible()`. Assert `OwnerVisible()` is false and true, respectively, on both returned definitions and their entries in `Definitions()` (definition visibility invariant).
- [ ] T027 [P] Write `internal/domain/model/business_event_test.go`: `Validate` rejects a zero ID, an empty event, and a zero time; `Copy` is deep (data-model invariants)
- [ ] T028 [P] Write same-package `internal/adapters/storage/memory/business_event_test.go`: `Append` sets `RecordedAt`, stores a deep copy, and returns `Conflict` for duplicate ID; `DeleteRecordedBefore` respects cutoff and limit and deletes oldest first. Inspect private adapter state without a production read API (FR-003, FR-009, FR-011).
- [ ] T029 [P] Write `tests/integration/storage/infra/business_event_test.go`: T028 cases against PostgreSQL, observed with explicit-column SQL; JSONB nested-attribute round trip; `Append` with a committed transaction in context; nullable policy-rendered principal; concurrent retention deletes preserve newer rows (FR-005, FR-009, FR-011; research R4). Start with 2,500 expired rows, newer rows, and a batch size of 1,000. In a test transaction, lock the first batch with `SELECT id FROM business_events WHERE recorded_at < $1 ORDER BY recorded_at, id LIMIT 1000 FOR UPDATE`. Start `DeleteRecordedBefore` in a goroutine. Poll `pg_stat_activity` with `pg_blocking_pids` until that delete waits on the test transaction, with a bounded timeout. Delete the selected IDs inside the test transaction and assert that it deletes 1,000 rows. Commit the test transaction, then assert that `DeleteRecordedBefore` returns zero without error. Assert that 1,500 expired rows and all newer rows remain (SC-005; research R10). Use bounded waits and cleanup that rolls back the transaction and joins the goroutine. Assert empty event validation, nil attributes stored as `{}`, both SQL CHECK constraints, and `Timeout` errors for expired contexts on `Append` and `DeleteRecordedBefore` (Principle IX).
- [ ] T030 [P] Write `internal/adapters/storage/postgres/business_event_test.go`: the two methods create `storage.append.business_event` and `storage.delete_recorded_before.business_event` spans, with status and attributes asserted as `span_test.go` does (OPENTELEMETRY.md section 3.1). T029 covers FR-005 against PostgreSQL.
- [ ] T031 [P] Add migration `048` to the apply, rollback, and repeat tests in `tests/integration/migrations/` (Principle IX)
- [ ] T032 Write `internal/adapters/telemetry/ledger_handler_test.go` with a capturing appender. Recognition: typed top-level events survive principal policy and store once; string, absent, zero, and grouped events are ignored. Occurrence metadata: construct a record with `slog.NewRecord` and a fixed historical timestamp, including fractional seconds. Assert captured `OccurredAt` equals `slog.Record.Time` and the occurrence ID has UUID version 7 (FR-003; research R6). Encoding: promoted keys leave the document; scalar, time, duration, group, LogValuer, and typed-ID values follow research R7; include `WithAttrs` and `WithGroup`. Level/cancellation: Info stores with operational Error level; cancelled requests still store. Metrics: use the global SDK meter provider with manual reader, restore it in cleanup, and do not parallelize these cases; assert stored/failed histogram outcomes, failure counter error kinds, bounded timeout, and nil Handle result. Rendering: identifiers agree in JSON and otelslog. Policy: a non-identity policy yields identical rendered `Principal`, `Actor`, and `operator_principal` in the row and sinks, including grouped and WithAttrs values, with no raw identity retained; typed events attached with WithAttrs still store (SR-002). Failure: exactly one fallback write-failed record, context-supplied actor/trace, no principal, failed-event attributes, or error cause; no appender recursion (FR-006; research R8). Safety: a model.Secret attribute stores only its redacted form (SR-001).
- [ ] T033 [P] Write `internal/domain/bizevent/retention/service_test.go` with a fake repository: a run repeats until a batch is short; it stops when the context ends; it does not log retention-driven deletion; it logs `business_event.ledger.retention_failed` with `error_kind` on an error and runs again at the next interval; no record has a principal (FR-009; contracts/ports.md section 4). Use controlled time, not wall-clock sleeps, to verify SC-005: immediate startup cleanup, expiry just after a run's cutoff, deletion by the next successful scheduled run, and multi-batch catch-up after a failed run. Restart the service with overdue rows to verify recovery after re-enable. Without concurrent retention deletes, assert that recovery clears all rows older than its start-time cutoff and preserves newer rows. Simulate a short concurrent delete while expired rows remain and the competing run fails. Assert that this service resumes cleanup at its next scheduled run and clears the backlog without deleting newer rows.
- [ ] T034 [P] Write `internal/adapters/telemetry/business_event_logging_guard_test.go`: it fails when a `bizevent.Definition` is logged under a key other than `event` (contracts/ports.md section 1)
- [ ] T035 [P] Write the configuration tests in `internal/config/`: defaults, range validation of `business_events.write_timeout` and of the three retention keys also with the flag off, environment variable binding (FR-007, FR-009, SR-003)
- [ ] T036 [P] Extend `internal/app/builder_test.go`: with the flag off the chain is the chain of `main`; with the flag on a typed event logged through `app.Logger` is stored; with the flag on and no appender the builder fails at start; the retention service runs with the flag on and stops on shutdown, and is not created with the flag off, as `session_cleanup_test.go` asserts for the session cleanup (FR-009)
- [ ] T037 Add the minimal stubs that let every test of T026 to T036 compile: `retention.NewService` with a `Run` that returns at once; the exported API of `internal/domain/bizevent` with empty bodies, `Validate` and `Copy` on the model with empty bodies, and `NewLedgerHandler` that returns a handler which stores nothing. Run `just test` and `just test-integration`. Record the semantic failure of each new test in the pull request description. A test that does not compile is not red (Principle VIII). Take the red evidence of T029 and T031 from CI

### Implementation

- [ ] T038 [P] Implement `internal/domain/bizevent/definition.go` (data-model.md; standard library imports only, research R13)
- [ ] T039 [P] Implement `Validate` and `Copy` in `internal/domain/model/business_event.go`
- [ ] T040 [P] Implement `internal/adapters/storage/memory/business_event.go` with compile-time assertions; update `internal/adapters/AGENTS.md`
- [ ] T041 [P] Implement `internal/adapters/storage/postgres/business_event.go` with `sqlx`. `Append` and `DeleteRecordedBefore` use `a.db`, not `storageExecutor(ctx)`. Each method starts its span with `startStorageSpan`. Wrap errors in `StorageError`
- [ ] T042 [P] After the configuration red gate of T037, install the defaults and environment bindings in `internal/config/loader.go`. Add the validation of `business_events.write_timeout` and of the three retention keys to `internal/config/validator.go` (FR-007, FR-009, SR-003).
- [ ] T043 Implement `internal/adapters/telemetry/ledger_handler.go` and the instruments of `contracts/observability.md`, obtaining `otel.Meter("business_events")` internally. Handle instrument creation errors per OPENTELEMETRY.md section 5.1. Update `principal_handler.go` to preserve a `bizevent.Definition` under `event` without resolving it; retain principal rendering for every other attribute. The handler receives only policy output, passes request context to fallback, and adds no context attributes (depends on T038).
- [ ] T044 [P] Implement `internal/domain/bizevent/retention/service.go` (contracts/ports.md section 4)
- [ ] T045 Wire the handler in `internal/app/builder.go` as `contracts/ports.md` section 3 shows, behind context and principal policy. Start retention only when the ledger is enabled, for every storage adapter, after fallible construction; cancel and await it on shutdown before storage and telemetry close, following the existing SessionCleanup lifecycle. No generalized scheduler, distributed lock, or change to the session-sweep schedule is introduced (research R10; depends on T042, T043, T044).
- [ ] T046 Run `just check`, `just test`, `just test-integration`, and `just test-e2e-backend`. All unit and integration tests of this phase pass. The E2E tests of T018, T019, and T020 remain red because no production event definition exists. T055 owns the green gate for T018 and T019, and T056 for T020, after T049–T050 introduce the typed revocation event.

**Checkpoint**: The ledger is wired and off, and retention runs. Nothing is stored even when the ledger is on. Retention unit and repository tests pass. User Story 2 awaits the E2E green gate at T056.

---

## Phase 3: User Story 1 - A logged business event is stored (Priority: P1) 🎯 MVP

**Goal**: A grant revocation stores one `consent.grant.revoked` event.

**Independent Test**: Enable the ledger, revoke a grant, read the ledger.

### Tests for User Story 1 [MANDATORY - Principle VIII] ⚠️

- [ ] T047 [US1] Extend external-package `consent_test` service tests: production service constructor, production ledger handler behind context and principal policy, and a test-local capturing appender. Assert one `consent.grant.revoked` row with principal, agent_id, grant_id; none on ErrGrantNotFound; one row with refresh_cleanup=failed and the returned cleanup error after committed deletion. No production reader port is needed. Grant deletion ends refresh authority independently of row cleanup; keep existing continuing-consent tests unchanged. The path handles no credential, so SR-001 needs no path test (LOGGING.md section 8; onboarding checklist).
- [ ] T048 [US1] Run T047. Confirm that the tests compile and fail because no event is stored. Record the output in the pull request description

### Implementation for User Story 1

- [ ] T049 [US1] Create `internal/domain/consent/events.go` with `GrantRevoked = bizevent.MustDefine("consent.grant.revoked", bizevent.OwnerVisible())`
- [ ] T050 [US1] In `RevokeConsentForPrincipal` of `internal/domain/consent/service.go`, replace `"action", "grant_revoked"` with `"event", GrantRevoked` and keep the other attributes. In the branch where the refresh cleanup fails after the committed delete, replace `"action", "grant_deleted"` with `"event", GrantRevoked` and keep `refresh_cleanup=failed`. Under `Breaking changes` in the pull request description, record both renames: `grant_revoked` → `consent.grant.revoked` and `grant_deleted` → `consent.grant.revoked`. This section is the changelog record required by LOGGING.md section 2.2.
- [ ] T051 [P] [US1] Update old-key assertions for grant_revoked and grant_deleted to `event` in `internal/domain/consent/service_test.go`, `tests/integration/grant_lifecycle_audit_test.go`, and `tests/e2e/grant_diagnostics_test.go`. Update the corresponding descriptions in `docs/configuration.md` Continuing consent and `ARCHITECTURE.md` Feature 049, preserving the cleanup-failure and unchanged HTTP behavior (LOGGING.md section 9; SC-002).
- [X] T052 [P] [US1] Align the Grant and Definition Lifecycle Audit glossary entry with LOGGING.md section 8, define Business Event terms now, and state their relationship to GrantRevoked and the existing domain events (Principles II, V).
- [ ] T053 [US1] Write `tests/e2e/business_event_ledger_performance_test.go` with `Label("performance")` by the protocol of SC-004 in `plan.md`. Run it with `just test-e2e-performance` where Docker is available and record both 99th percentiles and their difference in the pull request description (SC-004)
- [ ] T054 [US1] Fill in `checklists/onboarding.md` in the pull request description (FR-004, FR-012; SC-001)
- [ ] T055 Run `just check`, `just test`, `just test-integration`, and `just test-e2e-backend`. Confirm the four US1 ledger scenarios from T018–T019 pass unchanged in `just test-e2e-docker`, locally where Docker is available or from CI on the pushed commit.

**Checkpoint**: User Story 1 works end to end.

---

## Phase 4: User Story 2 - An operator limits how long events are kept (Priority: P2)

**Goal**: Rows older than the retention period are deleted.

**Independent Test**: Store two events, age one, wait for a retention run, read the ledger.

Retention does not depend on an event type, so its tests and implementation are in Phase 2.5
(T033, T044, and the retention parts of T028 to T030, T035, T036, T040 to T042, T045). Its E2E
test is T020.

- [ ] T056 [US2] After T049–T050 introduce the typed revocation event, confirm from CI that the E2E test of T020 passes without a change to its assertions. Verify SC-005 with the controlled-time scheduling and recovery tests of T033, the overlapping-delete PostgreSQL test of T029, the lifecycle tests of T036, and the PostgreSQL deletion test of T020. Also run section 6 of `quickstart.md` where PostgreSQL is available.

**Checkpoint**: Both user stories work. The feature is complete.

---

## 🔒 Phase N: Constitution Compliance Verification [MANDATORY COMPLIANCE SECTION]

**Purpose**: Verify constitution requirements and feature-scope compliance before the pull request is ready for review.

### Design Phase Verification [MANDATORY]

- [ ] T057 Verify that every task traces to a requirement, a scenario, or a binding constraint (Feature Scope)
- [ ] T058 Verify that the four domain terms are in the `ARCHITECTURE.md` Glossary. Verify that its observability section documents the performance, durability, principal-data, and retention constraints assigned to T007 (Principles II, V).
- [ ] T059 Verify that `examples/config/business-events.yaml` and its README entry exist (Principle VII)
- [ ] T060 Verify that the end-user API and the Admin API are unchanged (Principles IV, X)
- [ ] T061 Verify that the schema is documented and migration `048` has up and down files (Principle IX)
- [ ] T062 Verify that Journeys 1 to 4 match the implemented HTTP exchange, configuration, stored rows, log records, and metrics (Principle XIV)
- [ ] T063 Verify that all five acceptance scenarios have an `It()` in `tests/e2e/`, and that the pull request records their semantic red run (Principle XIII)
- [ ] T064 Verify that every new log record, span, and instrument is in `contracts/observability.md` (Principle XV)

### Implementation Phase Verification [MANDATORY]

**Feature Scope**:
- [ ] T065 Verify that unrelated remediation is excluded: only the grant revocation call site changed from `action` to `event`, and no second business event is declared

**API & Documentation** (Principles IV, X):
- [ ] T066 [P] Verify that no endpoint reads the ledger (SR-004)

**Architecture & Documentation** (Principle II):
- [ ] T067 [P] Verify that ADR 048 is Accepted and agrees with the implementation
- [ ] T068 [P] Verify that `docs/operations/business-events.md` states the retention period, how to change it, and that rows stay when the ledger is disabled. Verify that it states the healthy-operation age bound, startup and retry recovery behavior, and the absence of a fixed deletion deadline during failures (SR-003, SC-005).

**Configuration** (Principle VII):
- [ ] T069 [P] Verify that the configuration uses the broker's configuration system, and that no key is read from the environment directly
- [ ] T070 [P] Verify that the Helm values, configmap template, schema, and README have all five keys

**Database & Persistence** (Principle IX):
- [ ] T071 [P] Verify that migration `048` follows the sequential numbering and has apply, rollback, and repeat tests
- [ ] T072 [P] Verify both adapters are tested for every method of the two production-consumed ports, with no read or update operation (FR-010).

**Security** (Principles I, III):
- [ ] T073 [P] Verify that no custom cryptography was added
- [ ] T074 [P] Verify that the ledger handler test asserts that a credential is absent from the stored event, that no onboarded path handles a credential without its own such test, and that the write-failed record has no principal (SR-001)

**Coding Standards** (Principle XV):
- [ ] T075 Verify that every new or changed log call satisfies the LOGGING.md section 10 checklist, and that the ledger handler adds no `trace_id`, `actor`, or `calling_peer`
- [ ] T076 Verify both PostgreSQL methods create their `storage.*` spans, the handler acquires its meter from otel.Meter, and every instrument follows OPENTELEMETRY.md section 5.
- [ ] T077 Verify that `TestLoggedPrincipalsAreTyped` and the `forbidigo` rule are unchanged, and that no linter suppression was added

**Architecture Patterns** (Principles VI, XII):
- [ ] T078 [P] Verify that domain packages do not import adapters, that `bizevent` imports only the standard library, that the retention service is in `bizevent/retention`, and that ExtProc does not import `bizevent` (ADR 027)
- [ ] T079 [P] Verify that the logger and the appender are injected through the builder, with no `slog.Default()`

**Testing** (Principle VIII):
- [ ] T080 Verify that the pull request records a semantic red run for the tests of T021, T037, and T048, and that the tests changed minimally during implementation
- [ ] T081 Verify that with `business_events.enabled: false` the full suite passes, and that the only changed assertions in existing tests are those of T051 (SC-002)

**E2E Acceptance Testing** (Principle XIII):
- [ ] T082 Verify that each `It()` of `tests/e2e/business_event_ledger_test.go` maps to exactly one acceptance scenario, with its reference in a comment, and that the file has no `XIt`, `PIt`, or `Skip()`
- [ ] T083 Final gates: `just check`, `just test`, `just test-integration`, `just test-e2e-backend`; the container suites from CI on the pushed commit

---

## Dependencies & Execution Order

```text
Phase 1 ─► Phase 2 (2a ─ 2g) ─► Phase 2.5 ─► Phase 3 ─► Phase 4 ─► Phase N
```

- Phase 1 blocks implementation: review-revision approval must be recorded before code depends on the revised contracts; original approvals remain preserved.
- In Phase 2, T017 precedes T018, T019, T020, and T021. The other open tasks of Phase 2 touch different files.
- In Phase 2.5, T037 follows T026 to T036. T042 installs loader defaults and environment bindings only after T035 compiles and demonstrates missing configuration behavior at T037. T043 needs T038. T045 needs T042, T043, and T044.
- Phase 3 needs all of Phase 2.5.
- Tests before implementation. Ports before adapters. Adapters before wiring.

### Parallel opportunities

- Phase 2: T009, T010, T011, and T014.
- Phase 2.5 tests: T026 to T031, T033, and T034 to T036. T032 is one file and one task.
- Phase 2.5 implementation: T038 to T042 and T044.

## Implementation Strategy

All phases are one pull request. Commit after each task or logical group, so that the red run of
a test is a commit before its implementation. Validate with [quickstart.md](./quickstart.md) after
Phase 4. Later business events are separate features that build on this one.

## Notes

- A task with `[X]` is complete on the branch, not approved. Approval is recorded by T004 only.
- If `main` takes migration number `048` or ADR number `048` before the merge, renumber and update every reference in this directory.
