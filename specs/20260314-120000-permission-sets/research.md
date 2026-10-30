# Research: Permission Sets (019)

**Branch**: `20260314-120000-permission-sets` | **Date**: 2026-03-25

## 1. Agent Permission Set Storage: JSONB vs. Relational Join Table

**Decision**: Store `agent.permission_sets` as a JSONB column on the `agents` table (same pattern as `service_requirements`).

**Rationale**:
- Matches the existing `service_requirements JSONB` pattern introduced in migration 005.
- Order preservation is inherent in the JSON array without an extra `position` column.
- `permission_set_id` references do NOT require relational FK enforcement because the `agents` table stores this as an opaque JSONB blob — the application layer validates existence of each referenced ID (FR-006, SR-002).
- Referential integrity in the other direction (service → permission set) is handled via DB-level FK on the `permission_set_service_scopes` table (DB-006), where columns are relational.

**Alternatives considered**:
- `agent_permission_sets` join table with FK columns: would allow DB-level FK enforcement and ordering via a `position` integer, but adds schema complexity and is inconsistent with the `service_requirements` JSONB pattern already established. Rejected for pattern consistency.

---

## 2. UserGrant granted_permission_sets: JSONB for Structured Grant Storage

**Decision**: Store `granted_permission_sets` as a PostgreSQL `JSONB` column (structured map: `{ps_id: [included_service_ids]}`).

**Rationale**:
- The structured positive-inclusion model (Session 2026-03-29 Q3) requires storing per-PS service ID arrays, not a flat list.
- `JSONB` supports efficient containment operators (`@>`, `?`, `?|`) and GIN indexing.
- Go's `pgx` natively handles JSONB serialization/deserialization to `map[string][]string` or custom types.
- The `delegated_oauth2_tokens JSONB` column (complex objects) is being removed; the replacement uses a cleaner JSONB structure.
- GIN index on `JSONB` supports fast key/value containment queries.

**Alternatives considered**:
- JSONB: More flexible but unnecessarily complex for a flat UUID list. Would require a JSON array of UUID strings with full JSON parsing overhead. Rejected in favour of the native array type.

---

## 3. In-Process Permission Set Cache (NFR-001, FR-017)

**Decision**: Use a `sync.Map`-backed TTL cache with ~60s expiry, keyed by `PermissionSetID`. Implement as a thin wrapper inside `TokenExchangeService` with `time.Time` expiry per entry. No external cache dependency.

**Rationale**:
- The token exchange service already handles all resolution — keeping the cache here avoids adding a new infrastructure dependency.
- `sync.Map` is safe for concurrent reads (the dominant pattern on the hot token-exchange path) and allows individual entry expiry rather than a global TTL sweep.
- A single goroutine background sweep every 30s is sufficient for cleanup (cache is bounded by distinct permission set IDs, not request volume).
- NFR-002 requires ≤5ms overhead on cache hit — the map lookup + TTL check is O(1) and well under this budget.

**Alternatives considered**:
- `github.com/patrickmn/go-cache`: proven library but an extra dependency for a simple TTL map. Rejected to keep the dependency graph lean.
- Redis / external cache: far too heavy for a TTL≈60s, single-process use case. Rejected.

---

## 4. Permission Set Deletion Protection: Application vs. Database Level

**Decision**: Use application-layer protection for the agent→permission-set direction. Use database-level `RESTRICT` FK for the service→permission-set direction.

**Rationale**:
- Agent→permission-set: the agent's `permission_sets` field is JSONB (not a relational column), so a DB FK constraint is impossible. The application must query `CountAgentsReferencingPermissionSet(ctx, psID)` before deletion (SR-002, FR-004). This check uses a JSONB containment query on `agents.permission_sets`.
- Service→permission-set: the `permission_set_service_scopes` table has a `service_id UUID` column with a DB-level `REFERENCES thirdparty_oauth2_services(id) ON DELETE RESTRICT` FK (DB-006). This guarantees no orphaned `ServiceScope` entries even if the application check fails.

**Alternatives considered**:
- Database triggers for the JSONB direction: complex, non-portable, hard to test. Rejected.
- Application-only check for the service direction: would require a query before every service deletion. DB FK provides a safety net even if application logic is bypassed. Rejected.

---

## 5. PermissionSet Schema: Embedded ServiceScopes vs. Join Table

**Decision**: Normalised relational schema: `permission_sets` table + `permission_set_service_scopes` join table (NOT JSONB-embedded).

**Rationale**:
- `ServiceScope` entries are queried independently (filtering permission sets by `service_id`, DB-006 FK enforcement).
- DB-006 requires a FK from `permission_set_service_scopes.service_id → thirdparty_oauth2_services.id` with `ON DELETE RESTRICT` — this requires a relational column, not JSONB.
- The relational design also allows the `CountPermissionSetsForService` query needed when blocking service deletion.

**Alternatives considered**:
- JSONB `service_scopes` column on `permission_sets`: cannot enforce FK constraints; would require full-table JSONB scans for service-based filtering. Rejected.

---

## 6. Agent permission_sets JSONB: Containment Query for Deletion Protection

**Decision**: Use PostgreSQL JSONB containment operator `@>` to count agents referencing a permission set ID:

```sql
SELECT COUNT(*) FROM agents
WHERE permission_sets @> '[{"permission_set_id": "<id>"}]'::jsonb;
```

**Rationale**: Matches the existing pattern for `CountAgentsByServiceID` in `UserGrantRepository` which uses JSONB containment on `delegated_oauth2_tokens`. The `idx_agents_service_requirements` GIN index pattern is replicated for `permission_sets` with its own GIN index.

---

## 7. React Component Architecture for Consent Screen Restructure

**Decision**: Create two new application-specific components: `PermissionSetCard.tsx` and `PermissionSetsList.tsx` in `web/src/components/consent/`. Restructure `ConsentScreen` to render `PermissionSetsList` above `ServiceRequirementsList`.

**Rationale**:
- Follows the existing `ServiceRequirementCard.tsx` + `ServiceRequirementsList.tsx` pattern.
- Mandatory sets rendered as non-interactive pre-selected cards (`aria-disabled`, `aria-checked="true"`); optional sets rendered as togglable `role="checkbox"` cards (FR-008).
- Design system tokens: `trust` palette for mandatory, `neutral` for optional (spec requirement).

---

## 8. Migration Numbering

Next available migration number: **008**.

Sequence:
- `008_add_permission_sets` — creates `permission_sets` table and `permission_set_service_scopes` table.
- `009_add_agent_permission_sets` — adds `permission_sets JSONB` column to `agents` table.
- `010_migrate_user_grants_to_permission_sets` — adds `granted_permission_sets JSONB` to `user_grants` (structured `{ps_id: [service_ids]}` map), deletes all existing grant rows, drops `delegated_oauth2_tokens` column.

Split into three atomic migrations to: (a) isolate schema additions from destructive changes, (b) allow rollback at each step independently, (c) match DB-003 atomicity requirement.

---

## 9. New Typed ID: PermissionSetID

**Decision**: Add `{"PermissionSetID", "permission_set"}` to `gen_ids.go` and re-run `go generate ./internal/domain/id/`.

Follows ADR-20260313-120000 exactly. `ParsePermissionSetID` for production use; `MustParsePermissionSetID` for test fixtures only.
