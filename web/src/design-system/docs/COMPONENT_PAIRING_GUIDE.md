# Component Pairing Guide

## Authority and example status

[ADR 038](../../../../adrs/038-design-system-rebuilt-on-shadcn-radix.md) was accepted on 2026-09-27 and amended on 2026-10-04.
This guide describes feature 047 composition targets, not completed release validation.
Read owned source for exact props during implementation.

Use the existing design-system categories. Do not create another primitive library.
Applications obtain copy from `@copy`. Primitives receive labels and descriptions through props.
The `labels` values in these examples represent caller-supplied copy.

## Shell and page header

| Pair | Responsibility |
| --- | --- |
| ConsoleShell + PageHeader | Three-item navigation, 1120 px main container, title, muted count, icon toolbar |
| ConsoleShell + Sheet | Same navigation on mobile without a second implementation |
| ConsoleShell + Command | Acting-user record search in an application-owned slot |
| ConsoleShell + DropdownMenu + ThemeChoice | User menu with Settings and immediate appearance choices |
| DecisionShell + Wordmark | Focused consent or approval, local branding, no sidebar |

Shells fetch no data. PageHeader has no required purpose sentence and invents no page-level action.
Consent fits a 480 px three-zone card with a pinned decision footer; standalone approval review remains sidebar-free.
Keep Command and Motion outside decision-route import graphs and barrels.

## Collections, toolbar, and row actions

EntityCard and EntityRow share an icon, title, separate supporting, status and metadata slots, and at most two visible actions.
Agents and Connections use cards or a list. Page choice wins, then an explicit Appearance default, then list above twelve items or cards otherwise. Choosing the global default clears earlier page choices and applies to both collections; later per-page choices remain local.
Approvals use fixed-height EntityRow entries beside a separate detail panel. No decision step changes the list height.
Approval rows stack the tool name above secondary metadata instead of splitting the narrow inbox into horizontal columns. A selected row names and controls its matching detail panel.
Tool review keeps arguments visible and places the quiet View JSON disclosure in the Arguments heading, separate from the pinned decision actions.

| Collection | Visible content | Actions |
| --- | --- | --- |
| Agents | Name, expiry, relative change time from `lastModifiedAt` | Whole card opens detail with one link target; Revoke opens confirmation |
| Connections | Provider, scopes count, truthful state, relative connection age | Context action and confirmed Disconnect |
| Pending approvals | Tool, agent, risk, and request age, or remaining minutes in the final hour | Select row; decide in pinned detail panel |
| Remembered approvals | Tool, agent, remembered state, scope pattern | Confirmed Revoke |

The Agents list has no permission count, status filter, or origin badge. Never fetch per-agent counts.
Use 32 px outline row actions. Agent and connection collection actions use `destructive-quiet`, with `ShieldOff` and `Unlink`, respectively.
The whole card uses a link layer. Action buttons sit above it and never nest inside it.

CollectionToolbar places 32 px search, sort, optional facet filter, and grid/list controls in PageHeader. Remembered approval controls live below the shared Pending/Remembered tabs so header geometry stays fixed.
Search expands to 240 px. `/` focuses it and Escape clears and collapses it.
Mirror search, sort, and filters in the URL. Persist the grid/list choice per page and expose the default in Settings.
Render a filter control only when the page has a real facet. Put only applied facet chips below the header.
Search collapses when focus leaves its input/clear-control area without clearing the query; the collapsed icon indicates and describes an active query. Sort shows a menu check and a tinted icon for a non-default choice.
One-line titles use ellipsis and a tooltip only when measured as clipped. Multi-line descriptions show More only when clipped.

## Consent card and action group

The consent card shows who asks, what they get, and how long it lasts before the pinned decision footer.
The 480 px card uses one inset PermissionPanel with locked required groups, editable optional groups, and a row-level Connect action for missing services.
Keep the origin and any risk callout visible. Put client ID, redirect URI, and available raw scopes in a technical-details Dialog.
Show the verified agent domain and the redirect host when they differ. A loopback callback or unverified origin needs a visible risk warning.
Without CIMD metadata, show only a client ID that the API supplies. Do not invent a redirect URI or requested scopes.
After a confirmed Allow, draw the success check for 320 ms before a validated redirect. Do not delay the redirect under reduced motion.

```tsx
<div className="flex gap-3">
  <Button variant="outline" onClick={onDeny}>{labels.deny}</Button>
  <Button variant="primary" onClick={onContinue}>{labels.continue}</Button>
</div>
```

Keep Allow as the primary label. Disable it while selected connections are missing, show an explanation, and leave inline Connect actions available.
Keep the current permission and duration draft across the existing provider callback. Deny does not change an existing grant.
The buttons remain 40 px tall, with each taking half the footer width.

## Input, label, and error

```tsx
<div className="space-y-2">
  <label htmlFor={id} className="text-foreground">{labels.label}</label>
  <Input
    id={id}
    value={value}
    onChange={onChange}
    aria-invalid={Boolean(error)}
    aria-describedby={error ? errorId : hintId}
  />
  {error ? (
    <p id={errorId} role="alert" className="text-status-danger-foreground">{error}</p>
  ) : (
    <p id={hintId} className="text-muted-foreground">{labels.hint}</p>
  )}
</div>
```

Use unique IDs. Keep a visible label even when the input has a placeholder.
A form summary can link to invalid fields. It does not replace each field's error.
Do not move focus for background data updates.

RadioGroup represents one exclusive choice. Checkbox represents independent selections.
URL-backed underline Tabs switch views of one object. Do not style data-changing actions as tabs.

## Destructive action and confirmation Dialog

Agent and connection collections show Revoke and Disconnect as `destructive-quiet` buttons. The agent detail header keeps `destructive-outline`.
Use an overflow menu only when it holds at least three secondary actions; never hide a single Revoke item in a menu.
Keep Dialog outside a menu if a menu launches the confirmation.

Dialog names the target and explains the effect in one sentence. Focus Cancel initially and pair it with a destructive confirmation.
Keep the background inert. On cancellation, return focus to the trigger or a logical remaining control.
After a confirmed operation, animate the affected card or row out in the console and announce server-confirmed success.
Do not offer undo unless the API supports it.

## Status, metadata, and feedback

Badge uses a soft semantic background, colored text, and a 6 px dot or 12 px icon. It is 20 px tall with 6 px radius.
Put at most one badge after the name on the title baseline or in a fixed status slot. Do not place it in a wrapping badge row.
Use Connected, Needs sign-in, Expired, Unavailable, rated risk, Always allowed, and Always denied only in their applicable views.
Badges do not change data. A status tooltip can use a focusable trigger for keyboard guidance. A count can use a separate labeled popover trigger.
Every connection-state badge in the agent detail rail has an explanatory tooltip on hover and keyboard focus. Focus keeps its explanation through ancestor scrolling; Escape, blur, or an outside click dismisses it. Keep the badge informational; the separate Connect or Manage control performs the action.

Use Inter with tabular figures for dates and counts, relative times for recency, and absolute dates for expiry. Use JetBrains Mono for tool names, scope patterns, existing scope values, raw arguments, and IDs.
Do not invent last-use, publisher, account, or permission-risk fields. Do not add raw scopes to permission groups.

Skeleton matches the final layout's dimensions. EmptyState has an 80 px primary-soft tile with a 40 px illustrated icon, a concept sentence, and a next step where available. Console illustrated icons play once on entry and on hover, without movement under reduced motion.
Alert has a soft status background, subtle tinted border, 12 px padding, and a 16 px icon. Its inline form is a one-line field hint without a box.
Sonner toasts report results after server acceptance. They do not replace persistent error text.
Off-origin service images use local Avatar fallbacks without an external request.

## Spacing, motion, and accessibility

Use 4–12 px spacing inside controls, 16–24 px between components, and 32–48 px between sections.
Use 120–200 ms feedback and at most 320 ms emphasis with shared easing. Motion is permitted only on console routes.
Under reduced motion, remove movement and icon animation but retain opacity and color fades.

Every composition keeps visible, unobscured focus, keyboard access, and labels at 320 px and 200% zoom.
Every pattern and screen needs both-theme accessibility checks. Reviewed pixel references cover eligible light 1280 × 720 cases only, under [ADR 040](../../../../adrs/040-canonical-light-visual-reference-gate.md).
Principle XI sets WCAG 2.1 AA. Feature 047 targets WCAG 2.2 AA.

## Related guides

- [Composition patterns](COMPOSITION_PATTERNS.md): Route and mutation recipes
- [Decision trees](DECISION_TREES.md): Components and action variants
- [Color guide](COLOR_GUIDE.md): Exact semantic OKLCH contract
- [Token guide](TOKEN_GUIDE.md): Shared visual values
- [Accessibility guide](ACCESSIBILITY_GUIDE.md): Focus, labels, contrast, and announcements
