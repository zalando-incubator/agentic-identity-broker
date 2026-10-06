import { useEffect, useRef } from 'react';
import { Button } from '@design-system/components/primitives/Button';
import { commonCopy } from '@copy';
import { consentCopy } from '@copy/consent';

export function GrantEditBar({ pending, blocked, onCancel }: { pending: boolean; blocked: boolean; onCancel: () => void }) {
  const bar = useRef<HTMLDivElement>(null);
  useEffect(() => {
    const root = document.documentElement;
    const previousPadding = root.style.scrollPaddingTop;
    const element = bar.current;
    if (!element) return;
    function keepFocusVisible() {
      const focused = document.activeElement;
      if (!(focused instanceof HTMLElement) || element!.contains(focused)) return;
      const bounds = focused.getBoundingClientRect();
      const barBounds = element!.getBoundingClientRect();
      if (bounds.bottom > barBounds.top && bounds.top < barBounds.bottom) focused.scrollIntoView({ block: 'center', behavior: 'instant' });
    }
    function measure() {
      root.style.scrollPaddingTop = `${element!.getBoundingClientRect().height + 16}px`;
      keepFocusVisible();
    }
    measure();
    const observer = typeof ResizeObserver === 'undefined' ? undefined : new ResizeObserver(measure);
    observer?.observe(element);
    window.addEventListener('resize', measure);
    document.addEventListener('focusin', keepFocusVisible);
    return () => {
      root.style.scrollPaddingTop = previousPadding;
      observer?.disconnect();
      window.removeEventListener('resize', measure);
      document.removeEventListener('focusin', keepFocusVisible);
    };
  }, []);
  return <div ref={bar} data-testid="grant-save-bar" className="sticky top-2 z-10 flex flex-wrap items-center justify-end gap-3 rounded-xl border border-border bg-popover p-3 shadow-focal">
    <div role="status" className="mr-auto"><p className="text-sm font-semibold text-foreground">{consentCopy.unsavedChanges}</p><p className="text-xs text-muted-foreground">{consentCopy.saveBeforeLeaving}</p></div>
    <Button variant="secondary" disabled={pending} onClick={onCancel}>{commonCopy.cancel}</Button>
    <Button variant="primary" type="submit" disabled={pending || blocked} isLoading={pending}>{commonCopy.save}</Button>
  </div>;
}
