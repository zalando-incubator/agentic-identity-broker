# Quickstart: Validate the Capability Model

**Feature**: `specs/20261127-140720-capability-model` | **Spec**: [spec.md](spec.md) | **Contracts**: [contracts/](contracts/)

This guide shows how to prove that the feature works end to end. It refers to the contracts and does not repeat them.

## Prerequisites

- The toolchain from `CONTRIBUTING.md` (`just install-tools`), with Ginkgo installed.
- Docker or Podman for the infrastructure-backed migration tests.
- `.specify/feature.json` selecting this feature (git-ignored).

## 1. Static checks and unit tests

```bash
just check
just test
```

Expected:

- **Lint**: Passes, including telemetry lint, with no new spans or metrics.
- **Unit tests**:
  - Domain tests for the capability-group invariants, effective capabilities (FR-005 – FR-007), consent derivation (FR-010), exchange coverage (FR-011, FR-012) and the projection's `granted_capabilities` order all pass.
  - ExtProc decode tests reject a permit without `granted_capabilities`.
  - The disabled-mode exact-input test is unchanged and passes.

## 2. Migration up and down against PostgreSQL

```bash
go test -count=1 -tags=integration ./tests/integration/migrations/... -run 'TestMigration050'
```

Expected: every test in [contracts/migration-050.md](contracts/migration-050.md) passes.

- The Journey 2 grant keeps scope `repo:read` only.
- The down migration restores the snapshot.
- Every down guard refuses unrepresentable data.

Run the whole infrastructure-backed layer with `just test-integration-infra`.

## 3. Admin API journey (User Story 1)

```bash
ginkgo -v --focus='Capability Groups|Permission Sets' ./tests/e2e/
```

The tests send the requests from [contracts/admin-api.md](contracts/admin-api.md), with `Info.version 4.0.0`. Expected:

- US1.1: the group from Journey 1, Step 1, returns `201` with two capabilities.
- US1.2: each invalid capability returns `400`, and a later list shows nothing stored.
- US1.3: a set with capability entries is stored. An empty list, duplicate entries and `service_scopes` each return `400`.
- US1.4: an agent entry that relaxes a mandatory capability returns `400`. A subset, a restated mandatory and a tightened optional are accepted.
- US1.5: deleting a referenced capability or group returns `409`, and a set update that invalidates an agent entry returns `409`.

## 4. Upgrade journey (User Story 2)

```bash
ginkgo -v --label-filter='docker' --focus='Capability Model Upgrade' ./tests/e2e/
```

This test needs Docker or Podman. It is labelled `docker`, so run it with `just test-e2e-docker` or add `--label-filter=docker`. It seeds pre-upgrade data at migration `049` (`bootstrap.NewPostgresFixtureAtVersion`), applies the production migrations and starts the dual servers on the migrated store. Expected:

- US2.1 and US2.2: both seeded grants exchange tokens with the same scopes and `granted_permission_sets` as before the migration.
- US2.4: Alice consents through today's `POST /api/consent/agents/{agent-id}/grants` body. The response is unchanged, and exchange yields the pre-upgrade scopes.

## 5. Policy journey (User Story 3)

```bash
ginkgo -v --label-filter='grant-context' --focus='Capability Policy' ./tests/e2e/
just test-e2e-extproc
```

The tests run with the real broker, ExtProc with `grant_context.enabled: true`, and `examples/policies/capability-tool-map.rego`. Expected:

- US3.1: `calendar_create_event` is forwarded. The captured OPA input holds `{id, canonical_id}` references for `google-calendar-edit` and `google-calendar-view`, in that order.
- US3.2: `drive_share_file` is denied before the mock MCP target receives it.
- US3.3: a capability shared by two granted sets appears once.
- US3.4: after the operator removes a granted capability from the set, exchange returns `403 access_denied` with the consent `error_uri`, and the permit is a denial with the consent URL.
- US3.5: a policy that reads only `granted_permission_sets` decides the same as before.
- US3.6: with `grant_context.enabled: false`, the input has no `granted_capabilities`, and the sample policy denies mapped tools.

## 6. Manual smoke test (optional)

1. Start the broker with `configs/` development settings and ExtProc with `examples/config/extproc-grant-context.yaml`.
2. Use the Admin API to create the Journey 1 group, set and agent entry.
3. Consent as a mock user.
4. Call `POST /access/v1/evaluation`. The permit `context.granted_capabilities` lists both Calendar capabilities.

## 7. Full gate before the pull request

```bash
just telemetry-doc   # commit docs/reference/telemetry.md if it changed
just verify
```
