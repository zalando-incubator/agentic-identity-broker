# Design Principles

## Status and authority

**Current direction: the focused consent and console system in accepted ADR 037.**
The stakeholder accepted [ADR 037](../../../../adrs/037-design-system-rebuilt-on-shadcn-radix.md) on 2026-09-27.
[Feature 047](../../../../specs/047-redesign-consent-console/spec.md) defines the required behavior.

This guide governs the implementation. Acceptance does not establish runtime or validation completion.
The feature [tasks](../../../../specs/047-redesign-consent-console/tasks.md) record those results separately.
There is one accepted visual system, not a choice between old and new presentations.

Constitution Principle XI defines the design-system process and the WCAG 2.1 AA floor.
It does not prescribe a palette, typeface, border style, or animation duration.
ADR 037 selects those visual choices. Feature 047 targets WCAG 2.2 AA.

## Binding design-system process

Follow these requirements:

- Reuse design-system primitives and contribute universal components to the design system.
- Read `DECISION_TREES.md`, `COMPONENT_PAIRING_GUIDE.md`, and `COMMON_MISTAKES.md` before component work.
- Define semantic color, type, spacing, radius, motion, and theme tokens under `web/src/design-system/tokens/`.
- Use Tailwind v4 token mappings and CVA variants without component-specific token bypasses.
- Do not reference raw palette utilities in components.
- Support light and dark themes in every component story.
- Require accessibility and reviewed visual-regression checks in both themes, with failures that block CI.
- Use semantic HTML, appropriate ARIA, visible focus, and sufficient contrast.
- Self-host brand assets and fonts without automatic third-party font, script, or image requests.
- Obtain an accepted ADR before another change to the visual direction.

Existing source, snippets, and token names do not prove compliance.
Implementation and rendered evidence must satisfy the accepted contract.

## A focused security tool

A decision view explains who requests access, what access means, and what happens next.
A console view helps users inspect and change existing access.

Use neutral surfaces, 1px semantic borders, compact spacing, and a sans-serif hierarchy.
Do not use decorative gradients, textures, strong card shadows, editorial serif headings, or page-entry animation.

Each view has at most one accent-colored primary action.
Consent uses Allow. Tool review uses Approve once.
Deny and remembered approval remain non-accent actions.
Destructive confirmation appears only after an explicit request.

Required permissions remain locked. Optional choices remain explicit.
Color, animation, and browser preferences must never imply authorization.

## Two shared shells

| Component | Responsibility |
| --- | --- |
| ConsoleShell | Collapsible sidebar, local wordmark, search, navigation, pending count, user menu, and shared page header |
| DecisionShell | Centered column with a 640px maximum, local wordmark, focused content, and small footer |
| PageHeader | Page title, one-line purpose, and the action assigned by the UI contract, if any |

These universal components belong in `web/src/design-system/components/layout/`.
The mobile console uses Sheet. The collapsed sidebar uses a compact local mark with an accessible name.

The five existing route addresses remain unchanged. `/settings` adds browser theme preferences.
An authorization session selects DecisionShell on `/agents/:id`, including re-consent and invalid-session errors.
Without that context, agent detail uses ConsoleShell. Approval review always uses DecisionShell.

At 320px and 200% zoom, neither shell permits horizontal page scrolling.
Responsive table rows retain their actions and permission details.
The desktop Agents view must fit 12 rows in a 1080px-high viewport.

## Typography and local brand

| Token | Family | Use |
| --- | --- | --- |
| `font-display` | Zalando Sans Variable | Titles, decision names, empty-state headings, and statistics |
| `font-sans` | Inter Variable | Body text, controls, and navigation |
| `font-mono` | JetBrains Mono Variable | Identifiers, existing scope displays, arguments, and timestamps |

Use normal-width Zalando Sans. Do not add a SemiExpanded download.
Self-host the subsetted WOFF2 files and licenses in `web/public/fonts/`.
Keep fallback fonts and `font-display: swap`.
Preload only the Zalando Sans and Inter latin subsets. Load the mono face only where needed.
Measure the compressed decision-route budget instead of assuming that variable fonts are smaller.

Preserve the supplied wordmarks through text-to-path conversion in `web/public/brand/`.
Use the black wordmark in light mode and the white wordmark in dark mode.
Remove remote SVG font references. Derive the compact mark from the local favicon, not a new shield.

Unknown external logos use a local fallback.
User-directed external links and OAuth2 navigation remain available.
No page automatically loads third-party fonts, scripts, or images.

## Color and themes

[COLOR_GUIDE.md](COLOR_GUIDE.md) owns the complete semantic OKLCH color contract.
[TOKEN_GUIDE.md](TOKEN_GUIDE.md) defines token use and maps the roles through Tailwind 4 `@theme inline`.

Neutral blue is the sole primary accent. The `accent` token is a neutral hover and selection surface.
Status colors communicate success, warning, error, information, and authoritative tool risk.
Risk has a label and explanation. An unrated tool shows “Risk not rated”.
Permission groups show no risk indicator because permission sets carry no rating.

Light, dark, and system choices persist per browser under `aib.theme`.
The first-paint script and React provider use the same preference precedence.
System mode follows later OS changes. Explicit light or dark mode does not.
Native controls, scrollbars, and portaled overlays use the resolved theme.

## Owned components and copy

Use the Radix versions of shadcn/ui components in the existing design-system categories.
Keep CVA, existing aliases, and the `cn()` utility with `tailwind-merge`.
Do not create a competing `components/ui` library.

The Button variants are `primary`, `secondary`, `outline`, `ghost`, and `destructive`.
Use Dialog, DropdownMenu, Input, RadioGroup, Separator, and Toaster instead of their replaced components.
The [cutover inventory](../../../../specs/047-redesign-consent-console/cutover-inventory.md) records every replacement and retained component.

Wordmark, TruncatedText, ThemeChoice, ConsoleShell, DecisionShell, and PageHeader are universal design-system components.
Use Lucide icons with text labels. Hide decorative icons from assistive technology.
Give each icon-only control an accessible name.

TanStack Table owns table state, not markup or authorization. Command uses cmdk for search and keyboard interaction.
Table and Command must not enter the initial decision-route bundle.
No barrel imported by decision routes can re-export either component. Console consumers import their concrete modules.

Pages and application components take user-facing strings from `@copy`.
Design-system components receive UI copy through required props or children. They do not import `@copy`.
Use action-first wording and preserve technical identifiers exactly.

## Interaction and motion

Use CSS-only feedback at 120–200ms with ease-out timing.
Use 120ms for hover, 160ms for controls, and 200ms for overlays.
`@starting-style` is an optional visual enhancement, not an interaction requirement.

Reduced motion removes movement and delay.
Do not animate page entry or approval decisions with shared-layout motion.
Do not add Framer Motion.
[MOTION_GUIDE.md](MOTION_GUIDE.md) defines the motion tokens and reduced-motion contract.

Dialog and Sheet trap focus, provide a close control, and restore focus to the trigger.
Tooltips supplement visible labels and support keyboard focus.
Focus and interaction must not wait for animation completion.

## Truthful state

A domain-verified Agent Origin Label does not verify a publisher's legal identity.
Without CIMD metadata, consent shows “Registered by your administrator”.
Console views show no Agent Origin Label. No existing response identifies a publisher, so the UI shows no publisher field.
Localhost warnings remain prominent.

A connection is not a grant. Its state reflects token usability from existing session fields and authoritative operation results.
“No connection” appears only for a required service without a connection in agent context.
Do not infer missing scopes from the provider's scope catalogue.

Permission groups show their human-readable name and description.
Do not add raw scope strings to views that do not show them today.
Do not display last use or substitute creation and modification times for it.

The Agents list shows agent identity, expiry, View, and confirmed Revoke.
It has no permission-set count column and makes no per-agent count requests.
The existing `activeGrantCount` counts UserGrant records, not permission sets.
This clarification changes no API contract.

Revocation requires confirmation. An optimistic pending state is not server success.
Failure restores the row and announces an error.
Approval and grant creation wait for the server result.

## Accessibility and verification

Keep the WCAG 2.1 AA floor and meet the feature's WCAG 2.2 AA target.
All text requires at least 4.5:1 contrast. Controls and focus indicators require at least 3:1 against adjacent surfaces.

Every action has a keyboard path. Focus remains visible and unobscured by sticky controls.
Target sizes meet WCAG 2.2 AA, with larger touch targets where space permits.

New approvals, decision results, and toasts use status announcements without moving focus.
Accessible expansions expose truncated text and existing exact-scope displays.
React escapes untrusted text.

Every component story runs in both themes with the applicable interaction states and a reviewed visual baseline.
Accessibility and visual-regression failures block CI.
Browser journeys cover focus, zoom, reduced motion, and network privacy beyond automated accessibility checks.
[ACCESSIBILITY_GUIDE.md](ACCESSIBILITY_GUIDE.md) defines the evidence requirements.

## Single-cutover rule

Replace the design system and every affected caller in one cutover.
Do not introduce feature flags, parallel presentations, compatibility aliases, or a second design-system directory.
Remove obsolete presentation, tokens, fonts, animations, and dependencies in that cutover.

The [plan](../../../../specs/047-redesign-consent-console/plan.md) owns the full documentation inventory and acceptance gates.
Every current guide must describe the accepted system. Historical feature records preserve their original decisions.

## Historical note — Refined Trust Architecture, retired 2026-09-27

Refined Trust Architecture was the previous visual direction.
Its typography, palette, elevation, and motion decisions remain recorded in [ADR 006](../../../../adrs/006-frontend-stack.md).
[ADR 037](../../../../adrs/037-design-system-rebuilt-on-shadcn-radix.md) superseded that direction on 2026-09-27.
These facts describe history. They are not current implementation rules or a second token contract.
