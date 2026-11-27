# Research: Agent Grants (Admin-Issued Delegation to an Agent for Itself)

**Feature**: [spec.md](./spec.md) | **Plan**: [plan.md](./plan.md) | **Date**: 2026-11-27

The Technical Context has no open `NEEDS CLARIFICATION` items: the spec's two clarification
sessions settle every behavioural question. Each section records one design decision, its rationale
and the main alternatives. Decisions marked **Proposed** are not fixed by the spec, by issues #472,
#478 and #471, or by the maintainer's answers. They are listed in [plan.md](./plan.md#review-items)
for the maintainer's review of the PR.

## R1 — Where the domain code lives

**Decision**: A new `consent.AgentGrantService` in the existing `internal/domain/consent/`
package (`agent_grant_service.go`) holds the workflow. The entity `storage.AgentGrant` and the
value objects `storage.PrincipalKind` and `storage.ServiceAccountBinding` go into
`internal/domain/storage/` next to `UserGrant`. No new `domain/X/` package is created.

**Rationale**: The spec's Domain Model places the Agent Grant in the consent bounded context and
reuses its validation. `UserGrant`, `GrantedPermissionSetEntry` and the consent rules already live
in those two packages. A separate service type keeps `consent.Service` and its end-user
interface (`handlers/consent.ConsentService`) unchanged, so no end-user path can reach an
agent-grant operation (FR-001, API-004). Constitution Principle VI forbids a new package that is
not an independent bounded context. An agent grant is a `UserGrant` with extra rules, so it is not
independent.

**Alternatives considered**:

- New `internal/domain/agentgrant/` package: it would need the consent rules, the permission-set
  querier and the grant store, so it is not independent.
- Methods on `consent.Service`: they would widen the type that end-user handlers depend on.

## R2 — Principal kind: identity in storage, human scope on the existing ports

**Decision**: `user_grants` and `user_sessions` gain `principal_kind` (`human` | `agent`). It
becomes part of both unique constraints. The two existing ports keep their signatures and become
**human-scoped by contract**: every principal-keyed method of `ports.UserGrantRepository` and
`ports.UserSessionRepository`, and of `UserSessionRefreshRepository.WithLockedSession`, reads,
writes and deletes only rows with `principal_kind = 'human'`. Their ID-keyed methods do the
same: grant `Get`, `Update`, `Delete` and session `Get`, `Delete`. The adapters name the kind in
every statement: a literal in SQL, and the key prefix in memory. Agent-kind grant rows are
reachable only through a new `ports.AgentGrantRepository` (R4). `Create` and `CreateIfAbsent`
always write `human`. They refuse a `UserGrant` or `UserSession` whose kind is set to anything
else with a `validation` `StorageError`, so a caller cannot write an agent row through a human
port.

A few methods stay kind-agnostic on purpose, because they serve cross-cutting guards, not human
decisions:

| Method | Why it counts every kind |
|---|---|
| `UserGrantRepository.DeleteByAgent` | Agent deletion removes every grant to the agent (FR-015) |
| `UserGrantRepository.CountGrantsReferencingPermissionSet`, `permission_set_repository.go` delete-transaction count | Permission-set deletion is refused while any unexpired grant contains the set, agent grants included (FR-015) |
| `thirdparty_provider.go` `activeProviderGrantReferenceQuery` | Service deletion keeps its active-grant guard; an agent grant covering a service also holds the service |
| `UserSessionRepository.CountByService`, `ListExpiringSessions`, `CountSessionsByExpiry` | Service guards, sweep and health metrics. No agent session exists until #473 |

**Rationale**: The maintainer's answer makes the kind part of each record's identity (FR-016). The
storage inventory shows that human paths reach the grant port outside `consent.Service`:
`oauth2.AuthorizationService.HandleAuthorization` calls `grantRepo.FindByPrincipalAndAgent`
directly, and `oauth2session.Service` reads sessions and grants for the agents affected by ending
a session. Filtering in the adapter therefore covers every current human caller (FR-017, FR-018)
and every future one, without changing about 30 call sites in five domain packages. A separate
port for agent rows keeps "human" and "agent" from ever being one argument away from each other.
The kind is never derived from the principal text (FR-016). #473 can widen the resolver by adding
an agent branch that uses `AgentGrantRepository`, so the human port never needs a kind
parameter.

**Alternatives considered**:

- A `PrincipalKind` parameter on every principal-keyed port method: it makes the kind explicit at
  each call site, but changes about 30 call sites and their test doubles. Every existing call
  would pass `human`, so a mistaken `agent` argument becomes possible where today none can exist.
- A typed `id.GrantPrincipal{Kind, Principal}` replacing `id.Principal` in the ports: the same
  churn, plus a type change in the logging principal policy.
- A separate `agent_grants` table instead of `user_grants` rows: contradicts #472 ("stored as a
  `UserGrant` with principal = agent UUID, so the shared grant resolution … work[s] unchanged").

## R3 — Migration `051`: principal kind, binding storage, constraints

**Decision**: One atomic pair `051_agent_grants_principal_kind.{up,down}.sql`. Details are in
[data-model.md](./data-model.md#postgresql-schema--migration-051_agent_grants_principal_kind).
In summary:

1. `user_grants.principal_kind VARCHAR(16) NOT NULL DEFAULT 'human'` with
   `CHECK (principal_kind IN ('human', 'agent'))`. For an `agent` row,
   `CHECK (principal_kind <> 'agent' OR principal = agent_id::text)`, so an agent grant's
   principal is the agent's own UUID and the unique constraint gives at most one per agent.
   `uq_principal_agent` is replaced by `uq_user_grants_kind_principal_agent UNIQUE
   (principal_kind, principal, agent_id)`.
2. `user_sessions.principal_kind` with the same column and value check. `CHECK (principal_kind =
   'human')` is not added, because #473 adds machine sessions. `user_sessions_principal_service_unique`
   is replaced by `uq_user_sessions_kind_principal_service UNIQUE (principal_kind, principal,
   service_id)`. The redundant `idx_user_sessions_principal_service` is dropped because the new
   constraint's index covers it.
3. `agent_grants (grant_id UUID PRIMARY KEY REFERENCES user_grants(id) ON DELETE CASCADE, version
   BIGINT NOT NULL DEFAULT 1 CHECK (version >= 1))` carries the strong ETag beside the grant
   record.
4. `service_accounts` gains `uq_service_accounts_id_service UNIQUE (id, service_id)` as the target
   of a composite foreign key.
5. `agent_grant_service_account_bindings (grant_id, service_id, service_account_id)` with
   `PRIMARY KEY (grant_id, service_id)`. It has `FOREIGN KEY (grant_id) REFERENCES
   agent_grants(grant_id) ON DELETE CASCADE` and `FOREIGN KEY (service_account_id, service_id)
   REFERENCES service_accounts(id, service_id) ON DELETE RESTRICT`. The database itself thus
   refuses deleting a bound account and a binding to an account of another service (DB-002).
   It removes bindings with their grant, and so with their agent, through the
   `user_grants.agent_id` cascade. An index on `service_account_id` serves the delete guard and
   `bound_by_agents`.

DOWN deletes `agent`-kind grant rows, drops the two new tables and the composite unique
constraint, restores the original unique constraints and the dropped index, and drops the two
columns. Rolling back erases every agent grant. A restored `UNIQUE (principal, agent_id)` could
otherwise collide with a human grant whose principal text equals the agent's UUID.

Every statement is transactional. Adding a `NOT NULL` column with a constant default is
metadata-only in PostgreSQL 11 and later. The unique constraints and checks scan `user_grants`
and `user_sessions` once under the migration's lock. This is the same atomic pattern as `050`, so
no `-- migrate:no-transaction` exception is needed (Constitution Principle IX).

**Rationale**: DB-002 asks for the database itself to refuse deleting a bound account and to
refuse a cross-service binding. Only a composite foreign key does the latter without a trigger.
Keeping `version` and the bindings out of `user_grants` follows the maintainer's answer ("stored
beside the grant record, not inside `UserGrant`"; "`UserGrant` gains only the principal kind").

**Alternatives considered**:

- `version` and `service_account_bindings JSONB` columns on `user_grants`: simpler, but
  contradicts the maintainer's answer and cannot carry a foreign key.
- A trigger that checks the binding's service: a second enforcement language for something a
  constraint expresses.
- Splitting the unique-constraint swap into a non-atomic `CONCURRENTLY` migration: no accepted ADR
  exception covers it (`adrs/039-concurrent-session-expiry-index.md` names only `037`). The grant
  and session tables are administrator-scale and user-scale, so the atomic build is the
  constitutional default.

## R4 — Repository port for agent grants

**Decision**: A new `ports.AgentGrantRepository` in `internal/ports/storage.go` with four methods
(signatures in [data-model.md](./data-model.md#repository-port)):

- `Get(ctx, agentID)`: the agent's grant with its bindings and version, expired included. A
  missing grant is `not_found`.
- `Save(ctx, grant, expectedVersion)`: creates or replaces the grant, the grant row, its
  `agent_grants` row and its bindings in one transaction. It returns whether it created. With
  `expectedVersion` set it applies only where the stored version equals it, and an absent grant is
  a version mismatch. A replacement keeps `id` and `created_at`. It increments the version and
  sets `updated_at`.
- `Delete(ctx, agentID, expectedVersion)`: deletes the grant and returns the deleted snapshot for
  the audit record. Absent is `not_found` without a precondition and a version mismatch with one.
- `ListAgentsBindingServiceAccount(ctx, accountID)`: the sorted, distinct agent UUIDs whose
  grants bind the account, expired grants included.

Sentinels `ports.ErrAgentGrantVersionMismatch` (wrapped in `conflict`) and
`ports.ErrAgentGrantReferenceMissing` (wrapped in `not_found`, for a permission set or account
deleted during the write) classify errors without message parsing. This follows the
`ErrServiceAccountVersionMismatch` precedent.

**Rationale**: ISP and Principle IX: a focused interface of four methods per aggregate, in the
ports file. The agent grant is the aggregate root for writes (spec Domain Model), so its
repository writes the grant record and the bindings together. A partial write is impossible on
both adapters. "Absent with a precondition is a mismatch" implements RFC 9110 §13.1.1 atomically
in the store (FR-003, User Story 1 Scenario 14).

**Alternatives considered**:

- Extending `UserGrantRepository`: it already has 12 methods and is human-scoped (R2).
- A separate binding repository: bindings have no lifecycle of their own, and splitting them
  would make a grant write two repository calls without a shared transaction.

## R5 — Concurrency, ETag and no-op writes

**Decision**: `agent_grants.version` is the strong ETag (`"1"`). It is parsed and formatted with
the existing `parseStrongETag` and `strongETag` helpers, and malformed values answer `400` as on
service accounts. `AgentGrantService.Put` and `Delete`:

1. Load the agent's grant (absent allowed).
2. If `If-Match` is supplied: with no grant, answer `412`. With a different version, answer
   `412` (FR-003).
3. Build the candidate (R6, R7, R8). If it equals the stored grant (same permission sets, same
   coverage, same bindings, same `valid_until`), return outcome `unchanged`. That means `200`,
   the same ETag, no write and no audit record (FR-003, User Story 1 Scenario 9).
4. Save conditionally on the version read: `expectedVersion` = the stored version, or "absent"
   for a create. Without `If-Match`, a mismatch at save time means another writer committed in
   between. The service then re-reads and repeats from step 1, up to three attempts, as
   `ServiceAccountService` does. With `If-Match`, the mismatch is returned as `412`.

Request errors win over the precondition (`400` and `404` before `412`), as in the service-account
feature (RFC 9110 §13.2.1). Absent and expired grants are both "present" for the admin
operations (FR-006).

**Rationale**: This is the maintainer's answer and the #471 posture. The retry keeps "last writer
wins" from silently restoring a binding that a concurrent writer removed after this writer's
read. The precondition held by the store makes the check atomic on both adapters.

## R6 — Permission sets, mandatory sets and coverage (copy on write)

**Decision**: The request lists optional permission-set IDs. The service checks them in this
order and stops at the first failure:

1. No duplicate in the list (`permission_sets must not contain duplicates`).
2. Every listed set is declared by the agent (`permission set <id> is not declared by the agent`).
   A listed mandatory set is accepted and changes nothing (FR-004).
3. Granted sets = the agent's mandatory sets ∪ listed sets, sorted by UUID. An empty result is
   refused (`agent grant must contain at least one permission set`). In practice this happens
   only when the agent declares no mandatory set and the list is empty.
4. The sets are loaded with `PermissionSetQuerier.GetByIDs`. A set that no longer exists is refused
   (`permission set <id> no longer exists`).
5. Coverage: each granted set covers every service of its `ServiceScopes`, limited to the agent's
   service requirements when the agent declares any. A set that would cover no service is refused
   (`permission set <id> covers no service of the agent`) (FR-004a).
6. The resulting `GrantedPermissionSetEntry` list is checked with the consent rules shared with
   user grants (R10) and `UserGrant.ValidateForCreate`. This is defence in depth: the
   construction already satisfies them.

The result is stored in the grant (FR-004c). Later changes to the agent, its service requirements
or a permission set do not touch the stored grant. Session existence is never consulted (FR-005).

**Rationale**: This is the maintainer's revised answer: the request names optional sets only, and
the broker adds the mandatory sets and computes coverage. Reusing the consent structure check
guarantees that an agent grant is a `UserGrant` the shared resolution reads exactly as it reads a
consent (FR-002).

**Alternatives considered**:

- Calling `consent.Service.GrantConsent`: it requires active sessions and writes through the
  human port.

## R7 — Bindings and effectively mandatory services

**Decision**: `service_account_bindings` maps service ID to service-account ID. The service checks,
iterating services in ascending UUID order so the first failure is deterministic:

1. Every key is a service that some granted set covers. Otherwise:
   `service_account_bindings[<service>]: service is not covered by the grant` (FR-008).
2. `ServiceAccountRepository.Get(serviceID, accountID)` finds the account. A missing account and
   an account of another service are the same `not_found`:
   `service_account_bindings[<service>]: service account <id> is not an account of this service`
   (FR-009). Only the account's ceiling is read. Its credential is never decrypted (SR-004), and
   its status is not consulted (FR-014).
3. Every effectively mandatory covered service is bound. A service counts as effectively mandatory
   when the agent's service requirements mark it mandatory, or when a granted set's
   `ServiceScope` marks it mandatory and it lies within the agent's service requirements where the
   agent declares any. This is the definition `validateGrantStructureWithResolved` already uses.
   Otherwise: `service_account_bindings[<service>]: service is mandatory for the agent and must be
   bound` (FR-004b).

There is no default binding: an omitted service is unbound even when the service holds one account
(FR-007). Bindings store the account ID, so a rename has no effect (FR-009) and adding an account
changes nothing (FR-010). **Proposed**: the message texts. Journey 2 fixes the first one.

## R8 — Ceiling check and one effective-scope definition

**Decision**: The per-service computation in `tokenexchange.GrantResolver.resolveGrantCoverage`
moves, unchanged, into a pure function `storage.EffectiveScopes(agent, entries, sets)` in
`internal/domain/storage/effective_scopes.go`. It returns the effective scopes per service and
the reason each covered service was dropped (`service_requirement_excluded`,
`scope_intersection_empty`). The resolver calls it and maps the reasons to its existing
`FailureDetail` values, so every diagnostic stays as it is. `AgentGrantService` calls the same
function and, for each bound service, refuses the write when `effective \ ceiling` is non-empty.
It compares exact byte strings and names the excess scopes in sorted order:
`service_account_bindings[<service>]: scopes [<excess>] exceed the ceiling of service account
<account>` (FR-011, FR-012). A service with no effective scopes is trivially within the ceiling.

**Rationale**: FR-011 requires "the same effective-scope definition the shared grant resolution
uses today". A single function makes this true by construction. It also lets #473's
request-time check reuse it. `consent` cannot import `tokenexchange`, which imports `consent`, so
the function lives in the shared model package both already import. The move is a
behaviour-preserving refactor (Phase 0), covered by the existing resolver tests and the existing
exchange and evaluation E2E suites (SC-004).

**Alternatives considered**:

- A second implementation in `consent`: two definitions would drift, and FR-011 would hold only by
  review.
- Calling the resolver from consent: an import cycle, and the resolver needs a validated subject
  token.

## R9 — Service-account delete guard and `bound_by_agents`

**Decision**: `thirdparty.ServiceAccountBindingChecker` changes from `IsBound(ctx, accountID)
(bool, error)` to `BindingAgents(ctx, accountID) ([]id.AgentID, error)`.
`ServiceAccountService.Delete` refuses a non-empty result with a `*thirdparty.ServiceAccountBoundError{AgentIDs}`.
That error satisfies `errors.Is(err, ErrServiceAccountBound)`, so the existing `409` mapping and
message are kept. The handler adds `bound_by_agents` from the error. The builder wires
`b.storage.AgentGrants()`, whose `ListAgentsBindingServiceAccount` satisfies the interface through
a small adapter. `NoServiceAccountBindings` is deleted.

Store-level enforcement closes the race with a concurrent grant write:

- PostgreSQL: the `RESTRICT` composite foreign key makes the account `DELETE` fail with `23503`.
  `ServiceAccountRepository.Delete` maps that to `conflict` wrapping a new
  `ports.ErrServiceAccountReferenced`. The service then re-reads `BindingAgents` and returns the
  bound error.
- Memory: the service-account repository receives a binding guard (`WithBindingGuard`) and
  deletes inside `guard.WithoutBindings(accountID, func() error)`. The agent-grant store holds its
  own lock while it checks that no grant binds the account and runs the delete. Lock order is
  always the agent-grant store first, then the service-account store. `Save` also takes them in
  that order when it checks that the bound accounts exist, so the two cannot deadlock.

**Rationale**: FR-015 and the maintainer's answer require the binding agents in the response. The
port was introduced by #471 for exactly this replacement
(`adrs/20261125-154231-service-accounts-stored-jwt.md` decision 8). The change keeps it a
binding-checker port and keeps the `409` and its message, so it is a refinement, not a deviation.
The new ADR records it (R14).

**Alternatives considered**:

- Keeping `IsBound` and adding a second port for the agent list: two calls and a window between
  them for the list to be empty.

## R10 — Reusing consent's structure rules without behaviour change

**Decision**: Phase 0 turns `(*consent.Service).validateGrantStructureWithResolved` and
`validateScopeInclusionWithResolved` into package-level functions with the same bodies and
messages. `GrantConsent` calls them as before, and `AgentGrantService` calls them too. Their
receiver is unused today (`_ context.Context`, no field access).

**Rationale**: The spec says the agent grant "reuses [consent's] validation". A pure move keeps the
user-consent behaviour byte-identical while letting the agent path share the rules.

## R11 — HTTP adapter and request shape

**Decision**: A new `admin.AgentGrantsHandler` with `Get`, `Put` and `Delete`, registered in
`SetupAdminRoutes` as `/api/agents/{agent-id}/self-grant`. Its order of checks:

1. Operator principal (`401`, nothing read).
2. `{agent-id}` resolved with `agents.Service.ResolveID` (UUID or canonical ID, `404 agent not
   found`).
3. `If-Match` syntax (`400`).
4. Body: 64 KiB limit, `DisallowUnknownFields`, `null` members refused (`400`).
5. Domain rules (`400`).
6. Precondition (`412`), then the write.

`GET` and `DELETE` on an agent without a grant answer `404 agent grant not found`. The
representation is the one of API-002 with `permission_sets` and `granted_permission_sets` keys
in ascending UUID order. `PUT` answers `201` on create without a `Location` header, because the
resource URI is the request URI, and `200` on replace or no-op. `ETag` is on `200` and `201`.
**Proposed**: `404` message text, omission of `Location`, ordering.

**Rationale**: The resource is a singleton sub-resource of the agent (`self-grant`), so `PUT`
create-or-replace is the natural verb (RFC 9110 §9.3.4). The order of checks mirrors
`ServiceAccountsHandler`. The handler only parses input and maps errors (Principle VI).

## R12 — Audit records and telemetry

**Decision**: `AgentGrantService` writes one `Info` audit record after each committed write:
`consent.agent_grant.created`, `consent.agent_grant.replaced` or `consent.agent_grant.deleted`.
Each carries `operator_principal` as `id.Principal`, `agent_id`, `grant_id`, `permission_sets`,
`granted_permission_sets`, `service_account_bindings` and `valid_until`, plus `previous_observed_*`
on a replacement. A failed save writes `consent.agent_grant.write_failed` (`Error`, with
`error_kind`). Refusals and no-ops write nothing (FR-019). The grant's principal is not logged as a
principal: it is the agent's UUID and is already carried as `agent_id`. The PostgreSQL adapter adds
four `storage.*.agent_grant` spans. No metric is added. Details are in
[contracts/observability.md](./contracts/observability.md).

**Rationale**: The prefix follows the existing `consent.grant.*` events, and the fields follow
`ServiceAccountService.auditArgs`. No credential reaches any signal, because the feature never
touches one (SR-004).

## R13 — Memory store parity for agent deletion

**Decision**: The memory `AgentRepository.Delete` calls the memory grant store's `DeleteByAgent`
under the agent lock, which removes every grant to the agent and the agent grant's bindings.
PostgreSQL does this through `ON DELETE CASCADE` (`003`). **Proposed**: the cascade removes
human grants too, as in PostgreSQL, not only the agent grant.

**Rationale**: User Story 4 Scenario 5 and SC-007 require the agent grant to go with its agent on
both stores. The memory adapter does not cascade today, so this is the minimum correction on the
affected path (Constitution, Feature Scope). Cascading by agent ID is one call. It matches
production, and restricting it to agent-kind rows would add a memory-only rule PostgreSQL does not
have.

**Deferred finding (not planned)**: memory `ThirdpartyOAuth2ProviderRepository.Delete` has no
active-grant guard, but PostgreSQL's `activeProviderGrantReferenceQuery` has one. No acceptance
scenario of this feature depends on it, so it is reported to the maintainer instead
([plan.md](./plan.md#deferred-findings)).

## R14 — ADR

**Decision**: One new ADR, `adrs/20261127-183359-agent-grants-principal-kind.md`, introduced and
accepted in this PR. It records:

- the agent grant persisted as a `UserGrant` with principal kind `agent`;
- principal kind as part of grant and session identity;
- the human scope of the existing ports, and the kind-agnostic guards;
- `ports.AgentGrantRepository`, and binding storage beside the grant with the composite `RESTRICT`
  key;
- the shared effective-scope function;
- the refined binding-checker port and `bound_by_agents`;
- migration `051` and its rollback;
- Admin API `3.2.0`.

No accepted ADR is superseded. `adrs/004-storage-layer-architecture.md`,
`adrs/013-strongly-typed-entity-ids.md` and `adrs/045-agent-subject-impersonation.md` decision 6
are followed unchanged. The superseding ADR for ADR 045 decision 6 belongs to #473.

**Rationale**: `ARCHITECTURE.md` changes (glossary, data store, admin API, the consent domain).
Grant identity changes for every record, and a new port boundary appears. These are the triggers
the spec's Assumptions name.

## R15 — Typed IDs

**Decision**: No new typed ID. `AgentGrant` is identified by its `id.GrantID` and addressed by
`id.AgentID`. Bindings use `id.ServiceID` and `id.ServiceAccountID`. `PrincipalKind` is a typed
string in `internal/domain/storage`, not an ID.

**Rationale**: `adrs/013-strongly-typed-entity-ids.md` requires a typed ID per entity with a UUID
key. The agent grant's key is the existing grant UUID.

## R16 — Acceptance tests across stores and kinds

**Decision**: The E2E scenarios whose outcome depends on store semantics each keep one `It()`,
labelled `docker`, with a memory case and a PostgreSQL case inside. That covers version
preconditions, the `RESTRICT` key, kind identity in the unique constraints, and the migration
backfill. This is the `withServiceAccountBackend` pattern of #471. Everything else runs on the
memory store. One shared repository contract for `AgentGrantRepository` and for the human scope of
the two existing ports lives in `tests/integration/storage/contract/` and runs on both adapters.

User Story 5 Scenario 4 needs a human exchange to reach the grant lookup with a principal text
equal to the agent's UUID. The default detector would classify such a token as agent-subject and
reject it earlier. The test therefore configures an `agent_subject_expression` that never matches
(it still references `subject_token.sub`, as the compiler requires). The human branch then looks
up `(human, <agent UUID>, agent)`. Without the kind filter it would find the agent grant. With the
filter it is denied as "no grant". User Story 5 Scenario 5 starts from `NewPostgresFixtureAtVersion(50)`,
seeds a grant and a session with SQL, migrates to the latest version and exercises the end-user
reads.

**Rationale**: Principle XIII maps one `It()` to one scenario. Scenario 4 is the only way to prove
FR-018 through the real exchange rather than through a unit test of the adapter.
