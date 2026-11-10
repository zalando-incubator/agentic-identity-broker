/**
 * GlobalErrorBoundary Component
 *
 * React Error Boundary for global/application-level error handling.
 * Follows the "Refined Trust Architecture" design system.
 *
 * Features:
 * - Full-page error display for critical application errors
 * - Multiple recovery strategies (reload, navigate, contact support)
 * - Error reporting/logging integration
 * - Support contact information display
 * - Graceful degradation with limited functionality mode
 * - Error categorization (network, permission, generic)
 * - WCAG 2.1 AA compliant with proper ARIA attributes
 * - Production-ready with comprehensive error handling
 */

import React, { Component, ErrorInfo, ReactNode } from 'react';
import { cn } from '@design-system/utils';
import {
  FileText,
  House,
  Mail,
  Phone,
  RefreshCw,
  TriangleAlert,
} from 'lucide-react';

export interface GlobalErrorBoundaryProps {
  /** Child components to protect with error boundary */
  children: ReactNode;
  /** Application name to display in error messages */
  appName?: string;
  /** Support email for users to contact */
  supportEmail?: string;
  /** Callback when error is caught - use for error reporting services */
  onError?: (error: Error, errorInfo: ErrorInfo) => void;
  /** Show detailed error information (dev mode) */
  showDetails?: boolean;
  /** Recovery strategies available to user */
  recoveryStrategies?: ('reload' | 'navigate' | 'contact-support')[];
  /** Allow graceful degradation instead of full error page */
  allowGracefulDegradation?: boolean;
  /** Fallback content for graceful degradation mode */
  gracefulFallback?: ReactNode;
  /** Custom support phone number */
  supportPhone?: string;
  /** Custom support URL/documentation */
  supportUrl?: string;
}

interface GlobalErrorBoundaryState {
  hasError: boolean;
  error: Error | null;
  errorInfo: ErrorInfo | null;
  errorCategory: 'network' | 'permission' | 'generic';
  errorCount: number;
  isGracefulMode: boolean;
}

// Large error icon for full-page display
const LargeErrorIcon = () => (
  <TriangleAlert className="w-24 h-24" aria-hidden="true" strokeWidth={1.5} />
);

const RefreshIcon = () => (
  <RefreshCw className="w-5 h-5" aria-hidden="true" strokeWidth={1.5} />
);

const HomeIcon = () => (
  <House className="w-5 h-5" aria-hidden="true" strokeWidth={1.5} />
);

const EmailIcon = () => (
  <Mail className="w-5 h-5" aria-hidden="true" strokeWidth={1.5} />
);

const PhoneIcon = () => (
  <Phone className="w-5 h-5" aria-hidden="true" strokeWidth={1.5} />
);

const DocumentIcon = () => (
  <FileText className="w-5 h-5" aria-hidden="true" strokeWidth={1.5} />
);

/**
 * Categorize error based on error message and properties
 */
function categorizeError(error: Error): 'network' | 'permission' | 'generic' {
  const message = error.message.toLowerCase();

  if (
    message.includes('network') ||
    message.includes('fetch') ||
    message.includes('timeout') ||
    message.includes('connection') ||
    message.includes('offline')
  ) {
    return 'network';
  }

  if (
    message.includes('permission') ||
    message.includes('unauthorized') ||
    message.includes('forbidden') ||
    message.includes('access denied')
  ) {
    return 'permission';
  }

  return 'generic';
}

/**
 * GlobalErrorBoundary component for catching critical application errors.
 * Must be a class component as per React requirements.
 *
 * @example
 * ```tsx
 * // Basic usage - wrap entire app
 * <GlobalErrorBoundary appName="Agentic Identity Broker">
 *   <App />
 * </GlobalErrorBoundary>
 *
 * // With support information
 * <GlobalErrorBoundary
 *   appName="Agentic Identity Broker"
 *   supportEmail="support@example.com"
 *   supportPhone="1-800-SUPPORT"
 *   onError={(error, errorInfo) => logToSentry(error, errorInfo)}
 * >
 *   <App />
 * </GlobalErrorBoundary>
 *
 * // With graceful degradation
 * <GlobalErrorBoundary
 *   allowGracefulDegradation
 *   gracefulFallback={<LimitedFunctionalityApp />}
 * >
 *   <FullFeaturedApp />
 * </GlobalErrorBoundary>
 * ```
 */
export class GlobalErrorBoundary extends Component<
  GlobalErrorBoundaryProps,
  GlobalErrorBoundaryState
> {
  constructor(props: GlobalErrorBoundaryProps) {
    super(props);
    this.state = {
      hasError: false,
      error: null,
      errorInfo: null,
      errorCategory: 'generic',
      errorCount: 0,
      isGracefulMode: false,
    };
  }

  static getDerivedStateFromError(
    error: Error,
  ): Partial<GlobalErrorBoundaryState> {
    return {
      hasError: true,
      error,
      errorCategory: categorizeError(error),
    };
  }

  componentDidCatch(error: Error, errorInfo: ErrorInfo): void {
    const { onError } = this.props;
    const { errorCount } = this.state;

    // Update state with error details
    this.setState({
      errorInfo,
      errorCount: errorCount + 1,
    });

    // Log to console in development
    if (process.env.NODE_ENV === 'development') {
      console.error(
        '[GlobalErrorBoundary] Caught critical application error:',
        error,
      );
      console.error('Error Info:', errorInfo);
    }

    // Call custom error handler (e.g., Sentry, LogRocket)
    if (onError) {
      try {
        onError(error, errorInfo);
      } catch (handlerError) {
        console.error(
          'Error in GlobalErrorBoundary onError handler:',
          handlerError,
        );
      }
    }
  }

  handleReload = (): void => {
    window.location.reload();
  };

  handleNavigateHome = (): void => {
    window.location.href = '/';
  };

  handleGracefulMode = (): void => {
    this.setState({ isGracefulMode: true });
  };

  getErrorTitle(): string {
    const { errorCategory } = this.state;
    const { appName = 'Application' } = this.props;

    switch (errorCategory) {
      case 'network':
        return 'Connection Problem';
      case 'permission':
        return 'Access Denied';
      default:
        return `${appName} Encountered an Error`;
    }
  }

  getErrorDescription(): string {
    const { errorCategory } = this.state;

    switch (errorCategory) {
      case 'network':
        return "We're having trouble connecting to our servers. This might be due to your internet connection or a temporary service outage.";
      case 'permission':
        return "You don't have permission to access this resource. Please contact your administrator if you believe this is an error.";
      default:
        return 'An unexpected error has occurred. We apologize for the inconvenience. Our team has been notified and is working to resolve the issue.';
    }
  }

  renderFooter(): ReactNode {
    const {
      supportEmail,
      supportPhone,
      supportUrl,
      appName = 'Application',
    } = this.props;

    if (!supportEmail && !supportPhone && !supportUrl) {
      return null;
    }

    return (
      <footer className="mt-12 pt-8 border-t border-error-primary/20">
        <div className="max-w-xl mx-auto">
          <h3 className="text-sm font-medium text-error-dark/80 mb-4 text-center">
            Need Help?
          </h3>
          <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 gap-4">
            {supportEmail && (
              <a
                href={`mailto:${supportEmail}`}
                className={cn(
                  'flex items-center gap-3 p-4 rounded-lg',
                  'bg-white border border-error-primary/20',
                  'hover:border-error-primary/40 hover:bg-error-light/30',
                  'transition-colors duration-200',
                  'focus:outline-none focus:ring-2 focus:ring-error-primary focus:ring-offset-2',
                )}
              >
                <EmailIcon />
                <div className="flex-1 min-w-0 text-left">
                  <div className="text-xs text-error-dark/60 font-medium">
                    Email Support
                  </div>
                  <div className="text-sm text-error-dark font-medium truncate">
                    {supportEmail}
                  </div>
                </div>
              </a>
            )}

            {supportPhone && (
              <a
                href={`tel:${supportPhone}`}
                className={cn(
                  'flex items-center gap-3 p-4 rounded-lg',
                  'bg-white border border-error-primary/20',
                  'hover:border-error-primary/40 hover:bg-error-light/30',
                  'transition-colors duration-200',
                  'focus:outline-none focus:ring-2 focus:ring-error-primary focus:ring-offset-2',
                )}
              >
                <PhoneIcon />
                <div className="flex-1 min-w-0 text-left">
                  <div className="text-xs text-error-dark/60 font-medium">
                    Call Support
                  </div>
                  <div className="text-sm text-error-dark font-medium truncate">
                    {supportPhone}
                  </div>
                </div>
              </a>
            )}

            {supportUrl && (
              <a
                href={supportUrl}
                target="_blank"
                rel="noopener noreferrer"
                className={cn(
                  'flex items-center gap-3 p-4 rounded-lg',
                  'bg-white border border-error-primary/20',
                  'hover:border-error-primary/40 hover:bg-error-light/30',
                  'transition-colors duration-200',
                  'focus:outline-none focus:ring-2 focus:ring-error-primary focus:ring-offset-2',
                )}
              >
                <DocumentIcon />
                <div className="flex-1 min-w-0 text-left">
                  <div className="text-xs text-error-dark/60 font-medium">
                    Documentation
                  </div>
                  <div className="text-sm text-error-dark font-medium truncate">
                    Help Center
                  </div>
                </div>
              </a>
            )}
          </div>

          <p className="mt-6 text-xs text-error-dark/50 text-center">
            {appName} Support Team - We&apos;re here to help 24/7
          </p>
        </div>
      </footer>
    );
  }

  renderFullPageError(): ReactNode {
    const { error, errorInfo, errorCount, errorCategory } = this.state;
    const {
      showDetails,
      appName = 'Application',
      recoveryStrategies = ['reload', 'navigate', 'contact-support'],
      allowGracefulDegradation,
    } = this.props;

    if (!error) return null;

    const showReload = recoveryStrategies.includes('reload');
    const showNavigate = recoveryStrategies.includes('navigate');
    const showGraceful =
      allowGracefulDegradation &&
      recoveryStrategies.includes('contact-support');

    return (
      <div
        role="alert"
        aria-live="assertive"
        className="min-h-screen w-full bg-neutral-50 flex items-center justify-center p-6"
      >
        <div className="max-w-2xl w-full">
          {/* Main error card */}
          <div className="bg-white border-2 border-error-primary/30 rounded-2xl shadow-float p-8 sm:p-12">
            {/* Error icon and header */}
            <div className="flex flex-col items-center text-center mb-8">
              <div className="text-error-primary mb-6">
                <LargeErrorIcon />
              </div>
              <h1 className="text-3xl sm:text-4xl font-medium text-error-dark mb-3">
                {this.getErrorTitle()}
              </h1>
              <p className="text-lg text-error-dark/80 leading-relaxed">
                {this.getErrorDescription()}
              </p>
            </div>

            {/* Error details (dev mode) */}
            {showDetails && (
              <div className="mb-8 space-y-4">
                <div className="bg-error-dark/5 border border-error-primary/20 rounded-lg p-4">
                  <h3 className="text-xs font-medium text-error-dark mb-2 flex items-center gap-2">
                    <span className="inline-block w-2 h-2 bg-error-primary rounded-full"></span>
                    Error Details
                  </h3>
                  <p className="text-sm font-mono text-error-dark break-words">
                    {error.toString()}
                  </p>
                  {errorCategory && (
                    <div className="mt-2 inline-flex items-center gap-2 px-2 py-1 bg-error-primary/10 rounded text-xs font-medium text-error-dark">
                      Category: {errorCategory}
                    </div>
                  )}
                </div>

                {errorInfo?.componentStack && (
                  <details className="bg-error-dark/5 border border-error-primary/20 rounded-lg">
                    <summary className="cursor-pointer p-4 text-xs font-medium text-error-dark hover:bg-error-dark/10 transition-colors">
                      Component Stack (Click to expand)
                    </summary>
                    <pre className="p-4 pt-2 text-xs font-mono text-error-dark overflow-x-auto max-h-64 overflow-y-auto">
                      {errorInfo.componentStack}
                    </pre>
                  </details>
                )}

                {error.stack && (
                  <details className="bg-error-dark/5 border border-error-primary/20 rounded-lg">
                    <summary className="cursor-pointer p-4 text-xs font-medium text-error-dark hover:bg-error-dark/10 transition-colors">
                      Error Stack (Click to expand)
                    </summary>
                    <pre className="p-4 pt-2 text-xs font-mono text-error-dark overflow-x-auto max-h-64 overflow-y-auto">
                      {error.stack}
                    </pre>
                  </details>
                )}

                {errorCount > 1 && (
                  <div className="text-xs text-error-dark/70 bg-error-dark/5 rounded-lg px-4 py-3 flex items-center gap-2">
                    <span className="inline-block w-2 h-2 bg-warning-primary rounded-full animate-pulse"></span>
                    This error has occurred <strong>{errorCount}</strong> time
                    {errorCount === 1 ? '' : 's'} during this session.
                  </div>
                )}
              </div>
            )}

            {/* Action buttons */}
            <div className="flex flex-col sm:flex-row gap-3 justify-center">
              {showReload && (
                <button
                  type="button"
                  onClick={this.handleReload}
                  className={cn(
                    'inline-flex items-center justify-center gap-2 px-6 py-3 rounded-lg',
                    'bg-error-primary text-white font-medium text-base',
                    'hover:bg-error-dark transition-colors duration-150',
                    'focus:outline-none focus:ring-2 focus:ring-error-primary focus:ring-offset-2',
                    '',
                  )}
                >
                  <RefreshIcon />
                  Reload {appName}
                </button>
              )}

              {showNavigate && (
                <button
                  type="button"
                  onClick={this.handleNavigateHome}
                  className={cn(
                    'inline-flex items-center justify-center gap-2 px-6 py-3 rounded-lg',
                    'bg-white text-error-dark font-medium text-base',
                    'border-2 border-error-primary/30',
                    'hover:bg-error-light hover:border-error-primary/50 transition-colors duration-150',
                    'focus:outline-none focus:ring-2 focus:ring-error-primary focus:ring-offset-2',
                    '',
                  )}
                >
                  <HomeIcon />
                  Go to Home
                </button>
              )}

              {showGraceful && (
                <button
                  type="button"
                  onClick={this.handleGracefulMode}
                  className={cn(
                    'inline-flex items-center justify-center gap-2 px-6 py-3 rounded-lg',
                    'bg-white text-error-dark font-medium text-base',
                    'border border-error-primary/20',
                    'hover:bg-error-light/30 hover:border-error-primary/30 transition-colors duration-150',
                    'focus:outline-none focus:ring-2 focus:ring-error-primary focus:ring-offset-2',
                  )}
                >
                  Continue with Limited Features
                </button>
              )}
            </div>

            {/* Developer note */}
            {showDetails && (
              <div className="mt-8 pt-6 border-t border-error-primary/20">
                <p className="text-xs text-error-dark/50 text-center">
                  <strong>Developer Mode:</strong> Detailed error information is
                  shown because{' '}
                  <code className="bg-error-dark/10 px-1.5 py-0.5 rounded font-mono">
                    showDetails
                  </code>{' '}
                  is enabled. Disable in production.
                </p>
              </div>
            )}
          </div>

          {/* Footer with support information */}
          {this.renderFooter()}
        </div>
      </div>
    );
  }

  render(): ReactNode {
    const { hasError, isGracefulMode } = this.state;
    const { children, allowGracefulDegradation, gracefulFallback } = this.props;

    if (hasError) {
      // Render graceful degradation fallback
      if (isGracefulMode && allowGracefulDegradation && gracefulFallback) {
        return gracefulFallback;
      }

      // Render full-page error
      return this.renderFullPageError();
    }

    // Render children normally when no error
    return children;
  }
}

export default GlobalErrorBoundary;
