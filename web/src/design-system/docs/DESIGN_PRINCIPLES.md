# Design Principles — Consent UI v2

## Status and authority

This guide defines the target design for [feature 046](../../../../specs/046-redesign-consent-console/spec.md). [ADR 037](../../../../adrs/037-design-system-rebuilt-on-shadcn-radix.md) is proposed. Its acceptance is required before implementation.

The existing runtime still uses the previous design. This guide does not claim that the migration is complete. Other design guides describe legacy components until their phase-3 update. For v2 visual choices, use this guide and [COLOR_GUIDE.md](COLOR_GUIDE.md).

## A focused security tool

A decision view explains who requests access, what access means, and what happens next. A console view helps users inspect and change existing access.

Use neutral surfaces, thin borders, compact spacing, and a sans-serif hierarchy. Remove cream gradients, editorial serif headings, decorative textures, strong card shadows, and page-entry animation.

Each view has at most one accent-colored primary action. Consent uses Allow. Tool review uses Approve once. Deny and remembered approval remain secondary. Destructive confirmation appears only after an explicit request.

Required permissions remain locked. Optional choices remain explicit. Color, animation, or a browser default must never imply authorization.

## Two shared shells

| Shell | Responsibility |
| --- | --- |
| ConsoleShell | Collapsible sidebar, local wordmark, search, navigation, pending count, user menu, and page header |
| DecisionShell | Centered column with a 640 px maximum, local wordmark, focused content, and small footer |

Both shells belong in `web/src/design-system/components/layout/`. The mobile console uses Sheet. The collapsed sidebar uses a compact local mark with an accessible name.

The five existing route addresses remain unchanged. An authorization session selects the decision context on `/agents/:id`. Approval review always uses DecisionShell. The activity and settings routes use ConsoleShell.

At 320 px and 200% zoom, the page must not scroll horizontally. Tables use a responsive row layout without removing actions or permission details. Desktop agent lists must fit 12 rows in a 1080 px-high viewport.

## Typography and local brand

| Token | Family | Use |
| --- | --- | --- |
| `font-display` | Zalando Sans Variable | Titles, decision-view names, empty-state headings, and statistics |
| `font-sans` | Inter Variable | Body text, controls, and navigation |
| `font-mono` | JetBrains Mono Variable | Identifiers, scopes, arguments, and timestamps |

Use normal-width Zalando Sans. Do not add a second display-width download. Self-host subsetted WOFF2 files and licenses in `web/public/fonts/`. Keep fallback fonts and `font-display: swap`.

Preload the display and body subsets on decision routes. Load the mono face only when needed. Validate the real compressed route budget instead of assuming that variable fonts are smaller.

The black and white supplied wordmarks move into `web/public/brand/` after text-to-path conversion. Remove SVG font imports. Use the matching local favicon. The compact mark derives from the favicon, not a new shield.

No page loads third-party fonts, scripts, or images automatically. Unknown external logos use a local fallback. User-directed external links and OAuth2 navigation remain available.

## Color and themes

[COLOR_GUIDE.md](COLOR_GUIDE.md) owns the semantic color contract. Tokens live under `web/src/design-system/tokens/` and map through Tailwind 4 `@theme inline`.

Neutral blue is the sole primary accent. Status colors communicate success, warning, error, information, and authoritative risk. A risk color always has a label and explanation. Unknown risk uses “Risk not rated”.

Light, dark, and system choices persist per browser. The first-paint script and React theme provider use the same storage key and precedence. System mode reacts to later OS changes. Explicit light or dark mode does not.

## Owned components

Use the Radix versions of shadcn/ui components. Keep their source in the existing design-system categories. Keep CVA for variants and the existing `cn()` utility with `tailwind-merge`.

The foundation contains Button, Badge, Card, Dialog, DropdownMenu, Popover, Switch, Tabs, Tooltip, Sonner, Table, Command, Input, Select, Separator, Skeleton, and Sheet. Retained selection controls and disclosure controls follow the same tokens and accessibility rules.

Use Lucide icons with text labels. Decorative icons are hidden from assistive technology. Icon-only controls have an accessible name.

TanStack Table owns table state, not markup or authorization. cmdk owns command matching and keyboard interaction. The palette only searches records available to the acting user.

## Interaction and motion

Use 120 ms for hover feedback, 160 ms for control changes, and 200 ms for overlays. All transitions use ease-out. Use `@starting-style` only as progressive visual enhancement.

Reduced motion removes movement and delay. Do not animate full-page entry or approval decisions with shared-layout motion. Remove Framer Motion during phase 3.

Dialog and Sheet trap focus, close through an available control, and restore focus to the trigger. Portaled content inherits the root theme. Tooltips supplement visible labels and support keyboard focus.

## Truthful state

A domain-verified CIMD badge does not verify a publisher's legal identity. Missing publisher metadata uses “Publisher not provided”. Localhost risks remain prominent.

A connection is not a grant. Its state reflects token usability and required scope coverage, not the provider's optional scope catalogue.

Missing last-use data shows “Not recorded”. Activity history can contain gaps and starts at deployment. Do not replace missing last-use data with creation or modification times.

Revocation requires confirmation. An optimistic pending state is not a server success. Failure restores the row and announces an error. Approval and grant creation wait for the server result.

## Accessibility and verification

The feature targets WCAG 2.2 AA. All text uses at least 4.5:1 contrast. Controls and focus indicators use at least 3:1 against adjacent surfaces.

Every action has a keyboard path. Focus remains visible and unobscured by sticky bars. Target sizes meet WCAG 2.2 AA, with larger touch targets where space permits.

New approvals, decision results, and toasts use status announcements without moving focus. Accessible expansions expose long text and exact scopes. React escapes untrusted text.

Every component story runs in light and dark themes. Storybook a11y failures block CI. Browser journeys cover focus, zoom, reduced motion, and network privacy beyond automated axe checks.

Keep user-facing copy in one module for later localization. Use action-first wording and preserve technical identifiers exactly.

## Migration rule

Phase 1 establishes this system. Phase 2 moves decision surfaces behind broker configuration `ui.v2`. Phase 3 moves console surfaces and removes old presentation and configuration. Phase 4 adds activity, settings, command search, and final screenshots.

Do not create permanent compatibility aliases or a second design-system directory. Update all component callers and detailed guides at cutover. See the [plan](../../../../specs/046-redesign-consent-console/plan.md) for acceptance gates.
