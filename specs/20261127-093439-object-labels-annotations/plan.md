# Implementation Plan: Labels and Annotations on Agents, Services, and Permission Sets

**Branch**: `speckit/issue-427` | **Date**: 2026-11-27 | **Spec**: [spec.md](spec.md)

**Input**: Feature specification from `specs/20261127-093439-object-labels-annotations/spec.md`

## Summary

Add one immutable **Object Metadata** value, a label map and an annotation map, to the existing
Agent, ThirdpartyOAuth2Service, and PermissionSet entities. Operators set it through the Admin API
only. The broker validates it strictly and literally and stores it in two JSONB columns per
table. Admin responses return it with keys in byte order. Nothing at runtime reads it: no
authorization path, token, approval, End-User API response, log, span, or metric. The work extends
existing write paths, so there is no new entity, repository, endpoint, configuration, or
telemetry.

The [contract](contracts/object-metadata.md) defines the shape, rules, fixed error messages, and
write matrix. [admin-openapi-metadata.yaml](contracts/admin-openapi-metadata.yaml) holds the
OpenAPI fragments. [research.md](research.md) records the source evidence and the alternatives,
and [data-model.md](data-model.md) records the value object and the persistence design.

## Technical Context

**Language/Version**: Go 1.27.4 (repository toolchain). There are no frontend changes.
**Primary Dependencies**: The standard library's `encoding/json/jsontext` for strict metadata
decoding, which rejects duplicates and invalid UTF-8, and `regexp` for the Kubernetes-compatible
grammar. The existing chi, sqlx/pgx v5, and slog stack. No new module.
**Storage**: Two `JSONB NOT NULL DEFAULT '{}'` columns, with object-type checks, on `agents`,
`thirdparty_oauth2_services`, and `permission_sets`, added by atomic migration `050`. The
in-memory adapter has equivalent fields. Repository interfaces do not change.
**Testing**: Table-driven Go unit tests (testify), handler tests with `httptest`, PostgreSQL
integration and migration tests against the production chain, and Ginkgo E2E with the buffered
JSON logger, span recorder, and metric reader.
**Target Platform**: The broker's Admin server (`:14000`). The End-User server (`:8000`) and
ExtProc do not change.
**Project Type**: Backend only, in the existing Go monorepo.
**Performance Goals**: No extra queries. Metadata is read in the same row scans as the rest of
each entity. A worst-case Agent carries about 90 KB of metadata. No new performance promise.
**Constraints**: At most 64 labels. At most 65,536 decoded UTF-8 bytes of annotations. Fixed error
messages that never contain submitted text. No normalization. The Agent PUT and collection PUT
body caps rise from 64 KiB to 512 KiB (research §5).
**Scale/Scope**: 15 acceptance scenarios, three resources, seven Admin write operations, and two
OpenAPI documents, of which only the Admin document changes (3.0.0 → 3.1.0).

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-checked after Phase 1 design.*

| Principles | Decision and remaining gate |
|---|---|
| I, III | Validation fails closed. Strict `jsontext` decoding prevents silent last-wins resolution and U+FFFD replacement. The reserved prefix is enforced on every Admin write path. No cryptography is involved. |
| II, V | New ADR `adrs/<UTC creation stamp>-object-labels-annotations.md` (SR-005). It records the model, rules, reserved prefix, write semantics, body caps, and no runtime effect. It extends the field policy of `047-unique-agent-external-id-reconcile` and does not deviate from it (research §7). Add **Object Metadata**, **Label**, and **Annotation** to the `ARCHITECTURE.md` glossary. Update the §4 data-store columns and the ADR index there and in the root `AGENTS.md`. |
| IV, X | `contracts/` is the API draft. Maintainer confirmation (API-004) is a blocking Phase 2 precondition. After confirmation, publish it in `api/admin/openapi.yaml` as 3.1.0. `api/enduser/openapi.yaml` stays at 1.3.0. Document the user-facing behavior in `docs/guides/manage-agents-and-services.md`. |
| VI, XII | Adapters parse JSON shape and presence. The domain value object enforces the rules. The domain services own field policy and no-op detection. There is no handler-to-repository shortcut, new service, or builder change. |
| VII | No runtime configuration, so no changes to `internal/ports/config.go`, `examples/config/`, Helm, or ExtProc configuration. |
| VIII, XIII | Unit tests and 15 compiling, semantically failing `It()` blocks (one per scenario) come before behavior. No pending tests, skipped tests, or red-phase comments. |
| IX | One atomic migration pair with object-type checks. Both adapters change. PostgreSQL integration tests run up, down, and up on the production chain. |
| XI, XIV | No UI change, so no Wireloom or Playwright work. The spec already has four HTTP journeys that agree with the contract. No configuration or JWT examples apply. |
| XV | No new log, span, or metric. Fixed error strings keep the existing `WarnContext(..., "error", err)` calls free of metadata. E2E and unit tests assert that sentinel keys and values are absent from logs, audit records, spans, and metric attributes. `just telemetry-doc` runs after the Go changes. |

**Precondition status**:

| Precondition | Status |
|---|---|
| Domain model (value object, embedding, no new entity or typed ID) | Done: data-model.md |
| Glossary terms | Done: **Object Metadata**, **Label**, and **Annotation** in `ARCHITECTURE.md` §12 and the root `AGENTS.md` Domain Glossary |
| Configuration design | Not applicable, confirmed (T005): no configuration parameter. `internal/ports/config.go`, `examples/config/`, `examples/config/README.md`, `charts/agentic-identity-broker/`, and ExtProc configuration are unchanged |
| API design | **Confirmed (API-004)**: the maintainer confirmed `contracts/object-metadata.md` and `contracts/admin-openapi-metadata.yaml` as written, including the §4 messages, the §5 write matrix, and the §6 512 KiB caps. Given on 2026-11-27 in the `/speckit-implement` session for this branch (`speckit/issue-427`), in answer to an explicit confirmation question, and recorded in the [PR #577 confirmation comment](https://github.com/zalando-incubator/agentic-identity-broker/pull/577#issuecomment-7219784410). Published in `api/admin/openapi.yaml` 3.1.0 |
| Database design | Done: data-model.md, migration `050` (still the next unused number when written) |
| E2E design (1:1 mapping) | Done: table below. Tests are in `tests/e2e/object_metadata_test.go` |
| Frontend, Playwright, screenshots | Not applicable, confirmed (T010): `web/` and `api/enduser/openapi.yaml` do not change, so no design-system review, Wireloom, Playwright, or screenshot work applies |
| Observability (T017) | No log event, span, attribute, or metric is added or changed. Sentinel-absence assertions cover the existing audit records `agents.agent.created`/`updated`, `permissionset.permission_set.created`/`updated`, `service.thirdparty.provider_created`/`updated`, the branch records `service.thirdparty.discovered_service_created`/`updated`, `service.thirdparty.cimd_public_service_created`/`updated`, and `service.thirdparty.cimd_confidential_service_created`/`updated`, and the Admin handler `WarnContext(..., "error", err)` validation-failure calls. A metadata rejection uses that same diagnostic pattern: one constant `Warn` message, `metadata validation failed`, with the fixed error and no `event` attribute (`coding-standards/LOGGING.md` §2.2) |

**Gate outcome:** No violation, and no complexity exception is needed. The maintainer confirmed
the contracts, and the table above records that confirmation. The spec
wording on unknown `metadata` members is corrected (research §12).

**Post-design re-check (Phase 1):** The contracts keep the Admin-only boundary, the additive
versioning, and the existing audit records. The design keeps the hexagonal boundaries. The body
cap change is additive and is part of the API draft. The result is unchanged.

## Project Structure

### Documentation (this feature)

```text
specs/20261127-093439-object-labels-annotations/
├── spec.md
├── plan.md                              # This file
├── research.md                          # Phase 0
├── data-model.md                        # Phase 1
├── quickstart.md                        # Phase 1
├── contracts/
│   ├── object-metadata.md               # Phase 1: rules, errors, write matrix
│   └── admin-openapi-metadata.yaml      # Phase 1: OpenAPI 3.0.3 fragments
├── checklists/requirements.md
└── tasks.md                             # Phase 2 (/speckit-tasks)
```

### Source Code (repository root)

```text
internal/domain/model/
├── object_metadata.go                   # new: ObjectMetadata value object, grammar, limits, reserved prefix
├── object_metadata_test.go              # new: table-driven FR-004–FR-009 cases
└── thirdparty_oauth2_provider.go        # ObjectMetadata, ObjectMetadataProvided; re-check in ValidateFor*/ValidateDiscoveryRequest
internal/domain/storage/
├── agent.go                             # ObjectMetadata field; Validate/ValidateForCreate re-check
└── permission_set.go                    # ObjectMetadata field; validateCommon re-check
internal/domain/agents/reconcile.go      # sameDefinition compares ObjectMetadata.Equal
internal/domain/thirdparty/service.go    # prepareUpdate keeps persisted metadata when not provided
internal/adapters/http/handlers/admin/
├── object_metadata.go                   # new: top-level scan, strict jsontext decode, fixed errors, response type
├── agents_handler.go                    # POST/PUT/collection PUT wiring; POST reads body bytes; 512 KiB caps on both PUTs
├── services_handler.go                  # POST/PUT wiring; POST reads body bytes; presence for PUT
└── permission_sets_handler.go           # POST/PUT wiring; POST reads body bytes
internal/adapters/storage/memory/        # agent, permission-set, service records carry the value
internal/adapters/storage/postgres/      # explicit columns in every read/write; '{}' for empty
migrations/050_object_metadata.{up,down}.sql
api/admin/openapi.yaml                   # 3.1.0; schemas, field-policy row, examples
tests/e2e/object_metadata_test.go        # new: 15 canonical It() blocks
tests/integration/migrations/object_metadata_migration_test.go  # new
ARCHITECTURE.md, AGENTS.md, adrs/<UTC creation stamp>-object-labels-annotations.md
docs/guides/manage-agents-and-services.md
docs/reference/telemetry.md              # regenerate only; commit if changed
```

**Structure Decision**: Backend only, in the existing hexagonal layout. The value object lives in
`internal/domain/model/`, which `internal/domain/storage/` already imports. JSON parsing lives in
the Admin handler package. No new package boundary is needed.

## Implementation Phase Overview

| Phase | Purpose |
|---|---|
| Phase 1, Setup | Read the nearest `AGENTS.md` files, `LOGGING.md`, and `OPENTELEMETRY.md`. Re-check the next unused migration number. |
| Phase 2, Design preconditions | Record the maintainer's API confirmation. Write the ADR and the glossary and architecture updates. Publish the Admin OpenAPI 3.1.0 changes. Write the migration test, then the migration pair. Write the 15 E2E `It()` blocks and the unit tables before any behavior. |
| Phase 2.5, Foundation | `model.ObjectMetadata` and the handler decoder, test-first. These are shared by every story. |
| US1 (P1) | Agent wiring: create, ID-addressed PUT, and collection PUT, the no-op map equality, both adapters, and the body caps. |
| US2 (P1) | Every rejection class on every write path of all three resources, with messages that contain no submitted text. |
| US3 (P2) | Permission Set full replacement, Service keep and replace in `prepareUpdate`, and the runtime-isolation journey. |
| Phase N, Compliance | `just check`, `just telemetry-doc`, migration and PostgreSQL suites, the absence checks for logs, spans, and metrics, documentation, and the full `CONTRIBUTING.md` gate. |

- [x] Phase 0 (refactoring): skip. No structural blocker. The existing presence handling and the
  `IconSVGProvided` precedent already support the Service merge.
- [x] Phase 2.7 (entity boilerplate): skip. No new entity, handler, or repository.

## Security Design

| Risk | Control | Verification |
|---|---|---|
| The stored metadata differs from what was sent, which would let someone evade a future policy | `jsontext` rejects duplicates, invalid UTF-8, and unpaired surrogates. No trimming, folding, or normalization. A misspelled or repeated `metadata` member is rejected. | Handler tables with duplicate, U+FFFD, surrogate, whitespace, and case-variant inputs; US2.1 and US2.5 |
| Broker-defined keys are spoofed | Reserved-prefix rejection in both maps on all seven write operations | Unit table; US2.2 covers all three resources and collection PUT |
| Submitted text leaks through an error, log, or telemetry signal | Fixed messages. The `jsontext` error text is discarded. Audit records log named fields only. No new attributes. | US2.4, plus sentinel-absence checks over logs, spans, and metric attributes (SC-006) |
| Metadata reaches end users or policy | End-user DTOs are explicit, and no consumer reads `ObjectMetadata` | US3.5 compares consent, token claims, and approval matching. Repository search shows no `ObjectMetadata` read outside Admin handlers and adapters |
| Partial writes | Validation precedes discovery, branch-key, encryption, and repository calls. The whole write fails. | Service and Agent tests with a valid change and invalid metadata in the same request |
| Partly loaded entities overwrite stored metadata | Every row scan loads both columns. Agent writes go only through the domain service. | Adapter column-list review, plus memory and PostgreSQL round-trip tests |
| Stored metadata breaks OAuth2 client resolution or token exchange (FR-021) | Rules are write policy. Reads rebuild the value with `RestoreObjectMetadata`, which applies no rule, and fail only when a column is not a JSON object of strings. The entity re-check omits the reserved prefix. | PostgreSQL tests read a stored reserved-prefix key through `GetByClientID` and `GetByClientURI` |
| The metadata scan changes how POST bodies decode | The three POST handlers read the body bytes, then decode with `json.NewDecoder(bytes.NewReader(body))`. The scan reads only the first JSON value. | Handler tests assert unchanged decode errors and trailing-byte treatment on all three POSTs |
| Resource cost | 64 labels, 65,536 annotation bytes, and a 512 KiB cap on the two capped Agent paths | US2.3 boundary pair |

## Testing Strategy

### End-to-End (E2E) Acceptance Tests

**Test Location**: `tests/e2e/object_metadata_test.go`. Each `It()` carries a comment
`// Scenario USx.y from specs/20261127-093439-object-labels-annotations/spec.md`.

**Framework**: Ginkgo/Gomega under `tests/e2e/README.md`, with production bootstrap through
`tests/e2e/bootstrap/`, a fresh server and storage per test, and fixtures from
`tests/e2e/fixtures/`.

| Spec Scenario | Suite | Canonical `It()` |
|---|---|---|
| US1.1 | Backend | `should return submitted labels and annotations on create, read, and list` |
| US1.2 (and SC-002: 100 shuffled replays) | Docker (PostgreSQL) | `should treat a reconcile with reordered metadata keys as a no-op without write or audit` |
| US1.3 | Backend | `should update and audit a label-only change without metadata in the audit record` |
| US1.4 | Backend | `should store omitted or empty metadata maps as empty on every Agent write path` |
| US1.5 | Backend | `should reject null metadata labels or annotations on every Agent write path without change` |
| US2.1 | Backend | `should reject invalid label keys and values on all three resources and store nothing` |
| US2.2 | Backend | `should reject reserved-prefix keys and accept look-alike prefixes on all three resources` |
| US2.3 | Backend | `should accept 64 labels and 65536 annotation bytes and reject one more` |
| US2.4 | Backend | `should name the map and rule without echoing the submitted key or value` |
| US2.5 | Backend | `should keep keys that differ only in case distinct and exactly as sent` |
| US3.1 | Backend | `should return Permission Set labels and annotations on create, read, and list` |
| US3.2 | Backend | `should clear Permission Set metadata on a PUT that omits it and advance updated_at` |
| US3.3 | Backend | `should preserve Service metadata on an update that omits it` |
| US3.4 | Backend | `should replace whole Service metadata, clear it with {}, and reject null` |
| US3.5 | Backend | `should leave consent, token claims, approvals, and end-user responses unchanged by metadata` |

**Red-phase requirements**: Every `It()` compiles and asserts concrete status codes, bodies,
`updated_at` values, audit-record counts, and sentinel absence. Minimal request and response
struct fields are acceptable scaffolding. `XIt`, `PIt`, `Skip()`, and red-phase comments are
forbidden. The PostgreSQL scenario US1.2 may take its first red run in CI.

**Test data**: Use the existing agent, permission-set, service, and principal fixtures. Add one
fixture of metadata sentinels: distinctive keys and values, plus generators for the 64-label and
65,536-byte boundaries. For US3.5, reuse the existing consent, token-exchange, and approval
bootstrap. Run the same journey with metadata and without it, and compare the outcomes.

**Helpers**: `bootstrap.NewBufferedJSONLogger`, `bootstrap.NewInMemoryTracerProvider`, and an
`sdkmetric.ManualReader` as in `tests/e2e/session_metrics_test.go`. New
`tests/e2e/helpers/object_metadata.go` holds the key-order shuffle and one sentinel-absence check
over all three signals, used by every story. No new matcher is needed.

### Unit & Integration Tests

- **Domain** (`internal/domain/model/object_metadata_test.go`): table-driven tests of every
  FR-005 to FR-009 class, the exact boundaries (63 and 64 characters, 253 and 254 prefix
  characters, 64 and 65 labels, 65,536 and 65,537 bytes), NUL, multi-byte UTF-8 counting, the
  reserved prefix and its look-alikes, copy isolation, and `Equal`.
- **Domain** also covers `RestoreObjectMetadata`, which applies no rule, and `Validate`, which
  omits the reserved prefix.
- **Domain services**: Agent `sameDefinition` with reordered and changed maps. Service
  `prepareUpdate` with omitted, sent, and empty metadata, and no discovery or encryption call
  when metadata is invalid. Permission Set full replacement. Audit records contain no metadata
  sentinels.
- **Handlers**: shape, `null`, unknown members, duplicates, invalid UTF-8, the spelling and
  repetition of the top-level `metadata` member, the fixed message for each contract §4 row,
  existing error precedence, the 512 KiB caps, and response omission and ordering.
- **Adapters**: round-trips in memory and PostgreSQL, `'{}'` and never `null`, column coverage
  for every Agent read method, fail-closed reads of a column that is not a JSON object of
  strings, and unchanged reads of a stored reserved-prefix key.
- **Migrations** (`tests/integration/migrations/object_metadata_migration_test.go`): up, down,
  and up again on the production chain. Pre-migration rows read as empty. The object-type checks
  reject a JSON `null`. Unrelated data is preserved.

**Coverage goals**: every validation rule and write-matrix cell, both adapters, and 100% of the
acceptance scenarios.

## Complexity Tracking

No constitution violation. No new dependency, package, repository, endpoint, configuration, or
telemetry.

## Future Work (out of scope)

These follow-ups are taken from the spec's Out of Scope list. None is a task for this feature.
- Label selectors and their index.
- Admin UI.
- An ADR that exposes labels to CEL, OPA, or the Grant Context.
- End-user display.
- Metadata on Protected Resources and keys.
