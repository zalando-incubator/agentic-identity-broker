import { formatDistanceStrict } from 'date-fns';
import { ChevronDown, Unlink } from 'lucide-react';
import { AnimatedConnectionIcon } from '@components/icons/AnimatedIcons';
import { accessCopy, commonCopy } from '@copy';
import { connectionsCopy } from '@copy/connections';
import { EntityCard, EntityRow } from '@design-system/components/data-display/Entity';
import { Button } from '@design-system/components/primitives/Button';
import { Avatar } from '@design-system/components/primitives/Avatar';
import { Popover, PopoverContent, PopoverTrigger } from '@design-system/components/overlays/Popover';
import { Tooltip, TooltipContent, TooltipProvider, TooltipTrigger } from '@design-system/components/overlays/Tooltip';
import { cn } from '@design-system/utils/cn';
import type { SessionSummary } from '@services/api/sessions';
import { ConnectionStateBadge } from './ConnectionStateBadge';
import type { ConnectionState } from './connectionState';
import './ConnectionCard.css';

const dateFormatter = new Intl.DateTimeFormat(undefined, { day: 'numeric', month: 'short', year: 'numeric' });

export interface ConnectionCardProps {
  session: SessionSummary;
  state: ConnectionState;
  view: 'grid' | 'list';
  refreshing: boolean;
  disconnecting: boolean;
  onRefresh: (serviceId: string) => void;
  onDisconnect: (session: SessionSummary) => void;
  onRetry: () => void;
  updated?: { serviceId: string; animationKey: number };
}

export function ConnectionCard({ session, state, view, refreshing, disconnecting, onRefresh, onDisconnect, onRetry, updated }: ConnectionCardProps) {
  const busy = refreshing || disconnecting;
  const animated = updated?.serviceId === session.service_id;
  const avatar = <Avatar id={session.service_id} label={session.service_display_name}
    fallback={animated ? <AnimatedConnectionIcon playKey={updated?.animationKey} /> : undefined} />;
  const contextualAction = state.stale ? 'retry' : state.status === 'connected' ? null : state.action;
  const explanation = state.stale ? connectionsCopy.stale
    : state.status === 'expired' ? connectionsCopy.expiredExplanation
      : state.status === 'needs-reauthentication' ? (contextualAction === 'refresh' ? connectionsCopy.refreshExplanation : connectionsCopy.reauthenticationExplanation)
        : connectionsCopy.unavailableExplanation;
  const unavailable = state.status !== 'connected' || state.stale;
  const needsSignIn = state.status === 'needs-reauthentication' && !state.stale;
  const badge = <ConnectionStateBadge state={state} />;
  const status = needsSignIn ? <TooltipProvider><Tooltip>
    <TooltipTrigger type="button" className="inline-flex rounded-md">{badge}</TooltipTrigger>
    <TooltipContent>{explanation}</TooltipContent>
  </Tooltip></TooltipProvider> : badge;
  const scopeCount = session.scope.length > 0 ? <Popover><PopoverTrigger asChild>
    <Button data-testid="connection-scope-count" variant="ghost" size="sm" className="h-6 justify-start gap-1 rounded-sm px-0 text-xs font-normal text-muted-foreground underline underline-offset-4 hover:bg-transparent hover:text-foreground hover:underline [&_svg]:size-3" disabled={busy}>
      {connectionsCopy.scopeCount(session.scope.length)}<ChevronDown aria-hidden="true" />
    </Button>
  </PopoverTrigger><PopoverContent align="start" aria-label={connectionsCopy.scopes} className="w-72">
    <p className="text-sm font-medium">{connectionsCopy.scopes}</p>
    <p className="mt-1 break-words text-xs text-muted-foreground">{session.service_display_name}</p>
    <ul className="mt-3 max-h-64 divide-y divide-border-subtle overflow-y-auto">{session.scope.map((scope, index) => <li key={`${scope}-${index}`} className="py-2 first:pt-0 last:pb-0"><code className="block whitespace-pre-wrap break-all font-mono text-xs">{scope}</code></li>)}</ul>
  </PopoverContent></Popover> : <span data-testid="connection-scope-count" className="flex h-6 items-center text-xs">{connectionsCopy.scopeCount(0)}</span>;
  const supporting = unavailable && !needsSignIn ? <span title={view === 'grid' ? explanation : undefined} className="block w-full whitespace-normal break-words text-xs leading-4">{explanation}</span> : undefined;
  const connectedAt = new Date(session.initiated_at);
  const metadata = <span className={cn('flex min-w-0 items-center gap-y-0.5', view === 'grid' ? 'flex-wrap gap-x-3' : 'flex-col items-start')}>
    {scopeCount}
    <span className={cn('max-w-full tabular-nums', view === 'list' && 'block truncate')}>{connectionsCopy.connectedDate} <time data-testid="connection-created-at" dateTime={session.initiated_at} title={dateFormatter.format(connectedAt)}>{formatDistanceStrict(connectedAt, Date.now(), { addSuffix: true })}</time></span>
  </span>;
  const actions = <>
    {contextualAction === 'reconnect' && <Button asChild variant="outline" size="sm" disabled={busy}>
      <a role="button" data-testid="connection-action"
        href={`/api/third-party/${encodeURIComponent(session.service_id)}/oauth2/authorize?redirect_uri=${encodeURIComponent(`${window.location.origin}/connections`)}`}
        onKeyDown={event => { if (event.key === ' ') { event.preventDefault(); if (!busy) event.currentTarget.click(); } }}>
        {accessCopy.reconnect}
      </a>
    </Button>}
    {contextualAction === 'refresh' && <Button data-testid="connection-action" variant="outline" size="sm" disabled={busy} isLoading={refreshing} onClick={() => onRefresh(session.service_id)}>{accessCopy.refresh}</Button>}
    {contextualAction === 'retry' && <Button data-testid="connection-action" variant="outline" size="sm" disabled={busy} onClick={onRetry}>{commonCopy.retry}</Button>}
    <Button id={`disconnect-session-${session.id}`} variant="destructive-quiet" size="sm" disabled={busy} isLoading={disconnecting} onClick={() => onDisconnect(session)}><Unlink aria-hidden="true" />{accessCopy.disconnect}</Button>
  </>;
  const Entity = view === 'grid' ? EntityCard : EntityRow;
  return <Entity key={animated ? updated?.animationKey : undefined} id={`connection-${session.id}`} data-testid={view === 'grid' ? 'connection-card' : 'connection-row'}
    className={cn(animated && 'connection-just-updated')}
    columnTemplate={view === 'list' ? 'xl:grid-cols-[minmax(0,1.4fr)_minmax(0,8rem)_minmax(0,1fr)_minmax(12rem,14rem)]' : undefined}
    name={session.service_display_name} avatar={avatar} status={status}
    supporting={supporting} meta={metadata} busy={busy} actions={actions} />;
}
