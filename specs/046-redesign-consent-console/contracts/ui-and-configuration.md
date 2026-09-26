# UI and Temporary Configuration Contracts

**Status**: Proposed design. No runtime implementation is included.

## Routes and shells

| Address | Context | Shell | Phase |
| --- | --- | --- | --- |
| `/` | Entry | Replace history with `/delegations` | Unchanged |
| `/delegations` | Agent grants | ConsoleShell | 3 |
| `/agents/:id` | Validated authorization session | DecisionShell | 2 |
| `/agents/:id` | Grant management without authorization session | ConsoleShell | 3 |
| `/sessions` | Third-party connections | ConsoleShell | 3 |
| `/approvals` | Pending and standing decisions | ConsoleShell | 3 |
| `/approvals/:id` | Tool decision or resolved result | DecisionShell | 2 |
| `/activity` | Acting-user history | ConsoleShell | 4 |
| `/settings` | Browser preferences | ConsoleShell | 4 |

An invalid or expired authorization session stays an error in the decision flow. It must not silently become an editable management view. Only the backend validates authorization context.

ConsoleShell owns the collapsible sidebar, mobile Sheet, page heading, purpose text, pending count, and user menu. DecisionShell has no sidebar. Its content width is at most 640 px.

All route components remain lazy. Import concrete component modules where a barrel would pull Table or Command into decision bundles.

## Temporary broker configuration

```yaml
ui:
  v2: false
```

| Surface | Contract |
| --- | --- |
| YAML | `ui.v2`, boolean, default false |
| Environment | `IDENTITY_BROKER_UI_V2` |
| CLI | `--ui-v2` |
| Precedence | Existing broker order: CLI, environment, file, default |
| Validation | Reject invalid boolean values at startup |
| Browser exposure | Server-generated boolean in SPA HTML only |
| Removal | End of phase 3, including configuration port, adapters, CLI, environment binding, Helm, examples, and documentation |

Wire the value through `internal/ports/config.go` and `internal/app/builder.go`. Update the existing broker configuration implementation, examples, and Helm ConfigMap path. Do not use localStorage, a query parameter, or `VITE_UI_V2` as the rollout authority.

Use a fixed HTML bootstrap slot for the boolean. Generate it once from validated startup configuration. Do not serialize configuration or user data. Serve modified HTML with no-store semantics. Keep hashed assets cacheable. GET and HEAD must agree on headers and representation metadata.

Vite development must use the broker's resolved bootstrap value through its server-side development proxy. The browser must not receive a new endpoint or a separate flag source. Storybook uses isolated fixture values, not broker configuration.

During phase 2, true selects v2 decisions and leaves unmigrated console routes intact. During phase 3, true also selects v2 console routes. False preserves old presentation until the cutover gate passes. Authorization services are shared.

After phase 3, only the new presentation exists. Remove the bootstrap slot and its client parsing. Remove the flag from all documented and deployed configuration examples.

## Theme contract

Browser storage keys are `aib.theme`, `aib.approval-persistence`, and `aib.sidebar-collapsed`. These contain only appearance and UI preferences.

`aib.theme` accepts `light`, `dark`, or `system`. Missing or invalid values use system. If storage is unavailable, continue with an in-memory preference. The inline first-paint script and React provider share these rules.

The script sets a resolved `data-theme` on the document element before styles paint. The source preference remains separate from that resolved attribute. System mode listens for OS changes. Explicit choices ignore OS changes.

Add color-scheme metadata and CSS. Scope the CSS media fallback to a root without a resolved attribute. Portals and native controls inherit the resolved theme.

CSP uses `font-src 'self'`. The fixed theme script has a build-generated SHA-256 CSP hash. Never enable `unsafe-inline` to make it work. Preserve existing OAuth2 navigation and API behavior. Brand assets and fonts are same-origin.

## Data and mutation contract

Query keys start with the authenticated principal returned by the existing user endpoint. Lists and details add their resource identifiers and filters. Never infer identity from editable storage.

Axios remains the transport and error-normalization owner. Query functions forward AbortSignal. On identity change or authentication loss, cancel requests and clear principal-owned cached data before showing another principal's content.

Do not persist API data to localStorage. Move service caches and hook loading state into Query without double caching. Remove `apiCache` after all callers migrate.

For revocation, require confirmation before mutation. Cancel matching reads and mark the affected row pending. Preserve rollback data for that record. Do not restore an entire old list over newer results from unrelated mutations. Disable a second action on the same record. Invalidate affected lists, details, pending count, and activity after settlement. Announce success only after the server accepts revocation.

Approval, grant creation, and grant edits wait for authoritative server outcomes. Never replay a security mutation automatically after an error.

One console-level query refreshes `/api/approvals/pending` every 10 seconds. Sidebar and queue share it. Refresh immediately after a decision and when focus returns. Suspend unnecessary background work when the document is hidden. Network failure shows stale status, not a fabricated zero count.

No browser request reaches `GET /api/approvals`, the gateway-wide long-poll. No third additive API is authorized.

## Decision invariants

Delta consent compares requested selections with the existing `granted_permission_sets`. Preserve prior groups and selected services. Required groups and services remain locked. Never request an API delta computation.

Keep the current authorization-session parameter, expiry, callback, scope-preview, and continuation checks. Deny leaves existing grants untouched. Without an existing validated cancellation path, show a local cancelled outcome instead of inventing an OAuth2 redirect.

Grant durations remain Until revoked, 30 days, and a validated custom date. Tool persistence remains once, session, or permanent. Browser defaults preselect a choice but never submit it. Every remembered decision shows its exact scope before confirmation.

## Component and validation contract

Use all primitives named in ADR 037, plus retained controls needed by current flows. Use semantic tokens and owned source. The raw-palette ESLint rule covers className, `cn`, `clsx`, CVA, and variant-prefixed utilities.

Every component story runs with light and dark globals. Storybook uses the themes addon with `data-theme` and the a11y addon with `test: 'error'`. Two explicit browser-test projects run the same story set, one per theme. A toolbar switch alone does not constitute CI coverage.

Playwright visual baselines cover five existing routes, with both `/agents/:id` contexts. Thus the minimum original-route matrix is 12 images, not 10. Add activity and settings in phase 4 for 16 route/context/theme images.

Store screenshots under `tests/e2e/screenshots/`. Pin browser version, OS image, fonts, viewport, data, and time. Wait for fonts and query settlement. Compare pixels and publish expected, actual, and diff images on failure. Do not accept changed baselines automatically.

WCAG 2.2 AA, 320 px, 200% zoom, focus, reduced motion, theme persistence, and zero automatic third-party requests require browser journeys beyond screenshot comparison.
