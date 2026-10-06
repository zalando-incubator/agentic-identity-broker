import { useState } from 'react';
import { cn } from '@design-system/utils/cn';
import { Input, type InputProps } from '../Input/Input';

export interface DatePickerProps extends Omit<InputProps, 'type' | 'value' | 'defaultValue' | 'onChange'> {
  /** Native calendar date, YYYY-MM-DD, or an empty string. No timezone conversion. */
  value: string;
  /** Commits valid dates only; an optional empty field commits an empty string. */
  onValueChange: (value: string) => void;
  /** Caller-provided message for native required, min, max, and step violations. */
  validationMessage: string;
}

export function DatePicker({
  value,
  onValueChange,
  validationMessage,
  error,
  className,
  onBlur,
  onInvalid,
  ...props
}: DatePickerProps) {
  const [draft, setDraft] = useState({ committed: value, value, invalid: false });
  let current = draft;

  // A caller reset replaces the local invalid draft before it is painted.
  if (draft.committed !== value) {
    current = { committed: value, value, invalid: false };
    setDraft(current);
  }

  return (
    // The native calendar button can match :focus-within without :focus-visible.
    <Input
      {...props}
      className={cn(
        'block focus-within:ring-2 focus-within:ring-ring focus-within:ring-offset-2 focus-within:ring-offset-background',
        className,
      )}
      type="date"
      value={current.value}
      error={error || (current.invalid ? validationMessage : undefined)}
      onChange={(event) => {
        const input = event.currentTarget;
        const next = input.value;
        const invalid = !input.validity.valid;
        setDraft({ committed: value, value: next, invalid });
        if (!invalid) onValueChange(next);
      }}
      onBlur={(event) => {
        const input = event.currentTarget;
        setDraft({ committed: value, value: input.value, invalid: !input.validity.valid });
        onBlur?.(event);
      }}
      onInvalid={(event) => {
        // Inline validation replaces the browser's transient validation bubble.
        event.preventDefault();
        setDraft({ committed: value, value: event.currentTarget.value, invalid: true });
        onInvalid?.(event);
      }}
    />
  );
}
