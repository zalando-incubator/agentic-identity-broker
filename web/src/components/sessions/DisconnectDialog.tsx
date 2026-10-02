import { useQuery } from '@tanstack/react-query';
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from '@design-system/components/overlays/Dialog';
import { TruncatedText } from '@design-system/components/data-display/TruncatedText';
import { Button } from '@design-system/components/primitives/Button';
import { accessCopy, commonCopy } from '@copy';
import { connectionsCopy } from '@copy/connections';
import { sessionsApi, type SessionSummary } from '@services/api/sessions';
import { usePrincipal } from '@services/query/QueryProvider';
import { queryKeys } from '@services/query/queryKeys';

export interface DisconnectDialogProps {
  session: SessionSummary | null;
  onCancel: () => void;
  onConfirm: () => void;
  onReturnFocus?: () => void;
}

export function DisconnectDialog({ session, onCancel, onConfirm, onReturnFocus }: DisconnectDialogProps) {
  const { principal } = usePrincipal();
  const details = useQuery({
    queryKey: [...queryKeys.connections(principal), session?.service_id],
    queryFn: ({ signal }) => sessionsApi.getSessionDetails(session!.service_id, { signal }),
    enabled: session !== null,
    staleTime: 0,
  });
  // Do not confirm against a cached dependency list while its current read is pending or failed.
  const canConfirm = session !== null && details.isSuccess && !details.isFetching;
  return <Dialog open={session !== null} onOpenChange={open => { if (!open) onCancel(); }}>
    <DialogContent closeLabel={commonCopy.close} onCloseAutoFocus={event => { if (onReturnFocus) { event.preventDefault(); onReturnFocus(); } }}>
      <DialogHeader>
        <DialogTitle>{connectionsCopy.disconnectTitle}</DialogTitle>
        <DialogDescription asChild>
          <div className="min-w-0">
            <TruncatedText text={connectionsCopy.disconnectDescription(session?.service_display_name ?? '')} lines={2} expandLabel={commonCopy.showMore} collapseLabel={commonCopy.showLess} />
          </div>
        </DialogDescription>
      </DialogHeader>
      <p className="text-sm text-foreground">{connectionsCopy.providerWarning}</p>
      {details.isPending && <p role="status">{commonCopy.loading}</p>}
      {details.isError && <div role="alert" className="space-y-2">
        <p>{connectionsCopy.detailsError}</p>
        <Button variant="outline" onClick={() => void details.refetch()}>{commonCopy.retry}</Button>
      </div>}
      {details.isSuccess && (details.data.dependent_agents.length > 0 ? <div className="space-y-2">
        <p className="text-sm">{connectionsCopy.dependentAgents}</p>
        <ul className="space-y-2">{details.data.dependent_agents.map(agent => <li key={agent.id}>
          <TruncatedText text={agent.display_name} lines={2} expandLabel={commonCopy.showMore} collapseLabel={commonCopy.showLess} />
        </li>)}</ul>
      </div> : <p className="text-sm text-muted-foreground">{connectionsCopy.noDependentAgents}</p>)}
      <DialogFooter>
        <Button variant="outline" onClick={onCancel}>{commonCopy.cancel}</Button>
        <Button variant="destructive" disabled={!canConfirm} onClick={() => { if (canConfirm) onConfirm(); }}>{accessCopy.disconnect}</Button>
      </DialogFooter>
    </DialogContent>
  </Dialog>;
}
