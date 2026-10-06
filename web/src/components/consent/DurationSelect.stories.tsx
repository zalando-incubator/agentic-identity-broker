import { useState } from 'react';
import type { Meta, StoryObj } from '@storybook/react';
import { expect, fn, userEvent, waitFor, within } from 'storybook/test';
import { consentCopy } from '@copy/consent';
import { consentDetail, consentGrant } from '../../storybook/fixtures';
import { createConsentDraft } from './consentDraft';
import { DurationSelect, type DurationSelectProps } from './DurationSelect';

const indefinite = createConsentDraft({ context: 'decision', permissionSets: consentDetail.agent.permission_sets });
const custom = createConsentDraft({ context: 'decision', permissionSets: consentDetail.agent.permission_sets, existingGrant: consentGrant });

function InteractiveDuration(args: DurationSelectProps) {
  const [draft, setDraft] = useState(args.draft);
  return <div className="max-w-md"><DurationSelect {...args} draft={draft} onChange={(next) => { setDraft(next); args.onChange(next); }} /></div>;
}

const meta = {
  title: 'Patterns/Duration select', component: DurationSelect,
  tags: ['autodocs'], parameters: { layout: 'padded', a11y: { test: 'error' } },
  render: (args) => <InteractiveDuration {...args} />,
  args: { draft: indefinite, onChange: fn() },
} satisfies Meta<typeof DurationSelect>;
export default meta;
type Story = StoryObj<typeof DurationSelect>;

export const UntilRevoked: Story = {
  play: async ({ canvasElement }) => {
    const trigger = within(canvasElement).getByRole('combobox', { name: 'Access lasts' });
    await expect(trigger).toHaveTextContent('Until I revoke it');
    await userEvent.click(trigger);
    await userEvent.click(await within(canvasElement.ownerDocument.body).findByRole('option', { name: '30 days' }));
    await expect(within(canvasElement).getByRole('combobox', { name: 'Access lasts' })).toHaveTextContent('30 days');
  },
};
export const ThirtyDays: Story = { args: { draft: indefinite.setDuration('30-days') } };
export const CustomDate: Story = {
  args: { draft: custom.setDuration('custom', '2099-10-10') },
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    const trigger = canvas.getByRole('button', { name: 'Choose custom date' });
    const selectedDate = new Intl.DateTimeFormat(undefined, { day: 'numeric', month: 'short', year: 'numeric' }).format(new Date(2099, 9, 10));
    await expect(canvas.getByText(selectedDate)).toBeVisible();
    await userEvent.click(trigger);
    await expect(await within(canvasElement.ownerDocument.body).findByLabelText('Custom date', { selector: 'input' })).toHaveValue('2099-10-10');
    await userEvent.keyboard('{Escape}');
    await waitFor(() => expect(trigger).toHaveFocus());
    await expect(canvas.getByText(selectedDate)).toBeVisible();
  },
};
export const OpenCustomDate: Story = {
  args: { draft: indefinite.setDuration('custom') },
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    const trigger = canvas.getByRole('button', { name: 'Choose custom date' });
    const selector = canvas.getByRole('combobox', { name: 'Access lasts' });
    await expect(trigger.getBoundingClientRect().height).toBe(selector.getBoundingClientRect().height);
    await userEvent.click(trigger);
    const dialog = await within(canvasElement.ownerDocument.body).findByRole('dialog', { name: 'Custom date' });
    const input = within(dialog).getByLabelText('Custom date', { selector: 'input' });
    await waitFor(() => expect(input).toHaveFocus());
    await waitFor(() => {
      const bounds = dialog.getBoundingClientRect();
      expect(bounds.left).toBeGreaterThanOrEqual(16);
      expect(bounds.right).toBeLessThanOrEqual(window.innerWidth - 16);
      expect(bounds.top).toBeGreaterThanOrEqual(16);
      expect(bounds.bottom).toBeLessThanOrEqual(window.innerHeight - 16);
    });
  },
};
export const InvalidDate: Story = { args: { draft: custom.setDuration('custom', '2020-01-01'), error: consentCopy.invalidDate } };
export const Disabled: Story = { args: { disabled: true } };
