# Quickstart: Validate Unique Agent External IDs and Create-or-Update

**Feature**: [spec.md](./spec.md) | **Contract**: [contracts/admin-agents-openapi.yaml](./contracts/admin-agents-openapi.yaml) | **Data model**: [data-model.md](./data-model.md)

This guide proves the feature end to end after implementation. It does not replace the
automated suites. [ADR 045](../../adrs/045-unique-agent-external-id-upsert.md) records the revised
decision approved by mjungsbluth on 2026-11-04:
https://github.com/zalando-incubator/agentic-identity-broker/pull/336#issuecomment-6625694887.
Current timestamp implementation and validation evidence is in [tasks.md](./tasks.md).
PostgreSQL and container-backed acceptance validation runs in the workflow on the pushed commit
and remains a release gate.

## Prerequisites

- Go toolchain and tools from `just install-tools`.
- Docker or Podman for the PostgreSQL suites (`just test-e2e-docker`, `just test-integration-infra`).
- `curl` and `jq` for the manual checks.
- An existing permission set with canonical ID `github-read`.

## Automated validation

| Command | Proves |
|---|---|
| `just check` | Formatting, vet, lint |
| `just test` | Domain reconcile policy, no-op equality, validation, memory uniqueness, handler parsing |
| `just test-integration-infra` | Migration `046` block/apply/rollback/reapply and PostgreSQL conflict mapping (US3, DB-003) |
| `just test-e2e-backend` | US1.1, US1.5–US1.7, US2.1–US2.8, US4.1–US4.3 on the in-memory broker |
| `just test-e2e-docker` | US1.2–US1.3 single labeled scenarios each exercising memory and PostgreSQL multi-URI create/update replay with unchanged replay timestamps; US1.4 two-broker race; US3.1–US3.4 upgrade journeys |

Expected: all pass. `tests/e2e/agent_external_id_filter_test.go` no longer asserts that two agents
share one external ID.

## Acceptance mapping and previous validation

All 22 scenarios have one mapped `It()` with the revised timestamp assertions. The current
backend run passes all 15 in-memory feature scenarios (928 total passing specs). Seven
PostgreSQL/both-store scenarios are validated by the workflow’s container tests on the pushed
commit, not claimed passed locally. The superseded feature-054 shared-ID test and fixture remain
removed; its retained scenarios are regression coverage.

| Scenario | `tests/e2e/agent_external_id_upsert_test.go` location |
| --- | --- |
| US1.1 | :129 |
| US1.2 | :144 |
| US1.3 | :175 |
| US1.4 | :226 |
| US1.5 | :304 |
| US1.6 | :325 |
| US1.7 | :345 |
| US2.1 | :378 |
| US2.2 | :389 |
| US2.3 | :402 |
| US2.4 | :413 |
| US2.5 | :430 |
| US2.6 | :441 |
| US2.7 | :463 |
| US2.8 | :529 |
| US3.1 | :620 |
| US3.2 | :629 |
| US3.3 | :636 |
| US3.4 | :665 |
| US4.1 | :684 |
| US4.2 | :694 |
| US4.3 | :702 |

The field/error/preference matrices and migration inventory/relationship checks remain intact;
T014/T030/T031 now cover current-time assignment and whole-second UTC presentation.
Migration tests verify complete inventories beyond the first 100 entries and preserve grants, credentials,
authorization codes, refresh sessions, approvals and client URIs. Container-backed red/green evidence
comes from CI on the pushed commit; it is not claimed as local validation.

## Manual journey (in-memory broker)

Start the broker with the admin plain-header configuration used in the spec:

```bash
just run
ADMIN=http://localhost:14000
H=(-H 'X-Remote-User: ai-registry-sync@example.com' -H 'Content-Type: application/json' -H 'Prefer: reference-id=canonical')
SEL='urn%3Aexample%3Aai-registry%3Aagent%3A7f3a9c'
BODY='{"canonical_id":"research-assistant","display_name":"Research Assistant","description":"Helps with academic research","permission_sets":[{"permission_set_id":"github-read","requirement_type":"mandatory"}]}'
```

1. **Create** (US1.1): `curl -si -X PUT "${H[@]}" "$ADMIN/api/agents?external_id=$SEL" -d "$BODY"` →
   `201`, `Location: /api/agents/<uuid>`, `Preference-Applied: reference-id=canonical`.
2. **Replay** (US1.2): repeat step 1 → `200`, identical body including `updated_at`.
3. **Change** (US1.3): replace the display name with `Research Assistant v2` → `200`, same `id`
   and `created_at`, and `updated_at` at the update time in whole-second RFC 3339 form. Within the
   same second as the previous write, `updated_at` may be unchanged; replaying the change returns
   the identical body.
4. **Lookup** (US4.1): `curl -s "${H[@]}" "$ADMIN/api/agents?external_id=$SEL" | jq length` → `1`.
5. **Duplicate on create** (US2.1): `POST /api/agents` with the same `external_id` and another
   canonical ID → `409`, `external_id is already held by another agent`.
6. **Bad selectors** (US2.4): `PUT /api/agents` without a query, with `external_id=`, and with
   `external_id=a&external_id=a` → `400 invalid external_id` each; step 4 still returns one agent.
7. **Body mismatch and null** (US2.5): body `external_id` set to another value or `null` → `400
   invalid external_id`.
8. **Null client ID** (US1.7): body with `"client_id": null` → `400 validation failed`.

Repeat create and update with at least two valid client URIs supplied in reverse lexical order,
for example `https://agent.example.com/z-client.json` then `https://agent.example.com/a-client.json`.
Every Admin Agent response returns the byte-sorted pair, including the immediate write response,
replay, POST/ID-addressed PUT and single/list/filter GET. Reordered equal URI sets are also no-ops;
other collection ordering and optional omission remain unchanged. Use a fresh selector when
repeating the manual journey on PostgreSQL; compare full response bodies under each `Prefer` choice.

Automated update tests assert the changed definition, preserved `id` and `created_at`, current-time
assignment and whole-second `updated_at` presentation; the next replay and GET return the identical
representation, including `updated_at`, on both stores, with no replay write. They use no sleeps or
production clock API. The timestamp is not an ETag or a concurrent total ordering. The approval
cited in ADR 045 covers all eight current items, including API-ORDER and the timestamp revision.

Journey examples with exact bodies: [spec.md, Concrete Example Journeys](./spec.md#concrete-example-journeys-mandatory).

## Manual upgrade check (PostgreSQL)

Follow `docs/operations/agent-external-id-uniqueness.md` against a scratch database:

1. Migrate to version `45`; insert two agents sharing one external ID and one agent with `''`.
2. Run `migrate up` → fails with `agent external_id uniqueness migration blocked`; `DETAIL` lists the
   duplicate value, both UUIDs, and the empty-value UUID. Agent and grant counts are unchanged.
3. Run the runbook's inventory queries; remediate through the Admin API; `migrate force 45` after
   confirming the unchanged schema; `migrate up` → succeeds.
4. Verify `uq_agents_external_id` and `chk_agents_external_id_not_empty` exist and counts match.
5. `migrate down 1` then `migrate up` → succeeds with all agents and grants unchanged.
