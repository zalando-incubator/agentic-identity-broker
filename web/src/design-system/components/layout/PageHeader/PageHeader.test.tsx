import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { describe, expect, it, vi } from 'vitest';
import { Button } from '@design-system/components/primitives/Button/Button';
import { PageHeader } from './PageHeader';

describe('PageHeader', () => {
  it('introduces the page with one heading and purpose, without inventing an action', () => {
    render(<PageHeader title="Agents" purpose="Manage the access you grant to agents." />);
    expect(screen.getAllByRole('heading', { level: 1 })).toHaveLength(1);
    expect(screen.getByRole('heading', { level: 1 })).toHaveAccessibleName('Agents');
    expect(screen.getByText('Manage the access you grant to agents.')).toBeInTheDocument();
    expect(screen.queryByRole('button')).not.toBeInTheDocument();
  });

  it('keeps the assigned action keyboard-operable', async () => {
    const user = userEvent.setup();
    const save = vi.fn();
    render(<PageHeader title="Agent access" purpose="Choose what this agent can do." action={<Button onClick={save}>Save changes</Button>} />);
    await user.tab();
    expect(screen.getByRole('button', { name: 'Save changes' })).toHaveFocus();
    await user.keyboard('{Enter}');
    expect(save).toHaveBeenCalledOnce();
  });
});
