# Feature Specification: Redesign End-User Consent and Console

**Feature Branch**: `046-redesign-consent-console`

**Created**: 2026-09-25

**Status**: Draft

**Input**: Redesign the end-user consent frontend as a calm, fast security tool. Preserve existing capabilities and authorization behavior. Add only the specified user activity read endpoint and, if necessary, a session response field.

## Clarifications

### Session 2026-09-25

- Q: If the system cannot save an event to the new activity history, should the user's action still complete? → A: Preserve the action's normal outcome. Record the activity-storage failure in operational logs. The user-facing history can have gaps. Existing security audit logging remains required.

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Decide on an Agent Request (Priority: P1)

A user arrives from an authorization request and needs to understand the agent, the requested access, and the duration before choosing Allow or Deny. The decision stays focused even when the same agent already has access.

**Why this priority**: Consent is the point where the user grants security-sensitive access.

**Independent Test**: Start an authorization request, review its permissions, connect a required service if needed, and allow or deny without visiting the console.

**Acceptance Scenarios**:

1. **AS-01**: **Given** a first-time request, **When** the user opens `/agents/:id`, **Then** a single-column view identifies the agent and its verification status. It highlights localhost risks and orders required permission groups first. Each group explains access in plain language, expands to exact service scopes, and shows an accessible risk explanation. The view offers duration choices, one primary Allow action, secondary Deny, and an explanation of what happens next.
2. **AS-02**: **Given** an active grant, **When** the user returns through another request, **Then** only new access needs a decision. Existing groups appear collapsed and checked. Continuing preserves prior groups and never widens access without the user's selection.
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

**Independent Test**: Find an agent by name, filter by status, inspect it, and revoke its grant with confirmation.

**Acceptance Scenarios**:

1. **AS-06**: **Given** several delegations, **When** the user opens `/delegations`, searches, or filters by status, **Then** a dense list shows the agent, verification, granted services, last use when known, expiry, and status. Each row offers View and a confirmed Revoke action.
2. **AS-07**: **Given** no delegations or a revoked delegation, **When** the user opens or returns to `/delegations`, **Then** the empty state explains what a delegation is, or the revoked agent disappears from active access after confirmation. The user sees a clear result in either case.

---

### User Story 4 - Change an Agent Grant (Priority: P2)

A returning user reviews an agent's permissions and connections on `/agents/:id`, changes selectable access, and saves only intentional edits.

**Why this priority**: Access needs a manageable lifecycle after initial consent.

**Independent Test**: Open an existing grant, edit an optional permission or duration, cancel and save in separate runs, and revoke all access from the overflow menu.

**Acceptance Scenarios**:

1. **AS-08**: **Given** an existing agent grant, **When** the user opens the agent outside an authorization request, **Then** a console detail view shows identity, links, verification, Permissions and Sessions tabs, locked required groups, and editable optional groups. A sticky Cancel and Save changes bar appears only after an edit; cancel discards it and save persists it.
2. **AS-09**: **Given** an agent with active access, **When** the user chooses Revoke all access from the header overflow menu, **Then** a confirmation names the agent and explains the effect. Confirming revokes only that user's delegation; cancelling preserves it.

---

### User Story 5 - Manage Connections (Priority: P2)

A returning user reviews third-party services and reconnects, refreshes, or disconnects without losing the existing authorization flow.

**Why this priority**: A grant is only useful when the required service connection remains usable.

**Independent Test**: Inspect connection states, complete a service authorization callback, refresh a supported session, and disconnect after reading the warning.

**Acceptance Scenarios**:

1. **AS-10**: **Given** connected, insufficient-scope, expired, and unusable connections, **When** the user opens `/sessions`, **Then** each service shows provider, account if known, scope count, truthful state, creation time, last use when known, and an appropriate reconnect or refresh action.
2. **AS-11**: **Given** a connection that needs attention, **When** the user reconnects, completes the existing authorization callback, refreshes a supported session, or disconnects, **Then** the visible state updates without changing token semantics. Before disconnect, the user sees that broker disconnection does not revoke provider-side tokens when that is the case.

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

### User Story 8 - Review My Activity (Priority: P3)

A user reviews their own recent grants, revocations, approval decisions, and token exchanges on `/activity`.

**Why this priority**: A history helps the user understand how their access was used.

**Independent Test**: Open activity as two different users and filter by agent and event type; neither user can see the other's events.

**Acceptance Scenarios**:

1. **AS-16**: **Given** activity for multiple users, **When** one user filters `/activity` by agent or type, **Then** only their last 90 days of events appear. The trail includes grant changes, revocations, approval decisions, and token exchanges, plus denied or failed attempts attributable to that user. Each entry shows a time and safe outcome, but never tokens, secrets, raw tool arguments, or another user's activity. If activity storage fails during an action, the action retains its normal outcome and operational logs record the storage failure without credentials. The user-facing history can have gaps. Existing security audit logging remains required.

---

### User Story 9 - Set Approval Defaults (Priority: P3)

A user changes their theme and default approval persistence in `/settings`, without changing how earlier approvals work.

**Why this priority**: An explicit default saves time without making remembered authorization implicit.

**Independent Test**: Change the two settings, reload the browser, and open a new approval.

**Acceptance Scenarios**:

1. **AS-17**: **Given** browser defaults, **When** the user changes the theme or approval persistence in `/settings`, **Then** later decisions start with those choices. Every approval still requires an explicit user action, and a remembered scope still requires review.

---

### User Story 10 - Jump to a Record (Priority: P3)

A user opens a command palette to find an agent, connection, or approval, or change the theme.

**Why this priority**: Direct navigation reduces work for frequent users without hiding normal navigation.

**Independent Test**: Open the palette by keyboard, search for each record type, navigate, and change the theme.

**Acceptance Scenarios**:

1. **AS-18**: **Given** an open console, **When** the user opens the palette, searches for a record, chooses a result, or toggles theme, **Then** the user reaches the existing detail view or sees the new theme. Results and keyboard interaction remain scoped to the acting user.

### Edge Cases

- If an agent has no publisher or trusted domain, show “Publisher not provided” and “Unverified”; do not imply a publisher identity was checked.
- If a service or agent logo is only available at a third-party origin, show a local fallback instead of silently loading that image.
- If no authoritative risk rating exists for a permission group, show a neutral “Risk not rated” label. Never infer a security guarantee from a scope name.
- If no last-use event exists, show “Not recorded”, not the creation or last-modified time. Do not confuse an expired grant with an active one.
- If a user's previous grant is unchanged, do not prompt for previously granted permissions. Preserve the existing safe continuation and do not enlarge the grant.
- If a third-party connection is missing, a refresh token expires, or refresh fails, show the correct reconnect path. Do not label an unusable session “Connected”.
- If a pending approval expires or another session resolves it, stop offering actions and show its current state.
- If a requested service connection interrupts consent, preserve the current selections and authorization session through the existing callback flow.
- If a user denies a consent request, leave existing grants unchanged. Do not construct an unverified redirect or claim that provider-side tokens were revoked.
- If user-supplied or agent-supplied text is long or contains markup, show escaped, truncated text with an accessible expand action.
- If activity storage fails, preserve the normal outcome of consent, approval, token exchange, denial, and revocation actions. Record the storage failure in operational logs without credentials. The user-facing history can have gaps. Existing security audit logging remains required.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: Preserve `/delegations`, `/agents/:id`, `/sessions`, `/approvals`, and `/approvals/:id`, including existing authorization, session, grant, approval, and callback behavior. Keep `/` directed to `/delegations` (AS-01–AS-15).
- **FR-002**: Use the same `/agents/:id` address for a focused decision view when an authorization session is present and a console detail view otherwise. Decision views and `/approvals/:id` have no sidebar (AS-01, AS-04, AS-08).
- **FR-003**: Show the repository's black AIB wordmark in light mode, white wordmark in dark mode, and matching favicon. Replace the shield and “Consent Management” title throughout. Show “Powered by Zalando” only as small monochrome decision-view footer text if used (AS-01, AS-04, AS-14).
- **FR-004**: Use self-hosted Zalando Sans for the wordmark, page titles, decision-screen agent and service names, empty-state headlines, and statistics. Use a neutral self-hosted body face and a self-hosted monospaced face for identifiers, scopes, and timestamps. Remove Crimson Pro and Manrope (AS-01, AS-14, AS-15).
- **FR-005**: Replace the cream/taupe gradient, serif-led editorial styling, strong card shadows, loud multi-accent actions, marketing top navigation, and page-entry animation with a neutral surface, one accent, sans-serif hierarchy, thin borders, and restrained feedback. Show at most one accent-colored primary action per view (AS-01, AS-04, AS-14, AS-15).
- **FR-006**: The console has a collapsible sidebar with wordmark, search, navigation, pending-approval count, and a user menu with theme choice. Each console page has a title, one-line purpose, and a contextual primary action where one exists (AS-06, AS-12, AS-14).
- **FR-007**: The consent view shows an agent logo or safe fallback, name, publisher when known, governance link when available, and a verified-domain or unverified badge. A validated CIMD domain does not claim that a publisher's legal identity was verified. Promote the existing localhost/CIMD warning to a prominent banner (AS-01).
- **FR-008**: Describe the agent's requested access in one plain-language sentence. Group permissions by purpose with icon, title, one-line description, required or optional control, exact service names and scope strings one expansion away, and a labeled risk indicator with an accessible explanation (AS-01).
- **FR-009**: List required groups before optional groups and prevent users from disabling required groups or required services. Preserve optional permission-set and service selection, and existing validation (AS-01, AS-08).
- **FR-010**: Offer “Until revoked”, “30 days”, and “Custom date” grant duration choices. Keep the existing validity semantics and validate a custom date before submission (AS-03, AS-08).
- **FR-011**: On re-consent, compare the requested permission-set/service selections with the existing `granted_permission_sets`. Show only new access as a decision, display already-granted groups collapsed and checked, and preserve prior choices when saving. Use the existing user grant read response; do not add a delta API (AS-02).
- **FR-012**: Offer one primary Allow and a secondary Deny in consent, with no destructive styling. Allow resumes the existing validated authorization flow; Deny does not create or revoke a grant. Explain next steps and where access can later be reviewed (AS-01, AS-03).
- **FR-013**: Tool review identifies tool, agent, acting user, arguments in a collapsible monospace block, risk, and the approval scope. Preserve once, session, and permanent persistence and existing scope-preview validation; show the resolved outcome without new actions (AS-04, AS-05).
- **FR-014**: The agent list shows logo or fallback, name, trustworthy verification state, granted services, last use if recorded, expiry, status, search, status filter, View, and confirmed Revoke. Its empty state uses the wordmark and explains a delegation in one sentence (AS-06, AS-07).
- **FR-015**: The agent detail has identity, publisher when known, links, verification, Permissions and Sessions tabs, editable optional groups, and a sticky Cancel/Save changes bar only for unsaved changes. Hide the Activity tab until activity exists (AS-08).
- **FR-016**: Move “Revoke all access” to the detail header overflow menu. Require confirmation; do not show it as a resting red primary action (AS-09).
- **FR-017**: The connections list shows provider, account when available, scope count, state, creation time, and last use when recorded. Offer Reconnect, supported Refresh, and confirmed Disconnect without changing authorize/callback/refresh behavior (AS-10, AS-11).
- **FR-018**: Distinguish Connected, Missing scopes, Needs re-authentication, Expired, and no connection using reliable service and session information. Add only the permitted minimal session response field if required scopes cannot be determined from existing user-scoped data. Never infer a missing requirement from optional available scopes (AS-10).
- **FR-019**: Disconnect text states clearly when broker disconnection does not revoke provider-side tokens. Preserve dependent-agent warnings and current callback error handling (AS-11).
- **FR-020**: Show the pending queue above standing allow and deny decisions; support inline Approve/Deny, existing persistence and scope choices, and revocation of standing decisions. Keep each row's actions visually secondary to any page-level primary action (AS-12).
- **FR-021**: Refresh the existing user-scoped pending list regularly while the console is open. Show new approvals and sidebar count changes within 15 seconds without reloading. Announce arrivals and decision-state changes without taking focus; never use the gateway-wide long-poll for a browser (AS-13).
- **FR-022**: Support light, dark, and system themes. Remember the choice per browser, follow changes to the system theme when chosen, avoid the wrong-theme flash, and theme form controls and scrollbars (AS-14, AS-15).
- **FR-023**: Add `/activity` as a read-only, user-owned trail of grant creation and edits, revocations, approval decisions, and token exchanges. Include attributable denials and failures. Support agent and event-type filters without exposing credentials or other users' events. If activity storage fails, preserve the action's normal outcome and record the storage failure in operational logs without credentials. The user-facing history can have gaps. Existing security audit logging remains required (AS-16).
- **FR-024**: Add `/settings` with theme and default approval persistence. Keep the default per browser, default to “Approve once”, and require explicit confirmation for every decision (AS-17).
- **FR-025**: Add a keyboard-accessible command palette to find only the current user's agents, connections, and approvals, jump to their existing views, and change theme (AS-18).
- **FR-026**: Escape all agent-supplied strings and CIMD metadata. Truncate long display text with an accessible expansion. Do not load third-party fonts, scripts, or images while rendering the application; keep user-initiated external links and OAuth2 redirects functional (AS-01, AS-04, AS-15).
- **FR-027**: Collect user-facing copy in one place for later localization. Use second-person, present-tense, action-first wording for sentences and actions; keep proper names, technical scopes, status labels, and route titles accurate (AS-01–AS-18).
- **FR-028**: After all five existing views migrate, remove the old aesthetic, fonts, gradient, and page-entry animations. A temporary migration switch is permissible but not a permanent second theme (AS-14, AS-15).
- **FR-029**: Keep the existing user-facing end-to-end journeys working with selector changes only for existing behaviors. Add traceable acceptance coverage for changed journeys and both themes (AS-01–AS-18).
- **FR-030**: Add `/activity` and `/settings` without changing the five existing route addresses or authorization behavior. Record the route and visual-direction changes in an ADR before implementation (AS-14, AS-16, AS-17).
- **FR-031**: Produce README guidance and documentation screenshots for every original route and both new routes in light and dark modes, including both agent-detail contexts (AS-01, AS-04, AS-06, AS-08, AS-10, AS-12, AS-14, AS-16, AS-17).

### Domain Model *(if applicable - document before API or database design)*

The redesign does not change the meaning or ownership of an Agent, UserGrant, UserSession, Permission Set, or ToolApproval. A consent decision combines the current authorization request, existing grant, selected services, and chosen expiry. It never promotes a third-party connection to a grant.

A connection state summarizes token usability and unmet *required* scopes. A verified-domain label describes validated CIMD domain metadata, not a verified publisher identity. An Activity Event is an immutable, user-owned record of a completed action or user-attributable denial or failure. It carries a time, type, safe outcome, and applicable agent or connection, but never a token or raw tool arguments.

### API Requirements *(if applicable - design before database)*

- **API-001**: Keep existing end-user contracts and the consent-grants read response unchanged. The redesign must preserve the existing authorization session, third-party authorization/callback/refresh, scope preview, and approval-decision contracts.
- **API-002**: Provide one additive, authenticated end-user read endpoint for the acting user's activity. Include safe event types and outcomes, user-scoped filters, pagination, and a maximum age of 90 days. Document and confirm its contract before implementation.
- **API-003**: The existing third-party session response includes configured service scopes, granted session scopes, token expiry, and refresh-token presence. These do not identify the *required* scopes of dependent agents. If that gap cannot be answered by existing user-scoped data, add one backward-compatible field for the missing required scopes; do not change existing field semantics.
- **API-004**: The gateway-only approval long-poll is not a browser data source. Refresh the existing acting-user pending-list response regularly for live browser updates. No third additive API is authorized.
- **API-005**: Document and obtain stakeholder approval for each permitted additive end-user contract before implementing it. No other backend endpoint or OAuth2/token contract changes belong to this feature.

### Security Requirements *(mandatory for security-critical features)*

- **SR-001**: Preserve acting-user authorization on all reads and mutations. An Activity Event and pending-count result never expose another principal's data (AS-06, AS-13, AS-16).
- **SR-002**: Treat unverified agent identity and unrated permissions as unknown, not safe. Keep prominent localhost/CIMD warnings and prevent untrusted redirect or external resource loading (AS-01, AS-03, AS-15).
- **SR-003**: Do not store credentials, tokens, secrets, or raw tool arguments in user activity. Keep grant, approval, and connection revocation scopes unchanged (AS-09, AS-11, AS-16).
- **SR-004**: Show only activity from the last 90 days and delete records older than 90 days. Do not expose expired records while deletion is pending (AS-16).

### Frontend/Design System Requirements *(if applicable - document before implementation)*

- **FE-001**: Use one shared design system for both layouts. Update its semantic typography, color, spacing, border, interaction, and theme rules for the new visual direction. Do not keep obsolete aesthetic rules after cutover (AS-01, AS-14, AS-15).
- **FE-002**: Support WCAG 2.2 AA: text contrast at least 4.5:1, non-text controls at least 3:1, a keyboard path through every flow, visible focus, and state/toast announcements. Check every component story in both themes (AS-01–AS-18).
- **FE-003**: At 320 px width and 200% zoom, neither layout has horizontal page scrolling. Dense lists remain readable without removing actions or hiding permission details (AS-06, AS-15).
- **FE-004**: Provide a local font-safe rendition of the repository wordmarks. The current SVG's remote font import is not an acceptable runtime dependency. The optional footer partner mark is local and monochrome (AS-01, AS-04, AS-14).

### Key Entities *(include if feature involves data)*

- **Agent**: The named requester, its trustworthy origin information, optional publisher and documentation links, and user-owned grants.
- **Permission Set and UserGrant**: A group of allowed services with required or optional controls, and one user's selections and expiry. Existing selections remain part of delta re-consent.
- **UserSession**: A user's third-party connection, granted scopes, expiry and refresh capacity, and dependent agents. Its visible state must reflect usable access.
- **ToolApproval**: A user's pending or resolved tool decision with reviewed arguments, scope, risk, and once, session, or permanent persistence.
- **Activity Event**: A user-owned audit entry containing a time, type, safe outcome, and applicable agent or service. AS-16 defines event coverage and permits gaps after activity-storage failures. SR-004 limits retention to 90 days.
- **Appearance and Approval Defaults**: Per-browser choices that never replace an explicit consent or tool decision.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: In moderated first-time consent sessions, users decide within 30 seconds and can name each granted service and whether it has read or write access.
- **SC-002**: On a throttled 4G profile, the consent screen's main content appears within 1.5 seconds. Its initial compressed application code is under 150 kB.
- **SC-003**: Every screen passes WCAG 2.2 AA checks in both themes. Automated component checks find no text below 4.5:1 contrast; every flow works by keyboard and announces new approvals, decisions, and toasts.
- **SC-004**: At 320 px width and 200% zoom, 100% of routes remain usable without horizontal page scrolling.
- **SC-005**: At least 12 agent rows fit within a 1080 px-high viewport without page scrolling in the standard desktop layout.
- **SC-006**: Opening and using any application screen makes zero automatic requests to third-party origins. User-initiated OAuth2 navigation and external links still work.
- **SC-007**: No screen displays more than one accent-colored primary action. At least 90% of moderated participants identify the next action on the consent and tool-review screens without assistance.
- **SC-008**: Every original and new route has a README reference and a documentation screenshot in light and dark modes. Both agent-detail contexts are represented.
- **SC-009**: The existing end-to-end journeys pass with only selector changes for their preserved behaviors. Every new acceptance scenario has a corresponding end-to-end journey.

## Assumptions

- Users use evergreen browsers. System theme, local appearance settings, and keyboard navigation are available.
- The accent is selected during planning (Zalando orange or a neutral blue). The neutral body face is selected during planning (Inter or Geist). All selected fonts are self-hosted.
- A validated CIMD domain supports a domain-specific verification label only. Registered names alone do not prove a publisher's identity.
- Existing session data exposes service scopes, granted scopes, access-token expiry, refresh-token expiry, and refresh-token presence. It does not identify which service scopes are required by dependent agents.
- Existing agent and session summaries have no last-used timestamp. Until a user-owned activity source provides one, show “Not recorded” rather than substituting last modification or connection creation.
- External agent/provider logo URLs are not loaded during page rendering. A same-origin asset can be used when available; otherwise a local placeholder preserves identification without third-party requests.
- Deny cancels the current decision without creating or revoking a grant. Do not promise an OAuth2 error callback until an existing, validated cancellation path is identified; no new OAuth2 semantics are authorized here.
- The five browser routes from ADR 035 remain stable. `/activity` and `/settings` are additive P3 routes; a binding ADR must authorize the expanded route map and visual-direction change before implementation.
- Theme and default approval persistence are per-browser preferences, not cross-device account settings. The default persistence is once and never bypasses confirmation.
- The gateway long-poll remains machine-only. The browser refreshes its existing user-scoped pending list to meet the no-reload requirement without changing that contract.
- Activity recording starts when this feature ships. Existing structured logs do not provide a queryable, user-owned historical trail to backfill.
- The retained provider OAuth2 flow necessarily navigates to a third party when the user chooses it. The zero-external-requests goal applies to automatic page resources and background traffic, not user-directed external navigation.
- This feature excludes the admin port, tenant white-labeling, full localization, and changes to OAuth2 or token semantics.
