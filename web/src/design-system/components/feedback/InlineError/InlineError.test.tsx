import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { expect, it, vi } from 'vitest';
import { InlineError } from './InlineError';

it('announces a single validation error from the errors collection rather than the summary', () => {
  render(<InlineError message="Invalid settings" errors={['The end date must be in the future.']} />);
  expect(screen.getByRole('alert')).toHaveTextContent('The end date must be in the future.');
  expect(screen.queryByText('Invalid settings')).not.toBeInTheDocument();
});

it('runs the caller retry and blocks duplicate recovery while pending', async () => {
  const onRetry = vi.fn();
  const props = { message: 'Unable to save changes', onRetry, retryLabel: 'Save again', isRetrying: true };
  const view = render(<InlineError {...props} />);
  await userEvent.click(screen.getByRole('button', { name: 'Save again' }));
  expect(onRetry).not.toHaveBeenCalled();
  view.rerender(<InlineError {...{ ...props, isRetrying: false }} />);
  await userEvent.click(screen.getByRole('button', { name: 'Save again' }));
  expect(onRetry).toHaveBeenCalledTimes(1);
});
