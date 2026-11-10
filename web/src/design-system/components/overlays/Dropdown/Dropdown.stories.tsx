/**
 * Dropdown Component Stories
 */

import type { Meta, StoryObj } from '@storybook/react';
import { useState } from 'react';
import { Dropdown, type DropdownItem } from './Dropdown';
import { Button } from '@design-system/components/primitives/Button';

const meta = {
  title: 'Design System/Overlays/Dropdown',
  component: Dropdown,
  parameters: {
    layout: 'centered',
  },
  tags: ['autodocs'],
  argTypes: {
    items: {
      control: 'object',
      description: 'Array of dropdown menu items',
    },
    trigger: {
      control: 'text',
      description: 'Trigger element (button, link, etc.)',
    },
    align: {
      control: 'select',
      options: ['left', 'right'],
      description: 'Dropdown alignment relative to trigger',
    },
    size: {
      control: 'select',
      options: ['sm', 'md', 'lg'],
      description: 'Dropdown size variant',
    },
    onSelect: {
      action: 'selected',
      description: 'Callback when an item is selected',
    },
  },
} satisfies Meta<typeof Dropdown>;

export default meta;
type Story = StoryObj<typeof meta>;

// Sample icons
const EditIcon = () => (
  <svg
    className="w-4 h-4"
    fill="none"
    viewBox="0 0 24 24"
    stroke="currentColor"
  >
    <path
      strokeLinecap="round"
      strokeLinejoin="round"
      strokeWidth={2}
      d="M11 5H6a2 2 0 00-2 2v11a2 2 0 002 2h11a2 2 0 002-2v-5m-1.414-9.414a2 2 0 112.828 2.828L11.828 15H9v-2.828l8.586-8.586z"
    />
  </svg>
);

const TrashIcon = () => (
  <svg
    className="w-4 h-4"
    fill="none"
    viewBox="0 0 24 24"
    stroke="currentColor"
  >
    <path
      strokeLinecap="round"
      strokeLinejoin="round"
      strokeWidth={2}
      d="M19 7l-.867 12.142A2 2 0 0116.138 21H7.862a2 2 0 01-1.995-1.858L5 7m5 4v6m4-6v6m1-10V4a1 1 0 00-1-1h-4a1 1 0 00-1 1v3M4 7h16"
    />
  </svg>
);

const DuplicateIcon = () => (
  <svg
    className="w-4 h-4"
    fill="none"
    viewBox="0 0 24 24"
    stroke="currentColor"
  >
    <path
      strokeLinecap="round"
      strokeLinejoin="round"
      strokeWidth={2}
      d="M8 16H6a2 2 0 01-2-2V6a2 2 0 012-2h8a2 2 0 012 2v2m-6 12h8a2 2 0 002-2v-8a2 2 0 00-2-2h-8a2 2 0 00-2 2v8a2 2 0 002 2z"
    />
  </svg>
);

const ArchiveIcon = () => (
  <svg
    className="w-4 h-4"
    fill="none"
    viewBox="0 0 24 24"
    stroke="currentColor"
  >
    <path
      strokeLinecap="round"
      strokeLinejoin="round"
      strokeWidth={2}
      d="M5 8h14M5 8a2 2 0 110-4h14a2 2 0 110 4M5 8v10a2 2 0 002 2h10a2 2 0 002-2V8m-9 4h4"
    />
  </svg>
);

const ShareIcon = () => (
  <svg
    className="w-4 h-4"
    fill="none"
    viewBox="0 0 24 24"
    stroke="currentColor"
  >
    <path
      strokeLinecap="round"
      strokeLinejoin="round"
      strokeWidth={2}
      d="M8.684 13.342C8.886 12.938 9 12.482 9 12c0-.482-.114-.938-.316-1.342m0 2.684a3 3 0 110-2.684m0 2.684l6.632 3.316m-6.632-6l6.632-3.316m0 0a3 3 0 105.367-2.684 3 3 0 00-5.367 2.684zm0 9.316a3 3 0 105.368 2.684 3 3 0 00-5.368-2.684z"
    />
  </svg>
);

const DownloadIcon = () => (
  <svg
    className="w-4 h-4"
    fill="none"
    viewBox="0 0 24 24"
    stroke="currentColor"
  >
    <path
      strokeLinecap="round"
      strokeLinejoin="round"
      strokeWidth={2}
      d="M4 16v1a3 3 0 003 3h10a3 3 0 003-3v-1m-4-4l-4 4m0 0l-4-4m4 4V4"
    />
  </svg>
);

const UserIcon = () => (
  <svg
    className="w-4 h-4"
    fill="none"
    viewBox="0 0 24 24"
    stroke="currentColor"
  >
    <path
      strokeLinecap="round"
      strokeLinejoin="round"
      strokeWidth={2}
      d="M16 7a4 4 0 11-8 0 4 4 0 018 0zM12 14a7 7 0 00-7 7h14a7 7 0 00-7-7z"
    />
  </svg>
);

const SettingsIcon = () => (
  <svg
    className="w-4 h-4"
    fill="none"
    viewBox="0 0 24 24"
    stroke="currentColor"
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
);

const LogoutIcon = () => (
  <svg
    className="w-4 h-4"
    fill="none"
    viewBox="0 0 24 24"
    stroke="currentColor"
  >
    <path
      strokeLinecap="round"
      strokeLinejoin="round"
      strokeWidth={2}
      d="M17 16l4-4m0 0l-4-4m4 4H7m6 4v1a3 3 0 01-3 3H6a3 3 0 01-3-3V7a3 3 0 013-3h4a3 3 0 013 3v1"
    />
  </svg>
);

const defaultItems: DropdownItem[] = [
  { id: '1', label: 'Edit' },
  { id: '2', label: 'Duplicate' },
  { id: '3', label: 'Archive' },
  { id: '4', label: 'Delete', destructive: true },
];

const sectionsItems: DropdownItem[] = [
  { id: '1', label: 'Profile', section: 'Account' },
  { id: '2', label: 'Settings', divider: true },
  { id: '3', label: 'Billing', section: 'Organization' },
  { id: '4', label: 'Team Members', divider: true },
  { id: '5', label: 'Documentation', section: 'Support' },
  { id: '6', label: 'Contact Support', divider: true },
  { id: '7', label: 'Sign Out', destructive: true },
];

const withIconsItems: DropdownItem[] = [
  { id: '1', label: 'Edit', icon: <EditIcon /> },
  { id: '2', label: 'Duplicate', icon: <DuplicateIcon /> },
  { id: '3', label: 'Archive', icon: <ArchiveIcon /> },
  { id: '4', label: 'Share', icon: <ShareIcon /> },
  { id: '5', label: 'Download', icon: <DownloadIcon /> },
  { id: '6', label: 'Delete', icon: <TrashIcon />, destructive: true },
];

const withDescriptionsItems: DropdownItem[] = [
  {
    id: '1',
    label: 'Private',
    description: 'Only you can see this',
    icon: <UserIcon />,
  },
  {
    id: '2',
    label: 'Team',
    description: 'Visible to your team members',
    icon: <ShareIcon />,
  },
  {
    id: '3',
    label: 'Public',
    description: 'Anyone with the link can view',
    icon: <ShareIcon />,
  },
];

const disabledItems: DropdownItem[] = [
  { id: '1', label: 'Edit', icon: <EditIcon /> },
  { id: '2', label: 'Duplicate', icon: <DuplicateIcon />, disabled: true },
  { id: '3', label: 'Archive', icon: <ArchiveIcon />, disabled: true },
  { id: '4', label: 'Share', icon: <ShareIcon /> },
  { id: '5', label: 'Download', icon: <DownloadIcon /> },
  { id: '6', label: 'Delete', icon: <TrashIcon />, destructive: true },
];

const languageItems: DropdownItem[] = [
  { id: '1', label: 'English' },
  { id: '2', label: 'Spanish' },
  { id: '3', label: 'French' },
  { id: '4', label: 'German' },
  { id: '5', label: 'Italian' },
];

const destructiveItems: DropdownItem[] = [
  { id: '1', label: 'View Details' },
  { id: '2', label: 'Edit Properties' },
  { id: '3', label: 'Duplicate', divider: true },
  {
    id: '4',
    label: 'Archive',
    description: 'Hide from active list',
    destructive: true,
  },
  {
    id: '5',
    label: 'Delete',
    description: 'Permanently remove this item',
    icon: <TrashIcon />,
    destructive: true,
  },
];

const complexContentItems: DropdownItem[] = [
  {
    id: '1',
    label: 'John Doe',
    description: 'john.doe@example.com',
    icon: <UserIcon />,
  },
  {
    id: '2',
    label: 'Jane Smith',
    description: 'jane.smith@example.com',
    icon: <UserIcon />,
    divider: true,
  },
  {
    id: '3',
    label: 'Invite Team Member',
    description: 'Send an email invitation',
    icon: <ShareIcon />,
  },
];

const sizesItems: DropdownItem[] = [
  { id: '1', label: 'Edit', icon: <EditIcon /> },
  { id: '2', label: 'Duplicate', icon: <DuplicateIcon /> },
  { id: '3', label: 'Delete', icon: <TrashIcon />, destructive: true },
];

const alignmentItems: DropdownItem[] = [
  { id: '1', label: 'Profile', icon: <UserIcon /> },
  { id: '2', label: 'Settings', icon: <SettingsIcon /> },
  { id: '3', label: 'Sign Out', icon: <LogoutIcon />, destructive: true },
];

const userAccountMenuItems: DropdownItem[] = [
  {
    id: '1',
    label: 'John Doe',
    description: 'john.doe@example.com',
    icon: <UserIcon />,
    divider: true,
  },
  {
    id: '2',
    label: 'Profile',
    icon: <UserIcon />,
    section: 'Account',
  },
  {
    id: '3',
    label: 'Settings',
    icon: <SettingsIcon />,
    divider: true,
  },
  {
    id: '4',
    label: 'Documentation',
    section: 'Help',
  },
  {
    id: '5',
    label: 'Contact Support',
    divider: true,
  },
  {
    id: '6',
    label: 'Sign Out',
    icon: <LogoutIcon />,
    destructive: true,
  },
];

const consentActionsMenuItems: DropdownItem[] = [
  {
    id: '1',
    label: 'View Details',
    description: 'See full consent information',
  },
  {
    id: '2',
    label: 'Edit Permissions',
    description: 'Modify granted access',
    divider: true,
  },
  {
    id: '3',
    label: 'Extend Duration',
    description: 'Extend expiration date',
    section: 'Manage',
  },
  {
    id: '4',
    label: 'Pause Access',
    description: 'Temporarily suspend permissions',
    divider: true,
  },
  {
    id: '5',
    label: 'Revoke Consent',
    description: 'Permanently remove all access',
    icon: <TrashIcon />,
    destructive: true,
  },
];

const playgroundItems: DropdownItem[] = [
  { id: '1', label: 'First Item', icon: <EditIcon /> },
  {
    id: '2',
    label: 'Second Item',
    icon: <DuplicateIcon />,
    description: 'With description',
  },
  { id: '3', label: 'Disabled Item', icon: <ArchiveIcon />, disabled: true },
  {
    id: '4',
    label: 'Selected Item',
    icon: <ShareIcon />,
    selected: true,
    divider: true,
  },
  {
    id: '5',
    label: 'Destructive Action',
    icon: <TrashIcon />,
    destructive: true,
  },
];

/**
 * Default dropdown with basic items
 */
export const Default: Story = {
  render: (args) => {
    return (
      <div className="h-64 flex items-start justify-center">
        <Dropdown
          {...args}
          items={args.items || defaultItems}
          trigger={<Button>Actions</Button>}
        />
      </div>
    );
  },
  args: {
    align: 'left',
    size: 'md',
  },
};

/**
 * Dropdown with section grouping
 */
export const Sections: Story = {
  render: () => {
    return (
      <div className="h-96 flex items-start justify-center">
        <Dropdown
          items={sectionsItems}
          trigger={<Button>User Menu</Button>}
          onSelect={(item) => console.log('Selected:', item.label)}
        />
      </div>
    );
  },
  args: {
    align: 'left',
    size: 'md',
  },
};

/**
 * Dropdown with icons
 */
export const WithIcons: Story = {
  render: () => {
    return (
      <div className="h-80 flex items-start justify-center">
        <Dropdown
          items={withIconsItems}
          trigger={<Button>Actions</Button>}
          onSelect={(item) => console.log('Selected:', item.label)}
        />
      </div>
    );
  },
  args: {
    align: 'left',
    size: 'md',
  },
};

/**
 * Dropdown items with descriptions
 */
export const WithDescriptions: Story = {
  render: () => {
    return (
      <div className="h-80 flex items-start justify-center">
        <Dropdown
          items={withDescriptionsItems}
          trigger={<Button>Change Visibility</Button>}
          size="lg"
          onSelect={(item) => console.log('Selected:', item.label)}
        />
      </div>
    );
  },
  args: {
    align: 'left',
    size: 'lg',
  },
};

/**
 * Dropdown with disabled items
 */
export const DisabledItems: Story = {
  render: () => {
    return (
      <div className="h-80 flex items-start justify-center">
        <Dropdown
          items={disabledItems}
          trigger={<Button>Actions</Button>}
          onSelect={(item) => alert(`Selected: ${item.label}`)}
        />
      </div>
    );
  },
  args: {
    align: 'left',
    size: 'md',
  },
};

/**
 * Dropdown with checkmarks for selected items
 */
export const WithCheckmarks: Story = {
  render: () => {
    const [selectedId, setSelectedId] = useState('2');
    const items = languageItems.map((item) => ({
      ...item,
      selected: selectedId === item.id,
    }));

    return (
      <div className="h-80 flex items-start justify-center">
        <Dropdown
          items={items}
          trigger={<Button>Select Language</Button>}
          onSelect={(item) => setSelectedId(item.id)}
        />
      </div>
    );
  },
  args: {
    align: 'left',
    size: 'md',
  },
};

/**
 * Dropdown with destructive actions
 */
export const Destructive: Story = {
  render: () => {
    return (
      <div className="h-80 flex items-start justify-center">
        <Dropdown
          items={destructiveItems}
          trigger={<Button variant="outline">Manage Item</Button>}
          size="lg"
          onSelect={(item) => {
            if (item.destructive) {
              if (
                confirm(`Are you sure you want to ${item.label.toLowerCase()}?`)
              ) {
                alert(`${item.label} confirmed`);
              }
            } else {
              alert(`Selected: ${item.label}`);
            }
          }}
        />
      </div>
    );
  },
  args: {
    align: 'left',
    size: 'lg',
  },
};

/**
 * Dropdown with complex/rich content
 */
export const ComplexContent: Story = {
  render: () => {
    return (
      <div className="h-80 flex items-start justify-center">
        <Dropdown
          items={complexContentItems}
          trigger={<Button>Assign To</Button>}
          size="lg"
          onSelect={(item) => alert(`Assigned to: ${item.label}`)}
        />
      </div>
    );
  },
  args: {
    align: 'left',
    size: 'lg',
  },
};

/**
 * Dropdown sizes comparison
 */
export const Sizes: Story = {
  render: () => {
    return (
      <div className="h-80 flex items-start justify-center gap-4">
        <Dropdown
          items={sizesItems}
          trigger={<Button size="sm">Small</Button>}
          size="sm"
        />
        <Dropdown
          items={sizesItems}
          trigger={<Button size="md">Medium</Button>}
          size="md"
        />
        <Dropdown
          items={sizesItems}
          trigger={<Button size="lg">Large</Button>}
          size="lg"
        />
      </div>
    );
  },
  args: {
    align: 'left',
  },
};

/**
 * Dropdown alignment options
 */
export const Alignment: Story = {
  render: () => {
    return (
      <div className="h-80 w-full flex items-start justify-between px-8">
        <Dropdown
          items={alignmentItems}
          trigger={<Button>Align Left</Button>}
          align="left"
        />
        <Dropdown
          items={alignmentItems}
          trigger={<Button>Align Right</Button>}
          align="right"
        />
      </div>
    );
  },
  args: {},
};

/**
 * User account menu (real-world example)
 */
export const UserAccountMenu: Story = {
  render: () => {
    return (
      <div className="h-96 flex items-start justify-end pr-8">
        <Dropdown
          items={userAccountMenuItems}
          trigger={
            <button className="flex items-center gap-2 px-3 py-2 rounded-md hover:bg-neutral-100 transition-colors">
              <div className="w-8 h-8 bg-trust-hover text-white rounded-full flex items-center justify-center text-sm font-medium">
                JD
              </div>
              <svg
                className="w-4 h-4 text-neutral-600"
                fill="none"
                viewBox="0 0 24 24"
                stroke="currentColor"
              >
                <path
                  strokeLinecap="round"
                  strokeLinejoin="round"
                  strokeWidth={2}
                  d="M19 9l-7 7-7-7"
                />
              </svg>
            </button>
          }
          align="right"
          size="md"
          onSelect={(item) => {
            if (item.id === '6') {
              alert('Signing out...');
            } else {
              alert(`Selected: ${item.label}`);
            }
          }}
        />
      </div>
    );
  },
  args: {},
};

/**
 * Consent management actions dropdown
 */
export const ConsentActionsMenu: Story = {
  render: () => {
    return (
      <div className="h-96 flex items-start justify-center">
        <Dropdown
          items={consentActionsMenuItems}
          trigger={<Button variant="outline">Manage Consent</Button>}
          size="lg"
          onSelect={(item) => {
            if (item.destructive) {
              if (confirm('Are you sure you want to revoke this consent?')) {
                alert('Consent revoked');
              }
            } else {
              alert(`Selected: ${item.label}`);
            }
          }}
        />
      </div>
    );
  },
  args: {},
};

/**
 * Interactive playground with all controls
 */
export const Playground: Story = {
  render: (args) => {
    return (
      <div className="h-96 flex items-start justify-center">
        <Dropdown
          {...args}
          items={args.items || playgroundItems}
          trigger={args.trigger || <Button>Open Dropdown</Button>}
        />
      </div>
    );
  },
  args: {
    align: 'left',
    size: 'md',
  },
};
