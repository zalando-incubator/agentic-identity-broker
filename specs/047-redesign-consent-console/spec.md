# Feature Specification: Consent UI v2 — Redesign End-User Consent and Console

**Feature Branch**: `047-redesign-consent-console`

**Created**: 2026-09-25

**Status**: Requirements revised through the approved canonical-reference decision on 2026-10-06. `tasks.md` and `cutover-inventory.md` record implementation and observed verification. The feature uses normal repository CI, not a separate release-approval process.

**Input**: Redesign the end-user consent frontend as a calm, fast security tool. Preserve existing capabilities and authorization behavior. The 2026-10-04 approved UX amendment replaces presentation choices without backend, API, or persistence additions.

## Clarifications

### Session 2026-09-25

- Q: If the system cannot save an event to the new activity history, should the user's action still complete? → A: Preserve the action's normal outcome. Record the activity-storage failure in operational logs. The user-facing history can have gaps. Existing security audit logging remains required. *(Superseded 2026-09-26: activity history left this feature.)*

### Session 2026-09-26

- Deliver the full redesign in one cutover. Do not use implementation phases, feature flags, or backwards-compatibility layers.
- Migrate all consumers together. Remove obsolete components, tokens, aliases, fonts, animations, and caches in the same release.
- Principle XI defines design-system and accessibility requirements, not an aesthetic. An accepted ADR must authorize the visual-direction change.
- Keep current guidance authoritative until that decision. Update it for the accepted direction before implementation, and preserve historical feature decisions.
- Q: How should the agent verification badge work, given that verification data exists only during an authorization request and administrator-registered agents never have a verified domain? → A: The consent view shows “Verified domain: host” for validated CIMD, “Registered by your administrator” without CIMD metadata, and “Unverified” with the localhost banner only for CIMD requests without domain trust. Console views show no verification column or badge.
- Q: Where should the consent screen show exact scope strings, given that the existing response lists scopes per service for the whole agent and not per permission group? → A: Nowhere new. Raw scopes are usually not meaningful to users, so permission groups use their human-readable name and description. Do not add scope strings to any view that does not show them today.
- Q: Where should “last use” appear, given that existing summaries have no last-use timestamp? → A: Nowhere. This feature covers visual and UX changes only. Remove the activity endpoint, `/activity`, Activity Events, last use, the `required_scopes` session field, and the Missing scopes connection state. Keep `/settings` and the command palette as frontend-only UX. AS-16, FR-023, SR-004, API-002, and API-003 are retired, and their identifiers are not reused.
- Q: If someone sets a default approval persistence in `/settings`, what should the tool-review accent action do? → A: Descope the saved approval-persistence default. `/settings` offers only the theme. Existing once, session, and permanent approval choices keep their current behavior, and Approve once stays the sole accent action.
- Q: Should permission groups on the consent screen show a risk indicator, given that permission sets have no risk rating in the existing API? → A: No. Permission groups show name, description, required or optional control, and services. Tool approvals keep their server-provided risk level.

### Session 2026-09-27

- Q: How should the delegation list show granted access, given that the existing list response has no service names? → A: Do not add backend features. Service names stay one click away in the agent detail. The later count clarification in this session removes the count column.
- Q: What should the delegation status filter show, given that the existing list excludes expired grants? → A: Remove the status column and status filter. The list contains only unexpired delegations, and a row whose expiry passes while the page is open disappears.
- Q: Where can “No connection” appear, given that `/api/third-party/sessions` lists only stored sessions? → A: Only where an agent requires a service the user has not connected: the agent's Connections tab and the consent view's service prompt. `/sessions` lists stored connections only.
- Q: What should the UI show for an agent's publisher or a connection's account, given that no existing response has either field? → A: Neither. Show no publisher or account row and never imply that a publisher identity was checked.
- Q: Can the user remove previously granted access during re-consent? → A: No. Already-granted groups are read-only in the decision view; the console detail view changes or removes them. The duration choice starts from the existing grant's validity, and Allow applies the chosen duration to the whole grant.
- Q: The OpenAPI description of `GET /api/third-party/sessions` documents `{data: [ThirdPartyServiceWithSession]}`, but the handler, its integration test, and the browser client use `{data: {sessions: [UserSessionSummary]}}`. How should the drift be resolved? → A: Correct the OpenAPI documentation to the existing handler response. This documentation-only correction changes no runtime behavior and is the stakeholder confirmation required by Principles IV and X.
- Q: Runtime verification showed that `activeGrantCount` counts UserGrant records, not permission sets. How should the Agents table handle the missing count? → A: Remove the count column. Show the agent, expiry, View, and confirmed Revoke. Add no per-agent requests or API changes.
- Q: May the implementation correct verified existing consent-response documentation mismatches in addition to the session-list correction? → A: Correct the documentation to match the current handlers. Do not change endpoints, runtime responses, response fields, persistence, or authorization behavior.

### Session 2026-09-28

- Q: How should legacy journeys handle raw scope badges and an initially enabled Save button, which conflict with the approved redesign? → A: Use the redesigned behavior. Replace obsolete presentation assertions with permission-group and dirty-only-Save coverage. Make an explicit edit before saving in the CSRF journey, while preserving its security and storage assertions. Move embedded selectors into page objects and migrate consent-state fixtures to the canonical draft envelope.
- Q: Cold production measurement found 7.30 seconds with uncompressed assets, and the retained-stack decision graphs measured 164.3/155.9 kB gzip. Which performance contract should the UI-only cutover use? → A: Use measured-stack limits: 170 kB gzip and 5 seconds on cold Slow 4G. Add compressed static delivery and reduce code-loading delays without changing API, authorization, or persistence behavior.

### Session 2026-10-02

- Q: Must the redesign integrate current main's consent-state transport? → A: Integrate main and migrate every connection entry point to its existing form POST and tab-local `sessionStorage` flow. Preserve selections, duration, custom date, and the original return URL. Keep one path without legacy URL-state support or compatibility shims.
- Q: May documentation-only corrections cover existing session-detail/termination responses and approval error statuses? → A: Correct the documentation to current runtime. Do not change endpoints, responses, persistence, or authorization behavior.


### Session 2026-10-04 — approved presentation amendment

The stakeholder approved five decisions: Agents and Connections use cards by default with a list toggle; Motion is allowed only on console routes; consent changes its primary action to “Connect {Service} to continue” while a selected service lacks a connection; `/delegations` and `/sessions` become `/agents` and `/connections`; consent shows “Signed in as”. These choices amend the active presentation requirements below and [accepted ADR 038](../../adrs/038-design-system-rebuilt-on-shadcn-radix.md). The dated earlier clarifications remain historical. They do not authorize old routes, tables, a theme-only Settings page, or CSS-only console motion.

### Session 2026-10-05 — approved navigation-protection budget

The stakeholder approved a 190-kB gzip limit per decision graph to retain React Router's standard unsaved-navigation blocker. Measured graphs before this amendment were 183.3 kB for consent and 173.7 kB for approval. This replaces the 170-kB limit. The five-second cold-consent target and decision-route isolation remain unchanged.


### Implementation alignment — 2026-10-05

The active requirements below were updated to describe the implemented branch. These updates record implementation facts and add no stakeholder decision. The dated entries above remain historical.

- The approved route rename also moves the broker callback's provider-error and missing-return redirects from `/sessions` to `/connections`. `api/enduser/openapi.yaml` documents this target. The provider-facing callback parameters, the state token, and the success redirect do not change.
- Consent and agent-detail Connect use the existing form POST with a tab-local draft when selections, duration, or the return URL carry state. A clean default draft and stored-connection Reconnect use the existing same-origin GET authorize link.
- Revocation is server-confirmed. Only the affected entity shows a busy state, and it leaves the collection after the server accepts the request.
- Connection state includes Unavailable for a failed read or an unconfirmed refresh failure. The No connection badge appears only in the agent-detail rail.
- A saved per-page view choice takes precedence over the list default above twelve items.

### Session 2026-10-05 — follow-up presentation review

The stakeholder superseded the earlier consent branding/account placement and collection-preference presentation: use the full wordmark grouped above consent, no Powered by footer, agent icon/text horizontally together, and Signed in as once outside beneath the card. Without CIMD metadata use Registered agent without inferring a registrant. Use a square bold single-letter favicon. Agent details centralizes connection identities/status/actions in Connections and makes no registrant claim. Explicit global collection-view changes reset earlier page choices and outrank the initial density fallback. Search collapses on outside blur while retaining and indicating its query. Approval header/tabs stay fixed, remembered tools stay below them, and tab contents use a short opacity fade. Connection cards group identity/status, scope/date metadata, and actions with a responsive 160 px minimum. Authorization, principal, API, callback and provider-imagery policies remain unchanged.

### Session 2026-10-06 — canonical visual references

The user reduces pixel references to light mode at 1280 × 720. [Accepted ADR 040](../../adrs/040-canonical-light-visual-reference-gate.md) supersedes only ADR 038's visual-reference matrix. Eight route/context variants require `_light.png` references. Storybook captures and compares PNGs only in desktop `storybook-light`, with an effective light theme and an actual 1280 × 720 viewport. Intentional dark or narrow overrides keep behavioral and accessibility checks without pixel references. All six theme/viewport projects, dark and responsive support, and the 190-kB decision budget remain unchanged.

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Decide on an Agent Request (Priority: P1)

A user arrives from an authorization request and sees one compact card with the agent, requested access, duration, signed-in identity, and any risk signal. The decision footer remains visible during consent and re-consent.

**Why this priority**: Consent is the point where the user grants security-sensitive access.

**Independent Test**: Start an authorization request, review its permissions, connect a required service if needed, and allow or deny without visiting the console.

**Acceptance Scenarios**:

1. **AS-01**: **Given** a first-time request, **When** the user opens `/agents/:id`, **Then** the decision header shows “Signed in as {name}” above a 480 px consent card. The card shows the session-derived Agent Origin Label and any localhost or unverified risk. Required permission groups appear before optional groups in an inset panel. Only multi-service groups have a service disclosure. The duration select, Deny, and Allow stay in the footer; at 1280 × 720 with three groups and a risk callout, Allow is visible without scrolling. If a selected service is unconnected, the same primary slot reads “Connect {Service} to continue” until the preserved callback returns. No permission-group risk rating or per-group raw scopes appear.
2. **AS-02**: **Given** an active grant, **When** the user returns through another request, **Then** only new access needs a decision. Existing groups remain checked and read-only, with a “Granted” hint rather than a status badge. The duration starts from the grant's current validity. Continuing preserves prior groups and never widens access without selection.
3. **AS-03**: **Given** a selected duration and an authorization session, **When** the user allows, **Then** the selected permissions and duration take effect and the existing safe continuation resumes. **When** the user denies or the session expires, **Then** no new grant is created and the user gets a clear outcome without an unsafe redirect.

---

### User Story 2 - Review a Tool Call (Priority: P1)

A user reviews a pending tool call on `/approvals/:id`. They can approve this call once, approve a defined scope for later calls, or deny it.

**Why this priority**: Tool approvals can authorize immediate, potentially sensitive actions.

**Independent Test**: Open a pending approval and resolve it with each existing persistence choice. Check that the arguments and exact remembered scope are visible before confirmation.

**Acceptance Scenarios**:

1. **AS-04**: **Given** a pending request, **When** the user opens its review link, **Then** the decision view identifies the tool, agent, acting user, server-provided risk, arguments, and exact approval scope before a decision. For six or fewer top-level arguments, a two-column key/value list is visible; “View JSON” opens the raw payload in a dialog. Approve once is the sole accent action; remembered choices stay available behind explicit decision controls.
2. **AS-05**: **Given** a pending or resolved approval, **When** the user approves once, chooses an existing remembered duration and scope, or denies, **Then** only that permitted decision is recorded. A resolved or expired approval instead shows its outcome and cannot be submitted again.

---

### User Story 3 - Find and Revoke an Agent (Priority: P2)

A returning user finds and revokes an unexpired agent grant on `/agents`. Cards are the default; a list toggle remains available.

**Why this priority**: Users need to see active access before they can control it.

**Independent Test**: Find an agent by name, inspect it, and revoke its grant with confirmation.

**Acceptance Scenarios**:

1. **AS-06**: **Given** several agent grants, **When** the user opens `/agents` or searches by name, **Then** cards show each unexpired agent, duration or expiry, and the `lastModifiedAt` date. Each card is a link to `/agents/:id`, without a separate View control. Confirmed Revoke has a visible boundary. A list toggle presents the same data without a count or verification field. Without a saved page choice, the view defaults to a list above twelve items.
2. **AS-07**: **Given** no grants or a revoked grant, **When** the user opens or returns to `/agents`, **Then** an illustrated empty state explains agent access. After a confirmed revoke, only that entity shows a busy state until the server accepts the request, and then it leaves the collection. On failure, that entity stays and unrelated changes are preserved.

---

### User Story 4 - Change an Agent Grant (Priority: P2)

A returning user reviews an agent's permissions and connections on `/agents/:id`, changes selectable access, and saves only intentional edits.

**Why this priority**: Access needs a manageable lifecycle after initial consent.

**Independent Test**: Open an existing grant, edit an optional permission or duration, cancel and save in separate runs, and revoke from the visible header button.

**Acceptance Scenarios**:

1. **AS-08**: **Given** an existing agent grant, **When** the user opens it outside an authorization request, **Then** the console detail shows identity, origin, saved expiry and relative change time. The 8 + 4 grid contains an editable permissions card, an inline duration selector, and a rail with Connections and configured About links. Required groups remain locked. A sticky Cancel and Save changes bar appears only after an edit. Cancel discards the draft. Save persists it and reports success through a toast after server acceptance. No tabs divide permissions and connections.
2. **AS-09**: **Given** an agent with active access, **When** the user uses the visible `destructive-outline` Revoke access button in the header, **Then** the confirmation names the agent and explains the effect. Confirming revokes only that user's grant; cancelling preserves it.

---

### User Story 5 - Manage Connections (Priority: P2)

A returning user reviews third-party services and reconnects, refreshes, or disconnects without losing the existing authorization flow.

**Why this priority**: A grant is only useful when the required service connection remains usable.

**Independent Test**: Inspect connection states, complete a service authorization callback, refresh a supported session, and disconnect after reading the warning.

**Acceptance Scenarios**:

1. **AS-10**: **Given** stored usable, expired, and unusable connections plus an agent that requires an unconnected service, **When** the user opens `/connections` and the agent's Connections rail, **Then** each stored connection appears as a card or list row with its truthful state, creation time, and scope count. A scope popover shows only that connection's existing scopes. A contextual Reconnect, Refresh, or Retry action appears when supported. The unconnected service appears as No connection with Connect only in the agent rail. The consent view marks it with a warning and an inline Connect action.
2. **AS-11**: **Given** a connection that needs attention, **When** the user reconnects, completes the existing authorization callback, refreshes a supported session, or disconnects, **Then** the visible state updates without changing token semantics. Before disconnect, the user sees that broker disconnection does not revoke provider-side tokens.

---

### User Story 6 - Triage Approvals (Priority: P2)

A returning user handles pending tool requests and revokes standing allow or deny decisions on `/approvals`.

**Why this priority**: Pending requests are time-sensitive; remembered decisions need ongoing user control.

**Independent Test**: Select an inbox row, decide in the fixed detail panel, switch to `/approvals/remembered`, and revoke a standing decision. Observe new pending requests without reloading or row expansion.

**Acceptance Scenarios**:

1. **AS-12**: **Given** pending requests and remembered decisions, **When** the user opens `/approvals`, **Then** fixed-height pending rows show tool, agent, risk, and request age; when less than one hour remains, the row shows the remaining minutes instead. Selecting a row opens a stable detail panel with arguments and a pinned Deny or Approve once decision bar. Explicit caret choices offer session approval, Always approval with scope editing and confirmation, and Always deny. No choice expands the list row. `/approvals/remembered` shows always-allow and always-deny decisions with a filter and confirmed Revoke. The panel and `/approvals/:id` share decision content, and only a confirmed user action changes authorization.
2. **AS-13**: **Given** an open console and a new pending request, **When** it arrives, **Then** the queue and sidebar count update within 15 seconds without reloading. Assistive technology announces the change without taking focus. A regular refresh reads only the acting user's pending list; the gateway-wide long-poll is never exposed.

---

### User Story 7 - Use the Console in Either Theme (Priority: P2)

A returning user moves between Agents, Connections, and Approvals in a three-item console sidebar. Settings and appearance are in the user menu.

**Why this priority**: Navigation and legibility affect every self-service task.

**Independent Test**: Collapse the sidebar, switch themes, navigate all existing routes, and reload in a different system theme.

**Acceptance Scenarios**:

1. **AS-14**: **Given** a new or returning browser, **When** the user opens a console page, collapses the sidebar, chooses light, dark, or system in the user menu, and reloads, **Then** the selected theme persists per browser, follows later system changes in system mode, and never flashes the wrong theme. Controls and scrollbars match the theme.
2. **AS-15**: **Given** either theme and a narrow or zoomed viewport, **When** the user uses keyboard navigation, **Then** the cropped AIB wordmark, visible focus, readable labels, and non-scrolling page width remain available. The consent and approval decision actions stay visible or pinned. Reduced motion removes movement and icon drawing without removing necessary color or opacity feedback.

---

### User Story 8 - Choose Appearance in Settings (Priority: P3)

A user chooses appearance and the default collection view at `/settings/appearance`, opened from the user menu or command palette.

**Why this priority**: A settings page gives the theme choice a stable home beyond the user menu.

**Independent Test**: Change the theme and default grid/list preference in `/settings/appearance`, reload, then open an approval and a collection.

**Acceptance Scenarios**:

1. **AS-17**: **Given** browser defaults, **When** the user chooses a Light, Dark, or System preview tile and a default collection view at `/settings/appearance`, **Then** both preferences persist per browser and apply immediately. Appearance shows setting rows, not a one-item category list. It offers no approval-persistence default; approval decisions still require explicit action.

---

### User Story 9 - Jump to a Record (Priority: P3)

A user opens a command palette to find an agent, connection, or approval, or change the theme.

**Why this priority**: Direct navigation reduces work for frequent users without hiding normal navigation.

**Independent Test**: Open the palette by keyboard, search for each record type, navigate, and change the theme.

**Acceptance Scenarios**:

1. **AS-18**: **Given** an open console, **When** the user opens the palette, searches for a record, chooses a result, or toggles theme, **Then** an agent result opens `/agents/:id`, a connection result opens `/connections`, a pending approval opens `/approvals/:id`, and the Appearance entry opens `/settings/appearance`. A theme entry applies the theme. Results and keyboard interaction remain scoped to the acting user.

### Edge Cases

- No existing response identifies an agent's publisher. Show no publisher field and do not imply that a publisher identity was checked.
- If a consent request has no CIMD metadata, show the neutral Registered agent fallback without inferring who registered it. Reserve Unverified for a CIMD request without domain trust, such as a localhost client.
- Outside an authorization request, no existing response supplies verification data. Console views show no Agent Origin Label rather than a guessed or uniform one.
- If a service or agent logo is only available at a third-party origin, show a local fallback instead of silently loading that image.
- Permission sets carry no risk rating, so permission groups show no risk indicator. If a tool approval has no server-provided risk level, show a neutral “Risk not rated” label. Never infer a security guarantee from a scope or tool name.
- Never show an expired grant as active. The existing delegation list excludes expired grants; if a listed grant's expiry passes while the page is open, remove its row.
- If a user's previous grant is unchanged, do not prompt for previously granted permissions. Preserve the existing safe continuation and do not enlarge the grant.
- If a third-party connection is missing, a refresh token expires, or refresh fails, show the correct reconnect path. Do not label an unusable session “Connected”.
- If a pending approval expires or another session resolves it, stop offering actions and show its current state. At the expiry time, the selected detail panel removes its actions without a refetch. The row leaves the inbox at the next pending-list refresh.
- If a requested service connection interrupts consent, preserve the current selections and authorization session through the existing callback flow.
- If a user denies a consent request, leave existing grants unchanged. Do not construct an unverified redirect or claim that provider-side tokens were revoked.
- If supplied text contains markup, escape it. Single-line names truncate with a tooltip; multi-line descriptions show a “More” toggle only when measurement proves that text is clipped. No control expands a collection row or card in place.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: Route `/` to `/agents`; use `/agents`, `/agents/:id`, `/connections`, `/approvals`, `/approvals/remembered`, `/approvals/:id`, and `/settings/appearance` with no aliases or redirects from the retired view paths. Preserve authorization, grant, connection, approval, and provider callback behavior. Reconnect from `/connections` returns to `/connections`; Connect in consent or agent detail returns to the initiating `/agents/:id` page. The broker callback sends provider errors and a missing return URL to `/connections` instead of `/sessions`. Do not change the provider-facing callback parameters or state token (AS-01–AS-18).
- **FR-002**: Use the same `/agents/:id` address for a focused decision view when an authorization session is present and a console detail view otherwise. Decision views and `/approvals/:id` have no sidebar (AS-01, AS-04, AS-08).
- **FR-003**: Show the full local black AIB wordmark in light mode and white in dark mode, grouped above focused decisions. Keep supplied wordmarks and sidebar compact artwork self-hosted; use a square, bold outlined single-letter favicon for small-size readability. There is no Powered by footer (AS-01, AS-04, AS-14).
- **FR-004**: Use self-hosted Zalando Sans for headings, Inter for body text and tabular dates/counts, and JetBrains Mono for identifiers and scope strings. Console text starts at 14 px, consent at 15 px; use a 12/13/14/16/20/24 px scale. Remove Crimson Pro and Manrope (AS-01, AS-14, AS-15).
- **FR-005**: Use tinted neutral OKLCH surfaces, a stronger blue primary accent, soft semantic status colors with text or icons, and three distinct border roles: subtle edges and dividers, buttons and popovers, and form controls meeting 3:1. Use `--primary-soft` for selection. Provide 120/160/200 ms feedback and up to 320 ms emphasis. Console routes may use Motion; decision routes stay CSS/SVG-only. Reduced motion removes movement, not useful color or opacity feedback. One primary accent action appears per view (AS-01, AS-04, AS-14, AS-15).
- **FR-006**: Console content uses a centered 1120 px maximum, a 12-column grid, and 24 px side padding (16 px under 768 px). The 240 px sidebar (56 px collapsed) contains only Agents, Connections, and Approvals. Its header has a compact icon collapse control with the Ctrl/⌘ B shortcut. Search is a quiet row that opens the command palette (Ctrl/⌘ K). The user menu holds Settings, the theme choice, and a Documentation link. `PageHeader` has a title, count, optional purpose and a right-side actions slot; collection pages put the icon `CollectionToolbar` there. A stale-count warning uses a tooltip, not a line that changes sidebar height (AS-06, AS-12, AS-14).
- **FR-007**: Show the acting user as Signed in as {name} once outside and beneath the 480 px consent card. Inside, place the agent icon beside its name/request text and show the session-derived origin below. Use Verified domain: host only for validated CIMD domain metadata, Registered agent without CIMD, and Unverified for CIMD without domain trust. Show a different redirect host on the origin line and preserve localhost/unverified risk signals. About retains descriptions and configured external links. Infer no registrant or publisher identity (AS-01).
- **FR-008**: In the consent card's inset `PermissionPanel`, list required groups first with locked checkboxes and a Lock icon plus “Required”; optional groups need no badge. Show permission names and descriptions. A “Granted” hint identifies already granted groups. Only multi-service groups expose a service disclosure; a missing connection has an inline Connect action. With more than four groups, scroll the panel internally while the decision footer stays fixed. Do not assign risk or raw scopes to a permission group. A technical-details dialog can show existing client ID, redirect URI, and existing requested service scopes without inventing per-group scopes (AS-01, AS-02).
- **FR-009**: List required groups before optional groups and prevent users from disabling required groups or required services. Preserve optional permission-set and service selection, and existing validation (AS-01, AS-08).
- **FR-010**: Offer “Until I revoke it”, “30 days”, and “Custom date” in one duration select. Show the custom date in a popover. Preserve existing validity semantics and validate the date before submission (AS-03, AS-08).
- **FR-011**: On re-consent, compare requested permission-set and service selections with `granted_permission_sets`. Only new access needs a decision; already granted groups stay checked and read-only with a “Granted” hint. Preserve prior selections. Start duration from the existing grant's validity; a changed duration applies to the whole grant. Use the existing grant read; add no delta API (AS-02).
- **FR-012**: Pin a Deny (outline) and Allow (primary) footer in consent. If a selected service is missing, the same primary action reads “Connect {Service} to continue” and uses the existing provider flow. Restore draft selections, duration, custom date and original return URL after its callback, then show Allow. Allow resumes validated continuation. Deny creates or revokes nothing and makes no constructed redirect. Link to Agents for later changes (AS-01, AS-03).
- **FR-013**: Both approval detail contexts show tool, agent, acting user, server-provided risk, arguments, and exact decision scope. Show up to six top-level arguments as key/value rows; provide “View JSON” for the raw payload. For high and critical risk, tint the detail header; unrated risk stays neutral. Keep once, session and permanent persistence, scope-preview validation and resolved outcomes. Only Approve once is the accent action. The review also shows the request expiry and, when session identifiers exist, an on-demand Session context popover (AS-04, AS-05, AS-12).
- **FR-014**: The Agents collection uses a card grid by default with list toggle, shared name search, sort (name, recently changed, expiring soonest), and a per-page stored view choice. Without a saved page choice, it defaults to list above twelve items. Each whole card links to detail and shows logo or ID-derived local tint, name, expiry, and the `lastModifiedAt` date as “Last updated”, with a visible confirmed Revoke button. An expiry within seven days shows a warning icon and tint. It excludes expired grants, has no count or status column, and makes no per-agent count requests. Use an illustrated empty state and layout-matched skeletons (AS-06, AS-07).
- **FR-015**: Agent detail uses an 8 + 4 grid without tabs. The main column shows the shared `PermissionPanel` as a headed card and the shared `DurationSelect` directly in a duration card. Consent keeps its grey permission inset. The side rail lists each service requirement with its connection state and a Connect (No connection) or Manage (`/connections`) action. An About section follows only for a description or configured HTTP(S) links. The description clamps to three lines with a measured More toggle. The header shows a back link, Revoke access, stored origin with a neutral icon, saved grant expiry, and relative change time. Stored origin means client-URI hosts or administrator registration, not a verification claim. Show a sticky Cancel/Save changes bar only for an edited draft. Save success uses a toast after server acceptance. Keep required groups locked and optional groups editable (AS-08).
- **FR-016**: Show Revoke access as a `destructive-outline` header button. Confirmation names the agent, focuses Cancel and explains that revocation affects only the acting user's grant. Do not place a single action in an overflow menu (AS-09).
- **FR-017**: Connections uses an entity card grid with a list toggle. Stored connection cards show provider, a single truthful status badge, scope count with scope popover, creation date, and relevant contextual actions. Use an outline Reconnect, Refresh, or Retry action when supported; Disconnect uses `destructive-outline` with confirmation and dependent-agent warning. Sort defaults to needs-attention first, with recently connected and name as alternatives. A state filter offers Connected, Needs sign-in, Expired, and Unavailable. Preserve authorization, callback, refresh, and token semantics. Return from the provider to `/connections`, show the callback result as a toast, and replace the result URL with `/connections` (AS-10, AS-11).
- **FR-018**: Derive Connected, Needs re-authentication, Expired, Unavailable, and No connection only from existing session fields, refresh results and agent requirement status. Render the Needs re-authentication badge as “Needs sign-in”. A 409 or 502 refresh response is a rejected refresh and shows Needs sign-in with Reconnect. A failed read or other refresh failure shows Unavailable with Retry and never proves usable access. `/connections` lists stored sessions only. The No connection badge appears only in the agent-detail rail, for a service requirement without a stored session; consent marks that service with a warning and Connect. No Missing scopes inference or state is allowed (AS-10).
- **FR-019**: Disconnect text states clearly that broker disconnection does not revoke provider-side tokens. Preserve dependent-agent warnings and current callback error handling (AS-11).
- **FR-020**: `/approvals` is an inbox with fixed-height pending `EntityRow` items beside a sticky detail panel. Rows show tool, agent, risk, and request age, or the remaining minutes when less than one hour remains. The panel shows arguments, scope, and a pinned Deny/Approve once bar. Caret options expose session and permanent approval or permanent denial; permanent approval opens the scoped editor with Back and Confirm inside the panel. After a confirmed decision, remove the row and select the next request without changing list height. Under 1012 px, the panel is hidden and each row links to `/approvals/:id`. `/approvals/remembered` uses underline URL tabs, an All/Always allowed/Always denied filter in the `filter` query, scope popovers, and an outline Revoke with a confirmation dialog. Row actions do not expand inline (AS-12).
- **FR-021**: Refresh the existing user-scoped pending list regularly while the console is open. Show new approvals and sidebar count changes within 15 seconds without reloading. Announce arrivals and decision-state changes without taking focus; never use the gateway-wide long-poll for a browser (AS-13).
- **FR-022**: Support light, dark, and system themes. Remember the choice per browser, follow changes to the system theme when chosen, avoid the wrong-theme flash, and theme form controls and scrollbars (AS-14, AS-15).
- **FR-024**: Open Settings from the user menu and command palette at `/settings/appearance`. Appearance uses immediate Light, Dark, and System preview tiles and a default grid/list view preference. Hide the category menu while Appearance is the only category. Never save a default approval-persistence choice; each approval still needs an explicit action (AS-17).
- **FR-025**: Add a keyboard-accessible command palette (Ctrl/⌘ K) to find only the current user's agents, connections, and pending approvals. Agent results open `/agents/:id`, connection results open `/connections`, and approval results open `/approvals/:id`. The palette also opens `/settings/appearance` and changes theme (AS-18).
- **FR-026**: Escape all supplied text and CIMD metadata. Give single-line truncated names a tooltip. For multi-line descriptions, use a measured clipping check so “More” appears only when needed and never expands an entity row or card. Do not load third-party fonts, scripts, or images automatically. User-initiated external links and OAuth2 redirects remain functional (AS-01, AS-04, AS-15).
- **FR-027**: Collect user-facing copy in one place for later localization. Use second-person, present-tense, action-first wording for sentences and actions; keep proper names, technical scopes, status labels, and route titles accurate (AS-01–AS-18).
- **FR-028**: Deliver every redesigned route and capability in one cutover. Do not use implementation phases, feature flags, dual presentations, backwards-compatibility layers, or old-server fallbacks. Migrate every consumer and remove obsolete components, tokens, aliases, fonts, animations, and caches in the same release (AS-01–AS-18).
- **FR-029**: Preserve security, authorization, storage, principal-query and callback assertions in existing journeys. Update page objects when roles, labels, or test IDs change. Replace assertions about removed tables, View buttons, tabs and inline approvals with cards, panel decisions, and stable list bounds. The CSRF save journey edits before Save; callback fixtures keep the canonical bounded draft and selection-preservation assertions. Add acceptance and both-theme coverage (AS-01–AS-18).
- **FR-030**: Preserve the 2026-09-27 acceptance of ADR 038 and record the stakeholder's 2026-10-04 five-decision amendment there. The accepted design governs current guidance; do not describe it as an unaccepted proposal (AS-14, AS-17).
- **FR-031**: Keep the README high-level without a console section or route gallery. Keep route guidance in `docs/concepts/delegation-and-consent.md`. Keep light/dark route captures as test and review artifacts (AS-01, AS-04, AS-06, AS-08, AS-10, AS-12, AS-14, AS-17).
- **FR-032**: Use shared EntityCard and EntityRow anatomy: leading identity, name, one status slot, supporting content, deliberate metadata grouping, and at most two independent actions. Agent cards remain compact; connection cards use a responsive 160 px minimum and grow for actual explanations or wrapped metadata. Approval rows retain fixed geometry. No entity unfolds under the pointer (AS-06, AS-10, AS-12).
- **FR-033**: CollectionToolbar supplies search, sort, real facets and grid/list controls. Search expands on click or / and collapses when focus leaves its input/clear-control area without clearing its query; the collapsed icon identifies an active query. Escape clears/collapses and restores icon focus. Search, sort and facets remain URL state; only applied facets create chips. Page view wins, then explicit Appearance default, then initial density fallback (list above twelve records, otherwise grid). A global default change resets earlier page choices; later page toggles remain local. Remembered approval tools stay below stable shared tabs; tab changes fade content without retaining stale decisions (AS-06, AS-10, AS-12, AS-17).
- **FR-034**: Use owned Lucide icons with restrained hover/empty/success animation. Console routes use owned lucide-animated Bot, Connection, and ShieldCheck icons for navigation, empty states, and the connection callback, and Motion for list exit and reflow. Decision routes use CSS/SVG only, including the success check. Card-to-detail View Transitions run only when the browser supports them and reduced motion is off; otherwise navigation is direct. Enforce the no-Motion decision import boundary in the existing bundle check (AS-01, AS-04, AS-15).
- **FR-035**: Split each networked page into a data-owning container and a pure `*View` receiving data and callbacks. Render foundation, pattern and screen stories from shared fixtures without a network mock. Cover loading, empty, typical, dense, error, stale, long text and decision-specific states in both themes at 375, 768 and 1280 px; include keyboard flows, stable list bounds and consent footer fit at 1280 × 720 (AS-01–AS-18).
- **FR-036**: In the pending inbox at 1012 px and wider, J and K change row selection, Enter focuses detail, A chooses Approve once and D chooses Deny only for an actionable selected request. Ignore these shortcuts in editable controls, dialogs, menus, popovers, and the scope editor. Under 1012 px, rows link to `/approvals/:id` and the inbox shortcuts are off. Expired or resolved approvals do not offer actions, and permanent choices still require explicit scope preview and confirmation (AS-12, AS-13).

### Domain Model *(if applicable - document before API or database design)*

The redesign does not change the meaning or ownership of an Agent, UserGrant, UserSession, Permission Set, or ToolApproval. A consent decision combines the current authorization request, existing grant, selected services, and chosen expiry. It never promotes a third-party connection to a grant. The console calls a principal's unexpired UserGrant to one agent a delegation.

A connection state summarizes token usability from existing session fields, refresh responses, and agent requirement connection status. Agent Origin Labels derive only from the current authorization session. A verified-domain label describes validated CIMD domain metadata, not a verified publisher identity. Absent CIMD metadata means an administrator registered the agent. Console views carry no Agent Origin Label.

### API Requirements *(if applicable - design before database)*

- **API-001**: Keep existing end-user runtime contracts and the consent-grants read response unchanged. Preserve authorization sessions, third-party authorization/callback/refresh, scope preview, and approval decisions. Use current main's accepted consent-state transport. Approved documentation-only corrections align existing consent, session, and approval responses and statuses with their handlers (Clarifications, 2026-09-27 and 2026-10-02). They introduce no runtime changes.
- **API-004**: The gateway-only approval long-poll is not a browser data source. Refresh the existing acting-user pending-list response regularly for live browser updates.
- **API-005**: This feature adds or changes no end-user API endpoint, request or response field, JSON response, persistence, or OAuth2/token contract. Every screen uses existing end-user responses. Two HTTP-adapter changes support the cutover. The broker callback's provider-error and missing-return redirects move from `/sessions` to `/connections` with the approved route rename. The SPA handler adds the theme-script CSP hash, negotiated precompressed static assets, and decision-route preload headers. Correcting documentation to match an existing response is not a contract change.

### Security Requirements *(mandatory for security-critical features)*

- **SR-001**: Preserve acting-user authorization on all reads and mutations. Pending counts and command-palette results never expose another principal's data (AS-06, AS-13, AS-18).
- **SR-002**: Treat unverified agent identity and unrated tool calls as unknown, not safe. Never label a permission group as low-risk. Keep prominent localhost/CIMD warnings and prevent untrusted redirect or external resource loading (AS-01, AS-03, AS-15).
- **SR-003**: Keep grant, approval, and connection revocation scopes unchanged (AS-09, AS-11).

### Frontend/Design System Requirements *(if applicable - document before implementation)*

- **FE-001**: Use one owned shared design system for both shells. Principle XI requires semantic tokens, accessible stories and local assets without selecting an aesthetic. Accepted ADR 038 and its 2026-10-04 amendment govern the visual direction. Keep historical decisions as historical records; publish one current token and composition contract (AS-01, AS-14, AS-15).
- **FE-002**: Target WCAG 2.2 AA: text contrast at least 4.5:1, form-control contrast at least 3:1, complete keyboard paths, unobscured focus and announcements. Test all components and composed screen states in light and dark Storybook projects. Quiet card borders must not substitute for control boundaries (AS-01–AS-18).
- **FE-003**: At 320 px width and 200% zoom, neither shell has horizontal page scrolling. Decision footers stay available; collection cards and rows keep actions visible. At 1280 × 720, consent with three groups and a risk callout fits without scrolling (AS-01, AS-06, AS-15).
- **FE-004**: Supply ink-cropped local wordmark and mark artwork without remote font imports. The decision footer is local monochrome text, not a partner mark (AS-01, AS-04, AS-14).
- **FE-005**: Follow the eight design principles: visible decisions, decision-relevant facts first, stable collection bounds, no dead controls, recognizable controls, semantic color, shared entity anatomy and explanatory motion. Cards and rows hold at most one status badge and two actions. Risk remains text or icon backed, never color alone (AS-01, AS-06, AS-10, AS-12).
- **FE-006**: Use `--border-subtle` for card edges/dividers, `--border` for buttons/popovers and `--border-control` for checkboxes/inputs/radios. Set a separate `--primary-soft` selection surface; status badges use soft fills and a word or icon. Add contrast pairs to `contrast.test.ts` and review results in both themes (AS-01–AS-18).
- **FE-007**: Apply [ADR 040](../../adrs/040-canonical-light-visual-reference-gate.md) to pixel references only. Require light-mode route references at 1280 × 720 for all eight route/context variants. Storybook pixel checks require desktop `storybook-light`, effective light theme, and an actual 1280 × 720 viewport. All six projects retain interactions and blocking accessibility checks, including intentional dark or narrow overrides. Never approve references automatically (AS-01–AS-18).

### Key Entities *(include if feature involves data)*

- **Agent**: The named requester, its trustworthy origin information, optional governance and documentation links, and user-owned grants.
- **Permission Set and UserGrant**: A human-readable, named, and described group of allowed services with required or optional controls, and one user's selections and expiry. Existing selections remain part of delta re-consent. The console lists unexpired UserGrants as delegations.
- **UserSession**: A user's third-party connection, granted scopes, expiry and refresh capacity, and dependent agents. Its visible state must reflect usable access.
- **ToolApproval**: A user's pending or resolved tool decision with reviewed arguments, scope, risk, and once, session, or permanent persistence.
- **Appearance Preferences**: Per-browser theme (`aib.theme`), sidebar (`aib.sidebar-collapsed`), default collection view (`aib.collection-view-default`), and per-page grid/list choices (`aib.collection-view.agents`, `aib.collection-view.connections`). These choices never affect consent or tool decisions.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: In moderated first-time consent sessions, users decide within 30 seconds and can name each granted service and state what each selected permission set allows.
- **SC-002**: With a cold browser cache under the Chrome DevTools “Slow 4G” network preset, consent main content appears within 5 seconds. Each decision route’s entry and static JavaScript/CSS graph is under 190 kB gzip. Compressed public static delivery and code-preload hints may improve this path without API, authorization, or persistence changes.
- **SC-003**: Every screen passes WCAG 2.2 AA checks in both themes. Automated component checks find no text below 4.5:1 contrast; every flow works by keyboard and announces new approvals, decisions, and toasts.
- **SC-004**: At 320 px width and 200% zoom, 100% of routes remain usable without horizontal page scrolling.
- **SC-005**: At a 1120 px desktop content width, the Agents and Connections card grid fits three cards per row, with two near 768 px and one on narrow screens. Without a saved page choice, a collection above twelve entities defaults to list; switching views changes no grant or session data.
- **SC-006**: Opening and using any application screen makes zero automatic requests to third-party origins. User-initiated OAuth2 navigation and external links still work.
- **SC-007**: No screen displays more than one accent-colored primary action. At least 90% of moderated participants identify the next action on the consent and tool-review screens without assistance.
- **SC-008**: The documentation describes all seven destination routes. Eight reviewed light-mode PNG references at 1280 × 720 cover all route/context variants, including both `/agents/:id` contexts. Both-theme and responsive behavioral/accessibility coverage remains required under ADR 040.
- **SC-009**: Existing journeys pass with their security, authorization, storage, and callback assertions preserved. Approved presentation changes have replacement behavior coverage, and every new acceptance scenario has a corresponding end-to-end journey.
- **SC-010**: The release contains every active AS-01–AS-18 capability with one presentation and no feature flags or compatibility paths. Every current-guidance reference in the plan matches the accepted visual decision. Historical records retain their original decisions.
- **SC-011**: At 1280 × 720, the Allow action is within the viewport with three permission groups and a risk callout. On narrower screens, the decision footer stays pinned.
- **SC-012**: In an approval interaction story, the pending list's bounding box stays unchanged while a user opens a row, changes persistence, previews scope, or confirms. Decisions preserve owner and expiry checks.
- **SC-013**: Screen stories cover loading, empty, typical, dense, error, stale and longest realistic content at 375, 768, and 1280 px in both themes. The consent and approval variants and the no-Motion decision bundle gate also pass.

## Assumptions

- Users use evergreen browsers. System theme, local appearance settings, and keyboard navigation are available.
- ADR 038 remains accepted from 2026-09-27. Its five stakeholder-approved presentation choices dated 2026-10-04 replace the earlier visual and route choices without changing backend contracts.
- A validated CIMD domain supports a domain-specific verification label only. Registered names alone do not prove a publisher's identity. Existing agent reads outside an authorization session expose no CIMD or verification data.
- Existing session data exposes service scopes, granted scopes, access-token expiry, refresh-token expiry, and refresh-token presence. Connection state derives only from these fields, refresh results in the current page, and agent requirement status.
- Existing agent and session summaries have no last-used timestamp. The redesign does not show last use and never substitutes a modification or connection time.
- External agent/provider logo URLs are not loaded during page rendering. A same-origin asset can be used when available; otherwise a local placeholder preserves identification without third-party requests.
- Deny cancels the current decision without creating or revoking a grant. Do not promise an OAuth2 error callback until an existing, validated cancellation path is identified; no new OAuth2 semantics are authorized here.
- The former `/delegations`, `/sessions`, and `/settings` view paths do not remain as aliases or redirects. `/agents`, `/connections`, `/approvals/remembered`, and `/settings/appearance` are the active destinations; the root routes to `/agents`.
- Theme is a per-browser preference, not a cross-device account setting.
- The gateway long-poll remains machine-only. The browser refreshes its existing user-scoped pending list to meet the no-reload requirement without changing that contract.
- The retained provider OAuth2 flow necessarily navigates to a third party when the user chooses it. The zero-external-requests goal applies to automatic page resources and background traffic, not user-directed external navigation.
- This feature excludes the admin port, tenant white-labeling, full localization, user activity history, backend endpoint or response-field changes, persistence changes, and changes to OAuth2 or token semantics.
