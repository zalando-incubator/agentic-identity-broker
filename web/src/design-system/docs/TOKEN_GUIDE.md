# Design Token Guide

## Authority and delivery status

[ADR 037](../../../../adrs/037-design-system-rebuilt-on-shadcn-radix.md) became Accepted on 2026-09-27.
This guide defines token use for [feature 047](../../../../specs/047-redesign-consent-console/spec.md).
[DESIGN_PRINCIPLES.md](DESIGN_PRINCIPLES.md) defines the current visual direction.

These are implementation requirements, not a claim that the runtime cutover or verification is complete.
The feature [tasks](../../../../specs/047-redesign-consent-console/tasks.md) track those results.

## One token source

Principle XI requires centralized semantic tokens for color, type, spacing, radius, motion, and theme.
Definitions belong under `web/src/design-system/tokens/`.
Component CVA variants select these tokens. They do not define another palette or visual system.

[COLOR_GUIDE.md](COLOR_GUIDE.md#semantic-oklch-contract) contains the complete color table, with every token and its exact light/dark value.
That table is the sole color contract. This guide does not duplicate its values.
It includes surface foregrounds, status and risk foregrounds, input and focus roles, and the complete sidebar set.

T054 implements the color contract in `tokens/theme.css` and maps it through `@theme inline` in `web/src/styles/index.css`.
The runtime must define every role in both themes.
Do not preserve the retired palette as compatibility aliases.

## Semantic color use

Choose a role by meaning:

| Meaning | Utility pair |
| --- | --- |
| Page content | `bg-background text-foreground` |
| Contained content | `bg-card text-card-foreground` |
| Floating content | `bg-popover text-popover-foreground` |
| Supporting text | `text-muted-foreground` on a supported surface |
| Primary action | `bg-primary text-primary-foreground` |
| Secondary action | `bg-secondary text-secondary-foreground` |
| Neutral hover or selection | `bg-accent text-accent-foreground` |
| Input boundary | `border-input` |
| Required boundary | `border-border` |
| Decorative separator | `border-border-soft` |
| Focus indicator | `ring-ring` |
| Sidebar | `bg-sidebar text-sidebar-foreground` |

The `accent` surface is neutral. It does not introduce a second colored action.
Headings use the foreground for their surface, not the primary-action color.
Status text uses its status role. Filled status badges use the matching foreground from the color table.

Application examples take their labels from `@copy`.
Primitives receive labels and other UI copy through required props or children.
Primitives do not import the application copy catalogue.

```tsx
<section className="space-y-4 rounded-lg border border-border bg-card p-4 text-card-foreground">
  <h2 className="font-display text-xl font-semibold">{title}</h2>
  <p className="text-sm text-muted-foreground">{description}</p>
  {children}
</section>
```

The example uses caller-supplied content. It does not define application text or a second Card implementation.

## Button variants

| Variant | Role |
| --- | --- |
| `primary` | The view's single neutral-blue accent action |
| `secondary` | Supporting action on a neutral surface |
| `outline` | Supporting action with a visible boundary |
| `ghost` | Repeated row actions, menu triggers, and low-emphasis controls |
| `destructive` | Destructive confirmation after an explicit request |

Use at most one accent-colored primary action per view.
Consent uses Allow. Tool review uses Approve once.
Deny and remembered approval use non-accent variants.
A resting revoke control opens a confirmation instead of acting as a destructive primary action.
Use the owned Button variants rather than restyling buttons at each callsite.

## Typography

| Token | Family | Use |
| --- | --- | --- |
| `--font-display` / `font-display` | Zalando Sans Variable | Titles, decision names, empty-state headings, and statistics |
| `--font-sans` / `font-sans` | Inter Variable | Body text, controls, and navigation |
| `--font-mono` / `font-mono` | JetBrains Mono Variable | Identifiers, existing scope displays, arguments, and timestamps |

Use normal-width Zalando Sans. Do not add a SemiExpanded download.
Self-host the WOFF2 files and their licenses in `web/public/fonts/`.
Use the latin and latin-ext subsets for Zalando Sans and Inter.
Retain the existing JetBrains Mono files.

Keep system fallbacks and `font-display: swap` in font-face rules.
Preload only the Zalando Sans and Inter latin subsets.
Load the mono face only where needed.
Do not import fontsource packages into runtime code or load remote fonts.

Use the existing size scale with semantic HTML:

| Utility | Size / line height | Typical use |
| --- | --- | --- |
| `text-xs` | 12px / 16px | Secondary metadata |
| `text-sm` | 14px / 20px | Controls and compact rows |
| `text-base` | 16px / 24px | Body text |
| `text-lg` | 18px / 28px | Subheadings |
| `text-xl` | 20px / 28px | Section headings |
| `text-2xl` | 24px / 32px | Page headings |
| `text-3xl` | 30px / 36px | Decision headings |

Use regular weight for body text, medium for labels, and semibold or bold for headings.
Preserve a logical heading order regardless of font size.
Do not reduce text contrast for metadata, helper text, or placeholders.
Keep technical identifiers exact, even when the visual presentation truncates them.

## Spacing, borders, and radius

Keep the existing 4px base spacing scale.
Use compact spacing without removing labels, actions, or focus clearance.

| Context | Existing spacing utilities |
| --- | --- |
| Label to input | `gap-2` or `mb-2` |
| Input to helper text | `gap-1` or `mt-1` |
| Related controls | `gap-2` or `gap-3` |
| Form fields and contained content | `gap-4` or `space-y-4` |
| Sections | `gap-6` or `space-y-6` |
| Responsive page padding | `p-4` with more space where the layout permits |

Define radius values centrally and select them through component variants.
Do not copy conflicting historical radius tables or add per-page radius values.
Use 1px `border`-token boundaries for containment.
Do not add decorative gradients, card-lift effects, premium shadows, or page-entry decoration.

DecisionShell has a maximum content width of 640px.
At 320px and 200% zoom, neither shell permits horizontal page scrolling.
The desktop Agents view must fit at least 12 rows in a 1080px-high viewport.
Target size, text wrapping, and keyboard access take precedence over visual density.

## Motion tokens

[MOTION_GUIDE.md](MOTION_GUIDE.md) owns the motion behavior.
Define these values centrally:

| Token | Value | Use |
| --- | --- | --- |
| `--motion-feedback` | `120ms` | Hover feedback |
| `--motion-control` | `160ms` | Control state changes |
| `--motion-overlay` | `200ms` | Dialog, Sheet, menu, and popover feedback |
| `--motion-ease` | `ease-out` | Every transition |

Use CSS transitions only. `@starting-style` is an optional visual enhancement.
Do not add page-entry animation, spring motion, shared-layout motion, or Framer Motion.
Under `prefers-reduced-motion: reduce`, remove movement and transition delay.
Interaction, focus, and announcements must not depend on animation completion.

## Themes and layering

The preference key is `aib.theme`, with `light`, `dark`, and `system` values.
Missing or invalid values resolve to `system`.
Explicit light or dark takes precedence over system appearance.
Only system mode follows later OS changes.

The first-paint script and provider must use the same preference contract.
CSS applies its dark fallback only to `:root:not([data-theme])`.
Native controls, scrollbars, and portaled overlays must use the resolved theme.

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
Keep Table and Command out of barrels imported by decision routes. Import their concrete modules only in console consumers.

Principle XI keeps WCAG 2.1 AA as the floor. Feature 047 targets WCAG 2.2 AA.
Measure text at 4.5:1 and required boundaries and focus indicators at 3:1 in both themes.
Every component story needs accessibility checks and a reviewed visual baseline for each theme.
Both accessibility and visual-regression failures must block CI.

Token calculations do not prove rendered compliance.
Browser verification must cover focus, forced colors, zoom, reduced motion, and theme precedence.
Record results separately from the contract.
For a change to the visual direction, obtain ADR acceptance before implementation.
