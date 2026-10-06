import { useLayoutEffect, useRef } from 'react';
import { CancelledError, useMutation, useQuery, useQueryClient, type MutateOptions } from '@tanstack/react-query';
import { consentApi } from '@services/api/consent';
import { getAuthGeneration } from '@services/api/client';
import { usePrincipal } from '@services/query/QueryProvider';
import { queryKeys } from '@services/query/queryKeys';
import type { CreateOrUpdateGrantRequest, GrantResult } from '../types/consent';

export function useAgentGrant(agentId: string) {
  const { principal } = usePrincipal();
  return useQuery({
    queryKey: queryKeys.grant(principal, agentId),
    queryFn: ({ signal }) => consentApi.getAgentGrants(agentId, { signal }),
    select: grants => grants.find(grant => grant.agent_id === agentId && grant.principal === principal) ?? null,
    enabled: Boolean(agentId),
  });
}

interface GrantSaveAuthority {
  generation: number;
  routeLifetime: number;
}

/** Saving consent is server-authoritative, including its validated continuation. */
export function useSaveGrant(agentId: string, options?: { sessionToken?: string }) {
  const { principal } = usePrincipal();
  const client = useQueryClient();
  const lifetime = useRef({ active: false, generation: getAuthGeneration(), routeLifetime: 0 });
  useLayoutEffect(() => {
    const current = lifetime.current;
    current.active = true;
    return () => {
      current.active = false;
      current.routeLifetime += 1;
    };
  }, []);

  // Pages also call this immediately after await, before navigation or success feedback.
  function assertCurrent(authority?: GrantSaveAuthority) {
    const current = lifetime.current;
    if (!current.active || current.generation !== getAuthGeneration()
      || (authority && (authority.generation !== current.generation || authority.routeLifetime !== current.routeLifetime))) {
      throw new CancelledError({ silent: true });
    }
  }

  const mutation = useMutation<GrantResult, Error, CreateOrUpdateGrantRequest, GrantSaveAuthority>({
    mutationKey: [...queryKeys.grant(principal, agentId), 'save'],
    onMutate: () => {
      assertCurrent();
      return { generation: lifetime.current.generation, routeLifetime: lifetime.current.routeLifetime };
    },
    mutationFn: (request) => {
      assertCurrent();
      return consentApi.createOrUpdateGrant(agentId, request, options);
    },
    retry: false,
    onSuccess: async (_result, _request, authority) => {
      assertCurrent(authority);
      const keys = [
        queryKeys.grant(principal, agentId),
        queryKeys.delegations(principal),
        queryKeys.agent(principal, agentId),
      ];
      await Promise.all([
        ...keys.map((queryKey) => client.invalidateQueries({ queryKey })),
        ...(options?.sessionToken ? [client.invalidateQueries({
          queryKey: queryKeys.decision(principal, agentId, options.sessionToken),
        })] : []),
      ]);
      assertCurrent(authority);
    },
  });

  async function mutateAsync(
    request: CreateOrUpdateGrantRequest,
    callbacks?: MutateOptions<GrantResult, Error, CreateOrUpdateGrantRequest, GrantSaveAuthority>,
  ) {
    assertCurrent();
    const authority = { generation: lifetime.current.generation, routeLifetime: lifetime.current.routeLifetime };
    try {
      const result = await mutation.mutateAsync(request, callbacks);
      assertCurrent(authority);
      return result;
    } catch (error) {
      assertCurrent(authority);
      throw error;
    }
  }

  return { ...mutation, mutateAsync, assertCurrent };
}
