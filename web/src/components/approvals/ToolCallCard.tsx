import { Card, CardContent } from '@design-system/components/data-display/Card';
import type { ToolApprovalDetail } from '../../types/approval';
import { ApprovalRequestSummary } from './ApprovalRequestSummary';

interface ToolCallCardProps {
  approval: ToolApprovalDetail;
  actingPrincipal: string;
  framed?: boolean;
  showExpiry?: boolean;
  showSessionContext?: boolean;
  showScope?: boolean;
}

export function ToolCallCard({ approval, actingPrincipal, framed = false, showExpiry = true, showSessionContext = true, showScope = true }: ToolCallCardProps) {
  const content = <ApprovalRequestSummary approval={approval} actingPrincipal={actingPrincipal} showExpiry={showExpiry} showSessionContext={showSessionContext} showScope={showScope} />;
  return framed ? <Card><CardContent className="pt-4">{content}</CardContent></Card> : content;
}
