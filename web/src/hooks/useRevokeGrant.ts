import { consentApi } from '@services/api/consent';
import { usePrincipal } from '@services/query/QueryProvider';
import { queryKeys } from '@services/query/queryKeys';
import type { AgentDelegation } from '../types/consent';
import { useOptimisticRevoke } from './optimisticRevoke';

export type RevokeAgentTarget = Pick<AgentDelegation, 'agentId' | 'displayName'>;
export interface RevokeGrantOptions {
  onSuccess?: (agentId: string) => void;
  onError?: (error: unknown, agentId: string) => void;
}

/** Share confirmation and pending-record isolation with other console revocations. */
export function useRevokeGrant(options: RevokeGrantOptions = {}) {
  const { principal } = usePrincipal();
  return useOptimisticRevoke<RevokeAgentTarget>({
    listKey: queryKeys.delegations(principal),
    detailKeys: (agentId) => [queryKeys.grant(principal, agentId), queryKeys.agent(principal, agentId)],
    recordId: (delegation) => delegation.agentId,
    mutationFn: (agentId) => consentApi.deleteGrant(agentId),
    onSuccess: options.onSuccess,
    onError: options.onError,
  });
}
