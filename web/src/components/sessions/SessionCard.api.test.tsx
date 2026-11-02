import { beforeEach, describe, expect, it, vi } from 'vitest';
import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { apiClient } from '@services/api/client';
import { apiCache } from '@services/api/cache';
import { SessionsApiService } from '@services/api/sessions';
import { SessionCard } from './SessionCard';

vi.mock('@services/api/client', () => ({
  apiClient: { get: vi.fn(), post: vi.fn() },
}));

const legacySession = {
  id: 'session-1',
  service_id: 'google',
  service_display_name: 'Google Drive',
  token_type: 'Bearer',
  scope: ['read:email'],
  initiated_at: '2024-01-01T12:00:00Z',
  is_expired: false,
  access_token_expired: false,
  has_refresh_token: true,
  dependent_agent_count: 0,
  is_encrypted: true,
};

describe('Session recovery across broker versions', () => {
  beforeEach(() => {
    vi.resetAllMocks();
    apiCache.clear();
  });

  it.each([
    {
      name: 'legacy exhausted access-only session',
      fields: {
        is_expired: true,
        access_token_expired: true,
        has_refresh_token: false,
      },
      action: 'Login',
    },
    {
      name: 'legacy rejected refresh with still-valid access',
      fields: { refresh_token_expires_at: '2000-01-01T00:00:00Z' },
      action: 'Login',
    },
    {
      name: 'legacy expired access with usable refresh',
      fields: { access_token_expired: true },
      action: 'Refresh',
    },
    {
      name: 'modern rejection overrides legacy usable state',
      fields: { requires_reauthentication: true },
      action: 'Login',
    },
    {
      name: 'modern usable state overrides legacy expiry and rejection marker',
      fields: {
        requires_reauthentication: false,
        is_expired: true,
        refresh_token_expires_at: '2000-01-01T00:00:00Z',
      },
      action: 'Refresh',
    },
  ])('offers the recovery action for $name', async ({ fields, action }) => {
    vi.mocked(apiClient.get).mockResolvedValueOnce({
      data: { data: { sessions: [{ ...legacySession, ...fields }] } },
    });
    const service = new SessionsApiService();
    await service.listSessions();
    const [session] = await service.listSessions();
    const actions = {
      onLogin: vi.fn(),
      onRefresh: vi.fn(),
      onTerminate: vi.fn(),
    };
    render(<SessionCard session={session} {...actions} />);

    expect(
      screen.queryByRole('button', {
        name: action === 'Login' ? 'Refresh' : 'Login',
      }),
    ).toBeNull();
    await userEvent.click(screen.getByRole('button', { name: action }));
    expect(
      action === 'Login' ? actions.onLogin : actions.onRefresh,
    ).toHaveBeenCalledWith('google');
    expect(apiClient.get).toHaveBeenCalledOnce();
  });

  it('offers Login from cached legacy session details', async () => {
    vi.mocked(apiClient.get).mockResolvedValueOnce({
      data: {
        data: {
          session: {
            ...legacySession,
            refresh_token_expires_at: '2000-01-01T00:00:00Z',
          },
          dependent_agents: [],
        },
      },
    });
    const service = new SessionsApiService();
    await service.getSessionDetails('google');
    const details = await service.getSessionDetails('google');
    const actions = {
      onLogin: vi.fn(),
      onRefresh: vi.fn(),
      onTerminate: vi.fn(),
    };
    render(<SessionCard session={details.session} {...actions} />);
    expect(screen.getByRole('button', { name: 'Login' })).toBeEnabled();
    expect(screen.queryByRole('button', { name: 'Refresh' })).toBeNull();

    await userEvent.click(screen.getByRole('button', { name: 'Login' }));
    expect(actions.onLogin).toHaveBeenCalledWith('google');
    expect(apiClient.get).toHaveBeenCalledOnce();
  });
});
