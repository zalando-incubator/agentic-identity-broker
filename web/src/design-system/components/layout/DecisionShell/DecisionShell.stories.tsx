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
    wordmarkLabel: 'Agentic Identity Broker', skipToMainLabel: 'Skip to main content', footerLabel: 'Powered by Zalando',
    children: <div className="space-y-6"><h1 className="font-display text-3xl font-semibold">Review agent access</h1><p>Choose whether this agent can use the access you selected.</p><div className="flex flex-wrap gap-3"><Button>Allow</Button><Button variant="secondary">Deny</Button></div></div>,
  },
} satisfies Meta<typeof DecisionShell>;
export default meta;
type Story = StoryObj<typeof meta>;

export const Desktop: Story = {
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await expect(canvas.queryByRole('navigation')).not.toBeInTheDocument();
    await expect(canvas.getByRole('main').getBoundingClientRect().width).toBeLessThanOrEqual(640);
  },
};
export const WithoutFooter: Story = { args: { footerLabel: undefined } };
export const Narrow: Story = {
  globals: { viewport: { value: 'narrow320', isRotated: false } },
  play: async ({ canvasElement }) => {
    const document = canvasElement.ownerDocument;
    await expect(document.defaultView!.innerWidth).toBe(320);
    await expect(document.documentElement.scrollWidth).toBeLessThanOrEqual(320);
    for (const button of within(canvasElement).getAllByRole('button')) await expect(button).toBeVisible();
  },
};
export const KeyboardSkip: Story = {
  play: async ({ canvasElement }) => {
    await userEvent.tab();
    await userEvent.keyboard('{Enter}');
    await expect(within(canvasElement).getByRole('main')).toHaveFocus();
    await userEvent.tab();
    await expect(within(canvasElement).getByRole('button', { name: 'Allow' })).toHaveFocus();
  },
};
