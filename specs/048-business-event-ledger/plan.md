# Implementation Plan: Business Event Ledger

**Branch**: `048-business-event-ledger` | **Date**: 2026-09-26 | **Spec**: [specification](./spec.md)

**Input**: `specs/048-business-event-ledger/spec.md`, including the accepted crash-recovery clarification.

## Summary

Add a mandatory, credential-free ledger for all 28 broker-produced business facts. Generalize the existing storage transaction protocol.

On PostgreSQL, each state transition and its event commit together. Memory transactions provide real rollback/isolation.

Use immutable UUIDv7 event envelopes, a closed embedded JSON-schema registry, six-hour recorded-time partitions, and payload-free delivery references.

The background synchronous OpenTelemetry pipeline uses the existing destination and recovers after crashes. Lifecycle/subject barriers prevent post-deletion replay. A separate migration-owned scheduled Job owns partition DDL, not the runtime broker. Existing slog output and HTTP representations remain unchanged.

This plan records the Phase 0 research and Phase 1 design. It does not claim measured performance. [ADR 039](../../adrs/039-business-event-ledger.md) remains accepted and binding. [spec.md](spec.md) and this plan record the user-approved retirement of the performance gate and profile. The accepted storage/security design is unchanged, so no separate ADR is necessary. The [quickstart](quickstart.md) records implementation and validation evidence.

Scope covers only ledger behavior and its acceptance. It excludes unrelated dependency updates, documentation tooling, and advisory exceptions. Feature security verification covers the production broker runtime, not every repository manifest.

**Shared validation scope (user decision, 2026-10-06)**: The user removed feature-specific diagnostic tools, runner commands, and setup. No replacement performance suite is required. All 20 active functional scenarios remain required in the existing backend E2E and integration lanes. Historical measurements remain evidence only. Accepted ADR 039's storage/security design is unchanged.

**Future OTLP ingestion prerequisite (review, 2026-10-06)**: OTLP ingestion will add a second writer under the same lifecycle lock and use the same collector. Review findings M1, M2, and M9 are prerequisites for that work, not optional improvements. Their definitions and resolution evidence are absent from this repository. External ingestion remains outside Feature 048.

Before ingestion implementation, resolve those findings explicitly.

`just security broker` derives first-party package directories from the production import closure of `cmd/agentic-identity-broker`. Gosec scans that code, including schema, credential, encryption, storage and telemetry paths. Govulncheck analyzes the production entrypoint and its transitive vulnerable-symbol reachability. Relevant findings fail the gate. This scope loads no advisory ignore file. Repository-wide module and manifest audits remain available through `just security`.

## Technical Context

**Language/Version**: Go 1.27.1 from go.mod. No frontend changes.
**Primary Dependencies**: Existing sqlx 1.4.0, pgx/v5 5.11.0, google/uuid 1.6.0, Fosite 0.49.0, chi/v5 5.3.2, Cobra 1.10.2, Viper 1.21.0, OTel 1.46.0 and log/sdk-log 0.22.0. Promote existing google/jsonschema-go 0.4.2 from indirect to direct. Do not introduce a second schema validator.
**Storage**: PostgreSQL (existing PostgreSQL 17 deployment/test baseline) and memory. Six-hour UTC event/delivery partitions. Broker-owned business state remains in existing tables. Migration 036 follows the CIMD migrations 033–035 on main.
**Testing**: Standard Go tests/testify, Ginkgo 2.32.2/Gomega, shared testcontainers PostgreSQL, production app.Builder bootstrap, local OTLP receiver. No frontend Playwright work for this backend-only feature.
**Target Platform**: The existing Linux broker deployment on amd64/arm64 and local development on supported Go hosts. The Kubernetes chart plus an equivalent external PostgreSQL scheduler.
**Project Type**: The existing Go hexagonal backend within the monorepo, not a new service or event bus.
**Performance Goals**: The 5 ms p99 recording-overhead figure is unverified and non-blocking. Historical comparisons retain their measurement limits. No deployment profile, performance release gate, or replacement diagnostic suite is required. Collector work stays outside business commits.
**Constraints**: Mandatory fail-closed recording, 28-type coverage, same-transaction persistence, unchanged slog, and no credentials even in arbitrary strings. Stable event names, no post-erasure replay, no early retention deletion, and <=24h normal grace. No new HTTP/CLI read/erase surface and no runtime DDL.
**Scale/Scope**: 28 event types, 20 active acceptance scenarios, two storage backends. Default 90-day history corresponds to about 360 six-hour retained partitions plus 28 precreated future windows. Historical workload cardinalities remain documented. Repository size does not establish those cardinalities.

### Research closure

| Initial design unknown | Selected resolution |
|---|---|
| Shared transaction boundaries and memory rollback | Generalize the current context-carried manager/executor. Use joined scopes and a visibility gate across each memory transaction. |
| Event identity/namespace/schema validator | Named UUIDv7, fixed product functional name `agentic-identity-broker` per Zalando Rule 213. Existing google/jsonschema-go and embedded Draft 2020-12 contracts with URN schema identifiers. |
| Caller versus subject, missing span, unsafe context | Trusted domain event context. Caller from a verified peer. Subject from the affected identity. Only a matching span. Allowlisted context and fixed reasons. |
| Telemetry recovery/deletion race | An atomic delivery reference plus synchronous background export inside lifecycle/subject barriers. |
| Partition ownership/grace/operational erasure | Six-hour partitions, a five-minute migration-owned PostgreSQL Job, and an exact-subject SQL erasure function. |
| Performance scope | Retired release gate, deployment profile, and feature-specific diagnostic tools. Historical measurements remain, without a 5 ms pass claim. |

## Constitution Check

**Gate result before research**: PASS for planning. Scope identifies the domain model, required configuration, event contracts, database invariants, and 20 active scenarios. This plan selects no accepted-ADR exception. Implementation prerequisites are obligations. They are not claims that tests or migrations already exist.

**Gate result after design**: PASS for planning. The documents and schemas in Project Structure resolve the design unknowns and keep all 13 principles. Implementation remains gated on contract review, schema publication, architecture/configuration documentation, and semantic red acceptance evidence. This plan gives no blanket approval for public API changes.

### Design Preconditions (BLOCKING before implementation)

- [x] **Domain Model**: data-model.md defines Business Event, Event Type, Event Envelope, Actor Context, Delivery Reference and Retention Policy. The model documents relationships and invariants.
- [x] **Domain Concepts**: Update the ARCHITECTURE.md glossary in the feature implementation PR. Add the ledger transaction, retention, privacy and delivery design. Do not describe planned code as already deployed.
- [x] **Entity IDs**: Add generated id.BusinessEventID through internal/domain/id/gen_ids.go. Use UUIDv7 generation only for this type. Document the type in the ID context catalogue (ADR 013).
- [x] **Configuration Design**: contracts/configuration.md defines two keys, defaults, env/CLI/Helm mapping and YAML examples.
- [x] **Configuration Examples**: Before implementation completion, publish the reviewed snippets to examples/config/ and its index.
- [x] **Helm Deployment Contract**: Broker values/ConfigMap/README and the PostgreSQL-only maintenance Job are explicit. No ExtProc configuration enters the chart.
- [x] **API Design First**: The design includes 30 machine-readable event schemas and operational contracts. Before producer code, publish reviewed event schemas under api/events/v1. Before implementation, obtain agreement on those contracts.
- [x] **API Documentation**: There is no new HTTP API and no invented OpenAPI endpoint. During integration, verify existing admin/enduser OpenAPI failure representations. Obtain separate agreement for a discovered public behavior change.
- [x] **API Changes**: This plan preserves existing HTTP success/error forms and the spec-authorized fail-closed requirement. It does not implicitly approve an additional API change.
- [x] **Database Design**: The design specifies reversible numbered migrations, typed columns, indexes, partition ownership, privileges and rollback semantics.
- [x] **E2E Acceptance Tests**: The scenario mapping covers all 20 active scenarios. The retired US4-AS4 identifier is not reassigned.
- [x] **E2E Test Mapping**: One It block per scenario. Parameterized backend/workflow cases stay within their scenario and do not duplicate scenario identifiers.
- [x] **E2E Red Phase**: Compile the tests. Then verify that they fail on observable absent behavior. Use no skips, placeholder failures, or test-only production APIs.
- [x] **Frontend Playwright E2E**: Not applicable. There is no React change.
- [x] **Frontend Screenshots**: Not applicable. There is no changed UI surface.

These checked items mean that the plan contains a compliant design/commitment. They do not mean that implementation prerequisites ran.

### Implementation Considerations

| Principle | Compliance decision |
|---|---|
| I Security-first | Mandatory recording, typed trusted attribution, no raw credentials/errors, exact erasure, and no fail-open fallback |
| II Binding architecture/ADRs | Keep accepted ADR 002/004 storage/007/009 migration/011 telemetry/013 patterns. Accepted ADR 039 binds the ledger design. The specification and plan record the approved criteria change. ADR 033 remains Proposed. |
| III Library-first security | Existing crypto stays unchanged. Reuse UUID and schema libraries. No homemade cryptography or token decoding for invented identity. |
| IV OpenAPI transparency | Publish event schemas before producers. Preserve existing HTTP contracts. An extra change requires review. |
| V DDD | Business owners classify facts. The ledger enforces its own immutable-record invariants. Update the glossary. |
| VI Hexagonal architecture | Model -> ports <- adapters. Builder wiring. Extract the credential handler policy. No adapter-to-adapter imports. |
| VII Configuration | Common port/loader, two configuration controls, startup validation, Helm and examples, and one retention policy |
| VIII TDD | Unit and acceptance semantic red evidence before implementation. The smallest matching verification plus just check. |
| IX Persistence | ISP contracts in ports/storage.go, sqlx, two backends, and real PostgreSQL migration/rollback/concurrency coverage |
| X API-first | Before implementation, review the event/operational contract and a proposed public API change. |
| XI Design system | No UI changes. Not applicable. |
| XII Builder DI | Compose storage, registry, recorder, producers, recovery worker and shutdown in app/builder.go. |
| XIII E2E acceptance | Run 20 active journeys through production bootstrap. Run database-specific acceptance with PostgreSQL, not memory restarts. |

Event type names follow Zalando Rule 213. The remaining Zalando event guidance remains a review requirement (schema format, mandatory metadata, event category). This internal event envelope is not advertised as CloudEvents or a new REST resource.

Publish operational/event examples in docs/api/. Put compatibility and rollback guidance in the operations guide.

## Project Structure

### Documentation (this feature)

```text
specs/048-business-event-ledger/
  spec.md
  plan.md
  tasks.md
  research.md
  data-model.md
  quickstart.md
  performance-results.md            historical measurements and limits, not a release gate
  contracts/
    events.md
    storage.md
    configuration.md
    producers.md
    examples.json
    schemas/
      envelope.schema.json
      catalogue.schema.json
      <28 event-name>.schema.json
  checklists/requirements.md
adrs/039-business-event-ledger.md      accepted storage and telemetry design
```

### Source Code (planned changes, not yet generated)

```text
api/events/                         embedded reviewed schema publication (schemas.go, v1/)
internal/domain/id/                 generated BusinessEventID
internal/domain/model/              immutable shared business-event values
internal/domain/ledger/             registry, recorder, query/lifecycle invariants
internal/domain/consent/             grant transitions and lazy expiry
internal/domain/approval/            approval transitions and lazy expiry
internal/domain/agents/              agent transitions and dependent facts
internal/domain/oauth2/              accepted authorization and proxy outcome coordination
internal/domain/oauth2server/        local/Fosite issuance, credentials, signing selection
internal/domain/oauth2session/       establishment, refresh and termination
internal/domain/tokenexchange/       exchange result and refusal
internal/domain/impersonation/       permission decision versus mint result
internal/ports/storage.go            shared transactions and ledger ISP contracts
internal/ports/config.go             business-event settings
internal/adapters/storage/           factory, PostgreSQL and memory implementations
internal/adapters/telemetry/         separate synchronous ledger SDK pipeline
internal/adapters/http/              typed parse outcomes and staged proxy responses
internal/adapters/http/business_event_context.go  trusted request-context capture
internal/app/builder.go              recorder/worker composition and shutdown
internal/app/business_event_delivery.go   ledger telemetry copy delivery worker
internal/app/business_event_retention.go  in-memory logical retention worker
internal/config/                     defaults, bindings and validation
cmd/agentic-identity-broker/          dotted flags and lifecycle integration
migrations/036_business_event_ledger.{up,down}.sql
charts/agentic-identity-broker/       values, config, grants, maintenance Job, README
tests/e2e/                          20 mapped active acceptance journeys
tests/e2e/bootstrap/                builder wrappers, backend selection, child-process and fault-wrapped ports
tests/e2e/helpers/                  ledger inspection, operational SQL, historical fixtures, OTLP receiver
tests/e2e/matchers/                 envelope and credential-canary matchers
tests/e2e/fixtures/                 ledger workflow data and the legacy slog baseline
tests/integration/                  storage, migrations, Helm and lifecycle coverage
docs/ and examples/config/          operator/API/configuration updates
```

**Structure Decision**: Extend the existing broker.

The ledger is a bounded context, not a general-purpose event bus. Shared models prevent a ports/domain-service import cycle. The two background workers live in `internal/app/` because the builder owns their start, cancellation and shutdown order. They schedule work and call ports but hold no domain rules. This design adds no separate telemetry port, parallel configuration loader, external queue, or new HTTP service.

## Implementation Phase Overview

| Phase | Purpose | Required? |
|---|---|---|
| Phase 0 | A separate behavior-preserving refactor PR. Generalize transaction executor/ownership seams. Extract credential orchestration and the proxy completion seam. Preserve responses and old logs. | Required by breadth of structural change |
| Phase 1 | Setup. Promote the existing schema dependency. Prepare schema publication and typed ID generation. | Yes |
| Phase 2 | Design Preconditions. Domain/glossary, configuration/examples/Helm design, reviewed event/API contracts, migrations design, and 20 active red E2E scenarios. | MANDATORY |
| Phase 2.5 | Ledger foundations. Registry, real memory/PG transactions, immutable event append/get/scoped query, and payload-free delivery-reference insert. Lifecycle/subject barriers on append and migration-owned partition provisioning. | Yes |
| Phase 3 | US1 atomic catalogue recording. Implement every producer, expiration-recognition markers, no-op/competition/expiry and failure boundaries. | Yes, P1 |
| Phase 4 | US2 safe investigation. Trusted attribution, the schema-source extension seam, the full credential-exclusion matrix and investigation documentation. | Yes, P1 |
| Phase 5 | US3 erasure/retention. Erasure function, retention drops, scheduler, privileges and operations roles, backend parity, and no resurrection. | Yes, P2 |
| Phase 6 | US4 telemetry dispatch, recovery, and compatibility | Yes, P2 |
| Phase N | Constitution Compliance verification, documentation/deployment completion and the full final gate | MANDATORY |

Phase 0 contains no half-working ledger or API stubs. Existing tests must continue to pass. The refactor starts with capture of the legacy slog baseline that proves unchanged logs.

Phase 2f can declare the compile-only contract surface that acceptance tests call (types, port signatures, inert accessors, the schema-source seam). It cannot declare working ledger behavior. Foundations deliver real memory rollback and ledger behavior with their red/green tests. Behavior-preserving refactoring does not silently claim these behaviors.

Each ledger operation has exactly one implementing task after its tests. Foundations own append/get/query and pending insert. US3 owns erasure, retention drops, policy application and privileges. US4 owns dispatch.

Skip Phase 2.7.

This feature requires no empty CRUD handlers or separately shipped scaffold. No new public CRUD interface exists.

## Testing Strategy

### End-to-End Acceptance Tests

The framework uses existing Ginkgo/Gomega and app.Builder bootstrap. Full business journeys use the real admin/end-user HTTP servers. All listed locations are planned. This plan invents no test line numbers.

After those workflows, query via authorized operational access. Erase via authorized operational access.

| Spec Scenario | Planned E2E location | Test intent |
|---|---|---|
| US1-AS1 | `tests/e2e/business_event_ledger_test.go` | Catalogue coverage |
| US1-AS2 | `tests/e2e/business_event_ledger_postgres_test.go` | Crash after commit |
| US1-AS3 | `tests/e2e/business_event_ledger_test.go` | Rollback |
| US1-AS4 | `tests/e2e/business_event_ledger_test.go` | Decision without mutation |
| US1-AS5 | `tests/e2e/business_event_ledger_test.go` | Competing transitions |
| US1-AS6 | `tests/e2e/business_event_ledger_test.go` | Expiration |
| US1-AS7 | `tests/e2e/business_event_ledger_postgres_test.go` | Storage parity |
| US2-AS1 | `tests/e2e/business_event_ledger_test.go` | Scoped investigation |
| US2-AS2 | `tests/e2e/business_event_ledger_test.go` | Attribution and correlation |
| US2-AS3 | `tests/e2e/business_event_ledger_test.go` | Credential exclusion |
| US2-AS4 | `tests/e2e/business_event_ledger_test.go` | Unavailable identities |
| US2-AS5 | `tests/e2e/business_event_ledger_test.go` | Stable extensible contract |
| US3-AS1 | `tests/e2e/business_event_ledger_test.go` | Principal erasure |
| US3-AS2 | `tests/e2e/business_event_ledger_postgres_test.go` | Automatic retention |
| US3-AS3 | `tests/e2e/business_event_ledger_postgres_test.go` | Erasure during recording |
| US3-AS4 | `tests/e2e/business_event_ledger_postgres_test.go` | No resurrection |
| US3-AS5 | `tests/e2e/business_event_ledger_test.go` | Invalid retention configuration |
| US4-AS1 | `tests/e2e/business_event_ledger_test.go` | Compatible additional signal |
| US4-AS2 | `tests/e2e/business_event_ledger_test.go` | Independent disablement |
| US4-AS3 | `tests/e2e/business_event_ledger_postgres_test.go` | Export failure, rollback, and crash recovery |

US4-AS4 is a retired identifier, not an active E2E acceptance scenario. The user removed feature-specific recording and delivery diagnostics. All 20 functional scenarios remain required.

US1-AS1 runs all 28 business journeys inside its catalogue scenario. It verifies exact type, subject, actor, references, outcome and occurrence count. US2-AS3 injects distinct credential canaries into each applicable input/failure string for every type. It inspects the entire stored/OTLP envelope, not just token-named fields.

Existing legacy log compatibility is a user-mandated contract. Phase 0 captures the pre-feature slog baseline (event name, level, message and field keys per catalogued workflow) in `tests/e2e/fixtures/business_event_ledger_legacy_slog.go`. US4-AS1 compares against that fixture, not arbitrary source text.

US1-AS2 and US4-AS3 use a PostgreSQL-backed child process and controlled interruption after commit/before output or export. Memory cannot demonstrate restart durability. A committed ledger row observed from another connection is the commit oracle. The crash helper runs production builder logic. Scenario-specific helpers prevent a timing race from replacing a deterministic boundary.

Put child-process coordination and fault-injecting port wrappers in `tests/e2e/bootstrap/`. Put ledger inspection, operational SQL and OTLP capture in `tests/e2e/helpers/`. Put envelope and credential-canary matchers in `tests/e2e/matchers/`. Never add production debug endpoints or test-only APIs.

Use integration-tagged PostgreSQL scenarios in `tests/e2e/business_event_ledger_postgres_test.go`. When the lane is required, use no Skip.

All ledger specs carry `business-event-ledger`. `just test-e2e-backend 'business-event-ledger && !performance'` runs shared and PostgreSQL-specific scenarios with the default `integration` tag. Normal `just verify` includes this coverage through the existing backend lane, without a duplicate ledger run. CI runs the full integration-tagged backend suite through `just verify-e2e-backend-junit`. Both commands retain configured backend parallelism, existing `Serial` declarations, and backend reports.

The ledger is part of the broker, not a standalone CLI. This feature adds no ledger-specific CI job or step and no CI security gate. Existing scheduled security scans on main remain unchanged.

Keep one It per active specification scenario. Keep backend variants inside it.

Scenarios in `business_event_ledger_test.go` iterate the backends from a build-tag-selected bootstrap helper. `tests/e2e/bootstrap/business_event_ledger_backends.go` (`//go:build !integration`) returns memory only. `tests/e2e/bootstrap/business_event_ledger_backends_integration.go` (`//go:build integration`) returns memory and PostgreSQL. The existing `test-e2e-backend` recipe uses `integration` by default. Run `just test-e2e-backend 'business-event-ledger && !performance' ''` for memory only. The default tagged lane runs every shared scenario against both backends.

Each backend iteration inside an It bootstraps its own server and storage (a fresh memory adapter or a fresh template-database clone). It releases them through `DeferCleanup`. Iterations share no state. This satisfies the fresh-server-and-storage isolation in Principle XIII and keeps one It per scenario. US1-AS7 compares both backends inside one It, so it lives in the tagged file.

SC-008 evidence has two sources. The tagged run of the shared scenarios plus US1-AS7 provides evidence for event contents, query results and erasure results. Logical retention, erasure during recording and non-resurrection (US3-AS2/AS3/AS4) require database-clock history seeding or a process restart. Thus, those acceptance scenarios run only on PostgreSQL.

The memory lifecycle and retention-worker tests (T072, T074) provide memory equivalence for those three scenarios. These tests drive the adapter clock from inside the memory package. T083 records both sources.

The [performance report](performance-results.md) preserves historical recording and delivery measurements, their conditions, and their limits. The diagnostic suite, shared scenario runner, and performance bootstrap are removed. No replacement suite or setup is required. These changes neither establish 5 ms p99 nor reduce functional acceptance.

PostgreSQL maintenance reads the database clock. The database assigns immutable `recorded_at`, so retention scenarios seed history instead of a time change. A helper with migration-owner credentials calls the production `public.business_event_create_partition_pair` function for past six-hour windows. It inserts envelopes validated through the production registry with explicit historical `recorded_at`. Then it runs the real maintenance function. Live HTTP workflows supply the young side of each boundary.

Memory lifecycle unit tests control the adapter clock from inside the tests in the memory package.

Do not wait 90 days. Do not add time-travel APIs to production.

### Fixtures, bootstrap and red phase

Extend existing tests/e2e/fixtures with isolated principals, agents, permission sets, grants, sessions, approvals, credential IDs, safe expected envelopes and credential canaries. Reuse mock upstream/JWKS fixtures and local OTLP receivers. Use the existing shared PostgreSQL bootstrap/template-database clones for each isolated scenario. Do not use production secrets or live identity providers.

Write tests first. Make them compile. Then record semantic failures for the missing event/atomicity/recovery behavior. Do not use unconditional always-fail assertions or skipped/pending tests. Do not use assertions that merely verify a mock call or source string.

The plan expects only fixture changes after implementation. Changes to expected behavior require spec review.

### Unit and Integration Tests

Coverage:

- Domain: Type/outcome/reference validation, exact subject and caller attribution, failure classification precedence, safe context normalization, valid/invalid per-type schemas, and expiry marker transitions.
- Memory: Uncommitted invisibility, rollback restoration of touched records/indexes, concurrent readers/writers, nested ownership, and post-commit notification. Event append/get/scoped query and pending insert, dispatch claims, logical retention/erasure, and pending recovery during the process lifetime.
- Expiration recognition: Compare-and-set of `expiration_recorded_for` on grants and approvals in both backends. One winner under concurrency. Recognition starts again only for a changed effective expiry.
- PostgreSQL: Ambient transaction participation of each catalogued repository, row/CAS winners, independent failure commits, and Fosite failure-side revocation preservation. Subject phantom-insert barrier, synchronous emission/erase ordering, paired partition drop, permission denial, migration up/down/up, and business-data survival.
- Telemetry: EventName and the full flat envelope, including null/absent/empty distinction. Dropped-attribute protection. No acknowledgement of callback failure. Retry with the same ID. Collector outage and cancellation. No delayed SDK payload after deletion.
- Deployment: Real configuration precedence, false boolean preservation, positive sub-microsecond normalization, invalid configuration startup failure, and rendered maintenance credentials/role separation. Optional operational reader/erasure role grants and memory-chart omission.
- Coverage goals: All 28 facts, all 20 active scenarios, and all listed security/atomicity transitions. No invented blanket percentage substitutes for these guarantees.

Do not add tests that only compare copied configuration defaults.

### Verification and acceptance evidence

During implementation, run `just check`, affected package tests, and the matching shared backend/integration commands. Run `just security broker` for focused runtime security. Then run normal `just verify`.

The shared commands cover Go/race tests, infrastructure integration, and integration-tagged backend acceptance. Normal `just verify` also includes the existing Helm lint/template checks. Operational schedule evidence remains required. No feature-specific verification or performance suite is required.

Planning validation verifies JSON-schema resolution and representative positive/negative examples with the pinned Go validator. It also verifies reference integrity, full 28-type/20-active-scenario coverage, and removal of template placeholders. This planning command claims no ledger runtime or database tests.

## Complexity Tracking

This plan proposes no constitutional violation. The transaction refactor, delivery references and deletion barriers are necessary for FR-001/004/006/007/017. Simpler best-effort logging and snapshot-only deletion fail named acceptance requirements. The existing migration privilege boundary requires a separate maintenance Job. This Job is not a new deployment architecture exception. Contract review and executable red tests remain implementation gates, not unresolved design choices.
