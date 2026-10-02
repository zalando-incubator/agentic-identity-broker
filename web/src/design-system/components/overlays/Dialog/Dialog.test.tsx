import { render, screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { describe, expect, it } from 'vitest';
import { Dialog, DialogClose, DialogContent, DialogDescription, DialogTitle, DialogTrigger } from './Dialog';

function Confirmation() {
  return (
    <>
      <button>Outside action</button>
      <Dialog>
        <DialogTrigger>Review access</DialogTrigger>
        <DialogContent closeLabel="Dismiss access review">
          <DialogTitle>Remove access?</DialogTitle>
          <DialogDescription>The selected agent will lose access.</DialogDescription>
          <button>Keep access</button>
          <DialogClose asChild><button>Cancel review</button></DialogClose>
        </DialogContent>
      </Dialog>
    </>
  );
}

describe('Dialog', () => {
  it('portals a named modal, traps keyboard focus, and restores its trigger on Escape', async () => {
    const user = userEvent.setup();
    const { container } = render(<Confirmation />);
    const trigger = screen.getByRole('button', { name: 'Review access' });
    await user.click(trigger);
    const dialog = screen.getByRole('dialog', { name: 'Remove access?' });
    expect(container).not.toContainElement(dialog);
    expect(dialog).toHaveAccessibleDescription('The selected agent will lose access.');
    const first = within(dialog).getByRole('button', { name: 'Keep access' });
    const last = within(dialog).getByRole('button', { name: 'Dismiss access review' });
    expect(first).toHaveFocus();
    await user.tab({ shift: true });
    expect(last).toHaveFocus();
    await user.tab();
    expect(first).toHaveFocus();
    expect(screen.queryByRole('button', { name: 'Outside action' })).not.toBeInTheDocument();
    await user.keyboard('{Escape}');
    await waitFor(() => expect(screen.queryByRole('dialog')).not.toBeInTheDocument());
    await waitFor(() => expect(trigger).toHaveFocus());
  });

  it('closes through the caller-labeled icon and explicit Close control', async () => {
    const user = userEvent.setup();
    render(<Confirmation />);
    const trigger = screen.getByRole('button', { name: 'Review access' });
    await user.click(trigger);
    await user.click(screen.getByRole('button', { name: 'Dismiss access review' }));
    await waitFor(() => expect(trigger).toHaveFocus());
    await user.click(trigger);
    await user.click(screen.getByRole('button', { name: 'Cancel review' }));
    await waitFor(() => expect(screen.queryByRole('dialog')).not.toBeInTheDocument());
    await waitFor(() => expect(trigger).toHaveFocus());
  });
});
