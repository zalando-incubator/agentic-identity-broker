import type { Meta, StoryObj } from '@storybook/react';
import { expect, fn, userEvent, within } from 'storybook/test';
import { EmptyState } from './EmptyState';
import { Wordmark } from '@design-system/components/primitives/Wordmark';

const meta = { title: 'Feedback/EmptyState', component: EmptyState, tags: ['autodocs'], args: { title: 'No connections yet', description: 'Connections appear after you authorize a service.' } } satisfies Meta<typeof EmptyState>;
export default meta;
type Story = StoryObj<typeof meta>;
export const Default: Story = {};
export const Dark: Story = { globals: { theme: 'dark' } };
export const WithWordmark: Story = { args: { wordmark: <Wordmark label="Account console" /> } };
export const WithAction: Story = {
  args: { title: 'No matching connections', primaryAction: { label: 'Clear filters', onClick: fn() } },
  play: async ({ canvasElement, args }) => {
    await userEvent.click(within(canvasElement).getByRole('button', { name: 'Clear filters' }));
    await expect(args.primaryAction?.onClick).toHaveBeenCalledOnce();
  },
};
export const FocusVisible: Story = {
  args: WithAction.args,
  play: async ({ canvasElement }) => {
    await userEvent.tab();
    await expect(within(canvasElement).getByRole('button', { name: 'Clear filters' })).toHaveFocus();
  },
};
export const Hover: Story = {
  args: WithAction.args,
  play: async ({ canvasElement }) => { await userEvent.hover(within(canvasElement).getByRole('button', { name: 'Clear filters' })); },
};
