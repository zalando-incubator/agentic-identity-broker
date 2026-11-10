/**
 * ApprovalConfirmation - Success/denial confirmation screen.
 *
 * Shown after the user has approved or denied a tool call.
 * Displays the outcome and allows the user to close the page.
 */

import { Check, X } from 'lucide-react';

interface ApprovalConfirmationProps {
  type: 'approved' | 'denied';
  persistence?: 'once' | 'session' | 'permanent' | null;
  decidedAt?: string | null;
  toolName: string;
  agentName?: string;
  historical?: boolean;
}

export function ApprovalConfirmation({
  type,
  persistence,
  decidedAt,
  toolName,
  agentName,
  historical = false,
}: ApprovalConfirmationProps) {
  const isApproved = type === 'approved';

  const outcome = isApproved ? 'Approved' : 'Denied';

  if (isApproved && historical) {
    return (
      <div className="max-w-2xl mx-auto p-6">
        <div className="rounded-xl border border-line bg-white p-8 text-center space-y-4">
          <div
            className="mx-auto flex h-10 w-10 items-center justify-center rounded-lg border border-success-primary/20 bg-success-light text-success-primary"
            aria-hidden="true"
          >
            <Check className="h-5 w-5" strokeWidth={1.5} />
          </div>
          <p className="text-xl font-medium text-trust-deep">
            Decision recorded.
          </p>
          <p className="text-sm text-neutral-700">
            The next tool invocation will be allowed.
          </p>
          <p className="text-sm text-neutral-700">
            You can return to your agent now.
          </p>
        </div>
      </div>
    );
  }

  return (
    <div className="max-w-2xl mx-auto p-6">
      <div className="rounded-xl border border-line bg-white p-8 text-center space-y-4">
        <div
          className={`mx-auto flex h-10 w-10 items-center justify-center rounded-lg border ${
            isApproved
              ? 'border-success-primary/20 bg-success-light text-success-primary'
              : 'border-error-primary/20 bg-error-light text-error-primary'
          }`}
          aria-hidden="true"
        >
          {isApproved ? (
            <Check className="h-5 w-5" strokeWidth={1.5} />
          ) : (
            <X className="h-5 w-5" strokeWidth={1.5} />
          )}
        </div>
        <p className="text-xs font-medium text-neutral-600">
          Decision recorded
        </p>
        <h2
          className={`text-xl font-medium ${
            isApproved ? 'text-trust-deep' : 'text-error-dark'
          }`}
        >
          {outcome}
        </h2>
        <p className="text-sm text-neutral-700">
          <strong>{toolName}</strong>
          {agentName && (
            <>
              {' '}
              for <strong>{agentName}</strong>
            </>
          )}
          {persistence && (
            <>
              {' '}
              · <strong>{persistence}</strong>
            </>
          )}
        </p>
        {decidedAt && (
          <p className="text-xs text-neutral-500" data-screenshot-dynamic>
            {outcome} at {new Date(decidedAt).toLocaleString()}
          </p>
        )}
        <p className="text-sm text-neutral-700 pt-4">
          This request is complete and is shown read-only.
        </p>
        <p className="text-sm text-neutral-700">
          You can safely close this page.
        </p>
        {persistence === 'permanent' && (
          <p className="text-xs text-neutral-600">
            Manage this permanent decision from Tool Authorizations.
          </p>
        )}
      </div>
    </div>
  );
}

export default ApprovalConfirmation;
