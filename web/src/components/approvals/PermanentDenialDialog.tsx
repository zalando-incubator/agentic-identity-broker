import { Button } from '@design-system/components/primitives/Button';
import { Dialog, DialogContent, DialogHeader, DialogTitle, DialogDescription, DialogFooter } from '@design-system/components/overlays/Dialog';
import { commonCopy } from '@copy';
import { approvalCopy } from '@copy/approvals';
import type { ToolApprovalDetail } from '../../types/approval';
import { ApprovalDecisionIdentity } from './ApprovalDecisionIdentity';

interface PermanentDenialDialogProps {
  approval: ToolApprovalDetail;
  submitting: boolean;
  submittingAction: 'approve' | 'deny' | null;
  onCancel: () => void;
  onConfirm: () => void;
  onReturnFocus: () => void;
}

export default function PermanentDenialDialog({ approval, submitting, submittingAction, onCancel, onConfirm, onReturnFocus }: PermanentDenialDialogProps) {
  return <Dialog open onOpenChange={(open) => { if (!open) onCancel(); }}>
    <DialogContent closeLabel={commonCopy.close} onCloseAutoFocus={(event) => { event.preventDefault(); onReturnFocus(); }}>
      <DialogHeader>
        <DialogTitle>{approvalCopy.confirmPermanentDeny}</DialogTitle>
        <DialogDescription asChild>
          <div className="min-w-0 space-y-2">
            <ApprovalDecisionIdentity toolName={approval.tool_name} agentName={approval.agent_display_name ?? approval.agent_id} />
            <p>{approvalCopy.denyPermanentDescription}</p>
          </div>
        </DialogDescription>
      </DialogHeader>
      <DialogFooter>
        <Button variant="secondary" disabled={submitting} onClick={onCancel}>{commonCopy.cancel}</Button>
        <Button variant="destructive" disabled={submitting} isLoading={submittingAction === 'deny'} onClick={onConfirm}>{approvalCopy.confirmPermanentDeny}</Button>
      </DialogFooter>
    </DialogContent>
  </Dialog>;
}
