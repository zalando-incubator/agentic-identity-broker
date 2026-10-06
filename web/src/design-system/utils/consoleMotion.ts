import { useCallback, useMemo, useSyncExternalStore } from 'react';

export const consoleMotion = {
  feedback: 0.12,
  control: 0.16,
  overlay: 0.2,
  emphasis: 0.32,
  exitFactor: 0.75,
  ease: [0.2, 0, 0, 1] as [number, number, number, number],
};

export function useConsoleReducedMotion(): boolean {
  const media = useMemo(() => window.matchMedia?.('(prefers-reduced-motion: reduce)'), []);
  const subscribe = useCallback((onChange: () => void) => {
    media?.addEventListener('change', onChange);
    return () => media?.removeEventListener('change', onChange);
  }, [media]);
  const getSnapshot = useCallback(() => media?.matches ?? false, [media]);
  return useSyncExternalStore(subscribe, getSnapshot);
}
