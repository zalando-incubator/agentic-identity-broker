# UI and Single-Cutover Contracts

**Status**: Proposed design. No runtime implementation is included.

## Routes and shells

| Address | Context | Shell |
| --- | --- | --- |
| `/` | Entry | Replace history with `/delegations` |
| `/delegations` | Agent grants | ConsoleShell |
| `/agents/:id` | Validated authorization session | DecisionShell |
| `/agents/:id` | Grant management without authorization session | ConsoleShell |
| `/sessions` | Third-party connections | ConsoleShell |
| `/approvals` | Pending and standing decisions | ConsoleShell |
| `/approvals/:id` | Tool decision or resolved result | DecisionShell |
| `/settings` | Browser preferences | ConsoleShell |

An invalid or expired authorization session stays an error in the decision flow. It must not silently become an editable management view. Only the backend validates authorization context.

ConsoleShell owns the collapsible sidebar, mobile Sheet, page heading, purpose text, pending count, and user menu. DecisionShell has no sidebar. Its content width is at most 640 px.

All route components remain lazy. Import concrete component modules where a barrel would pull Table or Command into decision bundles.

## Primary actions

Each view shows at most one accent (`primary` variant) action. Row, menu, and dialog-cancel actions use non-accent variants.

| View | Accent action |
| --- | --- |
| `/delegations` | None |
| `/agents/:id` decision | Allow |
| `/agents/:id` console | Save changes, only in the edit bar while the draft has unsaved changes |
| `/sessions` | None |
| `/approvals` | None |
| `/approvals/:id` pending | Approve once |
| `/approvals/:id` resolved or expired | None |
| `/settings` | None; a theme change applies immediately |

An open modal dialog hides the page behind it from assistive technology. Its confirm button is then the view's single accent action, or uses the `destructive` variant for a destructive confirmation.

## Single cutover

Deliver all routes, shells, preferences, and command search in one release.
Do not introduce implementation phases, feature flags, parallel presentations, or backwards-compatibility layers.
Migrate every caller and remove obsolete components, tokens, aliases, fonts, animations, and caches together.

No redesign configuration belongs in the broker port, environment, CLI, Helm chart, or HTML bootstrap.
Do not add `ui.v2`, a browser flag, a build flag, or a configuration endpoint.
Browser preferences select appearance, never an old presentation.
The first-paint theme script remains necessary. It contains no principal, credential, or broker configuration.

The feature adds or changes no API response. Every screen reads existing end-user contracts.

## Theme contract

Browser storage keys are `aib.theme` and `aib.sidebar-collapsed`. These contain only appearance and UI preferences.

`aib.theme` accepts `light`, `dark`, or `system`. Missing or invalid values use system. If storage is unavailable, continue with an in-memory preference. The inline first-paint script and React provider share these rules.

The script sets a resolved `data-theme` on the document element before styles paint. The source preference remains separate from that resolved attribute. System mode listens for OS changes. Explicit choices ignore OS changes.

Add color-scheme metadata and CSS. Scope the CSS media fallback to a root without a resolved attribute. Portals and native controls inherit the resolved theme.

CSP uses `font-src 'self'`. The fixed theme script has a build-generated SHA-256 CSP hash. Never enable `unsafe-inline` to make it work. Preserve existing OAuth2 navigation and API behavior. Brand assets and fonts are same-origin.

## Data and mutation contract

Query keys start with the authenticated principal returned by the existing user endpoint. Lists and details add their resource identifiers and filters. Never infer identity from editable storage.

Axios remains the transport and error-normalization owner. Query functions forward AbortSignal. On identity change or authentication loss, cancel requests and clear principal-owned cached data before showing another principal's content.

Do not persist API data to localStorage. Move service caches and hook loading state into Query without double caching. Remove `apiCache` after all callers migrate.

For revocation, require confirmation before mutation. Cancel matching reads and mark the affected row pending. Preserve rollback data for that record. Do not restore an entire old list over newer results from unrelated mutations. Disable a second action on the same record. Invalidate affected lists, details, and pending count after settlement. Announce success only after the server accepts revocation.

Approval, grant creation, and grant edits wait for authoritative server outcomes. Never replay a security mutation automatically after an error.

One console-level query refreshes `/api/approvals/pending` every 10 seconds. Sidebar and queue share it. Refresh immediately after a decision and when focus returns. Suspend unnecessary background work when the document is hidden. Network failure shows stale status, not a fabricated zero count.

No browser request reaches `GET /api/approvals`, the gateway-wide long-poll. No additive API is authorized.

## Decision invariants

Delta consent compares requested selections with the existing `granted_permission_sets`. Preserve prior groups and selected services. Previously granted groups are read-only in the decision view. Required groups and services remain locked. Never request an API delta computation.

The re-consent duration starts from the existing grant: Until revoked for a null `valid_until`, otherwise Custom date. Unless the user changes it, submission sends the existing `valid_until` unchanged. A changed duration applies to the whole grant.

Keep the current authorization-session parameter, expiry, callback, scope-preview, and continuation checks. Deny leaves existing grants untouched. Without an existing validated cancellation path, show a local cancelled outcome instead of inventing an OAuth2 redirect.

Grant durations remain Until revoked, 30 days, and a validated custom date. Tool persistence remains once, session, or permanent with its existing behavior. No saved default preselects it. Every remembered decision shows its exact scope before confirmation.

## Component and validation contract

Use all primitives named in ADR 037, plus retained controls needed by current flows. Use semantic tokens and owned source. The raw-palette ESLint rule covers className, `cn`, `clsx`, CVA, and variant-prefixed utilities.

Pages and application components take every user-facing string from the copy catalogue in `web/src/copy/`. Design-system components receive user-facing strings through props and do not import the catalogue.

Every component story runs with light and dark globals. Storybook uses the themes addon with `data-theme` and the a11y addon with `test: 'error'`. Two explicit browser-test projects run the same story set, one per theme. A toolbar switch alone does not constitute CI coverage.

Every story also has a visual regression check in each theme project (Principle XI). A Vitest-only project annotation compares the rendered story root with a reviewed baseline through Vitest browser-mode `toMatchScreenshot`. Baselines are Linux Chromium images keyed by story ID and theme. CI never runs with `--update` and fails on a missing or changed image.

Playwright baselines cover all six routes in both themes, including both `/agents/:id` contexts. The minimum matrix contains 14 route/context/theme images.

Store screenshots under `tests/e2e/screenshots/`. Pin browser version, OS image, fonts, viewport, data, and time. Wait for fonts and query settlement. Compare pixels and publish expected, actual, and diff images on failure. Do not accept changed baselines automatically.

The route gate compares the 14 required images plus the state images listed in `tests/e2e/screenshots/visual-gate.txt`. Other journey screenshots remain documentation. No workflow writes to or commits `tests/e2e/screenshots/` or the Storybook baselines; automation uploads candidates as a review artifact, and every baseline changes only in a reviewed commit.

WCAG 2.2 AA, 320 px, 200% zoom, focus, reduced motion, theme persistence, and zero automatic third-party requests require browser journeys beyond screenshot comparison.
