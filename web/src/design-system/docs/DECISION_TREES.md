# Design Decision Trees

## Authority and example status

[ADR 037](../../../../adrs/037-design-system-rebuilt-on-shadcn-radix.md) was accepted on 2026-09-27.
This guide describes the feature 047 component contract, not completed release validation.
[DESIGN_PRINCIPLES.md](DESIGN_PRINCIPLES.md) and [COLOR_GUIDE.md](COLOR_GUIDE.md) govern visual choices.

## Choose the shell first

```text
Does /agents/:id have an authorization-session parameter?
  Yes -> DecisionShell, including invalid or expired session errors
  No  -> ConsoleShell for grant management

Is the route /approvals/:id?
  Yes -> DecisionShell, including resolved and expired outcomes

Is the route /delegations, /sessions, /approvals, or /settings?
  Yes -> ConsoleShell with PageHeader
```

Only the backend validates an authorization session.
Do not turn a failed decision request into editable console content.
DecisionShell has no sidebar and a content width of at most 640 px.
ConsoleShell owns the sidebar, mobile Sheet, navigation, search slot, and user-menu slot.
Shells fetch no data.

## Choose the action variant

```text
Does this button confirm destruction inside Dialog?
  Yes -> destructive
  No  -> Continue

Is this the single accent action assigned to this view?
  Yes -> primary
  No  -> Continue

Does this supporting action need a filled neutral surface?
  Yes -> secondary
  No  -> Does it need a visible boundary?
           Yes -> outline
           No  -> ghost
```

There is no fallback to `primary` merely because a button exists.
The limit is one accent action per view, not per section.

| View | Accent action | Other actions |
| --- | --- | --- |
| Agents (`/delegations`) | None | View and confirmed Revoke use non-accent row controls |
| Agent consent | Allow | Deny uses `secondary`, never destructive styling |
| Agent management | Save changes, only while the draft is dirty | Cancel and overflow menu use non-accent controls |
| Connections (`/sessions`) | None | Reconnect, supported Refresh, and Disconnect use non-accent controls |
| Approval queue | None | Inline Approve and Deny use non-accent controls |
| Pending tool review | Approve once | Approve and remember and Deny use non-accent controls |
| Resolved or expired tool review | None | No decision actions |
| Settings | None | Theme changes apply immediately |

An open modal Dialog makes the background inert.
Its confirmation can use `primary`, or `destructive` for revocation or disconnection.
Its cancel action remains non-accent.

## Choose a component

| User need | Component | Boundary |
| --- | --- | --- |
| Enter text | Input or TextArea | A visible label and linked error are required |
| Select one value | Select or RadioGroup | DropdownMenu is for actions, not form values |
| Select independent permissions | Checkbox | Required and previously granted decision groups stay locked |
| Change a binary preference | Switch | A preference never submits consent |
| Select grant expiry | DatePicker with duration choices | Preserve the existing validity semantics |
| Confirm an operation | Dialog | Name the target and explain the effect |
| Open an action menu | DropdownMenu | Revoke all access belongs in the detail overflow menu |
| Read short interactive help | Popover | Keep essential decision information visible |
| Read supplementary help | Tooltip | Provide a keyboard path |
| Open mobile navigation | Sheet | Preserve navigation and focus return |
| Switch detail panels | Tabs | Agent management has Permissions and Connections |
| Expand related details | Accordion | Permission groups expand to service names |
| Read a dense collection | Table | Use TanStack Table state with owned presentation |
| Search console records | Command | Scope results to the acting user |
| Read long metadata | TruncatedText | Escape text and provide accessible expansion |
| Read grouped content | Card | No decorative hover lift or nested action targets |
| See operation feedback | Alert, InlineError, or Toaster | Do not report success before server acceptance |
| See no data or pending data | EmptyState or Skeleton | Distinguish empty, loading, error, and stale content |

Use owned components in the existing categories. Do not add a competing application primitive library.
Import Table and Command through concrete module paths.
Do not re-export them from a barrel used by decision routes.

## Choose truthful content

```text
Is this an Agents list row?
  Show agent, expiry, View, and confirmed Revoke.
  Do not show count, status, or Agent Origin Label columns.
  Do not issue per-agent count requests.

Is this a stored connection row on /sessions?
  Show provider, scope count, connection state, and creation time.
  Do not show No connection or an account identifier.

Does an agent require a service without a stored connection?
  Show No connection in its Connections tab or consent service prompt.

Is this a permission group?
  Show name, description, required/optional control, and service names.
  Do not add risk indicators or raw scopes.
```

`activeGrantCount` counts UserGrant records, not permission sets.
Connection states derive from existing data, not guessed scope coverage.
Tool approvals use their server-provided risk. An absent risk uses “Risk not rated”.

## Choose text, surfaces, and assets

| Content | Semantic role |
| --- | --- |
| Heading or main text | `text-foreground` |
| Card text | `text-card-foreground` on `bg-card` |
| Supporting text | `text-muted-foreground` |
| Primary action text | `text-primary-foreground` on `bg-primary` |
| Status or risk | The matching semantic role, plus a visible label |
| Control boundary | `border-border` |
| Decorative separator | `border-border-soft` |
| Focus indicator | `ring-ring` |

Use `font-display` for Zalando Sans Variable, `font-sans` for Inter Variable, and `font-mono` for JetBrains Mono.
Keep fonts in `web/public/fonts/` and outlined artwork in `web/public/brand/`.
Use Wordmark and safe Avatar fallbacks instead of remote images.

## Choose spacing and motion

Use the central spacing and radius tokens through owned component variants.
Keep labels close to their fields and give separate actions a clear gap.
Do not add obsolete padding or size props to preserve an old example.

Use compact Table rows for console lists.
Below the small breakpoint, stack labels and values without hiding actions.
Preserve usability at 320 px and 200% zoom.

Use CSS-only feedback at 120–200 ms with ease-out.
Do not animate page entry or add decorative card movement.
Under reduced motion, remove movement while preserving feedback.
See [TOKEN_GUIDE.md](TOKEN_GUIDE.md) and [MOTION_GUIDE.md](MOTION_GUIDE.md).

## Choose the owner

Application components own domain composition and obtain UI copy from `@copy`.
Primitives receive strings through props and do not import the application catalogue.
Universal patterns belong in the design system, with stories and behavior tests.

Every component story needs light and dark accessibility checks and reviewed visual baselines.
Principle XI requires WCAG 2.1 AA. Feature 047 targets WCAG 2.2 AA.
See [COMPONENT_PAIRING_GUIDE.md](COMPONENT_PAIRING_GUIDE.md), [COMPOSITION_PATTERNS.md](COMPOSITION_PATTERNS.md), and [COMMON_MISTAKES.md](COMMON_MISTAKES.md).
