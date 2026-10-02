import { createColumnHelper, flexRender, getCoreRowModel, useReactTable } from '@tanstack/react-table';
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@design-system/components/data-display/Table';
import { TruncatedText } from '@design-system/components/data-display/TruncatedText';
import { Badge } from '@design-system/components/primitives/Badge';
import { Button } from '@design-system/components/primitives/Button';
import { accessCopy, commonCopy } from '@copy';
import { approvalQueueCopy as copy } from '@copy/approvalQueue';
import type { ToolApprovalDetail } from '../../types/approval';

interface StandingDecisionsTableProps {
  approvals: ToolApprovalDetail[];
  label: string;
  onRevoke: (approval: ToolApprovalDetail, trigger: HTMLButtonElement) => void;
  isPending: (id: string) => boolean;
}

const column = createColumnHelper<ToolApprovalDetail>();
const columns = [
    column.accessor('tool_name', { header: copy.tool, cell: ({ getValue }) => <TruncatedText data-testid="approval-tool-name" text={getValue()} lines={1} expandLabel={commonCopy.showMore} collapseLabel={commonCopy.showLess} /> }),
    column.accessor(row => row.agent_display_name || row.agent_id, { id: 'agent', header: copy.agent, cell: ({ getValue }) => <TruncatedText data-testid="approval-agent-name" text={getValue()} lines={1} expandLabel={commonCopy.showMore} collapseLabel={commonCopy.showLess} /> }),
    column.accessor('status', { header: copy.decision, cell: ({ getValue }) => <Badge data-testid="approval-decision" variant={getValue() === 'approved' ? 'success' : 'neutral'}>{getValue() === 'approved' ? copy.allowed : copy.denied}</Badge> }),
    column.accessor('pattern_preview', { header: copy.scope, cell: ({ getValue }) => <code data-testid="approval-scope-preview" className="whitespace-pre-wrap break-all font-mono text-xs">{getValue()}</code> }),
    column.display({ id: 'actions', header: copy.actions }),
];

export function StandingDecisionsTable({ approvals, label, onRevoke, isPending }: StandingDecisionsTableProps) {
  const table = useReactTable({ data: approvals, columns, getCoreRowModel: getCoreRowModel(), getRowId: row => row.id });
  return <Table aria-label={label}>
    <TableHeader>{table.getHeaderGroups().map(group => <TableRow key={group.id}>{group.headers.map(header => <TableHead key={header.id}>{flexRender(header.column.columnDef.header, header.getContext())}</TableHead>)}</TableRow>)}</TableHeader>
    <TableBody>{table.getRowModel().rows.map(row => <TableRow key={row.id} data-testid="standing-decision-row" aria-busy={isPending(row.id)}>
      {row.getVisibleCells().map(cell => <TableCell key={cell.id} label={String(cell.column.columnDef.header)}>
        {cell.column.id === 'actions'
          ? <Button variant="outline" size="sm" isLoading={isPending(row.id)} disabled={isPending(row.id)} onClick={event => onRevoke(row.original, event.currentTarget)}>{isPending(row.id) ? copy.revoking : accessCopy.revoke}</Button>
          : flexRender(cell.column.columnDef.cell, cell.getContext())}
      </TableCell>)}
    </TableRow>)}</TableBody>
  </Table>;
}
