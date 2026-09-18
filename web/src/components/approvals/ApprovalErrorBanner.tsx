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

import type { ApprovalErrorCode } from '../../types/approval';

interface ApprovalErrorBannerProps {
  errorCode: ApprovalErrorCode;
  message?: string | null;
  onRetry?: () => void;
}

const ERROR_CONFIG: Record<
  ApprovalErrorCode,
  { title: string; description: string; retryable: boolean; icon: string }
> = {
  EXPIRED: {
    title: 'Approval Expired',
    description:
      'This approval request has timed out. The agent will need to submit a new request.',
    retryable: false,
    icon: '⏱',
  },
  ALREADY_ACTIONED: {
    title: 'Already Resolved',
    description:
      'This approval has already been approved or denied. No further action is needed.',
    retryable: false,
    icon: '✓',
  },
  FORBIDDEN: {
    title: 'Access Denied',
    description: 'You do not have permission to view or act on this approval.',
    retryable: false,
    icon: '🔒',
  },
  NOT_FOUND: {
    title: 'Not Found',
    description:
      'This approval request could not be found. It may have been removed.',
    retryable: false,
    icon: '🔍',
  },
  NETWORK_ERROR: {
    title: 'Connection Error',
    description:
      'Unable to reach the server. Please check your connection and try again.',
    retryable: true,
    icon: '📡',
  },
  SERVER_ERROR: {
    title: 'Server Error',
    description:
      'Something went wrong on our end. Please try again in a moment.',
    retryable: true,
    icon: '⚠',
  },
  INVALID_PATTERN: {
    title: 'Scope Not Accepted',
    description:
      'This approval scope does not cover the request being reviewed. Adjust the scope and try again.',
    retryable: false,
    icon: '✎',
  },
};

export function ApprovalErrorBanner({
  errorCode,
  message,
  onRetry,
}: ApprovalErrorBannerProps) {
  const config = ERROR_CONFIG[errorCode];

  return (
    <div
      className="max-w-2xl mx-auto p-6"
      role="alert"
      aria-live="assertive"
    >
      <div className="rounded-xl border border-error-primary/30 bg-error-light p-6 text-center space-y-4">
        <div className="text-4xl" aria-hidden="true">
          {config.icon}
        </div>
        <h2 className="text-lg font-semibold text-error-dark">{config.title}</h2>
        <p className="text-sm text-error-primary">
          {message || config.description}
        </p>
        {config.retryable && onRetry && (
          <button
            onClick={onRetry}
            className="mt-4 inline-flex items-center px-4 py-2 rounded-lg bg-error-primary text-white text-sm font-medium hover:bg-error-hover focus:outline-none focus:ring-2 focus:ring-error-primary focus:ring-offset-2 transition-colors"
          >
            Try Again
          </button>
        )}
      </div>
    </div>
  );
}

export default ApprovalErrorBanner;
