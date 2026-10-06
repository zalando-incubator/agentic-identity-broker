import { useCallback, useRef } from 'react';
import { useIsMutating, useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { approvalApi } from '@services/api/approvals';
import { getAuthGeneration } from '@services/api/client';
import { usePrincipal } from '@services/query/QueryProvider';
import { queryKeys } from '@services/query/queryKeys';
import type { ApiError, UserInfo } from '../types/consent';
import type { ApprovalErrorCode, ApproveRequest, ApproveResponseData, DenyResponseData, ScopePreview, ToolApprovalDetail } from '../types/approval';

type Decision = { id: string; principal: string; generation: number } & (
  | { action: 'approve'; request: ApproveRequest }
  | { action: 'deny'; permanent: boolean }
);
type DecisionResult = { action: 'approve'; data: ApproveResponseData } | { action: 'deny'; data: DenyResponseData };

export interface ApprovalActions {
  submitting: boolean;
  submittingAction: 'approve' | 'deny' | null;
  errorCode: ApprovalErrorCode | null;
  errorMessage: string | null;
  approveResult: ApproveResponseData | null;
  denyResult: DenyResponseData | null;
  approve: (request: ApproveRequest) => Promise<void>;
  deny: (permanent?: boolean) => Promise<void>;
}
export interface ApprovalReview extends ApprovalActions {
  approval: ToolApprovalDetail | null;
  loading: boolean;
  onPreview: (paramsPattern: Record<string, string>, signal: AbortSignal) => Promise<ScopePreview>;
  refetch: () => Promise<void>;
}

interface ReviewOptions {
  onDecisionSuccess?: (action: 'approve' | 'deny', id: string) => void;
}

function approvalErrorCode(error: ApiError): ApprovalErrorCode {
  if (error.code === 'GONE' || error.status === 410) return 'EXPIRED';
  if (error.code === 'CONFLICT' || error.status === 409) return 'ALREADY_ACTIONED';
  if (error.status === 403) return 'FORBIDDEN';
  if (error.status === 404) return 'NOT_FOUND';
  if (error.code === 'invalid_pattern') return 'INVALID_PATTERN';
  if (!error.status) return 'NETWORK_ERROR';
  return 'SERVER_ERROR';
}

/** Shared by focused review and inbox decisions; never starts a detail poll. */
export function useApprovalActions(id: string, options?: ReviewOptions): ApprovalActions {
  const { principal } = usePrincipal();
  const client = useQueryClient();
  const pendingDecisions = useIsMutating({ mutationKey: [...queryKeys.principal(principal), 'approval'] });
  const inFlight = useRef<Decision | null>(null);
  const onDecisionSuccess = useRef(options?.onDecisionSuccess);
  onDecisionSuccess.current = options?.onDecisionSuccess;
  const ownsDecision = (decision: Decision) =>
    decision.generation === getAuthGeneration() &&
    client.getQueryData<UserInfo>(['identity'])?.principal === decision.principal;
  const mutation = useMutation<DecisionResult, ApiError, Decision>({
    mutationKey: [...queryKeys.approval(principal, id), 'decision'],
    retry: false,
    mutationFn: async (decision) => decision.action === 'approve'
      ? { action: 'approve', data: await approvalApi.approveApproval(decision.id, decision.request) }
      : { action: 'deny', data: await approvalApi.denyApproval(decision.id, decision.permanent ? { persistence: 'permanent' } : {}) },
    onSuccess: async (result, decision) => {
      if (!ownsDecision(decision)) return;
      const queryKey = queryKeys.approval(decision.principal, decision.id);
      await client.cancelQueries({ queryKey, exact: true });
      if (!ownsDecision(decision)) return;
      client.setQueryData<ToolApprovalDetail>(queryKey, (previous) => previous ? { ...previous, ...result.data } : undefined);
      const pendingKey = queryKeys.pending(decision.principal);
      await client.cancelQueries({ queryKey: pendingKey, exact: true });
      if (!ownsDecision(decision)) return;
      client.setQueryData<ToolApprovalDetail[]>(pendingKey, (previous) => previous?.filter((item) => item.id !== decision.id));
      onDecisionSuccess.current?.(result.action, decision.id);
    },
    onError: async (error, decision) => {
      const code = approvalErrorCode(error);
      if (code !== 'ALREADY_ACTIONED' && code !== 'EXPIRED') return;
      if (!ownsDecision(decision)) return;
      const queryKey = queryKeys.approval(decision.principal, decision.id);
      await client.cancelQueries({ queryKey, exact: true });
      if (!ownsDecision(decision)) return;
      await client.invalidateQueries({ queryKey, exact: true, refetchType: 'none' });
      if (!ownsDecision(decision)) return;
      try {
        await client.fetchQuery({
          queryKey,
          queryFn: ({ signal }) => approvalApi.getApproval(decision.id, { signal }),
          retry: false,
        });
      } catch {
        // Keep the conflict outcome when the authoritative read is unavailable.
      }
    },
    onSettled: async (_result, _error, decision) => {
      if (!ownsDecision(decision)) return;
      // The principal-keyed mutation cache refreshes pending approvals on every settlement.
      await client.invalidateQueries({ queryKey: queryKeys.standing(decision.principal) });
    },
  });

  const belongsToRoute = mutation.variables?.id === id && mutation.variables.principal === principal && mutation.variables.generation === getAuthGeneration();
  const error = belongsToRoute ? mutation.error : null;
  const result = belongsToRoute ? mutation.data : undefined;

  async function decide(decision: Decision) {
    if (inFlight.current) return;
    inFlight.current = decision;
    try {
      await mutation.mutateAsync(decision);
    } catch {
      // Consumers render the server error; a failed request never becomes success.
    } finally {
      if (inFlight.current === decision) inFlight.current = null;
    }
  }

  return {
    submitting: pendingDecisions > 0,
    submittingAction: belongsToRoute && mutation.isPending ? mutation.variables?.action ?? null : null,
    errorCode: error ? approvalErrorCode(error) : null,
    errorMessage: error?.message ?? null,
    approveResult: result?.action === 'approve' ? result.data : null,
    denyResult: result?.action === 'deny' ? result.data : null,
    approve: (request) => decide({ id, principal, generation: getAuthGeneration(), action: 'approve', request }),
    deny: (permanent = false) => decide({ id, principal, generation: getAuthGeneration(), action: 'deny', permanent }),
  };
}

export function useApprovalReview(id: string, options?: ReviewOptions): ApprovalReview {
  const { principal } = usePrincipal();
  const query = useQuery<ToolApprovalDetail, ApiError>({
    queryKey: queryKeys.approval(principal, id),
    queryFn: ({ signal }) => approvalApi.getApproval(id, { signal }),
    enabled: Boolean(id),
  });
  const actions = useApprovalActions(id, options);
  const onPreview = useCallback((paramsPattern: Record<string, string>, signal: AbortSignal) =>
    approvalApi.previewApprovalScope(id, { params_pattern: paramsPattern }, { signal }), [id]);
  return {
    ...actions,
    approval: query.data?.id === id && query.data.principal === principal ? query.data : null,
    loading: query.isPending,
    errorCode: actions.errorCode ?? (query.error ? approvalErrorCode(query.error) : null),
    onPreview,
    errorMessage: actions.errorMessage ?? query.error?.message ?? null,
    refetch: async () => { await query.refetch(); },
  };
}
