import type { Meta, StoryObj } from '@storybook/react';
import { expect, userEvent, within } from 'storybook/test';
import { Check, CircleAlert } from 'lucide-react';
import { Badge } from './Badge';

const meta = {
  title: 'Design System/Primitives/Badge', component: Badge,
  parameters: { layout: 'centered', a11y: { test: 'error' } },
  args: { children: 'Not connected' },
} satisfies Meta<typeof Badge>;
export default meta;
type Story = StoryObj<typeof meta>;
export const Default: Story = {};
export const Variants: Story = {
  render: () => <div className="flex flex-wrap gap-3">
    <Badge>Not connected</Badge><Badge variant="outline">Optional</Badge>
    <Badge variant="success"><Check aria-hidden="true" />Connected</Badge>
    <Badge variant="warning">Needs review</Badge>
    <Badge variant="danger"><CircleAlert aria-hidden="true" />Expired</Badge>
    <Badge variant="info">Pending approval</Badge>
  </div>,
};
export const Error: Story = { args: { variant: 'danger', children: 'Expired' } };
export const Hover: Story = {
  args: { asChild: true, variant: 'outline', children: <a href="#pending">Pending approvals</a> },
  play: async ({ canvasElement }) => { await userEvent.hover(within(canvasElement).getByRole('link')); },
};
export const FocusVisible: Story = {
  args: { asChild: true, variant: 'outline', children: <a href="#pending">Pending approvals</a> },
  play: async ({ canvasElement }) => {
    await userEvent.tab();
    await expect(within(canvasElement).getByRole('link')).toHaveFocus();
  },
};
