import { render, screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { afterEach, describe, expect, it } from 'vitest';
import { Sheet, SheetClose, SheetContent, SheetDescription, SheetTitle, SheetTrigger } from './Sheet';

const originalUrl = window.location.href;
afterEach(() => window.history.replaceState(null, '', originalUrl));

function Navigation() {
  return (
    <Sheet>
      <SheetTrigger>Open navigation</SheetTrigger>
      <SheetContent side="left" closeLabel="Close navigation">
        <SheetTitle>Console navigation</SheetTitle>
        <SheetDescription>Choose a console page.</SheetDescription>
        <nav aria-label="Console pages">
          <SheetClose asChild><a href="#agents">Agents</a></SheetClose>
          <SheetClose asChild><a href="#connections">Connections</a></SheetClose>
        </nav>
      </SheetContent>
    </Sheet>
  );
}

describe('Sheet', () => {
  it('portals navigation, contains focus, and restores the opener after Escape', async () => {
    const user = userEvent.setup();
    const { container } = render(<Navigation />);
    const trigger = screen.getByRole('button', { name: 'Open navigation' });
    await user.click(trigger);
    const sheet = screen.getByRole('dialog', { name: 'Console navigation' });
    expect(container).not.toContainElement(sheet);
    expect(sheet).toHaveAccessibleDescription('Choose a console page.');
    const first = within(sheet).getByRole('link', { name: 'Agents' });
    expect(sheet).toContainElement(document.activeElement as HTMLElement);
    first.focus();
    await user.tab({ shift: true });
    expect(within(sheet).getByRole('button', { name: 'Close navigation' })).toHaveFocus();
    await user.tab();
    expect(first).toHaveFocus();
    await user.keyboard('{Escape}');
    await waitFor(() => expect(screen.queryByRole('dialog')).not.toBeInTheDocument());
    await waitFor(() => expect(trigger).toHaveFocus());
  });

  it('closes after a navigation link is activated', async () => {
    const user = userEvent.setup();
    render(<Navigation />);
    const trigger = screen.getByRole('button', { name: 'Open navigation' });
    await user.click(trigger);
    await user.click(screen.getByRole('link', { name: 'Connections' }));
    await waitFor(() => expect(screen.queryByRole('dialog')).not.toBeInTheDocument());
    await waitFor(() => expect(trigger).toHaveFocus());
  });
});
