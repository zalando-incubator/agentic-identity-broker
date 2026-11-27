# Implementation Plan: Agent Grants (Admin-Issued Delegation to an Agent for Itself)

**Branch**: `speckit/issue-472` | **Date**: 2026-11-27 | **Spec**: [spec.md](./spec.md)

**Input**: Feature specification from `specs/20261127-144837-agent-grants/spec.md`

**Status**: Planned. The maintainer confirms the merged API contract in writing before any
implementation beyond Phase 0 begins (Constitution Principle X, spec API-006), for example in a PR
comment on the contract commit. The other decisions in [Review Items](#review-items) are reviewed
in the PR.

## Summary

An administrator can write, read, replace and delete one **agent grant** per agent: a delegation
of permission sets to the agent for its own use. Each covered service is bound explicitly to one
of that service's service accounts, and the write is refused when it exceeds a bound account's
ceiling. No agent can obtain a token with it yet (#473).

- **Same record, new kind.** The grant is a `user_grants` row with `principal_kind = 'agent'`
  and `principal = <agent UUID>`, so the shared grant model reads it unchanged. Grants and
  sessions gain `principal_kind` as part of their identity
  ([research R2](./research.md#r2--principal-kind-identity-in-storage-human-scope-on-the-existing-ports)).
- **Human ports stay human.** The existing `UserGrantRepository` and `UserSessionRepository`
  become human-scoped by contract in both adapters. Every human path, including
  `/oauth2/authorize`, which reads the grant port directly, therefore cannot see an agent grant.
  No call site changes.
- **One aggregate, one new port.** `consent.AgentGrantService` in the consent bounded context and
  `ports.AgentGrantRepository` (four methods). They write the grant row, its version and its
  bindings in one transaction ([R1](./research.md#r1--where-the-domain-code-lives),
  [R4](./research.md#r4--repository-port-for-agent-grants)).
- **Broker resolves, administrator binds.** The request lists optional sets; the broker adds the
  mandatory ones and computes coverage, then stores the result (copy on write). The broker
  checks bindings for coverage, ownership and mandatory services
  ([R6](./research.md#r6--permission-sets-mandatory-sets-and-coverage-copy-on-write),
  [R7](./research.md#r7--bindings-and-effectively-mandatory-services)).
- **One effective-scope definition.** The resolver's per-service scope computation moves into
  `storage.EffectiveScopes`. The ceiling check uses it, so "the same definition" holds by
  construction ([R8](./research.md#r8--ceiling-check-and-one-effective-scope-definition)).
- **Database-enforced bindings.** Migration `051` adds a composite
  `(service_account_id, service_id) → service_accounts ON DELETE RESTRICT` key. With it, a
  bound account cannot be deleted and a cross-service binding cannot be stored
  ([R3](./research.md#r3--migration-051-principal-kind-binding-storage-constraints)).
- **#471's `409` becomes real.** The binding checker returns the binding agents, and the response
  adds `bound_by_agents` ([R9](./research.md#r9--service-account-delete-guard-and-bound_by_agents)).
- **Admin API `3.2.0`.** `PUT`, `GET` and `DELETE /api/agents/{agent-id}/self-grant` take a strong
  ETag and an optional `If-Match`
  ([contract delta](./contracts/admin-agent-grants-openapi.yaml)).

## Technical Context

**Language/Version**: Go 1.27.4

**Primary Dependencies**: chi v5 (routing), sqlx and pgx v5 (PostgreSQL adapters), golang-migrate
(migrations), `log/slog`, OpenTelemetry API. No new dependency.

**Storage**: PostgreSQL (production) and in-memory (development, tests). Migration
`051_agent_grants_principal_kind.{up,down}.sql` changes `user_grants`, `user_sessions` and
`service_accounts`, and adds `agent_grants` and `agent_grant_service_account_bindings`.

**Testing**:

- Go unit tests with testify: domain storage model, consent service, thirdparty service, memory
  adapters and handler.
- Under the `integration` build tag: PostgreSQL migration and repository-contract tests, and the
  shared contract also on the memory store.
- Ginkgo/Gomega E2E through the production bootstrap, with `Label("docker")` for scenarios that
  depend on store semantics.

**Target Platform**: Linux broker container; Darwin for development.

**Project Type**: Backend Admin API feature in the Go monorepo. No React UI, ExtProc, end-user
API, configuration, Helm or CDK change.

**Performance Goals**:

- A read is one indexed query on the grant row plus one on its bindings.
- A write performs, in one transaction: one agent read, one batched permission-set read, one
  account read per binding, and the grant writes.
- The human-scoped queries add one equality predicate on an indexed leading column. Exchange and
  evaluation latency is unchanged.

**Constraints**:

- No human path may read or change an agent-kind row (FR-017, FR-018, SR-003).
- Exchange and evaluation decide exactly as before (FR-018, FR-020, SC-004).
- No credential is read (SR-004).
- Memory and PostgreSQL behave identically (DB-001, SC-007).
- The migration is atomic (DB-002).

**Scale/Scope**: At most one agent grant per agent, so hundreds in total. Grants and sessions
are user-scale; the `051` constraint rebuild is one scan of each table inside the migration
transaction. All 39 acceptance scenarios are tested in this feature.

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

Verified against [.specify/memory/constitution.md](../../.specify/memory/constitution.md):

**Design Preconditions (BLOCKING)**:

- [x] **Domain Model**: [data-model.md](./data-model.md) documents the `AgentGrant` aggregate, the
  `ServiceAccountBinding` and `PrincipalKind` value objects, the rules in evaluation order, write
  outcomes, preconditions and state.
- [x] **Domain Concepts**: Agent Grant, Service-Account Binding and Principal Kind are added to the
  `ARCHITECTURE.md` glossary and the root `AGENTS.md` glossary table with the definitions of #478.
- [x] **Entity IDs**: No new typed ID. The agent grant is keyed by the existing `id.GrantID` and
  addressed by `id.AgentID` ([R15](./research.md#r15--typed-ids)).
- [x] **Configuration Design**: No configuration parameter (spec Assumptions).
- [x] **Config Examples**: Not applicable; no new keys.
- [x] **Helm Deployment Contract**: No parameter change. The existing migration Job applies `051`.
- [x] **API Design First**:
  [contracts/admin-agent-grants-openapi.yaml](./contracts/admin-agent-grants-openapi.yaml) defines
  the change. It is merged into `api/admin/openapi.yaml` before any handler is implemented.
- [x] **API Documentation**: Admin API only (`api/admin/openapi.yaml`). The end-user API is
  unchanged (API-004).
- [ ] **API Changes**: The maintainer confirms API-AG, API-AG-SHAPE, API-SA-409 and API-VERSION
  in writing once the contract is merged into `api/admin/openapi.yaml`, before the Phase 2.7
  scaffold (Principle X, spec API-006). The confirmation's reference is recorded in `tasks.md`
  and the PR description.
- [x] **Database Design**: `051_agent_grants_principal_kind.{up,down}.sql`, go-migrate naming,
  atomic. The schema is in
  [data-model.md](./data-model.md#postgresql-schema--migration-051_agent_grants_principal_kind).
- [x] **E2E Acceptance Tests**: Planned for all 39 scenarios before implementation
  ([Testing Strategy](#testing-strategy)).
- [x] **E2E Test Mapping**: One `It()` per scenario ([Scenario Mapping](#scenario-mapping)).
- [x] **E2E Red Phase**: The tests assert concrete status codes, `ETag`s, members, messages and
  listings against handlers that first answer `501`.
- [x] **Frontend Playwright E2E**: Not applicable; no UI change.
- [x] **Frontend Screenshots**: Not applicable.

**Implementation Considerations**:

- [x] **Security-First**: Fail closed (SR-002). A missing reference aborts the transaction, and an
  absent grant with `If-Match` is `412`. Human scope is enforced in the store, not by callers
  (SR-003). No credential path (SR-004). The operator is required before any read (SR-001). The
  ADR 045 agent-subject rejection is untouched (SR-005).
- [x] **Architecture Docs**: `ARCHITECTURE.md` gets the glossary entries, the consent-domain
  component entry for `AgentGrantService`, the data-store entry, the Admin API `3.2.0` note and the
  ADR index row.
- [x] **ADRs**: New `adrs/20261127-183359-agent-grants-principal-kind.md`, introduced and accepted in
  this PR ([R14](./research.md#r14--adr)). No accepted ADR is superseded.
- [x] **Library-First Security**: No cryptographic or parsing code; standard library and sqlx only.
- [x] **Zalando Guidelines**: The kebab-case singleton sub-resource is `self-grant`. Members are
  snake_case. Status codes are standard, `PUT` creates or replaces, and `If-Match` is optional
  with strong ETags. The change is additive, as a minor version.
- [x] **End-User Docs**: Admin-only change; no `docs/api/` page.
  `docs/guides/manage-agents-and-services.md` gains a "Grant an agent access for itself" section,
  and `docs/reference/api.md` gets the admin API rows.
- [x] **Migration Testing**: Integration tests cover apply, rollback, reapply, the backfill of
  existing rows and each constraint.
- [x] **Hexagonal Architecture**: The rules, resolution, no-op detection and audit live in
  `internal/domain/consent`, and the shared scope function in `internal/domain/storage`. The
  handler parses input and maps errors. The adapters implement the ports. There is no cross-adapter
  import beyond the existing memory cross-wiring through injected interfaces.
- [x] **Persistence Patterns**:
  - The port is in `internal/ports/storage.go` with four methods.
  - Both adapters use sqlx and `StorageError`.
  - The storage adapter gets an `AgentGrants()` accessor.
  - The human-scope change is covered by a shared contract test on both adapters.
- [x] **Coding Standards (Principle XV)**: The log events and spans are listed in
  [contracts/observability.md](./contracts/observability.md). No amendment is needed and no lint
  rule is suppressed.
- [x] **Feature Scope**: No token hand-out, no exchange or evaluation change, no machine session,
  no `subject_kind`, no UI (FR-020). Two Phase 0 refactors are required by FR-011 and by the
  spec's "reuses its validation". One memory correction is required by User Story 4 Scenario 5
  ([R13](./research.md#r13--memory-store-parity-for-agent-deletion)). One unrelated finding is
  deferred ([Deferred Findings](#deferred-findings)).

**Post-design re-check (after Phase 1)**: All blocking preconditions pass except the API
confirmation, which the maintainer gives in writing before implementation (task T012a). No
violation needs Complexity Tracking.

## Review Items

The maintainer reviews these decisions in the PR. The four API items need the maintainer's written
confirmation before implementation beyond Phase 0 (Principle X); the others do not wait for a
separate approval.

| ID | Type | Decision | Details |
|---|---|---|---|
| API-AG | API | `GET`, `PUT` and `DELETE /api/agents/{agent-id}/self-grant` with the representation, messages, strong ETag and optional `If-Match` of the contract. `PUT` answers `201` without `Location`. `404 agent grant not found`. Unknown members and `granted_permission_sets` in a request are refused. | [contract](./contracts/admin-agent-grants-openapi.yaml), [R11](./research.md#r11--http-adapter-and-request-shape) |
| API-AG-SHAPE | API | The request lists optional sets only. `permission_sets` in the response holds all granted sets, and `granted_permission_sets` is a read-only map. Keys are ordered by ascending UUID. | [R6](./research.md#r6--permission-sets-mandatory-sets-and-coverage-copy-on-write) |
| API-SA-409 | API | The service-account delete `409` uses `ServiceAccountBoundConflict` with `bound_by_agents`. `error` and `message` are unchanged. | [R9](./research.md#r9--service-account-delete-guard-and-bound_by_agents) |
| API-VERSION | API | Raise the Admin API from `3.1.0` to `3.2.0` (additive) | spec API-006 |
| DEC-HUMAN-SCOPE | Decision | The existing grant and session ports are human-scoped by contract. The kind-agnostic guards are named in R2. There is no kind parameter on existing methods. | [R2](./research.md#r2--principal-kind-identity-in-storage-human-scope-on-the-existing-ports) |
| DEC-SCOPES | Decision | Extract `storage.EffectiveScopes` from `GrantResolver`, behaviour-preserving, and use it for the ceiling check | [R8](./research.md#r8--ceiling-check-and-one-effective-scope-definition) |
| DEC-MESSAGES | Decision | Rule order and message texts of the write-time checks | [data-model.md](./data-model.md#resolution-and-invariants-write-time) |
| DEC-MEMORY-CASCADE | Decision | Memory agent deletion removes all grants to the agent, as PostgreSQL's cascade does | [R13](./research.md#r13--memory-store-parity-for-agent-deletion) |
| DB-051 | Decision | Migration `051` as in the data model. DOWN erases every agent grant. | [data-model.md](./data-model.md#postgresql-schema--migration-051_agent_grants_principal_kind) |
| ADR-AG | ADR | New ADR `20261127-183359-agent-grants-principal-kind` | [R14](./research.md#r14--adr) |

## Deferred Findings

These were found during planning and are outside the feature's required work. They are not
planned. The constitution requires asking the user whether to file an issue.

| Finding | Where | Why not in scope |
|---|---|---|
| The memory service repository's `Delete` has no active-grant guard; PostgreSQL's `activeProviderGrantReferenceQuery` has one | `internal/adapters/storage/memory/thirdparty_provider.go:357` | No scenario of this feature deletes a service that a grant covers. A service with accounts is already guarded (#471). |

## Project Structure

### Documentation (this feature)

```text
specs/20261127-144837-agent-grants/
├── spec.md
├── plan.md                               # This file
├── research.md                           # Phase 0
├── data-model.md                         # Phase 1
├── quickstart.md                         # Phase 1
├── contracts/
│   ├── admin-agent-grants-openapi.yaml   # Phase 1: Admin API 3.2.0 delta
│   └── observability.md                  # Phase 1: log events, spans, required tests
├── checklists/requirements.md
└── tasks.md                              # Phase 2 (/speckit-tasks)
```

### Source Code (repository root)

```text
api/admin/openapi.yaml                                  # merge contract delta; version 3.2.0
adrs/20261127-183359-agent-grants-principal-kind.md     # new ADR
ARCHITECTURE.md                                         # glossary, consent domain, data store, admin API, ADR index
AGENTS.md                                               # glossary rows, ADR index row
docs/guides/manage-agents-and-services.md               # "Grant an agent access for itself"
docs/reference/api.md                                   # admin API rows
docs/reference/telemetry-descriptions.yaml              # four span descriptions
docs/reference/telemetry.md                             # regenerated (just telemetry-doc)

migrations/
├── 051_agent_grants_principal_kind.up.sql
└── 051_agent_grants_principal_kind.down.sql

internal/
├── domain/storage/principal_kind.go                    # PrincipalKind
├── domain/storage/agent_grant.go                       # AgentGrant, ServiceAccountBinding
├── domain/storage/effective_scopes.go                  # extracted from GrantResolver (Phase 0)
├── domain/storage/user_grant.go, user_session.go       # PrincipalKind field, Copy
├── domain/tokenexchange/grant_resolver.go              # calls storage.EffectiveScopes (Phase 0)
├── domain/consent/service.go                           # structure rules as package functions (Phase 0)
├── domain/consent/agent_grant_service.go               # workflow, resolution, ceiling, audit
├── domain/thirdparty/service_account_service.go        # BindingAgents, ServiceAccountBoundError
├── ports/storage.go                                    # AgentGrantRepository, sentinels, human-scope docs
├── ports/AGENTS.md                                     # catalogue entry
├── adapters/storage/memory/user_grants.go              # kind-prefixed index, agent-grant store, binding guard
├── adapters/storage/memory/user_session.go             # kind-prefixed index
├── adapters/storage/memory/agent_repository.go         # grant cascade on delete
├── adapters/storage/memory/service_account_repository.go # delete inside binding guard
├── adapters/storage/postgres/user_grants.go            # human predicate, ON CONFLICT target
├── adapters/storage/postgres/user_session.go           # human predicate, ON CONFLICT target
├── adapters/storage/postgres/agent_grant_repository.go # new repository
├── adapters/storage/postgres/service_account_repository.go # 23503 → ErrServiceAccountReferenced
├── adapters/storage/factory.go                         # AgentGrants() accessor, wiring
├── adapters/http/handlers/admin/agent_grants_handler.go
├── adapters/http/handlers/admin/service_accounts_handler.go # bound_by_agents
├── adapters/http/routing/admin.go                      # three routes
├── app/handlers.go                                     # AdminHandlers.AgentGrants
├── app/builder.go                                      # service, handler and binding-checker wiring
└── AGENTS.md                                           # handler list, key storage types

tests/
├── e2e/agent_grants_test.go                            # User Stories 1–3
├── e2e/agent_grants_lifecycle_test.go                  # User Stories 4–5
├── e2e/fixtures/agent_grant.go                         # spec cast: agents, sets, service, accounts, requests
├── e2e/helpers/agent_grant.go                          # PUT/GET/DELETE with If-Match, decode
├── integration/migrations/agent_grants_migration_test.go
├── integration/storage/contract/agent_grant_repository.go   # shared AgentGrantRepository contract
├── integration/storage/contract/principal_kind_scope.go     # shared human-scope contract for grants and sessions
├── integration/storage/agent_grant_repository_test.go       # contracts on the memory store
└── integration/storage/infra/agent_grant_test.go            # contracts on PostgreSQL
```

**Structure Decision**: Existing Go monorepo layout. The domain change stays in the consent
bounded context (`internal/domain/consent`, `internal/domain/storage`), and no domain package is
added. One new port interface, one new handler and two new tables.

## Design Notes for Task Generation

**Domain service** (`consent.AgentGrantService`): constructor, methods and rules are in
[data-model.md](./data-model.md#domain-service). The builder passes `b.storage.Agents()`, the
permission-set service, `b.storage.ServiceAccounts()`, `b.storage.AgentGrants()`, `time.Now` and
the logger.

**Order of checks**: operator principal (`401`) → agent (`404`) → `If-Match` syntax (`400`) →
body (`400`) → rules 1–10 (`400`) → precondition (`412`) → no-op check → write. Request errors
therefore win over a failed precondition, as on service accounts. A request without `If-Match`
that loses three successive races answers `412`. `GET` without a grant, and `DELETE` without a
grant and without `If-Match`, → `404 agent grant not found`.

**Wiring**:

- `storage.Adapter.AgentGrants()`.
- Memory: the agent-grant store shares the memory grant store; the agent repository gets the
  grant cascade; the service-account repository gets the binding guard.
- `thirdparty.NewServiceAccountService(..., agentGrantBindings{b.storage.AgentGrants()}, ...)`.
- `AdminHandlers.AgentGrants`; routes are registered only in `SetupAdminRoutes`.

## Implementation Phase Overview

*The detailed task breakdown is in `tasks.md` (generated by `/speckit-tasks`).*

| Phase | Purpose | Required? |
|-------|---------|-----------|
| **Phase 0** | Pre-implementation refactoring: extract `storage.EffectiveScopes`; consent structure rules as package functions | Included |
| **Phase 1** | Setup: read area instructions, reserve migration `051` | Required |
| **Phase 2** | Design Preconditions: ADR, OpenAPI merge, glossary, E2E tests, migration and contract tests | **MANDATORY** |
| **Phase 2.7** | Entity Boilerplate: types, port, empty adapters, `501` handlers, routes, wiring | Included |
| **Phase 2.5** | Foundational Infrastructure: migration `051`, principal kind in both stores (human scope), agent-grant repositories | Required |
| **Phase 3+** | User Stories: US1, US2, US3 (P1); US4, US5 (P2) | Required |
| **Phase N** | Constitution Compliance verification | **MANDATORY** |

- [x] **Phase 0 (refactoring): include.** Both moves are behaviour-preserving and are required by
  the feature: FR-011 requires one effective-scope definition, and the spec's Domain Model says
  the agent grant reuses consent validation. Existing resolver, consent and exchange tests prove
  no change. This can be its own commit or PR ahead of feature work.
- [x] **Phase 2.7 (entity boilerplate): include.** A new aggregate, port, two adapters and a
  handler. The scaffold lands first, so the E2E suite compiles and fails semantically. Whether it
  becomes its own PR is the maintainer's call.

Ordering constraints for tasks:

1. Phase 0 refactors with the existing tests green.
2. ADR; OpenAPI delta merged with `3.2.0`; glossary and `ARCHITECTURE.md`. The maintainer's
   written confirmation of the merged contract precedes step 3.
3. Scaffold, then the E2E tests for all scenarios and the migration and contract tests, failing
   on concrete assertions.
4. Migration `051`, then principal kind in both stores. Every existing test must stay green at
   this point, which proves SC-004 and User Story 5 Scenario 5 early. Then the agent-grant
   repositories.
5. Domain service and handler per user story: US1 → US2 → US3 → US4 (binding checker,
   `bound_by_agents`, memory cascade) → US5.
6. Guide, API reference, telemetry descriptions and `just telemetry-doc`; compliance gates.

## Testing Strategy

### End-to-End (E2E) Acceptance Tests

**Test Location**: `tests/e2e/agent_grants_test.go` (User Stories 1–3) and
`tests/e2e/agent_grants_lifecycle_test.go` (User Stories 4–5)

**Framework**: Ginkgo/Gomega following [tests/e2e/README.md](../../tests/e2e/README.md)

**Test Organization**:

- **Top-level Describe**: `"Agent grants"` in both files.
- **Contexts**: one per user story, with nested contexts for shared preconditions: the spec cast
  with no grants; triage-bot granted; both agents granted.
- **It blocks**: one per acceptance scenario, each preceded by
  `// Scenario N from specs/20261127-144837-agent-grants/spec.md (User Story M)`.

#### Scenario Mapping

The `file:line` column is filled in when the tests are written (Phase 2f). `docker` marks one
`It()` that runs the memory case and the PostgreSQL case
([research R16](./research.md#r16--acceptance-tests-across-stores-and-kinds)).

| Spec Scenario | File | Test Description |
|---|---|---|
| US1.1 | `agent_grants_test.go` | `It("should create the grant with the mandatory set, its coverage, the binding and a strong ETag")` |
| US1.2 | `agent_grants_test.go` | `It("should read the grant back with the same representation and ETag")` |
| US1.3 | `agent_grants_test.go` | `It("should replace the grant whole and keep exactly one grant for the agent")` |
| US1.4 | `agent_grants_test.go` | `It("should answer 404 for an agent without an agent grant")` |
| US1.5 | `agent_grants_test.go` | `It("should answer 404 for write, read and delete under an unknown agent and store nothing")` |
| US1.6 | `agent_grants_test.go` | `It("should refuse a permission set the agent does not declare")` |
| US1.7 | `agent_grants_test.go` | `It("should cover only services within the agent's service requirements")` |
| US1.8 | `agent_grants_test.go` | `It("should refuse a past valid_until and store a future one")` |
| US1.9 | `agent_grants_test.go` | `It("should answer 200 with the same ETag and no audit record for an identical write")` |
| US1.10 | `agent_grants_test.go` | `It("should always include the mandatory set whether or not it is listed")` |
| US1.11 | `agent_grants_test.go` | `It("should keep the stored grant when mandatory sets change and apply them on the next write")` |
| US1.12 | `agent_grants_test.go`, `docker` | `It("should answer 412 for a stale If-Match and carry out a current or absent one")` |
| US1.13 | `agent_grants_test.go` | `It("should return an expired grant and renew it in place keeping id and created_at")` |
| US1.14 | `agent_grants_test.go`, `docker` | `It("should answer 412 for any If-Match when no grant exists and create without one")` |
| US2.1 | `agent_grants_test.go` | `It("should store a covered non-mandatory service without a binding and choose no account")` |
| US2.2 | `agent_grants_test.go` | `It("should refuse a binding for a service the grant does not cover")` |
| US2.3 | `agent_grants_test.go` | `It("should refuse a binding to an account of another service")` |
| US2.4 | `agent_grants_test.go` | `It("should refuse a binding to a non-existent account")` |
| US2.5 | `agent_grants_test.go` | `It("should keep the binding by ID when the account is renamed")` |
| US2.6 | `agent_grants_test.go` | `It("should keep the binding when a second account is added to the service")` |
| US2.7 | `agent_grants_test.go` | `It("should refuse leaving an effectively mandatory service unbound")` |
| US3.1 | `agent_grants_test.go` | `It("should refuse effective scopes beyond the ceiling and name service, account and scopes")` |
| US3.2 | `agent_grants_test.go` | `It("should compare scopes literally so write does not contain read")` |
| US3.3 | `agent_grants_test.go` | `It("should check the union of granted sets, not each set alone")` |
| US3.4 | `agent_grants_test.go` | `It("should apply no ceiling check to an unbound service")` |
| US3.5 | `agent_grants_test.go` | `It("should accept a lowered ceiling, keep the grant and refuse its next write")` |
| US3.6 | `agent_grants_test.go` | `It("should accept a widened permission set and keep the grant")` |
| US4.1 | `agent_grants_lifecycle_test.go` | `It("should delete the grant and leave declared sets and user consents unchanged")` |
| US4.2 | `agent_grants_lifecycle_test.go` | `It("should answer 404 when deleting an absent grant")` |
| US4.3 | `agent_grants_lifecycle_test.go`, `docker` | `It("should refuse deleting a bound account with 409 and bound_by_agents")` |
| US4.4 | `agent_grants_lifecycle_test.go`, `docker` | `It("should delete the account once no grant binds it")` |
| US4.5 | `agent_grants_lifecycle_test.go`, `docker` | `It("should delete the agent grant with its agent and free its accounts")` |
| US4.6 | `agent_grants_lifecycle_test.go`, `docker` | `It("should keep protecting the account while only an expired grant binds it")` |
| US5.1 | `agent_grants_lifecycle_test.go` | `It("should show only the user's own consent in delegations and grants for the agent")` |
| US5.2 | `agent_grants_lifecycle_test.go`, `docker` | `It("should hide the agent grant from a user whose principal equals the agent UUID")` |
| US5.3 | `agent_grants_lifecycle_test.go` | `It("should give the agent's self token the same exchange and evaluation denial as without a grant")` |
| US5.4 | `agent_grants_lifecycle_test.go` | `It("should never read the agent grant as a human's consent in exchange or evaluation")` |
| US5.5 | `agent_grants_lifecycle_test.go`, `docker` (PostgreSQL case only) | `It("should record existing grants and sessions as human and keep their behaviour")` |
| US5.6 | `agent_grants_lifecycle_test.go`, `docker` | `It("should keep a same-text human consent and the agent grant independent on revocation")` |

Notes:

- US1.9 asserts "no audit record" through the test logger that the bootstrap injects, as the
  service-account suite does.
- US1.13 and US4.6 need an expired grant. The test writes a grant whose `valid_until` is a few
  seconds ahead and waits with `Eventually` on an observable effect. No clock is injected and
  there is no sleep (`tests/e2e/AGENTS.md`).
- US5.3 reuses the ADR 045 self-token issuance of `agent_subject_impersonation_test.go`. It
  compares status, error, description and the absence of `error_uri` with the same request made
  before the grant is written.
- US5.4 runs with an `agent_subject_expression` that never matches
  ([research R16](./research.md#r16--acceptance-tests-across-stores-and-kinds)).
- US5.5 is about migrating PostgreSQL data, so it has no memory case. The `It()` starts from
  `NewPostgresFixtureAtVersion(50)`.

**Red Phase Requirements**: Tests compile against the Phase 2.7 scaffold (handlers answering
`501`, an `AgentGrants()` accessor whose adapters return `not_found`). They fail on concrete
assertions: status codes, `ETag`, representation members, messages, `bound_by_agents`, listing
contents. No `XIt`, `PIt`, `Skip()` or red-phase comments. US5.1–US5.4 fail because the grant
cannot be written yet. Each first writes the grant and asserts the `201`.

**Test Data Strategy**:

- Existing fixtures: `fixtures.AdminPrincipal()`, `fixtures.NamedPlatformService(...)`, the
  service-account fixtures of #471 (`fixtures/service_account.go`) for the stored JWTs, and the
  agent and grant fixtures for Alice's consent.
- New `tests/e2e/fixtures/agent_grant.go` holds the spec cast with the journeys' IDs:
  - triage-bot and planner-bot;
  - *Platform reader*, *Platform operator* and *Platform admin*;
  - *Company Platform*;
  - `platform-triage-bot` and `platform-planner-bot`;
  - request builders.

**Bootstrap Strategy**:

- A fresh admin and end-user `bootstrap.TestServer` pair and storage per test (`BeforeEach` /
  `AfterEach`), on the dual-server pattern. US5 needs the end-user server.
- Store-dependent scenarios use a `withAgentGrantBackend` helper equivalent to
  `withServiceAccountBackend`.

**Helper Utilities**:

- Existing: `server.AuthenticatedGET`, `DirectRequest` (for `PUT`, `DELETE`, `If-Match`), the
  AuthZEN and token-exchange helpers.
- New in `tests/e2e/helpers/agent_grant.go`: PUT, GET and DELETE wrappers with optional
  `If-Match`, a decoder for the representation, and a decoder for the `409` conflict body.
- Mock services: none. The feature makes no outbound call.

### Unit & Integration Tests

**Unit Tests** (TDD, written first):

- `internal/domain/storage/effective_scopes_test.go`: table tests copied from the resolver's
  coverage cases (no service requirements, `require_all_scopes`, intersection, empty
  intersection dropped, service outside the requirements) plus their exclusion reasons.
- `internal/domain/storage/agent_grant_test.go`: kind and principal invariants, `Copy`, sorted
  accessors.
- `internal/domain/consent/agent_grant_service_test.go`:
  - every rule of the data model, in order, with its message, plus the first-failure
    determinism of the service iteration;
  - mandatory sets added; listed mandatory set ignored; coverage limited by service
    requirements; copy on write;
  - status `failed` accounts accepted; no session read (a session repository double that fails
    the test if called);
  - outcomes `created`, `replaced` and `unchanged`;
  - every precondition row, the re-read after a concurrent commit, and the `412` after an
    `If-Match` mismatch at save;
  - `ErrAgentGrantReferenceMissing` mapped to `400`;
  - audit records per the observability contract;
  - no credential accessor called on the account double.
- `internal/domain/consent/service_test.go`: the existing consent tests, unchanged, after the
  Phase 0 move.
- `internal/domain/tokenexchange/grant_resolver_test.go`: the existing tests unchanged after
  Phase 0.
- `internal/domain/thirdparty/service_account_service_test.go`: bound delete returns
  `ServiceAccountBoundError` with sorted agents and `errors.Is(ErrServiceAccountBound)`; a store
  `ErrServiceAccountReferenced` re-reads the agents.
- Memory adapter tests (`internal/adapters/storage/memory/`):
  - a human and an agent row with equal principal text coexist;
  - human methods never return the agent row;
  - `DeleteByAgent` removes both;
  - the agent cascade;
  - a concurrent `Save` and account `Delete` under `-race` never leave a binding to a missing
    account and never deadlock.
- `internal/adapters/storage/postgres/agent_grant_repository_unit_test.go` (no database): error
  mapping (`23505`, `23503`, version mismatch) and record conversion.
- `internal/adapters/http/handlers/admin/agent_grants_handler_test.go`: operator required before
  any read; agent resolution; `If-Match` parsing; body limit, unknown members and `null`; status
  and `ETag`; error mapping.
- `internal/adapters/http/handlers/admin/service_accounts_handler_test.go`: `bound_by_agents` in
  the `409`.

**Integration Tests**:

- `tests/integration/migrations/agent_grants_migration_test.go` (`integration` tag):
  - apply, rollback and reapply, repeated twice;
  - existing grants and sessions keep every column and get `human`;
  - each check and unique constraint;
  - an agent principal different from `agent_id` is refused;
  - a cross-service binding is refused (`23503`), and a bound account delete is refused;
  - agent delete cascades to bindings;
  - rollback removes only agent grants and the new tables.
- `tests/integration/storage/contract/agent_grant_repository.go` and `principal_kind_scope.go`:
  the shared contracts. They run on the memory store in `tests/integration/storage/` (`just
  test-integration`) and on PostgreSQL in `tests/integration/storage/infra/` (`integration` tag).
  PostgreSQL also covers the storage spans and `storage.outcome`.

**Test Coverage Goals**:

- Unit: all branches of the resolution rules, the service and the handler's error mapping.
- Integration: migration `051` in both directions; every `AgentGrantRepository` method and every
  human-scoped method on both stores.
- E2E: 100% of the acceptance scenarios (39).
- Frontend E2E: not applicable.

## Complexity Tracking

No constitution violations require justification.
