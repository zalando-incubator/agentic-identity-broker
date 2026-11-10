/**
 * EmptyState Component Stories
 *
 * Demonstrates various empty state scenarios for no data, no results,
 * no permissions, and onboarding experiences.
 */

import type { Meta, StoryObj } from '@storybook/react';
import { EmptyState } from './EmptyState';

// Icon components for stories
const SearchIcon = () => (
  <svg
    className="w-full h-full text-neutral-400"
    fill="none"
    viewBox="0 0 24 24"
    stroke="currentColor"
    aria-hidden="true"
  >
    <path
      strokeLinecap="round"
      strokeLinejoin="round"
      strokeWidth={1.5}
      d="M21 21l-6-6m2-5a7 7 0 11-14 0 7 7 0 0114 0z"
    />
  </svg>
);

const LockIcon = () => (
  <svg
    className="w-full h-full text-neutral-400"
    fill="none"
    viewBox="0 0 24 24"
    stroke="currentColor"
    aria-hidden="true"
  >
    <path
      strokeLinecap="round"
      strokeLinejoin="round"
      strokeWidth={1.5}
      d="M12 15v2m-6 4h12a2 2 0 002-2v-6a2 2 0 00-2-2H6a2 2 0 00-2 2v6a2 2 0 002 2zm10-10V7a4 4 0 00-8 0v4h8z"
    />
  </svg>
);

const DocumentIcon = () => (
  <svg
    className="w-full h-full text-neutral-400"
    fill="none"
    viewBox="0 0 24 24"
    stroke="currentColor"
    aria-hidden="true"
  >
    <path
      strokeLinecap="round"
      strokeLinejoin="round"
      strokeWidth={1.5}
      d="M9 12h6m-6 4h6m2 5H7a2 2 0 01-2-2V5a2 2 0 012-2h5.586a1 1 0 01.707.293l5.414 5.414a1 1 0 01.293.707V19a2 2 0 01-2 2z"
    />
  </svg>
);

const InboxIcon = () => (
  <svg
    className="w-full h-full text-neutral-400"
    fill="none"
    viewBox="0 0 24 24"
    stroke="currentColor"
    aria-hidden="true"
  >
    <path
      strokeLinecap="round"
      strokeLinejoin="round"
      strokeWidth={1.5}
      d="M20 13V6a2 2 0 00-2-2H6a2 2 0 00-2 2v7m16 0v5a2 2 0 01-2 2H6a2 2 0 01-2-2v-5m16 0h-2.586a1 1 0 00-.707.293l-2.414 2.414a1 1 0 01-.707.293h-3.172a1 1 0 01-.707-.293l-2.414-2.414A1 1 0 006.586 13H4"
    />
  </svg>
);

const StarIcon = () => (
  <svg
    className="w-full h-full text-neutral-400"
    fill="none"
    viewBox="0 0 24 24"
    stroke="currentColor"
    aria-hidden="true"
  >
    <path
      strokeLinecap="round"
      strokeLinejoin="round"
      strokeWidth={1.5}
      d="M11.049 2.927c.3-.921 1.603-.921 1.902 0l1.519 4.674a1 1 0 00.95.69h4.915c.969 0 1.371 1.24.588 1.81l-3.976 2.888a1 1 0 00-.363 1.118l1.518 4.674c.3.922-.755 1.688-1.538 1.118l-3.976-2.888a1 1 0 00-1.176 0l-3.976 2.888c-.783.57-1.838-.197-1.538-1.118l1.518-4.674a1 1 0 00-.363-1.118l-3.976-2.888c-.784-.57-.38-1.81.588-1.81h4.914a1 1 0 00.951-.69l1.519-4.674z"
    />
  </svg>
);

const ClipboardIcon = () => (
  <svg
    className="w-full h-full text-neutral-400"
    fill="none"
    viewBox="0 0 24 24"
    stroke="currentColor"
    aria-hidden="true"
  >
    <path
      strokeLinecap="round"
      strokeLinejoin="round"
      strokeWidth={1.5}
      d="M9 5H7a2 2 0 00-2 2v12a2 2 0 002 2h10a2 2 0 002-2V7a2 2 0 00-2-2h-2M9 5a2 2 0 002 2h2a2 2 0 002-2M9 5a2 2 0 012-2h2a2 2 0 012 2"
    />
  </svg>
);

// Illustration component for hero-style empty states
const WelcomeIllustration = () => (
  <svg
    className="w-full h-full text-trust-soft"
    viewBox="0 0 200 200"
    fill="none"
    xmlns="http://www.w3.org/2000/svg"
    aria-hidden="true"
  >
    <circle cx="100" cy="100" r="80" fill="currentColor" opacity="0.1" />
    <circle cx="100" cy="100" r="60" fill="currentColor" opacity="0.2" />
    <path
      d="M100 60v80M60 100h80"
      stroke="currentColor"
      strokeWidth="8"
      strokeLinecap="round"
      opacity="0.4"
    />
  </svg>
);

const meta = {
  title: 'Design System/Feedback/EmptyState',
  component: EmptyState,
  parameters: {
    layout: 'padded',
  },
  tags: ['autodocs'],
  argTypes: {
    title: {
      control: 'text',
      description: 'Main heading text (required)',
    },
    description: {
      control: 'text',
      description: 'Supporting description text (optional)',
    },
    size: {
      control: 'select',
      options: ['compact', 'default', 'expanded'],
      description: 'Size variant for spacing and typography',
    },
    icon: {
      control: false,
      description: 'Icon or illustration to display (decorative)',
    },
    primaryAction: {
      control: false,
      description: 'Primary call-to-action button',
    },
    secondaryAction: {
      control: false,
      description: 'Secondary action button',
    },
  },
} satisfies Meta<typeof EmptyState>;

export default meta;
type Story = StoryObj<typeof meta>;

/**
 * Default empty state with icon and message.
 * Basic usage for simple empty scenarios.
 */
export const Default: Story = {
  args: {
    title: 'No items yet',
    description: 'Get started by creating your first item.',
  },
};

/**
 * Empty state with a primary action button.
 * Encourages users to take action to populate the empty state.
 */
export const WithAction: Story = {
  args: {
    title: 'No consents found',
    description:
      "You haven't granted any permissions yet. Connect your first application to get started.",
    primaryAction: {
      label: 'Add Application',
      onClick: () => alert('Add application clicked'),
    },
  },
};

/**
 * Empty state with both primary and secondary actions.
 * Provides multiple pathways for users to proceed.
 */
export const WithMultipleActions: Story = {
  args: {
    title: 'Build your first workflow',
    description:
      'Workflows help automate consent management tasks. Start from scratch or use a template.',
    primaryAction: {
      label: 'Create Workflow',
      onClick: () => alert('Create workflow clicked'),
    },
    secondaryAction: {
      label: 'Browse Templates',
      onClick: () => alert('Browse templates clicked'),
    },
  },
};

/**
 * Empty state for search/filter results.
 * Shows when user searches or filters but no results match.
 */
export const NoResults: Story = {
  args: {
    icon: <SearchIcon />,
    title: 'No results found',
    description:
      "We couldn't find any matches for your search. Try adjusting your filters or search terms.",
    primaryAction: {
      label: 'Clear Filters',
      onClick: () => alert('Clear filters clicked'),
    },
  },
};

/**
 * Empty state for permission/access restrictions.
 * Shows when user lacks required permissions to view content.
 */
export const NoPermissions: Story = {
  args: {
    icon: <LockIcon />,
    title: 'Access restricted',
    description:
      "You don't have permission to view this content. Contact your administrator to request access.",
    primaryAction: {
      label: 'Request Access',
      onClick: () => alert('Request access clicked'),
    },
    secondaryAction: {
      label: 'Go Back',
      onClick: () => alert('Go back clicked'),
    },
  },
};

/**
 * Compact size variant for inline or sidebar use.
 * Reduces spacing and text size for constrained layouts.
 */
export const Compact: Story = {
  render: () => (
    <div className="max-w-xs border border-neutral-200 rounded-lg p-4">
      <EmptyState
        size="compact"
        icon={<InboxIcon />}
        title="No messages"
        description="Your inbox is empty."
        primaryAction={{
          label: 'Compose',
          onClick: () => alert('Compose clicked'),
        }}
      />
    </div>
  ),
  args: {
    size: 'compact',
    title: 'No messages',
    description: 'Your inbox is empty.',
  },
};

/**
 * Expanded size variant for prominent display.
 * Increases spacing and text size for onboarding or hero sections.
 */
export const Expanded: Story = {
  args: {
    size: 'expanded',
    icon: <StarIcon />,
    title: 'Welcome to Consent Manager',
    description:
      'Take control of your data privacy. Start by connecting your first application and managing consent preferences.',
    primaryAction: {
      label: 'Get Started',
      onClick: () => alert('Get started clicked'),
    },
    secondaryAction: {
      label: 'Learn More',
      onClick: () => alert('Learn more clicked'),
    },
  },
};

/**
 * Empty state with custom illustration.
 * Large hero-style empty state for onboarding flows.
 */
export const WithIllustration: Story = {
  args: {
    size: 'expanded',
    icon: <WelcomeIllustration />,
    title: 'Start managing consent requests',
    description:
      'Centralize all your consent management in one place. Review requests, grant permissions, and track usage across all connected applications.',
    primaryAction: {
      label: 'Connect First App',
      onClick: () => alert('Connect app clicked'),
    },
    secondaryAction: {
      label: 'View Documentation',
      onClick: () => alert('View docs clicked'),
    },
  },
};

/**
 * Interactive playground with all controls.
 * Experiment with different combinations of props.
 */
export const Playground: Story = {
  args: {
    title: 'Customize this empty state',
    description:
      'Use the controls below to experiment with different props and configurations.',
    size: 'default',
    primaryAction: {
      label: 'Primary Action',
      onClick: () => alert('Primary action clicked'),
    },
    secondaryAction: {
      label: 'Secondary Action',
      onClick: () => alert('Secondary action clicked'),
    },
  },
};

/**
 * Real-world consent management scenarios.
 * Demonstrates various empty states in context.
 */
export const ConsentScenarios: Story = {
  render: () => (
    <div className="space-y-12 max-w-4xl mx-auto">
      {/* No active consents */}
      <div className="border border-neutral-200 rounded-lg p-8">
        <h4 className="text-sm font-medium text-neutral-700 mb-6">
          Active Consents Tab
        </h4>
        <EmptyState
          icon={<ClipboardIcon />}
          title="No active consents"
          description="You don't have any active consent grants. Applications you authorize will appear here."
          primaryAction={{
            label: 'Browse Applications',
            onClick: () => alert('Browse applications'),
          }}
        />
      </div>

      {/* No pending requests */}
      <div className="border border-neutral-200 rounded-lg p-8">
        <h4 className="text-sm font-medium text-neutral-700 mb-6">
          Pending Requests Tab
        </h4>
        <EmptyState
          icon={<InboxIcon />}
          title="All caught up!"
          description="You have no pending consent requests at the moment. New requests will appear here."
        />
      </div>

      {/* No revoked consents */}
      <div className="border border-neutral-200 rounded-lg p-8">
        <h4 className="text-sm font-medium text-neutral-700 mb-6">
          Revoked History Tab
        </h4>
        <EmptyState
          icon={<DocumentIcon />}
          title="No revoked consents"
          description="You haven't revoked any permissions yet. When you revoke access to an application, it will appear in this history."
          secondaryAction={{
            label: 'View Active Consents',
            onClick: () => alert('View active'),
          }}
        />
      </div>

      {/* Search with no results */}
      <div className="border border-neutral-200 rounded-lg p-8">
        <div className="mb-6">
          <h4 className="text-sm font-medium text-neutral-700 mb-2">
            Search Results
          </h4>
          <input
            type="text"
            placeholder="Search applications..."
            value="xyzabc123"
            readOnly
            className="w-full px-3 py-2 border border-neutral-300 rounded-md text-sm"
          />
        </div>
        <EmptyState
          size="compact"
          icon={<SearchIcon />}
          title="No matching applications"
          description="No applications found matching 'xyzabc123'. Try a different search term."
          primaryAction={{
            label: 'Clear Search',
            onClick: () => alert('Clear search'),
          }}
        />
      </div>
    </div>
  ),
  args: {
    title: 'Consent scenarios',
  },
};

/**
 * Size comparison showing all variants together.
 * Helps understand spacing and typography differences.
 */
export const SizeComparison: Story = {
  render: () => (
    <div className="space-y-8">
      <div className="border border-neutral-200 rounded-lg p-4">
        <h4 className="text-xs font-medium text-neutral-500 uppercase mb-4">
          Compact Size
        </h4>
        <EmptyState
          size="compact"
          icon={<DocumentIcon />}
          title="No documents"
          description="Upload your first document to get started."
          primaryAction={{
            label: 'Upload',
            onClick: () => alert('Upload clicked'),
          }}
        />
      </div>

      <div className="border border-neutral-200 rounded-lg p-6">
        <h4 className="text-xs font-medium text-neutral-500 uppercase mb-6">
          Default Size
        </h4>
        <EmptyState
          size="default"
          icon={<DocumentIcon />}
          title="No documents"
          description="Upload your first document to get started. You can drag and drop files or use the upload button."
          primaryAction={{
            label: 'Upload Document',
            onClick: () => alert('Upload clicked'),
          }}
          secondaryAction={{
            label: 'Learn More',
            onClick: () => alert('Learn more clicked'),
          }}
        />
      </div>

      <div className="border border-neutral-200 rounded-lg p-8">
        <h4 className="text-xs font-medium text-neutral-500 uppercase mb-8">
          Expanded Size
        </h4>
        <EmptyState
          size="expanded"
          icon={<DocumentIcon />}
          title="Get started with document management"
          description="Securely store and manage all your important documents in one place. Upload files up to 10MB in PDF, DOC, or TXT format."
          primaryAction={{
            label: 'Upload Your First Document',
            onClick: () => alert('Upload clicked'),
          }}
          secondaryAction={{
            label: 'View Documentation',
            onClick: () => alert('Learn more clicked'),
          }}
        />
      </div>
    </div>
  ),
  args: {
    title: 'Size comparison',
  },
};

/**
 * Accessibility features demonstration.
 * Shows semantic HTML and ARIA attributes in action.
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
            • Uses <code>role="status"</code> and{' '}
            <code>aria-live="polite"</code>
          </li>
          <li>
            • Icons are decorative with <code>aria-hidden="true"</code>
          </li>
          <li>• All interactive elements are keyboard accessible</li>
          <li>• Semantic heading hierarchy (h3 for title)</li>
          <li>• Focus indicators meet WCAG 2.1 AA requirements</li>
          <li>• Color contrast ratios comply with AA standards</li>
          <li>• Button labels are clear and descriptive</li>
        </ul>
      </div>

      <EmptyState
        icon={<SearchIcon />}
        title="Search complete"
        description="No results found for your query. The empty state is announced to screen readers via aria-live."
        primaryAction={{
          label: 'Refine Search',
          onClick: () => alert('Refine search'),
        }}
        secondaryAction={{
          label: 'Reset Filters',
          onClick: () => alert('Reset filters'),
        }}
      />
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
          {
            id: 'button-name',
            enabled: true,
          },
        ],
      },
    },
  },
  args: {
    title: 'Accessible empty state',
  },
};
