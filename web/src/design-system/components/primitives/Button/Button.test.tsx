import { fireEvent, render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { describe, expect, it, vi } from 'vitest';
import { Button } from './Button';

describe('Button', () => {
  it('activates by keyboard without accidentally submitting a form', async () => {
    const activate = vi.fn();
    const submit = vi.fn((event: React.FormEvent) => event.preventDefault());
    render(<form onSubmit={submit}><Button onClick={activate}>Review</Button></form>);
    const user = userEvent.setup();
    await user.tab();
    await user.keyboard('{Enter}');
    expect(activate).toHaveBeenCalledOnce();
    expect(submit).not.toHaveBeenCalled();
  });

  it('blocks duplicate actions while loading and permits them after completion', async () => {
    const activate = vi.fn();
    const { rerender } = render(<Button isLoading onClick={activate}>Allow</Button>);
    const user = userEvent.setup();
    const button = screen.getByRole('button', { name: 'Allow' });
    expect(button).toHaveAttribute('aria-busy', 'true');
    expect(button).toBeDisabled();
    await user.click(button);
    expect(activate).not.toHaveBeenCalled();
    rerender(<Button onClick={activate}>Allow</Button>);
    await user.click(button);
    expect(activate).toHaveBeenCalledOnce();
  });

  it('preserves link semantics and blocks a disabled slotted child handler', async () => {
    const activate = vi.fn((event: React.MouseEvent) => event.preventDefault());
    const { rerender } = render(<Button asChild disabled><a href="/agents" onClick={activate}>Agents</a></Button>);
    const link = screen.getByRole('link', { name: 'Agents' });
    expect(link).toHaveAttribute('aria-disabled', 'true');
    fireEvent.click(link);
    expect(activate).not.toHaveBeenCalled();
    rerender(<Button asChild isLoading><a href="/agents" onClick={activate}>Agents</a></Button>);
    expect(link).toHaveAttribute('aria-busy', 'true');
    fireEvent.click(link);
    expect(activate).not.toHaveBeenCalled();
    rerender(<Button asChild><a href="/agents" onClick={activate}>Agents</a></Button>);
    await userEvent.setup().click(link);
    expect(activate).toHaveBeenCalledOnce();
    expect(screen.queryByRole('button')).not.toBeInTheDocument();
  });
});
