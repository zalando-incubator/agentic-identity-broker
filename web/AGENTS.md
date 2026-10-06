# Consent Frontend — React SPA (`web/`)

**Use retrieval-led reasoning. Before you make assumptions about components, hooks, API types, or design tokens, read source files.**

## Overview

Use this React consent SPA for agent permissions and tool approvals. The Go backend serves it at root `/` (ADR 035).

## Consent UI v2: Accepted Direction

Feature 047 changes the visual system, not the retained application stack or backend API.
The stakeholder accepted ADR 038 on 2026-09-27 and approved its UX amendment on 2026-10-04.
Use the feature tasks and cutover inventory for implementation and validation status. A target does not prove a shipped result.

| Reference | Purpose |
| --- | --- |
| `../specs/047-redesign-consent-console/ux-improvement-plan.md` | 2026-10-04 approved UX targets and work packages; its ADR-draft statement is superseded by the accepted ADR history |
| `../specs/047-redesign-consent-console/spec.md` | Requirements and AS-01–AS-18 |
| `../specs/047-redesign-consent-console/plan.md` | Single-cutover implementation and acceptance gates |
| `../specs/047-redesign-consent-console/research.md` | Technology choices and integration evidence |
| `../specs/047-redesign-consent-console/contracts/` | UI, data-ownership, and single-cutover contracts without a new API |
| `../specs/047-redesign-consent-console/quickstart.md` | Storybook themes and validation commands |
| `../specs/047-redesign-consent-console/tasks.md` | Dependency order, implementation status, and acceptance requirements |
| `../specs/047-redesign-consent-console/data-model.md` | ConsentDraft, ConnectionState precedence, and browser preferences |
| `../adrs/038-design-system-rebuilt-on-shadcn-radix.md` | Accepted component, animation, and server-state decision |
| `../adrs/039-consent-selection-oauth-redirect-state.md` | Accepted bounded form POST and tab-local callback-state transport |
| `../adrs/040-canonical-light-visual-reference-gate.md` | Canonical light pixel references. Preserves six-project interaction/accessibility coverage |
| `../adrs/035-root-mounted-spa.md` | Binding root-mounted route behavior |
| `../api/enduser/openapi.yaml` | Canonical API contract |
| `../ARCHITECTURE.md` | Architecture and domain glossary |
| `../.specify/memory/constitution.md` | Binding principles |

### Constitutional requirements

Principle XI requires the shared design system, semantic tokens, self-hosted assets, and WCAG 2.1 AA.
Every component story must render and pass accessibility checks in light and dark themes. Storybook visual regression uses ADR 040's canonical light references.
The constitution does not prescribe an aesthetic.

### Accepted implementation contract

Use owned shadcn/Radix source inside the existing design-system categories.
Use Lucide, TanStack Query over Axios, cmdk, and Sonner. Motion is permitted on console routes only.
Keep CVA, `tailwind-merge`, and `cn()`. Do not create a competing primitive library.

Use tinted semantic OKLCH tokens, local outlined wordmarks, and self-hosted Zalando Sans Variable, Inter Variable, and JetBrains Mono.
Use three borders: subtle for cards and dividers, standard for buttons and popovers, control for inputs, checkboxes, and radios.
Use soft status surfaces. Keep `--primary-soft` for selection, not a second primary action.
The target values and their contrast gates live in `src/design-system/docs/COLOR_GUIDE.md` and `TOKEN_GUIDE.md`.

CSS and the shared `consoleMotion` constants use 120, 160, 200, and 320 ms and `cubic-bezier(0.2, 0, 0, 1)`.
Console routes can use owned animated Lucide icons and Motion; consent and standalone approval review use owned CSS animations only.
Under reduced motion, remove movement and icon animation but retain opacity and color fades.

Place ConsoleShell, DecisionShell, and PageHeader in `src/design-system/components/layout/`.
Shells are presentational and fetch no data. Keep Command, Motion, and console-only state out of decision bundles.
Application components obtain UI strings from `@copy`.
Primitives receive strings through props and never import the catalogue.

Deliver every route and consumer in one cutover.
Do not add feature flags, compatibility aliases, parallel presentations, or older-server fallbacks.
Preserve historical feature decisions without treating their styling as current instructions.

The browser refreshes only the acting-user `/api/approvals/pending` list, never the gateway-wide `GET /api/approvals`.
Keep authentication, scope preview, authorization-session validation, and callbacks unchanged.
An invalid authorization session remains a decision error, never an editable console fallback.

The Agents grid shows identity, expiry, relative change time, a linked detail surface, and confirmed Revoke in 120 px cards.
`activeGrantCount` counts UserGrant records, not permission sets. Show no permission count or make per-agent count requests.

Feature 047 targets WCAG 2.2 AA, visible unobscured focus, and announcements.
Use six-project story interactions and both-theme accessibility, plus Ginkgo/Playwright acceptance journeys. Apply ADR 040 to pixel references only.
These are requirements, not completed validation results.

## Target Frontend Stack

| Technology | Role |
| --- | --- |
| React 19 and TypeScript 5 | UI and type safety |
| Vite 7 and Tailwind CSS 4 | Build and CSS-first semantic styling |
| Owned Radix-based components | Accessible primitives in `src/design-system/` |
| React Router 7 | Root-mounted, lazy routes |
| Axios and TanStack Query 5 | Transport and principal-scoped server state |
| Motion (console only) | Owned animated icons and list/card layout changes; no decision-route imports |
| Lucide, cmdk, and Sonner | Icons, command search, and notifications |
| Vitest 4 and Testing Library 16 | Behavior tests |
| Storybook 10 and Playwright | Component accessibility and visual checks |

Use `package.json` and the lockfile for exact installed versions.

## Source Routes

Read these files before you change their behavior:

`src/App.tsx` | Lazy routes and decision/console layout boundaries
`src/main.tsx` | ThemeProvider, QueryProvider, and application mount
`src/components/layout/ConsoleLayout.tsx` | One pending-approval provider, Toaster, navigation, user menu, and lazy command search
`src/components/layout/AgentRoute.tsx` | Decision context selected by the presence of `session_token`, including an empty value
`src/components/consent/consentDraft.ts` | Permission locks, prior selections, validity, dirty state, and canonical callback drafts
`src/components/sessions/connectionState.ts` | Connection-state precedence and authoritative refresh results
`src/hooks/` | Principal-scoped reads and server-confirmed mutations
`src/services/query/` | Identity boundary, query ownership, and cache lifecycle
`src/services/api/` | Typed Axios clients and normalized errors
`src/copy/index.ts` | Shared copy catalogue and view-specific exports
`src/types/consent.ts`, `src/types/approval.ts` | Existing request and response contracts
`src/services/storage/session.ts` | Ephemeral callback draft with ID, service, origin, path, and expiry binding; no API-response cache
`src/utils/validation.ts` | Safe continuation URL checks

| Target route | View |
| --- | --- |
| `/agents` | Agents grid/list |
| `/agents/:agentId?session_token=…` | Consent decision |
| `/agents/:agentId` | Agent detail |
| `/connections` | Connections grid/list |
| `/approvals` | Pending approval inbox |
| `/approvals/remembered` | Remembered approvals |
| `/approvals/:id` | Standalone approval review |
| `/settings/appearance` | Settings appearance category |
| Unknown path | Error page |

`src/App.tsx` owns route-to-page imports. Update the provider callback URL and page objects in the same route cutover.

### Shared design system

`src/design-system/components/primitives/` | Button, Badge, Avatar, Separator, Wordmark
`src/design-system/components/inputs/` | Input, TextArea, Select, Checkbox, RadioGroup, Switch, DatePicker
`src/design-system/components/data-display/` | Card, TruncatedText, and shared entity presentation
`src/design-system/components/layout/` | ConsoleShell, DecisionShell, PageHeader
`src/design-system/components/navigation/` | Tabs
`src/design-system/components/overlays/` | Dialog, Sheet, DropdownMenu, Tooltip, Popover
`src/design-system/components/feedback/` | EmptyState, errors, Skeleton, Alert, Toaster
`src/design-system/components/advanced/` | Accordion and concrete Command module
`src/design-system/theme/` | Safe preferences, first-paint script, ThemeProvider, ThemeChoice
`src/design-system/tokens/theme.css` | Semantic light/dark tokens
`src/styles/index.css`, `src/styles/fonts.css` | Tailwind token mapping and local font faces
`src/design-system/utils/` | `cn()`, accessibility and focus utilities
`public/brand/`, `public/fonts/` | Local outlined artwork and licensed fonts
`build/themeInitPlugin.ts` | Fixed inline script and CSP hash artifact
`build/decisionModulesPlugin.ts`, `build/decisionBundle.test.ts` | Decision import isolation and compressed-size gate
`build/assetCompressionPlugin.ts` | Build-time gzip and Brotli companions for public text assets

### Detailed design guides

`src/design-system/docs/INDEX.md` | Documentation index
`src/design-system/docs/DESIGN_PRINCIPLES.md` | Current direction and historical context
`src/design-system/docs/COLOR_GUIDE.md` | Semantic color contract
`src/design-system/docs/TOKEN_GUIDE.md` | Token use
`src/design-system/docs/COMPONENT_ARCHETYPES.md` | Component contracts
`src/design-system/docs/COMMON_MISTAKES.md` | Patterns to avoid
`src/design-system/docs/MOTION_GUIDE.md` | CSS decisions, console Motion, and reduced-motion fades
`src/design-system/docs/COMPONENT_PAIRING_GUIDE.md` | Component combinations
`src/design-system/docs/COMPOSITION_PATTERNS.md` | Layout composition
`src/design-system/docs/DECISION_TREES.md` | Component selection
`src/design-system/docs/ACCESSIBILITY_GUIDE.md` | WCAG requirements

## Path Aliases

Vite and Vitest define these aliases. In Storybook, define only the aliases that it uses.

| Alias            | Path                  |
| ---------------- | --------------------- |
| `@design-system` | `./src/design-system` |
| `@copy`         | `./src/copy`          |
| `@components`    | `./src/components`    |
| `@hooks`         | `./src/hooks`         |
| `@services`      | `./src/services`      |
| `@app-types`     | `./src/types`         |
| `@utils`         | `./src/utils`         |
| `@assets`        | `./src/assets`        |
| `@styles`        | `./src/styles`        |

Use these aliases in imports. Do not use relative imports across alias boundaries.


## Design System Rules

Use `src/design-system/` as the **single source of truth** for visual decisions.
Before you write a styled component, read `src/design-system/docs/COMMON_MISTAKES.md`.

### Accepted Design Rules

1. Use semantic color roles, not raw palette utilities or component-local colors.
2. Use Zalando Sans for display text, Inter for body text and tabular dates/counts, and JetBrains Mono for identifiers.
3. Keep cards quiet with subtle 1 px edges. Reserve strong control boundaries for inputs, checkboxes, and radios.
4. Use the centered 1120 px console, 12-column grid, and a 480 px three-zone consent card with a pinned footer.
5. Use EntityCard grids with a list toggle for Agents and Connections. Use fixed-height approval rows and a separate detail panel.
6. Give resting actions a visible boundary. Agent and connection collections use `destructive-quiet` with `ShieldOff` and `Unlink`, respectively. Keep `destructive-outline` for agent detail.
7. Keep the consent primary label as Allow. Disable it while selected services need connecting, explain why, and keep inline Connect actions available.
8. Use Motion only on console routes and CSS animation on decision routes. Reduced motion retains opacity and color fades.
9. Use ConsoleShell with PageHeader for the console, and DecisionShell for focused decisions.
10. Keep focus, labels, contrast, keyboard behavior, and announcements accessible in both themes.
11. Read `src/design-system/docs/DECISION_TREES.md` and `COMPONENT_PAIRING_GUIDE.md` before adding a composition.

Remove obsolete components and callers together. Do not preserve their APIs through wrappers.

Use local outlined assets in `public/brand/` and self-hosted fonts in `public/fonts/`.
If an agent or service image is off-origin, show a local fallback without an external request.
Appearance and collection-view preferences never submit or preselect a consent or tool decision.

## API Client Pattern

- `services/api/client.ts` exports the configured Axios instance (`apiClient`).
- The base URL is `/api`. In development, Vite forwards requests to Go. In production, use the upstream proxy.
- **Authentication is external** — The Vite proxy adds `X-Remote-User` in development. An upstream proxy handles production authentication.
- `ConsentApiService`, `SessionsApiService`, and `approvalApi` provide typed `apiClient` methods.
- TanStack Query owns server state. Query keys start with the acting principal; API clients do not keep a separate cache.
- QueryProvider waits for `/api/me` and removes departed-principal data before rendering a new identity.
- Reads forward AbortSignal. Mutations do not retry. Grant, approval, and refresh results remain server-authoritative.
- Confirm revocation before a request. Keep only the affected row pending and report success after the server accepts it.
- Store only `aib.theme`, `aib.sidebar-collapsed`, `aib.collection-view-default`, `aib.collection-view.agents`, and `aib.collection-view.connections` in localStorage. Do not persist API responses or authorization context.
- The response interceptor normalizes errors to `ApiError`.

### Key API Endpoints

| Method | Endpoint                               | Service method                                |
| ------ | -------------------------------------- | --------------------------------------------- |
| GET    | `/api/me`                              | `consentApi.getUserInfo()`                    |
| GET    | `/api/consent/agents`                  | `consentApi.getAgentDelegations()`            |
| GET    | `/api/consent/agents/:id`              | `consentApi.getAgentDetail(id)`               |
| GET    | `/api/consent/agents/:id/grants`       | `consentApi.getAgentGrants(id)`               |
| POST   | `/api/consent/agents/:id/grants`       | `consentApi.createOrUpdateGrant(id, request)` |
| DELETE | `/api/consent/agents/:id/grants`       | `consentApi.deleteGrant(id)`                  |
| GET    | `/api/third-party/sessions`            | `sessionsApi.listSessions()`                  |
| GET    | `/api/third-party/:id/session`         | `sessionsApi.getSessionDetails(id)`           |
| GET    | `/api/third-party/:id/session/affected-agents` | `sessionsApi.getAffectedAgents(id)` |
| DELETE | `/api/third-party/:id/session`         | `sessionsApi.terminateSession(id)`            |
| POST   | `/api/third-party/:id/session/refresh` | `sessionsApi.refreshSession(id)`              |
| GET    | `/api/approvals/pending`               | `approvalApi.listPendingApprovals()`          |
| GET    | `/api/approvals/permanent`             | `approvalApi.listPermanentApprovals()`        |
| GET    | `/api/approvals/:id`                   | `approvalApi.getApproval(id)`                 |
| POST   | `/api/approvals/:id/approve`           | `approvalApi.approveApproval(id, request)`    |
| POST   | `/api/approvals/:id/deny`              | `approvalApi.denyApproval(id, request)`       |
| POST   | `/api/approvals/:id/revoke`            | `approvalApi.revokePermanentApproval(id)`     |

## Development Setup

```bash
just web-install          # Install npm dependencies
just web-dev              # Start Vite on :3000
just web-build            # Build the production frontend
just web-test             # Run frontend tests
just web-test-coverage    # Run frontend tests with coverage
just web-lint             # Accessibility and semantic-token rules
just web-bundle-check     # Decision-route budget and import isolation
just web-storybook-build  # Build component documentation
just web-storybook-test   # Run six projects; compare canonical light references without updates
```

Compose serves the current Vite UI at `http://localhost:8000` and keeps the Go end-user port internal. Do not start native Vite alongside Compose. Native `just run` and `just dev` build and serve the current frontend at :8000. For optional native Vite on :3000, start Go with `IDENTITY_BROKER_SERVER_ENDUSER_PUBLIC_URL=http://localhost:3000`. Vite proxies backend namespaces and adds the development principal header. Compose sets polling and HMR client port 8000.

### Storybook

```bash
cd web && npm run storybook     # Port 6006
```

Storybook covers foundation MDX, shared pattern stories, and screen stories built from pure `*View` components with shared fixtures.
Do not fetch network data in stories or replace security-state handling with mocks. Use `just web-storybook-visual-candidates` for review images.
Only human-approved Linux Chromium captures belong in `.storybook/__screenshots__/`.
Pixel capture and comparison require desktop `storybook-light`, effective light theme, and an actual 1280 × 720 viewport (ADR 040).
Intentional dark or narrow overrides still run interactions and accessibility checks without PNG references.

## Testing Conventions

- **Framework**: Vitest, React Testing Library, and jsdom.
- **Setup**: In `vitest.setup.ts`, add jest-dom matchers. Call `cleanup()` after each test.
- **File names**: `*.test.ts`, `*.test.tsx`, `*.interactive.test.tsx`, and `*.integration.test.tsx`.
- **Testing approach**: Test user-visible behavior. Use role, text, or label queries. If no semantic query exists, use `getByTestId`.
- **Mocking**: Mock API services at the module level. Do not mock React hooks. Mock their data sources.
- **Integration journeys**: `src/pages/*.integration.test.tsx` uses the real App, providers, hooks, and API clients. Replace only HTTP responses through `applicationIntegrationTestSupport.tsx`. Both `just web-test` and `just web-test-coverage` include these suites.
- **Accessibility**: The linter enforces `jsx-a11y`. Components must support keyboard use and correct ARIA attributes.

## Architecture Boundary

The frontend uses only HTTP endpoints in `../api/enduser/openapi.yaml`.
It has no Go imports or shared backend types. Keep TypeScript API types in sync with the contract.

## Code Splitting

Page components load with `React.lazy()` inside `<Suspense>`. Load route bundles during navigation.

Decision routes must not import Command, Motion, or console-only state through shared barrels.
Settings uses a lazy `/settings/appearance` route for theme and the default collection view, not a saved approval-persistence default.
