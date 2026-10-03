import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { fireEvent, screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { consentApi } from '@services/api/consent';
import { AgentDecisionPage } from './AgentDecisionPage';
import { createConsentDraft } from '@components/consent/consentDraft';
import { startServiceLogin } from '@components/consent/ServiceConnectPrompt';
import { loadConsentDraft } from '@services/storage/session';
import { detail, grant, renderAgentPage } from './agentConsentTestSupport';

vi.mock('@services/api/consent', () => ({ consentApi: { getUserInfo: vi.fn(), getAgentDetail: vi.fn(), getAgentGrants: vi.fn(), createOrUpdateGrant: vi.fn(), deleteGrant: vi.fn() } }));
beforeEach(() => {
  vi.resetAllMocks();
  sessionStorage.clear();
  vi.mocked(consentApi.getUserInfo).mockResolvedValue({ principal: 'alice', displayName: 'Alice' });
  vi.mocked(consentApi.getAgentDetail).mockResolvedValue(detail);
  vi.mocked(consentApi.getAgentGrants).mockResolvedValue(grant);
  vi.mocked(consentApi.createOrUpdateGrant).mockResolvedValue({ kind: 'created', grant });
});
afterEach(() => { vi.restoreAllMocks(); sessionStorage.clear(); });
const open = (query = '?session_token=authorization') => renderAgentPage(<AgentDecisionPage />, query);

describe('focused consent decisions', () => {
  it('shows trusted identity and independent callback warning without remote images or console navigation', async () => {
    const { container } = open();
    await screen.findByRole('heading', { name: detail.agent.displayName });
    expect(screen.queryByRole('navigation')).not.toBeInTheDocument();
    expect(container.querySelector('img[src^="https://external.example"]')).toBeNull();
    expect(screen.getByText('Verified domain: trusted.example')).toBeVisible();
    expect(screen.getByTestId('localhost-warning')).toHaveTextContent('local machine');
    expect(screen.getByRole('link', { name: 'Governance' })).toHaveAttribute('rel', 'noopener noreferrer');
    expect(screen.queryByText(/publisher/i)).not.toBeInTheDocument();
    expect(screen.getByTestId('consent-next-steps')).toHaveTextContent('/delegations');
    expect(screen.getByRole('button', { name: 'Deny' })).toHaveAttribute('data-variant', 'secondary');
    expect(container.querySelectorAll('[data-variant="primary"]')).toHaveLength(1);
    expect(consentApi.createOrUpdateGrant).not.toHaveBeenCalled();
  });
  it('escapes long metadata and offers expansion without interpreting markup', async () => {
    const name = '<script>alert(1)</script>' + 'x'.repeat(300);
    vi.mocked(consentApi.getAgentDetail).mockResolvedValue({ ...detail, agent: { ...detail.agent, displayName: name } });
    const { container } = open();
    expect(await screen.findByRole('heading', { name })).toHaveTextContent(name);
    expect(container.querySelector('script')).toBeNull();
    expect(screen.getByRole('button', { name: 'Show full agent name' })).toHaveAttribute('aria-expanded', 'false');
    await userEvent.click(screen.getByRole('button', { name: 'Show full agent name' }));
    expect(screen.getByRole('button', { name: 'Collapse agent name' })).toHaveAttribute('aria-expanded', 'true');
  });
  it('locks prior access without widening its services and grants only explicit additions with exact prior validity', async () => {
    open();
    const read = await screen.findByRole('checkbox', { name: 'Read mail', exact: true });
    expect(read).toBeChecked(); expect(read).toBeDisabled();
    const prior = screen.getByRole('checkbox', { name: 'Existing access', exact: true });
    expect(prior).toBeChecked(); expect(prior).toBeDisabled();
    const groups = screen.getAllByTestId('permission-group');
    expect(within(groups[0]!).getByText('Read mail')).toBeVisible();
    expect(within(groups[2]!).getByRole('button', { name: /Existing access/ })).toHaveAttribute('aria-expanded', 'false');
    expect(screen.getByLabelText('Custom date', { selector: 'input' })).toHaveValue('2099-06-10');
    await userEvent.click(screen.getByRole('checkbox', { name: 'Write documents', exact: true }));
    await userEvent.click(screen.getByRole('button', { name: 'Allow', exact: true }));
    await waitFor(() => expect(consentApi.createOrUpdateGrant).toHaveBeenCalledWith('agent', { granted_permission_sets: { read: ['mail'], prior: ['drive'], write: ['drive'] }, valid_until: grant.valid_until }, { sessionToken: 'authorization' }));
    expect(await screen.findByTestId('consent-outcome')).toHaveTextContent(/access allowed/i);
  });
  it('discloses service names without raw scopes in permission groups and retains advanced CIMD details', async () => {
    open();
    await userEvent.click(await screen.findByRole('button', { name: /Read mail/ }));
    const group = screen.getAllByTestId('permission-group')[0]!;
    expect(within(group).getByText('Mail')).toBeVisible();
    expect(within(group).queryByText('mail:read')).not.toBeInTheDocument();
    expect(within(group).queryByText(/risk/i)).not.toBeInTheDocument();
    await userEvent.click(screen.getByRole('button', { name: 'Advanced details' }));
    expect(screen.getByText(detail.cimd_metadata!.client_id_url)).toBeVisible();
    expect(screen.getByText(detail.cimd_metadata!.redirect_uri)).toBeVisible();
    expect(screen.getByText('mail:read')).toBeVisible();
  });
  it('denies locally without saving, revoking, or navigating', async () => {
    open();
    await userEvent.click(await screen.findByRole('button', { name: 'Deny', exact: true }));
    expect(screen.getByTestId('consent-outcome')).toHaveTextContent(/denied/i);
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
    expect(consentApi.createOrUpdateGrant).not.toHaveBeenCalled();
    if (!token) expect(consentApi.getAgentDetail).not.toHaveBeenCalled();
  });
  it('rejects a past custom date before sending a grant', async () => {
    open();
    const input = await screen.findByLabelText('Custom date', { selector: 'input' });
    fireEvent.change(input, { target: { value: '2020-01-01' } });
    await userEvent.click(screen.getByRole('button', { name: 'Allow', exact: true }));
    expect(screen.getByText('Choose a date after today.')).toBeVisible();
    expect(consentApi.createOrUpdateGrant).not.toHaveBeenCalled();
  });
  it('rejects an unsafe server continuation without navigating or claiming success', async () => {
    vi.mocked(consentApi.createOrUpdateGrant).mockResolvedValue({ kind: 'redirect', redirectUrl: 'https://attacker.example/callback' });
    open();
    await userEvent.click(await screen.findByRole('button', { name: 'Allow', exact: true }));
    expect(await screen.findByTestId('consent-error')).toHaveTextContent('Invalid redirect URL');
    expect(screen.queryByTestId('consent-outcome')).not.toBeInTheDocument();
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
    expect(screen.getByRole('radio', { name: '30 days', exact: true })).toBeChecked();
    expect(consentApi.createOrUpdateGrant).not.toHaveBeenCalled();
  });
  it('does not leave consent when selected-service storage is unavailable', async () => {
    vi.mocked(consentApi.getAgentDetail).mockResolvedValue({ ...detail, agent: { ...detail.agent, active_session_service_ids: ['mail'] } });
    vi.spyOn(Storage.prototype, 'setItem').mockImplementation(() => { throw new Error('storage unavailable'); });
    const submit = vi.spyOn(HTMLFormElement.prototype, 'submit').mockImplementation(() => {});
    open();
    await userEvent.click(await screen.findByRole('checkbox', { name: 'Write documents', exact: true }));
    await userEvent.click(screen.getByRole('button', { name: 'Connect' }));
    expect(screen.getByText('Cannot save your choices in this tab. Enable browser storage and try connecting again.')).toBeVisible();
    expect(submit).not.toHaveBeenCalled();
    expect(consentApi.createOrUpdateGrant).not.toHaveBeenCalled();
  });
  it('does not round or reject an unchanged grant that expires later today', async () => {
    vi.useFakeTimers({ toFake: ['Date'] });
    vi.setSystemTime(new Date(2026, 9, 1, 12));
    try {
      const validUntil = new Date(2026, 9, 1, 18, 12, 34, 567).toISOString();
      vi.mocked(consentApi.getAgentGrants).mockResolvedValue({ ...grant, valid_until: validUntil });
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
