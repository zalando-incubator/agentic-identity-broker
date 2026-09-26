# Implementation Plan: Consent UI v2

**Branch**: `046-redesign-consent-console` | **Date**: 2026-09-25 | **Spec**: [spec.md](spec.md)

**Input**: The Consent UI v2 specification and the user's technical direction. This command ends after research and design. It does not generate `tasks.md` or implement the UI.

## Summary

Replace the design-system layer while preserving the application stack and all existing authorization flows. Use owned shadcn/Radix components, semantic OKLCH themes, local brand assets and fonts, and two shared shells.

Keep React 19, TypeScript, Vite 7, Tailwind 4, React Router 7, Axios, CVA, `tailwind-merge`, Vitest, Testing Library, Storybook 10, and Ginkgo/Playwright. Add TanStack Query, TanStack Table, Lucide, and cmdk. Use Sonner for notifications and CSS-only motion.

Preserve the five existing route addresses and root redirection. Add activity and settings only in phase 4. Keep decision and console bundles separate.

**Security resolution**: ADR 014's gateway long-poll is not a browser source. Refresh the existing acting-user pending list every 10 seconds. This follows ADR 018 and spec API-004 despite the user's long-poll shorthand.

**Approval status**: ADR 037 and detailed additive API contracts are proposed. Planning is complete only as a design deliverable. Runtime implementation remains blocked until their required acceptance.

## Technical Context

| Area | Decision |
| --- | --- |
| Language/version | TypeScript with React 19.2.4 declarations, Go 1.27.1 backend. Lockfiles govern resolved versions |
| Build/routing | Vite 7, Tailwind 4 CSS-first `@theme`, Router 7, root-mounted SPA, React.lazy/Suspense |
| Owned primitives | Radix-based shadcn source inside existing `design-system/components/` categories |
| Retained utilities | CVA, `tailwind-merge`, existing `cn()`, Axios and error interceptors |
| Added dependencies | TanStack Query v5, TanStack Table v8, Lucide, cmdk, Sonner, required Radix packages. Pin compatible versions during foundation work |
| Visual choices | Neutral blue accent, complete semantic OKLCH light/dark tokens, thin borders, no decorative gradients |
| Typography | Self-hosted variable Zalando Sans, Inter, JetBrains Mono. No SemiExpanded download |
| Brand | Outlined supplied wordmarks and favicon in `web/public/brand/`, compact local monogram |
| Motion | CSS transitions and progressive `@starting-style`, 120–200 ms ease-out, reduced-motion support. Remove Framer Motion |
| Storage | Browser-only appearance/defaults. New immutable activity events in memory and PostgreSQL. Session state remains derived |
| Testing | Vitest 4 + Testing Library, Storybook 10.3.5 with a11y/themes and browser-mode Vitest addon, Ginkgo + playwright-go, existing imgdiff |
| Target | Evergreen browsers, broker-served production SPA, Vite development, standalone Storybook |
| Performance | Consent content under 1.5 seconds on throttled 4G. Initial compressed application code under 150 kB. Pending update within 15 seconds on a reachable foreground console |
| Accessibility | WCAG 2.2 AA, text 4.5:1, controls 3:1, keyboard paths, visible unobscured focus, announcements |
| Layout | Decision column at most 640 px. No page overflow at 320 px or 200% zoom. At least 12 agent rows at 1080 px height |
| Scope | Five existing routes, two new routes, both agent contexts, 18 acceptance scenarios, four implementation phases |
| Constraints | No automatic third-party resource requests. No OAuth2/token semantic changes. No additional API beyond activity and the proposed session field |

## Constitution Check

### Before research

The supplied spec identifies the existing domain aggregates, the Activity Event, permitted API scope, security boundaries, and 18 acceptance scenarios. Constitution 2.1.0 permits a changed aesthetic through an ADR. The existing architecture remains binding.

Research was required to resolve the accent, body face, brand format, browser synchronization, session projection, configuration delivery, activity persistence, and CI gates. [research.md](research.md) resolves those choices.

### After design

| Principle or precondition | Design evidence | Implementation gate |
| --- | --- | --- |
| I Security-first | Principal-scoped reads, escaped metadata, no automatic external resources, preserved authorization and audit logging | Cross-user, unsafe redirect, expiry, callback, and no-external-request scenarios pass |
| II Architecture/ADRs | ADR 037 proposed, architecture includes a labeled design section and new glossary entries | Accept ADR 037 before departing from ADR 006 or extending ADR 035 |
| III Library-first security | No custom crypto or new token format. Fixed script hash uses standard build tooling | Preserve existing cryptographic validation |
| IV/X API-first | Proposed OpenAPI and session-field note exist under contracts | Stakeholder approves exact schema. Promote into `api/enduser/openapi.yaml` before implementation and render `docs/api/` examples |
| V Domain model | ActivityEvent, ConsentDraft, ConnectionState, and preferences defined in data-model.md | Keep glossary and model synchronized |
| ADR 013 typed IDs | Plan ActivityEventID through `internal/domain/id/gen_ids.go` | Generate type and update `internal/domain/id/AGENTS.md` during implementation |
| VI Hexagonal boundaries | Activity service/recorder and repository ports. Handlers parse, domain enforces ownership | No handler-to-repository bypass or adapter cross-import |
| VII Configuration | YAML, environment, CLI, default, validation, HTML bootstrap, and removal contract specified | Add and remove examples, config docs, Helm values/templates/README with the flag |
| VIII TDD | Behavior-level tests precede each phase's runtime changes | Demonstrate semantic red then green. No skipped or placeholder assertions |
| IX Persistence | Memory and sqlx PostgreSQL repositories, next migration pair, retention queries and cleanup | PostgreSQL apply/rollback and both-adapter isolation tests |
| XI Design system | Rewritten principles/color guide, owned components, story gate, semantic lint | Every story in both themes, local assets, WCAG validation |
| XII Builder wiring | Config bootstrap, activity service, repositories, and cleanup owned by builder | Routes receive constructed handlers. Worker stops on shutdown |
| XIII E2E traceability | AS-01–AS-18 mapping below, Playwright journeys and screenshots | One canonical Ginkgo It per scenario, with real production bootstrap |

**Gate result**: The design introduces no unapproved runtime deviation. No technical clarification remains unresolved. ADR acceptance and detailed API approval are still mandatory before implementation. Do not mark those approvals complete merely because planning generated documents.

## Project Structure

### Design artifacts

```text
specs/046-redesign-consent-console/
  spec.md
  plan.md
  research.md
  data-model.md
  quickstart.md
  contracts/
    enduser-additions.openapi.yaml
    session-state.md
    ui-and-configuration.md
```

`tasks.md` belongs to the later task-generation command. This plan supplies its required four-phase ordering.

Additional planning outputs are ADR 037, rewritten DESIGN_PRINCIPLES.md and COLOR_GUIDE.md, an updated design-doc index, updated web/AGENTS.md, and labeled architecture/glossary additions.

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
  src/pages/                         Existing lazy pages plus Activity/Settings
  src/styles/                        Font faces and global token imports
internal/
  ports/config.go                    Temporary UI configuration contract
  config/                            Loader, environment and CLI integration
  domain/activity/                   Proposed independent user-history context
  domain/id/                         Generated ActivityEventID
  ports/storage.go                   Focused ActivityEventRepository
  adapters/storage/{memory,postgres}/ Activity repositories
  adapters/http/                     Activity handler and SPA bootstrap
  app/builder.go                     All service and worker wiring
api/enduser/openapi.yaml              Approved additions before implementation
migrations/                          Next numbered activity UP/DOWN pair
tests/e2e/frontend/                   Acceptance journeys and visual capture
tests/e2e/pages/                      Existing page-object conventions
tests/e2e/screenshots/                Reviewed light/dark baselines
tests/integration/                   Real PostgreSQL lifecycle coverage
.github/workflows/                   Blocking a11y and visual gates
charts/agentic-identity-broker/        Temporary flag delivery and removal
```

Use the existing component categories rather than a second library. The new activity context owns user-history semantics, retention, and safe event content. It is not a package around a single CRUD handler.

## Implementation Phase Overview

The four phases below are the requested task phases. They are distinct from this planning command's Phase 0 research and Phase 1 design.

### Phase 1 — Foundation

**Prerequisites**: Accept ADR 037. Review the proposed additive contracts before their related backend work. Write traceable acceptance journeys before changing their behavior.

Deliver semantic tokens, the first-paint theme script, CSS media fallback, color-scheme metadata, local fonts, outlined brand assets, Lucide, and raw-palette ESLint enforcement. Preserve the existing category paths and `cn()` convention. Remove the inert Tailwind JS configuration after moving its required live references into CSS-first tokens.

Copy and own Button, Badge, Card, Dialog, DropdownMenu, Popover, Switch, Tabs, Tooltip, Sonner, Table, Command, Input, Select, Separator, Skeleton, and Sheet. Migrate retained Checkbox, Radio, disclosure, and date controls needed by current flows.

Build ConsoleShell and DecisionShell with mobile Sheet and compact mark. Introduce the Query provider over Axios without changing authentication. Keep table and command dependencies lazy and outside decision bundles.

Establish the temporary `ui.v2` configuration through the port, loader, CLI, environment, builder, HTML bootstrap, examples, and Helm. Default false. Do not expose another API. Development HTML gets the same server-resolved value.

Configure Storybook 10 a11y/themes with one explicit light and one dark browser test project. Add a blocking CI gate. Include every new primitive, interaction state, and shell. Install the required browser binary in CI.

Update ADR, design documents, architecture, and local agent guidance with actual completed behavior. Move brand assets and update README/Docusaurus consumers together.

**Exit**: Owned primitives and shells pass keyboard and both-theme story checks. Raw palette examples fail lint. The first-paint script works under production CSP. The flag reflects broker precedence and both HTML serving modes. No application journey loses existing behavior.

### Phase 2 — Decision surfaces behind ui.v2

Migrate consent and approval review. Consent uses DecisionShell whenever `session_token` is present, including re-consent with an existing grant. Expired context remains a decision error.

Preserve authorization-session validation, `consent_state`, provider callback behavior, safe continuation, required selection locks, duration validation, and scope preview. Implement delta consent without a delta API. Keep Deny local and non-mutating.

Approval review displays agent, acting user, tool, risk, expandable arguments, and exact persistence scope. Approve once is the sole accent action. Resolved/expired approvals show an outcome without new actions.

Query-backed reads preserve fresh authorization-session semantics. Do not cache decision context as ordinary long-lived agent detail. Do not optimistically grant or approve.

**Exit**: AS-01–AS-05 pass for v2. Existing authorization, CIMD, selection-preservation, CSRF, and approval journeys still pass. Both themes meet the decision-route budget and no-external-request requirement. False flag still selects the original presentation.

### Phase 3 — Console and complete cutover

Migrate agents, management-context agent detail, connections, and approvals. Use TanStack Table and owned Table styles for the three lists. Add search, filters, truthful state, confirmed revocation, sticky draft controls, and pending-first approval triage.

Implement shared user-scoped pending refresh and accessible count announcements. Use record-scoped optimistic revoke with rollback. Required-scope aggregation, if approved, lands OpenAPI-first before the new connection state relies on it.

After all five existing routes and both agent contexts pass, remove the old presentation. Remove Headless UI, Framer Motion, duplicate UI wrappers, old palette exports, gradients, entry animation, Crimson Pro, Manrope, and unused preloads/licenses. Remove `apiCache` after all resource callers migrate. Do not leave compatibility aliases.

Remove `ui.v2` from the port, loader, CLI, environment, HTML, client selection, examples, Helm, tests, and configuration docs. Remove legacy lint exceptions. Update all detailed design guides, GettingStarted.mdx, and session component guides. Historical feature specifications remain historical, not current instructions.

**Exit**: AS-06–AS-15 pass. Five original routes have reviewed light/dark visual baselines. The built app has one presentation and no rollout flag. Query and Axios are the only remote-data path.

### Phase 4 — Activity, settings, command palette, screenshots, and docs

Promote the approved activity OpenAPI into the canonical contract first. Generate ActivityEventID, add the next migration pair, and implement the activity domain service, recorder, repositories, handler, and builder-owned cleanup worker.

Record grant changes, revocations, approval decisions, session disconnects, and authenticated token exchanges. Include attributable denied/failed attempts. Preserve original outcomes on activity-storage failure and retain independent security audit logging. Never store raw arguments or credentials.

Add the read-only activity route with filters and keyset pagination. Enforce ownership and 90-day bounds on every page. Delete expired records in bounded background batches. Add the agent Activity tab only when the source exists. Show last recorded use only when supported by events.

Add browser-only theme and persistence preferences with once as the default. Add Command/cmdk navigation across acting-user records and theme choices. A preference never submits a decision.

Publish README guidance and screenshots for all seven routes, including both agent contexts in both themes. Convert the existing advisory screenshot flow into reviewed baseline management plus a separate blocking comparison job. Finish all acceptance and architecture/config/API documentation updates.

**Exit**: AS-16–AS-18 and all earlier scenarios pass. Sixteen route/context/theme screenshots are documented. Activity cannot expose another user or expired data. Failure injection proves that activity storage does not change action results.

### Phase rules

No separate refactoring-only phase is needed. Keep required refactors with their consumer migration. Do not create a deployable empty-handler or repository-stub phase. Each phase ends with real behavior and its verification.

## Testing Strategy

### Canonical acceptance mapping

Use `tests/e2e/frontend/consent_ui_v2_test.go` as the planned canonical scenario file. Reuse page objects in `tests/e2e/pages/`. One Ginkgo `It` carries each exact scenario identifier. Theme parameterization stays inside that scenario rather than duplicating its identity.

| Scenario | Journey and principal assertion | Phase |
| --- | --- | --- |
| AS-01 | First consent identifies agent, warns on localhost, orders required groups, exposes scopes/risk and one Allow | 2 |
| AS-02 | Re-consent shows only new access and preserves prior grant selections | 2 |
| AS-03 | Duration persists through Allow/callback, while Deny and expiry make no new grant | 2 |
| AS-04 | Tool review exposes identity, arguments, risk, exact scope, and one accent action | 2 |
| AS-05 | Once/session/permanent decisions preserve scope, resolved/expired requests cannot resubmit | 2 |
| AS-06 | Search/filter agents, inspect truthful expiry/verification, confirm revoke | 3 |
| AS-07 | Empty delegation explanation and confirmed-revoke result, including rollback on failure | 3 |
| AS-08 | Console detail edits only optional groups, Cancel discards and Save persists | 3 |
| AS-09 | Overflow revoke names the agent, cancel preserves access, confirm affects only owner | 3 |
| AS-10 | Required-scope and token fixtures produce truthful connection states | 3 |
| AS-11 | Provider callback, supported refresh, and disconnect warning preserve token semantics | 3 |
| AS-12 | Pending-first triage and standing allow/deny revocation require explicit decisions | 3 |
| AS-13 | New pending request updates queue/count within 15 seconds without focus movement or gateway request | 3 |
| AS-14 | Sidebar and light/dark/system preferences persist without wrong-theme paint | 3 |
| AS-15 | Both shells remain usable by keyboard at narrow width and zoom with local branding | 3 |
| AS-16 | Two-user filtered activity, all event outcomes, retention, secret exclusion, and write-failure independence | 4 |
| AS-17 | Browser preferences survive reload but never bypass explicit approval or scope review | 4 |
| AS-18 | Keyboard palette navigates only acting-user records and changes theme | 4 |

Use production builder/bootstrap and fresh servers, storage, principals, and browser contexts. Reuse existing mock provider fixtures. Add deterministic clock/data fixtures for expired grants, required scopes, approval races, long metadata, and storage failure.

Tests must compile and fail for absent behavior before implementation. Never use skips, source-text assertions, mock echoes, or trivial failures. Keep existing journey assertions intact when behavior is preserved. Update selectors through page objects only.

### Unit and integration coverage

Keep useful Vitest/Testing Library coverage. Add tests for uncertain state boundaries: re-consent selection preservation, custom expiry, session-state precedence, unknown scope coverage, optimistic rollback concurrency, and identity cache clearing.

Add domain/storage tests for principal isolation, safe event projection, stable pagination under inserts, 90-day boundary, retention deletion, and original outcome preservation. Use real PostgreSQL for migration apply/rollback and repository integration. Memory and PostgreSQL must produce the same ordering and ownership results.

### Storybook and visual gates

Run every design-system story with light and dark globals and `a11y.test: 'error'`. Include overlay-open, error, loading, focus, and disabled states. Browser-mode testing is required for rendered contrast.

The existing Ginkgo/Playwright harness captures to `tests/e2e/frontend/coverage/screenshots/`. Reviewed baselines stay in `tests/e2e/screenshots/`. Reuse `tools/imgdiff` with the existing 0.5% threshold as the starting policy. Fail on missing/new/unreviewed baselines or excess difference. Publish expected, actual, and comparison artifacts. Never automatically approve the gate's own output.

Original-route minimum: `/delegations`, two `/agents/:id` contexts, `/sessions`, `/approvals`, `/approvals/:id`, each light/dark: 12 images. Activity/settings add four images. Add meaningful loading, empty, error, and overlay states without replacing route coverage.

### Runtime and performance proof

After implementation, run actual production-served routes, Storybook, and the full journeys. Observe focus, console/network errors, CSP, callback results, preference persistence, and reduced motion. Tests do not replace this smoke proof.

Measure the initial consent dependency graph after compression, not the whole app total. Measure content time under throttled 4G and exclude console-only chunks. Verify row density and overflow at specified viewports. SC-001 and SC-007 need moderated user sessions, not automated timing claims.

Commands and expected outcomes are in [quickstart.md](quickstart.md). New gate recipes are explicitly marked as planned there.

## Complexity Tracking

No constitutional violation is waived. The temporary second presentation is explicitly requested and ends in phase 3. It does not duplicate authorization logic.

ADR 037 narrowly replaces the UI-components/animation clause and server-cache ownership in ADR 006. It keeps local UI state in React and keeps Axios. Its route extension preserves ADR 035's root mounting and protocol precedence.

API approval cannot be inferred from a generated proposal. Local consent cancellation cannot enter server history without a prohibited extra API. This limitation is explicit in the data model and contract.
