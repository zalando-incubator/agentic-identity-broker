/**
 * StatusIndicator Component Stories
 */

import type { Meta, StoryObj } from '@storybook/react';
import { StatusIndicator } from './StatusIndicator';
import { Tooltip } from '../../overlays/Tooltip';

const meta = {
  title: 'Design System/Data Display/StatusIndicator',
  component: StatusIndicator,
  parameters: {
    layout: 'centered',
  },
  tags: ['autodocs'],
  argTypes: {
    label: {
      control: 'text',
      description: 'Status label text',
    },
    variant: {
      control: 'select',
      options: ['default', 'success', 'warning', 'error', 'info'],
      description: 'Color variant of the indicator',
    },
    interactive: {
      control: 'boolean',
      description: 'Show cursor-help for tooltip context',
    },
  },
} satisfies Meta<typeof StatusIndicator>;

export default meta;
type Story = StoryObj<typeof meta>;

/**
 * Default status indicator
 */
export const Default: Story = {
  args: {
    label: 'Active',
  },
};

/**
 * Status indicator with icon
 */
export const WithIcon: Story = {
  args: {
    label: 'Encrypted',
    icon: (
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
    ),
  },
};

/**
 * All color variants
 */
export const Variants: Story = {
  render: () => (
    <div className="flex gap-6">
      <StatusIndicator label="Default" variant="default" />
      <StatusIndicator label="Success" variant="success" />
      <StatusIndicator label="Warning" variant="warning" />
      <StatusIndicator label="Error" variant="error" />
      <StatusIndicator label="Info" variant="info" />
    </div>
  ),
};

/**
 * Status indicator with tooltip (common pattern)
 */
export const WithTooltip: Story = {
  render: () => (
    <div className="flex gap-4">
      <Tooltip
        content="Data is encrypted in transit and at rest"
        position="bottom"
      >
        <StatusIndicator
          icon={
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
          }
          label="Encrypted"
          interactive
        />
      </Tooltip>

      <Tooltip content="Consent expires on Jan 15, 2026" position="bottom">
        <StatusIndicator label="Valid for 30 days" interactive />
      </Tooltip>

      <Tooltip content="Last accessed 2 hours ago" position="bottom">
        <StatusIndicator label="Active" variant="success" interactive />
      </Tooltip>
    </div>
  ),
};

/**
 * Permission metadata display (common use case)
 */
export const PermissionMetadata: Story = {
  render: () => (
    <div className="space-y-6 max-w-2xl">
      <div className="bg-white border border-neutral-200 rounded-lg p-6">
        <div className="mb-4">
          <h3 className="text-base font-medium text-neutral-900 mb-2">
            Analytics Dashboard
          </h3>
          <p className="text-sm text-neutral-600">
            Access to usage statistics and behavior patterns
          </p>
        </div>

        <div className="flex items-center gap-6 text-xs">
          <Tooltip
            content="Data is encrypted in transit and at rest"
            position="bottom"
          >
            <StatusIndicator
              icon={
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
              }
              label="Encrypted"
              interactive
            />
          </Tooltip>

          <Tooltip content="Consent expires on Jan 15, 2026" position="bottom">
            <StatusIndicator label="Valid for 30 days" interactive />
          </Tooltip>

          <Tooltip content="Last accessed 2 hours ago" position="bottom">
            <StatusIndicator label="Active" variant="success" interactive />
          </Tooltip>
        </div>
      </div>
    </div>
  ),
};

/**
 * Interactive - hover over indicators to see tooltips
 */
export const Interactive: Story = {
  render: () => (
    <div className="space-y-6 max-w-md">
      <Tooltip content="Permission is currently active" position="right">
        <StatusIndicator label="Active" variant="success" interactive />
      </Tooltip>

      <Tooltip content="This grant is no longer valid" position="right">
        <StatusIndicator label="Expired" variant="error" interactive />
      </Tooltip>

      <Tooltip content="Waiting for user approval" position="right">
        <StatusIndicator label="Pending" variant="warning" interactive />
      </Tooltip>

      <Tooltip content="Informational status" position="right">
        <StatusIndicator label="Processing" variant="info" interactive />
      </Tooltip>
    </div>
  ),
};
