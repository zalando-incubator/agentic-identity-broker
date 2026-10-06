---
applyTo: "web/**"
---

# Consent Frontend — React SPA

Full reference: `web/AGENTS.md`. Feature contract: `specs/047-redesign-consent-console/`.

## Tech Stack

React 19 + TypeScript 5 + Vite 7 + Tailwind CSS 4 + owned shadcn/Radix components + React Router 7 + Axios + TanStack Query 5 + Lucide + cmdk + Sonner + Vitest 4 + React Testing Library 16 + Storybook 10. Motion is permitted on console routes only.

## Source Structure

```
src/
  App.tsx              Router: / → /agents, /agents, /agents/:agentId, /connections, /approvals,
                       /approvals/remembered, /approvals/:id, /settings/appearance, * → error page
  components/          consent/ (PermissionPanel, DurationSelect), approvals/, sessions/, icons/, layout/, command/
  copy/                User-facing copy catalogue (@copy)
  design-system/       ★ AUTHORITATIVE design reference — read docs/ before styling
    components/        primitives, inputs, data-display, layout, navigation, overlays, feedback, advanced
    tokens/theme.css   Semantic light/dark OKLCH tokens
    theme/             Theme and collection-view preferences, first-paint script
    docs/              INDEX.md, DESIGN_PRINCIPLES.md, COLOR_GUIDE.md, COMMON_MISTAKES.md, etc.
  hooks/               Principal-scoped TanStack Query reads and server-confirmed mutations
  pages/               Lazy route containers and pure *View components
  services/api/        Typed Axios clients; services/query/ owns the identity boundary
  storybook/           Shared story fixtures
  types/               API request and response types
```

`/agents/:agentId` shows the consent decision when `session_token` is present and the console agent detail otherwise.

## Path Aliases (use these, not relative paths)

`@design-system`, `@copy`, `@components`, `@hooks`, `@services`, `@app-types`, `@utils`, `@assets`, `@styles`

## Design System Rules (Critical)

1. **Semantic tokens only** — no raw palette utilities or component-local colors. Use `--border-subtle` for cards and dividers, `--border` for buttons and popovers, and `--border-control` for form controls only.
2. **Typography** — Zalando Sans for display text, Inter for body text and tabular dates/counts, JetBrains Mono for identifiers.
3. **Collections** — EntityCard grids with a list toggle for Agents and Connections; fixed-height EntityRow items and a separate detail panel for Approvals. No control expands a row or card in place.
4. **Actions** — one accent action per view. Use `destructive-outline` for Revoke and Disconnect. Confirm revocation before the request.
5. **Motion** — 120/160/200 ms feedback, 320 ms emphasis, `cubic-bezier(0.2, 0, 0, 1)`. Motion and animated icons on console routes only; decision routes use CSS/SVG. Reduced motion removes movement but keeps opacity and color fades.
6. **Accessibility** — WCAG 2.2 AA target in both themes, visible focus, keyboard paths, and announcements.
7. **Copy** — application components take strings from `@copy`; design-system components receive strings through props.

Read `design-system/docs/COMMON_MISTAKES.md` before writing any styled component.

## API Client Pattern

- Base URL `/api` — Vite proxy → Go backend in dev, upstream proxy in prod
- Auth is external (the upstream proxy injects `X-Remote-User`)
- TanStack Query owns server state over Axios; query keys start with the acting principal. There is no separate API cache
- The browser polls `/api/approvals/pending` every 10 seconds and never calls the gateway-wide `GET /api/approvals`
- localStorage holds only appearance and collection-view preferences, never API responses or authorization context

## Testing Conventions

- **Framework**: Vitest + React Testing Library + jsdom; Storybook browser projects for light/dark accessibility and visual checks
- **Files**: Co-located `*.test.ts(x)`, `*.interactive.test.tsx`, `*.integration.test.tsx`
- **Approach**: Test user-visible behavior. Use role, text, or label queries; use `getByTestId` only without a semantic query
- **Mocking**: Mock API services at module level. Never mock hooks directly
- **Stories**: Render pure `*View` components with shared fixtures, without network mocks
- **A11y**: `eslint-plugin-jsx-a11y` enforced. Every interactive component keyboard-navigable

## Architecture Boundary

Frontend communicates with backend **exclusively through HTTP API endpoints** in `/api/enduser/openapi.yaml`. No Go imports, no shared types. TypeScript types in `src/types/` mirror OpenAPI schemas — keep in sync.
