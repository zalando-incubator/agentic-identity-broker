# Implementation Plan: Business Event Ledger

**Branch**: `056-business-event-ledger` (one branch and pull request per delivery step) | **Date**: 2026-11-12 | **Spec**: [spec.md](./spec.md)

**Input**: Feature specification from `/specs/056-business-event-ledger/spec.md`

## Summary

The broker stores security-relevant facts (grant revoked, session established, approval denied)
in a ledger that the activity page and, later, an SSF transmitter read. A business event is the
audit log record that `coding-standards/LOGGING.md` already requires, with a typed
`bizevent.Definition` as its `event` value. A `slog.Handler` in the builder's logger chain stores
each such record as one row of the table `business_events`, after the state change has committed
([ADR 048](../../adrs/048-business-events-as-typed-log-records.md)). The table has seven promoted
columns and one JSONB attribute document, so a new event needs no migration. Each use case is
onboarded in its own pull request by changing one log call.

## Technical Context

**Language/Version**: Go (version from `go.mod`)
**Primary Dependencies**: `log/slog`, `go.opentelemetry.io/contrib/bridges/otelslog` (existing), `github.com/google/uuid` v1.6.0 (existing, `NewV7`), `github.com/jmoiron/sqlx` (existing). No new dependency.
**Storage**: PostgreSQL (one new table, migration `048`), memory adapter
**Testing**: Go `testing` with table-driven tests, Ginkgo/Gomega E2E (`tests/e2e/`), testcontainers for PostgreSQL integration tests
**Target Platform**: Linux server (broker Deployment). ExtProc is not changed.
**Project Type**: single (Go backend). No frontend change.
**Performance Goals**: The ledger write adds at most 5 ms at the 99th percentile to an onboarded request on PostgreSQL (SC-004)
**Constraints**: The write is outside the state change's transaction (ADR 048, Decision 5). A failed write never changes the result of the logging operation (FR-006). Disabled by default (FR-007).
**Scale/Scope**: Grant, session, and approval events: a few per user per day, about 4.5 million rows at 10,000 users and 90 days ([data-model.md](./data-model.md#volume-estimate))

## Approval Items

| ID | Item | Where | Status |
|---|---|---|---|
| ADR-048 | Business events are typed log records; the ledger row is written after the commit, with the loss window that Decision 5 accepts | [ADR 048](../../adrs/048-business-events-as-typed-log-records.md) | Decision 5 (after-commit write) and Decision 9 (immutable events) approved by jbspeakr on 2026-11-12. The ADR status stays Proposed until the maintainer accepts the ADR as a whole |
| STD-LOGGING-8 | Amendment of LOGGING.md section 8 (ledger as projection; no audit record inside a transaction) | Delivery step 1 | Proposed |
| STD-LOGGING-2.2 | New rule in LOGGING.md section 2.2 for the typed definition, and the guard test in section 9 | Delivery step 2 | Proposed |
| DB-048 | Table `business_events` and its two indexes | [data-model.md](./data-model.md) | Proposed |
| CFG-BE | Configuration keys `business_events.*` | [contracts/configuration.md](./contracts/configuration.md) | Proposed |

No approval is inferred. A delivery step starts only when the items it depends on are approved.

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

**Design Preconditions (BLOCKING)**:

- [x] **Domain Model**: Documented in [data-model.md](./data-model.md): Business Event Definition, Business Event, Business Event ID, ledger repository.
- [x] **Domain Concepts**: The four terms go to the `ARCHITECTURE.md` Glossary in steps 2 and 4 (tasks T023, T042).
- [x] **Entity IDs**: `id.BusinessEventID` is added through `gen_ids.go` and documented in `internal/domain/id/AGENTS.md` (T017). It is minted as UUIDv7 (research R6).
- [x] **Configuration Design**: [contracts/configuration.md](./contracts/configuration.md) lists every key with a YAML example.
- [x] **Config Examples**: `examples/config/business-events.yaml` and the README entry (T040).
- [x] **Helm Deployment Contract**: `broker.businessEvents.*` values, configmap template, schema, and README (T041, T070). No new workload.
- [x] **API Design First**: No API change. No endpoint is added or changed.
- [x] **API Documentation**: Not applicable. The end-user API and the Admin API are not changed.
- [x] **API Changes**: None.
- [x] **Database Design**: Migration `048_create_business_events` with up and down scripts (T025).
- [x] **E2E Acceptance Tests**: All seven acceptance scenarios are mapped below. Each E2E test is written first, in the pull request of the step that makes it pass.
- [x] **E2E Test Mapping**: One `It()` per scenario (table in Testing Strategy).
- [x] **E2E Red Phase**: Each step's pull request records the semantic failure of its new tests before the implementation commit (T015, T036, T048, T063, T074).
- [x] **Frontend Playwright E2E**: Not applicable. No UI change.
- [x] **Frontend Screenshots**: Not applicable.

**Implementation Considerations**:

- [x] **Security-First**: The ledger adds no data class beyond LOGGING.md section 6. It is off by default because it stores principals and must not run without retention (SR-003); this is a data-minimisation default, not a disabled security control.
- [x] **Architecture Docs**: `ARCHITECTURE.md` gets the ADR index row (step 1), the glossary entries (steps 2, 4), and a revised "Grant and Definition Lifecycle Audit" entry when the first grant event is onboarded (step 5).
- [x] **ADRs**: ADR 048.
- [x] **Library-First Security**: No cryptography.
- [x] **Zalando Guidelines**: Not applicable. No API change.
- [x] **End-User Docs**: `docs/configuration.md` and `docs/operations/business-events.md`.
- [x] **Migration Testing**: Apply, rollback, and repeat in `tests/integration/migrations/` (T026).
- [x] **Hexagonal Architecture**: Definition type in the domain, three small ports, two storage adapters, the handler as a telemetry adapter. Domain services do not import the handler.
- [x] **Persistence Patterns**: Follows `specs/004-persistence-layer/quickstart.md`: ISP ports in `internal/ports/storage.go`, both adapters, `sqlx`, `StorageError`, factory accessors.

**Principle XV (coding standards)**: Every new log record and instrument is listed in
[contracts/observability.md](./contracts/observability.md). LOGGING.md is amended in steps 1 and 2
(approval items above). `TestLoggedPrincipalsAreTyped` is not narrowed; a second guard test is added.

**Result**: No open gate besides the approval items. The code steps can proceed after ADR-048,
the two standard amendments, DB-048, and CFG-BE are approved.

## Project Structure

### Documentation (this feature)

```text
specs/056-business-event-ledger/
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
adrs/048-business-events-as-typed-log-records.md           # step 1
coding-standards/LOGGING.md                                 # steps 1, 2, 4
internal/domain/bizevent/
├── definition.go                                           # step 2
├── definition_test.go
├── retention.go                                            # step 6
└── retention_test.go
internal/domain/id/{gen_ids.go,uuid_ids_gen.go,AGENTS.md}   # step 2
internal/domain/model/business_event.go                     # step 2
internal/ports/storage.go                                   # steps 2, 6
internal/ports/config.go                                    # steps 4, 6
internal/adapters/storage/factory.go                        # steps 2, 6
internal/adapters/storage/memory/business_event.go          # steps 2, 6
internal/adapters/storage/postgres/business_event.go        # steps 3, 6
internal/adapters/telemetry/
├── ledger_handler.go                                       # step 4
├── ledger_handler_test.go
└── business_event_logging_guard_test.go                    # step 2
internal/app/builder.go                                     # steps 4, 6
internal/config/{loader.go,validator.go}                    # steps 4, 6
internal/domain/consent/{events.go,service.go}              # step 5 and later use cases
migrations/048_create_business_events.{up,down}.sql         # step 3
examples/config/business-events.yaml                        # steps 4, 6
charts/agentic-identity-broker/                             # steps 4, 6
docs/configuration.md, docs/operations/business-events.md   # steps 4, 6
tests/e2e/business_event_ledger_test.go                     # steps 5, 6
tests/integration/migrations/, tests/integration/storage/   # step 3
```

**Structure Decision**: Single Go project in the existing hexagonal layout. The only new package
is `internal/domain/bizevent`. Everything else extends existing packages.

## Delivery Steps

Each step is one pull request that leaves `main` releasable. The step number is the unit of
review; `tasks.md` groups tasks by step.

| Step | Pull request content | Production files (estimate) | Depends on | User story |
|---|---|---|---|---|
| 1 | ADR 048, this specification set, LOGGING.md section 8 | 0 (documents) | none | all |
| 2 | `bizevent`, `id.BusinessEventID`, model, appender and reader ports, memory adapter, LOGGING.md section 2.2 and guard test | 7 | 1 | US1, US2 |
| 3 | Migration `048`, PostgreSQL adapter | 3 | 2 | US1 |
| 4 | Ledger handler, configuration, builder wiring, instruments, example and chart | 6 | 2 | US1 |
| 5 | First use case `consent.grant.revoked`, E2E scenarios US1.1 to US1.4 and US2.2 | 2 | 3, 4 | US1, US2 |
| 6 | Retention port and service, operations document, E2E scenario US3.1 | 6 | 5 | US3 |
| 7 | `consent.grant.created` and `consent.grant.updated`, E2E scenario US2.1 | 2 | 5 | US2 |
| 8 and later | One use case each, by the onboarding checklist. Not tasks of this feature's task list; each is a small follow-up pull request that cites FR-012. | 2 to 3 | 5 | US2 |

Steps 3 and 4 can be reviewed in parallel. Step 6 can be reviewed in parallel with step 7.

## Implementation Phase Overview

| Phase | Purpose | Required? | Delivery step |
|-------|---------|-----------|---------------|
| **Phase 0** | Pre-implementation refactoring | Skipped | none |
| **Phase 1** | Setup: record approvals | Customizable | 1 |
| **Phase 2** | Design Preconditions | **MANDATORY** | 1 (documents), 2 to 6 (code-bound items, in the step that needs them) |
| **Phase 2.7** | Entity Boilerplate | Included, merged with step 2 | 2 |
| **Phase 2.5** | Foundational Infrastructure: PostgreSQL adapter, handler, wiring | Customizable | 3, 4 |
| **Phase 3+** | User Stories | Customizable | 5 (US1), 7 (US2), 6 (US3) |
| **Phase N** | Constitution Compliance verification | **MANDATORY** | each step, and a final pass after step 7 |

- [x] Phase 0 (refactoring): skip. The builder change in step 4 is three lines around the existing chain and needs no preparatory refactoring.
- [x] Phase 2.7 (entity boilerplate): include, as step 2. The entity has no HTTP handler in this feature, so the boilerplate is the model, the ports, and the memory adapter; it is small enough to ship with its behavior and tests in one pull request.

**Deviation from the standard phase order**: the template writes all E2E tests in Phase 2f before
any implementation. This feature merges in steps, and a failing E2E test cannot merge to `main`.
Each E2E test is therefore written first inside the pull request of the step that makes it pass,
and that pull request records the red run. Principle XIII's 1:1 mapping and red-green order hold
per step.

## Testing Strategy

### End-to-End (E2E) Acceptance Tests

**Test Location**: `tests/e2e/business_event_ledger_test.go`

**Framework**: Ginkgo/Gomega BDD framework following patterns in [tests/e2e/README.md](../../tests/e2e/README.md)

**Test Organization**: `Describe("Business event ledger")` → `Context` per precondition (ledger
enabled, disabled, write failing; retention) → one `It` per scenario.

**Scenario Mapping**:

| Spec Scenario | E2E Test | Test Description | Step |
|---|---|---|---|
| US1.1 | `business_event_ledger_test.go` | `It("stores one consent.grant.revoked event with principal, actor, trace ID, and resource IDs when a user revokes a grant")` | 5 |
| US1.2 | same file | `It("stores no event when the revocation returns 404")` | 5 |
| US1.3 | same file | `It("stores no event and still logs the audit record when the ledger is disabled")` | 5 |
| US1.4 | same file | `It("returns 204, counts the failure, and logs one write-failed record when the ledger write fails")` | 5 |
| US2.1 | same file | `It("stores a consent.grant.created event whose attributes equal the audit record's attributes")` | 7 |
| US2.2 | same file | `It("stores no event for an audit record with a string event identifier")` | 5 |
| US3.1 | same file | `It("deletes only events older than the retention period")` | 6 |

Line numbers are added to this table in the pull request that adds each test.

**Red Phase Requirements**: As in the plan template. No `XIt`, `PIt`, or `Skip()`. Assertions
target the stored event (`Event`, `Principal`, `Actor`, `TraceID`, attribute keys), the HTTP
status, the log buffer, and the metric reader.

**Test Data Strategy**:
- Existing fixtures for agents, grants, and principals from `tests/e2e/fixtures/`.
- New fixture: a configuration variant with `business_events.enabled: true`.
- US1.4 needs a ledger write that fails. The memory adapter cannot fail, and `storage.Adapter` is
  a concrete type, so the test runs in the container E2E suite against PostgreSQL and makes the
  insert fail in the database (for example by revoking `INSERT` on `business_events` from the
  broker role for the duration of the test). No test-only production API is added. The handler's
  failure path is also covered by unit tests with a failing appender.
- US2.2 uses an existing audit record with a string identifier (`session.oauth2.flow_initiated`).

**Bootstrap Strategy**:
- Production bootstrap through `tests/e2e/bootstrap/` with the memory adapter. The test reads
  stored events through `storage.Adapter.BusinessEventReader()`.
- US1.3 and US1.4 assert the log output through a buffer logger injected with `WithLogger`.
- US1.4 runs only in the container E2E suite (`just test-e2e-docker`). US1.1 and US3.1
  run on the memory adapter and again on PostgreSQL in the container suite.

**Helper Utilities**:
- New matcher `HaveBusinessEvent(event, principal)` in `tests/e2e/matchers/`.
- Existing HTTP helpers.

### Frontend Playwright E2E Tests

Not applicable. No UI change.

### Unit & Integration Tests

**Unit Tests**:
- `internal/domain/bizevent/definition_test.go`: identifier validation, duplicate panic, `LogValue`, `Definitions()` order.
- `internal/domain/model/business_event_test.go`: `Validate`, `Copy`.
- `internal/adapters/storage/memory/business_event_test.go`: append, conflict, list filters and order, deletes.
- `internal/adapters/telemetry/ledger_handler_test.go`: recognition, ignored records, attribute encoding (research R7), `WithAttrs`, `WithGroup`, level independence, cancelled context, timeout, failure record without principal, no recursion, credential absence (SR-001).
- `internal/adapters/telemetry/business_event_logging_guard_test.go`: a definition is logged only under `event`.
- `internal/domain/bizevent/retention_test.go`: batching, stop on context end, failure logging.
- `internal/domain/consent/` service tests: one event per revocation, none on not-found.

**Integration Tests**:
- `tests/integration/migrations/`: migration `048` apply, rollback, repeat.
- `tests/integration/storage/infra/business_event_test.go`: PostgreSQL adapter against testcontainers, including `Append` with a context that carries a committed transaction (research R4) and JSONB round trip.
- `internal/app/builder_test.go`: chain with the flag on and off; start failure when the flag is on and no appender exists.

**Test Coverage Goals**:
- Unit: all branches of the handler and the definition type.
- Integration: both adapters for every port method.
- E2E: 100% of the acceptance scenarios in spec.md.
- Performance: one benchmark of an onboarded request with the ledger on and off for SC-004, recorded in the step 5 pull request.

## Complexity Tracking

| Violation | Why Needed | Simpler Alternative Rejected Because |
|-----------|------------|-------------------------------------|
| E2E tests are not all written before the first implementation step | The feature merges as small pull requests; a red test cannot merge | One large pull request with all tests first is the shape of PR #381 that this feature replaces |
| The ledger is off by default | It stores principals and must not run without retention (SR-003) | On by default would store personal data in deployments that have not planned retention |
