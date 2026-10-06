import type { Meta, StoryObj } from '@storybook/react';
import { expect, within } from 'storybook/test';
import { Skeleton } from './Skeleton';

const meta = { title: 'Feedback/Skeleton', component: Skeleton, tags: ['autodocs'], args: { label: 'Loading connections', count: 3 } } satisfies Meta<typeof Skeleton>;
export default meta;
type Story = StoryObj<typeof meta>;
export const Default: Story = {
  play: async ({ canvasElement }) => {
    await expect(within(canvasElement).getByRole('status')).toHaveTextContent('Loading connections');
  },
};
export const Dark: Story = { globals: { theme: 'dark' } };
export const Loading: Story = {
  render: () => <section aria-label="Loading agent details" aria-busy="true" className="space-y-4"><Skeleton variant="circle" width="3rem" /><Skeleton width="60%" /><Skeleton count={3} /></section>,
};
