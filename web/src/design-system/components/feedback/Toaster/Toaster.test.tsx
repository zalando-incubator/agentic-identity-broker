import { act, cleanup, render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { afterEach, expect, it, vi } from 'vitest';
import { toast } from 'sonner';
import { ThemeProvider, useTheme } from '@design-system/theme/ThemeProvider';
import { Toaster } from './Toaster';

afterEach(() => { cleanup(); toast.dismiss(); localStorage.clear(); vi.unstubAllGlobals(); });

it('announces outcomes politely and follows an explicit theme instead of the OS theme', async () => {
  vi.stubGlobal('matchMedia', vi.fn(() => ({ matches: false, addEventListener: vi.fn(), removeEventListener: vi.fn() })));
  localStorage.setItem('aib.theme', 'dark');
  function Controls() {
    const { setTheme } = useTheme();
    return <button onClick={() => setTheme('light')}>Light appearance</button>;
  }
  const props = { label: 'Notifications', closeButtonLabel: 'Dismiss notification' };
  render(<ThemeProvider><Controls /><Toaster {...props} /></ThemeProvider>);
  act(() => { toast.success('Access revoked'); });
  const message = await screen.findByText('Access revoked');
  expect(message.closest('[data-sonner-toaster]')).toHaveAttribute('data-sonner-theme', 'dark');
  expect(screen.getByRole('region', { name: 'Notifications' })).toHaveAttribute('aria-live', 'polite');
  await userEvent.click(screen.getByRole('button', { name: 'Light appearance' }));
  expect(message.closest('[data-sonner-toaster]')).toHaveAttribute('data-sonner-theme', 'light');
});
