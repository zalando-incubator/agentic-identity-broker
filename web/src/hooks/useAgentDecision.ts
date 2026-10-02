import { useQuery } from '@tanstack/react-query';
import { consentApi } from '@services/api/consent';
import { usePrincipal } from '@services/query/QueryProvider';
import { queryKeys } from '@services/query/queryKeys';
import { useAgentGrant } from './useAgentGrant';

export function useAgentDecision(agentId: string, sessionToken: string) {
  const { principal } = usePrincipal();
  const detail = useQuery({
    queryKey: queryKeys.decision(principal, agentId, sessionToken),
    queryFn: ({ signal }) => consentApi.getAgentDetail(agentId, { sessionToken, signal }),
    enabled: Boolean(agentId && sessionToken),
    staleTime: 0,
    gcTime: 0,
    retry: false,
    refetchOnWindowFocus: false,
  });
  const grant = useAgentGrant(sessionToken ? agentId : '');
  return { ...detail, grant, loading: detail.isPending || grant.isPending, error: detail.error ?? grant.error };
}
