import type { Meta, StoryObj } from '@storybook/react';
import { expect, userEvent, within } from 'storybook/test';
import { Button } from '@design-system/components/primitives/Button/Button';
import { DecisionShell } from './DecisionShell';

const meta = {
  title: 'Design System/Layout/DecisionShell',
  component: DecisionShell,
  tags: ['autodocs'],
  parameters: { layout: 'fullscreen', a11y: { test: 'error' } },
  args: {
    wordmarkLabel: 'Agentic Identity Broker', skipToMainLabel: 'Skip to main content',
    compact: true,
    children: <div className="space-y-6"><h1 className="font-display text-2xl font-semibold">Review agent access</h1><p>Choose whether this agent can use the access you selected.</p><div className="flex flex-wrap gap-3"><Button variant="outline">Deny</Button><Button>Allow</Button></div></div>,
  },
} satisfies Meta<typeof DecisionShell>;
export default meta;
type Story = StoryObj<typeof meta>;

export const Desktop: Story = {
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await expect(canvas.queryByRole('navigation')).not.toBeInTheDocument();
    await expect(canvas.getByRole('main').getBoundingClientRect().width).toBeLessThanOrEqual(512);
    await expect(canvas.getByRole('main').firstElementChild!.getBoundingClientRect().width).toBeLessThanOrEqual(480);
  },
};
export const StandardReviewWidth: Story = { args: { compact: false } };
export const Responsive: Story = {
  play: async ({ canvasElement }) => {
    const document = canvasElement.ownerDocument;
    await expect(document.documentElement.scrollWidth).toBeLessThanOrEqual(document.defaultView!.innerWidth);
    for (const button of within(canvasElement).getAllByRole('button')) await expect(button).toBeVisible();
  },
};
export const KeyboardSkip: Story = {
  play: async ({ canvasElement }) => {
    await userEvent.tab();
    await userEvent.keyboard('{Enter}');
    await expect(within(canvasElement).getByRole('main')).toHaveFocus();
    await userEvent.tab();
    await expect(within(canvasElement).getByRole('button', { name: 'Deny' })).toHaveFocus();
    await userEvent.tab();
    await expect(within(canvasElement).getByRole('button', { name: 'Allow' })).toHaveFocus();
  },
};
