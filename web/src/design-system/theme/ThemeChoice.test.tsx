import { createPortal } from 'react-dom';
import { DropdownMenu } from 'radix-ui';
import { render, screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { ThemeChoice } from './ThemeChoice';
import { ThemeProvider, useTheme } from './ThemeProvider';

const labels = { light: 'Clair', dark: 'Sombre', system: 'Système' };

function ThemeConsumer() {
  const { resolvedTheme } = useTheme();
  return createPortal(<output aria-label="Portal appearance">{resolvedTheme}</output>, document.body);
}

beforeEach(() => {
  localStorage.setItem('aib.theme', 'system');
  vi.stubGlobal('matchMedia', vi.fn(() => ({ matches: false, addEventListener: vi.fn(), removeEventListener: vi.fn() })));
});
afterEach(() => { vi.unstubAllGlobals(); localStorage.clear(); });

describe('ThemeChoice', () => {
  it('reflects the stored preference using caller labels and propagates changes to portal consumers', async () => {
    localStorage.setItem('aib.theme', 'light');
    const user = userEvent.setup();
    render(<ThemeProvider><ThemeChoice labels={labels} aria-label="Apparence" /><ThemeConsumer /></ThemeProvider>);
    const choices = screen.getByRole('radiogroup', { name: 'Apparence' });
    expect(within(choices).getByRole('radio', { name: labels.light })).toBeChecked();
    await user.click(within(choices).getByRole('radio', { name: labels.dark }));
    expect(within(choices).getByRole('radio', { name: labels.dark })).toBeChecked();
    expect(screen.getByLabelText('Portal appearance')).toHaveTextContent('dark');
    expect(document.documentElement.dataset.theme).toBe('dark');
    expect(localStorage.getItem('aib.theme')).toBe('dark');
  });

  it('changes form choices by keyboard without submitting its surrounding form', async () => {
    const user = userEvent.setup();
    const submit = vi.fn((event: React.FormEvent) => event.preventDefault());
    render(<ThemeProvider><form onSubmit={submit}><ThemeChoice labels={labels} aria-label="Apparence" /></form></ThemeProvider>);
    await user.tab();
    expect(screen.getByRole('radio', { name: labels.system })).toHaveFocus();
    try {
      await user.keyboard('{ArrowUp>}');
      await waitFor(() => {
        expect(screen.getByRole('radio', { name: labels.dark })).toHaveFocus();
        expect(screen.getByRole('radio', { name: labels.dark })).toBeChecked();
      });
    } finally {
      await user.keyboard('{/ArrowUp}');
    }
    expect(screen.getByRole('radio', { name: labels.dark })).toBeChecked();
    try {
      await user.keyboard('{ArrowUp>}');
      await waitFor(() => {
        expect(screen.getByRole('radio', { name: labels.light })).toHaveFocus();
        expect(screen.getByRole('radio', { name: labels.light })).toBeChecked();
      });
    } finally {
      await user.keyboard('{/ArrowUp}');
    }
    expect(screen.getByRole('radio', { name: labels.light })).toBeChecked();
    expect(submit).not.toHaveBeenCalled();
  });

  it('offers menu radio choices in an enclosing user menu and applies their selection', async () => {
    const user = userEvent.setup();
    render(
      <ThemeProvider>
        <DropdownMenu.Root>
          <DropdownMenu.Trigger>Account</DropdownMenu.Trigger>
          <DropdownMenu.Portal><DropdownMenu.Content>
            <ThemeChoice labels={labels} presentation="menu" aria-label="Apparence" />
          </DropdownMenu.Content></DropdownMenu.Portal>
        </DropdownMenu.Root>
        <ThemeConsumer />
      </ThemeProvider>,
    );
    await user.click(screen.getByRole('button', { name: 'Account' }));
    expect(screen.getByRole('menuitemradio', { name: labels.system })).toBeChecked();
    const dark = screen.getByRole('menuitemradio', { name: labels.dark });
    dark.focus();
    await user.keyboard('{Enter}');
    expect(screen.getByLabelText('Portal appearance')).toHaveTextContent('dark');
    expect(localStorage.getItem('aib.theme')).toBe('dark');
  });
});
