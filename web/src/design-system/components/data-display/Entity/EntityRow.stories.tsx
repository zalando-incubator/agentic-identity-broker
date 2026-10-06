import type { Meta, StoryObj } from '@storybook/react';
import { MemoryRouter } from 'react-router-dom';
import { expect, fn, userEvent, waitFor, within } from 'storybook/test';
import { Badge } from '@design-system/components/primitives/Badge';
import { Button } from '@design-system/components/primitives/Button';
import { EntityRow } from './Entity';

const onAction = fn();
const meta = {
  title: 'Design System/Data Display/EntityRow',
  component: EntityRow,
  parameters: { layout: 'padded', a11y: { test: 'error' } },
  tags: ['autodocs'],
  decorators: [(Story) => <MemoryRouter><div className="max-w-5xl"><Story /></div></MemoryRouter>],
  args: {
    id: 'sample-approval',
    name: 'Review calendar event',
    supporting: 'Documentation agent',
    meta: 'Expires in 4 min',
    status: <Badge variant="risk-medium">Medium risk</Badge>,
    onSelect: fn(),
    actions: <Button variant="outline" size="sm" onClick={onAction}>Details</Button>,
  },
} satisfies Meta<typeof EntityRow>;
export default meta;
type Story = StoryObj<typeof meta>;

export const Default: Story = {};
export const Dark: Story = { globals: { theme: 'dark' } };
export const LongName: Story = {
  args: { name: 'Review calendar event requested by a tool with a name far too long to fit one line' },
  play: async ({ canvasElement }) => {
    canvasElement.style.maxWidth = '375px';
    const canvas = within(canvasElement);
    const row = canvas.getByRole('button', { name: /Review calendar event/ }).closest('[data-slot="entity-row"]')!;
    const height = row.getBoundingClientRect().height;
    await userEvent.hover(canvas.getByRole('button', { name: /Review calendar event/ }));
    await waitFor(() => expect(within(canvasElement.ownerDocument.body).getByRole('tooltip')).toHaveTextContent('Review calendar event requested by a tool with a name far too long to fit one line'));
    await expect(row.getBoundingClientRect().height).toBe(height);
  },
};

export const PassiveLongName: Story = {
  args: {
    onSelect: undefined,
    name: 'Review calendar event requested by a tool with a name far too long to fit one line',
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
    await expect(canvas.getByRole('button', { name: 'Details' })).toHaveFocus();
  },
};

export const Hover: Story = {
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    const row = canvas.getByRole('button', { name: /Review calendar event/ }).closest('[data-slot="entity-row"]')!;
    const height = row.getBoundingClientRect().height;
    await userEvent.hover(canvas.getByRole('button', { name: /Review calendar event/ }));
    await expect(row.getBoundingClientRect().height).toBe(height);
  },
};
export const FocusVisible: Story = {
  decorators: [(Story) => <div style={{ maxWidth: 320 }}><Story /></div>],
  play: async ({ canvasElement }) => {
    await document.fonts.ready;
    await new Promise<void>(resolve => requestAnimationFrame(() => requestAnimationFrame(() => resolve())));
    const canvas = within(canvasElement);
    const button = canvas.getByRole('button', { name: /Review calendar event/ });
    const row = button.closest('[data-slot="entity-row"]')!;
    const before = getComputedStyle(button).boxShadow;
    const rowBefore = getComputedStyle(row).boxShadow;
    await userEvent.tab();
    await expect(button).toHaveFocus();
    await expect(getComputedStyle(button).boxShadow).not.toBe(before);
    const rect = button.getBoundingClientRect();
    await expect(button.contains(canvasElement.ownerDocument.elementFromPoint((rect.left + rect.right) / 2, (rect.top + rect.bottom) / 2))).toBe(true);
    const details = canvas.getByRole('button', { name: 'Details' });
    await userEvent.click(details);
    await expect(details).toHaveFocus();
    await expect(getComputedStyle(row).boxShadow).toBe(rowBefore);
    await waitFor(() => expect(canvasElement.ownerDocument.querySelector('[data-slot="tooltip-content"]')).not.toBeInTheDocument());
  },
};
export const Selected: Story = { args: { selected: true } };
export const Busy: Story = {
  args: { busy: true },
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await expect(canvas.getByText('Review calendar event').closest('[data-slot="entity-row"]')).toHaveAttribute('aria-busy', 'true');
    await expect(canvas.getByRole('button', { name: 'Details' })).toBeDisabled();
  },
};
export const UnavailableButton: Story = { args: { actions: <Button variant="outline" size="sm" disabled>Details</Button> } };
export const IsolatedAction: Story = {
  play: async ({ canvasElement, args }) => {
    onAction.mockClear();
    const canvas = within(canvasElement);
    const row = canvas.getByRole('button', { name: /Review calendar event/ }).closest('[data-slot="entity-row"]')!;
    const height = row.getBoundingClientRect().height;
    await userEvent.click(canvas.getByRole('button', { name: 'Details' }));
    await expect(onAction).toHaveBeenCalledOnce();
    await expect(args.onSelect).not.toHaveBeenCalled();
    await expect(row.getBoundingClientRect().height).toBe(height);
  },
};

export const WideSlots: Story = {
  globals: { viewport: { value: 'desktop', isRotated: false } },
  play: async ({ canvasElement }) => {
    const row = within(canvasElement).getByRole('button', { name: /Review calendar event/ }).closest('[data-slot="entity-row"]')!;
    const slots = ['entity-identity', 'entity-status', 'entity-supporting', 'entity-meta'].map(slot => row.querySelector(`[data-slot="${slot}"]`)!.getBoundingClientRect());
    for (let index = 0; index < slots.length - 1; index++) {
      await expect(slots[index]!.right).toBeLessThanOrEqual(slots[index + 1]!.left);
    }
    await expect(slots.at(-1)!.right).toBeLessThanOrEqual(within(row).getByRole('button', { name: 'Details' }).getBoundingClientRect().left);
    await expect(row.getBoundingClientRect().height).toBe(88);
  },
};

export const NarrowActions: Story = {
  globals: { viewport: { value: 'narrow320', isRotated: false } },
  play: async ({ canvasElement, args }) => {
    onAction.mockClear();
    const canvas = within(canvasElement);
    const row = canvas.getByRole('button', { name: /Review calendar event/ }).closest('[data-slot="entity-row"]')!;
    const action = within(row).getByRole('button', { name: 'Details' });
    await expect(action.getBoundingClientRect().right).toBeLessThanOrEqual(row.getBoundingClientRect().right);
    await expect(document.documentElement.scrollWidth).toBeLessThanOrEqual(window.innerWidth);
    await userEvent.click(action);
    await expect(onAction).toHaveBeenCalledOnce();
    await expect(args.onSelect).not.toHaveBeenCalled();
  },
};

export const ColumnsWithoutStatus: Story = {
  args: {
    status: undefined,
    columnTemplate: 'xl:grid-cols-[minmax(0,1fr)_minmax(0,1fr)_minmax(0,1fr)]',
    supporting: 'Until revoked',
    meta: 'Changed 2 hours ago',
  },
  globals: { viewport: { value: 'narrow320', isRotated: false } },
  play: async ({ canvasElement }) => {
    const row = within(canvasElement).getByRole('button', { name: 'Review calendar event' }).closest('[data-slot="entity-row"]')!;
    const supporting = row.querySelector('[data-slot="entity-supporting"]')!;
    const meta = row.querySelector('[data-slot="entity-meta"]')!;
    const action = within(row).getByRole('button', { name: 'Details' });
    await expect(meta.getBoundingClientRect().top).toBeGreaterThanOrEqual(supporting.getBoundingClientRect().bottom);
    await expect(action.getBoundingClientRect().top).toBeGreaterThanOrEqual(meta.getBoundingClientRect().bottom);
    await expect(action.getBoundingClientRect().right).toBeLessThanOrEqual(row.getBoundingClientRect().right);
    await expect(action.getBoundingClientRect().bottom).toBeLessThanOrEqual(row.getBoundingClientRect().bottom);
  },
};

export const TabletColumnsWithoutStatus: Story = {
  ...ColumnsWithoutStatus,
  globals: { viewport: { value: 'tablet', isRotated: false } },
  play: async ({ canvasElement }) => {
    const row = within(canvasElement).getByRole('button', { name: 'Review calendar event' }).closest('[data-slot="entity-row"]')!;
    const supporting = row.querySelector('[data-slot="entity-supporting"]')!;
    const meta = row.querySelector('[data-slot="entity-meta"]')!;
    const action = within(row).getByRole('button', { name: 'Details' });
    await expect(meta.getBoundingClientRect().left).toBeGreaterThanOrEqual(supporting.getBoundingClientRect().right);
    await expect(action.getBoundingClientRect().left).toBeGreaterThanOrEqual(meta.getBoundingClientRect().right);
    await expect(action.getBoundingClientRect().bottom).toBeLessThanOrEqual(row.getBoundingClientRect().bottom);
  },
};

export const StackedApproval: Story = {
  args: {
    rowLayout: 'stacked',
    className: 'max-w-[28rem]',
    selected: true,
    name: 'document_repository.collect_signed_vendor_agreements_for_compliance_case',
    supporting: 'Documentation assistant',
    status: <Badge variant="risk-medium">Medium risk</Badge>,
    meta: '4 min left',
    actions: undefined,
  },
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    const control = canvas.getByRole('button', { name: 'document_repository.collect_signed_vendor_agreements_for_compliance_case' });
    const row = control.closest('[data-slot="entity-row"]')!;
    const identity = row.querySelector<HTMLElement>('[data-slot="entity-identity"]')!;
    await expect(row).toHaveAttribute('data-layout', 'stacked');
    await expect(row.getBoundingClientRect().height).toBe(88);
    await expect(identity).toContainElement(canvas.getByTestId('entity-name'));
    await expect(identity).toContainElement(row.querySelector<HTMLElement>('[data-slot="entity-status"]'));
    await expect(canvas.getByTestId('entity-name').getBoundingClientRect().width).toBeGreaterThan(160);
  },
};
