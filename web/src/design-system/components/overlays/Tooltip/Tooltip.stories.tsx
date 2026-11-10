/**
 * Tooltip Component Stories
 */

import type { Meta, StoryObj } from '@storybook/react';
import { useState } from 'react';
import { Tooltip } from './Tooltip';
import { StatusIndicator } from '../../data-display/StatusIndicator';
import { Switch } from '../../inputs/Switch';

const meta = {
  title: 'Design System/Overlays/Tooltip',
  component: Tooltip,
  parameters: {
    layout: 'centered',
  },
  tags: ['autodocs'],
  argTypes: {
    content: {
      control: 'text',
      description: 'Content to display in the tooltip',
    },
    position: {
      control: 'select',
      options: ['top', 'right', 'bottom', 'left'],
      description: 'Position of tooltip relative to trigger',
    },
    theme: {
      control: 'select',
      options: ['dark', 'light'],
      description: 'Visual theme variant',
    },
    showArrow: {
      control: 'boolean',
      description: 'Whether to show arrow indicator',
    },
    delay: {
      control: 'number',
      description: 'Delay in milliseconds before showing tooltip',
    },
    disabled: {
      control: 'boolean',
      description: 'Whether tooltip is disabled',
    },
  },
} satisfies Meta<typeof Tooltip>;

export default meta;
type Story = StoryObj<typeof meta>;

/**
 * Default tooltip with dark theme and top position
 */
export const Default: Story = {
  args: {
    content: 'This is a helpful tooltip',
    position: 'top',
    theme: 'dark',
    showArrow: true,
    delay: 200,
    children: (
      <button className="px-4 py-2 bg-trust-deep text-white rounded-md hover:bg-trust-hover transition-colors">
        Hover me
      </button>
    ),
  },
};

/**
 * All position variants displayed together
 */
export const Positions: Story = {
  render: () => (
    <div className="flex gap-12 items-center justify-center p-24">
      <Tooltip content="Top tooltip" position="top">
        <button className="px-4 py-2 bg-trust-hover text-white rounded-md hover:bg-trust transition-colors">
          Top
        </button>
      </Tooltip>

      <Tooltip content="Right tooltip" position="right">
        <button className="px-4 py-2 bg-trust-hover text-white rounded-md hover:bg-trust transition-colors">
          Right
        </button>
      </Tooltip>

      <Tooltip content="Bottom tooltip" position="bottom">
        <button className="px-4 py-2 bg-trust-hover text-white rounded-md hover:bg-trust transition-colors">
          Bottom
        </button>
      </Tooltip>

      <Tooltip content="Left tooltip" position="left">
        <button className="px-4 py-2 bg-trust-hover text-white rounded-md hover:bg-trust transition-colors">
          Left
        </button>
      </Tooltip>
    </div>
  ),
  args: {
    content: 'Tooltip content',
    position: 'top',
    children: <button>Trigger</button>,
  },
};

/**
 * Dark and light theme variants
 */
export const Themes: Story = {
  render: () => (
    <div className="flex gap-12 items-center justify-center p-12">
      <Tooltip content="Dark theme tooltip (default)" theme="dark">
        <button className="px-4 py-2 bg-trust-hover text-white rounded-md hover:bg-trust transition-colors">
          Dark Theme
        </button>
      </Tooltip>

      <Tooltip content="Light theme tooltip with border" theme="light">
        <button className="px-4 py-2 bg-white border border-neutral-300 text-neutral-900 rounded-md hover:bg-neutral-50 transition-colors">
          Light Theme
        </button>
      </Tooltip>
    </div>
  ),
  args: {
    content: 'Tooltip content',
    theme: 'dark',
    children: <button>Trigger</button>,
  },
};

/**
 * Tooltips with and without arrow indicators
 */
export const WithArrow: Story = {
  render: () => (
    <div className="flex gap-12 items-center justify-center p-12">
      <Tooltip content="Tooltip with arrow" showArrow={true}>
        <button className="px-4 py-2 bg-trust-hover text-white rounded-md hover:bg-trust transition-colors">
          With Arrow
        </button>
      </Tooltip>

      <Tooltip content="Tooltip without arrow" showArrow={false}>
        <button className="px-4 py-2 bg-trust-hover text-white rounded-md hover:bg-trust transition-colors">
          Without Arrow
        </button>
      </Tooltip>
    </div>
  ),
  args: {
    content: 'Tooltip content',
    showArrow: true,
    children: <button>Trigger</button>,
  },
};

/**
 * Tooltips with different hover delays
 */
export const WithDelay: Story = {
  render: () => (
    <div className="flex gap-8 items-center justify-center p-12">
      <Tooltip content="No delay (instant)" delay={0}>
        <button className="px-4 py-2 bg-trust-hover text-white rounded-md hover:bg-trust transition-colors">
          0ms delay
        </button>
      </Tooltip>

      <Tooltip content="Default delay" delay={200}>
        <button className="px-4 py-2 bg-trust-hover text-white rounded-md hover:bg-trust transition-colors">
          200ms delay
        </button>
      </Tooltip>

      <Tooltip content="Long delay" delay={500}>
        <button className="px-4 py-2 bg-trust-hover text-white rounded-md hover:bg-trust transition-colors">
          500ms delay
        </button>
      </Tooltip>

      <Tooltip content="Very long delay" delay={1000}>
        <button className="px-4 py-2 bg-trust-hover text-white rounded-md hover:bg-trust transition-colors">
          1000ms delay
        </button>
      </Tooltip>
    </div>
  ),
  args: {
    content: 'Tooltip content',
    delay: 200,
    children: <button>Trigger</button>,
  },
};

/**
 * Tooltip with longer multi-line content
 */
export const LongContent: Story = {
  render: () => (
    <div className="flex gap-8 items-center justify-center p-12">
      <Tooltip
        content={
          <div className="max-w-xs">
            <p className="mb-2 font-medium">Enhanced Privacy Controls</p>
            <p className="text-sm leading-relaxed">
              This feature allows you to manage consent preferences with
              granular control over data sharing and third-party access.
            </p>
          </div>
        }
        position="top"
      >
        <button className="px-4 py-2 bg-trust-hover text-white rounded-md hover:bg-trust transition-colors">
          Hover for details
        </button>
      </Tooltip>

      <Tooltip
        content={
          <div className="space-y-1 text-xs">
            <div className="font-medium mb-1">Keyboard Shortcuts:</div>
            <div>Ctrl + S - Save</div>
            <div>Ctrl + Z - Undo</div>
            <div>Ctrl + Y - Redo</div>
            <div>Esc - Close</div>
          </div>
        }
        position="bottom"
        theme="light"
      >
        <button className="px-4 py-2 bg-white border border-neutral-300 text-neutral-900 rounded-md hover:bg-neutral-50 transition-colors">
          Shortcuts
        </button>
      </Tooltip>
    </div>
  ),
  args: {
    content: 'Long content tooltip',
    children: <button>Trigger</button>,
  },
};

/**
 * Tooltip on icon trigger (common use case)
 */
export const IconTrigger: Story = {
  render: () => {
    const InfoIcon = () => (
      <svg
        className="w-5 h-5 text-neutral-500 hover:text-neutral-700 transition-colors cursor-help"
        fill="none"
        viewBox="0 0 24 24"
        stroke="currentColor"
      >
        <path
          strokeLinecap="round"
          strokeLinejoin="round"
          strokeWidth={2}
          d="M13 16h-1v-4h-1m1-4h.01M21 12a9 9 0 11-18 0 9 9 0 0118 0z"
        />
      </svg>
    );

    const HelpIcon = () => (
      <svg
        className="w-5 h-5 text-neutral-500 hover:text-neutral-700 transition-colors cursor-help"
        fill="none"
        viewBox="0 0 24 24"
        stroke="currentColor"
      >
        <path
          strokeLinecap="round"
          strokeLinejoin="round"
          strokeWidth={2}
          d="M8.228 9c.549-1.165 2.03-2 3.772-2 2.21 0 4 1.343 4 3 0 1.4-1.278 2.575-3.006 2.907-.542.104-.994.54-.994 1.093m0 3h.01M21 12a9 9 0 11-18 0 9 9 0 0118 0z"
        />
      </svg>
    );

    return (
      <div className="flex flex-col gap-8 items-start p-12 max-w-lg">
        <div className="flex items-center gap-2">
          <label className="text-sm font-medium text-neutral-900">
            Email Address
          </label>
          <Tooltip
            content="We'll never share your email with anyone else"
            position="right"
          >
            <InfoIcon />
          </Tooltip>
        </div>

        <div className="flex items-center gap-2">
          <label className="text-sm font-medium text-neutral-900">
            Data Retention Period
          </label>
          <Tooltip
            content="How long we keep your data before automatic deletion (30-90 days)"
            position="right"
            theme="light"
          >
            <HelpIcon />
          </Tooltip>
        </div>

        <div className="flex items-center gap-2">
          <label className="text-sm font-medium text-neutral-900">
            Third-Party Access
          </label>
          <Tooltip
            content={
              <div className="text-xs">
                <div className="font-medium mb-1">Authorized Services:</div>
                <div>• Analytics Dashboard</div>
                <div>• Marketing Platform</div>
                <div>• CRM Integration</div>
              </div>
            }
            position="right"
          >
            <InfoIcon />
          </Tooltip>
        </div>
      </div>
    );
  },
  args: {
    content: 'Helpful information',
    position: 'right',
    children: (
      <svg
        className="w-5 h-5 text-neutral-500"
        fill="none"
        viewBox="0 0 24 24"
        stroke="currentColor"
      >
        <path
          strokeLinecap="round"
          strokeLinejoin="round"
          strokeWidth={2}
          d="M13 16h-1v-4h-1m1-4h.01M21 12a9 9 0 11-18 0 9 9 0 0118 0z"
        />
      </svg>
    ),
  },
};

/**
 * Tooltip keyboard focus support for accessibility
 */
export const KeyboardFocus: Story = {
  render: () => (
    <div className="p-12 space-y-6 max-w-2xl">
      <div className="p-4 bg-neutral-50 border border-neutral-200 rounded-lg">
        <h4 className="text-sm font-medium text-neutral-900 mb-2">
          Keyboard Accessibility
        </h4>
        <p className="text-sm text-neutral-700 mb-3">
          Press{' '}
          <kbd className="px-2 py-1 bg-white border border-neutral-300 rounded text-xs font-mono">
            Tab
          </kbd>{' '}
          to navigate between elements. Tooltips appear on focus for keyboard
          users.
        </p>
      </div>

      <div className="flex flex-wrap gap-4">
        <Tooltip content="First tooltip - accessible via Tab" position="top">
          <button className="px-4 py-2 bg-trust-hover text-white rounded-md hover:bg-trust focus:ring-2 focus:ring-trust focus:ring-offset-2 transition-colors">
            Tab to focus #1
          </button>
        </Tooltip>

        <Tooltip content="Second tooltip - keyboard accessible" position="top">
          <button className="px-4 py-2 bg-trust-hover text-white rounded-md hover:bg-trust focus:ring-2 focus:ring-trust focus:ring-offset-2 transition-colors">
            Tab to focus #2
          </button>
        </Tooltip>

        <Tooltip
          content="Third tooltip with light theme"
          position="top"
          theme="light"
        >
          <button className="px-4 py-2 bg-white border border-neutral-300 text-neutral-900 rounded-md hover:bg-neutral-50 focus:ring-2 focus:ring-trust focus:ring-offset-2 transition-colors">
            Tab to focus #3
          </button>
        </Tooltip>

        <Tooltip content="Works with links too" position="bottom">
          <a
            href="#"
            className="inline-flex items-center px-4 py-2 text-trust hover:text-trust-deep underline focus:ring-2 focus:ring-trust focus:ring-offset-2 rounded transition-colors"
            onClick={(e) => e.preventDefault()}
          >
            Focusable link
          </a>
        </Tooltip>

        <Tooltip
          content={
            <div className="text-xs">
              <div>Shift + Tab to go back</div>
              <div>Tab to move forward</div>
            </div>
          }
          position="bottom"
        >
          <button className="px-4 py-2 bg-success-primary text-white rounded-md hover:bg-success-hover focus:ring-2 focus:ring-success-primary focus:ring-offset-2 transition-colors">
            Interactive element
          </button>
        </Tooltip>
      </div>
    </div>
  ),
  args: {
    content: 'Keyboard accessible tooltip',
    children: <button>Focus me</button>,
  },
};

/**
 * Interactive playground with all controls
 */
export const Playground: Story = {
  args: {
    content: 'Customize this tooltip using the controls below',
    position: 'top',
    theme: 'dark',
    showArrow: true,
    delay: 200,
    disabled: false,
    children: (
      <button className="px-4 py-2 bg-trust-hover text-white rounded-md hover:bg-trust transition-colors">
        Hover or focus me
      </button>
    ),
  },
};

/**
 * Real-world consent management use cases
 */
export const RealWorldExamples: Story = {
  render: () => {
    const [marketingEnabled, setMarketingEnabled] = useState(false);
    const [analyticsEnabled, setAnalyticsEnabled] = useState(true);

    const LockIcon = () => (
      <svg
        className="w-full h-full"
        fill="none"
        viewBox="0 0 24 24"
        stroke="currentColor"
      >
        <path
          strokeLinecap="round"
          strokeLinejoin="round"
          strokeWidth={2}
          d="M12 15v2m-6 4h12a2 2 0 002-2v-6a2 2 0 00-2-2H6a2 2 0 00-2 2v6a2 2 0 002 2zm10-10V7a4 4 0 00-8 0v4h8z"
        />
      </svg>
    );

    return (
      <div className="space-y-8 max-w-3xl p-8">
        <div>
          <h3 className="text-lg font-medium text-neutral-900 mb-4">
            Consent Management UI Examples
          </h3>

          {/* Consent Card Example */}
          <div className="bg-white border border-neutral-200 rounded-lg p-6 space-y-4">
            <div className="flex items-start justify-between">
              <div className="flex-1">
                <div className="flex items-center gap-2 mb-2">
                  <h4 className="text-base font-medium text-neutral-900">
                    Analytics Dashboard
                  </h4>
                  <Tooltip
                    content="Third-party analytics service for usage insights"
                    position="right"
                  >
                    <svg
                      className="w-4 h-4 text-neutral-400 cursor-help"
                      fill="none"
                      viewBox="0 0 24 24"
                      stroke="currentColor"
                    >
                      <path
                        strokeLinecap="round"
                        strokeLinejoin="round"
                        strokeWidth={2}
                        d="M13 16h-1v-4h-1m1-4h.01M21 12a9 9 0 11-18 0 9 9 0 0118 0z"
                      />
                    </svg>
                  </Tooltip>
                </div>
                <p className="text-sm text-neutral-600">
                  Access to usage statistics and behavior patterns
                </p>
              </div>
              <Tooltip
                content={
                  <div className="text-xs space-y-1">
                    <div className="font-medium">Permissions:</div>
                    <div>• View usage data</div>
                    <div>• Read profile info</div>
                    <div>• Anonymous analytics</div>
                  </div>
                }
                position="left"
                theme="light"
              >
                <button className="px-3 py-1.5 text-xs font-medium text-trust hover:text-trust-deep border border-neutral-300 rounded-md hover:bg-trust-light transition-colors">
                  View Permissions
                </button>
              </Tooltip>
            </div>

            <div className="flex items-center gap-6 text-xs">
              <Tooltip
                content="Data is encrypted in transit and at rest"
                position="bottom"
              >
                <StatusIndicator
                  icon={<LockIcon />}
                  label="Encrypted"
                  interactive
                />
              </Tooltip>
              <Tooltip
                content="Consent expires on Jan 15, 2026"
                position="bottom"
              >
                <StatusIndicator label="Valid for 30 days" interactive />
              </Tooltip>
              <Tooltip content="Last accessed 2 hours ago" position="bottom">
                <StatusIndicator label="Active" variant="success" interactive />
              </Tooltip>
            </div>
          </div>

          {/* Permission Toggle Example */}
          <div className="mt-6 bg-white border border-neutral-200 rounded-lg p-6">
            <div className="space-y-4">
              <div className="flex items-center justify-between">
                <div className="flex items-center gap-2">
                  <span className="text-sm font-medium text-neutral-900">
                    Marketing Communications
                  </span>
                  <Tooltip
                    content="Receive promotional emails and product updates"
                    position="right"
                  >
                    <svg
                      className="w-4 h-4 text-neutral-400 cursor-help"
                      fill="none"
                      viewBox="0 0 24 24"
                      stroke="currentColor"
                    >
                      <path
                        strokeLinecap="round"
                        strokeLinejoin="round"
                        strokeWidth={2}
                        d="M13 16h-1v-4h-1m1-4h.01M21 12a9 9 0 11-18 0 9 9 0 0118 0z"
                      />
                    </svg>
                  </Tooltip>
                </div>
                <Tooltip
                  content="Click to enable marketing emails"
                  position="left"
                >
                  <Switch
                    checked={marketingEnabled}
                    onChange={setMarketingEnabled}
                    size="md"
                  />
                </Tooltip>
              </div>

              <div className="flex items-center justify-between">
                <div className="flex items-center gap-2">
                  <span className="text-sm font-medium text-neutral-900">
                    Usage Analytics
                  </span>
                  <Tooltip
                    content="Help improve our service by sharing usage data"
                    position="right"
                  >
                    <svg
                      className="w-4 h-4 text-neutral-400 cursor-help"
                      fill="none"
                      viewBox="0 0 24 24"
                      stroke="currentColor"
                    >
                      <path
                        strokeLinecap="round"
                        strokeLinejoin="round"
                        strokeWidth={2}
                        d="M13 16h-1v-4h-1m1-4h.01M21 12a9 9 0 11-18 0 9 9 0 0118 0z"
                      />
                    </svg>
                  </Tooltip>
                </div>
                <Tooltip content="Analytics enabled" position="left">
                  <Switch
                    checked={analyticsEnabled}
                    onChange={setAnalyticsEnabled}
                    variant="success"
                    size="md"
                  />
                </Tooltip>
              </div>
            </div>
          </div>
        </div>
      </div>
    );
  },
  args: {
    content: 'Real-world example',
    children: <button>Trigger</button>,
  },
};

/**
 * Disabled state - tooltip won't show
 */
export const Disabled: Story = {
  render: () => (
    <div className="flex gap-8 items-center justify-center p-12">
      <Tooltip content="This tooltip is enabled" disabled={false}>
        <button className="px-4 py-2 bg-trust-hover text-white rounded-md hover:bg-trust transition-colors">
          Enabled Tooltip
        </button>
      </Tooltip>

      <Tooltip content="This tooltip won't show" disabled={true}>
        <button className="px-4 py-2 bg-neutral-400 text-white rounded-md cursor-not-allowed">
          Disabled Tooltip
        </button>
      </Tooltip>
    </div>
  ),
  args: {
    content: 'This tooltip is disabled',
    disabled: true,
    children: <button>No tooltip appears</button>,
  },
};

/**
 * Accessibility features demonstration
 */
export const Accessibility: Story = {
  render: () => (
    <div className="space-y-6 max-w-3xl p-8">
      <div className="p-4 bg-neutral-50 border border-neutral-200 rounded-lg">
        <h4 className="text-sm font-medium text-neutral-900 mb-2">
          Accessibility Features
        </h4>
        <ul className="text-sm text-neutral-700 space-y-1">
          <li>
            • Uses <code>role="tooltip"</code> for screen readers
          </li>
          <li>
            • Trigger has <code>aria-describedby</code> pointing to tooltip
          </li>
          <li>• Shows on both hover and keyboard focus (Tab key)</li>
          <li>• Configurable delay prevents accidental triggers</li>
          <li>• High contrast themes meet WCAG AA standards</li>
          <li>• Keyboard navigable with proper focus indicators</li>
          <li>• Does not trap focus or interfere with navigation</li>
        </ul>
      </div>

      <div className="flex flex-wrap gap-4">
        <Tooltip
          content="WCAG 2.1 AA compliant dark tooltip"
          position="top"
          theme="dark"
        >
          <button className="px-4 py-2 bg-trust-hover text-white rounded-md hover:bg-trust focus:ring-2 focus:ring-trust focus:ring-offset-2 transition-colors">
            Dark (AA Compliant)
          </button>
        </Tooltip>

        <Tooltip
          content="WCAG 2.1 AA compliant light tooltip"
          position="top"
          theme="light"
        >
          <button className="px-4 py-2 bg-white border border-neutral-300 text-neutral-900 rounded-md hover:bg-neutral-50 focus:ring-2 focus:ring-trust focus:ring-offset-2 transition-colors">
            Light (AA Compliant)
          </button>
        </Tooltip>
      </div>
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
            id: 'aria-allowed-attr',
            enabled: true,
          },
        ],
      },
    },
  },
  args: {
    content: 'Accessible tooltip',
    children: <button>Trigger</button>,
  },
};
