import { AnimatedConnectionIcon } from '@components/icons/AnimatedIcons';
import type { Meta, StoryObj } from '@storybook/react';
import { expect, fn, userEvent, within } from 'storybook/test';
import { EmptyState } from './EmptyState';

const meta = { title: 'Feedback/EmptyState', component: EmptyState, tags: ['autodocs'], parameters: { a11y: { test: 'error' } }, args: { title: 'No connections yet', description: 'Connections appear after you authorize a service.', icon: <AnimatedConnectionIcon size={40} /> } } satisfies Meta<typeof EmptyState>;
export default meta;
type Story = StoryObj<typeof meta>;
export const Default: Story = {};
export const Dark: Story = { globals: { theme: 'dark' } };
export const LongDescription: Story = { args: { description: 'A connection lets an agent use a service on your behalf. Once you authorize a service, its connection appears here so that you can review access and manage it when your needs change.' } };
export const WithAction: Story = {
  args: { title: 'No matching connections', description: 'Try another search or clear the current filters.', primaryAction: { label: 'Clear filters', onClick: fn() } },
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
