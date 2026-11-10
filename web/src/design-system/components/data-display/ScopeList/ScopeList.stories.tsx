/**
 * ScopeList Storybook Stories
 *
 * Comprehensive examples of the ScopeList component for OAuth consent flows.
 * Demonstrates all features including grouping, selection, search, and risk indicators.
 */

import type { Meta, StoryObj } from '@storybook/react';
import { ScopeList, type Scope } from './ScopeList';
import { useState } from 'react';

const meta: Meta<typeof ScopeList> = {
  title: 'Data Display/ScopeList',
  component: ScopeList,
  parameters: {
    layout: 'padded',
    docs: {
      description: {
        component:
          'Domain-specific component for displaying OAuth scopes/permissions in consent UI. Supports grouping, selection, search, and risk indicators.',
      },
    },
  },
  tags: ['autodocs'],
  argTypes: {
    size: {
      control: 'select',
      options: ['compact', 'default'],
      description: 'Size variant of the scope list',
    },
    selectable: {
      control: 'boolean',
      description: 'Enable checkbox selection for scopes',
    },
    searchable: {
      control: 'boolean',
      description: 'Enable search/filter functionality',
    },
    expandable: {
      control: 'boolean',
      description: 'Enable expand/collapse for categories',
    },
    loading: {
      control: 'boolean',
      description: 'Show loading state with skeletons',
    },
  },
};

export default meta;
type Story = StoryObj<typeof ScopeList>;

// Sample scope data
const basicScopes: Scope[] = [
  {
    id: '1',
    name: 'user:read',
    description: 'Read your user profile information',
    category: 'Profile',
  },
  {
    id: '2',
    name: 'user:write',
    description: 'Update your user profile information',
    category: 'Profile',
  },
  {
    id: '3',
    name: 'email:read',
    description: 'Read your email messages',
    category: 'Email',
  },
  {
    id: '4',
    name: 'email:send',
    description: 'Send emails on your behalf',
    category: 'Email',
  },
];

const categorizedScopes: Scope[] = [
  // Profile scopes
  {
    id: 'profile-1',
    name: 'user:read',
    description: 'View your basic profile information',
    category: 'Profile',
    riskLevel: 'low',
  },
  {
    id: 'profile-2',
    name: 'user:write',
    description: 'Update your profile information',
    category: 'Profile',
    riskLevel: 'medium',
  },
  {
    id: 'profile-3',
    name: 'user:delete',
    description: 'Delete your account',
    category: 'Profile',
    riskLevel: 'high',
  },
  // Email scopes
  {
    id: 'email-1',
    name: 'email:read',
    description: 'Read your email messages',
    category: 'Email',
    riskLevel: 'medium',
  },
  {
    id: 'email-2',
    name: 'email:send',
    description: 'Send emails on your behalf',
    category: 'Email',
    riskLevel: 'high',
  },
  {
    id: 'email-3',
    name: 'email:metadata',
    description: 'View email headers and metadata',
    category: 'Email',
    riskLevel: 'low',
  },
  // Calendar scopes
  {
    id: 'calendar-1',
    name: 'calendar:read',
    description: 'View your calendar events',
    category: 'Calendar',
    riskLevel: 'low',
  },
  {
    id: 'calendar-2',
    name: 'calendar:write',
    description: 'Create and modify calendar events',
    category: 'Calendar',
    riskLevel: 'medium',
  },
  // Contacts scopes
  {
    id: 'contacts-1',
    name: 'contacts:read',
    description: 'View your contacts',
    category: 'Contacts',
    riskLevel: 'low',
  },
  {
    id: 'contacts-2',
    name: 'contacts:write',
    description: 'Add and modify contacts',
    category: 'Contacts',
    riskLevel: 'medium',
  },
];

const googleScopes: Scope[] = [
  {
    id: 'google-1',
    name: 'https://www.googleapis.com/auth/userinfo.profile',
    description: 'View your basic profile info',
    category: 'Profile',
    riskLevel: 'low',
  },
  {
    id: 'google-2',
    name: 'https://www.googleapis.com/auth/userinfo.email',
    description: 'View your email address',
    category: 'Profile',
    riskLevel: 'low',
  },
  {
    id: 'google-3',
    name: 'https://www.googleapis.com/auth/gmail.readonly',
    description: 'Read your Gmail messages',
    category: 'Email',
    riskLevel: 'medium',
  },
  {
    id: 'google-4',
    name: 'https://www.googleapis.com/auth/gmail.send',
    description: 'Send emails from your account',
    category: 'Email',
    riskLevel: 'high',
  },
  {
    id: 'google-5',
    name: 'https://www.googleapis.com/auth/calendar',
    description: 'Manage your calendars',
    category: 'Calendar',
    riskLevel: 'medium',
  },
  {
    id: 'google-6',
    name: 'https://www.googleapis.com/auth/calendar.events',
    description: 'View and edit events on all your calendars',
    category: 'Calendar',
    riskLevel: 'medium',
  },
  {
    id: 'google-7',
    name: 'https://www.googleapis.com/auth/drive.file',
    description:
      'View and manage Drive files that you have opened with this app',
    category: 'Storage',
    riskLevel: 'medium',
  },
  {
    id: 'google-8',
    name: 'https://www.googleapis.com/auth/drive',
    description: 'View and manage all of your Google Drive files',
    category: 'Storage',
    riskLevel: 'high',
  },
];

const githubScopes: Scope[] = [
  {
    id: 'github-1',
    name: 'repo',
    description: 'Full control of private repositories',
    category: 'Repositories',
    riskLevel: 'high',
  },
  {
    id: 'github-2',
    name: 'repo:status',
    description: 'Access commit status',
    category: 'Repositories',
    riskLevel: 'low',
  },
  {
    id: 'github-3',
    name: 'public_repo',
    description: 'Access public repositories',
    category: 'Repositories',
    riskLevel: 'low',
  },
  {
    id: 'github-4',
    name: 'user',
    description: 'Update ALL user data',
    category: 'User',
    riskLevel: 'high',
  },
  {
    id: 'github-5',
    name: 'read:user',
    description: 'Read ALL user profile data',
    category: 'User',
    riskLevel: 'low',
  },
  {
    id: 'github-6',
    name: 'user:email',
    description: 'Access user email addresses (read-only)',
    category: 'User',
    riskLevel: 'low',
  },
  {
    id: 'github-7',
    name: 'admin:org',
    description: 'Full control of orgs and teams, read and write org projects',
    category: 'Admin',
    riskLevel: 'high',
  },
  {
    id: 'github-8',
    name: 'read:org',
    description: 'Read org and team membership, read org projects',
    category: 'Admin',
    riskLevel: 'medium',
  },
];

/**
 * Basic scope list without any interactions
 */
export const BasicScopeList: Story = {
  args: {
    scopes: basicScopes,
    size: 'default',
    selectable: false,
    searchable: false,
    expandable: true,
  },
};

/**
 * Scope list with categories grouped and expandable
 */
export const WithCategories: Story = {
  args: {
    scopes: categorizedScopes,
    size: 'default',
    selectable: false,
    searchable: false,
    expandable: true,
  },
};

/**
 * Scope list with checkbox selection enabled
 */
export const Selectable: Story = {
  render: (args) => {
    const [selectedIds, setSelectedIds] = useState<string[]>([]);

    return (
      <div className="space-y-4">
        <div className="p-4 bg-neutral-100 rounded-lg">
          <p className="text-sm font-medium text-neutral-700">
            Selected scopes:
          </p>
          <p className="text-sm text-neutral-600 mt-1">
            {selectedIds.length > 0 ? selectedIds.join(', ') : 'None'}
          </p>
        </div>
        <ScopeList
          {...args}
          scopes={categorizedScopes}
          selectable
          onSelectionChange={setSelectedIds}
        />
      </div>
    );
  },
};

/**
 * Scope list with risk level indicators
 */
export const WithRiskLevels: Story = {
  args: {
    scopes: categorizedScopes,
    size: 'default',
    selectable: false,
    searchable: false,
    expandable: true,
  },
  parameters: {
    docs: {
      description: {
        story:
          'Scopes can be color-coded by risk level: low (green), medium (yellow), high (red).',
      },
    },
  },
};

/**
 * Compact size variant with reduced padding
 */
export const CompactSize: Story = {
  args: {
    scopes: categorizedScopes,
    size: 'compact',
    selectable: false,
    searchable: false,
    expandable: true,
  },
};

/**
 * Default size variant with standard padding
 */
export const DefaultSize: Story = {
  args: {
    scopes: categorizedScopes,
    size: 'default',
    selectable: false,
    searchable: false,
    expandable: true,
  },
};

/**
 * Scope list with search/filter functionality
 */
export const WithSearch: Story = {
  args: {
    scopes: categorizedScopes,
    size: 'default',
    selectable: false,
    searchable: true,
    expandable: true,
  },
  parameters: {
    docs: {
      description: {
        story: 'Search filters scopes by name, description, or category.',
      },
    },
  },
};

/**
 * Scope list with icons for each category
 */
export const WithIcons: Story = {
  args: {
    scopes: categorizedScopes,
    size: 'default',
    selectable: false,
    searchable: false,
    expandable: true,
  },
  parameters: {
    docs: {
      description: {
        story:
          'Category icons are automatically displayed based on category name.',
      },
    },
  },
};

/**
 * Loading state with skeleton placeholders
 */
export const LoadingState: Story = {
  args: {
    scopes: categorizedScopes,
    size: 'default',
    loading: true,
  },
};

/**
 * Empty state when no scopes are available
 */
export const EmptyState: Story = {
  args: {
    scopes: [],
    size: 'default',
    selectable: false,
    searchable: false,
    expandable: true,
  },
};

/**
 * Custom empty state content
 */
export const CustomEmptyState: Story = {
  args: {
    scopes: [],
    size: 'default',
    empty: (
      <div>
        <h3 className="text-lg font-medium text-neutral-900 mb-1">
          No permissions requested
        </h3>
        <p className="text-sm text-neutral-600">
          This application doesn't require any special permissions.
        </p>
      </div>
    ),
  },
};

/**
 * Scope list with some scopes pre-selected
 */
export const PreSelected: Story = {
  render: (args) => {
    const scopesWithPreSelection = categorizedScopes.map((scope) => ({
      ...scope,
      selected: scope.riskLevel === 'low',
    }));

    const [selectedIds, setSelectedIds] = useState<string[]>(
      scopesWithPreSelection.filter((s) => s.selected).map((s) => s.id),
    );

    return (
      <div className="space-y-4">
        <div className="p-4 bg-neutral-100 rounded-lg">
          <p className="text-sm font-medium text-neutral-700">
            Selected scopes:
          </p>
          <p className="text-sm text-neutral-600 mt-1">
            {selectedIds.length > 0 ? selectedIds.join(', ') : 'None'}
          </p>
        </div>
        <ScopeList
          {...args}
          scopes={scopesWithPreSelection}
          selectable
          onSelectionChange={setSelectedIds}
        />
      </div>
    );
  },
};

/**
 * Real OAuth scopes from Google APIs
 */
export const GoogleOAuthScopes: Story = {
  args: {
    scopes: googleScopes,
    size: 'default',
    selectable: false,
    searchable: true,
    expandable: true,
  },
  parameters: {
    docs: {
      description: {
        story: 'Example of real Google OAuth 2.0 scope URIs with risk levels.',
      },
    },
  },
};

/**
 * Real OAuth scopes from GitHub
 */
export const GitHubOAuthScopes: Story = {
  args: {
    scopes: githubScopes,
    size: 'default',
    selectable: false,
    searchable: true,
    expandable: true,
  },
  parameters: {
    docs: {
      description: {
        story: 'Example of real GitHub OAuth scope names with risk levels.',
      },
    },
  },
};

/**
 * Interactive consent flow with scope selection
 */
export const ConsentFlow: Story = {
  render: (args) => {
    const [selectedIds, setSelectedIds] = useState<string[]>(
      googleScopes.filter((s) => s.riskLevel === 'low').map((s) => s.id),
    );

    const selectedScopes = googleScopes.filter((s) =>
      selectedIds.includes(s.id),
    );
    const highRiskCount = selectedScopes.filter(
      (s) => s.riskLevel === 'high',
    ).length;

    return (
      <div className="max-w-2xl mx-auto space-y-6">
        {/* Header */}
        <div className="text-center">
          <h2 className="text-2xl font-medium text-neutral-900 mb-2">
            Grant Permissions
          </h2>
          <p className="text-sm text-neutral-600">
            Application "Example App" is requesting access to your account
          </p>
        </div>

        {/* Warning for high-risk scopes */}
        {highRiskCount > 0 && (
          <div className="p-4 bg-error-light border border-error-primary/20 rounded-lg">
            <div className="flex items-start gap-3">
              <svg
                className="w-5 h-5 text-error-primary flex-shrink-0 mt-0.5"
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
                <p className="text-sm font-medium text-error-dark">
                  High-risk permissions selected
                </p>
                <p className="text-xs text-error-dark mt-1">
                  You've selected {highRiskCount} high-risk permission
                  {highRiskCount > 1 ? 's' : ''}. Please review carefully before
                  granting access.
                </p>
              </div>
            </div>
          </div>
        )}

        {/* Scope list */}
        <ScopeList
          {...args}
          scopes={googleScopes}
          selectable
          searchable
          onSelectionChange={setSelectedIds}
        />

        {/* Summary */}
        <div className="p-4 bg-neutral-50 rounded-lg">
          <p className="text-sm font-medium text-neutral-700">Summary</p>
          <p className="text-sm text-neutral-600 mt-1">
            {selectedIds.length} of {googleScopes.length} permissions selected
          </p>
        </div>

        {/* Actions */}
        <div className="flex gap-3 justify-end">
          <button className="px-4 py-2 text-sm font-medium text-neutral-700 bg-white border border-neutral-300 rounded-lg hover:bg-neutral-50">
            Cancel
          </button>
          <button
            className="px-4 py-2 text-sm font-medium text-white bg-trust-deep border border-transparent rounded-lg hover:bg-trust-deep/90 disabled:opacity-50"
            disabled={selectedIds.length === 0}
          >
            Grant Access
          </button>
        </div>
      </div>
    );
  },
};

/**
 * All features combined: selection, search, expandable, with risk levels
 */
export const AllFeatures: Story = {
  render: (args) => {
    const [selectedIds, setSelectedIds] = useState<string[]>([]);

    return (
      <div className="space-y-4">
        <div className="p-4 bg-neutral-100 rounded-lg">
          <p className="text-sm font-medium text-neutral-700 mb-2">
            Features enabled:
          </p>
          <ul className="text-xs text-neutral-600 space-y-1">
            <li>✓ Selectable scopes with checkboxes</li>
            <li>✓ Search/filter by name, description, or category</li>
            <li>✓ Expandable/collapsible categories</li>
            <li>✓ Risk level badges (low, medium, high)</li>
            <li>✓ Category icons</li>
          </ul>
          <p className="text-sm font-medium text-neutral-700 mt-3">
            Selected: {selectedIds.length} scope
            {selectedIds.length !== 1 ? 's' : ''}
          </p>
        </div>
        <ScopeList
          {...args}
          scopes={categorizedScopes}
          selectable
          searchable
          expandable
          onSelectionChange={setSelectedIds}
        />
      </div>
    );
  },
};

/**
 * Non-expandable variant (all categories always visible)
 */
export const NonExpandable: Story = {
  args: {
    scopes: categorizedScopes,
    size: 'default',
    selectable: false,
    searchable: false,
    expandable: false,
  },
  parameters: {
    docs: {
      description: {
        story:
          'When expandable is false, all categories remain open and cannot be collapsed.',
      },
    },
  },
};
