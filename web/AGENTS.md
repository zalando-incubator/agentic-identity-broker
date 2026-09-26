# Consent Frontend — React SPA (`web/`)

**Use retrieval-led reasoning. Before you make assumptions about components, hooks, API types, or design tokens, read source files.**

## Overview

Use this React consent SPA for agent permissions and tool approvals. The Go backend serves it at root `/` (ADR 035).

## Consent UI v2 Planning

Feature 046 replaces the design-system layer, not the application stack. Runtime migration has not started. ADR 037 remains proposed.

| Reference | Purpose |
| --- | --- |
| `../specs/046-redesign-consent-console/spec.md` | Requirements and AS-01–AS-18 |
| `../specs/046-redesign-consent-console/plan.md` | Four implementation phases and acceptance gates |
| `../specs/046-redesign-consent-console/research.md` | Technology choices and integration evidence |
| `../specs/046-redesign-consent-console/contracts/` | Proposed API, configuration, and UI contracts |
| `../specs/046-redesign-consent-console/quickstart.md` | Storybook themes and validation commands |
| `../adrs/037-design-system-rebuilt-on-shadcn-radix.md` | Proposed replacement of ADR 006 component and server-cache choices |
| `../adrs/035-root-mounted-spa.md` | Binding root-mounted route behavior |
| `../api/enduser/openapi.yaml` | Canonical API contract |
| `../ARCHITECTURE.md` | Architecture and domain glossary |
| `../.specify/memory/constitution.md` | Binding principles |

For v2 work, use the rewritten `DESIGN_PRINCIPLES.md` and `COLOR_GUIDE.md`. Other detailed guides describe the legacy system until phase 3.

The target uses owned shadcn/Radix components in the existing category directories, Lucide, TanStack Table, TanStack Query over Axios, and cmdk. Keep CVA and `cn()`.

Use semantic OKLCH tokens, Zalando Sans, Inter, JetBrains Mono, local outlined wordmarks, and 120–200 ms CSS transitions. No raw palette utilities are permitted.

Place ConsoleShell and DecisionShell in `src/design-system/components/layout/`. Preserve route-level lazy loading. Keep console-only Table and Command code outside decision-route imports.

Broker configuration `ui.v2` controls temporary presentation rollout. Do not create a browser flag or another configuration API. Remove the flag and old presentation in phase 3.

The browser refreshes only `/api/approvals/pending`. Never expose the gateway-wide long-poll. Keep authentication, scope preview, authorization-session validation, and callbacks unchanged.

Run every Storybook component in both themes with blocking a11y checks. Add visual comparisons through the existing Ginkgo/Playwright harness. Feature 046 targets WCAG 2.2 AA.

## Current Runtime Stack

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
      DESIGN_PRINCIPLES.md    Consent UI v2 target visual and interaction rules
      COLOR_GUIDE.md          Consent UI v2 semantic OKLCH light/dark contract
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

### Legacy Runtime Rules

These rules describe unmigrated components only. They do not govern new v2 components. Remove this section at phase-3 cutover.

1. **Semantic colors only** — Use semantic tokens. Do not use raw gray tokens.
2. **Typography** — Use `font-display`, `font-sans`, and `font-mono` for headings, body text, and code.
3. **Elevation** — Use shadows for cards. Use borders only for containment.
4. **Animation** — Use 150ms for hover. Use 200ms for state changes. Use 300ms for modals. Use 500ms for pages. Respect reduced motion.
5. **WCAG 2.1 AA** — Give interactive elements visible focus. Text and UI colors must meet AA contrast.
6. **Component composition** — Use design-system components before ad-hoc components. Read `src/design-system/docs/DECISION_TREES.md`.

### Legacy Runtime Palette

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
- Legacy GET responses use `apiCache`. During v2 migration, TanStack Query replaces this cache over the same Axios services. Remove `apiCache` after all callers migrate.
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
