/**
 * GlobalErrorBoundary - Enhanced error boundary for the entire application.
 *
 * Features:
 * - Catches all React rendering errors
 * - Shows user-friendly error page with fallback UI
 * - Displays error details in development mode only
 * - Provides "Reload page" and "Go home" actions
 * - Logs errors to console (ready for external error tracking)
 * - Error icon/illustration
 *
 * Usage:
 * ```tsx
 * import { GlobalErrorBoundary } from '@components/ui/GlobalErrorBoundary';
 *
 * function App() {
 *   return (
 *     <GlobalErrorBoundary>
 *       <YourApp />
 *     </GlobalErrorBoundary>
 *   );
 * }
 * ```
 */

import React, { Component, ErrorInfo, ReactNode } from 'react';
import { House, Info, RefreshCw, TriangleAlert } from 'lucide-react';
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
 * Global error boundary that catches all React errors.
 * Displays a comprehensive fallback UI with actionable options.
 */
export class GlobalErrorBoundary extends Component<Props, State> {
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
    console.error('GlobalErrorBoundary caught an error:', error, errorInfo);

    // Update state with error details
    this.setState({
      error,
      errorInfo,
    });

    // TODO: Send error to external tracking service
    // Example: Sentry, LogRocket, etc.
    // if (import.meta.env.PROD) {
    //   logErrorToService(error, errorInfo);
    // }
  }

  handleReset = (): void => {
    this.setState({
      hasError: false,
      error: null,
      errorInfo: null,
    });
  };

  handleReload = (): void => {
    window.location.reload();
  };

  handleGoHome = (): void => {
    window.location.href = '/';
  };

  render(): ReactNode {
    if (this.state.hasError) {
      // Custom fallback if provided
      if (this.props.fallback) {
        return this.props.fallback;
      }

      // Check if running in development mode
      const isDevelopment = import.meta.env.DEV;

      // Default comprehensive fallback UI
      return (
        <div className="min-h-screen flex items-center justify-center bg-neutral-50 px-4 py-8">
          <div className="max-w-2xl w-full bg-white rounded-2xl shadow-2xl overflow-hidden border border-neutral-200">
            {/* Header Section with Icon */}
            <div className="bg-error-primary px-8 py-10 text-center">
              <div className="inline-flex items-center justify-center w-20 h-20 bg-white rounded-full mb-4">
                <TriangleAlert
                  className="w-12 h-12 text-error-primary"
                  aria-hidden="true"
                  strokeWidth={1.5}
                />
              </div>
              <h1 className="text-3xl font-medium text-white mb-2">
                Oops! Something went wrong
              </h1>
              <p className="text-error-light text-lg">
                We encountered an unexpected error
              </p>
            </div>

            {/* Content Section */}
            <div className="px-8 py-8">
              <p className="text-neutral-700 text-lg mb-6 leading-relaxed">
                We&apos;re sorry for the inconvenience. An unexpected error
                occurred while rendering this page. Our team has been notified
                and we&apos;ll look into it.
              </p>

              {/* Error Details (Development Only) */}
              {isDevelopment && this.state.error && (
                <details className="mb-6 p-4 bg-neutral-50 rounded-lg border border-neutral-200">
                  <summary className="cursor-pointer text-sm font-medium text-neutral-700 hover:text-neutral-900 mb-3 flex items-center gap-2">
                    <Info
                      className="w-5 h-5"
                      aria-hidden="true"
                      strokeWidth={1.5}
                    />
                    Error Details (Development Mode)
                  </summary>
                  <div className="space-y-3">
                    <div>
                      <p className="text-xs font-medium text-neutral-600 mb-1">
                        Error Message:
                      </p>
                      <pre className="text-xs font-mono text-error-dark whitespace-pre-wrap break-words bg-error-light p-3 rounded border border-error-primary/30">
                        {this.state.error.toString()}
                      </pre>
                    </div>
                    {this.state.errorInfo &&
                      this.state.errorInfo.componentStack && (
                        <div>
                          <p className="text-xs font-medium text-neutral-600 mb-1">
                            Component Stack:
                          </p>
                          <pre className="text-xs font-mono text-neutral-700 whitespace-pre-wrap break-words bg-neutral-100 p-3 rounded border border-neutral-300 max-h-64 overflow-y-auto">
                            {this.state.errorInfo.componentStack}
                          </pre>
                        </div>
                      )}
                  </div>
                </details>
              )}

              {/* Production Error Hint */}
              {!isDevelopment && (
                <div className="mb-6 p-4 bg-info-light rounded-lg border border-info-primary/30">
                  <div className="flex gap-3">
                    <Info
                      className="w-5 h-5 text-info-primary flex-shrink-0 mt-0.5"
                      aria-hidden="true"
                      strokeWidth={1.5}
                    />
                    <div>
                      <p className="text-sm font-medium text-info-dark mb-1">
                        What can you do?
                      </p>
                      <ul className="text-sm text-info-dark space-y-1 list-disc list-inside">
                        <li>Try reloading the page</li>
                        <li>Return to the home page and try again</li>
                        <li>Clear your browser cache</li>
                        <li>Contact support if the issue persists</li>
                      </ul>
                    </div>
                  </div>
                </div>
              )}

              {/* Action Buttons */}
              <div className="flex flex-col-reverse sm:flex-row gap-3">
                <Button
                  variant="outline"
                  className="flex-1"
                  onClick={this.handleGoHome}
                  iconBefore={<House className="h-4 w-4" strokeWidth={1.5} />}
                >
                  Go Home
                </Button>
                <Button
                  className="flex-1"
                  onClick={this.handleReload}
                  iconBefore={
                    <RefreshCw className="h-4 w-4" strokeWidth={1.5} />
                  }
                >
                  Reload Page
                </Button>
              </div>

              {/* Additional Help Text */}
              <div className="mt-6 pt-6 border-t border-neutral-200">
                <p className="text-sm text-neutral-500 text-center">
                  If this problem persists, please contact our support team
                </p>
              </div>
            </div>
          </div>
        </div>
      );
    }

    return this.props.children;
  }
}

export default GlobalErrorBoundary;
