# Design Decision Trees

## Authority and example status

[ADR 038](../../../../adrs/038-design-system-rebuilt-on-shadcn-radix.md) was accepted on 2026-09-27 and amended on 2026-10-04.
This guide defines feature 047 targets, not completed release validation.
[DESIGN_PRINCIPLES.md](DESIGN_PRINCIPLES.md) and [COLOR_GUIDE.md](COLOR_GUIDE.md) govern visual choices.

## Choose the shell first

```text
Does /agents/:id have an authorization-session parameter?
  Yes -> DecisionShell with the consent card, even for invalid or expired sessions
  No  -> ConsoleShell with the 8 + 4 grant-detail layout

Is the route /approvals/:id?
  Yes -> DecisionShell for standalone tool review and resolved outcomes

Is the route /agents, /connections, /approvals, /approvals/remembered, or /settings/appearance?
  Yes -> ConsoleShell with PageHeader and a centered 1120 px container
```

Only the backend validates an authorization session. Do not turn a failed decision request into editable console content.
ConsoleShell owns the three-item sidebar, mobile Sheet, search, user menu, and main region. Shells fetch no data.
Consent uses a 480 px three-zone card with a pinned footer, not a 640 px stack of equal-weight sections.

## Choose the action variant

```text
Is this the explicit destructive confirmation inside Dialog?
  Yes -> destructive
Is this Revoke or Disconnect in an agent or connection collection?
  Yes -> destructive-quiet with ShieldOff for Revoke or Unlink for Disconnect; it opens the confirmation
Is this the single Revoke action in the agent detail header?
  Yes -> destructive-outline
Is this the primary action in the current decision context?
  Yes -> primary
Is this an icon-only control with a tooltip and accessible name?
  Yes -> ghost
Does a supporting action need a visible boundary?
  Yes -> outline
Otherwise -> secondary
```

There is no fallback to `primary` merely because a button exists.

| View | Primary action | Other actions |
| --- | --- | --- |
| Agents (`/agents`) | None | Whole card opens detail; Revoke is `destructive-quiet` with `ShieldOff` |
| Agent consent | Allow, disabled while selected connections are missing | Deny and inline Connect use `outline`; explain why Allow is disabled |
| Agent management | Save changes only for a dirty draft | Revoke access is `destructive-outline`; Cancel is non-accent |
| Connections (`/connections`) | None | Reconnect, Refresh, Retry use `outline`; Disconnect is `destructive-quiet` with `Unlink` |
| Pending approval inbox panel | Approve once | Deny uses `outline`; caret menus hold remembered choices |
| Standalone pending tool review | Approve once | Deny and remembered choices use non-accent controls |
| Remembered and resolved approvals | None | Confirmed Revoke or no decision action |
| Settings | None | Appearance and view preferences apply immediately |

One decision panel has one primary action. The background list remains stable while the panel's choice changes.
Dialog makes the background inert and focuses Cancel for destructive confirmation.

## Choose a component

| User need | Component | Boundary |
| --- | --- | --- |
| Enter text | Input or TextArea | Visible label and linked error |
| Choose one value | Select or RadioGroup | DurationSelect uses Select, with Custom date in a popover |
| Select independent permissions | Checkbox in PermissionPanel | Required and already granted decision groups stay locked |
| Choose appearance or default collection view | ThemeChoice or a segmented/preview-tile control | Preferences never submit consent |
| Confirm an operation | Dialog | Name the target, describe the effect, focus Cancel |
| Open three or more secondary actions | DropdownMenu | Never use a one-item overflow menu |
| Read short interactive reference material | Popover | Keep decision facts visible without opening it |
| Read supplementary help | Tooltip | Keep full text available to keyboard users |
| Open mobile navigation | Sheet | Preserve navigation and focus return |
| Switch views of the same object | URL-backed underline Tabs | Pending and Remembered are separate routes |
| Select services within a permission | Visible pressed-state service buttons on console; Popover on consent | Required services stay locked; names wrap on console |
| Browse agents or connections | EntityCard grid with EntityRow list toggle | Card links open detail; actions remain separate |
| Triage approvals | Fixed-height EntityRow list + detail panel | Below 1012 px, row opens `/approvals/:id` |
| Search, sort, filter, change density | CollectionToolbar | Mirror search, sort, and filters in the URL |
| Read long text | TruncatedText | One-line tooltip or measured multi-line More toggle |
| See operation feedback | Alert, InlineError, or Toaster | Do not report success before server acceptance |
| See no data or pending data | EmptyState or matching Skeleton | Distinguish empty, loading, error, and stale |

Reuse owned components. Keep Command and Motion out of decision-route import graphs and shared barrels.

## Choose truthful content

```text
Is this an Agents card or row?
  Show identity, expiry, relative change time from lastModifiedAt, and confirmed Revoke.
  Do not show counts, publisher, status filters, or Agent Origin Labels.

Is this a stored connection on /connections?
  Show provider, available scope count, state, and relative connection age.
  Do not show No connection or an account identifier.

Does an agent require a service without a stored connection?
  Show a Connect control in its detail rail or consent permission row.

Is this a permission group?
  Show name, description, required/optional control, and selectable services.
  Do not add risk ratings or new raw scope displays.
```

`activeGrantCount` counts UserGrant records, not permission sets. Do not request per-agent counts.
Connection states derive from existing data, not guessed scope coverage.
Tool approvals use server-provided risk, or “Risk not rated” when no rating is available.

## Choose text, surfaces, and assets

| Content | Semantic role |
| --- | --- |
| Heading or main text | `text-foreground` |
| Card text and edge | `text-card-foreground` on `bg-card`, with `border-border-subtle` |
| Supporting text | `text-muted-foreground` |
| Primary action | `text-primary-foreground` on `bg-primary` |
| Selected row or active nav | `text-primary-soft-foreground` on `bg-primary-soft` |
| Labeled status or risk | Soft semantic background with matching foreground and icon |
| Button or popover edge | `border-border` |
| Input, checkbox, radio | `border-border-control` |
| Card or divider edge | `border-border-subtle` |
| Focus indicator | `ring-ring` |

Use Zalando Sans for headings and Inter with tabular figures for dates/counts. Use relative recency and absolute expiry dates. Use JetBrains Mono for tool names, scope patterns, identifiers, and raw arguments.
Use Wordmark and safe local Avatar fallbacks. Do not load remote images automatically.

## Choose spacing and motion

Use 4–12 px inside a component, 16–24 px between components, and 32–48 px between sections.
Center console content at 1120 px; use a 12-column grid, or an 8 + 4 split for detail.
Use a 16 px gap for the agent and connection card grids. Keep rows at fixed height and actions visible.
Preserve usability at 320 px and 200% zoom without horizontal overflow.

Use 120–200 ms for feedback, 320 ms for emphasis, and `cubic-bezier(0.2, 0, 0, 1)`.
Use Motion only on console routes. Decision routes use owned CSS animations.
Under reduced motion, stop movement and icon animation but retain opacity and color fades.
See [TOKEN_GUIDE.md](TOKEN_GUIDE.md) and [MOTION_GUIDE.md](MOTION_GUIDE.md).

## Choose the owner

Application components own domain composition and obtain UI copy from `@copy`.
Primitives receive strings through props and do not import the application catalogue.
Universal patterns belong in the design system, with stories and behavior tests.

Every component story needs light and dark accessibility checks. Reviewed pixel references cover eligible light 1280 × 720 cases only, under [ADR 040](../../../../adrs/040-canonical-light-visual-reference-gate.md).
Principle XI requires WCAG 2.1 AA. Feature 047 targets WCAG 2.2 AA.
See [COMPONENT_PAIRING_GUIDE.md](COMPONENT_PAIRING_GUIDE.md), [COMPOSITION_PATTERNS.md](COMPOSITION_PATTERNS.md), and [COMMON_MISTAKES.md](COMMON_MISTAKES.md).
