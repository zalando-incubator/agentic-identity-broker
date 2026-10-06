import type { Meta, StoryObj } from '@storybook/react';
import { Link, MemoryRouter } from 'react-router-dom';
import { expect, fn, userEvent, waitFor, within } from 'storybook/test';
import { Badge } from '@design-system/components/primitives/Badge';
import { Button } from '@design-system/components/primitives/Button';
import { EntityCard } from './Entity';

const meta = {
  title: 'Design System/Data Display/EntityCard',
  component: EntityCard,
  parameters: { layout: 'padded', a11y: { test: 'error' } },
  tags: ['autodocs'],
  decorators: [(Story) => <MemoryRouter><div className="max-w-sm"><Story /></div></MemoryRouter>],
  args: {
    id: 'sample-agent',
    name: 'Documentation agent',
    href: '/agents/sample-agent',
    status: <Badge variant="success">Active</Badge>,
    supporting: 'Access until revoked',
    meta: 'Last updated 3 Oct 2026',
    actions: <Button variant="destructive-outline" size="sm" onClick={fn()}>Revoke</Button>,
  },
} satisfies Meta<typeof EntityCard>;
export default meta;
type Story = StoryObj<typeof meta>;

export const Default: Story = {};
export const Dark: Story = { globals: { theme: 'dark' } };
export const LongName: Story = {
  args: { name: 'Documentation agent with an exceptionally descriptive eighty-character display name that never wraps' },
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    const card = canvas.getByRole('link').closest('[data-slot="entity-card"]')!;
    const height = card.getBoundingClientRect().height;
    await userEvent.hover(canvas.getByRole('link'));
    await waitFor(() => expect(within(canvasElement.ownerDocument.body).getByRole('tooltip')).toHaveTextContent('Documentation agent with an exceptionally descriptive eighty-character display name that never wraps'));
    await expect(card.getBoundingClientRect().height).toBe(height);
  },
};

export const PassiveLongName: Story = {
  args: {
    href: undefined,
    name: 'Documentation agent with an exceptionally descriptive eighty-character display name that never wraps',
    meta: <Button variant="ghost" size="sm" className="h-auto px-0 py-0">View scope</Button>,
  },
  play: async ({ canvasElement }) => {
    canvasElement.style.maxWidth = '320px';
    const canvas = within(canvasElement);
    const title = canvas.getByTestId('entity-name');
    const name = within(title).getByText(title.textContent!);
    await waitFor(() => expect(name).toHaveAttribute('tabindex', '0'));
    await userEvent.tab();
    await expect(name).toHaveFocus();
    await waitFor(() => expect(within(canvasElement.ownerDocument.body).getByRole('tooltip')).toHaveTextContent(name.textContent!));
    await userEvent.tab();
    await expect(canvas.getByRole('button', { name: 'View scope' })).toHaveFocus();
    await userEvent.tab();
    await expect(canvas.getByRole('button', { name: 'Revoke' })).toHaveFocus();
  },
};

export const Hover: Story = {
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    const card = canvas.getByRole('link').closest('[data-slot="entity-card"]')!;
    const height = card.getBoundingClientRect().height;
    await userEvent.hover(canvas.getByRole('link'));
    await expect(card.getBoundingClientRect().height).toBe(height);
  },
};
export const FocusVisible: Story = {
  play: async ({ canvasElement }) => {
    canvasElement.style.maxWidth = '320px';
    const canvas = within(canvasElement);
    const link = canvas.getByRole('link');
    const card = link.closest('[data-slot="entity-card"]')!;
    const before = getComputedStyle(link).boxShadow;
    const cardBefore = getComputedStyle(card).boxShadow;
    await userEvent.tab();
    await expect(link).toHaveFocus();
    await expect(getComputedStyle(link).boxShadow).not.toBe(before);
    const rect = link.getBoundingClientRect();
    await expect(link.contains(canvasElement.ownerDocument.elementFromPoint((rect.left + rect.right) / 2, (rect.top + rect.bottom) / 2))).toBe(true);
    await userEvent.click(canvas.getByRole('button', { name: 'Revoke' }));
    await expect(getComputedStyle(card).boxShadow).toBe(cardBefore);
  },
};
export const Selected: Story = { args: { selected: true } };
export const Busy: Story = {
  args: { busy: true },
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await expect(canvas.getByText('Documentation agent').closest('[data-slot="entity-card"]')).toHaveAttribute('aria-busy', 'true');
    await expect(canvas.queryByRole('link')).not.toBeInTheDocument();
    await expect(canvas.getByRole('button', { name: 'Revoke' })).toBeDisabled();
  },
};
export const UnavailableButton: Story = { args: { actions: <Button variant="destructive-outline" size="sm" disabled>Revoke</Button> } };
export const SelectWithoutLink: Story = {
  args: { href: undefined, onSelect: fn(), selected: true },
  play: async ({ canvasElement, args }) => {
    await userEvent.click(within(canvasElement).getByRole('button', { name: /Documentation agent/ }));
    await expect(args.onSelect).toHaveBeenCalledOnce();
  },
};
export const AnchoredStatus: Story = {
  args: {
    status: <Badge variant="warning">Needs sign-in</Badge>,
    supporting: <span className="block whitespace-normal">Sign in again to continue.</span>,
  },
  globals: { viewport: { value: 'narrow320', isRotated: false } },
  play: async ({ canvasElement }) => {
    const card = within(canvasElement).getByRole('link').closest('[data-slot="entity-card"]')!;
    const title = within(card).getByTestId('entity-name').getBoundingClientRect();
    const badge = card.querySelector('[data-slot="entity-status"]')!.getBoundingClientRect();
    const explanation = card.querySelector('[data-slot="entity-supporting"]')!.getBoundingClientRect();
    const meta = card.querySelector('[data-slot="entity-meta"]')!.getBoundingClientRect();
    const footer = card.querySelector('fieldset')!.getBoundingClientRect();
    await expect(badge.top).toBeGreaterThanOrEqual(title.bottom);
    await expect(badge.left).toBeGreaterThanOrEqual(title.left);
    await expect(explanation.top).toBeGreaterThanOrEqual(badge.bottom);
    await expect(meta.top).toBeGreaterThanOrEqual(explanation.bottom);
    await expect(footer.top).toBeGreaterThanOrEqual(meta.bottom);
    await expect(card.scrollHeight).toBeLessThanOrEqual(card.clientHeight);
    await expect(document.documentElement.scrollWidth).toBeLessThanOrEqual(window.innerWidth);
  },
};

export const CompactCard: Story = {
  args: {
    cardSize: 'compact', className: 'h-[120px]', status: undefined, supporting: 'Expires 8 Oct 2026', meta: 'Changed 2 hours ago',
    actions: <>
      <Button asChild variant="outline" size="sm"><Link to="/agents/sample-agent" aria-label="Details for Documentation agent">Details</Link></Button>
      <Button variant="destructive-quiet" size="sm" onClick={fn()}>Revoke</Button>
    </>,
  },
  globals: { viewport: { value: 'mobile', isRotated: false } },
  play: async ({ canvasElement }) => {
    const card = within(canvasElement).getByRole('link', { name: 'Documentation agent' }).closest('[data-slot="entity-card"]')!;
    const details = within(card).getByRole('link', { name: 'Details for Documentation agent' });
    const action = within(card).getByRole('button', { name: 'Revoke' });
    const meta = card.querySelector('[data-slot="entity-meta"]')!;
    await expect(details.getBoundingClientRect().top).toBeGreaterThanOrEqual(meta.getBoundingClientRect().bottom);
    await expect(action.getBoundingClientRect().top).toBeGreaterThanOrEqual(meta.getBoundingClientRect().bottom);
    await expect(action.getBoundingClientRect().bottom).toBeLessThanOrEqual(card.getBoundingClientRect().bottom);
  },
};

export const CompactCardDark: Story = {
  ...CompactCard,
  globals: { theme: 'dark', viewport: { value: 'mobile', isRotated: false } },
};
