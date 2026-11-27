# Quickstart: Validate Agent Grants

**Feature**: [spec.md](./spec.md) | **Plan**: [plan.md](./plan.md)

This guide lists the runs that prove the feature end to end. The contracts are in
[contracts/admin-agent-grants-openapi.yaml](./contracts/admin-agent-grants-openapi.yaml) and
[contracts/observability.md](./contracts/observability.md). The rules and schema are in
[data-model.md](./data-model.md).

## Prerequisites

- Go, `just` and Ginkgo, as pinned by the repository (`just install-tools`).
- Docker or Podman for the PostgreSQL-backed runs. Without a container runtime, those runs are
  left to CI (Constitution Principle VIII).
- No configuration change: the feature adds no configuration parameter, Helm value or dependency.

## 1. Static checks

```bash
just check
```

Expected: formatting, vet and lint pass, including `telemetrylint` and
`TestLoggedPrincipalsAreTyped`.

## 2. Unit tests

```bash
just test
```

Expected to pass, among others:

| Package | Proves |
|---|---|
| `internal/domain/storage` | `EffectiveScopes` matches the resolver's previous behaviour case by case; `PrincipalKind` values; `AgentGrant` invariants |
| `internal/domain/consent` | Every rule of [data-model.md](./data-model.md#resolution-and-invariants-write-time) in order, with its message; mandatory sets added; copy on write; no-op; preconditions incl. absent grant; re-read on concurrent commit; audit records |
| `internal/domain/tokenexchange` | Existing resolver and evaluation tests unchanged after the Phase 0 extraction |
| `internal/domain/thirdparty` | Bound delete refused with the binding agents; store-level conflict re-reads the agents |
| `internal/adapters/storage/memory` | Kind-prefixed indexes; agent cascade; binding guard lock order under `-race` |
| `internal/adapters/http/handlers/admin` | Status codes, `ETag`, `If-Match` parsing, unknown members, `bound_by_agents` |

## 3. Acceptance tests (memory store)

```bash
ginkgo -v --label-filter="!performance && !docker" --focus "Agent grants" ./tests/e2e/
```

Expected: one passing `It()` for each acceptance scenario of the spec, except the `docker`
scenarios of step 4. The mapping is in [plan.md](./plan.md#scenario-mapping).

Regression check for SC-004 (exchange and delegation check unchanged):

```bash
ginkgo -v --label-filter="!performance && !docker" ./tests/e2e/
```

## 4. PostgreSQL

```bash
just test-integration-infra
ginkgo -v --procs=1 --label-filter="docker" --focus "Agent grants" ./tests/e2e/
```

Expected:

- Migration `051` applies, rolls back and reapplies twice, with existing grants and sessions
  intact and recorded as `human`. Each new constraint holds: the kind check, the agent-principal
  check, both kind-scoped unique constraints, the composite `RESTRICT` key, and the cascade from
  agent to bindings.
- The PostgreSQL adapters pass the same `AgentGrantRepository` and human-scope contracts as the
  memory adapters.
- The `docker` scenarios listed in the plan pass on both stores.

## 5. Manual journey (optional)

Start the broker with the development configuration. Create *Company Platform*, the three
permission sets, *triage-bot* and *planner-bot*, and the two service accounts (#471) as in the
spec's cast. Then replay [Journey 1](./spec.md#concrete-example-journeys-mandatory) against the
admin server.

| Step | Request | Expected |
|---|---|---|
| Grant | `PUT /api/agents/triage-bot/self-grant` with the triage-bot binding | `201`, `ETag: "1"`, `permission_sets` holds *Platform reader* |
| Grant | `PUT /api/agents/planner-bot/self-grant` with *Platform operator* | `201`, both sets, union `read write` within the ceiling |
| Exceed | The planner-bot `PUT` with *Platform admin* | `400` naming `admin`; the grant still has `ETag: "1"` |
| Guard | `DELETE /api/services/company-platform/service-accounts/platform-triage-bot` | `409`, `bound_by_agents: [550e…41]` |
| Revoke | `DELETE /api/agents/triage-bot/self-grant` | `204`; then the account `DELETE` answers `204` |
| Invisible | `GET /api/consent/agents` on the end-user server as any user | No agent grant counted |

The broker log should show `consent.agent_grant.created` twice and `consent.agent_grant.deleted`
once. Refused writes should leave no record.

## 6. Reference pages and full gate

```bash
just telemetry-doc
just verify
```

Expected: `docs/reference/telemetry.md` lists the four `storage.*.agent_grant` spans and is
committed, and the full gate passes.
