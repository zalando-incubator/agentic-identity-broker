# Implementation Plan: Consent UI v2 — Redesign End-User Consent and Console

**Branch**: `047-redesign-consent-console` | **Updated**: 2026-10-05 | **Spec**: [spec.md](spec.md)

**Input**: The Consent UI v2 specification and the user's technical direction. This plan covers research and design. `tasks.md` holds the dependency-ordered task list. The 2026-10-05 revision aligns this plan with the implemented branch; `cutover-inventory.md` holds the dated verification evidence.

This existing feature directory is the redesign referenced as 047 in the review request. Do not create a duplicate feature.

## Summary

Retain the owned shadcn/Radix component layer, React 19, TypeScript, Vite 7, Tailwind 4, React Router 7, Axios, CVA, `tailwind-merge`, Vitest, Testing Library, Storybook 10, and Ginkgo/Playwright. TanStack Query owns principal-scoped server data. Lucide, cmdk, and Sonner remain. TanStack Table and the owned Table primitive are removed. Motion and owned lucide-animated source are permitted only in the console graph; decisions use CSS/SVG only.

Amend accepted ADR 038 under the stakeholder's five 2026-10-04 decisions. Deliver seven destination routes and both agent contexts in one cutover. No end-user API, storage, migration or provider-facing callback protocol changes. With the route rename, the broker callback returns provider errors and a missing return URL to `/connections` instead of `/sessions`. Retain the accepted form POST and tab-local callback draft, together with the approved documentation-only corrections for existing responses and statuses.

**Security resolution**: ADR 014's gateway long-poll is not a browser source. Refresh the existing acting-user pending list every 10 seconds. This follows ADR 018 and spec API-004 despite the user's long-poll shorthand.

**Approval status**: ADR 038 was accepted on 2026-09-27. The five UX decisions were approved on 2026-10-04 and amend its presentation choices. Neither approval proves runtime implementation, accessibility, visual or performance validation.

**Canonical-reference decision (2026-10-06)**: [Accepted ADR 040](../../adrs/040-canonical-light-visual-reference-gate.md) narrows pixel references to light mode at 1280 × 720. It supersedes only ADR 038's visual-reference matrix. The six Storybook projects retain interactions and accessibility checks. Design, APIs, authorization, dark/responsive support, and the accepted 190-kB decision budget remain unchanged.

**Verification status (2026-10-05)**: `cutover-inventory.md` records dated command results and unavailable evidence. The PR uses the repository's normal CI checks. It adds no separate release-approval process or requirement to reconstruct historical test runs or conduct moderated studies before release.

Visual baselines remain reviewed test inputs, not proof that a command passed. The README remains high-level; route guidance belongs in the documentation.

**Delivery constraint**: No implementation phases, feature flags, parallel old/new presentations, backwards-compatibility layers, or old-server fallbacks. Migrate every consumer together.

## Technical Context

| Area | Decision |
| --- | --- |
| Language/version | TypeScript with React 19.3 and matching type declarations, Go 1.27.1 backend. Lockfiles govern resolved versions |
| Build/routing | Vite 7, Tailwind 4 CSS-first `@theme`, Router 7, root-mounted SPA, React.lazy/Suspense |
| Owned primitives | Radix-based shadcn source inside existing `design-system/components/` categories |
| Retained utilities | CVA, `tailwind-merge`, existing `cn()`, Axios and error interceptors |
| Added dependencies | Pinned TanStack Query v5, Lucide, cmdk, Sonner and `radix-ui` remain. `motion` is added for console routes only; it installs `framer-motion` transitively. Owned lucide-animated icon source lives in `web/src/components/icons/`. TanStack Table and the Table primitive are removed |
| Visual choices | Tinted neutral OKLCH palette, one stronger blue accent, soft status fills, `--primary-soft` selections; `--border-subtle` for card edges and dividers, `--border` for buttons/popovers, `--border-control` for 3:1 form controls |
| Typography | Self-hosted variable Zalando Sans, Inter, JetBrains Mono; console 14 px base, consent 15 px, titles 20 px |
| Brand | Outlined supplied wordmarks and favicon in `web/public/brand/`; ink-cropped 18 px wordmark and a compact mark for decision headers and the collapsed sidebar. The decision footer is a local text line |
| Motion | Shared 120/160/200 ms feedback and 320 ms emphasis, `cubic-bezier(0.2, 0, 0, 1)`. Console: Motion list exits in `AnimatedCollection` and animated Bot, Connection and ShieldCheck icons. Decision: CSS/SVG `DecisionSuccessCheck` only. Reduced motion removes movement |
| Storage | Browser-only appearance preferences: `aib.theme`, `aib.sidebar-collapsed`, `aib.collection-view-default`, and `aib.collection-view.{agents,connections}`. No backend storage change. Connection state derives from existing session fields |
| Testing | Vitest 4 + Testing Library, Storybook 10.3.5 with a11y/themes and browser-mode Vitest addon, story-level `toMatchScreenshot`, Ginkgo + playwright-go, existing imgdiff |
| Target | Evergreen browsers, broker-served production SPA, Vite development, standalone Storybook |
| Performance | Cold consent content within 5 seconds under Chrome DevTools “Slow 4G”; each initial decision graph under the 2026-10-05 approved 190 kB gzip limit. Use precompressed public assets and decision-specific code hints. Pending updates remain within 15 seconds on a reachable foreground console |
| Accessibility | WCAG 2.2 AA, text 4.5:1, controls 3:1, keyboard paths, visible unobscured focus, announcements |
| Layout | Centered console 1120 px / 12 columns with 24 px side padding (16 px below 768 px). Consent card 480 px, at most 616 px high, with a pinned footer. Standalone approval review uses a 640 px decision column. Agent detail uses 8 + 4 and the approval inbox uses fixed rows and a 5 + 7 detail panel, both from 1012 px. No horizontal page overflow at 320 px or 200% zoom |
| Scope | Seven destination routes, both agent contexts, all 17 active acceptance scenarios in one release; `/` enters `/agents` with no retired aliases |
| Constraints | No automatic third-party resource requests. No OAuth2/token semantic changes. No API, response-field, persistence, or migration change. The callback's error and fallback redirect target follows the route rename |

## Constitution Check

### Before research

The supplied spec identifies the existing domain aggregates, the no-API boundary, security boundaries, and 17 active acceptance scenarios (AS-16 is retired). Constitution 2.1.0 permits a changed aesthetic through an ADR. The existing architecture remains binding.

Research resolved the accent, body face, brand format, browser synchronization, connection-state derivation, and CI gates. No rollout configuration is needed.

### After design

| Principle or precondition | Design evidence | Implementation gate |
| --- | --- | --- |
| I Security-first | Principal-scoped reads, escaped metadata, no automatic external resources, preserved authorization and audit logging | Cross-user, unsafe redirect, expiry, callback, and no-external-request scenarios pass |
| II Architecture/ADRs | ADR 038 accepted on 2026-09-27 and amended by stakeholder approval on 2026-10-04; architecture and active glossary reflect the new routes | Preserve both dated decisions and the accepted callback-state ADR; do not infer implementation completion |
| III Library-first security | No custom crypto or new token format. Fixed script hash uses standard build tooling | Preserve existing cryptographic validation |
| IV/X API-first | No end-user API contract change. Approved documentation-only corrections align existing consent, session, and approval responses/statuses with their handlers. `api/enduser/openapi.yaml` documents the callback's `/connections` error and fallback return | Any runtime API change requires a separate, approved specification |
| V Domain model | ConsentDraft, ConnectionState, and preferences defined in data-model.md as UI state | Keep glossary and model synchronized |
| VI Hexagonal boundaries | No domain, port, or storage change. `handlers/spa.go` gains the theme-script CSP hash, negotiated precompressed public assets, and build-manifest preload headers. The OAuth2 session adapter returns callback errors to `/connections` | No handler-to-repository bypass, private bootstrap data, or adapter cross-import |
| VII Configuration | No redesign flag, environment variable, CLI option, HTML flag bootstrap, or configuration endpoint | No redesign-specific configuration or Helm change |
| VIII TDD | Behavior-level tests precede their runtime changes | Demonstrate semantic red then green. No skipped or placeholder assertions |
| IX Persistence | No repository, schema, or migration change | None |
| XI Design system | Principle XI, accepted ADR 038 amendments, ADR 040's canonical-reference policy, and current design guides govern the shared system | Three border roles, both-theme stories and accessibility, canonical light visual regression, decision-route Motion exclusion, WCAG 2.2 AA target |
| XII Builder wiring | No new service or worker | None |
| XIII E2E traceability | AS-01–AS-18 mapping below, Playwright journeys and screenshots | One canonical Ginkgo It per scenario, with real production bootstrap |

**Gate result**: The 2026-09-27 ADR acceptance and 2026-10-04 five-decision amendment are recorded, not inferred. The September count-column decision and documentation-only API corrections remain approved. Runtime implementation must satisfy the amended design and validation gates.

## Project Structure

### Design artifacts

```text
specs/047-redesign-consent-console/
  spec.md
  plan.md
  research.md
  data-model.md
  quickstart.md
  contracts/
    ui-and-configuration.md
```

`tasks.md` retains the original dated task and evidence history. A separate package 0–10 checklist tracks this amendment. Its unchecked items include implementation work, test-first evidence, and release-gate tasks. The two task records do not convert old success reports into proof of the new implementation.

The stakeholder approved the planned direction on 2026-09-27 and the presentation amendment on 2026-10-04. Planning artifacts remain requirements, not proof of runtime completion.

### Implementation locations

```text
web/
  .storybook/                         Light/dark projects at 1280, 375 and 768 px, a11y, screenshot hook
  build/decisionBundle.test.ts        Decision graph size and forbidden-module gate
  public/fonts/                      Subsetted variable WOFF2 and licenses
  public/brand/                      Outlined wordmarks, favicon, compact mark
  src/design-system/
    tokens/                          Complete semantic token source
    theme/                           Theme and collection-view preferences, preview tiles
    components/
      primitives/ inputs/ navigation/ overlays/ advanced/
      data-display/                  Entity (EntityCard, EntityRow), CollectionToolbar, TruncatedText
      feedback/                      EmptyState, CollectionSkeleton, DecisionSuccessCheck
      layout/                        ConsoleShell, DecisionShell, PageHeader, SettingsLayout
  src/components/                    consent/ (PermissionPanel, DurationSelect), approvals/, sessions/, icons/, layout/, command/
  src/copy/                          User-facing copy
  src/services/api/                  Existing Axios transport and typed services
  src/hooks/                         Query-backed resource hooks and mutations
  src/pages/                         Lazy route containers plus pure *View components and screen stories
  src/storybook/                     Shared fixtures and screen shell
  src/styles/                        Font faces and global token imports
internal/
  adapters/http/handlers/spa.go      Theme-script CSP hash, precompressed assets, preload headers
  adapters/http/oauth2_sessions/     Callback error and fallback return to /connections
tests/e2e/frontend/                   Acceptance journeys and themed capture
tests/e2e/pages/                      Page objects
tests/e2e/screenshots/                Reviewed canonical light references and visual-gate.txt
tools/imgdiff/                        Route and state image gate
.github/workflows/                   ci.yml blocking gates; screenshots.yml candidate artifacts
```

Use the existing component categories rather than a second library.

## Single-Cutover Implementation

All work in this section belongs to one release. The headings group responsibilities, not implementation phases or independently releasable subsets.

Keep ADR 038's 2026-09-27 acceptance and record the 2026-10-04 approved amendment before replacing its original visual choices.
Write traceable acceptance journeys before changing their behavior. Keep required refactors with their consumers.

### Shared design system and shells

Retune the semantic tokens before rebuilding views. Tint neutrals at hue 260 and separate `--border-subtle`, `--border`, and `--border-control`. Add `--primary-soft`, soft status colors, a 12/13/14/16/20/24 px type scale, and distinct 6/8/12/16 px shape roles. Test contrast pairs rather than claiming unverified rendered contrast. Crop the local logos to their ink. Preserve first-paint theme, local fonts and CSP.

Replace the four-item sidebar with Agents, Connections, and Approvals. Settings, the theme choice and a Documentation link open from the user menu. Put a 28 px icon collapse control beside the logo with the Ctrl/⌘ B shortcut, make search a quiet row that opens the command palette (Ctrl/⌘ K), and move stale status to a tooltip. Center console content in a 1120 px container on 12 columns; use 8 + 4 for agent detail and 5 + 7 for the approval inbox. `PageHeader` has title/count, an optional purpose, and a right-side actions slot that collection pages fill with the icon `CollectionToolbar`. Use underline URL tabs for Pending and Remembered, not pill controls mixed with actions.

Build EntityCard, EntityRow, CollectionToolbar, TruncatedText, EmptyState, and layout-matched skeletons in the owned design system. Keep shared PermissionPanel and DurationSelect for consent and agent detail, with Connections owning connection status/actions on console details. Agent navigation and confirmed Revoke remain independent controls. Search preserves URL query state across focus-driven collapse. Resolve view from page choice, explicit global default, then initial density fallback; changing the global default resets earlier page choices. Connection cards use a responsive 160 px minimum. Measure clipped text before disclosures and never unfold an entity under the pointer.

Keep TanStack Query over Axios with principal-scoped cache isolation. Command remains lazy and outside decision bundles. Route `/` to `/agents`; use `/agents`, `/connections`, `/approvals`, `/approvals/remembered`, `/approvals/:id` and `/settings/appearance` with no aliases from deleted routes. Keep both contexts at `/agents/:id`. Connection Reconnect returns to `/connections`; Connect in consent or agent detail returns to the initiating page. The broker callback sends provider errors and a missing return URL to `/connections`. Preserve the provider form POST, tab-local draft, original return URL and callback validation. Remove the obsolete data tables and inline approval controls.

Configure Storybook browser projects for light and dark. Add foundations MDX, pattern stories, and composed screen stories built from pure `*View` props and shared fixtures, with no network mocking. Accessibility failures block all six projects. Canonical light pixel differences block acceptance after human-reviewed references, under ADR 040.
Validate the theme script under production CSP. Theme preferences never select an old presentation.
Move brand assets and update README and Docusaurus consumers together.

### Decision and console journeys

Migrate every existing route and both agent contexts directly to the new design system.
Consent uses DecisionShell whenever `session_token` is present, including re-consent. The decision header shows “Signed in as {name}” above one 480 px card with identity, origin and conditional risk callout, an inset permission panel, and a pinned footer. Keep Allow visible at 1280 × 720 with three groups and a risk callout; scroll only the permission panel past four groups. Show raw existing requested scopes in the technical-details dialog, never assigned to permission groups.
Expired authorization context remains a decision error.

Preserve authorization-session validation, provider callbacks, safe continuation, required selection locks, duration validation, and scope preview. When selections, duration or the return URL carry state, use the current `consent_state_id` form POST with a bounded tab-local draft and original return URL. A clean default draft uses the existing same-origin GET authorize link. Never put selections or nested authorization context in provider-facing state.
Implement delta consent from existing grant data. Deny remains local and non-mutating.
Approval review shows the agent, acting user, tool, risk, arguments as visible key/value rows for up to six top-level values, exact persistence scope, and the request expiry. When session identifiers exist, an on-demand Session context popover shows them. “View JSON” opens the raw payload. Approve once is the sole accent action. High/critical risk tints the detail header; resolved or expired approvals have no decision controls.
Never cache authorization context as ordinary agent detail. Do not optimistically grant or approve.

Use shared entity cards for Agents and Connections, with a list toggle. Their fixed anatomy contains an icon, name, one support line, at most one badge and two visible actions. Agents cards link to detail and show expiry, a seven-day expiry warning and the `lastModifiedAt` date; never show `activeGrantCount` or request per-row counts. Agents sort by name, recently changed or expiring soonest. Agent detail uses a headed permissions card and a card with `DurationSelect` directly visible. Its header shows stored origin, saved grant expiry and relative change time. Its rail lists each agent service requirement with state and Connect or Manage, followed by About only for a description or configured HTTP(S) links. Keep the dirty-only sticky edit bar and a visible confirmed Revoke button. Grant saves report server-confirmed success through Sonner. Agent collection Revoke uses `ShieldOff`. Connections cards expose existing scopes in a popover, show truthful stored-session state, and retain dependent-agent disconnect warnings. Needs sign-in guidance appears in a hover and keyboard-focus badge tooltip. Disconnect uses `Unlink`, distinct from the navigation icon. Connections sort needs-attention first and filter by state. Replace old inline notices with toasts; a callback success also pulses the updated card and plays its connection icon. Unify visible “connection” copy.

Approvals becomes a stable inbox: fixed-height pending rows in a 5-column list, a sticky 7-column detail panel, and a pinned Deny/Approve once bar with explicit persistence menus. Rows show request age, or the remaining minutes in the last hour. Permanent approval moves the panel into its scope editor without shifting list bounds. Below 1012 px, the panel is hidden and each row links to the existing `/approvals/:id` decision view. Place standing decisions on `/approvals/remembered` with a `filter` query (all, allowed, denied), outline Revoke and a confirmation dialog. On the wide inbox, keyboard J/K/A/D/Enter works outside editable fields, dialogs, menus, popovers and the scope editor, and requires the same server-authoritative decision checks. Share the user-scoped pending query with the sidebar; announce arrivals without moving focus.

Revocation is record-scoped and server-confirmed. The affected entity shows a busy state and leaves the collection only after the server accepts the request. On failure, it stays and unrelated changes are kept. Derive connection state from existing session fields, refresh responses, and agent requirement status, without a Missing scopes state. A 409 or 502 refresh response requires Reconnect; a failed read or other refresh failure shows Unavailable with Retry. The No connection badge appears only in the agent-detail rail; consent marks the missing service with a warning and Connect.

### Preferences and navigation

Deliver theme tiles and a default grid/list preference at `/settings/appearance`. Settings lives in the user menu and command palette, not the sidebar. Hide its category list while Appearance is the only category; preferences apply immediately. Do not add an approval-persistence default.
Add Command/cmdk navigation across acting-user records, the Appearance page and theme choices. Agent results open `/agents/:id`, connection results open `/connections`, and pending approvals open `/approvals/:id`. A preference never submits a decision.

### Removal and release acceptance

Remove the old presentation and every obsolete consumer in the same change.
Remove Headless UI, the direct Framer Motion dependency, duplicate wrappers, old palette exports, gradients, and page-entry animation. Keep only the new console-scoped `motion` dependency, which installs `framer-motion` transitively. Remove TanStack Table, the owned Table primitive, `InlineApprovalActions`, `PendingApprovalsTable`, `DelegationsTable`, `ConnectionsTable`, both standing approval tables, the agent overflow menu, Crimson Pro and Manrope.
Remove unused preloads and licenses. Remove `apiCache` after migrating all callers to Query over Axios.
Do not add compatibility aliases, adapters, alternate response parsing, or version negotiation.
Do not add `ui.v2`, environment or CLI equivalents, a flag bootstrap, or feature-flag deployment documentation.

All active AS-01–AS-18 scenarios must pass before release. Keep pure screen stories at 375/768/1280 px in both themes. Include interaction checks for stable row bounds and consent footer fit. Require all authorization journeys and eight reviewed light-mode route/context screenshots at 1280 × 720. The existing decision-bundle test must reject Motion imports. Validate all seven destination routes from the production-served app without a feature flag.
Keep route guidance in `docs/concepts/delegation-and-consent.md`. Keep the README high-level without a console section or route-by-route screenshot gallery.

## Guidance Review and Decision Gate

Principle XI sets design-system and accessibility requirements without prescribing an aesthetic.
`web/src/design-system/docs/DESIGN_PRINCIPLES.md` defines the accepted visual direction under ADR 038.
The stakeholder accepted ADR 038 on 2026-09-27.

The review inventory records the current-guidance files that must follow the accepted direction.
Update those guides before runtime implementation.
Deliver matching components, examples, tokens, stories, and documentation together. Do not leave guides for two active visual systems.

**Review completed on 2026-09-26**: All 19 current-guidance files and all 16 historical files in the inventories were reviewed.
Templates now reference the governing visual decision instead of a fixed aesthetic.
The 2026-09-26 review distinguished existing source, constitutional requirements, and the then-proposed replacement.
The root context no longer attributes Refined Trust Architecture to Principle XI.
Fifteen historical files have scoped authority notes. Feature 008's task record required no change.
The 2026-09-27 acceptance clears the decision gate. Guidance adoption and runtime validation remain separate tasks.

### Current guidance inventory

| References | Required alignment |
| --- | --- |
| `.specify/templates/overrides/spec-template.md`, `.specify/templates/overrides/tasks-template.md` | Reference Principle XI and the current accepted visual direction. Do not mandate a fixed aesthetic or palette |
| `web/src/design-system/docs/DESIGN_PRINCIPLES.md`, `INDEX.md`, `COLOR_GUIDE.md`, `TOKEN_GUIDE.md` | Separate current direction from the proposal. After acceptance, publish one authoritative token, typography, color, and theme contract |
| `web/src/design-system/docs/COMMON_MISTAKES.md`, `COMPONENT_ARCHETYPES.md`, `DECISION_TREES.md` | Align examples, variants, and component selection with the accepted direction |
| `web/src/design-system/docs/COMPONENT_PAIRING_GUIDE.md`, `COMPOSITION_PATTERNS.md` | Align composition rules and examples without competing component libraries |
| `web/src/design-system/docs/ACCESSIBILITY_GUIDE.md`, `MOTION_GUIDE.md` | Preserve Principle XI's WCAG 2.1 AA floor. Apply the feature's WCAG 2.2 AA target, both-theme checks, and accepted motion tokens |
| `web/src/design-system/GettingStarted.mdx` | Update onboarding examples and asset use to match the accepted system |
| `web/src/components/sessions/README.md`, `web/src/components/sessions/DESIGN_SYSTEM_USAGE.md` | Update session examples and remove outdated aesthetic instructions |
| `AGENTS.md`, `web/AGENTS.md`, `ARCHITECTURE.md` | Separate constitutional requirements, current runtime facts, and proposed decisions. Remove obsolete instructions at cutover |

### Historical feature inventory

Preserve the aesthetic decisions, acceptance records, and completed tasks recorded for these features.
Change only references that incorrectly present an old rule as current guidance.
Use explicit historical scope and a link to current authority instead of rewriting past decisions.

| Feature directory | Reviewed files |
| --- | --- |
| `specs/007-consent-frontend/` | `plan.md`, `quickstart.md`, `research.md`, `tasks.md` |
| `specs/008-thirdparty-oauth2-sessions/` | `tasks.md` |
| `specs/011-agent-permission-requirements/` | `quickstart.md`, `research.md`, `tasks.md` |
| `specs/016-jwt-preauth/` | `research.md`, `spec.md`, `tasks.md` |
| `specs/019-permission-sets/` | `spec.md`, `tasks.md` |
| `specs/024-approval-api-ui/` | `spec.md`, `tasks.md` |
| `specs/028-cimd-support/` | `tasks.md` |

## Testing Strategy

### Canonical acceptance mapping

Use `tests/e2e/frontend/consent_ui_v2_test.go` as the planned canonical scenario file. Reuse page objects in `tests/e2e/pages/`. One Ginkgo `It` carries each exact scenario identifier. Theme parameterization stays inside that scenario rather than duplicating its identity.

Only read-only assertions run once per theme, each in a fresh browser context: AS-15 and the themed route screenshots, which are captured before a scenario mutates anything. AS-14 and AS-17 switch themes as their subject. Every other scenario, including every scenario that changes server state, runs once in the default theme.

| Scenario | Journey and principal assertion |
| --- | --- |
| AS-01 | One 480 px consent card identifies acting user and origin, warns on risk, shows required groups first, and keeps Allow or Connect visible at 1280 × 720 |
| AS-02 | Re-consent decides only new access, preserves read-only prior groups and existing validity, and does not widen the grant |
| AS-03 | Duration and selections survive the required-service connection form POST and callback; Allow follows validated continuation, Deny/expiry add no grant |
| AS-04 | Standalone review shows acting user, tool inputs, server risk and scope before any choice; Approve once stays the only accent action |
| AS-05 | Once/session/permanent approval and denial preserve exact scope; resolved or expired requests cannot resubmit |
| AS-06 | `/agents` search, linked cards and list toggle show expiry and recency without a count or origin field; expired grant absent, Revoke confirmed |
| AS-07 | Illustrated empty state; confirmed revoke marks only the affected entity busy, removes it after server acceptance, and keeps it on failure |
| AS-08 | Agent detail uses shared permission panel, duration, Connections rail, dirty-only Save; Cancel restores and Save persists |
| AS-09 | Visible Revoke access button opens a named dialog; Cancel preserves, Confirm affects only the owner |
| AS-10 | `/connections` cards/list show truthful stored-session states and scope popover; No connection appears only in the agent-detail rail, and consent marks the missing service with Connect |
| AS-11 | Provider return reaches `/connections`, with existing authorize/callback/refresh/token and dependent-agent disconnect behavior intact |
| AS-12 | `/approvals` inbox keeps fixed row bounds through selection and persistence editing; `/approvals/remembered` filters and revokes standing decisions |
| AS-13 | New pending requests update inbox/count within 15 seconds without focus movement or gateway request |
| AS-14 | Three-item sidebar, user-menu Appearance and theme preferences persist without wrong-theme paint |
| AS-15 | Both shells remain keyboard usable at narrow width and zoom; decision footers remain available and reduced-motion movement stops |
| AS-17 | `/settings/appearance` theme tiles and default grid/list choice persist; no approval-persistence default exists |
| AS-18 | Keyboard palette navigates only acting-user records, Settings appearance and themes |

Use production builder/bootstrap and fresh servers, storage, principals, and browser contexts. Reuse existing mock provider fixtures. Add deterministic clock/data fixtures for expired grants, approval races, and long metadata.

Tests must fail on absent behavior before implementation. Preserve existing security, authorization, principal isolation, storage and callback assertions. Under the 2026-09-28 clarification, replace obsolete raw-scope and initially enabled Save assertions with permission-group and dirty-only-Save checks; edit explicitly in the CSRF save journey. Replace old table/inline-action assertions with card links, stable approval row bounds, and remembered decisions. Keep the canonical bounded callback draft and page-object selectors.

### Unit coverage

Keep useful Vitest/Testing Library coverage. Add tests for re-consent preservation, custom expiry, stored-session precedence, record-scoped pending revoke, identity cache clearing, clipped-text measurement, collection URL/preference restoration, risk-aware panel decisions, and Motion exclusion from decision graphs.

### Storybook and visual gates

Run foundation, pattern and pure-screen stories with light and dark globals and `a11y.test: 'error'`. Compose screen stories from shared fixtures without a network mock. Include loading, empty, typical, dense, error, stale, long text, consent states and approval scope editing at 375, 768 and 1280 px. Interactions must cover permission, duration, keyboard approval and revoke confirmation. Assert stable collection bounds and Allow within the viewport at 1280 × 720. Browser-mode testing is required for rendered contrast.

Principle XI also requires story visual regression. Under [ADR 040](../../adrs/040-canonical-light-visual-reference-gate.md), the Vitest-only `afterEach` hook compares `document.body`, including Radix portals. The check requires desktop `storybook-light`, effective light theme, and an actual 1280 × 720 viewport. All six projects still run interactions and accessibility at 1280 × 720, 375 × 812 and 768 × 1024 in both themes. Intentional dark or narrow story overrides receive no PNG reference.

A 320 px preset remains available for manual inspection. Eligible references are human-reviewed Linux Chromium images in `web/.storybook/__screenshots__/`. CI never updates them and fails on a missing or changed canonical image.

The Ginkgo/Playwright harness captures to `tests/e2e/frontend/coverage/screenshots/`. Reviewed references stay in `tests/e2e/screenshots/`. `just test-e2e-frontend-visual` runs `tools/imgdiff` with its unchanged 0.5% threshold.

The gate requires eight route/context stems in light mode at 1280 × 720: `agents`, `agent_consent`, `agent_detail`, `connections`, `approvals`, `remembered_approvals`, `approval_review`, and `settings`. Required state stems in `tests/e2e/screenshots/visual-gate.txt` use the same canonical configuration. Dark and narrow PNGs are not required.

The gate fails on a capture without a reviewed reference, a stale gated reference, or excess difference. Publish expected, actual and diff artifacts. Never automatically approve gate output.

The screenshots workflow stops writing to and committing `tests/e2e/screenshots/`. It uploads route, state, and Storybook candidates as a review artifact. Every baseline changes only in a reviewed commit.

Destination-route minimum: `/agents`, both `/agents/:id` contexts, `/connections`, `/approvals`, `/approvals/remembered`, `/approvals/:id`, and `/settings/appearance`. Their eight references use `_light.png` at 1280 × 720. Required loading, empty, error and overlay references use the same configuration without replacing route coverage.

### Runtime and performance proof

After implementation, run actual production-served routes, Storybook, and the full journeys. Observe focus, console/network errors, CSP, callback results, preference persistence, and reduced motion. Tests do not replace this smoke proof.

Measure each decision route's compressed initial graph, not the whole app. Measure consent content on cold Slow 4G and verify that the primary action fits at 1280 × 720. Exclude console-only Motion, Command and list chunks. Check grid and list layouts, fixed approval row bounds, and 320 px/200% zoom. SC-001 and SC-007 require moderated sessions, not automated timing claims.

Commands and expected outcomes are in [quickstart.md](quickstart.md). The `web-storybook-test`, `web-bundle-check`, and `test-e2e-frontend-visual` recipes exist in the `justfile`, and `.github/workflows/ci.yml` runs them as blocking gates.

## Complexity Tracking

No constitutional violation is waived. The redesign has one presentation and one release. Preserved authorization behavior does not require a compatibility layer.

ADR 038 narrowly replaces the UI-components/animation clause and server-cache ownership in ADR 006. It keeps local UI state in React and keeps Axios. Its route extension preserves ADR 035's root mounting and protocol precedence.

The feature adds no public API contract. Consent Deny stays local because no validated cancellation path exists. The callback's error and fallback path follows the approved route rename. PR review also restores the documented grants-list response and affected-agents route where implementation drifted from the retained contract. Optional logo fields remain in the contract without new persistence.
