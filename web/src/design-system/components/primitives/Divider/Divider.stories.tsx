/**
 * Divider Component Stories
 *
 * Demonstrates all Divider variants, orientations, and configurations.
 * Used for visual regression testing and component exploration.
 */

import type { Meta, StoryObj } from '@storybook/react';
import { Divider } from './Divider';

const meta = {
  title: 'Design System/Primitives/Divider',
  component: Divider,
  parameters: {
    layout: 'centered',
    docs: {
      description: {
        component:
          'Divider component for visual content separation. Supports horizontal and vertical orientations with optional centered text labels.',
      },
    },
  },
  tags: ['autodocs'],
  argTypes: {
    orientation: {
      control: 'select',
      options: ['horizontal', 'vertical'],
      description: 'The orientation of the divider',
    },
    variant: {
      control: 'select',
      options: ['default', 'subtle', 'muted'],
      description: 'The visual prominence variant',
    },
    style: {
      control: 'select',
      options: ['solid', 'dashed'],
      description: 'The line style',
    },
    label: {
      control: 'text',
      description:
        'Optional text label to display in the center (horizontal only)',
    },
    labelSpacing: {
      control: 'select',
      options: ['sm', 'md', 'lg'],
      description: 'Spacing around the label',
    },
  },
} satisfies Meta<typeof Divider>;

export default meta;
type Story = StoryObj<typeof meta>;

/**
 * Default horizontal divider
 */
export const Default: Story = {
  args: {},
};

/**
 * All divider variants from prominent to subtle
 */
export const Variants: Story = {
  render: () => (
    <div className="w-full max-w-md space-y-8">
      <div className="space-y-2">
        <h4 className="text-sm font-medium text-neutral-700">Default</h4>
        <Divider variant="default" />
      </div>

      <div className="space-y-2">
        <h4 className="text-sm font-medium text-neutral-700">Subtle</h4>
        <Divider variant="subtle" />
      </div>

      <div className="space-y-2">
        <h4 className="text-sm font-medium text-neutral-700">Muted</h4>
        <Divider variant="muted" />
      </div>
    </div>
  ),
};

/**
 * Divider line styles: solid and dashed
 */
export const Styles: Story = {
  render: () => (
    <div className="w-full max-w-md space-y-8">
      <div className="space-y-2">
        <h4 className="text-sm font-medium text-neutral-700">Solid</h4>
        <Divider style="solid" />
      </div>

      <div className="space-y-2">
        <h4 className="text-sm font-medium text-neutral-700">Dashed</h4>
        <Divider style="dashed" />
      </div>
    </div>
  ),
};

/**
 * Divider with centered label
 */
export const WithLabel: Story = {
  render: () => (
    <div className="w-full max-w-md space-y-8">
      <Divider label="Or" />
      <Divider label="Section Break" />
      <Divider label="More Options" style="dashed" />
      <Divider label="End of Content" variant="subtle" />
    </div>
  ),
};

/**
 * Label spacing variations
 */
export const LabelSpacing: Story = {
  render: () => (
    <div className="w-full max-w-md space-y-8">
      <div className="space-y-2">
        <h4 className="text-sm font-medium text-neutral-700">Small Spacing</h4>
        <Divider label="Or" labelSpacing="sm" />
      </div>

      <div className="space-y-2">
        <h4 className="text-sm font-medium text-neutral-700">Medium Spacing</h4>
        <Divider label="Or" labelSpacing="md" />
      </div>

      <div className="space-y-2">
        <h4 className="text-sm font-medium text-neutral-700">Large Spacing</h4>
        <Divider label="Or" labelSpacing="lg" />
      </div>
    </div>
  ),
};

/**
 * Vertical divider for side-by-side layouts
 */
export const Vertical: Story = {
  render: () => (
    <div className="flex items-center gap-4 h-32">
      <div className="flex-1 p-4 bg-neutral-50 rounded-lg">
        <p className="text-sm text-neutral-600">Left Section</p>
      </div>
      <Divider orientation="vertical" />
      <div className="flex-1 p-4 bg-neutral-50 rounded-lg">
        <p className="text-sm text-neutral-600">Right Section</p>
      </div>
    </div>
  ),
};

/**
 * Real-world use cases
 */
export const UseCases: Story = {
  render: () => (
    <div className="w-full max-w-md space-y-6 p-6 bg-white rounded-lg border border-neutral-200">
      {/* Form sections */}
      <div className="space-y-4">
        <div>
          <label className="text-sm font-medium text-neutral-700">Email</label>
          <input
            type="email"
            className="mt-1 block w-full rounded-md border border-neutral-300 px-3 py-2 text-sm"
            placeholder="you@example.com"
          />
        </div>
        <div>
          <label className="text-sm font-medium text-neutral-700">
            Password
          </label>
          <input
            type="password"
            className="mt-1 block w-full rounded-md border border-neutral-300 px-3 py-2 text-sm"
            placeholder="••••••••"
          />
        </div>
      </div>

      <Divider label="Or" />

      {/* Alternative method */}
      <button className="w-full py-2 px-3 bg-neutral-100 text-neutral-900 rounded-md text-sm font-medium hover:bg-neutral-200 transition">
        Sign in with Google
      </button>
    </div>
  ),
};

/**
 * Content sections separated by dividers
 */
export const ContentSeparation: Story = {
  render: () => (
    <div className="w-full max-w-lg p-6 space-y-6">
      {/* Section 1 */}
      <div>
        <h3 className="text-lg font-medium text-neutral-900 mb-2">Overview</h3>
        <p className="text-sm text-neutral-600">
          This is the overview section with important information about your
          account.
        </p>
      </div>

      <Divider variant="subtle" />

      {/* Section 2 */}
      <div>
        <h3 className="text-lg font-medium text-neutral-900 mb-2">Settings</h3>
        <p className="text-sm text-neutral-600">
          Configure your preferences and notification settings here.
        </p>
      </div>

      <Divider variant="subtle" />

      {/* Section 3 */}
      <div>
        <h3 className="text-lg font-medium text-neutral-900 mb-2">Security</h3>
        <p className="text-sm text-neutral-600">
          Manage your password and connected devices.
        </p>
      </div>

      <Divider variant="subtle" />

      {/* Section 4 */}
      <div>
        <h3 className="text-lg font-medium text-neutral-900 mb-2">Privacy</h3>
        <p className="text-sm text-neutral-600">
          Control how your data is shared and used.
        </p>
      </div>
    </div>
  ),
};

/**
 * Steps or timeline with dividers
 */
export const Timeline: Story = {
  render: () => (
    <div className="w-full max-w-md p-6 space-y-4">
      <div className="flex gap-3">
        <div className="w-8 h-8 rounded-full bg-success-primary text-white flex items-center justify-center text-sm font-medium">
          ✓
        </div>
        <div>
          <h4 className="text-sm font-medium text-neutral-900">
            Step 1: Review
          </h4>
          <p className="text-xs text-neutral-600">Completed</p>
        </div>
      </div>

      <div className="ml-4">
        <Divider orientation="vertical" style="dashed" />
      </div>

      <div className="flex gap-3">
        <div className="w-8 h-8 rounded-full bg-info-primary text-white flex items-center justify-center text-sm font-medium">
          2
        </div>
        <div>
          <h4 className="text-sm font-medium text-neutral-900">
            Step 2: Approve
          </h4>
          <p className="text-xs text-neutral-600">In progress</p>
        </div>
      </div>

      <div className="ml-4">
        <Divider orientation="vertical" style="dashed" />
      </div>

      <div className="flex gap-3">
        <div className="w-8 h-8 rounded-full bg-neutral-300 text-neutral-600 flex items-center justify-center text-sm font-medium">
          3
        </div>
        <div>
          <h4 className="text-sm font-medium text-neutral-900">
            Step 3: Complete
          </h4>
          <p className="text-xs text-neutral-600">Pending</p>
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
    orientation: 'horizontal',
    variant: 'default',
    style: 'solid',
    label: 'Or',
    labelSpacing: 'md',
  },
};

/**
 * Accessibility test - screen reader friendly
 */
export const Accessibility: Story = {
  render: () => (
    <div className="w-full max-w-md space-y-6">
      <div className="space-y-2">
        <h3 className="text-sm font-medium text-neutral-700">
          Dividers with semantic meaning
        </h3>
        <div className="space-y-4">
          <div>
            <p className="text-sm text-neutral-600 mb-2">Account Information</p>
            <Divider />
          </div>
          <div>
            <p className="text-sm text-neutral-600 mb-2">Security Settings</p>
            <Divider />
          </div>
        </div>
      </div>

      <div className="space-y-2">
        <h3 className="text-sm font-medium text-neutral-700">
          Labeled dividers
        </h3>
        <div className="space-y-4">
          <div>
            <p className="text-sm text-neutral-600 mb-2">Alternative Sign-In</p>
            <Divider label="Or continue with" />
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
