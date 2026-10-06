import type { Meta, StoryObj } from '@storybook/react';
import { expect, userEvent, waitFor, within } from 'storybook/test';
import { Button } from '@design-system/components/primitives/Button';
import { Tooltip, TooltipContent, TooltipProvider, TooltipTrigger } from './Tooltip';

function AccessHelp({ disabled = false }: { disabled?: boolean }) {
  return (
    <TooltipProvider delayDuration={200}>
      <Tooltip>
        <TooltipTrigger asChild><Button variant="outline" disabled={disabled}>Access duration</Button></TooltipTrigger>
        <TooltipContent>Access ends on the date you select.</TooltipContent>
      </Tooltip>
    </TooltipProvider>
  );
}

const meta = {
  title: 'Design System/Overlays/Tooltip',
  component: Tooltip,
  parameters: { layout: 'centered', a11y: { test: 'error' } },
  tags: ['autodocs'],
  render: () => <AccessHelp />,
} satisfies Meta<typeof Tooltip>;
export default meta;
type Story = StoryObj<typeof meta>;

export const Default: Story = {};
export const OverlayOpen: Story = {
  play: async ({ canvasElement }) => {
    await userEvent.hover(within(canvasElement).getByRole('button', { name: 'Access duration' }));
    await expect(await within(canvasElement.ownerDocument.body).findByRole('tooltip')).toHaveTextContent('Access ends on the date you select.');
  },
};
export const Hover: Story = { play: OverlayOpen.play };
export const FocusVisible: Story = {
  play: async ({ canvasElement }) => {
    await userEvent.tab();
    const trigger = within(canvasElement).getByRole('button', { name: 'Access duration' });
    await expect(trigger).toHaveFocus();
    await expect(await within(canvasElement.ownerDocument.body).findByRole('tooltip')).toHaveTextContent('Access ends on the date you select.');
    await expect(trigger).toHaveAccessibleDescription('Access ends on the date you select.');
  },
};
export const Escape: Story = {
  play: async ({ canvasElement }) => {
    const trigger = within(canvasElement).getByRole('button', { name: 'Access duration' });
    await userEvent.unhover(trigger);
    await userEvent.tab();
    await expect(trigger).toHaveFocus();
    const body = within(canvasElement.ownerDocument.body);
    await waitFor(() => expect(body.getByRole('tooltip')).toBeVisible());
    await userEvent.keyboard('{Escape}');
    await waitFor(() => expect(canvasElement.ownerDocument.querySelector('[data-slot="tooltip-content"]')).not.toBeInTheDocument());
    await expect(trigger).toHaveFocus();
  },
};
export const Disabled: Story = {
  render: () => <AccessHelp disabled />,
  play: async ({ canvasElement }) => {
    await expect(within(canvasElement).getByRole('button', { name: 'Access duration' })).toBeDisabled();
  },
};
