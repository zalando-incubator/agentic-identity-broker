# Implementation Plan: Business Event Ledger

**Branch**: `048-business-event-ledger` (one branch, one pull request) | **Date**: 2026-11-12 | **Spec**: [spec.md](./spec.md)

**Input**: Feature specification from `/specs/048-business-event-ledger/spec.md`

## Summary

The broker stores security-relevant facts (grant revoked, session established, approval denied)
in a ledger that the activity page and, later, an SSF transmitter read. A business event is the
audit log record that `coding-standards/LOGGING.md` already requires, with a typed
`bizevent.Definition` as its `event` value. A `slog.Handler` in the builder's logger chain stores
each such record as one row of the table `business_events`, after the state change has committed
([ADR 048](../../adrs/048-business-events-as-typed-log-records.md)). The table has seven promoted
columns and one JSONB attribute document, so a new event needs no migration. Retention deletes old
rows and does not depend on an event type. This feature ends with the first stored event,
`consent.grant.revoked`. It is delivered on one branch and in one
pull request.

## Technical Context

**Language/Version**: Go (version from `go.mod`)
**Primary Dependencies**: `log/slog`, `go.opentelemetry.io/contrib/bridges/otelslog` (existing), `github.com/google/uuid` v1.6.0 (existing, `NewV7`), `github.com/jmoiron/sqlx` (existing). No new dependency.
**Storage**: PostgreSQL (one new table, migration `048`), memory adapter
**Testing**: Go `testing` with table-driven tests, Ginkgo/Gomega E2E (`tests/e2e/`), testcontainers for PostgreSQL integration tests
**Target Platform**: Linux server (broker Deployment). ExtProc is not changed.
**Project Type**: single (Go backend). No frontend change.
**Performance Goals**: The 99th percentile of the grant revocation request rises by at most 5 ms with the ledger on, on PostgreSQL (SC-004)
**Constraints**: The write is outside the state change's transaction (ADR 048, Decision 5). A failed write never changes the result of the logging operation (FR-006). Disabled by default (FR-007).
**Scale/Scope**: One event type, grant revocation, kept for 90 days by default: well below one row per user per day ([data-model.md](./data-model.md#volume-estimate))

## Approval Items

| ID | Item | Where | Status |
|---|---|---|---|
| ADR-048 | Business events are typed log records; the ledger row is written after the commit, with the loss window that Decision 5 accepts | [ADR 048](../../adrs/048-business-events-as-typed-log-records.md) | Decisions 5 and 9 approved by jbspeakr on 2026-11-12. ADR accepted as a whole by jbspeakr on 2026-11-12 |
| STD-LOGGING-8 | Amendment of LOGGING.md section 8 (ledger as projection; no audit record inside a transaction). T025 removes the interim sentence "Feature 048 delivers the definition type and the handler; until they merge, no record is persisted." | Tasks T003 and T025 | Approved by jbspeakr on 2026-11-12. Extension for the T025 removal approved by jbspeakr on 2026-11-12 |
| STD-LOGGING-1 | Amendment of LOGGING.md section 1: the handler chain has a ledger branch, and the ledger handler sends its failure record through a context handler to the operational sinks | Task T025 | Approved by jbspeakr on 2026-11-12, as described here. T025 writes the text; a change beyond this description needs a new approval |
| STD-LOGGING-2.2 | Amend LOGGING.md section 2.2 for the typed definition and the PR-description changelog record for event renames. Add the guard test in section 9 and one line for the typed-definition rule to the section 10 review checklist | Task T025 | Approved by jbspeakr on 2026-11-12, as described here. Extension for the section 10 line approved by jbspeakr on 2026-11-12. T025 writes the text; a change beyond this description needs a new approval |
| STD-OTEL-2 | Amendment of OPENTELEMETRY.md section 2: add the instrumentation scope `business_events`, owned by the ledger handler | Task T025 | Approved by jbspeakr on 2026-11-12, as described here. T025 writes the text; a change beyond this description needs a new approval |
| DB-048 | Table `business_events` and retention index | [data-model.md](./data-model.md) | Original table and two-index design approved by jbspeakr on 2026-11-12. Review revision removes the speculative principal-query index; revised scope awaits approval |
| CFG-BE | Configuration keys `business_events.*` | [contracts/configuration.md](./contracts/configuration.md) | Approved by jbspeakr on 2026-11-12 |
| REVIEW-048 | Ledger behind principal policy with typed-marker preservation; authoritative definition visibility; append/retention-only ports and test-local inspection | ADR 048 Decisions 3, 7, 10; contracts/ports.md; research R3, R11, R12 | Proposed in response to review; original sign-off does not approve these revisions |

No approval is inferred. Task T004 retains the original approvals and records review-revision
approval separately. Phase 2.5 starts only after revised approval items and all Phase 2
preconditions are complete. The meter correction follows the existing standard; the retention
clarification changes no scheduling decision.

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

**Design Preconditions (BLOCKING)**:

A checked item records completed work or a confirmed non-applicable requirement, not merely an approved plan.
Unchecked items remain prerequisites for Phase 2.5. Task completion is recorded in `tasks.md`.

- [x] **Domain Model**: Documented in [data-model.md](./data-model.md): Business Event Definition, Business Event, Business Event ID, ledger repository.
- [ ] **Domain Concepts**: Glossary terms, the Domain Event relationship, and lifecycle-audit projection are now in `ARCHITECTURE.md`. T007 still owns the remaining non-functional constraints.
- [ ] **Entity IDs**: Add `id.BusinessEventID` through `gen_ids.go` and document it in `internal/domain/id/AGENTS.md` (T017). The handler must mint UUIDv7 IDs (research R6).
- [x] **Configuration Design**: [contracts/configuration.md](./contracts/configuration.md) lists every key with a YAML example.
- [ ] **Config Examples**: Commit `examples/config/business-events.yaml` and its README entry (T009).
- [ ] **Helm Deployment Contract**: Add `broker.businessEvents.*` values, configmap template, schema, README, and chart tests (T011). No new workload.
- [x] **API Design First**: No API change. No endpoint is added or changed.
- [x] **API Documentation**: Not applicable. The end-user API and the Admin API are not changed.
- [x] **API Changes**: None.
- [ ] **Database Design**: Create the designed migration `048_create_business_events` with up and down scripts (T014).
- [ ] **E2E Acceptance Tests**: Write all five mapped container-backed acceptance tests in Phase 2f, before implementation (T018 to T020).
- [x] **E2E Test Mapping**: One `It()` per scenario (table in Testing Strategy).
- [ ] **E2E Red Phase**: Run the tests against the scaffold of T017 and record their semantic failures in the pull request (T021).
- [x] **Frontend Playwright E2E**: Not applicable. No UI change.
- [x] **Frontend Screenshots**: Not applicable.

**Implementation Considerations (planned, not verified)**:

- **Security-First**: The ledger adds no data class beyond LOGGING.md section 6. It is off by default because it stores principals; an enabled ledger always runs retention (SR-003). The default is a data-minimisation default, not a disabled security control.
- **Architecture Docs**: `ARCHITECTURE.md` contains the glossary and revised lifecycle-audit entry (T052), plus logger-chain authority. T007 completes the 5 ms p99 budget, synchronous timeout, accepted loss window, policy-rendered principal handling, and retention bounds and recovery behavior. T058 verifies these constraints (Principle II).
- **ADRs**: ADR 048.
- **Library-First Security**: No cryptography.
- **Zalando Guidelines**: Not applicable. No API change.
- **End-User Docs**: `docs/configuration.md` and `docs/operations/business-events.md` (T010).
- **Migration Testing**: Apply, rollback, and repeat in `tests/integration/migrations/` (T031).
- **Hexagonal Architecture**: Definition type and retention service in the domain, two focused ports with production consumers, two storage adapters, and the handler as a telemetry adapter. Domain services do not import the handler.
- **Persistence Patterns**: Follow `specs/004-persistence-layer/quickstart.md`: ISP ports in `internal/ports/storage.go`, both adapters, `sqlx`, `StorageError`, factory accessors.

**Principle XV (coding standards)**: Every new log record and instrument is listed in
[contracts/observability.md](./contracts/observability.md). LOGGING.md is amended in sections 1,
2.2, 8, 9, and 10, and OPENTELEMETRY.md in section 2 for the new scope `business_events`
(approval items above). The ledger handler adds no `trace_id` or `actor`: its
failure record passes a context handler (LOGGING.md section 1). Each of the two PostgreSQL
repository methods creates a `storage.*` span (OPENTELEMETRY.md section 3.1). `TestLoggedPrincipalsAreTyped`
is not narrowed; a second guard test is added.

**Result**: Review revisions need approval, and the execution gate remains open.
T007, T009–T011, T014, T017–T021, and T025 remain incomplete in `tasks.md`.
T025 must write the approved standard amendments before Phase 2.5 begins; no section 5.1 exception is requested.
Phase N verifies implementation compliance; the planned considerations here do not certify it.

## Project Structure

### Documentation (this feature)

```text
specs/048-business-event-ledger/
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
    ├── requirements.md
    └── onboarding.md
```

### Source Code (repository root)

```text
adrs/048-business-events-as-typed-log-records.md
coding-standards/LOGGING.md
coding-standards/OPENTELEMETRY.md
internal/domain/bizevent/{definition.go,definition_test.go}
internal/domain/bizevent/retention/{service.go,service_test.go}
internal/domain/id/{gen_ids.go,uuid_ids_gen.go,AGENTS.md}
internal/domain/model/business_event.go
internal/ports/{storage.go,config.go}
internal/adapters/storage/factory.go
internal/adapters/storage/memory/business_event.go
internal/adapters/storage/postgres/business_event.go
internal/adapters/telemetry/
├── ledger_handler.go
├── ledger_handler_test.go
├── principal_handler.go
└── business_event_logging_guard_test.go
internal/app/builder.go
internal/config/{loader.go,validator.go}
internal/domain/consent/{events.go,service.go}
migrations/048_create_business_events.{up,down}.sql
examples/config/business-events.yaml
charts/agentic-identity-broker/
docs/configuration.md, docs/operations/business-events.md
tests/e2e/business_event_ledger_test.go
tests/e2e/business_event_ledger_performance_test.go
tests/e2e/matchers/
tests/integration/migrations/, tests/integration/storage/infra/
```

**Structure Decision**: Single Go project in the existing hexagonal layout. The new packages are
`internal/domain/bizevent` and its subpackage `retention`. Everything else extends existing packages.

## Delivery

One branch, `048-business-event-ledger`, and one pull request. The phases of
[tasks.md](./tasks.md) are the order of work and of commits. Each red run is a commit before its
implementation, so a reviewer can follow the pull request commit by commit.

## Implementation Phase Overview

| Phase | Purpose | Required? | Tasks |
|-------|---------|-----------|-------|
| **Phase 0** | Pre-implementation refactoring | Skipped | none |
| **Phase 1** | Setup: ADR, standard amendment, approvals | Customizable | T001 to T004 |
| **Phase 2** | Design Preconditions, 2a to 2g | **MANDATORY** | T005 to T025 |
| **Phase 2.7** | Entity Boilerplate | Folded into Phase 2f | T017 |
| **Phase 2.5** | Foundational Infrastructure: definition type, adapters, handler, retention, wiring | Customizable | T026 to T046 |
| **Phase 3** | User Story 1: `consent.grant.revoked` | Customizable | T047 to T055 |
| **Phase 4** | User Story 2: retention, verified end to end (its code is in Phase 2.5) | Customizable | T056 |
| **Phase N** | Constitution Compliance verification | **MANDATORY** | T057 to T083 |

- [x] Phase 0 (refactoring): skip. The builder change is a few lines around the existing chain and needs no preparatory refactoring.
- [x] Phase 2.7 (entity boilerplate): not a separate phase. The E2E tests of Phase 2f must compile before implementation (Principle VIII), and they read stored events through the storage adapter. T017 therefore adds the empty-but-compiling scaffold of the one new entity inside Phase 2f. Principle VIII permits minimal structure for this purpose. The entity has no HTTP handler.

Phase 2 must be complete before Phase 2.5 begins: glossary and non-functional constraints,
the committed configuration example, the Helm contract, migration files, standard amendments,
and all five E2E tests with semantic red evidence. These prerequisites are not yet complete.
T017 adds configuration types only. E2E fixtures supply all five values explicitly.
T035 and T037 establish configuration red evidence before T042 installs loader defaults and environment bindings.

## Testing Strategy

### End-to-End (E2E) Acceptance Tests

**Test Location**: `tests/e2e/business_event_ledger_test.go`

**Framework**: Ginkgo/Gomega BDD framework following patterns in [tests/e2e/README.md](../../tests/e2e/README.md)

**Test Organization**: `Describe("Business event ledger")` → `Context` per precondition (ledger
enabled, disabled, write failing; retention) → one `It` per scenario.

**Scenario Mapping**:

| Spec Scenario | E2E Test | Test Description | Red before implementation because |
|---|---|---|---|
| US1.1 | `business_event_ledger_test.go`, `Label("docker")` | `It("stores exactly one consent.grant.revoked event with principal, actor, trace ID, and resource IDs when a user revokes a grant")` | No row is stored |
| US1.2 | same file, `Label("docker")` | `It("keeps exactly one event when a second revocation returns 404")` | The row of the first revocation is absent |
| US1.3 | same file, `Label("docker")` | `It("stores no event and still logs the consent.grant.revoked audit record when the ledger is disabled")` | The audit record has `action`, not `event` |
| US1.4 | same file, `Label("docker")` | `It("returns 204, counts the failure, and logs one write-failed record when the ledger write fails")` | No failure is counted or logged |
| US2.1 | same file, `Label("docker")` | `It("deletes only the event older than the retention period without logging the deletion")` | The old row is still present |

Line numbers are added to this table by T021.

**Red Phase Requirements**: As in the plan template. No `XIt`, `PIt`, or `Skip()`. Assertions
target the stored event (`Event`, `Principal`, `Actor`, `TraceID`, attribute keys), the HTTP
status, the log buffer, and the metric reader.

**Test Data Strategy**:
- Existing fixtures for agents, grants, and principals from `tests/e2e/fixtures/`.
- New fixture: a configuration variant with `business_events.enabled: true`.
- US1.4 needs a ledger write that fails. The memory adapter cannot fail, and `storage.Adapter` is
  a concrete type, so the test runs in the container E2E suite against PostgreSQL. The E2E
  database user `testuser` is a superuser, so revoking a privilege has no effect. The test
  creates a `BEFORE INSERT` trigger on `business_events` whose function raises an exception, and
  drops the trigger and the function in `AfterEach`. No test-only production API is added. The
  handler's failure path is also covered by unit tests with a failing appender.
- US1.1 creates the grant through the API first. That audit record has no typed definition, so
  "exactly one row" also proves that a record without a definition is not stored (FR-001).
- US2.1 needs a row that is older than the retention period. The smallest valid period is 24
  hours and no production API sets the storage time, so the test runs against PostgreSQL: it
  revokes two grants, sets `recorded_at` of the first row to two days ago with SQL, and uses
  `retention.period: 24h` and `retention.interval: 1s`.
- T020 cannot turn green in Phase 2.5: its two revocations need the typed event from T049–T050.
  Retention unit and repository tests turn green in Phase 2.5. T056 owns the retention E2E green gate.

**Bootstrap Strategy**:
- All five scenarios use production bootstrap against PostgreSQL and inspect committed rows with
  explicit-column SQL in test helpers; no production reader port is introduced for assertions.
- US1.3 and US1.4 assert log output through the existing buffer-logger bootstrap path.
- These scenarios run in `just test-e2e-docker`. The memory adapter remains covered by its
  same-package repository tests and handler/consent tests with test-local append capture.

**Helper Utilities**:
- New matcher `HaveBusinessEvent(event, principal)` in `tests/e2e/matchers/`.
- Existing HTTP helpers.

### Frontend Playwright E2E Tests

Not applicable. No UI change.

### Unit & Integration Tests

**Unit Tests**:
- `internal/domain/bizevent/definition_test.go`: identifier validation, duplicate panic, `LogValue`, `Definitions()` order.
- `internal/domain/model/business_event_test.go`: `Validate`, `Copy`.
- `internal/adapters/storage/memory/business_event_test.go`: append, conflict, defensive copy, and retention delete, observed through adapter-private state in same-package tests.
- `internal/domain/bizevent/retention/service_test.go`: batching, cancellation, no deletion log, and failed-run logging. Controlled-time tests cover immediate startup, expiry just after a cutoff, scheduled deletion, and multi-batch recovery after failure or re-enable (T033; SC-005). Healthy runs start no more than one interval apart and complete before the next scheduled start. Recovery tests assert deletion of all rows older than the recovery run's start-time cutoff and preservation of newer rows.
- `internal/adapters/telemetry/ledger_handler_test.go`: recognition after principal policy, ignored records, attribute encoding (research R7), `WithAttrs`, `WithGroup`, level independence, cancelled context, timeout and failure metrics, fallback context attributes, no recursion, credential absence (SR-001), and identifier rendering in JSON and `otelslog` (FR-002). A non-identity policy must produce identical `principal`, `actor`, and `operator_principal` values in the captured row and operational sinks, including groups and `WithAttrs` (SR-002). Metric tests use the global meter provider with cleanup, not constructor injection.
- `internal/adapters/storage/postgres/business_event_test.go`: the two `storage.*` spans. FR-005 needs PostgreSQL and is covered by the integration test.
- `internal/adapters/telemetry/business_event_logging_guard_test.go`: a definition is logged only under `event`.
- `internal/domain/consent/` service tests, in the external test package `consent_test`: production ledger handler with a test-local capturing appender, one event per revocation, none on not-found, one with `refresh_cleanup=failed`. No reader port is needed.
- `internal/config/`: configuration defaults, environment bindings, and range validation (T035). T037 records semantic failures before T042 implements these behaviors.

**Integration Tests**:
- `tests/integration/migrations/`: migration `048` apply, rollback, repeat.
- `tests/integration/storage/infra/business_event_test.go`: PostgreSQL adapter against testcontainers, inspected with explicit-column SQL: `Append` with a committed transaction in context (research R4), JSONB round trip, two concurrent `DeleteRecordedBefore` calls that delete no row newer than the cutoff, both `CHECK` constraints, nil attributes stored as `{}`, and `Timeout` errors for an expired context deadline (Principle IX).
- `internal/app/builder_test.go`: chain with the flag on and off; start failure when the flag is on and no appender exists; the retention service runs with the flag on, stops on shutdown, and is not created with the flag off.

**Test Coverage Goals**:
- Unit: all branches of the handler and the definition type.
- Integration: both adapters for every port method.
- E2E: 100% of the acceptance scenarios in spec.md.
- Performance: the measurement below.

### SC-004 Measurement Protocol

`tests/e2e/business_event_ledger_performance_test.go`, `Label("performance")`, run with
`just test-e2e-performance`. It needs Docker and is not part of the default suites.

1. Start one PostgreSQL container with the production migrations. Start two brokers through the
   production bootstrap against it, in the same process: one with `business_events.enabled: false`
   and one with `true`. Both use the same pool settings and no telemetry export.
2. For each broker, 100 warm-up cycles, then 1,000 measured cycles. One cycle creates a grant for
   Alice and one agent through the API and then revokes it. Only the `DELETE` request is timed,
   from the send to the complete response, with a monotonic clock. Requests are sequential, one
   client, one connection.
3. Alternate the brokers in blocks of 100 cycles, so that drift of the host affects both alike.
4. For each broker, sort the 1,000 samples. The 99th percentile is the sample at index 989.
5. The test fails when `p99(on) - p99(off)` is above 5 ms. It reports both percentiles and both
   medians. The pull request description records them (T053).

## Complexity Tracking

| Violation | Why Needed | Simpler Alternative Rejected Because |
|-----------|------------|-------------------------------------|
| The ledger is off by default | It stores principals, and an operator must choose the retention period first | On by default would store personal data in deployments that have not planned retention |
