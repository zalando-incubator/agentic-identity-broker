import type { Meta, StoryObj } from '@storybook/react';
import { expect, fn, userEvent, within } from 'storybook/test';
import { ArrowRight, Trash2, Unplug } from 'lucide-react';
import { Button } from './Button';

const meta = {
  title: 'Design System/Primitives/Button',
  component: Button,
  parameters: { layout: 'centered', a11y: { test: 'error' } },
  args: { children: 'Review access', onClick: fn() },
} satisfies Meta<typeof Button>;
export default meta;
type Story = StoryObj<typeof meta>;

export const Default: Story = {};
export const Variants: Story = {
  render: () => <div className="flex flex-wrap gap-3">
    <Button>Allow</Button><Button variant="secondary">Deny</Button>
    <Button variant="outline">Cancel</Button><Button variant="ghost" size="icon" aria-label="Details" title="Details"><ArrowRight aria-hidden="true" /></Button>
    <Button variant="destructive-outline"><Trash2 aria-hidden="true" />Revoke</Button>
    <Button variant="destructive-quiet"><Unplug aria-hidden="true" />Disconnect</Button>
    <Button variant="destructive"><Trash2 aria-hidden="true" />Confirm revoke</Button>
  </div>,
};
export const Sizes: Story = {
  render: () => <div className="flex flex-wrap items-center gap-3">
    <Button size="sm" variant="outline">Small</Button><Button variant="outline">Default</Button>
    <Button size="lg" variant="outline">Large</Button>
    <Button size="icon" variant="ghost" aria-label="Continue"><ArrowRight aria-hidden="true" /></Button>
  </div>,
};
export const Hover: Story = {
  play: async ({ canvasElement }) => { await userEvent.hover(within(canvasElement).getByRole('button')); },
};
export const FocusVisible: Story = {
  play: async ({ canvasElement, args }) => {
    await userEvent.tab();
    await expect(within(canvasElement).getByRole('button')).toHaveFocus();
    await userEvent.keyboard('{Enter}');
    await expect(args.onClick).toHaveBeenCalledOnce();
  },
};
export const Disabled: Story = {
  args: { disabled: true },
  play: async ({ canvasElement, args }) => {
    const button = within(canvasElement).getByRole('button');
    await expect(button).toBeDisabled();
    await userEvent.click(button);
    await expect(args.onClick).not.toHaveBeenCalled();
  },
};
export const Loading: Story = {
  args: { isLoading: true, children: 'Saving access' },
  play: async ({ canvasElement, args }) => {
    const button = within(canvasElement).getByRole('button');
    await expect(button).toHaveAttribute('aria-busy', 'true');
    await userEvent.click(button);
    await expect(args.onClick).not.toHaveBeenCalled();
  },
};
export const Destructive: Story = { args: { variant: 'destructive', children: 'Revoke access' } };
export const DestructiveOutline: Story = { args: { variant: 'destructive-outline', children: 'Revoke access' } };
export const DestructiveQuiet: Story = { args: { variant: 'destructive-quiet', children: <><Unplug aria-hidden="true" />Disconnect</> } };
export const Link: Story = {
  args: { asChild: true, variant: 'outline', children: <a href="#agent-details">Agent details</a> },
  play: async ({ canvasElement }) => {
    await userEvent.tab();
    await expect(within(canvasElement).getByRole('link', { name: 'Agent details' })).toHaveFocus();
  },
};
