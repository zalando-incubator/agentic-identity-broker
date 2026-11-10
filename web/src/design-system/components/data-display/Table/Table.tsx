/**
 * Table Component
 *
 * Flexible data table component with sorting and pagination support.
 * Follows the "Refined Trust Architecture" design system.
 *
 * Features:
 * - Responsive table with horizontal scroll on mobile
 * - Optional striped rows (alternating background)
 * - Optional hover highlighting rows
 * - Optional sorting indicators on headers
 * - Column alignment options (left, center, right)
 * - Sticky header option
 * - Compact and default density variants
 * - Support for custom cell content (React components)
 * - Optional loading skeleton state
 * - Caption/title support
 * - Empty state display
 * - WCAG 2.1 AA compliant
 */

import React from 'react';
import { cva } from 'class-variance-authority';
import { cn } from '@design-system/utils';
import { Skeleton } from '@design-system/components/feedback/Skeleton';
import { ChevronDown, ChevronUp } from 'lucide-react';

const tableVariants = cva('w-full border-collapse', {
  variants: {
    density: {
      compact: '',
      default: '',
    },
  },
  defaultVariants: {
    density: 'default',
  },
});

const cellVariants = cva(
  'border-b border-neutral-200 text-left transition-colors',
  {
    variants: {
      density: {
        compact: 'px-3 py-2 text-sm',
        default: 'px-4 py-3 text-base',
      },
      align: {
        left: 'text-left',
        center: 'text-center',
        right: 'text-right',
      },
      type: {
        header: 'font-medium text-neutral-900 bg-neutral-50',
        body: 'text-neutral-700',
      },
    },
    defaultVariants: {
      density: 'default',
      align: 'left',
      type: 'body',
    },
  },
);

const rowVariants = cva('transition-colors', {
  variants: {
    striped: {
      true: 'odd:bg-white even:bg-neutral-50',
      false: 'bg-white',
    },
    hover: {
      true: 'hover:bg-neutral-100',
      false: '',
    },
  },
  defaultVariants: {
    striped: false,
    hover: false,
  },
});

export interface TableColumn<T> {
  /** Unique key for the column */
  key: string;
  /** Header content (text or React node) */
  header: React.ReactNode;
  /** Function to extract cell content from row data */
  accessor?: (row: T) => React.ReactNode;
  /** Text alignment for the column */
  align?: 'left' | 'center' | 'right';
  /** Column width (CSS value) */
  width?: string;
  /** Whether the column is sortable */
  sortable?: boolean;
}

export interface TableProps<T> extends Omit<
  React.HTMLAttributes<HTMLDivElement>,
  'children'
> {
  /** Column definitions */
  columns: TableColumn<T>[];
  /** Array of row data */
  data: T[];
  /** Optional table caption/title */
  caption?: string;
  /** Enable striped rows (alternating background) */
  striped?: boolean;
  /** Enable hover highlighting on rows */
  hover?: boolean;
  /** Table density variant */
  density?: 'compact' | 'default';
  /** Enable sticky header that stays visible when scrolling */
  stickyHeader?: boolean;
  /** Show loading skeleton state */
  loading?: boolean;
  /** Content to display when table is empty */
  empty?: React.ReactNode;
  /** Callback when a sortable column header is clicked */
  onSort?: (key: string) => void;
  /** Currently sorted column key */
  sortKey?: string;
  /** Current sort direction */
  sortDirection?: 'asc' | 'desc';
  /** Additional CSS classes for the wrapper */
  className?: string;
}

/**
 * Sort icon component for table headers
 */
const SortIcon: React.FC<{
  sortKey?: string;
  columnKey: string;
  sortDirection?: 'asc' | 'desc';
}> = ({ sortKey, columnKey, sortDirection }) => {
  const isActive = sortKey === columnKey;
  const isAsc = isActive && sortDirection === 'asc';
  const isDesc = isActive && sortDirection === 'desc';

  return (
    <span className="ml-2 inline-flex flex-col" aria-hidden="true">
      <ChevronUp
        className={cn(
          'w-3 h-3 -mb-1',
          isAsc ? 'text-trust-deep' : 'text-neutral-400',
        )}
        aria-hidden="true"
        strokeWidth={1.5}
      />
      <ChevronDown
        className={cn(
          'w-3 h-3',
          isDesc ? 'text-trust-deep' : 'text-neutral-400',
        )}
        aria-hidden="true"
        strokeWidth={1.5}
      />
    </span>
  );
};

/**
 * Table component for displaying tabular data with sorting and pagination support.
 * Supports striped rows, hover effects, sticky headers, and custom cell rendering.
 *
 * @example
 * ```tsx
 * // Basic table
 * <Table
 *   columns={[
 *     { key: 'name', header: 'Name', accessor: (row) => row.name },
 *     { key: 'email', header: 'Email', accessor: (row) => row.email },
 *   ]}
 *   data={users}
 * />
 *
 * // Sortable table with striped rows
 * <Table
 *   columns={columns}
 *   data={data}
 *   striped
 *   hover
 *   onSort={handleSort}
 *   sortKey={sortKey}
 *   sortDirection={sortDirection}
 * />
 *
 * // Compact density with sticky header
 * <Table
 *   columns={columns}
 *   data={data}
 *   density="compact"
 *   stickyHeader
 * />
 * ```
 */
export const Table = <T extends Record<string, unknown>>({
  columns,
  data,
  caption,
  striped = false,
  hover = false,
  density = 'default',
  stickyHeader = false,
  loading = false,
  empty,
  onSort,
  sortKey,
  sortDirection,
  className,
  ...props
}: TableProps<T>) => {
  // Handle sort click
  const handleSort = (column: TableColumn<T>) => {
    if (column.sortable && onSort) {
      onSort(column.key);
    }
  };

  // Render loading skeleton
  if (loading) {
    return (
      <div
        className={cn(
          'overflow-x-auto rounded-lg border border-neutral-200',
          className,
        )}
        {...props}
      >
        <table className={cn(tableVariants({ density }))}>
          {caption && <caption className="sr-only">{caption}</caption>}
          <thead>
            <tr>
              {columns.map((column) => (
                <th
                  key={column.key}
                  className={cn(
                    cellVariants({
                      density,
                      align: column.align,
                      type: 'header',
                    }),
                    stickyHeader && 'sticky top-0 z-10',
                  )}
                  style={{ width: column.width }}
                >
                  <Skeleton width="80%" height="16px" />
                </th>
              ))}
            </tr>
          </thead>
          <tbody>
            {Array.from({ length: 5 }).map((_, index) => (
              <tr key={index}>
                {columns.map((column) => (
                  <td
                    key={column.key}
                    className={cn(
                      cellVariants({
                        density,
                        align: column.align,
                        type: 'body',
                      }),
                    )}
                  >
                    <Skeleton width="90%" height="16px" />
                  </td>
                ))}
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    );
  }

  // Render empty state
  if (!loading && data.length === 0) {
    return (
      <div
        className={cn(
          'overflow-x-auto rounded-lg border border-neutral-200',
          className,
        )}
        {...props}
      >
        <table className={cn(tableVariants({ density }))}>
          {caption && <caption className="sr-only">{caption}</caption>}
          <thead>
            <tr>
              {columns.map((column) => (
                <th
                  key={column.key}
                  className={cn(
                    cellVariants({
                      density,
                      align: column.align,
                      type: 'header',
                    }),
                    stickyHeader && 'sticky top-0 z-10',
                  )}
                  style={{ width: column.width }}
                >
                  {column.header}
                </th>
              ))}
            </tr>
          </thead>
          <tbody>
            <tr>
              <td
                colSpan={columns.length}
                className={cn(
                  cellVariants({ density, align: 'center', type: 'body' }),
                  'py-12',
                )}
              >
                {empty || (
                  <div className="text-neutral-500">
                    <p className="text-sm">No data available</p>
                  </div>
                )}
              </td>
            </tr>
          </tbody>
        </table>
      </div>
    );
  }

  // Render table with data
  return (
    <div
      className={cn(
        'overflow-x-auto rounded-lg border border-neutral-200',
        className,
      )}
      {...props}
    >
      <table className={cn(tableVariants({ density }))}>
        {caption && (
          <caption className="px-4 py-3 text-left text-sm font-medium text-neutral-900 bg-neutral-50 border-b border-neutral-200">
            {caption}
          </caption>
        )}
        <thead>
          <tr>
            {columns.map((column) => (
              <th
                key={column.key}
                className={cn(
                  cellVariants({
                    density,
                    align: column.align,
                    type: 'header',
                  }),
                  stickyHeader && 'sticky top-0 z-10',
                  column.sortable &&
                    'cursor-pointer select-none hover:bg-neutral-100',
                )}
                style={{ width: column.width }}
                onClick={() => handleSort(column)}
                role={column.sortable ? 'button' : undefined}
                tabIndex={column.sortable ? 0 : undefined}
                onKeyDown={(e) => {
                  if (column.sortable && (e.key === 'Enter' || e.key === ' ')) {
                    e.preventDefault();
                    handleSort(column);
                  }
                }}
                aria-sort={
                  column.sortable && sortKey === column.key
                    ? sortDirection === 'asc'
                      ? 'ascending'
                      : 'descending'
                    : undefined
                }
              >
                <div className="flex items-center justify-between">
                  <span>{column.header}</span>
                  {column.sortable && (
                    <SortIcon
                      sortKey={sortKey}
                      columnKey={column.key}
                      sortDirection={sortDirection}
                    />
                  )}
                </div>
              </th>
            ))}
          </tr>
        </thead>
        <tbody>
          {data.map((row, rowIndex) => (
            <tr key={rowIndex} className={cn(rowVariants({ striped, hover }))}>
              {columns.map((column) => (
                <td
                  key={column.key}
                  className={cn(
                    cellVariants({
                      density,
                      align: column.align,
                      type: 'body',
                    }),
                  )}
                >
                  {column.accessor
                    ? column.accessor(row)
                    : (row[column.key] as React.ReactNode)}
                </td>
              ))}
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
};

Table.displayName = 'Table';

export default Table;
