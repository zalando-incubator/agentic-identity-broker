/**
 * TypeScript types for tool approval feature.
 * Maps to backend API schemas from openapi.yaml.
 */

/** Approval lifecycle status */
export type ApprovalStatus = 'pending' | 'approved' | 'denied';

/** Persistence scope for approval decisions */
export type ApprovalPersistence = 'once' | 'session' | 'permanent';

/** Risk level for tool calls */
export type RiskLevel = 'low' | 'medium' | 'critical' | (string & {});

/**
 * Detailed tool approval entity.
 * Returned by GET /api/approvals/:id
 */
export interface ToolApprovalDetail {
  id: string;
  principal: string;
  agent_id: string;
  agent_display_name?: string;
  mcp_session_id?: string | null;
  agent_session_id?: string | null;
  tool_invocation_id?: string | null;
  tool_name: string;
  arguments: Record<string, unknown>;
  tool_pattern: string;
  params_pattern: Record<string, string>;
  description?: string;
  risk_level?: RiskLevel;
  status: ApprovalStatus;
  persistence?: ApprovalPersistence | null;
  consumed?: boolean;
  approval_url: string;
  created_at: string;
  approved_at?: string | null;
  denied_at?: string | null;
  consumed_at?: string | null;
  expires_at: string;
}

/** Response wrapper for GET /api/approvals/:id */
export interface ApprovalDetailResponse {
  data: ToolApprovalDetail;
}

/** Request body for POST /api/approvals/:id/approve */
export interface ApproveRequest {
  persistence: ApprovalPersistence;
  tool_pattern?: string;
  params_pattern?: Record<string, string>;
}

/** Response data for POST /api/approvals/:id/approve */
export interface ApproveResponseData {
  id: string;
  status: 'approved';
  persistence: ApprovalPersistence;
  approved_at: string;
}

/** Response wrapper for POST /api/approvals/:id/approve */
export interface ApproveResponse {
  data: ApproveResponseData;
}

/** Request body for POST /api/approvals/:id/deny */
export interface DenyRequest {
  persistence?: 'permanent';
}

/** Response data for POST /api/approvals/:id/deny */
export interface DenyResponseData {
  id: string;
  status: 'denied';
  persistence?: 'permanent' | null;
  denied_at: string;
}

/** Response wrapper for POST /api/approvals/:id/deny */
export interface DenyResponse {
  data: DenyResponseData;
}

/** Error codes specific to approval operations */
export type ApprovalErrorCode =
  | 'EXPIRED'
  | 'ALREADY_ACTIONED'
  | 'FORBIDDEN'
  | 'NOT_FOUND'
  | 'NETWORK_ERROR'
  | 'SERVER_ERROR'
  | 'INVALID_PATTERN';
