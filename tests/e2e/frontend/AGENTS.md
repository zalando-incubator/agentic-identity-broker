# Frontend E2E Tests (`tests/e2e/frontend/`)

All paths in this file are relative to the repository root. Directory trees start at the named root.
Run commands from the repository root.

Read `tests/e2e/AGENTS.md` for acceptance rules and canonical sources.
Read the changed tests, `tests/e2e/frontend/frontend_suite_test.go`, and the relevant page objects before you add browser tests.

Browser tests use Playwright through `github.com/mxschmitt/playwright-go`.
The harness uses production broker bootstrap and a fresh browser context for each test.

## Local Sources

| Source | Purpose |
|---|---|
| `tests/e2e/frontend/frontend_suite_test.go` | Bundle, browser, per-test lifecycle, coverage collection |
| `tests/e2e/pages/` | Selectors, interactions, waits, screenshot helpers |
| `tests/e2e/README.md` — Frontend E2E Testing | Page object and test structure patterns |
| `web/vite.config.ts` | Development proxy and identity header |
| `.github/workflows/screenshots.yml` | Serial capture and tracked artifact publication |
| `justfile` | Frontend recipes and worker controls |

## Directory Layout

```
tests/e2e/frontend/
  frontend_suite_test.go          Ginkgo suite and Playwright setup
  *_test.go                       Consent, CIMD, approval, permission-set, session, CSRF, and selection journeys
```

## Running Frontend E2E Tests

```bash
just test-e2e-frontend                          # Built frontend
just web-dev                                    # Vite, in another terminal
GINKGO_FRONTEND_PROCS=1 just test-e2e-frontend-dev # Serial development mode
just test-e2e-frontend-coverage                 # Built frontend with Go coverage
```

`E2E_FRONTEND_MODE` defaults to `built`. `HEADLESS` defaults to `true`.
Built mode adds `X-Remote-User` from `fixtures.DefaultPrincipal()` to browser contexts.
Development mode uses Vite at `http://localhost:3000` and a fixed broker port of `8000`.

For development mode, use one worker and keep port 8000 free before the suite starts.
Read `web/vite.config.ts` before you debug proxy identity failures.
Its proxy injects `dev@example.com`, while the default fixture principal is `user@example.com`.

## Playwright Lifecycle

- Before the tests, build the frontend with the recipe. CI requires an existing `web/dist/index.html`.
- For direct local runs, let process 1 reuse an existing bundle. If absent, the local harness builds it.
- Keep browser startup and shutdown in the suite lifecycle. Each worker owns one browser and Playwright driver.
- Keep per-test cleanup in the harness. It closes the browser context, server, storage, and upstream.
- In built mode, keep specs isolated for `GINKGO_FRONTEND_PROCS` workers.
- Use page object methods in tests. Keep selectors in `tests/e2e/pages/`.
- Use `bootstrap.TestLogger(level)` and suite helpers for routine logs.

## Page Objects

| File | Type | Responsibilities |
|---|---|---|
| `tests/e2e/pages/page.go` | `Page` | Navigation, waits, screenshots |
| `tests/e2e/pages/consent_page.go` | `ConsentPage` | Consent interactions |
| `tests/e2e/pages/approval_page.go` | `ApprovalPage` | Tool-approval review interactions |
| `tests/e2e/pages/sessions_page.go` | `SessionsPage` | Third-party session interactions |
| `tests/e2e/pages/tool_authorizations_page.go` | `ToolAuthorizationsPage` | Tool authorization list interactions |

## Writing Frontend Tests

- Add page objects to `tests/e2e/pages/` for new UI screens.
- Create fresh browser state in `BeforeEach`. Do not share cookies or page state.
- Use Playwright waits or Gomega `Eventually`. Do not use fixed sleeps.
- If controls have repeated labels, scope selectors to the relevant card.
- Exercise shared page methods with multiple matching cards.

## Screenshots and Performance

Principle XIII requires screenshots for each meaningful UI state.
The normal functional recipes leave capture disabled to limit browser work.

- Add screenshot calls with stable, descriptive, unique names for the required UI states.
- For artifact capture, set `E2E_CAPTURE_SCREENSHOTS=true` and use one worker.
- Use `Page.TakeScreenshot()` or `Page.TakeLocatorScreenshot()` for a dialog or overlay.
- Keep screenshot publication in `.github/workflows/screenshots.yml`.

The helpers capture only if `E2E_CAPTURE_SCREENSHOTS` is `true` (case-insensitive) or `1`.
The frontend suite writes images to `tests/e2e/frontend/coverage/screenshots/`.
The screenshot workflow copies successful captures to `tests/e2e/screenshots/`.
It runs only for same-repository pull requests.
