# Integration Tests (`tests/integration/`)

Read the suite helpers before you add infrastructure-backed tests.

This directory has two integration test styles:

- **Self-contained integration** — `go test ./tests/integration/...`
- **Infra-backed integration** — `go test -tags=integration ...`

## Helper Inventory

### Shared PostgreSQL bootstrap

Read `tests/integration/bootstrap/postgres.go` before you write a PostgreSQL-backed test.

Use these helpers:

- `bootstrap.RequireSharedPostgres(t)` — Starts or reuses one PostgreSQL container for the package.
- `(*SharedPostgres).SetupDatabaseFromTemplate(...)` — Clones a fresh database from a prepared template.
- `(*SharedPostgres).ApplyMigrationsUpTo(...)` — Applies a bounded migration set into a template DB.
- `(*SharedPostgres).ApplyMigration(...)`, `QuerySQL(...)`, `ExecuteSQL(...)` — SQL helpers.
- `bootstrap.TerminateSharedPostgres(ctx)` — Call from `TestMain` for a shared container.

**Preferred pattern:**

1. Start one shared container per package.
2. Build a template DB once.
3. Clone a fresh DB per test.
4. Close adapters and drop the cloned DB in `defer`.

For many small CRUD assertions against one schema, reuse a cloned database in sequential `t.Run(...)` subtests:

1. Clone one DB for the test group (often one top-level `TestXxxRepo`).
2. Reuse one adapter/repository across sequential `t.Run(...)` subtests.
3. Generate fresh fixtures inside each subtest.
4. If necessary, isolate special global-state assertions.

If a test does not cover container startup, do not start one PostgreSQL container per test.

### AWS emulator bootstrap

`tests/integration/bootstrap/aws_emulator.go` provides:

- `StartAWSEmulator()`
- `StartAWSEmulatorForSuite()`

If multiple tests in a package need KMS and DynamoDB, use the suite-level variant.

### HTTP readiness helper

Use `waitForEndpoint(t, url)` for HTTP readiness. Do not use fixed `time.Sleep(...)` delays.

## Performance Rules

### 1. Keep infra packages parallel-safe

CI uses package parallelism: `go test -tags=integration -p 2 ...`.

Apply these rules:

- Do not share mutable files across packages.
- Do not assume a package execution order.
- Restore package-global environment mutations.
- Isolate databases per test with cloned DBs. Do not use shared mutable schemas.

### 2. Prefer cloned databases over repeated migrations

If several tests need the same migrated schema:

- Create a template DB once.
- Clone it for each test.
- For very small repo assertions, you can share one cloned DB across sequential subtests in the same file.

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

### `infra/`

- Use this directory for migration-specific or AWS-emulator-backed assertions.
- Put shared suite lifecycle in `main_test.go`.
- For schema changes, reuse the shared PostgreSQL helper.

### `migrations/`

- Use `framework.go` for migration lifecycle tests.
- Keep migration coverage representative. Add per-column coverage only as risk requires.

### `storage/infra/`

- Use the shared PostgreSQL bootstrap.
- Keep repository behavior isolated per cloned DB.
- Reuse adapter setup helpers. Do not rebuild them in each test.

### top-level self-contained tests

- Prefer polling to sleeps for server readiness.
If the behavior does not need real infra, use in-memory adapters.
- Use the broker configuration loader for example configuration coverage. See `config_test.go` for assertions on effective values.
- Assert behavior, not documentation wording or source comments.

## Anti-patterns

Flag these in review:

- For a shared template, do not use one Docker/Postgres container per test.
- Repeated full migration application in each test body.
- Fixed sleeps for server startup or readiness.
- Retry or cancellation tests that depend on production-sized delays.
- Package-global state that prevents safe `-p 2` execution.
