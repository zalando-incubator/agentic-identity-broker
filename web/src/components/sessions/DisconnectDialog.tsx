import { useRef } from 'react';
import { useQuery } from '@tanstack/react-query';
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from '@design-system/components/overlays/Dialog';
import { TruncatedText } from '@design-system/components/data-display/TruncatedText';
import { Button } from '@design-system/components/primitives/Button';
import { accessCopy, commonCopy } from '@copy';
import { connectionsCopy } from '@copy/connections';
import { sessionsApi, type AffectedAgent, type SessionSummary } from '@services/api/sessions';
import { usePrincipal } from '@services/query/QueryProvider';
import { queryKeys } from '@services/query/queryKeys';

export interface DisconnectDialogProps {
  session: SessionSummary | null;
  onCancel: () => void;
  onConfirm: () => void;
  onReturnFocus?: () => void;
}

export interface DisconnectDialogViewProps extends DisconnectDialogProps {
  affectedAgents: AffectedAgent[] | null;
  loading: boolean;
  error: boolean;
  onRetry: () => void;
}

/** Pure dialog surface; confirmation requires a fresh successful dependent-agent read. */
export function DisconnectDialogView({ session, affectedAgents, loading, error, onCancel, onConfirm, onReturnFocus, onRetry }: DisconnectDialogViewProps) {
  const cancelRef = useRef<HTMLButtonElement>(null);
  return <Dialog open={session !== null} onOpenChange={open => { if (!open) onCancel(); }}>
    <DialogContent closeLabel={commonCopy.close} onOpenAutoFocus={event => { event.preventDefault(); cancelRef.current?.focus(); }}
      onCloseAutoFocus={event => { if (onReturnFocus) { event.preventDefault(); onReturnFocus(); } }}>
      <DialogHeader>
        <DialogTitle>{connectionsCopy.disconnectTitle}</DialogTitle>
        <DialogDescription asChild>
          <div className="min-w-0"><TruncatedText text={connectionsCopy.disconnectDescription(session?.service_display_name ?? '')} lines={2} expandLabel={commonCopy.showMore} collapseLabel={commonCopy.showLess} /></div>
        </DialogDescription>
      </DialogHeader>
      <p className="text-sm text-foreground">{connectionsCopy.providerWarning}</p>
      {loading && <p role="status">{commonCopy.loading}</p>}
      {error && <div role="alert" className="space-y-2">
        <p>{connectionsCopy.detailsError}</p>
        <Button variant="outline" onClick={onRetry}>{commonCopy.retry}</Button>
      </div>}
      {affectedAgents && (affectedAgents.length > 0 ? <div className="space-y-2">
        <p className="text-sm">{connectionsCopy.dependentAgents}</p>
        <ul className="space-y-2">{affectedAgents.map(agent => <li key={agent.agent_id}>
          <TruncatedText text={agent.display_name} lines={2} expandLabel={commonCopy.showMore} collapseLabel={commonCopy.showLess} />
        </li>)}</ul>
      </div> : <p className="text-sm text-muted-foreground">{connectionsCopy.noDependentAgents}</p>)}
      <DialogFooter>
        <Button ref={cancelRef} variant="outline" onClick={onCancel}>{commonCopy.cancel}</Button>
        <Button variant="destructive" disabled={!affectedAgents || loading} onClick={onConfirm}>{accessCopy.disconnect}</Button>
      </DialogFooter>
    </DialogContent>
  </Dialog>;
}

export function DisconnectDialog({ session, ...callbacks }: DisconnectDialogProps) {
  const { principal } = usePrincipal();
  const affectedAgents = useQuery({
    queryKey: [...queryKeys.connections(principal), session?.service_id, 'affected-agents'],
    queryFn: ({ signal }) => sessionsApi.getAffectedAgents(session!.service_id, { signal }),
    enabled: session !== null,
    staleTime: 0,
  });
  // Never allow confirmation against cached dependencies while a current read is pending or failed.
  return <DisconnectDialogView session={session} {...callbacks} affectedAgents={session && affectedAgents.isSuccess && !affectedAgents.isFetching ? affectedAgents.data : null}
    loading={session !== null && (affectedAgents.isPending || affectedAgents.isFetching)} error={session !== null && affectedAgents.isError}
    onRetry={() => void affectedAgents.refetch()} />;
}
