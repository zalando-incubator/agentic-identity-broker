/**
 * Toast Component Stories
 */

import type { Meta, StoryObj } from '@storybook/react';
import { useState } from 'react';
import { Toast } from './Toast';

const meta = {
  title: 'Design System/Feedback/Toast',
  component: Toast,
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
    duration: {
      control: 'number',
      description:
        'Auto-dismiss duration in milliseconds (0 = no auto-dismiss)',
    },
    position: {
      control: 'select',
      options: ['top-right', 'top-left', 'bottom-right', 'bottom-left'],
      description: 'Toast position on screen',
    },
    hideIcon: {
      control: 'boolean',
      description: 'Hide the default icon',
    },
    children: {
      control: 'text',
      description: 'Toast message content',
    },
  },
} satisfies Meta<typeof Toast>;

export default meta;
type Story = StoryObj<typeof meta>;

/**
 * Default toast with info variant
 */
export const Default: Story = {
  args: {
    variant: 'info',
    duration: 0, // Disable auto-dismiss for story
    children:
      'This is an informational notification to provide quick feedback.',
  },
};

/**
 * All toast variants displayed together
 */
export const Variants: Story = {
  render: () => {
    const [toasts, setToasts] = useState({
      info: true,
      success: true,
      warning: true,
      error: true,
    });

    return (
      <div className="space-y-4 max-w-2xl">
        {toasts.info && (
          <Toast
            variant="info"
            duration={0}
            onDismiss={() => setToasts({ ...toasts, info: false })}
          >
            This is an informational toast. It provides helpful context or
            additional information.
          </Toast>
        )}

        {toasts.success && (
          <Toast
            variant="success"
            duration={0}
            onDismiss={() => setToasts({ ...toasts, success: false })}
          >
            Success! Your changes have been saved and will take effect
            immediately.
          </Toast>
        )}

        {toasts.warning && (
          <Toast
            variant="warning"
            duration={0}
            onDismiss={() => setToasts({ ...toasts, warning: false })}
          >
            Warning: This action cannot be undone. Please review your selection
            carefully.
          </Toast>
        )}

        {toasts.error && (
          <Toast
            variant="error"
            duration={0}
            onDismiss={() => setToasts({ ...toasts, error: false })}
          >
            Error: Unable to process your request. Please check your connection
            and try again.
          </Toast>
        )}

        {!toasts.info &&
          !toasts.success &&
          !toasts.warning &&
          !toasts.error && (
            <div className="text-center p-8 border border-dashed border-neutral-300 rounded-lg">
              <p className="text-neutral-600 mb-4">All toasts dismissed</p>
              <button
                onClick={() =>
                  setToasts({
                    info: true,
                    success: true,
                    warning: true,
                    error: true,
                  })
                }
                className="px-4 py-2 bg-trust-deep text-white rounded-md hover:bg-trust-hover transition-colors"
              >
                Show All Toasts
              </button>
            </div>
          )}
      </div>
    );
  },
  args: {
    variant: 'info',
    children: 'Toast content',
  },
};

/**
 * Toasts with leading icons (default behavior)
 */
export const WithIcons: Story = {
  render: () => {
    const [toasts, setToasts] = useState({
      info: true,
      success: true,
      warning: true,
      error: true,
    });

    return (
      <div className="space-y-4 max-w-2xl">
        {toasts.info && (
          <Toast
            variant="info"
            duration={0}
            onDismiss={() => setToasts({ ...toasts, info: false })}
          >
            Each toast variant comes with a default icon that matches its
            semantic meaning.
          </Toast>
        )}

        {toasts.success && (
          <Toast
            variant="success"
            duration={0}
            onDismiss={() => setToasts({ ...toasts, success: false })}
          >
            The success icon indicates a positive outcome or completed action.
          </Toast>
        )}

        {toasts.warning && (
          <Toast
            variant="warning"
            duration={0}
            onDismiss={() => setToasts({ ...toasts, warning: false })}
          >
            The warning icon draws attention to important caution messages.
          </Toast>
        )}

        {toasts.error && (
          <Toast
            variant="error"
            duration={0}
            onDismiss={() => setToasts({ ...toasts, error: false })}
          >
            The error icon clearly indicates a problem that needs attention.
          </Toast>
        )}
      </div>
    );
  },
  args: {
    variant: 'info',
    children: 'Toast with icon',
  },
};

/**
 * Toasts with action buttons
 */
export const WithActions: Story = {
  render: () => {
    const [toasts, setToasts] = useState({
      action1: true,
      action2: true,
      action3: true,
    });

    return (
      <div className="space-y-4 max-w-2xl">
        {toasts.action1 && (
          <Toast
            variant="warning"
            duration={0}
            onDismiss={() => setToasts({ ...toasts, action1: false })}
            action={{
              label: 'Review',
              onClick: () => alert('Action clicked!'),
            }}
          >
            Your subscription will expire in 3 days. Review your plan to
            continue using all features.
          </Toast>
        )}

        {toasts.action2 && (
          <Toast
            variant="error"
            duration={0}
            onDismiss={() => setToasts({ ...toasts, action2: false })}
            action={{
              label: 'Retry',
              onClick: () => alert('Retrying...'),
            }}
          >
            Payment processing failed. Please verify your payment method and try
            again.
          </Toast>
        )}

        {toasts.action3 && (
          <Toast
            variant="success"
            duration={0}
            onDismiss={() => setToasts({ ...toasts, action3: false })}
            action={{
              label: 'View',
              onClick: () => alert('Opening...'),
            }}
          >
            New features are available! Check out our latest updates.
          </Toast>
        )}

        {!toasts.action1 && !toasts.action2 && !toasts.action3 && (
          <div className="text-center p-8 border border-dashed border-neutral-300 rounded-lg">
            <p className="text-neutral-600 mb-4">All toasts dismissed</p>
            <button
              onClick={() =>
                setToasts({ action1: true, action2: true, action3: true })
              }
              className="px-4 py-2 bg-trust-deep text-white rounded-md hover:bg-trust-hover transition-colors"
            >
              Show All Toasts
            </button>
          </div>
        )}
      </div>
    );
  },
  args: {
    variant: 'info',
    children: 'Toast with actions',
  },
};

/**
 * Toasts with auto-dismiss timer (5 seconds)
 */
export const WithTimer: Story = {
  render: () => {
    const [toasts, setToasts] = useState({
      toast1: false,
      toast2: false,
      toast3: false,
    });

    return (
      <div className="space-y-4 max-w-2xl">
        <div className="p-6 border border-dashed border-neutral-300 rounded-lg">
          <h4 className="text-sm font-medium text-neutral-900 mb-3">
            Auto-Dismiss Demo
          </h4>
          <p className="text-sm text-neutral-600 mb-4">
            Click the buttons below to show toasts that auto-dismiss after 5
            seconds. You can also manually dismiss them before the timer ends.
          </p>
          <div className="flex gap-2 flex-wrap">
            <button
              onClick={() => setToasts({ ...toasts, toast1: true })}
              className="px-4 py-2 bg-info-primary text-white rounded-md hover:bg-info-dark transition-colors"
            >
              Show Info Toast
            </button>
            <button
              onClick={() => setToasts({ ...toasts, toast2: true })}
              className="px-4 py-2 bg-success-primary text-white rounded-md hover:bg-success-dark transition-colors"
            >
              Show Success Toast
            </button>
            <button
              onClick={() => setToasts({ ...toasts, toast3: true })}
              className="px-4 py-2 bg-warning-primary text-white rounded-md hover:bg-warning-dark transition-colors"
            >
              Show Warning Toast
            </button>
          </div>
        </div>

        {toasts.toast1 && (
          <Toast
            variant="info"
            duration={5000}
            onDismiss={() => setToasts({ ...toasts, toast1: false })}
          >
            This toast will automatically dismiss after 5 seconds.
          </Toast>
        )}

        {toasts.toast2 && (
          <Toast
            variant="success"
            duration={5000}
            onDismiss={() => setToasts({ ...toasts, toast2: false })}
          >
            Changes saved! This notification will disappear in 5 seconds.
          </Toast>
        )}

        {toasts.toast3 && (
          <Toast
            variant="warning"
            title="Auto-Dismiss"
            duration={5000}
            onDismiss={() => setToasts({ ...toasts, toast3: false })}
          >
            You can still close this manually before the timer ends.
          </Toast>
        )}
      </div>
    );
  },
  args: {
    variant: 'info',
    duration: 5000,
    children: 'This toast will auto-dismiss',
  },
};

/**
 * Multiple toasts stacked together
 */
export const Stacked: Story = {
  render: () => {
    const [nextId, setNextId] = useState(4);
    const [toasts, setToasts] = useState([
      {
        id: 1,
        variant: 'info' as const,
        message: 'First notification in the stack',
      },
      {
        id: 2,
        variant: 'success' as const,
        message: 'Successfully saved your changes',
      },
      {
        id: 3,
        variant: 'warning' as const,
        message: 'Connection is unstable',
      },
    ]);

    const addToast = (variant: 'info' | 'success' | 'warning' | 'error') => {
      const messages = {
        info: 'New information available',
        success: 'Operation completed successfully',
        warning: 'Please review this warning',
        error: 'An error occurred',
      };

      setToasts([
        ...toasts,
        { id: nextId, variant, message: messages[variant] },
      ]);
      setNextId(nextId + 1);
    };

    const removeToast = (id: number) => {
      setToasts(toasts.filter((t) => t.id !== id));
    };

    return (
      <div className="space-y-4">
        <div className="p-6 border border-dashed border-neutral-300 rounded-lg max-w-2xl">
          <h4 className="text-sm font-medium text-neutral-900 mb-3">
            Toast Stack Manager
          </h4>
          <p className="text-sm text-neutral-600 mb-4">
            Add multiple toasts to see how they stack. Toasts appear at the top
            of the stack and stack downward.
          </p>
          <div className="flex gap-2 flex-wrap">
            <button
              onClick={() => addToast('info')}
              className="px-3 py-2 text-sm bg-info-primary text-white rounded-md hover:bg-info-dark transition-colors"
            >
              Add Info
            </button>
            <button
              onClick={() => addToast('success')}
              className="px-3 py-2 text-sm bg-success-primary text-white rounded-md hover:bg-success-dark transition-colors"
            >
              Add Success
            </button>
            <button
              onClick={() => addToast('warning')}
              className="px-3 py-2 text-sm bg-warning-primary text-white rounded-md hover:bg-warning-dark transition-colors"
            >
              Add Warning
            </button>
            <button
              onClick={() => addToast('error')}
              className="px-3 py-2 text-sm bg-error-primary text-white rounded-md hover:bg-error-dark transition-colors"
            >
              Add Error
            </button>
            <button
              onClick={() => setToasts([])}
              className="px-3 py-2 text-sm bg-neutral-600 text-white rounded-md hover:bg-neutral-700 transition-colors"
            >
              Clear All
            </button>
          </div>
          <p className="text-xs text-neutral-500 mt-3">
            Active toasts: {toasts.length}
          </p>
        </div>

        <div className="space-y-2 max-w-2xl">
          {toasts.map((toast) => (
            <Toast
              key={toast.id}
              variant={toast.variant}
              duration={0}
              position={undefined as any} // Remove fixed positioning for stacking demo
              onDismiss={() => removeToast(toast.id)}
            >
              {toast.message}
            </Toast>
          ))}
        </div>
      </div>
    );
  },
  args: {
    variant: 'info',
    children: 'Stacked toast',
  },
};

/**
 * Toast positioning variations
 */
export const Positions: Story = {
  render: () => {
    const [activePosition, setActivePosition] = useState<
      'top-right' | 'top-left' | 'bottom-right' | 'bottom-left' | null
    >(null);

    return (
      <div className="h-96 relative border border-dashed border-neutral-300 rounded-lg">
        <div className="absolute inset-0 flex items-center justify-center">
          <div className="text-center">
            <h4 className="text-sm font-medium text-neutral-900 mb-3">
              Choose a Position
            </h4>
            <p className="text-sm text-neutral-600 mb-4 max-w-md">
              Click a button to show a toast in that corner. Toasts will appear
              with a slide-in animation from their respective edge.
            </p>
            <div className="grid grid-cols-2 gap-3">
              <button
                onClick={() => setActivePosition('top-left')}
                className="px-4 py-2 bg-trust-deep text-white rounded-md hover:bg-trust-hover transition-colors"
              >
                Top Left
              </button>
              <button
                onClick={() => setActivePosition('top-right')}
                className="px-4 py-2 bg-trust-deep text-white rounded-md hover:bg-trust-hover transition-colors"
              >
                Top Right
              </button>
              <button
                onClick={() => setActivePosition('bottom-left')}
                className="px-4 py-2 bg-trust-deep text-white rounded-md hover:bg-trust-hover transition-colors"
              >
                Bottom Left
              </button>
              <button
                onClick={() => setActivePosition('bottom-right')}
                className="px-4 py-2 bg-trust-deep text-white rounded-md hover:bg-trust-hover transition-colors"
              >
                Bottom Right
              </button>
            </div>
          </div>
        </div>

        {activePosition && (
          <Toast
            variant="success"
            position={activePosition}
            duration={0}
            onDismiss={() => setActivePosition(null)}
            title={`Toast at ${activePosition}`}
          >
            This toast appears in the {activePosition.replace('-', ' ')} corner
            of the screen.
          </Toast>
        )}
      </div>
    );
  },
  args: {
    variant: 'info',
    position: 'top-right',
    children: 'Positioned toast',
  },
};

/**
 * Custom duration timing examples
 */
export const CustomDuration: Story = {
  render: () => {
    const [activeToast, setActiveToast] = useState<string | null>(null);

    const showToast = (duration: number, label: string) => {
      setActiveToast(label);
      // Auto-reset for demo purposes
      setTimeout(() => setActiveToast(null), duration);
    };

    return (
      <div className="space-y-4 max-w-2xl">
        <div className="p-6 border border-dashed border-neutral-300 rounded-lg">
          <h4 className="text-sm font-medium text-neutral-900 mb-3">
            Duration Options
          </h4>
          <p className="text-sm text-neutral-600 mb-4">
            Configure how long toasts remain visible before auto-dismissing.
            Choose from quick (3s), standard (5s), or extended (10s) durations.
          </p>
          <div className="flex gap-2 flex-wrap">
            <button
              onClick={() => showToast(3000, '3s')}
              disabled={activeToast === '3s'}
              className="px-4 py-2 bg-trust-deep text-white rounded-md hover:bg-trust-hover transition-colors disabled:opacity-50 disabled:cursor-not-allowed"
            >
              Quick (3s)
            </button>
            <button
              onClick={() => showToast(5000, '5s')}
              disabled={activeToast === '5s'}
              className="px-4 py-2 bg-trust-deep text-white rounded-md hover:bg-trust-hover transition-colors disabled:opacity-50 disabled:cursor-not-allowed"
            >
              Standard (5s)
            </button>
            <button
              onClick={() => showToast(10000, '10s')}
              disabled={activeToast === '10s'}
              className="px-4 py-2 bg-trust-deep text-white rounded-md hover:bg-trust-hover transition-colors disabled:opacity-50 disabled:cursor-not-allowed"
            >
              Extended (10s)
            </button>
            <button
              onClick={() => {
                setActiveToast('persistent');
              }}
              disabled={activeToast === 'persistent'}
              className="px-4 py-2 bg-neutral-600 text-white rounded-md hover:bg-neutral-700 transition-colors disabled:opacity-50 disabled:cursor-not-allowed"
            >
              No Auto-Dismiss
            </button>
          </div>
        </div>

        {activeToast === '3s' && (
          <Toast
            variant="info"
            duration={3000}
            onDismiss={() => setActiveToast(null)}
          >
            Quick toast - dismisses after 3 seconds
          </Toast>
        )}

        {activeToast === '5s' && (
          <Toast
            variant="success"
            duration={5000}
            onDismiss={() => setActiveToast(null)}
          >
            Standard toast - dismisses after 5 seconds
          </Toast>
        )}

        {activeToast === '10s' && (
          <Toast
            variant="warning"
            duration={10000}
            onDismiss={() => setActiveToast(null)}
          >
            Extended toast - dismisses after 10 seconds
          </Toast>
        )}

        {activeToast === 'persistent' && (
          <Toast
            variant="error"
            duration={0}
            onDismiss={() => setActiveToast(null)}
            title="Persistent Toast"
          >
            This toast requires manual dismissal - no auto-dismiss timer
          </Toast>
        )}
      </div>
    );
  },
  args: {
    variant: 'info',
    duration: 5000,
    children: 'Toast with custom duration',
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
          <p className="text-neutral-600 mb-4">Toast dismissed</p>
          <button
            onClick={() => setIsVisible(true)}
            className="px-4 py-2 bg-trust-deep text-white rounded-md hover:bg-trust-hover transition-colors"
          >
            Show Toast
          </button>
        </div>
      );
    }

    return (
      <Toast
        {...args}
        position={undefined as any} // Remove fixed positioning for playground
        onDismiss={() => {
          setIsVisible(false);
          args.onDismiss?.();
        }}
      />
    );
  },
  args: {
    variant: 'info',
    title: 'Toast Title',
    children:
      'This is the toast message content. You can customize all properties using the controls below.',
    duration: 0,
    hideIcon: false,
    action: undefined,
  },
};

/**
 * Toasts with titles
 */
export const WithTitle: Story = {
  render: () => {
    const [toasts, setToasts] = useState({
      toast1: true,
      toast2: true,
      toast3: true,
      toast4: true,
    });

    return (
      <div className="space-y-4 max-w-2xl">
        {toasts.toast1 && (
          <Toast
            variant="info"
            title="Information"
            duration={0}
            onDismiss={() => setToasts({ ...toasts, toast1: false })}
          >
            Titles help organize complex toasts and provide clear context for
            the message.
          </Toast>
        )}

        {toasts.toast2 && (
          <Toast
            variant="success"
            title="Access Granted"
            duration={0}
            onDismiss={() => setToasts({ ...toasts, toast2: false })}
          >
            You now have editor permissions for the "Marketing Assets"
            workspace.
          </Toast>
        )}

        {toasts.toast3 && (
          <Toast
            variant="warning"
            title="Action Required"
            duration={0}
            onDismiss={() => setToasts({ ...toasts, toast3: false })}
          >
            Your account requires two-factor authentication within 7 days.
          </Toast>
        )}

        {toasts.toast4 && (
          <Toast
            variant="error"
            title="Authentication Failed"
            duration={0}
            onDismiss={() => setToasts({ ...toasts, toast4: false })}
          >
            Your session may have expired. Please sign in again.
          </Toast>
        )}

        {!toasts.toast1 &&
          !toasts.toast2 &&
          !toasts.toast3 &&
          !toasts.toast4 && (
            <div className="text-center p-8 border border-dashed border-neutral-300 rounded-lg">
              <p className="text-neutral-600 mb-4">All toasts dismissed</p>
              <button
                onClick={() =>
                  setToasts({
                    toast1: true,
                    toast2: true,
                    toast3: true,
                    toast4: true,
                  })
                }
                className="px-4 py-2 bg-trust-deep text-white rounded-md hover:bg-trust-hover transition-colors"
              >
                Show All Toasts
              </button>
            </div>
          )}
      </div>
    );
  },
  args: {
    variant: 'info',
    title: 'Toast Title',
    children: 'Toast description goes here',
  },
};

/**
 * Real-world use case examples
 */
export const RealWorldExamples: Story = {
  render: () => {
    const [notifications, setNotifications] = useState<
      Array<{ id: number; type: string }>
    >([]);
    const [nextId, setNextId] = useState(1);

    const showNotification = (type: string) => {
      setNotifications([...notifications, { id: nextId, type }]);
      setNextId(nextId + 1);
    };

    const removeNotification = (id: number) => {
      setNotifications(notifications.filter((n) => n.id !== id));
    };

    return (
      <div className="space-y-6 max-w-3xl">
        <div>
          <h3 className="text-lg font-medium text-neutral-900 mb-4">
            Consent Management Scenarios
          </h3>

          <div className="p-6 border border-dashed border-neutral-300 rounded-lg mb-4">
            <p className="text-sm text-neutral-600 mb-4">
              Simulate common consent management notifications:
            </p>
            <div className="flex gap-2 flex-wrap">
              <button
                onClick={() => showNotification('granted')}
                className="px-3 py-2 text-sm bg-success-primary text-white rounded-md hover:bg-success-dark transition-colors"
              >
                Grant Consent
              </button>
              <button
                onClick={() => showNotification('revoked')}
                className="px-3 py-2 text-sm bg-warning-primary text-white rounded-md hover:bg-warning-dark transition-colors"
              >
                Revoke Consent
              </button>
              <button
                onClick={() => showNotification('expired')}
                className="px-3 py-2 text-sm bg-info-primary text-white rounded-md hover:bg-info-dark transition-colors"
              >
                Consent Expired
              </button>
              <button
                onClick={() => showNotification('error')}
                className="px-3 py-2 text-sm bg-error-primary text-white rounded-md hover:bg-error-dark transition-colors"
              >
                Security Alert
              </button>
            </div>
          </div>

          <div className="space-y-2">
            {notifications.map((notification) => {
              if (notification.type === 'granted') {
                return (
                  <Toast
                    key={notification.id}
                    variant="success"
                    title="Consent Granted"
                    duration={5000}
                    position={undefined as any}
                    onDismiss={() => removeNotification(notification.id)}
                  >
                    You've successfully granted access to your profile data.
                    This permission is valid for 30 days.
                  </Toast>
                );
              }

              if (notification.type === 'revoked') {
                return (
                  <Toast
                    key={notification.id}
                    variant="warning"
                    title="Consent Revoked"
                    duration={5000}
                    position={undefined as any}
                    onDismiss={() => removeNotification(notification.id)}
                  >
                    Access permissions have been revoked. The application will
                    no longer have access to your data.
                  </Toast>
                );
              }

              if (notification.type === 'expired') {
                return (
                  <Toast
                    key={notification.id}
                    variant="info"
                    title="Consent Expired"
                    duration={0}
                    position={undefined as any}
                    onDismiss={() => removeNotification(notification.id)}
                    action={{
                      label: 'Renew',
                      onClick: () => {
                        alert('Renewing consent...');
                        removeNotification(notification.id);
                      },
                    }}
                  >
                    Your consent for "Marketing Platform" has expired. Renew to
                    continue sharing data.
                  </Toast>
                );
              }

              if (notification.type === 'error') {
                return (
                  <Toast
                    key={notification.id}
                    variant="error"
                    title="Security Alert"
                    duration={0}
                    position={undefined as any}
                    onDismiss={() => removeNotification(notification.id)}
                    action={{
                      label: 'Review',
                      onClick: () => alert('Opening security settings...'),
                    }}
                  >
                    Unusual activity detected. Please review your recent consent
                    grants.
                  </Toast>
                );
              }

              return null;
            })}
          </div>
        </div>
      </div>
    );
  },
  args: {
    variant: 'info',
    children: 'Real-world example',
  },
};
