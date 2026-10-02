import { Accordion, AccordionItem, AccordionTrigger, AccordionContent } from '@design-system/components/advanced/Accordion';
import { TruncatedText } from '@design-system/components/data-display/TruncatedText';
import { Avatar } from '@design-system/components/primitives/Avatar';
import { commonCopy } from '@copy';
import { approvalCopy } from '@copy/approvals';
import type { ToolApprovalDetail } from '../../types/approval';
import { RiskBadge } from './RiskBadge';

interface ApprovalRequestSummaryProps {
  approval: ToolApprovalDetail;
  actingPrincipal?: string;
  showExpiry?: boolean;
  showSessionContext?: boolean;
  showScope?: boolean;
}

export function ApprovalRequestSummary({ approval, actingPrincipal, showExpiry = false, showSessionContext = false, showScope = true }: ApprovalRequestSummaryProps) {
  const agentName = approval.agent_display_name?.trim() || approval.agent_id;
  const sessionEntries = [
    { label: approvalCopy.agentSession, value: approval.agent_session_id },
    { label: approvalCopy.mcpSession, value: approval.mcp_session_id },
    { label: approvalCopy.toolInvocation, value: approval.tool_invocation_id },
  ].filter((entry) => Boolean(entry.value));
  return <div className="min-w-0 space-y-4">
    <div className="flex flex-wrap items-start justify-between gap-3">
      <div className="flex min-w-0 flex-1 items-start gap-3">
        <Avatar label={agentName} fallback={agentName.charAt(0).toUpperCase()} size="sm" />
        <div className="min-w-0">
          <p className="text-xs text-muted-foreground">{approvalCopy.requestedBy}</p>
          <TruncatedText text={agentName} as="p" lines={2} expandLabel={commonCopy.showMore} collapseLabel={commonCopy.showLess} className="text-sm font-semibold [&>p]:[overflow-wrap:anywhere]" data-testid="approval-agent-name" />
        </div>
      </div>
      <RiskBadge level={approval.risk_level} />
    </div>
    {actingPrincipal && <div>
      <p className="text-xs text-muted-foreground">{approvalCopy.actingUser}</p>
      <p data-testid="approval-acting-user" className="break-all text-sm">{actingPrincipal}</p>
    </div>}
    {approval.description?.trim() && <div className="space-y-1">
      <p className="text-xs text-muted-foreground">{approvalCopy.action}</p>
      <TruncatedText text={approval.description} as="p" lines={2} expandLabel={commonCopy.showMore} collapseLabel={commonCopy.showLess} />
    </div>}
    <div className="space-y-1">
      <p className="text-xs text-muted-foreground">{approvalCopy.technicalDetails}</p>
      <TruncatedText text={approval.tool_name} as="h3" lines={2} expandLabel={commonCopy.showMore} collapseLabel={commonCopy.showLess} className="font-mono text-sm font-semibold" data-testid="approval-tool-name" />
    </div>
    <Accordion type="single" collapsible>
      <AccordionItem value="arguments">
        <AccordionTrigger>{approvalCopy.arguments}</AccordionTrigger>
        <AccordionContent>
          <pre data-testid="approval-arguments" className="max-h-80 overflow-auto whitespace-pre-wrap rounded-md border border-border bg-muted p-3 font-mono text-xs [overflow-wrap:anywhere]">{JSON.stringify(approval.arguments, null, 2)}</pre>
        </AccordionContent>
      </AccordionItem>
    </Accordion>
    {showScope && <div aria-label={approvalCopy.patternPreview} className="space-y-1 border-t border-border-soft pt-3">
      <p className="text-xs text-muted-foreground">{approvalCopy.covers}</p>
      <code data-testid="approval-scope-preview" className="block whitespace-pre-wrap font-mono text-xs [overflow-wrap:anywhere]">{approval.pattern_preview}</code>
    </div>}
    {showSessionContext && sessionEntries.length > 0 && <div className="space-y-2">
      <p className="text-xs text-muted-foreground">{approvalCopy.sessionContext}</p>
      <dl className="space-y-2 text-sm">
        {sessionEntries.map(({ label, value }) => <div key={label} className="flex flex-wrap justify-between gap-2">
          <dt className="text-muted-foreground">{label}</dt><dd className="break-all font-mono">{value}</dd>
        </div>)}
      </dl>
    </div>}
    {showExpiry && <p className="text-xs text-muted-foreground" data-screenshot-dynamic>{approvalCopy.expiry(new Date(approval.expires_at).toLocaleString())}</p>}
  </div>;
}
