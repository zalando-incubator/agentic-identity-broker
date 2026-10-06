import { useEffect, useRef } from 'react';
import type { RefObject } from 'react';
import { DatePicker } from '@design-system/components/inputs/DatePicker';
import { Popover, PopoverAnchor, PopoverContent } from '@design-system/components/overlays/Popover';
import { accessCopy, consentCopy } from '@copy';
import type { ConsentDraft } from './consentDraft';

interface CustomDatePopoverProps {
  draft: ConsentDraft;
  disabled?: boolean;
  error?: string;
  onChange: (draft: ConsentDraft) => void;
  trigger: RefObject<HTMLButtonElement | null>;
  open: boolean;
  onOpenChange: (open: boolean) => void;
  onReady: () => void;
}

export default function CustomDatePopover({ draft, disabled, error, onChange, trigger, open, onOpenChange, onReady }: CustomDatePopoverProps) {
  const dateInput = useRef<HTMLInputElement>(null);
  const outside = useRef(false);
  useEffect(() => { onReady(); }, [onReady]);
  useEffect(() => { if (error && open) dateInput.current?.focus(); }, [error, open]);

  return <Popover open={open} onOpenChange={onOpenChange}>
    <PopoverAnchor virtualRef={trigger} />
    <PopoverContent align="end" collisionPadding={16} aria-label={accessCopy.customDate} className="w-72"
      onOpenAutoFocus={(event) => { event.preventDefault(); dateInput.current?.focus(); }}
      onInteractOutside={(event) => {
        if (event.target instanceof Node && trigger.current?.contains(event.target)) event.preventDefault();
        else outside.current = true;
      }}
      onCloseAutoFocus={(event) => {
        event.preventDefault();
        if (!outside.current) trigger.current?.focus();
        outside.current = false;
      }}>
      <DatePicker ref={dateInput} label={accessCopy.customDate} value={draft.customDate} disabled={disabled} validationMessage={consentCopy.invalidDate} error={error} onValueChange={(value) => onChange(draft.setDuration('custom', value))} />
    </PopoverContent>
  </Popover>;
}
