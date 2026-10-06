# Color Guide

## Authority and delivery status

[ADR 038](../../../../adrs/038-design-system-rebuilt-on-shadcn-radix.md) became Accepted on 2026-09-27. The stakeholder approved its UX amendment on 2026-10-04.
This guide specifies color targets for [feature 047](../../../../specs/047-redesign-consent-console/spec.md).
[DESIGN_PRINCIPLES.md](DESIGN_PRINCIPLES.md) defines the current visual direction.

Acceptance establishes targets, not runtime completion or accessibility results.
Implement the roles in `web/src/design-system/tokens/theme.css`. The feature [tasks](../../../../specs/047-redesign-consent-console/tasks.md) track implementation and verification separately.

## Semantic OKLCH contract

This table owns the target light and dark values. [TOKEN_GUIDE.md](TOKEN_GUIDE.md) defines their use without a second palette.
These are target values from the approved UX plan, not measured browser results. Values for roles not listed in that plan follow the same surface and text pairings.

| Token | Light target | Dark target | Purpose |
| --- | --- | --- | --- |
| `--background` | `oklch(0.985 0.003 260)` | `oklch(0.175 0.006 260)` | Page surface |
| `--console-background` | `var(--background)` | `var(--card)` | Console main surface |
| `--foreground` | `oklch(0.21 0.012 260)` | `oklch(0.93 0.005 260)` | Main text |
| `--card` | `oklch(1 0.003 260)` | `oklch(0.21 0.008 260)` | Contained surface |
| `--card-foreground` | `oklch(0.21 0.012 260)` | `oklch(0.93 0.005 260)` | Card text |
| `--popover` | `oklch(1 0.003 260)` | `oklch(0.245 0.009 260)` | Raised surface |
| `--popover-foreground` | `oklch(0.21 0.012 260)` | `oklch(0.93 0.005 260)` | Raised content text |
| `--muted` | `oklch(0.965 0.005 260)` | `oklch(0.26 0.01 260)` | Inset or secondary surface |
| `--muted-foreground` | `oklch(0.47 0.012 260)` | `oklch(0.72 0.012 260)` | Supporting text |
| `--primary` | `oklch(0.50 0.19 262)` | `oklch(0.70 0.15 262)` | Primary action and links |
| `--primary-foreground` | `oklch(1 0 0)` | `oklch(0.175 0.006 260)` | Primary action text |
| `--primary-hover` | `oklch(0.46 0.19 262)` | `oklch(0.74 0.15 262)` | Primary button hover, 0.04 lightness shift |
| `--primary-soft` | `oklch(0.95 0.03 262)` | `oklch(0.30 0.06 262)` | Selected rows, active navigation, icon tiles |
| `--primary-soft-foreground` | `oklch(0.42 0.17 262)` | `oklch(0.82 0.11 262)` | Text on primary-soft |
| `--secondary` | `oklch(0.965 0.005 260)` | `oklch(0.26 0.01 260)` | Secondary action surface |
| `--secondary-foreground` | `oklch(0.21 0.012 260)` | `oklch(0.93 0.005 260)` | Secondary action text |
| `--accent` | `oklch(0.965 0.005 260)` | `oklch(0.26 0.01 260)` | Neutral hover surface, not active selection |
| `--accent-foreground` | `oklch(0.21 0.012 260)` | `oklch(0.93 0.005 260)` | Neutral hover text |
| `--border-subtle` | `oklch(0.93 0.006 260)` | `oklch(0.275 0.01 260)` | Card edges and dividers |
| `--border` | `oklch(0.88 0.008 260)` | `oklch(0.33 0.012 260)` | Button and popover edges |
| `--border-control` | `oklch(0.60 0.012 260)` | `oklch(0.54 0.012 260)` | Inputs, checkboxes, and radios |
| `--ring` | `oklch(0.50 0.19 262)` | `oklch(0.70 0.15 262)` | Visible focus indicator |
| `--destructive` | `oklch(0.45 0.12 25)` | `oklch(0.78 0.10 25)` | Filled destructive confirmation only |
| `--destructive-foreground` | `oklch(1 0 0)` | `oklch(0.175 0.006 260)` | Destructive confirmation text |
| `--destructive-hover` | `oklch(0.41 0.12 25)` | `oklch(0.82 0.10 25)` | Destructive confirmation hover, 0.04 lightness shift |
| `--success` | `oklch(0.95 0.04 150)` | `oklch(0.28 0.05 150)` | Soft success background |
| `--success-foreground` | `oklch(0.42 0.11 150)` | `oklch(0.82 0.11 150)` | Text on soft success |
| `--warning` | `oklch(0.95 0.04 75)` | `oklch(0.28 0.05 75)` | Soft warning background |
| `--warning-foreground` | `oklch(0.42 0.11 75)` | `oklch(0.82 0.11 75)` | Text on soft warning |
| `--status-danger` | `oklch(0.95 0.04 25)` | `oklch(0.28 0.05 25)` | Soft danger background |
| `--status-danger-foreground` | `oklch(0.42 0.11 25)` | `oklch(0.82 0.11 25)` | Text on soft danger |
| `--info` | `oklch(0.95 0.04 262)` | `oklch(0.28 0.05 262)` | Soft information background |
| `--info-foreground` | `oklch(0.42 0.11 262)` | `oklch(0.82 0.11 262)` | Text on soft information |
| `--risk-low` | `oklch(0.95 0.04 150)` | `oklch(0.28 0.05 150)` | Soft server-provided low risk |
| `--risk-low-foreground` | `oklch(0.42 0.11 150)` | `oklch(0.82 0.11 150)` | Low-risk text |
| `--risk-medium` | `oklch(0.95 0.04 75)` | `oklch(0.28 0.05 75)` | Soft server-provided medium risk |
| `--risk-medium-foreground` | `oklch(0.42 0.11 75)` | `oklch(0.82 0.11 75)` | Medium-risk text |
| `--risk-high` | `oklch(0.95 0.04 25)` | `oklch(0.28 0.05 25)` | Soft server-provided high or critical risk |
| `--risk-high-foreground` | `oklch(0.42 0.11 25)` | `oklch(0.82 0.11 25)` | High-risk text |
| `--sidebar` | `oklch(0.965 0.005 260)` | `oklch(0.175 0.006 260)` | Muted light / page dark sidebar surface |
| `--sidebar-foreground` | `oklch(0.21 0.012 260)` | `oklch(0.93 0.005 260)` | Sidebar text |
| `--sidebar-primary` | `oklch(0.50 0.19 262)` | `oklch(0.70 0.15 262)` | Sidebar action |
| `--sidebar-primary-foreground` | `oklch(1 0 0)` | `oklch(0.175 0.006 260)` | Sidebar action text |
| `--sidebar-accent` | `oklch(0.95 0.03 262)` | `oklch(0.30 0.06 262)` | Active sidebar item |
| `--sidebar-accent-foreground` | `oklch(0.42 0.17 262)` | `oklch(0.82 0.11 262)` | Active sidebar text |
| `--sidebar-border` | `oklch(0.93 0.006 260)` | `oklch(0.275 0.01 260)` | Sidebar edge |
| `--sidebar-ring` | `oklch(0.50 0.19 262)` | `oklch(0.70 0.15 262)` | Sidebar focus |
| `--avatar-1` | `oklch(0.95 0.06 25)` | `oklch(0.28 0.06 25)` | Local avatar tint for ID bucket 1 |
| `--avatar-1-foreground` | `oklch(0.40 0.12 25)` | `oklch(0.82 0.12 25)` | Bucket 1 initial text |
| `--avatar-2` | `oklch(0.95 0.06 60)` | `oklch(0.28 0.06 60)` | Local avatar tint for ID bucket 2 |
| `--avatar-2-foreground` | `oklch(0.40 0.12 60)` | `oklch(0.82 0.12 60)` | Bucket 2 initial text |
| `--avatar-3` | `oklch(0.95 0.06 100)` | `oklch(0.28 0.06 100)` | Local avatar tint for ID bucket 3 |
| `--avatar-3-foreground` | `oklch(0.40 0.12 100)` | `oklch(0.82 0.12 100)` | Bucket 3 initial text |
| `--avatar-4` | `oklch(0.95 0.06 145)` | `oklch(0.28 0.06 145)` | Local avatar tint for ID bucket 4 |
| `--avatar-4-foreground` | `oklch(0.40 0.12 145)` | `oklch(0.82 0.12 145)` | Bucket 4 initial text |
| `--avatar-5` | `oklch(0.95 0.06 185)` | `oklch(0.28 0.06 185)` | Local avatar tint for ID bucket 5 |
| `--avatar-5-foreground` | `oklch(0.40 0.12 185)` | `oklch(0.82 0.12 185)` | Bucket 5 initial text |
| `--avatar-6` | `oklch(0.95 0.06 225)` | `oklch(0.28 0.06 225)` | Local avatar tint for ID bucket 6 |
| `--avatar-6-foreground` | `oklch(0.40 0.12 225)` | `oklch(0.82 0.12 225)` | Bucket 6 initial text |
| `--avatar-7` | `oklch(0.95 0.06 265)` | `oklch(0.28 0.06 265)` | Local avatar tint for ID bucket 7 |
| `--avatar-7-foreground` | `oklch(0.40 0.12 265)` | `oklch(0.82 0.12 265)` | Bucket 7 initial text |
| `--avatar-8` | `oklch(0.95 0.06 315)` | `oklch(0.28 0.06 315)` | Local avatar tint for ID bucket 8 |
| `--avatar-8-foreground` | `oklch(0.40 0.12 315)` | `oklch(0.82 0.12 315)` | Bucket 8 initial text |

Define every listed CSS variable explicitly in each theme. Do not preserve retired `--border-soft` or `--input` as aliases.
Neutral colors carry 0.003–0.012 chroma at hue 260. Dark surfaces separate by lightness instead of bright outlines.
Status hues stay at 150 (success), 75 (warning), and 25 (danger). Critical risk adds a filled icon, not another badge fill.
Agents and services without a same-origin logo use one of the eight fixed avatar tint/initial-text pairs, selected by ID.
Do not generate raw colors inside a component. Add every pair to the both-theme contrast test.

## Choosing a color role

Use semantic utility pairs:

| Context | Utilities |
| --- | --- |
| Page | `bg-background text-foreground` |
| Card | `bg-card text-card-foreground border-border-subtle` |
| Raised content | `bg-popover text-popover-foreground border-border` |
| Supporting text | `text-muted-foreground` |
| Primary action | `bg-primary text-primary-foreground` |
| Active navigation or selected row | `bg-primary-soft text-primary-soft-foreground` |
| Neutral hover | `bg-accent text-accent-foreground` |
| Input, checkbox, or radio | `border-border-control` |
| Row action | `border-border` |
| Divider | `border-border-subtle` |
| Focus | `ring-ring` |
| Status | Soft background with its paired foreground and an icon or word |

Only form controls require a 3:1 boundary against their adjacent surface. A card or divider's decorative boundary does not.
Do not use `--border-control` for cards, filled buttons, or decorative dividers.
Badges use a soft background, colored text, a dot or icon, and no border. They are never solid status fills or action controls.
Alerts use a soft status background with a tinted, subtle-weight border and a 16 px icon.

Use risk colors only for a server-provided tool risk level. For unrated tools, use neutral text and “Risk not rated”.
Do not derive risk from scope names or label permission groups with risk. Low risk is not proof of safety.

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
The UX plan reports proposed contrast estimates for selected new values, not verified rendered results.
These estimates include a 3.8:1 control boundary on the background, 7.0:1 dark primary on the background, and 8.0–8.6:1 dark status text on its soft background.

| Pair to measure in both themes | Minimum |
| --- | --- |
| Main, card, and popover text on their surfaces | 4.5:1 |
| Muted text on muted, background, and card | 4.5:1 |
| Primary and destructive confirmation text on their fills | 4.5:1 |
| Primary-soft, soft status/risk, and each avatar initial on its background | 4.5:1 |
| Control boundary against each adjacent surface | 3:1 |
| Focus ring against each adjacent surface | 3:1 |

Extend `contrast.test.ts` to these pairs, including every soft status, risk, and avatar tint pair. Do not use a decorative border as a required control boundary.
Measure actual colors in both themes after opacity, overlays, hover, focus, and disabled styles apply.
The [research](../../../../specs/047-redesign-consent-console/research.md#2-tokens-accent-and-enforcement) contains earlier planning calculations, not current release evidence.

Every component story needs both-theme accessibility checks. Pixel references cover eligible light 1280 × 720 cases only, under [ADR 040](../../../../adrs/040-canonical-light-visual-reference-gate.md).
Storybook accessibility uses `parameters.a11y.test = 'error'`. Fail the CI job on accessibility or visual-regression failures.
Browser checks must cover focus, native controls, theme flash, and forced colors. See [ACCESSIBILITY_GUIDE.md](ACCESSIBILITY_GUIDE.md).

## Cutover rule

Replace token callers by semantic purpose, not by matching numeric shades.
Remove obsolete palette exports and scattered overrides in the same cutover.
Do not preserve retired token names as aliases or add a second active visual system.
Historical design decisions remain historical records, not component styling instructions.
