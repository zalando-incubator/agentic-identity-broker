/**
 * Service session page from specs/055-incremental-service-authorization (FR-015–FR-016).
 */

import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { MemoryRouter, Route, Routes } from 'react-router-dom';
import { ThirdPartySessionDetailPage } from './ThirdPartySessionDetailPage';
import { sessionsApi, type ServiceSessionDetail } from '@services/api/sessions';

vi.mock('@services/api/sessions', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@services/api/sessions')>();
  return {
    ...actual,
    sessionsApi: {
      getServiceSessionDetail: vi.fn(),
      refreshSession: vi.fn(),
      terminateSession: vi.fn(),
    },
  };
});

const serviceID = '880e8400-e29b-41d4-a716-446655440010';

const detail = (session: ServiceSessionDetail['session']): ServiceSessionDetail => ({
  service: {
    id: serviceID,
    canonical_id: 'github',
    display_name: 'GitHub Production',
    scope_url: 'https://broker.example.com/sessions/github',
  },
  session,
  dependent_agents: [{ agent_id: 'agent-1', display_name: 'Data Analysis Assistant' }],
});

const activeSession = {
  session_id: 'session-1',
  initiated_at: '2026-10-27T09:00:00Z',
  access_token_expires_at: '2099-10-27T10:00:00Z',
  is_expired: false,
  has_refresh_token: true,
  scope: ['repo', 'user:email'],
  dependent_agent_count: 1,
  tokens_encrypted: true,
};

const renderPage = (path = '/sessions/github') =>
  render(
    <MemoryRouter initialEntries={[path]}>
      <Routes>
        <Route path="/sessions/:service" element={<ThirdPartySessionDetailPage />} />
      </Routes>
    </MemoryRouter>,
  );

describe('ThirdPartySessionDetailPage', () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it('shows the service, the missing session and a Login action without granting anything', async () => {
    vi.mocked(sessionsApi.getServiceSessionDetail).mockResolvedValue(detail(null));
    renderPage();

    expect(await screen.findByRole('heading', { level: 2, name: 'GitHub Production' })).toBeInTheDocument();
    expect(screen.getByTestId('service-session-status')).toHaveTextContent('No active session');
    expect(within(screen.getByTestId('service-dependent-agents')).getByText('Data Analysis Assistant')).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Login' })).toBeInTheDocument();
    expect(screen.queryByRole('button', { name: 'Terminate' })).not.toBeInTheDocument();
    expect(sessionsApi.getServiceSessionDetail).toHaveBeenCalledWith('github');
  });

  it('marks a definitively unusable session as needing login', async () => {
    vi.mocked(sessionsApi.getServiceSessionDetail).mockResolvedValue(detail({ ...activeSession, is_expired: true }));
    renderPage();

    expect(await screen.findByTestId('service-session-status')).toHaveTextContent('Expired');
    expect(screen.getByRole('button', { name: 'Login' })).toBeInTheDocument();
  });

  it('shows an active session, refreshes it and terminates it after confirmation', async () => {
    const user = userEvent.setup();
    vi.mocked(sessionsApi.getServiceSessionDetail)
      .mockResolvedValueOnce(detail(activeSession))
      .mockResolvedValueOnce(detail(activeSession))
      .mockResolvedValue(detail(null));
    vi.mocked(sessionsApi.refreshSession).mockResolvedValue({} as never);
    vi.mocked(sessionsApi.terminateSession).mockResolvedValue();
    renderPage(`/sessions/${serviceID}`);

    const status = await screen.findByTestId('service-session-status');
    expect(status).toHaveTextContent('Active');
    expect(status).toHaveTextContent('user:email');

    await user.click(screen.getByRole('button', { name: 'Refresh' }));
    expect(await screen.findByText('Session token refreshed successfully.')).toBeInTheDocument();
    expect(sessionsApi.refreshSession).toHaveBeenCalledWith(serviceID);

    await user.click(screen.getByRole('button', { name: 'Terminate' }));
    await user.click(await screen.findByRole('button', { name: 'Terminate session with GitHub Production' }));
    await waitFor(() => expect(screen.getByTestId('service-session-status')).toHaveTextContent('No active session'));
    expect(sessionsApi.terminateSession).toHaveBeenCalledWith(serviceID);
  });

  it('starts provider sign-in that returns to the same service page', async () => {
    const user = userEvent.setup();
    const assign = vi.fn();
    vi.stubGlobal('location', { ...window.location, origin: 'https://broker.example.com', assign });
    vi.mocked(sessionsApi.getServiceSessionDetail).mockResolvedValue(detail(null));
    renderPage();

    await user.click(await screen.findByRole('button', { name: 'Login' }));
    expect(assign).toHaveBeenCalledWith(
      `/api/third-party/${serviceID}/oauth2/authorize?redirect_uri=${encodeURIComponent('https://broker.example.com/sessions/github')}`,
    );
    vi.unstubAllGlobals();
  });

  it('shows not found for an unknown service and a retry for other failures', async () => {
    vi.mocked(sessionsApi.getServiceSessionDetail).mockRejectedValueOnce({ status: 404, code: 'NOT_FOUND', message: 'The requested resource was not found.' });
    renderPage('/sessions/unknown');
    expect(await screen.findByText('Service not found')).toBeInTheDocument();
  });

  it('offers a retry when the detail cannot be loaded', async () => {
    const user = userEvent.setup();
    vi.mocked(sessionsApi.getServiceSessionDetail)
      .mockRejectedValueOnce({ status: 500, code: 'SERVER_ERROR', message: 'Service temporarily unavailable. Please try again later.' })
      .mockResolvedValue(detail(null));
    renderPage();
    await user.click(await screen.findByRole('button', { name: 'Retry' }));
    expect(await screen.findByTestId('service-session-status')).toHaveTextContent('No active session');
  });
});
