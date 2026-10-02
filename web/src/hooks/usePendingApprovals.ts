import { createContext, createElement, useContext, type ReactNode } from 'react';
import { useQuery, type UseQueryResult } from '@tanstack/react-query';
import { approvalApi } from '@services/api/approvals';
import { usePrincipal } from '@services/query/QueryProvider';
import { queryKeys } from '@services/query/queryKeys';
import type { ToolApprovalDetail } from '../types/approval';

export type PendingApprovalsState = UseQueryResult<ToolApprovalDetail[]> & {
  count: number | undefined;
  stale: boolean;
};

const PendingApprovalsContext = createContext<PendingApprovalsState | null>(null);

/** Mount once in the console, never in a decision route. */
export function PendingApprovalsProvider({ children }: { children: ReactNode }) {
  const { principal } = usePrincipal();
  const query = useQuery({
    queryKey: queryKeys.pending(principal),
    queryFn: ({ signal }) => approvalApi.listPendingApprovals({ signal }),
    refetchInterval: 10_000,
    refetchIntervalInBackground: false,
    refetchOnWindowFocus: true,
  });
  const value: PendingApprovalsState = {
    ...query,
    count: query.data?.length,
    stale: query.isError || query.failureCount > 0,
  };
  return createElement(PendingApprovalsContext.Provider, { value }, children);
}

export function usePendingApprovals(): PendingApprovalsState {
  const state = useContext(PendingApprovalsContext);
  if (!state) throw new Error('PendingApprovalsProvider is required in the console');
  return state;
}
