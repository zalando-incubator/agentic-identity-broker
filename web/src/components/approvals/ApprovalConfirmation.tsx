import { Card, CardContent } from '@design-system/components/data-display/Card';
import { TruncatedText } from '@design-system/components/data-display/TruncatedText';
import { Badge } from '@design-system/components/primitives/Badge';
import { commonCopy } from '@copy';
import { approvalCopy } from '@copy/approvals';
import type { ApprovalPersistence } from '../../types/approval';

interface ApprovalConfirmationProps {
  type: 'approved' | 'denied';
  persistence?: ApprovalPersistence | null;
  decidedAt?: string | null;
  toolName: string;
  agentName?: string;
  historical?: boolean;
  consumed?: boolean;
}

export function ApprovalConfirmation({ type, persistence, decidedAt, toolName, agentName, historical = false, consumed = false }: ApprovalConfirmationProps) {
  const approved = type === 'approved';
  const outcome = approved ? approvalCopy.approved : approvalCopy.denied;
  return <Card data-testid="approval-outcome" role="status">
    <CardContent className="space-y-4 pt-6">
      <p className="text-sm text-muted-foreground">{approvalCopy.recorded}</p>
      <h2 className="font-display text-xl font-semibold">{outcome}</h2>
      <TruncatedText as="p" text={approvalCopy.decisionContext(toolName, agentName)} lines={2} expandLabel={commonCopy.showMore} collapseLabel={commonCopy.showLess} className="text-sm" />
      {persistence && <Badge variant="neutral">{approvalCopy.persistenceValue[persistence]}</Badge>}
      {decidedAt && <p className="text-xs text-muted-foreground" data-screenshot-dynamic>{approvalCopy.decidedAt(outcome, new Date(decidedAt).toLocaleString())}</p>}
      {approved && persistence === 'once' && <p className="text-sm">{consumed ? approvalCopy.consumed : approvalCopy.nextInvocation}</p>}
      <p className="text-sm">{approvalCopy.readonly}</p>
      <p className="text-sm">{historical ? approvalCopy.returnToAgent : approvalCopy.closePage}</p>
      {persistence === 'permanent' && <p className="text-sm text-muted-foreground">{approvalCopy.managePermanent}</p>}
    </CardContent>
  </Card>;
}
