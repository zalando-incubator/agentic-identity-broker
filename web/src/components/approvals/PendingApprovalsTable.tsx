import { createColumnHelper, flexRender, getCoreRowModel, useReactTable } from '@tanstack/react-table';
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@design-system/components/data-display/Table';
import { TruncatedText } from '@design-system/components/data-display/TruncatedText';
import { commonCopy } from '@copy';
import { approvalQueueCopy as copy } from '@copy/approvalQueue';
import type { ToolApprovalDetail } from '../../types/approval';
import { InlineApprovalActions } from './InlineApprovalActions';
import { RiskBadge } from './RiskBadge';

function requestAge(createdAt: string): string {
  const elapsed = Date.now() - Date.parse(createdAt);
  if (!Number.isFinite(elapsed)) return copy.ageUnknown;
  const minutes = Math.max(0, Math.floor(elapsed / 60_000));
  if (minutes < 1) return copy.ageSeconds;
  if (minutes < 60) return copy.ageMinutes(minutes);
  const hours = Math.floor(minutes / 60);
  return hours < 24 ? copy.ageHours(hours) : copy.ageDays(Math.floor(hours / 24));
}

const column = createColumnHelper<ToolApprovalDetail>();
const columns = [
  column.accessor('tool_name', { header: copy.tool, cell: ({ getValue }) => <TruncatedText data-testid="approval-tool-name" text={getValue()} lines={1} expandLabel={commonCopy.showMore} collapseLabel={commonCopy.showLess} /> }),
  column.accessor(row => row.agent_display_name || row.agent_id, { id: 'agent', header: copy.agent, cell: ({ getValue }) => <TruncatedText data-testid="approval-agent-name" text={getValue()} lines={1} expandLabel={commonCopy.showMore} collapseLabel={commonCopy.showLess} /> }),
  column.accessor('risk_level', { header: copy.risk, cell: ({ getValue }) => <RiskBadge level={getValue()} /> }),
  column.accessor('created_at', { header: copy.age, cell: ({ getValue }) => <time dateTime={getValue()} title={new Date(getValue()).toLocaleString()}>{requestAge(getValue())}</time> }),
  column.display({ id: 'actions', header: copy.actions, cell: ({ row }) => <InlineApprovalActions approval={row.original} /> }),
];

export function PendingApprovalsTable({ approvals }: { approvals: ToolApprovalDetail[] }) {
  const table = useReactTable({ data: approvals, columns, getCoreRowModel: getCoreRowModel(), getRowId: row => row.id });
  return <Table data-testid="pending-approvals" aria-label={copy.pendingTitle}>
    <TableHeader>{table.getHeaderGroups().map(group => <TableRow key={group.id}>{group.headers.map(header => <TableHead key={header.id} className={header.id === 'actions' ? 'sm:w-2/5' : undefined}>{flexRender(header.column.columnDef.header, header.getContext())}</TableHead>)}</TableRow>)}</TableHeader>
    <TableBody>{table.getRowModel().rows.map(row => <TableRow key={row.id} data-testid="pending-approval-row">{row.getVisibleCells().map(cell => <TableCell key={cell.id} label={String(cell.column.columnDef.header)} className="align-top">{flexRender(cell.column.columnDef.cell, cell.getContext())}</TableCell>)}</TableRow>)}</TableBody>
  </Table>;
}
