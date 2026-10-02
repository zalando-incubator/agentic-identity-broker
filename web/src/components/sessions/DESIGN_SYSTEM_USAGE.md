# Design System Usage for Connections

## Authority and example status

[ADR 037](../../../../adrs/037-design-system-rebuilt-on-shadcn-radix.md) was accepted on 2026-09-27.
[Design principles](../../design-system/docs/DESIGN_PRINCIPLES.md) define the accepted feature 047 system.
The source uses ConnectionsTable, ConnectionStateBadge, and DisconnectDialog on the unchanged `/sessions` route.
Browser and release validation remain separate acceptance gates.
See [README.md](README.md) for source contracts, service methods, and development commands.

Application components obtain UI strings from `@copy`.
The `labels` in the action sketch represent application catalogue values.
Primitives receive labels and descriptions through props and do not import the catalogue.

## Component map

| Responsibility | Owned component |
| --- | --- |
| Console navigation and mobile navigation | ConsoleShell and Sheet |
| Page title and purpose | PageHeader |
| Dense connections list | Table presentation parts with TanStack Table state |
| Escaped long name | TruncatedText with accessible expansion |
| Derived state | Badge inside application-owned ConnectionStateBadge |
| Row actions | Button with `secondary`, `outline`, or `ghost` |
| Destructive confirmation | Dialog inside application-owned DisconnectDialog |
| Errors and warnings | Visible alert text with a retry action |
| Loading and no data | Status text and a successful-empty explanation |
| Server-confirmed result | Polite inline status notice |

Use the existing design-system categories, not a second primitive library.
The page has no accent action.
Keep Table and Command out of every barrel that decision routes import.

## Connections table

Table uses compact rows, semantic headings, and an accessible name.
The application supplies provider, scope count, connection state, creation time, and actions from existing session data.
Below the small breakpoint, rows stack into label-value groups without losing actions.

The following sketch shows a row's action group:

```tsx
import { Button } from '@design-system/components/primitives/Button';

<div className="flex flex-wrap gap-2">
  {canReconnect && (
    <Button variant="outline" onClick={onReconnect} disabled={pending}>
      {labels.reconnect}
    </Button>
  )}
  {canRefresh && (
    <Button variant="ghost" onClick={onRefresh} disabled={pending} aria-busy={refreshing}>
      {labels.refresh}
    </Button>
  )}
  <Button variant="ghost" onClick={onRequestDisconnect} disabled={pending}>
    {labels.disconnect}
  </Button>
</div>
```

`onRequestDisconnect` opens confirmation. It does not delete the session.
A busy row cannot submit a second operation.
Reconnect retains the existing authorization URL. Refresh appears only when supported.

Use `font-mono` for the supplied creation timestamp.
Use TruncatedText with `lines={1}` for the service name in a table cell.
Pass its required `expandLabel` and `collapseLabel` from the application catalogue.
Do not load a remote service image before selecting the local fallback.

## Truthful state badges

ConnectionStateBadge belongs to the application because token usability is domain-specific.
Badge supplies presentation without deciding whether a session is usable.

| Label | Presentation | Required interpretation |
| --- | --- | --- |
| Connected | `success` Badge | Existing data establishes usable access |
| Needs re-authentication | `warning` Badge | Existing data or authoritative refresh results require attention |
| Expired | `danger` Badge | The data-model expiry rules apply |
| No connection | `neutral` Badge | An agent requires a service without a stored session |

The full precedence is in the [data model](../../../../specs/047-redesign-consent-console/data-model.md).
`/sessions` contains stored sessions only, so No connection never appears there.
A failed read shows an error or explicitly stale state, never a fabricated Connected state.
A network-only refresh error does not prove that credentials were rejected.

Do not infer missing scopes or show account and last-use metadata.
Do not use a status badge as a security guarantee.
The visible label carries meaning without color. Decorative icons use `aria-hidden="true"`.

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

The dialog owns the current dependent-agent query and disables confirmation until that query succeeds.
The page owns mutation results and focus recovery after row removal.

The warning must state that broker disconnection does not revoke provider-side tokens.
Do not claim that every dependent grant is revoked or that provider access is removed.
Read dependent agents through the existing session detail operation.
Escape all service and agent names.

The open Dialog makes background content inert.
Preserve its accessible title, description, focus boundary, Escape behavior, and focus return.
Cancel leaves the connection unchanged.

Only after confirmation can the application mark the affected record pending.
On failure, restore that record without overwriting unrelated changes.
Announce success only after server acceptance.
Keep a failed mutation visible in the page alert after restoring the affected row.

## Loading, empty, error, and callback states

Status text announces the initial read.
The empty-state message explains a successful empty result without inventing a global connection-creation action.
The page alert provides a retry action for failed reads.
A failed request does not become an empty success state.

ConnectionsPage preserves the `success`, `error`, and `error_description` callback outcomes.
Map errors to application copy rather than exposing arbitrary server text.
The page announces server-confirmed results through a polite status notice.
For toast notifications elsewhere, use the shared Toaster instead of a second notification provider.

## Theme, assets, and motion

Use [COLOR_GUIDE.md](../../design-system/docs/COLOR_GUIDE.md) for exact semantic OKLCH values in both themes.
Use `background`, `card`, `foreground`, `muted-foreground`, `border`, and `ring` roles through generated utilities.
Control boundaries use `border`. Decorative separators can use `border-soft`.
Do not add raw palette examples or literal component colors.

Use local Wordmark artwork from `web/public/brand/` and fonts from `web/public/fonts/`.
Display text uses Zalando Sans Variable, body text uses Inter Variable, and technical values use JetBrains Mono.
Do not request third-party fonts, scripts, or images.

Use shared spacing and radius tokens, neutral surfaces, and 1 px borders.
Use CSS-only feedback at 120–200 ms with ease-out.
Remove movement under reduced motion, including overlay movement.
Do not add card lifts, decorative gradients, or page-entry animation.

## Accessibility and behavior requirements

Principle XI requires WCAG 2.1 AA. Feature 047 targets WCAG 2.2 AA.
These requirements do not record completed validation.

- Preserve table relationships and visible labels on narrow layouts.
- Keep every row action available at 320 px and 200% zoom.
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
