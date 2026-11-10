/**
 * Pagination Component
 *
 * Navigation component for datasets with multiple pages.
 * Follows the "Refined Trust Architecture" design system.
 *
 * Features:
 * - Full pagination with numbered pages
 * - Simple mode with only prev/next buttons
 * - 3 sizes: sm, md, lg
 * - Smart ellipsis for large page counts
 * - Optional page info display
 * - Full keyboard accessibility
 * - WCAG 2.1 AA compliant
 */

import React from 'react';
import { cva } from 'class-variance-authority';
import { cn } from '@design-system/utils';
import { ChevronLeft, ChevronRight } from 'lucide-react';

const paginationVariants = cva(
  // Base styles - applied to all variants
  'flex items-center justify-center gap-1',
  {
    variants: {
      size: {
        sm: 'text-sm',
        md: 'text-base',
        lg: 'text-lg',
      },
    },
    defaultVariants: {
      size: 'md',
    },
  },
);

const pageButtonVariants = cva(
  'inline-flex items-center justify-center font-medium rounded-md transition-colors duration-150 focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-trust disabled:cursor-not-allowed disabled:opacity-40',
  {
    variants: {
      size: {
        sm: 'min-w-[2rem] h-8 px-2 text-sm',
        md: 'min-w-[2.5rem] h-10 px-3 text-base',
        lg: 'min-w-[3rem] h-12 px-4 text-lg',
      },
      variant: {
        default:
          'bg-white text-trust-deep border border-neutral-300 hover:bg-neutral-50 hover:border-neutral-400',
        active: 'bg-trust-deep text-white border-transparent',
        ghost: 'bg-transparent text-trust-deep hover:bg-neutral-100',
      },
    },
    defaultVariants: {
      size: 'md',
      variant: 'default',
    },
  },
);

export interface PaginationProps extends Omit<
  React.ComponentPropsWithoutRef<'nav'>,
  'onChange'
> {
  /** Current active page (1-indexed) */
  currentPage: number;
  /** Total number of pages */
  totalPages: number;
  /** Callback when page changes */
  onPageChange: (page: number) => void;
  /** Size variant */
  size?: 'sm' | 'md' | 'lg';
  /** Display mode */
  variant?: 'simple' | 'full';
  /** Maximum visible page numbers (for full mode) */
  maxVisible?: number;
  /** Show page info text (e.g., "Page 2 of 10") */
  showInfo?: boolean;
  /** Disable all interactions */
  disabled?: boolean;
}

/**
 * Generate array of page numbers with ellipsis
 * Algorithm: Always show first, last, current, and adjacent pages
 */
function generatePageNumbers(
  currentPage: number,
  totalPages: number,
  maxVisible: number,
): (number | 'ellipsis')[] {
  if (totalPages <= maxVisible) {
    return Array.from({ length: totalPages }, (_, i) => i + 1);
  }

  const pages: (number | 'ellipsis')[] = [];
  const halfVisible = Math.floor(maxVisible / 2);

  // Always show first page
  pages.push(1);

  // Calculate range around current page
  let start = Math.max(2, currentPage - halfVisible);
  let end = Math.min(totalPages - 1, currentPage + halfVisible);

  // Adjust if we're near the start or end
  if (currentPage <= halfVisible + 1) {
    end = Math.min(totalPages - 1, maxVisible - 1);
  }
  if (currentPage >= totalPages - halfVisible) {
    start = Math.max(2, totalPages - maxVisible + 2);
  }

  // Add ellipsis before if needed
  if (start > 2) {
    pages.push('ellipsis');
  }

  // Add page numbers in range
  for (let i = start; i <= end; i++) {
    pages.push(i);
  }

  // Add ellipsis after if needed
  if (end < totalPages - 1) {
    pages.push('ellipsis');
  }

  // Always show last page (if more than 1 page)
  if (totalPages > 1) {
    pages.push(totalPages);
  }

  return pages;
}

/**
 * Pagination component for navigating through dataset pages.
 * Supports both simple (prev/next only) and full (with page numbers) modes.
 *
 * @example
 * ```tsx
 * // Full pagination
 * <Pagination
 *   currentPage={5}
 *   totalPages={20}
 *   onPageChange={(page) => console.log('Go to page:', page)}
 * />
 *
 * // Simple mode with info
 * <Pagination
 *   variant="simple"
 *   showInfo
 *   currentPage={2}
 *   totalPages={10}
 *   onPageChange={handlePageChange}
 * />
 *
 * // Small size
 * <Pagination
 *   size="sm"
 *   currentPage={1}
 *   totalPages={5}
 *   onPageChange={handlePageChange}
 * />
 * ```
 */
export const Pagination = React.forwardRef<HTMLElement, PaginationProps>(
  (
    {
      currentPage,
      totalPages,
      onPageChange,
      size = 'md',
      variant = 'full',
      maxVisible = 7,
      showInfo = false,
      disabled = false,
      className,
      ...props
    },
    ref,
  ) => {
    const isFirstPage = currentPage === 1;
    const isLastPage = currentPage === totalPages;
    const hasSinglePage = totalPages <= 1;

    // Ensure size always has a value for CVA
    const resolvedSize = (size ?? 'md') as 'sm' | 'md' | 'lg';

    const handlePrevious = () => {
      if (!isFirstPage && !disabled) {
        onPageChange(currentPage - 1);
      }
    };

    const handleNext = () => {
      if (!isLastPage && !disabled) {
        onPageChange(currentPage + 1);
      }
    };

    const handlePageClick = (page: number) => {
      if (!disabled && page !== currentPage) {
        onPageChange(page);
      }
    };

    const pages =
      variant === 'full'
        ? generatePageNumbers(currentPage, totalPages, maxVisible)
        : [];

    return (
      <nav
        ref={ref}
        role="navigation"
        aria-label="Pagination navigation"
        className={cn('flex flex-col items-center gap-4', className)}
        {...props}
      >
        <div className={paginationVariants({ size: resolvedSize })}>
          {/* Previous button */}
          <button
            type="button"
            onClick={handlePrevious}
            disabled={isFirstPage || disabled || hasSinglePage}
            aria-label="Go to previous page"
            className={pageButtonVariants({
              size: resolvedSize,
              variant: 'default',
            })}
          >
            <ChevronLeft
              className={cn(
                'shrink-0',
                resolvedSize === 'sm' && 'w-4 h-4',
                resolvedSize === 'md' && 'w-5 h-5',
                resolvedSize === 'lg' && 'w-6 h-6',
              )}
              aria-hidden="true"
              strokeWidth={1.5}
            />
          </button>

          {/* Page numbers (full mode only) */}
          {variant === 'full' &&
            pages.map((page, index) => {
              if (page === 'ellipsis') {
                return (
                  <span
                    key={`ellipsis-${index}`}
                    className={cn(
                      'inline-flex items-center justify-center text-neutral-500',
                      resolvedSize === 'sm' && 'w-8 text-sm',
                      resolvedSize === 'md' && 'w-10 text-base',
                      resolvedSize === 'lg' && 'w-12 text-lg',
                    )}
                    aria-hidden="true"
                  >
                    ...
                  </span>
                );
              }

              const pageNum = page as number;
              const isActive = pageNum === currentPage;

              return (
                <button
                  key={pageNum}
                  type="button"
                  onClick={() => handlePageClick(pageNum)}
                  disabled={disabled}
                  aria-label={`Go to page ${pageNum}`}
                  aria-current={isActive ? 'page' : undefined}
                  className={pageButtonVariants({
                    size: resolvedSize,
                    variant: isActive ? 'active' : 'default',
                  })}
                >
                  {pageNum}
                </button>
              );
            })}

          {/* Next button */}
          <button
            type="button"
            onClick={handleNext}
            disabled={isLastPage || disabled || hasSinglePage}
            aria-label="Go to next page"
            className={pageButtonVariants({
              size: resolvedSize,
              variant: 'default',
            })}
          >
            <ChevronRight
              className={cn(
                'shrink-0',
                resolvedSize === 'sm' && 'w-4 h-4',
                resolvedSize === 'md' && 'w-5 h-5',
                resolvedSize === 'lg' && 'w-6 h-6',
              )}
              aria-hidden="true"
              strokeWidth={1.5}
            />
          </button>
        </div>

        {/* Page info text */}
        {showInfo && totalPages > 0 && (
          <p
            className={cn(
              'text-neutral-600',
              resolvedSize === 'sm' && 'text-xs',
              resolvedSize === 'md' && 'text-sm',
              resolvedSize === 'lg' && 'text-base',
            )}
            aria-live="polite"
            aria-atomic="true"
          >
            Page {currentPage} of {totalPages}
          </p>
        )}
      </nav>
    );
  },
);

Pagination.displayName = 'Pagination';

export default Pagination;
