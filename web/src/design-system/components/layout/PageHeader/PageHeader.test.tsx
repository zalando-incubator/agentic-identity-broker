import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { describe, expect, it, vi } from 'vitest';
import { Button } from '@design-system/components/primitives/Button/Button';
import { PageHeader } from './PageHeader';

describe('PageHeader', () => {
  it('shows a single page heading without requiring purpose or actions', () => {
    render(<PageHeader title="Agents" />);
    expect(screen.getAllByRole('heading', { level: 1 })).toHaveLength(1);
    expect(screen.getByRole('heading', { level: 1 })).toHaveAccessibleName('Agents');
    expect(screen.queryByRole('button')).not.toBeInTheDocument();
  });

  it('keeps a zero count visible beside the title and an optional purpose below it', () => {
    const { rerender } = render(<PageHeader title="Agents" count={0} />);
    expect(screen.getByRole('heading', { level: 1 })).toHaveAccessibleName('Agents');
    expect(screen.getByText('· 0')).toBeInTheDocument();
    rerender(<PageHeader title="Agents" count={4} purpose="Manage the access you grant to agents." />);
    expect(screen.getByText('· 4')).toBeInTheDocument();
    expect(screen.getByText('Manage the access you grant to agents.')).toBeInTheDocument();
  });

  it('keeps assigned actions keyboard-operable', async () => {
    const user = userEvent.setup();
    const save = vi.fn();
    render(<PageHeader title="Agent access" actions={<Button onClick={save}>Save changes</Button>} />);
    await user.tab();
    expect(screen.getByRole('button', { name: 'Save changes' })).toHaveFocus();
    await user.keyboard('{Enter}');
    expect(save).toHaveBeenCalledOnce();
  });
});
