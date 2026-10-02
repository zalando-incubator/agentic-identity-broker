/**
 * TypeScript types for consent management feature.
 * Maps to backend API responses from data-model.md.
 */

/**
 * Current authenticated user information.
 * Returned by GET /api/me
 */
export interface UserInfo {
  /** User's principal identifier (e.g., email, UUID) */
  principal: string;

  /** Human-readable display name */
  displayName: string;

  /** Optional email address extracted from JWT claims */
  email?: string;

  /** URL to user's profile picture/avatar */
  pictureUrl?: string;
}

/**
 * Summary of an agent the user has delegated access to.
 * Returned by GET /api/consent/agents
 */
export interface AgentDelegation {
  /** Unique agent identifier */
  agentId: string;

  /** Agent's display name shown to users */
  displayName: string;

  /** URL to agent's logo/avatar */
  logoUrl?: string;

  /** Number of active service grants for this agent */
  activeGrantCount: number;

  /** ISO 8601 timestamp of last grant modification */
  lastModifiedAt: string;

  /** Optional grant expiration (null = indefinite) */
  expiresAt?: string | null;
}

/**
 * Detailed agent information for grant management page.
 * Returned by GET /api/consent/agents/{id} (unified response)
 */
export interface AgentDetail {
  /** Unique agent identifier */
  agentId: string;

  /** Agent's display name */
  displayName: string;

  /** Agent description/purpose */
  description: string;

  /** URL to agent's logo/avatar */
  logoUrl?: string;

  /** Link to agent governance documentation */
  governanceUrl?: string;

  /** Link to user-facing agent documentation */
  userDocumentationUrl?: string;

  /** Link to agent's public interface (if applicable) */
  agentInterfaceUrl?: string;

  /** Permission sets for this agent */
  permission_sets?: ResolvedPermissionSetEntry[];

  /** Service IDs with active OAuth2 sessions */
  active_session_service_ids?: string[];

  /** Agent's service requirements with mandatory/optional types */
  service_requirements?: Array<{
    service_id: string;
    requirement_type: 'mandatory' | 'optional';
  }>;
}

/**
 * Service entry within a permission set for the consent-info response.
 * Raw scopes are intentionally omitted per FR-007; use requirement_type to determine lock status.
 */
export interface ServiceScopeInfo {
  /** Third-party service identifier */
  service_id: string;

  /** Whether this service is mandatory or optional within the permission set */
  requirement_type: 'mandatory' | 'optional';
}

/**
 * Permission set definition with grouped OAuth2 scopes.
 */
export interface PermissionSetInfo {
  /** Unique permission set identifier */
  id: string;

  /** Human-readable permission set name */
  name: string;

  /** Permission set description */
  description: string;

  /** Scopes grouped by service */
  service_scopes: ServiceScopeInfo[];
}

/**
 * Permission set entry with requirement type (mandatory or optional).
 * Returned by GET /api/consent/agents/{id}/consent-info
 */
export interface ResolvedPermissionSetEntry {
  /** Permission set definition */
  permission_set: PermissionSetInfo;

  /** Whether mandatory (locked) or optional (user-selectable) */
  requirement_type: 'mandatory' | 'optional';
}

/**
 * User's existing grant to an agent.
 * Returned by GET /api/consent/agents/:agent-id/grants
 * Note: Uses snake_case to match backend API response
 */
export interface UserGrant {
  /** Unique grant identifier */
  id: string;

  /** Agent receiving the grant */
  agent_id: string;

  /** User principal who created the grant */
  principal: string;

  /** Granted permission sets: map of PS ID → included service IDs (positive-inclusion model) */
  granted_permission_sets: Record<string, string[]>;

  /** Optional expiration timestamp (null = indefinite) */
  valid_until?: string | null;

  /** ISO 8601 timestamp of grant creation */
  created_at: string;

  /** ISO 8601 timestamp of last update */
  updated_at: string;
}

// API Request/Response Types

/**
 * Response from GET /api/me
 */
export interface GetUserInfoResponse {
  data: UserInfo;
}

/**
 * Response from GET /api/consent/agents
 */
export interface GetAgentDelegationsResponse {
  data: AgentDelegation[];
}

/**
 * CIMD metadata included in the agent detail response when the authorization
 * request originates from a Client ID Metadata Document URL (client_id).
 * Null/absent for opaque UUID-based client_id values.
 */
export interface CIMDMetadata {
  /** The CIMD URL used as client_id */
  client_id_url: string;
  /** The requested redirect_uri */
  redirect_uri: string;
  /** Hostname from client_id_url, pre-registered and verified */
  verified_domain: string;
  /** OAuth2 scopes requested by this authorization */
  requested_scopes: string[];
  /** Logo URL from the CIMD document, if present */
  logo_uri?: string;
}

/**
 * Response from GET /api/consent/agents/{id} (unified)
 */
export interface GetAgentDetailResponse {
  data: {
    agent: {
      agentId: string;
      client_id?: string;
      client_uris?: string[];
      display_name: string;
      description: string;
      governance_url?: string;
      user_documentation_url?: string;
      agent_interface_url?: string;
      created_at: string;
      updated_at: string;
    };
    services: Array<Omit<ServiceRequirement, 'kind'>>;
    permission_sets?: ResolvedPermissionSetEntry[];
    active_session_service_ids?: string[];
    service_requirements?: Array<{
      service_id: string;
      requirement_type: 'mandatory' | 'optional';
    }>;
    cimd_metadata?: CIMDMetadata | null;
  };
}

/**
 * Response from GET /api/consent/agents/:agent-id/grants
 * Returns a single grant (or null if no grant exists) due to 1:1 relationship per (principal, agent_id)
 */
export interface GetAgentGrantsResponse {
  data: UserGrant | null;
}

/**
 * Request body for POST /api/consent/agents/{id}/grants (spec 019)
 */
export interface CreateOrUpdateGrantRequest {
  /** Granted permission sets: map of PS ID → included service IDs */
  granted_permission_sets: Record<string, string[]>;

  /** Optional expiration timestamp (omit for indefinite) */
  valid_until?: string | null;
}

/**
 * Response from POST /api/consent/agents/:agent-id/grants
 */
export interface CreateOrUpdateGrantResponse {
  data: UserGrant;
  redirect_url?: string;
}

export type GrantResult =
  | { kind: 'created'; grant: UserGrant }
  | { kind: 'redirect'; redirectUrl: string };

/**
 * Standard error response from backend APIs.
 */
export interface ApiError {
  /** HTTP status code */
  status: number;

  /** Error code (e.g., "INVALID_AGENT_ID") */
  code: string;

  /** Human-readable error message */
  message: string;

  /** Optional field-level validation errors */
  details?: Record<string, string[]>;
}

/**
 * Service requirement for an agent (Phase 6).
 * Specifies which services an agent needs access to and the permissions disclosed for consent.
 */
export interface ServiceRequirement {
  kind: 'requirement';
  serviceId: string;
  serviceName: string;
  requirementType: 'mandatory' | 'optional';
  /**
   * Explicit requirements expose their configured scope ceiling. All-scope requirements expose
   * the sorted, deduplicated Permission Set union for this service.
   */
  requiredScopes: Array<{
    name: string;
    description?: string;
  }>;
  connectionStatus: 'connected' | 'not_connected';
  logoUrl?: string;
}

