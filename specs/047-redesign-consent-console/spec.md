# Feature Specification: Consent UI v2 — Redesign End-User Consent and Console

**Feature Branch**: `047-redesign-consent-console`

**Created**: 2026-09-25

**Status**: Draft

**Input**: Redesign the end-user consent frontend as a calm, fast security tool. Preserve existing capabilities and authorization behavior. Revised 2026-09-26: visual and UX changes only, without backend, API, or persistence additions.

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

- Q: How should the delegation list show granted access, given that the existing list response has no service names? → A: Do not add backend features. Show the number of granted permission sets from the existing `activeGrantCount`. Service names stay one click away in the agent detail.
- Q: What should the delegation status filter show, given that the existing list excludes expired grants? → A: Remove the status column and status filter. The list contains only unexpired delegations, and a row whose expiry passes while the page is open disappears.
- Q: Where can “No connection” appear, given that `/api/third-party/sessions` lists only stored sessions? → A: Only where an agent requires a service the user has not connected: the agent's Connections tab and the consent view's service prompt. `/sessions` lists stored connections only.
- Q: What should the UI show for an agent's publisher or a connection's account, given that no existing response has either field? → A: Neither. Show no publisher or account row and never imply that a publisher identity was checked.
- Q: Can the user remove previously granted access during re-consent? → A: No. Already-granted groups are read-only in the decision view; the console detail view changes or removes them. The duration choice starts from the existing grant's validity, and Allow applies the chosen duration to the whole grant.
- Q: The OpenAPI description of `GET /api/third-party/sessions` documents `{data: [ThirdPartyServiceWithSession]}`, but the handler, its integration test, and the browser client use `{data: {sessions: [UserSessionSummary]}}`. How should the drift be resolved? → A: Correct the OpenAPI documentation to the existing handler response. This documentation-only correction changes no runtime behavior and is the stakeholder confirmation required by Principles IV and X.

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Decide on an Agent Request (Priority: P1)

A user arrives from an authorization request and needs to understand the agent, the requested access, and the duration before choosing Allow or Deny. The decision stays focused even when the same agent already has access.

**Why this priority**: Consent is the point where the user grants security-sensitive access.

**Independent Test**: Start an authorization request, review its permissions, connect a required service if needed, and allow or deny without visiting the console.

**Acceptance Scenarios**:

1. **AS-01**: **Given** a first-time request, **When** the user opens `/agents/:id`, **Then** a single-column view identifies the agent and its Agent Origin Label. It highlights localhost risks and orders required permission groups first. Each group explains access in plain language from its human-readable name and description and expands to its service names, without a risk indicator. The view offers duration choices, one primary Allow action, secondary Deny, and an explanation of what happens next.
2. **AS-02**: **Given** an active grant, **When** the user returns through another request, **Then** only new access needs a decision. Existing groups appear collapsed, checked, and read-only; the console detail view changes or removes them. The duration choice starts from the existing grant's validity. Continuing preserves prior groups and never widens access without the user's selection.
3. **AS-03**: **Given** a selected duration and an authorization session, **When** the user allows, **Then** the selected permissions and duration take effect and the existing safe continuation resumes. **When** the user denies or the session expires, **Then** no new grant is created and the user gets a clear outcome without an unsafe redirect.

---

### User Story 2 - Review a Tool Call (Priority: P1)

A user reviews a pending tool call on `/approvals/:id`. They can approve this call once, approve a defined scope for later calls, or deny it.

**Why this priority**: Tool approvals can authorize immediate, potentially sensitive actions.

**Independent Test**: Open a pending approval and resolve it with each existing persistence choice; review the resolved state.

**Acceptance Scenarios**:

1. **AS-04**: **Given** a pending request, **When** the user opens its review link, **Then** a focused view identifies the tool, agent, acting user, risk, arguments on expansion, and the exact approval scope before a decision. Approve once is the sole accent action; Approve and remember and Deny remain available.
2. **AS-05**: **Given** a pending or resolved approval, **When** the user approves once, chooses an existing remembered duration and scope, or denies, **Then** only that permitted decision is recorded. A resolved or expired approval instead shows its outcome and cannot be submitted again.

---

### User Story 3 - Find and Revoke an Agent (Priority: P2)

A returning user wants one dense view of their agent delegations and a safe way to revoke one.

**Why this priority**: Users need to see active access before they can control it.

**Independent Test**: Find an agent by name, inspect it, and revoke its grant with confirmation.

**Acceptance Scenarios**:

1. **AS-06**: **Given** several delegations, **When** the user opens `/delegations` or searches by name, **Then** a dense list shows each agent with an unexpired grant, its number of granted permission sets, and its expiry. Each row offers View and a confirmed Revoke action.
2. **AS-07**: **Given** no delegations or a revoked delegation, **When** the user opens or returns to `/delegations`, **Then** the empty state explains what a delegation is, or the revoked agent disappears from active access after confirmation. The user sees a clear result in either case.

---

### User Story 4 - Change an Agent Grant (Priority: P2)

A returning user reviews an agent's permissions and connections on `/agents/:id`, changes selectable access, and saves only intentional edits.

**Why this priority**: Access needs a manageable lifecycle after initial consent.

**Independent Test**: Open an existing grant, edit an optional permission or duration, cancel and save in separate runs, and revoke all access from the overflow menu.

**Acceptance Scenarios**:

1. **AS-08**: **Given** an existing agent grant, **When** the user opens the agent outside an authorization request, **Then** a console detail view shows identity, links, Permissions and Connections tabs, locked required groups, and editable optional groups. A sticky Cancel and Save changes bar appears only after an edit; cancel discards it and save persists it.
2. **AS-09**: **Given** an agent with active access, **When** the user chooses Revoke all access from the header overflow menu, **Then** a confirmation names the agent and explains the effect. Confirming revokes only that user's delegation; cancelling preserves it.

---

### User Story 5 - Manage Connections (Priority: P2)

A returning user reviews third-party services and reconnects, refreshes, or disconnects without losing the existing authorization flow.

**Why this priority**: A grant is only useful when the required service connection remains usable.

**Independent Test**: Inspect connection states, complete a service authorization callback, refresh a supported session, and disconnect after reading the warning.

**Acceptance Scenarios**:

1. **AS-10**: **Given** connected, expired, and unusable connections and an agent that requires an unconnected service, **When** the user opens `/sessions` and that agent's Connections tab, **Then** `/sessions` lists each stored connection with provider, scope count, truthful state, creation time, and an appropriate reconnect or refresh action, and the agent's Connections tab shows the unconnected service as No connection with a Connect action.
2. **AS-11**: **Given** a connection that needs attention, **When** the user reconnects, completes the existing authorization callback, refreshes a supported session, or disconnects, **Then** the visible state updates without changing token semantics. Before disconnect, the user sees that broker disconnection does not revoke provider-side tokens.

---

### User Story 6 - Triage Approvals (Priority: P2)

A returning user handles pending tool requests and revokes standing allow or deny decisions on `/approvals`.

**Why this priority**: Pending requests are time-sensitive; remembered decisions need ongoing user control.

**Independent Test**: Resolve a pending request inline with an existing persistence choice and revoke a standing decision; observe a newly arrived request without reloading.

**Acceptance Scenarios**:

1. **AS-12**: **Given** pending requests and standing decisions, **When** the user opens `/approvals`, **Then** pending requests appear first with inline Approve and Deny, a persistence choice and scope preview, and standing allow or deny decisions below with Revoke. Only a user-confirmed action changes authorization.
2. **AS-13**: **Given** an open console and a new pending request, **When** it arrives, **Then** the queue and sidebar count update within 15 seconds without reloading. Assistive technology announces the change without taking focus. A regular refresh reads only the acting user's pending list; the gateway-wide long-poll is never exposed.

---

### User Story 7 - Use the Console in Either Theme (Priority: P2)

A returning user moves between Agents, Connections, and Approvals using a compact console. They choose light, dark, or system appearance.

**Why this priority**: Navigation and legibility affect every self-service task.

**Independent Test**: Collapse the sidebar, switch themes, navigate all existing routes, and reload in a different system theme.

**Acceptance Scenarios**:

1. **AS-14**: **Given** a new or returning browser, **When** the user opens a console page, collapses the sidebar, chooses light, dark, or system in the user menu, and reloads, **Then** the selected theme persists per browser, follows later system changes in system mode, and never flashes the wrong theme. Controls and scrollbars match the theme.
2. **AS-15**: **Given** either theme and a narrow or zoomed viewport, **When** the user navigates the console and decision views by keyboard, **Then** the AIB wordmark, visible focus, readable labels, and usable layouts remain available without horizontal scrolling or a second competing primary action.

---

### User Story 8 - Choose Appearance in Settings (Priority: P3)

A user changes their theme in `/settings`.

**Why this priority**: A settings page gives the theme choice a stable home beyond the user menu.

**Independent Test**: Change the theme in `/settings`, reload the browser, and open an approval.

**Acceptance Scenarios**:

1. **AS-17**: **Given** browser defaults, **When** the user changes the theme in `/settings` and reloads, **Then** every console and decision view uses that theme. `/settings` offers no approval-persistence default, and every approval keeps its existing persistence choices and explicit user action.

---

### User Story 9 - Jump to a Record (Priority: P3)

A user opens a command palette to find an agent, connection, or approval, or change the theme.

**Why this priority**: Direct navigation reduces work for frequent users without hiding normal navigation.

**Independent Test**: Open the palette by keyboard, search for each record type, navigate, and change the theme.

**Acceptance Scenarios**:

1. **AS-18**: **Given** an open console, **When** the user opens the palette, searches for a record, chooses a result, or toggles theme, **Then** the user reaches the existing detail view or sees the new theme. Results and keyboard interaction remain scoped to the acting user.

### Edge Cases

- No existing response identifies an agent's publisher. Show no publisher field and do not imply that a publisher identity was checked.
- If a consent request has no CIMD metadata, show “Registered by your administrator”, not “Unverified”. Reserve “Unverified” for a CIMD request without domain trust, such as a localhost client.
- Outside an authorization request, no existing response supplies verification data. Console views show no Agent Origin Label rather than a guessed or uniform one.
- If a service or agent logo is only available at a third-party origin, show a local fallback instead of silently loading that image.
- Permission sets carry no risk rating, so permission groups show no risk indicator. If a tool approval has no server-provided risk level, show a neutral “Risk not rated” label. Never infer a security guarantee from a scope or tool name.
- Never show an expired grant as active. The existing delegation list excludes expired grants; if a listed grant's expiry passes while the page is open, remove its row.
- If a user's previous grant is unchanged, do not prompt for previously granted permissions. Preserve the existing safe continuation and do not enlarge the grant.
- If a third-party connection is missing, a refresh token expires, or refresh fails, show the correct reconnect path. Do not label an unusable session “Connected”.
- If a pending approval expires or another session resolves it, stop offering actions and show its current state.
- If a requested service connection interrupts consent, preserve the current selections and authorization session through the existing callback flow.
- If a user denies a consent request, leave existing grants unchanged. Do not construct an unverified redirect or claim that provider-side tokens were revoked.
- If user-supplied or agent-supplied text contains markup or exceeds its display area (two lines for names and descriptions, one line in table cells), show escaped, truncated text with an accessible expand action.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: Preserve `/delegations`, `/agents/:id`, `/sessions`, `/approvals`, and `/approvals/:id`, including existing authorization, session, grant, approval, and callback behavior. Keep `/` directed to `/delegations` (AS-01–AS-15).
- **FR-002**: Use the same `/agents/:id` address for a focused decision view when an authorization session is present and a console detail view otherwise. Decision views and `/approvals/:id` have no sidebar (AS-01, AS-04, AS-08).
- **FR-003**: Show the repository's black AIB wordmark in light mode, white wordmark in dark mode, and matching favicon. Replace the shield and “Consent Management” title throughout. Show “Powered by Zalando” only as small monochrome decision-view footer text if used (AS-01, AS-04, AS-14).
- **FR-004**: Use self-hosted Zalando Sans for the wordmark, page titles, decision-screen agent and service names, and empty-state headlines. Use a neutral self-hosted body face and a self-hosted monospaced face for identifiers, scopes, and timestamps. Remove Crimson Pro and Manrope (AS-01, AS-14, AS-15).
- **FR-005**: Replace the cream/taupe gradient, serif-led editorial styling, strong card shadows, loud multi-accent actions, marketing top navigation, and page-entry animation with neutral `background` and `card` surfaces, one `primary` accent, a sans-serif hierarchy, 1 px `border`-token borders, and feedback limited to toasts, inline status, and 120–200 ms CSS transitions. Show at most one accent-colored primary action per view (AS-01, AS-04, AS-14, AS-15).
- **FR-006**: The console has a collapsible sidebar with wordmark, search, navigation, pending-approval count, and a user menu with theme choice. Each console page has a title, a one-line purpose, and only the primary action that the UI contract assigns to it, if any (AS-06, AS-12, AS-14).
- **FR-007**: The consent view shows an agent logo or safe fallback, name, governance link when available, and one Agent Origin Label from the authorization session: “Verified domain: host” for validated CIMD metadata, “Registered by your administrator” when CIMD metadata is absent, or “Unverified” for a CIMD request without domain trust. A validated CIMD domain does not claim that a publisher's legal identity was verified. Promote the existing localhost/CIMD warning to a prominent banner (AS-01).
- **FR-008**: Describe the agent's requested access in one plain-language sentence. Group permissions by purpose with icon, the permission set's human-readable name and description, required or optional control, and service names one expansion away. Show no risk indicator on permission groups. Do not add raw scope strings to permission groups or to any other view that does not show them today (AS-01).
- **FR-009**: List required groups before optional groups and prevent users from disabling required groups or required services. Preserve optional permission-set and service selection, and existing validation (AS-01, AS-08).
- **FR-010**: Offer “Until revoked”, “30 days”, and “Custom date” grant duration choices. Keep the existing validity semantics and validate a custom date before submission (AS-03, AS-08).
- **FR-011**: On re-consent, compare the requested permission-set/service selections with the existing `granted_permission_sets`. Show only new access as a decision, display already-granted groups collapsed, checked, and read-only, and preserve prior choices when saving. Start the duration choice from the existing grant's validity; Allow applies the chosen duration to the whole grant. Use the existing user grant read response; do not add a delta API (AS-02).
- **FR-012**: Offer one primary Allow and a secondary Deny in consent, with no destructive styling. Allow resumes the existing validated authorization flow; Deny does not create or revoke a grant. Explain next steps and where access can later be reviewed (AS-01, AS-03).
- **FR-013**: Tool review identifies tool, agent, acting user, arguments in a collapsible monospace block, the server-provided risk level with a label and accessible explanation, and the approval scope. Preserve once, session, and permanent persistence and existing scope-preview validation; show the resolved outcome without new actions (AS-04, AS-05).
- **FR-014**: The agent list shows logo or fallback, name, the number of granted permission sets from the existing `activeGrantCount`, expiry, name search, View, and confirmed Revoke. It lists only unexpired delegations, because the existing response excludes expired grants, and has no status column or status filter. Its empty state uses the wordmark and explains a delegation in one sentence (AS-06, AS-07).
- **FR-015**: The agent detail has identity, links, Permissions and Connections tabs, editable optional groups, and a sticky Cancel/Save changes bar only for unsaved changes (AS-08).
- **FR-016**: Move “Revoke all access” to the detail header overflow menu. Require confirmation; do not show it as a resting red primary action (AS-09).
- **FR-017**: The connections list shows provider, scope count, state, and creation time for each stored connection. Offer Reconnect, supported Refresh, and confirmed Disconnect without changing authorize/callback/refresh behavior (AS-10, AS-11).
- **FR-018**: Distinguish Connected, Needs re-authentication, Expired, and No connection using only existing session fields, refresh responses, and agent requirement connection status. `/sessions` lists stored connections only, so No connection appears only for a service an agent requires but the user has not connected: on the agent's Connections tab and in the consent view's service prompt. Do not add a missing-scope state or infer missing requirements from available scopes (AS-10).
- **FR-019**: Disconnect text states clearly that broker disconnection does not revoke provider-side tokens. Preserve dependent-agent warnings and current callback error handling (AS-11).
- **FR-020**: Show the pending queue above standing allow and deny decisions; support inline Approve/Deny, existing persistence and scope choices, and revocation of standing decisions. Row actions use non-accent styling, and `/approvals` has no page-level accent action (AS-12).
- **FR-021**: Refresh the existing user-scoped pending list regularly while the console is open. Show new approvals and sidebar count changes within 15 seconds without reloading. Announce arrivals and decision-state changes without taking focus; never use the gateway-wide long-poll for a browser (AS-13).
- **FR-022**: Support light, dark, and system themes. Remember the choice per browser, follow changes to the system theme when chosen, avoid the wrong-theme flash, and theme form controls and scrollbars (AS-14, AS-15).
- **FR-024**: Add `/settings` with the per-browser theme choice. Do not add a saved approval-persistence default; approval persistence choices keep their existing behavior (AS-17).
- **FR-025**: Add a keyboard-accessible command palette to find only the current user's agents, connections, and approvals, jump to their existing views, and change theme (AS-18).
- **FR-026**: Escape all agent-supplied strings and CIMD metadata. Truncate long display text with an accessible expansion. Do not load third-party fonts, scripts, or images while rendering the application; keep user-initiated external links and OAuth2 redirects functional (AS-01, AS-04, AS-15).
- **FR-027**: Collect user-facing copy in one place for later localization. Use second-person, present-tense, action-first wording for sentences and actions; keep proper names, technical scopes, status labels, and route titles accurate (AS-01–AS-18).
- **FR-028**: Deliver every redesigned route and capability in one cutover. Do not use implementation phases, feature flags, dual presentations, backwards-compatibility layers, or old-server fallbacks. Migrate every consumer and remove obsolete components, tokens, aliases, fonts, animations, and caches in the same release (AS-01–AS-18).
- **FR-029**: Keep the existing user-facing end-to-end journeys working with selector changes only for existing behaviors. Add traceable acceptance coverage for changed journeys and both themes (AS-01–AS-18).
- **FR-030**: Record the `/settings` route and the visual-direction change in an ADR that is accepted before implementation (AS-14, AS-17).
- **FR-031**: Produce README guidance and documentation screenshots for every original route and the new `/settings` route in light and dark modes, including both agent-detail contexts (AS-01, AS-04, AS-06, AS-08, AS-10, AS-12, AS-14, AS-17).

### Domain Model *(if applicable - document before API or database design)*

The redesign does not change the meaning or ownership of an Agent, UserGrant, UserSession, Permission Set, or ToolApproval. A consent decision combines the current authorization request, existing grant, selected services, and chosen expiry. It never promotes a third-party connection to a grant. The console calls a principal's unexpired UserGrant to one agent a delegation.

A connection state summarizes token usability from existing session fields, refresh responses, and agent requirement connection status. Agent Origin Labels derive only from the current authorization session. A verified-domain label describes validated CIMD domain metadata, not a verified publisher identity. Absent CIMD metadata means an administrator registered the agent. Console views carry no Agent Origin Label.

### API Requirements *(if applicable - design before database)*

- **API-001**: Keep existing end-user contracts and the consent-grants read response unchanged. The redesign must preserve the existing authorization session, third-party authorization/callback/refresh, scope preview, and approval-decision contracts. The only `api/enduser/openapi.yaml` change is the documentation correction of the existing `GET /api/third-party/sessions` response to `{data: {sessions: [UserSessionSummary]}}` (Clarifications, 2026-09-27).
- **API-004**: The gateway-only approval long-poll is not a browser data source. Refresh the existing acting-user pending-list response regularly for live browser updates.
- **API-005**: This feature adds or changes no backend endpoint, response field, runtime response, persistence, or OAuth2/token contract. Every screen uses existing end-user responses. Correcting documentation to match an existing response is not a contract change.

### Security Requirements *(mandatory for security-critical features)*

- **SR-001**: Preserve acting-user authorization on all reads and mutations. Pending counts and command-palette results never expose another principal's data (AS-06, AS-13, AS-18).
- **SR-002**: Treat unverified agent identity and unrated tool calls as unknown, not safe. Never label a permission group as low-risk. Keep prominent localhost/CIMD warnings and prevent untrusted redirect or external resource loading (AS-01, AS-03, AS-15).
- **SR-003**: Keep grant, approval, and connection revocation scopes unchanged (AS-09, AS-11).

### Frontend/Design System Requirements *(if applicable - document before implementation)*

- **FE-001**: Use one shared design system for both layouts. Principle XI requires semantic tokens, accessible light/dark stories, and self-hosted assets without prescribing an aesthetic. The current direction in `DESIGN_PRINCIPLES.md` remains authoritative until an accepted ADR changes it. After acceptance, update all current guidance listed in the plan for that direction. Preserve historical decisions and replace only references that incorrectly present them as current rules (AS-01, AS-14, AS-15).
- **FE-002**: Support WCAG 2.2 AA: text contrast at least 4.5:1, non-text controls at least 3:1, a keyboard path through every flow, visible focus, and state/toast announcements. Check every component story in both themes (AS-01–AS-18).
- **FE-003**: At 320 px width and 200% zoom, neither layout has horizontal page scrolling. Dense lists remain readable without removing actions or hiding permission details (AS-06, AS-15).
- **FE-004**: Provide a local font-safe rendition of the repository wordmarks. The current SVG's remote font import is not an acceptable runtime dependency. The optional footer partner mark is local and monochrome (AS-01, AS-04, AS-14).

### Key Entities *(include if feature involves data)*

- **Agent**: The named requester, its trustworthy origin information, optional governance and documentation links, and user-owned grants.
- **Permission Set and UserGrant**: A human-readable, named, and described group of allowed services with required or optional controls, and one user's selections and expiry. Existing selections remain part of delta re-consent. The console lists unexpired UserGrants as delegations.
- **UserSession**: A user's third-party connection, granted scopes, expiry and refresh capacity, and dependent agents. Its visible state must reflect usable access.
- **ToolApproval**: A user's pending or resolved tool decision with reviewed arguments, scope, risk, and once, session, or permanent persistence.
- **Appearance Preferences**: Per-browser theme and sidebar choices. They never affect a consent or tool decision.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: In moderated first-time consent sessions, users decide within 30 seconds and can name each granted service and state what each selected permission set allows.
- **SC-002**: Under the Chrome DevTools “Slow 4G” network preset, the consent screen's main content appears within 1.5 seconds. Its initial compressed application code is under 150 kB.
- **SC-003**: Every screen passes WCAG 2.2 AA checks in both themes. Automated component checks find no text below 4.5:1 contrast; every flow works by keyboard and announces new approvals, decisions, and toasts.
- **SC-004**: At 320 px width and 200% zoom, 100% of routes remain usable without horizontal page scrolling.
- **SC-005**: At least 12 agent rows fit within a 1080 px-high viewport without page scrolling in the standard desktop layout.
- **SC-006**: Opening and using any application screen makes zero automatic requests to third-party origins. User-initiated OAuth2 navigation and external links still work.
- **SC-007**: No screen displays more than one accent-colored primary action. At least 90% of moderated participants identify the next action on the consent and tool-review screens without assistance.
- **SC-008**: Every original and new route has a README reference and a documentation screenshot in light and dark modes. Both agent-detail contexts are represented.
- **SC-009**: The existing end-to-end journeys pass with only selector changes for their preserved behaviors. Every new acceptance scenario has a corresponding end-to-end journey.
- **SC-010**: The release contains every active AS-01–AS-18 capability with one presentation and no feature flags or compatibility paths. Every current-guidance reference in the plan matches the accepted visual decision. Historical records retain their original decisions.

## Assumptions

- Users use evergreen browsers. System theme, local appearance settings, and keyboard navigation are available.
- Planning selects a neutral blue accent and Inter as the body face. All fonts are self-hosted. These proposed choices require ADR acceptance.
- A validated CIMD domain supports a domain-specific verification label only. Registered names alone do not prove a publisher's identity. Existing agent reads outside an authorization session expose no CIMD or verification data.
- Existing session data exposes service scopes, granted scopes, access-token expiry, refresh-token expiry, and refresh-token presence. Connection state derives only from these fields.
- Existing agent and session summaries have no last-used timestamp. The redesign does not show last use and never substitutes a modification or connection time.
- External agent/provider logo URLs are not loaded during page rendering. A same-origin asset can be used when available; otherwise a local placeholder preserves identification without third-party requests.
- Deny cancels the current decision without creating or revoking a grant. Do not promise an OAuth2 error callback until an existing, validated cancellation path is identified; no new OAuth2 semantics are authorized here.
- The five browser routes from ADR 035 remain stable. `/settings` is an additive P3 route; a binding ADR must authorize the expanded route map and visual-direction change before implementation.
- Theme is a per-browser preference, not a cross-device account setting.
- The gateway long-poll remains machine-only. The browser refreshes its existing user-scoped pending list to meet the no-reload requirement without changing that contract.
- The retained provider OAuth2 flow necessarily navigates to a third party when the user chooses it. The zero-external-requests goal applies to automatic page resources and background traffic, not user-directed external navigation.
- This feature excludes the admin port, tenant white-labeling, full localization, user activity history, backend endpoint or response-field changes, persistence changes, and changes to OAuth2 or token semantics.
