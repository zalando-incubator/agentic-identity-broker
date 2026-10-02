import { useEffect, useId, useRef } from 'react';
import { RadioGroup, RadioGroupItem } from '@design-system/components/inputs/RadioGroup';
import { DatePicker } from '@design-system/components/inputs/DatePicker';
import { accessCopy } from '@copy';
import { consentCopy } from '@copy/consent';
import type { ConsentDraft, GrantDuration } from './consentDraft';

const durations: { value: GrantDuration; label: string }[] = [
  { value: 'until-revoked', label: accessCopy.untilRevoked },
  { value: '30-days', label: accessCopy.thirtyDays },
  { value: 'custom', label: accessCopy.customDate },
];

export function DurationChoice({ draft, onChange, disabled, error }: { draft: ConsentDraft; onChange: (draft: ConsentDraft) => void; disabled?: boolean; error?: string }) {
  const id = useId();
  const dateInput = useRef<HTMLInputElement>(null);
  useEffect(() => { if (error) dateInput.current?.focus(); }, [error]);
  return <fieldset disabled={disabled} className="space-y-3">
    <legend className="mb-3 font-display text-lg font-semibold">{consentCopy.accessDuration}</legend>
    <RadioGroup aria-label={consentCopy.accessDuration} value={draft.duration} disabled={disabled} onValueChange={(value) => onChange(draft.setDuration(value as GrantDuration))}>
      {durations.map(({ value, label }) => <div key={value} className="flex items-center gap-3">
        <RadioGroupItem id={`${id}-${value}`} value={value} aria-labelledby={`${id}-${value}-label`} />
        <label id={`${id}-${value}-label`} htmlFor={`${id}-${value}`} className="text-sm">{label}</label>
      </div>)}
    </RadioGroup>
    {draft.duration === 'custom' && <DatePicker ref={dateInput} label={accessCopy.customDate} value={draft.customDate} aria-required="true" disabled={disabled} validationMessage={consentCopy.invalidDate} error={error} onValueChange={(value) => onChange(draft.setDuration('custom', value))} />}
  </fieldset>;
}
