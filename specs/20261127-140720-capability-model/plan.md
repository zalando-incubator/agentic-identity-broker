# Implementation Plan: Capability Model for Permission Sets

**Branch**: `speckit/issue-579` | **Date**: 2026-11-27 | **Spec**: [spec.md](spec.md)

**Input**: Feature specification from `specs/20261127-140720-capability-model/spec.md` (issue #579, part 1 of #285)

## Summary

Operators define **capability groups** that own **capabilities**. Each capability has:

- a required, globally unique canonical ID;
- a `read_only` flag;
- one or more service scopes.

Permission sets include capability entries instead of service scopes, and agent entries may tighten them. Grants store explicit granted capability IDs. Token exchange unions the granted capabilities' scopes per service under today's service-requirement ceiling, and fails closed on stale or undeclared capabilities with the existing responses. The ADR `048-authzen-delegation-pip` permit gains a required `granted_capabilities` member, which enabled-mode ExtProc passes to OPA as `input.context.granted_capabilities`. Disabled mode and the token-exchange response do not change.

Atomic migration `050` converts each legacy set into a group with one capability per former service scope and rewrites grants to the matching capability IDs, so no grant is broadened. Its down migration refuses unrepresentable data. Admin API moves to `4.0.0`, and end-user API moves to `1.4.0` (additive). The consent screen and end-user grant request keep their current shape.

## Technical Context

- **Language/Version**: Go (version from `go.mod`); Rego for the sample policy.
- **Primary Dependencies**: chi v5, sqlx and pgx v5, the embedded OPA SDK (ExtProc), OpenTelemetry (`otelslog`). No new dependency.
- **Storage**: PostgreSQL with four new tables (`capability_groups`, `capabilities`, `capability_service_scopes`, `permission_set_capabilities`). `permission_set_service_scopes` is dropped. `agents.permission_sets` and `user_grants.granted_permission_sets` change JSONB element shape. In-memory adapter parity is required.
- **Testing**: Go `testing` with testify for unit tests, `tests/integration/migrations` (testcontainers) for migrations, Ginkgo/Gomega backend E2E (`tests/e2e/`), the ExtProc E2E suite (`tests/e2e/extproc/`), and `opa test` for the sample policy.
- **Target Platform**: Linux containers (broker, migration image, standalone ExtProc).
- **Project Type**: Go monorepo service with a standalone ExtProc binary. The React SPA is unchanged.
- **Performance Goals**: Exchange and permit latency stay within today's budgets. Capability definitions are read through the same TTL cache pattern as permission sets (`permissionset.Service`), so there is no extra query per exchange on a warm cache.
- **Constraints**: Fail closed on every invalid or stale reference (SR-003). Disabled-mode OPA input is unchanged byte for byte. Migrations are atomic. Queries list explicit columns. No principal appears in metrics, and principals appear in logs only as `id.Principal`.
- **Scale/Scope**: No upper bound on capability entries per set (FR-003). Migration volume is one capability per existing `permission_set_service_scopes` row.

No technical unknowns remain. Phase 0 recorded all design choices in [research.md](research.md).

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-checked after Phase 1 design (below).*

**Design Preconditions (BLOCKING)**:

- [x] **Domain Model**: Entities, aggregate, value objects and derived views are documented in [data-model.md](data-model.md) and the spec's Domain Model.
- [x] **Domain Concepts**: Capability, Capability Group, Capability Entry and Effective Capability are added to the `ARCHITECTURE.md` glossary. PermissionSet, ServiceScope, AgentPermissionSetEntry and GrantedPermissionSetEntry are revised (Phase 2a).
- [x] **Entity IDs**: `CapabilityGroupID` and `CapabilityID` are added via `gen_ids.go` and documented in `internal/domain/id/AGENTS.md` (ADR `013-strongly-typed-entity-ids`).
- [x] **Configuration Design**: There is no new parameter (research R17). The feature relies on the existing `grant_context.enabled` and `authorization.enabled`.
- [x] **Config Examples**: Only the header comment of `examples/config/extproc-grant-context.yaml` changes, to list the new member. The new sample policy goes in `examples/policies/`.
- [x] **Helm Deployment Contract**: No chart-managed parameter changes. ExtProc stays outside the chart under ADR `011-extproc-standalone-binary`.
- [x] **API Design First**: [contracts/admin-api.md](contracts/admin-api.md) and [contracts/policy-input.md](contracts/policy-input.md) go into the OpenAPI files in Phase 2c, before handlers.
- [x] **API Documentation**: Changes go to `api/admin/openapi.yaml` (4.0.0) and `api/enduser/openapi.yaml` (1.4.0). The end-user rendering goes to `docs/api/authzen-evaluation.md`.
- [ ] **API Changes**: Stakeholder confirmation of both contracts is **pending**. It is recorded in the PR (see Stakeholder decisions).
- [x] **Database Design**: Migration pair `050_capability_model` uses go-migrate naming ([contracts/migration-050.md](contracts/migration-050.md)).
- [x] **E2E Acceptance Tests**: Every scenario US1.1–US3.6 is written before implementation (Phase 2f).
- [x] **E2E Test Mapping**: Each scenario maps 1:1 to one `It()` (table below).
- [x] **E2E Red Phase**: Assertions target real responses and policy decisions. There are no placeholder failures and no `Skip`, `XIt` or `PIt`.
- [x] **Frontend Playwright E2E**: Not applicable, because the React UI is unchanged (FR-013). Existing `tests/e2e/frontend/consent_flow_test.go` seeding moves to capability entries only where it seeds via the Admin API.
- [x] **Frontend Screenshots**: Not applicable, because there is no UI change.

**Implementation Considerations**:

- [x] **Security-First**: Every validation and resolution failure is closed, with no fallback to "all capabilities" or the set's services. The new permit member is required, so a version mismatch fails closed. SR-002 controls are stated in the ADR and guide.
- [x] **Architecture Docs**: `ARCHITECTURE.md` is updated: glossary, data model and storage tables (§ 1216–1218), Grant Context and ExtProc input (§ 1029, 1633), API versions.
- [x] **ADRs**: New ADR `20261127-190555-capability-model` (Proposed). It is to be Accepted in this feature's PR and extends ADR `048-authzen-delegation-pip` §§ 4–5 by addition (research R13).
- [x] **Library-First Security**: No cryptography is added. `md5` in migration SQL derives a stable identifier only, not a security property.
- [x] **Zalando Guidelines**: Kebab-case paths, snake_case members, full-replacement `PUT`, and the existing error envelope and `Prefer` handling.
- [x] **End-User Docs**: `docs/api/authzen-evaluation.md`, `docs/guides/token-exchange-gateway.md` and `docs/guides/manage-agents-and-services.md` (Admin API 4.0.0 migration) are updated.
- [x] **Migration Testing**: Up, down, re-up, abort and guard tests run in `tests/integration/migrations/`.
- [x] **Hexagonal Architecture**: The new `domain/capability` service depends only on ports. Permission-set validation uses a narrow `CapabilityResolver`. Handlers call domain services only. Effective-capability logic is a domain function (research R1, R4).
- [x] **Persistence Patterns**: ISP repositories go in `internal/ports/storage.go`, with memory and PostgreSQL adapters, sqlx, `StorageError` wrapping and testcontainers tests.

**Gate result**: All design gates pass except stakeholder confirmation of the API contracts, which Principles IV and X require **before implementation**, not before planning. Planning may continue, but implementation must not start until confirmation is recorded.

### Post-design re-check

- [x] No new configuration, no chart change, and no UI change were introduced by Phase 1.
- [x] Disabled-mode policy input and the token-exchange response remain unchanged (contracts/policy-input.md §§ 2, 4).
- [x] The migration stays atomic, and no ADR `039-concurrent-session-expiry-index` exception is extended.
- [x] Every new error path reuses an existing wire response (research R7).
- [x] No telemetry instrument is added. The audit-event field changes follow LOGGING.md ([contracts/observability.md](contracts/observability.md)).

## Stakeholder decisions before implementation

| # | Decision for approval | Where |
|---|---|---|
| D1 | Admin API 4.0.0 contract: capability-group resources, `capabilities[]` on sets and agent entries, `service_scopes` rejected | [contracts/admin-api.md](contracts/admin-api.md) |
| D2 | End-user API 1.4.0: required permit member `granted_capabilities`; broker-before-ExtProc upgrade order | [contracts/policy-input.md](contracts/policy-input.md) |
| D3 | Set update rejected with `409` when it would invalidate an agent entry; allowed when only grants depend on the dropped capability | research R3 |
| D4 | Granted capability no longer declared by the agent fails closed with the existing undeclared-set response | research R7 |
| D5 | Consent rejects an included service that yields no capability (`400`) | research R5 |
| D6 | Guarded down migration that refuses unrepresentable data | [contracts/migration-050.md](contracts/migration-050.md) |
| D7 | ADR `20261127-190555-capability-model` moves from Proposed to Accepted | `adrs/` |

## Project Structure

### Documentation (this feature)

```text
specs/20261127-140720-capability-model/
├── spec.md
├── plan.md              # this file
├── research.md          # Phase 0
├── data-model.md        # Phase 1
├── quickstart.md        # Phase 1
├── contracts/
│   ├── admin-api.md
│   ├── policy-input.md
│   ├── migration-050.md
│   └── observability.md
├── checklists/requirements.md
└── tasks.md             # /speckit-tasks (not created here)
```

### Source code and documentation affected

```text
adrs/20261127-190555-capability-model.md                 # new ADR
ARCHITECTURE.md                                          # glossary, storage, Grant Context, ExtProc input, API versions
api/admin/openapi.yaml                                   # 4.0.0
api/enduser/openapi.yaml                                 # 1.4.0 (GrantContext.granted_capabilities)
migrations/050_capability_model.{up,down}.sql

internal/domain/id/gen_ids.go, uuid_ids_gen.go, AGENTS.md    # CapabilityGroupID, CapabilityID
internal/domain/storage/capability.go (new), permission_set.go, agent.go, user_grant.go
internal/domain/capability/                              # new bounded context: service, validation, audit
internal/domain/permissionset/service.go                 # capability entries, EffectiveCapabilities, R3 checks
internal/domain/agents/service.go, reconcile.go          # agent-entry capability validation, coverage via capabilities
internal/domain/consent/service.go                       # FR-010 derivation, derived service view (R5, R6)
internal/domain/tokenexchange/grant_resolver.go, evaluation.go, service.go   # R7, permit projection
internal/ports/storage.go                                # new and changed repositories
internal/adapters/storage/memory/                        # capability repos, changed set, grant and agent repos
internal/adapters/storage/postgres/                      # same, sqlx, serializable delete checks
internal/adapters/storage/factory.go                     # accessors
internal/adapters/http/handlers/admin/capability_groups_handler.go (new), permission_sets_handler.go, agents_handler.go
internal/adapters/http/handlers/consent/                 # only if the read model needs derived services
internal/adapters/http/enduser/authzen_evaluation.go     # body type unchanged; context gains member
internal/adapters/http/routing/admin.go                  # /api/capability-groups routes
internal/app/builder.go                                  # wire capability service and resolvers (Principle XII)

internal/extproc/grantcontext/context.go, client.go      # decode and validate granted_capabilities
internal/extproc/authorization/input.go                  # ContextInput.GrantedCapabilities
internal/extproc/server/grant_context.go                 # copy in contextInputFromGrantContext
internal/extproc/AGENTS.md                               # input member list

examples/policies/capability-tool-map.rego (new)
examples/config/extproc-grant-context.yaml               # header comment only
scripts/setup-dev-data.sh                                # Admin API 4.0.0 request bodies
docs/api/authzen-evaluation.md, docs/guides/token-exchange-gateway.md, docs/guides/manage-agents-and-services.md
docs/reference/telemetry.md                              # regenerated by just telemetry-doc if it changes

tests/integration/migrations/capability_model_migration_test.go (new)
tests/e2e/capability_groups_test.go (new), capability_model_upgrade_test.go (new), capability_policy_journey_test.go (new)
tests/e2e/permission_sets_test.go, fixtures/grants.go, canonical_resource_ids_test.go, grant_diagnostics_test.go,
  frontend/consent_flow_test.go                          # seed through capability entries
tests/e2e/extproc/fixtures/policies/grant_context_disabled_input.rego   # forbid granted_capabilities
tests/e2e/extproc/bootstrap/authzen_broker.go            # mock permit carries granted_capabilities
```

**Structure Decision**: This is the existing hexagonal monorepo layout. One new bounded context, `internal/domain/capability`, is added. Everything else extends existing packages.

## Design and Implementation Approach

1. **Model and storage (Phase 2.7 scaffold, then User Story 1)**
   - Typed IDs, storage entities, ports, memory and PostgreSQL adapters, and migration `050`.
   - Capability-group repositories keep a group and its capabilities in one aggregate write. The PostgreSQL `Update` replaces capability scopes and diffs capabilities by ID inside one transaction.
   - Deletes re-check references in a serializable transaction (research R3).
2. **Capability service and Admin API (User Story 1)**
   - `capability.Service` enforces the group invariants and emits audit events.
   - `permissionset.Service` validates capability entries through `CapabilityResolver` and rejects set updates that would invalidate agent entries.
   - `agents.Service` validates agent-entry lists (FR-005) and computes service coverage through capabilities.
   - Admin handlers decode, convert canonical references and map errors as today.
3. **Grants (User Story 2, US2.4)**: `consent.Service` derives granted capabilities from the included-service request (research R5). The end-user read models derive services (research R6).
4. **Exchange and permit (User Story 3)**
   - `GrantResolver` coverage uses capabilities and reuses the existing failure details (research R7).
   - `project` adds `granted_capabilities`, sorted and de-duplicated.
   - The `TokenExchangeResponse` builder derives `granted_permission_sets` from capabilities.
5. **ExtProc (User Story 3)**: decode and validate the required member, then copy it into `ContextInput` only in enabled mode. Tests guard that disabled mode is unchanged.
6. **Documentation and examples**: the sample policy, guides, ARCHITECTURE.md, ADR acceptance, and `scripts/setup-dev-data.sh`.

## Implementation Phase Overview

| Phase | Purpose | Applies |
|-------|---------|---------|
| **Phase 0** | Pre-implementation refactoring | **Skip.** No behavior-neutral restructuring is needed. Changes are additive or replace fields in place. |
| **Phase 1** | Setup | Typed ID generation only |
| **Phase 2** | Design Preconditions: domain model, config, API, DB, E2E, observability | **MANDATORY**: 2a, 2b, 2c, 2d, 2f, 2g. 2e skipped: no UI |
| **Phase 2.7** | Entity boilerplate | **Include.** Capability-group ports, both adapters and 501 handlers are scaffolded so the structural review is separate from business logic |
| **Phase 2.5** | Foundational infrastructure | Migration `050` and the changed repository shapes for sets, agents and grants. User Stories 1–3 all depend on these |
| **Phase 3** | User Story 1: define capabilities and compose sets (P1) | |
| **Phase 4** | User Story 2: upgrade keeps grants (P1) | |
| **Phase 5** | User Story 3: policy receives capabilities (P1) | |
| **Phase N** | Constitution compliance verification | **MANDATORY** |

- [x] Phase 0 (refactoring): skip, because no structural change precedes the feature.
- [x] Phase 2.7 (entity boilerplate): include, because there are two new entities with new ports, adapters and Admin routes.

## Testing Strategy

### End-to-end acceptance tests

- **Framework**: Ginkgo v2 and Gomega, following `tests/e2e/AGENTS.md` (dual servers via `bootstrap.NewEndUserTestServer` and `NewAdminTestServer`).
- **Policy scenarios**: These run with the real broker and ExtProc with the embedded OPA engine (`bootstrap.NewExtProcEnvironmentWithOptions` with `GrantContext.Enabled`), like `grant_context_journey_test.go`, under `Label("grant-context")`.
- **Disabled mode**: US3.6 starts ExtProc with grant context disabled.

| Spec Scenario | E2E Test Location | Test Description |
|---|---|---|
| US1.1 | `tests/e2e/capability_groups_test.go` | `It("creates a capability group and returns its capabilities, canonical IDs, flags and service scopes")` |
| US1.2 | `tests/e2e/capability_groups_test.go` | `It("rejects invalid capabilities with 400 and stores nothing")` (table entries: missing, duplicate, UUID-shaped, non-URL-safe canonical ID; no service scopes) |
| US1.3 | `tests/e2e/permission_sets_test.go` | `It("stores a permission set composed of capability entries and rejects empty, duplicate or service_scopes requests")` |
| US1.4 | `tests/e2e/agent_permission_requirements_test.go` | `It("accepts agent entries that tighten or subset capabilities and rejects entries that relax or add them")` |
| US1.5 | `tests/e2e/capability_groups_test.go` | `It("refuses with 409 to delete a capability or group that a set or grant references")` |
| US2.1 | `tests/e2e/capability_model_upgrade_test.go` | `It("converts each legacy set into a group with one capability per former service scope")` |
| US2.2 | `tests/e2e/capability_model_upgrade_test.go` | `It("keeps effective scopes and granted_permission_sets for a grant that omitted an optional service")` |
| US2.3 | `tests/integration/migrations/capability_model_migration_test.go` | `TestMigration050DownRestoresPreUpgradeShape` (a migration-level scenario: E2E cannot run a down migration against a live server; DB-003 requires the integration test) |
| US2.4 | `tests/e2e/capability_model_upgrade_test.go` | `It("accepts today's consent request unchanged and yields the pre-upgrade scopes")` |
| US3.1 | `tests/e2e/capability_policy_journey_test.go` | `It("passes granted capability canonical IDs to policy and forwards a mapped tool")` |
| US3.2 | `tests/e2e/capability_policy_journey_test.go` | `It("denies a tool mapped to a capability the grant does not hold before it reaches the MCP target")` |
| US3.3 | `tests/e2e/capability_policy_journey_test.go` | `It("lists a capability included by two granted sets exactly once")` |
| US3.4 | `tests/e2e/capability_policy_journey_test.go` | `It("fails closed with the consent error URI when a granted capability leaves its set")` |
| US3.5 | `tests/e2e/capability_policy_journey_test.go` | `It("keeps decisions of a policy that reads only granted_permission_sets")` |
| US3.6 | `tests/e2e/capability_policy_journey_test.go` | `It("omits granted_capabilities when grant-context mode is disabled")` |

US2.3 maps to an integration test, not an E2E `It()`, because a down migration cannot run under a live broker. This is the only deviation from Principle XIII's 1:1 E2E mapping. It is recorded in Complexity Tracking.

**Red phase**: Tests are written in Phase 2f against the contracts. They compile against the scaffolded API and fail on real status codes, bodies and policy outcomes. No `Skip`, `XIt` or `PIt`, and no red-phase comments.

**Test data**:

- **New fixtures**:
  - `tests/e2e/fixtures/capabilities.go` with the Journey 1 Calendar group, a Drive group, and a capability shared by two sets for US3.3;
  - a legacy seed helper for US2 that writes pre-`050` rows into a `bootstrap.NewPostgresFixtureAtVersion(ctx, 49)` database before migrating and starting the broker;
  - the mock MCP target from existing ExtProc fixtures.
- **Changed fixtures**: `fixtures/grants.go` and other Admin seeders move to capability entries.

**Bootstrap**: Production bootstrap (`app.Builder`) is used. US2 needs a PostgreSQL-backed broker, so it runs under `Label("docker")` like `agent_external_id_upsert_test.go`. It uses `bootstrap.NewPostgresFixtureAtVersion(ctx, 49)` to seed legacy rows, then applies the production migrations to `050` and starts the dual servers on that database.

### Unit tests

- **Domain**:
  - group invariants and canonical-ID rules;
  - `EffectiveCapabilities` (table-driven over FR-005 – FR-007);
  - permission-set R3 checks;
  - agent-entry validation;
  - consent derivation (R5) and derived service view (R6);
  - `resolveGrantCoverage` with stale, undeclared, union and ceiling cases;
  - `project` order and de-duplication;
  - audit-event field sets with credential absence.
- **ExtProc**:
  - `parsePermitContext` cases for absent, null, empty, duplicate and unsorted lists;
  - enabled-mode `ContextInput` JSON;
  - the unchanged disabled-mode exact test.
- **Policy**: `opa test` for `examples/policies/capability-tool-map.rego`.

### Integration tests

- Migration tests per [contracts/migration-050.md](contracts/migration-050.md).
- PostgreSQL repository tests for the new repositories, the changed set, grant and agent queries, serializable delete checks, and unique-violation mapping.

**Coverage goals**: 100% of acceptance scenarios. The domain validation and resolution paths are fully table-tested, and every repository method is integration-tested.

## Complexity Tracking

| Violation | Why Needed | Simpler Alternative Rejected Because |
|---|---|---|
| US2.3 is verified by a PostgreSQL migration integration test, not an E2E `It()` (Principle XIII 1:1 mapping) | A down migration replaces the schema the running broker depends on. DB-003 and the spec's testing note already require the integration test | An E2E test that stops the broker, runs `down` and inspects tables would only re-run the integration test through a slower harness, and would assert no user journey |
