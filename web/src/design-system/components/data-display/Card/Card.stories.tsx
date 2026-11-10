/**
 * Card Component Stories
 *
 * Demonstrates all variants and use cases of the Card component.
 */

import type { Meta, StoryObj } from '@storybook/react';
import { Card } from './Card';
import { Button } from '@design-system/components/primitives/Button';
import { Badge } from '@design-system/components/primitives/Badge';
import { Avatar } from '@design-system/components/primitives/Avatar';
import { Stack } from '@design-system/components/layout/Stack';

const meta = {
  title: 'Data Display/Card',
  component: Card,
  parameters: {
    layout: 'padded',
    docs: {
      description: {
        component:
          'Flexible container component for displaying grouped content with consistent styling. Supports headers, footers, various padding options, and interactive states.',
      },
    },
  },
  tags: ['autodocs'],
  argTypes: {
    padding: {
      control: 'select',
      options: ['none', 'compact', 'default', 'spacious'],
      description: 'Padding size for the card',
    },
    border: {
      control: 'select',
      options: ['subtle', 'bordered'],
      description: 'Border variant',
    },
    hover: {
      control: 'select',
      options: ['none', 'lift'],
      description: 'Hover effect',
    },
    backgroundColor: {
      control: 'select',
      options: ['white', 'neutral-50', 'blue-50'],
      description: 'Background color',
    },
    clickable: {
      control: 'boolean',
      description: 'Makes the card interactive with cursor pointer',
    },
    divider: {
      control: 'boolean',
      description: 'Show dividers between header, body, and footer',
    },
  },
} satisfies Meta<typeof Card>;

export default meta;
type Story = StoryObj<typeof meta>;

/**
 * Default card with simple content and medium padding.
 */
export const Default: Story = {
  args: {
    children: (
      <Stack gap="sm">
        <h3 className="text-lg font-medium text-neutral-900">Simple Card</h3>
        <p className="text-neutral-700">
          This is a simple card with default styling. It uses medium padding,
          subtle border, and white background.
        </p>
      </Stack>
    ),
  },
};

/**
 * Card with header section.
 */
export const WithHeader: Story = {
  args: {
    header: (
      <Stack gap="xs">
        <h2 className="text-xl font-medium text-neutral-900">Card Header</h2>
        <p className="text-sm text-neutral-600">
          Optional subtitle or metadata
        </p>
      </Stack>
    ),
    children: (
      <p className="text-neutral-700">
        This card has a header section that is visually separated from the main
        content area. Perfect for titled sections or labeled containers.
      </p>
    ),
  },
};

/**
 * Card with footer section.
 */
export const WithFooter: Story = {
  args: {
    children: (
      <Stack gap="sm">
        <h3 className="text-lg font-medium text-neutral-900">
          Card with Footer
        </h3>
        <p className="text-neutral-700">
          This card demonstrates the footer area, commonly used for actions or
          additional metadata.
        </p>
      </Stack>
    ),
    footer: (
      <Stack direction="row" gap="sm" justify="end">
        <Button variant="ghost" size="sm">
          Cancel
        </Button>
        <Button variant="primary" size="sm">
          Confirm
        </Button>
      </Stack>
    ),
  },
};

/**
 * Card with header and footer showing complete structure.
 */
export const WithHeaderAndFooter: Story = {
  args: {
    header: (
      <Stack direction="row" justify="space-between" align="center">
        <h2 className="text-xl font-medium text-neutral-900">Complete Card</h2>
        <Badge variant="info" size="sm">
          New
        </Badge>
      </Stack>
    ),
    children: (
      <Stack gap="sm">
        <p className="text-neutral-700">
          This card demonstrates the complete structure with both header and
          footer sections. Use this pattern for complex card layouts that need
          clear separation of title, content, and actions.
        </p>
        <ul className="list-disc list-inside text-neutral-600 text-sm space-y-1">
          <li>Header for title and metadata</li>
          <li>Body for main content</li>
          <li>Footer for actions or supplementary info</li>
        </ul>
      </Stack>
    ),
    footer: (
      <Stack direction="row" justify="space-between" align="center">
        <span className="text-sm text-neutral-600">
          Last updated: 2 hours ago
        </span>
        <Stack direction="row" gap="sm">
          <Button variant="outline" size="sm">
            Edit
          </Button>
          <Button variant="primary" size="sm">
            View Details
          </Button>
        </Stack>
      </Stack>
    ),
  },
};

/**
 * Padding variants: none, sm, md, lg.
 */
export const PaddingVariants: Story = {
  args: { children: null },
  render: () => (
    <Stack gap="lg">
      <Card padding="none">
        <div className="p-4 bg-blue-50 text-blue-900 rounded">
          <p className="font-medium">Padding: none</p>
          <p className="text-sm">No padding - full control to content</p>
        </div>
      </Card>

      <Card padding="compact">
        <Stack gap="xs">
          <p className="font-medium text-neutral-900">
            Padding: compact (16px)
          </p>
          <p className="text-sm text-neutral-600">
            Compact cards for tight layouts
          </p>
        </Stack>
      </Card>

      <Card padding="default">
        <Stack gap="xs">
          <p className="font-medium text-neutral-900">
            Padding: default (24px)
          </p>
          <p className="text-sm text-neutral-600">Standard spacing - default</p>
        </Stack>
      </Card>

      <Card padding="spacious">
        <Stack gap="xs">
          <p className="font-medium text-neutral-900">
            Padding: spacious (32px)
          </p>
          <p className="text-sm text-neutral-600">
            Generous spacing for emphasis
          </p>
        </Stack>
      </Card>
    </Stack>
  ),
};

/**
 * Border variants: subtle ring and bordered.
 */
export const BorderVariants: Story = {
  args: { children: null },
  render: () => (
    <Stack gap="lg">
      <Card border="subtle">
        <Stack gap="sm">
          <h3 className="text-lg font-medium text-neutral-900">
            Subtle Border
          </h3>
          <p className="text-neutral-700">
            Uses a ring-based border (ring-1) for a softer, more refined
            appearance. This is the default style.
          </p>
        </Stack>
      </Card>

      <Card border="bordered">
        <Stack gap="sm">
          <h3 className="text-lg font-medium text-neutral-900">Bordered</h3>
          <p className="text-neutral-700">
            Uses a solid border for stronger definition and visual separation.
            Good for high-contrast layouts.
          </p>
        </Stack>
      </Card>
    </Stack>
  ),
};

/**
 * Hover lift effect demonstration.
 */
export const HoverLift: Story = {
  args: { children: null },
  render: () => (
    <Stack gap="lg">
      <Card hover="none">
        <Stack gap="sm">
          <h3 className="text-lg font-medium text-neutral-900">
            No Hover Effect
          </h3>
          <p className="text-neutral-700">
            Static card with no hover interaction. Use for non-interactive
            content displays.
          </p>
        </Stack>
      </Card>

      <Card hover="lift">
        <Stack gap="sm">
          <h3 className="text-lg font-medium text-neutral-900">
            Hover Lift Effect
          </h3>
          <p className="text-neutral-700">
            Hover over this card to see the lift effect. The card subtly
            elevates with increased shadow. Perfect for interactive cards.
          </p>
        </Stack>
      </Card>
    </Stack>
  ),
};

/**
 * Clickable interactive card with onClick handler.
 */
export const Clickable: Story = {
  args: {
    clickable: true,
    hover: 'lift',
    onClick: () => alert('Card clicked!'),
    children: (
      <Stack gap="sm">
        <h3 className="text-lg font-medium text-neutral-900">Clickable Card</h3>
        <p className="text-neutral-700">
          This entire card is clickable. Notice the cursor changes to pointer on
          hover. Click anywhere to trigger the action.
        </p>
        <p className="text-sm text-neutral-600">
          Supports keyboard navigation with Enter/Space keys.
        </p>
      </Stack>
    ),
  },
};

/**
 * Background color variants.
 */
export const BackgroundVariants: Story = {
  args: { children: null },
  render: () => (
    <Stack gap="lg">
      <Card backgroundColor="white">
        <Stack gap="sm">
          <h3 className="text-lg font-medium text-neutral-900">
            White Background
          </h3>
          <p className="text-neutral-700">
            Pure white background - the default. Best for most use cases and
            provides maximum contrast.
          </p>
        </Stack>
      </Card>

      <Card backgroundColor="neutral-50">
        <Stack gap="sm">
          <h3 className="text-lg font-medium text-neutral-900">
            Gray-50 Background
          </h3>
          <p className="text-neutral-700">
            Subtle gray background for secondary or less prominent cards.
            Creates visual hierarchy through background color.
          </p>
        </Stack>
      </Card>

      <Card backgroundColor="blue-50">
        <Stack gap="sm">
          <h3 className="text-lg font-medium text-neutral-900">
            Blue-50 Background
          </h3>
          <p className="text-neutral-700">
            Light blue background for highlighted or informational content.
            Draws attention without being too bold.
          </p>
        </Stack>
      </Card>
    </Stack>
  ),
};

/**
 * Cards with dividers between sections.
 */
export const WithDividers: Story = {
  args: { children: null },
  render: () => (
    <Stack gap="lg">
      <Card
        header={
          <h2 className="text-xl font-medium text-neutral-900">
            With Dividers
          </h2>
        }
        footer={
          <Stack direction="row" gap="sm" justify="end">
            <Button variant="outline" size="sm">
              Cancel
            </Button>
            <Button variant="primary" size="sm">
              Save
            </Button>
          </Stack>
        }
        divider
      >
        <p className="text-neutral-700">
          This card uses dividers to create clear visual separation between
          header, body, and footer sections. Great for structured content.
        </p>
      </Card>

      <Card
        header={
          <h2 className="text-xl font-medium text-neutral-900">
            Without Dividers
          </h2>
        }
        footer={
          <Stack direction="row" gap="sm" justify="end">
            <Button variant="outline" size="sm">
              Cancel
            </Button>
            <Button variant="primary" size="sm">
              Save
            </Button>
          </Stack>
        }
        divider={false}
      >
        <p className="text-neutral-700">
          This card omits dividers for a more seamless appearance. The sections
          flow together with spacing alone providing separation.
        </p>
      </Card>
    </Stack>
  ),
};

/**
 * Card with icon in header.
 */
export const WithIcon: Story = {
  args: {
    header: <h2 className="text-xl font-medium text-neutral-900">Settings</h2>,
    headerIcon: (
      <svg
        className="w-6 h-6"
        fill="none"
        stroke="currentColor"
        viewBox="0 0 24 24"
      >
        <path
          strokeLinecap="round"
          strokeLinejoin="round"
          strokeWidth={2}
          d="M10.325 4.317c.426-1.756 2.924-1.756 3.35 0a1.724 1.724 0 002.573 1.066c1.543-.94 3.31.826 2.37 2.37a1.724 1.724 0 001.065 2.572c1.756.426 1.756 2.924 0 3.35a1.724 1.724 0 00-1.066 2.573c.94 1.543-.826 3.31-2.37 2.37a1.724 1.724 0 00-2.572 1.065c-.426 1.756-2.924 1.756-3.35 0a1.724 1.724 0 00-2.573-1.066c-1.543.94-3.31-.826-2.37-2.37a1.724 1.724 0 00-1.065-2.572c-1.756-.426-1.756-2.924 0-3.35a1.724 1.724 0 001.066-2.573c-.94-1.543.826-3.31 2.37-2.37.996.608 2.296.07 2.572-1.065z"
        />
        <path
          strokeLinecap="round"
          strokeLinejoin="round"
          strokeWidth={2}
          d="M15 12a3 3 0 11-6 0 3 3 0 016 0z"
        />
      </svg>
    ),
    divider: true,
    children: (
      <Stack gap="sm">
        <p className="text-neutral-700">
          Cards can include an icon in the header section. The icon is
          automatically positioned before the header content with proper
          spacing.
        </p>
        <div className="space-y-2">
          <label className="flex items-center gap-2 text-sm text-neutral-700">
            <input type="checkbox" className="rounded" />
            Enable notifications
          </label>
          <label className="flex items-center gap-2 text-sm text-neutral-700">
            <input type="checkbox" className="rounded" />
            Auto-save changes
          </label>
        </div>
      </Stack>
    ),
  },
};

/**
 * Different content sizes and complexity.
 */
export const SizingVariants: Story = {
  args: { children: null },
  render: () => (
    <div className="grid grid-cols-1 md:grid-cols-3 gap-6">
      <Card padding="compact">
        <Stack gap="xs">
          <h4 className="text-sm font-medium text-neutral-900">Compact</h4>
          <p className="text-xs text-neutral-600">
            Small card with minimal content
          </p>
        </Stack>
      </Card>

      <Card padding="default">
        <Stack gap="sm">
          <h3 className="text-base font-medium text-neutral-900">Standard</h3>
          <p className="text-sm text-neutral-600">
            Regular card with moderate amount of content and spacing
          </p>
        </Stack>
      </Card>

      <Card padding="spacious">
        <Stack gap="md">
          <h2 className="text-lg font-medium text-neutral-900">Expanded</h2>
          <p className="text-base text-neutral-600">
            Large card with generous spacing and more detailed content. Ideal
            for feature highlights or important information.
          </p>
        </Stack>
      </Card>
    </div>
  ),
};

/**
 * Multiple cards in a grid layout.
 */
export const GridLayout: Story = {
  args: { children: null },
  render: () => (
    <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-6">
      {[1, 2, 3, 4, 5, 6].map((num) => (
        <Card
          key={num}
          hover="lift"
          clickable
          onClick={() => alert(`Card ${num} clicked`)}
        >
          <Stack gap="sm">
            <h3 className="text-lg font-medium text-neutral-900">Card {num}</h3>
            <p className="text-neutral-700">
              This is card number {num} in a responsive grid layout. Cards adapt
              to 1, 2, or 3 columns based on screen size.
            </p>
            <div className="flex gap-2 mt-2">
              <Badge variant="primary" size="sm">
                Tag
              </Badge>
              <Badge variant="success" size="sm">
                Active
              </Badge>
            </div>
          </Stack>
        </Card>
      ))}
    </div>
  ),
};

/**
 * Real-world example: Consent application card showing service/scope.
 */
export const ConsentApplicationCard: Story = {
  args: { children: null },
  render: () => (
    <div className="max-w-2xl">
      <Card
        header={
          <Stack direction="row" gap="md" align="center">
            <Avatar initials="AP" size="lg" />
            <Stack gap="xs">
              <h2 className="text-xl font-medium text-neutral-900">
                Analytics Platform
              </h2>
              <p className="text-sm text-neutral-600">analytics.example.com</p>
            </Stack>
          </Stack>
        }
        footer={
          <Stack gap="md">
            <div className="flex items-center gap-2 text-sm text-neutral-600">
              <svg
                className="w-4 h-4"
                fill="none"
                stroke="currentColor"
                viewBox="0 0 24 24"
              >
                <path
                  strokeLinecap="round"
                  strokeLinejoin="round"
                  strokeWidth={2}
                  d="M13 16h-1v-4h-1m1-4h.01M21 12a9 9 0 11-18 0 9 9 0 0118 0z"
                />
              </svg>
              <span>Your consent will be valid for 90 days</span>
            </div>
            <Stack direction="row" gap="sm" justify="end">
              <Button variant="outline" size="md" fullWidth>
                Deny
              </Button>
              <Button variant="primary" size="md" fullWidth>
                Allow Access
              </Button>
            </Stack>
          </Stack>
        }
        divider
        padding="spacious"
      >
        <Stack gap="md">
          <div>
            <h3 className="text-base font-medium text-neutral-900 mb-2">
              Requested Permissions
            </h3>
            <Stack gap="sm">
              <div className="flex items-start gap-3">
                <svg
                  className="w-5 h-5 text-success-primary mt-0.5 flex-shrink-0"
                  fill="none"
                  stroke="currentColor"
                  viewBox="0 0 24 24"
                >
                  <path
                    strokeLinecap="round"
                    strokeLinejoin="round"
                    strokeWidth={2}
                    d="M5 13l4 4L19 7"
                  />
                </svg>
                <div>
                  <p className="text-sm font-medium text-neutral-900">
                    Read your profile information
                  </p>
                  <p className="text-xs text-neutral-600">
                    Access your name, email, and profile picture
                  </p>
                </div>
              </div>

              <div className="flex items-start gap-3">
                <svg
                  className="w-5 h-5 text-success-primary mt-0.5 flex-shrink-0"
                  fill="none"
                  stroke="currentColor"
                  viewBox="0 0 24 24"
                >
                  <path
                    strokeLinecap="round"
                    strokeLinejoin="round"
                    strokeWidth={2}
                    d="M5 13l4 4L19 7"
                  />
                </svg>
                <div>
                  <p className="text-sm font-medium text-neutral-900">
                    Access your organization data
                  </p>
                  <p className="text-xs text-neutral-600">
                    View organization membership and roles
                  </p>
                </div>
              </div>

              <div className="flex items-start gap-3">
                <svg
                  className="w-5 h-5 text-success-primary mt-0.5 flex-shrink-0"
                  fill="none"
                  stroke="currentColor"
                  viewBox="0 0 24 24"
                >
                  <path
                    strokeLinecap="round"
                    strokeLinejoin="round"
                    strokeWidth={2}
                    d="M5 13l4 4L19 7"
                  />
                </svg>
                <div>
                  <p className="text-sm font-medium text-neutral-900">
                    Read analytics reports
                  </p>
                  <p className="text-xs text-neutral-600">
                    Access usage statistics and reports
                  </p>
                </div>
              </div>
            </Stack>
          </div>

          <div className="bg-blue-50 -mx-6 px-6 py-3 rounded">
            <p className="text-sm text-neutral-700">
              <span className="font-medium">Privacy notice:</span> We will never
              share your data with third parties without your explicit consent.
            </p>
          </div>
        </Stack>
      </Card>
    </div>
  ),
};

/**
 * User profile card example.
 */
export const UserProfileCard: Story = {
  args: { children: null },
  render: () => (
    <div className="max-w-md">
      <Card hover="lift" padding="spacious">
        <Stack gap="md">
          <Stack direction="row" gap="md" align="center">
            <Avatar initials="JD" size="xl" />
            <Stack gap="xs">
              <h2 className="text-xl font-medium text-neutral-900">John Doe</h2>
              <p className="text-sm text-neutral-600">Senior Developer</p>
              <div className="flex gap-2">
                <Badge variant="success" size="sm">
                  Active
                </Badge>
                <Badge variant="info" size="sm">
                  Verified
                </Badge>
              </div>
            </Stack>
          </Stack>

          <div className="border-t border-neutral-200 pt-4">
            <Stack gap="sm">
              <div className="flex justify-between text-sm">
                <span className="text-neutral-600">Email</span>
                <span className="text-neutral-900 font-medium">
                  john.doe@example.com
                </span>
              </div>
              <div className="flex justify-between text-sm">
                <span className="text-neutral-600">Department</span>
                <span className="text-neutral-900 font-medium">
                  Engineering
                </span>
              </div>
              <div className="flex justify-between text-sm">
                <span className="text-neutral-600">Location</span>
                <span className="text-neutral-900 font-medium">
                  San Francisco, CA
                </span>
              </div>
            </Stack>
          </div>

          <Stack direction="row" gap="sm">
            <Button variant="primary" size="sm" fullWidth>
              Send Message
            </Button>
            <Button variant="outline" size="sm" fullWidth>
              View Profile
            </Button>
          </Stack>
        </Stack>
      </Card>
    </div>
  ),
};
