# Implementation Plan: Consent UI v2 — Redesign End-User Consent and Console

**Branch**: `047-redesign-consent-console` | **Updated**: 2026-09-26 | **Spec**: [spec.md](spec.md)

**Input**: The Consent UI v2 specification and the user's technical direction. This plan covers research and design. `tasks.md` holds the dependency-ordered task list. Nothing in these documents implements the UI.

This existing feature directory is the redesign referenced as 047 in the review request. Do not create a duplicate feature.

## Summary

Replace the design-system layer while preserving the application stack and all existing authorization flows. Use owned shadcn/Radix components, semantic OKLCH themes, local brand assets and fonts, and two shared shells.

Keep React 19, TypeScript, Vite 7, Tailwind 4, React Router 7, Axios, CVA, `tailwind-merge`, Vitest, Testing Library, Storybook 10, and Ginkgo/Playwright. Add TanStack Query, TanStack Table, Lucide, and cmdk. Use Sonner for notifications and CSS-only motion.

Deliver all six routes, both agent contexts, settings, and command search in one cutover. The feature adds no API, persistence, or migration. It corrects the OpenAPI documentation of the existing `GET /api/third-party/sessions` response to match the handler. Keep decision and console bundles separate.

**Security resolution**: ADR 014's gateway long-poll is not a browser source. Refresh the existing acting-user pending list every 10 seconds. This follows ADR 018 and spec API-004 despite the user's long-poll shorthand.

**Approval status**: ADR 037 is proposed. Planning is complete only as a design deliverable. Runtime implementation remains blocked until its acceptance.

**Delivery constraint**: No implementation phases, feature flags, parallel old/new presentations, backwards-compatibility layers, or old-server fallbacks. Migrate every consumer together.

## Technical Context

| Area | Decision |
| --- | --- |
| Language/version | TypeScript with React 19.2.4 declarations, Go 1.27.1 backend. Lockfiles govern resolved versions |
| Build/routing | Vite 7, Tailwind 4 CSS-first `@theme`, Router 7, root-mounted SPA, React.lazy/Suspense |
| Owned primitives | Radix-based shadcn source inside existing `design-system/components/` categories |
| Retained utilities | CVA, `tailwind-merge`, existing `cn()`, Axios and error interceptors |
| Added dependencies | TanStack Query v5, TanStack Table v8, Lucide, cmdk, Sonner, required Radix packages. Pin compatible versions during foundation work |
| Visual choices | Neutral blue accent, complete semantic OKLCH light/dark tokens, 1 px `border`-token borders, no decorative gradients |
| Typography | Self-hosted variable Zalando Sans, Inter, JetBrains Mono. No SemiExpanded download |
| Brand | Outlined supplied wordmarks and favicon in `web/public/brand/`, compact local monogram |
| Motion | CSS transitions and progressive `@starting-style`, 120–200 ms ease-out, reduced-motion support. Remove Framer Motion |
| Storage | Browser-only appearance preferences. No backend storage change. Connection state derives from existing session fields |
| Testing | Vitest 4 + Testing Library, Storybook 10.3.5 with a11y/themes and browser-mode Vitest addon, story-level `toMatchScreenshot`, Ginkgo + playwright-go, existing imgdiff |
| Target | Evergreen browsers, broker-served production SPA, Vite development, standalone Storybook |
| Performance | Consent content under 1.5 seconds under the Chrome DevTools “Slow 4G” preset. Initial compressed application code under 150 kB. Pending update within 15 seconds on a reachable foreground console |
| Accessibility | WCAG 2.2 AA, text 4.5:1, controls 3:1, keyboard paths, visible unobscured focus, announcements |
| Layout | Decision column at most 640 px. No page overflow at 320 px or 200% zoom. At least 12 agent rows at 1080 px height |
| Scope | Five existing routes, one new `/settings` route, both agent contexts, all 17 active acceptance scenarios in one release |
| Constraints | No automatic third-party resource requests. No OAuth2/token semantic changes. No API, response-field, persistence, or migration change |

## Constitution Check

### Before research

The supplied spec identifies the existing domain aggregates, the no-API boundary, security boundaries, and 17 active acceptance scenarios (AS-16 is retired). Constitution 2.1.0 permits a changed aesthetic through an ADR. The existing architecture remains binding.

Research resolved the accent, body face, brand format, browser synchronization, connection-state derivation, and CI gates. No rollout configuration is needed.

### After design

| Principle or precondition | Design evidence | Implementation gate |
| --- | --- | --- |
| I Security-first | Principal-scoped reads, escaped metadata, no automatic external resources, preserved authorization and audit logging | Cross-user, unsafe redirect, expiry, callback, and no-external-request scenarios pass |
| II Architecture/ADRs | ADR 037 proposed, architecture includes a labeled design section and new glossary entries | Accept ADR 037 before departing from ADR 006 or extending ADR 035 |
| III Library-first security | No custom crypto or new token format. Fixed script hash uses standard build tooling | Preserve existing cryptographic validation |
| IV/X API-first | No API contract change. The session-list documentation is corrected to the existing handler response, with stakeholder confirmation recorded in the spec (2026-09-27) | Any API change requires a separate, approved specification |
| V Domain model | ConsentDraft, ConnectionState, and preferences defined in data-model.md as UI state | Keep glossary and model synchronized |
| VI Hexagonal boundaries | No domain, port, or storage change. The HTTP adapter only gains the theme-script CSP hash | No handler-to-repository bypass or adapter cross-import |
| VII Configuration | No redesign flag, environment variable, CLI option, HTML flag bootstrap, or configuration endpoint | No redesign-specific configuration or Helm change |
| VIII TDD | Behavior-level tests precede their runtime changes | Demonstrate semantic red then green. No skipped or placeholder assertions |
| IX Persistence | No repository, schema, or migration change | None |
| XI Design system | Aesthetic-neutral principle, current direction in DESIGN_PRINCIPLES.md, proposed ADR 037, and the guidance inventory below | Accept the ADR, then align current guidance. Preserve history. Validate semantic tokens, both-theme stories, story visual regression, local assets, and accessibility |
| XII Builder wiring | No new service or worker | None |
| XIII E2E traceability | AS-01–AS-18 mapping below, Playwright journeys and screenshots | One canonical Ginkgo It per scenario, with real production bootstrap |

**Gate result**: The design introduces no unapproved runtime deviation. No technical clarification remains unresolved. ADR acceptance is still mandatory before implementation. Do not mark that approval complete merely because planning generated documents.

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

`tasks.md` is one dependency-ordered task list without partial-release milestones. It keeps the constitution's mandatory Phase 2 and Phase N headings verbatim. Its other headings group responsibilities, not implementation phases.

Planning artifacts describe the proposed direction. They do not make it current guidance before ADR acceptance.

### Implementation locations

```text
web/
  .storybook/                         Themes and browser a11y projects
  public/fonts/                      Subsetted variable WOFF2 and licenses
  public/brand/                      Outlined wordmarks, favicon, compact mark
  src/design-system/
    tokens/                          Complete semantic token source
    components/
      primitives/ inputs/ data-display/ navigation/ overlays/ feedback/ advanced/
      layout/                        ConsoleShell and DecisionShell
  src/services/api/                  Existing Axios transport and typed services
  src/hooks/                         Query-backed resource hooks and mutations
  src/pages/                         Existing lazy pages plus Settings
  src/styles/                        Font faces and global token imports
internal/
  adapters/http/                     Theme-script CSP hash
tests/e2e/frontend/                   Acceptance journeys and visual capture
tests/e2e/pages/                      Existing page-object conventions
tests/e2e/screenshots/                Reviewed light/dark baselines
.github/workflows/                   Blocking a11y and visual gates
```

Use the existing component categories rather than a second library.

## Single-Cutover Implementation

All work in this section belongs to one release. The headings group responsibilities, not implementation phases or independently releasable subsets.

Accept ADR 037 before changing the visual direction or component architecture.
Write traceable acceptance journeys before changing their behavior. Keep required refactors with their consumers.

### Shared design system and shells

Deliver semantic tokens, the first-paint theme script, CSS media fallback, color-scheme metadata, local fonts, outlined brand assets, and Lucide.
Enforce semantic token use through ESLint. Preserve existing component categories and `cn()`.
Remove the inert Tailwind JS configuration after moving required live references into CSS-first tokens.

Copy and own Button, Badge, Card, Dialog, DropdownMenu, Popover, Switch, Tabs, Tooltip, Sonner, Table, Command, Input, Select, Separator, Skeleton, and Sheet.
Migrate retained Checkbox, Radio, disclosure, and date controls into the same system.
Build ConsoleShell and DecisionShell with a mobile Sheet and compact mark.
Use TanStack Query over Axios without changing authentication. Keep Table and Command outside decision-route bundles.

Configure Storybook browser projects for light and dark. Make accessibility failures block CI for every primitive, shell, and interaction state.
Validate the theme script under production CSP. Theme preferences never select an old presentation.
Move brand assets and update README and Docusaurus consumers together.

### Decision and console journeys

Migrate every existing route and both agent contexts directly to the new design system.
Consent uses DecisionShell whenever `session_token` is present, including re-consent with an existing grant.
Expired authorization context remains a decision error.

Preserve authorization-session validation, `consent_state`, provider callbacks, safe continuation, required selection locks, duration validation, and scope preview.
Implement delta consent from existing grant data. Deny remains local and non-mutating.
Approval review shows the agent, acting user, tool, risk, expandable arguments, and exact persistence scope.
Approve once is the sole accent action. Resolved or expired approvals show an outcome without actions.
Never cache authorization context as ordinary agent detail. Do not optimistically grant or approve.

Use TanStack Table and owned Table styles for agents, connections, and approvals.
Add name search, truthful state, confirmed revocation, sticky draft controls, and pending-first approval triage.
The delegation list shows the existing `activeGrantCount` as granted permission sets and removes a row whose expiry passes. It adds no per-row requests.
Share the user-scoped pending query between the queue and sidebar. Announce count changes without moving focus.
Use record-scoped optimistic revoke with rollback. Derive connection state from existing session fields, refresh responses, and agent requirement status, without a Missing scopes state. No connection appears only on the agent Connections tab and in the consent service prompt.

### Preferences and navigation

Deliver the browser-only theme preference in `/settings`. Do not add an approval-persistence default.
Add Command/cmdk navigation across acting-user records and theme choices. A preference never submits a decision.

### Removal and release acceptance

Remove the old presentation and every obsolete consumer in the same change.
Remove Headless UI, Framer Motion, duplicate wrappers, old palette exports, gradients, page-entry animation, Crimson Pro, and Manrope.
Remove unused preloads and licenses. Remove `apiCache` after migrating all callers to Query over Axios.
Do not add compatibility aliases, adapters, alternate response parsing, or version negotiation.
Do not add `ui.v2`, environment or CLI equivalents, a flag bootstrap, or feature-flag deployment documentation.

All active AS-01–AS-18 scenarios must pass before release. Include both-theme Storybook checks, authorization journeys, performance measurements, and 14 reviewed route/context/theme screenshots.
Verify all six routes from the production-served application without a feature flag.
Publish README guidance and screenshots for all routes, including both agent contexts.

## Guidance Review and Decision Gate

Principle XI sets design-system and accessibility requirements without prescribing an aesthetic.
`web/src/design-system/docs/DESIGN_PRINCIPLES.md` remains the current visual authority until an accepted ADR changes it.
ADR 037 remains proposed. Its target is not an instruction to restyle existing components before acceptance.

Review results belong to this planning change. Adoption of target guidance depends on ADR acceptance.
After acceptance, update every current guide for the approved direction before implementation.
Deliver matching components, examples, tokens, stories, and documentation together. Do not leave guides for two active visual systems.

**Review completed on 2026-09-26**: All 19 current-guidance files and all 16 historical files in the inventories were reviewed.
Templates now reference the governing visual decision instead of a fixed aesthetic.
Current guides distinguish existing source, constitutional requirements, and the proposed replacement.
The root context no longer attributes Refined Trust Architecture to Principle XI.
Fifteen historical files have scoped authority notes. Feature 008's task record required no change.
Approval of ADR 037 and adoption of its target guidance remain open decision gates, not completed implementation.

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
| AS-01 | First consent shows the session-derived Agent Origin Label, warns on localhost, orders required groups, shows permission-set names, descriptions, and services without a risk indicator or raw scopes, and one Allow |
| AS-02 | Re-consent shows only new access, keeps prior groups read-only, starts from the existing validity, and preserves prior grant selections |
| AS-03 | Duration persists through Allow/callback, while Deny and expiry make no new grant |
| AS-04 | Tool review exposes identity, arguments, risk, exact scope, and one accent action |
| AS-05 | Once/session/permanent decisions preserve scope, resolved/expired requests cannot resubmit |
| AS-06 | Search agents, see permission-set counts and expiry without an Agent Origin Label, confirm the expired fixture is absent, confirm revoke |
| AS-07 | Empty delegation explanation and confirmed-revoke result, including rollback on failure |
| AS-08 | Console detail with Permissions and Connections tabs edits only optional groups, Cancel discards and Save persists |
| AS-09 | Overflow revoke names the agent, cancel preserves access, confirm affects only owner |
| AS-10 | Token-expiry and refresh fixtures produce truthful states on `/sessions`, an unconnected required service shows No connection on the agent Connections tab, and no Missing scopes state exists |
| AS-11 | Provider callback, supported refresh, and disconnect warning preserve token semantics |
| AS-12 | Pending-first triage and standing allow/deny revocation require explicit decisions |
| AS-13 | New pending request updates queue/count within 15 seconds without focus movement or gateway request |
| AS-14 | Sidebar and light/dark/system preferences persist without wrong-theme paint |
| AS-15 | Both shells remain usable by keyboard at narrow width and zoom with local branding |
| AS-17 | Settings theme choice survives reload; no approval-persistence default exists |
| AS-18 | Keyboard palette navigates only acting-user records and changes theme |

Use production builder/bootstrap and fresh servers, storage, principals, and browser contexts. Reuse existing mock provider fixtures. Add deterministic clock/data fixtures for expired grants, approval races, and long metadata.

Tests must compile and fail for absent behavior before implementation. Never use skips, source-text assertions, mock echoes, or trivial failures. Keep existing journey assertions intact when behavior is preserved. Update selectors through page objects only.

### Unit coverage

Keep useful Vitest/Testing Library coverage. Add tests for uncertain state boundaries: re-consent selection preservation, custom expiry, session-state precedence, optimistic rollback concurrency, and identity cache clearing.

### Storybook and visual gates

Run every design-system story with light and dark globals and `a11y.test: 'error'`. Include overlay-open, error, loading, focus, and disabled states. Browser-mode testing is required for rendered contrast.

Principle XI also requires story visual regression. A Vitest-only `afterEach` project annotation in `web/.storybook/vitest.setup.ts` asserts `toMatchScreenshot` on each rendered story root in both theme projects. Baselines are reviewed Linux Chromium images keyed by story ID and theme. CI never updates them and fails on a missing or changed image.

The existing Ginkgo/Playwright harness captures to `tests/e2e/frontend/coverage/screenshots/`. Reviewed baselines stay in `tests/e2e/screenshots/`. Reuse `tools/imgdiff` with the existing 0.5% threshold as the starting policy. The gate covers the 14 required images plus the state stems listed in `tests/e2e/screenshots/visual-gate.txt`. It fails on a gated capture without a baseline, a gated baseline without a capture, or excess difference. Publish expected, actual, and comparison artifacts. Never automatically approve the gate's own output.

The screenshots workflow stops writing to and committing `tests/e2e/screenshots/`. It uploads route, state, and Storybook candidates as a review artifact. Every baseline changes only in a reviewed commit.

Original-route minimum: `/delegations`, two `/agents/:id` contexts, `/sessions`, `/approvals`, `/approvals/:id`, each light/dark: 12 images. Settings adds two images. Add meaningful loading, empty, error, and overlay states without replacing route coverage.

### Runtime and performance proof

After implementation, run actual production-served routes, Storybook, and the full journeys. Observe focus, console/network errors, CSP, callback results, preference persistence, and reduced motion. Tests do not replace this smoke proof.

Measure each decision route's initial dependency graph after compression, not the whole app total. Measure consent content time under the “Slow 4G” preset once the P1 flows work, and again before release. Exclude console-only chunks. Verify row density and overflow at specified viewports. SC-001 and SC-007 need moderated user sessions, not automated timing claims.

Commands and expected outcomes are in [quickstart.md](quickstart.md). New gate recipes are explicitly marked as planned there.

## Complexity Tracking

No constitutional violation is waived. The redesign has one presentation and one release. Preserved authorization behavior does not require a compatibility layer.

ADR 037 narrowly replaces the UI-components/animation clause and server-cache ownership in ADR 006. It keeps local UI state in React and keeps Axios. Its route extension preserves ADR 035's root mounting and protocol precedence.

The feature adds no API. Consent Deny stays local because no existing validated cancellation path exists.
