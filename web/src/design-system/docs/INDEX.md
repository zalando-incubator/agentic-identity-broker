# Design System Documentation Index

## Authority and delivery status

The stakeholder accepted [ADR 038](../../../../adrs/038-design-system-rebuilt-on-shadcn-radix.md) on 2026-09-27 and approved its UX amendment on 2026-10-04.
[DESIGN_PRINCIPLES.md](DESIGN_PRINCIPLES.md) defines the current direction for [feature 047](../../../../specs/047-redesign-consent-console/spec.md).
These guides describe one target system, not an alternative presentation or a completed release.

The feature [tasks](../../../../specs/047-redesign-consent-console/tasks.md) track implementation and observed verification separately.
The [plan](../../../../specs/047-redesign-consent-console/plan.md) describes the documentation inventory and CI checks.
Principle XI requires semantic tokens, self-hosted assets, accessible light/dark stories, and WCAG 2.1 AA as the floor.
Feature 047 targets WCAG 2.2 AA. Both-theme accessibility checks and canonical light visual-regression checks block CI under [ADR 040](../../../../adrs/040-canonical-light-visual-reference-gate.md).

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
| [DESIGN_PRINCIPLES.md](DESIGN_PRINCIPLES.md) | Eight UX principles, authorization boundaries, shared shells, and historical note |
| [COLOR_GUIDE.md](COLOR_GUIDE.md) | Accepted semantic OKLCH tokens, three border roles, soft status pairs, and contrast requirements |
| [TOKEN_GUIDE.md](TOKEN_GUIDE.md) | Semantic utilities, typography, spacing, shape, motion tokens, and variants |
| [COMPONENT_ARCHETYPES.md](COMPONENT_ARCHETYPES.md) | Shared component anatomy, states, and role contracts |
| [DECISION_TREES.md](DECISION_TREES.md) | Selecting shells, components, actions, and semantic roles |
| [COMPONENT_PAIRING_GUIDE.md](COMPONENT_PAIRING_GUIDE.md) | Composing controls, collections, permission choices, feedback, and actions |
| [COMPOSITION_PATTERNS.md](COMPOSITION_PATTERNS.md) | Console, consent, agent detail, inbox, settings, and security-state recipes |
| [COMMON_MISTAKES.md](COMMON_MISTAKES.md) | Token, interaction, authorization, accessibility, and performance mistakes |
| [ACCESSIBILITY_GUIDE.md](ACCESSIBILITY_GUIDE.md) | WCAG floor and target, keyboard, focus, announcements, state matrix, and gates |
| [MOTION_GUIDE.md](MOTION_GUIDE.md) | CSS on decision routes, Motion on console routes, reduced-motion fades, and evidence |

## Accepted system at a glance

| Area | Target contract |
| --- | --- |
| Color | Tinted OKLCH neutrals; stronger blue action accent and blue-soft selection |
| Boundaries | `--border-subtle` for cards/dividers, `--border` for buttons/popovers, `--border-control` for forms |
| Status | Soft tinted background and readable text with a dot or icon, no solid status badge |
| Typography | Zalando Sans headings, Inter body and tabular dates/counts, JetBrains Mono identifiers |
| Layout | 1120 px centered 12-column console; 480 px three-zone consent card |
| Collections | Agents/Connections grid or list; fixed-height approvals inbox with detail panel |
| Motion | 120/160/200 ms feedback, 320 ms emphasis; CSS decisions, Motion permitted on console routes |
| Reduced motion | No movement or icon animation; retain opacity and color fades |
| Button variants | `primary`, `secondary`, `outline`, `ghost`, `destructive-outline`, `destructive` |
| Decisions | Pinned footer; consent Allow becomes Connect {Service} to continue when a selected connection is missing |
| Console | Three sidebar destinations, optional header purpose, icon toolbar, Settings in the user menu |
| Bundles | Concrete imports; Command and Motion excluded from decision-route graphs |

One accent action is visible per decision context. Remembered choices live in caret menus or a dedicated scope editor.
Deny and row actions use visible non-accent boundaries. Destructive confirmation remains a named Dialog.

## Component ownership

Keep owned shadcn/Radix source in the existing categories under `web/src/design-system/components/`.
Keep aliases, CVA, and `cn()` with `tailwind-merge`. Do not create a second primitive library.

Use EntityCard, EntityRow, CollectionToolbar, EmptyState, PermissionPanel, and DurationSelect for shared compositions.
The [cutover inventory](../../../../specs/047-redesign-consent-console/cutover-inventory.md) records earlier replacement decisions; current UX targets supersede its table-first examples.
Import concrete modules. Keep Command and Motion out of decision-route import barrels and the decision bundle.
`tokens/theme.css` owns visual values; the application stylesheet maps semantic roles through Tailwind v4 `@theme inline`.

Agents cards show identity, expiry, a Last updated date, and confirmed Revoke. Never add a permission-count column or per-agent count requests.
`activeGrantCount` counts UserGrant records, not permission sets. This clarification changes no API contract.

## Evidence required before release

Every primitive, pattern, and screen story needs light and dark coverage for applicable default, hover, focus-visible, disabled, error, loading, and overlay-open states.
Screen stories add empty, typical, dense, stale, and realistic long content at 375, 768, and 1280 px.
Consent adds a risk callout, missing connection, already-granted choice, denied outcome, and invalid session.
Approvals adds high-risk selection, expired request, and open remembered-scope editor.

Both-theme accessibility checks use `parameters.a11y.test = 'error'`. Reviewed pixel references cover eligible light 1280 × 720 cases only, under [ADR 040](../../../../adrs/040-canonical-light-visual-reference-gate.md).
Story `play` tests must cover permission choice, duration, keyboard approval, and confirmed revoke.
Assert that approval list bounds do not change during a decision and Allow stays visible at 1280 × 720.
Update Ginkgo and Playwright journeys through page objects while preserving security and callback behavior.

Browser evidence covers focus, keyboard paths, announcements, zoom, reduced motion, both themes, and no automatic third-party requests.
Neither shell permits horizontal page scrolling at 320 px or 200% zoom.
The feature requires one complete cutover without flags, dual presentations, or compatibility paths.

## Historical note — 2026-09-27

Refined Trust Architecture is the retired direction, not current implementation guidance.
[ADR 038](../../../../adrs/038-design-system-rebuilt-on-shadcn-radix.md) records its replacement.
[DESIGN_PRINCIPLES.md](DESIGN_PRINCIPLES.md#historical-note--refined-trust-architecture-retired-2026-09-27) preserves a dated summary.
Historical feature decisions remain intact. They do not authorize a second active palette or motion system.
