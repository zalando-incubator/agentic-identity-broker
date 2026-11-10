/**
 * ApprovalErrorBanner - Error states for approval operations.
 *
 * Displays contextual error messages for:
 * - expired: Approval has timed out
 * - already_actioned: Approval already approved/denied
 * - forbidden: User doesn't own this approval
 * - not_found: Approval doesn't exist
 * - network_error: Connection issues
 * - server_error: Backend failure
 */

import {
  CircleCheck,
  Clock,
  Lock,
  PencilLine,
  RotateCw,
  SearchX,
  TriangleAlert,
  WifiOff,
  type LucideIcon,
} from 'lucide-react';
import type { ApprovalErrorCode } from '../../types/approval';
import { Button } from '@design-system/components/primitives/Button';

interface ApprovalErrorBannerProps {
  errorCode: ApprovalErrorCode;
  message?: string | null;
  onRetry?: () => void;
}

const ERROR_CONFIG: Record<
  ApprovalErrorCode,
  {
    title: string;
    description: string;
    retryable: boolean;
    icon: LucideIcon;
    tone: 'neutral' | 'error';
  }
> = {
  EXPIRED: {
    title: 'Approval Expired',
    description:
      'This approval request has timed out. The agent will need to submit a new request.',
    retryable: false,
    icon: Clock,
    tone: 'neutral',
  },
  ALREADY_ACTIONED: {
    title: 'Already Resolved',
    description:
      'This approval has already been approved or denied. No further action is needed.',
    retryable: false,
    icon: CircleCheck,
    tone: 'neutral',
  },
  FORBIDDEN: {
    title: 'Access Denied',
    description: 'You do not have permission to view or act on this approval.',
    retryable: false,
    icon: Lock,
    tone: 'error',
  },
  NOT_FOUND: {
    title: 'Not Found',
    description:
      'This approval request could not be found. It may have been removed.',
    retryable: false,
    icon: SearchX,
    tone: 'neutral',
  },
  NETWORK_ERROR: {
    title: 'Connection Error',
    description:
      'Unable to reach the server. Please check your connection and try again.',
    retryable: true,
    icon: WifiOff,
    tone: 'error',
  },
  SERVER_ERROR: {
    title: 'Server Error',
    description:
      'Something went wrong on our end. Please try again in a moment.',
    retryable: true,
    icon: TriangleAlert,
    tone: 'error',
  },
  INVALID_PATTERN: {
    title: 'Scope Not Accepted',
    description:
      'This approval scope does not cover the request being reviewed. Adjust the scope and try again.',
    retryable: false,
    icon: PencilLine,
    tone: 'error',
  },
};

export function ApprovalErrorBanner({
  errorCode,
  message,
  onRetry,
}: ApprovalErrorBannerProps) {
  const config = ERROR_CONFIG[errorCode];
  const Icon = config.icon;

  return (
    <div className="max-w-2xl mx-auto p-6" role="alert" aria-live="assertive">
      <div className="rounded-xl border border-line bg-white p-8 text-center">
        <div
          className={`mx-auto mb-5 flex h-10 w-10 items-center justify-center rounded-lg border ${
            config.tone === 'error'
              ? 'border-error-primary/20 bg-error-light text-error-primary'
              : 'border-line bg-neutral-100 text-neutral-600'
          }`}
          aria-hidden="true"
        >
          <Icon className="h-5 w-5" strokeWidth={1.5} />
        </div>
        <h2 className="text-lg font-medium text-trust-deep">{config.title}</h2>
        <p className="mt-2 text-sm text-neutral-600">
          {message || config.description}
        </p>
        {config.retryable && onRetry && (
          <Button
            variant="outline"
            size="sm"
            className="mt-6"
            onClick={onRetry}
            iconBefore={<RotateCw className="h-4 w-4" strokeWidth={1.5} />}
          >
            Try Again
          </Button>
        )}
      </div>
    </div>
  );
}

export default ApprovalErrorBanner;
