/**
 * API service for third-party OAuth2 session management.
 *
 * Provides type-safe methods for interacting with the OAuth2 session backend APIs.
 * Query owns caching; each read reaches the transport and supports cancellation.
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

  /** Whether the entire session is expired */
  is_expired: boolean;

  /** Whether the access token specifically is expired */
  access_token_expired: boolean;

  /** Whether a refresh token is stored for this session */
  has_refresh_token: boolean;

  /** ISO 8601 timestamp when refresh token expires (if applicable) */
  refresh_token_expires_at?: string;

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
  session: {
    id: string;
    principal: string;
    service_id: string;
    token_type: string;
    access_token_expires_at?: string;
    refresh_token_expires_at?: string;
    scope: string[] | null;
    encryption_context: { service_id: string };
    initiated_at: string;
    created_at: string;
    updated_at: string;
  };
  dependent_agents: AgentInfo[];
  dependent_agent_count: number;
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
  /** Read the acting user's stored OAuth2 sessions. */
  async listSessions(options?: { signal?: AbortSignal }): Promise<SessionSummary[]> {
    const response = await apiClient.get<ListSessionsResponse>(
      '/third-party/sessions',
      { signal: options?.signal },
    );
    return response.data.data.sessions;
  }

  /** Read a session and its dependent agents. */
  async getSessionDetails(
    serviceId: string,
    options?: { signal?: AbortSignal },
  ): Promise<SessionDetail> {
    const response = await apiClient.get<GetSessionDetailResponse>(
      `/third-party/${serviceId}/session`,
      { signal: options?.signal },
    );
    return response.data.data;
  }

  /** Disconnect at the broker without claiming to revoke provider-side tokens. */
  async terminateSession(serviceId: string): Promise<void> {
    await apiClient.delete(`/third-party/${serviceId}/session`);
  }

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
