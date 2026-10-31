# Consent Frontend — React SPA (`web/`)

**Use retrieval-led reasoning. Before you make assumptions about components, hooks, API types, or design tokens, read source files.**

## Overview

Use this React consent SPA for agent permissions and tool approvals. The Go backend serves it at root `/` (ADR 035). Use the Refined Trust Architecture and WCAG 2.1 AA.

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
src/
  App.tsx              Router setup for consent, sessions, approvals, and tool authorizations
  main.tsx             Entry point — React.StrictMode mount

  components/
    consent/           Consent-specific components (DelegationCard, ServiceCard, ScopeList,
                       GrantValidityControl, ServiceRequirementCard, GrantStatusBadge, etc.)
    approvals/         Tool-approval review components
    layout/            AppLayout, Header — page chrome
    sessions/          SessionCard, TerminationDialog — OAuth2 session management
    ui/                Reusable UI primitives (Button, Toast, ErrorBoundary, Skeleton,
                       EmptyState, Switch, InlineError, PageTransition, DatePicker)

  design-system/       ★ AUTHORITATIVE design reference — read before styling anything
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
    docs/              ★ Read before any UI work:
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
    ThirdPartySessionDetailPage /sessions/:service — one service's session (Virtual Service Scope page)
    ApprovalPage            /approvals/:id — tool-approval review
    ToolAuthorizationsPage  /approvals — permanent approvals
    ErrorPage               * — fallback

  services/
    api/
      client.ts        Axios instance — baseURL: /api, 30s timeout, error interceptors
      consent.ts       Consent data and grant requests
      sessions.ts      Session list, detail, termination, and refresh requests
      approvals.ts     Tool-approval get, approve, deny, list, and revoke requests
      cache.ts         In-memory cache for GET responses
      index.ts         Barrel export

  styles/
    index.css          Global styles + Tailwind directives
    fonts.css          Font imports (Crimson Pro, Manrope, JetBrains Mono)

  types/
    consent.ts         All API response/request TypeScript types (UserInfo, AgentDelegation,
                       AgentDetail, service, grant, scope, and permission-set types
    approval.ts         Tool-approval request and response types

  utils/
    validation.ts      Input validation (URL safety, etc.)
    scrollToError.ts   Scroll-to-first-error UX helper
```

## Path Aliases

Vite and Vitest define these aliases. In Storybook, define only the aliases that it uses.

| Alias            | Path                  |
| ---------------- | --------------------- |
| `@design-system` | `./src/design-system` |
| `@components`    | `./src/components`    |
| `@hooks`         | `./src/hooks`         |
| `@services`      | `./src/services`      |
| `@types`         | `./src/types`         |
| `@utils`         | `./src/utils`         |
| `@assets`        | `./src/assets`        |
| `@styles`        | `./src/styles`        |

Use these aliases in imports. Do not use relative imports across alias boundaries.

## Design System Rules

Use `src/design-system/` as the **single source of truth** for visual decisions.
Before you write a styled component, read `src/design-system/docs/COMMON_MISTAKES.md`.

### Critical Rules

1. **Semantic colors only** — Use semantic tokens. Do not use raw gray tokens.
2. **Typography** — Use `font-display`, `font-sans`, and `font-mono` for headings, body text, and code.
3. **Elevation** — Use shadows for cards. Use borders only for containment.
4. **Animation** — Use 150ms for hover. Use 200ms for state changes. Use 300ms for modals. Use 500ms for pages. Respect reduced motion.
5. **WCAG 2.1 AA** — Give interactive elements visible focus. Text and UI colors must meet AA contrast.
6. **Component composition** — Use design-system components before ad-hoc components. Read `src/design-system/docs/DECISION_TREES.md`.

### Semantic Color Palette

| Token Family | Hex (primary)                            | Use                                 |
| ------------ | ---------------------------------------- | ----------------------------------- |
| `trust-*`    | #0A2540 (deep), #1E4D6B, #E8F1F5 (light) | Primary brand, headings, actions    |
| `cta-*`      | #D97706                                  | Call-to-action buttons and links    |
| `success-*`  | #059669                                  | Granted permissions, success states |
| `error-*`    | #DC2626                                  | Error states, destructive actions   |
| `warning-*`  | #D97706                                  | Warnings, attention signals         |
| `neutral-*`  | #faf9f7 → #1a1a1a (50–900 scale)         | Backgrounds, body text, borders     |

## API Client Pattern

- `services/api/client.ts` exports the configured Axios instance (`apiClient`).
- The base URL is `/api`. In development, Vite forwards requests to Go. In production, use the upstream proxy.
- **Authentication is external** — The Vite proxy adds `X-Remote-User` in development. An upstream proxy handles production authentication.
- `ConsentApiService`, `SessionsApiService`, and `approvalApi` provide typed `apiClient` methods.
- GET responses use the in-memory `apiCache`. Invalidate relevant cache entries after mutations.
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
| GET    | `/api/third-party/sessions/:service`   | `sessionsApi.getServiceSessionDetail(service)` |
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
```

Vite uses port 3000. Vite forwards non-frontend requests to the Go server. Vite adds `X-Remote-User` in development. Set `VITE_USE_POLLING=true` for Docker.

### Storybook

```bash
cd web && npm run storybook     # Port 6006
```

Storybook renders `src/design-system/` stories and MDX. Do not add application components.

## Testing Conventions

- **Framework**: Vitest, React Testing Library, and jsdom.
- **Setup**: In `vitest.setup.ts`, add jest-dom matchers. Call `cleanup()` after each test.
- **File names**: `*.test.ts`, `*.test.tsx`, `*.interactive.test.tsx`, and `*.integration.test.tsx`.
- **Testing approach**: Test user-visible behavior. Use role, text, or label queries. If no semantic query exists, use `getByTestId`.
- **Mocking**: Mock API services at the module level. Do not mock React hooks. Mock their data sources.
- **Accessibility**: The linter enforces `jsx-a11y`. Components must support keyboard use and correct ARIA attributes.

## Architecture Boundary

The frontend uses only HTTP endpoints in `../api/enduser/openapi.yaml`.
It has no Go imports or shared backend types. Keep TypeScript API types in sync with the contract.

## Code Splitting

Page components load with `React.lazy()` inside `<Suspense>`. Load route bundles during navigation.
