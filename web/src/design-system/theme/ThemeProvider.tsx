import {
  createContext,
  useCallback,
  useContext,
  useLayoutEffect,
  useMemo,
  useState,
  useSyncExternalStore,
  type ReactNode,
} from 'react';
import {
  readThemePreference,
  resolveTheme,
  writeThemePreference,
  type ResolvedTheme,
  type ThemePreference,
} from './themePreference';

export interface ThemeContextValue {
  preference: ThemePreference;
  resolvedTheme: ResolvedTheme;
  setTheme: (preference: ThemePreference) => void;
}

const ThemeContext = createContext<ThemeContextValue | null>(null);

export function ThemeProvider({ children }: { children: ReactNode }) {
  const [preference, setPreference] = useState(readThemePreference);
  const media = useMemo(() => window.matchMedia('(prefers-color-scheme: dark)'), []);
  const subscribe = useCallback((onChange: () => void) => {
    if (preference !== 'system') return () => {};
    media.addEventListener('change', onChange);
    return () => media.removeEventListener('change', onChange);
  }, [media, preference]);
  const getSystemDark = useCallback(() => media.matches, [media]);
  const systemDark = useSyncExternalStore(subscribe, getSystemDark);
  const resolvedTheme = resolveTheme(preference, systemDark);

  useLayoutEffect(() => {
    document.documentElement.dataset.theme = resolvedTheme;
    document.documentElement.style.colorScheme = resolvedTheme;
  }, [resolvedTheme]);

  const setTheme = useCallback((nextPreference: ThemePreference) => {
    writeThemePreference(nextPreference);
    setPreference(nextPreference);
  }, []);
  const value = useMemo(
    () => ({ preference, resolvedTheme, setTheme }),
    [preference, resolvedTheme, setTheme],
  );

  return <ThemeContext.Provider value={value}>{children}</ThemeContext.Provider>;
}

export function useTheme(): ThemeContextValue {
  const theme = useContext(ThemeContext);
  if (!theme) throw new Error('useTheme must be used within ThemeProvider');
  return theme;
}
