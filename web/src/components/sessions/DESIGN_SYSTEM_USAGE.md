# Design System Usage for Connections

## Authority and example status

[ADR 038](../../../../adrs/038-design-system-rebuilt-on-shadcn-radix.md) was accepted on 2026-09-27.
[Design principles](../../design-system/docs/DESIGN_PRINCIPLES.md) define the accepted feature 047 system.
The console composes `ConnectionsView`, `ConnectionCard`, `ConnectionStateBadge`, and `DisconnectDialog` on `/connections`.
Use the frontend integration tests and browser journeys to verify connection behavior.
See [README.md](README.md) for source contracts, service methods, and development commands.

Application components obtain UI strings from `@copy`. Primitives receive labels and descriptions through props; they do not import the catalogue.

## Component map

| Responsibility | Owned component |
| --- | --- |
| Console navigation and mobile navigation | ConsoleShell and Sheet |
| Page header, count, and controls | PageHeader and controlled CollectionToolbar |
| Grid and list | EntityCard and EntityRow in ConnectionCard, wrapped by AnimatedCollection |
| One-line name and safe avatar | Shared Entity tooltip and Avatar fallback |
| Derived state | Soft Badge inside application-owned ConnectionStateBadge |
| Raw scope details | Quiet text-and-chevron Button and provider-labeled Popover in ConnectionCard |
| Card actions | Outline contextual Button; destructive-outline Disconnect Button |
| Destructive confirmation | Dialog inside application-owned DisconnectDialog |
| Read failure and recovery | Alert with explicit retry; stale card shows Unavailable |
| Loading and no data | CollectionSkeleton and EmptyState |
| Server-confirmed and callback results | Shared Sonner Toaster |

Use the existing design-system categories, not a second primitive library.
The page has no accent action.
Keep Table and Command out of every decision-route import.

## Cards, rows, and controls

`ConnectionsView` is pure: it takes the records and callbacks from `ConnectionsPage` rather than calling a hook. The page mirrors `q`, `sort`, and `state` in the URL, while `useCollectionView('connections', count)` persists only the grid/list choice. Sort choices are attention first, recently connected by `initiated_at`, and provider name. The state filter can select Connected, Needs sign-in, Expired, or Unavailable. Active URL choices appear as removable chips below the header.

```tsx
import { ConnectionCard } from '@components/sessions/ConnectionCard';

<ConnectionCard session={session} state={getState(session.service_id)} view="grid"
  refreshing={isRefreshing(session.service_id)} disconnecting={isDisconnecting(session.service_id)}
  onRefresh={refresh} onRetry={refetch} onDisconnect={requestDisconnect} />
```

Connection cards use a 160 px minimum and grow for real explanations or wrapped metadata. List rows are 208 px below 768 px, 144 px from 768 through 1240 px, and 88 px above 1240 px. Provider identity and a fixed status slot form the header. Explanations wrap without clipping. A deliberate metadata area groups the quiet scope disclosure and relative connection age; the action footer remains separate.

The scope count uses a text-and-chevron disclosure, not an outlined pill beside the status badge. Its popover names the provider and shows wrapping monospace scope values. Zero scopes remain plain text without a dead control. Every stored state displays the count and real initiated date. No account, last-use, or modified timestamp is invented. Name overflow uses the shared one-line tooltip, not an expanding toggle.

Avatar rejects off-origin images before loading them.

The connected card shows only Disconnect, with `destructive-outline` styling. Needs sign-in exposes Refresh only when supported and Reconnect otherwise; Expired exposes Reconnect; Unavailable exposes Retry. The contextual action uses `outline`. Disconnect opens confirmation and never deletes on its own. Busy cards prevent duplicate operations. Reconnect navigates through the existing same-origin authorization URL, returning to `/connections`.

## Truthful state badges

ConnectionStateBadge belongs to the application because token usability is domain-specific. Badge supplies presentation without deciding whether access is usable.

| Label | Presentation | Required interpretation |
| --- | --- | --- |
| Connected | `success` Badge | Stored evidence establishes usable access |
| Needs sign-in | `warning` Badge | Access is unusable; supported refresh or reconnect is needed |
| Expired | `warning` Badge | Session or refresh capacity expired; reconnect is needed |
| Unavailable | `danger` Badge | Read failure/stale evidence cannot confirm current status |

The full precedence is in the [data model](../../../../specs/047-redesign-consent-console/data-model.md). A service required by an agent with no stored connection can show No connection elsewhere; it is never a stored `/connections` card. A failed read must not fabricate Connected. A network-only refresh error does not prove credential rejection. Do not infer missing scopes. The badge's visible label carries meaning without color; icons are decorative.

## Disconnect confirmation

Import DisconnectDialog through its concrete application module:

```tsx
import { DisconnectDialog } from '@components/sessions/DisconnectDialog';

<DisconnectDialog
  session={disconnect.confirmation}
  onCancel={disconnect.cancelRevoke}
  onConfirm={() => void disconnect.confirmRevoke()}
  onReturnFocus={restoreFocus}
/>
```

The dialog owns the current dependent-agent query and disables confirmation until that query succeeds. Cancel receives initial focus; the page owns mutation outcomes and focus recovery after removal.

The warning must state that broker disconnection does not revoke provider-side tokens.
Do not claim that every dependent grant is revoked or that provider access is removed.
Read dependent agents through the existing session detail operation.
Escape all service and agent names.

The open Dialog makes background content inert.
Preserve its accessible title, description, focus boundary, Escape behavior, and focus return.
Cancel leaves the connection unchanged.

Only after confirmation can the application mark the affected record pending.
Announce success only after server acceptance.
On failure, restore the record and show safe copy through the shared Toaster rather than a second inline notice.

## Loading, empty, error, and callback states

CollectionSkeleton announces the initial read. EmptyState explains a successful empty list without inventing a global connection-creation action. A failed read presents an Alert with retry, not an empty success; stale records stay visible with an Unavailable badge and retry.

ConnectionsPage consumes `success`, `error`, and `error_description` callback parameters once on `/connections`. The matching `service_id` from a successful callback pulses its card's border for 320 ms and plays the connection icon, including when a new record arrives. Display only local allowlisted error copy, never arbitrary provider descriptions. Server-confirmed refresh and disconnect, callback outcomes, and operation errors use the existing shared Toaster; there is no inline notice timer.

## Theme, assets, and motion

Use [COLOR_GUIDE.md](../../design-system/docs/COLOR_GUIDE.md) for exact semantic OKLCH values in both themes.
Use `background`, `card`, `foreground`, `muted-foreground`, `border`, and `ring` roles through generated utilities.
Control boundaries use `border`; quiet separators use `border-subtle`; `border-control` is reserved for form controls.
Do not add raw palette examples or literal component colors.

Use local Wordmark artwork from `web/public/brand/` and fonts from `web/public/fonts/`.
Display text uses Zalando Sans Variable, body text uses Inter Variable, and technical values use JetBrains Mono.
Do not request third-party fonts, scripts, or images.

Use shared spacing and radius tokens, neutral surfaces, and 1 px borders. Motion is confined to console composition: AnimatedCollection handles removal, the connection icon explains updates, and CSS pulses the matching border for 320 ms. Disable movement under reduced motion. Do not add decorative gradients or page-entry motion.

## Accessibility and behavior requirements

Principle XI requires WCAG 2.1 AA. Feature 047 targets WCAG 2.2 AA.
These requirements do not record completed validation.

- Keep card and row actions available at 320 px and 200% zoom without changing their height.
- Keep keyboard focus visible and unobscured.
- Return focus after cancellation and move it predictably after record removal.
- Announce state changes and results without taking focus during background updates.
- Validate 4.5:1 text contrast and 3:1 control contrast in both themes.
- Cover default, hover, focus-visible, disabled, error, loading, and overlay-open story states where applicable.
- Require both-theme story accessibility checks and reviewed visual baselines.

Use T116–T122 for hook, page, callback, refresh, and disconnect acceptance requirements.
Keep browser selectors inside the existing session page object.

## References

- [Design system index](../../design-system/docs/INDEX.md)
- [Component archetypes](../../design-system/docs/COMPONENT_ARCHETYPES.md)
- [Composition patterns](../../design-system/docs/COMPOSITION_PATTERNS.md)
- [Accessibility guide](../../design-system/docs/ACCESSIBILITY_GUIDE.md)
- [UI contract](../../../../specs/047-redesign-consent-console/contracts/ui-and-configuration.md)
