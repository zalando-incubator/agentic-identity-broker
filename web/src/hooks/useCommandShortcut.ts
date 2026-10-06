import { useEffect } from 'react';

/** Console-only shortcut; the caller owns lazy mounting and open state. */
export function useCommandShortcut(onToggle: () => void, enabled = true): void {
  useEffect(() => {
    if (!enabled) return;
    const onKeyDown = (event: KeyboardEvent) => {
      if (event.isComposing || event.repeat || event.altKey || event.key.toLowerCase() !== 'k' || (!event.ctrlKey && !event.metaKey)) return;
      event.preventDefault();
      onToggle();
    };
    document.addEventListener('keydown', onKeyDown);
    return () => document.removeEventListener('keydown', onKeyDown);
  }, [enabled, onToggle]);
}
