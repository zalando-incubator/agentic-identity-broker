# Consent UI v2 Cutover Inventory

## Execution status

The first T001 attempt stopped on 2026-09-27 because frontend dependency installation failed.
The dependency mismatch is resolved. The unit baseline, production build, and browser baseline pass.
No redesign runtime implementation task started.

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

The stakeholder selected **Accept ADR 037** in this session's `/speckit-implement` approval prompt on 2026-09-27.
The prompt requested permission to record the conversation as the decision reference and proceed with the complete single-cutover implementation.

T013 is complete: ADR 037 records acceptance, ADRs 006 and 035 reference it, and the root ADR index includes it.
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
- apiCache is entirely owned by web/src/services/api/ (cache.ts + consent/sessions consumers + one test mock); ADR 037 moves server-state ownership, so this module is the cutover's server-state surface.
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

ADR 037 is Accepted, dated 2026-09-27. Its acceptance permits guidance changes but does not prove runtime completion.
This section records documentation review and the replacement contract only.

I read every section of the four required guides before their replacement:

| Reviewed file | Pre-edit evidence | Required correction |
| --- | --- | --- |
| `web/src/design-system/docs/INDEX.md` | Lines 1–355. The authority section said ADR 037 was unaccepted. Its summaries named old fonts, gradients, strong shadows, 150–500ms motion, danger buttons, and Modal. | Publish accepted authority, one guide map, new fonts, semantic tokens, current variants, CSS-only motion, and blocking both-theme evidence. T015 updates this file. |
| `web/src/design-system/docs/DECISION_TREES.md` | Lines 1–552. The action tree used danger and allowed one primary per section. Text and status trees used the retired palette. Motion and shadow trees selected spring effects, card lifts, and page transitions. | T017 must use destructive confirmation, at most one accent action per view, semantic foregrounds, 120–200ms ease-out, and current owned component names. |
| `web/src/design-system/docs/COMPONENT_PAIRING_GUIDE.md` | Lines 1–725. Pairings used primary plus danger for approval, repeated card primaries, TextInput, Modal, raw scopes, a scopeCount-derived permission count, last-use text, and palette literals. | T018 must preserve consent semantics, use ConsoleShell/DecisionShell/PageHeader, use current components, remove unsupported counts and last use, and keep Table/Command out of decision imports. |
| `web/src/design-system/docs/COMMON_MISTAKES.md` | Lines 1–620. Several recommended fixes still selected the retired palette. Other rules required card shadows, old fonts, longer motion, danger zones, and a primary per section. | T017 must replace the fixes themselves, not only their authority note. Current examples must use semantic roles, current variants, truthful state, and the accepted visual system. |

These ranges refer to the files as reviewed, not to line numbers after the documentation rewrite.
The review also used feature 047 `spec.md`, `plan.md`, `tasks.md`, and `data-model.md`, accepted ADR 037, and Constitution Principle XI.

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

Each story needs a reviewed visual-regression baseline per theme under T065.
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
| T015 | `web/src/design-system/docs/DESIGN_PRINCIPLES.md`, `INDEX.md` | Accepted ADR 037 governs current guidance. Dated historical notes preserve Refined Trust Architecture. Feature labels use 047. |
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
| Required-service connection | `GET /api/third-party/{serviceId}/oauth2/authorize`, followed by the existing provider callback |
| Connections | `GET /api/third-party/sessions`; `GET` and `DELETE /api/third-party/{serviceId}/session`; `POST /api/third-party/{serviceId}/session/refresh` |
| Console sidebar and pending queue | `GET /api/approvals/pending` |
| Standing decisions | `GET /api/approvals/permanent`; `POST /api/approvals/{id}/revoke` |
| Approval review and inline decision | `GET /api/approvals/{id}`; `POST` to its `scope-preview`, `approve`, and `deny` operations |
| Command palette | Existing principal-scoped agent, connection, and pending-approval query results; no new endpoint |

All operations exist in `api/enduser/openapi.yaml`. No browser GET call targets the gateway-only `/api/approvals` operation.
The session list remains `{data: {sessions: [UserSessionSummary]}}`. Approval operations keep their existing data envelopes and principal checks.

### Verified consent documentation corrections

| Operation | Source evidence | Correction |
| --- | --- | --- |
| `GET /api/me` | `user_info_handler.go:43-89; internal/domain/consent/user_info.go:7-20`; client `consent.ts:41-57` | Documented nested data, principal-as-displayName fallback, and omission rather than null for unavailable email/pictureUrl. |
| `GET /api/consent/agents` | `agents_handler.go:31-73; internal/domain/consent/service.go:725-731,778-833`; client `consent.ts:67-84` | activeGrantCount counts active UserGrant records, not services or permission sets. Corrected examples, latest update/earliest finite expiration descriptions, omitted indefinite expiresAt, and removed unproduced logoUrl documentation. |
| `GET /api/consent/agents/{agent-id}` | `agent_detail_handler.go:74-135,215-329,369-420`; client `consent.ts:122-145; types/consent.ts:231-253` | Corrected mixed wire casing: agentId plus snake_case metadata. Documented existing client_id/client_uris and required timestamps. Required all five always-emitted detail fields. Replaced nonexistent available_services with existing service_requirements; removed now-unused AvailableService schema. Completed response example. Removed unsupported detail logos. Corrected omitted optional fields and CIMD metadata semantics: no exposed authorization-session claims or null placeholder. |
| `GET /api/consent/agents/{agent-id}/grants` | `grants_handler.go:56-94,348-365; internal/domain/consent/service.go:747-772`; client `consent.ts:162-180; types/consent.ts:260-262` | Replaced array schema/examples with one grant object or null. Grant lookup can include expired grants: memory/user_grants.go:141-156 and postgres/user_grants.go:361-365 do not filter expiry. Corrected permission-set content and omission of indefinite valid_until. |
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
