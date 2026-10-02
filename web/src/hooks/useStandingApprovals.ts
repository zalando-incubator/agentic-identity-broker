import { useQuery } from '@tanstack/react-query';
import { approvalApi } from '@services/api/approvals';
import { usePrincipal } from '@services/query/QueryProvider';
import { queryKeys } from '@services/query/queryKeys';
import type { ToolApprovalDetail } from '../types/approval';
import { useOptimisticRevoke } from './optimisticRevoke';

export function useStandingApprovals(options?: { onRevokeSuccess?: (id: string) => void }) {
  const { principal } = usePrincipal();
  const query = useQuery({
    queryKey: queryKeys.standing(principal),
    queryFn: ({ signal }) => approvalApi.listPermanentApprovals({ signal }),
  });
  const revoke = useOptimisticRevoke<ToolApprovalDetail>({
    listKey: queryKeys.standing(principal),
    detailKeys: id => [queryKeys.approval(principal, id)],
    recordId: record => record.id,
    mutationFn: id => approvalApi.revokePermanentApproval(id),
    onSuccess: options?.onRevokeSuccess,
  });
  return { ...query, stale: query.isError || query.failureCount > 0, revoke };
}
