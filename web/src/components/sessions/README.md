# OAuth2 Connection Components

## Authority and implementation status

[ADR 037](../../../../adrs/037-design-system-rebuilt-on-shadcn-radix.md) was accepted on 2026-09-27.
[Design principles](../../design-system/docs/DESIGN_PRINCIPLES.md) define the accepted feature 047 system.
The source uses the replacement components below. Browser and release validation remain separate acceptance gates.

The `/sessions` route remains unchanged.
The redesign adds no API field, endpoint, persistence, or OAuth2/token behavior.
See the [feature specification](../../../../specs/047-redesign-consent-console/spec.md) and [tasks](../../../../specs/047-redesign-consent-console/tasks.md).

## Source contracts

| Source | Responsibility |
| --- | --- |
| `ConnectionsTable.tsx` | Provider, scope count, state, creation time, and row actions |
| `ConnectionStateBadge.tsx` | Text and semantic color for the derived state |
| `connectionState.ts` | State precedence from session data and refresh results |
| `DisconnectDialog.tsx` | Named confirmation and current dependent-agent details |
| `../../pages/ConnectionsPage.tsx` | `/sessions`, callbacks, loading, error, empty, and content states |
| `../../hooks/useConnections.ts` | Principal-scoped queries, refresh, and record-scoped disconnect |
| `../../services/api/sessions.ts` | Uncached typed Axios transport |

Import these components through their concrete modules. There is no session-component barrel.
Keep Table and Command out of decision-route imports.

`ConnectionsTable` receives sessions, derived state, pending-state readers, and refresh, disconnect, and retry callbacks.
`DisconnectDialog` receives the selected session, cancellation and confirmation callbacks, and an optional focus-return callback.
It reads dependent agents before it enables confirmation.

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
  PageHeader: Connections and a one-line purpose
  Loading, error, empty, or content state
    ConnectionsTable
      Provider name with accessible expansion
      Scope count
      ConnectionStateBadge with text
      Creation time in monospace
      Non-accent Reconnect, supported Refresh, and Disconnect
  DisconnectDialog
    Named service and dependent-agent warning
    Provider-token warning
    Secondary Cancel and destructive confirmation
```

The page has no accent action.
The table stacks labels and values on narrow screens without hiding row actions.
The composition uses owned shadcn/Radix components in the existing design-system categories.
See [DESIGN_SYSTEM_USAGE.md](DESIGN_SYSTEM_USAGE.md) for examples.

## Connection state and operations

The full precedence table lives in the [data model](../../../../specs/047-redesign-consent-console/data-model.md).
Connection state derives from existing session fields and current authoritative refresh results.

| State | Meaning and action |
| --- | --- |
| Connected | Access is usable. Supported Refresh and confirmed Disconnect remain available |
| Needs re-authentication | Access is unusable or refresh remains unproven. Refresh or Reconnect follows the data-model precedence |
| Expired | Refresh capacity expired, or access expired without usable refresh capacity. Reconnect is available |
| No connection | Only an agent-required service without a session, outside the stored `/sessions` list |

A failed read never implies Connected.
A network-only refresh error preserves the last authoritative state as stale.
A rejected refresh response uses the data-model rules, not a guessed credential state.
Do not add Missing scopes, account, or last-use fields.

Reconnect preserves the existing authorization and callback flow.
Refresh waits for the server and recomputes state.
Disconnect requires confirmation and dependent-agent details from the existing detail operation.
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
| Disconnect at the broker | `DELETE /api/third-party/{service_id}/session` | `terminateSession(serviceId)` |
| Refresh tokens | `POST /api/third-party/{service_id}/session/refresh` | `refreshSession(serviceId)` |
| Reconnect | `/api/third-party/{service_id}/oauth2/authorize` | Existing browser authorization navigation |

Preserve the current `success`, `error`, and `error_description` callback outcomes.
Keep Axios error normalization and the upstream authentication boundary.
Do not put credentials or API responses in browser preference storage.

## Presentation and accessibility

Applications obtain strings from `@copy`. Primitives receive those strings through props.
Use semantic OKLCH roles in both themes, not palette utilities or component-local color values.
Use local Wordmark artwork and safe Avatar fallbacks.
Self-host Zalando Sans Variable, Inter Variable, and JetBrains Mono.
Use 120–200 ms CSS ease-out feedback and remove movement under reduced motion.

Principle XI requires WCAG 2.1 AA. Feature 047 targets WCAG 2.2 AA.
Keep semantic tables, visible labels, keyboard actions, unobscured focus, and Dialog focus return.
Announce state changes without moving focus.
Every design-system story requires both-theme accessibility checks and reviewed visual baselines.
These requirements are not a claim of completed validation.

## Testing routes

Current coverage lives in `connectionState.test.ts`, `../../hooks/useConnections.test.ts`, and `../../pages/ConnectionsPage.test.tsx`.
These tests cover state precedence, refresh outcomes, scoped rollback, disconnect warnings, and callback outcomes.
T116 and T117 define the hook and page acceptance requirements.
AS-10 and AS-11 cover the browser journeys.
Existing session-refresh journeys retain their assertions.
Selector changes belong in `tests/e2e/pages/sessions_page.go`.

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

The development route is `http://localhost:3000/sessions`.
`web/vite.config.ts` owns the proxy and development `X-Remote-User` header.
Its default backend target is `http://localhost:8000`.

For the broker-served frontend:

```bash
just web-build && just run
```

Open `http://localhost:8000/sessions`.
Use [web/AGENTS.md](../../../AGENTS.md) for retained architecture, aliases, commands, and testing rules.

## Historical scope

The original session notes discussed a separate detail page, token timelines, and activity logs.
Those notes do not define feature 047 requirements or unfinished redesign work.
The accepted redesign preserves existing capabilities and adds no activity endpoint or last-use field.
