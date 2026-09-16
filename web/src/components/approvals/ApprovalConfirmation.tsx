/**
 * ApprovalConfirmation - Success/denial confirmation screen.
 *
 * Shown after the user has approved or denied a tool call.
 * Displays the outcome and allows the user to close the page.
 */

interface ApprovalConfirmationProps {
  type: 'approved' | 'denied';
  persistence?: 'once' | 'session' | 'permanent' | null;
  decidedAt?: string | null;
  toolName: string;
  agentName?: string;
}

export function ApprovalConfirmation({
  type,
  persistence,
  decidedAt,
  toolName,
  agentName,
}: ApprovalConfirmationProps) {
  const isApproved = type === 'approved';

  const outcome = isApproved ? 'Approved' : 'Denied';

  if (isApproved) {
    return (
      <div className="max-w-2xl mx-auto p-6">
        <div className="rounded-xl border border-success-primary/30 bg-success-light p-8 text-center space-y-4">
          <p className="text-xl font-semibold text-success-dark">Decision recorded.</p>
          <p className="text-sm text-neutral-700">The next tool invocation will be allowed.</p>
          <p className="text-sm text-neutral-700">You can return to your agent now.</p>
        </div>
      </div>
    );
  }

  return (
    <div className="max-w-2xl mx-auto p-6">
      <div
        className={`rounded-xl border p-8 text-center space-y-4 ${
          isApproved
            ? 'border-success-primary/30 bg-success-light'
            : 'border-error-primary/30 bg-error-light'
        }`}
      >
        <div className="text-5xl" aria-hidden="true">
          {isApproved ? '✓' : '✕'}
        </div>
        <p className="text-xs font-semibold uppercase tracking-wide text-neutral-600">
          Decision recorded
        </p>
        <h2
          className={`text-xl font-semibold ${
            isApproved ? 'text-success-dark' : 'text-error-primary'
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
          <p className="text-xs text-neutral-500">
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
