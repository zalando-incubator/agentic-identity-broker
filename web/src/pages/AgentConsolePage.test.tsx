import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { act, fireEvent, screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { Toaster, toast } from 'sonner';
import { consentApi } from '@services/api/consent';
import { sessionsApi } from '@services/api/sessions';
import { queryKeys } from '@services/query/queryKeys';
import { AgentConsolePage } from './AgentConsolePage';
import { detail, grant as decisionGrant, renderAgentPage } from './agentConsentTestSupport';
import { loadConsentDraft } from '@services/storage/session';
import type { GrantResult } from '../types/consent';
const grant = { ...decisionGrant, granted_permission_sets: { read: ['mail'], prior: ['drive', 'mail'] } };
const dayFormatter = new Intl.DateTimeFormat(undefined, { day: 'numeric', month: 'short', year: 'numeric' });
const dateText = (day: string) => dayFormatter.format(new Date(`${day}T15:24:31.123Z`));


vi.mock('@services/api/consent', () => ({ consentApi: { getUserInfo: vi.fn(), getAgentDetail: vi.fn(), getAgentGrants: vi.fn(), createOrUpdateGrant: vi.fn(), deleteGrant: vi.fn() } }));
vi.mock('@services/api/sessions', () => ({ sessionsApi: { listSessions: vi.fn(), refreshSession: vi.fn() } }));
beforeEach(() => {
  vi.resetAllMocks();
  sessionStorage.clear();
  vi.mocked(consentApi.getUserInfo).mockResolvedValue({ principal: 'alice', displayName: 'Alice' });
  vi.mocked(consentApi.getAgentDetail).mockResolvedValue(detail);
  vi.mocked(consentApi.getAgentGrants).mockResolvedValue([grant]);
  vi.mocked(sessionsApi.listSessions).mockResolvedValue([]);
});
afterEach(() => { vi.restoreAllMocks(); sessionStorage.clear(); toast.dismiss(); });

describe('agent grant management', () => {
  it('preserves edits when navigation is cancelled and leaves only after explicit discard', async () => {
    const user = userEvent.setup();
    renderAgentPage(<AgentConsolePage />);
    await user.click(await screen.findByRole('checkbox', { name: 'Write documents', exact: true }));
    await user.click(screen.getByRole('link', { name: 'Back to Agents' }));
    const dialog = await screen.findByRole('dialog', { name: 'Discard unsaved changes?' });
    expect(screen.getByLabelText('Current route')).toHaveTextContent('/agents/agent');
    expect(within(dialog).getByRole('button', { name: 'Keep editing' })).toHaveFocus();
    await user.click(within(dialog).getByRole('button', { name: 'Keep editing' }));
    expect(screen.getByRole('checkbox', { name: 'Write documents', exact: true })).toBeChecked();
    expect(screen.getByTestId('grant-save-bar')).toBeVisible();
    await user.click(screen.getByRole('link', { name: 'Back to Agents' }));
    await user.click(within(await screen.findByRole('dialog')).getByRole('button', { name: 'Discard changes' }));
    expect(await screen.findByText('Agent list')).toBeVisible();
    expect(consentApi.createOrUpdateGrant).not.toHaveBeenCalled();
  });

  it('warns on tab departure only while edits are unsaved and previews the duration before saving', async () => {
    const user = userEvent.setup();
    renderAgentPage(<AgentConsolePage />);
    await screen.findByRole('heading', { name: detail.agent.displayName });
    const cleanDeparture = new Event('beforeunload', { cancelable: true });
    window.dispatchEvent(cleanDeparture);
    expect(cleanDeparture.defaultPrevented).toBe(false);
    await user.click(screen.getByRole('combobox', { name: 'Access lasts' }));
    await user.click(await screen.findByRole('option', { name: 'Until I revoke it' }));
    expect(screen.getByText('After saving:').parentElement).toHaveTextContent('Until revoked');
    expect(screen.getByText('Current access:').parentElement).toHaveTextContent(dateText('2099-06-10'));
    const dirtyDeparture = new Event('beforeunload', { cancelable: true });
    window.dispatchEvent(dirtyDeparture);
    expect(dirtyDeparture.defaultPrevented).toBe(true);
    await user.click(screen.getByRole('button', { name: 'Cancel', exact: true }));
    const cancelledDeparture = new Event('beforeunload', { cancelable: true });
    window.dispatchEvent(cancelledDeparture);
    expect(cancelledDeparture.defaultPrevented).toBe(false);
    expect(screen.queryByText('After saving:')).not.toBeInTheDocument();
  });

  it('owns one connection action per service in Connections with always-visible permission choices', async () => {
    vi.mocked(consentApi.getAgentDetail).mockResolvedValue({ ...detail,
      agent: { ...detail.agent, active_session_service_ids: [] },
      services: detail.services.map(service => ({ ...service, connectionStatus: 'not_connected' as const })),
    });
    const submit = vi.spyOn(HTMLFormElement.prototype, 'submit').mockImplementation(function (this: HTMLFormElement) {
      expect(new URL(this.action).pathname).toBe('/api/third-party/mail/oauth2/authorize');
      const form = new FormData(this);
      expect(loadConsentDraft(String(form.get('consent_state_id')), 'mail', '/agents/agent')).toMatchObject({ selections: grant.granted_permission_sets });
      this.remove();
    });
    const user = userEvent.setup();
    renderAgentPage(<AgentConsolePage />);
    await screen.findByRole('heading', { name: detail.agent.displayName });
    const permissions = screen.getByTestId('permission-groups');
    const rail = screen.getByTestId('agent-connections');
    await within(rail).findAllByText('No connection');
    expect(screen.getAllByRole('button', { name: /^Connect(?: |$)/ })).toHaveLength(2);
    expect(within(permissions).queryByRole('button', { name: /^Connect(?: |$)/ })).not.toBeInTheDocument();
    const priorGroup = within(permissions).getByRole('checkbox', { name: 'Existing access', exact: true }).closest('li')!;
    expect(within(priorGroup).getByRole('button', { name: 'Drive', exact: true })).toHaveAttribute('aria-pressed', 'true');
    await user.click(within(rail).getByRole('button', { name: 'Connect Mail', exact: true }));
    expect(submit).toHaveBeenCalledOnce();
    expect(consentApi.createOrUpdateGrant).not.toHaveBeenCalled();
  });

  it('keeps client URI metadata in technical details without changing the detail header', async () => {
    const { client } = renderAgentPage(<AgentConsolePage />);
    const heading = await screen.findByRole('heading', { name: detail.agent.displayName });
    const headerText = heading.closest('header')!.textContent;
    const uri = 'https://example.org/agent-metadata.json';
    vi.mocked(consentApi.getAgentDetail).mockResolvedValue({ ...detail, agent: { ...detail.agent, clientUris: [uri] } });
    await act(async () => { await client.refetchQueries(); });
    expect(within(screen.getByTestId('agent-technical-details')).getByText(uri)).toBeVisible();
    expect(heading.closest('header')!.textContent).toBe(headerText);
  });
  it('edits optional existing groups only in the console and discards changes on cancel', async () => {
    const { container } = renderAgentPage(<AgentConsolePage />);
    expect(await screen.findByRole('heading', { name: detail.agent.displayName })).toBeVisible();
    const header = within(screen.getByRole('heading', { name: detail.agent.displayName }).closest('header')!);
    expect(header.getByText('Access until')).toHaveTextContent(`Access until ${dateText('2099-06-10')}`);
    expect(header.getByText(/Changed/)).toBeVisible();
    expect(screen.queryByText(/publisher/i)).not.toBeInTheDocument();
    expect(within(screen.getByTestId('agent-about')).getByRole('link', { name: 'Governance' })).toHaveAttribute('href', detail.agent.governanceUrl);
    expect(container.querySelector('img[src^="https://external.example"]')).toBeNull();
    expect(screen.queryByRole('tab')).not.toBeInTheDocument();
    expect(screen.getByTestId('agent-connections')).toBeVisible();
    const required = screen.getAllByTestId('permission-group')[0]!;
    expect(required).toHaveAttribute('data-selected', 'true');
    expect(required).toHaveAttribute('data-read-only', 'true');
    expect(within(required).getByText('Required')).toBeVisible();
    expect(within(required).queryByRole('checkbox', { name: 'Read mail', exact: true })).not.toBeInTheDocument();
    expect(screen.queryByTestId('grant-save-bar')).not.toBeInTheDocument();
    await userEvent.click(screen.getByRole('checkbox', { name: 'Existing access', exact: true }));
    expect(screen.getByTestId('grant-save-bar')).toBeVisible();
    expect(screen.getByRole('checkbox', { name: 'Existing access', exact: true })).not.toBeChecked();
    await userEvent.click(screen.getByRole('button', { name: 'Cancel', exact: true }));
    expect(screen.getByRole('checkbox', { name: 'Existing access', exact: true })).toBeChecked();
    expect(screen.queryByTestId('grant-save-bar')).not.toBeInTheDocument();
    expect(consentApi.createOrUpdateGrant).not.toHaveBeenCalled();
  });
  it('saves a dirty draft only after server acceptance and preserves exact existing expiry', async () => {
    let resolve!: (value: GrantResult) => void;
    vi.mocked(consentApi.createOrUpdateGrant).mockReturnValue(new Promise<GrantResult>((done) => { resolve = done; }));
    renderAgentPage(<><AgentConsolePage /><Toaster /></>);
    await userEvent.click(await screen.findByRole('checkbox', { name: 'Write documents', exact: true }));
    const addition = screen.getByRole('checkbox', { name: 'Write documents', exact: true }).closest('li')!;
    const removal = screen.getByRole('checkbox', { name: 'Existing access', exact: true }).closest('li')!;
    expect(within(addition).getByText('Addition pending')).toBeVisible();
    expect(within(addition).queryByText('Granted')).not.toBeInTheDocument();
    await userEvent.click(within(removal).getByRole('button', { name: 'Drive', exact: true }));
    expect(within(removal).getByText('Granted')).toBeVisible();
    expect(within(removal).queryByText(/pending/)).not.toBeInTheDocument();
    await userEvent.click(within(removal).getByRole('checkbox', { name: 'Existing access', exact: true }));
    expect(within(removal).getByText('Granted')).toBeVisible();
    expect(within(removal).getByText('Removal pending')).toBeVisible();
    await userEvent.click(screen.getByRole('button', { name: 'Save changes' }));
    expect(screen.getByTestId('grant-save-bar')).toBeVisible();
    expect(screen.getByRole('button', { name: 'Save changes' })).toBeDisabled();
    expect(screen.queryByText('Grant updated successfully.')).not.toBeInTheDocument();
    expect(within(addition).getByText('Addition pending')).toBeVisible();
    expect(within(removal).getByText('Removal pending')).toBeVisible();
    const updated = { ...grant, granted_permission_sets: { read: ['mail'], write: ['drive'] } };
    vi.mocked(consentApi.getAgentGrants).mockResolvedValue([updated]);
    resolve({ kind: 'created', grant: updated });
    await waitFor(() => expect(screen.queryByTestId('grant-save-bar')).not.toBeInTheDocument());
    expect(consentApi.createOrUpdateGrant).toHaveBeenCalledWith('agent', { granted_permission_sets: updated.granted_permission_sets, valid_until: grant.valid_until }, undefined);
    expect(await screen.findByText('Grant updated successfully.')).toBeVisible();
    expect(within(addition).getByText('Granted')).toBeVisible();
    expect(within(addition).queryByText('Addition pending')).not.toBeInTheDocument();
    expect(within(removal).queryByText('Granted')).not.toBeInTheDocument();
    expect(within(removal).queryByText('Removal pending')).not.toBeInTheDocument();
  });
  it('keeps unsaved edits visible after a rejected save', async () => {
    vi.mocked(consentApi.createOrUpdateGrant).mockRejectedValue({ status: 403, code: 'FORBIDDEN', message: "You don't have permission to access this resource." });
    renderAgentPage(<AgentConsolePage />);
    await userEvent.click(await screen.findByRole('combobox', { name: 'Access lasts' }));
    await userEvent.click(await screen.findByRole('option', { name: 'Until I revoke it' }));
    await userEvent.click(screen.getByRole('checkbox', { name: 'Write documents', exact: true }));
    await userEvent.click(screen.getByRole('checkbox', { name: 'Existing access', exact: true }));
    await userEvent.click(screen.getByRole('button', { name: 'Save changes' }));
    expect(await screen.findByRole('alert')).toHaveTextContent("You don't have permission to access this resource.");
    expect(screen.getByTestId('grant-save-bar')).toBeVisible();
    expect(screen.getByRole('combobox', { name: 'Access lasts' })).toHaveTextContent('Until I revoke it');
    const addition = screen.getByRole('checkbox', { name: 'Write documents', exact: true }).closest('li')!;
    const removal = screen.getByRole('checkbox', { name: 'Existing access', exact: true }).closest('li')!;
    expect(within(addition).getByText('Addition pending')).toBeVisible();
    expect(within(addition).queryByText('Granted')).not.toBeInTheDocument();
    expect(within(removal).getByText('Granted')).toBeVisible();
    expect(within(removal).getByText('Removal pending')).toBeVisible();
  });
  it('requires named confirmation and navigates only after server-confirmed revoke', async () => {
    const response = Promise.withResolvers<void>();
    vi.mocked(consentApi.deleteGrant).mockReturnValue(response.promise);
    renderAgentPage(<AgentConsolePage />);
    await userEvent.click(await screen.findByRole('button', { name: 'Revoke access' }));
    expect(screen.getByRole('dialog')).toHaveTextContent(detail.agent.displayName);
    await userEvent.click(within(screen.getByRole('dialog')).getByRole('button', { name: 'Cancel' }));
    expect(consentApi.deleteGrant).not.toHaveBeenCalled();
    await userEvent.click(screen.getByRole('button', { name: 'Revoke access' }));
    await userEvent.click(within(screen.getByRole('dialog')).getByRole('button', { name: 'Revoke' }));
    await waitFor(() => expect(consentApi.deleteGrant).toHaveBeenCalledWith('agent'));
    expect(screen.getByLabelText('Current route')).toHaveTextContent('/agents/agent');
    await act(async () => response.resolve());
    await screen.findByText('Agent list');
  });
  it.each([
    { isExpired: true, accessExpired: true, label: 'Expired' },
    { isExpired: false, accessExpired: true, label: 'Needs sign-in' },
    { isExpired: false, accessExpired: false, label: 'Connected' },
  ])('shows the stored connection as $label when the agent requirement says not connected', async ({ isExpired, accessExpired, label }) => {
    vi.mocked(consentApi.getAgentDetail).mockResolvedValue({ ...detail, services: [{ ...detail.services[0]!, connectionStatus: 'not_connected' }] });
    vi.mocked(sessionsApi.listSessions).mockResolvedValue([{
      id: 'mail-session', service_id: 'mail', service_display_name: 'Mail', token_type: 'Bearer', scope: ['read'],
      initiated_at: '2026-09-01T00:00:00Z', is_expired: isExpired, access_token_expired: accessExpired,
      has_refresh_token: true, dependent_agent_count: 1, is_encrypted: true,
    }]);
    const submit = vi.spyOn(HTMLFormElement.prototype, 'submit').mockImplementation(function (this: HTMLFormElement) {
      expect(new URL(this.action).pathname).toBe('/api/third-party/mail/oauth2/authorize');
      const form = new FormData(this);
      expect(loadConsentDraft(String(form.get('consent_state_id')), 'mail', '/agents/agent')).toMatchObject({ selections: grant.granted_permission_sets });
      this.remove();
    });
    renderAgentPage(<AgentConsolePage />);
    const rail = await screen.findByTestId('agent-connections');
    expect(await within(rail).findByText(label)).toBeVisible();
    expect(within(rail).queryByText('No connection')).not.toBeInTheDocument();
    expect(within(rail).queryByRole('button', { name: /^Connect / })).not.toBeInTheDocument();
    if (label === 'Connected') {
      expect(within(rail).getByRole('link', { name: 'Manage connection' })).toHaveAttribute('href', '/connections');
      expect(submit).not.toHaveBeenCalled();
    } else {
      await userEvent.click(within(rail).getByRole('button', { name: 'Reconnect Mail', exact: true }));
      expect(submit).toHaveBeenCalledOnce();
    }
  });
  it('keeps a failed stored-connections read unavailable until a successful retry proves there is no connection', async () => {
    vi.mocked(consentApi.getAgentDetail).mockResolvedValue({ ...detail, services: [{ ...detail.services[0]!, connectionStatus: 'not_connected' }] });
    vi.mocked(sessionsApi.listSessions).mockRejectedValueOnce(new Error('offline'));
    const submit = vi.spyOn(HTMLFormElement.prototype, 'submit').mockImplementation(function (this: HTMLFormElement) {
      expect(new URL(this.action).pathname).toBe('/api/third-party/mail/oauth2/authorize');
    });
    const user = userEvent.setup();
    renderAgentPage(<AgentConsolePage />);
    const rail = await screen.findByTestId('agent-connections');
    expect(await within(rail).findByRole('alert')).toBeVisible();
    expect(within(rail).getByText('Unavailable')).toBeVisible();
    expect(within(rail).queryByText('No connection')).not.toBeInTheDocument();
    expect(within(rail).queryByRole('button', { name: /^Connect / })).not.toBeInTheDocument();
    expect(consentApi.getAgentDetail).toHaveBeenCalledWith('agent', expect.objectContaining({ signal: expect.any(AbortSignal) }));
    expect(sessionsApi.listSessions).toHaveBeenCalledTimes(1);
    expect(submit).not.toHaveBeenCalled();

    await user.click(within(rail).getByRole('button', { name: 'Try again' }));
    expect(await within(rail).findByText('No connection')).toBeVisible();
    expect(within(rail).queryByRole('alert')).not.toBeInTheDocument();
    expect(within(rail).queryByText('Unavailable')).not.toBeInTheDocument();
    expect(sessionsApi.listSessions).toHaveBeenCalledTimes(2);
    await user.click(within(rail).getByRole('button', { name: 'Connect Mail', exact: true }));
    expect(submit).toHaveBeenCalledOnce();
  });
  it('posts a saved draft from the Connections rail instead of leaking it in a URL', async () => {
    vi.mocked(consentApi.getAgentDetail).mockResolvedValue({ ...detail, services: [{ ...detail.services[0]!, connectionStatus: 'not_connected' }] });
    const submit = vi.spyOn(HTMLFormElement.prototype, 'submit').mockImplementation(function (this: HTMLFormElement) {
      expect(new URL(this.action).pathname).toBe('/api/third-party/mail/oauth2/authorize');
      expect(new URL(this.action).search).toBe('');
      const form = new FormData(this);
      expect(form.get('redirect_uri')).toBe(`${window.location.origin}/agents/agent`);
      expect(loadConsentDraft(String(form.get('consent_state_id')), 'mail', '/agents/agent')).toMatchObject({
        selections: grant.granted_permission_sets,
        duration: 'custom', customDate: '2099-06-10',
      });
      this.remove();
    });
    renderAgentPage(<AgentConsolePage />);
    const rail = await screen.findByTestId('agent-connections');
    expect(await within(rail).findByText('No connection')).toBeVisible();
    await userEvent.click(within(rail).getByRole('button', { name: 'Connect Mail', exact: true }));
    expect(submit).toHaveBeenCalledOnce();
    expect(consentApi.createOrUpdateGrant).not.toHaveBeenCalled();
  });
  it('stays on the agent and explains how to retry when tab storage refuses a connection', async () => {
    vi.mocked(consentApi.getAgentDetail).mockResolvedValue({ ...detail, services: [{ ...detail.services[0]!, connectionStatus: 'not_connected' }] });
    vi.spyOn(Storage.prototype, 'setItem').mockImplementation(() => { throw new Error('storage unavailable'); });
    const submit = vi.spyOn(HTMLFormElement.prototype, 'submit').mockImplementation(() => {});
    renderAgentPage(<AgentConsolePage />);
    const rail = await screen.findByTestId('agent-connections');
    expect(await within(rail).findByText('No connection')).toBeVisible();
    await userEvent.click(within(rail).getByRole('button', { name: 'Connect Mail', exact: true }));
    expect(screen.getByRole('alert')).toHaveTextContent('Enable browser storage and try connecting again');
    expect(submit).not.toHaveBeenCalled();
    expect(consentApi.createOrUpdateGrant).not.toHaveBeenCalled();
  });
  it('keeps About absent for unconfigured descriptions and links', async () => {
    vi.mocked(consentApi.getAgentDetail).mockResolvedValue({ ...detail, cimd_metadata: undefined, agent: { ...detail.agent, description: '', governanceUrl: undefined, userDocumentationUrl: undefined, agentInterfaceUrl: undefined } });
    const { container } = renderAgentPage(<AgentConsolePage />);
    expect(await screen.findByRole('heading', { name: detail.agent.displayName })).toBeVisible();
    expect(screen.queryByTestId('agent-about')).not.toBeInTheDocument();
    expect(container.querySelector('img[src^="https://"]')).toBeNull();
  });

  it('shows a description only in About even without configured links', async () => {
    vi.mocked(consentApi.getAgentDetail).mockResolvedValue({ ...detail, agent: { ...detail.agent, description: 'Agent issued local tokens (no upstream client_id)', governanceUrl: undefined, userDocumentationUrl: undefined, agentInterfaceUrl: undefined } });
    renderAgentPage(<AgentConsolePage />);
    const heading = await screen.findByRole('heading', { name: detail.agent.displayName });
    expect(within(heading.closest('header')!).queryByText('Agent issued local tokens (no upstream client_id)')).not.toBeInTheDocument();
    const about = screen.getByTestId('agent-about');
    expect(within(about).getByText('Agent issued local tokens (no upstream client_id)')).toBeVisible();
    expect(within(about).queryByRole('link')).not.toBeInTheDocument();
  });

  it('shows exact registered identifiers and URLs in technical details, separate from the header and About', async () => {
    const clientId = 'https://agents.example.org/clients/research-agent/metadata.json';
    const clientUris = ['https://agents.example.org/products/research-agent?tenant=operations', 'https://agents.example.org/agents/research-agent/technical-information'];
    const saved = { ...grant, updated_at: '2026-02-01T00:00:00Z' };
    vi.mocked(consentApi.getAgentDetail).mockResolvedValue({ ...detail, agent: { ...detail.agent, clientId, clientUris } });
    vi.mocked(consentApi.getAgentGrants).mockResolvedValue([saved]);
    renderAgentPage(<AgentConsolePage />);
    const technical = await screen.findByRole('region', { name: 'Technical details' });
    expect(within(technical).getByText(detail.agent.agentId)).toBeVisible();
    expect(within(technical).getByText(clientId)).toBeVisible();
    for (const uri of clientUris) expect(within(technical).getByText(uri)).toBeVisible();
    expect(within(technical).getByText(saved.id)).toBeVisible();
    expect(within(technical).getByText(saved.created_at)).toBeVisible();
    expect(within(technical).getByText(saved.updated_at)).toBeVisible();
    const header = screen.getByRole('heading', { name: detail.agent.displayName }).closest('header')!;
    expect(header).toHaveTextContent('Access until');
    expect(header).toHaveTextContent('Changed');
    expect(header).not.toHaveTextContent('agents.example.org');
    expect(within(screen.getByTestId('agent-about')).getByRole('link', { name: 'Governance' })).toHaveAttribute('href', detail.agent.governanceUrl);
    expect(within(technical).queryByText(detail.cimd_metadata!.redirect_uri)).not.toBeInTheDocument();
    expect(within(technical).queryByText('Requested scopes')).not.toBeInTheDocument();
  });

  it.each([undefined, []])('omits absent optional technical data when no client URIs are provided (%s)', async (clientUris) => {
    vi.mocked(consentApi.getAgentDetail).mockResolvedValue({ ...detail, agent: { ...detail.agent, clientId: undefined, clientUris } });
    vi.mocked(consentApi.getAgentGrants).mockResolvedValue([]);
    renderAgentPage(<AgentConsolePage />);
    const technical = await screen.findByRole('region', { name: 'Technical details' });
    expect(within(technical).getByText(detail.agent.agentId)).toBeVisible();
    for (const label of ['Client ID', 'Registered client URIs', 'Grant ID', 'Grant created', 'Grant updated', 'Redirect URI', 'Requested scopes']) {
      expect(within(technical).queryByText(label)).not.toBeInTheDocument();
    }
    expect(within(technical).queryByText(detail.cimd_metadata!.client_id_url)).not.toBeInTheDocument();
  });

  it.each([
    { clientId: 'opaque-client-id', clientUris: undefined, shown: 'opaque-client-id', absent: 'Registered client URIs' },
    { clientId: undefined, clientUris: ['https://agents.example.org/client-a'], shown: 'https://agents.example.org/client-a', absent: 'Client ID' },
  ])('shows only supplied $shown without inventing other client metadata', async ({ clientId, clientUris, shown, absent }) => {
    vi.mocked(consentApi.getAgentDetail).mockResolvedValue({ ...detail, agent: { ...detail.agent, clientId, clientUris } });
    renderAgentPage(<AgentConsolePage />);
    const technical = await screen.findByRole('region', { name: 'Technical details' });
    expect(within(technical).getByText(shown)).toBeVisible();
    expect(within(technical).queryByText(absent)).not.toBeInTheDocument();
    expect(within(technical).queryByText(detail.cimd_metadata!.client_id_url)).not.toBeInTheDocument();
  });

  it('shows permissions without a Revoke control when no grant exists', async () => {
    vi.mocked(consentApi.getAgentGrants).mockResolvedValue([]);
    renderAgentPage(<AgentConsolePage />);
    expect(await screen.findByRole('heading', { name: detail.agent.displayName })).toBeVisible();
    expect(screen.getByTestId('permission-groups')).toBeVisible();
    expect(screen.queryByRole('button', { name: 'Revoke access' })).not.toBeInTheDocument();
    expect(consentApi.deleteGrant).not.toHaveBeenCalled();
  });

  it('keeps a dirty immutable draft on a failed grant refresh and offers retry', async () => {
    const { client } = renderAgentPage(<AgentConsolePage />);
    await userEvent.click(await screen.findByRole('checkbox', { name: 'Existing access', exact: true }));
    vi.mocked(consentApi.getAgentGrants).mockRejectedValue(new Error('Unavailable'));
    await act(async () => { await client.invalidateQueries({ queryKey: queryKeys.grant('alice', 'agent') }); });
    expect(await screen.findByText(/details may be out of date/i)).toBeVisible();
    expect(screen.getByRole('button', { name: 'Try again' })).toBeVisible();
    expect(screen.getByRole('checkbox', { name: 'Existing access', exact: true })).not.toBeChecked();
    expect(screen.getByTestId('grant-save-bar')).toBeVisible();
    expect(consentApi.createOrUpdateGrant).not.toHaveBeenCalled();
  });

  it('edits duration directly without saving and cancels back to the saved expiry', async () => {
    const user = userEvent.setup();
    renderAgentPage(<AgentConsolePage />);
    const control = await screen.findByRole('combobox', { name: 'Access lasts' });
    expect(consentApi.createOrUpdateGrant).not.toHaveBeenCalled();
    control.focus();
    await user.keyboard('{Enter}');
    expect(await screen.findByRole('listbox')).toBeVisible();
    expect(screen.queryByTestId('grant-save-bar')).not.toBeInTheDocument();
    await user.click(screen.getByRole('option', { name: '30 days' }));
    expect(screen.getByRole('combobox', { name: 'Access lasts' })).toHaveTextContent('30 days');
    expect(screen.getByTestId('grant-save-bar')).toBeVisible();
    expect(screen.getByText('Access until')).toHaveTextContent(`Access until ${dateText('2099-06-10')}`);
    await user.click(screen.getByRole('button', { name: 'Cancel', exact: true }));
    await user.click(screen.getByRole('button', { name: 'Choose custom date' }));
    expect(await screen.findByLabelText('Custom date', { selector: 'input' })).toHaveValue('2099-06-10');
    expect(screen.queryByTestId('grant-save-bar')).not.toBeInTheDocument();
    expect(consentApi.createOrUpdateGrant).not.toHaveBeenCalled();
  });

  it('preserves an edited date across an equivalent grant refresh and cancels to the saved date', async () => {
    const { client } = renderAgentPage(<AgentConsolePage />);
    await userEvent.click(await screen.findByRole('button', { name: 'Choose custom date' }));
    const date = await screen.findByLabelText('Custom date', { selector: 'input' });
    fireEvent.change(date, { target: { value: '2099-08-12' } });
    vi.mocked(consentApi.getAgentGrants).mockResolvedValue([{ ...grant, updated_at: '2026-02-01T00:00:00Z' }]);
    await act(async () => { await client.invalidateQueries({ queryKey: queryKeys.grant('alice', 'agent') }); });
    expect(screen.getByLabelText('Custom date', { selector: 'input' })).toHaveValue('2099-08-12');
    expect(screen.getByTestId('grant-save-bar')).toBeVisible();
    await userEvent.click(screen.getByRole('button', { name: 'Cancel', exact: true }));
    await userEvent.click(screen.getByRole('button', { name: 'Choose custom date' }));
    expect(await screen.findByLabelText('Custom date', { selector: 'input' })).toHaveValue('2099-06-10');
    expect(consentApi.createOrUpdateGrant).not.toHaveBeenCalled();
  });
  it.each(['2099-09-14T16:45:12.345Z', undefined])('rehydrates a clean editor when saved validity changes to %s', async (validUntil) => {
    const { client } = renderAgentPage(<AgentConsolePage />);
    await userEvent.click(await screen.findByRole('button', { name: 'Choose custom date' }));
    expect(await screen.findByLabelText('Custom date', { selector: 'input' })).toHaveValue('2099-06-10');
    const refreshedGrant = { ...grant };
    if (validUntil) refreshedGrant.valid_until = validUntil;
    else delete refreshedGrant.valid_until;
    vi.mocked(consentApi.getAgentGrants).mockResolvedValue([refreshedGrant]);
    await act(async () => { await client.invalidateQueries({ queryKey: queryKeys.grant('alice', 'agent') }); });
    if (validUntil) {
      await waitFor(() => expect(screen.getByLabelText('Custom date', { selector: 'input' })).toHaveValue('2099-09-14'));
      expect(screen.getByText('Access until')).toHaveTextContent(`Access until ${dateText('2099-09-14')}`);
    } else {
      await waitFor(() => expect(screen.getByRole('combobox', { name: 'Access lasts' })).toHaveTextContent('Until I revoke it'));
      expect(screen.queryByLabelText('Custom date', { selector: 'input' })).not.toBeInTheDocument();
    }
    expect(screen.queryByTestId('grant-save-bar')).not.toBeInTheDocument();
    expect(consentApi.createOrUpdateGrant).not.toHaveBeenCalled();
  });
  it('keeps an accepted save authoritative when the follow-up grant read fails', async () => {
    renderAgentPage(<><AgentConsolePage /><Toaster /></>);
    await userEvent.click(await screen.findByRole('button', { name: 'Choose custom date' }));
    const date = await screen.findByLabelText('Custom date', { selector: 'input' });
    fireEvent.change(date, { target: { value: '2099-08-12' } });
    vi.mocked(consentApi.createOrUpdateGrant).mockResolvedValue({
      kind: 'created', grant: { ...grant, valid_until: '2099-08-12T00:00:00.000Z' },
    });
    vi.mocked(consentApi.getAgentGrants).mockRejectedValue({ status: 503, code: 'SERVER_ERROR', message: 'Service temporarily unavailable. Please try again later.' });
    await userEvent.click(screen.getByRole('button', { name: 'Save changes' }));
    await screen.findByText('Grant updated successfully.');
    expect(screen.getByText('Access until')).toHaveTextContent(`Access until ${dateText('2099-08-12')}`);
    await userEvent.click(screen.getByRole('button', { name: 'Choose custom date' }));
    expect(await screen.findByLabelText('Custom date', { selector: 'input' })).toHaveValue('2099-08-12');
    expect(screen.queryByTestId('grant-save-bar')).not.toBeInTheDocument();
    expect(consentApi.createOrUpdateGrant).toHaveBeenCalledTimes(1);
  });
  it('announces date validation on the directly available duration control', async () => {
    renderAgentPage(<AgentConsolePage />);
    await userEvent.click(await screen.findByRole('button', { name: 'Choose custom date' }));
    fireEvent.change(await screen.findByLabelText('Custom date', { selector: 'input' }), { target: { value: '2000-01-01' } });
    await userEvent.click(screen.getByRole('button', { name: 'Save changes' }));
    const trigger = screen.getByRole('combobox', { name: 'Access lasts' });
    expect(screen.getAllByRole('alert').some((alert) => alert.textContent?.includes('Choose a date after today.'))).toBe(true);
    expect(trigger).toHaveAttribute('aria-invalid', 'true');
    expect(trigger).toHaveAccessibleDescription('Choose a date after today.');
    expect(consentApi.createOrUpdateGrant).not.toHaveBeenCalled();
    expect(await screen.findByLabelText('Custom date', { selector: 'input' })).toHaveValue('2000-01-01');
  });

  it('restores a clean grant and exact expiry after keyboard duration exploration', async () => {
    const user = userEvent.setup();
    vi.mocked(consentApi.createOrUpdateGrant).mockResolvedValue({ kind: 'created', grant });
    renderAgentPage(<AgentConsolePage />);
    await user.click(await screen.findByRole('combobox', { name: 'Access lasts' }));
    expect(await screen.findByRole('listbox')).toBeVisible();
    await user.keyboard('{Home}{Enter}');
    expect(screen.getByRole('combobox', { name: 'Access lasts' })).toHaveTextContent('Until I revoke it');
    expect(screen.getByTestId('grant-save-bar')).toBeVisible();
    await user.click(screen.getByRole('combobox', { name: 'Access lasts' }));
    await user.click(screen.getByRole('option', { name: '30 days' }));
    expect(screen.getByTestId('grant-save-bar')).toBeVisible();
    await user.click(screen.getByRole('combobox', { name: 'Access lasts' }));
    await user.click(screen.getByRole('option', { name: 'Custom date' }));
    expect(screen.getByRole('combobox', { name: 'Access lasts' })).toHaveTextContent('Custom date');
    expect(screen.queryByTestId('grant-save-bar')).not.toBeInTheDocument();
    expect(await screen.findByLabelText('Custom date', { selector: 'input' })).toHaveValue('2099-06-10');
    expect(consentApi.createOrUpdateGrant).not.toHaveBeenCalled();
    await user.click(screen.getByRole('checkbox', { name: 'Write documents', exact: true }));
    await user.click(screen.getByRole('button', { name: 'Save changes' }));
    await waitFor(() => expect(consentApi.createOrUpdateGrant).toHaveBeenCalledWith('agent', {
      granted_permission_sets: { read: ['mail'], prior: ['drive', 'mail'], write: ['drive'] },
      valid_until: grant.valid_until,
    }, undefined));
  });
});
