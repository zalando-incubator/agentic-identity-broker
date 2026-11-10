/**
 * Tabs Component Stories
 */

import type { Meta, StoryObj } from '@storybook/react';
import { useState } from 'react';
import { Tabs } from './Tabs';
import type { TabItem } from './Tabs';

const meta = {
  title: 'Design System/Navigation/Tabs',
  component: Tabs,
  parameters: {
    layout: 'padded',
  },
  tags: ['autodocs'],
} satisfies Meta<typeof Tabs>;

export default meta;
type Story = StoryObj<typeof meta>;

// Sample icons for stories
const HomeIcon = () => (
  <svg fill="none" viewBox="0 0 24 24" stroke="currentColor">
    <path
      strokeLinecap="round"
      strokeLinejoin="round"
      strokeWidth={2}
      d="M3 12l2-2m0 0l7-7 7 7M5 10v10a1 1 0 001 1h3m10-11l2 2m-2-2v10a1 1 0 01-1 1h-3m-6 0a1 1 0 001-1v-4a1 1 0 011-1h2a1 1 0 011 1v4a1 1 0 001 1m-6 0h6"
    />
  </svg>
);

const UserIcon = () => (
  <svg fill="none" viewBox="0 0 24 24" stroke="currentColor">
    <path
      strokeLinecap="round"
      strokeLinejoin="round"
      strokeWidth={2}
      d="M16 7a4 4 0 11-8 0 4 4 0 018 0zM12 14a7 7 0 00-7 7h14a7 7 0 00-7-7z"
    />
  </svg>
);

const SettingsIcon = () => (
  <svg fill="none" viewBox="0 0 24 24" stroke="currentColor">
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

const ChartIcon = () => (
  <svg fill="none" viewBox="0 0 24 24" stroke="currentColor">
    <path
      strokeLinecap="round"
      strokeLinejoin="round"
      strokeWidth={2}
      d="M9 19v-6a2 2 0 00-2-2H5a2 2 0 00-2 2v6a2 2 0 002 2h2a2 2 0 002-2zm0 0V9a2 2 0 012-2h2a2 2 0 012 2v10m-6 0a2 2 0 002 2h2a2 2 0 002-2m0 0V5a2 2 0 012-2h2a2 2 0 012 2v14a2 2 0 01-2 2h-2a2 2 0 01-2-2z"
    />
  </svg>
);

const basicTabs: TabItem[] = [
  { id: 'overview', label: 'Overview' },
  { id: 'details', label: 'Details' },
  { id: 'settings', label: 'Settings' },
];

const iconTabs: TabItem[] = [
  { id: 'home', label: 'Home', icon: <HomeIcon /> },
  { id: 'profile', label: 'Profile', icon: <UserIcon /> },
  { id: 'analytics', label: 'Analytics', icon: <ChartIcon /> },
  { id: 'settings', label: 'Settings', icon: <SettingsIcon /> },
];

const permissionTabs: TabItem[] = [
  { id: 'granted', label: 'Granted' },
  { id: 'pending', label: 'Pending' },
  { id: 'revoked', label: 'Revoked' },
];

export const Default: Story = {
  render: () => (
    <Tabs tabs={basicTabs} defaultTab="overview" className="w-full max-w-2xl">
      <div className="p-4 bg-neutral-50 rounded border border-neutral-200">
        <h3 className="font-medium text-neutral-900 mb-2">Overview</h3>
        <p className="text-neutral-700">
          This is the overview panel with general information and summary data.
        </p>
      </div>
      <div className="p-4 bg-neutral-50 rounded border border-neutral-200">
        <h3 className="font-medium text-neutral-900 mb-2">Details</h3>
        <p className="text-neutral-700">
          Detailed information and specific data points are displayed here.
        </p>
      </div>
      <div className="p-4 bg-neutral-50 rounded border border-neutral-200">
        <h3 className="font-medium text-neutral-900 mb-2">Settings</h3>
        <p className="text-neutral-700">
          Configuration options and preferences can be adjusted here.
        </p>
      </div>
    </Tabs>
  ),
};

export const Variants: Story = {
  render: () => (
    <div className="space-y-12 w-full max-w-2xl">
      {/* Underline variant */}
      <div>
        <h3 className="text-sm font-medium text-neutral-500 uppercase tracking-wide mb-4">
          Underline (Default)
        </h3>
        <Tabs tabs={basicTabs} defaultTab="overview" variant="underline">
          <div className="p-4 bg-neutral-50 rounded">Overview content</div>
          <div className="p-4 bg-neutral-50 rounded">Details content</div>
          <div className="p-4 bg-neutral-50 rounded">Settings content</div>
        </Tabs>
      </div>

      {/* Pill variant */}
      <div>
        <h3 className="text-sm font-medium text-neutral-500 uppercase tracking-wide mb-4">
          Pill
        </h3>
        <Tabs tabs={basicTabs} defaultTab="overview" variant="pill">
          <div className="p-4 bg-neutral-50 rounded">Overview content</div>
          <div className="p-4 bg-neutral-50 rounded">Details content</div>
          <div className="p-4 bg-neutral-50 rounded">Settings content</div>
        </Tabs>
      </div>

      {/* Button variant */}
      <div>
        <h3 className="text-sm font-medium text-neutral-500 uppercase tracking-wide mb-4">
          Button
        </h3>
        <Tabs tabs={basicTabs} defaultTab="overview" variant="button">
          <div className="p-4 bg-neutral-50 rounded">Overview content</div>
          <div className="p-4 bg-neutral-50 rounded">Details content</div>
          <div className="p-4 bg-neutral-50 rounded">Settings content</div>
        </Tabs>
      </div>
    </div>
  ),
};

export const DisabledTabs: Story = {
  render: () => {
    const tabsWithDisabled: TabItem[] = [
      { id: 'active', label: 'Active' },
      { id: 'disabled', label: 'Disabled', disabled: true },
      { id: 'another', label: 'Another Active' },
      { id: 'locked', label: 'Locked', disabled: true },
    ];

    return (
      <Tabs
        tabs={tabsWithDisabled}
        defaultTab="active"
        className="w-full max-w-2xl"
      >
        <div className="p-4 bg-success-light border border-success-light rounded">
          <p className="text-success-dark">Active tab content is accessible</p>
        </div>
        <div className="p-4 bg-neutral-50 border border-neutral-200 rounded">
          <p className="text-neutral-500">This tab is disabled</p>
        </div>
        <div className="p-4 bg-success-light border border-success-light rounded">
          <p className="text-success-dark">Another active tab</p>
        </div>
        <div className="p-4 bg-neutral-50 border border-neutral-200 rounded">
          <p className="text-neutral-500">This tab is locked</p>
        </div>
      </Tabs>
    );
  },
};

export const Vertical: Story = {
  render: () => (
    <div className="space-y-12 w-full max-w-3xl">
      {/* Vertical underline */}
      <div>
        <h3 className="text-sm font-medium text-neutral-500 uppercase tracking-wide mb-4">
          Vertical Underline
        </h3>
        <Tabs
          tabs={basicTabs}
          defaultTab="overview"
          variant="underline"
          orientation="vertical"
        >
          <div className="p-4 bg-neutral-50 rounded border border-neutral-200">
            <h4 className="font-medium text-neutral-900 mb-2">Overview</h4>
            <p className="text-neutral-700">
              Vertical layout with underline variant.
            </p>
          </div>
          <div className="p-4 bg-neutral-50 rounded border border-neutral-200">
            <h4 className="font-medium text-neutral-900 mb-2">Details</h4>
            <p className="text-neutral-700">
              Detailed view in vertical orientation.
            </p>
          </div>
          <div className="p-4 bg-neutral-50 rounded border border-neutral-200">
            <h4 className="font-medium text-neutral-900 mb-2">Settings</h4>
            <p className="text-neutral-700">
              Settings panel in vertical layout.
            </p>
          </div>
        </Tabs>
      </div>

      {/* Vertical button */}
      <div>
        <h3 className="text-sm font-medium text-neutral-500 uppercase tracking-wide mb-4">
          Vertical Button
        </h3>
        <Tabs
          tabs={iconTabs}
          defaultTab="home"
          variant="button"
          orientation="vertical"
        >
          <div className="p-4 bg-neutral-50 rounded">
            Home dashboard content
          </div>
          <div className="p-4 bg-neutral-50 rounded">Profile information</div>
          <div className="p-4 bg-neutral-50 rounded">Analytics and reports</div>
          <div className="p-4 bg-neutral-50 rounded">
            Settings and preferences
          </div>
        </Tabs>
      </div>
    </div>
  ),
};

export const Sizes: Story = {
  render: () => (
    <div className="space-y-12 w-full max-w-2xl">
      {/* Small */}
      <div>
        <h3 className="text-sm font-medium text-neutral-500 uppercase tracking-wide mb-4">
          Small
        </h3>
        <Tabs tabs={basicTabs} defaultTab="overview" size="sm">
          <div className="p-3 bg-neutral-50 rounded text-sm">
            Small tab content
          </div>
          <div className="p-3 bg-neutral-50 rounded text-sm">
            Small tab content
          </div>
          <div className="p-3 bg-neutral-50 rounded text-sm">
            Small tab content
          </div>
        </Tabs>
      </div>

      {/* Medium (default) */}
      <div>
        <h3 className="text-sm font-medium text-neutral-500 uppercase tracking-wide mb-4">
          Medium (Default)
        </h3>
        <Tabs tabs={basicTabs} defaultTab="overview" size="md">
          <div className="p-4 bg-neutral-50 rounded">Medium tab content</div>
          <div className="p-4 bg-neutral-50 rounded">Medium tab content</div>
          <div className="p-4 bg-neutral-50 rounded">Medium tab content</div>
        </Tabs>
      </div>

      {/* Large */}
      <div>
        <h3 className="text-sm font-medium text-neutral-500 uppercase tracking-wide mb-4">
          Large
        </h3>
        <Tabs tabs={basicTabs} defaultTab="overview" size="lg">
          <div className="p-6 bg-neutral-50 rounded text-lg">
            Large tab content
          </div>
          <div className="p-6 bg-neutral-50 rounded text-lg">
            Large tab content
          </div>
          <div className="p-6 bg-neutral-50 rounded text-lg">
            Large tab content
          </div>
        </Tabs>
      </div>
    </div>
  ),
};

export const WithIcons: Story = {
  render: () => (
    <div className="space-y-12 w-full max-w-2xl">
      {/* Underline with icons */}
      <div>
        <h3 className="text-sm font-medium text-neutral-500 uppercase tracking-wide mb-4">
          Underline with Icons
        </h3>
        <Tabs tabs={iconTabs} defaultTab="home" variant="underline">
          <div className="p-4 bg-neutral-50 rounded">
            <h4 className="font-medium text-neutral-900 mb-2">
              Home Dashboard
            </h4>
            <p className="text-neutral-700">Welcome to your dashboard.</p>
          </div>
          <div className="p-4 bg-neutral-50 rounded">
            <h4 className="font-medium text-neutral-900 mb-2">User Profile</h4>
            <p className="text-neutral-700">Manage your profile information.</p>
          </div>
          <div className="p-4 bg-neutral-50 rounded">
            <h4 className="font-medium text-neutral-900 mb-2">Analytics</h4>
            <p className="text-neutral-700">
              View your analytics and insights.
            </p>
          </div>
          <div className="p-4 bg-neutral-50 rounded">
            <h4 className="font-medium text-neutral-900 mb-2">Settings</h4>
            <p className="text-neutral-700">Configure your preferences.</p>
          </div>
        </Tabs>
      </div>

      {/* Pill with icons */}
      <div>
        <h3 className="text-sm font-medium text-neutral-500 uppercase tracking-wide mb-4">
          Pill with Icons
        </h3>
        <Tabs tabs={iconTabs} defaultTab="home" variant="pill">
          <div className="p-4 bg-neutral-50 rounded">
            Home content with icons
          </div>
          <div className="p-4 bg-neutral-50 rounded">
            Profile content with icons
          </div>
          <div className="p-4 bg-neutral-50 rounded">
            Analytics content with icons
          </div>
          <div className="p-4 bg-neutral-50 rounded">
            Settings content with icons
          </div>
        </Tabs>
      </div>
    </div>
  ),
};

export const Controlled: Story = {
  render: () => {
    const [activeTab, setActiveTab] = useState('granted');

    return (
      <div className="w-full max-w-2xl space-y-4">
        <div className="p-4 bg-trust-light border border-neutral-200 rounded-lg">
          <p className="text-sm text-trust-deep mb-2">
            <strong>Controlled mode:</strong> The parent component manages the
            active tab state.
          </p>
          <p className="text-sm text-trust">
            Current active tab:{' '}
            <code className="px-2 py-0.5 bg-trust-light rounded">
              {activeTab}
            </code>
          </p>
        </div>

        <Tabs
          tabs={permissionTabs}
          selectedTab={activeTab}
          onTabChange={setActiveTab}
          variant="pill"
        >
          <div className="p-6 bg-success-light border border-success-light rounded">
            <h4 className="font-medium text-success-dark mb-3">
              Granted Permissions
            </h4>
            <ul className="space-y-2 text-success-dark">
              <li>• Read access to documents</li>
              <li>• Edit personal profile</li>
              <li>• View analytics dashboard</li>
            </ul>
          </div>
          <div className="p-6 bg-amber-50 border border-amber-200 rounded">
            <h4 className="font-medium text-amber-900 mb-3">
              Pending Approvals
            </h4>
            <ul className="space-y-2 text-amber-800">
              <li>• Admin access - awaiting approval</li>
              <li>• Delete permissions - under review</li>
            </ul>
          </div>
          <div className="p-6 bg-red-50 border border-red-200 rounded">
            <h4 className="font-medium text-red-900 mb-3">Revoked Access</h4>
            <ul className="space-y-2 text-red-800">
              <li>• Export data - revoked on 2024-01-15</li>
              <li>• Share externally - revoked on 2024-01-10</li>
            </ul>
          </div>
        </Tabs>

        <div className="flex gap-2 pt-4">
          <button
            onClick={() => setActiveTab('granted')}
            className="px-4 py-2 text-sm border border-neutral-300 rounded hover:bg-neutral-50 transition-colors"
          >
            Go to Granted
          </button>
          <button
            onClick={() => setActiveTab('pending')}
            className="px-4 py-2 text-sm border border-neutral-300 rounded hover:bg-neutral-50 transition-colors"
          >
            Go to Pending
          </button>
          <button
            onClick={() => setActiveTab('revoked')}
            className="px-4 py-2 text-sm border border-neutral-300 rounded hover:bg-neutral-50 transition-colors"
          >
            Go to Revoked
          </button>
        </div>
      </div>
    );
  },
};

export const ConsentManagementExample: Story = {
  render: () => {
    const [activeTab, setActiveTab] = useState('current');

    const consentTabs: TabItem[] = [
      { id: 'current', label: 'Current Consents', icon: <HomeIcon /> },
      { id: 'history', label: 'History', icon: <ChartIcon /> },
      { id: 'preferences', label: 'Preferences', icon: <SettingsIcon /> },
    ];

    return (
      <div className="w-full max-w-3xl p-6 bg-white border border-neutral-200 rounded-lg shadow-float">
        <div className="mb-6">
          <h2 className="text-2xl font-display font-medium text-trust-deep mb-2">
            Consent Management
          </h2>
          <p className="text-neutral-600">
            Manage your data sharing permissions and consent preferences
          </p>
        </div>

        <Tabs
          tabs={consentTabs}
          selectedTab={activeTab}
          onTabChange={setActiveTab}
          variant="underline"
          size="md"
        >
          {/* Current Consents */}
          <div className="space-y-4">
            <div className="p-4 border border-neutral-200 rounded-lg">
              <div className="flex items-start justify-between mb-2">
                <h4 className="font-medium text-neutral-900">
                  Healthcare Provider Access
                </h4>
                <span className="px-2 py-1 text-xs font-medium bg-success-light text-success-dark rounded">
                  Active
                </span>
              </div>
              <p className="text-sm text-neutral-600 mb-3">
                Allows Dr. Smith to access your medical records
              </p>
              <div className="flex items-center justify-between text-xs text-neutral-500">
                <span>Granted: Jan 15, 2024</span>
                <span>Expires: Jan 15, 2025</span>
              </div>
            </div>
            <div className="p-4 border border-neutral-200 rounded-lg">
              <div className="flex items-start justify-between mb-2">
                <h4 className="font-medium text-neutral-900">
                  Research Study Participation
                </h4>
                <span className="px-2 py-1 text-xs font-medium bg-success-light text-success-dark rounded">
                  Active
                </span>
              </div>
              <p className="text-sm text-neutral-600 mb-3">
                Anonymous data sharing for clinical research
              </p>
              <div className="flex items-center justify-between text-xs text-neutral-500">
                <span>Granted: Dec 1, 2023</span>
                <span>Expires: Dec 1, 2024</span>
              </div>
            </div>
          </div>

          {/* History */}
          <div className="space-y-3">
            <div className="p-3 bg-neutral-50 border border-neutral-200 rounded">
              <div className="flex items-center justify-between mb-1">
                <span className="text-sm font-medium text-neutral-900">
                  Lab Results Access
                </span>
                <span className="text-xs text-neutral-500">Revoked</span>
              </div>
              <p className="text-xs text-neutral-600">
                Revoked on Jan 10, 2024
              </p>
            </div>
            <div className="p-3 bg-neutral-50 border border-neutral-200 rounded">
              <div className="flex items-center justify-between mb-1">
                <span className="text-sm font-medium text-neutral-900">
                  Pharmacy Access
                </span>
                <span className="text-xs text-neutral-500">Expired</span>
              </div>
              <p className="text-xs text-neutral-600">
                Expired on Dec 31, 2023
              </p>
            </div>
            <div className="p-3 bg-neutral-50 border border-neutral-200 rounded">
              <div className="flex items-center justify-between mb-1">
                <span className="text-sm font-medium text-neutral-900">
                  Insurance Verification
                </span>
                <span className="text-xs text-neutral-500">Completed</span>
              </div>
              <p className="text-xs text-neutral-600">
                Completed on Nov 15, 2023
              </p>
            </div>
          </div>

          {/* Preferences */}
          <div className="space-y-4">
            <div className="p-4 border border-neutral-200 rounded-lg">
              <h4 className="font-medium text-neutral-900 mb-3">
                Default Consent Duration
              </h4>
              <select className="w-full px-3 py-2 border border-neutral-300 rounded focus:outline-none focus:ring-2 focus:ring-trust-hover">
                <option>30 days</option>
                <option>90 days</option>
                <option selected>1 year</option>
                <option>Until revoked</option>
              </select>
            </div>
            <div className="p-4 border border-neutral-200 rounded-lg">
              <h4 className="font-medium text-neutral-900 mb-3">
                Notification Preferences
              </h4>
              <div className="space-y-2">
                <label className="flex items-center gap-2">
                  <input type="checkbox" checked className="rounded" />
                  <span className="text-sm text-neutral-700">
                    Email me when consent is requested
                  </span>
                </label>
                <label className="flex items-center gap-2">
                  <input type="checkbox" checked className="rounded" />
                  <span className="text-sm text-neutral-700">
                    Notify before consent expires
                  </span>
                </label>
                <label className="flex items-center gap-2">
                  <input type="checkbox" className="rounded" />
                  <span className="text-sm text-neutral-700">
                    Weekly consent summary
                  </span>
                </label>
              </div>
            </div>
          </div>
        </Tabs>
      </div>
    );
  },
};

export const Playground: Story = {
  render: (args) => {
    const [activeTab, setActiveTab] = useState(args.defaultTab || 'overview');

    return (
      <Tabs
        {...args}
        selectedTab={activeTab}
        onTabChange={setActiveTab}
        className="w-full max-w-2xl"
      >
        <div className="p-4 bg-neutral-50 rounded border border-neutral-200">
          <h3 className="font-medium text-neutral-900 mb-2">Overview Panel</h3>
          <p className="text-neutral-700">
            This is the overview content panel.
          </p>
        </div>
        <div className="p-4 bg-neutral-50 rounded border border-neutral-200">
          <h3 className="font-medium text-neutral-900 mb-2">Details Panel</h3>
          <p className="text-neutral-700">This is the details content panel.</p>
        </div>
        <div className="p-4 bg-neutral-50 rounded border border-neutral-200">
          <h3 className="font-medium text-neutral-900 mb-2">Settings Panel</h3>
          <p className="text-neutral-700">
            This is the settings content panel.
          </p>
        </div>
      </Tabs>
    );
  },
  args: {
    tabs: basicTabs,
    variant: 'underline',
    size: 'md',
    orientation: 'horizontal',
    defaultTab: 'overview',
  },
  argTypes: {
    variant: {
      control: 'select',
      options: ['underline', 'pill', 'button'],
      description: 'Visual style of the tabs',
    },
    size: {
      control: 'select',
      options: ['sm', 'md', 'lg'],
      description: 'Size of the tabs',
    },
    orientation: {
      control: 'select',
      options: ['horizontal', 'vertical'],
      description: 'Tab orientation',
    },
    defaultTab: {
      control: 'text',
      description: 'ID of the initially selected tab',
    },
  },
};
