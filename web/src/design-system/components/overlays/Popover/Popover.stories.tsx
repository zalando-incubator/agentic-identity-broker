import { useId } from 'react';
import type { Meta, StoryObj } from '@storybook/react';
import { expect, userEvent, waitFor, within } from 'storybook/test';
import { Button } from '@design-system/components/primitives/Button';
import { Popover, PopoverClose, PopoverContent, PopoverTrigger } from './Popover';

function AccessDetails({ disabled = false }: { disabled?: boolean }) {
  const titleId = useId();
  const descriptionId = useId();
  return (
    <Popover>
      <PopoverTrigger asChild><Button variant="outline" disabled={disabled}>View access details</Button></PopoverTrigger>
      <PopoverContent aria-labelledby={titleId} aria-describedby={descriptionId} className="space-y-3">
        <h2 id={titleId} className="font-semibold">Access details</h2>
        <p id={descriptionId} className="text-sm text-muted-foreground">This connection allows the agent to read your calendar.</p>
        <PopoverClose asChild><Button variant="secondary">Close details</Button></PopoverClose>
      </PopoverContent>
    </Popover>
  );
}

const meta = {
  title: 'Design System/Overlays/Popover',
  component: Popover,
  parameters: { layout: 'centered', a11y: { test: 'error' } },
  tags: ['autodocs'],
  render: () => <AccessDetails />,
} satisfies Meta<typeof Popover>;
export default meta;
type Story = StoryObj<typeof meta>;

export const Default: Story = {};
export const OverlayOpen: Story = {
  play: async ({ canvasElement }) => {
    await userEvent.click(within(canvasElement).getByRole('button', { name: 'View access details' }));
    await waitFor(() => expect(within(canvasElement.ownerDocument.body).getByRole('dialog', { name: 'Access details' })).toBeVisible());
  },
};
export const FocusVisible: Story = {
  play: async ({ canvasElement }) => {
    await userEvent.tab();
    await userEvent.keyboard('{Enter}');
    await waitFor(() => expect(within(canvasElement.ownerDocument.body).getByRole('button', { name: 'Close details' })).toHaveFocus());
  },
};
export const FocusReturn: Story = {
  play: async ({ canvasElement }) => {
    const trigger = within(canvasElement).getByRole('button', { name: 'View access details' });
    await userEvent.click(trigger);
    await userEvent.keyboard('{Escape}');
    await waitFor(() => expect(within(canvasElement.ownerDocument.body).queryByRole('dialog')).not.toBeInTheDocument());
    await waitFor(() => expect(trigger).toHaveFocus());
  },
};
export const Hover: Story = {
  play: async ({ canvasElement }) => {
    await userEvent.hover(within(canvasElement).getByRole('button', { name: 'View access details' }));
  },
};
export const Disabled: Story = {
  render: () => <AccessDetails disabled />,
  play: async ({ canvasElement }) => {
    await expect(within(canvasElement).getByRole('button', { name: 'View access details' })).toBeDisabled();
  },
};
