# Data Model: Canonical Resource IDs

## Canonical ID

| Field | Type | Rules |
|---|---|---|
| `canonical_id` | nullable string; API validation accepts 1–128 characters (storage column may be `VARCHAR(255)`) | Optional; when present case-sensitive, opaque, and limited to `A–Z`, `a–z`, `0–9`, `.`, `_`, and `-`; it must not parse as a UUID. Unique within the owning resource type. |

`canonical_id` is entity metadata, not the persisted primary key. It is distinct from an agent's `external_id` and must not reuse that field.

## Managed resources

| Resource | Stable primary key | Canonical column | Uniqueness | References resolved by |
|---|---|---|---|---|
| Agent | `id.AgentID` / `agents.id` UUID | `agents.canonical_id` | `agents` only | `AgentRepository.GetByCanonicalID` |
| Third-party OAuth2 service | `id.ServiceID` / `thirdparty_oauth2_services.id` UUID | `thirdparty_oauth2_services.canonical_id` | services only | `ThirdpartyOAuth2ProviderRepository.GetByCanonicalID` |
| Permission set | `id.PermissionSetID` / `permission_sets.id` UUID | `permission_sets.canonical_id` | permission sets only | `PermissionSetRepository.GetByCanonicalID` |

The same canonical text may be assigned once in each resource type. `NULL` represents no canonical ID and does not participate in uniqueness.

## Identifier semantics

| Context | Submitted or returned field | Semantics |
|---|---|---|
| Administrative path/query/reference input | `id`, `service_id`, `permission_set_id` | UUID or canonical ID for that field's target resource type. The resolver returns the stable typed UUID. |
| Top-level response | `id` | Stable entity UUID; identical whether or not a canonical ID is assigned. |
| Top-level response | `canonical_id` | Optional canonical text; read-only, null when absent per OpenAPI nullable encoding. |
| Nested response (default) | `service_id` / `permission_set_id` | Target entity UUID; identical whether or not the target has a canonical ID. |
| Nested response under honored `Prefer: reference-id=canonical` | `service_id` / `permission_set_id` | Target canonical ID when present, otherwise UUID; still a valid write input. Acknowledged via `Preference-Applied`; response sends `Vary: Prefer`. |

## Relationship invariants

1. The database and internal domain continue to store UUID primary and foreign identifiers only.
2. `Agent.ServiceRequirements[].ServiceID` and `Agent.PermissionSets[].PermissionSetID` are resolved UUIDs before JSONB serialization.
3. `PermissionSet.ServiceScopes[].ServiceID` is resolved UUID before the foreign-key write.
4. A missing or deleted canonical ID does not affect relationships already persisted by UUID.
5. Create/update rejects a UUID-shaped or empty canonical ID before mutation. A same-table collision returns conflict without changing the existing entity.
6. A reference that cannot resolve to the expected target type rejects the entire write; no partial entity update is stored.

## Persistence and concurrency

Migration `029_add_canonical_ids` adds all three nullable columns plus these per-table partial unique indexes:

```sql
CREATE UNIQUE INDEX uq_agents_canonical_id
  ON agents (canonical_id) WHERE canonical_id IS NOT NULL;
CREATE UNIQUE INDEX uq_thirdparty_oauth2_services_canonical_id
  ON thirdparty_oauth2_services (canonical_id) WHERE canonical_id IS NOT NULL;
CREATE UNIQUE INDEX uq_permission_sets_canonical_id
  ON permission_sets (canonical_id) WHERE canonical_id IS NOT NULL;
```

The matching down migration drops indexes before columns. PostgreSQL maps uniqueness violations to the domain conflict error. Memory adapters maintain one canonical lookup/index map per resource repository and modify the entity plus index atomically while holding the existing write mutex.