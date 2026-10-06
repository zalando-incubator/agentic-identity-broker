import { lazy, Suspense, useCallback, useEffect, useId, useRef, useState } from 'react';
import { isValid, parseISO } from 'date-fns';
import { CalendarDays, ChevronDown } from 'lucide-react';
import { Button } from '@design-system/components/primitives/Button';
import { consentCopy } from '@copy';
import type { ConsentDraft } from './consentDraft';
import { durationOptions } from './durationOptions';

const DurationChoices = lazy(() => import('./DurationChoices'));
const CustomDatePopover = lazy(() => import('./CustomDatePopover'));
const dateFormatter = new Intl.DateTimeFormat(undefined, { day: 'numeric', month: 'short', year: 'numeric' });

export interface DurationSelectProps {
  draft: ConsentDraft;
  disabled?: boolean;
  error?: string;
  onChange: (draft: ConsentDraft) => void;
}

export function DurationSelect({ draft, disabled, error, onChange }: DurationSelectProps) {
  const id = useId();
  const dateTrigger = useRef<HTMLButtonElement>(null);
  const [selectRequested, setSelectRequested] = useState(false);
  const [selectReady, setSelectReady] = useState(false);
  const [selectOpen, setSelectOpen] = useState(false);
  const [dateOpen, setDateOpen] = useState(false);
  const [dateReady, setDateReady] = useState(false);
  const onSelectReady = useCallback(() => setSelectReady(true), []);
  const onDateReady = useCallback(() => setDateReady(true), []);
  const selectedDate = /^\d{4}-\d{2}-\d{2}$/.test(draft.customDate) ? parseISO(draft.customDate) : undefined;
  const customDateLabel = selectedDate && isValid(selectedDate) ? dateFormatter.format(selectedDate) : undefined;
  useEffect(() => { if (error && draft.duration === 'custom') setDateOpen(true); }, [draft.duration, error]);

  return <div className="space-y-1">
    <div className="flex min-w-0 flex-wrap items-center justify-between gap-x-3 gap-y-1">
      <label htmlFor={id} className="shrink-0 text-sm font-medium">{consentCopy.accessLasts}</label>
      <div className="flex min-w-0 flex-wrap items-center justify-end gap-1">
        {!selectReady && <Button id={id} role="combobox" type="button" size="sm" variant="outline" disabled={disabled}
          aria-label={consentCopy.accessLasts} aria-haspopup="listbox" aria-expanded={selectOpen} aria-busy={selectRequested || undefined}
          aria-invalid={Boolean(error)} aria-describedby={error ? `${id}-error` : undefined}
          className="w-44 max-w-44 min-w-0 shrink justify-between gap-2 border-border-control font-normal hover:border-ring hover:bg-background [&>span]:truncate"
          onClick={() => { setSelectRequested(true); setSelectOpen(true); }}
          onKeyDown={(event) => {
            if (event.key === 'ArrowDown' || event.key === 'ArrowUp') { event.preventDefault(); setSelectRequested(true); setSelectOpen(true); }
            if (event.key === 'Escape' && !selectReady) setSelectOpen(false);
          }}><span>{durationOptions.find(({ value }) => value === draft.duration)?.label}</span><ChevronDown aria-hidden="true" /></Button>}
        {selectRequested && <Suspense fallback={null}>
          <DurationChoices id={id} draft={draft} disabled={disabled} error={error} onChange={onChange} open={selectOpen} active={selectReady} onOpenChange={setSelectOpen} onCustom={setDateOpen} onReady={onSelectReady} />
        </Suspense>}
        {draft.duration === 'custom' && <>
          <Button ref={dateTrigger} type="button" size="sm" variant="outline" className="shrink-0 font-normal tabular-nums" disabled={disabled}
            aria-label={consentCopy.chooseCustomDate} aria-describedby={customDateLabel ? `${id}-date` : undefined} aria-haspopup="dialog" aria-expanded={dateOpen} aria-busy={dateOpen && !dateReady || undefined} aria-invalid={Boolean(error)}
            onClick={() => setDateOpen((before) => !before)}
            onKeyDown={(event) => { if (event.key === 'Escape' && !dateReady) setDateOpen(false); }}><CalendarDays aria-hidden="true" className="size-4" />{customDateLabel && <time id={`${id}-date`} dateTime={draft.customDate}>{customDateLabel}</time>}</Button>
          {(dateOpen || dateReady) && <Suspense fallback={null}>
            <CustomDatePopover draft={draft} disabled={disabled} error={error} onChange={onChange} trigger={dateTrigger} open={dateOpen} onOpenChange={setDateOpen} onReady={onDateReady} />
          </Suspense>}
        </>}
      </div>
    </div>
    {error && <p id={`${id}-error`} role="alert" className="text-xs text-status-danger-foreground">{error}</p>}
  </div>;
}
