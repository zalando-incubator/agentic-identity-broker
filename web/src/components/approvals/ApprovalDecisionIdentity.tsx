import { TruncatedText } from '@design-system/components/data-display/TruncatedText';
import { commonCopy } from '@copy';
import { approvalCopy } from '@copy/approvals';

/** Keep the technical tool identifier distinct from the agent in decision contexts. */
export function ApprovalDecisionIdentity({ toolName, agentName }: { toolName: string; agentName?: string }) {
  return <div className="min-w-0 space-y-1 text-sm">
    <TruncatedText as="p" text={toolName} lines={2} expandLabel={commonCopy.showMore} collapseLabel={commonCopy.showLess} className="font-mono" />
    {agentName && <TruncatedText as="p" text={approvalCopy.forAgent(agentName)} lines={2} expandLabel={commonCopy.showMore} collapseLabel={commonCopy.showLess} />}
  </div>;
}
