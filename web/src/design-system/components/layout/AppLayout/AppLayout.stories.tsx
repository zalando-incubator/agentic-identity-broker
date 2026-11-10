/**
 * AppLayout Stories
 *
 * Demonstrates all variants and use cases for the AppLayout component.
 * Includes responsive patterns, collapsible sidebars, and real-world examples.
 */

import type { Meta, StoryObj } from '@storybook/react';
import { AppLayout } from './AppLayout';
import { Stack } from '../Stack';
import { Container } from '../Container';
import { Button } from '../../primitives/Button';

const meta = {
  title: 'Layout/AppLayout',
  component: AppLayout,
  parameters: {
    layout: 'fullscreen',
    docs: {
      description: {
        component:
          'AppLayout is a flexible full-page layout component supporting header, sidebar, content, and footer sections. Features responsive design, collapsible sidebars, sticky positioning, and mobile drawer patterns.',
      },
    },
  },
  tags: ['autodocs'],
  argTypes: {
    header: {
      control: false,
      description: 'Header content (optional)',
    },
    sidebar: {
      control: false,
      description: 'Sidebar content (optional)',
    },
    sidebarPosition: {
      control: 'radio',
      options: ['left', 'right'],
      description: 'Sidebar position',
    },
    sidebarWidth: {
      control: 'radio',
      options: ['sm', 'md', 'lg'],
      description: 'Sidebar width variant',
    },
    sidebarCollapsible: {
      control: 'boolean',
      description: 'Enable collapsible sidebar with toggle button',
    },
    stickyHeader: {
      control: 'boolean',
      description: 'Make header sticky at top of viewport',
    },
    stickyFooter: {
      control: 'boolean',
      description: 'Make footer sticky at bottom of viewport',
    },
    footer: {
      control: false,
      description: 'Footer content (optional)',
    },
    mobileDrawer: {
      control: 'boolean',
      description:
        'Use drawer overlay pattern on mobile instead of hiding sidebar',
    },
  },
} satisfies Meta<typeof AppLayout>;

export default meta;
type Story = StoryObj<typeof meta>;

// Sample components for stories
const SampleHeader = () => (
  <div className="flex items-center justify-between px-6 py-4">
    <div className="flex items-center gap-3">
      <div className="w-8 h-8 bg-trust-deep rounded-md" />
      <h1 className="text-xl font-medium text-trust-deep">
        Agentic Identity Broker
      </h1>
    </div>
    <nav className="flex items-center gap-4">
      <a href="#" className="text-sm font-medium text-trust hover:text-trust">
        Dashboard
      </a>
      <a href="#" className="text-sm font-medium text-trust hover:text-trust">
        Settings
      </a>
      <Button size="sm" variant="outline">
        Sign out
      </Button>
    </nav>
  </div>
);

const SampleSidebar = () => (
  <nav className="p-4">
    <Stack gap="sm">
      <a
        href="#"
        className="px-4 py-2 rounded-md text-sm font-medium bg-trust text-white"
      >
        Home
      </a>
      <a
        href="#"
        className="px-4 py-2 rounded-md text-sm font-medium text-trust hover:bg-neutral-200 transition-colors"
      >
        Applications
      </a>
      <a
        href="#"
        className="px-4 py-2 rounded-md text-sm font-medium text-trust hover:bg-neutral-200 transition-colors"
      >
        Sessions
      </a>
      <a
        href="#"
        className="px-4 py-2 rounded-md text-sm font-medium text-trust hover:bg-neutral-200 transition-colors"
      >
        Audit Logs
      </a>
      <a
        href="#"
        className="px-4 py-2 rounded-md text-sm font-medium text-trust hover:bg-neutral-200 transition-colors"
      >
        Settings
      </a>
    </Stack>
  </nav>
);

const SampleContent = () => (
  <Container padding="lg" size="xl">
    <Stack gap="lg">
      <div>
        <h2 className="text-3xl font-medium text-trust-deep mb-2">Dashboard</h2>
        <p className="text-neutral-600">
          Welcome to your identity management dashboard.
        </p>
      </div>

      <div className="grid grid-cols-1 md:grid-cols-3 gap-6">
        <div className="p-6 bg-white border border-neutral-200 rounded-lg shadow-sm">
          <h3 className="text-lg font-medium text-trust-deep mb-2">
            Active Sessions
          </h3>
          <p className="text-3xl font-medium text-trust">127</p>
        </div>
        <div className="p-6 bg-white border border-neutral-200 rounded-lg shadow-sm">
          <h3 className="text-lg font-medium text-trust-deep mb-2">
            Applications
          </h3>
          <p className="text-3xl font-medium text-trust">12</p>
        </div>
        <div className="p-6 bg-white border border-neutral-200 rounded-lg shadow-sm">
          <h3 className="text-lg font-medium text-trust-deep mb-2">
            Active Users
          </h3>
          <p className="text-3xl font-medium text-trust">89</p>
        </div>
      </div>

      <div className="p-6 bg-white border border-neutral-200 rounded-lg shadow-sm">
        <h3 className="text-lg font-medium text-trust-deep mb-4">
          Recent Activity
        </h3>
        <Stack gap="md">
          <div className="flex justify-between items-center py-2 border-b border-neutral-100">
            <div>
              <p className="font-medium text-trust-deep">User logged in</p>
              <p className="text-sm text-neutral-600">john@example.com</p>
            </div>
            <span className="text-sm text-neutral-500">2 minutes ago</span>
          </div>
          <div className="flex justify-between items-center py-2 border-b border-neutral-100">
            <div>
              <p className="font-medium text-trust-deep">Consent granted</p>
              <p className="text-sm text-neutral-600">jane@example.com</p>
            </div>
            <span className="text-sm text-neutral-500">15 minutes ago</span>
          </div>
          <div className="flex justify-between items-center py-2 border-b border-neutral-100">
            <div>
              <p className="font-medium text-trust-deep">Session expired</p>
              <p className="text-sm text-neutral-600">bob@example.com</p>
            </div>
            <span className="text-sm text-neutral-500">1 hour ago</span>
          </div>
        </Stack>
      </div>
    </Stack>
  </Container>
);

const SampleFooter = () => (
  <div className="flex items-center justify-between px-6 py-4">
    <p className="text-sm text-neutral-600">
      © 2025 Agentic Identity Broker. All rights reserved.
    </p>
    <div className="flex items-center gap-4">
      <a href="#" className="text-sm text-neutral-600 hover:text-trust">
        Privacy Policy
      </a>
      <a href="#" className="text-sm text-neutral-600 hover:text-trust">
        Terms of Service
      </a>
      <a href="#" className="text-sm text-neutral-600 hover:text-trust">
        Documentation
      </a>
    </div>
  </div>
);

// Story: Default with all sections
export const Default: Story = {
  args: {
    header: <SampleHeader />,
    sidebar: <SampleSidebar />,
    footer: <SampleFooter />,
    children: <SampleContent />,
  },
};

// Story: Header only
export const HeaderOnly: Story = {
  args: {
    header: <SampleHeader />,
    children: <SampleContent />,
  },
};

// Story: Sidebar only
export const SidebarOnly: Story = {
  args: {
    sidebar: <SampleSidebar />,
    children: <SampleContent />,
  },
};

// Story: Sidebar on right
export const SidebarRight: Story = {
  args: {
    header: <SampleHeader />,
    sidebar: (
      <div className="p-4">
        <Stack gap="md">
          <h3 className="text-sm font-medium text-trust-deep">Filters</h3>
          <div className="space-y-2">
            <label className="flex items-center gap-2">
              <input type="checkbox" className="rounded" />
              <span className="text-sm">Active</span>
            </label>
            <label className="flex items-center gap-2">
              <input type="checkbox" className="rounded" />
              <span className="text-sm">Pending</span>
            </label>
            <label className="flex items-center gap-2">
              <input type="checkbox" className="rounded" />
              <span className="text-sm">Expired</span>
            </label>
          </div>
          <Button size="sm" fullWidth>
            Apply Filters
          </Button>
        </Stack>
      </div>
    ),
    sidebarPosition: 'right',
    children: <SampleContent />,
  },
};

// Story: Small sidebar
export const SidebarSmall: Story = {
  args: {
    header: <SampleHeader />,
    sidebar: (
      <nav className="p-2">
        <Stack gap="xs">
          <button className="p-2 rounded-md bg-trust text-white w-full flex items-center justify-center">
            <svg
              className="w-5 h-5"
              fill="none"
              stroke="currentColor"
              viewBox="0 0 24 24"
            >
              <path
                strokeLinecap="round"
                strokeLinejoin="round"
                strokeWidth={2}
                d="M3 12l2-2m0 0l7-7 7 7M5 10v10a1 1 0 001 1h3m10-11l2 2m-2-2v10a1 1 0 01-1 1h-3m-6 0a1 1 0 001-1v-4a1 1 0 011-1h2a1 1 0 011 1v4a1 1 0 001 1m-6 0h6"
              />
            </svg>
          </button>
          <button className="p-2 rounded-md hover:bg-neutral-200 w-full flex items-center justify-center">
            <svg
              className="w-5 h-5"
              fill="none"
              stroke="currentColor"
              viewBox="0 0 24 24"
            >
              <path
                strokeLinecap="round"
                strokeLinejoin="round"
                strokeWidth={2}
                d="M9 19v-6a2 2 0 00-2-2H5a2 2 0 00-2 2v6a2 2 0 002 2h2a2 2 0 002-2zm0 0V9a2 2 0 012-2h2a2 2 0 012 2v10m-6 0a2 2 0 002 2h2a2 2 0 002-2m0 0V5a2 2 0 012-2h2a2 2 0 012 2v14a2 2 0 01-2 2h-2a2 2 0 01-2-2z"
              />
            </svg>
          </button>
          <button className="p-2 rounded-md hover:bg-neutral-200 w-full flex items-center justify-center">
            <svg
              className="w-5 h-5"
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
          </button>
        </Stack>
      </nav>
    ),
    sidebarWidth: 'sm',
    children: <SampleContent />,
  },
};

// Story: Medium sidebar (default)
export const SidebarMedium: Story = {
  args: {
    header: <SampleHeader />,
    sidebar: <SampleSidebar />,
    sidebarWidth: 'md',
    children: <SampleContent />,
  },
};

// Story: Large sidebar
export const SidebarLarge: Story = {
  args: {
    header: <SampleHeader />,
    sidebar: (
      <nav className="p-6">
        <Stack gap="lg">
          <div>
            <h2 className="text-lg font-medium text-trust-deep mb-4">
              Navigation
            </h2>
            <Stack gap="sm">
              <a
                href="#"
                className="px-4 py-3 rounded-md text-sm font-medium bg-trust text-white flex items-center gap-3"
              >
                <svg
                  className="w-5 h-5"
                  fill="none"
                  stroke="currentColor"
                  viewBox="0 0 24 24"
                >
                  <path
                    strokeLinecap="round"
                    strokeLinejoin="round"
                    strokeWidth={2}
                    d="M3 12l2-2m0 0l7-7 7 7M5 10v10a1 1 0 001 1h3m10-11l2 2m-2-2v10a1 1 0 01-1 1h-3m-6 0a1 1 0 001-1v-4a1 1 0 011-1h2a1 1 0 011 1v4a1 1 0 001 1m-6 0h6"
                  />
                </svg>
                <span>Dashboard</span>
              </a>
              <a
                href="#"
                className="px-4 py-3 rounded-md text-sm font-medium text-trust hover:bg-neutral-200 transition-colors flex items-center gap-3"
              >
                <svg
                  className="w-5 h-5"
                  fill="none"
                  stroke="currentColor"
                  viewBox="0 0 24 24"
                >
                  <path
                    strokeLinecap="round"
                    strokeLinejoin="round"
                    strokeWidth={2}
                    d="M9 19v-6a2 2 0 00-2-2H5a2 2 0 00-2 2v6a2 2 0 002 2h2a2 2 0 002-2zm0 0V9a2 2 0 012-2h2a2 2 0 012 2v10m-6 0a2 2 0 002 2h2a2 2 0 002-2m0 0V5a2 2 0 012-2h2a2 2 0 012 2v14a2 2 0 01-2 2h-2a2 2 0 01-2-2z"
                  />
                </svg>
                <span>Applications</span>
              </a>
              <a
                href="#"
                className="px-4 py-3 rounded-md text-sm font-medium text-trust hover:bg-neutral-200 transition-colors flex items-center gap-3"
              >
                <svg
                  className="w-5 h-5"
                  fill="none"
                  stroke="currentColor"
                  viewBox="0 0 24 24"
                >
                  <path
                    strokeLinecap="round"
                    strokeLinejoin="round"
                    strokeWidth={2}
                    d="M12 4.354a4 4 0 110 5.292M15 21H3v-1a6 6 0 0112 0v1zm0 0h6v-1a6 6 0 00-9-5.197M13 7a4 4 0 11-8 0 4 4 0 018 0z"
                  />
                </svg>
                <span>Users</span>
              </a>
            </Stack>
          </div>
          <div className="pt-4 border-t border-neutral-200">
            <h3 className="text-xs font-medium text-neutral-500 uppercase mb-2">
              Quick Stats
            </h3>
            <div className="space-y-2">
              <div className="text-sm">
                <span className="text-neutral-600">Sessions:</span>{' '}
                <span className="font-medium">127</span>
              </div>
              <div className="text-sm">
                <span className="text-neutral-600">Apps:</span>{' '}
                <span className="font-medium">12</span>
              </div>
            </div>
          </div>
        </Stack>
      </nav>
    ),
    sidebarWidth: 'lg',
    children: <SampleContent />,
  },
};

// Story: Collapsible sidebar
export const SidebarCollapsible: Story = {
  args: {
    header: <SampleHeader />,
    sidebar: <SampleSidebar />,
    sidebarCollapsible: true,
    children: <SampleContent />,
  },
};

// Story: Sticky header
export const StickyHeader: Story = {
  args: {
    header: <SampleHeader />,
    sidebar: <SampleSidebar />,
    stickyHeader: true,
    children: (
      <Container padding="lg" size="xl">
        <Stack gap="lg">
          <div>
            <h2 className="text-3xl font-medium text-trust-deep mb-2">
              Long Content Page
            </h2>
            <p className="text-neutral-600">
              Scroll down to see the sticky header in action.
            </p>
          </div>
          {Array.from({ length: 20 }).map((_, i) => (
            <div
              key={i}
              className="p-6 bg-white border border-neutral-200 rounded-lg shadow-sm"
            >
              <h3 className="text-lg font-medium text-trust-deep mb-2">
                Section {i + 1}
              </h3>
              <p className="text-neutral-600">
                This is sample content to demonstrate scrolling behavior. The
                header will remain fixed at the top of the viewport as you
                scroll down the page.
              </p>
            </div>
          ))}
        </Stack>
      </Container>
    ),
  },
};

// Story: Sticky footer
export const StickyFooter: Story = {
  args: {
    header: <SampleHeader />,
    sidebar: <SampleSidebar />,
    footer: <SampleFooter />,
    stickyFooter: true,
    children: (
      <Container padding="lg" size="xl">
        <Stack gap="lg">
          <div>
            <h2 className="text-3xl font-medium text-trust-deep mb-2">
              Short Content
            </h2>
            <p className="text-neutral-600">
              With minimal content, the footer stays at the bottom of the
              viewport.
            </p>
          </div>
          <div className="p-6 bg-white border border-neutral-200 rounded-lg shadow-sm">
            <h3 className="text-lg font-medium text-trust-deep mb-2">
              Content Section
            </h3>
            <p className="text-neutral-600">
              The footer is sticky at the bottom.
            </p>
          </div>
        </Stack>
      </Container>
    ),
  },
};

// Story: Sticky header and footer
export const StickyHeaderAndFooter: Story = {
  args: {
    header: <SampleHeader />,
    sidebar: <SampleSidebar />,
    footer: <SampleFooter />,
    stickyHeader: true,
    stickyFooter: true,
    children: <SampleContent />,
  },
};

// Story: Responsive drawer
export const ResponsiveDrawer: Story = {
  args: {
    header: <SampleHeader />,
    sidebar: <SampleSidebar />,
    mobileDrawer: true,
    children: <SampleContent />,
  },
  parameters: {
    docs: {
      description: {
        story:
          'On mobile screens, the sidebar appears as an overlay drawer. Click the hamburger button (bottom right on mobile) to toggle. On desktop (≥768px), it behaves normally.',
      },
    },
  },
};

// Story: Real-world consent page example
export const ConsentPage: Story = {
  args: {
    header: (
      <div className="px-6 py-4 flex items-center justify-between">
        <div className="flex items-center gap-3">
          <div className="w-8 h-8 bg-trust-deep rounded-md" />
          <span className="text-lg font-medium text-trust-deep">
            Identity Broker
          </span>
        </div>
        <Button size="sm" variant="ghost">
          Cancel
        </Button>
      </div>
    ),
    sidebar: (
      <div className="p-6">
        <Stack gap="lg">
          <div>
            <h3 className="text-sm font-medium text-trust-deep mb-2">
              Application Details
            </h3>
            <div className="space-y-2 text-sm">
              <div>
                <span className="text-neutral-600">Name:</span>{' '}
                <span className="font-medium">Analytics Dashboard</span>
              </div>
              <div>
                <span className="text-neutral-600">Developer:</span>{' '}
                <span className="font-medium">Acme Corp</span>
              </div>
              <div>
                <span className="text-neutral-600">Website:</span>{' '}
                <a href="#" className="text-trust hover:underline">
                  analytics.acme.com
                </a>
              </div>
            </div>
          </div>
          <div className="pt-4 border-t border-neutral-200">
            <h3 className="text-sm font-medium text-trust-deep mb-2">
              Privacy Policy
            </h3>
            <p className="text-sm text-neutral-600">
              By granting access, you agree to share the requested information
              with this application.
            </p>
            <a
              href="#"
              className="text-sm text-trust hover:underline mt-2 inline-block"
            >
              Read full policy
            </a>
          </div>
        </Stack>
      </div>
    ),
    sidebarWidth: 'sm',
    sidebarPosition: 'right',
    footer: (
      <div className="px-6 py-4 flex items-center justify-between">
        <p className="text-xs text-neutral-500">
          Session expires in 10 minutes
        </p>
        <a href="#" className="text-xs text-trust hover:underline">
          Need help?
        </a>
      </div>
    ),
    children: (
      <Container padding="lg" size="md">
        <Stack gap="lg">
          <div className="text-center">
            <div className="w-16 h-16 bg-trust-deep rounded-full mx-auto mb-4 flex items-center justify-center">
              <svg
                className="w-8 h-8 text-white"
                fill="none"
                stroke="currentColor"
                viewBox="0 0 24 24"
              >
                <path
                  strokeLinecap="round"
                  strokeLinejoin="round"
                  strokeWidth={2}
                  d="M12 15v2m-6 4h12a2 2 0 002-2v-6a2 2 0 00-2-2H6a2 2 0 00-2 2v6a2 2 0 002 2zm10-10V7a4 4 0 00-8 0v4h8z"
                />
              </svg>
            </div>
            <h1 className="text-2xl font-medium text-trust-deep mb-2">
              Grant Access
            </h1>
            <p className="text-neutral-600">
              Analytics Dashboard wants to access your account
            </p>
          </div>

          <div className="p-6 bg-neutral-50 border border-neutral-200 rounded-lg">
            <h2 className="text-lg font-medium text-trust-deep mb-4">
              This application will be able to:
            </h2>
            <Stack gap="sm">
              <div className="flex items-start gap-3">
                <svg
                  className="w-5 h-5 text-success-primary flex-shrink-0 mt-0.5"
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
                  <p className="font-medium text-trust-deep">
                    Read your profile information
                  </p>
                  <p className="text-sm text-neutral-600">
                    Name, email, and profile picture
                  </p>
                </div>
              </div>
              <div className="flex items-start gap-3">
                <svg
                  className="w-5 h-5 text-success-primary flex-shrink-0 mt-0.5"
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
                  <p className="font-medium text-trust-deep">
                    Access your activity data
                  </p>
                  <p className="text-sm text-neutral-600">
                    View your usage patterns and preferences
                  </p>
                </div>
              </div>
              <div className="flex items-start gap-3">
                <svg
                  className="w-5 h-5 text-success-primary flex-shrink-0 mt-0.5"
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
                  <p className="font-medium text-trust-deep">
                    Send you notifications
                  </p>
                  <p className="text-sm text-neutral-600">
                    Important updates and alerts
                  </p>
                </div>
              </div>
            </Stack>
          </div>

          <div className="bg-amber-50 border border-amber-200 rounded-lg p-4">
            <div className="flex gap-3">
              <svg
                className="w-5 h-5 text-amber-600 flex-shrink-0 mt-0.5"
                fill="none"
                stroke="currentColor"
                viewBox="0 0 24 24"
              >
                <path
                  strokeLinecap="round"
                  strokeLinejoin="round"
                  strokeWidth={2}
                  d="M12 9v2m0 4h.01m-6.938 4h13.856c1.54 0 2.502-1.667 1.732-3L13.732 4c-.77-1.333-2.694-1.333-3.464 0L3.34 16c-.77 1.333.192 3 1.732 3z"
                />
              </svg>
              <div>
                <p className="font-medium text-amber-900">Important</p>
                <p className="text-sm text-amber-800">
                  You can revoke access at any time from your account settings.
                </p>
              </div>
            </div>
          </div>

          <Stack direction="row" gap="md" justify="center">
            <Button variant="outline" size="lg">
              Deny
            </Button>
            <Button variant="primary" size="lg">
              Grant Access
            </Button>
          </Stack>
        </Stack>
      </Container>
    ),
  },
};
