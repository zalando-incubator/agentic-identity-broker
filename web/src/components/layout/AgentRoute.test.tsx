import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { Suspense, type ReactNode } from 'react';
import { consentApi } from '@services/api/consent';
import { saveConsentDraft } from '@services/storage/session';
import { ThemeProvider } from '@design-system/theme/ThemeProvider';
import AgentRoute from './AgentRoute';
import { detail, grant, renderAgentPage } from '../../pages/agentConsentTestSupport';

vi.mock('@services/api/consent', () => ({ consentApi: { getUserInfo: vi.fn(), getAgentDetail: vi.fn(), getAgentGrants: vi.fn(), createOrUpdateGrant: vi.fn(), deleteGrant: vi.fn() } }));
vi.mock('./ConsoleLayout', () => ({ default: ({ children }: { children: ReactNode }) => <section data-testid="console-shell">{children}</section> }));

beforeEach(() => {
  vi.resetAllMocks();
  vi.stubGlobal('matchMedia', vi.fn(() => ({ matches: false, addEventListener: vi.fn(), removeEventListener: vi.fn() })));
  sessionStorage.clear();
  window.history.replaceState(null, '', '/agents/agent');
  vi.mocked(consentApi.getUserInfo).mockResolvedValue({ principal: 'alice', displayName: 'Alice' });
  vi.mocked(consentApi.getAgentDetail).mockResolvedValue(detail);
  vi.mocked(consentApi.getAgentGrants).mockResolvedValue([grant]);
});
afterEach(() => {
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
  sessionStorage.clear();
  window.history.replaceState(null, '', '/');
});

const returnPath = '/agents/agent?session_token=opaque-session&other=retained#original';
const openRoute = (query: string) => renderAgentPage(<ThemeProvider><Suspense fallback={<p role="status">Loading</p>}><AgentRoute /></Suspense></ThemeProvider>, query);

describe('agent callback route', () => {
  it('restores the decision shell, full URL, and draft before any authorization-sensitive read', async () => {
    const draft = { selections: { read: ['mail'], prior: ['drive'], write: ['drive'] }, duration: '30-days' as const, customDate: '' };
    const stateID = saveConsentDraft(draft, 'drive', `${window.location.origin}${returnPath}`)!;
    const { unmount } = openRoute(`?success=true&service_id=drive&consent_state_id=${stateID}`);

    expect(await screen.findByRole('checkbox', { name: 'Write documents', exact: true }, { timeout: 5_000 })).toBeChecked();
    const card = screen.getByTestId('consent-card');
    const account = screen.getByTestId('consent-account');
    expect(card).not.toContainElement(account);
    expect(screen.getByRole('main')).toContainElement(account);
    expect(account).toHaveTextContent('Signed in as Alice');
    expect(screen.getAllByText('Signed in as Alice')).toHaveLength(1);
    expect(screen.getByRole('combobox', { name: 'Access lasts' })).toHaveTextContent('30 days');
    expect(screen.queryByTestId('console-shell')).not.toBeInTheDocument();
    expect(window.location.search).toContain('session_token=opaque-session');
    expect(window.location.search).toContain('other=retained');
    expect(window.location.search).toContain('success=true');
    expect(window.location.search).toContain('service_id=drive');
    expect(window.location.search).not.toContain('consent_state_id');
    expect(window.location.hash).toBe('#original');
    await waitFor(() => expect(consentApi.getAgentDetail).toHaveBeenCalledWith('agent', expect.objectContaining({ sessionToken: 'opaque-session' })));
    expect(vi.mocked(consentApi.getAgentDetail).mock.calls.every(([, options]) => options?.sessionToken === 'opaque-session')).toBe(true);
    expect(consentApi.createOrUpdateGrant).not.toHaveBeenCalled();

    unmount();
    openRoute(window.location.search);
    expect(await screen.findByRole('checkbox', { name: 'Write documents', exact: true })).not.toBeChecked();
    expect(consentApi.createOrUpdateGrant).not.toHaveBeenCalled();
  });

  it('restores a callback without an authorization session into the console without saving', async () => {
    const draft = { selections: { read: ['mail'], prior: ['drive'], write: ['drive'] }, duration: 'custom' as const, customDate: '2099-09-04' };
    const stateID = saveConsentDraft(draft, 'drive', `${window.location.origin}/agents/agent?tab=connections#selection`)!;
    openRoute(`?success=true&service_id=drive&consent_state_id=${stateID}`);
    expect(await screen.findByTestId('console-shell')).toBeVisible();
    expect(await screen.findByRole('checkbox', { name: 'Write documents', exact: true })).toBeChecked();
    await userEvent.click(screen.getByRole('button', { name: 'Choose custom date' }));
    expect(await screen.findByLabelText('Custom date', { selector: 'input' })).toHaveValue('2099-09-04');
    expect(window.location.search).toContain('tab=connections');
    expect(window.location.search).not.toContain('consent_state_id');
    expect(window.location.hash).toBe('#selection');
    expect(consentApi.createOrUpdateGrant).not.toHaveBeenCalled();
  });

  it('ignores an unknown or mismatched callback ID instead of restoring another saved decision', async () => {
    const stateID = saveConsentDraft({ selections: { write: ['drive'] }, duration: '30-days', customDate: '' }, 'drive', `${window.location.origin}${returnPath}`)!;
    openRoute(`?success=true&service_id=mail&consent_state_id=${stateID}`);
    expect(await screen.findByTestId('console-shell')).toBeVisible();
    expect(window.location.search).not.toContain('session_token');
    expect(consentApi.createOrUpdateGrant).not.toHaveBeenCalled();
  });

  it('keeps an invalid explicit authorization token in the decision error shell', async () => {
    vi.mocked(consentApi.getAgentDetail).mockRejectedValue({ status: 400, code: 'session_expired', message: 'Authorization session expired' });
    openRoute('?session_token=expired&success=true&service_id=drive&consent_state_id=missing');
    expect(await screen.findByTestId('consent-error')).toHaveTextContent('invalid or expired');
    expect(screen.getByTestId('consent-account')).toHaveTextContent('Signed in as Alice');
    expect(screen.queryByTestId('console-shell')).not.toBeInTheDocument();
    expect(consentApi.createOrUpdateGrant).not.toHaveBeenCalled();
  });
});
