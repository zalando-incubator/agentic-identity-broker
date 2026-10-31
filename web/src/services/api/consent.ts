/**
 * API service for consent management endpoints.
 *
 * Provides type-safe methods for interacting with the consent backend APIs.
 * Includes caching layer for GET requests to reduce network load.
 */

import { isAxiosError } from 'axios';
import { apiClient } from './client';
import { apiCache } from './cache';
import { isSafeRedirectUrl } from '../../utils/validation';
import type {
  UserInfo,
  AgentDelegation,
  AgentDetail,
  ThirdpartyService,
  ServiceWithScopes,
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

/**
 * Consent API service class providing all consent-related endpoints.
 */
export class ConsentApiService {
  /**
   * Get current authenticated user information.
   * Results are cached for 5 minutes.
   *
   * @returns User information
   * @throws {ApiError} if request fails
   */
  async getUserInfo(): Promise<UserInfo> {
    const cacheKey = '/me';

    // Check cache first
    const cached = apiCache.get<UserInfo>(cacheKey);
    if (cached) {
      return cached;
    }

    // Fetch from API
    const response = await apiClient.get<GetUserInfoResponse>('/me');
    const data = response.data.data;

    // Cache the result (5 minutes TTL)
    apiCache.set(cacheKey, data, 5 * 60 * 1000);

    return data;
  }

  /**
   * Get all agent delegations for the current user.
   * Results are cached for 5 minutes.
   *
   * @returns Array of agent delegations
   * @throws {ApiError} if request fails
   */
  async getAgentDelegations(): Promise<AgentDelegation[]> {
    const cacheKey = '/consent/agents';

    // Check cache first
    const cached = apiCache.get<AgentDelegation[]>(cacheKey);
    if (cached) {
      return cached;
    }

    // Fetch from API
    const response =
      await apiClient.get<GetAgentDelegationsResponse>('/consent/agents');
    const data = response.data.data;

    // Cache the result (5 minutes TTL)
    apiCache.set(cacheKey, data, 5 * 60 * 1000);

    return data;
  }

  /**
   * Get detailed information about a specific agent including permission sets and CIMD metadata.
   * Pass sessionToken for CIMD authorization flows. Results are cached for 5 minutes.
   *
   * @param agentId - Unique agent identifier
   * @param options - Optional: sessionToken for session-based CIMD flows
   * @returns Agent details, services, permission sets, session info, and optional CIMD metadata
   * @throws {ApiError} if request fails or agent not found
   */
  async getAgentDetail(
    agentId: string,
    options?: {
      sessionToken?: string;
    },
  ): Promise<{
    agent: AgentDetail;
    services: ThirdpartyService[];
    cimd_metadata?: CIMDMetadata | null;
  }> {
    let agentUrl = `/consent/agents/${agentId}`;
    if (options?.sessionToken) {
      agentUrl += `?session_token=${encodeURIComponent(options.sessionToken)}`;
    }

    const cacheKey = `/consent/agents/${agentId}${options?.sessionToken ? `?session_token=${options.sessionToken}` : ''}`;

    const cached = apiCache.get<{
      agent: AgentDetail;
      services: ThirdpartyService[];
      cimd_metadata?: CIMDMetadata | null;
    }>(cacheKey);
    if (cached) {
      return cached;
    }

    const response = await apiClient.get<GetAgentDetailResponse>(agentUrl);
    const responseData = response.data.data;

    const agent: AgentDetail = {
      agentId: responseData.agent.agentId,
      displayName: responseData.agent.display_name,
      description: responseData.agent.description,
      governanceUrl: responseData.agent.governance_url,
      userDocumentationUrl: responseData.agent.user_documentation_url,
      agentInterfaceUrl: responseData.agent.agent_interface_url,
      permission_sets: responseData.permission_sets,
      active_session_service_ids: responseData.active_session_service_ids,
      service_requirements: responseData.service_requirements,
      requested_service_ids: responseData.requested_service_ids ?? [],
      requested_service_coverage: responseData.requested_service_coverage ?? [],
    };

    const rawServices = responseData.services ?? [];
    const services: ThirdpartyService[] = rawServices.map((s) => {
      const obj = s as unknown as Record<string, unknown>;
      return 'requirementType' in obj && obj.requirementType
        ? ({ ...obj, kind: 'requirement' } as ServiceRequirement)
        : ({ ...obj, kind: 'scoped' } as ServiceWithScopes);
    });

    const data = { agent, services, cimd_metadata: responseData.cimd_metadata };

    if (!options?.sessionToken) {
      apiCache.set(cacheKey, data, 5 * 60 * 1000);
    }

    return data;
  }

  /**
   * Get grant for a specific agent for the current user.
   * Results are cached for 5 minutes.
   *
   * @param agentId - Unique agent identifier
   * @returns User grant for this agent (null if no grant exists)
   * @throws {ApiError} if request fails
   */
  async getAgentGrants(agentId: string): Promise<UserGrant | null> {
    const cacheKey = `/consent/agents/${agentId}/grants`;

    // Check cache first
    const cached = apiCache.get<UserGrant | null>(cacheKey);
    if (cached) {
      return cached;
    }

    // Fetch from API
    const response = await apiClient.get<GetAgentGrantsResponse>(
      `/consent/agents/${agentId}/grants`,
    );
    const data = response.data.data;

    // Cache the result (5 minutes TTL)
    apiCache.set(cacheKey, data, 5 * 60 * 1000);

    return data;
  }

  /**
   * Create or update a grant for a specific agent.
   * Invalidates relevant caches after successful update.
   *
   * @param agentId - Unique agent identifier
   * @param request - Grant configuration
   * @returns Created or updated grant (or null if redirected)
   * @throws {ApiError} if request fails or validation errors
   */
  async createOrUpdateGrant(
    agentId: string,
    request: CreateOrUpdateGrantRequest,
    options?: { sessionToken?: string },
  ): Promise<GrantResult> {
    let url = `/consent/agents/${agentId}/grants`;
    if (options?.sessionToken) {
      url += `?session_token=${encodeURIComponent(options.sessionToken)}`;
    }

    const response = await apiClient.post<CreateOrUpdateGrantResponse>(
      url,
      request,
    );

    // Invalidate caches for this agent since data changed
    apiCache.invalidatePattern(`/consent/agents/${agentId}*`);
    apiCache.invalidate('/consent/agents');

    // Handle 204 No Content response (grant revoked with empty tokens)
    if (response.status === 204) {
      return { kind: 'noContent' };
    }

    // Handle 201 Created response
    if (response.status === 201) {
      const redirectUrl = response.data.redirect_url;
      if (redirectUrl) {
        if (!isSafeRedirectUrl(redirectUrl)) {
          throw new Error(
            'Redirect URL validation failed: URL must be same-origin',
          );
        }
        return { kind: 'redirect', redirectUrl };
      }

      return { kind: 'created', grant: response.data.data };
    }

    // Unexpected status
    throw new Error(`Unexpected status code: ${response.status}`);
  }

  /**
   * Delete all grants for a specific agent (hard delete).
   * Calls DELETE /consent/agents/{agentId}/grants.
   * Invalidates relevant caches after successful deletion.
   *
   * @param agentId - Unique agent identifier
   * @throws {ApiError} if request fails; 404 throws a user-friendly message
   */
  async deleteGrant(agentId: string): Promise<void> {
    try {
      await apiClient.delete(`/consent/agents/${agentId}/grants`);
    } catch (err) {
      if (isAxiosError(err) && err.response?.status === 404) {
        throw new Error('Grant not found — it may have already been revoked');
      }
      throw err;
    }

    // Invalidate caches on success
    apiCache.invalidatePattern(`/consent/agents/${agentId}*`);
    apiCache.invalidate('/consent/agents');
  }
}

/**
 * Singleton instance of the consent API service.
 * Use this throughout the application for API calls.
 */
export const consentApi = new ConsentApiService();
