---
title: Agent external-ID uniqueness upgrade
description: Inventory and deliberately remediate legacy agents before migration 046, with guarded retry and non-destructive rollback.
---

# Upgrade to unique Agent external IDs

**Delivery status:** Feature 055 has maintainer approval but implementation and PostgreSQL
acceptance validation are not complete. This runbook describes the approved upgrade procedure;
do not deploy the feature until its mandatory tests and migration implementation pass.

Admin API `3.0.0` replaces `2.1.0` on the existing routes. A set `external_id` must be non-empty
and unique by exact, case-sensitive equality. Multiple agents may have no external ID (`NULL`).
Existing POST and ID-addressed PUT reject duplicates with `409 conflict` and empty strings with
`400 validation failed`. Filtered GET returns zero or one Agent in an array. Consumers must also
accept byte-sorted `client_uris` and UTC microsecond timestamps rather than whole-second-only
presentation. [ADR 045](../../adrs/045-unique-agent-external-id-upsert.md) records approval.

## 1. Prepare and coordinate writers

1. Back up the database and record the deployed broker/migration image versions.
2. Pause governance sync jobs and administrative writes. Keep the existing authenticated admin
   proxy available for deliberate remediation; keep its Host, principal, cross-origin and JSON
   protections unchanged. Restrict writers operationally, not by weakening authentication.
3. Use the existing broker at schema version 45 to perform remediation. It supports full
   ID-addressed PUT and can read all legacy records before the new constraints apply.
4. After remediation, stop all old writers, including old broker replicas, before migration.
   Do not resume mixed old/new writers. Migration obtains `ACCESS EXCLUSIVE MODE NOWAIT` on
   `agents`; it fails immediately if another transaction holds a conflicting lock, and its
   statement timeout is five seconds. Wait for active transactions to finish before retrying.
5. Use the separate version-matched migration image and migration role. Do not add schema
   privileges or migration tooling to the runtime broker. The Helm pre-upgrade Job remains the
   deployment owner in both grants modes (ADR 009).

Record a baseline of Agent UUIDs and relationships in a controlled operator record. Counts are
useful, but are not proof of unchanged relationships; compare identifiers and definitions too.
Do not include credentials or token material in the record.

```sql
SELECT id, canonical_id, display_name, external_id, permission_sets, service_requirements,
       created_at, updated_at
FROM agents
ORDER BY id;
SELECT id, agent_id FROM user_grants ORDER BY id;
SELECT agent_id, client_uri FROM agent_client_uris ORDER BY agent_id, client_uri;
```

## 2. Inventory every affected Agent

Run read-only queries with the application schema search path. Do not append a LIMIT: these
queries provide the complete inventory, including entries omitted from bounded error messages.

```sql
SELECT external_id, array_agg(id ORDER BY created_at, id) AS agent_ids,
       COUNT(*) AS agent_count
FROM agents
WHERE external_id IS NOT NULL AND external_id <> ''
GROUP BY external_id
HAVING COUNT(*) > 1
ORDER BY external_id;

SELECT a.id, a.canonical_id, a.display_name, a.external_id, a.created_at, a.updated_at
FROM agents AS a
JOIN (
    SELECT external_id
    FROM agents
    WHERE external_id IS NOT NULL AND external_id <> ''
    GROUP BY external_id
    HAVING COUNT(*) > 1
) AS duplicates ON duplicates.external_id = a.external_id
ORDER BY a.external_id, a.created_at, a.id;

SELECT id, canonical_id, display_name, external_id, created_at, updated_at
FROM agents
WHERE external_id = ''
ORDER BY created_at, id;
```

Whitespace is not empty. Do not trim, case-fold, normalize, or suffix identifiers automatically.
An external ID can be a URN, a URL, or a UUID-shaped opaque value; it is never a broker UUID selector.

A blocked migration reports `agent external_id uniqueness migration blocked`. Its DETAIL contains
both duplicate groups (value and Agent UUIDs) and empty-value Agent UUIDs: the first 100 of each
category plus remainder counts. A HINT points here and states that no Agents were changed. A zero
remainder means the bounded report covers that category; otherwise use the complete SQL inventory.
Lock/timeout errors require writer coordination, not data cleanup. No failed attempt selects a
survivor, deletes records, changes UUIDs, or reassigns grants.

## 3. Remediate deliberately through the existing Admin API

For each affected Agent, decide whether it should have a distinct registry identifier or no
external ID. Keep every Agent UUID and its relationships. Do not DELETE or merge agents.

Read the Agent by UUID using `GET /api/agents/{uuid}`. Prepare a complete `AgentUpdateRequest`
from the existing definition, including required `display_name`, `description`, nonempty
`permission_sets`, all metadata URLs and optional collections that must remain. Copy only
writable properties, not timestamps or `id`. ID-addressed PUT replaces metadata and collections;
a minimal body can clear them. Preserve omitted `client_id`/`canonical_id` according to their
existing rules, or repeat the current values. Never substitute collection PUT for legacy repair.

The following synthetic example assigns the EU agent a registry-approved distinct ID. In a real
request retain its existing complete permission declarations, service requirements and collections.

```http
PUT /api/agents/6a1f4c2e-93b7-4d1a-8c55-0e7d2b9f4a10 HTTP/1.1
Host: broker-admin.example.com
Content-Type: application/json
X-Remote-User: admin@example.com

{"display_name":"Research Assistant (EU)","description":"EU research deployment","external_id":"urn:example:ai-registry:agent:7f3a9c-eu","permission_sets":[{"permission_set_id":"a1b2c3d4-e5f6-7890-abcd-ef1234567890","requirement_type":"mandatory"}]}
```

To remove Data Analyst's empty ID, use ID-addressed PUT with `external_id` omitted (or null), while
retaining the rest of its writable definition. This clears the ID, not the Agent. Read each Agent
back and compare UUID, permissions, service requirements, client URIs and grants against the
baseline. Intentional remediation changes external IDs and update timestamps only. Repeat the
full inventory and require both problem categories to be empty.

## 4. Inspect and recover a blocked migration

`golang-migrate` records a dirty version separately from the atomic SQL statement. Do not let a
Job restart loop clear this flag, and never run `force` automatically. Stop competing migration
Jobs and inspect with the migration role:

```sql
SELECT version, dirty FROM schema_migrations;
SELECT to_regclass('idx_agents_external_id') AS old_index,
       to_regclass('uq_agents_external_id') AS unique_index;
SELECT i.indexrelid::regclass AS index_name, i.indrelid::regclass AS table_name,
       i.indisunique, i.indisvalid, pg_get_indexdef(i.indexrelid) AS definition
FROM pg_index AS i
WHERE i.indexrelid IN (to_regclass('idx_agents_external_id'),
                      to_regclass('uq_agents_external_id'));
SELECT conname, convalidated, pg_get_constraintdef(oid) AS definition
FROM pg_constraint
WHERE conrelid = 'agents'::regclass AND conname = 'chk_agents_external_id_not_empty';
```

After an ordinary blocked UP, require **dirty version 46**, the valid old non-unique partial index
`idx_agents_external_id ON agents(external_id) WHERE external_id IS NOT NULL`, no
`uq_agents_external_id`, and no `chk_agents_external_id_not_empty`. Verify all pre-attempt rows and
relationships are intact and inspect the original error. Only when actual schema still matches
version 45 may the operator run the migration image's `migrate ... force 45`, then `migrate ... up`.
`force` changes metadata only; it does not validate or apply SQL.

If any expected object is missing, a new object is already present, the version is unexpected,
or index identity/definition differs, stop the release for database review. Do not drop objects
or clear metadata blindly. A completed SQL statement with a subsequent metadata failure needs
review of actual version-46 state, not the blocked-UP `force 45` procedure.

## 5. Apply and verify before resuming writers

Apply migration 046 through the separate migration image/Helm Job. Clean UP must end at version
46 (or the intended later clean version if later migrations are present). Reuse the inspection
queries above and require:

- Valid unique partial index `uq_agents_external_id` on `agents(external_id)` where non-null.
- Validated `chk_agents_external_id_not_empty CHECK (external_id <> '')`.
- No old `idx_agents_external_id`.
- Empty duplicate/empty inventory; unchanged UUIDs and all relationships relative to the
  deliberately remediated baseline.

Start only the new broker. Through the authenticated admin proxy verify exact filtered GET
returns one Agent for each assigned external ID and `[]` for an unknown one. In a disposable
acceptance fixture, duplicate ordinary POST/update returns 409 with no mutation, empty IDs return
400, collection PUT returns 201 then identical 200 replays, and multiple absent IDs succeed.
Never create a deliberate duplicate in production as a validation step. Resume governance jobs
only after the mandatory migration and complete E2E suites pass.

## 6. Rollback and reapply

Coordinate all writers and take a new baseline. From clean version 46 only, use the migration
image's `migrate ... down 1` to return to clean version 45. If later versions are present, follow
their reviewed downgrade procedures first; `down 1` is not necessarily migration 046.

DOWN restores the valid old non-unique partial index, removes the non-empty check and unique index,
and changes no Agent or relationship row. Verify those objects and the baseline explicitly before
starting a compatible older broker. It permits legacy duplicates/empties again; therefore repeat
the complete inventory and deliberate repair before reapplication. Reapply UP through the same
runner and verify version, schema, UUIDs and relationships again.

An interrupted DOWN or an unexpected dirty version requires database review using actual object
state; do not apply the blocked-UP `force 45` recipe. Rollback does not undo deliberate registry
ID choices and does not remove grants.
