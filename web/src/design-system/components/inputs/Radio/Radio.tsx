/**
 * Radio Component
 *
 * Accessible radio button with label and validation states.
 * Follows the "Refined Trust Architecture" design system.
 *
 * Features:
 * - 3 sizes: sm, md, lg
 * - Disabled state
 * - Error/success states
 * - Label with description
 * - Full keyboard accessibility (arrow keys, Tab)
 * - WCAG 2.1 AA compliant
 */

import React from 'react';
import { cva, type VariantProps } from 'class-variance-authority';
import { cn } from '@design-system/utils';

const radioVariants = cva(
  'rounded-full border-1.5 transition-colors duration-150 cursor-pointer flex-shrink-0',
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

export interface RadioProps
  extends
    Omit<React.InputHTMLAttributes<HTMLInputElement>, 'size' | 'type'>,
    VariantProps<typeof radioVariants> {
  /** Label text displayed next to radio */
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
 * Radio component for single selection from multiple options.
 * Supports labels, descriptions, and validation states.
 *
 * @example
 * ```tsx
 * <fieldset>
 *   <legend>Choose an option</legend>
 *   <Radio name="option" value="1" label="Option 1" />
 *   <Radio name="option" value="2" label="Option 2" />
 * </fieldset>
 *
 * <Radio
 *   name="duration"
 *   value="7days"
 *   label="7 days"
 *   description="Grant access for one week"
 *   checked={true}
 * />
 * ```
 */
export const Radio = React.forwardRef<HTMLInputElement, RadioProps>(
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
        {/* Radio input */}
        <div className="flex items-center h-5 pt-0.5">
          <input
            ref={ref}
            id={id}
            type="radio"
            checked={checked}
            disabled={disabled}
            className={cn(
              radioVariants({ size, variant }),
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

Radio.displayName = 'Radio';

export default Radio;
