# Research: Capability Model for Permission Sets

**Feature**: `specs/20261127-140720-capability-model` | **Date**: 2026-11-27 | **Spec**: [spec.md](spec.md)

Each section records a decision, its rationale, and the alternatives considered. Every NEEDS CLARIFICATION in the spec was resolved in its Clarifications section. The design decisions below therefore follow from those answers.

## Sources read

- `.specify/memory/constitution.md` (4.7.0), `AGENTS.md`, `ARCHITECTURE.md` glossary (§ PermissionSet, ServiceScope, AgentPermissionSetEntry, Grant Context) and API versioning (§ 5, line 1230).
- ADR `048-authzen-delegation-pip` (Accepted; §§ 4, 5, 8, 10), ADR `029-token-exchange-client-assertion-trust-anchor` (Proposed), ADR `047-unique-agent-external-id-reconcile` (Admin API major-bump precedent), ADR `013-strongly-typed-entity-ids`, ADR `004-storage-layer-architecture`.
- Domain: `internal/domain/storage/{permission_set,agent,user_grant,requirement_type,service_requirement,canonical_id}.go`, `internal/domain/canonical/id.go`, `internal/domain/permissionset/service.go`, `internal/domain/agents/service.go`, `internal/domain/consent/service.go`, `internal/domain/tokenexchange/{grant_resolver,evaluation,service,response,diagnostic}.go`.
- Storage: `internal/ports/storage.go`, `internal/adapters/storage/{postgres,memory}/{permission_set_repository,user_grants,agent_repository}.go`, migrations `017`–`020`, `029`, `036`, `049`.
- HTTP: `internal/adapters/http/handlers/admin/{permission_sets_handler,agents_handler}.go`, `internal/adapters/http/handlers/consent/grants_handler.go`, `internal/adapters/http/enduser/authzen_evaluation.go`, `api/admin/openapi.yaml` (3.0.0), `api/enduser/openapi.yaml` (1.3.0).
- ExtProc: `internal/extproc/grantcontext/{context,client,resolver}.go`, `internal/extproc/server/{grant_context,server,exchange_result_binding}.go`, `internal/extproc/authorization/{input,input_builder}.go`.
- Tests: `tests/e2e/{permission_sets_test,token_exchange_test,grant_context_journey_test,authzen_fixture_test}.go`, `tests/e2e/extproc/`, `tests/integration/migrations/`.

## R1. Bounded context and domain packaging

**Decision**: Add `internal/domain/capability/` with a `Service` that owns the Capability Group aggregate (CRUD, canonical-ID resolution, deletion protection). Storage entities live in `internal/domain/storage/capability.go` beside `PermissionSet`. `permissionset.Service` validates capability entries through a narrow `CapabilityResolver` interface (`GetByIDs`), the same way `agents.Service` depends on `PermissionSetResolver`.

**Rationale**: Operators manage capability groups independently of permission sets. Groups have their own lifecycle, identity, invariants and Admin resources, so they satisfy Principle VI's "genuinely independent bounded context". Narrow resolver interfaces keep the dependency one-way: permission sets reference capabilities, never the reverse.

**Alternatives considered**: Folding capabilities into `permissionset` would make one aggregate own two lifecycles and contradict the spec's "group is aggregate root". A separate `domain/capabilitygroup` package for groups only would split one aggregate in two.

## R2. Persistence shape

**Decision**: Four relational tables plus changes to two JSONB shapes (see [data-model.md](data-model.md)):

- `capability_groups`, `capabilities`, `capability_service_scopes` (mirrors `permission_set_service_scopes`), and `permission_set_capabilities` (replaces `permission_set_service_scopes`).
- `agents.permission_sets` JSONB elements gain an optional `capabilities` array.
- `user_grants.granted_permission_sets` JSONB elements replace `included_service_ids` with `granted_capability_ids`.

**Rationale**: Set and capability references are relational today (`permission_set_service_scopes` with `RESTRICT` to services). `ON DELETE RESTRICT` from `permission_set_capabilities` enforces FR-008 for set references in the database, not just in an advisory check. Agent entries and grant entries stay JSONB, as they are today, which keeps the existing GIN indexes and containment queries working.

**Alternatives considered**: A child table for grant entries would change grant reads, the upsert and the `refresh_token_sessions` link for no requirement in this feature. JSONB for set capabilities would lose the database-level `RESTRICT`.

## R3. Deletion protection and set updates (FR-008, US1.5, US3.4)

**Decision**:

- Deleting a capability, either by removing it from its group in a `PUT` or by deleting the group, returns `409` if any permission set includes it or any active grant references it. The PostgreSQL adapter re-checks inside a serializable transaction, in the same way as `permission_set_repository.deleteInSerializableTx`. Grant references are found with `granted_permission_sets @> '[{"granted_capability_ids":["<uuid>"]}]'`, which the existing GIN index `idx_grants_permission_sets` serves.
- A permission set update MAY drop a capability that grants reference. Exchange then fails closed for those grants (US3.4).
- A permission set update MUST be rejected with `409` when it would invalidate an agent entry under FR-005. That happens when the update drops a capability that an agent entry lists, or makes mandatory a capability that an agent entry marks optional. This mirrors FR-005's ban on silent overrides (SR-003).

**Rationale**: The spec requires grant-referenced capabilities to be undeletable but lets sets change (US3.4 needs that state to exist). Rejecting set changes that would invalidate agent entries keeps every stored agent valid without a silent rewrite.

**Alternatives considered**: Silently pruning agent entries on set update would be a silent narrowing or relaxation, which SR-003 forbids. Blocking every set change that a grant depends on would freeze sets and make US3.4 unreachable.

## R4. Effective capabilities for an agent (FR-005 – FR-007)

**Decision**: `permissionset` exposes a pure domain function. Given a set and an agent entry, it returns the agent's effective capability list: the agent entry's list, or every set entry when the agent entry has none. Each element carries its effective requirement type: mandatory if the set entry or the agent entry says mandatory. Consent, token exchange, the AuthZEN projection, the end-user consent view and agent validation all use it.

**Rationale**: One function prevents the five readers from disagreeing. It is domain logic, so it lives in the domain and not in handlers (Principle VI, logic leakage).

## R5. Grant creation through today's request shape (FR-010, US2.4)

**Decision**: `consent.Service.GrantConsent` keeps the `{permission_set_id: [included service IDs]}` request.

- For each submitted set, the broker computes the agent's effective capabilities (R4) and stores those whose services are all included.
- The request is rejected with `400` (`ErrInvalidServiceInclusion`) in two cases:
  - a capability mandatory for the agent needs an excluded service;
  - an included service would contribute no stored capability.
- All other validations stay unchanged. Those include service-requirement intersection, active sessions and the empty-grant rule.
- The idempotent-upsert comparison (`grantMatchesRequest`) compares the computed capability IDs.

**Rationale**: The second rejection keeps "no silent narrowing" (SR-003). The user included a service, but no capability would carry it. For migrated data, every capability needs exactly one service, so every included service maps to exactly one capability, and today's consent journeys behave identically (US2.4).

**Alternatives considered**: Silently storing an entry without that service would hide a narrowing from the user. Dropping the set entry would change which sets the user sees as granted.

## R6. End-user consent view stays unchanged (FR-013, API-003)

**Decision**: The end-user `PermissionSetWithRequirement.permission_set.service_scopes` (`api/enduser/openapi.yaml`, line 3779) is derived per agent from the effective capabilities:

- A service is listed if any effective capability needs it.
- Its `requirement_type` is `mandatory` if any capability that is mandatory for the agent needs it, and `optional` otherwise.

The grant read model's `granted_permission_sets` map is derived as the union of services of the granted capabilities (FR-015).

**Rationale**: It keeps the response shape. The requirement per service becomes agent-specific, which the spec's first edge case requires: an agent that makes a capability mandatory makes its service mandatory for that agent.

## R7. Token exchange and permit resolution (FR-011, FR-012, SR-001)

**Decision**: `GrantResolver.resolveGrantCoverage` changes only steps 3–4 (from the agent report: included-service index and scope union).

- For each grant entry, every granted capability ID must be in the set's current entries. Otherwise it fails closed with today's missing-permission-set response: `access_denied`, 403, consent-page `error_uri`, and detail `grant_permission_set_missing`.
- Every granted capability ID must also be in the agent's effective list (R4). Otherwise it fails with today's undeclared-set response (`grant_permission_set_undeclared`).
- The per-service union is taken over the granted capabilities' service scopes.
- Steps 5–7 are unchanged: the service-requirement ceiling, intersection, emptiness and omitted-service details.

**Rationale**: FR-012 requires the same response as a missing set. Reusing the existing diagnostic details keeps the AuthZEN denial-code enum (`AccessEvaluationDenial`, nine values) and the `/oauth2/token` error contract unchanged. The undeclared check covers an agent entry that an operator later narrowed. Without it, a grant would keep a capability the agent no longer declares, which is a silent broadening.

**Alternatives considered**: New detail codes, for example `grant_capability_missing`, would extend the AuthZEN denial enum and ExtProc's denial mapping (`grantcontext/client.go`, lines 59–60). That is an end-user API change the spec does not ask for. Logging which capability failed is enough for operators. That log attribute must name the capability by UUID only, under LOGGING.md.

## R8. Policy input source (FR-014 – FR-016, clarification 2)

**Decision**:

- **Broker**: `tokenexchange.GrantContext` gains `granted_capabilities []GrantEntityRef`. It holds one `{id, canonical_id}` reference per granted capability across all granted sets, de-duplicated and sorted by `canonical_id`, with no set attribution. The `/access/v1/evaluation` permit carries it.
- **ExtProc decode**: `grantcontext` decodes it into `Context.GrantedCapabilities`. The member is **required** with at least one item, so a permit without it fails closed like any other malformed permit (terminal 503).
- **OPA input**: `authorization.ContextInput` gains `GrantedCapabilities []CapabilityContext` (`{id, canonical_id}`) with `json:"granted_capabilities,omitempty"`. Only `contextInputFromGrantContext` sets it.
- **Disabled mode**: The disabled-mode paths never set the member, so disabled-mode JSON stays byte for byte identical, and `TestGrantContext_DisabledModeMakesNoResolverCallsAndKeepsInput` stays unchanged.
- **Token-exchange response**: `TokenExchangeResponse` is unchanged.

**Rationale**: This follows clarification 2. It must be required because ADR 048 permits fail closed on malformed content. An optional member would let a broker-version mismatch silently drop every capability, and the sample policy would then deny everything. That fails safe, but operators would not see the cause. With a required member, the mismatch surfaces at the first request. Every granted set entry holds at least one capability, so the list is never empty.

**Deployment order**: Upgrade the broker before ExtProc. An old ExtProc ignores the new member (ADR 048 drops unknown members), but a new ExtProc rejects permits from an old broker. Upgrade documentation must state this.

**Binding and caches**: Neither `ExchangeResultBinding` nor the permit and token cache keys depend on permit content (`exchange_result_binding.go`, lines 13–18; `resolver.go`, lines 21–24). The new member therefore changes neither result reuse nor cache identity.

**Shape**: Each entry follows the `{id, canonical_id}` reference shape of the permit's `agent` and `permission_sets` entries, so policies use one style for every reference and may key on either identifier. Unlike those entries, `canonical_id` is always present, because capabilities require it.

**Alternatives considered**: A flat list of canonical ID strings was the first draft. Document review rejected it because it deviates from the existing reference shape. An optional member is rejected for the reasons above. Per-set capability lists in `permission_sets[]` would let policy see which set granted a capability, which US3.3 forbids.

## R9. Migration `050` (FR-018 – FR-020, clarification 1)

**Decision**: One atomic migration pair, `050_capability_model.{up,down}.sql`. The SQL is designed in [contracts/migration-050.md](contracts/migration-050.md). Summary:

- **Groups**: One capability group per permission set, with the same name and description and `canonical_id` = the set's canonical ID.
- **Capabilities**: One capability per `permission_set_service_scopes` row. It carries that row's scopes, `read_only = false`, and name = the service's display name. Its set entry carries that row's `requirement_type`.
- **Grants**: Each grant entry's `included_service_ids` become the capability IDs for those `(set, service)` pairs.
- **Agents**: Agent JSONB is untouched (FR-007).
- **Down migration**: The down migration runs only when the data is still representable without loss of security meaning:
  - every capability has exactly one service scope and is included by exactly one set;
  - no set includes two capabilities for the same service;
  - no agent entry lists capabilities.

  Otherwise it raises an exception before changing anything, so the transaction rolls back. It then recreates `permission_set_service_scopes` and the grant service lists, and drops the new tables.
- **Atomicity**: Both directions are atomic (Principle IX). No non-atomic exception is needed.

**Rationale**: The guarded down migration follows the precedent of ADR `047-unique-agent-external-id-reconcile`: fail before changing anything. An unguarded down migration would merge capabilities into service scopes. A grant that held only *View* of a service that also has *Edit* would then roll back to the whole service, which broadens the grant.

## R10. Deterministic canonical IDs for migrated capabilities (FR-019)

**Decision**:

- **Set key**: The set's canonical ID, or the set UUID when it has none.
- **Service key**: The service's canonical ID, or the service UUID when it has none.
- **Candidate**: `<set key>-<service key>`, for example `ops-tools-github-production`.
- **Fallback**: `cap-` followed by the 32 lowercase hex characters of `md5(set_uuid || ':' || service_uuid)`. It applies to every row whose candidate:
  - is longer than 128 characters;
  - fails `^[A-Za-z0-9._-]{1,128}$`;
  - parses as a UUID;
  - collides with another candidate or an existing value.
- The migration-local helper uses the same rules as `internal/domain/canonical/id.go`. The integration test asserts that every generated value passes `storage.ValidateCanonicalID`.

**Rationale**: The result is deterministic, readable in the common case and always unique. MD5 is used only for a stable identifier, not for security. Migration 040 already validates canonical IDs with the same regular expression in SQL.

**Alternatives considered**: Always using the hash would make sample policies unreadable. A sequence number would make IDs depend on row order.

## R11. Admin API 4.0.0 (API-001, clarification 3)

**Decision**: Contract in [contracts/admin-api.md](contracts/admin-api.md).

- **Capability-group resources**: Add `/api/capability-groups` (list, create) and `/api/capability-groups/{capability-group-id}` (get, `PUT` full replacement, delete). The path ID accepts a UUID or canonical ID, as permission sets do.
- **Nested capabilities**: Capabilities are nested in the group. In a `PUT`, a capability with an `id` is updated, one without is created, and an omitted one is deleted (`409` if referenced).
- **Permission sets**: `service_scopes` is replaced by required `capabilities[]`, and a request that sends `service_scopes` returns `400`.
- **Agent entries**: Agent permission-set entries gain optional `capabilities[]`.
- **References**: `capability_id` accepts a UUID or canonical ID, like `service_id`. `Prefer: reference-id=canonical` renders capability references canonically.
- **Version**: `info.version: 4.0.0` on the existing routes.
- **Migration guide**: A migration guide is added beside the Admin API `2.0.0` guide in `docs/guides/manage-agents-and-services.md`.

**Rationale**: It follows ARCHITECTURE.md's versioning rule and the precedents of ADR `047-unique-agent-external-id-reconcile` and feature 051. Full-replacement `PUT` with stable nested IDs matches the permission-set and agent resources.

## R12. End-user API version

**Decision**: End-user API `1.3.0` → `1.4.0`. The only change is the additive required member `GrantContext.granted_capabilities`.

**Rationale**: A new response member is additive under ARCHITECTURE.md § 5, and ADR 048 consumers ignore unknown members. Feature 054's `1.2.0` → `1.3.0` bump is the precedent.

## R13. Architecture decision record

**Decision**: A new ADR `20261127-190555-capability-model` (status Proposed, to be Accepted in this feature's PR under Principle II). It records:

- the capability model and its invariants;
- the effective-capability rule;
- the fail-closed exchange rules;
- the permit-only policy source, which extends ADR `048-authzen-delegation-pip` §§ 4–5 by adding one permit and OPA member;
- the migration and guarded down migration;
- the gateway-only enforcement assumption and its deployment controls (SR-002).

**Rationale**: ADR 048 is Accepted and merged, so its permit and OPA input contract may change only through an Accepted ADR (Principle II). The new ADR adds a member and leaves every existing member unchanged. It therefore extends ADR 048 without superseding any decision. SR-002 requires the deployment controls to be stated, because ADR `029-token-exchange-client-assertion-trust-anchor` is not Accepted.

## R14. Gateway-only enforcement controls (SR-002)

**Decision**: The ADR and the operator guide state the required controls:

1. Agents receive no credential that `/oauth2/token` accepts outside the gateway path. Only the ExtProc client is registered as the privileged token-exchange client.
2. Network policy keeps agents away from the broker's token endpoint and from the upstream provider APIs. Only the gateway egresses to them.
3. `grant_context.enabled: true` and `authorization.enabled: true` are both set, with a deny-by-default policy.

The broker cannot verify these controls, so the guide states that without them capabilities narrow only OAuth2 scopes, not tool calls.

## R15. Observability and audit (SR-004, Principle XV)

**Decision**:

- **New spans and metrics**: None.
- **Audit log events**:
  - Capability groups get `capability.capability_group.created`, `.updated` and `.deleted`, following the `permissionset.permission_set.*` pattern. Fields are IDs, canonical IDs, names and service scopes.
  - Permission-set events replace `service_scopes` with `capabilities`.
  - Grant events log `granted_permission_sets` entries with `granted_capability_ids` (spec: Domain Events).
- **Principals**: Principals appear only as `id.Principal`.
- **Telemetry reference**: `just telemetry-doc` regenerates `docs/reference/telemetry.md`. Spans and metrics are unchanged, so only incidental lines may change.

The contract is in [contracts/observability.md](contracts/observability.md).

## R16. Sample policy and documentation (FR-017)

**Decision**: Add `examples/policies/capability-tool-map.rego`. It is deny by default and maps tools to capability canonical IDs, as in Journey 1. `examples/` holds no Rego today, so this creates `examples/policies/`. The ExtProc E2E suite loads the same file, so the sample is tested. Docs updated:

- `docs/api/authzen-evaluation.md` (`granted_capabilities` member);
- `docs/guides/token-exchange-gateway.md` (capability policy and SR-002 controls);
- `docs/guides/manage-agents-and-services.md` (capabilities and Admin API 4.0.0 migration);
- `docs/configuration.md` (none: no new parameter);
- the header comment in `examples/config/extproc-grant-context.yaml`.

## R17. Configuration

**Decision**: No new configuration parameter. The feature depends on the existing `grant_context.enabled` and `authorization.enabled`. No Helm change: the chart deploys only the broker, ExtProc's schema is separate under ADR `011-extproc-standalone-binary`, and neither gains a parameter.

## Deferred findings (not tasks of this feature)

- The end-user OpenAPI documents an empty `{}` grant request as a revoke with `204`, but `grants_handler.go` has no such branch (agent report, consent § 5). This predates the feature and is outside its scope. Under the Feature Scope rules, ask the user whether to file an issue.
