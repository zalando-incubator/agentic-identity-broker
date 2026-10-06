import { describe, expect, it } from 'vitest';
import { act, fireEvent, screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { saveConsentDraft } from '@services/storage/session';
import { agentDetail, grant, renderApplication, type HttpReply } from './applicationIntegrationTestSupport';

const grantPath = '/consent/agents/agent/grants';

describe('consent routes through HTTP and principal-scoped Query', () => {
  it('loads the grants list, cancels edits, then saves and refetches the exact permission and expiry changes', async () => {
    const user = userEvent.setup();
    const saved = Promise.withResolvers<HttpReply>();
    const existingGrant = { ...grant, granted_permission_sets: { read: ['mail'], prior: ['drive', 'mail'] } };
    let currentGrant: typeof grant = existingGrant;
    const { requests } = renderApplication('/agents/agent', config => {
      if (config.method === 'get' && config.url === '/consent/agents/agent') return { body: { data: agentDetail } };
      if (config.method === 'get' && config.url === grantPath) return { body: { data: [currentGrant] } };
      if (config.url === '/third-party/sessions') return { body: { data: { sessions: [] } } };
      if (config.method === 'post' && config.url === grantPath) return saved.promise;
    });

    expect(await screen.findByRole('heading', { name: 'Research Agent' })).toBeVisible();
    expect(screen.getByRole('checkbox', { name: 'Existing access' })).toBeChecked();
    expect(screen.getAllByTestId('permission-group')[0]).toHaveAttribute('data-read-only', 'true');
    await user.click(screen.getByRole('checkbox', { name: 'Write documents' }));
    await user.click(screen.getByRole('button', { name: 'Cancel' }));
    expect(screen.getByRole('checkbox', { name: 'Write documents' })).not.toBeChecked();
    expect(screen.queryByRole('button', { name: 'Save changes' })).not.toBeInTheDocument();
    expect(requests.mock.calls.filter(([config]) => config.method === 'post')).toHaveLength(0);

    await user.click(screen.getByRole('checkbox', { name: 'Write documents' }));
    const readsBeforeSave = requests.mock.calls.filter(([config]) => config.method === 'get' && config.url === grantPath).length;
    await user.click(screen.getByRole('button', { name: 'Save changes' }));
    await waitFor(() => expect(requests.mock.calls.filter(([config]) => config.method === 'post')).toHaveLength(1));
    const posted = requests.mock.calls.find(([config]) => config.method === 'post')![0];
    expect(posted.url).toBe(grantPath);
    expect(JSON.parse(posted.data)).toEqual({
      granted_permission_sets: { ...existingGrant.granted_permission_sets, write: ['drive'] },
      valid_until: grant.valid_until,
    });
    expect(screen.getByRole('button', { name: 'Save changes' })).toBeDisabled();
    expect(screen.queryByText('Grant updated successfully.')).not.toBeInTheDocument();

    currentGrant = { ...existingGrant, granted_permission_sets: { ...existingGrant.granted_permission_sets, write: ['drive'] } };
    await act(async () => saved.resolve({ status: 201, body: { data: currentGrant } }));
    expect(await screen.findByText('Grant updated successfully.')).toBeVisible();
    await waitFor(() => expect(screen.queryByRole('button', { name: 'Save changes' })).not.toBeInTheDocument());
    expect(screen.getByRole('checkbox', { name: 'Write documents' })).toBeChecked();
    expect(requests.mock.calls.filter(([config]) => config.method === 'get' && config.url === grantPath).length).toBeGreaterThan(readsBeforeSave);
    expect(requests.mock.calls.filter(([config]) => config.method === 'get' && config.url === '/consent/agents/agent')).toHaveLength(2);
  });

  it('retains a rejected permission draft and saves it only after an explicit retry succeeds', async () => {
    const user = userEvent.setup();
    let currentGrant = grant;
    let attempts = 0;
    const { requests } = renderApplication('/agents/agent', config => {
      if (config.method === 'get' && config.url === '/consent/agents/agent') return { body: { data: agentDetail } };
      if (config.method === 'get' && config.url === grantPath) return { body: { data: [currentGrant] } };
      if (config.url === '/third-party/sessions') return { body: { data: { sessions: [] } } };
      if (config.method === 'post' && config.url === grantPath) {
        if (++attempts === 1) return { status: 422, body: { error: 'grant_rejected', message: 'Policy rejected this change.' } };
        currentGrant = { ...grant, granted_permission_sets: JSON.parse(config.data).granted_permission_sets };
        return { status: 201, body: { data: currentGrant } };
      }
    });

    await user.click(await screen.findByRole('checkbox', { name: 'Write documents' }));
    await user.click(screen.getByRole('button', { name: 'Save changes' }));
    expect(await screen.findByRole('alert')).toHaveTextContent('Policy rejected this change.');
    expect(screen.getByRole('checkbox', { name: 'Write documents' })).toBeChecked();
    expect(screen.getByRole('button', { name: 'Save changes' })).toBeEnabled();
    expect(screen.queryByText('Grant updated successfully.')).not.toBeInTheDocument();
    expect(attempts).toBe(1);
    await user.click(screen.getByRole('button', { name: 'Save changes' }));
    expect(await screen.findByText('Grant updated successfully.')).toBeVisible();
    expect(attempts).toBe(2);
    const bodies = requests.mock.calls.filter(([config]) => config.method === 'post').map(([config]) => JSON.parse(config.data));
    expect(bodies[1]).toEqual(bodies[0]);
  });

  it('locks previously granted access and submits only an explicit addition in the authorization session', async () => {
    const user = userEvent.setup();
    const token = 'authorization/+&';
    let currentGrant = grant;
    const { requests } = renderApplication(`/agents/agent?session_token=${encodeURIComponent(token)}`, config => {
      if (config.method === 'get' && config.url === `/consent/agents/agent?session_token=${encodeURIComponent(token)}`) return { body: { data: agentDetail } };
      if (config.method === 'get' && config.url === grantPath) return { body: { data: [currentGrant] } };
      if (config.method === 'post' && config.url === `${grantPath}?session_token=${encodeURIComponent(token)}`) {
        currentGrant = { ...grant, granted_permission_sets: JSON.parse(config.data).granted_permission_sets };
        return { status: 201, body: { data: currentGrant } };
      }
    });

    const read = await screen.findByRole('checkbox', { name: 'Read mail' });
    expect(read).toBeChecked();
    expect(read).toBeDisabled();
    const existing = screen.getByRole('checkbox', { name: 'Existing access' });
    expect(existing).toBeChecked();
    expect(existing).toBeDisabled();
    expect(screen.queryByRole('navigation')).not.toBeInTheDocument();
    expect(screen.getByTestId('consent-account')).toHaveTextContent('Signed in as Alice');
    expect(requests.mock.calls.filter(([config]) => config.method === 'post')).toHaveLength(0);
    await user.click(screen.getByRole('checkbox', { name: 'Write documents' }));
    await user.click(screen.getByRole('button', { name: 'Allow' }));
    expect(await screen.findByTestId('consent-outcome')).toHaveTextContent(/access allowed/i);
    const posted = requests.mock.calls.find(([config]) => config.method === 'post')![0];
    expect(posted.url).toBe(`${grantPath}?session_token=${encodeURIComponent(token)}`);
    expect(JSON.parse(posted.data)).toEqual({ granted_permission_sets: { read: ['mail'], prior: ['drive'], write: ['drive'] }, valid_until: grant.valid_until });
    expect(screen.queryByRole('button', { name: 'Allow' })).not.toBeInTheDocument();
  });

  it('validates an empty restored service selection before issuing a grant request', async () => {
    const user = userEvent.setup();
    const permissionSets = agentDetail.permission_sets.filter(entry => entry.permission_set.id === 'prior');
    const stateId = saveConsentDraft({ selections: { prior: [] }, duration: 'until-revoked', customDate: '' }, 'drive', `${window.location.origin}/agents/agent?session_token=authorization`);
    expect(stateId).toBeDefined();
    const { requests } = renderApplication(`/agents/agent?success=true&service_id=drive&consent_state_id=${stateId}`, config => {
      if (config.url === '/consent/agents/agent?session_token=authorization') return { body: { data: { ...agentDetail, permission_sets: permissionSets, service_requirements: [] } } };
      if (config.url === grantPath) return { body: { data: [] } };
    });

    expect(await screen.findByRole('checkbox', { name: 'Existing access' })).toBeChecked();
    await user.click(screen.getByRole('button', { name: 'Allow' }));
    expect(screen.getByTestId('consent-validation-summary')).toBeVisible();
    expect(within(screen.getByTestId('permission-group')).getByRole('alert')).toHaveTextContent('Select at least one service in each permission group.');
    expect(requests.mock.calls.filter(([config]) => config.method !== 'get')).toHaveLength(0);
  });

  it('rejects a past expiry before transport and leaves the decision editable', async () => {
    const user = userEvent.setup();
    const { requests } = renderApplication('/agents/agent?session_token=authorization', config => {
      if (config.url === '/consent/agents/agent?session_token=authorization') return { body: { data: agentDetail } };
      if (config.url === grantPath) return { body: { data: [grant] } };
    });

    await user.click(await screen.findByRole('button', { name: 'Choose custom date' }));
    const date = await screen.findByLabelText('Custom date', { selector: 'input' });
    fireEvent.change(date, { target: { value: '2020-01-01' } });
    await user.click(screen.getByRole('button', { name: 'Allow' }));
    expect(screen.getByTestId('consent-validation-summary')).toBeVisible();
    expect(screen.getAllByText('Choose a date after today.')[0]).toBeVisible();
    expect(screen.getByRole('button', { name: 'Allow' })).toBeEnabled();
    expect(requests.mock.calls.filter(([config]) => config.method !== 'get')).toHaveLength(0);
  });

  it('denies a decision without saving or revoking existing access', async () => {
    const user = userEvent.setup();
    const { requests } = renderApplication('/agents/agent?session_token=authorization', config => {
      if (config.url === '/consent/agents/agent?session_token=authorization') return { body: { data: agentDetail } };
      if (config.url === grantPath) return { body: { data: [grant] } };
    });
    await user.click(await screen.findByRole('button', { name: 'Deny' }));
    expect(screen.getByTestId('consent-outcome')).toHaveTextContent('Nothing was changed.');
    expect(window.location.pathname).toBe('/agents/agent');
    expect(requests.mock.calls.filter(([config]) => config.method !== 'get')).toHaveLength(0);
  });

  it.each(['', 'expired'])('keeps an invalid authorization session "%s" out of the editable console', async token => {
    const { requests } = renderApplication(`/agents/agent?session_token=${token}`, config => {
      if (config.url?.startsWith('/consent/agents/agent?session_token=')) return { status: 400, body: { error: 'session_expired', message: 'Authorization session expired.' } };
      if (config.url === grantPath) return { body: { data: [grant] } };
    });
    expect(await screen.findByTestId('consent-error')).toHaveTextContent(/authorization session/i);
    expect(screen.queryByRole('navigation')).not.toBeInTheDocument();
    expect(screen.queryByRole('button', { name: 'Allow' })).not.toBeInTheDocument();
    expect(screen.queryByRole('button', { name: 'Save changes' })).not.toBeInTheDocument();
    expect(requests.mock.calls.filter(([config]) => config.method !== 'get')).toHaveLength(0);
    if (!token) expect(requests.mock.calls.filter(([config]) => config.url !== '/me')).toHaveLength(0);
  });
});
