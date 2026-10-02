import type { ReactNode } from 'react';
import { ErrorBoundary, type ErrorBoundaryProps } from '../ErrorBoundary/ErrorBoundary';
import { Alert } from '../Alert/Alert';
import { Button } from '@design-system/components/primitives/Button';

export interface GlobalErrorBoundaryProps extends ErrorBoundaryProps {
  wordmark?: ReactNode;
  actions?: ReactNode;
}

export function GlobalErrorBoundary({ fallback, wordmark, actions, title, description, retryLabel, ...props }: GlobalErrorBoundaryProps) {
  return <ErrorBoundary {...props} title={title} description={description} retryLabel={retryLabel} fallback={(error, reset) => (
    <main data-testid="global-error-boundary" className="flex min-h-dvh items-center justify-center bg-background p-6 text-foreground">
      <div className="w-full max-w-lg space-y-6">
        {wordmark}
        {fallback !== undefined ? (typeof fallback === 'function' ? fallback(error, reset) : fallback) : (
          <Alert variant="error" title={title}>
            {description && <p>{description}</p>}
            <div className="mt-4 flex flex-wrap gap-3"><Button variant="outline" onClick={reset}>{retryLabel}</Button>{actions}</div>
          </Alert>
        )}
      </div>
    </main>
  )} />;
}

export default GlobalErrorBoundary;
