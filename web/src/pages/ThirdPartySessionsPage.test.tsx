import { beforeEach, describe, expect, it, vi } from 'vitest';
import { act, fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import { MemoryRouter } from 'react-router-dom';
import { ThirdPartySessionsPage } from './ThirdPartySessionsPage';
import { sessionsApi, type SessionSummary } from '@services/api/sessions';
import { consentApi } from '@services/api/consent';

vi.mock('@services/api/sessions', () => ({
  sessionsApi: { listSessions: vi.fn(), refreshSession: vi.fn(), getSessionDetails: vi.fn(), terminateSession: vi.fn() },
}));
vi.mock('@services/api/consent', () => ({
  consentApi: { getUserInfo: vi.fn(), getAgentDelegations: vi.fn() },
}));

const source = '<svg xmlns="http://www.w3.org/2000/svg"><circle r="3"/></svg>';
const replacement = '<svg xmlns="http://www.w3.org/2000/svg"><rect width="4" height="4"/></svg>';
const detailSource = '<svg xmlns="http://www.w3.org/2000/svg"><ellipse rx="2" ry="1"/></svg>';
const session: SessionSummary = {
  id: 'session', service_id: 'atlas', service_display_name: 'Atlas Files',
  token_type: 'Bearer', scope: ['files.read'], initiated_at: '2026-11-18T10:00:00Z',
  requires_reauthentication: false, access_token_expired: false,
  has_refresh_token: true, dependent_agent_count: 1, is_encrypted: true,
};

function pendingSessionList() {
  let resolve!: (sessions: SessionSummary[]) => void;
  const promise = new Promise<SessionSummary[]>((settle) => { resolve = settle; });
  return { promise, resolve };
}

describe('ThirdPartySessionsPage current icon metadata', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    vi.mocked(consentApi.getUserInfo).mockResolvedValue({ principal: 'alice@example.com', displayName: 'Alice' });
    vi.mocked(consentApi.getAgentDelegations).mockResolvedValue([]);
  });

  it('recovers a failed list icon on refresh and uses fresh detail metadata in the dialog', async () => {
    vi.mocked(sessionsApi.listSessions).mockResolvedValueOnce([{ ...session, service_icon_svg: source }]);
    vi.mocked(sessionsApi.refreshSession).mockResolvedValue({ ...session, service_icon_svg: replacement });
    const { container } = render(<MemoryRouter><ThirdPartySessionsPage /></MemoryRouter>);
    const refresh = await screen.findByRole('button', { name: 'Refresh' });
    fireEvent.error(container.querySelector('img[src^="data:image/svg+xml"]')!);
    expect(container.querySelector('img[src^="data:image/svg+xml"]')).toBeNull();
    const pending = pendingSessionList();
    vi.mocked(sessionsApi.listSessions).mockReturnValueOnce(pending.promise);
    fireEvent.click(refresh);
    await waitFor(() => expect(sessionsApi.listSessions).toHaveBeenCalledTimes(2));
    expect(screen.queryByRole('button', { name: 'Refresh' })).not.toBeInTheDocument();
    expect(screen.queryByRole('button', { name: 'Terminate' })).not.toBeInTheDocument();
    await act(async () => pending.resolve([{ ...session, service_icon_svg: replacement }]));
    const recovered = container.querySelector('img[src^="data:image/svg+xml"]')!;
    expect(recovered).toHaveAttribute('src', `data:image/svg+xml;charset=utf-8,${encodeURIComponent(replacement)}`);
    fireEvent.load(recovered);
    await waitFor(() => expect(recovered).toBeVisible());
    const refreshedButton = screen.getByRole('button', { name: 'Refresh' });
    expect(refreshedButton).toBeEnabled();
    vi.mocked(sessionsApi.getSessionDetails).mockResolvedValue({
      session: { ...session, service_icon_svg: detailSource }, dependent_agents: [],
    });
    fireEvent.click(screen.getByRole('button', { name: 'Terminate' }));
    const dialog = await screen.findByRole('dialog');
    expect(dialog).toHaveAccessibleDescription('Are you sure you want to terminate your session with Atlas Files?');
    const detailImage = dialog.querySelector('img')!;
    expect(detailImage).toHaveAttribute('src', `data:image/svg+xml;charset=utf-8,${encodeURIComponent(detailSource)}`);
    expect(recovered).toHaveAttribute('src', `data:image/svg+xml;charset=utf-8,${encodeURIComponent(replacement)}`);
    expect(detailImage).toHaveAttribute('alt', '');
    expect(dialog.querySelector('#termination-dialog-description img')).toBeNull();
    fireEvent.click(within(dialog).getByRole('button', { name: 'Cancel session termination' }));
    const cleared = pendingSessionList();
    vi.mocked(sessionsApi.listSessions).mockReturnValueOnce(cleared.promise);
    fireEvent.click(refreshedButton);
    await waitFor(() => expect(sessionsApi.listSessions).toHaveBeenCalledTimes(3));
    expect(recovered).not.toBeInTheDocument();
    await act(async () => cleared.resolve([session]));
    expect(container.querySelector('img[src^="data:image/svg+xml"]')).toBeNull();
    expect(screen.getByRole('button', { name: 'Refresh' })).toBeEnabled();
    await waitFor(() => expect(screen.getByRole('heading', { name: 'Atlas Files' })).toBeVisible());
  });

  it('shows only the remaining actionable session after termination refetches the list', async () => {
    const peer = { ...session, id: 'peer', service_id: 'cobalt', service_display_name: 'Cobalt Notes' };
    const pending = pendingSessionList();
    vi.mocked(sessionsApi.listSessions)
      .mockResolvedValueOnce([session, peer])
      .mockReturnValueOnce(pending.promise);
    vi.mocked(sessionsApi.getSessionDetails).mockResolvedValue({ session, dependent_agents: [] });
    vi.mocked(sessionsApi.terminateSession).mockResolvedValue(undefined);
    render(<MemoryRouter><ThirdPartySessionsPage /></MemoryRouter>);
    const [terminate] = await screen.findAllByRole('button', { name: 'Terminate' });
    fireEvent.click(terminate);
    fireEvent.click(await screen.findByRole('button', { name: 'Terminate session with Atlas Files' }));
    await waitFor(() => expect(sessionsApi.listSessions).toHaveBeenCalledTimes(2));
    expect(sessionsApi.terminateSession).toHaveBeenCalledWith('atlas');
    expect(screen.queryByRole('button', { name: 'Terminate' })).not.toBeInTheDocument();
    await act(async () => pending.resolve([peer]));
    expect(screen.queryByRole('heading', { name: 'Atlas Files' })).not.toBeInTheDocument();
    await waitFor(() => expect(screen.getByRole('heading', { name: 'Cobalt Notes' })).toBeVisible());
    expect(screen.getByRole('button', { name: 'Terminate' })).toBeEnabled();
  });
});
