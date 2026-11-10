/**
 * Pagination Component Stories
 *
 * Demonstrates all Pagination variants, sizes, states, and usage patterns.
 */

import type { Meta, StoryObj } from '@storybook/react';
import { useState } from 'react';
import { Pagination } from './Pagination';

const meta = {
  title: 'Design System/Navigation/Pagination',
  component: Pagination,
  parameters: {
    layout: 'centered',
  },
  tags: ['autodocs'],
  argTypes: {
    currentPage: {
      control: { type: 'number', min: 1 },
      description: 'Current active page (1-indexed)',
      table: {
        type: { summary: 'number' },
      },
    },
    totalPages: {
      control: { type: 'number', min: 1 },
      description: 'Total number of pages',
      table: {
        type: { summary: 'number' },
      },
    },
    size: {
      control: 'select',
      options: ['sm', 'md', 'lg'],
      description: 'Size variant',
      table: {
        type: { summary: 'string' },
        defaultValue: { summary: 'md' },
      },
    },
    variant: {
      control: 'select',
      options: ['simple', 'full'],
      description: 'Display mode',
      table: {
        type: { summary: 'string' },
        defaultValue: { summary: 'full' },
      },
    },
    maxVisible: {
      control: { type: 'number', min: 3, max: 15 },
      description: 'Maximum visible page numbers (full mode)',
      table: {
        type: { summary: 'number' },
        defaultValue: { summary: 7 },
      },
    },
    showInfo: {
      control: 'boolean',
      description: 'Show page info text',
      table: {
        defaultValue: { summary: false },
      },
    },
    disabled: {
      control: 'boolean',
      description: 'Disable all interactions',
      table: {
        defaultValue: { summary: false },
      },
    },
  },
} satisfies Meta<typeof Pagination>;

export default meta;
type Story = StoryObj<typeof meta>;

/**
 * Default pagination with numbered pages.
 * Shows standard pagination with page numbers and prev/next buttons.
 */
export const Default: Story = {
  args: {
    currentPage: 5,
    totalPages: 10,
    onPageChange: (page) => console.log('Go to page:', page),
    size: 'md',
  },
};

/**
 * Pagination with page info display.
 * Shows "Page X of Y" below the pagination controls.
 */
export const WithInfo: Story = {
  args: {
    currentPage: 2,
    totalPages: 10,
    showInfo: true,
    onPageChange: (page) => console.log('Go to page:', page),
  },
};

/**
 * All pagination sizes side-by-side.
 * - sm: Compact tables, mobile layouts
 * - md: Standard desktop layouts
 * - lg: Prominent navigation, hero sections
 */
export const Sizes: Story = {
  render: () => (
    <div className="space-y-8">
      <div>
        <p className="text-sm font-medium text-neutral-700 mb-3">Small</p>
        <Pagination
          size="sm"
          currentPage={3}
          totalPages={10}
          onPageChange={(page) => console.log('Small - page:', page)}
        />
      </div>

      <div>
        <p className="text-sm font-medium text-neutral-700 mb-3">Medium</p>
        <Pagination
          size="md"
          currentPage={3}
          totalPages={10}
          onPageChange={(page) => console.log('Medium - page:', page)}
        />
      </div>

      <div>
        <p className="text-sm font-medium text-neutral-700 mb-3">Large</p>
        <Pagination
          size="lg"
          currentPage={3}
          totalPages={10}
          onPageChange={(page) => console.log('Large - page:', page)}
        />
      </div>
    </div>
  ),
};

/**
 * Simple mode with only prev/next buttons.
 * Useful when you want minimal UI or don't need direct page access.
 */
export const SimpleMode: Story = {
  args: {
    variant: 'simple',
    currentPage: 5,
    totalPages: 20,
    showInfo: true,
    onPageChange: (page) => console.log('Go to page:', page),
  },
};

/**
 * Pagination with ellipsis for large datasets.
 * Smart truncation shows relevant pages with ellipsis.
 */
export const Ellipsis: Story = {
  render: () => {
    const [currentPage, setCurrentPage] = useState(15);
    const totalPages = 50;

    return (
      <div className="space-y-8">
        <div>
          <p className="text-sm font-medium text-neutral-700 mb-3">
            Current Page: {currentPage} / {totalPages}
          </p>
          <Pagination
            currentPage={currentPage}
            totalPages={totalPages}
            onPageChange={setCurrentPage}
            showInfo
          />
        </div>

        <div className="text-sm text-neutral-600 space-y-2">
          <p className="font-medium">
            Try these pages to see ellipsis behavior:
          </p>
          <div className="flex gap-2">
            <button
              onClick={() => setCurrentPage(1)}
              className="px-3 py-1 bg-neutral-100 hover:bg-neutral-200 rounded text-sm"
            >
              First (1)
            </button>
            <button
              onClick={() => setCurrentPage(5)}
              className="px-3 py-1 bg-neutral-100 hover:bg-neutral-200 rounded text-sm"
            >
              Early (5)
            </button>
            <button
              onClick={() => setCurrentPage(25)}
              className="px-3 py-1 bg-neutral-100 hover:bg-neutral-200 rounded text-sm"
            >
              Middle (25)
            </button>
            <button
              onClick={() => setCurrentPage(45)}
              className="px-3 py-1 bg-neutral-100 hover:bg-neutral-200 rounded text-sm"
            >
              Late (45)
            </button>
            <button
              onClick={() => setCurrentPage(50)}
              className="px-3 py-1 bg-neutral-100 hover:bg-neutral-200 rounded text-sm"
            >
              Last (50)
            </button>
          </div>
        </div>
      </div>
    );
  },
};

/**
 * Edge cases: first page, last page, single page, and empty.
 * Shows how pagination handles boundary conditions.
 */
export const EdgeCases: Story = {
  render: () => (
    <div className="space-y-8">
      <div>
        <p className="text-sm font-medium text-neutral-700 mb-3">
          First Page (Previous disabled)
        </p>
        <Pagination
          currentPage={1}
          totalPages={10}
          onPageChange={(page) => console.log('First page - page:', page)}
          showInfo
        />
      </div>

      <div>
        <p className="text-sm font-medium text-neutral-700 mb-3">
          Last Page (Next disabled)
        </p>
        <Pagination
          currentPage={10}
          totalPages={10}
          onPageChange={(page) => console.log('Last page - page:', page)}
          showInfo
        />
      </div>

      <div>
        <p className="text-sm font-medium text-neutral-700 mb-3">
          Single Page (Both disabled)
        </p>
        <Pagination
          currentPage={1}
          totalPages={1}
          onPageChange={(page) => console.log('Single page - page:', page)}
          showInfo
        />
      </div>

      <div>
        <p className="text-sm font-medium text-neutral-700 mb-3">
          Few Pages (No ellipsis needed)
        </p>
        <Pagination
          currentPage={2}
          totalPages={5}
          onPageChange={(page) => console.log('Few pages - page:', page)}
          showInfo
        />
      </div>
    </div>
  ),
};

/**
 * Disabled state prevents all interactions.
 * Useful during data loading or when pagination should be temporarily locked.
 */
export const Disabled: Story = {
  args: {
    currentPage: 5,
    totalPages: 10,
    disabled: true,
    onPageChange: (page) => console.log('Go to page:', page),
  },
};

/**
 * Different maxVisible values control ellipsis behavior.
 * Lower values show fewer page numbers, higher values show more.
 */
export const MaxVisibleVariants: Story = {
  render: () => (
    <div className="space-y-8">
      <div>
        <p className="text-sm font-medium text-neutral-700 mb-3">
          maxVisible: 5 (Compact)
        </p>
        <Pagination
          currentPage={10}
          totalPages={20}
          maxVisible={5}
          onPageChange={(page) => console.log('maxVisible 5 - page:', page)}
        />
      </div>

      <div>
        <p className="text-sm font-medium text-neutral-700 mb-3">
          maxVisible: 7 (Default)
        </p>
        <Pagination
          currentPage={10}
          totalPages={20}
          maxVisible={7}
          onPageChange={(page) => console.log('maxVisible 7 - page:', page)}
        />
      </div>

      <div>
        <p className="text-sm font-medium text-neutral-700 mb-3">
          maxVisible: 11 (Extended)
        </p>
        <Pagination
          currentPage={10}
          totalPages={20}
          maxVisible={11}
          onPageChange={(page) => console.log('maxVisible 11 - page:', page)}
        />
      </div>
    </div>
  ),
};

/**
 * Interactive example with state management.
 * Shows full pagination behavior with stateful page changes.
 */
export const Interactive: Story = {
  render: () => {
    const [currentPage, setCurrentPage] = useState(1);
    const totalPages = 15;

    return (
      <div className="space-y-6">
        <div className="p-6 bg-neutral-50 rounded-lg">
          <h3 className="text-lg font-medium mb-2">
            Current Page: {currentPage}
          </h3>
          <p className="text-sm text-neutral-600">
            Click on page numbers or use prev/next buttons to navigate.
          </p>
        </div>

        <Pagination
          currentPage={currentPage}
          totalPages={totalPages}
          onPageChange={setCurrentPage}
          showInfo
        />

        <div className="flex gap-2">
          <button
            onClick={() => setCurrentPage(1)}
            className="px-4 py-2 bg-trust-deep text-white rounded-md hover:bg-trust-hover transition-colors text-sm"
          >
            Reset to First Page
          </button>
          <button
            onClick={() => setCurrentPage(Math.ceil(totalPages / 2))}
            className="px-4 py-2 bg-neutral-600 text-white rounded-md hover:bg-neutral-700 transition-colors text-sm"
          >
            Jump to Middle
          </button>
        </div>
      </div>
    );
  },
};

/**
 * Responsive layout example.
 * Shows how pagination adapts to different container widths.
 */
export const Responsive: Story = {
  render: () => (
    <div className="space-y-8 w-full">
      <div>
        <p className="text-sm font-medium text-neutral-700 mb-3">
          Mobile (320px) - Use simple mode or small size
        </p>
        <div className="w-80 border-2 border-dashed border-neutral-300 p-4">
          <Pagination
            size="sm"
            variant="simple"
            currentPage={5}
            totalPages={20}
            showInfo
            onPageChange={(page) => console.log('Mobile - page:', page)}
          />
        </div>
      </div>

      <div>
        <p className="text-sm font-medium text-neutral-700 mb-3">
          Tablet (768px) - Medium with reduced maxVisible
        </p>
        <div className="w-[768px] border-2 border-dashed border-neutral-300 p-4">
          <Pagination
            size="md"
            currentPage={5}
            totalPages={20}
            maxVisible={5}
            onPageChange={(page) => console.log('Tablet - page:', page)}
          />
        </div>
      </div>

      <div>
        <p className="text-sm font-medium text-neutral-700 mb-3">
          Desktop (1024px+) - Full pagination
        </p>
        <div className="w-full border-2 border-dashed border-neutral-300 p-4">
          <Pagination
            size="md"
            currentPage={5}
            totalPages={20}
            showInfo
            onPageChange={(page) => console.log('Desktop - page:', page)}
          />
        </div>
      </div>
    </div>
  ),
};

/**
 * Interactive playground to experiment with all props.
 * Try different combinations of size, variant, and page counts.
 */
export const Playground: Story = {
  args: {
    currentPage: 5,
    totalPages: 20,
    size: 'md',
    variant: 'full',
    maxVisible: 7,
    showInfo: false,
    disabled: false,
    onPageChange: (page) => console.log('Go to page:', page),
  },
  parameters: {
    docs: {
      description: {
        story:
          'Experiment with all pagination props using the controls below. Try different sizes, variants, and page counts to see how the component adapts.',
      },
    },
  },
};

/**
 * Accessibility features demonstration.
 * All pagination controls support:
 * - Keyboard navigation (Tab to focus, Enter/Space to activate)
 * - Screen reader labels with ARIA attributes
 * - Visible focus indicators (ring on focus)
 * - aria-current for current page
 * - aria-live for page info updates
 */
export const Accessibility: Story = {
  render: () => {
    const [currentPage, setCurrentPage] = useState(3);

    return (
      <div className="space-y-6">
        <div>
          <p className="text-sm font-medium text-neutral-700 mb-3">
            Keyboard Navigation
          </p>
          <p className="text-sm text-neutral-600 mb-4">
            Press Tab to focus buttons, Enter or Space to activate. Current page
            is announced as "current page" to screen readers.
          </p>
          <Pagination
            currentPage={currentPage}
            totalPages={10}
            onPageChange={setCurrentPage}
            showInfo
          />
        </div>

        <div>
          <p className="text-sm font-medium text-neutral-700 mb-3">
            Focus Indicators
          </p>
          <p className="text-sm text-neutral-600 mb-4">
            Visible focus rings meet WCAG 2.1 AA contrast requirements. Try
            tabbing through the buttons.
          </p>
          <Pagination
            currentPage={5}
            totalPages={10}
            onPageChange={(page) => console.log('Focus test - page:', page)}
          />
        </div>

        <div>
          <p className="text-sm font-medium text-neutral-700 mb-3">
            Disabled State
          </p>
          <p className="text-sm text-neutral-600 mb-4">
            Disabled pagination prevents all interactions and is announced to
            screen readers.
          </p>
          <Pagination
            currentPage={5}
            totalPages={10}
            disabled
            onPageChange={(page) => console.log('Disabled - page:', page)}
          />
        </div>

        <div>
          <p className="text-sm font-medium text-neutral-700 mb-3">
            Live Region Updates
          </p>
          <p className="text-sm text-neutral-600 mb-4">
            Page info uses aria-live="polite" to announce changes to screen
            readers without interrupting.
          </p>
          <Pagination
            currentPage={currentPage}
            totalPages={10}
            showInfo
            onPageChange={setCurrentPage}
          />
        </div>
      </div>
    );
  },
  parameters: {
    docs: {
      description: {
        story:
          'Pagination is fully accessible with keyboard support, visible focus indicators, and proper ARIA attributes. All navigation controls are keyboard accessible and properly labeled for screen readers.',
      },
    },
  },
};
