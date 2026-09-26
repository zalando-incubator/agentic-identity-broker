# Implementation Plan: Business Event Ledger

**Branch**: `048-business-event-ledger` | **Date**: 2026-09-26 | **Spec**: [specification](./spec.md)

**Input**: `specs/048-business-event-ledger/spec.md`, including the accepted crash-recovery clarification.

## Summary

Add a mandatory, credential-free ledger for all 28 broker-produced business facts. Generalize the existing storage transaction protocol so each state transition and its event commit together on PostgreSQL and have real rollback/isolation in memory. Use immutable UUIDv7 event envelopes, a closed embedded JSON-schema registry, six-hour recorded-time partitions, and payload-free pending-delivery references. A background synchronous OpenTelemetry pipeline uses the existing destination and recovers after crashes; lifecycle/subject barriers prevent post-deletion replay. Partition DDL belongs to a separate migration-owned scheduled Job, not the runtime broker. Existing slog output and HTTP representations remain unchanged.

This plan provides the complete Phase 0 research and Phase 1 design, not production implementation, migrations, acceptance tests or measured performance. [ADR 037](../../adrs/037-business-event-ledger.md) is Proposed, not self-approved.

## Technical Context

**Language/Version**: Go 1.27.1 from go.mod; no frontend changes.
**Primary Dependencies**: Existing sqlx 1.4.0, pgx/v5 5.11.0, google/uuid 1.6.0, Fosite 0.49.0, chi/v5 5.3.2, Cobra 1.10.2, Viper 1.21.0, OTel 1.46.0 and log/sdk-log 0.22.0. Promote existing google/jsonschema-go 0.4.2 from indirect to direct; do not introduce a second schema validator.
**Storage**: PostgreSQL (existing PostgreSQL 17 deployment/test baseline) and memory. Six-hour UTC event/delivery partitions; broker-owned business state remains in existing tables. Plan migration 035 because the unmerged `046-cimd-upstream-client` branch holds 033 and 034; recheck allocation before implementation.
**Testing**: Standard Go tests/testify, Ginkgo 2.32.2/Gomega, shared testcontainers PostgreSQL, production app.Builder bootstrap, local OTLP receiver. No frontend Playwright work for this backend-only feature.
**Target Platform**: Existing Linux broker deployment on amd64/arm64, local development on supported Go hosts; Kubernetes chart plus equivalent external PostgreSQL scheduler.
**Project Type**: Existing Go hexagonal backend within the monorepo, not a new service or event bus.
**Performance Goals**: Added recording transaction p99 <=5 ms at documented deployment load. Reference and deployment-profile baseline methodology is in quickstart.md; no measurement result is asserted here. Collector work stays outside business commits.
**Constraints**: Mandatory fail-closed recording, 28-type coverage, same-transaction persistence, unchanged slog, no credentials even in arbitrary strings, stable event names, no post-erasure replay, no early retention deletion, <=24h normal grace, no new HTTP/CLI read/erase surface, no runtime DDL.
**Scale/Scope**: 28 event types, 21 acceptance scenarios, two storage backends. Default 90-day history corresponds to about 360 six-hour retained partitions plus 28 precreated future windows. Workload cardinalities are measured during acceptance, not guessed from repository size.

### Research closure

| Initial design unknown | Selected resolution |
|---|---|
| Shared transaction boundaries and memory rollback | Generalize the current context-carried manager/executor; joined scopes and transaction-wide memory visibility gate |
| Event identity/namespace/schema validator | Named UUIDv7, fixed product functional name `agentic-identity-broker` per Zalando Rule 213, existing google/jsonschema-go and embedded Draft 2020-12 contracts with URN schema identifiers |
| Caller versus subject, missing span, unsafe context | Trusted domain event context; caller from verified peer, subject from affected identity; matching span only; allowlisted context and fixed reasons |
| Telemetry recovery/deletion race | Atomic pending reference plus synchronous background export held inside lifecycle/subject barriers |
| Partition ownership/grace/operational erasure | Six-hour partitions, five-minute migration-owned PostgreSQL Job, exact-subject SQL erasure function |
| Current-load acceptance evidence | Explicit paired baseline/enabled procedure, reference profile and deployment-profile capture before release |

## Constitution Check

**Gate result before research**: PASS for planning. Scope already identifies the domain model, required configuration, event contracts, database invariants and 21 scenarios. No accepted-ADR exception is selected. Implementation prerequisites are obligations, not claims that tests or migrations already exist.

**Gate result after design**: PASS for planning. The documents and schemas below resolve the design unknowns and keep all 13 principles. Implementation remains gated on contract/ADR review, schema publication, architecture/configuration documentation and semantic red acceptance evidence. There is no blanket public-API-change approval.

### Design Preconditions (BLOCKING before implementation)

- [x] **Domain Model**: Business Event, Event Type, Event Envelope, Actor Context, Delivery Reference and Retention Policy defined in data-model.md; relationships and invariants documented.
- [x] **Domain Concepts**: Update ARCHITECTURE.md glossary and add ledger transaction, retention, privacy and delivery design in the feature implementation PR; do not describe planned code as already deployed.
- [x] **Entity IDs**: Add generated id.BusinessEventID through internal/domain/id/gen_ids.go with UUIDv7 generation only for this type; document the type in the ID context catalogue (ADR 013).
- [x] **Configuration Design**: Two keys, defaults, env/CLI/Helm mapping and YAML examples defined in contracts/configuration.md.
- [x] **Config Examples**: Publish the reviewed snippets to examples/config/ and its index before implementation completion.
- [x] **Helm Deployment Contract**: Broker values/ConfigMap/README and PostgreSQL-only maintenance Job are explicit; no ExtProc settings enter the chart.
- [x] **API Design First**: 30 machine-readable event schemas and operational contracts designed. Publish reviewed event schemas under api/events/v1 before producer code. Confirm those contracts before implementation.
- [x] **API Documentation**: No new HTTP API, so no invented OpenAPI endpoint. Check existing admin/enduser OpenAPI failure representations during integration; separately confirm any discovered public behavior change.
- [x] **API Changes**: This plan preserves existing HTTP success/error forms and the spec-authorized fail-closed requirement; no additional API change is implicitly approved.
- [x] **Database Design**: Reversible numbered migrations, typed columns, indexes, partition ownership, privileges and rollback semantics specified.
- [x] **E2E Acceptance Tests**: All 21 scenarios mapped below; write them before feature production code.
- [x] **E2E Test Mapping**: One It block per scenario; parameterized backend/workflow cases live within its scenario, not duplicated scenario identifiers.
- [x] **E2E Red Phase**: Compile then fail on observable absent behavior; no skips, placeholder failures, or test-only production APIs.
- [x] **Frontend Playwright E2E**: Not applicable: no React change.
- [x] **Frontend Screenshots**: Not applicable: no changed UI surface.

The checked items above mean the plan contains a compliant design/commitment. They do not mark implementation prerequisites as executed.

### Implementation Considerations

| Principle | Compliance decision |
|---|---|
| I Security-first | Mandatory recording; typed trusted attribution; no raw credentials/errors; exact erasure; no fail-open fallback |
| II Binding architecture/ADRs | Retain accepted ADR 002/004 storage/007/009 migration/011 telemetry/013 patterns; proposed ADR 037 documents new design; ADR 033 stays Proposed |
| III Library-first security | Existing crypto untouched; UUID and schema libraries reused; no homemade cryptography or token decoding to invent identity |
| IV OpenAPI transparency | Event schemas published before producers; existing HTTP contracts preserved and any extra change requires review |
| V DDD | Business owners classify facts; ledger enforces its own immutable-record invariants; glossary updated |
| VI Hexagonal architecture | Model -> ports <- adapters; builder wiring; credential handler policy extracted; no adapter-to-adapter imports |
| VII Configuration | Common port/loader, two settings, startup validation, Helm and examples, one retention policy |
| VIII TDD | Unit and acceptance semantic red evidence before implementation; smallest matching verification plus just check |
| IX Persistence | ISP contracts in ports/storage.go, sqlx, two backends, real PostgreSQL migration/rollback/concurrency coverage |
| X API-first | Review event/operational contract and any public API delta before producing implementation |
| XI Design system | No UI changes; not applicable |
| XII Builder DI | Storage, registry, recorder, producers, recovery worker and shutdown composed in app/builder.go |
| XIII E2E acceptance | 21 complete journeys through production bootstrap; database-specific acceptance runs use PostgreSQL, not memory restarts |

Event type names follow Zalando Rule 213; the remaining Zalando event guidance (schema format, mandatory metadata, event category) remains a review requirement; this internal event envelope is not advertised as CloudEvents or a new REST resource. Publish operational/event examples in docs/api/; compatibility and rollback guidance belongs in the operations guide.

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
adrs/037-business-event-ledger.md
```

### Source Code (planned changes, not generated now)

```text
api/events/                         embedded reviewed schema publication
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
internal/domain/thirdparty/          dependent session deletion orchestration
internal/ports/storage.go            shared transactions and ledger ISP contracts
internal/ports/config.go             business-event settings
internal/adapters/storage/           factory, PostgreSQL and memory implementations
internal/adapters/telemetry/         separate synchronous ledger SDK pipeline
internal/adapters/http/              typed parse outcomes and staged proxy responses
internal/app/builder.go              recorder/worker composition and shutdown
internal/config/                     defaults, bindings and validation
cmd/agentic-identity-broker/          dotted flags and lifecycle integration
migrations/035_business_event_ledger.{up,down}.sql
charts/agentic-identity-broker/       values, config, grants, maintenance Job, README
tests/e2e/                          21 mapped acceptance journeys
tests/e2e/bootstrap/                builder wrappers, backend selection, child-process and fault-wrapped ports
tests/e2e/helpers/                  ledger inspection, operational SQL, historical fixtures, OTLP receiver
tests/e2e/matchers/                 envelope and credential-canary matchers
tests/e2e/fixtures/                 ledger workflow data and the legacy slog baseline
tests/integration/                  storage, migrations, Helm and lifecycle coverage
docs/ and examples/config/          operator/API/configuration updates
```

**Structure Decision**: Extend the existing broker; ledger is a bounded context, not a catch-all event bus. Shared models avoid a ports/domain-service import cycle. No separate telemetry port, parallel config loader, external queue, or new HTTP service.

## Implementation Phase Overview

| Phase | Purpose | Required? |
|---|---|---|
| Phase 0 | Separate behavior-preserving refactor PR: generalize transaction executor/ownership seams, extract credential orchestration and proxy completion seam without changing responses or old logs | Required by breadth of structural change |
| Phase 1 | Setup: promote existing schema dependency, prepare schema publication and typed ID generation | Yes |
| Phase 2 | Design Preconditions: domain/glossary, config/examples/Helm design, reviewed event/API contracts, migrations design, 21 red E2E scenarios | MANDATORY |
| Phase 2.5 | Ledger foundations: registry, real memory/PG transactions, immutable event append/get/scoped query, payload-free pending-reference insert, lifecycle/subject barriers on append and migration-owned partition provisioning | Yes |
| Phase 3 | US1 atomic catalogue recording: implement every producer, expiration-recognition markers, no-op/competition/expiry and failure boundaries | Yes, P1 |
| Phase 4 | US2 safe investigation: trust attribution, schema-source extension seam, full credential-exclusion matrix and investigation documentation | Yes, P1 |
| Phase 5 | US3 erasure/retention: erasure function, retention drops, scheduler, privileges and operations roles, backend parity and no resurrection | Yes, P2 |
| Phase 6 | US4 telemetry dispatch/recovery/compatibility and measured p99 overhead | Yes, P2 |
| Phase N | Constitution Compliance verification, documentation/deployment completion and full final gate | MANDATORY |

Phase 0 contains no half-working ledger or API stubs and must keep the existing tests passing; it starts by capturing the legacy slog baseline that proves unchanged logs. Phase 2f may declare the compile-only contract surface the acceptance tests call (types, port signatures, inert accessors, the schema-source seam), never working ledger behavior. Real memory rollback and ledger behavior are delivered with their red/green tests in foundations, not silently asserted as behavior-preserving refactoring. Each ledger operation has exactly one implementing task that follows its tests: foundations own append/get/query and pending insert; US3 owns erasure, retention drops, policy application and privileges; US4 owns dispatch. Skip Phase 2.7: this feature does not need empty CRUD handlers or a separately shipped scaffold. No new public CRUD interface exists.

## Testing Strategy

### End-to-End Acceptance Tests

Framework: existing Ginkgo/Gomega and app.Builder bootstrap. Complete business journeys use the real admin/end-user HTTP servers; query and erase via authorized operational access after those workflows. All listed locations are planned; no test line numbers are invented.

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
| US4-AS4 | `tests/e2e/business_event_ledger_performance_test.go` | Performance |

US1-AS1 loops through all 28 business journeys inside its catalogue scenario, asserting exact type, subject, actor, references, outcome and occurrence count. US2-AS3 injects distinct credential canaries into each applicable input/failure string for every type and inspects the entire stored/OTLP envelope, not just token-named fields. Existing legacy log compatibility is a user-mandated contract. Phase 0 captures the pre-feature slog baseline (event name, level, message and field keys per catalogued workflow) into `tests/e2e/fixtures/business_event_ledger_legacy_slog.go`, and US4-AS1 compares against that fixture, not arbitrary source text.

US1-AS2 and US4-AS3 use a PostgreSQL-backed child process and controlled interruption after commit/before output or export; memory cannot demonstrate restart durability. Put child-process coordination and fault-injecting port wrappers in `tests/e2e/bootstrap/`, ledger inspection, operational SQL and OTLP capture in `tests/e2e/helpers/`, and envelope and credential-canary matchers in `tests/e2e/matchers/`; never add production debug endpoints or test-only APIs. A committed ledger row observed from another connection is the commit oracle. The crash helper runs production builder logic. Scenario-specific helpers prevent a timing race from substituting for a deterministic boundary.

Use integration-tagged PostgreSQL scenarios in tests/e2e/business_event_ledger_postgres_test.go, executed with Ginkgo's integration tag; no Skip when the acceptance lane is required. Add a dedicated just recipe and CI lane so the normal memory-only E2E command does not silently count these scenarios as run. Keep one It per spec scenario, with PG assertion variants inside the relevant scenario. Other PG-specific invariants remain focused integration tests, not duplicate E2E identifiers.

Scenarios in `business_event_ledger_test.go` iterate the backends returned by a build-tag-selected bootstrap helper: `tests/e2e/bootstrap/business_event_ledger_backends.go` (`//go:build !integration`) returns memory only, and `tests/e2e/bootstrap/business_event_ledger_backends_integration.go` (`//go:build integration`) returns memory and PostgreSQL. The default lane therefore proves memory behavior, and the tagged lane runs every shared scenario against both backends; that tagged run is the SC-008 backend-consistency evidence. US1-AS7 compares both backends inside one It, so it lives in the tagged file.

US4-AS4 is performance-labelled and runs alone with one Ginkgo process; repeatable profile/baseline instructions are in quickstart.md. Besides the latency thresholds it asserts that the feature run retained exactly one event of the expected type for every completed catalogued action. It therefore fails semantically until recording exists, instead of passing because the feature run equals the baseline.

PostgreSQL maintenance reads the database clock, and `recorded_at` is database-assigned and immutable, so retention scenarios seed history rather than moving time. A helper connected with migration-owner credentials calls the production `public.business_event_create_partition_pair` function for past six-hour windows, inserts envelopes validated through the production registry with explicit historical `recorded_at`, and then runs the real maintenance function. Live HTTP workflows supply the young side of each boundary. Memory lifecycle unit tests control the adapter clock from inside the memory package's tests. Do not wait 90 days or add time-travel APIs to production.

### Fixtures, bootstrap and red phase

Extend existing tests/e2e/fixtures with isolated principals, agents, permission sets, grants, sessions, approvals, credential IDs, safe expected envelopes and credential canaries. Reuse mock upstream/JWKS fixtures and local OTLP receivers. Use the existing shared PostgreSQL bootstrap/template-database clones for each isolated scenario. No production secrets or live identity providers.

Write tests first, make them compile, then record semantic failures for the missing event/atomicity/recovery behavior. No unconditional always-fail assertions, no skipped/pending tests, no assertion that merely checks a mock call or source string. After implementation only fixture changes should be needed; changes to expected behavior require spec review.

### Unit and Integration Tests

- Domain: type/outcome/reference validation, exact subject and caller attribution, failure classification precedence, safe context normalization, valid/invalid per-type schemas, expiry marker transitions.
- Memory: uncommitted invisibility, rollback restoration of touched records/indexes, concurrent readers/writers, nested ownership, post-commit notification, event append/get/scoped query and pending insert, dispatch claims, logical retention/erasure and pending recovery while process lives.
- Expiration recognition: compare-and-set of `expiration_recorded_for` on grants and approvals in both backends, one winner under concurrency, reopening only for a changed effective expiry.
- PostgreSQL: ambient transaction participation of each catalogued repository, row/CAS winners, independent failure commits, Fosite failure-side revocation preservation, subject phantom-insert barrier, synchronous emission/erase ordering, paired partition drop, permission denial, migration up/down/up and business-data survival.
- Telemetry: EventName and complete flat envelope including null/absent/empty distinction; dropped-attribute protection; callback failure is not acknowledged; retry with same ID; collector outage and cancellation; no delayed SDK payload after deletion.
- Deployment: real config precedence, false boolean preservation, positive sub-microsecond normalization, invalid configuration startup failure, rendered maintenance credentials/role separation, optional operational reader/erasure role grants and memory-chart omission. Do not add tests that only compare copied config defaults.
- Coverage goals: all 28 facts, all 21 scenarios and all listed security/atomicity transitions. No invented blanket percentage substitutes for these guarantees.

### Verification and acceptance evidence

Implementation loop: just check, smallest affected Go/package tests, matching memory E2E and PostgreSQL integration/E2E lanes, then just verify as the final gate. Render/lint Helm after deployment changes. Actual current-load measurement and operational schedule evidence are required before release; a green schema smoke is not ledger runtime proof.

Planning validation checks JSON-schema resolution and representative positive/negative examples using the pinned Go validator, reference integrity, full 28-type/21-scenario coverage, and removal of template placeholders. No ledger runtime or database tests are claimed by this planning command.

## Complexity Tracking

No constitutional violation is proposed. The transaction refactor, pending references and deletion barriers are necessary for FR-001/004/006/007/017; simpler best-effort logging and snapshot-only deletion fail named acceptance requirements. A separate maintenance Job is required by the existing migration privilege boundary, not a new deployment architecture exception. Contract/ADR acceptance and executable red tests remain implementation gates, not unresolved design choices.
