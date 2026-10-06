import type { AgentDetailData } from '@services/api/consent';
import type { SessionDetail, SessionSummary } from '@services/api/sessions';
import type { AgentDelegation, ResolvedPermissionSetEntry, UserGrant } from '@app-types/consent';
import type { ScopePreview, ToolApprovalDetail } from '@app-types/approval';

// Fixed fixture dates are paired with the screenshot runner's fixed Date.now().
export const storyNow = Date.parse('2026-10-04T12:00:00Z');
export const longAgentName = 'International records and regulatory evidence preservation assistant for operations and compliance reporting'.slice(0, 80);
export const longDescription = ('This agent reviews signed records from the connected services and compares their contents with the retention rules before organizing a summary for your compliance team. ').repeat(4).slice(0, 400);

export const agents: AgentDelegation[] = [
  { agentId: 'calendar', displayName: 'Calendar assistant', activeGrantCount: 2, lastModifiedAt: '2026-10-03T12:00:00Z', expiresAt: '2026-10-08T12:00:00Z' },
  { agentId: 'research', displayName: 'Research assistant', activeGrantCount: 1, lastModifiedAt: '2026-09-19T12:00:00Z' },
  { agentId: 'planning', displayName: 'Planning assistant', activeGrantCount: 3, lastModifiedAt: '2026-09-26T12:00:00Z', expiresAt: '2026-11-12T12:00:00Z' },
];
export const denseAgents: AgentDelegation[] = Array.from({ length: 13 }, (_, index) => ({
  ...agents[index % agents.length]!, agentId: `agent-${index + 1}`, displayName: `Agent ${index + 1}`,
}));
export const longAgent: AgentDelegation = { ...agents[1]!, agentId: 'international-records', displayName: longAgentName };

const permissionGroups = Array.from({ length: 8 }, (_, index): ResolvedPermissionSetEntry => ({
  requirement_type: index === 0 ? 'mandatory' : 'optional',
  permission_set: {
    id: `group-${index + 1}`,
    name: index === 0 ? 'Read mail' : `Access group ${index + 1}`,
    description: index === 0 ? 'Search existing messages in your inbox.' : `Use the connected drive for workflow ${index + 1}.`,
    service_scopes: index === 2
      ? [{ service_id: 'mail', requirement_type: 'mandatory' }, { service_id: 'drive', requirement_type: 'optional' }]
      : [{ service_id: index === 0 ? 'mail' : 'drive', requirement_type: index === 0 ? 'mandatory' : 'optional' }],
  },
}));
export const consentDetail: AgentDetailData = {
  agent: {
    agentId: 'research-agent', displayName: 'Research assistant',
    description: 'Find documents and organize your work across connected services.',
    governanceUrl: 'https://example.com/governance', userDocumentationUrl: 'https://example.com/docs',
    agentInterfaceUrl: 'https://example.com/agent',
    permission_sets: permissionGroups.slice(0, 3), active_session_service_ids: ['mail', 'drive'],
    service_requirements: [{ service_id: 'mail', requirement_type: 'mandatory' }, { service_id: 'drive', requirement_type: 'optional' }],
  },
  services: [
    { kind: 'requirement', serviceId: 'mail', serviceName: 'Mail', requirementType: 'mandatory', connectionStatus: 'connected', requiredScopes: [{ name: 'mail:read', description: 'Read messages' }] },
    { kind: 'requirement', serviceId: 'drive', serviceName: 'Drive', requirementType: 'optional', connectionStatus: 'connected', requiredScopes: [{ name: 'drive:write', description: 'Write files' }] },
  ],
  cimd_metadata: { client_id_url: 'https://trusted.example/agent.json', redirect_uri: 'https://trusted.example/callback', verified_domain: 'trusted.example', requested_scopes: [] },
};
export const consentGrant: UserGrant = {
  id: 'grant', agent_id: consentDetail.agent.agentId, principal: 'alice',
  granted_permission_sets: { 'group-1': ['mail'], 'group-3': ['mail', 'drive'] },
  valid_until: '2099-06-10T15:24:31.123Z', created_at: '2026-09-01T00:00:00Z', updated_at: '2026-10-03T00:00:00Z',
};
export const consentDenseDetail: AgentDetailData = {
  ...consentDetail, agent: { ...consentDetail.agent, displayName: longAgentName, description: longDescription, permission_sets: permissionGroups },
};
export const consentRiskDetail: AgentDetailData = {
  ...consentDetail,
  cimd_metadata: { ...consentDetail.cimd_metadata!, redirect_uri: 'http://localhost:3001/callback' },
};
export const consentEmptyDetail: AgentDetailData = {
  ...consentDetail,
  agent: { ...consentDetail.agent, permission_sets: [], service_requirements: [], governanceUrl: undefined, userDocumentationUrl: undefined, agentInterfaceUrl: undefined },
  services: [],
};

export const connectedSession: SessionSummary = {
  id: 'mail-connection', service_id: 'mail', service_display_name: 'Mail', token_type: 'Bearer',
  scope: ['mail.read', 'mail.send'], initiated_at: '2026-09-12T10:00:00Z', is_expired: false,
  access_token_expired: false, has_refresh_token: true, dependent_agent_count: 1, is_encrypted: true,
};
export const needsSignInSession: SessionSummary = {
  ...connectedSession, id: 'calendar-connection', service_id: 'calendar', service_display_name: 'Calendar',
  scope: ['calendar.read'], initiated_at: '2026-09-10T10:00:00Z', access_token_expired: true,
};
export const expiredSession: SessionSummary = {
  ...connectedSession, id: 'files-connection', service_id: 'files', service_display_name: 'Files',
  scope: ['files.read'], initiated_at: '2026-09-01T10:00:00Z', is_expired: true, has_refresh_token: false,
};
export const unavailableSession: SessionSummary = {
  ...connectedSession, id: 'ledger-connection', service_id: 'ledger', service_display_name: 'Ledger',
  scope: ['ledger.read'], initiated_at: '2026-08-22T10:00:00Z',
};
export const sessions: SessionSummary[] = [connectedSession, needsSignInSession, expiredSession];
export const denseSessions: SessionSummary[] = Array.from({ length: 13 }, (_, index) => ({
  ...connectedSession, id: `service-${index}-connection`, service_id: `service-${index}`,
  service_display_name: `Service ${String(index + 1).padStart(2, '0')}`,
}));
export const sessionByService: Record<string, SessionSummary> = {
  mail: connectedSession, calendar: needsSignInSession, files: expiredSession, ledger: unavailableSession,
};
export const sessionDetail: SessionDetail = {
  session: {
    id: connectedSession.id, principal: 'alex@example.com', service_id: connectedSession.service_id,
    token_type: connectedSession.token_type, scope: connectedSession.scope,
    encryption_context: { service_id: connectedSession.service_id }, initiated_at: connectedSession.initiated_at,
    created_at: connectedSession.initiated_at, updated_at: connectedSession.initiated_at,
  },
  dependent_agents: [{ id: 'helper', display_name: 'Mail helper' }], dependent_agent_count: 1,
};

export const calendarRequest: ToolApprovalDetail = {
  id: 'calendar-event', principal: 'alex@example.com', agent_id: 'project-coordinator', agent_display_name: 'Project coordinator',
  tool_name: 'calendar.create_event', description: 'Add a safety review with the release team to the shared calendar.',
  arguments: { calendar: 'team-ops', title: 'Release safety review', start: '2026-10-06T14:00:00Z', attendees: ['alex@example.com', 'sam@example.com'] },
  tool_pattern: 'calendar.create_event', params_pattern: { calendar: 'team-ops', title: 'Release safety review' },
  pattern_preview: 'calendar.create_event(calendar=team-ops, title=Release safety review)',
  risk_level: 'medium', status: 'pending', approval_url: '/approvals/calendar-event',
  created_at: '2026-10-04T11:58:00Z', expires_at: '2099-06-10T15:24:31Z',
};
export const fileRequest: ToolApprovalDetail = {
  ...calendarRequest, id: 'read-project-file', tool_name: 'filesystem.read_file',
  description: 'Read the signed release checklist from the project workspace.',
  arguments: { path: '/projects/autumn-release/signed-checklist.pdf' }, tool_pattern: 'filesystem.read_file',
  params_pattern: { path: '/projects/autumn-release/signed-checklist.pdf' },
  pattern_preview: 'filesystem.read_file(path=/projects/autumn-release/signed-checklist.pdf)',
  risk_level: 'low', approval_url: '/approvals/read-project-file',
};
export const criticalRequest: ToolApprovalDetail = {
  ...calendarRequest, id: 'delete-archived-records', tool_name: 'filesystem.delete_directory',
  description: 'Remove every archived financial report in the shared finance workspace.',
  arguments: { path: '/shared/finance/archive/2025', recursive: true }, tool_pattern: 'filesystem.delete_directory',
  params_pattern: { path: '/shared/finance/archive/2025', recursive: 'true' },
  pattern_preview: 'filesystem.delete_directory(path=/shared/finance/archive/2025, recursive=true)',
  risk_level: 'critical', approval_url: '/approvals/delete-archived-records',
};
export const expiredRequest: ToolApprovalDetail = {
  ...calendarRequest, created_at: '2026-10-04T11:40:00Z', expires_at: '2026-10-04T11:55:00Z',
};
export const pendingRequests: ToolApprovalDetail[] = [calendarRequest, fileRequest, criticalRequest];
export const denseRequests: ToolApprovalDetail[] = Array.from({ length: 13 }, (_, index) => ({
  ...(index % 2 === 0 ? calendarRequest : fileRequest), id: `request-${index + 1}`,
  agent_display_name: ['Project coordinator', 'Documentation assistant', 'Finance assistant'][index % 3],
  approval_url: `/approvals/request-${index + 1}`,
}));
export const rememberedAllow: ToolApprovalDetail = {
  ...calendarRequest, id: 'remembered-allow', status: 'approved', persistence: 'permanent',
  approved_at: '2026-10-02T12:00:00Z', created_at: '2026-10-02T11:55:00Z', expires_at: '2026-10-02T12:10:00Z',
};
export const rememberedDeny: ToolApprovalDetail = {
  ...criticalRequest, id: 'remembered-deny', status: 'denied', persistence: 'permanent',
  denied_at: '2026-10-02T13:00:00Z', created_at: '2026-10-02T12:55:00Z', expires_at: '2026-10-02T13:10:00Z',
};
export const rememberedRequests: ToolApprovalDetail[] = [rememberedAllow, rememberedDeny];
export const longApproval: ToolApprovalDetail = {
  ...calendarRequest, id: 'approval-long-records', agent_display_name: longAgentName,
  tool_name: 'document_repository.collect_signed_vendor_agreements_and_contract_amendments_for_regulatory_compliance_case',
  description: longDescription,
  arguments: {
    folder: '/operations/vendors/agreements/2026', case_id: 'SEC-2068', include_nested_folders: true,
    include_versions: false, types: ['agreement', 'statement-of-work', 'amendment'],
    reviewers: ['security', 'finance'], include_metadata: true, retention_days: 30,
  },
  tool_pattern: 'document_repository.collect_signed_vendor_agreements_and_contract_amendments_for_regulatory_compliance_case',
  params_pattern: { folder: '/operations/vendors/agreements/2026', case_id: 'SEC-2068' },
  pattern_preview: 'document_repository.collect_signed_vendor_agreements_and_contract_amendments_for_regulatory_compliance_case(folder=/operations/vendors/agreements/2026, case_id=SEC-2068)',
  approval_url: '/approvals/approval-long-records',
};

// Only the supplied request scope has a server-resolved preview in these fixtures.
export function exactApprovalPreview(detail: ToolApprovalDetail) {
  return async (paramsPattern: Record<string, string>): Promise<ScopePreview> => {
    const exact = Object.keys(paramsPattern).length === Object.keys(detail.params_pattern).length
      && Object.entries(detail.params_pattern).every(([key, value]) => paramsPattern[key] === value);
    if (!exact) throw new Error('No scope preview fixture for this pattern.');
    return { tool_pattern: detail.tool_pattern, params_pattern: detail.params_pattern, preview: detail.pattern_preview };
  };
}
