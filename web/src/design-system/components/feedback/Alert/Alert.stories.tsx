/**
 * Alert Component Stories
 */

import type { Meta, StoryObj } from '@storybook/react';
import { useState } from 'react';
import { Alert } from './Alert';

const meta = {
  title: 'Design System/Feedback/Alert',
  component: Alert,
  parameters: {
    layout: 'padded',
  },
  tags: ['autodocs'],
  argTypes: {
    variant: {
      control: 'select',
      options: ['info', 'success', 'warning', 'error'],
      description: 'Visual style variant based on message severity',
    },
    title: {
      control: 'text',
      description: 'Optional bold title text',
    },
    dismissible: {
      control: 'boolean',
      description: 'Whether the alert can be dismissed',
    },
    banner: {
      control: 'boolean',
      description: 'Display as full-width banner',
    },
    hideIcon: {
      control: 'boolean',
      description: 'Hide the default icon',
    },
    children: {
      control: 'text',
      description: 'Alert message content',
    },
  },
} satisfies Meta<typeof Alert>;

export default meta;
type Story = StoryObj<typeof meta>;

/**
 * Default alert with info variant
 */
export const Default: Story = {
  args: {
    variant: 'info',
    children:
      'This is an informational message to provide context or guidance to the user.',
  },
};

/**
 * All alert variants displayed together
 */
export const Variants: Story = {
  render: () => (
    <div className="space-y-4 max-w-2xl">
      <Alert variant="info">
        This is an informational alert. It provides helpful context or
        additional information about the current action.
      </Alert>

      <Alert variant="success">
        Success! Your changes have been saved and will take effect immediately.
      </Alert>

      <Alert variant="warning">
        Warning: This action cannot be undone. Please review your selection
        carefully before proceeding.
      </Alert>

      <Alert variant="error">
        Error: Unable to process your request. Please check your connection and
        try again.
      </Alert>
    </div>
  ),
  args: {
    variant: 'info',
    children: 'Alert content',
  },
};

/**
 * Alerts with leading icons (default behavior)
 */
export const WithIcons: Story = {
  render: () => (
    <div className="space-y-4 max-w-2xl">
      <Alert variant="info">
        Each alert variant comes with a default icon that matches its semantic
        meaning.
      </Alert>

      <Alert variant="success">
        The success icon indicates a positive outcome or completed action.
      </Alert>

      <Alert variant="warning">
        The warning icon draws attention to important caution messages.
      </Alert>

      <Alert variant="error">
        The error icon clearly indicates a problem that needs attention.
      </Alert>
    </div>
  ),
  args: {
    variant: 'info',
    children: 'Alert with icon',
  },
};

/**
 * Alerts with action buttons and/or dismiss buttons
 */
export const WithActions: Story = {
  render: () => {
    const [alerts, setAlerts] = useState({
      action: true,
      dismiss: true,
      both: true,
    });

    return (
      <div className="space-y-4 max-w-2xl">
        {alerts.action && (
          <Alert
            variant="warning"
            action={{
              label: 'Review',
              onClick: () => alert('Action clicked!'),
            }}
          >
            Your subscription will expire in 3 days. Review your plan to
            continue using all features.
          </Alert>
        )}

        {alerts.dismiss && (
          <Alert
            variant="info"
            dismissible
            onDismiss={() => setAlerts({ ...alerts, dismiss: false })}
          >
            New features are available! Check out our latest updates in the
            changelog.
          </Alert>
        )}

        {alerts.both && (
          <Alert
            variant="error"
            dismissible
            onDismiss={() => setAlerts({ ...alerts, both: false })}
            action={{
              label: 'Retry',
              onClick: () => alert('Retrying...'),
            }}
          >
            Payment processing failed. Please verify your payment method and try
            again.
          </Alert>
        )}

        {!alerts.action && !alerts.dismiss && !alerts.both && (
          <div className="text-center p-8 border border-dashed border-neutral-300 rounded-lg">
            <p className="text-neutral-600 mb-4">All alerts dismissed</p>
            <button
              onClick={() =>
                setAlerts({ action: true, dismiss: true, both: true })
              }
              className="px-4 py-2 bg-trust-deep text-white rounded-md hover:bg-trust-hover transition-colors"
            >
              Reset Alerts
            </button>
          </div>
        )}
      </div>
    );
  },
  args: {
    variant: 'info',
    children: 'Alert with actions',
  },
};

/**
 * Alerts with bold titles and descriptions
 */
export const WithTitle: Story = {
  render: () => (
    <div className="space-y-4 max-w-2xl">
      <Alert variant="info" title="Information">
        Titles help organize complex alerts and provide clear context for the
        message. They appear in bold above the description.
      </Alert>

      <Alert variant="success" title="Access Granted">
        You now have editor permissions for the "Marketing Assets" workspace.
        You can view, edit, and share all documents.
      </Alert>

      <Alert variant="warning" title="Action Required">
        Your account requires two-factor authentication. Please enable 2FA
        within 7 days to maintain access to sensitive resources.
      </Alert>

      <Alert variant="error" title="Authentication Failed">
        We couldn't verify your identity. Your session may have expired or your
        credentials are invalid. Please sign in again.
      </Alert>
    </div>
  ),
  args: {
    variant: 'info',
    title: 'Alert Title',
    children: 'Alert description goes here',
  },
};

/**
 * Dismissible alerts with fade-out animation
 */
export const Dismissible: Story = {
  render: () => {
    const [alertStates, setAlertStates] = useState({
      alert1: true,
      alert2: true,
      alert3: true,
      alert4: true,
    });

    const resetAlerts = () => {
      setAlertStates({
        alert1: true,
        alert2: true,
        alert3: true,
        alert4: true,
      });
    };

    const anyDismissed = !Object.values(alertStates).some((v) => v);

    return (
      <div className="space-y-4 max-w-2xl">
        {alertStates.alert1 && (
          <Alert
            variant="info"
            title="Tip"
            dismissible
            onDismiss={() => setAlertStates({ ...alertStates, alert1: false })}
          >
            Click the × button to dismiss any alert. It will fade out smoothly.
          </Alert>
        )}

        {alertStates.alert2 && (
          <Alert
            variant="success"
            dismissible
            onDismiss={() => setAlertStates({ ...alertStates, alert2: false })}
          >
            Your profile has been updated successfully.
          </Alert>
        )}

        {alertStates.alert3 && (
          <Alert
            variant="warning"
            title="Reminder"
            dismissible
            onDismiss={() => setAlertStates({ ...alertStates, alert3: false })}
          >
            Don't forget to save your work before closing the window.
          </Alert>
        )}

        {alertStates.alert4 && (
          <Alert
            variant="error"
            dismissible
            onDismiss={() => setAlertStates({ ...alertStates, alert4: false })}
          >
            Connection lost. Attempting to reconnect...
          </Alert>
        )}

        {anyDismissed && (
          <div className="text-center p-6 border border-dashed border-neutral-300 rounded-lg">
            <p className="text-neutral-600 mb-4">
              Some alerts have been dismissed
            </p>
            <button
              onClick={resetAlerts}
              className="px-4 py-2 bg-trust-deep text-white rounded-md hover:bg-trust-hover transition-colors"
            >
              Show All Alerts
            </button>
          </div>
        )}
      </div>
    );
  },
  args: {
    variant: 'info',
    dismissible: true,
    children: 'This alert can be dismissed',
  },
};

/**
 * Full-width banner style alerts
 */
export const Banner: Story = {
  render: () => (
    <div className="space-y-0 -mx-4">
      <Alert
        variant="info"
        banner
        action={{ label: 'Learn More', onClick: () => alert('Learn more!') }}
      >
        New consent management features are now available. Learn about enhanced
        privacy controls and compliance tools.
      </Alert>

      <div className="p-4">
        <p className="text-neutral-700 mb-4">
          Banner alerts are typically used at the top of a page or section to
          display important system-wide messages.
        </p>
      </div>

      <Alert
        variant="warning"
        banner
        dismissible
        onDismiss={() => alert('Banner dismissed')}
      >
        Scheduled maintenance on Saturday, Dec 21 from 2:00 AM - 4:00 AM UTC.
        Services may be temporarily unavailable.
      </Alert>

      <div className="p-4">
        <p className="text-neutral-700">
          They span the full width of their container and have no rounded
          corners on the sides.
        </p>
      </div>

      <Alert
        variant="error"
        banner
        title="System Alert"
        action={{ label: 'View Status', onClick: () => alert('View status') }}
      >
        We're experiencing higher than normal response times. Our team is
        working to resolve this issue.
      </Alert>
    </div>
  ),
  args: {
    variant: 'info',
    banner: true,
    children: 'This is a full-width banner alert',
  },
};

/**
 * Interactive playground with all controls
 */
export const Playground: Story = {
  render: (args) => {
    const [isVisible, setIsVisible] = useState(true);

    if (!isVisible) {
      return (
        <div className="text-center p-8 border border-dashed border-neutral-300 rounded-lg">
          <p className="text-neutral-600 mb-4">Alert dismissed</p>
          <button
            onClick={() => setIsVisible(true)}
            className="px-4 py-2 bg-trust-deep text-white rounded-md hover:bg-trust-hover transition-colors"
          >
            Show Alert
          </button>
        </div>
      );
    }

    return (
      <Alert
        {...args}
        onDismiss={() => {
          setIsVisible(false);
          args.onDismiss?.();
        }}
      />
    );
  },
  args: {
    variant: 'info',
    title: 'Alert Title',
    children:
      'This is the alert message content. You can customize all properties using the controls below.',
    dismissible: true,
    banner: false,
    hideIcon: false,
    action: undefined,
  },
};

/**
 * Real-world use case examples
 */
export const RealWorldExamples: Story = {
  render: () => {
    const [consents, setConsents] = useState({
      pending: true,
      granted: false,
      expired: true,
    });

    return (
      <div className="space-y-6 max-w-3xl">
        <div>
          <h3 className="text-lg font-medium text-neutral-900 mb-4">
            Consent Management Scenarios
          </h3>

          <div className="space-y-4">
            {consents.pending && (
              <Alert
                variant="warning"
                title="Consent Request Pending"
                dismissible
                onDismiss={() => setConsents({ ...consents, pending: false })}
                action={{
                  label: 'Review Request',
                  onClick: () => alert('Opening consent request...'),
                }}
              >
                A third-party application is requesting access to your profile
                data. Please review and respond to this consent request.
              </Alert>
            )}

            {consents.granted && (
              <Alert variant="success" title="Access Granted">
                You've successfully granted "Analytics Dashboard" access to your
                usage statistics. This permission is valid for 30 days.
              </Alert>
            )}

            {consents.expired && (
              <Alert
                variant="info"
                title="Consent Expired"
                dismissible
                onDismiss={() => setConsents({ ...consents, expired: false })}
                action={{
                  label: 'Renew',
                  onClick: () => {
                    setConsents({ ...consents, expired: false, granted: true });
                    alert('Consent renewed!');
                  },
                }}
              >
                Your consent for "Marketing Platform" has expired. Renew to
                continue sharing data with this service.
              </Alert>
            )}

            <Alert
              variant="error"
              title="Security Alert"
              action={{
                label: 'Secure Account',
                onClick: () => alert('Opening security settings...'),
              }}
            >
              We detected unusual activity on your account. Please review your
              recent consent grants and revoke any suspicious permissions.
            </Alert>
          </div>
        </div>

        <div className="pt-4 border-t border-neutral-200">
          <button
            onClick={() =>
              setConsents({ pending: true, granted: false, expired: true })
            }
            className="text-sm text-trust-deep hover:text-trust-hover underline"
          >
            Reset Examples
          </button>
        </div>
      </div>
    );
  },
  args: {
    variant: 'info',
    children: 'Real-world example',
  },
};

/**
 * Alerts without icons
 */
export const WithoutIcons: Story = {
  render: () => (
    <div className="space-y-4 max-w-2xl">
      <Alert variant="info" hideIcon title="Plain Information">
        Sometimes you may want to display alerts without icons for a cleaner
        look or when the icon doesn't add meaningful context.
      </Alert>

      <Alert variant="success" hideIcon>
        Your settings have been saved.
      </Alert>

      <Alert variant="warning" hideIcon>
        This feature is currently in beta.
      </Alert>
    </div>
  ),
  args: {
    variant: 'info',
    hideIcon: true,
    children: 'Alert without icon',
  },
};

/**
 * Custom icons (replacing defaults)
 */
export const CustomIcons: Story = {
  render: () => {
    const DocumentIcon = () => (
      <svg
        className="w-5 h-5"
        fill="none"
        viewBox="0 0 24 24"
        stroke="currentColor"
      >
        <path
          strokeLinecap="round"
          strokeLinejoin="round"
          strokeWidth={2}
          d="M9 12h6m-6 4h6m2 5H7a2 2 0 01-2-2V5a2 2 0 012-2h5.586a1 1 0 01.707.293l5.414 5.414a1 1 0 01.293.707V19a2 2 0 01-2 2z"
        />
      </svg>
    );

    const BellIcon = () => (
      <svg
        className="w-5 h-5"
        fill="none"
        viewBox="0 0 24 24"
        stroke="currentColor"
      >
        <path
          strokeLinecap="round"
          strokeLinejoin="round"
          strokeWidth={2}
          d="M15 17h5l-1.405-1.405A2.032 2.032 0 0118 14.158V11a6.002 6.002 0 00-4-5.659V5a2 2 0 10-4 0v.341C7.67 6.165 6 8.388 6 11v3.159c0 .538-.214 1.055-.595 1.436L4 17h5m6 0v1a3 3 0 11-6 0v-1m6 0H9"
        />
      </svg>
    );

    return (
      <div className="space-y-4 max-w-2xl">
        <Alert variant="info" icon={<DocumentIcon />} title="New Document">
          You can override the default icon with any custom React component or
          SVG.
        </Alert>

        <Alert
          variant="success"
          icon={<BellIcon />}
          title="Notification Settings Updated"
        >
          You will now receive email notifications for important account
          activities.
        </Alert>
      </div>
    );
  },
  args: {
    variant: 'info',
    icon: undefined,
    children: 'Alert with custom icon',
  },
};

/**
 * Accessibility features demonstration
 */
export const Accessibility: Story = {
  render: () => (
    <div className="space-y-6 max-w-3xl">
      <div className="p-4 bg-neutral-50 border border-neutral-200 rounded-lg">
        <h4 className="text-sm font-medium text-neutral-900 mb-2">
          Accessibility Features
        </h4>
        <ul className="text-sm text-neutral-700 space-y-1">
          <li>
            • Error alerts use <code>role="alert"</code> and{' '}
            <code>aria-live="assertive"</code>
          </li>
          <li>
            • Other variants use <code>role="status"</code> and{' '}
            <code>aria-live="polite"</code>
          </li>
          <li>
            • Dismiss buttons have <code>aria-label="Dismiss alert"</code>
          </li>
          <li>
            • Icons are marked with <code>aria-hidden="true"</code>
          </li>
          <li>• All interactive elements are keyboard accessible</li>
          <li>• Focus indicators meet WCAG 2.1 AA requirements</li>
          <li>• Color contrast ratios comply with AA standards</li>
        </ul>
      </div>

      <Alert variant="error" title="Critical Error" dismissible>
        This error alert will be announced immediately to screen readers due to
        its assertive aria-live setting.
      </Alert>

      <Alert
        variant="success"
        dismissible
        action={{
          label: 'Undo',
          onClick: () => alert('Undo action'),
        }}
      >
        Changes saved. Use Tab to navigate between action and dismiss buttons.
      </Alert>
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
        ],
      },
    },
  },
  args: {
    variant: 'info',
    children: 'Accessible alert',
  },
};

/**
 * Complex content with multiple paragraphs
 */
export const ComplexContent: Story = {
  render: () => (
    <div className="space-y-4 max-w-3xl">
      <Alert variant="info" title="Terms of Service Update">
        <div className="space-y-2">
          <p>
            We've updated our Terms of Service to provide better clarity on data
            usage and your privacy rights.
          </p>
          <p>
            Key changes include enhanced data portability options, clearer
            consent management workflows, and updated retention policies.
          </p>
          <p className="font-medium">
            Please review the changes by January 1, 2026 to continue using our
            services.
          </p>
        </div>
      </Alert>

      <Alert
        variant="warning"
        title="Data Export Ready"
        action={{
          label: 'Download',
          onClick: () => alert('Downloading...'),
        }}
      >
        <div className="space-y-2">
          <p>Your requested data export is ready for download.</p>
          <ul className="list-disc list-inside text-sm space-y-1">
            <li>Profile information and settings</li>
            <li>Consent history and audit logs</li>
            <li>Connected applications and permissions</li>
          </ul>
          <p className="text-xs mt-2">
            Download link expires in 7 days (December 26, 2025)
          </p>
        </div>
      </Alert>
    </div>
  ),
  args: {
    variant: 'info',
    title: 'Complex Content',
    children: 'Alert with structured content',
  },
};
