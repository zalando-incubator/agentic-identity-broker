# ADR 037: Design system rebuilt on shadcn/Radix

**Status**: Proposed
**Date**: 2026-09-25
**Feature**: [046-redesign-consent-console](../specs/046-redesign-consent-console/spec.md)
**Supersedes on acceptance**: ADR 006's UI Components clause, animation choice, and custom-hook server-state ownership. Its client UI-state guidance remains unchanged.
**Extends on acceptance**: ADR 035's browser route list with `/activity` and `/settings`. Root mounting and protocol precedence remain unchanged.

## Context

The existing UI uses Headless UI, Framer Motion, and an editorial visual style. Consent UI v2 needs focused decisions and a compact console.

The user requested this replacement during planning on 2026-09-25. This request selects the technical direction, not acceptance of this proposed ADR or detailed API schemas.

ADR 006 also assigns server-state management to custom hooks. TanStack Query requires an explicit, narrow exception to that decision. Axios remains the transport.

## Decision

### Retained platform

Keep React 19, TypeScript, Vite 7, Tailwind 4, React Router 7, route-level lazy loading, Axios, and `X-Remote-User` authentication boundaries. Keep CVA, `tailwind-merge`, Vitest, Testing Library, Storybook 10, and Ginkgo with Playwright.

### Owned components

Copy the Radix versions of shadcn/ui components into `web/src/design-system/components/`. Keep the existing category directories, aliases, and `cn()` utility. Do not create a competing `components/ui` library.

The inventory includes Button, Badge, Card, Dialog, DropdownMenu, Popover, Switch, Tabs, Tooltip, Sonner, Table, Command, Input, Select, Separator, Skeleton, and Sheet. Migrate other retained controls, including Checkbox, Radio, Accordion, and date input, to the same system. Command uses cmdk. Icons use Lucide. Tables use TanStack Table with owned Table presentation components.

shadcn source is application-owned code. Pin package versions and review updates as source changes. Do not run an unreviewed registry update over customized components.

### Visual system

Use semantic OKLCH tokens with explicit light and dark values. The accent is neutral blue. Status colors communicate state, not competing primary actions.

Use self-hosted variable Zalando Sans for display text, Inter for body text, and JetBrains Mono for technical values. Use normal width, not an additional SemiExpanded font. Copy subsetted WOFF2 files and licenses into `web/public/fonts/`. Remove Crimson Pro and Manrope at final cutover.

Preserve the supplied black and white wordmark artwork. Convert its text to paths during the asset build and remove remote font references. Move both wordmarks and `favicon.svg` into `web/public/brand/`. Derive the collapsed-sidebar mark from the local favicon artwork. An accessible name remains HTML, not an SVG font dependency.

Use CSS transitions and `@starting-style` for optional entry feedback, with 120–200 ms ease-out timing. Reduced motion removes movement. Remove Framer Motion. No shared-layout exception is needed.

### Themes and layout

A small inline script sets the resolved `data-theme` before first paint. A CSP hash authorizes this fixed script without `unsafe-inline`. Add color-scheme metadata and theme form controls. CSS uses system appearance only when no explicit resolved theme exists. Explicit browser preferences override system appearance.

ConsoleShell contains the sidebar and page header. Sheet contains the mobile sidebar. DecisionShell has a centered column no wider than 640 px, a wordmark, and a footer. Both shells live under `design-system/components/layout/`.

Preserve the five existing route addresses. `/agents/:id` uses DecisionShell for an authorization session and ConsoleShell otherwise. `/approvals/:id` always uses DecisionShell. Add `/activity` and `/settings` as lazy console routes.

### Data and authorization

TanStack Query owns remote data caching over the existing Axios services. React state and context still own form state and browser preferences. Remove the old GET cache after its callers migrate.

Optimistic revocation follows explicit confirmation. Show a pending result, restore data on failure, and reconcile with the server. Do not optimistically grant or approve access.

ADR 014's synchronization endpoint remains gateway-only. ADR 018 and feature API-004 prohibit browser use. A shared query refreshes `/api/approvals/pending` every 10 seconds while the console is visible. Sidebar and queue share that result. Resume refresh immediately on focus. The 15-second target assumes a reachable server and a foreground browser.

The only additive API scope is user activity plus one minimal session required-scopes field if necessary. API contracts need stakeholder review before implementation. No new runtime-configuration endpoint is permitted.

### Rollout and verification

The temporary broker configuration `ui.v2` selects old or new presentation during phases 2–3. It uses the existing configuration port, environment and CLI bindings, startup validation, and Helm contract. Server HTML carries only the boolean. It never contains a principal, credential, or full configuration.

Phase 1 supplies foundations. Phase 2 migrates decision views behind the flag. Phase 3 migrates console views and removes the flag and obsolete visual layer. Phase 4 adds activity, settings, command search, screenshots, and final documentation.

Every component story runs in both themes with Storybook accessibility failures blocking CI. Playwright visual comparisons cover the five existing routes in both themes, including both agent contexts. Ginkgo acceptance journeys preserve authorization behavior. WCAG 2.2 AA is the feature target.

## Consequences

The team owns component source, theme integration, and dependency updates. Radix does not remove the need for keyboard, focus, contrast, and screen-reader validation.

The migration temporarily carries two presentations, not two authentication paths. Phase 3 removes old primitives, fonts, animations, theme, cache, flag bindings, and deployment documentation.

Table, Command, and console-only dependencies must stay outside the initial decision-route bundle. The consent route retains the under-150-kB compressed-code target.

A proposed ADR does not authorize implementation against accepted ADRs. Acceptance of this ADR and detailed additive API contracts is an implementation gate.

## Alternatives considered

- Retaining Headless UI does not satisfy the requested owned shadcn/Radix component direction.
- A second component library leaves conflicting variants and token rules. Existing directories remain the integration point.
- Live HTML wordmark text removes font imports but changes supplied artwork metrics. Outlined artwork preserves the brand assets.
- Zalando orange competes with warning states. Neutral blue separates primary action from warning meaning.
- A browser gateway long-poll violates the existing authentication boundary. User-scoped polling meets the spec without a third API.
- A permanent migration flag leaves two systems to maintain. The flag ends in phase 3.

## References

- [ADR 006](006-frontend-stack.md)
- [ADR 014](014-long-poll-listen-notify.md)
- [ADR 018](018-approval-endpoint-auth-boundaries.md)
- [ADR 035](035-root-mounted-spa.md)
- [Implementation plan](../specs/046-redesign-consent-console/plan.md)
- [Design principles](../web/src/design-system/docs/DESIGN_PRINCIPLES.md)
- [Color guide](../web/src/design-system/docs/COLOR_GUIDE.md)
