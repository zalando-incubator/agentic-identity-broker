# Storage Integration Tests

## Scope

[Memory lifecycle tests](lifecycle_test.go) run without containers.
The [infra package](infra/) contains PostgreSQL adapter and third-party service tests under the `integration` build tag.
Additional PostgreSQL repository tests live in [internal/adapters/storage/postgres/](../../../internal/adapters/storage/postgres/).

Read the [integration instructions](../AGENTS.md) before you add tests.
The [justfile](../../../justfile) defines package selection and execution commands.

## Running tests

Run these commands from the repository root ([integration instructions](../AGENTS.md)).

For self-contained storage tests:

```bash
go test -v ./tests/integration/storage/...
```

This package uses the memory adapter in [lifecycle_test.go](lifecycle_test.go).
`just test-integration` runs the wider self-contained integration suite ([recipe](../../../justfile)).

For PostgreSQL-backed storage tests, provide an accessible Docker or Podman runtime.
The [runtime helper](../bootstrap/postgres.go) checks `docker ps`, then `podman ps`.

```bash
go test -count=1 -tags=integration -v ./tests/integration/storage/infra/...
go test -count=1 -tags=integration -v ./internal/adapters/storage/postgres/...
```

These commands select the storage packages from the [infrastructure recipe](../../../justfile).
The recipe uses `-count=1` to prevent cached results from replacing a live infrastructure run.

For all infrastructure-backed integration packages, use `just test-integration-infra`.
For both integration layers, use `just test-integration-all` ([recipes](../../../justfile)).
The infrastructure recipe also includes migration and encryption tests.
`INTEGRATION_INFRA_PACKAGE_PROCS` controls package parallelism and defaults to two ([package configuration](../../../justfile)).

## Coverage map

This map describes source coverage, not a test total or pass report:

| Source | Scope |
|---|---|
| [lifecycle_test.go](lifecycle_test.go) | Memory lifecycle, concurrent operations, cancellation, pagination, and error recovery |
| [infra/postgres_test.go](infra/postgres_test.go) | Adapter error paths and CIMD signing-key persistence |
| [infra/thirdparty_service_test.go](infra/thirdparty_service_test.go) | Authorization parameters and protected-resource lookup behavior |
| [PostgreSQL repository package](../../../internal/adapters/storage/postgres/) | Repository-level integration tests |
| [infra/main_test.go](infra/main_test.go) | Shared PostgreSQL teardown after the package finishes |

The repository tests cover approval, permission-set, and grant transactions, expiry, conflicts, and failed writes.
[TestUserSessionReadsAfterSchemaExpansion](../../../internal/adapters/storage/postgres/user_session_reliability_test.go)
adds an unmapped `future_metadata` column. It checks that session reads and refresh writes preserve that column.
Keep this test column separate from production columns when you add migrations.

[TestProtectedResourceUpdatePreservesUnchangedCreatedAt](infra/thirdparty_service_test.go)
covers omitted resources, normalized-equivalent membership, replacement, empty membership, and stale CAS through the domain service.
For unchanged membership, it checks protected-resource ownership, creation timestamps, and the parent version increment.

Names alone do not prove coverage.
`TestPostgresAdapter_FullLifecycle_Integration` does not apply a schema or assert a successful full lifecycle ([test body](infra/postgres_test.go#L288-L342)).
`TestPostgresAdapter_CRUD_Operations_Simulation` exercises an uninitialized adapter, not successful persisted CRUD ([test body](infra/postgres_test.go#L344-L395)).
Use the migrated database harness in [thirdparty_service_test.go](infra/thirdparty_service_test.go) for an example of real persistence assertions.

## Database schema setup

For ordinary PostgreSQL tests, call `bootstrap.RequireSharedPostgres(t)`, then `(*SharedPostgres).SetupMigratedDatabase(t)`.
The [helper](../bootstrap/postgres.go) discovers every migration in `migrations/` with `golang-migrate`.
It prepares one fully migrated template per package and clones an isolated database for each test.
New migrations require no test fixture version or filename updates.
Application-level journeys must use the current adapter schema, including credential-source migration 041.
The PostgreSQL adapter package and PostgreSQL E2E fixture also apply all discovered migrations.

Only migration-specific upgrade and rollback tests can request a historical schema.
For those tests, use `SetupDatabaseFromTemplate` with `ApplyMigrationsUpTo(t, dbName, version)`.
Do not maintain SQL filename lists.
Connection-error and uninitialized-adapter tests deliberately do not provision a schema.

`just test-integration-infra` includes the [bootstrap regression tests](../bootstrap/).
These tests cover current-schema discovery, clone isolation, and historical fixture upgrades, including concurrent-index migrations.

## Container lifecycle

The [shared bootstrap](../bootstrap/postgres.go) starts `postgres:15-alpine` once per test process.
Its template helper creates a database clone and returns a cleanup function.
The [third-party service harness](infra/thirdparty_service_test.go) uses that helper for isolated databases.

The [storage package's `TestMain`](infra/main_test.go) calls `TerminateSharedPostgres` after `m.Run()` and before `os.Exit`.
Termination errors go to stderr. They do not change the test exit code.
The separate [adapter lifecycle test](infra/postgres_test.go#L288-L342) uses deferred termination for its own container.

`TESTCONTAINERS_RYUK_DISABLED=true` disables the reaper, not explicit termination.
The [shared bootstrap](../bootstrap/postgres.go) and [storage package initialization](infra/postgres_test.go) set this value when it is unset.
It does not preserve containers for debugging: `TestMain` and deferred cleanup still request termination ([teardown](infra/main_test.go), [lifecycle test](infra/postgres_test.go#L288-L342)).

These cleanup paths do not guarantee removal after a forced process exit or failed termination.
The [teardown implementation](infra/main_test.go) depends on control reaching the cleanup call and on the runtime accepting termination.

## Runtime failures and troubleshooting

The [shared bootstrap](../bootstrap/postgres.go) skips locally when the runtime is unavailable or container startup fails.
When `CI` is nonempty, those paths fail the test instead.
The helper also skips in short mode, so do not use `-short` as evidence of infrastructure coverage.
The [standalone adapter lifecycle test](infra/postgres_test.go#L288-L342) uses the same local-versus-CI distinction.

For runtime errors, inspect the relevant runtime with `docker ps` or `podman ps` ([runtime helper](../bootstrap/postgres.go)).
Then rerun the storage command with `-count=1 -tags=integration -v`, as shown in [the infrastructure recipe](../../../justfile).

For image or startup errors, inspect the [container request](../bootstrap/postgres.go).
It specifies `postgres:15-alpine` and waits for the PostgreSQL readiness log.
Do not assume that a local skipped test exercised PostgreSQL.

For cleanup errors, inspect stderr for the message from [TestMain](infra/main_test.go).
Inspect individual containers before manual removal. Do not remove every container with the PostgreSQL image.
The [bootstrap](../bootstrap/postgres.go) uses a shared image, not an image unique to this test run.

## CI and result evidence

The [CI workflow](../../../.github/workflows/ci.yml) runs `just verify-integration-self-contained-junit`, not `just verify-integration-junit`.
That [recipe](../../../justfile) writes `test-results/integration-self-contained-junit.xml` without the `integration` build tag.
That report cannot establish coverage of tagged PostgreSQL or migration infrastructure tests.

For a local run of both integration layers, use `just verify-integration-junit` ([recipe](../../../justfile)).
It writes separate `integration-self-contained-junit.xml` and `integration-infra-junit.xml` reports, then merges them into `integration-junit.xml`, all under `test-results/`.
The `verify-integration-infra-junit` recipe uses `INTEGRATION_INFRA_GO_TEST_ARGS`, which includes `-count=1 -tags=integration`.

Use the report from the relevant run for passed, failed, and skipped tests.
This guide does not claim a fixed test count, pass status, or execution time.
