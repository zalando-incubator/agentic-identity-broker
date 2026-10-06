import { Button } from '@design-system/components/primitives/Button';
import { commonCopy } from '@copy';
import { approvalCopy } from '@copy/approvals';
import type { ApprovalErrorCode } from '../../types/approval';

interface ApprovalErrorBannerProps {
  errorCode: ApprovalErrorCode;
  message?: string | null;
  onRetry?: () => void;
  outcome?: boolean;
}

export function ApprovalErrorBanner({ errorCode, message, onRetry, outcome = true }: ApprovalErrorBannerProps) {
  const error = approvalCopy.errors[errorCode];
  return <div role="alert" data-testid={outcome ? 'approval-outcome' : 'approval-error'} className="space-y-3 rounded-lg border border-border bg-card p-4 text-card-foreground">
    <h2 className="font-display text-lg font-semibold">{error.title}</h2>
    <p className="break-words text-sm text-muted-foreground">{message || error.description}</p>
    {error.retryable && onRetry && <Button variant="secondary" onClick={onRetry}>{commonCopy.retry}</Button>}
  </div>;
}
