# Color Guide — Consent UI v2

## Status

This is the target token contract for [ADR 037](../../../../adrs/037-design-system-rebuilt-on-shadcn-radix.md), which remains proposed. The runtime token migration belongs to implementation phase 1.

Use [DESIGN_PRINCIPLES.md](DESIGN_PRINCIPLES.md) for interaction and layout. This guide replaces the warm-neutral, trust, and CTA palette guidance for v2.

## Semantic OKLCH tokens

Store all values in `web/src/design-system/tokens/`. The following table specifies the starting palette. Each value uses `oklch(L C H)`.

| Token | Light | Dark | Purpose |
| --- | --- | --- | --- |
| `--background` | `oklch(0.985 0 0)` | `oklch(0.16 0 0)` | Page surface |
| `--foreground` | `oklch(0.20 0 0)` | `oklch(0.96 0 0)` | Main text |
| `--card` | `oklch(1 0 0)` | `oklch(0.21 0 0)` | Contained surface |
| `--card-foreground` | `oklch(0.20 0 0)` | `oklch(0.96 0 0)` | Card text |
| `--muted` | `oklch(0.95 0 0)` | `oklch(0.26 0 0)` | Secondary surface |
| `--muted-foreground` | `oklch(0.45 0 0)` | `oklch(0.78 0 0)` | Supporting text |
| `--border` | `oklch(0.62 0 0)` | `oklch(0.58 0 0)` | Required control boundaries |
| `--border-soft` | `oklch(0.90 0 0)` | `oklch(0.32 0 0)` | Decorative separators only |
| `--primary` | `oklch(0.48 0.14 255)` | `oklch(0.78 0.10 250)` | Primary action and links |
| `--primary-foreground` | `oklch(1 0 0)` | `oklch(0.16 0 0)` | Primary action text |
| `--ring` | `oklch(0.48 0.14 255)` | `oklch(0.78 0.10 250)` | Focus indicator |
| `--destructive` | `oklch(0.45 0.12 25)` | `oklch(0.78 0.10 25)` | Error and confirmed destruction |
| `--success` | `oklch(0.45 0.09 150)` | `oklch(0.78 0.10 150)` | Successful result |
| `--warning` | `oklch(0.45 0.08 75)` | `oklch(0.78 0.10 75)` | Attention needed |
| `--info` | `oklch(0.48 0.14 255)` | `oklch(0.78 0.10 250)` | Informational state |
| `--risk-low` | `oklch(0.45 0.09 150)` | `oklch(0.78 0.10 150)` | Authoritative low risk |
| `--risk-medium` | `oklch(0.45 0.08 75)` | `oklch(0.78 0.10 75)` | Authoritative medium risk |
| `--risk-high` | `oklch(0.45 0.12 25)` | `oklch(0.78 0.10 25)` | Authoritative high risk |

Status text and icons sit on background, card, or muted surfaces. Filled status badges use a paired `--*-foreground`: white in light mode and background in dark mode. Define each pair explicitly in the token file.

Map shadcn roles without extra palettes: popover to card, secondary and accent surfaces to muted, input to border, and sidebar to background. Their foregrounds use the corresponding semantic text token. These are component roles, not legacy compatibility aliases.

Unknown risk uses muted text and a textual explanation. Low risk is not proof of safety. Do not derive risk from scope names.

## Tailwind mapping

Map semantic values through `@theme inline`, for example `--color-background: var(--background)` and `--color-primary: var(--primary)`. Map every token in the table and each paired foreground.

Use `bg-background text-foreground`, `bg-card text-card-foreground`, `text-muted-foreground`, `border-border`, and `ring-ring`. A primary action uses `bg-primary text-primary-foreground`.

Use `border-border-soft` only when the border is decorative. Input boundaries, focus, and state indicators require the stronger token.

Do not add palette values in component styles. Do not reintroduce `trust-*`, `cta-*`, `neutral-50`, or raw Tailwind color families.

## Theme precedence

The light values are the default. `[data-theme="dark"]` defines every dark value. `[data-theme="light"]` always keeps the light values.

A `prefers-color-scheme: dark` rule applies dark values only to `:root:not([data-theme])`. This fallback does not override an explicit preference.

Before first paint, the inline script reads the browser preference and resolves system appearance. It sets `data-theme` to `light` or `dark`. React keeps that attribute current after system or preference changes.

Use `<meta name="color-scheme" content="light dark">`. Set CSS `color-scheme: light` or `dark` for the resolved theme. Form controls and scrollbars must match. A fixed CSP script hash permits the bootstrap without `unsafe-inline`.

## Palette enforcement

Add an ESLint rule to the existing flat configuration. It must fail on raw palette utilities in JSX, template strings, `clsx`, `cn`, and CVA variants.

The rule must recognize variant prefixes, opacity suffixes, important modifiers, and arbitrary literal colors. Examples include `hover:bg-blue-500/50`, `dark:text-gray-100`, and `bg-[#ffffff]`.

Permit semantic color utilities, `currentColor`, and transparent presentation where appropriate. Reject dynamically assembled color class names that bypass analysis. Raw color definitions belong only in the token source and local brand artwork.

A Tailwind source allow-list alone does not produce a clear lint failure. Use ESLint for enforcement. During migration, scope exceptions to named legacy files. Remove those exceptions in phase 3.

## Contrast acceptance

Validate rendered colors in both themes after opacity, overlays, hover, disabled presentation, and focus styles apply. Do not claim compliance from token names.

| Pair | Minimum |
| --- | --- |
| Foreground on background/card | 4.5:1 |
| Muted foreground on muted/background/card | 4.5:1 |
| Primary foreground on primary | 4.5:1 |
| Status and risk text on supported surfaces | 4.5:1 |
| Paired foreground on filled status | 4.5:1 |
| Control border against adjacent surface | 3:1 |
| Focus ring against adjacent surface | 3:1 |

Soft separators are decorative and do not establish control boundaries. Never use color alone for status, risk, or selection. Labels and icons carry the same meaning.

Storybook must run every component story in both themes with `parameters.a11y.test = 'error'`. Browser validation still checks focus, native controls, theme flash, and forced-color behavior.

## Migration

Replace legacy token callers by semantic purpose, not by matching numeric shades. Remove legacy palette exports and overrides after all five views migrate. Update the remaining design guides in the same cutover.

See [research](../../../../specs/046-redesign-consent-console/research.md) for the accent decision and [quickstart](../../../../specs/046-redesign-consent-console/quickstart.md) for validation.
