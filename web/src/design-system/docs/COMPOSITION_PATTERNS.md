# Composition Patterns

## Authority and example status

[ADR 037](../../../../adrs/037-design-system-rebuilt-on-shadcn-radix.md) was accepted on 2026-09-27.
These recipes describe the feature 047 component system.
Composition sketches describe responsibilities, not exact prop declarations or completed release validation.
Read the owned component source before implementation.

Keep universal components in the existing design-system categories.
Keep domain behavior, queries, navigation, and application copy outside primitives.
Applications obtain UI strings from `@copy` and pass them through props.

## Console layout

```text
Application route
  ConsoleShell
    Wordmark
    Navigation and pending count
    Sheet for mobile navigation
    Search slot -> application search -> Command
    User-menu slot -> DropdownMenu and ThemeChoice
    Main content
      PageHeader
      Route content
    Polite live region
```

ConsoleShell and PageHeader are presentational and fetch no data.
The application supplies navigation, search results, identity, pending count, and user-menu content.
PageHeader shows a title, one-line purpose, and only the action assigned by the route contract.

The console routes are `/delegations`, `/sessions`, `/approvals`, `/settings`, and `/agents/:id` without an authorization session.
The sidebar uses Agents, Connections, and Approvals navigation.
The user menu provides settings and theme access.
The sidebar collapse preference is browser-local and uses `aib.sidebar-collapsed`.

At narrow widths, Sheet contains the same navigation.
Preserve the skip link, navigation landmark, main landmark, Escape behavior, and focus return.

## Focused decision layout

```text
Application decision route
  DecisionShell
    Wordmark header
    Main content, centered and at most 640 px wide
      Identity and authoritative context
      Request details and choices
      One assigned accent action
      Next steps or resolved outcome
    Optional monochrome footer
```

`/agents/:id` uses DecisionShell whenever an authorization-session parameter is present.
An invalid or expired session stays a decision error.
`/approvals/:id` always uses DecisionShell, including resolved and expired results.
Neither decision route includes console navigation or search.

Keep route modules lazy. Import concrete component modules.
Do not re-export Table or Command from barrels that decision routes import.
This boundary protects the initial decision-route bundle from console dependencies.

## Dense collection with search

TanStack Table owns filtering and row state. Owned Table parts supply semantic presentation.
The console route supplies the data and a labeled Input for search.

```text
PageHeader
Search Input with visible label
Loading, error, empty, or content state
  Table
    TableCaption
    TableHeader -> TableRow -> TableHead
    TableBody -> TableRow -> TableCell
      TruncatedText for long metadata
      Non-accent row actions
```

The Agents columns are agent, expiry, and actions.
Show “Until revoked” for a null expiry and a monospace date otherwise.
When a row's expiry passes, remove the row.
View navigates to the agent detail. Revoke opens a confirmation Dialog.

Do not show count, status, or Agent Origin Label columns.
`activeGrantCount` counts UserGrant records, not permission sets.
Do not issue per-agent requests for counts.

Connection rows show provider, scope count, connection state, and creation time.
Pending approval rows appear before standing allow and deny decisions.
Their approval actions require explicit persistence and scope choices.

Below the small breakpoint, rows stack into label-value groups without hiding actions.
Preserve table relationships, readable text, and no horizontal page overflow at 320 px and 200% zoom.
At 1920×1080, the Agents table must show at least 12 rows.

## Agent detail with an unsaved draft

```text
ConsoleShell
  PageHeader and identity links
  DropdownMenu -> Revoke all access -> confirmation Dialog
  Tabs
    Permissions -> permission groups and duration
    Connections -> required services and truthful connection states
  Dirty draft only -> sticky Cancel and Save changes bar
```

React state owns the draft. Queries own remote data.
The console detail has no Agent Origin Label.
Required groups stay locked. Optional selections and duration remain editable.
Cancel restores the original draft. Save waits for the authoritative server response.

The sticky bar must not obscure focused controls.
Save changes is the sole accent action and appears only after an edit.
The overflow menu and Cancel use non-accent variants.

## Consent selection and continuation

Group required permissions before optional permissions.
Use Checkbox with Accordion for a name, description, selection state, and expandable service names.
Do not add risk indicators or raw scope strings.

On re-consent, show previously granted groups collapsed, checked, and read-only.
Only new access needs a decision.
Preserve prior permission-set and service selections in the submitted grant.

Use RadioGroup for Until revoked, 30 days, and Custom date, with DatePicker for a custom expiry.
Start re-consent from the existing validity and preserve it unless the user changes the duration.
A changed duration applies to the whole grant.

Keep the draft through the existing `consent_state` provider callback flow.
Allow is the sole `primary` action. Deny uses `secondary` and leaves the grant unchanged.
Deny shows a local outcome without constructing a redirect.
Invalid custom dates use linked inline errors before submission.

## Tool review

Show tool, agent, acting user, server-provided risk, and exact approval scope before a decision.
Use Accordion for the escaped argument block, with JetBrains Mono for technical values.
If risk is absent, show the neutral “Risk not rated” label.

Approve once is the sole accent action.
Approve and remember and Deny use non-accent controls.
Remembered decisions preserve the existing once, session, and permanent semantics.
Resolved and expired requests show their outcome without decision actions.

## Confirmed revocation

The application opens Dialog from a non-accent row action or DropdownMenu item.
Dialog names the target, explains the effect, and pairs secondary Cancel with destructive confirmation.
It traps focus, supports Escape, and keeps background content inert.

After confirmation, mark only the affected record pending.
Cancel matching reads and retain rollback data for that record.
On failure, restore that record without overwriting unrelated changes.
On settlement, reconcile affected lists, details, and counts with the server.
Announce success only after server acceptance.

Grant creation, grant edits, and approvals never use optimistic success.
Do not automatically replay security mutations after errors.

## Connection management

The application derives connection state from existing session fields and current authoritative refresh results.
The full precedence table is in the [data model](../../../../specs/047-redesign-consent-console/data-model.md).

`/sessions` lists stored connections, so it never shows No connection.
That state belongs only to a missing required service in agent detail or the consent service prompt.
Do not infer missing scopes or invent account and last-use metadata.

Reconnect uses the existing authorization flow. Refresh appears only when supported.
Disconnect requires a named confirmation and dependent-agent warnings.
Its copy states that broker disconnection does not revoke provider-side tokens.

## Command search and appearance

Command uses cmdk inside the console search composition.
The application supplies only the acting user's agents, connections, and approvals.
A selection navigates to an existing record view or changes the theme.
It never grants, approves, revokes, or preselects a decision.

ThemeChoice is a RadioGroup with caller-supplied Light, Dark, and System labels.
It appears in `/settings` and the user menu.
Changes apply immediately and use `aib.theme` in browser storage.
Settings contains no saved approval-persistence default.

## Feedback and shared query ownership

TanStack Query owns remote state over Axios. React owns draft and appearance state.
Queries are principal-scoped and clear on identity change or authentication loss.
Do not persist API data in browser storage or keep a second GET cache.

One console query refreshes `/api/approvals/pending` every 10 seconds while visible.
Sidebar and queue share its result and announce changes without moving focus.
Refresh after a decision and when focus returns.
Never call the gateway-wide `GET /api/approvals` from the browser.

Use Skeleton for initial loading, EmptyState for a successful empty result, and Alert or InlineError for errors.
Keep stale content visibly stale rather than converting a failed read to an empty list or zero count.
Use the theme-aware Sonner Toaster for polite server-confirmed results.

## Shared presentation and validation

Use semantic OKLCH roles from [COLOR_GUIDE.md](COLOR_GUIDE.md) in both themes.
Use local Wordmark artwork and safe Avatar fallbacks.
Fonts are self-hosted Zalando Sans Variable, Inter Variable, and JetBrains Mono.
Use CSS-only feedback at 120–200 ms with ease-out and no movement under reduced motion.

Every component story requires both-theme accessibility checks and reviewed visual baselines.
Preserve keyboard paths, visible unobscured focus, and state announcements.
Principle XI requires WCAG 2.1 AA. Feature 047 targets WCAG 2.2 AA.
See [COMPONENT_PAIRING_GUIDE.md](COMPONENT_PAIRING_GUIDE.md) and [ACCESSIBILITY_GUIDE.md](ACCESSIBILITY_GUIDE.md).
