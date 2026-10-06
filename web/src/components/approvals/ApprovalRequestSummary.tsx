import { lazy, Suspense, useState } from 'react';
import { Braces, Info } from 'lucide-react';
import { TruncatedText } from '@design-system/components/data-display/TruncatedText';
import { Avatar } from '@design-system/components/primitives/Avatar';
import { Button } from '@design-system/components/primitives/Button';
import { commonCopy } from '@copy';
import { approvalCopy } from '@copy/approvals';
import { approvalQueueCopy } from '@copy/approvalQueue';
import { approvalTimeAgo } from './approvalTimeAgo';
import type { ToolApprovalDetail } from '../../types/approval';

const ApprovalRawArgumentsDialog = lazy(() => import('./ApprovalRawArgumentsDialog'));
const ApprovalSessionContext = lazy(() => import('./ApprovalSessionContext'));

function SessionContextTrigger({ approval }: { approval: ToolApprovalDetail }) {
  const [requested, setRequested] = useState(false);
  return requested ? <Suspense fallback={<Button variant="ghost" size="sm" className="h-auto min-h-8 px-1 text-xs text-muted-foreground hover:text-foreground" aria-haspopup="dialog" aria-busy="true" disabled><Info aria-hidden="true" />{approvalCopy.sessionContext}</Button>}>
    <ApprovalSessionContext approval={approval} />
  </Suspense> : <Button variant="ghost" size="sm" className="h-auto min-h-8 px-1 text-xs text-muted-foreground hover:text-foreground" aria-haspopup="dialog" onClick={() => setRequested(true)}><Info aria-hidden="true" />{approvalCopy.sessionContext}</Button>;
}

interface ApprovalRequestSummaryProps {
  approval: ToolApprovalDetail;
  inboxSelected?: boolean;
  actingPrincipal?: string;
  showExpiry?: boolean;
  showSessionContext?: boolean;
  showScope?: boolean;
}

export function ApprovalRequestSummary({ approval, actingPrincipal, inboxSelected = false, showExpiry = false, showSessionContext = false, showScope = true }: ApprovalRequestSummaryProps) {
  const [jsonRequested, setJsonRequested] = useState(false);
  const agentName = approval.agent_display_name?.trim() || approval.agent_id;
  const argumentsList = Object.entries(approval.arguments);
  const visibleArguments = argumentsList.length > 6 ? argumentsList.slice(0, 6) : argumentsList;
  const sessionContext = showSessionContext && Boolean(approval.agent_session_id || approval.mcp_session_id || approval.tool_invocation_id) && <SessionContextTrigger approval={approval} />;
  return <div className="min-w-0 space-y-4">
    {!inboxSelected && <>
    <div className="flex flex-wrap items-start justify-between gap-3">
      <div className="flex min-w-0 flex-1 items-start gap-3">
        <Avatar id={approval.agent_id} label={agentName} fallback={agentName.charAt(0).toUpperCase()} size="sm" />
        <div className="min-w-0">
          <p className="text-xs text-muted-foreground">{approvalCopy.requestedBy}</p>
          <TruncatedText text={agentName} as="p" lines={2} expandLabel={commonCopy.showMore} collapseLabel={commonCopy.showLess} className="text-sm font-semibold [&>p]:[overflow-wrap:anywhere]" data-testid="approval-agent-name" />
          <time dateTime={approval.created_at} title={new Date(approval.created_at).toLocaleString()} data-screenshot-dynamic className="text-xs tabular-nums text-muted-foreground">{approvalCopy.requestedAt(approvalTimeAgo(approval.created_at))}</time>
        </div>
      </div>
    </div>
    {actingPrincipal && <div>
      <p className="text-xs text-muted-foreground">{approvalCopy.actingUser}</p>
      <p data-testid="approval-acting-user" className="break-all text-sm">{actingPrincipal}</p>
    </div>}
    {approval.description?.trim() && <div className="space-y-1">
      <p className="text-xs text-muted-foreground">{approvalCopy.action}</p>
      <TruncatedText text={approval.description} as="p" lines={2} expandLabel={commonCopy.showMore} collapseLabel={commonCopy.showLess} />
    </div>}
    <div className="min-w-0 space-y-1">
      <div className="flex flex-wrap items-center justify-between gap-x-3 gap-y-1">
        <p className="text-xs text-muted-foreground">{approvalCopy.technicalDetails}</p>
        {sessionContext}
      </div>
      <TruncatedText text={approval.tool_name} as="h2" lines={2} expandLabel={commonCopy.showMore} collapseLabel={commonCopy.showLess} className="font-mono text-sm font-semibold" data-testid="approval-tool-name" />
    </div>
    </>}
    <section aria-label={approvalCopy.arguments} className="min-w-0 space-y-2 border-t border-border-subtle pt-3 first:border-0 first:pt-0">
      <div className="flex flex-wrap items-center justify-between gap-x-3 gap-y-1">
        <h3 className="text-xs font-semibold">{approvalCopy.arguments}</h3>
        {jsonRequested ? <Suspense fallback={<Button variant="ghost" size="sm" className="h-auto min-h-8 px-1 text-xs text-muted-foreground hover:text-foreground" aria-haspopup="dialog" aria-busy="true" disabled><Braces aria-hidden="true" />{approvalQueueCopy.viewJson}</Button>}>
          <ApprovalRawArgumentsDialog arguments={approval.arguments} />
        </Suspense> : <Button variant="ghost" size="sm" className="h-auto min-h-8 px-1 text-xs text-muted-foreground hover:text-foreground" aria-haspopup="dialog" onClick={() => setJsonRequested(true)}><Braces aria-hidden="true" />{approvalQueueCopy.viewJson}</Button>}
      </div>
      {argumentsList.length === 0 && <p className="text-sm text-muted-foreground">{approvalQueueCopy.noArguments}</p>}
      {visibleArguments.length > 0 && <dl className="space-y-2 text-sm">
        {visibleArguments.map(([key, value]) => <div key={key} className="grid min-w-0 grid-cols-[minmax(0,2fr)_minmax(0,3fr)] gap-2 border-b border-border-subtle pb-2 last:border-b-0">
          <dt className="min-w-0 break-all font-mono text-muted-foreground">{key}</dt>
          <dd className="min-w-0 whitespace-pre-wrap break-all font-mono">{typeof value === 'string' ? value : JSON.stringify(value) ?? String(value)}</dd>
        </div>)}
      </dl>}
      {argumentsList.length > 6 && <p className="text-xs text-muted-foreground">{approvalCopy.moreArguments(argumentsList.length - 6)}</p>}
    </section>
    {inboxSelected && sessionContext && <div>{sessionContext}</div>}
    {showScope && <div aria-label={approvalCopy.patternPreview} className="space-y-1 border-t border-border-subtle pt-3">
      <p className="text-xs text-muted-foreground">{approvalCopy.covers}</p>
      <code data-testid="approval-scope-preview" className="block whitespace-pre-wrap font-mono text-xs [overflow-wrap:anywhere]">{approval.pattern_preview}</code>
    </div>}
    {showExpiry && !inboxSelected && <p className="text-xs tabular-nums text-muted-foreground" data-screenshot-dynamic><time dateTime={approval.expires_at}>{approvalCopy.expiry(new Date(approval.expires_at).toLocaleString())}</time></p>}
  </div>;
}
