# E2E Tests (`tests/e2e/`)

Read changed tests and adjacent READMEs before you write a test. Use `tests/e2e/README.md` for broker HTTP E2E patterns. It is historical. Use the current test tree for feature coverage.

Read [007-e2e-testing-with-ginkgo.md](../../adrs/007-e2e-testing-with-ginkgo.md). This ADR binds the E2E test architecture.

## Constitution Principle XIII — Mandatory Rules

1. **Spec-to-test mapping**: Map each `It()` to one acceptance scenario.
   Read it from `specs/<feature>/spec.md`. A shared `Describe` comment can map one scenario when it clearly covers that block.
2. **Spec traceability**: Add a nearby comment with the actual scenario identifier.
   For example: `// US1-S1 from specs/024-approval-api-ui/spec.md`.
   You can also use `// Scenario 1 from specs/036-canonical-resource-ids/spec.md`.
3. **Production bootstrap**: Use `app.Builder` and production DI. Do not use custom test implementations.
4. **Observe production behavior**: Use production constructors and externally visible behavior for startup and readiness assertions.

## Directory Layout

```
tests/e2e/
  e2e_suite_test.go               Ginkgo suite entrypoint
  bootstrap/                      Volatile production-bootstrap wrappers
    test_server.go                TestServer wrapper — AuthenticatedGET/POST, PublicGET
    server_factory.go             Builds production app.App via DI
    storage.go                    In-memory storage initialization
    logger.go, telemetry.go       Quiet logging and telemetry helpers for parallel runs
    cimd.go                       CIMD-specific bootstrap helpers
  fixtures/                       Deterministic test data factories
    *.go                          Agents, grants, principals, OAuth2 config, services, sessions,
                                  encryption, and multi-agent fixtures
  helpers/                        Shared HTTP/JWT/upstream/JWKS/PKCE/signing-key utilities
  matchers/                       Custom Gomega matchers for OAuth2, telemetry, and claims
  pages/                          Page objects for frontend E2E tests
    page.go                        Base navigation and screenshot methods
    approval_page.go               Tool-approval review interactions
    consent_page.go                Consent interactions
    tool_authorizations_page.go    Tool-authorization interactions
  extproc/                        Separate ExtProc Ginkgo suite; read `internal/extproc/AGENTS.md`
  frontend/                       Frontend E2E tests; read frontend/AGENTS.md
  screenshots/                    Maintained screenshot artifacts
```

## Architecture: Stable vs Volatile Layers

### Performance-sensitive helpers

- `bootstrap.TestLogger()` discards logs unless `E2E_VERBOSE=1`. To limit I/O during parallel runs, use it.
- `e2e_suite_test.go` shares the read-only mock upstream across backend workers.
- Use `Eventually` or polling helpers. Do not use `time.Sleep(...)` in new specs.

**Broker HTTP suite**: Use HTTP contracts and `server.AuthenticatedGET()` or `server.PublicGET()`.
Do not depend on routing or DI.

**ExtProc suite**: Use `tests/e2e/extproc/` bootstrap, fixtures, and helpers. Some agentgateway scenarios require Docker and use `Ordered` to share a container.

**Volatile layer** (`bootstrap/`): On broker wiring changes, update the thin production-bootstrap wrappers.

**Result**: Routing or DI changes affect `bootstrap/`, not the broker HTTP scenarios.

## Test Structure Pattern

```go
var _ = Describe("Feature Name", func() {
    var (
        server      *bootstrap.TestServer
        testStorage *storageadapter.Adapter
    )

    BeforeEach(func() {
        // Fresh storage and server per test — no shared state
        testStorage, _ = createTestStorage()
        server, _ = createTestServer()
    })

    AfterEach(func() {
        server.Close()
    })

    Describe("when condition", func() {
        BeforeEach(func() {
            // Scenario-level setup
            agent := fixtures.ValidAgent()
            testStorage.Agents().Create(ctx, agent)
        })

        // Scenario identifier from specs/<feature>/spec.md
        It("should verify expected behavior", func() {
            resp, _ := server.AuthenticatedGET("/endpoint", principal)
            Expect(resp.StatusCode).To(Equal(http.StatusOK))
        })
    })
})
```

## Anti-Patterns (Flag for Review)

- **Missing spec reference**: An `It()` has no nearby scenario reference or a clearly applicable shared `Describe` reference.
- **Setup in It()**: More than 15 setup lines occur in an `It()` block. Use `BeforeEach` instead.
- **Duplicate setup**: Put repeated setup in a `Context` block.
- **Direct DI**: Tests create services instead of using `bootstrap/` wrappers.
- **Shared state**: Mutable state crosses `It()` blocks without a `BeforeEach` reset.

## Dual Server Pattern

Use separate servers that match the production architecture:

- `NewEndUserTestServer()` — OAuth2, consent UI, and public APIs (port 8000 in production)
- `NewAdminTestServer()` — Agent and service APIs (port 14000 in production)

Create both server instances for tests that need both route types.

## Fixtures Rules

- Return production domain types. Do not return test-specific objects.
- Use deterministic principals and configuration.
- Generate new entity IDs for isolation.
- Pass domain validation.
- Do not use external files or network dependencies.

## Running E2E Tests

```bash
just test-e2e-backend          # Backend E2E suite only
just test-e2e-backend-coverage # Backend E2E suite with coverage report
just test-e2e-backend-watch    # Backend E2E watch mode for TDD
just test-e2e                  # Run backend, ExtProc, and frontend suites
ginkgo -v --label-filter="!performance" --focus="pattern" ./tests/e2e/    # Focused backend run
```

Use `just test-e2e-extproc` for the ExtProc suite. Some agentgateway scenarios require Docker.
`test-e2e-performance` runs the warmed local-mode, signed-subject 100-request SC-001 measurement with one Ginkgo worker. Its verbose `SC-001: <ok>/100 succeeded; p95=<duration> max=<duration> min=<duration>` line is recorded in PR #464.

### Parallelism expectations

- Backend E2E runs with `GINKGO_PROCS`. To make parallel execution safe, keep specs isolated.
- ExtProc E2E runs in parallel. Some agentgateway scenarios share a Docker container and use `Ordered`.
- Frontend E2E uses a separate suite. Read `frontend/AGENTS.md` for its screenshot rules.
