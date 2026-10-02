import { useState } from 'react';
import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { describe, expect, it } from 'vitest';
import { DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuRadioGroup, DropdownMenuRadioItem, DropdownMenuTrigger } from './DropdownMenu';

function AppearanceMenu() {
  const [theme, setTheme] = useState('system');
  return (
    <DropdownMenu>
      <DropdownMenuTrigger>User menu</DropdownMenuTrigger>
      <DropdownMenuContent>
        <DropdownMenuItem disabled>Unavailable action</DropdownMenuItem>
        <DropdownMenuRadioGroup value={theme} onValueChange={setTheme} aria-label="Appearance">
          <DropdownMenuRadioItem value="system">System</DropdownMenuRadioItem>
          <DropdownMenuRadioItem value="light">Light</DropdownMenuRadioItem>
          <DropdownMenuRadioItem value="dark">Dark</DropdownMenuRadioItem>
        </DropdownMenuRadioGroup>
      </DropdownMenuContent>
    </DropdownMenu>
  );
}

describe('DropdownMenu', () => {
  it('portals its menu, skips disabled items, and returns keyboard focus after Escape', async () => {
    const user = userEvent.setup();
    const { container } = render(<AppearanceMenu />);
    await user.tab();
    const trigger = screen.getByRole('button', { name: 'User menu' });
    await user.keyboard('{ArrowDown}');
    const menu = screen.getByRole('menu');
    expect(container).not.toContainElement(menu);
    expect(screen.getByRole('menuitem', { name: 'Unavailable action' })).toHaveAttribute('aria-disabled', 'true');
    await waitFor(() => expect(screen.getByRole('menuitemradio', { name: 'System' })).toHaveFocus());
    await user.keyboard('{ArrowDown}');
    expect(screen.getByRole('menuitemradio', { name: 'Light' })).toHaveFocus();
    await user.keyboard('{Escape}');
    await waitFor(() => expect(screen.queryByRole('menu')).not.toBeInTheDocument());
    await waitFor(() => expect(trigger).toHaveFocus());
  });

  it('selects one radio value and retains that choice when reopened', async () => {
    const user = userEvent.setup();
    render(<AppearanceMenu />);
    const trigger = screen.getByRole('button', { name: 'User menu' });
    await user.tab();
    await user.keyboard('{Enter}');
    await waitFor(() => expect(screen.getByRole('menuitemradio', { name: 'System' })).toHaveFocus());
    await user.keyboard('{ArrowDown}{ArrowDown}{Enter}');
    await waitFor(() => expect(screen.queryByRole('menu')).not.toBeInTheDocument());
    await waitFor(() => expect(trigger).toHaveFocus());
    await user.keyboard('{Enter}');
    expect(screen.getByRole('menuitemradio', { name: 'Dark' })).toHaveAttribute('aria-checked', 'true');
    expect(screen.getByRole('menuitemradio', { name: 'System' })).toHaveAttribute('aria-checked', 'false');
  });
});
