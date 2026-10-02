import { fireEvent, render, screen } from '@testing-library/react';
import { afterEach, beforeEach, expect, it, vi } from 'vitest';
import { ThemeProvider, useTheme } from '@design-system/theme/ThemeProvider';
import { Wordmark } from './Wordmark';

beforeEach(() => {
  localStorage.setItem('aib.theme', 'light');
  vi.stubGlobal('matchMedia', vi.fn(() => ({
    matches: false,
    media: '(prefers-color-scheme: dark)',
    onchange: null,
    addEventListener: vi.fn(),
    removeEventListener: vi.fn(),
    addListener: vi.fn(),
    removeListener: vi.fn(),
    dispatchEvent: vi.fn(),
  })));
});
afterEach(() => {
  localStorage.removeItem('aib.theme');
  delete document.documentElement.dataset.theme;
  vi.unstubAllGlobals();
});

function ThemeSwitch() {
  const { setTheme } = useTheme();
  return <button onClick={() => setTheme('dark')}>Dark appearance</button>;
}

it('preserves the accessible brand name across appearance and compact transitions', () => {
  const { rerender } = render(<ThemeProvider><ThemeSwitch /><Wordmark label="Agent Identity Broker" /></ThemeProvider>);
  const mark = screen.getByRole('img', { name: 'Agent Identity Broker' });
  expect(mark).toHaveAttribute('data-testid', 'wordmark');
  expect(mark).toHaveAttribute('data-variant', 'black');
  fireEvent.click(screen.getByRole('button', { name: 'Dark appearance' }));
  expect(mark).toHaveAttribute('data-variant', 'white');
  rerender(<ThemeProvider><Wordmark compact label="Agent Identity Broker" /></ThemeProvider>);
  expect(screen.getByRole('img', { name: 'Agent Identity Broker' })).toHaveAttribute('data-variant', 'compact');
});
