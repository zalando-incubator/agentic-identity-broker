import { afterEach, beforeEach, expect, vi } from 'vitest';
import { cleanup, configure, getConfig, render } from '@testing-library/react';
import { AxiosError, type InternalAxiosRequestConfig } from 'axios';
import type { QueryClient } from '@tanstack/react-query';
import { toast } from 'sonner';
import { apiClient } from '@services/api/client';
import { QueryProvider } from '@services/query/QueryProvider';
import { createQueryClient } from '@services/query/queryClient';
import { ThemeProvider } from '@design-system/theme/ThemeProvider';
import type { GetAgentDetailResponse } from '@app-types/consent';
import App from '../App';
import { detail, grant } from './agentConsentTestSupport';

export { grant };
export const agentDetail: GetAgentDetailResponse['data'] = {
  agent: {
    agentId: detail.agent.agentId,
    display_name: detail.agent.displayName,
    description: detail.agent.description,
    governance_url: detail.agent.governanceUrl,
    user_documentation_url: detail.agent.userDocumentationUrl,
    created_at: grant.created_at,
    updated_at: grant.updated_at,
  },
  services: detail.services,
  permission_sets: detail.agent.permission_sets,
  active_session_service_ids: detail.agent.active_session_service_ids,
  service_requirements: detail.agent.service_requirements,
  cimd_metadata: detail.cimd_metadata,
};

export interface HttpReply {
  body: unknown;
  status?: number;
}

const originalAdapter = apiClient.defaults.adapter;
const originalAsyncTimeout = getConfig().asyncUtilTimeout;
const clients: QueryClient[] = [];
const unexpectedRequests: string[] = [];

beforeEach(() => {
  configure({ asyncUtilTimeout: 5_000 });
  localStorage.clear();
  sessionStorage.clear();
  vi.stubGlobal('matchMedia', (media: string) => Object.assign(new EventTarget(), { matches: false, media }));
});
afterEach(() => {
  cleanup();
  configure({ asyncUtilTimeout: originalAsyncTimeout });
  clients.splice(0).forEach(client => client.clear());
  apiClient.defaults.adapter = originalAdapter;
  toast.dismiss();
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
  localStorage.clear();
  sessionStorage.clear();
  window.history.replaceState({}, '', '/');
  expect(unexpectedRequests.splice(0)).toEqual([]);
});

export function renderApplication(route: string, respond: (config: InternalAxiosRequestConfig) => HttpReply | undefined | Promise<HttpReply | undefined>) {
  const requests = vi.fn(async (config: InternalAxiosRequestConfig) => {
    let reply = await respond(config);
    if (!reply && config.method === 'get' && config.url === '/me') {
      reply = { body: { data: { principal: 'alice', displayName: 'Alice' } } };
    }
    if (!reply && config.method === 'get' && ['/approvals/pending', '/approvals/permanent'].includes(config.url ?? '')) {
      reply = { body: { data: [] } };
    }
    if (!reply) {
      unexpectedRequests.push(`${config.method} ${config.url}`);
      throw new Error(`Unexpected integration request: ${config.method} ${config.url}`);
    }
    const response = { config, data: reply.body, status: reply.status ?? 200, statusText: '', headers: {} };
    if (response.status >= 400) throw new AxiosError('HTTP request failed', 'ERR_BAD_RESPONSE', config, undefined, response);
    return response;
  });
  apiClient.defaults.adapter = requests;
  window.history.replaceState({}, '', route);
  const client = createQueryClient();
  client.setDefaultOptions({ queries: { retry: false }, mutations: { retry: false } });
  clients.push(client);
  return { ...render(<ThemeProvider><QueryProvider client={client}><App /></QueryProvider></ThemeProvider>), requests, client };
}
