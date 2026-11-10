/**
 * Skeleton Component Stories
 */

import type { Meta, StoryObj } from '@storybook/react';
import { Skeleton } from './Skeleton';

const meta = {
  title: 'Design System/Feedback/Skeleton',
  component: Skeleton,
  parameters: {
    layout: 'padded',
  },
  tags: ['autodocs'],
  argTypes: {
    variant: {
      control: 'select',
      options: ['line', 'circle', 'rectangle', 'rounded'],
      description: 'Shape variant of the skeleton',
    },
    animate: {
      control: 'boolean',
      description: 'Whether to animate with pulse effect',
    },
    width: {
      control: 'text',
      description: 'Custom width (CSS value: px, %, rem, etc.)',
    },
    height: {
      control: 'text',
      description: 'Custom height (CSS value: px, %, rem, etc.)',
    },
    count: {
      control: 'number',
      description: 'Number of skeletons to render',
    },
    gap: {
      control: 'text',
      description: 'Gap between multiple skeletons (CSS value)',
    },
  },
} satisfies Meta<typeof Skeleton>;

export default meta;
type Story = StoryObj<typeof meta>;

/**
 * Default skeleton with line variant
 */
export const Default: Story = {
  args: {
    variant: 'line',
    animate: true,
  },
};

/**
 * Multiple text lines simulating a paragraph
 */
export const Text: Story = {
  render: () => (
    <div className="max-w-2xl space-y-6">
      <div>
        <h3 className="text-sm font-medium text-neutral-900 mb-3">
          Article Loading
        </h3>
        <div className="space-y-3">
          {/* Title */}
          <Skeleton width="70%" height="32px" />
          {/* Paragraph lines */}
          <div className="space-y-2 pt-2">
            <Skeleton />
            <Skeleton />
            <Skeleton width="90%" />
            <Skeleton width="85%" />
          </div>
        </div>
      </div>

      <div>
        <h3 className="text-sm font-medium text-neutral-900 mb-3">
          Comment Loading
        </h3>
        <div className="space-y-2">
          <Skeleton count={4} gap="0.5rem" />
          <Skeleton width="60%" />
        </div>
      </div>
    </div>
  ),
  args: {
    variant: 'line',
    count: 3,
  },
};

/**
 * Circular avatar skeleton
 */
export const Avatar: Story = {
  render: () => (
    <div className="space-y-6 max-w-2xl">
      <div>
        <h3 className="text-sm font-medium text-neutral-900 mb-3">
          Avatar Sizes
        </h3>
        <div className="flex items-center gap-4">
          <Skeleton variant="circle" width="32px" height="32px" />
          <Skeleton variant="circle" width="40px" height="40px" />
          <Skeleton variant="circle" width="48px" height="48px" />
          <Skeleton variant="circle" width="64px" height="64px" />
          <Skeleton variant="circle" width="80px" height="80px" />
        </div>
      </div>

      <div>
        <h3 className="text-sm font-medium text-neutral-900 mb-3">
          User Profile Loading
        </h3>
        <div className="flex items-center gap-3">
          <Skeleton variant="circle" width="48px" height="48px" />
          <div className="flex-1 space-y-2">
            <Skeleton width="150px" height="16px" />
            <Skeleton width="200px" height="14px" />
          </div>
        </div>
      </div>
    </div>
  ),
  args: {
    variant: 'circle',
    width: '48px',
    height: '48px',
  },
};

/**
 * Full card skeleton with image and text
 */
export const Card: Story = {
  render: () => (
    <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-6 max-w-6xl">
      {Array.from({ length: 3 }).map((_, index) => (
        <div
          key={index}
          className="bg-white rounded-lg border border-neutral-200 overflow-hidden"
        >
          {/* Card Image */}
          <Skeleton variant="rectangle" width="100%" height="200px" animate />

          {/* Card Content */}
          <div className="p-4 space-y-3">
            {/* Title */}
            <Skeleton width="80%" height="20px" />

            {/* Description */}
            <div className="space-y-2">
              <Skeleton />
              <Skeleton />
              <Skeleton width="60%" />
            </div>

            {/* Footer */}
            <div className="flex items-center gap-2 pt-2">
              <Skeleton variant="circle" width="24px" height="24px" />
              <Skeleton width="100px" height="14px" />
            </div>
          </div>
        </div>
      ))}
    </div>
  ),
  args: {
    variant: 'rounded',
  },
};

/**
 * Table skeleton with rows
 */
export const Table: Story = {
  render: () => (
    <div className="max-w-4xl">
      <div className="bg-white rounded-lg border border-neutral-200 overflow-hidden">
        {/* Table Header */}
        <div className="border-b border-neutral-200 bg-neutral-50 p-4">
          <div className="grid grid-cols-4 gap-4">
            <Skeleton width="60%" height="14px" />
            <Skeleton width="70%" height="14px" />
            <Skeleton width="50%" height="14px" />
            <Skeleton width="40%" height="14px" />
          </div>
        </div>

        {/* Table Rows */}
        <div className="divide-y divide-neutral-200">
          {Array.from({ length: 5 }).map((_, index) => (
            <div key={index} className="p-4">
              <div className="grid grid-cols-4 gap-4 items-center">
                <div className="flex items-center gap-2">
                  <Skeleton variant="circle" width="32px" height="32px" />
                  <Skeleton width="80px" />
                </div>
                <Skeleton width="120px" />
                <Skeleton width="90px" />
                <Skeleton width="70px" />
              </div>
            </div>
          ))}
        </div>
      </div>
    </div>
  ),
  args: {
    variant: 'line',
  },
};

/**
 * List item skeleton repeated
 */
export const List: Story = {
  render: () => (
    <div className="max-w-2xl space-y-6">
      <div>
        <h3 className="text-sm font-medium text-neutral-900 mb-3">
          Simple List
        </h3>
        <div className="bg-white rounded-lg border border-neutral-200 divide-y divide-neutral-200">
          {Array.from({ length: 5 }).map((_, index) => (
            <div key={index} className="p-4">
              <Skeleton width="70%" />
            </div>
          ))}
        </div>
      </div>

      <div>
        <h3 className="text-sm font-medium text-neutral-900 mb-3">
          Detailed List
        </h3>
        <div className="bg-white rounded-lg border border-neutral-200 divide-y divide-neutral-200">
          {Array.from({ length: 4 }).map((_, index) => (
            <div key={index} className="p-4 flex items-start gap-3">
              <Skeleton variant="circle" width="40px" height="40px" />
              <div className="flex-1 space-y-2">
                <Skeleton width="60%" height="18px" />
                <Skeleton width="90%" />
                <Skeleton width="70%" />
              </div>
              <Skeleton variant="rounded" width="60px" height="24px" />
            </div>
          ))}
        </div>
      </div>
    </div>
  ),
  args: {
    variant: 'line',
  },
};

/**
 * Different shape variants
 */
export const Shapes: Story = {
  render: () => (
    <div className="space-y-6 max-w-3xl">
      <div>
        <h3 className="text-sm font-medium text-neutral-900 mb-3">
          Line (default)
        </h3>
        <Skeleton variant="line" />
      </div>

      <div>
        <h3 className="text-sm font-medium text-neutral-900 mb-3">Circle</h3>
        <div className="flex gap-4">
          <Skeleton variant="circle" width="60px" height="60px" />
          <Skeleton variant="circle" width="80px" height="80px" />
          <Skeleton variant="circle" width="100px" height="100px" />
        </div>
      </div>

      <div>
        <h3 className="text-sm font-medium text-neutral-900 mb-3">Rectangle</h3>
        <Skeleton variant="rectangle" width="100%" height="120px" />
      </div>

      <div>
        <h3 className="text-sm font-medium text-neutral-900 mb-3">
          Rounded Rectangle
        </h3>
        <Skeleton variant="rounded" width="100%" height="120px" />
      </div>

      <div>
        <h3 className="text-sm font-medium text-neutral-900 mb-3">
          Custom Sizes
        </h3>
        <div className="space-y-3">
          <Skeleton width="100%" height="8px" />
          <Skeleton width="80%" height="12px" />
          <Skeleton width="60%" height="16px" />
          <Skeleton width="40%" height="20px" />
        </div>
      </div>
    </div>
  ),
  args: {
    variant: 'line',
  },
};

/**
 * Animation variations
 */
export const Animation: Story = {
  render: () => (
    <div className="space-y-6 max-w-2xl">
      <div>
        <h3 className="text-sm font-medium text-neutral-900 mb-3">
          With Pulse Animation (default)
        </h3>
        <div className="space-y-2">
          <Skeleton animate={true} />
          <Skeleton animate={true} width="90%" />
          <Skeleton animate={true} width="80%" />
        </div>
      </div>

      <div>
        <h3 className="text-sm font-medium text-neutral-900 mb-3">
          Without Animation
        </h3>
        <div className="space-y-2">
          <Skeleton animate={false} />
          <Skeleton animate={false} width="90%" />
          <Skeleton animate={false} width="80%" />
        </div>
      </div>

      <div>
        <h3 className="text-sm font-medium text-neutral-900 mb-3">
          Mixed Animation States
        </h3>
        <div className="grid grid-cols-2 gap-6">
          <div className="space-y-4">
            <h4 className="text-xs font-medium text-neutral-700">Animated</h4>
            <Skeleton variant="circle" width="64px" height="64px" animate />
            <Skeleton variant="rounded" width="100%" height="100px" animate />
          </div>
          <div className="space-y-4">
            <h4 className="text-xs font-medium text-neutral-700">Static</h4>
            <Skeleton
              variant="circle"
              width="64px"
              height="64px"
              animate={false}
            />
            <Skeleton
              variant="rounded"
              width="100%"
              height="100px"
              animate={false}
            />
          </div>
        </div>
      </div>
    </div>
  ),
  args: {
    animate: true,
  },
};

/**
 * Interactive playground with all controls
 */
export const Playground: Story = {
  args: {
    variant: 'line',
    animate: true,
    width: '100%',
    height: undefined,
    count: 1,
    gap: '0.5rem',
  },
};

/**
 * Real-world consent management examples
 */
export const ConsentManagementExamples: Story = {
  render: () => (
    <div className="space-y-8 max-w-4xl">
      <div>
        <h3 className="text-lg font-medium text-neutral-900 mb-4">
          Consent Request Loading
        </h3>
        <div className="bg-white rounded-lg border border-neutral-200 p-6">
          <div className="flex items-start gap-4 mb-6">
            <Skeleton variant="circle" width="56px" height="56px" />
            <div className="flex-1 space-y-3">
              <Skeleton width="60%" height="24px" />
              <Skeleton width="90%" />
              <Skeleton width="70%" />
            </div>
          </div>

          <div className="space-y-4 mb-6">
            <div className="flex items-center gap-3">
              <Skeleton variant="circle" width="20px" height="20px" />
              <Skeleton width="80%" />
            </div>
            <div className="flex items-center gap-3">
              <Skeleton variant="circle" width="20px" height="20px" />
              <Skeleton width="70%" />
            </div>
            <div className="flex items-center gap-3">
              <Skeleton variant="circle" width="20px" height="20px" />
              <Skeleton width="85%" />
            </div>
          </div>

          <div className="flex gap-3">
            <Skeleton variant="rounded" width="120px" height="40px" />
            <Skeleton variant="rounded" width="100px" height="40px" />
          </div>
        </div>
      </div>

      <div>
        <h3 className="text-lg font-medium text-neutral-900 mb-4">
          Consent History Loading
        </h3>
        <div className="bg-white rounded-lg border border-neutral-200">
          <div className="p-4 border-b border-neutral-200">
            <div className="flex items-center justify-between">
              <Skeleton width="200px" height="20px" />
              <Skeleton variant="rounded" width="100px" height="32px" />
            </div>
          </div>
          <div className="divide-y divide-neutral-200">
            {Array.from({ length: 4 }).map((_, index) => (
              <div key={index} className="p-4 flex items-center gap-4">
                <Skeleton variant="circle" width="40px" height="40px" />
                <div className="flex-1 space-y-2">
                  <Skeleton width="50%" height="16px" />
                  <Skeleton width="70%" height="14px" />
                </div>
                <Skeleton variant="rounded" width="80px" height="24px" />
              </div>
            ))}
          </div>
        </div>
      </div>

      <div>
        <h3 className="text-lg font-medium text-neutral-900 mb-4">
          Dashboard Loading
        </h3>
        <div className="grid grid-cols-1 md:grid-cols-3 gap-4">
          {Array.from({ length: 3 }).map((_, index) => (
            <div
              key={index}
              className="bg-white rounded-lg border border-neutral-200 p-6"
            >
              <Skeleton width="40%" height="14px" className="mb-4" />
              <Skeleton width="60%" height="32px" className="mb-2" />
              <Skeleton width="80%" height="12px" />
            </div>
          ))}
        </div>
      </div>
    </div>
  ),
  args: {
    variant: 'line',
  },
};

/**
 * Form loading states
 */
export const FormLoading: Story = {
  render: () => (
    <div className="max-w-2xl">
      <div className="bg-white rounded-lg border border-neutral-200 p-6">
        <Skeleton width="40%" height="28px" className="mb-6" />

        <div className="space-y-6">
          {/* Text input field */}
          <div>
            <Skeleton width="120px" height="14px" className="mb-2" />
            <Skeleton variant="rounded" width="100%" height="40px" />
          </div>

          {/* Text input field */}
          <div>
            <Skeleton width="150px" height="14px" className="mb-2" />
            <Skeleton variant="rounded" width="100%" height="40px" />
          </div>

          {/* Textarea */}
          <div>
            <Skeleton width="100px" height="14px" className="mb-2" />
            <Skeleton variant="rounded" width="100%" height="120px" />
          </div>

          {/* Checkboxes */}
          <div className="space-y-3">
            <Skeleton width="180px" height="14px" className="mb-3" />
            <div className="flex items-center gap-3">
              <Skeleton variant="rounded" width="20px" height="20px" />
              <Skeleton width="60%" />
            </div>
            <div className="flex items-center gap-3">
              <Skeleton variant="rounded" width="20px" height="20px" />
              <Skeleton width="50%" />
            </div>
            <div className="flex items-center gap-3">
              <Skeleton variant="rounded" width="20px" height="20px" />
              <Skeleton width="55%" />
            </div>
          </div>

          {/* Submit button */}
          <div className="flex gap-3 pt-4">
            <Skeleton variant="rounded" width="120px" height="44px" />
            <Skeleton variant="rounded" width="100px" height="44px" />
          </div>
        </div>
      </div>
    </div>
  ),
  args: {
    variant: 'line',
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
            • Uses <code>role="status"</code> for screen reader announcements
          </li>
          <li>
            • Includes <code>aria-label="Loading"</code> for context
          </li>
          <li>
            • Uses <code>aria-live="polite"</code> to avoid interruptions
          </li>
          <li>
            • Contains <code className="sr-only">Loading...</code> text for
            screen readers
          </li>
          <li>• Non-interactive element (no focus management needed)</li>
          <li>• Sufficient color contrast for visibility</li>
          <li>• Animation can be disabled via prefers-reduced-motion</li>
        </ul>
      </div>

      <div className="bg-white rounded-lg border border-neutral-200 p-6">
        <div className="space-y-4">
          <Skeleton count={3} />
        </div>
        <p className="text-xs text-neutral-500 mt-4">
          Screen readers will announce "Loading" when this skeleton appears
        </p>
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
  args: {
    variant: 'line',
  },
};
