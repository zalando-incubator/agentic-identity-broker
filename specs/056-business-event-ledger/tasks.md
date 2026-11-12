# Tasks: Business Event Ledger

**Input**: Design documents from `specs/056-business-event-ledger/`.
**Prerequisites**: `spec.md`, `plan.md`, `research.md`, `data-model.md`, `contracts/`, `quickstart.md`.
**Status**: Delivery step 1 complete in its pull request, pending approval. No code task has started.
**Tests**: Mandatory red-green-refactor under Principles VIII and XIII. In each step, write the tests first, record their semantic failure, then implement.
**Organization**: Tasks are grouped by delivery step. One delivery step is one pull request ([plan.md](./plan.md#delivery-steps)). The template phase of each group is named in its heading. Paths are relative to the repository root. `[P]` means different files and no dependency on an open task in the same step.

## 🔒 Feature Scope [MANDATORY]

Every task cites a requirement, an acceptance scenario, a success criterion, or a binding
principle. A task that needs an approval item from `plan.md` does not start before the item is
approved. Out of scope, by `spec.md`: any API or UI that reads the ledger, SSF, events from
ExtProc and agentgateway, and a transactional guarantee.

Findings outside this scope that appear during the work are reported to the user under
"Feature Scope & Deferred Findings" and are not fixed here. Known at planning time: several
services still log an `action` key and not `event` (LOGGING.md section 2.2). A use-case pull
request corrects only the call site that it onboards.

**Validation ownership**: Do not run container-backed recipes (`just test-integration-infra`,
`just test-e2e-docker`) in a sandbox without Docker. Report them as deferred to CI on the pushed
commit, not as validated.

---

## Step 1 — Decision and specification (Phase 1 Setup, Phase 2 documents)

**Pull request**: documents only. **Goal**: the decision, the standard, and the design are reviewable before code exists.

- [X] T001 Write `adrs/048-business-events-as-typed-log-records.md` with Context, Decision, Consequences, Alternatives, status Proposed (Principle II)
- [X] T002 [P] Add ADR 048 to the ADR indexes in `AGENTS.md` and `ARCHITECTURE.md` (Principle II)
- [X] T003 [P] Amend `coding-standards/LOGGING.md` section 8 and its binding sources (STD-LOGGING-8; Principle XV)
- [X] T004 Phase 2a: document the domain model and invariants in `specs/056-business-event-ledger/data-model.md` (Principle V)
- [X] T005 [P] Phase 2b: document configuration keys with a YAML example in `contracts/configuration.md` (Principle VII)
- [X] T006 [P] Phase 2c: draft the only API change in `contracts/admin-business-events-openapi.yaml`; confirm that the end-user API does not change (Principles IV, X)
- [X] T007 [P] Phase 2d: document table, indexes, and migration name in `data-model.md` (Principle IX)
- [X] T008 [P] Phase 2g: read both coding standards; list every new log record and instrument in `contracts/observability.md` (Principle XV)
- [X] T009 Phase 2f: map all acceptance scenarios to `It()` descriptions in `plan.md` Testing Strategy and review Journey 1 against `api/enduser/openapi.yaml` (Principles XIII, XIV)
- [ ] T010 Record each approval that the maintainer supplies for ADR-048 and STD-LOGGING-8 in `plan.md` Approval Items and in the ADR status line, as `Accepted (approved by <login> on <date>)`. Do not infer an approval.

**Checkpoint**: ADR 048 is Accepted. Steps 2 to 5 can start.

---

## Step 2 — Definition type, model, ports, memory adapter (Phase 2.7 Entity Boilerplate)

**Pull request**: about 7 production files. **Needs**: ADR-048, STD-LOGGING-2.2. **Goal**: the types exist and are tested; nothing is wired.

### Tests first

- [ ] T011 [P] Write `internal/domain/bizevent/definition_test.go`: valid and invalid identifiers, panic on duplicate, `LogValue` and `String` return the identifier, `Definitions()` is sorted, zero value is not a definition (FR-002; research R12)
- [ ] T012 [P] Write `internal/domain/model/business_event_test.go`: `Validate` rejects zero ID, empty event, zero time; `Copy` is deep (data-model invariants)
- [ ] T013 [P] Write `internal/adapters/storage/memory/business_event_test.go`: `Append` sets `RecordedAt` and stores a copy, duplicate ID is `Conflict`, `List` filters by principal and events, orders by `(OccurredAt, ID)` descending, respects cursor and limit (FR-003, FR-011)
- [ ] T014 [P] Write `internal/adapters/telemetry/business_event_logging_guard_test.go`: fails when a `bizevent.Definition` is logged under a key other than `event` (contracts/ports.md section 1)
- [ ] T015 Run the new tests and record their compile or semantic failures in the pull request

### Implementation

- [ ] T016 [P] Implement `internal/domain/bizevent/definition.go` (data-model.md; standard library imports only, research R13)
- [ ] T017 [P] Add `BusinessEventID` to `internal/domain/id/gen_ids.go`, regenerate `uuid_ids_gen.go`, document it in `internal/domain/id/AGENTS.md` with the UUIDv7 note (ADR 013; research R6)
- [ ] T018 Implement `internal/domain/model/business_event.go` (depends on T017)
- [ ] T019 Add `BusinessEventAppender`, `BusinessEventReader`, `BusinessEventFilter`, and `BusinessEventCursor` to `internal/ports/storage.go`; update the catalogue in `internal/ports/AGENTS.md` (Principle IX; depends on T018)
- [ ] T020 Implement `internal/adapters/storage/memory/business_event.go` with compile-time assertions (depends on T019)
- [ ] T021 Add the accessors `BusinessEvents()` and `BusinessEventReader()` to `internal/adapters/storage/factory.go`; the PostgreSQL adapter returns nil until step 3; update `internal/adapters/AGENTS.md`
- [ ] T022 [P] Add the typed-definition rule to `coding-standards/LOGGING.md` section 2.2, the guard test to section 9, and one line to the section 10 checklist; state each change and its rationale in the pull request (STD-LOGGING-2.2; Principle XV)
- [ ] T023 [P] Add "Business Event Definition", "Business Event", and "Business Event ID" to the `ARCHITECTURE.md` Glossary (Principle V)
- [ ] T024 Run `just check` and `just test`; confirm that `internal/domain/architecture_test.go` passes

**Checkpoint**: A definition can be declared and logged; stdout shows the identifier; nothing is stored.

---

## Step 3 — Migration and PostgreSQL adapter (Phase 2.5 Foundation)

**Pull request**: about 3 production files. **Needs**: step 2, DB-048. Parallel with step 4.

- [ ] T025 Create `migrations/048_create_business_events.up.sql` and `.down.sql` from `data-model.md`; if `048` is taken on `main`, use the next free number and update the specification set (Principle IX)
- [ ] T026 [P] Add migration `048` to the apply, rollback, and repeat tests in `tests/integration/migrations/` (Principle IX)
- [ ] T027 [P] Write `tests/integration/storage/infra/business_event_test.go`: the T013 cases against PostgreSQL, JSONB round trip of nested attributes, `Append` succeeds with a context that carries a committed transaction, a row without principal is not returned by a principal filter (FR-005, FR-011; research R4)
- [ ] T028 Implement `internal/adapters/storage/postgres/business_event.go` with `sqlx`; `Append` uses `a.db`, not `storageExecutor(ctx)`; wrap errors in `StorageError`; add the factory accessors
- [ ] T029 [P] Add a unit test with `sqlmock` or the existing unit pattern that `Append` does not use a context transaction (FR-005)
- [ ] T030 Run `just check`, `just test`, `just test-integration`; report `just test-integration-infra` as deferred to CI

**Checkpoint**: Both adapters implement the appender and the reader.

---

## Step 4 — Ledger handler and wiring (Phase 2.5 Foundation)

**Pull request**: about 6 production files. **Needs**: step 2, CFG-BE. Parallel with step 3.

### Tests first

- [ ] T031 Write `internal/adapters/telemetry/ledger_handler_test.go` with a fake appender: a typed event is stored once; a string event, a record without `event`, and a zero definition are ignored; `principal`, `actor`, `trace_id` map to their fields and leave the attribute document; scalar, time, duration, group, `LogValuer`, and typed-ID attributes encode as research R7 says; `WithAttrs` and `WithGroup` are included; `Enabled` is true at `Info` when the operational level is `Error`; a cancelled request context still stores; a slow appender stops at the timeout (FR-001, FR-003, FR-005, FR-008)
- [ ] T032 [P] In the same file: on appender error the fallback handler receives exactly one `business_event.ledger.write_failed` record without `principal` and without `error`, the counter increases by one with `error_kind`, the duration histogram records `outcome=failed`, `Handle` returns nil, and the failure record does not enter the appender (FR-006; research R8)
- [ ] T033 [P] In the same file: a record that carries a `model.Secret` attribute stores the redacted form (SR-001)
- [ ] T034 [P] Write configuration tests in `internal/config/`: defaults, range validation of `business_events.write_timeout`, environment variable binding (FR-007)
- [ ] T035 [P] Extend `internal/app/builder_test.go`: flag off gives the chain of `main`; flag on stores a typed event logged through `app.Logger`; flag on without an appender fails at start
- [ ] T036 Run the new tests and record their failures in the pull request

### Implementation

- [ ] T037 Implement `internal/adapters/telemetry/ledger_handler.go` and its two instruments from `contracts/observability.md`; handle instrument constructor errors as OPENTELEMETRY.md section 5.1 requires
- [ ] T038 [P] Add `BusinessEventsConfig` to `internal/ports/config.go`, defaults to `internal/config/loader.go`, and validation to `internal/config/validator.go` (`enabled`, `write_timeout`)
- [ ] T039 Wire the handler in `internal/app/builder.go` as `contracts/ports.md` section 3 shows (depends on T037, T038)
- [ ] T040 [P] Add `examples/config/business-events.yaml` and its entry in `examples/config/README.md`; add the two keys to `docs/configuration.md` (Principle VII)
- [ ] T041 [P] Add `broker.businessEvents.enabled` and `.writeTimeout` to `charts/agentic-identity-broker/values.yaml`, `templates/configmap.yaml`, `values.schema.json`, and the chart README; extend the chart tests
- [ ] T042 [P] Add "Business Event Ledger" to the `ARCHITECTURE.md` Glossary and describe the logger chain with the ledger branch in the observability section (Principles II, V)
- [ ] T043 [P] In `coding-standards/LOGGING.md`: name the ledger handler in the section 1 handler-chain rule and remove the sentence "until they merge, no record is persisted" from section 8 (Principle XV)
- [ ] T044 Run `just check`, `just test`, `just test-integration`, and `just test-e2e-backend` with the flag off (SC-002)

**Checkpoint**: The ledger is wired and off. No definition exists in production code, so nothing is stored even when it is on.

---

## Step 5 — User Story 1: a logged business event is stored (Phase 3, P1) 🎯 MVP

**Pull request**: about 2 production files. **Needs**: steps 3 and 4. **Independent test**: enable the ledger, revoke a grant, read the ledger.

### Tests first

- [ ] T045 [US1] Write `tests/e2e/business_event_ledger_test.go` with one `It()` each for US1.1, US1.2, US1.3, and US2.2, with spec scenario references in comments; add the `business_events.enabled` configuration fixture and the matcher `HaveBusinessEvent` in `tests/e2e/matchers/` (Principle XIII)
- [ ] T046 [P] [US1] Write the US1.4 scenario for the container E2E suite: make the insert fail in PostgreSQL, assert `204`, the write-failed record, and the failure counter (FR-006)
- [ ] T047 [P] [US1] Extend the consent service tests in `internal/domain/consent/`: one stored `consent.grant.revoked` event with `principal`, `agent_id`, `grant_id` on revocation; none on `ErrGrantNotFound`; none when refresh cleanup fails after the delete (LOGGING.md section 8)
- [ ] T048 [US1] Run T045 and T047, record the semantic failures (no event stored) in the pull request, and add the line numbers to the scenario table in `plan.md`

### Implementation

- [ ] T049 [US1] Create `internal/domain/consent/events.go` with `GrantRevoked = bizevent.MustDefine("consent.grant.revoked", bizevent.OwnerVisible())`
- [ ] T050 [US1] In `internal/domain/consent/service.go` `RevokeConsentForPrincipal`, replace `"action", "grant_revoked"` with `"event", GrantRevoked`; keep the other attributes; record the rename from `action=grant_revoked` as LOGGING.md section 2.2 requires
- [ ] T051 [P] [US1] Update existing assertions on the old key (for example `tests/integration/grant_lifecycle_audit_test.go`) to assert `event` (LOGGING.md section 9)
- [ ] T052 [P] [US1] Revise the "Grant and Definition Lifecycle Audit" glossary entry in `ARCHITECTURE.md`: the grant revocation record is now also a business event (Principle II)
- [ ] T053 [US1] Add a benchmark of the revocation request with the ledger on and off on PostgreSQL; record the result against SC-004 in the pull request
- [ ] T054 [US1] Fill in `checklists/onboarding.md` in the pull request description
- [ ] T055 Run `just check`, `just test`, `just test-integration`, `just test-e2e-backend`; report the container suites as deferred to CI

**Checkpoint**: User Story 1 works end to end. The ledger can be enabled outside production.

---

## Step 6 — User Story 3: retention and erasure (Phase 5, P3)

**Pull request**: about 8 production files. **Needs**: step 5, API-ERASURE. Parallel with step 7. Split the erasure endpoint into its own pull request if the diff grows.

- [ ] T056 [US3] Record the supplied approval of API-ERASURE in `plan.md`; stop if it is not supplied
- [ ] T057 [US3] Merge the draft into `api/admin/openapi.yaml`, raise the Admin API minor version, and render the admin reference in `docs/api/` (Principles IV, X)

### Tests first

- [ ] T058 [P] [US3] Add `It()` blocks for US3.1 and US3.2 to `tests/e2e/business_event_ledger_test.go` (Principle XIII)
- [ ] T059 [P] [US3] Write adapter tests for `DeleteRecordedBefore` (cutoff, limit, oldest first) and `DeleteByPrincipal` (exact match, rows without principal untouched) in the memory test file and in `tests/integration/storage/infra/business_event_test.go` (FR-009, FR-010)
- [ ] T060 [P] [US3] Write `internal/domain/bizevent/retention_test.go`: repeats until a short batch, stops on context end, logs `rows_expired` only when rows were deleted, logs `retention_failed` on error (contracts/ports.md section 4)
- [ ] T061 [P] [US3] Write admin handler tests: `200` with `deleted_count`, idempotent repeat, `400` on empty principal, `409` when the ledger is disabled, audit record `business_event.ledger.principal_erased` with typed `principal` and `operator_principal`, and no new ledger row for the erased principal
- [ ] T062 [P] [US3] Write configuration tests for the three retention keys
- [ ] T063 [US3] Run the new tests and record their failures in the pull request

### Implementation

- [ ] T064 [US3] Add `BusinessEventRetention` to `internal/ports/storage.go` and implement it in both adapters with the accessor `BusinessEventRetention()`; update `internal/ports/AGENTS.md`
- [ ] T065 [P] [US3] Add the retention keys to `internal/ports/config.go`, loader defaults, and validator
- [ ] T066 [US3] Implement `internal/domain/bizevent/retention.go` and the `business_events.retention.deleted` counter
- [ ] T067 [US3] Start the retention service in `internal/app/builder.go` when the ledger is enabled; stop it on shutdown
- [ ] T068 [US3] Implement `internal/adapters/http/handlers/admin/business_events_handler.go` and its route on the admin server
- [ ] T069 [P] [US3] Write `docs/operations/business-events.md`: enabling, retention, erasure, the loss window of ADR 048, and the production rule of SR-003; add the retention keys to `docs/configuration.md` and to `examples/config/business-events.yaml`
- [ ] T070 [P] [US3] Add the retention values to the Helm chart (values, configmap, schema, README, tests)
- [ ] T071 Run `just check`, `just test`, `just test-integration`, `just test-e2e-backend`; report the container suites as deferred to CI

**Checkpoint**: The ledger can be enabled in production.

---

## Step 7 — User Story 2: onboard a second use case by the checklist (Phase 4, P2)

**Pull request**: about 2 production files. **Needs**: step 5.

- [ ] T072 [US2] Add the `It()` for US2.1 to `tests/e2e/business_event_ledger_test.go`: a `consent.grant.created` event whose attributes equal the audit record's attributes
- [ ] T073 [P] [US2] Extend the consent service tests: `consent.grant.created` on a new grant, `consent.grant.updated` on a changed grant, no event when the consent is unchanged
- [ ] T074 [US2] Run T072 and T073 and record their semantic failures in the pull request
- [ ] T075 [US2] Declare `GrantCreated` and `GrantUpdated` in `internal/domain/consent/events.go`; in the grant upsert log call of `internal/domain/consent/service.go`, replace the `action` variable with the typed definition; record the renames
- [ ] T076 [US2] Fill in `checklists/onboarding.md` in the pull request description and confirm SC-001: at most three production files, no migration, schema, or API contract file
- [ ] T077 Run `just check`, `just test`, `just test-integration`, `just test-e2e-backend`

**Checkpoint**: The onboarding rule is proven. Further use cases are separate small pull requests that cite FR-012; they are not tasks of this list.

---

## 🔒 Phase N: Constitution Compliance Verification [MANDATORY COMPLIANCE SECTION]

Run the items that apply at the end of each step's pull request, and all of them after step 7.

### Design Phase Verification [MANDATORY]

- [X] T078 Domain model and invariants are documented in `data-model.md` (Principle V)
- [ ] T079 All four domain terms are in the `ARCHITECTURE.md` Glossary (after steps 2 and 4)
- [X] T080 Configuration keys are documented with a YAML example in `contracts/configuration.md` (Principle VII)
- [ ] T081 `examples/config/business-events.yaml` and its README entry exist (after step 4)
- [X] T082 The API change is designed in `contracts/` before implementation; the end-user API has no change (Principle X)
- [ ] T083 API-ERASURE approval is recorded (before step 6)
- [X] T084 The schema is documented with go-migrate file names (Principle IX)
- [X] T085 Every acceptance scenario is mapped to one `It()` (Principle XIII)
- [X] T086 Journey 1 uses real paths, headers, and status codes of `api/enduser/openapi.yaml` (Principle XIV)

### Implementation Phase Verification [MANDATORY]

- [ ] T087 [P] `api/admin/openapi.yaml` matches the implemented erasure operation exactly (Principle IV)
- [ ] T088 [P] ADR 048 status and the implementation agree before the first code step merges (Principle II)
- [ ] T089 [P] Configuration uses the broker's configuration system; no key is read from the environment directly (Principle VII)
- [ ] T090 [P] Migration `048` follows sequential numbering and has apply, rollback, and repeat tests (Principle IX)
- [ ] T091 [P] Both adapters are tested for every method of the three ports (Principle IX)
- [ ] T092 [P] No custom cryptography (Principle III)
- [ ] T093 [P] Every new or changed log call satisfies the LOGGING.md section 10 checklist; every new instrument is in `contracts/observability.md` and follows OPENTELEMETRY.md section 5 (Principle XV)
- [ ] T094 [P] Tests assert that a credential on an onboarded path is absent from the stored event and that the write-failed record has no principal (SR-001)
- [ ] T095 [P] `TestLoggedPrincipalsAreTyped` and the `forbidigo` rule are unchanged; no linter suppression was added (Principle XV)
- [ ] T096 [P] Domain packages do not import adapters; `bizevent` imports only the standard library; ExtProc does not import `bizevent` (Principle VI; ADR 027)
- [ ] T097 [P] The logger and the appender are injected through the builder; no `slog.Default()` (Principle XII)
- [ ] T098 [P] All eight acceptance scenarios have a passing `It()`; no `XIt`, `PIt`, or `Skip()` (Principle XIII)
- [ ] T099 [P] With `business_events.enabled: false`, the full suite passes without a changed assertion outside the onboarded call sites (SC-002)
- [ ] T100 [P] `docs/operations/business-events.md` states that the ledger is not enabled in production before step 6 is deployed (SR-003)
- [ ] T101 No endpoint reads the ledger (SR-004)
- [ ] T102 Final gates after step 7: `just check`, `just test`, `just test-integration`, `just test-e2e-backend`; container suites from CI on the pushed commit

---

## Dependencies & Execution Order

### Step dependencies

```text
1 ──► 2 ──► 3 ──┐
        └─► 4 ──┴─► 5 ──► 6 (needs API-ERASURE)
                      └─► 7 ──► further use cases, one pull request each
```

- Step 1 blocks everything: ADR 048 must be Accepted before code depends on it (Principle II).
- Steps 3 and 4 depend only on step 2 and touch different files.
- Step 5 needs both. It is the MVP.
- Steps 6 and 7 depend only on step 5 and touch different files, except
  `tests/e2e/business_event_ledger_test.go`; merge one, then rebase the other.

### Within each step

- Tests before implementation; record the red run in the pull request.
- Ports before adapters; adapters before wiring.
- Documents (glossary, configuration reference, standard) in the same pull request as the code that they describe.

### Parallel opportunities

- Step 2: T011 to T014 together; T016, T017, T022, T023 together.
- Step 4: T031 to T035 together; T040 to T043 together.
- Steps 3 and 4 by different contributors. Steps 6 and 7 by different contributors.

## Implementation Strategy

### MVP first

Steps 1 to 5. Stop and validate with [quickstart.md](./quickstart.md) sections 1 to 5. At this
point one event is stored end to end and the activity feature can start its read path on the
memory adapter.

### Incremental delivery

Step 6 makes the ledger safe for production. Step 7 proves that onboarding is small. After that,
each use case is one pull request by the onboarding checklist, in the order that the consumers
need: session established and terminated, session refresh failed, approval decisions, the
impersonation decision, operator events. Token issuance and exchange wait for the SC-004 result.

## Notes

- A task with `[X]` is complete in the pull request of its step, not approved. Approval is recorded by T010 and T056 only.
- If `main` takes migration number `048` or ADR number `048` before a step merges, renumber in that step and update every reference in this directory.
- Commit after each task or logical group. Do not mix two delivery steps in one pull request.
