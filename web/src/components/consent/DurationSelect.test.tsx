import { useState } from 'react';
import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { expect, it } from 'vitest';
import { createConsentDraft } from './consentDraft';
import type { ConsentDraft } from './consentDraft';
import { DurationSelect } from './DurationSelect';

const dateFormatter = new Intl.DateTimeFormat(undefined, { day: 'numeric', month: 'short', year: 'numeric' });

function DurationFixture({ initial }: { initial: ConsentDraft }) {
  const [draft, setDraft] = useState(initial);
  return <DurationSelect draft={draft} onChange={setDraft} />;
}

it('opens the Radix duration options on the first action and restores combobox focus on Escape', async () => {
  const user = userEvent.setup();
  render(<DurationFixture initial={createConsentDraft({ context: 'decision', permissionSets: [] })} />);
  const trigger = screen.getByRole('combobox', { name: 'Access lasts' });
  expect(trigger).toHaveTextContent('Until I revoke it');
  await user.click(trigger);
  expect(await screen.findByRole('listbox')).toBeVisible();
  await user.click(screen.getByRole('option', { name: '30 days' }));
  expect(screen.getByRole('combobox', { name: 'Access lasts' })).toHaveTextContent('30 days');
  await waitFor(() => expect(screen.getByRole('combobox', { name: 'Access lasts' })).toHaveFocus());
  await user.keyboard('{Enter}');
  expect(await screen.findByRole('listbox')).toBeVisible();
  await user.keyboard('{Escape}');
  await waitFor(() => expect(screen.getByRole('combobox', { name: 'Access lasts' })).toHaveFocus());
});

it('keeps the chosen calendar date visible after closing and reopens the same selection', async () => {
  const user = userEvent.setup();
  render(<DurationFixture initial={createConsentDraft({ context: 'decision', permissionSets: [] }).setDuration('custom', '2099-06-10')} />);
  const trigger = screen.getByRole('button', { name: 'Choose custom date' });
  expect(trigger).toHaveTextContent(dateFormatter.format(new Date(2099, 5, 10)));
  expect(trigger).toHaveAccessibleDescription(dateFormatter.format(new Date(2099, 5, 10)));
  await user.click(trigger);
  const date = await screen.findByLabelText('Custom date', { selector: 'input' });
  expect(date).toHaveValue('2099-06-10');
  expect(date).toHaveFocus();
  fireEvent.change(date, { target: { value: '2099-10-10' } });
  expect(date).toHaveValue('2099-10-10');
  await user.keyboard('{Escape}');
  await waitFor(() => expect(trigger).toHaveFocus());
  await waitFor(() => expect(screen.queryByLabelText('Custom date', { selector: 'input' })).not.toBeInTheDocument());
  expect(trigger).toHaveTextContent(dateFormatter.format(new Date(2099, 9, 10)));
  expect(trigger).toHaveAccessibleDescription(dateFormatter.format(new Date(2099, 9, 10)));
  expect(screen.getByText(dateFormatter.format(new Date(2099, 9, 10)))).toBeVisible();
  await user.click(trigger);
  expect(await screen.findByLabelText('Custom date', { selector: 'input' })).toHaveValue('2099-10-10');
});

it.each(['', 'invalid', '2099', '2099-02-30'])('does not format an incomplete or invalid custom date: %s', (customDate) => {
  render(<DurationFixture initial={createConsentDraft({ context: 'decision', permissionSets: [] }).setDuration('custom', customDate)} />);
  expect(screen.getByRole('button', { name: 'Choose custom date' })).toHaveTextContent(/^$/);
});
