/**
 * Badge Component Stories
 *
 * Demonstrates all Badge variants, sizes, and configurations.
 * Used for visual regression testing and component exploration.
 */

import type { Meta, StoryObj } from '@storybook/react';
import { Badge } from './Badge';

const meta = {
  title: 'Design System/Primitives/Badge',
  component: Badge,
  parameters: {
    layout: 'centered',
    docs: {
      description: {
        component:
          'Versatile badge component for status indicators, labels, and counts. Supports multiple variants, sizes, icons, and dot indicators.',
      },
    },
  },
  tags: ['autodocs'],
  argTypes: {
    variant: {
      control: 'select',
      options: ['success', 'error', 'warning', 'info', 'neutral', 'primary'],
      description: 'The semantic variant of the badge',
    },
    size: {
      control: 'select',
      options: ['sm', 'md', 'lg'],
      description: 'The size of the badge',
    },
    shape: {
      control: 'select',
      options: ['rounded', 'pill'],
      description: 'The shape of the badge corners',
    },
    showDot: {
      control: 'boolean',
      description: 'Show a dot indicator before the content',
    },
    children: {
      control: 'text',
      description: 'The content of the badge',
    },
  },
} satisfies Meta<typeof Badge>;

export default meta;
type Story = StoryObj<typeof meta>;

/**
 * Default badge with neutral variant
 */
export const Default: Story = {
  args: {
    children: 'Badge',
  },
};

/**
 * All badge variants showcasing semantic colors
 */
export const Variants: Story = {
  render: () => (
    <div className="flex flex-wrap gap-4">
      <Badge variant="success">Success</Badge>
      <Badge variant="error">Error</Badge>
      <Badge variant="warning">Warning</Badge>
      <Badge variant="info">Info</Badge>
      <Badge variant="neutral">Neutral</Badge>
      <Badge variant="primary">Primary</Badge>
    </div>
  ),
};

/**
 * Badge sizes from small to large
 */
export const Sizes: Story = {
  render: () => (
    <div className="flex flex-wrap items-center gap-4">
      <Badge variant="primary" size="sm">
        Small
      </Badge>
      <Badge variant="primary" size="md">
        Medium
      </Badge>
      <Badge variant="primary" size="lg">
        Large
      </Badge>
    </div>
  ),
};

/**
 * Badge shapes: rounded vs pill
 */
export const Shapes: Story = {
  render: () => (
    <div className="flex flex-wrap gap-4">
      <Badge variant="primary" shape="pill">
        Pill Shape
      </Badge>
      <Badge variant="primary" shape="rounded">
        Rounded Shape
      </Badge>
    </div>
  ),
};

/**
 * Badges with dot indicators for status
 */
export const WithDot: Story = {
  render: () => (
    <div className="flex flex-wrap gap-4">
      <Badge variant="success" showDot>
        Active
      </Badge>
      <Badge variant="error" showDot>
        Offline
      </Badge>
      <Badge variant="warning" showDot>
        Away
      </Badge>
      <Badge variant="info" showDot>
        Online
      </Badge>
    </div>
  ),
};

/**
 * Badges with icons before content
 */
export const WithIconBefore: Story = {
  render: () => (
    <div className="flex flex-wrap gap-4">
      <Badge
        variant="success"
        iconBefore={
          <svg
            fill="none"
            stroke="currentColor"
            viewBox="0 0 24 24"
            className="w-full h-full"
          >
            <path
              strokeLinecap="round"
              strokeLinejoin="round"
              strokeWidth={2}
              d="M9 12l2 2 4-4m6 2a9 9 0 11-18 0 9 9 0 0118 0z"
            />
          </svg>
        }
      >
        Verified
      </Badge>
      <Badge
        variant="error"
        iconBefore={
          <svg
            fill="none"
            stroke="currentColor"
            viewBox="0 0 24 24"
            className="w-full h-full"
          >
            <path
              strokeLinecap="round"
              strokeLinejoin="round"
              strokeWidth={2}
              d="M10 14l2-2m0 0l2-2m-2 2l-2-2m2 2l2 2m7-2a9 9 0 11-18 0 9 9 0 0118 0z"
            />
          </svg>
        }
      >
        Failed
      </Badge>
      <Badge
        variant="warning"
        iconBefore={
          <svg
            fill="none"
            stroke="currentColor"
            viewBox="0 0 24 24"
            className="w-full h-full"
          >
            <path
              strokeLinecap="round"
              strokeLinejoin="round"
              strokeWidth={2}
              d="M12 9v2m0 4h.01m-6.938 4h13.856c1.54 0 2.502-1.667 1.732-3L13.732 4c-.77-1.333-2.694-1.333-3.464 0L3.34 16c-.77 1.333.192 3 1.732 3z"
            />
          </svg>
        }
      >
        Warning
      </Badge>
    </div>
  ),
};

/**
 * Badges with icons after content
 */
export const WithIconAfter: Story = {
  render: () => (
    <div className="flex flex-wrap gap-4">
      <Badge
        variant="info"
        iconAfter={
          <svg
            fill="none"
            stroke="currentColor"
            viewBox="0 0 24 24"
            className="w-full h-full"
          >
            <path
              strokeLinecap="round"
              strokeLinejoin="round"
              strokeWidth={2}
              d="M13 7l5 5m0 0l-5 5m5-5H6"
            />
          </svg>
        }
      >
        Next
      </Badge>
      <Badge
        variant="primary"
        iconAfter={
          <svg
            fill="none"
            stroke="currentColor"
            viewBox="0 0 24 24"
            className="w-full h-full"
          >
            <path
              strokeLinecap="round"
              strokeLinejoin="round"
              strokeWidth={2}
              d="M10 6H6a2 2 0 00-2 2v10a2 2 0 002 2h10a2 2 0 002-2v-4M14 4h6m0 0v6m0-6L10 14"
            />
          </svg>
        }
      >
        External
      </Badge>
    </div>
  ),
};

/**
 * Real-world use cases for badges
 */
export const UseCases: Story = {
  render: () => (
    <div className="flex flex-col gap-6">
      {/* Status badges */}
      <div className="space-y-2">
        <h3 className="text-sm font-medium text-neutral-700">
          Status Indicators
        </h3>
        <div className="flex flex-wrap gap-2">
          <Badge variant="success" showDot>
            Active
          </Badge>
          <Badge variant="error" showDot>
            Expired
          </Badge>
          <Badge variant="warning" showDot>
            Pending
          </Badge>
          <Badge variant="neutral" showDot>
            Inactive
          </Badge>
        </div>
      </div>

      {/* Counts and labels */}
      <div className="space-y-2">
        <h3 className="text-sm font-medium text-neutral-700">
          Counts & Labels
        </h3>
        <div className="flex flex-wrap gap-2">
          <Badge variant="neutral" size="sm" shape="rounded">
            Beta
          </Badge>
          <Badge variant="primary" size="sm" shape="rounded">
            New
          </Badge>
          <Badge variant="info" size="sm" shape="pill">
            3
          </Badge>
          <Badge variant="warning" size="sm" shape="pill">
            99+
          </Badge>
        </div>
      </div>

      {/* Permission badges */}
      <div className="space-y-2">
        <h3 className="text-sm font-medium text-neutral-700">Permissions</h3>
        <div className="flex flex-wrap gap-2">
          <Badge variant="success" size="sm">
            Read
          </Badge>
          <Badge variant="warning" size="sm">
            Write
          </Badge>
          <Badge variant="error" size="sm">
            Delete
          </Badge>
          <Badge variant="info" size="sm">
            Admin
          </Badge>
        </div>
      </div>
    </div>
  ),
};

/**
 * Interactive playground for testing all combinations
 */
export const Playground: Story = {
  args: {
    children: 'Custom Badge',
    variant: 'primary',
    size: 'md',
    shape: 'pill',
    showDot: false,
  },
};

/**
 * Accessibility test - screen reader friendly
 */
export const Accessibility: Story = {
  render: () => (
    <div className="flex flex-col gap-4">
      <div className="space-y-2">
        <h3 className="text-sm font-medium text-neutral-700">
          Badges with semantic meaning
        </h3>
        <div className="flex flex-wrap gap-2">
          <Badge variant="success" role="status" aria-label="Status: Active">
            Active
          </Badge>
          <Badge variant="error" role="status" aria-label="Status: Failed">
            Failed
          </Badge>
          <Badge variant="warning" role="status" aria-label="Status: Pending">
            Pending
          </Badge>
        </div>
      </div>

      <div className="space-y-2">
        <h3 className="text-sm font-medium text-neutral-700">
          Notification counts with aria-label
        </h3>
        <div className="flex flex-wrap gap-2">
          <Badge variant="info" shape="pill" aria-label="3 unread messages">
            3
          </Badge>
          <Badge
            variant="error"
            shape="pill"
            aria-label="99 or more notifications"
          >
            99+
          </Badge>
        </div>
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
        ],
      },
    },
  },
};
