# End-to-End Test Suite

## Scope and sources

The [broker suite](e2e_suite_test.go) uses Ginkgo and Gomega.
Its [server factory](bootstrap/server_factory.go) builds the production application through `app.NewBuilder()`.
The [suite lifecycle](e2e_suite_test.go) supplies a mock OAuth2 upstream, so this is not a test against a live identity provider.

The governing sources are [Constitution Principle XIII](../../.specify/memory/constitution.md#xiii-end-to-end-acceptance-testing--spec-traceability)
and [007-e2e-testing-with-ginkgo.md — Accepted](../../adrs/007-e2e-testing-with-ginkgo.md).
The [suite instructions](AGENTS.md) describe acceptance rules and current helper usage.

## Suite boundaries

The [justfile](../../justfile) defines these separate entry points:

| Suite | Scope and source | Command |
|---|---|---|
| Broker HTTP | Production application and HTTP scenarios in [this package](.) | `just test-e2e-backend` |
| ExtProc | Separate acceptance scenarios in [extproc/](extproc/) | `just test-e2e-extproc` |
| Frontend | Browser journeys through the [Playwright harness](frontend/frontend_suite_test.go) | `just test-e2e-frontend` |
| Integration | Self-contained component tests under [tests/integration/](../integration/AGENTS.md) | `just test-integration` |
| Infrastructure integration | Tagged PostgreSQL, migration, and encryption packages from [the recipe](../../justfile) | `just test-integration-infra` |

`just test-e2e` runs all three functional E2E suites.
`just test` excludes those suites and the integration suites.
See the [package selectors and recipes](../../justfile) for exact boundaries.

## Running E2E tests

Run commands from the repository root, as specified in the [suite instructions](AGENTS.md).
Use `just install-tools` for the repository's Ginkgo and Playwright installation commands ([recipe](../../justfile)).

The [justfile](../../justfile) provides these commands:

```bash
just test-e2e-backend
just test-e2e-backend-coverage
just test-e2e-backend-watch
just test-e2e-extproc
just test-e2e-frontend
just test-e2e
just test-e2e-coverage
```

Backend functional recipes exclude the `performance` label and build frontend assets first.
The backend coverage recipe writes `coverage/e2e-backend.html`. It does not open a browser.
The [recipes](../../justfile) define both behaviors.

For a focused backend run, build the assets first.
Then select scenarios in the package, rather than passing an individual test file ([suite instructions](AGENTS.md#running-e2e-tests)):

```bash
just web-build
ginkgo -v --label-filter="!performance" --focus="Authorization" ./tests/e2e/
```

The separate `just test-e2e-performance` recipe selects the `performance` label with one worker ([recipe](../../justfile)).
Do not use that measurement as evidence that the functional suites passed.

The [worker configuration](../../justfile) defaults backend and ExtProc runs to four workers and frontend runs to two.
`GINKGO_BACKEND_PROCS`, `GINKGO_EXTPROC_PROCS`, and `GINKGO_FRONTEND_PROCS` override those values.

## Test architecture

The [accepted E2E decision](../../adrs/007-e2e-testing-with-ginkgo.md) separates stable scenarios from production-bootstrap details.
Use these source locations instead of copied helper signatures:

| Layer | Source and purpose |
|---|---|
| Suite lifecycle | [e2e_suite_test.go](e2e_suite_test.go): shared upstream setup and shutdown |
| Application bootstrap | [server_factory.go](bootstrap/server_factory.go): production application builder |
| HTTP servers | [test_server.go](bootstrap/test_server.go): end-user and admin servers, requests, and URL-aware builder |
| Storage | [storage.go](bootstrap/storage.go): storage factory and lifecycle |
| Fixtures | [fixtures/](fixtures/): test data factories |
| Helpers | [helpers/](helpers/): HTTP, JWT, upstream, JWKS, and signing-key utilities |
| Matchers | [matchers/](matchers/): OAuth2, claim, and telemetry assertions |
| Browser interactions | [pages/](pages/): page objects |

Use separate end-user and admin servers for scenarios that need both route types ([server guidance](AGENTS.md#dual-server-pattern)).
Use `NewTestServerBuilder` when assertions depend on the listener URL ([implementation](bootstrap/test_server.go)).

## Adding acceptance scenarios

Apply the [acceptance rules](AGENTS.md#acceptance-rules) and [Constitution Principle XIII](../../.specify/memory/constitution.md#xiii-end-to-end-acceptance-testing--spec-traceability):

1. Read the feature's `spec.md`, `plan.md`, and `tasks.md` under [specs/](../../specs/).
2. Map each acceptance scenario to one `It()`.
3. Put the specification path, user story, and scenario identifier beside that `It()`.
4. Group scenarios with `Describe` and condition-based `Context` blocks.
5. Create fresh mutable fixtures, storage, and servers in `BeforeEach`.
6. Close resources in `AfterEach`, and close HTTP response bodies after each request.
7. Assert observable responses or UI behavior, not routing or dependency-injection internals.

Use the [metadata pattern](AGENTS.md#test-structure-pattern) for current bootstrap calls.
Keep production wiring in [bootstrap/](bootstrap/), not in scenario bodies.
Do not share mutable scenario state or depend on test order ([acceptance rules](AGENTS.md#acceptance-rules)).

## Frontend E2E Testing

The [frontend harness](frontend/frontend_suite_test.go) owns browser startup and per-test resources.
The [frontend instructions](frontend/AGENTS.md) describe page objects, selectors, screenshots, and browser cleanup.

For built assets, use `just test-e2e-frontend` ([recipe](../../justfile)).
For development mode, start Vite in another terminal, then use one worker ([development-mode guidance](frontend/AGENTS.md#running-frontend-e2e-tests)):

```bash
just web-dev
```

```bash
GINKGO_FRONTEND_PROCS=1 just test-e2e-frontend-dev
```

Development mode uses fixed broker port 8000.
The one-worker override is necessary because the recipe still defaults to two workers ([frontend guidance](frontend/AGENTS.md), [recipe](../../justfile)).

The normal recipes disable screenshot capture by default ([configuration](../../justfile)).
For artifact capture, use the serial configuration from the [frontend instructions](frontend/AGENTS.md#screenshots-and-performance):

```bash
E2E_CAPTURE_SCREENSHOTS=true GINKGO_FRONTEND_PROCS=1 just test-e2e-frontend
```

The [screenshot workflow](../../.github/workflows/screenshots.yml) owns publication of tracked screenshot artifacts.

## Reports and troubleshooting

The [CI workflow](../../.github/workflows/ci.yml) uses separate backend, ExtProc, and frontend JUnit recipes from the [justfile](../../justfile).
The [scheduled coverage workflow](../../.github/workflows/scheduled-coverage.yml) runs the coverage recipes.
Use the results from the relevant run for test totals, skips, failures, and duration. This guide does not declare a current pass count.

For diagnosis, use these sources:

| Problem | Source to inspect |
|---|---|
| Missing frontend assets | [Backend recipe dependencies](../../justfile) and [frontend harness](frontend/frontend_suite_test.go) |
| Wrong server or public URL | [Server constructors and builder](bootstrap/test_server.go) |
| Browser state, identity, or port conflicts | [Frontend lifecycle and development guidance](frontend/AGENTS.md) |
| Container availability or storage cleanup | [Storage integration guide](../integration/storage/README.md) |
| Missing production coverage | [Coverage recipes and their `--coverpkg` arguments](../../justfile) |

Test totals and timings depend on the selected suite and run. The [recipes](../../justfile) select different packages, labels, and worker counts.
