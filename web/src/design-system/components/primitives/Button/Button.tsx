/**
 * Button Component
 *
 * Primary interactive element with multiple variants and states.
 * Follows the "Refined Trust Architecture" design system.
 *
 * Features:
 * - 5 variants: primary, secondary, outline, ghost, danger
 * - 4 sizes: sm, md, lg, xl
 * - Loading state with spinner
 * - Full keyboard accessibility
 * - WCAG 2.1 AA compliant
 */

import React from 'react';
import { cva, type VariantProps } from 'class-variance-authority';
import { cn } from '@design-system/utils';

const buttonVariants = cva(
  // Base styles - applied to all variants
  'inline-flex items-center justify-center font-medium rounded-lg transition-colors duration-150 focus:outline-none focus:ring-2 focus:ring-offset-2 disabled:cursor-not-allowed disabled:opacity-60',
  {
    variants: {
      variant: {
        // Primary: The accent - the one action the screen is for
        primary:
          'bg-accent text-accent-ink border-transparent hover:bg-accent-hover focus:ring-accent',

        // Secondary: Granting and connecting actions share the accent
        secondary:
          'bg-success-primary text-accent-ink border-transparent hover:bg-success-hover focus:ring-success-primary',

        // Outline: Surface with a hairline - the quiet alternative (Decline, Cancel)
        outline:
          'bg-white text-trust-deep border border-line-strong hover:bg-neutral-50 hover:border-neutral-400 focus:ring-accent',

        // Ghost: Transparent bg - subtle actions
        ghost:
          'bg-transparent text-trust-deep hover:bg-neutral-100 focus:ring-accent',

        // Danger: Outlined red - destructive actions stay legible without shouting
        danger:
          'bg-white text-error-primary border border-error-primary/40 hover:bg-error-light hover:border-error-primary focus:ring-error-primary',
      },
      size: {
        sm: 'px-3 py-1.5 text-sm h-9',
        md: 'px-4 py-3 text-base h-11',
        lg: 'px-6 py-3 text-lg h-12',
        xl: 'px-8 py-4 text-xl h-14',
      },
      fullWidth: {
        true: 'w-full',
        false: 'w-auto',
      },
    },
    defaultVariants: {
      variant: 'primary',
      size: 'md',
      fullWidth: false,
    },
  },
);

export interface ButtonProps
  extends
    React.ButtonHTMLAttributes<HTMLButtonElement>,
    VariantProps<typeof buttonVariants> {
  /** Whether button is in loading state */
  isLoading?: boolean;
  /** Icon to display before children */
  iconBefore?: React.ReactNode;
  /** Icon to display after children */
  iconAfter?: React.ReactNode;
}

/**
 * Button component with consistent styling and behavior.
 * Shows spinner when loading and disables interactions.
 *
 * @example
 * ```tsx
 * <Button variant="primary" size="md" onClick={handleClick}>
 *   Click me
 * </Button>
 *
 * <Button variant="outline" isLoading>
 *   Loading...
 * </Button>
 *
 * <Button variant="danger" iconBefore={<TrashIcon />}>
 *   Delete
 * </Button>
 * ```
 */
export const Button = React.forwardRef<HTMLButtonElement, ButtonProps>(
  (
    {
      variant,
      size,
      fullWidth,
      className,
      children,
      isLoading = false,
      disabled = false,
      iconBefore,
      iconAfter,
      type = 'button',
      ...props
    },
    ref,
  ) => {
    const isDisabled = disabled || isLoading;

    return (
      <button
        ref={ref}
        type={type}
        disabled={isDisabled}
        className={cn(buttonVariants({ variant, size, fullWidth }), className)}
        {...props}
      >
        {/* Loading spinner */}
        {isLoading && (
          <svg
            className="animate-spin -ml-1 mr-2 h-4 w-4"
            xmlns="http://www.w3.org/2000/svg"
            fill="none"
            viewBox="0 0 24 24"
            aria-hidden="true"
            data-testid="button-spinner"
          >
            <circle
              className="opacity-25"
              cx="12"
              cy="12"
              r="10"
              stroke="currentColor"
              strokeWidth="4"
            />
            <path
              className="opacity-75"
              fill="currentColor"
              d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z"
            />
          </svg>
        )}

        {/* Icon before */}
        {!isLoading && iconBefore && (
          <span className="mr-2 -ml-1 flex items-center">{iconBefore}</span>
        )}

        {/* Button content */}
        {children}

        {/* Icon after */}
        {iconAfter && (
          <span className="ml-2 -mr-1 flex items-center">{iconAfter}</span>
        )}
      </button>
    );
  },
);

Button.displayName = 'Button';

export default Button;
