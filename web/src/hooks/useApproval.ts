/**
 * useApproval hook manages fetching and state for a single tool approval.
 *
 * Features:
 * - Fetch approval by ID
 * - Loading and error states
 * - Approve/deny actions with optimistic state updates
 * - Refetch capability
 */

import { useState, useEffect, useCallback } from 'react';
import { approvalApi } from '@services/api/approvals';
import type {
  ToolApprovalDetail,
  ApprovalErrorCode,
  ApproveResponseData,
  DenyResponseData,
  ApproveRequest,
} from '../types/approval';
import type { ApiError } from '../types/consent';

interface UseApprovalResult {
  /** Approval data */
  approval: ToolApprovalDetail | null;
  /** Loading state for initial fetch */
  loading: boolean;
  /** Submitting state for approve/deny actions */
  submitting: boolean;
  /** Error code for display */
  errorCode: ApprovalErrorCode | null;
  /** Raw error message */
  errorMessage: string | null;
  /** Approve response data after successful approval */
  approveResult: ApproveResponseData | null;
  /** Deny response data after successful denial */
  denyResult: DenyResponseData | null;
  /** Approve the approval with the selected coverage. */
  approve: (request: ApproveRequest) => Promise<void>;
  /** Deny the approval with optional permanent persistence */
  deny: (permanent?: boolean) => Promise<void>;
  /** Refetch the approval data */
  refetch: () => Promise<void>;
}

function mapHttpStatusToErrorCode(
  status: number | undefined,
  code?: string,
): ApprovalErrorCode {
  if (code === 'GONE' || status === 410) return 'EXPIRED';
  if (code === 'CONFLICT' || status === 409) return 'ALREADY_ACTIONED';
  if (status === 403) return 'FORBIDDEN';
  if (status === 404) return 'NOT_FOUND';
  if (!status || status === 0) return 'NETWORK_ERROR';
  if (code === 'invalid_pattern' || status === 422) return 'INVALID_PATTERN';
  return 'SERVER_ERROR';
}

/**
 * Custom hook to fetch and manage a single tool approval.
 *
 * @param approvalId - UUID of the approval to fetch
 * @returns Approval data, loading/error states, and action functions
 */
export function useApproval(approvalId: string): UseApprovalResult {
  const [approval, setApproval] = useState<ToolApprovalDetail | null>(null);
  const [loading, setLoading] = useState(true);
  const [submitting, setSubmitting] = useState(false);
  const [errorCode, setErrorCode] = useState<ApprovalErrorCode | null>(null);
  const [errorMessage, setErrorMessage] = useState<string | null>(null);
  const [approveResult, setApproveResult] =
    useState<ApproveResponseData | null>(null);
  const [denyResult, setDenyResult] = useState<DenyResponseData | null>(null);

  const fetchApproval = useCallback(async () => {
    if (!approvalId) return;
    try {
      setLoading(true);
      setErrorCode(null);
      setErrorMessage(null);
      const data = await approvalApi.getApproval(approvalId);
      setApproval(data);
    } catch (err) {
      const apiErr = err as ApiError;
      setErrorCode(mapHttpStatusToErrorCode(apiErr.status, apiErr.code));
      setErrorMessage(apiErr.message || 'Failed to load approval');
    } finally {
      setLoading(false);
    }
  }, [approvalId]);

  const approve = useCallback(
    async (request: ApproveRequest) => {
      if (!approvalId) return;
      try {
        setSubmitting(true);
        setErrorCode(null);
        setErrorMessage(null);
        const result = await approvalApi.approveApproval(approvalId, request);
        setApproveResult(result);
        if (approval) {
          setApproval({ ...approval, status: 'approved', persistence: request.persistence, approved_at: result.approved_at });
        }
      } catch (err) {
        const apiErr = err as ApiError;
        setErrorCode(mapHttpStatusToErrorCode(apiErr.status, apiErr.code));
        setErrorMessage(apiErr.message || 'Failed to approve');
      } finally {
        setSubmitting(false);
      }
    },
    [approvalId, approval],
  );

  const deny = useCallback(
    async (permanent?: boolean) => {
      if (!approvalId) return;
      try {
        setSubmitting(true);
        setErrorCode(null);
        setErrorMessage(null);
        const request = permanent ? { persistence: 'permanent' as const } : {};
        const result = await approvalApi.denyApproval(approvalId, request);
        setDenyResult(result);
        if (approval) {
          setApproval({
            ...approval,
            status: 'denied',
            persistence: permanent ? 'permanent' : approval.persistence,
            denied_at: result.denied_at,
          });
        }
      } catch (err) {
        const apiErr = err as ApiError;
        setErrorCode(mapHttpStatusToErrorCode(apiErr.status, apiErr.code));
        setErrorMessage(apiErr.message || 'Failed to deny');
      } finally {
        setSubmitting(false);
      }
    },
    [approvalId, approval],
  );

  useEffect(() => {
    void fetchApproval();
  }, [fetchApproval]);

  return {
    approval,
    loading,
    submitting,
    errorCode,
    errorMessage,
    approveResult,
    denyResult,
    approve,
    deny,
    refetch: fetchApproval,
  };
}
