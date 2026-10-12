# Tests (`tests/`)

All paths in this file are relative to the repository root. Run commands from the repository root.

Read the changed tests and the nearest suite instructions before you write a test.

## Read First

| Source | Purpose |
|---|---|
| `.specify/memory/constitution.md` — Principles VIII and XIII | Test-first delivery, semantic failures, acceptance coverage |
| `ARCHITECTURE.md` — End-to-End Testing Architecture | Stable scenarios and production bootstrap |
| `adrs/007-e2e-testing-with-ginkgo.md` | Accepted E2E architecture |
| `tests/e2e/AGENTS.md` | Broker HTTP and ExtProc E2E rules |
| `tests/e2e/frontend/AGENTS.md` | Browser lifecycle, page objects, screenshots |
| `tests/integration/AGENTS.md` | Component tests, infrastructure helpers, commands |
| `justfile` | Suite boundaries, worker counts, report recipes |

Before you change behavior, read the governing feature's `spec.md`, `plan.md`, and `tasks.md` under `specs/`.
For HTTP assertions, read the relevant operation in `api/enduser/openapi.yaml` or `api/admin/openapi.yaml`.

## Choose the Suite

| Suite | Framework | Scope | Infrastructure | Command |
|---|---|---|---|---|
| `tests/e2e/` | Ginkgo v2 / Gomega | Broker HTTP acceptance scenarios | Production bootstrap, normally in-memory storage | `just test-e2e-backend` |
| `tests/e2e/extproc/` | Ginkgo v2 / Gomega | ExtProc acceptance scenarios | Some agentgateway scenarios require Docker | `just test-e2e-extproc` |
| `tests/e2e/frontend/` | Ginkgo v2 / Playwright Go | Browser acceptance scenarios | Built frontend and Chromium | `just test-e2e-frontend` |
| `tests/integration/` | Standard `go test` / testify assertions | Component behavior and configuration | Self-contained by default | `just test-integration` |
| Tagged integration packages | Standard `go test` / testify assertions | PostgreSQL, migrations, encryption | PostgreSQL and AWS emulator containers | `just test-integration-infra` |

`just test` excludes E2E and integration suites. `just test-e2e` runs the three functional E2E suites.
`just test-integration-all` runs both integration layers.

Keep self-contained tests free of container dependencies.
Retain real infrastructure coverage for behavior that depends on PostgreSQL, KMS, or DynamoDB.
Do not add browser or screenshot work to non-frontend suites.
