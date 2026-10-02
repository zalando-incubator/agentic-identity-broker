import { cleanup, render, screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { MemoryRouter } from 'react-router-dom';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { ConsoleShell } from '@design-system/components/layout/ConsoleShell/ConsoleShell';
import { ThemeProvider } from '@design-system/theme/ThemeProvider';
import { commonCopy, navigationCopy } from '@copy';
import SettingsPage from './SettingsPage';

function renderSettings() {
  return render(
    <MemoryRouter><ThemeProvider>
      <ConsoleShell navigation={[]} search={null} userMenu={null} pendingCount={undefined} labels={{
        wordmark: commonCopy.brand,
        skipToMain: navigationCopy.skipToContent,
        navigation: navigationCopy.mainNavigation,
        collapseSidebar: navigationCopy.collapseSidebar,
        expandSidebar: navigationCopy.expandSidebar,
        openNavigation: navigationCopy.openNavigation,
        closeNavigation: navigationCopy.closeNavigation,
        navigationDescription: navigationCopy.navigationDescription,
        pendingCount: navigationCopy.pendingCount,
        pendingUnknown: navigationCopy.pendingLoading,
        pendingStale: navigationCopy.pendingStale,
      }}>
        <SettingsPage />
      </ConsoleShell>
    </ThemeProvider></MemoryRouter>,
  );
}

beforeEach(() => {
  localStorage.setItem('aib.theme', 'system');
  vi.stubGlobal('matchMedia', vi.fn(() => ({ matches: false, addEventListener: vi.fn(), removeEventListener: vi.fn() })));
});
afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  localStorage.clear();
});

describe('SettingsPage', () => {
  it('changes only browser appearance immediately and restores the preference on a fresh mount', async () => {
    const user = userEvent.setup();
    const first = renderSettings();
    const main = screen.getByRole('main');
    expect(within(main).getByRole('heading', { level: 1, name: 'Settings' })).toBeVisible();
    expect(within(main).getByRole('heading', { name: 'Appearance' })).toBeVisible();
    const choices = within(main).getByRole('radiogroup', { name: 'Appearance' });
    expect(within(choices).getByRole('radio', { name: 'System' })).toBeChecked();
    expect(within(choices).getByRole('radio', { name: 'Light' })).not.toBeChecked();
    expect(within(main).queryByText(/approval|persistence|remember.*decision/i)).not.toBeInTheDocument();
    expect(within(main).queryByRole('combobox')).not.toBeInTheDocument();
    expect(within(main).queryByRole('switch')).not.toBeInTheDocument();
    await user.click(within(choices).getByRole('radio', { name: 'Dark' }));
    expect(localStorage.getItem('aib.theme')).toBe('dark');
    expect(document.documentElement.dataset.theme).toBe('dark');
    first.unmount();
    renderSettings();
    expect(screen.getByRole('radio', { name: 'Dark' })).toBeChecked();
    expect(document.documentElement.dataset.theme).toBe('dark');
  });

  it('supports arrow-key selection without a save or submit action', async () => {
    const user = userEvent.setup();
    renderSettings();
    const system = screen.getByRole('radio', { name: 'System' });
    system.focus();
    try {
      await user.keyboard('{ArrowUp>}');
      await waitFor(() => {
        expect(screen.getByRole('radio', { name: 'Dark' })).toHaveFocus();
        expect(screen.getByRole('radio', { name: 'Dark' })).toBeChecked();
      });
    } finally {
      await user.keyboard('{/ArrowUp}');
    }
    expect(localStorage.getItem('aib.theme')).toBe('dark');
    expect(document.documentElement.dataset.theme).toBe('dark');
    expect(within(screen.getByRole('main')).queryByRole('button', { name: /save|submit/i })).not.toBeInTheDocument();
  });
});
