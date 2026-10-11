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
| Docker-tagged broker HTTP | Agentgateway policy scenarios in [this package](.) | `just test-e2e-docker` |
| ExtProc | Separate acceptance scenarios in [extproc/](extproc/) | `just test-e2e-extproc` |
| Frontend | Browser journeys through the [Playwright harness](frontend/frontend_suite_test.go) | `just test-e2e-frontend` |
| Integration | Self-contained component tests under [tests/integration/](../integration/AGENTS.md) | `just test-integration` |
| Infrastructure integration | Tagged PostgreSQL, migration, and encryption packages from [the recipe](../../justfile) | `just test-integration-infra` |

`just test-e2e` runs the backend, Docker-tagged backend, ExtProc, and frontend recipes.
`just test` excludes those suites and the integration suites.
See the [package selectors and recipes](../../justfile) for exact boundaries.

## Running E2E tests

Run commands from the repository root, as specified in the [suite instructions](AGENTS.md).
Use `just install-tools` for the repository's Ginkgo and Playwright installation commands ([recipe](../../justfile)).

The [justfile](../../justfile) provides these commands:

```bash
just test-e2e-backend
just test-e2e-docker
just test-e2e-backend-coverage
just test-e2e-backend-watch
just test-e2e-extproc
just test-e2e-frontend
just test-e2e
just test-e2e-coverage
```

Backend functional recipes exclude the `performance` and `docker` labels and build frontend assets first.
The Docker-tagged backend recipe uses a real agentgateway container and requires Docker.
`AGENTGATEWAY_IMAGE` overrides its pinned image.
The backend coverage recipe writes `coverage/e2e-backend.html`. It does not open a browser.
The [recipes](../../justfile) define both behaviors.

For a focused backend run, build the assets first.
Then select scenarios in the package, rather than passing an individual test file ([suite instructions](AGENTS.md#running-e2e-tests)):

```bash
just web-build
ginkgo -v --label-filter="!performance && !docker" --focus="Authorization" ./tests/e2e/
```

The separate `just test-e2e-performance` recipe selects the `performance` label with one worker ([recipe](../../justfile)).
Do not use that measurement as evidence that the functional suites passed.
Record the verbose `SC-001: <ok>/100 succeeded; p95=<duration> max=<duration> min=<duration>` result in PR #464.

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

### Credential-file bootstrap and recovery journeys

The [credential subprocess](bootstrap/credential_process.go) retains both listeners from configuration preparation through startup.
It passes the listeners through `exec.Cmd.ExtraFiles` to prevent another parallel test from acquiring those ports.
Server and subprocess cleanup close their listener copies.
The subprocess remains alive until `Close`, with a 10-second readiness deadline and a five-second graceful-shutdown deadline.

[US7-S2](oauth2_secret_file_deployment_test.go) from `specs/051-oauth2-secret-file-overrides/spec.md` checks both health endpoints for 36 seconds.
It then rotates credentials, refreshes the same session, and checks that cleanup releases both ports.

[US5-S3](token_exchange_test.go) from `specs/013-token-exchange/spec.md` follows the returned recovery URL for missing and rejected sessions.
It checks the callback return to `/sessions`, the existing session ID, and removal of the rejection marker.
It then checks resumed exchange and refresh with the replacement credential.

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

### Positive example: bootstrap, isolation, and semantic assertions

This example adapts the [metadata tests](oauth2_metadata_test.go) to show the full lifecycle and an explicit scenario reference.
Use the package's existing imports for `bootstrap`, `fixtures`, and `storageadapter`.
The standard-library imports are `encoding/json`, `log/slog`, and `net/http`, with Ginkgo and Gomega dot imports.

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
            Expect(resp.Header.Get("Content-Type")).To(ContainSubstring("application/json"))
            var metadata map[string]any
            Expect(json.NewDecoder(resp.Body).Decode(&metadata)).To(Succeed())
            Expect(metadata).To(HaveKeyWithValue("issuer", server.BaseURL()))
        })
    })
})
```

The `BeforeEach` runs again for each `It()`. Variables are available to each test, but mutable instances are not shared.
Configure the application before `Build()`. Do not change configuration afterward and assume that initialized services read the new values.
Keep related assertions together when they prove one scenario. A complete journey can require several HTTP requests and more than 15 lines.

### Positive and negative organization examples

The pairs retain the earlier guide's organization lessons without its obsolete helper calls or unchecked errors.
These small excerpts show structure, not additional acceptance scenarios.
For a real test, use its actual specification path, user story, and scenario identifier.

#### 1. Setup belongs in lifecycle hooks

Do not create storage and servers inside each `It()`:

```go
It("should return metadata", func() {
    logger := bootstrap.TestLogger(slog.LevelInfo)
    storageFactory := bootstrap.NewStorageFactory(logger)
    testStorage, err := storageFactory.NewTestStorage()
    Expect(err).NotTo(HaveOccurred())
    defer func() { Expect(storageFactory.CloseStorage(testStorage)).To(Succeed()) }()
})
```

This negative example only creates storage. It never exercises the promised metadata behavior.
Use the full lifecycle example for setup and cleanup. Keep the HTTP action and its assertions in `It()`.

#### 2. Group related scenarios instead of a flat list

Do not mix unrelated conditions in a generic container:

```text
Describe("Tests")
  It("metadata works")
  It("rejects an unknown client")
  It("preserves authorization state")
```

Use feature names and condition-based containers:

```text
Describe("OAuth2 metadata")
  Context("when a client requests metadata")
    It("should return a JSON metadata document")
Describe("OAuth2 authorization")
  Context("when the client is unknown")
    It("should reject the request")
```

#### 3. Declare variables outside hooks, initialize them inside hooks

Do not create a mutable fixture once during container construction:

```go
agent := fixtures.ValidAgent() // Shared mutable instance across tests.
```

Declare the variable at `Describe` scope. Create a new instance in `BeforeEach`:

```go
var agent *storage.Agent
BeforeEach(func() {
    agent = fixtures.ValidAgent()
    Expect(testStorage.Agents().Create(ctx, agent)).To(Succeed())
})
```

This excerpt assumes the lifecycle example's storage and a `context.Context` named `ctx`.
The `storage` import is `internal/domain/storage`, not the storage adapter.

#### 4. Separate unrelated scenarios, not the steps of one journey

Do not combine unrelated invalid requests in `It("should handle edge cases")`:

```text
It("should handle edge cases")
  Send a request without client_id; assert its error.
  Send a request without redirect_uri; assert its error.
```

Use one `It()` per actual acceptance scenario:

```text
Context("when client_id is missing")
  It("should reject a request without client_id")
Context("when redirect_uri is missing")
  It("should reject a request without redirect_uri")
```

Do not split a recovery journey into outcome-dependent tests.
The [US5-S3 recovery test](token_exchange_test.go) keeps recovery, resumed exchange, and subsequent refresh in one journey.

#### 5. Name the observable behavior

Do not use `It("works")` or `It("test")`.
Use `It("should return a JSON metadata document")` or the governing specification's behavior name.
Put conditions in `Context` names instead of long `It()` names.

#### 6. Use valid fixtures instead of handwritten entity IDs

Do not construct an agent with `ID: "test-agent-id"`. Entity IDs use domain types, and handwritten objects can omit required fields.
Use the [fixture factories](fixtures/README.md):

```go
agent := fixtures.ValidAgent()
principal := fixtures.DefaultPrincipal().String()
grant := fixtures.ActiveGrant(principal, agent.ID.String(), service.ID.String(), []string{"repo"})
```

This excerpt assumes a service fixture named `service`.
The [grant fixture source](fixtures/grants.go) defines the required permission-set and session seed helpers.
Seed those prerequisites before you assert behavior that resolves grant scopes or requires a usable service session.
Do not copy the former two-argument `ActiveGrant` example. The current helper takes four arguments.

#### 7. Use Context hooks for variants

Do not repeat agent creation in each test for absent, active, and expired grants.
Use a shared `BeforeEach` for the agent, then a separate `Context` for each grant state:

```text
Describe("OAuth2 authorization")
  BeforeEach: Create fresh storage, an agent, and the production server.
  AfterEach: Close the server and storage.
  Context("when no grant exists")
    It: Assert the specification's consent behavior.
  Context("when an active grant exists")
    BeforeEach: Create the grant and required service/session fixtures.
    It: Assert the specification's authorized behavior.
  Context("when an expired grant exists")
    BeforeEach: Create an expired grant.
    It: Assert the specification's expired-grant behavior.
```

Each context receives fresh base fixtures. No test depends on a grant from another `It()`.

#### 8. Keep nesting shallow

Do not create a separate `Context` for every word in a condition:

```text
Describe("Feature") > Context("when A") > Context("and B") > Context("and C") > Context("and D")
```

Combine related conditions instead:

```text
Describe("Feature") > Context("when A and B with C and D") > It("should return the specified result")
```

#### 9. Keep Given, When, and Then in order

Do not send a request before you create the fixtures that the request requires.
Use `BeforeEach` for Given, the request in `It()` for When, and concrete response assertions for Then.
For a longer journey, use `By()` to name each action and outcome.
Do not replace error checks or semantic assertions with `By()` descriptions.

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

### Page objects, selectors, and waits

Keep selectors inside [page objects](pages/), not in acceptance tests.
Use user-action names such as `DelegateService`, not selector-oriented names such as `ClickServiceButton(selector)`.
Return meaningful values such as scope names or connection status, not raw elements for tests to inspect.

Do not put CSS selectors or fixed sleeps in a scenario:

```go
time.Sleep(2 * time.Second)
Expect(page.Locator(".btn.primary.large").Click()).To(Succeed())
```

Use an existing page object instead:

```go
Expect(consentPage.DelegateService(ctx, "GitHub")).To(Succeed())
```

In page objects, prefer semantic selectors. These excerpts use the current `ConsentPage` API:

```go
button := cp.page().GetByRole("button", playwright.PageGetByRoleOptions{Name: "Approve & Delegate"})
endDate := cp.page().GetByLabel("End date", playwright.PageGetByLabelOptions{Exact: playwright.Bool(true)})
```

Use `data-testid` when the component has no suitable semantic selector.
The [scope helper](pages/consent_page.go) uses `[data-testid^='scope-indicator-']` for scope badges.
Do not prefer styling classes or XPath over roles, labels, and visible text.
If cards repeat a control label, scope the selector to the relevant card.
Use `.First()` only when the behavior explicitly concerns the first card.
Exercise shared page methods with multiple matching cards.

Use Playwright waits or `Eventually` for readiness, not fixed sleeps.
Keep waits inside page objects so scenarios describe user actions rather than timing details.

### Positive example: observable browser behavior and By steps

This excerpt assumes that the frontend harness supplies `ctx` and a prepared `consentPage` with `repo` and `user` scopes.
Use the actual scenario reference beside the containing `It()`.

```go
By("reading the visible service scopes")
scopes, err := consentPage.GetAvailableScopes(ctx)
Expect(err).NotTo(HaveOccurred(), "read the visible service scopes")
Expect(scopes).To(ContainElements("repo", "user"))

By("capturing the consent state")
Expect(consentPage.TakeScreenshot(ctx, "consent_service_scopes")).To(Succeed())
```

Do not assert React state or private component fields instead of visible output.
Do not replace a required scope assertion with `Expect(scopes).NotTo(BeEmpty())`.
That weaker assertion passes for the wrong nonempty scopes.
The page object owns extraction; the scenario owns the expected values.

Use one action or outcome per `By()` description.
Keep all steps of one acceptance journey in the same `It()`.
Simple single-action tests do not require `By()`.
Screenshot capture still requires the serial artifact command in this section.

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
