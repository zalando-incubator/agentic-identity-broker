# Composition Patterns

## Authority and example status

[ADR 038](../../../../adrs/038-design-system-rebuilt-on-shadcn-radix.md) was accepted on 2026-09-27 and amended on 2026-10-04.
These feature 047 recipes describe targets, not completed runtime or release validation.
Composition sketches describe responsibilities, not exact prop declarations.
Read the owned component source before implementation.

Keep universal components in the existing design-system categories.
Keep domain behavior, queries, navigation, and application copy outside primitives.
Applications obtain UI strings from `@copy` and pass them through props.

## Console layout

```text
Application route
  ConsoleShell: 240 px sidebar (56 px collapsed), Sheet below 1012 px, three destinations
    Wordmark + 28 px collapse icon, quiet search, Agents / Connections / Approvals
    User menu -> Settings, Appearance, Documentation
    1120 px centered main (24 px sides, 16 px below 768 px)
      PageHeader -> title, optional count, icon toolbar
      Route view -> collection or 8 + 4 detail
    Polite live region
```

ConsoleShell and PageHeader fetch no data. The application supplies identity, pending count, user-menu content, and search results.
The desktop sidebar background and divider span the full page. Its navigation and user menu stay pinned within the viewport.
The PageHeader purpose sentence is optional. Agents, Connections, and Approvals explain their purpose in their empty states.
The sidebar pending count hides at zero. Put stale information in a warning-icon tooltip; do not add a row that shifts navigation.

Use `/agents`, `/connections`, `/approvals`, `/approvals/remembered`, and `/settings/appearance` for console views.
Without an authorization session, `/agents/:id` uses the console for grant management.
Keep the same navigation landmarks in mobile Sheet and restore focus when it closes.
Persist `aib.theme`, `aib.sidebar-collapsed`, `aib.collection-view-default`, and `aib.collection-view.agents` / `.connections` as browser preferences. Never persist API data or a draft in localStorage.

## Focused decision layout

```text
Application decision route
  DecisionShell: no sidebar, full local wordmark grouped above the card
    Consent card: 480 px, three zones, pinned footer
      Who: agent icon beside request text, origin and risk callout
      What: permission panel and inline connection action
      Decide: DurationSelect, Deny, Allow or Connect, technical details link
    Standalone approval review: request, arguments, scope, pinned decision controls
    Signed in as: quiet account line outside and beneath consent
```

`/agents/:id` uses DecisionShell whenever an authorization-session parameter is present, including invalid or expired sessions.
`/approvals/:id` always uses DecisionShell. Neither route includes console navigation, Command, or Motion.
Keep route modules lazy and avoid importing console-only barrels. The bundle test must exclude Motion from decision routes.
Below 640 px, consent becomes full bleed and fixes its decision footer at the bottom.
At desktop widths, consent height follows its content up to the viewport-safe 616 px limit. Do not stretch blank space between the permission panel and decision footer.

## Entity collections and toolbar

Agent and connection collections use `EntityCard` grids or `EntityRow` lists.
Page choice wins, then an explicit Appearance default. Only without either choice does a collection above twelve entries use list, with cards otherwise. Changing the global default clears earlier Agents/Connections page choices and applies immediately; later page toggles remain local.
Use `repeat(auto-fill, minmax(320px, 1fr))` with 16 px gaps inside the 1120 px main container.
Agent cards and collection rows keep their specified heights. Connection cards use a 160 px minimum and grow for real explanations or wrapped metadata. The whole entity surface links to detail where available; action buttons sit outside the link layer.

Each entity shows a 40 px rounded-square icon, a semibold title, separate status and supporting slots, deliberate metadata grouping, and at most two actions. Agent and connection-card names use 16 px; other entity titles use 14 px. Agent expiry and changed metadata use quiet 12 px text. Connection explanations wrap without clipping; status stays in the identity header rather than floating among metadata.
Truncate long names with a tooltip, not a height-changing toggle. Use measured More only for clipped multi-line descriptions.

CollectionToolbar places search, sort, optional facet filter, and grid/list control in PageHeader.
Search expands to 240 px. `/` focuses it, and Escape clears and collapses it.
Sort offers Name, Recently changed, and Expiring soonest. Show a filter only for a real facet.
Mirror search, sort, and filter in the URL. Persist the view per page in browser preferences.
Loading uses layout-matched Skeleton; empty collections use an illustrated icon, a concept sentence, and an available next step.

Agent cards show identity, absolute expiry, relative “Changed” time from `lastModifiedAt`, and confirmed Revoke. A grant near expiry gets a warning. The card has one detail link and a separate Revoke button. Keep the name prominent and use consistent 12 px outer insets. The summary API supplies no descriptions, provider details, or permission counts. Do not make per-agent enrichment requests.
When a grant expires, remove it from active access. Connections cards show provider, scope count, state, relative connection age, and contextual action.
The quiet scope disclosure sits with the connection date, separate from the status badge. Its popover shows complete wrapping scope values. Do not add raw scopes to consent permission groups.

## Agent detail with an unsaved draft

```text
ConsoleShell
  Header: back to Agents, identity, saved expiry, relative change time, visible Revoke access
  Dirty draft only: top-sticky Cancel / Save changes bar, immediately below the header
  Main 8 columns: headed PermissionPanel card + DurationSelect with saved-versus-pending expiry
  Side 4 columns: Connections with status tooltips, About content, Technical details
```

Registered client URIs appear as exact technical values in the Technical details panel, not as header origin clutter. The panel shows the supplied agent ID, optional client ID and registered client URIs, and available grant ID and record timestamps. Omit absent optional values. These records make no domain-verification or legal-publisher claim. About keeps the description and configured human-facing links separate.
Required groups stay locked; optional selections and duration remain editable.
Required console groups show one lock cue and Required instead of a disabled checkbox. Editable groups use one 20 px checkbox. Service names and descriptions wrap without a hiding toggle; service choices stay visible. Required services are informational, and independent optional services use labeled pressed-state buttons instead of a second level of checkboxes.
Omit a single required service's redundant control, but retain independent optional service choices.
Connections is the single owner of service identity tiles, status warnings, and Connect/Reconnect controls on agent details. Permission rows retain service names and selection without repeated tiles or warning icons. Reconnect uses the same draft-preserving authorization callback as Connect. No console header infers a registrant from missing client URIs.
Move the side rail below the main content under 1012 px. Show DurationSelect directly in its card, as on consent.
Cancel restores the saved draft. Save waits for the authoritative server response and announces success through Sonner. Header expiry and change time use the saved grant. Duration edits show current access and access after saving; the selected custom date remains visible beside the selector. Custom dates use the local calendar and serialize to local midnight. Permission-only edits preserve the exact saved expiry timestamp.
The raised save bar sits above the editor, remains visible while scrolling, and must not obscure focus. Save changes is the only accent action while the draft is dirty. Internal navigation opens a discard confirmation with Keep editing focused; reload, tab close, and external departure use the browser's unsaved-change prompt. A deliberate provider connection carries the draft through its existing callback flow.
Revoke is a visible `destructive-outline` button that opens a named confirmation Dialog, not a one-item overflow menu.

## Consent selection and continuation

Use one consent card with three zones: who asks, what they get, and decide.
Group the full local wordmark above consent. Inside the card, place the agent icon beside its name and request sentence, with available origin information below. Place “Signed in as” once outside and beneath the card, without a second letter avatar or connector. There is no Powered by footer.
Keep a localhost or unverified-origin risk signal visible in a soft callout. A different redirect host also appears in the origin line.
Put documentation, governance, and agent-interface links in an About popover. Put client ID, redirect URI, and available raw scopes in a technical-details Dialog.

PermissionPanel places required groups first. Each row has a checkbox, name, description, and service icons.
Show Lock and “Required” after a required name. Do not give Optional or Granted a badge.
Already-granted groups remain checked, locked, and unchanged in decision mode. Show Granted in muted inline text.
Only groups with multiple selectable services get a chevron beside the name. Put Connect in the affected row if a selected service is missing.
After four groups, scroll the inset panel internally rather than pushing the decision footer away.
Do not infer a risk rating for permission groups or invent per-group raw scopes.

DurationSelect uses a one-line Select with Until I revoke it, 30 days, and Custom date. Open the date picker in a popover.
Start re-consent from the current grant's validity. A changed duration applies to the whole grant.
Keep earlier permission-set and service selections when submitting a new choice.

Preserve the canonical draft through tab-local `consent_state_id`, bounded form POST, and provider callback.
Keep the original return URL out of provider-facing state. Restore only a matching, unexpired record; then remove the ID from history.
If connection is needed, the primary control becomes “Connect {Service} to continue”; after return it becomes Allow.
Keep Deny as an `outline` action. Deny changes no grant and constructs no redirect.
Link inline validation errors to the affected row and summarize them in the footer.
Keep the action visible at 1280 × 720 with three groups and a risk callout.

## Tool review and approval inbox

Show the tool, agent, acting user, server-provided risk, arguments, and exact approval scope before a decision.
The Arguments section shows up to six top-level keys and values before a decision. A quiet View JSON disclosure sits in its heading and opens the complete payload in a Dialog. Additional arguments have a visible count. Session context uses the same quiet treatment beside Technical details.
If risk is absent, show neutral “Risk not rated”. High and critical risk tint the detail header, and critical risk includes a filled icon.

The pending inbox uses a five-column fixed-height list beside a seven-column sticky detail panel.
Pending rows reserve their main text width for the tool name, with up to two lines. Agent and risk use a secondary line, with request age, or the remaining minutes when less than one hour remains; the age stays available to screen readers. Pending rows stay 88 px tall. Selecting a row updates only the detail panel. The row and detail share a selection rail and explicit tool/agent identity.
Pin Deny (`outline`) and Approve once (`primary`) to the panel footer.
Their caret menus offer session and permanent choices. “Always…” replaces the panel body with a scope editor, Back, and Confirm.
On server-confirmed success, remove the row and select the next request. Below 1012 px, open `/approvals/:id` from a row.
At 1012 px and wider, focused inbox controls support `J`/`K` for selection, `A`/`D` for decisions, and Enter for panel focus. Show shortcuts in tooltips. Shortcuts do nothing with body, unrelated-control, editor, or overlay focus. Below 1012 px, the inbox shortcuts are off.

Remembered is an underline tab at `/approvals/remembered`, with All, Always allowed, and Always denied filters.
Keep the shared header and tab strip at the same position on both approval routes. Remembered-only search and outcome filters belong below the tabs, inside that panel. Tab contents enter with a 160 ms opacity fade; interactions stay available immediately and reduced motion retains the fade.
Remembered rows show the tool, agent, soft state badge, scope pattern with a full-value popover, and Revoke. They stay 88 px tall on desktop and 160 px below 768 px. The additional mobile space preserves the scope and action without crowding identity.
The standalone review page and inbox panel share one implementation. Resolved and expired requests show outcomes without actions.

## Confirmed revocation

The application opens Dialog from a visible `destructive-outline` button.
Dialog names the target, explains the effect, and pairs focused Cancel with a destructive confirmation.
It traps focus, supports Escape, and keeps background content inert.

After confirmation, mark only the affected record pending. Keep rollback data for that record.
On failure, restore it without overwriting unrelated changes. Reconcile affected data with the server on settlement.
On server-confirmed success, animate the card or row out and announce the result. Offer no undo without API support.
Grant creation, grant edits, and approvals never use optimistic success. Do not automatically replay security mutations after errors.

## Connection management

The application derives connection state from existing session fields and current authoritative refresh results.
The full precedence table is in the [data model](../../../../specs/047-redesign-consent-console/data-model.md).

`/connections` lists stored connections, so it never shows No connection.
That state belongs only to a missing required service in agent detail or the consent permission row.
Do not infer missing scopes or invent account and last-use metadata.

Reconnect uses the existing authorization flow. Refresh appears only when supported.
Disconnect requires a named confirmation and dependent-agent warnings.
Its copy states that broker disconnection does not revoke provider-side tokens.
Replace inline notices with server-confirmed toasts. A callback can pulse the affected card border once for 320 ms.

## Command search and appearance

Command uses cmdk for acting-user record search and navigation. A result never grants, approves, revokes, or preselects a decision.
Keep Settings in the user menu and command palette, not the sidebar.
`/settings/appearance` shows Light, Dark, and System preview tiles and the default collection view.
Store only browser theme, sidebar, and collection-view preferences; a change applies immediately without Save or a toast.
Use setting rows and subtle dividers, not a card per setting. Hide the category list until a second category exists.
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
Use 120–200 ms feedback and up to 320 ms emphasis. Console routes can use Motion; decision routes use CSS only.

Every component story requires both-theme accessibility checks. Reviewed pixel references cover eligible light 1280 × 720 cases only, under [ADR 040](../../../../adrs/040-canonical-light-visual-reference-gate.md).
Preserve keyboard paths, visible unobscured focus, and state announcements.
Principle XI requires WCAG 2.1 AA. Feature 047 targets WCAG 2.2 AA.
See [COMPONENT_PAIRING_GUIDE.md](COMPONENT_PAIRING_GUIDE.md) and [ACCESSIBILITY_GUIDE.md](ACCESSIBILITY_GUIDE.md).
