import type { Meta, StoryObj } from '@storybook/react';
import { expect, userEvent, waitFor, within } from 'storybook/test';
import { TruncatedText } from './TruncatedText';

const meta = {
  title: 'Design System/Data Display/TruncatedText',
  component: TruncatedText,
  parameters: { layout: 'padded', a11y: { test: 'error' } },
  args: {
    text: 'An unusually long service identity with enough metadata to wrap across several lines and remain fully readable when expanded.',
    lines: 2,
    expandLabel: 'More',
    collapseLabel: 'Less',
    className: 'max-w-64',
  },
} satisfies Meta<typeof TruncatedText>;
export default meta;
type Story = StoryObj<typeof meta>;

export const Default: Story = {};
export const Fits: Story = {
  args: { text: 'Calendar assistant' },
  play: async ({ canvasElement }) => {
    await document.fonts.ready;
    await expect(within(canvasElement).queryByRole('button')).not.toBeInTheDocument();
  },
};
export const SingleLine: Story = {
  args: { lines: 1 },
  play: async ({ canvasElement, args }) => {
    const canvas = within(canvasElement);
    await expect(canvas.queryByRole('button')).not.toBeInTheDocument();
    const name = canvas.getByText(args.text);
    await waitFor(() => expect(name).toHaveAttribute('tabindex', '0'));
    name.focus();
    await expect(await within(document.body).findByRole('tooltip')).toHaveTextContent(args.text);
  },
};
export const Hover: Story = {
  play: async ({ canvasElement }) => {
    await userEvent.hover(await within(canvasElement).findByRole('button', { name: 'More' }));
  },
};
export const FocusVisible: Story = {
  play: async ({ canvasElement }) => {
    const toggle = await within(canvasElement).findByRole('button', { name: 'More' });
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
    const toggle = await canvas.findByRole('button', { name: 'More' });
    const content = document.getElementById(toggle.getAttribute('aria-controls')!)!;
    const height = content.getBoundingClientRect().height;
    await userEvent.click(toggle);
    await expect(content.getBoundingClientRect().height).toBeGreaterThan(height);
    await expect(canvas.getByRole('button', { name: 'Less' })).toHaveAttribute('aria-expanded', 'true');
  },
};
export const UntrustedMetadata: Story = {
  args: { text: '<img src="https://untrusted.invalid/image" onerror="alert(1)">This is text, not markup.' },
  play: async ({ canvasElement }) => {
    await expect(canvasElement.querySelector('img, script')).toBeNull();
    await expect(within(canvasElement).getByText(/This is text, not markup\./)).toBeVisible();
  },
};
export const IdentityHeading: Story = {
  args: { as: 'h1', text: 'Research assistant with an exceptionally detailed display name', lines: 1, className: 'max-w-64 font-display text-xl font-semibold' },
  play: async ({ canvasElement, args }) => {
    await expect(within(canvasElement).getByRole('heading', { level: 1, name: args.text })).toHaveAccessibleName(args.text);
    await expect(within(canvasElement).queryByRole('button')).not.toBeInTheDocument();
  },
};
