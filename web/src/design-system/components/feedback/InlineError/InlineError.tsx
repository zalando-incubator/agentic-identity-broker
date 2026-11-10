/**
 * InlineError Component
 *
 * Field-level error message component for form validation feedback.
 * Follows the "Refined Trust Architecture" design system.
 *
 * Features:
 * - Compact inline display below form fields
 * - Optional error icon
 * - Support for multiple validation errors
 * - Optional suggestion text for guidance
 * - Support for actionable links
 * - WCAG 2.1 AA compliant with proper ARIA attributes
 */

import React from 'react';
import { cva } from 'class-variance-authority';
import { cn } from '@design-system/utils';
import { CircleX } from 'lucide-react';

const inlineErrorVariants = cva(
  // Base styles - compact for field-level display
  'flex gap-2 mt-1 text-sm text-error-dark transition-opacity duration-150',
  {
    variants: {
      size: {
        sm: 'text-xs',
        md: 'text-sm',
      },
    },
    defaultVariants: {
      size: 'md',
    },
  },
);

const iconVariants = cva('flex-shrink-0 text-error-primary', {
  variants: {
    size: {
      sm: 'w-3.5 h-3.5 mt-0.5',
      md: 'w-4 h-4 mt-0.5',
    },
  },
  defaultVariants: {
    size: 'md',
  },
});

export interface InlineErrorProps extends Omit<
  React.ComponentPropsWithoutRef<'div'>,
  'children'
> {
  /** Error message text */
  message: string;
  /** Optional custom error icon (overrides default) */
  icon?: React.ReactNode;
  /** Hide the default error icon */
  hideIcon?: boolean;
  /** Array of validation error messages (for multiple errors) */
  errors?: string[];
  /** Optional suggestion text to help user fix the error */
  suggestions?: string[];
  /** Optional helper text shown below error */
  helperText?: string;
  /** Associated field label (for context) */
  fieldLabel?: string;
  /** Size variant */
  size?: 'sm' | 'md';
}

// Default error icon (X in circle)
const ErrorIcon = ({ className }: { className?: string }) => (
  <CircleX
    className={cn('w-4 h-4', className)}
    aria-hidden="true"
    strokeWidth={1.5}
  />
);

/**
 * InlineError component for displaying field-level validation errors.
 * Typically positioned below form inputs to provide immediate feedback.
 *
 * @example
 * ```tsx
 * <InlineError message="Email address is required" />
 *
 * <InlineError
 *   message="Password is too weak"
 *   suggestions={["Use at least 8 characters", "Include numbers and symbols"]}
 * />
 *
 * <InlineError
 *   errors={["Username is required", "Username must be at least 3 characters"]}
 * />
 * ```
 */
export const InlineError = React.forwardRef<HTMLDivElement, InlineErrorProps>(
  (
    {
      message,
      icon,
      hideIcon = false,
      errors,
      suggestions,
      helperText,
      fieldLabel,
      size = 'md',
      className,
      ...props
    },
    ref,
  ) => {
    // Determine which icon to show
    const displayIcon = icon || <ErrorIcon />;

    // Build list of error messages
    const errorMessages = errors && errors.length > 0 ? errors : [message];
    const hasMultipleErrors = errorMessages.length > 1;

    return (
      <div
        ref={ref}
        role="alert"
        aria-live="polite"
        aria-atomic="true"
        className={cn('space-y-1', className)}
        {...props}
      >
        {/* Main error message(s) */}
        {hasMultipleErrors ? (
          <ul className="space-y-1">
            {errorMessages.map((error, index) => (
              <li key={index} className={inlineErrorVariants({ size })}>
                {!hideIcon && (
                  <span className={iconVariants({ size })}>{displayIcon}</span>
                )}
                <span className="flex-1">{error}</span>
              </li>
            ))}
          </ul>
        ) : (
          <div className={inlineErrorVariants({ size })}>
            {!hideIcon && (
              <span className={iconVariants({ size })}>{displayIcon}</span>
            )}
            <span className="flex-1">{message}</span>
          </div>
        )}

        {/* Suggestions (if provided) */}
        {suggestions && suggestions.length > 0 && (
          <div
            className={cn(
              'ml-6 text-xs text-error-dark/80 space-y-0.5',
              size === 'sm' && 'ml-5',
            )}
          >
            {suggestions.map((suggestion, index) => (
              <div key={index} className="flex items-start gap-1.5">
                <span className="text-error-primary mt-0.5">•</span>
                <span>{suggestion}</span>
              </div>
            ))}
          </div>
        )}

        {/* Helper text (if provided) */}
        {helperText && (
          <div
            className={cn(
              'ml-6 text-xs text-error-dark/70 italic',
              size === 'sm' && 'ml-5',
            )}
          >
            {helperText}
          </div>
        )}

        {/* Field label context (if provided) */}
        {fieldLabel && (
          <span className="sr-only">Error for field: {fieldLabel}</span>
        )}
      </div>
    );
  },
);

InlineError.displayName = 'InlineError';

export default InlineError;
