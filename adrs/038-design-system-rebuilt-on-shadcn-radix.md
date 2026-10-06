# ADR 038: Design system rebuilt on shadcn/Radix

**Status**: Accepted
**Date**: 2026-09-27
**Decision reference**: The stakeholder selected "Accept ADR 037" in the `/speckit-implement` approval prompt on 2026-09-27. PR review on 2026-10-05 renumbered this decision to ADR 038.
**Feature**: [047-redesign-consent-console](../specs/047-redesign-consent-console/spec.md)
**Partially supersedes**: ADR 006's UI Components clause, animation choice, and custom-hook server-state ownership. Its client UI-state guidance remains unchanged.
**Extends**: ADR 035's browser route list with `/agents`, `/connections`, `/approvals/remembered`, and `/settings/appearance`. Root mounting and protocol precedence remain unchanged.
**Partially superseded by**: [ADR 040](040-canonical-light-visual-reference-gate.md) replaces only the visual-reference matrix. Both-theme rendering and accessibility requirements remain unchanged.

## Context

The existing UI uses Headless UI, Framer Motion, and an editorial visual style. Consent UI v2 needs focused decisions and a compact console.

The user's 2026-09-25 request selected the technical direction. The stakeholder accepted this ADR on 2026-09-27.

The user's 2026-09-26 revision required one complete cutover, without implementation phases, feature flags, or backwards compatibility.
Principle XI sets design-system and accessibility requirements without selecting an aesthetic.
Before ADR acceptance, `DESIGN_PRINCIPLES.md` retained its earlier direction. The accepted ADR governs current guidance.

ADR 006 also assigns server-state management to custom hooks. TanStack Query requires an explicit, narrow exception to that decision. Axios remains the transport.

## Decision

### Retained platform

Keep React 19, TypeScript, Vite 7, Tailwind 4, React Router 7, route-level lazy loading, Axios, and `X-Remote-User` authentication boundaries. Keep CVA, `tailwind-merge`, Vitest, Testing Library, Storybook 10, and Ginkgo with Playwright.

### Owned components

Copy the Radix versions of shadcn/ui components into `web/src/design-system/components/`. Keep the existing category directories, aliases, and `cn()` utility. Do not create a competing `components/ui` library.

The owned primitive inventory includes Button, Badge, Card, Dialog, DropdownMenu, Popover, Switch, Tabs, Tooltip, Sonner, Command, Input, Select, Separator, Skeleton, and Sheet. Agents, Connections, and Approvals use cards or stable rows rather than data tables. Remove the orphaned Table primitive and TanStack Table dependency with their final consumers. Migrate Checkbox, Radio, Accordion, and date input into the same system. Command uses cmdk. Icons use Lucide.

shadcn source is application-owned code. Pin package versions and review updates as source changes. Do not run an unreviewed registry update over customized components.

### Visual system

Use tinted neutral OKLCH tokens with explicit light and dark values. Blue remains the single primary accent; soft status colors communicate state and risk. Separate `--border-subtle` for dividers and card edges, `--border` for buttons and popovers, and `--border-control` for form controls. The latter meets the 3:1 non-text control contrast target; quiet card borders do not carry that role. Add `--primary-soft` for selected cards and navigation. Validate rendered contrast in both themes instead of treating proposed token calculations as validation results.

Use self-hosted variable Zalando Sans for display text, Inter for body text, and JetBrains Mono for technical values. Use normal width, not an additional SemiExpanded font. Copy subsetted WOFF2 files and licenses into `web/public/fonts/`. Remove Crimson Pro and Manrope at final cutover.

Preserve the supplied black and white wordmark artwork. Convert its text to paths during the asset build and remove remote font references. Move both wordmarks and `favicon.svg` into `web/public/brand/`. Crop the local wordmark and compact mark to their visible ink. An accessible name remains HTML, not an SVG font dependency.

Use 120, 160, and 200 ms feedback tokens with `cubic-bezier(0.2, 0, 0, 1)` easing and up to 320 ms for emphasis. Console routes may use owned lucide-animated icon source and Motion for layout exits and list reflow. Wrap the console in `MotionConfig reducedMotion="user"`. Decision routes use only owned CSS/SVG animation and must never import Motion or console animation modules. Reduced motion removes movement and icon drawing but keeps useful opacity and color feedback. The bundle gate must enforce this route boundary; remove the old Framer Motion dependency.

### Themes and layout

A small inline script sets the resolved `data-theme` before first paint. A CSP hash authorizes this fixed script without `unsafe-inline`. Add color-scheme metadata and theme form controls. CSS uses system appearance only when no explicit resolved theme exists. Explicit browser preferences override system appearance.

ConsoleShell contains the quiet three-item sidebar (Agents, Connections, Approvals), a user-menu entry for Settings, and an optional-purpose page header. Console content uses a centered 1120 px container with a 12-column grid. DecisionShell has no sidebar; the consent card is at most 480 px wide with a pinned decision footer. Both shells live under `design-system/components/layout/`.

Use `/agents`, `/agents/:id`, `/connections`, `/approvals`, `/approvals/remembered`, `/approvals/:id`, and `/settings/appearance`. `/` replaces history with `/agents`. Remove the old `/delegations`, `/sessions`, and `/settings` view routes without aliases or redirects. `/agents/:id` uses DecisionShell for an authorization session and ConsoleShell otherwise. `/approvals/:id` always uses DecisionShell. Update the connection authorization callback return URL with the route cutover; do not change the provider-facing callback protocol.

### Data and authorization

TanStack Query owns remote data caching over the existing Axios services. React state and context still own form state and browser preferences. Remove the old GET cache after its callers migrate.

Optimistic revocation follows explicit confirmation. Show a pending result, restore data on failure, and reconcile with the server. Do not optimistically grant or approve access.

ADR 014's synchronization endpoint remains gateway-only. ADR 018 and feature API-004 prohibit browser use. A shared query refreshes `/api/approvals/pending` every 10 seconds while the console is visible. Sidebar and queue share that result. Resume refresh immediately on focus. The 15-second target assumes a reachable server and a foreground browser.

This decision adds or changes no API contract, response field, or persistence. No new runtime-configuration endpoint is permitted.

### Single cutover and verification

Deliver every route, both shells, settings, and command search in one release.
Do not add feature flags, broker rollout configuration, HTML flag delivery, parallel presentations, or backwards-compatibility layers.
Migrate all consumers and remove obsolete code in the same change.

After ADR acceptance, update all current guidance in the plan's inventory for the approved direction.
Preserve historical feature decisions. Replace only references that incorrectly present old rules as current guidance.

**Historical matrix, superseded by [ADR 040](040-canonical-light-visual-reference-gate.md) on 2026-10-06**: Every component story runs in both themes with Storybook accessibility and visual-regression failures blocking CI. Playwright visual comparisons cover all seven destination routes in both themes, including both agent contexts (16 images). Ginkgo acceptance journeys preserve authorization behavior. WCAG 2.2 AA is the feature target.

### Approved amendment: 2026-10-04

The stakeholder approved all five choices in the [Consent Console UX improvement plan](../specs/047-redesign-consent-console/ux-improvement-plan.md) on 2026-10-04:

1. Agents and Connections show cards by default, with a list view through the toggle.
2. Motion is allowed on console routes. Decision routes stay CSS-only.
3. On consent, the primary button becomes "Connect {Service} to continue" when a connection is missing.
4. `/delegations` and `/sessions` are renamed to `/agents` and `/connections`.
5. The consent screen shows "Signed in as".

This amendment replaces only the earlier presentation and motion choices. The 2026-09-27 acceptance, platform, API, authorization, principal-query, and callback-state decisions remain in force. A blocked consent decision has one primary Connect action until the required service returns; then Allow becomes primary. Consent has one 480 px card with identity, a permission panel, and a pinned footer. Agents and Connections use an entity card grid with a list toggle; Approvals uses fixed-height inbox rows with a separate decision panel and a Remembered route. Agent detail uses an 8 + 4 layout without tabs. Settings opens through the user menu at `/settings/appearance`. Compose page stories from pure views and shared fixtures, with no network mock or decision-route Motion import. Implement and validate these changes before claiming runtime completion.

### Approved visual amendment: 2026-10-05

The stakeholder requested implementation of the visual review on 2026-10-05. Keep ordinary cards flat. Add semantic soft overlay and lighter focal shadows for floating layers, the consent card, and the sticky save bar. Consent may use a static top accent wash at 3–4% opacity, a 1 px ring, and a CSS-only agent-to-user connector; this is the sole decorative-gradient exception. Put “Signed in as” inside the consent card. The stakeholder subsequently withdrew provider marks; service identities retain tinted initials and no provider-mark registry is shipped. Empty states use an 80 px primary-soft tile with a 40 px icon that plays once on entry and on hover. Separate the light sidebar with the muted surface; in dark mode retain the page surface on the sidebar and use the card surface for console content. Console approval selection may cross-fade at 120 ms, move its accent bar with shared layout, and bump the pending count on a new request; reduced motion removes movement. Use relative recency dates, absolute expiry dates, and monospace tool names and scope patterns. API, authorization, callback, and decision-bundle boundaries remain unchanged.

### Approved agent presentation refinement: 2026-10-05

The stakeholder requested a follow-up review of agent detail and overview presentation. Compact agent cards remain 120 px tall, with consistent 12 px insets, a more prominent name, and smaller expiry and recency metadata. Add an explicit Details link alongside confirmed Revoke, outside the linked card surface. Shared enabled buttons use the pointer cursor. Required console permission groups keep one lock cue and the Required label. Connection status badges in the detail rail provide explanatory hover and keyboard-focus tooltips. A separate technical-details panel shows supplied agent/client identifiers and registered client URIs; human-facing About content stays separate. Overview content remains limited to the existing summary response, without permission-count claims or per-agent enrichment requests. Authorization, consent, callbacks, provider-image policy, and API contracts remain unchanged.

### Approved console and consent polish: 2026-10-05

The stakeholder requested further corrections to connection duplication, notification affordances, collection preferences, approval tab layout, search focus, connection-card density, consent branding, and the favicon. On agent details, Connections owns each service identity, status, and Connect/Reconnect action; permission rows show service names and selection without duplicate tiles, warnings, or connection buttons. Consent warnings remain in normal layout rather than clipped avatar overlays. Console headers make no registrant claim from absent client URIs. Notification close controls use pointer cursors and visible hover feedback.

An explicit global collection-view choice applies immediately to Agents and Connections, clearing their earlier page choices. Later page toggles remain local. Precedence is page choice, explicit global choice, then the initial density fallback. Search collapses when focus leaves its input/clear-control area while preserving and indicating an active query. Remembered-only outcome filters stay below the shared approval tabs; header/tab positions stay fixed and content enters with a short opacity fade. Noncompact connection cards use a 160 px minimum and grow for real explanations or wrapped metadata; compact agent cards and fixed approval rows keep their earlier contracts.

This refinement replaces the earlier consent relationship motif and in-card account placement: the full local wordmark sits above the focused card, the agent icon and request text sit horizontally together, and “Signed in as” is a separate line beneath the card. Remove the Powered by footer. Without CIMD origin metadata, use the neutral “Registered agent” fallback, not a registrant claim. The favicon may use a square, high-contrast single-letter outline for small sizes; the supplied full wordmarks and sidebar compact artwork stay unchanged. These presentation changes alter no authorization, principal, API, callback, or provider-imagery contract.

### Approved review corrections: 2026-10-05

The stakeholder requested a simpler agent editor: one group checkbox, informational required services, and visible pressed-state choices for independent optional services. Console permission descriptions and service names wrap without a hiding toggle. The save bar moves immediately below the header and stays visible during scrolling. Duration edits compare saved access with access after saving, and the custom-date control retains its chosen date. Unsaved edits require explicit discard on internal navigation and a browser warning on document departure, except for deliberate draft-preserving provider navigation. Grant calendar dates are consistently local; unrelated permission edits preserve the exact saved timestamp.

The stakeholder also removed the duplicate Details action from already-linked agent cards. This replaces the earlier explicit Details-link requirement without changing card navigation or confirmed Revoke. Connection states update locally at known refresh-token expiry deadlines. Single-letter approval decisions require focus within the inbox/review surface, not merely a selected request. These corrections change no API, authorization, principal, or callback contract.


## Consequences

The team owns component source, theme integration, and dependency updates. Radix does not remove the need for keyboard, focus, contrast, and screen-reader validation.

The cutover removes old primitives, fonts, animations, tokens, wrappers, and caches. No compatibility aliases or older-server fallbacks remain.

Console-only Motion, Command, and list dependencies must stay outside the initial decision-route bundle. The 2026-09-28 acceptance set a 170-kB gzip limit and five-second cold-consent target. On 2026-10-05, the stakeholder approved a 190-kB limit to retain React Router's standard unsaved-navigation blocker. The five-second target and decision-module isolation remain unchanged.

The stakeholder accepted this ADR on 2026-09-27 and approved the amendment on 2026-10-04. Neither decision establishes implementation or validation completion.

## Alternatives considered

- Retaining Headless UI does not satisfy the requested owned shadcn/Radix component direction.
- A second component library leaves conflicting variants and token rules. Existing directories remain the integration point.
- Live HTML wordmark text removes font imports but changes supplied artwork metrics. Outlined artwork preserves the brand assets.
- Zalando orange competes with warning states. Neutral blue separates primary action from warning meaning.
- A browser gateway long-poll violates the existing authentication boundary. User-scoped polling meets the spec without a new API.
- A staged rollout or temporary flag creates two presentations. The requested release contains only the new presentation.

## References

- [ADR 006](006-frontend-stack.md)
- [ADR 014](014-long-poll-listen-notify.md)
- [ADR 018](018-approval-endpoint-auth-boundaries.md)
- [ADR 035](035-root-mounted-spa.md)
- [ADR 040: Canonical light visual-reference gate](040-canonical-light-visual-reference-gate.md)
- [Implementation plan](../specs/047-redesign-consent-console/plan.md)
- [Design principles](../web/src/design-system/docs/DESIGN_PRINCIPLES.md)
- [Color guide](../web/src/design-system/docs/COLOR_GUIDE.md)
