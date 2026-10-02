import { Button } from '@design-system/components/primitives/Button';
import { Dialog, DialogContent, DialogHeader, DialogTitle, DialogDescription, DialogFooter } from '@design-system/components/overlays/Dialog';
import { TruncatedText } from '@design-system/components/data-display/TruncatedText';
import { commonCopy } from '@copy';
import { approvalCopy } from '@copy/approvals';
import type { ToolApprovalDetail } from '../../types/approval';

interface PermanentDenialDialogProps {
  approval: ToolApprovalDetail;
  submitting: boolean;
  onCancel: () => void;
  onConfirm: () => void;
  onReturnFocus: () => void;
}

export default function PermanentDenialDialog({ approval, submitting, onCancel, onConfirm, onReturnFocus }: PermanentDenialDialogProps) {
  return <Dialog open onOpenChange={(open) => { if (!open) onCancel(); }}>
    <DialogContent closeLabel={commonCopy.close} onCloseAutoFocus={(event) => { event.preventDefault(); onReturnFocus(); }}>
      <DialogHeader>
        <DialogTitle>{approvalCopy.confirmPermanentDeny}</DialogTitle>
        <DialogDescription asChild>
          <div className="min-w-0 space-y-2">
            <TruncatedText text={approvalCopy.decisionContext(approval.tool_name, approval.agent_display_name ?? approval.agent_id)} lines={2} expandLabel={commonCopy.showMore} collapseLabel={commonCopy.showLess} />
            <p>{approvalCopy.denyPermanentDescription}</p>
          </div>
        </DialogDescription>
      </DialogHeader>
      <DialogFooter>
        <Button variant="secondary" disabled={submitting} onClick={onCancel}>{commonCopy.cancel}</Button>
        <Button variant="secondary" disabled={submitting} isLoading={submitting} onClick={onConfirm}>{approvalCopy.confirmPermanentDeny}</Button>
      </DialogFooter>
    </DialogContent>
  </Dialog>;
}
