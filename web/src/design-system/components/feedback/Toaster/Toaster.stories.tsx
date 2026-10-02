import { useEffect, useId } from 'react';
import type { Meta, StoryObj } from '@storybook/react';
import { expect, userEvent, waitFor, within } from 'storybook/test';
import { toast } from 'sonner';
import { Button } from '@design-system/components/primitives/Button';
import { Toaster } from './Toaster';

function NotificationExample({ kind = 'success' }: { kind?: 'success' | 'error' | 'loading' }) {
  const id = useId();
  useEffect(() => () => { toast.dismiss(id); }, [id]);
  return <><Button variant="outline" onClick={() => toast[kind](kind === 'loading' ? 'Saving permission changes' : kind === 'error' ? 'Permission changes could not be saved' : 'Permission changes saved', { id, duration: Infinity })}>Show notification</Button><Toaster label="Notifications" closeButtonLabel="Dismiss notification" closeButton /></>;
}
const meta = { title: 'Feedback/Toaster', component: Toaster, tags: ['autodocs'], args: { label: 'Notifications', closeButtonLabel: 'Dismiss notification' }, render: () => <NotificationExample /> } satisfies Meta<typeof Toaster>;
export default meta;
type Story = StoryObj<typeof meta>;
const open: Story['play'] = async ({ canvasElement }) => {
  await userEvent.click(within(canvasElement).getByRole('button', { name: 'Show notification' }));
  await expect(within(canvasElement.ownerDocument.body).getByRole('region', { name: 'Notifications' })).toHaveAttribute('aria-live', 'polite');
};
export const Default: Story = { play: open };
export const Dark: Story = { globals: { theme: 'dark' }, play: open };
export const Error: Story = { render: () => <NotificationExample kind="error" />, play: open };
export const Loading: Story = { render: () => <NotificationExample kind="loading" />, play: open };
export const Dismissible: Story = {
  play: async (context) => {
    await open(context);
    const body = within(context.canvasElement.ownerDocument.body);
    await userEvent.click(await body.findByRole('button', { name: 'Dismiss notification' }));
    await waitFor(() => expect(body.queryByText('Permission changes saved')).not.toBeInTheDocument());
  },
};
export const FocusVisible: Story = {
  play: async (context) => {
    await open(context);
    await userEvent.tab();
    await userEvent.tab();
    await expect(within(context.canvasElement.ownerDocument.body).getByRole('button', { name: 'Dismiss notification' })).toHaveFocus();
  },
};
export const Hover: Story = {
  play: async (context) => {
    await open(context);
    await userEvent.hover(within(context.canvasElement.ownerDocument.body).getByRole('button', { name: 'Dismiss notification' }));
  },
};
