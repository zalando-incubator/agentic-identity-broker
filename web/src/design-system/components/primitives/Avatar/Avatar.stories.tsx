/**
 * Avatar Component Stories
 *
 * Demonstrates all Avatar variants, sizes, and configurations.
 * Used for visual regression testing and component exploration.
 */

import type { Meta, StoryObj } from '@storybook/react';
import { Avatar } from './Avatar';

const meta = {
  title: 'Design System/Primitives/Avatar',
  component: Avatar,
  parameters: {
    layout: 'centered',
    docs: {
      description: {
        component:
          'Avatar component for displaying user or agent profile pictures, initials, or fallback icons. Supports multiple sizes, shapes, and status indicators.',
      },
    },
  },
  tags: ['autodocs'],
  argTypes: {
    size: {
      control: 'select',
      options: ['xs', 'sm', 'md', 'lg', 'xl'],
      description: 'The size of the avatar',
    },
    shape: {
      control: 'select',
      options: ['circle', 'rounded', 'square'],
      description: 'The shape of the avatar',
    },
    status: {
      control: 'select',
      options: ['online', 'offline', 'away', 'busy'],
      description: 'Status indicator',
    },
    src: {
      control: 'text',
      description: 'Image source URL',
    },
    alt: {
      control: 'text',
      description: 'Alt text for the image',
    },
    initials: {
      control: 'text',
      description: 'Initials to display (1-2 characters)',
    },
  },
} satisfies Meta<typeof Avatar>;

export default meta;
type Story = StoryObj<typeof meta>;

/**
 * Default avatar with fallback icon
 */
export const Default: Story = {
  args: {},
};

/**
 * All avatar sizes from extra small to extra large
 */
export const Sizes: Story = {
  render: () => (
    <div className="flex items-end gap-4">
      <div className="flex flex-col items-center gap-2">
        <Avatar size="xs" initials="XS" />
        <span className="text-xs text-neutral-600">Extra Small</span>
      </div>
      <div className="flex flex-col items-center gap-2">
        <Avatar size="sm" initials="SM" />
        <span className="text-xs text-neutral-600">Small</span>
      </div>
      <div className="flex flex-col items-center gap-2">
        <Avatar size="md" initials="MD" />
        <span className="text-xs text-neutral-600">Medium</span>
      </div>
      <div className="flex flex-col items-center gap-2">
        <Avatar size="lg" initials="LG" />
        <span className="text-xs text-neutral-600">Large</span>
      </div>
      <div className="flex flex-col items-center gap-2">
        <Avatar size="xl" initials="XL" />
        <span className="text-xs text-neutral-600">Extra Large</span>
      </div>
    </div>
  ),
};

/**
 * Avatar shapes: circle, rounded, square
 */
export const Shapes: Story = {
  render: () => (
    <div className="flex items-center gap-4">
      <div className="flex flex-col items-center gap-2">
        <Avatar shape="circle" initials="JD" />
        <span className="text-xs text-neutral-600">Circle</span>
      </div>
      <div className="flex flex-col items-center gap-2">
        <Avatar shape="rounded" initials="JD" />
        <span className="text-xs text-neutral-600">Rounded</span>
      </div>
      <div className="flex flex-col items-center gap-2">
        <Avatar shape="square" initials="JD" />
        <span className="text-xs text-neutral-600">Square</span>
      </div>
    </div>
  ),
};

/**
 * Avatars displaying user initials
 */
export const WithInitials: Story = {
  render: () => (
    <div className="flex items-center gap-4">
      <Avatar initials="JD" alt="John Doe" />
      <Avatar initials="AS" alt="Alice Smith" />
      <Avatar initials="BJ" alt="Bob Johnson" />
      <Avatar initials="MK" alt="Mary King" />
    </div>
  ),
};

/**
 * Avatars with status indicators
 */
export const WithStatus: Story = {
  render: () => (
    <div className="flex items-center gap-6">
      <div className="flex flex-col items-center gap-2">
        <Avatar initials="JD" status="online" />
        <span className="text-xs text-neutral-600">Online</span>
      </div>
      <div className="flex flex-col items-center gap-2">
        <Avatar initials="AS" status="away" />
        <span className="text-xs text-neutral-600">Away</span>
      </div>
      <div className="flex flex-col items-center gap-2">
        <Avatar initials="BJ" status="busy" />
        <span className="text-xs text-neutral-600">Busy</span>
      </div>
      <div className="flex flex-col items-center gap-2">
        <Avatar initials="MK" status="offline" />
        <span className="text-xs text-neutral-600">Offline</span>
      </div>
    </div>
  ),
};

/**
 * Avatars with images
 */
export const WithImage: Story = {
  render: () => (
    <div className="flex items-center gap-4">
      <Avatar
        src="https://images.unsplash.com/photo-1472099645785-5658abf4ff4e?w=100&h=100&fit=crop"
        alt="John Doe"
      />
      <Avatar
        src="https://images.unsplash.com/photo-1494790108377-be9c29b29330?w=100&h=100&fit=crop"
        alt="Alice Smith"
      />
      <Avatar
        src="https://images.unsplash.com/photo-1500648767791-00dcc994a43e?w=100&h=100&fit=crop"
        alt="Bob Johnson"
      />
      <Avatar
        src="https://images.unsplash.com/photo-1438761681033-6461ffad8d80?w=100&h=100&fit=crop"
        alt="Mary King"
      />
    </div>
  ),
};

/**
 * Avatar with broken image fallback to initials
 */
export const ImageFallback: Story = {
  render: () => (
    <div className="flex items-center gap-4">
      <div className="flex flex-col items-center gap-2">
        <Avatar src="/broken-image.jpg" initials="JD" alt="John Doe" />
        <span className="text-xs text-neutral-600">Fallback to initials</span>
      </div>
      <div className="flex flex-col items-center gap-2">
        <Avatar src="/broken-image.jpg" alt="Alice Smith" />
        <span className="text-xs text-neutral-600">Fallback to icon</span>
      </div>
    </div>
  ),
};

/**
 * Avatar with custom fallback icon
 */
export const CustomFallbackIcon: Story = {
  render: () => (
    <div className="flex items-center gap-4">
      <Avatar
        fallbackIcon={
          <svg
            fill="currentColor"
            viewBox="0 0 20 20"
            className="w-full h-full"
          >
            <path
              fillRule="evenodd"
              d="M6.267 3.455a3.066 3.066 0 001.745-.723 3.066 3.066 0 013.976 0 3.066 3.066 0 001.745.723 3.066 3.066 0 012.812 2.812c.051.643.304 1.254.723 1.745a3.066 3.066 0 010 3.976 3.066 3.066 0 00-.723 1.745 3.066 3.066 0 01-2.812 2.812 3.066 3.066 0 00-1.745.723 3.066 3.066 0 01-3.976 0 3.066 3.066 0 00-1.745-.723 3.066 3.066 0 01-2.812-2.812 3.066 3.066 0 00-.723-1.745 3.066 3.066 0 010-3.976 3.066 3.066 0 00.723-1.745 3.066 3.066 0 012.812-2.812zm7.44 5.252a1 1 0 00-1.414-1.414L9 10.586 7.707 9.293a1 1 0 00-1.414 1.414l2 2a1 1 0 001.414 0l4-4z"
              clipRule="evenodd"
            />
          </svg>
        }
        alt="Verified agent"
      />
      <Avatar
        fallbackIcon={
          <svg
            fill="currentColor"
            viewBox="0 0 20 20"
            className="w-full h-full"
          >
            <path d="M2 10.5a1.5 1.5 0 113 0v6a1.5 1.5 0 01-3 0v-6zM6 10.333v5.43a2 2 0 001.106 1.79l.05.025A4 4 0 008.943 18h5.416a2 2 0 001.962-1.608l1.2-6A2 2 0 0015.56 8H12V4a2 2 0 00-2-2 1 1 0 00-1 1v.667a4 4 0 01-.8 2.4L6.8 7.933a4 4 0 00-.8 2.4z" />
          </svg>
        }
        alt="AI Agent"
        shape="rounded"
      />
    </div>
  ),
};

/**
 * Avatar groups showing multiple users
 */
export const AvatarGroup: Story = {
  render: () => (
    <div className="flex flex-col gap-6">
      {/* Stacked avatars */}
      <div className="space-y-2">
        <h3 className="text-sm font-medium text-neutral-700">Stacked Group</h3>
        <div className="flex -space-x-2">
          <Avatar
            src="https://images.unsplash.com/photo-1472099645785-5658abf4ff4e?w=100&h=100&fit=crop"
            alt="User 1"
            className="ring-2 ring-white"
          />
          <Avatar
            src="https://images.unsplash.com/photo-1494790108377-be9c29b29330?w=100&h=100&fit=crop"
            alt="User 2"
            className="ring-2 ring-white"
          />
          <Avatar
            src="https://images.unsplash.com/photo-1500648767791-00dcc994a43e?w=100&h=100&fit=crop"
            alt="User 3"
            className="ring-2 ring-white"
          />
          <Avatar initials="+5" className="ring-2 ring-white" />
        </div>
      </div>

      {/* Spaced avatars */}
      <div className="space-y-2">
        <h3 className="text-sm font-medium text-neutral-700">Spaced Group</h3>
        <div className="flex gap-2">
          <Avatar initials="JD" status="online" size="sm" />
          <Avatar initials="AS" status="away" size="sm" />
          <Avatar initials="BJ" status="busy" size="sm" />
          <Avatar initials="MK" status="offline" size="sm" />
        </div>
      </div>
    </div>
  ),
};

/**
 * Real-world use cases
 */
export const UseCases: Story = {
  render: () => (
    <div className="flex flex-col gap-6 p-6">
      {/* User profile card */}
      <div className="space-y-2">
        <h3 className="text-sm font-medium text-neutral-700">Profile Card</h3>
        <div className="flex items-center gap-3 p-4 bg-white border border-neutral-200 rounded-lg">
          <Avatar
            src="https://images.unsplash.com/photo-1472099645785-5658abf4ff4e?w=100&h=100&fit=crop"
            alt="John Doe"
            status="online"
            size="lg"
          />
          <div>
            <h4 className="text-sm font-medium text-neutral-900">John Doe</h4>
            <p className="text-xs text-neutral-500">john.doe@example.com</p>
          </div>
        </div>
      </div>

      {/* Agent card */}
      <div className="space-y-2">
        <h3 className="text-sm font-medium text-neutral-700">AI Agent</h3>
        <div className="flex items-center gap-3 p-4 bg-white border border-neutral-200 rounded-lg">
          <Avatar initials="AI" shape="rounded" status="online" size="lg" />
          <div>
            <h4 className="text-sm font-medium text-neutral-900">
              Email Assistant
            </h4>
            <p className="text-xs text-neutral-500">
              Active · 3 permissions granted
            </p>
          </div>
        </div>
      </div>

      {/* Comment thread */}
      <div className="space-y-2">
        <h3 className="text-sm font-medium text-neutral-700">Comment Thread</h3>
        <div className="space-y-3">
          <div className="flex gap-3">
            <Avatar initials="JD" size="sm" />
            <div className="flex-1">
              <p className="text-xs font-medium text-neutral-900">John Doe</p>
              <p className="text-xs text-neutral-600">
                This looks great! When can we ship?
              </p>
            </div>
          </div>
          <div className="flex gap-3">
            <Avatar initials="AS" size="sm" />
            <div className="flex-1">
              <p className="text-xs font-medium text-neutral-900">
                Alice Smith
              </p>
              <p className="text-xs text-neutral-600">
                Ready to go live tomorrow.
              </p>
            </div>
          </div>
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
    initials: 'JD',
    size: 'md',
    shape: 'circle',
    status: 'online',
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
          Avatars with proper alt text
        </h3>
        <div className="flex gap-4">
          <Avatar
            src="https://images.unsplash.com/photo-1472099645785-5658abf4ff4e?w=100&h=100&fit=crop"
            alt="John Doe, Senior Developer"
          />
          <Avatar
            initials="AS"
            alt="Alice Smith, Product Manager"
            status="online"
          />
          <Avatar alt="Anonymous user" />
        </div>
      </div>

      <div className="space-y-2">
        <h3 className="text-sm font-medium text-neutral-700">
          Status indicators with aria-label
        </h3>
        <div className="flex gap-4">
          <Avatar initials="JD" status="online" alt="John Doe" />
          <Avatar initials="AS" status="away" alt="Alice Smith" />
          <Avatar initials="BJ" status="busy" alt="Bob Johnson" />
          <Avatar initials="MK" status="offline" alt="Mary King" />
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
