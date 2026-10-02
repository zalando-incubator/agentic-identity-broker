import { describe, expect, it } from 'vitest';
import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { Accordion } from './Accordion';

const items = [
  { id: 'request', title: 'Request details', content: 'Requested permissions' },
  { id: 'disabled', title: 'Unavailable details', content: 'Hidden permissions', disabled: true },
  { id: 'metadata', title: 'Client metadata', content: 'Verified client details' },
];

describe('Accordion', () => {
  it('moves keyboard focus past disabled sections and wraps around', async () => {
    const user = userEvent.setup();
    render(<Accordion items={items} />);

    screen.getByRole('button', { name: 'Request details' }).focus();
    await user.keyboard('{ArrowDown}');
    expect(screen.getByRole('button', { name: 'Client metadata' })).toHaveFocus();

    await user.keyboard('{ArrowDown}');
    expect(screen.getByRole('button', { name: 'Request details' })).toHaveFocus();

    await user.keyboard('{ArrowUp}');
    expect(screen.getByRole('button', { name: 'Client metadata' })).toHaveFocus();
    expect(screen.getByRole('button', { name: 'Unavailable details' })).toBeDisabled();
  });

  it('closes the previous section when a different section opens', async () => {
    const user = userEvent.setup();
    render(<Accordion items={items} />);

    await user.click(screen.getByRole('button', { name: 'Request details' }));
    expect(screen.getByText('Requested permissions')).toBeVisible();

    await user.click(screen.getByRole('button', { name: 'Client metadata' }));
    expect(screen.getByRole('button', { name: 'Request details' })).toHaveAttribute('aria-expanded', 'false');
    expect(screen.getByText('Verified client details')).toBeVisible();
    await waitFor(() => {
      expect(screen.queryByText('Requested permissions')).not.toBeInTheDocument();
    });
  });
});
