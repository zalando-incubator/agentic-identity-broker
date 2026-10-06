import { useRef } from 'react';
import { useSearchParams } from 'react-router-dom';
import { toast } from 'sonner';
import { delegationsCopy } from '@copy/delegations';
import { useCollectionView } from '@hooks/useCollectionView';
import { useDelegations } from '@hooks/useDelegations';
import { useRevokeGrant } from '@hooks/useRevokeGrant';
import type { AgentDelegation } from '../types/consent';
import { AgentsView, type AgentSort } from './AgentsView';

export function DelegationsPage() {
  const { delegations, loading, error, refetch } = useDelegations();
  const [params, setParams] = useSearchParams();
  const search = params.get('q') ?? '';
  const sortParam = params.get('sort');
  const sort: AgentSort = sortParam === 'recent' || sortParam === 'expiring' ? sortParam : 'name';
  const { view, setView } = useCollectionView('agents', delegations.length);
  const heading = useRef<HTMLDivElement>(null);
  const revokeOpener = useRef<HTMLButtonElement | null>(null);
  const revoke = useRevokeGrant({
    onSuccess: () => {
      toast.success(delegationsCopy.revokeSuccess);
      if (document.activeElement === document.body) heading.current?.focus();
    },
    onError: () => toast.error(delegationsCopy.revokeFailure),
  });

  const updateParam = (key: 'q' | 'sort', value: string) => setParams(current => {
    const next = new URLSearchParams(current);
    if (value && !(key === 'sort' && value === 'name')) next.set(key, value);
    else next.delete(key);
    return next;
  }, { replace: true });

  return <AgentsView
    delegations={delegations} loading={loading} error={Boolean(error)} onRetry={() => { void refetch(); }}
    search={search} onSearchChange={value => updateParam('q', value)} sort={sort} onSortChange={value => updateParam('sort', value)}
    view={view} onViewChange={setView} headingRef={heading}
    onRevoke={(record: AgentDelegation) => {
      revokeOpener.current = document.activeElement instanceof HTMLButtonElement ? document.activeElement : null;
      revoke.requestRevoke(record);
    }}
    isPending={revoke.isPending} confirmation={revoke.confirmation} onCancelRevoke={revoke.cancelRevoke}
    onConfirmRevoke={revoke.confirmRevoke} onReturnFocus={() => {
      const target = revokeOpener.current;
      if (target?.isConnected && !target.disabled) target.focus();
      else heading.current?.focus();
    }}
  />;
}

export default DelegationsPage;
