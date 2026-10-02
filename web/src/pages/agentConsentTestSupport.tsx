import { render } from '@testing-library/react';
import { MemoryRouter, Route, Routes, useLocation } from 'react-router-dom';
import type { ReactNode } from 'react';
import { QueryProvider } from '@services/query/QueryProvider';
import { createQueryClient } from '@services/query/queryClient';
import type { AgentDetailData } from '@services/api/consent';
import type { UserGrant } from '../types/consent';

export const detail: AgentDetailData = {
  agent: {
    agentId: 'agent', displayName: 'Research Agent', description: 'Find documents for your work.',
    logoUrl: 'https://external.example/agent.png', governanceUrl: 'https://example.com/governance',
    userDocumentationUrl: 'https://example.com/docs', active_session_service_ids: ['mail', 'drive'],
    service_requirements: [{ service_id: 'mail', requirement_type: 'mandatory' }, { service_id: 'drive', requirement_type: 'optional' }],
    permission_sets: [
      { requirement_type: 'optional', permission_set: { id: 'write', name: 'Write documents', description: 'Create documents in your drive.', service_scopes: [{ service_id: 'drive', requirement_type: 'optional' }] } },
      { requirement_type: 'mandatory', permission_set: { id: 'read', name: 'Read mail', description: 'Find messages in your mailbox.', service_scopes: [{ service_id: 'mail', requirement_type: 'mandatory' }] } },
      { requirement_type: 'optional', permission_set: { id: 'prior', name: 'Existing access', description: 'Read existing documents.', service_scopes: [{ service_id: 'drive', requirement_type: 'optional' }, { service_id: 'mail', requirement_type: 'optional' }] } },
    ],
  },
  services: [
    { kind: 'requirement', serviceId: 'mail', serviceName: 'Mail', requirementType: 'mandatory', connectionStatus: 'connected', requiredScopes: [{ name: 'mail:read', description: 'Read mail' }] },
    { kind: 'requirement', serviceId: 'drive', serviceName: 'Drive', requirementType: 'optional', connectionStatus: 'connected', requiredScopes: [{ name: 'drive:write', description: 'Write drive' }] },
  ],
  cimd_metadata: { client_id_url: 'https://trusted.example/client.json', redirect_uri: 'http://localhost:3001/callback', verified_domain: 'trusted.example', requested_scopes: ['mail:read'] },
};
export const grant: UserGrant = { id: 'grant', agent_id: 'agent', principal: 'alice', granted_permission_sets: { read: ['mail'], prior: ['drive'] }, valid_until: '2099-06-10T15:24:31.123Z', created_at: '2026-01-01T00:00:00Z', updated_at: '2026-01-01T00:00:00Z' };
export function renderAgentPage(page: ReactNode, query = '') {
  const client = createQueryClient();
  client.setDefaultOptions({ queries: { retry: false }, mutations: { retry: false } });
  function Location() { return <span aria-label="Current route">{useLocation().pathname}</span>; }
  const view = render(<QueryProvider client={client}><MemoryRouter initialEntries={[`/agents/agent${query}`]}><Location /><Routes><Route path="/agents/:agentId" element={page} /><Route path="/delegations" element={<p>Agent list</p>} /></Routes></MemoryRouter></QueryProvider>);
  return { ...view, client };
}
