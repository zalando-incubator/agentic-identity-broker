/**
 * API service for tool approval endpoints.
 *
 * Provides type-safe methods for interacting with the approval backend APIs.
 */

import { apiClient } from './client';
import type {
  ApprovalDetailResponse,
  ApproveRequest,
  ApproveResponse,
  ApproveResponseData,
  DenyRequest,
  DenyResponse,
  DenyResponseData,
  ScopePreview,
  ScopePreviewRequest,
  ToolApprovalDetail,
} from '../../types/approval';

export const approvalApi = {
  async getApproval(approvalId: string): Promise<ToolApprovalDetail> {
    const response = await apiClient.get<ApprovalDetailResponse>(
      `/approvals/${encodeURIComponent(approvalId)}`,
    );
    return response.data.data;
  },

  async approveApproval(
    approvalId: string,
    request: ApproveRequest,
  ): Promise<ApproveResponseData> {
    const response = await apiClient.post<ApproveResponse>(
      `/approvals/${encodeURIComponent(approvalId)}/approve`,
      request,
    );
    return response.data.data;
  },

  async previewApprovalScope(
    approvalId: string,
    request: ScopePreviewRequest,
  ): Promise<ScopePreview> {
    const response = await apiClient.post<{ data: ScopePreview }>(
      `/approvals/${encodeURIComponent(approvalId)}/scope-preview`,
      request,
    );
    return response.data.data;
  },

  async denyApproval(
    approvalId: string,
    request?: DenyRequest,
  ): Promise<DenyResponseData> {
    const response = await apiClient.post<DenyResponse>(
      `/approvals/${encodeURIComponent(approvalId)}/deny`,
      request ?? {},
    );
    return response.data.data;
  },

  async listPendingApprovals(): Promise<ToolApprovalDetail[]> {
    const response = await apiClient.get<{ data: ToolApprovalDetail[] }>(
      '/approvals/pending',
    );
    return response.data.data ?? [];
  },

  async listPermanentApprovals(): Promise<ToolApprovalDetail[]> {
    const response = await apiClient.get<{ data: ToolApprovalDetail[] }>(
      '/approvals/permanent',
    );
    return response.data.data ?? [];
  },

  async revokePermanentApproval(
    approvalId: string,
  ): Promise<{ id: string; status: string; denied_at: string }> {
    const response = await apiClient.post<{
      data: { id: string; status: string; denied_at: string };
    }>(`/approvals/${encodeURIComponent(approvalId)}/revoke`);
    return response.data.data;
  },
};
