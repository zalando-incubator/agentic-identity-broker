import { useMemo } from 'react';
import { Link } from 'react-router-dom';
import { flexRender, getCoreRowModel, getFilteredRowModel, useReactTable, type ColumnDef } from '@tanstack/react-table';
import { accessCopy, commonCopy, navigationCopy } from '@copy';
import { delegationsCopy } from '@copy/delegations';
import { Avatar } from '@design-system/components/primitives/Avatar';
import { Button } from '@design-system/components/primitives/Button';
import { TruncatedText } from '@design-system/components/data-display/TruncatedText';
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@design-system/components/data-display/Table';
import type { AgentDelegation } from '../../types/consent';

interface DelegationsTableProps {
  delegations: AgentDelegation[];
  search: string;
  isPending: (agentId: string) => boolean;
  onRevoke: (record: AgentDelegation) => void;
}

export function DelegationsTable({ delegations, search, isPending, onRevoke }: DelegationsTableProps) {
  const columns = useMemo<ColumnDef<AgentDelegation>[]>(() => [
    {
      id: 'agent', accessorKey: 'displayName', header: delegationsCopy.agent,
      cell: ({ row }) => <div className="flex min-w-0 items-center gap-2">
        <Avatar label={row.original.displayName} src={row.original.logoUrl} size="sm" aria-hidden="true" />
        <TruncatedText text={row.original.displayName} lines={1} expandLabel={commonCopy.showMore} collapseLabel={commonCopy.showLess} className="flex min-w-0 flex-1 items-center gap-2 [&>span]:min-w-0 [&>span]:flex-1 [&>button]:mt-0 [&>button]:shrink-0" />
      </div>,
    },
    {
      id: 'expiry', header: delegationsCopy.expiry,
      cell: ({ row }) => row.original.expiresAt
        ? <time className="font-mono text-xs" dateTime={row.original.expiresAt}>{new Date(row.original.expiresAt).toLocaleDateString()}</time>
        : <span className="font-mono text-xs">{accessCopy.untilRevoked}</span>,
    },
    {
      id: 'actions', header: delegationsCopy.actions,
      cell: ({ row }) => <div className="flex flex-wrap items-center gap-1">
        <Button asChild variant="ghost" size="sm"><Link to={`/agents/${encodeURIComponent(row.original.agentId)}`}>{delegationsCopy.view}</Link></Button>
        <Button variant="ghost" size="sm" disabled={isPending(row.original.agentId)} isLoading={isPending(row.original.agentId)} onClick={() => onRevoke(row.original)}>
          {isPending(row.original.agentId) ? delegationsCopy.revoking : accessCopy.revoke}
        </Button>
      </div>,
    },
  ], [isPending, onRevoke]);
  const columnFilters = useMemo(() => search ? [{ id: 'agent', value: search }] : [], [search]);
  const table = useReactTable({
    data: delegations, columns,
    getRowId: (row) => row.agentId,
    state: { columnFilters },
    getCoreRowModel: getCoreRowModel(), getFilteredRowModel: getFilteredRowModel(),
  });

  return <Table aria-label={navigationCopy.agents}>
    <TableHeader>{table.getHeaderGroups().map((group) => <TableRow key={group.id}>
      {group.headers.map((header) => <TableHead key={header.id} className={header.id === 'agent' ? 'sm:w-1/2' : undefined}>{flexRender(header.column.columnDef.header, header.getContext())}</TableHead>)}
    </TableRow>)}</TableHeader>
    <TableBody>
      {table.getRowModel().rows.map((row) => <TableRow key={row.id} aria-busy={isPending(row.id)}>
        {row.getVisibleCells().map((cell) => <TableCell key={cell.id} data-column={cell.column.id} label={String(cell.column.columnDef.header)} className="py-1.5">
          {flexRender(cell.column.columnDef.cell, cell.getContext())}
        </TableCell>)}
      </TableRow>)}
      {table.getRowModel().rows.length === 0 && <TableRow><TableCell colSpan={3}><p role="status" className="py-4 text-muted-foreground">{delegationsCopy.noMatches}</p></TableCell></TableRow>}
    </TableBody>
  </Table>;
}
