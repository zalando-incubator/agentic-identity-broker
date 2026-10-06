import { X } from 'lucide-react';
import type { ReactNode, Ref } from 'react';
import { AnimatedConnectionIcon } from '@components/icons/AnimatedIcons';
import { AnimatedCollection } from '@components/layout/AnimatedCollection';
import { ConnectionCard } from '@components/sessions/ConnectionCard';
import type { ConnectionState } from '@components/sessions/connectionState';
import { collectionCopy, commonCopy, navigationCopy } from '@copy';
import { connectionsCopy } from '@copy/connections';
import { CollectionToolbar } from '@design-system/components/data-display/CollectionToolbar';
import { Alert } from '@design-system/components/feedback/Alert';
import { CollectionSkeleton } from '@design-system/components/feedback/CollectionSkeleton';
import { EmptyState } from '@design-system/components/feedback/EmptyState';
import { PageHeader } from '@design-system/components/layout/PageHeader';
import { Button } from '@design-system/components/primitives/Button';
import type { SessionSummary } from '@services/api/sessions';

export type ConnectionSort = 'attention' | 'recent' | 'name';
export type ConnectionFilter = '' | 'connected' | 'needs-reauthentication' | 'expired' | 'error';

export const connectionSortOptions = [
  { value: 'attention', label: connectionsCopy.sortAttention },
  { value: 'recent', label: connectionsCopy.sortRecentlyConnected },
  { value: 'name', label: collectionCopy.name },
] as const;

export const connectionFilterOptions = [
  { value: 'connected', label: connectionsCopy.filterConnected },
  { value: 'needs-reauthentication', label: connectionsCopy.filterNeedsSignIn },
  { value: 'expired', label: connectionsCopy.filterExpired },
  { value: 'error', label: connectionsCopy.filterUnavailable },
] as const;

const filterLabels: Record<Exclude<ConnectionFilter, ''>, string> = {
  connected: connectionsCopy.filterConnected,
  'needs-reauthentication': connectionsCopy.filterNeedsSignIn,
  expired: connectionsCopy.filterExpired,
  error: connectionsCopy.filterUnavailable,
};
const attentionRank: Record<ConnectionState['status'], number> = {
  error: 0, expired: 1, 'needs-reauthentication': 2, 'no-connection': 3, connected: 4,
};

export interface ConnectionsViewProps {
  sessions: readonly SessionSummary[];
  loading: boolean;
  error: boolean;
  onRetry: () => void;
  getState: (serviceId: string) => ConnectionState;
  isRefreshing: (serviceId: string) => boolean;
  isDisconnecting: (serviceId: string) => boolean;
  onRefresh: (serviceId: string) => void;
  onDisconnect: (session: SessionSummary) => void;
  search: string;
  onSearchChange: (value: string) => void;
  sort: ConnectionSort;
  onSortChange: (value: ConnectionSort) => void;
  filter: ConnectionFilter;
  onFilterChange: (value: ConnectionFilter) => void;
  view: 'grid' | 'list';
  onViewChange: (view: 'grid' | 'list') => void;
  updated?: { serviceId: string; animationKey: number };
  dialog?: ReactNode;
  headingRef?: Ref<HTMLDivElement>;
}

export function ConnectionsView({ sessions, loading, error, onRetry, getState, isRefreshing, isDisconnecting, onRefresh, onDisconnect, search, onSearchChange, sort, onSortChange, filter, onFilterChange, view, onViewChange, updated, dialog, headingRef }: ConnectionsViewProps) {
  const term = search.trim().toLocaleLowerCase();
  const entries = sessions.map(session => ({ session, state: getState(session.service_id) }));
  const visible = entries.filter(({ session, state }) => {
    const status = state.stale ? 'error' : state.status;
    return (!term || session.service_display_name.toLocaleLowerCase().includes(term) || session.service_id.toLocaleLowerCase().includes(term))
      && (!filter || filter === status);
  }).sort((left, right) => {
    const names = left.session.service_display_name.localeCompare(right.session.service_display_name) || left.session.service_id.localeCompare(right.session.service_id);
    if (sort === 'name') return names;
    if (sort === 'recent') return Date.parse(right.session.initiated_at) - Date.parse(left.session.initiated_at) || names;
    const leftRank = left.state.stale ? 0 : attentionRank[left.state.status];
    const rightRank = right.state.stale ? 0 : attentionRank[right.state.status];
    return leftRank - rightRank || names;
  });

  return <>
    <div ref={headingRef} tabIndex={-1} className="outline-none focus-visible:ring-2 focus-visible:ring-ring">
      <PageHeader title={navigationCopy.connections} count={!loading && !error || sessions.length ? sessions.length : undefined}
        actions={<CollectionToolbar search={search} onSearchChange={onSearchChange} sort={sort} onSortChange={value => onSortChange(value as ConnectionSort)}
          sortOptions={connectionSortOptions} view={view} onViewChange={onViewChange} filters={connectionFilterOptions}
          filter={filter} onFilterChange={value => onFilterChange(value as ConnectionFilter)} labels={collectionCopy} />} />
    </div>
    {filter && <div aria-label={connectionsCopy.activeFilters} className="mb-4 flex flex-wrap items-center gap-2">
      <Button variant="outline" size="sm" onClick={() => onFilterChange('')}><X aria-hidden="true" />{collectionCopy.filter}: {filterLabels[filter]}</Button>
    </div>}
    {loading && sessions.length === 0 ? <CollectionSkeleton label={commonCopy.loading} view={view} /> : <>
      {error && <Alert variant="error" className="mb-4" action={{ label: commonCopy.retry, onClick: onRetry }}>{sessions.length ? connectionsCopy.stale : connectionsCopy.readError}</Alert>}
      {sessions.length === 0 && !error && <EmptyState data-testid="connections-empty-state" title={connectionsCopy.emptyTitle} description={connectionsCopy.empty} icon={<AnimatedConnectionIcon size={32} />} />}
      {sessions.length > 0 && visible.length === 0 && <EmptyState size="compact" title={collectionCopy.noResults}>
        <div className="flex flex-wrap justify-center gap-3">
          {search && <Button variant="outline" onClick={() => onSearchChange('')}>{collectionCopy.clearSearch}</Button>}
          {filter && <Button variant="outline" onClick={() => onFilterChange('')}>{collectionCopy.clearFilter}</Button>}
        </div>
      </EmptyState>}
      {visible.length > 0 && <AnimatedCollection data-testid="connections-collection" aria-label={navigationCopy.connections}
        className={view === 'grid' ? 'grid grid-cols-[repeat(auto-fill,minmax(min(100%,320px),1fr))] gap-4' : 'flex flex-col gap-3'}>
        {visible.map(({ session, state }) => <ConnectionCard key={session.service_id} session={session} state={state} view={view}
          refreshing={isRefreshing(session.service_id)} disconnecting={isDisconnecting(session.service_id)}
          onRefresh={onRefresh} onDisconnect={onDisconnect} onRetry={onRetry} updated={updated} />)}
      </AnimatedCollection>}
    </>}
    {dialog}
  </>;
}
