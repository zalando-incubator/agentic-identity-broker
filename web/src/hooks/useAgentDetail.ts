import { useQuery } from '@tanstack/react-query';
import { consentApi } from '@services/api/consent';
import { usePrincipal } from '@services/query/QueryProvider';
import { queryKeys } from '@services/query/queryKeys';
import { useAgentGrant } from './useAgentGrant';

export function useAgentDetail(agentId: string) {
  const { principal } = usePrincipal();
  const detail = useQuery({
    queryKey: queryKeys.agent(principal, agentId),
    queryFn: ({ signal }) => consentApi.getAgentDetail(agentId, { signal }),
    enabled: Boolean(agentId),
  });
  const grant = useAgentGrant(agentId);
  return { ...detail, grant, loading: detail.isPending || grant.isPending, error: detail.error ?? grant.error };
}
