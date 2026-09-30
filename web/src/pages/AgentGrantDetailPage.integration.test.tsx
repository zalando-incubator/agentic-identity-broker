/** Grant page interactions through real hooks and permission controls. */

import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import { MemoryRouter, Route, Routes } from 'react-router-dom';
import { addDays, format, startOfDay } from 'date-fns';
import { AxiosError, AxiosHeaders } from 'axios';
import { AgentGrantDetailPage } from './AgentGrantDetailPage';
import { consentApi } from '@services/api/consent';
import { ToastProvider } from '../components/ui/Toast';
import type {
  AgentDetail,
  ThirdpartyService,
  UserGrant,
} from '../types/consent';

// Keep the real page, hooks, and permission controls; only replace their API boundary.
vi.mock('@services/api/consent', () => ({
  consentApi: {
    getUserInfo: vi.fn(),
    getAgentDelegations: vi.fn(),
    getAgentDetail: vi.fn(),
    getAgentGrants: vi.fn(),
    createOrUpdateGrant: vi.fn(),
    deleteGrant: vi.fn(),
  },
}));

describe('AgentGrantDetailPage - Integration', () => {
  const mockAgent: AgentDetail = {
    agentId: 'test-agent',
    displayName: 'Test Agent',
    description: 'A test agent',
    logoUrl: 'https://example.com/logo.png',
    governanceUrl: 'https://example.com/governance',
    userDocumentationUrl: 'https://example.com/docs',
    agentInterfaceUrl: 'https://example.com/interface',
    permission_sets: [
      {
        permission_set: {
          id: 'ps-core',
          name: 'Core Access',
          description: 'Required GitHub access',
          service_scopes: [
            { service_id: 'github', requirement_type: 'mandatory' },
          ],
        },
        requirement_type: 'mandatory',
      },
      {
        permission_set: {
          id: 'ps-files',
          name: 'Drive Files',
          description: 'Optional Google Drive access',
          service_scopes: [
            { service_id: 'drive', requirement_type: 'optional' },
          ],
        },
        requirement_type: 'optional',
      },
    ],
    service_requirements: [
      { service_id: 'github', requirement_type: 'mandatory' },
      { service_id: 'drive', requirement_type: 'optional' },
    ],
    active_session_service_ids: ['github', 'drive'],
  };

  const mockServices: ThirdpartyService[] = [
    {
      kind: 'scoped',
      serviceId: 'github',
      displayName: 'GitHub',
      scopes: [
        { value: 'read:user', description: 'Read user profile' },
        { value: 'read:repo', description: 'Read repositories' },
      ],
    },
    {
      kind: 'scoped',
      serviceId: 'drive',
      displayName: 'Google Drive',
      scopes: [{ value: 'drive.readonly', description: 'Read files' }],
    },
  ];

  const mockGrant: UserGrant = {
    id: 'grant-123',
    agent_id: 'test-agent',
    principal: 'user@example.com',
    granted_permission_sets: { 'ps-core': ['github'] },
    valid_until: null,
    created_at: '2024-01-01T00:00:00Z',
    updated_at: '2024-01-01T00:00:00Z',
  };

  beforeEach(() => {
    vi.resetAllMocks();

    // The header also loads current-user information through the real useConsent hook.
    vi.mocked(consentApi.getUserInfo).mockResolvedValue({
      principal: 'user@example.com',
      displayName: 'Test User',
      pictureUrl: 'https://example.com/avatar.png',
    });

    vi.mocked(consentApi.getAgentDelegations).mockResolvedValue([]);

    vi.mocked(consentApi.getAgentDetail).mockResolvedValue({
      agent: mockAgent,
      services: mockServices,
    });

    vi.mocked(consentApi.getAgentGrants).mockResolvedValue(mockGrant);
  });

  afterEach(() => {
    vi.restoreAllMocks();
  });

  const renderPage = (initialEntry = '/agents/test-agent') => {
    return render(
      <ToastProvider>
        <MemoryRouter initialEntries={[initialEntry]}>
          <Routes>
            <Route path="/agents/:agentId" element={<AgentGrantDetailPage />} />
          </Routes>
        </MemoryRouter>
      </ToastProvider>,
    );
  };

  it('shows a load error and recovers when the user retries', async () => {
    vi.mocked(consentApi.getAgentDetail)
      .mockRejectedValueOnce(new Error('Agent details temporarily unavailable'));

    renderPage();

    expect(
      await screen.findByText('Agent details temporarily unavailable'),
    ).toBeInTheDocument();
    expect(screen.queryByRole('button', { name: 'Save' })).not.toBeInTheDocument();

    fireEvent.click(screen.getByRole('button', { name: 'Try again' }));

    expect(await screen.findByRole('heading', { name: 'Test Agent' })).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Save' })).toBeEnabled();
    expect(screen.queryByText('Agent details temporarily unavailable')).not.toBeInTheDocument();
    expect(consentApi.getAgentDetail).toHaveBeenCalledTimes(2);
    expect(consentApi.getAgentDetail).toHaveBeenNthCalledWith(2, 'test-agent', undefined);
    expect(consentApi.getAgentGrants).toHaveBeenCalledTimes(2);
  });

  it('guides an expired authorization session back instead of retrying its token', async () => {
    const expiredError = new AxiosError(
      'Authorization session expired',
      'ERR_BAD_REQUEST',
      undefined,
      undefined,
      {
        data: { error: 'session_expired' },
        status: 410,
        statusText: 'Gone',
        headers: new AxiosHeaders(),
        config: { headers: new AxiosHeaders() },
      },
    );
    vi.mocked(consentApi.getAgentDetail).mockRejectedValue(expiredError);
    const goBack = vi.spyOn(window.history, 'back').mockImplementation(() => {});

    renderPage('/agents/test-agent?session_token=expired-session');

    expect(
      await screen.findByText(
        'Your authorization session has expired. Please go back and restart the authorization flow.',
      ),
    ).toBeInTheDocument();
    expect(consentApi.getAgentDetail).toHaveBeenCalledWith('test-agent', {
      sessionToken: 'expired-session',
    });
    expect(screen.queryByRole('button', { name: 'Try again' })).not.toBeInTheDocument();
    expect(screen.queryByRole('button', { name: 'Save' })).not.toBeInTheDocument();

    fireEvent.click(screen.getByRole('button', { name: 'Go back' }));
    expect(goBack).toHaveBeenCalledOnce();
    expect(consentApi.getAgentDetail).toHaveBeenCalledTimes(1);
  });

  it('blocks saving when no permission set remains selected', async () => {
    vi.mocked(consentApi.getAgentDetail).mockResolvedValue({
      agent: {
        ...mockAgent,
        permission_sets: mockAgent.permission_sets?.slice(1),
        service_requirements: [{ service_id: 'drive', requirement_type: 'optional' }],
      },
      services: [mockServices[1]],
    });
    vi.mocked(consentApi.getAgentGrants).mockResolvedValue({
      ...mockGrant,
      granted_permission_sets: { 'ps-files': ['drive'] },
    });

    renderPage();

    const filesToggle = await screen.findByRole('switch', { name: 'Toggle Drive Files' });
    expect(filesToggle).toHaveAttribute('aria-checked', 'true');
    fireEvent.click(filesToggle);
    expect(filesToggle).toHaveAttribute('aria-checked', 'false');
    fireEvent.click(screen.getByRole('button', { name: 'Save' }));

    expect(screen.getByRole('alert')).toHaveTextContent(
      'Please select at least one permission set',
    );
    expect(consentApi.createOrUpdateGrant).not.toHaveBeenCalled();
  });

  it('validates expiry, then saves selected permission sets and refreshes the grant', async () => {
    const expiresAt = addDays(startOfDay(new Date()), 7);
    const updatedGrant: UserGrant = {
      ...mockGrant,
      granted_permission_sets: { 'ps-core': ['github'], 'ps-files': ['drive'] },
      valid_until: expiresAt.toISOString(),
    };
    vi.mocked(consentApi.getAgentGrants)
      .mockResolvedValueOnce(mockGrant)
      .mockResolvedValueOnce(updatedGrant);
    vi.mocked(consentApi.createOrUpdateGrant).mockResolvedValue({
      kind: 'created',
      grant: updatedGrant,
    });

    renderPage();

    const save = await screen.findByRole('button', { name: 'Save' });
    fireEvent.click(screen.getByRole('checkbox', { name: 'Specific end date' }));
    const endDate = screen.getByLabelText('End date');
    fireEvent.change(endDate, { target: { value: '' } });
    fireEvent.click(save);

    expect(screen.getByRole('alert')).toHaveTextContent(
      'Enter an end date when Specific end date is selected.',
    );
    expect(consentApi.createOrUpdateGrant).not.toHaveBeenCalled();

    fireEvent.change(endDate, {
      target: { value: format(expiresAt, 'yyyy-MM-dd') },
    });
    expect(endDate).toHaveValue(format(expiresAt, 'yyyy-MM-dd'));
    const filesToggle = screen.getByRole('switch', { name: 'Toggle Drive Files' });
    expect(filesToggle).toHaveAttribute('aria-checked', 'false');
    fireEvent.click(filesToggle);
    expect(filesToggle).toHaveAttribute('aria-checked', 'true');
    fireEvent.click(save);

    await waitFor(() => {
      expect(consentApi.createOrUpdateGrant).toHaveBeenCalledTimes(1);
      expect(consentApi.createOrUpdateGrant).toHaveBeenCalledWith(
        'test-agent',
        {
          granted_permission_sets: { 'ps-core': ['github'], 'ps-files': ['drive'] },
          valid_until: expiresAt.toISOString(),
        },
        undefined,
      );
    });
    expect(await screen.findByText('Grant updated successfully!')).toBeInTheDocument();
    expect(consentApi.getAgentDetail).toHaveBeenCalledTimes(2);
    expect(consentApi.getAgentGrants).toHaveBeenCalledTimes(2);
    expect(screen.getByRole('switch', { name: 'Toggle Drive Files' })).toHaveAttribute(
      'aria-checked',
      'true',
    );
    expect(screen.queryByText('Enter an end date when Specific end date is selected.'))
      .not.toBeInTheDocument();
  });

  it('shows a failed save and retries the same grant with its session token', async () => {
    vi.mocked(consentApi.createOrUpdateGrant)
      .mockRejectedValueOnce(new Error('Unable to save grant'))
      .mockResolvedValueOnce({ kind: 'created', grant: mockGrant });

    renderPage('/agents/test-agent?session_token=live-session');

    fireEvent.click(await screen.findByRole('button', { name: 'Save' }));

    const retry = await screen.findByRole('button', { name: 'Try again' });
    expect(retry.closest('[role="alert"]')).toHaveTextContent('Unable to save grant');
    await waitFor(() => {
      expect(screen.getByRole('region', { name: 'Notifications' })).toHaveTextContent(
        'Unable to save grant',
      );
    });
    expect(consentApi.getAgentGrants).toHaveBeenCalledTimes(1);

    fireEvent.click(retry);

    expect(await screen.findByText('Grant updated successfully!')).toBeInTheDocument();
    expect(consentApi.createOrUpdateGrant).toHaveBeenCalledTimes(2);
    for (const call of [1, 2]) {
      expect(consentApi.createOrUpdateGrant).toHaveBeenNthCalledWith(
        call,
        'test-agent',
        {
          granted_permission_sets: { 'ps-core': ['github'] },
          valid_until: undefined,
        },
        { sessionToken: 'live-session' },
      );
    }
    expect(consentApi.getAgentGrants).toHaveBeenCalledTimes(2);
    expect(screen.queryByRole('button', { name: 'Try again' })).not.toBeInTheDocument();
  });
});
