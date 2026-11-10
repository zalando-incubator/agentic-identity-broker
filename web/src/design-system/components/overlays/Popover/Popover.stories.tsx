/**
 * Popover Component Stories
 */

import type { Meta, StoryObj } from '@storybook/react';
import { useState } from 'react';
import { Popover } from './Popover';
import { Button } from '@design-system/components/primitives/Button';

// Mock InfoIcon component
const InfoIcon = () => (
  <svg
    className="w-5 h-5"
    fill="none"
    viewBox="0 0 24 24"
    stroke="currentColor"
    aria-hidden="true"
  >
    <path
      strokeLinecap="round"
      strokeLinejoin="round"
      strokeWidth={2}
      d="M13 16h-1v-4h-1m1-4h.01M21 12a9 9 0 11-18 0 9 9 0 0118 0z"
    />
  </svg>
);

const UserIcon = () => (
  <svg
    className="w-5 h-5"
    fill="none"
    viewBox="0 0 24 24"
    stroke="currentColor"
    aria-hidden="true"
  >
    <path
      strokeLinecap="round"
      strokeLinejoin="round"
      strokeWidth={2}
      d="M16 7a4 4 0 11-8 0 4 4 0 018 0zM12 14a7 7 0 00-7 7h14a7 7 0 00-7-7z"
    />
  </svg>
);

const meta = {
  title: 'Design System/Overlays/Popover',
  component: Popover,
  parameters: {
    layout: 'centered',
  },
  tags: ['autodocs'],
  argTypes: {
    trigger: {
      control: false,
      description: 'Element that triggers the popover',
    },
    children: {
      control: 'text',
      description: 'Popover content',
    },
    header: {
      control: 'text',
      description: 'Optional header section (title or custom content)',
    },
    footer: {
      control: false,
      description: 'Optional footer section (actions or custom content)',
    },
    position: {
      control: 'select',
      options: ['top', 'right', 'bottom', 'left'],
      description: 'Position of popover relative to trigger',
    },
    width: {
      control: 'select',
      options: ['sm', 'md', 'lg', 'full'],
      description: 'Width variant',
    },
    showArrow: {
      control: 'boolean',
      description: 'Whether to show arrow indicator',
    },
    isOpen: {
      control: 'boolean',
      description: 'Controlled open state (optional)',
    },
  },
} satisfies Meta<typeof Popover>;

export default meta;
type Story = StoryObj<typeof meta>;

/**
 * Default popover with basic content
 */
export const Default: Story = {
  render: (args) => {
    return (
      <div className="flex items-center justify-center min-h-[400px]">
        <Popover
          {...args}
          trigger={<Button variant="outline">Open Popover</Button>}
        >
          <p>
            This is a basic popover with simple content. Click outside or press
            ESC to close.
          </p>
        </Popover>
      </div>
    );
  },
  args: {
    trigger: <Button variant="outline">Open Popover</Button>,
    children: 'Default popover content',
    position: 'bottom',
    width: 'md',
    showArrow: true,
  },
};

/**
 * All position variants: top, right, bottom, left
 */
export const Positions: Story = {
  render: () => {
    return (
      <div className="flex items-center justify-center min-h-[600px] gap-20">
        <div className="grid grid-cols-3 gap-20 items-center">
          {/* Top */}
          <div className="col-start-2 flex justify-center">
            <Popover
              trigger={<Button variant="outline">Top</Button>}
              position="top"
              showArrow
            >
              <p>Popover positioned at the top</p>
            </Popover>
          </div>

          {/* Left */}
          <div className="col-start-1 flex justify-center">
            <Popover
              trigger={<Button variant="outline">Left</Button>}
              position="left"
              showArrow
            >
              <p>Popover positioned on the left</p>
            </Popover>
          </div>

          {/* Center placeholder */}
          <div className="col-start-2 flex justify-center">
            <div className="w-24 h-24 rounded-lg border-2 border-dashed border-neutral-300 flex items-center justify-center text-sm text-neutral-500">
              Trigger
            </div>
          </div>

          {/* Right */}
          <div className="col-start-3 flex justify-center">
            <Popover
              trigger={<Button variant="outline">Right</Button>}
              position="right"
              showArrow
            >
              <p>Popover positioned on the right</p>
            </Popover>
          </div>

          {/* Bottom */}
          <div className="col-start-2 flex justify-center">
            <Popover
              trigger={<Button variant="outline">Bottom</Button>}
              position="bottom"
              showArrow
            >
              <p>Popover positioned at the bottom</p>
            </Popover>
          </div>
        </div>
      </div>
    );
  },
  args: {
    trigger: <Button variant="outline">Position</Button>,
    children: 'Popover content',
    position: 'bottom',
    showArrow: true,
  },
};

/**
 * Popover with header section
 */
export const WithHeader: Story = {
  render: () => {
    return (
      <div className="flex items-center justify-center min-h-[400px]">
        <Popover
          trigger={
            <Button variant="outline" iconBefore={<InfoIcon />}>
              User Information
            </Button>
          }
          header="Profile Details"
          position="bottom"
          showArrow
        >
          <div className="space-y-3">
            <div>
              <p className="text-xs font-medium text-neutral-500 mb-1">Name</p>
              <p className="font-medium text-neutral-900">John Doe</p>
            </div>
            <div>
              <p className="text-xs font-medium text-neutral-500 mb-1">Email</p>
              <p className="text-neutral-700">john.doe@example.com</p>
            </div>
            <div>
              <p className="text-xs font-medium text-neutral-500 mb-1">Role</p>
              <p className="text-neutral-700">Administrator</p>
            </div>
          </div>
        </Popover>
      </div>
    );
  },
  args: {
    trigger: <Button variant="outline">User Info</Button>,
    children: 'User information content',
    header: 'Profile Details',
    position: 'bottom',
    showArrow: true,
  },
};

/**
 * Popover with footer actions
 */
export const WithFooter: Story = {
  render: () => {
    return (
      <div className="flex items-center justify-center min-h-[400px]">
        <Popover
          trigger={<Button variant="outline">Quick Actions</Button>}
          position="bottom"
          showArrow
          footer={
            <div className="flex gap-2 justify-end">
              <Button size="sm" variant="outline">
                Cancel
              </Button>
              <Button size="sm" variant="primary">
                Apply
              </Button>
            </div>
          }
        >
          <p>
            Select an action to perform. Use the buttons below to confirm or
            cancel.
          </p>
        </Popover>
      </div>
    );
  },
  args: {
    trigger: <Button variant="outline">Quick Actions</Button>,
    children: 'Action content',
    position: 'bottom',
    showArrow: true,
  },
};

/**
 * Popover with both header and footer
 */
export const WithHeaderAndFooter: Story = {
  render: () => {
    return (
      <div className="flex items-center justify-center min-h-[500px]">
        <Popover
          trigger={<Button variant="primary">Confirm Consent</Button>}
          header="Grant Access"
          position="bottom"
          width="md"
          showArrow
          footer={
            <div className="flex gap-2 justify-end">
              <Button size="sm" variant="outline">
                Deny
              </Button>
              <Button size="sm" variant="primary">
                Grant Access
              </Button>
            </div>
          }
        >
          <div className="space-y-3">
            <p>
              <strong>Analytics Dashboard</strong> is requesting access to:
            </p>
            <ul className="list-disc list-inside space-y-1 text-sm pl-2">
              <li>Basic profile information</li>
              <li>Usage statistics</li>
              <li>Preference settings</li>
            </ul>
            <div className="mt-3 p-3 bg-info-light border border-info-primary/20 rounded-md">
              <p className="text-xs text-info-dark">
                This permission will be valid for 30 days and can be revoked at
                any time.
              </p>
            </div>
          </div>
        </Popover>
      </div>
    );
  },
  args: {
    trigger: <Button variant="primary">Confirm Consent</Button>,
    children: 'Consent content',
    header: 'Grant Access',
    position: 'bottom',
    width: 'md',
    showArrow: true,
  },
};

/**
 * Arrow indicator toggle
 */
export const WithArrow: Story = {
  render: () => {
    return (
      <div className="flex items-center justify-center min-h-[400px] gap-8">
        <Popover
          trigger={<Button variant="outline">With Arrow</Button>}
          position="bottom"
          showArrow={true}
        >
          <p>This popover has an arrow indicator pointing to the trigger.</p>
        </Popover>

        <Popover
          trigger={<Button variant="outline">Without Arrow</Button>}
          position="bottom"
          showArrow={false}
        >
          <p>This popover has no arrow indicator.</p>
        </Popover>
      </div>
    );
  },
  args: {
    trigger: <Button variant="outline">Arrow Toggle</Button>,
    children: 'Arrow content',
    position: 'bottom',
    showArrow: true,
  },
};

/**
 * Wide content demonstration
 */
export const WideContent: Story = {
  render: () => {
    return (
      <div className="flex items-center justify-center min-h-[500px] gap-6">
        <Popover
          trigger={<Button variant="outline">Small (300px)</Button>}
          header="Small Width"
          position="bottom"
          width="sm"
          showArrow
        >
          <p>This is a small popover at 300px width.</p>
        </Popover>

        <Popover
          trigger={<Button variant="outline">Medium (400px)</Button>}
          header="Medium Width"
          position="bottom"
          width="md"
          showArrow
        >
          <p>This is a medium popover at 400px width (default).</p>
        </Popover>

        <Popover
          trigger={<Button variant="outline">Large (500px)</Button>}
          header="Large Width"
          position="bottom"
          width="lg"
          showArrow
        >
          <div className="space-y-3">
            <p>This is a large popover at 500px width.</p>
            <p className="text-sm text-neutral-600">
              Perfect for more detailed content or data tables.
            </p>
            <div className="p-3 bg-neutral-50 rounded-md border border-neutral-200">
              <p className="text-xs font-medium text-neutral-700">
                Example content area with more space
              </p>
            </div>
          </div>
        </Popover>
      </div>
    );
  },
  args: {
    trigger: <Button variant="outline">Width Demo</Button>,
    children: 'Width content',
    position: 'bottom',
    width: 'md',
    showArrow: true,
  },
};

/**
 * Keyboard focus accessibility
 */
export const KeyboardFocus: Story = {
  render: () => {
    return (
      <div className="flex flex-col items-center justify-center min-h-[400px] gap-8">
        <div className="text-center max-w-md mb-4">
          <h3 className="text-lg font-medium text-neutral-900 mb-2">
            Keyboard Navigation
          </h3>
          <p className="text-sm text-neutral-600">
            Try using Tab to navigate between buttons, then press Enter or Space
            to open the popover. Press Escape to close.
          </p>
        </div>

        <div className="flex gap-4">
          <Popover
            trigger={
              <button
                className="px-4 py-2 text-sm font-medium text-neutral-700 bg-white border border-neutral-300 rounded-md hover:bg-neutral-50 focus:outline-none focus:ring-2 focus:ring-trust-deep focus:ring-offset-2"
                tabIndex={0}
              >
                <span className="flex items-center gap-2">
                  <InfoIcon />
                  Focusable Button 1
                </span>
              </button>
            }
            header="Accessible Popover"
            position="bottom"
            showArrow
          >
            <p>
              This popover can be triggered with keyboard navigation. Press Tab
              to focus, Enter to open, and Escape to close.
            </p>
          </Popover>

          <Popover
            trigger={
              <button
                className="px-4 py-2 text-sm font-medium text-neutral-700 bg-white border border-neutral-300 rounded-md hover:bg-neutral-50 focus:outline-none focus:ring-2 focus:ring-trust-deep focus:ring-offset-2"
                tabIndex={0}
              >
                <span className="flex items-center gap-2">
                  <UserIcon />
                  Focusable Button 2
                </span>
              </button>
            }
            header="WCAG 2.1 AA Compliant"
            position="bottom"
            showArrow
          >
            <p>
              All popovers support full keyboard accessibility and screen reader
              announcements.
            </p>
          </Popover>
        </div>
      </div>
    );
  },
  args: {
    trigger: <button>Keyboard Focus</button>,
    children: 'Keyboard focus content',
    position: 'bottom',
    showArrow: true,
  },
};

/**
 * Interactive playground with all controls
 */
export const Playground: Story = {
  render: (args) => {
    return (
      <div className="flex items-center justify-center min-h-[600px]">
        <Popover
          {...args}
          trigger={<Button variant="primary">Open Playground</Button>}
        >
          {args.children || (
            <div>
              <p className="mb-3">
                Use the controls below to customize this popover's appearance
                and behavior.
              </p>
              <ul className="list-disc list-inside text-sm space-y-1">
                <li>Change position (top, right, bottom, left)</li>
                <li>Adjust width (sm, md, lg, full)</li>
                <li>Toggle arrow indicator</li>
                <li>Add header or footer sections</li>
              </ul>
            </div>
          )}
        </Popover>
      </div>
    );
  },
  args: {
    trigger: <Button variant="primary">Open Playground</Button>,
    children: 'Playground content',
    position: 'bottom',
    width: 'md',
    showArrow: true,
    header: 'Playground Popover',
  },
};

/**
 * Controlled mode with custom state management
 */
export const Controlled: Story = {
  render: () => {
    const [isOpen, setIsOpen] = useState(false);

    return (
      <div className="flex flex-col items-center justify-center min-h-[400px] gap-6">
        <div className="text-center">
          <p className="text-sm text-neutral-600 mb-3">
            External controls (controlled mode):
          </p>
          <div className="flex gap-2 justify-center">
            <Button size="sm" variant="outline" onClick={() => setIsOpen(true)}>
              Open Popover
            </Button>
            <Button
              size="sm"
              variant="outline"
              onClick={() => setIsOpen(false)}
            >
              Close Popover
            </Button>
          </div>
          <p className="text-xs text-neutral-500 mt-2">
            Current state: {isOpen ? 'Open' : 'Closed'}
          </p>
        </div>

        <Popover
          trigger={<Button variant="primary">Controlled Popover</Button>}
          header="Controlled Mode"
          position="bottom"
          showArrow
          isOpen={isOpen}
          onOpenChange={setIsOpen}
        >
          <p>
            This popover's state is controlled externally. You can open/close it
            using the buttons above or by clicking the trigger.
          </p>
        </Popover>
      </div>
    );
  },
  args: {
    trigger: <Button variant="primary">Controlled</Button>,
    children: 'Controlled content',
    position: 'bottom',
    showArrow: true,
  },
};

/**
 * Real-world consent management examples
 */
export const ConsentExamples: Story = {
  render: () => {
    return (
      <div className="flex flex-col items-center justify-center min-h-[500px] gap-6">
        <h3 className="text-lg font-medium text-neutral-900">
          Consent Management Use Cases
        </h3>

        <div className="flex flex-wrap gap-4 justify-center max-w-2xl">
          {/* Quick Info Popover */}
          <Popover
            trigger={
              <Button variant="outline" size="sm" iconBefore={<InfoIcon />}>
                Consent Info
              </Button>
            }
            header="What is Consent?"
            position="bottom"
            width="sm"
            showArrow
          >
            <p className="text-sm">
              Consent allows applications to access your data with your
              permission. You can revoke access at any time.
            </p>
          </Popover>

          {/* Grant Consent Popover */}
          <Popover
            trigger={
              <Button variant="primary" size="sm">
                Grant Access
              </Button>
            }
            header="Grant Consent"
            position="bottom"
            width="md"
            showArrow
            footer={
              <div className="flex gap-2 justify-end">
                <Button size="sm" variant="outline">
                  Deny
                </Button>
                <Button size="sm" variant="primary">
                  Allow
                </Button>
              </div>
            }
          >
            <div className="space-y-2">
              <p className="text-sm font-medium">
                <strong>Analytics Platform</strong> requests:
              </p>
              <ul className="list-disc list-inside text-sm space-y-1 pl-2">
                <li>Read profile data</li>
                <li>Access usage statistics</li>
              </ul>
            </div>
          </Popover>

          {/* View Details Popover */}
          <Popover
            trigger={
              <Button variant="outline" size="sm">
                View Details
              </Button>
            }
            header="Active Consent"
            position="bottom"
            width="md"
            showArrow
          >
            <div className="space-y-3 text-sm">
              <div>
                <p className="font-medium text-neutral-900 mb-1">
                  Marketing Dashboard
                </p>
                <p className="text-xs text-neutral-600">
                  Granted on Dec 1, 2025 - Expires in 15 days
                </p>
              </div>
              <div>
                <p className="text-xs font-medium text-neutral-700 mb-1">
                  Permissions:
                </p>
                <ul className="text-xs text-neutral-600 space-y-0.5 pl-3">
                  <li>• Basic profile access</li>
                  <li>• Usage data (read-only)</li>
                </ul>
              </div>
            </div>
          </Popover>

          {/* Revoke Warning Popover */}
          <Popover
            trigger={
              <Button variant="danger" size="sm">
                Revoke
              </Button>
            }
            header="Revoke Consent?"
            position="top"
            width="sm"
            showArrow
            footer={
              <div className="flex gap-2 justify-end">
                <Button size="sm" variant="outline">
                  Cancel
                </Button>
                <Button size="sm" variant="danger">
                  Revoke
                </Button>
              </div>
            }
          >
            <p className="text-sm">
              This will immediately remove the application's access to your
              data.
            </p>
          </Popover>
        </div>
      </div>
    );
  },
  args: {
    trigger: <Button variant="outline">Consent</Button>,
    children: 'Consent content',
    position: 'bottom',
    showArrow: true,
  },
};
