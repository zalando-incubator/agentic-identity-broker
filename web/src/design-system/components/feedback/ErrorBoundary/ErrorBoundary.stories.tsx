/**
 * ErrorBoundary Component Stories
 */

import type { Meta, StoryObj } from '@storybook/react';
import { useState } from 'react';
import { ErrorBoundary } from './ErrorBoundary';
import { Button } from '@design-system/components/primitives/Button';

const meta = {
  title: 'Design System/Feedback/ErrorBoundary',
  component: ErrorBoundary,
  parameters: {
    layout: 'padded',
  },
  tags: ['autodocs'],
  argTypes: {
    showDetails: {
      control: 'boolean',
      description: 'Show detailed error information (dev mode)',
    },
    resetOnPropsChange: {
      control: 'boolean',
      description: 'Reset error state when props change',
    },
    boundaryName: {
      control: 'text',
      description: 'Custom name for error boundary identification',
    },
  },
} satisfies Meta<typeof ErrorBoundary>;

export default meta;
type Story = StoryObj<typeof meta>;

/**
 * Component that throws an error when button is clicked
 */
const BuggyComponent = ({ shouldThrow }: { shouldThrow: boolean }) => {
  if (shouldThrow) {
    throw new Error('Intentional error from BuggyComponent for testing');
  }
  return (
    <div className="p-6 bg-green-50 border border-green-200 rounded-lg">
      <h3 className="text-lg font-medium text-green-900 mb-2">
        Component Working Correctly
      </h3>
      <p className="text-green-700">
        This component will throw an error when you click the button below.
      </p>
    </div>
  );
};

/**
 * Component that throws error immediately
 */
const BrokenComponent = () => {
  throw new Error('This component always throws an error during render');
};

/**
 * Default error boundary catching a component error
 */
export const Default: Story = {
  render: () => {
    const [shouldThrow, setShouldThrow] = useState(false);

    return (
      <div className="space-y-4 max-w-2xl">
        <div className="p-4 bg-neutral-50 border border-neutral-200 rounded-lg">
          <p className="text-sm text-neutral-700 mb-3">
            Click the button below to trigger an error and see how the
            ErrorBoundary catches it.
          </p>
          <Button
            variant="danger"
            onClick={() => setShouldThrow(true)}
            size="sm"
          >
            Trigger Error
          </Button>
        </div>

        <ErrorBoundary>
          <BuggyComponent shouldThrow={shouldThrow} />
        </ErrorBoundary>
      </div>
    );
  },
  args: {
    children: <div>Default content</div>,
  },
};

/**
 * Error boundary with custom error message
 */
export const WithCustomMessage: Story = {
  render: () => {
    const [shouldThrow, setShouldThrow] = useState(false);

    return (
      <div className="space-y-4 max-w-2xl">
        <div className="p-4 bg-neutral-50 border border-neutral-200 rounded-lg">
          <p className="text-sm text-neutral-700 mb-3">
            This error boundary has a custom name that appears in the error
            message.
          </p>
          <Button
            variant="danger"
            onClick={() => setShouldThrow(true)}
            size="sm"
          >
            Trigger Error
          </Button>
        </div>

        <ErrorBoundary boundaryName="User Profile Section">
          <BuggyComponent shouldThrow={shouldThrow} />
        </ErrorBoundary>
      </div>
    );
  },
  args: {
    children: <div>Content</div>,
  },
};

/**
 * Error boundary with retry/reset actions
 */
export const WithActions: Story = {
  render: () => {
    const [shouldThrow, setShouldThrow] = useState(false);
    const [key, setKey] = useState(0);

    const handleReset = () => {
      setShouldThrow(false);
      setKey(key + 1); // Force re-mount
    };

    return (
      <div className="space-y-4 max-w-2xl">
        <div className="p-4 bg-neutral-50 border border-neutral-200 rounded-lg">
          <p className="text-sm text-neutral-700 mb-3">
            The error boundary provides "Try Again" and "Reload Page" buttons to
            recover from errors.
          </p>
          <div className="flex gap-2">
            <Button
              variant="danger"
              onClick={() => setShouldThrow(true)}
              size="sm"
            >
              Trigger Error
            </Button>
            <Button variant="outline" onClick={handleReset} size="sm">
              Reset Component
            </Button>
          </div>
        </div>

        <ErrorBoundary key={key} showDetails>
          <BuggyComponent shouldThrow={shouldThrow} />
        </ErrorBoundary>
      </div>
    );
  },
  args: {
    children: <div>Content</div>,
  },
};

/**
 * Multiple nested error boundaries
 */
export const Nested: Story = {
  render: () => {
    const [outerError, setOuterError] = useState(false);
    const [innerError, setInnerError] = useState(false);

    return (
      <div className="space-y-4 max-w-3xl">
        <div className="p-4 bg-neutral-50 border border-neutral-200 rounded-lg">
          <p className="text-sm text-neutral-700 mb-3">
            Nested error boundaries allow you to isolate errors to specific
            parts of your component tree. The inner boundary catches inner
            errors, while the outer boundary only catches if the inner one
            fails.
          </p>
          <div className="flex gap-2">
            <Button
              variant="danger"
              onClick={() => setOuterError(true)}
              size="sm"
            >
              Trigger Outer Error
            </Button>
            <Button
              variant="danger"
              onClick={() => setInnerError(true)}
              size="sm"
            >
              Trigger Inner Error
            </Button>
          </div>
        </div>

        {/* Outer boundary */}
        <ErrorBoundary boundaryName="Outer Container">
          <div className="border-2 border-blue-300 rounded-lg p-4 bg-blue-50">
            <h3 className="text-sm font-medium text-blue-900 mb-3">
              Outer Error Boundary
            </h3>

            <BuggyComponent shouldThrow={outerError} />

            <div className="mt-4">
              {/* Inner boundary */}
              <ErrorBoundary boundaryName="Inner Widget">
                <div className="border-2 border-purple-300 rounded-lg p-4 bg-purple-50">
                  <h4 className="text-sm font-medium text-purple-900 mb-2">
                    Inner Error Boundary
                  </h4>
                  <BuggyComponent shouldThrow={innerError} />
                </div>
              </ErrorBoundary>
            </div>
          </div>
        </ErrorBoundary>
      </div>
    );
  },
  args: {
    children: <div>Content</div>,
  },
};

/**
 * Development mode with detailed error information
 */
export const DevelopmentMode: Story = {
  render: () => {
    return (
      <div className="space-y-4 max-w-3xl">
        <div className="p-4 bg-blue-50 border border-blue-200 rounded-lg">
          <h4 className="text-sm font-medium text-blue-900 mb-2">
            Development Mode Features
          </h4>
          <ul className="text-sm text-blue-800 space-y-1 list-disc list-inside">
            <li>Shows detailed error message</li>
            <li>Displays component stack trace</li>
            <li>Shows full error stack</li>
            <li>Includes "Reload Page" button</li>
            <li>Shows error count</li>
            <li>Logs to browser console</li>
          </ul>
        </div>

        <ErrorBoundary showDetails boundaryName="Development Component">
          <BrokenComponent />
        </ErrorBoundary>
      </div>
    );
  },
  args: {
    children: <div>Content</div>,
  },
};

/**
 * Production mode with minimal error display
 */
export const ProductionMode: Story = {
  render: () => {
    return (
      <div className="space-y-4 max-w-3xl">
        <div className="p-4 bg-amber-50 border border-amber-200 rounded-lg">
          <h4 className="text-sm font-medium text-amber-900 mb-2">
            Production Mode Features
          </h4>
          <ul className="text-sm text-amber-800 space-y-1 list-disc list-inside">
            <li>Shows user-friendly error message</li>
            <li>Hides technical details</li>
            <li>Provides "Try Again" button only</li>
            <li>Logs still sent to error tracking service</li>
          </ul>
        </div>

        <ErrorBoundary showDetails={false} boundaryName="Production Component">
          <BrokenComponent />
        </ErrorBoundary>
      </div>
    );
  },
  args: {
    children: <div>Content</div>,
  },
};

/**
 * Error boundary with custom fallback UI
 */
export const WithFallbackUI: Story = {
  render: () => {
    const [shouldThrow, setShouldThrow] = useState(false);

    const customFallback = (error: Error, retry: () => void) => (
      <div className="p-8 bg-gradient-to-br from-purple-50 to-pink-50 border-2 border-purple-200 rounded-xl text-center">
        <div className="inline-flex items-center justify-center w-16 h-16 bg-purple-100 rounded-full mb-4">
          <svg
            className="w-8 h-8 text-purple-600"
            fill="none"
            viewBox="0 0 24 24"
            stroke="currentColor"
          >
            <path
              strokeLinecap="round"
              strokeLinejoin="round"
              strokeWidth={2}
              d="M12 8v4m0 4h.01M21 12a9 9 0 11-18 0 9 9 0 0118 0z"
            />
          </svg>
        </div>
        <h3 className="text-xl font-medium text-purple-900 mb-2">
          Oops! Something broke
        </h3>
        <p className="text-purple-700 mb-1 text-sm">
          Don't worry, it happens to the best of us.
        </p>
        <p className="text-purple-600 text-xs mb-6 font-mono">
          {error.message}
        </p>
        <Button variant="primary" onClick={retry} size="md">
          Let's Try That Again
        </Button>
      </div>
    );

    return (
      <div className="space-y-4 max-w-2xl">
        <div className="p-4 bg-neutral-50 border border-neutral-200 rounded-lg">
          <p className="text-sm text-neutral-700 mb-3">
            You can provide a custom fallback UI as a function that receives the
            error and retry callback.
          </p>
          <Button
            variant="danger"
            onClick={() => setShouldThrow(true)}
            size="sm"
          >
            Trigger Error
          </Button>
        </div>

        <ErrorBoundary fallback={customFallback}>
          <BuggyComponent shouldThrow={shouldThrow} />
        </ErrorBoundary>
      </div>
    );
  },
  args: {
    children: <div>Content</div>,
  },
};

/**
 * Interactive playground with all controls
 */
export const Playground: Story = {
  render: (args) => {
    const [shouldThrow, setShouldThrow] = useState(false);
    const [key, setKey] = useState(0);

    const handleReset = () => {
      setShouldThrow(false);
      setKey(key + 1);
    };

    return (
      <div className="space-y-4 max-w-2xl">
        <div className="p-4 bg-neutral-50 border border-neutral-200 rounded-lg">
          <p className="text-sm text-neutral-700 mb-3">
            Use the controls below to customize the error boundary behavior.
            Click the button to trigger an error and see it in action.
          </p>
          <div className="flex gap-2">
            <Button
              variant="danger"
              onClick={() => setShouldThrow(true)}
              size="sm"
            >
              Trigger Error
            </Button>
            <Button variant="outline" onClick={handleReset} size="sm">
              Reset
            </Button>
          </div>
        </div>

        <ErrorBoundary key={key} {...args}>
          <BuggyComponent shouldThrow={shouldThrow} />
        </ErrorBoundary>
      </div>
    );
  },
  args: {
    children: <div>Playground content</div>,
    showDetails: true,
    resetOnPropsChange: false,
    boundaryName: 'Playground Component',
  },
};

/**
 * Real-world consent management examples
 */
export const RealWorldExamples: Story = {
  args: {
    children: <div>Content</div>,
  },
  render: () => {
    const [consentFormError, setConsentFormError] = useState(false);
    const [apiError, setApiError] = useState(false);

    return (
      <div className="space-y-6 max-w-3xl">
        <div>
          <h3 className="text-lg font-medium text-neutral-900 mb-4">
            Consent Management Error Scenarios
          </h3>

          <div className="space-y-4">
            {/* Consent Form Error */}
            <div>
              <h4 className="text-sm font-medium text-neutral-800 mb-2">
                1. Consent Form Validation Error
              </h4>
              <div className="mb-2">
                <Button
                  variant="danger"
                  onClick={() => setConsentFormError(true)}
                  size="sm"
                >
                  Simulate Form Error
                </Button>
              </div>
              <ErrorBoundary
                boundaryName="Consent Form"
                showDetails={false}
                onError={(error) => {
                  console.log('Logging consent form error:', error);
                }}
              >
                <div className="p-4 bg-white border border-neutral-200 rounded-lg">
                  <BuggyComponent shouldThrow={consentFormError} />
                </div>
              </ErrorBoundary>
            </div>

            {/* API Integration Error */}
            <div>
              <h4 className="text-sm font-medium text-neutral-800 mb-2">
                2. API Integration Error
              </h4>
              <div className="mb-2">
                <Button
                  variant="danger"
                  onClick={() => setApiError(true)}
                  size="sm"
                >
                  Simulate API Error
                </Button>
              </div>
              <ErrorBoundary
                boundaryName="API Dashboard"
                showDetails={false}
                fallback={(error, retry) => (
                  <div className="p-6 bg-red-50 border border-red-200 rounded-lg text-center">
                    <h4 className="text-lg font-medium text-red-900 mb-2">
                      Unable to Load Dashboard
                    </h4>
                    <p className="text-sm text-red-700 mb-4">
                      We're having trouble connecting to our servers. Please
                      check your connection and try again.
                    </p>
                    <Button variant="primary" onClick={retry} size="sm">
                      Retry Connection
                    </Button>
                  </div>
                )}
              >
                <div className="p-4 bg-white border border-neutral-200 rounded-lg">
                  <BuggyComponent shouldThrow={apiError} />
                </div>
              </ErrorBoundary>
            </div>

            {/* Graceful Degradation */}
            <div>
              <h4 className="text-sm font-medium text-neutral-800 mb-2">
                3. Feature Widget with Graceful Degradation
              </h4>
              <div className="grid grid-cols-2 gap-4">
                <ErrorBoundary
                  boundaryName="Analytics Widget"
                  fallback={
                    <div className="p-4 bg-neutral-100 border border-neutral-300 rounded-lg text-center">
                      <p className="text-sm text-neutral-600">
                        Analytics temporarily unavailable
                      </p>
                    </div>
                  }
                >
                  <div className="p-4 bg-white border border-neutral-200 rounded-lg">
                    <h5 className="font-medium text-sm mb-2">Analytics</h5>
                    <p className="text-sm text-neutral-600">
                      Widget working correctly
                    </p>
                  </div>
                </ErrorBoundary>

                <ErrorBoundary
                  boundaryName="Reports Widget"
                  fallback={
                    <div className="p-4 bg-neutral-100 border border-neutral-300 rounded-lg text-center">
                      <p className="text-sm text-neutral-600">
                        Reports temporarily unavailable
                      </p>
                    </div>
                  }
                >
                  <div className="p-4 bg-white border border-neutral-200 rounded-lg">
                    <h5 className="font-medium text-sm mb-2">Reports</h5>
                    <p className="text-sm text-neutral-600">
                      Widget working correctly
                    </p>
                  </div>
                </ErrorBoundary>
              </div>
            </div>
          </div>
        </div>

        <div className="pt-4 border-t border-neutral-200">
          <button
            onClick={() => {
              setConsentFormError(false);
              setApiError(false);
            }}
            className="text-sm text-trust-deep hover:text-trust-hover underline"
          >
            Reset All Examples
          </button>
        </div>
      </div>
    );
  },
};

/**
 * Error logging integration example
 */
export const WithErrorLogging: Story = {
  args: {
    children: <div>Content</div>,
  },
  render: () => {
    const [logs, setLogs] = useState<string[]>([]);
    const [shouldThrow, setShouldThrow] = useState(false);

    const handleError = (error: Error, errorInfo: React.ErrorInfo) => {
      const logEntry = `[${new Date().toLocaleTimeString()}] ${error.message}`;
      setLogs((prev) => [...prev, logEntry]);

      // In production, you would send this to your error tracking service
      // Example: Sentry.captureException(error, { contexts: { react: errorInfo } });
    };

    return (
      <div className="space-y-4 max-w-3xl">
        <div className="p-4 bg-neutral-50 border border-neutral-200 rounded-lg">
          <h4 className="text-sm font-medium text-neutral-900 mb-2">
            Error Logging Integration
          </h4>
          <p className="text-sm text-neutral-700 mb-3">
            The onError callback allows you to integrate with error tracking
            services like Sentry, LogRocket, or custom logging solutions.
          </p>
          <Button
            variant="danger"
            onClick={() => setShouldThrow(true)}
            size="sm"
          >
            Trigger Error
          </Button>
        </div>

        {logs.length > 0 && (
          <div className="p-4 bg-neutral-900 text-green-400 rounded-lg font-mono text-xs">
            <div className="flex justify-between items-center mb-2">
              <span className="font-medium">Error Log:</span>
              <button
                onClick={() => setLogs([])}
                className="text-neutral-400 hover:text-white text-xs underline"
              >
                Clear
              </button>
            </div>
            {logs.map((log, index) => (
              <div key={index}>{log}</div>
            ))}
          </div>
        )}

        <ErrorBoundary onError={handleError} showDetails={false}>
          <BuggyComponent shouldThrow={shouldThrow} />
        </ErrorBoundary>
      </div>
    );
  },
};

/**
 * Reset on props change demonstration
 */
export const ResetOnPropsChange: Story = {
  args: {
    children: <div>Content</div>,
  },
  render: () => {
    const [userId, setUserId] = useState(1);
    const [shouldThrow, setShouldThrow] = useState(false);

    return (
      <div className="space-y-4 max-w-2xl">
        <div className="p-4 bg-neutral-50 border border-neutral-200 rounded-lg">
          <h4 className="text-sm font-medium text-neutral-900 mb-2">
            Auto-Reset on Route/Prop Changes
          </h4>
          <p className="text-sm text-neutral-700 mb-3">
            When resetOnPropsChange is enabled, the error boundary automatically
            resets when props change (useful for route transitions).
          </p>
          <div className="flex gap-2">
            <Button
              variant="danger"
              onClick={() => setShouldThrow(true)}
              size="sm"
            >
              Trigger Error
            </Button>
            <Button
              variant="outline"
              onClick={() => setUserId(userId + 1)}
              size="sm"
            >
              Change User (ID: {userId})
            </Button>
          </div>
        </div>

        <ErrorBoundary resetOnPropsChange showDetails={false}>
          <div className="p-4 bg-white border border-neutral-200 rounded-lg">
            <h4 className="text-sm font-medium mb-2">User Profile: {userId}</h4>
            <BuggyComponent shouldThrow={shouldThrow} />
          </div>
        </ErrorBoundary>
      </div>
    );
  },
};

/**
 * Accessibility features demonstration
 */
export const Accessibility: Story = {
  args: {
    children: <div>Content</div>,
  },
  render: () => (
    <div className="space-y-6 max-w-3xl">
      <div className="p-4 bg-neutral-50 border border-neutral-200 rounded-lg">
        <h4 className="text-sm font-medium text-neutral-900 mb-2">
          Accessibility Features
        </h4>
        <ul className="text-sm text-neutral-700 space-y-1 list-disc list-inside">
          <li>
            Uses <code>role="alert"</code> for immediate screen reader
            announcement
          </li>
          <li>
            <code>aria-live="assertive"</code> ensures errors are announced
            immediately
          </li>
          <li>Buttons are fully keyboard accessible with focus indicators</li>
          <li>
            Error icons are marked with <code>aria-hidden="true"</code>
          </li>
          <li>Clear, semantic heading structure</li>
          <li>Collapsible sections use proper details/summary elements</li>
          <li>Color contrast meets WCAG 2.1 AA requirements</li>
        </ul>
      </div>

      <ErrorBoundary showDetails boundaryName="Accessible Error Example">
        <BrokenComponent />
      </ErrorBoundary>
    </div>
  ),
  parameters: {
    a11y: {
      config: {
        rules: [
          {
            id: 'color-contrast',
            enabled: true,
          },
          {
            id: 'button-name',
            enabled: true,
          },
          {
            id: 'aria-roles',
            enabled: true,
          },
        ],
      },
    },
  },
};
