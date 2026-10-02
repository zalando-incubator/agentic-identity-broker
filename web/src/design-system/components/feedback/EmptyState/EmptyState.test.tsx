import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { expect, it, vi } from 'vitest';
import { EmptyState } from './EmptyState';

it('keeps caller-supplied recovery controls usable with optional branding', async () => {
  const onClear = vi.fn();
  render(<EmptyState {...{ title: 'No matching connections', wordmark: <span role="img" aria-label="Account console" /> }}><button onClick={onClear}>Clear filters</button></EmptyState>);
  expect(screen.getByRole('img', { name: 'Account console' })).toBeVisible();
  await userEvent.click(screen.getByRole('button', { name: 'Clear filters' }));
  expect(onClear).toHaveBeenCalledTimes(1);
});
