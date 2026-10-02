export type ThemePreference = 'light' | 'dark' | 'system';
export type ResolvedTheme = 'light' | 'dark';

export const THEME_STORAGE_KEY = 'aib.theme';
export const SIDEBAR_STORAGE_KEY = 'aib.sidebar-collapsed';

let inMemoryPreference: ThemePreference = 'system';

export function parseThemePreference(value: unknown): ThemePreference {
  return value === 'light' || value === 'dark' || value === 'system' ? value : 'system';
}

export function parseSidebarCollapsed(value: unknown): boolean {
  return value === 'true';
}

export function resolveTheme(preference: ThemePreference, systemDark: boolean): ResolvedTheme {
  return preference === 'system' ? (systemDark ? 'dark' : 'light') : preference;
}

export function readThemePreference(): ThemePreference {
  try {
    inMemoryPreference = parseThemePreference(window.localStorage.getItem(THEME_STORAGE_KEY));
  } catch {
    // The last choice remains usable when browser storage is blocked.
  }
  return inMemoryPreference;
}

export function writeThemePreference(preference: ThemePreference): void {
  inMemoryPreference = preference;
  try {
    window.localStorage.setItem(THEME_STORAGE_KEY, preference);
  } catch {
    // Appearance changes must not depend on storage availability.
  }
}
