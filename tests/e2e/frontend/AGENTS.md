# Frontend E2E Tests (`tests/e2e/frontend/`)

Read existing test files and page objects before you write new tests.

Browser-based tests use Playwright via `playwright-go`.

## Rules (inherit from `../AGENTS.md`)

Apply all E2E rules. Map each `It()` to one specification scenario. Use production bootstrap. Read [../AGENTS.md](../AGENTS.md).

Use the exact scenario identifier from the feature specification. A nearby `It()` comment or a clearly applicable shared `Describe` comment is valid.

## Directory Layout

```
tests/e2e/frontend/
  frontend_suite_test.go          Ginkgo suite and Playwright setup
  *_test.go                        Consent, CIMD, approval, permission-set, CSRF, and selection journeys
```

## Running Frontend E2E Tests

```bash
just test-e2e-frontend             # Build mode
just web-dev                       # Start Vite before development mode
just test-e2e-frontend-dev         # Development mode
just test-e2e-frontend-coverage    # Build mode with coverage
```

`E2E_FRONTEND_MODE` is `built` by default. Use `dev` only with Vite running.
`HEADLESS` is `true` by default. Built mode adds `X-Remote-User` to browser contexts.
The Vite proxy adds it in development mode.

## Playwright Lifecycle

- Process 1 builds the frontend once in `frontend_suite_test.go`.
- Each worker starts and stops its own Playwright browser.
- The suite closes each page with its browser context.
- `GINKGO_FRONTEND_PROCS` runs multiple workers. Each spec needs its own server, browser context, and storage.
- Page objects in `../pages/` hide selectors. Use interaction methods in test files. Do not use raw selectors.
- Use `bootstrap.TestLogger()` and suite helpers for routine logs.

## Page Objects (`../pages/`)

| File | Type | Responsibilities |
|---|---|---|
| `page.go` | `Page` | Navigation, waits, screenshots |
| `consent_page.go` | `ConsentPage` | Consent interactions |
| `approval_page.go` | `ApprovalPage` | Tool-approval review interactions |
| `tool_authorizations_page.go` | `ToolAuthorizationsPage` | Tool authorization list interactions |

## Writing Frontend Tests

- Use page objects. Do not interact with raw selectors in test files.
- Add page objects to `../pages/` for each new UI screen.
- `BeforeEach` isolates the browser context. Do not share browser state.
- Prefer Playwright waits or Gomega `Eventually`. Do not use `time.Sleep(...)`.

## Screenshots and Performance

- Unless `E2E_CAPTURE_SCREENSHOTS=true`, `Page.TakeScreenshot()` does nothing.
- The suite writes captured images to `coverage/screenshots/` relative to its working directory. The screenshot workflow copies maintained images to `tests/e2e/screenshots/`.
- Normal verification does not capture screenshots.
- `.github/workflows/screenshots.yml` captures tracked PNG files in serial mode.
- Capture screenshots only for a maintained artifact or review aid.
- Screenshot tests use the workflow's serial mode and stable, unique file names.
