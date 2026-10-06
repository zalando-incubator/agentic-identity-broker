import { render, screen } from '@testing-library/react';
import { expect, it } from 'vitest';
import { Skeleton } from './Skeleton';

it('keeps decorative placeholders out of the live region when their parent announces loading', () => {
  render(<section aria-busy="true" aria-label="Loading connections"><Skeleton /><Skeleton /></section>);
  expect(screen.queryByRole('status')).not.toBeInTheDocument();
  expect(screen.getByRole('region', { name: 'Loading connections' })).toHaveAttribute('aria-busy', 'true');
});

it('announces caller-provided loading copy once for a group of placeholders', () => {
  render(<Skeleton {...{ label: 'Reading permissions', count: 3 }} />);
  expect(screen.getByRole('status')).toHaveTextContent('Reading permissions');
  expect(screen.getAllByText('Reading permissions')).toHaveLength(1);
});
