import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { describe, expect, it } from 'vitest';
import { Tooltip, TooltipContent, TooltipProvider, TooltipTrigger } from './Tooltip';

function Help() {
  return (
    <TooltipProvider delayDuration={0}>
      <Tooltip>
        <TooltipTrigger>Access help</TooltipTrigger>
        <TooltipContent>Access ends on the selected date.</TooltipContent>
      </Tooltip>
      <button>Next action</button>
    </TooltipProvider>
  );
}

describe('Tooltip', () => {
  it('portals a description on focus and dismisses on Escape without moving focus', async () => {
    const user = userEvent.setup();
    const { container } = render(<Help />);
    await user.tab();
    const trigger = screen.getByRole('button', { name: 'Access help' });
    const tooltip = await screen.findByRole('tooltip');
    expect(container).not.toContainElement(tooltip);
    expect(trigger).toHaveAccessibleDescription('Access ends on the selected date.');
    expect(trigger).toHaveFocus();
    await user.keyboard('{Escape}');
    await waitFor(() => expect(screen.queryByRole('tooltip')).not.toBeInTheDocument());
    expect(trigger).toHaveFocus();
  });

  it('does not trap focus and dismisses when the trigger loses keyboard focus', async () => {
    const user = userEvent.setup();
    render(<Help />);
    await user.tab();
    await screen.findByRole('tooltip');
    await user.tab();
    expect(screen.getByRole('button', { name: 'Next action' })).toHaveFocus();
    await waitFor(() => expect(screen.queryByRole('tooltip')).not.toBeInTheDocument());
  });
});
