import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { expect, it, vi } from 'vitest';
import { Badge } from './Badge';

it('keeps a status link keyboard accessible without adding an interactive wrapper', async () => {
  const navigate = vi.fn((event: React.MouseEvent) => event.preventDefault());
  render(<Badge asChild variant="warning"><a href="/approvals" onClick={navigate}>Needs review</a></Badge>);
  const user = userEvent.setup();
  await user.tab();
  expect(screen.getByRole('link', { name: 'Needs review' })).toHaveFocus();
  expect(screen.getByRole('link', { name: 'Needs review' })).toHaveAttribute('data-variant', 'warning');
  await user.keyboard('{Enter}');
  expect(navigate).toHaveBeenCalledOnce();
});
