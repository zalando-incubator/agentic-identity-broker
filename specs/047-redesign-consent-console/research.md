# Research — Consent UI v2

**Branch**: `047-redesign-consent-console`
**Status**: Technical choices resolved. ADR acceptance remains the implementation gate. The feature adds no API.

## 1. Retain the platform, replace the component layer

**Decision**: Keep the installed React 19, TypeScript, Vite 7, Tailwind 4, Router 7, Axios, CVA, `tailwind-merge`, Vitest, Testing Library, Storybook 10, and Ginkgo stack. Copy the Radix versions of shadcn components into the existing design-system categories.

**Rationale**: `web/package.json` already contains the retained stack. The design system already supplies aliases, categories, and `cn()`. Current shadcn documentation supports React 19, Tailwind 4, OKLCH, and `@theme inline`. Explicitly select Radix rather than another registry base.

**Alternatives considered**: Retaining Headless UI does not meet the request. A second `components/ui` library duplicates component ownership. Replacing Router or Axios adds unrelated scope.

**Evidence**: `web/package.json:9-65`, `web/AGENTS.md`, [shadcn Tailwind v4](https://ui.shadcn.com/docs/tailwind-v4), [manual installation](https://ui.shadcn.com/docs/installation/manual). Context7 returned current source guidance on 2026-09-25.

## 2. Tokens, accent, and enforcement

**Decision**: Select neutral blue as the primary accent. Define complete semantic OKLCH light/dark values under `design-system/tokens/`. Map them with Tailwind `@theme inline`. Add a raw-palette ESLint rule rather than relying on a source allow-list.

**Rationale**: Blue distinguishes the primary action from warning meaning. `web/src/styles/index.css` imports CSS tokens, while the legacy `tailwind.config.ts` is not loaded with `@config`. Consolidating CSS-first tokens removes that conflicting source. ESLint can produce actionable errors for palette classes and arbitrary literals.

**Alternatives considered**: Orange is permitted by the spec but overlaps warnings. A source allow-list can omit generated CSS without identifying the invalid class. A dark-only inversion does not define complete component contrast.

**Evidence**: `web/src/design-system/tokens/colors.css`, `web/src/styles/index.css`, `web/tailwind.config.ts`, [color contract](../../web/src/design-system/docs/COLOR_GUIDE.md).

**Planning measurement**: A throwaway OKLCH-to-linear-sRGB calculation validated 74 text/control pairs. Minimum text contrast was 5.70:1 in light and 7.46:1 in dark. Minimum control contrast was 3.15:1 and 3.63:1. Fourteen filled-status pairs also passed, with minimum ratios of 6.60:1 and 9.32:1. These are opaque token calculations, not rendered accessibility results.

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

**Decision**: Use every primitive named in ADR 037. Use Lucide icons, TanStack Table for agents/connections/approvals, Sonner for notifications, and cmdk through Command. Remove Framer Motion. Use CSS transitions and `@starting-style` with 120–200 ms ease-out timing.

**Rationale**: Current shadcn guidance replaces its old toast with Sonner. Table and Command remain headless behavior with owned presentation. No required interaction needs a shared-layout animation.

**Alternatives considered**: Handwritten table state and keyboard command matching duplicate mature libraries. Keeping Framer Motion for page entry conflicts with FR-005. A generic application-wide state library is not needed.

**Integration**: Keep React state for drafts and context for appearance. Keep Table and Command outside decision-route imports. Preserve selection controls and native date validation even though they extend the named primitive inventory.

**Evidence**: `web/src/design-system/components/`, `web/src/components/ui/`, [shadcn data table](https://ui.shadcn.com/docs/components/data-table), [Command](https://ui.shadcn.com/docs/components/command), [Sonner](https://ui.shadcn.com/docs/components/sonner).

## 6. Query lifecycle and pending count

**Decision**: TanStack Query owns remote caching over the existing Axios services. Only confirmed revocation gets an optimistic pending state. Use record-scoped rollback and settlement invalidation. Grant and approval decisions remain server-confirmed.

**Rationale**: `apiCache` uses 5-minute and 2-minute TTLs with a module cleanup timer. Approvals currently fetch once on page mount. A shared pending query removes duplicate sidebar and page fetch loops.

**Security resolution**: Poll `/api/approvals/pending` every 10 seconds while the console is visible, plus immediate focus and mutation refresh. Do not use ADR 014's gateway long-poll. ADR 018 and spec API-004 explicitly prohibit that browser source. This is a deliberate deviation from the user's long-poll shorthand, not a new endpoint.

**Alternatives considered**: Two cache layers can return stale data after mutation. Optimistic approval implies authorization before the server accepts it. A pending-count endpoint exceeds API scope. A 15-second timer leaves no response-time allowance for the 15-second requirement.

**Evidence**: `web/src/services/api/cache.ts`, `consent.ts`, `sessions.ts`, `approvals.ts`, `web/src/pages/ToolAuthorizationsPage.tsx:273-292`, ADRs 014 and 018, [TanStack optimistic updates](https://tanstack.com/query/latest/docs/framework/react/guides/optimistic-updates).

## 7. Shell selection and existing consent behavior

**Decision**: The presence of `session_token` selects decision context on `/agents/:id`. Backend validation determines whether that context is valid. Existing grants do not change shell selection. Approval review always uses DecisionShell.

**Rationale**: `AgentGrantDetailPage.tsx` reads `session_token` and preserves selections through `consent_state` during service authorization. Delta consent still needs a focused decision even with an existing grant.

**Deny**: No consent-deny backend endpoint exists. Deny shows a local terminal result and performs no grant mutation or constructed redirect. Preserve existing validated continuation for Allow.

**Alternatives considered**: Choosing a shell from grant existence breaks re-consent. A new OAuth2 denial callback or telemetry write endpoint exceeds the authorized API scope.

**Evidence**: `web/src/App.tsx:38-75`, `web/src/pages/AgentGrantDetailPage.tsx:47-64,255-264`, `internal/domain/oauth2/service.go:479-481`, `tests/e2e/frontend/selection_preservation_test.go`.

## 8. Single cutover

**Decision**: Deliver the complete redesign in one release. Do not add implementation phases, feature flags, or backwards-compatibility layers.

**Rationale**: The user's 2026-09-26 revision replaces the earlier rollout proposal. All routes and consumers migrate together.
Remove old components, tokens, aliases, fonts, animations, and caches in the same change.
Do not add broker configuration, environment or CLI bindings, Helm values, or HTML flag delivery.

**Alternatives considered**: A staged presentation switch creates a second system to maintain. Old-server fallbacks preserve a contract that this release does not support.

**Evidence**: The revised FR-028 and [UI contract](contracts/ui-and-configuration.md) define the release boundary. Authorization behavior remains unchanged.

## 9. Connection state from existing session fields

**Decision**: Derive Connected, Needs re-authentication, Expired, and No connection from the existing session response only. Add no session field, backend state, or Missing scopes state.

**Rationale**: The 2026-09-26 clarification limits this feature to visual and UX changes. Existing session reads expose granted scopes, access and refresh expiry, and refresh-token presence. They do not expose required scope coverage, so the UI must not claim a scope gap.

**Alternatives considered**: Inferring requirements from available provider scopes labels valid sessions incorrectly. Fetching every agent detail to reconstruct requirements adds browser fan-out for a state this feature does not show.

**Evidence**: `api/enduser/openapi.yaml`, `web/src/components/sessions/SessionCard.tsx`, `internal/domain/oauth2session/service.go`.

## 10. Storybook and visual gates

**Decision**: Keep Storybook 10 and its a11y/themes addons. Add the matching Vitest addon with browser-mode projects for light and dark. Set global `parameters.a11y.test = 'error'`. Reuse Ginkgo/Playwright and `tools/imgdiff` for a blocking route snapshot gate.

**Rationale**: Storybook currently has no CI gate. The themes addon is registered but not wired. Existing screenshot automation is advisory and updates tracked images. It is not a regression gate.

**Alternatives considered**: A themes toolbar alone exercises only one theme in CI. jsdom cannot validate rendered contrast. A second JavaScript Playwright E2E suite duplicates existing infrastructure. Automatic baseline acceptance masks visual regressions.

**Implementation**: Install Chromium for the Storybook browser projects. Pin the existing 10.3.5 addon family together. Parameterize every story in both themes. Keep the current screenshot capture path and promote only reviewed baselines into `tests/e2e/screenshots/`.

**Story visual regression**: Principle XI requires visual regression testing for design-system stories. No story-level tooling exists today. Vitest 4.1 browser mode provides `toMatchScreenshot` with a pixelmatch comparator, and Storybook runs project-level `afterEach` annotations after each story renders. A Vitest-only annotation in `web/.storybook/vitest.setup.ts` therefore compares every story root in both theme projects without affecting the Storybook UI. Baselines are Linux Chromium images reviewed like route baselines. Cloud visual-testing services add an external dependency and are not needed.

**Evidence**: Context7 on 2026-09-27 returned Vitest v4.1.6 `toMatchScreenshot` and `browser.expect.toMatchScreenshot` configuration, and Storybook v10.2.9 project-level `afterEach`. Validate against the installed 10.3.5 release during foundation work.

**Evidence**: `web/.storybook/main.ts`, `preview.ts`, `web/package.json`, `.github/workflows/ci.yml`, `.github/workflows/screenshots.yml`, `tests/e2e/frontend/frontend_suite_test.go`, `tests/e2e/pages/page.go`, [Storybook accessibility testing](https://storybook.js.org/docs/writing-tests/accessibility-testing). Context7 verified the `error` gate using Storybook 10.2.9 documentation. Validate integration against the installed 10.3.5 release during foundation work.

## Resolved unknowns and remaining approvals

No technical choice remains marked NEEDS CLARIFICATION. The plan supplies concrete defaults and contracts. ADR acceptance is a governance gate, not an assumed result. The feature adds no API contract. Planning does not authorize runtime changes.
