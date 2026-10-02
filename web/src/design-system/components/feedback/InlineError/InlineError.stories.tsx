import type { Meta, StoryObj } from '@storybook/react';
import { expect, fn, userEvent, within } from 'storybook/test';
import { InlineError } from './InlineError';

const meta = { title: 'Feedback/InlineError', component: InlineError, tags: ['autodocs'], args: { message: 'Unable to load connections.' } } satisfies Meta<typeof InlineError>;
export default meta;
type Story = StoryObj<typeof meta>;
export const Default: Story = {};
export const Dark: Story = { globals: { theme: 'dark' } };
export const Error: Story = { args: { errors: ['Choose an end date.', 'Choose at least one permission.'] } };
export const Retry: Story = {
  args: { onRetry: fn(), retryLabel: 'Load connections again' },
  play: async ({ canvasElement, args }) => {
    await userEvent.click(within(canvasElement).getByRole('button', { name: 'Load connections again' }));
    await expect(args.onRetry).toHaveBeenCalledOnce();
  },
};
export const Loading: Story = {
  args: { ...Retry.args, onRetry: fn(), isRetrying: true },
  play: async ({ canvasElement, args }) => {
    const button = within(canvasElement).getByRole('button', { name: 'Load connections again' });
    await expect(button).toBeDisabled();
    await userEvent.click(button);
    await expect(args.onRetry).not.toHaveBeenCalled();
  },
};
export const FocusVisible: Story = {
  args: Retry.args,
  play: async ({ canvasElement }) => {
    await userEvent.tab();
    await expect(within(canvasElement).getByRole('button', { name: 'Load connections again' })).toHaveFocus();
  },
};
export const Hover: Story = {
  args: Retry.args,
  play: async ({ canvasElement }) => { await userEvent.hover(within(canvasElement).getByRole('button', { name: 'Load connections again' })); },
};
