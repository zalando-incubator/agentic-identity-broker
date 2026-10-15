/**
 * API service for third-party OAuth2 session management.
 *
 * Provides type-safe methods for interacting with the OAuth2 session backend APIs.
 * Includes caching layer for GET requests to reduce network load.
 */

import { apiClient } from './client';
import { apiCache } from './cache';

/**
 * Summary of a user's OAuth2 session with a third-party service.
 * Returned by GET /api/third-party/sessions
 */
export interface SessionSummary {
  /** Unique session identifier */
  id: string;

  /** Third-party service identifier (e.g., "google", "github") */
  service_id: string;

  /** Human-readable service name */
  service_display_name: string;

  /** OAuth2 token type (e.g., "Bearer") */
  token_type: string;

  /** Granted OAuth2 scopes */
  scope: string[];

  /** ISO 8601 timestamp when session was initiated */
  initiated_at: string;

  /** Whether provider rejection or exhausted access-only credentials require login */
  requires_reauthentication: boolean;

  /** Whether the access token specifically is expired */
  access_token_expired: boolean;

  /** Whether a refresh token is stored for this session */
  has_refresh_token: boolean;

  /** Number of agents depending on this session */
  dependent_agent_count: number;

  /** Whether session data is encrypted at rest */
  is_encrypted: boolean;
}

/**
 * Response from GET /api/third-party/sessions
 */
export interface ListSessionsResponse {
  data: {
    sessions: SessionSummary[];
  };
}

/**
 * Agent information with ID and display name.
 * Represents an agent that uses a session.
 */
export interface AgentInfo {
  /** Unique agent identifier */
  id: string;
  /** Human-readable agent display name */
  display_name: string;
}

/**
 * Detailed session information including dependent agents.
 * Returned by GET /api/third-party/:service-id/session
 */
export interface SessionDetail {
  session: SessionSummary;
  dependent_agents: AgentInfo[];
}

/**
 * Response from GET /api/third-party/:service-id/session
 */
export interface GetSessionDetailResponse {
  data: SessionDetail;
}

/**
 * Third-party OAuth2 sessions API service class.
 */
export class SessionsApiService {
  /**
   * Fetch all OAuth2 sessions for the current user.
   * Results are cached for 2 minutes.
   *
   * @returns Array of session summaries
   * @throws {ApiError} if request fails
   */
  async listSessions(): Promise<SessionSummary[]> {
    const cacheKey = '/third-party/sessions';

    // Check cache first
    const cached = apiCache.get<SessionSummary[]>(cacheKey);
    if (cached) {
      return cached;
    }

    // Fetch from API
    const response = await apiClient.get<ListSessionsResponse>(
      '/third-party/sessions',
    );
    const sessions = response.data.data.sessions || [];

    // Cache the result (2 minutes TTL)
    apiCache.set(cacheKey, sessions, 2 * 60 * 1000);

    return sessions;
  }

  /**
   * Get detailed information about a specific session including dependent agents.
   * Results are cached for 2 minutes.
   *
   * @param serviceId - Third-party service identifier
   * @returns Detailed session information
   * @throws {ApiError} if request fails or session not found
   */
  async getSessionDetails(serviceId: string): Promise<SessionDetail> {
    const cacheKey = `/third-party/${serviceId}/session`;

    // Check cache first
    const cached = apiCache.get<SessionDetail>(cacheKey);
    if (cached) {
      return cached;
    }

    // Fetch from API
    const response = await apiClient.get<GetSessionDetailResponse>(
      `/third-party/${serviceId}/session`,
    );
    const data = response.data.data;

    // Cache the result (2 minutes TTL)
    apiCache.set(cacheKey, data, 2 * 60 * 1000);

    return data;
  }

  /**
   * Terminate an OAuth2 session with a third-party service.
   * Invalidates relevant caches after successful termination.
   *
   * @param serviceId - Third-party service identifier
   * @throws {ApiError} if request fails
   */
  async terminateSession(serviceId: string): Promise<void> {
    await apiClient.delete(`/third-party/${serviceId}/session`);

    // Invalidate caches since data changed
    apiCache.invalidatePattern('/third-party/*');
  }

  /**
   * Refresh an OAuth2 session (renew access token using refresh token).
   * Invalidates relevant caches even when provider rejection changes session state.
   *
   * @param serviceId - Third-party service identifier
   * @returns Updated session information
   * @throws {ApiError} if request fails or refresh token is invalid
   */
  async refreshSession(serviceId: string): Promise<SessionSummary> {
    try {
      const response = await apiClient.post<{ data: SessionSummary }>(
        `/third-party/${serviceId}/session/refresh`,
      );
      return response.data.data;
    } finally {
      apiCache.invalidatePattern('/third-party/*');
    }
  }
}

/**
 * Singleton instance of the sessions API service.
 * Use this throughout the application for OAuth2 session API calls.
 */
export const sessionsApi = new SessionsApiService();
