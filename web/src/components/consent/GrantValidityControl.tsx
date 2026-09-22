/**
 * GrantValidityControl component - Controls grant expiration settings.
 *
 * Features:
 * - Checkbox to enable/disable expiration
 * - DatePicker for selecting expiration date
 * - Suggested date shortcuts (1 month, 3 months, 1 year)
 * - Current date display
 * - Validation
 *
 * Refactored to use design system primitives:
 * - Stack for layout
 * - Button for suggested date shortcuts
 * - Checkbox from design system
 * - DatePicker from design system
 */

import React, { useMemo } from 'react';
import { addMonths, addYears, format } from 'date-fns';
import { Stack } from '../../design-system/components/layout/Stack';
import { Button } from '../../design-system/components/primitives/Button';
import { Checkbox } from '../../design-system/components/inputs/Checkbox';
import { DatePicker } from '../../design-system/components/inputs/DatePicker';
import type { GrantValidityState } from '../../types/consent';

interface GrantValidityControlProps {
  /** Current validity state */
  value: GrantValidityState;
  /** Callback when validity state changes */
  onChange: (state: GrantValidityState) => void;
  /** Additional CSS classes */
  className?: string;
}

/**
 * GrantValidityControl allows users to set grant expiration date.
 * Provides checkbox to enable expiration and date picker with suggested dates.
 */
export function GrantValidityControl({
  value,
  onChange,
  className = '',
}: GrantValidityControlProps) {
  const today = useMemo(() => new Date(), []);
  const tomorrow = useMemo(() => {
    const date = new Date();
    date.setDate(date.getDate() + 1);
    return date;
  }, []);

  // Suggested dates
  const suggestedDates = useMemo(
    () => [
      { label: '1 month', date: addMonths(today, 1) },
      { label: '3 months', date: addMonths(today, 3) },
      { label: '1 year', date: addYears(today, 1) },
    ],
    [today],
  );

  // Handle checkbox toggle
  const handleCheckboxChange = (e: React.ChangeEvent<HTMLInputElement>) => {
    const checked = e.target.checked;
    onChange({
      noExpiration: !checked,
      expiresAt: checked ? addMonths(today, 3) : undefined,
    });
  };

  // Handle date change
  const handleDateChange = (date: Date | null) => {
    onChange({
      noExpiration: false,
      expiresAt: date || undefined,
    });
  };

  // Handle suggested date click
  const handleSuggestedDateClick = (date: Date) => {
    onChange({
      noExpiration: false,
      expiresAt: date,
    });
  };

  const hasExpiration = !value.noExpiration;

  return (
    <Stack gap="md" className={className}>
      {/* Current date info */}
      <p className="text-sm text-neutral-600">
        Today:{' '}
        <span className="font-medium">{format(today, 'MMMM d, yyyy')}</span>
      </p>

      {/* Expiration checkbox */}
      <Checkbox
        id="grant-expires"
        checked={hasExpiration}
        onChange={handleCheckboxChange}
        label="Specific end date"
      />

      {/* Date picker (only shown when checkbox is checked) */}
      {hasExpiration && (
        <Stack gap="sm" className="pl-7">
          <DatePicker
            value={value.expiresAt || null}
            onChange={handleDateChange}
            minDate={tomorrow}
            label="End date"
            errorMessage={
              value.expiresAt && value.expiresAt <= today
                ? 'End date must be in the future'
                : undefined
            }
          />

          {/* Suggested dates */}
          <Stack gap="xs">
            <p className="text-xs text-neutral-600">Quick suggestions:</p>
            <Stack direction="row" gap="sm" wrap>
              {suggestedDates.map((suggestion) => (
                <Button
                  key={suggestion.label}
                  type="button"
                  variant="ghost"
                  size="sm"
                  onClick={() => handleSuggestedDateClick(suggestion.date)}
                  className="bg-neutral-100 text-neutral-700 hover:bg-neutral-200"
                >
                  {suggestion.label}
                  <span className="ml-1.5 text-neutral-500">
                    ({format(suggestion.date, 'MMM d, yyyy')})
                  </span>
                </Button>
              ))}
            </Stack>
          </Stack>
        </Stack>
      )}

      {/* No expiration message */}
      {!hasExpiration && (
        <p className="pl-7 text-sm text-neutral-600">
          After saving, the agent can use your permissions until revoked
        </p>
      )}
    </Stack>
  );
}

export default GrantValidityControl;
