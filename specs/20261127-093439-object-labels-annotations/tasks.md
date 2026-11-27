---

description: "Task list for labels and annotations on Agents, Services, and Permission Sets"
---

# Tasks: Labels and Annotations on Agents, Services, and Permission Sets

**Input**: `specs/20261127-093439-object-labels-annotations/`: spec.md, plan.md, research.md,
data-model.md, contracts/object-metadata.md, contracts/admin-openapi-metadata.yaml, and
quickstart.md.

**Prerequisites**: The maintainer confirms the API drafts (T006, API-004) before any production
behavior changes.

**Tests**: Required by the spec (Constitution VIII and XIII). Write each test before the behavior
it covers. Tests must compile and fail on their assertions, not on undefined symbols. Minimal
production signatures are acceptable scaffolding. Do not use `XIt`, `PIt`, `Skip()`, placeholder
always-fail assertions, or red-phase comments. Red-run evidence is not required. Container-backed
suites (`just test-integration-infra`, `just test-e2e-docker`) may first run in CI.

**Organization**: Tasks are grouped by user story. Data-model citations use the form
`data-model.md` `Entity.field`. The canonical constraint is in data-model.md. A task states a
constraint only when it adds to or narrows that rule.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Can run in parallel (different files, no dependency on an incomplete task)
- **[Story]**: The user story the task belongs to (US1, US2, US3)

## 🔒 Feature Scope [MANDATORY]

- Every task cites a requirement (FR, API, DB, SR, SC), an acceptance scenario, or a binding
  principle.
- No label selectors or index, Admin UI, End-User exposure, CEL, OPA, or Grant Context wiring, and
  no metadata on Protected Resources or keys (spec Out of Scope). No new entity, typed ID,
  repository, endpoint, configuration, or telemetry.
- Phase 0 (refactoring) and Phase 2.7 (entity boilerplate) are skipped, as recorded in plan.md.
- For unrelated findings, ask the user before filing an issue. Record only user-approved issue
  URLs in a non-checkbox Future Work note in plan.md.

---

## Phase 1: Setup

**Purpose**: Load the governing instructions and confirm the migration slot.

- [ ] T001 Confirm that `.specify/feature.json` selects `specs/20261127-093439-object-labels-annotations`. Read `internal/AGENTS.md`, `internal/domain/AGENTS.md`, `internal/adapters/AGENTS.md`, `tests/AGENTS.md`, `tests/e2e/AGENTS.md`, `tests/integration/AGENTS.md`, `coding-standards/LOGGING.md`, `coding-standards/OPENTELEMETRY.md`, and `adrs/047-unique-agent-external-id-reconcile.md` before changing those areas (Constitution II, XV).
- [ ] T002 List `migrations/` and confirm that `050` is still the next unused number. If another branch has taken it, update the migration number in `specs/20261127-093439-object-labels-annotations/data-model.md`, `plan.md`, and `quickstart.md` before T008 (DB-001; Constitution IX).

---

## 🔒 Phase 2: Design Preconditions (Blocking Prerequisites) [MANDATORY]

**⚠️ CRITICAL**: No production behavior changes until this whole phase is complete, including the
API confirmation in T006.

### Phase 2a: Domain Model & Glossary

- [ ] T003 [P] Add **Object Metadata**, **Label**, and **Annotation** to the `ARCHITECTURE.md` glossary and to the Domain Glossary table in root `AGENTS.md`, using the definitions in spec.md Domain Model and `data-model.md` Value object. In `ARCHITECTURE.md` §4, add the `labels` and `annotations` columns to the `agents`, `thirdparty_oauth2_services`, and `permission_sets` data-store descriptions (FR-001; Constitution V).
- [ ] T004 Create `adrs/<UTC creation stamp>-object-labels-annotations.md`, with the stamp taken at creation time. Record research.md §1–§11: the value object, strict `jsontext` decoding, top-level member scan, grammar, reserved prefix, limits, 512 KiB caps on Agent PUT and collection PUT, write semantics per resource, persistence, no telemetry, and no runtime effect until a superseding ADR wires labels into CEL and OPA. State that it extends `047-unique-agent-external-id-reconcile` without deviating from it. Set the status to Accepted only after T006. Add the ADR by its full file stem to the ADR index in `ARCHITECTURE.md` and the ADR Decision Index in root `AGENTS.md` (SR-005; Constitution II).

### Phase 2b: Configuration Design

- [ ] T005 [P] Confirm the plan's conclusion that the feature adds no configuration. Leave `internal/ports/config.go`, `examples/config/`, `examples/config/README.md`, `charts/agentic-identity-broker/`, and ExtProc configuration unchanged. Note the result in the plan.md Precondition status table (Constitution VII).

### Phase 2c: API Design

- [ ] T006 Get the maintainer's confirmation of `contracts/object-metadata.md` and `contracts/admin-openapi-metadata.yaml`, including the fixed messages in §4, the write matrix in §5, and the 512 KiB caps in §6. Record who confirmed, when, and where in the plan.md Precondition status table, and change the contract status line. Do not mark this task done without a real confirmation (API-004; Constitution X).
- [ ] T007 After T006, publish the confirmed contract in `api/admin/openapi.yaml`. Set the version to 3.1.0. Add `ObjectMetadata`, `MetadataLabels`, and `MetadataAnnotations`. Add `metadata` to `Agent`, `AgentCreateRequest`, `AgentUpdateRequest`, `Service`, `ServiceCreateRequest`, `ServiceUpdateRequest`, `CreatePermissionSetRequest`, and `PermissionSetResponse`. Add the `metadata` row to the collection PUT field-policy table, the Service PUT omission exception, and the Permission Set PUT clearing rule. Add the request, response, and four `400` examples listed in the fragment's operation notes. Leave `api/enduser/openapi.yaml` at 1.3.0 and unchanged (API-001, API-002, API-003; Constitution IV).

### Phase 2d: Database Design

- [ ] T008 Write `migrations/050_object_metadata.up.sql` and `migrations/050_object_metadata.down.sql` (number from T002). Implement the six columns and six named constraints in the `data-model.md` Persistence table: `agents.labels`, `agents.annotations`, `thirdparty_oauth2_services.labels`, `thirdparty_oauth2_services.annotations`, `permission_sets.labels`, and `permission_sets.annotations`. Both directions are atomic. Add no index, backfill, or timestamp rewrite. The down migration drops the constraints, then the columns (DB-001, DB-003; Constitution IX).
- [ ] T009 [P] Write `tests/integration/migrations/object_metadata_migration_test.go` on the production migration chain, following `tests/integration/migrations/service_icons_migration_test.go`. Run up, down, and up again. Rows inserted at version `049` read as `{}` for each `data-model.md` Persistence column. Each named check rejects JSON `null` and a non-object value. Unrelated columns survive both directions. Use explicit column lists (DB-002; Constitution IX).

### Phase 2e: Frontend/Design System Review

- [ ] T010 [P] Record in plan.md that no design-system review applies: `web/` and `api/enduser/openapi.yaml` do not change, and there is no Wireloom or Playwright work (FR-020; Constitution XI).

### Phase 2f: E2E Acceptance Test Design

- [ ] T011 [P] Add `tests/e2e/fixtures/object_metadata.go` with distinctive sentinel label and annotation keys and values. Add generators for 64 and 65 labels, and for annotations that total exactly 65,536 and 65,537 decoded UTF-8 bytes. Include multi-byte characters and JSON escapes, so the byte count is measured after decoding (FR-007, FR-008; SC-006).
- [ ] T012 Write the US1.1–US1.5 `It()` blocks in new `tests/e2e/object_metadata_test.go`, with the canonical names from the plan.md E2E table and a `// Scenario USx.y from specs/20261127-093439-object-labels-annotations/spec.md` comment on each. Use `Describe` → `Context` → `It`, production bootstrap, a fresh server per test, and existing agent, permission-set, and principal fixtures. Label US1.2 `docker` and run it against PostgreSQL, following `tests/e2e/agent_external_id_backend_test.go`. Assert status codes, bodies, key order, `updated_at`, and the count of `agents.agent.updated` records in a `bootstrap.NewBufferedJSONLogger` buffer, and the absence of sentinels from that buffer (US1.1–US1.5; Constitution XIII).
- [ ] T013 Add the US2.1–US2.5 `It()` blocks to `tests/e2e/object_metadata_test.go`. Cover all three resources and Agent collection PUT for US2.1 and US2.2. Cover the T011 boundary pairs for US2.3. For US2.4, assert the exact contract §4 message and that the response body contains no sentinel. For US2.5, assert that `exposure` and `Exposure` stay distinct (US2.1–US2.5; SC-003).
- [ ] T014 Add the US3.1–US3.5 `It()` blocks to `tests/e2e/object_metadata_test.go`. For US3.5, run the existing consent, token-exchange, and approval bootstrap twice, with and without metadata on the involved resources. Compare consent decisions, decoded token claims, and approval matching. Assert that no End-User response, including `GET /api/consent/agents` and `GET /api/consent/agents/{id}`, contains `metadata` or a sentinel. Attach `bootstrap.NewInMemoryTracerProvider` and an `sdkmetric.ManualReader`, as in `tests/e2e/session_metrics_test.go`, and assert that no span attribute, span event, or metric attribute contains a sentinel (US3.1–US3.5; FR-019, FR-021, FR-022; SC-004, SC-005, SC-006).
- [ ] T015 Make T012–T014 compile and fail on their assertions where they run locally. Check that the file has exactly 15 `It()` blocks, one per scenario, with the canonical names from plan.md. Check that there are no pending or skipped cases and no placeholder assertions (Constitution VIII, XIII).
- [ ] T016 After T007, check spec.md Journeys 1–4 against the published `api/admin/openapi.yaml` and `contracts/object-metadata.md`: request and response shapes, key order, omission of empty maps, status codes, and the exact `400` messages. Confirm that no configuration, JWT, or Wireloom example applies, and that each journey links its scenarios (Constitution XIV).

### Phase 2g: Observability Applicability

- [ ] T017 [P] Record in plan.md that the feature adds or changes no log event, span, attribute, or metric. List the existing paths that need sentinel-absence assertions: the audit records `agents.agent.created`/`updated`, `permissionset.permission_set.created`/`updated`, and `service.thirdparty.provider_created`/`updated`, and the Admin handler `WarnContext(..., "error", err)` validation-failure calls (FR-022; SR-004; Constitution XV).

**Checkpoint**: Maintainer confirmation recorded. ADR, glossary, Admin OpenAPI 3.1.0, migration
pair and test, and all 15 compiling acceptance cases are in place.

---

## Phase 2.5: Foundational Infrastructure

**Purpose**: The domain value object and the Admin decoder that every story uses.

**⚠️ CRITICAL**: No user story work begins until this phase is complete.

- [ ] T018 [P] Write table-driven tests in new `internal/domain/model/object_metadata_test.go` for `data-model.md` `ObjectMetadata.labels` and `ObjectMetadata.annotations` under contract §3. Cover the boundaries 63/64 name characters, 253/254 prefix characters, 64/65 labels, and 65,536/65,537 annotation bytes. Cover `/`-count, empty prefix and name, uppercase prefix, whitespace, tab, non-ASCII look-alikes, empty label value, NUL, multi-byte counting, `agenticidentitybroker.dev` and a subdomain of it, and the allowed `example.agenticidentitybroker.dev.evil.com`. Also cover case-distinct keys, input and output copy isolation, `Equal` with reordered and changed maps, `IsEmpty`, the valid zero value, and that no error message contains the input (FR-004–FR-009, FR-012).
- [ ] T019 Implement `model.ObjectMetadata` in new `internal/domain/model/object_metadata.go` with `NewObjectMetadata`, `Labels`, `Annotations`, `Equal`, `IsEmpty`, and a re-check method the entity validators call, as described in data-model.md Value object. Export fixed-message errors for the key, reserved-key, value, count, and size rows of contract §4, and the per-entry checks that the T021 decoder applies in document order. Use only `regexp` and the standard library. Make T018 pass (FR-004–FR-009; SR-001).
- [ ] T020 [P] Write tests in new `internal/adapters/http/handlers/admin/object_metadata_test.go` for the decoder: exact top-level `metadata`, repeated and case-variant members, and presence. Cover `null` and non-object at all three levels, unknown and repeated `metadata` members, duplicate keys, invalid UTF-8, unpaired surrogates, and non-string values. Assert the fixed message for every contract §4 row and its check order, and that no message contains submitted text or the `jsontext` error text. Assert that invalid UTF-8 or duplicates in other top-level members keep their current behavior. For the response type, assert that empty maps and an empty `metadata` are omitted and that keys are in byte order (FR-001, FR-003, FR-010–FR-012; SR-004).
- [ ] T021 Implement new `internal/adapters/http/handlers/admin/object_metadata.go`. Scan the body's top level with `encoding/json/jsontext`. Decode `metadata` strictly. Apply the T019 entry checks in document order, then build the value with `model.NewObjectMetadata`. Return presence, the value, and a fixed-message error. Discard `jsontext` error text. Add the omitting response type. Make T020 pass (research §1, §2, §9; FR-010–FR-012).

**Checkpoint**: T018 → T019 and T020 → T021 are green.

---

## Phase 3: User Story 1 - Categorize and Annotate an Agent, Including by GitOps Reconciliation (Priority: P1) 🎯 MVP

**Goal**: Agent create, ID-addressed PUT, and collection PUT store, replace, and return metadata. A
reordered replay is a no-op. A label-only change is an audited update.

**Independent Test**: Create an Agent with metadata, read it back, reconcile it with reordered maps
and then with one changed label, and compare `updated_at` and the audit records (US1.1–US1.5).

### Tests for User Story 1 [MANDATORY - Principle VIII] ⚠️

- [ ] T022 [P] [US1] Add tests in `internal/domain/storage/agent_test.go` that `data-model.md` `storage.Agent.Metadata` survives `Copy()` and that `Validate` and `ValidateForCreate` apply the value's re-check (FR-013; Constitution VI).
- [ ] T023 [P] [US1] Add tests in `internal/domain/agents/reconcile_test.go`: reordered maps compare equal and the replay writes nothing, emits no audit record, and keeps `updated_at`; a change to one label or one annotation is an update that advances `updated_at` and emits one `agents.agent.updated` record without sentinels. Add tests in `internal/domain/agents/service_test.go` that omitted and empty metadata replace stored metadata on update (FR-013, FR-014, FR-022; SC-002).
- [ ] T024 [P] [US1] Write new `internal/adapters/http/handlers/admin/agents_metadata_test.go` for Agent POST, ID-addressed PUT, and collection PUT. Cover omitted, `{}`, one member, and both members, each passed to the domain as a value. Cover `null` at each level with `400` and no domain call, response omission and order, existing error precedence, and a 512 KiB body accepted on both PUTs while a larger body returns `400 invalid request body` (FR-011, FR-013; contract §6; US1.4, US1.5).
- [ ] T025 [P] [US1] Write new `internal/adapters/storage/memory/object_metadata_test.go` with Agent cases: `data-model.md` `storage.Agent.Metadata` round-trips through Create, Update, and every read, and two Agents with equal maps stay independent (DB-002).
- [ ] T026 [P] [US1] Write new `tests/integration/storage/infra/object_metadata_test.go` with Agent cases against PostgreSQL: round-trips through `Create`, `Update`, `Get`, `GetByIDs`, `List`, `GetByExternalID`, `GetByClientID`, and `GetByClientURI`. An empty map is stored as `'{}'`, never JSON `null`, in `data-model.md` `agents.labels` and `agents.annotations`. A stored value that fails `model.NewObjectMetadata` returns an internal storage error and no entity (DB-002; SR-001).

### Implementation for User Story 1

- [ ] T027 [US1] Add `data-model.md` `storage.Agent.Metadata` to `internal/domain/storage/agent.go`, keep it in `Copy()`, and call the value's re-check from `Validate` and `ValidateForCreate`. Add no JSON tag. Make T022 pass (FR-001; Constitution VI).
- [ ] T028 [US1] In `internal/domain/agents/reconcile.go`, make `sameDefinition` return false when `!a.Metadata.Equal(b.Metadata)`. Confirm that the update branch in `internal/domain/agents/service.go` replaces metadata in full. Make T023 pass (FR-013, FR-014).
- [ ] T029 [P] [US1] Carry `storage.Agent.Metadata` through `internal/adapters/storage/memory/agent_repository.go`. Make the T025 Agent cases pass (DB-002).
- [ ] T030 [P] [US1] Add `labels` and `annotations` to every explicit `SELECT`, `INSERT`, `UPDATE`, and `RETURNING` list in `internal/adapters/storage/postgres/agent_repository.go`, including `GetByClientID` and `GetByClientURI`. Marshal `Labels()` and `Annotations()` as `{}` when empty. Rebuild reads with `model.NewObjectMetadata` and fail closed. Make the T026 Agent cases pass (data-model.md Persistence `agents.labels`, `agents.annotations`; DB-002).
- [ ] T031 [US1] In `internal/adapters/http/handlers/admin/agents_handler.go`, wire the T021 decoder into create, ID-addressed PUT, and collection PUT after the existing v1 decode and before any domain call. Pass the empty value when `metadata` is omitted. Map every metadata failure to `400 validation failed`. Add `metadata` to the Agent response. Raise both `http.MaxBytesReader` caps from 64 KiB to 512 KiB. Make T024 pass (FR-001–FR-003, FR-013; contract §6).
- [ ] T032 [US1] Run `just check`, the US1 unit and adapter tests, the US1.1 and US1.3–US1.5 cases with `just test-e2e-backend`, and US1.2 with `just test-e2e-docker` where a container runtime is available. Record actual results and unavailable infrastructure in `specs/20261127-093439-object-labels-annotations/quickstart.md` (US1.1–US1.5).

**Checkpoint**: User Story 1 works on its own on both Agent write paths and both adapters.

---

## Phase 4: User Story 2 - Reject Ambiguous, Malformed, or Reserved Metadata (Priority: P1)

**Goal**: Every rejection class fails closed on all seven Admin write operations, with fixed
messages and no partial write.

**Independent Test**: Submit each invalid class from contract §4 on each resource and check for
`400 validation failed`, an unchanged resource, and a message that contains neither key nor value
(US2.1–US2.5).

**Dependency**: The Agent paths come from US1. The Permission Set and Service parsing and entity
fields are added here. The accepted look-alike prefix in US2.2 is read back on Permission Sets and
Services only after the US3 response and persistence tasks, so the US2.2 E2E case turns green
after T049.

### Tests for User Story 2 [MANDATORY - Principle VIII] ⚠️

- [ ] T033 [P] [US2] Write new `internal/adapters/http/handlers/admin/object_metadata_write_paths_test.go`. For every contract §4 row, cover Agent POST, Agent PUT, Agent collection PUT, Service POST and PUT in both discovery and explicit branches, and Permission Set POST and PUT. Assert `400 validation failed`, the exact message, no sentinel in the body or in the captured handler log, and no domain, discovery, branch-key, encryption, or repository call. Include requests that combine valid field changes with invalid metadata (FR-010–FR-012, FR-022; SR-001, SR-003, SR-004; SC-003).
- [ ] T034 [P] [US2] Add tests in `internal/domain/storage/permission_set_test.go` that `data-model.md` `storage.PermissionSet.Metadata` survives `Copy()` and that `validateCommon` applies the value's re-check (Constitution VI).
- [ ] T035 [P] [US2] Write new `internal/domain/thirdparty/object_metadata_test.go`: `ValidateForCreate`, `ValidateForUpdate`, and `ValidateDiscoveryRequest` apply the value's re-check before any other step, and a failing Service create or update makes no discovery, branch-key, encryption, or repository call (FR-012; SR-001).

### Implementation for User Story 2

- [ ] T036 [P] [US2] Add `data-model.md` `storage.PermissionSet.Metadata` to `internal/domain/storage/permission_set.go`, keep it in `Copy()`, and call the value's re-check from `validateCommon`. Make T034 pass (FR-001).
- [ ] T037 [P] [US2] Add `data-model.md` `ThirdpartyOAuth2ProviderEntity.Metadata` and `ThirdpartyOAuth2ProviderEntity.MetadataProvided` to `internal/domain/model/thirdparty_oauth2_provider.go`, following `IconSVG` and `IconSVGProvided`. Copy both in `Copy()` and `RedactedCopy()`. Apply the value's re-check first in `ValidateForCreate`, `ValidateForUpdate`, and `ValidateDiscoveryRequest`. Make T035 pass (FR-001, FR-012).
- [ ] T038 [US2] In `internal/adapters/http/handlers/admin/permission_sets_handler.go`, wire the T021 decoder into POST and PUT after the existing decode and before the domain call. Set the entity value, with the empty value when `metadata` is omitted. Map failures to `400 validation failed` (FR-010–FR-012, FR-015; SR-003).
- [ ] T039 [US2] In `internal/adapters/http/handlers/admin/services_handler.go`, wire the T021 decoder into POST and PUT in both discovery and explicit branches, before discovery. Set `Metadata` and set `MetadataProvided` from presence. Map failures to `400 validation failed`. Make T033 pass (FR-010–FR-012, FR-016; SR-003).
- [ ] T040 [US2] Run `just check`, the T033–T035 tests, and the US2.1 and US2.3–US2.5 cases with `just test-e2e-backend`. Record results in `specs/20261127-093439-object-labels-annotations/quickstart.md` (US2.1–US2.5).

**Checkpoint**: Every write path rejects every invalid class and changes nothing.

---

## Phase 5: User Story 3 - Categorize Permission Sets and Services (Priority: P2)

**Goal**: Permission Sets replace metadata in full. Service updates keep omitted metadata and
replace sent metadata whole. Metadata has no runtime effect.

**Independent Test**: Create and replace a Permission Set with and without metadata, update a
Service with and without metadata, and read both back (US3.1–US3.5).

### Tests for User Story 3 [MANDATORY - Principle VIII] ⚠️

- [ ] T041 [P] [US3] Write new `internal/adapters/http/handlers/admin/permission_sets_metadata_test.go` for create, read, list, and PUT: an omitted `metadata` clears stored metadata, `updated_at` advances, one `permissionset.permission_set.updated` record is emitted without sentinels, and the response omits empty maps in byte order (FR-003, FR-015, FR-017, FR-022; US3.1, US3.2).
- [ ] T042 [P] [US3] Write new `internal/domain/thirdparty/object_metadata_update_test.go` for `prepareUpdate`: an omitted `metadata` keeps the persisted value, a sent value replaces both maps with an omitted member empty, `{}` clears both, keys are never merged, and the row version advances on an accepted change (FR-016, FR-017; US3.3, US3.4).
- [ ] T043 [P] [US3] Write new `internal/adapters/http/handlers/admin/services_metadata_test.go` for Service create, read, list, and PUT: keep, replace, `{}`, and `null` rejected without change. Assert the `ETag` change and response omission and order, and one `service.thirdparty.provider_updated` record without sentinels (FR-003, FR-011, FR-016, FR-017, FR-022).
- [ ] T044 [US3] Add Permission Set and Service cases to `internal/adapters/storage/memory/object_metadata_test.go`: `storage.PermissionSet.Metadata` and `ThirdpartyOAuth2ProviderEntity.Metadata` round-trip, the stored Service record has `ThirdpartyOAuth2ProviderEntity.MetadataProvided` false (`data-model.md`), and equal values stay independent (DB-002).
- [ ] T045 [US3] Add Permission Set and Service cases to `tests/integration/storage/infra/object_metadata_test.go` for every read and write method of both repositories. Empty maps are stored as `'{}'` in `data-model.md` `permission_sets.labels`, `permission_sets.annotations`, `thirdparty_oauth2_services.labels`, and `thirdparty_oauth2_services.annotations`. Invalid stored values fail closed (DB-002; SR-001).

### Implementation for User Story 3

- [ ] T046 [US3] In `internal/domain/thirdparty/service.go` `prepareUpdate`, set `entity.Metadata = persisted.Metadata` when `!entity.MetadataProvided`, before discovery, branch-key, and encryption steps. Make T042 pass (FR-016).
- [ ] T047 [P] [US3] Carry the value through `internal/adapters/storage/memory/permission_set_repository.go`, `internal/adapters/storage/memory/thirdparty_provider.go`, and `internal/adapters/storage/memory/thirdparty_provider_record.go`. Clear `MetadataProvided` in the stored record, as `IconSVGProvided` is cleared today. Make T044 pass (DB-002).
- [ ] T048 [P] [US3] Add `labels` and `annotations` to every explicit column list in `internal/adapters/storage/postgres/permission_set_repository.go`, `internal/adapters/storage/postgres/thirdparty_provider.go`, and `internal/adapters/storage/postgres/thirdparty_provider_record.go`. Write `{}` for empty maps, and rebuild reads with `model.NewObjectMetadata`, failing closed. Keep the existing locking and row versions. Make T045 pass (DB-002).
- [ ] T049 [US3] Add `metadata` to the Permission Set response in `internal/adapters/http/handlers/admin/permission_sets_handler.go` and to the Service response in `internal/adapters/http/handlers/admin/services_handler.go`, using the T021 response type. Make T041 and T043 pass (FR-001, FR-003).
- [ ] T050 [US3] Search the repository for reads of `Metadata` on the three entities. Confirm that only the Admin handlers, the storage adapters, entity validation, and Agent reconcile read it. Confirm that `consent.AgentDelegation`, `AgentDetailData`, and the other End-User DTOs gained no field. Remove any other read (FR-019, FR-021; SR-002).
- [ ] T051 [US3] Run `just check`, the US3 unit and memory tests, `just test-integration-infra` where available, and US3.1–US3.5 plus US2.2 with `just test-e2e-backend`. Record results in `specs/20261127-093439-object-labels-annotations/quickstart.md` (US3.1–US3.5, US2.2).

**Checkpoint**: All three stories work, and all 15 acceptance cases pass where they can run.

---

## 🔒 Phase N: Constitution Compliance Verification [MANDATORY]

### Design Phase Verification

- [ ] T052 Verify that every task and change traces to a requirement, scenario, or binding principle, and that no out-of-scope work was added (Feature Scope).
- [ ] T053 [P] Verify the Object Metadata, Label, and Annotation glossary entries in `ARCHITECTURE.md` and root `AGENTS.md` (Constitution V).
- [ ] T054 [P] Verify that `examples/config/`, `examples/config/README.md`, `internal/ports/config.go`, and `charts/agentic-identity-broker/` are unchanged, as no configuration was added (Constitution VII).
- [ ] T055 Verify that `api/admin/openapi.yaml` is 3.1.0 with the confirmed fragments, that `api/enduser/openapi.yaml` is unchanged at 1.3.0, and that the T006 confirmation is recorded in plan.md and referenced in the PR description (Constitution IV, X).
- [ ] T056 [P] Verify that the migration design in data-model.md matches `migrations/050_object_metadata.{up,down}.sql` (Constitution IX).
- [ ] T057 [P] Verify that no frontend change was made, so no design-system review, Playwright test, or screenshot applies (Constitution XI).
- [ ] T058 Verify that spec.md Journeys 1–4 link their scenarios and match the published contract (Constitution XIV).
- [ ] T059 Verify that the 15 E2E cases were written before the behavior, with realistic assertions and no pending, skipped, or placeholder cases, and no red-phase comments (Constitution XIII).
- [ ] T060 [P] Verify that no log event, span, or metric instrument was added, so no `contracts/observability.md` is needed (Constitution XV).

### Implementation Phase Verification

**Feature Scope**:
- [ ] T061 Verify that no unrelated remediation is included and that any user-approved issue URLs appear only under Future Work in plan.md.

**API & Documentation** (Principles IV, X):
- [ ] T062 Verify that the implementation matches `api/admin/openapi.yaml` and `contracts/object-metadata.md` exactly: field names, omission, key order, write matrix, fixed messages, body caps, and the `400` envelope.
- [ ] T063 [P] Document labels and annotations for operators in `docs/guides/manage-agents-and-services.md`: shape, rules, reserved prefix, write behavior per resource, and that metadata is admin-only with no runtime effect. Do not add runbooks or implementation details (FR-001–FR-021; root `AGENTS.md` Documentation Scope).

**Architecture & Documentation** (Principle II):
- [ ] T064 Verify that the T004 ADR is Accepted and agrees with the final code, and that `ARCHITECTURE.md` reflects the new columns and the ADR (SR-005).

**Configuration** (Principle VII):
- [ ] T065 [P] Verify that no configuration path was added to the broker or ExtProc.

**Database & Persistence** (Principle IX):
- [ ] T066 [P] Verify that the migration uses the next sequential number, is atomic in both directions, and passes `tests/integration/migrations/object_metadata_migration_test.go`.
- [ ] T067 [P] Verify that the PostgreSQL Agent, Permission Set, and Service repositories pass `tests/integration/storage/infra/object_metadata_test.go`, and that both adapters behave the same.

**Security** (Principles I, III):
- [ ] T068 Verify fail-closed behavior: every rejection class is enforced on all seven write operations, invalid stored values fail reads, and there is no bypass (SR-001, SR-003).
- [ ] T069 [P] Verify that no cryptography was added and that decoding uses only the standard library.

**Coding Standards** (Principle XV):
- [ ] T070 Review the changed code against `coding-standards/LOGGING.md` and `coding-standards/OPENTELEMETRY.md` and their review checklists. Confirm that the E2E and handler tests assert sentinel absence from logs, audit records, spans, span events, and metric attributes (FR-022; SC-006).
- [ ] T071 Verify that no lint or `telemetrylint` check is disabled or suppressed. Run `just telemetry-doc` and commit `docs/reference/telemetry.md` if it changed.

**Architecture Patterns** (Principle VI):
- [ ] T072 Verify that handlers parse shape and presence only, that the domain value object and services own the rules and field policy, and that there is no handler-to-repository path and no builder change (Constitution VI, XII).

**Testing** (Principle VIII):
- [ ] T073 Verify that the unit, adapter, and integration tests were written before the behavior, drove the design, changed minimally during implementation, and that no Bash script checks correctness.

**E2E Acceptance Testing** (Principle XIII):
- [ ] T074 Verify that `tests/e2e/object_metadata_test.go` has exactly one `It()` per scenario, uses Ginkgo/Gomega per `tests/e2e/README.md` with `Describe` → `Context` → `It`, carries scenario comments, and changed only in fixtures during implementation.
- [ ] T075 Run `just check`, `just test`, `just test-integration-all`, `just test-e2e-backend`, and `just test-e2e-docker`, then the full `just verify` gate from `CONTRIBUTING.md` before the PR. Record actual commands, results, and unavailable infrastructure in `specs/20261127-093439-object-labels-annotations/quickstart.md`. Do not claim that a run passed if it did not run.

---

## Dependencies & Execution Order

### Phase Dependencies

- **Setup (Phase 1)**: No dependencies.
- **Design Preconditions (Phase 2)**: Depend on Phase 1. T006 blocks T007, T016, and the
  Accepted status in T004. T008 needs T002. T012–T014 need T011. T015 needs T012–T014.
- **Foundation (Phase 2.5)**: Depends on all of Phase 2. T019 needs T018, and T021 needs T019 and
  T020.
- **US1 (Phase 3)**: Depends on Phase 2.5.
- **US2 (Phase 4)**: Depends on Phase 2.5 and, for the Agent rows of T033, on T031.
- **US3 (Phase 5)**: Depends on T036–T039 from US2 for the entity fields and handler parsing.
- **Compliance (Phase N)**: Depends on all three stories.

### User Story Dependencies

- **US1 (P1)**: Independent after Phase 2.5. This is the MVP.
- **US2 (P1)**: Its Permission Set and Service rejections are independent of US1. Its Agent rows
  reuse the US1 wiring. The US2.2 E2E case waits for T049.
- **US3 (P2)**: Builds on the US2 entity fields and parsing for Permission Sets and Services.

### Within Each User Story

- Tests come before the code they cover.
- Domain entities, then domain services, then adapters, then handlers.
- Run the story's checks before you start the next story.

### Parallel Opportunities

- Phase 2: T003, T005, T009 (after T008), T010, T011, and T017 touch different files.
- Phase 2.5: T018 and T020.
- US1: T022–T026 together, then T029 and T030 together.
- US2: T033–T035 together, then T036 and T037 together.
- US3: T041–T043 together, then T047 and T048 together.
- Phase N: the tasks marked [P].

---

## Parallel Example: User Story 1

```bash
# Tests, in parallel:
Task: "T022 storage.Agent metadata tests in internal/domain/storage/agent_test.go"
Task: "T023 reconcile no-op and update tests in internal/domain/agents/reconcile_test.go"
Task: "T024 Agent handler metadata tests in internal/adapters/http/handlers/admin/agents_metadata_test.go"
Task: "T025 memory round-trip tests in internal/adapters/storage/memory/object_metadata_test.go"
Task: "T026 PostgreSQL round-trip tests in tests/integration/storage/infra/object_metadata_test.go"

# After T027 and T028, the adapters in parallel:
Task: "T029 memory agent_repository.go"
Task: "T030 postgres agent_repository.go column lists"
```

## Parallel Example: User Story 2

```bash
Task: "T033 write-path rejection tables in internal/adapters/http/handlers/admin/object_metadata_write_paths_test.go"
Task: "T034 PermissionSet tests in internal/domain/storage/permission_set_test.go"
Task: "T035 provider validation tests in internal/domain/thirdparty/object_metadata_test.go"
```

## Parallel Example: User Story 3

```bash
Task: "T041 Permission Set handler tests"
Task: "T042 prepareUpdate tests in internal/domain/thirdparty/object_metadata_update_test.go"
Task: "T043 Service handler tests"
```

---

## Implementation Strategy

### MVP First (User Story 1 Only)

1. Phase 1, then Phase 2 with the maintainer's API confirmation.
2. Phase 2.5: value object and decoder.
3. Phase 3: Agent metadata on all three Agent write paths.
4. Stop and validate US1.1–US1.5.

### Incremental Delivery

1. Foundation, then US1 (MVP).
2. US2: strict rejection on every write path of all three resources.
3. US3: Permission Set and Service write semantics and runtime isolation.
4. Phase N: compliance and the full gate. The feature ships in one PR with all three stories.

---

## Notes

- [P] tasks touch different files and have no dependency on an incomplete task.
- Commit after each task or logical group.
- Do not commit or push without a request.
