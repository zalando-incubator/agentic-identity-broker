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

Names alone do not prove coverage.
`TestPostgresAdapter_FullLifecycle_Integration` does not apply a schema or assert a successful full lifecycle ([test body](infra/postgres_test.go#L288-L342)).
`TestPostgresAdapter_CRUD_Operations_Simulation` exercises an uninitialized adapter, not successful persisted CRUD ([test body](infra/postgres_test.go#L344-L395)).
Use the migrated database harness in [thirdparty_service_test.go](infra/thirdparty_service_test.go) for an example of real persistence assertions.

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

The [CI workflow](../../../.github/workflows/ci.yml) runs `just verify-integration-junit`.
That [recipe](../../../justfile) runs both integration layers and writes JUnit reports under `test-results/`.
The infrastructure invocation includes `-count=1` and the `integration` build tag.

Use the report from the relevant run for passed, failed, and skipped tests.
This guide does not claim a fixed test count, pass status, or execution time.
