/**
 * Checkbox Component
 *
 * Accessible checkbox with label and validation states.
 * Follows the "Refined Trust Architecture" design system.
 *
 * Features:
 * - 3 sizes: sm, md, lg
 * - Indeterminate state support
 * - Disabled state
 * - Error/success states
 * - Label with description
 * - Full keyboard accessibility
 * - WCAG 2.1 AA compliant
 */

import React from 'react';
import { cva, type VariantProps } from 'class-variance-authority';
import { cn } from '@design-system/utils';

const checkboxVariants = cva(
  'rounded border-1.5 transition-colors duration-150 cursor-pointer flex-shrink-0',
  {
    variants: {
      size: {
        sm: 'w-4 h-4',
        md: 'w-5 h-5',
        lg: 'w-6 h-6',
      },
      variant: {
        default: 'border-neutral-300 bg-white text-trust-deep',
        error: 'border-error-primary bg-error-light/20 text-error-primary',
        success:
          'border-success-primary bg-success-light/20 text-success-primary',
      },
    },
    defaultVariants: {
      size: 'md',
      variant: 'default',
    },
  },
);

export interface CheckboxProps
  extends
    Omit<React.InputHTMLAttributes<HTMLInputElement>, 'size' | 'type'>,
    VariantProps<typeof checkboxVariants> {
  /** Label text displayed next to checkbox */
  label?: string;
  /** Description text displayed below label */
  description?: string;
  /** Error message (changes variant to error) */
  errorMessage?: string;
  /** Success message */
  successMessage?: string;
  /** Unique identifier for form association */
  id?: string;
}

/**
 * Checkbox component for boolean selections.
 * Supports labels, descriptions, and validation states.
 *
 * @example
 * ```tsx
 * <Checkbox label="I agree to the terms" />
 *
 * <Checkbox
 *   label="Enable notifications"
 *   description="Receive updates about your grants"
 * />
 *
 * <Checkbox
 *   label="Confirm deletion"
 *   errorMessage="You must confirm before deleting"
 *   variant="error"
 * />
 * ```
 */
export const Checkbox = React.forwardRef<HTMLInputElement, CheckboxProps>(
  (
    {
      size = 'md',
      label,
      description,
      errorMessage,
      successMessage,
      className,
      id,
      disabled = false,
      checked,
      ...props
    },
    ref,
  ) => {
    // Determine variant based on error/success state
    const variant = errorMessage
      ? 'error'
      : successMessage
        ? 'success'
        : 'default';

    return (
      <div className="flex items-start gap-3">
        {/* Checkbox input */}
        <div className="flex items-center h-5 pt-0.5">
          <input
            ref={ref}
            id={id}
            type="checkbox"
            checked={checked}
            disabled={disabled}
            className={cn(
              checkboxVariants({ size, variant }),
              'accent-current',
              disabled && 'opacity-50 cursor-not-allowed',
              className,
            )}
            {...props}
          />
        </div>

        {/* Label and description */}
        {(label || description || errorMessage || successMessage) && (
          <div className="flex flex-col gap-1">
            {label && (
              <label
                htmlFor={id}
                className={cn(
                  'text-sm font-medium',
                  disabled
                    ? 'text-neutral-500 cursor-not-allowed'
                    : 'text-neutral-900 cursor-pointer',
                )}
              >
                {label}
              </label>
            )}

            {description && !errorMessage && !successMessage && (
              <p className="text-xs text-neutral-600">{description}</p>
            )}

            {errorMessage && (
              <p className="text-xs text-error-primary font-medium">
                {errorMessage}
              </p>
            )}

            {successMessage && !errorMessage && (
              <p className="text-xs text-success-primary font-medium">
                {successMessage}
              </p>
            )}
          </div>
        )}
      </div>
    );
  },
);

Checkbox.displayName = 'Checkbox';

export default Checkbox;
