/**
 * Grid Component Stories
 *
 * Demonstrates all Grid variants, column configurations, and usage patterns.
 */

import type { Meta, StoryObj } from '@storybook/react';
import { Grid } from './Grid';

const meta = {
  title: 'Design System/Layout/Grid',
  component: Grid,
  parameters: {
    layout: 'padded',
  },
  tags: ['autodocs'],
  argTypes: {
    columns: {
      control: 'select',
      options: [1, 2, 3, 4, 5, 6],
      description: 'Number of columns (1-6)',
      table: {
        type: { summary: 'number' },
        defaultValue: { summary: '3' },
      },
    },
    gap: {
      control: 'select',
      options: ['sm', 'md', 'lg'],
      description: 'Gap size between items',
      table: {
        type: { summary: 'string' },
        defaultValue: { summary: 'md' },
      },
    },
    children: {
      control: 'text',
      description: 'Grid content',
    },
  },
} satisfies Meta<typeof Grid>;

export default meta;
type Story = StoryObj<typeof meta>;

// Helper component for consistent demo items
const GridItem = ({
  children,
  variant = 'default',
  tall = false,
}: {
  children: React.ReactNode;
  variant?: 'default' | 'highlight' | 'accent';
  tall?: boolean;
}) => {
  const variantClasses = {
    default: 'bg-trust-light text-trust-deep border-neutral-200',
    highlight: 'bg-success-light text-success-dark border-success-light',
    accent: 'bg-sand text-secondary-900 border-slate',
  };

  return (
    <div
      className={`px-4 py-6 rounded-lg border-2 font-medium text-sm flex items-center justify-center text-center ${variantClasses[variant]} ${tall ? 'min-h-[120px]' : 'min-h-[80px]'}`}
    >
      {children}
    </div>
  );
};

// Helper component for card demos
const DemoCard = ({
  title,
  description,
}: {
  title: string;
  description: string;
}) => {
  return (
    <div className="bg-white rounded-lg shadow-card p-6 border-2 border-trust-light hover:shadow-card-hover transition-shadow">
      <h3 className="text-lg font-display font-medium text-trust-deep mb-2">
        {title}
      </h3>
      <p className="text-sm text-secondary-600">{description}</p>
    </div>
  );
};

/**
 * Default 3-column grid with medium gap.
 * This is the most common grid configuration for balanced layouts.
 */
export const Default: Story = {
  args: {
    columns: 3,
    gap: 'md',
    children: (
      <>
        <GridItem>Item 1</GridItem>
        <GridItem>Item 2</GridItem>
        <GridItem>Item 3</GridItem>
        <GridItem>Item 4</GridItem>
        <GridItem>Item 5</GridItem>
        <GridItem>Item 6</GridItem>
      </>
    ),
  },
};

/**
 * All column configurations for visual comparison.
 * Choose column count based on content density and viewport:
 * - 1 column: Mobile, full-width content
 * - 2 columns: Split layouts, tablet
 * - 3 columns: Balanced layouts, desktop (default)
 * - 4 columns: Dense grids, wide viewports
 * - 5 columns: Specialized layouts
 * - 6 columns: Maximum density, extra-wide viewports
 */
export const Columns: Story = {
  render: () => (
    <div className="space-y-8 bg-cream p-6 rounded-xl">
      <div>
        <div className="mb-3 text-sm font-medium text-secondary-600">
          1 Column - Full Width
        </div>
        <Grid columns={1}>
          <GridItem>Item 1</GridItem>
          <GridItem>Item 2</GridItem>
          <GridItem>Item 3</GridItem>
        </Grid>
      </div>

      <div>
        <div className="mb-3 text-sm font-medium text-secondary-600">
          2 Columns - Split Layout
        </div>
        <Grid columns={2}>
          <GridItem>Item 1</GridItem>
          <GridItem>Item 2</GridItem>
          <GridItem>Item 3</GridItem>
          <GridItem>Item 4</GridItem>
        </Grid>
      </div>

      <div>
        <div className="mb-3 text-sm font-medium text-secondary-600">
          3 Columns - Balanced (Default)
        </div>
        <Grid columns={3}>
          <GridItem>Item 1</GridItem>
          <GridItem>Item 2</GridItem>
          <GridItem>Item 3</GridItem>
          <GridItem>Item 4</GridItem>
          <GridItem>Item 5</GridItem>
          <GridItem>Item 6</GridItem>
        </Grid>
      </div>

      <div>
        <div className="mb-3 text-sm font-medium text-secondary-600">
          4 Columns - Dense Grid
        </div>
        <Grid columns={4}>
          <GridItem>Item 1</GridItem>
          <GridItem>Item 2</GridItem>
          <GridItem>Item 3</GridItem>
          <GridItem>Item 4</GridItem>
          <GridItem>Item 5</GridItem>
          <GridItem>Item 6</GridItem>
          <GridItem>Item 7</GridItem>
          <GridItem>Item 8</GridItem>
        </Grid>
      </div>

      <div>
        <div className="mb-3 text-sm font-medium text-secondary-600">
          6 Columns - Maximum Density
        </div>
        <Grid columns={6}>
          <GridItem>1</GridItem>
          <GridItem>2</GridItem>
          <GridItem>3</GridItem>
          <GridItem>4</GridItem>
          <GridItem>5</GridItem>
          <GridItem>6</GridItem>
          <GridItem>7</GridItem>
          <GridItem>8</GridItem>
          <GridItem>9</GridItem>
          <GridItem>10</GridItem>
          <GridItem>11</GridItem>
          <GridItem>12</GridItem>
        </Grid>
      </div>
    </div>
  ),
};

/**
 * All gap sizes for visual comparison.
 * Choose gap sizes based on visual hierarchy and spacing needs:
 * - sm (12px): Tight grouping, dense layouts
 * - md (16px): Standard spacing (default)
 * - lg (24px): Generous spacing, distinct items
 */
export const Gaps: Story = {
  render: () => (
    <div className="space-y-8 bg-cream p-6 rounded-xl">
      <div>
        <div className="mb-3 text-sm font-medium text-secondary-600">
          Small Gap (sm - 12px)
        </div>
        <Grid columns={3} gap="sm">
          <GridItem>Item 1</GridItem>
          <GridItem>Item 2</GridItem>
          <GridItem>Item 3</GridItem>
          <GridItem>Item 4</GridItem>
          <GridItem>Item 5</GridItem>
          <GridItem>Item 6</GridItem>
        </Grid>
      </div>

      <div>
        <div className="mb-3 text-sm font-medium text-secondary-600">
          Medium Gap (md - 16px) - Default
        </div>
        <Grid columns={3} gap="md">
          <GridItem>Item 1</GridItem>
          <GridItem>Item 2</GridItem>
          <GridItem>Item 3</GridItem>
          <GridItem>Item 4</GridItem>
          <GridItem>Item 5</GridItem>
          <GridItem>Item 6</GridItem>
        </Grid>
      </div>

      <div>
        <div className="mb-3 text-sm font-medium text-secondary-600">
          Large Gap (lg - 24px)
        </div>
        <Grid columns={3} gap="lg">
          <GridItem>Item 1</GridItem>
          <GridItem>Item 2</GridItem>
          <GridItem>Item 3</GridItem>
          <GridItem>Item 4</GridItem>
          <GridItem>Item 5</GridItem>
          <GridItem>Item 6</GridItem>
        </Grid>
      </div>
    </div>
  ),
};

/**
 * Responsive grid with column changes at breakpoints.
 * Grid adapts from 1 column on mobile to 2 on tablet to 4 on desktop.
 * This pattern is essential for mobile-first responsive design.
 */
export const Responsive: Story = {
  render: () => (
    <div className="space-y-8 bg-cream p-6 rounded-xl">
      <div className="bg-white rounded-lg p-6 shadow-md-premium">
        <h2 className="text-xl font-display font-medium text-trust-deep mb-4">
          Responsive Grid Behavior
        </h2>
        <p className="text-secondary-600 mb-4">
          Try resizing your browser window to see how the grid adapts:
        </p>
        <ul className="list-disc list-inside text-secondary-600 space-y-2 mb-6">
          <li>Mobile (default): 1 column</li>
          <li>Tablet (sm: 640px+): 2 columns</li>
          <li>Desktop (lg: 1024px+): 4 columns</li>
        </ul>

        <Grid columns={1} gap="md" className="sm:grid-cols-2 lg:grid-cols-4">
          <GridItem variant="highlight">Responsive 1</GridItem>
          <GridItem variant="highlight">Responsive 2</GridItem>
          <GridItem variant="highlight">Responsive 3</GridItem>
          <GridItem variant="highlight">Responsive 4</GridItem>
          <GridItem variant="highlight">Responsive 5</GridItem>
          <GridItem variant="highlight">Responsive 6</GridItem>
          <GridItem variant="highlight">Responsive 7</GridItem>
          <GridItem variant="highlight">Responsive 8</GridItem>
        </Grid>
      </div>

      <div className="bg-white rounded-lg p-6 shadow-md-premium">
        <h3 className="text-lg font-display font-medium text-trust-deep mb-3">
          Mobile-First Card Grid
        </h3>
        <p className="text-secondary-600 mb-4">
          1 column on mobile, 2 on tablet, 3 on desktop:
        </p>

        <Grid columns={1} gap="lg" className="sm:grid-cols-2 md:grid-cols-3">
          <GridItem variant="accent" tall>
            Card 1<br />
            Mobile First
          </GridItem>
          <GridItem variant="accent" tall>
            Card 2<br />
            Mobile First
          </GridItem>
          <GridItem variant="accent" tall>
            Card 3<br />
            Mobile First
          </GridItem>
        </Grid>
      </div>
    </div>
  ),
};

/**
 * Auto-fit columns with minimum and maximum widths.
 * Columns automatically adjust based on available space and content.
 * Useful for responsive card grids without breakpoints.
 */
export const AutoFit: Story = {
  render: () => (
    <div className="space-y-8 bg-cream p-6 rounded-xl">
      <div className="bg-white rounded-lg p-6 shadow-md-premium">
        <h2 className="text-xl font-display font-medium text-trust-deep mb-4">
          Auto-Fit Grid
        </h2>
        <p className="text-secondary-600 mb-4">
          Columns automatically fit based on container width. Minimum 200px,
          maximum 1fr.
        </p>
        <p className="text-sm text-secondary-500 mb-6">
          Try resizing to see columns adjust without media queries.
        </p>

        <Grid
          gap="md"
          className="grid-cols-[repeat(auto-fit,minmax(200px,1fr))]"
        >
          <GridItem variant="highlight">Auto 1</GridItem>
          <GridItem variant="highlight">Auto 2</GridItem>
          <GridItem variant="highlight">Auto 3</GridItem>
          <GridItem variant="highlight">Auto 4</GridItem>
          <GridItem variant="highlight">Auto 5</GridItem>
          <GridItem variant="highlight">Auto 6</GridItem>
        </Grid>
      </div>

      <div className="bg-white rounded-lg p-6 shadow-md-premium">
        <h3 className="text-lg font-display font-medium text-trust-deep mb-3">
          Auto-Fill Grid (Larger Minimum)
        </h3>
        <p className="text-secondary-600 mb-4">
          Minimum 250px per column - fewer columns, more breathing room.
        </p>

        <Grid
          gap="lg"
          className="grid-cols-[repeat(auto-fill,minmax(250px,1fr))]"
        >
          <GridItem variant="accent">Auto-Fill 1</GridItem>
          <GridItem variant="accent">Auto-Fill 2</GridItem>
          <GridItem variant="accent">Auto-Fill 3</GridItem>
          <GridItem variant="accent">Auto-Fill 4</GridItem>
        </Grid>
      </div>
    </div>
  ),
};

/**
 * Real-world card grid implementation.
 * Demonstrates how to use Grid with actual card components
 * for features, products, or content listings.
 */
export const CardGrid: Story = {
  render: () => (
    <div className="space-y-8 bg-cream p-6 rounded-xl">
      <div>
        <h2 className="text-2xl font-display font-medium text-trust-deep mb-2">
          Features Overview
        </h2>
        <p className="text-secondary-600 mb-6">
          Responsive card grid showcasing product features
        </p>

        <Grid columns={1} gap="lg" className="sm:grid-cols-2 lg:grid-cols-3">
          <DemoCard
            title="Secure Authentication"
            description="Industry-standard OAuth 2.0 and OpenID Connect protocols with enterprise-grade security."
          />
          <DemoCard
            title="Fine-Grained Consent"
            description="Granular permission management allowing users to control exactly what data they share."
          />
          <DemoCard
            title="Multi-Provider Support"
            description="Connect with multiple identity providers seamlessly with unified consent flows."
          />
          <DemoCard
            title="Real-Time Monitoring"
            description="Track authentication events and consent decisions in real-time with detailed analytics."
          />
          <DemoCard
            title="Custom Branding"
            description="White-label consent screens to match your brand identity and user experience."
          />
          <DemoCard
            title="Compliance Ready"
            description="Built-in GDPR, CCPA, and privacy regulation compliance with audit trails."
          />
        </Grid>
      </div>

      <div>
        <h2 className="text-2xl font-display font-medium text-trust-deep mb-2">
          Dense Layout (4 Columns)
        </h2>
        <p className="text-secondary-600 mb-6">
          Higher density for desktop viewing
        </p>

        <Grid columns={4} gap="sm">
          <DemoCard
            title="Feature 1"
            description="Quick overview of capability"
          />
          <DemoCard
            title="Feature 2"
            description="Quick overview of capability"
          />
          <DemoCard
            title="Feature 3"
            description="Quick overview of capability"
          />
          <DemoCard
            title="Feature 4"
            description="Quick overview of capability"
          />
          <DemoCard
            title="Feature 5"
            description="Quick overview of capability"
          />
          <DemoCard
            title="Feature 6"
            description="Quick overview of capability"
          />
          <DemoCard
            title="Feature 7"
            description="Quick overview of capability"
          />
          <DemoCard
            title="Feature 8"
            description="Quick overview of capability"
          />
        </Grid>
      </div>
    </div>
  ),
};

/**
 * Interactive playground to experiment with all props.
 * Try different combinations of columns and gap sizes.
 */
export const Playground: Story = {
  args: {
    columns: 3,
    gap: 'md',
    children: (
      <>
        <GridItem>Item 1</GridItem>
        <GridItem>Item 2</GridItem>
        <GridItem>Item 3</GridItem>
        <GridItem>Item 4</GridItem>
        <GridItem>Item 5</GridItem>
        <GridItem>Item 6</GridItem>
        <GridItem>Item 7</GridItem>
        <GridItem>Item 8</GridItem>
        <GridItem>Item 9</GridItem>
      </>
    ),
  },
  parameters: {
    docs: {
      description: {
        story:
          'Experiment with all grid props using the controls below. Try different column counts and gap sizes to see how they affect the layout.',
      },
    },
  },
};

/**
 * Accessibility features demonstration.
 * Grids use semantic HTML and support:
 * - Proper document structure with logical reading order
 * - Keyboard navigation for interactive children
 * - Sufficient spacing for touch targets
 * - Responsive behavior for all screen sizes
 * - Screen reader friendly markup
 */
export const Accessibility: Story = {
  render: () => (
    <div className="bg-cream p-8">
      <div className="bg-white rounded-lg shadow-md-premium p-6">
        <h2 className="text-xl font-display font-medium text-trust-deep mb-3">
          Accessibility Features
        </h2>
        <p className="text-secondary-600 mb-6">
          Grids are designed with accessibility in mind:
        </p>

        <Grid columns={1} gap="md" className="sm:grid-cols-2">
          <div className="flex items-start gap-3">
            <div className="flex-shrink-0 w-6 h-6 rounded-full bg-success-light text-success-hover flex items-center justify-center text-sm font-medium">
              ✓
            </div>
            <div>
              <h3 className="font-medium text-trust-deep">Semantic HTML</h3>
              <p className="text-sm text-secondary-600">
                Uses standard div elements with CSS Grid, no ARIA required
              </p>
            </div>
          </div>

          <div className="flex items-start gap-3">
            <div className="flex-shrink-0 w-6 h-6 rounded-full bg-success-light text-success-hover flex items-center justify-center text-sm font-medium">
              ✓
            </div>
            <div>
              <h3 className="font-medium text-trust-deep">
                Logical Reading Order
              </h3>
              <p className="text-sm text-secondary-600">
                Content flows naturally left-to-right, top-to-bottom for screen
                readers
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
              <h3 className="font-medium text-trust-deep">Fully Responsive</h3>
              <p className="text-sm text-secondary-600">
                Adapts gracefully across all viewport sizes and zoom levels
              </p>
            </div>
          </div>

          <div className="flex items-start gap-3">
            <div className="flex-shrink-0 w-6 h-6 rounded-full bg-success-light text-success-hover flex items-center justify-center text-sm font-medium">
              ✓
            </div>
            <div>
              <h3 className="font-medium text-trust-deep">
                Keyboard Navigation
              </h3>
              <p className="text-sm text-secondary-600">
                Interactive grid children fully support keyboard navigation
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
        </Grid>
      </div>
    </div>
  ),
  parameters: {
    docs: {
      description: {
        story:
          'Grids follow accessibility best practices with semantic HTML, logical reading order, and consideration for all users regardless of device or ability.',
      },
    },
  },
};
