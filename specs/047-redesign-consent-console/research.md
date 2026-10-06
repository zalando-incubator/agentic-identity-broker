# Research — Consent UI v2

**Branch**: `047-redesign-consent-console`
**Status**: ADR 038 accepted 2026-09-27 and amended by five approved stakeholder choices on 2026-10-04. The branch implements these decisions; `cutover-inventory.md` records the validation evidence. No end-user API change. With the route rename, the broker callback sends error and missing-return redirects to `/connections`.

Evidence entries cite the sources that motivated each decision. The cutover removed several of them: `colors.css`, `tailwind.config.ts`, `cache.ts`, `ToolAuthorizationsPage.tsx`, and `SessionCard.tsx`.

## 1. Retain the platform, replace the component layer

**Decision**: Keep the installed React 19, TypeScript, Vite 7, Tailwind 4, Router 7, Axios, CVA, `tailwind-merge`, Vitest, Testing Library, Storybook 10, and Ginkgo stack. Copy the Radix versions of shadcn components into the existing design-system categories.

**Rationale**: `web/package.json` already contains the retained stack. The design system already supplies aliases, categories, and `cn()`. Current shadcn documentation supports React 19, Tailwind 4, OKLCH, and `@theme inline`. Explicitly select Radix rather than another registry base.

**Alternatives considered**: Retaining Headless UI does not meet the request. A second `components/ui` library duplicates component ownership. Replacing Router or Axios adds unrelated scope.

**Evidence**: `web/package.json:9-65`, `web/AGENTS.md`, [shadcn Tailwind v4](https://ui.shadcn.com/docs/tailwind-v4), [manual installation](https://ui.shadcn.com/docs/installation/manual). Context7 returned current source guidance on 2026-09-25.

## 2. Tokens, accent, and enforcement

**Decision, amended 2026-10-04**: Keep blue as the sole accent and semantic OKLCH light/dark tokens. Tint neutral surfaces at hue 260. Use separate `--border-subtle` for card edges/dividers, `--border` for buttons/popovers, and `--border-control` for form controls meeting 3:1. Add `--primary-soft` for active navigation/selection, and soft status fills with readable labels. Keep the raw-palette ESLint rule.

**Rationale**: Blue distinguishes the primary action from warning meaning. `web/src/styles/index.css` imports CSS tokens, while the legacy `tailwind.config.ts` is not loaded with `@config`. Consolidating CSS-first tokens removes that conflicting source. ESLint can produce actionable errors for palette classes and arbitrary literals.

**Alternatives considered**: Orange is permitted by the spec but overlaps warnings. A source allow-list can omit generated CSS without identifying the invalid class. A dark-only inversion does not define complete component contrast.

**Evidence**: `web/src/design-system/tokens/colors.css`, `web/src/styles/index.css`, `web/tailwind.config.ts`, [color contract](../../web/src/design-system/docs/COLOR_GUIDE.md).

**Historical planning measurement (2026-09-27)**: Earlier opaque-token arithmetic checked 74 pairs for the previous palette. It does not validate the retuned border roles or rendered components. Expand `contrast.test.ts` and review both themes before accepting the new palette.

## 3. First-paint theme and CSP

**Decision**: Store light/dark/system preference per browser. A fixed inline script resolves it before paint and sets `data-theme`. Use an exact CSP hash, color-scheme metadata, and a CSS media fallback only without a resolved attribute.

**Rationale**: A React effect runs too late to prevent wrong-theme paint. Explicit light must override a dark OS. System mode must keep following OS changes. The current SPA CSP only sets `frame-ancestors 'none'`.

**Alternatives considered**: A React-only initializer risks a flash. `unsafe-inline` for scripts weakens CSP unnecessarily. Browser storage holds preferences, not authorization.

**Design boundary**: Production fonts, scripts, images, and connections use same-origin sources. Preserve frame protection and safe OAuth2 top-level navigation. Radix positioning can use inline style attributes. Validate those separately instead of blindly adding a restrictive style policy that breaks overlays. Do not broaden `script-src`.

**Evidence**: `internal/adapters/http/handlers/spa.go:32-35`, `web/index.html`, [UI contract](contracts/ui-and-configuration.md).

## 4. Fonts and brand

**Decision**: Use normal-width Zalando Sans Variable, Inter Variable, and JetBrains Mono Variable. Source Zalando Sans through `@fontsource-variable/zalando-sans`. Copy required language subsets and licenses into `web/public/fonts/`. Preload decision-route display and body subsets. Convert supplied wordmark and favicon text to outlines.

**Rationale**: The SVG files in `assets/docusaurus/static/img/` contain live text and a Bunny Fonts import. Outlines remove font requests and preserve deterministic artwork. The two wordmark weights remain 300 and 700. Inter provides the neutral body face requested by the spec.

**Alternatives considered**: Live HTML branding is accessible but changes supplied artwork metrics. Keeping SVG text can vary between renderers. Geist is viable, but a second body-face choice adds no requirement. SemiExpanded adds bytes without a stated need.

**Asset move**: Move black/white wordmarks and favicon into `web/public/brand/`. Update README and Docusaurus consumers and their asset-copy configuration. Do not leave stale paths or duplicate authoritative artwork. Derive the compact sidebar mark from outlined favicon artwork. Supply theme-appropriate foregrounds.

**Evidence**: `web/src/styles/fonts.css`, `web/public/fonts/`, `web/index.html`, the three SVGs under `assets/docusaurus/static/img/`, [Fontsource Zalando Sans](https://fontsource.org/fonts/zalando-sans/use).

## 5. Components, tables, command search, and motion

**Decision, amended 2026-10-04**: Keep owned shadcn/Radix components, Lucide, Sonner, cmdk and TanStack Query. Replace the three TanStack data tables with `EntityCard`/`EntityRow` compositions: cards plus list toggle for Agents and Connections, stable inbox rows with a separate approval detail panel. Retire `InlineApprovalActions` and its expanding row. Use Motion and owned lucide-animated icons only in console routes; author decision-route feedback as CSS/SVG. Keep one motion constants module, reduced-motion support and the no-Motion decision-bundle gate.

**Rationale**: A card makes state visible for a small mixed set; the list remains useful for scanning large or similar sets. Approval controls belong in a stable detail region, not an Actions cell that changes row height. Console list reflow can show removed entities, while decision routes keep the measured bundle limit and visual focus.

**Alternatives considered**: A table-first console hides state in columns, and in-row persistence editing moves controls under the pointer. CSS-only motion everywhere was the earlier decision, superseded for console routes by the stakeholder. A second component library remains unnecessary.

**Integration**: Keep React state for drafts and context for appearance. Use the shared `PermissionPanel` and `DurationSelect` in consent and agent detail. Keep console Motion and Command outside decision imports, preserve selection controls and native date validation, and verify decision bundles after implementation.

**Evidence**: [Approved UX improvement plan](ux-improvement-plan.md), [ADR 038](../../adrs/038-design-system-rebuilt-on-shadcn-radix.md), and current owned `web/src/design-system/components/` sources. The reviewed plan describes intended changes, not rendered validation.

**Dependency pin (2026-10-04)**: The console dependency is `motion@12.30.0`, installed with React 18/19 compatibility. The registry package-age gate excludes the latest Motion 14 release. This is an installation record, not evidence that animations, reduced motion, or bundle checks pass.

## 6. Query lifecycle and pending count

**Decision**: TanStack Query owns remote caching over the existing Axios services. Only confirmed revocation gets a pending state, on the affected record only. The record leaves the collection after the server accepts the revocation and stays on failure. Invalidate affected queries on settlement. Grant and approval decisions remain server-confirmed.

**Rationale**: Before the cutover, `apiCache` used 5-minute and 2-minute TTLs with a module cleanup timer, and Approvals fetched once on page mount. A shared pending query removes duplicate sidebar and page fetch loops.

**Security resolution**: Poll `/api/approvals/pending` every 10 seconds while the console is visible, plus immediate focus and mutation refresh. Do not use ADR 014's gateway long-poll. ADR 018 and spec API-004 explicitly prohibit that browser source. This is a deliberate deviation from the user's long-poll shorthand, not a new endpoint.

**Alternatives considered**: Two cache layers can return stale data after mutation. Optimistic approval implies authorization before the server accepts it. A pending-count endpoint exceeds API scope. A 15-second timer leaves no response-time allowance for the 15-second requirement.

**Evidence**: `web/src/services/api/cache.ts`, `consent.ts`, `sessions.ts`, `approvals.ts`, `web/src/pages/ToolAuthorizationsPage.tsx:273-292`, ADRs 014 and 018, [TanStack optimistic updates](https://tanstack.com/query/latest/docs/framework/react/guides/optimistic-updates).

## 7. Shell selection and existing consent behavior

**Decision**: The presence of `session_token` selects decision context on `/agents/:id`. Backend validation determines whether that context is valid. Existing grants do not change shell selection. Approval review always uses DecisionShell.

**Rationale**: `AgentRoute` restores a matching tab-local callback draft before choosing decision or console context. The current form POST sends only a bounded UUID and clean return path. Delta consent remains focused even with an existing active grant.

**Deny**: No consent-deny backend endpoint exists. Deny shows a local terminal result and performs no grant mutation or constructed redirect. Preserve existing validated continuation for Allow.

**Alternatives considered**: Choosing a shell from grant existence breaks re-consent. A new OAuth2 denial callback or telemetry write endpoint exceeds the authorized API scope.

**Evidence**: `web/src/components/layout/AgentRoute.tsx`, `web/src/services/storage/session.ts`, `web/src/components/consent/startServiceLogin.ts`, and `tests/e2e/frontend/selection_preservation_test.go`; accepted transport ADR 039 and Feature 008 amendment. `startServiceLogin` uses the form POST when the draft or return URL carries state, and the existing GET authorize link otherwise.

## 8. Single cutover

**Decision**: Deliver the complete redesign in one release. Do not add implementation phases, feature flags, or backwards-compatibility layers.

**Rationale**: The user's 2026-09-26 revision replaces the earlier rollout proposal. All routes and consumers migrate together.
Remove old components, tokens, aliases, fonts, animations, and caches in the same change.
Do not add broker configuration, environment or CLI bindings, Helm values, or HTML flag delivery.

**Alternatives considered**: A staged presentation switch creates a second system to maintain. Old-server fallbacks preserve a contract that this release does not support.

**Evidence**: The revised FR-028 and [UI contract](contracts/ui-and-configuration.md) define the release boundary. Authorization behavior remains unchanged.

## 9. Connection state from existing session fields

**Decision**: Derive Connected, Needs re-authentication, Expired, Unavailable, and No connection from the existing session response, refresh results in the current page, and agent requirement status only. Unavailable marks a failed read or a refresh failure without an authoritative rejection. Add no session field, backend state, or Missing scopes state.

**Rationale**: The 2026-09-26 clarification limits this feature to visual and UX changes. Existing session reads expose granted scopes, access and refresh expiry, and refresh-token presence. They do not expose required scope coverage, so the UI must not claim a scope gap.

**Alternatives considered**: Inferring requirements from available provider scopes labels valid sessions incorrectly. Fetching every agent detail to reconstruct requirements adds browser fan-out for a state this feature does not show.

**Evidence**: `api/enduser/openapi.yaml`, `web/src/components/sessions/SessionCard.tsx`, `internal/domain/oauth2session/service.go`.

## 10. Storybook and visual gates

**Decision**: Keep Storybook 10 and its a11y/themes addons. Add the matching Vitest addon with browser-mode projects for light and dark. Set global `parameters.a11y.test = 'error'`. Reuse Ginkgo/Playwright and `tools/imgdiff` for a blocking route snapshot gate.

**Historical rationale (2026-09-27)**: Storybook had no CI gate then. The themes addon was registered but not wired, and screenshot automation updated tracked images without a blocking regression check. These observations do not describe the amended UI's present validation status.

**Alternatives considered**: A themes toolbar alone exercises only one theme in CI. jsdom cannot validate rendered contrast. A second JavaScript Playwright E2E suite duplicates existing infrastructure. Automatic baseline acceptance masks visual regressions.

**Implementation**: Install Chromium for the Storybook browser projects. Pin the existing 10.3.5 addon family together. Parameterize every story in both themes. Keep the current screenshot capture path and promote only reviewed baselines into `tests/e2e/screenshots/`.

**Story visual regression design, amended 2026-10-06**: Principle XI requires visual regression, not a six-way pixel matrix. [ADR 040](../../adrs/040-canonical-light-visual-reference-gate.md) limits PNG capture and comparison to desktop `storybook-light`, effective light theme, and an actual 1280 × 720 viewport. All six projects keep interactions and blocking accessibility checks. Intentional dark or narrow overrides receive no pixel reference. The Vitest-only annotation compares the body, including portal overlays. Human reviewers approve Linux Chromium references. The earlier both-theme reference design remains historical.

**Evidence**: Context7 on 2026-09-27 returned Vitest v4.1.6 `toMatchScreenshot` and `browser.expect.toMatchScreenshot` configuration, and Storybook v10.2.9 project-level `afterEach`. Validate against the installed 10.3.5 release during foundation work.

**Evidence**: `web/.storybook/main.ts`, `preview.ts`, `web/package.json`, `.github/workflows/ci.yml`, `.github/workflows/screenshots.yml`, `tests/e2e/frontend/frontend_suite_test.go`, `tests/e2e/pages/page.go`, [Storybook accessibility testing](https://storybook.js.org/docs/writing-tests/accessibility-testing). Context7 verified the `error` gate using Storybook 10.2.9 documentation. Validate integration against the installed 10.3.5 release during foundation work.

## 11. Approved presentation amendment (2026-10-04)

**Decision**: The stakeholder approved cards by default with a list toggle for Agents and Connections; Motion on console routes but CSS only on decision routes; a consent primary Connect action for missing selected services; `/agents` and `/connections` instead of `/delegations` and `/sessions`; and “Signed in as” on consent. Keep ADR 038's 2026-09-27 accepted status and record this amendment there. Route `/` to `/agents`, move remembered decisions to `/approvals/remembered`, and open Settings at `/settings/appearance` from the user menu.

**Rationale**: The earlier interface was a sound technical base, but full-width tables, fixed-width consent stacks, bright neutral borders and expanding inline approvals obscured decisions. The amended layout uses a centered 1120 px console, one 480 px consent card, an approval detail panel and three border roles. Soft semantic statuses keep the accent reserved for the current decision.

**Alternatives considered**: Keeping `/delegations` and `/sessions` as aliases leaves two active view maps. Adding a backend callback endpoint changes a prohibited API boundary. A theme-only Settings page duplicates the user menu without offering a default collection preference.

**Token targets**: Dark background `oklch(0.175 0.006 260)`, card `oklch(0.21 0.008 260)`, raised popover `oklch(0.245 0.009 260)`, subtle border `oklch(0.275 0.01 260)`, button border `oklch(0.33 0.012 260)`, control border `oklch(0.54 0.012 260)`, and primary `oklch(0.70 0.15 262)`. Light background `oklch(0.985 0.003 260)`, muted `oklch(0.965 0.005 260)`, subtle border `oklch(0.93 0.006 260)`, button border `oklch(0.88 0.008 260)`, control border `oklch(0.60 0.012 260)`, and primary `oklch(0.50 0.19 262)`. These are starting values, not measured acceptance. See [the plan's token guidance](ux-improvement-plan.md) for the remaining text, selection and status pairs.

**Validation design**: Add pure page `*View` stories with shared fixtures and no network mock. Cover long content, error/stale, approvals risk/scope, consent callback states and 375/768/1280 px in both themes. Assert stable list bounds and the consent footer at 1280 × 720. Keep existing principal-query, approval, callback, CSP, authorization and draft tests.

## Resolved unknowns and remaining approvals

No technical choice remains marked NEEDS CLARIFICATION. ADR 038 and the 2026-10-04 amendment have explicit stakeholder acceptance. The existing callback-state transport and existing-response documentation corrections remain approved. No end-user API contract changes; the broker callback's error and missing-return redirect follows the route rename. The branch implements the amendment. The inventory records verification limits. The feature uses normal repository CI, with no separate release-approval process.
