import type { Meta, StoryObj } from '@storybook/react';
import { expect, userEvent, waitFor, within } from 'storybook/test';
import { Button } from '@design-system/components/primitives/Button';
import { Dialog, DialogClose, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle, DialogTrigger } from './Dialog';

function Confirmation({ disabled = false, pending = false, error = false }: { disabled?: boolean; pending?: boolean; error?: boolean }) {
  return (
    <Dialog>
      <DialogTrigger asChild><Button variant="outline" disabled={disabled}>Review access</Button></DialogTrigger>
      <DialogContent closeLabel="Close access review">
        <DialogHeader>
          <DialogTitle>Remove Research Assistant access?</DialogTitle>
          <DialogDescription>The agent will no longer be able to use this connection.</DialogDescription>
        </DialogHeader>
        {error && <p role="alert" className="text-sm text-destructive">Access could not be removed. Try again.</p>}
        <DialogFooter>
          <DialogClose asChild><Button variant="secondary">Keep access</Button></DialogClose>
          <Button variant="destructive" isLoading={pending}>{pending ? 'Removing access' : 'Remove access'}</Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

const meta = {
  title: 'Design System/Overlays/Dialog',
  component: Dialog,
  parameters: { layout: 'centered', a11y: { test: 'error' } },
  tags: ['autodocs'],
  render: () => <Confirmation />,
} satisfies Meta<typeof Dialog>;
export default meta;
type Story = StoryObj<typeof meta>;

export const Default: Story = {};
export const OverlayOpen: Story = {
  play: async ({ canvasElement }) => {
    await userEvent.click(within(canvasElement).getByRole('button', { name: 'Review access' }));
    await expect(within(canvasElement.ownerDocument.body).getByRole('dialog')).toHaveAccessibleName('Remove Research Assistant access?');
  },
};
export const FocusVisible: Story = {
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await userEvent.tab();
    await expect(canvas.getByRole('button', { name: 'Review access' })).toHaveFocus();
    await userEvent.keyboard('{Enter}');
    const body = within(canvasElement.ownerDocument.body);
    await expect(body.getByRole('button', { name: 'Keep access' })).toHaveFocus();
    await userEvent.tab({ shift: true });
    await expect(body.getByRole('button', { name: 'Close access review' })).toHaveFocus();
  },
};
export const FocusReturn: Story = {
  play: async ({ canvasElement }) => {
    const trigger = within(canvasElement).getByRole('button', { name: 'Review access' });
    await userEvent.click(trigger);
    await userEvent.keyboard('{Escape}');
    await waitFor(() => expect(within(canvasElement.ownerDocument.body).queryByRole('dialog')).not.toBeInTheDocument());
    await waitFor(() => expect(trigger).toHaveFocus());
  },
};
export const Hover: Story = {
  play: async ({ canvasElement }) => {
    await userEvent.hover(within(canvasElement).getByRole('button', { name: 'Review access' }));
  },
};
export const Disabled: Story = {
  render: () => <Confirmation disabled />,
  play: async ({ canvasElement }) => {
    await expect(within(canvasElement).getByRole('button', { name: 'Review access' })).toBeDisabled();
  },
};
export const Error: Story = { render: () => <Confirmation error />, play: OverlayOpen.play };
export const Loading: Story = { render: () => <Confirmation pending />, play: OverlayOpen.play };
