# Consent Frontend — React SPA (`web/`)

Read the relevant source before you change components, hooks, API types, or design tokens.
All file paths are repository-root-relative. Indented tree entries extend their parent path. Browser routes are URL paths.

## Overview

This React SPA manages agent permissions and tool approvals. The Go backend serves `web/dist` at `/`.
The browser redirects `/` to `/delegations` with history replacement.

## Governing References

| Source | Use |
| ------ | --- |
| `.specify/memory/constitution.md`, Principles XI and XIII | Design-system requirements and frontend acceptance tests |
| `ARCHITECTURE.md`, section 3.1.2 | Frontend architecture and backend boundary |
| `adrs/005-spa-serving-pattern.md` | Same-origin SPA deployment decision |
| `adrs/035-root-mounted-spa.md` | Current routes, filesystem serving, and superseded portions of ADR 005 |
| `adrs/006-frontend-stack.md` | Stack decisions. Use `web/package.json` for current dependency versions |
| `adrs/016-authorization-session-anti-spoofing.md` | Trusted consent metadata and opaque authorization redirects |
| `api/enduser/openapi.yaml` | HTTP request and response contracts |
| `specs/024-approval-api-ui/spec.md`, `specs/024-approval-api-ui/plan.md`, `specs/024-approval-api-ui/tasks.md` | Approval UI requirements, design, and implementation tasks |
| `specs/024-approval-api-ui/glob-approval-matching.md` | Approval parameter scopes and previews |

For feature work, read the governing feature's specification, plan, and tasks before changing behavior.

## Tech Stack

| Technology            | Version | Role                                           |
| --------------------- | ------- | ---------------------------------------------- |
| React                 | 19      | UI framework (StrictMode)                      |
| TypeScript            | 5.3+    | Type safety                                    |
| Vite                  | 7       | Build tool + dev server                        |
| Tailwind CSS          | 4       | Utility-first styling via semantic tokens      |
| Headless UI           | 2       | Accessible unstyled component primitives       |
| React Router          | 7       | Client-side routing (basename `/`)             |
| Axios                 | 1       | HTTP client with interceptors                  |
| Framer Motion         | 12      | Page transitions and animations                |
| Vitest                | 4       | Unit/integration testing                       |
| React Testing Library | 16      | Component testing                              |
| Storybook             | 10      | Design system documentation and visual testing |

## Source Structure

```
web/src/
  App.tsx              Router setup for consent, sessions, approvals, and tool authorizations
  main.tsx             Entry point — React.StrictMode mount

  components/
    consent/           DelegationCard, ServiceCard, ScopeList, GrantValidityControl,
                       ServiceRequirementCard, GrantStatusBadge
    approvals/         Tool-approval review components
    layout/            AppLayout, Header — page chrome
    sessions/          SessionCard, TerminationDialog — OAuth2 session management
    ui/                Reusable UI primitives (Button, Toast, ErrorBoundary, Skeleton,
                       EmptyState, Switch, InlineError, PageTransition, DatePicker)

  design-system/       Shared components and tokens — governed by Constitution XI
    components/        Design system component library (8 categories):
      primitives/        Button, Badge, Avatar, Spinner, Divider
      inputs/            TextInput, TextArea, Select, Checkbox, Radio, Switch, DatePicker
      data-display/      Card, Table, ScopeList, StatusIndicator
      layout/            AppLayout, Container, Stack, Grid, PageTransition
      navigation/        Tabs, Pagination, Breadcrumb
      overlays/          Modal, Tooltip, Dropdown, Popover
      feedback/          EmptyState, InlineError, Skeleton, Alert
      advanced/          Accordion, Progress
    tokens/            Colors, typography, spacing, shadows, radius, animation, z-index, and breakpoints
    utils/             cn() (clsx + tailwind-merge), a11y helpers, focus utilities
    docs/              Detailed design guides:
      INDEX.md                Complete documentation index
      DESIGN_PRINCIPLES.md    Visual philosophy — "Refined Trust Architecture"
      COLOR_GUIDE.md          Full color palette with hex values + WCAG ratios
      TOKEN_GUIDE.md          All design tokens with usage examples
      COMPONENT_ARCHETYPES.md Foundational component specifications
      COMMON_MISTAKES.md      Anti-patterns with correct solutions
      MOTION_GUIDE.md         Animation timing and easing specifications
      COMPONENT_PAIRING_GUIDE.md  Component composition patterns
      COMPOSITION_PATTERNS.md     Complex layout recipes
      DECISION_TREES.md           Which component to use when
      ACCESSIBILITY_GUIDE.md      WCAG 2.1 AA compliance requirements

  hooks/               Custom React hooks
    useConsent          Consent overview data fetching + state
    useAgentGrants      Agent detail + grant management
    useSessions         OAuth2 session listing + operations
    useToggleGrant      Grant enable/disable toggle logic
    useUpdateValidity   Grant expiration date management
    useApproval         Tool-approval data and actions

  pages/               Route components (lazy-loaded with React.lazy)
    ConsentOverviewPage     /delegations — agent delegation list
    AgentGrantDetailPage    /agents/:agentId — per-agent grants
    ThirdPartySessionsPage  /sessions — session management
    ApprovalPage            /approvals/:id — tool-approval review
    ToolAuthorizationsPage  /approvals — permanent approvals
    ErrorPage               * — fallback

  services/
    api/
      client.ts        Axios instance — baseURL: /api, 30s timeout, error interceptors
      consent.ts       Consent data and grant requests
      sessions.ts      Session list, detail, termination, and refresh requests
      approvals.ts     Tool-approval review, scope preview, decisions, lists, and revocation
      cache.ts         In-memory cache for GET responses
      index.ts         Barrel export

  styles/
    index.css          Global styles + Tailwind directives
    fonts.css          Self-hosted Crimson Pro, Manrope, and JetBrains Mono declarations

  types/
    consent.ts         Consent API types: user, agent, service, grant, scope, and permission set
    approval.ts         Tool-approval request and response types

  utils/
    validation.ts      Input validation and redirect URL safety
    scrollToError.ts   Scroll-to-first-error UX helper
```

## Path Aliases

`web/vite.config.ts`, `web/vitest.config.ts`, and `web/tsconfig.app.json` define application aliases.
`web/.storybook/main.ts` defines the subset that Storybook uses.

| Alias            | Path                  |
| ---------------- | --------------------- |
| `@design-system` | `web/src/design-system` |
| `@components`    | `web/src/components`    |
| `@hooks`         | `web/src/hooks`         |
| `@services`      | `web/src/services`      |
| `@types`         | `web/src/types`         |
| `@utils`         | `web/src/utils`         |
| `@assets`        | `web/src/assets`        |
| `@styles`        | `web/src/styles`        |

Use these aliases in imports. Do not use relative imports across alias boundaries.
The configured `@assets` target does not exist. Current font assets are in `web/public/fonts/`.

## Design System Rules

Before component work, read these guides:

- `web/src/design-system/docs/INDEX.md` for the documentation map
- `web/src/design-system/docs/DECISION_TREES.md` for component variants
- `web/src/design-system/docs/COMPONENT_PAIRING_GUIDE.md` for composition
- `web/src/design-system/docs/COMMON_MISTAKES.md` for known mistakes.

### Required Process

1. Build application components from `web/src/design-system/` primitives.
2. Add generally reusable components to `web/src/design-system/components/`.
3. Define color, type, spacing, radius, motion, and theme decisions as semantic tokens in `web/src/design-system/tokens/`.
4. Do not reference raw palette utilities from components.
5. Use Tailwind CSS v4 `@theme` tokens and CVA for component variants.
6. Support light and dark themes. Add stories and visual regression tests for new design-system components.
7. Make every story pass the accessibility addon in both themes.
8. Meet WCAG 2.1 AA. Use semantic HTML, correct ARIA attributes, visible focus, keyboard support, and reduced-motion behavior.
9. Self-host brand assets. Do not load fonts, scripts, or images from third-party origins.

Constitution XI governs these requirements. Existing code or guide examples do not prove compliance.
For conflicting examples, apply the constitution and report the source conflict.

### Current Visual Direction

`web/src/design-system/docs/DESIGN_PRINCIPLES.md` defines Refined Trust Architecture. A change to that direction requires an ADR.
The constitution does not prescribe that aesthetic.

| Detail | Current design guide |
| ------ | -------------------- |
| Typography | `font-display` for headings, `font-sans` for body text, `font-mono` for code |
| Elevation | Card shadows, with borders for containment |
| Motion | 150ms hover, 200ms state changes, 300ms modals, 500ms pages, with reduced-motion support |

Read `web/src/design-system/docs/TOKEN_GUIDE.md` and `web/src/design-system/docs/MOTION_GUIDE.md` before changing these details.

### Semantic Color Palette

These values describe `web/src/design-system/tokens/colors.css`, not permanent visual requirements.
Use semantic roles instead of copying hex values or raw palette scales into components.

| Token Family | Hex (primary)                            | Use                                 |
| ------------ | ---------------------------------------- | ----------------------------------- |
| `trust-*`    | #0A2540 (deep), #1E4D6B, #E8F1F5 (light) | Primary brand, headings, actions    |
| `cta-*`      | #D97706                                  | Call-to-action buttons and links    |
| `success-*`  | #059669                                  | Granted permissions, success states |
| `error-*`    | #DC2626                                  | Error states, destructive actions   |
| `warning-*`  | #D97706                                  | Warnings, attention signals         |
| `neutral-*`  | #faf9f7 → #1a1511 (50–900 scale)         | Palette behind semantic background, text, and border roles |

## API Client Pattern

- `web/src/services/api/client.ts` exports `apiClient`, with base URL `/api` and a 30-second timeout.
- The browser uses same-origin endpoints. Vite forwards backend namespaces in development. The Go backend serves production APIs.
- The Vite proxy adds `X-Remote-User` in development. An upstream proxy handles production authentication.
- `ConsentApiService`, `SessionsApiService`, and `approvalApi` provide typed `apiClient` methods.
- Consent GETs use a five-minute cache. Session GETs use a two-minute cache. Approval GETs do not use `apiCache`.
- Consent detail requests with `session_token` do not populate the cache.
- The response interceptor normalizes errors to `ApiError`.

After mutations, invalidate relevant cache entries.
For authorization resumption, treat the server's `redirect_url` as opaque.
Preserve redirect URL validation and full-page navigation, as required by ADR 016 and the OpenAPI contract.

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
| DELETE | `/api/third-party/:id/session`         | `sessionsApi.terminateSession(id)`            |
| POST   | `/api/third-party/:id/session/refresh` | `sessionsApi.refreshSession(id)`              |
| GET    | `/api/approvals/pending`               | `approvalApi.listPendingApprovals()`          |
| GET    | `/api/approvals/permanent`             | `approvalApi.listPermanentApprovals()`        |
| GET    | `/api/approvals/:id`                   | `approvalApi.getApproval(id)`                 |
| POST   | `/api/approvals/:id/approve`           | `approvalApi.approveApproval(id, request)`    |
| POST   | `/api/approvals/:id/scope-preview`     | `approvalApi.previewApprovalScope(id, request)` |
| POST   | `/api/approvals/:id/deny`              | `approvalApi.denyApproval(id, request)`       |
| POST   | `/api/approvals/:id/revoke`            | `approvalApi.revokePermanentApproval(id)`     |

## Development Setup

Run these commands from the repository root. `web/package.json` requires Node.js 24 or later.

```bash
just web-install          # Install npm dependencies
just web-dev              # Start Vite on :3000
just web-build            # Build the production frontend
just web-test             # Run frontend tests
just web-test-coverage    # Run frontend tests with coverage
```

Vite defaults to port 3000. It proxies `/api`, `/oauth2`, `/.well-known`, and `/health` to Go.
`VITE_API_URL` overrides the default backend target, `http://localhost:8000`.
Set `VITE_USE_POLLING=true` for Docker.

### Storybook

```bash
cd web && npm run storybook     # Port 6006
```

Storybook loads stories and MDX from `web/src/design-system/`. Keep application-specific components outside that catalog.
`web/.storybook/main.ts` and `web/.storybook/preview.ts` control the catalog and accessibility configuration.

## Testing Conventions

- **Framework**: Vitest, React Testing Library, and jsdom.
- **Setup**: `web/vitest.setup.ts` installs jest-dom matchers and calls `cleanup()` after each test.
- **File names**: `*.test.ts`, `*.test.tsx`, `*.interactive.test.tsx`, and `*.integration.test.tsx`.
- **Testing approach**: Test user-visible behavior. Use role, text, or label queries. If no semantic query exists, use `getByTestId`.
- **Mocking**: Mock API services at the module level. Do not mock React hooks. Mock their data sources.
- **Accessibility**: The linter enforces `jsx-a11y`. Components must support keyboard use and correct ARIA attributes.
- **Browser acceptance tests**: For UI changes, add or amend Playwright tests in `tests/e2e/frontend/`.
- **Screenshots**: Capture each meaningful UI state in `tests/e2e/screenshots/`, as required by Constitution XIII.
- **Harness guidance**: Read `tests/e2e/AGENTS.md` and `tests/e2e/README.md` before browser test work.

## Architecture Boundary

The frontend uses only HTTP endpoints in `api/enduser/openapi.yaml`.
It has no Go imports or shared backend types.
Keep `web/src/types/consent.ts`, `web/src/types/approval.ts`, and session types in `web/src/services/api/sessions.ts` aligned with the contract.

## Code Splitting

Page components load with `React.lazy()` inside `<Suspense>`. Load route bundles during navigation.
