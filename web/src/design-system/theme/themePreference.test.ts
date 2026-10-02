import { createElement } from 'react';
import { act, renderHook } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { ThemeProvider, useTheme } from './ThemeProvider';
import {
  parseThemePreference,
  parseSidebarCollapsed,
  readThemePreference,
  resolveTheme,
  writeThemePreference,
  type ThemePreference,
} from './themePreference';

function systemScheme(initialDark: boolean) {
  let matches = initialDark;
  const listeners = new Set<(event: MediaQueryListEvent) => void>();
  const media = {
    get matches() { return matches; },
    media: '(prefers-color-scheme: dark)',
    addEventListener: vi.fn((_type: string, listener: (event: MediaQueryListEvent) => void) => listeners.add(listener)),
    removeEventListener: vi.fn((_type: string, listener: (event: MediaQueryListEvent) => void) => listeners.delete(listener)),
  };
  vi.stubGlobal('matchMedia', vi.fn(() => media));
  return {
    media,
    change(dark: boolean) {
      matches = dark;
      for (const listener of listeners) listener({ matches } as MediaQueryListEvent);
    },
    subscriberCount: () => listeners.size,
  };
}

beforeEach(() => { localStorage.clear(); });
afterEach(() => { vi.restoreAllMocks(); vi.unstubAllGlobals(); });

const wrapper = ({ children }: { children: React.ReactNode }) => createElement(ThemeProvider, null, children);

describe('browser appearance preferences', () => {
  it.each(['light', 'dark', 'system'] as const)('accepts the exact %s preference and persists it', (value) => {
    expect(parseThemePreference(value)).toBe(value);
    writeThemePreference(value);
    expect(localStorage.getItem('aib.theme')).toBe(value);
    expect(readThemePreference()).toBe(value);
  });

  it.each([null, '', 'Dark', ' dark ', 'false', '<script>'])('treats invalid preference %s as system', (value) => {
    expect(parseThemePreference(value)).toBe('system');
  });

  it.each([
    ['light', false, 'light'], ['light', true, 'light'],
    ['dark', false, 'dark'], ['dark', true, 'dark'],
    ['system', false, 'light'], ['system', true, 'dark'],
  ] as const)('resolves %s with OS dark=%s to %s', (preference, dark, expected) => {
    expect(resolveTheme(preference, dark)).toBe(expected);
  });

  it.each([['true', true], ['false', false], [null, false], ['', false], ['TRUE', false], ['1', false]] as const)(
    'parses sidebar value %s as %s', (value, expected) => {
      expect(parseSidebarCollapsed(value)).toBe(expected);
    },
  );

  it('keeps a changed preference in memory when storage reads and writes throw', () => {
    systemScheme(false);
    vi.spyOn(Storage.prototype, 'getItem').mockImplementation(() => { throw new DOMException('Blocked', 'SecurityError'); });
    vi.spyOn(Storage.prototype, 'setItem').mockImplementation(() => { throw new DOMException('Blocked', 'SecurityError'); });
    const { result, unmount } = renderHook(useTheme, { wrapper });
    act(() => result.current.setTheme('dark'));
    expect(result.current.preference).toBe('dark');
    expect(result.current.resolvedTheme).toBe('dark');
    expect(document.documentElement.dataset.theme).toBe('dark');
    expect(document.documentElement.style.colorScheme).toBe('dark');
    unmount();
    const remounted = renderHook(useTheme, { wrapper });
    expect(remounted.result.current.preference).toBe('dark');
    expect(remounted.result.current.resolvedTheme).toBe('dark');
  });

  it('subscribes only in system mode and follows the latest OS scheme when reselected', () => {
    localStorage.setItem('aib.theme', 'system');
    const os = systemScheme(false);
    const { result, unmount } = renderHook(useTheme, { wrapper });
    act(() => os.change(true));
    expect(result.current.resolvedTheme).toBe('dark');
    act(() => result.current.setTheme('light'));
    expect(os.subscriberCount()).toBe(0);
    act(() => os.change(false));
    act(() => os.change(true));
    expect(result.current.preference).toBe('light');
    expect(result.current.resolvedTheme).toBe('light');
    act(() => result.current.setTheme('system'));
    expect(result.current.resolvedTheme).toBe('dark');
    expect(os.subscriberCount()).toBe(1);
    act(() => os.change(false));
    expect(result.current.resolvedTheme).toBe('light');
    expect(document.documentElement.style.colorScheme).toBe('light');
    unmount();
    expect(os.subscriberCount()).toBe(0);
  });

  it.each(['light', 'dark'] satisfies ThemePreference[])('does not subscribe for stored explicit %s', (preference) => {
    localStorage.setItem('aib.theme', preference);
    const os = systemScheme(preference !== 'dark');
    const { result } = renderHook(useTheme, { wrapper });
    expect(result.current.resolvedTheme).toBe(preference);
    expect(os.subscriberCount()).toBe(0);
  });
});
