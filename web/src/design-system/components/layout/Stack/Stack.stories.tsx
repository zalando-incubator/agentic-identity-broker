/**
 * Stack Component Stories
 *
 * Demonstrates all Stack variants, directions, and usage patterns.
 */

import type { Meta, StoryObj } from '@storybook/react';
import { Stack } from './Stack';

const meta = {
  title: 'Design System/Layout/Stack',
  component: Stack,
  parameters: {
    layout: 'padded',
  },
  tags: ['autodocs'],
  argTypes: {
    direction: {
      control: 'select',
      options: ['row', 'column'],
      description: 'Direction of stack - row (horizontal) or column (vertical)',
      table: {
        type: { summary: 'string' },
        defaultValue: { summary: 'column' },
      },
    },
    gap: {
      control: 'select',
      options: ['xs', 'sm', 'md', 'lg', 'xl'],
      description: 'Gap size between items',
      table: {
        type: { summary: 'string' },
        defaultValue: { summary: 'md' },
      },
    },
    align: {
      control: 'select',
      options: ['start', 'center', 'end', 'stretch'],
      description: 'Alignment on cross axis',
      table: {
        type: { summary: 'string' },
        defaultValue: { summary: 'stretch' },
      },
    },
    justify: {
      control: 'select',
      options: ['start', 'center', 'end', 'space-between', 'space-around'],
      description: 'Justification on main axis',
      table: {
        type: { summary: 'string' },
        defaultValue: { summary: 'start' },
      },
    },
    wrap: {
      control: 'boolean',
      description: 'Enable wrapping for multi-line layouts',
      table: {
        type: { summary: 'boolean' },
        defaultValue: { summary: 'false' },
      },
    },
    children: {
      control: 'text',
      description: 'Stack content',
    },
  },
} satisfies Meta<typeof Stack>;

export default meta;
type Story = StoryObj<typeof meta>;

// Helper component for consistent demo items
const StackItem = ({
  children,
  variant = 'default',
  height,
}: {
  children: React.ReactNode;
  variant?: 'default' | 'highlight' | 'accent';
  height?: string;
}) => {
  const variantClasses = {
    default: 'bg-trust-light text-trust-deep border-neutral-200',
    highlight: 'bg-success-light text-success-dark border-success-light',
    accent: 'bg-sand text-secondary-900 border-slate',
  };

  return (
    <div
      className={`px-4 py-3 rounded-lg border-2 font-medium text-sm ${variantClasses[variant]}`}
      style={height ? { height } : undefined}
    >
      {children}
    </div>
  );
};

/**
 * Default vertical stack with medium gap.
 * This is the most common stack configuration for vertical layouts.
 */
export const Default: Story = {
  args: {
    direction: 'column',
    gap: 'md',
    align: 'stretch',
    justify: 'start',
    wrap: false,
    children: (
      <>
        <StackItem>Item 1</StackItem>
        <StackItem>Item 2</StackItem>
        <StackItem>Item 3</StackItem>
      </>
    ),
  },
};

/**
 * Horizontal stack (row direction).
 * Useful for navigation bars, button groups, and horizontal layouts.
 */
export const Horizontal: Story = {
  args: {
    direction: 'row',
    gap: 'md',
    align: 'center',
    justify: 'start',
    wrap: false,
    children: (
      <>
        <StackItem>Action 1</StackItem>
        <StackItem>Action 2</StackItem>
        <StackItem>Action 3</StackItem>
      </>
    ),
  },
};

/**
 * All gap sizes for visual comparison.
 * Choose gap sizes based on visual hierarchy and spacing needs:
 * - xs (4px): Minimal spacing, tight grouping
 * - sm (8px): Small spacing, related items
 * - md (16px): Standard spacing (default)
 * - lg (24px): Generous spacing, distinct sections
 * - xl (32px): Maximum spacing, strong separation
 */
export const Gaps: Story = {
  render: () => (
    <div className="space-y-8 bg-cream p-6 rounded-xl">
      <div>
        <div className="mb-2 text-sm font-medium text-secondary-600">
          Extra Small Gap (xs - 4px)
        </div>
        <Stack gap="xs">
          <StackItem>Item 1</StackItem>
          <StackItem>Item 2</StackItem>
          <StackItem>Item 3</StackItem>
        </Stack>
      </div>

      <div>
        <div className="mb-2 text-sm font-medium text-secondary-600">
          Small Gap (sm - 8px)
        </div>
        <Stack gap="sm">
          <StackItem>Item 1</StackItem>
          <StackItem>Item 2</StackItem>
          <StackItem>Item 3</StackItem>
        </Stack>
      </div>

      <div>
        <div className="mb-2 text-sm font-medium text-secondary-600">
          Medium Gap (md - 16px) - Default
        </div>
        <Stack gap="md">
          <StackItem>Item 1</StackItem>
          <StackItem>Item 2</StackItem>
          <StackItem>Item 3</StackItem>
        </Stack>
      </div>

      <div>
        <div className="mb-2 text-sm font-medium text-secondary-600">
          Large Gap (lg - 24px)
        </div>
        <Stack gap="lg">
          <StackItem>Item 1</StackItem>
          <StackItem>Item 2</StackItem>
          <StackItem>Item 3</StackItem>
        </Stack>
      </div>

      <div>
        <div className="mb-2 text-sm font-medium text-secondary-600">
          Extra Large Gap (xl - 32px)
        </div>
        <Stack gap="xl">
          <StackItem>Item 1</StackItem>
          <StackItem>Item 2</StackItem>
          <StackItem>Item 3</StackItem>
        </Stack>
      </div>
    </div>
  ),
};

/**
 * Cross-axis alignment options.
 * Controls how items align perpendicular to the stack direction:
 * - start: Align to start of cross axis
 * - center: Center items on cross axis
 * - end: Align to end of cross axis
 * - stretch: Fill cross axis (default)
 */
export const Alignment: Story = {
  render: () => (
    <div className="space-y-8 bg-cream p-6 rounded-xl">
      <div>
        <div className="mb-2 text-sm font-medium text-secondary-600">
          Align Start
        </div>
        <Stack
          direction="row"
          gap="md"
          align="start"
          className="bg-white p-4 rounded-lg border-2 border-dashed border-neutral-200 min-h-[120px]"
        >
          <StackItem height="40px">Short</StackItem>
          <StackItem height="60px">Medium</StackItem>
          <StackItem height="80px">Tall</StackItem>
        </Stack>
      </div>

      <div>
        <div className="mb-2 text-sm font-medium text-secondary-600">
          Align Center
        </div>
        <Stack
          direction="row"
          gap="md"
          align="center"
          className="bg-white p-4 rounded-lg border-2 border-dashed border-neutral-200 min-h-[120px]"
        >
          <StackItem height="40px">Short</StackItem>
          <StackItem height="60px">Medium</StackItem>
          <StackItem height="80px">Tall</StackItem>
        </Stack>
      </div>

      <div>
        <div className="mb-2 text-sm font-medium text-secondary-600">
          Align End
        </div>
        <Stack
          direction="row"
          gap="md"
          align="end"
          className="bg-white p-4 rounded-lg border-2 border-dashed border-neutral-200 min-h-[120px]"
        >
          <StackItem height="40px">Short</StackItem>
          <StackItem height="60px">Medium</StackItem>
          <StackItem height="80px">Tall</StackItem>
        </Stack>
      </div>

      <div>
        <div className="mb-2 text-sm font-medium text-secondary-600">
          Align Stretch (Default)
        </div>
        <Stack
          direction="row"
          gap="md"
          align="stretch"
          className="bg-white p-4 rounded-lg border-2 border-dashed border-neutral-200 min-h-[120px]"
        >
          <StackItem>Stretch 1</StackItem>
          <StackItem>Stretch 2</StackItem>
          <StackItem>Stretch 3</StackItem>
        </Stack>
      </div>
    </div>
  ),
};

/**
 * Main-axis justification options.
 * Controls how items are distributed along the stack direction:
 * - start: Pack items to start (default)
 * - center: Pack items to center
 * - end: Pack items to end
 * - space-between: First item at start, last at end, equal spacing
 * - space-around: Equal space around each item
 */
export const Justify: Story = {
  render: () => (
    <div className="space-y-8 bg-cream p-6 rounded-xl">
      <div>
        <div className="mb-2 text-sm font-medium text-secondary-600">
          Justify Start (Default)
        </div>
        <Stack
          direction="row"
          gap="md"
          justify="start"
          className="bg-white p-4 rounded-lg border-2 border-dashed border-neutral-200"
        >
          <StackItem>Item 1</StackItem>
          <StackItem>Item 2</StackItem>
          <StackItem>Item 3</StackItem>
        </Stack>
      </div>

      <div>
        <div className="mb-2 text-sm font-medium text-secondary-600">
          Justify Center
        </div>
        <Stack
          direction="row"
          gap="md"
          justify="center"
          className="bg-white p-4 rounded-lg border-2 border-dashed border-neutral-200"
        >
          <StackItem>Item 1</StackItem>
          <StackItem>Item 2</StackItem>
          <StackItem>Item 3</StackItem>
        </Stack>
      </div>

      <div>
        <div className="mb-2 text-sm font-medium text-secondary-600">
          Justify End
        </div>
        <Stack
          direction="row"
          gap="md"
          justify="end"
          className="bg-white p-4 rounded-lg border-2 border-dashed border-neutral-200"
        >
          <StackItem>Item 1</StackItem>
          <StackItem>Item 2</StackItem>
          <StackItem>Item 3</StackItem>
        </Stack>
      </div>

      <div>
        <div className="mb-2 text-sm font-medium text-secondary-600">
          Justify Space Between
        </div>
        <Stack
          direction="row"
          gap="md"
          justify="space-between"
          className="bg-white p-4 rounded-lg border-2 border-dashed border-neutral-200"
        >
          <StackItem>Item 1</StackItem>
          <StackItem>Item 2</StackItem>
          <StackItem>Item 3</StackItem>
        </Stack>
      </div>

      <div>
        <div className="mb-2 text-sm font-medium text-secondary-600">
          Justify Space Around
        </div>
        <Stack
          direction="row"
          gap="md"
          justify="space-around"
          className="bg-white p-4 rounded-lg border-2 border-dashed border-neutral-200"
        >
          <StackItem>Item 1</StackItem>
          <StackItem>Item 2</StackItem>
          <StackItem>Item 3</StackItem>
        </Stack>
      </div>
    </div>
  ),
};

/**
 * Responsive stack with direction changes.
 * Stack changes from row (horizontal) on desktop to column (vertical) on mobile.
 * This pattern is useful for responsive navigation, card grids, and adaptive layouts.
 */
export const Responsive: Story = {
  render: () => (
    <div className="space-y-8 bg-cream p-6 rounded-xl">
      <div className="bg-white rounded-lg p-6 shadow-md-premium">
        <h2 className="text-xl font-display font-medium text-trust-deep mb-4">
          Responsive Stack Behavior
        </h2>
        <p className="text-secondary-600 mb-4">
          Try resizing your browser window to see how the stack adapts:
        </p>
        <ul className="list-disc list-inside text-secondary-600 space-y-2 mb-6">
          <li>On wide screens: horizontal layout (row)</li>
          <li>On narrow screens: vertical layout (column)</li>
          <li>Breakpoint: 768px (Tailwind md: breakpoint)</li>
        </ul>

        <Stack direction="row" gap="md" className="md:flex-row flex-col">
          <StackItem variant="highlight">Responsive Item 1</StackItem>
          <StackItem variant="highlight">Responsive Item 2</StackItem>
          <StackItem variant="highlight">Responsive Item 3</StackItem>
        </Stack>
      </div>

      <div className="bg-white rounded-lg p-6 shadow-md-premium">
        <h3 className="text-lg font-display font-medium text-trust-deep mb-3">
          Mobile-First Approach
        </h3>
        <p className="text-secondary-600 mb-4">
          Stack defaults to column on mobile, switches to row on tablet+:
        </p>

        <Stack direction="column" gap="sm" className="sm:flex-row">
          <StackItem variant="accent">Mobile: Column</StackItem>
          <StackItem variant="accent">Tablet+: Row</StackItem>
        </Stack>
      </div>
    </div>
  ),
};

/**
 * Nested stacks for complex layouts.
 * Outer stack provides overall structure,
 * inner stacks handle specific sections.
 */
export const Nested: Story = {
  render: () => (
    <Stack gap="lg" className="bg-cream p-6 rounded-xl">
      {/* Header with horizontal stack */}
      <Stack
        direction="row"
        justify="space-between"
        align="center"
        className="bg-white p-4 rounded-lg shadow-sm"
      >
        <div className="text-lg font-display font-medium text-trust-deep">
          Application Header
        </div>
        <Stack direction="row" gap="sm">
          <StackItem variant="default">Profile</StackItem>
          <StackItem variant="default">Settings</StackItem>
        </Stack>
      </Stack>

      {/* Main content area */}
      <Stack gap="md" className="bg-white p-6 rounded-lg shadow-sm">
        <h2 className="text-xl font-display font-medium text-trust-deep">
          Main Content Section
        </h2>

        {/* Nested horizontal stack for cards */}
        <Stack direction="row" gap="md" wrap={true}>
          <StackItem variant="highlight">Card 1</StackItem>
          <StackItem variant="highlight">Card 2</StackItem>
          <StackItem variant="highlight">Card 3</StackItem>
        </Stack>

        {/* Nested vertical stack for list */}
        <Stack gap="sm">
          <div className="text-sm font-medium text-secondary-600">
            List Items:
          </div>
          <StackItem variant="accent">List item A</StackItem>
          <StackItem variant="accent">List item B</StackItem>
          <StackItem variant="accent">List item C</StackItem>
        </Stack>
      </Stack>

      {/* Footer with horizontal stack */}
      <Stack
        direction="row"
        justify="space-between"
        align="center"
        className="bg-white p-4 rounded-lg shadow-sm"
      >
        <div className="text-sm text-secondary-600">Copyright 2024</div>
        <Stack direction="row" gap="md">
          <StackItem variant="default">Terms</StackItem>
          <StackItem variant="default">Privacy</StackItem>
          <StackItem variant="default">Contact</StackItem>
        </Stack>
      </Stack>
    </Stack>
  ),
};

/**
 * Interactive playground to experiment with all props.
 * Try different combinations of direction, gap, alignment, and justification.
 */
export const Playground: Story = {
  args: {
    direction: 'column',
    gap: 'md',
    align: 'stretch',
    justify: 'start',
    wrap: false,
    children: (
      <>
        <StackItem>Item 1</StackItem>
        <StackItem>Item 2</StackItem>
        <StackItem>Item 3</StackItem>
        <StackItem>Item 4</StackItem>
      </>
    ),
  },
  parameters: {
    docs: {
      description: {
        story:
          'Experiment with all stack props using the controls below. Try different directions, gaps, alignments, and justification options.',
      },
    },
  },
};

/**
 * Real-world usage examples showing common patterns.
 * These demonstrate how stacks are typically used in applications.
 */
export const RealWorldExamples: Story = {
  render: () => (
    <Stack gap="xl" className="bg-cream p-6 rounded-xl">
      {/* Button Group Pattern */}
      <div className="bg-white p-6 rounded-lg shadow-md-premium">
        <h3 className="text-lg font-display font-medium text-trust-deep mb-4">
          Button Group Pattern
        </h3>
        <Stack direction="row" gap="sm" justify="end">
          <button className="px-4 py-2 rounded-lg border-2 border-neutral-200 text-trust-deep hover:bg-trust-light">
            Cancel
          </button>
          <button className="px-4 py-2 rounded-lg bg-success-primary text-white hover:bg-success-hover">
            Confirm
          </button>
        </Stack>
      </div>

      {/* Form Layout Pattern */}
      <div className="bg-white p-6 rounded-lg shadow-md-premium">
        <h3 className="text-lg font-display font-medium text-trust-deep mb-4">
          Form Layout Pattern
        </h3>
        <Stack gap="md">
          <div>
            <label className="block text-sm font-medium text-secondary-700 mb-1">
              Name
            </label>
            <input
              type="text"
              className="w-full px-3 py-2 border-2 border-slate rounded-lg"
              placeholder="Enter your name"
            />
          </div>
          <div>
            <label className="block text-sm font-medium text-secondary-700 mb-1">
              Email
            </label>
            <input
              type="email"
              className="w-full px-3 py-2 border-2 border-slate rounded-lg"
              placeholder="Enter your email"
            />
          </div>
          <Stack direction="row" gap="sm" justify="end">
            <button className="px-4 py-2 rounded-lg border-2 border-neutral-200 text-trust-deep">
              Cancel
            </button>
            <button className="px-4 py-2 rounded-lg bg-success-primary text-white">
              Submit
            </button>
          </Stack>
        </Stack>
      </div>

      {/* Navigation Pattern */}
      <div className="bg-white p-4 rounded-lg shadow-md-premium">
        <Stack direction="row" justify="space-between" align="center">
          <div className="text-xl font-display font-medium text-trust-deep">
            Logo
          </div>
          <Stack direction="row" gap="md" align="center">
            <a href="#" className="text-trust-deep hover:text-success-primary">
              Home
            </a>
            <a href="#" className="text-trust-deep hover:text-success-primary">
              About
            </a>
            <a href="#" className="text-trust-deep hover:text-success-primary">
              Contact
            </a>
            <button className="px-4 py-2 rounded-lg bg-success-primary text-white">
              Sign In
            </button>
          </Stack>
        </Stack>
      </div>

      {/* Card Grid with Wrapping */}
      <div className="bg-white p-6 rounded-lg shadow-md-premium">
        <h3 className="text-lg font-display font-medium text-trust-deep mb-4">
          Card Grid with Wrapping
        </h3>
        <Stack direction="row" gap="md" wrap={true}>
          <div className="bg-trust-light p-4 rounded-lg border-2 border-neutral-200 min-w-[150px]">
            <div className="font-medium text-trust-deep mb-2">Feature 1</div>
            <div className="text-sm text-secondary-600">Description</div>
          </div>
          <div className="bg-trust-light p-4 rounded-lg border-2 border-neutral-200 min-w-[150px]">
            <div className="font-medium text-trust-deep mb-2">Feature 2</div>
            <div className="text-sm text-secondary-600">Description</div>
          </div>
          <div className="bg-trust-light p-4 rounded-lg border-2 border-neutral-200 min-w-[150px]">
            <div className="font-medium text-trust-deep mb-2">Feature 3</div>
            <div className="text-sm text-secondary-600">Description</div>
          </div>
          <div className="bg-trust-light p-4 rounded-lg border-2 border-neutral-200 min-w-[150px]">
            <div className="font-medium text-trust-deep mb-2">Feature 4</div>
            <div className="text-sm text-secondary-600">Description</div>
          </div>
        </Stack>
      </div>
    </Stack>
  ),
};

/**
 * Accessibility features demonstration.
 * Stacks use semantic HTML and support:
 * - Proper document structure
 * - Keyboard navigation for interactive children
 * - Sufficient spacing for touch targets
 * - No reliance on specific visual order
 */
export const Accessibility: Story = {
  render: () => (
    <div className="bg-cream p-8">
      <Stack gap="md" className="bg-white rounded-lg shadow-md-premium p-6">
        <div>
          <h2 className="text-xl font-display font-medium text-trust-deep mb-3">
            Accessibility Features
          </h2>
          <p className="text-secondary-600">
            Stacks are designed with accessibility in mind:
          </p>
        </div>

        <Stack gap="md">
          <div className="flex items-start gap-3">
            <div className="flex-shrink-0 w-6 h-6 rounded-full bg-success-light text-success-hover flex items-center justify-center text-sm font-medium">
              ✓
            </div>
            <div>
              <h3 className="font-medium text-trust-deep">Semantic HTML</h3>
              <p className="text-sm text-secondary-600">
                Uses standard div elements with flexbox, no ARIA required
              </p>
            </div>
          </div>

          <div className="flex items-start gap-3">
            <div className="flex-shrink-0 w-6 h-6 rounded-full bg-success-light text-success-hover flex items-center justify-center text-sm font-medium">
              ✓
            </div>
            <div>
              <h3 className="font-medium text-trust-deep">
                Logical Source Order
              </h3>
              <p className="text-sm text-secondary-600">
                Visual order matches DOM order for screen readers
              </p>
            </div>
          </div>

          <div className="flex items-start gap-3">
            <div className="flex-shrink-0 w-6 h-6 rounded-full bg-success-light text-success-hover flex items-center justify-center text-sm font-medium">
              ✓
            </div>
            <div>
              <h3 className="font-medium text-trust-deep">
                Touch-Friendly Spacing
              </h3>
              <p className="text-sm text-secondary-600">
                Gap options ensure adequate spacing for touch targets (minimum
                44x44px)
              </p>
            </div>
          </div>

          <div className="flex items-start gap-3">
            <div className="flex-shrink-0 w-6 h-6 rounded-full bg-success-light text-success-hover flex items-center justify-center text-sm font-medium">
              ✓
            </div>
            <div>
              <h3 className="font-medium text-trust-deep">
                Responsive & Flexible
              </h3>
              <p className="text-sm text-secondary-600">
                Works across all viewport sizes and zoom levels
              </p>
            </div>
          </div>

          <div className="flex items-start gap-3">
            <div className="flex-shrink-0 w-6 h-6 rounded-full bg-success-light text-success-hover flex items-center justify-center text-sm font-medium">
              ✓
            </div>
            <div>
              <h3 className="font-medium text-trust-deep">
                No Motion Dependencies
              </h3>
              <p className="text-sm text-secondary-600">
                Pure layout component with no animations (respects
                prefers-reduced-motion)
              </p>
            </div>
          </div>
        </Stack>
      </Stack>
    </div>
  ),
  parameters: {
    docs: {
      description: {
        story:
          'Stacks follow accessibility best practices with semantic HTML, logical source order, and consideration for all users regardless of device or ability.',
      },
    },
  },
};
