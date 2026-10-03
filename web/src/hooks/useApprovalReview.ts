import { useRef } from 'react';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { approvalApi } from '@services/api/approvals';
import { getAuthGeneration } from '@services/api/client';
import { usePrincipal } from '@services/query/QueryProvider';
import { queryKeys } from '@services/query/queryKeys';
import type { ApiError, UserInfo } from '../types/consent';
import type { ApprovalErrorCode, ApproveRequest, ApproveResponseData, DenyResponseData, ToolApprovalDetail } from '../types/approval';

type Decision = { id: string; principal: string; generation: number } & (
  | { action: 'approve'; request: ApproveRequest }
  | { action: 'deny'; permanent: boolean }
);
type DecisionResult = { action: 'approve'; data: ApproveResponseData } | { action: 'deny'; data: DenyResponseData };

export interface ApprovalActions {
  submitting: boolean;
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
  refetch: () => Promise<void>;
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

/** Shared by focused review and inline queue decisions; never starts a detail poll. */
export function useApprovalActions(id: string): ApprovalActions {
  const { principal } = usePrincipal();
  const client = useQueryClient();
  const inFlight = useRef<Decision | null>(null);
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
    if (inFlight.current?.id === id && inFlight.current.principal === principal) return;
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
    submitting: belongsToRoute && mutation.isPending,
    errorCode: error ? approvalErrorCode(error) : null,
    errorMessage: error?.message ?? null,
    approveResult: result?.action === 'approve' ? result.data : null,
    denyResult: result?.action === 'deny' ? result.data : null,
    approve: (request) => decide({ id, principal, generation: getAuthGeneration(), action: 'approve', request }),
    deny: (permanent = false) => decide({ id, principal, generation: getAuthGeneration(), action: 'deny', permanent }),
  };
}

export function useApprovalReview(id: string): ApprovalReview {
  const { principal } = usePrincipal();
  const query = useQuery<ToolApprovalDetail, ApiError>({
    queryKey: queryKeys.approval(principal, id),
    queryFn: ({ signal }) => approvalApi.getApproval(id, { signal }),
    enabled: Boolean(id),
  });
  const actions = useApprovalActions(id);
  return {
    ...actions,
    approval: query.data ?? null,
    loading: query.isPending,
    errorCode: actions.errorCode ?? (query.error ? approvalErrorCode(query.error) : null),
    errorMessage: actions.errorMessage ?? query.error?.message ?? null,
    refetch: async () => { await query.refetch(); },
  };
}
