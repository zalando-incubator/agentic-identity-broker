import { describe, it, expect, vi } from 'vitest';
import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { SessionCard } from './SessionCard';
import type { SessionSummary } from '@services/api/sessions';

const createMockSession = (
  overrides?: Partial<SessionSummary>,
): SessionSummary => ({
  id: 'test-session-id',
  service_id: 'google',
  service_display_name: 'Google Drive',
  token_type: 'Bearer',
  scope: ['read:email', 'write:files'],
  initiated_at: '2024-01-01T12:00:00Z',
  requires_reauthentication: false,
  access_token_expired: false,
  has_refresh_token: true,
  dependent_agent_count: 2,
  is_encrypted: true,
  ...overrides,
});

describe('SessionCard', () => {
  it.each([false, true])(
    'replaces refresh with login after rejection and restores it after recovery (access expired: %s)',
    (accessTokenExpired) => {
      const session = createMockSession();
      const actions = {
        onTerminate: vi.fn(),
        onRefresh: vi.fn(),
        onLogin: vi.fn(),
      };
      const { rerender } = render(
        <SessionCard session={session} {...actions} />,
      );

      expect(screen.getByText('Active')).toHaveClass('bg-success-primary');
      expect(screen.getByRole('button', { name: 'Refresh' })).toBeEnabled();
      expect(screen.queryByRole('button', { name: 'Login' })).toBeNull();

      rerender(
        <SessionCard
          session={{
            ...session,
            requires_reauthentication: true,
            access_token_expired: accessTokenExpired,
          }}
          {...actions}
        />,
      );

      expect(screen.getByText('Reauthentication required')).toHaveClass(
        'bg-error-primary',
      );
      expect(screen.queryByText('Access Token Expired')).toBeNull();
      expect(screen.queryByRole('button', { name: 'Refresh' })).toBeNull();
      expect(screen.getByRole('button', { name: 'Login' })).toBeEnabled();
      expect(screen.getByRole('button', { name: 'Terminate' })).toBeEnabled();

      rerender(<SessionCard session={session} {...actions} />);

      expect(screen.getByText('Active')).toHaveClass('bg-success-primary');
      expect(screen.getByRole('button', { name: 'Refresh' })).toBeEnabled();
      expect(screen.queryByRole('button', { name: 'Login' })).toBeNull();
      expect(screen.getByRole('button', { name: 'Terminate' })).toBeEnabled();
    },
  );

  it('keeps refresh available when only the access token expired', () => {
    render(
      <SessionCard
        session={createMockSession({ access_token_expired: true })}
        onTerminate={vi.fn()}
        onRefresh={vi.fn()}
        onLogin={vi.fn()}
      />,
    );

    expect(screen.getByText('Access Token Expired')).toHaveClass(
      'bg-warning-primary',
    );
    expect(screen.getByRole('button', { name: 'Refresh' })).toBeEnabled();
    expect(screen.queryByRole('button', { name: 'Login' })).toBeNull();
    expect(screen.getByRole('button', { name: 'Terminate' })).toBeEnabled();
  });

  it('offers login when an access-only session is exhausted', () => {
    const session = createMockSession({ has_refresh_token: false });
    const actions = {
      onTerminate: vi.fn(),
      onRefresh: vi.fn(),
      onLogin: vi.fn(),
    };
    const { rerender } = render(
      <SessionCard session={session} {...actions} />,
    );

    expect(screen.getByText('Active')).toHaveClass('bg-success-primary');
    expect(screen.queryByRole('button', { name: 'Refresh' })).toBeNull();
    expect(screen.queryByRole('button', { name: 'Login' })).toBeNull();

    rerender(
      <SessionCard
        session={{
          ...session,
          access_token_expired: true,
          requires_reauthentication: true,
        }}
        {...actions}
      />,
    );

    expect(screen.getByText('Reauthentication required')).toHaveClass(
      'bg-error-primary',
    );
    expect(screen.getByRole('button', { name: 'Login' })).toBeEnabled();
    expect(screen.getByRole('button', { name: 'Terminate' })).toBeEnabled();
    expect(screen.queryByRole('button', { name: 'Refresh' })).toBeNull();
  });

  it('retains termination when login is unavailable for a rejected session', () => {
    render(
      <SessionCard
        session={createMockSession({ requires_reauthentication: true })}
        onTerminate={vi.fn()}
        onRefresh={vi.fn()}
      />,
    );

    expect(screen.queryByRole('button', { name: 'Login' })).toBeNull();
    expect(screen.queryByRole('button', { name: 'Refresh' })).toBeNull();
    expect(screen.getByRole('button', { name: 'Terminate' })).toBeEnabled();
  });

  it.each(['loading', 'refreshing'] as const)(
    'disables healthy and rejected card actions while %s',
    (pendingAction) => {
      const session = createMockSession();
      const actions = {
        onTerminate: vi.fn(),
        onViewDetails: vi.fn(),
        onRefresh: vi.fn(),
        onLogin: vi.fn(),
        [pendingAction]: true,
      };
      const { rerender } = render(
        <SessionCard session={session} {...actions} />,
      );

      expect(screen.getByRole('button', { name: 'View Details' })).toBeDisabled();
      expect(screen.getByRole('button', { name: 'Refresh' })).toBeDisabled();
      expect(screen.getByRole('button', { name: 'Terminate' })).toBeDisabled();

      rerender(
        <SessionCard
          session={{ ...session, requires_reauthentication: true }}
          {...actions}
        />,
      );

      expect(screen.getByRole('button', { name: 'View Details' })).toBeDisabled();
      expect(screen.getByRole('button', { name: 'Login' })).toBeDisabled();
      expect(screen.getByRole('button', { name: 'Terminate' })).toBeDisabled();
      expect(screen.queryByRole('button', { name: 'Refresh' })).toBeNull();
    },
  );

  it('allows keyboard navigation to recovery and termination', async () => {
    const user = userEvent.setup();
    render(
      <SessionCard
        session={createMockSession({ requires_reauthentication: true })}
        onTerminate={vi.fn()}
        onLogin={vi.fn()}
      />,
    );

    await user.tab();
    expect(screen.getByRole('button', { name: 'Login' })).toHaveFocus();
    await user.tab();
    expect(screen.getByRole('button', { name: 'Terminate' })).toHaveFocus();
  });
});
