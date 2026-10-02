import { useRef, useState } from 'react';
import { toast } from 'sonner';
import { commonCopy, navigationCopy } from '@copy';
import { delegationsCopy } from '@copy/delegations';
import { useDelegations } from '@hooks/useDelegations';
import { useRevokeGrant } from '@hooks/useRevokeGrant';
import { DelegationsTable } from '@components/delegations/DelegationsTable';
import { RevokeAgentDialog } from '@components/consent/RevokeAgentDialog';
import { PageHeader } from '@design-system/components/layout/PageHeader';
import { Input } from '@design-system/components/inputs/Input';
import { Button } from '@design-system/components/primitives/Button';
import { Wordmark } from '@design-system/components/primitives/Wordmark';

export function DelegationsPage() {
  const { delegations, loading, error, refetch } = useDelegations();
  const [search, setSearch] = useState('');
  const searchInput = useRef<HTMLInputElement>(null);
  const revokeOpener = useRef<HTMLButtonElement | null>(null);
  const revoke = useRevokeGrant({
    onSuccess: () => {
      toast.success(delegationsCopy.revokeSuccess);
      // A successful revoke removes the focused row; never strand focus on body.
      if (document.activeElement === document.body) searchInput.current?.focus();
    },
    onError: () => toast.error(delegationsCopy.revokeFailure),
  });

  return <>
    <PageHeader title={navigationCopy.agents} purpose={delegationsCopy.purpose} />
    {loading ? <p role="status" className="text-muted-foreground">{commonCopy.loading}</p> : <>
      {error && <div role="alert" className="mb-4 space-y-2 text-destructive">
        <p>{delegations.length ? delegationsCopy.stale : delegationsCopy.loadFailure}</p>
        <Button variant="outline" onClick={() => { void refetch(); }}>{commonCopy.retry}</Button>
      </div>}
      {!error || delegations.length > 0 ? <div className="mb-4 max-w-md"><Input ref={searchInput} type="search" label={delegationsCopy.search} value={search} onChange={(event) => setSearch(event.target.value)} /></div> : null}
      {delegations.length > 0 ? <div>
        <DelegationsTable delegations={delegations} search={search} isPending={revoke.isPending} onRevoke={(record) => {
          revokeOpener.current = document.activeElement instanceof HTMLButtonElement ? document.activeElement : null;
          revoke.requestRevoke(record);
        }} />
      </div> : !error && <div data-testid="delegations-empty-state" className="space-y-4 rounded-lg border border-border bg-card p-6 text-card-foreground">
        <Wordmark label={commonCopy.brand} />
        <p className="text-sm text-muted-foreground">{delegationsCopy.empty}</p>
      </div>}
    </>}
    <RevokeAgentDialog open={revoke.confirmation !== null} agentName={revoke.confirmation?.displayName ?? ''} onCancel={revoke.cancelRevoke} onConfirm={revoke.confirmRevoke} onReturnFocus={() => {
      const target = revokeOpener.current;
      if (target?.isConnected && !target.disabled) target.focus();
      else searchInput.current?.focus();
    }} />
  </>;
}

export default DelegationsPage;
