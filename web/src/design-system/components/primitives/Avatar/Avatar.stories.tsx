import { useState } from 'react';
import type { Meta, StoryObj } from '@storybook/react';
import { expect, userEvent, waitFor, within } from 'storybook/test';
import { Button } from '@design-system/components/primitives/Button';
import { Avatar } from './Avatar';

const meta = {
  title: 'Design System/Primitives/Avatar', component: Avatar,
  parameters: { layout: 'centered', a11y: { test: 'error' } },
  args: { label: 'Inventory agent', id: 'inventory-agent' },
} satisfies Meta<typeof Avatar>;
export default meta;
type Story = StoryObj<typeof meta>;
export const Default: Story = {};
export const Person: Story = { args: { label: 'Ada Lovelace', id: 'person-ada', person: true } };
export const LocalImage: Story = { args: { src: '/brand/aib-mark.svg' } };
export const OffOriginFallback: Story = {
  args: { src: 'https://untrusted.example/avatar.png' },
  play: async ({ canvasElement }) => {
    await expect(within(canvasElement).getByRole('img', { name: 'Inventory agent' })).toHaveTextContent('I');
    await expect(canvasElement.querySelector('img')).toBeNull();
  },
};
export const Sizes: Story = {
  render: (args) => <div className="flex items-center gap-3"><Avatar {...args} size="sm" /><Avatar {...args} /><Avatar {...args} size="lg" /></div>,
};
export const Tints: Story = {
  render: () => <div className="flex flex-wrap items-center gap-3">
    {['avatar-3', 'avatar-8', 'avatar-5', 'avatar-2', 'avatar-7', 'avatar-4', 'avatar-1', 'avatar-6']
      .map((id, index) => <Avatar key={id} id={id} label={`Agent ${index + 1}`} />)}
  </div>,
};
function FailedImageExample() {
  const [src, setSrc] = useState('/brand/aib-mark.svg');
  return <div className="flex items-center gap-3"><Avatar src={src} label="Inventory agent" id="inventory-agent" />
    <Button variant="outline" onClick={() => setSrc('/brand/missing-avatar.png')}>Load unavailable image</Button></div>;
}
export const Error: Story = {
  render: () => <FailedImageExample />,
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await userEvent.click(canvas.getByRole('button', { name: 'Load unavailable image' }));
    await waitFor(() => expect(canvasElement.querySelector('img')).toBeNull());
    await expect(canvas.getByRole('img', { name: 'Inventory agent' })).toHaveTextContent('I');
  },
};
