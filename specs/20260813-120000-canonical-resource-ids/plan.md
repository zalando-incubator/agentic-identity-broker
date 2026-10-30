# Implementation Plan: Canonical Resource IDs

**Branch**: `20260813-120000-canonical-resource-ids` | **Date**: 2026-08-13 | **Spec**: [spec.md](spec.md)

**Input**: Canonical IDs supplement UUIDs for admin-managed agents, third-party OAuth2 services, and permission sets. The user confirmed the administrative API change and clarified uniqueness is per resource type/table.

## Summary

Add an optional canonical ID to each in-scope managed resource. Administrative paths and relationship write fields accept a UUID or the target type's canonical ID; API reads keep the stable UUID `id` unchanged and add a nullable `canonical_id`, and nested references remain UUIDs on read. Preserve `AgentID`, `ServiceID`, and `PermissionSetID` as UUID-backed internal identity types under ADR 20260313-120000. The minimal safe reuse of the existing ID design is at the wire resolver boundary: resolve UUID-or-canonical input to the existing typed UUID before domain and persistence operations. PostgreSQL partial unique indexes and matching memory indexes provide per-table concurrency-safe uniqueness. Read responses default to UUIDs; a client MAY send `Prefer: reference-id=canonical` to render nested references as canonical, acknowledged via `Preference-Applied` and `Vary: Prefer`.

## Technical Context

**Language/Version**: Go 1.25.6; OpenAPI 3.0.3; SQL migrations

**Primary Dependencies**: chi v5, sqlx/pgx v5, `github.com/google/uuid`, Ginkgo/Gomega, testify

**Storage**: PostgreSQL in production and in-memory adapters for development/test; UUID primary/foreign keys and JSONB relationship fields

**Testing**: Go unit tests, PostgreSQL integration tests, Ginkgo/Gomega HTTP E2E using production bootstrap

**Target Platform**: Broker HTTP services on macOS/Linux; admin API defaults to port 14000

**Project Type**: Web service with a separate end-user React SPA; no admin SPA exists

**Performance Goals**: Preserve current single-resource admin lookup behavior; canonical resolution adds at most one indexed, type-scoped repository lookup. Canonical presentation (`Prefer: reference-id=canonical`) adds a per-read batch lookup of referenced targets' canonical IDs, incurred only when the preference is sent; the default read path adds no lookups.

**Constraints**:
- A canonical ID is a non-empty, case-sensitive, opaque, non-UUID token of 1–128 characters using only `A–Z`, `a–z`, `0–9`, `.`, `_`, and `-`; storage may use `VARCHAR(255)` without expanding accepted input.
- Canonical IDs are unique only within `agents`, `thirdparty_oauth2_services`, or `permission_sets`; the same text may occur in different tables.
- UUIDs remain the only internal, relational, JSONB, and encryption-context identifiers.
- All reference resolution must complete before persistence; rejected writes must not partially mutate relationships.
- Existing UUID-only administrative clients remain valid.

**Scale/Scope**: Three managed resource types, five direct administrative path families (including agent client-credential paths and the service protected-resource sub-resource paths), one list filter, three supported write-reference fields, and one opt-in read presentation preference across the in-scope GET/list operations.

## Constitution Check

*GATE: Passed for planning. The following preconditions are planned as mandatory implementation tasks; implementation remains blocked until their API, database, and red-phase test tasks are complete.*

### Design Preconditions

- [x] **Domain Model**: `data-model.md` defines canonical IDs, managed resources, stable identifier semantics, relationships, and invariants.
- [x] **Domain Concepts**: Add `Canonical ID` and `Stable Identifier`; amend Agent, ThirdpartyOAuth2Provider, PermissionSet, ServiceScope, and AgentPermissionSetEntry glossary definitions in `ARCHITECTURE.md` during implementation.
- [x] **Entity IDs**: No new UUID-primary-key entity is introduced. ADR 20260313-120000 types remain UUID-backed; canonical ID is optional metadata, not a replacement typed ID.
- [x] **Configuration Design**: No runtime configuration changes.
- [x] **Config Examples / Helm Chart**: Not applicable; no configuration changes.
- [x] **API Design First**: [contracts/admin-canonical-ids.md](contracts/admin-canonical-ids.md) defines the confirmed admin API delta before implementation.
- [x] **API Documentation**: Update `api/admin/openapi.yaml` from the contract before handlers.
- [x] **API Changes**: The user is the API stakeholder and confirmation is recorded in `spec.md`.
- [x] **Database Design**: One reversible go-migrate pair adds nullable columns and per-table partial unique indexes; no global registry.
- [x] **E2E Acceptance Tests**: `tests/e2e/canonical_resource_ids_test.go` is planned before production implementation.
- [x] **E2E Test Mapping**: Ten acceptance scenarios are mapped below, one `It()` each.
- [x] **E2E Red Phase**: Tests will compile and fail semantically against real admin HTTP output before implementation.
- [x] **Frontend Playwright / Screenshots**: Not applicable; no admin React client and no consent SPA changes.

### Implementation Considerations

- [x] **Security-First**: UUID parsing remains safe; canonical resolution is type-scoped and authenticated admin handlers preserve authentication-before-input-validation ordering.
- [x] **Architecture Docs**: Update glossary only; no architectural boundary changes.
- [x] **ADRs**: No new ADR required because UUID-backed typed IDs remain compliant with ADR 20260313-120000. Changing those ID structs to physically store canonical text is rejected by [research.md](research.md) and would require an accepted superseding ADR.
- [x] **Library-First Security**: UUID syntax validation continues to use the vetted `google/uuid` dependency; no cryptography changes.
- [x] **Zalando Guidelines**: Document path/query/request/response/error changes in OpenAPI.
- [x] **End-User Docs**: Not applicable; this is admin API-only.
- [x] **Migration Testing**: PostgreSQL integration tests cover apply/rollback and unique-index behavior.
- [x] **Hexagonal Architecture**: Domain services depend on focused repository resolver/validation ports; HTTP handlers only parse and delegate; adapters implement storage semantics.
- [x] **Persistence Patterns**: Implement matching repository behavior in memory and PostgreSQL adapters, with domain error mapping.

## Project Structure

### Documentation (this feature)

```text
specs/20260813-120000-canonical-resource-ids/
├── spec.md
├── plan.md
├── research.md
├── data-model.md
├── quickstart.md
└── contracts/
    └── admin-canonical-ids.md
```

### Source Code

```text
api/admin/openapi.yaml
migrations/029_add_canonical_ids.{up,down}.sql
internal/
├── domain/
│   ├── model/thirdparty_oauth2_provider.go
│   └── storage/{agent.go,permission_set.go,...}
├── ports/{storage.go,thirdparty_provider.go}
└── adapters/
    ├── http/handlers/admin/
    │   ├── agents_handler.go
    │   ├── services_handler.go
    │   ├── permission_sets_handler.go
    │   ├── protected_resources_handler.go
    │   └── client_credentials_handler.go
    └── storage/
        ├── memory/
        └── postgres/
tests/
├── e2e/canonical_resource_ids_test.go
└── integration/ or internal/adapters/storage/postgres/*_test.go
```

**Structure Decision**: Extend established domain → ports → adapters flow. Keep UUID typed IDs and persisted relationships intact; add canonical metadata and type-scoped lookup/validation through the existing repositories.

## Implementation Phase Overview

| Phase | Decision | Rationale |
|---|---|---|
| Phase 0: Refactoring | Skip | Boundary resolvers and repository additions can be made directly without a behavior-preserving structural extraction. |
| Phase 1: Setup | Include | Update generated API contract and fixture/test helpers only as required for red-phase tests. |
| Phase 2: Design Preconditions | Include | Required domain glossary, OpenAPI, migration, adapter parity, and E2E design work. |
| Phase 2.5: Foundational infrastructure | Include | Canonical value validation, resolver methods, per-table indexes, and memory/postgres repository parity are prerequisites for stories. |
| Phase 2.7: Entity boilerplate | Skip | No new entity, port family, or CRUD handler is introduced. |
| Phase 3+: User stories | Include | Implement direct paths, references, and representation rules in priority order. |
| Phase N: Constitution compliance | Include | Verify all principles, migration semantics, API contract, and E2E traceability. |

## Design and Implementation Sequence

1. **Contract and glossary**: Update `api/admin/openapi.yaml` exactly per `contracts/admin-canonical-ids.md`; add canonical domain terms to `ARCHITECTURE.md`.
2. **Red tests**: Add the complete E2E suite and targeted unit/integration tests first; prove semantic failure.
3. **Domain model and ports**: Add nullable canonical metadata to Agent, service, and PermissionSet models. Add focused type-scoped canonical lookup and duplicate-exclusion operations without weakening interface segregation. For all three update DTOs, reuse the established raw-field presence pattern so omitted preserves, explicit JSON `null` removes, and a string replaces `canonical_id`; a plain `*string` is insufficient. Test all three states per resource.
4. **Persistence**: Add migration `029_add_canonical_ids` with `canonical_id VARCHAR(255)` and one partial unique index per table. Implement repository reads, writes, conflict mapping, and atomic in-memory canonical indexes. Ensure update/removal/delete correctly replaces or releases indexes.
5. **Resolution and reference integrity**: Implement UUID-first / canonical-fallback resolution in the relevant domain services. Resolve every supported reference to a UUID before storing it. Hold/revalidate service targets transactionally for agent JSONB service requirements; reuse existing transactional reference validation for permission-set relationships.
6. **HTTP and representation**: Replace direct UUID parsing at direct paths, the service protected-resource sub-resource paths (`ProtectedResourcesHandler`), the permission-set service filter, and write references with injected resolver calls. Keep handlers thin. Return the stable UUID `id` plus a nullable `canonical_id` on each resource; nested references stay UUIDs on read by default. Parse the RFC 7240 `Prefer` header in the HTTP adapter; when `reference-id=canonical` is honored, batch-load referenced targets' canonical IDs, render nested references as canonical where present, and set `Preference-Applied`; always set `Vary: Prefer` on in-scope reads.
7. **Verification**: Turn tests green, validate migration rollback, and execute the focused E2E/integration suites plus `just check`.

## Testing Strategy

### End-to-End Acceptance Tests

**Test Location**: `tests/e2e/canonical_resource_ids_test.go`

**Framework**: Ginkgo v2 / Gomega; production admin server bootstrap with isolated storage in each test.

**Scenario Mapping**:

| Spec scenario | E2E test description |
|---|---|
| User Story 1, Scenario 1 | `It("creates each managed resource with canonical and UUID identifiers", ...)` |
| User Story 1, Scenario 2 | `It("manages each resource through canonical ID and UUID", ...)` |
| User Story 1, Scenario 3 | `It("keeps UUID-only resources manageable without a canonical ID", ...)` |
| User Story 1, Scenario 4 | `It("preserves, removes, and replaces canonical IDs on update", ...)` |
| User Story 2, Scenario 1 | `It("resolves every supported cross-resource reference by canonical ID", ...)` |
| User Story 2, Scenario 2 | `It("resolves every supported cross-resource reference by UUID", ...)` |
| User Story 2, Scenario 3 | `It("rejects unresolved references without persisting a partial update", ...)` |
| User Story 3, Scenario 1 | `It("returns stable UUID ids and canonical_id attributes in reads", ...)` |
| User Story 3, Scenario 2 | `It("returns UUID ids with null canonical_id when canonical IDs are absent", ...)` |
| User Story 3, Scenario 3 | `It("renders nested references as canonical under Prefer: reference-id=canonical and sets Preference-Applied/Vary", ...)` |

Each `It()` includes its `// Scenario X.Y from specs/20260813-120000-canonical-resource-ids/spec.md` reference and actual HTTP status/body assertions. Add edge coverage for UUID-shaped IDs (`400`), same-table duplicate (`409`), cross-table reuse (success), unknown path (`404`), removal, deletion, and canonical-ID reuse after deletion.

**Unit Tests**: Table-driven canonical-ID validation; resolver UUID/canonical/not-found behavior; response identifier stability (UUID `id` plus a `canonical_id` attribute); same-table duplicate exclusion on update; `Prefer` header parsing (canonical/uuid/absent/unrecognized) and the resulting nested rendering plus `Preference-Applied`/`Vary` headers.

**Integration Tests**: Real PostgreSQL migration apply/rollback; each partial index; conflict mapping; canonical lookup; JSONB/FK references retaining UUID after canonical removal; transaction-safe reference validation. Memory tests exercise matching indexes under its lock.

**Frontend Playwright E2E**: Not applicable. No admin frontend consumes these routes and the end-user SPA is unchanged.

## Complexity Tracking

| Decision | Why Needed | Simpler alternative rejected because |
|---|---|---|
| Keep UUID typed IDs and add canonical metadata/resolution | Preserves ADR 20260313-120000's typed, storage-safe UUID identity and limits changes to adapter/domain boundaries | Changing existing ID structs into UUID-or-string values would break UUID SQL/JSONB/FK contracts, map identity, and binding ADR 20260313-120000. |
| Three per-table partial unique indexes | User requires uniqueness inside each resource type and concurrent PostgreSQL writes need database enforcement | A global registry contradicts clarified scope; application-only checks race. |
| Keep nested reference schemas shared; relax the identifier field to accept canonical on write | Reads round-trip through writes verbatim and stay backwards compatible while the write side accepts a UUID-or-canonical superset | Resolving read values to canonical or adding read-only `_uuid` companions makes identifiers polymorphic and desymmetrizes read/write for no write-side benefit. |
| Opt-in canonical presentation via `Prefer: reference-id=canonical` (default UUID) | Lets human-facing clients read canonical nested references without a body-schema change while keeping the default byte-for-byte backwards compatible | A canonical-by-default flip is an opt-out breaking change that silently alters header-less clients; a query parameter forks the resource URL and cannot be modeled as a header-varying representation. |