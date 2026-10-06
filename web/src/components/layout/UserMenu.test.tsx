import { cleanup, render, screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { MemoryRouter } from 'react-router-dom';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import type { QueryClient } from '@tanstack/react-query';
import { ThemeProvider } from '@design-system/theme/ThemeProvider';
import { QueryProvider } from '@services/query/QueryProvider';
import { createQueryClient } from '@services/query/queryClient';
import { consentApi } from '@services/api/consent';
import UserMenu from './UserMenu';

const clients: QueryClient[] = [];

function renderMenu() {
  const client = createQueryClient();
  clients.push(client);
  return render(<MemoryRouter><QueryProvider client={client}><ThemeProvider><UserMenu /></ThemeProvider></QueryProvider></MemoryRouter>);
}

beforeEach(() => {
  localStorage.setItem('aib.theme', 'system');
  vi.stubGlobal('matchMedia', vi.fn(() => ({ matches: false, addEventListener: vi.fn(), removeEventListener: vi.fn() })));
  vi.spyOn(consentApi, 'getUserInfo').mockResolvedValue({
    principal: 'alice@example.test', displayName: 'Alice Example', pictureUrl: 'https://untrusted.example/avatar.png',
  });
});
afterEach(() => {
  cleanup();
  for (const client of clients.splice(0)) client.clear();
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
  localStorage.clear();
});

describe('UserMenu', () => {
  it('shows authenticated identity without loading its external image and applies a persisted theme', async () => {
    const user = userEvent.setup();
    renderMenu();
    await user.click(await screen.findByRole('button', { name: 'User menu' }));
    const menu = screen.getByRole('menu');
    expect(within(menu).getByText('Alice Example')).toBeVisible();
    expect(within(menu).getByText('alice@example.test')).toBeVisible();
    expect(document.querySelector('img[src="https://untrusted.example/avatar.png"]')).not.toBeInTheDocument();
    expect(within(menu).getByRole('menuitemradio', { name: 'System' })).toBeChecked();
    expect(within(menu).getByRole('menuitemradio', { name: 'Light' })).not.toBeChecked();
    await user.click(within(menu).getByRole('menuitemradio', { name: 'Dark' }));
    expect(localStorage.getItem('aib.theme')).toBe('dark');
    expect(document.documentElement.dataset.theme).toBe('dark');
    await user.click(screen.getByRole('button', { name: 'User menu' }));
    expect(screen.getByRole('menuitemradio', { name: 'Dark' })).toBeChecked();
  });

  it('selects appearance by keyboard and returns focus when selected or dismissed', async () => {
    const user = userEvent.setup();
    renderMenu();
    const trigger = await screen.findByRole('button', { name: 'User menu' });
    await user.tab();
    expect(trigger).toHaveFocus();
    await user.keyboard('{Enter}');
    await user.keyboard('d');
    await waitFor(() => expect(screen.getByRole('menuitemradio', { name: 'Dark' })).toHaveFocus());
    await user.keyboard('{Enter}');
    expect(localStorage.getItem('aib.theme')).toBe('dark');
    await waitFor(() => expect(trigger).toHaveFocus());
    expect(screen.queryByRole('menu')).not.toBeInTheDocument();
    await user.keyboard('{Enter}');
    expect(screen.getByRole('menuitemradio', { name: 'Dark' })).toBeChecked();
    await user.keyboard('{Escape}');
    await waitFor(() => expect(trigger).toHaveFocus());
    expect(screen.queryByRole('menu')).not.toBeInTheDocument();
  });
});
