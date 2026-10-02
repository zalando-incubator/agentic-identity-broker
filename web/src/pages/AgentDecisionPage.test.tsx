import { beforeEach, describe, expect, it, vi } from 'vitest';
import { fireEvent, screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { consentApi } from '@services/api/consent';
import { AgentDecisionPage } from './AgentDecisionPage';
import { createConsentDraft } from '@components/consent/consentDraft';
import { buildServiceLoginUrl } from '@components/consent/ServiceConnectPrompt';
import { detail, grant, renderAgentPage } from './agentConsentTestSupport';

vi.mock('@services/api/consent', () => ({ consentApi: { getUserInfo: vi.fn(), getAgentDetail: vi.fn(), getAgentGrants: vi.fn(), createOrUpdateGrant: vi.fn(), deleteGrant: vi.fn() } }));
beforeEach(() => {
  vi.resetAllMocks();
  vi.mocked(consentApi.getUserInfo).mockResolvedValue({ principal: 'alice', displayName: 'Alice' });
  vi.mocked(consentApi.getAgentDetail).mockResolvedValue(detail);
  vi.mocked(consentApi.getAgentGrants).mockResolvedValue(grant);
  vi.mocked(consentApi.createOrUpdateGrant).mockResolvedValue({ kind: 'created', grant });
});
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
    vi.mocked(consentApi.getAgentDetail).mockRejectedValue({ status: 410, code: 'SESSION_EXPIRED', message: 'Expired session' });
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
  it('preserves the canonical draft and session in the existing provider callback URL', () => {
    const draft = createConsentDraft({ permissionSets: detail.agent.permission_sets!, serviceRequirements: detail.agent.service_requirements, existingGrant: grant, context: 'decision', sessionToken: 'auth' }).setPermissionSet('write', true).setDuration('30-days');
    const url = new URL(buildServiceLoginUrl('drive', draft, 'http://localhost/agents/agent?session_token=auth&other=value'), 'http://localhost');
    expect(url.pathname).toBe('/api/third-party/drive/oauth2/authorize');
    const callback = new URL(url.searchParams.get('redirect_uri')!);
    expect(callback.searchParams.get('session_token')).toBe('auth');
    expect(callback.searchParams.get('other')).toBe('value');
    const restored = createConsentDraft({ permissionSets: detail.agent.permission_sets!, serviceRequirements: detail.agent.service_requirements, existingGrant: grant, context: 'decision', consentState: callback.searchParams.get('consent_state') });
    expect(restored.selections).toEqual(draft.selections);
    expect(restored.duration).toBe('30-days');
  });
  it('restores selection and duration after provider login without automatically submitting', async () => {
    const state = createConsentDraft({ permissionSets: detail.agent.permission_sets!, existingGrant: grant, context: 'decision' }).setPermissionSet('write', true).setDuration('30-days').toConsentState();
    open(`?session_token=authorization&consent_state=${state}`);
    expect(await screen.findByRole('checkbox', { name: 'Write documents', exact: true })).toBeChecked();
    expect(screen.getByRole('radio', { name: '30 days', exact: true })).toBeChecked();
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
