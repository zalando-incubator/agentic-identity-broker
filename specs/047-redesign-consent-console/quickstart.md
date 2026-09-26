# Quickstart — Consent UI v2 Validation

## Status

This is a validation guide for the planned implementation. The planning change does not implement components, themes, or new command recipes.

Existing commands are identified below. Proposed commands become available during implementation. This guide is not a passing runtime report.

## Prerequisites

Use Node 24 or later, the repository Go toolchain, npm, just, Ginkgo, and the existing Playwright browser setup.

Before implementation, obtain acceptance for ADR 037. The feature adds no API contract, persistence, or migration.

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

Open `http://localhost:6006`. After implementation, select **Light** and **Dark** in the themes toolbar. The decorator sets `data-theme`, including portal content.

1. Open each primitive and shell story.
2. Switch between Light and Dark.
3. Open dialogs, menus, popovers, tooltips, Sheet, Command, and notifications.
4. Use the keyboard to reach and close each interactive element.
5. Inspect the Accessibility panel in both themes.

Expected result: readable controls, visible focus, theme-matched overlays, no automatic third-party requests, and no a11y violations.

The toolbar alone does not prove CI coverage. Implementation adds these recipes:

```bash
just web-storybook-build
just web-storybook-test
```

`web-storybook-build` wraps the existing `npm --prefix web run build-storybook` command. `web-storybook-test` runs the complete story set in two Vitest browser projects, one light and one dark. CI installs Chromium and uses the same Storybook 10 addon versions as the app.

Expected result: both projects run, and an intentional inaccessible fixture fails the gate during gate development. Production stories must use `parameters.a11y.test = 'error'`. Do not suppress failures with `todo`.

`web-storybook-test` also compares every story in each theme with its reviewed screenshot baseline (Principle XI visual regression). A missing or changed image fails the run. Baselines are Linux Chromium images from the screenshots workflow's review artifact; commit them only after review. Never pass `--update` in CI.

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

After the Go CSP change for the theme script, run:

```bash
just check
just test
```

The final merge gate is `just verify`.

## Shared design system smoke

Use Storybook to inspect both shells at desktop, 320 px, and 200% browser zoom. No page-level horizontal scrolling is permitted. Focus must remain visible when the mobile sidebar or a dialog opens.

Inspect font and image requests in the browser. All automatic resource requests must stay on the same origin. Outlined SVGs must contain no remote font imports. The black wordmark belongs to light mode and the white wordmark to dark mode.

Set theme to Dark, reload, and inspect the first frame. It must not flash light. Repeat with explicit Light on a dark OS, then System while changing the OS theme. Verify native controls and scrollbars.

Inspect the production response CSP. Fonts must use `font-src 'self'`. The inline theme script must run with its exact hash, without script `unsafe-inline`.

Verify that the complete application has one presentation without a feature flag.
No broker configuration, environment variable, CLI option, browser preference, or query parameter selects an old design.
Verify production and Vite routes against the same UI contract.

## Decision journeys

Run AS-01–AS-05 through the Ginkgo/Playwright suite. AS-15 and the themed route screenshots cover both themes; scenarios that change state run once in the default theme.

Start consent through the existing authorization flow, not a fabricated `session_token`. Select optional access and a duration. Connect a required provider, return, and verify that selections remain intact. Allow and observe the existing validated continuation.

Repeat with an existing grant. Only new selections require a decision. Existing selections remain granted and read-only, and the duration starts from the existing validity. Deny must leave the grant unchanged and show a local outcome without a constructed redirect.

Open a pending approval. Inspect the exact scope and expanded arguments. Exercise once, session, permanent, and denial in isolated runs. Resolve a request in another context and verify that stale actions disappear.

Expected result: one accent primary action, no sidebar, truthful identity/risk, and no change to existing authorization semantics.

## Console journeys

Run AS-06–AS-15. Search agents, confirm that an expired grant is absent, edit and cancel a draft, save a deliberate edit, and revoke with confirmation. Inject a failed revoke and verify that the pending row returns without overwriting unrelated row changes.

Use connection fixtures for expired refresh, usable access, and refresh rejection on `/sessions`. Click Refresh on the rejection fixture before checking its state. Use an agent that requires an unconnected service to see No connection on its Connections tab; `/sessions` never shows it. No fixture produces a Missing scopes state. Disconnect copy must state the provider-token limitation.

Create a pending approval while the console remains open. The queue and count must update within 15 seconds on the foreground test browser. Inspect network requests: only `/api/approvals/pending` supplies browser updates, never the gateway-wide endpoint.

Verify all original routes and both agent contexts without a flag. Inspect dependencies and build output for obsolete packages, fonts, palettes, caches, and animations.

## Preferences and command search

Run AS-17 and AS-18. Change the theme in Settings, reload, and open an approval. Settings offers no approval-persistence default, and approval choices behave as before. Use Command by keyboard to navigate only acting-user records.

See [the data model](data-model.md) and [UI contract](contracts/ui-and-configuration.md) for state and preference details.

## Visual snapshots

The current capture command is:

```bash
E2E_CAPTURE_SCREENSHOTS=true GINKGO_FRONTEND_PROCS=1 just test-e2e-frontend
```

Current captures go to `tests/e2e/frontend/coverage/screenshots/`. Reviewed baselines live in `tests/e2e/screenshots/`. Current capture is not a blocking comparison gate.

Implementation adds the planned gate recipe:

```bash
just test-e2e-frontend-visual
```

The recipe captures both themes serially and compares against reviewed baselines with the existing `tools/imgdiff`. It gates the required route images and the state stems listed in `tests/e2e/screenshots/visual-gate.txt`. It fails on a gated image without a baseline, a gated baseline without a capture, or excess difference, and publishes comparison artifacts. Baseline approval is a separate review action; no workflow commits baselines.

Required original-route filenames use these stems with `_light.png` and `_dark.png`:

- `delegations`
- `agent_consent`
- `agent_detail`
- `sessions`
- `approvals`
- `approval_review`.

Include `settings` in the same release. This produces at least 14 route/context/theme images. Include README references to every image. Keep capture data, clock, browser, OS, fonts, and viewport stable.

## Performance and manual acceptance

Build with `just web-build`. Measure the compressed initial dependency graph of each decision route. Each must remain below 150 kB without console Table or Command chunks.

Measure main-content appearance under the Chrome DevTools “Slow 4G” preset. It must remain below 1.5 seconds. Confirm at least 12 agent rows at a 1080 px-high desktop viewport.

Run moderated consent sessions for SC-001 and SC-007. Record decision time, service comprehension, and primary-action recognition. Automated accessibility and timing checks cannot establish those human outcomes.

## Evidence to attach

Attach both-theme Storybook results, semantic red/green test evidence, E2E results, reviewed screenshots, network/CSP observations, bundle measurements, and moderated-study results. List unavailable checks explicitly. Do not claim a rendered result from token arithmetic alone.
