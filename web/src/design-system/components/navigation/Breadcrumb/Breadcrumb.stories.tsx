/**
 * Breadcrumb Component Stories
 *
 * Demonstrates all Breadcrumb variants, sizes, and usage patterns.
 */

import type { Meta, StoryObj } from '@storybook/react';
import { Breadcrumb } from './Breadcrumb';

const meta = {
  title: 'Design System/Navigation/Breadcrumb',
  component: Breadcrumb,
  parameters: {
    layout: 'padded',
  },
  tags: ['autodocs'],
  argTypes: {
    items: {
      control: 'object',
      description: 'Array of breadcrumb items',
      table: {
        type: { summary: 'BreadcrumbItem[]' },
      },
    },
    size: {
      control: 'select',
      options: ['sm', 'md', 'lg'],
      description: 'Breadcrumb size',
      table: {
        type: { summary: 'string' },
        defaultValue: { summary: 'md' },
      },
    },
    separator: {
      control: 'text',
      description: 'Custom separator element',
      table: {
        type: { summary: 'React.ReactNode' },
      },
    },
    maxItems: {
      control: 'number',
      description: 'Maximum number of items before truncation',
      table: {
        type: { summary: 'number' },
      },
    },
    showIcon: {
      control: 'boolean',
      description: 'Whether to show icons for items',
      table: {
        defaultValue: { summary: 'true' },
      },
    },
  },
} satisfies Meta<typeof Breadcrumb>;

export default meta;
type Story = StoryObj<typeof meta>;

// Common icons for examples
const HomeIcon = () => (
  <svg className="w-4 h-4" fill="currentColor" viewBox="0 0 20 20">
    <path d="M10.707 2.293a1 1 0 00-1.414 0l-7 7a1 1 0 001.414 1.414L4 10.414V17a1 1 0 001 1h2a1 1 0 001-1v-2a1 1 0 011-1h2a1 1 0 011 1v2a1 1 0 001 1h2a1 1 0 001-1v-6.586l.293.293a1 1 0 001.414-1.414l-7-7z" />
  </svg>
);

const FolderIcon = () => (
  <svg className="w-4 h-4" fill="currentColor" viewBox="0 0 20 20">
    <path d="M2 6a2 2 0 012-2h5l2 2h5a2 2 0 012 2v6a2 2 0 01-2 2H4a2 2 0 01-2-2V6z" />
  </svg>
);

const DocumentIcon = () => (
  <svg className="w-4 h-4" fill="currentColor" viewBox="0 0 20 20">
    <path
      fillRule="evenodd"
      d="M4 4a2 2 0 012-2h4.586A2 2 0 0112 2.586L15.414 6A2 2 0 0116 7.414V16a2 2 0 01-2 2H6a2 2 0 01-2-2V4z"
      clipRule="evenodd"
    />
  </svg>
);

const SettingsIcon = () => (
  <svg className="w-4 h-4" fill="currentColor" viewBox="0 0 20 20">
    <path
      fillRule="evenodd"
      d="M11.49 3.17c-.38-1.56-2.6-1.56-2.98 0a1.532 1.532 0 01-2.286.948c-1.372-.836-2.942.734-2.106 2.106.54.886.061 2.042-.947 2.287-1.561.379-1.561 2.6 0 2.978a1.532 1.532 0 01.947 2.287c-.836 1.372.734 2.942 2.106 2.106a1.532 1.532 0 012.287.947c.379 1.561 2.6 1.561 2.978 0a1.533 1.533 0 012.287-.947c1.372.836 2.942-.734 2.106-2.106a1.533 1.533 0 01.947-2.287c1.561-.379 1.561-2.6 0-2.978a1.532 1.532 0 01-.947-2.287c.836-1.372-.734-2.942-2.106-2.106a1.532 1.532 0 01-2.287-.947zM10 13a3 3 0 100-6 3 3 0 000 6z"
      clipRule="evenodd"
    />
  </svg>
);

/**
 * Default breadcrumb with basic navigation path.
 * Shows the most common usage with a simple three-level hierarchy.
 */
export const Default: Story = {
  args: {
    items: [
      { label: 'Home', href: '/' },
      { label: 'Settings', href: '/settings' },
      { label: 'Profile' },
    ],
    size: 'md',
  },
};

/**
 * Breadcrumb with icons for visual clarity.
 * Icons help users quickly identify page types and hierarchy levels.
 */
export const WithIcon: Story = {
  args: {
    items: [
      { label: 'Home', href: '/', icon: <HomeIcon /> },
      { label: 'Projects', href: '/projects', icon: <FolderIcon /> },
      { label: 'Document.pdf', icon: <DocumentIcon /> },
    ],
    size: 'md',
    showIcon: true,
  },
};

/**
 * Current page indicator.
 * The last item without an href is automatically styled as the current page
 * and marked with aria-current="page" for accessibility.
 */
export const CurrentPage: Story = {
  args: {
    items: [
      { label: 'Home', href: '/' },
      { label: 'Dashboard', href: '/dashboard' },
      { label: 'Analytics', href: '/dashboard/analytics' },
      { label: 'Reports' }, // Current page - no href
    ],
    size: 'md',
  },
};

/**
 * Truncated breadcrumb for long paths.
 * When maxItems is set, middle items are collapsed with an ellipsis indicator.
 * First and last items are always visible.
 */
export const Truncated: Story = {
  args: {
    items: [
      { label: 'Home', href: '/' },
      { label: 'Documents', href: '/documents' },
      { label: 'Work', href: '/documents/work' },
      { label: '2024', href: '/documents/work/2024' },
      { label: 'Q4', href: '/documents/work/2024/q4' },
      { label: 'Reports', href: '/documents/work/2024/q4/reports' },
      { label: 'Final Report.pdf' },
    ],
    maxItems: 4,
    size: 'md',
  },
};

/**
 * Collapsed middle section for very long paths.
 * Shows how the breadcrumb handles deep hierarchies by collapsing
 * intermediate items while preserving navigation context.
 */
export const Collapsed: Story = {
  render: () => (
    <div className="space-y-6">
      <div>
        <p className="text-sm font-medium text-neutral-700 mb-3">
          7 levels collapsed to 3 items
        </p>
        <Breadcrumb
          items={[
            { label: 'Root', href: '/' },
            { label: 'Level 1', href: '/1' },
            { label: 'Level 2', href: '/1/2' },
            { label: 'Level 3', href: '/1/2/3' },
            { label: 'Level 4', href: '/1/2/3/4' },
            { label: 'Level 5', href: '/1/2/3/4/5' },
            { label: 'Current Page' },
          ]}
          maxItems={3}
        />
      </div>

      <div>
        <p className="text-sm font-medium text-neutral-700 mb-3">
          10 levels collapsed to 5 items
        </p>
        <Breadcrumb
          items={[
            { label: 'Home', href: '/', icon: <HomeIcon /> },
            { label: 'Organizations', href: '/orgs' },
            { label: 'Acme Corp', href: '/orgs/acme' },
            { label: 'Teams', href: '/orgs/acme/teams' },
            { label: 'Engineering', href: '/orgs/acme/teams/eng' },
            { label: 'Backend', href: '/orgs/acme/teams/eng/backend' },
            {
              label: 'Services',
              href: '/orgs/acme/teams/eng/backend/services',
            },
            {
              label: 'Auth',
              href: '/orgs/acme/teams/eng/backend/services/auth',
            },
            {
              label: 'Config',
              href: '/orgs/acme/teams/eng/backend/services/auth/config',
            },
            { label: 'production.yaml' },
          ]}
          maxItems={5}
          showIcon
        />
      </div>
    </div>
  ),
};

/**
 * All breadcrumb sizes for different contexts.
 * - sm: Compact spaces, tight layouts
 * - md: Default size for most pages
 * - lg: Prominent navigation, spacious layouts
 */
export const Sizes: Story = {
  render: () => (
    <div className="space-y-6">
      <div>
        <p className="text-sm font-medium text-neutral-700 mb-3">Small (sm)</p>
        <Breadcrumb
          items={[
            { label: 'Home', href: '/', icon: <HomeIcon /> },
            { label: 'Settings', href: '/settings', icon: <SettingsIcon /> },
            { label: 'Profile' },
          ]}
          size="sm"
        />
      </div>

      <div>
        <p className="text-sm font-medium text-neutral-700 mb-3">Medium (md)</p>
        <Breadcrumb
          items={[
            { label: 'Home', href: '/', icon: <HomeIcon /> },
            { label: 'Settings', href: '/settings', icon: <SettingsIcon /> },
            { label: 'Profile' },
          ]}
          size="md"
        />
      </div>

      <div>
        <p className="text-sm font-medium text-neutral-700 mb-3">Large (lg)</p>
        <Breadcrumb
          items={[
            { label: 'Home', href: '/', icon: <HomeIcon /> },
            { label: 'Settings', href: '/settings', icon: <SettingsIcon /> },
            { label: 'Profile' },
          ]}
          size="lg"
        />
      </div>
    </div>
  ),
};

/**
 * Custom separators for different visual styles.
 * You can use any React node as a separator: text, icons, or custom components.
 */
export const CustomSeparators: Story = {
  render: () => (
    <div className="space-y-6">
      <div>
        <p className="text-sm font-medium text-neutral-700 mb-3">
          Slash separator (/)
        </p>
        <Breadcrumb
          items={[
            { label: 'Home', href: '/' },
            { label: 'Products', href: '/products' },
            { label: 'Laptops' },
          ]}
          separator={<span>/</span>}
        />
      </div>

      <div>
        <p className="text-sm font-medium text-neutral-700 mb-3">
          Arrow separator (→)
        </p>
        <Breadcrumb
          items={[
            { label: 'Home', href: '/' },
            { label: 'Products', href: '/products' },
            { label: 'Laptops' },
          ]}
          separator={<span>→</span>}
        />
      </div>

      <div>
        <p className="text-sm font-medium text-neutral-700 mb-3">
          Dot separator (•)
        </p>
        <Breadcrumb
          items={[
            { label: 'Home', href: '/' },
            { label: 'Products', href: '/products' },
            { label: 'Laptops' },
          ]}
          separator={<span>•</span>}
        />
      </div>

      <div>
        <p className="text-sm font-medium text-neutral-700 mb-3">
          Pipe separator (|)
        </p>
        <Breadcrumb
          items={[
            { label: 'Home', href: '/' },
            { label: 'Products', href: '/products' },
            { label: 'Laptops' },
          ]}
          separator={<span>|</span>}
        />
      </div>
    </div>
  ),
};

/**
 * Disabled breadcrumb items.
 * Disabled items are styled with reduced opacity and cannot be interacted with.
 */
export const DisabledItems: Story = {
  args: {
    items: [
      { label: 'Home', href: '/' },
      { label: 'Restricted Area', href: '/restricted', disabled: true },
      { label: 'Settings', href: '/settings' },
      { label: 'Profile' },
    ],
    size: 'md',
  },
};

/**
 * Without icons for a cleaner look.
 * Set showIcon to false to hide all icons, even if items have them defined.
 */
export const WithoutIcons: Story = {
  args: {
    items: [
      { label: 'Home', href: '/', icon: <HomeIcon /> },
      { label: 'Projects', href: '/projects', icon: <FolderIcon /> },
      { label: 'Document.pdf', icon: <DocumentIcon /> },
    ],
    size: 'md',
    showIcon: false,
  },
};

/**
 * Real-world examples in common application contexts.
 */
export const RealWorldExamples: Story = {
  render: () => (
    <div className="space-y-8">
      <div>
        <h3 className="text-base font-medium text-neutral-900 mb-3">
          E-commerce Product Page
        </h3>
        <Breadcrumb
          items={[
            { label: 'Home', href: '/', icon: <HomeIcon /> },
            { label: 'Electronics', href: '/electronics' },
            { label: 'Computers', href: '/electronics/computers' },
            { label: 'Laptops', href: '/electronics/computers/laptops' },
            { label: 'MacBook Pro 16"' },
          ]}
        />
      </div>

      <div>
        <h3 className="text-base font-medium text-neutral-900 mb-3">
          Admin Dashboard Settings
        </h3>
        <Breadcrumb
          items={[
            { label: 'Dashboard', href: '/admin', icon: <HomeIcon /> },
            {
              label: 'Settings',
              href: '/admin/settings',
              icon: <SettingsIcon />,
            },
            { label: 'Security', href: '/admin/settings/security' },
            { label: 'Two-Factor Authentication' },
          ]}
        />
      </div>

      <div>
        <h3 className="text-base font-medium text-neutral-900 mb-3">
          Documentation Navigation
        </h3>
        <Breadcrumb
          items={[
            { label: 'Docs', href: '/docs', icon: <DocumentIcon /> },
            { label: 'Components', href: '/docs/components' },
            { label: 'Navigation', href: '/docs/components/navigation' },
            { label: 'Breadcrumb' },
          ]}
          size="sm"
        />
      </div>

      <div>
        <h3 className="text-base font-medium text-neutral-900 mb-3">
          File Manager
        </h3>
        <Breadcrumb
          items={[
            { label: 'My Files', href: '/files', icon: <FolderIcon /> },
            {
              label: 'Documents',
              href: '/files/documents',
              icon: <FolderIcon />,
            },
            {
              label: 'Projects',
              href: '/files/documents/projects',
              icon: <FolderIcon />,
            },
            {
              label: '2024',
              href: '/files/documents/projects/2024',
              icon: <FolderIcon />,
            },
            { label: 'proposal.pdf', icon: <DocumentIcon /> },
          ]}
          maxItems={4}
        />
      </div>
    </div>
  ),
};

/**
 * Interactive playground to experiment with all props.
 * Try different combinations of items, sizes, separators, and truncation.
 */
export const Playground: Story = {
  args: {
    items: [
      { label: 'Home', href: '/', icon: <HomeIcon /> },
      { label: 'Category', href: '/category', icon: <FolderIcon /> },
      { label: 'Subcategory', href: '/category/subcategory' },
      { label: 'Current Page' },
    ],
    size: 'md',
    showIcon: true,
    maxItems: undefined,
  },
  parameters: {
    docs: {
      description: {
        story:
          'Experiment with all breadcrumb props using the controls below. Try different item arrays, sizes, separators, and truncation settings.',
      },
    },
  },
};

/**
 * Accessibility features demonstration.
 * All breadcrumbs support:
 * - Semantic HTML (nav, ol, li elements)
 * - aria-label for navigation landmark
 * - aria-current="page" for current page
 * - aria-disabled for disabled items
 * - Keyboard navigation (Tab to focus links, Enter to activate)
 * - Screen reader friendly structure
 */
export const Accessibility: Story = {
  render: () => (
    <div className="space-y-6">
      <div>
        <p className="text-sm font-medium text-neutral-700 mb-3">
          Semantic HTML Structure
        </p>
        <p className="text-sm text-neutral-600 mb-4">
          Uses proper nav, ol, and li elements for screen reader navigation. The
          last item is marked with aria-current="page".
        </p>
        <Breadcrumb
          items={[
            { label: 'Home', href: '/' },
            { label: 'Products', href: '/products' },
            { label: 'Laptops' },
          ]}
          aria-label="Page navigation"
        />
      </div>

      <div>
        <p className="text-sm font-medium text-neutral-700 mb-3">
          Keyboard Navigation
        </p>
        <p className="text-sm text-neutral-600 mb-4">
          Press Tab to focus links, Enter to navigate. Links have visible focus
          indicators.
        </p>
        <Breadcrumb
          items={[
            { label: 'Dashboard', href: '/dashboard' },
            { label: 'Settings', href: '/settings' },
            { label: 'Profile', href: '/settings/profile' },
            { label: 'Edit Profile' },
          ]}
        />
      </div>

      <div>
        <p className="text-sm font-medium text-neutral-700 mb-3">
          Disabled Items
        </p>
        <p className="text-sm text-neutral-600 mb-4">
          Disabled items are marked with aria-disabled and cannot be interacted
          with.
        </p>
        <Breadcrumb
          items={[
            { label: 'Home', href: '/' },
            { label: 'Restricted', href: '/restricted', disabled: true },
            { label: 'Public', href: '/public' },
            { label: 'Page' },
          ]}
        />
      </div>

      <div>
        <p className="text-sm font-medium text-neutral-700 mb-3">
          Screen Reader Support
        </p>
        <p className="text-sm text-neutral-600 mb-4">
          Separators are hidden from screen readers with aria-hidden. Icons have
          proper aria-hidden attributes.
        </p>
        <Breadcrumb
          items={[
            { label: 'Docs', href: '/docs', icon: <DocumentIcon /> },
            { label: 'Guides', href: '/docs/guides', icon: <FolderIcon /> },
            { label: 'Accessibility Guide', icon: <DocumentIcon /> },
          ]}
          showIcon
        />
      </div>
    </div>
  ),
  parameters: {
    docs: {
      description: {
        story:
          'Breadcrumbs are fully accessible with semantic HTML, ARIA attributes, keyboard support, and screen reader compatibility. All navigation states are properly announced.',
      },
    },
  },
};
