import { createElement, type ReactNode } from 'react';
import { renderHook, waitFor } from '@testing-library/react';
import { beforeEach, expect, it, vi } from 'vitest';
import { consentApi } from '@services/api/consent';
import { QueryProvider } from '@services/query/QueryProvider';
import { createQueryClient } from '@services/query/queryClient';
import { useAgentDetail } from './useAgentDetail';
import { useAgentDecision } from './useAgentDecision';
import { detail, grant } from '../pages/agentConsentTestSupport';
vi.mock('@services/api/consent', () => ({ consentApi: { getUserInfo: vi.fn(), getAgentDetail: vi.fn(), getAgentGrants: vi.fn() } }));
beforeEach(() => {
  vi.resetAllMocks();
  vi.mocked(consentApi.getUserInfo).mockResolvedValue({ principal: 'alice', displayName: 'Alice' });
  vi.mocked(consentApi.getAgentDetail).mockResolvedValue(detail);
  vi.mocked(consentApi.getAgentGrants).mockResolvedValue([grant]);
});
it('keeps console reads principal-scoped and separate from authorization context', async () => {
  const client = createQueryClient();
  const wrapper = ({ children }: { children: ReactNode }) => createElement(QueryProvider, { client }, children);
  const view = renderHook(() => useAgentDetail('agent'), { wrapper });
  await waitFor(() => expect(view.result.current.data).toEqual(detail));
  expect(client.getQueryData(['principal', 'alice', 'agent', 'agent'])).toEqual(detail);
  expect(consentApi.getAgentDetail).toHaveBeenCalledWith('agent', { signal: expect.any(AbortSignal) });
  view.unmount(); client.clear();
});
it('starts grant and decision reads in parallel and discards authorization data when unmounted', async () => {
  const client = createQueryClient();
  vi.mocked(consentApi.getAgentDetail).mockImplementation(() => new Promise(() => {}));
  const wrapper = ({ children }: { children: ReactNode }) => createElement(QueryProvider, { client }, children);
  const view = renderHook(() => useAgentDecision('agent', 'authorization'), { wrapper });
  await waitFor(() => expect(consentApi.getAgentGrants).toHaveBeenCalled());
  expect(consentApi.getAgentDetail).toHaveBeenCalledWith('agent', { sessionToken: 'authorization', signal: expect.any(AbortSignal) });
  view.unmount();
  await waitFor(() => expect(client.getQueryState(['principal', 'alice', 'agent-decision', 'agent', 'authorization'])).toBeUndefined());
  client.clear();
});
