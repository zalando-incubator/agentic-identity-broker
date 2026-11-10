/**
 * ErrorBoundary Component
 *
 * React Error Boundary for catching and handling component tree errors.
 * Follows the "Refined Trust Architecture" design system.
 *
 * Features:
 * - Catches render errors, lifecycle errors, and constructor errors
 * - Custom fallback UI with error details
 * - Retry/reset functionality
 * - Dev/prod mode support
 * - Error logging integration
 * - Nested boundary support
 * - WCAG 2.1 AA compliant with proper ARIA attributes
 */

import React, { Component, ErrorInfo, ReactNode } from 'react';
import { cn } from '@design-system/utils';
import { RefreshCw, TriangleAlert } from 'lucide-react';

export interface ErrorBoundaryProps {
  /** Child components to protect with error boundary */
  children: ReactNode;
  /** Custom fallback UI - can be static JSX or function receiving (error, retry) */
  fallback?: ReactNode | ((error: Error, retry: () => void) => ReactNode);
  /** Callback when error is caught */
  onError?: (error: Error, errorInfo: ErrorInfo) => void;
  /** Reset error state when props change (useful for route changes) */
  resetOnPropsChange?: boolean;
  /** Show detailed error information (dev mode) */
  showDetails?: boolean;
  /** Custom error boundary name for identification */
  boundaryName?: string;
}

interface ErrorBoundaryState {
  hasError: boolean;
  error: Error | null;
  errorInfo: ErrorInfo | null;
  errorCount: number;
}

const ErrorIcon = () => (
  <TriangleAlert className="w-6 h-6" aria-hidden="true" strokeWidth={1.5} />
);

const RefreshIcon = () => (
  <RefreshCw className="w-5 h-5" aria-hidden="true" strokeWidth={1.5} />
);

/**
 * ErrorBoundary component for catching React component errors.
 * Must be a class component as per React requirements.
 *
 * @example
 * ```tsx
 * <ErrorBoundary>
 *   <MyComponent />
 * </ErrorBoundary>
 *
 * <ErrorBoundary
 *   showDetails={true}
 *   onError={(error, errorInfo) => logErrorToService(error, errorInfo)}
 * >
 *   <RiskyComponent />
 * </ErrorBoundary>
 *
 * <ErrorBoundary
 *   fallback={(error, retry) => (
 *     <CustomErrorUI error={error} onRetry={retry} />
 *   )}
 * >
 *   <FeatureComponent />
 * </ErrorBoundary>
 * ```
 */
export class ErrorBoundary extends Component<
  ErrorBoundaryProps,
  ErrorBoundaryState
> {
  constructor(props: ErrorBoundaryProps) {
    super(props);
    this.state = {
      hasError: false,
      error: null,
      errorInfo: null,
      errorCount: 0,
    };
  }

  static getDerivedStateFromError(error: Error): Partial<ErrorBoundaryState> {
    // Update state so the next render will show the fallback UI
    return {
      hasError: true,
      error,
    };
  }

  componentDidCatch(error: Error, errorInfo: ErrorInfo): void {
    // Log error details
    const { onError, boundaryName } = this.props;
    const { errorCount } = this.state;

    // Update error count
    this.setState({
      errorInfo,
      errorCount: errorCount + 1,
    });

    // Log to console in development
    if (process.env.NODE_ENV === 'development') {
      console.error(
        `[ErrorBoundary${boundaryName ? ` "${boundaryName}"` : ''}] Caught error:`,
        error,
      );
      console.error('Error Info:', errorInfo);
    }

    // Call custom error handler
    if (onError) {
      try {
        onError(error, errorInfo);
      } catch (handlerError) {
        console.error('Error in onError handler:', handlerError);
      }
    }
  }

  componentDidUpdate(prevProps: ErrorBoundaryProps): void {
    const { hasError } = this.state;
    const { resetOnPropsChange, children } = this.props;

    // Reset error state if props change and resetOnPropsChange is enabled
    if (hasError && resetOnPropsChange && children !== prevProps.children) {
      this.resetErrorBoundary();
    }
  }

  resetErrorBoundary = (): void => {
    this.setState({
      hasError: false,
      error: null,
      errorInfo: null,
    });
  };

  renderDefaultFallback(): ReactNode {
    const { error, errorInfo, errorCount } = this.state;
    const { showDetails, boundaryName } = this.props;

    if (!error) return null;

    return (
      <div
        role="alert"
        aria-live="assertive"
        className={cn(
          'bg-error-light border border-error-primary/30 rounded-lg p-6',
          'text-error-dark',
        )}
      >
        {/* Header with icon and title */}
        <div className="flex items-start gap-3 mb-4">
          <div className="flex-shrink-0 text-error-primary mt-0.5">
            <ErrorIcon />
          </div>
          <div className="flex-1 min-w-0">
            <h3 className="text-lg font-medium text-error-dark mb-1">
              Something went wrong
            </h3>
            <p className="text-sm text-error-dark/80">
              {boundaryName
                ? `An error occurred in ${boundaryName}.`
                : 'An unexpected error has occurred.'}
            </p>
          </div>
        </div>

        {/* Error details (dev mode) */}
        {showDetails && (
          <div className="mb-4 space-y-3">
            <div className="bg-error-dark/5 border border-error-primary/20 rounded-md p-4">
              <h4 className="text-xs font-medium text-error-dark mb-2">
                Error Message
              </h4>
              <p className="text-sm font-mono text-error-dark break-words">
                {error.toString()}
              </p>
            </div>

            {errorInfo?.componentStack && (
              <details className="bg-error-dark/5 border border-error-primary/20 rounded-md">
                <summary className="cursor-pointer p-4 text-xs font-medium text-error-dark hover:bg-error-dark/5">
                  Component Stack (Click to expand)
                </summary>
                <pre className="p-4 pt-2 text-xs font-mono text-error-dark overflow-x-auto">
                  {errorInfo.componentStack}
                </pre>
              </details>
            )}

            {error.stack && (
              <details className="bg-error-dark/5 border border-error-primary/20 rounded-md">
                <summary className="cursor-pointer p-4 text-xs font-medium text-error-dark hover:bg-error-dark/5">
                  Error Stack (Click to expand)
                </summary>
                <pre className="p-4 pt-2 text-xs font-mono text-error-dark overflow-x-auto">
                  {error.stack}
                </pre>
              </details>
            )}

            {errorCount > 1 && (
              <div className="text-xs text-error-dark/70 bg-error-dark/5 rounded px-3 py-2">
                This error has occurred <strong>{errorCount}</strong> time
                {errorCount === 1 ? '' : 's'}.
              </div>
            )}
          </div>
        )}

        {/* Production error message */}
        {!showDetails && (
          <div className="mb-4 text-sm text-error-dark/80">
            <p>
              We apologize for the inconvenience. The error has been logged and
              our team will investigate.
            </p>
          </div>
        )}

        {/* Action buttons */}
        <div className="flex flex-wrap gap-3">
          {showDetails && (
            <button
              type="button"
              onClick={() => window.location.reload()}
              className={cn(
                'inline-flex items-center px-4 py-2 rounded-md',
                'bg-white text-error-dark font-medium text-sm',
                'border border-error-primary/30',
                'hover:bg-error-light transition-colors duration-200',
                'focus:outline-none focus:ring-2 focus:ring-error-primary focus:ring-offset-2',
              )}
            >
              Reload Page
            </button>
          )}

          <button
            type="button"
            onClick={this.resetErrorBoundary}
            className={cn(
              'inline-flex items-center gap-2 px-4 py-2 rounded-md',
              'bg-error-primary text-white font-medium text-sm',
              'hover:bg-error-dark transition-colors duration-200',
              'focus:outline-none focus:ring-2 focus:ring-error-primary focus:ring-offset-2',
              'shadow-sm hover:shadow-md',
            )}
          >
            <RefreshIcon />
            Try Again
          </button>
        </div>

        {/* Additional context for developers */}
        {showDetails && (
          <div className="mt-4 pt-4 border-t border-error-primary/20">
            <p className="text-xs text-error-dark/60">
              <strong>Note:</strong> Detailed error information is shown because{' '}
              <code className="bg-error-dark/10 px-1 py-0.5 rounded">
                showDetails
              </code>{' '}
              is enabled. Disable this in production environments.
            </p>
          </div>
        )}
      </div>
    );
  }

  render(): ReactNode {
    const { hasError, error } = this.state;
    const { children, fallback } = this.props;

    if (hasError && error) {
      // Render custom fallback if provided
      if (fallback) {
        if (typeof fallback === 'function') {
          return fallback(error, this.resetErrorBoundary);
        }
        return fallback;
      }

      // Render default fallback
      return this.renderDefaultFallback();
    }

    // Render children normally when no error
    return children;
  }
}

export default ErrorBoundary;
