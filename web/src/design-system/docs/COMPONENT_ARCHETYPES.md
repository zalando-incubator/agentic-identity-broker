# Component Archetypes

## Authority and example status

[ADR 038](../../../../adrs/038-design-system-rebuilt-on-shadcn-radix.md) was accepted on 2026-09-27 and amended on 2026-10-04.
These archetypes define feature 047 targets, not completed release validation.
Read owned component source for exact props during implementation.

Owned shadcn/Radix source stays in `web/src/design-system/components/` under the existing categories.
CVA defines variants, and `cn()` merges classes.
Primitives receive user-facing copy through props. Applications supply that copy from `@copy`.

## Shared visual contract

| Role | Contract |
| --- | --- |
| Page | `bg-background text-foreground` |
| Card | `bg-card text-card-foreground border-border-subtle`, with 12 px radius |
| Raised content | `bg-popover text-popover-foreground border-border` |
| Form control edge | `border-border-control`, at least 3:1 against adjacent surface |
| Selection | `bg-primary-soft text-primary-soft-foreground` |
| Focus | `ring-ring`, visible without obstruction |
| Headings | `font-display`, self-hosted Zalando Sans Variable |
| Body, date, count | `font-sans`, self-hosted Inter Variable, tabular figures for dates and counts |
| Technical values | `font-mono`, self-hosted JetBrains Mono |
| Motion | Central 120/160/200/320 ms tokens; CSS on decisions, Motion permitted on console routes |

[COLOR_GUIDE.md](COLOR_GUIDE.md) owns exact OKLCH targets. [TOKEN_GUIDE.md](TOKEN_GUIDE.md) owns shape, spacing, and type.
Do not add component-local palettes, strong card shadows, or page-entry decoration. Use the shared overlay/focal elevation tokens. Only consent permits the static 3–4% accent wash approved in ADR 038's 2026-10-05 amendment.

## Button

Button represents an action. Navigation remains a link, with `asChild` where needed.
Expose variant through `data-variant`. Keep a pending spinner and `aria-busy` without claiming server success.

| Variant | Use |
| --- | --- |
| `primary` | One decision-context action; Allow on consent, disabled while selected connections are missing |
| `secondary` | Neutral supporting action |
| `outline` | Visible row action, Deny, or other supporting action |
| `ghost` | Tooltip-labeled icon button only |
| `destructive-quiet` | Agent and connection collection actions: neutral outline, danger color on hover or focus |
| `destructive-outline` | The single Revoke action in the agent detail header |
| `destructive` | Filled destructive confirmation inside Dialog |

Primary and destructive hover adjust lightness by 0.04. A press scales to 0.98 unless reduced motion removes movement.
Row actions are 32 px tall. Consent Deny and Allow are 40 px tall and share the footer width.
Do not add another primary action inside an entity card or disable Allow without showing the Connect path.

## Card, EntityCard, and EntityRow

Card groups related content; it does not imply navigation, permission, or a security guarantee.
Use owned Card parts and spacing tokens for ordinary content. EntityCard and EntityRow share an anatomy in two densities:

| Slot | Contract |
| --- | --- |
| Leading | 40 px rounded-square agent or service logo, or local ID-derived tint |
| Title | 14 px semibold name, without a competing status badge |
| Status | Separate anchored badge slot |
| Supporting | Duration, age, scopes, or a wrapping provider-state explanation |
| Meta | Anchored service icons or date |
| Actions | At most two visible: card footer or row right edge |
| Navigation | Linked whole surface with action buttons above, never nested in the link |

Agent cards are 120 px tall. Noncompact connection cards use a 160 px minimum and grow for actual explanations or wrapped metadata. `EntityRow.columnTemplate` controls list columns without descendant slot overrides.
Only clipped names show a tooltip. Keyboard focus on a linked surface reveals its clipped name without another tab stop.
Hover raises the card edge from `--border-subtle` to `--border`; focus shows the ring.
Busy dims only the affected card and puts a spinner in the active action.
Agents and Connections support grid and list views: page choice wins, then an explicit Appearance default, then the initial density fallback (list above twelve items, otherwise grid). Choosing a global default resets earlier page choices; later page toggles remain local. Approval inbox rows remain fixed-height beside a separate detail panel.

## Input and selection controls

Input uses a semantic surface, `--border-control`, and a focus ring. A placeholder never replaces a visible label.
Connect errors to their controls with `aria-invalid` and `aria-describedby`. Provide an inline one-line hint where useful.
Use Checkbox for independent permission and service choices. Required and already granted decision groups stay checked and locked.
Required uses a Lock icon and the word beside the group name. Optional is not a badge. Granted stays muted inline text in consent; agent detail uses a neutral check badge beside the permission name.
In decision mode, show the service-list chevron only for multiple selectable services. Console mode also preserves independent optional single-service choices.

DurationSelect uses one Select: Until I revoke it, 30 days, Custom date. Custom date opens a DatePicker in Popover.
Preserve native date validity and the minimum-date constraint. Agent detail shows DurationSelect directly in its duration card, without an extra editor popover.
ThemeChoice keeps a keyboard-operable Light/Dark/System choice. Settings presents three preview tiles and a grid/list preference.

## Dialog and other overlays

Dialog provides a named confirmation or short form. Radix supplies focus containment, Escape, and focus return.
The confirmation describes the target and effect in one sentence. Start focus on Cancel before a destructive action.
The consent technical-details Dialog shows existing client ID, redirect URI, and requested scopes.
Dialogs must fit narrow viewports and remain usable at 200% zoom. The full consent decision stays in DecisionShell.

| Overlay | Purpose |
| --- | --- |
| DropdownMenu | Three or more secondary actions, not a one-item Revoke menu |
| Popover | Contextual About links, existing scope values, or a custom-date picker |
| Tooltip | Supplementary help or full text for one-line truncation |
| Sheet | Console navigation on narrow screens |

## Status, assets, and feedback

Badge variants are soft `neutral`, `success`, `warning`, `danger`, and `info`.
Use 20 px height, 12 px medium text, 6 px radius, 6 px horizontal padding, and a 6 px dot or 12 px icon.
Keep a badge on the title baseline or in a fixed right-hand slot; never in a wrapping row.
Use one per entity. Risk variants represent only server-provided tool risk; “Risk not rated” remains neutral with a dash icon.
Badge `danger` reports a state. Filled Button `destructive` confirms an operation.

Wordmark receives an accessible `label` and uses local cropped black or white artwork. Compact mode uses the local mark.
Agent and service Avatars have rounded-square shape and a deterministic eight-hue fallback by ID. People remain circular.
Never load off-origin images automatically. Decorative icons are hidden from assistive technology.

TruncatedText uses ellipsis and a tooltip for one-line names. For multi-line descriptions it compares `scrollHeight` and `clientHeight` in ResizeObserver.
Only clipped content shows the keyboard-operable “More” / “Less” toggle. It does not change a collection row's height.

Skeleton matches the final structure. EmptyState uses an illustration, concept sentence, and next step where one exists.
Alert uses soft status color, tinted subtle border, 12 px padding, and a 16 px icon. Inline Alert has no box.
Sonner Toaster reports server-confirmed outcomes and follows the resolved theme.

## Shells, toolbar, and shared patterns

ConsoleShell and PageHeader compose the console at a centered 1120 px width. DecisionShell has no sidebar.
Shells fetch no data. PageHeader has an optional purpose sentence, a muted count, and a right-hand icon toolbar.
CollectionToolbar has 32 px search, sort, optional facet filter, and grid/list controls.
PermissionPanel and DurationSelect are shared by consent and agent detail without sharing authorization state.
Approval inbox panel and standalone `/approvals/:id` review share one presentation with separate route ownership.
Command uses cmdk and receives only acting-user records from its application caller.
Do not import Command, Motion, or console-only code into a decision route.
See [COMPOSITION_PATTERNS.md](COMPOSITION_PATTERNS.md) for route composition.

## Required story states

Every primitive, shared pattern, and screen needs applicable light and dark stories with blocking accessibility checks. Pixel references cover eligible light 1280 × 720 cases only, under [ADR 040](../../../../adrs/040-canonical-light-visual-reference-gate.md).
Cover default, hover, focus-visible, disabled, error, loading, and overlay-open states where applicable.
Screen stories must add typical, dense, empty, stale, and longest-realistic-content cases at 375, 768, and 1280 px.
Interaction tests cover permission selection, duration choice, keyboard approval, and confirmed revocation.

Principle XI sets the WCAG 2.1 AA floor. Feature 047 targets WCAG 2.2 AA.
Measure rendered contrast, keyboard behavior, focus return, announcements, viewport fit, and reduced motion before release.
These targets do not claim that runtime migration or validation is complete.
