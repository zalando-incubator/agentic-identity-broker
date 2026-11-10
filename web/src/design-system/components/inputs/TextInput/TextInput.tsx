/**
 * TextInput Component
 *
 * Flexible text input with validation, icons, and helper text.
 * Follows the "Refined Trust Architecture" design system.
 *
 * Features:
 * - 3 sizes: sm, md, lg
 * - Multiple variants: default, error, success
 * - Icon support (leading/trailing)
 * - Helper text and error messages
 * - Character count
 * - Label support with required indicator
 * - Full keyboard accessibility
 * - WCAG 2.1 AA compliant
 */

import React from 'react';
import { cva, type VariantProps } from 'class-variance-authority';
import { cn } from '@design-system/utils';
import { CircleCheck, Info } from 'lucide-react';

const inputVariants = cva(
  'w-full px-3 py-2 rounded-md border-1.5 transition-colors font-sans',
  {
    variants: {
      size: {
        sm: 'px-2.5 py-1.5 text-sm h-9',
        md: 'px-4 py-3 text-base h-11',
        lg: 'px-4 py-3 text-lg h-12',
      },
      variant: {
        default:
          'border-neutral-300 text-neutral-900 placeholder-neutral-500 focus:border-trust-deep focus:ring-1 focus:ring-trust',
        error:
          'border-error-primary bg-error-light/20 text-neutral-900 placeholder-neutral-500 focus:border-error-primary focus:ring-1 focus:ring-error-primary',
        success:
          'border-success-primary bg-success-light/20 text-neutral-900 placeholder-neutral-500 focus:border-success-primary focus:ring-1 focus:ring-success-primary',
      },
    },
    compoundVariants: [
      {
        size: 'sm',
        className: 'text-sm',
      },
      {
        size: 'lg',
        className: 'text-lg',
      },
    ],
    defaultVariants: {
      size: 'md',
      variant: 'default',
    },
  },
);

export interface TextInputProps
  extends
    Omit<React.InputHTMLAttributes<HTMLInputElement>, 'size'>,
    VariantProps<typeof inputVariants> {
  /** Label text displayed above input */
  label?: string;
  /** Show required indicator on label */
  required?: boolean;
  /** Helper text displayed below input */
  helperText?: string;
  /** Error message (changes variant to error) */
  errorMessage?: string;
  /** Success message */
  successMessage?: string;
  /** Icon element to display on the left */
  iconBefore?: React.ReactNode;
  /** Icon element to display on the right */
  iconAfter?: React.ReactNode;
  /** Show character count */
  showCharCount?: boolean;
  /** Maximum character count for display */
  maxLength?: number;
  /** Unique identifier for form association */
  id?: string;
  /** Callback when field state changes */
  onChange?: (e: React.ChangeEvent<HTMLInputElement>) => void;
}

/**
 * TextInput component for single-line text entry.
 * Supports labels, validation states, icons, and helper text.
 *
 * @example
 * ```tsx
 * <TextInput
 *   label="Email"
 *   type="email"
 *   placeholder="you@example.com"
 *   helperText="We'll never share your email"
 * />
 *
 * <TextInput
 *   label="Password"
 *   type="password"
 *   errorMessage="Password must be at least 8 characters"
 *   variant="error"
 * />
 *
 * <TextInput
 *   label="Search"
 *   iconBefore={<SearchIcon />}
 *   placeholder="Search..."
 * />
 *
 * <TextInput
 *   label="Bio"
 *   maxLength={100}
 *   showCharCount
 *   helperText="Tell us about yourself"
 * />
 * ```
 */
export const TextInput = React.forwardRef<HTMLDivElement, TextInputProps>(
  (
    {
      size = 'md',
      label,
      required = false,
      helperText,
      errorMessage,
      successMessage,
      iconBefore,
      iconAfter,
      showCharCount = false,
      className,
      id,
      value = '',
      maxLength,
      disabled = false,
      onChange,
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

    // Character count
    const charCount = typeof value === 'string' ? value.length : 0;
    const displayCharCount =
      showCharCount && maxLength ? `${charCount}/${maxLength}` : null;

    // Icon sizing based on input size
    const iconSize =
      size === 'sm' ? 'w-4 h-4' : size === 'lg' ? 'w-5 h-5' : 'w-4.5 h-4.5';

    return (
      <div ref={ref} className="w-full">
        {/* Label */}
        {label && (
          <label
            htmlFor={id}
            className="block text-sm font-medium text-neutral-900 mb-2"
          >
            {label}
            {required && <span className="ml-1 text-error-primary">*</span>}
          </label>
        )}

        {/* Input container with icons */}
        <div className="relative flex items-center">
          {/* Icon before */}
          {iconBefore && (
            <span
              className={cn(
                'absolute left-3 text-neutral-500 pointer-events-none flex items-center',
                iconSize,
              )}
              aria-hidden="true"
            >
              {iconBefore}
            </span>
          )}

          {/* Input field */}
          <input
            id={id}
            value={value}
            maxLength={maxLength}
            disabled={disabled}
            onChange={onChange}
            className={cn(
              inputVariants({ size, variant }),
              iconBefore && 'pl-10',
              iconAfter && 'pr-10',
              disabled && 'bg-neutral-50 cursor-not-allowed opacity-60',
              className,
            )}
            {...props}
          />

          {/* Icon after */}
          {iconAfter && (
            <span
              className={cn(
                'absolute right-3 text-neutral-500 pointer-events-none flex items-center',
                iconSize,
              )}
              aria-hidden="true"
            >
              {iconAfter}
            </span>
          )}

          {/* Status icon (error/success) */}
          {(errorMessage || successMessage) && !iconAfter && (
            <span
              className={cn(
                'absolute right-3 flex items-center',
                errorMessage ? 'text-error-primary' : 'text-success-primary',
                iconSize,
              )}
              aria-hidden="true"
            >
              {errorMessage ? (
                <Info aria-hidden="true" strokeWidth={1.5} />
              ) : (
                <CircleCheck aria-hidden="true" strokeWidth={1.5} />
              )}
            </span>
          )}
        </div>

        {/* Helper text, error message, or character count */}
        <div className="mt-1.5 flex items-center justify-between">
          {errorMessage && (
            <span className="text-xs text-error-primary font-medium">
              {errorMessage}
            </span>
          )}
          {successMessage && !errorMessage && (
            <span className="text-xs text-success-primary font-medium">
              {successMessage}
            </span>
          )}
          {helperText && !errorMessage && !successMessage && (
            <span className="text-xs text-neutral-600">{helperText}</span>
          )}

          {/* Character count on right */}
          {displayCharCount && (
            <span
              className={cn(
                'text-xs ml-auto',
                charCount > maxLength! * 0.8
                  ? 'text-warning-primary'
                  : 'text-neutral-500',
              )}
            >
              {displayCharCount}
            </span>
          )}
        </div>
      </div>
    );
  },
);

TextInput.displayName = 'TextInput';

export default TextInput;
