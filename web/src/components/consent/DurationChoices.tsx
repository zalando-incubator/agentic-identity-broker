import { useEffect, useRef } from 'react';
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@design-system/components/inputs/Select';
import { durationOptions } from './durationOptions';
import type { DurationSelectProps } from './DurationSelect';
import type { GrantDuration } from './consentDraft';

interface DurationChoicesProps extends DurationSelectProps {
  id: string;
  open: boolean;
  active: boolean;
  onOpenChange: (open: boolean) => void;
  onCustom: (open: boolean) => void;
  onReady: () => void;
}

export default function DurationChoices({ id, draft, disabled, error, onChange, open, active, onOpenChange, onCustom, onReady }: DurationChoicesProps) {
  const openDateOnClose = useRef(false);
  useEffect(() => { onReady(); }, [onReady]);
  if (!active) return null;

  return <Select value={draft.duration} disabled={disabled} open={open} onOpenChange={onOpenChange} onValueChange={(value) => {
    onChange(draft.setDuration(value as GrantDuration));
    openDateOnClose.current = value === 'custom';
    if (value !== 'custom') onCustom(false);
  }}>
    <SelectTrigger id={id} size="sm" className="w-44 min-w-0 shrink" aria-invalid={Boolean(error)} aria-describedby={error ? `${id}-error` : undefined}>
      <SelectValue />
    </SelectTrigger>
    <SelectContent onCloseAutoFocus={(event) => {
      if (openDateOnClose.current) {
        event.preventDefault();
        openDateOnClose.current = false;
        onCustom(true);
      }
    }}>
      {durationOptions.map(({ value, label }) => <SelectItem key={value} value={value}>{label}</SelectItem>)}
    </SelectContent>
  </Select>;
}
