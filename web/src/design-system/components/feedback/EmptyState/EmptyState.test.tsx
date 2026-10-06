import { Plug } from 'lucide-react';
import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { expect, it, vi } from 'vitest';
import { EmptyState } from './EmptyState';

it('explains the concept with an illustrated icon and a caller-supplied next step', async () => {
  const onConnect = vi.fn();
  render(<EmptyState title="No connections yet" description="Connections appear after you authorize a service."
    icon={<Plug />} primaryAction={{ label: 'Connect a service', onClick: onConnect }} />);
  expect(screen.getByRole('status')).toHaveTextContent('Connections appear after you authorize a service.');
  await userEvent.click(screen.getByRole('button', { name: 'Connect a service' }));
  expect(onConnect).toHaveBeenCalledOnce();
});
