import { ClockAlert, ShieldOff } from 'lucide-react';
import { formatDistanceStrict } from 'date-fns';
import type { Ref } from 'react';
import type { RevokeAgentTarget } from '@hooks/useRevokeGrant';
import { accessCopy, collectionCopy, commonCopy, navigationCopy } from '@copy';
import { delegationsCopy } from '@copy/delegations';
import { EntityCard, EntityRow } from '@design-system/components/data-display/Entity';
import { CollectionToolbar } from '@design-system/components/data-display/CollectionToolbar';
import { Alert } from '@design-system/components/feedback/Alert';
import { CollectionSkeleton } from '@design-system/components/feedback/CollectionSkeleton';
import { EmptyState } from '@design-system/components/feedback/EmptyState';
import { AnimatedBotIcon } from '@components/icons/AnimatedIcons';
import { PageHeader } from '@design-system/components/layout/PageHeader';
import { Button } from '@design-system/components/primitives/Button';
import { AnimatedCollection } from '@components/layout/AnimatedCollection';
import { RevokeAgentDialog } from '@components/consent/RevokeAgentDialog';
import type { AgentDelegation } from '../types/consent';

export type AgentSort = 'name' | 'recent' | 'expiring';

export const agentSortOptions = [
  { value: 'name', label: collectionCopy.name },
  { value: 'recent', label: collectionCopy.recentlyChanged },
  { value: 'expiring', label: collectionCopy.expiringSoonest },
] as const;

const dateFormatter = new Intl.DateTimeFormat(undefined, { day: 'numeric', month: 'short', year: 'numeric' });
const dayMilliseconds = 24 * 60 * 60 * 1000;

export interface AgentsViewProps {
  delegations: readonly AgentDelegation[];
  loading: boolean;
  error: boolean;
  onRetry: () => void;
  search: string;
  onSearchChange: (search: string) => void;
  sort: AgentSort;
  onSortChange: (sort: AgentSort) => void;
  view: 'grid' | 'list';
  onViewChange: (view: 'grid' | 'list') => void;
  onRevoke: (record: AgentDelegation) => void;
  isPending: (agentId: string) => boolean;
  confirmation: RevokeAgentTarget | null;
  onCancelRevoke: () => void;
  onConfirmRevoke: () => void | Promise<void>;
  onReturnFocus?: () => void;
  headingRef?: Ref<HTMLDivElement>;
  now?: number;
}

export function AgentsView({ delegations, loading, error, onRetry, search, onSearchChange, sort, onSortChange, view, onViewChange, onRevoke, isPending, confirmation, onCancelRevoke, onConfirmRevoke, onReturnFocus, headingRef, now = Date.now() }: AgentsViewProps) {
  const filtered = delegations.filter(record => record.displayName.toLocaleLowerCase().includes(search.trim().toLocaleLowerCase()));
  const visible = [...filtered].sort((a, b) => {
    if (sort === 'recent') return Date.parse(b.lastModifiedAt) - Date.parse(a.lastModifiedAt) || a.displayName.localeCompare(b.displayName);
    if (sort === 'expiring') return (a.expiresAt ? Date.parse(a.expiresAt) : Infinity) - (b.expiresAt ? Date.parse(b.expiresAt) : Infinity) || a.displayName.localeCompare(b.displayName);
    return a.displayName.localeCompare(b.displayName);
  });
  const Entity = view === 'grid' ? EntityCard : EntityRow;

  return <>
    <div ref={headingRef} tabIndex={-1} className="outline-none focus-visible:ring-2 focus-visible:ring-ring">
      <PageHeader title={navigationCopy.agents} count={!loading && !error || delegations.length ? delegations.length : undefined} actions={<CollectionToolbar
        search={search} onSearchChange={onSearchChange} sort={sort} onSortChange={(value) => onSortChange(value as AgentSort)} sortOptions={agentSortOptions}
        view={view} onViewChange={onViewChange} labels={{ ...collectionCopy, search: delegationsCopy.searchAgents }}
      />} />
    </div>
    {loading && delegations.length === 0 ? <CollectionSkeleton label={commonCopy.loading} view={view} cardSize="compact" /> : <>
      {error && <Alert variant="error" className="mb-4" action={{ label: commonCopy.retry, onClick: onRetry }}>{delegations.length ? delegationsCopy.stale : delegationsCopy.loadFailure}</Alert>}
      {delegations.length === 0 && !error ? <EmptyState data-testid="delegations-empty-state" title={delegationsCopy.emptyTitle} description={delegationsCopy.purpose} icon={<AnimatedBotIcon />} /> : null}
      {delegations.length > 0 && visible.length === 0 ? <EmptyState size="compact" title={collectionCopy.noResults} secondaryAction={{ label: collectionCopy.clearSearch, onClick: () => onSearchChange('') }} /> : null}
      {visible.length > 0 && <AnimatedCollection data-testid="agents-collection" aria-label={navigationCopy.agents} className={view === 'grid' ? 'grid grid-cols-[repeat(auto-fill,minmax(min(100%,320px),1fr))] gap-4' : 'flex flex-col gap-3'}>
        {visible.map(record => {
          const expiresAt = record.expiresAt ? new Date(record.expiresAt) : null;
          const expires = expiresAt?.getTime();
          const nearExpiry = expires !== undefined && expires > now && expires - now <= 7 * dayMilliseconds;
          const changedAt = new Date(record.lastModifiedAt);
          const changedFull = changedAt.toLocaleString();
          const pending = isPending(record.agentId);
          const href = `/agents/${encodeURIComponent(record.agentId)}`;
          return <Entity key={record.agentId} data-testid="agent-entity" id={record.agentId} name={record.displayName} href={href}
            cardSize="compact"
            nameClassName={view === 'list' ? 'text-base' : undefined}
            columnTemplate={view === 'list' ? 'xl:grid-cols-[minmax(0,1fr)_minmax(0,1fr)_minmax(0,1fr)]' : undefined}
            className={view === 'grid' ? 'h-[120px] max-[240px]:h-auto max-[240px]:min-h-[120px]' : undefined}
            busy={pending} supporting={<span data-testid="agent-expiry" className={nearExpiry ? 'inline-flex max-w-full items-center gap-1 text-xs font-normal text-warning-foreground max-[240px]:flex-wrap' : 'inline-block max-w-full text-xs font-normal'}>
              {nearExpiry && <><ClockAlert aria-hidden="true" className="size-3 shrink-0" /><span>{delegationsCopy.expiresSoon} ·</span></>}
              {expiresAt ? <span>{!nearExpiry && `${delegationsCopy.expires} `}<time dateTime={record.expiresAt} title={expiresAt.toLocaleString()}>{dateFormatter.format(expiresAt)}<span className="sr-only">, {expiresAt.toLocaleTimeString()}</span></time></span> : accessCopy.untilRevoked}
            </span>}
            meta={<span className="text-xs tabular-nums text-muted-foreground">{delegationsCopy.changed} <span className="sr-only">{changedFull}, </span><time data-testid="agent-changed-at" dateTime={record.lastModifiedAt} title={changedFull}>{formatDistanceStrict(changedAt, now, { addSuffix: true })}</time></span>}
            actions={<Button type="button" variant="destructive-quiet" size="sm" onClick={() => onRevoke(record)}><ShieldOff aria-hidden="true" />{accessCopy.revoke}</Button>} />;
        })}
      </AnimatedCollection>}
    </>}
    <RevokeAgentDialog open={confirmation !== null} agentName={confirmation?.displayName ?? ''} pending={confirmation !== null && isPending(confirmation.agentId)} onCancel={onCancelRevoke} onConfirm={onConfirmRevoke} onReturnFocus={onReturnFocus} />
  </>;
}
