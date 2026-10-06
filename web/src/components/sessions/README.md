# OAuth2 Connection Components

## Authority and implementation status

[ADR 038](../../../../adrs/038-design-system-rebuilt-on-shadcn-radix.md) was accepted on 2026-09-27.
[Design principles](../../design-system/docs/DESIGN_PRINCIPLES.md) define the accepted feature 047 system.
The source uses the replacement components below. Use the frontend integration tests and browser journeys to verify connection behavior.

The console route is `/connections`; provider callbacks return to that route without an alias.
The redesign adds no API field, endpoint, or OAuth2/token behavior. Only the per-page grid/list preference is stored locally.
See the [feature specification](../../../../specs/047-redesign-consent-console/spec.md) and [tasks](../../../../specs/047-redesign-consent-console/tasks.md).

## Source contracts

| Source | Responsibility |
| --- | --- |
| `ConnectionCard.tsx` | Fixed-height card/row with truthful state, quiet scope disclosure, initiated date, and contextual actions |
| `ConnectionStateBadge.tsx` | Text and semantic color for the derived state |
| `connectionState.ts` | State precedence from stored connection data and refresh results |
| `DisconnectDialog.tsx` | Named confirmation, current dependent-agent details, and pure dialog view |
| `../../pages/ConnectionsView.tsx` / `ConnectionsPage.tsx` | Network-free collection view and `/connections` container (callbacks, URL filters, toasts) |
| `../../hooks/useConnections.ts` | Principal-scoped queries, refresh, and record-scoped disconnect |
| `../../services/api/sessions.ts` | Uncached typed Axios transport |

Import these components through their concrete modules. There is no session-component barrel.
Keep Table and Command out of decision-route imports.

`ConnectionsView` receives records, derived-state readers, controlled search/sort/state-filter/view values, operation callbacks, and an optional dialog node. The page container owns the principal-scoped queries, mutation effects, URL state, and focus return.
`ConnectionCard` composes `EntityCard` or `EntityRow`; `AnimatedCollection` wraps direct keyed cards/rows only on the console route.
`DisconnectDialog` reads dependent agents before it enables confirmation. Cancel receives initial focus. On close, focus returns to the current Disconnect button, or to the heading if that record was removed or disabled.

The hook exposes the shared list and its operations:

```typescript
const {
  sessions, loading, error, refetch,
  getState, refresh, isRefreshing, refreshError,
} = useConnections();
const disconnect = useDisconnectConnection();
```

TanStack Query owns principal-scoped server state over Axios. The transport has no second GET cache.

## Page composition

```text
ConsoleShell
  PageHeader: Connections count and CollectionToolbar
    Search, attention/recently connected/name sort, state filter, persisted grid/list toggle
  Removable URL filter chips below the header
  CollectionSkeleton, error with retry, EmptyState, or AnimatedCollection
    ConnectionCard: provider avatar/name, one state badge, explanation when needed
      Quiet scope disclosure and initiated_at date in a separate metadata area
      Reconnect/Refresh/Retry (only when needed) and outlined Disconnect
  DisconnectDialog: dependent agents, provider-token warning, focused Cancel
  Shared Sonner Toaster: callback, refresh, and disconnect outcomes
```

Cards use a 160 px minimum and grow for real explanations or wrapped metadata. Rows keep the specified responsive heights. Provider identity/status, scope/date metadata, and the action footer form distinct groups. Scope disclosures open a popover without expanding the entity. Search (`q`), sort (`sort`), and state (`state`) live in the URL. The view toggle persists under `aib.collection-view.connections`, while an explicit global default resets earlier page choices. "Recently connected" uses only `initiated_at`, never a made-up modified timestamp.
See [DESIGN_SYSTEM_USAGE.md](DESIGN_SYSTEM_USAGE.md) for examples.

## Connection state and operations

The full precedence table lives in the [data model](../../../../specs/047-redesign-consent-console/data-model.md).
Connection state derives from existing session fields and current authoritative refresh results.

| State | Badge and contextual action |
| --- | --- |
| Connected | Usable access: only outlined Disconnect on the card; no redundant Refresh action |
| Needs sign-in | Access token unusable: Refresh when server-supported, otherwise Reconnect. After a `409` or `502` refresh rejection, Reconnect |
| Expired | Session or refresh capacity expired: Reconnect |
| Unavailable | Read failure or stale evidence: Retry the read without guessing credential validity |

Only an agent-required service without a stored connection has "No connection"; it is not a stored card state. A failed read never invents Connected. Network-only refresh errors retain the last authoritative state in the hook but present an Unavailable badge and a retry on the card. Rejected refresh uses the data-model precedence, not a guessed credential state. Do not add Missing scopes, account, or last-use fields.

Reconnect preserves the existing authorization and callback flow.
Refresh waits for the server and recomputes state.
Known refresh-token expiry deadlines update the visible state locally without fetching. Expired refresh capacity replaces Connected or Refresh with Expired and Reconnect. New session data cancels and replaces the scheduled deadline; unknown lifetimes remain server-authoritative.
Disconnect requires confirmation and a fresh successful read from the affected-agents operation.
Its warning states that broker disconnection does not revoke provider-side tokens.

After confirmation, optimistic disconnection affects only that record.
On failure, restore that record without overwriting other changes.
Announce success only after server acceptance.

## API integration

The canonical contract is [api/enduser/openapi.yaml](../../../../api/enduser/openapi.yaml).
The session list response is `{data: {sessions: [UserSessionSummary]}}`.

| Operation | Existing endpoint | Service method |
| --- | --- | --- |
| List stored sessions | `GET /api/third-party/sessions` | `listSessions()` |
| Read dependent agents and details | `GET /api/third-party/{service_id}/session` | `getSessionDetails(serviceId)` |
| Read disconnect dependencies | `GET /api/third-party/{service_id}/session/affected-agents` | `getAffectedAgents(serviceId)` |
| Disconnect at the broker | `DELETE /api/third-party/{service_id}/session` | `terminateSession(serviceId)` |
| Refresh tokens | `POST /api/third-party/{service_id}/session/refresh` | `refreshSession(serviceId)` |
| Reconnect | `/api/third-party/{service_id}/oauth2/authorize` | Existing browser authorization navigation |

Consume `success`, `error`, `error_description`, and success `service_id` on `/connections` once, replacing the callback URL. Errors display only allowlisted local copy; never render arbitrary provider descriptions. The success service ID selects the card for a 320 ms accent-border pulse and animated icon, including newly loaded cards. Keep Axios error normalization and the upstream authentication boundary. Store no credentials or API responses in browser preference storage.

## Presentation and accessibility

Applications obtain strings from `@copy`. Primitives receive those strings through props.
Use semantic OKLCH roles in both themes, not palette utilities or component-local color values.
Use local Wordmark artwork and safe Avatar fallbacks.
Self-host Zalando Sans Variable, Inter Variable, and JetBrains Mono.
Use 120–200 ms CSS ease-out feedback and a 320 ms border pulse; stop illustrative movement under reduced motion.

Principle XI requires WCAG 2.1 AA. Feature 047 targets WCAG 2.2 AA.
Keep visible labels, keyboard popover actions, unobscured focus, and Dialog focus return.
Announce outcomes through the shared Toaster without moving focus.
Every design-system story requires both-theme accessibility checks and reviewed visual baselines.
These requirements are not a claim of completed validation.

## Testing routes

Current coverage lives in `connectionState.test.ts`, `../../hooks/useConnections.test.ts`, and `../../pages/ConnectionsPage.test.tsx`; network-free view states and the scope-popover height interaction live in `../../pages/ConnectionsView.stories.tsx`. Preserve state precedence, scoped rollback, dependency warnings, and callback-outcome tests. AS-10 and AS-11 cover browser journeys; page-object selectors belong in `tests/e2e/pages/connections_page.go`.

From the repository root:

```bash
just web-test
just web-test-coverage
just test-e2e-frontend
```

From `web/`, `npm test -- --watch` runs the unit watch mode.
Mock service boundaries rather than hooks, and assert user-visible behavior with semantic queries.

## Local development

Start the backend and Vite in separate terminals from the repository root:

```bash
just run
just web-dev
```

The development route is `http://localhost:3000/connections`.
`web/vite.config.ts` owns the proxy and development `X-Remote-User` header.
Its default backend target is `http://localhost:8000`.

For the broker-served frontend:

```bash
just web-build && just run
```

Open `http://localhost:8000/connections`.
Use [web/AGENTS.md](../../../AGENTS.md) for retained architecture, aliases, commands, and testing rules.

## Historical scope

The original session notes discussed a separate detail page, token timelines, and activity logs.
Those notes do not define feature 047 requirements or unfinished redesign work.
The accepted redesign preserves existing capabilities and adds no activity endpoint or last-use field.
