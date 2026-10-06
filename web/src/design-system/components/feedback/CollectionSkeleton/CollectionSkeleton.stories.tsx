import type { Meta, StoryObj } from '@storybook/react';
import { CollectionSkeleton } from './CollectionSkeleton';

const meta = {
  title: 'Design System/Feedback/CollectionSkeleton',
  component: CollectionSkeleton,
  parameters: { layout: 'padded', a11y: { test: 'error' } },
  tags: ['autodocs'],
  args: { label: 'Loading agents', view: 'grid', count: 6 },
} satisfies Meta<typeof CollectionSkeleton>;
export default meta;
type Story = StoryObj<typeof meta>;

export const Grid: Story = {};
export const List: Story = {
  args: { view: 'list', count: 4 },
};
export const Dark: Story = { globals: { theme: 'dark' } };

export const CompactGrid: Story = {
  args: { cardSize: 'compact', count: 3 },
  globals: { viewport: { value: 'narrow320', isRotated: false } },
};
export const CompactGridDark: Story = {
  ...CompactGrid,
  globals: { theme: 'dark', viewport: { value: 'narrow320', isRotated: false } },
};
export const CompactList: Story = {
  args: { view: 'list', cardSize: 'compact', count: 3 },
  globals: { viewport: { value: 'mobile', isRotated: false } },
};
export const CompactListDark: Story = {
  ...CompactList,
  globals: { theme: 'dark', viewport: { value: 'mobile', isRotated: false } },
};
