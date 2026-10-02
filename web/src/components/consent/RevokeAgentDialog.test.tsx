import { useState } from 'react';
import { render, screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { describe, expect, it, vi } from 'vitest';
import { RevokeAgentDialog } from './RevokeAgentDialog';

function ConfirmationHarness({ onConfirm }: { onConfirm: () => void }) {
  const [open, setOpen] = useState(false);
  return <>
    <button onClick={() => setOpen(true)}>Manage Calendar Agent access</button>
    <RevokeAgentDialog open={open} agentName="Calendar Agent" onCancel={() => setOpen(false)} onConfirm={onConfirm} />
  </>;
}

describe('agent revoke confirmation', () => {
  it('names the affected agent and explains lost access before a destructive confirmation', () => {
    render(<RevokeAgentDialog open agentName="Calendar Agent" onCancel={vi.fn()} onConfirm={vi.fn()} />);
    const dialog = screen.getByRole('dialog', { name: 'Revoke access' });
    expect(dialog).toHaveAccessibleDescription(/Calendar Agent.*delegated access.*broker/i);
    expect(dialog).toHaveAccessibleDescription(/connections.*remain active/i);
    expect(within(dialog).getByRole('button', { name: 'Revoke', exact: true })).toBeEnabled();
  });

  it('keeps access unchanged on Cancel and returns keyboard focus to the invoking control', async () => {
    const user = userEvent.setup();
    const revoke = vi.fn();
    render(<ConfirmationHarness onConfirm={revoke} />);
    const opener = screen.getByRole('button', { name: 'Manage Calendar Agent access' });
    await user.click(opener);
    const cancel = screen.getByRole('button', { name: 'Cancel', exact: true });
    expect(cancel).toHaveFocus();
    await user.click(cancel);
    await waitFor(() => expect(screen.queryByRole('dialog')).not.toBeInTheDocument());
    expect(revoke).not.toHaveBeenCalled();
    expect(opener).toHaveFocus();
  });

  it('treats Escape as cancellation rather than a security mutation', async () => {
    const user = userEvent.setup();
    const revoke = vi.fn();
    render(<ConfirmationHarness onConfirm={revoke} />);
    await user.click(screen.getByRole('button', { name: 'Manage Calendar Agent access' }));
    await user.keyboard('{Escape}');
    await waitFor(() => expect(screen.queryByRole('dialog')).not.toBeInTheDocument());
    expect(revoke).not.toHaveBeenCalled();
  });

  it('confirms only once per activation and blocks confirmation while pending', async () => {
    const user = userEvent.setup();
    const confirm = vi.fn();
    const cancel = vi.fn();
    const { rerender } = render(<RevokeAgentDialog open agentName="Calendar Agent" pending onCancel={cancel} onConfirm={confirm} />);
    const button = screen.getByRole('button', { name: 'Revoke', exact: true });
    expect(button).toBeDisabled();
    await user.click(button);
    expect(confirm).not.toHaveBeenCalled();
    rerender(<RevokeAgentDialog open agentName="Calendar Agent" onCancel={cancel} onConfirm={confirm} />);
    await user.click(screen.getByRole('button', { name: 'Revoke', exact: true }));
    expect(confirm).toHaveBeenCalledTimes(1);
    expect(cancel).not.toHaveBeenCalled();
  });
});
