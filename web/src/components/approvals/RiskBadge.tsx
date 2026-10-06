import { CircleAlert, Minus } from 'lucide-react';
import { useId } from 'react';
import { Badge } from '@design-system/components/primitives/Badge';
import { accessCopy } from '@copy';
import { approvalCopy } from '@copy/approvals';
import type { RiskLevel } from '../../types/approval';

interface RiskBadgeProps { level?: RiskLevel | null }
const labels = { low: approvalCopy.lowRisk, medium: approvalCopy.mediumRisk, high: approvalCopy.highRisk, critical: approvalCopy.criticalRisk };

export function RiskBadge({ level }: RiskBadgeProps) {
  const explanationId = useId();
  const normalized = level?.trim().toLowerCase();
  const label = !normalized ? accessCopy.riskNotRated
    : normalized === 'low' || normalized === 'medium' || normalized === 'high' || normalized === 'critical'
      ? labels[normalized]
      : approvalCopy.otherRisk(level!.trim());
  const variant = normalized === 'low' ? 'risk-low' : normalized === 'medium' ? 'risk-medium' : normalized === 'critical' || normalized === 'high' ? 'risk-high' : 'neutral';
  return <>
    <Badge variant={variant} icon={normalized === 'critical' ? <CircleAlert fill="currentColor" className="text-status-danger-foreground [&_circle]:stroke-status-danger [&_line]:stroke-status-danger" /> : !normalized ? <Minus /> : undefined} data-testid="approval-risk" aria-describedby={explanationId}>{label}</Badge>
    <span id={explanationId} className="sr-only">{normalized ? approvalCopy.riskExplanation : approvalCopy.unratedExplanation}</span>
  </>;
}
