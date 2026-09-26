---

description: "Task list for Consent UI v2 (047-redesign-consent-console)"
---

# Tasks: Consent UI v2 — Redesign End-User Consent and Console

**Input**: Design documents from `/specs/047-redesign-consent-console/`

**Prerequisites**: plan.md, spec.md, research.md, data-model.md, contracts/ui-and-configuration.md, quickstart.md

**Tests**: Per Constitution Principle VIII (Test-Driven Development & Automated Testing), automated tests are MANDATORY. Every test task precedes the implementation it drives and MUST fail on the missing behavior before that implementation starts. Do not use skips, pending markers, placeholder assertions, source-text assertions, or mock echoes.

**Organization**: Tasks are grouped by user story for traceability to spec.md.

**Single cutover (FR-028)**: This is one dependency-ordered list for one release.
Section headings group responsibilities and user stories. They are not phases, milestones, or independently releasable subsets.
Nothing merges to `main` until the Cutover Removal section and the Phase N gate pass.
Do not add feature flags, dual presentations, compatibility aliases, or old-server fallbacks at any point.
The `Phase 2` and `Phase N` headings are kept verbatim because the constitution's Task List Requirements mandate them. All other headings avoid phase numbering, as plan.md requests.

**Implementation gate**: T013 (ADR 037 acceptance) blocks every later task. Do not mark T013 done without a recorded stakeholder decision.

**Evidence file**: T001 creates `specs/047-redesign-consent-console/cutover-inventory.md`. It collects baseline, inventory, red-phase, removal, and performance evidence for the PR.

**Omitted optional sections**:
- Phase 0 (Pre-implementation Refactoring): plan.md requires refactors to stay with their consumers. A separate refactoring PR would ship a partial presentation.
- Phase 2.7 (Entity Boilerplate): the feature adds no domain entity, port, storage adapter, or HTTP handler.

**Scenario map**: 17 scenarios are active: AS-01–AS-15, AS-17, and AS-18. AS-16, FR-023, SR-004, API-002, and API-003 are retired. Never reuse their identifiers.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Can run in parallel (different files, no dependencies on incomplete tasks)
- **[Story]**: User story from spec.md (US1–US9)
- Every task names exact file paths

## Path Conventions

- Frontend: `web/` (React SPA). Aliases: `@design-system`, `@components`, `@hooks`, `@services`, `@types`, `@utils`, `@styles`, and `@copy` (added by T044)
- Backend (CSP only): `internal/adapters/http/handlers/`
- E2E: scenarios in `tests/e2e/frontend/consent_ui_v2_test.go`, page objects in `tests/e2e/pages/`, fixtures in `tests/e2e/fixtures/`, reviewed baselines in `tests/e2e/screenshots/`
- Design-system components: `web/src/design-system/components/<category>/<Component>/{Component.tsx,Component.stories.tsx,Component.test.tsx,index.ts}`

---

## Setup: Baseline and Cutover Inventory

**Purpose**: Record the starting state and every consumer that the cutover must migrate or remove. These tasks do not change runtime behavior or current guidance.

- [ ] T001 Create `specs/047-redesign-consent-console/cutover-inventory.md` with a "Baseline" section: run `just check`, `just web-test`, and `just test-e2e-frontend` on the unchanged branch; record pass and fail counts and every pre-existing failure so later regressions are attributable
- [ ] T002 [P] Add a "Consumers to migrate" section to `specs/047-redesign-consent-console/cutover-inventory.md`: record the exact `rg` command and every hit under `web/src/` and `web/index.html` for `@headlessui/react`, `framer-motion`, `apiCache`, `@components/ui/`, `components/ui/`, `PageTransition`, TypeScript imports of `design-system/tokens`, `Crimson Pro`, `Manrope`, `linear-gradient`, `bg-gradient-`, and Tailwind palette utilities (`(bg|text|border|ring|from|to|via|fill|stroke|divide|outline)-(slate|gray|zinc|neutral|stone|red|orange|amber|yellow|lime|green|emerald|teal|cyan|sky|blue|indigo|violet|purple|fuchsia|pink|rose)-[0-9]{2,3}`). T148 reruns the same commands and must get zero hits
- [ ] T003 [P] Add a "Performance baseline" section to `specs/047-redesign-consent-console/cutover-inventory.md`: run `just web-build`, then record the gzip size of the entry chunk, the `AgentGrantDetailPage` chunk, their static imports, and CSS in `web/dist/assets/`
- [ ] T004 [P] Add a "Selector changes" section to `specs/047-redesign-consent-console/cutover-inventory.md`: list each selector in `tests/e2e/pages/consent_page.go`, `approval_page.go`, `tool_authorizations_page.go`, and `sessions_page.go` that depends on text, roles, or test IDs the redesign changes (overview and detail revoke buttons, approve and deny labels, refresh button, success toasts, validity controls). FR-029 selector updates stay inside these page objects

**Checkpoint**: Baseline and inventory recorded

---

## 🔒 Phase 2: Design Preconditions (Blocking Prerequisites) [MANDATORY]

**Purpose**: Domain model, configuration, API, and database design MUST all be complete before implementation

**⚠️ CRITICAL**: No code implementation can begin until this entire phase is complete

**🔒 CONSTITUTION REQUIREMENT**: This phase maps directly to the constitution PRECONDITIONS checklist. All sub-phases (2a-2f) are included.

### Phase 2a: Domain Model & Glossary [MANDATORY]

**Constitution Reference**: Principles II (Architecture Documentation), V (Domain-Driven Design & Glossary Management)

- [ ] T005 Confirm in `specs/047-redesign-consent-console/data-model.md` that Agent, UserGrant, Permission Set, UserSession, ToolApproval, and Authorization context keep their ownership and semantics. Confirm that the only new concepts are UI state and projections: ConsentDraft, ConnectionState, Agent Origin Label, the Delegation list projection, and browser preferences (`aib.theme`: values `light, dark, system`, default `system`, scope Browser; `aib.sidebar-collapsed`: values `true, false`, default `false`, scope Browser). Correct any drift from the 2026-09-26 clarifications in `spec.md`
- [ ] T006 [P] Update the Glossary (§12) in `ARCHITECTURE.md`:
      - Rename "Connection State (feature 046 presentation)" to feature 047. List its states verbatim: "Connected", "Needs re-authentication", "Expired", "No connection". State that No connection appears only for a service an agent requires but the user has not connected. Add no Missing scopes state
      - Add **Delegation**: a principal's unexpired UserGrant to one agent, as listed on `/delegations` under the Agents navigation item
      - Add **Consent Draft**: transient selections, existing grant, duration, custom expiry, and dirty state. It is never persisted beyond the existing `consent_state` URL parameter
      - Add **Agent Origin Label**: "Verified domain: host" for validated CIMD metadata, "Registered by your administrator" when CIMD metadata is absent, or "Unverified" for a CIMD request without domain trust. It derives only from the authorization session and never claims legal publisher verification
      - Add **Delta Re-consent**: a decision on only the access that is not already in `granted_permission_sets`
      - Add **Appearance Preferences**: per-browser theme and sidebar choices that never affect a consent or tool decision
- [ ] T007 In the Consent UI v2 section of `ARCHITECTURE.md` (about line 146), change "feature 046" to "feature 047" and document these invariants:
      - Re-consent never widens a grant beyond the user's selection
      - Deny creates nothing, revokes nothing, and constructs no redirect
      - An invalid or expired authorization session is a decision error, never a console fallback
      - Console views carry no Agent Origin Label
      - Preferences never submit or preselect a decision
      - The browser never calls the gateway long-poll `GET /api/approvals`

**Checkpoint**: Domain model complete and documented

### Phase 2b: Configuration Design [MANDATORY]

**Constitution Reference**: Principle VII (Configuration-Driven Design)

- [ ] T008 Confirm that the feature adds no configuration. Per `contracts/ui-and-configuration.md`, it adds no `ui.v2` setting, environment variable, CLI option, HTML flag bootstrap, or configuration endpoint. Record "No configuration change" with that reference in `specs/047-redesign-consent-console/cutover-inventory.md`. Leave `examples/config/` and `examples/config/README.md` unchanged
- [ ] T009 [P] Verify the deployment contract in `charts/agentic-identity-broker/` (values, templates, and README). The broker serves the SPA and sets its headers. Confirm that no chart value, ingress annotation, or template sets `Content-Security-Policy` or other SPA headers that would conflict with T052. Record in `specs/047-redesign-consent-console/cutover-inventory.md` that no Helm change is needed

**Checkpoint**: Configuration requirements confirmed (none) and deployment contract verified

### Phase 2c: API Design [MANDATORY]

**Constitution Reference**: Principles IV (API Documentation & OpenAPI Transparency), X (API-First Development)

- [ ] T010 Map every screen to the existing end-user operations it calls. Confirm that each operation exists unchanged in `api/enduser/openapi.yaml`:
      - `/api/me`
      - The consent agent list, and agent detail with `session_token`
      - Agent grant read, create or update, and delete
      - `/api/third-party/{service_id}/oauth2/authorize`
      - Session list, detail, terminate, and refresh
      - `/api/approvals/pending`, and the permanent approval list and revoke
      - Approval get, approve, deny, and scope preview

      Record the map in `specs/047-redesign-consent-console/cutover-inventory.md`. Confirm that each documented response matches its handler; the session-list documentation was corrected to `{data: {sessions: [UserSessionSummary]}}` on 2026-09-27 (spec Clarifications). Confirm that no browser code calls `GET /api/approvals`
- [ ] T011 [P] Record the stakeholder confirmations in `specs/047-redesign-consent-console/cutover-inventory.md` for the PR description: API-005 allows no endpoint, response-field, runtime-response, persistence, or OAuth2/token contract change (spec Clarifications, 2026-09-26), and the documentation-only correction of the session-list response in `api/enduser/openapi.yaml` and `docs/reference/api.md` (spec Clarifications, 2026-09-27). The feature does not affect `api/admin/openapi.yaml`

**Checkpoint**: No API change confirmed and recorded

### Phase 2d: Database Design [MANDATORY]

**Constitution Reference**: Principle IX (Persistence Pattern Consistency & Database Migration Management)

- [ ] T012 Confirm that the schema does not change. The feature adds no file to `migrations/` and changes no repository in `internal/ports/storage.go`. Record this in `specs/047-redesign-consent-console/cutover-inventory.md`

**Checkpoint**: No database change confirmed

### Phase 2e: Frontend/Design System Review [MANDATORY IF FRONTEND]

**Constitution Reference**: Principle XI (Design System Compliance & Consistency)

- [ ] T013 🚦 GATE: Obtain stakeholder acceptance of ADR 037. Then:
      - Set `adrs/037-design-system-rebuilt-on-shadcn-radix.md` to **Accepted**, with the date and a decision reference
      - Add "Partially superseded by ADR 037 (UI components, animation, server-state ownership)" to `adrs/006-frontend-stack.md`
      - Add "Extended by ADR 037 (`/settings`)" to `adrs/035-root-mounted-spa.md`
      - Add an ADR 037 row to the ADR Decision Index in `AGENTS.md`

      Every later task is blocked until this task is done
- [ ] T014 Review `web/src/design-system/docs/INDEX.md`, `DECISION_TREES.md`, `COMPONENT_PAIRING_GUIDE.md`, and `COMMON_MISTAKES.md`. Then add a "Design system" section to `specs/047-redesign-consent-console/cutover-inventory.md` that records:
      - (a) The target of each component, marked as replaced in place (the existing directory is rewritten), new, or replacing a differently named component:
        - `primitives/`: Button, Badge, and Avatar in place; Separator replaces Divider; Wordmark is new
        - `inputs/`: Select, Switch, Checkbox, TextArea, and DatePicker in place; Input replaces TextInput; RadioGroup replaces Radio
        - `overlays/`: Popover and Tooltip in place; Dialog replaces Modal; DropdownMenu replaces Dropdown; Sheet is new
        - `navigation/`: Tabs in place
        - `data-display/`: Card and Table in place; TruncatedText is new
        - `feedback/`: Skeleton, Alert, EmptyState, InlineError, ErrorBoundary, and GlobalErrorBoundary in place; Toaster replaces Toast
        - `advanced/`: Accordion in place; Command is new
        - `layout/`: ConsoleShell, DecisionShell, and PageHeader are new and replace AppLayout and PageTransition
        - `web/src/design-system/theme/ThemeChoice` is new
      - (b) The replaced components to delete: `overlays/Modal`, `overlays/Dropdown`, `inputs/TextInput`, `inputs/Radio`, `primitives/Divider`, `layout/AppLayout`, `layout/PageTransition`, `feedback/Toast`, and all of `web/src/components/ui/`
      - (c) The universal components added to the design system: Wordmark, TruncatedText, ThemeChoice, and the three shells
      - (d) The story state matrix that every component must cover in light and dark themes: default, hover and focus-visible, disabled, error, loading, and overlay-open where applicable. Every story also gets a reviewed visual-regression baseline per theme (T065)
      - (e) The self-hosted assets:
        - Zalando Sans and Inter WOFF2 files (latin and latin-ext subsets) and the existing JetBrains Mono files in `web/public/fonts/`
        - Outlined wordmarks, favicon, and compact mark in `web/public/brand/`
        - No third-party font, script, or image loads
- [ ] T015 [P] After T013, make the accepted direction current in `web/src/design-system/docs/DESIGN_PRINCIPLES.md` and `web/src/design-system/docs/INDEX.md`. Remove the "proposal" framing. Move Refined Trust Architecture to a dated historical note that links ADR 037. Replace "Feature 046" with "feature 047"
- [ ] T016 [P] Publish the single semantic OKLCH token contract in `web/src/design-system/docs/COLOR_GUIDE.md` and `web/src/design-system/docs/TOKEN_GUIDE.md`. Include token names, light and dark values, and the raw-palette ESLint rule from T046. Remove the retired palette as current guidance and replace "feature 046" with "feature 047"
- [ ] T017 [P] Align examples, variants, and component selection with the accepted system in `web/src/design-system/docs/COMMON_MISTAKES.md`, `COMPONENT_ARCHETYPES.md`, and `DECISION_TREES.md`. Examples include Button `primary` as the single accent action, Dialog instead of Modal, and DropdownMenu instead of Dropdown
- [ ] T018 [P] Align composition rules and examples in `web/src/design-system/docs/COMPONENT_PAIRING_GUIDE.md` and `COMPOSITION_PATTERNS.md` with ConsoleShell, DecisionShell, PageHeader, Table, and Command, without a competing component library
- [ ] T019 [P] Update `web/src/design-system/docs/ACCESSIBILITY_GUIDE.md` and `MOTION_GUIDE.md`:
      - Keep Principle XI's WCAG 2.1 AA floor
      - Add the feature's WCAG 2.2 AA target: visible, unobscured focus and announcements
      - Require both-theme story checks
      - Use CSS-only motion at 120–200 ms with ease-out, remove movement under `prefers-reduced-motion`, and add no Framer Motion
- [ ] T020 [P] Update the onboarding and asset examples in `web/src/design-system/GettingStarted.mdx`. Update the session examples in `web/src/components/sessions/README.md` and `web/src/components/sessions/DESIGN_SYSTEM_USAGE.md`, and remove their outdated aesthetic instructions
- [ ] T021 [P] Update `AGENTS.md`, `web/AGENTS.md`, and `ARCHITECTURE.md`:
      - Separate constitutional requirements, current runtime facts, and the accepted decision
      - Replace "Feature 046" with "feature 047" in `web/AGENTS.md`
      - Point the design-system guidance at the accepted direction

**Checkpoint**: ADR accepted. Design-system usage planned. Current guidance describes one active visual system

### Phase 2f: E2E Acceptance Test Design [MANDATORY]

**Constitution Reference**: Principle XIII (End-to-End Acceptance Testing & Spec Traceability)

- [ ] T022 [P] Add helpers to `tests/e2e/pages/page.go`, with unit tests in `tests/e2e/pages/page_test.go`:
      - `SetThemePreference(value)`: a browser-context init script that writes `localStorage['aib.theme']` before any page script runs
      - `EmulateColorScheme(scheme)`
      - `ResolvedTheme(ctx)`: reads `document.documentElement.dataset.theme`
      - `FirstPaintTheme(ctx)`: an init script that records `dataset.theme` and the computed `color-scheme` in the first `requestAnimationFrame`
      - `StartRequestRecorder()` and `RecordedRequests()`: record every request URL and method, including subresources
      - `HasHorizontalPageOverflow(ctx)`: `scrollWidth > clientWidth` on `document.documentElement`
      - `EmulateZoom(percent)`: a 1280×1080 viewport divided by the zoom factor; for example, 640×540 at 200%
      - `PrimaryAccentActionCount(ctx)`: visible elements that match `[data-variant="primary"]` outside `aria-hidden="true"` and `inert` subtrees, so an open modal dialog counts only its own actions
      - `ActiveElementDescription(ctx)`
      - `TakeThemedScreenshots(ctx, stem)`: writes `<stem>_light.png` and `<stem>_dark.png` after the preference changes, the page reloads, the fonts load, and queries settle. Scenarios call it before they change server state
- [ ] T023 [P] Create `tests/e2e/pages/console_shell.go` with a `ConsoleShell` page object:
      - Navigation to Agents, Connections, Approvals, and Settings
      - `CollapseSidebar`, `IsSidebarCollapsed`, and `HasSidebar`
      - `PendingApprovalCount`
      - `OpenUserMenu` and `ChooseTheme(label)`
      - `OpenCommandPalette` (keyboard shortcut), `SearchCommandPalette(text)`, `CommandPaletteResults`, and `ChooseCommandResult(label)`
      - `WordmarkVariant`
      - `LiveRegionText`
- [ ] T024 [P] Create `tests/e2e/pages/delegations_page.go` with a `DelegationsPage` page object:
      - `Navigate` and `Search(text)`
      - `Rows()`: agent, permission-set count, and expiry for each row
      - `ColumnHeaders()`
      - `VisibleRowCount`
      - `ClickView(agent)` and `ClickRevoke(agent)`
      - `ConfirmRevoke` and `CancelRevoke`
      - `RevokeDialogText`, `EmptyStateText`, and `RowIsPending(agent)`
- [ ] T025 [P] Create `tests/e2e/pages/settings_page.go` with a `SettingsPage` page object: `Navigate`, `ChooseTheme(label)`, `SelectedTheme`, and `HasApprovalPersistenceControl`
- [ ] T026 [P] Add decision and console-detail methods to `tests/e2e/pages/consent_page.go`:
      - Decision view: `OriginLabelText`, `HasLocalhostBanner`, and `PermissionGroups()` (order, required flag, name, description, already-granted state, and read-only state)
      - Permission groups: `ExpandPermissionGroup`, `GroupServices`, `GroupHasRiskIndicator`, and `GroupShowsScopeStrings`
      - Duration and actions: `ChooseDuration(label)`, `SelectedDuration`, `CustomDateValue`, `SetCustomDate`, `ClickAllow`, `ClickDeny`, `DecisionOutcomeText`, and `DecisionErrorText`
      - Console detail: `OpenTab(label)`, `ConnectionsTabRows()` (service, state label, and action), `IsSaveBarVisible`, `CancelChanges`, `SaveChanges`, `OpenOverflowMenu`, and `ChooseRevokeAllAccess`
- [ ] T027 [P] Add these methods to `tests/e2e/pages/approval_page.go`: `ExpandArguments`, `ArgumentsText`, `ActingUser`, `RiskLabel`, `ApprovalScopeText`, `ClickApproveOnce`, `ClickApproveAndRemember`, `ChooseRememberDuration`, `HasDecisionActions`, and `ResolvedOutcomeText`
- [ ] T028 [P] Add these methods to `tests/e2e/pages/tool_authorizations_page.go`: `SectionOrder`, `PendingRows`, `ApproveRow`, `DenyRow`, `ChoosePersistenceForRow`, `ScopePreviewForRow`, `StandingDecisions`, and `RevokeStanding(tool)`
- [ ] T029 [P] Add these methods to `tests/e2e/pages/sessions_page.go`: `ConnectionRows` (provider, scope count, state label, action, and creation time), `ClickReconnect`, `ClickRefresh`, `ClickDisconnect`, `DisconnectDialogText`, and `ConfirmDisconnect`
- [ ] T030 [P] Add deterministic fixtures in `tests/e2e/fixtures/consent_ui_v2.go`, reusing the builders in `agents.go`, `grants.go`, `services.go`, `sessions.go`, and `principals.go`:
      - Agents:
        - An agent with two required and two optional permission sets across two services
        - An existing grant with a future `valid_until` that covers one optional set (AS-02)
        - An expired grant, which the existing delegation list excludes (AS-06)
        - Fourteen granted agents (SC-005)
        - An administrator-registered agent without CIMD
        - An agent that requires a service the principal has not connected (No connection, AS-10)
        - A localhost CIMD client and a validated-domain CIMD client
        - An agent with a 300-character name that contains `<script>` markup and has no publisher
      - Stored sessions for the ConnectionState precedence rows that `/sessions` can show:
        - Usable access
        - An expired refresh token
        - An expired access token with a refresh token, on a service whose mock upstream accepts refresh (AS-11)
        - An expired access token with a refresh token, on a service whose mock upstream rejects refresh, so the refresh call returns `502`
      - Approvals:
        - Pending, approved, denied, and expired tool approvals, including one without `risk_level`
        - Standing permanent allow and deny decisions
      - Cross-user checks (SR-001): a second principal that owns its own agent, session, and pending approval
      - A fixed clock value for screenshot timestamps
- [ ] T031 Add suite helpers in `tests/e2e/frontend/consent_ui_v2_helpers_test.go`:
      - `startAuthorizationRequest(...)`: drives the browser through the real `/oauth2/authorize` flow, using the pattern in `tests/e2e/frontend/cimd_flow_test.go`, and waits for `/agents/:id?session_token=…`. Never build a session token by hand
      - `forEachTheme(body)`: runs a read-only body once per theme within a single `It`, each time in a fresh browser context with `SetThemePreference`. It fails the test if the body issues a non-GET request to `/api/`. Only AS-15 uses it. AS-14 and AS-17 switch themes as their subject, and every other scenario runs once in the default theme
      - `expectNoThirdPartyOrGatewayRequests(recorded)`: asserts that every automatic request is same-origin and that none is `GET /api/approvals`
- [ ] T032 Create `tests/e2e/frontend/consent_ui_v2_test.go` with `Describe("Consent UI v2")` and `Context("Decide on an agent request")`. Add one `It` each for AS-01, AS-02, and AS-03, with the exact identifier in the `It` text and a spec reference comment.

      AS-01 asserts:
      - The Agent Origin Label for each CIMD fixture, and the localhost banner
      - Required groups before optional groups. Each group shows a name, description, and services, with no risk indicator and no scope strings
      - "Until revoked", "30 days", and "Custom date"
      - Exactly one primary accent action ("Allow") and a secondary Deny
      - The next-steps text
      - No sidebar

      AS-02 asserts that previously granted groups are collapsed, checked, and read-only, only new groups are decisions, the duration starts as "Custom date" at the existing `valid_until`, and the saved grant equals prior ∪ newly selected with the existing `valid_until` unchanged.

      AS-03 asserts:
      - The chosen duration persists through a required-service connect callback and Allow's validated continuation
      - Deny leaves the grant unchanged and shows a local outcome with no navigation
      - An expired session shows a decision error

      Capture `agent_consent_{light,dark}.png`
- [ ] T033 Add `Context("Review a tool call")` to `tests/e2e/frontend/consent_ui_v2_test.go`.

      AS-04 asserts:
      - The tool, agent, and acting user
      - The risk label, or "Risk not rated" for the fixture without `risk_level`
      - Arguments in a monospace block after expansion
      - The exact approval scope before a decision
      - Exactly one primary accent action ("Approve once"), with "Approve and remember" and Deny available
      - No sidebar

      AS-05 asserts:
      - Isolated runs for once, session, permanent, and deny each record only that decision in storage
      - Resolved and expired approvals show their outcome and no actions
      - A request resolved in a second browser context loses its actions after a refresh

      Capture `approval_review_{light,dark}.png`
- [ ] T034 Add `Context("Find and revoke an agent")` to `tests/e2e/frontend/consent_ui_v2_test.go`.

      AS-06 asserts:
      - Search by name
      - Each row shows the agent, the number of granted permission sets, and the expiry ("Until revoked" or a date). The expired-grant fixture is absent
      - No status column, status filter, or Agent Origin Label column
      - View navigates to the agent, and Revoke requires confirmation
      - At least 12 rows are visible at 1920×1080 (SC-005)

      AS-07 asserts:
      - The empty state shows the wordmark and a one-sentence explanation
      - A confirmed revoke removes the row and shows a result
      - A revoke failure injected with `page.Route` restores the row without overwriting another row that changed meanwhile

      Capture `delegations_{light,dark}.png`
- [ ] T035 Add `Context("Change an agent grant")` to `tests/e2e/frontend/consent_ui_v2_test.go`.

      AS-08 asserts that the console detail, opened without `session_token`, has:
      - A sidebar, identity, links, and Permissions and Connections tabs
      - No Agent Origin Label
      - Locked required groups and editable optional groups
      - No save bar until an edit. Cancel restores the selections, and Save persists them (assert storage)

      AS-09 asserts:
      - The overflow menu offers "Revoke all access", and its dialog names the agent
      - Cancel preserves the grant
      - Confirm revokes only this principal's grant. The second principal's grant stays untouched

      Capture `agent_detail_{light,dark}.png`
- [ ] T036 Add `Context("Manage connections")` to `tests/e2e/frontend/consent_ui_v2_test.go`.

      AS-10 asserts:
      - On `/sessions`, each stored-session fixture shows its exact label ("Connected", "Expired", or "Needs re-authentication" with Refresh), plus the provider, scope count, creation time, and matching action
      - After Refresh on the rejecting fixture, that row shows "Needs re-authentication" with Reconnect
      - `/sessions` has no row for the unconnected service, and the agent's Connections tab shows it as "No connection" with Connect
      - The text "Missing scopes" appears nowhere

      AS-11 asserts:
      - Reconnect through the mock upstream callback returns and updates the state
      - A supported refresh updates the state
      - Before confirmation, the disconnect dialog shows dependent agents and states that disconnecting does not revoke provider-side tokens

      Capture `sessions_{light,dark}.png`
- [ ] T037 Add `Context("Triage approvals")` to `tests/e2e/frontend/consent_ui_v2_test.go`.

      AS-12 asserts:
      - The pending section precedes the standing allow and deny sections
      - An inline approve with a persistence choice and scope preview changes storage only after explicit confirmation
      - Revoking a standing decision requires confirmation

      AS-13 inserts a pending approval into storage while `/approvals` is open and asserts, within `Eventually(...).WithTimeout(15 * time.Second)`:
      - The row and the sidebar count update without a reload
      - A polite live region announces the arrival, and the focused element is unchanged
      - Recorded requests include `/api/approvals/pending` and never `GET /api/approvals`
      - The second principal's approval never appears

      Capture `approvals_{light,dark}.png`
- [ ] T038 Add `Context("Use the console in either theme")` to `tests/e2e/frontend/consent_ui_v2_test.go`.

      AS-14 collapses the sidebar and chooses each theme in the user menu, then asserts:
      - The choice persists after a reload
      - `FirstPaintTheme` matches, with no flash, for explicit Light on a dark OS and explicit Dark on a light OS
      - System mode follows an emulated OS change
      - `color-scheme` matches the theme

      AS-15 runs at 320 px width and at emulated 200% zoom, in both themes through `forEachTheme`. On every console route and both decision views, it asserts:
      - No horizontal page overflow
      - Tab reaches every action with visible focus
      - The local wordmark is present: black in light mode, white in dark mode
      - At most one primary accent action
      - No third-party requests
- [ ] T039 Add `Context("Settings and command palette")` to `tests/e2e/frontend/consent_ui_v2_test.go`.

      AS-17 changes the theme in `/settings`, reloads, and opens `/approvals/:id` in the chosen theme. It asserts that `/settings` exposes no approval-persistence control and that the approval review keeps its existing choices.

      AS-18 asserts:
      - The palette opens with the keyboard shortcut
      - A search for each record type (agent, connection, approval) reaches its existing view
      - The palette toggles the theme
      - The second principal's records never appear

      Capture `settings_{light,dark}.png`
- [ ] T040 Add a `gate` mode to `tools/imgdiff/main.go`: `imgdiff gate --actual <dir> --baseline <dir> --required <comma-separated stems> --manifest <file> --threshold 0.5 --out <dir>`. First write table-driven tests in `tools/imgdiff/gate_test.go` for a passing set, a gated capture without a baseline, a gated baseline without a capture, a difference above the threshold, and an ungated capture that the gate ignores. Confirm that they fail, then implement:
      - It gates `<stem>_light.png` and `<stem>_dark.png` for the required stems `delegations`, `agent_consent`, `agent_detail`, `sessions`, `approvals`, `approval_review`, and `settings`, plus every stem listed in the manifest `tests/e2e/screenshots/visual-gate.txt`
      - Captures that are neither required nor in the manifest are documentation and are ignored
      - It writes the expected, actual, and diff PNG for each gated image
      - It exits non-zero on a gated capture without a baseline (unreviewed), a gated baseline without a capture (stale), or a difference above the threshold

      Add a `test-e2e-frontend-visual` recipe to `justfile`. It runs the frontend suite with `E2E_CAPTURE_SCREENSHOTS=true` and `GINKGO_FRONTEND_PROCS=1`, then runs `go run ./tools/imgdiff gate` from `tests/e2e/frontend/coverage/screenshots/` to `tests/e2e/screenshots/` with `--manifest tests/e2e/screenshots/visual-gate.txt`, with output in `tests/e2e/frontend/coverage/visual-diff/`. Create the manifest with a comment header and no stems; T149 adds the reviewed state stems
- [ ] T041 Add a blocking `just test-e2e-frontend-visual` step to the frontend E2E job in `.github/workflows/ci.yml`. On failure, upload `tests/e2e/frontend/coverage/visual-diff/`. In `.github/workflows/screenshots.yml`, replace the "Sync screenshots" and "Commit and push" steps: the workflow no longer writes to or commits `tests/e2e/screenshots/`, and instead uploads every capture from `tests/e2e/frontend/coverage/screenshots/` as a review artifact. Every baseline changes only in a reviewed commit
- [ ] T042 Verify the red phase. Run `just test-e2e-frontend`. Every new AS `It` must compile and fail on its behavioral expectation. Record the failing assertion for each scenario in a "Red phase" section of `specs/047-redesign-consent-console/cutover-inventory.md`. Confirm that no test uses `Skip`, `PIt`, `XIt`, a placeholder assertion, or a "red phase" comment, and that existing journeys still pass

**Checkpoint**: E2E acceptance tests written and verified to fail semantically. The visual gate and screenshot stems are configured

---

## Foundational: Design System, Themes, Data Layer, and Shells

**Purpose**: The shared design system, first-paint theming, CSP, the Query data layer, shared UI-state models, and both shells. Every user story depends on this work.

**⚠️ CRITICAL**: No user story work can begin until this section is complete

### Tooling and copy

- [ ] T043 Add dependencies with exact pinned versions to `web/package.json` and `web/package-lock.json`:
      - Runtime: `@tanstack/react-query` v5, `@tanstack/react-table` v8, `lucide-react`, `cmdk`, `sonner`, and the Radix packages that the copied shadcn Radix sources import
      - Development: `@fontsource-variable/zalando-sans` and `@fontsource-variable/inter` (font sources only, never imported at runtime), `@storybook/addon-vitest@10.3.5`, the Vitest 4 Playwright browser provider, `playwright`, and `culori`

      Keep `@headlessui/react` and `framer-motion` until T144. Run `just web-install`, and confirm that `npm --prefix web ls` reports no errors
- [ ] T044 [P] Create the copy catalogue `web/src/copy/index.ts` (FR-027). Group the strings by view and use second-person, present-tense, action-first wording. Include these strings verbatim: "Agentic Identity Broker", "Verified domain: {host}", "Registered by your administrator", "Unverified", "Risk not rated", "Until revoked", "30 days", "Custom date", "Allow", "Deny", "Approve once", "Approve and remember", "Revoke all access", "Save changes", "Cancel", "Connected", "Needs re-authentication", "Expired", "No connection", "Connect", "Reconnect", "Refresh", "Disconnect", "Light", "Dark", "System", "Show more", "Show less", and "Powered by Zalando". Pages and application components take every user-facing string from `@copy`. Design-system components receive user-facing strings through props and never import `@copy`. Add the `@copy` alias to `web/vite.config.ts`, `web/vitest.config.ts`, `web/.storybook/main.ts`, and `web/tsconfig.app.json`
- [ ] T045 [P] Write RuleTester cases in `web/eslint-rules/no-raw-palette.test.js`, run by Vitest.
      - Reject Tailwind palette utilities (for example, `bg-blue-600`, `text-neutral-500`, `border-white`, and `from-amber-50`)
      - Reject arbitrary color literals (`bg-[#fff]`, `text-[oklch(…)]`, and `[rgb(…)]`)
      - Apply both rejections with and without variant prefixes (`dark:`, `hover:`, `md:`, and `focus-visible:`), in `className` strings, template literals, `cn()`, `clsx()`, and `cva()` base and variant values
      - Accept semantic token utilities (`bg-background`, `text-foreground`, `bg-primary`, `border-border`, and `text-muted-foreground`)
- [ ] T046 Implement `web/eslint-rules/no-raw-palette.js` and register it as an `error` for `src/**/*.{ts,tsx}` in `web/eslint.config.js`. Add a `web-lint` recipe to `justfile` that runs `npm --prefix web run lint`, and run it in the web job of `.github/workflows/ci.yml`. The rule reports existing violations until T148

### Themes and CSP

- [ ] T047 [P] Write `web/src/design-system/theme/themePreference.test.ts` and `web/src/design-system/theme/themeInit.test.ts`. They assert:
      - `aib.theme` accepts `light`, `dark`, and `system`. Missing or invalid values use `system`
      - When storage is unavailable (`getItem` and `setItem` throw), the app continues with an in-memory preference
      - System mode follows `matchMedia('(prefers-color-scheme: dark)')` change events. Explicit choices ignore them
      - The script text from `theme-init.js`, run in jsdom for each preference and OS-scheme combination and for invalid and throwing storage, sets the same resolved `data-theme` and `colorScheme` on `document.documentElement` as `resolveTheme()`
      - `aib.sidebar-collapsed` accepts `true` and `false`, with a default of `false`
- [ ] T048 Implement:
      - `web/src/design-system/theme/themePreference.ts`: the keys, parsing, resolution, and safe storage
      - `web/src/design-system/theme/theme-init.js`: a dependency-free IIFE that sets `data-theme` and `style.colorScheme`. It contains no principal, credential, or configuration
      - `web/src/design-system/theme/ThemeProvider.tsx` with `useTheme()`: returns the preference, the resolved theme, and `setTheme`. It subscribes to OS changes only in System mode
- [ ] T049 [P] Write `web/build/themeInitPlugin.test.ts`. It asserts that the plugin injects the exact `theme-init.js` text as the first `<head>` script in dev and build HTML. It also asserts that the plugin emits `csp-hashes.json`, whose `script-src` entry equals `'sha256-' + base64(sha256(injected text))`
- [ ] T050 Implement `web/build/themeInitPlugin.ts` with `transformIndexHtml` and an emitted `csp-hashes.json` asset. Compute the hash with Node's `crypto`. Register the plugin in `web/vite.config.ts` and add `build/` to `web/tsconfig.node.json`. Then update `web/index.html`:
      - Add `<meta name="color-scheme" content="light dark">`
      - Set the favicon to `/brand/favicon.svg`
      - Add light and dark `theme-color` metas
      - Preload only the Zalando Sans and Inter latin subsets. Remove the Crimson Pro, Manrope, and JetBrains Mono preloads
      - Replace "Consent Management" in `<title>` and the meta descriptions with "Agentic Identity Broker"
- [ ] T051 [P] Write CSP tests in `internal/adapters/http/handlers/spa_test.go`, using a temporary static directory with `index.html` and `csp-hashes.json`.
      - The SPA response CSP contains `script-src 'self' 'sha256-…'`, `font-src 'self'`, `img-src 'self' data:`, `connect-src 'self'`, `object-src 'none'`, `base-uri 'self'`, and `frame-ancestors 'none'`
      - `script-src` never contains `'unsafe-inline'`
      - A missing or malformed `csp-hashes.json` (an entry that does not match `^'sha256-[A-Za-z0-9+/]+=*'$`) produces `script-src 'self'` only and an error log. This fails closed
      - `X-Frame-Options: DENY` and `X-Content-Type-Options: nosniff` stay
      - `/api/` paths still return 404
- [ ] T052 Implement the CSP in `internal/adapters/http/handlers/spa.go`. `NewSPAHandler` reads and validates `csp-hashes.json` once from `staticPath` and builds the header once.
      - Do not add `default-src` or `style-src` restrictions that block Radix inline positioning styles
      - Keep the constructor signature and the call at `internal/app/builder.go:1132` unchanged

      Run `just check` and `just test`

### Tokens, fonts, and brand

- [ ] T053 [P] Write `web/src/design-system/tokens/contrast.test.ts`. It parses `theme.css` and uses `culori` to assert, in light and dark themes:
      - Every text and background pair reaches at least 4.5:1
      - Every control and indicator pair reaches at least 3:1

      Cover the 74 text and control pairs and the 14 filled-status pairs from research.md §2
- [ ] T054 Create `web/src/design-system/tokens/theme.css` from the contract in `COLOR_GUIDE.md` and `TOKEN_GUIDE.md` (T016):
      - Complete semantic OKLCH tokens for `:root, [data-theme="light"]` and `[data-theme="dark"]`: background, foreground, card, popover, muted, primary (neutral blue), secondary, accent, border, input, ring, destructive, success, warning, info, their foregrounds, and a sidebar set
      - A `@media (prefers-color-scheme: dark) { :root:not([data-theme]) {…} }` fallback
      - `color-scheme` for each theme
      - Radius tokens and font families (`Zalando Sans Variable`, `Inter Variable`, `JetBrains Mono Variable`)
      - Motion durations of 120–200 ms with ease-out, and a `prefers-reduced-motion: reduce` override

      In `web/src/styles/index.css`, map the tokens with `@theme inline`. Replace the cream, sand, and taupe `:root` block, the body gradient, and the link animations. Theme scrollbars (`scrollbar-color`) and native controls (`accent-color`), and remove the `tokens/colors.css` import
- [ ] T055 [P] Copy the normal-width variable WOFF2 latin and latin-ext subsets of Zalando Sans and Inter from the fontsource packages into `web/public/fonts/`, with `LICENSE-zalando-sans.txt` and `LICENSE-inter.txt`. Rewrite `web/src/styles/fonts.css` with `@font-face` rules (`font-display: swap` and fontsource `unicode-range`) for Zalando Sans, Inter, and the existing JetBrains Mono files. Do not add SemiExpanded files
- [ ] T056 [P] Write `web/src/design-system/brand/brandAssets.test.ts`. It reads every SVG in `web/public/brand/` and asserts:
      - No `<text`, `@import`, `url(`, or `http(s)://` reference exists, other than the SVG namespace
      - Every wordmark has a `viewBox`
      - The black and white wordmarks share path geometry
- [ ] T057 Outline the supplied artwork (weights 300 and 700) from `assets/docusaurus/static/img/AIB_Wordmark_Black.svg`, `AIB_Wordmark_White.svg`, and `favicon.svg`. Save the results to `web/public/brand/` under the same file names, with text converted to paths and the Bunny Fonts import removed.
      - Derive `web/public/brand/aib-mark.svg` from the favicon, with a `currentColor` foreground
      - Delete the three originals
      - Update the consumers: lines 3–5 of `README.md`; the favicon, navbar `src` and `srcDark`, and a `'../../web/public/brand'` entry in `staticDirectories` in `assets/docusaurus/docusaurus.config.js`; and lines 11–12 of `assets/docusaurus/src/pages/index.js`

      Confirm that `just docs-build` succeeds

### Owned components

- [ ] T058 [P] In `web/src/design-system/components/primitives/{Button,Badge,Separator,Avatar,Wordmark}/`, first write each component's `.test.tsx` (variant attributes, `data-variant`, the Avatar off-origin fallback, and the Wordmark accessible name and variant) and confirm that it fails. Then:
      - Replace Button and Badge in place with owned shadcn Radix sources, and add Separator, which replaces Divider:
        - Button: variants `primary` (accent), `secondary`, `outline`, `ghost`, and `destructive`. It renders `data-variant` and supports `asChild`. Its loading state uses Lucide `Loader2` and `aria-busy`
        - Badge: `neutral`, `outline`, `success`, `warning`, `danger`, and `info`
      - Replace Avatar in place. It renders only same-origin sources or a local fallback, never an off-origin `src`
      - Add Wordmark. It renders both local wordmarks and shows the one that matches `[data-theme]` through CSS, so it never flashes. Its accessible name comes from a required `label` prop, and its `compact` variant uses `aib-mark.svg`

      Give each component stories for the T014 state matrix
- [ ] T059 [P] In `web/src/design-system/components/inputs/{Input,Select,Switch,Checkbox,RadioGroup,TextArea,DatePicker}/`, first write tests for labels, `aria-invalid`, keyboard operation, and the DatePicker minimum date, and confirm that they fail. Then:
      - Replace Select, Switch, and Checkbox in place with owned Radix sources
      - Add Input, which replaces TextInput, and RadioGroup, which replaces Radio
      - Migrate TextArea in place
      - Migrate DatePicker in place to a native `<input type="date">` with a minimum date and an inline validation message

      Add stories for the T014 state matrix
- [ ] T060 [P] In `web/src/design-system/components/overlays/{Dialog,DropdownMenu,Popover,Tooltip,Sheet}/`, first write tests for focus trapping, Escape, and focus return, and confirm that they fail. Then own the shadcn Radix sources: replace Popover and Tooltip in place, add Dialog (replacing Modal) and DropdownMenu (replacing Dropdown), and add Sheet. Use CSS transitions and `@starting-style` (120–200 ms, no movement under reduced motion). Add stories with overlay-open states in both themes
- [ ] T061 [P] For each component below, first write its tests and confirm that they fail, then build it with stories:
      - Replace Tabs (Radix) in place in `web/src/design-system/components/navigation/Tabs/`
      - Replace Card in place in `web/src/design-system/components/data-display/Card/`
      - Replace `web/src/design-system/components/data-display/Table/` in place with owned Table presentation parts: `Table`, `TableHeader`, `TableBody`, `TableRow`, `TableHead`, `TableCell`, and `TableCaption`. They use compact row density. Below the `sm` breakpoint, rows stack as label and value groups that keep every action. A `Narrow` story with a play function asserts that a 320 px viewport has no horizontal overflow and that all row actions stay visible
      - Add TruncatedText in `web/src/design-system/components/data-display/TruncatedText/`: escaped text, a CSS line clamp with a `lines` prop (2 for names and descriptions, 1 in table cells), and an expand toggle with `aria-expanded` whose labels come from required `expandLabel` and `collapseLabel` props
- [ ] T062 [P] In `web/src/design-system/components/feedback/`, first write tests for Toaster announcements and theme, Skeleton, and the migrated components, and confirm that they fail. Then:
      - Replace Skeleton in place, and add Toaster, which replaces Toast. Toaster wraps Sonner, follows `useTheme()`, and makes polite announcements
      - Migrate Alert, EmptyState (with an optional wordmark), InlineError, ErrorBoundary, and GlobalErrorBoundary in place to semantic tokens

      Add stories for the T014 state matrix
- [ ] T063 [P] In `web/src/design-system/components/advanced/{Accordion,Command}/`, first write keyboard-navigation tests and confirm that they fail. Then replace Accordion (Radix) in place and add Command (cmdk) from owned shadcn sources. No barrel that decision routes import may re-export `advanced/Command` or `data-display/Table`, so consumers import the concrete module path. Add stories
- [ ] T064 First write `web/src/design-system/theme/ThemeChoice.test.tsx` and confirm that it fails. Then build `web/src/design-system/theme/ThemeChoice.tsx` with a story. It is a RadioGroup of Light, Dark, and System, wired to `useTheme()`, with option labels from a required `labels` prop. It reflects the current preference, calls `setTheme`, and is keyboard operable

### Storybook gate

- [ ] T065 Configure Storybook for both themes:
      - In `web/.storybook/preview.ts`, use `withThemeByDataAttribute` (`light` and `dark`, with the `data-theme` attribute on `html`). Remove the cream, sand, and navy backgrounds, and set `parameters.a11y.test = 'error'`
      - Add `@storybook/addon-vitest` to `web/.storybook/main.ts`
      - Convert `web/vitest.config.ts` to three projects: `unit` (the current jsdom configuration), `storybook-light`, and `storybook-dark`. The two Storybook projects run in browser mode with headless Playwright Chromium, and each applies its theme global through project annotations in `web/.storybook/vitest.setup.ts`
      - Add story visual regression (Principle XI). In `web/.storybook/vitest.setup.ts`, add a Vitest-only project-level `afterEach` annotation that waits for fonts and asserts `toMatchScreenshot('<story id>-<theme>')` on the story root. Configure `browser.expect.toMatchScreenshot` with the pixelmatch comparator and a `resolveScreenshotPath` that stores Linux Chromium baselines as `web/.storybook/__screenshots__/<story id>-<theme>.png`
- [ ] T066 Add Storybook recipes and a CI gate:
      - Add `web-storybook-build` (which wraps `npm --prefix web run build-storybook`), `web-storybook-test` (which runs both Storybook projects, including the screenshot comparison, and never updates baselines), and `web-storybook-visual-candidates` (which runs both projects with `--update`) to `justfile`
      - Add a step to `.github/workflows/screenshots.yml` that runs `just web-storybook-visual-candidates` and uploads the changed images in `web/.storybook/__screenshots__/` as a review artifact without committing them. Review and commit the initial baselines from that artifact
      - Restrict `web-test` and `web-test-coverage` to the `unit` project
      - Add a blocking step to the web job in `.github/workflows/ci.yml` that runs `npx playwright install --with-deps chromium` first
      - Prove the gate with a temporary low-contrast story, and separately with a temporary style change to an existing story; each must fail both projects. Remove both before you commit

### Data layer and shared UI-state models

- [ ] T067 [P] Write these tests:
      - `web/src/services/query/queryKeys.test.ts`: every key starts with `['principal', principal]`, followed by the resource, identifiers, and filters
      - `web/src/services/query/QueryProvider.test.tsx`: children wait for `/api/me`. On a principal change or a 401, the provider cancels queries and removes the previous principal's data before it renders. Nothing is written to `localStorage` or `sessionStorage`
      - `web/src/hooks/usePendingApprovals.test.ts`, with fake timers:
        - A refetch runs every 10 000 ms while the page is visible, and none runs while `document.hidden` is true
        - A refetch runs immediately on window focus and after approve, deny, and revoke mutations
        - A network error keeps the last list marked stale and never yields a count of 0
        - Only `approvalApi.listPendingApprovals` is called
      - `web/src/hooks/optimisticRevoke.test.ts`:
        - Confirmation precedes the mutation, and matching reads are cancelled
        - Only the affected record is marked pending and saved for rollback
        - A rollback restores only that record and does not overwrite newer unrelated rows
        - A second action on the same record is disabled
        - Lists, details, and the pending count are invalidated on settle
        - Success is reported only after the server accepts the revocation, and nothing retries automatically
- [ ] T068 First extend `web/src/services/api/consent.test.ts` and `client.test.ts`, and add `sessions.test.ts` and `approvals.test.ts`. Assert that every read forwards its `signal` to `apiClient`, that no read or mutation touches `apiCache`, and that a 401 notifies auth-loss subscribers. Confirm that they fail. Then update `web/src/services/api/consent.ts`, `sessions.ts`, and `approvals.ts`:
      - Every read accepts `{ signal?: AbortSignal }` and forwards it to `apiClient`
      - Remove every `apiCache` read, write, and invalidation, because TanStack Query owns caching

      In `web/src/services/api/client.ts`, expose an auth-loss subscription that fires on a 401
- [ ] T069 Implement `web/src/services/query/queryClient.ts`, `queryKeys.ts`, and `QueryProvider.tsx`, following the "Data and mutation contract" section in `contracts/ui-and-configuration.md`. Mutations never retry. Queries retry a bounded number of times and never retry a 4xx
- [ ] T070 Implement `web/src/hooks/optimisticRevoke.ts` and `web/src/hooks/usePendingApprovals.ts`. `usePendingApprovals` is one shared console-level query for `/api/approvals/pending`, with `refetchInterval: 10_000`, `refetchIntervalInBackground: false`, `refetchOnWindowFocus: true`, and a stale flag on error
- [ ] T071 [P] Write `web/src/components/consent/consentDraft.test.ts`. It asserts:
      - Required groups come first, then optional groups
      - Required groups and required services are locked
      - Groups in `granted_permission_sets` are marked already granted (collapsed, checked, and read-only in decision context) and are excluded from new decisions
      - On re-consent, the initial duration is Until revoked for a null `valid_until` and Custom date at the existing `valid_until` otherwise. Unless the user changes the duration, `toGrantRequest()` sends the existing `valid_until` unchanged; a changed duration applies to the whole grant
      - `consent_state` from the URL restores the selections
      - `toGrantRequest()` returns existing ∪ newly selected services. It never drops an existing selection and never adds an unselected one
      - Dirty detection works, and `reset()` restores the loaded state
      - `resolveValidUntil('until-revoked' | '30-days' | 'custom', date, now)` omits `valid_until` for Until revoked, uses now + 30 days for 30 days, and rejects a custom date that is not after today
      - Its output matches the `valid_until` format that `web/src/components/consent/GrantValidityControl.tsx` and `web/src/hooks/useUpdateValidity.ts` produce today
- [ ] T072 Implement `web/src/components/consent/consentDraft.ts` with the ConsentDraft transitions from data-model.md. It adds no persistence beyond the existing `consent_state` URL parameter
- [ ] T073 [P] Write `web/src/components/sessions/connectionState.test.ts`. It covers each row of the data-model.md precedence table, in order:
      - "No session for a service the agent requires" (`connectionStatus: not_connected`) → No connection, with Connect. Only the agent Connections tab and the consent service prompt use this row
      - "Known rejected refresh result": a `409` or `502` response to the refresh call in the current page → Needs re-authentication, with Reconnect
      - "Refresh token expired, or access expired without usable refresh capacity" → Expired, with Reconnect
      - "Access expired with apparently usable refresh token, but refresh not yet successful" → Needs re-authentication with a refresh explanation, with Refresh when supported and Reconnect otherwise
      - "Usable access" → Connected, with refresh and a confirmed disconnect
      - "Session read fails" → an error or an explicitly stale prior state, never Connected

      It also asserts:
      - A refresh failure caused only by the network, or any other non-authoritative failure, keeps the last authoritative state and marks it stale
      - A `404` refresh response requests a list refetch
      - A reload discards a known rejected refresh result, so the session fields decide the state again
      - An unknown token lifetime follows the existing usability semantics
      - No state named Missing scopes exists
- [ ] T074 Implement `web/src/components/sessions/connectionState.ts`, using only the existing `SessionSummary` fields from `web/src/services/api/sessions.ts`, refresh responses in the current page, and `ServiceRequirementForUser.connectionStatus`
- [ ] T075 [P] Write these tests:
      - `web/src/hooks/useAgentGrant.test.ts`: the grant read is keyed by principal and agent. `useSaveGrant` waits for the server, never updates optimistically, and invalidates the grant, the delegations list, and the agent detail
      - `web/src/hooks/useRevokeGrant.test.ts`: revocation uses `optimisticRevoke`
      - `web/src/components/consent/RevokeAgentDialog.test.tsx`: the dialog names the agent and explains the effect. Cancel preserves the grant, confirm calls revoke once, and the pending state disables confirm
- [ ] T076 Implement `web/src/hooks/useAgentGrant.ts` (`useAgentGrant` and `useSaveGrant`), `web/src/hooks/useRevokeGrant.ts`, and `web/src/components/consent/RevokeAgentDialog.tsx`. The dialog is built on Dialog and uses strings from `@copy`

### Shells, routing, and bundle guard

- [ ] T077 [P] Write `ConsoleShell.test.tsx`, `DecisionShell.test.tsx`, and `PageHeader.test.tsx` in `web/src/design-system/components/layout/{ConsoleShell,DecisionShell,PageHeader}/`:
      - ConsoleShell has a skip link, `nav` and `main` landmarks, and a Wordmark. It provides `search` and `userMenu` slots and a polite live region. Its navigation contains Agents → `/delegations`, Connections → `/sessions`, and Approvals → `/approvals` with a pending-count badge. The collapse toggle persists `aib.sidebar-collapsed` and shows the compact mark. Below the `md` breakpoint, the sidebar opens in a Sheet
      - DecisionShell has no `nav`. It has a Wordmark header, a centered column no wider than 640 px, a `main` landmark, and a footer with optional monochrome "Powered by Zalando" text
      - PageHeader renders a title, a one-line purpose, and an optional primary action
- [ ] T078 Implement ConsoleShell, DecisionShell, and PageHeader in `web/src/design-system/components/layout/`. Add stories for desktop, collapsed, and 320 px with the Sheet open, in both themes. The shells are presentational and fetch no data
- [ ] T079 [P] Rewrite `web/src/App.test.tsx`. It asserts:
      - `/` replaces the history entry with `/delegations`
      - `/delegations`, `/sessions`, `/approvals`, and `/agents/:id` without `session_token` render inside ConsoleShell
      - `/agents/:id?session_token=…` and `/approvals/:id` render inside DecisionShell, without `nav`
      - Unknown paths render ErrorPage
      - Providers mount in this order: ThemeProvider → QueryProvider → Toaster
- [ ] T080 Implement routing and app chrome:
      - Mount the providers in `web/src/main.tsx`
      - `web/src/components/layout/ConsoleLayout.tsx`: ConsoleShell, the `usePendingApprovals` count, and user info, rendered around `<Outlet/>`
      - `web/src/components/layout/AgentRoute.tsx`: selects the decision or console context only from the presence of `session_token`
      - `web/src/App.tsx`: layout routes, with every page lazy-loaded, and a `LoadingFallback` built from Skeleton

      Existing pages render inside the new layouts until their story replaces them. `ToastProvider` stays mounted until T142
- [ ] T081 [P] Write `web/build/decisionBundle.test.ts` as a Vitest `bundle` project that reads `web/dist/.vite/manifest.json`. For each decision route (`AgentDecisionPage` and `ApprovalPage`) separately, take the static import graph of the entry and that route's chunk, together with their CSS. Assert that each graph is under 150 kB gzip-compressed (SC-002 applies to the consent graph; the approval graph uses the same budget) and contains no module from `@tanstack/react-table`, `cmdk`, `data-display/Table`, or `advanced/Command`
- [ ] T082 Enable `build.manifest` in `web/vite.config.ts`. Add a `web-bundle-check` recipe to `justfile` that builds and then runs the `bundle` project, and run it in the web job of `.github/workflows/ci.yml`. The recipe goes green after US1 and US2

**Checkpoint**: The foundation is ready. Tokens, themes, CSP, components, Storybook gate, data layer, and shells exist. User stories can start

---

## User Story 1: Decide on an Agent Request (Priority: P1)

**Goal**: A focused, single-column decision on `/agents/:id?session_token=…`. It shows a truthful Agent Origin Label, permission groups (required first), duration, one Allow action, a non-mutating Deny, and delta re-consent (AS-01, AS-02, AS-03; FR-002, FR-007–FR-012, FR-026; SR-002)

**Independent Test**: Start a real authorization request. Review the groups, connect a required service, then Allow. Repeat with an existing grant and with Deny. The console is not needed

### Tests for User Story 1 [MANDATORY - Principle VIII] ⚠️

- [ ] T083 [P] [US1] Write `web/src/components/consent/origin.test.ts`. It asserts:
      - An absent `cimd_metadata` → "Registered by your administrator"
      - A `verified_domain` that is not loopback → "Verified domain: {host}"
      - `localhost`, `*.localhost`, `127.0.0.0/8`, and `[::1]`, with or without a port → "Unverified", plus the banner flag
      - No label ever mentions a publisher
      - The console context (no session) → no label
- [ ] T084 [P] [US1] Write `web/src/pages/AgentDecisionPage.test.tsx`. Port the preserved decision assertions from `web/src/pages/AgentGrantDetailPage.test.tsx` and `AgentGrantDetailPage.integration.test.tsx`: safe-redirect validation, the `consent_state` round trip, the service-login URL, and the CIMD advanced details. Then assert:
      - Layout and identity:
        - DecisionShell renders without `nav`
        - A logo from a third-party origin renders the local fallback and issues no image request
        - Markup in the agent name renders as text, and a 300-character name is truncated with an accessible expand control
        - No publisher field appears, and the governance link appears when available
        - The Agent Origin Label and the localhost banner appear
      - Access:
        - One plain-language sentence describes the requested access
        - Groups are ordered required first. Each shows a name, a description, and its services on expansion, with no risk indicator and no scope strings
        - On re-consent, granted groups are collapsed, checked, and read-only, and the duration starts from the existing validity
        - The three duration choices appear, with custom-date validation
      - Actions:
        - Exactly one `data-variant="primary"` action ("Allow"), and a non-destructive Deny
        - The next-steps text names `/delegations`
        - Allow saves the merged selection, then follows the existing `isSafeRedirectUrl` continuation
        - Deny issues no mutation or navigation and shows a local outcome
        - An expired or invalid session renders a decision error, never the console view

### Implementation for User Story 1

- [ ] T085 [US1] Implement `web/src/components/consent/origin.ts`. Move the loopback detection out of `web/src/components/consent/CIMDLocalhostWarning.tsx`
- [ ] T086 [US1] Implement `web/src/hooks/useAgentDecision.ts`. It calls `getAgentDetail(agentId, { sessionToken, signal })` with the key `['principal', p, 'agent-decision', agentId, sessionToken]`, `gcTime: 0`, and `staleTime: 0`, so the authorization context is never cached as ordinary agent detail. It combines the result with `useAgentGrant` and requests the agent detail and the grant in parallel once the principal is known, never one after the other
- [ ] T087 [P] [US1] Build these components in `web/src/components/consent/`:
      - `AgentIdentityHeader.tsx`: an Avatar fallback, a TruncatedText name, governance and documentation links with `rel="noopener noreferrer"`, and an optional Agent Origin Label rendered with Badge. It shows no publisher field
      - `LocalhostBanner.tsx`: a prominent Alert that replaces `CIMDLocalhostWarning` and keeps its warning content
      - `CIMDDetails.tsx`: the existing CIMD advanced-details disclosure (client ID, redirect URI, and requested scopes), rebuilt on Accordion with monospace values
- [ ] T088 [P] [US1] Build `web/src/components/consent/PermissionGroupList.tsx` and `PermissionGroupItem.tsx` on Accordion, Checkbox or Switch, and Badge. Each item shows a Lucide icon, the permission-set name and description, and a locked required control or an optional toggle. Service names are one expansion away. Already-granted groups are collapsed, checked, and read-only in decision context. Show no risk indicator and no scope strings
- [ ] T089 [P] [US1] Build these components in `web/src/components/consent/`:
      - `DurationChoice.tsx`: a RadioGroup with "Until revoked", "30 days", and "Custom date", plus a DatePicker. Its initial value and validation come from `consentDraft`
      - `ConsentActions.tsx`: a primary Allow, a secondary Deny, and the next-steps text
      - `ServiceConnectPrompt.tsx`: connects a required service. It encodes the draft into `consent_state` and navigates to the existing `/api/third-party/{serviceId}/oauth2/authorize?redirect_uri=…` URL
- [ ] T090 [US1] Implement `web/src/pages/AgentDecisionPage.tsx` in DecisionShell from T085–T089, and switch the decision branch of `web/src/components/layout/AgentRoute.tsx` to it. Move the session-token, `consent_state`, redirect, and service-login logic from `web/src/pages/AgentGrantDetailPage.tsx` (about lines 47–64 and 230–272) without changing its semantics
- [ ] T091 [US1] Update the decision-view selectors in `tests/e2e/pages/consent_page.go` only, following T004. The decision-context journeys in `cimd_consent_test.go`, `cimd_flow_test.go`, `selection_preservation_test.go`, and `permission_sets_frontend_test.go` must pass with unchanged assertions
- [ ] T092 [US1] Run `ginkgo -v --focus "AS-0[1-3]" ./tests/e2e/frontend/` and the journeys from T091 until they pass, then run `just web-test`. Measure SC-002 once with T204's procedure and record it in the "Performance" section of `cutover-inventory.md`; fix a miss before continuing

**Checkpoint (not a release)**: AS-01–AS-03 pass on the branch

---

## User Story 2: Review a Tool Call (Priority: P1)

**Goal**: A focused `/approvals/:id` review with truthful identity, risk, arguments, and scope. "Approve once" is the only accent action. Resolved and expired approvals show outcomes without actions (AS-04, AS-05; FR-013)

**Independent Test**: Open a pending approval and resolve it with each persistence choice in separate runs. Open a resolved and an expired approval

### Tests for User Story 2 [MANDATORY - Principle VIII] ⚠️

- [ ] T093 [P] [US2] Write `web/src/hooks/useApprovalReview.test.ts`. It asserts:
      - The approval detail query is keyed by principal and ID
      - Approve and deny wait for the server, with no optimistic change
      - A 409 or 410 conflict refetches and exposes the current state
      - Settlement invalidates the pending query
- [ ] T094 [P] [US2] Rewrite `web/src/components/approvals/ApprovalReviewPage.test.tsx`. Keep its preserved persistence and scope-preview assertions, and keep `ApprovalScopeEditor.test.tsx`. Then assert:
      - Identity and risk:
        - DecisionShell renders without `nav`
        - The page shows the tool, the agent, and the acting user (the principal from `/api/me`)
        - The server risk level appears as a labelled Badge with an accessible explanation, or as "Risk not rated" when `risk_level` is absent
        - Agent-supplied text is escaped and truncated
      - Arguments and scope:
        - Arguments are collapsed in a monospace block with an expand control
        - The exact approval scope from the preview appears before a decision
      - Actions:
        - Exactly one `data-variant="primary"` action ("Approve once")
        - "Approve and remember" reveals the session and permanent choices with the scope editor and requires confirmation
        - Deny is secondary, and no saved default is preselected
        - Resolved or expired approvals show their outcome and no action buttons

### Implementation for User Story 2

- [ ] T095 [US2] Implement `web/src/hooks/useApprovalReview.ts`
- [ ] T096 [P] [US2] Rebuild `web/src/components/approvals/ToolCallCard.tsx` (tool, agent, acting user, and collapsible monospace arguments) and `web/src/components/approvals/RiskBadge.tsx` on design-system primitives, using strings from `@copy`
- [ ] T097 [P] [US2] Rebuild these files in `web/src/components/approvals/` on design-system primitives: `PersistenceSelector.tsx`, `ApprovalScopeEditor.tsx`, `ApprovalConfirmation.tsx`, `ApprovalErrorBanner.tsx`, `ApprovalLoadingSkeleton.tsx`, and `ApprovalRequestSummary.tsx`. Keep the once, session, and permanent behavior and the scope-preview validation unchanged
- [ ] T098 [US2] Rebuild `web/src/components/approvals/ApprovalReviewPage.tsx` in DecisionShell, and wire `web/src/pages/ApprovalPage.tsx` to `useApprovalReview`
- [ ] T099 [US2] Update selectors in `tests/e2e/pages/approval_page.go` only, so that `tests/e2e/frontend/approval_ui_test.go` passes with unchanged assertions
- [ ] T100 [US2] Run `ginkgo -v --focus "AS-0[45]" ./tests/e2e/frontend/` and `approval_ui_test.go` until they pass. `just web-bundle-check` passes from this point

**Checkpoint (not a release)**: AS-04 and AS-05 pass. Both P1 decision flows work on the branch

---

## User Story 3: Find and Revoke an Agent (Priority: P2)

**Goal**: A dense, searchable `/delegations` list with a confirmed revoke and a meaningful empty state (AS-06, AS-07; FR-014; SR-001, SR-003)

**Independent Test**: Search for an agent, open it, and revoke its grant with confirmation. Inject a revoke failure. View the empty state

### Tests for User Story 3 [MANDATORY - Principle VIII] ⚠️

- [ ] T101 [P] [US3] Write `web/src/hooks/useDelegations.test.ts`. It asserts that the delegations list is keyed by principal, exposes `activeGrantCount` as the permission-set count, drops a row once its `expiresAt` passes (fake timers, without a refetch), and that revoking through `useRevokeGrant` removes only the affected row and rolls back only that row on failure
- [ ] T102 [P] [US3] Write `web/src/pages/DelegationsPage.test.tsx`. It asserts:
      - The PageHeader shows a title and a one-line purpose
      - Table columns show the agent (fallback logo and a truncated, escaped name), the number of granted permission sets from `activeGrantCount`, and the expiry in monospace ("Until revoked" when `expiresAt` is null). There is no status column, status filter, or Agent Origin Label column
      - A row whose `expiresAt` passes while the page is open disappears
      - Name search is case-insensitive
      - Row actions use non-accent variants, and the page has no accent action
      - View links to `/agents/:id`
      - Revoke opens `RevokeAgentDialog`. Confirm marks the row pending, then removes it and shows a success toast after the server succeeds. A failure restores the row and shows an error toast
      - The empty state shows the wordmark and a one-sentence explanation of a delegation

### Implementation for User Story 3

- [ ] T103 [US3] Implement `web/src/hooks/useDelegations.ts`
- [ ] T104 [P] [US3] Build `web/src/components/delegations/DelegationsTable.tsx` (TanStack Table with `data-display/Table` at compact density)
- [ ] T105 [US3] Implement `web/src/pages/DelegationsPage.tsx`, and route `/delegations` to it in `web/src/App.tsx`
- [ ] T106 [US3] Update the overview selectors in `tests/e2e/pages/consent_page.go` (`NavigateToOverview`, `IsOverviewRevokeButtonPresent`, and `ClickOverviewRevokeButton`), so that the overview scenarios in `tests/e2e/frontend/revoke_grant_flow_test.go` pass with unchanged assertions
- [ ] T107 [US3] Run `ginkgo -v --focus "AS-0[67]" ./tests/e2e/frontend/` and the overview revoke journeys until they pass

**Checkpoint (not a release)**: AS-06 and AS-07 pass

---

## User Story 4: Change an Agent Grant (Priority: P2)

**Goal**: A console detail on `/agents/:id` without a session. It has tabs, locked required groups, editable optional groups, a sticky save bar that appears only for edits, and Revoke all access in the overflow menu (AS-08, AS-09; FR-009, FR-010, FR-015, FR-016)

**Independent Test**: Open an existing grant and edit an optional group or the duration. Cancel in one run and save in another. Revoke all access from the overflow menu

**Depends on**: US1 components T087–T089 (AgentIdentityHeader, PermissionGroupList, and DurationChoice)

### Tests for User Story 4 [MANDATORY - Principle VIII] ⚠️

- [ ] T108 [P] [US4] Write `web/src/hooks/useAgentDetail.test.ts`. It asserts that the console-context agent detail is keyed `['principal', p, 'agent', agentId]` and is never requested with `session_token`
- [ ] T109 [P] [US4] Write `web/src/pages/AgentConsolePage.test.tsx`. It asserts:
      - The page renders in ConsoleShell. AgentIdentityHeader shows no Agent Origin Label and no publisher field, and links appear
      - The page has Permissions and Connections tabs
      - Required groups are locked. Optional groups and the duration are editable
      - The sticky "Cancel"/"Save changes" bar is absent until an edit. Cancel restores the loaded selections. Save persists through `useSaveGrant` and hides the bar
      - The bar never obscures the focused element
      - The header overflow menu offers "Revoke all access", with no resting red primary action. Its dialog names the agent. Cancel preserves access, and confirm revokes and returns to `/delegations`
      - The Connections tab lists this agent's required services: a `not_connected` service shows No connection with Connect, and a connected service shows its `deriveConnectionState` label

### Implementation for User Story 4

- [ ] T110 [US4] Implement `web/src/hooks/useAgentDetail.ts`
- [ ] T111 [P] [US4] Build `web/src/components/consent/GrantEditBar.tsx` and `web/src/components/consent/AgentOverflowMenu.tsx`. GrantEditBar is sticky and sets `scroll-padding-bottom` so focus stays visible. AgentOverflowMenu is a DropdownMenu that opens `RevokeAgentDialog`
- [ ] T112 [P] [US4] Build `web/src/components/consent/AgentConnectionsTab.tsx`. It lists the services the agent requires by joining `services[].connectionStatus` from the agent detail with `useConnections`: `not_connected` shows No connection with a Connect link to the existing authorize URL, and a connected service shows its ConnectionState badge and a link to `/sessions`
- [ ] T113 [US4] Implement `web/src/pages/AgentConsolePage.tsx`, reusing AgentIdentityHeader, PermissionGroupList, DurationChoice, and `consentDraft`. Switch the console branch of `web/src/components/layout/AgentRoute.tsx` to it
- [ ] T114 [US4] Update the detail selectors in `tests/e2e/pages/consent_page.go`: `GetRevokeButton`, `ClickRevokeButton`, and `IsRevokeButtonPresent` now go through the overflow menu, and the save-button methods change too. These journeys must pass with unchanged assertions: `consent_flow_test.go`, `csrf_grant_save_test.go`, the detail scenarios of `revoke_grant_flow_test.go`, and the console scenarios of `permission_sets_frontend_test.go`
- [ ] T115 [US4] Run `ginkgo -v --focus "AS-0[89]" ./tests/e2e/frontend/` and the journeys from T114 until they pass

**Checkpoint (not a release)**: AS-08 and AS-09 pass. Both `/agents/:id` contexts are migrated

---

## User Story 5: Manage Connections (Priority: P2)

**Goal**: `/sessions` shows truthful connection states derived from existing session fields. It offers reconnect, refresh, and a confirmed disconnect, with the provider-token warning (AS-10, AS-11; FR-017–FR-019)

**Independent Test**: Inspect every connection-state fixture, complete a provider callback, refresh a supported session, and disconnect after reading the warning

### Tests for User Story 5 [MANDATORY - Principle VIII] ⚠️

- [ ] T116 [P] [US5] Write `web/src/hooks/useConnections.test.ts`. It asserts:
      - The sessions list is keyed by principal
      - A refresh waits for the server and recomputes the state
      - A `409` or `502` refresh response yields Needs re-authentication with Reconnect, a `404` refetches the list, and a network failure keeps the prior state and marks it stale
      - Disconnect uses `optimisticRevoke` after confirmation
- [ ] T117 [P] [US5] Write `web/src/pages/ConnectionsPage.test.tsx`. It asserts:
      - Each row shows the provider, the scope count, the ConnectionState label, and the creation time in monospace, below a PageHeader. Rows come only from stored sessions, so No connection never appears on this page
      - Row actions use non-accent variants, and the page has no accent action
      - Reconnect navigates to the existing `/api/third-party/{serviceId}/oauth2/authorize` URL
      - Refresh appears only when supported
      - The Disconnect dialog lists dependent agents from `getSessionDetails`, and states that disconnecting does not revoke provider-side tokens
      - The existing `success`, `error`, and `error_description` callback parameters produce the same outcomes as `web/src/pages/ThirdPartySessionsPage.tsx` today
      - A failed list read shows a retry action, never Connected
      - The text "Missing scopes" never appears

### Implementation for User Story 5

- [ ] T118 [US5] Implement `web/src/hooks/useConnections.ts`
- [ ] T119 [P] [US5] Build `web/src/components/sessions/ConnectionsTable.tsx`, `web/src/components/sessions/ConnectionStateBadge.tsx`, and `web/src/components/sessions/DisconnectDialog.tsx`. DisconnectDialog replaces `TerminationDialog.tsx`
- [ ] T120 [US5] Implement `web/src/pages/ConnectionsPage.tsx`, porting the callback handling from `web/src/pages/ThirdPartySessionsPage.tsx` (about lines 95–110). Route `/sessions` to it in `web/src/App.tsx`
- [ ] T121 [US5] Update selectors in `tests/e2e/pages/sessions_page.go` only, so that `tests/e2e/frontend/session_refresh_button_test.go` passes with unchanged assertions
- [ ] T122 [US5] Run `ginkgo -v --focus "AS-1[01]" ./tests/e2e/frontend/` and `session_refresh_button_test.go` until they pass

**Checkpoint (not a release)**: AS-10 and AS-11 pass

---

## User Story 6: Triage Approvals (Priority: P2)

**Goal**: `/approvals` shows the pending queue first, with inline decisions and scope preview. Standing decisions appear below with Revoke. A shared 10-second pending refresh updates the queue and sidebar count and announces arrivals (AS-12, AS-13; FR-020, FR-021; API-004)

**Independent Test**: Resolve a pending request inline, revoke a standing decision, and watch a new request arrive without a reload

### Tests for User Story 6 [MANDATORY - Principle VIII] ⚠️

- [ ] T123 [P] [US6] Write `web/src/hooks/useStandingApprovals.test.ts`. It asserts that permanent approvals are keyed by principal, that revoke uses `optimisticRevoke`, and that approve and deny from the queue wait for the server and refresh the pending query
- [ ] T124 [P] [US6] Write `web/src/pages/ApprovalsPage.test.tsx` and `web/src/components/layout/ConsoleLayout.test.tsx`. They assert:
      - The pending section appears above the standing allow and deny sections
      - Each pending row shows the tool, agent, risk label, and age. Its inline Approve and Deny use non-accent variants, and the page has no accent action
      - Choosing session or permanent shows the scope preview before confirmation, and nothing mutates without an explicit click
      - Standing decisions revoke only after confirmation
      - A newly fetched pending item appears, and a polite live region announces it while `document.activeElement` stays unchanged
      - With the queue and the sidebar both mounted, `listPendingApprovals` runs once per interval

### Implementation for User Story 6

- [ ] T125 [US6] Implement `web/src/hooks/useStandingApprovals.ts`
- [ ] T126 [P] [US6] Build `web/src/components/approvals/PendingApprovalsTable.tsx` and `web/src/components/approvals/InlineApprovalActions.tsx`, reusing `PersistenceSelector` and `ApprovalScopeEditor`
- [ ] T127 [P] [US6] Build `web/src/components/approvals/StandingDecisionsTable.tsx` and `web/src/components/approvals/ApprovalArrivalAnnouncer.tsx`. The announcer compares pending IDs between refetches and announces arrivals and decision-state changes
- [ ] T128 [US6] Implement `web/src/pages/ApprovalsPage.tsx`, and route `/approvals` to it in `web/src/App.tsx`
- [ ] T129 [US6] Update selectors in `tests/e2e/pages/tool_authorizations_page.go` only, so that `tests/e2e/frontend/tool_authorizations_test.go` passes with unchanged assertions
- [ ] T130 [US6] Run `ginkgo -v --focus "AS-1[23]" ./tests/e2e/frontend/` and `tool_authorizations_test.go` until they pass

**Checkpoint (not a release)**: AS-12 and AS-13 pass. The browser never calls `GET /api/approvals`

---

## User Story 7: Use the Console in Either Theme (Priority: P2)

**Goal**: The user menu offers a theme choice. Every view works in both themes, by keyboard, at 320 px, and at 200% zoom, with the local wordmark (AS-14, AS-15; FR-003–FR-006, FR-022; FE-002–FE-004)

**Independent Test**: Collapse the sidebar, switch themes, navigate all routes by keyboard, and reload under a different OS theme

**Depends on**: T133 audits the pages from US1–US6

### Tests for User Story 7 [MANDATORY - Principle VIII] ⚠️

- [ ] T131 [P] [US7] Write `web/src/components/layout/UserMenu.test.tsx`. It asserts that the menu shows the principal and a ThemeChoice for Light, Dark, and System. Choosing a theme persists `aib.theme` and updates `data-theme`. The menu is keyboard operable

### Implementation for User Story 7

- [ ] T132 [US7] Implement `web/src/components/layout/UserMenu.tsx` as a DropdownMenu that contains ThemeChoice, and mount it in the `userMenu` slot of `web/src/components/layout/ConsoleLayout.tsx`
- [ ] T133 [US7] Audit and fix both shells and every page for AS-15, and record each fix by file in `specs/047-redesign-consent-console/cutover-inventory.md`:
      - Every interactive primitive shows the `:focus-visible` ring token
      - No sticky element obscures focus
      - No page overflows at 320 px or 200% zoom
      - No view has a second accent action
      - Reduced motion removes all movement
- [ ] T134 [US7] Run `ginkgo -v --focus "AS-1[45]" ./tests/e2e/frontend/` until it passes

**Checkpoint (not a release)**: AS-14 and AS-15 pass

---

## User Story 8: Choose Appearance in Settings (Priority: P3)

**Goal**: `/settings` offers only the per-browser theme choice (AS-17; FR-024, FR-030)

**Independent Test**: Change the theme in `/settings`, reload, and open an approval

### Tests for User Story 8 [MANDATORY - Principle VIII] ⚠️

- [ ] T135 [P] [US8] Write `web/src/pages/SettingsPage.test.tsx`. It asserts:
      - The page renders in ConsoleShell with the PageHeader "Settings"
      - An Appearance section shows ThemeChoice with the current preference
      - A change persists `aib.theme` and applies immediately
      - No approval-persistence control or text exists
- [ ] T136 [P] [US8] Add a case to `internal/adapters/http/handlers/spa_test.go` that asserts `/settings` falls back to `index.html` with the CSP from T052

### Implementation for User Story 8

- [ ] T137 [US8] Implement `web/src/pages/SettingsPage.tsx`. Add a lazy `/settings` route under ConsoleLayout in `web/src/App.tsx`, and add a Settings item to the sidebar navigation in `web/src/components/layout/ConsoleLayout.tsx`
- [ ] T138 [US8] Run `ginkgo -v --focus "AS-17" ./tests/e2e/frontend/` until it passes

**Checkpoint (not a release)**: AS-17 passes

---

## User Story 9: Jump to a Record (Priority: P3)

**Goal**: A keyboard command palette over the acting user's agents, connections, and approvals, with theme actions (AS-18; FR-025; SR-001)

**Independent Test**: Open the palette by keyboard, find each record type, navigate to it, and change the theme

**Depends on**: US3 `useDelegations` (T103), US5 `useConnections` (T118), and foundation `usePendingApprovals` (T070)

### Tests for User Story 9 [MANDATORY - Principle VIII] ⚠️

- [ ] T139 [P] [US9] Write `web/src/components/command/CommandPalette.test.tsx`. It asserts:
      - Ctrl+K, ⌘K, and the ConsoleShell search button open the palette. Focus is trapped, and Escape returns it
      - The groups are Agents, Connections, Approvals, and Theme
      - Results come only from the principal-scoped data of `useDelegations`, `useConnections`, and `usePendingApprovals`, with no new request type
      - Choosing an agent opens `/agents/:id`, a connection opens `/sessions`, an approval opens `/approvals/:id`, and a theme item calls `setTheme`
      - Agent-supplied names render escaped

### Implementation for User Story 9

- [ ] T140 [US9] Implement `web/src/components/command/CommandPalette.tsx`, which uses `advanced/Command` inside a Dialog, and `web/src/hooks/useCommandShortcut.ts`. `web/src/components/layout/ConsoleLayout.tsx` lazy-loads the palette on first open and fills the `search` slot. Decision routes never import it
- [ ] T141 [US9] Run `ginkgo -v --focus "AS-18" ./tests/e2e/frontend/` until it passes, then rerun `just web-bundle-check`

**Checkpoint (not a release)**: All 17 active scenarios pass on the branch

---

## Cutover Removal and Release Acceptance

**Purpose**: Remove the old presentation and every obsolete consumer in the same release. Publish the reviewed baselines and documentation (FR-028, FR-031; SC-008, SC-010)

- [ ] T142 Delete the superseded application code. First confirm that a passing replacement test covers each preserved behavior. Delete:
      - Pages: `web/src/pages/AgentGrantDetailPage.tsx` (with `.test.tsx` and `.integration.test.tsx`), `ConsentOverviewPage.tsx` (with its test), `ThirdPartySessionsPage.tsx`, and `ToolAuthorizationsPage.tsx`
      - Layout: `web/src/components/layout/AppLayout.tsx` and `Header.tsx` (with `Header.test.tsx`)
      - Everything in `web/src/components/ui/`
      - Superseded consent components in `web/src/components/consent/`, together with their tests: `DelegationCard`, `DelegationList`, `ServiceCard`, `ScopeList`, `GrantStatusBadge`, `GrantValidityControl`, `RevokeGrantButton`, `RevokeGrantDialog`, `PermissionSetCard`, `PermissionSetsList`, `ServiceRequirementCard`, `ServiceRequirementsList`, `CIMDLocalhostWarning`, `CIMDConsentSummary`, `CIMDSection`, and `CIMDAdvancedDetails`
      - Sessions: `web/src/components/sessions/SessionCard.tsx`, `TerminationDialog.tsx`, and `index.ts`
      - Hooks, together with their tests: `useConsent`, `useAgentGrants`, `useSessions`, `useToggleGrant`, `useUpdateValidity`, and `useApproval`

      Then update `web/src/hooks/index.ts` and unmount `ToastProvider`
- [ ] T143 [P] Delete the design-system components that T014(b) marks as replaced: `overlays/Modal`, `overlays/Dropdown`, `inputs/TextInput`, `inputs/Radio`, `primitives/Divider`, `layout/AppLayout`, `layout/PageTransition`, and `feedback/Toast`. Also delete any of these that have no consumer, and record the `rg` proof in `cutover-inventory.md`: `primitives/Spinner`, `layout/{Container,Stack,Grid}`, `navigation/{Breadcrumb,Pagination}`, `advanced/Progress`, and `data-display/{StatusIndicator,ScopeList}`. Update the category `index.ts` barrels in `web/src/design-system/components/`
- [ ] T144 [P] Remove `@headlessui/react` and `framer-motion` from `web/package.json` and `web/package-lock.json`
- [ ] T145 [P] Delete `web/src/services/api/cache.ts` and its re-exports from `web/src/services/api/index.ts`
- [ ] T146 [P] Delete these token and configuration sources:
      - `web/tailwind.config.ts`, and its entry in `web/tsconfig.node.json`
      - `web/src/design-system/tokens/colors.css`
      - Any TypeScript token module that has no runtime consumer left: `colors.ts`, `shadows.ts`, `animation.ts`, `typography.ts`, `spacing.ts`, or `radius.ts`. Verify each one with `rg`

      Update `web/src/design-system/tokens/index.ts`. Update or delete `web/src/design-system/oauth2-semantic-tokens.md` so that no document describes removed tokens
- [ ] T147 [P] Delete the Crimson Pro and Manrope WOFF2 files, `LICENSE-crimson-pro.txt`, and `LICENSE-manrope.txt` from `web/public/fonts/`. Also delete any JetBrains Mono subset that `web/src/styles/fonts.css` no longer references
- [ ] T148 Verify the removal, and record the results in `specs/047-redesign-consent-console/cutover-inventory.md`:
      - Every T002 command returns zero hits
      - `just web-lint` reports zero raw-palette errors
      - `npm --prefix web ls @headlessui/react framer-motion` finds neither package
      - `rg -n "ui\.v2|feature[_-]?flag" web internal charts examples` finds nothing
      - No `linear-gradient`, page-entry animation, or `PageTransition` reference remains
- [ ] T149 Capture, review, and commit the baselines:
      - Run `E2E_CAPTURE_SCREENSHOTS=true GINKGO_FRONTEND_PROCS=1 just test-e2e-frontend`
      - A human reviewer approves each image
      - Commit the 14 route, context, and theme images, plus meaningful loading, empty, error, and overlay states, to `tests/e2e/screenshots/`, and list the state stems in `tests/e2e/screenshots/visual-gate.txt`
      - Replace every retained journey screenshot with its reviewed new-UI capture
      - Remove old-UI baselines that no test generates anymore

      Then `just test-e2e-frontend-visual` must pass
- [ ] T150 [P] Add a "Consent UI" section to `README.md` (FR-031). Write one short paragraph for each route: `/delegations`, `/agents/:id` in both the decision and console contexts, `/sessions`, `/approvals`, `/approvals/:id`, and `/settings`. Show each route's light and dark screenshots from `tests/e2e/screenshots/` in `<picture>` elements with `prefers-color-scheme` sources. Replace the old-UI teaser `assets/docusaurus/static/img/teaser-browser.webp` (referenced near line 87 of `README.md`) with a current capture
- [ ] T151 [P] Update `docs/concepts/delegation-and-consent.md` and `docs/get-started/index.md` wherever they describe the old "Consent Management" UI, the shield, or the old routes. Then run `just docs-build`

**Checkpoint**: One presentation remains, with no obsolete code, dependencies, fonts, caches, or flags. Baselines and README are published

---

## 🔒 Phase N: Constitution Compliance & Polish [MANDATORY COMPLIANCE SECTION]

**Purpose**: Verify constitution requirements and final polish

**🔒 CONSTITUTION REQUIREMENT**: This entire section MUST be included in every tasks.md file. The compliance tasks map directly to the constitution Implementation Phase checklist and MUST be completed before considering the feature done.

### 🔒 Constitution Compliance Verification [MANDATORY]

#### Design Phase Verification [MANDATORY]

**Constitution Reference**: PRECONDITIONS checklist. Verify that the Phase 2 tasks were completed correctly

- [ ] T152 Verify that the Glossary (§12) in `ARCHITECTURE.md` contains Consent Draft, Agent Origin Label, Delta Re-consent, Appearance Preferences, Delegation, and the feature-047 Connection State entry (Principle V)
- [ ] T153 Verify that no configuration example is needed: `examples/config/` is unchanged, and `cutover-inventory.md` records T008 (Principle VII)
- [ ] T154 Verify that `examples/config/README.md` needs no new reference, because no configuration section was added (Principle VII)
- [ ] T155 Verify that `git diff main -- api/enduser/openapi.yaml` contains only the 2026-09-27 documentation correction of the `GET /api/third-party/sessions` response, that `git diff main -- api/admin/openapi.yaml` is empty, and that T010's endpoint map is attached to the PR (Principles IV, X)
- [ ] T156 Verify that the PR description references the stakeholder confirmation recorded in `specs/047-redesign-consent-console/cutover-inventory.md` (T011) and the ADR 037 acceptance in `adrs/037-design-system-rebuilt-on-shadcn-radix.md` (T013) (Principle X)
- [ ] T157 Verify that `git diff main -- migrations/ internal/ports/storage.go` is empty (Principle IX)
- [ ] T158 [IF FRONTEND] Verify that the T014 review is recorded and that the universal components (Wordmark, TruncatedText, ThemeChoice, ConsoleShell, DecisionShell, and PageHeader) live in `web/src/design-system/` (Principle XI)
- [ ] T159 Verify that `tests/e2e/frontend/consent_ui_v2_test.go` has one `It` for each of AS-01–AS-15, AS-17, and AS-18, and none for AS-16 (Principle XIII)
- [ ] T160 Verify the T042 red-phase record in `cutover-inventory.md`: detailed expectations were written and failed, with no placeholder always-fail assertions, no `XIt`/`PIt`/`Skip()` markers, and no "red phase" comments in `tests/e2e/frontend/` (Principle XIII)
- [ ] T161 [IF FRONTEND] Verify that the page objects in `tests/e2e/pages/` cover every new interaction and that the test files in `tests/e2e/frontend/` use no raw selectors (Principle XIII)
- [ ] T162 [IF FRONTEND] Verify that `tests/e2e/screenshots/` contains the 14 route, context, and theme images, using the stems from quickstart.md (Principle XIII)

#### Implementation Phase Verification [MANDATORY]

**Constitution Reference**: Implementation Phase checklist. Verify that all principles were followed during implementation

**API & Documentation** (Principles IV, X):
- [ ] T163 [P] Verify that `api/enduser/openapi.yaml` needed no update beyond the session-list documentation correction, because every implemented call in the T010 map exists unchanged
- [ ] T164 [P] Verify that the request and response types in `web/src/services/api/*.ts` and `web/src/types/*.ts` match `api/enduser/openapi.yaml` exactly
- [ ] T165 Verify that `docs/api/` needs no change, because no API changed, and that `docs/reference/api.md` describes the corrected session list. End-user UI documentation lives in `README.md` and `docs/` (T150, T151)

**Architecture & Documentation** (Principle II):
- [ ] T166 Update the frontend section of `ARCHITECTURE.md` (about lines 135–160) to the delivered stack. Remove the "proposed" framing and the pre-migration inventory (Headless UI, React Router 6, and "no external state library")
- [ ] T167 Verify that the Glossary in `ARCHITECTURE.md` matches the terms used in code and in `web/src/copy/index.ts`
- [ ] T168 [P] Verify that `adrs/037-design-system-rebuilt-on-shadcn-radix.md` is Accepted and cross-referenced from ADRs 006 and 035. Write a further ADR only if the implementation deviated from ADR 037

**Configuration** (Principle VII):
- [ ] T169 [P] Verify that no configuration path was added: `internal/ports/config.go` has no new keys, and nothing adds an environment variable or CLI option. The SPA handler reads only the build artifact `csp-hashes.json`
- [ ] T170 [IF CONFIG CHANGED] Not applicable: verify that `git diff main -- charts/` is empty and that T009 is recorded (Principle VII)

**Database & Persistence** (Principle IX):
- [ ] T171 [P] Not applicable: verify that `migrations/` has no new files
- [ ] T172 [P] Not applicable: verify that `just test-integration` passes with `tests/integration/` unchanged, which covers migrations
- [ ] T173 [P] Not applicable: verify that no PostgreSQL repository in `internal/adapters/storage/postgres/` changed
- [ ] T174 Verify that persistence stays within its boundary. The browser stores only `aib.theme` and `aib.sidebar-collapsed` in `localStorage`, and never persists API data (`web/src/services/query/QueryProvider.tsx`)

**Security** (Principles I, III):
- [ ] T175 Verify that security features are on by default, with no bypass:
      - The CSP is always on and fails closed (`internal/adapters/http/handlers/spa.go`)
      - No flag exists
      - `session_token` validation, `isSafeRedirectUrl`, and callback handling are unchanged
      - Nothing grants or approves optimistically, and Deny does not mutate
      - The cross-principal assertions in AS-09, AS-13, and AS-18 pass
- [ ] T176 [P] Verify that no custom cryptography exists. The CSP hash uses Node `crypto.createHash('sha256')` in `web/build/themeInitPlugin.ts`, and Go only validates the hash string
- [ ] T177 [P] Verify structured logging. `SPAHandler` logs a missing or invalid `csp-hashes.json` at error level with its path. The existing audit logging for grant, approval, and session mutations is unchanged

**Architecture Patterns** (Principle VI):
- [ ] T178 Verify that `git diff main --stat -- internal/ cmd/` touches only `internal/adapters/http/handlers/spa.go` and `spa_test.go`, with no domain, port, or builder change

**Testing** (Principle VIII - Unit & Integration Tests):
- [ ] T179 Verify that the unit tests in `web/src/`, `web/build/`, `web/eslint-rules/`, and `internal/adapters/http/handlers/spa_test.go` were written first and failed before implementation. Check each test task (T045, T047, T049, T051, T053, T056, T067, T071, T073, T075, T077, T079, T081, and every story test task) and each test-first substep (T040, T058–T064, and T068) against its implementation commit
- [ ] T180 Verify that the tests drove the design, so that the implementation in `web/src/` and `internal/adapters/http/handlers/spa.go` emerged from their requirements
- [ ] T181 Verify that the tests in `web/src/**/*.test.ts(x)` and `internal/adapters/http/handlers/spa_test.go` changed minimally during implementation
- [ ] T182 Verify with the `justfile` recipes that `just web-test`, `just web-storybook-test`, `just web-bundle-check`, and `just test` pass
- [ ] T183 Verify that no Bash script validates code correctness. The visual gate lives in Go (`tools/imgdiff`), the bundle check in Vitest (`web/build/decisionBundle.test.ts`), and the palette check in ESLint (`web/eslint-rules/no-raw-palette.js`)

**E2E Acceptance Testing** (Principle XIII):
- [ ] T184 Verify that `tests/e2e/frontend/consent_ui_v2_test.go` has tests for all 17 active acceptance scenarios from spec.md
- [ ] T185 Verify that each `It()` block in `tests/e2e/frontend/consent_ui_v2_test.go` maps to exactly one acceptance scenario, with theme parameterization inside the scenario
- [ ] T186 Verify that the E2E tests in `tests/e2e/frontend/consent_ui_v2_test.go` were written before implementation and failed initially (T042)
- [ ] T187 Verify that the E2E tests changed minimally during implementation, with only adjustments in `tests/e2e/fixtures/` and `tests/e2e/pages/`
- [ ] T188 Verify that the E2E tests in `tests/e2e/frontend/consent_ui_v2_test.go` turned green as the implementation satisfied the acceptance criteria
- [ ] T189 Verify that the E2E tests use Ginkgo/Gomega and follow the patterns in `tests/e2e/README.md`
- [ ] T190 Verify that `tests/e2e/frontend/consent_ui_v2_test.go` uses a Describe → Context → It hierarchy
- [ ] T191 Verify that `tests/e2e/frontend/consent_ui_v2_test.go` includes comments that reference spec scenarios
- [ ] T192 Run the full E2E suite in `tests/e2e/` with `just test-e2e` (backend, ExtProc, and frontend). All tests must pass
- [ ] T193 [IF FRONTEND] Verify that the preserved Playwright journeys in `tests/e2e/frontend/` pass with selector-only changes (SC-009)
- [ ] T194 [IF FRONTEND] Verify that the screenshots in `tests/e2e/screenshots/` have descriptive file names
- [ ] T195 [IF FRONTEND] Run the frontend E2E suite with `ginkgo -v ./tests/e2e/frontend/`, then run `just test-e2e-frontend-visual`. Both must pass

**Frontend** (Principle XI - if applicable):
- [ ] T196 [IF FRONTEND] Verify that the components in `web/src/components/` and `web/src/pages/` use design-system primitives and semantic tokens
- [ ] T197 [IF FRONTEND] Verify that the universal components are in `web/src/design-system/components/` with Storybook stories and reviewed visual-regression baselines in `web/.storybook/__screenshots__/`
- [ ] T198 [IF FRONTEND] Verify that no custom CSS bypasses the design tokens: `just web-lint` passes, and `web/src/styles/` contains only fonts and token mapping
- [ ] T199 [IF FRONTEND] Verify WCAG 2.1 AA compliance (4.5:1 text contrast, 3:1 UI contrast) and the feature's WCAG 2.2 AA target, using `web/src/design-system/tokens/contrast.test.ts` and the Storybook a11y results
- [ ] T200 [IF FRONTEND] Verify that every component story in `web/src/design-system/components/` renders, passes the accessibility addon, and matches its reviewed screenshot baseline in light and dark themes (`just web-storybook-test`)
- [ ] T201 [IF FRONTEND] Verify that all visual decisions use the centralized semantic tokens in `web/src/design-system/tokens/theme.css`, without raw palette utilities
- [ ] T202 [IF FRONTEND] Verify that brand assets are self-hosted in `web/public/brand/` and `web/public/fonts/`, and that the frontend loads no third-party fonts, scripts, or images (SC-006, the AS-15 request recorder, and CSP `font-src 'self'`)
- [ ] T203 [IF FRONTEND] Verify that the visual direction matches `web/src/design-system/docs/DESIGN_PRINCIPLES.md` and ADR 037, and that no current guide describes two active visual systems (SC-010)

### Additional Polish [CUSTOMIZABLE]

- [ ] T204 Measure SC-002 on a production-served build (`just build-all`, then run the broker). Consent main content on `/agents/:id?session_token=…` must appear within 1.5 s under the Chrome "Slow 4G" preset. Record the measurement and the `just web-bundle-check` size in the "Performance" section of `cutover-inventory.md`
- [ ] T205 [P] Run a runtime smoke test on the production-served build. Visit all six routes and both agent contexts in both themes, and observe console errors, CSP violations, cross-origin requests, the provider callback, preference persistence, and reduced motion. Record the results in `cutover-inventory.md`
- [ ] T206 [P] Run moderated consent and tool-review sessions for SC-001 and SC-007: decision time, service comprehension, and primary-action recognition. Record the results in `cutover-inventory.md`, or list them explicitly as unavailable in the PR
- [ ] T207 Run every step in `specs/047-redesign-consent-console/quickstart.md` and attach the evidence it lists
- [ ] T208 Run `just verify` (defined in `justfile`) as the final merge gate

---

## Dependencies & Execution Order

### Section Dependencies

- **Setup (T001–T004)**: No dependencies. Setup does not change runtime behavior or current guidance
- **Phase 2 (T005–T042)**: T005–T012 only record and confirm, so they may run before the gate. **T013 (ADR 037 acceptance) blocks every later task.** T015–T021 update current guidance after acceptance. T022–T031 build the E2E harness, T032–T039 write the red scenarios in one file (sequentially), and T040–T041 add the visual gate. T042 proves the red phase
- **Foundational (T043–T082)**: Depends on all of Phase 2. It blocks every user story. Internal order:
  - T043 comes before everything else in this section
  - T058–T064 and T068 write their tests first inside the task and confirm the failure before implementing. T040 does the same in Phase 2
  - Each test task comes before its implementation: T045→T046, T047→T048, T049→T050, T051→T052, T053→T054, T056→T057, T067→T068–T070, T071→T072, T073→T074, T075→T076, T077→T078, T079→T080, and T081→T082
  - T054 needs T016. T058–T063 need T054. T064 needs T048 and T059. T065 and T066 need T058–T064. T078 needs T058 and T060. T080 needs T048, T062, T069, T070, and T078
- **User stories (T083–T141)**: All depend on Foundational
  - US1 and US2 (P1) are independent of each other
  - US3, US5, and US6 (P2) are independent of each other and of US1 and US2
  - US4 depends on the US1 components T087–T089
  - US7's T133 audit depends on US1–US6
  - US8 depends only on Foundational
  - US9 depends on US3 (T103) and US5 (T118)
- **Cutover Removal (T142–T151)**: Depends on all user stories. T142 must finish before T143–T147. T148 depends on T142–T147. T149 depends on T148, and T150 depends on T149
- **Phase N (T152–T208)**: Depends on Cutover Removal. T208 is the last task

### Within Each User Story

- Tests are written first and must fail before the implementation
- Hooks come before components, and components before the page
- Page-object selector updates come after the page, followed by the green E2E run
- Update only selectors in page objects. Existing test assertions stay unchanged (FR-029)

### Scenario Traceability

| Scenario | Red test | Story implementation | Green run |
| --- | --- | --- | --- |
| AS-01–AS-03 | T032 | T083–T091 | T092 |
| AS-04, AS-05 | T033 | T093–T099 | T100 |
| AS-06, AS-07 | T034 | T101–T106 | T107 |
| AS-08, AS-09 | T035 | T108–T114 | T115 |
| AS-10, AS-11 | T036 | T116–T121 | T122 |
| AS-12, AS-13 | T037 | T123–T129 | T130 |
| AS-14, AS-15 | T038 | T131–T133 | T134 |
| AS-17 | T039 | T135–T137 | T138 |
| AS-18 | T039 | T139–T140 | T141 |

### Parallel Opportunities

- Setup: T002, T003, and T004
- Phase 2: T006 and T009; the guidance updates T015–T021 after T013; the page objects and fixtures T022–T030
- Foundational: the test tasks T045, T047, T049, T051, T053, and T056; the owned-component groups T058–T063; the shared-model tests T067, T071, T073, and T075
- After Foundational, US1, US2, US3, US5, US6, and US8 can proceed in parallel on separate worktrees of the same branch
- Cutover removal: T143–T147 after T142, and T150 and T151 after T149

---

## Parallel Example: Foundational Components

```bash
# After T054 (tokens) lands, build the owned component groups together:
Task: "T058 primitives/{Button,Badge,Separator,Avatar,Wordmark} in web/src/design-system/components/primitives/"
Task: "T059 inputs/{Input,Select,Switch,Checkbox,RadioGroup,TextArea,DatePicker} in web/src/design-system/components/inputs/"
Task: "T060 overlays/{Dialog,DropdownMenu,Popover,Tooltip,Sheet} in web/src/design-system/components/overlays/"
Task: "T061 Tabs, Card, Table, TruncatedText in navigation/ and data-display/"
Task: "T062 feedback/{Skeleton,Toaster,Alert,EmptyState,InlineError,ErrorBoundary,GlobalErrorBoundary}"
Task: "T063 advanced/{Accordion,Command}"
```

## Parallel Example: User Story 1

```bash
# Tests first, together:
Task: "T083 [US1] web/src/components/consent/origin.test.ts"
Task: "T084 [US1] web/src/pages/AgentDecisionPage.test.tsx"

# After T085–T086, the components together:
Task: "T087 [US1] AgentIdentityHeader, LocalhostBanner, CIMDDetails"
Task: "T088 [US1] PermissionGroupList, PermissionGroupItem"
Task: "T089 [US1] DurationChoice, ConsentActions, ServiceConnectPrompt"
```

---

## Implementation Strategy

### Single Cutover (No MVP Release)

FR-028 and SC-010 rule out a partial release. The minimum shippable scope is all 17 active scenarios, with one presentation and the old code removed. Checkpoints in this file are review points on the feature branch. They are not deployments.

1. Complete Setup and record the baseline
2. Complete Phase 2: the confirmations, then **the ADR 037 acceptance gate (T013)**, then the guidance updates and the red E2E scenarios
3. Complete Foundational (design system, themes, CSP, data layer, and shells)
4. **First review point**: US1 and US2, the P1 decision flows. Validate AS-01–AS-05 on the branch
5. Complete US3–US6 (P2), then US7's cross-cutting audit
6. Complete US8 and US9 (P3)
7. Complete Cutover Removal, then Phase N. Merge and release once, only after T208 passes

### Parallel Team Strategy

1. One agent drives T013 with the stakeholder, while others prepare T001–T012
2. After acceptance, split up T015–T021 (guidance) and T022–T030 (E2E harness). One agent writes T031–T039 sequentially
3. Split Foundational by the parallel groups above. One owner handles `web/vitest.config.ts`, `web/vite.config.ts`, and `justfile` edits to avoid conflicts
4. Assign US1/US4, US2, US3/US9, US5, US6, and US8 to separate agents, then audit with US7
5. One agent performs Cutover Removal and Phase N

---

## Notes

- [P] tasks touch different files and have no dependency on incomplete tasks
- [Story] labels map tasks to spec.md user stories for traceability
- Commit after each task or logical group. Keep the branch unreleased until T208
- Never accept visual baselines automatically, and never replay a security mutation automatically
- Avoid vague tasks, same-file conflicts, and cross-story dependencies other than the ones declared above
