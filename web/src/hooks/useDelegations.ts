import { useEffect, useMemo, useState } from 'react';
import { useQuery } from '@tanstack/react-query';
import { consentApi } from '@services/api/consent';
import { usePrincipal } from '@services/query/QueryProvider';
import { queryKeys } from '@services/query/queryKeys';

// Browsers overflow larger delays into near-immediate callbacks.
const MAX_TIMEOUT_MS = 2_147_483_647;

export function useDelegations() {
  const { principal } = usePrincipal();
  const query = useQuery({
    queryKey: queryKeys.delegations(principal),
    queryFn: ({ signal }) => consentApi.getAgentDelegations({ signal }),
  });
  const [clock, setClock] = useState(Date.now);
  const delegations = useMemo(() => {
    const now = Math.max(clock, Date.now());
    return (query.data ?? []).filter((record) =>
      !record.expiresAt || Date.parse(record.expiresAt) > now,
    );
  }, [query.data, clock]);

  useEffect(() => {
    const now = Math.max(clock, Date.now());
    let nextDeadline = Infinity;
    for (const record of query.data ?? []) {
      if (!record.expiresAt) continue;
      const deadline = Date.parse(record.expiresAt);
      if (deadline > now && deadline < nextDeadline) nextDeadline = deadline;
    }
    if (!Number.isFinite(nextDeadline)) return;
    const timer = window.setTimeout(() => setClock(Date.now()), Math.min(MAX_TIMEOUT_MS, Math.max(0, nextDeadline - Date.now())));
    return () => window.clearTimeout(timer);
  }, [query.data, clock]);

  return { delegations, loading: query.isPending, error: query.error, refetch: query.refetch };
}
