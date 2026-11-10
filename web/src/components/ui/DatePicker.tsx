/**
 * DatePicker component - Simple date input control.
 *
 * Features:
 * - HTML5 date input
 * - Parsing and formatting with date-fns
 * - Min date validation
 * - Error state display
 * - Disabled state support
 */

import React from 'react';
import { format, parse, isValid, isBefore } from 'date-fns';

interface DatePickerProps {
  /** Current date value */
  value: Date | null;
  /** Callback when date changes */
  onChange: (date: Date | null) => void;
  /** Minimum allowed date */
  minDate?: Date;
  /** Whether the input is disabled */
  disabled?: boolean;
  /** Input label */
  label?: string;
  /** Error message to display */
  error?: string;
  /** Additional CSS classes */
  className?: string;
}

/**
 * DatePicker provides a simple date input with validation.
 * Uses HTML5 date input for broad compatibility.
 */
export function DatePicker({
  value,
  onChange,
  minDate,
  disabled = false,
  label,
  error,
  className = '',
}: DatePickerProps) {
  // Format date for input (YYYY-MM-DD)
  const formatForInput = (date: Date | null): string => {
    if (!date || !isValid(date)) return '';
    return format(date, 'yyyy-MM-dd');
  };

  // Parse input value to Date
  const parseInputValue = (inputValue: string): Date | null => {
    if (!inputValue) return null;
    const parsed = parse(inputValue, 'yyyy-MM-dd', new Date());
    if (!isValid(parsed)) return null;
    return parsed;
  };

  // Handle input change
  const handleChange = (e: React.ChangeEvent<HTMLInputElement>) => {
    const inputValue = e.target.value;
    if (!inputValue) {
      onChange(null);
      return;
    }

    const parsed = parseInputValue(inputValue);
    if (!parsed) {
      // Invalid date format
      return;
    }

    // Validate against minDate
    if (minDate && isBefore(parsed, minDate)) {
      // Don't update if before min date
      return;
    }

    onChange(parsed);
  };

  const inputValue = formatForInput(value);
  const minDateValue = minDate ? formatForInput(minDate) : undefined;
  const hasError = !!error;
  const inputId = `date-picker-${Math.random().toString(36).substr(2, 9)}`;

  return (
    <div className={className}>
      {label && (
        <label
          htmlFor={inputId}
          className="block text-sm font-medium text-neutral-700 mb-2"
        >
          {label}
        </label>
      )}
      <input
        id={inputId}
        type="date"
        value={inputValue}
        onChange={handleChange}
        min={minDateValue}
        disabled={disabled}
        className={`
          block w-full rounded-md border shadow-sm
          px-3 py-2 text-sm
          focus:outline-none focus:ring-2 focus:ring-offset-0
          disabled:bg-neutral-100 disabled:text-neutral-500 disabled:cursor-not-allowed
          ${
            hasError
              ? 'border-error-primary/30 focus:border-error-primary focus:ring-error-primary'
              : 'border-neutral-300 focus:border-accent focus:ring-accent'
          }
        `}
        aria-invalid={hasError}
        aria-describedby={hasError ? 'date-error' : undefined}
      />
      {hasError && (
        <p
          id="date-error"
          className="mt-2 text-sm text-error-primary"
          role="alert"
        >
          {error}
        </p>
      )}
    </div>
  );
}

export default DatePicker;
