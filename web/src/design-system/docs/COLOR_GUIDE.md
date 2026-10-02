# Color Guide

## Authority and delivery status

[ADR 037](../../../../adrs/037-design-system-rebuilt-on-shadcn-radix.md) became Accepted on 2026-09-27.
This guide defines the approved color contract for [feature 047](../../../../specs/047-redesign-consent-console/spec.md).
[DESIGN_PRINCIPLES.md](DESIGN_PRINCIPLES.md) defines the current visual direction.

Acceptance establishes the contract, not runtime completion or accessibility results.
T054 implements these values in `web/src/design-system/tokens/theme.css`.
The feature [tasks](../../../../specs/047-redesign-consent-console/tasks.md) record implementation and verification separately.

## Semantic OKLCH contract

This table is the single authority for color names and light/dark values.
[TOKEN_GUIDE.md](TOKEN_GUIDE.md) defines their use without a second palette.
All values retain the approved palette and its specified role mappings.

| Token | Light | Dark | Purpose |
| --- | --- | --- | --- |
| `--background` | `oklch(0.985 0 0)` | `oklch(0.16 0 0)` | Page surface |
| `--foreground` | `oklch(0.20 0 0)` | `oklch(0.96 0 0)` | Main text |
| `--card` | `oklch(1 0 0)` | `oklch(0.21 0 0)` | Contained surface |
| `--card-foreground` | `oklch(0.20 0 0)` | `oklch(0.96 0 0)` | Card text |
| `--popover` | `oklch(1 0 0)` | `oklch(0.21 0 0)` | Floating surface |
| `--popover-foreground` | `oklch(0.20 0 0)` | `oklch(0.96 0 0)` | Floating content text |
| `--muted` | `oklch(0.95 0 0)` | `oklch(0.26 0 0)` | Secondary surface |
| `--muted-foreground` | `oklch(0.45 0 0)` | `oklch(0.78 0 0)` | Supporting text |
| `--primary` | `oklch(0.48 0.14 255)` | `oklch(0.78 0.10 250)` | Primary action and links |
| `--primary-foreground` | `oklch(1 0 0)` | `oklch(0.16 0 0)` | Primary action text |
| `--secondary` | `oklch(0.95 0 0)` | `oklch(0.26 0 0)` | Secondary action surface |
| `--secondary-foreground` | `oklch(0.20 0 0)` | `oklch(0.96 0 0)` | Secondary action text |
| `--accent` | `oklch(0.95 0 0)` | `oklch(0.26 0 0)` | Neutral hover and selection surface |
| `--accent-foreground` | `oklch(0.20 0 0)` | `oklch(0.96 0 0)` | Hover and selection text |
| `--border` | `oklch(0.62 0 0)` | `oklch(0.58 0 0)` | Required boundaries |
| `--border-soft` | `oklch(0.90 0 0)` | `oklch(0.32 0 0)` | Decorative separators only |
| `--input` | `oklch(0.62 0 0)` | `oklch(0.58 0 0)` | Input boundary |
| `--ring` | `oklch(0.48 0.14 255)` | `oklch(0.78 0.10 250)` | Focus indicator |
| `--destructive` | `oklch(0.45 0.12 25)` | `oklch(0.78 0.10 25)` | Error and confirmed destruction |
| `--destructive-foreground` | `oklch(1 0 0)` | `oklch(0.16 0 0)` | Filled destructive text |
| `--success` | `oklch(0.45 0.09 150)` | `oklch(0.78 0.10 150)` | Successful result |
| `--success-foreground` | `oklch(1 0 0)` | `oklch(0.16 0 0)` | Filled success text |
| `--warning` | `oklch(0.45 0.08 75)` | `oklch(0.78 0.10 75)` | Attention needed |
| `--warning-foreground` | `oklch(1 0 0)` | `oklch(0.16 0 0)` | Filled warning text |
| `--info` | `oklch(0.48 0.14 255)` | `oklch(0.78 0.10 250)` | Informational state |
| `--info-foreground` | `oklch(1 0 0)` | `oklch(0.16 0 0)` | Filled information text |
| `--risk-low` | `oklch(0.45 0.09 150)` | `oklch(0.78 0.10 150)` | Authoritative low risk |
| `--risk-low-foreground` | `oklch(1 0 0)` | `oklch(0.16 0 0)` | Filled low-risk text |
| `--risk-medium` | `oklch(0.45 0.08 75)` | `oklch(0.78 0.10 75)` | Authoritative medium risk |
| `--risk-medium-foreground` | `oklch(1 0 0)` | `oklch(0.16 0 0)` | Filled medium-risk text |
| `--risk-high` | `oklch(0.45 0.12 25)` | `oklch(0.78 0.10 25)` | Authoritative high risk |
| `--risk-high-foreground` | `oklch(1 0 0)` | `oklch(0.16 0 0)` | Filled high-risk text |
| `--sidebar` | `oklch(0.985 0 0)` | `oklch(0.16 0 0)` | Sidebar surface |
| `--sidebar-foreground` | `oklch(0.20 0 0)` | `oklch(0.96 0 0)` | Sidebar text |
| `--sidebar-primary` | `oklch(0.48 0.14 255)` | `oklch(0.78 0.10 250)` | Sidebar primary role |
| `--sidebar-primary-foreground` | `oklch(1 0 0)` | `oklch(0.16 0 0)` | Sidebar primary text |
| `--sidebar-accent` | `oklch(0.95 0 0)` | `oklch(0.26 0 0)` | Sidebar hover and selection surface |
| `--sidebar-accent-foreground` | `oklch(0.20 0 0)` | `oklch(0.96 0 0)` | Sidebar hover and selection text |
| `--sidebar-border` | `oklch(0.62 0 0)` | `oklch(0.58 0 0)` | Sidebar boundary |
| `--sidebar-ring` | `oklch(0.48 0.14 255)` | `oklch(0.78 0.10 250)` | Sidebar focus indicator |

Popover uses the card pair. Secondary and accent use the muted surface with the main foreground.
Sidebar uses the background pair and the corresponding primary, accent, border, and ring roles.
These component roles are not compatibility aliases for retired tokens.

The `accent` token is neutral interaction feedback, not another colored primary action.
Neutral blue remains the sole primary accent. Sidebar selection does not authorize another accent-colored action.

## Choosing a color role

Use semantic utility pairs:

| Context | Utilities |
| --- | --- |
| Page | `bg-background text-foreground` |
| Card | `bg-card text-card-foreground border-border` |
| Floating content | `bg-popover text-popover-foreground` |
| Supporting text | `text-muted-foreground` |
| Primary action | `bg-primary text-primary-foreground` |
| Secondary action | `bg-secondary text-secondary-foreground` |
| Neutral interaction | `bg-accent text-accent-foreground` |
| Input boundary | `border-input` |
| Focus | `ring-ring` |
| Filled status | The status background and its matching foreground |

Use status and risk text on background, card, or muted surfaces.
Use `border-border-soft` only for decorative separators.
Use the stronger boundary token for controls and required indicators.
Do not lower text or boundary contrast through opacity without rendered evidence.

Pair status colors with text and an icon where useful.
Use risk tokens only for a server-provided tool risk level.
For an unrated tool, use muted text and the label “Risk not rated”.
Do not derive risk from scope names or label permission groups with risk.
Low risk is not proof of safety.

## Tailwind mapping and theme precedence

Define every table entry in `:root, [data-theme="light"]` and `[data-theme="dark"]` in `tokens/theme.css`.
Map every role through `@theme inline` in `web/src/styles/index.css`.
For example, `--color-background: var(--background)` exposes `bg-background`.
`--color-primary-foreground: var(--primary-foreground)` exposes `text-primary-foreground`.

Use a `prefers-color-scheme: dark` fallback only for `:root:not([data-theme])`.
An explicit light or dark preference takes precedence over system appearance.
System mode follows later OS changes.

The first-paint script and React provider must share the `aib.theme` preference contract.
The script sets the resolved `data-theme` before first paint.
A fixed CSP hash permits the script without `unsafe-inline`.
Set the matching CSS `color-scheme` and the `light dark` color-scheme metadata.
Theme native controls and scrollbars with semantic roles.
Portaled content must inherit the root theme.

## Raw-palette enforcement

T046 requires `web/eslint-rules/no-raw-palette.js` as an error for `src/**/*.{ts,tsx}` in `web/eslint.config.js`.
The `web-lint` recipe and the CI web job must run that rule.
The rule must report existing violations until their migration completes. There are no legacy-file exceptions.

The rule must enforce these behaviors:

- Reject raw Tailwind palette utilities, including numbered families, named black/white colors, and gradient stops.
- Reject arbitrary literal colors in utility classes, including hexadecimal, RGB, HSL, and OKLCH values.
- Inspect `className` strings, template literals, `cn()`, `clsx()`, and `cva()` base and variant values.
- Apply the same restrictions through responsive, state, and theme prefixes, opacity suffixes, and important modifiers.
- Reject dynamically assembled color classes that bypass analysis.
- Accept the semantic color utilities from this contract.
- Permit `currentColor` and transparent presentation where appropriate.

Raw color definitions belong only in the token source and local brand artwork.
Do not add a second palette in component CSS, inline styles, or application configuration.
A Tailwind source allow-list does not replace the ESLint failure.
[TOKEN_GUIDE.md](TOKEN_GUIDE.md) describes how variants select these roles.

## Contrast and release evidence

Principle XI requires WCAG 2.1 AA. Feature 047 targets WCAG 2.2 AA.
Token names and opaque color calculations do not prove rendered compliance.

| Pair | Minimum |
| --- | --- |
| Main foreground on its surface | 4.5:1 |
| Muted foreground on muted, background, and card | 4.5:1 |
| Primary foreground on primary | 4.5:1 |
| Status and risk text on supported surfaces | 4.5:1 |
| Paired foreground on each filled status or risk color | 4.5:1 |
| Required control boundary against its adjacent surface | 3:1 |
| Focus ring against its adjacent surface | 3:1 |

Measure rendered combinations in both themes, including hover, focus, disabled presentation, opacity, and overlays.
T053 covers the research matrix of 74 text/control pairs and 14 filled-status pairs.
The [research](../../../../specs/047-redesign-consent-console/research.md#2-tokens-accent-and-enforcement) records planning calculations, not release evidence.

Every component story needs light and dark accessibility checks and a reviewed visual baseline per theme.
Storybook accessibility uses `parameters.a11y.test = 'error'`. Accessibility and visual-regression failures must block CI.
Browser checks also cover focus, native controls, theme flash, and forced colors.
See [ACCESSIBILITY_GUIDE.md](ACCESSIBILITY_GUIDE.md) for the full state matrix.

## Cutover rule

Replace token callers by semantic purpose, not by matching numeric shades.
Remove obsolete palette exports and scattered overrides in the same cutover.
Do not preserve retired token names as aliases or add a second active visual system.
Historical design decisions remain historical records, not component styling instructions.
