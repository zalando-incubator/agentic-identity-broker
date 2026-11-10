/**
 * Container Component Stories
 *
 * Demonstrates all Container variants, sizes, and usage patterns.
 */

import type { Meta, StoryObj } from '@storybook/react';
import { Container } from './Container';

const meta = {
  title: 'Design System/Layout/Container',
  component: Container,
  parameters: {
    layout: 'fullscreen',
  },
  tags: ['autodocs'],
  argTypes: {
    size: {
      control: 'select',
      options: ['sm', 'md', 'lg', 'xl', 'full'],
      description: 'Maximum width constraint',
      table: {
        type: { summary: 'string' },
        defaultValue: { summary: 'lg' },
      },
    },
    centered: {
      control: 'boolean',
      description: 'Center content horizontally',
      table: {
        type: { summary: 'boolean' },
        defaultValue: { summary: 'true' },
      },
    },
    padding: {
      control: 'select',
      options: [false, true, 'sm', 'md', 'lg'],
      description: 'Internal padding',
      table: {
        type: { summary: 'boolean | "sm" | "md" | "lg"' },
        defaultValue: { summary: 'false' },
      },
    },
    children: {
      control: 'text',
      description: 'Container content',
    },
  },
} satisfies Meta<typeof Container>;

export default meta;
type Story = StoryObj<typeof meta>;

// Helper component for visualizing container boundaries
const ContainerDemo = ({
  children,
  label,
}: {
  children: React.ReactNode;
  label?: string;
}) => (
  <div className="bg-cream min-h-[200px] py-8">
    {label && (
      <div className="text-center mb-4">
        <span className="inline-block px-3 py-1 text-xs font-medium bg-trust-light text-trust-deep rounded-full">
          {label}
        </span>
      </div>
    )}
    <div className="relative">
      {/* Visual grid for reference */}
      <div className="absolute inset-0 bg-grid-pattern opacity-5 pointer-events-none" />
      {children}
    </div>
  </div>
);

const SampleContent = () => (
  <div className="bg-white rounded-lg shadow-md-premium p-6 border border-slate">
    <h2 className="text-2xl font-display font-medium text-trust-deep mb-4">
      Sample Content
    </h2>
    <p className="text-base text-secondary-600 leading-relaxed mb-4">
      This is sample content to demonstrate the container behavior. The
      container constrains the maximum width and can optionally center content
      horizontally.
    </p>
    <p className="text-base text-secondary-600 leading-relaxed">
      Containers are essential for creating readable layouts and maintaining
      consistent spacing across different screen sizes.
    </p>
  </div>
);

/**
 * Default container with large max-width (1024px) and centered content.
 * This is the most common container configuration for page content.
 */
export const Default: Story = {
  args: {
    size: 'lg',
    centered: true,
    padding: false,
    children: <SampleContent />,
  },
  render: (args) => (
    <ContainerDemo label="Default Container (lg, centered)">
      <Container {...args} />
    </ContainerDemo>
  ),
};

/**
 * All container sizes side-by-side for visual comparison.
 * Choose sizes based on content type:
 * - sm (640px): Compact content, forms, sidebars
 * - md (768px): Articles, blog posts, narrow content
 * - lg (1024px): Standard page content (default)
 * - xl (1280px): Wide dashboards, marketing pages
 * - full: No constraint, spans full viewport
 */
export const Sizes: Story = {
  render: () => (
    <div className="space-y-8 bg-cream py-8">
      <ContainerDemo label="Small (640px)">
        <Container size="sm">
          <SampleContent />
        </Container>
      </ContainerDemo>

      <ContainerDemo label="Medium (768px)">
        <Container size="md">
          <SampleContent />
        </Container>
      </ContainerDemo>

      <ContainerDemo label="Large (1024px) - Default">
        <Container size="lg">
          <SampleContent />
        </Container>
      </ContainerDemo>

      <ContainerDemo label="Extra Large (1280px)">
        <Container size="xl">
          <SampleContent />
        </Container>
      </ContainerDemo>

      <ContainerDemo label="Full Width (100%)">
        <Container size="full">
          <SampleContent />
        </Container>
      </ContainerDemo>
    </div>
  ),
};

/**
 * Comparison of centered vs left-aligned containers.
 * Centered is default and recommended for most content.
 * Left-aligned is useful for navigation bars and full-width sections.
 */
export const Centered: Story = {
  render: () => (
    <div className="space-y-8">
      <ContainerDemo label="Centered (default)">
        <Container size="md" centered={true}>
          <SampleContent />
        </Container>
      </ContainerDemo>

      <ContainerDemo label="Left-aligned">
        <Container size="md" centered={false}>
          <SampleContent />
        </Container>
      </ContainerDemo>
    </div>
  ),
};

/**
 * Container padding variants.
 * Padding adds internal spacing and is useful for:
 * - Mobile responsiveness (prevents content from touching edges)
 * - Creating visual separation
 * - Section backgrounds
 */
export const Padding: Story = {
  render: () => (
    <div className="space-y-8">
      <ContainerDemo label="No Padding (default)">
        <Container size="md">
          <SampleContent />
        </Container>
      </ContainerDemo>

      <ContainerDemo label="Small Padding (px-3 py-2)">
        <Container size="md" padding="sm">
          <div className="bg-success-light rounded-lg border-2 border-dashed border-success-light">
            <SampleContent />
          </div>
        </Container>
      </ContainerDemo>

      <ContainerDemo label="Medium Padding (px-4 py-4)">
        <Container size="md" padding="md">
          <div className="bg-amber-50 rounded-lg border-2 border-dashed border-amber-300">
            <SampleContent />
          </div>
        </Container>
      </ContainerDemo>

      <ContainerDemo label="Large Padding (px-6 py-6)">
        <Container size="md" padding="lg">
          <div className="bg-trust-light rounded-lg border-2 border-dashed border-neutral-300">
            <SampleContent />
          </div>
        </Container>
      </ContainerDemo>
    </div>
  ),
};

/**
 * Nested containers for complex layouts.
 * Outer container provides overall page width constraint,
 * inner containers can further constrain specific sections.
 */
export const Nested: Story = {
  render: () => (
    <ContainerDemo label="Outer Container (xl) with Inner Container (md)">
      <Container size="xl" padding="lg" className="bg-sand rounded-xl">
        <div className="mb-6">
          <h1 className="text-3xl font-display font-medium text-trust-deep mb-2">
            Page with Nested Container
          </h1>
          <p className="text-secondary-600">
            This outer container spans 1280px max width
          </p>
        </div>

        <Container
          size="md"
          padding="md"
          className="bg-white rounded-lg shadow-md-premium border border-slate"
        >
          <h2 className="text-xl font-display font-medium text-trust-deep mb-3">
            Nested Inner Container
          </h2>
          <p className="text-secondary-600 mb-3">
            This inner container has a medium width constraint (768px), creating
            a narrower reading area within the wider page.
          </p>
          <p className="text-secondary-600">This pattern is useful for:</p>
          <ul className="list-disc list-inside text-secondary-600 mt-2 space-y-1">
            <li>Long-form articles within wide layouts</li>
            <li>Forms within dashboard pages</li>
            <li>Focused content within marketing pages</li>
          </ul>
        </Container>
      </Container>
    </ContainerDemo>
  ),
};

/**
 * Responsive behavior demonstration.
 * Containers automatically adapt to smaller screens while maintaining padding
 * and centering. On mobile, content gracefully fills available space.
 */
export const Responsive: Story = {
  render: () => (
    <div className="space-y-8 bg-cream p-4">
      <div className="bg-white rounded-lg p-6 shadow-md-premium">
        <h2 className="text-xl font-display font-medium text-trust-deep mb-4">
          Responsive Behavior
        </h2>
        <p className="text-secondary-600 mb-4">
          Try resizing your browser window to see how containers adapt:
        </p>
        <ul className="list-disc list-inside text-secondary-600 space-y-2">
          <li>On wide screens, container constrains to max-width</li>
          <li>On narrow screens, container fills available space</li>
          <li>Padding helps prevent edge-to-edge content on mobile</li>
          <li>Content remains readable across all viewport sizes</li>
        </ul>
      </div>

      <Container size="lg" padding="md" className="bg-trust-light rounded-lg">
        <div className="bg-white rounded-lg p-6 shadow-sm">
          <h3 className="text-lg font-medium text-trust-deep mb-2">
            Large Container with Padding
          </h3>
          <p className="text-secondary-600">
            Max-width: 1024px on desktop, 100% on mobile with padding
          </p>
        </div>
      </Container>

      <Container size="md" padding="sm" className="bg-success-light rounded-lg">
        <div className="bg-white rounded-lg p-6 shadow-sm">
          <h3 className="text-lg font-medium text-trust-deep mb-2">
            Medium Container with Small Padding
          </h3>
          <p className="text-secondary-600">
            Max-width: 768px on desktop, 100% on mobile with padding
          </p>
        </div>
      </Container>
    </div>
  ),
};

/**
 * Real-world usage examples showing common patterns.
 * These demonstrate how containers are typically used in applications.
 */
export const RealWorldExamples: Story = {
  render: () => (
    <div className="space-y-0">
      {/* Header with full-width background, constrained content */}
      <div className="bg-trust-deep text-white py-4">
        <Container size="xl" padding="md">
          <div className="flex items-center justify-between">
            <h1 className="text-xl font-display font-medium">
              Application Name
            </h1>
            <nav className="flex gap-4 text-sm">
              <a href="#" className="hover:text-neutral-200">
                Home
              </a>
              <a href="#" className="hover:text-neutral-200">
                About
              </a>
              <a href="#" className="hover:text-neutral-200">
                Contact
              </a>
            </nav>
          </div>
        </Container>
      </div>

      {/* Hero section with large container */}
      <div className="bg-gradient-to-br from-trust-light to-success-light py-16">
        <Container size="lg">
          <div className="text-center">
            <h2 className="text-4xl font-display font-medium text-trust-deep mb-4">
              Welcome to Our Service
            </h2>
            <p className="text-lg text-secondary-600 max-w-2xl mx-auto">
              This hero section uses a large container to constrain the width
              while allowing the background to span full width.
            </p>
          </div>
        </Container>
      </div>

      {/* Content section with medium container */}
      <div className="py-12 bg-white">
        <Container size="md" padding="md">
          <article className="prose prose-lg max-w-none">
            <h2 className="text-2xl font-display font-medium text-trust-deep mb-4">
              Article Content
            </h2>
            <p className="text-secondary-600 leading-relaxed mb-4">
              Article content uses a medium container for optimal reading width.
              Research shows that line lengths of 60-75 characters improve
              readability.
            </p>
            <p className="text-secondary-600 leading-relaxed">
              The medium container (768px) naturally creates comfortable line
              lengths for reading, reducing eye strain and improving
              comprehension.
            </p>
          </article>
        </Container>
      </div>

      {/* Footer with full-width background */}
      <div className="bg-secondary-800 text-secondary-100 py-8">
        <Container size="xl" padding="md">
          <div className="text-center text-sm">
            <p>&copy; 2024 Your Company. All rights reserved.</p>
          </div>
        </Container>
      </div>
    </div>
  ),
};

/**
 * Interactive playground to experiment with all props.
 * Try different combinations of size, centered, and padding.
 */
export const Playground: Story = {
  args: {
    size: 'lg',
    centered: true,
    padding: false,
    children: <SampleContent />,
  },
  render: (args) => (
    <ContainerDemo label="Interactive Playground">
      <Container {...args} />
    </ContainerDemo>
  ),
  parameters: {
    docs: {
      description: {
        story:
          'Experiment with all container props using the controls below. Try different sizes, toggle centering, and adjust padding.',
      },
    },
  },
};

/**
 * Accessibility features demonstration.
 * Containers use semantic HTML and support:
 * - Proper document structure
 * - Responsive design for all devices
 * - Sufficient spacing for touch targets
 * - No reliance on specific viewport sizes
 */
export const Accessibility: Story = {
  render: () => (
    <div className="bg-cream p-8">
      <Container
        size="md"
        padding="md"
        className="bg-white rounded-lg shadow-md-premium"
      >
        <div className="space-y-6">
          <div>
            <h2 className="text-xl font-display font-medium text-trust-deep mb-3">
              Accessibility Features
            </h2>
            <p className="text-secondary-600">
              Containers are designed with accessibility in mind:
            </p>
          </div>

          <div className="space-y-4">
            <div className="flex items-start gap-3">
              <div className="flex-shrink-0 w-6 h-6 rounded-full bg-success-light text-success-hover flex items-center justify-center text-sm font-medium">
                ✓
              </div>
              <div>
                <h3 className="font-medium text-trust-deep">Semantic HTML</h3>
                <p className="text-sm text-secondary-600">
                  Uses standard div elements with no ARIA required
                </p>
              </div>
            </div>

            <div className="flex items-start gap-3">
              <div className="flex-shrink-0 w-6 h-6 rounded-full bg-success-light text-success-hover flex items-center justify-center text-sm font-medium">
                ✓
              </div>
              <div>
                <h3 className="font-medium text-trust-deep">
                  Responsive Design
                </h3>
                <p className="text-sm text-secondary-600">
                  Adapts to all screen sizes, from mobile to ultra-wide
                </p>
              </div>
            </div>

            <div className="flex items-start gap-3">
              <div className="flex-shrink-0 w-6 h-6 rounded-full bg-success-light text-success-hover flex items-center justify-center text-sm font-medium">
                ✓
              </div>
              <div>
                <h3 className="font-medium text-trust-deep">Touch-Friendly</h3>
                <p className="text-sm text-secondary-600">
                  Padding options ensure content doesn't touch viewport edges
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
          </div>
        </div>
      </Container>
    </div>
  ),
  parameters: {
    docs: {
      description: {
        story:
          'Containers follow accessibility best practices with semantic HTML, responsive design, and consideration for all users regardless of device or ability.',
      },
    },
  },
};
