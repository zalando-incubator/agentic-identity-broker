import { Button } from '@design-system/components/primitives/Button';
import { Info } from 'lucide-react';
import { Popover, PopoverContent, PopoverTrigger } from '@design-system/components/overlays/Popover';
import { approvalCopy } from '@copy/approvals';
import type { ToolApprovalDetail } from '../../types/approval';

interface ApprovalSessionContextProps {
  approval: ToolApprovalDetail;
}

export default function ApprovalSessionContext({ approval }: ApprovalSessionContextProps) {
  const entries = [
    { label: approvalCopy.agentSession, value: approval.agent_session_id },
    { label: approvalCopy.mcpSession, value: approval.mcp_session_id },
    { label: approvalCopy.toolInvocation, value: approval.tool_invocation_id },
  ].filter((entry) => Boolean(entry.value));

  return <Popover defaultOpen>
    <PopoverTrigger asChild><Button variant="ghost" size="sm" className="h-auto min-h-8 px-1 text-xs text-muted-foreground hover:text-foreground"><Info aria-hidden="true" />{approvalCopy.sessionContext}</Button></PopoverTrigger>
    <PopoverContent align="start" aria-label={approvalCopy.sessionContext}><dl className="space-y-2 text-sm">
      {entries.map(({ label, value }) => <div key={label} className="min-w-0 space-y-1">
        <dt className="text-muted-foreground">{label}</dt><dd className="break-all font-mono">{value}</dd>
      </div>)}
    </dl></PopoverContent>
  </Popover>;
}
