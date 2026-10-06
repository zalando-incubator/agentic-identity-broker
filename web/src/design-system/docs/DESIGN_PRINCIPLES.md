# Design Principles

## Status and authority

**Current direction: the consent and console system in accepted ADR 038, amended on 2026-10-04 and 2026-10-05.**
The stakeholder accepted [ADR 038](../../../../adrs/038-design-system-rebuilt-on-shadcn-radix.md) on 2026-09-27, approved its UX amendment on 2026-10-04, and requested the visual-review amendment on 2026-10-05.
[Feature 047](../../../../specs/047-redesign-consent-console/spec.md) defines required behavior.
[ADR 040](../../../../adrs/040-canonical-light-visual-reference-gate.md) governs canonical light pixel references. It changes no design choice or both-theme accessibility requirement.

This guide states the target, not completed runtime or validation work.
The feature [tasks](../../../../specs/047-redesign-consent-console/tasks.md) record implementation and verification separately.
There is one accepted visual system, not a choice between old and new presentations.

Constitution Principle XI defines the design-system process and the WCAG 2.1 AA floor.
It does not prescribe a palette, typeface, border style, or animation duration.
ADR 038 selects those visual choices. Feature 047 targets WCAG 2.2 AA.

## Binding design-system process

Follow these requirements:

- Reuse design-system primitives and contribute universal components to the design system.
- Read `DECISION_TREES.md`, `COMPONENT_PAIRING_GUIDE.md`, and `COMMON_MISTAKES.md` before component work.
- Define semantic color, type, spacing, radius, motion, and theme tokens under `web/src/design-system/tokens/`.
- Use Tailwind v4 token mappings and CVA variants without component-specific token bypasses.
- Do not reference raw palette utilities in components.
- Support light and dark themes in every component story.
- Require blocking accessibility checks in both themes and reviewed canonical light pixel checks under ADR 040.
- Use semantic HTML, appropriate ARIA, visible focus, and sufficient contrast.
- Self-host brand assets and fonts without automatic third-party font, script, or image requests.
- Obtain an accepted ADR before another change to the visual direction.

Existing source, snippets, and token names do not prove compliance.
Implementation and rendered evidence must satisfy the accepted contract.

## Eight design principles

1. **The decision is always on screen.** Consent and approval primary actions remain visible at 1280 × 720. Pin them on smaller screens.
2. **Show what changes the decision; link the rest.** Show who asks, what access they get, how long it lasts, and risk signals. Put documentation, client IDs, and raw scopes one click away where those details already exist.
3. **Nothing moves under the pointer.** Do not expand a row or card in place. Open details in a panel, dialog, or reserved region.
4. **No dead controls.** Render a control only when it has an effect. Measure clipping before showing a More toggle.
5. **Controls look like what they are.** Give action buttons visible boundaries. Make navigation a link or linked card. Never make a status badge clickable.
6. **Color carries meaning.** Use the primary accent for the current selection and one primary action. Use labeled semantic colors for status and risk. Keep other elements neutral.
7. **One anatomy for every entity.** Agents, connections, approvals, and remembered decisions share an icon, name, one supporting line, one status, and at most two visible actions.
8. **Motion explains a change.** Use 120–200 ms feedback and at most 320 ms emphasis. Remove movement and icon animation for reduced motion.

Keep the few facts that affect consent or risk impossible to miss. Do not add risk to permission groups that have no risk rating.
Required permissions stay locked. Optional choices stay explicit. Neither color, motion, nor browser preferences authorize an action.

## Two shared shells

| Component | Responsibility |
| --- | --- |
| ConsoleShell | Three destinations: Agents, Connections, Approvals. Collapsible sidebar, quiet search, user menu with Settings, and a centered main container. |
| DecisionShell | Focused, sidebar-free decisions. Consent uses a 480 px three-zone card with a pinned decision footer. Approval review keeps the same decision boundary. |
| PageHeader | One row with a title, optional muted count, and an icon toolbar or assigned actions. The purpose sentence is optional and belongs in list empty states. |

These universal components belong in `web/src/design-system/components/layout/`.
The mobile console uses Sheet. The collapsed sidebar uses a compact local mark with an accessible name.

Use `/agents` and `/connections` for lists, `/agents/:id` for agent detail or consent, and `/approvals` and `/approvals/remembered` for the inbox. Use `/settings/appearance` for preferences.
An authorization session selects DecisionShell on `/agents/:id`, including re-consent and invalid-session errors.
Without that context, agent detail uses ConsoleShell. Standalone `/approvals/:id` review uses DecisionShell.

Console content has a centered 1120 px maximum, with a 12-column grid. At 320 px and 200% zoom, neither shell permits horizontal page scrolling.
Keep the consent Allow action inside the viewport at 1280 × 720 with three groups and a risk callout.

## Typography and local brand

| Token | Family | Use |
| --- | --- | --- |
| `font-display` | Zalando Sans Variable | Page titles, decision names, and empty-state headings |
| `font-sans` | Inter Variable | Body text, controls, navigation, dates, and counts with tabular figures |
| `font-mono` | JetBrains Mono Variable | Tool names, identifiers, scope patterns, existing scope displays, and raw arguments |

Use normal-width Zalando Sans. Do not add a SemiExpanded download.
Self-host the subsetted WOFF2 files and licenses in `web/public/fonts/`.
Keep fallback fonts and `font-display: swap`.
Preload only the Zalando Sans and Inter latin subsets. Load the mono face only where needed.
Measure the compressed decision-route budget instead of assuming that variable fonts are smaller.

Preserve the supplied wordmarks through text-to-path conversion in `web/public/brand/`.
Crop the wordmark to `viewBox="47.4 15 406.7 38.5"` and the mark to `viewBox="10 17.9 46.2 27.7"`.
Use the black wordmark in light mode and the white wordmark in dark mode.
Remove remote SVG font references. Keep the supplied sidebar compact artwork separate from the square small-size favicon, which uses one bold outlined letter rather than three compressed letters.

Service identities use tinted initials. Unknown external agent logos use a local fallback.
User-directed external links and OAuth2 navigation remain available.
No page automatically loads third-party fonts, scripts, or images.

## Color and themes

[COLOR_GUIDE.md](COLOR_GUIDE.md) owns the complete semantic OKLCH color contract.
[TOKEN_GUIDE.md](TOKEN_GUIDE.md) defines token use and maps the roles through Tailwind 4 `@theme inline`.

The stronger blue primary is the sole action accent; `--primary-soft` marks active navigation, selection, and icon tiles.
Tint neutral surfaces. Use `--border-subtle` for cards and dividers, `--border` for buttons and popovers, and `--border-control` for form controls.
Keep collection cards flat. Use soft semantic elevation on floating layers and lighter elevation on consent and sticky draft controls. Consent alone may have a static 3–4% top accent wash and a 1 px ring. The light sidebar uses muted; dark console content uses card against the page-colored sidebar.
Soft status colors communicate success, warning, error, information, and authoritative tool risk with text and an icon or word.
Risk has a label and explanation. An unrated tool shows “Risk not rated”. Permission groups have no risk indicator.

Light, dark, and system choices persist per browser under `aib.theme`.
The first-paint script and React provider use the same preference precedence.
System mode follows later OS changes. Explicit light or dark mode does not.
Native controls, scrollbars, and portaled overlays use the resolved theme.

## Owned components and copy

Use the Radix versions of shadcn/ui components in the existing design-system categories.
Keep CVA, existing aliases, and the `cn()` utility with `tailwind-merge`.
Do not create a competing `components/ui` library.

Button variants are `primary`, `secondary`, `outline`, `ghost`, `destructive-quiet`, `destructive-outline`, and `destructive`.
Use `destructive-quiet` for agent and connection collection actions. Their neutral outline changes to danger color on hover or focus.
The agent detail header keeps `destructive-outline`. Reserve `ghost` for labeled icon controls.
Use Dialog, DropdownMenu, Input, Select, Separator, and Toaster for their accepted roles.
The [cutover inventory](../../../../specs/047-redesign-consent-console/cutover-inventory.md) records replacements and retained components.

Wordmark, TruncatedText, ThemeChoice, ConsoleShell, DecisionShell, PageHeader, EntityCard, EntityRow, CollectionToolbar, PermissionPanel, and DurationSelect are shared design-system components.
Use Lucide icons with labels. Hide decorative icons from assistive technology. Give each icon-only control an accessible name.

Command uses cmdk for console search. Keep Command, Motion, and other console-only dependencies out of decision-route bundles and imported barrels.
The decision-route bundle test must enforce the Motion boundary. Import concrete modules to keep these routes isolated.

Pages and application components take user-facing strings from `@copy`.
Design-system components receive UI copy through required props or children. They do not import `@copy`.
Use action-first wording and preserve technical identifiers exactly.

## Interaction and motion

Use `--motion-feedback: 120ms`, `--motion-control: 160ms`, `--motion-overlay: 200ms`, and `--motion-emphasis: 320ms`.
Use `cubic-bezier(0.2, 0, 0, 1)` and exits at 75% of the entry duration. Keep immediate focus and interaction.

Console routes can use owned lucide-animated icons and Motion for card or row exits, layout changes, and a `MotionConfig reducedMotion="user"` boundary.
Use browser View Transitions for card-to-detail navigation as a progressive enhancement. Without API support, navigate directly.
Consent and standalone approval review use owned CSS-animated SVGs only and must not import Motion.
Under reduced motion, retain opacity and color feedback but remove movement and icon animation.
[MOTION_GUIDE.md](MOTION_GUIDE.md) defines the full behavior.

Dialog and Sheet trap focus, provide a close control, and restore focus to the trigger.
Tooltips supplement visible labels and support keyboard focus.
Do not delay focus or authorization results for animation.

## Truthful state

A domain-verified Agent Origin Label does not verify a publisher's legal identity.
Without CIMD metadata, consent uses the neutral “Registered agent” fallback. It identifies no registrant or publisher.
Console headers make no origin or registrant claim from absent client URIs. Supplied identifiers and URI records appear in Technical details; no existing response identifies a publisher.
Localhost warnings remain prominent.

A connection is not a grant. Its state reflects token usability from existing session fields and authoritative operation results.
“No connection” appears only for a required service without a connection in agent context.
Do not infer missing scopes from the provider's scope catalogue.

Permission groups show their human-readable name and description. Only expose raw scopes where an existing response already supports them.
Do not display last use or substitute creation and modification times for it.

Agents cards show identity, grant expiry and `lastModifiedAt` as “Changed” metadata, and confirmed Revoke. The whole card links to detail.
The Agents list has no permission-set count or per-agent count requests.
`activeGrantCount` counts UserGrant records, not permission sets. This clarification changes no API contract.

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

Every component story runs in both themes with applicable interaction and accessibility checks across all six theme/viewport projects.
Under [ADR 040](../../../../adrs/040-canonical-light-visual-reference-gate.md), PNG capture and comparison run only in desktop `storybook-light`. The effective theme must be light and the actual viewport must be 1280 × 720. Intentional dark or narrow overrides keep interactions and accessibility checks without PNG references.
Accessibility failures and missing or changed required canonical references block CI. Human review remains mandatory for reference changes.
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
[ADR 038](../../../../adrs/038-design-system-rebuilt-on-shadcn-radix.md) superseded that direction on 2026-09-27.
These facts describe history. They are not current implementation rules or a second token contract.
