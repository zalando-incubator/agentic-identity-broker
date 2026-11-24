# Implementation Plan: Domain Event Ledger

**Date**: 2026-11-12 | **Spec**: [spec.md](./spec.md)

**Input**: Feature specification from `/specs/20261118-125900-domain-event-ledger/spec.md`

## Summary

The ledger stores audit records for selected domain events, such as grant revocation. The activity
page and, later, an SSF transmitter read the ledger. A service marks a record for the ledger with
a `domainevent.Definition` under `event`. The builder installs a `slog.Handler` that stores the record
in `domain_events` after the state change commits
([ADR 20261118-125900-domain-events-as-typed-log-records](../../adrs/20261118-125900-domain-events-as-typed-log-records.md)).
The table has seven promoted columns and one JSONB document. Adding an event type needs no migration.
Retention deletes old rows without regard to event type. The first stored event is `consent.grant.revoked`.

## Technical Context

**Language/Version**: Go (version from `go.mod`)
**Primary Dependencies**: `log/slog`, `go.opentelemetry.io/contrib/bridges/otelslog` (existing), `github.com/google/uuid` v1.6.0 (existing, `NewV7`), `github.com/jmoiron/sqlx` (existing). No new dependency.
**Storage**: PostgreSQL (one new table, migration `049`; `048` is already occupied), memory adapter
**Testing**: Go `testing` with table-driven tests, Ginkgo/Gomega E2E (`tests/e2e/`), testcontainers for PostgreSQL integration tests
**Target Platform**: Linux server (broker Deployment). ExtProc is not changed.
**Project Type**: single (Go backend). No frontend change.
**Performance Goals**: The 99th percentile of the grant revocation request rises by at most 5 ms with the ledger on, on PostgreSQL (SC-004)
**Constraints**: The write is outside the state change's transaction (ADR 20261118-125900-domain-events-as-typed-log-records, Decision 5). A failed write never changes the result of the logging operation (FR-006). Disabled by default (FR-007).
**Scale/Scope**: One event type, grant revocation, kept for 90 days by default: well below one row per user per day ([data-model.md](./data-model.md#volume-estimate))

## Approval Items

| ID | Item | Where | Status |
|---|---|---|---|
| ADR-048 | Typed audit records represent selected domain events. The ledger writes after commit, with the loss window that Decision 5 accepts. | [ADR 20261118-125900-domain-events-as-typed-log-records](../../adrs/20261118-125900-domain-events-as-typed-log-records.md) | Decisions 5 and 9 approved by jbspeakr on 2026-11-12. ADR accepted as a whole by jbspeakr on 2026-11-12 |
| STD-LOGGING-8 | Amendment of LOGGING.md section 8 (ledger as projection; no audit record inside a transaction). T025 removes the interim sentence "Feature 048 delivers the definition type and the handler; until they merge, no record is persisted." | Tasks T003 and T025 | Approved by jbspeakr on 2026-11-12. Extension for the T025 removal approved by jbspeakr on 2026-11-12 |
| STD-LOGGING-1 | Amendment of LOGGING.md section 1: the handler chain has a ledger branch, and the ledger handler sends its failure record through a context handler to the operational sinks | Task T025 | Approved by jbspeakr on 2026-11-12, as described here. T025 writes the text; a change beyond this description needs a new approval |
| STD-LOGGING-2.2 | Amend LOGGING.md section 2.2 for the typed definition and the PR-description changelog record for event renames. Add the guard test in section 9 and one line for the typed-definition rule to the section 10 review checklist | Task T025 | Approved by jbspeakr on 2026-11-12, as described here. Extension for the section 10 line approved by jbspeakr on 2026-11-12. T025 writes the text; a change beyond this description needs a new approval |
| STD-OTEL-2 | Amendment of OPENTELEMETRY.md section 2: add the instrumentation scope `domain_events`, owned by the ledger handler | Task T025 | Approved by jbspeakr on 2026-11-12, as described here. T025 writes the text; a change beyond this description needs a new approval |
| DB-048 | Table `domain_events`, retention index, and principal/agent history indexes | [data-model.md](./data-model.md) | Original table and two-index design approved by jbspeakr on 2026-11-12. Retention-only revision approved by the user on 2026-11-18; superseded for index scope by the user's subsequent explicit request, "we should create indexes on principal and agent". Preserve retention and add both history indexes; no reader is introduced. |
| CFG-BE | Configuration keys `domain_events.*` | [contracts/configuration.md](./contracts/configuration.md) | Approved by jbspeakr on 2026-11-12 |
| REVIEW-048 | Ledger behind principal policy with typed-marker preservation; authoritative visibility; append/retention-only ports; retention recovery; duplicate-key precedence; CLI precedence; safe instrument-initialization warnings | `20261118-125900-domain-events-as-typed-log-records` Decisions 3, 7, 10; contracts/ports.md; contracts/configuration.md; contracts/observability.md; research R3, R10–R12, R14; spec SC-005 | Approved explicitly by the user in this conversation on 2026-11-18: "I approve the outstanding approvals. complete the documented prerequisites" |

No approval is inferred. Task T004 retains the original approvals and records review-revision
approval separately. Phase 2.5 starts only after revised approval items and all Phase 2
preconditions are complete. The meter correction follows the existing standard; the retention
clarification changes no scheduling decision.

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

**Design Preconditions (BLOCKING)**:

A checked item records completed work or a confirmed non-applicable requirement, not merely an approved plan.
The artifact prerequisites are complete. Task completion and remaining verification are recorded in `tasks.md`; checked artifacts do not certify unrun container-backed checks.

- [x] **Domain Model**: Documented in [data-model.md](./data-model.md): Domain Event Definition, Domain Event Occurrence, Domain Event ID, and ledger repository.
- [x] **Domain Concepts**: Ledger glossary terms and linked non-functional requirements are in `ARCHITECTURE.md` (T007, T052).
- [x] **Entity IDs**: `id.DomainEventID` is generated and documented. The handler mints UUIDv7 occurrence IDs (T017; research R6).
- [x] **Configuration Design**: [contracts/configuration.md](./contracts/configuration.md) lists every key with a YAML example.
- [x] **Config Examples**: `examples/config/domain-events.yaml` and its README entry exist and are included with the implementation.
- [x] **Helm Deployment Contract**: All five values, the ConfigMap, schema, README, and chart tests are implemented. No new workload (T011).
- [x] **API Design First**: No API change. No endpoint is added or changed.
- [x] **API Documentation**: Not applicable. The end-user API and the Admin API are not changed.
- [x] **API Changes**: None.
- [x] **Database Design**: Atomic migration `049_create_domain_events` and schema-specific tests are present. PostgreSQL migration and repository execution passed through `just test-integration-infra` (T014, T031, T071, T089).
- [x] **E2E Acceptance Tests**: The original five container-backed scenarios preceded foundation behavior (T018–T020). Review coverage adds US1.5, for six mapped scenarios.
- [x] **E2E Test Mapping**: One `It()` per scenario (table in Testing Strategy).
- [x] **E2E Red Phase**: Compiling acceptance/performance tests with persistence assertions preceded foundation behavior. Docker was initially unavailable; the final PostgreSQL acceptance and SC-004 performance runs have now passed (T021, T084, T053, T055, T056).
- [x] **Frontend Playwright E2E**: Not applicable. No UI change.
- [x] **Frontend Screenshots**: Not applicable.

**Implementation Design**:

- **Security-First**: The ledger adds no data class beyond LOGGING.md section 6. It is off by default because it stores principals; an enabled ledger always runs retention (SR-003). The default is a data-minimisation default, not a disabled security control.
- **Architecture Docs**: `ARCHITECTURE.md` includes the ADR index, four glossary terms, implemented ledger behavior, and linked non-functional requirements (T002, T007, T052).
- **ADRs**: `20261118-125900-domain-events-as-typed-log-records`, including the explicitly approved review revisions.
- **Library-First Security**: No cryptography.
- **Zalando Guidelines**: Not applicable. No API change.
- **Documentation**: The runtime settings are listed in `docs/configuration.md`. Deployment and recovery details are in [contracts/configuration.md](./contracts/configuration.md) (T010).
- **Migration Testing**: Apply, rollback, and repeat in `tests/integration/migrations/` (T031).
- **Hexagonal Architecture**: Definition type and retention service in the domain, two focused ports with production consumers, two storage adapters, and the handler as a telemetry adapter. Domain services do not import the handler.
- **Persistence Patterns**: Follow `specs/004-persistence-layer/quickstart.md`: ISP ports in `internal/ports/storage.go`, both adapters, `sqlx`, `StorageError`, factory accessors.

**Principle XV (coding standards)**: Every new log record and instrument is listed in
[contracts/observability.md](./contracts/observability.md). T025's approved LOGGING.md amendments
are applied in sections 1, 2.2, 8, 9, and 10. OPENTELEMETRY.md registers the `domain_events` scope.
The ledger handler adds no `trace_id` or `actor`: its failure record passes the context handler.
The PostgreSQL append, clock sample, and delete methods create the contracted `storage.*` spans. The existing principal guard
is unchanged, and a separate typed-event key guard is present.

**Result**: See [Current verification status](./tasks.md#current-verification-status) for the ledger checks and the open full PR-readiness gate.

## Project Structure

### Documentation (this feature)

```text
specs/20261118-125900-domain-event-ledger/
├── spec.md
├── plan.md
├── research.md
├── data-model.md
├── quickstart.md
├── tasks.md
├── contracts/
│   ├── ports.md
│   ├── configuration.md
│   └── observability.md
└── checklists/
    └── requirements.md
```

### Source Code (repository root)

```text
adrs/20261118-125900-domain-events-as-typed-log-records.md
coding-standards/LOGGING.md
coding-standards/OPENTELEMETRY.md
internal/domain/domainevent/{definition.go,definition_test.go}
internal/domain/domainevent/retention/{service.go,service_test.go}
internal/domain/id/{gen_ids.go,uuid_ids_gen.go,AGENTS.md}
internal/domain/model/domain_event.go
internal/ports/{storage.go,config.go}
internal/adapters/storage/factory.go
internal/adapters/storage/memory/domain_event.go
internal/adapters/storage/postgres/domain_event.go
internal/adapters/telemetry/
├── ledger_handler.go
├── ledger_handler_test.go
├── principal_handler.go
└── domain_event_logging_guard_test.go
internal/app/builder.go
internal/config/{loader.go,validator.go}
internal/domain/consent/{events.go,service.go}
cmd/agentic-identity-broker/root.go
migrations/049_create_domain_events.{up,down}.sql
examples/config/domain-events.yaml
charts/agentic-identity-broker/
docs/configuration.md
tests/e2e/domain_event_ledger_test.go
tests/e2e/domain_event_ledger_performance_test.go
tests/e2e/matchers/
tests/integration/migrations/, tests/integration/storage/infra/
```

**Structure Decision**: Single Go project in the existing hexagonal layout. The new packages are
`internal/domain/domainevent` and its subpackage `retention`. Everything else extends existing packages.

## Delivery

One branch and one pull request. Commit reviewable groups of tests
and implementation in the dependency order below. Tests precede the behavior they exercise;
neither separate red commits nor recorded red-run output are required by the constitution.

## Implementation Phase Overview

| Phase | Purpose | Required? | Tasks |
|-------|---------|-----------|-------|
| **Phase 0** | Pre-implementation refactoring | Skipped | none |
| **Phase 1** | Setup: ADR, standard amendment, approvals | Customizable | T001 to T004 |
| **Phase 2** | Design Preconditions, 2a to 2g | **MANDATORY** | T005 to T025, plus T031 before T014 and T084 before Phase 2.5 |
| **Phase 2.7** | Entity Boilerplate | Folded into Phase 2f | T017 |
| **Phase 2.5** | Foundational Infrastructure: definition type, adapters, handler, retention, wiring | Customizable | T026 to T046, except T031 |
| **Phase 3** | User Story 1: `consent.grant.revoked` | Customizable | T047 to T055 |
| **Phase 4** | User Story 2: retention, verified end to end (its code is in Phase 2.5) | Customizable | T056 |
| **Phase N** | Constitution Compliance verification | **MANDATORY** | T057 to T083 |

- [x] Phase 0 (refactoring): skip. The builder change is a few lines around the existing chain and needs no preparatory refactoring.
- [x] Phase 2.7 (entity boilerplate): not a separate phase. The E2E tests of Phase 2f must compile before implementation (Principle VIII), and production bootstrap needs the model, ports, and adapter scaffold from T017. Tests inspect committed rows through explicit-column SQL in test helpers, not a production reader port. T017 adds only the empty-but-compiling scaffold inside Phase 2f, as Principle VIII permits. The entity has no HTTP handler.

Phase 2 must be complete before Phase 2.5 begins: glossary and non-functional constraints,
the committed configuration example, the Helm contract, migration files, standard amendments,
and the original five E2E acceptance tests plus the SC-004 performance test written before foundation behavior. These prerequisites are complete. Review coverage adds the sixth scenario, US1.5.
T017 adds configuration types only. E2E fixtures supply all five values explicitly.
T035 precedes T042's loader defaults, environment bindings, CLI registration, and validation.

## Detailed Implementation Phases

The stages below explain the implementation boundaries and integration order. Task IDs refer to
the existing `tasks.md`; they are traceability references, not permission to follow stale task
instructions over the constitution. Each stage writes its behavior tests before implementation.
Container-backed tests may first run in CI when no local container runtime is available.

### Phase 2 completion: executable design prerequisites

**Entry**: Preserve original approvals and obtain the review-revision approvals of T004. The
architecture remains governed by `20261118-125900-domain-events-as-typed-log-records`.

Complete T007, T009–T011, and T025 before foundation implementation. T017 supplies only the types,
ports, and empty methods needed to compile acceptance tests. T018–T020 define the original five complete
HTTP acceptance scenarios, using PostgreSQL inspection in test code, not a production reader.
T031 defines migration-specific schema assertions before T014 creates the SQL. Both tasks belong
to Phase 2d. The existing general migration lifecycle test does not assert the new table's
columns, constraints, or retention, principal, and agent index definitions.
T084 writes the SC-004 performance test after T017 and before Phase 2.5. Its enabled-block
persistence assertions must detect the missing ledger behavior. If a local container runtime is
available, verify semantic failure before implementation. If no local container runtime is available,
[Constitution Principle VIII](../../.specify/memory/constitution.md#viii-test-driven-development--automated-testing)
permits the first run in CI on a commit that includes the implementation. T053 runs the final
measurement with unchanged assertions. T080 verifies this test-first ordering.

**Exit**: Design approvals and prerequisite artifacts are complete; acceptance tests compile
and exercise concrete outcomes; schema-specific tests precede the migration. No production
domain event is declared. Semantic red-green development is required, but saved failure logs
and separate red commits are not delivery conditions.

### Phase 2.5a: definition and occurrence semantics

**Scope**: T026–T027, T038–T039; the scaffold comes from T017.

Implement the standard-library-only definition package and the occurrence model described in
[data-model.md](./data-model.md). The registry owns identifier uniqueness and sorted enumeration;
the declaration owns visibility. Keep the definition separate from a stored occurrence: visibility
is resolved from the current definition, not copied into historical rows. The model validates
the occurrence. The adapters own the stored snapshot. Do not introduce a reader, cursor, or update operation.

**Exit**: Definition and model tests pass for invalid identifiers, duplicates, zero definitions,
visibility, and occurrence validation. Adapter tests cover snapshot isolation in Phase 2.5b.

### Phase 2.5b: persistence adapters

**Scope**: T028–T030, T040–T041; migration tests T031 and migration T014 belong to Phase 2d.

Implement the two ports in both adapters and expose them through the storage factory. Memory
storage owns its copies and assigns storage time. PostgreSQL uses explicit-column SQL, `sqlx`,
the retention index, configured storage timeouts, and `StorageError`. Both PostgreSQL methods
use the pool rather than a transaction in context. Do not reuse the state-change transaction.
Create the two storage spans through the existing helper, without principal attributes.

**Exit**: Repository tests verify append, duplicate IDs, storage time, JSONB representation,
copy isolation, bounded deletes, constraints, and timeout classification. PostgreSQL tests prove
append works with a committed transaction still in context and overlapping deletes preserve
newer rows. Migration apply, rollback, and reapply tests pass where PostgreSQL is available.
Timestamp tests follow [the precision contract](./data-model.md#timestamp-precision): memory retains
occurrence nanoseconds, while PostgreSQL round trips truncate to microseconds without rounding.
Both adapters preserve the caller's `OccurredAt`; only `RecordedAt` is assigned back.

### Phase 2.5c: policy-safe ledger handler

**Scope**: T032, T034, T043; depends on Phase 2.5a and the storage ports, not on finished adapters.

Preserve the typed `event` marker through principal policy, while rendering all principals before
fan-out. Implement inherited attributes and groups, top-level recognition, promoted columns,
and the encoding rules in research R7. Mint UUIDv7 occurrence IDs and preserve the record time.
The ledger branch accepts Info and above independently of the operational branch's level.

Append synchronously using a cancellation-independent context with a `write_timeout` deadline. On
failure, measure and count the failed attempt, emit exactly one safe diagnostic through the
context-and-policy fallback, and return nil. The fallback never includes the ledger handler;
neither it nor the row receives raw identities through a side channel. Instrument creation
follows OPENTELEMETRY section 5.1 and must not fail startup or a request.

Start the deadline immediately before `Append`, after event construction. It does not bound
attribute encoding, instrumentation, operational sinks, or the total logging operation or request.
A timed-out append still has an unknown database outcome, as the configuration contract states.

**Exit**: Handler tests prove recognition, encoding, inherited/grouped behavior, policy equality,
level independence, cancellation handling, timeout behavior, credential absence, and failure
isolation. The logging guard rejects definitions under keys other than `event`.

### Phase 2.5d: configuration and retention service

**Scope**: T033, T035, T042, T044; depends on the shared ports and configuration types.

Expose all five settings through the broker's file, environment, and CLI configuration paths,
with CLI over environment over file over defaults. Extend the existing command registration
and loader conventions rather than adding a separate configuration mechanism. Validate duration
and batch bounds even while disabled; keep examples, reference documentation, and Helm aligned.

Implement retention against its focused port in `domainevent/retention`. Sample the storage clock once per run to fix the cutoff.
Delete batches until a short batch, cancellation, or failure, and avoid overlapping runs
within a replica. Run immediately at startup and then on the configured schedule. Log failures,
not successful deletions; retry on the next interval. Concurrent replicas may end a run early,
so use SC-005's recovery contract rather than claiming a successful run clears every backlog.

**Exit**: Configuration tests verify defaults, bounds, and file/environment/CLI precedence.
Controlled-time service tests verify startup, scheduling, cancellation, multi-batch cleanup,
failure recovery, re-enable recovery, and the short-concurrent-delete case. No deletion counter
or success log is introduced.

### Phase 2.5e: builder integration and lifecycle

**Scope**: T036, T045–T046; depends on Phases 2.5b, 2.5c, and 2.5d.

Assemble the exact chain in [contracts/ports.md](./contracts/ports.md#3-ledger-handler-internaladapterstelemetryledger_handlergo).
When disabled, retain the existing logger chain and create no retention service. When enabled,
require the appender, construct fallible dependencies before starting background work, and start
retention for either storage adapter. On shutdown, cancel and await retention before closing
storage or telemetry, following the existing SessionCleanup lifecycle.

**Exit**: Builder tests verify disabled behavior, synthetic typed-event persistence through
`app.Logger`, enabled construction failure without an appender, and retention startup/shutdown.
Foundation unit and integration tests pass. The six production-event acceptance scenarios are
not required to pass yet: the consent definition is deliberately introduced in Phase 3.

### Phase 3: first production event and consumer-visible verification

**Scope**: T047–T055; depends on the integrated foundation.

Write consent tests in the internal `consent` package with local repository and log-handler doubles.
Cover event emission, rejected deletion, unchanged retries, and the deleted grant snapshot.
The integration test `TestGrantLifecycleAudit_RevocationDomainEventPipeline` proves deletion precedes append through the production handler chain.
Declare `GrantRevoked` once in the consent package, then convert both committed-delete branches:
normal completion and refresh-cleanup failure. Preserve the cleanup error, `refresh_cleanup`, resource attributes, and HTTP behavior.
A missing grant emits no event. Update affected log-contract assertions and documentation, and record both identifier renames.

**Exit**: The five US1 acceptance scenarios pass, including cleanup failure, insert rejection, and disabled
mode. SC-001 establishes the package-local change limit, verified by T054. Run SC-004's actual
PostgreSQL request benchmark, with untimed persistence checks described below; a no-op ledger
must not satisfy the latency criterion.

### Phase 4 and Phase N: retention acceptance and final compliance

**Scope**: T056–T083; depends on Phase 3 because retention acceptance produces real revocations.

Verify the remaining US2 scenario against PostgreSQL without changing its assertions. Combine
that result with controlled-time recovery and concurrent-delete tests; a single successful
retention run does not establish the deployment-wide recovery bound. Follow the quickstart with
test data only. Verify final architecture, configuration, standards, migration, and onboarding
compliance, then run the matching final recipes. Preserve the accepted loss window and explicitly
report any container checks not run; do not claim them as validated.

**Exit**: All six acceptance scenarios and the performance criterion pass in a suitable runtime,
the disabled-mode regression suite passes, and compliance checks are complete. No additional
event, reader API, UI, SSF delivery mechanism, or erasure operation is included.

### Dependencies and implementation gate

After Phase 2.5a, Phases 2.5b–2.5d can proceed independently against the stable ports. Phase 2.5e
waits for all three; Phase 3 waits for integration; Phase 4 and Phase N verify the completed flow.

**Implementation gate:** See [Current verification status](./tasks.md#current-verification-status).

The [specification-quality checklist](./checklists/requirements.md) retains four unchecked criteria.
On 2026-11-18, the user explicitly accepted all four documented specification-quality deviations
and authorized implementation. Checklist markers remain unchanged. This acceptance does not waive
implementation, security, testing, or compliance requirements. The T054 onboarding review is published in PR #467.

## Testing Strategy

### End-to-End (E2E) Acceptance Tests

**Test Location**: `tests/e2e/domain_event_ledger_test.go`

**Framework**: Ginkgo/Gomega BDD framework following patterns in [tests/e2e/README.md](../../tests/e2e/README.md)

**Test Organization**: `Describe("Domain event ledger")` → `Context` per precondition (ledger
enabled, disabled, write failing; retention) → one `It` per scenario.

**Scenario Mapping**:

| Spec Scenario | E2E Test | Test Description | Red before implementation because |
|---|---|---|---|
| US1.1 | `domain_event_ledger_test.go`, `Label("docker")` | `It("stores exactly one consent.grant.revoked event with request and resource identities")` | No row is stored |
| US1.2 | `domain_event_ledger_test.go`, `Label("docker")` | `It("keeps exactly one event when a second revocation returns 404")` | The row of the first revocation is absent |
| US1.3 | `domain_event_ledger_test.go`, `Label("docker")` | `It("stores no event and still logs the revocation when the ledger is disabled")` | The audit record has `action`, not `event` |
| US1.4 | `domain_event_ledger_test.go`, `Label("docker")` | `It("returns 204 and reports exactly one failed write without losing the audit record")` | No failure is counted or logged, and the audit record has `action`, not `event` |
| US1.5 | `domain_event_ledger_test.go`, `Label("docker")` | `It("stores the deleted grant snapshot when refresh cleanup returns an HTTP error")` | Cleanup failure omits the full deleted grant snapshot |
| US2.1 | `domain_event_ledger_test.go`, `Label("docker")` | `It("deletes only the expired event without a deletion log or counter")` | No events are stored; after onboarding but before retention, the expired row remains |

The original five scenarios passed through `just test-e2e-docker`. All six ledger scenarios,
including US1.5, passed the focused PostgreSQL-backed Ginkgo run on 2026-11-19.
The SC-004 test at `domain_event_ledger_performance_test.go:94` previously passed through
`GOMAXPROCS=14 just test-e2e-performance`, with unchanged assertions; its complete environment
and latency record is published in PR #467.

**Red Phase Requirements**: As in the plan template. No `XIt`, `PIt`, or `Skip()`. Assertions
target the stored event (`Event`, `Principal`, `Actor`, `TraceID`, attribute keys), the HTTP
status, the log buffer, and the metric reader.

**Test Data Strategy**:
- Existing fixtures for agents, grants, and principals from `tests/e2e/fixtures/`.
  PostgreSQL acceptance requires `SeedDefaultConsentData` to set `UserSession.Scope` to an empty slice.
  A nil slice reaches `user_sessions.scope` as SQL `NULL`, which the `NOT NULL` column rejects.
- New fixture: a configuration variant with `domain_events.enabled: true`.
- US1.4 needs a ledger write that fails. The memory adapter cannot fail, and `storage.Adapter` is
  a concrete type, so the test runs in the container E2E suite against PostgreSQL. The E2E
  database user `testuser` is a superuser, so revoking a privilege has no effect. The test
  creates a `BEFORE INSERT` trigger on `domain_events` whose function raises an exception, and
  drops the trigger and the function in `AfterEach`. No test-only production API is added. The
  handler's failure path is also covered by unit tests with a failing appender.
- US1.5 creates a grant with a non-null expiry through the consent API and seeds one refresh session.
  A PostgreSQL `BEFORE UPDATE` trigger rejects cleanup, and the real HTTP revocation returns `500`.
  SQL verifies committed grant deletion, the uncleaned refresh row, and one ledger snapshot with the UTC RFC3339Nano expiry.
  A repeated DELETE returns `404`. SQL verifies that the same refresh row remains uncleaned and the grant remains absent.
  The ledger occurrence stays unchanged.
- US1.1 creates the grant through the API first. That audit record has no typed definition, so
  "exactly one row" also proves that a record without a definition is not stored (FR-001).
- US2.1 needs a row that is older than the retention period. The smallest valid period is 24
  hours and no production API sets the storage time, so the test runs against PostgreSQL: it
  revokes two grants and asserts that both committed events exist before ageing either row.
  It sets the first row's `recorded_at` to two days ago with SQL and asserts that exactly one row changes.
  It uses `retention.period: 24h` and `retention.interval: 1s`, then asserts that only the second row remains.
  Before event onboarding, the test fails because events are absent. T029 and T033 isolate deletion behavior with controlled fixtures.
- T020 cannot turn green in Phase 2.5: its two revocations need the typed event from T049–T050.
  Retention unit and repository tests turn green in Phase 2.5. T056 owns the retention E2E green gate.

**Bootstrap Strategy**:
- All six scenarios use production bootstrap against PostgreSQL and inspect committed rows with
  explicit-column SQL in test helpers; no production reader port is introduced for assertions.
- US1.3 and US1.4 assert log output through the existing buffer-logger bootstrap path.
- These scenarios run in `just test-e2e-docker`. Same-package repository tests cover the memory adapter.
  Handler tests use test-local append capture. Consent tests use local doubles, and integration tests cover the production handler chain.

**Helper Utilities**:
- New matcher `HaveDomainEvent(event, principal)` in `tests/e2e/matchers/`.
- Existing HTTP helpers.

### Frontend Playwright E2E Tests

Not applicable. No UI change.

### Unit & Integration Tests

**Unit Tests**:
- `internal/domain/domainevent/definition_test.go`: identifier validation, duplicate panic, `LogValue`, `Definitions()` order. Definitions without and with `domainevent.OwnerVisible()` return false and true, respectively, including their entries in `Definitions()`.
- `internal/domain/model/domain_event_test.go`: `Validate`, `Copy`.
- `internal/adapters/storage/memory/domain_event_test.go`: append, conflict, defensive copy, and retention delete, observed through adapter-private state in same-package tests.
  Include fixed sub-microsecond occurrence times and assert full nanosecond preservation with `time.Time.Equal` (T028).
- `internal/domain/domainevent/retention/service_test.go`: batching, cancellation, no deletion log, and failed-run logging. Controlled-time tests cover immediate startup, expiry just after a cutoff, scheduled deletion, and multi-batch recovery after failure or re-enable (T033; SC-005). Healthy runs start no more than one interval apart and complete before the next scheduled start. Without concurrent retention deletes, recovery tests assert deletion of all rows older than the recovery run's cutoff and preservation of newer rows. A simulated short concurrent delete leaves expired rows while the competing run fails. The next scheduled run resumes cleanup and clears the backlog without deleting newer rows.
- `internal/adapters/telemetry/ledger_handler_test.go`: recognition after principal policy, ignored records, attribute encoding (research R7), `WithAttrs`, `WithGroup`, level independence, cancelled context, timeout and failure metrics, fallback context attributes, no recursion, credential absence (SR-001), and identifier rendering in JSON and `otelslog` (FR-002). A fixed historical `slog.Record.Time` with fractional seconds must equal the captured `OccurredAt`. The generated occurrence ID must have UUID version 7 (FR-003; research R6). A non-identity policy must produce identical `principal`, `actor`, and `operator_principal` values in the captured row and operational sinks, including groups and `WithAttrs` (SR-002). Metric tests use the global meter provider with cleanup, not constructor injection.
  Include a non-UTC record time with `.123456789` fractional seconds; capture the same instant in UTC without truncation (T032).
- `internal/adapters/storage/postgres/domain_event_test.go`: the two `storage.*` spans. FR-005 needs PostgreSQL and is covered by the integration test.
- `internal/adapters/telemetry/domain_event_logging_guard_test.go`: a definition is logged only under `event`.
- `internal/domain/consent/` service tests use the internal `consent` package with local repository and log-handler doubles.
  They cover the deleted grant snapshot, one event after committed deletion, and no event on missing or failed deletion.
  They also cover unchanged retries and cleanup failure.
- `internal/config/` and `cmd/agentic-identity-broker/`: defaults, range validation, all five CLI flags, and file/environment/CLI precedence (T035). Tests precede T042. T037 verifies missing behavior where the runtime is available; recorded red-run output is not required.

**Integration Tests**:
- `tests/integration/grant_lifecycle_audit_test.go`: `TestGrantLifecycleAudit_RevocationDomainEventPipeline` uses the production context, principal-policy, fan-out, and ledger handlers.
  The appender asserts that the grant is absent before append. The test compares operational and stored snapshots for successful and failed cleanup.
- `tests/integration/migrations/`: migration `049` apply, rollback, repeat; assert all three secondary index definitions from `data-model.md`, including the agent JSONB expression and history sort keys. Verify principal/agent equality filtering and deterministic ordering for tied occurrence times, with absent-principal and absent-agent fixtures remaining valid.
- `tests/integration/storage/infra/domain_event_test.go`: PostgreSQL adapter against testcontainers, inspected with explicit-column SQL: `Append` with a committed transaction in context (research R4), JSONB round trip, concurrent retention deletes, both `CHECK` constraints, nil attributes stored as `{}`, and `Timeout` errors for an expired context deadline (Principle IX). For T029, a test transaction locks the first 1,000 of 2,500 expired rows with `SELECT id ... FOR UPDATE`. The test starts `DeleteRecordedBefore` in a goroutine. It polls `pg_stat_activity` with `pg_blocking_pids` until that delete waits on the test transaction. The test transaction deletes the locked IDs and commits. The adapter call then returns zero, with 1,500 expired rows and all newer rows still present (SC-005). T029 specifies the SQL, bounded waits, and transaction/goroutine cleanup.
  T029 also appends fixed occurrence times ending in `.123456789` and `.999999999`, including a non-UTC offset.
  Read `occurred_at` and `recorded_at` with explicit-column SQL. Compare `occurred_at` by instant to
  `input.OccurredAt.UTC().Truncate(time.Microsecond)`, never a tolerance or `Round`; verify the caller's
  `OccurredAt` is unchanged and its returned `RecordedAt` equals the stored `recorded_at` (FR-003; research R16).
- `internal/app/builder_test.go`: chain with the flag on and off; start failure when the flag is on and no appender exists; the retention service runs with the flag on, stops on shutdown, and is not created with the flag off.

**Test Coverage Goals**:
- Unit: all branches of the handler and the definition type.
- Integration: both adapters for every port method.
- E2E: 100% of the acceptance scenarios in spec.md.
- Performance: the measurement below.

### SC-004 Measurement Protocol

`tests/e2e/domain_event_ledger_performance_test.go`, `Label("performance")`, run with
`just test-e2e-performance`. It needs Docker and is not part of the default suites.

T084 authors this test in Phase 2f before foundation implementation, using T017's scaffold and
explicit values for all five configuration settings. The enabled-block row assertions must fail
without ledger persistence, even if latency meets the budget. T053 performs the final measurement
after integration; it does not introduce or weaken the test's assertions.

**Environment record (required for acceptance)**:

Record these values in the PR description alongside the latency results:

- Host or CI runner identity, CPU model and architecture, available CPU count, and memory.
- Host OS, Go version, `GOMAXPROCS`, and Docker version.
- Docker VM and PostgreSQL container CPU and memory limits, or an explicit statement that no limits apply.
- PostgreSQL server version, container image tag and digest, storage type, and non-default server configuration, excluding credentials.
- Broker/database placement and connection path, pool configuration, and all five domain-event configuration values for each broker.

Run both brokers and the PostgreSQL container on the same host, as step 1 specifies.
Keep resource limits and configuration unchanged across enabled and disabled blocks, except for the enable flag.
Run no unrelated test or benchmark workload on that host during measurement.
The 5 ms gate applies to this documented environment, not to every deployment.
Results without the environment record do not establish SC-004 acceptance.
Keep the complete result of each acceptance run with its environment record, including failed runs.

1. Start one PostgreSQL container with the production migrations. Start two brokers through the
   production bootstrap against it, in the same process: one with `domain_events.enabled: false`
   and one with `true`. Both use the same pool settings and no telemetry export.
2. For each broker, 100 warm-up cycles, then 1,000 measured cycles. Each cycle creates a grant
   for Alice and one agent through the API, then revokes it. Require successful grant creation
   and a `204` revocation response in every cycle. Only the `DELETE` request is timed,
   from send to complete response, with a monotonic clock. Requests are sequential, with one
   client and one connection.
3. Alternate brokers in blocks of 100 cycles, including warm-up blocks, to limit host drift.
   Before and after each block, query committed ledger rows with explicit-column SQL.
   Require exactly 100 new rows for enabled blocks and zero new rows for disabled blocks.
   Require no `domain_event.ledger.write_failed` diagnostic during either block.
   Exclude SQL inspection and diagnostic assertions from samples. A bypassed ledger must fail
   the row assertions, even when its latency satisfies the budget.
4. For each broker, sort the 1,000 samples. The 99th percentile is the sample at index 989.
5. The test fails when `p99(on) - p99(off)` is above 5 ms. It reports both percentiles and both
   medians. The pull request description records them (T053).

## Complexity Tracking

| Violation | Why Needed | Simpler Alternative Rejected Because |
|-----------|------------|-------------------------------------|
| The ledger is off by default | It stores principals, and an operator must choose the retention period first | On by default would store personal data in deployments that have not planned retention |

## Published PR-description record

The following changelog, amendment rationales, and onboarding review were published to [PR #467](https://github.com/zalando-incubator/agentic-identity-broker/pull/467) on 2026-11-19. The PR description explicitly retains the unresolved validation gates.

### Breaking changes

- `action=grant_revoked` becomes `event=consent.grant.revoked`.
- `action=grant_deleted` becomes `event=consent.grant.revoked` after committed deletion with failed refresh cleanup. `refresh_cleanup=failed` and the returned cleanup error remain unchanged.
- Dashboard, alert, and log-query filters must use the new discriminator and identifier.

### Coding-standard amendments and rationale

- LOGGING section 1 names the ledger branch after context and principal policy. Its nonrecursive operational fallback preserves request metadata without re-entering the ledger.
- LOGGING sections 2.2, 9, and 10 require one typed declaration under `event`, a guard test, and a review check. They prevent duplicate catalogues and marker misuse.
- LOGGING section 2.2 makes the PR description's `Breaking changes` section the event-rename changelog record. Generated release notes link to that record without duplicating it.
- LOGGING section 8 removes the interim implementation notice. Post-commit projection semantics and unknown write-timeout outcomes remain explicit.
- OPENTELEMETRY section 2 registers `domain_events` so the ledger instruments have an explicit owner.

### Onboarding review: FR-012 and SC-001

Onboarding changes two consent production files: `events.go` and `service.go`. One package-local declaration has explicit owner visibility.
Both top-level emissions follow committed deletion, retain canonical resource attributes, and distinguish cleanup failure without changing the operation's return behavior.
Internal-package consent tests use local repository and log-handler doubles for snapshots, missing or failed deletion, unchanged retries, and cleanup failure.
`TestGrantLifecycleAudit_RevocationDomainEventPipeline` in `tests/integration/grant_lifecycle_audit_test.go` proves deletion precedes append through the production handler chain.
Revocation handles no credential. Onboarding adds no migration, API, or builder change. Those belong to the separate shared foundation.

