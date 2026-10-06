import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { describe, expect, it } from 'vitest';
import { Popover, PopoverClose, PopoverContent, PopoverTrigger } from './Popover';

function Details({ modal = false }: { modal?: boolean }) {
  return (
    <>
      <Popover modal={modal}>
        <PopoverTrigger>Show details</PopoverTrigger>
        <PopoverContent aria-label="Access details">
          <a href="#documentation">Documentation</a>
          <PopoverClose>Close details</PopoverClose>
        </PopoverContent>
      </Popover>
      <button>Outside action</button>
    </>
  );
}

describe('Popover', () => {
  it('portals modal content, traps focus, and restores the trigger after Escape', async () => {
    const user = userEvent.setup();
    const { container } = render(<Details modal />);
    const trigger = screen.getByRole('button', { name: 'Show details' });
    await user.click(trigger);
    expect(container).not.toContainElement(screen.getByRole('dialog', { name: 'Access details' }));
    const first = screen.getByRole('link', { name: 'Documentation' });
    expect(screen.getByRole('dialog', { name: 'Access details' })).toContainElement(document.activeElement as HTMLElement);
    first.focus();
    await user.tab({ shift: true });
    expect(screen.getByRole('button', { name: 'Close details' })).toHaveFocus();
    await user.tab();
    expect(first).toHaveFocus();
    await user.keyboard('{Escape}');
    await waitFor(() => expect(screen.queryByRole('dialog')).not.toBeInTheDocument());
    await waitFor(() => expect(trigger).toHaveFocus());
  });

  it('allows an outside interaction to dismiss nonmodal content without stealing focus', async () => {
    const user = userEvent.setup();
    render(<Details />);
    await user.click(screen.getByRole('button', { name: 'Show details' }));
    const outside = screen.getByRole('button', { name: 'Outside action' });
    await user.click(outside);
    await waitFor(() => expect(screen.queryByRole('dialog')).not.toBeInTheDocument());
    expect(outside).toHaveFocus();
  });
});
