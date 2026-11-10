/**
 * ErrorBoundary component to catch React errors during rendering.
 *
 * Provides a fallback UI when an error occurs in any child component,
 * preventing the entire app from crashing.
 */

import React, { Component, ErrorInfo, ReactNode } from 'react';
import { TriangleAlert } from 'lucide-react';
import { Button } from '@design-system/components/primitives/Button';

interface Props {
  children: ReactNode;
  fallback?: ReactNode;
}

interface State {
  hasError: boolean;
  error: Error | null;
  errorInfo: ErrorInfo | null;
}

/**
 * Error boundary that catches errors in child components.
 * Displays a fallback UI and provides a retry mechanism.
 */
export class ErrorBoundary extends Component<Props, State> {
  constructor(props: Props) {
    super(props);
    this.state = {
      hasError: false,
      error: null,
      errorInfo: null,
    };
  }

  static getDerivedStateFromError(error: Error): Partial<State> {
    // Update state to trigger fallback UI
    return {
      hasError: true,
      error,
    };
  }

  componentDidCatch(error: Error, errorInfo: ErrorInfo): void {
    // Log error details to console
    console.error('ErrorBoundary caught an error:', error, errorInfo);

    // Update state with error details
    this.setState({
      error,
      errorInfo,
    });

    // TODO: Log to error tracking service (e.g., Sentry)
    // logErrorToService(error, errorInfo);
  }

  handleReset = (): void => {
    this.setState({
      hasError: false,
      error: null,
      errorInfo: null,
    });
  };

  render(): ReactNode {
    if (this.state.hasError) {
      // Custom fallback if provided
      if (this.props.fallback) {
        return this.props.fallback;
      }

      // Default fallback UI
      return (
        <div className="min-h-screen flex items-center justify-center bg-neutral-50 px-4">
          <div className="max-w-md w-full bg-white rounded-xl border border-error-light p-6">
            <div className="flex items-center gap-3 mb-4">
              <TriangleAlert
                className="w-8 h-8 text-error-primary"
                aria-hidden="true"
                strokeWidth={1.5}
              />
              <h2 className="text-xl font-medium text-neutral-900">
                Something went wrong
              </h2>
            </div>

            <p className="text-neutral-600 mb-4">
              We encountered an unexpected error while rendering this page. This
              has been logged and we&apos;ll look into it.
            </p>

            {this.state.error && (
              <details className="mb-4 p-3 bg-neutral-50 rounded border border-neutral-200">
                <summary className="cursor-pointer text-sm font-medium text-neutral-700 mb-2">
                  Error Details
                </summary>
                <div className="text-xs font-mono text-error-primary whitespace-pre-wrap break-words">
                  {this.state.error.toString()}
                  {this.state.errorInfo && (
                    <div className="mt-2 text-neutral-600">
                      {this.state.errorInfo.componentStack}
                    </div>
                  )}
                </div>
              </details>
            )}

            <div className="flex gap-3">
              <Button
                variant="outline"
                className="flex-1"
                onClick={() => window.location.reload()}
              >
                Reload Page
              </Button>
              <Button className="flex-1" onClick={this.handleReset}>
                Try Again
              </Button>
            </div>
          </div>
        </div>
      );
    }

    return this.props.children;
  }
}

export default ErrorBoundary;
