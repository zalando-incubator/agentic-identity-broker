# Design System Documentation Index

## Authority and delivery status

The stakeholder accepted [ADR 037](../../../../adrs/037-design-system-rebuilt-on-shadcn-radix.md) on 2026-09-27.
[DESIGN_PRINCIPLES.md](DESIGN_PRINCIPLES.md) defines the current direction for [feature 047](../../../../specs/047-redesign-consent-console/spec.md).
These guides describe one accepted system, not parallel visual choices.

Acceptance establishes implementation requirements. It does not prove runtime completion, theme support, or accessibility results.
The feature [tasks](../../../../specs/047-redesign-consent-console/tasks.md) track implementation and verification separately.
The [plan](../../../../specs/047-redesign-consent-console/plan.md) contains the full guidance inventory and release gates.

Principle XI requires semantic tokens, self-hosted assets, accessible light/dark stories, and a WCAG 2.1 AA floor.
Feature 047 targets WCAG 2.2 AA and requires blocking accessibility and visual-regression checks in both themes.
The constitution does not mandate a particular aesthetic.

## Reading order

Before component work, read these guides:

1. Read [DESIGN_PRINCIPLES.md](DESIGN_PRINCIPLES.md) for visual authority, shared shells, truthful state, and cutover rules.
2. Read [DECISION_TREES.md](DECISION_TREES.md) for component and variant selection.
3. Read [COMPONENT_PAIRING_GUIDE.md](COMPONENT_PAIRING_GUIDE.md) for composition and interaction hierarchy.
4. Read [COMMON_MISTAKES.md](COMMON_MISTAKES.md) for prohibited patterns.
5. Read [COLOR_GUIDE.md](COLOR_GUIDE.md) and [TOKEN_GUIDE.md](TOKEN_GUIDE.md) for the token contract.
6. Read [ACCESSIBILITY_GUIDE.md](ACCESSIBILITY_GUIDE.md) and [MOTION_GUIDE.md](MOTION_GUIDE.md) for behavior and evidence requirements.

Existing source and copied examples do not prove compliance.
Use the accepted contract and the owned component API, not retired examples or compatibility aliases.

## Guide map

| Guide | Responsibility |
| --- | --- |
| [DESIGN_PRINCIPLES.md](DESIGN_PRINCIPLES.md) | Accepted visual direction, authorization boundaries, shared shells, copy ownership, and historical note |
| [COLOR_GUIDE.md](COLOR_GUIDE.md) | Single complete semantic OKLCH table, exact light/dark values, contrast requirements, theme precedence, and palette enforcement |
| [TOKEN_GUIDE.md](TOKEN_GUIDE.md) | Semantic utility use, typography, spacing, radius, motion tokens, variants, and centralized token ownership |
| [COMPONENT_ARCHETYPES.md](COMPONENT_ARCHETYPES.md) | Owned component anatomy, variants, and interaction states |
| [DECISION_TREES.md](DECISION_TREES.md) | Selection of components, actions, roles, and visual hierarchy |
| [COMPONENT_PAIRING_GUIDE.md](COMPONENT_PAIRING_GUIDE.md) | Composition of controls, content, feedback, and actions |
| [COMPOSITION_PATTERNS.md](COMPOSITION_PATTERNS.md) | ConsoleShell, DecisionShell, PageHeader, tables, forms, and command search |
| [COMMON_MISTAKES.md](COMMON_MISTAKES.md) | Token, component, authorization, accessibility, and performance mistakes |
| [ACCESSIBILITY_GUIDE.md](ACCESSIBILITY_GUIDE.md) | WCAG floor and target, keyboard and focus behavior, announcements, state matrix, and blocking checks |
| [MOTION_GUIDE.md](MOTION_GUIDE.md) | CSS-only 120–200ms ease-out feedback, reduced motion, and motion verification |

## Accepted system at a glance

| Area | Contract |
| --- | --- |
| Color | Semantic OKLCH in both themes, neutral surfaces, one neutral-blue primary accent |
| Boundaries | 1px semantic borders, without decorative gradients or strong card shadows |
| Display font | Self-hosted Zalando Sans Variable, normal width |
| Body font | Self-hosted Inter Variable |
| Technical font | Self-hosted JetBrains Mono Variable |
| Motion | CSS-only 120ms hover, 160ms controls, and 200ms overlays, all ease-out |
| Reduced motion | No movement or transition delay |
| Button variants | `primary`, `secondary`, `outline`, `ghost`, `destructive` |
| Decisions | DecisionShell, maximum 640px, no sidebar |
| Console | ConsoleShell, PageHeader, collapsible sidebar, mobile Sheet |
| Copy | `@copy` in applications, required props or children in design-system components |
| Assets | Local outlined wordmarks, favicon, compact mark, and fonts |
| Bundles | Table and Command use concrete module imports and stay out of decision-route barrels |

The `accent` token names neutral hover and selection feedback. It is not another colored action.
Allow and Approve once are the respective decision-view primary actions.
Deny, remembered approval, and repeated row actions use non-accent variants.
Destructive styling belongs to explicit destructive confirmation.

## Component ownership

Keep owned shadcn/Radix source in the existing category directories under `web/src/design-system/components/`.
Keep existing aliases, CVA, and `cn()` with `tailwind-merge`.
Do not create a competing `components/ui` library.

Use Dialog, DropdownMenu, Input, RadioGroup, Separator, and Toaster for the replaced component names.
Wordmark, TruncatedText, ThemeChoice, ConsoleShell, DecisionShell, and PageHeader are universal design-system components.
The [cutover inventory](../../../../specs/047-redesign-consent-console/cutover-inventory.md) owns the full replacement and deletion matrix.

The retired components and category barrels are removed. Import components from their concrete directories.
`tokens/theme.css` owns the visual tokens. The application stylesheet maps semantic colors through Tailwind v4 `@theme inline`.
There is no TypeScript token barrel or Tailwind JavaScript configuration.

The Agents list shows agent identity, expiry, View, and confirmed Revoke.
It has no permission-set count column or per-agent count requests.
This clarification changes no API contract.

## Evidence required before release

Every component story needs light and dark coverage for these applicable states:

- Default
- Hover and focus-visible
- Disabled
- Error
- Loading
- Overlay-open

Each story also needs a reviewed visual-regression baseline per theme.
Accessibility and visual-regression failures must block CI.
Automation cannot approve new or changed baselines.

Browser evidence covers focus, keyboard paths, announcements, reduced motion, both themes, and the absence of automatic third-party requests.
Neither shell permits horizontal page scrolling at 320px or 200% zoom.
The feature requires one complete cutover without flags, dual presentations, or compatibility paths.

## Historical note — 2026-09-27

Refined Trust Architecture is the retired direction, not current implementation guidance.
[ADR 037](../../../../adrs/037-design-system-rebuilt-on-shadcn-radix.md) records its replacement.
[DESIGN_PRINCIPLES.md](DESIGN_PRINCIPLES.md#historical-note--refined-trust-architecture-retired-2026-09-27) preserves a dated summary.
Historical feature decisions remain intact. They do not authorize a second active palette or motion system.
