import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { expect, it, vi } from 'vitest';
import { Alert } from './Alert';

it('announces the error and dismisses it using the caller supplied accessible name', async () => {
  const onDismiss = vi.fn();
  render(<Alert {...{ variant: 'error' as const, dismissible: true, dismissLabel: 'Dismiss connection error', onDismiss }}>Connection failed</Alert>);
  expect(screen.getByRole('alert')).toHaveTextContent('Connection failed');
  await userEvent.click(screen.getByRole('button', { name: 'Dismiss connection error' }));
  expect(screen.queryByRole('alert')).not.toBeInTheDocument();
  expect(onDismiss).toHaveBeenCalledTimes(1);
});
