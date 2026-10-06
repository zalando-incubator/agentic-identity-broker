# Design Token Guide

## Authority and delivery status

[ADR 038](../../../../adrs/038-design-system-rebuilt-on-shadcn-radix.md) became Accepted on 2026-09-27 and its UX amendment was approved on 2026-10-04.
This guide defines token targets for [feature 047](../../../../specs/047-redesign-consent-console/spec.md).
[DESIGN_PRINCIPLES.md](DESIGN_PRINCIPLES.md) defines the current visual direction.

The runtime defines these tokens. The feature [plan](../../../../specs/047-redesign-consent-console/plan.md) describes implementation and CI verification. The feature [tasks](../../../../specs/047-redesign-consent-console/tasks.md) record task status.

## One token source

Principle XI requires centralized semantic tokens for color, type, spacing, radius, motion, and theme.
Definitions belong under `web/src/design-system/tokens/`.
Component CVA variants select these tokens. They do not define another palette or visual system.

[COLOR_GUIDE.md](COLOR_GUIDE.md#semantic-oklch-contract) owns the exact light and dark color targets.
This guide defines their use, not a second palette. The target includes foreground, soft status and risk, selection, three borders, and focus roles.

Implement the targets in `tokens/theme.css` and map them through `@theme inline` in `web/src/styles/index.css`.
The runtime must define every role in both themes. Remove retired palette tokens rather than preserving compatibility aliases.

## Semantic color use

Choose a role by meaning:

| Meaning | Utility pair |
| --- | --- |
| Page content | `bg-background text-foreground`; console main uses `bg-console-background` |
| Contained content | `bg-card text-card-foreground border-border-subtle` |
| Raised content | `bg-popover text-popover-foreground border-border` |
| Supporting text | `text-muted-foreground` on a supported surface |
| Primary action | `bg-primary text-primary-foreground` |
| Active navigation and selection | `bg-primary-soft text-primary-soft-foreground` |
| Neutral hover | `bg-accent text-accent-foreground` |
| Input, checkbox, and radio | `border-border-control` |
| Button boundary | `border-border` |
| Card edge and divider | `border-border-subtle` |
| Focus indicator | `ring-ring` |
| Sidebar | `bg-sidebar text-sidebar-foreground` |

Headings use their surface's foreground, not the primary-action color.
Soft status and risk badges use their paired background and foreground with a dot or icon. No solid status badges remain.
Use the stronger `--border-control` only on form controls. It must meet 3:1 against their adjacent surface.

Application examples take their labels from `@copy`. Primitives receive strings through props or children.

```tsx
<section className="space-y-4 rounded-xl border border-border-subtle bg-card p-4 text-card-foreground">
  <h2 className="font-display text-xl font-semibold">{title}</h2>
  <p className="text-sm text-muted-foreground">{description}</p>
  {children}
</section>
```

This snippet illustrates tokens, not a second Card implementation or observed runtime behavior.

## Button variants

| Variant | Role |
| --- | --- |
| `primary` | The view's one accent action; consent Allow stays visible but disabled while selected connections are missing |
| `secondary` | Supporting action on a neutral surface |
| `outline` | Visible supporting action, including row actions and consent Deny |
| `ghost` | Tooltip-labeled icon control, never an invisible row action |
| `destructive-outline` | Resting Revoke or Disconnect, with confirmation before mutation |
| `destructive` | Filled destructive confirmation inside Dialog |

Consent keeps the Allow label. Disable it while selected services need connecting, show an explanation, and keep inline outline Connect actions available.
Tool review uses Approve once. Approval inbox decisions appear in the detail panel, not on expanding rows.
Use 32 px outline row actions and 40 px consent actions. Primary and destructive hover shift lightness by 0.04 and press scales to 0.98.
Do not underline a button on hover. Use semantic Button variants rather than per-page restyling.

## Typography

| Token | Family | Use |
| --- | --- | --- |
| `--font-display` / `font-display` | Zalando Sans Variable | Titles, decision names, empty-state headings |
| `--font-sans` / `font-sans` | Inter Variable | Body text, controls, navigation, dates, counts |
| `--font-mono` / `font-mono` | JetBrains Mono Variable | Tool names, identifiers, scope patterns, existing scopes, raw arguments |

Use normal-width Zalando Sans. Self-host the WOFF2 files and licenses in `web/public/fonts/`.
Preload only Zalando Sans and Inter latin subsets. Load the mono face only where needed.
Keep `font-display: swap` and system fallbacks. Do not load remote fonts.

| Size | Use |
| --- | --- |
| 12 px | Badge text and fine metadata |
| 13 px | Supporting metadata |
| 14 px | Console base, controls, entity titles |
| 16 px | Agent and connection-card names, consent headings, and long-form body where needed |
| 20 px | Semibold page titles |
| 24 px | Large decision heading only where it fits |

Set consent-card body text to 15 px. Use tabular Inter figures for dates and counts, not the mono face.
Cap running text at `70ch`. Keep technical identifiers exact even when the visible text truncates.
Use relative times for changed, connected, and requested metadata, with an absolute date in the tooltip. Use absolute dates for expiry.
Keep a logical heading order and 4.5:1 text contrast in both themes.

## Spacing, borders, and radius

Use the 4 px base with only 4, 8, 12, 16, 24, 32, and 48 px steps.
Use 4–12 px inside components, 16–24 px between components, and 32–48 px between sections.

| Element | Radius target |
| --- | --- |
| Badge | 6 px |
| Button, input, select, rounded-square avatar | 8 px |
| Card, alert, inset permission panel | 12 px |
| Dialog, consent card | 16 px |

People retain circular avatars. Agents and services use a rounded square, with a local fallback tint from eight deterministic ID-based hues.
Badge height is 20 px with 12 px medium text, a leading 6 px dot or 12 px icon, and 6 px horizontal padding.
Place at most one badge on the entity title baseline or in a fixed status slot. Do not give it a separate wrapping row.

Center console content at a maximum width of 1120 px. Use 24 px side padding, or 16 px below 768 px.
Within the container use a 12-column grid with 24 px gutters; detail views use an 8 + 4 split.
Card grids use `repeat(auto-fill, minmax(320px, 1fr))` and a 16 px gap.
Compact agent cards stay 120 px tall at supported unzoomed widths, with 12 px outer insets, 16 px semibold names, and 12 px expiry and relative change metadata. Details and Revoke are separate actions below the identity. At very narrow zoomed widths, preserve readable metadata and controls through reflow rather than clipping.
Consent uses a 480 px card and a pinned footer. Group the full wordmark above it and a quiet Signed in as line beneath it, outside the form. Below 640 px, the card is full bleed and reserves the branding/account space while keeping the decision actions visible.

Use a 1 px subtle border for card containment. Keep ordinary cards flat. Use `shadow-overlay` (`--elevation-overlay`) for floating menus, popovers, dialogs, sheets, tooltips, selects, and toasts; use `shadow-focal` (`--elevation-focal`) for the consent card and sticky save bar. Both tokens have explicit light and darker-shadow dark values. The consent card adds a 1 px ring; its page alone permits a static top accent wash at 3–4% opacity under ADR 038's 2026-10-05 amendment. Do not add other decorative gradients or page-entry animation.
At 320 px and 200% zoom, neither shell permits horizontal page scrolling. Target sizes and focus clearance take precedence over density.

## Motion tokens

[MOTION_GUIDE.md](MOTION_GUIDE.md) defines the motion behavior.

| Token | Value | Use |
| --- | --- | --- |
| `--motion-feedback` | `120ms` | Hover feedback |
| `--motion-control` | `160ms` | Controls and disclosure |
| `--motion-overlay` | `200ms` | Overlays |
| `--motion-emphasis` | `320ms` | Success and illustrative changes |
| `--motion-ease` | `cubic-bezier(0.2, 0, 0, 1)` | CSS and shared Motion transitions |

Use 75% of the entry duration for an exit. The shared `consoleMotion` constants module reads these values for console Motion transitions.
Console routes can use Motion for card and row exits. Decision routes use owned CSS animations only.
Under reduced motion, remove movement and icon animation but preserve opacity, color feedback, focus, and announcements.

## Themes and layering

The preference key is `aib.theme`, with `light`, `dark`, and `system` values.
Missing or invalid values resolve to `system`.
Explicit light or dark takes precedence over system appearance.
Only system mode follows later OS changes.
The collapsed sidebar uses `aib.sidebar-collapsed`. The default view uses `aib.collection-view-default`.
Per-page grid/list choices use `aib.collection-view.agents` and `aib.collection-view.connections`.

The first-paint script and provider must use the same preference contract.
CSS applies its dark fallback only to `:root:not([data-theme])`.
Native controls, scrollbars, and portaled overlays must use the resolved theme.
The light sidebar resolves `--sidebar` to `--muted`. Dark mode keeps the sidebar on `--background` and resolves `--console-background` to `--card`. Floating layers retain the lighter dark `--popover` surface.

Keep stacking order in shared components rather than per-page overrides.
Dialog and Sheet must keep their controls and focus indicators above inactive content.
Nested menus and popovers must remain usable within the active overlay.
Sticky draft controls must not hide focused content.

## Raw-palette ESLint rule

[COLOR_GUIDE.md](COLOR_GUIDE.md#raw-palette-enforcement) owns the detailed T046 rule contract.
`web/eslint-rules/no-raw-palette.js` must report errors for `src/**/*.{ts,tsx}` through the existing flat ESLint configuration.
`web-lint` and the CI web job must run it.

The rule rejects raw palette utilities and arbitrary literal colors.
It inspects JSX class strings, template literals, `cn()`, `clsx()`, and CVA base and variant values.
Variants, opacity suffixes, and important modifiers do not exempt a class.
Dynamic color-class construction cannot bypass the rule.
Semantic utilities, `currentColor`, and appropriate transparent presentation remain valid.

Do not add legacy exceptions or move raw values into inline styles to bypass enforcement.
Raw color definitions belong only in tokens and local brand artwork.
If a semantic role is missing, define it centrally before component use.

## Change and verification requirements

Before component work, read [DECISION_TREES.md](DECISION_TREES.md), [COMPONENT_PAIRING_GUIDE.md](COMPONENT_PAIRING_GUIDE.md), and [COMMON_MISTAKES.md](COMMON_MISTAKES.md).
Reuse the existing design-system categories, aliases, CVA, and `cn()` utility.
Do not create a competing component library.
Keep Command and Motion out of barrels imported by decision routes. Import their concrete modules only in console consumers.

Principle XI keeps WCAG 2.1 AA as the floor. Feature 047 targets WCAG 2.2 AA.
Measure text at 4.5:1 and required boundaries and focus indicators at 3:1 in both themes.
Every component story needs both-theme accessibility checks. Reviewed pixel references cover eligible light 1280 × 720 cases only, under [ADR 040](../../../../adrs/040-canonical-light-visual-reference-gate.md).
Both accessibility and visual-regression failures must block CI.

Token calculations do not prove rendered compliance.
Browser verification must cover focus, forced colors, zoom, reduced motion, and theme precedence.
Record results separately from the contract.
For a change to the visual direction, obtain ADR acceptance before implementation.
