import script from './theme-init.js?raw';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { parseThemePreference, resolveTheme } from './themePreference';


afterEach(() => {
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
  localStorage.clear();
  delete document.documentElement.dataset.theme;
  document.documentElement.style.removeProperty('color-scheme');
});

describe('first-paint theme initialization', () => {
  for (const dark of [false, true]) {
    it.each(['light', 'dark', 'system', null, '', 'DARK', 'invalid'])(
      `resolves preference %s before paint with OS dark=${dark}`, (preference) => {
        if (preference === null) localStorage.removeItem('aib.theme');
        else localStorage.setItem('aib.theme', preference);
        vi.stubGlobal('matchMedia', vi.fn(() => ({ matches: dark })));
        document.documentElement.dataset.theme = dark ? 'light' : 'dark';
        document.documentElement.style.colorScheme = dark ? 'light' : 'dark';
        window.eval(script);
        const resolved = resolveTheme(parseThemePreference(preference), dark);
        expect(document.documentElement.dataset.theme).toBe(resolved);
        expect(document.documentElement.style.colorScheme).toBe(resolved);
      },
    );
    it(`uses the system scheme when accessing storage throws with OS dark=${dark}`, () => {
      vi.spyOn(window, 'localStorage', 'get').mockImplementation(() => { throw new DOMException('Blocked', 'SecurityError'); });
      vi.stubGlobal('matchMedia', vi.fn(() => ({ matches: dark })));
      window.eval(script);
      expect(document.documentElement.dataset.theme).toBe(dark ? 'dark' : 'light');
      expect(document.documentElement.style.colorScheme).toBe(dark ? 'dark' : 'light');
    });
  }
});
