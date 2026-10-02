import { useState } from 'react';
import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { expect, it } from 'vitest';
import { Select, SelectTrigger, SelectValue, SelectContent, SelectItem } from './Select';

it('selects with the keyboard, skips disabled options, and restores trigger focus', async () => {
  const user = userEvent.setup();
  function Example() {
    const [value, setValue] = useState('any');
    return <Select value={value} onValueChange={setValue}><SelectTrigger aria-label="Match mode"><SelectValue /></SelectTrigger><SelectContent><SelectItem value="any">Any</SelectItem><SelectItem value="exact" disabled>Exact</SelectItem><SelectItem value="all">All</SelectItem></SelectContent></Select>;
  }
  render(<Example />);
  await user.tab();
  await user.keyboard('{Enter}');
  expect(await screen.findByRole('listbox')).toBeVisible();
  await user.keyboard('{ArrowDown}{Enter}');
  expect(screen.getByRole('combobox', { name: 'Match mode' })).toHaveTextContent('All');
  expect(screen.queryByRole('listbox')).not.toBeInTheDocument();
  expect(screen.getByRole('combobox')).toHaveFocus();
  await user.keyboard('{Enter}{Escape}');
  expect(screen.queryByRole('listbox')).not.toBeInTheDocument();
  expect(screen.getByRole('combobox')).toHaveFocus();
});
