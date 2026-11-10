/**
 * GlobalErrorBoundary Component Stories
 */

import type { Meta, StoryObj } from '@storybook/react';
import { useState } from 'react';
import { GlobalErrorBoundary } from './GlobalErrorBoundary';
import { Button } from '@design-system/components/primitives/Button';

const meta = {
  title: 'Design System/Feedback/GlobalErrorBoundary',
  component: GlobalErrorBoundary,
  parameters: {
    layout: 'fullscreen',
  },
  tags: ['autodocs'],
  argTypes: {
    appName: {
      control: 'text',
      description: 'Application name to display in error messages',
    },
    supportEmail: {
      control: 'text',
      description: 'Support email for users to contact',
    },
    supportPhone: {
      control: 'text',
      description: 'Support phone number',
    },
    supportUrl: {
      control: 'text',
      description: 'URL to support documentation or help center',
    },
    showDetails: {
      control: 'boolean',
      description: 'Show detailed error information (dev mode)',
    },
    allowGracefulDegradation: {
      control: 'boolean',
      description: 'Allow graceful degradation instead of full error page',
    },
    recoveryStrategies: {
      control: 'check',
      options: ['reload', 'navigate', 'contact-support'],
      description: 'Available recovery strategies',
    },
  },
} satisfies Meta<typeof GlobalErrorBoundary>;

export default meta;
type Story = StoryObj<typeof meta>;

/**
 * Component that throws an error when triggered
 */
const BuggyApp = ({ shouldThrow }: { shouldThrow: boolean }) => {
  if (shouldThrow) {
    throw new Error('Critical application error occurred');
  }
  return (
    <div className="min-h-screen bg-gradient-to-br from-blue-50 to-indigo-100 flex items-center justify-center p-6">
      <div className="bg-white rounded-xl shadow-lg p-8 max-w-lg w-full">
        <h2 className="text-2xl font-medium text-neutral-900 mb-4">
          Application Running Normally
        </h2>
        <p className="text-neutral-700 mb-6">
          This represents your full application. Click the button below to
          simulate a critical error that will be caught by the
          GlobalErrorBoundary.
        </p>
        <div className="flex gap-3">
          <Button variant="primary" size="md">
            Normal Action
          </Button>
          <Button variant="secondary" size="md">
            Another Action
          </Button>
        </div>
      </div>
    </div>
  );
};

/**
 * Component that always throws
 */
const BrokenApp = () => {
  throw new Error('Application failed to initialize properly');
};

/**
 * Network error simulator
 */
const NetworkErrorApp = ({ shouldThrow }: { shouldThrow: boolean }) => {
  if (shouldThrow) {
    throw new Error(
      'Network connection timeout - failed to fetch data from server',
    );
  }
  return <BuggyApp shouldThrow={false} />;
};

/**
 * Permission error simulator
 */
const PermissionErrorApp = ({ shouldThrow }: { shouldThrow: boolean }) => {
  if (shouldThrow) {
    throw new Error(
      'Permission denied: Unauthorized access to protected resource',
    );
  }
  return <BuggyApp shouldThrow={false} />;
};

/**
 * Default - Basic global boundary wrapper
 */
export const Default: Story = {
  render: () => {
    const [shouldThrow, setShouldThrow] = useState(false);

    return (
      <div className="relative">
        {!shouldThrow && (
          <div className="fixed top-4 left-1/2 transform -translate-x-1/2 z-50">
            <Button
              variant="danger"
              onClick={() => setShouldThrow(true)}
              size="lg"
            >
              Trigger Critical Error
            </Button>
          </div>
        )}

        <GlobalErrorBoundary appName="Agentic Identity Broker">
          <BuggyApp shouldThrow={shouldThrow} />
        </GlobalErrorBoundary>
      </div>
    );
  },
};

/**
 * WithFullPageError - Large full-page error display
 */
export const WithFullPageError: Story = {
  render: () => {
    return (
      <GlobalErrorBoundary
        appName="Agentic Identity Broker"
        showDetails={false}
      >
        <BrokenApp />
      </GlobalErrorBoundary>
    );
  },
};

/**
 * WithFooterActions - Footer with support/contact information
 */
export const WithFooterActions: Story = {
  render: () => {
    return (
      <GlobalErrorBoundary
        appName="Agentic Identity Broker"
        supportEmail="agentic-identity-broker@example.com"
        supportPhone="0-000-000-0000"
        supportUrl="https://agentic-identity-broker-docs.example.com/help"
        showDetails={false}
      >
        <BrokenApp />
      </GlobalErrorBoundary>
    );
  },
};

/**
 * WithErrorReporting - Send error to backend/logging service
 */
export const WithErrorReporting: Story = {
  render: () => {
    const [errorLogs, setErrorLogs] = useState<string[]>([]);

    const handleError = (error: Error, errorInfo: React.ErrorInfo) => {
      const timestamp = new Date().toLocaleTimeString();
      const logEntry = `[${timestamp}] Error reported: ${error.message}`;
      setErrorLogs((prev) => [...prev, logEntry]);

      // In production, send to error tracking service
      // Example: Sentry.captureException(error, { contexts: { react: errorInfo } });
      console.log('Sending error to monitoring service:', {
        error: error.message,
        stack: error.stack,
        componentStack: errorInfo.componentStack,
      });
    };

    const [shouldThrow, setShouldThrow] = useState(false);

    return (
      <div className="relative">
        {/* Error log display */}
        {errorLogs.length > 0 && !shouldThrow && (
          <div className="fixed top-4 right-4 z-50 bg-neutral-900 text-green-400 rounded-lg p-4 max-w-md font-mono text-xs shadow-2xl">
            <div className="flex justify-between items-center mb-2">
              <span className="font-medium">Error Reporting Log:</span>
              <button
                onClick={() => setErrorLogs([])}
                className="text-neutral-400 hover:text-white underline"
              >
                Clear
              </button>
            </div>
            {errorLogs.map((log, index) => (
              <div key={index} className="mb-1">
                {log}
              </div>
            ))}
          </div>
        )}

        {!shouldThrow && (
          <div className="fixed top-4 left-1/2 transform -translate-x-1/2 z-50">
            <Button
              variant="danger"
              onClick={() => setShouldThrow(true)}
              size="lg"
            >
              Trigger Error (with Reporting)
            </Button>
          </div>
        )}

        <GlobalErrorBoundary
          appName="Agentic Identity Broker"
          onError={handleError}
          showDetails={false}
        >
          <BuggyApp shouldThrow={shouldThrow} />
        </GlobalErrorBoundary>
      </div>
    );
  },
};

/**
 * WithRecovery - Auto-recovery/retry mechanisms
 */
export const WithRecovery: Story = {
  render: () => {
    const [shouldThrow, setShouldThrow] = useState(false);
    const [attempt, setAttempt] = useState(1);

    return (
      <div className="relative">
        {!shouldThrow && (
          <div className="fixed top-4 left-1/2 transform -translate-x-1/2 z-50 flex flex-col items-center gap-2">
            <Button
              variant="danger"
              onClick={() => setShouldThrow(true)}
              size="lg"
            >
              Trigger Error (Attempt #{attempt})
            </Button>
            <span className="text-xs text-neutral-600 bg-white px-3 py-1 rounded-full shadow">
              Try the recovery buttons to reload or navigate
            </span>
          </div>
        )}

        <GlobalErrorBoundary
          appName="Agentic Identity Broker"
          recoveryStrategies={['reload', 'navigate', 'contact-support']}
          showDetails={false}
          onError={() => setAttempt(attempt + 1)}
        >
          <BuggyApp shouldThrow={shouldThrow} />
        </GlobalErrorBoundary>
      </div>
    );
  },
};

/**
 * GracefulDegradation - App continues with reduced features
 */
export const GracefulDegradation: Story = {
  render: () => {
    const [shouldThrow, setShouldThrow] = useState(false);

    const LimitedApp = () => (
      <div className="min-h-screen bg-gradient-to-br from-amber-50 to-orange-100 flex items-center justify-center p-6">
        <div className="bg-white rounded-xl shadow-lg p-8 max-w-lg w-full border-2 border-amber-400">
          <div className="flex items-center gap-3 mb-4">
            <div className="w-12 h-12 bg-amber-100 rounded-full flex items-center justify-center">
              <svg
                className="w-6 h-6 text-amber-600"
                fill="none"
                viewBox="0 0 24 24"
                stroke="currentColor"
              >
                <path
                  strokeLinecap="round"
                  strokeLinejoin="round"
                  strokeWidth={2}
                  d="M12 9v2m0 4h.01m-6.938 4h13.856c1.54 0 2.502-1.667 1.732-3L13.732 4c-.77-1.333-2.694-1.333-3.464 0L3.34 16c-.77 1.333.192 3 1.732 3z"
                />
              </svg>
            </div>
            <h2 className="text-2xl font-medium text-amber-900">
              Limited Functionality Mode
            </h2>
          </div>
          <p className="text-amber-800 mb-6">
            Some features are temporarily unavailable, but you can continue
            using essential functions. Full functionality will be restored
            shortly.
          </p>
          <div className="space-y-3">
            <Button variant="primary" size="md" fullWidth>
              Access Core Features
            </Button>
            <Button variant="outline" size="md" fullWidth>
              View Available Services
            </Button>
            <button
              onClick={() => window.location.reload()}
              className="w-full text-sm text-amber-700 hover:text-amber-900 underline"
            >
              Reload to restore full functionality
            </button>
          </div>
        </div>
      </div>
    );

    return (
      <div className="relative">
        {!shouldThrow && (
          <div className="fixed top-4 left-1/2 transform -translate-x-1/2 z-50">
            <Button
              variant="danger"
              onClick={() => setShouldThrow(true)}
              size="lg"
            >
              Trigger Error (Graceful Mode)
            </Button>
          </div>
        )}

        <GlobalErrorBoundary
          appName="Agentic Identity Broker"
          allowGracefulDegradation
          gracefulFallback={<LimitedApp />}
          recoveryStrategies={['reload', 'navigate', 'contact-support']}
          showDetails={false}
        >
          <BuggyApp shouldThrow={shouldThrow} />
        </GlobalErrorBoundary>
      </div>
    );
  },
};

/**
 * ErrorCategories - Different error types (network, permission, etc.)
 */
export const ErrorCategories: Story = {
  render: () => {
    const [errorType, setErrorType] = useState<
      'none' | 'network' | 'permission' | 'generic'
    >('none');

    return (
      <div className="relative">
        {errorType === 'none' && (
          <div className="fixed top-4 left-1/2 transform -translate-x-1/2 z-50">
            <div className="bg-white rounded-xl shadow-2xl p-6 border-2 border-neutral-200">
              <h3 className="text-lg font-medium text-neutral-900 mb-4 text-center">
                Simulate Different Error Types
              </h3>
              <div className="flex flex-col gap-2">
                <Button
                  variant="danger"
                  onClick={() => setErrorType('network')}
                  size="md"
                  fullWidth
                >
                  Network Error
                </Button>
                <Button
                  variant="danger"
                  onClick={() => setErrorType('permission')}
                  size="md"
                  fullWidth
                >
                  Permission Error
                </Button>
                <Button
                  variant="danger"
                  onClick={() => setErrorType('generic')}
                  size="md"
                  fullWidth
                >
                  Generic Error
                </Button>
              </div>
            </div>
          </div>
        )}

        <GlobalErrorBoundary
          appName="Agentic Identity Broker"
          supportEmail="agentic-identity-broker@example.com"
          showDetails={false}
        >
          {errorType === 'network' && <NetworkErrorApp shouldThrow={true} />}
          {errorType === 'permission' && (
            <PermissionErrorApp shouldThrow={true} />
          )}
          {errorType === 'generic' && <BrokenApp />}
          {errorType === 'none' && <BuggyApp shouldThrow={false} />}
        </GlobalErrorBoundary>
      </div>
    );
  },
};

/**
 * Development mode with detailed error information
 */
export const DevelopmentMode: Story = {
  render: () => {
    return (
      <div className="relative">
        <div className="fixed top-4 left-1/2 transform -translate-x-1/2 z-50">
          <div className="bg-blue-900 text-white px-4 py-2 rounded-lg shadow-lg text-sm font-medium">
            Development Mode - Full Error Details Shown
          </div>
        </div>

        <GlobalErrorBoundary
          appName="Agentic Identity Broker"
          showDetails={true}
          supportEmail="dev-support@agentic-identity-broker.example.com"
          supportUrl="https://agentic-identity-broker-docs.example.com/troubleshooting"
        >
          <BrokenApp />
        </GlobalErrorBoundary>
      </div>
    );
  },
};

/**
 * Production mode with minimal error display
 */
export const ProductionMode: Story = {
  render: () => {
    return (
      <div className="relative">
        <div className="fixed top-4 left-1/2 transform -translate-x-1/2 z-50">
          <div className="bg-green-900 text-white px-4 py-2 rounded-lg shadow-lg text-sm font-medium">
            Production Mode - User-Friendly Display
          </div>
        </div>

        <GlobalErrorBoundary
          appName="Agentic Identity Broker"
          showDetails={false}
          supportEmail="support@agentic-identity-broker.example.com"
          supportPhone="0-000-000-0000"
          supportUrl="https://help.agentic-identity-broker.example.com"
        >
          <BrokenApp />
        </GlobalErrorBoundary>
      </div>
    );
  },
};

/**
 * Complete example with all features
 */
export const CompleteExample: Story = {
  render: () => {
    const [errorLogs, setErrorLogs] = useState<string[]>([]);
    const [shouldThrow, setShouldThrow] = useState(false);

    const handleError = (error: Error, errorInfo: React.ErrorInfo) => {
      const timestamp = new Date().toISOString();
      const logEntry = `[${timestamp}] ${error.message}`;
      setErrorLogs((prev) => [...prev, logEntry]);
    };

    const LimitedApp = () => (
      <div className="min-h-screen bg-gradient-to-br from-amber-50 to-orange-100 flex items-center justify-center p-6">
        <div className="bg-white rounded-xl shadow-lg p-8 max-w-lg w-full">
          <h2 className="text-2xl font-medium text-amber-900 mb-4">
            Limited Mode Active
          </h2>
          <p className="text-amber-800 mb-6">
            Operating with reduced functionality. Core features remain
            available.
          </p>
          <Button variant="primary" size="md" fullWidth>
            Access Core Features
          </Button>
        </div>
      </div>
    );

    return (
      <div className="relative">
        {errorLogs.length > 0 && !shouldThrow && (
          <div className="fixed top-4 right-4 z-50 bg-neutral-900 text-green-400 rounded-lg p-4 max-w-sm font-mono text-xs shadow-2xl">
            <div className="font-medium mb-2">Error Logs:</div>
            {errorLogs.slice(-3).map((log, index) => (
              <div key={index} className="truncate">
                {log}
              </div>
            ))}
          </div>
        )}

        {!shouldThrow && (
          <div className="fixed top-4 left-1/2 transform -translate-x-1/2 z-50">
            <Button
              variant="danger"
              onClick={() => setShouldThrow(true)}
              size="lg"
            >
              Trigger Error (Full Featured)
            </Button>
          </div>
        )}

        <GlobalErrorBoundary
          appName="Agentic Identity Broker"
          supportEmail="support@agentic-identity-broker.example.com"
          supportPhone="0-000-000-0000"
          supportUrl="https://docs.agentic-identity-broker.example.com/help"
          onError={handleError}
          showDetails={false}
          recoveryStrategies={['reload', 'navigate', 'contact-support']}
          allowGracefulDegradation
          gracefulFallback={<LimitedApp />}
        >
          <BuggyApp shouldThrow={shouldThrow} />
        </GlobalErrorBoundary>
      </div>
    );
  },
};

/**
 * Playground - Interactive controls for all props
 */
export const Playground: Story = {
  render: (args) => {
    const [shouldThrow, setShouldThrow] = useState(false);

    return (
      <div className="relative">
        {!shouldThrow && (
          <div className="fixed top-4 left-1/2 transform -translate-x-1/2 z-50">
            <div className="bg-white rounded-xl shadow-2xl p-6 border-2 border-neutral-200">
              <h3 className="text-lg font-medium text-neutral-900 mb-4 text-center">
                Playground Controls
              </h3>
              <p className="text-sm text-neutral-600 mb-4 text-center">
                Adjust controls in the panel below, then trigger the error
              </p>
              <Button
                variant="danger"
                onClick={() => setShouldThrow(true)}
                size="lg"
                fullWidth
              >
                Trigger Error
              </Button>
            </div>
          </div>
        )}

        <GlobalErrorBoundary {...args}>
          <BuggyApp shouldThrow={shouldThrow} />
        </GlobalErrorBoundary>
      </div>
    );
  },
  args: {
    appName: 'Agentic Identity Broker',
    supportEmail: 'support@agentic-identity-broker.example.com',
    supportPhone: '0-000-000-0000',
    supportUrl: 'https://docs.agentic-identity-broker.example.com/help',
    showDetails: false,
    recoveryStrategies: ['reload', 'navigate', 'contact-support'],
    allowGracefulDegradation: false,
  },
};

/**
 * Real-world consent management examples
 */
export const RealWorldScenarios: Story = {
  render: () => {
    const [scenario, setScenario] = useState<
      'none' | 'auth' | 'network' | 'permission'
    >('none');

    const AuthFailureApp = () => {
      throw new Error('Authentication service failed to initialize');
    };

    return (
      <div className="relative">
        {scenario === 'none' && (
          <div className="fixed top-4 left-1/2 transform -translate-x-1/2 z-50">
            <div className="bg-white rounded-xl shadow-2xl p-6 border-2 border-neutral-200 max-w-md">
              <h3 className="text-lg font-medium text-neutral-900 mb-4">
                Real-World Error Scenarios
              </h3>
              <p className="text-sm text-neutral-600 mb-4">
                Simulate common errors in consent management applications
              </p>
              <div className="space-y-2">
                <Button
                  variant="danger"
                  onClick={() => setScenario('auth')}
                  size="md"
                  fullWidth
                >
                  Authentication Service Failure
                </Button>
                <Button
                  variant="danger"
                  onClick={() => setScenario('network')}
                  size="md"
                  fullWidth
                >
                  Network Connectivity Issue
                </Button>
                <Button
                  variant="danger"
                  onClick={() => setScenario('permission')}
                  size="md"
                  fullWidth
                >
                  Insufficient Permissions
                </Button>
              </div>
            </div>
          </div>
        )}

        <GlobalErrorBoundary
          appName="Consent Manager"
          supportEmail="support@agentic-identity-broker.example.com"
          supportPhone="0-000-000-0000"
          supportUrl="https://docs.agentic-identity-broker.example.com"
          showDetails={false}
          recoveryStrategies={['reload', 'navigate', 'contact-support']}
        >
          {scenario === 'auth' && <AuthFailureApp />}
          {scenario === 'network' && <NetworkErrorApp shouldThrow={true} />}
          {scenario === 'permission' && (
            <PermissionErrorApp shouldThrow={true} />
          )}
          {scenario === 'none' && <BuggyApp shouldThrow={false} />}
        </GlobalErrorBoundary>
      </div>
    );
  },
};

/**
 * Accessibility features demonstration
 */
export const Accessibility: Story = {
  render: () => (
    <div className="relative">
      <div className="fixed top-4 left-1/2 transform -translate-x-1/2 z-50">
        <div className="bg-white rounded-xl shadow-2xl p-6 border-2 border-blue-200 max-w-lg">
          <h3 className="text-lg font-medium text-blue-900 mb-3">
            Accessibility Features
          </h3>
          <ul className="text-sm text-blue-800 space-y-2 list-disc list-inside">
            <li>role="alert" for immediate screen reader announcement</li>
            <li>aria-live="assertive" for critical errors</li>
            <li>Full keyboard navigation support</li>
            <li>WCAG 2.1 AA compliant color contrast</li>
            <li>Clear, semantic heading structure</li>
            <li>Focus indicators on all interactive elements</li>
          </ul>
        </div>
      </div>

      <GlobalErrorBoundary
        appName="Agentic Identity Broker"
        supportEmail="accessibility@agentic-identity-broker.example.com"
        showDetails={true}
      >
        <BrokenApp />
      </GlobalErrorBoundary>
    </div>
  ),
  parameters: {
    a11y: {
      config: {
        rules: [
          { id: 'color-contrast', enabled: true },
          { id: 'button-name', enabled: true },
          { id: 'aria-roles', enabled: true },
          { id: 'landmark-one-main', enabled: false }, // Error pages don't need main landmarks
        ],
      },
    },
  },
};
