import { useRef, useState } from 'react';
import { Link } from 'react-router-dom';
import { Alert } from '@design-system/components/feedback/Alert';
import { ArrowRight } from 'lucide-react';
import { Skeleton } from '@design-system/components/feedback/Skeleton';
import { Avatar } from '@design-system/components/primitives/Avatar';
import { Button } from '@design-system/components/primitives/Button';
import { ConnectionStateBadge } from '@components/sessions/ConnectionStateBadge';
import { Tooltip, TooltipContent, TooltipProvider, TooltipTrigger } from '@design-system/components/overlays/Tooltip';
import { accessCopy, commonCopy, navigationCopy } from '@copy';
import { consentCopy } from '@copy/consent';
import { connectionsCopy } from '@copy/connections';
import type { ConnectionState } from '@components/sessions/connectionState';
import type { ServiceRequirement } from '../../types/consent';

export interface AgentConnection {
  service: ServiceRequirement;
  state: ConnectionState;
}

export function AgentConnectionsRail({ connections, loading, error, storageError, disabled, onRetry, onConnect }: {
  connections: readonly AgentConnection[];
  loading: boolean;
  error: boolean;
  storageError: boolean;
  disabled?: boolean;
  onRetry: () => void;
  onConnect: (serviceId: string) => void;
}) {
  return <section data-testid="agent-connections" aria-label={navigationCopy.connections} className="space-y-3 rounded-xl border border-border-subtle bg-card p-4">
    <h2 className="font-display text-base font-semibold">{navigationCopy.connections}</h2>
    {loading && <div role="status" aria-label={commonCopy.loading}>{connections.map(({ service }) => <div key={service.serviceId} aria-hidden="true" className="grid h-20 grid-cols-[2rem_minmax(0,1fr)] grid-rows-2 items-center gap-x-2 gap-y-1 py-2"><Skeleton variant="rounded" className="row-span-2 size-8" /><Skeleton className="w-3/4" /><div className="col-start-2 flex items-center justify-between gap-2"><Skeleton className="w-16" /><Skeleton className="w-16" /></div></div>)}</div>}
    {error && <Alert variant="error" action={{ label: commonCopy.retry, onClick: onRetry }}>{consentCopy.connectionsLoadError}</Alert>}
    {storageError && <Alert variant="error">{consentCopy.connectionStorageError}</Alert>}
    {connections.length === 0 && !loading && <p className="text-sm text-muted-foreground">{consentCopy.noConnections}</p>}
    {!loading && connections.length > 0 && <TooltipProvider><ul className="divide-y divide-border-subtle">
      {connections.map(({ service, state }) => <li key={service.serviceId} data-testid="agent-connection-row" className="grid min-h-20 min-w-0 grid-cols-[2rem_minmax(0,1fr)] items-center gap-x-2 gap-y-1 py-2">
        <Avatar id={service.serviceId} label={service.serviceName} size="sm" aria-hidden="true" className="row-span-2" />
        <span data-testid="connection-provider" className="min-w-0 text-sm [overflow-wrap:anywhere]">{service.serviceName}</span>
        <div className="col-start-2 flex min-w-0 items-center justify-between gap-1">
          <ConnectionStateTooltip state={state} />
          {!state.stale && (state.status === 'no-connection' || state.status === 'expired' || state.status === 'needs-reauthentication')
            ? <Button type="button" variant="outline" size="sm" disabled={disabled} data-testid="connection-action" aria-label={state.status === 'no-connection' ? consentCopy.connectService(service.serviceName) : consentCopy.reconnectService(service.serviceName)} onClick={() => onConnect(service.serviceId)}>{state.status === 'no-connection' ? accessCopy.connect : accessCopy.reconnect}</Button>
            : <Link data-testid="connection-action" aria-label={consentCopy.manageConnections} className="inline-flex shrink-0 items-center gap-1 text-sm text-primary hover:underline" to="/connections">{consentCopy.manage}<ArrowRight aria-hidden="true" className="size-4" /></Link>}
        </div>
      </li>)}
    </ul></TooltipProvider>}
  </section>;
}

function ConnectionStateTooltip({ state }: { state: ConnectionState }) {
  const [open, setOpen] = useState(false);
  const trigger = useRef<HTMLElement | null>(null);
  const close = () => setOpen(false);
  return <Tooltip open={open} onOpenChange={(nextOpen) => {
    // Radix dismisses on ancestor scroll, including scrolling caused by keyboard focus.
    if (nextOpen || document.activeElement !== trigger.current) setOpen(nextOpen);
  }}>
    <TooltipTrigger asChild ref={(node) => { trigger.current = node; }} tabIndex={0} onBlur={close} onPointerDown={close} className="inline-flex min-w-0 rounded-sm">
      <span><ConnectionStateBadge state={state} /></span>
    </TooltipTrigger>
    <TooltipContent onEscapeKeyDown={close} onPointerDownOutside={close}>{connectionExplanation(state)}</TooltipContent>
  </Tooltip>;
}

function connectionExplanation(state: ConnectionState): string {
  if (state.stale) return `${connectionsCopy.stale} ${consentCopy.checkConnections}`;
  switch (state.status) {
    case 'connected': return consentCopy.connectedExplanation;
    case 'no-connection': return consentCopy.noConnectionExplanation;
    case 'expired': return connectionsCopy.expiredExplanation;
    case 'needs-reauthentication': return connectionsCopy.reauthenticationExplanation;
    case 'error': return connectionsCopy.unavailableExplanation;
  }
}
