import { cleanup, render, screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { MemoryRouter, Route, Routes } from 'react-router-dom';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { ConsoleShell } from '@design-system/components/layout/ConsoleShell/ConsoleShell';
import { ThemeProvider } from '@design-system/theme/ThemeProvider';
import { commonCopy, navigationCopy, settingsCopy } from '@copy';
import SettingsPage from './SettingsPage';

function renderSettings() {
  return render(
    <MemoryRouter initialEntries={['/settings/appearance']}><ThemeProvider>
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
        <Routes>
          <Route path="/settings/appearance" element={<SettingsPage />} />
          <Route path="/agents" element={<output aria-label="Current route">Agents</output>} />
        </Routes>
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
  it('shows appearance rows without a single-item category menu or decision defaults', () => {
    renderSettings();
    const main = screen.getByRole('main');
    expect(within(main).getByRole('heading', { level: 1, name: 'Settings' })).toBeVisible();
    expect(within(main).getByRole('heading', { level: 2, name: 'Appearance' })).toBeVisible();
    expect(within(main).getByRole('heading', { level: 3, name: settingsCopy.themeLabel })).toBeVisible();
    expect(within(main).getByRole('heading', { level: 3, name: settingsCopy.defaultCollectionView })).toBeVisible();
    expect(within(main).queryByRole('navigation', { name: settingsCopy.categoriesLabel })).not.toBeInTheDocument();
    expect(within(main).queryByText(/approval|persistence|remember.*decision/i)).not.toBeInTheDocument();
    expect(within(main).queryByRole('button', { name: /save|submit/i })).not.toBeInTheDocument();
  });

  it('returns to the concrete Agents route from Appearance', async () => {
    const user = userEvent.setup();
    renderSettings();
    const back = screen.getByRole('link', { name: commonCopy.returnToAgents });
    expect(back).toHaveAttribute('href', '/agents');
    await user.click(back);
    expect(screen.getByLabelText('Current route')).toHaveTextContent('Agents');
  });

  it('updates theme and collection view immediately and restores both on a fresh mount', async () => {
    localStorage.setItem('aib.collection-view.agents', 'grid');
    localStorage.setItem('aib.collection-view.connections', 'grid');
    const user = userEvent.setup();
    const first = renderSettings();
    const theme = screen.getByRole('radiogroup', { name: 'Appearance' });
    const collection = screen.getByRole('group', { name: settingsCopy.defaultCollectionView });
    expect(within(theme).getByRole('radio', { name: 'System' })).toBeChecked();
    expect(within(collection).getByRole('button', { name: 'Grid view' })).toHaveAttribute('aria-pressed', 'true');
    await user.click(within(theme).getByRole('radio', { name: 'Dark' }));
    expect(localStorage.getItem('aib.theme')).toBe('dark');
    expect(document.documentElement.dataset.theme).toBe('dark');
    await user.click(within(collection).getByRole('button', { name: 'List view' }));
    expect(localStorage.getItem('aib.collection-view-default')).toBe('list');
    expect(localStorage.getItem('aib.collection-view.agents')).toBeNull();
    expect(localStorage.getItem('aib.collection-view.connections')).toBeNull();
    first.unmount();
    renderSettings();
    expect(within(screen.getByRole('radiogroup', { name: 'Appearance' })).getByRole('radio', { name: 'Dark' })).toBeChecked();
    expect(within(screen.getByRole('group', { name: settingsCopy.defaultCollectionView })).getByRole('button', { name: 'List view' })).toHaveAttribute('aria-pressed', 'true');
    expect(document.documentElement.dataset.theme).toBe('dark');
  });

  it('clears prior page choices when choosing Grid even if Grid is already displayed', async () => {
    localStorage.setItem('aib.collection-view.agents', 'list');
    localStorage.setItem('aib.collection-view.connections', 'list');
    const user = userEvent.setup();
    renderSettings();
    const collection = screen.getByRole('group', { name: settingsCopy.defaultCollectionView });
    await user.click(within(collection).getByRole('button', { name: 'Grid view' }));
    expect(localStorage.getItem('aib.collection-view-default')).toBe('grid');
    expect(localStorage.getItem('aib.collection-view.agents')).toBeNull();
    expect(localStorage.getItem('aib.collection-view.connections')).toBeNull();
  });

  it('supports arrow-key theme choice and keyboard collection choice without a save action', async () => {
    const user = userEvent.setup();
    renderSettings();
    const theme = screen.getByRole('radiogroup', { name: 'Appearance' });
    await user.click(within(theme).getByRole('radio', { name: 'System' }));
    try {
      await user.keyboard('{ArrowLeft>}');
      await waitFor(() => {
        expect(within(theme).getByRole('radio', { name: 'Dark' })).toHaveFocus();
        expect(within(theme).getByRole('radio', { name: 'Dark' })).toBeChecked();
      });
    } finally {
      await user.keyboard('{/ArrowLeft}');
    }
    expect(localStorage.getItem('aib.theme')).toBe('dark');
    const collection = screen.getByRole('group', { name: settingsCopy.defaultCollectionView });
    const list = within(collection).getByRole('button', { name: 'List view' });
    list.focus();
    await user.keyboard('{Enter}');
    expect(list).toHaveAttribute('aria-pressed', 'true');
    expect(localStorage.getItem('aib.collection-view-default')).toBe('list');
    expect(within(screen.getByRole('main')).queryByRole('button', { name: /save|submit/i })).not.toBeInTheDocument();
  });
});
