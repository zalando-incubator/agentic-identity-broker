import { apiClient } from './client';
import { isSafeRedirectUrl } from '@utils/validation';
import type {
  UserInfo,
  AgentDelegation,
  AgentDetail,
  ServiceRequirement,
  CIMDMetadata,
  UserGrant,
  GrantResult,
  GetUserInfoResponse,
  GetAgentDelegationsResponse,
  GetAgentDetailResponse,
  GetAgentGrantsResponse,
  CreateOrUpdateGrantRequest,
  CreateOrUpdateGrantResponse,
} from '../../types/consent';

export interface AgentDetailData {
  agent: AgentDetail;
  services: ServiceRequirement[];
  cimd_metadata?: CIMDMetadata;
}

/** Uncached transport; principal-scoped Query observers own read lifetimes. */
export class ConsentApiService {
  async getUserInfo(options?: { signal?: AbortSignal }): Promise<UserInfo> {
    const response = await apiClient.get<GetUserInfoResponse>('/me', { signal: options?.signal });
    return response.data.data;
  }

  async getAgentDelegations(options?: { signal?: AbortSignal }): Promise<AgentDelegation[]> {
    const response = await apiClient.get<GetAgentDelegationsResponse>(
      '/consent/agents',
      { signal: options?.signal },
    );
    return response.data.data;
  }

  async getAgentDetail(
    agentId: string,
    options?: { sessionToken?: string; signal?: AbortSignal },
  ): Promise<AgentDetailData> {
    let agentUrl = `/consent/agents/${agentId}`;
    if (options?.sessionToken) {
      agentUrl += `?session_token=${encodeURIComponent(options.sessionToken)}`;
    }
    const response = await apiClient.get<GetAgentDetailResponse>(agentUrl, { signal: options?.signal });
    const data = response.data.data;
    return {
      agent: {
        agentId: data.agent.agentId,
        displayName: data.agent.display_name,
        clientId: data.agent.client_id,
        clientUris: data.agent.client_uris,
        description: data.agent.description,
        logoUrl: data.agent.logoUrl,
        governanceUrl: data.agent.governance_url,
        userDocumentationUrl: data.agent.user_documentation_url,
        agentInterfaceUrl: data.agent.agent_interface_url,
        permission_sets: data.permission_sets,
        active_session_service_ids: data.active_session_service_ids,
        service_requirements: data.service_requirements,
      },
      services: data.services.map((service) => ({ ...service, kind: 'requirement' })),
      cimd_metadata: data.cimd_metadata,
    };
  }

  async getAgentGrants(
    agentId: string,
    options?: { signal?: AbortSignal },
  ): Promise<UserGrant[]> {
    const response = await apiClient.get<GetAgentGrantsResponse>(
      `/consent/agents/${agentId}/grants`,
      { signal: options?.signal },
    );
    return response.data.data;
  }

  /** A successful grant POST returns 201; revocation uses DELETE instead. */
  async createOrUpdateGrant(
    agentId: string,
    request: CreateOrUpdateGrantRequest,
    options?: { sessionToken?: string },
  ): Promise<GrantResult> {
    let url = `/consent/agents/${agentId}/grants`;
    if (options?.sessionToken) {
      url += `?session_token=${encodeURIComponent(options.sessionToken)}`;
    }
    const response = await apiClient.post<CreateOrUpdateGrantResponse>(url, request);
    if (response.status !== 201) {
      throw new Error(`Unexpected status code: ${response.status}`);
    }
    const redirectUrl = response.data.redirect_url;
    if (redirectUrl) {
      if (!isSafeRedirectUrl(redirectUrl)) {
        throw new Error('Redirect URL validation failed: URL must be same-origin');
      }
      return { kind: 'redirect', redirectUrl };
    }
    return { kind: 'created', grant: response.data.data };
  }

  async deleteGrant(agentId: string): Promise<void> {
    await apiClient.delete(`/consent/agents/${agentId}/grants`);
  }
}

export const consentApi = new ConsentApiService();
