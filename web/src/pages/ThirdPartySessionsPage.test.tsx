import { useEffect } from 'react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { render, screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { MemoryRouter, Route, Routes, useLocation } from 'react-router-dom';
import { consentApi } from '@services/api/consent';
import { sessionsApi } from '@services/api/sessions';
import type { SessionDetail, SessionSummary } from '@services/api/sessions';
import { ThirdPartySessionsPage } from './ThirdPartySessionsPage';

vi.mock('@services/api/consent', () => ({
  consentApi: {
    getUserInfo: vi.fn(),
    getAgentDelegations: vi.fn(),
  },
}));

vi.mock('@services/api/sessions', () => ({
  sessionsApi: {
    listSessions: vi.fn(),
    getSessionDetails: vi.fn(),
    refreshSession: vi.fn(),
    terminateSession: vi.fn(),
  },
}));

const driveSession: SessionSummary = {
  id: 'session-drive',
  service_id: 'drive',
  service_display_name: 'Drive Space',
  token_type: 'Bearer',
  scope: ['files:read'],
  initiated_at: '2024-03-01T12:00:00Z',
  is_expired: false,
  access_token_expired: false,
  has_refresh_token: true,
  refresh_token_expires_at: '2099-01-01T00:00:00Z',
  dependent_agent_count: 1,
  is_encrypted: true,
};

const driveDetails: SessionDetail = {
  session: driveSession,
  dependent_agents: [{ id: 'agent-1', display_name: 'Research Bot' }],
};

function RouteObserver({ routes }: { routes: string[] }) {
  const location = useLocation();
  useEffect(() => {
    routes.push(`${location.pathname}${location.search}`);
  }, [location, routes]);
  return null;
}

function renderPage(initialEntry = '/sessions') {
  const routes: string[] = [];
  render(
    <MemoryRouter initialEntries={[initialEntry]}>
      <RouteObserver routes={routes} />
      <Routes>
        <Route path="/sessions" element={<ThirdPartySessionsPage />} />
      </Routes>
    </MemoryRouter>,
  );
  return routes;
}

describe('ThirdPartySessionsPage', () => {
  beforeEach(() => {
    vi.resetAllMocks();
    vi.mocked(consentApi.getUserInfo).mockResolvedValue({
      principal: 'alex@example.com',
      displayName: 'Alex User',
    });
    vi.mocked(consentApi.getAgentDelegations).mockResolvedValue([]);
    vi.mocked(sessionsApi.listSessions).mockResolvedValue([]);
  });

  afterEach(() => {
    vi.restoreAllMocks();
  });

  it('shows a list error, then displays sessions after Retry', async () => {
    vi.mocked(sessionsApi.listSessions)
      .mockRejectedValueOnce(new Error('Session list unavailable'))
      .mockResolvedValueOnce([driveSession]);
    const user = userEvent.setup();
    renderPage();

    const error = await screen.findByRole('alert');
    expect(error).toHaveTextContent('Session list unavailable');
    await user.click(within(error).getByRole('button', { name: 'Retry' }));

    expect(
      await screen.findByRole('heading', { name: 'Drive Space' }),
    ).toBeInTheDocument();
    expect(screen.queryByText('Session list unavailable')).not.toBeInTheDocument();
  });

  it('cleans an OAuth callback error URL and lets the user dismiss its message', async () => {
    const user = userEvent.setup();
    const routes = renderPage(
      '/sessions?error=access_denied&error_description=ignored',
    );
    const message = 'You denied access to the service. No tokens were stored.';

    expect(await screen.findByText(message)).toBeInTheDocument();
    await waitFor(() => {
      expect(routes[routes.length - 1]).toBe('/sessions');
    });
    await user.click(screen.getByRole('button', { name: 'Dismiss alert' }));
    await waitFor(() => {
      expect(screen.queryByText(message)).not.toBeInTheDocument();
    });
    expect(screen.getByText('No Sessions Found')).toBeInTheDocument();
  });

  it('shows callback success and refetches the newly connected session', async () => {
    vi.mocked(sessionsApi.listSessions)
      .mockResolvedValueOnce([])
      .mockResolvedValueOnce([driveSession]);
    const user = userEvent.setup();
    renderPage('/sessions?success=true');
    const message =
      'Successfully connected to service. You can now delegate access to agents.';

    expect(await screen.findByText(message)).toBeInTheDocument();
    expect(
      await screen.findByRole('heading', { name: 'Drive Space' }),
    ).toBeInTheDocument();
    expect(sessionsApi.listSessions).toHaveBeenCalledTimes(2);
    await user.click(screen.getByRole('button', { name: 'Dismiss alert' }));
    await waitFor(() => {
      expect(screen.queryByText(message)).not.toBeInTheDocument();
    });
    expect(screen.getByRole('heading', { name: 'Drive Space' })).toBeInTheDocument();
  });

  it('shows expiry guidance when no refresh token can recover the session', async () => {
    vi.mocked(sessionsApi.listSessions).mockResolvedValue([{
      ...driveSession,
      is_expired: true,
      access_token_expired: true,
      has_refresh_token: false,
    }]);
    renderPage();

    expect(
      await screen.findByRole('heading', { name: 'Drive Space' }),
    ).toBeInTheDocument();
    expect(screen.getByText('Expired')).toBeInTheDocument();
    expect(screen.getByText('Re-authenticate required')).toBeInTheDocument();
    expect(screen.queryByRole('button', { name: 'Terminate' })).not.toBeInTheDocument();
    expect(screen.queryByRole('button', { name: 'Refresh' })).not.toBeInTheDocument();
  });

  it('recovers from a failed refresh of an expired access token', async () => {
    vi.mocked(sessionsApi.listSessions)
      .mockResolvedValueOnce([{ ...driveSession, access_token_expired: true }])
      .mockResolvedValueOnce([driveSession]);
    vi.mocked(sessionsApi.refreshSession)
      .mockRejectedValueOnce(new Error('Refresh token rejected'))
      .mockResolvedValueOnce(driveSession);
    vi.spyOn(console, 'error').mockImplementation(() => {});
    const user = userEvent.setup();
    renderPage();

    expect(await screen.findByText('Access Token Expired')).toBeInTheDocument();
    await user.click(screen.getByRole('button', { name: 'Refresh' }));
    expect(await screen.findByText('Refresh token rejected')).toBeInTheDocument();
    expect(screen.getByText('Access Token Expired')).toBeInTheDocument();
    expect(sessionsApi.listSessions).toHaveBeenCalledTimes(1);
    await waitFor(() => {
      expect(screen.getByRole('button', { name: 'Refresh' })).toBeEnabled();
    });

    await user.click(screen.getByRole('button', { name: 'Refresh' }));
    expect(await screen.findByText('Session token refreshed successfully.')).toBeInTheDocument();
    expect(screen.getByText('Active')).toBeInTheDocument();
    expect(screen.queryByText('Access Token Expired')).not.toBeInTheDocument();
    expect(sessionsApi.refreshSession).toHaveBeenCalledWith('drive');
    expect(sessionsApi.listSessions).toHaveBeenCalledTimes(2);
  });

  it('reports a details fetch failure, then allows retry and cancel without termination', async () => {
    vi.mocked(sessionsApi.listSessions).mockResolvedValue([driveSession]);
    vi.mocked(sessionsApi.getSessionDetails)
      .mockRejectedValueOnce(new Error('Details unavailable'))
      .mockResolvedValueOnce(driveDetails);
    vi.spyOn(console, 'error').mockImplementation(() => {});
    const user = userEvent.setup();
    renderPage();

    await screen.findByRole('heading', { name: 'Drive Space' });
    await user.click(screen.getByRole('button', { name: 'Terminate' }));
    const detailsError = await screen.findByRole('alert');
    expect(detailsError).toHaveTextContent(
      'Failed to load session details. Please try again.',
    );
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument();

    await user.click(screen.getByRole('button', { name: 'Terminate' }));
    const dialog = await screen.findByRole('dialog', { name: 'Terminate Session?' });
    expect(within(dialog).getByText('Research Bot')).toBeInTheDocument();
    expect(
      screen.queryByText('Failed to load session details. Please try again.'),
    ).not.toBeInTheDocument();
    await user.click(within(dialog).getByRole('button', { name: 'Cancel session termination' }));

    expect(screen.queryByRole('dialog')).not.toBeInTheDocument();
    expect(screen.getByRole('heading', { name: 'Drive Space' })).toBeInTheDocument();
    expect(sessionsApi.terminateSession).not.toHaveBeenCalled();
  });

  it('keeps the confirmation open after a termination error, then refetches the removed session', async () => {
    vi.mocked(sessionsApi.listSessions)
      .mockResolvedValueOnce([driveSession])
      .mockResolvedValueOnce([]);
    vi.mocked(sessionsApi.getSessionDetails).mockResolvedValue(driveDetails);
    vi.mocked(sessionsApi.terminateSession)
      .mockRejectedValueOnce(new Error('Unable to revoke session'))
      .mockResolvedValueOnce(undefined);
    vi.spyOn(console, 'error').mockImplementation(() => {});
    const user = userEvent.setup();
    renderPage();

    await screen.findByRole('heading', { name: 'Drive Space' });
    await user.click(screen.getByRole('button', { name: 'Terminate' }));
    const dialog = await screen.findByRole('dialog', { name: 'Terminate Session?' });
    expect(within(dialog).getByText('Research Bot')).toBeInTheDocument();
    const confirm = within(dialog).getByRole('button', {
      name: 'Terminate session with Drive Space',
    });

    await user.click(confirm);
    expect(await within(dialog).findByText('Unable to revoke session')).toBeInTheDocument();
    expect(screen.getByRole('heading', { name: 'Drive Space' })).toBeInTheDocument();
    await waitFor(() => {
      expect(confirm).toBeEnabled();
    });

    await user.click(confirm);
    expect(await screen.findByText('Session terminated successfully.')).toBeInTheDocument();
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument();
    expect(screen.queryByRole('heading', { name: 'Drive Space' })).not.toBeInTheDocument();
    expect(screen.getByText('No Sessions Found')).toBeInTheDocument();
    expect(sessionsApi.terminateSession).toHaveBeenCalledTimes(2);
    expect(sessionsApi.terminateSession).toHaveBeenCalledWith('drive');
    expect(sessionsApi.listSessions).toHaveBeenCalledTimes(2);
  });
});
