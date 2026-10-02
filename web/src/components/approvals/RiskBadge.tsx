import { useId } from 'react';
import { Badge } from '@design-system/components/primitives/Badge';
import { accessCopy } from '@copy';
import { approvalCopy } from '@copy/approvals';
import type { RiskLevel } from '../../types/approval';

interface RiskBadgeProps { level?: RiskLevel | null }

export function RiskBadge({ level }: RiskBadgeProps) {
  const explanationId = useId();
  const normalized = level?.trim().toLowerCase();
  const labels = { low: approvalCopy.lowRisk, medium: approvalCopy.mediumRisk, high: approvalCopy.highRisk, critical: approvalCopy.criticalRisk };
  const label = !normalized ? accessCopy.riskNotRated
    : normalized === 'low' || normalized === 'medium' || normalized === 'high' || normalized === 'critical'
      ? labels[normalized]
      : approvalCopy.otherRisk(level!.trim());
  const variant = normalized === 'low' ? 'success' : normalized === 'medium' ? 'warning' : normalized === 'critical' || normalized === 'high' ? 'danger' : 'neutral';
  return <>
    <Badge variant={variant} data-testid="approval-risk" aria-describedby={explanationId}>{label}</Badge>
    <span id={explanationId} className="sr-only">{normalized ? approvalCopy.riskExplanation : approvalCopy.unratedExplanation}</span>
  </>;
}
