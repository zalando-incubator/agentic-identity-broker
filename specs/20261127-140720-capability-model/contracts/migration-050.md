# Contract: Migration `050_capability_model`

**Files**: `migrations/050_capability_model.up.sql`, `migrations/050_capability_model.down.sql` | **Spec**: US2.1–US2.3, FR-018–FR-020, DB-001–DB-004 | **Research**: R9, R10

Both directions run in one transaction (Principle IX). Neither carries `-- migrate:no-transaction`. Every query lists explicit columns. The target schema is in [data-model.md](../data-model.md).

## Up

1. **Create tables**: Create the four tables and indexes from data-model.md. `permission_set_service_scopes` stays in place until step 5.
2. **Groups**: Insert one `capability_groups` row per `permission_sets` row:
   - `id`: `gen_random_uuid()`;
   - `canonical_id`: the set's `canonical_id` (set canonical IDs are unique, so group canonical IDs are too);
   - `name`, `description`: the set's;
   - `created_at`, `updated_at`: the migration time.
3. **Capabilities**: Insert one `capabilities` row per `permission_set_service_scopes` row `(ps, svc)`, joined to its group:
   - `position`: `row_number()` over `(PARTITION BY ps ORDER BY svc)`, starting at 0;
   - `name`: `thirdparty_oauth2_services.display_name`;
   - `description`: `'Migrated from permission set ' || permission_sets.name`;
   - `read_only`: `FALSE`;
   - `canonical_id`: as specified below.

   A temporary mapping table `capability_migration_map(permission_set_id, service_id, capability_id)` records the pairs and is dropped at the end of the migration.
4. **Scopes and set entries**: `capability_service_scopes` gets `(capability_id, svc, scopes)`, copied from the row. `permission_set_capabilities` gets `(ps, capability_id, position, requirement_type)`, also copied from the row.
5. **Grants**: Rewrite `user_grants.granted_permission_sets`. Every element `{permission_set_id, included_service_ids}` becomes `{permission_set_id, granted_capability_ids}`: each included service ID maps through `capability_migration_map`, in `included_service_ids` order. `updated_at` stays unchanged, because the grant's meaning is the same and `GrantDetails.updated_at` feeds `ExchangeResultBinding`.
   - **Guard**: If any included service has no mapping row (a stored grant referencing a service its set no longer defines), the migration raises an exception and aborts. Silently dropping the reference would narrow, and silently keeping it would be impossible. The exception message names the count only, never principals.
6. **Drop**: Drop `permission_set_service_scopes`.
7. **Agents**: Leave `agents.permission_sets` unchanged (FR-007).

### Generated capability canonical IDs (FR-019)

Every migrated capability has one candidate:

```text
candidate = coalesce(ps.canonical_id, ps.id::text) || '-' || coalesce(svc.canonical_id, svc.id::text)
```

- **Fallback rule**: A capability uses `'cap-' || md5(ps.id::text || ':' || svc.id::text)` instead when its candidate meets any of these conditions:
  - is longer than 128 characters;
  - fails `candidate COLLATE "C" ~ '^[A-Za-z0-9._-]{1,128}$'`;
  - matches the UUID pattern of migration 040;
  - occurs more than once among all candidates.
- **Uniqueness**: The fallback is 36 characters, not UUID-shaped, and unique per `(ps, svc)`.
- **Constraint**: The `uq_capabilities_canonical_id` constraint guarantees uniqueness. If it were ever violated, the whole migration would roll back.

Journey 2 example: the set `ops-tools` and the services `github-production` and `slack` produce `ops-tools-github-production` and `ops-tools-slack`.

## Down

1. **Guards**: Each guard below raises `RAISE EXCEPTION` if the data cannot return to its pre-upgrade shape without changing what a grant allows. The exception aborts the transaction before any change:
   - a capability has more or fewer than one `capability_service_scopes` row;
   - a capability is included by a number of permission sets other than one;
   - a permission set includes two capabilities for the same service;
   - an `agents.permission_sets` element has a `capabilities` member;
   - a permission set has no capability entries;
   - a grant entry references a capability ID that does not exist or is not included by that entry's set.

   The messages name counts and the condition, never principals or scopes.
2. **Recreate the service-scope table**: Recreate `permission_set_service_scopes` with its migration-020 shape, including the `requirement_type` default. Fill it from `permission_set_capabilities` joined to `capability_service_scopes`, with `(set, service, scopes, requirement_type)`.
3. **Grants**: Rewrite each grant element to `{permission_set_id, included_service_ids}`, where each capability ID maps to its single service in the stored order. `updated_at` stays unchanged.
4. **Drop**: Drop `permission_set_capabilities`, `capability_service_scopes`, `capabilities` and `capability_groups`.

### What the down migration loses

Capability names, descriptions, `read_only` flags, generated canonical IDs and groups are lost. None of these affects a grant's effective scopes. Re-running `up` regenerates the same canonical IDs from the same data, but with new UUIDs.

## Integration tests (`tests/integration/migrations/capability_model_migration_test.go`)

These tests follow the pattern of `TestMigration040CredentialSourceLegacyBackfillAndCompatibleReapply`.

| Test | Steps and assertions |
|---|---|
| `TestMigration050ConvertsSetsAndGrantsWithoutBroadening` (US2.1, US2.2, SC-001) | Up to `049`, seed Journey 2 (set with mandatory GitHub and optional Slack, one grant with GitHub only, one with both) plus a set without a canonical ID, then up to `050`. Assert one group per set, one capability per former scope row with that row's scopes and requirement type, `read_only = false`, the generated canonical IDs, the grant entries' capability IDs and unchanged `updated_at` |
| `TestMigration050GeneratedCanonicalIDsAreValidAndUnique` (FR-019) | Seed a long canonical ID, a collision pair (`a-b` + `c` and `a` + `b-c`) and services without canonical IDs. Assert the fallback form and that every value passes `storage.ValidateCanonicalID` |
| `TestMigration050AbortsOnGrantServiceWithoutDefinition` | Seed a grant that includes a service the set lacks. Assert that `up` fails, the version is still `049`, the database is not dirty after `Force`, and the data is unchanged |
| `TestMigration050DownRestoresPreUpgradeShape` (US2.3) | Run up, then down, then compare `permission_set_service_scopes` and the grant JSON to the pre-upgrade snapshot. Then run up again and compare to the first up (same canonical IDs) |
| `TestMigration050DownRefusesUnrepresentableData` | One sub-test per down guard. Assert that `down` fails and both schema and data stay at `050` |

The memory adapter has no migrations. Its repositories implement the post-`050` model directly.
