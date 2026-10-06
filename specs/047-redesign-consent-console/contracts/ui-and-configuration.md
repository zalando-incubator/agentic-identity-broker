# UI and Single-Cutover Contracts

**Status**: Approved design contract, amended 2026-10-04 and aligned with the branch implementation on 2026-10-05. `cutover-inventory.md` holds validation evidence. `plan.md` describes implementation and normal CI verification.

## Routes and shells

| Address | Context | Shell |
| --- | --- | --- |
| `/` | Entry | Replace history with `/agents` |
| `/agents` | Unexpired agent grants, cards or list | ConsoleShell |
| `/agents/:id` | Validated authorization session | DecisionShell, one 480 px consent card |
| `/agents/:id` | Grant management without authorization session | ConsoleShell, 8 + 4 detail grid |
| `/connections` | Stored third-party connections, cards or list | ConsoleShell |
| `/approvals` | Pending inbox with separate detail panel | ConsoleShell |
| `/approvals/remembered` | Standing allow and deny decisions | ConsoleShell |
| `/approvals/:id` | Standalone tool decision or resolved result | DecisionShell |
| `/settings/appearance` | Browser preferences | ConsoleShell |

An invalid or expired authorization session stays an error in the decision flow. It must not silently become an editable management view. Only the backend validates authorization context.

ConsoleShell owns the three-item Agents, Connections and Approvals sidebar, mobile Sheet, page header, pending count and user menu. The user menu holds Settings, the theme choice and a Documentation link. Ctrl/⌘ B collapses the desktop sidebar, and Ctrl/⌘ K opens command search. Console content is centered to 1120 px on 12 columns with 24 px gutters and 24 px side padding (16 px below 768 px). DecisionShell has no sidebar. It groups the full local wordmark directly above the focused card and has no Powered by footer. On consent, the existing authenticated identity appears once as “Signed in as {name}” outside and beneath the card, without a second letter avatar. Consent content is one 480 px card with a pinned decision footer; below 640 px it is full bleed and reserves space for branding/account context. The standalone approval review retains its wider column.

The `/approvals/remembered` static route takes precedence over `/approvals/:id`. `/delegations`, `/sessions` and `/settings` are removed view routes; no aliases or redirects remain. All destination components remain lazy. Do not import console Motion, Command or collection modules into decision graphs.

## Primary actions

Each view shows at most one accent (`primary` variant) action. Row, menu, and dialog-cancel actions use non-accent variants.

| View | Accent action |
| --- | --- |
| `/agents` | None |
| `/agents/:id` decision | Allow; if a selected service lacks a connection, "Connect {Service} to continue" uses the same slot until callback return |
| `/agents/:id` console | Save changes, only while the draft is dirty |
| `/connections` | None |
| `/approvals` selected request | Selected approval action in the detail panel, initially Approve once, not in a list row |
| `/approvals/remembered` | None |
| `/approvals/:id` pending | Selected approval action, initially Approve once |
| `/approvals/:id` resolved or expired | None |
| `/settings/appearance` | None; preferences apply immediately |

An open modal dialog hides the page behind it from assistive technology. Its confirm button is then the view's single accent action, or uses the `destructive` variant for a destructive confirmation.

## Presentation and interaction contract

- Console grid: cards use `repeat(auto-fill, minmax(320px, 1fr))` with a 16 px gap. Agent detail spans 8 + 4 columns and the approval inbox 5 + 7. Below 1012 px, the approval detail panel is hidden and each row links to `/approvals/:id`. The agent rail moves below its main content.
- Entity anatomy: `EntityCard` and `EntityRow` share a 40 px rounded-square image or safe local tint, name, one supporting line, at most one inline status badge, optional meta, and at most two visible actions. The whole surface links to detail only where a detail route exists; actions are distinct buttons, never nested links. Agent cards are 120 px tall. Collection rows use `columnTemplate` for responsive columns without descendant slot overrides. Approval rows stay fixed-height during decisions.
- Collections: page choice wins, then an explicit Appearance default, then the initial density fallback (list above twelve items, otherwise grid). Choosing a global default clears earlier Agents/Connections page choices and updates both immediately; later page toggles stay local. `CollectionToolbar` offers expandable 240 px search (`/` focus, Esc clear/collapse), sort, real facet filters, removable chips, and a grid/list switch. Search collapses when focus leaves its input/clear-control area while preserving and indicating an active query. Agents sort by Name, Recently changed or Expiring soonest and have no facet. Connections sort by Needs attention first, Recently connected or Name, and filter by state. The URL holds search (`q`), sort (`sort`) and the Connections state facet (`state`); default values stay out of the URL.
- Collection review: only facet filters create removable chips. A collapsed active search has a tinted, query-aware icon and tooltip; reopening restores its value. Non-default sort shows a tinted icon and a menu check. Agents show relative change time from `lastModifiedAt`. Provider marks are not shipped; service identities use local tinted initials. Both collections use a compact no-results state with Clear search. Connections also offer Clear filter without changing search or sort.
- Consent: the full local wordmark is grouped above the card. The identity zone places the agent icon beside its name/request text, with session-derived origin and a soft risk callout below. Without CIMD metadata, “Registered agent” is a neutral fallback, not a registrant claim. Signed in as appears once outside and beneath the card. Show a different redirect host on the origin line. The About popover holds the description and configured Governance, Documentation and Agent interface links. `PermissionPanel` keeps required groups locked, prior grants as Granted, and service disclosure only where selectable services require it. Missing-service warnings sit in normal layout, never clipped over avatar corners. The pinned footer holds DurationSelect, Deny and Allow or Connect, reassurance and technical details. Keep Allow visible at1280×720 with three groups and risk; authorization and callback-state transport are unchanged.
- Agent detail: reuse PermissionPanel in the editable 8-column area, followed by DurationSelect. The header shows saved grant expiry and relative change time, with no registrant inference. Connections in the 4-column rail is the sole owner of each service identity/status and Connect or Reconnect action; connected/unavailable records retain Manage guidance. These connection actions preserve the current draft through the existing authorization callback. Permission rows show service names and selection without repeated identity tiles, connection buttons or warning icons. Keep About separate from supplied IDs, registered client URIs and available grant record timestamps in Technical details. No tabs. Keep the back link, confirmed destructive-outline Revoke access, dirty-only Save bar and server-confirmed notifications.
- Connections: noncompact cards use a 160 px minimum and grow for real explanations or wrapped metadata. Group provider identity/status, scope/date metadata and the independent action footer. A single badge derives from existing session state: Connected, Needs sign-in, Expired or Unavailable. Needs sign-in exposes truthful hover/focus guidance; Refresh is offered only when supported, otherwise Reconnect. No connection appears only in agent detail. Stored scopes remain in a provider-labeled popover. Connected cards show Disconnect; otherwise a contextual Reconnect/Refresh/Retry uses outline. Disconnect stays destructive-quiet with Unlink and a dependent-agent/provider-token confirmation. Return to /connections consumes existing callback parameters once before replacing history. Notifications use visible hover and pointer affordances on their close controls.
- Approvals: Pending and Remembered are underline URL tabs with identical shared header/tab geometry. Remembered-only search and saved allow/deny outcome filters belong below the tabs within that panel. Tab changes enter with a 160 ms opacity fade, with immediate interactions and no retained stale decision controls; initial content is not hidden for an entrance animation. Pending keeps fixed-height rows, risk/request age and a sticky decision panel. Preserve visible arguments, scope preview, absolute expiry, session context, once/session/permanent choices and keyboard guards. After server confirmation, remove the decided row and select the next. Standalone review shares content without Motion. Remembered retains exact scope popovers and confirmed revocation; query filters never change authorization.
- Review corrections: agent cards have one linked detail surface, with no duplicate Details action. Console permission descriptions and service names wrap. Optional services use visible pressed-state buttons, not a second level of checkboxes or a hiding toggle. The save bar sits below the header and stays visible during scrolling. Unsaved edits require explicit discard for internal navigation and a browser warning for document departure. Duration edits show current access and access after saving. The chosen custom date remains visible beside the selector. Known refresh-token expiry updates connection status locally. Approval shortcuts require focus inside the pending inbox or review panel and exclude editors and overlays.
- Settings: `/settings/appearance` has immediate Light/Dark/System preview tiles and a default grid/list preference. Use plain setting rows with subtle dividers. Keep the category navigation hidden until another category exists. Never add a saved approval-persistence default.
- Feedback: icons and soft badges include words or icons, not color alone. Use a visible button border for Revoke/Disconnect and outline row actions. Single-line names use ellipsis with a tooltip. A multi-line “More” toggle appears only after `ResizeObserver` shows clipping; no entity expands in place. Skeletons match final geometry and empty states include an icon, explanation and next step when one exists.
- Entity name tooltips appear only after `TruncatedText` measures clipping. A linked surface reveals its clipped name on keyboard focus without another tab stop. Agent collection Revoke uses `destructive-quiet` with `ShieldOff`. The detail header keeps `destructive-outline`.

## Color and motion contract

Tint neutral surfaces at OKLCH hue 260. Use `--border-subtle` for card edges/dividers, `--border` for buttons/popovers, and `--border-control` only for inputs, checkboxes and radios with at least 3:1 contrast against their surrounding surface. `--primary-soft` marks active navigation, selections and icon tiles. Use soft status surfaces and text with a dot/icon; reserve solid fill for primary and destructive confirmation. Add contrast pairs in `contrast.test.ts` and inspect Storybook in both themes. Use restrained Lucide icons with default strokes 1.75 at 16 px and 1.5 at 20 px or larger; crop local logos to visible ink.

The following values from the approved UX plan are starting targets. They are not rendered contrast results. Light and dark variants must pass the 4.5:1 text and 3:1 control targets after implementation.

| Token or role | Light target | Dark target |
| --- | --- | --- |
| `--background` | `oklch(0.985 0.003 260)` | `oklch(0.175 0.006 260)` |
| `--card` | Quiet light surface | `oklch(0.21 0.008 260)` |
| `--popover` | Raised light surface | `oklch(0.245 0.009 260)` |
| `--muted` | `oklch(0.965 0.005 260)` | Tinted dark inset surface |
| `--foreground` | `oklch(0.21 0.012 260)` | `oklch(0.93 0.005 260)` |
| `--muted-foreground` | `oklch(0.47 0.012 260)` | `oklch(0.72 0.012 260)` |
| `--border-subtle` | `oklch(0.93 0.006 260)` | `oklch(0.275 0.01 260)` |
| `--border` | `oklch(0.88 0.008 260)` | `oklch(0.33 0.012 260)` |
| `--border-control` | `oklch(0.60 0.012 260)` | `oklch(0.54 0.012 260)` |
| `--primary` | `oklch(0.50 0.19 262)` | `oklch(0.70 0.15 262)` |
| `--primary-soft` | `oklch(0.95 0.03 262)`, text `oklch(0.42 0.17 262)` | `oklch(0.30 0.06 262)`, contrast-tested accent text |
| Soft status | Background `oklch(0.95 0.04 h)`, text `oklch(0.42 0.11 h)` | Background `oklch(0.28 0.05 h)`, text `oklch(0.82 0.11 h)` |

Use status hue 150 for success, 75 for warning, and 25 for danger. Badges use a 20 px height, 12 px medium text, 6 px radius and a leading dot or icon; they never occupy their own row. Buttons and controls use 8 px corners, cards and alerts 12 px, and dialogs and the consent card 16 px. For local brand SVGs, crop the wordmark to `viewBox="47.4 15 406.7 38.5"` and the mark to `viewBox="10 17.9 46.2 27.7"`; use height-based sizing.

Shared motion constants are 120, 160 and 200 ms, with 320 ms for emphasis, `cubic-bezier(0.2, 0, 0, 1)` and exits at 75% of entrance time. Motion and owned lucide-animated source are console-only for list exits/reflow and selected hover/empty/connection icons. A progressive browser View Transition can connect a card to detail; navigation still works without support. Decision routes use owned CSS/SVG animation only and import no Motion code. Reduced motion removes movement and icon drawing while retaining helpful color and opacity changes. The decision-bundle test enforces this boundary.

## Single cutover

Deliver all routes, shells, preferences, and command search in one release.
Do not introduce implementation phases, feature flags, parallel presentations, or backwards-compatibility layers.
Migrate every caller and remove obsolete components, tokens, aliases, fonts, animations, and caches together.

No redesign configuration belongs in the broker port, environment, CLI, Helm chart, or HTML bootstrap.
Do not add `ui.v2`, a browser flag, a build flag, or a configuration endpoint.
Browser preferences select appearance and collection presentation, never an old application view or an approval default.
The first-paint theme script remains necessary. It contains no principal, credential, or broker configuration.

The feature adds or changes no end-user API response. Every screen reads existing end-user contracts. With the route rename, the broker callback sends provider errors and a missing return URL to `/connections`.

## Theme contract

Browser storage holds `aib.theme`, `aib.sidebar-collapsed`, `aib.collection-view-default`, and page-specific collection choices. These are UI preferences only. They contain no principal, credential, selected permission, grant, approval, callback state or API response.

`aib.theme` accepts `light`, `dark`, or `system`. Missing or invalid values use system. If storage is unavailable, continue with an in-memory preference. The inline first-paint script and React provider share these rules.

`aib.collection-view-default` accepts `grid` or `list`. Page choice wins, then an explicit global choice, then the initial density fallback (list above twelve items, otherwise grid). Choosing a global default clears earlier Agents/Connections page choices and updates both immediately; later page toggles remain local. Invalid or unavailable storage falls back to in-memory settings, including independent write/removal failures. Search, sort, and filters are URL state; they do not affect security mutations.

The script sets a resolved `data-theme` on the document element before styles paint. The source preference remains separate from that resolved attribute. System mode listens for OS changes. Explicit choices ignore OS changes.

Add color-scheme metadata and CSS. Scope the CSS media fallback to a root without a resolved attribute. Portals and native controls inherit the resolved theme.

CSP uses `font-src 'self'`. The fixed theme script has a build-generated SHA-256 CSP hash. Never enable `unsafe-inline` to make it work. Preserve existing OAuth2 navigation and API behavior. Brand assets and fonts are same-origin.

## Static delivery

The approved performance contract is 190 kB gzip per initial decision graph and 5 seconds for cold consent content on Chrome Slow 4G. The stakeholder raised the bundle limit on 2026-10-05 to retain standard unsaved-navigation protection. Build gzip and Brotli companions for public text assets. The SPA handler negotiates those representations without compressing API responses. Decision HTML may preload only the public entry and selected decision’s static asset graph from the Vite build manifest; it must not embed private bootstrap data or preload console-only code.

Reconnect on `/connections` uses the existing `GET /api/third-party/{service_id}/oauth2/authorize` operation with a same-origin `redirect_uri` of `/connections`. Consent and agent-detail Connect return to the initiating page. They use the accepted `consent_state_id` form POST and bounded tab-local draft when the draft or return URL carries state, and the GET link otherwise. On return to `/connections`, `ConnectionsPage` consumes the existing `success`, `error` and `error_description` parameters once; it then replaces browser history with `/connections`. The broker callback sends provider errors and a missing return URL to `/connections`. Do not create aliases for `/sessions` or change the provider-facing callback parameters.


## Data and mutation contract

Query keys start with the authenticated principal returned by the existing user endpoint. Lists and details add their resource identifiers and filters. Never infer identity from editable storage.

Axios remains the transport and error-normalization owner. Query functions forward AbortSignal. On identity change or authentication loss, cancel requests and clear principal-owned cached data before showing another principal's content.

Do not persist API data to localStorage. Move service caches and hook loading state into Query without double caching. Remove `apiCache` after all callers migrate.

For revocation, require confirmation before mutation. Cancel matching reads and mark the affected card or row pending. The record stays visible while pending and leaves the collection only after the server accepts revocation. Preserve rollback data for that record, and restore it on failure only if a concurrent read removed it. Do not restore an entire old list over newer results from unrelated mutations. Disable a second action on the same record. Invalidate affected lists, details, and pending count after settlement. Announce success only after the server accepts revocation.

Approval, grant creation, and grant edits wait for authoritative server outcomes. Never replay a security mutation automatically after an error.

One console-level query refreshes `/api/approvals/pending` every 10 seconds. Sidebar and queue share it. Refresh immediately after a decision and when focus returns. Suspend unnecessary background work when the document is hidden. Network failure shows stale status, not a fabricated zero count.

No browser request reaches `GET /api/approvals`, the gateway-wide long-poll. No additive API is authorized.

## Decision invariants

Delta consent compares requested selections with the existing `granted_permission_sets`. Preserve prior groups and selected services. Previously granted groups are read-only in the decision view. Required groups and services remain locked. Never request an API delta computation.

The re-consent duration starts from the existing grant: Until revoked for a null `valid_until`, otherwise Custom date. Unless the user changes it, submission sends the existing `valid_until` unchanged. A changed duration applies to the whole grant.

Keep the current authorization-session parameter, expiry, callback, scope-preview, and continuation checks. Deny leaves existing grants untouched. Without an existing validated cancellation path, show a local cancelled outcome instead of inventing an OAuth2 redirect.

Grant durations remain Until revoked, 30 days, and a validated custom date. The control label “Until I revoke it” retains the existing null-validity behavior. Tool persistence remains once, session, or permanent with its existing semantics. No saved default preselects it. Show exact remembered scope before confirmation; keyboard shortcuts cannot bypass the scope editor or server checks.

Approval split menus select the main button action without submitting. Session/permanent selection reveals and validates the scope; the main button remains disabled until the server accepts the preview and then submits the selected approval directly. There is no separate Confirm approval or Back action; choosing Approve once closes scope editing. Denial options only select the main action, and permanent denial still requires its confirmation dialog after activation. Selection resets with the request. Inbox shortcuts retain focus, editor, expiry, principal, and overlay guards.

## Component and validation contract

Use all primitives named in ADR 038, plus retained controls needed by current flows. Use semantic tokens and owned source. The raw-palette ESLint rule covers className, `cn`, `clsx`, CVA, and variant-prefixed utilities.

Pages and application components take every user-facing string from the copy catalogue in `web/src/copy/`. Design-system components receive user-facing strings through props and do not import the catalogue.

Split networked route containers from pure `*View` components that receive data and callbacks. Patterns and screen stories use shared fixtures, not mocked network hooks. Require stories for loading, empty, typical, dense (ten or more), error, stale and longest realistic content (80-character name, 400-character description, eight permission groups), at 375, 768 and 1280 px in both themes. Include consent risk/missing connection/granted/denied/invalid states and approval high risk/expiry/remembered scope editor. Story `play` tests cover key permission, duration, keyboard decision and revoke flows; they measure unchanged list bounds and a visible Allow button at 1280 × 720.

Every component story runs with light and dark globals. Storybook uses the themes addon with `data-theme` and the a11y addon with `test: 'error'`. Six explicit browser projects cover both themes at 375 × 812, 768 × 1024, and 1280 × 720. The recipes serialize projects and story files to prevent capture contention. A toolbar switch alone does not constitute CI coverage.

[ADR 040](../../../adrs/040-canonical-light-visual-reference-gate.md) governs pixel references. A Vitest-only annotation compares the body, including portal overlays, through browser-mode `toMatchScreenshot`. It runs only in desktop `storybook-light` with effective light theme and an actual 1280 × 720 viewport. Intentional dark or narrow overrides retain interactions and accessibility checks without PNG references. Eligible Linux Chromium references live in `web/.storybook/__screenshots__/`. CI never runs with `--update` and fails on a missing or changed required canonical image. Principle XI requires no amendment.

Playwright references cover all seven destination routes, including both `/agents/:id` contexts, in light mode at 1280 × 720. The required set contains eight `_light.png` files. Dark and narrow configurations retain behavioral and accessibility coverage without required pixel references.

Store route screenshots under `tests/e2e/screenshots/`. Pin browser version, OS image, fonts, canonical viewport, data, and time. Route fixtures use stable logical identifiers within fresh scenario storage. Wait for fonts and query settlement.

Story interactions use ordinary motion. Capture then normalizes final states without changing product animation behavior. Compare pixels and publish expected, actual, and diff images on failure. Do not accept changed references automatically.

The route gate compares the eight required light images plus canonical light state images listed in `tests/e2e/screenshots/visual-gate.txt`. Other journey screenshots remain documentation. Existing thresholds remain unchanged. No workflow writes to or commits reviewed route or Storybook references. Automation uploads candidates as a review artifact. Every reference changes only in a reviewed commit.

WCAG 2.2 AA, 320 px, 200% zoom, focus, reduced motion, theme persistence, and zero automatic third-party requests require browser journeys beyond screenshot comparison.
