import { useRef } from 'react';
import { toast } from 'sonner';
import { Button } from '@design-system/components/primitives/Button';
import { TruncatedText } from '@design-system/components/data-display/TruncatedText';
import { PageHeader } from '@design-system/components/layout/PageHeader';
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from '@design-system/components/overlays/Dialog';
import { PendingApprovalsTable } from '@components/approvals/PendingApprovalsTable';
import { StandingDecisionsTable } from '@components/approvals/StandingDecisionsTable';
import { usePendingApprovals } from '@hooks/usePendingApprovals';
import { useStandingApprovals } from '@hooks/useStandingApprovals';
import { approvalCopy, commonCopy, navigationCopy } from '@copy';
import { approvalQueueCopy as copy } from '@copy/approvalQueue';
import type { ToolApprovalDetail } from '../types/approval';

export default function ApprovalsPage() {
  const pending = usePendingApprovals();
  const standing = useStandingApprovals({ onRevokeSuccess: () => toast.success(copy.revokeSuccess) });
  const revokeTrigger = useRef<HTMLButtonElement | null>(null);
  const standingHeading = useRef<HTMLHeadingElement | null>(null);
  const allowed = standing.data?.filter(approval => approval.status === 'approved') ?? [];
  const denied = standing.data?.filter(approval => approval.status === 'denied') ?? [];

  function requestRevoke(approval: ToolApprovalDetail, trigger: HTMLButtonElement) {
    revokeTrigger.current = trigger;
    standing.revoke.requestRevoke(approval);
  }

  return <div className="min-w-0 space-y-8">
    <PageHeader title={navigationCopy.approvals} purpose={copy.purpose} />
    <section data-testid="approval-section" aria-labelledby="pending-requests-heading" className="min-w-0 space-y-4">
      <h2 id="pending-requests-heading" className="font-display text-xl font-semibold">{copy.pendingTitle}</h2>
      {pending.isPending && <p className="text-muted-foreground">{navigationCopy.pendingLoading}</p>}
      {pending.stale && <div role="alert" className="space-y-2 text-sm text-destructive">
        <p>{pending.data ? navigationCopy.pendingStale : copy.pendingError}</p>
        <Button variant="outline" size="sm" onClick={() => void pending.refetch()}>{commonCopy.retry}</Button>
      </div>}
      {pending.data && (pending.data.length ? <PendingApprovalsTable approvals={pending.data} /> : <div className="space-y-2 rounded-lg border border-border p-4"><p>{copy.pendingEmpty}</p><p className="text-sm text-muted-foreground">{copy.pendingEmptyDescription}</p></div>)}
    </section>
    <div data-testid="standing-decisions" className="min-w-0 space-y-8">
      {standing.isPending && <p className="text-muted-foreground">{copy.standingLoading}</p>}
      {standing.stale && <div role="alert" className="space-y-2 text-sm text-destructive">
        <p>{standing.data ? copy.standingStale : copy.standingError}</p>
        <Button variant="outline" size="sm" onClick={() => void standing.refetch()}>{commonCopy.retry}</Button>
      </div>}
      {standing.revoke.error != null && <p role="alert" className="text-sm text-destructive">{copy.revokeError}</p>}
      <section data-testid="approval-section" aria-labelledby="standing-allow-heading" className="min-w-0 space-y-4">
        <h2 id="standing-allow-heading" ref={standingHeading} tabIndex={-1} className="font-display text-xl font-semibold focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring">{copy.standingAllowTitle}</h2>
        {standing.data && (allowed.length ? <StandingDecisionsTable approvals={allowed} label={copy.standingAllowTitle} onRevoke={requestRevoke} isPending={standing.revoke.isPending} /> : <p className="text-sm text-muted-foreground">{copy.allowEmpty}</p>)}
      </section>
      <section data-testid="approval-section" aria-labelledby="standing-deny-heading" className="min-w-0 space-y-4">
        <h2 id="standing-deny-heading" className="font-display text-xl font-semibold">{copy.standingDenyTitle}</h2>
        {standing.data && (denied.length ? <StandingDecisionsTable approvals={denied} label={copy.standingDenyTitle} onRevoke={requestRevoke} isPending={standing.revoke.isPending} /> : <p className="text-sm text-muted-foreground">{copy.denyEmpty}</p>)}
      </section>
    </div>
    <Dialog open={standing.revoke.confirmation !== null} onOpenChange={open => { if (!open) standing.revoke.cancelRevoke(); }}>
      <DialogContent closeLabel={commonCopy.close} onCloseAutoFocus={event => {
        event.preventDefault();
        const trigger = revokeTrigger.current;
        if (trigger?.isConnected && !trigger.disabled) trigger.focus();
        else standingHeading.current?.focus();
      }}>
        <DialogHeader>
          <DialogTitle>{copy.revokeTitle}</DialogTitle>
          <DialogDescription asChild>
            <div className="min-w-0 space-y-2">
              <p>{copy.revokeDescription}</p>
              {standing.revoke.confirmation && <TruncatedText text={approvalCopy.decisionContext(standing.revoke.confirmation.tool_name, standing.revoke.confirmation.agent_display_name ?? standing.revoke.confirmation.agent_id)} lines={2} expandLabel={commonCopy.showMore} collapseLabel={commonCopy.showLess} />}
              <p>{copy.revokeEffect}</p>
            </div>
          </DialogDescription>
        </DialogHeader>
        <DialogFooter>
          <Button variant="outline" onClick={standing.revoke.cancelRevoke}>{commonCopy.cancel}</Button>
          <Button variant="destructive" onClick={() => void standing.revoke.confirmRevoke()}>{copy.confirmRevoke}</Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  </div>;
}
