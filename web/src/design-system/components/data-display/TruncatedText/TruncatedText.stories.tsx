import type { Meta, StoryObj } from '@storybook/react';
import { expect, userEvent, within } from 'storybook/test';
import { TruncatedText } from './TruncatedText';

const meta = {
  title: 'Design System/Data Display/TruncatedText',
  component: TruncatedText,
  parameters: { layout: 'padded', a11y: { test: 'error' } },
  args: {
    text: 'An unusually long service identity with enough metadata to wrap across several lines and remain fully readable when expanded.',
    lines: 2,
    expandLabel: 'Show full identity',
    collapseLabel: 'Show less',
    className: 'max-w-64',
  },
} satisfies Meta<typeof TruncatedText>;
export default meta;
type Story = StoryObj<typeof meta>;

export const Default: Story = {};
export const SingleLine: Story = {
  args: { lines: 1 },
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    const toggle = canvas.getByRole('button', { name: 'Show full identity' });
    const content = canvasElement.ownerDocument.getElementById(toggle.getAttribute('aria-controls')!)!;
    const collapsedHeight = content.getBoundingClientRect().height;
    await userEvent.click(toggle);
    await expect(content.getBoundingClientRect().height).toBeGreaterThan(collapsedHeight);
    await userEvent.click(canvas.getByRole('button', { name: 'Show less' }));
    await expect(content.getBoundingClientRect().height).toBe(collapsedHeight);
  },
};
export const Hover: Story = {
  play: async ({ canvasElement }) => {
    await userEvent.hover(within(canvasElement).getByRole('button', { name: 'Show full identity' }));
  },
};
export const FocusVisible: Story = {
  play: async ({ canvasElement }) => {
    const toggle = within(canvasElement).getByRole('button', { name: 'Show full identity' });
    toggle.focus();
    await userEvent.keyboard('{Enter}');
    await userEvent.keyboard('{Enter}');
    await expect(toggle).toHaveFocus();
    await expect(toggle).toHaveAttribute('aria-expanded', 'false');
  },
};
export const Expanded: Story = {
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await userEvent.click(canvas.getByRole('button', { name: 'Show full identity' }));
    await expect(canvas.getByRole('button', { name: 'Show less' })).toHaveAttribute('aria-expanded', 'true');
  },
};
export const UntrustedMetadata: Story = {
  args: { text: '<img src="https://untrusted.invalid/image" onerror="alert(1)">This is text, not markup.' },
  play: async ({ canvasElement }) => {
    await userEvent.click(within(canvasElement).getByRole('button', { name: 'Show full identity' }));
    await expect(canvasElement.querySelector('img, script')).toBeNull();
    await expect(within(canvasElement).getByText(/This is text, not markup\./)).toBeVisible();
  },
};

export const IdentityHeading: Story = {
  args: { as: 'h1', text: 'Research assistant with an exceptionally detailed display name', className: 'max-w-64 font-display text-2xl font-semibold' },
  play: async ({ canvasElement, args }) => {
    const canvas = within(canvasElement);
    const heading = canvas.getByRole('heading', { level: 1, name: args.text });
    await userEvent.click(canvas.getByRole('button', { name: 'Show full identity' }));
    await expect(heading).toHaveAccessibleName(args.text);
    await expect(heading).not.toContainElement(canvas.getByRole('button', { name: 'Show less' }));
  },
};
