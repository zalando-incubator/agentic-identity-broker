/**
 * Badge Component
 *
 * Versatile badge component for status indicators, labels, and counts.
 * Follows the "Refined Trust Architecture" design system.
 *
 * Features:
 * - 5 semantic variants: success, error, warning, info, neutral
 * - 3 sizes: sm, md, lg
 * - Optional icon support (before/after)
 * - Optional dot indicator
 * - Pill or rounded shape
 * - WCAG 2.1 AA compliant
 */

import React from 'react';
import { cva, type VariantProps } from 'class-variance-authority';
import { cn } from '@design-system/utils';

const badgeVariants = cva(
  // Base styles - applied to all variants
  'inline-flex items-center justify-center font-medium border transition-colors',
  {
    variants: {
      variant: {
        // Success: the granted state on its tint, accent text
        success: 'bg-success-light text-success-dark border-success-primary/30',

        // Error: negative or destructive status on its tint
        error: 'bg-error-light text-error-dark border-error-primary/30',

        // Warning: caution status on its tint
        warning: 'bg-warning-light text-warning-dark border-warning-primary/30',

        // Info: informational status on its tint
        info: 'bg-info-light text-info-dark border-info-primary/30',

        // Neutral: raised surface with a hairline, for optional or inactive
        neutral: 'bg-neutral-100 text-neutral-600 border-neutral-200',

        // Primary: ink, for a fact that always holds, such as a required set
        primary: 'bg-trust-deep text-white border-trust-deep',
      },
      size: {
        sm: 'px-2 py-0.5 text-xs gap-1',
        md: 'px-2.5 py-1 text-sm gap-1.5',
        lg: 'px-3 py-1.5 text-base gap-2',
      },
      shape: {
        rounded: 'rounded-md',
        pill: 'rounded-full',
      },
    },
    defaultVariants: {
      variant: 'neutral',
      size: 'md',
      shape: 'rounded',
    },
  },
);

export interface BadgeProps
  extends
    React.HTMLAttributes<HTMLSpanElement>,
    VariantProps<typeof badgeVariants> {
  /** Icon to display before children */
  iconBefore?: React.ReactNode;
  /** Icon to display after children */
  iconAfter?: React.ReactNode;
  /** Show a dot indicator before the content */
  showDot?: boolean;
}

/**
 * Badge component for displaying status, labels, or counts.
 * Supports icons, dots, and multiple semantic variants.
 *
 * @example
 * ```tsx
 * <Badge variant="success">Active</Badge>
 *
 * <Badge variant="warning" showDot>
 *   Pending
 * </Badge>
 *
 * <Badge variant="info" iconBefore={<InfoIcon />}>
 *   New
 * </Badge>
 *
 * <Badge variant="neutral" size="sm" shape="rounded">
 *   Beta
 * </Badge>
 * ```
 */
export const Badge = React.forwardRef<HTMLSpanElement, BadgeProps>(
  (
    {
      variant,
      size,
      shape,
      className,
      children,
      iconBefore,
      iconAfter,
      showDot = false,
      ...props
    },
    ref,
  ) => {
    // Icon sizing based on badge size
    const iconSize =
      size === 'sm' ? 'w-3 h-3' : size === 'lg' ? 'w-4 h-4' : 'w-3.5 h-3.5';
    const dotSize =
      size === 'sm' ? 'w-1.5 h-1.5' : size === 'lg' ? 'w-2.5 h-2.5' : 'w-2 h-2';

    return (
      <span
        ref={ref}
        className={cn(badgeVariants({ variant, size, shape }), className)}
        {...props}
      >
        {/* Dot indicator */}
        {showDot && (
          <span
            className={cn('rounded-full bg-current', dotSize)}
            aria-hidden="true"
          />
        )}

        {/* Icon before */}
        {iconBefore && (
          <span
            className={cn('flex items-center', iconSize)}
            aria-hidden="true"
          >
            {iconBefore}
          </span>
        )}

        {/* Badge content */}
        {children}

        {/* Icon after */}
        {iconAfter && (
          <span
            className={cn('flex items-center', iconSize)}
            aria-hidden="true"
          >
            {iconAfter}
          </span>
        )}
      </span>
    );
  },
);

Badge.displayName = 'Badge';

export default Badge;
