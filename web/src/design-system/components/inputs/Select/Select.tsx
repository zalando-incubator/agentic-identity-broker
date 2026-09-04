/**
 * Select Component
 *
 * Accessible dropdown select using Headless UI Listbox.
 * Follows the "Refined Trust Architecture" design system.
 *
 * Features:
 * - Single and multi-select variants
 * - 3 sizes: sm, md, lg
 * - Search/filter support
 * - Custom option rendering
 * - Error/success validation states
 * - Full keyboard accessibility (arrow keys, Tab, Enter)
 * - WCAG 2.1 AA compliant
 */

import React, { useState, useMemo } from 'react';
import { Listbox, Transition } from '@headlessui/react';
import { cva } from 'class-variance-authority';
import { cn } from '@design-system/utils';

export interface SelectOption {
  value: string | number;
  label: string;
  disabled?: boolean;
}

export interface SelectOptionGroup {
  label: string;
  options: SelectOption[];
}

const selectButtonVariants = cva(
  'relative w-full text-left bg-white border-1.5 rounded-md transition-all focus:outline-none focus-visible:ring-2 focus-visible:ring-offset-2 focus-visible:ring-trust-deep disabled:opacity-50 disabled:cursor-not-allowed flex items-center justify-between px-4 py-3',
  {
    variants: {
      size: {
        sm: 'text-sm py-2 px-3',
        md: 'text-base py-3 px-4',
        lg: 'text-lg py-3.5 px-4.5',
      },
      variant: {
        default: 'border-neutral-300 hover:border-neutral-400',
        error: 'border-error-primary bg-error-light/20',
        success: 'border-success-primary bg-success-light/20',
      },
      open: {
        true: 'rounded-b-none',
        false: '',
      },
    },
    defaultVariants: {
      size: 'md',
      variant: 'default',
      open: false,
    },
  },
);

const selectOptionsVariants = cva(
  'absolute top-full left-0 right-0 z-50 w-full bg-white border-1.5 border-t-0 border-neutral-300 rounded-b-lg shadow-lg focus:outline-none max-h-60 overflow-y-auto',
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

const selectOptionVariants = cva(
  'relative cursor-pointer select-none py-2 px-3 flex items-center justify-between transition-colors',
  {
    variants: {
      selected: {
        true: 'bg-trust-light text-trust-deep font-medium',
        false: 'text-neutral-900 hover:bg-neutral-50',
      },
      disabled: {
        true: 'opacity-50 cursor-not-allowed',
        false: '',
      },
    },
    defaultVariants: {
      selected: false,
      disabled: false,
    },
  },
);

export interface SelectProps extends Omit<
  React.HTMLAttributes<HTMLDivElement>,
  'onChange'
> {
  /** Array of options or grouped options */
  options: (SelectOption | SelectOptionGroup)[];
  /** Selected value(s) */
  value: string | number | (string | number)[] | null;
  /** Callback when value changes */
  onChange: (value: string | number | (string | number)[] | null) => void;
  /** Enable multi-select mode */
  multiselect?: boolean;
  /** Label text */
  label?: string;
  /** Helper text */
  helperText?: string;
  /** Error message */
  errorMessage?: string;
  /** Success message */
  successMessage?: string;
  /** Placeholder text */
  placeholder?: string;
  /** Disable the select */
  disabled?: boolean;
  /** Enable search/filter */
  searchable?: boolean;
  /** Required field indicator */
  required?: boolean;
  /** Component size */
  size?: 'sm' | 'md' | 'lg';
  /** Custom option renderer */
  renderOption?: (option: SelectOption, isSelected: boolean) => React.ReactNode;
  /** Custom label renderer */
  renderLabel?: (
    value: string | number | (string | number)[] | null,
  ) => React.ReactNode;
  /** Unique identifier */
  id?: string;
}

/**
 * Select component for choosing from a list of options.
 * Supports single and multi-select with search capability.
 *
 * @example
 * ```tsx
 * <Select
 *   options={[
 *     { value: 'option1', label: 'Option 1' },
 *     { value: 'option2', label: 'Option 2' },
 *   ]}
 *   value={selected}
 *   onChange={setSelected}
 *   label="Choose an option"
 *   placeholder="Select one..."
 * />
 * ```
 */
export const Select = React.forwardRef<HTMLDivElement, SelectProps>(
  (
    {
      options,
      value,
      onChange,
      multiselect = false,
      label,
      helperText,
      errorMessage,
      successMessage,
      placeholder = 'Select...',
      disabled = false,
      searchable = false,
      required = false,
      size = 'md',
      renderOption,
      renderLabel,
      id,
      className,
      'aria-label': ariaLabel,
      ...props
    },
    ref,
  ) => {
    const [isOpen, setIsOpen] = useState(false);
    const [searchTerm, setSearchTerm] = useState('');

    // Flatten options to handle both grouped and flat options
    const flatOptions = useMemo<SelectOption[]>(() => {
      return options.flatMap((opt) =>
        'options' in opt ? opt.options : [opt],
      );
    }, [options]);

    // Filter options based on search term
    const filteredOptions = useMemo(() => {
      if (!searchTerm) return flatOptions;
      return flatOptions.filter((opt) =>
        opt.label.toLowerCase().includes(searchTerm.toLowerCase()),
      );
    }, [flatOptions, searchTerm]);

    // Get display value
    const displayValue = useMemo(() => {
      if (renderLabel) return renderLabel(value);

      if (multiselect && Array.isArray(value)) {
        if (value.length === 0) return placeholder;
        if (value.length === 1) {
          const opt = flatOptions.find((o) => o.value === value[0]);
          return opt?.label || placeholder;
        }
        return `${value.length} selected`;
      }

      if (value) {
        const opt = flatOptions.find((o) => o.value === value);
        return opt?.label || placeholder;
      }

      return placeholder;
    }, [value, flatOptions, multiselect, placeholder, renderLabel]);

    // Determine variant based on error/success state
    const variant = errorMessage
      ? 'error'
      : successMessage
        ? 'success'
        : 'default';

    // Handle option selection
    const handleSelect = (
      optionValue: string | number | (string | number)[],
    ) => {
      if (multiselect) {
        // In multiple mode, Headless UI passes the entire updated array
        if (Array.isArray(optionValue)) {
          onChange(optionValue);
        } else {
          // Fallback for single values (shouldn't normally happen in multiple mode)
          const currentValue = Array.isArray(value) ? value : [];
          if (currentValue.includes(optionValue)) {
            onChange(currentValue.filter((v) => v !== optionValue));
          } else {
            onChange([...currentValue, optionValue]);
          }
        }
      } else {
        onChange(optionValue as string | number);
        setIsOpen(false);
        setSearchTerm('');
      }
    };

    return (
      <div
        ref={ref}
        className={cn('flex flex-col gap-1.5', className)}
        {...props}
      >
        {/* Label */}
        {label && (
          <label
            htmlFor={id}
            className={cn(
              'text-sm font-medium',
              disabled ? 'text-neutral-500' : 'text-neutral-900',
              required &&
                "after:content-['*'] after:ml-1 after:text-error-primary",
            )}
          >
            {label}
          </label>
        )}

        {/* Select button */}
        <div className="relative w-full">
          <Listbox
            value={value ?? undefined}
            onChange={handleSelect}
            disabled={disabled}
            multiple={multiselect}
            as="div"
            className="relative"
          >
            <Listbox.Button
              id={id}
              aria-label={ariaLabel}
              className={selectButtonVariants({ size, variant, open: isOpen })}
              onClick={() => setIsOpen(!isOpen)}
              onFocus={() => searchable && setIsOpen(true)}
            >
              <span className="block truncate">{displayValue}</span>
              <svg
                className={cn(
                  'w-5 h-5 transition-transform flex-shrink-0',
                  isOpen && 'rotate-180',
                )}
                fill="none"
                viewBox="0 0 20 20"
                stroke="currentColor"
                aria-hidden="true"
              >
                <path
                  strokeLinecap="round"
                  strokeLinejoin="round"
                  strokeWidth={1.5}
                  d="M7 8l3 3 3-3m-3 3V4"
                />
              </svg>
            </Listbox.Button>

            {/* Options dropdown */}
            <div className="relative">
              <Transition
                show={isOpen && !disabled}
                enter="transition ease-out duration-100"
                enterFrom="transform opacity-0 scale-95"
                enterTo="transform opacity-100 scale-100"
                leave="transition ease-in duration-75"
                leaveFrom="transform opacity-100 scale-100"
                leaveTo="transform opacity-0 scale-95"
              >
                <Listbox.Options
                  className={selectOptionsVariants({ size })}
                  static
                >
                  {/* Search input */}
                  {searchable && (
                    <div className="sticky top-0 bg-white border-b border-neutral-200 p-2">
                      <input
                        type="text"
                        placeholder="Search..."
                        value={searchTerm}
                        onChange={(e) => setSearchTerm(e.target.value)}
                        className="w-full px-2 py-1 text-sm border border-neutral-300 rounded focus:outline-none focus:border-trust-deep"
                        onClick={(e) => e.stopPropagation()}
                      />
                    </div>
                  )}

                  {/* Options list */}
                  {filteredOptions.length > 0 ? (
                    filteredOptions.map((option) => {
                      const isSelected = multiselect
                        ? Array.isArray(value) && value.includes(option.value)
                        : value === option.value;

                      return (
                        <Listbox.Option
                          key={option.value}
                          value={option.value}
                          disabled={option.disabled}
                          as="div"
                          className={selectOptionVariants({
                            selected: isSelected,
                            disabled: option.disabled,
                          })}
                        >
                          {renderOption ? (
                            renderOption(option, isSelected)
                          ) : (
                            <>
                              <span>{option.label}</span>
                              {isSelected && (
                                <svg
                                  className="w-5 h-5 flex-shrink-0"
                                  fill="currentColor"
                                  viewBox="0 0 20 20"
                                  aria-hidden="true"
                                >
                                  <path
                                    fillRule="evenodd"
                                    d="M16.707 5.293a1 1 0 010 1.414l-8 8a1 1 0 01-1.414 0l-4-4a1 1 0 011.414-1.414L8 12.586l7.293-7.293a1 1 0 011.414 0z"
                                    clipRule="evenodd"
                                  />
                                </svg>
                              )}
                            </>
                          )}
                        </Listbox.Option>
                      );
                    })
                  ) : (
                    <div className="px-3 py-2 text-sm text-neutral-500">
                      No options found
                    </div>
                  )}
                </Listbox.Options>
              </Transition>
            </div>

            {/* Close options when clicking outside */}
            {isOpen && (
              <button
                type="button"
                className="fixed inset-0 z-40"
                aria-label="Close options"
                onClick={() => {
                  setIsOpen(false);
                  setSearchTerm('');
                }}
              />
            )}
          </Listbox>
        </div>

        {/* Helper text / Error / Success message */}
        <div className="flex items-center gap-1.5 min-h-5">
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
          {helperText && !errorMessage && !successMessage && (
            <p className="text-xs text-neutral-600">{helperText}</p>
          )}
        </div>
      </div>
    );
  },
);

Select.displayName = 'Select';

export default Select;
