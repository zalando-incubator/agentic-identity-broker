# Quickstart — Consent UI v2 Validation

## Status

This is a validation guide for the planned implementation. The planning change does not implement components, themes, API handlers, or new command recipes.

Existing commands are identified below. Proposed commands become available in the implementation phase that owns them. Do not interpret this guide as a passing runtime report.

## Prerequisites

Use Node 24 or later, the repository Go toolchain, npm, just, Ginkgo, and the existing Playwright browser setup. Use Docker or Podman for PostgreSQL integration tests.

Before implementation, obtain acceptance for ADR 037 and approval for the detailed additive API contracts. Merge approved schemas into `api/enduser/openapi.yaml` before backend changes.

Run commands from the repository root unless specified otherwise.

## Install and inspect commands

These commands exist today:

```bash
just --list
just web-ci
```

`web-ci` uses the lockfile and strict engine checking. It does not install the planned component dependencies until implementation updates the manifest.

## Storybook: light and dark

This command exists today:

```bash
npm --prefix web run storybook
```

Open `http://localhost:6006`. After phase 1, select **Light** and **Dark** in the themes toolbar. The decorator must set `data-theme` on the preview document, including portal content.

1. Open each primitive and shell story.
2. Switch between Light and Dark.
3. Open dialogs, menus, popovers, tooltips, Sheet, Command, and notifications.
4. Use the keyboard to reach and close each interactive element.
5. Inspect the Accessibility panel in both themes.

Expected result: readable controls, visible focus, theme-matched overlays, no automatic third-party requests, and no a11y violations.

The toolbar alone does not prove CI coverage. Phase 1 adds these recipes:

```bash
just web-storybook-build
just web-storybook-test
```

`web-storybook-build` wraps the existing `npm --prefix web run build-storybook` command. `web-storybook-test` runs the complete story set in two Vitest browser projects, one light and one dark. CI installs Chromium and uses the same Storybook 10 addon versions as the app.

Expected result: both projects run, and an intentional inaccessible fixture fails the gate during gate development. Production stories must use `parameters.a11y.test = 'error'`. Do not suppress failures with `todo`.

## Existing unit and end-to-end commands

```bash
just web-test
just web-test-coverage
just test-e2e-frontend
```

Built-mode frontend E2E starts isolated broker instances through the production bootstrap. It supplies the principal through the existing test authentication setup. This is the preferred complete-flow validation path.

For interactive Vite work, run `just web-dev` in another terminal. Then run:

```bash
just test-e2e-frontend-dev
```

Do not start a separate example broker on Vite's port 3000. Built-mode E2E avoids that conflict.

After Go, configuration, or migration implementation changes, run:

```bash
just check
just test
just test-integration
just test-integration-infra
```

The last command requires a container runtime. Use the repository's existing integration setup and real migrations. The final merge gate is `just verify`.

## Phase 1: foundation smoke

Use Storybook to inspect both shells at desktop, 320 px, and 200% browser zoom. No page-level horizontal scrolling is permitted. Focus must remain visible when the mobile sidebar or a dialog opens.

Inspect font and image requests in the browser. All automatic resource requests must stay on the same origin. Outlined SVGs must contain no remote font imports. The black wordmark belongs to light mode and the white wordmark to dark mode.

Set theme to Dark, reload, and inspect the first frame. It must not flash light. Repeat with explicit Light on a dark OS, then System while changing the OS theme. Verify native controls and scrollbars.

Inspect the production response CSP. Fonts must use `font-src 'self'`. The inline theme script must run with its exact hash, without script `unsafe-inline`.

During phases 2–3, use the broker configuration example with:

```yaml
ui:
  v2: true
```

The planned environment override is `IDENTITY_BROKER_UI_V2=true`. The planned CLI override is `--ui-v2=true`. Validate CLI over environment over file over default. Verify both GET and HEAD HTML responses, and the Vite server-side bootstrap path.

Expected result: only presentation changes. No browser storage or query parameter can select the flag. False keeps the old presentation until the phase-3 cutover. Remove these configuration values after that cutover.

## Phase 2: decision journeys

Run AS-01–AS-05 through the Ginkgo/Playwright suite with both flag values during migration.

Start consent through the existing authorization flow, not a fabricated `session_token`. Select optional access and a duration. Connect a required provider, return, and verify that selections remain intact. Allow and observe the existing validated continuation.

Repeat with an existing grant. Only new selections require a decision. Existing selections remain granted. Deny must leave the grant unchanged and show a local outcome without a constructed redirect.

Open a pending approval. Inspect the exact scope and expanded arguments. Exercise once, session, permanent, and denial in isolated runs. Resolve a request in another context and verify that stale actions disappear.

Expected result: one accent primary action, no sidebar, truthful identity/risk, and no change to existing authorization semantics.

## Phase 3: console journeys and cutover

Run AS-06–AS-15. Search/filter agents, edit and cancel a draft, save a deliberate edit, and revoke with confirmation. Inject a failed revoke and verify that the pending row returns without overwriting unrelated row changes.

Use connection fixtures for no session, missing required scopes, expired refresh, usable access, and refresh rejection. Optional provider scopes must not create a false Missing scopes result. Disconnect copy must state the provider-token limitation.

Create a pending approval while the console remains open. The queue and count must update within 15 seconds on the foreground test browser. Inspect network requests: only `/api/approvals/pending` supplies browser updates, never the gateway-wide endpoint.

After cutover, remove `ui.v2` from test configuration. Verify all original routes and both agent contexts without a flag. Search dependency/build output for old component packages, fonts, palettes, cache, and entry animations.

## Phase 4: activity and preferences

Run AS-16–AS-18 with two authenticated principals. Produce successful, denied, and failed attributable actions. Filter activity by agent and type. Verify that one user's cursor cannot reveal another user's records.

Use a deterministic clock to cover just inside and outside the 90-day boundary. Expired records must remain invisible before cleanup runs. Verify keyset traversal while new events arrive. Run cleanup and verify expired-record deletion in both adapters.

Inject activity append failure during a successful action and during an existing denial. Each original outcome must stay unchanged. Operational logs must report the storage failure without credentials. Read failure must produce an error, not a fake empty history.

The activity response must contain no principal, token, secret, raw tool arguments, or free-form internal errors. Local consent cancellation produces no server event because it sends no request.

Change theme and approval persistence in Settings. Reload and open a new approval. The choice can preselect a default but must never submit the decision. Use Command by keyboard to navigate only acting-user records.

See [the data model](data-model.md), [activity OpenAPI](contracts/enduser-additions.openapi.yaml), and [session contract](contracts/session-state.md) for field details.

## Visual snapshots

The current capture command is:

```bash
E2E_CAPTURE_SCREENSHOTS=true GINKGO_FRONTEND_PROCS=1 just test-e2e-frontend
```

Current captures go to `tests/e2e/frontend/coverage/screenshots/`. Reviewed baselines live in `tests/e2e/screenshots/`. Current capture is not a blocking comparison gate.

Phase 4 completes the planned gate recipe:

```bash
just test-e2e-frontend-visual
```

The recipe captures both themes serially and compares against reviewed baselines with the existing `tools/imgdiff`. It fails on unreviewed images or excess difference and publishes comparison artifacts. Baseline approval is a separate review action.

Required original-route filenames use these stems with `_light.png` and `_dark.png`:

- `delegations`
- `agent_consent`
- `agent_detail`
- `sessions`
- `approvals`
- `approval_review`.

Add `activity` and `settings` in phase 4. This produces at least 16 route/context/theme images. Include README references to every image. Keep capture data, clock, browser, OS, fonts, and viewport stable.

## Performance and manual acceptance

Build with `just web-build`. Measure the compressed initial consent dependency graph. It must remain below 150 kB without console Table or Command chunks.

Measure main-content appearance on throttled 4G. It must remain below 1.5 seconds. Confirm at least 12 agent rows at a 1080 px-high desktop viewport.

Run moderated consent sessions for SC-001 and SC-007. Record decision time, service comprehension, and primary-action recognition. Automated accessibility and timing checks cannot establish those human outcomes.

## Evidence to attach

Attach both-theme Storybook results, semantic red/green test evidence, E2E results, reviewed screenshots, network/CSP observations, bundle measurements, migration results, and moderated-study results. List unavailable checks explicitly. Do not claim a rendered result from token arithmetic alone.
