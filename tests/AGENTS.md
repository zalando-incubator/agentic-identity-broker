# Tests — E2E & Integration (`tests/`)

Read changed tests and adjacent READMEs before you write a test. Use `tests/e2e/README.md` for broker HTTP E2E tests. Use `tests/integration/storage/README.md` for storage integration tests.

## Overview

Use the suite that matches the test scope and infrastructure:

Read the suite-specific guidance before you change either test layer.

| Suite | Framework | Scope | Storage |
|---|---|---|---|
| `e2e/` | Ginkgo v2 / Gomega | Full system | In-memory or Docker |
| `integration/` | Standard `go test` / testify | Component-level | Self-contained or infra-backed |

ExtProc agentgateway E2E tests use Docker. Infra-backed integration tests use PostgreSQL
and an AWS emulator. They require Docker or Podman.

Read [007-e2e-testing-with-ginkgo.md](../adrs/007-e2e-testing-with-ginkgo.md). It defines the E2E test architecture.

## E2E Tests (`tests/e2e/`)

Read [`e2e/AGENTS.md`](e2e/AGENTS.md) for the full rules, directory layout, test patterns, and running instructions.

---

## Integration Tests (`tests/integration/`)

Read [`integration/AGENTS.md`](integration/AGENTS.md) for helper APIs, shared PostgreSQL bootstrap patterns, and performance guardrails.

### Purpose

Use one of these integration test slices:

- **Self-contained integration**: Use component tests within the process boundary (`httptest`, in-memory adapters, config wiring).
- **Infra-backed integration**: Use tests that need PostgreSQL or a LocalStack-compatible AWS emulator via testcontainers.

Keep the default integration loop fast. Retain infrastructure coverage.

### Directory Layout

```
tests/integration/
  bootstrap/
    aws_emulator.go                AWS emulator lifecycle
    aws_emulator_*_test.go         Emulator bootstrap and cleanup tests
    postgres.go                    Shared PostgreSQL and template database helpers
  infra/                           Infra-backed integration tests (build tag: integration)
    main_test.go                   Shared AWS emulator suite lifecycle
    *_test.go                      Keyring, signing-key, and migration checks
  storage/
    lifecycle_test.go              Memory adapter full lifecycle + concurrency
    infra/
      postgres_test.go             PostgreSQL adapter tests with testcontainers
      thirdparty_service_test.go   Third-party service tests
  migrations/
    doc.go                         Integration build tag marker
    framework.go                   PostgreSQL + go-migrate helper framework
    main_test.go                   Shared PostgreSQL container teardown
    migrations_test.go             Migration up/down verification with real database
  http_wait_test.go                Polling helper coverage for local server readiness
  *_test.go                        Self-contained configuration, endpoint, session, agent, and Helm checks
                                   (see files for exact coverage)
```

### Build Tags

- Run self-contained integration with `go test`. Do not use containers.
- Run infra-backed integration with `//go:build integration` and testcontainers. Use Docker or Podman.

```bash
go test -v ./tests/integration/...                               # Self-contained integration suites
go test -tags=integration -p 2 -v ./tests/integration/infra/... \
  ./tests/integration/migrations/... \
  ./tests/integration/storage/infra/... \
  ./internal/adapters/storage/postgres/...                       # Infra-backed integration suites
just test-integration                                            # Via justfile (self-contained default)
just test-integration-infra                                      # Via justfile
just test-integration-all                                        # Runs both layers
```

### AWS Emulator Bootstrap

Use `StartAWSEmulator()` or `StartAWSEmulatorForSuite()` from `bootstrap/aws_emulator.go`:

- Runs a LocalStack-compatible AWS emulator with KMS and DynamoDB services.
- Creates a KMS key and returns the key ID.
- Sets AWS SDK environment variables for test clients.
- Supports per-test startup and a shared suite-level container.
- `infra/encryption_vault_keyring_test.go` uses these helpers for real envelope encryption tests.

### Testing Conventions

- **Framework**: Use standard `go test` and `testify`. Use `require` for fatal assertions. Use `assert` for soft assertions.
- **Subtests**: Use `t.Run("description", ...)` to organize subtests.
- **Cleanup**: Use `defer` to end containers and restore environment variables.
- **Container runtime**: Locally, container-backed tests skip if Docker and Podman are unavailable. In CI, missing or failed infrastructure must fail the suite.
- **Ryuk disabled**: Set `TESTCONTAINERS_RYUK_DISABLED=true` for Podman or constrained Docker compatibility.
- **Shared PostgreSQL**: Use `bootstrap.RequireSharedPostgres()` and template database clones.
- **Shared DB subtests**: Reuse one clone in sequential `t.Run(...)` tests. Read `tests/integration/AGENTS.md`.
- **HTTP readiness**: Use polling helpers such as `waitForEndpoint`. Do not use fixed sleeps.
- **Failure paths**: Use short retries or TTLs and bounded handlers. Do not use long sleeps.

### Test Categories

| Category | Files | Infrastructure |
|---|---|---|
| Storage lifecycle | `storage/lifecycle_test.go` | None (in-memory) |
| PostgreSQL adapter | `storage/infra/postgres_test.go` | Testcontainers PostgreSQL |
| Migration verification | `migrations/migrations_test.go` | Testcontainers PostgreSQL |
| Encryption keyring | `infra/encryption_vault_keyring_test.go` | LocalStack-compatible AWS emulator (KMS + DynamoDB) |
| HTTP endpoints | `oauth2_*.go`, `server_test.go` | None (httptest) |
| Config/middleware | `config_test.go`, `principal_middleware_test.go` | None |

### Performance Guardrails

- Treat infra-backed tests as shared CI cost. Use one container per package and cloned databases.
- Add `TestMain` to new PostgreSQL test packages. End their shared containers. Migration and storage packages own their lifecycle. `infra/` owns the AWS emulator lifecycle.
- Keep package-level tests independent. Then `go test -tags=integration -p 2 ...` remains safe.
- Do not add unconditional screenshot or browser work to non-frontend suites.
