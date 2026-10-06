# Consent UI v2 Cutover Inventory

## Execution status

The first T001 attempt stopped on 2026-09-27 because frontend dependency installation failed.
The dependency mismatch is resolved. The unit baseline, production build, and browser baseline pass.
The redesign, legacy-journey migration, and approved upstream callback integration are implemented. Automated checks and production-browser evidence are recorded below. Later entries record superseding stakeholder decisions. These dated results do not define a separate release-approval process.

## Superseding reference policy — 2026-10-06

The user's current request authorizes [accepted ADR 040](../../adrs/040-canonical-light-visual-reference-gate.md). Required pixel references use light mode at 1280 × 720. Eight route/context files use `_light.png`. Storybook captures and compares PNGs only in desktop `storybook-light`, with an effective light theme and the actual canonical viewport.

All six theme/viewport projects keep interaction and accessibility checks. Intentional dark or narrow overrides receive no pixel reference. Dated both-theme and six-way reference requirements and approvals below remain history, not the active matrix. This decision changes no design, authorization, API, threshold, product motion, or accepted 190-kB budget.

The retained reviewed set contains 376 canonical Storybook references and eight required light route references. This count describes reference inputs, not a new test-run result.

### Published canonical reference verification

The stakeholder approved the refreshed canonical set for publication on 2026-10-06. The repository contains 376 Storybook PNGs and eight route PNGs, all at 1280 × 720 in light mode.

- The Linux Chromium canonical Storybook comparison passed all 403 stories without updating references.
- All eight required route comparisons passed at 0.0000% difference.
- `just check`, `just web-lint`, and all 817 frontend unit tests passed.
- The comparator, stable-fixture, and isolated fixed-viewport capture regressions passed.
- Dark/responsive interaction and accessibility coverage remains enabled. The final cold-overlay regression passed all 64 targeted cases across the four noncanonical viewports.

These are local verification results. No Git commit, push, or GitHub CI rerun was performed.


## Baseline

These commands ran before source or dependency changes:

| Command | Result | Passed tests | Failed tests | Limitation |
| --- | --- | --- | --- | --- |
| `just check` | Passed | Not applicable | Not applicable | `golangci-lint` was unavailable. The recipe used its `go vet ./...` fallback |
| `just web-test` | Exit 1 during dependency installation | 0 executed | 0 executed | No test results. npm returned `ERESOLVE` |
| `just test-e2e-frontend` | Exit 1 during dependency installation | 0 executed | 0 executed | No browser suite or production build results. npm returned `ERESOLVE` |

The initial attempts executed no tests. The resumed results below complete T001 without a change to application source.

### Existing dependency conflict

The original manifest declared `@eslint/js` as `^10.0.1` and `eslint` as `^9.0.0`.
Both failed commands reported the same conflict:

```text
While resolving: @eslint/js@10.0.1
Found: eslint@9.39.5
Could not resolve dependency:
peerOptional eslint@"^10.0.0" from @eslint/js@10.0.1
error: recipe `web-ensure-deps` failed with exit code 1
```

The npm reports are:

- `/Users/brennenstuhl/.npm/_logs/2026-09-27T17_58_34_669Z-eresolve-report.txt` for `just web-test`
- `/Users/brennenstuhl/.npm/_logs/2026-09-27T17_58_34_766Z-eresolve-report.txt` for `just test-e2e-frontend`.

No command used `--force` or `--legacy-peer-deps`.

### Dependency repair and resumed baseline

The manifest now declares `@eslint/js` as `^9.39.5`, consistent with the existing ESLint 9 toolchain.
`just web-install` updated `web/package-lock.json` and installed 534 packages successfully.
`npm --prefix web ls @eslint/js eslint` reports version `9.39.5` for both packages, without a peer conflict.
`npm --prefix web ls --depth=0` also succeeds.

A smoke run used the actual repository ESLint configuration through standard input.
Valid exported TypeScript returned exit 0. An unused variable returned exit 1 with `@typescript-eslint/no-unused-vars`.
The smoke run created no source file.

| Command | Result | Passed tests | Failed tests |
| --- | --- | --- | --- |
| `just web-test` | Passed: 28 test files | 246 | 0 |
| `just test-e2e-frontend` after dependency repair | Production build passed. The browser runner was unavailable: `ginkgo: command not found` | 0 executed | 0 executed |

Ginkgo `v2.32.2`, as pinned in `go.mod` and `justfile`, is installed in `/Users/brennenstuhl/go/bin`.
The resumed browser command adds that directory to its process-local `PATH`. It changes no shell configuration.

The first Ginkgo run found that the Playwright `v1.62.1` driver was missing.
Both workers failed in `SynchronizedBeforeSuite`, so none of the 61 scenarios executed.
`go run github.com/mxschmitt/playwright-go/cmd/playwright@v0.6201.1 install chromium` installed the pinned driver and Chromium.
The next run succeeded:

```text
env PATH="/Users/brennenstuhl/go/bin:$PATH" just test-e2e-frontend

Ran 61 of 61 Specs in 35.636 seconds
SUCCESS! -- 61 Passed | 0 Failed | 0 Pending | 0 Skipped
Test Suite Passed
```

T001 is complete. The baseline contains 246 passing unit tests and 61 passing browser scenarios, with no behavioral failures.
The dependency repair changed only the ESLint configuration package and its generated lockfile. Application source remains unchanged.

## Stakeholder decision

The stakeholder selected **Accept ADR 037** (now ADR 038) in this session's `/speckit-implement` approval prompt on 2026-09-27.
The prompt requested permission to record the conversation as the decision reference and proceed with the complete single-cutover implementation.

T013 is complete: ADR 038 records acceptance, ADRs 006 and 035 reference it, and the root ADR index includes it.
Acceptance does not establish runtime completion. The remaining tasks stay unchecked.

## Checklist gate

`checklists/requirements.md` contains 16 checked items and no unchecked items. The gate passed. No checklist marker changed.

## Contract discrepancy: delegation permission-set count

T005 and T010 found a conflict between the feature assumptions and the current runtime.
`internal/domain/consent/service.go:813` increments `ActiveGrantCount` once per `UserGrant`.
It does not count `GrantedPermissionSets`.

A temporary Go program used the real memory repositories, `fixtures.ValidAgent()`, and `consent.NewService`.
It stored one user grant with three permission sets and called `GetAgentDelegations`.
The observed result was:

```text
Stored permission sets: 3
activeGrantCount: 1
```

The script was removed after the smoke run. No domain, handler, API, or storage behavior changed.

The initial plan labeled this field as a permission-set count. That would show incorrect information.
The stakeholder selected **Remove the count column** on 2026-09-27.
AS-06, FR-014, the data model, plan, and affected tasks now require agent identity, expiry, View, and confirmed Revoke only.
The feature keeps its prohibition on backend response changes and per-row count requests. This discrepancy is resolved.

## Performance baseline

The production build ran through the `web-build` prerequisite of `just test-e2e-frontend`.
The installed TypeScript parser identified static imports in the entry and `AgentGrantDetailPage` chunks. Dynamic imports are excluded.
Sizes use Node `zlib.gzipSync` at its default compression level. Vite's displayed gzip sizes use a different compression setting.

All paths in this table are relative to `web/dist/assets/`.

| Asset | Bytes | Gzip bytes | Static imports |
| --- | ---: | ---: | --- |
| `index-DhajffCI.js` | 396317 | 127190 | None |
| `AgentGrantDetailPage-CYHdeWtt.js` | 58286 | 15320 | index, AppLayout, PageTransition, transition, InlineError, Button, api, Badge, RevokeGrantDialog, Accordion |
| `AppLayout-BSgI9RU4.js` | 90216 | 30909 | index |
| `PageTransition-DD4ao5Y8.js` | 330 | 229 | index |
| `transition-CXKkFxph.js` | 35683 | 12980 | index |
| `InlineError-DggLBVo1.js` | 1510 | 653 | index |
| `Button-3t0YYo1i.js` | 6530 | 2217 | index, AppLayout |
| `api-C3EQpADV.js` | 5229 | 1672 | index, AppLayout |
| `Badge-hHMl0j8p.js` | 1387 | 645 | index, AppLayout |
| `RevokeGrantDialog-BQZpj8OL.js` | 34807 | 11395 | index, AppLayout, transition, Button |
| `Accordion-C2Z1Z2al.js` | 19514 | 6929 | index, AppLayout, transition |
| `index-BKcYjOQM.css` | 84361 | 14124 | None |

The combined entry, consent static-import graph, and CSS total **224263 gzip bytes**.
This is a pre-redesign measurement, not SC-002 acceptance.

## Selector changes

This is the pre-cutover inventory of selectors in `tests/e2e/pages/consent_page.go`, `approval_page.go`, `tool_authorizations_page.go`, and `sessions_page.go`. The 2026-09-28 clarification below supersedes the initial selector-only restriction where an old presentation conflicts with the approved redesign. Security, authorization, storage, and callback assertions remain required.

Confidence key:
- **Pinned** — planned label/role comes verbatim from the copy catalogue (T044) or a task body (T026–T029, the scenario requirements, T084, T094, T102, T124).
- **Derived** — implied by a pinned FR/AS or component contract; exact string not pinned, verify at implementation.
- **TBD** — no pinned planned copy exists; the selector must be re-derived from the new component when its story lands.

Planned-string sources (verbatim catalogue, tasks.md T044): "Agentic Identity Broker", "Verified domain: {host}", "Registered by your administrator", "Unverified", "Risk not rated", "Until revoked", "30 days", "Custom date", "Allow", "Deny", "Approve once", "Approve and remember", "Revoke all access", "Save changes", "Cancel", "Connected", "Needs re-authentication", "Expired", "No connection", "Connect", "Reconnect", "Refresh", "Disconnect", "Light", "Dark", "System", "Show more", "Show less", "Powered by Zalando".

---

### 1. tests/e2e/pages/consent_page.go

#### 1a. Decision view (T091) — `/agents/:id?session_token=…` → AgentDecisionPage in DecisionShell

| Method | Line | Current selector / label | Planned change |
|---|---|---|---|
| `getConsentActionButton` | 154–159 | `GetByRole("button", {Name: label})` — generic helper | Role unchanged; consumed with new labels below. |
| `SubmitConsent` | 247 | button "Approve & Delegate" | **Pinned**: button "Allow", the view's single `data-variant="primary"` accent action (T044, T089 ConsentActions, T084). |
| `IsConsentButtonEnabled` | 518 | label "Approve & Delegate" | **Pinned**: label "Allow". |
| *(new)* `ClickDeny` / `DecisionOutcomeText` | — | no Deny selector today | **Pinned**: T026 adds `ClickDeny`, `DecisionOutcomeText`, `DecisionErrorText`; "Deny" is a secondary non-destructive action (T044, the scenario requirements). |
| `EnableSpecificEndDate` | 307–311 | `GetByRole("checkbox", {Name: "Specific end date"})` | **Pinned**: replaced by DurationChoice RadioGroup; option "Custom date" (T044, T089). `ChooseDuration(label)` covers "Until revoked" / "30 days" / "Custom date" (T026). |
| `endDateInput` (helper) | 254–257 | `GetByLabel("End date", exact)` | **Pinned** component: DatePicker (native `input type="date"`, min date, inline validation, T059, T089). New accessor `CustomDateValue` / `SetCustomDate` (T026). Locator label TBD. |
| `SetExpiration` / `SetExpirationDate` / `GetExpirationDate` | 285, 329, 642 | fill/read the "End date" input | **Derived**: rewritten on DurationChoice + DatePicker; validation "custom date not after today" now inline (T071 `resolveValidUntil`). |
| `GetSuccessMessage` | 543–545 | `GetByRole("status")` | **Derived**: success feedback moves to Sonner Toaster (T062, the scenario requirements). Sonner toasts announce politely — re-verify the accessible role at T091; a role-agnostic `[data-sonner-toast]` selector must be checked against the rendered component. |
| `WaitForGrantSuccess` | 1480–1482 | `GetByRole("status")` filtered `HasText: "Grant updated successfully"` | **TBD (text)**: Toaster replaces Toast (T062); the grant-save success toast copy is not in the T044 catalogue — re-derive from `@copy` when AgentDecisionPage lands. Toast region is `role="status"` per T062 "polite announcements". |
| `WaitForGrantError` | 1505 | `GetByRole("alert")` | **TBD**: error toast also comes from Sonner now; Sonner does not emit `role="alert"` toasts by default — expect re-derivation (likely `[data-sonner-toast]` or status-region filter). Copy not pinned. |
| `waitForAgentNameHeading` | 183 | `GetByTestId("agent-name-heading")` | **TBD**: name moves to AgentIdentityHeader (Avatar + TruncatedText, T087); the `agent-name-heading` test ID is not guaranteed to survive — verify when the component story lands. |
| `GetAgentName` | 362–364 | first `h1` heading | **Derived**: kept (page identity heading), rendered through TruncatedText with an accessible expand control ("Show more"/"Show less", T044). |
| `GetAvailableScopes` | 409 | `[data-testid^='scope-indicator-']` | **Pinned obsolete**: FR-008 forbids raw scope strings on permission groups and StatusIndicator is deleted (T143); scope indicators disappear in the decision view. Method must be rewritten or its call sites migrated. |
| `GetSelectedScopes` | 470–501 | all `GetByRole("checkbox")` + `aria-label`/`label[for]` fallback | **TBD**: selection is now the PermissionGroupItem optional toggle (owned Switch/Checkbox on Radix, T088, T059); accessible-name scheme re-derived. |
| `TogglePermissionSet` | 731–733 | `GetByRole("switch", {Name: "Toggle " + permissionSetName})` | **TBD**: PermissionGroupItem replaces PermissionSetCard; the "Toggle …" aria-label scheme is not pinned. Switch stays a Radix switch (T059) — verify the accessible name. |
| `ServiceScopeRequiredIndicatorCount` | 1241, 1252 | `GetByTestId(psTestID)` card + `GetByLabel("(required)")` chips | **TBD**: PermissionSetCard is deleted (T143); locked required services become the locked control inside PermissionGroupItem (T088). The `"<Service> (required)"` chip aria-label and ps test IDs are not pinned. |
| `DelegateService` | 680–716 | h3 heading `Name: serviceDisplayName, Level: 3` + `article[aria-label="Service: <name>"]` + `[data-testid='service-login-button']` / `[data-testid='service-delegate-button']` | **Pinned**: ServiceCard/ServiceRequirementsList deleted (T143); connection of a required service happens through ServiceConnectPrompt (T089) — **Pinned** action label "Connect" (T044, T073). Headings and article aria-labels disappear. |
| `RevokeService` | 755–782 | h3 service heading + `article[aria-label="Service: <name>"]` + button "Revoke" | **TBD**: no per-service Revoke exists in the redesign (revocation is grant-level, SR-003); call sites migrate to grant revoke or the method is retired. Verify against `revoke_grant_flow_test.go` detail scenarios (T114). |
| `IsMandatoryServiceConnected` | 978–1003 | h3 heading + article + button "Revoke" presence | **Derived**: replaced by the connection state of the required service — "No connection" with "Connect" (consent service prompt, T073) or its Connections-tab row label (the scenario requirements). |
| `GetServiceCount` / `GetServiceNames` | 812–815, 846–849 | all h3 headings | **Derived**: services become names "one expansion away" inside PermissionGroupItem (T088); T026 `GroupServices` replaces these. h3 service headings disappear. |
| `GetValidationErrorCount` / `GetValidationErrors` | 887–900, 925–938 | `GetByRole("alert")` + `GetByRole("listitem")` | **Derived**: existing validation preserved (FR-009) on InlineError/Alert (T062); `role="alert"` semantics expected to survive — verify list markup. |
| `GetErrorMessage` / `HasError` | 583, 620 | `GetByRole("alert")` | **Derived**: unchanged in intent (Alert rebuilt on semantic tokens, T062); re-verify no duplicate alerts now that LocalhostBanner is also an alert. |
| `GetExpirationDate` doc example | 639–641 | (text) | Reads the same "End date"-derived value through the new DatePicker accessor. |

#### 1b. CIMD identity (T091) — CIMD components replaced by AgentIdentityHeader / LocalhostBanner / CIMDDetails (T085–T087, T143)

| Method | Line | Current selector / label | Planned change |
|---|---|---|---|
| `IsCIMDSummaryVisible` | 1323–1325 | `GetByText("wants to access")` | **TBD**: CIMDConsentSummary deleted (T143); replaced by the one plain-language access sentence + Agent Origin Label badge (FR-007, FR-008). Sentence copy not pinned. |
| `HasCIMDDomainBadge` | 1338–1339 | `GetByText("Verified domain:")` | **Pinned**: Agent Origin Label badge "Verified domain: {host}" (T044) — same phrase, now a Badge in AgentIdentityHeader; keeps `GetByText` viable. |
| `HasCIMDLocalhostWarning` | 1353–1355 | `GetByRole("alert")` filtered `HasText: "This app is requesting a redirect to your local machine"` | **Derived**: LocalhostBanner "keeps its warning content" (T087) as a prominent Alert — text and `role="alert"` expected unchanged; verify. |
| `GetCIMDLocalhostWarningText` | 1422 | `GetByRole("alert")` (unfiltered) | **Derived**: same as above; may need the same `HasText` filter once more alerts exist. |
| `ClickCIMDAdvancedDetails` | 1368 | button "Advanced Details" | **TBD**: CIMDDetails is rebuilt on Accordion (T087, T063); the trigger label "Advanced Details" is not pinned — verify name/`aria-expanded` on the Accordion trigger. |
| `IsCIMDAdvancedDetailsExpanded` | 1385–1386 | `GetByText("Client ID", exact)` | **Derived**: Accordion content with monospace values (T087); "Client ID" label expected to survive; verify. |
| `GetCIMDClientNameFromDetails` / `HasCIMDRedirectURIInDetails` / `HasCIMDScopeInDetails` / `HasCIMDClientIDInDetails` | 1434, 1445, 1457, 1468 | `GetByRole("definition")` filtered by value | **TBD**: the `definition`/`<dd>` structure is a CIMDSection artifact; Accordion content may not use a description list — re-derive scoping selectors. |
| `HasCIMDClientName` / `HasCIMDDomainText` | 1399, 1410 | page-wide `GetByText(...)` | Unchanged in mechanism; verify the console/decision split (no Agent Origin Label outside a session, T083). |

#### 1c. Overview revoke (T106) — `/delegations` → DelegationsPage Table

| Method | Line | Current selector / label | Planned change |
|---|---|---|---|
| `NavigateToOverview` | 142–145 | `Navigate(overviewPath)` with `overviewPath = "/delegations"` (line 22) | **Pinned**: route unchanged (ADR 035 map); method itself unchanged. |
| `getOverviewRevokeButton` (helper) | 1194–1196 | `GetByLabel("Revoke access for", {Exact: false}).First()` — DelegationCard `aria-label="Revoke access for {name}"` | **Pinned** component, **TBD** name: DelegationCard deleted (T143); the row action lives in the compact Table (T061, T102) offering View and a confirmed Revoke (AS-06, FR-014). Per-row accessible name not pinned — expect a row-scoped `GetByRole("button", {Name: "Revoke"})` or a new aria-label scheme. |
| `IsOverviewRevokeButtonPresent` | 1201–1207 | waits on the helper | Same locator change; "present" now means a row action button in the delegations Table. |
| `ClickOverviewRevokeButton` | 1213–1220 | clicks the helper | Same locator change; opens RevokeAgentDialog (T076) instead of RevokeGrantDialog. |
| *(related)* `ConfirmRevoke` / `CancelRevoke` / `WaitForRevokeDialog*` | see 1d | shared with detail flow | Revoke requires confirmation; success shows a toast only after the server accepts (the scenario requirements, T067 optimisticRevoke). |

#### 1d. Detail revoke + save bar (T114) — `/agents/:id` console → AgentConsolePage

| Method | Line | Current selector / label | Planned change |
|---|---|---|---|
| `revokeDialogTitle` (const) | 23 | `"Revoke All Access"` | **Pinned**: catalogue string is "Revoke all access" (T044) — casing change flows into every consumer below. The dialog is RevokeAgentDialog on Radix Dialog (T076), and it names the agent (T075, the scenario requirements). |
| `GetRevokeButton` | 1014–1018 | `GetByRole("button", {Name: "Revoke All Access"})` — resting red primary button | **Pinned**: no resting destructive action; "Revoke all access" moves into the header AgentOverflowMenu (DropdownMenu, T111, FR-016). Reached via new T026 `OpenOverflowMenu` + `ChooseRevokeAllAccess`. |
| `ClickRevokeButton` | 1023–1038 | clicks the resting button | **Pinned**: same overflow path; opens RevokeAgentDialog. |
| `IsRevokeButtonPresent` | 1107–1121 | counts the resting button | **Pinned**: now "overflow menu offers Revoke all access" (the scenario requirements) — presence check becomes menu-item presence after `OpenOverflowMenu`. |
| `ConfirmRevoke` | 1050–1066 | dialog button "Revoke All Access", fallback "Confirm" | **Derived**: RevokeAgentDialog confirm uses the `destructive` variant (contract §39); label not pinned — likely "Revoke all access"; confirm is disabled while pending (T075). |
| `CancelRevoke` | 1081–1099 | dialog button "Cancel" | **Pinned**: "Cancel" survives verbatim in the catalogue (T044); Radix Dialog replaces Headless UI (T060). |
| `revokeDialogHeading` | 1130–1133 | heading `Name: "Revoke All Access"` | **Derived**: dialog heading still names the agent; title text follows the catalogue casing; re-verify heading vs. text at T114. |
| `GetRevokeDialog` / `WaitForRevokeDialog` / `IsRevokeDialogVisible` / `WaitForRevokeDialogDismissed` | 1044–1046, 1140–1143, 1167–1173, 1227–1234 | heading-anchored, `xpath=ancestor::*[@role='dialog'][1]` | **Derived**: mechanism survives (`role="dialog"` on Radix Dialog), anchored to the renamed heading; the Headless UI portal/display:none caveat may no longer apply. |
| `revokeDialogPanel` (helper) | 1147–1149 | `[role="dialog"] .bg-white.rounded-2xl` | **Pinned must change**: raw Tailwind palette class disappears under the semantic-token rule (T054, no-raw-palette T046); re-derive from the owned Dialog part markers (e.g. `[data-slot]`). |
| `TakeRevokeDialogScreenshot` | 1153–1156 | via `revokeDialogPanel` | Follows the panel locator change. |
| `RevokeDialogContainsText` | 1180 | page-wide `GetByText(text)` | Mechanism unchanged; dialog copy ("names the agent and explains the effect", T075) TBD. |
| `IsSaveButtonEnabled` | 522–523 | `isConsentActionEnabled("Save")` | **Pinned**: GrantEditBar (T111) shows a sticky bar with "Cancel" and "Save changes" only while the draft is dirty (the scenario requirements, FR-015, contract §32). New T026 methods: `IsSaveBarVisible`, `CancelChanges`, `SaveChanges`. |
| *(new)* `OpenTab`, `ConnectionsTabRows`, `OpenOverflowMenu`, `ChooseRevokeAllAccess` | — | none today | **Pinned** additions (T026, the scenario requirements): Permissions/Connections tabs; Connections rows show state label ("Connected" / "Needs re-authentication" / "Expired" / "No connection") + action ("Connect" / "Reconnect" / "Refresh" / "Disconnect"). |

---

### 2. tests/e2e/pages/approval_page.go (T099) — `/approvals/:id` → rebuilt ApprovalReviewPage in DecisionShell

| Method | Line | Current selector / label | Planned change |
|---|---|---|---|
| `IsLoading` | 52 | `[aria-busy='true']` | **Derived**: ApprovalLoadingSkeleton rebuilt (T097); `aria-busy` preserved on Button loading (T058) and skeleton — verify. |
| `HasErrorAlert` / `GetErrorTitle` | 64, 76 | `GetByRole("alert")` → `GetByRole("heading")` | **Derived**: ApprovalErrorBanner rebuilt (T097); alert+heading structure expected to survive. |
| `HasRetryButton` | 86–87 | button "Try Again" | **TBD**: retry label not in the catalogue; re-derive from `@copy`. |
| `WaitForReviewPage` / `HasReviewHeading` | 100–101, 114–115 | heading "Tool Approval Request" | **TBD**: rebuilt page (T098); heading copy not pinned. |
| `GetToolName` | 126 | first `h3` | **TBD**: ToolCallCard rebuilt (T096); heading level/tag not pinned — T027 adds `ToolName`-style access via new methods; verify. |
| `GetAgentName` | 143 | `GetByText("Requested by")` | **Derived**: acting-user identity stays (T094); T027 adds `ActingUser`; the "Requested by" prefix is not pinned. |
| `HasSectionLabel` / `HasVisibleText` | 160–161, 172–173 | generic `GetByText` | Mechanism unchanged; used against new copy. |
| `HasGlobalErrorBoundary` | 191–192 | `GetByText("Oops! Something went wrong", exact)` | **Derived**: GlobalErrorBoundary migrated in place (T062); text expected unchanged. |
| `HasRiskBadge` | 203 | `GetByText(level + " Risk")` | **Pinned addition**: server risk becomes a labelled Badge with an accessible explanation; absent level renders **Pinned** "Risk not rated" (T044, T094). T027 adds `RiskLabel`. |
| `SelectPersistence` | 216–217 | `GetByRole("radio", {Name: label})` — labels "Just this once" / "For this session" / "Always allow" | **Derived**: PersistenceSelector rebuilt (T097) with once/session/permanent behavior preserved (T094, FR-013); radio labels not pinned — re-derive; T027 `ChooseRememberDuration` implies the choices live under "Approve and remember". |
| `HasPermanentWarning` | 227 | `GetByText("Warning:")` | **TBD**: warning copy not pinned; permanent choice still warns before confirmation (the scenario requirements). |
| `ClickApprove` | 239–240 | button "Approve" | **Pinned**: sole accent action becomes "Approve once" (T044, the scenario requirements, AS-04). T027 adds `ClickApproveOnce`. |
| `ClickDeny` | 250–252 | button "Deny" (exact) | **Pinned**: "Deny" stays (T044), now the secondary non-accent action (the scenario requirements). |
| *(new)* `ClickApproveAndRemember`, `HasDecisionActions` | — | none today | **Pinned** additions (T027, the scenario requirements): "Approve and remember" reveals session/permanent choices with the scope editor and requires confirmation. |
| `ExpandApprovalScope` | 262 | button "Approval scope" + `aria-expanded` | **Derived**: ApprovalScopeEditor is kept (T094 keeps its tests); disclosure label expected unchanged — verify. |
| `parameterScope` | 278 | `[data-testid='approval-scope-param-<key>']` | **Derived**: ApprovalScopeEditor kept — test IDs expected unchanged; verify at T099. |
| `SetParameterMode` | 285, 289 | `button[id$='-mode']` trigger + `GetByRole("option", {Name: mode})` — modes "This value" / "Any value" / "Custom match" | **Derived**: scope-preview behavior preserved unchanged (T097); Radix Select replaces the old dropdown (T059) — option role survives, trigger markup may change. |
| `SetParameterCustomPattern` | 298 | `GetByRole("textbox")` inside the param scope | **Derived**: preserved (T097); Input replaces TextInput (T059) — role unchanged. |
| `GetPatternPreview` | 307 | `GetByLabel("Approval pattern preview")` → `code` | **Derived**: scope preview preserved (T094); label expected unchanged — verify. |
| `WaitForApprovedConfirmation` / `WaitForDeniedConfirmation` | 319–320, 333–334 | headings "Approved" / "Denied" | **TBD**: ApprovalConfirmation rebuilt (T097); confirmation headings not pinned. T027 adds `ResolvedOutcomeText`. |
| `HasCloseMessage` | 347 | `GetByText("You can safely close this page")` | **TBD**: copy not pinned. |
| `HasPersistenceText` | 357 | generic `GetByText(text)` | Mechanism unchanged. |
| `HasPermanentDenialNote` | 367 | `GetByText("permanent and will apply to future requests")` | **TBD**: copy not pinned; permanent denial note expected to survive (AS-05 resolved outcomes). |
| *(new)* `ExpandArguments`, `ArgumentsText` | — | none today | **Pinned** additions (T027, T094): arguments collapsed in a monospace block with an expand control ("Show more"/"Show less" candidates, T044). |
| *(behavior)* | — | no dedicated outcome-action accessor in this page object | **Pinned**: resolved or expired approvals show their outcome and **no action buttons** (the scenario requirements, AS-05) — `HasDecisionActions` asserts absence. |

---

### 3. tests/e2e/pages/sessions_page.go (T121) — `/sessions` → ConnectionsPage

| Method | Line | Current selector / label | Planned change |
|---|---|---|---|
| `refreshButtonLocator` (helper) | 83–87 | `GetByRole("button", {Name: "Refresh", Exact: true})` | **Pinned**: "Refresh" survives verbatim (T044) but moves from SessionCard into the connections Table row actions (T061/T068; T029 `ClickRefresh`); SessionCard is deleted (T143). Locator mechanism survives; scoping may become row-based. |
| `GetRefreshButtonCount` | 27–28 | counts the helper | Follows the helper; count semantics = one per refreshable connection row. |
| `IsRefreshButtonVisible` / `ClickRefreshButton` | 35–47 | helper `.First()` | Follow the helper. |
| `WaitForSuccessMessage` | 54–55 | `GetByRole("status")` filtered `HasText: message` | **Derived**: Toaster (Sonner, T062) replaces Toast; polite `role="status"` announcements expected to keep this working — verify toast region at T121. Message text is caller-supplied (assertions unchanged). |
| `waitForPageLoad` | 70–72 | heading `Name: "Third-Party Sessions", Level: 2` | **TBD**: PageHeader title/purpose (FR-006, the scenario requirements) not pinned; nav item is "Connections" (T077) — the h2 title may change; re-derive at T121. |

---

### 4. tests/e2e/pages/tool_authorizations_page.go (T129) — `/approvals` → ApprovalsPage queue + standing decisions

| Method | Line | Current selector / label | Planned change |
|---|---|---|---|
| `WaitForPageHeading` | 46–47 | heading "Tool Authorizations" | **TBD**: PageHeader title (FR-006) not pinned; re-derive at T129. |
| `HasGlobalErrorBoundary` | 74–75 | `GetByText("Oops! Something went wrong", exact)` | **Derived**: GlobalErrorBoundary migrated in place (T062); expected unchanged. |
| `HasEmptyState` | 88 | `GetByText("No tool authorizations yet")` | **TBD**: EmptyState rebuilt with optional wordmark (T062); copy not pinned. |
| `HasPendingSection` | 100–101 | heading "Pending Approvals" | **Derived**: pending queue precedes standing decisions (FR-020, the scenario requirements); section heading copy not pinned. T028 adds `SectionOrder` and `PendingRows`. |
| `CountApproveButtons` (uses "Approve" exact) | 113–115 | `GetByRole("button", {Name: "Approve", Exact: true})` | **Pinned**: inline Approve row actions remain (AS-12, the scenario requirements), non-accent variants — label "Approve" survives; scoping becomes per-row (T028 `ApproveRow`). |
| `HasToolName` | 126 | generic `GetByText(toolName)` | Mechanism unchanged. |
| `HasRiskBadge` | 136 | `GetByText(level + " Risk")` | **Pinned addition**: labelled risk Badge; "Risk not rated" when the level is absent (T044, the scenario requirements). |
| `ClickApproveOnFirst` | 148–150 | button "Approve" (exact), `.First()` | **Pinned**: stays "Approve", row-scoped via `ApproveRow` (T028); mutation only after explicit confirmation (the scenario requirements). |
| `ClickDenyOnFirst` | 160–162 | button "Deny" (exact), `.First()` | **Pinned**: "Deny" stays; row-scoped via `DenyRow` (T028). |
| `SelectPersistence` | 172–173 | `GetByRole("radio", {Name: label}).First()` | **Derived**: per-row persistence choice (the scenario requirements `ChoosePersistenceForRow`, T028); radio labels not pinned. |
| `ClickConfirmApprove` | 183–184 | button "Confirm Approve" | **TBD**: confirmation step survives ("nothing mutates without an explicit click", the scenario requirements); label not pinned; scope preview shown before it (`ScopePreviewForRow`, T028). |
| `ClickDenyThisRequest` | 194–195 | button "Deny this request" | **TBD**: label not pinned; may collapse into inline Deny + persistence confirm — verify at T129. |
| `HasPermanentWarning` | 205 | `GetByText("grants permanent access")` | **TBD**: copy not pinned. |
| `HasPermanentSection` | 217–218 | heading "Permanent Authorizations" | **Derived**: standing allow and deny decisions sections (AS-12, the scenario requirements); headings not pinned. T028 adds `StandingDecisions`. |
| `HasPermanentlyAllowed` / `HasPermanentlyDenied` | 229, 239 | `GetByText("Permanently allowed")` / `"Permanently denied"` | **TBD**: standing-decision state labels not pinned. |
| `HasRevokeButton` / `ClickRevokeOnFirst` | 249–250, 261–262 | `GetByRole("button", {Name: "Revoke"})` | **Pinned**: standing-decision Revoke survives (AS-12, FR-020) but requires confirmation (the scenario requirements; T028 `RevokeStanding(tool)`); row-scoped, non-accent. |

---

### 5. Cross-cutting summary

- **Overview revoke (T106)**: `GetByLabel("Revoke access for …")` (DelegationCard) → Table row action "Revoke" with confirmation dialog; route `/delegations` unchanged.
- **Detail revoke (T114)**: resting "Revoke All Access" button → overflow-menu item **"Revoke all access"** (casing change, T044) in AgentOverflowMenu; `RevokeAgentDialog` (Radix Dialog) replaces Headless UI `RevokeGrantDialog`; dialog names the agent; the raw-class panel locator `.bg-white.rounded-2xl` must be replaced.
- **Approve/Deny labels (T091, T099)**: "Approve & Delegate" → **"Allow"**; new secondary **"Deny"** in consent; "Approve" → **"Approve once"** (sole accent) with new **"Approve and remember"**; "Deny" unchanged everywhere it exists.
- **Refresh (T121)**: button "Refresh" label unchanged (catalogue), relocated from SessionCard to connection-row actions.
- **Success toasts (all)**: Toast → Sonner Toaster (T062), polite `role="status"` announcements; "Grant updated successfully" copy and error-toast `role="alert"` are **not pinned** — re-derive at T091; revocation success/error toasts pinned only as "success toast after the server succeeds / error toast" (the scenario requirements).
- **Validity controls (T091)**: checkbox "Specific end date" + label "End date" → RadioGroup **"Until revoked" / "30 days" / "Custom date"** + DatePicker with min-date and inline validation (T044, T059, T071, T089); re-consent preselects "Custom date" at the existing `valid_until`, or "Until revoked" for a null one.
- **Save bar (T114)**: always-on "Save" button → sticky GrantEditBar "Cancel" / **"Save changes"**, present only while dirty.
- **Connection states (T114/T121-adjacent)**: pinned labels "Connected", "Needs re-authentication", "Expired", "No connection", "Connect", "Reconnect", "Refresh", "Disconnect" (T044, T073).

## Consumers to migrate

Feature: 047-redesign-consent-console, task T002. Evidence collected 2026-09-27 on worktree worktree-rapid-harbor-2be6 (branch state after T001). Scope: web/src/ and web/index.html. No repository files were edited; this artifact is the evidence record for the parent's cutover-inventory.md integration.

### Exact commands (rg equivalents of the grep-tool runs)

All searches ran with the omp grep tool (Rust regex, gitignore-respecting, case-sensitive) over the combined scope web/src + web/index.html. Equivalent reproducible commands for T148:

```sh
rg -n '@headlessui/react' web/src web/index.html
rg -n 'framer-motion' web/src web/index.html
rg -n 'apiCache' web/src web/index.html
rg -n '@components/ui/' web/src web/index.html
rg -n 'components/ui/' web/src web/index.html
rg -n 'PageTransition' web/src web/index.html
rg -n "(from|import)\s+['\"][^'\"]*design-system/tokens" web/src web/index.html
rg -n 'Crimson Pro' web/src web/index.html
rg -n 'Manrope' web/src web/index.html
rg -n 'linear-gradient' web/src web/index.html
rg -n 'bg-gradient-' web/src web/index.html
rg -n '(bg|text|border|ring|from|to|via|fill|stroke|divide|outline)-(slate|gray|zinc|neutral|stone|red|orange|amber|yellow|lime|green|emerald|teal|cyan|sky|blue|indigo|violet|purple|fuchsia|pink|rose)-[0-9]{2,3}' web/src web/index.html
```

Note for T148: the design-system/tokens command matches import/from '…design-system/tokens' lines only (the task's TypeScript imports). A bare rg -n 'design-system/tokens' additionally matches docs prose and a CSS @import; both variants are recorded below.

### Summary counts

| # | Pattern | Files | Hits | web/index.html |
|---|---------|-------|------|----------------|
| 1 | @headlessui/react | 8 | 8 | 0 |
| 2 | framer-motion | 5 | 5 | 0 |
| 3 | apiCache | 4 | 29 | 0 |
| 4 | @components/ui/ | 15 | 28 | 0 |
| 5 | components/ui/ (superset incl. relative) | 17 | 30 | 0 |
| 6 | PageTransition | 12 | 71 | 0 |
| 7 | TS imports of design-system/tokens | 0 runtime | 0 runtime TS imports; 3 non-TS references | 0 |
| 8 | Crimson Pro | 7 | 21 | 0 |
| 9 | Manrope | 10 | 27 | 0 |
| 10 | linear-gradient | 2 | 2 | 0 |
| 11 | bg-gradient- | 8 | 12 | 0 |
| 12 | Tailwind palette utilities | 113 | 390 line hits (see section 12) | 0 |

web/index.html: zero hits for every pattern family (verified with the combined alternation of all patterns against web/index.html; result: no matches).

### 1. @headlessui/react — 8 hits / 8 files

- web/src/design-system/components/advanced/Accordion/Accordion.tsx:22 import { Disclosure, Transition } from '@headlessui/react';
- web/src/design-system/components/inputs/Select/Select.tsx:18 import { Listbox, Transition } from '@headlessui/react';
- web/src/design-system/components/inputs/Switch/Switch.tsx:17 import { Switch as HeadlessSwitch } from '@headlessui/react';
- web/src/design-system/components/layout/PageTransition/PageTransition.tsx:19 import { Transition } from '@headlessui/react';
- web/src/design-system/components/navigation/Tabs/Tabs.tsx:19 import { Tab } from '@headlessui/react';
- web/src/design-system/components/overlays/Dropdown/Dropdown.tsx:19 import { Menu, Transition } from '@headlessui/react';
- web/src/design-system/components/overlays/Modal/Modal.tsx:19 import { Dialog, Transition } from '@headlessui/react';
- web/src/design-system/components/overlays/Popover/Popover.tsx:21 import { Popover as HeadlessPopover, Transition } from '@headlessui/react';

### 2. framer-motion — 5 hits / 5 files

- web/src/components/consent/DelegationList.tsx:12 import { motion } from 'framer-motion';
- web/src/components/ui/PageTransition.tsx:25 import { motion, Variants } from 'framer-motion';
- web/src/components/ui/Skeleton.tsx:10 import { motion } from 'framer-motion';
- web/src/components/ui/Toast.tsx:34 import { motion, AnimatePresence } from 'framer-motion';
- web/src/design-system/docs/MOTION_GUIDE.md:409 import { motion } from 'framer-motion'; (doc example)

### 3. apiCache — 29 hits / 4 files

web/src/services/api/cache.ts (8): :14 (JSDoc import example), :17 (JSDoc get), :22 (JSDoc set), :25 (JSDoc invalidate), :162 (JSDoc invalidatePattern), :238 export const apiCache = new ApiCache();, :244 apiCache.cleanup();, :250 export default apiCache;

web/src/services/api/consent.test.ts (1): :14 (vi.mock factory key)

web/src/services/api/consent.ts (13): :10 import { apiCache } from './cache';, :45 get(UserInfo), :55 set 5min, :71 get(AgentDelegation[]), :82 set 5min, :113 get agent detail, :148 set 5min, :166 get(UserGrant or null), :178 set 5min, :208 invalidatePattern('/consent/agents/${agentId}*'), :209 invalidate('/consent/agents'), :254 invalidatePattern('/consent/agents/${agentId}*'), :255 invalidate('/consent/agents')

web/src/services/api/sessions.ts (7): :9 import { apiCache } from './cache';, :104 get(SessionSummary[]), :116 set 2min, :133 get(SessionDetail), :145 set 2min, :161 invalidatePattern('/third-party/*'), :178 invalidatePattern('/third-party/*')

### 4. @components/ui/ (aliased imports) — 28 hits / 15 files

- web/src/App.tsx:20 GlobalErrorBoundary, :21 ToastProvider
- web/src/components/approvals/ApprovalLoadingSkeleton.tsx:5 Skeleton
- web/src/components/approvals/ApprovalReviewPage.tsx:9 Button
- web/src/components/approvals/ApprovalScopeEditor.tsx:15 Button
- web/src/components/consent/RevokeGrantButton.test.tsx:14 ToastProvider
- web/src/components/consent/RevokeGrantButton.tsx:10 useToast
- web/src/components/ui/GlobalErrorBoundary.tsx:14 (JSDoc example)
- web/src/components/ui/PageTransition.tsx:12 (JSDoc example)
- web/src/components/ui/Toast.tsx:14 (JSDoc example)
- web/src/pages/AgentGrantDetailPage.tsx:20 PageTransition, :21 Skeleton, :22 InlineError, :23 Button, :24 useToast
- web/src/pages/ConsentOverviewPage.test.tsx:17 ToastProvider
- web/src/pages/ConsentOverviewPage.tsx:18 PageTransition, :19 DelegationListSkeleton, :20 InlineError, :21 EmptyState, :25 useToast
- web/src/pages/NotFoundPage.tsx:13 PageTransition, :14 Button
- web/src/pages/ThirdPartySessionsPage.tsx:25 PageTransition
- web/src/pages/ToolAuthorizationsPage.tsx:11 PageTransition, :12 InlineError, :13 Button, :14 Skeleton

### 5. components/ui/ (any path form) — 30 hits / 17 files

Everything in section 4 plus the two relative-form imports:
- web/src/pages/AgentGrantDetailPage.integration.test.tsx:10 import { ToastProvider } from '../components/ui/Toast';
- web/src/pages/AgentGrantDetailPage.test.tsx:15 import { ToastProvider } from '../components/ui/Toast';

(Section 4 lists the other 28. Total 30 hits, 17 files. Note: web/src/components/ui/ itself contains Button, DatePicker, EmptyState, ErrorBoundary, GlobalErrorBoundary, InlineError, PageTransition, Skeleton, Toast — the whole directory is deleted per T014(b).)

### 6. PageTransition — 71 hits / 12 files

web/src/components/ui/PageTransition.tsx (8, legacy component to delete): :2 (JSDoc header), :12 (JSDoc import example), :16 (JSDoc JSX), :18 interface PageTransitionProps, :27 (JSDoc), :58 (doc comment), :61 export function PageTransition({, :118 export default PageTransition;

web/src/design-system/GettingStarted.mdx (1): :89
web/src/design-system/components/layout/index.ts (1): :8 re-export
web/src/design-system/components/layout/PageTransition/PageTransition.stories.tsx (17): :2, :9, :12, :13, :19, :26, :59, :61, :92, :113, :281, :291, :298, :302, :479, :483, :504
web/src/design-system/components/layout/PageTransition/PageTransition.tsx (17): :2, :33, :140, :141, :147, :149, :152, :154, :157, :158, :163, :165, :166, :170, :172, :246, :248
web/src/design-system/components/layout/PageTransition/index.ts (3): :6, :7, :8
web/src/design-system/docs/COMPOSITION_PATTERNS.md (3): :125 import, :129, :137
web/src/pages/AgentGrantDetailPage.tsx (3): :20 import, :492 open, :755 close
web/src/pages/ConsentOverviewPage.tsx (3): :18 import, :79 open, :153 close
web/src/pages/NotFoundPage.tsx (3): :13 import, :67 open, :137 close
web/src/pages/ThirdPartySessionsPage.tsx (9): :25 import, :224, :264, :273, :305, :314, :353, :361, :411 (JSX open/close across the four return branches)
web/src/pages/ToolAuthorizationsPage.tsx (3): :11 import, :324 open, :390 close

Both the legacy components/ui/PageTransition and the design-system layout/PageTransition are removed by the cutover (T014: layout shells ConsoleShell/DecisionShell/PageHeader replace them).

### 7. TypeScript imports of design-system/tokens — 0 runtime TS imports

Pattern (from|import) followed by a quoted path containing design-system/tokens matched 3 lines, none a real TS import:
- web/src/design-system/GettingStarted.mdx:102 (MDX code example)
- web/src/design-system/tokens/index.ts:3 (doc comment)
- web/src/styles/index.css:2 @import '../design-system/tokens/colors.css'; (CSS import)

Broader design-system/tokens prose references (docs, not imports): COLOR_GUIDE.md:11,48; DESIGN_PRINCIPLES.md:27,83; TOKEN_GUIDE.md:413,440. Out-of-pattern caveat: relative token imports such as from './colors' (tokens/index.ts:6) are not covered by the task's specified pattern.

### 8. Crimson Pro — 21 hits / 7 files

- web/src/design-system/GettingStarted.mdx:36, :45
- web/src/design-system/docs/COMMON_MISTAKES.md:410, :421, :457, :468, :605
- web/src/design-system/docs/DESIGN_PRINCIPLES.md:13
- web/src/design-system/docs/INDEX.md:49, :65, :328
- web/src/design-system/docs/TOKEN_GUIDE.md:216, :240
- web/src/design-system/tokens/typography.ts:5, :10
- web/src/styles/fonts.css:3, :8, :16, :26, :36, :165 (three @font-face blocks at :15-:42 plus --font-display at :165)

### 9. Manrope — 27 hits / 10 files

- web/src/design-system/GettingStarted.mdx:36, :46
- web/src/design-system/docs/COMMON_MISTAKES.md:422, :468
- web/src/design-system/docs/COMPONENT_ARCHETYPES.md:27, :47, :242
- web/src/design-system/docs/DESIGN_PRINCIPLES.md:13
- web/src/design-system/docs/INDEX.md:50, :66, :104, :329
- web/src/design-system/docs/TOKEN_GUIDE.md:223, :241
- web/src/design-system/tokens/typography.ts:5, :11
- web/src/styles/fonts.css:4, :8, :46, :56, :66, :76, :86, :96, :166 (six @font-face blocks at :45-:100 plus --font-sans at :166)
- web/src/styles/index.css:16

### 10. linear-gradient — 2 hits / 2 files

- web/src/design-system/docs/MOTION_GUIDE.md:270 (skeleton shimmer doc example)
- web/src/styles/index.css:29 (body background, cream to sand)

### 11. bg-gradient- — 12 hits / 8 files

- web/src/components/layout/AppLayout.tsx:41 (from-trust-hover to-trust), :125 (from-cream via-sand to-taupe/20)
- web/src/components/ui/GlobalErrorBoundary.tsx:106 (from-neutral-50 to-neutral-100), :109 (from-red-500 to-red-600)
- web/src/design-system/components/advanced/Progress/Progress.tsx:57 (striped variant)
- web/src/design-system/components/feedback/ErrorBoundary/ErrorBoundary.stories.tsx:310 (from-purple-50 to-pink-50)
- web/src/design-system/components/feedback/GlobalErrorBoundary/GlobalErrorBoundary.stories.tsx:61 (from-blue-50 to-indigo-100), :294, :499 (from-amber-50 to-orange-100)
- web/src/design-system/components/feedback/GlobalErrorBoundary/GlobalErrorBoundary.tsx:431 (from-error-light via-error-light/70 to-error-light/50)
- web/src/design-system/components/layout/Container/Container.stories.tsx:342 (from-trust-light to-success-light)
- web/src/pages/AgentGrantDetailPage.tsx:515 (from-trust to-trust-hover)

### 12. Tailwind palette utilities — 390 line hits / 113 files

Pattern: (bg|text|border|ring|from|to|via|fill|stroke|divide|outline)-(slate|gray|zinc|neutral|stone|red|orange|amber|yellow|lime|green|emerald|teal|cyan|sky|blue|indigo|violet|purple|fuchsia|pink|rose)-[0-9]{2,3}

Per-file hit lines (line numbers only; content is the listed className or prose line):

1. web/src/App.tsx — 39, 41, 42
2. web/src/components/approvals/ApprovalConfirmation.tsx — 36, 39, 59, 69, 85, 89, 92, 96
3. web/src/components/approvals/ApprovalRequestSummary.tsx — 40, 43, 50, 53, 62, 63, 65, 69, 70, 81, 84, 90, 91, 101, 102, 103, 109
4. web/src/components/approvals/ApprovalReviewPage.tsx — 121, 124, 158, 181, 190
5. web/src/components/approvals/ApprovalScopeEditor.tsx — 200, 207, 218, 219, 280, 282, 300
6. web/src/components/approvals/PersistenceSelector.tsx — 52, 68
7. web/src/components/consent/CIMDAdvancedDetails.tsx — 15, 18, 23, 41, 42
8. web/src/components/consent/CIMDConsentSummary.tsx — 10, 24, 27, 30
9. web/src/components/consent/CIMDLocalhostWarning.tsx — 11
10. web/src/components/consent/DelegationCard.tsx — 59, 76, 79, 87, 107, 114, 127
11. web/src/components/consent/GrantStatusBadge.tsx — 141
12. web/src/components/consent/GrantValidityControl.tsx — 91, 121, 130, 133, 145
13. web/src/components/consent/PermissionSetCard.tsx — 110, 136, 143, 162, 172, 186, 209, 244
14. web/src/components/consent/PermissionSetsList.tsx — 255, 267, 287
15. web/src/components/consent/RevokeGrantDialog.tsx — 74
16. web/src/components/consent/ScopeList.tsx — 65, 89
17. web/src/components/consent/ServiceCard.tsx — 108
18. web/src/components/consent/ServiceRequirementCard.test.tsx — 80
19. web/src/components/consent/ServiceRequirementCard.tsx — 107, 114, 121, 131
20. web/src/components/consent/ServiceRequirementsList.tsx — 63, 74, 97
21. web/src/components/layout/AppLayout.tsx — 36, 60, 80, 92, 104
22. web/src/components/layout/Header.tsx — 39, 64, 77
23. web/src/components/sessions/DESIGN_SYSTEM_USAGE.md — 283, 286, 536, 631, 634
24. web/src/components/sessions/SessionCard.tsx — 111, 250
25. web/src/components/sessions/TerminationDialog.tsx — 89, 95, 117
26. web/src/components/ui/DatePicker.tsx — 92, 108, 111, 112, 119
27. web/src/components/ui/EmptyState.tsx — 44, 60, 63, 73
28. web/src/components/ui/ErrorBoundary.tsx — 74, 75, 78, 90, 95, 101, 102, 105, 108, 119, 125
29. web/src/components/ui/GlobalErrorBoundary.tsx — 106, 107, 109, 112, 129, 136, 144, 145, 163, 166, 176, 187, 190, 203, 206, 221, 240, 260, 261
30. web/src/components/ui/InlineError.tsx — 35, 42, 58, 59, 66, 77
31. web/src/components/ui/Skeleton.tsx — 33
32. web/src/components/ui/Toast.tsx — 169, 170, 171, 172, 175, 176, 177, 178, 181, 182, 183, 184, 187, 188, 189, 190, 230
33. web/src/design-system/components/advanced/Accordion/Accordion.stories.tsx — 106, 136, 148, 152, 166, 170, 252, 257, 262, 267, 282, 288, 294, 309, 315, 321, 342, 353, 363, 373
34. web/src/design-system/components/advanced/Accordion/Accordion.tsx — 28, 53, 56, 70, 257, 302, 306, 317
35. web/src/design-system/components/advanced/Progress/Progress.stories.tsx — 139, 149, 159, 169, 183, 188, 195, 236, 249, 262, 275, 297, 304, 311, 318, 339, 351, 364, 411, 416
36. web/src/design-system/components/advanced/Progress/Progress.tsx — 27, 53, 54, 80, 85, 104, 105, 221, 226, 278, 304, 333
37. web/src/design-system/components/data-display/Card/Card.stories.tsx — 69, 70, 86, 87, 93, 108, 111, 137, 145, 150, 159, 183, 191, 194, 201, 205, 211, 214, 232, 235
38. web/src/design-system/components/data-display/Card/Card.tsx — 45, 48, 62, 65, 252
39. web/src/design-system/components/data-display/ScopeList/ScopeList.stories.tsx — 312, 313, 316, 452, 455, 479, 480, 483, 558, 561, 607, 608, 609, 616, 640, 641, 644, 651
40. web/src/design-system/components/data-display/ScopeList/ScopeList.tsx — 51, 63, 71, 430, 449, 457, 469, 472, 492, 499, 507, 508, 509, 522, 540, 543, 552, 565, 568, 582
41. web/src/design-system/components/data-display/StatusIndicator/StatusIndicator.stories.tsx — 132, 134, 137
42. web/src/design-system/components/data-display/StatusIndicator/StatusIndicator.tsx — 21, 25
43. web/src/design-system/components/data-display/Table/Table.stories.tsx — 348, 349, 384, 415, 427, 430, 583, 592, 771, 772, 815
44. web/src/design-system/components/data-display/Table/Table.tsx — 40, 53, 54, 68, 72, 146, 156, 230, 287, 324, 340, 347, 364
45. web/src/design-system/components/feedback/Alert/Alert.stories.tsx — 174, 175, 297, 298, 335, 352, 385, 386, 494, 608, 609, 612
46. web/src/design-system/components/feedback/EmptyState/EmptyState.stories.tsx — 14, 31, 48, 65, 82, 99, 265, 358, 359, 374, 375, 386, 387, 402, 404, 412, 440, 441, 456, 457
47. web/src/design-system/components/feedback/EmptyState/EmptyState.tsx — 59, 72, 125
48. web/src/design-system/components/feedback/ErrorBoundary/ErrorBoundary.stories.tsx — 44, 45, 48, 71, 72, 105, 145, 185, 212, 213, 222, 223, 247, 248, 279, 280
49. web/src/design-system/components/feedback/GlobalErrorBoundary/GlobalErrorBoundary.stories.tsx — 61, 63, 66, 206, 211, 267, 294, 295, 297, 299, 312, 316, 330, 380, 381, 439, 465, 499, 501, 504
50. web/src/design-system/components/feedback/GlobalErrorBoundary/GlobalErrorBoundary.tsx — 491
51. web/src/design-system/components/feedback/InlineError/InlineError.stories.tsx — 106, 115, 124, 149, 181, 210, 254, 284, 328, 362, 409, 431, 432, 455, 480, 519, 542, 578, 597, 632
52. web/src/design-system/components/feedback/Skeleton/Skeleton.stories.tsx — 64, 81, 104, 117, 146, 184, 186, 196, 226, 229, 239, 242, 270, 277, 286, 293, 300, 324, 335, 346
53. web/src/design-system/components/feedback/Skeleton/Skeleton.tsx — 22
54. web/src/design-system/components/feedback/Toast/Toast.stories.tsx — 124, 125, 271, 272, 305, 306, 309, 310, 420, 421, 424, 425, 455, 460, 497, 500, 503, 573, 574, 577, 607, 608, 673
55. web/src/design-system/components/inputs/Checkbox/Checkbox.stories.tsx — 83, 84
56. web/src/design-system/components/inputs/Checkbox/Checkbox.tsx — 31, 132, 133, 141
57. web/src/design-system/components/inputs/DatePicker/DatePicker.stories.tsx — 155, 156, 169
58. web/src/design-system/components/inputs/DatePicker/DatePicker.tsx — 33, 35, 37, 177, 195, 231
59. web/src/design-system/components/inputs/Radio/Radio.stories.tsx — 89, 90, 158
60. web/src/design-system/components/inputs/Radio/Radio.tsx — 30, 132, 133, 141
61. web/src/design-system/components/inputs/Select/Select.stories.tsx — 191, 192, 193, 256, 402, 404, 485, 486, 501, 502, 505, 556
62. web/src/design-system/components/inputs/Select/Select.tsx — 43, 61, 82, 261, 324, 330, 379, 415
63. web/src/design-system/components/inputs/Switch/Switch.stories.tsx — 135, 136, 179, 195, 196
64. web/src/design-system/components/inputs/Switch/Switch.tsx — 32, 34, 150, 151, 159
65. web/src/design-system/components/inputs/TextArea/TextArea.stories.tsx — 109, 111
66. web/src/design-system/components/inputs/TextArea/TextArea.tsx — 27, 29, 31, 144, 170, 190, 200
67. web/src/design-system/components/inputs/TextInput/TextInput.stories.tsx — 201, 203, 245, 264, 282
68. web/src/design-system/components/inputs/TextInput/TextInput.tsx — 33, 35, 37, 163, 176, 196, 206, 259, 269
69. web/src/design-system/components/layout/AppLayout/AppLayout.stories.tsx — 107, 113, 119, 125, 138, 144, 145, 150, 151, 156, 157, 164, 165, 169, 172, 174, 176, 179, 181, 184, 186, 198, 202
70. web/src/design-system/components/layout/AppLayout/AppLayout.tsx — 35, 60, 114
71. web/src/design-system/components/layout/Container/Container.stories.tsx — 207, 215, 327, 330
72. web/src/design-system/components/layout/Grid/Grid.stories.tsx — 57, 59
73. web/src/design-system/components/layout/PageTransition/PageTransition.stories.tsx — 437, 438, 538, 539, 545
74. web/src/design-system/components/layout/Stack/Stack.stories.tsx — 83, 85, 229, 245, 261, 277, 308, 324, 340, 356, 371, 537, 573, 612, 616, 620, 624
75. web/src/design-system/components/navigation/Breadcrumb/Breadcrumb.stories.tsx — 172, 190, 233, 245, 257, 280, 293, 307, 321, 376, 391, 409, 424, 494, 497, 512, 515, 530, 533, 548
76. web/src/design-system/components/navigation/Breadcrumb/Breadcrumb.tsx — 77
77. web/src/design-system/components/navigation/Pagination/Pagination.stories.tsx — 115, 125, 135, 173, 184, 191, 197, 203, 209, 215, 234, 246, 258, 270, 305, 317, 329, 354, 358, 379
78. web/src/design-system/components/navigation/Pagination/Pagination.tsx — 49, 51, 263, 333
79. web/src/design-system/components/navigation/Tabs/Tabs.stories.tsx — 95, 96, 97, 101, 102, 103, 107, 108, 109, 122, 126, 127, 128, 138, 139, 140, 150
80. web/src/design-system/components/navigation/Tabs/Tabs.tsx — 37, 38, 50, 65, 68, 94, 105, 117
81. web/src/design-system/components/overlays/Dropdown/Dropdown.stories.tsx — 659, 664
82. web/src/design-system/components/overlays/Dropdown/Dropdown.tsx — 60, 61, 260, 304, 305, 319
83. web/src/design-system/components/overlays/Modal/Modal.stories.tsx — 60, 107, 112, 150, 167, 184, 188, 189, 192, 215, 249, 297, 300, 378, 448, 457, 465, 474, 490
84. web/src/design-system/components/overlays/Modal/Modal.tsx — 183, 197, 207, 232
85. web/src/design-system/components/overlays/Popover/Popover.stories.tsx — 153, 210, 211, 214, 215, 218, 219, 396, 399, 400, 426, 429, 439, 461, 540, 555, 591, 661, 664, 669
86. web/src/design-system/components/overlays/Popover/Popover.tsx — 27, 58, 188, 190, 202, 212
87. web/src/design-system/components/overlays/Tooltip/Tooltip.stories.tsx — 120, 233, 252, 268, 285, 297, 310, 335, 357, 358, 361, 363, 389, 472, 477, 481, 489, 503, 519, 549
88. web/src/design-system/components/overlays/Tooltip/Tooltip.tsx — 28, 31, 50, 51
89. web/src/design-system/components/primitives/Avatar/Avatar.stories.tsx — 73, 77, 80, 84, 88, 103, 107, 110, 139, 143, 146, 150, 191, 195, 248, 273, 293, 302, 303
90. web/src/design-system/components/primitives/Avatar/Avatar.tsx — 22, 58, 160, 166
91. web/src/design-system/components/primitives/Badge/Badge.stories.tsx — 264, 285, 306, 346, 363
92. web/src/design-system/components/primitives/Badge/Badge.tsx — 39
93. web/src/design-system/components/primitives/Button/Button.stories.tsx — 118, 127, 142, 281, 298, 373, 376, 387, 390, 400, 403
94. web/src/design-system/components/primitives/Button/Button.tsx — 35, 39
95. web/src/design-system/components/primitives/Divider/Divider.stories.tsx — 70, 75, 80, 94, 99, 127, 134, 141, 156, 157, 160, 161, 172, 176, 179, 184, 189, 198, 213, 216
96. web/src/design-system/components/primitives/Divider/Divider.tsx — 49, 50, 51, 58, 59, 64, 202
97. web/src/design-system/components/primitives/Spinner/Spinner.stories.tsx — 70, 74, 78, 82, 96, 100, 104, 108, 112, 116, 152, 156, 162, 173, 176, 190, 204, 207, 210, 215
98. web/src/design-system/components/primitives/Spinner/Spinner.tsx — 44
99. web/src/design-system/docs/COLOR_GUIDE.md — 101
100. web/src/design-system/docs/COMMON_MISTAKES.md — 38, 42, 63, 106, 339, 385, 528, 537, 607
101. web/src/design-system/docs/COMPONENT_ARCHETYPES.md — 191, 207, 307, 420, 426, 446, 449
102. web/src/design-system/docs/COMPONENT_PAIRING_GUIDE.md — 40, 143, 197, 209, 227, 355, 380, 386, 398, 465, 466, 475, 509, 542, 567, 603, 666, 721
103. web/src/design-system/docs/DECISION_TREES.md — 132, 135, 436, 437, 438, 462, 463, 535
104. web/src/design-system/docs/TOKEN_GUIDE.md — 115, 116, 117, 118, 119, 274
105. web/src/design-system/oauth2-semantic-tokens.md — 177, 180, 187, 188, 189, 190, 192, 215, 222, 223, 225, 261, 287, 531, 559
106. web/src/design-system/tokens/colors.css — 14
107. web/src/design-system/utils/cn.ts — 6
108. web/src/pages/AgentGrantDetailPage.tsx — 533, 683, 687, 698
109. web/src/pages/ConsentOverviewPage.tsx — 83, 86, 96, 114, 123
110. web/src/pages/ErrorPage.tsx — 14, 15, 18, 24
111. web/src/pages/NotFoundPage.tsx — 66, 70, 72, 87, 90, 93, 94, 101, 104, 133
112. web/src/pages/ThirdPartySessionsPage.tsx — 240, 243, 250, 289, 292, 331, 333, 340, 377, 380, 387
113. web/src/pages/ToolAuthorizationsPage.tsx — 52, 58, 133, 170, 186, 199, 232, 235, 237, 240, 254, 328, 329, 349, 350, 361, 376

Pagination record: 7 tool pages (skip = 0, 20, 40, 60, 80, 100, plus a single-file verification of ToolAuthorizationsPage.tsx confirming the file ends at line 379 and the hit list is complete). web/index.html contributed zero hits (checked with the full alternation of all patterns).

Counts note: 390 is the sum of the per-file line numbers above. neutral-* dominates the raw palette usage; note that neutral here is the current warm-neutral token scale defined in web/src/design-system/tokens/colors.css — T046's ESLint rule and T016 decide whether those neutral-NNN utilities are retired with the raw palette or re-anchored to semantic tokens; the inventory records hits mechanically per the T002 pattern.

### Cutover reading (for parent's integration, not repo edits)

- @headlessui/react is imported only inside design-system components that T014 replaces in place or deletes; no page imports it directly.
- framer-motion runtime imports live in legacy components/ui/* (deleted) and components/consent/DelegationList.tsx (migrated to CSS-only motion per T019); MOTION_GUIDE.md:409 is doc content updated by T019.
- apiCache is entirely owned by web/src/services/api/ (cache.ts + consent/sessions consumers + one test mock); ADR 038 moves server-state ownership, so this module is the cutover's server-state surface.
- components/ui/ has 17 consumer files (15 aliased + 2 relative test imports) plus App.tsx.
- Both PageTransition implementations (legacy + design-system) and all 12 consumer files are in scope for the shell replacement.
- design-system/tokens has zero runtime TS imports; only an MDX example, a doc comment, and a CSS @import reference it.
- Fonts: Crimson Pro and Manrope are defined in web/src/styles/fonts.css and referenced across design-system docs and tokens/typography.ts; T014(e) replaces both with Zalando Sans + Inter.

## Design preconditions

T005 confirms that Agent, UserGrant, Permission Set, UserSession, ToolApproval, and authorization context retain their ownership and semantics.
Only UI projections and transient state are new. Browser preferences remain `aib.theme` (`light`, `dark`, `system`) and `aib.sidebar-collapsed` (`true`, `false`).
Defaults remain `system` and `false`. Neither preference contains credentials or submits a decision.

T006 and T007 add the six presentation terms and the consent/security invariants to `ARCHITECTURE.md`.

### Configuration and deployment

T008: **No configuration change**. `contracts/ui-and-configuration.md` prohibits redesign flags, environment variables, CLI options, HTML configuration delivery, and configuration endpoints.
No change is required in `examples/config/` or its README.

T009: The Helm values, templates, and README contain no SPA CSP or conflicting frame/content-type header setting.
Ingress annotations are user-supplied, with empty defaults. Documented annotations concern certificates, rate limits, and IP restrictions, not SPA response headers.
Telemetry exporter headers are outbound collector configuration, not browser headers.
The broker remains the owner of SPA response headers. No Helm change is needed.

### Persistence

T012: The feature adds no database entity, schema, migration, or storage repository.
`migrations/` and `internal/ports/storage.go` require no change.

### Stakeholder API scope

T011 records the 2026-09-26 prohibition on endpoint, response-field, runtime-response, persistence, and OAuth2/token changes.
The stakeholder approved the session-list documentation correction on 2026-09-27.
During this implementation session, the stakeholder also selected **Correct the documentation** for verified existing consent-response documentation mismatches.
That permission covers documentation only. It does not authorize a new endpoint or a runtime response or authorization change.
The admin API is unaffected.

## Visual gate development evidence

T040's five CLI regression cases failed before implementation because the old command returned usage exit 2 instead of a gate result.
After implementation, `go test ./tools/imgdiff -count=1` passed.
A temporary runtime smoke invoked `go run ./tools/imgdiff gate` with generated PNG captures and baselines.
Identical captures returned exit 0. One changed pixel in a one-pixel image returned exit 1 with a 100% difference.
Both themes produced actual, expected, and diff PNG artifacts. The temporary files were removed.

T041 changes screenshot automation to review artifacts only. It no longer commits baselines or modifies PR descriptions.
The frontend CI job runs the visual comparison recipe and uploads difference artifacts on failure.

## Design system

### T014 review evidence — 2026-09-27

ADR 038 is Accepted, dated 2026-09-27. Its acceptance permits guidance changes but does not prove runtime completion.
This section records documentation review and the replacement contract only.

I read every section of the four required guides before their replacement:

| Reviewed file | Pre-edit evidence | Required correction |
| --- | --- | --- |
| `web/src/design-system/docs/INDEX.md` | Lines 1–355. The authority section said ADR 038 was unaccepted. Its summaries named old fonts, gradients, strong shadows, 150–500ms motion, danger buttons, and Modal. | Publish accepted authority, one guide map, new fonts, semantic tokens, current variants, CSS-only motion, and blocking both-theme evidence. T015 updates this file. |
| `web/src/design-system/docs/DECISION_TREES.md` | Lines 1–552. The action tree used danger and allowed one primary per section. Text and status trees used the retired palette. Motion and shadow trees selected spring effects, card lifts, and page transitions. | T017 must use destructive confirmation, at most one accent action per view, semantic foregrounds, 120–200ms ease-out, and current owned component names. |
| `web/src/design-system/docs/COMPONENT_PAIRING_GUIDE.md` | Lines 1–725. Pairings used primary plus danger for approval, repeated card primaries, TextInput, Modal, raw scopes, a scopeCount-derived permission count, last-use text, and palette literals. | T018 must preserve consent semantics, use ConsoleShell/DecisionShell/PageHeader, use current components, remove unsupported counts and last use, and keep Table/Command out of decision imports. |
| `web/src/design-system/docs/COMMON_MISTAKES.md` | Lines 1–620. Several recommended fixes still selected the retired palette. Other rules required card shadows, old fonts, longer motion, danger zones, and a primary per section. | T017 must replace the fixes themselves, not only their authority note. Current examples must use semantic roles, current variants, truthful state, and the accepted visual system. |

These ranges refer to the files as reviewed, not to line numbers after the documentation rewrite.
The review also used feature 047 `spec.md`, `plan.md`, `tasks.md`, and `data-model.md`, accepted ADR 038, and Constitution Principle XI.

### Component replacement inventory

“In place” means rewrite the existing directory, not retain its old API or presentation.
“Replaces” identifies a differently named source that must disappear after its consumers migrate.
This table specifies targets. It is not proof that their runtime implementation exists.

| Category | Component | Target |
| --- | --- | --- |
| `primitives/` | Button | In place |
| `primitives/` | Badge | In place |
| `primitives/` | Avatar | In place |
| `primitives/` | Separator | Replaces Divider |
| `primitives/` | Wordmark | New |
| `inputs/` | Select | In place |
| `inputs/` | Switch | In place |
| `inputs/` | Checkbox | In place |
| `inputs/` | TextArea | In place |
| `inputs/` | DatePicker | In place |
| `inputs/` | Input | Replaces TextInput |
| `inputs/` | RadioGroup | Replaces Radio |
| `overlays/` | Popover | In place |
| `overlays/` | Tooltip | In place |
| `overlays/` | Dialog | Replaces Modal |
| `overlays/` | DropdownMenu | Replaces Dropdown |
| `overlays/` | Sheet | New |
| `navigation/` | Tabs | In place |
| `data-display/` | Card | In place |
| `data-display/` | Table | In place |
| `data-display/` | TruncatedText | New |
| `feedback/` | Skeleton | In place |
| `feedback/` | Alert | In place |
| `feedback/` | EmptyState | In place |
| `feedback/` | InlineError | In place |
| `feedback/` | ErrorBoundary | In place |
| `feedback/` | GlobalErrorBoundary | In place |
| `feedback/` | Toaster | Replaces Toast, using Sonner |
| `advanced/` | Accordion | In place |
| `advanced/` | Command | New, using cmdk |
| `layout/` | ConsoleShell | New, part of the AppLayout/PageTransition replacement |
| `layout/` | DecisionShell | New, part of the AppLayout/PageTransition replacement |
| `layout/` | PageHeader | New, part of the AppLayout/PageTransition replacement |
| `web/src/design-system/theme/` | ThemeChoice | New |

Keep the existing categories, path aliases, CVA, and `cn()` utility.
Do not create a competing `components/ui` library.
No barrel imported by decision routes can re-export Table or Command. Console consumers import concrete modules.

The new universal components are Wordmark, TruncatedText, ThemeChoice, ConsoleShell, DecisionShell, and PageHeader.
Their labels and other UI copy come through required props or children.
Pages and application components obtain their strings from `@copy`.

### Components and duplicate library to delete

After migration, delete these replaced paths:

- `web/src/design-system/components/overlays/Modal/`
- `web/src/design-system/components/overlays/Dropdown/`
- `web/src/design-system/components/inputs/TextInput/`
- `web/src/design-system/components/inputs/Radio/`
- `web/src/design-system/components/primitives/Divider/`
- `web/src/design-system/components/layout/AppLayout/`
- `web/src/design-system/components/layout/PageTransition/`
- `web/src/design-system/components/feedback/Toast/`
- All of `web/src/components/ui/`

T143 separately requires consumer proof before deletion of optional obsolete components.
Those candidates are Spinner, Container, Stack, Grid, Breadcrumb, Pagination, Progress, StatusIndicator, and ScopeList.
This documentation review does not establish that those components have no consumers.

### Story state matrix

Every component needs every applicable state in light and dark themes:

| State | Required evidence |
| --- | --- |
| Default | Semantic structure, names, text contrast, and control boundaries |
| Hover | Appropriate feedback without pointer-only information |
| Focus-visible | Keyboard access and visible, unobscured focus |
| Disabled | Meaningful label/state and no available disabled action |
| Error | Associated error text and appropriate announcement |
| Loading | Accessible loading status without decorative announcement noise |
| Overlay-open | Accessible name, focus behavior, dismissal, focus return, and inherited theme |

**Historical T065 matrix, superseded by ADR 040 on 2026-10-06**: Each story needed a reviewed visual-regression baseline per theme. Current pixel references cover only eligible canonical light 1280 × 720 cases. Both-theme accessibility and interaction coverage remains required.
Storybook accessibility uses `parameters.a11y.test = 'error'`.
Accessibility and visual-regression failures must block CI. Baseline changes require review.
A themes toolbar alone does not provide both-theme CI evidence.

Keep the WCAG 2.1 AA floor and meet the WCAG 2.2 AA feature target.
Text requires 4.5:1 contrast. Required controls and focus indicators require 3:1 against adjacent surfaces.
Browser evidence must also cover keyboard paths, announcements, zoom, reduced motion, forced colors, and native controls.

### Self-hosted assets

The asset contract includes:

- Normal-width Zalando Sans Variable WOFF2 latin and latin-ext subsets in `web/public/fonts/`, with its license.
- Inter Variable WOFF2 latin and latin-ext subsets in `web/public/fonts/`, with its license.
- Existing JetBrains Mono files in `web/public/fonts/`.
- Outlined black and white wordmarks in `web/public/brand/`.
- The matching favicon and a compact mark derived from that artwork in `web/public/brand/`.

Preload only the Zalando Sans and Inter latin subsets. Load mono only where needed.
Do not add SemiExpanded files or remote SVG font references.
No page automatically loads third-party fonts, scripts, or images.
Unknown external logos use a local fallback. User-directed external links and OAuth2 navigation remain available.

### Stakeholder clarification

The Agents list shows agent identity, expiry, View, and confirmed Revoke.
It has no permission-set count column and adds no per-agent count requests.
`activeGrantCount` counts UserGrant records, not permission sets.
This clarification changes no API contract.
The guides must not reintroduce a count through a generic table or card example.

### Completed documentation work

| Task | Files changed | Result |
| --- | --- | --- |
| T015 | `web/src/design-system/docs/DESIGN_PRINCIPLES.md`, `INDEX.md` | Accepted ADR 038 governs current guidance. Dated historical notes preserve Refined Trust Architecture. Feature labels use 047. |
| T016 | `web/src/design-system/docs/COLOR_GUIDE.md`, `TOKEN_GUIDE.md` | One 40-role semantic OKLCH table includes exact light/dark values and every required foreground, status, risk, and sidebar role. TOKEN_GUIDE links that authority. Both guides document T046 behavior without a retired palette API. |
| T019 | `web/src/design-system/docs/ACCESSIBILITY_GUIDE.md`, `MOTION_GUIDE.md` | WCAG 2.1 AA floor, WCAG 2.2 AA target, both-theme blocking accessibility/visual checks, CSS-only 120–200ms ease-out, and no reduced-motion movement or delay. |
| T014 review portion | `local://design-review.md` | Required guides reviewed. Replacement, deletion, universal-component, state, and asset matrices ready for parent integration. |

No runtime file changed. No build, test, lint, formatter, or smoke run was performed, as instructed for this documentation slice.
No compliance or runtime-completion claim follows from these edits.

## Existing endpoint map and documentation audit

| Screen or shared function | Existing operations |
| --- | --- |
| Identity bootstrap, user menu, Settings identity | `GET /api/me` |
| Agents list | `GET /api/consent/agents`; confirmed `DELETE /api/consent/agents/{agent-id}/grants` |
| Agent decision and console detail | `GET /api/consent/agents/{agent-id}` (decision includes `session_token`); `GET` and `POST /api/consent/agents/{agent-id}/grants`; confirmed `DELETE` of that grant |
| Required-service connection | Existing `GET` or `POST /api/third-party/{serviceId}/oauth2/authorize`, followed by the existing provider callback; POST carries the bounded tab-local consent-state reference |
| Connections | `GET /api/third-party/sessions`; `GET` and `DELETE /api/third-party/{serviceId}/session`; `GET /api/third-party/{serviceId}/session/affected-agents`; `POST /api/third-party/{serviceId}/session/refresh` |
| Console sidebar and pending queue | `GET /api/approvals/pending` |
| Standing decisions | `GET /api/approvals/permanent`; `POST /api/approvals/{id}/revoke` |
| Approval review and inline decision | `GET /api/approvals/{id}`; `POST` to its `scope-preview`, `approve`, and `deny` operations |
| Command palette | Existing principal-scoped agent, connection, and pending-approval query results; no new endpoint |

All operations exist in `api/enduser/openapi.yaml`. No browser GET call targets the gateway-only `/api/approvals` operation.
The session list remains `{data: {sessions: [UserSessionSummary]}}`. Approval operations keep their existing data envelopes and principal checks.

### Consent contract review

| Operation | Source evidence | Correction |
| --- | --- | --- |
| `GET /api/me` | `user_info_handler.go:43-89; internal/domain/consent/user_info.go:7-20`; client `consent.ts:41-57` | Documented nested data, principal-as-displayName fallback, and omission rather than null for unavailable email/pictureUrl. |
| `GET /api/consent/agents` | `agents_handler.go`; `internal/domain/consent/service.go`; `web/src/types/consent.ts` | `activeGrantCount` counts active UserGrant records. PR review requires retention of optional nullable `logoUrl`; an absent stored logo does not justify deleting its contract. |
| `GET /api/consent/agents/{agent-id}` | `agent_detail_handler.go`; `web/src/services/api/consent.ts` | Preserve existing wire casing and optional logo metadata. Correct unrelated inaccurate field documentation without removing supported contract fields. |
| `GET /api/consent/agents/{agent-id}/grants` | `grants_handler.go`; `web/src/services/api/consent.ts` | PR review restores the documented array envelope. Return zero or one grant under the current user-agent uniqueness rule, not an object or null. Align handlers and callers with the contract instead of changing the contract to match implementation drift. |
| `POST /api/consent/agents/{agent-id}/grants` | `grants_handler.go:107-306; internal/domain/consent/service.go:291-396,401-545`; client `consent.ts:192-229; types/consent.ts:278-281` | Preserved existing 201 envelope with grant in data and optional sibling redirect_url. Removed incorrect POST 204/revocation response and empty-object revocation example. Empty maps/arrays return 400; request map requires at least one property. Corrected permission-set/service validation and token error documentation: expired=session_expired, invalid=invalid_token. Removed null valid_until from response examples. |
| `DELETE /api/consent/agents/{agent-id}/grants` | `grants_handler.go:309-325`; client `consent.ts:243-255` | Removed false recommendation to revoke through an empty POST. Documented DELETE as revocation operation and preserved connected-session independence. |

The YAML parser accepted OpenAPI 3.0.3, and all 142 internal references resolve. This is a structural documentation check, not a runtime contract change.

## Red phase

`just check` passed after the harness integration, using the configured `go vet` fallback because golangci-lint is unavailable.
`go test ./tests/e2e/pages ./tools/imgdiff -count=1` passed, including real Chromium instrumentation regressions.
The first full frontend run preserved all **61 existing journeys** but exposed duplicate permission-set names in the new fixture.
The next focused run exposed the backend 255-character agent-name limit. Neither setup failure is counted as semantic-red evidence.

After those fixture repairs, this command compiled and executed every new scenario:

```sh
E2E_FRONTEND_MODE=built /Users/brennenstuhl/go/bin/ginkgo -v --procs=2 --output-interceptor-mode=none --focus="Consent UI v2" ./tests/e2e/frontend/
```

Result: **17 failures in It assertions, 0 passing new scenarios, 0 pending**. The 61 existing cases were excluded by the focus filter, not disabled in source.
No runtime redesign source had changed. The failures identify absent presentation or interactions, not setup or compilation errors.

| Scenario | First missing observation |
| --- | --- |
| AS-01 | getByTestId('agent-origin-label') |
| AS-02 | getByTestId('permission-groups') |
| AS-03 | getByRole('radiogroup', { name: 'Access duration', exact: true }).getByRole('radio', { name: 'Custom date', exact: true }) |
| AS-04 | getByTestId('approval-tool-name') |
| AS-05 | getByRole('button', { name: 'Approve once', exact: true }) |
| AS-06 | getByRole('heading', { name: 'Agents', exact: true }) to be visible |
| AS-07 | getByRole('heading', { name: 'Agents', exact: true }) to be visible |
| AS-08 | Expected       <bool>: false   to be true |
| AS-09 | getByRole('button', { name: 'Agent actions', exact: true }) |
| AS-10 | getByTestId('connections-table') |
| AS-11 | getByTestId('connection-row').filter({ has: getByText('Expired connection', { exact: true }) }).getByRole('button', { name: 'Reconnect', exact: true }) |
| AS-12 | Expected       <[]string / len:0, cap:0>: []   to equal       <[]string / len:3, cap:3>: [           "Pending requests",           "Standing allow decisions",           "Standing deny decisions",       ] |
| AS-13 | getByTestId('pending-approvals') |
| AS-14 | getByRole('heading', { name: 'Agents', exact: true }) to be visible |
| AS-15 | Expected       <string>:    to equal       <string>: light |
| AS-17 | getByRole('heading', { name: 'Settings', exact: true }) to be visible |
| AS-18 | getByRole('heading', { name: 'Agents', exact: true }) to be visible |

### Fixture boundaries

The backend accepts agent names of at most 255 characters. The 300-character escaped-metadata case uses a genuine authenticated detail response with only its display name replaced at the browser boundary. Stored metadata remains valid and unchanged.
AS-03 uses a genuinely issued authorization token and injects the documented session_expired response to exercise its terminal UI error. It does not claim natural backend TTL-expiry proof.
CIMD flows use the existing TLS fake-host bootstrap and real authorize flow. Re-consent temporarily expires a provider session to trigger real authorization, then restores it before decision assertions.
Both-theme captures use fresh authenticated contexts and never reload or mutate the caller. Focus instrumentation rejects transparent ancestors and decorative shadows, traverses roving controls, and restores local selection.

The final full `just test-e2e-frontend` run executed all78 scenarios together: **61 passed, 17 failed in the new It assertions, 0 pending, 0 skipped**. This completes T042 before foundation runtime changes.

## Foundation verification

- Exact dependency pins install without peer errors. `npm --prefix web ls --depth=0` succeeds. Versions respect the repository's seven-day release-age policy.
- Semantic-token contrast tests cover 206 cases; outlined asset tests cover six cases. The actual black and white wordmarks were inspected in Chromium against their intended backgrounds.
- `just docs-build` succeeds with the shared local artwork. No browser asset depends on a remote font.
- Theme and pure-state model tests pass. The first theme-unit attempts were blocked by the Node 26/jsdom storage harness, so those attempts are not semantic-red evidence. The earlier real frontend theme scenarios did fail before runtime implementation. The corrected theme/model batch passed 65 tests.
- CSP tests first failed, then passed. `just check` and the full fast `just test` suite passed after the Go handler change. The lint recipe used its documented `go vet` fallback because `golangci-lint` was unavailable.
- A real Chromium smoke served the SPA handler with a valid script hash and observed the dark first-paint theme and native color scheme. With the manifest missing, the script did not execute. Temporary server code was removed.
- A broad foundation run passed 42 test files. The remaining route test failures were in the old page consumers; after route cutover, all nine App context-isolation cases passed.
- Query, mutation, and model tests cover principal isolation, cancellation, concurrent record rollback, exact prior validity, callback drafts, and connection-state precedence. An additional regression proved that an old removed mutation cannot settle a newer same-record request.
- `just web-storybook-build` succeeds. Native Chromium keyboard interaction selected the next enabled Radix radio and skipped the disabled option.

### Accessibility and visual gate proof

Both Chromium theme projects passed all 18 Button stories. A temporary low-contrast story with `#bbb` text on `#fff` failed both projects with the actual axe `color-contrast` violation (1.91:1, required 4.5:1). A separate temporary square-corner change to the existing Button story failed both screenshot comparisons. The probe and style change were removed.

The proof also exposed two test-infrastructure issues. Concurrent theme projects shared Storybook's optimizer cache; they now use separate caches. Screenshot directory normalization duplicated the workspace path; the resolver now uses the resolved project directory and preserves the Storybook component/story separator. A fresh 18-story run passed and produced separate candidates in `web/.storybook/candidates/`.

These local macOS captures are candidates, not approved Linux baselines. T066 remains open until the Linux artifact is reviewed by a human and the approved images are published. Comparison recipes never update reviewed baselines.

## Approved legacy-journey clarification

On 2026-09-28, the stakeholder selected **Use the redesigned behavior** after reviewing the conflict between FR-008/AS-08 and old raw-scope/initial-Save assertions.

- Replace obsolete raw-scope presentation assertions with permission-group behavior coverage.
- Verify that Save is absent until an edit. Make an explicit edit before saving in the CSRF journey, without weakening its security or storage assertions.
- Move embedded selectors out of legacy test bodies into page objects.
- Migrate callback fixture encoding and decoding to the canonical `{selections, duration, customDate}` envelope while preserving exact selection assertions.
- Keep authorization, acting-user isolation, server mutations, safe redirects, provider callbacks, and revocation scopes unchanged.

The decision is recorded in `spec.md`, `plan.md`, FR-029, SC-009, T091, and T114. It does not authorize a compatibility path or backend change.

## Implementation resume — 2026-10-02

The prerequisite script initially had no saved feature context. The active branch identified feature 047, and the explicit `SPECIFY_FEATURE_DIRECTORY` override resolved all required artifacts. The read-only requirements checklist passed: 16 checked, 0 unchecked.

### Build-source recovery

The first `just web-storybook-build` failed because all three plugins imported by `web/vite.config.ts` were absent. `web/build/` had been excluded by both repository and frontend `build/` ignore rules. No copy was present in the available worktrees.

Restored `themeInitPlugin.ts`, `assetCompressionPlugin.ts`, `decisionModulesPlugin.ts`, their theme/compression integration tests, and `decisionBundle.test.ts`. Repository and Docker ignore rules now explicitly retain `web/build/`; the frontend ignore file no longer excludes that source directory. The repository also ignores general `*.log` artifacts.

Observed verification:

- Locked frontend dependency installation succeeded.
- The first integration-test run exposed macOS `/var` versus `/private/var` fixture paths. Canonicalizing temporary roots fixed the fixture; the corrected run passed all 3 tests across 2 files.
- The build-source TypeScript project passes `tsc -p web/tsconfig.node.json --noEmit` with its target aligned to Node's ES2022 support.
- `just web-test` passed all 667 tests across 70 files, including the recovered build regressions.
- `just web-storybook-build` passed after source recovery.
- `just web-bundle-check` passed both route checks. The production consent graph measured 164,039 bytes gzip across 16 static JavaScript/CSS assets; approval measured 155,595 bytes across 14. Both are below 170,000 bytes and contain no rendered Table or Command modules.
- Module-inventory IDs are relative to the project root rather than exposing the checkout path; the production build and both bundle checks pass with that normalization.
- A temporary server using the real `SPAHandler` served the production build. Chromium observed Dark and `color-scheme: dark`, no CSP violations, a script-hash CSP, and Brotli HTML delivery. Both gzip and Brotli wordmark responses decoded to SVG.
- A real Storybook browser session changed ThemeChoice from Light to Dark and observed the selected radio and resolved document theme. Temporary smoke-server source and processes were removed.
- `just web-storybook-visual-candidates` passed all 432 stories across 68 light/dark project files. These macOS captures are unreviewed candidates, not Linux baselines.
- Two Linux amd64 container attempts exited 137 before reporting any story results; the second serialized test files and limited the Node heap. Neither produced candidate images or establishes an accessibility failure.
- Native Ubuntu Noble arm64 capture with Playwright 1.62.1 passed 216 Light and 216 Dark tests (34 files per project). Separate, unreviewed Linux candidates are in `web/.storybook/candidates/linux-arm64/`. The amd64 CI renderer has not been validated against them.
- `ripwire --quality-delta` could not index the build-source path in the repository-root scan, and the source-directory scan had no Git baseline. A scoped structural/lexical quality panel reported no ranked findings; this is not a clean quality-delta claim.
- Container dependency installation reported one high-severity advisory. No automatic dependency upgrade was applied.

The user explicitly authorized deferring T066's human visual review while continuing the remaining implementation tasks. This changes execution order, not release requirements: approved Linux baselines and comparison on the amd64 CI renderer remain mandatory before completion.


### Journey migration and accessibility

T091, T099, T106, T114, T121, and T129 now use the current decision, table, overflow, save-bar, and confirmation controls. Canonical consent-state journeys retain exact group/service selections and also cover duration and custom-date preservation. The CSRF journey makes a deliberate edit before saving and retains its storage and safe-redirect assertions. Canonical acceptance assertions were not weakened.

Additional requirement fixes:

| File | Fix and evidence |
| --- | --- |
| `web/src/components/sessions/DisconnectDialog.tsx` | Long provider descriptions use the existing two-line TruncatedText control with accessible expansion. Dependency warnings and provider-token text remain visible. |
| `web/src/pages/ConnectionsPage.tsx` | Focus returning after removal of the trigger row has a visible semantic ring on the page fallback. |
| `web/src/components/approvals/PermanentDenialDialog.tsx` | Long tool/agent context uses TruncatedText; the permanent-denial effect stays separately visible. At 320 px, real Chromium measured 286 px dialog width and 2,089 px content width before repair; afterward both widths were 286 px, including after expanding the full names. The temporary reproduction story was removed. |
| `tests/e2e/pages/page.go` | Legacy captures failed with `font Crimson Pro unavailable` in the real CIMD journey. Readiness now checks the CSS-declared Zalando Sans Variable, Inter Variable, and JetBrains Mono Variable families without suppressing unavailable-font errors. |

After the application-dialog fixes, `just web-lint` passed and `just web-test` passed all 667 tests across 70 files. AS-01 passed with themed capture enabled before the legacy readiness repair; it uses a separate themed-capture path. The full migrated frontend capture run is the subsequent green checkpoint.

`just check` uncovered a second ignored-source omission: `tools/imgdiff/main.go` called an absent `runGate`. The executable ignore rule is now root-scoped (`/imgdiff`) instead of excluding the source directory. The gate and its filesystem/PNG regressions are restored; available images become review artifacts, never automatically accepted baselines.


### Stable list reads and pre-integration checkpoint

The first complete migrated capture run passed 74 of 75 journeys. AS-15 exposed a real focus problem: tied-timestamp pending records changed order on refresh and moved a focused control out of the viewport. A later run exposed unordered session reads as the same problem on Connections. PostgreSQL already orders sessions by creation time; the memory repository iterates its map without an order.

The shared pending, connection, and standing queries now keep newest-first order, with a displayed-name and ID tie-breaker. No backend endpoint, response, token, or persistence behavior changed. Three regressions failed semantically before their fixes and then passed; the complete targeted hook batch passed 20 tests. Temporary focus diagnostics and source scaffolds were removed.

The final pre-integration run executed all 75 frontend journeys with capture enabled: 75 passed, 0 failed, 0 pending, 0 skipped. This includes all 17 active acceptance scenarios and the preserved security, storage, and callback journeys. The frontend unit gate passed 670 tests across 70 files. Frontend lint, both decision-bundle cases, and the updated Docusaurus build passed.

The quality-delta report flagged locator/error-handling clone matches, including semantically unrelated browser and PNG helpers, plus complexity in the new image gate. Those findings were examined without refactoring code merely to satisfy the heuristic. The report is not recorded as clean.

### Performance

The actual broker binary and frontend were built with `just build-all`. A temporary harness launched that binary with real memory encryption, signed approval credentials, isolated loopback providers, and authenticated HTTP seeding.

Chromium measured cold consent content at **4,340.4 ms**. It used Chrome DevTools' current Slow 4G constants: 562.5 ms latency, 180,000 bytes/s download, and 84,375 bytes/s upload. The preset values came from `ChromeDevTools/devtools-frontend/front_end/core/sdk/NetworkManager.ts`. The browser cache was empty and disabled. A genuine `/oauth2/authorize` redirect supplied the authorization session before the measured consent navigation.

The HTML response used Brotli and public module-preload hints. The browser observed no CSP violations and no automatic cross-origin requests. This satisfies the 5-second measurement on this pre-integration build; no claim is made about moderated decision-time studies.

### Newly approved upstream integration

The user approved integrating current `main` and migrating to its existing consent-state form POST and tab-local storage flow. The user also approved documentation-only corrections for existing session-detail/termination responses and approval error statuses. Neither approval permits new API behavior or compatibility fallbacks.

The expired-grant draft defect and optional-field/error-contract mismatches are corrected. The six initially failing expired-grant assertions now pass; all 24 draft tests and the 16 integrated route/error regressions pass. The earlier implementation's test-first chronology cannot be reconstructed from its combined implementation commit; historical assertions remain unverified rather than inferred from task checkmarks.

## Integrated verification — 2026-10-03

### Automated gates

| Check | Observed result |
| --- | --- |
| `just check` | Passed: vet and lint, 0 issues |
| `just web-test` and `just web-lint` | 710 tests in 72 files passed; lint passed |
| `just test` | Fast Go suite passed with race detection |
| `just test-integration` | Self-contained suites passed; integration sources unchanged |
| `just cdk-test mock-sample-agent-test mock-upstream-oauth2-test test-integration-infra` | All four recipes passed, including real PostgreSQL and AWS-emulator coverage |
| `just test-e2e` | Backend: 641 passed, 2 performance-labelled cases excluded; ExtProc: 112 passed; frontend: 77 passed, 0 pending or skipped |
| `just web-storybook-build` | Production component documentation built |
| `just web-storybook-visual-candidates` | 432 light/dark stories in 68 project files passed accessibility and generated separate macOS review candidates |
| Production decision-bundle gate | Both final route cases passed; consent 165,993 bytes gzip across 16 assets; approval 157,556 bytes across 14, each below 170,000 bytes |
| End-user OpenAPI structural check | OpenAPI 3.0.3 parsed; 154 internal references, none unresolved |

The full frontend capture run also passed all 77 journeys serially and produced all 14 required route/context/theme images. Captures remain candidates under `tests/e2e/frontend/coverage/screenshots/`; no image was accepted automatically.

The integrated native Ubuntu Noble arm64 capture, using the pinned Playwright 1.62.1 browser image and the compiled Go suite, passed **77/77 journeys** serially and produced Linux candidates under `tests/e2e/frontend/coverage/screenshots/linux-arm64/`. It generated all 14 required route/context/theme images plus retained-journey captures. The first invocation failed before running any tests because the test binary does not accept the CLI-only `ginkgo.procs` option; the corrected serial invocation passed. These are not reviewed baselines and do not establish amd64 renderer compatibility.

### Removal and boundary proof

Searches over `web/src/` and `web/index.html` returned no Headless UI, Framer Motion, separate API-cache, old UI-component, PageTransition, TypeScript token-import, retired-font, gradient, or raw Tailwind-palette references. The surviving stylesheet import is the intended semantic `tokens/theme.css`. `npm --prefix web ls @headlessui/react framer-motion` returned an empty dependency tree; its exit 1 means neither requested package is installed.

The optional obsolete component search (`primitives/Spinner|layout/(Container|Stack|Grid)|navigation/(Breadcrumb|Pagination)|advanced/Progress|data-display/(StatusIndicator|ScopeList)`) returned zero consumers. Those components, the named superseded pages/layout/consent/session/hooks, retired token modules, and unused font files are absent. Every remaining JetBrains Mono subset is referenced by `fonts.css`.

No `ui.v2` or feature-flag match remains in runtime source. Required `git diff main` boundary checks are empty for configuration examples, charts, migrations, storage ports, configuration ports, integration tests, and PostgreSQL adapters. The only backend runtime changes against main are the SPA handler and its tests. API operations, authorization, server configuration, and persistence are unchanged. The six glossary entries, ADR 038 acceptance/cross-references, component locations, 17-scenario hierarchy, page-object selector boundary, and existing-response wire types were checked.

### Production-browser evidence

The integrated production broker served all seven route/context surfaces in Light and Dark. Chromium observed the expected decision versus console shells, matching native color schemes, no page errors or console errors, no CSP violations, and no automatic cross-origin requests. Reduced-motion emulation showed no active movement. Changing theme and collapsing the sidebar survived reload; localStorage contained only `aib.theme` and `aib.sidebar-collapsed`.

A real provider authorization used exactly the POST fields `redirect_uri` and `consent_state_id`, with a clean return path and an opaque UUID. Its existing callback returned HTTP 302. The browser restored the optional calendar selection, custom date `2099-11-06`, authorization-session token, original query and fragment, and decision context; it removed the state ID from the URL. No grant or approval was submitted by this smoke.

Cold consent main content appeared at **4,737.7 ms** with cache disabled under the same documented Slow 4G constants. HTML used Brotli and public module-preload hints; there were no CSP violations or automatic cross-origin requests. Production GET and HEAD with the same identity encoding returned matching representation/security headers and an empty HEAD body.

### Final-gate repairs and unresolved security dependency

The first `just verify` stopped on five G304 filesystem accesses and the image gate's directory permissions. Root-scoped Go filesystem APIs now reject escaping manifest/artifact symlinks; output directories use 0750 and new artifacts use 0600. Both new regressions failed before implementation and pass afterward. The full image-gate and SPA-handler packages pass. `gosec` subsequently reported **0 issues** without a new suppression.

The upstream root-routing acceptance test pinned the old whole CSP and compared representations negotiated differently by Go's automatic GET gzip handling. It now checks required CSP restrictions and GET/HEAD parity with matching `Accept-Encoding`, and includes `/settings`. The focused routing cases and complete E2E suite pass without weakening authorization assertions.

The next `just verify` stopped at `govulncheck`: **GO-2026-6443 / CVE-2026-84445**, missing-authority/Host panic in gRPC's xDS routing. The repository requires `google.golang.org/grpc v1.84.0`; the version proxy also reports v1.84.0 as latest stable. The advisory identifies `v1.85.0-dev.0.20260825072537-93e31b48545e` as the fixed version for this release line. No dependency upgrade, scanner exclusion, or security bypass was applied. T208 remains unchecked pending an explicitly approved dependency change and a complete final-gate run.

The stakeholder selected **Keep the dependency unchanged** in the security-scope decision on 2026-10-03. gRPC remains at v1.84.0; T208 and release stay blocked. No dependency change or scanner bypass is authorized.

The final feature-scoped quality delta reported 20 gating clone findings, including unrelated browser/JWK and snapshot/storage pairs, straightforward typed API transports, and minor complexity/verbosity changes. No blanket acknowledgement or metric-driven abstraction was added. This is not recorded as a clean quality-delta result.

### Remaining release evidence

- T066/T149 and their dependent visual/documentation gates require human-reviewed Linux route and component baselines, comparison on the amd64 CI renderer, and publication of README screenshots. Existing macOS and Linux arm64 images are candidates only.
- T155/T156 require the endpoint map and stakeholder/ADR references in an actual PR. No feature-047 PR was found among the repository's open PRs.
- Moderated SC-001/SC-007 studies are unavailable. Decision time, service comprehension, and primary-action recognition must be listed explicitly as unavailable in the PR; automated timing/accessibility results do not establish them.
- T179/T180/T181/T186/T187 remain unverified: available history contains a combined implementation commit, not per-task chronological evidence. The written T042 red record is preserved and reviewed, but its original run artifact and red-time source snapshot are unavailable.
- T182/T195/T197/T200/T207 are not claimed complete while reviewed visual comparisons and manual acceptance evidence are missing.


### Final disconnect focus regression

The disconnect coverage audit found that cancellation focused the page instead of the remaining row action: a table rerender detached the saved DOM element. The regression failed before the fix. `ConnectionsTable.tsx` now gives each Disconnect control a stable session identity, and `ConnectionsPage.tsx` resolves the current control when returning focus. A missing or disabled control still falls back to the page. Permanent tests cover both cancellation and successful row removal.

All 15 ConnectionsPage cases pass; the full frontend gate still passes 710 tests, lint passes, and both final bundle cases pass. A throwaway production-bootstrap Chromium smoke exercised Escape cancellation and confirmed removal in both Light and Dark, observing restored Disconnect focus, retained session on cancellation, visible page focus after removal, and actual server-side deletion on confirmation. Both browser cases passed. The temporary smoke source, broker harness, compiled capture binaries, and other owned scaffolds were removed; review images were retained.

The production timing and route/callback measurements above preceded this console-only focus fix. The final compressed decision graphs remain below the limit. No new cold-timing result is inferred from their sizes.

The final post-fix `just check` passed with 0 lint issues, `just test-e2e-frontend` passed all 77 preserved journeys, and `just docs-build` built the corrected end-user documentation. The stakeholder initially selected **Keep local** for publication, leaving changes uncommitted and unpublished. The subsequent merge-commit authorization below supersedes only the uncommitted status; PR-specific tasks remain open. The endpoint map and acceptance references above are ready for a future PR, which must also state the unavailable moderated-study and historical test-first evidence.

### Local merge commit authorization

The stakeholder subsequently requested completion of the active merge and commitment of all necessary changes. This authorizes a local merge commit and supersedes the earlier instruction to leave changes uncommitted. It does not authorize a push, PR publication, dependency upgrade, scanner bypass, or automatic acceptance of visual baselines. The merge must include the upstream callback transport and its frontend consumers together to keep the integration consistent.

## Convergence fixes — 2026-10-03

T211, T214, and T218 are complete. The requirements checklist still has 16 checked items and no unchecked items. Its markers did not change.
Project setup inspection found the existing Git, Docker, ESLint, Prettier, and Helm exclusions sufficient for this work. The SPA is not an npm publication. No ignore file, dependency, runtime API, backend, or persistence change was needed.

### Test-first evidence

| Task | Command before implementation | Observed failure |
| --- | --- | --- |
| T211 | `npm --prefix web test -- --run src/components/sessions/connectionState.test.ts src/pages/AgentConsolePage.test.tsx` | Six failures: stored sessions incorrectly returned No connection instead of Connected, Expired, or Needs re-authentication; the corresponding console labels were absent. |
| T214 | `npm --prefix web test -- --run src/pages/ApprovalsPage.test.tsx` | Two failures: open Approve and Deny confirmations did not update at the request deadline. Neither row exposed the expired status after clock advancement. |
| T218 | `npm --prefix web test -- --run src/components/approvals/ApprovalReviewPage.test.tsx src/pages/ApprovalsPage.test.tsx -t 'bounds unbroken'` | Three failures: approved and denied outcomes and the revoke dialog had no accessible Show more control for long names. Other tests were command-filtered, not marked skipped in source. |

The first T211 green attempt exposed an incorrect label in the new test: Manage connections instead of the existing Manage connection. The new assertion now uses the actual action label. No existing acceptance assertion was weakened.
After integration, `just web-test web-lint web-bundle-check` passed: 720 tests in 72 files, no lint errors, a production build, and both decision-graph budget/import-isolation tests.

### Production-browser smoke

A throwaway Ginkgo harness used the existing production bootstrap, real stored sessions, real approval records, and Chromium. Both Light and Dark runs passed at 320 px.

- An agent requiring providers with expired and refreshable stored sessions displayed Expired and Needs re-authentication, never No connection.
- A real pending approval expired with its inline confirmation open. The row announced expiry and disabled the decision and confirmation without another pending-list request or a mutation.
- A resolved approval and a standing-decision revoke dialog displayed a long tool name and markup-containing agent name as text. Enter expanded the context. Neither the page nor the dialog overflowed, and the supplied script did not execute.
- Cancelling the revoke dialog submitted no decision. The smoke did not grant, approve, or revoke access.

The first smoke attempt rejected markup in the stored tool pattern before reaching the text checks. The corrected fixture used a valid long tool name and retained the adversarial agent name. This was a fixture error, not an application failure. The complete corrected smoke passed 2 of 2 selected cases; the temporary Go source was removed.

### Quality and remaining release requirements

The scoped quality delta reported two major findings: the expiry lifecycle increased InlineApprovalActions complexity from 14 to 16, and the bounded revoke context increased ApprovalsPage size from 57 to 63 lines. Both changes implement required behavior with existing components. No metric-driven abstraction or blanket acknowledgement was added. This is not a clean quality-delta claim.

The remaining 27 tasks stay unchecked. Human-reviewed Linux component and route baselines and amd64 comparisons are still unavailable. README screenshot publication depends on those approvals. Manual keyboard, overlay-focus, and actual 200% browser-zoom evidence remains incomplete.
Original historical red artifacts and source snapshots for T179, T180, T181, T186, and T187 remain unavailable. This session's new regression failures do not establish the earlier implementation's chronology.
PR publication remains unauthorized, so endpoint-map attachments and acceptance references are not claimed published. Moderated SC-001 and SC-007 results remain unavailable.
The recorded decision to keep gRPC unchanged still applies. T208 and T213 remain blocked by GO-2026-6443 / CVE-2026-84445. No scanner bypass, dependency change, baseline approval, or publication authorization was inferred. `just verify` was not rerun merely to confirm the known blocker.

### Final regression results

| Command | Observed result |
| --- | --- |
| `just check` | Passed formatting checks, vet, and lint with 0 issues. |
| `just test-e2e-frontend` | All 77 journeys passed, with 0 failed, pending, or skipped cases. |
| `just docs-build` | Generated the updated production documentation. Existing CSS-minifier and Node storage warnings did not stop the build. |

These results follow removal of the temporary browser-smoke source. They do not replace the blocked reviewed-visual, historical, manual, PR, or final-security gates.

## Manual review preparation — 2026-10-03

The stakeholder requested a screenshot review package and instructions for a local UI run. Screenshot approval and manual browser checks remain separate from automated checks. No candidate image is an approved baseline until the stakeholder reviews it.

### User study

The stakeholder confirmed that no dedicated user study will occur. SC-001 decision time and service comprehension, and SC-007 participant action recognition, remain unmeasured. The PR must disclose that these study results are unavailable. Automated checks do not establish those results.

### Separate security work

The stakeholder directs that GO-2026-6443 / CVE-2026-84445 is separate remediation work and must not block this UI release. This supersedes the earlier feature-specific instruction to wait for dependency remediation. The gRPC dependency and all security checks remain unchanged. The known `just verify` security result is still a failure, not a passing or waived scanner result. No dependency upgrade, scanner exclusion, or security-control bypass is part of this work.

### Manual PR creation and main baseline

The stakeholder will create the PR manually. No push or automated PR creation is authorized. `git fetch origin main` succeeded, and `git rev-list --left-right --count HEAD...origin/main` reported 11 commits ahead and 0 behind at `792edd32`. The branch already contains current `origin/main`; no merge or rebase was needed.

The PR evidence is in this inventory: T010's existing endpoint map, the approved documentation-only response corrections, and the acceptance reference in `adrs/038-design-system-rebuilt-on-shadcn-radix.md`. Original historical test-first evidence remains unavailable. The PR must disclose that limitation and the unavailable user-study results.

### Screenshot review package

The current frontend suite compiled from runtime revision `792edd32` and ran in the pinned Playwright 1.62.1 Noble Linux arm64 image. All 77 journeys passed, with 0 failed, pending, or skipped cases. The run produced 14 current route/context/theme images and 27 current state images under `tmp/consent-ui-linux-current/`.

The offline gallery is `tmp/consent-ui-review/index.html`; its archive is `tmp/consent-ui-review.zip`. It includes those 41 current Linux images, 432 prior Linux component candidates, and 44 prior macOS images for comparison. Each source and its capture status appear in `source-files.json`. The component and macOS capture revisions are unrecorded, not inferred from file dates.

Chromium opened the gallery through `file://` and HTTP. All 517 images loaded. Filename search showed the two Settings views, and selecting the consent image opened its full-resolution PNG. The local HTTP archive download returned a 9,119,458-byte ZIP. No baseline directory or visual manifest changed. The temporary gallery generator and compiled capture binary were removed.

These are Linux arm64 review candidates. This run does not establish the amd64 CI renderer comparison or human approval. The screenshot review service listens only on `127.0.0.1:8766`.

### Local manual-review runtime

The `consent-ui-review` Compose project runs five isolated services: broker, Sample Agent, upstream OAuth2, third-party OAuth2, and CIMD mock. Its container names, images, network, secrets, and volume names are separate from the original `aib-*` resources. The ignored override preserves the seed script's provider DNS alias. Secrets remain in a mode-0600 file under `tmp/` and are not part of the screenshot archive.

The native broker configuration did not enable approval authentication, so the approval helper returned 503 there. The supported Docker configuration enables that existing path. An old development image used Go 1.26.8 and could not compile the Go 1.27.1 module. Reusing a verified Go 1.27.1 development image addressed the toolchain mismatch, but the broker's in-container compiler then exited with `signal: killed`. The current host-built Linux arm64 broker is mounted into the isolated container and runs through the existing startup/seed script. No runtime source, dependency, scanner, or VM setting changed.

The current broker, native mocks, Sample Agent, and frontend built successfully. The three isolated mock images also built from current source. `just build-linux-arm64` produced the broker used by the review container. The five review containers became healthy. Vite serves the application at `http://localhost:3000/`; Storybook serves components at `http://localhost:6006/`.

The real proxy-client flow reached consent, connected the mock provider, preserved the 30-day choice through its callback, saved the grant, and completed upstream authorization for `dev@example.com`. The grant appears on Agents and its console detail. Connections shows the mock session as Connected with four scopes. The existing approval helper created a real pending `send_finance_report` request and printed a working review URL. Approval links expire after ten minutes.

Parent Chromium smoke observed those console rows and the pending review's acting user and Approve once action. A real Local Client authorization opened DecisionShell with Allow and Deny and no navigation. The agent Connections tab showed Connected for the stored mock session and No connection for the two unconnected example providers. Dark persisted after a Settings reload. The live Storybook Button rendered as Review access in Light and Dark. No page or console errors occurred during these observations. The smoke submitted no grant or approval decision.

An active Sample Agent grant causes another proxy login to skip consent. The quickstart explains how to revoke that local test grant and disconnect the mock session before repeating first-time consent and the provider callback. It also contains the exercised startup, stop, restart, and fresh-approval commands. The local services remain available for manual review. Original `aib-*` containers were not started, removed, or recreated.

## Approved UX amendment — 2026-10-05

This section records the 2026-10-04 UX amendment. Earlier verification tables describe the September implementation, not this amended presentation.

The source uses the seven destination routes and both agent-detail contexts. It removes the old collection tables, detail tabs, overflow revoke menu, and inline approval decisions. Shared permission and duration controls serve both agent contexts. Console-only Motion provides icon and collection transitions. Decision routes retain CSS-only motion.

### Runtime findings and corrections

- The custom-date page object could close a picker that the duration choice had already opened. It now observes the expanded state and waits for the input.
- Connection scope controls were behind a stretched entity surface. Noninteractive entities no longer stretch a pseudo-element over their controls.
- Busy agent entities retain their visible name, expiry, and recency without an active link. Collection observations use that identity and exclude inert exit-animation records.
- A 2 px inbox difference was viewport scrolling, not reflow. The selected `read_roadmap` button ended at 722 px in a 720 px viewport. Chromium scrolled 2 px to expose it. Exact list and row comparisons now use document coordinates. Width, height, and document position still must remain unchanged.
- A tooltip covered the focused skip link during narrow-screen keyboard traversal. Skip links now appear above overlay layers. Entity navigation and selection controls carry their own inset focus ring.
- Storybook exposed unnamed popovers, skipped heading levels, and insufficient contrast from busy-entity opacity. Popovers have names, heading levels follow their section, and busy identity text keeps its contrast.
- A closed tooltip still faded over Back to Agents and its positioning wrapper intercepted the focus hit test. Closed tooltip content now becomes invisible immediately, and the wrapper does not intercept input. Open content remains hoverable.
- Lazy overlays require readiness and exit observations. Stories wait for real mounted controls, visible content, dismissal, and focus return. Radio stories retain keydown until Radix completes its focus-driven selection.
- Live reduced-motion changes were not reactive in the library hook. A real hover smoke observed 16 changing icon frames after enabling the preference. The AS-15 regression failed with 16 instead of 1 before the fix. A shared `useSyncExternalStore` preference now stops icons and removes collection movement without reloading. AS-15 verifies normal motion, a live stop, and resumption in both themes.

### Screenshot and renderer contract

The route gate requires these eight stems in both themes: `agents`, `agent_consent`, `agent_detail`, `connections`, `approvals`, `remembered_approvals`, `approval_review`, and `settings`.

Storybook covers light and dark at 375 × 812, 768 × 1024, and 1280 × 720. Project-local `fileParallelism: false` prevents concurrent capture within a project. The recipes run projects serially. The pinned browser uses full Chromium headless mode (`channel: chromium`), not the separate headless-shell executable.

Candidate generation writes private review images. It does not approve them or overwrite reviewed baselines. Human approval still precedes baseline publication, README image replacement, and non-updating visual comparisons. The recorded dependency-security and study limitations remain unchanged.

### Final automated results and review package

| Check | Observed result |
| --- | --- |
| `just web-test` | 710 tests passed in 77 files. |
| `just web-lint` | Passed. |
| `just check` | Formatting, vet, and lint passed with 0 issues. |
| `go test ./tests/e2e/pages/...` | Passed, including document-space geometry that detects reflow but ignores viewport scrolling. |
| `go test ./internal/adapters/http/oauth2_sessions/...` | Passed. |
| Production frontend browser suite after the motion fix | All 77 journeys passed, with 0 failed, pending, or skipped cases. |
| Decision bundle gate | Both initial graphs remain below 170,000 gzip bytes. The forbidden-module traversal also covers lazy decision-route chunks. |
| `just web-storybook-build` | Passed. |
| Six Linux Chromium candidate projects | 354 stories passed in each theme/viewport project: 2,124 cases across 282 project files. Accessibility failures are fatal. |
| Post-motion Linux comparison against private candidates | Agents, Connections, and Approvals stories passed 76 cases in both desktop themes without updating candidates. These are consistency checks, not reviewed-baseline approval. |
| Linux production route capture | Generated the 16 current route/context/theme images and 27 state images. The full capture run passed 75 journeys and exposed two observation races. After correction, the large callback and AS-15 passed 2/2 focused Linux cases. No single 77/77 Linux run is claimed for that capture. |

The callback observation now waits for the rendered agent before checking URL cleanup. Keyboard observations wait for fonts and retain semantic identity across a lazy tooltip remount. Neither correction relaxes the authorization, draft, visible-focus, or action-set assertions.

The private review gallery is `http://127.0.0.1:6036/`. All 16 route images loaded in an actual browser smoke. Route files are in `tmp/aib-ux-linux-route-candidates/coverage/screenshots/`; component candidates are in `web/.storybook/candidates/`. The gallery also exposes current state and component captures. They remain unapproved.

The local native Linux renderer is arm64. Both headless-shell and full Chromium exited before rendering under local amd64 emulation. The amd64 CI comparison remains unavailable locally. Human-reviewed baselines, README image publication, and the non-updating visual gates remain open. No old 14-image set is accepted as the current matrix.

The quality delta still reports 25 major findings against the starting revision. The review removed unused view props/refs and redundant remembered filtering. No blanket acknowledgement or metric-driven security/UI abstraction was added. This is not a clean quality-delta result.

Original per-task red chronology is not inferred from passing tests. The amendment's source and current coverage are implemented, but unchecked test-first markers retain that evidence requirement. The live reduced-motion red/green result is recorded separately above.

`just verify` was not rerun to reconfirm the recorded gRPC advisory. The dependency and scanner remain unchanged. No commit, push, PR, or automatic baseline approval was made for this amendment.

The stakeholder requested tighter consent spacing, consistent agent-detail checkbox sizing, and loaded Connections, Approvals, and Remembered review images. Baselines remain unapproved.

### Visual feedback corrections

Desktop consent now follows its content height while retaining the viewport-safe cap and pinned mobile footer. The permission panel-to-footer gap measured 12 px in the actual browser, down from the failing 81 px story measurement. Permission-group and service controls both use the shared 24 × 24 px checkbox and indicator.

The themed capture readiness check now requires a mounted application main area and no loading status before capture. The previous check could pass on the first themed HTML frame before React mounted. A rendered regression reproduced loading-only captures for all three console routes in both themes, then passed after the readiness fix. Intentional loading-state screenshots remain separate artifacts.

The corrected consent, permission-panel, and agent-detail stories passed 198 cases across both themes and all three widths. The page-object package, 710 frontend unit tests, frontend lint, and static checks passed. Fresh Linux route and component candidates replace the stale review images; they are not approved baselines.

The fresh Linux AS-01, AS-08, AS-10, and AS-12 run passed 4/4 cases and regenerated both-theme images for all five affected views. Visual inspection confirmed five loaded connection cards, seven pending requests with their detail panel, and four remembered decisions rather than skeletons. The corrected Linux component captures passed 198 cases serially across the six theme/viewport projects. Both decision graph gates passed, and all 16 route gallery images loaded. Screenshot baselines remain unapproved.

### Commit-time verification — 2026-10-05

The commit review ran the current working-tree checks without approving screenshot baselines:

- `just check` passed formatting, vet, and lint with 0 issues.
- `just web-test` passed 715 tests in 78 files. `just web-lint` passed.
- `just web-bundle-check` passed both decision graph checks, including deferred-import isolation. `just web-storybook-build` passed.
- The OAuth2 session adapter and E2E page-object package tests passed. The focused session API integration tests passed.
- `just test-e2e-frontend` passed all 77 production-browser journeys, with no failed, pending, or skipped cases.
- The screenshot-enabled AS-01, AS-08, AS-10, and AS-12 run passed all four journeys. It exercised themed capture readiness without updating reviewed baselines.
- A separate Chromium smoke used the current-source broker with fresh in-memory storage. It showed Agents, Connections, Pending, Remembered, and Appearance. Dark and the default List view survived reload after changes through their actual controls. Retired `/delegations`, `/sessions`, and `/settings` paths showed the not-found view without redirects. No page errors or off-origin requests occurred. The review stopped the isolated smoke process.

The current Ripwire quality delta against `2e6755bc7` exited 2 with 311 gating findings. Categories include complexity, duplication, verbosity, dead code, clone reuse, error masking, and short-horizon churn. This differs from the earlier recorded run. The review suppressed or blanket-acknowledged no findings. Its test gate names coverage obligations but does not observe test execution.

The targeted backend run passed 32 of 33 checks. The root GET returned 404 during a concurrent frontend rebuild of the shared `web/dist` directory. After the build finished, the serialized root-routing group passed all three checks. This execution-order correction changed no product code.

The review corrected three contract and accessibility defects. Console detail no longer shows an Agent Origin Label, even with CIMD metadata. Linked and selectable entity controls expose status, supporting context, and metadata through their accessible descriptions. Collection loading regions contain live text without `aria-busy` announcement deferral. The console assertion and four entity-description cases failed before their fixes.

After these corrections, `just web-test` passed 712 tests in 77 files and `just web-lint` passed. The review removed three incidental geometry test cases instead of preserving class names or child counts as contracts. Both decision bundle checks and the Storybook build passed. The final production-browser rerun passed all 77 journeys.

An actual Storybook Chromium smoke observed the linked entity description and a Critical Risk approval description with its requesting agent. It also observed loading text and the console fixture without a trust claim. No page errors occurred. This proves browser accessibility-tree exposure, not a screen-reader announcement or baseline approval.

Human-reviewed Linux screenshot baselines, non-updating visual comparisons, original test-first chronology, and unmeasured study outcomes remain separate release requirements. This commit review does not close those requirements or claim a full `just verify` result.

## Follow-up presentation review — 2026-10-05

The follow-up replaces the consent relationship motif with a horizontal agent identity and an account line beneath the card. The full wordmark groups with the card. The Powered by footer is absent. Without CIMD metadata, consent shows Registered agent without a registrant claim. The square single-letter favicon remains legible in the observed 16 px and 32 px samples.

Connections owns service identities and Connect/Reconnect actions on agent details. Permission rows retain names, selections, required locks, and descriptions without duplicate connection controls. Connection cards group identity, status, metadata, and actions. Their 160 px minimum grows for actual content.

An explicit global collection view clears earlier page choices. Search collapses on outside blur without clearing its query. Approval tabs retain their header positions and fade their content. Exiting approval panels disable decisions and close decision portals while retaining their visual fade.

The journey migration removes dependencies on retired headers, provider tiles, inline save messages, and fixed card heights. The journeys retain server-authoritative grants, CSRF, callback drafts, scope boundaries, principal isolation, and keyboard coverage.

Observed checks:

| Check | Result |
| --- | --- |
| Frontend lint | Passed. |
| Frontend unit suite | 767 tests passed in 79 files. |
| Production build and decision bundle gate | Build passed. Both bundle checks passed. |
| Changed browser stories | 93 cases passed in each of six serial projects: 558 cases across both themes and three viewport sizes. |

A fixture-backed Chromium smoke exercised Connect callbacks, restored unsaved selections, and returned Reconnect to `/connections`. It observed the applied global view, retained collapsed search, error-message close hover, and stable approval headers during the opacity fade. Browser Back closed an open permanent-denial dialog without a decision request. These observations produced no page errors.

Private candidate images remain separate from reviewed baselines. This review does not approve baselines or claim a full `just verify` result.

The final scoped quality delta exits 2 with 102 findings: 67 duplication, 17 clone-reuse, and 18 short-horizon-churn findings. It reports no major complexity or dead-code regression in these scopes. Page-object getters retain the existing explicit locator pattern rather than a new generic abstraction. The review adds no acknowledgement ledger and does not claim a clean quality gate.

Final integration results:

- `just check` passed formatting, vet, and lint with 0 issues.
- `go test ./tests/e2e/pages` passed.
- The production-built frontend suite passed all 78 journeys with 0 failed, pending, or skipped cases.

The final browser run includes the new rapid-selection regression and the retained consent, CSRF, callback, scope, ownership, and keyboard journeys.

## PR review corrections — 2026-10-05

- ADR 037 remains the CIMD key-domain decision. The design decision is ADR 038; callback-state transport is ADR 039. References use these distinct numbers.
- Compose exposes one current UI at :8000. Its Go end-user port is internal. The production image builds frontend source instead of copying host `web/dist`.
- Native `just run` builds both artifacts. Native Air rebuilds frontend source and Go together. Optional native Vite requires the matching :3000 public URL.
- Four Vitest integration suites add 20 current-UI journeys. They use real routes, providers, hooks, and API clients with HTTP responses replaced. Default frontend tests and coverage include them.
- The README console section and route gallery are withdrawn. Route guidance stays in the documentation. Verification records do not create a separate release-approval process.
- Optional nullable `logoUrl` fields remain in the contract. No logo persistence is added. Grants return an array, including an empty array after revocation.
- The documented affected-agents operation uses existing principal-authorized session lookup. The disconnect dialog reads it before confirmation. Prior object/null behavior and the missing route were implementation drift, not grounds to narrow the published API.

Observed verification:

| Check | Result |
| --- | --- |
| Default Vitest and coverage commands | Each passed 796 tests in 83 files, including all 20 restored integration cases. Line coverage: 89.2%. |
| Consent/session handler, routing, and domain package tests | All five packages passed. |
| Focused production-bootstrap API journeys | Five passed, covering grants-list submission/revocation and affected-agent principal isolation. |
| Production-served browser journeys | All 78 passed, with no failed, pending, or skipped cases. |
| Backend/frontend build and decision-bundle check | Passed. Both decision graphs passed. |
| Static Go checks and frontend lint | Passed. Go formatting, vet, and lint reported 0 issues. |
| Documentation site build | Passed. The build retained its CSS-minimizer and Node localStorage warnings. |
| Compose configuration | One published browser port, :8000, with internal API proxying and matching HMR port. |
| Actual native and isolated Compose browser smoke | Agents, agent detail, affected-agent disconnect warning, canceled decisions, and Dark appearance passed without page errors. Confirmed revocation returned an empty grants array. |
| Source-built production Docker smoke | Image built successfully. Its real broker served current Agents, Connections, and Appearance; missing-agent validation remained intact. |
| OpenAPI structural check | All 161 internal references resolved. Grants are an array; affected-agents and three optional logo schemas remain documented. |

The first integrated tests exposed worker contention during lazy imports, a detached animated-card assertion, and an inconsistent prior-grant fixture. The runner uses four workers. The tests inspect current elements and use a grant that satisfies required-service rules. No production authorization assertion was weakened.

A separate test-only TypeScript check still reports 100 existing unit-test diagnostics, primarily unsupported `exact` role-query options and untyped `closest` results. The four restored integration suites and their helper have no diagnostics. Production TypeScript compilation passes. These unrelated unit-test typing errors remain unchanged.

The structural quality check reports nine major heuristic findings for explicit API transport methods and the extracted existing session-validation path. These retain local patterns rather than adding generic transport or validation abstractions. No quality acknowledgement or clean full-`just verify` claim is made.

Owned smoke services, containers, volumes, image tags, scripts, and private environment files were removed. Existing user-owned review services were left unchanged.
