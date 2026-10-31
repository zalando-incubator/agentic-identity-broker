import { describe, it, expect, vi } from 'vitest';
import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { RequestedServiceBanner } from './RequestedServiceBanner';
import type { RequestedServiceCoverage } from '../../types/consent';

const coverage = (usability: RequestedServiceCoverage['usability']): RequestedServiceCoverage => ({
  service_id: 'gh',
  required_scopes: ['repo', 'user:email'],
  usability,
  covering_permission_sets: [
    { permission_set_id: 'read', supplied_required_scopes: ['repo', 'user:email'], usability },
    { permission_set_id: 'triage', supplied_required_scopes: ['repo'], usability },
  ],
});

const props = {
  serviceNames: { gh: 'GitHub Production' },
  permissionSetNames: { read: 'GitHub Read Access', triage: 'GitHub Issue Triage' },
};

describe('RequestedServiceBanner', () => {
  it('names the requested service, its candidates and the login action', async () => {
    const onLogin = vi.fn();
    render(
      <RequestedServiceBanner
        {...props}
        coverage={[coverage('missing')]}
        results={[{ serviceId: 'gh', serviceName: 'GitHub Production', satisfied: false, reason: 'Log in to GitHub Production before saving.' }]}
        onLogin={onLogin}
        onRetry={vi.fn()}
      />,
    );
    expect(screen.getByText('GitHub Production was requested')).toBeInTheDocument();
    expect(screen.getByTestId('requested-service-status')).toHaveTextContent('Needs login');
    expect(screen.getByText(/GitHub Read Access, GitHub Issue Triage/)).toBeInTheDocument();
    expect(screen.getByText('Log in to GitHub Production before saving.')).toBeInTheDocument();
    await userEvent.click(screen.getByRole('button', { name: 'Log in to GitHub Production' }));
    expect(onLogin).toHaveBeenCalledWith('gh');
  });

  it('shows a connected service without a login action and offers retry when unavailable', async () => {
    const onRetry = vi.fn();
    const { rerender } = render(
      <RequestedServiceBanner {...props} coverage={[coverage('usable')]} results={[]} onLogin={vi.fn()} onRetry={onRetry} />,
    );
    expect(screen.getByTestId('requested-service-status')).toHaveTextContent('Connected');
    expect(screen.queryByRole('button', { name: /Log in/ })).not.toBeInTheDocument();

    rerender(<RequestedServiceBanner {...props} coverage={[coverage('unavailable')]} results={[]} onLogin={vi.fn()} onRetry={onRetry} />);
    await userEvent.click(screen.getByRole('button', { name: 'Retry' }));
    expect(onRetry).toHaveBeenCalled();
  });
});
