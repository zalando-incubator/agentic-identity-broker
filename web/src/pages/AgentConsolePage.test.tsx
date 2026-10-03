import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { act, fireEvent, screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { consentApi } from '@services/api/consent';
import { sessionsApi } from '@services/api/sessions';
import { queryKeys } from '@services/query/queryKeys';
import { AgentConsolePage } from './AgentConsolePage';
import { detail, grant as decisionGrant, renderAgentPage } from './agentConsentTestSupport';
import { loadConsentDraft } from '@services/storage/session';
import type { GrantResult } from '../types/consent';
const grant = { ...decisionGrant, granted_permission_sets: { read: ['mail'], prior: ['drive', 'mail'] } };

vi.mock('@services/api/consent', () => ({ consentApi: { getUserInfo: vi.fn(), getAgentDetail: vi.fn(), getAgentGrants: vi.fn(), createOrUpdateGrant: vi.fn(), deleteGrant: vi.fn() } }));
vi.mock('@services/api/sessions', () => ({ sessionsApi: { listSessions: vi.fn(), refreshSession: vi.fn() } }));
beforeEach(() => {
  vi.resetAllMocks();
  sessionStorage.clear();
  vi.mocked(consentApi.getUserInfo).mockResolvedValue({ principal: 'alice', displayName: 'Alice' });
  vi.mocked(consentApi.getAgentDetail).mockResolvedValue(detail);
  vi.mocked(consentApi.getAgentGrants).mockResolvedValue(grant);
  vi.mocked(sessionsApi.listSessions).mockResolvedValue([]);
});
afterEach(() => { vi.restoreAllMocks(); sessionStorage.clear(); });

describe('agent grant management', () => {
  it('edits optional existing groups only in the console and discards changes on cancel', async () => {
    renderAgentPage(<AgentConsolePage />);
    expect(await screen.findByRole('heading', { name: detail.agent.displayName })).toBeVisible();
    expect(screen.queryByTestId('agent-origin-label')).not.toBeInTheDocument();
    expect(screen.queryByText(/publisher/i)).not.toBeInTheDocument();
    expect(screen.getByRole('link', { name: 'Governance' })).toHaveAttribute('href', detail.agent.governanceUrl);
    expect(screen.getByRole('tab', { name: 'Permissions' })).toBeVisible();
    expect(screen.getByRole('tab', { name: 'Connections' })).toBeVisible();
    expect(screen.getByRole('checkbox', { name: 'Read mail', exact: true })).toBeDisabled();
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
    renderAgentPage(<AgentConsolePage />);
    await userEvent.click(await screen.findByRole('checkbox', { name: 'Write documents', exact: true }));
    await userEvent.click(screen.getByRole('button', { name: 'Save changes' }));
    expect(screen.getByTestId('grant-save-bar')).toBeVisible();
    expect(screen.getByRole('button', { name: 'Save changes' })).toBeDisabled();
    const updated = { ...grant, granted_permission_sets: { read: ['mail'], prior: ['drive', 'mail'], write: ['drive'] } };
    vi.mocked(consentApi.getAgentGrants).mockResolvedValue(updated);
    resolve({ kind: 'created', grant: updated });
    await waitFor(() => expect(screen.queryByTestId('grant-save-bar')).not.toBeInTheDocument());
    expect(consentApi.createOrUpdateGrant).toHaveBeenCalledWith('agent', { granted_permission_sets: updated.granted_permission_sets, valid_until: grant.valid_until }, undefined);
    expect(screen.getByRole('status')).toHaveTextContent('Grant updated successfully');
  });
  it('keeps unsaved edits visible after a rejected save', async () => {
    vi.mocked(consentApi.createOrUpdateGrant).mockRejectedValue({ status: 403, code: 'FORBIDDEN', message: "You don't have permission to access this resource." });
    renderAgentPage(<AgentConsolePage />);
    await userEvent.click(await screen.findByRole('radio', { name: 'Until revoked' }));
    await userEvent.click(screen.getByRole('button', { name: 'Save changes' }));
    expect(await screen.findByRole('alert')).toHaveTextContent("You don't have permission to access this resource.");
    expect(screen.getByTestId('grant-save-bar')).toBeVisible();
    expect(screen.getByRole('radio', { name: 'Until revoked' })).toBeChecked();
  });
  it('requires named confirmation and navigates only after successful revoke', async () => {
    renderAgentPage(<AgentConsolePage />);
    await userEvent.click(await screen.findByRole('button', { name: 'Agent actions' }));
    await userEvent.click(screen.getByRole('menuitem', { name: 'Revoke all access' }));
    expect(screen.getByRole('dialog')).toHaveTextContent(detail.agent.displayName);
    await userEvent.click(within(screen.getByRole('dialog')).getByRole('button', { name: 'Cancel' }));
    expect(consentApi.deleteGrant).not.toHaveBeenCalled();
    await userEvent.click(screen.getByRole('button', { name: 'Agent actions' }));
    await userEvent.click(screen.getByRole('menuitem', { name: 'Revoke all access' }));
    vi.mocked(consentApi.deleteGrant).mockResolvedValue(undefined);
    await userEvent.click(within(screen.getByRole('dialog')).getByRole('button', { name: 'Revoke' }));
    await screen.findByText('Agent list');
    expect(consentApi.deleteGrant).toHaveBeenCalledWith('agent');
  });
  it('posts a saved draft from the Connections tab instead of leaking it in a URL', async () => {
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
    await userEvent.click(await screen.findByRole('tab', { name: 'Connections' }));
    const panel = screen.getByRole('tabpanel', { name: 'Connections' });
    expect(await within(panel).findByText('No connection')).toBeVisible();
    await userEvent.click(within(panel).getByRole('button', { name: 'Connect' }));
    expect(submit).toHaveBeenCalledOnce();
    expect(consentApi.createOrUpdateGrant).not.toHaveBeenCalled();
  });
  it('stays on the agent and explains how to retry when tab storage refuses a connection', async () => {
    vi.mocked(consentApi.getAgentDetail).mockResolvedValue({ ...detail, services: [{ ...detail.services[0]!, connectionStatus: 'not_connected' }] });
    vi.spyOn(Storage.prototype, 'setItem').mockImplementation(() => { throw new Error('storage unavailable'); });
    const submit = vi.spyOn(HTMLFormElement.prototype, 'submit').mockImplementation(() => {});
    renderAgentPage(<AgentConsolePage />);
    await userEvent.click(await screen.findByRole('tab', { name: 'Connections' }));
    await userEvent.click(within(screen.getByRole('tabpanel', { name: 'Connections' })).getByRole('button', { name: 'Connect' }));
    expect(screen.getByRole('alert')).toHaveTextContent('Enable browser storage and try connecting again');
    expect(submit).not.toHaveBeenCalled();
    expect(consentApi.createOrUpdateGrant).not.toHaveBeenCalled();
  });
  it('preserves an edited date across an equivalent grant refresh and cancels to the saved date', async () => {
    const { client } = renderAgentPage(<AgentConsolePage />);
    const date = await screen.findByLabelText('Custom date', { selector: 'input' });
    fireEvent.change(date, { target: { value: '2099-08-12' } });
    vi.mocked(consentApi.getAgentGrants).mockResolvedValue({ ...grant, updated_at: '2026-02-01T00:00:00Z' });
    await act(async () => { await client.invalidateQueries({ queryKey: queryKeys.grant('alice', 'agent') }); });
    expect(screen.getByLabelText('Custom date', { selector: 'input' })).toHaveValue('2099-08-12');
    expect(screen.getByTestId('grant-save-bar')).toBeVisible();
    await userEvent.click(screen.getByRole('button', { name: 'Cancel', exact: true }));
    expect(screen.getByLabelText('Custom date', { selector: 'input' })).toHaveValue('2099-06-10');
    expect(consentApi.createOrUpdateGrant).not.toHaveBeenCalled();
  });
  it.each(['2099-09-14T16:45:12.345Z', undefined])('rehydrates a clean editor when saved validity changes to %s', async (validUntil) => {
    const { client } = renderAgentPage(<AgentConsolePage />);
    expect(await screen.findByLabelText('Custom date', { selector: 'input' })).toHaveValue('2099-06-10');
    const refreshedGrant = { ...grant };
    if (validUntil) refreshedGrant.valid_until = validUntil;
    else delete refreshedGrant.valid_until;
    vi.mocked(consentApi.getAgentGrants).mockResolvedValue(refreshedGrant);
    await act(async () => { await client.invalidateQueries({ queryKey: queryKeys.grant('alice', 'agent') }); });
    if (validUntil) {
      await waitFor(() => expect(screen.getByLabelText('Custom date', { selector: 'input' })).toHaveValue('2099-09-14'));
    } else {
      await waitFor(() => expect(screen.getByRole('radio', { name: 'Until revoked' })).toBeChecked());
      expect(screen.queryByLabelText('Custom date', { selector: 'input' })).not.toBeInTheDocument();
    }
    expect(screen.queryByTestId('grant-save-bar')).not.toBeInTheDocument();
    expect(consentApi.createOrUpdateGrant).not.toHaveBeenCalled();
  });
  it('keeps an accepted save authoritative when the follow-up grant read fails', async () => {
    renderAgentPage(<AgentConsolePage />);
    const date = await screen.findByLabelText('Custom date', { selector: 'input' });
    fireEvent.change(date, { target: { value: '2099-08-12' } });
    vi.mocked(consentApi.createOrUpdateGrant).mockResolvedValue({
      kind: 'created', grant: { ...grant, valid_until: '2099-08-12T00:00:00.000Z' },
    });
    vi.mocked(consentApi.getAgentGrants).mockRejectedValue({ status: 503, code: 'SERVER_ERROR', message: 'Service temporarily unavailable. Please try again later.' });
    await userEvent.click(screen.getByRole('button', { name: 'Save changes' }));
    await screen.findByText('Grant updated successfully.');
    expect(screen.getByLabelText('Custom date', { selector: 'input' })).toHaveValue('2099-08-12');
    expect(screen.queryByTestId('grant-save-bar')).not.toBeInTheDocument();
    expect(consentApi.createOrUpdateGrant).toHaveBeenCalledTimes(1);
  });
  it('restores a clean grant and exact expiry after keyboard duration exploration', async () => {
    const user = userEvent.setup();
    vi.mocked(consentApi.createOrUpdateGrant).mockResolvedValue({ kind: 'created', grant });
    renderAgentPage(<AgentConsolePage />);
    const custom = await screen.findByRole('radio', { name: 'Custom date', exact: true });
    custom.focus();
    for (const [key, label] of [
      ['ArrowRight', 'Until revoked'],
      ['ArrowRight', '30 days'],
      ['ArrowLeft', 'Until revoked'],
      ['ArrowLeft', 'Custom date'],
    ]) {
      try {
        await user.keyboard(`{${key}>}`);
        await waitFor(() => {
          expect(screen.getByRole('radio', { name: label, exact: true })).toHaveFocus();
          expect(screen.getByRole('radio', { name: label, exact: true })).toBeChecked();
        });
      } finally {
        await user.keyboard(`{/${key}}`);
      }
      if (label !== 'Custom date') expect(screen.getByTestId('grant-save-bar')).toBeVisible();
    }
    expect(screen.getByRole('radio', { name: 'Custom date', exact: true })).toBeChecked();
    expect(screen.getByLabelText('Custom date', { selector: 'input' })).toHaveValue('2099-06-10');
    expect(screen.queryByTestId('grant-save-bar')).not.toBeInTheDocument();
    expect(consentApi.createOrUpdateGrant).not.toHaveBeenCalled();
    await user.click(screen.getByRole('checkbox', { name: 'Write documents', exact: true }));
    await user.click(screen.getByRole('button', { name: 'Save changes' }));
    await waitFor(() => expect(consentApi.createOrUpdateGrant).toHaveBeenCalledWith('agent', {
      granted_permission_sets: { read: ['mail'], prior: ['drive', 'mail'], write: ['drive'] },
      valid_until: grant.valid_until,
    }, undefined));
  });
});
