# Quickstart — Consent UI v2

## Status

This guide defines the post-cutover review. The branch implements the routes and controls below. Use the normal automated checks and reviewed visual baselines. No separate release-approval process is required. Do not approve screenshots automatically.

## Prerequisites

Install Docker with Compose, Go 1.27.1, Node 24 or later, npm, just, OpenSSL, curl, Python 3, and jq. Automated browser checks also need the existing Playwright browser setup and Ginkgo.

Console motion uses the installed `motion@12.30.0` pin; the registry package-age gate excluded Motion 14. Decision routes do not import Motion. This dependency pin is not a build, accessibility, bundle or browser-check result.

Run commands from the repository root. Compose starts the frontend, broker, and mocks in the background. Start Storybook in a separate terminal.

The local broker trusts a development identity header. Run these services only on a trusted machine and network. Never commit the locally generated broker secrets.

## Install and inspect commands

These commands exist today:

```bash
just --list
just web-ci
```

`web-ci` installs the committed lockfile with strict engine checking.

## Start the live UI and sample data

An isolated Compose project runs Vite, the broker, Sample Agent, and OAuth2 mocks. Its only browser origin is `http://localhost:8000`.
Use this setup for real approval links: root `config.yaml` does not configure approval authentication. The manual approval script does not need an MCP gateway.

1. Make sure that ports 6006, 8000, 9000, 9001, 9002, 9004, and 14000 are free. Do not stop another person's service.
   The broker links and callbacks use port 8000. Port 3000 remains internal to the frontend container.
   If these ports are occupied, use different project names and port overrides. Update the broker public URL, HMR client port, and Sample Agent browser URLs together.

   ```bash
   lsof -nP -iTCP:6006 -iTCP:8000 -iTCP:9000 -iTCP:9001 -iTCP:9002 -iTCP:9004 -iTCP:14000 -sTCP:LISTEN
   docker ps -a --filter label=com.docker.compose.project=consent-ui-review
   ```

2. Create an ignored Compose override for the mounted broker binary and separate image names. Compose assigns container names and networks per project.

   ```bash
   mkdir -p tmp
   cat > tmp/consent-ui-compose.override.yaml <<'YAML'
   services:
     identity-broker:
       image: consent-ui-review-identity-broker:local
       command: ["bash", "scripts/start-broker.sh"]
       volumes:
         - ./bin/linux/arm64/agentic-identity-broker:/app/tmp/agentic-identity-broker:ro
     frontend:
       image: consent-ui-review-frontend:local
     sample-agent:
       image: consent-ui-review-sample-agent:local
     upstream-oauth2:
       image: consent-ui-review-upstream-oauth2:local
     third-party-oauth2:
       image: consent-ui-review-third-party-oauth2:local
     cimd-mock:
       image: consent-ui-review-cimd-mock:local
   volumes:
     aib-frontend-node-modules:
       name: consent-ui-review-frontend-node-modules
   YAML
   ```
   The mount uses Linux arm64. Run `docker info --format '{{.Architecture}}'` to see Docker's architecture. If it reports `x86_64`, change `arm64` to `amd64` in the mount.

3. Generate two local secrets once. Keep the ignored `tmp/consent-ui-secrets.env` file for restarts. Do not print, share, or commit its contents.

   ```bash
   umask 077
   printf 'IDENTITY_BROKER_JWE_SIGNING_KEY=%s\nIDENTITY_BROKER_ENCRYPTION_MEMORY_RAW_KEY=%s\n' \
     "$(./scripts/generate-jwe-key.sh)" \
     "$(./scripts/generate-jwe-key.sh)" > tmp/consent-ui-secrets.env
   ```

4. Build the current Linux broker binary. For Docker `aarch64` (Apple Silicon), run:

   ```bash
   just build-linux-arm64
   ```

   If Docker reports `x86_64`, run `just build-linux-amd64` instead. The mounted binary avoids a heavy Go build inside the broker container.

5. Build the isolated development images from the current Dockerfiles. These unique image names do not replace another project's images.

   ```bash
   docker compose -p consent-ui-review --env-file tmp/consent-ui-secrets.env \
     -f docker-compose.yml -f tmp/consent-ui-compose.override.yaml \
     build identity-broker frontend sample-agent upstream-oauth2 third-party-oauth2 cimd-mock
   ```

6. Start the frontend, broker, Sample Agent, and their declared mock dependencies. Wait for the health checks to finish.

   ```bash
   docker compose -p consent-ui-review --env-file tmp/consent-ui-secrets.env \
     -f docker-compose.yml -f tmp/consent-ui-compose.override.yaml \
     up --no-build -d --wait identity-broker frontend sample-agent
   ```

7. Open `http://localhost:8000/`. Do not start `just web-dev` alongside this Compose project.
   Compose serves the current `web/` source through Vite, including HMR and the development API proxy.

The broker seeds its in-memory data after startup. List the five sample agents through the admin API:

```bash
curl -fsS -H 'X-Remote-User: dev@example.com' http://localhost:14000/api/agents | jq -r '.[].display_name'
```

Look for **Weather Assistant**, **Task Manager Pro**, **Sample Agent**, **Local Research Agent**, and **CIMD Demo Agent**. The seed script also creates four services: Mock OAuth2, Weather, Calendar, and Email. Its permission sets are Identity Access, Weather Access, and Calendar Sync.

If the list is empty after the Sample Agent becomes healthy, seed the broker once with its existing script:

```bash
docker compose -p consent-ui-review --env-file tmp/consent-ui-secrets.env \
  -f docker-compose.yml -f tmp/consent-ui-compose.override.yaml \
  exec -e MOCK_SERVER_TOKEN_URL=http://third-party-oauth2:9000 \
  identity-broker bash scripts/setup-dev-data.sh
```

The local addresses are:

| Service | Address |
| --- | --- |
| Browser UI and API proxy (Vite) | `http://localhost:8000/` |
| Sample Agent | `http://localhost:9002/` |
| Third-party OAuth2 | `http://localhost:9000/` |
| Upstream OAuth2 | `http://localhost:9001/` |
| Broker health | `http://localhost:8000/health` |
| Admin health | `http://localhost:14000/health` |
| CIMD mock (self-signed certificate) | `https://localhost:9004/health` |

## Review the seven destination routes and both agent contexts

The Vite server at `http://localhost:8000/` supplies development identity `dev@example.com` to the broker API. The mock upstream provider uses the same user. Do not invent a `session_token`. The Sample Agent creates the authorization request.

1. Open `http://localhost:9002/` and select **Login as Proxy Client**. The browser opens the decision view at `/agents/:id?session_token=…` on port 8000.
2. Inspect the required **Identity Access** group in the inset permission panel, optional groups, signed-in identity, and pinned duration/actions. Leave **Weather Access** and **Calendar Sync** off for this run. Their provider URLs are examples, not running local services.
3. Select **30 days**. If Mock OAuth2 Service is selected but unconnected, **Allow** is disabled with an explanation. Choose the inline **Connect** action for Mock OAuth2 Service. The mock provider opens at `http://localhost:9000/oauth/authorize`.
4. Click **Approve** on the mock provider. The browser returns to the decision card and keeps the 30-day choice. **Allow** is now enabled; click it.
5. On the upstream provider at `http://localhost:9001/oauth/authorize`, click **Approve**. The browser returns to the Sample Agent with an OAuth2 success message.
6. Open `http://localhost:8000/agents`. Find **Sample Agent** and its expiry. Open its whole card to inspect `/agents/:id` without `session_token`. The editable permission panel and Connections rail appear together, not as tabs.
7. Open `http://localhost:8000/connections`. Find the connected **Mock OAuth2 Service (Dev)** and open its four-scope popover.
8. In a third terminal, create a real pending approval:

   ```bash
   bash scripts/create-manual-tool-approval.sh
   ```

   The script obtains signed tokens from the mock upstream server. It prints **Pending approval created** and an **Open:** URL at `http://localhost:8000/approvals/:id`. Open `http://localhost:8000/approvals` to see the pending inbox; select the row to inspect the detail panel and decide without expanding the list. Below 1012 px, the row opens `/approvals/:id` instead. Open the printed URL for the standalone decision view. The request expires after ten minutes. Run the script again for a fresh request.

   The inbox panel leads with the request description, or the tool name when no description exists.
   Agent, risk, and time left follow the title. Arguments start the panel body; the acting user appears only on standalone review.
   The list legend shows J, K, and Enter. The joined decision buttons show A and D shortcuts.
   These shortcuts require focus inside the pending inbox or its review panel. Body and unrelated-control focus cannot authorize or deny a request.
   Only the submitted action spins. Both actions stay disabled until the server responds.

9. Open `http://localhost:8000/approvals/remembered`. Search tools, agents, or patterns with the page-header toolbar.
   Combine search with All, Always allowed, or Always denied. The URL preserves search and filter choices.
   Inspect a scope popover before a confirmed revoke.
10. Open `http://localhost:8000/settings/appearance` from the user menu. The return link opens Agents.
    Choose Light, Dark, or System and the default grid/list view. Reload to inspect saved preferences.
    Settings rows use compact labels and a 672 px maximum width.
    Console search shows ⌘K on macOS and Ctrl K elsewhere. Clipped text uses an inline More or Less link.

If Sample Agent already has access, another proxy login skips the decision and opens the upstream provider. To see consent again, revoke Sample Agent on `/agents` and confirm the dialog. To repeat Connect, disconnect **Mock OAuth2 Service (Dev)** on `/connections`. Use the stop and restart steps that follow for a full reset.

The decision and console detail views share `/agents/:id`; only a validated authorization request selects the decision view. The linked agent card opens console detail. Step 8 prints a real `/approvals/:id` link. Do not invent a session token or approval ID.

## Stop and restart

Stop only the isolated review containers with this command:

```bash
docker compose -p consent-ui-review --env-file tmp/consent-ui-secrets.env \
  -f docker-compose.yml -f tmp/consent-ui-compose.override.yaml stop
```

For a restart, keep the same secret file and repeat startup step 6. The frontend container provides Vite again.
The broker stores data in memory. Stopping or restarting the broker clears grants, connections, and approvals.
It seeds sample agents again. Repeat the consent flow and approval command. Do not use `just compose-down`.
That command targets a different Compose project.

## Storybook: light and dark

In another terminal, start Storybook:

```bash
npm --prefix web run storybook -- --no-open
```

Open `http://localhost:6006/`. Choose **Light** and **Dark** in the themes toolbar. Foundation stories show token roles, pattern stories cover shared entity and decision components, and screen stories compose pure views from shared fixtures without a broker or network mock.

1. Inspect foundation token roles and patterns in both themes.
2. Inspect consent, Agents, agent detail, Connections, Approvals and Settings screen states at 375, 768 and 1280 px.
3. Open dialogs, menus, popovers, tooltips, Sheet, Command and notifications.
4. Use the keyboard to complete a permission, duration, approval and revoke interaction.
5. Inspect the Accessibility panel and reduced-motion behavior in both themes.

Expected result: readable controls, visible focus, no layout jump on approval choices, a visible Allow button at 1280 × 720, and no automatic third-party requests or a11y violations.

Stop Storybook with Ctrl+C in its terminal. The main UI on port 8000 continues to run.

The toolbar alone does not prove CI coverage. Use these recipes:

```bash
just web-storybook-build
just web-storybook-test
```

`web-storybook-build` wraps `npm --prefix web run build-storybook`. The local test and candidate recipes run six browser projects serially: light and dark at 375 × 812, 768 × 1024, and 1280 × 720. GitHub CI runs those projects as six parallel Storybook matrix jobs, independently of Unit tests. Each runner executes only its selected project, and the final CI gate requires the complete matrix. Shared component documentation builds once in the canonical light project. CI installs the same Storybook addon versions as the app.

CI and component-candidate captures use the same digest-pinned `mcr.microsoft.com/playwright:v1.62.1-noble` image. It fixes Chromium and system fallback fonts, including the Command-key glyph. Local PNG comparisons require the same image. `PLAYWRIGHT_WS_ENDPOINT` can connect Vitest to a Playwright server in that image.

Expected result: all six projects run. An intentional inaccessible fixture fails the gate during gate development. Production stories use `parameters.a11y.test = 'error'`. Do not suppress failures with `todo`.

Under [ADR 040](../../adrs/040-canonical-light-visual-reference-gate.md), `web-storybook-test` compares PNGs only in desktop `storybook-light` with effective light theme and an actual 1280 × 720 viewport. Intentional dark or narrow overrides keep interactions and accessibility checks without references. A missing or changed required canonical image fails the run. References are human-reviewed Linux Chromium images from the screenshots workflow's artifact. Never pass `--update` in CI.

Story interactions run with normal motion and an isolated pointer position. Capture then uses reduced-motion final states and completes finite visual feedback. The harness removes Storybook's portable-story animation pause before capture so stale exit overlays cannot remain in the reference image. Focus stories wait for real dismissal and focus restoration.

## Existing unit and end-to-end commands

```bash
just web-test
just web-test-coverage
just test-e2e-frontend
```

Built-mode frontend E2E starts isolated broker instances through the production bootstrap. It supplies the principal through the existing test authentication setup. This is the preferred complete-flow validation path.

For native Vite E2E work without this Compose project, run `just web-dev` in another terminal. Then run:

```bash
just test-e2e-frontend-dev
```

Do not start a separate example broker on Vite's port 3000. Built-mode E2E avoids that conflict.

After a change to the Go SPA handler (CSP hash, compressed assets, preload headers) or the OAuth2 session callback, run:

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

Run AS-01–AS-05 through the Ginkgo/Playwright suite. AS-15 covers both themes and responsive layouts. Route pixel references use only light mode at 1280 × 720. Scenarios that change state run once in the default theme.

Start consent through the existing authorization flow, not a fabricated `session_token`. Select optional access and a duration. Connect a required provider, return, and verify that selections remain intact. Allow and observe the existing validated continuation.

Repeat with an existing grant. Only new selections require a decision. Existing selections remain granted and read-only, and the duration starts from the existing validity. Deny must leave the grant unchanged and show a local outcome without a constructed redirect.

Open a pending approval. Its top-level arguments appear in the detail panel when six or fewer keys exist, and **View JSON** opens the full payload. Inspect exact scope before a remembered choice. Exercise once, session, permanent and denial in isolated runs. Resolve a request in another context and make sure stale actions disappear.

Expected result: one accent primary action, no sidebar on standalone decisions, truthful identity and risk, and unchanged authorization semantics. Decision bundles import no Motion code.

## Console journeys

Run AS-06–AS-15. Search `/agents`, follow an agent card, confirm that an expired grant is absent, edit and cancel a draft, save a deliberate edit, and revoke through a named confirmation. Toggle grid/list, then clear the saved page choice and check the default list above twelve entities. Inject a failed revoke: only that entity shows a busy state, and it stays in the collection.

On agent detail, service names and permission descriptions wrap without a hiding toggle. Optional services have visible selected-state controls. Change **Access lasts** and compare **Current access** with **After saving**. A selected custom date stays beside the selector. The save bar appears immediately below the header. Follow **Back to Agents** before saving: **Keep editing** preserves the draft, while **Discard changes** leaves without saving. Reloading or closing the tab also warns about unsaved edits.

Use connection fixtures for expired refresh, usable access and refresh rejection on `/connections`. Click Refresh on the rejection fixture before checking its state. Only the agent-detail Connections rail shows No connection, and only without a stored session. Consent marks the missing service with a warning icon and Connect. `/connections` never shows No connection. A failed read shows Unavailable with Retry. There is no Missing scopes state. The Disconnect dialog states the provider-token limit. Inspect the return URL and success/error history replacement on `/connections`.

Create a pending approval while the console remains open. Queue/count updates must arrive within 15 seconds in the foreground. Select a row and open the scope editor; the pending list's bounds must not move. Open `/approvals/remembered` for standing decisions. Browser updates use `/api/approvals/pending`, never the gateway-wide endpoint.

Visit all seven destination routes and both agent contexts without a flag or old-route aliases. Inspect dependencies and build output for obsolete packages, fonts, palettes, caches and animation imports on decision routes.

## Preferences and command search

Run AS-17 and AS-18. Change the theme and default grid/list choice at `/settings/appearance`, reload, and open an approval and a collection. Settings has no approval-persistence default. Use Command by keyboard to navigate only acting-user records.

See [the data model](data-model.md) and [UI contract](contracts/ui-and-configuration.md) for state and preference details.

## Visual snapshots

The current capture command is:

```bash
E2E_CAPTURE_SCREENSHOTS=true GINKGO_FRONTEND_PROCS=1 just test-e2e-frontend
```

Current captures go to `tests/e2e/frontend/coverage/screenshots/`. Reviewed baselines live in `tests/e2e/screenshots/`. Current capture is not a blocking comparison gate.

Run the comparison gate after human-approved baselines are published:

```bash
just test-e2e-frontend-visual
```

The recipe compares canonical light captures at 1280 × 720 with reviewed references through the existing `tools/imgdiff`. It gates the required route images and canonical light state stems in `tests/e2e/screenshots/visual-gate.txt`. The capture helper can also emit dark candidates, which the gate ignores.

It fails on a gated image without a reference, a gated reference without a capture, or excess difference. It publishes comparison artifacts. Existing thresholds remain unchanged. Reference approval is a separate review action. No workflow commits references.

Required destination-route stems use `_light.png` at 1280 × 720:

- `agents`.
- `agent_consent`.
- `agent_detail`.
- `connections`.
- `approvals`.
- `remembered_approvals`.
- `approval_review`.
- `settings`.

These eight route/context stems produce eight required light images under [ADR 040](../../adrs/040-canonical-light-visual-reference-gate.md). Dark and narrow PNGs are not required. Keep route guidance in the documentation, not a README gallery. Keep capture data, clock, browser, OS, fonts and viewport stable.

**Historical approval (2026-10-05)**: The stakeholder approved the initial Linux Chromium set of 16 route images and 2,418 Storybook images. It covered 403 stories across six theme/viewport projects, captured with Playwright 1.62.1. ADR 040 supersedes that reference matrix on 2026-10-06.

The retained canonical set contains 376 Storybook references and eight required route references. Every retained reference uses effective light mode at an actual 1280 × 720 viewport. Intentional dark or narrow Storybook overrides remain behavioral/accessibility cases without PNG references. See the [reference index](../../tests/e2e/README.md#reviewed-frontend-reference-images).

Route fixtures use stable logical identifiers inside fresh scenario storage. This keeps avatar tints and technical identifiers repeatable without sharing mutable entities, changing authorization, or changing server TTLs.

## Performance and manual acceptance

Build with `just web-build`. Measure the compressed initial graph of each decision route. Each stays below the approved 190-kB gzip limit and imports no console Motion, Command or list chunks.

Measure consent content with a cold cache on Chrome DevTools “Slow 4G”; it must appear within 5 seconds. Check the pinned Allow position at 1280 × 720 with three permission groups and a risk callout. Confirm compressed public assets and only public code hints. Inspect the three/two/one card columns and, without a saved page choice, the list default above twelve items.

Use the manual browser walk-through to report unclear labels and misleading actions. No dedicated user study will occur. SC-001 decision-time and comprehension results, and SC-007 participant-recognition results, remain unmeasured.

## Evidence to attach

Attach both-theme Storybook results, E2E results, reviewed screenshots, network/CSP observations, and bundle measurements. List unavailable checks explicitly. Do not claim a rendered result from token arithmetic alone.
