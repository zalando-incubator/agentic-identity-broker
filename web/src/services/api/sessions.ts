/**
 * API service for third-party OAuth2 session management.
 *
 * Provides type-safe methods for interacting with the OAuth2 session backend APIs.
 */

import { apiClient } from './client';

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

  /**
   * The provider rejected the stored refresh token, access expired without one, or the
   * establishing client identity or revision is unusable. The user must sign in again.
   */
  requires_reauthentication: boolean;

  /** Whether the access token specifically is expired */
  access_token_expired: boolean;

  /** Whether a refresh token is stored for this session (it may have been rejected) */
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
   *
   * @returns Array of session summaries
   * @throws {ApiError} if request fails
   */
  async listSessions(): Promise<SessionSummary[]> {
    const response = await apiClient.get<ListSessionsResponse>(
      '/third-party/sessions',
    );
    return response.data.data.sessions || [];
  }

  /**
   * Get detailed information about a specific session including dependent agents.
   *
   * @param serviceId - Third-party service identifier
   * @returns Detailed session information
   * @throws {ApiError} if request fails or session not found
   */
  async getSessionDetails(serviceId: string): Promise<SessionDetail> {
    const response = await apiClient.get<GetSessionDetailResponse>(
      `/third-party/${serviceId}/session`,
    );
    return response.data.data;
  }

  /**
   * Terminate an OAuth2 session with a third-party service.
   *
   * @param serviceId - Third-party service identifier
   * @throws {ApiError} if request fails
   */
  async terminateSession(serviceId: string): Promise<void> {
    await apiClient.delete(`/third-party/${serviceId}/session`);
  }

  /**
   * Refresh an OAuth2 session (renew access token using refresh token).
   *
   * @param serviceId - Third-party service identifier
   * @returns Updated session information
   * @throws {ApiError} if request fails or refresh token is invalid
   */
  async refreshSession(serviceId: string): Promise<SessionSummary> {
    const response = await apiClient.post<{ data: SessionSummary }>(
      `/third-party/${serviceId}/session/refresh`,
    );
    return response.data.data;
  }
}

/**
 * Singleton instance of the sessions API service.
 * Use this throughout the application for OAuth2 session API calls.
 */
export const sessionsApi = new SessionsApiService();
