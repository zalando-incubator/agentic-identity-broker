import { act, screen } from '@testing-library/react';
import { expect, it, vi } from 'vitest';
import ReactDOM from 'react-dom/client';
import type { Root } from 'react-dom/client';
import { consentApi } from '@services/api/consent';
import { approvalApi } from '@services/api/approvals';
import type { UserInfo } from '@app-types/consent';

it('loads a console without briefly showing decision-page branding before identity resolves', async () => {
  vi.stubGlobal('matchMedia', (media: string) => Object.assign(new EventTarget(), { matches: false, media }));
  const identity = Promise.withResolvers<UserInfo>();
  vi.spyOn(consentApi, 'getUserInfo').mockReturnValue(identity.promise);
  vi.spyOn(consentApi, 'getAgentDelegations').mockResolvedValue([]);
  vi.spyOn(approvalApi, 'listPendingApprovals').mockResolvedValue([]);
  const element = document.createElement('div');
  element.id = 'root';
  document.body.append(element);
  window.history.replaceState({}, '', '/agents');
  const createRoot = ReactDOM.createRoot;
  let root: Root | undefined;
  vi.spyOn(ReactDOM, 'createRoot').mockImplementation((...args) => {
    root = createRoot(...args);
    return root;
  });
  try {
    // main mounts immediately, so install the root and delayed identity before loading it.
    await act(async () => { await import('./main'); });
    expect(await screen.findByRole('status')).toHaveTextContent('Loading…');
    expect(screen.queryByRole('img', { name: 'Agentic Identity Broker' })).not.toBeInTheDocument();
    expect(consentApi.getAgentDelegations).not.toHaveBeenCalled();
    await act(async () => identity.resolve({ principal: 'alice', displayName: 'Alice' }));
    expect(await screen.findByRole('navigation', { name: 'Main navigation' })).toBeVisible();
    expect(await screen.findByRole('heading', { name: 'Agents', exact: true }, { timeout: 5000 })).toBeVisible();
  } finally {
    await act(async () => root?.unmount());
    element.remove();
    window.history.replaceState({}, '', '/');
    vi.restoreAllMocks();
    vi.unstubAllGlobals();
  }
});
