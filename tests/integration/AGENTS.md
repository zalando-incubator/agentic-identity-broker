# Integration Tests (`tests/integration/`)

All paths in this file are relative to the repository root. Run commands from the repository root.

Read `tests/AGENTS.md` for the test architecture and governing sources.
Read the suite helpers before you add infrastructure-backed tests.
Use `tests/integration/storage/README.md` for storage coverage and runtime troubleshooting.

## Suites and Commands

Use standard `go test` with testify assertions. Use `require` for fatal assertions and `assert` for nonfatal assertions.
Organize related cases with `t.Run`.

Self-contained tests use in-memory adapters, `httptest`, or local configuration and chart tooling.
Container-backed tests require the `integration` build tag and a compatible Docker or Podman runtime.
The infra recipe includes PostgreSQL repository tests under `internal/adapters/storage/postgres/`.

```bash
go test -v ./tests/integration/...
go test -count=1 -tags=integration -p 2 -v ./tests/integration/infra/... \
  ./tests/integration/migrations/... \
  ./tests/integration/storage/infra/... \
  ./internal/adapters/storage/postgres/...
just test-integration
just test-integration-infra
just test-integration-all
```

Use `-count=1` for live infrastructure coverage. A cached success must not bypass the runtime requirement.
`INTEGRATION_INFRA_PACKAGE_PROCS` controls package parallelism in `justfile` and defaults to `2`.

## Directory and Coverage Map

| Path | Purpose |
|---|---|
| `tests/integration/bootstrap/postgres.go` | Shared container, template databases, SQL helpers |
| `tests/integration/bootstrap/aws_emulator.go` | AWS emulator lifecycle, KMS key, DynamoDB table, SDK environment |
| `tests/integration/bootstrap/*_test.go` | Bootstrap and cleanup behavior |
| `tests/integration/infra/main_test.go` | Shared AWS emulator and PostgreSQL teardown |
| `tests/integration/infra/*_test.go` | Encryption keyring, signing keys, targeted migration assertions |
| `tests/integration/storage/lifecycle_test.go` | Memory adapter lifecycle and concurrency |
| `tests/integration/storage/infra/postgres_test.go` | PostgreSQL adapter behavior |
| `tests/integration/storage/infra/thirdparty_service_test.go` | Third-party service repository behavior |
| `tests/integration/storage/infra/main_test.go` | Shared PostgreSQL teardown |
| `tests/integration/migrations/doc.go` | Integration build-tag marker |
| `tests/integration/migrations/framework.go` | PostgreSQL and go-migrate lifecycle helpers |
| `tests/integration/migrations/main_test.go` | Shared PostgreSQL teardown |
| `tests/integration/migrations/migrations_test.go` | Migration apply and rollback coverage |
| `tests/integration/http_wait_test.go` | Local HTTP readiness helper |
| `tests/integration/oauth2_*_test.go`, `tests/integration/server_test.go` | HTTP endpoint behavior |
| `tests/integration/config_test.go`, `tests/integration/principal_middleware_test.go` | Configuration and principal middleware |
| `tests/integration/*_test.go` | Additional session, agent, health, audit, and Helm coverage |

## Container Lifecycle

Local suites can skip unavailable infrastructure through their existing bootstrap helpers.
In CI, missing or failed infrastructure must fail the suite.
Do not add skips that hide missing feature behavior.

The bootstrap helpers default `TESTCONTAINERS_RYUK_DISABLED` to `true` unless the environment already sets it.
This disables the reaper, not explicit teardown.
Close test resources with `defer` or `t.Cleanup`.
End shared containers in `TestMain` before `os.Exit`.
Restore environment variables after tests that change them.

## Helper Inventory

### Shared PostgreSQL bootstrap

Read `tests/integration/bootstrap/postgres.go` before you write a PostgreSQL-backed test.

Use these helpers:

- `bootstrap.RequireSharedPostgres(t)` — Starts or reuses one PostgreSQL container for the package.
- `(*SharedPostgres).SetupDatabaseFromTemplate(...)` — Clones a fresh database from a prepared template.
- `(*SharedPostgres).ApplyMigrationsUpTo(...)` — Applies a bounded migration set into a template DB.
- `(*SharedPostgres).ApplyMigration(...)`, `QuerySQL(...)`, `ExecuteSQL(...)` — SQL helpers.
- `bootstrap.TerminateSharedPostgres(ctx)` — Call from `TestMain` for a shared container.

Use this pattern:

1. Start one shared container per package.
2. Build a template DB once.
3. Clone a fresh DB per test.
4. Close adapters before the clone cleanup function deletes the database.

For small CRUD cases against one schema, reuse a clone in sequential `t.Run` subtests:

1. Clone one database for the test group.
2. Reuse one adapter/repository across sequential `t.Run(...)` subtests.
3. Generate fresh fixtures inside each subtest.
4. If necessary, isolate special global-state assertions.

If a test does not cover container startup, do not start one PostgreSQL container per test.

### AWS emulator bootstrap

`tests/integration/bootstrap/aws_emulator.go` provides:

- `StartAWSEmulator()`
- `StartAWSEmulatorForSuite()`

If multiple tests in a package need KMS and DynamoDB, use the suite-level variant.
Read `tests/integration/infra/main_test.go` for container teardown and `CleanupEnvironment()`.
The helper uses a LocalStack-compatible emulator, not live AWS.

### HTTP readiness helper

Within the top-level integration package, use `waitForEndpoint(t, url)` from `tests/integration/http_wait_test.go`.
Do not use fixed `time.Sleep(...)` delays for readiness.

## Performance Rules

### 1. Keep infra packages parallel-safe

The infra recipes default to package parallelism of `2`.

Apply these rules:

- Do not share mutable files across packages.
- Do not assume a package execution order.
- Restore package-global environment mutations.
- Isolate top-level test groups with cloned databases. Share a clone only among sequential, independently seeded subtests.

### 2. Prefer cloned databases over repeated migrations

If several tests need the same migrated schema:

- Create a template DB once.
- Clone it for each test.
- For small repository assertions, share one clone only across sequential subtests in the same test group.

### 3. Bounded failure-path tests only

For timeout, retry, or cancellation scenarios:

- Shorten retry delays or TTLs in the test configuration.
- Use handlers that unblock on context cancellation. Use a short fallback timeout.

If duration is not the behavior under test, do not use multi-second sleeps.

### 4. Use SQL helpers for migration assertions

Use `QuerySQL` and `ExecuteSQL` for migration assertions. Focus on:

- Schema existence.
- Column or index presence.
- Representative data behavior.

### 5. How to keep shared-DB subtests stable

If subtests share a cloned database, apply these rules:

- **Unique data per subtest** — Create unique IDs and other fixtures.
- **No `t.Parallel()` in shared DB tests** — These subtests are sequential.
- **Do not use global counts**. Store a baseline or filter for subtest records.
- **Prefer presence or absence checks**. Do not use exact table counts.
- **Separate special cases** that need a pristine table. Also separate cases that need global process state.
- **Keep global-state tests isolated** — for example, OpenTelemetry global provider replacement.
- **Do not rely on subtest order**. Use independent fixtures in each subtest.

## Package Guidance

### `tests/integration/infra/`

- Use this directory for migration-specific or AWS-emulator-backed assertions.
- Put shared suite lifecycle in `tests/integration/infra/main_test.go`.
- For schema changes, reuse the shared PostgreSQL helper.

### `tests/integration/migrations/`

- Use `tests/integration/migrations/framework.go` for migration lifecycle tests.
- Keep migration coverage representative. Add per-column coverage only as risk requires.

### `tests/integration/storage/infra/`

- Use the shared PostgreSQL bootstrap.
- Keep repository behavior isolated per cloned DB.
- Reuse adapter setup helpers. Do not rebuild them in each test.

### Top-level self-contained tests

- Use polling for server readiness.
- If the behavior does not need containers, use in-memory adapters.
- Use the broker configuration loader for example coverage. Read `tests/integration/config_test.go` for effective-value assertions.
- Assert behavior, not documentation wording or source comments.

## Anti-patterns

Flag these in review:

- One container per test for behavior that can use a shared template
- Full migrations in each test body for an unchanged schema
- Fixed sleeps for server startup or readiness
- Production-sized delays in retry or cancellation tests
- Package-global state or shared files that prevent safe package parallelism.
