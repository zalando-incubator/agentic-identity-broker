/**
 * Breadcrumb Component
 *
 * Navigation component that displays the current location hierarchy.
 * Follows the "Refined Trust Architecture" design system.
 *
 * Features:
 * - Semantic HTML (nav, ol, li)
 * - 3 sizes: sm, md, lg
 * - Customizable separators
 * - Optional icons per item
 * - Current page indicator
 * - Truncation support for long paths
 * - Collapsible middle items
 * - Full keyboard accessibility
 * - WCAG 2.1 AA compliant
 */

import React from 'react';
import { cva } from 'class-variance-authority';
import { cn } from '@design-system/utils';
import { ChevronRight, EllipsisVertical } from 'lucide-react';

const breadcrumbVariants = cva(
  // Base styles for the nav container
  'inline-flex items-center',
  {
    variants: {
      size: {
        sm: 'text-xs gap-1',
        md: 'text-sm gap-2',
        lg: 'text-base gap-3',
      },
    },
    defaultVariants: {
      size: 'md',
    },
  },
);

const breadcrumbItemVariants = cva(
  // Base styles for individual breadcrumb items
  'inline-flex items-center transition-colors duration-200',
  {
    variants: {
      size: {
        sm: 'gap-1',
        md: 'gap-1.5',
        lg: 'gap-2',
      },
      isCurrent: {
        true: 'font-medium text-trust-deep',
        false: '',
      },
    },
    defaultVariants: {
      size: 'md',
      isCurrent: false,
    },
  },
);

const breadcrumbLinkVariants = cva(
  'hover:underline focus:outline-none focus:ring-2 focus:ring-trust focus:ring-offset-1 rounded-sm transition-colors duration-200',
  {
    variants: {
      disabled: {
        true: 'opacity-50 cursor-not-allowed pointer-events-none',
        false: 'text-trust-hover hover:text-trust-deep',
      },
    },
    defaultVariants: {
      disabled: false,
    },
  },
);

const breadcrumbSeparatorVariants = cva('text-neutral-400 select-none', {
  variants: {
    size: {
      sm: 'text-xs',
      md: 'text-sm',
      lg: 'text-base',
    },
  },
  defaultVariants: {
    size: 'md',
  },
});

export interface BreadcrumbItem {
  /** Display label for the breadcrumb item */
  label: string;
  /** Optional href for navigation */
  href?: string;
  /** Optional icon to display before label */
  icon?: React.ReactNode;
  /** Whether this item is disabled */
  disabled?: boolean;
}

export interface BreadcrumbProps extends React.ComponentPropsWithoutRef<'nav'> {
  /** Array of breadcrumb items to display */
  items: BreadcrumbItem[];
  /** Size variant */
  size?: 'sm' | 'md' | 'lg';
  /** Custom separator element (defaults to /) */
  separator?: React.ReactNode;
  /** Maximum number of items to show (truncates middle items if exceeded) */
  maxItems?: number;
  /** Whether to show icons for items that have them */
  showIcon?: boolean;
  /** Custom aria-label for the navigation */
  'aria-label'?: string;
}

/**
 * Default chevron separator icon
 */
const ChevronSeparator: React.FC<{ className?: string }> = ({ className }) => (
  <ChevronRight className={className} aria-hidden="true" strokeWidth={1.5} />
);

/**
 * Ellipsis icon for collapsed items
 */
const EllipsisIcon: React.FC<{ className?: string }> = ({ className }) => (
  <EllipsisVertical
    className={className}
    aria-hidden="true"
    strokeWidth={1.5}
  />
);

/**
 * Breadcrumb navigation component for showing location hierarchy.
 * Automatically marks the last item as current page (non-interactive).
 *
 * @example
 * ```tsx
 * <Breadcrumb
 *   items={[
 *     { label: 'Home', href: '/' },
 *     { label: 'Settings', href: '/settings' },
 *     { label: 'Profile' },
 *   ]}
 * />
 *
 * <Breadcrumb
 *   items={items}
 *   size="lg"
 *   separator={<span>→</span>}
 *   maxItems={3}
 * />
 * ```
 */
export const Breadcrumb = React.forwardRef<HTMLElement, BreadcrumbProps>(
  (
    {
      items,
      size = 'md',
      separator,
      maxItems,
      showIcon = true,
      className,
      'aria-label': ariaLabel = 'Breadcrumb',
      ...props
    },
    ref,
  ) => {
    // Process items for truncation
    const processedItems = React.useMemo(() => {
      if (!maxItems || items.length <= maxItems) {
        return items;
      }

      // Keep first item, last item, and show ellipsis for middle items
      const firstItem = items[0];
      const lastItems = items.slice(-(maxItems - 1));
      const collapsedCount = items.length - maxItems;

      return [
        firstItem,
        {
          label: `${collapsedCount} more`,
          disabled: true,
          icon: <EllipsisIcon className="w-4 h-4" />,
        } as BreadcrumbItem,
        ...lastItems,
      ];
    }, [items, maxItems]);

    // Default separator
    const defaultSeparator = (
      <ChevronSeparator
        className={cn(
          'w-3 h-3',
          size === 'sm' && 'w-2.5 h-2.5',
          size === 'lg' && 'w-3.5 h-3.5',
        )}
      />
    );

    const separatorElement = separator ?? defaultSeparator;

    return (
      <nav
        ref={ref}
        aria-label={ariaLabel}
        className={cn(breadcrumbVariants({ size }), className)}
        {...props}
      >
        <ol className="inline-flex items-center list-none m-0 p-0">
          {processedItems.map((item, index) => {
            const isLast = index === processedItems.length - 1;
            const isCurrent = isLast && !item.href;
            const isLink = item.href && !item.disabled;

            return (
              <li
                key={`${item.label}-${index}`}
                className={cn(breadcrumbItemVariants({ size, isCurrent }))}
              >
                {/* Item content */}
                {isLink ? (
                  <a
                    href={item.href}
                    className={cn(
                      breadcrumbLinkVariants({ disabled: item.disabled }),
                      'inline-flex items-center',
                      size === 'sm' && 'gap-1',
                      size === 'md' && 'gap-1.5',
                      size === 'lg' && 'gap-2',
                    )}
                    aria-current={isCurrent ? 'page' : undefined}
                    aria-disabled={item.disabled}
                  >
                    {showIcon && item.icon && (
                      <span
                        className="inline-flex items-center flex-shrink-0"
                        aria-hidden="true"
                      >
                        {item.icon}
                      </span>
                    )}
                    <span>{item.label}</span>
                  </a>
                ) : (
                  <span
                    className={cn(
                      'inline-flex items-center',
                      size === 'sm' && 'gap-1',
                      size === 'md' && 'gap-1.5',
                      size === 'lg' && 'gap-2',
                      item.disabled && 'opacity-50 cursor-not-allowed',
                    )}
                    aria-current={isCurrent ? 'page' : undefined}
                    aria-disabled={item.disabled}
                  >
                    {showIcon && item.icon && (
                      <span
                        className="inline-flex items-center flex-shrink-0"
                        aria-hidden="true"
                      >
                        {item.icon}
                      </span>
                    )}
                    <span>{item.label}</span>
                  </span>
                )}

                {/* Separator */}
                {!isLast && (
                  <span
                    className={cn(
                      breadcrumbSeparatorVariants({ size }),
                      'mx-1',
                      size === 'sm' && 'mx-0.5',
                      size === 'lg' && 'mx-2',
                    )}
                    aria-hidden="true"
                  >
                    {separatorElement}
                  </span>
                )}
              </li>
            );
          })}
        </ol>
      </nav>
    );
  },
);

Breadcrumb.displayName = 'Breadcrumb';

export default Breadcrumb;
