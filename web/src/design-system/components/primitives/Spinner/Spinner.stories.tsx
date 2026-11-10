/**
 * Spinner Component Stories
 *
 * Demonstrates all Spinner variants, sizes, and configurations.
 * Used for visual regression testing and component exploration.
 */

import type { Meta, StoryObj } from '@storybook/react';
import { Spinner } from './Spinner';

const meta = {
  title: 'Design System/Primitives/Spinner',
  component: Spinner,
  parameters: {
    layout: 'centered',
    docs: {
      description: {
        component:
          'Loading indicator component with multiple sizes and semantic color variants. Uses smooth CSS animations and includes proper ARIA attributes for accessibility.',
      },
    },
  },
  tags: ['autodocs'],
  argTypes: {
    size: {
      control: 'select',
      options: ['xs', 'sm', 'md', 'lg'],
      description: 'The size of the spinner',
    },
    variant: {
      control: 'select',
      options: [
        'primary',
        'success',
        'error',
        'warning',
        'info',
        'neutral',
        'white',
      ],
      description: 'The semantic color variant',
    },
    label: {
      control: 'text',
      description: 'Accessible label for screen readers',
    },
  },
} satisfies Meta<typeof Spinner>;

export default meta;
type Story = StoryObj<typeof meta>;

/**
 * Default spinner with primary variant
 */
export const Default: Story = {
  args: {
    label: 'Loading...',
  },
};

/**
 * All spinner sizes from extra small to large
 */
export const Sizes: Story = {
  render: () => (
    <div className="flex items-center gap-8">
      <div className="flex flex-col items-center gap-2">
        <Spinner size="xs" />
        <span className="text-xs text-neutral-600">Extra Small</span>
      </div>
      <div className="flex flex-col items-center gap-2">
        <Spinner size="sm" />
        <span className="text-xs text-neutral-600">Small</span>
      </div>
      <div className="flex flex-col items-center gap-2">
        <Spinner size="md" />
        <span className="text-xs text-neutral-600">Medium</span>
      </div>
      <div className="flex flex-col items-center gap-2">
        <Spinner size="lg" />
        <span className="text-xs text-neutral-600">Large</span>
      </div>
    </div>
  ),
};

/**
 * All semantic color variants
 */
export const Variants: Story = {
  render: () => (
    <div className="flex flex-wrap items-center gap-8">
      <div className="flex flex-col items-center gap-2">
        <Spinner variant="primary" />
        <span className="text-xs text-neutral-600">Primary</span>
      </div>
      <div className="flex flex-col items-center gap-2">
        <Spinner variant="success" />
        <span className="text-xs text-neutral-600">Success</span>
      </div>
      <div className="flex flex-col items-center gap-2">
        <Spinner variant="error" />
        <span className="text-xs text-neutral-600">Error</span>
      </div>
      <div className="flex flex-col items-center gap-2">
        <Spinner variant="warning" />
        <span className="text-xs text-neutral-600">Warning</span>
      </div>
      <div className="flex flex-col items-center gap-2">
        <Spinner variant="info" />
        <span className="text-xs text-neutral-600">Info</span>
      </div>
      <div className="flex flex-col items-center gap-2">
        <Spinner variant="neutral" />
        <span className="text-xs text-neutral-600">Neutral</span>
      </div>
    </div>
  ),
};

/**
 * White spinner for dark backgrounds
 */
export const OnDarkBackground: Story = {
  render: () => (
    <div className="flex items-center justify-center gap-8 p-8 bg-trust-deep rounded-lg">
      <div className="flex flex-col items-center gap-2">
        <Spinner variant="white" size="sm" />
        <span className="text-xs text-white">Small</span>
      </div>
      <div className="flex flex-col items-center gap-2">
        <Spinner variant="white" size="md" />
        <span className="text-xs text-white">Medium</span>
      </div>
      <div className="flex flex-col items-center gap-2">
        <Spinner variant="white" size="lg" />
        <span className="text-xs text-white">Large</span>
      </div>
    </div>
  ),
};

/**
 * Spinner with text labels
 */
export const WithText: Story = {
  render: () => (
    <div className="flex flex-col gap-6">
      <div className="flex items-center gap-3">
        <Spinner size="sm" />
        <span className="text-sm text-neutral-700">Loading data...</span>
      </div>
      <div className="flex items-center gap-3">
        <Spinner size="md" variant="success" />
        <span className="text-base text-neutral-700">
          Processing request...
        </span>
      </div>
      <div className="flex items-center gap-3">
        <Spinner size="lg" variant="primary" />
        <span className="text-lg text-neutral-700">Please wait...</span>
      </div>
    </div>
  ),
};

/**
 * Centered spinner for full-page loading
 */
export const CenteredLoading: Story = {
  render: () => (
    <div className="flex items-center justify-center w-96 h-64 bg-neutral-50 rounded-lg border border-neutral-200">
      <div className="flex flex-col items-center gap-4">
        <Spinner size="lg" />
        <p className="text-sm text-neutral-600">Loading your data...</p>
      </div>
    </div>
  ),
};

/**
 * Inline spinners in different contexts
 */
export const InlineUseCases: Story = {
  render: () => (
    <div className="flex flex-col gap-6 p-6">
      {/* In a button */}
      <div className="space-y-2">
        <h3 className="text-sm font-medium text-neutral-700">
          Button Loading State
        </h3>
        <button
          className="inline-flex items-center gap-2 px-4 py-2 bg-trust-deep text-white rounded-md"
          disabled
        >
          <Spinner size="sm" variant="white" />
          Processing...
        </button>
      </div>

      {/* In a card header */}
      <div className="space-y-2">
        <h3 className="text-sm font-medium text-neutral-700">
          Card Loading State
        </h3>
        <div className="p-4 bg-white border border-neutral-200 rounded-lg">
          <div className="flex items-center gap-2 mb-3">
            <Spinner size="xs" />
            <h4 className="text-sm font-medium text-neutral-900">
              Fetching updates...
            </h4>
          </div>
          <div className="space-y-2">
            <div className="h-4 bg-neutral-100 rounded animate-pulse" />
            <div className="h-4 bg-neutral-100 rounded animate-pulse w-5/6" />
            <div className="h-4 bg-neutral-100 rounded animate-pulse w-4/6" />
          </div>
        </div>
      </div>

      {/* In a list item */}
      <div className="space-y-2">
        <h3 className="text-sm font-medium text-neutral-700">
          List Item Loading
        </h3>
        <div className="space-y-2">
          <div className="flex items-center gap-3 p-3 bg-white border border-neutral-200 rounded-lg">
            <Spinner size="xs" variant="info" />
            <span className="text-sm text-neutral-700">
              Syncing permissions...
            </span>
          </div>
          <div className="flex items-center gap-3 p-3 bg-white border border-neutral-200 rounded-lg">
            <Spinner size="xs" variant="success" />
            <span className="text-sm text-neutral-700">Updating grants...</span>
          </div>
        </div>
      </div>
    </div>
  ),
};

/**
 * Different loading states for operations
 */
export const LoadingStates: Story = {
  render: () => (
    <div className="flex flex-wrap gap-4">
      <div className="flex items-center gap-2 px-3 py-2 bg-blue-50 rounded-md">
        <Spinner size="xs" variant="info" />
        <span className="text-sm text-blue-900">Fetching...</span>
      </div>
      <div className="flex items-center gap-2 px-3 py-2 bg-green-50 rounded-md">
        <Spinner size="xs" variant="success" />
        <span className="text-sm text-green-900">Saving...</span>
      </div>
      <div className="flex items-center gap-2 px-3 py-2 bg-amber-50 rounded-md">
        <Spinner size="xs" variant="warning" />
        <span className="text-sm text-amber-900">Processing...</span>
      </div>
      <div className="flex items-center gap-2 px-3 py-2 bg-red-50 rounded-md">
        <Spinner size="xs" variant="error" />
        <span className="text-sm text-red-900">Retrying...</span>
      </div>
    </div>
  ),
};

/**
 * Interactive playground for testing all combinations
 */
export const Playground: Story = {
  args: {
    size: 'md',
    variant: 'primary',
    label: 'Loading...',
  },
};

/**
 * Accessibility test - screen reader friendly
 */
export const Accessibility: Story = {
  render: () => (
    <div className="flex flex-col gap-6">
      <div className="space-y-2">
        <h3 className="text-sm font-medium text-neutral-700">
          Spinners with descriptive labels
        </h3>
        <div className="flex flex-wrap gap-4">
          <Spinner label="Loading user profile data" />
          <Spinner variant="success" label="Saving changes to the database" />
          <Spinner variant="error" label="Retrying failed request" />
        </div>
      </div>

      <div className="space-y-2">
        <h3 className="text-sm font-medium text-neutral-700">
          Live region announcement (for dynamic loading)
        </h3>
        <div className="p-4 bg-neutral-50 rounded-lg">
          <div className="flex items-center gap-3">
            <Spinner size="sm" />
            <div aria-live="polite" aria-atomic="true">
              <p className="text-sm text-neutral-700">
                Loading... Please wait while we fetch your data.
              </p>
            </div>
          </div>
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
