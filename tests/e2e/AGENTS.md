# E2E Tests (`tests/e2e/`)

All paths in this file are relative to the repository root. Directory trees start at the named root.
Run commands from the repository root.

Read `tests/AGENTS.md` for the governing test rules.
Read these sources before you change E2E tests:

| Source | Purpose |
|---|---|
| `.specify/memory/constitution.md` — Principle XIII | Acceptance coverage, isolation, semantic assertions |
| `ARCHITECTURE.md` — End-to-End Testing Architecture | Stable HTTP scenarios and production bootstrap |
| `adrs/007-e2e-testing-with-ginkgo.md` | Accepted E2E architecture |
| `tests/e2e/README.md` | Structure, commands, and anti-patterns |
| `tests/e2e/fixtures/README.md` | Fixture rules and categories |
| `.github/instructions/e2e-tests-code-review.instructions.md` | Test review checklist |
| `api/enduser/openapi.yaml`, `api/admin/openapi.yaml` | HTTP operation contracts |

The guides contain historical inventories and examples. Read current source for helper signatures and feature coverage.
Read the governing feature's `spec.md`, `plan.md`, and `tasks.md` under `specs/` before you derive assertions.

## Acceptance Rules

1. Map each acceptance scenario to one `It()`. Keep unrelated behaviors in separate tests.
2. Put the specification path, user story, and actual scenario identifier next to each `It()`.
   For example: `// Scenario 1 from specs/024-approval-api-ui/spec.md (User Story 1)`.
   Do not rely on an ambiguous shared `Describe` comment.
3. Use concrete assertions against HTTP responses, rendered UI, or other observable production behavior.
4. Use production constructors and DI through the suite bootstrap. Do not duplicate application wiring in scenarios.
5. Keep setup in `BeforeEach` and cleanup in `AfterEach`. Use `Context` for variants with shared setup.
6. Create fresh mutable fixtures, storage, and servers for each test.
7. Close response bodies after each request.

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
    sessions_page.go               Third-party session interactions
    tool_authorizations_page.go    Tool-authorization interactions
  extproc/                        Separate ExtProc Ginkgo suite
  frontend/                       Separate browser Ginkgo suite
  screenshots/                    Maintained screenshot artifacts
```

## Architecture: Stable vs Volatile Layers

### Performance-sensitive helpers

- Use `bootstrap.TestLogger(level)` for routine logs. It discards output unless `E2E_VERBOSE=1`.
- Keep the shared upstream in `tests/e2e/e2e_suite_test.go` read-only across workers.
- Use `Eventually` or polling helpers. Do not use fixed sleeps in new scenarios.

**Broker HTTP suite**: Assert HTTP contracts through `bootstrap.TestServer`, not routing or DI internals.
On broker wiring changes, update the thin wrappers in `tests/e2e/bootstrap/`.

**ExtProc suite**: Reuse `tests/e2e/extproc/` bootstrap, fixtures, and helpers.
Read `internal/extproc/AGENTS.md` for standalone architecture and contracts.
Some agentgateway scenarios require Docker and use `Ordered` for container lifecycle.
Do not introduce dependencies between scenario outcomes.
For approval sync, read `specs/026-extproc-approval-sync/plan.md` — Testing Strategy.
It assigns broker response assertions to broker tests and cache behavior to ExtProc tests.

**Frontend suite**: Read `tests/e2e/frontend/AGENTS.md` for browser lifecycle and screenshots.

## Test Structure Pattern

Use the metadata test pattern with current bootstrap APIs:

```go
var _ = Describe("OAuth2 metadata", func() {
    var (
        server *bootstrap.TestServer
        testStorage *storageadapter.Adapter
        storageFactory *bootstrap.StorageFactory
    )

    BeforeEach(func() {
        logger := bootstrap.TestLogger(slog.LevelInfo)
        storageFactory = bootstrap.NewStorageFactory(logger)
        var err error
        testStorage, err = storageFactory.NewTestStorage()
        Expect(err).NotTo(HaveOccurred())
        config := fixtures.DefaultOAuth2Config()
        factory := bootstrap.NewServerFactory(config, logger)
        builder, err := bootstrap.NewTestServerBuilder(config, testStorage, factory, logger)
        Expect(err).NotTo(HaveOccurred())
        server, err = builder.Build()
        Expect(err).NotTo(HaveOccurred())
    })

    AfterEach(func() {
        if server != nil {
            server.Close()
        }
        if testStorage != nil {
            Expect(storageFactory.CloseStorage(testStorage)).To(Succeed())
        }
    })

    Context("when a client requests metadata", func() {
        // Scenario 1 from specs/009-oauth2-auth-server/spec.md (User Story 3)
        It("should return a JSON metadata document", func() {
            resp, err := server.PublicGET("/.well-known/oauth-authorization-server")
            Expect(err).NotTo(HaveOccurred())
            defer resp.Body.Close()
            Expect(resp.StatusCode).To(Equal(http.StatusOK))
            var metadata map[string]any
            Expect(json.NewDecoder(resp.Body).Decode(&metadata)).To(Succeed())
            Expect(metadata).To(HaveKey("issuer"))
        })
    })
})
```

Use descriptive `Describe` names, condition-based `Context` names, and `It("should …")` behavior names.
For longer scenarios, use `By` to separate actions and assertions.

## Anti-Patterns (Flag for Review)

- Missing or ambiguous specification references
- Setup inside `It()` instead of `BeforeEach`
- Repeated setup instead of a shared `BeforeEach` within `Describe` or `Context`
- Direct service wiring instead of suite bootstrap
- Mutable state or outcome dependencies across `It()` blocks
- Missing cleanup or unchecked request errors
- Multiple unrelated behaviors in one test.

Treat an `It()` longer than 15 lines as a review signal, not a limit on necessary assertions.

## Dual Server Pattern

Use separate servers that match the production architecture:

- `bootstrap.NewEndUserTestServer()` — OAuth2, consent UI, and end-user APIs (default production port 8000)
- `bootstrap.NewAdminTestServer()` — Agent and service APIs (default production port 14000).

Create both server instances for tests that need both route types.
If assertions depend on the actual public URL, use `bootstrap.NewTestServerBuilder`.
It sets the listener URL before production services initialize.

## Fixtures Rules

- Return production domain types. Do not return test-specific objects.
- Use deterministic principals and configuration.
- Generate new entity IDs for isolation.
- Pass domain validation.
- Do not use external files or network dependencies.

## Running E2E Tests

```bash
just test-e2e-backend          # Backend functional suite
just test-e2e-backend-coverage # Backend coverage report
just test-e2e-backend-watch    # Backend watch mode
just test-e2e-extproc          # ExtProc suite
just test-e2e-performance      # Dedicated SC-001 measurement
just test-e2e                  # Backend, ExtProc, and frontend suites
ginkgo -v --label-filter="!performance" --focus="pattern" ./tests/e2e/
```

Backend recipes build the frontend first. Before a direct Ginkgo run that needs frontend assets, run `just web-build`.
`just test-e2e-performance` runs the warmed, local-mode, signed-subject 100-request measurement from `tests/e2e/impersonation_test.go`.
It uses one worker and reports successes plus p95, maximum, and minimum duration.
Functional recipes exclude this machine-sensitive measurement.

### Parallelism expectations

- Keep backend specs isolated for parallel execution.
- Keep shared ExtProc containers within their existing lifecycle groups.
- Keep frontend browser state isolated per test.

`GINKGO_BACKEND_PROCS` and `GINKGO_EXTPROC_PROCS` override `GINKGO_PROCS`, which defaults to `4`.
`GINKGO_FRONTEND_PROCS` defaults to `2`.
