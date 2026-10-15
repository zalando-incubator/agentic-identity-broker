# Implementation Plan: Protected Resource Subresources on Third-Party Services API

**Branch**: `035-protected-resource-subresources` | **Date**: 2026-08-12 | **Spec**: [spec.md](./spec.md)

**Input**: Feature specification from `/specs/035-protected-resource-subresources/spec.md`

## Summary

Expose body-based POST and retained member-addressed PUT add operations, plus remove, rename, and list of a
third-party service's **protected resources** as first-class sub-resources of the existing admin services
API, so a client no longer has to fetch, edit, and resubmit the whole service (resending the client secret and risking lost updates).
The design normalizes protected resources out of the `thirdparty_oauth2_services.protected_resources TEXT[]` column into a child table
`service_protected_resources` whose `resource_uri` is a globally `UNIQUE` primary key — making global
uniqueness (FR-007) a database-guaranteed invariant and each single-resource op an atomic single-row write
(FR-011). The full-service `PUT` keeps accepting the complete set but `protected_resources` becomes
optional (omitted → untouched) and, when it replaces the set, is guarded by an `If-Match`/`version` (ETag)
optimistic-concurrency check so it can never silently erase a concurrent single-resource edit
(FR-011/FR-015). Token-exchange resolution reads the same child table (FR-013). See
[research.md](./research.md) for decisions and rejected alternatives.

## Technical Context

**Language/Version**: Go 1.25.6
**Primary Dependencies**: chi v5 (HTTP), sqlx + pgx v5 (Postgres), go-migrate (migrations), OpenTelemetry
(`otelhttp`/`otelchi`), Ginkgo/Gomega (E2E)
**Storage**: PostgreSQL (prod) + in-memory (dev/test) — both adapters implement the same repository
contract (Constitution IX, ADR 004)
**Testing**: Go `testing` + testify assertions (unit), testcontainers (integration), Ginkgo/Gomega (E2E)
**Target Platform**: Linux server (dual admin `:14000` / end-user `:8000`, ADR 004)
**Project Type**: Web backend (Go hexagonal); no frontend change in scope
**Performance Goals**: single-resource op is one indexed child-table write; resolution is one unique-key
lookup — no regression versus the current GIN containment scan
**Constraints**: fail-closed uniqueness under concurrency; single-resource ops never touch the client
secret or unrelated fields (FR-012); default read/write timeouts (5s/10s) preserved
**Escaped URI addressing**: Handlers capture `{resource}` from the escaped request path as exactly one segment, percent-decode once, and only then apply existing URI validation and normalization. For PATCH, `{resource}` identifies the source URI. Router-decoded values that could turn `%2F` into a route delimiter are not used.
**Scale/Scope**: admin-managed configuration data (tens–hundreds of services, small resource sets per
service). No NEEDS CLARIFICATION — all forks resolved in research.md.

## Constitution Check

*GATE: The design is documented; implementation remains blocked until every incomplete Phase 2 precondition is completed and verified.*

**Design Preconditions (BLOCKING)**:

- [ ] **Domain Model**: Reconcile `ProtectedResource` (child entity, URI identity) and the modified `ThirdpartyOAuth2ProviderEntity` (`Version`, child-backed `ProtectedResources`) from [data-model.md](./data-model.md) with the domain model.
- [ ] **Domain Concepts**: Update the ARCHITECTURE.md **Protected Resources** glossary entry (currently "TEXT[] … GIN index", line ~1256) to the child-table, uniqueness, and optimistic-concurrency model.
- [x] **Entity IDs**: N/A — no new UUID-PK entity. A protected resource's identity **is** its normalized
  URI (spec Assumption §Identity model); the parent already uses `id.ServiceID`. No `gen_ids.go` change
  (ADR 013 not triggered).
- [x] **Configuration Design**: N/A — no new runtime configuration.
- [x] **Config Examples**: N/A.
- [x] **Helm Chart**: N/A — no config parameters added/changed/removed (Principle VII not triggered).
- [x] **API Design First**: OpenAPI authored first —
  [contracts/protected-resources.openapi.yaml](./contracts/protected-resources.openapi.yaml).
- [ ] **API Documentation**: During implementation, merge the feature contract into `api/admin/openapi.yaml`; user-facing usage examples remain tracked in T010.
- [x] **API Changes confirmed by stakeholder**: Stakeholder approved the body-based POST add operation, the retained member-addressed PUT add operation, and full-service `PUT /api/services/{id}` behavior change (`protected_resources` optional; `If-Match` required on replacement; `412`/`428`). This entry records the confirmation.
- [ ] **Database Design**: Finalize migration `028_normalize_service_protected_resources.{up,down}.sql` (go-migrate), including a fail-closed canonicality and collision preflight, from [data-model.md](./data-model.md).
- [ ] **E2E Acceptance Tests**: Write `tests/e2e/service_protected_resources_test.go` before implementation, mapping 1:1 to spec scenarios (see Testing Strategy).
- [ ] **E2E Test Mapping**: Verify each acceptance scenario maps to one `It()`.
- [ ] **E2E Red Phase**: Verify realistic status, response-body, and resolution assertions fail semantically without `XIt` or `Skip`.
- [x] **Frontend Playwright E2E**: N/A — no React UI change (spec actor is admin/API client only).
- [x] **Frontend Screenshots**: N/A.

**Implementation Considerations**:

- [ ] **Security-First**: Implement fail-closed uniqueness/conflict behavior, client-secret isolation (FR-012), existing-admin-middleware authorization (FR-014), and structured non-secret audit events for add/remove/rename (FR-017).
- [ ] **Architecture Docs**: Update ARCHITECTURE.md storage and glossary documentation in this PR (Principle II).
- [ ] **ADRs**: Add **ADR 030 — normalize protected resources** (child table, uniqueness constraint, optimistic-concurrency contract) with Proposed status; it does not self-justify this design.
- [x] **Library-First Security**: N/A — no cryptography touched.
- [x] **Zalando Guidelines**: Use sub-collection resource modeling, `If-Match`/`ETag`, and `409`/`412`/`428` status semantics.
- [ ] **End-User Docs**: Add `docs/api/` examples for the new operations.
- [ ] **Migration Testing**: Add apply/rollback/repeat and fail-closed preflight fixtures in PostgreSQL integration tests (Principle IX).
- [x] **Hexagonal Architecture**: Use handlers → domain service → repository port with domain validation and no port bypass (Constitution VI).
- [ ] **Persistence Patterns**: Implement ISP repository methods, both adapters, `StorageError` wrapping, sqlx, and testcontainers as required by ADR 004 and `specs/004-persistence-layer/quickstart.md`.

*Stakeholder API confirmation is complete. All remaining Phase 2 design, documentation, and red-test preconditions still block implementation.*

## Project Structure

### Documentation (this feature)

```text
specs/035-protected-resource-subresources/
├── plan.md                                   # this file
├── research.md                               # Phase 0 — decisions + rationale
├── data-model.md                             # Phase 1 — entities, child table, migration, ports
├── quickstart.md                             # Phase 1 — validation/run guide
├── contracts/
│   └── protected-resources.openapi.yaml      # Phase 1 — API contract (merged into api/admin/openapi.yaml)
├── checklists/requirements.md                # spec quality checklist (existing)
└── tasks.md                                  # Phase 2 — /speckit-tasks (NOT created here)
```

### Source Code (repository root)

```text
api/admin/openapi.yaml                                             # + sub-resource paths; PUT delta
migrations/
├── 028_normalize_service_protected_resources.up.sql              # new: version col, child table, preflight, backfill, drop array
└── 028_normalize_service_protected_resources.down.sql            # new: reverse
adrs/030-normalize-protected-resources.md                         # new ADR
internal/
├── domain/
│   ├── model/thirdparty_oauth2_provider.go                       # + Version; per-URI validate/normalize reuse
│   └── thirdparty/service.go                                     # + Add/Remove/Rename/ListProtectedResource; Update precondition
├── ports/thirdparty_provider.go                                  # + 4 methods; Update gains expectedVersion
├── adapters/
│   ├── storage/
│   │   ├── postgres/thirdparty_provider.go (+_record.go)         # child-table reads/writes, txn, CAS, unique→conflict
│   │   └── memory/thirdparty_provider.go                         # resourceOwners map + version parity
│   └── http/
│       ├── handlers/admin/protected_resources_handler.go        # new thin handler (PUT/POST add, remove/rename/list)
│       ├── handlers/admin/services_handler.go                   # PUT: optional protected_resources + If-Match
│       └── routing/admin.go                                     # register sub-resource routes
├── app/
│   ├── handlers.go                                              # + ProtectedResources handler field
│   └── builder.go                                               # wire handler (Constitution XII)
tests/e2e/service_protected_resources_test.go                     # new E2E suite (1:1 scenarios)
docs/api/…, ARCHITECTURE.md                                      # docs + glossary
```

**Structure Decision**: existing Go hexagonal layout (domain → ports → adapters → app). The feature adds
one new driving adapter (handler), extends one port + its two storage adapters, adds one migration, and
one new domain entity backed by a child table. No new bounded context.

## Implementation Phase Overview

| Phase | Purpose | Applies? |
|-------|---------|----------|
| **Phase 0** — pre-impl refactoring | isolate structural change | **Skip** |
| **Phase 1** — setup | none needed (existing stack) | Minimal |
| **Phase 2** — Design Preconditions | domain/config/API/DB/E2E | **Mandatory** |
| **Phase 2.7** — entity boilerplate | empty CRUD scaffolding | **Skip** |
| **Phase 2.5** — foundational infra | migrate persistence array→child table | **Include** |
| **Phase 3+** — user stories | US1–US4 business logic | Include |
| **Phase N** — constitution compliance | verification | **Mandatory** |

- [x] **Phase 0 (refactoring): skip** — the array→child-table change alters behavior (uniqueness,
  atomicity) and is inseparable from the feature; it is treated as foundational infrastructure
  (Phase 2.5), not a behavior-neutral refactor warranting a separate no-op-change PR.
- [x] **Phase 2.7 (entity boilerplate): skip** — no new top-level CRUD entity with its own resource
  collection/handlers; protected resources are a sub-collection of the existing service aggregate. The
  single new handler is small and ships with US1.

**Phase 2.5 (Foundational Infrastructure)** — the enabling, story-independent work: migration `027`
(version column + child table + fail-closed preflight + backfill + drop array); port methods + both
adapters reading/writing the child table; `Create`/`Update`/`Get`/`List`/`FindByProtectedResource`
repointed to the child table with the in-memory parity map. Land this before US business logic so the
resolver and full-service path converge on one source of truth.

## Testing Strategy

### End-to-End (E2E) Acceptance Tests

**Test Location**: `tests/e2e/service_protected_resources_test.go`
**Framework**: Ginkgo/Gomega, dual-server bootstrap (`tests/e2e/bootstrap/`, `tests/e2e/AGENTS.md`).
**Organization**: `Describe("Protected Resource Subresources")` → `Context` per user story → one `It()`
per acceptance scenario.

**Scenario Mapping**:

| Spec Scenario | E2E `It()` |
|---|---|
| US1-S1 add unclaimed | `adds a new resource and leaves other fields unchanged` |
| US1-S1 POST add unclaimed | `adds a new resource through POST and leaves other fields unchanged` |
| FR-013 / SC-004 add resolution | `makes a newly added resource resolvable to a new token-exchange attempt` |
| US1-S2 idempotent re-add | `is idempotent when the URI is already owned by the service` |
| US1-S3 cross-service conflict | `rejects a URI owned by another service with 409` |
| US1-S4 invalid URI | `rejects a malformed/relative/empty URI with 400` |
| US1-S5 missing service | `returns 404 when adding to a non-existent service` |
| US2-S1 remove owned | `removes an owned resource and preserves other fields` |
| US2-S2 remove not-owned | `returns 404 removing a URI the service does not own` |
| US2-S3 resolution reflects removal | `stops new token-exchange resolution for a removed URI` |
| US2-S4 remove last | `allows removing the last resource, leaving an empty set` |
| US3-S1 rename to unclaimed | `renames a resource to a new unclaimed URI` |
| US3-S2 rename conflict | `rejects renaming to a URI owned elsewhere/present with 409` |
| US3-S3 rename missing | `returns 404 renaming a resource not on the service` |
| US3-S4 rename to same | `treats rename to the same URI as a no-op success` |
| US4-S1 list | `returns all normalized resource URIs for a service` |
| US4-S2 list missing service | `returns 404 listing resources of a non-existent service` |
| Edge: concurrent claim race | `admits only one of two concurrent claims of the same new URI` (integration) |
| Edge: concurrent different edits | `persists both concurrent edits of different resources` (integration) |
| Edge: full-PUT vs single op | `rejects a stale replacing PUT with 412 and preserves the added resource` |
| Edge: replace without If-Match | `returns 428 when replacing the set without If-Match` |
| Edge: field/secret isolation | `never requires or alters the client secret` |

**Red Phase**: realistic assertions (`Expect(resp.StatusCode).To(Equal(...))`, body/set assertions,
resolution outcomes). No `XIt`/`PIt`/`Skip`, no red-phase comments.

- **Test Data**: reuse `tests/e2e/fixtures/` service/config fixtures; add a second service fixture for cross-service conflict; concurrency + migration edge cases run at integration level against real PostgreSQL (`just test-integration-infra`) so DB constraints/transactions are exercised.

**Audit Tests**: handler tests capture structured records for successful and rejected PUT/POST add, remove, and rename operations, asserting required metadata; normalized URI field(s) after successful validation; null URI field(s) for invalid inputs; and the absence of raw submitted URIs and credential material.

### Unit & Integration Tests

- **Unit**: per-URI validation/normalization and domain-service orchestration
  (`internal/domain/thirdparty/service_test.go`, `model/*_test.go`) — TDD, table-driven.
- **Integration**: both storage adapters against the same contract
  (`internal/adapters/storage/{memory,postgres}/thirdparty_provider_test.go`): uniqueness/atomicity/
  idempotency/version/CAS; migration `027` apply/rollback/repeat + fail-closed preflight fixtures
  (non-canonical value, cross-service duplicate).
- **Coverage goals**: unit — critical paths (validation, conflict/idempotency/not-found, CAS); integration
  — every new/modified repository method on Postgres; E2E — 100% of spec scenarios (Principle XIII).

### Frontend Playwright E2E
N/A — no UI change in scope.

## Complexity Tracking

| Decision | Why needed | Simpler alternative rejected because |
|---|---|---|
| New child table + `UNIQUE(resource_uri)` (drop `TEXT[]`) | FR-007 requires global uniqueness **under concurrency**; the array + handler check-then-write has no structural guarantee | `TEXT[]` + advisory-lock/optimistic protocol keeps uniqueness app-enforced and needs the legacy PUT under the same bespoke discipline; no native `text[]` overlap-exclusion constraint exists |
| `version BIGINT` optimistic-concurrency column + `If-Match` on replacement | FR-011/FR-015: a stale full replacement must not silently erase a concurrent single-resource edit | Bare row lock is last-writer-wins (still loses the delta); `updated_at` equality is not a reliable monotonic token across DB precision / adapter parity |
| Optional `protected_resources` on PUT (`*[]string`) | Lets routine service updates (secret rotation) avoid touching the set, and confines `If-Match` to genuine replacements | Required-always `If-Match` breaks existing non-resource PUT clients; plain `[]string` can't distinguish omitted from explicit-empty |
