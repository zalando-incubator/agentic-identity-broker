import { flexRender, getCoreRowModel, useReactTable, type ColumnDef } from '@tanstack/react-table';
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@design-system/components/data-display/Table';
import { TruncatedText } from '@design-system/components/data-display/TruncatedText';
import { Button } from '@design-system/components/primitives/Button';
import { accessCopy, commonCopy, navigationCopy } from '@copy';
import { connectionsCopy } from '@copy/connections';
import type { SessionSummary } from '@services/api/sessions';
import type { ConnectionState } from './connectionState';
import { ConnectionStateBadge } from './ConnectionStateBadge';

export interface ConnectionsTableProps {
  sessions: SessionSummary[];
  getState: (serviceId: string) => ConnectionState;
  isRefreshing: (serviceId: string) => boolean;
  isDisconnecting: (serviceId: string) => boolean;
  onRefresh: (serviceId: string) => void;
  onDisconnect: (session: SessionSummary) => void;
  onRetry: () => void;
}

const createdFormatter = new Intl.DateTimeFormat(undefined, { dateStyle: 'medium', timeStyle: 'short' });

export function ConnectionsTable({ sessions, getState, isRefreshing, isDisconnecting, onRefresh, onDisconnect, onRetry }: ConnectionsTableProps) {
  const columns: ColumnDef<SessionSummary>[] = [
    { id: 'provider', header: connectionsCopy.provider, cell: ({ row }) => <div data-testid="connection-provider">
      <TruncatedText text={row.original.service_display_name} lines={1} expandLabel={commonCopy.showMore} collapseLabel={commonCopy.showLess} />
    </div> },
    { id: 'scopes', header: connectionsCopy.scopes, cell: ({ row }) => <span data-testid="connection-scope-count">{connectionsCopy.scopeCount(row.original.scope.length)}</span> },
    { id: 'state', header: connectionsCopy.state, cell: ({ row }) => <ConnectionStateBadge state={getState(row.original.service_id)} /> },
    { id: 'created', header: connectionsCopy.created, cell: ({ row }) => <time data-testid="connection-created-at" className="font-mono text-xs" dateTime={row.original.initiated_at}>{createdFormatter.format(new Date(row.original.initiated_at))}</time> },
    { id: 'actions', header: connectionsCopy.actions, cell: ({ row }) => {
      const session = row.original;
      const state = getState(session.service_id);
      const refreshing = isRefreshing(session.service_id);
      const disconnecting = isDisconnecting(session.service_id);
      const pending = refreshing || disconnecting;
      return <div className="flex flex-wrap gap-2">
        {state.action === 'reconnect' && <Button asChild variant="outline" size="sm">
          <a role="button" data-testid="connection-action" href={`/api/third-party/${encodeURIComponent(session.service_id)}/oauth2/authorize?redirect_uri=${encodeURIComponent(`${window.location.origin}/sessions`)}`} aria-disabled={pending || undefined}
            onClick={event => { if (pending) event.preventDefault(); }}
            onKeyDown={event => { if (event.key === ' ') { event.preventDefault(); if (!pending) event.currentTarget.click(); } }}>
            {accessCopy.reconnect}
          </a>
        </Button>}
        {state.action === 'refresh' && <Button data-testid="connection-action" variant="outline" size="sm" disabled={pending} isLoading={refreshing} onClick={() => onRefresh(session.service_id)}>{accessCopy.refresh}</Button>}
        {state.action === 'retry' && <Button data-testid="connection-action" variant="outline" size="sm" disabled={pending} onClick={onRetry}>{commonCopy.retry}</Button>}
        <Button variant="ghost" size="sm" disabled={pending} isLoading={disconnecting} onClick={() => onDisconnect(session)}>{accessCopy.disconnect}</Button>
      </div>;
    } },
  ];
  const table = useReactTable({ data: sessions, columns, getCoreRowModel: getCoreRowModel(), getRowId: session => session.service_id });
  return <Table data-testid="connections-table" aria-label={navigationCopy.connections}>
    <TableHeader>{table.getHeaderGroups().map(group => <TableRow key={group.id}>{group.headers.map(header =>
      <TableHead key={header.id}>{flexRender(header.column.columnDef.header, header.getContext())}</TableHead>,
    )}</TableRow>)}</TableHeader>
    <TableBody>{table.getRowModel().rows.map(row => <TableRow key={row.id} data-testid="connection-row" aria-busy={isRefreshing(row.id) || isDisconnecting(row.id)}>
      {row.getVisibleCells().map(cell => <TableCell key={cell.id} label={String(cell.column.columnDef.header)}>{flexRender(cell.column.columnDef.cell, cell.getContext())}</TableCell>)}
    </TableRow>)}</TableBody>
  </Table>;
}
