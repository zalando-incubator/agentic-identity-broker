/**
 * InlineError component displays error messages with retry functionality.
 *
 * Features:
 * - Red alert styling
 * - Error icon
 * - Retry button
 * - Dismissible with close button
 */

import React, { useState } from 'react';
import { CircleAlert, X } from 'lucide-react';
import { Button } from '@design-system/components/primitives/Button';

interface InlineErrorProps {
  /** Error message to display */
  error: string;
  /** Callback function for retry action */
  onRetry: () => void;
  /** Label for the retry button. Defaults to "Try again". */
  retryLabel?: string;
}

/**
 * InlineError displays an error message with an option to retry the failed action.
 * Can be dismissed by the user.
 */
export function InlineError({
  error,
  onRetry,
  retryLabel = 'Try again',
}: InlineErrorProps) {
  const [isDismissed, setIsDismissed] = useState(false);

  if (isDismissed) {
    return null;
  }

  return (
    <div
      className="bg-error-light border border-error-primary/30 rounded-lg p-4"
      role="alert"
      aria-live="assertive"
    >
      <div className="flex items-start gap-3">
        {/* Error icon */}
        <CircleAlert
          className="w-5 h-5 text-error-primary flex-shrink-0 mt-0.5"
          aria-hidden="true"
          strokeWidth={1.5}
        />

        {/* Error content */}
        <div className="flex-1 min-w-0">
          <h3 className="text-sm font-medium text-error-dark">Error</h3>
          <p className="mt-1 text-sm text-error-dark">{error}</p>

          {/* Actions */}
          <div className="mt-3 flex items-center gap-3">
            <Button variant="outline" size="sm" onClick={onRetry}>
              {retryLabel}
            </Button>
          </div>
        </div>

        {/* Dismiss button */}
        <button
          type="button"
          onClick={() => setIsDismissed(true)}
          className="flex-shrink-0 text-error-primary hover:text-error-primary focus:outline-none focus:ring-2 focus:ring-error-primary focus:ring-offset-2 rounded"
          aria-label="Dismiss error"
        >
          <X className="w-5 h-5" aria-hidden="true" strokeWidth={1.5} />
        </button>
      </div>
    </div>
  );
}

export default InlineError;
