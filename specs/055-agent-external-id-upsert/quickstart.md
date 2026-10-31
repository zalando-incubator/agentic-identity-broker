# Quickstart: Validate Unique Agent External IDs and Create-or-Update

**Feature**: [spec.md](./spec.md) | **Contract**: [contracts/admin-agents-openapi.yaml](./contracts/admin-agents-openapi.yaml) | **Data model**: [data-model.md](./data-model.md)

This guide proves the feature end to end after implementation. It does not replace the
automated suites. Status: partial implementation-phase progress; the supplied maintainer approval
is recorded in [ADR 045](../../adrs/045-unique-agent-external-id-upsert.md). Runtime implementation
and all PostgreSQL acceptance work remain incomplete. Do not deploy this feature.

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
| `just test-e2e-docker` | US1.2–US1.3 single labeled scenarios each exercising memory and PostgreSQL multi-URI create/update replay and same-second timestamps; US1.4 two-broker race; US3.1–US3.4 upgrade journeys |

Expected: all pass. `tests/e2e/agent_external_id_filter_test.go` no longer asserts that two agents
share one external ID.

## Current acceptance coverage

The new suite currently authors 15 of 22 scenarios. A focused run compiled and produced 12
missing-feature failures and three unchanged-compatibility passes. Collection PUT currently has only the approved compiling handler scaffold and returns 501
instead of the required 201/200/400/409; ordinary duplicate POST/update still returns 201/200 and
empty-ID POST returns 201 instead of 400. These are semantic red observations, not green delivery.
No pending or skipped markers were added. The existing feature 054 suite remains unchanged until
uniqueness enforcement and its replacement coverage are delivered together.

| Scenario | `tests/e2e/agent_external_id_upsert_test.go` location |
| --- | --- |
| US1.1 | :125 |
| US1.5 | :153 |
| US1.6 | :174 |
| US1.7 | :194 |
| US2.1 | :219 |
| US2.2 | :230 |
| US2.3 | :243 |
| US2.4 | :254 |
| US2.5 | :271 |
| US2.6 | :282 |
| US2.7 | :304 |
| US2.8 | :328 |
| US4.1 | :349 |
| US4.2 | :359 |
| US4.3 | :367 |

Still required: US1.2–US1.4 (both-store replay, microsecond updates and two-broker concurrency),
US3.1–US3.4 (complete upgrade/remediation/rollback journeys), and remaining detailed variants of
T014/T015/T018. Both `just test-e2e-docker` and `just test-integration-infra` failed because no
Docker/Podman runtime is accessible. This is not semantic red evidence and cannot satisfy T019.
A permitted worker with a usable container runtime must complete that gate before behavior work.

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
   and `created_at`, newer `updated_at` even within the same second. Responses expose UTC zero-to-six
   fractional digits at microsecond precision, not whole-second-only timestamps.
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

Automated same-second tests seed a future stored `updated_at` via test fixtures, then issue an
actual update: the minimum advances it by exactly one microsecond, even if wall time is behind it.
The next replay and GET retain that exact fractional timestamp and creation time on both stores.
Service helper tests use fixed clock values; do not use sleeps or a new production clock API.
The timestamp is not an ETag or a concurrent total ordering. Proposed API-ORDER/API-TIMESTAMPS
require sign-off along with the other items before implementation.

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
