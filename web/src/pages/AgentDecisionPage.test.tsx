import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { fireEvent, screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { consentApi } from '@services/api/consent';
import { accessCopy } from '@copy';
import { AgentDecisionPage } from './AgentDecisionPage';
import { createConsentDraft } from '@components/consent/consentDraft';
import { startServiceLogin } from '@components/consent/startServiceLogin';
import { loadConsentDraft } from '@services/storage/session';
import { detail, grant, renderAgentPage } from './agentConsentTestSupport';

vi.mock('@services/api/consent', () => ({ consentApi: { getUserInfo: vi.fn(), getAgentDetail: vi.fn(), getAgentGrants: vi.fn(), createOrUpdateGrant: vi.fn(), deleteGrant: vi.fn() } }));
beforeEach(() => {
  vi.resetAllMocks();
  sessionStorage.clear();
  vi.mocked(consentApi.getUserInfo).mockResolvedValue({ principal: 'alice', displayName: 'Alice' });
  vi.mocked(consentApi.getAgentDetail).mockResolvedValue(detail);
  vi.mocked(consentApi.getAgentGrants).mockResolvedValue([grant]);
  vi.mocked(consentApi.createOrUpdateGrant).mockResolvedValue({ kind: 'created', grant });
});
afterEach(() => { vi.restoreAllMocks(); sessionStorage.clear(); });
const open = (query = '?session_token=authorization') => renderAgentPage(<AgentDecisionPage />, query);

describe('focused consent decisions', () => {
  it('shows trusted domain and independent local callback warning without remote images or console navigation', async () => {
    const { container } = open();
    await screen.findByRole('heading', { name: `${detail.agent.displayName} wants to act on your behalf` });
    const card = screen.getByTestId('consent-card');
    const account = screen.getByTestId('consent-account');
    expect(card).not.toContainElement(account);
    expect(account).toHaveTextContent('Signed in as Alice');
    expect(screen.getAllByText('Signed in as Alice')).toHaveLength(1);
    const identity = within(card).getByTestId('agent-identity');
    expect(within(identity).getByRole('heading', { name: `${detail.agent.displayName} wants to act on your behalf` }).parentElement).toContainElement(within(identity).getByRole('img', { name: detail.agent.displayName }));
    expect(within(card).queryByRole('img', { name: 'Alice' })).not.toBeInTheDocument();
    expect(screen.queryByRole('navigation')).not.toBeInTheDocument();
    expect(container.querySelector('img[src^="https://external.example"]')).toBeNull();
    expect(screen.getByText('Verified domain: trusted.example')).toBeVisible();
    expect(screen.getByText('Returns to localhost:3001')).toBeVisible();
    expect(screen.getByTestId('localhost-warning')).toHaveTextContent('own computer');
    await userEvent.click(screen.getByRole('button', { name: 'About this agent' }));
    expect(await screen.findByRole('link', { name: 'Governance' })).toHaveAttribute('rel', 'noopener noreferrer');
    expect(screen.queryByText(/publisher/i)).not.toBeInTheDocument();
    expect(screen.getByTestId('consent-next-steps')).toHaveTextContent('Agents');
    expect(screen.getByRole('link', { name: 'Agents' })).toHaveAttribute('href', '/agents');
    expect(screen.getByRole('button', { name: 'Deny' })).toHaveAttribute('data-variant', 'outline');
    expect(container.querySelectorAll('[data-variant="primary"]')).toHaveLength(1);
    expect(consentApi.createOrUpdateGrant).not.toHaveBeenCalled();
  });
  it('uses the authenticated principal when the account has no display name', async () => {
    vi.mocked(consentApi.getUserInfo).mockResolvedValue({ principal: 'alice@example.test', displayName: '' });
    open();
    await screen.findByTestId('agent-identity');
    expect(screen.getByTestId('consent-account')).toHaveTextContent('Signed in as alice@example.test');
    expect(screen.getByTestId('consent-card')).not.toContainElement(screen.getByTestId('consent-account'));
    expect(screen.getAllByText('Signed in as alice@example.test')).toHaveLength(1);
  });
  it('restores the originating controls after opening About and technical details for the first time', async () => {
    const user = userEvent.setup();
    open();
    const about = await screen.findByRole('button', { name: 'About this agent' });
    await user.click(about);
    expect(await screen.findByRole('dialog', { name: 'About this agent' })).toBeVisible();
    await user.keyboard('{Escape}');
    await waitFor(() => expect(about).toHaveFocus());

    const technical = screen.getByRole('button', { name: 'Technical details' });
    await user.click(technical);
    expect(await screen.findByRole('dialog', { name: 'Technical details' })).toBeVisible();
    await user.keyboard('{Escape}');
    await waitFor(() => expect(technical).toHaveFocus());
  });
  it('escapes long agent metadata while disclosing description only in About', async () => {
    const name = '<script>alert(1)</script>' + 'x'.repeat(300);
    const description = '<img src=x onerror=alert(1)>' + 'd'.repeat(400);
    vi.mocked(consentApi.getAgentDetail).mockResolvedValue({ ...detail, agent: { ...detail.agent, displayName: name, description } });
    const { container } = open();
    expect(await screen.findByTestId('agent-name')).toHaveTextContent(name);
    expect(container.querySelector('script, img[src="x"]')).toBeNull();
    expect(screen.queryByText(description)).not.toBeInTheDocument();
    await userEvent.click(screen.getByRole('button', { name: 'About this agent' }));
    expect(await screen.findByText(description)).toBeVisible();
    expect(container.querySelector('script, img[src="x"]')).toBeNull();
  });
  it('locks prior access without widening its services and grants only explicit additions with exact prior validity', async () => {
    open();
    const read = await screen.findByRole('checkbox', { name: 'Read mail', exact: true });
    expect(read).toBeChecked(); expect(read).toBeDisabled();
    const prior = screen.getByRole('checkbox', { name: 'Existing access', exact: true });
    expect(prior).toBeChecked(); expect(prior).toBeDisabled();
    const groups = screen.getAllByTestId('permission-group');
    expect(within(groups[0]!).getByText('Read mail')).toBeVisible();
    expect(within(groups[2]!).queryByRole('button', { name: /Choose services for Existing access/ })).not.toBeInTheDocument();
    expect(within(groups[2]!).getByTestId('permission-group-granted')).toHaveTextContent('Granted');
    await userEvent.click(screen.getByRole('button', { name: 'Choose custom date' }));
    expect(await screen.findByLabelText('Custom date', { selector: 'input' })).toHaveValue('2099-06-10');
    await userEvent.click(screen.getByRole('checkbox', { name: 'Write documents', exact: true }));
    await userEvent.click(screen.getByRole('button', { name: 'Allow', exact: true }));
    await waitFor(() => expect(consentApi.createOrUpdateGrant).toHaveBeenCalledWith('agent', { granted_permission_sets: { read: ['mail'], prior: ['drive'], write: ['drive'] }, valid_until: grant.valid_until }, { sessionToken: 'authorization' }));
    expect(await screen.findByTestId('consent-outcome')).toHaveTextContent(/access allowed/i);
  });
  it('discloses service names without raw scopes in permission rows and shows only available technical details', async () => {
    open();
    const group = (await screen.findAllByTestId('permission-group'))[0]!;
    expect(within(group).getByRole('img', { name: 'Mail' })).toBeVisible();
    expect(within(group).queryByText('mail:read')).not.toBeInTheDocument();
    await userEvent.click(screen.getByRole('button', { name: 'Technical details' }));
    const dialog = await screen.findByRole('dialog');
    expect(within(dialog).getByText(detail.cimd_metadata!.client_id_url)).toBeVisible();
    expect(within(dialog).getByText(detail.cimd_metadata!.redirect_uri)).toBeVisible();
    expect(within(dialog).getByText('mail:read')).toBeVisible();
  });
  it('shows only a supplied opaque client ID and never invents redirect metadata', async () => {
    vi.mocked(consentApi.getAgentDetail).mockResolvedValue({ ...detail, agent: { ...detail.agent, clientId: 'opaque-client-id' }, cimd_metadata: undefined });
    open();
    expect(await screen.findByTestId('agent-origin-label')).toHaveTextContent(accessCopy.registered);
    expect(screen.queryByText(/administrator/i)).not.toBeInTheDocument();
    expect(screen.queryByTestId('localhost-warning')).not.toBeInTheDocument();
    expect(screen.queryByText(/Returns to/)).not.toBeInTheDocument();
    await userEvent.click(screen.getByRole('button', { name: 'Technical details' }));
    const dialog = await screen.findByRole('dialog');
    expect(within(dialog).getByText('opaque-client-id')).toBeVisible();
    expect(within(dialog).queryByText('Redirect URI')).not.toBeInTheDocument();
    expect(within(dialog).queryByText('Requested scopes')).not.toBeInTheDocument();
  });

  it('flags an unverified origin without claiming the callback is local', async () => {
    vi.mocked(consentApi.getAgentDetail).mockResolvedValue({ ...detail, cimd_metadata: { ...detail.cimd_metadata!, verified_domain: '', redirect_uri: 'https://different.example/callback' } });
    open();
    expect(await screen.findByText('Unverified')).toBeVisible();
    expect(screen.getByTestId('consent-risk')).toHaveTextContent('origin is unverified');
    expect(screen.queryByTestId('localhost-warning')).not.toBeInTheDocument();
    expect(screen.getByText('Returns to different.example')).toBeVisible();
  });

  it('marks an empty restored service selection in its row and footer without submitting', async () => {
    const permissionSets = detail.agent.permission_sets.filter((entry) => entry.permission_set.id === 'prior');
    vi.mocked(consentApi.getAgentDetail).mockResolvedValue({ ...detail, agent: { ...detail.agent, permission_sets: permissionSets, service_requirements: [] } });
    vi.mocked(consentApi.getAgentGrants).mockResolvedValue([]);
    renderAgentPage(<AgentDecisionPage restoredDraft={{ selections: { prior: [] }, duration: 'until-revoked', customDate: '' }} />, '?session_token=authorization');
    await screen.findByRole('checkbox', { name: 'Existing access' });
    await userEvent.click(screen.getByRole('button', { name: 'Allow', exact: true }));
    expect(within(screen.getByTestId('permission-group')).getByText('Select at least one service in each permission group.')).toBeVisible();
    expect(screen.getByTestId('consent-validation-summary')).toBeVisible();
    expect(consentApi.createOrUpdateGrant).not.toHaveBeenCalled();
  });
  it('denies locally without saving, revoking, or navigating', async () => {
    open();
    await userEvent.click(await screen.findByRole('button', { name: 'Deny', exact: true }));
    expect(screen.getByTestId('consent-outcome')).toHaveTextContent(/denied/i);
    expect(screen.getByTestId('consent-outcome')).toHaveTextContent('Nothing was changed.');
    expect(screen.getByTestId('consent-outcome')).toHaveTextContent('You can close this tab.');
    expect(screen.getByTestId('consent-account')).toHaveTextContent('Signed in as Alice');
    expect(screen.getByTestId('consent-card')).not.toContainElement(screen.getByTestId('consent-account'));
    expect(screen.getAllByText('Signed in as Alice')).toHaveLength(1);
    expect(screen.getByLabelText('Current route')).toHaveTextContent('/agents/agent');
    expect(screen.queryByRole('button', { name: 'Allow', exact: true })).not.toBeInTheDocument();
    expect(consentApi.createOrUpdateGrant).not.toHaveBeenCalled();
    expect(consentApi.deleteGrant).not.toHaveBeenCalled();
  });
  it.each(['', 'expired'])('keeps invalid session %s in a noneditable decision error', async (token) => {
    vi.mocked(consentApi.getAgentDetail).mockRejectedValue({ status: 400, code: 'session_expired', message: 'authorization session has expired, please restart the authorization flow' });
    open(`?session_token=${token}`);
    expect(await screen.findByTestId('consent-error')).toHaveTextContent(/authorization session/i);
    expect(screen.queryByRole('tab')).not.toBeInTheDocument();
    expect(screen.queryByRole('button', { name: 'Allow', exact: true })).not.toBeInTheDocument();
    expect(screen.getByTestId('consent-card')).toContainElement(screen.getByTestId('consent-error'));
    expect(screen.queryByRole('button', { name: 'Try again' })).not.toBeInTheDocument();
    expect(consentApi.createOrUpdateGrant).not.toHaveBeenCalled();
    if (!token) expect(consentApi.getAgentDetail).not.toHaveBeenCalled();
  });
  it('rejects a past custom date before sending a grant and summarizes the row error', async () => {
    open();
    await userEvent.click(await screen.findByRole('button', { name: 'Choose custom date' }));
    const input = await screen.findByLabelText('Custom date', { selector: 'input' });
    fireEvent.change(input, { target: { value: '2020-01-01' } });
    await userEvent.click(screen.getByRole('button', { name: 'Allow', exact: true }));
    expect(screen.getAllByText('Choose a date after today.')[0]).toBeVisible();
    expect(screen.getByTestId('consent-validation-summary')).toBeVisible();
    expect(consentApi.createOrUpdateGrant).not.toHaveBeenCalled();
  });
  it('rejects an unsafe server continuation without navigating or claiming success', async () => {
    vi.mocked(consentApi.createOrUpdateGrant).mockResolvedValue({ kind: 'redirect', redirectUrl: 'https://attacker.example/callback' });
    open();
    await userEvent.click(await screen.findByRole('button', { name: 'Allow', exact: true }));
    expect(await screen.findByTestId('consent-error')).toHaveTextContent('Invalid redirect URL');
    expect(screen.queryByTestId('consent-outcome')).not.toBeInTheDocument();
  });
  it('rejects a backslash continuation that the browser would normalize to another origin', async () => {
    vi.mocked(consentApi.createOrUpdateGrant).mockResolvedValue({ kind: 'redirect', redirectUrl: String.raw`\\attacker.example/callback` });
    open();
    await userEvent.click(await screen.findByRole('button', { name: 'Allow', exact: true }));
    expect(await screen.findByTestId('consent-error')).toHaveTextContent('Invalid redirect URL');
    expect(screen.queryByTestId('consent-outcome')).not.toBeInTheDocument();
  });
  it.each(['details', 'grants'] as const)('keeps the accepted outcome when the post-save %s read fails', async read => {
    if (read === 'details') vi.mocked(consentApi.getAgentDetail).mockResolvedValueOnce(detail).mockRejectedValue({ status: 410 });
    else vi.mocked(consentApi.getAgentGrants).mockResolvedValueOnce([grant]).mockRejectedValue(new Error('Offline'));
    open();
    await userEvent.click(await screen.findByRole('button', { name: 'Allow', exact: true }));
    expect(await screen.findByTestId('consent-outcome')).toHaveTextContent(/Access allowed/);
    expect(screen.queryByTestId('consent-error')).not.toBeInTheDocument();
    expect(screen.queryByRole('button', { name: 'Try again' })).not.toBeInTheDocument();
  });
  it('posts only a UUID and clean return path while preserving the full draft in this tab', () => {
    const draft = createConsentDraft({ permissionSets: detail.agent.permission_sets, serviceRequirements: detail.agent.service_requirements, existingGrant: grant, context: 'decision', sessionToken: 'auth' }).setPermissionSet('write', true).setDuration('30-days');
    const submit = vi.spyOn(HTMLFormElement.prototype, 'submit').mockImplementation(function (this: HTMLFormElement) {
      expect(this.method).toBe('post');
      expect(new URL(this.action).pathname).toBe('/api/third-party/drive/oauth2/authorize');
      expect(new URL(this.action).search).toBe('');
      const form = new FormData(this);
      expect(Array.from(form.keys())).toEqual(['redirect_uri', 'consent_state_id']);
      expect(form.get('redirect_uri')).toBe(`${window.location.origin}/agents/agent`);
      const stateID = String(form.get('consent_state_id'));
      expect(stateID).toMatch(/^[0-9a-f-]{36}$/i);
      expect(loadConsentDraft(stateID, 'drive', '/agents/agent')).toMatchObject({
        ...draft.snapshot(), returnURL: `${window.location.origin}/agents/agent?session_token=auth&other=value#section`,
      });
      this.remove();
    });
    expect(startServiceLogin('drive', draft, `${window.location.origin}/agents/agent?session_token=auth&consent_state_id=old&success=true&service_id=other&other=value#section`)).toBe(true);
    expect(submit).toHaveBeenCalledOnce();
  });
  it('posts even an empty selection map when only return URL data must survive', () => {
    const empty = createConsentDraft({ permissionSets: [], context: 'decision' });
    const submit = vi.spyOn(HTMLFormElement.prototype, 'submit').mockImplementation(function (this: HTMLFormElement) {
      const form = new FormData(this);
      expect(form.get('redirect_uri')).toBe(`${window.location.origin}/agents/agent`);
      expect(loadConsentDraft(String(form.get('consent_state_id')), 'drive', '/agents/agent')).toMatchObject({
        selections: {}, returnURL: `${window.location.origin}/agents/agent?session_token=opaque#hash`,
      });
      this.remove();
    });
    expect(startServiceLogin('drive', empty, `${window.location.origin}/agents/agent?session_token=opaque#hash`)).toBe(true);
    expect(submit).toHaveBeenCalledOnce();
  });
  it('blocks a return-data-only login when current-tab storage refuses the record', () => {
    const empty = createConsentDraft({ permissionSets: [], context: 'decision' });
    vi.spyOn(Storage.prototype, 'setItem').mockImplementation(() => { throw new Error('storage unavailable'); });
    const submit = vi.spyOn(HTMLFormElement.prototype, 'submit').mockImplementation(() => {});
    expect(startServiceLogin('drive', empty, `${window.location.origin}/agents/agent#hash`)).toBe(false);
    expect(submit).not.toHaveBeenCalled();
  });
  it('posts a changed duration even without selected groups or return parameters', () => {
    const draft = createConsentDraft({ permissionSets: [], context: 'console' }).setDuration('30-days');
    const submit = vi.spyOn(HTMLFormElement.prototype, 'submit').mockImplementation(function (this: HTMLFormElement) {
      const form = new FormData(this);
      expect(loadConsentDraft(String(form.get('consent_state_id')), 'drive', '/agents/agent')).toMatchObject(draft.snapshot());
      this.remove();
    });
    expect(startServiceLogin('drive', draft, `${window.location.origin}/agents/agent`)).toBe(true);
    expect(submit).toHaveBeenCalledOnce();
  });
  it('restores the full saved draft without automatically submitting', async () => {
    const restoredDraft = createConsentDraft({ permissionSets: detail.agent.permission_sets, existingGrant: grant, context: 'decision' }).setPermissionSet('write', true).setDuration('30-days').snapshot();
    renderAgentPage(<AgentDecisionPage restoredDraft={restoredDraft} sessionToken="authorization" />, '?session_token=authorization');
    expect(await screen.findByRole('checkbox', { name: 'Write documents', exact: true })).toBeChecked();
    expect(screen.getByRole('combobox', { name: 'Access lasts' })).toHaveTextContent('30 days');
    expect(consentApi.createOrUpdateGrant).not.toHaveBeenCalled();
  });
  it('does not leave consent when selected-service storage is unavailable', async () => {
    vi.mocked(consentApi.getAgentDetail).mockResolvedValue({ ...detail, agent: { ...detail.agent, active_session_service_ids: ['mail'] } });
    vi.spyOn(Storage.prototype, 'setItem').mockImplementation(() => { throw new Error('storage unavailable'); });
    const submit = vi.spyOn(HTMLFormElement.prototype, 'submit').mockImplementation(() => {});
    open();
    await userEvent.click(await screen.findByRole('checkbox', { name: 'Write documents', exact: true }));
    await userEvent.click(screen.getByRole('button', { name: 'Connect Drive to continue' }));
    expect(screen.getByText('Cannot save your choices in this tab. Enable browser storage and try connecting again.')).toBeVisible();
    expect(submit).not.toHaveBeenCalled();
    expect(consentApi.createOrUpdateGrant).not.toHaveBeenCalled();
  });
  it('does not round or reject an unchanged grant that expires later today', async () => {
    vi.useFakeTimers({ toFake: ['Date'] });
    vi.setSystemTime(new Date(2026, 9, 1, 12));
    try {
      const validUntil = new Date(2026, 9, 1, 18, 12, 34, 567).toISOString();
      vi.mocked(consentApi.getAgentGrants).mockResolvedValue([{ ...grant, valid_until: validUntil }]);
      open();
      await userEvent.click(await screen.findByRole('button', { name: 'Allow', exact: true }));
      await waitFor(() => expect(consentApi.createOrUpdateGrant).toHaveBeenCalledWith('agent', {
        granted_permission_sets: grant.granted_permission_sets, valid_until: validUntil,
      }, { sessionToken: 'authorization' }));
    } finally {
      vi.useRealTimers();
    }
  });
});
