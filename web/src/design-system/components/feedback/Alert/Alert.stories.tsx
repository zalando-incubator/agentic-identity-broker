import type { Meta, StoryObj } from '@storybook/react';
import { expect, fn, userEvent, within } from 'storybook/test';
import { Alert } from './Alert';

const meta = {
  title: 'Feedback/Alert',
  component: Alert,
  tags: ['autodocs'],
  args: { title: 'Connection status', children: 'The connection is ready to use.' },
} satisfies Meta<typeof Alert>;
export default meta;
type Story = StoryObj<typeof meta>;

export const Default: Story = {};
export const Dark: Story = { globals: { theme: 'dark' } };
export const Error: Story = { args: { variant: 'error', title: 'Connection failed', children: 'No changes were saved.' } };
export const Warning: Story = { args: { variant: 'warning', title: 'Connection expires soon' } };
export const Success: Story = { args: { variant: 'success' } };
export const Dismissible: Story = {
  args: { dismissible: true, dismissLabel: 'Dismiss connection status', onDismiss: fn() },
  play: async ({ canvasElement, args }) => {
    const canvas = within(canvasElement);
    await userEvent.click(canvas.getByRole('button', { name: 'Dismiss connection status' }));
    await expect(canvas.queryByRole('status')).not.toBeInTheDocument();
    await expect(args.onDismiss).toHaveBeenCalledOnce();
  },
};
export const FocusVisible: Story = {
  args: { action: { label: 'Review connection', onClick: fn() } },
  play: async ({ canvasElement }) => {
    await userEvent.tab();
    await expect(within(canvasElement).getByRole('button', { name: 'Review connection' })).toHaveFocus();
  },
};
export const Hover: Story = {
  args: FocusVisible.args,
  play: async ({ canvasElement }) => {
    await userEvent.hover(within(canvasElement).getByRole('button', { name: 'Review connection' }));
  },
};
